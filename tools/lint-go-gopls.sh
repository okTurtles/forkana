#!/bin/bash
set -uo pipefail

cd "$(dirname -- "${BASH_SOURCE[0]}")" && cd ..

IGNORE_PATTERNS=(
  "is deprecated" # TODO: fix these
)

# Install gopls if not already installed, then use the installed binary for
# faster execution. Using 'go run' each time adds overhead.
"$GO" install "$GOPLS_PACKAGE"

# Determine gopls binary path, accounting for GOBIN if set
GOBIN_DIR=$("$GO" env GOBIN)
if [[ -z "$GOBIN_DIR" ]]; then
  GOBIN_DIR=$("$GO" env GOPATH)/bin
fi
GOPLS_BIN="$GOBIN_DIR/gopls"

# Verify gopls was installed successfully
if [[ ! -x "$GOPLS_BIN" ]]; then
  echo "Error: Failed to install gopls" >&2
  exit 1
fi

# Parallelism is configurable via GOPLS_PARALLEL env var.
# Default to 1 (sequential) to avoid memory pressure on local machines.
# CI can set a higher value if desired (e.g., GOPLS_PARALLEL=4).
# In Devin's environment, default to (N-1) cores for better performance.
if [[ -z "${GOPLS_PARALLEL:-}" ]]; then
  if [[ -d "/opt/.devin" ]]; then
    NPROC=$(nproc)
    PARALLEL=$((NPROC > 1 ? NPROC - 1 : 1))
  else
    PARALLEL=1
  fi
else
  PARALLEL=$GOPLS_PARALLEL
fi

# lint all go files with 'gopls check' and look for lines starting with the
# current absolute path, indicating a error was found. This is necessary
# because the tool does not set non-zero exit code when errors are found.
# ref: https://github.com/golang/go/issues/67078
# Use xargs with configurable parallelism (-P) to process files.
# Use null-delimited format (-0) to handle filenames with spaces correctly.
ERROR_LINES=$(printf '%s\0' "$@" | xargs -0 -P "$PARALLEL" -n 100 "$GOPLS_BIN" check -severity=warning 2>/dev/null | grep -E "^$PWD" | grep -vFf <(printf '%s\n' "${IGNORE_PATTERNS[@]}"));
NUM_ERRORS=$(echo -n "$ERROR_LINES" | wc -l)

if [ "$NUM_ERRORS" -eq "0" ]; then
  exit 0;
else
  echo "$ERROR_LINES"
  echo "Found $NUM_ERRORS 'gopls check' errors"
  exit 1;
fi
