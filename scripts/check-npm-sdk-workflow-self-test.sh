#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd); source="$root/.github/workflows/npm-sdk-publish.yml"; tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
sed 's/"beta" ]] ||/"next" ]] ||/' "$source" >"$tmp/tag.yml"
if TINKERCLOUD_NPM_SDK_WORKFLOW_FILE="$tmp/tag.yml" "$root/scripts/check-npm-sdk-workflow.sh" >/dev/null 2>&1; then exit 1; fi
echo 'SDK reusable npm workflow checker self-test passed'
