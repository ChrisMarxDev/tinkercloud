import {
  cp,
  mkdtemp,
  mkdir,
  readFile,
  readdir,
  rm,
  writeFile,
} from "node:fs/promises";
import { tmpdir } from "node:os";
import { basename, join } from "node:path";
import { fileURLToPath } from "node:url";

const root = fileURLToPath(new URL(".", import.meta.url));
const sdk = fileURLToPath(
  new URL("../../sdk/typescript/dist/index.js", import.meta.url),
);
const shared = join(root, "shared");
const checkOnly = process.argv.includes("--check");

async function exampleNames() {
  const entries = await readdir(root, { withFileTypes: true });
  return entries
    .filter((entry) => entry.isDirectory() && entry.name !== "shared")
    .map((entry) => entry.name)
    .sort();
}

async function rewriteJavaScript(path) {
  const source = await readFile(path, "utf8");
  const browserSource = source
    .replaceAll('from "@tinkercloud/sdk"', 'from "./tinker-sdk.js"')
    .replaceAll('import("@tinkercloud/sdk")', 'import("./tinker-sdk.js")')
    .replaceAll('from "../../shared/errors.js"', 'from "./errors.js"');
  await writeFile(path, browserSource);
}

async function buildExample(name, output) {
  const source = join(root, name, "src");
  await mkdir(output, { recursive: true });
  await cp(source, output, { recursive: true });
  await cp(join(shared, "styles.css"), join(output, "styles.css"));
  await cp(join(shared, "errors.js"), join(output, "errors.js"));
  await cp(sdk, join(output, "tinker-sdk.js"));
  await rewriteJavaScript(join(output, "app.js"));
  await rewriteJavaScript(join(output, "errors.js"));
}

async function assertRelease(name, output) {
  const required = [
    "app.js",
    "errors.js",
    "index.html",
    "styles.css",
    "tinker-sdk.js",
  ];
  const files = (await readdir(output)).sort();
  for (const file of required) {
    if (!files.includes(file)) {
      throw new Error(`${name}: generated release is missing ${file}`);
    }
  }

  for (const file of files.filter((candidate) =>
    /\.(?:html|js|css)$/.test(candidate)
  )) {
    const source = await readFile(join(output, file), "utf8");
    if (
      file !== "tinker-sdk.js" &&
      /(?:from\s+|import\()["']@tinkercloud\/sdk["']/.test(source)
    ) {
      throw new Error(`${name}/${file}: unresolved SDK package import`);
    }
    if (
      file !== "tinker-sdk.js" &&
      (source.includes("http://") || source.includes("https://"))
    ) {
      throw new Error(`${name}/${file}: remote URL is not allowed`);
    }
    if (
      file !== "tinker-sdk.js" &&
      /\b(?:appId|app_id|viewerToken|deployerToken|apiSecret)\b/.test(source)
    ) {
      throw new Error(`${name}/${file}: client-selected identity or credential`);
    }
  }
}

const names = await exampleNames();
if (names.length < 3) {
  throw new Error("expected at least three SDK example apps");
}

let temporaryRoot;
try {
  if (checkOnly) {
    temporaryRoot = await mkdtemp(join(tmpdir(), "tinker-sdk-examples-"));
  }
  for (const name of names) {
    const output = checkOnly
      ? join(temporaryRoot, basename(name))
      : join(root, name, "dist");
    if (!checkOnly) {
      await rm(output, { recursive: true, force: true });
    }
    await buildExample(name, output);
    await assertRelease(name, output);
    process.stdout.write(`built ${name}\n`);
  }
} finally {
  if (temporaryRoot) {
    await rm(temporaryRoot, { recursive: true, force: true });
  }
}
