import { execFileSync } from "node:child_process";
import { mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const cache = mkdtempSync(join(tmpdir(), "tinkercloud-sdk-pack-"));

try {
  const output = execFileSync("npm", ["pack", "--dry-run", "--json"], {
    encoding: "utf8",
    env: { ...process.env, npm_config_cache: cache },
  });
  const result = JSON.parse(output);
  const paths = result[0].files.map((file) => file.path).sort();
  const expected = [
    "LICENSE",
    "README.md",
    "dist/index.d.ts",
    "dist/index.js",
    "package.json",
  ];

  if (JSON.stringify(paths) !== JSON.stringify(expected)) {
    throw new Error(
      `unexpected SDK package contents:\n${paths.map((path) => `- ${path}`).join("\n")}`,
    );
  }
} finally {
  rmSync(cache, { recursive: true, force: true });
}
