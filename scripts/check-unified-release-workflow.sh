#!/bin/sh
# Static deny gates for the sole manual public-release entrypoint.
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
workflow=${TINKERCLOUD_RELEASE_WORKFLOW_FILE:-"$root/.github/workflows/release.yml"}
test -f "$workflow" || { echo "unified release workflow unavailable" >&2; exit 1; }
require() { grep -F -- "$1" "$workflow" >/dev/null || { echo "$2" >&2; exit 1; }; }
reject() { ! grep -E -- "$1" "$workflow" >/dev/null || { echo "$2" >&2; exit 1; }; }
require 'workflow_dispatch:' 'release must be manually dispatched'
reject '^  (push|pull_request|pull_request_target|schedule|repository_dispatch):' 'release has an unauthorized trigger'
require 'cancel-in-progress: false' 'release must not cancel publication'
require 'Public release channel' 'channel selection is missing'
require '  - beta' 'beta channel is missing'
require '  - stable' 'stable channel is missing'
require 'preflight:' 'preflight must deny incomplete releases before signing'
require 'for package in %40tinkercloud%2Fsdk %40tinkercloud%2Fcli' 'both npm packages must be preflighted'
require '$package/$VERSION' 'both npm package versions must be preflighted'
require 'npm package version already exists' 'existing npm version must deny before public mutation'
require 'uses: ./.github/workflows/beta-release.yml' 'beta signing path is missing'
require 'uses: ./.github/workflows/stable-release.yml' 'stable signing path is missing'
require 'uses: ./.github/workflows/npm-sdk-publish.yml' 'SDK publishing path is missing'
require 'uses: ./.github/workflows/npm-cli-publish.yml' 'CLI publishing path is missing'
require 'dist_tag: beta' 'beta SDK tag is missing'
require 'dist_tag: next' 'beta CLI tag is missing'
test "$(grep -c -F -- 'dist_tag: latest' "$workflow")" = 2 || { echo 'stable must use latest for both packages' >&2; exit 1; }
require 'needs: beta-release' 'beta package publication must follow signed release'
require 'needs: stable-release' 'stable package publication must follow signed release'
reject 'npm[[:space:]]+(publish|unpublish)' 'entrypoint must delegate rather than publish directly'
reject 'secrets:[[:space:]]+inherit' 'release must not inherit ambient secrets'
for file in beta-release.yml stable-release.yml npm-sdk-publish.yml npm-cli-publish.yml; do
  candidate="$root/.github/workflows/$file"
  test -f "$candidate" || { echo "reusable workflow missing: $file" >&2; exit 1; }
  grep -F 'workflow_call:' "$candidate" >/dev/null || { echo "$file must be internal reusable workflow" >&2; exit 1; }
  if grep -E '^  workflow_dispatch:' "$candidate" >/dev/null; then echo "$file must not be manually dispatchable" >&2; exit 1; fi
done
echo 'unified release workflow contract passed'
