#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd); npm="$root/.github/workflows/npm-cli-publish.yml"; tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
sed 's/"next" ]] ||/"beta" ]] ||/' "$npm" >"$tmp/tag.yml"
if TINKERCLOUD_NPM_WORKFLOW_FILE="$tmp/tag.yml" "$root/scripts/check-stable-release-workflows.sh" >/dev/null 2>&1; then exit 1; fi
echo 'stable and CLI reusable workflow checker self-test passed'
