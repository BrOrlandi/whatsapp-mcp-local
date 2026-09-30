package panel

// pageSource holds every template the panel renders. The shell, the masthead,
// the tabs, the cards, the dialogs and the colophon are v1's markup; what
// differs is what a local daemon has instead of a server: one WhatsApp account
// rather than instances, no login, and a chat preview to confirm the sync.
const pageSource = `
{{define "head"}}<!doctype html>
<html lang="pt-BR"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><link rel="icon" href="/favicon.svg" type="image/svg+xml"><link rel="alternate icon" href="/favicon.ico" sizes="16x16 32x32 48x48"><link rel="apple-touch-icon" href="/apple-touch-icon.png"><script src="/assets/theme.js"></script><title>{{.Title}} · WhatsApp MCP</title><style>{{css}}</style></head><body><div class="shell">{{end}}

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

{{define "brandmark"}}<span class="brand__mark">{{logo}}</span><span class="brand__name">WhatsApp MCP<span class="brand__tagline">Painel de controle · neste computador</span></span>{{end}}

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
</nav>
{{with .Error}}<p class="alert" role="alert">{{.}}</p>{{end}}
{{end}}

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
<div class="dialog__head"><h2 id="nova-conexao-titulo">Conectar uma ferramenta de IA</h2><a class="dialog__close" href="#" aria-label="Fechar">&times;</a></div>
<div class="dialog__body">{{template "clientTabs" .Setup}}</div>
</div></div>
{{end}}

{{define "chatpreview"}}
<div class="preview-grid" data-chats>
<section class="card">
<div class="card__head"><h2>Conversas recentes</h2><span class="pill pill--off pill--plain" data-chats-count>…</span></div>
<div class="card__body">
<p class="muted">As 10 conversas mais recentes que este computador recebeu. Toque numa para ver as mensagens.</p>
<ul class="chats" data-chat-list><li><div class="skeleton"></div></li><li><div class="skeleton"></div></li><li><div class="skeleton"></div></li></ul>
</div></section>
<section class="card">
<div class="card__head"><h2>Últimas mensagens recebidas</h2></div>
<div class="card__body">
<p class="muted">O que chegou por último, em qualquer conversa.</p>
<ul class="inbox" data-inbox><li><div class="skeleton"></div></li><li><div class="skeleton"></div></li></ul>
</div></section>
</div>
<p class="note" style="margin-top:-2px">Compare com o seu celular. Se alguma conversa ou mensagem recente estiver faltando, abra a conversa aqui e toque em <strong>Buscar mensagens mais antigas</strong>, ou use o botão de histórico abaixo.</p>

<div class="overlay" id="conversa" role="dialog" aria-modal="true" aria-labelledby="conversa-titulo">
<div class="dialog dialog--wide">
<div class="dialog__head"><h2 id="conversa-titulo" data-thread-title>Conversa</h2><a class="dialog__close" href="#" aria-label="Fechar">&times;</a></div>
<div class="dialog__body stack">
<div class="thread" data-thread></div>
<p class="muted" data-thread-meta></p>
<div class="actions"><button class="btn btn--ghost btn--small" type="button" data-history-chat>Buscar mensagens mais antigas desta conversa</button></div>
<p class="busy" data-history-note role="status" hidden></p>
</div></div></div>
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
<p class="lead">Aqui você vê se está tudo funcionando e liga o seu WhatsApp a um assistente de inteligência artificial — o Claude, o Cursor, o que você usar.</p>

<section class="overview" data-wait-client="{{if .LiveCount}}false{{else}}true{{end}}">
<div class="overview__item overview__item--{{if eq .SyncTone "ok"}}ok{{else}}wait{{end}}">
<span class="overview__icon" aria-hidden="true">{{if eq .SyncTone "ok"}}&#10003;{{else}}!{{end}}</span>
<div class="overview__body">
<p class="overview__title">{{if eq .SyncTone "ok"}}WhatsApp conectado{{else}}{{.SyncLabel}}{{end}}</p>
<p class="overview__detail">{{if .Phone}}<strong class="overview__phone">{{.Phone}}</strong>{{end}}{{.Name}}</p>
</div></div>

{{if .LiveCount}}
<div class="overview__item overview__item--ok">
<span class="overview__icon" aria-hidden="true">&#10003;</span>
<div class="overview__body">
<p class="overview__title">{{plural .LiveCount "ferramenta de IA conectada" "ferramentas de IA conectadas"}}</p>
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

<div class="hero"><a class="btn btn--big" href="#nova-conexao">Conectar uma ferramenta de IA</a></div>

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
<div class="card__head"><h2>Conta</h2><span class="pill pill--{{.SyncTone}}">{{.SyncLabel}}</span></div>
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
<h1>Estado do serviço</h1>
<p class="lead">O mesmo retrato que a ferramenta <code>health</code> do MCP e o endereço <code>/health</code> reportam.</p>

<section class="card">
<div class="card__head"><h2>WhatsApp</h2><span class="pill pill--{{.SyncTone}}">{{.SyncLabel}}</span></div>
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

<section class="card">
<div class="card__head"><h2>Sincronização</h2></div>
<div class="card__body">
<dl class="facts">
<div class="fact"><dt>Estado</dt><dd>{{.SyncLabel}}<span class="fact__detail">desde {{moment .Sync.Since}}</span></dd></div>
<div class="fact"><dt>Reinícios inesperados</dt><dd>{{.Sync.Restarts}}</dd></div>
<div class="fact"><dt>Endereço do MCP</dt><dd class="mono" style="font-size:.85rem">{{.Endpoint}}</dd></div>
</dl>
{{with .Sync.LastError}}<p class="muted">Último erro: <code>{{.}}</code></p>{{end}}
</div></section>

<section class="card">
<div class="card__head"><h2>Verificação externa</h2></div>
<div class="card__body">
<div class="actions"><a class="btn btn--ghost btn--small" href="/health">/health</a><a class="btn btn--ghost btn--small" href="/healthz">/healthz</a></div>
<p class="muted">O <code>/health</code> responde 503 quando alguma verificação falha, para quem quiser monitorar.</p>
</div></section>
{{template "foot"}}{{end}}

{{define "instalacao"}}{{template "head" .}}
{{if eq .Step 1}}<header class="masthead"><a class="brand" href="/">{{template "brandmark"}}</a>
<div class="masthead__tools">{{template "themeswitch"}}</div></header>
{{else}}{{template "nav" .}}{{end}}
<div class="wizard-shell"{{if ge .Step 2}} style="max-width:{{if eq .Step 2}}880px{{else}}620px{{end}}"{{end}}>
<ol class="wizard" aria-label="Etapas da instalação"{{if ge .Step 2}} style="max-width:520px;margin-left:auto;margin-right:auto"{{end}}>
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
<ol class="guide">
<li>No celular, abra o WhatsApp e vá em <strong>Dispositivos conectados</strong>.</li>
<li>Toque em <strong>Conectar um dispositivo</strong> e depois em <strong>Conectar com número de telefone</strong>.</li>
<li>Digite o código abaixo.</li>
</ol>
<p class="paircode" data-code></p>
<div class="actions" style="justify-content:center"><button class="linkbtn" type="button" data-pair-back>Voltar para o QR code</button></div>
</div>
<div data-pair-sync hidden>
<p class="lead"><span class="progressline"><span class="spinner" aria-hidden="true"></span><strong>Conectado! Trazendo as suas conversas…</strong></span></p>
<p class="muted" data-sync-progress>O celular está mandando o histórico recente. Mantenha ele com internet; em instantes você segue.</p>
</div>
<div data-pair-error hidden>
<p class="alert" role="alert" data-error-text></p>
<div class="actions"><button class="btn" type="button" data-pair-retry>Tentar de novo</button></div>
</div>
</div></section>
<p class="muted" style="text-align:center">Tudo roda neste computador: as mensagens ficam aqui, e nada passa por servidor de terceiros.</p>
{{end}}

{{if eq .Step 2}}
<section class="card card--accent">
<div class="card__head"><h2>WhatsApp conectado</h2><span class="pill pill--ok">Conectado</span></div>
<div class="card__body">
<div class="hello"><span class="avatar" style="background:#128c7e">{{initial .Name}}</span><div><p class="hello__name">{{with .Name}}Olá, {{.}}!{{else}}Tudo certo!{{end}}</p><p class="hello__phone">{{.Phone}}</p></div></div>
<p class="muted" style="margin:0">Confira abaixo se as suas conversas mais recentes são as mesmas do celular. Se estiverem, siga para o próximo passo.</p>
{{with .Pairing}}{{if eq .State "syncing"}}<p class="busy" role="status" data-sync-note><span class="spinner" aria-hidden="true"></span>O celular ainda está mandando o histórico (<span data-sync-count>{{count .Synced}}</span> mensagens até agora). A lista se atualiza sozinha.</p>{{end}}{{end}}
</div></section>
{{template "chatpreview" .}}
<form method="post" action="/instalacao/avancar"><input type="hidden" name="to" value="claude">
<div class="actions" style="justify-content:center;margin-top:18px"><button class="btn btn--big" type="submit">Está certo, conectar ao Claude →</button></div></form>
{{end}}

{{if eq .Step 3}}
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
{{end}}
</div>
{{template "foot"}}{{end}}

{{define "transcricao"}}{{template "head" .}}{{template "nav" .}}
<h1>Transcrição de áudios</h1>
<p class="lead">Com uma chave da OpenAI, a sua ferramenta de IA transcreve os áudios que você recebe no WhatsApp usando o Whisper.</p>
{{with .OK}}<p class="alert alert--ok" role="status">{{.}}</p>{{end}}

{{if not .KeyHint}}
<section class="card card--accent">
<div class="card__head"><h2>Como ativar</h2><span class="pill pill--off">Não configurada</span></div>
<div class="card__body stack">
<p class="muted">Leva uns cinco minutos. A chave é da sua conta na OpenAI: os áudios são cobrados nela, e só nela. Se o seu computador for um Mac com Apple Silicon, a sua ferramenta de IA também consegue transcrever de graça, no próprio computador, sem chave nenhuma — é só pedir.</p>
<ol class="guide">
<li><strong>Crie uma conta na plataforma da OpenAI.</strong> É a plataforma de desenvolvedores, separada do ChatGPT: uma assinatura do ChatGPT Plus não inclui créditos para a API.
<div class="actions" style="margin-top:8px"><a class="btn btn--ghost btn--small" href="https://platform.openai.com/signup" rel="noopener noreferrer" target="_blank">Criar conta na OpenAI ↗</a></div></li>
<li><strong>Adicione créditos.</strong> Em <em>Billing</em>, cadastre um cartão e compre créditos: o mínimo é US$&nbsp;5. O Whisper custa US$&nbsp;0,006 por minuto de áudio, então US$&nbsp;5 dão para cerca de 800 minutos.
<div class="actions" style="margin-top:8px"><a class="btn btn--ghost btn--small" href="https://platform.openai.com/settings/organization/billing/overview" rel="noopener noreferrer" target="_blank">Adicionar créditos ↗</a></div></li>
<li><strong>Crie a chave de API.</strong> Em <em>API keys</em>, clique em <em>Create new secret key</em>, dê um nome como <code>WhatsApp MCP</code> e deixe as permissões em <em>All</em>. Copie a chave na hora: a OpenAI só mostra ela uma vez.
<div class="actions" style="margin-top:8px"><a class="btn btn--ghost btn--small" href="https://platform.openai.com/api-keys" rel="noopener noreferrer" target="_blank">Criar chave de API ↗</a></div></li>
<li><strong>Cole a chave aqui embaixo e salve.</strong> Ela é conferida com a OpenAI antes de ser guardada.</li>
</ol>
{{template "transcricaoform" .}}
</div></section>
{{else}}
<section class="card card--accent">
<div class="card__head"><h2>Chave da OpenAI</h2><span class="pill pill--ok">Configurada</span></div>
<div class="card__body stack">
<dl class="facts"><div class="fact"><dt>Chave salva</dt><dd class="mono">{{.KeyHint}}</dd></div></dl>
<div>
<p class="field__label">Na sua conta da OpenAI</p>
<div class="actions" style="margin-top:8px">
<a class="btn btn--ghost btn--small" href="https://platform.openai.com/usage" rel="noopener noreferrer" target="_blank">Uso e custos ↗</a>
<a class="btn btn--ghost btn--small" href="https://platform.openai.com/settings/organization/billing/overview" rel="noopener noreferrer" target="_blank">Créditos ↗</a>
<a class="btn btn--ghost btn--small" href="https://platform.openai.com/settings/organization/limits" rel="noopener noreferrer" target="_blank">Limite de gastos ↗</a>
<a class="btn btn--ghost btn--small" href="https://platform.openai.com/api-keys" rel="noopener noreferrer" target="_blank">Revisar chaves ↗</a>
</div>
<p class="muted" style="margin-top:8px">Um limite mensal de gastos em <em>Limits</em> evita surpresa na fatura. Se revogar a chave lá, cadastre uma nova aqui.</p>
</div>
<details class="step">
<summary class="step__summary"><span class="step__title">Trocar ou remover a chave</span></summary>
<div class="stack">
{{template "transcricaoform" .}}
<form method="post" action="/transcricao/remover">
<div class="actions"><button class="btn btn--danger btn--small" type="submit">Remover chave</button></div>
</form>
</div>
</details>
</div></section>
{{end}}
<section class="card">
<div class="card__head"><h2>Como usar</h2></div>
<div class="card__body">
<p class="muted">Peça à sua ferramenta de IA algo como a mensagem abaixo. Ela encontra os áudios com as ferramentas de leitura e usa <code>transcribe_audio</code> em cada um.</p>
<div class="snippet"><div class="snippet__head"><span class="snippet__title">Exemplo</span></div>
<pre class="plain" data-copy><code>Transcreva os áudios que recebi hoje no WhatsApp.</code></pre></div>
<p class="muted">O áudio é enviado para a OpenAI e cobrado na conta desta chave (<a href="https://openai.com/api/pricing/" rel="noopener noreferrer" target="_blank">US$ 0,006 por minuto</a>). Cada áudio é transcrito uma vez: pedir de novo devolve o texto guardado, sem nova cobrança. A chave também pode ser salva pela própria ferramenta de IA com <code>set_transcription_key</code>, mas por aqui ela não passa pela conversa.</p>
</div></section>
{{template "foot"}}{{end}}

{{define "transcricaoform"}}<form method="post" action="/transcricao" data-busy="Verificando…">
<label class="field" for="api_key"><span class="field__label">{{if .KeyHint}}Nova chave{{else}}Chave de API{{end}}</span>
<span class="field__hint">Começa com <code>sk-</code>. Depois de salva, ela nunca mais aparece inteira.</span></label>
<input id="api_key" type="password" name="api_key" required placeholder="sk-…" autocomplete="off" spellcheck="false">
<div class="actions"><button class="btn" type="submit">Salvar chave</button></div>
</form>{{end}}

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
<p class="lead">Nenhuma destas precisa de código novo. O gateway só responde pelo WhatsApp quando perguntado — esperar a hora, vigiar um termo, montar o relatório, tudo isso é trabalho do assistente, escrito como instrução. Cada receita é um prompt para colar.</p>
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
{{template "foot"}}{{end}}`
