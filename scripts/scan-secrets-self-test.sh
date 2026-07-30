#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/tinkercloud-secret-scan.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
mkdir -p "$tmp/allow" "$tmp/deny"

printf '%s\n' '-----BEGIN PUBLIC KEY-----' >"$tmp/allow/release-public-key.pem"
printf '%s\n' 'sk-test-placeholder-abcdefghijklmnopqrstuvwxyz' >"$tmp/allow/placeholder.txt"
"$root/scripts/scan-secrets.sh" "$tmp/allow/release-public-key.pem" "$tmp/allow/placeholder.txt" >/dev/null

private_prefix='-----BEGIN '
private_suffix='PRIVATE KEY-----'
printf '%s%s\n' "$private_prefix" "$private_suffix" >"$tmp/deny/private.pem"
if "$root/scripts/scan-secrets.sh" "$tmp/deny/private.pem" >/dev/null 2>&1; then
  echo "private key was accepted by secret scan" >&2
  exit 1
fi
printf '%s%s\n' 'ghp_' 'abcdefghijklmnopqrstuvwxyz0123456789AB' >"$tmp/deny/token.txt"
if "$root/scripts/scan-secrets.sh" "$tmp/deny/token.txt" >/dev/null 2>&1; then
  echo "live token was accepted by secret scan" >&2
  exit 1
fi

# A repository with no commits must still scan non-ignored files intended for
# its first commit. Ignored local credentials remain outside that source scan.
mkdir -p "$tmp/repo"
git -C "$tmp/repo" init -q
printf '%s\n' '.env' >"$tmp/repo/.gitignore"
printf '%s%s\n' "$private_prefix" "$private_suffix" >"$tmp/repo/untracked-private.pem"
printf '%s%s\n' 'ghp_' 'abcdefghijklmnopqrstuvwxyz0123456789AB' >"$tmp/repo/.env"
if TINKERCLOUD_SECRET_SCAN_ROOT="$tmp/repo" "$root/scripts/scan-secrets.sh" >/dev/null 2>&1; then
  echo "untracked first-commit secret was accepted by secret scan" >&2
  exit 1
fi
rm "$tmp/repo/untracked-private.pem"
TINKERCLOUD_SECRET_SCAN_ROOT="$tmp/repo" "$root/scripts/scan-secrets.sh" >/dev/null

echo "secret scan deny/allow/first-commit self-test passed"
