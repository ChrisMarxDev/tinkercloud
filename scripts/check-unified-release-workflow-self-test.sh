#!/bin/sh
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
checker="$root/scripts/check-unified-release-workflow.sh"
source="$root/.github/workflows/release.yml"
tmp=$(mktemp -d "${TMPDIR:-/tmp}/tinkercloud-unified-release-self-test.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
deny() { if TINKERCLOUD_RELEASE_WORKFLOW_FILE="$1" "$checker" >/dev/null 2>&1; then echo "checker accepted $2" >&2; exit 1; fi; }
sed 's/dist_tag: next/dist_tag: latest/' "$source" >"$tmp/beta-cli-latest.yml"; deny "$tmp/beta-cli-latest.yml" 'beta CLI latest tag'
sed 's/needs: beta-release/needs: stable-release/' "$source" >"$tmp/wrong-beta-dependency.yml"; deny "$tmp/wrong-beta-dependency.yml" 'beta release dependency drift'
sed 's/npm package version already exists/npm version exists/' "$source" >"$tmp/no-npm-denial.yml"; deny "$tmp/no-npm-denial.yml" 'missing npm preflight denial'
sed 's/publish-\$CHANNEL-\$tag/publish-beta-\$tag/' "$source" >"$tmp/fixed-confirmation.yml"; deny "$tmp/fixed-confirmation.yml" 'fixed-channel confirmation'
sed '/SDK version declarations do not match \$VERSION/d' "$source" >"$tmp/no-version-parity.yml"; deny "$tmp/no-version-parity.yml" 'missing shared version parity'
sed '/\[\[ "$GITHUB_SHA" == "$source_commit" \]\]/d' "$source" >"$tmp/wrong-workflow-ref.yml"; deny "$tmp/wrong-workflow-ref.yml" 'missing exact workflow ref gate'
sed '/workflow_dispatch:/d' "$source" >"$tmp/no-dispatch.yml"; deny "$tmp/no-dispatch.yml" 'missing manual trigger'
printf '\n  push:\n' >>"$tmp/no-dispatch.yml"; deny "$tmp/no-dispatch.yml" 'automatic trigger'
sed '/beta-validation:/,/beta-release:/{s/contents: read/contents: write/;}' "$source" >"$tmp/beta-validation-write.yml"; deny "$tmp/beta-validation-write.yml" 'beta validation write authority'
sed '/stable-validation:/,/stable-release:/{s/contents: read/contents: write/;}' "$source" >"$tmp/stable-validation-write.yml"; deny "$tmp/stable-validation-write.yml" 'stable validation write authority'
echo 'unified release workflow checker self-test passed'
