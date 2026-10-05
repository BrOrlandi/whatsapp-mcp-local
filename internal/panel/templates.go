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
<a class="colophon__link" href="{{authorURL}}" rel="noopener noreferrer" target="_blank">{{author}}</a>
<span class="colophon__sep">&middot;</span>
<a class="colophon__link" href="{{repositoryURL}}" rel="noopener noreferrer" target="_blank">{{template "githubmark"}}<span>C&oacute;digo-fonte</span></a>
<span class="colophon__sep">&middot;</span>
<span class="colophon__version" title="Vers&atilde;o em execu&ccedil;&atilde;o">v{{version}}</span>
{{with supportURL}}<span class="colophon__sep">&middot;</span>
<a class="colophon__support" href="{{.}}" rel="noopener noreferrer" target="_blank">Apoie o projeto</a>{{end}}
</p>
</footer>
<script src="/assets/app.js" defer></script></div></body></html>{{end}}

{{define "githubmark"}}<svg class="colophon__icon" viewBox="0 0 16 16" width="15" height="15" aria-hidden="true" focusable="false"><path fill="currentColor" d="M8 0C3.58 0 0 3.58 0 8a8 8 0 0 0 5.47 7.59c.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82a7.4 7.4 0 0 1 2-.27c.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0 0 16 8c0-4.42-3.58-8-8-8Z"/></svg>{{end}}

{{define "themeswitch"}}<span class="theme" hidden data-theme-switch>
<select class="theme__select" aria-label="Tema da interface" data-theme-select>
<option value="system" title="Seguir o sistema">◐</option>
<option value="light" title="Tema claro">☀</option>
<option value="dark" title="Tema escuro">☾</option>
</select></span>{{end}}

{{define "brandmark"}}<span class="brand__mark">{{logo}}</span><span class="brand__name">WhatsApp MCP</span>{{end}}

{{define "nav"}}
<header class="masthead"><a class="brand" href="/">{{template "brandmark"}}</a>
<div class="masthead__tools">{{template "themeswitch"}}</div></header>
<nav class="nav" aria-label="Seções">
<a href="/"{{if eq .Active "conectar"}} aria-current="page"{{end}}>Conectar</a>
<a href="/whatsapp"{{if eq .Active "whatsapp"}} aria-current="page"{{end}}>WhatsApp</a>
<a href="/estado"{{if eq .Active "estado"}} aria-current="page"{{end}}>{{if and (ne .HealthTone "ok") (ne .HealthTone "")}}<span class="nav__alert{{if eq .HealthTone "warn"}} nav__alert--warn{{end}}" aria-hidden="true">!</span><span class="sr-only">Atenção: </span>{{end}}Estado</a>
<a href="/transcricao"{{if eq .Active "transcricao"}} aria-current="page"{{end}}>Transcrição</a>
<a href="/documentacao"{{if eq .Active "documentacao"}} aria-current="page"{{end}}>Documentação</a>
<a href="/receitas"{{if eq .Active "receitas"}} aria-current="page"{{end}}>Receitas</a>
{{if .App}}<a href="/configuracoes"{{if eq .Active "configuracoes"}} aria-current="page"{{end}}>Configurações</a>{{end}}
</nav>
{{template "portbanner" .}}
{{if and .App (ne .Active "configuracoes") (eq .Update.State "available" "ready" "manual")}}<p class="note">A versão {{.Update.Latest}} do WhatsApp MCP está disponível. <a href="/configuracoes#atualizacoes">Atualizar</a></p>{{end}}
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
{{define "syncpill"}}<span class="pill pill--{{.SyncTone}}{{if .SyncBusy}} pill--busy{{end}}"{{if .SyncBusy}} data-sync-wait="{{.Sync.State}}"{{end}}>{{if .SyncBusy}}<span class="spinner" aria-hidden="true"></span>{{end}}{{.SyncLabel}}</span>{{end}}

