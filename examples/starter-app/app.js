const date = new Date();
const dateKey = [
  date.getFullYear(),
  String(date.getMonth() + 1).padStart(2, "0"),
  String(date.getDate()).padStart(2, "0"),
].join("-");
const storageKey = `tinker-ritual:${dateKey}`;

const today = document.querySelector("#today");
const progress = document.querySelector("#progress");
const progressCopy = document.querySelector("#progress-copy");
const reset = document.querySelector("#reset");
const steps = [...document.querySelectorAll("[data-step]")];

today.dateTime = dateKey;
today.textContent = new Intl.DateTimeFormat(undefined, {
  weekday: "long",
  month: "long",
  day: "numeric",
}).format(date);

function loadState() {
  try {
    const value = JSON.parse(localStorage.getItem(storageKey) ?? "[]");
    return Array.isArray(value) ? new Set(value) : new Set();
  } catch {
    return new Set();
  }
}

function saveState() {
  const complete = steps
    .filter((step) => step.checked)
    .map((step) => step.dataset.step);

  try {
    localStorage.setItem(storageKey, JSON.stringify(complete));
  } catch {
    // The app remains usable if browser storage is unavailable.
  }
}

function render() {
  const complete = steps.filter((step) => step.checked).length;
  const message = `${complete} of ${steps.length} complete`;
  progress.value = complete;
  progress.textContent = message;
  progressCopy.textContent =
    complete === steps.length ? "A clear start. Nicely done." : message;
  document.body.dataset.complete = String(complete === steps.length);
}

const saved = loadState();
for (const step of steps) {
  step.checked = saved.has(step.dataset.step);
  step.addEventListener("change", () => {
    saveState();
    render();
  });
}

reset.addEventListener("click", () => {
  for (const step of steps) {
    step.checked = false;
  }
  try {
    localStorage.removeItem(storageKey);
  } catch {
    // Resetting the visible state still succeeds.
  }
  render();
  steps[0].focus();
});

render();
