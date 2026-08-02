#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd); source="$root/.github/workflows/stable-release.yml"; npm="$root/.github/workflows/npm-cli-publish.yml"; tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
cp "$source" "$tmp/stable.yml"
sed 's/workflow_call:/workflow_dispatch:/' "$source" >"$tmp/direct.yml"
if TINKERCLOUD_STABLE_WORKFLOW_FILE="$tmp/direct.yml" "$root/scripts/check-stable-release-workflows.sh" >/dev/null 2>&1; then exit 1; fi
printf '\n      - run: echo "${{ secrets.TINKERCLOUD_RELEASE_SIGNING_KEY_B64 }}"\n' >>"$tmp/stable.yml"
if TINKERCLOUD_STABLE_WORKFLOW_FILE="$tmp/stable.yml" "$root/scripts/check-stable-release-workflows.sh" >/dev/null 2>&1; then exit 1; fi
sed 's/"next" ]] ||/"beta" ]] ||/' "$npm" >"$tmp/tag.yml"
if TINKERCLOUD_NPM_WORKFLOW_FILE="$tmp/tag.yml" "$root/scripts/check-stable-release-workflows.sh" >/dev/null 2>&1; then exit 1; fi
echo 'stable release secret-boundary checker self-test passed'