{{define "autostartcheck"}}{{if .App}}<div class="wizard__escape"><label class="check check--inline"><input type="checkbox" data-setting="autostart"{{if .Settings.Autostart}} checked{{end}}><span>Abrir o WhatsApp MCP quando o computador ligar</span></label></div>
<p class="busy" data-setting-note role="status" hidden></p>{{end}}{{end}}

{{/* The three routes out of this panel, in the order to try them: the app
that needs no terminal, the terminal, and the "whatever you use" escape hatch.
Each has the one-click way first and the by-hand way beneath it. */}}
{{define "clientTabs"}}
<div class="tabs">
<input class="tabs__radio" type="radio" name="aba" id="tab-desktop" checked>
<input class="tabs__radio" type="radio" name="aba" id="tab-code">
<input class="tabs__radio" type="radio" name="aba" id="tab-outros">
<div class="tabs__bar" role="tablist">
<label class="tabs__tab" for="tab-desktop">Claude Desktop</label>
<label class="tabs__tab" for="tab-code">Claude Code</label>
<label class="tabs__tab" for="tab-outros">Outra ferramenta</label>
</div>

<div class="tabs__panel tabs__panel--desktop">
{{if .Desktop.Configured}}<p class="alert alert--ok">O Claude Desktop já está configurado. Se ele ainda não aparece conectado, feche e abra o Claude Desktop.</p>{{end}}
<p class="muted">O aplicativo do Claude no computador. Serve para o chat e para o Cowork.</p>
<div class="actions"><button class="btn" type="button" data-add-client="claude-desktop">{{if .Desktop.Configured}}Configurar de novo{{else}}Adicionar ao Claude Desktop{{end}}</button></div>
<p class="busy" data-client-note="claude-desktop" role="status" hidden></p>
<ol class="guide" style="margin-top:14px">
<li>Clique no botão acima: o painel escreve a configuração do Claude Desktop por você, guardando uma cópia do arquivo anterior.</li>
<li><strong>Feche o Claude Desktop e abra de novo.</strong> Só assim ele lê a configuração nova.</li>
<li>Pronto: esta tela avisa sozinha quando ele se conectar.</li>
</ol>
<details class="resend"><summary>Prefiro fazer à mão</summary>
<p class="muted">No Claude Desktop, vá em <strong>Configurações → Desenvolvedor → Editar configuração</strong>, cole o texto abaixo e reinicie o app. Se já houver outros servidores no arquivo, acrescente só o trecho <code>"{{serverName}}"</code> dentro de <code>mcpServers</code>, sem apagar o resto.</p>
<div class="snippet"><div class="snippet__head"><span class="snippet__title">{{.DesktopPath}}</span></div>
<pre data-copy><code>{{.JSON}}</code></pre></div>
</details>
</div>

<div class="tabs__panel tabs__panel--code">
{{if .Code.Configured}}<p class="alert alert--ok">O Claude Code já está configurado. Abra uma sessão nova do Claude Code para ele carregar o WhatsApp.</p>{{end}}
<p class="muted">O Claude que roda no terminal. Ele conecta direto em <code>{{.Endpoint}}</code>.</p>
<div class="actions"><button class="btn" type="button" data-add-client="claude-code"{{if not .Code.Found}} disabled title="O comando claude não foi encontrado neste computador"{{end}}>{{if .Code.Configured}}Configurar de novo{{else}}Adicionar ao Claude Code{{end}}</button></div>
<p class="busy" data-client-note="claude-code" role="status" hidden></p>
{{if not .Code.Found}}<p class="muted">O comando <code>claude</code> não foi encontrado. Instale o Claude Code ou rode o comando abaixo onde ele estiver.</p>{{end}}
<details class="resend"{{if not .Code.Found}} open{{end}}><summary>Prefiro rodar o comando</summary>
<div class="snippet"><div class="snippet__head"><span class="snippet__title">Comando</span></div>
<pre data-copy><code>{{.Command}}</code></pre></div>
<p class="muted">Confira depois com <code>claude mcp list</code>.</p>
</details>
</div>

