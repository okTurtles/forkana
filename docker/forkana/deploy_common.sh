#!/usr/bin/env bash
# deploy_common.sh - Shared library for Forkana deploy scripts.
#
# Sourced by deploy_debian.sh and deploy_fedora.sh.
# This file only defines functions - it does NOT execute any code when sourced.

# Guard against direct execution.
if [[ "${BASH_SOURCE[0]}" == "${0}" ]]; then
  echo "ERROR: deploy_common.sh must be sourced, not executed directly." >&2
  exit 1
fi

# ---------------------------------------------------------------------------
# Logging helpers
# ---------------------------------------------------------------------------
log()  { printf '[deploy %s] %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"; }
die()  { log "FATAL: $*"; exit 1; }

# ---------------------------------------------------------------------------
# deploy_init - Set up configuration variables and resolve commit SHA.
#
# Arguments:
#   $1 - commit SHA (or empty; falls back to SSH_ORIGINAL_COMMAND)
#   $2 - optional OS label for log messages (e.g. "Fedora")
# ---------------------------------------------------------------------------
deploy_init() {
  local commit_arg="${1:-}"
  local os_label="${2:-}"

  # --- Configuration ---
  DEPLOY_DIR="${HOME}/forkana"

  # Detect whether we are inside a git checkout that contains dev.yml.
  # If so, use the working tree to locate dev.yml for local testing
  # (Step 1 copies it to COMPOSE_DIR).  On production servers REPO_DIR
  # will not contain dev.yml, so Step 1 expects it pre-deployed.
  LOCAL_REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || true)"
  if [[ -n "${LOCAL_REPO_ROOT}" && -f "${LOCAL_REPO_ROOT}/docker/forkana/dev.yml" ]]; then
    REPO_DIR="${LOCAL_REPO_ROOT}"
  else
    REPO_DIR="${DEPLOY_DIR}/repo"
  fi
  COMPOSE_DIR="${DEPLOY_DIR}/compose"
  REGISTRY="localhost:5000"
  # Use explicit IPv4 for curl checks - avoids IPv6 resolution issues on hosts
  # where "localhost" may resolve to ::1 before 127.0.0.1.
  REGISTRY_HTTP="http://127.0.0.1:5000"
  IMAGE_NAME="forkana"
  PROJECT_NAME="forkana"
  HOST_PORT="${FORKANA_HOST_PORT:-3000}"
  COMPOSE_BASE="${COMPOSE_DIR}/dev.yml"
  COMPOSE_OVERRIDE="${COMPOSE_DIR}/compose.override.yml"

  # --- Resolve commit SHA - prefer argument, fall back to SSH_ORIGINAL_COMMAND ---
  COMMIT_SHA="${commit_arg:-${SSH_ORIGINAL_COMMAND:-}}"
  [[ -z "${COMMIT_SHA}" ]] && die "No commit SHA provided."

  # Validate: exactly 40 lowercase hex characters.
  if [[ ! "${COMMIT_SHA}" =~ ^[0-9a-f]{40}$ ]]; then
    SAFE_DISPLAY="$(printf '%.80s' "${COMMIT_SHA}" | tr -cd '[:print:]')"
    die "Invalid commit SHA: '${SAFE_DISPLAY}' (expected 40-char hex)."
  fi

  if [[ -n "${os_label}" ]]; then
    log "Starting deployment for commit ${COMMIT_SHA} (${os_label})"
  else
    log "Starting deployment for commit ${COMMIT_SHA}"
  fi
}

