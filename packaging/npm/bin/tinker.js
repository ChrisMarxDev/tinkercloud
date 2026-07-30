#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import { fileURLToPath } from "node:url";
import { dirname, join } from "node:path";

const platforms = {
  "darwin:x64": "tinker-darwin-amd64",
  "darwin:arm64": "tinker-darwin-arm64",
  "linux:x64": "tinker-linux-amd64",
  "linux:arm64": "tinker-linux-arm64",
};
const artifact = platforms[`${process.platform}:${process.arch}`];
if (!artifact) {
  console.error(`tinker: unsupported platform ${process.platform}/${process.arch}`);
  process.exit(1);
}
const root = dirname(dirname(fileURLToPath(import.meta.url)));
const result = spawnSync(join(root, "vendor", artifact), process.argv.slice(2), {
  stdio: "inherit",
});
if (result.error) {
  console.error("tinker: native executable could not be started");
  process.exit(1);
}
process.exit(result.status ?? 1);
