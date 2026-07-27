#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
TEST_DIR="$(mktemp -d)"
trap 'rm -rf "${TEST_DIR}"' EXIT

mkdir -p "${TEST_DIR}/bin"

cat >"${TEST_DIR}/bin/gh" <<'SH'
#!/usr/bin/env bash
set -Eeuo pipefail

scenario="${FAKE_GH_SCENARIO:-empty}"

if [[ "${1:-}" == "label" && "${2:-}" == "create" ]]; then
  printf '%s\n' "${3:?}" >>"${FAKE_LABEL_CALLS_FILE:?}"
  exit 0
fi

if [[ "${1:-}" == "issue" && "${2:-}" == "list" ]]; then
  label=""
  previous=""
  for argument in "$@"; do
    if [[ "${previous}" == "--label" ]]; then
      label="${argument}"
      break
    fi
    previous="${argument}"
  done

  case "${scenario}" in
    inbox)
      if [[ "${label}" == "inbox" ]]; then
        printf '%s\n' '[{"number":1,"title":"Inbox","body":"Untrusted intake","author":{"login":"reporter"},"labels":[{"name":"inbox"}],"url":"https://example.test/1","updatedAt":"2026-07-27T00:00:00Z","createdAt":"2026-07-27T00:00:00Z"}]'
      else
        printf '[]\n'
      fi
      ;;
    open_only)
      if [[ "${label}" == "open" ]]; then
        printf '%s\n' '[{"number":2,"title":"Open","body":"Ready but not approved","author":{"login":"reporter"},"labels":[{"name":"open"}],"url":"https://example.test/2","updatedAt":"2026-07-27T00:00:00Z","createdAt":"2026-07-27T00:00:00Z"}]'
      else
        printf '[]\n'
      fi
      ;;
    trusted_implement|untrusted_implement|event_failure)
      if [[ "${label}" == "open" || "${label}" == "implement" ]]; then
        printf '%s\n' '[{"number":3,"title":"Approved work","body":"Implement the bounded slice","author":{"login":"reporter"},"labels":[{"name":"open"},{"name":"implement"}],"url":"https://example.test/3","updatedAt":"2026-07-27T00:00:00Z","createdAt":"2026-07-27T00:00:00Z"}]'
      else
        printf '[]\n'
      fi
      ;;
    pending_implement)
      if [[ "${label}" == "open" || "${label}" == "pending" || "${label}" == "implement" ]]; then
        printf '%s\n' '[{"number":4,"title":"Blocked work","body":"Do not implement yet","author":{"login":"reporter"},"labels":[{"name":"open"},{"name":"pending"},{"name":"implement"}],"url":"https://example.test/4","updatedAt":"2026-07-27T00:00:00Z","createdAt":"2026-07-27T00:00:00Z"}]'
      else
        printf '[]\n'
      fi
      ;;
    empty)
      printf '[]\n'
      ;;
    *)
      echo "Unknown fake scenario: ${scenario}" >&2
      exit 2
      ;;
  esac
  exit 0
fi

if [[ "${1:-}" == "api" ]]; then
  case "${scenario}" in
    trusted_implement|pending_implement)
      printf '%s\n' '[[{"event":"labeled","label":{"name":"implement"},"actor":{"login":"TrustedMaintainer"}}]]'
      ;;
    untrusted_implement)
      printf '%s\n' '[[{"event":"labeled","label":{"name":"implement"},"actor":{"login":"OtherUser"}}]]'
      ;;
    event_failure)
      exit 1
      ;;
    *)
      printf '[]\n'
      ;;
  esac
  exit 0
fi

echo "Unsupported fake gh command: $*" >&2
exit 2
SH

cat >"${TEST_DIR}/bin/flock" <<'SH'
#!/usr/bin/env bash
exit 0
SH

cat >"${TEST_DIR}/bin/hermes" <<'SH'
#!/usr/bin/env bash
set -Eeuo pipefail
printf 'called\n' >>"${HERMES_CALLS_FILE:?}"
SH

chmod +x "${TEST_DIR}/bin/gh" "${TEST_DIR}/bin/flock" "${TEST_DIR}/bin/hermes"

for script in \
  "${SCRIPT_DIR}/fetch-issues.sh" \
  "${SCRIPT_DIR}/run-hermes-task-loop.sh" \
  "${SCRIPT_DIR}/setup-github-labels.sh" \
  "${SCRIPT_DIR}/test.sh"; do
  bash -n "${script}"
