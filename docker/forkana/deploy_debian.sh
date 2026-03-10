#!/usr/bin/env bash
# deploy.sh — Build-on-server deploy script for Forkana.
#
# Invoked via SSH forced-command from GitHub Actions.  The commit SHA is
# passed either as $1 (direct invocation) or via SSH_ORIGINAL_COMMAND
# (forced-command mode).
#
# Usage:  deploy.sh <40-char-hex-commit-sha>
#
# Local testing:
#   Run from anywhere inside the git checkout.  The script detects the
#   repo root via `git rev-parse` and skips fetch/checkout (step 3).
#     ./docker/forkana/deploy.sh "$(git rev-parse HEAD)"

set -euo pipefail

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
DEPLOY_DIR="${HOME}/forkana"

# Detect whether we are already inside a git checkout that contains the
# deploy infrastructure.  If so, use the working tree directly (local
# testing mode) instead of the production clone at ${DEPLOY_DIR}/repo.
LOCAL_REPO_ROOT="$(git rev-parse --show-toplevel 2>/dev/null || true)"
if [[ -n "${LOCAL_REPO_ROOT}" && -f "${LOCAL_REPO_ROOT}/docker/forkana/dev.yml" ]]; then
  LOCAL_MODE=true
  REPO_DIR="${LOCAL_REPO_ROOT}"
else
  LOCAL_MODE=false
  REPO_DIR="${DEPLOY_DIR}/repo"
fi
COMPOSE_DIR="${DEPLOY_DIR}/compose"
REGISTRY="localhost:5000"
# Use explicit IPv4 for curl checks — avoids IPv6 resolution issues on hosts
# where "localhost" may resolve to ::1 before 127.0.0.1.
REGISTRY_HTTP="http://127.0.0.1:5000"
IMAGE_NAME="forkana"
PROJECT_NAME="forkana"
COMPOSE_BASE="${COMPOSE_DIR}/dev.yml"
COMPOSE_OVERRIDE="${COMPOSE_DIR}/compose.override.yml"

