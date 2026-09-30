# WhatsApp MCP v2 (localhost, sobre wacli)

Um daemon que roda na sua própria máquina e coloca o seu WhatsApp atrás de um
endpoint MCP em `http://127.0.0.1:47821/mcp`. Por baixo ele usa o
[wacli](https://github.com/openclaw/wacli), que pareia como dispositivo
vinculado (protocolo do WhatsApp Web via whatsmeow) e espelha as mensagens num
SQLite local com busca full-text.

É a versão local do [whatsapp-mcp](https://github.com/BrOrlandi/whatsapp-mcp):
não precisa de VPS, Evolution, RabbitMQ nem Postgres. Ficam só um binário, o
wacli e o disco da sua máquina.

> **Use por sua conta e risco.** O WhatsApp não oferece API oficial para contas
> pessoais. O wacli é um cliente não oficial. Uma conta vinculada pode ser
> desconectada, quebrar por mudança de protocolo ou ser restringida.

## Como funciona

```
Claude Code ──HTTP──┐
                    ├──► whatsapp-mcp-v2 serve (127.0.0.1:47821)
Claude Desktop ─┐   │        │
  (Chat, Cowork)│   │        ├─ supervisiona  wacli sync --follow   (dono da sessão e do lock)
  stdio: bridge ┘───┘        ├─ envia via     socket de delegação do sync
                             ├─ lê            wacli --read-only --json   +   wacli.db (somente leitura)
                             └─ guarda        ~/.whatsapp-mcp-v2 (transcrições, chave OpenAI, mídia)
```

- **Um único daemon é dono da sessão.** O wacli permite um só processo com o
  lock do store. O daemon mantém o `sync --follow` sempre rodando, e todos os
  clientes falam com esse mesmo daemon. Nenhum cliente sobe um segundo
  WhatsApp.
- **Envios** (texto, arquivo, localização, enquete, reação, edição) são
  delegados ao sync em execução, sem interrompê-lo.
- **Operações que precisam do lock** são exclusivas: sync_history, revogar
  mensagem, arquivar/fixar/silenciar, check_numbers, foto de perfil e
  get_group com `live`. O daemon pausa o sync por alguns segundos, executa a
  operação e religa o sync. A fila offline do WhatsApp entrega na reconexão o
  que chegou nesse intervalo. Envios feitos durante a pausa esperam na fila em
  vez de falhar.
- **Leituras** usam `wacli --read-only` e uma conexão SQLite somente leitura
  (modo que o wacli documenta como seguro durante o sync). Nada escreve no
  `wacli.db`.

## Instalação (macOS)

```sh
# 1. wacli
brew install openclaw/tap/wacli

# 2. este daemon
go install github.com/BrOrlandi/whatsapp-mcp-v2/cmd/whatsapp-mcp-v2@latest
# ou: git clone … && go build -o ~/bin/whatsapp-mcp-v2 ./cmd/whatsapp-mcp-v2

# 3. parear o WhatsApp (QR code no terminal; celular > Dispositivos conectados)
wacli auth

# 4. deixar o daemon rodando no login, reiniciando se cair
whatsapp-mcp-v2 service install

# 5. ver a configuração dos clientes
whatsapp-mcp-v2 config
```

Rode o `wacli auth` antes de instalar o serviço. Depois do pareamento, não rode
`wacli sync` à mão: o daemon é quem roda o sync.

Logs no macOS: `~/Library/Logs/whatsapp-mcp-v2.log`. No Linux, `service
install` cria uma unit `systemd --user`.

## Conectar os clientes

**Claude Code** (HTTP direto):

```sh
claude mcp add --scope user --transport http whatsapp http://127.0.0.1:47821/mcp
```

**Claude Desktop (Chat e Cowork)** em
`~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "whatsapp": {
      "command": "/caminho/absoluto/para/whatsapp-mcp-v2",
      "args": ["bridge"]
    }
  }
}
```

O `bridge` é um servidor stdio que só repassa cada mensagem para o daemon.
Assim o Desktop continua usando a mesma sessão, sem abrir outra. O Cowork usa
os servidores desse arquivo através do próprio Desktop.

**O que não funciona:** "Settings > Connectors > Add custom connector" no
Desktop e no claude.ai, e o app do celular. Nesses casos quem conecta é a nuvem
da Anthropic, e ela não alcança o `localhost` da sua máquina. Para eles é
preciso o gateway hospedado (v1) ou um túnel com autenticação.

## Segurança

- Escuta só em `127.0.0.1`.
- Recusa `Host` que não seja loopback (proteção contra DNS rebinding) e
  qualquer `Origin` de fora (uma página aberta no navegador não consegue chamar
  as tools).
- `WHATSAPP_MCP_TOKEN` opcional: com ele definido, toda chamada precisa de
  `Authorization: Bearer <token>`. Isso fecha o endpoint também para outros
  programas locais. `whatsapp-mcp-v2 config` imprime a configuração com o
  token.
- O conteúdo das mensagens é de terceiros. Os resultados avisam o modelo para
  tratá-lo como dado, nunca como instrução.

## Configuração

| Variável | Padrão |
|---|---|
| `WHATSAPP_MCP_PORT` | `47821` |
| `WHATSAPP_MCP_TOKEN` | vazio (sem token) |
| `WACLI_BIN` | `wacli` no PATH ou no Homebrew |
| `WACLI_STORE_DIR` | o padrão do wacli (`~/.wacli` no macOS) |
| `WHATSAPP_MCP_DATA` | `~/.whatsapp-mcp-v2` |

## Tools

As mesmas do v1, com três diferenças que vêm do wacli:

| Diferença | Por quê |
|---|---|
| `send_contact` não existe | o wacli não envia cartão de contato (vCard) |
| `sync_history` não tem `before`; ganhou `rounds` | o wacli sempre ancora na mensagem mais antiga que tem de cada conversa. `rounds` pagina mais para trás na mesma chamada |
| `download_media` devolve `path` | o arquivo já está no disco da sua máquina, e um cliente com acesso a arquivos lê direto dali |

`whatsapp_status` informa se o WhatsApp está pareado, o estado do sync, até
onde o índice alcança e as **janelas desconhecidas**: períodos em que nenhuma
conversa recebeu nada, que é o rastro de um notebook fechado.

## Limitações de rodar localmente

- **O índice só cresce com o daemon rodando.** Com o notebook fechado, o
  WhatsApp guarda uma fila para o dispositivo vinculado e entrega na
  reconexão. Para períodos longos essa entrega não é garantida, e um
  dispositivo inativo por semanas é deslogado.
- **Histórico antigo é best-effort.** O pareamento traz o pacote inicial que o
  celular decide mandar. Depois disso, `sync_history` pede ao celular (que
  precisa estar online) as mensagens anteriores à mais antiga conhecida de cada
  conversa. Não existe forma de pedir "o que veio depois de X", então um buraco
  numa conversa que ficou quieta só pode ser preenchido quando chegar uma
  mensagem nova nela.
- **Mídia antiga expira no CDN do WhatsApp.** Nesse caso,
  `wacli media retry --chat <jid>` pede ao celular que envie o arquivo de novo.

## Desenvolvimento

```sh
go build -o bin/whatsapp-mcp-v2 ./cmd/whatsapp-mcp-v2
go test ./...
WHATSAPP_MCP_PORT=47899 WACLI_STORE_DIR=/tmp/wacli-test ./bin/whatsapp-mcp-v2 serve
```

O teste do supervisor usa um wacli falso (bash + python) que imita o lock, o
socket de delegação e o `sync --follow`. Ele verifica que operações exclusivas
pausam e religam o sync, e que envios concorrentes esperam na fila em vez de
falhar.