<div class="tabs__panel tabs__panel--outros">
<p class="muted">Serve para Cursor, Windsurf, Codex e qualquer outro assistente que aceite MCP e rode neste computador. Em vez de você configurar, peça para ele: copie o texto abaixo e mande no chat da ferramenta.</p>
<div class="snippet"><div class="snippet__head"><span class="snippet__title">Copie e mande para o seu assistente</span></div>
<pre class="plain" data-copy><code>{{.AgentPrompt}}</code></pre></div>
<p class="muted">O claude.ai no navegador, o app do celular e o ChatGPT na web não alcançam este computador: quem conecta, neles, é a nuvem da empresa, e ela não chega a um servidor local.</p>
</div>
</div>
{{end}}

{{define "overlays"}}
<div class="overlay" id="nova-conexao" role="dialog" aria-modal="true" aria-labelledby="nova-conexao-titulo">
<div class="dialog dialog--wide">
<div class="dialog__head"><h2 id="nova-conexao-titulo">Conectar ferramenta de IA ao MCP</h2><a class="dialog__close" href="#" aria-label="Fechar">&times;</a></div>
<div class="dialog__body">{{template "clientTabs" .Setup}}</div>
</div></div>
{{end}}

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
<p class="muted">Este computador guarda o que chegou desde que o WhatsApp foi conectado, mais o histórico que o celular mandou no começo. Para ir mais para trás, o painel pede ao celular as mensagens anteriores às que já estão aqui. O celular precisa estar com internet, e cada pedido recua mais um trecho.</p>
{{with .History}}<p class="muted">{{if .FinishedAt}}Último pedido: {{count .Added}} mensagens novas em {{len .Chats}} conversas.{{else}}Buscando: {{.Done}} de {{len .Chats}} conversas, {{count .Added}} mensagens novas até agora.{{end}}</p>{{end}}
<div class="actions"><button class="btn btn--ghost btn--small" type="button" data-history-all>Buscar mensagens mais antigas das conversas recentes</button></div>
<p class="busy" data-history-all-note role="status" hidden></p>
</div></section>
{{end}}

{{define "conectar"}}{{template "head" .}}{{template "nav" .}}
<h1>Seu WhatsApp nas suas ferramentas de IA</h1>
<p class="lead">Aqui você vê se está tudo funcionando e liga o seu WhatsApp a uma ferramenta de inteligência artificial que aceite MCP, como o Claude, o Codex ou a que você usar.</p>

<section class="overview" data-wait-client="{{if .LiveCount}}false{{else}}true{{end}}">
<div class="overview__item overview__item--{{if eq .SyncTone "ok"}}ok{{else}}wait{{end}}"{{if .SyncBusy}} data-sync-wait="{{.Sync.State}}"{{end}}>
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

<div class="hero"><a class="btn btn--big" href="#nova-conexao">Conectar ferramenta de IA ao MCP</a></div>

<section class="card">
<div class="card__head"><h2>Suas conexões</h2>{{if .Connections}}<a class="btn btn--ghost btn--small" href="#nova-conexao">Nova conexão</a>{{end}}</div>
<div class="card__body">
{{if .Connections}}
<ul class="rows">
{{range $i, $c := .Connections}}<li class="row{{if not .Live}} row--waiting{{end}}">
<span class="tool-mark" aria-hidden="true">{{initial .Tool}}</span>
<span class="row__main"><span class="row__title">{{.Tool}}</span>
<span class="row__meta">{{if .Live}}Funcionando &middot; usada {{relativeSince .LastUsed}}{{else if .Configured}}Configurada &middot; ainda não se conectou; reinicie a ferramenta{{else}}Usada {{relativeSince .LastUsed}}{{end}}{{with .Version}} &middot; <span class="mono">{{.}}</span>{{end}}</span></span>
{{if .Live}}<span class="pill pill--ok">Conectada</span>{{else}}<span class="pill pill--warn">Aguardando</span>{{end}}
<a class="btn btn--danger btn--small" href="#desconectar-{{$i}}">Desconectar</a>
</li>{{end}}
</ul>
<p class="muted">Todas usam a mesma sessão do WhatsApp neste computador. Desconectar uma não mexe nas outras.</p>
{{else}}
<div class="empty"><p class="empty__title">Nenhuma ferramenta de IA conectada ainda</p>
<p class="muted">Clique no botão acima, escolha onde você vai usar e siga o passo a passo. Leva menos de um minuto.</p></div>
{{end}}
</div></section>

