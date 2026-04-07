#!/usr/bin/env bash
# deploy_fedora.sh - Deploy script for Forkana (Fedora).
#
# Loads a pre-built Docker image tarball and deploys it via the local
# registry.  Invoked via SSH forced-command from GitHub Actions.  The
# commit SHA is passed either as $1 (direct invocation) or via
# SSH_ORIGINAL_COMMAND (forced-command mode).
#
# Usage:  deploy_fedora.sh <40-char-hex-commit-sha>
#
# Local testing:
#   Build and save the image tarball first, then run from inside the
#   git checkout.  See DEPLOYMENT_GUIDE.md § Maintenance for details.
#     ./docker/forkana/deploy_fedora.sh "$(git rev-parse HEAD)"

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=deploy_common.sh
source "${SCRIPT_DIR}/deploy_common.sh"

# ---------------------------------------------------------------------------
# 0. Verify required packages are installed (Fedora)
#
# The deploy user typically has no sudo access, so this function only
# validates that prerequisites exist - it does not attempt to install them.
# Install missing packages as root before the first deploy (see
# DEPLOYMENT_GUIDE.md § Server Preparation).
# ---------------------------------------------------------------------------
install_os_packages() {
  log "Verifying required packages are installed..."
  local missing=()
  for cmd in docker jq curl; do
    if ! command -v "${cmd}" &>/dev/null; then
      missing+=("${cmd}")
    fi
  done

  # Ensure docker compose (v2 plugin) is available.
  if command -v docker &>/dev/null && ! docker compose version &>/dev/null; then
    missing+=("docker-compose-plugin")
  fi

  if [[ ${#missing[@]} -gt 0 ]]; then
    die "Missing required commands: ${missing[*]}. Install them as root before deploying (see DEPLOYMENT_GUIDE.md)."
  fi
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
deploy_init "${1:-}" "Fedora"
deploy_run
