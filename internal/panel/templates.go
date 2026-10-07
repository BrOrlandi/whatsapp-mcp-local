package panel

// pageSource holds every template the panel renders. The shell, the masthead,
// the tabs, the cards, the dialogs and the colophon are v1's markup; what
// differs is what a local daemon has instead of a server: one WhatsApp account
// rather than instances, no login, and a chat preview to confirm the sync.
const pageSource = `
{{define "head"}}<!doctype html>
<html lang="pt-BR"{{if .App}} data-app{{end}}><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="icon" href="/favicon.svg" type="image/svg+xml"><link rel="alternate icon" href="/favicon.ico" sizes="16x16 32x32 48x48"><link rel="apple-touch-icon" href="/apple-touch-icon.png"><script src="/assets/theme.js"></script><title>{{.Title}} · WhatsApp MCP</title><style>{{css}}</style></head><body><div class="shell">{{end}}

{{define "foot"}}
<footer class="colophon">
<p class="colophon__line">
<a class="colophon__link" href="{{repositoryURL}}" rel="noopener noreferrer" target="_blank">{{template "githubmark"}}<span>C&oacute;digo-fonte</span></a>
<span class="colophon__sep">&middot;</span>
<span class="colophon__version" title="Vers&atilde;o em execu&ccedil;&atilde;o">v{{version}}</span>
{{with supportURL}}<span class="colophon__sep">&middot;</span>
<a class="colophon__support" href="{{.}}" rel="noopener noreferrer" target="_blank">Apoie o projeto</a>{{end}}
</p>
</footer>
{{if .App}}{{template "updatebanner" .}}{{end}}
<script src="/assets/app.js" defer></script></div></body></html>{{end}}

{{/* A new version, on every page of the app, in the bottom left corner. */}}
{{define "updatebanner"}}<aside class="update-banner" data-update-banner aria-live="polite"{{if not (eq .Update.State "available" "downloading" "ready" "manual")}} hidden{{end}}>
<button class="update-banner__close" type="button" aria-label="Agora não" data-update-banner-close>&times;</button>
<p class="update-banner__title">Nova versão disponível</p>
<p class="update-banner__text" data-update-banner-text>O WhatsApp MCP {{.Update.Latest}} está disponível. O aplicativo será reiniciado após a instalação.</p>
<div class="actions"><button class="btn btn--small" type="button" data-update-banner-install>Atualizar agora</button><a class="btn btn--ghost btn--small" data-update-banner-page href="{{.Update.Page}}" target="_blank" rel="noopener" hidden>Baixar a versão nova</a></div>
</aside>{{end}}

{{define "githubmark"}}<svg class="colophon__icon" viewBox="0 0 16 16" width="15" height="15" aria-hidden="true" focusable="false"><path fill="currentColor" d="M8 0C3.58 0 0 3.58 0 8a8 8 0 0 0 5.47 7.59c.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82a7.4 7.4 0 0 1 2-.27c.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8Z"/></svg>{{end}}

{{/* The way into Configurações, in the top right corner of every page. */}}
{{define "settingslink"}}<a class="gear" href="/configuracoes" aria-label="Configurações" title="Configurações"{{if eq .Active "configuracoes"}} aria-current="page"{{end}}>{{icon "gear"}}</a>{{end}}

{{define "brandmark"}}<span class="brand__mark">{{logo}}</span><span class="brand__name">WhatsApp MCP</span>{{end}}

{{define "nav"}}
<header class="masthead"><a class="brand" href="/">{{template "brandmark"}}</a>
<div class="masthead__tools">{{template "settingslink" .}}</div></header>
<nav class="nav" aria-label="Seções"{{if .LiveKey}} data-live="{{.LiveKey}}" data-live-tone="{{.HealthTone}}"{{if .LiveBusy}} data-live-busy{{end}}{{end}}>
<a href="/"{{if eq .Active "conectar"}} aria-current="page"{{end}}>{{icon "plug"}}<span>Conectar MCP</span></a>
<a href="/whatsapp"{{if eq .Active "whatsapp"}} aria-current="page"{{end}}>{{icon "chat"}}<span>WhatsApp</span></a>
<a href="/status" data-nav-status{{if eq .Active "status"}} aria-current="page"{{end}}>{{icon "activity"}}{{if and (ne .HealthTone "ok") (ne .HealthTone "")}}<span class="sr-only">Atenção: </span>{{end}}<span class="nav__label">Status</span>{{if and (ne .HealthTone "ok") (ne .HealthTone "")}}<span class="nav__alert{{if eq .HealthTone "warn"}} nav__alert--warn{{end}}" aria-hidden="true">!</span>{{end}}</a>
<a href="/funcoes"{{if eq .Active "funcoes"}} aria-current="page"{{end}}>{{icon "list"}}<span>Funções</span></a>
<a href="/receitas"{{if eq .Active "receitas"}} aria-current="page"{{end}}>{{icon "book"}}<span>Receitas</span></a>
<a href="/ajuda"{{if eq .Active "ajuda"}} aria-current="page"{{end}}>{{icon "help"}}<span>Ajuda</span></a>
</nav>
{{template "portbanner" .}}
{{with .Error}}<p class="alert" role="alert">{{.}}</p>{{end}}
{{end}}

{{/* The MCP's port taken by another program: WhatsApp keeps running, and the
way out is one click away. */}}
{{define "portbanner"}}{{with .MCPProblem}}<div class="alert" role="alert">
<p style="margin:0"><strong>As ferramentas de IA não alcançam o WhatsApp agora.</strong> {{if .OtherGateway}}Outra cópia do WhatsApp MCP, provavelmente a versão de linha de comando, já usa a porta {{.Port}}.{{else}}A porta {{.Port}} já está em uso por outro programa.{{end}} O WhatsApp continua conectado e recebendo; só o MCP espera uma porta livre.</p>
{{if .Suggest}}<form method="post" action="/configuracoes/porta" class="actions" style="margin-top:10px"><input type="hidden" name="port" value="{{.Suggest}}"><button class="btn btn--small" type="submit">Usar a porta {{.Suggest}}</button><a class="btn btn--quiet btn--small" href="/configuracoes#porta">Escolher outra</a></form>{{end}}
</div>{{end}}{{end}}

{{/* The connection's state; one that passes on its own spins, and the page
refreshes when it has passed. */}}
{{define "syncpill"}}<span class="pill pill--{{.SyncTone}}{{if .SyncBusy}} pill--busy{{end}}" data-sync>{{if .SyncBusy}}<span class="spinner" aria-hidden="true"></span>{{end}}{{.SyncLabel}}</span>{{end}}

{{define "autostartcheck"}}{{if .App}}<div class="wizard__escape"><label class="check check--inline"><input type="checkbox" data-setting="autostart"{{if .Settings.Autostart}} checked{{end}}><span>Abrir o WhatsApp MCP quando o computador ligar</span></label></div>
<p class="busy" data-setting-note role="status" hidden></p>{{end}}{{end}}

{{/* Connecting an AI tool is a flow of its own: the tool first, then the
steps for that one, which end when it connects. No tabs on the way. */}}
{{define "flowhead"}}
<header class="masthead"><a class="brand" href="/">{{template "brandmark"}}</a>
<div class="masthead__tools">{{template "settingslink" .}}</div></header>
<div class="flow">
{{template "portbanner" .}}
{{with .Error}}<p class="alert" role="alert">{{.}}</p>{{end}}
{{end}}

{{define "toolpicks"}}<ul class="toolpicks">
{{range .}}<li><a class="toolpick" href="/conectar/{{.Key}}"><span class="toolpick__icon">{{toolmark .Mark}}</span>
<span class="toolpick__text"><span class="toolpick__name">{{.Name}}</span><span class="toolpick__hint">{{.Hint}}</span></span>
{{if eq .State "live"}}<span class="pill pill--ok">Conectado</span>{{else if eq .State "configured"}}<span class="pill pill--warn">Aguardando</span>{{end}}
<span class="toolpick__go" aria-hidden="true">{{icon "chevron"}}</span></a></li>
{{end}}</ul>{{end}}

{{define "conectarescolha"}}{{template "head" .}}{{template "flowhead" .}}
<a class="flow__back" href="{{if .InSetup}}/instalacao{{else}}/{{end}}">{{icon "back"}}Voltar</a>
<h1>Conectar ferramenta de IA ao MCP</h1>
<p class="lead">Onde você vai usar o seu WhatsApp?</p>
{{template "toolpicks" .Tools}}
<p class="muted flow__note">Não funciona no Claude pelo navegador ou pelo celular, nem no ChatGPT na web: só em programas instalados neste computador.</p>
</div>
{{template "foot" .}}{{end}}

{{define "conectarferramenta"}}{{template "head" .}}{{template "flowhead" .}}
{{if not .Done}}<a class="flow__back" href="/conectar">{{icon "back"}}Escolher outra ferramenta</a>{{end}}
<div class="flow__title"><span class="toolpick__icon">{{toolmark .Tool.Mark}}</span><h1>{{.Tool.Title}}</h1></div>

{{if .Done}}
<section class="card card--accent">
<div class="card__head"><h2>{{.Tool.Connected}}</h2><span class="pill pill--ok">Conectado</span></div>
<div class="card__body stack">
<p class="muted">Já pode pedir coisas do seu WhatsApp para a sua IA. Para testar, mande isto no chat:</p>
<div class="snippet"><pre class="plain" data-copy><code>{{.Verification}}</code></pre></div>
{{if .InSetup}}<form method="post" action="/instalacao/avancar"><input type="hidden" name="to" value="done"><div class="actions"><button class="btn" type="submit">Ir para o painel</button></div></form>
{{else}}<div class="actions"><a class="btn" href="/">Voltar ao painel</a></div>{{end}}
</div></section>
{{else}}
{{if .Live}}<p class="alert alert--ok">{{.Tool.Connected}}. Siga os passos abaixo só se quiser configurar de novo.</p>{{end}}
<section class="card">
<div class="card__body">
<ol class="flowsteps">
{{if eq .Tool.Key "claude-desktop"}}
<li class="flowstep"><h2 class="flowstep__title">Adicione o WhatsApp ao Claude Desktop</h2>
{{if not .Setup.Desktop.Found}}<p class="note">O Claude Desktop não parece estar instalado neste computador. <a href="https://claude.ai/download" target="_blank" rel="noopener">Baixar o Claude Desktop</a></p>{{end}}
<div class="actions"><button class="btn" type="button" data-add-client="claude-desktop">{{if .Setup.Desktop.Configured}}Configurar de novo{{else}}Adicionar ao Claude Desktop{{end}}</button></div>
<p class="busy" data-client-note="claude-desktop" role="status" hidden></p>
<details class="resend"><summary>Prefiro fazer à mão</summary>
<p class="muted">No Claude Desktop, vá em <strong>Configurações → Desenvolvedor → Editar configuração</strong>, cole o texto abaixo e reinicie o app. Se já houver outros servidores no arquivo, acrescente só o trecho <code>"{{serverName}}"</code> dentro de <code>mcpServers</code>, sem apagar o resto.</p>
<div class="snippet"><div class="snippet__head"><span class="snippet__title">{{.Setup.DesktopPath}}</span></div>
<pre data-copy><code>{{.Setup.JSON}}</code></pre></div>
</details></li>
<li class="flowstep"><h2 class="flowstep__title">Feche o Claude Desktop e abra de novo</h2>
<p class="muted">Só assim ele carrega o WhatsApp.</p></li>
{{else if eq .Tool.Key "claude-code"}}
<li class="flowstep"><h2 class="flowstep__title">Adicione o WhatsApp ao Claude Code</h2>
<div class="actions"><button class="btn" type="button" data-add-client="claude-code"{{if not .Setup.Code.Found}} disabled title="O comando claude não foi encontrado neste computador"{{end}}>{{if .Setup.Code.Configured}}Configurar de novo{{else}}Adicionar ao Claude Code{{end}}</button></div>
<p class="busy" data-client-note="claude-code" role="status" hidden></p>
{{if not .Setup.Code.Found}}<p class="muted">O comando <code>claude</code> não foi encontrado. Instale o Claude Code ou rode o comando abaixo onde ele estiver.</p>{{end}}
<details class="resend"{{if not .Setup.Code.Found}} open{{end}}><summary>Prefiro rodar o comando</summary>
<div class="snippet"><div class="snippet__head"><span class="snippet__title">Comando</span></div>
<pre data-copy><code>{{.Setup.Command}}</code></pre></div>
</details></li>
<li class="flowstep"><h2 class="flowstep__title">Abra uma sessão nova do Claude Code</h2>
<p class="muted">Uma sessão que já estava aberta não vê o WhatsApp.</p></li>
{{else if eq .Tool.Key "codex"}}
<li class="flowstep"><h2 class="flowstep__title">Adicione o WhatsApp ao Codex</h2>
<div class="actions"><button class="btn" type="button" data-add-client="codex"{{if not .Setup.Codex.Found}} disabled title="O comando codex não foi encontrado neste computador"{{end}}>{{if .Setup.Codex.Configured}}Configurar de novo{{else}}Adicionar ao Codex{{end}}</button></div>
<p class="busy" data-client-note="codex" role="status" hidden></p>
{{if not .Setup.Codex.Found}}<p class="muted">O comando <code>codex</code> não foi encontrado. Configure à mão, logo abaixo.</p>{{end}}
<details class="resend"{{if not .Setup.Codex.Found}} open{{end}}><summary>Prefiro fazer à mão</summary>
<p class="muted">Acrescente este trecho ao arquivo de configuração do Codex, que vale para o app do ChatGPT, o app do Codex, o terminal e o editor de código.</p>
<div class="snippet"><div class="snippet__head"><span class="snippet__title">{{.Setup.CodexPath}}</span></div>
<pre data-copy><code>{{.Setup.CodexTOML}}</code></pre></div>
<p class="muted">Ou rode este comando:</p>
<div class="snippet"><pre data-copy><code>{{.Setup.CodexCommand}}</code></pre></div>
</details></li>
<li class="flowstep"><h2 class="flowstep__title">Abra uma conversa nova no Codex</h2>
<p class="muted">Pode ser no app do ChatGPT, no app do Codex, no terminal ou no editor. Se o app estiver aberto, feche e abra de novo.</p></li>
{{else if eq .Tool.Key "cursor"}}
<li class="flowstep"><h2 class="flowstep__title">Adicione o WhatsApp ao Cursor</h2>
{{if not .Setup.Cursor.Found}}<p class="note">O Cursor não parece estar instalado neste computador. <a href="https://cursor.com/download" target="_blank" rel="noopener">Baixar o Cursor</a></p>{{end}}
<div class="actions"><button class="btn" type="button" data-add-client="cursor">{{if .Setup.Cursor.Configured}}Configurar de novo{{else}}Adicionar ao Cursor{{end}}</button></div>
<p class="busy" data-client-note="cursor" role="status" hidden></p>
<details class="resend"><summary>Prefiro fazer à mão</summary>
<p class="muted">Abra o arquivo abaixo, que o Cursor também abre pelas configurações de MCP dele, e cole o texto. Se já houver outros servidores no arquivo, acrescente só o trecho <code>"{{serverName}}"</code> dentro de <code>mcpServers</code>, sem apagar o resto.</p>
<div class="snippet"><div class="snippet__head"><span class="snippet__title">{{.Setup.CursorPath}}</span></div>
<pre data-copy><code>{{.Setup.CursorJSON}}</code></pre></div>
</details></li>
<li class="flowstep"><h2 class="flowstep__title">Feche o Cursor e abra de novo</h2>
<p class="muted">Depois, peça ao agente do Cursor o que quiser do seu WhatsApp.</p></li>
{{else}}
<li class="flowstep"><h2 class="flowstep__title">Peça para a sua ferramenta se configurar</h2>
<p class="muted">Serve para o Windsurf e qualquer outra que aceite MCP. Copie o texto abaixo e mande no chat dela.</p>
<div class="snippet"><div class="snippet__head"><span class="snippet__title">Copie e mande para a sua ferramenta</span></div>
<pre class="plain" data-copy><code>{{.Setup.AgentPrompt}}</code></pre></div></li>
<li class="flowstep"><h2 class="flowstep__title">Reinicie a ferramenta</h2>
<p class="muted">Quando ela terminar a configuração, feche e abra de novo, ou comece uma conversa nova.</p></li>
{{end}}
<li class="flowstep"><h2 class="flowstep__title">Espere a conexão</h2><p class="busy" role="status" data-wait-tool="{{.Tool.Client}}" data-wait-known="{{.Known}}" data-wait-since="{{.Since}}"><span class="spinner" aria-hidden="true"></span>Esta tela avisa quando {{.Tool.Who}} se conectar.</p></li>
</ol>
</div></section>
{{end}}
</div>
{{template "foot" .}}{{end}}

{{define "chatpreview"}}
<section class="card" data-chats>
<div class="card__head"><h2>Conversas recentes</h2><span class="pill pill--off pill--plain" data-chats-count>…</span></div>
<div class="card__body">
<p class="muted">As 10 conversas que chegaram por último. Escolha uma para ver as mensagens dela e compare com o seu celular.</p>
<div class="preview-grid">
<ul class="chats" data-chat-list><li><div class="skeleton"></div></li><li><div class="skeleton"></div></li><li><div class="skeleton"></div></li></ul>
<div class="pane">
<div class="pane__head"><span class="avatar" data-thread-avatar></span><div class="pane__who"><strong data-thread-title>…</strong><span class="pane__meta" data-thread-meta></span></div></div>
<div class="thread" data-thread><div class="skeleton"></div></div>
<div class="actions" style="margin-top:12px"><button class="btn btn--ghost btn--small" type="button" data-history-chat>Buscar mensagens mais antigas desta conversa</button></div>
<p class="busy" data-history-note role="status" hidden></p>
</div>
</div>
<p class="note" style="margin:16px 0 0">Se alguma conversa ou mensagem recente estiver faltando, peça o histórico ao celular: pela conversa, no botão acima, ou por todas as recentes, logo abaixo.</p>
</div></section>
{{end}}

{{define "historycard"}}
<section class="card">
<div class="card__head"><h2>Histórico</h2>{{with .History}}{{if .FinishedAt}}<span class="pill pill--ok">Último pedido concluído</span>{{else}}<span class="pill pill--warn">Buscando…</span>{{end}}{{end}}</div>
<div class="card__body stack">
<p class="muted">Cada pedido pode trazer mais histórico do WhatsApp para este computador, e ele fica disponível para a sua ferramenta de IA consultar pelo MCP. O celular precisa estar com internet.</p>
{{with .History}}<p class="muted">{{if .FinishedAt}}Último pedido: {{count .Added}} mensagens novas em {{len .Chats}} conversas.{{else}}Buscando: {{.Done}} de {{len .Chats}} conversas, {{count .Added}} mensagens novas até agora.{{end}}</p>{{end}}
<div class="actions"><button class="btn btn--ghost btn--small" type="button" data-history-all>Buscar mensagens mais antigas das conversas recentes</button></div>
<p class="busy" data-history-all-note role="status" hidden></p>
</div></section>
{{end}}

{{define "conectar"}}{{template "head" .}}{{template "nav" .}}
<h1>Seu WhatsApp nas suas ferramentas de IA</h1>
<p class="lead">Aqui você vê se está tudo funcionando e liga o seu WhatsApp a uma ferramenta de inteligência artificial que aceite MCP, como o Claude, o Codex ou a que você usar.</p>

<section class="overview" data-wait-client="{{if .LiveCount}}false{{else}}true{{end}}">
<div class="overview__item overview__item--{{if eq .SyncTone "ok"}}ok{{else}}wait{{end}}" data-sync>
<span class="overview__icon" aria-hidden="true">{{if .SyncBusy}}<span class="spinner"></span>{{else if eq .SyncTone "ok"}}&#10003;{{else}}!{{end}}</span>
<div class="overview__body">
<p class="overview__title">{{if eq .SyncTone "ok"}}WhatsApp conectado{{else}}{{.SyncLabel}}{{end}}</p>
<p class="overview__detail">{{if .Phone}}<strong class="overview__phone">{{.Phone}}</strong>{{end}}{{.Name}}</p>
</div></div>

{{if .LiveCount}}
<div class="overview__item overview__item--ok">
<span class="overview__icon" aria-hidden="true">&#10003;</span>
<div class="overview__body">
<p class="overview__title">{{plural .LiveCount "ferramenta de IA conectada ao MCP" "ferramentas de IA conectadas ao MCP"}}</p>
<p class="overview__detail">Já pode pedir coisas do seu WhatsApp para a sua IA. Última vez em uso: {{relativeSince .LastUse}}.</p>
</div></div>
{{else if .Connections}}
<div class="overview__item overview__item--wait">
<span class="overview__icon" aria-hidden="true">&hellip;</span>
<div class="overview__body">
<p class="overview__title">Esperando a sua ferramenta de IA</p>
<p class="overview__detail">A configuração já está feita. Falta reiniciar a ferramenta. Esta tela avisa sozinha quando ela aparecer.</p>
</div></div>
{{else}}
<div class="overview__item">
<span class="overview__icon" aria-hidden="true">+</span>
<div class="overview__body">
<p class="overview__title">Nenhuma ferramenta de IA conectada</p>
<p class="overview__detail">Falta um passo: ligar o seu assistente de IA a este WhatsApp.</p>
</div></div>
{{end}}
</section>

<div class="hero"><a class="btn btn--big" href="/conectar">Conectar ferramenta de IA ao MCP</a></div>

<section class="card">
<div class="card__head"><h2>Suas conexões</h2>{{if .Connections}}<a class="btn btn--ghost btn--small" href="/conectar">Nova conexão</a>{{end}}</div>
<div class="card__body">
{{if .Connections}}
<ul class="rows">
{{range $i, $c := .Connections}}<li class="row{{if not .Live}} row--waiting{{end}}">
{{if .Mark}}<span class="tool-mark tool-mark--logo" aria-hidden="true">{{toolmark .Mark}}</span>{{else}}<span class="tool-mark" aria-hidden="true">{{initial .Tool}}</span>{{end}}
<span class="row__main"><span class="row__title">{{.Tool}}</span>
<span class="row__meta">{{if .Live}}Funcionando &middot; usada {{relativeSince .LastUsed}}{{else if .Configured}}Configurada &middot; ainda não se conectou; reinicie a ferramenta{{else}}Usada {{relativeSince .LastUsed}}{{end}}{{with .Version}} <span class="tip tip--quiet" tabindex="0" aria-describedby="tip-versao-{{$i}}"><span aria-hidden="true">?</span><span class="tip__body" role="tooltip" id="tip-versao-{{$i}}">{{$c.Tool}} {{.}}</span></span>{{end}}</span></span>
{{if .Live}}<span class="pill pill--ok">Conectada</span>{{else}}<span class="pill pill--warn">Aguardando</span>{{end}}
{{if not .Mark}}<a class="btn btn--quiet btn--small" href="#renomear-{{$i}}">Renomear</a>{{end}}
<a class="btn btn--danger btn--small" href="#desconectar-{{$i}}">Desconectar</a>
</li>{{end}}
</ul>
<p class="muted">Todas usam a mesma sessão do WhatsApp neste computador. Desconectar uma não mexe nas outras.</p>
{{else}}
<div class="empty"><p class="empty__title">Nenhuma ferramenta de IA conectada ainda</p>
<p class="muted">Clique no botão acima, escolha onde você vai usar e siga o passo a passo. Leva menos de um minuto.</p></div>
{{end}}
</div></section>

{{/* The first days only: after that the examples live in Ajuda. */}}
{{if and .LiveCount .Newcomer}}
<section class="card">
<div class="card__head"><h2>Experimente pedir</h2></div>
<div class="card__body">
<p class="muted">Escreva isso no chat da sua ferramenta de IA. Ler e procurar é seguro: mandar mensagem só acontece quando você pede.</p>
<ul class="prompts">
{{range .Prompts}}<li class="prompt"><div class="snippet"><pre data-copy><code>{{.}}</code></pre></div></li>{{end}}
</ul>
<p class="muted" style="margin-bottom:0">Mais exemplos e respostas para dúvidas na <a href="/ajuda">Ajuda</a>.</p>
</div></section>
{{end}}

{{range $i, $c := .Connections}}
{{if not .Mark}}<div class="overlay" id="renomear-{{$i}}" role="dialog" aria-modal="true" aria-labelledby="renomear-{{$i}}-titulo">
<div class="dialog">
<div class="dialog__head"><h2 id="renomear-{{$i}}-titulo">Renomear esta conexão</h2><a class="dialog__close" href="#" aria-label="Fechar">&times;</a></div>
<div class="dialog__body">
<form method="post" action="/conexoes/renomear"><input type="hidden" name="client" value="{{.Key}}">
<label class="field" for="nome-{{$i}}"><span class="field__label">Nome</span><span class="field__hint">Como esta ferramenta aparece aqui. Deixe em branco para voltar ao nome original.</span></label>
<input id="nome-{{$i}}" type="text" name="name" value="{{.Tool}}" maxlength="40" autocomplete="off" spellcheck="false">
<div class="actions actions--end" style="margin-top:14px"><a class="btn btn--quiet" href="#">Cancelar</a><button class="btn" type="submit">Salvar</button></div>
</form>
</div></div></div>{{end}}
<div class="overlay" id="desconectar-{{$i}}" role="dialog" aria-modal="true" aria-labelledby="desconectar-{{$i}}-titulo">
<div class="dialog">
<div class="dialog__head"><h2 id="desconectar-{{$i}}-titulo">Desconectar esta ferramenta?</h2><a class="dialog__close" href="#" aria-label="Fechar">&times;</a></div>
<div class="dialog__body">
<div class="target"><span class="target__name">{{.Tool}}</span></div>
<p>{{if .Configured}}A configuração do WhatsApp é removida do <strong>{{.Tool}}</strong>{{if eq .Key "claude-desktop"}} (com uma cópia do arquivo guardada ao lado){{end}}. Ele perde o acesso quando for reiniciado.{{else}}O <strong>{{.Tool}}</strong> sai desta lista. Se ele ainda tiver a configuração, volta a aparecer quando usar o WhatsApp de novo: remova-a na própria ferramenta.{{end}}</p>
<p class="muted">As suas outras conexões continuam funcionando normalmente.</p>
<form method="post" action="/conexoes/remover"><input type="hidden" name="client" value="{{.Key}}">
<div class="actions actions--end"><a class="btn btn--quiet" href="#">Cancelar</a><button class="btn btn--danger" type="submit">Desconectar</button></div>
</form>
</div></div></div>
{{end}}
{{template "foot" .}}{{end}}

{{define "whatsapp"}}{{template "head" .}}{{template "nav" .}}
<h1>WhatsApp</h1>
<p class="lead">A conta conectada a este computador e o que já chegou dela.</p>
{{with .OK}}<p class="alert alert--ok" role="status">{{.}}</p>{{end}}

<section class="card">
<div class="card__head"><h2>Conta</h2>{{template "syncpill" .}}</div>
<div class="card__body stack">
<div class="hello"><span class="avatar" style="background:#128c7e">{{initial .Name}}</span><div><p class="hello__name">{{with .Name}}{{.}}{{else}}Sua conta{{end}}</p><p class="hello__phone">{{.Phone}}</p></div></div>
<dl class="facts">
<div class="fact"><dt>Última mensagem recebida</dt><dd>{{relativeSince .Activity.NewestIncoming}}<span class="fact__detail">{{moment .Activity.NewestIncoming}}</span></dd></div>
<div class="fact"><dt>Mensagens guardadas</dt><dd>{{count .Coverage.Messages}}</dd></div>
<div class="fact"><dt>Conversas</dt><dd>{{count .Coverage.Chats}}</dd></div>
<div class="fact"><dt>Histórico desde</dt><dd>{{moment .Coverage.Oldest}}</dd></div>
</dl>
</div></section>

{{template "chatpreview" .}}
{{template "historycard" .}}

<section class="card">
<div class="card__head"><h2>Zona de risco</h2></div>
<div class="card__body">
<div class="actions"><a class="btn btn--danger btn--small" href="#encerrar-sessao">Desconectar este computador do WhatsApp</a></div>
<p class="muted">O computador sai da lista de dispositivos conectados do seu celular. As mensagens já guardadas continuam aqui.</p>
</div></section>

<div class="overlay" id="encerrar-sessao" role="dialog" aria-modal="true" aria-labelledby="encerrar-titulo">
<div class="dialog">
<div class="dialog__head"><h2 id="encerrar-titulo">Desconectar do WhatsApp?</h2><a class="dialog__close" href="#" aria-label="Fechar">×</a></div>
<div class="dialog__body">
<p>O pareamento é desfeito. Para voltar a usar, será preciso ler um novo QR code no celular.</p>
<p class="muted">As mensagens já guardadas continuam neste computador.</p>
<form method="post" action="/whatsapp/sair" data-busy="Desconectando…"><div class="actions actions--end"><a class="btn btn--quiet" href="#">Cancelar</a><button class="btn btn--danger" type="submit">Desconectar</button></div></form>
</div></div></div>
{{template "foot" .}}{{end}}

{{define "status"}}{{template "head" .}}{{template "nav" .}}
<h1>Status</h1>
<p class="lead">Se o seu WhatsApp está conectado e recebendo mensagens neste computador.</p>

<section class="card">
<div class="card__head"><h2>WhatsApp</h2>{{template "syncpill" .}}</div>
<div class="card__body stack">
{{if eq .Tone "ok"}}<p class="alert alert--ok">Nenhum problema detectado. As mensagens estão sendo recebidas e guardadas.</p>
{{else}}<ul class="problems">{{range .Checks}}{{if or (eq .Status "warn") (eq .Status "fail")}}<li{{if eq .Status "warn"}} class="problems__warn"{{end}}><strong>{{.Title}}:</strong> {{.Text}}</li>{{end}}{{end}}</ul>{{end}}
<dl class="facts">
<div class="fact"><dt>Última mensagem recebida</dt><dd>{{relativeSince .Activity.NewestIncoming}}<span class="fact__detail">{{moment .Activity.NewestIncoming}}</span></dd></div>
{{with .Name}}<div class="fact"><dt>Conta</dt><dd>{{.}}</dd></div>{{end}}
<div class="fact"><dt>Mensagens guardadas</dt><dd>{{count .Coverage.Messages}}</dd></div>
<div class="fact"><dt>Histórico desde</dt><dd>{{moment .Coverage.Oldest}}</dd></div>
</dl>
</div></section>

<section class="card">
<div class="card__head"><h2>Verificações</h2></div>
<div class="card__body">
<ul class="rows">
{{range .Checks}}<li class="row">
<span class="row__main"><span class="row__title">{{.Title}}</span><span class="row__meta">{{.Text}}</span></span>
<span class="pill pill--{{if eq .Status "ok"}}ok{{else if eq .Status "busy"}}warn pill--busy{{else if eq .Status "warn"}}warn{{else}}off{{end}}">{{if eq .Status "ok"}}Ok{{else if eq .Status "busy"}}<span class="spinner" aria-hidden="true"></span>Carregando{{else if eq .Status "warn"}}Atenção{{else}}Problema{{end}}</span>
</li>{{end}}
</ul>
</div></section>

{{if .Gaps}}
<section class="card">
<div class="card__head"><h2>Períodos sem nenhuma mensagem</h2></div>
<div class="card__body">
<ul class="rows">
{{range .Gaps}}<li class="row"><span class="row__main"><span class="row__title">{{moment .From}} → {{moment .Until}}</span><span class="row__meta">{{hours .Hours}} sem mensagem em nenhuma conversa</span></span></li>{{end}}
</ul>
<p class="muted">Costuma ser o computador desligado ou dormindo. O que o WhatsApp ainda guardava na fila chega sozinho quando ele volta; o resto só volta pedindo o histórico ao celular.</p>
</div></section>
{{end}}

{{template "foot" .}}{{end}}

{{define "instalacao"}}{{template "head" .}}
<header class="masthead"><a class="brand" href="/instalacao">{{template "brandmark"}}</a>
<div class="masthead__tools">{{template "settingslink" .}}</div></header>
<div class="wizard-shell"{{if eq .Step 2}} style="max-width:620px"{{end}}>
{{template "portbanner" .}}
<ol class="wizard" aria-label="Etapas da instalação" style="max-width:520px;margin-left:auto;margin-right:auto">
{{range .Steps}}<li class="wizard__step wizard__step--{{.State}}"{{if eq .State "now"}} aria-current="step"{{end}}>
<span class="wizard__n" aria-hidden="true">{{if eq .State "done"}}✓{{else}}{{.Number}}{{end}}</span><span class="wizard__label">{{.Label}}</span></li>{{end}}
</ol>

{{if eq .Step 1}}
<section class="card" data-pairing>
<div class="card__head"><h2>Conecte o seu WhatsApp</h2></div>
<div class="card__body">
<div data-pair-qr>
<ol class="guide">
<li>Abra o <strong>WhatsApp</strong> no celular.</li>
<li>Toque em <strong>Configurações</strong> (no Android, o menu <strong>⋮</strong>).</li>
<li>Toque em <strong>Dispositivos conectados</strong>, depois em <strong>Conectar um dispositivo</strong>.</li>
<li>Aponte a câmera para o código abaixo.</li>
</ol>
<div class="qrframe"><img class="qrcode" data-qr alt="QR code para conectar o WhatsApp" width="260" height="260" hidden><div class="qrframe__wait" data-qr-wait><span class="progressline"><span class="spinner" aria-hidden="true"></span>Gerando o código…</span></div></div>
<p class="muted" style="text-align:center">O código se renova sozinho a cada poucos segundos.</p>
<div class="actions" style="justify-content:center"><button class="linkbtn" type="button" data-pair-phone-toggle>Prefiro digitar um código no celular</button></div>
<div data-pair-phone hidden>
<label class="field" for="phone"><span class="field__label">Número deste WhatsApp</span>
<span class="field__hint">Com DDI e DDD. O celular pede um código de 8 caracteres, que vai aparecer aqui.</span></label>
<input id="phone" type="text" inputmode="tel" autocomplete="tel" placeholder="+55 11 91234-5678" data-pair-phone-input>
<div class="actions"><button class="btn" type="button" data-pair-phone-go>Gerar código</button></div>
</div>
</div>
<div data-pair-code hidden>
<p class="muted" data-code-wait><span class="progressline"><span class="spinner" aria-hidden="true"></span>Pedindo o código ao WhatsApp…</span></p>
<div data-code-ready hidden>
<ol class="guide">
<li>No celular, abra o WhatsApp e vá em <strong>Dispositivos conectados</strong>.</li>
<li>Toque em <strong>Conectar um dispositivo</strong> e depois em <strong>Conectar com número de telefone</strong>.</li>
<li>Digite o código abaixo.</li>
</ol>
<p class="paircode" data-code></p>
</div>
<div class="actions" style="justify-content:center"><button class="linkbtn" type="button" data-pair-back>Voltar para o QR code</button></div>
</div>
<div data-pair-sync hidden>
<p class="lead"><span class="progressline"><span class="spinner" aria-hidden="true"></span><strong>Conectado!</strong></span></p>
<p class="muted">Só um instante…</p>
</div>
<div data-pair-error hidden>
<p class="alert" role="alert" data-error-text></p>
<div class="actions"><button class="btn" type="button" data-pair-retry>Tentar de novo</button></div>
</div>
</div></section>
<p class="muted" style="text-align:center">Tudo roda neste computador: as mensagens ficam aqui, e nada passa por servidor de terceiros.</p>
{{template "autostartcheck" .}}
{{end}}

{{if eq .Step 2}}
<section class="overview" style="grid-template-columns:1fr">
<div class="overview__item overview__item--ok">
<span class="overview__icon" aria-hidden="true">&#10003;</span>
<div class="overview__body">
<p class="overview__title">WhatsApp conectado{{with .Name}}: {{.}}{{end}}</p>
<p class="overview__detail"><strong class="overview__phone">{{.Phone}}</strong>Confira se é este o número que você queria conectar.</p>
{{if .Arriving}}<p class="busy" data-sync-note style="margin-top:6px"><span class="spinner" aria-hidden="true"></span><span>O celular está mandando o seu histórico: <span data-sync-count>{{count .ArrivingCount}}</span> mensagens até agora. Pode seguir enquanto isso.</span></p>{{end}}
</div></div>
</section>
{{if .LiveCount}}
<section class="card card--accent">
<div class="card__head"><h2>Tudo pronto</h2><span class="pill pill--ok">{{plural .LiveCount "ferramenta conectada" "ferramentas conectadas"}}</span></div>
<div class="card__body">
<p class="lead">{{range $i, $c := .Connections}}{{if .Live}}{{if $i}}, {{end}}<strong>{{.Tool}}</strong>{{end}}{{end}} já está usando o seu WhatsApp.</p>
<p class="muted">Para testar, mande isto no chat:</p>
<div class="snippet"><pre class="plain" data-copy><code>{{.Verification}}</code></pre></div>
<form method="post" action="/instalacao/avancar"><input type="hidden" name="to" value="done">
<div class="actions" style="margin-top:14px"><button class="btn btn--block" type="submit">Ir para o painel</button></div></form>
</div></section>
{{else}}
<section class="card" data-wait-client="true">
<div class="card__head"><h2>Conecte a sua ferramenta de IA</h2></div>
<div class="card__body">
<p class="muted">Onde você vai usar o seu WhatsApp? Escolha e siga o passo a passo.</p>
{{template "toolpicks" .Tools}}
</div></section>
<form method="post" action="/instalacao/avancar"><input type="hidden" name="to" value="done">
<div class="wizard__escape"><button class="btn btn--quiet" type="submit">Pular por enquanto</button></div></form>
{{end}}
{{template "autostartcheck" .}}
{{end}}
</div>
{{template "foot" .}}{{end}}

{{define "funcoes"}}{{template "head" .}}{{template "nav" .}}
<h1>O que o MCP sabe fazer</h1>
<p class="lead">As {{.Count}} funções que a sua ferramenta de IA pode usar no seu WhatsApp.</p>
<div class="tools">
{{range .Tools}}
<article class="tool">
<div class="tool__head"><span class="tool__name">{{.Name}}</span></div>
<p class="tool__desc">{{.Description}}</p>
{{if .Arguments}}<ul class="tool__args">
{{range .Arguments}}<li class="tool__arg"><span class="tool__argname">{{.Name}}</span><span class="tool__type">{{.Type}}</span>{{if .Required}}<span class="tool__req">obrigatório</span>{{end}}<span class="tool__argdesc">{{.Description}}{{if .Choices}} Valores: {{range $i, $c := .Choices}}{{if $i}}, {{end}}{{$c}}{{end}}.{{end}}</span></li>{{end}}
</ul>{{else}}<p class="tool__args muted">Sem argumentos.</p>{{end}}
</article>
{{end}}
</div>
{{template "foot" .}}{{end}}

{{define "ajuda"}}{{template "head" .}}{{template "nav" .}}
<h1>Ajuda</h1>
<p class="lead">Respostas para as dúvidas mais comuns e exemplos para começar.</p>

<section class="card" id="perguntas">
<div class="card__head"><h2 class="card__title">{{icon "help"}}Perguntas frequentes</h2></div>
<div class="card__body">
<div class="faq">
<details class="faq__item"><summary>Funciona no Claude pelo navegador ou pelo celular?{{icon "chevron"}}</summary>
<div class="faq__answer"><p>Não. O WhatsApp MCP fica neste computador, e só programas instalados nele conseguem usá-lo, como o Claude Desktop, o Claude Code, o Codex e o Cursor. O Claude no navegador, o app do celular e o ChatGPT na web rodam na nuvem e não alcançam o seu computador.</p></div></details>
<details class="faq__item"><summary>A IA pode mandar mensagens sem eu pedir?{{icon "chevron"}}</summary>
<div class="faq__answer"><p>Não. Ler e procurar é seguro: mandar, reagir, editar ou apagar uma mensagem só acontece quando você pede.</p></div></details>
<details class="faq__item"><summary>As minhas mensagens saem deste computador?{{icon "chevron"}}</summary>
<div class="faq__answer"><p>O WhatsApp MCP guarda as mensagens só aqui e não manda nada para servidores de terceiros. Quando você pede algo à sua ferramenta de IA, ela lê as mensagens de que precisa para responder, e o que ela faz com elas segue as regras de privacidade dela.</p></div></details>
<details class="faq__item"><summary>O computador precisa ficar ligado?{{icon "chevron"}}</summary>
<div class="faq__answer"><p>Sim, para receber as mensagens. Se ele ficar desligado ou dormindo por pouco tempo, o WhatsApp entrega o que ficou pendente quando ele volta. Se ainda faltar alguma coisa, peça o histórico ao celular na aba <a href="/whatsapp">WhatsApp</a>.</p></div></details>
<details class="faq__item"><summary>Como trazer mensagens mais antigas?{{icon "chevron"}}</summary>
<div class="faq__answer"><p>Na aba <a href="/whatsapp">WhatsApp</a>, use "Buscar mensagens mais antigas", para uma conversa ou para todas as recentes. Cada pedido traz mais histórico do celular para este computador, e ele fica disponível para a sua ferramenta de IA consultar. O celular precisa estar com internet.</p></div></details>
<details class="faq__item"><summary>A IA consegue ouvir os meus áudios?{{icon "chevron"}}</summary>
<div class="faq__answer"><p>Sim, depois de instalar a <a href="/configuracoes#transcricao">Transcrição de áudio</a> nas Configurações. Quando você pede, o áudio vira texto aqui mesmo, sem sair do computador. O nome da conversa e as últimas mensagens ajudam a acertar nomes e termos, e cada áudio é transcrito uma vez só.</p></div></details>
<details class="faq__item"><summary>Posso conectar mais de uma ferramenta de IA?{{icon "chevron"}}</summary>
<div class="faq__answer"><p>Pode. Todas usam o mesmo WhatsApp deste computador, e desconectar uma não mexe nas outras. Para adicionar outra, use <a href="/conectar">Conectar ferramenta de IA ao MCP</a>.</p></div></details>
<details class="faq__item"><summary>A minha ferramenta não aparece conectada. E agora?{{icon "chevron"}}</summary>
<div class="faq__answer"><p>Feche a ferramenta e abra de novo, ou comece uma conversa nova: ela só carrega o WhatsApp ao iniciar. Se continuar assim, refaça o passo a passo em <a href="/conectar">Conectar ferramenta de IA ao MCP</a> e confira a aba <a href="/status">Status</a>.</p></div></details>
<details class="faq__item"><summary>O que a IA consegue fazer no meu WhatsApp?{{icon "chevron"}}</summary>
<div class="faq__answer"><p>Ler, procurar e resumir conversas, transcrever áudios, mandar mensagens, fotos, enquetes e localização, reagir, editar e apagar mensagens e organizar conversas. A lista completa está em <a href="/funcoes">Funções</a>.</p></div></details>
<details class="faq__item"><summary>Como desconectar este computador do WhatsApp?{{icon "chevron"}}</summary>
<div class="faq__answer"><p>Na aba <a href="/whatsapp">WhatsApp</a>, em Zona de risco. O computador sai dos dispositivos conectados do seu celular, e as mensagens já guardadas continuam aqui.</p></div></details>
{{if .App}}<details class="faq__item"><summary>Onde ficam os meus dados?{{icon "chevron"}}</summary>
<div class="faq__answer"><p>Numa pasta deste computador, que aparece em <a href="/configuracoes#dados">Configurações › Dados</a>. Lá também dá para abrir a pasta ou apagar tudo.</p></div></details>{{end}}
</div>
</div></section>

<section class="card" id="como-usar">
<div class="card__head"><h2 class="card__title">{{icon "chat"}}Como usar</h2></div>
<div class="card__body stack">
<p class="muted">Escreva isso no chat da sua ferramenta de IA. Ler e procurar é seguro: mandar mensagem só acontece quando você pede.</p>
<ul class="prompts">
{{range .Prompts}}<li class="prompt"><div class="snippet"><pre data-copy><code>{{.}}</code></pre></div></li>{{end}}
</ul>
<div class="note"><p style="margin:0">Quer ir além? As <a href="/receitas">Receitas</a> trazem pedidos prontos para agendar mensagens, vigiar assuntos, resumir grupos e mais.</p></div>
</div></section>
{{template "foot" .}}{{end}}

{{define "receitas"}}{{template "head" .}}{{template "nav" .}}
<h1>Receitas</h1>
<p class="lead">O WhatsApp MCP só responde pelo WhatsApp quando perguntado: esperar a hora, vigiar um termo e montar o relatório são trabalho da sua ferramenta de IA, escrito como instrução. Cada receita é um texto para colar.</p>
<div class="recipes">
{{range .Recipes}}
<article class="recipe">
<div class="recipe__head">
<h2 class="recipe__title">{{.Title}}</h2>
<p class="recipe__summary">{{.Summary}}</p>
</div>
<div class="recipe__meta">
{{range .Uses}}<span class="recipe__tool">{{.}}</span>{{end}}
{{with .Schedule}}<span class="recipe__tool recipe__tool--when">⏱ {{.}}</span>{{end}}
</div>
<div class="recipe__prompt">
<div class="snippet"><div class="snippet__head"><span class="snippet__title">Prompt</span></div>
<pre data-copy><code>{{.Prompt}}</code></pre></div>
</div>
{{with .Caveat}}<p class="recipe__caveat"><strong>Atenção:</strong> {{.}}</p>{{end}}
</article>
{{end}}
</div>
{{template "foot" .}}{{end}}

{{define "configuracoes"}}{{template "head" .}}{{template "nav" .}}
<h1>Configurações</h1>
<p class="lead">Como o WhatsApp MCP roda neste computador.</p>
{{with .OK}}<p class="alert alert--ok" role="status">{{.}}</p>{{end}}

{{if and .App .PortChanged}}
<section class="card card--accent">
<div class="card__head"><h2 class="card__title">{{icon "bell"}}Avise as suas ferramentas de IA</h2></div>
<div class="card__body stack">
<ul class="rows">
<li class="row"><span class="row__main"><span class="row__title">Claude Desktop e Cowork</span><span class="row__meta">Nada a fazer: ele encontra a porta nova sozinho. Uma conversa já aberta reconecta no próximo uso.</span></span><span class="pill pill--ok">Automático</span></li>
<li class="row"><span class="row__main"><span class="row__title">Claude Code</span><span class="row__meta">{{if .Setup.Code.Configured}}Já usa o endereço novo.{{else if .Setup.Code.Found}}Ele guarda o endereço com a porta: atualize com um clique.{{else}}O comando claude não foi encontrado neste computador.{{end}}</span></span>
{{if and .Setup.Code.Found (not .Setup.Code.Configured)}}<button class="btn btn--small" type="button" data-add-client="claude-code">Atualizar o Claude Code agora</button>{{end}}</li>
{{if or .Setup.Codex.Configured .Setup.Codex.Stale}}<li class="row"><span class="row__main"><span class="row__title">Codex</span><span class="row__meta">{{if .Setup.Codex.Configured}}Já usa o endereço novo.{{else}}Ele guarda o endereço com a porta: atualize com um clique.{{end}}</span></span>
{{if not .Setup.Codex.Configured}}<button class="btn btn--small" type="button" data-add-client="codex">Atualizar o Codex agora</button>{{end}}</li>{{end}}
</ul>
<p class="busy" data-client-note="claude-code" role="status" hidden></p>
<p class="busy" data-client-note="codex" role="status" hidden></p>
{{if or .Setup.Cursor.Configured .Setup.Cursor.Stale}}<ul class="rows" style="margin-top:12px"><li class="row"><span class="row__main"><span class="row__title">Cursor</span><span class="row__meta">{{if .Setup.Cursor.Configured}}Já usa o endereço novo.{{else}}Ele guarda o endereço com a porta: atualize com um clique.{{end}}</span></span>
{{if not .Setup.Cursor.Configured}}<button class="btn btn--small" type="button" data-add-client="cursor">Atualizar o Cursor agora</button>{{end}}</li></ul>
<p class="busy" data-client-note="cursor" role="status" hidden></p>{{end}}
<p class="muted">Outras ferramentas (Windsurf e outras) guardam o endereço antigo e precisam ser configuradas de novo. Mande isto no chat delas:</p>
<div class="snippet"><pre class="plain" data-copy><code>{{.Setup.AgentPrompt}}</code></pre></div>
</div></section>
{{end}}

{{if .App}}
<section class="card" id="porta">
<div class="card__head"><h2 class="card__title">{{icon "port"}}Porta do MCP</h2></div>
<div class="card__body stack">
<p class="muted">As ferramentas de IA falam com o WhatsApp MCP por este endereço, que só responde a programas instalados e rodando neste computador. Troque a porta se outro programa já usa esta, ou se você quiser uma porta específica.</p>
<div class="snippet"><div class="snippet__head"><span class="snippet__title">Endereço</span></div><pre data-copy><code>{{.Setup.Endpoint}}</code></pre></div>
{{if .Settings.PortLocked}}<p class="note">A porta está definida pela variável de ambiente <code>WHATSAPP_MCP_PORT</code> e não pode ser trocada aqui.</p>
{{else}}<form method="post" action="/configuracoes/porta" data-busy="Trocando…">
<label class="field" for="port"><span class="field__label">Porta</span><span class="field__hint">Um número de 1024 a 65535. Trocar a porta desconecta as ferramentas de IA do MCP. <span class="tip" tabindex="0" aria-describedby="tip-porta"><span aria-hidden="true">?</span><span class="tip__body" role="tooltip" id="tip-porta">O WhatsApp continua conectado. As ferramentas de IA ligadas por MCP, como Claude, Codex e outras, perdem a conexão e precisam do endereço novo. Depois de salvar, o app mostra como atualizar cada uma.</span></span></span></label>
<div class="actions"><input id="port" class="input--short" type="number" name="port" min="1024" max="65535" value="{{.Port}}" required><button class="btn" type="submit">Salvar</button></div>
</form>{{end}}
</div></section>

<section class="card">
<div class="card__head"><h2 class="card__title">{{icon "power"}}Ao ligar e ao fechar</h2></div>
<div class="card__body stack">
<label class="check"><input type="checkbox" data-setting="autostart"{{if .Settings.Autostart}} checked{{end}}><span><strong>Abrir o WhatsApp MCP quando o computador ligar</strong><span class="check__hint">Ele abre só na {{tray}}, sem janela, e as ferramentas de IA já encontram o WhatsApp.</span></span></label>
{{if .Settings.CanHide}}<label class="check"><input type="checkbox" data-setting="close_to_tray"{{if .Settings.CloseToTray}} checked{{end}}><span><strong>Fechar a janela mantém o app na {{tray}}</strong><span class="check__hint">{{if mac}}Vale também para o ⌘Q. {{end}}Desmarcado, fechar a janela encerra o WhatsApp MCP, e as ferramentas de IA perdem o acesso até ele ser aberto de novo.</span></span></label>
{{else}}<p class="muted">Este sistema não mostra ícones na bandeja (no GNOME, isso pede a extensão AppIndicator). Por isso, fechar a janela só a minimiza.</p>{{end}}
<p class="busy" data-setting-note role="status" hidden></p>
<div class="actions"><a class="btn btn--danger btn--small" href="#encerrar-app">Encerrar o WhatsApp MCP</a></div>
<p class="muted">Desliga o app de vez, até você abrir de novo.</p>
</div></section>

<div class="overlay" id="encerrar-app" role="dialog" aria-modal="true" aria-labelledby="encerrar-app-titulo">
<div class="dialog">
<div class="dialog__head"><h2 id="encerrar-app-titulo">Encerrar o WhatsApp MCP?</h2><a class="dialog__close" href="#" aria-label="Fechar">&times;</a></div>
<div class="dialog__body">
<p>O MCP é desligado: as ferramentas de IA perdem o acesso ao WhatsApp, e este computador para de receber mensagens enquanto o app estiver fechado.</p>
<p class="muted">Para voltar, abra o WhatsApp MCP de novo.</p>
<form method="post" action="/configuracoes/encerrar" data-busy="Encerrando…"><div class="actions actions--end"><a class="btn btn--quiet" href="#">Cancelar</a><button class="btn btn--danger" type="submit">Encerrar</button></div></form>
</div></div></div>

{{end}}

<section class="card" id="webhooks" data-webhooks>
<div class="card__head"><h2 class="card__title">{{icon "code"}}Webhooks</h2><a class="btn btn--ghost btn--small" href="/webhooks/documentacao">{{icon "book"}}Documentação</a></div>
<div class="card__body stack">
<p class="muted">Avisam um programa seu a cada mensagem nova que chega, para ele agir sozinho: responder, registrar numa planilha, avisar em outro lugar. São para quem usa scripts ou automações. <span class="tip" tabindex="0" aria-describedby="tip-webhooks"><span aria-hidden="true">?</span><span class="tip__body" role="tooltip" id="tip-webhooks">A cada mensagem, o WhatsApp MCP faz um POST com JSON no endereço, assinado com a chave do webhook. Se o endereço não responder com sucesso, ele tenta de novo até 10 vezes em menos de um minuto; depois disso o webhook é desligado e os avisos que esperavam são descartados. O formato de cada aviso está em Documentação, no alto deste card.</span></span></p>
<p class="note" data-webhooks-unavailable hidden>O wacli instalado não avisa mensagens novas: atualize-o para os webhooks funcionarem.</p>
<p class="busy" data-webhooks-loading role="status"><span class="spinner" aria-hidden="true"></span>Carregando…</p>
<ul class="rows" data-webhooks-list hidden></ul>
<div class="empty" data-webhooks-empty hidden><p class="empty__title">Nenhum webhook ainda</p><p class="muted" style="margin:6px 0 0">Adicione abaixo o endereço do seu programa.</p></div>
<div class="secret" data-webhook-secret hidden>
<p class="secret__title">Webhook adicionado. Guarde a chave dele</p>
<p class="muted" style="margin:0">Ela não aparece de novo. Com ela, o seu programa confere que cada aviso veio mesmo do WhatsApp MCP.</p>
<code class="secret__value" data-webhook-secret-value></code>
<div class="actions"><button class="btn btn--ghost btn--small" type="button" data-webhook-secret-copy>Copiar a chave</button><button class="btn btn--quiet btn--small" type="button" data-webhook-secret-close>Já guardei</button></div>
</div>
<form class="stack" data-webhook-form>
<h3 class="card__sub">Adicionar um webhook</h3>
<label class="field" for="webhook-url"><span class="field__label">Endereço</span><span class="field__hint">O endereço do seu programa, começando com http:// ou https://.</span></label>
<input id="webhook-url" type="text" inputmode="url" autocomplete="off" spellcheck="false" placeholder="http://127.0.0.1:8080/whatsapp" required data-webhook-url>
<span class="field__label">Avisar quando</span>
<label class="check"><input type="checkbox" value="message" checked data-webhook-event><span>Chegar uma mensagem</span></label>
<label class="check"><input type="checkbox" value="reaction" data-webhook-event><span>Alguém reagir a uma mensagem</span></label>
<label class="check"><input type="checkbox" value="receipt" data-webhook-event><span>Uma mensagem sua for entregue, lida ou ouvida</span></label>
<label class="check"><input type="checkbox" data-webhook-own><span>Incluir as mensagens que você manda<span class="check__hint">Do celular ou pela sua ferramenta de IA.</span></span></label>
<div class="actions"><button class="btn btn--small" type="submit">Adicionar webhook</button></div>
<p class="alert" role="alert" data-webhook-error hidden></p>
</form>
</div></section>

<section class="card" id="transcricao" data-asr>
<div class="card__head"><h2 class="card__title">{{icon "mic"}}Transcrição de áudio</h2>{{if .Local.Ready}}<span class="pill pill--ok">Ativa</span>{{else}}<span class="pill pill--off">Não instalada</span>{{end}}</div>
<div class="card__body stack">
{{if .Local.Ready}}
<p class="muted">Quando você pede à sua ferramenta de IA para ler um áudio, ele é transcrito aqui mesmo, com o Whisper (large-v3-turbo) rodando {{accel .Local.Accel}}: grátis, e o áudio não sai do computador. Nada é transcrito sem você pedir. O nome da conversa, as pessoas e as últimas mensagens entram como contexto, e a sua ferramenta de IA confere a transcrição contra a conversa e corrige o que soou estranho.</p>
<dl class="facts">
<div class="fact"><dt>Áudios transcritos</dt><dd>{{.Total}}</dd></div>
<div class="fact"><dt>Corrigidos pelo contexto</dt><dd>{{.Corrected}}</dd></div>
</dl>
{{else if .Local.Supported}}
<p class="muted">Transcreva os áudios aqui mesmo, de graça e sem o áudio sair do computador, com o Whisper (large-v3-turbo) rodando {{accel .Local.Accel}}. A instalação baixa cerca de 600 MB, uma vez só.</p>
<div class="actions"><button class="btn" type="button" data-asr-install>Instalar a transcrição local</button></div>
<p class="busy" data-asr-progress role="status"{{if ne .Local.Install.State "running"}} hidden{{end}}><span class="spinner" aria-hidden="true"></span><span data-asr-step>{{.Local.Install.Step}}</span></p>
{{if eq .Local.Install.State "error"}}<p class="alert" role="alert">{{.Local.Install.Error}}</p>{{end}}
{{else}}
<p class="muted">A transcrição neste computador ainda não está disponível para este sistema. Enquanto isso, a sua ferramenta de IA pode baixar o áudio e transcrevê-lo do jeito que preferir.</p>
{{end}}
{{if and .Local.Supported (eq .Local.Accel "cpu")}}<p class="note">Este computador não tem uma placa de vídeo que o Whisper aproveite, então ele roda no processador: cada áudio leva mais ou menos o próprio tempo para ser transcrito. Funciona, só é mais lento.</p>{{end}}
<h3 class="card__sub">Como usar</h3>
<p class="muted">Peça à sua ferramenta de IA algo como a mensagem abaixo. Os áudios já transcritos vêm junto das mensagens; os outros ela transcreve na hora, confere com a conversa e guarda a versão corrigida.</p>
<div class="snippet"><div class="snippet__head"><span class="snippet__title">Exemplo</span></div>
<pre class="plain" data-copy><code>Transcreva os áudios que recebi hoje no WhatsApp.</code></pre></div>
<p class="muted">Cada áudio é transcrito uma vez: pedir de novo devolve o texto guardado.</p>
</div></section>

{{if .App}}
<section class="card" id="atualizacoes" data-update>
<div class="card__head"><h2 class="card__title">{{icon "refresh"}}Versão e atualizações</h2></div>
<div class="card__body stack">
<dl class="facts"><div class="fact"><dt>Versão instalada</dt><dd>{{.Settings.Version}}</dd></div><div class="fact"><dt>Mais recente</dt><dd data-update-latest>{{with .Update.Latest}}{{.}}{{else}}—{{end}}</dd></div></dl>
<p class="muted" data-update-text hidden></p>
<div class="actions"><button class="btn btn--ghost btn--small" type="button" data-update-check>Procurar atualização</button><button class="btn btn--small" type="button" data-update-install hidden>Instalar e reiniciar</button><a class="btn btn--ghost btn--small" data-update-page href="#" target="_blank" rel="noopener" hidden>Baixar a versão nova</a></div>
</div></section>

{{end}}

<section class="card" id="aparencia" hidden data-theme-switch>
<div class="card__head"><h2 class="card__title">{{icon "palette"}}Aparência</h2></div>
<div class="card__body">
<fieldset class="themes"><legend class="sr-only">Tema</legend>
<label class="theme-pick"><input type="radio" name="tema" value="light" data-theme-choice><span class="theme-pick__preview" aria-hidden="true"><span class="mini mini--light"><i></i><i></i><i></i></span></span><span class="theme-pick__label">{{icon "sun"}}Claro</span></label>
<label class="theme-pick"><input type="radio" name="tema" value="dark" data-theme-choice><span class="theme-pick__preview" aria-hidden="true"><span class="mini mini--dark"><i></i><i></i><i></i></span></span><span class="theme-pick__label">{{icon "moon"}}Escuro</span></label>
<label class="theme-pick"><input type="radio" name="tema" value="system" data-theme-choice><span class="theme-pick__preview" aria-hidden="true"><span class="mini mini--light"><i></i><i></i><i></i></span><span class="mini mini--dark mini--half"><i></i><i></i><i></i></span></span><span class="theme-pick__label">{{icon "monitor"}}Sistema</span></label>
</fieldset>
</div></section>

<section class="card" id="arquivos" data-media>
<div class="card__head"><h2 class="card__title">{{icon "folder"}}Arquivos baixados</h2></div>
<div class="card__body stack">
<p class="muted">Fotos, áudios e documentos que a sua ferramenta de IA abriu ficam guardados neste computador, para não serem baixados de novo. Apagar não perde nenhuma mensagem: se precisar, o arquivo é baixado outra vez, enquanto o WhatsApp ainda o tiver.</p>
<dl class="facts">
<div class="fact"><dt>Espaço usado</dt><dd data-media-bytes>…</dd></div>
<div class="fact"><dt>Arquivos</dt><dd data-media-files>…</dd></div>
<div class="fact"><dt>Exportações</dt><dd data-media-exports>…</dd></div>
</dl>
<p class="muted" data-media-types hidden></p>
<div class="actions"><label class="check check--inline"><input type="checkbox" data-media-retention><span>Apagar sozinho os arquivos baixados há mais de</span></label><input class="input--short" type="number" min="1" max="3650" value="30" aria-label="dias" data-media-days disabled><span class="muted">dias</span></div>
<p class="muted" style="margin:0">As exportações de conversas ficam até você apagar. <span class="tip" tabindex="0" aria-describedby="tip-exportacoes"><span aria-hidden="true">?</span><span class="tip__body" role="tooltip" id="tip-exportacoes">Quando você pede à sua ferramenta de IA para exportar conversas, ela grava um arquivo com as mensagens neste computador, para analisar sem carregar tudo na conversa.</span></span></p>
<div class="actions"><button class="btn btn--ghost btn--small" type="button" data-media-clear hidden>Apagar os arquivos baixados</button><button class="btn btn--ghost btn--small" type="button" data-media-clear-exports hidden>Apagar as exportações</button></div>
<p class="busy" data-media-note role="status" hidden></p>
</div></section>
<script src="/assets/settings.js" defer></script>

{{if .App}}
<section class="card" id="dados">
<div class="card__head"><h2 class="card__title">{{icon "folder"}}Dados</h2></div>
<div class="card__body stack">
<p class="muted">As mensagens, a sessão do WhatsApp, as transcrições, o modelo de transcrição e as mídias baixadas ficam nesta pasta. Desinstalar o app não a apaga.</p>
<div class="snippet"><pre data-copy><code>{{.Settings.DataDir}}</code></pre></div>
<div class="actions"><button class="btn btn--ghost btn--small" type="button" data-open-folder>Abrir a pasta</button><a class="btn btn--danger btn--small" href="#apagar-tudo">Apagar todos os dados deste computador</a></div>
</div></section>

<div class="overlay" id="apagar-tudo" role="dialog" aria-modal="true" aria-labelledby="apagar-tudo-titulo">
<div class="dialog">
<div class="dialog__head"><h2 id="apagar-tudo-titulo">Apagar tudo?</h2><a class="dialog__close" href="#" aria-label="Fechar">&times;</a></div>
<div class="dialog__body">
<p>Este computador sai dos dispositivos conectados do seu WhatsApp. As mensagens guardadas, as transcrições, o modelo e as mídias são apagados, e o app fecha. Não tem volta.</p>
<p class="muted">O WhatsApp no celular não perde nada.</p>
<form method="post" action="/configuracoes/apagar" data-busy="Apagando…">
<label class="field" for="confirm"><span class="field__label">Digite <strong>apagar</strong> para confirmar</span></label>
<input id="confirm" type="text" name="confirm" autocomplete="off" spellcheck="false" required>
<div class="actions actions--end" style="margin-top:14px"><a class="btn btn--quiet" href="#">Cancelar</a><button class="btn btn--danger" type="submit">Apagar tudo</button></div>
</form>
</div></div></div>
{{end}}
{{template "foot" .}}{{end}}

{{define "encerrado"}}{{template "head" .}}
<div class="wizard-shell" style="max-width:520px">
<section class="card">
<div class="card__head"><h2>Encerrando</h2></div>
<div class="card__body"><p class="muted">O WhatsApp MCP está fechando. Para voltar a usar, abra o app de novo.</p></div>
</section></div></div></body></html>{{end}}

{{define "apagado"}}{{template "head" .}}
<div class="wizard-shell" style="max-width:520px">
<section class="card">
<div class="card__head"><h2>Tudo apagado</h2></div>
<div class="card__body"><p class="muted">Este computador saiu dos dispositivos conectados do seu WhatsApp, e os dados foram apagados. O WhatsApp MCP vai fechar.</p></div>
</section></div></div></body></html>{{end}}`
