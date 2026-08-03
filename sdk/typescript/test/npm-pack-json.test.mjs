import assert from "node:assert/strict";
import { parseNpmPackJson } from "./npm-pack-json.mjs";

const packageName = "@tinkercloud/sdk";
const packResult = { name: packageName, files: [{ path: "package.json" }] };

assert.deepEqual(
  parseNpmPackJson(JSON.stringify([packResult]), packageName),
  packResult,
);
assert.deepEqual(
  parseNpmPackJson(JSON.stringify({ [packageName]: packResult }), packageName),
  packResult,
);

for (const malformed of [
  "not JSON",
  "[]",
  JSON.stringify([packResult, packResult]),
  JSON.stringify([{ ...packResult, name: "@other/sdk" }]),
  JSON.stringify({}),
  JSON.stringify({ "@other/sdk": packResult }),
  JSON.stringify({ [packageName]: packResult, "@other/sdk": packResult }),
  JSON.stringify({ [packageName]: {} }),
  JSON.stringify({ [packageName]: { files: [{}] } }),
]) {
  assert.throws(() => parseNpmPackJson(malformed, packageName));
}
