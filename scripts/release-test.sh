#!/bin/sh
# Negative release-security checks. A disposable Ed25519 key is generated only
# in a temporary directory; no development or production private key is used.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
version=$(node "$root/scripts/extract-sdk-version.mjs" "$root/sdk/typescript/src/index.ts")
case "$version" in
  ''|*[!0-9.]*|*.*.*.*|.*|*.) echo "invalid SDK semantic version" >&2; exit 1;;
esac
printf '%s' "$version" | grep -E '^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$' >/dev/null || { echo "invalid SDK semantic version" >&2; exit 1; }
tmp=$(mktemp -d "${TMPDIR:-/tmp}/tinkercloud-release-test.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
openssl genpkey -algorithm ED25519 -out "$tmp/private.pem" >/dev/null 2>&1
openssl pkey -in "$tmp/private.pem" -pubout -out "$tmp/public.pem" >/dev/null 2>&1

# A release label that disagrees with the package's semantic version must fail
# before the expensive platform build or any artifact creation.
if TINKERCLOUD_RELEASE_SIGNING_KEY="$tmp/private.pem" \
TINKERCLOUD_RELEASE_PUBLIC_KEY="$tmp/public.pem" \
"$root/scripts/release-build.sh" 9.9.9 "$tmp/wrong-version" >/dev/null 2>&1; then
  echo "release accepted an SDK version mismatch" >&2
  exit 1
fi

# A key that is not the repository's release authority must be rejected before
# compilation, so an accidental developer key cannot create a trusted-looking
# artifact.
if TINKERCLOUD_RELEASE_SIGNING_KEY="$tmp/private.pem" "$root/scripts/release-build.sh" "$version" "$tmp/wrong-key" >/dev/null 2>&1; then
  echo "mismatched release key accepted" >&2
  exit 1
fi

TINKERCLOUD_RELEASE_SIGNING_KEY="$tmp/private.pem" \
TINKERCLOUD_RELEASE_PUBLIC_KEY="$tmp/public.pem" \
SOURCE_DATE_EPOCH=0 "$root/scripts/release-build.sh" "$version" "$tmp/release" >/dev/null
TINKERCLOUD_RELEASE_PUBLIC_KEY="$tmp/public.pem" "$root/scripts/release-verify.sh" "$tmp/release" >/dev/null
release_base="https://github.com/ChrisMarxDev/tinkercloud/releases/download/v$version/"
test "$(grep -F -c -- '__TINKERCLOUD_RELEASE_BASE__' "$root/packaging/install-host.sh")" = 1 || {
  echo "source host installer placeholder count drifted" >&2
  exit 1
}
grep -F -- "$release_base" "$tmp/release/install-host.sh" >/dev/null || {
  echo "released host installer does not embed its exact versioned release base" >&2
  exit 1
}
if grep -F -- '__TINKERCLOUD_RELEASE_BASE__' "$tmp/release/install-host.sh" >/dev/null; then
  echo "released host installer shipped the development origin placeholder" >&2
  exit 1
fi
test "$(grep -F -c -- '__TINKERCLOUD_CLIENT_RELEASE_BASE__' "$root/packaging/install-client.sh")" = 1 || {
  echo "source client installer placeholder count drifted" >&2
  exit 1
}
grep -F -- "$release_base" "$tmp/release/install-client.sh" >/dev/null || {
  echo "released client installer does not embed its exact versioned release base" >&2
  exit 1
}
if grep -F -- '__TINKERCLOUD_CLIENT_RELEASE_BASE__' "$tmp/release/install-client.sh" >/dev/null; then
  echo "released client installer shipped the development origin placeholder" >&2
  exit 1
fi
if "$root/scripts/distribution-prepare.sh" "$version" "$tmp/release" "$tmp/wrong-package" \
  "@wrong/cli" Tinker tinker \
  "$release_base" >/dev/null 2>&1; then
  echo "distribution accepted a noncanonical npm package" >&2
  exit 1
fi
if "$root/scripts/distribution-prepare.sh" "$version" "$tmp/release" "$tmp/wrong-formula" \
  "@tinkercloud/cli" Wrong tinker \
  "$release_base" >/dev/null 2>&1; then
  echo "distribution accepted a noncanonical Homebrew formula" >&2
  exit 1
fi
if "$root/scripts/distribution-prepare.sh" "$version" "$tmp/release" "$tmp/wrong-command" \
  "@tinkercloud/cli" Tinker wrong \
  "$release_base" >/dev/null 2>&1; then
  echo "distribution accepted a noncanonical CLI command" >&2
  exit 1
fi
if "$root/scripts/distribution-prepare.sh" "$version" "$tmp/release" "$tmp/wrong-origin" \
  "@tinkercloud/cli" Tinker tinker \
  "https://github.com/ChrisMarxDev/elsewhere/releases/download/v$version/" >/dev/null 2>&1; then
  echo "distribution accepted a noncanonical release origin" >&2
  exit 1
fi
TINKERCLOUD_RELEASE_PUBLIC_KEY="$tmp/public.pem" \
"$root/scripts/distribution-prepare.sh" "$version" "$tmp/release" "$tmp/distribution" \
  "@tinkercloud/cli" Tinker tinker \
  "$release_base" >/dev/null
VERSION="$version" node - "$tmp/distribution" <<'NODE'
const fs = require("node:fs");
const path = require("node:path");
const root = process.argv[2];
const manifest = JSON.parse(fs.readFileSync(path.join(root, "npm", "package.json"), "utf8"));
if (manifest.name !== "@tinkercloud/cli" || manifest.version !== process.env.VERSION || manifest.dependencies || manifest.scripts) {
  throw new Error("invalid npm CLI candidate");
}
const vendors = fs.readdirSync(path.join(root, "npm", "vendor")).sort();
if (vendors.join(" ") !== "tinker-darwin-amd64 tinker-darwin-arm64 tinker-linux-amd64 tinker-linux-arm64") {
  throw new Error("incomplete npm CLI platform matrix");
}
const formula = fs.readFileSync(path.join(root, "homebrew", "tinker.rb"), "utf8");
if (!formula.includes(`version "${process.env.VERSION}"`) || !formula.includes("tinker-darwin-amd64") || !formula.includes("tinker-darwin-arm64")) {
  throw new Error("invalid Homebrew formula candidate");
}
NODE

# Rebuild the executable inputs with the same pinned toolchain/source and
# assert the release promises are not merely documentation.
TINKERCLOUD_RELEASE_SIGNING_KEY="$tmp/private.pem" \
TINKERCLOUD_RELEASE_PUBLIC_KEY="$tmp/public.pem" \
SOURCE_DATE_EPOCH=0 "$root/scripts/release-build.sh" "$version" "$tmp/release-again" >/dev/null
for artifact in tinkercloud-linux-amd64 tinker-linux-amd64 tinker-linux-arm64 tinker-darwin-amd64 tinker-darwin-arm64; do
  cmp "$tmp/release/$artifact" "$tmp/release-again/$artifact" || { echo "build was not reproducible: $artifact" >&2; exit 1; }
done

custom_release_base="https://releases.example.test/tinkercloud/v$version/"
TINKERCLOUD_RELEASE_SIGNING_KEY="$tmp/private.pem" \
TINKERCLOUD_RELEASE_PUBLIC_KEY="$tmp/public.pem" \
TINKERCLOUD_HOST_INSTALL_RELEASE_BASE="$custom_release_base" \
SOURCE_DATE_EPOCH=0 "$root/scripts/release-build.sh" "$version" "$tmp/custom-release" >/dev/null
grep -F -- "$custom_release_base" "$tmp/custom-release/install-host.sh" >/dev/null || {
  echo "custom release build did not bind the host installer to its explicit release base" >&2
  exit 1
}
if TINKERCLOUD_RELEASE_SIGNING_KEY="$tmp/private.pem" \
TINKERCLOUD_RELEASE_PUBLIC_KEY="$tmp/public.pem" \
TINKERCLOUD_HOST_INSTALL_RELEASE_BASE=http://releases.example.test/ \
"$root/scripts/release-build.sh" "$version" "$tmp/insecure-custom-release" >/dev/null 2>&1; then
  echo "release build accepted an insecure custom host-installer release base" >&2
  exit 1
fi
if TINKERCLOUD_RELEASE_SIGNING_KEY="$tmp/private.pem" \
TINKERCLOUD_RELEASE_PUBLIC_KEY="$tmp/public.pem" \
TINKERCLOUD_HOST_INSTALL_RELEASE_BASE="https://releases.example.test/tinkercloud/../v$version/" \
"$root/scripts/release-build.sh" "$version" "$tmp/ambiguous-custom-release" >/dev/null 2>&1; then
  echo "release build accepted an ambiguous custom host-installer release base" >&2
  exit 1
fi
grep -F -- "host_installer_release_base=$custom_release_base" "$tmp/custom-release/provenance.txt" >/dev/null || {
  echo "custom host-installer release base is missing from provenance" >&2
  exit 1
}
grep -F -- "host_installer_release_base=$custom_release_base" "$tmp/custom-release/dependency-evidence.txt" >/dev/null || {
  echo "custom host-installer release base is missing from dependency evidence" >&2
  exit 1
}

# Assert the Linux server artifact contains the exact key that signed it.
key_b64=$(openssl pkey -pubin -in "$tmp/public.pem" -outform DER | tail -c 32 | base64 | tr -d '\n')
grep -aF "$key_b64" "$tmp/release/tinkercloud-linux-amd64" >/dev/null || { echo "server binary did not pin release key" >&2; exit 1; }

cp -R "$tmp/release" "$tmp/tampered"
printf x >>"$tmp/tampered/tinkercloud-linux-amd64"
if TINKERCLOUD_RELEASE_PUBLIC_KEY="$tmp/public.pem" "$root/scripts/release-verify.sh" "$tmp/tampered" >/dev/null 2>&1; then
  echo "tampered binary accepted" >&2
  exit 1
fi
cp -R "$tmp/release" "$tmp/metadata-tampered"
sed -i.bak 's/"api":"1"/"api":"2"/' "$tmp/metadata-tampered/tinkercloud-linux-amd64.metadata.json"
rm "$tmp/metadata-tampered/tinkercloud-linux-amd64.metadata.json.bak"
if TINKERCLOUD_RELEASE_PUBLIC_KEY="$tmp/public.pem" "$root/scripts/release-verify.sh" "$tmp/metadata-tampered" >/dev/null 2>&1; then
  echo "tampered metadata accepted" >&2
  exit 1
fi
cp -R "$tmp/release" "$tmp/signature-tampered"
printf A >>"$tmp/signature-tampered/tinkercloud-linux-amd64.signature"
if TINKERCLOUD_RELEASE_PUBLIC_KEY="$tmp/public.pem" "$root/scripts/release-verify.sh" "$tmp/signature-tampered" >/dev/null 2>&1; then
  echo "tampered signature accepted" >&2
  exit 1
fi
cp -R "$tmp/release" "$tmp/evidence-tampered"
printf 'forged evidence\n' >"$tmp/evidence-tampered/provenance.txt"
(cd "$tmp/evidence-tampered" && sha256sum * | LC_ALL=C sort >SHA256SUMS)
if TINKERCLOUD_RELEASE_PUBLIC_KEY="$tmp/public.pem" "$root/scripts/release-verify.sh" "$tmp/evidence-tampered" >/dev/null 2>&1; then
  echo "tampered provenance accepted" >&2
  exit 1
fi
cp -R "$tmp/release" "$tmp/missing-client"
rm "$tmp/missing-client/tinker-darwin-arm64" "$tmp/missing-client/tinker-darwin-arm64.metadata.json" "$tmp/missing-client/tinker-darwin-arm64.signature"
if TINKERCLOUD_RELEASE_PUBLIC_KEY="$tmp/public.pem" "$root/scripts/release-verify.sh" "$tmp/missing-client" >/dev/null 2>&1; then
  echo "incomplete client platform matrix accepted" >&2
  exit 1
fi
echo "release pipeline tamper tests passed"
