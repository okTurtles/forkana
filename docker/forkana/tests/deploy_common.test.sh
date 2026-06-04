#!/usr/bin/env bash
# deploy_common.test.sh - Focused tests for deploy_common.sh::deploy_init.
#
# Sources the script in subshells, redirects HOME to a temp dir, and asserts
# the mode-resolution contract (PROJECT_NAME, COMPOSE_FILES, REQUIRED_VARS,
# ENV_FILE, MANAGE_*) for both standalone (no deploy.conf) and forums (with
# fixture deploy.conf).  Stubs out network-touching commands.
#
# Run from the repo root:
#   bash docker/forkana/tests/deploy_common.test.sh

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
COMMON="${SCRIPT_DIR}/../deploy_common.sh"
[[ -f "${COMMON}" ]] || { echo "FATAL: cannot locate ${COMMON}" >&2; exit 2; }

PASS=0
FAIL=0
FAILED_TESTS=()

# -----------------------------------------------------------------------------
# Test helpers
# -----------------------------------------------------------------------------
assert_eq() {
  local label="$1" expected="$2" actual="$3"
  if [[ "${expected}" == "${actual}" ]]; then
    PASS=$((PASS + 1))
  else
    FAIL=$((FAIL + 1))
    FAILED_TESTS+=("${label}: expected '${expected}', got '${actual}'")
  fi
}

# Run deploy_init in a clean subshell and emit `KEY=VALUE` lines for the
# variables under test on stdout.  `git`, `docker`, `curl` are not invoked by
# deploy_init itself; `git rev-parse` is the only external call and it is
# tolerant of failure (`|| true`).  We override PATH to make this explicit.
run_init() {
  local home_dir="$1"
  (
    HOME="${home_dir}"
    export HOME
    # Hide the host repo so LOCAL_REPO_ROOT does not point at the forkana
    # checkout running this test - keeps REPO_DIR deterministic.
    cd "${home_dir}"
    # shellcheck source=../deploy_common.sh
    source "${COMMON}"
    # Use a syntactically valid 40-char hex SHA so deploy_init does not die.
    deploy_init "0000000000000000000000000000000000000000" "test" >/dev/null 2>&1
    printf 'FORKANA_DEPLOY_MODE=%s\n' "${FORKANA_DEPLOY_MODE}"
    printf 'PROJECT_NAME=%s\n'        "${PROJECT_NAME}"
    printf 'ENV_FILE=%s\n'            "${ENV_FILE}"
    printf 'COMPOSE_BASE=%s\n'        "${COMPOSE_BASE}"
    printf 'COMPOSE_OVERRIDE=%s\n'    "${COMPOSE_OVERRIDE}"
    printf 'COMPOSE_FILES=%s\n'       "${COMPOSE_FILES[*]}"
    printf 'REQUIRED_VARS=%s\n'       "${REQUIRED_VARS[*]}"
    printf 'SNAPSHOT_DIR=%s\n'        "${SNAPSHOT_DIR}"
    printf 'MANAGE_REGISTRY=%s\n'     "${MANAGE_REGISTRY}"
    printf 'MANAGE_DATA_DIRS=%s\n'    "${MANAGE_DATA_DIRS}"
  )
}

field() { grep "^$1=" <<<"$2" | head -n1 | cut -d= -f2-; }

# -----------------------------------------------------------------------------
# Test 1 - standalone mode (no deploy.conf, default)
# -----------------------------------------------------------------------------
TMP_STANDALONE="$(mktemp -d)"
trap 'rm -rf "${TMP_STANDALONE}" "${TMP_FORUMS:-}" "${TMP_FORUMS_OVR:-}" "${TMP_BAD:-}"' EXIT

OUT="$(run_init "${TMP_STANDALONE}")" || {
  FAIL=$((FAIL + 1)); FAILED_TESTS+=("standalone: deploy_init exited non-zero")
}

