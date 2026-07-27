#!/bin/sh
# Verify a release directory before installation or publication. It never
# executes an artifact and accepts only the pinned/publicly supplied key.
set -eu

test $# -eq 1 || { echo "usage: $0 RELEASE_DIRECTORY" >&2; exit 2; }
dir=$1
root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
public_key=${TINYHOST_RELEASE_PUBLIC_KEY:-"$root/packaging/release-public-key.pem"}
test -d "$dir" && test -f "$public_key" || { echo "release input unavailable" >&2; exit 1; }
command -v openssl >/dev/null || { echo "openssl is required" >&2; exit 1; }

work=$(mktemp -d "${TMPDIR:-/tmp}/tinyhost-release-verify.XXXXXX")
trap 'rm -rf "$work"' EXIT HUP INT TERM
verify_one() {
  artifact=$1
  metadata="$dir/$artifact.metadata.json"
  signature="$dir/$artifact.signature"
  test -f "$dir/$artifact" && test -f "$metadata" && test -f "$signature" || return 1
  python3 - "$metadata" "$dir/$artifact" "$work/signed" <<'PY'
import hashlib, json, pathlib, re, sys
try:
    raw = pathlib.Path(sys.argv[1]).read_bytes()
    if len(raw) == 0 or len(raw) > 4096:
        raise ValueError()
    v = json.loads(raw)
    if set(v) != {"version", "api", "schema", "sha256"}:
        raise ValueError()
    if not all(isinstance(v[x], str) and v[x] and len(v[x]) <= 128 for x in ("version", "api", "schema")):
        raise ValueError()
    if not re.fullmatch(r"[0-9a-f]{64}", v["sha256"]):
        raise ValueError()
    if hashlib.sha256(pathlib.Path(sys.argv[2]).read_bytes()).hexdigest() != v["sha256"]:
        raise ValueError()
    pathlib.Path(sys.argv[3]).write_bytes((v["version"] + "\n" + v["api"] + "\n" + v["schema"] + "\n" + v["sha256"]).encode())
except Exception:
    raise SystemExit(1)
PY
  # BSD base64 accepts decoded data on stdin but not as a positional file.
  base64 -d <"$signature" >"$work/signature" 2>/dev/null || return 1
  test "$(wc -c <"$work/signature" | tr -d ' ')" = 64 || return 1
  openssl pkeyutl -verify -pubin -inkey "$public_key" -rawin -in "$work/signed" -sigfile "$work/signature" >/dev/null 2>&1
}

test -f "$dir/SHA256SUMS" || { echo "release checksum manifest unavailable" >&2; exit 1; }
(cd "$dir" && sha256sum -c SHA256SUMS >/dev/null)
python3 - "$dir" <<'PY'
import pathlib, re, sys

root = pathlib.Path(sys.argv[1])
artifacts = sorted(p.name[:-len('.metadata.json')] for p in root.glob('*.metadata.json'))
expected_clients = {
    'tiny-linux-amd64', 'tiny-linux-arm64',
    'tiny-darwin-amd64', 'tiny-darwin-arm64',
}
if 'tinyhost-linux-amd64' not in artifacts or not expected_clients.issubset(artifacts):
    raise SystemExit('missing supported server or client artifact')
if 'release-manifest.json' not in artifacts:
    raise SystemExit('missing signed release manifest')
if any(not re.fullmatch(r'release-manifest\.json|tinyhost-sdk-[A-Za-z0-9._+-]+\.tgz|tinyhost-linux-amd64|tiny-(?:linux|darwin)-(?:amd64|arm64)', a) for a in artifacts):
    raise SystemExit('unknown signed release artifact')
lines = root.joinpath('SHA256SUMS').read_text(encoding='ascii').splitlines()
listed = set()
for line in lines:
    match = re.fullmatch(r'[0-9a-f]{64}  ([-A-Za-z0-9._+]+)', line)
    if not match or match.group(1) == 'SHA256SUMS':
        raise SystemExit('noncanonical checksum manifest')
    listed.add(match.group(1))
actual = {p.name for p in root.iterdir() if p.is_file() and p.name != 'SHA256SUMS'}
if listed != actual:
    raise SystemExit('checksum manifest does not cover the complete release')
PY
for metadata in "$dir"/*.metadata.json; do
  artifact=$(basename "$metadata" .metadata.json)
  verify_one "$artifact" || { echo "release artifact verification failed: $artifact" >&2; exit 1; }
done
python3 - "$dir" <<'PY'
import hashlib, json, pathlib, re, sys

root = pathlib.Path(sys.argv[1])
try:
    manifest = json.loads(root.joinpath('release-manifest.json').read_bytes())
    if set(manifest) != {'schema', 'files'} or manifest['schema'] != '1' or not isinstance(manifest['files'], dict):
        raise ValueError('invalid signed release manifest')
    excluded = {'SHA256SUMS', 'release-manifest.json', 'release-manifest.json.metadata.json', 'release-manifest.json.signature'}
    actual = {p.name for p in root.iterdir() if p.is_file()} - excluded
    if actual != set(manifest['files']):
        raise ValueError('signed release manifest does not cover complete release evidence')
    for name, digest in manifest['files'].items():
        if not re.fullmatch(r'[-A-Za-z0-9._+]+', name) or not isinstance(digest, str) or not re.fullmatch(r'[0-9a-f]{64}', digest):
            raise ValueError('invalid signed release manifest entry')
        if hashlib.sha256(root.joinpath(name).read_bytes()).hexdigest() != digest:
            raise ValueError('signed release manifest digest mismatch')
except Exception as exc:
    raise SystemExit('release manifest verification failed: ' + str(exc))
PY
echo "release verified"
