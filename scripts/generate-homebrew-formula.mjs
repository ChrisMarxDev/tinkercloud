#!/usr/bin/env node
import { mkdir, readFile, writeFile } from "node:fs/promises";
import { createHash } from "node:crypto";
import { dirname, resolve } from "node:path";
import process from "node:process";

const [releaseInput, outputInput, className, commandName, version, releaseBase] = process.argv.slice(2);
if (!releaseInput || !outputInput || !className || !commandName || !version || !releaseBase) {
  console.error("usage: generate-homebrew-formula.mjs RELEASE_DIR OUTPUT_FILE CLASS COMMAND VERSION HTTPS_RELEASE_BASE");
  process.exit(2);
}
if (!/^[A-Z][A-Za-z0-9]*$/.test(className) || !/^[a-z][a-z0-9-]*$/.test(commandName)) {
  throw new Error("invalid formula identity");
}
if (!/^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)$/.test(version)) {
  throw new Error("invalid formula version");
}
const origin = new URL(releaseBase);
if (origin.protocol !== "https:" || origin.username || origin.password || origin.search || origin.hash) {
  throw new Error("formula release base must be a credential-free HTTPS directory");
}
const base = origin.toString().replace(/\/?$/, "/");
const releaseDir = resolve(releaseInput);
const checksums = new Map();
for (const line of (await readFile(resolve(releaseDir, "SHA256SUMS"), "ascii")).split("\n")) {
  const match = /^([0-9a-f]{64})  ([-A-Za-z0-9._+]+)$/.exec(line);
  if (match) checksums.set(match[2], match[1]);
}
for (const artifact of ["tinker-darwin-amd64", "tinker-darwin-arm64"]) {
  if (!checksums.has(artifact)) throw new Error(`missing checksum: ${artifact}`);
  const digest = createHash("sha256").update(await readFile(resolve(releaseDir, artifact))).digest("hex");
  if (digest !== checksums.get(artifact)) throw new Error(`release artifact drift: ${artifact}`);
}
const formula = `# Generated from a locally verified signed release. Do not edit hashes.
class ${className} < Formula
  desc "Tinker workstation CLI"
  homepage "${origin.origin}/"
  version "${version}"
  license "Apache-2.0"

  on_macos do
    if Hardware::CPU.arm?
      url "${base}tinker-darwin-arm64"
      sha256 "${checksums.get("tinker-darwin-arm64")}"
    else
      url "${base}tinker-darwin-amd64"
      sha256 "${checksums.get("tinker-darwin-amd64")}"
    end
  end

  def install
    bin.install Dir["tinker-darwin-*"].first => "${commandName}"
  end

  test do
    assert_match version.to_s, shell_output("#{bin}/${commandName} version")
  end
end
`;
const output = resolve(outputInput);
await mkdir(dirname(output), { recursive: true });
await writeFile(output, formula, { flag: "wx", mode: 0o644 });
