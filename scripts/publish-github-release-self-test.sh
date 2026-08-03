#!/bin/sh
# Missing protected inputs must stop before the publisher can invoke gh.
set -eu
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/tinkercloud-publish-release-self-test.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
mkdir "$tmp/bin"
printf '#!/bin/sh\nprintf gh-called >&2\nexit 99\n' >"$tmp/bin/gh"
chmod 700 "$tmp/bin/gh"
if PATH="$tmp/bin:$PATH" RUNNER_TEMP="$tmp" VERSION=9.9.9 SOURCE_COMMIT=unused \
  "$root/scripts/publish-github-release.sh" beta >"$tmp/output" 2>&1; then
  echo 'publisher accepted missing signing secret' >&2
  exit 1
fi
grep -F 'protected release inputs are unavailable' "$tmp/output" >/dev/null || {
  echo 'publisher did not deny missing input before mutation' >&2
  exit 1
}
! grep -F 'gh-called' "$tmp/output" >/dev/null || {
  echo 'publisher invoked gh before denying missing input' >&2
  exit 1
}
echo 'protected publisher missing-input self-test passed'
