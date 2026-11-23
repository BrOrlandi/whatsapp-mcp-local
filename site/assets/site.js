(() => {
  const REPO = "BrOrlandi/whatsapp-mcp-local";
  const reduceMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

  // The page's language (<html lang>) picks the words this script writes.
  const lang = { es: "es", en: "en" }[document.documentElement.lang.slice(0, 2)] || "pt";
  const text = {
    pt: {
      locale: "pt-BR",
      version: (version, date) => `Versão ${version}, de ${date}`,
      copy: "Copiar",
      copied: "Copiado",
      copyFailed: "Selecione e copie",
    },
    es: {
      locale: "es",
      version: (version, date) => `Versión ${version}, del ${date}`,
      copy: "Copiar",
      copied: "Copiado",
      copyFailed: "Selecciona y copia",
    },
    en: {
      locale: "en-US",
      version: (version, date) => `Version ${version}, released ${date}`,
      copy: "Copy",
      copied: "Copied",
      copyFailed: "Select and copy",
    },
  }[lang];

  // The language menu remembers the choice, so the Portuguese pages (the
  // default URLs) send this visitor back to it rather than to the browser's
  // language; see the script in their <head>.
  document.querySelectorAll(".langs a[hreflang]").forEach((link) => {
    link.addEventListener("click", () => {
      try {
        localStorage.setItem("lang", link.hreflang);
      } catch {
        // Storage blocked: the menu still works, it just isn't remembered.
        // Portuguese says so in its address (?lang=pt), so the browser's
        // language does not send the visitor back, in a new tab too.
      }
      // The sections keep the same ids in every language.
      if (location.hash && !link.hash) link.hash = location.hash;
    });
  });

  // ?lang=pt that could not be stored goes along to the other Portuguese
  // pages, which would otherwise follow the browser's language again.
  if (/[?&]lang=pt\b/.test(location.search)) {
    let stored = false;
    try {
      stored = localStorage.getItem("lang") === "pt-BR";
    } catch {}
    if (!stored) {
      document.querySelectorAll('a[href^="/"]').forEach((link) => {
        const url = new URL(link.href);
        if (/^\/(es|en|assets)(\/|$)/.test(url.pathname) || url.searchParams.has("lang")) return;
        url.searchParams.set("lang", "pt");
        link.href = url.pathname + url.search + url.hash;
      });
    }
  }

  // The visitor's system, to point at the right download.
  function detectOS() {
    const ua = navigator.userAgent;
    const platform = (navigator.userAgentData && navigator.userAgentData.platform) || navigator.platform || "";
    if (/iPhone|iPad|iPod|Android/i.test(ua) || (/Mac/i.test(platform) && navigator.maxTouchPoints > 1)) return "mobile";
    if (/Mac/i.test(platform) || /Mac OS X/.test(ua)) return "mac";
    if (/Win/i.test(platform) || /Windows/.test(ua)) return "windows";
    if (/Linux/i.test(platform) || /Linux|X11/.test(ua)) return "linux";
    return "";
  }

  const os = detectOS();
  if (os === "mobile") document.documentElement.classList.add("is-mobile");
  document.querySelectorAll(`[data-os="${os}"]`).forEach((el) => el.classList.add("is-current"));
  // On the download page, the visitor's system comes first, on a row of its own.
  const osGrid = document.querySelector(".os-grid");
  if (osGrid && osGrid.querySelector(".os.is-current")) osGrid.classList.add("has-current");

  // Download links point at releases/latest; with the API, they also get the version and size.
  const assetLinks = document.querySelectorAll("[data-asset]");
  if (assetLinks.length) {
    fetch(`https://api.github.com/repos/${REPO}/releases/latest`, { headers: { Accept: "application/vnd.github+json" } })
      .then((r) => (r.ok ? r.json() : Promise.reject(r.status)))
      .then((release) => {
        const version = release.tag_name.replace(/^v/, "");
        const date = new Date(release.published_at).toLocaleDateString(text.locale, { day: "numeric", month: "long", year: "numeric" });
        document.querySelectorAll("[data-version]").forEach((el) => {
          el.textContent = text.version(version, date);
        });
        assetLinks.forEach((link) => {
          const pattern = new RegExp(link.dataset.asset);
          const asset = release.assets.find((a) => pattern.test(a.name));
          if (!asset) return;
          link.href = asset.browser_download_url;
          const size = link.parentElement.querySelector("[data-size]");
          if (size) size.textContent = `, ${Math.max(1, Math.round(asset.size / 1e6))} MB`;
        });
      })
      .catch(() => {});
  }

  // Copy buttons next to commands.
  document.querySelectorAll("[data-copy]").forEach((button) => {
    const label = button.querySelector("span");
    button.addEventListener("click", async () => {
      const command = document.getElementById(button.dataset.copy).textContent.trim();
      try {
        await navigator.clipboard.writeText(command);
        label.textContent = text.copied;
      } catch {
        label.textContent = text.copyFailed;
      }
      setTimeout(() => (label.textContent = text.copy), 2000);
    });
  });

  // A link to a question (#banimento) opens it.
  function openFromHash() {
    const target = location.hash && document.getElementById(decodeURIComponent(location.hash.slice(1)));
    if (target && target.tagName === "DETAILS") target.open = true;
  }
  openFromHash();
  window.addEventListener("hashchange", openFromHash);

  // Opening a question puts its link in the address bar, ready to share; closing it takes it out.
  document.querySelectorAll("details.faq__item[id]").forEach((item) => {
    item.addEventListener("toggle", () => {
      const hash = "#" + item.id;
      if (item.open && location.hash !== hash) {
        history.replaceState(null, "", hash);
      } else if (!item.open && location.hash === hash) {
        history.replaceState(null, "", location.pathname + location.search);
      }
    });
  });

  // The explainer plays muted by itself, with the browser's own controls to
  // seek, change the speed or go full screen; the button turns its narration on.
  const video = document.querySelector("[data-video]");
  const sound = document.querySelector("[data-sound]");
  if (video && sound) {
    if (reduceMotion) {
      // Whoever asked the system for less motion gets the video still, with controls.
      video.removeAttribute("autoplay");
      video.pause();
      video.controls = true;
      sound.hidden = true;
    } else {
      let heard = false;
      // The native controls change the sound too: the button follows them,
      // and sound turned on there counts as heard, with no jump to the start.
      video.addEventListener("volumechange", () => {
        sound.setAttribute("aria-pressed", String(!video.muted));
        if (!video.muted) heard = true;
      });
      sound.addEventListener("click", () => {
        video.muted = !video.muted;
        sound.setAttribute("aria-pressed", String(!video.muted));
        // The narration tells a story from its start: the first time the sound
        // comes on, the video goes back to the beginning to be heard whole.
        if (!video.muted && !heard) {
          heard = true;
          video.currentTime = 0;
        }
        video.play();
      });
    }
  }

  // The explainer in each language there is a cut of. A language joins this
  // list only once its video and poster are in /assets/video/ (made by the
  // video project's render, video/README.md); with two or more, buttons below
  // the video switch between them, and a page opens on the cut in its own
  // language when there is one.
  const videoCuts = [
    { lang: "pt-BR", flag: "br", name: "Português", src: "/assets/video/como-funciona.mp4", poster: "/assets/video/como-funciona.jpg" },
    { lang: "es", flag: "es", name: "Español", src: "/assets/video/como-funciona-es.mp4", poster: "/assets/video/como-funciona-es.jpg" },
    { lang: "en", flag: "us", name: "English", src: "/assets/video/como-funciona-en.mp4", poster: "/assets/video/como-funciona-en.jpg" },
  ];
  const cutButtons = document.querySelector("[data-video-langs]");
  if (video && cutButtons && videoCuts.length > 1) {
    const current = () => videoCuts.find((cut) => cut.src === video.getAttribute("src"));
    const buttons = new Map();
    // A switch keeps the moment and whether it was playing; a paused video
    // stays paused (autoplay would start the new one by itself), and a moment
    // past the end of a shorter cut lands near its end. The page's own cut,
    // chosen as it opens, keeps the autoplay. A switch made before the last
    // one loaded replaces it, keeping the moment the visitor was at; a cut
    // that does not load goes back to that one, and loses its button.
    let pending = null;
    const switchTo = (cut, opening = false, resume = null) => {
      if (cut === current()) return;
      const state = resume || (pending && pending.state) || { at: video.currentTime, playing: !video.paused && !video.ended, from: current() };
      if (pending) pending.cancel();
      if (!opening) video.autoplay = false;
      const done = () => {
        video.removeEventListener("loadedmetadata", loaded);
        video.removeEventListener("error", failed);
        pending = null;
      };
      const loaded = () => {
        done();
        const end = video.duration;
        if (state.at > 0) video.currentTime = Number.isFinite(end) ? Math.min(state.at, Math.max(0, end - 0.5)) : state.at;
        if (state.playing) video.play().catch(() => {});
      };
      const failed = () => {
        done();
        buttons.get(cut)?.remove();
        buttons.delete(cut);
        if (state.from && state.from !== cut) switchTo(state.from, false, { ...state, from: null });
      };
      pending = { state, cancel: done };
      video.addEventListener("loadedmetadata", loaded);
      video.addEventListener("error", failed);
      video.poster = cut.poster;
      video.src = cut.src;
      buttons.forEach((button, c) => button.setAttribute("aria-pressed", String(c === cut)));
    };
    for (const cut of videoCuts) {
      const button = document.createElement("button");
      button.type = "button";
      button.lang = cut.lang;
      button.innerHTML = `<svg class="video-langs__flag" aria-hidden="true" focusable="false"><use href="#flag-${cut.flag}"/></svg>`;
      button.append(cut.name);
      button.addEventListener("click", () => switchTo(cut));
      buttons.set(cut, button);
      cutButtons.append(button);
    }
    buttons.forEach((button, c) => button.setAttribute("aria-pressed", String(c === current())));
    const own = videoCuts.find((cut) => cut.lang.slice(0, 2) === document.documentElement.lang.slice(0, 2));
    if (own && own !== current()) switchTo(own, true);
    cutButtons.hidden = false;
  }

  // The hero conversation: a request, what the app did in WhatsApp, and the answer.
  const chat = document.querySelector("[data-chat]");
  // Help videos: a poster of our own until clicked, then YouTube's player,
  // already playing. Before the click there is no player, so none of its
  // title bar and shade sits over the picture; without JS the link opens
  // the video on YouTube.
  document.querySelectorAll("a.video-frame[data-yt]").forEach((link) => {
    link.addEventListener("click", (event) => {
      event.preventDefault();
      const frame = document.createElement("iframe");
      frame.src = `https://www.youtube-nocookie.com/embed/${link.dataset.yt}?autoplay=1&rel=0&playsinline=1`;
      frame.title = link.getAttribute("aria-label") || "";
      frame.allow = "accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture; web-share";
      frame.referrerPolicy = "strict-origin-when-cross-origin";
      frame.allowFullscreen = true;
      const box = document.createElement("div");
      box.className = "video-frame";
      box.append(frame);
      link.replaceWith(box);
      frame.focus();
    });
  });

  // The setup demo on the home page: the app's screens, the phone, Claude
  // Desktop. The HTML holds the last scene; this plays the way there once, when
  // the figure comes into view, pausing while it is out of view. Whoever asked
  // for less motion keeps the last scene, still.
  const demo = document.querySelector("[data-demo]");
  if (demo && !reduceMotion && "IntersectionObserver" in window) {
    const control = demo.querySelector("[data-demo-control]");
    const label = control.querySelector("span");
    const steps = document.querySelectorAll(".how .steps > li");
    const stage = demo.querySelector(".demo__stage");
    const pointer = demo.querySelector(".demo-pointer");
    const typed = demo.querySelector("[data-type]");
    const fullAsk = typed.textContent;
    const parts = demo.querySelectorAll(".dc-answer > *");
    let paused = false;
    let visible = false;
    let playing = false;
    let take = 0;

    // Waits that stand still while paused or out of view, and give up when a newer take started.
    const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
    async function wait(ms, id) {
      let left = ms;
      while (left > 0) {
        if (id !== take) throw new Error("stale");
        await sleep(50);
        if (!paused && visible) left -= 50;
      }
      if (id !== take) throw new Error("stale");
    }
    const scene = (name) => (demo.dataset.scene = name);
    const step = (n) => steps.forEach((li, i) => li.classList.toggle("is-now", i === n));
    function aim(el, dx = 0.3, dy = 0.55) {
      const s = stage.getBoundingClientRect();
      const r = el.getBoundingClientRect();
      pointer.style.setProperty("--x", ((r.left - s.left + r.width * dx) / s.width) * 100 + "%");
      pointer.style.setProperty("--y", ((r.top - s.top + r.height * dy) / s.height) * 100 + "%");
    }
    async function press(el, id) {
      pointer.style.transform = "scale(0.85)";
      el.classList.add("is-pressed");
      await wait(160, id);
      pointer.style.transform = "";
      el.classList.remove("is-pressed");
    }
    function setControl(state) {
      control.dataset.state = state;
      label.textContent = control.dataset[state];
      control.hidden = false;
    }

    async function play() {
      const id = ++take;
      playing = true;
      paused = false;
      setControl("pause");
      steps[0].parentElement.classList.add("is-playing");
      parts.forEach((p) => p.classList.remove("is-in"));
      typed.textContent = fullAsk;
      try {
        step(0);
        scene("qr");
        await wait(1800, id);
        step(1);
        scene("scan");
        await wait(2400, id);
        scene("paired");
        await wait(1900, id);
        step(2);
        const target = demo.querySelector(".dp-pick--target");
        pointer.style.transition = "none";
        pointer.style.setProperty("--x", "70%");
        pointer.style.setProperty("--y", "92%");
        scene("pick");
        void pointer.offsetWidth;
        pointer.style.transition = "";
        aim(target);
        await wait(1100, id);
        await press(target, id);
        scene("add");
        const button = demo.querySelector(".dp-btn--add");
        aim(button, 0.4, 0.6);
        await wait(1000, id);
        await press(button, id);
        scene("wait");
        await wait(1700, id);
        scene("ok");
        await wait(1900, id);
        step(-1);
        steps[0].parentElement.classList.remove("is-playing");
        typed.textContent = "";
        scene("ask");
        await wait(700, id);
        for (let i = 1; i <= fullAsk.length; i++) {
          typed.textContent = fullAsk.slice(0, i);
          await wait(38, id);
        }
        await wait(500, id);
        scene("tool");
        await wait(1500, id);
        scene("answer");
        for (const p of parts) {
          p.classList.add("is-in");
          await wait(500, id);
        }
        scene("end");
        playing = false;
        setControl("again");
      } catch {
        // A newer take started.
      }
    }

    control.addEventListener("click", () => {
      if (!playing) return play();
      paused = !paused;
      setControl(paused ? "play" : "pause");
    });
    let started = false;
    new IntersectionObserver(
      (entries) => {
        visible = entries[0].isIntersecting;
        if (visible && !started) {
          started = true;
          play();
        }
      },
      { threshold: 0.45 },
    ).observe(stage);
  }

  // The app's screens: a tab list (arrows move between tabs); without JS every
  // screen shows, one under the other.
  const screens = document.querySelector("[data-screens]");
  if (screens) {
    const tabs = [...screens.querySelectorAll('[role="tab"]')];
    const list = screens.querySelector(".screens__tabs");
    // Each tab gets a thin bar that fills in SCREEN_MS; when it is full the next
    // screen shows (after the last, the first). Not with reduced motion.
    const SCREEN_MS = 5000;
    const auto = !reduceMotion;
    let current = tabs[0];
    let elapsed = 0;
    const bars = new Map();
    if (auto) {
      tabs.forEach((t) => {
        const bar = document.createElement("span");
        bar.className = "screens__bar";
        bar.setAttribute("aria-hidden", "true");
        bar.innerHTML = "<span></span>";
        t.append(bar);
        bars.set(t, bar.firstChild);
      });
    }
    // The next screen loads ahead, so it is already there when its turn comes.
    const preload = () => {
      const next = tabs[(tabs.indexOf(current) + 1) % tabs.length];
      for (const t of [current, next]) {
        const img = document.getElementById(t.getAttribute("aria-controls")).querySelector("img");
        if (img) img.loading = "eager";
      }
    };
    const paint = () => bars.forEach((fill, t) => (fill.style.transform = `scaleX(${t === current ? elapsed / SCREEN_MS : 0})`));
    const show = (tab, focus) => {
      current = tab;
      elapsed = 0;
      tabs.forEach((t) => {
        const on = t === tab;
        t.setAttribute("aria-selected", String(on));
        t.tabIndex = on ? 0 : -1;
        document.getElementById(t.getAttribute("aria-controls")).hidden = !on;
      });
      if (focus) tab.focus();
      if (auto && !pause.away) preload();
      // On a phone the tabs scroll sideways: keep the picked one in sight, moving
      // only the list (never the page, as the screens also change on their own).
      if (list.scrollWidth > list.clientWidth) {
        list.scrollTo({ left: tab.parentElement.offsetLeft - (list.clientWidth - tab.offsetWidth) / 2, behavior: reduceMotion ? "auto" : "smooth" });
      }
      paint();
    };
    // Why it is paused now: the mouse on the screen, a click or tap on it, the
    // browser tab hidden, the section out of sight.
    const pause = { hover: false, click: false, hidden: document.hidden, away: true };
    if (auto) {
      const panels = screens.querySelector(".screens__panels");
      let last = 0;
      const tick = (now) => {
        const running = !Object.values(pause).some(Boolean);
        if (running && last) {
          // Slow frames still count (a busy or throttled page); a long gap does not.
          elapsed += Math.min(now - last, 1000);
          if (elapsed >= SCREEN_MS) show(tabs[(tabs.indexOf(current) + 1) % tabs.length], false);
          else paint();
        }
        last = running ? now : 0;
        requestAnimationFrame(tick);
      };
      requestAnimationFrame(tick);
      const mark = () => screens.classList.toggle("is-paused", Object.values(pause).some(Boolean));
      panels.addEventListener("pointerenter", (e) => {
        if (e.pointerType === "mouse") (pause.hover = true), mark();
      });
      panels.addEventListener("pointerleave", (e) => {
        if (e.pointerType === "mouse") (pause.hover = false), (pause.click = false), mark();
      });
      // A click pauses; on a touch screen a tap pauses and the next one resumes.
      let pointer = "mouse";
      panels.addEventListener("pointerdown", (e) => (pointer = e.pointerType));
      panels.addEventListener("click", (e) => {
        if (e.target.closest("a")) return;
        pause.click = pointer === "mouse" ? true : !pause.click;
        mark();
      });
      document.addEventListener("visibilitychange", () => ((pause.hidden = document.hidden), mark()));
      new IntersectionObserver(([entry]) => {
        pause.away = !entry.isIntersecting;
        if (!pause.away) preload();
        mark();
      }, { threshold: 0.3 }).observe(panels);
      // Picking a screen starts its bar again and drops a click's pause.
      tabs.forEach((t) => t.addEventListener("click", () => ((pause.click = false), mark())));
    }
    tabs.forEach((tab, i) => {
      tab.addEventListener("click", () => show(tab, false));
      tab.addEventListener("keydown", (event) => {
        const next = { ArrowDown: 1, ArrowRight: 1, ArrowUp: -1, ArrowLeft: -1 }[event.key];
        if (next) {
          event.preventDefault();
          show(tabs[(i + next + tabs.length) % tabs.length], true);
        } else if (event.key === "Home" || event.key === "End") {
          event.preventDefault();
          show(tabs[event.key === "Home" ? 0 : tabs.length - 1], true);
        }
      });
    });
    tabs.forEach((t, i) => (document.getElementById(t.getAttribute("aria-controls")).hidden = i !== 0));
    screens.classList.add("is-ready");
  }

  if (!chat) return;

  const scenarios = {
    pt: [
      {
        title: "Resumo do grupo da família",
        ask: "Resume o que rolou hoje no grupo da família",
        working: "Lendo as mensagens de Família",
        done: "Leu 52 mensagens de Família",
        reply: [
          "<p>Hoje no grupo da família:</p>",
          "<ul><li><strong>Almoço de domingo:</strong> na casa da vó, às 13h.</li><li><strong>Sobremesa:</strong> a Carla leva. Ainda falta alguém para as bebidas.</li><li><strong>Fotos:</strong> o Pedro mandou 12 da formatura.</li></ul>",
          "<p>Ninguém te marcou. Quer que eu avise no grupo que você leva as bebidas?</p>",
        ],
      },
      {
        title: "Endereço da Ana",
        ask: "Qual endereço a Ana mandou em março?",
        working: "Procurando nas conversas com a Ana",
        done: "Achou 1 mensagem de 14 de março",
        reply: [
          "<p>A Ana mandou em 14 de março, às 18h07:</p>",
          "<blockquote>Rua Harmonia, 412, apto 31, Vila Madalena. O porteiro já sabe que você vem.</blockquote>",
          "<p>Quer que eu encaminhe para alguém?</p>",
        ],
      },
      {
        title: "Áudio do Marcos",
        ask: "Transcreve o áudio que o Marcos mandou agora",
        working: "Transcrevendo um áudio de 1:42 neste computador",
        done: "Transcreveu um áudio de 1:42 neste computador",
        reply: [
          "<p>O Marcos disse:</p>",
          "<blockquote>Fala! Consegui adiantar a reunião com o cliente para quinta, às 10h. Se puder, leva o contrato impresso, que eles querem assinar lá mesmo.</blockquote>",
          "<p><strong>Resumindo:</strong> reunião na quinta às 10h, e levar o contrato impresso.</p>",
        ],
      },
    ],
    es: [
      {
        title: "Resumen del grupo de la familia",
        ask: "Resume lo que pasó hoy en el grupo de la familia",
        working: "Leyendo los mensajes de Familia",
        done: "Leyó 52 mensajes de Familia",
        reply: [
          "<p>Hoy en el grupo de la familia:</p>",
          "<ul><li><strong>Almuerzo del domingo:</strong> en casa de la abuela, a la 1 de la tarde.</li><li><strong>Postre:</strong> lo lleva Carla. Todavía falta alguien para las bebidas.</li><li><strong>Fotos:</strong> Pedro mandó 12 de la graduación.</li></ul>",
          "<p>Nadie te mencionó. ¿Quieres que avise en el grupo que tú llevas las bebidas?</p>",
        ],
      },
      {
        title: "Dirección de Ana",
        ask: "¿Qué dirección mandó Ana en marzo?",
        working: "Buscando en las conversaciones con Ana",
        done: "Encontró 1 mensaje del 14 de marzo",
        reply: [
          "<p>Ana la mandó el 14 de marzo, a las 18:07:</p>",
          "<blockquote>Calle Armonía 412, depto. 31. El portero ya sabe que vienes.</blockquote>",
          "<p>¿Quieres que se la reenvíe a alguien?</p>",
        ],
      },
      {
        title: "Audio de Marcos",
        ask: "Transcribe el audio que Marcos acaba de mandar",
        working: "Transcribiendo un audio de 1:42 en esta computadora",
        done: "Transcribió un audio de 1:42 en esta computadora",
        reply: [
          "<p>Marcos dijo:</p>",
          "<blockquote>¡Hola! Logré adelantar la reunión con el cliente para el jueves a las 10. Si puedes, lleva el contrato impreso, que lo quieren firmar ahí mismo.</blockquote>",
          "<p><strong>En resumen:</strong> reunión el jueves a las 10 y llevar el contrato impreso.</p>",
        ],
      },
    ],
    en: [
      {
        title: "Family group summary",
        ask: "Sum up what happened in the family group today",
        working: "Reading the messages in Family",
        done: "Read 52 messages from Family",
        reply: [
          "<p>Today in the family group:</p>",
          "<ul><li><strong>Sunday lunch:</strong> at Grandma’s, at 1 pm.</li><li><strong>Dessert:</strong> Carla’s bringing it. Someone still needs to bring drinks.</li><li><strong>Photos:</strong> Pedro sent 12 from the graduation.</li></ul>",
          "<p>Nobody mentioned you. Want me to tell the group you’ll bring the drinks?</p>",
        ],
      },
      {
        title: "Ana’s address",
        ask: "What address did Ana send in March?",
        working: "Searching the chats with Ana",
        done: "Found 1 message from March 14",
        reply: [
          "<p>Ana sent it on March 14, at 6:07 pm:</p>",
          "<blockquote>412 Harmony Street, apartment 31. The doorman knows you’re coming.</blockquote>",
          "<p>Want me to forward it to someone?</p>",
        ],
      },
      {
        title: "Voice message from Marcos",
        ask: "Transcribe the voice message Marcos just sent",
        working: "Transcribing a 1:42 voice message on this computer",
        done: "Transcribed a 1:42 voice message on this computer",
        reply: [
          "<p>Marcos said:</p>",
          "<blockquote>Hey! I managed to move the client meeting up to Thursday at 10. If you can, bring a printed copy of the contract, they want to sign it right there.</blockquote>",
          "<p><strong>In short:</strong> meeting on Thursday at 10 am, and bring the printed contract.</p>",
        ],
      },
    ],
  }[lang];

  const body = chat.querySelector(".chat__body");
  const title = chat.querySelector(".chat__title");
  const chips = document.querySelectorAll("[data-scenario]");
  const logo = '<svg viewBox="0 0 64 64" aria-hidden="true"><use href="#i-logo"/></svg>';
  const chevron = '<svg class="tool__chevron" viewBox="0 0 24 24" aria-hidden="true"><use href="#i-chevron"/></svg>';
  let run = 0;

  function el(html) {
    const t = document.createElement("template");
    t.innerHTML = html.trim();
    return t.content.firstElementChild;
  }

  function add(node, parent = body) {
    if (!reduceMotion) node.classList.add("enter");
    parent.appendChild(node);
    return node;
  }

  function wait(ms, id) {
    return new Promise((resolve, reject) => setTimeout(() => (id === run ? resolve() : reject()), reduceMotion ? 0 : ms));
  }

  async function play(index) {
    const id = ++run;
    const s = scenarios[index];
    chips.forEach((chip) => chip.setAttribute("aria-pressed", String(Number(chip.dataset.scenario) === index)));
    title.textContent = s.title;
    body.replaceChildren();
    try {
      add(el('<div class="ask"></div>')).textContent = s.ask;
      await wait(550, id);
      const tool = add(el('<div class="tool"><span class="tool__spin" aria-hidden="true"></span><span></span></div>'));
      tool.lastChild.textContent = s.working;
      await wait(1100, id);
      tool.firstChild.replaceWith(el(logo));
      tool.lastChild.textContent = s.done;
      tool.appendChild(el(chevron));
      await wait(350, id);
      const answer = body.appendChild(el('<div class="answer"></div>'));
      for (const part of s.reply) {
        add(el(part), answer);
        await wait(380, id);
      }
    } catch {
      // A newer scenario took over.
    }
  }

  chips.forEach((chip) => chip.addEventListener("click", () => play(Number(chip.dataset.scenario))));
  play(0);
})();
