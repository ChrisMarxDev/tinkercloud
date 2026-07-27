#!/bin/sh
# Negative release-security checks. A disposable Ed25519 key is generated only
# in a temporary directory; no development or production private key is used.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/tinyhost-release-test.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
openssl genpkey -algorithm ED25519 -out "$tmp/private.pem" >/dev/null 2>&1
openssl pkey -in "$tmp/private.pem" -pubout -out "$tmp/public.pem" >/dev/null 2>&1

# A release label that disagrees with the package's semantic version must fail
# before the expensive platform build or any artifact creation.
if TINYHOST_RELEASE_SIGNING_KEY="$tmp/private.pem" \
TINYHOST_RELEASE_PUBLIC_KEY="$tmp/public.pem" \
"$root/scripts/release-build.sh" 9.9.9 "$tmp/wrong-version" >/dev/null 2>&1; then
  echo "release accepted an SDK version mismatch" >&2
  exit 1
fi

# A key that is not the repository's release authority must be rejected before
# compilation, so an accidental developer key cannot create a trusted-looking
# artifact.
if TINYHOST_RELEASE_SIGNING_KEY="$tmp/private.pem" "$root/scripts/release-build.sh" 0.1.0 "$tmp/wrong-key" >/dev/null 2>&1; then
  echo "mismatched release key accepted" >&2
  exit 1
fi

TINYHOST_RELEASE_SIGNING_KEY="$tmp/private.pem" \
TINYHOST_RELEASE_PUBLIC_KEY="$tmp/public.pem" \
SOURCE_DATE_EPOCH=0 "$root/scripts/release-build.sh" 0.1.0 "$tmp/release" >/dev/null
TINYHOST_RELEASE_PUBLIC_KEY="$tmp/public.pem" "$root/scripts/release-verify.sh" "$tmp/release" >/dev/null

# Rebuild the executable inputs with the same pinned toolchain/source and
# assert the release promises are not merely documentation.
TINYHOST_RELEASE_SIGNING_KEY="$tmp/private.pem" \
TINYHOST_RELEASE_PUBLIC_KEY="$tmp/public.pem" \
SOURCE_DATE_EPOCH=0 "$root/scripts/release-build.sh" 0.1.0 "$tmp/release-again" >/dev/null
for artifact in tinyhost-linux-amd64 tiny-linux-amd64 tiny-linux-arm64 tiny-darwin-amd64 tiny-darwin-arm64; do
  cmp "$tmp/release/$artifact" "$tmp/release-again/$artifact" || { echo "build was not reproducible: $artifact" >&2; exit 1; }
done

# Assert the Linux server artifact contains the exact key that signed it.
key_b64=$(openssl pkey -pubin -in "$tmp/public.pem" -outform DER | tail -c 32 | base64 | tr -d '\n')
grep -aF "$key_b64" "$tmp/release/tinyhost-linux-amd64" >/dev/null || { echo "server binary did not pin release key" >&2; exit 1; }

cp -R "$tmp/release" "$tmp/tampered"
printf x >>"$tmp/tampered/tinyhost-linux-amd64"
if TINYHOST_RELEASE_PUBLIC_KEY="$tmp/public.pem" "$root/scripts/release-verify.sh" "$tmp/tampered" >/dev/null 2>&1; then
  echo "tampered binary accepted" >&2
  exit 1
fi
cp -R "$tmp/release" "$tmp/metadata-tampered"
sed -i.bak 's/"api":"1"/"api":"2"/' "$tmp/metadata-tampered/tinyhost-linux-amd64.metadata.json"
rm "$tmp/metadata-tampered/tinyhost-linux-amd64.metadata.json.bak"
if TINYHOST_RELEASE_PUBLIC_KEY="$tmp/public.pem" "$root/scripts/release-verify.sh" "$tmp/metadata-tampered" >/dev/null 2>&1; then
  echo "tampered metadata accepted" >&2
  exit 1
fi
cp -R "$tmp/release" "$tmp/signature-tampered"
printf A >>"$tmp/signature-tampered/tinyhost-linux-amd64.signature"
if TINYHOST_RELEASE_PUBLIC_KEY="$tmp/public.pem" "$root/scripts/release-verify.sh" "$tmp/signature-tampered" >/dev/null 2>&1; then
  echo "tampered signature accepted" >&2
  exit 1
fi
cp -R "$tmp/release" "$tmp/evidence-tampered"
printf 'forged evidence\n' >"$tmp/evidence-tampered/provenance.txt"
(cd "$tmp/evidence-tampered" && sha256sum * | LC_ALL=C sort >SHA256SUMS)
if TINYHOST_RELEASE_PUBLIC_KEY="$tmp/public.pem" "$root/scripts/release-verify.sh" "$tmp/evidence-tampered" >/dev/null 2>&1; then
  echo "tampered provenance accepted" >&2
  exit 1
fi
cp -R "$tmp/release" "$tmp/missing-client"
rm "$tmp/missing-client/tiny-darwin-arm64" "$tmp/missing-client/tiny-darwin-arm64.metadata.json" "$tmp/missing-client/tiny-darwin-arm64.signature"
if TINYHOST_RELEASE_PUBLIC_KEY="$tmp/public.pem" "$root/scripts/release-verify.sh" "$tmp/missing-client" >/dev/null 2>&1; then
  echo "incomplete client platform matrix accepted" >&2
  exit 1
fi
echo "release pipeline tamper tests passed"
