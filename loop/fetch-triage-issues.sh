#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
STATE_DIR="${STATE_DIR:-${REPO_DIR}/.hermes-triage-loop}"
OUT_FILE="${1:-${STATE_DIR}/issues.json}"
SEEN_FILE="${SEEN_FILE:-${STATE_DIR}/seen.json}"
TOKEN_FILE="${TRIAGE_TOKEN_FILE:-${REPO_DIR}/TRIAGE_GITHUB_TOKEN}"
HERMES_ENV_FILE="${HERMES_ENV_FILE:-${HERMES_HOME:-$HOME/.hermes}/.env}"
GH_BIN="${GH_BIN:-gh}"
REPO_SLUG="${REPO_SLUG:-ChrisMarxDev/tinkercloud}"

mkdir -p "$(dirname "${OUT_FILE}")"

if ! command -v "${GH_BIN}" >/dev/null 2>&1; then
  echo "GitHub CLI not found. Set GH_BIN or install gh." >&2
  exit 1
fi

triage_token="${TRIAGE_GH_TOKEN:-${TRIAGE_GITHUB_TOKEN:-}}"
if [[ -z "${triage_token}" && -s "${TOKEN_FILE}" ]]; then
  triage_token="$(tr -d '\r\n' <"${TOKEN_FILE}")"
elif [[ -z "${triage_token}" && -s "${HERMES_ENV_FILE}" ]]; then
  triage_token="$(
    python3 - "${HERMES_ENV_FILE}" <<'PY'
import sys
from pathlib import Path

for line in Path(sys.argv[1]).read_text().splitlines():
    line = line.strip()
    if not line or line.startswith("#") or "=" not in line:
        continue
    key, value = line.split("=", 1)
    if key.strip() == "TRIAGE_GITHUB_TOKEN":
        print(value.strip().strip('"').strip("'"))
        break
PY
  )"
fi

if [[ -z "${triage_token}" ]]; then
  echo "Missing triage token. Set TRIAGE_GH_TOKEN/TRIAGE_GITHUB_TOKEN, ${HERMES_ENV_FILE}, or ${TOKEN_FILE}." >&2
  exit 1
fi
GH_TOKEN="${triage_token}"
export GH_TOKEN

tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT

"${GH_BIN}" issue list \
  --repo "${REPO_SLUG}" \
  --state open \
  --limit 100 \
  --json number,title,body,author,labels,url,updatedAt,createdAt \
  >"${tmp_dir}/open.json"

python3 - "${OUT_FILE}" "${tmp_dir}/open.json" "${SEEN_FILE}" "${REPO_SLUG}" <<'PY'
import json
import sys
from pathlib import Path

out_file = Path(sys.argv[1])
issues_file = Path(sys.argv[2])
seen_file = Path(sys.argv[3])
repo = sys.argv[4]
status_labels = {
    "status/needs-triage",
    "status/needs-info",
    "status/accepted",
    "status/blocked",
    "status/in-progress",
}
command_labels = {"agent/plan", "agent/implement"}

try:
    seen = json.loads(seen_file.read_text()) if seen_file.exists() else {}
except (json.JSONDecodeError, OSError):
    seen = {}

issues = []
for issue in json.loads(issues_file.read_text()):
    labels = {label["name"] for label in issue.get("labels", [])}
    issue["bodyTruncated"] = len(issue.get("body") or "") > 16000
    issue["body"] = (issue.get("body") or "")[:16000]
    issue["statusLabels"] = sorted(labels.intersection(status_labels))
    issue["commandLabels"] = sorted(labels.intersection(command_labels))
    issue["changedSinceTriage"] = seen.get(str(issue["number"])) != issue.get("updatedAt")
    issue["needsStatusNormalization"] = len(issue["statusLabels"]) != 1
    issue["triageActionable"] = (
        issue["changedSinceTriage"]
        or issue["needsStatusNormalization"]
        or issue["statusLabels"] == ["status/needs-triage"]
    )
    issues.append(issue)

actionable = [issue for issue in issues if issue["triageActionable"]]
payload = {
    "repo": repo,
    "counts": {
        "open": len(issues),
        "needsTriage": sum(issue["statusLabels"] == ["status/needs-triage"] for issue in issues),
        "needsNormalization": sum(issue["needsStatusNormalization"] for issue in issues),
        "changed": sum(issue["changedSinceTriage"] for issue in issues),
        "actionable": len(actionable),
    },
    "actionableIssueNumbers": sorted(issue["number"] for issue in actionable),
    "issues": sorted(issues, key=lambda item: item["number"]),
}

out_file.write_text(json.dumps(payload, indent=2) + "\n")
print(json.dumps(payload["counts"], sort_keys=True))
PY
