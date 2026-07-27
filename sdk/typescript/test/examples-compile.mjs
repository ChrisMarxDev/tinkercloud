import assert from "node:assert/strict";
import { readdir } from "node:fs/promises";
import ts from "typescript";

const fixtures = new URL("../../../examples/test-apps/", import.meta.url);
const fixtureNames = (await readdir(fixtures, { withFileTypes: true }))
  .filter((entry) => entry.isDirectory() && entry.name !== "vps-smoke")
  .map((entry) => new URL(`${entry.name}/app.js`, fixtures).pathname);
const examples = new URL("../../../examples/sdk-apps/", import.meta.url);
const exampleNames = (await readdir(examples, { withFileTypes: true }))
  .filter((entry) => entry.isDirectory() && entry.name !== "shared")
  .map((entry) => new URL(`${entry.name}/src/app.js`, examples).pathname);
const names = [
  ...fixtureNames,
  ...exampleNames,
  new URL("shared/errors.js", examples).pathname,
];
assert.ok(exampleNames.length >= 3, "expected three public SDK example apps");
const options = {
  allowJs: true,
  checkJs: true,
  noEmit: true,
  // TypeScript 4.1 is the pinned V1 compiler and predates the ES2022 enum.
  target: ts.ScriptTarget.ES2020,
  module: ts.ModuleKind.ESNext,
  moduleResolution: ts.ModuleResolutionKind.NodeJs,
  baseUrl: new URL("..", import.meta.url).pathname,
  paths: { "@tinyhost/sdk": ["src/index.ts"] },
  skipLibCheck: true,
};
const program = ts.createProgram(names, options);
const diagnostics = ts.getPreEmitDiagnostics(program);
assert.equal(
  diagnostics.length,
  0,
  diagnostics.map((d) => ts.flattenDiagnosticMessageText(d.messageText, "\n")).join("\n"),
);
