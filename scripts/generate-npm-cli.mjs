#!/usr/bin/env node
import { chmod, copyFile, mkdir, readFile, writeFile } from "node:fs/promises";
import { createHash } from "node:crypto";
import { basename, join, resolve } from "node:path";
import { fileURLToPath } from "node:url";
import process from "node:process";

const [releaseInput, outputInput, packageName, version] = process.argv.slice(2);
if (!releaseInput || !outputInput || !packageName || !version) {
  console.error("usage: generate-npm-cli.mjs RELEASE_DIR OUTPUT_DIR PACKAGE_NAME VERSION");
  process.exit(2);
}
if (!/^(@[a-z0-9][a-z0-9._-]*\/)?[a-z0-9][a-z0-9._-]*$/.test(packageName)) {
  throw new Error("invalid npm package name");
}
if (!/^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/.test(version)) {
  throw new Error("invalid package version");
}
const releaseDir = resolve(releaseInput);
const outputDir = resolve(outputInput);
if (releaseDir === outputDir || outputDir.startsWith(releaseDir + "/")) {
  throw new Error("output must not be inside the immutable release directory");
}
await mkdir(outputDir, { recursive: false, mode: 0o755 });
await mkdir(join(outputDir, "bin"), { mode: 0o755 });
await mkdir(join(outputDir, "vendor"), { mode: 0o755 });

const artifacts = [
  "tinker-linux-amd64",
  "tinker-linux-arm64",
  "tinker-darwin-amd64",
  "tinker-darwin-arm64",
];
for (const artifact of artifacts) {
  const source = join(releaseDir, artifact);
  const metadata = JSON.parse(await readFile(source + ".metadata.json", "utf8"));
  if (
    Object.keys(metadata).sort().join(",") !== "api,schema,sha256,version" ||
    metadata.version !== version ||
    metadata.api !== "1" ||
    metadata.schema !== "1"
  ) {
    throw new Error(`release metadata drift: ${artifact}`);
  }
  const bytes = await readFile(source);
  if (createHash("sha256").update(bytes).digest("hex") !== metadata.sha256) {
    throw new Error(`release artifact drift: ${artifact}`);
  }
  await copyFile(source, join(outputDir, "vendor", basename(artifact)));
  await chmod(join(outputDir, "vendor", artifact), 0o755);
}
const root = resolve(fileURLToPath(new URL("../", import.meta.url)));
await copyFile(join(root, "packaging", "npm", "bin", "tinker.js"), join(outputDir, "bin", "tinker.js"));
await copyFile(join(root, "packaging", "npm", "README.md"), join(outputDir, "README.md"));
await copyFile(join(root, "LICENSE"), join(outputDir, "LICENSE"));
await chmod(join(outputDir, "bin", "tinker.js"), 0o755);
const manifest = {
  name: packageName,
  version,
  description: "Native Tinker workstation CLI",
  license: "Apache-2.0",
  type: "module",
  bin: { tinker: "./bin/tinker.js" },
  files: ["bin/tinker.js", "vendor/tinker-*", "README.md", "LICENSE"],
  os: ["darwin", "linux"],
  cpu: ["x64", "arm64"],
  engines: { node: ">=18" },
  sideEffects: false,
  preferUnplugged: true,
  repository: {
    type: "git",
    url: "git+https://github.com/ChrisMarxDev/tinkercloud.git",
  },
  bugs: { url: "https://github.com/ChrisMarxDev/tinkercloud/issues" },
  homepage: "https://github.com/ChrisMarxDev/tinkercloud#readme",
  keywords: ["tinkercloud", "cli", "deployment", "self-hosted"],
  publishConfig: {
    access: "public",
    registry: "https://registry.npmjs.org/",
  },
};
await writeFile(join(outputDir, "package.json"), JSON.stringify(manifest, null, 2) + "\n", { mode: 0o644 });
