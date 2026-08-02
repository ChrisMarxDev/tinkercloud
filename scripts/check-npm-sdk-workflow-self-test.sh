#!/bin/sh
# Prove the SDK npm workflow checker denies representative authority drift.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
checker="$root/scripts/check-npm-sdk-workflow.sh"
workflow="$root/.github/workflows/npm-sdk-publish.yml"
tmp=$(mktemp -d "${TMPDIR:-/tmp}/tinkercloud-npm-sdk-workflow-self-test.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

expect_denied() {
  label=$1 candidate=$2
  if TINKERCLOUD_NPM_SDK_WORKFLOW_FILE="$candidate" "$checker" >/dev/null 2>&1; then
    echo "npm SDK workflow checker accepted $label" >&2
    exit 1
  fi
}

cp "$workflow" "$tmp/trigger.yml"
printf '\n  push:\n' >>"$tmp/trigger.yml"
expect_denied "an automatic trigger" "$tmp/trigger.yml"

sed '/id-token: write/d' "$workflow" >"$tmp/no-oidc.yml"
expect_denied "publication without OIDC" "$tmp/no-oidc.yml"

sed 's/environment: npm-sdk/environment: unprotected/' "$workflow" >"$tmp/unprotected.yml"
expect_denied "an unprotected Environment" "$tmp/unprotected.yml"

sed 's/TINKERCLOUD_REQUIRED_RELEASE_CHANNEL=beta/TINKERCLOUD_REQUIRED_RELEASE_CHANNEL=production/g' \
  "$workflow" >"$tmp/production-key.yml"
expect_denied "production authority in the beta workflow" "$tmp/production-key.yml"

sed 's/false\\ttrue\\t/false\\tfalse\\t/g' "$workflow" >"$tmp/stable-release.yml"
expect_denied "a stable GitHub release" "$tmp/stable-release.yml"

sed 's/package_status" == "200"/package_status" != "500"/' "$workflow" >"$tmp/no-bootstrap.yml"
expect_denied "a missing first-package bootstrap gate" "$tmp/no-bootstrap.yml"

sed 's/--tag beta/--tag latest/' "$workflow" >"$tmp/latest.yml"
expect_denied "the latest dist-tag" "$tmp/latest.yml"

sed '/verify-sdk-publish-candidate\.mjs/d' "$workflow" >"$tmp/no-candidate-check.yml"
expect_denied "publication without exact candidate verification" "$tmp/no-candidate-check.yml"

cp "$workflow" "$tmp/token.yml"
printf '\n      # NODE_AUTH_TOKEN\n' >>"$tmp/token.yml"
expect_denied "a long-lived npm token" "$tmp/token.yml"

cp "$workflow" "$tmp/rebuild.yml"
printf '\n      - run: npm pack\n' >>"$tmp/rebuild.yml"
expect_denied "a second SDK build path" "$tmp/rebuild.yml"

sed 's/@tinkercloud\/sdk/@tinkercloud\/cli/g' "$workflow" >"$tmp/cli.yml"
expect_denied "CLI npm publication" "$tmp/cli.yml"

echo "npm SDK beta workflow checker self-test passed"
