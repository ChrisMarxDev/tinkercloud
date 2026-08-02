#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd); source="$root/.github/workflows/beta-release.yml"; tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
cp "$source" "$tmp/beta.yml"
sed 's/workflow_call:/workflow_dispatch:/' "$source" >"$tmp/direct.yml"
if TINKERCLOUD_BETA_WORKFLOW_FILE="$tmp/direct.yml" "$root/scripts/check-beta-release-workflow.sh" >/dev/null 2>&1; then exit 1; fi
printf '\n      - run: echo "${{ secrets.TINKERCLOUD_BETA_RELEASE_SIGNING_KEY_B64 }}"\n' >>"$tmp/beta.yml"
if TINKERCLOUD_BETA_WORKFLOW_FILE="$tmp/beta.yml" "$root/scripts/check-beta-release-workflow.sh" >/dev/null 2>&1; then exit 1; fi
echo 'beta release secret-boundary checker self-test passed'
