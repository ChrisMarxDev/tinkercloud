#!/bin/sh
# Static deny gates for the SDK-only npm beta publisher.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
workflow=${TINKERCLOUD_NPM_SDK_WORKFLOW_FILE:-"$root/.github/workflows/npm-sdk-publish.yml"}
test -f "$workflow" || { echo "npm SDK workflow unavailable" >&2; exit 1; }

require() {
  pattern=$1 message=$2
  grep -F -- "$pattern" "$workflow" >/dev/null || { echo "$message" >&2; exit 1; }
}
reject() {
  pattern=$1 message=$2
  if grep -E -- "$pattern" "$workflow" >/dev/null; then echo "$message" >&2; exit 1; fi
}

require "workflow_dispatch:" "npm SDK workflow must be manually dispatched"
reject '^  (push|pull_request|pull_request_target|schedule|repository_dispatch|workflow_call):' \
  "npm SDK workflow has an unauthorized trigger"
require "cancel-in-progress: false" "npm SDK workflow must not cancel active publication"
reject 'secrets:[[:space:]]+inherit' "npm SDK workflow may not inherit ambient secrets"
reject 'git[[:space:]]+(tag[[:space:]]+-f|push[[:space:]].*--force)' \
  "npm SDK workflow may not move source tags"
for ref in $(sed -n 's/.*uses: [^@]*@\([^ #]*\).*/\1/p' "$workflow"); do
  printf '%s\n' "$ref" | grep -E '^[0-9a-f]{40}$' >/dev/null || {
    echo "npm SDK action is not pinned to a full commit SHA: $ref" >&2
    exit 1
  }
done

reject 'contents:[[:space:]]+write' "npm SDK workflow does not need GitHub contents write"
test "$(grep -c -F -- 'id-token: write' "$workflow")" = 1 || {
  echo "npm SDK workflow must grant OIDC only to its publication job" >&2
  exit 1
}
require "environment: npm-sdk" "npm SDK Environment gate missing"
require 'publish-npm-sdk-beta-$tag' "exact npm SDK beta confirmation missing"
require 'TINKERCLOUD_REQUIRED_RELEASE_CHANNEL=beta' "beta-key policy gate missing"
reject 'TINKERCLOUD_REQUIRED_RELEASE_CHANNEL=production' \
  "npm SDK beta workflow may not claim production authority"
require 'git merge-base --is-ancestor "$source_commit" origin/main' \
  "npm SDK main ancestry gate missing"
require 'false\ttrue\t' "published GitHub prerelease-state gate missing"
require 'package_status" == "200"' "first-package bootstrap denial missing"
require 'version_status" == "404"' "existing npm version denial missing"
require 'node-version: "24"' "trusted-publishing Node version missing"
test "$(grep -c -F -- 'npm install --global npm@11.5.1' "$workflow")" = 2 || {
  echo "pinned trusted-publishing npm CLI must be installed in both jobs" >&2
  exit 1
}
require 'gh release download "$tag"' "canonical beta release download missing"
require './scripts/release-verify.sh "$release_dir"' "complete signed release verification missing"
require 'node ./scripts/verify-sdk-publish-candidate.mjs "$VERSION" "$sdk_tarball"' \
  "exact SDK candidate verification missing"
require 'npm publish "$SDK_TARBALL" --access public --tag beta --ignore-scripts --provenance' \
  "fixed public npm beta publish command missing"
require 'version.dist?.attestations?.url' "published provenance verification missing"
require 'root["dist-tags"]?.beta' "published beta dist-tag verification missing"
require 'version.dist?.integrity !== process.env.SDK_INTEGRITY' \
  "published tarball identity verification missing"
require '"@tinkercloud/sdk@$VERSION"' "clean exact-version consumer install missing"

reject 'NODE_AUTH_TOKEN|NPM_TOKEN|secrets\.' \
  "npm SDK workflow must use OIDC without a registry token"
reject '(^|[[:space:]])(npm[[:space:]]+(pack|run[[:space:]]+build|test)|jsr[[:space:]]+publish|brew([[:space:]]|$))' \
  "npm SDK workflow may not rebuild or publish another channel"
reject 'npm[[:space:]]+publish[^\n]*(--tag[[:space:]]+(latest|next)|\$DIST_TAG)' \
  "npm SDK workflow may not publish a non-beta dist-tag"
reject 'npm[[:space:]]+(unpublish|deprecate|dist-tag[[:space:]]+(rm|remove))' \
  "npm SDK workflow may not conceal published state"
reject '@tinkercloud/cli|CLI_TARBALL|tinkercloud-linux' \
  "npm SDK workflow may not package the CLI or server"

echo "npm SDK beta workflow contract passed"
