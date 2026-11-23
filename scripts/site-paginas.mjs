// Writes the site's recipe and tool pages, in Portuguese, Spanish and English,
// and the sitemap:
//
//   site/receitas/index.html, site/receitas/<recipe>/index.html   (and es/, en/)
//   site/ferramentas/index.html                                     (and es/, en/)
//   site/sitemap.xml, site/robots.txt
//
//   node scripts/site-paginas.mjs
//
// The recipes are the panel's own, word for word in each language
// (scripts/site-dados/receitas.<lang>.json, saved by scripts/site-telas.sh from
// the running panel). The tool list is the panel's too (ferramentas.json); the
// plain-language words for each tool are in ferramentas-simples.json, and the
// page is refused when the two lists differ, so a new tool cannot go missing.
// Everything is plain HTML, readable without JS. Only these pages and the
// sitemap are written; nothing else in site/ is touched.
import fs from "node:fs";
import path from "node:path";

const ROOT = path.resolve(path.dirname(new URL(import.meta.url).pathname), "..");
const SITE = path.join(ROOT, "site");
const DATA = path.join(ROOT, "scripts", "site-dados");
const ORIGIN = "https://whatsappmcp.com.br";
const REPO = "https://github.com/BrOrlandi/whatsapp-mcp-local";
const LANGS = ["pt", "es", "en"];

const read = (f) => JSON.parse(fs.readFileSync(path.join(DATA, f), "utf8"));
const esc = (s) => String(s).replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;").replace(/"/g, "&quot;");
const slug = (s) => s.normalize("NFD").replace(/[̀-ͯ]/g, "").toLowerCase().replace(/[^a-z0-9]+/g, "-").replace(/^-|-$/g, "");
const prefix = (lang) => (lang === "pt" ? "" : `/${lang}`);
function describe(text, max = 158) {
  if (text.length <= max) return text;
  const cut = text.slice(0, max - 1);
  return cut.slice(0, cut.lastIndexOf(" ")).replace(/[,;:.]$/, "") + "…";
}

// --- data and checks -------------------------------------------------------

const tools = read("ferramentas.json");
const simple = read("ferramentas-simples.json");
const recipes = Object.fromEntries(LANGS.map((l) => [l, read(`receitas.${l}.json`)]));

const listed = simple.groups.flatMap((g) => g.tools.map((t) => t.name));
const known = new Set(tools.map((t) => t.name));
const missing = [...known].filter((n) => !listed.includes(n));
const unknown = listed.filter((n) => !known.has(n));
const twice = listed.filter((n, i) => listed.indexOf(n) !== i);
if (missing.length || unknown.length || twice.length) {
  console.error("ferramentas-simples.json does not match the panel's tool list:");
  if (missing.length) console.error("  missing:", missing.join(", "));
  if (unknown.length) console.error("  not in the app:", unknown.join(", "));
  if (twice.length) console.error("  listed twice:", twice.join(", "));
  process.exit(1);
}
for (const l of LANGS) {
  if (recipes[l].length !== recipes.pt.length) throw new Error(`receitas.${l}.json has ${recipes[l].length} recipes, receitas.pt.json ${recipes.pt.length}`);
}
const toolInfo = Object.fromEntries(simple.groups.flatMap((g) => g.tools.map((t) => [t.name, { ...t, group: g.id }])));
// The recipes' addresses come from their Portuguese titles, the same in every language.
const recipeSlugs = recipes.pt.map((r) => slug(r.title));
// Which recipes use each tool, for links both ways.
const usedBy = {};
recipes.pt.forEach((r, i) => r.uses.forEach((u) => (usedBy[u] ||= []).push(i)));

// --- words -----------------------------------------------------------------

