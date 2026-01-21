#!/bin/bash
set -uo pipefail

cd "$(dirname -- "${BASH_SOURCE[0]}")" && cd ..

IGNORE_PATTERNS=(
  "is deprecated" # TODO: fix these
)

# Install gopls if not already installed, then use the installed binary for
# faster execution. Using 'go run' each time adds overhead.
"$GO" install "$GOPLS_PACKAGE"
GOPLS_BIN=$("$GO" env GOPATH)/bin/gopls

# Verify gopls was installed successfully
if [[ ! -x "$GOPLS_BIN" ]]; then
  echo "Error: Failed to install gopls" >&2
  exit 1
fi

# Detect number of CPUs in a cross-platform way (Linux and macOS)
if command -v nproc &> /dev/null; then
  NPROC=$(nproc)
elif command -v sysctl &> /dev/null; then
  NPROC=$(sysctl -n hw.ncpu)
else
  NPROC=1
fi

# lint all go files with 'gopls check' and look for lines starting with the
# current absolute path, indicating a error was found. This is necessary
# because the tool does not set non-zero exit code when errors are found.
# ref: https://github.com/golang/go/issues/67078
# Use xargs with parallel execution (-P) to speed up checking many files.
# Each batch of 100 files is processed in parallel across available cores.
# Use null-delimited format (-0) to handle filenames with spaces correctly.
ERROR_LINES=$(printf '%s\0' "$@" | xargs -0 -P "$NPROC" -n 100 "$GOPLS_BIN" check -severity=warning 2>/dev/null | grep -E "^$PWD" | grep -vFf <(printf '%s\n' "${IGNORE_PATTERNS[@]}"));
NUM_ERRORS=$(echo -n "$ERROR_LINES" | wc -l)

if [ "$NUM_ERRORS" -eq "0" ]; then
  exit 0;
else
  echo "$ERROR_LINES"
  echo "Found $NUM_ERRORS 'gopls check' errors"
  exit 1;
fi
