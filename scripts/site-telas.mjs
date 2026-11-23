// The browser half of scripts/site-telas.sh: a forwarder that lets the
// browser-proxy (on another machine) open the instances on this Mac's
// 127.0.0.1, and the screenshots themselves, over the Chrome DevTools
// Protocol with nothing but Node's own fetch and WebSocket.
//
// The forwarder presents each request to the instance as a local one (the
// panel only answers to 127.0.0.1) and, in what comes back, writes the
// instance's temporary port as the app's own (47821), so the screens show the
// address a person really sees. Run it through site-telas.sh, not by hand.
import http from "node:http";
import fs from "node:fs";
import path from "node:path";
import { parseArgs } from "node:util";

const { values: opt } = parseArgs({
  options: {
    proxy: { type: "string" },
    ip: { type: "string" },
    lang: { type: "string" },
    out: { type: "string" },
    local: { type: "string" },
    server: { type: "string" },
    public: { type: "string" },
    scan: { type: "string" },
    only: { type: "string" },
    data: { type: "string" },
    bin: { type: "string" },
    home: { type: "string" },
  },
});

const APP_PORT = "47821";
const PUBLIC_URL = "https://whatsapp.example.com";
const WIDTH = 1120;
const HEIGHT = 840;
const SCALE = 1.5;
const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
fs.mkdirSync(opt.out, { recursive: true });

// --- forwarder -------------------------------------------------------------

function forward(target, listenPort, rewrites) {
  const [name, tport] = target.split(":");
  const thost = name === "localhost" ? "127.0.0.1" : name;
  const outside = `${opt.ip}:${listenPort}`;
  const server = http.createServer((req, res) => {
    const headers = { ...req.headers, host: target, "accept-encoding": "identity" };
    for (const h of ["origin", "referer"]) if (headers[h]) headers[h] = headers[h].replace(outside, target);
    const up = http.request({ host: thost, port: tport, method: req.method, path: req.url, headers }, (r) => {
      const h = { ...r.headers };
      if (h.location) h.location = h.location.replace(`//${target}`, `//${outside}`);
      const type = String(h["content-type"] || "");
      if (!/text\/html|javascript|json|text\/plain/.test(type)) {
        res.writeHead(r.statusCode, h);
        r.pipe(res);
        return;
      }
      const chunks = [];
      r.on("data", (c) => chunks.push(c));
      r.on("end", () => {
        let body = Buffer.concat(chunks).toString("utf8");
        for (const [from, to] of rewrites) body = body.replaceAll(from, to);
        delete h["content-length"];
        res.writeHead(r.statusCode, h);
        res.end(body);
      });
    });
    up.on("error", (err) => {
      console.error(`  forwarder: ${req.method} ${req.url}: ${err.message}`);
      res.writeHead(502);
      res.end();
    });
    req.pipe(up);
  });
  return new Promise((resolve) => server.listen(listenPort, opt.ip, () => resolve(server)));
}

// --- CDP -------------------------------------------------------------------

let ws;
let seq = 0;
const pending = new Map();
const listeners = new Set();

async function connect() {
  const info = await (await fetch(`${opt.proxy}/agente/site-telas/json/version`)).json();
  ws = new WebSocket(info.webSocketDebuggerUrl);
  await new Promise((resolve, reject) => {
    ws.onopen = resolve;
    ws.onerror = reject;
  });
  ws.onmessage = (event) => {
    const msg = JSON.parse(event.data);
    if (msg.id && pending.has(msg.id)) {
      const { resolve, reject } = pending.get(msg.id);
      pending.delete(msg.id);
      msg.error ? reject(new Error(msg.error.message)) : resolve(msg.result);
    } else if (msg.method) {
      listeners.forEach((l) => l(msg));
    }
  };
}

function send(method, params = {}, sessionId) {
  const id = ++seq;
  ws.send(JSON.stringify({ id, method, params, sessionId }));
  return new Promise((resolve, reject) => pending.set(id, { resolve, reject }));
}

let session;
let targetId;
const cmd = (method, params) => send(method, params, session);

async function evaluate(expression) {
  const r = await cmd("Runtime.evaluate", { expression, returnByValue: true, awaitPromise: true });
  return r.result.value;
}