const W = {
  pt: {
    html: "pt-BR", og: "pt_BR", skip: "Pular para o conteúdo", langNav: "Idioma", mainNav: "Principal", footNav: "Rodapé",
    how: "Como funciona", recipes: "Receitas", tools: "Ferramentas", help: "Ajuda", server: "Versão servidor", download: "Baixar",
    footer: "Código aberto, licença MIT. Feito por", whatsNew: "O que mudou", support: "Apoie o projeto",
    recipesTitle: "Receitas para usar o WhatsApp com IA",
    recipesHead: "Receitas",
    recipesLead: "Pedidos prontos para colar na sua ferramenta de IA: agendar mensagens, vigiar palavras, resumir grupos, ler documentos e mais. São as mesmas receitas que o app traz.",
    recipesDesc: "Pedidos prontos para usar o WhatsApp no Claude, no Codex e em outras ferramentas de IA: agendar mensagens, vigiar palavras-chave, resumir grupos, ler documentos e mais.",
    recipeTitle: (t) => `${t}: receita do WhatsApp MCP`,
    readRecipe: "Ver a receita",
    when: "Quando", uses: "Ferramentas que usa", prompt: "O texto para colar", copy: "Copiar", caveat: "Atenção:",
    howTitle: "Como usar",
    howSteps: [
      `<a href="/download">Baixe o WhatsApp MCP</a> e conecte o seu WhatsApp com o QR code.`,
      "Conecte a sua ferramenta de IA: o Claude Desktop e o Claude Code com um clique; as outras com um texto pronto.",
      "Cole o texto no chat da ferramenta, trocando o que estiver entre colchetes. Quando a receita tem horário, quem espera a hora é a ferramenta de IA: use uma que saiba agendar tarefas.",
    ],
    more: "Outras receitas", allRecipes: "Todas as receitas", crumbs: "Receitas",
    toolsTitle: (n) => `As ${n} ferramentas do WhatsApp MCP`,
    toolsHead: (n) => `O que a sua IA pode fazer no WhatsApp`,
    toolsLead: (n) => `As ${n} ferramentas que o WhatsApp MCP dá à sua ferramenta de IA, em palavras simples. Ela escolhe qual usar a partir do que você pede.`,
    toolsDesc: (n) => `As ${n} ferramentas do WhatsApp MCP explicadas em palavras simples: ler e procurar conversas, enviar e responder, transcrever áudios, organizar e administrar grupos.`,
    inRecipes: "Nas receitas:", toolsPrompt: "O nome que a IA vê", groupsNav: "Grupos de ferramentas",
    ctaTitle: "Baixe o WhatsApp MCP Local", ctaText: "Grátis, para macOS, Windows e Linux.", ctaButton: "Baixar o app",
    toolsLink: "Ver todas as ferramentas",
    recipesOnTools: "Para ver as ferramentas trabalhando juntas, veja as",
  },
  es: {
    html: "es", og: "es_LA", skip: "Saltar al contenido", langNav: "Idioma", mainNav: "Principal", footNav: "Pie de página",
    how: "Cómo funciona", recipes: "Recetas", tools: "Herramientas", help: null, server: "Versión servidor", download: "Descargar",
    footer: "Código abierto, licencia MIT. Hecho por", whatsNew: "Novedades", support: "Apoya el proyecto",
    recipesTitle: "Recetas para usar WhatsApp con IA",
    recipesHead: "Recetas",
    recipesLead: "Pedidos listos para pegar en tu herramienta de IA: programar mensajes, vigilar palabras, resumir grupos, leer documentos y más. Son las mismas recetas que trae la app.",
    recipesDesc: "Pedidos listos para usar WhatsApp en Claude, en Codex y en otras herramientas de IA: programar mensajes, vigilar palabras clave, resumir grupos, leer documentos y más.",
    recipeTitle: (t) => `${t}: receta de WhatsApp MCP`,
    readRecipe: "Ver la receta",
    when: "Cuándo", uses: "Herramientas que usa", prompt: "El texto para pegar", copy: "Copiar", caveat: "Atención:",
    howTitle: "Cómo usarla",
    howSteps: [
      `<a href="/es/download">Descarga WhatsApp MCP</a> y conecta tu WhatsApp con el código QR.`,
      "Conecta tu herramienta de IA: Claude Desktop y Claude Code con un clic; las demás con un texto listo.",
      "Pega el texto en el chat de la herramienta, cambiando lo que esté entre corchetes. Cuando la receta tiene horario, quien espera la hora es la herramienta de IA: usa una que sepa programar tareas.",
    ],
    more: "Otras recetas", allRecipes: "Todas las recetas", crumbs: "Recetas",
    toolsTitle: (n) => `Las ${n} herramientas de WhatsApp MCP`,
    toolsHead: (n) => `Lo que tu IA puede hacer en WhatsApp`,
    toolsLead: (n) => `Las ${n} herramientas que WhatsApp MCP le da a tu herramienta de IA, en palabras simples. Ella elige cuál usar a partir de lo que pides.`,
    toolsDesc: (n) => `Las ${n} herramientas de WhatsApp MCP explicadas en palabras simples: leer y buscar conversaciones, enviar y responder, transcribir audios, organizar y administrar grupos.`,
    inRecipes: "En las recetas:", toolsPrompt: "El nombre que ve la IA", groupsNav: "Grupos de herramientas",
    ctaTitle: "Descarga WhatsApp MCP Local", ctaText: "Gratis, para macOS, Windows y Linux.", ctaButton: "Descargar la app",
    toolsLink: "Ver todas las herramientas",
    recipesOnTools: "Para ver las herramientas trabajando juntas, mira las",
  },
  en: {
    html: "en", og: "en_US", skip: "Skip to content", langNav: "Language", mainNav: "Main", footNav: "Footer",
    how: "How it works", recipes: "Recipes", tools: "Tools", help: null, server: "Server version", download: "Download",
    footer: "Open source, MIT license. Made by", whatsNew: "What’s new", support: "Support the project",
    recipesTitle: "Recipes for using WhatsApp with AI",
    recipesHead: "Recipes",
    recipesLead: "Ready-made requests to paste into your AI tool: schedule messages, watch for words, sum up groups, read documents and more. They are the same recipes the app comes with.",
    recipesDesc: "Ready-made requests for using WhatsApp in Claude, Codex and other AI tools: schedule messages, watch for keywords, sum up groups, read documents and more.",
    recipeTitle: (t) => `${t}: a WhatsApp MCP recipe`,
    readRecipe: "See the recipe",
    when: "When", uses: "Tools it uses", prompt: "The text to paste", copy: "Copy", caveat: "Warning:",
    howTitle: "How to use it",
    howSteps: [
      `<a href="/en/download">Download WhatsApp MCP</a> and connect your WhatsApp with the QR code.`,
      "Connect your AI tool: Claude Desktop and Claude Code with one click; the others with a ready-made text.",
      "Paste the text into the tool’s chat, replacing whatever is in square brackets. When a recipe has a time, it’s the AI tool that waits for it: use one that can schedule tasks.",
    ],
    more: "More recipes", allRecipes: "All recipes", crumbs: "Recipes",
    toolsTitle: (n) => `The ${n} tools of WhatsApp MCP`,
    toolsHead: (n) => `What your AI can do in WhatsApp`,
    toolsLead: (n) => `The ${n} tools WhatsApp MCP gives your AI tool, in plain words. It picks which one to use from what you ask.`,
    toolsDesc: (n) => `The ${n} tools of WhatsApp MCP explained in plain words: read and search chats, send and reply, transcribe voice messages, keep track and manage groups.`,
    inRecipes: "In the recipes:", toolsPrompt: "The name the AI sees", groupsNav: "Tool groups",
    ctaTitle: "Download WhatsApp MCP Local", ctaText: "Free, for macOS, Windows and Linux.", ctaButton: "Download the app",
    toolsLink: "See all the tools",
    recipesOnTools: "To see the tools working together, have a look at the",
  },
};

