import assert from "node:assert/strict";
import { execFileSync } from "node:child_process";
import {
  mkdtempSync,
  mkdirSync,
  readFileSync,
  rmSync,
  writeFileSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

const work = mkdtempSync(join(tmpdir(), "tinkercloud-sdk-consumer-"));
const cache = join(work, "npm-cache");
const packageRoot = new URL("../", import.meta.url);
const expectedVersion = JSON.parse(
  readFileSync(new URL("package.json", packageRoot), "utf8"),
).version;
const npmEnvironment = {
  ...process.env,
  npm_config_cache: cache,
  // A parent `npm publish --dry-run` exports its dry-run setting to lifecycle
  // scripts. This nested pack is intentionally real and remains local.
  npm_config_dry_run: "false",
  npm_config_provenance: "false",
};

try {
  const packed = execFileSync(
    "npm",
    ["pack", "--silent", "--pack-destination", work],
    {
      cwd: packageRoot,
      encoding: "utf8",
      env: npmEnvironment,
    },
  ).trim();
  const tarball = join(work, packed.split(/\r?\n/).at(-1));
  const consumer = join(work, "consumer");
  mkdirSync(consumer);
  writeFileSync(
    join(consumer, "package.json"),
    JSON.stringify({ private: true, type: "module" }),
  );

  execFileSync(
    "npm",
    [
      "install",
      "--offline",
      "--ignore-scripts",
      "--no-audit",
      "--no-fund",
      "--package-lock=false",
      "--prefix",
      consumer,
      tarball,
    ],
    {
      stdio: "pipe",
      env: npmEnvironment,
    },
  );

  const installedManifest = JSON.parse(
    readFileSync(
      join(consumer, "node_modules", "@tinkercloud", "sdk", "package.json"),
      "utf8",
    ),
  );
  assert.equal(installedManifest.name, "@tinkercloud/sdk");
  assert.equal(installedManifest.version, expectedVersion);
  assert.equal(installedManifest.dependencies, undefined);

  const probe = join(consumer, "probe.mjs");
  writeFileSync(
    probe,
    [
      'import assert from "node:assert/strict";',
      'import { SDK_VERSION, createTinker, tinker } from "@tinkercloud/sdk";',
      `assert.equal(SDK_VERSION, ${JSON.stringify(expectedVersion)});`,
      'assert.equal(typeof createTinker, "function");',
      'assert.equal(typeof tinker.user.current, "function");',
      "",
    ].join("\n"),
  );
  execFileSync(process.execPath, [probe], { cwd: consumer, stdio: "pipe" });
} finally {
  rmSync(work, { recursive: true, force: true });
}
