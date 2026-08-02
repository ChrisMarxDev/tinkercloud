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
reject 'NODE_AUTH_TOKEN|NPM_TOKEN|npm[[:space:]]+unpublish' 'SDK publisher may not use long-lived credentials or unpublish'
echo 'SDK reusable npm workflow contract passed'
