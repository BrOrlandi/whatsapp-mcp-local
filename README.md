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

## Instalação

### 🤖 O jeito fácil: peça para o Claude

Copie o prompt abaixo e cole no **Claude Code**, no **Cowork** ou em qualquer
agente de IA que rode comandos no seu computador. Ele instala tudo e abre o
painel no navegador. Você só escaneia o QR code.

<details>
<summary><strong>📋 Clique para abrir o prompt — copie tudo</strong></summary>

```
Quero instalar o WhatsApp MCP local neste computador, para que você (e o Claude
Desktop) possam ler e enviar mensagens pelo meu WhatsApp.

O projeto é este: https://github.com/BrOrlandi/whatsapp-mcp-v2
Leia o README dele antes de começar e siga o que está lá, sem inventar passos.

O QUE FAZER
1. Rode o instalador e me diga, em uma frase, o que ele vai fazer antes de
   rodar:
     curl -fsSL https://raw.githubusercontent.com/BrOrlandi/whatsapp-mcp-v2/main/install.sh | bash
   Se o repositório for privado e o curl falhar, clone com
   `gh repo clone BrOrlandi/whatsapp-mcp-v2` e rode ./install.sh de dentro da
   pasta. Ele instala o wacli e o whatsapp-mcp-v2 em ~/.local/bin, sem sudo,
   deixa o serviço rodando no login e abre o painel no navegador.
2. Quando o painel abrir (http://127.0.0.1:47821/), me diga para clicar em
   "Conectar WhatsApp" e escanear o QR code pelo celular em
   WhatsApp > Dispositivos conectados > Conectar dispositivo. Espere eu
   confirmar que conectei.
3. Confira se está tudo funcionando: `curl -s http://127.0.0.1:47821/health`.
   O status deve ser "ok" ou "warn". Se vier "fail", leia o campo "fix" das
   checagens e resolva comigo.
4. No painel, em "Conectar ao Claude", me diga para clicar nos botões do Claude
   Code e/ou do Claude Desktop. O Desktop precisa ser fechado e aberto de novo
   depois.

REGRAS
- Explique em português simples e uma coisa de cada vez.
- Nunca me peça para colar aqui o QR code, códigos de pareamento ou chaves.
- Se algo der errado, mostre a mensagem de erro exata e o que fazer. Os logs
  ficam em ~/Library/Logs/whatsapp-mcp-v2.log (macOS).
- Se você não puder rodar comandos no meu computador (por exemplo, no chat do
  claude.ai), me ensine a abrir o Terminal e a colar o comando do passo 1.
```

</details>

### Ou rode você mesmo

```sh
curl -fsSL https://raw.githubusercontent.com/BrOrlandi/whatsapp-mcp-v2/main/install.sh | bash
```

Enquanto o repositório for privado, rode a partir de um clone:

```sh
gh repo clone BrOrlandi/whatsapp-mcp-v2 && cd whatsapp-mcp-v2 && ./install.sh
```

O instalador:

1. instala o **wacli** (pelo Homebrew, ou baixando o release oficial);
2. instala o **whatsapp-mcp-v2** em `~/.local/bin`, baixando o release ou
   compilando o código com Go quando não houver release;
3. deixa o daemon rodando no login e reiniciando se cair (LaunchAgent no macOS,
   `systemd --user` no Linux);
4. abre o **painel** em `http://127.0.0.1:47821/`.

Tudo o mais é feito no painel:

- **Conectar WhatsApp** mostra o QR code. No celular, vá em *Dispositivos
  conectados > Conectar dispositivo*. Se preferir, dá para digitar um código de
  8 caracteres usando o número de telefone. Por trás, o painel roda o
  `wacli auth` e acompanha a primeira sincronização do histórico.
- **Saúde** diz se o WhatsApp está pareado, se o sync está conectado e se as
  mensagens estão chegando.
- **Conectar ao Claude** tem botões que adicionam o servidor ao Claude Code
  (`claude mcp add`) e ao Claude Desktop, editando a configuração com um backup
  ao lado.

Rodar o instalador de novo atualiza tudo no lugar. Para abrir o painel depois:
`whatsapp-mcp-v2 open`. Não rode `wacli sync` à mão, porque quem roda o sync é
o daemon. Logs: `~/Library/Logs/whatsapp-mcp-v2.log` (macOS) ou
`journalctl --user -u whatsapp-mcp-v2` (Linux).

## Conectar os clientes

O painel faz isso com um clique. Os dois clientes aparecem como
`whatsapp-local`, então convivem com o `whatsapp` do v1 hospedado.

**Claude Code** (HTTP direto):

```sh
claude mcp add --scope user --transport http whatsapp-local http://127.0.0.1:47821/mcp
```

**Claude Desktop (Chat e Cowork)** em
`~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "whatsapp-local": {
      "command": "/caminho/absoluto/para/whatsapp-mcp-v2",
      "args": ["bridge"]
    }
  }
}
```

O `bridge` é um servidor stdio que só repassa cada mensagem para o daemon.
Assim o Desktop continua usando a mesma sessão, sem abrir outra. O Cowork usa
os servidores desse arquivo através do próprio Desktop. Depois de editar, feche
e abra o Claude Desktop.

**O que não funciona:** "Settings > Connectors > Add custom connector" no
Desktop e no claude.ai, e o app do celular. Nesses casos quem conecta é a nuvem
da Anthropic, e ela não alcança o `localhost` da sua máquina. Para eles é
preciso o gateway hospedado (v1) ou um túnel com autenticação.

## Segurança

- Escuta só em `127.0.0.1`.
- Recusa `Host` que não seja loopback (proteção contra DNS rebinding) e
  qualquer `Origin` que não seja o próprio servidor. Nem outra aba aberta num
  `localhost` de outra porta consegue chamar as tools.
- A API do painel, que roda o `wacli auth` e edita configurações, só aceita
  chamadas da própria página: exige `Sec-Fetch-Site: same-origin` e um header
  próprio que força preflight de CORS.
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

`health` dá um veredito (ok, warn ou fail) sobre o daemon, o pareamento, o
sync e a chegada de mensagens. Cada checagem que não está ok diz o que fazer. O
mesmo relatório responde em `GET /health`, com 503 quando algo falha, para
monitoramento.

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
