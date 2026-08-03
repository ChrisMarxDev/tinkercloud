#!/bin/sh
# Proves release SDK packaging has no write-capable npm invocation in the
# tracked SDK directory. The fake npm keeps the release build real while
# making an accidental source-directory invocation deterministic.
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
version=$(node "$root/scripts/extract-sdk-version.mjs" "$root/sdk/typescript/src/index.ts")
tmp=$(mktemp -d "${TMPDIR:-/tmp}/tinkercloud-release-sdk-workspace.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

sdk_source=$(CDPATH= cd -- "$root/sdk/typescript" && pwd)
mkdir -p "$tmp/bin"
cat >"$tmp/bin/npm" <<'SH'
#!/bin/sh
set -eu

printf '%s\n' "$PWD" >>"$TINKERCLOUD_TEST_NPM_LOG"
if test "$PWD" = "$TINKERCLOUD_TEST_SDK_SOURCE"; then
  echo "release build invoked npm in tracked SDK source" >&2
  exit 91
fi

case "$1" in
  ci)
    ;;
  run)
    test "${2:-}" = build || { echo "unexpected npm run command" >&2; exit 92; }
    ;;
  pack)
    destination=$PWD
    shift
    while test $# -gt 0; do
      case "$1" in
        --pack-destination)
          shift
          test $# -gt 0 || { echo "missing npm pack destination" >&2; exit 93; }
          destination=$1
          ;;
      esac
      shift
    done
    case "$destination" in
      "$TINKERCLOUD_TEST_SDK_SOURCE"|"$TINKERCLOUD_TEST_SDK_SOURCE"/*)
        echo "release build wrote npm pack output to tracked SDK source" >&2
        exit 94
        ;;
    esac
    tarball="$destination/tinkercloud-sdk-$TINKERCLOUD_TEST_SDK_VERSION.tgz"
    fixture="$destination/.tinkercloud-sdk-package-fixture"
    mkdir -p "$fixture/package"
    printf '{"name":"@tinkercloud/sdk","version":"%s"}\n' "$TINKERCLOUD_TEST_SDK_VERSION" >"$fixture/package/package.json"
    tar -czf "$tarball" -C "$fixture" package
    rm -rf "$fixture"
    basename "$tarball"
    ;;
  *)
    echo "unexpected npm command: $1" >&2
    exit 95
    ;;
esac
SH
chmod 0755 "$tmp/bin/npm"

openssl genpkey -algorithm ED25519 -out "$tmp/private.pem" >/dev/null 2>&1
openssl pkey -in "$tmp/private.pem" -pubout -out "$tmp/public.pem" >/dev/null 2>&1

PATH="$tmp/bin:$PATH" \
TINKERCLOUD_TEST_NPM_LOG="$tmp/npm.log" \
TINKERCLOUD_TEST_SDK_SOURCE="$sdk_source" \
TINKERCLOUD_TEST_SDK_VERSION="$version" \
TINKERCLOUD_RELEASE_SIGNING_KEY="$tmp/private.pem" \
TINKERCLOUD_RELEASE_PUBLIC_KEY="$tmp/public.pem" \
SOURCE_DATE_EPOCH=0 \
"$root/scripts/release-build.sh" "$version" "$tmp/release" >/dev/null

test -f "$tmp/release/tinkercloud-sdk-$version.tgz" || {
  echo "release build did not produce the SDK package" >&2
  exit 1
}
if grep -F -x -- "$sdk_source" "$tmp/npm.log" >/dev/null; then
  echo "release build used npm in tracked SDK source" >&2
  exit 1
fi
TINKERCLOUD_RELEASE_PUBLIC_KEY="$tmp/public.pem" \
  "$root/scripts/release-verify.sh" "$tmp/release" >/dev/null
