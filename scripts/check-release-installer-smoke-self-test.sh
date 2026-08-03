#!/bin/sh
# Prove the static release-smoke contract rejects the failure that left v0.1.4
# partially published: a caller-selected directory that was never created.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/tinkercloud-release-installer-smoke-test.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

sed '/^  mkdir -p "$install_dir"$/d' "$root/scripts/publish-github-release.sh" >"$tmp/publisher"
if TINKERCLOUD_RELEASE_PUBLISHER_FILE="$tmp/publisher" \
  "$root/scripts/check-release-installer-smoke.sh" >"$tmp/output" 2>&1; then
  echo 'release installer smoke contract accepted a missing install-directory creation' >&2
  exit 1
fi
grep -F 'must create TINKER_INSTALL_DIR before invoking the installer' "$tmp/output" >/dev/null || {
  echo 'release installer smoke contract did not explain the missing directory denial' >&2
  exit 1
}

echo 'release installer smoke directory self-test passed'
