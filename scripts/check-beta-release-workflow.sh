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
reject 'npm[[:space:]]+(publish|unpublish)' 'signing workflow must not publish npm'
reject 'secrets:[[:space:]]+inherit' 'internal workflow must not inherit ambient secrets'
echo 'beta reusable workflow contract passed'
