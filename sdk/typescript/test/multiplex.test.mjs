import assert from "node:assert/strict";
import { createTinker } from "../dist/index.js";

class FakeSocket {
  constructor() { this.readyState = 0; this.sent = []; this.closeCalls = 0; this.onopen = this.onclose = this.onerror = this.onmessage = null; }
  send(value) { this.sent.push(JSON.parse(value)); }
  open() { this.readyState = 1; this.onopen?.({}); }
  close() { this.closeCalls += 1; this.readyState = 3; this.onclose?.({}); }
  message(value) { this.onmessage?.({ data: JSON.stringify(value) }); }
}

const sockets = [];
const timers = [];
const flush = async () => { for (let index = 0; index < 4; index += 1) await Promise.resolve(); };
const tinker = createTinker({
  origin: "https://app.test",
  fetch: async () => new Response(JSON.stringify({ documents: [], revision: 0 }), { headers: { "Content-Type": "application/json" } }),
  webSocket: () => { const socket = new FakeSocket(); sockets.push(socket); return socket; },
  schedule: (fn) => { timers.push(fn); return fn; },
  cancel: (id) => { const index = timers.indexOf(id); if (index >= 0) timers.splice(index, 1); },
  random: () => 0,
});

const calls = [];
let taskSnapshots = 0;
const stops = [
  tinker.live.onKvChange({ prefix: "a/" }, () => calls.push("a")),
  tinker.live.onKvChange({ prefix: "b/" }, () => calls.push("b")),
  tinker.live.onKvChange({ prefix: "c/" }, () => calls.push("c")),
  tinker.db.collection("tasks").subscribe({ onSnapshot: () => { taskSnapshots += 1; } }),
  tinker.db.collection("notes").subscribe({ onSnapshot() {}, onCreate: () => calls.push("notes") }),
  tinker.db.collection("events").subscribe({ onSnapshot() {}, onCreate: () => calls.push("events") }),
];

assert.equal(sockets.length, 1, "all managed capability listeners share one socket");
sockets[0].open();
await flush();
assert.deepEqual(sockets[0].sent, [
  { v: 1, type: "subscribe_kv", prefix: "a/" },
  { v: 1, type: "subscribe_kv", prefix: "b/" },
  { v: 1, type: "subscribe_kv", prefix: "c/" },
  { v: 1, type: "subscribe_collection", collection: "tasks" },
  { v: 1, type: "subscribe_collection", collection: "notes" },
  { v: 1, type: "subscribe_collection", collection: "events" },
]);

stops[0]();
stops[4]();
assert.deepEqual(sockets[0].sent.slice(-2), [
  { v: 1, type: "unsubscribe_kv", prefix: "a/" },
  { v: 1, type: "unsubscribe_collection", collection: "notes" },
]);
sockets[0].message({ v: 1, type: "kv.changed", key: "a/ignored", version: 1, deleted: false });
sockets[0].message({ v: 1, type: "collection.changed", collection: "notes", id: "doc_abcdefghijklmnopqrstuv", version: 1, revision: 1, deleted: false });
sockets[0].message({ v: 1, type: "kv.changed", key: "b/live", version: 1, deleted: false });
assert.deepEqual(calls, ["b"], "unsubscribed handlers never receive stale callbacks");

// Reconnect only restores the four still-active server subscriptions.
sockets[0].close();
assert.equal(timers.length, 1);
timers.shift()();
assert.equal(sockets.length, 2);
sockets[1].open();
await flush();
assert.deepEqual(sockets[1].sent, [
  { v: 1, type: "subscribe_kv", prefix: "b/" },
  { v: 1, type: "subscribe_kv", prefix: "c/" },
  { v: 1, type: "subscribe_collection", collection: "tasks" },
  { v: 1, type: "subscribe_collection", collection: "events" },
]);
sockets[1].message({ v: 1, type: "collection.changed", collection: "tasks", id: "doc_bcdefghijklmnopqrstuvw", version: 1, revision: 1, deleted: false });
await flush();
assert.ok(taskSnapshots >= 1, "active collection keeps its snapshot subscription through reconnect");

for (const index of [1, 2, 3, 5]) stops[index]();
assert.equal(sockets[1].readyState, 3, "the final managed unsubscribe closes the shared socket");
assert.equal(sockets[1].closeCalls, 1, "the final managed unsubscribe closes its socket exactly once");

const explicit = tinker.live.channel("direct");
const connecting = explicit.connect();
assert.equal(sockets.length, 3, "explicit channels keep a dedicated socket");
sockets[2].open();
await connecting;
explicit.close();
explicit.close();
assert.equal(sockets[2].closeCalls, 1, "explicit close is idempotent at the underlying socket");
