import assert from "node:assert/strict";
import { stat } from "node:fs/promises";

// This is intentionally an uncompressed output gate: it catches accidental
// runtime dependencies before compression masks their impact on small apps.
// Blobs add one binary-response path and multipart upload while preserving a
// dependency-free browser module. Keep the V1 ceiling explicit at 16 KiB.
const MAX_BYTES = 16 * 1024;
const bytes = (await stat(new URL("../dist/index.js", import.meta.url))).size;
assert.ok(bytes <= MAX_BYTES, `SDK bundle is ${bytes} bytes; limit is ${MAX_BYTES}`);
