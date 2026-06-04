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
# read_env_var FILE NAME
#
# Echo the parsed value of NAME from a dotenv-style FILE.  Last definition
# wins; surrounding whitespace and a single layer of matching quotes are
# stripped.  Echoes the empty string when FILE is missing, NAME is absent,
# or the value resolves to empty (matching docker compose's env_file
# behavior).  Safe to call under set -euo pipefail.
# ---------------------------------------------------------------------------
read_env_var() {
  local file="$1" name="$2"
  [[ -f "${file}" ]] || return 0
  { grep -E "^[[:space:]]*${name}[[:space:]]*=" "${file}" || true; } \
    | tail -n1 \
    | cut -d= -f2- \
    | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//' \
          -e 's/^"\(.*\)"$/\1/' -e "s/^'\(.*\)'$/\1/"
}

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

  # --- Deploy mode resolution ---
  # Default to standalone for backward compatibility; forums hosts opt in by
  # writing ${DEPLOY_DIR}/deploy.conf.  See DEPLOYMENT_GUIDE.md
  # § "Deployment mode".
  FORKANA_DEPLOY_MODE="${FORKANA_DEPLOY_MODE:-standalone}"
  if [[ -f "${DEPLOY_DIR}/deploy.conf" ]]; then
    # shellcheck source=/dev/null
    . "${DEPLOY_DIR}/deploy.conf"
  fi

  REGISTRY_PORT="${REGISTRY_PORT:-5000}"
  REGISTRY="localhost:${REGISTRY_PORT}"
  # Use explicit IPv4 for curl checks - avoids IPv6 resolution issues on hosts
  # where "localhost" may resolve to ::1 before 127.0.0.1.
  REGISTRY_HTTP="http://127.0.0.1:${REGISTRY_PORT}"
  IMAGE_NAME="forkana"

  case "${FORKANA_DEPLOY_MODE}" in
    standalone)
      PROJECT_NAME="forkana"
      COMPOSE_DIR="${DEPLOY_DIR}/compose"
      COMPOSE_BASE="${COMPOSE_DIR}/dev.yml"
      COMPOSE_OVERRIDE="${COMPOSE_DIR}/compose.override.yml"
      COMPOSE_FILES=( -f "${COMPOSE_BASE}" -f "${COMPOSE_OVERRIDE}" )
      COMPOSE_PROJECT_DIRECTORY="${COMPOSE_DIR}"
      ENV_FILE="${COMPOSE_DIR}/.env"
      SNAPSHOT_DIR="${COMPOSE_DIR}"
      MANAGE_REGISTRY=1
      MANAGE_DATA_DIRS=1
      REQUIRED_VARS=(POSTGRES_PASSWORD FORKANA_DOMAIN FORKANA_SECRET_KEY \
                     FORKANA_INTERNAL_TOKEN FORKANA_JWT_SECRET)
      ;;
    forums)
      [[ -n "${FORUMS_DIR:-}" ]] || die "FORUMS_DIR not set in ${DEPLOY_DIR}/deploy.conf (required when FORKANA_DEPLOY_MODE=forums)."
      [[ -d "${FORUMS_DIR}"   ]] || die "FORUMS_DIR does not exist: ${FORUMS_DIR}"
      # Operator-overridable knobs; defaults are layout-agnostic so the
      # public code does not bake in any specific external-stack layout.
      # Relative paths resolve against ${FORUMS_DIR}.  See
      # DEPLOYMENT_GUIDE.md § "Deployment mode" for the recognised
      # variables.
      FORUMS_ENV_FILE="${FORUMS_ENV_FILE:-${FORUMS_DIR}/.env}"
      [[ "${FORUMS_ENV_FILE}" = /* ]] || FORUMS_ENV_FILE="${FORUMS_DIR}/${FORUMS_ENV_FILE}"
      FORUMS_OVERRIDE_FILE="${FORUMS_OVERRIDE_FILE:-${FORUMS_DIR}/forkana-override.yml}"
      [[ "${FORUMS_OVERRIDE_FILE}" = /* ]] || FORUMS_OVERRIDE_FILE="${FORUMS_DIR}/${FORUMS_OVERRIDE_FILE}"
      PROJECT_NAME="forums"
      COMPOSE_DIR="${FORUMS_DIR}"
      COMPOSE_BASE="${FORUMS_DIR}/docker-compose.yml"
      COMPOSE_OVERRIDE="${FORUMS_OVERRIDE_FILE}"
      COMPOSE_FILES=( -f "${COMPOSE_BASE}" )
      # Optional whitespace-separated list of extra compose files to layer
      # before the digest-pinned override (e.g. a sibling-services file
      # provided by the external stack).  Relative entries resolve against
      # ${FORUMS_DIR}.
      if [[ -n "${FORUMS_EXTRA_COMPOSE_FILES:-}" ]]; then
        local _extra
        for _extra in ${FORUMS_EXTRA_COMPOSE_FILES}; do
          [[ "${_extra}" = /* ]] || _extra="${FORUMS_DIR}/${_extra}"
          COMPOSE_FILES+=( -f "${_extra}" )
        done
      fi
      COMPOSE_FILES+=( -f "${COMPOSE_OVERRIDE}" )
      COMPOSE_PROJECT_DIRECTORY="${FORUMS_DIR}"
      ENV_FILE="${FORUMS_ENV_FILE}"
      SNAPSHOT_DIR="${DEPLOY_DIR}/snapshots"
      MANAGE_REGISTRY=0
      MANAGE_DATA_DIRS=0
      REQUIRED_VARS=(POSTGRES_FORKANA_PASSWORD FORKANA_DOMAIN FORKANA_ROOT_URL \
                     FORKANA_SECRET_KEY FORKANA_INTERNAL_TOKEN FORKANA_JWT_SECRET \
                     FORKANA_HOST_PORT)
      ;;
    *)
      die "Unknown FORKANA_DEPLOY_MODE: '${FORKANA_DEPLOY_MODE}' (expected: standalone | forums)."
      ;;
  esac

  # Resolve HOST_PORT for the health-check probe.  Prefer the exported shell
  # variable; otherwise read FORKANA_HOST_PORT from ${ENV_FILE} so the probe
  # targets the same port Compose binds via interpolation.  Without this,
  # setting FORKANA_HOST_PORT only in the env file (as documented) caused
  # every deploy to fail health-check and roll back.
  HOST_PORT="${FORKANA_HOST_PORT:-3000}"
  if [[ -z "${FORKANA_HOST_PORT:-}" ]]; then
    local env_port
    env_port="$(read_env_var "${ENV_FILE}" FORKANA_HOST_PORT)"
    if [[ -n "${env_port}" ]]; then
      if [[ "${env_port}" =~ ^[0-9]+$ ]]; then
        HOST_PORT="${env_port}"
      else
        die "Invalid FORKANA_HOST_PORT in ${ENV_FILE}: '${env_port}' (must be numeric)."
      fi
    fi
  fi

  # Set by deploy_run step 2; true when this deploy owns the compose-managed
  # registry service, false when an external registry is being reused.
  REGISTRY_MANAGED_BY_US=false

  # --- Resolve commit SHA - prefer argument, fall back to SSH_ORIGINAL_COMMAND ---
  COMMIT_SHA="${commit_arg:-${SSH_ORIGINAL_COMMAND:-}}"
  [[ -z "${COMMIT_SHA}" ]] && die "No commit SHA provided."

  # Validate: exactly 40 lowercase hex characters.
  if [[ ! "${COMMIT_SHA}" =~ ^[0-9a-f]{40}$ ]]; then
    SAFE_DISPLAY="${COMMIT_SHA:0:80}"
    SAFE_DISPLAY="${SAFE_DISPLAY//[^[:print:]]/}"
    die "Invalid commit SHA: '${SAFE_DISPLAY}' (expected 40-char hex)."
  fi

  # Short SHA used for artifact file names (matches GitHub Actions workflow,
  # which writes forkana-${SHORT_SHA}.tar.gz to ~/forkana/images/).
  SHORT_SHA="${COMMIT_SHA:0:7}"

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
  # Standalone owns the compose layout under ${COMPOSE_DIR}.  In forums mode
  # the compose files live inside ${FORUMS_DIR} and are managed by the forums
  # framework; we only rewrite ${COMPOSE_OVERRIDE} (step 7).
  if (( MANAGE_DATA_DIRS )); then
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

    printf 'services:\n  forkana:\n    image: %s/forkana:latest\n' "${REGISTRY}" \
      > "${COMPOSE_OVERRIDE}"
    log "Wrote placeholder ${COMPOSE_OVERRIDE}"
  else
    # Forums mode: compose layout owned by the external stack.  Verify
    # every -f file we will pass to docker compose exists before any
    # invocation; the digest-pinned override is created below, so skip it.
    local _i _f
    for ((_i = 0; _i < ${#COMPOSE_FILES[@]}; _i++)); do
      [[ "${COMPOSE_FILES[_i]}" == "-f" ]] || continue
      _f="${COMPOSE_FILES[_i + 1]}"
      [[ "${_f}" == "${COMPOSE_OVERRIDE}" ]] && continue
      [[ -f "${_f}" ]] || die "Missing compose file: ${_f} - the external stack must provide it (see deploy.conf knobs FORUMS_EXTRA_COMPOSE_FILES)."
    done
    mkdir -p "$(dirname "${COMPOSE_OVERRIDE}")"
  fi

  # --- Step 1b: Ensure host volume directories exist with correct ownership ---
  # Only chmod when we create the directory: once a container (postgres initdb,
  # gitea entrypoint) has taken ownership of its bind-mounted data dir, a later
  # chmod by the deploying user would return EPERM and break subsequent
  # redeploys.  Forums mode skips this entirely: bind-mount roots for the
  # forkana service are bootstrapped once by the operator with chown 1000:1000.
  if (( MANAGE_DATA_DIRS )); then
    log "Ensuring volume directories exist with correct ownership..."
    for dir in \
      "${DEPLOY_DIR}/data" \
      "${DEPLOY_DIR}/data/git" \
      "${DEPLOY_DIR}/data/custom" \
      "${DEPLOY_DIR}/config" \
      "${DEPLOY_DIR}/postgres"; do
      if [[ ! -d "${dir}" ]]; then
        mkdir -p "${dir}"
        chmod 0755 "${dir}"
      fi
    done
  fi

  # --- Step 1c: Verify env file exists and required vars are populated ---
  # This must happen before Step 2 (registry startup) because the compose
  # files interpolate variables like ${POSTGRES_PASSWORD}.  Validate early
  # to give a clear error message instead of a confusing compose failure.
  # REQUIRED_VARS is mode-specific and set in deploy_init.
  if [[ ! -f "${ENV_FILE}" ]]; then
    die "Missing ${ENV_FILE} - create it with: ${REQUIRED_VARS[*]} (see DEPLOYMENT_GUIDE.md)."
  fi

  local missing=()
  # Parse each value through read_env_var so empty quoted forms (FOO="",
  # FOO='') and surrounding whitespace are caught; a bare "[^space#]+" grep
  # would treat a lone quote as a valid value and let an empty secret reach
  # the container.
  for var in "${REQUIRED_VARS[@]}"; do
    local val
    val="$(read_env_var "${ENV_FILE}" "${var}")"
    if [[ -z "${val}" ]]; then
      missing+=("${var}")
    fi
  done

  if (( ${#missing[@]} > 0 )); then
    die "Missing or empty required variables in ${ENV_FILE}: ${missing[*]}. Please set valid values before deploying."
  fi

  # --- Step 2: Ensure the local registry is running ---
  # The registry on 127.0.0.1:${REGISTRY_PORT} may be shared infrastructure
  # (not owned by Forkana).  In standalone mode, start a compose-managed
  # registry as a fallback when nothing is listening.  In forums mode the
  # registry is always external (managed by the surrounding stack outside
  # this compose project); we only pre-flight that it is reachable.
  log "Ensuring local registry is running..."
  if curl -sf "${REGISTRY_HTTP}/v2/" > /dev/null 2>&1; then
    log "Registry already reachable at ${REGISTRY_HTTP} - reusing it."
    REGISTRY_MANAGED_BY_US=false
  elif (( MANAGE_REGISTRY )); then
    log "No registry on ${REGISTRY_HTTP} - starting compose-managed registry..."
    docker compose \
      -p "${PROJECT_NAME}" \
      --project-directory "${COMPOSE_PROJECT_DIRECTORY}" \
      "${COMPOSE_FILES[@]}" \
      up -d registry
    REGISTRY_MANAGED_BY_US=true

    log "Waiting for registry to become ready..."
    for i in $(seq 1 12); do
      if curl -sf "${REGISTRY_HTTP}/v2/" > /dev/null 2>&1; then break; fi
      if [[ "$i" -eq 12 ]]; then
        die "Registry did not become ready within 60s."
      fi
      log "  attempt ${i}/12 - registry not ready yet"
      sleep 5
    done
  else
    die "External registry not reachable at ${REGISTRY_HTTP} - start the registry on the host before deploying."
  fi
  log "Registry is ready at ${REGISTRY}."

  # --- Step 3: Load pre-built image ---
  IMAGE_TAG="sha-${SHORT_SHA}"
  FULL_TAG="${REGISTRY}/${IMAGE_NAME}:${IMAGE_TAG}"
  TARBALL="${DEPLOY_DIR}/images/forkana-${SHORT_SHA}.tar.gz"

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

  # --- Step 5: Push to local registry (capture digest from push output) ---
  # The digest in `docker push`'s trailer is by definition the digest for
  # the registry just pushed to, so it cannot be confused with a digest
  # from a different registry the image may also be tagged for.
  log "Pushing ${FULL_TAG} to local registry..."
  PUSH_LOG="$(docker push "${FULL_TAG}" 2>&1)" || die "docker push failed: ${PUSH_LOG}"
  printf '%s\n' "${PUSH_LOG}"
  # `docker push` prints a trailer like:
  #   <tag>: digest: sha256:abc... size: 1234
  # Layer IDs earlier in the output are 12-char short IDs without the
  # `sha256:` prefix, so the regex below matches only the digest trailer.
  DIGEST="$(printf '%s\n' "${PUSH_LOG}" \
            | grep -oE 'sha256:[0-9a-f]{64}' | tail -n 1 || true)"

  # --- Step 6: Fall back to a registry-filtered docker inspect if needed ---
  # Filter RepoDigests by the local-registry prefix so we never pick up a
  # digest from a different registry (e.g. GHCR) the image is also tagged
  # for.  `index 0` would not give that guarantee.
  if [[ -z "${DIGEST}" ]]; then
    log "Push output did not include a digest - querying docker inspect..."
    INSPECT_FMT='{{range .RepoDigests}}{{println .}}{{end}}'
    REPO_PREFIX="${REGISTRY}/${IMAGE_NAME}@"
    if ! RAW="$(docker inspect --format="${INSPECT_FMT}" "${FULL_TAG}" 2>&1)"; then
      die "docker inspect failed for ${FULL_TAG}: ${RAW}"
    fi
    LINE="$(printf '%s\n' "${RAW}" | grep -F "${REPO_PREFIX}" | head -n 1 || true)"
    DIGEST="${LINE##*@}"
  fi

  if [[ -z "${DIGEST}" || "${DIGEST}" != sha256:* ]]; then
    die "Could not resolve a digest for ${FULL_TAG} in ${REGISTRY}. Push may have failed."
  fi

  PINNED_REF="${REGISTRY}/${IMAGE_NAME}@${DIGEST}"
  log "Resolved digest: ${DIGEST}"
  log "Pinned image ref: ${PINNED_REF}"

  # --- Step 7: Generate the digest-pinned override file ---
  # Snapshot the current override so step 9 can restore it on failure.
  # On a first deploy the snapshot is the placeholder written in step 1
  # (standalone) or absent (forums); on subsequent deploys it is the
  # previous pinned digest.  In forums mode the snapshot lives under
  # ${SNAPSHOT_DIR} (off the forums working tree) so the rollback artefact
  # never pollutes the shared checkout.
  mkdir -p "${SNAPSHOT_DIR}"
  PREV_OVERRIDE=""
  if [[ -f "${COMPOSE_OVERRIDE}" ]]; then
    PREV_OVERRIDE="${SNAPSHOT_DIR}/$(basename "${COMPOSE_OVERRIDE}").prev"
    cp -f "${COMPOSE_OVERRIDE}" "${PREV_OVERRIDE}"
  fi
  printf 'services:\n  forkana:\n    image: %s\n' "${PINNED_REF}" \
    > "${COMPOSE_OVERRIDE}"
  log "Wrote ${COMPOSE_OVERRIDE}"

  # --- Step 8: Deploy with docker compose ---
  # Select services explicitly so we do not try to (re)bind shared host
  # ports owned by sibling services.  --remove-orphans is intentionally
  # omitted: compose would otherwise classify any unlisted service (the
  # external registry in standalone, or every other forums service in
  # forums mode) as an "orphan" and stop it.
  log "Running docker compose up..."
  local compose_services
  if (( MANAGE_DATA_DIRS )); then
    # Standalone: bring up the full forkana project on a dedicated host.
    compose_services=(postgres forkana)
    if [[ "${REGISTRY_MANAGED_BY_US}" == "true" ]]; then
      compose_services=(registry postgres forkana)
    fi
  else
    # Forums mode: only recreate forkana; siblings (postgres, nginx, ...)
    # are owned by the forums stack and must not be touched here.
    compose_services=(forkana)
  fi
  docker compose \
    -p "${PROJECT_NAME}" \
    --project-directory "${COMPOSE_PROJECT_DIRECTORY}" \
    "${COMPOSE_FILES[@]}" \
    up -d "${compose_services[@]}"

  # --- Step 9: Health check ---
  log "Waiting for health check (up to 150s)..."
  for i in $(seq 1 30); do
    if curl -sf "http://127.0.0.1:${HOST_PORT}/api/healthz" > /dev/null 2>&1; then
      log "Service is healthy!"
      log "Deployment of ${COMMIT_SHA} complete."
      [[ -n "${PREV_OVERRIDE}" && -f "${PREV_OVERRIDE}" ]] && rm -f "${PREV_OVERRIDE}"
      exit 0
    fi
    log "  attempt ${i}/30 - not ready yet"
    sleep 5
  done

  log "ERROR: Service did not become healthy within 150s."
  # GitHub Actions annotation - surfaces the failing image in the job log.
  printf '::error::Deployment of %s failed health check (pinned ref: %s)\n' \
    "${COMMIT_SHA}" "${PINNED_REF}" >&2

  docker compose \
    -p "${PROJECT_NAME}" \
    --project-directory "${COMPOSE_PROJECT_DIRECTORY}" \
    "${COMPOSE_FILES[@]}" \
    logs --tail=100 forkana || true

  if [[ -n "${PREV_OVERRIDE}" && -f "${PREV_OVERRIDE}" ]]; then
    log "Rolling back to previous pinned image from ${PREV_OVERRIDE}..."
    mv -f "${PREV_OVERRIDE}" "${COMPOSE_OVERRIDE}"
    # Re-run compose on forkana only; siblings are already healthy and
    # do not need to be recreated.
    if docker compose \
         -p "${PROJECT_NAME}" \
         --project-directory "${COMPOSE_PROJECT_DIRECTORY}" \
         "${COMPOSE_FILES[@]}" \
         up -d forkana; then
      log "Rollback complete - previous image is active again."
    else
      log "ERROR: Rollback compose up failed; manual intervention required."
    fi
  else
    log "No previous override to roll back to (first deploy?)."
  fi
  exit 1
}
