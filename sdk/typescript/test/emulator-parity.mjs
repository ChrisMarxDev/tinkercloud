import assert from "node:assert/strict";
import { createTinker } from "../dist/index.js";

const origin = process.env.TINKER_EMULATOR_PARITY_ORIGIN;
assert.ok(origin, "TINKER_EMULATOR_PARITY_ORIGIN is required");
assert.equal(typeof WebSocket, "function", "this parity test requires Node's built-in WebSocket");

const sockets = [];
function browserSocket(url, protocols) {
  const raw = new WebSocket(url, protocols);
  const bridge = {
    get readyState() { return raw.readyState; },
    send(value) { raw.send(value); },
    close() { raw.close(); },
    onopen: null,
    onclose: null,
    onerror: null,
    onmessage: null,
  };
  raw.onopen = (event) => bridge.onopen?.(event);
  raw.onclose = (event) => bridge.onclose?.(event);
  raw.onerror = (event) => bridge.onerror?.(event);
  raw.onmessage = (event) => bridge.onmessage?.(event);
  sockets.push(bridge);
  return bridge;
}
function deferred() {
  let resolve;
  let reject;
  const promise = new Promise((ok, fail) => { resolve = ok; reject = fail; });
  return { promise, resolve, reject };
}
async function within(promise, label) {
  let timer;
  try {
    return await Promise.race([
      promise,
      new Promise((_, reject) => { timer = setTimeout(() => reject(new Error(`timed out: ${label}`)), 3000); }),
    ]);
  } finally {
    clearTimeout(timer);
  }
}

const tinker = createTinker({
  origin,
  webSocket: browserSocket,
  // Browser fetch resolves Tinkercloud's same-origin relative paths. Node does
  // not, so this narrow adapter preserves the browser request shape.
  fetch: (path, init) => fetch(new URL(path, origin), init),
});
const first = await tinker.kv.set("parity-one", { value: 1 });
assert.equal(first.version, 1);
assert.deepEqual((await tinker.kv.get("parity-one")).value, { value: 1 });

const tasks = tinker.db.collection("tasks");
const created = await tasks.create({ title: "first", done: false });
assert.deepEqual((await tasks.get(created.id)).data, { title: "first", done: false });
const updated = await tasks.update(created.id, { title: "first", done: true }, { expectedVersion: created.version });
const snapshot = await tasks.list();
assert.equal(snapshot.revision, 2);
assert.equal(snapshot.documents[0].version, updated.version);

const initial = deferred();
const refreshed = deferred();
const reconnected = deferred();
let snapshots = 0;
let connections = 0;
const stop = tasks.subscribe({
  onSnapshot: (page) => {
    snapshots += 1;
    if (snapshots === 1) initial.resolve(page);
    if (page.documents.some((entry) => entry.data.title === "after reconnect")) refreshed.resolve(page);
  },
  onStatus: (status) => {
    if (status === "connected") {
      connections += 1;
      if (connections === 2) reconnected.resolve();
    }
  },
  onError: (error) => initial.reject(error),
});
await within(initial.promise, "initial collection snapshot");
assert.equal(sockets.length, 1, "collection subscription opens one browser-shaped socket");
sockets[0].close();
await within(reconnected.promise, "collection reconnect");
assert.equal(sockets.length, 2, "SDK created a fresh socket after close");
await tasks.create({ title: "after reconnect", done: false });
await within(refreshed.promise, "post-reconnect collection recovery");
stop();
