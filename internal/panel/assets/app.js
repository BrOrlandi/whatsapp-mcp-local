// The panel's behaviour. Copy buttons and busy forms are v1's; the rest is what
// a local daemon adds: pairing that starts by itself, a chat preview to compare
// with the phone, and screens that move on when the next thing happens.
(function () {
  "use strict";

  function api(path, body) {
    var options = { headers: { Accept: "application/json" }, credentials: "same-origin" };
    if (body !== undefined) {
      options.method = "POST";
      options.headers["Content-Type"] = "application/json";
      options.body = JSON.stringify(body);
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

  // ---- copy buttons (v1) ----
  function label(button, text, restoreAfter) {
    button.textContent = text;
    if (restoreAfter) {
      window.setTimeout(function () {
        button.textContent = "Copiar";
        button.classList.remove("copy--done", "copy--failed");
      }, 2000);
    }
  }
  function copy(text, button) {
    if (!navigator.clipboard) {
      button.classList.add("copy--failed");
      label(button, "Selecione e copie", true);
      return;
    }
    navigator.clipboard.writeText(text).then(
      function () { button.classList.add("copy--done"); label(button, "Copiado", true); },
      function () { button.classList.add("copy--failed"); label(button, "Falhou", true); }
    );
  }
  document.querySelectorAll("[data-copy]").forEach(function (block) {
    var source = block.querySelector("code") || block;
    var button = el("button", "copy", "Copiar");
    button.type = "button";
    button.addEventListener("click", function () { copy(source.textContent, button); });
    var snippet = block.closest(".snippet");
    var head = snippet && snippet.querySelector(".snippet__head");
    if (head) { head.appendChild(button); return; }
    if (snippet) { snippet.classList.add("snippet--loose"); snippet.insertBefore(button, block); return; }
    block.parentNode.insertBefore(button, block);
  });

  // ---- busy forms (v1) ----
  document.querySelectorAll("form[data-busy]").forEach(function (form) {
    form.addEventListener("submit", function (event) {
      if (form.dataset.state === "busy") { event.preventDefault(); return; }
      form.dataset.state = "busy";
      var button = form.querySelector("button[type=submit]");
      window.setTimeout(function () {
        if (!button) return;
        button.disabled = true;
        button.textContent = form.getAttribute("data-busy") || "Aguarde";
        button.insertBefore(el("span", "spinner"), button.firstChild);
      }, 0);
    });
  });

  // ---- one-click client setup ----
  document.querySelectorAll("[data-add-client]").forEach(function (button) {
    button.addEventListener("click", function () {
      var client = button.getAttribute("data-add-client");
      var note = document.querySelector('[data-client-note="' + client + '"]');
      var text = button.textContent;
      button.disabled = true;
      button.textContent = "Configurando…";
      api("/api/clients/" + client, {}).then(function () {
        button.textContent = "Configurado ✓";
        if (note) {
          note.hidden = false;
          note.textContent = client === "claude-desktop"
            ? "Pronto. Agora feche o Claude Desktop e abra de novo; esta tela avisa quando ele se conectar."
            : "Pronto. Abra uma sessão nova do Claude Code; esta tela avisa quando ele se conectar.";
        }
      }).catch(function (error) {
        button.disabled = false;
        button.textContent = text;
        if (note) { note.hidden = false; note.textContent = error.message; }
      });
    });
  });

  // ---- waiting for a client ----
  // The daemon stamps every client that talks to it, so the page only has to
  // ask. Polling stops after about ten minutes: by then a reload is honest.
  var waiting = document.querySelector("[data-wait-client='true']");
  if (waiting) {
    var polls = 0;
    var watch = window.setInterval(function () {
      if (++polls > 150) { window.clearInterval(watch); return; }
      api("/api/state").then(function (state) {
        if (state.clients_live > 0) { window.clearInterval(watch); window.location.reload(); }
      }).catch(function () {});
    }, 4000);
  }

  // ---- pairing ----
  var pairing = document.querySelector("[data-pairing]");
  if (pairing) {
    var parts = {
      qr: pairing.querySelector("[data-pair-qr]"),
      code: pairing.querySelector("[data-pair-code]"),
      sync: pairing.querySelector("[data-pair-sync]"),
      error: pairing.querySelector("[data-pair-error]")
    };
    var img = pairing.querySelector("[data-qr]");
    var wait = pairing.querySelector("[data-qr-wait]");
    var lastQR = -1;
    var usingPhone = false;

    function showOnly(name) {
      Object.keys(parts).forEach(function (key) { parts[key].hidden = key !== name; });
    }
    function start(phone) {
      return api("/api/pair", phone ? { phone: phone } : {}).catch(function (error) {
        showOnly("error");
        pairing.querySelector("[data-error-text]").textContent = error.message;
      });
    }
    function render(state) {
      if (state.account && state.account.authenticated && state.pairing.state !== "syncing") {
        window.location.reload();
        return;
      }
      var p = state.pairing;
      switch (p.state) {
        case "qr":
          showOnly("qr");
          if (p.qr_version !== lastQR) {
            lastQR = p.qr_version;
            img.src = "/api/pair/qr.png?v=" + lastQR;
            img.hidden = false;
            wait.hidden = true;
          }
          break;
        case "code":
          showOnly("code");
          pairing.querySelector("[data-code-wait]").hidden = true;
          pairing.querySelector("[data-code-ready]").hidden = false;
          pairing.querySelector("[data-code]").textContent = p.code || "";
          break;
        case "syncing":
          showOnly("sync");
          // Paired: the next step connects Claude while the history keeps
          // arriving in the background.
          if (state.account && state.account.authenticated) window.location.reload();
          break;
        case "error":
          showOnly("error");
          pairing.querySelector("[data-error-text]").textContent = "Não foi possível conectar: " + p.error;
          break;
        case "done":
          window.location.reload();
          return;
        case "idle":
        case "cancelled":
          // Nothing running: start, so the QR code is simply there.
          if (!usingPhone) start();
          break;
        default:
          // Starting: keep showing whichever way the person chose.
          showOnly(usingPhone ? "code" : "qr");
      }
    }
    function poll() {
      api("/api/state").then(render).catch(function () {}).then(function () { window.setTimeout(poll, 1500); });
    }

    pairing.querySelector("[data-pair-phone-toggle]").addEventListener("click", function () {
      var form = pairing.querySelector("[data-pair-phone]");
      form.hidden = !form.hidden;
      if (!form.hidden) pairing.querySelector("[data-pair-phone-input]").focus();
    });
    pairing.querySelector("[data-pair-phone-go]").addEventListener("click", function (event) {
      var phone = pairing.querySelector("[data-pair-phone-input]").value;
      usingPhone = true;
      event.target.disabled = true;
      // The code takes a few seconds to come back from WhatsApp; say so
      // instead of leaving the old QR code, or a transient state, on screen.
      showOnly("code");
      pairing.querySelector("[data-code-wait]").hidden = false;
      pairing.querySelector("[data-code-ready]").hidden = true;
      api("/api/pair/cancel", {}).then(function () { return start(phone); }).then(function () { event.target.disabled = false; });
    });
    pairing.querySelector("[data-pair-back]").addEventListener("click", function () {
      usingPhone = false;
      lastQR = -1;
      img.hidden = true;
      wait.hidden = false;
      showOnly("qr");
      api("/api/pair/cancel", {}).then(function () { return start(); });
    });
    pairing.querySelector("[data-pair-retry]").addEventListener("click", function () {
      usingPhone = false;
      showOnly("qr");
      start();
    });
    poll();
  }

  // ---- chat preview ----
  // The ten latest chats, drawn like the phone's list, and the last messages of
  // the one selected: enough to see at a glance that what arrives here is what
  // the phone shows.
  var preview = document.querySelector("[data-chats]");
  if (preview) {
    var palette = ["#128c7e", "#34b7f1", "#e0654a", "#8e5bd8", "#d6a01b", "#2c9f6a", "#c2477f", "#4a6fd6"];
    var list = preview.querySelector("[data-chat-list]");
    var countPill = preview.querySelector("[data-chats-count]");
    var thread = preview.querySelector("[data-thread]");
    var threadTitle = preview.querySelector("[data-thread-title]");
    var threadMeta = preview.querySelector("[data-thread-meta]");
    var threadAvatar = preview.querySelector("[data-thread-avatar]");
    var historyNote = preview.querySelector("[data-history-note]");
    var chats = [];
    var selected = null;
    var shown = "";

    function color(key) {
      var h = 0;
      for (var i = 0; i < key.length; i++) h = (h * 31 + key.charCodeAt(i)) >>> 0;
      return palette[h % palette.length];
    }
    function initial(name) {
      var m = (name || "").match(/[\p{L}\p{N}]/u);
      return m ? m[0].toUpperCase() : "#";
    }
    function paintAvatar(node, name, key) {
      node.textContent = initial(name);
      node.style.background = color(key);
    }
    function when(iso) {
      var d = new Date(iso), now = new Date();
      if (d.toDateString() === now.toDateString()) return d.toLocaleTimeString("pt-BR", { hour: "2-digit", minute: "2-digit" });
      if (new Date(now.getTime() - 86400000).toDateString() === d.toDateString()) return "Ontem";
      if (now - d < 6 * 86400000) return d.toLocaleDateString("pt-BR", { weekday: "long" });
      return d.toLocaleDateString("pt-BR", { day: "2-digit", month: "2-digit", year: "2-digit" });
    }
    var mediaLabels = { image: "📷 Foto", video: "🎥 Vídeo", audio: "🎤 Áudio", document: "📄 Documento", sticker: "Figurinha", location: "📍 Localização", live_location: "📍 Localização em tempo real", contact: "👤 Contato", poll: "📊 Enquete" };
    function mediaText(media, text) {
      var lbl = mediaLabels[media] || (media ? "Mídia" : "");
      if (lbl && text && text.charAt(0) !== "[") return lbl + " · " + text;
      return lbl || text;
    }
    function firstName(name) {
      return /^\+/.test(name) ? name : name.split(" ")[0];
    }

    function renderChats() {
      list.textContent = "";
      countPill.textContent = chats.length ? chats.length + " conversas" : "nenhuma ainda";
      if (!chats.length) {
        list.appendChild(el("li", "empty", "Nenhuma conversa guardada ainda. Assim que o celular mandar o histórico, elas aparecem aqui."));
        return;
      }
      chats.forEach(function (c) {
        var li = el("li");
        var row = el("button", "chat" + (selected && selected.jid === c.jid ? " chat--on" : ""));
        row.type = "button";
        var av = el("span", "avatar");
        paintAvatar(av, c.name, c.jid);
        row.appendChild(av);
        var main = el("span", "chat__main");
        var top = el("span", "chat__top");
        top.appendChild(el("span", "chat__name", c.name));
        top.appendChild(el("span", "chat__time" + (c.unread ? " chat__time--unread" : ""), when(c.at)));
        var bottom = el("span", "chat__bottom");
        var prefix = c.last_from_me ? "Você: " : (c.group && c.last_sender ? firstName(c.last_sender) + ": " : "");
        bottom.appendChild(el("span", "chat__last", prefix + mediaText(c.last_media, c.last_text)));
        if (c.unread) bottom.appendChild(el("span", "chat__badge", String(c.unread)));
        else if (c.pinned) bottom.appendChild(el("span", "chat__pin", "📌"));
        main.appendChild(top);
        main.appendChild(bottom);
        row.appendChild(main);
        row.addEventListener("click", function () { select(c); });
        li.appendChild(row);
        list.appendChild(li);
      });
    }

    function bubble(m, showWho) {
      var b = el("div", "bubble" + (m.from_me ? " bubble--me" : ""));
      if (showWho && !m.from_me && m.sender) b.appendChild(el("span", "bubble__who", m.sender));
      if (m.media) b.appendChild(el("div", "bubble__media", mediaLabels[m.media] || "Mídia"));
      if (m.text && m.text.charAt(0) !== "[") b.appendChild(el("div", "bubble__text", m.text));
      b.appendChild(el("span", "bubble__time", new Date(m.at).toLocaleTimeString("pt-BR", { hour: "2-digit", minute: "2-digit" })));
      return b;
    }

    function select(c) {
      if (!selected || selected.jid !== c.jid) {
        historyNote.hidden = true;
        shown = "";
      }
      selected = c;
      renderChats();
      threadTitle.textContent = c.name;
      paintAvatar(threadAvatar, c.name, c.jid);
      loadThread();
    }
    function loadThread() {
      if (!selected) return;
      var c = selected;
      api("/api/chats/" + encodeURIComponent(c.jid)).then(function (data) {
        if (selected !== c) return;
        // Redraw only when something changed, so a refresh does not jump the
        // scroll position under the reader.
        var key = data.messages.map(function (m) { return m.id; }).join(",");
        if (key !== shown) {
          shown = key;
          thread.textContent = "";
          var lastDay = "";
          data.messages.forEach(function (m) {
            var day = new Date(m.at).toLocaleDateString("pt-BR", { day: "2-digit", month: "long", year: "numeric" });
            if (day !== lastDay) { thread.appendChild(el("div", "thread__day", day)); lastDay = day; }
            thread.appendChild(bubble(m, c.group));
          });
          if (!data.messages.length) thread.appendChild(el("div", "empty", "Nenhuma mensagem desta conversa ainda."));
          thread.scrollTop = thread.scrollHeight;
        }
        threadMeta.textContent = c.messages.toLocaleString("pt-BR") + (c.messages === 1 ? " mensagem guardada" : " mensagens guardadas") +
          (data.oldest ? ", desde " + new Date(data.oldest).toLocaleDateString("pt-BR", { day: "2-digit", month: "short", year: "numeric" }) : "") +
          " · as últimas " + data.messages.length + " aqui";
      }).catch(function (error) { thread.textContent = error.message; });
    }

    function requestHistory(chat, button, note) {
      button.disabled = true;
      api("/api/history", chat ? { chat_jid: chat } : {}).then(function (data) {
        note.hidden = false;
        note.textContent = data.started
          ? "Pedido enviado ao celular. As mensagens mais antigas chegam em até um minuto; a tela se atualiza sozinha."
          : "Já há um pedido de histórico em andamento. Assim que ele terminar, dá para pedir de novo.";
        watchHistory(button, note);
      }).catch(function (error) {
        button.disabled = false;
        note.hidden = false;
        note.textContent = error.message;
      });
    }
    function watchHistory(button, note) {
      var tries = 0;
      var t = window.setInterval(function () {
        if (++tries > 90) { window.clearInterval(t); button.disabled = false; return; }
        api("/api/chats").then(function (data) {
          var job = data.history;
          if (job && job.finished_at) {
            window.clearInterval(t);
            button.disabled = false;
            var failed = job.failures ? Object.keys(job.failures).length : 0;
            note.textContent = job.messages_added
              ? "Chegaram " + job.messages_added.toLocaleString("pt-BR") + " mensagens mais antigas."
              : failed
                ? "O celular não respondeu. Confira se ele está com internet e tente de novo."
                : "O celular não tinha mensagens mais antigas para mandar.";
            refresh();
          } else if (job) {
            note.textContent = "Buscando… " + job.chats_done + " de " + job.chats.length + " conversas, " + job.messages_added + " mensagens novas até agora.";
          }
        }).catch(function () {});
      }, 3000);
    }

    var oneChat = preview.querySelector("[data-history-chat]");
    oneChat.addEventListener("click", function () { if (selected) requestHistory(selected.jid, oneChat, historyNote); });
    var allChats = document.querySelector("[data-history-all]");
    if (allChats) allChats.addEventListener("click", function () { requestHistory("", allChats, document.querySelector("[data-history-all-note]")); });

    function refresh() {
      api("/api/chats").then(function (data) {
        chats = data.chats;
        if (selected) {
          var still = chats.filter(function (c) { return c.jid === selected.jid; })[0];
          selected = still || selected;
        }
        renderChats();
        if (!selected && chats.length) select(chats[0]);
        else loadThread();
      }).catch(function (error) {
        list.textContent = "";
        list.appendChild(el("li", "empty", error.message));
      });
    }
    refresh();
    window.setInterval(refresh, 15000);
  }

  // The first history sync may still be arriving after pairing.
  var syncing = document.querySelector("[data-sync-note]");
  if (syncing) {
    var syncWatch = window.setInterval(function () {
      api("/api/state").then(function (state) {
        var p = state.pairing;
        if (p.state === "syncing") {
          syncing.querySelector("[data-sync-count]").textContent = (p.messages_synced || 0).toLocaleString("pt-BR");
        } else {
          window.clearInterval(syncWatch);
          syncing.textContent = "Histórico recebido. Confira se é este o número que você queria conectar.";
        }
      }).catch(function () {});
    }, 3000);
  }
})();
