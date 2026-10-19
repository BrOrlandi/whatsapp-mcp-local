# Num servidor sem tela

A versão de linha de comando faz pelo terminal tudo o que o painel faz:
instala o serviço, conecta o WhatsApp (com um QR code desenhado no próprio
terminal, ou com um código para digitar no celular), mostra o estado, troca a
porta e o token, configura webhooks, mostra o registro e se atualiza. Serve
para um servidor acessado por SSH, um computador que fica ligado o tempo todo
ou um contêiner Docker.

O MCP continua respondendo só em `127.0.0.1`: quem usa o WhatsApp é um
programa no próprio servidor (o Claude Code, um agente, um script). Para usar
de outro computador, veja [De outro computador](#de-outro-computador).

## Sem Docker

No macOS e no Linux, sem `sudo`:

```sh
curl -fsSL https://raw.githubusercontent.com/BrOrlandi/whatsapp-mcp-local/main/install.sh | bash
```

O instalador põe o `wacli` e o `whatsapp-mcp` em `~/.local/bin`, registra o
serviço (systemd do usuário no Linux, launchd no macOS) e, sem tela, não tenta
abrir o navegador. Depois:

```sh
whatsapp-mcp pair      # o QR code aparece no terminal
whatsapp-mcp status    # "pronto" quando o WhatsApp estiver conectado
whatsapp-mcp config    # como ligar o Claude Code e outras ferramentas
```

No Linux, um serviço do usuário para quando a última sessão SSH termina, a
menos que o systemd mantenha os serviços desse usuário rodando (o *linger*).
O `service install` liga o linger sozinho quando o sistema deixa; quando não
deixa, ele avisa, e o comando é:

```sh
sudo loginctl enable-linger $USER
```

## Com Docker

A imagem leva a linha de comando e o wacli da versão do app
(`build/wacli.env`, conferido pelo sha256). Não há imagem publicada: ela é
montada a partir do repositório.

```sh
git clone https://github.com/BrOrlandi/whatsapp-mcp-local.git
cd whatsapp-mcp-local
docker compose -f build/docker/compose.yaml up -d --build
docker compose -f build/docker/compose.yaml exec whatsapp-mcp whatsapp-mcp pair
```

O [`build/docker/compose.yaml`](../build/docker/compose.yaml) é um ponto de
partida; copie para onde ficam os seus serviços e ajuste o `context`. O que
importa nele:

- **`network_mode: host`.** O daemon só responde em `127.0.0.1`; com a rede do
  servidor, o MCP fica em `http://127.0.0.1:47821/mcp` no próprio servidor.
  Sem isso, nada fora do contêiner alcança o MCP.
- **O volume `/data`** guarda tudo o que precisa sobreviver ao contêiner: a
  sessão e as mensagens do WhatsApp (`/data/wacli`) e o estado, as
  configurações e o token do daemon (`/data/mcp`). Apagar o volume desconecta
  o WhatsApp. A pasta pertence ao usuário 1000 do contêiner.
- **O contêiner é o serviço.** Os comandos `service` não se aplicam:
  `docker compose up -d`, `restart` e `stop` fazem esse papel, e o registro
  sai em `docker compose logs -f`.
- **Atualizar** é montar a imagem de novo com o repositório atualizado:
  `git pull && docker compose -f build/docker/compose.yaml up -d --build`. O
  `whatsapp-mcp update` só funciona fora do Docker.

Cada comando roda ao lado do serviço com `docker compose exec whatsapp-mcp
whatsapp-mcp <comando>`. Com `-it` (o padrão do `exec`), o QR code sai com as
cores certas; num `exec -T` (sem terminal), sai em blocos simples.

## Conectar o WhatsApp

```sh
whatsapp-mcp pair
```

O QR code aparece no terminal, desenhado com meios-blocos (cerca de 65
colunas por 33 linhas), e é trocado na tela a cada vez que o WhatsApp troca o
código: o primeiro dura um minuto, os seguintes uns 20 segundos. No celular:
*WhatsApp › Configurações › Dispositivos conectados › Conectar dispositivo*.
Quando o celular confirma, o comando termina; o histórico continua chegando
em segundo plano, e o `status` mostra quando acabou.

Abaixo do desenho, o comando imprime **o conteúdo do QR code**, o texto que
ele codifica, para outra interface gerar a imagem: um app, uma página, um bot
do Telegram. Hoje esse conteúdo é um link
(`https://wa.me/settings/linked_devices#2@…`), mas trate-o como um texto
qualquer. Quem tiver o `qrencode` gera a imagem com
`qrencode -t ansiutf8 '<conteúdo>'`. O comando também dá o endereço da imagem
PNG no próprio servidor (`/api/pair/qr.png`), e `--qr-file qr.png` grava a
imagem num arquivo, trocado a cada código novo.

> O conteúdo do QR code liga a conta de quem o ler a este servidor. Não o
> publique nem o mande a um gerador de QR code na internet; passe-o só para
> quem vai ler o código.

| Opção | O que faz |
|---|---|
| `--phone '+55 11 91234-5678'` | Em vez do QR code, um código de 8 caracteres para digitar em *Conectar dispositivo › Conectar com número de telefone*. O número vai com DDI e DDD |
| `--timeout 5m` | Desiste depois desse tempo (saída 5) e cancela a conexão |
| `--no-wait` | Termina assim que mostra o primeiro QR code (ou o código); a conexão continua, e outro programa acompanha pela API |
| `--qr-file <arquivo>` | Grava o QR code em PNG, trocado a cada código novo |
| `--invert` | Sem cores (num `exec -T`, num log), desenha para um terminal de fundo claro |
| `--json` | Uma linha de JSON por acontecimento ([abaixo](#automação)) |

Ctrl+C cancela a conexão. Com o WhatsApp já conectado, `pair` só avisa e
termina com sucesso; para trocar de número, `whatsapp-mcp logout` antes.

## Os comandos

| Comando | O que faz |
|---|---|
| `status` | Onde as coisas estão e o que fazer em seguida |
| `pair` | Conecta o WhatsApp ([acima](#conectar-o-whatsapp)) |
| `logout [--yes]` | Desconecta o WhatsApp deste servidor; sem terminal, pede `--yes` |
| `service install` | Instala e inicia o serviço; `uninstall`, `stop`, `start` e `restart` |
| `port [<porta>]` | Mostra a porta do MCP ou troca (de 1024 a 65535), reiniciando o serviço |
| `token [show\|new\|set <token>\|clear]` | A chave que o `/mcp`, o `/health` e as ações da API pedem ([abaixo](#token)) |
| `webhooks` | Lista; `add <url>`, `remove <id>`, `enable <id>`, `disable <id>`, `test <id>` |
| `logs [-f] [-n 100]` | O registro do serviço (`journalctl` no Linux, o arquivo de log no macOS) |
| `update [--check]` | Baixa a versão mais recente, confere o sha256 e reinicia o serviço |
| `config` | Como configurar o Claude Code e o Claude Desktop |

`webhooks add` aceita `--events message,reaction,receipt`, `--include-own` e
`--chats <jid>,<jid>`, os mesmos campos da [API](api.md#webhooks). O segredo
que assina as entregas aparece uma vez, na criação.

A porta e o token ficam na pasta de dados (`~/.whatsapp-mcp/`, ou `/data/mcp`
no Docker): a porta no `config.json`, o token no arquivo `token`, legível só
pelo dono. As variáveis `WHATSAPP_MCP_PORT` e `WHATSAPP_MCP_TOKEN` vencem os
arquivos; quando uma delas está definida, `port` e `token` não trocam o valor
e dizem para mudar a variável. No Docker, a troca vale depois de
`docker compose restart`.

### Token

Sem token, qualquer programa do servidor usa o MCP. `whatsapp-mcp token new`
gera um, reinicia o serviço e o mostra; a partir daí, o `/mcp`, o `/health` e
as ações da API pedem `Authorization: Bearer <token>`, e os próprios comandos
o usam sozinhos. As leituras da API (`GET`) continuam abertas para os
programas do servidor. `whatsapp-mcp config` já traz o token na configuração
de cada ferramenta.

## Automação

Todos os comandos, menos `serve`, `bridge` e `logs`, aceitam `--json`: o
resultado sai como um objeto JSON na saída padrão, e um erro também, como
`{"error": "…", "exit_code": 4}`. O `status --json` é o mesmo
[`GET /api/status`](api.md#get-apistatus).

O `pair --json` escreve uma linha por acontecimento:

```json
{"event":"qr","qr":"https://wa.me/settings/linked_devices#2@…","qr_png":"http://127.0.0.1:47821/api/pair/qr.png","qr_version":1}
{"event":"qr","qr":"https://wa.me/settings/linked_devices#2@…","qr_png":"http://127.0.0.1:47821/api/pair/qr.png","qr_version":2}
{"event":"paired","phone":"+55 (11) 91234-5678","name":"Maria Silva"}
```

Com `--phone`, o primeiro é `{"event":"code","code":"ABCD-EFGH","phone":"…"}`;
com o WhatsApp já conectado, `{"event":"already_paired",…}`.

| Saída | Significa |
|---|---|
| 0 | Feito; no `status`, o WhatsApp está pronto |
| 1 | Algo deu errado; a mensagem diz o quê |
| 2 | O comando foi dado errado |
| 3 | O daemon não responde na porta (o serviço não está rodando) |
| 4 | O WhatsApp não está pronto (`status`: o campo `next` diz por quê) ou a conexão terminou sem ele |
| 5 | O `--timeout` acabou |
| 6 | Não dá para fazer aqui: uma porta ocupada, um valor fixado por variável, um comando que não vale no Docker, um pedido que a API recusou |
| 130 | Interrompido com Ctrl+C |

Um bot que conecta o WhatsApp por outra tela, por exemplo, roda
`whatsapp-mcp pair --json`, manda cada `qr` como imagem para a pessoa e para
quando chega `paired`. Ou pede o código: `pair --phone <número> --json
--no-wait` devolve o código e deixa a conexão correndo, e `status --json`
diz quando terminou.

## De outro computador

O daemon recusa qualquer pedido que não venha de `127.0.0.1`, e as leituras
da API não pedem token. Abrir a porta para a rede exporia as conversas a quem
alcançasse o servidor, e por isso não há opção para isso nesta versão. Para
usar o painel ou o MCP de outro computador, um túnel SSH leva a porta até lá:

```sh
ssh -N -L 47821:127.0.0.1:47821 usuario@servidor
```

Com o túnel aberto, o painel fica em `http://127.0.0.1:47821/` e o MCP em
`http://127.0.0.1:47821/mcp` no seu computador, como se o daemon rodasse nele.

## Transcrição de áudio

No servidor, a transcrição roda só no processador: a mesma
`whatsapp-mcp transcription install` baixa o whisper.cpp e o modelo (cerca de
600 MB). Um áudio leva mais ou menos a sua própria duração para virar texto
([Transcrição](transcricao.md)).
