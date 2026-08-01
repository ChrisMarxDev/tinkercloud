#!/bin/sh
# Bind the human release-channel classification to the committed public key.
# This check never reads private key material.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
policy="$root/packaging/release-key-policy.json"
public_key="$root/packaging/release-public-key.pem"
required=${TINKERCLOUD_REQUIRED_RELEASE_CHANNEL:-}

test -f "$policy" && test -f "$public_key" || {
  echo "release key policy input unavailable" >&2
  exit 1
}
case "$required" in
  ""|beta|production) ;;
  *) echo "invalid required release channel" >&2; exit 1 ;;
esac

fingerprint=$(openssl pkey -pubin -in "$public_key" -outform DER 2>/dev/null |
  sha256sum | awk '{print $1}')
python3 - "$policy" "$fingerprint" "$required" <<'PY'
import json, pathlib, re, sys

try:
    policy = json.loads(pathlib.Path(sys.argv[1]).read_bytes())
    if set(policy) != {"schema", "channel", "public_key_sha256"}:
        raise ValueError("invalid shape")
    if policy["schema"] != 1 or policy["channel"] not in {"beta", "production"}:
        raise ValueError("invalid policy")
    if not re.fullmatch(r"[0-9a-f]{64}", policy["public_key_sha256"]):
        raise ValueError("invalid fingerprint")
    if policy["public_key_sha256"] != sys.argv[2]:
        raise ValueError("public key fingerprint drift")
    if sys.argv[3] and policy["channel"] != sys.argv[3]:
        raise ValueError(f"release authority is {policy['channel']}, not {sys.argv[3]}")
except Exception as exc:
    raise SystemExit(f"release key policy rejected: {exc}")
PY
if test -n "$required"; then
  echo "release key policy passed ($required required)"
else
  echo "release key policy passed"
fi
