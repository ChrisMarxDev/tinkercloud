#!/bin/sh
# Build a V1 release without ever copying the signing key into the release.
# Required environment: TINKERCLOUD_RELEASE_SIGNING_KEY=/root/.../ed25519-private.pem
set -eu

usage() {
  echo "usage: TINKERCLOUD_RELEASE_SIGNING_KEY=FILE $0 VERSION OUTPUT_DIRECTORY" >&2
  exit 2
}

test $# -eq 2 || usage
version=$1
out=$2
case "$version" in *[!A-Za-z0-9._+-]*|'') usage;; esac
test -n "${TINKERCLOUD_RELEASE_SIGNING_KEY:-}" || usage
# Keep the explicit path in this shell only; Go and npm child builds do not
# inherit a signing-key environment variable.
signing_key=$TINKERCLOUD_RELEASE_SIGNING_KEY
unset TINKERCLOUD_RELEASE_SIGNING_KEY
test -f "$signing_key" || { echo "release signing key unavailable" >&2; exit 1; }

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
public_key=${TINKERCLOUD_RELEASE_PUBLIC_KEY:-"$root/packaging/release-public-key.pem"}
test -f "$public_key" || { echo "release public key unavailable" >&2; exit 1; }
command -v openssl >/dev/null || { echo "openssl is required" >&2; exit 1; }
command -v go >/dev/null || { echo "go is required" >&2; exit 1; }
command -v npm >/dev/null || { echo "npm is required" >&2; exit 1; }
command -v node >/dev/null || { echo "node is required" >&2; exit 1; }

sdk_version=$(cd "$root/sdk/typescript" && node -p "require('./package.json').version")
node -e 'if (!/^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/.test(process.argv[1])) process.exit(1)' "$version" || {
  echo "release version must be strict MAJOR.MINOR.PATCH" >&2
  exit 1
}
test "$version" = "$sdk_version" || {
  echo "release version $version does not match SDK version $sdk_version" >&2
  exit 1
}

# Ed25519 SubjectPublicKeyInfo ends in the raw 32-byte public key. Comparing
# canonical DER prevents a private key for a different release authority from
# producing artifacts that the shipped binary cannot verify.
work=$(mktemp -d "${TMPDIR:-/tmp}/tinkercloud-release.XXXXXX")
trap 'rm -rf "$work"' EXIT HUP INT TERM
openssl pkey -pubin -in "$public_key" -outform DER >"$work/public.der" 2>/dev/null || { echo "invalid release public key" >&2; exit 1; }
openssl pkey -in "$signing_key" -pubout -outform DER >"$work/private-public.der" 2>/dev/null || { echo "invalid release signing key" >&2; exit 1; }
cmp -s "$work/public.der" "$work/private-public.der" || { echo "release signing key does not match pinned public key" >&2; exit 1; }
key_b64=$(tail -c 32 "$work/public.der" | base64 | tr -d '\n')
test "${#key_b64}" -eq 44 || { echo "invalid Ed25519 release public key" >&2; exit 1; }

mkdir -p "$out"
test -d "$out" || { echo "output directory unavailable" >&2; exit 1; }
stage=$(mktemp -d "$out/.tinkercloud-release.XXXXXX")
cleanup_stage() { rm -rf "$stage"; }
trap 'cleanup_stage; rm -rf "$work"' EXIT HUP INT TERM

server_build_flags="-buildid= -s -w -X main.releasePublicKeyBase64=$key_b64 -X main.buildVersion=$version"
client_build_flags="-buildid= -s -w -X github.com/ChrisMarxDev/tinkercloud/internal/client.BuildVersion=$version"
(
  cd "$root"
  # -trimpath, disabled VCS stamping, and an empty build ID make identical
  # source/toolchain/input builds byte-stable. SOURCE_DATE_EPOCH is recorded
  # below if supplied by the release environment.
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags "$server_build_flags" -o "$stage/tinkercloud-linux-amd64" ./cmd/tinkercloud
  CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags "$client_build_flags" -o "$stage/tinker-linux-amd64" ./cmd/tinker
  CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -buildvcs=false -ldflags "$client_build_flags" -o "$stage/tinker-linux-arm64" ./cmd/tinker
  CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build -trimpath -buildvcs=false -ldflags "$client_build_flags" -o "$stage/tinker-darwin-amd64" ./cmd/tinker
  CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build -trimpath -buildvcs=false -ldflags "$client_build_flags" -o "$stage/tinker-darwin-arm64" ./cmd/tinker
)
chmod 0755 "$stage/tinkercloud-linux-amd64" "$stage"/tinker-linux-* "$stage"/tinker-darwin-*
cp "$root/packaging/systemd/tinkercloud.service" "$stage/tinkercloud.service"
# A released installer has no safe need to accept a caller-controlled origin.
# Bake the exact immutable release directory into both signed installers before
# their metadata and signatures are generated. Source placeholders exist only
# for repository development/tests and must never enter a published release.
release_base=${TINKERCLOUD_HOST_INSTALL_RELEASE_BASE:-"https://github.com/ChrisMarxDev/tinkercloud/releases/download/v$version/"}
if ! node - "$release_base" <<'NODE'
const raw = process.argv[2];
try {
  const url = new URL(raw);
  if (url.protocol !== "https:" || !url.hostname || url.username || url.password ||
      url.search || url.hash || (url.port && url.port !== "443") ||
      !raw.endsWith("/") || /[\x00-\x20\x7f]/.test(raw) || url.href !== raw) throw new Error();
} catch {
  process.exit(1);
}
NODE
then
  echo "host installer release base must be a credential-free HTTPS directory" >&2
  exit 1
