#!/bin/sh
# Static deny gates for stable GitHub and npm CLI publication workflows.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
stable=${TINKERCLOUD_STABLE_WORKFLOW_FILE:-"$root/.github/workflows/stable-release.yml"}
npm=${TINKERCLOUD_NPM_WORKFLOW_FILE:-"$root/.github/workflows/npm-cli-publish.yml"}
test -f "$stable" && test -f "$npm" || {
  echo "stable distribution workflow unavailable" >&2
  exit 1
}

require() {
  file=$1 pattern=$2 message=$3
  grep -F -- "$pattern" "$file" >/dev/null || { echo "$message" >&2; exit 1; }
}
reject() {
  file=$1 pattern=$2 message=$3
  if grep -E -- "$pattern" "$file" >/dev/null; then echo "$message" >&2; exit 1; fi
}
manual_only() {
  file=$1 label=$2
  require "$file" "workflow_dispatch:" "$label must be manually dispatched"
  reject "$file" '^  (push|pull_request|pull_request_target|schedule|repository_dispatch|workflow_call):' \
    "$label has an unauthorized trigger"
  require "$file" "cancel-in-progress: false" "$label must not cancel active publication"
  reject "$file" 'secrets:[[:space:]]+inherit' "$label may not inherit ambient secrets"
  reject "$file" 'git[[:space:]]+(tag[[:space:]]+-f|push[[:space:]].*--force)' "$label may not move source tags"
  reject "$file" '--clobber|gh[[:space:]]+release[[:space:]]+(delete|upload)' "$label may not replace release state"
  for ref in $(sed -n 's/.*uses: [^@]*@\([^ #]*\).*/\1/p' "$file"); do
    printf '%s\n' "$ref" | grep -E '^[0-9a-f]{40}$' >/dev/null || {
      echo "$label action is not pinned to a full commit SHA: $ref" >&2
      exit 1
    }
  done
}

manual_only "$stable" "stable release"
test "$(grep -c -F -- 'contents: write' "$stable")" = 1 || {
  echo "stable release must grant contents write only to its publication job" >&2
  exit 1
}
reject "$stable" '^[[:space:]]+(actions|checks|deployments|discussions|id-token|issues|packages|pages|pull-requests|security-events|statuses):[[:space:]]+write' \
  "stable release grants an unnecessary write permission"
require "$stable" "environment: stable-release" "stable release Environment gate missing"
require "$stable" 'publish-stable-$tag' "exact stable confirmation missing"
require "$stable" 'TINKERCLOUD_REQUIRED_RELEASE_CHANNEL=production' "production-key policy gate missing"
require "$stable" 'git merge-base --is-ancestor "$source_commit" origin/main' "stable main ancestry gate missing"
require "$stable" 'gh release view "$tag"' "existing stable release denial missing"
require "$stable" 'visibility" != "PUBLIC"' "public repository gate missing"
require "$stable" 'TINKERCLOUD_RELEASE_SIGNING_KEY_B64' "stable signing secret missing"
test "$(grep -c -F -- 'TINKERCLOUD_RELEASE_SIGNING_KEY_B64' "$stable")" = 1 || {
  echo "stable signing secret has more than one exposure" >&2
  exit 1
}
require "$stable" 'mktemp "$RUNNER_TEMP/.tinkercloud-stable-release-key.XXXXXX"' "random stable-key path missing"
require "$stable" 'unset SIGNING_KEY_B64' "stable-key environment cleanup missing"
require "$stable" './scripts/release-build.sh "$VERSION" "$release_dir"' "canonical release builder missing"
test "$(grep -c -F -- './scripts/release-verify.sh' "$stable")" -ge 2 || {
  echo "stable local and remote verification are required" >&2
  exit 1
}
require "$stable" "--draft" "stable draft-first publication missing"
require "$stable" 'gh release download "$tag"' "stable remote draft download missing"
require "$stable" 'cmp "$RUNNER_TEMP/local-assets.txt" "$RUNNER_TEMP/remote-assets.txt"' "stable asset-set comparison missing"
require "$stable" "--latest" "stable latest promotion missing"
require "$stable" '"download/v$VERSION" "latest/download"' "exact and latest installer smoke tests missing"
reject "$stable" '(^|[[:space:]])(npm[[:space:]]+publish|jsr[[:space:]]+publish|brew([[:space:]]|$))' \
  "stable GitHub workflow may not publish a package-manager channel"
reject "$stable" '--prerelease([[:space:]]|$)' "stable release may not become a prerelease"

manual_only "$npm" "npm CLI publish"
reject "$npm" 'contents:[[:space:]]+write' "npm CLI workflow does not need GitHub contents write"
test "$(grep -c -F -- 'id-token: write' "$npm")" = 1 || {
  echo "npm CLI publish must grant OIDC only to its publication job" >&2
  exit 1
}
require "$npm" "environment: npm-cli" "npm CLI Environment gate missing"
require "$npm" 'publish-npm-cli-$tag' "exact npm CLI confirmation missing"
require "$npm" 'TINKERCLOUD_REQUIRED_RELEASE_CHANNEL=production' "npm CLI production-key policy gate missing"
require "$npm" 'git merge-base --is-ancestor "$source_commit" origin/main' "npm CLI main ancestry gate missing"
require "$npm" 'false\tfalse\t' "stable GitHub release-state gate missing"
require "$npm" 'package_status" == "200"' "first-package bootstrap denial missing"
require "$npm" 'version_status" == "404"' "existing npm version denial missing"
require "$npm" 'node-version: "24"' "npm trusted publishing Node version missing"
test "$(grep -c -F -- 'npm install --global npm@11.5.1' "$npm")" = 2 || {
  echo "pinned trusted-publishing npm CLI must be installed in both npm jobs" >&2
  exit 1
}
require "$npm" 'gh release download "$tag"' "canonical release download missing"
require "$npm" './scripts/release-verify.sh "$release_dir"' "downloaded release verification missing"
require "$npm" './scripts/distribution-prepare.sh' "canonical npm candidate generator missing"
require "$npm" 'npm publish "$CLI_TARBALL" --access public --tag "$DIST_TAG"' "exact public npm publish command missing"
reject "$npm" 'NODE_AUTH_TOKEN|NPM_TOKEN|secrets\.' "npm CLI workflow must use OIDC without a registry token"
reject "$npm" 'npm[[:space:]]+(unpublish|deprecate|dist-tag[[:space:]]+(rm|remove))' "npm CLI workflow may not conceal published state"

echo "stable distribution workflow contracts passed"