// --- page shell ------------------------------------------------------------

const SPRITE = fs.readFileSync(path.join(DATA, "sprite.svg"), "utf8").trim();
// The Portuguese pages are the default addresses: the same language pick as the home page's.
const REDIRECT = `  <script>
    (function () {
      var keep = /[?&]lang=pt\\b/.test(location.search), lang = null;
      try {
        if (keep) localStorage.setItem("lang", "pt-BR");
        lang = localStorage.getItem("lang");
      } catch (e) {}
      if (keep || /bot|crawl|spider|slurp/i.test(navigator.userAgent)) return;
      if (lang !== "pt-BR" && lang !== "es" && lang !== "en") {
        lang = null;
        var wanted = navigator.languages && navigator.languages.length ? navigator.languages : [navigator.language || ""];
        for (var i = 0; i < wanted.length && !lang; i++) {
          var code = String(wanted[i]).slice(0, 2).toLowerCase();
          lang = code === "pt" ? "pt-BR" : code === "es" || code === "en" ? code : null;
        }
      }
      if (lang === "es" || lang === "en") location.replace("/" + lang + location.pathname.replace(/\\/+$/, "") + location.search + location.hash);
    })();
  </script>
`;

function page({ lang, route, title, description, active, head, body, jsonld }) {
  const w = W[lang];
  const url = (l) => ORIGIN + prefix(l) + route;
  const langLinks = [
    ["pt", "pt-BR", "br", "Português"],
    ["es", "es", "es", "Español"],
    ["en", "en", "us", "English"],
  ]
    .map(([l, code, flag, name]) => {
      const href = l === "pt" ? `${route}?lang=pt` : `/${l}${route}`;
      return `      <a href="${href}" lang="${code}" hreflang="${code}"${l === lang ? ' aria-current="true"' : ""}><svg class="langs__flag" aria-hidden="true" focusable="false"><use href="#flag-${flag}"/></svg>${name}</a>`;
    })
    .join("\n");
  const p = prefix(lang);
  const nav = [
    `<a class="nav__link nav__link--page nav__link--mid" href="${p || "/"}#como-funciona">${w.how}</a>`,
    // The recipes and the tools live inside the help (pt only), so its link is the current one there.
    w.help ? `<a class="nav__link nav__link--page" href="/ajuda"${active ? ' aria-current="page"' : ""}>${w.help}</a>` : "",
    `<a class="nav__link nav__link--page" href="${p}/servidor">${w.server}</a>`,
    `<a class="nav__link nav__link--gh" href="${REPO}"><svg aria-hidden="true"><use href="#i-github"/></svg><span>GitHub</span></a>`,
    `<a class="btn btn--mint btn--sm" href="${p}/download">${w.download}</a>`,
  ]
    .filter(Boolean)
    .map((a) => `        ${a}`)
    .join("\n");
  return `<!doctype html>
<html lang="${w.html}">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
${lang === "pt" ? REDIRECT : ""}  <title>${esc(title)}</title>
  <meta name="description" content="${esc(description)}">
  <link rel="canonical" href="${url(lang)}">
  <link rel="alternate" hreflang="pt-BR" href="${url("pt")}">
  <link rel="alternate" hreflang="es" href="${url("es")}">
  <link rel="alternate" hreflang="en" href="${url("en")}">
  <link rel="alternate" hreflang="x-default" href="${url("pt")}">
  <meta name="theme-color" content="#073d35">
  <meta property="og:type" content="article">
  <meta property="og:locale" content="${w.og}">
  <meta property="og:site_name" content="WhatsApp MCP">
  <meta property="og:title" content="${esc(title)}">
  <meta property="og:description" content="${esc(description)}">
  <meta property="og:url" content="${url(lang)}">
  <meta property="og:image" content="${ORIGIN}/assets/og.png?v=2">
  <meta property="og:image:width" content="1200">
  <meta property="og:image:height" content="630">
  <meta name="twitter:card" content="summary_large_image">
  <link rel="icon" href="/favicon.svg" type="image/svg+xml">
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link rel="stylesheet" href="https://fonts.googleapis.com/css2?family=Schibsted+Grotesk:wght@400..800&family=JetBrains+Mono:wght@500&display=swap">
  <link rel="stylesheet" href="/assets/site.css">
  <script src="/assets/site.js" defer></script>
${jsonld ? `  <script type="application/ld+json">${JSON.stringify(jsonld)}</script>\n` : ""}</head>
<body>
  ${SPRITE}

  <a class="skip" href="#conteudo">${w.skip}</a>

  <div class="top">
    <nav class="wrap langs" aria-label="${w.langNav}">
      <svg class="langs__icon" viewBox="0 0 24 24" aria-hidden="true"><g fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3a14 14 0 0 1 0 18M12 3a14 14 0 0 0 0 18"/></g></svg>
${langLinks}
    </nav>
    <header class="wrap bar">
      <a class="brand" href="${p || "/"}"><svg viewBox="0 0 64 64" aria-hidden="true"><use href="#i-logo"/></svg>WhatsApp MCP</a>
      <nav class="nav" aria-label="${w.mainNav}">
${nav}
      </nav>
    </header>
    <div class="wrap page-head page-head--short" id="conteudo">
${head}
    </div>
  </div>

  <main>
${body}
    <section class="cta">
      <div class="wrap cta__inner">
        <div>
          <h2>${w.ctaTitle}</h2>
          <p>${w.ctaText}</p>
        </div>
        <a class="btn btn--mint btn--lg" href="${p}/download"><svg aria-hidden="true"><use href="#i-download"/></svg>${w.ctaButton}</a>
      </div>
    </section>
  </main>

  <footer class="foot">
    <div class="wrap foot__grid">
      <p>${w.footer} <a href="https://github.com/BrOrlandi">Bruno Orlandi</a>.</p>
      <nav aria-label="${w.footNav}">
        <a href="${REPO}">GitHub</a>
${w.help ? `        <a href="/ajuda">${w.help}</a>\n` : ""}        <a href="${p}/receitas">${w.recipes}</a>
        <a href="${p}/ferramentas">${w.tools}</a>
        <a href="${REPO}/blob/main/CHANGELOG.md">${w.whatsNew}</a>
        <a href="${p}/servidor">${w.server}</a>
        <a href="https://donate.stripe.com/8x200jdhA6c1d375jF9Ve06">${w.support}</a>
      </nav>
    </div>
  </footer>
</body>
</html>
`;
}

