// @ts-check
import {
  TinkerCapabilityUnavailableError,
  TinkerVersionConflictError,
  tinker,
} from "@tinkercloud/sdk";
import { describeTinkerError } from "../../shared/errors.js";

const prefix = "checklist/tasks/";
const pageLifetime = new AbortController();
let refreshRequest;
let stopLive = () => {};
/** @type {import("@tinkercloud/sdk").KVEntry[]} */
let tasks = [];

const taskForm = /** @type {HTMLFormElement} */ (
  document.querySelector("#task-form")
);
const titleInput = /** @type {HTMLInputElement} */ (
  document.querySelector("#task-title")
);
const addButton = /** @type {HTMLButtonElement} */ (
  document.querySelector("#add-task")
);
const clearButton = /** @type {HTMLButtonElement} */ (
  document.querySelector("#clear-complete")
);
const refreshButton = /** @type {HTMLButtonElement} */ (
  document.querySelector("#refresh")
);
const list = /** @type {HTMLUListElement} */ (
  document.querySelector("#task-list")
);
const empty = /** @type {HTMLElement} */ (document.querySelector("#empty"));
const taskCount = /** @type {HTMLElement} */ (
  document.querySelector("#task-count")
);
const syncStatus = /** @type {HTMLElement} */ (
  document.querySelector("#sync-status")
);
const syncCopy = /** @type {HTMLElement} */ (
  document.querySelector("#sync-copy")
);
const notice = /** @type {HTMLElement} */ (document.querySelector("#notice"));
const noticeTitle = /** @type {HTMLElement} */ (
  document.querySelector("#notice-title")
);
const noticeMessage = /** @type {HTMLElement} */ (
  document.querySelector("#notice-message")
);
const noticeRequest = /** @type {HTMLElement} */ (
  document.querySelector("#notice-request")
);

function setSync(copy, tone = "warning") {
  syncCopy.textContent = copy;
  syncStatus.dataset.tone = tone;
}

function clearNotice() {
  notice.hidden = true;
  noticeRequest.textContent = "";
}

/** @param {unknown} error */
function showError(error) {
  const detail = describeTinkerError(error);
  noticeTitle.textContent = detail.title;
  noticeMessage.textContent = detail.message;
  noticeRequest.textContent = detail.requestId
    ? `Request ${detail.requestId}`
    : "";
  notice.hidden = false;
}

function render() {
  list.textContent = "";
  empty.hidden = tasks.length > 0;
  taskCount.textContent = `${tasks.length} ${
    tasks.length === 1 ? "task" : "tasks"
  }`;
  clearButton.disabled = !tasks.some((entry) =>
    /** @type {{complete?: unknown}} */ (entry.value).complete === true
  );

  for (const entry of tasks) {
    const value = /** @type {{title?: unknown, complete?: unknown, by?: unknown}} */ (
      entry.value
    );
    const item = document.createElement("li");
    item.className = "list-item";
    item.dataset.complete = String(value.complete === true);

    const checkbox = document.createElement("input");
    checkbox.className = "check";
    checkbox.type = "checkbox";
    checkbox.checked = value.complete === true;
    checkbox.setAttribute(
      "aria-label",
      `Mark ${String(value.title ?? "task")} ${
        checkbox.checked ? "incomplete" : "complete"
      }`,
    );
    checkbox.addEventListener("change", () => {
      void updateTask(entry, checkbox.checked);
    });

    const copy = document.createElement("div");
    copy.className = "list-copy";
    const name = document.createElement("strong");
    name.textContent = String(value.title ?? "Untitled task");
    const meta = document.createElement("small");
    meta.textContent = value.by ? `Added by ${String(value.by)}` : "Shared task";
    copy.append(name, meta);

    const remove = document.createElement("button");
    remove.className = "button button-quiet button-danger";
    remove.type = "button";
    remove.textContent = "Delete";
    remove.setAttribute("aria-label", `Delete ${name.textContent}`);
    remove.addEventListener("click", () => void deleteTask(entry));

    item.append(checkbox, copy, remove);
    list.append(item);
  }
}

