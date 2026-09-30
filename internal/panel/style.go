package panel

import "strings"

// The stylesheet is v1's, unchanged, so the local panel and the hosted one
// look like the same product; extraCSS adds only what v1 has no page for: the
// WhatsApp-style previews and the pairing code.

const darkPalette = `
--bg:#0a1513;--surface:#11211d;--surface-soft:#152b26;--surface-sunken:#0d1b18;--border:#23413a;--border-strong:#2f544b;
--text:#e4f1ec;--text-soft:#cbe0d8;--muted:#a7c0b8;
--brand:#2fc9a0;--brand-strong:#4adcb4;--brand-ink:#04211b;--brand-soft:#123029;
--accent:#93b4ff;--accent-soft:#16224a;
--danger:#ffaea7;--danger-bg:#361917;--danger-border:#67312e;
--ok:#6fdcaa;--ok-bg:#0f3526;--ok-border:#1d5a40;
--warn:#f2ce85;--warn-bg:#352a10;--off:#a4b9bf;--off-bg:#1a292d;
--shadow:0 1px 2px rgba(0,0,0,.4),0 8px 24px rgba(0,0,0,.3);
--shadow-lift:0 12px 40px rgba(0,0,0,.5);
`

const darkMarker = "/*dark-palette*/"

var stylesheet = strings.NewReplacer(darkMarker, darkPalette).Replace(baseCSS + extraCSS)

