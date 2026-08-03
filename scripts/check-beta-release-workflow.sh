#!/bin/sh
# Environment signing secrets must stay in direct release.yml jobs, never a reusable call.
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
release=${TINKERCLOUD_RELEASE_WORKFLOW_FILE:-"$root/.github/workflows/release.yml"}
reusable=${TINKERCLOUD_BETA_WORKFLOW_FILE:-"$root/.github/workflows/beta-release.yml"}
publisher=${TINKERCLOUD_RELEASE_PUBLISHER_FILE:-"$root/scripts/publish-github-release.sh"}
test -f "$release" && test -f "$reusable" && test -f "$publisher" || { echo 'release workflow component unavailable' >&2; exit 1; }
require() { grep -F -- "$1" "$2" >/dev/null || { echo "$3" >&2; exit 1; }; }
reject() { ! grep -E -- "$1" "$2" >/dev/null || { echo "$3" >&2; exit 1; }; }
require 'beta-release:' "$release" 'direct beta release job is missing'
require 'environment: beta-release' "$release" 'direct beta environment is missing'
require 'SIGNING_KEY_B64: ${{ secrets.TINKERCLOUD_BETA_RELEASE_SIGNING_KEY_B64 }}' "$release" 'beta secret must be attached directly to release.yml'
require './scripts/publish-github-release.sh beta' "$release" 'direct beta publisher is missing'
require 'cancel-in-progress: false' "$release" 'release must not cancel publication'
test "$(grep -c -F -- 'contents: write' "$release")" = 2 || { echo 'only direct protected jobs may have contents write' >&2; exit 1; }
require 'beta-validation:' "$release" 'beta validation job is missing'
require 'contents: read' "$release" 'beta validation must have read-only contents'
require 'TINKERCLOUD_REQUIRED_RELEASE_CHANNEL=beta' "$reusable" 'beta key policy is required'
require 'publish-beta-$tag' "$reusable" 'beta confirmation is required'
require 'git merge-base --is-ancestor "$source_commit" origin/main' "$reusable" 'beta ancestry gate is missing'
require 'visibility" != "PUBLIC"' "$reusable" 'beta public-repository denial is missing'
require './scripts/release-test.sh' "$reusable" 'beta release tamper gate is missing'
require 'GitHub release already exists' "$reusable" 'beta existing-release denial is missing'
require 'sdk_version=$(node ./scripts/extract-sdk-version.mjs sdk/typescript/src/index.ts)' "$reusable" 'beta SDK version gate is missing'
test "$(grep -c -F -- 'TINKERCLOUD_BETA_RELEASE_SIGNING_KEY_B64' "$release")" = 1 || { echo 'beta signing secret exposure drift' >&2; exit 1; }
require 'mktemp "$RUNNER_TEMP/.tinkercloud-${key_label}-release-key.XXXXXX"' "$publisher" 'random beta key path missing'
require 'unset SIGNING_KEY_B64' "$publisher" 'beta key cleanup missing'
require './scripts/release-build.sh "$VERSION" "$release_dir"' "$publisher" 'canonical release builder missing'
test "$(grep -c -F -- './scripts/release-verify.sh' "$publisher")" -ge 2 || { echo 'local and remote verification required' >&2; exit 1; }
require '--draft' "$publisher" 'draft-first release missing'; require '--prerelease' "$publisher" 'prerelease classification missing'; require 'gh release download "$tag"' "$publisher" 'remote draft download missing'; require 'cmp "$RUNNER_TEMP/local-assets.txt" "$RUNNER_TEMP/remote-assets.txt"' "$publisher" 'remote asset comparison missing'; require '--latest=false' "$publisher" 'beta latest denial missing'
require 'TINKER_INSTALL_DIR="$install_dir" sh' "$publisher" 'beta public installer smoke is missing'
"$root/scripts/check-release-installer-smoke.sh"
reject 'npm[[:space:]]+(publish|unpublish)' "$release" 'GitHub signing stage must not publish npm'
require 'workflow_call:' "$reusable" 'beta validation must remain reusable only'
reject '^  workflow_dispatch:' "$reusable" 'beta validation must not be dispatched directly'
reject 'TINKERCLOUD_BETA_RELEASE_SIGNING_KEY_B64|SIGNING_KEY_B64|environment:[[:space:]]+beta-release|contents:[[:space:]]+write' "$reusable" 'beta reusable workflow must not receive signing authority'
reject 'secrets:[[:space:]]+inherit' "$release" 'release must not inherit ambient secrets'
reject '--clobber|gh[[:space:]]+release[[:space:]]+(delete|upload)' "$publisher" 'release may not replace public state'
reject 'git[[:space:]]+(tag[[:space:]]+-f|push[[:space:]].*--force)' "$release" 'release may not move tags'
for file in "$release" "$reusable"; do for ref in $(sed -n 's/.*uses: [^@]*@\([^ #]*\).*/\1/p' "$file"); do printf '%s\n' "$ref" | grep -E '^[0-9a-f]{40}$' >/dev/null || { echo "unpinned action: $ref" >&2; exit 1; }; done; done
echo 'beta release secret-boundary contract passed'
