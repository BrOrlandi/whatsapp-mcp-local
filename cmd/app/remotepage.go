package main

// remotePageSource is the app's page for remote mode, /__app/remoto: pairing
// with a server, and how the two connections stand.
const remotePageSource = `<!doctype html>
<html lang="{{.Lang}}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{t "Servidor remoto"}} · WhatsApp MCP</title><style>{{.CSS}}</style>{{if and .Remote (ne .Link.State "connected")}}<meta http-equiv="refresh" content="5">{{end}}</head>
<body><div class="shell"><header class="masthead"><a class="brand" href="/"><span class="brand__name">WhatsApp MCP</span></a></header>
<main class="page">
<p><a class="flow__back" href="/configuracoes#remoto">← {{t "Voltar ao painel"}}</a></p>
<h1>{{t "Servidor remoto"}}</h1>
{{with .Error}}<p class="alert" role="alert">{{.}}</p>{{end}}{{with .OK}}<p class="alert alert--ok" role="status">{{.}}</p>{{end}}
{{if .Remote}}
<section class="card"><div class="card__head"><h2 class="card__title">{{t "Modo remoto"}}</h2><span class="pill pill--accent pill--plain">{{.Server}}</span></div>
<p>{{t "Este app não roda o WhatsApp neste computador: ele mostra e controla o WhatsApp MCP do servidor. As ferramentas de IA deste computador continuam no mesmo endereço, e o app leva os pedidos até o servidor."}}</p>
<ul class="list">
<li class="list__row"><span><strong>{{t "App ↔ servidor"}}</strong><br><span class="note">{{t "a conexão cifrada deste app com o servidor, por HTTPS"}}</span></span><span class="pill pill--{{.LinkTone}}" data-remote-link>{{.LinkLabel}}</span></li>
{{with .Link.LastError}}<li class="list__row"><span class="note">{{t "Último erro:"}} {{.}}</span></li>{{end}}
<li class="list__row"><span><strong>{{t "Servidor ↔ WhatsApp"}}</strong><br><span class="note">{{t "o WhatsApp conectado no servidor"}}{{if .WhatsApp}}{{with .WhatsApp.Name}} · {{.}}{{end}}{{with .WhatsApp.Phone}} · {{.}}{{end}}{{end}}</span></span><span class="pill pill--{{.WATone}}">{{.WALabel}}</span></li>
</ul>
<p class="note">{{t "Servidor:"}} <code>{{.Server}}</code> · {{t "chave do servidor"}} <code>{{.Finger}}</code> · {{t "este app"}} <code>{{.Device}}</code>{{if not .Checked.IsZero}} · {{tf "conferido às %s" (moment .Checked)}}{{end}}</p>
<div class="actions">
<form method="post" action="/__app/remoto/tentar"><button class="btn" type="submit">{{t "Tentar de novo agora"}}</button></form>
<form method="post" action="/__app/remoto/girar"><button class="btn btn--quiet" type="submit">{{t "Trocar a chave deste app"}}</button></form>
</div></section>
<section class="card"><div class="card__head"><h2 class="card__title">{{t "Voltar ao modo local"}}</h2></div>
<p>{{t "O app volta a rodar o WhatsApp neste computador, com os dados que estão aqui. O servidor continua conectado: para não ter dois aparelhos com a mesma sessão, desconecte o WhatsApp no servidor antes de conectar este computador de novo."}}</p>
<form method="post" action="/__app/remoto/local"><label class="check"><input type="checkbox" name="server_disconnected" value="1"><span>{{t "Já desconectei o WhatsApp no servidor (ou este computador tem outro número)"}}</span></label>
<label class="check"><input type="checkbox" name="forget" value="1"><span>{{t "Apagar a chave deste app do Keychain (para usar o servidor de novo, será preciso parear)"}}</span></label>
<div class="actions"><button class="btn btn--quiet" type="submit">{{t "Voltar ao modo local"}}</button></div></form></section>
{{else}}
<section class="card"><div class="card__head"><h2 class="card__title">{{t "Usar um servidor"}}</h2></div>
<p>{{t "No servidor, gere um código de pareamento: whatsapp-mcp remote code, ou Configurações › Acesso remoto no painel dele. O código vale uma vez, por 10 minutos."}}</p>
<p class="note">{{t "A conexão é só por HTTPS, com o certificado conferido, e cifrada de ponta a ponta entre este app e o servidor: um proxy no caminho não lê o conteúdo. A chave deste app fica no Keychain deste Mac."}}</p>
<form method="post" action="/__app/remoto/parear" autocomplete="off">
<label class="field" for="server"><span class="field__label">{{t "Endereço do servidor"}}</span><input id="server" name="server" type="url" required pattern="https://.+" placeholder="{{t "https://whatsapp.exemplo.com"}}" title="{{t "Só endereços https://"}}"></label>
<label class="field" for="code"><span class="field__label">{{t "Código de pareamento"}}</span><input id="code" name="code" type="text" required placeholder="{{t "wamcp1-…"}}" spellcheck="false"></label>
<label class="field" for="name"><span class="field__label">{{t "Nome deste app no servidor"}}</span><input id="name" name="name" type="text" value="{{.Hostname}}"></label>
<p class="note">{{t "Ao conectar, o app para de rodar o WhatsApp neste computador. Os dados daqui ficam onde estão."}}</p>
<label class="check"><input type="checkbox" name="set_aside" value="1"><span>{{t "Levei os dados deste computador para o servidor: guardar a sessão do WhatsApp daqui numa pasta à parte, para este computador nunca abri-la de novo"}}</span></label>
<div class="actions"><button class="btn" type="submit">{{t "Conectar ao servidor"}}</button></div></form></section>
{{end}}
</main></div></body></html>`