# ---------------------------------------------------------------------------
# deploy_run - Execute the full deployment pipeline (steps 1–10).
#
# Expects deploy_init to have been called first.
# If the caller defines an install_os_packages function, it is called
# before the pipeline begins (step 0).
# ---------------------------------------------------------------------------
deploy_run() {
  # Step 0: OS-specific package installation (if defined by caller).
  if declare -F install_os_packages > /dev/null 2>&1; then
    install_os_packages
  fi

  # --- Step 1: Prepare compose directory and base file ---
  mkdir -p "${COMPOSE_DIR}"
  if [[ -f "${REPO_DIR}/docker/forkana/dev.yml" ]]; then
    cp "${REPO_DIR}/docker/forkana/dev.yml" "${COMPOSE_BASE}"
    log "Copied dev.yml to ${COMPOSE_BASE}"
  else
    if [[ ! -f "${COMPOSE_BASE}" ]]; then
      die "Missing ${COMPOSE_BASE} - ensure dev.yml is deployed to ${COMPOSE_DIR}"
    fi
    log "Using pre-deployed dev.yml at ${COMPOSE_BASE}"
  fi

  printf 'services:\n  forkana:\n    image: localhost:5000/forkana:latest\n' \
    > "${COMPOSE_OVERRIDE}"
  log "Wrote placeholder ${COMPOSE_OVERRIDE}"

  # --- Step 1b: Ensure host volume directories exist with correct ownership ---
  log "Ensuring volume directories exist with correct ownership..."
  for dir in \
    "${DEPLOY_DIR}/data" \
    "${DEPLOY_DIR}/data/git" \
    "${DEPLOY_DIR}/data/custom" \
    "${DEPLOY_DIR}/config" \
    "${DEPLOY_DIR}/postgres"; do
    mkdir -p "${dir}"
    chmod 0755 "${dir}"
  done

  # --- Step 1c: Verify .env exists before any docker compose invocation ---
  # This must happen before Step 2 (registry startup) because dev.yml interpolates
  # environment variables like ${POSTGRES_PASSWORD}. If .env is missing, docker compose
  # will fail with a confusing error. Validate early to give users a clear message.
  ENV_FILE="${COMPOSE_DIR}/.env"
  if [[ ! -f "${ENV_FILE}" ]]; then
    die "Missing ${ENV_FILE} - create it with POSTGRES_PASSWORD, FORKANA_DOMAIN, FORKANA_SECRET_KEY, FORKANA_INTERNAL_TOKEN, and FORKANA_JWT_SECRET (see DEPLOYMENT_GUIDE.md)."
  fi

  local required_vars=(POSTGRES_PASSWORD FORKANA_DOMAIN FORKANA_SECRET_KEY FORKANA_INTERNAL_TOKEN FORKANA_JWT_SECRET)
  local missing=()

  for var in "${required_vars[@]}"; do
    if ! grep -Eq "^[[:space:]]*${var}[[:space:]]*=[[:space:]]*[^[:space:]#]+" "${ENV_FILE}"; then
      missing+=("${var}")
    fi
  done

  if (( ${#missing[@]} > 0 )); then
    die "Missing or empty required variables in ${ENV_FILE}: ${missing[*]}. Please set valid values before deploying."
  fi

  # --- Step 2: Ensure the local registry is running ---
  # The registry on 127.0.0.1:5000 may be shared infrastructure (not owned by
  # Forkana).  Reuse it if already reachable; only start one via compose if
  # nothing is listening.
  log "Ensuring local registry is running..."
  if ! curl -sf "${REGISTRY_HTTP}/v2/" > /dev/null 2>&1; then
    # Check for an existing container named "registry" (shared) or
    # "forkana-registry" (compose-managed).
    REGISTRY_STATE=""
    for cname in registry forkana-registry; do
      REGISTRY_STATE="$(docker inspect -f '{{.State.Status}}' "${cname}" 2>/dev/null || true)"
      if [[ "${REGISTRY_STATE}" == "running" ]]; then
        log "Found running container '${cname}' - waiting for it to respond..."
        break
      fi
    done

    if [[ "${REGISTRY_STATE}" != "running" ]]; then
      log "No registry container running - starting one via compose..."
      docker compose \
        -p "${PROJECT_NAME}" \
        --project-directory "${COMPOSE_DIR}" \
        -f "${COMPOSE_BASE}" \
        -f "${COMPOSE_OVERRIDE}" \
        up -d registry
    fi

    log "Waiting for registry to become ready..."
    for i in $(seq 1 12); do
      if curl -sf "${REGISTRY_HTTP}/v2/" > /dev/null 2>&1; then
        break
      fi
      if [[ "$i" -eq 12 ]]; then
        die "Registry did not become ready within 60s."
      fi
      log "  attempt ${i}/12 - registry not ready yet"
      sleep 5
    done
  fi
  log "Registry is ready at ${REGISTRY}."

  # --- Step 3: Load pre-built image ---
  IMAGE_TAG="sha-${COMMIT_SHA::7}"
  FULL_TAG="${REGISTRY}/${IMAGE_NAME}:${IMAGE_TAG}"
  TARBALL="${DEPLOY_DIR}/images/forkana-${COMMIT_SHA::7}.tar.gz"

  log "Loading pre-built image from ${TARBALL} ..."
  if [[ ! -f "${TARBALL}" ]]; then
    die "Missing image tarball: ${TARBALL}. Ensure GitHub Actions transferred it."
  fi

  docker load -i "${TARBALL}"
  log "Image loaded successfully."

  # --- Step 4: Tag for local registry ---
  log "Tagging image as ${FULL_TAG} ..."
  # The image in the tarball is tagged forkana:${COMMIT_SHA} by the CI workflow.
  # We tag it for our local registry so it can be pushed in Step 5.
  docker tag "forkana:${COMMIT_SHA}" "${FULL_TAG}" 2>/dev/null || \
    docker tag "forkana:${IMAGE_TAG}" "${FULL_TAG}" 2>/dev/null || \
    die "Failed to tag loaded image as ${FULL_TAG}. The tarball may not contain the expected image tag."

  # --- Step 5: Push to local registry ---
  log "Pushing ${FULL_TAG} to local registry..."
  docker push "${FULL_TAG}"

  # --- Step 6: Resolve the pushed digest (immutable reference) ---
  DIGEST="$(docker inspect --format='{{if .RepoDigests}}{{index .RepoDigests 0}}{{end}}' "${FULL_TAG}" \
            | sed 's/.*@//')"
  [[ -z "${DIGEST}" ]] && die "Failed to resolve digest for ${FULL_TAG}. Push may have failed - check registry connectivity."

  PINNED_REF="${REGISTRY}/${IMAGE_NAME}@${DIGEST}"
  log "Resolved digest: ${DIGEST}"
  log "Pinned image ref: ${PINNED_REF}"

  # --- Step 7: Generate compose.override.yml (digest-pinned) ---
  printf 'services:\n  forkana:\n    image: %s\n' "${PINNED_REF}" \
    > "${COMPOSE_OVERRIDE}"
  log "Wrote ${COMPOSE_OVERRIDE}"

  # --- Step 8: Deploy with docker compose ---
  log "Running docker compose up..."
  docker compose \
    -p "${PROJECT_NAME}" \
    --project-directory "${COMPOSE_DIR}" \
    -f "${COMPOSE_BASE}" \
    -f "${COMPOSE_OVERRIDE}" \
    up -d --remove-orphans

  # --- Step 9: Health check ---
  log "Waiting for health check (up to 150s)..."
  for i in $(seq 1 30); do
    if curl -sf "http://127.0.0.1:${HOST_PORT}/api/healthz" > /dev/null 2>&1; then
      log "Service is healthy!"
      log "Deployment of ${COMMIT_SHA} complete."
      exit 0
    fi
    log "  attempt ${i}/30 - not ready yet"
    sleep 5
  done

  log "ERROR: Service did not become healthy within 150s."
  docker compose \
    -p "${PROJECT_NAME}" \
    --project-directory "${COMPOSE_DIR}" \
    -f "${COMPOSE_BASE}" \
    -f "${COMPOSE_OVERRIDE}" \
    logs --tail=50 forkana
  exit 1
}