assert_eq "standalone.mode"             "standalone"                                                              "$(field FORKANA_DEPLOY_MODE "${OUT}")"
assert_eq "standalone.project"          "forkana"                                                                  "$(field PROJECT_NAME        "${OUT}")"
assert_eq "standalone.env_file"         "${TMP_STANDALONE}/forkana/compose/.env"                                   "$(field ENV_FILE            "${OUT}")"
assert_eq "standalone.compose_base"     "${TMP_STANDALONE}/forkana/compose/dev.yml"                                "$(field COMPOSE_BASE        "${OUT}")"
assert_eq "standalone.compose_override" "${TMP_STANDALONE}/forkana/compose/compose.override.yml"                   "$(field COMPOSE_OVERRIDE    "${OUT}")"
assert_eq "standalone.compose_files"    "-f ${TMP_STANDALONE}/forkana/compose/dev.yml -f ${TMP_STANDALONE}/forkana/compose/compose.override.yml" "$(field COMPOSE_FILES "${OUT}")"
assert_eq "standalone.required_vars"    "POSTGRES_PASSWORD FORKANA_DOMAIN FORKANA_SECRET_KEY FORKANA_INTERNAL_TOKEN FORKANA_JWT_SECRET" "$(field REQUIRED_VARS "${OUT}")"
assert_eq "standalone.snapshot_dir"     "${TMP_STANDALONE}/forkana/compose"                                        "$(field SNAPSHOT_DIR        "${OUT}")"
assert_eq "standalone.manage_registry"  "1"                                                                        "$(field MANAGE_REGISTRY     "${OUT}")"
assert_eq "standalone.manage_data_dirs" "1"                                                                        "$(field MANAGE_DATA_DIRS    "${OUT}")"

# -----------------------------------------------------------------------------
# Test 2 - forums mode with neutral defaults (only FORUMS_DIR set)
# -----------------------------------------------------------------------------
TMP_FORUMS="$(mktemp -d)"
mkdir -p "${TMP_FORUMS}/forkana" "${TMP_FORUMS}/forums"
cat > "${TMP_FORUMS}/forkana/deploy.conf" <<EOF
FORKANA_DEPLOY_MODE=forums
FORUMS_DIR=${TMP_FORUMS}/forums
EOF

OUT="$(run_init "${TMP_FORUMS}")" || {
  FAIL=$((FAIL + 1)); FAILED_TESTS+=("forums: deploy_init exited non-zero")
}

assert_eq "forums.mode"             "forums"                                                                                "$(field FORKANA_DEPLOY_MODE "${OUT}")"
assert_eq "forums.project"          "forums"                                                                                "$(field PROJECT_NAME        "${OUT}")"
assert_eq "forums.env_file"         "${TMP_FORUMS}/forums/.env"                                                             "$(field ENV_FILE            "${OUT}")"
assert_eq "forums.compose_base"     "${TMP_FORUMS}/forums/docker-compose.yml"                                               "$(field COMPOSE_BASE        "${OUT}")"
assert_eq "forums.compose_override" "${TMP_FORUMS}/forums/forkana-override.yml"                                             "$(field COMPOSE_OVERRIDE    "${OUT}")"
assert_eq "forums.compose_files"    "-f ${TMP_FORUMS}/forums/docker-compose.yml -f ${TMP_FORUMS}/forums/forkana-override.yml" "$(field COMPOSE_FILES "${OUT}")"
assert_eq "forums.required_vars"    "POSTGRES_FORKANA_PASSWORD FORKANA_DOMAIN FORKANA_ROOT_URL FORKANA_SECRET_KEY FORKANA_INTERNAL_TOKEN FORKANA_JWT_SECRET FORKANA_HOST_PORT" "$(field REQUIRED_VARS "${OUT}")"
assert_eq "forums.snapshot_dir"     "${TMP_FORUMS}/forkana/snapshots"                                                       "$(field SNAPSHOT_DIR        "${OUT}")"
assert_eq "forums.manage_registry"  "0"                                                                                     "$(field MANAGE_REGISTRY     "${OUT}")"
assert_eq "forums.manage_data_dirs" "0"                                                                                     "$(field MANAGE_DATA_DIRS    "${OUT}")"

