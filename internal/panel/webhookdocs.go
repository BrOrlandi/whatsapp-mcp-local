package panel

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/webhook"
)

// The webhook documentation page: what reaches a webhook, event by event.
// Its examples are marshalled from the very types the deliveries are, so the
// page cannot drift from what a script receives.

type webhookExample struct {
	ID    string
	Title string
	Note  string
	JSON  string
}

type webhookExampleGroup struct {
	ID       string
	Title    string
	Intro    string
	Examples []webhookExample
}

func exampleJSON(ev webhook.Event) string {
	body, _ := json.MarshalIndent(ev, "", "  ")
	return string(body)
}

// webhookExamples are deliveries as a script receives them, with made-up
// people and ids.
func webhookExamples() []webhookExampleGroup {
	at := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	sent := at.Add(time.Second)
	const (
		maria   = "5511912345678@s.whatsapp.net"
		joao    = "5511987654321@s.whatsapp.net"
		familia = "120363000000000001@g.us"
	)
	direct := func(id, text string) *webhook.Message {
		return &webhook.Message{ID: id, ChatJID: maria, ChatName: "Maria Silva", Timestamp: at, SenderJID: maria, SenderName: "Maria Silva", Text: text}
	}
	inGroup := func(id, text string) *webhook.Message {
		return &webhook.Message{ID: id, ChatJID: familia, ChatName: "Família", Group: true, Timestamp: at, SenderJID: joao, SenderName: "João", Text: text}
	}
	event := func(kind, delivery string, m *webhook.Message) string {
		return exampleJSON(webhook.Event{Kind: kind, ID: delivery, At: sent, Message: m})
	}

	reply := inGroup("3EB0B2C4D6E8F0A1B3C5", "Pode deixar, eu levo o bolo")
	reply.ReplyTo = &webhook.Reference{ID: "3EB0A1B2C3D4E5F60718", SenderJID: maria, Text: "Quem leva o bolo no domingo?"}
	photo := direct("3EB0C1D2E3F4A5B6C7D8", "Olha a vista daqui")
	photo.Media = &webhook.Media{Type: "image", MimeType: "image/jpeg", Bytes: 184320, Caption: "Olha a vista daqui"}
	document := direct("3EB0D1E2F3A4B5C6D7E8", "")
	document.Media = &webhook.Media{Type: "document", MimeType: "application/pdf", Filename: "boleto-outubro.pdf", Bytes: 92416}
	voice := direct("3EB0E1F2A3B4C5D6E7F8", "")
	voice.Media = &webhook.Media{Type: "audio", MimeType: "audio/ogg; codecs=opus", Bytes: 23552}
	place := direct("3EB0F1A2B3C4D5E6F7A8", "")
	place.Location = map[string]any{"latitude": -23.561684, "longitude": -46.655981, "name": "MASP", "address": "Av. Paulista, 1578", "live": false}
	poll := inGroup("3EB0A2B3C4D5E6F7A8B9", "")
	poll.Poll = map[string]any{"question": "Churrasco no sábado?", "options": []string{"Sim", "Não", "Talvez"}, "max_answers": 1}
	edited := direct("3EB0A1B2C3D4E5F60718", "Chego às 19h30, não às 19h")
	edited.Edited = true
	revoked := direct("3EB0A1B2C3D4E5F60718", "")
	revoked.Revoked = true
	own := &webhook.Message{ID: "3EB0B1C2D3E4F5A6B7C8", ChatJID: maria, ChatName: "Maria Silva", Timestamp: at, FromMe: true, Text: "Combinado!"}
	reaction := direct("3EB0C2D3E4F5A6B7C8D9", "")
	reaction.Reaction = &webhook.Reaction{To: "3EB0B1C2D3E4F5A6B7C8", Emoji: "❤️"}
	unreact := direct("3EB0C3D4E5F6A7B8C9D0", "")
	unreact.Reaction = &webhook.Reaction{To: "3EB0B1C2D3E4F5A6B7C8", Emoji: ""}
	forwarded := direct("3EB0D2E3F4A5B6C7D8E9", "Promoção válida até sexta")
	forwarded.Forwarded = true

	return []webhookExampleGroup{
		{ID: "message", Title: "Mensagens (message)", Intro: "Cada mensagem nova, de uma conversa ou de um grupo. Os campos que não se aplicam ficam de fora do JSON.", Examples: []webhookExample{
			{ID: "texto", Title: "Texto numa conversa", JSON: event("message", "9f2c41d07a3b8e65", direct("3EB0C767D26A1D8B1E00", "Oi! Você vem amanhã?"))},
			{ID: "grupo", Title: "Resposta num grupo", Note: "group é true, e reply_to diz qual mensagem foi citada. sender_jid é quem escreveu; chat_jid, o grupo.", JSON: event("message", "1b7e93c2a4f05d68", reply)},
			{ID: "foto", Title: "Foto com legenda", Note: "A mídia vem descrita, sem o arquivo; a legenda vem em text e em media.caption. Para o conteúdo, a sua ferramenta de IA usa read_media ou download_media com o id.", JSON: event("message", "2c8f04d3b5a16e79", photo)},
			{ID: "documento", Title: "Documento", JSON: event("message", "3d9015e4c6b27f8a", document)},
			{ID: "audio", Title: "Áudio", Note: "Áudios de voz chegam assim, sem transcrição: a transcrição acontece quando a sua ferramenta de IA pede.", JSON: event("message", "4ea126f5d7c3809b", voice)},
			{ID: "localizacao", Title: "Localização", JSON: event("message", "5fb23706e8d491ac", place)},
			{ID: "enquete", Title: "Enquete", Note: "Os votos não chegam como avisos; o resultado se lê com get_poll_results.", JSON: event("message", "60c34817f9e5a2bd", poll)},
			{ID: "encaminhada", Title: "Mensagem encaminhada", JSON: event("message", "71d45928a0f6b3ce", forwarded)},
			{ID: "editada", Title: "Mensagem editada", Note: "Chega de novo com o id da mensagem original, edited true e o texto novo.", JSON: event("message", "82e56a39b107c4df", edited)},
			{ID: "apagada", Title: "Mensagem apagada para todos", Note: "Chega com o id da mensagem apagada, revoked true e sem texto.", JSON: event("message", "93f67b4ac218d5e0", revoked)},
			{ID: "sua", Title: "Mensagem que você mandou", Note: "Só chega se o webhook incluir as mensagens que você manda. from_me é true e não há sender_name.", JSON: event("message", "a4078c5bd329e6f1", own)},
		}},
		{ID: "reaction", Title: "Reações (reaction)", Intro: "Alguém reagiu a uma mensagem. reaction.to é o id da mensagem; um emoji vazio quer dizer que a reação foi tirada.", Examples: []webhookExample{
			{ID: "reacao", Title: "Reação", JSON: event("reaction", "b5189d6ce43af702", reaction)},
			{ID: "reacao-removida", Title: "Reação tirada", JSON: event("reaction", "c629ae7df54b0813", unreact)},
		}},
		{ID: "receipt", Title: "Entregas e leituras (receipt)", Intro: "Uma mensagem sua foi entregue, lida ou ouvida. Vários ids podem vir num aviso só.", Examples: []webhookExample{
			{ID: "lida", Title: "Lida", Note: "type é delivered (entregue), read (lida) ou played (áudio ouvido). Num grupo, sender_jid é quem leu.",
				JSON: exampleJSON(webhook.Event{Kind: "receipt", ID: "d73abf8e065c1924", At: sent, Receipt: &webhook.Receipt{ChatJID: maria, SenderJID: maria,
					MessageIDs: []string{"3EB0B1C2D3E4F5A6B7C8", "3EB0B1C2D3E4F5A6B7C9"}, Type: "read", Timestamp: at}})},
		}},
		{ID: "test", Title: "Aviso de teste", Intro: "O que o botão Testar manda. Tem test true; não conta para o desligamento automático.", Examples: []webhookExample{
			{ID: "teste", Title: "Teste", JSON: exampleJSON(webhook.Event{Kind: "message", ID: "e84bc09f176d2a35", At: sent, Test: true,
				Message: &webhook.Message{ID: "TEST", ChatJID: "5511900000000@s.whatsapp.net", ChatName: "Teste", Timestamp: at,
					SenderJID: "5511900000000@s.whatsapp.net", SenderName: "Teste", Text: "Mensagem de teste do WhatsApp MCP"}})},
		}},
	}
}

