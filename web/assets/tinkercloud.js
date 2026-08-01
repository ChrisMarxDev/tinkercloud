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
    return document.querySelector("[data-tinker-toast-region]");
  }

  function initializeQRCodes() {
    var size = 57;
    var quiet = 4;
    var scale = 10;
    var codes = document.querySelectorAll("canvas[data-tinker-qr]");
    codes.forEach(function (canvas) {
      var encoded = canvas.getAttribute("data-tinker-qr");
      var binary;
      try {
        binary = window.atob(encoded || "");
      } catch (_) {
        return;
      }
      if (binary.length !== Math.ceil(size * size / 8)) {
        return;
      }
      var context = canvas.getContext("2d");
      if (!context) {
        return;
      }
      var style = window.getComputedStyle(canvas);
      var ink = style.getPropertyValue("--tinker-qr-ink").trim() || "#000000";
      var surface = style.getPropertyValue("--tinker-qr-surface").trim() || "#ffffff";
      canvas.width = (size + quiet * 2) * scale;
      canvas.height = canvas.width;
      context.imageSmoothingEnabled = false;
      context.fillStyle = surface;
      context.fillRect(0, 0, canvas.width, canvas.height);
      context.fillStyle = ink;
      for (var index = 0; index < size * size; index++) {
        var value = binary.charCodeAt(index >> 3);
        if (value & (1 << (7 - (index & 7)))) {
          var x = index % size;
          var y = Math.floor(index / size);
          context.fillRect((x + quiet) * scale, (y + quiet) * scale, scale, scale);
        }
      }
    });
  }

  function initializeAppFilters() {
    var filters = document.querySelectorAll("[data-tinker-app-filter]");
    filters.forEach(function (filter) {
      var query = filter.querySelector("[data-tinker-app-filter-query]");
      var status = filter.querySelector("[data-tinker-app-filter-status]");
      var count = filter.querySelector("[data-tinker-app-filter-count]");
      var list = filter.parentElement.querySelector("[data-tinker-app-list]");
      var empty = filter.parentElement.querySelector("[data-tinker-app-filter-empty]");
      if (!query || !status || !count || !list || !empty) {
        return;
      }

      var cards = list.querySelectorAll("[data-tinker-app-card]");
      if (!cards.length) {
        return;
      }

      function apply() {
        var search = query.value.trim().toLocaleLowerCase();
        var wantedStatus = status.value;
        var visible = 0;
        cards.forEach(function (card) {
          var slug = (card.getAttribute("data-tinker-app-slug") || "").toLocaleLowerCase();
          var description = (card.getAttribute("data-tinker-app-description") || "").toLocaleLowerCase();
          var appStatus = card.getAttribute("data-tinker-app-status") || "";
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

  // Catalog filtering is deliberately presentation-only. The page contains
  // only server-authorized cards; this helper neither fetches nor persists
  // data, and all cards remain visible when JavaScript is unavailable.
  function initializeCatalogFilters() {
    var filters = document.querySelectorAll("[data-tinker-catalog-filter]");
    filters.forEach(function (filter) {
      var query = filter.querySelector("[data-tinker-catalog-filter-query]");
      var tag = filter.querySelector("[data-tinker-catalog-filter-tag]");
      var count = filter.querySelector("[data-tinker-catalog-filter-count]");
      var list = filter.parentElement.querySelector("[data-tinker-catalog-list]");
      var empty = filter.parentElement.querySelector("[data-tinker-catalog-filter-empty]");
      if (!query || !tag || !count || !list || !empty) {
        return;
      }

      var cards = list.querySelectorAll("[data-tinker-catalog-card]");
      if (!cards.length) {
        return;
      }

      function apply() {
        var search = query.value.trim().toLocaleLowerCase();
        var wantedTag = tag.value;
        var visible = 0;
        cards.forEach(function (card) {
          var slug = (card.getAttribute("data-tinker-catalog-slug") || "").toLocaleLowerCase();
          var description = (card.getAttribute("data-tinker-catalog-description") || "").toLocaleLowerCase();
          var tags = (card.getAttribute("data-tinker-catalog-tags") || "").trim().split(/\s+/);
          var matches = (!search || slug.indexOf(search) >= 0 || description.indexOf(search) >= 0 || tags.join(" ").toLocaleLowerCase().indexOf(search) >= 0) &&
            (!wantedTag || tags.indexOf(wantedTag) >= 0);
          card.hidden = !matches;
          if (matches) {
            visible += 1;
          }
        });
        count.textContent = visible === 1 ? "Showing 1 app." : "Showing " + visible + " apps.";
        empty.hidden = visible !== 0;
      }

      query.addEventListener("input", apply);
      tag.addEventListener("change", apply);
      filter.hidden = false;
      apply();
    });
  }

  // Copy is only a convenience over server-rendered, selectable text. It
  // neither selects an endpoint nor stores credentials, prompts, or state.
  function initializeCopyControls() {
    var controls = document.querySelectorAll("[data-tinker-copy-target]");
    controls.forEach(function (control) {
      var target = document.getElementById(control.getAttribute("data-tinker-copy-target") || "");
      var feedback = document.getElementById(control.getAttribute("data-tinker-copy-feedback") || "");
      if (!target || !navigator.clipboard || typeof navigator.clipboard.writeText !== "function") {
        return;
      }
      control.hidden = false;
      control.addEventListener("click", function () {
        navigator.clipboard.writeText(target.value || target.textContent || "").then(function () {
          if (feedback) {
            feedback.textContent = "Prompt copied.";
          }
        }, function () {
          if (feedback) {
            feedback.textContent = "Copy unavailable. Select the prompt to copy it manually.";
          }
        });
      });
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
    }, cssDuration("--tinker-motion-fast", 140));
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

    item.className = "tinker-toast tinker-toast--" + tone;
    if (tone === "danger") {
      item.setAttribute("role", "alert");
    }

    copy.className = "tinker-toast__copy";
    title.className = "tinker-toast__title";
    title.textContent = settings.title || "Tinkercloud";
    body.className = "tinker-toast__message";
    body.textContent = message;

    close.className = "tinker-toast__close";
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
    dialog.removeAttribute("data-tinker-dialog-state");
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

    dialog.dataset.tinkerDialogState = "closing";
    var duration = cssDuration("--tinker-motion-base", 190);
    var animation = dialog.animate([
      { opacity: 1, transform: "translateY(0) scale(1)" },
      { opacity: 0, transform: "translateY(7px) scale(0.985)" }
    ], {
      duration: duration,
      easing: cssEasing("--tinker-ease-standard", "ease"),
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
      dialog.removeAttribute("data-tinker-dialog-state");
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

    var state = details.dataset.tinkerDisclosureState;
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

    details.dataset.tinkerDisclosureState = opening ? "opening" : "closing";
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
      duration: cssDuration("--tinker-motion-disclosure", 240),
      easing: cssEasing("--tinker-ease-out", "ease-out")
    });

    disclosureAnimations.set(details, animation);
    animation.onfinish = function () {
      disclosureAnimations.delete(details);
      details.open = opening;
      details.style.height = "";
      details.style.overflow = "";
      details.removeAttribute("data-tinker-disclosure-state");
      summary.removeAttribute("aria-expanded");
    };
    return true;
  }

  document.addEventListener("click", function (event) {
    var summary = event.target.closest("summary");
    var details = summary && summary.parentElement;
    if (
      details &&
      details.matches(".tinker-details, .tinker-disclosure-card") &&
      !reducedMotion() &&
      typeof details.animate === "function"
    ) {
      event.preventDefault();
      toggleDisclosure(details);
    }

    var dialogTrigger = event.target.closest("[data-tinker-dialog-open]");
    if (dialogTrigger) {
      openDialog(dialogTrigger.getAttribute("data-tinker-dialog-open"));
    }

    var toastTrigger = event.target.closest("[data-tinker-toast-message]");
    if (toastTrigger) {
      toast(toastTrigger.getAttribute("data-tinker-toast-message"), {
        title: toastTrigger.getAttribute("data-tinker-toast-title") || undefined,
        tone: toastTrigger.getAttribute("data-tinker-toast-tone") || "info",
        duration: toastTrigger.getAttribute("data-tinker-toast-duration") || undefined
      });
    }

    var dialogButton = event.target.closest("button");
    var dialogForm = dialogButton && dialogButton.form;
    var parentDialog = dialogForm && dialogForm.closest("dialog.tinker-dialog");
    if (
      parentDialog &&
      dialogForm.getAttribute("method") &&
      dialogForm.getAttribute("method").toLowerCase() === "dialog"
    ) {
      event.preventDefault();
      closeDialog(parentDialog, dialogButton.value);
    } else if (event.target.matches("dialog.tinker-dialog")) {
      closeDialog(event.target, "backdrop");
    }
  });

  document.addEventListener("cancel", function (event) {
    if (event.target.matches("dialog.tinker-dialog")) {
      event.preventDefault();
      closeDialog(event.target, "cancel");
    }
  }, true);

  initializeQRCodes();
  initializeAppFilters();
  initializeCatalogFilters();
  initializeCopyControls();

  window.TinkerUI = Object.freeze({
    closeDialog: closeDialog,
    openDialog: openDialog,
    toast: toast,
    toggleDisclosure: toggleDisclosure
  });
}());