async function until(expression, timeout = 20000) {
  const end = Date.now() + timeout;
  while (Date.now() < end) {
    try {
      if (await evaluate(expression)) return true;
    } catch {}
    await sleep(300);
  }
  return false;
}

async function go(url) {
  const loaded = new Promise((resolve) => {
    const l = (m) => {
      if (m.sessionId === session && m.method === "Page.loadEventFired") {
        listeners.delete(l);
        resolve();
      }
    };
    listeners.add(l);
    setTimeout(resolve, 15000);
  });
  await cmd("Page.navigate", { url });
  await loaded;
  await until("document.fonts ? document.fonts.status === 'loaded' : true", 5000);
  await sleep(900);
}

// height: CSS pixels to keep from the top (a window's worth by default);
// "full" keeps the whole page.
async function shot(name, height = HEIGHT) {
  if (opt.only && !opt.only.split(",").includes(name)) return;
  await evaluate("window.scrollTo(0, 0); document.activeElement && document.activeElement.blur(); true");
  const full = await evaluate("Math.ceil(document.documentElement.scrollHeight)");
  const h = height === "full" ? full : Math.min(full, height);
  const r = await cmd("Page.captureScreenshot", {
    format: "png",
    captureBeyondViewport: true,
    clip: { x: 0, y: 0, width: WIDTH, height: Math.max(h, 200), scale: 1 },
  });
  fs.writeFileSync(path.join(opt.out, `${name}.png`), Buffer.from(r.data, "base64"));
  console.log(`  ${name}`);
}

// The MCP handshake a client makes, so the panel lists it as connected.
async function mcpHello(target, client) {
  const r = await fetch(`http://${target}/mcp`, {
    method: "POST",
    headers: { "content-type": "application/json", accept: "application/json, text/event-stream" },
    body: JSON.stringify({ jsonrpc: "2.0", id: 1, method: "initialize", params: { protocolVersion: "2025-06-18", capabilities: {}, clientInfo: { name: client, version: "1" } } }),
  });
  await r.text();
}

// The first run's last click ("Ir para o painel").
async function finishSetup(target) {
  const r = await fetch(`http://${target}/instalacao/avancar`, {
    method: "POST",
    headers: { "content-type": "application/x-www-form-urlencoded", origin: `http://${target}` },
    body: "to=done",
    redirect: "manual",
  });
  if (r.status >= 400) throw new Error(`finishing the first run: HTTP ${r.status}`);
}

// The recipes and the tools as the panel shows them, for scripts/site-paginas.mjs:
// the site's pages carry the same words as the app, in each language.
const RECIPES = `[...document.querySelectorAll("article.recipe")].map((a) => ({
  title: a.querySelector(".recipe__title").textContent.trim(),
  summary: a.querySelector(".recipe__summary").textContent.trim(),
  uses: [...a.querySelectorAll(".recipe__tool:not(.recipe__tool--when)")].map((e) => e.textContent.trim()),
  schedule: (a.querySelector(".recipe__tool--when")?.textContent || "").replace(/^⏱\\s*/, "").trim(),
  prompt: a.querySelector(".recipe__prompt code").textContent.trim(),
  caveat: (a.querySelector(".recipe__caveat")?.textContent || "").replace(/^[^:]+:\\s*/, "").trim(),
}))`;
const TOOLS = `[...document.querySelectorAll("article.tool")].map((a) => ({
  name: a.querySelector(".tool__name").textContent.trim(),
  description: a.querySelector(".tool__desc").textContent.trim(),
}))`;

function saveData(file, value) {
  if (!opt.data || !value || !value.length) return;
  fs.mkdirSync(opt.data, { recursive: true });
  fs.writeFileSync(path.join(opt.data, file), JSON.stringify(value, null, 2) + "\n");
}

// --- the screens -----------------------------------------------------------

