// @ts-check
import {
  TinyCapabilityUnavailableError,
  TinyVersionConflictError,
  tiny,
} from "@tinyhost/sdk";
import { describeTinyError } from "../../shared/errors.js";

const prefix = "team-pulse/viewers/";
const pageLifetime = new AbortController();
const pulseLabels = {
  focused: { emoji: "◎", label: "Focused" },
  open: { emoji: "✦", label: "Open to help" },
  blocked: { emoji: "△", label: "Blocked" },
  away: { emoji: "☕", label: "Away" },
};
let viewer;
let channel;
let stopChannelEvents = () => {};
let statusTimer;
let lastLiveStatus = "offline";
let selectedPulse = "focused";
let myEntry;
/** @type {import("@tinyhost/sdk").KVEntry[]} */
let pulses = [];

const form = /** @type {HTMLFormElement} */ (
  document.querySelector("#pulse-form")
);
const note = /** @type {HTMLTextAreaElement} */ (
  document.querySelector("#pulse-note")
);
const save = /** @type {HTMLButtonElement} */ (
  document.querySelector("#save-pulse")
);
const clear = /** @type {HTMLButtonElement} */ (
  document.querySelector("#clear-pulse")
);
const refresh = /** @type {HTMLButtonElement} */ (
  document.querySelector("#refresh")
);
const grid = /** @type {HTMLElement} */ (
  document.querySelector("#pulse-grid")
);
const empty = /** @type {HTMLElement} */ (document.querySelector("#empty"));
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
  const detail = describeTinyError(error);
  document.querySelector("#notice-title").textContent = detail.title;
  document.querySelector("#notice-message").textContent = detail.message;
  document.querySelector("#notice-request").textContent = detail.requestId
    ? `Request ${detail.requestId}`
    : "";
  notice.hidden = false;
}

function render() {
  grid.textContent = "";
  empty.hidden = pulses.length > 0;
  document.querySelector("#pulse-count").textContent = `${pulses.length} ${
    pulses.length === 1 ? "signal" : "signals"
  }`;

  for (const entry of pulses) {
    const value = /** @type {{email?: unknown, pulse?: unknown, note?: unknown, updatedAt?: unknown}} */ (
      entry.value
    );
    const kind = String(value.pulse ?? "focused");
    const detail = pulseLabels[kind] ?? { emoji: "◌", label: "Checking in" };
    const card = document.createElement("article");
    card.className = "pulse-card";

    const emoji = document.createElement("span");
    emoji.className = "choice-emoji";
    emoji.setAttribute("aria-hidden", "true");
    emoji.textContent = detail.emoji;
    const label = document.createElement("strong");
    label.textContent = `${String(value.email ?? "Viewer")} · ${detail.label}`;
    const context = document.createElement("small");
    context.textContent = value.note ? String(value.note) : "No note";
    const time = document.createElement("small");
    const parsed = new Date(String(value.updatedAt ?? entry.updated_at));
    time.textContent = Number.isNaN(parsed.valueOf())
      ? "Updated recently"
      : `Updated ${new Intl.DateTimeFormat(undefined, {
        hour: "numeric",
        minute: "2-digit",
      }).format(parsed)}`;

    card.append(emoji, label, context, time);
    grid.append(card);
  }
}

async function readPulses() {
  /** @type {import("@tinyhost/sdk").KVEntry[]} */
  const entries = [];
  let cursor;
  do {
    const page = await tiny.kv.list({
      prefix,
      limit: 50,
      cursor,
      signal: pageLifetime.signal,
    });
    entries.push(...page.entries);
    cursor = page.next_cursor;
  } while (cursor && entries.length < 500);
  return entries.sort((a, b) => b.updated_at.localeCompare(a.updated_at));
}

async function refreshPulses() {
  refresh.disabled = true;
  try {
    pulses = await readPulses();
    myEntry = pulses.find((entry) =>
      entry.key === `${prefix}${viewer.identity.id}`
    );
    render();
    hideError();
  } catch (error) {
    showError(error);
  } finally {
    refresh.disabled = false;
  }
}

function publishRefreshHint() {
  if (!channel || channel.connectionStatus !== "connected") {
    setLiveStatus("Saved · manual refresh");
    return;
  }
  try {
    channel.publish("pulse.changed", { changed: true });
  } catch (error) {
    showError(error);
    setLiveStatus("Saved · reconnecting");
  }
}

