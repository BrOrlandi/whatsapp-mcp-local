package main

// remotePageSource is the app's page for remote mode, /__app/remoto: pairing
// with a server, and how the two connections stand.
const remotePageSource = `<!doctype html>
<html lang="{{.Lang}}"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">
<title>{{t "Servidor remoto"}} · WhatsApp MCP</title><style>{{.CSS}}</style></head>
<body><div class="shell"><header class="masthead"><a class="brand" href="/"><span class="brand__name">WhatsApp MCP</span></a></header>
<main class="page">
<p><a class="flow__back" href="/configuracoes#remoto">← {{t "Voltar ao painel"}}</a></p>
<h1 class="flow__title">{{t "Servidor remoto"}}</h1>
{{with .Error}}<p class="alert" role="alert">{{.}}</p>{{end}}{{with .OK}}<p class="alert alert--ok" role="status">{{.}}</p>{{end}}
{{if .Remote}}
<section class="card"><div class="card__head"><h2 class="card__title">{{t "Modo remoto"}}</h2><span class="pill pill--ok pill--plain">{{.Server}}</span></div>
<div class="card__body stack">
<p>{{t "Este app não roda o WhatsApp neste computador: ele mostra e controla o WhatsApp MCP do servidor. As ferramentas de IA deste computador continuam no mesmo endereço, e o app leva os pedidos até o servidor."}}</p>
<ul class="rows">
<li class="row"><span class="row__main"><span class="row__title">{{t "App ↔ servidor"}}</span><span class="row__meta">{{t "a conexão cifrada deste app com o servidor, por HTTPS"}}{{with .Link.LastError}} <span class="tip" tabindex="0" aria-describedby="tip-erro"><span aria-hidden="true">?</span><span class="tip__body" role="tooltip" id="tip-erro">{{t "Último erro:"}} {{.}}</span></span>{{end}}</span></span><span class="pill pill--{{.LinkTone}}" data-remote-link>{{.LinkLabel}}</span></li>
<li class="row"><span class="row__main"><span class="row__title">{{t "Servidor ↔ WhatsApp"}}</span><span class="row__meta">{{t "o WhatsApp conectado no servidor"}}{{if .WhatsApp}}{{with .WhatsApp.Name}} · {{.}}{{end}}{{with .WhatsApp.Phone}} · {{.}}{{end}}{{end}}</span></span><span class="pill pill--{{.WATone}}">{{.WALabel}}</span></li>
</ul>
<p class="muted">{{t "Servidor:"}} <strong>{{.Server}}</strong>{{if not .Checked.IsZero}} · {{tf "conferido às %s" (moment .Checked)}}{{end}} <span class="tip" tabindex="0" aria-describedby="tip-chaves"><span aria-hidden="true">?</span><span class="tip__body" role="tooltip" id="tip-chaves">{{t "chave do servidor"}} {{.Finger}} · {{t "este app"}} {{.Device}}</span></span></p>
{{if not .AllOK}}<div class="actions"><form method="post" action="/__app/remoto/tentar"><button class="btn" type="submit">{{t "Verificar a conexão agora"}}</button></form></div>{{end}}</div></section>
<section class="card"><div class="card__head"><h2 class="card__title">{{t "Computadores que gerenciam o servidor"}}</h2></div>
<div class="card__body stack">
<p>{{t "Este app já está conectado e não precisa de código. Para outro computador também gerenciar o servidor, gere um código aqui e cole no app dele, em Configurações › Acesso remoto › Usar um servidor. O código vale uma vez, por 10 minutos."}}</p>
{{if .Devices}}<ul class="rows">{{range .Devices}}<li class="row"><span class="row__main"><span class="row__title">{{.Name}}{{if .This}} <span class="pill pill--accent pill--plain">{{t "este app"}}</span>{{end}}</span><span class="row__meta">{{if .LastSeen}}{{tf "visto em %s" (moment .LastSeen)}}{{else}}{{t "nunca conectado"}}{{end}}</span></span>
{{if not .This}}<form method="post" action="/__app/remoto/revogar"><input type="hidden" name="id" value="{{.ID}}"><button class="btn btn--quiet btn--small" type="submit">{{t "Revogar"}}</button></form>{{end}}</li>{{end}}</ul>{{end}}
{{with .Code}}<div class="stack"><p class="note">{{t "Copie agora: o código não aparece de novo."}}</p><p><code>{{.Code}}</code></p><p class="note">{{t "Impressão digital do servidor:"}} {{.Fingerprint}}</p></div>{{end}}
<div class="actions"><form method="post" action="/__app/remoto/codigo"><button class="btn btn--quiet" type="submit">{{t "Parear outro computador como gerenciador"}}</button></form>
<button class="btn btn--quiet" type="button" data-rotate-open>{{t "Trocar a chave deste app"}}</button>
</div>
</div></section>
<dialog class="dialog dialog--native" id="trocar-chave" aria-labelledby="trocar-chave-titulo" aria-describedby="trocar-chave-descricao">
<div class="dialog__head"><h2 id="trocar-chave-titulo">{{t "Trocar a chave deste app"}}</h2><button class="dialog__close" type="button" data-rotate-close aria-label="{{t "Fechar"}}">&times;</button></div>
<div class="dialog__body stack">
<p id="trocar-chave-descricao">{{t "A chave identifica este app no servidor. Ao trocá-la, o app se desconecta por um instante e tenta conectar de novo automaticamente. Pedidos das ferramentas de IA que passam por este app podem falhar nesse intervalo e precisar ser repetidos."}}</p>
<p>{{t "O WhatsApp no servidor e os outros computadores continuam funcionando. A chave anterior só deixa de valer depois que a conexão com a nova chave é estabelecida."}}</p>
<form method="post" action="/__app/remoto/girar" data-rotate-form><div class="actions"><button class="btn" type="submit">{{t "Trocar a chave deste app"}}</button><button class="btn btn--quiet" type="button" data-rotate-close>{{t "Cancelar"}}</button></div></form>
</div></dialog>
<section class="card"><div class="card__head"><h2 class="card__title">{{t "Chaves de agentes remotos"}}</h2></div>
<div class="card__body stack">
<p>{{t "Agentes fora deste computador usam o MCP do servidor com uma chave. Para criar uma, escolha o agente em Conectar ferramenta de IA › Agentes remotos."}}</p>
{{if .Keys}}<ul class="rows">{{range .Keys}}<li class="row keyrow"><span class="toolpick__icon">{{.Mark}}</span><span class="row__main"><span class="row__title">{{.Name}}</span><span class="row__meta">{{.Meta}}</span></span>
{{if .Used}}<span class="pill pill--ok">{{t "Em uso"}}</span>{{else}}<span class="pill pill--warn">{{t "Aguardando o primeiro uso"}}</span>{{end}}
<button class="btn btn--quiet btn--small" type="button" data-key-revoke="{{.ID}}">{{t "Revogar"}}</button></li>{{end}}</ul>
{{else}}<p class="muted">{{t "Nenhuma chave criada."}}</p>{{end}}
<div class="actions"><a class="btn btn--quiet" href="/conectar#agentes-remotos">{{t "Conectar um agente remoto"}}</a></div>
</div></section>
{{if .Keys}}<dialog class="dialog keydialog" id="revogar-chave" aria-labelledby="revogar-chave-titulo">
<div class="dialog__head"><h2 id="revogar-chave-titulo">{{t "Revogar a chave?"}}</h2><button class="dialog__close" type="button" data-key-close aria-label="{{t "Fechar"}}">&times;</button></div>
<div class="dialog__body stack">
<ul class="rows"><li class="row keyrow" data-key-preview></li></ul>
<p>{{t "O agente que usa esta chave perde o acesso ao WhatsApp na hora. Para conectá-lo de novo, será preciso criar outra chave."}}</p>
<form method="post" action="/__app/remoto/chaves/revogar"><input type="hidden" name="id" data-key-id>
<div class="actions"><button class="btn btn--danger" type="submit">{{t "Revogar a chave"}}</button><button class="btn btn--quiet" type="button" data-key-close>{{t "Cancelar"}}</button></div></form>
</div></dialog>
<script>
(function () {
  var dialog = document.getElementById("revogar-chave");
  var preview = dialog.querySelector("[data-key-preview]");
  document.querySelectorAll("[data-key-revoke]").forEach(function (button) {
    button.addEventListener("click", function () {
      var row = button.closest(".keyrow");
      preview.innerHTML = "";
      preview.appendChild(row.querySelector(".toolpick__icon").cloneNode(true));
      var main = preview.appendChild(row.querySelector(".row__main").cloneNode(true));
      var pill = main.appendChild(row.querySelector(".pill").cloneNode(true));
      pill.style.alignSelf = "flex-start";
      pill.style.marginTop = "6px";
      dialog.querySelector("[data-key-id]").value = button.getAttribute("data-key-revoke");
      dialog.showModal();
    });
  });
  dialog.querySelectorAll("[data-key-close]").forEach(function (button) {
    button.addEventListener("click", function () { dialog.close(); });
  });
})();
</script>{{end}}
<section class="card"><div class="card__head"><h2 class="card__title">{{t "Voltar ao modo local"}}</h2></div>
<div class="card__body stack">
<p>{{t "O app volta a rodar o WhatsApp neste computador, com os dados que estão aqui. O servidor continua conectado: para não ter dois aparelhos com a mesma sessão, desconecte o WhatsApp no servidor antes de conectar este computador de novo."}}</p>
<div class="actions"><button class="btn btn--ghost" type="button" data-local-open>{{t "Voltar ao modo local"}}</button></div></div></section>
<dialog class="dialog dialog--native" id="voltar-local" aria-labelledby="voltar-local-titulo">
<div class="dialog__head"><h2 id="voltar-local-titulo">{{t "Voltar ao modo local"}}</h2><button class="dialog__close" type="button" data-local-close aria-label="{{t "Fechar"}}">&times;</button></div>
<div class="dialog__body stack">
<p>{{t "O app volta a rodar o WhatsApp neste computador, com os dados que estão aqui. O servidor continua conectado: para não ter dois aparelhos com a mesma sessão, desconecte o WhatsApp no servidor antes de conectar este computador de novo."}}</p>
<form class="stack" method="post" action="/__app/remoto/local" data-local-form>
<label class="check"><input type="checkbox" name="server_disconnected" value="1" required><span>{{t "Já desconectei o WhatsApp no servidor (ou este computador tem outro número)"}}</span></label>
<label class="check"><input type="checkbox" name="forget" value="1" required><span>{{t "Apagar a chave deste app do Keychain (para usar o servidor de novo, será preciso parear)"}}</span></label>
<div class="actions"><button class="btn" type="submit" data-local-submit disabled>{{t "Voltar ao modo local"}}</button><button class="btn btn--quiet" type="button" data-local-close>{{t "Cancelar"}}</button></div>
</form></div></dialog>
{{else}}
<section class="card"><div class="card__head"><h2 class="card__title">{{t "Usar um servidor"}}</h2></div>
<div class="card__body stack">
<p>{{t "Gere um código de pareamento no servidor (whatsapp-mcp remote code) ou num computador que já o gerencia (Configurações › Acesso remoto › Gerenciar a conexão com o servidor). O código vale uma vez, por 10 minutos."}}</p>
<p class="muted">{{t "Ainda não tem o servidor?"}} <a href="https://github.com/BrOrlandi/whatsapp-mcp-local/blob/main/docs/modo-servidor.md" target="_blank" rel="noopener">{{t "Veja o passo a passo para montar o servidor"}}</a></p>
<p class="note">{{t "A conexão é só por HTTPS, com o certificado conferido, e cifrada de ponta a ponta entre este app e o servidor: um proxy no caminho não lê o conteúdo. A chave deste app fica no Keychain deste Mac."}}</p>
<form class="stack" method="post" action="/__app/remoto/parear" autocomplete="off">
<label class="field" for="server"><span class="field__label">{{t "Endereço do servidor"}}</span><input id="server" name="server" type="url" required pattern="https://.+" placeholder="{{t "https://whatsapp.exemplo.com"}}" title="{{t "Só endereços https://"}}"></label>
<label class="field" for="code"><span class="field__label">{{t "Código de pareamento"}}</span><input id="code" name="code" type="text" required placeholder="{{t "wamcp1-…"}}" spellcheck="false"></label>
<label class="field" for="name"><span class="field__label">{{t "Nome deste app no servidor"}}</span><input id="name" name="name" type="text" value="{{.Hostname}}"></label>
<p class="note">{{t "Ao conectar, o app para de rodar o WhatsApp neste computador. Os dados daqui ficam onde estão."}}</p>
<label class="check"><input type="checkbox" name="set_aside" value="1"><span>{{t "Levei os dados deste computador para o servidor: guardar a sessão do WhatsApp daqui numa pasta à parte, para este computador nunca abri-la de novo"}}</span></label>
<div class="actions"><button class="btn" type="submit">{{t "Conectar ao servidor"}}</button></div></form></div></section>
{{end}}
<p><a class="flow__back" href="/configuracoes#remoto">← {{t "Voltar ao painel"}}</a></p>
</main></div>
{{if .Remote}}<script>
(function () {
  var rotation = document.getElementById("trocar-chave");
  document.querySelector("[data-rotate-open]").addEventListener("click", function () {
    rotation.showModal();
  });
  rotation.querySelectorAll("[data-rotate-close]").forEach(function (button) {
    button.addEventListener("click", function () { rotation.close(); });
  });
  rotation.querySelector("[data-rotate-form]").addEventListener("submit", function () {
    rotation.querySelector("button[type=submit]").disabled = true;
  });
  var dialog = document.getElementById("voltar-local");
  var form = dialog.querySelector("[data-local-form]");
  var submit = dialog.querySelector("[data-local-submit]");
  function update() { submit.disabled = !form.checkValidity(); }
  document.querySelector("[data-local-open]").addEventListener("click", function () {
    form.reset();
    update();
    dialog.showModal();
  });
  dialog.querySelectorAll("[data-local-close]").forEach(function (button) {
    button.addEventListener("click", function () { dialog.close(); });
  });
  form.addEventListener("change", update);
  form.addEventListener("submit", function (event) {
    if (!form.checkValidity()) { event.preventDefault(); return; }
    submit.disabled = true;
  });
  {{if not .AllOK}}setInterval(function () {
    if (!dialog.open && !rotation.open) window.location.reload();
  }, 5000);{{end}}
})();
</script>{{end}}</body></html>`