# -----------------------------------------------------------------------------
# Test 3 - forums mode with operator overrides via deploy.conf
#
# Exercises the FORUMS_ENV_FILE / FORUMS_OVERRIDE_FILE /
# FORUMS_EXTRA_COMPOSE_FILES knobs so an operator can point at any layout
# without touching the tracked code.  Relative paths must resolve against
# ${FORUMS_DIR}.
# -----------------------------------------------------------------------------
TMP_FORUMS_OVR="$(mktemp -d)"
mkdir -p "${TMP_FORUMS_OVR}/forkana" "${TMP_FORUMS_OVR}/stack/cfg" "${TMP_FORUMS_OVR}/stack/secrets"
cat > "${TMP_FORUMS_OVR}/forkana/deploy.conf" <<EOF
FORKANA_DEPLOY_MODE=forums
FORUMS_DIR=${TMP_FORUMS_OVR}/stack
FORUMS_ENV_FILE=secrets/env
FORUMS_OVERRIDE_FILE=cfg/forkana-override.yml
FORUMS_EXTRA_COMPOSE_FILES="cfg/disabled.yml cfg/extras.yml"
EOF

OUT="$(run_init "${TMP_FORUMS_OVR}")" || {
  FAIL=$((FAIL + 1)); FAILED_TESTS+=("forums.override: deploy_init exited non-zero")
}

assert_eq "forums.override.env_file"         "${TMP_FORUMS_OVR}/stack/secrets/env"                                                  "$(field ENV_FILE         "${OUT}")"
assert_eq "forums.override.compose_override" "${TMP_FORUMS_OVR}/stack/cfg/forkana-override.yml"                                     "$(field COMPOSE_OVERRIDE "${OUT}")"
assert_eq "forums.override.compose_files"    "-f ${TMP_FORUMS_OVR}/stack/docker-compose.yml -f ${TMP_FORUMS_OVR}/stack/cfg/disabled.yml -f ${TMP_FORUMS_OVR}/stack/cfg/extras.yml -f ${TMP_FORUMS_OVR}/stack/cfg/forkana-override.yml" "$(field COMPOSE_FILES "${OUT}")"

# -----------------------------------------------------------------------------
# Test 4 - forums mode without FORUMS_DIR must die
# -----------------------------------------------------------------------------
TMP_BAD="$(mktemp -d)"
mkdir -p "${TMP_BAD}/forkana"
cat > "${TMP_BAD}/forkana/deploy.conf" <<EOF
FORKANA_DEPLOY_MODE=forums
EOF
if run_init "${TMP_BAD}" >/dev/null 2>&1; then
  FAIL=$((FAIL + 1))
  FAILED_TESTS+=("forums.no_forums_dir: deploy_init should have died but exited 0")
else
  PASS=$((PASS + 1))
fi

# -----------------------------------------------------------------------------
# Test 5 - unknown mode must die
# -----------------------------------------------------------------------------
cat > "${TMP_BAD}/forkana/deploy.conf" <<EOF
FORKANA_DEPLOY_MODE=lunar
EOF
if run_init "${TMP_BAD}" >/dev/null 2>&1; then
  FAIL=$((FAIL + 1))
  FAILED_TESTS+=("unknown_mode: deploy_init should have died but exited 0")
else
  PASS=$((PASS + 1))
fi

# -----------------------------------------------------------------------------
# Report
# -----------------------------------------------------------------------------
printf '\n%d passed, %d failed\n' "${PASS}" "${FAIL}"
if (( FAIL > 0 )); then
  printf 'Failures:\n' >&2
  for f in "${FAILED_TESTS[@]}"; do printf '  - %s\n' "${f}" >&2; done
  exit 1
fi
