# API local

O WhatsApp MCP responde em `http://127.0.0.1:47821`, só para programas deste
computador. Além do MCP, em `/mcp`, ele tem uma API pequena para um agente de
IA (Claude Code, Codex, Cursor…) instalar, conectar e conferir o app sem
precisar da janela. O app e a versão de linha de comando têm a mesma API; só
`/api/app/open` é exclusivo do app.

## Acesso

- **Leituras** (`GET`) respondem a qualquer programa deste computador, sem
  chave. Isso inclui as rotas internas do painel, como as conversas recentes.
- **Ações** (`POST`, `PATCH`, `DELETE`) pedem `Content-Type: application/json`,
  mesmo sem corpo. Uma página de outro site aberta no navegador não consegue
  ler nem chamar nada daqui.
- Com a variável `WHATSAPP_MCP_TOKEN` definida, o `/mcp`, o `/health` e as
  ações pedem `Authorization: Bearer <token>`; as leituras continuam abertas.
  Uma chave de acesso para todas as rotas, ligada em Configurações, fica para
  uma versão futura.
- **A porta** é 47821, a menos que tenha sido trocada em Configurações. Ela
  fica no `config.json` (campo `port`) da pasta de dados:

  | Sistema | Pasta de dados |
  |---|---|
  | macOS | `~/Library/Application Support/WhatsApp MCP/` |
  | Windows | `%LOCALAPPDATA%\WhatsApp MCP\` |
  | Linux | `~/.local/share/whatsapp-mcp/` |
  | Linha de comando | `~/.whatsapp-mcp/` |

## Endpoints

| Método e caminho | O que faz |
|---|---|
| `GET /api/status` | Onde as coisas estão e o que fazer em seguida (abaixo) |
| `POST /api/pair` | Começa a conectar o WhatsApp com um QR code. Com `{"phone": "+55 11 91234-5678"}`, pede um código para digitar no celular (e troca o QR code por ele) |
| `GET /api/pair/qr.png` | O QR code da vez, em PNG, enquanto `pairing.state` é `qr` |
| `POST /api/pair/cancel` | Cancela a conexão em andamento |
| `POST /api/clients/claude-code` | Configura o MCP no Claude Code deste computador. `{"replace": true}` substitui uma configuração antiga |
| `POST /api/clients/claude-desktop` | O mesmo no Claude Desktop (chat e Cowork), que precisa ser reiniciado |
| `POST /api/clients/codex` | O mesmo no Codex (o do app do ChatGPT, o app do Codex, o terminal e o editor), pelo comando `codex mcp add` |
| `POST /api/clients/cursor` | O mesmo no Cursor, no `~/.cursor/mcp.json` de todos os projetos (com uma cópia do arquivo anterior) |
| `POST /api/app/open` | Só no app: abre a janela, opcionalmente numa página: `{"path": "/status"}` |
| `GET /api/webhooks` | Os webhooks, com o estado e a última entrega de cada um ([abaixo](#webhooks)) |
| `POST /api/webhooks` | Cria um webhook e devolve o segredo que assina as entregas |
| `GET /api/webhooks/{id}` | Um webhook |
| `PATCH /api/webhooks/{id}` | Altera um webhook; `{"enabled": true}` religa um que foi desligado |
| `DELETE /api/webhooks/{id}` | Apaga um webhook |
| `POST /api/webhooks/{id}/test` | Manda uma entrega de teste agora e diz o que voltou |
| `GET /api/media` | Quanto ocupam os arquivos que as ferramentas baixaram: por tipo, por conversa, repetidos |
| `POST /api/media/retention` | Por quantos dias guardar os arquivos baixados: `{"days": 30}`, ou `0` para guardar sempre |
| `POST /api/media/purge` | Apaga os arquivos baixados (`{"what": "media"}`, opcionalmente só os de mais de `older_than_days`) ou as exportações (`{"what": "exports"}`) |
| `GET /health` | O relatório de saúde completo; responde 503 quando algo falha |
| `GET /healthz` | Só diz que o processo está de pé |

Um erro volta como `{"error": "…"}`, em português, com o status 409 quando é
algo a resolver (por exemplo, um WhatsApp já conectado).

## `GET /api/status`

```json
{
  "app": "WhatsApp MCP Local",
  "version": "1.2.0",
  "desktop": true,
  "mcp_url": "http://127.0.0.1:47821/mcp",
  "whatsapp": {
    "state": "connected",
    "paired": true,
    "connected": true,
    "phone": "+55 (11) 91234-5678",
    "name": "Maria Silva",
    "last_message_at": "2026-10-06T03:15:49Z"
  },
  "pairing": { "state": "idle" },
  "health": { "status": "ok", "summary": "working: connected to WhatsApp and receiving messages", "checks": [] },
  "next": "ready"
}
```

`whatsapp.connected` é verdadeiro quando o WhatsApp está recebendo e pronto para
enviar. `next` diz o que fazer:

| `next` | Significa | O que fazer |
|---|---|---|
| `ready` | Conectado, recebendo e pronto para enviar | Usar o MCP |
| `pair` | Nenhum WhatsApp conectado | Abrir o app (a primeira tela já mostra o QR code) ou chamar `POST /api/pair` |
| `scan` | Esperando a pessoa no celular | Ler o QR code (`pairing.qr_png`, também na janela do app) ou digitar `pairing.code` |
| `syncing` | Conectado; o celular está mandando o histórico | Esperar alguns minutos |
| `wait` | Abrindo, reconectando ou pausado por instantes | Perguntar de novo em alguns segundos |
| `error` | A sincronização parou | Ler `health.checks`: cada verificação que falha diz o que fazer |

No celular, o QR code se lê em *WhatsApp › Configurações › Dispositivos
conectados › Conectar dispositivo*; o código, no mesmo lugar, em *Conectar com
número de telefone*.

## Webhooks

Um webhook é um endereço que recebe um `POST` a cada mensagem nova, para um
script neste computador (ou na rede) agir por conta própria: responder,
registrar, avisar em outro lugar. Para criar:

```sh
curl -s -X POST -H 'Content-Type: application/json' \
  -d '{"url": "http://127.0.0.1:8080/whatsapp", "events": ["message"]}' \
  http://127.0.0.1:47821/api/webhooks
