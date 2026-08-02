#!/bin/sh
# Deny drift in the internal beta signing workflow; only release.yml may dispatch it.
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
workflow=${TINKERCLOUD_BETA_WORKFLOW_FILE:-"$root/.github/workflows/beta-release.yml"}
test -f "$workflow" || exit 1
require() { grep -F -- "$1" "$workflow" >/dev/null || { echo "$2" >&2; exit 1; }; }
reject() { ! grep -E -- "$1" "$workflow" >/dev/null || { echo "$2" >&2; exit 1; }; }
require 'workflow_call:' 'beta signing workflow must be reusable only'
reject '^  workflow_dispatch:' 'beta signing workflow must not be dispatched directly'
require 'environment: beta-release' 'beta signing environment is required'
require 'TINKERCLOUD_REQUIRED_RELEASE_CHANNEL=beta' 'beta key policy is required'
require 'publish-beta-$tag' 'beta confirmation is required'
require 'GitHub release already exists' 'existing release must deny'
require 'cancel-in-progress: false' 'beta release must not cancel publication'
test "$(grep -c -F -- 'contents: write' "$workflow")" = 1 || { echo 'beta contents write scope drift' >&2; exit 1; }
reject '^[[:space:]]+(actions|checks|deployments|discussions|id-token|issues|packages|pages|pull-requests|security-events|statuses):[[:space:]]+write' 'beta grants unnecessary write permission'
require 'git merge-base --is-ancestor "$source_commit" origin/main' 'beta main ancestry gate missing'
require 'visibility" != "PUBLIC"' 'public repository gate missing'
require 'sdk_version=$(node ./scripts/extract-sdk-version.mjs sdk/typescript/src/index.ts)' 'SDK version gate missing'
require 'TINKERCLOUD_BETA_RELEASE_SIGNING_KEY_B64' 'beta signing secret missing'
test "$(grep -c -F -- 'TINKERCLOUD_BETA_RELEASE_SIGNING_KEY_B64' "$workflow")" = 1 || { echo 'beta signing secret exposure drift' >&2; exit 1; }
require 'mktemp "$RUNNER_TEMP/.tinkercloud-beta-release-key.XXXXXX"' 'random beta key path missing'
require 'unset SIGNING_KEY_B64' 'beta key cleanup missing'
require './scripts/release-build.sh "$VERSION" "$release_dir"' 'canonical release builder missing'
test "$(grep -c -F -- './scripts/release-verify.sh' "$workflow")" -ge 2 || { echo 'local and remote release verification required' >&2; exit 1; }
require '--draft' 'draft-first release missing'
require '--prerelease' 'prerelease classification missing'
require 'gh release download "$tag"' 'remote draft download missing'
require 'cmp "$RUNNER_TEMP/local-assets.txt" "$RUNNER_TEMP/remote-assets.txt"' 'remote asset comparison missing'
require '--latest=false' 'beta latest denial missing'
require 'TINKER_INSTALL_DIR="$install_dir" sh' 'public installer smoke missing'
reject 'npm[[:space:]]+(publish|unpublish)' 'signing workflow must not publish npm'
reject 'secrets:[[:space:]]+inherit' 'internal workflow must not inherit ambient secrets'
reject '--clobber|gh[[:space:]]+release[[:space:]]+(delete|upload)' 'beta may not replace release state'
reject 'git[[:space:]]+(tag[[:space:]]+-f|push[[:space:]].*--force)' 'beta may not move tags'
for ref in $(sed -n 's/.*uses: [^@]*@\([^ #]*\).*/\1/p' "$workflow"); do
  printf '%s\n' "$ref" | grep -E '^[0-9a-f]{40}$' >/dev/null || { echo "unpinned beta action: $ref" >&2; exit 1; }
done
echo 'beta reusable workflow contract passed'
