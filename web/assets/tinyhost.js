(function () {
  "use strict";

  var tones = ["info", "success", "warning", "danger"];
  var disclosureAnimations = new WeakMap();
  var dialogAnimations = new WeakMap();

  function reducedMotion() {
    return window.matchMedia &&
      window.matchMedia("(prefers-reduced-motion: reduce)").matches;
  }

  function cssDuration(name, fallback) {
    var value = window.getComputedStyle(document.documentElement)
      .getPropertyValue(name).trim();
    if (!value) {
      return fallback;
    }
    if (value.endsWith("ms")) {
      return Number.parseFloat(value);
    }
    if (value.endsWith("s")) {
      return Number.parseFloat(value) * 1000;
    }
    return fallback;
  }

  function cssEasing(name, fallback) {
    return window.getComputedStyle(document.documentElement)
      .getPropertyValue(name).trim() || fallback;
  }

  function toastRegion() {
    return document.querySelector("[data-tiny-toast-region]");
  }

  function initializeAppFilters() {
    var filters = document.querySelectorAll("[data-tiny-app-filter]");
    filters.forEach(function (filter) {
      var query = filter.querySelector("[data-tiny-app-filter-query]");
      var status = filter.querySelector("[data-tiny-app-filter-status]");
      var count = filter.querySelector("[data-tiny-app-filter-count]");
      var list = filter.parentElement.querySelector("[data-tiny-app-list]");
      var empty = filter.parentElement.querySelector("[data-tiny-app-filter-empty]");
      if (!query || !status || !count || !list || !empty) {
        return;
      }

      var cards = list.querySelectorAll("[data-tiny-app-card]");
      if (!cards.length) {
        return;
      }

      function apply() {
        var search = query.value.trim().toLocaleLowerCase();
        var wantedStatus = status.value;
        var visible = 0;
        cards.forEach(function (card) {
          var slug = (card.getAttribute("data-tiny-app-slug") || "").toLocaleLowerCase();
          var description = (card.getAttribute("data-tiny-app-description") || "").toLocaleLowerCase();
          var appStatus = card.getAttribute("data-tiny-app-status") || "";
          var matches = (!search || slug.indexOf(search) >= 0 || description.indexOf(search) >= 0) &&
            (!wantedStatus || appStatus === wantedStatus);
          card.hidden = !matches;
          if (matches) {
            visible += 1;
          }
        });
        count.textContent = visible === 1 ? "Showing 1 app." : "Showing " + visible + " apps.";
        empty.hidden = visible !== 0;
      }

      query.addEventListener("input", apply);
      status.addEventListener("change", apply);
      filter.hidden = false;
      apply();
    });
  }

  function removeToast(toast) {
    if (!toast || toast.dataset.leaving === "true") {
      return;
    }
    toast.dataset.leaving = "true";
    if (reducedMotion()) {
      toast.remove();
      return;
    }
    window.setTimeout(function () {
      toast.remove();
    }, cssDuration("--tiny-motion-fast", 140));
  }

  function toast(message, options) {
    var region = toastRegion();
    if (!region || !message) {
      return null;
    }

    var settings = options || {};
    var tone = tones.indexOf(settings.tone) >= 0 ? settings.tone : "info";
    var item = document.createElement("div");
    var copy = document.createElement("div");
    var title = document.createElement("strong");
    var body = document.createElement("p");
    var close = document.createElement("button");

    item.className = "tiny-toast tiny-toast--" + tone;
    if (tone === "danger") {
      item.setAttribute("role", "alert");
    }

    copy.className = "tiny-toast__copy";
    title.className = "tiny-toast__title";
    title.textContent = settings.title || "TinyHost";
    body.className = "tiny-toast__message";
    body.textContent = message;

    close.className = "tiny-toast__close";
    close.type = "button";
    close.setAttribute("aria-label", "Dismiss notification");
    close.textContent = "×";
    close.addEventListener("click", function () {
      removeToast(item);
    });

    copy.append(title, body);
    item.append(copy, close);
    region.append(item);

    var duration = Number(settings.duration);
    if (!Number.isFinite(duration)) {
      duration = 6000;
    }
    if (duration > 0) {
      var timer = window.setTimeout(function () {
        removeToast(item);
      }, Math.max(duration, 3000));
      item.addEventListener("mouseenter", function () {
        window.clearTimeout(timer);
      }, { once: true });
      item.addEventListener("focusin", function () {
        window.clearTimeout(timer);
      }, { once: true });
    }

    return item;
  }

  function openDialog(id) {
    var dialog = document.getElementById(id);
    if (!dialog || dialog.tagName !== "DIALOG") {
      return false;
    }
    var closing = dialogAnimations.get(dialog);
    if (closing) {
      closing.animation.onfinish = null;
      closing.animation.cancel();
      window.clearTimeout(closing.timer);
      dialogAnimations.delete(dialog);
    }
    dialog.removeAttribute("data-tiny-dialog-state");
    if (typeof dialog.showModal === "function") {
      if (!dialog.open) {
        dialog.showModal();
      }
    } else {
      dialog.setAttribute("open", "");
    }
    return true;
  }

  function closeDialog(dialog, returnValue) {
    if (!dialog || !dialog.open) {
      return false;
    }

    var current = dialogAnimations.get(dialog);
    if (current) {
      current.animation.onfinish = null;
      current.animation.cancel();
      window.clearTimeout(current.timer);
    }

    if (reducedMotion() || typeof dialog.animate !== "function") {
      dialog.close(returnValue || "");
      return true;
    }

    dialog.dataset.tinyDialogState = "closing";
    var duration = cssDuration("--tiny-motion-base", 190);
    var animation = dialog.animate([
      { opacity: 1, transform: "translateY(0) scale(1)" },
      { opacity: 0, transform: "translateY(7px) scale(0.985)" }
    ], {
      duration: duration,
      easing: cssEasing("--tiny-ease-standard", "ease"),
      fill: "both"
    });

    var record = { animation: animation, timer: 0 };
    function finish() {
      if (dialogAnimations.get(dialog) !== record) {
        return;
      }
      dialogAnimations.delete(dialog);
      window.clearTimeout(record.timer);
      dialog.close(returnValue || "");
      dialog.removeAttribute("data-tiny-dialog-state");
    }

    record.timer = window.setTimeout(finish, duration + 50);
    dialogAnimations.set(dialog, record);
    animation.onfinish = finish;
    return true;
  }

  function disclosureHeight(details) {
    var style = window.getComputedStyle(details);
    return details.scrollHeight +
      Number.parseFloat(style.borderTopWidth || 0) +
      Number.parseFloat(style.borderBottomWidth || 0);
  }

  function toggleDisclosure(details) {
    var summary = details.querySelector(":scope > summary");
    if (!summary) {
      return false;
    }

    var state = details.dataset.tinyDisclosureState;
    var opening = !details.open || state === "closing";
    var startHeight = details.getBoundingClientRect().height;
    var current = disclosureAnimations.get(details);

    if (current) {
      current.onfinish = null;
      current.cancel();
    }

    if (opening && !details.open) {
      details.open = true;
    }

    details.dataset.tinyDisclosureState = opening ? "opening" : "closing";
    summary.setAttribute("aria-expanded", String(opening));
    details.style.height = startHeight + "px";
    details.style.overflow = "clip";

    var endHeight;
    if (opening) {
      details.style.height = "auto";
      endHeight = disclosureHeight(details);
      details.style.height = startHeight + "px";
    } else {
      var style = window.getComputedStyle(details);
      endHeight = summary.getBoundingClientRect().height +
        Number.parseFloat(style.borderTopWidth || 0) +
        Number.parseFloat(style.borderBottomWidth || 0);
    }

    var animation = details.animate([
      { height: startHeight + "px" },
      { height: endHeight + "px" }
    ], {
      duration: cssDuration("--tiny-motion-disclosure", 240),
      easing: cssEasing("--tiny-ease-out", "ease-out")
    });

    disclosureAnimations.set(details, animation);
    animation.onfinish = function () {
      disclosureAnimations.delete(details);
      details.open = opening;
      details.style.height = "";
      details.style.overflow = "";
      details.removeAttribute("data-tiny-disclosure-state");
      summary.removeAttribute("aria-expanded");
    };
    return true;
  }

  document.addEventListener("click", function (event) {
    var summary = event.target.closest("summary");
    var details = summary && summary.parentElement;
    if (
      details &&
      details.matches(".tiny-details, .tiny-disclosure-card") &&
      !reducedMotion() &&
      typeof details.animate === "function"
    ) {
      event.preventDefault();
      toggleDisclosure(details);
    }

    var dialogTrigger = event.target.closest("[data-tiny-dialog-open]");
    if (dialogTrigger) {
      openDialog(dialogTrigger.getAttribute("data-tiny-dialog-open"));
    }

    var toastTrigger = event.target.closest("[data-tiny-toast-message]");
    if (toastTrigger) {
      toast(toastTrigger.getAttribute("data-tiny-toast-message"), {
        title: toastTrigger.getAttribute("data-tiny-toast-title") || undefined,
        tone: toastTrigger.getAttribute("data-tiny-toast-tone") || "info",
        duration: toastTrigger.getAttribute("data-tiny-toast-duration") || undefined
      });
    }

    var dialogButton = event.target.closest("button");
    var dialogForm = dialogButton && dialogButton.form;
    var parentDialog = dialogForm && dialogForm.closest("dialog.tiny-dialog");
    if (
      parentDialog &&
      dialogForm.getAttribute("method") &&
      dialogForm.getAttribute("method").toLowerCase() === "dialog"
    ) {
      event.preventDefault();
      closeDialog(parentDialog, dialogButton.value);
    } else if (event.target.matches("dialog.tiny-dialog")) {
      closeDialog(event.target, "backdrop");
    }
  });

  document.addEventListener("cancel", function (event) {
    if (event.target.matches("dialog.tiny-dialog")) {
      event.preventDefault();
      closeDialog(event.target, "cancel");
    }
  }, true);

  initializeAppFilters();

  window.TinyUI = Object.freeze({
    closeDialog: closeDialog,
    openDialog: openDialog,
    toast: toast,
    toggleDisclosure: toggleDisclosure
  });
}());
