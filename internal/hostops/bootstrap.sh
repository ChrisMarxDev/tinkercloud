#!/bin/sh
# Transported by `tinker host install`. It authenticates the full installer as a
# signed release artifact before executing it.
set -eu
fail() { echo "tinkercloud bootstrap: $*" >&2; exit 1; }
test "$(id -u)" = 0 || fail "run as root"
for command in curl openssl python3; do
  command -v "$command" >/dev/null 2>&1 || fail "$command is required"
done
test $# -eq 1 || fail "HTTPS release directory required"
base=$1
case "$base" in https://*) ;; *) fail "HTTPS release directory required" ;; esac
case "$base" in */) ;; *) base="$base/" ;; esac
umask 077
work=$(mktemp -d "${TMPDIR:-/tmp}/tinkercloud-bootstrap.XXXXXX")
trap 'rm -rf "$work"' EXIT HUP INT TERM
cat >"$work/key.pem" <<'EOF'
-----BEGIN PUBLIC KEY-----
MCowBQYDK2VwAyEAYhkssT8gJdyQLriNH5b4f+olvZ90xXbE2G6CrVVAX4g=
-----END PUBLIC KEY-----
EOF
for name in install-host.sh install-host.sh.metadata.json install-host.sh.signature; do
  curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' \
    --max-filesize 1048576 --output "$work/$name" "$base$name"
done
python3 - "$work" <<'PY'
import base64, hashlib, json, pathlib, re, sys
root = pathlib.Path(sys.argv[1])
meta = json.loads(root.joinpath("install-host.sh.metadata.json").read_bytes())
if set(meta) != {"version", "api", "schema", "sha256"} or meta["api"] != "1" or meta["schema"] != "1":
    raise SystemExit("tinkercloud bootstrap: invalid installer metadata")
if not re.fullmatch(r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)", meta["version"]):
    raise SystemExit("tinkercloud bootstrap: invalid installer version")
if hashlib.sha256(root.joinpath("install-host.sh").read_bytes()).hexdigest() != meta["sha256"]:
    raise SystemExit("tinkercloud bootstrap: installer digest mismatch")
signature_text = root.joinpath("install-host.sh.signature").read_bytes()
if len(signature_text) > 1024:
    raise SystemExit("tinkercloud bootstrap: invalid installer signature")
# Release sidecars are canonical Base64 with one trailing LF. Accept only
# surrounding ASCII transport whitespace; strict decoding still rejects any
# interior whitespace or non-Base64 bytes.
signature = base64.b64decode(signature_text.strip(b" \t\r\n"), validate=True)
if len(signature) != 64:
    raise SystemExit("tinkercloud bootstrap: invalid installer signature")
root.joinpath("signed").write_bytes(
    (meta["version"] + "\n" + meta["api"] + "\n" + meta["schema"] + "\n" + meta["sha256"]).encode()
)
root.joinpath("signature.raw").write_bytes(signature)
PY
openssl pkeyutl -verify -pubin -inkey "$work/key.pem" -rawin \
  -in "$work/signed" -sigfile "$work/signature.raw" >/dev/null 2>&1 ||
  fail "installer signature invalid"
exec sh "$work/install-host.sh"
