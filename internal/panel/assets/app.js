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
      var add = function (replace) {
        return api("/api/clients/" + client, { replace: replace }).then(function (result) {
          if (result.conflict === undefined) return result;
          // Another server already answers to this name, most likely the
          // hosted v1. It is the person's to replace, not the page's.
          var yes = window.confirm("O " + result.client + " já tem um servidor chamado \"whatsapp\", que aponta para:\n\n" +
            result.conflict + "\n\nSubstituir por este WhatsApp local?" +
            (client === "claude-desktop" ? " Uma cópia da configuração atual fica guardada." : " A configuração antiga é removida do Claude Code."));
          if (!yes) throw new Error("Nada foi alterado: o servidor que já estava configurado continua lá.");
          return add(true);
        });
      };
      add(false).then(function () {
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
      if (m.transcript) {
        var tr = el("div", "bubble__transcript", m.transcript);
        if (m.transcript_corrected) tr.title = "Corrigida pelo contexto da conversa";
        b.appendChild(tr);
      }
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

  // The connection, followed in the background: every two seconds while it
  // is on its way, every ten otherwise. The Estado tab's alert changes in
  // place; a page that shows the connection refreshes when it moves on,
  // unless the person is in the middle of something there.
  var live = document.querySelector("[data-live]");
  if (live) {
    var liveKey = live.getAttribute("data-live");
    var liveTone = live.getAttribute("data-live-tone");
    var liveBusy = live.hasAttribute("data-live-busy");
    var showsSync = !!document.querySelector("[data-sync]");
    var reloading = false;

    var showTone = function (tone) {
      var tab = live.querySelector('a[href="/estado"]');
      if (!tab) return;
      var mark = tab.querySelector(".nav__alert");
      var said = tab.querySelector(".sr-only");
      if (tone === "ok") {
        if (mark) mark.remove();
        if (said) said.remove();
        return;
      }
      if (!mark) {
        said = el("span", "sr-only", "Atenção: ");
        mark = el("span", "nav__alert", "!");
        mark.setAttribute("aria-hidden", "true");
        tab.insertBefore(said, tab.firstChild);
        tab.insertBefore(mark, said);
      }
      mark.classList.toggle("nav__alert--warn", tone === "warn");
    };

    var undisturbed = function () {
      var active = document.activeElement;
      if (active && /^(INPUT|TEXTAREA|SELECT)$/.test(active.tagName)) return false;
      if (document.querySelector("form[data-state=busy]")) return false;
      var open = window.location.hash.length > 1 && document.getElementById(window.location.hash.slice(1));
      return !(open && open.classList.contains("overlay"));
    };

    var follow = function () {
      window.setTimeout(function () {
        if (document.hidden || reloading) { follow(); return; }
        api("/api/pulse").then(function (pulse) {
          liveBusy = pulse.busy;
          if (showsSync && (pulse.key !== liveKey || pulse.tone !== liveTone) && undisturbed()) {
            reloading = true;
            window.location.reload();
            return;
          }
          if (pulse.tone !== liveTone) {
            liveTone = pulse.tone;
            showTone(liveTone);
          }
        }).catch(function () {}).then(follow);
      }, liveBusy ? 2000 : 10000);
    };
    follow();
  }

  // The first history sync may still be arriving after pairing.
  var syncing = document.querySelector("[data-sync-note]");
  if (syncing) {
    var syncWatch = window.setInterval(function () {
      api("/api/state").then(function (state) {
        if (state.arriving) {
          syncing.querySelector("[data-sync-count]").textContent = (state.arriving_count || 0).toLocaleString("pt-BR");
        } else {
          window.clearInterval(syncWatch);
          syncing.textContent = "Histórico recebido.";
          syncing.classList.remove("busy");
        }
      }).catch(function () {});
    }, 3000);
  }
})();

