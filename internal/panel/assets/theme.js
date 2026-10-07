// The theme the panel paints in.
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
