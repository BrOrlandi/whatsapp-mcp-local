// The panel's dialogs, on every page, and the cards of Configurações that
// read and change their state through the local API, the same one scripts
// use (docs/api.md), so the page and a program always see the same thing.
// Each part does nothing on a page without its card.
(function () {
  "use strict";

  function request(method, path, body) {
    var options = { method: method, credentials: "same-origin", headers: { Accept: "application/json" } };
    if (method !== "GET") {
      options.headers["Content-Type"] = "application/json";
      options.body = JSON.stringify(body || {});
    }
    return fetch(path, options).then(function (response) {
      return response.json().catch(function () { return {}; }).then(function (data) {
        if (!response.ok) throw new Error(data.error || t("erro %d", response.status));
        return data;
      });
    });
  }

  function el(tag, cls, text) {
    var node = document.createElement(tag);
    if (cls) node.className = cls;
    if (text !== undefined && text !== null) node.textContent = text;
    return node;
  }

  function button(text, cls) {
    var b = el("button", "btn btn--small " + (cls || "btn--ghost"), text);
    b.type = "button";
    return b;
  }

  // A button that acts on the second click, for what cannot be undone.
  function confirming(b, confirmText, act) {
    var original = "", armed = false, timer = null;
    b.addEventListener("click", function () {
      if (!armed) {
        armed = true;
        original = b.textContent;
        b.textContent = confirmText;
        b.classList.add("btn--danger");
        timer = window.setTimeout(function () { armed = false; b.textContent = original; b.classList.remove("btn--danger"); }, 4000);
        return;
      }
      window.clearTimeout(timer);
      armed = false;
      act();
    });
  }

  function size(bytes) {
    if (!bytes) return "0 MB";
    var units = ["bytes", "KB", "MB", "GB"], i = 0, n = bytes;
    while (n >= 1024 && i < units.length - 1) { n /= 1024; i++; }
    var digits = i === 0 ? 0 : (n < 10 ? 1 : 0);
    return Number(i === 0 ? n : n.toFixed(digits)).toLocaleString(document.documentElement.lang, { useGrouping: false, minimumFractionDigits: digits, maximumFractionDigits: digits }) + " " + units[i];
  }

  function ago(iso) {
    var s = Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000);
    if (s < 60) return t("agora há pouco");
    if (s < 3600) return t("há %d min", Math.floor(s / 60));
    if (s < 86400) return t("há %d h", Math.floor(s / 3600));
    var d = Math.floor(s / 86400);
    return d === 1 ? t("há 1 dia") : t("há %d dias", d);
  }

  var visible = function (node) { return node && node.isConnected && node.offsetParent !== null; };

  // A dialog that opens through the address (#id), like the panel's other
  // dialogs, so the page's own reload waits while it is open and Back closes
  // it. Esc closes it, Tab stays inside it, and on close the focus goes back
  // to what opened it. Opening and closing never move the page: a new
  // address scrolls to its element (the dialog, or the card its × points
  // at), and the page goes back to where it was before that jump. Links to
  // the dialog, and its × and Cancelar (links to the card, so they work
  // without a script too), go through open and close. o.closeTo is the hash
  // it leaves for; o.focus puts the focus inside; o.reset runs when it opens
  // from a link or the address alone (or Forward); o.fallback is the focus
  // target when the opener is gone.
  function modal(dialog, o) {
    var HASH = "#" + dialog.id;
    var returnTo = null; // finds what gets the focus back on close
    var keep = null; // where the page was before the address changed
    var pushed = false; // opened here, so closing is going back
    var leaving = false; // going back, not there yet
    var isOpen = function () { return window.location.hash === HASH; };
    // The card the address names loses its id for that moment, so the
    // browser finds nothing to scroll to (and nothing to scroll back to
    // later); a dialog keeps its own, which :target needs.
    var go = function (hash) {
      keep = { x: window.scrollX, y: window.scrollY };
      var card = hash.length > 1 && document.getElementById(hash.slice(1));
      if (card && card.classList.contains("overlay")) card = null;
      if (card) card.id = "";
      window.location.hash = hash;
      if (card) card.id = hash.slice(1);
      window.scrollTo(keep.x, keep.y);
    };
    var open = function (back) {
      returnTo = back;
      if (isOpen()) { o.focus(); return; }
      go(HASH);
      pushed = true;
    };
    // Closing what this page opened goes back in the history, which leaves
    // the page where it was (the address of the card would scroll to it,
    // and the browser may do that after the script restored the place); a
    // dialog reached by its address goes to the card.
    var close = function () {
      if (!isOpen() || leaving) return;
      if (pushed) {
        pushed = false;
        leaving = true;
        keep = { x: window.scrollX, y: window.scrollY };
        window.history.back();
        return;
      }
      go(o.closeTo);
    };
    var wasOpen = isOpen();
    window.addEventListener("hashchange", function () {
      if (keep) {
        window.scrollTo(keep.x, keep.y);
        keep = null;
      }
      var now = isOpen();
      if (now && !wasOpen) {
        if (!returnTo) o.reset();
        o.focus();
      } else if (!now && wasOpen) {
        pushed = leaving = false;
        var target = returnTo && returnTo();
        returnTo = null;
        if (!visible(target)) target = o.fallback();
        if (visible(target)) target.focus({ preventScroll: true });
      }
      wasOpen = now;
    });
    if (wasOpen) { o.reset(); o.focus(); }

    document.addEventListener("click", function (event) {
      if (event.defaultPrevented || event.button !== 0 || event.metaKey || event.ctrlKey || event.shiftKey || event.altKey) return;
      var a = event.target.closest && event.target.closest("a[href]");
      if (!a) return;
      var href = a.getAttribute("href");
      if (href === HASH && !dialog.contains(a)) {
        event.preventDefault();
        if (!isOpen()) o.reset();
        open(function () { return a; });
      } else if (href === o.closeTo && dialog.contains(a) && isOpen()) {
        event.preventDefault();
        close();
      }
    });
    document.addEventListener("keydown", function (event) {
      if (!isOpen()) return;
      if (event.key === "Escape") { event.preventDefault(); close(); return; }
      if (event.key !== "Tab") return;
      // Keep the keyboard inside the dialog while it is open.
      var inside = Array.prototype.filter.call(dialog.querySelectorAll("a[href], button, input"), function (n) { return !n.disabled && visible(n); });
      if (!inside.length) return;
      var first = inside[0], last = inside[inside.length - 1];
      if (event.shiftKey && document.activeElement === first) { event.preventDefault(); last.focus(); }
      else if (!event.shiftKey && document.activeElement === last) { event.preventDefault(); first.focus(); }
    });
    // .returnTo changes what gets the focus on close, while it is open.
    return { isOpen: isOpen, open: open, close: close, returnTo: function (back) { returnTo = back; } };
  }

  // ---- the dialogs with a plain form ----
  // Renaming, disconnecting, the port, a new key: forms posted to the server,
  // marked data-modal. They get the same keyboard as the dialogs below (Esc,
  // Tab inside, the focus back on what opened them, the page where it was). A red button with data-confirm="#id" asks once more,
  // in that box inside the dialog, before it acts; without a script it acts
  // at once, the dialog being the step that asked.
  document.querySelectorAll(".overlay[data-modal]").forEach(function (dialog) {
    var x = dialog.querySelector(".dialog__close");
    var openers = function () {
      return Array.prototype.filter.call(document.querySelectorAll('a[href="#' + dialog.id + '"]'), function (a) { return !dialog.contains(a); });
    };
    var confirms = dialog.querySelectorAll("[data-confirm]");
    var asking = function (b, on) {
      var box = dialog.querySelector(b.getAttribute("data-confirm"));
      var actions = b.closest(".dialog__actions");
      if (box) box.hidden = !on;
      if (actions) actions.hidden = on;
      return box;
    };
    modal(dialog, {
      closeTo: (x && x.getAttribute("href")) || "#",
      focus: function () {
        var first = dialog.querySelector("input:not([type=hidden]), .dialog__actions .btn:not(.btn--danger), .actions .btn");
        if (first) first.focus({ preventScroll: true });
      },
      reset: function () {
        dialog.querySelectorAll("form").forEach(function (f) { f.reset(); });
        Array.prototype.forEach.call(confirms, function (b) { asking(b, false); });
      },
      fallback: function () { return openers().filter(visible)[0]; }
    });
    Array.prototype.forEach.call(confirms, function (b) {
      b.addEventListener("click", function (event) {
        var box = asking(b, true);
        if (!box) return;
        event.preventDefault();
        var no = box.querySelector("[data-confirm-no]");
        if (no) {
          no.onclick = function () { asking(b, false); b.focus(); };
          no.focus();
        }
      });
    });
  });

  // ---- webhooks ----
  (function () {
    var card = document.querySelector("[data-webhooks]");
    var dialog = document.querySelector("[data-webhook-dialog]");
    if (!card || !dialog) return;
    var list = card.querySelector("[data-webhooks-list]");
    var empty = card.querySelector("[data-webhooks-empty]");
    var loading = card.querySelector("[data-webhooks-loading]");
    var addBar = card.querySelector("[data-webhooks-add]");
    var secretBox = card.querySelector("[data-webhook-secret]");
    var intro = card.querySelector("[data-webhooks-intro]");
    var form = dialog.querySelector("[data-webhook-form]");
    var formError = dialog.querySelector("[data-webhook-error]");
    var title = dialog.querySelector("[data-webhook-dialog-title]");
    var submit = dialog.querySelector("[data-webhook-submit]");
    var urlField = form.querySelector("[data-webhook-url]");
    var ownBox = form.querySelector("[data-webhook-own]");
    var eventBoxes = form.querySelectorAll("[data-webhook-event]");
    var formActions = dialog.querySelector("[data-webhook-actions]");
    var del = dialog.querySelector("[data-webhook-delete]");
    var delConfirm = dialog.querySelector("[data-webhook-delete-confirm]");
    var delYes = dialog.querySelector("[data-webhook-delete-yes]");
    var delNo = dialog.querySelector("[data-webhook-delete-cancel]");
    var testIcon = card.querySelector("[data-webhook-test-icon]");
    // With no webhook the card only says the feature exists; once there is
    // one, a button opens the same dialog to add more.
    var count = 0;
    // While a new webhook's key is on screen, the box's own Finalizar is
    // the way on: no button to add another until it is closed.
    var layout = function () {
      var key = !secretBox.hidden;
      empty.hidden = count > 0 || key;
      intro.hidden = !count;
      addBar.hidden = !count || key;
    };
    var EVENTS = { message: t("Mensagens"), reaction: t("Reações"), receipt: t("Entregas e leituras"), online: t("De volta on-line") };
    // What a row last said (a test's result, an error), kept across the
    // refreshes that redraw the list.
    var notes = {};

    // ---- the add/edit dialog ----
    var editing = null; // the webhook being changed; null when adding

    // Deleting lives in the dialog, for a webhook that exists, and asks
    // once more before it happens.
    var asking = function (on) {
      delConfirm.hidden = !on;
      formActions.hidden = on;
    };
    var fill = function (w) {
      editing = w || null;
      form.reset();
      formError.hidden = true;
      asking(false);
      del.hidden = !w;
      del.disabled = delYes.disabled = false;
      title.textContent = w ? t("Editar webhook") : t("Adicionar webhook");
      submit.textContent = w ? t("Salvar") : t("Adicionar webhook");
      if (!w) return;
      urlField.value = w.url;
      eventBoxes.forEach(function (box) { box.checked = (w.events || []).indexOf(box.value) >= 0; });
      ownBox.checked = !!w.include_own;
    };
    var dlg = modal(dialog, {
      closeTo: "#" + card.id,
      focus: function () { urlField.focus(); },
      reset: function () { fill(null); },
      fallback: function () { return visible(addBar) ? addBar.querySelector("button") : card.querySelector("[data-webhook-start]"); }
    });
    var isOpen = dlg.isOpen, close = dlg.close;
    var open = function (w, back) {
      fill(w);
      dlg.open(back);
    };

    del.addEventListener("click", function () {
      if (!editing) return;
      formError.hidden = true;
      asking(true);
      delNo.focus();
    });
    delNo.addEventListener("click", function () {
      asking(false);
      del.focus();
    });
    delYes.addEventListener("click", function () {
      var w = editing;
      if (!w) return;
      delYes.disabled = true;
      request("DELETE", "/api/webhooks/" + w.id).then(function () {
        delete notes[w.id];
        return load().then(function () {
          editing = null;
          form.reset();
          close();
        });
      }).catch(function (e) {
        asking(false);
        formError.textContent = e.message;
        formError.hidden = false;
      }).then(function () { delYes.disabled = false; });
    });

    var row = function (w) {
      var li = el("li", "row hook" + (w.enabled ? "" : " hook--off"));
      var main = el("span", "row__main");
      main.appendChild(el("span", "row__title mono", w.url));
      var names = Object.keys(EVENTS).filter(function (e) { return (w.events || []).indexOf(e) >= 0; }).map(function (e) { return EVENTS[e]; });
      var what = t("Eventos: %s", names.join(" · "));
      if (w.include_own) what += " · " + t("inclui as suas");
      if (w.chats && w.chats.length) what += " · " + (w.chats.length === 1 ? t("1 conversa") : t("%d conversas", w.chats.length));
      main.appendChild(el("span", "row__meta", what));
      // The state lines change with the switch: they get their own place,
      // under the controls on a phone, so the switch does not move.
      var state = el("span", "hook__state");
      var status;
      if (!w.enabled && w.disabled_reason) status = t("Desligado sozinho %s: %s", ago(w.disabled_at), w.disabled_reason);
      else if (!w.enabled) status = t("Não recebe avisos enquanto estiver desligado.");
      else if (w.last_success_at) status = t("Última entrega %s", ago(w.last_success_at)) + " · " + (w.delivered === 1 ? t("1 entregue") : t("%d entregues", w.delivered));
      else status = t("Nenhuma mensagem entregue ainda.");
      if (w.queued) status += " · " + t("%d esperando", w.queued);
      state.appendChild(el("span", "row__meta", status));
      var note = el("span", "row__meta", notes[w.id] || "");
      note.hidden = !notes[w.id];
      state.appendChild(note);
      var say = function (text) { notes[w.id] = text; note.textContent = text; note.hidden = !text; };
      li.appendChild(main);
      li.appendChild(state);

      // On and off is a switch, not a button: what it shows is the state.
      var controls = el("span", "hook__controls");
      var toggle = el("button", "switch");
      toggle.type = "button";
      toggle.setAttribute("role", "switch");
      toggle.setAttribute("aria-checked", w.enabled ? "true" : "false");
      toggle.setAttribute("aria-label", t("Mandar avisos para %s", w.url));
      toggle.setAttribute("data-webhook-switch", w.id);
      toggle.appendChild(el("span", "switch__track")).setAttribute("aria-hidden", "true");
      // Both words, the other one hidden: the label keeps the longer one's
      // width, so nothing beside it moves when it flips.
      var label = toggle.appendChild(el("span", "switch__label"));
      label.setAttribute("aria-hidden", "true");
      label.appendChild(el("span", "switch__on", t("Ligado")));
      label.appendChild(el("span", "switch__off", t("Desligado")));
      toggle.addEventListener("click", function () {
        toggle.disabled = true;
        request("PATCH", "/api/webhooks/" + w.id, { enabled: !w.enabled }).then(load).catch(function (e) {
          say(e.message); toggle.disabled = false;
        });
      });
      var test = button(t("Testar"), "btn--quiet hook__test");
      test.setAttribute("data-webhook-test", w.id);
      if (testIcon && testIcon.content.firstElementChild) test.insertBefore(testIcon.content.firstElementChild.cloneNode(true), test.firstChild);
      test.addEventListener("click", function () {
        test.disabled = true;
        say(t("Mandando um aviso de teste…"));
        request("POST", "/api/webhooks/" + w.id + "/test").then(function (r) {
          say(r.ok ? t("O teste chegou, em %d ms.", r.ms) : t("O teste não chegou: %s.", r.result || r.error));
        }).catch(function (e) { say(e.message); }).then(function () { test.disabled = false; });
      });
      var edit = button(t("Editar"), "btn--ghost hook__edit");
      edit.setAttribute("data-webhook-edit", w.id);
      edit.addEventListener("click", function () {
        open(w, function () { return list.querySelector('[data-webhook-edit="' + w.id + '"]'); });
      });
      controls.appendChild(toggle);
      controls.appendChild(test);
      controls.appendChild(edit);
      li.appendChild(controls);
      return li;
    };

    var load = function () {
      return request("GET", "/api/webhooks").then(function (data) {
        loading.hidden = true;
        card.querySelector("[data-webhooks-unavailable]").hidden = data.available !== false;
        // Redrawing the list must not drop the keyboard: what had the
        // focus (a switch just flipped, say) gets it back.
        var had = null, active = document.activeElement;
        if (active && list.contains(active)) {
          ["data-webhook-switch", "data-webhook-test", "data-webhook-edit"].forEach(function (a) {
            if (active.hasAttribute(a)) had = "[" + a + '="' + active.getAttribute(a) + '"]';
          });
        }
        list.textContent = "";
        (data.webhooks || []).forEach(function (w) { list.appendChild(row(w)); });
        if (had) { var again = list.querySelector(had); if (again) again.focus(); }
        count = (data.webhooks || []).length;
        list.hidden = !count;
        layout();
      }).catch(function (e) {
        loading.hidden = false;
        loading.textContent = t("Não foi possível ler os webhooks: %s", e.message);
      });
    };

    form.addEventListener("submit", function (event) {
      event.preventDefault();
      formError.hidden = true;
      var events = [];
      eventBoxes.forEach(function (box) { if (box.checked) events.push(box.value); });
      if (!events.length) {
        formError.textContent = t("Escolha pelo menos um evento.");
        formError.hidden = false;
        eventBoxes[0].focus();
        return;
      }
      // Only what the dialog shows: a filter of conversations set by a
      // script, or whether it is on, stays as it is.
      var body = { url: urlField.value.trim(), events: events, include_own: ownBox.checked };
      var was = editing;
      submit.disabled = true;
      (was ? request("PATCH", "/api/webhooks/" + was.id, body) : request("POST", "/api/webhooks", body)).then(function (r) {
        if (!was) {
          secretBox.querySelector("[data-webhook-secret-value]").textContent = r.secret;
          secretBox.hidden = false;
          layout();
          dlg.returnTo(function () { return copyKey; });
        }
        return load().then(function () {
          editing = null;
          form.reset();
          close();
          if (!was) secretBox.scrollIntoView({ block: "nearest" });
        });
      }).catch(function (e) {
        formError.textContent = e.message;
        formError.hidden = false;
      }).then(function () { submit.disabled = false; });
    });

    var start = card.querySelector("[data-webhook-start]");
    start.addEventListener("click", function () { open(null, function () { return start; }); });
    var add = addBar.querySelector("[data-webhook-add]");
    add.addEventListener("click", function () { open(null, function () { return add; }); });

    var copyKey = secretBox.querySelector("[data-webhook-secret-copy]");
    copyKey.addEventListener("click", function () {
      var value = secretBox.querySelector("[data-webhook-secret-value]").textContent;
      if (!navigator.clipboard) { copyKey.textContent = t("Selecione e copie"); return; }
      navigator.clipboard.writeText(value).then(function () { copyKey.textContent = t("Copiada"); }, function () { copyKey.textContent = t("Selecione e copie"); });
    });
    // The documentation opens beside the key, never in its place: in a new
    // tab, or, in the desktop app, in the browser (it sends full addresses
    // there).
    var docs = secretBox.querySelector("[data-webhook-secret-docs]");
    if (docs) docs.href = new URL(docs.getAttribute("href"), window.location.href).href;
    secretBox.querySelector("[data-webhook-secret-close]").addEventListener("click", function () {
      secretBox.hidden = true;
      secretBox.querySelector("[data-webhook-secret-value]").textContent = "";
      copyKey.textContent = t("Copiar a chave");
      layout();
      var back = visible(addBar) ? addBar.querySelector("button") : start;
      if (visible(back)) back.focus();
    });

    load();
    // A webhook can be turned off by itself, or changed by a script: follow
    // it, but not under an open dialog, whose button would vanish.
    window.setInterval(function () { if (!document.hidden && !isOpen()) load(); }, 30000);
  })();

  // ---- logs for support ----
  // The dialog's form is a plain GET that downloads the file without this
  // script; with it, the file goes through the API, which in the desktop
  // app saves it in Downloads (its window cannot download) and elsewhere
  // answers with the address to follow. The dialog stays open, for the
  // support address.
  (function () {
    var dialog = document.querySelector("[data-logs-dialog]");
    var card = document.getElementById("logs");
    if (!dialog || !card) return;
    var opener = card.querySelector("[data-logs-open]");
    var form = dialog.querySelector("[data-logs-form]");
    var submit = form.querySelector("[data-logs-submit]");
    var note = form.querySelector("[data-logs-note]");
    var error = form.querySelector("[data-logs-error]");
    var cancel = form.querySelector("[data-logs-cancel]");
    var show = function (text) { note.textContent = text; note.hidden = !text; };
    var reset = function () {
      form.reset();
      show("");
      error.hidden = true;
      cancel.textContent = t("Cancelar");
    };
    var checked = function () { return form.querySelector("input[name=horas]:checked"); };
    var dlg = modal(dialog, {
      closeTo: "#" + card.id,
      focus: function () { (checked() || submit).focus(); },
      reset: reset,
      fallback: function () { return opener; }
    });
    opener.addEventListener("click", function (event) {
      event.preventDefault();
      reset();
      dlg.open(function () { return opener; });
    });
    form.addEventListener("submit", function (event) {
      event.preventDefault();
      var box = checked();
      error.hidden = true;
      show(t("Gerando o arquivo…"));
      submit.disabled = true;
      request("POST", "/api/diagnostico", { horas: Number(box ? box.value : 72) }).then(function (data) {
        if (data.url) {
          window.location.href = data.url;
          show(t("O arquivo foi baixado para a pasta de downloads."));
        } else {
          show(data.note || data.name || "");
        }
        cancel.textContent = t("Fechar");
      }).catch(function (e) {
        show("");
        error.textContent = e.message;
        error.hidden = false;
      }).then(function () { submit.disabled = false; });
    });
  })();

  // ---- audio transcription ----
  // The card says whether it works and where voice notes go; the dialog
  // picks the place and what it needs, saves it and tests it at once. A
  // place that does not answer keeps the dialog open with what to do. The
  // card follows a saved mode and an install as they happen, in place.
  (function () {
    var box = document.querySelector("[data-asr-remote]");
    var card = document.getElementById("transcricao");
    if (!box || !card) return;
    var server = card.hasAttribute("data-asr-server");
    var mode = card.getAttribute("data-asr-mode") || "local";
    var supported = card.hasAttribute("data-asr-supported");
    var ready = card.hasAttribute("data-asr-local-ready");
    // Installed, but the engine does not start on this computer (a Windows
    // without the Visual C++ runtime): the card points at the other places.
    var unavailable = card.hasAttribute("data-asr-unavailable");
    var state = box.querySelector("[data-asr-remote-state]");
    var opener = box.querySelector("[data-asr-open]");
    var install = card.querySelector("[data-asr-install]");
    var progress = card.querySelector("[data-asr-progress]");
    var step = card.querySelector("[data-asr-step]");
    var installError = card.querySelector("[data-asr-install-error]");
    var hide = function (sel, on) { var n = card.querySelector(sel); if (n) n.hidden = on; };
    // One main action: installing while voice notes stay here and nothing
    // is installed; otherwise changing where they go.
    var weigh = function (b, main) {
      if (!b) return;
      b.classList.toggle("btn--ghost", main);
      b.classList.toggle("btn--quiet", !main);
    };
    var layout = function () {
      var offsite = mode !== "local";
      var on = ready || offsite;
      hide("[data-asr-pill=on]", !on);
      hide("[data-asr-pill=missing]", on || !supported || unavailable);
      hide("[data-asr-pill=unavailable]", on || (supported && !unavailable));
      hide("[data-asr-ready]", !ready || offsite);
      hide("[data-asr-missing]", ready || unavailable);
      hide("[data-asr-unavailable-note]", !unavailable || offsite);
      hide("[data-asr-cpu]", offsite || unavailable);
      hide("[data-asr-howto]", !((supported && !unavailable) || offsite));
      if (unavailable && installError) installError.hidden = true;
      // The dialog's hint for this computer, and the line that says where
      // voice notes go while they stay here.
      document.querySelectorAll("[data-asr-local-hint]").forEach(function (h) {
        h.hidden = (h.getAttribute("data-asr-local-hint") === "unavailable") !== unavailable;
      });
      if (!offsite && state) state.textContent = describe(mode, null).text;
      var installing = supported && !ready && !unavailable && !offsite;
      weigh(install, !offsite);
      weigh(opener, !installing);
    };

    // What the screen says about the other end; the original stays in the
    // answer (and the app's log), never on the page.
    var describe = function (mode, remote) {
      if (!remote || !remote.configured) {
        if (unavailable) return { ok: true, text: t("Os áudios ainda não podem ser transcritos.") };
        return { ok: true, text: server ? t("Os áudios são transcritos neste servidor.") : t("Os áudios são transcritos neste computador.") };
      }
      var where = mode === "openai" ? t("a OpenAI") : remote.url;
      if (remote.reachable && remote.ready) return { ok: true, text: t("Os áudios vão para %s, que está respondendo.", where) };
      var meanwhile = unavailable ? "" : server ? t("Enquanto isso, os áudios são transcritos neste servidor, se a transcrição estiver instalada nele.") : t("Enquanto isso, os áudios são transcritos neste computador, se a transcrição estiver instalada aqui.");
      var why;
      if (remote.reason === "refused") why = mode === "openai" ? t("A OpenAI recusou a chave da API. Confira a chave em Trocar onde transcrever.") : t("O servidor recusou a chave. Confira a chave em Trocar onde transcrever.");
      else if (remote.reason === "not_installed") why = t("O servidor ainda não tem a transcrição instalada.");
      else if (remote.reason === "unavailable") why = t("O servidor não consegue transcrever: a transcrição local ainda não funciona no Windows.");
      else if (remote.reason === "model") why = t("Esta chave da OpenAI não pode usar o modelo escolhido.");
      else if (remote.reason === "not_compatible") why = t("O programa nesse endereço não transcreve áudios. Confira o endereço em Trocar onde transcrever.");
      else why = t("%s não está respondendo agora.", where);
      return { ok: false, text: meanwhile ? why + " " + meanwhile : why };
    };
    var show = function (mode, remote) {
      var d = describe(mode, remote);
      state.textContent = d.text;
      return d;
    };
    if (box.hasAttribute("data-asr-remote-on")) {
      request("GET", "/api/asr?remote=1").then(function (d) { show(d.mode, d.remote); }).catch(function () {});
    }

    // Installing here: the progress in the card, and at the end the card
    // says it is on, or what to click, with the original only in the log.
    var poll = function () {
      request("GET", "/api/asr").then(function (d) {
        var st = d.status || {}, i = st.install || {};
        if (st.ready || i.state === "error" || i.state === "done") {
          ready = !!st.ready;
          unavailable = !!st.unavailable;
          progress.hidden = true;
          install.disabled = false;
          installError.hidden = i.state !== "error" || ready || unavailable;
          layout();
          return;
        }
        if (i.state === "running") {
          progress.hidden = false;
          step.textContent = (i.step || t("Preparando")) + (i.total ? " · " + Math.floor(100 * (i.downloaded || 0) / i.total) + "%" : "");
        }
        window.setTimeout(poll, 1500);
      }).catch(function () { window.setTimeout(poll, 3000); });
    };
    if (install) {
      install.addEventListener("click", function () {
        install.disabled = true;
        installError.hidden = true;
        progress.hidden = false;
        step.textContent = t("Preparando…");
        request("POST", "/api/asr/install").then(poll).catch(function () {
          progress.hidden = true;
          install.disabled = false;
          installError.hidden = false;
        });
      });
      if (!progress.hidden) poll();
    }

    // The conversation context follows the place unless one was chosen: a
    // new place rewrites its line (a notice when the text leaves the
    // computer), the dialog's hint and the choice it opens with.
    var context = function (c) {
      // Only a server or OpenAI has a context to choose; here it is the names.
      var block = card.querySelector("[data-asr-context]");
      if (block) block.hidden = !c.offsite;
      var line = card.querySelector("[data-asr-context-line]");
      if (line) {
        line.textContent = c.line;
        line.classList.toggle("note", !!c.leaves);
        line.classList.toggle("muted", !c.leaves);
      }
      var where = document.querySelector("[data-asr-context-where]");
      if (where) where.textContent = c.where;
      document.querySelectorAll("[data-asr-context-level]").forEach(function (i) {
        i.checked = i.defaultChecked = i.value === c.level;
      });
    };

    // Mudar o contexto saves without leaving the page, like Trocar onde
    // transcrever: the card's line changes in place and the dialog closes.
    // Without a script, the form posts and the page comes back.
    var ctxForm = document.querySelector("[data-asr-context-form]");
    if (ctxForm) {
      var ctxError = ctxForm.querySelector("[data-asr-context-error]");
      var ctxSubmit = ctxForm.querySelector("button[type=submit]");
      ctxForm.addEventListener("submit", function (event) {
        var c = ctxForm.querySelector("[data-asr-context-level]:checked");
        if (!c) return; // the server says what is missing
        event.preventDefault();
        if (ctxError) ctxError.hidden = true;
        ctxSubmit.disabled = true;
        request("POST", "/api/asr/contexto", { context: c.value }).then(function (d) {
          context(d);
          var x = ctxForm.closest(".overlay").querySelector(".dialog__close");
          if (x) x.click();
        }).catch(function (e) {
          if (!ctxError) return;
          ctxError.textContent = (e instanceof TypeError || !e.message) ? t("Não foi possível salvar agora. Tente de novo.") : e.message;
          ctxError.hidden = false;
        }).then(function () { ctxSubmit.disabled = false; });
      });
    }

    var dialog = document.querySelector("[data-asr-dialog]");
    if (!dialog || !opener) return;
    var form = dialog.querySelector("[data-asr-remote-form]");
    var error = dialog.querySelector("[data-asr-remote-error]");
    var submit = dialog.querySelector("[data-asr-submit]");
    var cancel = dialog.querySelector("[data-asr-cancel]");
    var chosen = function () {
      var c = form.querySelector("[data-asr-mode]:checked");
      return c ? c.value : "local";
    };
    var fields = function () {
      var m = chosen();
      form.querySelectorAll("[data-asr-fields]").forEach(function (f) { f.hidden = f.getAttribute("data-asr-fields") !== m; });
    };
    form.querySelectorAll("[data-asr-mode]").forEach(function (r) { r.addEventListener("change", fields); });
    var reset = function () {
      form.reset();
      fields();
      // The model stays folded away unless one is set.
      form.querySelectorAll("[data-asr-advanced]").forEach(function (d) {
        var input = d.querySelector("input");
        d.open = !!(input && input.value && input.value !== "whisper-1");
      });
      error.hidden = true;
      submit.disabled = false;
      cancel.textContent = t("Cancelar");
    };
    var dlg = modal(dialog, {
      closeTo: "#" + card.id,
      focus: function () { (form.querySelector("[data-asr-mode]:checked") || submit).focus(); },
      reset: reset,
      fallback: function () { return opener; }
    });
    opener.addEventListener("click", function (event) {
      event.preventDefault();
      reset();
      dlg.open(function () { return opener; });
    });
    form.addEventListener("submit", function (event) {
      event.preventDefault();
      error.hidden = true;
      submit.disabled = true;
      var m = chosen();
      var body = { mode: m };
      if (m === "remote") {
        body.url = form.querySelector("[data-asr-remote-url]").value;
        body.token = form.querySelector("[data-asr-remote-token]").value;
        body.model = form.querySelector("[data-asr-remote-model]").value;
      } else if (m === "openai") {
        body.token = form.querySelector("[data-asr-openai-key]").value;
        body.model = form.querySelector("[data-asr-openai-model]").value;
      }
      request("POST", "/api/asr/remote", body).then(function (d) {
        form.querySelectorAll("input[type=password]").forEach(function (i) { i.value = ""; });
        // What was saved is what the dialog shows the next time it opens.
        form.querySelectorAll("input[type=text]").forEach(function (i) { i.defaultValue = i.value; });
        form.querySelectorAll("[data-asr-mode]").forEach(function (i) { i.defaultChecked = i.checked; });
        mode = d.mode || m;
        if (d.status) { ready = !!d.status.ready; unavailable = !!d.status.unavailable; }
        if (d.context) context(d.context);
        layout();
        var r = show(d.mode, d.remote);
        if (r.ok) { dlg.close(); return; }
        // Saved, but the other end does not answer as it should: say so here,
        // where the fields to fix it are.
        error.textContent = r.text;
        error.hidden = false;
        cancel.textContent = t("Fechar");
      }).catch(function (e) {
        // The server's answer is already a sentence for the person; a
        // failed connection is not.
        error.textContent = (e instanceof TypeError || !e.message) ? t("Não foi possível salvar agora. Tente de novo.") : e.message;
        error.hidden = false;
      }).then(function () { submit.disabled = false; });
    });
  })();

  // ---- downloaded files ----
  (function () {
    var media = document.querySelector("[data-media]");
    if (!media) return;
    var note = media.querySelector("[data-media-note]");
    var retention = media.querySelector("[data-media-retention]");
    var days = media.querySelector("[data-media-days]");
    var clearMedia = media.querySelector("[data-media-clear]");
    var clearExports = media.querySelector("[data-media-clear-exports]");
    var KINDS = { image: t("Fotos"), audio: t("Áudios"), video: t("Vídeos"), document: t("Documentos") };

    var show = function (text) { note.textContent = text; note.hidden = !text; };

    var render = function (inv) {
      media.querySelector("[data-media-bytes]").textContent = size(inv.bytes);
      media.querySelector("[data-media-files]").textContent = inv.files;
      media.querySelector("[data-media-exports]").textContent = inv.exports.files ? inv.exports.files + " · " + size(inv.exports.bytes) : t("Nenhuma");
      var parts = Object.keys(KINDS).filter(function (k) { return inv.by_type[k]; }).map(function (k) { return KINDS[k] + " " + size(inv.by_type[k].bytes); });
      if (inv.duplicate_bytes) parts.push(t("repetidos %s", size(inv.duplicate_bytes)));
      var types = media.querySelector("[data-media-types]");
      types.textContent = parts.join(" · ");
      types.hidden = !parts.length;
      retention.checked = inv.retention_days > 0;
      if (inv.retention_days > 0) days.value = inv.retention_days;
      days.disabled = !retention.checked;
      clearMedia.hidden = !inv.files;
      clearMedia.textContent = t("Apagar os arquivos baixados (%s)", size(inv.bytes));
      clearExports.hidden = !inv.exports.files;
    };

    var load = function () {
      return request("GET", "/api/media").then(render).catch(function (e) { show(t("Não foi possível medir os arquivos: %s", e.message)); });
    };

    var saveRetention = function () {
      var n = retention.checked ? Math.max(1, parseInt(days.value, 10) || 30) : 0;
      days.disabled = !retention.checked;
      request("POST", "/api/media/retention", { days: n }).then(function (r) {
        show(r.deleted_files ? (r.deleted_files === 1 ? t("1 arquivo antigo apagado, %s liberados.", size(r.freed_bytes)) : t("%d arquivos antigos apagados, %s liberados.", r.deleted_files, size(r.freed_bytes))) : "");
        return load();
      }).catch(function (e) { show(e.message); });
    };
    retention.addEventListener("change", saveRetention);
    days.addEventListener("change", function () { if (retention.checked) saveRetention(); });

    var purge = function (what, b) {
      b.disabled = true;
      request("POST", "/api/media/purge", { what: what }).then(function (r) {
        show(r.deleted_files === 1 ? t("1 arquivo apagado, %s liberados.", size(r.freed_bytes)) : t("%d arquivos apagados, %s liberados.", r.deleted_files, size(r.freed_bytes)));
        return load();
      }).catch(function (e) { show(e.message); }).then(function () { b.disabled = false; });
    };
    confirming(clearMedia, t("Confirmar: apagar todos"), function () { purge("media", clearMedia); });
    confirming(clearExports, t("Confirmar: apagar as exportações"), function () { purge("exports", clearExports); });

    load();
  })();

  // ---- the (?) tooltips ----
  // The text opens centred over its (?); near an edge of the screen or of a
  // dialog it slides sideways to stay whole.
  (function () {
    var place = function (tip) {
      var body = tip.querySelector(".tip__body");
      if (!body) return;
      body.style.removeProperty("--tip-dx");
      var r = body.getBoundingClientRect();
      if (!r.width) return;
      var width = document.documentElement.clientWidth;
      // A dialog scrolls: its inner width leaves out the border and the bar.
      var box = tip.closest(".dialog");
      var left = 0, right = width;
      if (box) {
        left = box.getBoundingClientRect().left + box.clientLeft;
        right = left + box.clientWidth;
      }
      var lo = Math.max(left, 0) + 8, hi = Math.min(right, width) - 8;
      var dx = 0;
      if (r.right > hi) dx = hi - r.right;
      if (r.left + dx < lo) dx = lo - r.left;
      if (dx) body.style.setProperty("--tip-dx", Math.round(dx) + "px");
    };
    ["mouseover", "focusin"].forEach(function (type) {
      document.addEventListener(type, function (event) {
        var tip = event.target.closest && event.target.closest(".tip");
        if (!tip) return;
        place(tip);
        window.requestAnimationFrame(function () { place(tip); });
      });
    });
  })();
})();
