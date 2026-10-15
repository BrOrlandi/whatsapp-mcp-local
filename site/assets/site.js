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
      const text = document.getElementById(button.dataset.copy).textContent.trim();
      try {
        await navigator.clipboard.writeText(text);
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

  // The explainer plays muted by itself; the button turns its narration on.
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
    // past the end of a shorter cut lands near its end.
    // The page's own cut, chosen as it opens, keeps the autoplay; going back
    // from a cut that failed resumes where the visitor was.
    const switchTo = (cut, opening = false, resume = null) => {
      const from = current();
      if (cut === from) return;
      const at = resume ? resume.at : video.currentTime;
      const playing = resume ? resume.playing : !video.paused && !video.ended;
      if (!opening) video.autoplay = false;
      const loaded = () => {
        video.removeEventListener("error", failed);
        const end = video.duration;
        if (at > 0) video.currentTime = Number.isFinite(end) ? Math.min(at, Math.max(0, end - 0.5)) : at;
        if (playing) video.play().catch(() => {});
      };
      // A cut that does not load goes back to the one before, and loses its button.
      const failed = () => {
        video.removeEventListener("loadedmetadata", loaded);
        buttons.get(cut)?.remove();
        buttons.delete(cut);
        if (from) switchTo(from, false, { at, playing });
      };
      video.addEventListener("loadedmetadata", loaded, { once: true });
      video.addEventListener("error", failed, { once: true });
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
        done: "Transcreveu um áudio de 1:42 sem tirar do computador",
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
        done: "Transcribió un audio de 1:42 sin sacarlo de la computadora",
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
        done: "Transcribed a 1:42 voice message without it leaving the computer",
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