async function readAllTasks() {
  /** @type {import("@tinkercloud/sdk").KVEntry[]} */
  const entries = [];
  let cursor;
  do {
    const page = await tinker.kv.list({
      prefix,
      limit: 50,
      cursor,
      signal: refreshRequest.signal,
    });
    entries.push(...page.entries);
    cursor = page.next_cursor;
  } while (cursor && entries.length < 500);
  return entries.sort((a, b) => a.updated_at.localeCompare(b.updated_at));
}

async function refreshTasks() {
  refreshRequest?.abort();
  refreshRequest = new AbortController();
  refreshButton.disabled = true;
  setSync("Reading current state");
  try {
    tasks = await readAllTasks();
    render();
    clearNotice();
    setSync("Current state loaded", "success");
  } catch (error) {
    if (!(error instanceof DOMException && error.name === "AbortError")) {
      showError(error);
      setSync("Refresh needed");
    }
  } finally {
    refreshButton.disabled = false;
  }
}

/** @param {import("@tinkercloud/sdk").KVEntry} entry @param {boolean} complete */
async function updateTask(entry, complete) {
  clearNotice();
  try {
    const current = /** @type {{[key: string]: import("@tinkercloud/sdk").JSONValue}} */ (
      entry.value
    );
    await tinker.kv.set(
      entry.key,
      { ...current, complete },
      { expectedVersion: entry.version, signal: pageLifetime.signal },
    );
  } catch (error) {
    showError(error);
    if (error instanceof TinkerVersionConflictError) await refreshTasks();
  }
}

/** @param {import("@tinkercloud/sdk").KVEntry} entry */
async function deleteTask(entry) {
  clearNotice();
  try {
    await tinker.kv.delete(entry.key, {
      expectedVersion: entry.version,
      signal: pageLifetime.signal,
    });
  } catch (error) {
    showError(error);
    if (error instanceof TinkerVersionConflictError) await refreshTasks();
  }
}

taskForm.addEventListener("submit", async (event) => {
  event.preventDefault();
  const title = titleInput.value.trim();
  if (!title || title.length > 120) return;
  addButton.disabled = true;
  clearNotice();
  try {
    const viewer = await tinker.user.current({ signal: pageLifetime.signal });
    const random = new Uint32Array(4);
    crypto.getRandomValues(random);
    const id = Array.from(random, (part) => part.toString(16)).join("-");
    await tinker.kv.set(
      `${prefix}${id}`,
      {
        title,
        complete: false,
        by: viewer.identity.email,
        createdAt: new Date().toISOString(),
      },
      { signal: pageLifetime.signal },
    );
    titleInput.value = "";
    titleInput.focus();
  } catch (error) {
    showError(error);
  } finally {
    addButton.disabled = false;
  }
});

clearButton.addEventListener("click", async () => {
  clearButton.disabled = true;
  clearNotice();
  try {
    for (const entry of tasks.filter((task) =>
      /** @type {{complete?: unknown}} */ (task.value).complete === true
    )) {
      await tinker.kv.delete(entry.key, {
        expectedVersion: entry.version,
        signal: pageLifetime.signal,
      });
    }
  } catch (error) {
    showError(error);
    await refreshTasks();
  } finally {
    clearButton.disabled = false;
  }
});

refreshButton.addEventListener("click", () => void refreshTasks());
document.addEventListener("visibilitychange", () => {
  if (document.visibilityState === "visible") void refreshTasks();
});

async function start() {
  try {
    const [viewer, app, result] = await Promise.all([
      tinker.user.current({ signal: pageLifetime.signal }),
      tinker.app.info({ signal: pageLifetime.signal }),
      tinker.capabilities.list({ signal: pageLifetime.signal }),
    ]);
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

    await refreshTasks();
    if (capabilities.has("live")) {
      stopLive = tinker.live.onKvChange({ prefix }, () => void refreshTasks());
    } else {
      setSync("Manual refresh · live unavailable");
    }
  } catch (error) {
    showError(error);
    setSync("App unavailable");
    taskForm.querySelectorAll("input, button").forEach((control) => {
      if (
        control instanceof HTMLInputElement ||
        control instanceof HTMLButtonElement
      ) {
        control.disabled = true;
      }
    });
  }
}

window.addEventListener("pagehide", () => {
  pageLifetime.abort();
  refreshRequest?.abort();
  stopLive();
});

void start();
