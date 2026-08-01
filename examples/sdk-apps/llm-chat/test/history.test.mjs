import assert from "node:assert/strict";
import { beginTurn, completeTurn } from "../src/history.js";

let history = [];
for (let index = 0; index < 5; index += 1) {
  history = completeTurn(beginTurn(history, `question ${index}`), `answer ${index}`);
}
assert.equal(history.length, 10);
const before = history;
const pending = beginTurn(history, "cancelled question");
assert.equal(pending.length, 9);
assert.deepEqual(before, history, "beginTurn never mutates completed context");
assert.equal(before.length % 2, 0, "a failed request can restore complete pairs");
history = completeTurn(pending, "new answer");
assert.equal(history.length, 10);
for (let index = 0; index < history.length; index += 1) {
  assert.equal(history[index].role, index % 2 === 0 ? "user" : "assistant");
}
