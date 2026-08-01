#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
TEST_DIR="$(mktemp -d)"
trap 'rm -rf "${TEST_DIR}"' EXIT

mkdir -p "${TEST_DIR}/bin"

cat >"${TEST_DIR}/bin/gh" <<'SH'
#!/usr/bin/env bash
set -Eeuo pipefail

scenario="${FAKE_GH_SCENARIO:-empty}"
updated_at="${FAKE_UPDATED_AT:-2026-08-01T00:00:00Z}"

if [[ "${1:-}" == "label" && "${2:-}" == "create" ]]; then
  printf '%s\n' "${3:?}" >>"${FAKE_LABEL_CALLS_FILE:?}"
  exit 0
fi

if [[ "${1:-}" == "issue" && "${2:-}" == "list" ]]; then
  case "${scenario}" in
    triage_new)
      printf '[{"number":1,"title":"New bug","body":"Synthetic report","author":{"login":"reporter"},"labels":[{"name":"type/bug"},{"name":"status/needs-triage"}],"url":"https://example.test/1","updatedAt":"%s","createdAt":"2026-08-01T00:00:00Z"}]\n' "${updated_at}"
      ;;
    triage_unlabelled)
      printf '[{"number":2,"title":"Unlabelled","body":"Synthetic report","author":{"login":"reporter"},"labels":[],"url":"https://example.test/2","updatedAt":"%s","createdAt":"2026-08-01T00:00:00Z"}]\n' "${updated_at}"
      ;;
    triage_accepted|accepted_only)
      printf '[{"number":3,"title":"Accepted","body":"Refined work","author":{"login":"reporter"},"labels":[{"name":"type/maintenance"},{"name":"status/accepted"}],"url":"https://example.test/3","updatedAt":"%s","createdAt":"2026-08-01T00:00:00Z"}]\n' "${updated_at}"
      ;;
    trusted_plan|untrusted_plan)
      printf '[{"number":4,"title":"Plan work","body":"Plan the bounded slice","author":{"login":"reporter"},"labels":[{"name":"status/accepted"},{"name":"agent/plan"}],"url":"https://example.test/4","updatedAt":"%s","createdAt":"2026-08-01T00:00:00Z"}]\n' "${updated_at}"
      ;;
    trusted_implement|untrusted_implement|event_failure)
      printf '[{"number":5,"title":"Implement work","body":"Implement the bounded slice","author":{"login":"reporter"},"labels":[{"name":"status/accepted"},{"name":"agent/implement"}],"url":"https://example.test/5","updatedAt":"%s","createdAt":"2026-08-01T00:00:00Z"}]\n' "${updated_at}"
      ;;
    blocked_implement)
      printf '[{"number":6,"title":"Blocked work","body":"Do not implement","author":{"login":"reporter"},"labels":[{"name":"status/blocked"},{"name":"agent/implement"}],"url":"https://example.test/6","updatedAt":"%s","createdAt":"2026-08-01T00:00:00Z"}]\n' "${updated_at}"
      ;;
    ambiguous_commands)
      printf '[{"number":7,"title":"Ambiguous work","body":"Do not choose a command","author":{"login":"reporter"},"labels":[{"name":"status/accepted"},{"name":"agent/plan"},{"name":"agent/implement"}],"url":"https://example.test/7","updatedAt":"%s","createdAt":"2026-08-01T00:00:00Z"}]\n' "${updated_at}"
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
    trusted_plan)
      printf '%s\n' '[[{"event":"labeled","label":{"name":"agent/plan"},"actor":{"login":"TrustedMaintainer"}}]]'
      ;;
    untrusted_plan)
      printf '%s\n' '[[{"event":"labeled","label":{"name":"agent/plan"},"actor":{"login":"OtherUser"}}]]'
      ;;
    trusted_implement|blocked_implement)
      printf '%s\n' '[[{"event":"labeled","label":{"name":"agent/implement"},"actor":{"login":"TrustedMaintainer"}}]]'
      ;;
    untrusted_implement)
      printf '%s\n' '[[{"event":"labeled","label":{"name":"agent/implement"},"actor":{"login":"OtherUser"}}]]'
      ;;
    ambiguous_commands)
      printf '%s\n' '[[{"event":"labeled","label":{"name":"agent/plan"},"actor":{"login":"TrustedMaintainer"}},{"event":"labeled","label":{"name":"agent/implement"},"actor":{"login":"TrustedMaintainer"}}]]'
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
  "${SCRIPT_DIR}/fetch-triage-issues.sh" \
  "${SCRIPT_DIR}/run-hermes-task-loop.sh" \
  "${SCRIPT_DIR}/run-hermes-triage-loop.sh" \
  "${SCRIPT_DIR}/setup-github-labels.sh" \
  "${SCRIPT_DIR}/test.sh"; do
  bash -n "${script}"
