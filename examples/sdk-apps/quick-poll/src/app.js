// @ts-check
import {
  TinkerCapabilityUnavailableError,
  TinkerVersionConflictError,
  tinker,
} from "@tinkercloud/sdk";
import { describeTinkerError } from "../../shared/errors.js";

const configKey = "quick-poll/config";
const votePrefix = "quick-poll/votes/";
const defaultPoll = {
  question: "Where should we hold the next team day?",
  options: [
    { id: "studio", label: "City studio" },
    { id: "park", label: "Lakeside park" },
    { id: "remote", label: "Remote, with lunch credit" },
  ],
};
const pageLifetime = new AbortController();
let viewer;
let poll = defaultPoll;
let myVote;
let stopLive = () => {};
/** @type {import("@tinkercloud/sdk").KVEntry[]} */
let votes = [];

const choices = /** @type {HTMLElement} */ (
  document.querySelector("#vote-choices")
);
const results = /** @type {HTMLElement} */ (
  document.querySelector("#results")
);
const empty = /** @type {HTMLElement} */ (document.querySelector("#empty"));
const withdraw = /** @type {HTMLButtonElement} */ (
  document.querySelector("#withdraw")
);
const refresh = /** @type {HTMLButtonElement} */ (
  document.querySelector("#refresh")
);
const liveStatus = /** @type {HTMLElement} */ (
  document.querySelector("#live-status")
);
const liveCopy = /** @type {HTMLElement} */ (
  document.querySelector("#live-copy")
);
const notice = /** @type {HTMLElement} */ (document.querySelector("#notice"));

function setLiveStatus(copy, tone = "warning") {
  liveCopy.textContent = copy;
  liveStatus.dataset.tone = tone;
}

function hideError() {
  notice.hidden = true;
  document.querySelector("#notice-request").textContent = "";
}

/** @param {unknown} error */
function showError(error) {
  const detail = describeTinkerError(error);
  document.querySelector("#notice-title").textContent = detail.title;
  document.querySelector("#notice-message").textContent = detail.message;
  document.querySelector("#notice-request").textContent = detail.requestId
    ? `Request ${detail.requestId}`
    : "";
  notice.hidden = false;
}

function render() {
  document.querySelector("#poll-question").textContent = poll.question;
  document.querySelector("#vote-count").textContent = `${votes.length} ${
    votes.length === 1 ? "response" : "responses"
  }`;
  empty.hidden = votes.length > 0;
  choices.textContent = "";
  results.textContent = "";

  const currentChoice = myVote
    ? String(
      /** @type {{option?: unknown}} */ (myVote.value).option ?? "",
    )
    : "";
  const selected = poll.options.find((option) => option.id === currentChoice);
  document.querySelector("#your-vote").textContent = selected
    ? `Your vote: ${selected.label}`
    : "No vote yet";
  withdraw.disabled = !myVote;

  for (const option of poll.options) {
    const button = document.createElement("button");
    button.className = currentChoice === option.id
      ? "button button-primary"
      : "button button-secondary";
    button.type = "button";
    button.textContent = option.label;
    button.setAttribute(
      "aria-pressed",
      String(currentChoice === option.id),
    );
    button.addEventListener("click", () => void castVote(option.id));
    choices.append(button);

    const count = votes.filter((entry) =>
      String(/** @type {{option?: unknown}} */ (entry.value).option ?? "") ===
        option.id
    ).length;
    const percentage = votes.length === 0
      ? 0
      : Math.round((count / votes.length) * 100);
    const row = document.createElement("div");
    row.className = "poll-option";
    const summary = document.createElement("div");
    summary.className = "summary-row";
    const label = document.createElement("strong");
    label.textContent = option.label;
    const total = document.createElement("span");
    total.textContent = `${count} · ${percentage}%`;
    summary.append(label, total);
    const bar = document.createElement("progress");
    bar.className = "bar";
    bar.max = 100;
    bar.value = percentage;
    bar.textContent = `${percentage}%`;
    row.append(summary, bar);
    results.append(row);
  }
}

