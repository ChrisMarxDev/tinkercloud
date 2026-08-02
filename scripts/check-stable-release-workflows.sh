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
require "$stable" 'cancel-in-progress: false' 'stable release must not cancel publication'
test "$(grep -c -F -- 'contents: write' "$stable")" = 1 || { echo 'stable contents write scope drift' >&2; exit 1; }
reject "$stable" '^[[:space:]]+(actions|checks|deployments|discussions|id-token|issues|packages|pages|pull-requests|security-events|statuses):[[:space:]]+write' 'stable grants unnecessary write permission'
require "$stable" 'git merge-base --is-ancestor "$source_commit" origin/main' 'stable ancestry gate missing'
require "$stable" 'GitHub release already exists' 'stable existing release denial missing'
require "$stable" 'visibility" != "PUBLIC"' 'stable public repository gate missing'
require "$stable" 'TINKERCLOUD_RELEASE_SIGNING_KEY_B64' 'stable signing secret missing'
test "$(grep -c -F -- 'TINKERCLOUD_RELEASE_SIGNING_KEY_B64' "$stable")" = 1 || { echo 'stable signing secret exposure drift' >&2; exit 1; }
require "$stable" 'mktemp "$RUNNER_TEMP/.tinkercloud-stable-release-key.XXXXXX"' 'random stable key path missing'
require "$stable" 'unset SIGNING_KEY_B64' 'stable key cleanup missing'
require "$stable" './scripts/release-build.sh "$VERSION" "$release_dir"' 'stable builder missing'
test "$(grep -c -F -- './scripts/release-verify.sh' "$stable")" -ge 2 || { echo 'stable local/remote verification missing' >&2; exit 1; }
require "$stable" '--draft' 'stable draft-first missing'
require "$stable" 'gh release download "$tag"' 'stable remote download missing'
require "$stable" 'cmp "$RUNNER_TEMP/local-assets.txt" "$RUNNER_TEMP/remote-assets.txt"' 'stable asset comparison missing'
require "$stable" '--latest' 'stable latest promotion missing'
reject "$stable" 'npm[[:space:]]+(publish|unpublish)' 'stable signing workflow must not publish npm'
reject "$stable" '--prerelease([[:space:]]|$)' 'stable may not become prerelease'
reject "$stable" '--clobber|gh[[:space:]]+release[[:space:]]+(delete|upload)' 'stable may not replace release state'
require "$npm" 'environment: npm-cli' 'CLI publisher requires npm-cli environment'
require "$npm" 'id-token: write' 'CLI publisher requires OIDC'
require "$npm" '[[ "$DIST_TAG" == "next" ]]' 'CLI beta next tag must be fixed'
require "$npm" '[[ "$DIST_TAG" == "latest" ]]' 'CLI stable latest tag must be fixed'
require "$npm" 'publish-$CHANNEL-$tag' 'CLI must use parent confirmation'
require "$npm" 'npm publish "$CLI_TARBALL"' 'CLI must publish verified candidate'
require "$npm" 'cancel-in-progress: false' 'CLI publisher must not cancel publication'
reject "$npm" 'contents:[[:space:]]+write' 'CLI publisher does not need contents write'
test "$(grep -c -F -- 'id-token: write' "$npm")" = 1 || { echo 'CLI OIDC scope drift' >&2; exit 1; }
require "$npm" 'TINKERCLOUD_REQUIRED_RELEASE_CHANNEL="$required_authority"' 'CLI selected key policy missing'
require "$npm" 'required_authority=beta' 'CLI beta authority missing'
require "$npm" 'required_authority=production' 'CLI stable authority missing'
require "$npm" 'git merge-base --is-ancestor "$source_commit" origin/main' 'CLI ancestry gate missing'
require "$npm" 'false\ttrue\t' 'CLI prerelease-state gate missing'
require "$npm" 'false\tfalse\t' 'CLI stable-state gate missing'
require "$npm" 'package_status" == "200"' 'CLI bootstrap denial missing'
require "$npm" 'version_status" == "404"' 'CLI existing-version denial missing'
require "$npm" 'node-version: "24"' 'CLI trusted-publishing Node missing'
test "$(grep -c -F -- 'npm install --global npm@11.5.1' "$npm")" = 2 || { echo 'CLI pinned npm drift' >&2; exit 1; }
require "$npm" 'gh release download "$tag"' 'CLI release download missing'
require "$npm" './scripts/release-verify.sh "$release_dir"' 'CLI release verification missing'
require "$npm" './scripts/distribution-prepare.sh' 'CLI candidate generator missing'
reject "$npm" 'NODE_AUTH_TOKEN|NPM_TOKEN|npm[[:space:]]+unpublish' 'CLI publisher may not use long-lived credentials or unpublish'
reject "$npm" 'secrets:[[:space:]]+inherit' 'CLI publisher may not inherit secrets'
reject "$npm" 'npm[[:space:]]+(unpublish|deprecate|dist-tag[[:space:]]+(rm|remove))' 'CLI publisher may not conceal state'
for file in "$stable" "$npm"; do
  reject "$file" 'git[[:space:]]+(tag[[:space:]]+-f|push[[:space:]].*--force)' 'release workflow may not move tags'
  for ref in $(sed -n 's/.*uses: [^@]*@\([^ #]*\).*/\1/p' "$file"); do
    printf '%s\n' "$ref" | grep -E '^[0-9a-f]{40}$' >/dev/null || { echo "unpinned release action: $ref" >&2; exit 1; }
  done
done
echo 'stable and CLI reusable workflow contracts passed'