# ---------------------------------------------------------------------------
# Logging helpers
# ---------------------------------------------------------------------------
log()  { printf '[deploy %s] %s\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)" "$*"; }
die()  { log "FATAL: $*"; exit 1; }

# ---------------------------------------------------------------------------
# Resolve commit SHA — prefer $1, fall back to SSH_ORIGINAL_COMMAND
# ---------------------------------------------------------------------------
COMMIT_SHA="${1:-${SSH_ORIGINAL_COMMAND:-}}"
[[ -z "${COMMIT_SHA}" ]] && die "No commit SHA provided."

# Validate: exactly 40 lowercase hex characters.
if [[ ! "${COMMIT_SHA}" =~ ^[0-9a-f]{40}$ ]]; then
  SAFE_DISPLAY="$(printf '%.80s' "${COMMIT_SHA}" | tr -cd '[:print:]')"
  die "Invalid commit SHA: '${SAFE_DISPLAY}' (expected 40-char hex)."
fi

log "Starting deployment for commit ${COMMIT_SHA}"

# ---------------------------------------------------------------------------
# 1. Prepare compose directory and base file
# ---------------------------------------------------------------------------
# Copy dev.yml early so that every compose invocation (registry startup in
# step 2 and full deploy in step 10) uses the same file + project-directory +
# .env context.  This keeps the config hash identical and prevents compose
# from needlessly recreating the registry container.
mkdir -p "${COMPOSE_DIR}"
cp "${REPO_DIR}/docker/forkana/dev.yml" "${COMPOSE_BASE}"
log "Copied dev.yml to ${COMPOSE_BASE}"

# Generate a placeholder compose.override.yml so that step 2 (registry) and
# step 10 (full deploy) use the SAME set of compose files.  Docker Compose
# hashes the resolved config per-service; if the input file set differs
# between invocations the hash changes and compose recreates the registry.
# Step 7 overwrites this with the real digest-pinned image reference.
printf 'services:\n  forkana:\n    image: localhost:5000/forkana:latest\n' \
  > "${COMPOSE_OVERRIDE}"
log "Wrote placeholder ${COMPOSE_OVERRIDE}"

# ---------------------------------------------------------------------------
# 1b. Ensure host volume directories exist with correct ownership
# ---------------------------------------------------------------------------
# The Forkana container runs as UID 1000 (git:git).  The deploy user must
# also be UID 1000 so that directories it creates are automatically owned
# by the container user — no sudo or chown required.
#
# Pre-create the specific subdirectories the entrypoint (docker-setup.sh)
# needs so its mkdir calls become no-ops.  Without this, Docker may
# auto-create them as root, causing "Operation not permitted" inside the
# container.
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

# ---------------------------------------------------------------------------
# 2. Ensure the local registry is running
# ---------------------------------------------------------------------------
log "Ensuring local registry is running..."
if ! curl -sf "${REGISTRY_HTTP}/v2/" > /dev/null 2>&1; then
  # Registry isn't responding yet.  Check whether the container exists but is
  # still starting (e.g. healthcheck hasn't passed yet) vs not running at all.
  REGISTRY_STATE="$(docker inspect -f '{{.State.Status}}' forkana-registry 2>/dev/null || true)"

  if [[ "${REGISTRY_STATE}" != "running" ]]; then
    log "Registry container not running (state: '${REGISTRY_STATE:-absent}') — starting it via compose..."
    docker compose \
      -p "${PROJECT_NAME}" \
      --project-directory "${COMPOSE_DIR}" \
      -f "${COMPOSE_BASE}" \
      -f "${COMPOSE_OVERRIDE}" \
      up -d registry
  else
    log "Registry container is running but not yet responding — waiting..."
  fi

  log "Waiting for registry to become ready..."
  for i in $(seq 1 12); do
    if curl -sf "${REGISTRY_HTTP}/v2/" > /dev/null 2>&1; then
      break
    fi
    if [[ "$i" -eq 12 ]]; then
      die "Registry did not become ready within 60s."
    fi
    log "  attempt ${i}/12 — registry not ready yet"
    sleep 5
  done
fi
log "Registry is ready at ${REGISTRY}."

# ---------------------------------------------------------------------------
# 3. Git fetch & checkout
# ---------------------------------------------------------------------------
if [[ "${LOCAL_MODE}" == true ]]; then
  log "Local mode — skipping git fetch/checkout (using working tree at ${REPO_DIR})"
else
  log "Fetching latest objects..."
  cd "${REPO_DIR}"
  git fetch --all --prune --quiet
  git checkout --force "${COMMIT_SHA}"
  git submodule update --init --recursive --quiet
  log "Checked out ${COMMIT_SHA}"

  # Re-copy dev.yml after checkout so the compose base matches this commit.
  cp "${REPO_DIR}/docker/forkana/dev.yml" "${COMPOSE_BASE}"
fi

# Ensure CWD is the repo root for docker build (relative Dockerfile path).
cd "${REPO_DIR}"

# ---------------------------------------------------------------------------
# 4. Docker build
# ---------------------------------------------------------------------------
IMAGE_TAG="sha-${COMMIT_SHA::7}"
FULL_TAG="${REGISTRY}/${IMAGE_NAME}:${IMAGE_TAG}"

log "Building image ${FULL_TAG} ..."
docker build \
  --file docker/forkana/Dockerfile \
  --tag "${FULL_TAG}" \
  .
log "Build complete."

# ---------------------------------------------------------------------------
# 5. Push to local registry
# ---------------------------------------------------------------------------
log "Pushing ${FULL_TAG} to local registry..."
docker push "${FULL_TAG}"

# ---------------------------------------------------------------------------
# 6. Resolve the pushed digest (immutable reference)
# ---------------------------------------------------------------------------
DIGEST="$(docker inspect --format='{{index .RepoDigests 0}}' "${FULL_TAG}" \
          | sed 's/.*@//')"
[[ -z "${DIGEST}" ]] && die "Failed to resolve digest for ${FULL_TAG}."

PINNED_REF="${REGISTRY}/${IMAGE_NAME}@${DIGEST}"
log "Resolved digest: ${DIGEST}"
log "Pinned image ref: ${PINNED_REF}"

# ---------------------------------------------------------------------------
# 7. Generate compose.override.yml (digest-pinned)
# ---------------------------------------------------------------------------
printf 'services:\n  forkana:\n    image: %s\n' "${PINNED_REF}" \
  > "${COMPOSE_OVERRIDE}"
log "Wrote ${COMPOSE_OVERRIDE}"

# ---------------------------------------------------------------------------
# 8. Verify .env exists (secrets must be provisioned before first deploy)
# ---------------------------------------------------------------------------
ENV_FILE="${COMPOSE_DIR}/.env"
if [[ ! -f "${ENV_FILE}" ]]; then
  die "Missing ${ENV_FILE} — create it with POSTGRES_PASSWORD, FORKANA_DOMAIN, FORKANA_SECRET_KEY, FORKANA_INTERNAL_TOKEN, and FORKANA_JWT_SECRET (see DEPLOYMENT_GUIDE.md)."
fi

# ---------------------------------------------------------------------------
# 9. Deploy with docker compose
# (Volume ownership was already fixed in step 1b.)
# ---------------------------------------------------------------------------
log "Running docker compose up..."
docker compose \
  -p "${PROJECT_NAME}" \
  --project-directory "${COMPOSE_DIR}" \
  -f "${COMPOSE_BASE}" \
  -f "${COMPOSE_OVERRIDE}" \
  up -d --remove-orphans

# ---------------------------------------------------------------------------
# 10. Health check
# ---------------------------------------------------------------------------
log "Waiting for health check (up to 150s)..."
for i in $(seq 1 30); do
  if curl -sf http://127.0.0.1:3000/api/healthz > /dev/null 2>&1; then
    log "Service is healthy!"
    log "Deployment of ${COMMIT_SHA} complete."
    exit 0
  fi
  log "  attempt ${i}/30 — not ready yet"
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
