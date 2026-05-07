#!/usr/bin/env bash
# deploy.sh — OS-detecting wrapper for Forkana deploy scripts.
#
# Invoked via SSH forced-command from GitHub Actions.  The commit SHA is
# passed either as $1 (direct invocation) or via SSH_ORIGINAL_COMMAND
# (forced-command mode).
#
# This wrapper detects the host OS via /etc/os-release and delegates to
# the appropriate OS-specific deploy script (deploy_debian.sh or
# deploy_fedora.sh).
#
# Usage:  deploy.sh <40-char-hex-commit-sha>

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# ---------------------------------------------------------------------------
# Detect OS family from /etc/os-release
# (Pattern follows contrib/upgrade.sh)
# ---------------------------------------------------------------------------
if [[ ! -f /etc/os-release ]]; then
  echo "FATAL: /etc/os-release not found — cannot detect OS." >&2
  exit 1
fi

os_release="$(cat /etc/os-release)"

if [[ "${os_release}" =~ ID=debian || "${os_release}" =~ ID=ubuntu || "${os_release}" =~ ID_LIKE=.*debian ]]; then
  OS_SCRIPT="${SCRIPT_DIR}/deploy_debian.sh"
elif [[ "${os_release}" =~ ID=fedora || "${os_release}" =~ ID_LIKE=.*fedora ]]; then
  OS_SCRIPT="${SCRIPT_DIR}/deploy_fedora.sh"
else
  echo "FATAL: Unsupported OS. Only Debian/Ubuntu and Fedora are supported." >&2
  echo "Contents of /etc/os-release:" >&2
  cat /etc/os-release >&2
  exit 1
fi

if [[ ! -x "${OS_SCRIPT}" ]]; then
  echo "FATAL: OS-specific deploy script not found or not executable: ${OS_SCRIPT}" >&2
  exit 1
fi

# Delegate to the OS-specific script, forwarding all arguments.
exec "${OS_SCRIPT}" "$@"