for (const button of document.querySelectorAll("[data-pulse]")) {
  if (!(button instanceof HTMLButtonElement)) continue;
  button.setAttribute("aria-pressed", String(button.dataset.pulse === selectedPulse));
  button.addEventListener("click", () => {
    selectedPulse = button.dataset.pulse ?? "focused";
    for (const choice of document.querySelectorAll("[data-pulse]")) {
      if (!(choice instanceof HTMLButtonElement)) continue;
      choice.setAttribute(
        "aria-pressed",
        String(choice.dataset.pulse === selectedPulse),
      );
    }
  });
}

form.addEventListener("submit", async (event) => {
  event.preventDefault();
  const trimmedNote = note.value.trim();
  if (trimmedNote.length > 80) return;
  save.disabled = true;
  hideError();
  try {
    await tiny.kv.set(
      `${prefix}${viewer.identity.id}`,
      {
        email: viewer.identity.email,
        pulse: selectedPulse,
        note: trimmedNote,
        updatedAt: new Date().toISOString(),
      },
      {
        expectedVersion: myEntry?.version,
        signal: pageLifetime.signal,
      },
    );
    await refreshPulses();
    publishRefreshHint();
  } catch (error) {
    showError(error);
    if (error instanceof TinyVersionConflictError) await refreshPulses();
  } finally {
    save.disabled = false;
  }
});

clear.addEventListener("click", async () => {
  if (!myEntry) return;
  clear.disabled = true;
  try {
    await tiny.kv.delete(myEntry.key, {
      expectedVersion: myEntry.version,
      signal: pageLifetime.signal,
    });
    await refreshPulses();
    publishRefreshHint();
  } catch (error) {
    showError(error);
    if (error instanceof TinyVersionConflictError) await refreshPulses();
  } finally {
    clear.disabled = false;
  }
});

refresh.addEventListener("click", () => void refreshPulses());
document.addEventListener("visibilitychange", () => {
  if (document.visibilityState === "visible") void refreshPulses();
});

async function startLive() {
  channel = tiny.live.channel("team-pulse");
  stopChannelEvents = channel.on(
    "pulse.changed",
    () => void refreshPulses(),
  );
  try {
    await channel.connect();
    setLiveStatus("Live hints connected", "success");
  } catch (error) {
    showError(error);
    setLiveStatus("KV ready · live reconnecting");
  }

  statusTimer = window.setInterval(() => {
    const current = channel.connectionStatus;
    if (current === "connected") {
      setLiveStatus("Live hints connected", "success");
      if (lastLiveStatus === "reconnecting") void refreshPulses();
    } else if (current === "reconnecting" || current === "connecting") {
      setLiveStatus("Current view kept · reconnecting");
    } else {
      setLiveStatus("KV ready · manual refresh");
    }
    lastLiveStatus = current;
  }, 1000);
}

async function start() {
  try {
    const [currentViewer, app, result] = await Promise.all([
      tiny.user.current({ signal: pageLifetime.signal }),
      tiny.app.info({ signal: pageLifetime.signal }),
      tiny.capabilities.list({ signal: pageLifetime.signal }),
    ]);
    viewer = currentViewer;
    document.querySelector("#viewer-email").textContent = viewer.identity.email;
    document.querySelector("#viewer-avatar").textContent =
      viewer.identity.email.slice(0, 1).toUpperCase();
    document.querySelector("#app-name").textContent = `${app.slug} · private`;

    const capabilities = new Set(result.capabilities.map((item) => item.name));
    if (!capabilities.has("kv")) {
      throw new TinyCapabilityUnavailableError(
        "This example requires KV.",
        "capability_unavailable",
      );
    }

    await refreshPulses();
    if (capabilities.has("live")) {
      await startLive();
    } else {
      setLiveStatus("KV ready · live unavailable");
    }
  } catch (error) {
    showError(error);
    setLiveStatus("App unavailable");
    form.querySelectorAll("button, textarea").forEach((control) => {
      if (
        control instanceof HTMLButtonElement ||
        control instanceof HTMLTextAreaElement
      ) {
        control.disabled = true;
      }
    });
  }
}

window.addEventListener("pagehide", () => {
  pageLifetime.abort();
  if (statusTimer) window.clearInterval(statusTimer);
  stopChannelEvents();
  channel?.close();
});

void start();