fi
replace_installer_placeholder() {
  source=$1 destination=$2 placeholder=$3 label=$4
  if ! node - "$source" "$destination" "$placeholder" "$release_base" <<'NODE'
const fs = require("node:fs");
const [sourcePath, destinationPath, placeholder, releaseBase] = process.argv.slice(2);
const source = fs.readFileSync(sourcePath, "utf8");
const count = source.split(placeholder).length - 1;
if (count !== 1) process.exit(1);
fs.writeFileSync(destinationPath, source.replace(placeholder, releaseBase), { mode: 0o755 });
NODE
  then
    echo "release $label installer placeholder replacement failed" >&2
    exit 1
  fi
  staged_placeholder_count=$(grep -F -c -- "$placeholder" "$destination" || true)
  test "$staged_placeholder_count" = 0 || { echo "release $label installer still contains the development origin placeholder" >&2; exit 1; }
  baked_base_count=$(grep -F -c -- "$release_base" "$destination" || true)
  test "$baked_base_count" = 1 || { echo "release $label installer does not contain its exact immutable release directory" >&2; exit 1; }
}
replace_installer_placeholder "$root/packaging/install-host.sh" "$stage/install-host.sh" '__TINKERCLOUD_RELEASE_BASE__' host
replace_installer_placeholder "$root/packaging/install-client.sh" "$stage/install-client.sh" '__TINKERCLOUD_CLIENT_RELEASE_BASE__' client
chmod 0644 "$stage/tinkercloud.service"
chmod 0755 "$stage/install-host.sh"
chmod 0755 "$stage/install-client.sh"

# The SDK package inputs are copied into the task-local release workspace
# before npm runs. This keeps npm's install/build/pack side effects out of the
# tracked checkout even when other SDK jobs share it. Generated source outputs
# are excluded so the package is built only from its versioned inputs.
# A task-local cache also keeps release tooling from mutating an operator's npm
# cache. npm pack creates only a tarball; it does not publish anything.
npm_cache="$work/npm-cache"
sdk_workspace="$work/sdk-typescript"
mkdir -p "$sdk_workspace"
(
  cd "$root/sdk/typescript"
  tar --exclude='./node_modules' --exclude='./dist' --exclude='./*.tgz' -cf - .
) | (
  cd "$sdk_workspace"
  tar -xf -
)
sdk_tarball=$(
  cd "$sdk_workspace"
  npm_config_cache="$npm_cache" npm ci --ignore-scripts >/dev/null
  npm_config_cache="$npm_cache" npm run build >/dev/null
  npm_config_cache="$npm_cache" npm pack --silent --pack-destination "$sdk_workspace"
)
test -f "$sdk_workspace/$sdk_tarball" || { echo "SDK package build failed" >&2; exit 1; }
mv "$sdk_workspace/$sdk_tarball" "$stage/tinkercloud-sdk-$version.tgz"

