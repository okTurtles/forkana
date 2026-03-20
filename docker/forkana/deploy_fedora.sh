#!/usr/bin/env bash
# deploy_fedora.sh — Build-on-server deploy script for Forkana (Fedora).
#
# Invoked via SSH forced-command from GitHub Actions.  The commit SHA is
# passed either as $1 (direct invocation) or via SSH_ORIGINAL_COMMAND
# (forced-command mode).
#
# Usage:  deploy_fedora.sh <40-char-hex-commit-sha>
#
# Local testing:
#   Run from anywhere inside the git checkout.  The script detects the
#   repo root via `git rev-parse` and skips fetch/checkout (step 3).
#     ./docker/forkana/deploy_fedora.sh "$(git rev-parse HEAD)"

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=deploy_common.sh
source "${SCRIPT_DIR}/deploy_common.sh"

# ---------------------------------------------------------------------------
# 0. Ensure required packages are installed (Fedora — dnf)
# ---------------------------------------------------------------------------
install_os_packages() {
  log "Ensuring required packages are installed..."
  for cmd in git docker jq curl; do
    if ! command -v "${cmd}" &>/dev/null; then
      log "Installing missing command: ${cmd}"
      case "${cmd}" in
        docker)
          sudo dnf install -y docker-ce docker-ce-cli containerd.io docker-compose-plugin 2>/dev/null \
            || sudo dnf install -y docker docker-compose 2>/dev/null \
            || die "Failed to install Docker. Please install Docker manually."
          sudo systemctl start docker
          sudo systemctl enable docker
          sudo usermod -aG docker "$(whoami)" || true
          ;;
        *)
          sudo dnf install -y "${cmd}" || die "Failed to install ${cmd}."
          ;;
      esac
    fi
  done

  # Ensure docker compose (v2 plugin) is available.
  if ! docker compose version &>/dev/null; then
    log "Installing docker-compose-plugin..."
    sudo dnf install -y docker-compose-plugin 2>/dev/null \
      || die "Failed to install docker-compose-plugin. Please install Docker Compose v2 manually."
  fi
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
deploy_init "${1:-}" "Fedora"
deploy_run