const baseCSS = `*,*::before,*::after{box-sizing:border-box}
:root{
color-scheme:light dark;
--bg:#eef3f1;--surface:#fff;--surface-soft:#f4f9f7;--surface-sunken:#e9f0ed;--border:#dbe6e1;--border-strong:#c3d5cd;
--text:#10241c;--text-soft:#2c443d;--muted:#54695f;
--brand:#0b6b5d;--brand-strong:#0a8172;--brand-ink:#fff;--brand-soft:#e3f3ef;
--accent:#1d4ed8;--accent-soft:#e8eeff;
--danger:#96201f;--danger-bg:#fdeceb;--danger-border:#f0c6c3;
--ok:#0b6b45;--ok-bg:#e1f4ea;--ok-border:#b6e0c9;
--warn:#7a5200;--warn-bg:#fbf0d6;--off:#4f646b;--off-bg:#e8eef0;
--radius:14px;--radius-sm:10px;--ring:#12a08c;
--shadow:0 1px 2px rgba(16,36,28,.06),0 8px 24px rgba(16,36,28,.06);
--shadow-lift:0 12px 40px rgba(16,36,28,.18);
}
@media (prefers-color-scheme:dark){:root:not([data-theme=light]){/*dark-palette*/}}
:root[data-theme=light]{color-scheme:light}
:root[data-theme=dark]{color-scheme:dark;/*dark-palette*/}
html{-webkit-text-size-adjust:100%}
body{margin:0;background:var(--bg);color:var(--text);font:16px/1.6 system-ui,-apple-system,"Segoe UI",Roboto,sans-serif}
h1,h2,h3{line-height:1.25;margin:0;color:var(--text)}
h1{font-size:clamp(1.35rem,1.1rem + 1.2vw,1.7rem)}
h2{font-size:1.15rem}
h3{font-size:.95rem;text-transform:uppercase;letter-spacing:.06em;color:var(--muted)}
p{margin:.6em 0}
a{color:var(--brand-strong)}
code{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:.88em;background:var(--surface-sunken);border:1px solid var(--border);padding:1px 5px;border-radius:5px;overflow-wrap:anywhere}
pre{position:relative;overflow:auto;background:#07201b;color:#dffff4;padding:14px 16px;border-radius:var(--radius-sm);font-size:.85rem;line-height:1.55;margin:0}
pre code{background:none;border:0;padding:0;color:inherit;font-size:1em}

/* ---- shell ---- */
.shell{max-width:1000px;margin:0 auto;padding:0 clamp(16px,4vw,24px) 72px}
.masthead{display:flex;flex-wrap:wrap;align-items:center;justify-content:space-between;gap:12px;padding:20px 0 14px}
.masthead__tools{display:flex;align-items:center;gap:8px}
.theme{display:inline-flex}
.theme[hidden]{display:none}
.theme__select{appearance:none;-webkit-appearance:none;font:inherit;font-size:1rem;line-height:1;width:38px;height:38px;padding:0;text-align:center;text-align-last:center;border:1px solid var(--border-strong);border-radius:var(--radius-sm);background:var(--surface);color:var(--text);cursor:pointer}
.theme__select:hover{background:var(--surface-soft)}
.brand{display:flex;align-items:center;gap:12px;min-width:0;text-decoration:none;color:inherit}
.brand__mark{width:38px;height:38px;flex:none}
.brand__mark svg{width:100%;height:100%;display:block}
.brand__name{font-weight:700;font-size:1rem;color:var(--brand);letter-spacing:-.01em}
.brand__tagline{display:block;font-weight:400;font-size:.78rem;color:var(--muted);letter-spacing:0}
/* ---- tools (documentação) ---- */
.tools{display:grid;gap:14px;margin-top:24px}
.tool{border:1px solid var(--border);border-radius:var(--radius);background:var(--surface);box-shadow:var(--shadow);overflow:hidden}
.tool__head{display:flex;flex-wrap:wrap;align-items:baseline;gap:10px;padding:16px 18px 0}
.tool__name{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-weight:700;color:var(--brand);font-size:1rem}
.tool__desc{margin:0;padding:8px 18px 16px;color:var(--text-soft);font-size:.95rem;line-height:1.7;max-width:76ch}
.tool__args{margin:0;padding:14px 18px 16px;list-style:none;display:grid;gap:12px;border-top:1px solid var(--border);background:var(--surface-soft)}
.tool__arg{display:flex;flex-wrap:wrap;gap:8px;align-items:baseline;font-size:.9rem;line-height:1.65}
.tool__argname{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-weight:600;color:var(--text)}
.tool__type{color:var(--muted);font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:.82rem}
.tool__req{font-size:.7rem;text-transform:uppercase;letter-spacing:.06em;font-weight:700;color:var(--warn)}
.tool__argdesc{color:var(--text-soft);flex:1 1 240px;min-width:0}

/* ---- recipes ---- */
.recipes{display:grid;gap:20px;margin-top:24px}
.recipe{border:1px solid var(--border);border-radius:var(--radius);background:var(--surface);box-shadow:var(--shadow);overflow:hidden}
.recipe__head{padding:clamp(18px,3vw,22px) clamp(18px,3vw,24px) 0}
.recipe__title{margin:0 0 10px;font-size:1.18rem;letter-spacing:-.01em}
.recipe__summary{margin:0;color:var(--text-soft);font-size:.97rem;line-height:1.75;max-width:68ch}
.recipe__meta{display:flex;flex-wrap:wrap;gap:8px;padding:16px clamp(18px,3vw,24px) 20px}
.recipe__tool{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:.78rem;line-height:1.5;padding:4px 10px;border-radius:999px;background:var(--surface-soft);border:1px solid var(--border-strong);color:var(--text-soft)}
.recipe__tool--when{font-family:inherit;background:var(--brand-soft);border-color:var(--brand-soft);color:var(--brand-strong);font-weight:600}
.recipe__prompt{padding:0 clamp(18px,3vw,24px) clamp(18px,3vw,22px)}
.recipe__prompt .snippet{margin:0}
.recipe__prompt pre{border:1px solid var(--border)}
.recipe__caveat{margin:0;padding:14px clamp(18px,3vw,24px) 16px;border-top:1px solid var(--border);background:var(--surface-soft);font-size:.9rem;color:var(--text-soft);line-height:1.7}
.recipe__caveat strong{color:var(--warn)}
.nav{display:flex;flex-wrap:wrap;gap:4px;border-bottom:1px solid var(--border);margin-bottom:24px}
.nav a{position:relative;display:inline-flex;align-items:center;gap:7px;padding:10px 14px;text-decoration:none;color:var(--muted);font-weight:600;font-size:.94rem;border-radius:var(--radius-sm) var(--radius-sm) 0 0;border-bottom:2px solid transparent;margin-bottom:-1px}
.nav a:hover{color:var(--text);background:var(--surface-soft)}
.nav a[aria-current=page]{color:var(--brand);border-bottom-color:var(--brand)}
/* The tab bar carries a marker only when something is wrong. A green dot that
   is always green is read as decoration within a day, and then the one day it
   turns amber nobody notices. */
.nav__alert{display:inline-flex;align-items:center;justify-content:center;width:17px;height:17px;flex:none;border-radius:50%;background:var(--danger-bg);color:var(--danger);border:1px solid var(--danger-border);font-size:.68rem;font-weight:800;line-height:1}
.nav__alert--warn{background:var(--warn-bg);color:var(--warn);border-color:var(--warn)}
.nav__spacer{flex:1}

/* ---- cards ---- */
.card{background:var(--surface);border:1px solid var(--border);border-radius:var(--radius);box-shadow:var(--shadow);margin:0 0 18px;overflow:hidden}
.card__head{display:flex;flex-wrap:wrap;align-items:center;justify-content:space-between;gap:10px 16px;padding:16px clamp(16px,3vw,22px);border-bottom:1px solid var(--border);background:var(--surface-soft)}
.card__body{padding:clamp(16px,3vw,22px)}
.card__body>:first-child{margin-top:0}
.card__body>:last-child{margin-bottom:0}
.card--accent .card__head{background:var(--brand-soft);border-bottom-color:var(--border-strong)}
.card--accent .card__head h2{color:var(--brand)}
.lead{color:var(--text-soft);margin-top:0;font-size:1.02rem;line-height:1.72;max-width:72ch}

/* ---- steps ---- */
.step{padding:4px 0}
.step+.step{border-top:1px solid var(--border)}
.step__summary{display:flex;align-items:center;gap:12px;padding:14px 0;cursor:pointer;list-style:none}
.step__summary::-webkit-details-marker{display:none}
.step__summary:hover .step__title{color:var(--brand)}
.step[data-locked] .step__summary{cursor:default;opacity:.6}
.step__n{width:30px;height:30px;flex:none;border-radius:50%;background:var(--brand);color:var(--brand-ink);display:grid;place-items:center;font-weight:700;font-size:.9rem}
.step--done .step__n{background:var(--ok);font-size:1rem}
.step[data-locked] .step__n{background:var(--off-bg);color:var(--off)}
.step__title{font-weight:700}
.step--done .step__title{color:var(--muted);font-weight:600}
.step__state{font-size:.72rem;font-weight:700;text-transform:uppercase;letter-spacing:.06em;color:var(--ok)}
.step__state--waiting{color:var(--warn);display:inline-flex;align-items:center;gap:6px}
.step__state--waiting::before{content:"";width:7px;height:7px;border-radius:50%;background:currentColor;animation:pulse 1.6s ease-in-out infinite}
@keyframes pulse{0%,100%{opacity:1}50%{opacity:.25}}
@media (prefers-reduced-motion:reduce){.step__state--waiting::before{animation:none}}
.step__body{padding:0 0 18px 42px}
.step__body>:first-child{margin-top:0}
@media (max-width:520px){.step__body{padding-left:0}}

/* ---- pills, badges ---- */
.pill{display:inline-flex;align-items:center;gap:6px;font-size:.8rem;font-weight:600;padding:4px 10px;border-radius:999px;white-space:nowrap;border:1px solid transparent}
.pill::before{content:"";width:7px;height:7px;border-radius:50%;background:currentColor;flex:none}
.pill--ok{background:var(--ok-bg);color:var(--ok);border-color:var(--ok-border)}
.pill--warn{background:var(--warn-bg);color:var(--warn)}
.pill--off{background:var(--off-bg);color:var(--off)}
.pill--plain::before{display:none}
.pill--accent{background:var(--accent-soft);color:var(--accent)}

/* ---- buttons ---- */
.btn{display:inline-flex;align-items:center;justify-content:center;gap:8px;font:inherit;font-size:.94rem;font-weight:600;text-decoration:none;padding:10px 16px;border:1px solid transparent;border-radius:var(--radius-sm);cursor:pointer;background:var(--brand);color:var(--brand-ink);white-space:nowrap}
.btn:hover{background:var(--brand-strong)}
.btn--block{width:100%}
.btn--ghost{background:var(--surface);color:var(--brand-strong);border-color:var(--border-strong)}
.btn--ghost:hover{background:var(--surface-soft)}
.btn--quiet{background:transparent;color:var(--muted);border-color:transparent;padding:8px 10px;font-weight:500}
.btn--quiet:hover{background:var(--surface-soft);color:var(--text)}
.btn--danger{background:transparent;color:var(--danger);border-color:var(--danger-border)}
.btn--danger:hover{background:var(--danger-bg)}
.btn--small{padding:6px 11px;font-size:.86rem}
:where(a,button,input,summary):focus-visible{outline:3px solid var(--ring);outline-offset:2px}
.actions{display:flex;flex-wrap:wrap;gap:10px;align-items:center}
.actions--end{justify-content:flex-end}
.btn[disabled]{cursor:progress;opacity:.72}
.btn[disabled]:hover{background:var(--brand)}
.btn[aria-disabled=true]{pointer-events:none;opacity:.45}
.spinner{width:15px;height:15px;flex:none;border:2px solid currentColor;border-right-color:transparent;border-radius:50%;animation:spin .7s linear infinite}
@keyframes spin{to{transform:rotate(360deg)}}
@media (prefers-reduced-motion:reduce){.spinner{animation-duration:2.4s}}
.busy{display:flex;align-items:center;gap:10px;margin:14px 0 0;color:var(--muted);font-size:.88rem;line-height:1.5}
.busy[hidden]{display:none}

/* ---- numbered how-to list ---- */
.guide{list-style:none;counter-reset:guide;margin:0;padding:0;display:grid;gap:9px}
.guide li{position:relative;counter-increment:guide;padding-left:32px;line-height:1.5}
.guide li::before{content:counter(guide);position:absolute;left:0;top:1px;width:22px;height:22px;border-radius:50%;background:var(--brand-soft);color:var(--brand);font-size:.76rem;font-weight:700;display:inline-flex;align-items:center;justify-content:center}

/* ---- forms ---- */
/* A field is the label sitting above one control. The gap that separates one
   field from the next belongs to whatever follows the control, not to the
   label alone — with only a label margin, the next label lands flush against
   the input above it. */
.field{display:block;margin:0 0 7px}
.field__label{display:block;font-weight:600;font-size:.9rem}
.field__hint{display:block;font-weight:400;color:var(--muted);font-size:.84rem;margin-top:3px}
input[type=text],input[type=email],input[type=password]{width:100%;font:inherit;padding:10px 12px;color:var(--text);background:var(--surface-soft);border:1px solid var(--border-strong);border-radius:var(--radius-sm)}
input[type=text]:hover,input[type=email]:hover,input[type=password]:hover{border-color:var(--brand-strong)}
input[type=radio]{accent-color:var(--brand-strong);width:18px;height:18px;flex:none;margin:0}
/* Anything after a control opens a new block. .reveal is that same control
   once password.js has wrapped it with the show/hide button, so both spellings
   carry the rule and the spacing does not depend on the script having run. */
input+.field,.reveal+.field{margin-top:20px}
input+.actions,.reveal+.actions,input+.muted,.reveal+.muted{margin-top:16px}

/* ---- alerts ---- */
.alert{display:block;margin:0 0 18px;padding:12px 14px;border-radius:var(--radius-sm);border:1px solid var(--danger-border);background:var(--danger-bg);color:var(--danger);font-size:.93rem}
.alert--ok{border-color:var(--ok-border);background:var(--ok-bg);color:var(--ok)}
.problems{list-style:none;margin:0 0 16px;padding:0;display:grid;gap:8px}
.problems li{padding:11px 13px;border-radius:var(--radius-sm);border:1px solid var(--danger-border);background:var(--danger-bg);color:var(--danger);font-size:.93rem}

/* ---- lists ---- */
.rows{list-style:none;margin:0;padding:0;border:1px solid var(--border);border-radius:var(--radius-sm);overflow:hidden}
.row{display:flex;flex-wrap:wrap;align-items:center;gap:10px 14px;padding:13px 15px;background:var(--surface)}
.row+.row{border-top:1px solid var(--border)}
.row--on{background:var(--brand-soft)}
.row__main{flex:1 1 220px;min-width:0}
.row__title{font-weight:600;overflow-wrap:anywhere}
.row__meta{display:block;font-weight:400;font-size:.84rem;color:var(--muted)}
.row__label{display:flex;align-items:center;gap:11px;flex:1 1 220px;min-width:0;cursor:pointer}
.row__remove{margin-left:auto;flex:none;display:inline-flex;align-items:center;justify-content:center;width:30px;height:30px;border-radius:8px;text-decoration:none;color:var(--muted);font-size:.95rem;line-height:1;border:1px solid transparent}
.row__remove:hover{background:var(--danger-bg);border-color:var(--danger-border);color:var(--danger)}
.target{display:flex;flex-wrap:wrap;align-items:center;gap:8px 10px;padding:12px 14px;margin-bottom:14px;border:1px solid var(--border);border-radius:var(--radius-sm);background:var(--surface-soft)}
.target__name{font-weight:700;overflow-wrap:anywhere}
.target__meta{color:var(--muted);font-size:.88rem;overflow-wrap:anywhere}
.mono{font-family:ui-monospace,SFMono-Regular,Menlo,monospace}

/* ---- facts ---- */
.facts{display:grid;grid-template-columns:repeat(auto-fit,minmax(160px,1fr));gap:14px;margin:0}
.fact{padding:12px 14px;border:1px solid var(--border);border-radius:var(--radius-sm);background:var(--surface-soft)}
.fact dt{font-size:.78rem;color:var(--muted);font-weight:600;text-transform:uppercase;letter-spacing:.04em}
.fact dd{margin:3px 0 0;font-weight:600;font-size:1.02rem}
.fact__detail{display:block;font-weight:400;font-size:.82rem;color:var(--muted)}

/* ---- overview ---- */
/* The two sentences the landing page exists to say: this phone line is up, and
   these AI tools are plugged into it. Tone lives on the item, so the badge
   picks it up through currentColor and the prose stays readable. */
.overview{display:grid;grid-template-columns:repeat(auto-fit,minmax(250px,1fr));gap:14px;margin:0 0 20px}
.overview__item{display:flex;align-items:flex-start;gap:13px;padding:16px 18px;border:1px solid var(--border);border-radius:var(--radius);background:var(--surface);box-shadow:var(--shadow);color:var(--muted)}
.overview__item--ok{color:var(--ok);border-color:var(--ok-border);background:var(--ok-bg)}
.overview__item--wait{color:var(--warn);border-color:var(--warn);background:var(--warn-bg)}
.overview__icon{width:32px;height:32px;flex:none;border-radius:50%;display:grid;place-items:center;font-size:1rem;font-weight:800;line-height:1;background:var(--surface);border:1px solid currentColor}
.overview__body{min-width:0}
.overview__title{margin:0;font-weight:700;color:var(--text);font-size:1rem;line-height:1.35}
.overview__detail{margin:4px 0 0;color:var(--text-soft);font-size:.9rem;line-height:1.5;overflow-wrap:anywhere}
.overview__phone{display:block;font-size:1.08rem;font-weight:700;color:var(--text);font-variant-numeric:tabular-nums}

/* ---- connections ---- */
.tool-mark{width:36px;height:36px;flex:none;border-radius:10px;display:grid;place-items:center;background:var(--brand-soft);color:var(--brand);border:1px solid var(--border);font-weight:800;font-size:1rem;text-transform:uppercase}
.row--waiting .tool-mark{background:var(--warn-bg);color:var(--warn)}
/* The primary action is a button, not a banner: stretched across the column
   it read as a section header. It stays the width of its own label. */
.hero{display:flex;justify-content:center;margin:0 0 22px}
.btn--big{padding:11px 18px;font-size:.97rem}

/* ---- picks (one question, whole-row targets) ---- */
.disclose{margin:0}
.disclose summary{cursor:pointer;font-weight:600;font-size:.94rem;color:var(--brand-strong);list-style:none}
.disclose summary::-webkit-details-marker{display:none}
.disclose[open] summary{margin-bottom:16px}
.picks{list-style:none;margin:0 0 18px;padding:0;display:grid;gap:9px}
.pick{display:flex;align-items:flex-start;gap:11px;padding:12px 13px;border:1px solid var(--border-strong);border-radius:var(--radius-sm);background:var(--surface);cursor:pointer}
.pick:hover{background:var(--surface-soft);border-color:var(--brand-strong)}
.pick input{margin-top:3px}
.pick:has(input:checked){border-color:var(--brand);background:var(--brand-soft)}
.pick__text{min-width:0}
.pick__title{display:block;font-weight:700;font-size:.95rem}
.pick__hint{display:block;color:var(--muted);font-size:.85rem;line-height:1.5;margin-top:2px}

/* ---- empty ---- */
.empty{padding:28px 20px;text-align:center;border:1px dashed var(--border-strong);border-radius:var(--radius-sm);background:var(--surface-soft)}
.empty__title{font-weight:600;color:var(--text);margin:0}

/* ---- snippets ---- */
.snippet{margin:0 0 16px}
.snippet__head{display:flex;flex-wrap:wrap;align-items:baseline;justify-content:space-between;gap:8px;margin-bottom:7px}
.snippet__title{font-weight:600;font-size:.92rem}
.snippet__note{font-size:.84rem;color:var(--muted)}
.copy{font:inherit;font-size:.78rem;font-weight:600;padding:4px 10px;border-radius:7px;border:1px solid var(--border-strong);background:var(--surface);color:var(--brand-strong);cursor:pointer;flex:none}
.snippet--loose{position:relative}
.snippet--loose .copy{position:absolute;top:8px;right:8px;z-index:1}
.copy:hover{background:var(--surface-soft)}
.copy--done{color:var(--ok);border-color:var(--ok-border)}
.copy--failed{color:var(--danger);border-color:var(--danger-border)}

/* ---- secret ---- */
.secret{border:2px solid var(--ok);border-radius:var(--radius);background:var(--ok-bg);padding:18px;margin:0 0 20px}
.secret__title{margin:0 0 4px;font-weight:700;color:var(--ok)}
.secret__value{display:block;margin:12px 0 8px;padding:14px;border-radius:var(--radius-sm);background:var(--surface);border:1px solid var(--ok-border);font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:1.05rem;font-weight:600;overflow-wrap:anywhere;user-select:all}

/* ---- dialogs (CSS only) ---- */
.overlay{position:fixed;inset:0;background:rgba(6,20,16,.55);display:none;place-items:center;padding:20px;z-index:50}
.overlay:target{display:grid}
.dialog{background:var(--surface);border:1px solid var(--border-strong);border-radius:var(--radius);box-shadow:var(--shadow-lift);width:min(460px,100%);max-height:90vh;overflow:auto}
.dialog__head{display:flex;align-items:center;justify-content:space-between;gap:12px;padding:16px 20px;border-bottom:1px solid var(--border)}
.dialog__head h2{font-size:1.05rem}
.dialog__body{padding:20px}
.dialog__close{text-decoration:none;color:var(--muted);font-size:1.5rem;line-height:1;padding:0 4px}
.dialog__close:hover{color:var(--text)}

.tabs__radio{position:absolute;width:1px;height:1px;opacity:0;pointer-events:none}
.tabs__bar{display:flex;gap:4px;border-bottom:1px solid var(--border);margin-bottom:16px}
.tabs__tab{padding:8px 14px;font-weight:600;font-size:.92rem;color:var(--muted);cursor:pointer;border-bottom:2px solid transparent;margin-bottom:-1px;border-radius:var(--radius-sm) var(--radius-sm) 0 0}
.tabs__tab:hover{color:var(--text);background:var(--surface-soft)}
.tabs__panel{display:none}
#tab-code:checked~.tabs__panel--code,#tab-desktop:checked~.tabs__panel--desktop,#tab-outros:checked~.tabs__panel--outros{display:block}
#tab-code:checked~.tabs__bar label[for=tab-code],#tab-desktop:checked~.tabs__bar label[for=tab-desktop],#tab-outros:checked~.tabs__bar label[for=tab-outros]{color:var(--brand);border-bottom-color:var(--brand)}
.tabs__radio:focus-visible~.tabs__bar label{outline:3px solid var(--ring);outline-offset:2px}
.prompts{list-style:none;margin:12px 0 0;padding:0;display:grid;gap:8px}
.prompt .snippet{margin:0}
.prompt pre{background:var(--surface-sunken);color:var(--text);border:1px solid var(--border);padding:10px 12px;font-size:.92rem;white-space:pre-wrap}
.prompt pre,.prompt pre code{font-family:inherit}
/* The copy button floats over a headerless snippet, so the text has to keep
   out from under it — otherwise a wrapped line runs beneath the button. */
.prompt pre{padding-right:82px}
.guide+.snippet{margin-top:16px}
/* A block of plain Portuguese is not code: it gets the page's own colours and
   wraps, instead of a terminal's palette and a sideways scrollbar. */
pre.plain{background:var(--surface-sunken);color:var(--text);border:1px solid var(--border);white-space:pre-wrap;font-size:.92rem}
pre.plain,pre.plain code{font-family:inherit}
.qrcode{display:block;margin:0 auto;width:250px;height:250px;max-width:100%;background:#fff;padding:12px;border-radius:var(--radius-sm);border:1px solid var(--border)}
.sr-only{position:absolute;width:1px;height:1px;margin:-1px;padding:0;overflow:hidden;clip:rect(0 0 0 0);white-space:nowrap;border:0}
.muted{color:var(--text-soft);font-size:.9rem;line-height:1.65}
.stack>*+*{margin-top:16px}
@media (max-width:520px){.row{align-items:flex-start}.row form,.row .btn{width:100%}}
/* ---- installation wizard ---- */
/* The first run is one column, one card and one question at a time: the panel
   proper is what comes after it. */
.wizard-shell{max-width:520px;margin:0 auto}
.wizard{list-style:none;display:flex;align-items:center;gap:10px;margin:8px 0 22px;padding:0}
.wizard__step{display:flex;align-items:center;gap:9px;color:var(--muted);font-size:.85rem;font-weight:600;white-space:nowrap}
.wizard__step+.wizard__step{flex:1;min-width:0}
.wizard__step+.wizard__step::before{content:"";flex:1;min-width:14px;height:1px;background:var(--border-strong)}
.wizard__n{width:26px;height:26px;flex:none;border-radius:50%;display:grid;place-items:center;font-size:.8rem;font-weight:700;background:var(--surface-sunken);color:var(--muted);border:1px solid var(--border-strong)}
.wizard__step--now{color:var(--brand)}
.wizard__step--now .wizard__n{background:var(--brand);color:var(--brand-ink);border-color:var(--brand)}
.wizard__step--done{color:var(--ok)}
.wizard__step--done .wizard__n{background:var(--ok-bg);color:var(--ok);border-color:var(--ok-border)}
.wizard__escape{display:flex;justify-content:center;margin-top:10px}
.resend{margin-top:20px;border-top:1px solid var(--border);padding-top:14px}
.resend summary{cursor:pointer;font-size:.9rem;font-weight:600;color:var(--brand-strong);list-style:none}
.resend summary::-webkit-details-marker{display:none}
.resend[open] summary{margin-bottom:12px}
@media (max-width:460px){.wizard__label{display:none}}
/* ---- colophon ---- */
.colophon{margin-top:40px;padding:18px 0 4px;border-top:1px solid var(--border)}
.colophon__line{display:flex;flex-wrap:wrap;align-items:center;justify-content:center;gap:8px;margin:0;font-size:.85rem;color:var(--muted)}
.colophon__link{display:inline-flex;align-items:center;gap:5px;color:var(--muted);text-decoration:none;font-weight:600}
.colophon__link:hover{color:var(--brand-strong);text-decoration:underline}
.colophon__icon{flex:none;display:block}
.colophon__sep{color:var(--border-strong)}
.colophon__support{display:inline-flex;align-items:center;gap:6px;padding:4px 12px;border-radius:999px;background:var(--brand-soft);border:1px solid var(--border);color:var(--brand);font-weight:700;text-decoration:none}
.colophon__support:hover{background:var(--brand);color:var(--brand-ink);border-color:var(--brand)}
/* ---- update banner ---- */
.update{margin:0 0 18px;padding:12px 14px;border-radius:var(--radius-sm);border:1px solid var(--warn);background:var(--warn-bg);color:var(--text)}
.update__line{display:flex;flex-wrap:wrap;align-items:baseline;gap:6px;margin:0 0 8px;font-size:.93rem}
.update__version{font-weight:700;color:var(--warn)}
.update__notes{color:var(--muted);font-weight:600;text-decoration:none}
.update__notes:hover{color:var(--brand-strong);text-decoration:underline}
.update .snippet{margin:0}
.update__note{margin:8px 0 0;font-size:.84rem;color:var(--muted)}
.update--rollback{border-color:var(--danger-border);background:var(--danger-bg)}
.update--rollback .update__version{color:var(--danger)}
.update form{margin:0}
.progress{list-style:none;margin:0;padding:0;display:grid;gap:6px;font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:.82rem;color:var(--text-soft);max-height:260px;overflow:auto;background:var(--surface-soft);border:1px solid var(--border);border-radius:var(--radius-sm);padding:10px 12px}
.colophon__version{font-variant-numeric:tabular-nums}
/* ---- password reveal ---- */
.reveal{position:relative;display:block}
.reveal input{width:100%;padding-right:44px}
.reveal__toggle{position:absolute;top:50%;right:6px;transform:translateY(-50%);display:inline-flex;align-items:center;justify-content:center;width:32px;height:32px;padding:0;border:0;border-radius:8px;background:none;color:var(--muted);cursor:pointer}
.reveal__toggle:hover{color:var(--text);background:var(--surface-sunken)}
.reveal__toggle:focus-visible{outline:3px solid var(--ring);outline-offset:1px}
`