{{if .LiveCount}}
<section class="card">
<div class="card__head"><h2>Experimente pedir</h2></div>
<div class="card__body">
<p class="muted">Escreva isso no chat da sua ferramenta de IA. Ler e procurar é seguro: mandar mensagem só acontece quando você pede.</p>
<ul class="prompts">
{{range .Prompts}}<li class="prompt"><div class="snippet"><pre data-copy><code>{{.}}</code></pre></div></li>{{end}}
</ul>
</div></section>
{{end}}

<section class="card">
<div class="card__head"><h2>Ver o passo a passo de novo</h2></div>
<div class="card__body">
<details class="disclose">
<summary>Mostrar como configurar uma ferramenta de IA</summary>
{{template "clientTabs" .Setup}}
<p class="muted">O endereço deste MCP é <code>{{.Endpoint}}</code>. Ele só responde a programas deste computador.</p>
</details>
</div></section>

{{template "overlays" .}}
{{range $i, $c := .Connections}}
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
{{template "foot"}}{{end}}

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
{{template "foot"}}{{end}}

{{define "estado"}}{{template "head" .}}{{template "nav" .}}
<h1>Estado</h1>
<p class="lead">Se o seu WhatsApp está conectado e recebendo mensagens neste computador.</p>

<section class="card">
<div class="card__head"><h2>WhatsApp</h2>{{template "syncpill" .}}</div>
<div class="card__body stack">
{{if eq .Health.Status "ok"}}<p class="alert alert--ok">Nenhum problema detectado. As mensagens estão sendo recebidas e guardadas.</p>
{{else}}<ul class="problems">{{range .Checks}}{{if ne .Status "ok"}}<li{{if eq .Status "warn"}} class="problems__warn"{{end}}><strong>{{.Title}}:</strong> {{.Text}}</li>{{end}}{{end}}</ul>{{end}}
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
<span class="pill pill--{{if eq .Status "ok"}}ok{{else if eq .Status "warn"}}warn{{else}}off{{end}}">{{if eq .Status "ok"}}Ok{{else if eq .Status "warn"}}Atenção{{else}}Problema{{end}}</span>
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

{{template "foot"}}{{end}}

{{define "instalacao"}}{{template "head" .}}
<header class="masthead"><a class="brand" href="/instalacao">{{template "brandmark"}}</a>
<div class="masthead__tools">{{template "themeswitch"}}</div></header>
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
<div class="card__head"><h2>Conecte ao Claude</h2></div>
<div class="card__body">
<p class="muted">Onde você vai usar o seu WhatsApp? Escolha e siga o passo a passo.</p>
{{template "clientTabs" .Setup}}
<p class="busy" role="status"><span class="spinner" aria-hidden="true"></span>Esperando a ferramenta se conectar. Esta tela avisa sozinha.</p>
</div></section>
<form method="post" action="/instalacao/avancar"><input type="hidden" name="to" value="done">
<div class="wizard__escape"><button class="btn btn--quiet" type="submit">Pular por enquanto</button></div></form>
{{end}}
{{template "autostartcheck" .}}
{{end}}
</div>
{{template "foot"}}{{end}}

