# Num servidor sem tela

A versão de linha de comando faz pelo terminal tudo o que o painel faz:
instala o serviço, conecta o WhatsApp (com um QR code desenhado no próprio
terminal, ou com um código para digitar no celular), mostra o estado, troca a
porta e o token, configura webhooks, mostra o registro e se atualiza. Serve
para um servidor acessado por SSH, um computador que fica ligado o tempo todo
ou um contêiner Docker.

O MCP continua respondendo só em `127.0.0.1`: quem usa o WhatsApp é um
programa no próprio servidor (o Claude Code, um agente, um script), ou o app
de outro computador, pelo [acesso remoto](#usar-pelo-app-de-outro-computador).

Primeira vez montando um servidor? O [Modo servidor](modo-servidor.md) é o
passo a passo, da contratação da máquina ao app conectado.

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
mkdir -p build/docker/data && sudo chown 1000:1000 build/docker/data
docker compose -f build/docker/compose.yaml up -d --build
docker compose -f build/docker/compose.yaml exec whatsapp-mcp whatsapp-mcp pair
```

O [`build/docker/compose.yaml`](../build/docker/compose.yaml) é um ponto de
partida; copie para onde ficam os seus serviços e ajuste o `context`. O que
importa nele:

- **`network_mode: host`.** O daemon só responde em `127.0.0.1`; com a rede do
  servidor, o MCP fica em `http://127.0.0.1:47821/mcp` no próprio servidor.
  Sem isso, nada fora do contêiner alcança o MCP. Para usar pelo app de outro
  computador, o `install-docker.sh` monta outro compose, atrás de um proxy
  HTTPS ([abaixo](#usar-pelo-app-de-outro-computador)).
- **O volume `/data`** guarda tudo o que precisa sobreviver ao contêiner: a
  sessão e as mensagens do WhatsApp (`/data/wacli`) e o estado, as
  configurações e o token do daemon (`/data/mcp`). Apagar o volume desconecta
  o WhatsApp. A pasta `data/`, ao lado do compose, precisa existir e
  pertencer ao usuário 1000 do contêiner antes do primeiro `up`: sem isso, o
  Docker a cria como root e o daemon não consegue gravar nela (o
  `install-docker.sh` já cuida disso).
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

Os comandos `status`, `pair`, `logout`, `port`, `token`, `webhooks`,
`update` e `remote` aceitam `--json`: o resultado sai como um objeto JSON na saída padrão, e um erro também, como
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

## Usar pelo app de outro computador

O app do Mac (ou do Windows e do Linux) pode ser o controle deste servidor:
mostra o painel dele, conecta o WhatsApp dele por QR code ou código, e leva as
ferramentas de IA daquele computador até o MCP daqui. A conexão é só por
HTTPS, com uma camada de criptografia de ponta a ponta entre o app e o
servidor; os detalhes estão em [Acesso remoto](acesso-remoto.md).

O que precisa:

- **Um endereço HTTPS** para o servidor, com certificado válido (Let's
  Encrypt, por exemplo), servido por um proxy: um Traefik que já roda no
  Docker, ou outro proxy seu. Não há opção para HTTP nem para certificado
  inválido, nem na configuração inicial.
- **`WHATSAPP_MCP_REMOTE_LISTEN`** no daemon, com o endereço em que o proxy o
  alcança. O daemon serve ali só as rotas do acesso remoto; o painel, a API e
  o MCP continuam só no `127.0.0.1` do servidor.
- **`WHATSAPP_MCP_REMOTE_TRUSTED_PROXY`**, quando outros contêineres dividem a
  rede do proxy: o nome do contêiner do proxy (ou o IP, ou a faixa). As rotas
  remotas recusam qualquer outro.

O [`install-docker.sh`](../install-docker.sh) pergunta o endereço e o proxy, e
monta tudo isso:

```sh
curl -fsSL https://raw.githubusercontent.com/BrOrlandi/whatsapp-mcp-local/main/install-docker.sh -o install-docker.sh
bash install-docker.sh
```

Com um Traefik, o contêiner entra na rede Docker dele e recebe só o caminho
`/remote/v1/` do endereço escolhido; com outro proxy, a porta do acesso remoto
fica só no `127.0.0.1` do servidor, para o proxy encaminhar com
`X-Forwarded-Proto: https`.

Depois, para cada app:

```sh
docker exec whatsapp-mcp whatsapp-mcp remote code --name "Mac do escritório"
```

O código vale uma vez, por 10 minutos, e não aparece de novo. No app:
*Configurações › Acesso remoto › Usar um servidor*, com o endereço `https://`
e o código. O app para de rodar o WhatsApp no próprio computador e mostra, no
topo de cada tela, que está no modo remoto, qual servidor usa e como estão as
duas conexões: a do app com o servidor e a do servidor com o WhatsApp.

| Comando | O que faz |
|---|---|
| `remote` | Lista os aparelhos pareados, quando cada um apareceu e a impressão digital do servidor |
| `remote code [--name N] [--scope admin\|tools]` | Um código para mais um aparelho; `tools` alcança só o MCP |
| `remote revoke <id>` | Corta um aparelho na hora; ele precisa ser pareado de novo |
| `apikey [list]` | As chaves de API dos agentes que usam o MCP pelo endereço do servidor |
| `apikey new <nome>` | Uma chave para um agente; aparece uma vez só e deixa de valer se não for usada em 24 horas (também em *Conectar ferramenta de IA › Agentes remotos*) |
| `apikey revoke <id>` | Corta uma chave na hora |

## Trazer os dados de outro computador

Para levar a conexão do WhatsApp, as conversas já sincronizadas, as
transcrições, os webhooks e a mídia baixada de um computador para o
servidor, sem conectar o celular de novo:

1. **No computador, encerre o app** (*Encerrar*, no ícone). Uma sessão do
   WhatsApp não pode estar aberta em dois lugares: até o fim, só um dos dois
   roda.
2. **Faça um backup** da pasta de dados inteira, com o app encerrado (no
   macOS, `~/Library/Application Support/WhatsApp MCP/`), num arquivo `.tar`
   guardado fora dela. É ele que permite voltar atrás.
3. **Confira as versões do wacli.** O app traz um wacli (`wacli --version`
   dentro do app) e a imagem traz o de `build/wacli.env`. Com a mesma versão,
   o banco é o mesmo; com versões diferentes, veja
   [Dependências](dependencias.md#wacli) antes.
4. **No servidor, pare o contêiner**, copie a pasta `wacli/` para
   `data/wacli/` e o resto (`state.db`, `media/`, `exports/`, `models/` se
   houver) para `data/mcp/`, e dê a pasta ao usuário 1000:
   `sudo chown -R 1000:1000 data`.
5. **Suba o contêiner** e espere `whatsapp-mcp status` dizer que está
   conectado, sem pedir QR code. As conversas recentes devem bater com as do
   celular.
6. **No computador**, pareie o app com o servidor marcando *Levei os dados
   deste computador para o servidor*: o app guarda a pasta `wacli/` antiga com
   outro nome (não apaga), para nunca abrir aquela sessão de novo.

**Para voltar:** pare o contêiner, devolva a pasta guardada ao nome original
(ou restaure o backup) e, no app, *Voltar ao modo local*, confirmando que o
WhatsApp do servidor foi desconectado. Enquanto este computador guardar a
mesma sessão que o servidor usa, o app recusa voltar sem essa confirmação.

## Transcrição de áudio

No servidor, a transcrição roda só no processador: a mesma
`whatsapp-mcp transcription install` baixa o whisper.cpp e o modelo (cerca de
600 MB). Um áudio leva mais ou menos a sua própria duração para virar texto
([Transcrição](transcricao.md)).