const servers = [];
try {
  const localPort = opt.local.split(":")[1];
  // The temporary program and HOME the screens would print, as the Mac app's own
  // and a plain user folder.
  const user = { pt: "voce", es: "tu", en: "you" }[opt.lang.slice(0, 2)];
  const paths = [];
  for (const [from, to] of [[opt.bin, "/Applications/WhatsApp MCP.app/Contents/MacOS/whatsapp-mcp-bridge"], [opt.home, `/Users/${user}`]]) {
    if (!from) continue;
    paths.push([fs.realpathSync(from), to], [from, to]);
  }
  servers.push(
    await forward(opt.local, 47991, [...paths, [`127.0.0.1:${localPort}`, `127.0.0.1:${APP_PORT}`], [`localhost:${localPort}`, `localhost:${APP_PORT}`]]),
    // The server's public address, as if it were the https one people give it.
    await forward(opt.public, 47992, [[`http://${opt.public}`, PUBLIC_URL], [opt.public, new URL(PUBLIC_URL).host]]),
  );
  const L = `http://${opt.ip}:47991`;
  const S = `http://${opt.ip}:47992`;

  await connect();
  ({ targetId } = await send("Target.createTarget", { url: "about:blank" }));
  ({ sessionId: session } = await send("Target.attachToTarget", { targetId, flatten: true }));
  await cmd("Page.enable");
  await cmd("Runtime.enable");
  await cmd("Emulation.setDeviceMetricsOverride", { width: WIDTH, height: HEIGHT, deviceScaleFactor: SCALE, mobile: false });
  await cmd("Emulation.setEmulatedMedia", { features: [{ name: "prefers-color-scheme", value: "light" }, { name: "prefers-reduced-motion", value: "reduce" }] });

  // The first run: the QR code, then the phone "reads" it.
  await go(`${L}/instalacao`);
  await until("(() => { const i = document.querySelector('img.qrcode'); return i && i.complete && i.naturalWidth > 0; })()");
  await sleep(600);
  await shot("instalacao");
  fs.writeFileSync(opt.scan, "");
  await until("location.pathname !== '/instalacao' || !document.querySelector('img.qrcode')", 30000);
  await sleep(9000);
  await shot("instalacao-conectado");

  // Connecting the AI tools.
  for (const [name, url] of [
    ["conectar-claude-desktop", "/conectar/claude-desktop"],
    ["conectar-claude-code", "/conectar/claude-code"],
    ["conectar-outra", "/conectar/outra"],
  ]) {
    await go(L + url);
    await shot(name);
  }

  // An AI tool connects: the MCP sees Claude Desktop, as when it really opens.
  await mcpHello(opt.local, "claude-ai");
  await finishSetup(opt.local);

  // The panel, every day.
  for (const [name, url, height] of [
    ["painel-conectar", "/"],
    ["painel-whatsapp", "/whatsapp"],
    ["painel-status", "/status"],
    ["painel-funcoes", "/funcoes"],
    ["painel-receitas", "/receitas"],
    ["painel-configuracoes", "/configuracoes", 1500],
  ]) {
    await go(L + url);
    await sleep(1200);
    await shot(name, height);
    if (url === "/receitas") saveData(`receitas.${opt.lang.slice(0, 2)}.json`, await evaluate(RECIPES));
    if (url === "/funcoes") saveData("ferramentas.json", await evaluate(TOOLS));
  }

  // The server mode, through its public address, signed in with the panel's password.
  await mcpHello(opt.server, "claude-ai");
  await finishSetup(opt.server);
  await go(`${S}/entrar`);
  await evaluate(`(() => { const i = document.querySelector("#password"); i.value = ${JSON.stringify(process.env.SITE_TELAS_PASSWORD || "")}; i.form.submit(); return true; })()`);
  if (!(await until("location.pathname !== '/entrar'", 15000))) throw new Error("could not sign in to the server's panel");
  await sleep(1500);
  for (const [name, url, height] of [
    ["servidor-conectar", "/"],
    ["servidor-conexoes", "/conexoes"],
  ]) {
    await go(S + url);
    await sleep(800);
    if (!(await evaluate(`location.pathname === ${JSON.stringify(url)}`))) throw new Error(`${url} did not open (signed out?)`);
    await shot(name, height);
  }
} finally {
  if (targetId) await send("Target.closeTarget", { targetId }).catch(() => {});
  if (ws) ws.close();
  servers.forEach((s) => s.closeAllConnections?.() || s.close());
}
process.exit(0);
