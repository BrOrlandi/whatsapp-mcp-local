// The Webhooks and Arquivos baixados cards of Configurações: both read and
// change their state through the local API, the same one scripts use
// (docs/api.md), so the page and a program always see the same thing.
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
        if (!response.ok) throw new Error(data.error || "erro " + response.status);
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
    return (i === 0 ? n : n.toFixed(n < 10 ? 1 : 0)).toString().replace(".", ",") + " " + units[i];
  }

  function ago(iso) {
    var s = Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000);
    if (s < 60) return "agora há pouco";
    if (s < 3600) return "há " + Math.floor(s / 60) + " min";
    if (s < 86400) return "há " + Math.floor(s / 3600) + " h";
    return "há " + Math.floor(s / 86400) + " dias";
  }

  // ---- webhooks ----
  (function () {
    var card = document.querySelector("[data-webhooks]");
    if (!card) return;
    var list = card.querySelector("[data-webhooks-list]");
    var empty = card.querySelector("[data-webhooks-empty]");
    var loading = card.querySelector("[data-webhooks-loading]");
    var form = card.querySelector("[data-webhook-form]");
    var formError = card.querySelector("[data-webhook-error]");
    var secretBox = card.querySelector("[data-webhook-secret]");
    var intro = card.querySelector("[data-webhooks-intro]");
    var cancel = form.querySelector("[data-webhook-cancel]");
    // With no webhook the card only says the feature exists; the form opens
    // when someone sets up the first one.
    var count = 0, adding = false;
    var layout = function () {
      var open = count > 0 || adding;
      empty.hidden = open;
      intro.hidden = !open;
      form.hidden = !open;
      cancel.hidden = count > 0;
    };
    var EVENTS = { message: "Mensagens", reaction: "Reações", receipt: "Entregas e leituras" };
    // What a row last said (a test's result, an error), kept across the
    // refreshes that redraw the list.
    var notes = {};

    var row = function (w) {
      var li = el("li", "row");
      var main = el("span", "row__main");
      main.appendChild(el("span", "row__title mono", w.url));
      var what = (w.events || []).map(function (e) { return EVENTS[e] || e; }).join(" · ");
      if (w.include_own) what += " · inclui as suas";
      if (w.chats && w.chats.length) what += " · " + w.chats.length + (w.chats.length === 1 ? " conversa" : " conversas");
      main.appendChild(el("span", "row__meta", what));
      var status;
      if (!w.enabled && w.disabled_reason) status = "Desligado sozinho " + ago(w.disabled_at) + ": " + w.disabled_reason;
      else if (!w.enabled) status = "Desligado.";
      else if (w.last_success_at) status = "Última entrega " + ago(w.last_success_at) + " · " + w.delivered + (w.delivered === 1 ? " entregue" : " entregues");
      else status = "Nenhuma mensagem entregue ainda.";
      if (w.queued) status += " · " + w.queued + " esperando";
      main.appendChild(el("span", "row__meta", status));
      var note = el("span", "row__meta", notes[w.id] || "");
      note.hidden = !notes[w.id];
      main.appendChild(note);
      var say = function (text) { notes[w.id] = text; note.textContent = text; note.hidden = !text; };
      li.appendChild(main);
      li.appendChild(el("span", "pill " + (w.enabled ? "pill--ok" : "pill--off"), w.enabled ? "Ligado" : "Desligado"));

      var actions = el("span", "actions");
      var test = button("Testar");
      test.addEventListener("click", function () {
        test.disabled = true;
        say("Mandando um aviso de teste…");
        request("POST", "/api/webhooks/" + w.id + "/test").then(function (r) {
          say(r.ok ? "O teste chegou, em " + r.ms + " ms." : "O teste não chegou: " + (r.result || r.error) + ".");
        }).catch(function (e) { say(e.message); }).then(function () { test.disabled = false; });
      });
      var toggle = button(w.enabled ? "Desligar" : "Religar");
      toggle.addEventListener("click", function () {
        toggle.disabled = true;
        request("PATCH", "/api/webhooks/" + w.id, { enabled: !w.enabled }).then(load).catch(function (e) {
          say(e.message); toggle.disabled = false;
        });
      });
      var remove = button("Apagar");
      confirming(remove, "Confirmar: apagar", function () {
        remove.disabled = true;
        request("DELETE", "/api/webhooks/" + w.id).then(function () { delete notes[w.id]; return load(); }).catch(function (e) {
          say(e.message); remove.disabled = false;
        });
      });
      actions.appendChild(test);
      actions.appendChild(toggle);
      actions.appendChild(remove);
      li.appendChild(actions);
      return li;
    };

    var load = function () {
      return request("GET", "/api/webhooks").then(function (data) {
        loading.hidden = true;
        card.querySelector("[data-webhooks-unavailable]").hidden = data.available !== false;
        list.textContent = "";
        (data.webhooks || []).forEach(function (w) { list.appendChild(row(w)); });
        count = (data.webhooks || []).length;
        list.hidden = !count;
        layout();
      }).catch(function (e) {
        loading.hidden = false;
        loading.textContent = "Não foi possível ler os webhooks: " + e.message;
      });
    };

    form.addEventListener("submit", function (event) {
      event.preventDefault();
      formError.hidden = true;
      var events = [];
      form.querySelectorAll("[data-webhook-event]").forEach(function (box) { if (box.checked) events.push(box.value); });
      var submit = form.querySelector("button[type=submit]");
      submit.disabled = true;
      request("POST", "/api/webhooks", {
        url: form.querySelector("[data-webhook-url]").value.trim(),
        events: events,
        include_own: form.querySelector("[data-webhook-own]").checked
      }).then(function (r) {
        form.reset();
        adding = false;
        secretBox.querySelector("[data-webhook-secret-value]").textContent = r.secret;
        secretBox.hidden = false;
        secretBox.scrollIntoView({ block: "nearest" });
        return load();
      }).catch(function (e) {
        formError.textContent = e.message;
        formError.hidden = false;
      }).then(function () { submit.disabled = false; });
    });

    card.querySelector("[data-webhook-start]").addEventListener("click", function () {
      adding = true;
      layout();
      form.querySelector("[data-webhook-url]").focus();
    });
    cancel.addEventListener("click", function () {
      adding = false;
      form.reset();
      formError.hidden = true;
      layout();
    });

    var copyKey = secretBox.querySelector("[data-webhook-secret-copy]");
    copyKey.addEventListener("click", function () {
      var value = secretBox.querySelector("[data-webhook-secret-value]").textContent;
      if (!navigator.clipboard) { copyKey.textContent = "Selecione e copie"; return; }
      navigator.clipboard.writeText(value).then(function () { copyKey.textContent = "Copiada"; }, function () { copyKey.textContent = "Selecione e copie"; });
    });
    secretBox.querySelector("[data-webhook-secret-close]").addEventListener("click", function () {
      secretBox.hidden = true;
      secretBox.querySelector("[data-webhook-secret-value]").textContent = "";
      copyKey.textContent = "Copiar a chave";
    });

    load();
    // A webhook can be turned off by itself, or changed by a script: follow it.
    window.setInterval(function () { if (!document.hidden) load(); }, 30000);
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
    var KINDS = { image: "Fotos", audio: "Áudios", video: "Vídeos", document: "Documentos" };

    var show = function (text) { note.textContent = text; note.hidden = !text; };

    var render = function (inv) {
      media.querySelector("[data-media-bytes]").textContent = size(inv.bytes);
      media.querySelector("[data-media-files]").textContent = inv.files;
      media.querySelector("[data-media-exports]").textContent = inv.exports.files ? inv.exports.files + " · " + size(inv.exports.bytes) : "Nenhuma";
      var parts = Object.keys(KINDS).filter(function (k) { return inv.by_type[k]; }).map(function (k) { return KINDS[k] + " " + size(inv.by_type[k].bytes); });
      if (inv.duplicate_bytes) parts.push("repetidos " + size(inv.duplicate_bytes));
      var types = media.querySelector("[data-media-types]");
      types.textContent = parts.join(" · ");
      types.hidden = !parts.length;
      retention.checked = inv.retention_days > 0;
      if (inv.retention_days > 0) days.value = inv.retention_days;
      days.disabled = !retention.checked;
      clearMedia.hidden = !inv.files;
      clearMedia.textContent = "Apagar os arquivos baixados (" + size(inv.bytes) + ")";
      clearExports.hidden = !inv.exports.files;
    };

    var load = function () {
      return request("GET", "/api/media").then(render).catch(function (e) { show("Não foi possível medir os arquivos: " + e.message); });
    };

    var saveRetention = function () {
      var n = retention.checked ? Math.max(1, parseInt(days.value, 10) || 30) : 0;
      days.disabled = !retention.checked;
      request("POST", "/api/media/retention", { days: n }).then(function (r) {
        show(r.deleted_files ? r.deleted_files + " arquivos antigos apagados, " + size(r.freed_bytes) + " liberados." : "");
        return load();
      }).catch(function (e) { show(e.message); });
    };
    retention.addEventListener("change", saveRetention);
    days.addEventListener("change", function () { if (retention.checked) saveRetention(); });

    var purge = function (what, b) {
      b.disabled = true;
      request("POST", "/api/media/purge", { what: what }).then(function (r) {
        show(r.deleted_files + (r.deleted_files === 1 ? " arquivo apagado, " : " arquivos apagados, ") + size(r.freed_bytes) + " liberados.");
        return load();
      }).catch(function (e) { show(e.message); }).then(function () { b.disabled = false; });
    };
    confirming(clearMedia, "Confirmar: apagar todos", function () { purge("media", clearMedia); });
    confirming(clearExports, "Confirmar: apagar as exportações", function () { purge("exports", clearExports); });

    load();
  })();
})();
