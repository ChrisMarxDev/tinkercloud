#!/bin/sh
set -eu
# The candidate must never execute before verification. This installer pins the
# release public key shipped beside it and uses the host's OpenSSL verifier.
test "$(uname -s)" = Linux || { echo "Linux required" >&2; exit 1; }
test "$(uname -m)" = x86_64 || { echo "x86_64 required" >&2; exit 1; }
test "$(id -u)" = 0 || { echo "run as root" >&2; exit 1; }
test $# -eq 3 || { echo "usage: install.sh BINARY METADATA SIGNATURE" >&2; exit 2; }
binary=$1
metadata=$2
signature=$3
test -f "$binary" && test -f "$metadata" && test -f "$signature" || { echo "artifact files required" >&2; exit 1; }
key=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)/release-public-key.pem
test -f "$key" || { echo "trusted release key unavailable" >&2; exit 1; }
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT HUP INT TERM
python3 - "$metadata" "$binary" "$work/signed" <<'PY'
import hashlib, json, pathlib, sys
try:
    meta = pathlib.Path(sys.argv[1]).read_bytes()
    if len(meta) == 0 or len(meta) > 4096:
        raise ValueError()
    obj = json.loads(meta)
    if set(obj) != {"version", "api", "schema", "sha256"}:
        raise ValueError()
    if any(not isinstance(obj[k], str) or not obj[k] or len(obj[k]) > 128 for k in ("version", "api", "schema")):
        raise ValueError()
    digest = obj["sha256"]
    if len(digest) != 64 or digest.lower() != digest or any(c not in "0123456789abcdef" for c in digest):
        raise ValueError()
    if hashlib.sha256(pathlib.Path(sys.argv[2]).read_bytes()).hexdigest() != digest:
        raise ValueError()
    pathlib.Path(sys.argv[3]).write_bytes((obj["version"] + "\n" + obj["api"] + "\n" + obj["schema"] + "\n" + digest).encode("utf-8"))
except Exception:
    raise SystemExit("artifact metadata invalid")
PY
# BSD base64 accepts decoded data on stdin but not as a positional file.
base64 -d <"$signature" >"$work/signature" || { echo "artifact signature invalid" >&2; exit 1; }
test "$(wc -c <"$work/signature" | tr -d ' ')" = 64 || { echo "artifact signature invalid" >&2; exit 1; }
openssl pkeyutl -verify -pubin -inkey "$key" -rawin -in "$work/signed" -sigfile "$work/signature" >/dev/null 2>&1 || { echo "artifact signature invalid" >&2; exit 1; }
install -m 0755 "$1" /usr/local/bin/tinkercloud
base=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
install -m 0644 "$base/systemd/tinkercloud.service" /etc/systemd/system/tinkercloud.service
systemctl daemon-reload
echo "Run: tinkercloud init"
