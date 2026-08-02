#!/bin/sh
# Prove the stable workflow checker denies representative authority drift.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
checker="$root/scripts/check-stable-release-workflows.sh"
stable="$root/.github/workflows/stable-release.yml"
npm="$root/.github/workflows/npm-cli-publish.yml"
tmp=$(mktemp -d "${TMPDIR:-/tmp}/tinkercloud-stable-workflow-self-test.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

expect_denied() {
  label=$1 stable_candidate=$2 npm_candidate=$3
  if TINKERCLOUD_STABLE_WORKFLOW_FILE="$stable_candidate" \
    TINKERCLOUD_NPM_WORKFLOW_FILE="$npm_candidate" \
    "$checker" >/dev/null 2>&1; then
    echo "stable workflow checker accepted $label" >&2
    exit 1
  fi
}

cp "$stable" "$tmp/trigger.yml"
printf '\n  push:\n' >>"$tmp/trigger.yml"
expect_denied "an automatic stable trigger" "$tmp/trigger.yml" "$npm"

sed 's/environment: stable-release/environment: unprotected/' "$stable" >"$tmp/unprotected-stable.yml"
expect_denied "an unprotected stable environment" "$tmp/unprotected-stable.yml" "$npm"

sed 's/TINKERCLOUD_REQUIRED_RELEASE_CHANNEL=production/TINKERCLOUD_REQUIRED_RELEASE_CHANNEL=beta/' \
  "$stable" >"$tmp/beta-key.yml"
expect_denied "the beta authority for stable" "$tmp/beta-key.yml" "$npm"

cp "$stable" "$tmp/stable-npm.yml"
printf '\n      - run: npm publish\n' >>"$tmp/stable-npm.yml"
expect_denied "npm publication in the GitHub release job" "$tmp/stable-npm.yml" "$npm"

sed '/id-token: write/d' "$npm" >"$tmp/no-oidc.yml"
expect_denied "npm publication without OIDC" "$stable" "$tmp/no-oidc.yml"

cp "$npm" "$tmp/token.yml"
printf '\n      # NODE_AUTH_TOKEN\n' >>"$tmp/token.yml"
expect_denied "a long-lived npm token" "$stable" "$tmp/token.yml"

sed 's/package_status" == "200"/package_status" != "500"/' "$npm" >"$tmp/no-bootstrap.yml"
expect_denied "a missing first-package bootstrap gate" "$stable" "$tmp/no-bootstrap.yml"

sed 's/--access public/--access restricted/' "$npm" >"$tmp/private-package.yml"
expect_denied "a private CLI package" "$stable" "$tmp/private-package.yml"

sed '/install-host\.sh | sh/d' "$stable" >"$tmp/no-root-shell-install.yml"
expect_denied "missing root-shell host installation notes" "$tmp/no-root-shell-install.yml" "$npm"

sed "s/--proto-redir '=https' //" "$stable" >"$tmp/root-shell-redirect-downgrade.yml"
expect_denied "a root-shell installer command without HTTPS-only redirects" "$tmp/root-shell-redirect-downgrade.yml" "$npm"

echo "stable distribution workflow checker self-test passed"
