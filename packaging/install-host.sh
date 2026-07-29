#!/bin/sh
# Root-only first installation of the TinyHost server from one HTTPS release
# directory. Existing installations must use `tinyhost update`.
set -eu

fail() { echo "tinyhost installer: $*" >&2; exit 1; }
test "$(id -u)" = 0 || fail "run as root"
test "$(uname -s)" = Linux || fail "Linux required"
case "$(uname -m)" in x86_64|amd64) ;; *) fail "x86_64 required" ;; esac
test ! -e /usr/local/bin/tinyhost || fail "tinyhost already installed; use tinyhost update"
for command in curl openssl python3 install systemctl; do
  command -v "$command" >/dev/null 2>&1 || fail "$command is required"
done
test $# -eq 1 || fail "usage: install-host.sh HTTPS_RELEASE_DIRECTORY"
base=$1

python3 - "$base" <<'PY' || exit 1
import ipaddress, socket, sys, urllib.parse
try:
    url = urllib.parse.urlsplit(sys.argv[1])
    if url.scheme != "https" or not url.hostname or url.username or url.password or url.query or url.fragment:
        raise ValueError()
    if url.port not in (None, 443):
        raise ValueError()
    for value in socket.getaddrinfo(url.hostname, 443, type=socket.SOCK_STREAM):
        address = ipaddress.ip_address(value[4][0])
        if not address.is_global:
            raise ValueError()
except Exception:
    raise SystemExit("tinyhost installer: release directory must be public credential-free HTTPS")
PY
case "$base" in */) ;; *) base="$base/" ;; esac

umask 077
work=$(mktemp -d "${TMPDIR:-/tmp}/tinyhost-install.XXXXXX")
trap 'rm -rf "$work"' EXIT HUP INT TERM
cat >"$work/release-public-key.pem" <<'EOF'
-----BEGIN PUBLIC KEY-----
MCowBQYDK2VwAyEAQPDqQfUvtNnVIG+6Cr3kYXaBlDRLCSjDQorOLNX1dug=
-----END PUBLIC KEY-----
EOF
fetch() {
  curl --fail --silent --show-error --proto '=https' --proto-redir '=https' \
    --max-filesize 134217728 --output "$work/$1" "$base$1"
}
for name in tinyhost-linux-amd64 tinyhost-linux-amd64.metadata.json tinyhost-linux-amd64.signature tinyhost.service tinyhost.service.metadata.json tinyhost.service.signature SHA256SUMS; do
  fetch "$name"
done

python3 - "$work" <<'PY'
import base64, hashlib, json, pathlib, re, sys
root = pathlib.Path(sys.argv[1])
entries = {}
for line in root.joinpath("SHA256SUMS").read_text(encoding="ascii").splitlines():
    match = re.fullmatch(r"([0-9a-f]{64})  ([-A-Za-z0-9._+]+)", line)
    if not match or match.group(2) in entries:
        raise SystemExit("tinyhost installer: invalid checksum manifest")
    entries[match.group(2)] = match.group(1)
for artifact in ("tinyhost-linux-amd64", "tinyhost.service"):
    names = (artifact, artifact + ".metadata.json", artifact + ".signature")
    if any(name not in entries for name in names):
        raise SystemExit("tinyhost installer: incomplete release evidence")
    for name in names:
        if hashlib.sha256(root.joinpath(name).read_bytes()).hexdigest() != entries[name]:
            raise SystemExit("tinyhost installer: checksum mismatch")
    meta = json.loads(root.joinpath(artifact + ".metadata.json").read_bytes())
    if set(meta) != {"version", "api", "schema", "sha256"} or meta["api"] != "1" or meta["schema"] != "1":
        raise SystemExit("tinyhost installer: invalid metadata")
    if not re.fullmatch(r"(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)", meta["version"]):
        raise SystemExit("tinyhost installer: invalid version")
    if hashlib.sha256(root.joinpath(artifact).read_bytes()).hexdigest() != meta["sha256"]:
        raise SystemExit("tinyhost installer: artifact digest mismatch")
    signature = base64.b64decode(root.joinpath(artifact + ".signature").read_bytes(), validate=True)
    if len(signature) != 64:
        raise SystemExit("tinyhost installer: invalid signature")
    root.joinpath(artifact + ".signed").write_bytes(
        (meta["version"] + "\n" + meta["api"] + "\n" + meta["schema"] + "\n" + meta["sha256"]).encode()
    )
    root.joinpath(artifact + ".signature.raw").write_bytes(signature)
PY
for artifact in tinyhost-linux-amd64 tinyhost.service; do
  openssl pkeyutl -verify -pubin -inkey "$work/release-public-key.pem" -rawin \
    -in "$work/$artifact.signed" -sigfile "$work/$artifact.signature.raw" >/dev/null 2>&1 ||
    fail "release signature invalid"
done

install -m 0755 "$work/tinyhost-linux-amd64" /usr/local/bin/.tinyhost.new
install -m 0644 "$work/tinyhost.service" /etc/systemd/system/.tinyhost.service.new
mv /usr/local/bin/.tinyhost.new /usr/local/bin/tinyhost
mv /etc/systemd/system/.tinyhost.service.new /etc/systemd/system/tinyhost.service
systemctl daemon-reload
echo "tinyhost installed; run tinyhost init with the required operator configuration"