async function ensurePoll() {
  const current = await tinker.kv.get(configKey, {
    signal: pageLifetime.signal,
  });
  if (!current) {
    const created = await tinker.kv.set(configKey, defaultPoll, {
      signal: pageLifetime.signal,
    });
    poll = /** @type {typeof defaultPoll} */ (created.value);
    return;
  }
  const value = /** @type {{question?: unknown, options?: unknown}} */ (
    current.value
  );
  if (
    typeof value.question !== "string" ||
    !Array.isArray(value.options) ||
    value.options.length < 2
  ) {
    throw new Error("The current poll definition is malformed.");
  }
  poll = /** @type {typeof defaultPoll} */ (current.value);
}

async function readVotes() {
  /** @type {import("@tinkercloud/sdk").KVEntry[]} */
  const entries = [];
  let cursor;
  do {
    const page = await tinker.kv.list({
      prefix: votePrefix,
      limit: 50,
      cursor,
      signal: pageLifetime.signal,
    });
    entries.push(...page.entries);
    cursor = page.next_cursor;
  } while (cursor && entries.length < 500);
  return entries;
}

async function refreshPoll() {
  refresh.disabled = true;
  try {
    await ensurePoll();
    votes = await readVotes();
    myVote = votes.find((entry) =>
      entry.key === `${votePrefix}${viewer.identity.id}`
    );
    render();
    hideError();
  } catch (error) {
    showError(error);
  } finally {
    refresh.disabled = false;
  }
}

async function castVote(option) {
  if (!poll.options.some((item) => item.id === option)) return;
  hideError();
  try {
    await tinker.kv.set(
      `${votePrefix}${viewer.identity.id}`,
      { option, updatedAt: new Date().toISOString() },
      {
        expectedVersion: myVote?.version,
        signal: pageLifetime.signal,
      },
    );
    await refreshPoll();
  } catch (error) {
    showError(error);
    if (error instanceof TinkerVersionConflictError) await refreshPoll();
  }
}

withdraw.addEventListener("click", async () => {
  if (!myVote) return;
  withdraw.disabled = true;
  hideError();
  try {
    await tinker.kv.delete(myVote.key, {
      expectedVersion: myVote.version,
      signal: pageLifetime.signal,
    });
    await refreshPoll();
  } catch (error) {
    showError(error);
    if (error instanceof TinkerVersionConflictError) await refreshPoll();
  } finally {
    withdraw.disabled = false;
  }
});

refresh.addEventListener("click", () => void refreshPoll());
document.addEventListener("visibilitychange", () => {
  if (document.visibilityState === "visible") void refreshPoll();
});

function startLive() {
  // A KV mutation already publishes this bounded, best-effort hint. Reading
  // current KV keeps results correct if an event is missed during reconnect.
  stopLive = tinker.live.onKvChange(
    { prefix: "quick-poll/" },
    () => void refreshPoll(),
  );
  setLiveStatus("Live updates enabled", "success");
}

async function start() {
  try {
    const [currentViewer, app, result] = await Promise.all([
      tinker.user.current({ signal: pageLifetime.signal }),
      tinker.app.info({ signal: pageLifetime.signal }),
      tinker.capabilities.list({ signal: pageLifetime.signal }),
    ]);
    viewer = currentViewer;
    document.querySelector("#viewer-email").textContent = viewer.identity.email;
    document.querySelector("#viewer-avatar").textContent =
      viewer.identity.email.slice(0, 1).toUpperCase();
    document.querySelector("#app-name").textContent = `${app.slug} · private`;

    const capabilities = new Set(result.capabilities.map((item) => item.name));
    if (!capabilities.has("kv")) {
      throw new TinkerCapabilityUnavailableError(
        "This example requires KV.",
        "capability_unavailable",
      );
    }

    await refreshPoll();
    if (capabilities.has("live")) {
      startLive();
    } else {
      setLiveStatus("KV ready · live unavailable");
    }
  } catch (error) {
    showError(error);
    setLiveStatus("App unavailable");
    choices.querySelectorAll("button").forEach((button) => {
      button.disabled = true;
    });
    withdraw.disabled = true;
  }
}

window.addEventListener("pagehide", () => {
  pageLifetime.abort();
  stopLive();
});

void start();