sha256() { sha256sum "$1" | awk '{print $1}'; }
sign_artifact() {
  artifact=$1
  digest=$(sha256 "$stage/$artifact")
  metadata="$stage/$artifact.metadata.json"
  signature="$stage/$artifact.signature"
  # This byte-for-byte JSON form is the update artifact contract. Its signed
  # payload is exactly VERSION\\nAPI\\nSCHEMA\\nSHA256 (see internal/update).
  printf '{"version":"%s","api":"1","schema":"1","sha256":"%s"}\n' "$version" "$digest" >"$metadata"
  printf '%s\n1\n1\n%s' "$version" "$digest" >"$work/signed"
  openssl pkeyutl -sign -inkey "$signing_key" -rawin -in "$work/signed" -out "$work/signature" >/dev/null 2>&1
  test "$(wc -c <"$work/signature" | tr -d ' ')" = 64 || { echo "unexpected signature length" >&2; exit 1; }
  base64 <"$work/signature" | tr -d '\n' >"$signature"
  printf '\n' >>"$signature"
}
sign_artifact tinkercloud-linux-amd64
sign_artifact tinker-linux-amd64
sign_artifact tinker-linux-arm64
sign_artifact tinker-darwin-amd64
sign_artifact tinker-darwin-arm64
sign_artifact tinkercloud.service
sign_artifact install-host.sh
sign_artifact install-client.sh
sign_artifact "tinkercloud-sdk-$version.tgz"

# Dependency evidence is deliberately plain and reviewable, rather than a
# claimed vulnerability scan. It is enough to reproduce exactly what was built.
(
  cd "$root"
  {
    echo "Tinkercloud dependency evidence"
    echo "version=$version"
    echo "go=$(go version)"
    echo "source_date_epoch=${SOURCE_DATE_EPOCH:-unset}"
    echo "public_key_sha256=$(sha256sum "$public_key" | awk '{print $1}')"
    echo "host_installer_release_base=$release_base"
    echo "client_installer_release_base=$release_base"
    echo
    go list -m all
    echo
    echo "sdk_package_json_sha256=$(sha256sum sdk/typescript/package.json | awk '{print $1}')"
    echo "sdk_lockfile_sha256=$(if test -f sdk/typescript/package-lock.json; then sha256sum sdk/typescript/package-lock.json | awk '{print $1}'; else echo absent; fi)"
  } >"$stage/dependency-evidence.txt"
)
{
  echo "Tinkercloud reproducible build provenance"
  echo "version=$version"
  echo "server_goos=linux"
  echo "server_goarch=amd64"
  echo "client_platforms=linux/amd64 linux/arm64 darwin/amd64 darwin/arm64"
  echo "cgo_enabled=0"
  echo "go_build_flags=-trimpath -buildvcs=false"
  echo "go_ldflags=-buildid= -s -w"
  echo "source_date_epoch=${SOURCE_DATE_EPOCH:-unset}"
  echo "release_public_key_sha256=$(sha256sum "$public_key" | awk '{print $1}')"
  echo "host_installer_release_base=$release_base"
  echo "client_installer_release_base=$release_base"
  for artifact in tinkercloud-linux-amd64 tinker-linux-amd64 tinker-linux-arm64 tinker-darwin-amd64 tinker-darwin-arm64 tinkercloud.service install-host.sh install-client.sh "tinkercloud-sdk-$version.tgz"; do
    echo "artifact=$artifact sha256=$(sha256sum "$stage/$artifact" | awk '{print $1}')"
  done
} >"$stage/provenance.txt"

# Individual signatures authenticate payloads. This signed manifest additionally
# binds every payload sidecar and every review-evidence file, so replacing
# SHA256SUMS cannot conceal forged provenance or dependency evidence.
(
  cd "$stage"
  {
    printf '{"schema":"2","version":"%s","compatibility":{"server_version":"%s","control_api":{"version":"1","client":{"min_inclusive":"0.1.0","max_exclusive":"1.0.0"}},"app_api":{"version":"1","client":{"min_inclusive":"0.1.0","max_exclusive":"1.0.0"}},"schema_version":"1","release_manifest_schema":"2"},"files":{' "$version" "$version"
    first=1
    for file in $(find . -maxdepth 1 -type f ! -name release-manifest.json -print | sed 's#^./##' | LC_ALL=C sort); do
      test "$first" = 1 || printf ','
      first=0
      printf '"%s":"%s"' "$file" "$(sha256sum "$file" | awk '{print $1}')"
    done
    printf '}}\n'
  } >release-manifest.json
)
sign_artifact release-manifest.json

(
  cd "$stage"
  LC_ALL=C sha256sum * | LC_ALL=C sort >SHA256SUMS
)

# Do not overwrite a previously generated candidate accidentally. A release is
# immutable once its named artifacts have been published to this directory.
for f in "$stage"/*; do
  target="$out/$(basename "$f")"
  test ! -e "$target" || { echo "refusing to overwrite existing release artifact: $(basename "$f")" >&2; exit 1; }
done
for f in "$stage"/*; do mv "$f" "$out/"; done
rmdir "$stage"
trap 'rm -rf "$work"' EXIT HUP INT TERM
echo "release artifacts written to $out"
