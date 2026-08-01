#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
TOKEN_FILE="${TOKEN_FILE:-${REPO_DIR}/GITHUB_TOKEN}"
HERMES_ENV_FILE="${HERMES_ENV_FILE:-${HERMES_HOME:-$HOME/.hermes}/.env}"
GH_BIN="${GH_BIN:-gh}"
REPO_SLUG="${REPO_SLUG:-ChrisMarxDev/tinkercloud}"

if ! command -v "${GH_BIN}" >/dev/null 2>&1; then
  echo "GitHub CLI not found. Set GH_BIN or install gh." >&2
  exit 1
fi

if [[ -z "${GH_TOKEN:-}" ]]; then
  if [[ -n "${GITHUB_TOKEN:-}" ]]; then
    GH_TOKEN="${GITHUB_TOKEN}"
  elif [[ -s "${TOKEN_FILE}" ]]; then
    GH_TOKEN="$(tr -d '\r\n' <"${TOKEN_FILE}")"
  elif [[ -s "${HERMES_ENV_FILE}" ]]; then
    GH_TOKEN="$(
      python3 - "${HERMES_ENV_FILE}" <<'PY'
import sys
from pathlib import Path

for line in Path(sys.argv[1]).read_text().splitlines():
    line = line.strip()
    if not line or line.startswith("#") or "=" not in line:
        continue
    key, value = line.split("=", 1)
    if key.strip() == "GITHUB_TOKEN":
        print(value.strip().strip('"').strip("'"))
        break
PY
    )"
  fi
fi

if [[ -z "${GH_TOKEN:-}" ]]; then
  echo "Missing GitHub token. Set GH_TOKEN/GITHUB_TOKEN, ${HERMES_ENV_FILE}, or ${TOKEN_FILE}." >&2
  exit 1
fi
export GH_TOKEN

while IFS='|' read -r name color description; do
  "${GH_BIN}" label create "${name}" \
    --repo "${REPO_SLUG}" \
    --color "${color}" \
    --description "${description}" \
    --force
done <<'LABELS'
status/needs-triage|d4c5f9|New or reopened issue awaiting classification
status/needs-info|fbca04|Waiting for concrete reporter information
status/accepted|0e8a16|Refined and valid without implementation authority
status/blocked|b60205|Waiting for a maintainer decision or external dependency
status/in-progress|1d76db|An open pull request is addressing this issue
type/bug|d73a4a|A reproducible defect in supported behavior
type/feature|a2eeef|A focused product capability request
type/docs|0075ca|Documentation content or usability
type/question|d876e3|Setup, usage, or support question
type/maintenance|c5def5|Repository, dependency, test, or tooling maintenance
priority/critical|b60205|Release blocker or urgent supported-behavior regression
priority/next|fbca04|Deliberately selected for near-term work
priority/backlog|ededed|Valid work without a near-term commitment
resolution/duplicate|cfd3d7|Closed in favor of a canonical issue
resolution/invalid|e4e669|Closed because the report is invalid or not reproducible
resolution/not-planned|ffffff|Closed because the project will not pursue it
agent/plan|5319e7|Trusted maintainer command requesting a technical plan
agent/implement|000000|Trusted maintainer command authorizing implementation
good first issue|7057ff|Suitable for a first contribution
help wanted|008672|Maintainer welcomes an external contribution
LABELS
