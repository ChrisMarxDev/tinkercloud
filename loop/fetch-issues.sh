#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
STATE_DIR="${STATE_DIR:-${REPO_DIR}/.hermes-task-loop}"
OUT_FILE="${1:-${STATE_DIR}/issues.json}"
TOKEN_FILE="${TOKEN_FILE:-${REPO_DIR}/GITHUB_TOKEN}"
HERMES_ENV_FILE="${HERMES_ENV_FILE:-${HERMES_HOME:-$HOME/.hermes}/.env}"
GH_BIN="${GH_BIN:-gh}"
REPO_SLUG="${REPO_SLUG:-ChrisMarxDev/tiny}"
APPROVER_LOGINS="${APPROVER_LOGINS:-ChrisMarxDev}"

mkdir -p "$(dirname "${OUT_FILE}")"

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

tmp_dir="$(mktemp -d)"
trap 'rm -rf "${tmp_dir}"' EXIT

labels=(inbox open pending plan implement)
for label in "${labels[@]}"; do
  "${GH_BIN}" issue list \
    --repo "${REPO_SLUG}" \
    --state open \
    --label "${label}" \
    --limit 100 \
    --json number,title,body,author,labels,url,updatedAt,createdAt \
    >"${tmp_dir}/${label}.json"
done

while IFS= read -r issue_number; do
  [[ -n "${issue_number}" ]] || continue
  if "${GH_BIN}" api \
    --paginate \
    --slurp \
    "repos/${REPO_SLUG}/issues/${issue_number}/events?per_page=100" \
    >"${tmp_dir}/${issue_number}-events.json"; then
    :
  else
    printf '[]\n' >"${tmp_dir}/${issue_number}-events.json"
    : >"${tmp_dir}/${issue_number}-events-failed"
  fi
done < <(
  python3 - "${tmp_dir}/implement.json" <<'PY'
import json
import sys
from pathlib import Path

for issue in json.loads(Path(sys.argv[1]).read_text()):
    print(issue["number"])
PY
)

python3 - "${OUT_FILE}" "${tmp_dir}" "${REPO_SLUG}" "${APPROVER_LOGINS}" <<'PY'
import json
import sys
from pathlib import Path

out_file = Path(sys.argv[1])
tmp_dir = Path(sys.argv[2])
repo = sys.argv[3]
approvers = {
    login.strip().lower()
    for login in sys.argv[4].split(",")
    if login.strip()
}
state_labels = ("inbox", "open", "pending", "plan")
all_workflow_labels = (*state_labels, "implement")

by_number = {}
for label in all_workflow_labels:
    issues = json.loads((tmp_dir / f"{label}.json").read_text())
    for issue in issues:
        number = issue["number"]
        entry = by_number.setdefault(number, issue)
        entry_labels = {item["name"] for item in entry.get("labels", [])}
        entry["stateLabels"] = sorted(entry_labels.intersection(state_labels))
        entry["hasImplementLabel"] = "implement" in entry_labels

for issue in by_number.values():
    issue["bodyTruncated"] = len(issue.get("body") or "") > 16000
    issue["body"] = (issue.get("body") or "")[:16000]
    issue["implementationApproved"] = False
    issue["approvalActor"] = None
    issue["approvalLookupFailed"] = False

    if not issue["hasImplementLabel"]:
        continue

    number = issue["number"]
    if (tmp_dir / f"{number}-events-failed").exists():
        issue["approvalLookupFailed"] = True
        continue

    pages = json.loads((tmp_dir / f"{number}-events.json").read_text())
    events = []
    for page in pages:
        if isinstance(page, list):
            events.extend(page)
        elif isinstance(page, dict):
            events.append(page)

    implement_events = [
        (index, event)
        for index, event in enumerate(events)
        if event.get("event") in {"labeled", "unlabeled"}
        and (event.get("label") or {}).get("name") == "implement"
    ]
    if not implement_events:
        issue["approvalLookupFailed"] = True
        continue

    _, latest = max(
        implement_events,
        key=lambda item: (
            item[1].get("created_at") or "",
            item[1].get("id") if isinstance(item[1].get("id"), int) else -1,
            item[0],
        ),
    )
    actor = (latest.get("actor") or {}).get("login")
    issue["approvalActor"] = actor
    issue["implementationApproved"] = (
        latest.get("event") == "labeled"
        and isinstance(actor, str)
        and actor.lower() in approvers
    )

def is_actionable(issue):
    states = set(issue.get("stateLabels", []))
    if "pending" in states:
        return False
    if issue.get("implementationApproved"):
        return True
    return bool(states.intersection({"inbox", "plan"}))

actionable = [issue for issue in by_number.values() if is_actionable(issue)]
payload = {
    "repo": repo,
    "approverLogins": sorted(approvers),
    "counts": {
        "inbox": sum("inbox" in issue.get("stateLabels", []) for issue in by_number.values()),
        "open": sum("open" in issue.get("stateLabels", []) for issue in by_number.values()),
        "pending": sum("pending" in issue.get("stateLabels", []) for issue in by_number.values()),
        "plan": sum("plan" in issue.get("stateLabels", []) for issue in by_number.values()),
        "implement": sum(issue.get("hasImplementLabel", False) for issue in by_number.values()),
        "approvedImplement": sum(issue.get("implementationApproved", False) for issue in by_number.values()),
        "actionable": len(actionable),
    },
    "actionableIssueNumbers": sorted(issue["number"] for issue in actionable),
    "issues": sorted(by_number.values(), key=lambda item: item["number"]),
}

out_file.write_text(json.dumps(payload, indent=2) + "\n")
print(json.dumps(payload["counts"], sort_keys=True))
PY
