#!/usr/bin/env python3
import json
import os
import sys
from pathlib import Path


def main() -> int:
    if len(sys.argv) != 3:
        print("usage: mark-triage-seen.py SNAPSHOT SEEN_FILE", file=sys.stderr)
        return 2

    snapshot_path = Path(sys.argv[1])
    seen_path = Path(sys.argv[2])
    snapshot = json.loads(snapshot_path.read_text())
    try:
        seen = json.loads(seen_path.read_text()) if seen_path.exists() else {}
    except (json.JSONDecodeError, OSError):
        seen = {}

    open_numbers = {
        str(issue["number"])
        for issue in snapshot.get("issues", [])
        if isinstance(issue.get("number"), int)
    }
    seen = {number: revision for number, revision in seen.items() if number in open_numbers}
    actionable = set(snapshot.get("actionableIssueNumbers", []))
    for issue in snapshot.get("issues", []):
        if issue.get("number") in actionable and issue.get("updatedAt"):
            seen[str(issue["number"])] = issue["updatedAt"]

    seen_path.parent.mkdir(parents=True, exist_ok=True)
    temporary = seen_path.with_name(f".{seen_path.name}.{os.getpid()}.tmp")
    temporary.write_text(json.dumps(seen, indent=2, sort_keys=True) + "\n")
    temporary.replace(seen_path)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
