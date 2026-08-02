#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
verifier="$root/scripts/verify-sdk-publish-candidate.mjs"
tmp=$(mktemp -d "${TMPDIR:-/tmp}/tinkercloud-sdk-candidate-self-test.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

make_candidate() {
  destination=$1 version=$2 mutation=${3:-none}
  fixture="$tmp/fixture"
  rm -rf "$fixture"
  mkdir -p "$fixture/package/dist" "$destination"
  printf 'Apache License 2.0\n' >"$fixture/package/LICENSE"
  printf '# Tinkercloud SDK\n' >"$fixture/package/README.md"
  printf 'export declare const SDK_VERSION = "%s";\n' "$version" >"$fixture/package/dist/index.d.ts"
  printf 'export const SDK_VERSION = "%s"; export const createTinker = () => ({}); export const tinker = { user: { current() {} } };\n' "$version" >"$fixture/package/dist/index.js"
  node - "$fixture/package/package.json" "$version" "$mutation" <<'NODE'
const fs = require("node:fs");
const [file, version, mutation] = process.argv.slice(2);
const manifest = {
  name: "@tinkercloud/sdk",
  version,
  license: "Apache-2.0",
  type: "module",
  types: "./dist/index.d.ts",
  module: "./dist/index.js",
  sideEffects: false,
  files: ["dist", "README.md", "LICENSE"],
  exports: { ".": { types: "./dist/index.d.ts", import: "./dist/index.js", default: "./dist/index.js" } },
  repository: { type: "git", url: "git+https://github.com/ChrisMarxDev/tinkercloud.git", directory: "sdk/typescript" },
  publishConfig: { access: "public", provenance: true, registry: "https://registry.npmjs.org/" },
};
if (mutation === "install-hook") manifest.scripts = { install: "echo unsafe" };
if (mutation === "dependency") manifest.dependencies = { unsafe: "1.0.0" };
fs.writeFileSync(file, JSON.stringify(manifest));
NODE
  if [ "$mutation" = extra-file ]; then printf 'extra\n' >"$fixture/package/extra.txt"; fi
  tar -czf "$destination/tinkercloud-sdk-$version.tgz" -C "$fixture" \
    package/LICENSE package/README.md package/dist/index.d.ts package/dist/index.js package/package.json \
    $(if [ "$mutation" = extra-file ]; then printf '%s' package/extra.txt; fi)
}

expect_denied() {
  label=$1 candidate=$2 version=${3:-1.2.3}
  if node "$verifier" "$version" "$candidate" >/dev/null 2>&1; then
    echo "SDK candidate verifier accepted $label" >&2
    exit 1
  fi
}

make_candidate "$tmp/good" 1.2.3
node "$verifier" 1.2.3 "$tmp/good/tinkercloud-sdk-1.2.3.tgz" >/dev/null

make_candidate "$tmp/hook" 1.2.3 install-hook
expect_denied "an install lifecycle hook" "$tmp/hook/tinkercloud-sdk-1.2.3.tgz"

make_candidate "$tmp/dependency" 1.2.3 dependency
expect_denied "a runtime dependency" "$tmp/dependency/tinkercloud-sdk-1.2.3.tgz"

make_candidate "$tmp/extra" 1.2.3 extra-file
expect_denied "an extra payload file" "$tmp/extra/tinkercloud-sdk-1.2.3.tgz"

expect_denied "a version mismatch" "$tmp/good/tinkercloud-sdk-1.2.3.tgz" 1.2.4

echo "SDK candidate verifier self-test passed"