{{define "transcricao"}}{{template "head" .}}{{template "nav" .}}
<h1>Transcrição de áudios</h1>
<p class="lead">A sua ferramenta de IA lê os áudios que você recebe no WhatsApp como texto.</p>
{{with .OK}}<p class="alert alert--ok" role="status">{{.}}</p>{{end}}

<section class="card card--accent" data-asr>
<div class="card__head"><h2>Transcrição neste computador</h2>{{if .Local.Ready}}<span class="pill pill--ok">Ativa</span>{{else}}<span class="pill pill--off">Não instalada</span>{{end}}</div>
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
</div></section>

<section class="card">
<div class="card__head"><h2>Como usar</h2></div>
<div class="card__body">
<p class="muted">Peça à sua ferramenta de IA algo como a mensagem abaixo. Os áudios já transcritos vêm junto das mensagens; os outros ela transcreve na hora, confere com a conversa e guarda a versão corrigida.</p>
<div class="snippet"><div class="snippet__head"><span class="snippet__title">Exemplo</span></div>
<pre class="plain" data-copy><code>Transcreva os áudios que recebi hoje no WhatsApp.</code></pre></div>
<p class="muted">Cada áudio é transcrito uma vez: pedir de novo devolve o texto guardado.</p>
</div></section>
{{template "foot"}}{{end}}

{{define "documentacao"}}{{template "head" .}}{{template "nav" .}}
<h1>O que o MCP sabe fazer</h1>
<p class="lead">{{.Count}} ferramentas, lidas do próprio servidor. Esta página não é uma cópia mantida à mão: ela descreve exatamente a superfície que o MCP publica, então só fica errada se o servidor estiver.</p>
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
{{template "foot"}}{{end}}

{{define "receitas"}}{{template "head" .}}{{template "nav" .}}
<h1>Receitas</h1>
<p class="lead">Nenhuma destas precisa de código novo. O WhatsApp MCP só responde pelo WhatsApp quando perguntado: esperar a hora, vigiar um termo e montar o relatório são trabalho da sua ferramenta de IA, escrito como instrução. Cada receita é um texto para colar.</p>
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
{{template "foot"}}{{end}}

{{define "configuracoes"}}{{template "head" .}}{{template "nav" .}}
<h1>Configurações</h1>
<p class="lead">Como o WhatsApp MCP roda neste computador.</p>
{{with .OK}}<p class="alert alert--ok" role="status">{{.}}</p>{{end}}

{{if .PortChanged}}
<section class="card card--accent">
<div class="card__head"><h2>Avise as suas ferramentas de IA</h2></div>
<div class="card__body stack">
<ul class="rows">
<li class="row"><span class="row__main"><span class="row__title">Claude Desktop e Cowork</span><span class="row__meta">Nada a fazer: ele encontra a porta nova sozinho. Uma conversa já aberta reconecta no próximo uso.</span></span><span class="pill pill--ok">Automático</span></li>
<li class="row"><span class="row__main"><span class="row__title">Claude Code</span><span class="row__meta">{{if .Setup.Code.Configured}}Já usa o endereço novo.{{else if .Setup.Code.Found}}Ele guarda o endereço com a porta: atualize com um clique.{{else}}O comando claude não foi encontrado neste computador.{{end}}</span></span>
{{if and .Setup.Code.Found (not .Setup.Code.Configured)}}<button class="btn btn--small" type="button" data-add-client="claude-code">Atualizar o Claude Code agora</button>{{end}}</li>
</ul>
<p class="busy" data-client-note="claude-code" role="status" hidden></p>
<p class="muted">Outras ferramentas (Cursor, Windsurf, Codex…) guardam o endereço antigo e precisam ser configuradas de novo. Mande isto no chat delas:</p>
<div class="snippet"><pre class="plain" data-copy><code>{{.Setup.AgentPrompt}}</code></pre></div>
</div></section>
{{end}}