function write(rel, html) {
  const file = path.join(SITE, rel);
  fs.mkdirSync(path.dirname(file), { recursive: true });
  fs.writeFileSync(file, html);
}

const breadcrumbs = (items) => ({
  "@context": "https://schema.org",
  "@type": "BreadcrumbList",
  itemListElement: items.map(([name, url], i) => ({ "@type": "ListItem", position: i + 1, name, item: ORIGIN + url })),
});

// The trail above a page's title, and the same in JSON-LD: the help first, where
// the language has one, then the given [name, address] items.
function trail(w, items) {
  const all = [...(w.help ? [[w.help, "/ajuda"]] : []), ...items];
  return {
    html: all.length ? `      <p class="crumbs">${all.map(([n, u]) => `<a href="${u}">${esc(n)}</a>`).join('<span aria-hidden="true"> › </span>')}</p>\n` : "",
    all,
  };
}

// --- recipes ---------------------------------------------------------------

function toolChip(lang, name) {
  const t = toolInfo[name];
  return `<a class="tool-chip" href="${prefix(lang)}/ferramentas#${name}">${esc(t.title[lang])}</a>`;
}

for (const lang of LANGS) {
  const w = W[lang];
  const p = prefix(lang);
  const list = recipes[lang];

  const cards = list
    .map(
      (r, i) => `      <a class="help-card recipe-card" href="${p}/receitas/${recipeSlugs[i]}">
        ${r.schedule ? `<span class="help-card__tag">${esc(r.schedule)}</span>` : ""}
        <h2>${esc(r.title)}</h2>
        <p>${esc(r.summary)}</p>
        <span class="help-card__more">${w.readRecipe}<svg aria-hidden="true"><use href="#i-arrow"/></svg></span>
      </a>`,
    )
    .join("\n");
  write(`${p.slice(1)}/receitas/index.html`.replace(/^\//, ""), page({
    lang,
    route: "/receitas",
    title: w.recipesTitle,
    description: w.recipesDesc,
    active: "receitas",
    head: `${trail(w, []).html}      <h1>${w.recipesHead}</h1>\n      <p>${w.recipesLead}</p>`,
    body: `    <div class="wrap help-list">\n${cards}\n    </div>\n    <p class="wrap recipes-more"><a href="${p}/ferramentas">${w.toolsLink}</a></p>\n`,
    jsonld: {
      "@context": "https://schema.org",
      "@type": "ItemList",
      name: w.recipesTitle,
      itemListElement: list.map((r, i) => ({ "@type": "ListItem", position: i + 1, name: r.title, url: `${ORIGIN}${p}/receitas/${recipeSlugs[i]}` })),
    },
  }));

  list.forEach((r, i) => {
    const id = `prompt-${recipeSlugs[i]}`;
    const others = list
      .map((o, j) => (j === i ? "" : `          <li><a href="${p}/receitas/${recipeSlugs[j]}">${esc(o.title)}</a></li>`))
      .filter(Boolean)
      .join("\n");
    const body = `    <article class="wrap article recipe-page">
      <dl class="recipe-facts">
${r.schedule ? `        <div><dt>${w.when}</dt><dd>${esc(r.schedule)}</dd></div>\n` : ""}        <div><dt>${w.uses}</dt><dd class="tool-chips">${r.uses.map((u) => toolChip(lang, u)).join(" ")}</dd></div>
      </dl>
      <h2>${w.prompt}</h2>
      <div class="cmd cmd--light">
        <pre id="${id}">${esc(r.prompt)}</pre>
        <button type="button" data-copy="${id}"><svg aria-hidden="true"><use href="#i-copy"/></svg><span>${w.copy}</span></button>
      </div>
${r.caveat ? `      <p class="caution"><strong>${w.caveat}</strong> ${esc(r.caveat)}</p>\n` : ""}      <h2>${w.howTitle}</h2>
      <ol class="steps">
${w.howSteps.map((s) => `        <li><p>${s}</p></li>`).join("\n")}
      </ol>
      <h2>${w.more}</h2>
      <ul class="recipe-links">
${others}
      </ul>
      <p class="article__more"><a href="${p}/receitas">${w.allRecipes}</a> · <a href="${p}/ferramentas">${w.toolsLink}</a></p>
    </article>
`;
    write(`${p.slice(1)}/receitas/${recipeSlugs[i]}/index.html`.replace(/^\//, ""), page({
      lang,
      route: `/receitas/${recipeSlugs[i]}`,
      title: w.recipeTitle(r.title),
      description: describe(r.summary),
      active: "receitas",
      head: `${trail(w, [[w.crumbs, `${p}/receitas`]]).html}      <h1>${esc(r.title)}</h1>\n      <p>${esc(r.summary)}</p>`,
      body,
      jsonld: breadcrumbs([...trail(w, [[w.crumbs, `${p}/receitas`]]).all, [r.title, `${p}/receitas/${recipeSlugs[i]}`]]),
    }));
  });
}

