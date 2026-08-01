#!/bin/sh
# Prove release workflows can extract the SDK version without GNU/BSD sed differences.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
extractor="$root/scripts/extract-sdk-version.mjs"
tmp=$(mktemp -d "${TMPDIR:-/tmp}/tinkercloud-sdk-version-test.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

expect_denied() {
  label=$1
  candidate=$2
  if node "$extractor" "$candidate" >/dev/null 2>&1; then
    echo "SDK version extractor accepted $label" >&2
    exit 1
  fi
}

printf 'export const SDK_VERSION = "1.2.3";\n' >"$tmp/lf.ts"
test "$(node "$extractor" "$tmp/lf.ts")" = "1.2.3"

printf 'export const SDK_VERSION = "1.2.3";\r\n' >"$tmp/crlf.ts"
test "$(node "$extractor" "$tmp/crlf.ts")" = "1.2.3"

printf 'export const SDK_VERSION = "01.2.3";\n' >"$tmp/noncanonical.ts"
expect_denied "a noncanonical semantic version" "$tmp/noncanonical.ts"

printf 'export const SDK_VERSION = "1.2.3";\nexport const SDK_VERSION = "1.2.4";\n' >"$tmp/duplicate.ts"
expect_denied "duplicate declarations" "$tmp/duplicate.ts"

printf 'export const SDK_VERSION: string = "1.2.3";\n' >"$tmp/malformed.ts"
expect_denied "a declaration outside the release grammar" "$tmp/malformed.ts"

echo "SDK version extractor self-test passed"
