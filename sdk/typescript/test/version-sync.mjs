import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";

const root = new URL("../", import.meta.url);
const packageManifest = JSON.parse(
  await readFile(new URL("package.json", root), "utf8"),
);
const jsrManifest = JSON.parse(
  await readFile(new URL("jsr.json", root), "utf8"),
);
const source = await readFile(new URL("src/index.ts", root), "utf8");
const sourceVersion = source.match(
  /export const SDK_VERSION = "([^"]+)";/,
)?.[1];

assert.equal(packageManifest.name, "@tinyhost/sdk");
assert.equal(jsrManifest.name, packageManifest.name);
assert.equal(jsrManifest.version, packageManifest.version);
assert.equal(sourceVersion, packageManifest.version);
assert.equal(packageManifest.private, undefined);
assert.equal(packageManifest.sideEffects, false);
assert.equal(packageManifest.types, "./dist/index.d.ts");
assert.equal(packageManifest.module, "./dist/index.js");
assert.deepEqual(packageManifest.exports, {
  ".": {
    types: "./dist/index.d.ts",
    import: "./dist/index.js",
    default: "./dist/index.js",
  },
});
assert.deepEqual(packageManifest.publishConfig, {
  access: "public",
  provenance: true,
  registry: "https://registry.npmjs.org/",
});
assert.equal(jsrManifest.exports, "./src/index.ts");
assert.deepEqual(jsrManifest.publish, {
  include: ["src/**/*.ts", "README.md", "LICENSE"],
});

for (const script of ["preinstall", "install", "postinstall"]) {
  assert.equal(
    packageManifest.scripts?.[script],
    undefined,
    `consumer install lifecycle script is forbidden: ${script}`,
  );
}
assert.equal(packageManifest.dependencies, undefined);
assert.equal(packageManifest.peerDependencies, undefined);