<section class="card" id="porta">
<div class="card__head"><h2>Porta do MCP</h2></div>
<div class="card__body stack">
<p class="muted">As ferramentas de IA falam com o WhatsApp MCP por este endereço, que só responde a programas deste computador. Troque a porta se outro programa já usa esta, ou se você quiser uma porta específica.</p>
<div class="snippet"><div class="snippet__head"><span class="snippet__title">Endereço</span></div><pre data-copy><code>{{.Setup.Endpoint}}</code></pre></div>
{{if .Settings.PortLocked}}<p class="note">A porta está definida pela variável de ambiente <code>WHATSAPP_MCP_PORT</code> e não pode ser trocada aqui.</p>
{{else}}<form method="post" action="/configuracoes/porta" data-busy="Trocando…">
<label class="field" for="port"><span class="field__label">Porta</span><span class="field__hint">Um número de 1024 a 65535. Trocar a porta desconecta as ferramentas de IA do MCP. <span class="tip" tabindex="0" aria-describedby="tip-porta"><span aria-hidden="true">?</span><span class="tip__body" role="tooltip" id="tip-porta">O WhatsApp continua conectado. As ferramentas de IA ligadas por MCP, como Claude, Codex e outras, perdem a conexão e precisam do endereço novo. Depois de salvar, o app mostra como atualizar cada uma.</span></span></span></label>
<div class="actions"><input id="port" class="input--short" type="number" name="port" min="1024" max="65535" value="{{.Port}}" required><button class="btn" type="submit">Salvar</button></div>
</form>{{end}}
</div></section>

<section class="card">
<div class="card__head"><h2>Ao ligar e ao fechar</h2></div>
<div class="card__body stack">
<label class="check"><input type="checkbox" data-setting="autostart"{{if .Settings.Autostart}} checked{{end}}><span><strong>Abrir o WhatsApp MCP quando o computador ligar</strong><span class="check__hint">Ele abre só na {{tray}}, sem janela, e as ferramentas de IA já encontram o WhatsApp.</span></span></label>
{{if .Settings.CanHide}}<label class="check"><input type="checkbox" data-setting="close_to_tray"{{if .Settings.CloseToTray}} checked{{end}}><span><strong>Fechar a janela mantém o app na {{tray}}</strong><span class="check__hint">Desmarcado, fechar a janela encerra o WhatsApp MCP, e as ferramentas de IA perdem o acesso até ele ser aberto de novo.</span></span></label>
{{else}}<p class="muted">Este sistema não mostra ícones na bandeja (no GNOME, isso pede a extensão AppIndicator). Por isso, fechar a janela só a minimiza.</p>{{end}}
<p class="busy" data-setting-note role="status" hidden></p>
</div></section>

<section class="card" id="atualizacoes" data-update>
<div class="card__head"><h2>Versão e atualizações</h2></div>
<div class="card__body stack">
<dl class="facts"><div class="fact"><dt>Versão instalada</dt><dd>{{.Settings.Version}}</dd></div><div class="fact"><dt>Mais recente</dt><dd data-update-latest>{{with .Update.Latest}}{{.}}{{else}}—{{end}}</dd></div></dl>
<p class="muted" data-update-text>O app procura uma versão nova uma vez por dia.</p>
<div class="actions"><button class="btn btn--ghost btn--small" type="button" data-update-check>Procurar atualização</button><button class="btn btn--small" type="button" data-update-install hidden>Instalar e reiniciar</button><a class="btn btn--ghost btn--small" data-update-page href="#" target="_blank" rel="noopener" hidden>Baixar a versão nova</a></div>
</div></section>

<section class="card">
<div class="card__head"><h2>Dados</h2></div>
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
{{template "foot"}}{{end}}

{{define "apagado"}}{{template "head" .}}
<div class="wizard-shell" style="max-width:520px">
<section class="card">
<div class="card__head"><h2>Tudo apagado</h2></div>
<div class="card__body"><p class="muted">Este computador saiu dos dispositivos conectados do seu WhatsApp, e os dados foram apagados. O WhatsApp MCP vai fechar.</p></div>
</section></div></div></body></html>{{end}}`
