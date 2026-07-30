import assert from "node:assert/strict";
import {
  createTinker,
  TinkerQuotaExceededError,
} from "../dist/index.js";

const calls = [];
const fetch = async (path, init = {}) => {
  calls.push({ path, init });
  if (path === "/_tinker/api/v1/blobs" && init.method === "POST") {
    return new Response(JSON.stringify({
      id: "blob_opaque",
      name: "notes.txt",
      size: 5,
      content_type: "text/plain",
      created_at: "2026-07-27T12:00:00Z",
    }), { status: 201, headers: { "Content-Type": "application/json" } });
  }
  if (path === "/_tinker/api/v1/blobs?limit=10") {
    return new Response(JSON.stringify({
      blobs: [{
        id: "blob_opaque",
        name: "notes.txt",
        size: 5,
        content_type: "text/plain",
        created_at: "2026-07-27T12:00:00Z",
      }],
      next_cursor: "cursor_opaque",
    }), { headers: { "Content-Type": "application/json" } });
  }
  if (path === "/_tinker/api/v1/blobs/blob_opaque") {
    if (init.method === "DELETE") {
      return new Response(JSON.stringify({ deleted: true }), {
        headers: { "Content-Type": "application/json" },
      });
    }
    return new Response("hello", {
      headers: { "Content-Type": "text/plain" },
    });
  }
  if (path === "/_tinker/api/v1/blobs/blob_missing") return new Response("", { status: 404 });
  return new Response(JSON.stringify({
    error: { code: "quota_exceeded", message: "full", request_id: "req_safe" },
  }), { status: 429, headers: { "Content-Type": "application/json" } });
};

const tinker = createTinker({ fetch });
const file = new Blob(["hello"], { type: "text/plain" });
Object.defineProperty(file, "name", { value: "notes.txt" });

const uploaded = await tinker.blobs.upload(file);
assert.deepEqual(uploaded, {
  id: "blob_opaque",
  name: "notes.txt",
  size: 5,
  contentType: "text/plain",
  createdAt: "2026-07-27T12:00:00Z",
});
const upload = calls.at(-1);
assert.equal(upload.path, "/_tinker/api/v1/blobs");
assert.equal(upload.init.method, "POST");
assert.ok(upload.init.body instanceof FormData);
const sentFile = upload.init.body.get("file");
assert.equal(sentFile.name, "notes.txt");
assert.equal(await sentFile.text(), "hello");

const list = await tinker.blobs.list({ limit: 10 });
assert.equal(list.blobs[0].contentType, "text/plain");
assert.equal(list.nextCursor, "cursor_opaque");

const data = await tinker.blobs.get("blob_opaque");
assert.ok(data instanceof Blob);
assert.equal(await data.text(), "hello");
assert.equal(await tinker.blobs.get("blob_missing"), null);
assert.deepEqual(await tinker.blobs.delete("blob_opaque"), { deleted: true });

await assert.rejects(
  tinker.blobs.list({ cursor: "server-error" }),
  (error) => error instanceof TinkerQuotaExceededError && error.requestId === "req_safe",
);
