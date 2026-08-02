#!/bin/sh
# Deny drift in the internal SDK npm publisher.
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
workflow=${TINKERCLOUD_NPM_SDK_WORKFLOW_FILE:-"$root/.github/workflows/npm-sdk-publish.yml"}
test -f "$workflow" || exit 1
require() { grep -F -- "$1" "$workflow" >/dev/null || { echo "$2" >&2; exit 1; }; }
reject() { ! grep -E -- "$1" "$workflow" >/dev/null || { echo "$2" >&2; exit 1; }; }
require 'workflow_call:' 'SDK publisher must be reusable only'
reject '^  workflow_dispatch:' 'SDK publisher must not be dispatched directly'
require 'environment: npm-sdk' 'SDK publisher requires npm-sdk environment'
require 'id-token: write' 'SDK publisher requires OIDC'
require '[[ "$CHANNEL" == "beta" ]]' 'SDK beta channel must be explicit'
require '[[ "$DIST_TAG" == "beta" ]]' 'SDK beta tag must be fixed'
require '[[ "$DIST_TAG" == "latest" ]]' 'SDK stable tag must be fixed'
require 'publish-$CHANNEL-$tag' 'SDK must use parent confirmation'
require 'npm publish "$SDK_TARBALL"' 'SDK must publish exact release tarball'
require 'cancel-in-progress: false' 'SDK publisher must not cancel publication'
reject 'contents:[[:space:]]+write' 'SDK publisher does not need contents write'
test "$(grep -c -F -- 'id-token: write' "$workflow")" = 1 || { echo 'SDK OIDC scope drift' >&2; exit 1; }
require 'TINKERCLOUD_REQUIRED_RELEASE_CHANNEL="$required_authority"' 'SDK selected key policy missing'
require 'required_authority=beta' 'SDK beta authority missing'
require 'required_authority=production' 'SDK stable authority missing'
require 'git merge-base --is-ancestor "$source_commit" origin/main' 'SDK main ancestry gate missing'
require 'false\ttrue\t' 'SDK prerelease-state gate missing'
require 'false\tfalse\t' 'SDK stable-state gate missing'
require 'package_status" == "200"' 'SDK bootstrap denial missing'
require 'version_status" == "404"' 'SDK existing-version denial missing'
require 'node-version: "24"' 'SDK trusted-publishing Node missing'
test "$(grep -c -F -- 'npm install --global npm@11.5.1' "$workflow")" = 2 || { echo 'SDK pinned npm drift' >&2; exit 1; }
require 'gh release download "$tag"' 'SDK canonical release download missing'
require './scripts/release-verify.sh "$release_dir"' 'SDK complete release verification missing'
require 'node ./scripts/verify-sdk-publish-candidate.mjs "$VERSION" "$sdk_tarball"' 'SDK candidate verification missing'
require 'version.dist?.attestations?.url' 'SDK provenance verification missing'
require 'root["dist-tags"]?.[process.env.DIST_TAG]' 'SDK dist-tag verification missing'
require 'version.dist?.integrity !== process.env.SDK_INTEGRITY' 'SDK integrity verification missing'
require '"@tinkercloud/sdk@$VERSION"' 'SDK clean consumer missing'
reject 'NODE_AUTH_TOKEN|NPM_TOKEN|npm[[:space:]]+unpublish' 'SDK publisher may not use long-lived credentials or unpublish'
reject 'secrets:[[:space:]]+inherit' 'SDK publisher may not inherit secrets'
reject 'git[[:space:]]+(tag[[:space:]]+-f|push[[:space:]].*--force)' 'SDK publisher may not move tags'
reject '(^|[[:space:]])(npm[[:space:]]+(pack([[:space:]]|$)|run[[:space:]]+build|test([[:space:]]|$))|jsr[[:space:]]+publish|brew([[:space:]]|$))' 'SDK publisher may not rebuild or publish other channels'
reject 'npm[[:space:]]+(unpublish|deprecate|dist-tag[[:space:]]+(rm|remove))' 'SDK publisher may not conceal state'
reject '@tinkercloud/cli|CLI_TARBALL|tinkercloud-linux' 'SDK publisher may not package CLI/server'
for ref in $(sed -n 's/.*uses: [^@]*@\([^ #]*\).*/\1/p' "$workflow"); do
  printf '%s\n' "$ref" | grep -E '^[0-9a-f]{40}$' >/dev/null || { echo "unpinned SDK action: $ref" >&2; exit 1; }
done
echo 'SDK reusable npm workflow contract passed'
