#!/bin/sh
# Install the unprivileged Tinker deployer client. The script embeds the public
# release key and never downloads or installs the privileged tinkercloud binary.
set -eu

fail() { echo "tinker client installer: $*" >&2; exit 1; }

test "$(id -u)" != 0 || fail "this is the deployer/workstation Tinker CLI installer; on a VPS root shell use the version-matched install-host.sh release installer"
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
artifact="tinker-$platform-$arch"

embedded_base='__TINKERCLOUD_CLIENT_RELEASE_BASE__'
development_placeholder_suffix='BASE__'
case "$embedded_base" in
"__TINKERCLOUD_CLIENT_RELEASE_$development_placeholder_suffix")
  # Repository copies are deliberately not installable from an unspecified
  # origin. Tests and reviewed custom distributions must name one explicitly.
  base=${TINKER_RELEASE_BASE:-}
  test -n "$base" || fail "TINKER_RELEASE_BASE must name an HTTPS release directory for this development installer"
  ;;
*)
  test -z "${TINKER_RELEASE_BASE:-}" || fail "released installers use their embedded immutable release directory; TINKER_RELEASE_BASE cannot override it"
  base=$embedded_base
  ;;
esac
base=$(python3 - "$base" <<'PY'
import ipaddress, re, sys
from urllib.parse import urlsplit

raw = sys.argv[1]
try:
    if not raw or any(ord(c) <= 0x20 or ord(c) == 0x7f for c in raw):
        raise ValueError()
    url = urlsplit(raw)
    host = url.hostname
    if (url.scheme != 'https' or not url.hostname or url.username or url.password
            or url.query or url.fragment or not raw.endswith('/')
            or url.port not in (None, 443) or '//' in url.path
            or any(part in ('.', '..') for part in url.path.split('/'))):
        raise ValueError()
    try:
        address = ipaddress.ip_address(host)
    except ValueError:
        # A DNS name must be canonical lowercase ASCII. Do not accept an
        # alternate spelling which could bypass origin review.
        if (host != host.encode('idna').decode('ascii') or
                not re.fullmatch(r'(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?', host)):
            raise ValueError()
    else:
        if not address.is_global:
            raise ValueError()
except (ValueError, UnicodeError):
    raise SystemExit(1)
print(raw)
PY
) || fail "release base must be a credential-free canonical HTTPS directory"

