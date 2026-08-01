#!/usr/bin/env node

import { readFile } from "node:fs/promises";

const [sourcePath] = process.argv.slice(2);
if (!sourcePath || process.argv.length !== 3) {
  console.error("usage: extract-sdk-version.mjs SOURCE");
  process.exit(2);
}

const source = await readFile(sourcePath, "utf8");
const declarations = [...source.matchAll(/^export const SDK_VERSION\b.*$/gm)];
const exact = [
  ...source.matchAll(
    /^export const SDK_VERSION = "((?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*)\.(?:0|[1-9][0-9]*))";\r?$/gm,
  ),
];

if (declarations.length !== 1 || exact.length !== 1) {
  console.error("SDK_VERSION must have exactly one strict MAJOR.MINOR.PATCH declaration");
  process.exit(1);
}

process.stdout.write(`${exact[0][1]}\n`);
