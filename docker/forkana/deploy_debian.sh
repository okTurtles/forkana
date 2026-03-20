#!/usr/bin/env bash
# deploy_debian.sh — Build-on-server deploy script for Forkana (Debian/Ubuntu).
#
# Invoked via SSH forced-command from GitHub Actions.  The commit SHA is
# passed either as $1 (direct invocation) or via SSH_ORIGINAL_COMMAND
# (forced-command mode).
#
# Usage:  deploy_debian.sh <40-char-hex-commit-sha>
#
# Local testing:
#   Run from anywhere inside the git checkout.  The script detects the
#   repo root via `git rev-parse` and skips fetch/checkout (step 3).
#     ./docker/forkana/deploy_debian.sh "$(git rev-parse HEAD)"

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=deploy_common.sh
source "${SCRIPT_DIR}/deploy_common.sh"

# No OS-specific package installation for Debian — prerequisites are assumed
# to be pre-installed (git, docker, jq, curl).

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------
deploy_init "${1:-}"
deploy_run