const extraCSS = `
[hidden]{display:none!important}
/* ---- WhatsApp-style previews ---- */
/* Drawn to resemble the phone's own chat list, so checking "is this synced?"
   is a glance from one screen to the other, not a reading exercise. */
.chats{list-style:none;margin:0;padding:0;border:1px solid var(--border);border-radius:var(--radius-sm);overflow:hidden;background:var(--surface)}
.chat{display:flex;align-items:center;gap:12px;padding:10px 14px;cursor:pointer;background:var(--surface);border:0;width:100%;font:inherit;color:inherit;text-align:left}
.chat+.chat,.chats li+li .chat{border-top:1px solid var(--border)}
.chat:hover{background:var(--surface-soft)}
.avatar{width:42px;height:42px;flex:none;border-radius:50%;display:grid;place-items:center;font-weight:700;font-size:1rem;color:#fff;text-transform:uppercase}
.avatar--group::after{content:"";}
.chat__main{flex:1;min-width:0}
.chat__top{display:flex;align-items:baseline;justify-content:space-between;gap:10px}
.chat__name{font-weight:600;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.chat__time{flex:none;font-size:.76rem;color:var(--muted);font-variant-numeric:tabular-nums}
.chat__time--unread{color:var(--brand-strong);font-weight:700}
.chat__bottom{display:flex;align-items:center;justify-content:space-between;gap:10px;margin-top:1px}
.chat__last{font-size:.88rem;color:var(--muted);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.chat__badge{flex:none;min-width:20px;height:20px;padding:0 6px;border-radius:999px;background:var(--brand-strong);color:var(--brand-ink);font-size:.72rem;font-weight:700;display:grid;place-items:center}
.chat__pin{flex:none;color:var(--muted);font-size:.8rem}
.thread{display:flex;flex-direction:column;gap:6px;padding:14px;border-radius:var(--radius-sm);border:1px solid var(--border);background:var(--surface-sunken);max-height:460px;overflow:auto}
.bubble{max-width:82%;padding:6px 10px 5px;border-radius:10px;background:var(--surface);border:1px solid var(--border);box-shadow:0 1px 1px rgba(0,0,0,.04);align-self:flex-start;overflow-wrap:anywhere}
.bubble--me{align-self:flex-end;background:var(--ok-bg);border-color:var(--ok-border)}
.bubble__who{display:block;font-size:.78rem;font-weight:700;color:var(--brand-strong)}
.bubble__text{font-size:.92rem;line-height:1.45;white-space:pre-wrap}
.bubble__media{font-size:.86rem;color:var(--muted);font-style:italic}
.bubble__time{display:block;text-align:right;font-size:.7rem;color:var(--muted);margin-top:1px;font-variant-numeric:tabular-nums}
.thread__day{align-self:center;font-size:.72rem;font-weight:600;color:var(--muted);background:var(--surface);border:1px solid var(--border);border-radius:999px;padding:2px 10px;margin:4px 0}
.inbox{list-style:none;margin:0;padding:0;display:grid;gap:8px}
.inbox .bubble{max-width:100%;align-self:stretch}
.inbox__where{display:flex;justify-content:space-between;gap:10px;font-size:.78rem;color:var(--muted);margin-bottom:2px}
.inbox__chat{font-weight:700;color:var(--text);white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.preview-grid{display:grid;grid-template-columns:minmax(0,1.1fr) minmax(0,1fr);gap:18px}
@media (max-width:760px){.preview-grid{grid-template-columns:1fr}}
.dialog--wide{width:min(620px,100%)}
.skeleton{height:62px;border-radius:var(--radius-sm);background:linear-gradient(90deg,var(--surface-soft),var(--surface-sunken),var(--surface-soft));background-size:200% 100%;animation:shimmer 1.4s linear infinite}
@keyframes shimmer{to{background-position:-200% 0}}
@media (prefers-reduced-motion:reduce){.skeleton{animation:none}}
.problems .problems__warn{border-color:var(--warn);background:var(--warn-bg);color:var(--text)}
.problems__warn strong{color:var(--warn)}
.note{margin:0 0 18px;padding:11px 13px;border-radius:var(--radius-sm);background:var(--brand-soft);border:1px solid var(--border);font-size:.9rem;color:var(--text-soft);line-height:1.6}
/* ---- pairing ---- */
.qrframe{position:relative;width:260px;max-width:100%;margin:6px auto 0}
.qrframe .qrcode{width:260px;height:260px}
.qrframe__wait{width:260px;height:260px;max-width:100%;display:grid;place-items:center;border:1px dashed var(--border-strong);border-radius:var(--radius-sm);background:var(--surface-soft);color:var(--muted);font-size:.9rem}
.paircode{font:700 1.8rem/1.2 ui-monospace,SFMono-Regular,Menlo,monospace;letter-spacing:.2em;text-align:center;padding:18px;border-radius:var(--radius-sm);border:1px solid var(--border-strong);background:var(--surface-soft);margin:6px 0 0}
.progressline{display:flex;align-items:center;gap:10px;color:var(--text-soft);font-size:.94rem}
.hello{display:flex;align-items:center;gap:14px;margin:0 0 16px}
.hello__name{margin:0;font-weight:700;font-size:1.1rem}
.hello__phone{margin:0;color:var(--muted);font-variant-numeric:tabular-nums}
.linkbtn{background:none;border:0;padding:0;font:inherit;font-size:.9rem;font-weight:600;color:var(--brand-strong);cursor:pointer;text-decoration:underline}
.pick__action{margin-top:10px}
`
