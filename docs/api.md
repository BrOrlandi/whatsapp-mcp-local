# API local

O WhatsApp MCP responde em `http://127.0.0.1:47821`, só para programas deste
computador. Além do MCP, em `/mcp`, ele tem uma API pequena para um agente de
IA (Claude Code, Codex, Cursor…) instalar, conectar e conferir o app sem
precisar da janela. O app e a versão de linha de comando têm a mesma API; só
`/api/app/open` é exclusivo do app.

## Acesso

- **Leituras** (`GET`) respondem a qualquer programa deste computador, sem
  chave. Isso inclui as rotas internas do painel, como as conversas recentes.
- **Ações** (`POST`) pedem `Content-Type: application/json`. Uma página de
  outro site aberta no navegador não consegue ler nem chamar nada daqui.
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