done
python3 -m py_compile "${SCRIPT_DIR}/mark-triage-seen.py"

run_task_fetch_case() {
  local scenario="$1"
  local expected_actionable="$2"
  local expected_plan="$3"
  local expected_implement="$4"
  local state_dir="${TEST_DIR}/task-fetch-${scenario}"
  local out_file="${state_dir}/issues.json"

  mkdir -p "${state_dir}"
  PATH="${TEST_DIR}/bin:${PATH}" \
    GH_BIN=gh \
    GH_TOKEN=test-token \
    REPO_SLUG=example/tinker \
    APPROVER_LOGINS=TrustedMaintainer \
    FAKE_GH_SCENARIO="${scenario}" \
    STATE_DIR="${state_dir}" \
    bash "${SCRIPT_DIR}/fetch-issues.sh" "${out_file}" >/dev/null

  python3 - "${out_file}" "${expected_actionable}" "${expected_plan}" "${expected_implement}" <<'PY'
import json
import sys
from pathlib import Path

payload = json.loads(Path(sys.argv[1]).read_text())
assert payload["counts"]["actionable"] == int(sys.argv[2]), payload
assert payload["counts"]["approvedPlan"] == int(sys.argv[3]), payload
assert payload["counts"]["approvedImplement"] == int(sys.argv[4]), payload
PY
}

run_task_fetch_case empty 0 0 0
run_task_fetch_case accepted_only 0 0 0
run_task_fetch_case trusted_plan 1 1 0
run_task_fetch_case untrusted_plan 0 0 0
run_task_fetch_case trusted_implement 1 0 1
run_task_fetch_case untrusted_implement 0 0 0
run_task_fetch_case blocked_implement 0 0 1
run_task_fetch_case ambiguous_commands 0 1 1
run_task_fetch_case event_failure 0 0 0

run_triage_fetch() {
  local scenario="$1"
  local updated_at="$2"
  local state_dir="$3"
  local out_file="${state_dir}/issues.json"

  mkdir -p "${state_dir}"
  PATH="${TEST_DIR}/bin:${PATH}" \
    GH_BIN=gh \
    TRIAGE_GH_TOKEN=triage-token \
    REPO_SLUG=example/tinker \
    FAKE_GH_SCENARIO="${scenario}" \
    FAKE_UPDATED_AT="${updated_at}" \
    STATE_DIR="${state_dir}" \
    SEEN_FILE="${state_dir}/seen.json" \
    bash "${SCRIPT_DIR}/fetch-triage-issues.sh" "${out_file}" >/dev/null
}

triage_state="${TEST_DIR}/triage-fetch"
mkdir -p "${triage_state}"
printf '%s\n' '{"999":"2025-01-01T00:00:00Z"}' >"${triage_state}/seen.json"
run_triage_fetch triage_accepted 2026-08-01T00:00:00Z "${triage_state}"
python3 - "${triage_state}/issues.json" <<'PY'
import json
import sys
from pathlib import Path
assert json.loads(Path(sys.argv[1]).read_text())["counts"]["actionable"] == 1
PY
python3 "${SCRIPT_DIR}/mark-triage-seen.py" "${triage_state}/issues.json" "${triage_state}/seen.json"
python3 - "${triage_state}/seen.json" <<'PY'
import json
import sys
from pathlib import Path
seen = json.loads(Path(sys.argv[1]).read_text())
assert set(seen) == {"3"}, seen
PY
run_triage_fetch triage_accepted 2026-08-01T00:00:00Z "${triage_state}"
python3 - "${triage_state}/issues.json" <<'PY'
import json
import sys
from pathlib import Path
assert json.loads(Path(sys.argv[1]).read_text())["counts"]["actionable"] == 0
PY
run_triage_fetch triage_accepted 2026-08-02T00:00:00Z "${triage_state}"
python3 - "${triage_state}/issues.json" <<'PY'
import json
import sys
from pathlib import Path
assert json.loads(Path(sys.argv[1]).read_text())["counts"]["actionable"] == 1
PY

for scenario in triage_new triage_unlabelled; do
  state_dir="${TEST_DIR}/triage-${scenario}"
  run_triage_fetch "${scenario}" 2026-08-01T00:00:00Z "${state_dir}"
  python3 - "${state_dir}/issues.json" <<'PY'
import json
import sys
from pathlib import Path
assert json.loads(Path(sys.argv[1]).read_text())["counts"]["actionable"] == 1
PY
done

missing_task_state="${TEST_DIR}/missing-task-token"
mkdir -p "${missing_task_state}"
if env -u GH_TOKEN -u GITHUB_TOKEN \
  PATH="${TEST_DIR}/bin:${PATH}" \
  GH_BIN=gh \
  TOKEN_FILE="${TEST_DIR}/missing-task-token-file" \
  HERMES_ENV_FILE="${TEST_DIR}/missing-env-file" \
  STATE_DIR="${missing_task_state}" \
  bash "${SCRIPT_DIR}/fetch-issues.sh" "${missing_task_state}/issues.json" \
  >/dev/null 2>&1; then
  echo "fetch-issues.sh unexpectedly accepted missing authentication" >&2
  exit 1
