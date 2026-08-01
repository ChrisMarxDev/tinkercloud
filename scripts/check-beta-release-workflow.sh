#!/bin/sh
# Static denial gates for the only pre-stable hosted-release mutation path.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
workflow=${TINKERCLOUD_BETA_WORKFLOW_FILE:-"$root/.github/workflows/beta-release.yml"}
test -f "$workflow" || {
  echo "beta release workflow is unavailable" >&2
  exit 1
}

require() {
  pattern=$1
  message=$2
  grep -F -- "$pattern" "$workflow" >/dev/null || {
    echo "$message" >&2
    exit 1
  }
}

reject() {
  pattern=$1
  message=$2
  if grep -E -- "$pattern" "$workflow" >/dev/null; then
    echo "$message" >&2
    exit 1
  fi
}

require "workflow_dispatch:" "beta release must be manually dispatched"
reject '^  (push|pull_request|pull_request_target|schedule|repository_dispatch|workflow_call):' \
  "beta release has an unauthorized trigger"
require "cancel-in-progress: false" "beta release must not cancel an active publication"
test "$(grep -c -F -- 'contents: write' "$workflow")" = 1 || {
  echo "beta release must grant contents write only to its publication job" >&2
  exit 1
}
reject '^[[:space:]]+(actions|checks|deployments|discussions|id-token|issues|packages|pages|pull-requests|security-events|statuses):[[:space:]]+write' \
  "beta release grants an unnecessary write permission"
require "environment: beta-release" "beta signing environment gate is missing"
require "TINKERCLOUD_REQUIRED_RELEASE_CHANNEL=beta" "beta-key policy gate is missing"
require "publish-beta-\$tag" "exact beta publication confirmation is missing"
require 'git merge-base --is-ancestor "$source_commit" origin/main' \
  "reviewed main ancestry gate is missing"
require 'gh release view "$tag"' "existing-release denial gate is missing"
require 'visibility" != "PUBLIC"' "public repository gate is missing"
require "TINKERCLOUD_BETA_RELEASE_SIGNING_KEY_B64" "beta signing secret is missing"
test "$(grep -c -F -- 'TINKERCLOUD_BETA_RELEASE_SIGNING_KEY_B64' "$workflow")" = 1 || {
  echo "beta signing secret has more than one workflow exposure" >&2
  exit 1
}
require 'mktemp "$RUNNER_TEMP/.tinkercloud-beta-release-key.XXXXXX"' \
  "random temporary beta-key path is missing"
require "unset SIGNING_KEY_B64" \
  "decoded beta-key environment cleanup is missing"
require './scripts/release-build.sh "$VERSION" "$release_dir"' \
  "canonical signed release builder is missing"
test "$(grep -c -F -- './scripts/release-verify.sh' "$workflow")" -ge 2 || {
  echo "local and remote release verification are both required" >&2
  exit 1
}
require "--draft" "draft-first release creation is missing"
require "--prerelease" "GitHub prerelease classification is missing"
require 'gh release download "$tag"' "remote draft download is missing"
require 'cmp "$RUNNER_TEMP/local-assets.txt" "$RUNNER_TEMP/remote-assets.txt"' \
  "remote draft asset-set comparison is missing"
require "--latest=false" "beta latest-channel denial is missing"
require 'TINKER_RELEASE_BASE="$release_base"' \
  "public installer smoke test is missing"
require '"$release_base/tinkercloud-sdk-$VERSION.tgz"' \
  "public SDK tarball smoke test is missing"

reject '(^|[[:space:]])(npm[[:space:]]+publish|jsr[[:space:]]+publish|brew([[:space:]]|$))' \
  "beta workflow contains a package-manager publication command"
reject '--clobber' "beta workflow may not overwrite release assets"
reject 'gh[[:space:]]+release[[:space:]]+(delete|upload)' \
  "beta workflow may not delete releases or mutate an existing asset set"
reject 'git[[:space:]]+(tag[[:space:]]+-f|push[[:space:]].*--force)' \
  "beta workflow may not move source tags"
reject 'secrets:[[:space:]]+inherit' "beta workflow may not inherit ambient secrets"

for ref in $(sed -n 's/.*uses: [^@]*@\([^ #]*\).*/\1/p' "$workflow"); do
  printf '%s\n' "$ref" | grep -E '^[0-9a-f]{40}$' >/dev/null || {
    echo "beta workflow action is not pinned to a full commit SHA: $ref" >&2
    exit 1
  }
done

echo "beta release workflow contract passed"
