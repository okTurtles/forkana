#!/usr/bin/env bash
# cleanup-images.sh - Prune old Forkana image tarballs from ~/forkana/images.
#
# Keeps the most recent N image tarballs (default 3) and removes older ones
# to prevent disk bloat on the deploy user's home directory.  Each tarball
# produced by GitHub Actions is ~500MB, so unattended hosts fill quickly
# without periodic cleanup.
#
# Usage:  cleanup-images.sh [keep_count]
#   keep_count - number of most recent tarballs to keep (default: 3)
#
# Typical installation (run as the deploy user):
#   crontab -e
#   0 2 * * * $HOME/forkana/cleanup-images.sh

set -euo pipefail

IMAGES_DIR="${HOME}/forkana/images"
KEEP_COUNT="${1:-3}"

if [[ ! "${KEEP_COUNT}" =~ ^[0-9]+$ ]]; then
  echo "ERROR: keep_count must be a non-negative integer (got '${KEEP_COUNT}')." >&2
  exit 1
fi

if [[ ! -d "${IMAGES_DIR}" ]]; then
  echo "ERROR: ${IMAGES_DIR} does not exist." >&2
  exit 1
fi

cd "${IMAGES_DIR}"

# ls -1t lists tarballs newest-first; tail -n +N+1 skips the newest N entries,
# leaving only the older ones that should be pruned.
removed=0
while IFS= read -r file; do
  [[ -z "${file}" ]] && continue
  echo "Removing old image: ${file}"
  rm -f "${file}"
  removed=$((removed + 1))
done < <(ls -1t forkana-*.tar.gz 2>/dev/null | tail -n +$((KEEP_COUNT + 1)) || true)

echo "Cleanup complete. Removed ${removed} tarball(s); kept the last ${KEEP_COUNT}."