fi

missing_triage_state="${TEST_DIR}/missing-triage-token"
mkdir -p "${missing_triage_state}"
if env -u TRIAGE_GH_TOKEN -u TRIAGE_GITHUB_TOKEN \
  GH_TOKEN=task-token \
  PATH="${TEST_DIR}/bin:${PATH}" \
  GH_BIN=gh \
  TRIAGE_TOKEN_FILE="${TEST_DIR}/missing-triage-token-file" \
  HERMES_ENV_FILE="${TEST_DIR}/missing-env-file" \
  STATE_DIR="${missing_triage_state}" \
  bash "${SCRIPT_DIR}/fetch-triage-issues.sh" "${missing_triage_state}/issues.json" \
  >/dev/null 2>&1; then
  echo "fetch-triage-issues.sh unexpectedly reused the task token" >&2
  exit 1
fi

if env -u TRIAGE_GH_TOKEN -u TRIAGE_GITHUB_TOKEN \
  GH_TOKEN=task-token \
  PATH="${TEST_DIR}/bin:${PATH}" \
  HERMES_BIN=hermes \
  TRIAGE_TOKEN_FILE="${TEST_DIR}/missing-triage-token-file" \
  HERMES_ENV_FILE="${TEST_DIR}/missing-env-file" \
  LOCK_FILE="${missing_triage_state}/issue-loop.lock" \
  STATE_DIR="${missing_triage_state}" \
  bash "${SCRIPT_DIR}/run-hermes-triage-loop.sh" \
  >/dev/null 2>&1; then
  echo "run-hermes-triage-loop.sh unexpectedly reused the task token" >&2
  exit 1
fi

run_task_loop_case() {
  local scenario="$1"
  local expected_calls="$2"
  local state_dir="${TEST_DIR}/task-runner-${scenario}"
  local calls_file="${state_dir}/hermes-calls"

  mkdir -p "${state_dir}"
  : >"${calls_file}"
  PATH="${TEST_DIR}/bin:${PATH}" \
    GH_BIN=gh \
    GH_TOKEN=test-token \
    REPO_SLUG=example/tinker \
    APPROVER_LOGINS=TrustedMaintainer \
    FAKE_GH_SCENARIO="${scenario}" \
    HERMES_BIN=hermes \
    HERMES_CALLS_FILE="${calls_file}" \
    LOCK_FILE="${state_dir}/issue-loop.lock" \
    STATE_DIR="${state_dir}" \
    bash "${SCRIPT_DIR}/run-hermes-task-loop.sh" >/dev/null

  [[ "$(wc -l <"${calls_file}" | tr -d ' ')" == "${expected_calls}" ]]
}

run_task_loop_case empty 0
run_task_loop_case accepted_only 0
run_task_loop_case untrusted_plan 0
run_task_loop_case blocked_implement 0
run_task_loop_case ambiguous_commands 0
run_task_loop_case trusted_plan 1
run_task_loop_case trusted_implement 1

triage_runner_state="${TEST_DIR}/triage-runner"
mkdir -p "${triage_runner_state}"
: >"${triage_runner_state}/hermes-calls"
for expected_calls in 1 1; do
  PATH="${TEST_DIR}/bin:${PATH}" \
    GH_BIN=gh \
    TRIAGE_GH_TOKEN=triage-token \
    REPO_SLUG=example/tinker \
    FAKE_GH_SCENARIO=triage_accepted \
    HERMES_BIN=hermes \
    HERMES_CALLS_FILE="${triage_runner_state}/hermes-calls" \
    LOCK_FILE="${triage_runner_state}/issue-loop.lock" \
    STATE_DIR="${triage_runner_state}" \
    bash "${SCRIPT_DIR}/run-hermes-triage-loop.sh" >/dev/null
  [[ "$(wc -l <"${triage_runner_state}/hermes-calls" | tr -d ' ')" == "${expected_calls}" ]]
done

label_calls="${TEST_DIR}/label-calls"
: >"${label_calls}"
PATH="${TEST_DIR}/bin:${PATH}" \
  GH_BIN=gh \
  GH_TOKEN=test-token \
  REPO_SLUG=example/tinker \
  FAKE_LABEL_CALLS_FILE="${label_calls}" \
  bash "${SCRIPT_DIR}/setup-github-labels.sh"

if [[ "$(wc -l <"${label_calls}" | tr -d ' ')" != "20" ]]; then
  echo "setup-github-labels.sh did not configure all twenty labels" >&2
  exit 1
fi

echo "GitHub issue loop tests passed."