test -n "${HOME:-}" && test -d "$HOME" && test ! -L "$HOME" || fail "a real HOME directory is required"
uid=$(id -u)
owner_uid() {
  stat -f '%u' "$1" 2>/dev/null || stat -c '%u' "$1" 2>/dev/null
}
safe_owned_directory() {
  case "$1" in /*) ;; *) return 1 ;; esac
  candidate=$1
  while test "$candidate" != /; do
    # macOS commonly presents /var as a compatibility symlink to /private/var.
    # It is an OS-owned namespace root, not a caller-selected install ancestor.
    test "$candidate" = /var || test ! -L "$candidate" || return 1
    candidate=${candidate%/*}
    test -n "$candidate" || candidate=/
  done
  test -d "$1" && test -w "$1" && test "$(owner_uid "$1")" = "$uid"
}
path_has() { case ":${PATH:-}:" in *":$1:"*) return 0 ;; *) return 1 ;; esac; }

install_dir=
if test -n "${TINKER_INSTALL_DIR:-}"; then
  install_dir=$TINKER_INSTALL_DIR
  safe_owned_directory "$install_dir" || fail "TINKER_INSTALL_DIR must be an existing, non-symlink directory owned and writable by this account"
else
  # Do not turn an arbitrary user-writable PATH entry into an install target.
  # These are the conventional per-user locations plus opt-in system locations
  # that must already be owned by this account and present in PATH.
  for candidate in "$HOME/.local/bin" "$HOME/bin" /usr/local/bin /opt/homebrew/bin; do
    if path_has "$candidate" && safe_owned_directory "$candidate"; then
      install_dir=$candidate
      break
    fi
  done
  if test -z "$install_dir"; then
    install_dir="$HOME/.local/bin"
    test ! -L "$HOME/.local" && test ! -L "$install_dir" || fail "refusing symlinked local bin directory"
    mkdir -p "$install_dir"
    safe_owned_directory "$install_dir" || fail "local bin directory must be owned and writable by this account"
  fi
fi

ensure_profile_path() {
  path_has "$install_dir" && return 0
  case "${SHELL:-}" in
    */bash) profile="$HOME/.bash_profile" ;;
    */zsh) profile="$HOME/.zprofile" ;;
    */sh) profile="$HOME/.profile" ;;
    *) echo "installed tinker, but no supported shell profile was identified; add $install_dir to PATH" >&2; return 0 ;;
  esac
  if test -e "$profile" || test -L "$profile"; then
    test -f "$profile" && test ! -L "$profile" && test "$(owner_uid "$profile")" = "$uid" || fail "refusing unsafe shell profile path: $profile"
    mode=$(stat -f '%Lp' "$profile" 2>/dev/null || stat -c '%a' "$profile" 2>/dev/null) || fail "cannot inspect shell profile: $profile"
    case "$mode" in *[2367][0-7]|*[2367]) fail "refusing group/world-writable shell profile: $profile" ;; esac
  fi
  marker='# Added by Tinker client installer'
  grep -Fqx "$marker" "$profile" 2>/dev/null && return 0
  profile_tmp=$(mktemp "$HOME/.tinker-profile.XXXXXX") || fail "cannot create shell profile update"
  trap 'rm -f "$profile_tmp" "${target_tmp:-}"; rm -rf "$work"' EXIT HUP INT TERM
  if test -e "$profile"; then
    cat "$profile" >"$profile_tmp" || fail "cannot read shell profile: $profile"
    chmod "$mode" "$profile_tmp" || fail "cannot preserve shell profile mode"
  else
    chmod 0600 "$profile_tmp" || fail "cannot set shell profile mode"
  fi
  {
    printf '\n%s\n' "$marker"
    printf 'case ":$PATH:" in *":%s:"*) ;; *) export PATH="%s:$PATH" ;; esac\n' "$install_dir" "$install_dir"
  } >>"$profile_tmp" || fail "cannot prepare shell profile update"
  # rename replaces a raced symlink rather than following it; the old profile
  # remains untouched until the complete replacement is ready.
  mv -f "$profile_tmp" "$profile" || fail "cannot atomically update shell profile: $profile"
}

umask 077
work=$(mktemp -d "${TMPDIR:-/tmp}/tinker-client-install.XXXXXX")
trap 'rm -rf "$work"' EXIT HUP INT TERM
cat >"$work/release-public-key.pem" <<'EOF'
-----BEGIN PUBLIC KEY-----
MCowBQYDK2VwAyEAYhkssT8gJdyQLriNH5b4f+olvZ90xXbE2G6CrVVAX4g=
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
    sig_text = root.joinpath(artifact + '.signature').read_bytes()
    if len(sig_text) > 1024:
        raise ValueError('invalid signature')
    # Release-build emits a final LF. Trim only outer ASCII transport
    # whitespace before strict decoding so embedded junk remains invalid.
    sig = base64.b64decode(sig_text.strip(b' \t\r\n'), validate=True)
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
# verified candidate is never executed by this installer. Prepare the complete
# binary before touching the profile, so a local install failure cannot leave a
# new PATH entry pointing at no executable.
target_tmp="$install_dir/.tinker.new.$$"
trap 'rm -f "$target_tmp"; rm -rf "$work"' EXIT HUP INT TERM
install -m 0755 "$work/$artifact" "$target_tmp"
ensure_profile_path
mv -f "$target_tmp" "$install_dir/tinker"
echo "installed $artifact to $install_dir/tinker"