// --- tools -----------------------------------------------------------------

for (const lang of LANGS) {
  const w = W[lang];
  const p = prefix(lang);
  const n = tools.length;
  const jump = simple.groups.map((g) => `<a href="#${g.id}">${esc(g.title[lang])}</a>`).join("\n        ");
  const groups = simple.groups
    .map((g) => {
      const items = g.tools
        .map((t) => {
          const used = (usedBy[t.name] || []).map((i) => `<a href="${p}/receitas/${recipeSlugs[i]}">${esc(recipes[lang][i].title)}</a>`);
          return `        <article class="tool-item" id="${t.name}">
          <h3>${esc(t.title[lang])}</h3>
          <p>${esc(t.text[lang])}</p>
          <p class="tool-item__meta"><span title="${w.toolsPrompt}"><code>${t.name}</code></span>${used.length ? ` <span>${w.inRecipes} ${used.join(", ")}</span>` : ""}</p>
        </article>`;
        })
        .join("\n");
      return `      <section class="tool-group" id="${g.id}">
        <h2>${esc(g.title[lang])}</h2>
        <p class="tool-group__intro">${esc(g.intro[lang])}</p>
${items}
      </section>`;
    })
    .join("\n");
  write(`${p.slice(1)}/ferramentas/index.html`.replace(/^\//, ""), page({
    lang,
    route: "/ferramentas",
    title: w.toolsTitle(n),
    description: w.toolsDesc(n),
    active: "ferramentas",
    head: `${trail(w, []).html}      <h1>${w.toolsHead(n)}</h1>\n      <p>${w.toolsLead(n)}</p>`,
    body: `    <div class="wrap article tools-page">
      <nav class="tool-jump" aria-label="${w.groupsNav}">
        ${jump}
      </nav>
${groups}
      <p class="article__more">${w.recipesOnTools} <a href="${p}/receitas">${w.recipes.toLowerCase()}</a>.</p>
    </div>
`,
  }));
}

