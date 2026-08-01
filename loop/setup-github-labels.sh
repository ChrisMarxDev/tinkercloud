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
inbox|d4c5f9|Untrusted intake awaiting refinement
open|0e8a16|Refined and ready, but not authorized for implementation
pending|fbca04|Blocked on a decision, review, dependency, or external input
plan|5319e7|Agent should post a technical plan, not code
implement|b60205|Trusted maintainer command authorizing implementation
LABELS

