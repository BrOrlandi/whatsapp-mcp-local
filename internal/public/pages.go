package public

import (
	"html/template"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/brand"
)

// The pages this listener draws itself (the panel's sign-in, and the consent
// an AI tool sends the person to) share one look: the site's dark green
// around a card, with the logo above it. The colours are the panel's tokens
// (internal/panel/style.go) and the site's (site/assets/site.css), copied
// here, since the panel's stylesheet is not served to someone not signed in.
// Everything is inline: the pages' policy loads nothing from anywhere else.
const pageCSS = `*,*::before,*::after{box-sizing:border-box}
:root{color-scheme:light dark;
--deep:#073d35;--on-deep:#eef8f4;--mint:#2fc9a0;
--surface:#fff;--border:#dbe6e1;--border-strong:#c3d5cd;--card-border:transparent;
--text:#10241c;--text-soft:#2c443d;--muted:#54695f;
--brand:#0b6b5d;--brand-ink:#fff;--brand-soft:#e3f3ef;
--accent:#7d5e00;--accent-soft:#f8eec4;
--danger:#96201f;--danger-bg:#fdeceb;--danger-border:#f0c6c3;
--ring:#12a08c;--shadow:0 12px 40px rgba(0,0,0,.28)}
@media (prefers-color-scheme:dark){:root{
--deep:#052e28;--surface:#11211d;--border:#23413a;--border-strong:#2f544b;--card-border:#2f544b;
--text:#e4f1ec;--text-soft:#cbe0d8;--muted:#a7c0b8;
--brand:#2fc9a0;--brand-ink:#04211b;--brand-soft:#123029;
--accent:#e8c860;--accent-soft:#2d2610;
--danger:#ffaea7;--danger-bg:#361917;--danger-border:#67312e;
--shadow:0 12px 40px rgba(0,0,0,.5)}}
html{-webkit-text-size-adjust:100%}
body{margin:0;min-height:100vh;display:flex;flex-direction:column;align-items:center;padding:32px 16px 40px;background:var(--deep);color:var(--text);font:16px/1.55 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif}
.mast{display:flex;align-items:center;gap:10px;margin:0 0 20px;color:var(--on-deep);font-weight:800;font-size:1.125rem;letter-spacing:-.01em}
.mast svg{width:34px;height:34px;color:var(--mint);flex:none}
main{width:100%;max-width:460px;background:var(--surface);border:1px solid var(--card-border);border-radius:14px;padding:28px;box-shadow:var(--shadow)}
main.narrow{max-width:380px}
h1{font-size:1.35rem;line-height:1.25;margin:0 0 10px}
p{margin:0 0 14px}
.lead{color:var(--text-soft)}
.muted{color:var(--muted);font-size:.875rem}
.number{display:flex;align-items:center;gap:12px;background:var(--brand-soft);border-radius:10px;padding:12px 14px;margin:0 0 14px}
.number__icon{display:grid;place-items:center;width:38px;height:38px;border-radius:50%;background:var(--brand);color:var(--brand-ink);flex:none}
.number__icon svg{width:20px;height:20px}
.number__label{display:block;font-size:.8125rem;color:var(--muted)}
.number__value{display:block;font-weight:700;font-size:1.0625rem;letter-spacing:.02em;font-variant-numeric:tabular-nums;overflow-wrap:anywhere}
.who{border:1px solid var(--border);border-radius:10px;padding:10px 12px;margin:0 0 14px;font-size:.875rem;color:var(--text-soft);overflow-wrap:anywhere}
.who .muted{display:block;margin-top:4px}
.warn,.error{border-radius:10px;padding:10px 12px;margin:0 0 14px;font-size:.9375rem}
.warn{background:var(--accent-soft);color:var(--accent)}
.error{background:var(--danger-bg);color:var(--danger);border:1px solid var(--danger-border)}
fieldset{border:0;padding:0;margin:0 0 14px;min-width:0}
legend{font-weight:700;padding:0;margin:0 0 8px}
.choice{display:flex;gap:10px;align-items:flex-start;border:1px solid var(--border-strong);border-radius:10px;padding:10px 12px;margin:0 0 8px;cursor:pointer}
.choice:has(input:checked){border-color:var(--brand);background:var(--brand-soft)}
.choice input{accent-color:var(--brand);margin:5px 0 0;flex:none}
label.field{display:block;font-weight:700;margin:0 0 6px}
input[type=password]{display:block;width:100%;padding:10px 12px;font:inherit;color:inherit;background:var(--surface);border:1px solid var(--border-strong);border-radius:10px;margin:0 0 16px}
.actions{display:flex;flex-direction:row-reverse;gap:10px}
.step{margin:0 0 6px;font-size:.8125rem;font-weight:700;letter-spacing:.02em;color:var(--muted)}
button{flex:1;min-height:44px;padding:10px 14px;font:inherit;font-weight:700;border-radius:10px;cursor:pointer}
.allow{border:1px solid var(--brand);background:var(--brand);color:var(--brand-ink)}
.deny{border:1px solid var(--border-strong);background:transparent;color:var(--text)}
.wide{width:100%}
input:focus-visible,button:focus-visible{outline:2px solid var(--ring);outline-offset:2px}
.note{margin:16px 0 0}
main>:last-child{margin-bottom:0}
@media (max-width:460px){body{padding:20px 16px 32px}main{padding:22px 18px}.mast{margin-bottom:16px}}
`

// pageTop and pageBottom wrap each page: its title, the stylesheet, and the
// logo with the name over the card the page writes into.
const pageTop = `<!doctype html>
<html lang="{{.Lang}}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<meta name="theme-color" content="#073d35"><title>{{.Title}} · WhatsApp MCP</title><link rel="icon" href="/favicon.svg">
<style>` + pageCSS + `</style></head>
<body><div class="mast">{{mark}}<span>WhatsApp MCP</span></div>
`

const pageBottom = `
</body></html>`

// phoneIcon is the handset beside the number on the consent page.
const phoneIcon = `<svg viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path fill="none" stroke="currentColor" stroke-width="1.9" stroke-linecap="round" stroke-linejoin="round" d="M6.6 3.5h2.6l1.4 4-2 1.5a12 12 0 0 0 6.4 6.4l1.5-2 4 1.4v2.6a2 2 0 0 1-2.2 2A16.5 16.5 0 0 1 4.6 5.7a2 2 0 0 1 2-2.2Z"/></svg>`

// page parses one of these pages: body goes between the logo and the end.
func page(name, body string) *template.Template {
	return template.Must(template.New(name).Funcs(template.FuncMap{
		"mark":  brand.LogoMarkSVG,
		"phone": func() template.HTML { return phoneIcon },
	}).Parse(pageTop + body + pageBottom))
}
