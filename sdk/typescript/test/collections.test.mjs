import assert from "node:assert/strict";
import {
  createTiny,
  TinyVersionConflictError,
} from "../dist/index.js";

class FakeSocket {
  constructor() {
    this.readyState = 0;
    this.sent = [];
    this.onopen = this.onclose = this.onerror = this.onmessage = null;
  }
  send(value) { this.sent.push(JSON.parse(value)); }
  open() { this.readyState = 1; this.onopen?.({}); }
  close() { this.readyState = 3; this.onclose?.({}); }
  message(value) { this.onmessage?.({ data: JSON.stringify(value) }); }
}

const sockets = [];
const timers = [];
let revision = 0;
let documents = [];
let nextID = 1;

function response(body, status = 200) {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

async function fakeFetch(input, init = {}) {
  const url = new URL(input, "https://app.test");
  const parts = url.pathname.split("/").filter(Boolean);
  assert.deepEqual(parts.slice(0, 4), ["_tiny", "api", "v1", "db"]);
  assert.equal(decodeURIComponent(parts[4]), "tasks");
  const id = parts[5] && decodeURIComponent(parts[5]);
  const method = init.method ?? "GET";
  const body = init.body ? JSON.parse(init.body) : {};

  if (method === "POST" && !id) {
    const now = new Date().toISOString();
    const document = {
      id: `doc_${nextID++}`,
      data: body.data,
      version: 1,
      created_at: now,
      updated_at: now,
    };
    documents.push(document);
    revision++;
    return response(document, 201);
  }
  if (method === "GET" && !id) {
    return response({ documents, revision, next_cursor: "" });
  }
  const index = documents.findIndex((document) => document.id === id);
  if (method === "GET") {
    return index < 0
      ? response({ error: { code: "not_found", message: "missing" } }, 404)
      : response(documents[index]);
  }
  if (method === "PUT") {
    if (index < 0) return response({ error: { code: "not_found", message: "missing" } }, 404);
    if (body.expected_version !== undefined &&
        body.expected_version !== documents[index].version) {
      return response({
        error: { code: "version_conflict", message: "changed" },
      }, 409);
    }
    documents[index] = {
      ...documents[index],
      data: body.data,
      version: documents[index].version + 1,
      updated_at: new Date().toISOString(),
    };
    revision++;
    return response(documents[index]);
  }
  if (method === "DELETE") {
    if (index < 0) return response({ deleted: false });
    documents.splice(index, 1);
    revision++;
    return response({ deleted: true });
  }
  throw new Error(`unexpected request ${method} ${url}`);
}

const tiny = createTiny({
  origin: "https://app.test",
  fetch: fakeFetch,
  webSocket: () => {
    const socket = new FakeSocket();
    sockets.push(socket);
    return socket;
  },
  schedule: (fn) => { timers.push(fn); return fn; },
  cancel: (id) => {
    const index = timers.indexOf(id);
    if (index >= 0) timers.splice(index, 1);
  },
  random: () => 0,
});
const tasks = tiny.db.collection("tasks");

const created = await tasks.create({ title: "One", done: false });
assert.equal(created.id, "doc_1");
assert.equal(created.version, 1);
assert.deepEqual((await tasks.get(created.id)).data, {
  title: "One",
  done: false,
});
const updated = await tasks.update(created.id, {
  title: "One",
  done: true,
}, { expectedVersion: created.version });
assert.equal(updated.version, 2);
await assert.rejects(
  tasks.update(created.id, { title: "stale" }, { expectedVersion: 1 }),
  TinyVersionConflictError,
);
assert.deepEqual((await tasks.list()).documents.map((document) => document.id), [
  created.id,
]);

const snapshots = [];
const creates = [];
const updates = [];
const deletes = [];
const statuses = [];
const stop = tasks.subscribe({
  onSnapshot: (snapshot) => snapshots.push(snapshot),
  onCreate: (document) => creates.push(document.id),
  onUpdate: (document) => updates.push(document.id),
  onDelete: (id) => deletes.push(id),
  onStatus: (status) => statuses.push(status),
  onError: (error) => { throw error; },
});
sockets[0].open();
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(sockets[0].sent, [{
  v: 1,
  type: "subscribe_collection",
  collection: "tasks",
}]);
assert.equal(snapshots.length, 1);
assert.deepEqual(creates, [], "initial documents are a snapshot, not creates");

const second = await tasks.create({ title: "Two" });
sockets[0].message({
  v: 1,
  type: "collection.changed",
  collection: "tasks",
  id: second.id,
  version: 1,
  revision,
  deleted: false,
});
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(creates, [second.id]);

await tasks.update(second.id, { title: "Two updated" }, {
  expectedVersion: second.version,
});
sockets[0].message({
  v: 1,
  type: "collection.changed",
  collection: "tasks",
  id: second.id,
  version: 2,
  revision,
  deleted: false,
});
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(updates, [second.id]);

// A mutation missed while disconnected is recovered from the authoritative
// snapshot as soon as the SDK reconnects.
sockets[0].close();
await tasks.delete(second.id, { expectedVersion: 2 });
assert.equal(timers.length, 1);
timers.shift()();
sockets[1].open();
await new Promise((resolve) => setTimeout(resolve, 0));
assert.deepEqual(sockets[1].sent, [{
  v: 1,
  type: "subscribe_collection",
  collection: "tasks",
}]);
assert.deepEqual(deletes, [second.id]);
assert.ok(statuses.includes("reconnecting"));

stop();
assert.equal(statuses.at(-1), "closed");