// Local transcription install, with its progress.
(function () {
  var button = document.querySelector("[data-asr-install]");
  if (!button) return;
  var progress = document.querySelector("[data-asr-progress]");
  var step = document.querySelector("[data-asr-step]");
  function poll() {
    fetch("/api/asr", { credentials: "same-origin" }).then(function (r) { return r.json(); }).then(function (d) {
      var i = d.status && d.status.install || {};
      if (d.status && d.status.ready) { window.location.reload(); return; }
      if (i.state === "error") { window.location.reload(); return; }
      if (i.state === "running") {
        progress.hidden = false;
        step.textContent = (i.step || "Preparando") + (i.total ? " · " + Math.floor(100 * i.downloaded / i.total) + "%" : "");
      }
      window.setTimeout(poll, 1500);
    }).catch(function () { window.setTimeout(poll, 3000); });
  }
  button.addEventListener("click", function () {
    button.disabled = true;
    progress.hidden = false;
    step.textContent = "Preparando…";
    fetch("/api/asr/install", { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json" }, body: "{}" })
      .then(function () { poll(); });
  });
  if (!progress.hidden) poll();
})();

// Inside the desktop app: links leave for the browser, settings save as they
// change, and updates are looked for and installed from the Configurações page.
(function () {
  "use strict";
  if (!document.documentElement.hasAttribute("data-app")) return;

  function el(tag, cls) {
    var node = document.createElement(tag);
    if (cls) node.className = cls;
    return node;
  }

  function post(path, body) {
    return fetch(path, { method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json", Accept: "application/json" }, body: JSON.stringify(body || {}) })
      .then(function (r) {
        return r.json().catch(function () { return {}; }).then(function (data) {
          if (!r.ok) throw new Error(data.error || "erro " + r.status);
          return data;
        });
      });
  }

  // The window shows only the panel: anything on the web opens in the
  // browser, the way a link in any other app does.
  document.addEventListener("click", function (event) {
    var link = event.target.closest && event.target.closest("a[href]");
    if (!link) return;
    var href = link.getAttribute("href");
    if (!/^https?:\/\//i.test(href)) return;
    event.preventDefault();
    post("/api/abrir", { url: href }).catch(function () {});
  });

  document.querySelectorAll("[data-setting]").forEach(function (input) {
    input.addEventListener("change", function () {
      var body = {};
      body[input.getAttribute("data-setting")] = input.checked;
      var note = input.closest(".card__body, .wizard-shell, body").querySelector("[data-setting-note]");
      input.disabled = true;
      post("/api/configuracoes", body).then(function () {
        if (note) note.hidden = true;
      }).catch(function (error) {
        input.checked = !input.checked;
        if (note) { note.hidden = false; note.textContent = error.message; }
      }).then(function () { input.disabled = false; });
    });
  });

  var folder = document.querySelector("[data-open-folder]");
  if (folder) folder.addEventListener("click", function () { post("/api/abrir-pasta").catch(function () {}); });

  // Updates: the card in Configurações and the banner on every page show the
  // same state, asked for every minute so a version found in the background
  // appears without a reload.
  var card = document.querySelector("[data-update]");
  var banner = document.querySelector("[data-update-banner]");
  if (!card && !banner) return;
  var timer = null;
  var DISMISSED = "wamcp-update-dismissed";

  function today() { return new Date().toISOString().slice(0, 10); }
  function dismissed(version) {
    try { return window.localStorage.getItem(DISMISSED) === version + "@" + today(); } catch (e) { return false; }
  }

  function describe(u) {
    switch (u.state) {
      case "checking": return "Procurando uma versão nova…";
      case "current": return "Você já tem a versão mais recente (" + u.current + ").";
      case "available": return "A versão " + u.latest + " está disponível. Atualizar baixa a versão nova, confere que ela chegou inteira e reinicia o app; o MCP fica fora do ar por alguns segundos.";
      case "downloading": return "Baixando a versão " + u.latest + "… " + (u.progress || 0) + "%";
      case "ready": return "Versão " + u.latest + " pronta. O app está reiniciando.";
      case "manual": return "A versão " + u.latest + " está disponível. Neste sistema, ela é instalada pelo pacote: baixe e instale como da primeira vez.";
      case "unsupported": return "Esta é uma versão de desenvolvimento (" + u.current + "), que não se atualiza sozinha.";
      case "error": return "Não foi possível atualizar agora: " + u.error;
    }
    return "O app procura uma versão nova duas vezes por dia.";
  }

  function renderCard(u) {
    if (!card) return;
    var latest = card.querySelector("[data-update-latest]");
    var install = card.querySelector("[data-update-install]");
    var page = card.querySelector("[data-update-page]");
    if (u.latest) latest.textContent = u.latest;
    else if (u.state === "current") latest.textContent = u.current;
    card.querySelector("[data-update-text]").textContent = describe(u);
    install.hidden = u.state !== "available";
    install.disabled = false;
    page.hidden = !(u.state === "manual" && u.page);
    if (u.page) page.href = u.page;
  }

  function renderBanner(u) {
    if (!banner) return;
    var shown = (u.state === "available" || u.state === "manual") ? !dismissed(u.latest) : (u.state === "downloading" || u.state === "ready");
    banner.hidden = !shown;
    if (!shown) return;
    var text = banner.querySelector("[data-update-banner-text]");
    var install = banner.querySelector("[data-update-banner-install]");
    var page = banner.querySelector("[data-update-banner-page]");
    install.hidden = u.state === "manual";
    page.hidden = u.state !== "manual";
    if (u.page) page.href = u.page;
    if (u.state === "available") {
      text.textContent = "O WhatsApp MCP " + u.latest + " está pronto para instalar. O MCP fica fora do ar por alguns segundos enquanto o app reinicia.";
      install.disabled = false;
      install.textContent = "Atualizar agora";
    } else if (u.state === "manual") {
      text.textContent = "O WhatsApp MCP " + u.latest + " está disponível. Baixe e instale o pacote novo.";
    } else {
      text.textContent = describe(u);
      install.disabled = true;
    }
  }

  function render(u) {
    renderCard(u);
    renderBanner(u);
    window.clearTimeout(timer);
    // Follow a download closely; otherwise look again in a minute.
    timer = window.setTimeout(poll, (u.state === "checking" || u.state === "downloading" || u.state === "ready") ? 1000 : 60000);
  }
  function poll() {
    fetch("/api/atualizacao", { credentials: "same-origin" }).then(function (r) { return r.json(); }).then(render).catch(function () {
      timer = window.setTimeout(poll, 60000);
    });
  }

  function busy(button, label) {
    button.disabled = true;
    button.textContent = label;
    var spin = el("span", "spinner");
    spin.setAttribute("aria-hidden", "true");
    button.insertBefore(spin, button.firstChild);
  }
  function install(button) {
    busy(button, "Atualizando…");
    post("/api/atualizacao/instalar").then(render).catch(function (e) {
      button.disabled = false;
      button.textContent = "Tentar de novo";
      var text = (banner && !banner.hidden) ? banner.querySelector("[data-update-banner-text]") : card && card.querySelector("[data-update-text]");
      if (text) text.textContent = e.message;
    });
  }

  if (card) {
    var check = card.querySelector("[data-update-check]");
    check.addEventListener("click", function () {
      busy(check, "Procurando…");
      // The answer comes back once GitHub has replied: a new version, or
      // none, or why it could not be asked.
      post("/api/atualizacao/verificar").then(render).catch(function (e) {
        card.querySelector("[data-update-text]").textContent = "Não foi possível procurar agora: " + e.message;
      }).then(function () {
        check.disabled = false;
        check.textContent = "Procurar atualização";
      });
    });
    var cardInstall = card.querySelector("[data-update-install]");
    cardInstall.addEventListener("click", function () { install(cardInstall); });
  }
  if (banner) {
    var bannerInstall = banner.querySelector("[data-update-banner-install]");
    bannerInstall.addEventListener("click", function () { install(bannerInstall); });
    banner.querySelector("[data-update-banner-close]").addEventListener("click", function () {
      banner.hidden = true;
      fetch("/api/atualizacao", { credentials: "same-origin" }).then(function (r) { return r.json(); }).then(function (u) {
        try { window.localStorage.setItem(DISMISSED, u.latest + "@" + today()); } catch (e) {}
      }).catch(function () {});
    });
  }
  poll();
})();
