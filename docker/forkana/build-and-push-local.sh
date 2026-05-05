#!/usr/bin/env bash
# build-and-push-local.sh - Build Forkana image locally and push to the
# local Docker registry (container name: `registry`, port 5000).
#
# This mirrors the build step from .github/workflows/deploy-forkana-dev.yml
# but keeps everything on the local machine so the forums docker-compose
# project can reference `localhost:5000/forkana:latest`.
#
# Usage:
#   ./build-and-push-local.sh              # tags :latest and :sha-<short>
#   ./build-and-push-local.sh mytag        # additionally tags :mytag
#
# Env overrides:
#   REGISTRY       default: localhost:5000 (host:port, optional http(s)://)
#   IMAGE_NAME     default: forkana
#   BUILD_TAGS     default: "sqlite sqlite_unlock_notify"
#   REGISTRY_NAME  default: registry  (docker container name to auto-start)

set -euo pipefail

REGISTRY="${REGISTRY:-localhost:5000}"
REGISTRY="${REGISTRY#http://}"
REGISTRY="${REGISTRY#https://}"
REGISTRY="${REGISTRY%/}"
IMAGE_NAME="${IMAGE_NAME:-forkana}"
BUILD_TAGS="${BUILD_TAGS:-sqlite sqlite_unlock_notify}"
REGISTRY_NAME="${REGISTRY_NAME:-registry}"
EXTRA_TAG="${1:-}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"

if [[ "${REGISTRY}" == */* || "${REGISTRY}" != *:* ]]; then
  echo "ERROR: REGISTRY must be host:port, optionally prefixed with http:// or https://" >&2
  exit 1
fi

REGISTRY_PORT="${REGISTRY##*:}"
if [[ -z "${REGISTRY_PORT}" || ! "${REGISTRY_PORT}" =~ ^[0-9]+$ ]]; then
  echo "ERROR: REGISTRY must include a numeric port" >&2
  exit 1
fi

cd "${REPO_ROOT}"

# --- Resolve commit SHA (best-effort; falls back to "local") ------------
if COMMIT_SHA="$(git rev-parse HEAD 2>/dev/null)"; then
  SHORT_SHA="${COMMIT_SHA:0:7}"
else
  COMMIT_SHA="local"
  SHORT_SHA="local"
fi

echo "==> Forkana build"
echo "    registry:   ${REGISTRY}"
echo "    image:      ${IMAGE_NAME}"
echo "    commit:     ${COMMIT_SHA}"
echo "    build tags: ${BUILD_TAGS}"

# --- Ensure the local registry container is running ---------------------
if ! curl -fsS "http://${REGISTRY}/v2/" >/dev/null 2>&1; then
  echo "==> Registry not reachable at ${REGISTRY}; attempting to (re)start '${REGISTRY_NAME}'"
  if docker inspect "${REGISTRY_NAME}" >/dev/null 2>&1; then
    docker start "${REGISTRY_NAME}" >/dev/null
  else
    docker run -d --restart=always -p "${REGISTRY_PORT}:5000" --name "${REGISTRY_NAME}" registry:2 >/dev/null
  fi
  # Wait for it to come up
  for _ in $(seq 1 20); do
    curl -fsS "http://${REGISTRY}/v2/" >/dev/null 2>&1 && break
    sleep 1
  done
  curl -fsS "http://${REGISTRY}/v2/" >/dev/null 2>&1 \
    || { echo "ERROR: registry at ${REGISTRY} did not become ready" >&2; exit 1; }
fi

# --- Build --------------------------------------------------------------
# The GOPROXY was found to be needed on our server to successfully build
LOCAL_TAG="${IMAGE_NAME}:${COMMIT_SHA}"
echo "==> Building ${LOCAL_TAG}"
docker build \
  --file docker/forkana/Dockerfile \
  --build-arg GOPROXY="${GOPROXY:-https://proxy.golang.org,direct}" \
  --build-arg TAGS="${BUILD_TAGS}" \
  --tag "${LOCAL_TAG}" \
  --label "commit=${COMMIT_SHA}" \
  .

# --- Tag & push ---------------------------------------------------------
tags=("latest" "sha-${SHORT_SHA}")
[[ -n "${EXTRA_TAG}" ]] && tags+=("${EXTRA_TAG}")

for t in "${tags[@]}"; do
  ref="${REGISTRY}/${IMAGE_NAME}:${t}"
  echo "==> Tag + push ${ref}"
  docker tag "${LOCAL_TAG}" "${ref}"
  docker push "${ref}"
done

echo
echo "Done. Reference from forums docker-compose as:"
echo "    image: ${REGISTRY}/${IMAGE_NAME}:latest"
echo "or pin to:"
echo "    image: ${REGISTRY}/${IMAGE_NAME}:sha-${SHORT_SHA}"
