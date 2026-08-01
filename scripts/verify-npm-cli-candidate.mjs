#!/usr/bin/env node
import { access, readFile, readdir, stat } from "node:fs/promises";
import { constants } from "node:fs";
import { spawnSync } from "node:child_process";
import { join, resolve } from "node:path";
import process from "node:process";

const [directoryInput, version, tarballInput] = process.argv.slice(2);
if (!directoryInput || !version || !tarballInput) {
  console.error("usage: verify-npm-cli-candidate.mjs DIRECTORY VERSION TARBALL");
  process.exit(2);
}
if (!/^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/.test(version)) {
  throw new Error("invalid CLI package version");
}

const directory = resolve(directoryInput);
const tarball = resolve(tarballInput);
const manifest = JSON.parse(await readFile(join(directory, "package.json"), "utf8"));
const expectedKeys = [
  "bin", "bugs", "cpu", "description", "engines", "files", "homepage",
  "keywords", "license", "name", "os", "publishConfig", "repository",
  "sideEffects", "preferUnplugged", "type", "version",
].sort();
if (Object.keys(manifest).sort().join("\n") !== expectedKeys.join("\n")) {
  throw new Error("unexpected npm CLI manifest field");
}
if (
  manifest.name !== "@tinkercloud/cli" ||
  manifest.version !== version ||
  manifest.license !== "Apache-2.0" ||
  manifest.type !== "module" ||
  manifest.sideEffects !== false ||
  manifest.preferUnplugged !== true ||
  JSON.stringify(manifest.bin) !== JSON.stringify({ tinker: "./bin/tinker.js" }) ||
  JSON.stringify(manifest.os) !== JSON.stringify(["darwin", "linux"]) ||
  JSON.stringify(manifest.cpu) !== JSON.stringify(["x64", "arm64"]) ||
  manifest.repository?.url !== "git+https://github.com/ChrisMarxDev/tinkercloud.git" ||
  manifest.repository?.type !== "git" ||
  manifest.homepage !== "https://github.com/ChrisMarxDev/tinkercloud#readme" ||
  manifest.bugs?.url !== "https://github.com/ChrisMarxDev/tinkercloud/issues" ||
  manifest.publishConfig?.access !== "public" ||
  manifest.publishConfig?.registry !== "https://registry.npmjs.org/"
) {
  throw new Error("npm CLI identity or publication metadata drift");
}
for (const forbidden of ["dependencies", "devDependencies", "peerDependencies", "optionalDependencies", "scripts"]) {
  if (forbidden in manifest) throw new Error(`forbidden npm CLI field: ${forbidden}`);
}

const vendors = (await readdir(join(directory, "vendor"))).sort();
const expectedVendors = [
  "tinker-darwin-amd64", "tinker-darwin-arm64",
  "tinker-linux-amd64", "tinker-linux-arm64",
];
if (vendors.join("\n") !== expectedVendors.join("\n")) {
  throw new Error("incomplete npm CLI platform matrix");
}
for (const path of ["LICENSE", "README.md", "bin/tinker.js", ...expectedVendors.map((name) => `vendor/${name}`)]) {
  await access(join(directory, path), constants.R_OK);
}
if (((await stat(join(directory, "bin/tinker.js"))).mode & 0o111) === 0) {
  throw new Error("npm CLI launcher is not executable");
}
for (const name of expectedVendors) {
  if (((await stat(join(directory, "vendor", name))).mode & 0o111) === 0) {
    throw new Error(`npm CLI vendor binary is not executable: ${name}`);
  }
}

const listing = spawnSync("tar", ["-tzf", tarball], { encoding: "utf8" });
if (listing.status !== 0) throw new Error("npm CLI tarball is unreadable");
const actualEntries = listing.stdout.trim().split("\n").filter(Boolean).sort();
const expectedEntries = [
  "package/LICENSE", "package/README.md", "package/bin/tinker.js",
  "package/package.json", ...expectedVendors.map((name) => `package/vendor/${name}`),
].sort();
if (actualEntries.join("\n") !== expectedEntries.join("\n")) {
  throw new Error("npm CLI tarball contains unexpected files");
}
console.log("npm CLI candidate verified");
