#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
release=${TINKERCLOUD_RELEASE_WORKFLOW_FILE:-"$root/.github/workflows/release.yml"}
reusable=${TINKERCLOUD_STABLE_WORKFLOW_FILE:-"$root/.github/workflows/stable-release.yml"}
publisher=${TINKERCLOUD_RELEASE_PUBLISHER_FILE:-"$root/scripts/publish-github-release.sh"}
npm=${TINKERCLOUD_NPM_WORKFLOW_FILE:-"$root/.github/workflows/npm-cli-publish.yml"}
require() { grep -F -- "$1" "$2" >/dev/null || { echo "$3" >&2; exit 1; }; }
reject() { ! grep -E -- "$1" "$2" >/dev/null || { echo "$3" >&2; exit 1; }; }
require 'stable-release:' "$release" 'direct stable release job is missing'
require 'environment: stable-release' "$release" 'direct stable environment is missing'
require 'SIGNING_KEY_B64: ${{ secrets.TINKERCLOUD_RELEASE_SIGNING_KEY_B64 }}' "$release" 'stable secret must be attached directly to release.yml'
require './scripts/publish-github-release.sh stable' "$release" 'direct stable publisher is missing'
test "$(grep -c -F -- 'contents: write' "$release")" = 2 || { echo 'only direct protected jobs may have contents write' >&2; exit 1; }
require 'stable-validation:' "$release" 'stable validation job is missing'
require 'TINKERCLOUD_REQUIRED_RELEASE_CHANNEL=production' "$reusable" 'stable production key policy is required'
require 'publish-stable-$tag' "$reusable" 'stable confirmation is required'
require 'git merge-base --is-ancestor "$source_commit" origin/main' "$reusable" 'stable ancestry gate is missing'
require './scripts/release-test.sh' "$reusable" 'stable release tamper gate is missing'
require 'latest/download' "$publisher" 'stable latest installer smoke is missing'
reject 'npm[[:space:]]+(publish|unpublish)' "$release" 'GitHub signing stage must not publish npm'
require 'workflow_call:' "$reusable" 'stable validation must remain reusable only'
reject 'TINKERCLOUD_RELEASE_SIGNING_KEY_B64|SIGNING_KEY_B64|environment:[[:space:]]+stable-release|contents:[[:space:]]+write' "$reusable" 'stable reusable workflow must not receive signing authority'
reject 'secrets:[[:space:]]+inherit' "$release" 'release must not inherit ambient secrets'
reject '--clobber|gh[[:space:]]+release[[:space:]]+(delete|upload)' "$publisher" 'release may not replace public state'
test -f "$release" && test -f "$reusable" && test -f "$publisher" && test -f "$npm" || { echo 'release component unavailable' >&2; exit 1; }
require 'cancel-in-progress: false' "$release" 'release must not cancel publication'
require 'GitHub release already exists' "$reusable" 'stable existing release denial missing'
require '[[ "$visibility" == "PUBLIC" ]]' "$reusable" 'stable public repository gate missing'
test "$(grep -c -F -- 'TINKERCLOUD_RELEASE_SIGNING_KEY_B64' "$release")" = 1 || { echo 'stable signing secret exposure drift' >&2; exit 1; }
require 'mktemp "$RUNNER_TEMP/.tinkercloud-${key_label}-release-key.XXXXXX"' "$publisher" 'random stable key path missing'; require 'unset SIGNING_KEY_B64' "$publisher" 'stable key cleanup missing'; require './scripts/release-build.sh "$VERSION" "$release_dir"' "$publisher" 'stable builder missing'; test "$(grep -c -F -- './scripts/release-verify.sh' "$publisher")" -ge 2 || { echo 'stable local/remote verification missing' >&2; exit 1; }; require '--draft' "$publisher" 'stable draft-first missing'; require 'gh release download "$tag"' "$publisher" 'stable remote download missing'; require 'cmp "$RUNNER_TEMP/local-assets.txt" "$RUNNER_TEMP/remote-assets.txt"' "$publisher" 'stable asset comparison missing'; require '--prerelease=false --latest' "$publisher" 'stable may not become prerelease'
require 'environment: npm-cli' "$npm" 'CLI publisher requires npm-cli environment'
require 'id-token: write' "$npm" 'CLI publisher requires OIDC'
require '[[ "$DIST_TAG" == "next" ]]' "$npm" 'CLI beta next tag must be fixed'
require '[[ "$DIST_TAG" == "latest" ]]' "$npm" 'CLI stable latest tag must be fixed'
require 'publish-$CHANNEL-$tag' "$npm" 'CLI must use parent confirmation'
require 'npm publish "$CLI_TARBALL"' "$npm" 'CLI must publish verified candidate'
require 'cancel-in-progress: false' "$npm" 'CLI publisher must not cancel publication'
reject 'contents:[[:space:]]+write' "$npm" 'CLI publisher does not need contents write'
reject 'NODE_AUTH_TOKEN|NPM_TOKEN|npm[[:space:]]+unpublish' "$npm" 'CLI publisher may not use long-lived credentials or unpublish'
reject 'secrets:[[:space:]]+inherit' "$npm" 'CLI publisher may not inherit secrets'
reject 'npm[[:space:]]+(unpublish|deprecate|dist-tag[[:space:]]+(rm|remove))' "$npm" 'CLI publisher may not conceal state'
test "$(grep -c -F -- 'id-token: write' "$npm")" = 1 || { echo 'CLI OIDC scope drift' >&2; exit 1; }; require 'TINKERCLOUD_REQUIRED_RELEASE_CHANNEL="$required_authority"' "$npm" 'CLI selected key policy missing'; require 'required_authority=beta' "$npm" 'CLI beta authority missing'; require 'required_authority=production' "$npm" 'CLI stable authority missing'; require 'git merge-base --is-ancestor "$source_commit" origin/main' "$npm" 'CLI ancestry gate missing'; require 'false\ttrue\t' "$npm" 'CLI prerelease-state gate missing'; require 'false\tfalse\t' "$npm" 'CLI stable-state gate missing'; require 'package_status" == "200"' "$npm" 'CLI bootstrap denial missing'; require 'version_status" == "404"' "$npm" 'CLI existing-version denial missing'; require 'node-version: "24"' "$npm" 'CLI trusted-publishing Node missing'; test "$(grep -c -F -- 'npm install --global npm@11.5.1' "$npm")" = 2 || { echo 'CLI pinned npm drift' >&2; exit 1; }; require 'gh release download "$tag"' "$npm" 'CLI release download missing'; require './scripts/release-verify.sh "$release_dir"' "$npm" 'CLI release verification missing'; require './scripts/distribution-prepare.sh' "$npm" 'CLI candidate generator missing'
for file in "$release" "$reusable" "$npm"; do reject 'git[[:space:]]+(tag[[:space:]]+-f|push[[:space:]].*--force)' "$file" 'release workflow may not move tags'; for ref in $(sed -n 's/.*uses: [^@]*@\([^ #]*\).*/\1/p' "$file"); do printf '%s\n' "$ref" | grep -E '^[0-9a-f]{40}$' >/dev/null || { echo "unpinned release action: $ref" >&2; exit 1; }; done; done
echo 'stable release secret-boundary contract passed'
