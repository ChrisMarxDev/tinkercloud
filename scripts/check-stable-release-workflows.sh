#!/bin/sh
# Deny drift in stable signing and internal CLI npm workflows.
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
stable=${TINKERCLOUD_STABLE_WORKFLOW_FILE:-"$root/.github/workflows/stable-release.yml"}
npm=${TINKERCLOUD_NPM_WORKFLOW_FILE:-"$root/.github/workflows/npm-cli-publish.yml"}
require() { grep -F -- "$2" "$1" >/dev/null || { echo "$3" >&2; exit 1; }; }
reject() { ! grep -E -- "$2" "$1" >/dev/null || { echo "$3" >&2; exit 1; }; }
for file in "$stable" "$npm"; do test -f "$file" || exit 1; require "$file" 'workflow_call:' 'internal workflow must be reusable only'; reject "$file" '^  workflow_dispatch:' 'internal workflow must not dispatch directly'; done
require "$stable" 'environment: stable-release' 'stable signing environment is required'
require "$stable" 'TINKERCLOUD_REQUIRED_RELEASE_CHANNEL=production' 'stable production key policy is required'
require "$stable" 'publish-stable-$tag' 'stable confirmation is required'
reject "$stable" 'npm[[:space:]]+(publish|unpublish)' 'stable signing workflow must not publish npm'
require "$npm" 'environment: npm-cli' 'CLI publisher requires npm-cli environment'
require "$npm" 'id-token: write' 'CLI publisher requires OIDC'
require "$npm" '[[ "$DIST_TAG" == "next" ]]' 'CLI beta next tag must be fixed'
require "$npm" '[[ "$DIST_TAG" == "latest" ]]' 'CLI stable latest tag must be fixed'
require "$npm" 'publish-$CHANNEL-$tag' 'CLI must use parent confirmation'
require "$npm" 'npm publish "$CLI_TARBALL"' 'CLI must publish verified candidate'
reject "$npm" 'NODE_AUTH_TOKEN|NPM_TOKEN|npm[[:space:]]+unpublish' 'CLI publisher may not use long-lived credentials or unpublish'
echo 'stable and CLI reusable workflow contracts passed'