done

run_fetch_case() {
  local scenario="$1"
  local expected_actionable="$2"
  local expected_approved="$3"
  local state_dir="${TEST_DIR}/fetch-${scenario}"
  local out_file="${state_dir}/issues.json"

  mkdir -p "${state_dir}"
  PATH="${TEST_DIR}/bin:${PATH}" \
    GH_BIN=gh \
    GH_TOKEN=test-token \
    REPO_SLUG=example/tiny \
    APPROVER_LOGINS=TrustedMaintainer \
    FAKE_GH_SCENARIO="${scenario}" \
    STATE_DIR="${state_dir}" \
    bash "${SCRIPT_DIR}/fetch-issues.sh" "${out_file}" >/dev/null

  python3 - "${out_file}" "${expected_actionable}" "${expected_approved}" <<'PY'
import json
import sys
from pathlib import Path

payload = json.loads(Path(sys.argv[1]).read_text())
expected_actionable = int(sys.argv[2])
expected_approved = int(sys.argv[3])
assert payload["counts"]["actionable"] == expected_actionable, payload
assert payload["counts"]["approvedImplement"] == expected_approved, payload
PY
}

run_fetch_case empty 0 0
run_fetch_case inbox 1 0
run_fetch_case open_only 0 0
run_fetch_case trusted_implement 1 1
run_fetch_case untrusted_implement 0 0
run_fetch_case pending_implement 0 1
run_fetch_case event_failure 0 0

missing_token_state="${TEST_DIR}/missing-token"
mkdir -p "${missing_token_state}"
if env \
  -u GH_TOKEN \
  -u GITHUB_TOKEN \
  PATH="${TEST_DIR}/bin:${PATH}" \
  GH_BIN=gh \
  TOKEN_FILE="${TEST_DIR}/missing-token-file" \
  HERMES_ENV_FILE="${TEST_DIR}/missing-env-file" \
  STATE_DIR="${missing_token_state}" \
  bash "${SCRIPT_DIR}/fetch-issues.sh" "${missing_token_state}/issues.json" \
  >/dev/null 2>&1; then
  echo "fetch-issues.sh unexpectedly accepted missing authentication" >&2
  exit 1
fi

run_loop_case() {
  local scenario="$1"
  local expected_calls="$2"
  local state_dir="${TEST_DIR}/runner-${scenario}"
  local calls_file="${state_dir}/hermes-calls"

  mkdir -p "${state_dir}"
  : >"${calls_file}"

  PATH="${TEST_DIR}/bin:${PATH}" \
    GH_BIN=gh \
    GH_TOKEN=test-token \
    REPO_SLUG=example/tiny \
    APPROVER_LOGINS=TrustedMaintainer \
    FAKE_GH_SCENARIO="${scenario}" \
    HERMES_BIN=hermes \
    HERMES_CALLS_FILE="${calls_file}" \
    LOCK_FILE="${state_dir}/loop.lock" \
    STATE_DIR="${state_dir}" \
    bash "${SCRIPT_DIR}/run-hermes-task-loop.sh" >/dev/null

  actual_calls="$(wc -l <"${calls_file}" | tr -d ' ')"
  if [[ "${actual_calls}" != "${expected_calls}" ]]; then
    echo "${scenario}: expected ${expected_calls} Hermes calls, got ${actual_calls}" >&2
    exit 1
  fi
}

run_loop_case empty 0
run_loop_case open_only 0
run_loop_case untrusted_implement 0
run_loop_case pending_implement 0
run_loop_case trusted_implement 1
run_loop_case inbox 1

label_calls="${TEST_DIR}/label-calls"
: >"${label_calls}"
PATH="${TEST_DIR}/bin:${PATH}" \
  GH_BIN=gh \
  GH_TOKEN=test-token \
  REPO_SLUG=example/tiny \
  FAKE_LABEL_CALLS_FILE="${label_calls}" \
  bash "${SCRIPT_DIR}/setup-github-labels.sh"

if [[ "$(wc -l <"${label_calls}" | tr -d ' ')" != "5" ]]; then
  echo "setup-github-labels.sh did not configure all five workflow labels" >&2
  exit 1
fi

echo "GitHub issue loop tests passed."
