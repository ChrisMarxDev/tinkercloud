#!/bin/sh
# Fail when any installer/bootstrap trusts a release authority other than the
# committed public key. Private key material is never read by this check.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
public_key="$root/packaging/release-public-key.pem"
test -f "$public_key" || {
  echo "committed release public key is unavailable" >&2
  exit 1
}

tmp=$(mktemp -d "${TMPDIR:-/tmp}/tinyhost-release-key-drift.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

openssl pkey -pubin -in "$public_key" -outform DER \
  -out "$tmp/canonical.der" >/dev/null 2>&1 || {
  echo "committed release public key is invalid" >&2
  exit 1
}

extract_and_compare() {
  source=$1
  label=$2
  extracted="$tmp/$label.pem"
  der="$tmp/$label.der"

  test "$(grep -c -- '-----BEGIN PUBLIC KEY-----' "$source")" = 1 || {
    echo "release trust anchor count drift: $source" >&2
    exit 1
  }
  test "$(grep -c -- '-----END PUBLIC KEY-----' "$source")" = 1 || {
    echo "release trust anchor count drift: $source" >&2
    exit 1
  }
  sed -n '/-----BEGIN PUBLIC KEY-----/,/-----END PUBLIC KEY-----/p' \
    "$source" >"$extracted"
  openssl pkey -pubin -in "$extracted" -outform DER \
    -out "$der" >/dev/null 2>&1 || {
    echo "embedded release public key is invalid: $source" >&2
    exit 1
  }
  cmp -s "$tmp/canonical.der" "$der" || {
    echo "embedded release public key drift: $source" >&2
    exit 1
  }
}

extract_and_compare "$root/packaging/install-client.sh" client-installer
extract_and_compare "$root/packaging/install-host.sh" host-installer
extract_and_compare "$root/internal/hostops/bootstrap.sh" cli-host-bootstrap

grep -F 'unset TINYHOST_RELEASE_SIGNING_KEY' \
  "$root/scripts/release-build.sh" >/dev/null || {
  echo "release builder exposes the signing-key path to child builds" >&2
  exit 1
}

echo "release trust anchors match"