```

| Campo | O que é |
|---|---|
| `url` | O endereço, `http://` ou `https://` (obrigatório) |
| `events` | O que ele recebe: `message` (mensagens novas, o padrão), `reaction` (reações) e `receipt` (as suas mensagens entregues, lidas ou ouvidas) |
| `include_own` | `true` também entrega as mensagens que a própria conta manda, do celular ou pelas ferramentas |
| `chats` | Só estas conversas (JIDs); vazio, todas |
| `enabled` | `false` desliga sem apagar |

A resposta traz o `secret`, uma única vez: ele assina cada entrega. Cada
entrega é um `POST` com JSON e estes cabeçalhos:

- `X-WhatsApp-MCP-Event`: `message`, `reaction` ou `receipt`;
- `X-WhatsApp-MCP-Delivery`: o id da entrega;
- `X-WhatsApp-MCP-Signature`: `sha256=` seguido do HMAC-SHA256 do corpo com o
  segredo, em hexadecimal. Confira antes de confiar no conteúdo.

```json
{
  "event": "message",
  "delivery_id": "9f2c41d07a3b8e65",
  "sent_at": "2026-10-07T12:00:01Z",
  "message": {
    "id": "3EB0C767D26A1D8B1E",
    "chat_jid": "5511912345678@s.whatsapp.net",
    "chat_name": "Maria Silva",
    "group": false,
    "timestamp": "2026-10-07T12:00:00Z",
    "from_me": false,
    "sender_jid": "5511912345678@s.whatsapp.net",
    "sender_name": "Maria Silva",
    "text": "Oi! Você vem amanhã?",
    "reply_to": { "id": "3EB0A1…", "text": "Combinado então" }
  }
}
```

Uma mensagem com mídia traz `media` (`type`, `mime_type`, `filename`, `bytes`,
`caption`), sem o arquivo: o script pede pelo MCP (`download_media`,
`read_media`) se precisar. Uma reação traz `reaction` (`to`, `emoji`; vazio
quando foi removida); uma confirmação, `receipt` (`message_ids`, `type`:
`delivered`, `read` ou `played`). Só chegam mensagens novas: o histórico que o
celular manda depois de conectar não é entregue.

**Entrega.** Cada webhook tem uma fila, entregue em ordem. Qualquer resposta
`2xx` conta como entregue; responda em até 3 segundos e faça o trabalho
pesado depois. Uma entrega que falha é tentada de novo, em intervalos
crescentes, até 10 vezes em menos de um minuto, com as mensagens seguintes
esperando na fila. Na 10ª falha o webhook é **desligado** e a fila de
pendentes é **descartada**; `GET /api/webhooks` mostra `enabled: false`,
`disabled_at` e `disabled_reason`, e `PATCH` com `{"enabled": true}` o religa,
com a fila vazia. `POST /api/webhooks/{id}/test` manda uma entrega de teste
(com `"test": true`) sem contar para o desligamento.

A mesma configuração aparece em **Configurações › Webhooks**, no app e no
painel da linha de comando, e o botão **Documentação** do card abre uma página
com um exemplo de cada tipo de aviso, os campos, a conferência da assinatura e
um receptor mínimo em Python.

Os webhooks precisam de um wacli que entregue mensagens novas (o que vem com o
app entrega); com um mais antigo, `GET /api/webhooks` responde
`"available": false`.

## Exemplos

```sh
# macOS e Linux
curl -s http://127.0.0.1:47821/api/status
curl -s -X POST -H 'Content-Type: application/json' -d '{}' http://127.0.0.1:47821/api/pair
curl -s -X POST -H 'Content-Type: application/json' -d '{"phone": "+55 11 91234-5678"}' http://127.0.0.1:47821/api/pair
curl -s -X POST -H 'Content-Type: application/json' -d '{}' http://127.0.0.1:47821/api/clients/claude-code
```

```powershell
# Windows (PowerShell)
Invoke-RestMethod http://127.0.0.1:47821/api/status
Invoke-RestMethod -Method Post -ContentType 'application/json' -Body '{}' http://127.0.0.1:47821/api/pair
Invoke-RestMethod -Method Post -ContentType 'application/json' -Body '{}' http://127.0.0.1:47821/api/app/open
```
