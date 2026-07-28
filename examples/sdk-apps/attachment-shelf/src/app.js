// @ts-check
import { TinyCapabilityUnavailableError, tiny } from "@tinyhost/sdk";
import { describeTinyError } from "../../shared/errors.js";

const pageLifetime = new AbortController();
let request;
let nextCursor;
/** @type {import("@tinyhost/sdk").TinyBlob[]} */
let blobs = [];

const form = /** @type {HTMLFormElement} */ (document.querySelector("#upload-form"));
const fileInput = /** @type {HTMLInputElement} */ (document.querySelector("#file"));
const upload = /** @type {HTMLButtonElement} */ (document.querySelector("#upload"));
const cancel = /** @type {HTMLButtonElement} */ (document.querySelector("#cancel"));
const refresh = /** @type {HTMLButtonElement} */ (document.querySelector("#refresh"));
const more = /** @type {HTMLButtonElement} */ (document.querySelector("#more"));
const moreWrap = /** @type {HTMLElement} */ (document.querySelector("#more-wrap"));
const list = /** @type {HTMLUListElement} */ (document.querySelector("#blob-list"));
const empty = /** @type {HTMLElement} */ (document.querySelector("#empty"));
const count = /** @type {HTMLElement} */ (document.querySelector("#blob-count"));
const status = /** @type {HTMLElement} */ (document.querySelector("#storage-status"));
const statusCopy = /** @type {HTMLElement} */ (document.querySelector("#storage-copy"));
const notice = /** @type {HTMLElement} */ (document.querySelector("#notice"));
const noticeTitle = /** @type {HTMLElement} */ (document.querySelector("#notice-title"));
const noticeMessage = /** @type {HTMLElement} */ (document.querySelector("#notice-message"));
const noticeRequest = /** @type {HTMLElement} */ (document.querySelector("#notice-request"));

function setStatus(copy, tone = "warning") {
  statusCopy.textContent = copy;
  status.dataset.tone = tone;
}
function clearNotice() {
  notice.hidden = true;
  noticeRequest.textContent = "";
}
/** @param {unknown} error */
function showError(error) {
  const detail = describeTinyError(error);
  noticeTitle.textContent = detail.title;
  noticeMessage.textContent = detail.message;
  noticeRequest.textContent = detail.requestId ? `Request ${detail.requestId}` : "";
  notice.hidden = false;
}
function size(bytes) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${Math.ceil(bytes / 1024)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}
function render() {
  list.textContent = "";
  empty.hidden = blobs.length > 0;
  count.textContent = `${blobs.length} ${blobs.length === 1 ? "attachment" : "attachments"}`;
  moreWrap.hidden = !nextCursor;
  for (const blob of blobs) {
    const item = document.createElement("li");
    item.className = "list-item";
    const copy = document.createElement("div");
    copy.className = "list-copy";
    const name = document.createElement("strong");
    name.textContent = blob.name;
    const meta = document.createElement("small");
    meta.textContent = `${size(blob.size)} · ${blob.contentType || "file"}`;
    copy.append(name, meta);
    const actions = document.createElement("div");
    actions.className = "button-row";
    const download = document.createElement("button");
    download.className = "button button-quiet";
    download.type = "button";
    download.textContent = "Download";
    download.addEventListener("click", () => void downloadBlob(blob));
    const remove = document.createElement("button");
    remove.className = "button button-quiet button-danger";
    remove.type = "button";
    remove.textContent = "Delete";
    remove.addEventListener("click", () => void deleteBlob(blob));
    actions.append(download, remove);
    item.append(copy, actions);
    list.append(item);
  }
}
async function load(reset) {
  request?.abort();
  request = new AbortController();
  refresh.disabled = true;
  more.disabled = true;
  setStatus("Reading current attachments");
  try {
    const page = await tiny.blobs.list({
      cursor: reset ? undefined : nextCursor,
      limit: 25,
      signal: request.signal,
    });
    blobs = reset ? page.blobs : [...blobs, ...page.blobs];
    nextCursor = page.nextCursor;
    render();
    clearNotice();
    setStatus("Attachments available", "success");
  } catch (error) {
    if (!(error instanceof DOMException && error.name === "AbortError")) {
      showError(error);
      setStatus("Refresh needed");
    }
  } finally {
    refresh.disabled = false;
    more.disabled = false;
  }
}
/** @param {import("@tinyhost/sdk").TinyBlob} blob */
async function downloadBlob(blob) {
  try {
    const bytes = await tiny.blobs.get(blob.id, { signal: pageLifetime.signal });
    if (!bytes) {
      await load(true);
      return;
    }
    const href = URL.createObjectURL(bytes);
    const anchor = document.createElement("a");
    anchor.href = href;
    anchor.download = blob.name;
    anchor.click();
    window.setTimeout(() => URL.revokeObjectURL(href), 0);
  } catch (error) {
    showError(error);
  }
}
/** @param {import("@tinyhost/sdk").TinyBlob} blob */
async function deleteBlob(blob) {
  clearNotice();
  try {
    await tiny.blobs.delete(blob.id, { signal: pageLifetime.signal });
    await load(true);
  } catch (error) {
    showError(error);
  }
}

form.addEventListener("submit", async (event) => {
  event.preventDefault();
  const file = fileInput.files?.[0];
  if (!file) return;
  request?.abort();
  request = new AbortController();
  upload.disabled = true;
  cancel.disabled = false;
  clearNotice();
  setStatus("Uploading attachment");
  try {
    await tiny.blobs.upload(file, { signal: request.signal });
    fileInput.value = "";
    await load(true);
  } catch (error) {
    if (!(error instanceof DOMException && error.name === "AbortError")) showError(error);
    setStatus("Upload did not finish");
  } finally {
    upload.disabled = false;
    cancel.disabled = true;
  }
});
cancel.addEventListener("click", () => request?.abort());
refresh.addEventListener("click", () => void load(true));
more.addEventListener("click", () => void load(false));

async function start() {
  try {
    const [viewer, app, result] = await Promise.all([
      tiny.user.current({ signal: pageLifetime.signal }),
      tiny.app.info({ signal: pageLifetime.signal }),
      tiny.capabilities.list({ signal: pageLifetime.signal }),
    ]);
    document.querySelector("#viewer-email").textContent = viewer.identity.email;
    document.querySelector("#viewer-avatar").textContent = viewer.identity.email.slice(0, 1).toUpperCase();
    document.querySelector("#app-name").textContent = `${app.slug} · private`;
    if (!result.capabilities.some((capability) => capability.name === "blobs")) {
      throw new TinyCapabilityUnavailableError(
        "This example requires blob storage.",
        "capability_unavailable",
      );
    }
    await load(true);
  } catch (error) {
    showError(error);
    setStatus("Storage unavailable");
    form.querySelectorAll("input, button").forEach((control) => {
      if (control instanceof HTMLInputElement || control instanceof HTMLButtonElement) control.disabled = true;
    });
  }
}
window.addEventListener("pagehide", () => {
  pageLifetime.abort();
  request?.abort();
});
void start();
