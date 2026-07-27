#!/bin/sh
# Install the unprivileged Tiny deployer client. The script embeds the public
# release key and never downloads or installs the privileged tinyhost binary.
set -eu

fail() { echo "tiny client installer: $*" >&2; exit 1; }

test "$(id -u)" != 0 || fail "refusing to run as root"
command -v curl >/dev/null 2>&1 || fail "curl is required"
command -v openssl >/dev/null 2>&1 || fail "openssl is required"
command -v python3 >/dev/null 2>&1 || fail "python3 is required"
command -v install >/dev/null 2>&1 || fail "install is required"

case "$(uname -s)" in
  Linux) platform=linux ;;
  Darwin) platform=darwin ;;
  *) fail "unsupported operating system" ;;
esac
case "$(uname -m)" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) fail "unsupported architecture" ;;
esac
artifact="tiny-$platform-$arch"

base=${TINYHOST_CLIENT_RELEASE_BASE:-}
case "$base" in
  https://*) ;;
  *) fail "TINYHOST_CLIENT_RELEASE_BASE must be an HTTPS release directory" ;;
esac
case "$base" in */) ;; *) base="$base/" ;; esac

install_dir=${TINYHOST_CLIENT_INSTALL_DIR:-"${HOME:-}/.local/bin"}
test -n "$install_dir" || fail "client install directory unavailable"
mkdir -p "$install_dir"
test -d "$install_dir" || fail "client install directory unavailable"

umask 077
work=$(mktemp -d "${TMPDIR:-/tmp}/tiny-client-install.XXXXXX")
trap 'rm -rf "$work"' EXIT HUP INT TERM
cat >"$work/release-public-key.pem" <<'EOF'
-----BEGIN PUBLIC KEY-----
MCowBQYDK2VwAyEAQPDqQfUvtNnVIG+6Cr3kYXaBlDRLCSjDQorOLNX1dug=
-----END PUBLIC KEY-----
EOF

fetch() {
  name=$1
  curl --fail --silent --show-error --location --proto '=https' --proto-redir '=https' \
    --output "$work/$name" "$base$name"
}
fetch "$artifact"
fetch "$artifact.metadata.json"
fetch "$artifact.signature"
fetch SHA256SUMS

# The manifest is additional complete-release evidence; metadata/signature are
# the trust decision for this selected artifact. Both must agree before the
# candidate can reach the user's bin directory.
python3 - "$work" "$artifact" <<'PY'
import base64, hashlib, json, pathlib, re, sys

root, artifact = pathlib.Path(sys.argv[1]), sys.argv[2]
try:
    wanted = [artifact, artifact + '.metadata.json', artifact + '.signature']
    manifest = root.joinpath('SHA256SUMS').read_text(encoding='ascii').splitlines()
    entries = {}
    for line in manifest:
        match = re.fullmatch(r'([0-9a-f]{64})  ([-A-Za-z0-9._+]+)', line)
        if not match or match.group(2) in entries:
            raise ValueError('malformed checksum manifest')
        entries[match.group(2)] = match.group(1)
    if any(name not in entries for name in wanted):
        raise ValueError('checksum manifest omits selected artifact')
    for name in wanted:
        if hashlib.sha256(root.joinpath(name).read_bytes()).hexdigest() != entries[name]:
            raise ValueError('checksum mismatch')
    raw = root.joinpath(artifact + '.metadata.json').read_bytes()
    if not raw or len(raw) > 4096:
        raise ValueError('invalid metadata size')
    meta = json.loads(raw)
    if set(meta) != {'version', 'api', 'schema', 'sha256'}:
        raise ValueError('invalid metadata shape')
    if any(not isinstance(meta[k], str) or not meta[k] or len(meta[k]) > 128 for k in ('version', 'api', 'schema')):
        raise ValueError('invalid metadata value')
    if not re.fullmatch(r'[0-9a-f]{64}', meta['sha256']):
        raise ValueError('invalid metadata digest')
    if hashlib.sha256(root.joinpath(artifact).read_bytes()).hexdigest() != meta['sha256']:
        raise ValueError('metadata digest mismatch')
    sig = base64.b64decode(root.joinpath(artifact + '.signature').read_bytes(), validate=True)
    if len(sig) != 64:
        raise ValueError('invalid signature')
    root.joinpath('signed').write_bytes(('\n'.join((meta['version'], meta['api'], meta['schema'], meta['sha256']))).encode())
    root.joinpath('signature.raw').write_bytes(sig)
except Exception as exc:
    raise SystemExit('release verification failed: ' + str(exc))
PY
openssl pkeyutl -verify -pubin -inkey "$work/release-public-key.pem" -rawin \
  -in "$work/signed" -sigfile "$work/signature.raw" >/dev/null 2>&1 || fail "release signature invalid"

# Stage in the destination directory so the final replacement is atomic. A
# verified candidate is never executed by this installer.
target_tmp="$install_dir/.tiny.new.$$"
trap 'rm -f "$target_tmp"; rm -rf "$work"' EXIT HUP INT TERM
install -m 0755 "$work/$artifact" "$target_tmp"
mv -f "$target_tmp" "$install_dir/tiny"
echo "installed $artifact to $install_dir/tiny"