func (p *Panel) webhookDocs(w http.ResponseWriter, r *http.Request) (string, any) {
	s := p.snapshot(r.Context())
	return "webhookdocs", struct {
		layout
		Groups  []webhookExampleGroup
		APIBase string
	}{p.layout(r, "Webhooks: documentação", "configuracoes", s), webhookExamples(), strings.TrimSuffix(p.endpoint(), "/mcp")}
}

const webhookDocsSource = `{{define "webhookdocs"}}{{template "head" .}}{{template "nav" .}}
<p class="muted" style="margin:0 0 6px"><a href="/configuracoes#webhooks">← Configurações › Webhooks</a></p>
<h1>Webhooks: o que chega</h1>
<p class="lead">Cada aviso é um POST com JSON no endereço do webhook. Aqui estão o formato, como conferir que ele veio mesmo do WhatsApp MCP e um exemplo de cada tipo.</p>
<nav class="note" aria-label="Nesta página"><p style="margin:0"><strong>Nesta página:</strong> <a href="#como-chega">Como chega</a> · <a href="#assinatura">Conferir a assinatura</a> · <a href="#envelope">Os campos</a> · <a href="#exemplo-programa">Um programa que recebe</a></p>
{{range .Groups}}<p style="margin:6px 0 0"><a href="#{{.ID}}"><strong>{{.Title}}</strong></a>: {{range $i, $e := .Examples}}{{if $i}} · {{end}}<a href="#exemplo-{{$e.ID}}">{{$e.Title}}</a>{{end}}</p>
{{end}}</nav>

<section class="card" id="como-chega">
<div class="card__head"><h2 class="card__title">{{icon "code"}}Como chega</h2></div>
<div class="card__body stack">
<ul class="rows">
<li class="row"><span class="row__main"><span class="row__title mono">Content-Type: application/json</span><span class="row__meta">O corpo é o aviso, em UTF-8.</span></span></li>
<li class="row"><span class="row__main"><span class="row__title mono">X-WhatsApp-MCP-Event</span><span class="row__meta">O tipo do aviso: message, reaction ou receipt. É o mesmo campo event do corpo.</span></span></li>
<li class="row"><span class="row__main"><span class="row__title mono">X-WhatsApp-MCP-Delivery</span><span class="row__meta">O id do aviso, o mesmo delivery_id do corpo. Uma nova tentativa do mesmo aviso repete o id: use-o para não processar duas vezes.</span></span></li>
<li class="row"><span class="row__main"><span class="row__title mono">X-WhatsApp-MCP-Signature</span><span class="row__meta">sha256= seguido do HMAC-SHA256 do corpo com a chave do webhook, em hexadecimal.</span></span></li>
</ul>
<ul class="stack" style="margin:0;padding-left:20px">
<li><strong>Responda com qualquer 2xx em até 3 segundos</strong> e faça o trabalho pesado depois. Outra resposta, ou nenhuma, conta como falha.</li>
<li><strong>Cada webhook tem uma fila, entregue em ordem.</strong> Um aviso que falha é tentado de novo, em intervalos crescentes, até 10 vezes em menos de um minuto, e os seguintes esperam.</li>
<li><strong>Na 10ª falha seguida o webhook é desligado</strong> e os avisos que esperavam são descartados. Ele volta em Configurações, com a fila vazia.</li>
<li><strong>Só chegam mensagens novas.</strong> O histórico que o celular manda depois de conectar não vira aviso, nem os votos de enquete.</li>
</ul>
</div></section>

<section class="card" id="assinatura">
<div class="card__head"><h2 class="card__title">{{icon "code"}}Conferir a assinatura</h2></div>
<div class="card__body stack">
<p class="muted">Calcule o HMAC-SHA256 do corpo exatamente como chegou, com a chave que apareceu ao criar o webhook, e compare com o cabeçalho X-WhatsApp-MCP-Signature. Se não bater, ignore o aviso.</p>
<div class="snippet"><div class="snippet__head"><span class="snippet__title">Python</span></div><pre data-copy><code>import hashlib, hmac

def assinatura_valida(corpo: bytes, cabecalho: str, chave: str) -&gt; bool:
    esperado = "sha256=" + hmac.new(chave.encode(), corpo, hashlib.sha256).hexdigest()
    return hmac.compare_digest(esperado, cabecalho or "")</code></pre></div>
<div class="snippet"><div class="snippet__head"><span class="snippet__title">Node.js</span></div><pre data-copy><code>const crypto = require("crypto");

function assinaturaValida(corpo, cabecalho, chave) {
  const esperado = "sha256=" + crypto.createHmac("sha256", chave).update(corpo).digest("hex");
  return typeof cabecalho === "string" &amp;&amp; cabecalho.length === esperado.length &amp;&amp;
    crypto.timingSafeEqual(Buffer.from(cabecalho), Buffer.from(esperado));
}</code></pre></div>
</div></section>

<section class="card" id="envelope">
<div class="card__head"><h2 class="card__title">{{icon "code"}}Os campos</h2></div>
<div class="card__body stack">
<h3 class="card__sub" style="border:0;padding:0">Em todo aviso</h3>
<ul class="rows">
<li class="row"><span class="row__main"><span class="row__title mono">event</span><span class="row__meta">message, reaction ou receipt.</span></span></li>
<li class="row"><span class="row__main"><span class="row__title mono">delivery_id</span><span class="row__meta">O id do aviso; repete nas novas tentativas.</span></span></li>
<li class="row"><span class="row__main"><span class="row__title mono">sent_at</span><span class="row__meta">Quando o aviso foi montado, em UTC.</span></span></li>
<li class="row"><span class="row__main"><span class="row__title mono">test</span><span class="row__meta">true no aviso do botão Testar; ausente nos outros.</span></span></li>
</ul>
<h3 class="card__sub">message, em message e reaction</h3>
<ul class="rows">
<li class="row"><span class="row__main"><span class="row__title mono">id</span><span class="row__meta">O id da mensagem no WhatsApp, o mesmo que as ferramentas do MCP usam (responder, reagir, read_media).</span></span></li>
<li class="row"><span class="row__main"><span class="row__title mono">chat_jid · chat_name · group</span><span class="row__meta">A conversa: um número@s.whatsapp.net, ou um grupo terminado em @g.us. group diz se é grupo.</span></span></li>
<li class="row"><span class="row__main"><span class="row__title mono">timestamp</span><span class="row__meta">Quando a mensagem foi enviada, em UTC.</span></span></li>
<li class="row"><span class="row__main"><span class="row__title mono">from_me</span><span class="row__meta">true quando a mensagem é da própria conta.</span></span></li>
<li class="row"><span class="row__main"><span class="row__title mono">sender_jid · sender_name</span><span class="row__meta">Quem escreveu, com o nome da sua agenda quando houver. Em alguns grupos o WhatsApp esconde o número, e sender_jid termina em @lid.</span></span></li>
<li class="row"><span class="row__main"><span class="row__title mono">text</span><span class="row__meta">O texto, ou a legenda de uma mídia.</span></span></li>
<li class="row"><span class="row__main"><span class="row__title mono">media</span><span class="row__meta">type (image, video, audio, document, sticker), mime_type, filename, bytes e caption. O arquivo não vem junto.</span></span></li>
<li class="row"><span class="row__main"><span class="row__title mono">reply_to</span><span class="row__meta">A mensagem citada: id, sender_jid e o texto dela.</span></span></li>
<li class="row"><span class="row__main"><span class="row__title mono">reaction</span><span class="row__meta">Só em reaction: to (o id da mensagem) e emoji (vazio quando a reação foi tirada).</span></span></li>
<li class="row"><span class="row__main"><span class="row__title mono">location · poll</span><span class="row__meta">location: latitude, longitude, name, address, live. poll: question, options, max_answers.</span></span></li>
<li class="row"><span class="row__main"><span class="row__title mono">forwarded · edited · revoked</span><span class="row__meta">Aparecem quando são true: encaminhada, editada (com o texto novo), apagada para todos (sem texto).</span></span></li>
</ul>
<h3 class="card__sub">receipt</h3>
<ul class="rows">
<li class="row"><span class="row__main"><span class="row__title mono">chat_jid · sender_jid</span><span class="row__meta">A conversa e quem recebeu, leu ou ouviu.</span></span></li>
<li class="row"><span class="row__main"><span class="row__title mono">message_ids</span><span class="row__meta">Os ids das suas mensagens a que o aviso se refere.</span></span></li>
<li class="row"><span class="row__main"><span class="row__title mono">type · timestamp</span><span class="row__meta">delivered, read ou played, e quando.</span></span></li>
</ul>
</div></section>

{{range .Groups}}
<section class="card" id="{{.ID}}">
<div class="card__head"><h2 class="card__title">{{icon "code"}}{{.Title}}</h2></div>
<div class="card__body stack">
<p class="muted">{{.Intro}}</p>
{{range .Examples}}
<div class="stack" id="exemplo-{{.ID}}">
<h3 class="card__sub">{{.Title}}</h3>
{{with .Note}}<p class="muted" style="margin:0">{{.}}</p>{{end}}
<div class="snippet"><div class="snippet__head"><span class="snippet__title">JSON</span></div><pre data-copy><code>{{.JSON}}</code></pre></div>
</div>
{{end}}
</div></section>
{{end}}

<section class="card" id="exemplo-programa">
<div class="card__head"><h2 class="card__title">{{icon "terminal"}}Um programa que recebe</h2></div>
<div class="card__body stack">
<p class="muted">Um receptor mínimo em Python, sem bibliotecas: confere a assinatura, responde na hora e mostra quem escreveu o quê. Rode, adicione o webhook com o endereço http://127.0.0.1:8080/whatsapp e cole a chave no lugar indicado.</p>
<div class="snippet"><div class="snippet__head"><span class="snippet__title">receptor.py</span></div><pre data-copy><code># Recebe os avisos do WhatsApp MCP em http://127.0.0.1:8080/whatsapp
import hashlib, hmac, json
from http.server import BaseHTTPRequestHandler, HTTPServer

CHAVE = "cole aqui a chave do webhook"

class Webhook(BaseHTTPRequestHandler):
    def do_POST(self):
        corpo = self.rfile.read(int(self.headers["Content-Length"]))
        esperado = "sha256=" + hmac.new(CHAVE.encode(), corpo, hashlib.sha256).hexdigest()
        if not hmac.compare_digest(esperado, self.headers.get("X-WhatsApp-MCP-Signature", "")):
            self.send_response(401)
            self.end_headers()
            return
        self.send_response(204)  # responda logo; trabalhe depois
        self.end_headers()
        aviso = json.loads(corpo)
        if aviso["event"] == "message":
            m = aviso["message"]
            print(m.get("chat_name"), "·", m.get("sender_name", "eu"), ":", m.get("text", ""))

HTTPServer(("127.0.0.1", 8080), Webhook).serve_forever()</code></pre></div>
<p class="muted">Os webhooks também se criam e se apagam por programa, pela API local deste computador:</p>
<div class="snippet"><div class="snippet__head"><span class="snippet__title">Criar um webhook</span></div><pre data-copy><code>curl -s -X POST -H 'Content-Type: application/json' \
  -d '{"url": "http://127.0.0.1:8080/whatsapp", "events": ["message", "reaction"]}' \
  {{.APIBase}}/api/webhooks</code></pre></div>
</div></section>
{{template "foot" .}}{{end}}`
