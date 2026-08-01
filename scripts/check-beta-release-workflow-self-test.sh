#!/bin/sh
# Prove the beta workflow checker denies representative publication drift.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
source_workflow="$root/.github/workflows/beta-release.yml"
checker="$root/scripts/check-beta-release-workflow.sh"
tmp=$(mktemp -d "${TMPDIR:-/tmp}/tinkercloud-beta-workflow-self-test.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

expect_denied() {
  label=$1
  candidate=$2
  if TINKERCLOUD_BETA_WORKFLOW_FILE="$candidate" "$checker" >/dev/null 2>&1; then
    echo "beta workflow checker accepted $label" >&2
    exit 1
  fi
}

cp "$source_workflow" "$tmp/unauthorized-trigger.yml"
printf '\n  push:\n' >>"$tmp/unauthorized-trigger.yml"
expect_denied "an unauthorized push trigger" "$tmp/unauthorized-trigger.yml"

sed 's/environment: beta-release/environment: unprotected/' \
  "$source_workflow" >"$tmp/unprotected-environment.yml"
expect_denied "an unprotected signing environment" "$tmp/unprotected-environment.yml"

cp "$source_workflow" "$tmp/registry-publish.yml"
printf '\n      - run: npm publish\n' >>"$tmp/registry-publish.yml"
expect_denied "an npm publication command" "$tmp/registry-publish.yml"

cp "$source_workflow" "$tmp/asset-clobber.yml"
printf '\n      # --clobber\n' >>"$tmp/asset-clobber.yml"
expect_denied "asset clobbering" "$tmp/asset-clobber.yml"

cp "$source_workflow" "$tmp/package-write.yml"
printf '\n    permissions:\n      packages: write\n' >>"$tmp/package-write.yml"
expect_denied "unnecessary package write permission" "$tmp/package-write.yml"

sed 's/gh release download "$tag"/gh release fetch "$tag"/' \
  "$source_workflow" >"$tmp/no-remote-download.yml"
expect_denied "missing remote draft download" "$tmp/no-remote-download.yml"

sed "s/printf 'tinker host install root@HOST\\\\n'/printf 'tinker host install root@HOST --release-base %s\\\\n'/" \
  "$source_workflow" >"$tmp/no-version-derived-host-install.yml"
expect_denied "missing version-derived host installation notes" "$tmp/no-version-derived-host-install.yml"

sed 's#sdk_version=$(node ./scripts/extract-sdk-version.mjs sdk/typescript/src/index.ts)#sdk_version=0.0.0#' \
  "$source_workflow" >"$tmp/no-portable-version-extraction.yml"
expect_denied "missing portable exact SDK version extraction" "$tmp/no-portable-version-extraction.yml"

echo "beta release workflow checker self-test passed"