// --- sitemap ---------------------------------------------------------------

function pages(dir, out = []) {
  for (const e of fs.readdirSync(dir, { withFileTypes: true })) {
    const f = path.join(dir, e.name);
    if (e.isDirectory() && e.name !== "assets") pages(f, out);
    else if (e.name.endsWith(".html")) out.push(f);
  }
  return out;
}
const urls = pages(SITE)
  .map((f) => "/" + path.relative(SITE, f).replace(/index\.html$/, "").replace(/\.html$/, "").replace(/\/$/, ""))
  .map((u) => (u === "/" ? "/" : u))
  .sort((a, b) => a.localeCompare(b));
fs.writeFileSync(
  path.join(SITE, "sitemap.xml"),
  `<?xml version="1.0" encoding="UTF-8"?>\n<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">\n${urls.map((u) => `  <url><loc>${ORIGIN}${u === "/" ? "/" : u}</loc></url>`).join("\n")}\n</urlset>\n`,
);
fs.writeFileSync(path.join(SITE, "robots.txt"), `User-agent: *\nAllow: /\n\nSitemap: ${ORIGIN}/sitemap.xml\n`);
console.log(`${recipes.pt.length} recipes and ${tools.length} tools in ${LANGS.length} languages; ${urls.length} addresses in the sitemap`);
