#!/bin/bash
set -euo pipefail

make --no-print-directory watch-frontend &
make --no-print-directory watch-backend-debug &

trap 'kill $(jobs -p) 2>/dev/null || true' EXIT INT TERM
wait
