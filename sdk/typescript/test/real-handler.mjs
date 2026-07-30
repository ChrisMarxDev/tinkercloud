import assert from "node:assert/strict";
import { request } from "node:http";
import { createTiny, TinyNotAuthorizedError, TinyVersionIncompatibleError } from "../dist/index.js";

const origin = process.env.TINY_SDK_CONTRACT_ORIGIN;
const cookie = process.env.TINY_SDK_CONTRACT_COOKIE;
const appHost = process.env.TINY_SDK_CONTRACT_HOST;
assert.ok(origin && cookie && appHost, "real handler contract environment is missing");

async function networkFetch(input, init = {}, host = appHost, includeCookie = true) {
  const url = new URL(input, origin);
  const headers = new Headers(init.headers);
  if (includeCookie) headers.set("Cookie", cookie);
  // CI has no wildcard DNS. This browser-shaped adapter connects to the local
  // listener while retaining the hostname a browser would send after DNS.
  headers.set("Host", host);
  if (["PUT", "POST", "DELETE"].includes(init.method)) headers.set("Origin", `http://${host}`);
  let body = init.body;
  if (body instanceof FormData) {
    const encoded = new Request("http://tinyhost.invalid", { method: "POST", body });
    headers.set("Content-Type", encoded.headers.get("Content-Type"));
    body = Buffer.from(await encoded.arrayBuffer());
  }
  if (typeof body === "string") body = Buffer.from(body);
  if (body) headers.set("Content-Length", String(body.length));
  return new Promise((resolve, reject) => {
    const req = request(url, { method: init.method, headers: Object.fromEntries(headers) }, (res) => {
      const chunks = [];
      res.on("data", (chunk) => chunks.push(chunk));
      res.on("end", () => {
        const body = Buffer.concat(chunks);
        resolve(new Response(body, { status: res.statusCode, headers: res.headers }));
      });
    });
    req.on("error", reject);
    if (body) req.write(body);
    req.end();
  });
}
const realFetch = (input, init = {}) => networkFetch(input, init);

const tiny = createTiny({ fetch: realFetch, origin });
const user = await tiny.user.current();
assert.equal(user.identity.email, "viewer@example.com");
assert.equal(user.app.slug, "alpha");
assert.deepEqual(await tiny.app.info(), { slug: "alpha", features: { kv: true, db: true, blobs: true, realtime: true, llm_chat: false } });
assert.deepEqual((await tiny.capabilities.list()).capabilities.map((c) => c.name), ["user", "kv", "db", "blobs", "live"]);

assert.equal(await tiny.kv.get("items/a"), null);

const tasks = tiny.db.collection("tasks");
const task = await tasks.create({ title: "Ship", done: false });
assert.equal(task.version, 1);
assert.deepEqual((await tasks.get(task.id)).data, { title: "Ship", done: false });
const changedTask = await tasks.update(task.id, { title: "Ship", done: true }, { expectedVersion: task.version });
assert.equal(changedTask.version, 2);
const taskSnapshot = await tasks.list();
assert.equal(taskSnapshot.revision, 2);
assert.deepEqual(taskSnapshot.documents.map((entry) => entry.id), [task.id]);
assert.deepEqual(await tasks.delete(task.id, { expectedVersion: changedTask.version }), { deleted: true });

const file = new Blob(["hello"], { type: "text/plain" });
Object.defineProperty(file, "name", { value: "notes.txt" });
const uploaded = await tiny.blobs.upload(file);
assert.equal(uploaded.name, "notes.txt");
assert.equal(uploaded.contentType, "text/plain");
assert.equal(uploaded.size, 5);
assert.equal((await tiny.blobs.list()).blobs[0].id, uploaded.id);
const bytes = await tiny.blobs.get(uploaded.id);
assert.equal(await bytes.text(), "hello");
assert.deepEqual(await tiny.blobs.delete(uploaded.id), { deleted: true });
assert.equal(await tiny.blobs.get(uploaded.id), null);
const first = await tiny.kv.set("items/a", { value: 1 });
assert.equal(first.version, 1);
await tiny.kv.set("items/b", { value: 2 });
assert.deepEqual((await tiny.kv.get("items/a")).value, { value: 1 });
assert.deepEqual((await tiny.kv.list({ prefix: "items/" })).entries.map((entry) => entry.key), ["items/a", "items/b"]);
assert.deepEqual(await tiny.kv.delete("items/a", { expectedVersion: first.version }), { deleted: true });
assert.equal(await tiny.kv.get("items/a"), null);

// There is no app selector in the SDK. A surplus JavaScript property cannot
// alter the host-derived app either.
await tiny.kv.set("tenant-proof", { app: "alpha" }, { appId: "beta" });
const betaResponse = await networkFetch("/_tiny/api/v1/kv/tenant-proof", {}, appHost.replace("alpha.localhost", "beta.localhost"));
assert.equal(betaResponse.status, 401, "an alpha session must not read beta");

// No header remains compatible for raw clients. It is intentionally checked
// against the same listener and session, not a fake fetch response.
const raw = await networkFetch("/_tiny/api/v1/me");
assert.equal(raw.status, 200);

const incompatible = createTiny({
  fetch: (input, init = {}) => {
    const headers = new Headers(init.headers);
    headers.set("X-Tiny-SDK-Version", "9.0.0");
    return realFetch(input, { ...init, headers });
  },
  origin,
});
await assert.rejects(
  incompatible.user.current(),
  (error) => error instanceof TinyVersionIncompatibleError && error.code === "sdk_version_incompatible" && !!error.requestId,
);

const anonymous = createTiny({ fetch: (input, init = {}) => networkFetch(input, init, appHost, false), origin });
await assert.rejects(anonymous.user.current(), TinyNotAuthorizedError);
