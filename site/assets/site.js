(() => {
  const REPO = "BrOrlandi/whatsapp-mcp-local";
  const reduceMotion = window.matchMedia("(prefers-reduced-motion: reduce)").matches;

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
        const date = new Date(release.published_at).toLocaleDateString("pt-BR", { day: "numeric", month: "long", year: "numeric" });
        document.querySelectorAll("[data-version]").forEach((el) => {
          el.textContent = `Versão ${version}, de ${date}`;
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
        label.textContent = "Copiado";
      } catch {
        label.textContent = "Selecione e copie";
      }
      setTimeout(() => (label.textContent = "Copiar"), 2000);
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

  // The hero conversation: a request, what the app did in WhatsApp, and the answer.
  const chat = document.querySelector("[data-chat]");
  if (!chat) return;

  const scenarios = [
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
  ];

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
