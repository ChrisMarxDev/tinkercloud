#!/usr/bin/env node

import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, join, resolve } from "node:path";
import { pathToFileURL } from "node:url";

const [version, candidateInput] = process.argv.slice(2);
if (
  !/^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/.test(
    version ?? "",
  ) ||
  !candidateInput
) {
  console.error("usage: verify-sdk-publish-candidate.mjs VERSION TARBALL");
  process.exit(2);
}

const candidate = resolve(candidateInput);
assert.equal(
  basename(candidate),
  `tinkercloud-sdk-${version}.tgz`,
  "SDK candidate filename does not match the release version",
);

const expectedFiles = [
  "package/LICENSE",
  "package/README.md",
  "package/dist/index.d.ts",
  "package/dist/index.js",
  "package/package.json",
];
const listedFiles = execFileSync("tar", ["-tzf", candidate], {
  encoding: "utf8",
})
  .split(/\r?\n/)
  .filter(Boolean)
  .sort();
assert.deepEqual(
  listedFiles,
  expectedFiles,
  "SDK candidate does not contain the exact five-file payload",
);
assert.equal(
  new Set(listedFiles).size,
  listedFiles.length,
  "SDK candidate contains duplicate paths",
);

const manifestRaw = execFileSync(
  "tar",
  ["-xOzf", candidate, "package/package.json"],
  { encoding: "utf8", maxBuffer: 128 * 1024 },
);
assert.ok(manifestRaw.length <= 128 * 1024, "SDK manifest is unexpectedly large");
const manifest = JSON.parse(manifestRaw);

assert.equal(manifest.name, "@tinkercloud/sdk");
assert.equal(manifest.version, version);
assert.equal(manifest.license, "Apache-2.0");
assert.equal(manifest.type, "module");
assert.equal(manifest.types, "./dist/index.d.ts");
assert.equal(manifest.module, "./dist/index.js");
assert.equal(manifest.sideEffects, false);
assert.deepEqual(manifest.files, ["dist", "README.md", "LICENSE"]);
assert.deepEqual(manifest.exports, {
  ".": {
    types: "./dist/index.d.ts",
    import: "./dist/index.js",
    default: "./dist/index.js",
  },
});
assert.deepEqual(manifest.repository, {
  type: "git",
  url: "git+https://github.com/ChrisMarxDev/tinkercloud.git",
  directory: "sdk/typescript",
});
assert.deepEqual(manifest.publishConfig, {
  access: "public",
  provenance: true,
  registry: "https://registry.npmjs.org/",
});

for (const field of [
  "dependencies",
  "optionalDependencies",
  "peerDependencies",
  "bundledDependencies",
  "bundleDependencies",
  "bin",
]) {
  assert.equal(manifest[field], undefined, `SDK candidate contains ${field}`);
}
for (const hook of ["preinstall", "install", "postinstall"]) {
  assert.equal(
    manifest.scripts?.[hook],
    undefined,
    `SDK candidate contains the ${hook} lifecycle hook`,
  );
}

const work = mkdtempSync(join(tmpdir(), "tinkercloud-sdk-candidate-"));
try {
  execFileSync("tar", ["-xzf", candidate, "-C", work]);
  const runtime = await import(
    `${pathToFileURL(join(work, "package", "dist", "index.js")).href}?verify=${Date.now()}`
  );
  assert.equal(runtime.SDK_VERSION, version);
  assert.equal(typeof runtime.createTinker, "function");
  assert.equal(typeof runtime.tinker?.user?.current, "function");
  assert.match(
    readFileSync(join(work, "package", "dist", "index.d.ts"), "utf8"),
    /SDK_VERSION/,
  );
} finally {
  rmSync(work, { recursive: true, force: true });
}

console.log(`SDK npm candidate verified: @tinkercloud/sdk@${version}`);
