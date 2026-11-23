// The theme the panel paints in, and what else must happen before the first
// paint (the place kept across a language change, below).
//
// The stylesheet already follows the operating system. This only adds the
// override: a choice made in Configurações › Aparência, kept in this browser
// alone, applied before the first paint so the page never flashes the other
// palette on its way to the right one. Without this file the system preference
// still decides, and the Aparência section stays hidden rather than sitting
// there doing nothing.
(function () {
  "use strict";

  var KEY = "wamcp-theme";
  var root = document.documentElement;

  function stored() {
    try {
      return window.localStorage.getItem(KEY);
    } catch (error) {
      // A browser with site data blocked has no preference to read.
      return null;
    }
  }

  function apply(choice) {
    if (choice === "light" || choice === "dark") {
      root.setAttribute("data-theme", choice);
      return;
    }
    root.removeAttribute("data-theme");
  }

  function remember(choice) {
    try {
      if (choice === "system") {
        window.localStorage.removeItem(KEY);
        return;
      }
      window.localStorage.setItem(KEY, choice);
    } catch (error) {
      /* The theme still changes for this page view. */
    }
  }

  apply(stored());
  // The Aparência card shows by this mark, before the first paint: unhidden
  // later, it would push everything below it down a card's height.
  root.setAttribute("data-themes", "");

  // ---- after a language change ----
  // The page comes back in the new language with the control just chosen
  // where it was on the screen. Done here, before the first paint, and held
  // while the cards above it fill in, so nothing jumps.
  var ANCHOR = "language-anchor";
  function take() {
    try {
      var raw = window.sessionStorage.getItem(ANCHOR);
      window.sessionStorage.removeItem(ANCHOR);
      var a = raw && JSON.parse(raw);
      return a && /^[A-Za-z-]*$/.test(a.code) && isFinite(a.top) ? a : null;
    } catch (error) {
      return null;
    }
  }
  function switching() {
    try {
      return window.sessionStorage.getItem(ANCHOR) !== null;
    } catch (error) {
      return false;
    }
  }
  // The stylesheet cross-fades panel pages where the browser can; only a
  // language change keeps it, every other page opens as it always did.
  window.addEventListener("pageswap", function (event) {
    if (event.viewTransition && !switching()) event.viewTransition.skipTransition();
  });
  var anchor = take();
  window.addEventListener("pagereveal", function (event) {
    if (event.viewTransition && !anchor) event.viewTransition.skipTransition();
  });
  if (anchor) {
    var control = function () {
      var c = document.querySelector('[data-language-pick="' + anchor.code + '"]') ||
        document.querySelector('input[data-language][value="' + anchor.code + '"]');
      return c && (c.closest("label") || c);
    };
    var held = true;
    var hold = function () {
      var c = held && control();
      if (!c) return;
      var off = c.getBoundingClientRect().top - anchor.top;
      if (Math.abs(off) >= 1) window.scrollBy(0, off);
    };
    var watch = new MutationObserver(hold);
    watch.observe(root, { childList: true, subtree: true, attributes: true, attributeFilter: ["hidden"] });
    var resize = window.ResizeObserver ? new ResizeObserver(hold) : null;
    if (resize) resize.observe(root);
    // Once the person scrolls, or the page has settled, it is theirs again.
    var release = function () {
      if (!held) return;
      held = false;
      watch.disconnect();
      if (resize) resize.disconnect();
      ["wheel", "touchstart", "keydown", "pointerdown"].forEach(function (type) {
        window.removeEventListener(type, release, true);
      });
    };
    ["wheel", "touchstart", "keydown", "pointerdown"].forEach(function (type) {
      window.addEventListener(type, release, { capture: true, passive: true });
    });
    window.addEventListener("load", function () {
      hold();
      window.setTimeout(release, 2000);
    });
  }

  document.addEventListener("DOMContentLoaded", function () {
    var choices = document.querySelectorAll("[data-theme-choice]");
    if (!choices.length) {
      return;
    }
    var current = stored() || "system";
    choices.forEach(function (input) {
      input.checked = input.value === current;
      input.addEventListener("change", function () {
        if (!input.checked) {
          return;
        }
        remember(input.value);
        apply(input.value);
      });
    });
    var holder = document.querySelector("[data-theme-switch]");
    if (holder) {
      holder.hidden = false;
    }
  });
})();
