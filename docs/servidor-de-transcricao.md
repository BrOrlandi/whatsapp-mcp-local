# Servidor de transcrição

Como subir um servidor de transcrição de áudio num computador seu (um Mac, um
PC com Linux, um servidor) e fazer o WhatsApp MCP mandar os áudios para ele,
seja o app desktop, seja a versão servidor. Este guia é para quem sabe usar o
terminal. Como a transcrição funciona por dentro está em
[Transcrição](transcricao.md). Este guia em inglês:
[Transcription server](transcription-server.md).

O servidor é a própria linha de comando do WhatsApp MCP:
`whatsapp-mcp transcription serve`. Ele usa o mesmo motor da transcrição local
(o whisper.cpp com o modelo Whisper large-v3-turbo) e fala a API de
transcrição da OpenAI, com uma chave. Não precisa de conta do WhatsApp nem do
resto do app: só da linha de comando e do motor.

## Quando vale a pena

Transcrever é pesado. Num computador sem placa de vídeo, um áudio leva mais
ou menos o próprio tempo para virar texto, ou mais num processador fraco (num
servidor com 4 núcleos fracos, mais de 5 minutos para 2 a 3 minutos de fala).
Num Mac com chip Apple, que usa a GPU (Metal), um áudio de 10 segundos leva
uns 2 segundos.

Então o caso típico é este: o WhatsApp MCP roda num servidor ou num mini PC
sem GPU, e um Mac com chip Apple na mesma casa ou no mesmo escritório faz a
transcrição. O áudio vai para o Mac, volta como texto, e não sai da sua rede.

| Onde o servidor roda | Aceleração | Vale a pena? |
|---|---|---|
| Mac com chip Apple (M1 em diante) | GPU (Metal) | Sim, é o caso para o qual ele foi feito |
| Mac com Intel | o pacote do macOS é o mesmo; não medido | Talvez; teste antes |
| Linux (amd64 ou arm64) | só processador: o pacote do Linux não usa GPU | Só se esse computador for bem mais forte que o que roda o WhatsApp MCP |
| Linux com placa NVIDIA | o pacote não usa a placa | Prefira uma das [alternativas](#alternativas) com CUDA |
| Windows | a transcrição local ainda não é suportada ([issue #14](https://github.com/BrOrlandi/whatsapp-mcp-local/issues/14)) | Não; este guia não cobre Windows |

O que ele precisa:

- **Disco**: cerca de 600 MB, uma vez (o modelo tem 574 MB, mais o programa
  de transcrição e um `ffmpeg` mínimo).
- **Memória**: cerca de 1 GB livre enquanto transcreve (o whisper.cpp com
  esse modelo usa uns 900 MB).
- **Rede**: o computador que manda os áudios precisa alcançar a porta do
  servidor (47823, por padrão). Veja [Acesso de fora](#acesso-de-fora).
- **Ficar ligado**: quando o servidor não responde, o WhatsApp MCP transcreve
  no próprio computador, se puder ([abaixo](#se-o-servidor-não-responder)).

Ele transcreve um áudio por vez; um segundo pedido espera o primeiro terminar.
Aceita arquivos de até 64 MB, horas de fala no formato do WhatsApp.

## 1. Instalar a linha de comando

Baixe só a linha de comando, do último release. **Não use o `install.sh`
para isso**: ele também instala o wacli e registra o WhatsApp MCP inteiro como
serviço, o que um servidor de transcrição não precisa.

macOS com chip Apple:

```sh
mkdir -p ~/.local/bin
curl -fsSL https://github.com/BrOrlandi/whatsapp-mcp-local/releases/latest/download/whatsapp-mcp_darwin_arm64.tar.gz | tar -xz -C ~/.local/bin
xattr -d com.apple.quarantine ~/.local/bin/whatsapp-mcp 2>/dev/null || true
~/.local/bin/whatsapp-mcp version
```

Linux (troque `amd64` por `arm64` num processador ARM):

```sh
mkdir -p ~/.local/bin
curl -fsSL https://github.com/BrOrlandi/whatsapp-mcp-local/releases/latest/download/whatsapp-mcp_linux_amd64.tar.gz | tar -xz -C ~/.local/bin
~/.local/bin/whatsapp-mcp version
```

Os arquivos de cada release e o `checksums.txt` estão em
[Releases](https://github.com/BrOrlandi/whatsapp-mcp-local/releases). Os
exemplos abaixo usam `~/.local/bin/whatsapp-mcp`; se essa pasta está no seu
`PATH`, basta `whatsapp-mcp`.

**A pasta de dados.** A linha de comando guarda tudo em `~/.whatsapp-mcp`
(ou na pasta de `WHATSAPP_MCP_DATA`): o motor em `bin/`, o modelo em
`models/` e a chave do servidor. Num Mac que **já tem o app** com a
transcrição instalada, aponte para a pasta do app e use o motor e o modelo que
ele já baixou, sem baixar de novo:

```sh
export WHATSAPP_MCP_DATA="$HOME/Library/Application Support/WhatsApp MCP"
```

Faça isso em todos os comandos abaixo (e no serviço, na seção 5).

## 2. Instalar o motor e o modelo

```sh
~/.local/bin/whatsapp-mcp transcription install
```

Ele baixa o programa de transcrição do seu sistema (conferido por sha256) e o
modelo do Hugging Face (também conferido), cerca de 600 MB, uma vez. Se o
`whisper-cli` e o `ffmpeg` já estão instalados no computador (pelo Homebrew,
por exemplo), ele usa esses e só baixa o modelo. Para conferir:

```sh
~/.local/bin/whatsapp-mcp transcription status
```

A linha `on this computer:` deve dizer `installed (metal)` num Mac com chip
Apple, ou `installed (cpu)`.

## 3. A chave e o primeiro teste

```sh
~/.local/bin/whatsapp-mcp transcription serve
```

Na primeira vez, ele cria a chave e a guarda em
`transcription-server.token`, na pasta de dados, com permissão só para o
dono. A chave nunca aparece na tela nem no log; ele diz em que arquivo ela
está e os endereços em que atende:

```
transcription server (OpenAI API) listening on [::]:47823 (metal)
on the computer that sends the voice notes, point WhatsApp MCP at it, with one of these addresses:
  http://192.168.0.20:47823
token: the one in /Users/voce/.whatsapp-mcp/transcription-server.token
```

`Ctrl+C` para. No macOS, se ele perguntar, permita conexões de entrada.

As opções:

| Opção | O que faz | Padrão |
|---|---|---|
| `--listen <endereço:porta>` | onde ele atende. `127.0.0.1:47823` atende só neste computador (para um proxy na frente, ou um túnel) | `0.0.0.0:47823`, todas as redes |
| `--token-file <arquivo>` | o arquivo com a chave. Se ele não existe ou está vazio, é criado com uma chave nova | `transcription-server.token` na pasta de dados |

A variável `WHATSAPP_MCP_TRANSCRIPTION_TOKEN`, quando definida, vence o
arquivo. A chave precisa ter pelo menos 16 caracteres. Para escolher a sua:

```sh
openssl rand -hex 24 > ~/transcricao.token && chmod 600 ~/transcricao.token
~/.local/bin/whatsapp-mcp transcription serve --token-file ~/transcricao.token
```

Para trocar a chave, apague o arquivo (ou troque o conteúdo), reinicie o
servidor e informe a nova no WhatsApp MCP.

### Testar com curl

Com o servidor rodando, de outro terminal (troque o caminho da chave e o
áudio; serve qualquer áudio que o `ffmpeg` leia, como o `.ogg` do WhatsApp):

```sh
CHAVE=$(cat ~/.whatsapp-mcp/transcription-server.token)

# está de pé e aceita a chave?
curl -s -H "Authorization: Bearer $CHAVE" http://127.0.0.1:47823/v1/health
# {"accel":"metal","engine":"whisper.cpp large-v3-turbo","ready":true,"version":"..."}

# transcrever
curl -s http://127.0.0.1:47823/v1/audio/transcriptions \
  -H "Authorization: Bearer $CHAVE" \
  -F file=@audio.ogg -F model=whisper-1 -F language=pt
# {"engine":"whisper.cpp large-v3-turbo","text":"..."}
```

É o formato da OpenAI: `POST /v1/audio/transcriptions` em
`multipart/form-data`, com `file` (obrigatório), `model` (qualquer valor),
`language` (sem ele, o idioma é detectado), `prompt` (texto que ajuda a
escrever nomes) e `response_format` (`json`, `text` ou `verbose_json`). Sem a
chave, a resposta é `401` com `invalid_api_key`. `GET /v1/models` também
responde, como na OpenAI. O protocolo completo está em
[Transcrição](transcricao.md#servidor-de-transcrição).

Como fala a API da OpenAI, qualquer cliente dela usa o servidor, como o SDK
oficial com `base_url="http://<servidor>:47823/v1"` e a chave como
`api_key`.

## 4. Conectar o WhatsApp MCP

No computador (ou servidor) onde o WhatsApp MCP roda, você informa o endereço
do servidor e a chave. O endereço é o que o `serve` mostrou
(`http://192.168.0.20:47823`), ou o seu endereço `https` se ele está atrás de
um proxy; sem caminho, o app completa com `/v1`.

**No painel**: *Configurações › Transcrição de áudio › Trocar onde
transcrever*, escolha **Em um servidor compatível com a OpenAI**, preencha
**Endereço** e **Chave** e clique em **Salvar e testar**. Em *Avançado*, o
**Modelo** pode ficar em branco: este servidor aceita qualquer nome.

**Pelo terminal**, com a chave pela entrada padrão (digitada sem aparecer,
ou de um arquivo; nunca num argumento, que outros usuários do computador
conseguem ver):

```sh
whatsapp-mcp transcription remote http://192.168.0.20:47823
# token (empty: keep the one kept, or none):   ← cole a chave e Enter

whatsapp-mcp transcription remote http://192.168.0.20:47823 < transcricao.token   # ou de um arquivo
whatsapp-mcp transcription status
```

O `status` deve dizer `transcription server http://192.168.0.20:47823:
answering, model whisper-1 (whisper.cpp large-v3-turbo, metal, <versão>)`. Com
a chave errada, ele diz que o servidor recusou a chave. Para voltar a
transcrever no próprio computador: `whatsapp-mcp transcription local`.

**Na versão servidor (Docker)**, no `.env` ao lado do `compose.yml`, e depois
`docker compose up -d`:

```sh
WHATSAPP_MCP_TRANSCRIPTION_URL=https://transcricao.exemplo.com
WHATSAPP_MCP_TRANSCRIPTION_TOKEN=a-chave-do-servidor
# opcionais:
# WHATSAPP_MCP_TRANSCRIPTION_MODE=remote      (basta a _URL para ser remote)
# WHATSAPP_MCP_TRANSCRIPTION_MODEL=whisper-1  (este servidor aceita qualquer um)
# WHATSAPP_MCP_TRANSCRIPTION_CONTEXT=names    (abaixo)
```

Com essas variáveis, o painel só mostra a configuração, sem deixar trocar.
Todas as variáveis estão em [Transcrição](transcricao.md#os-três-modos).

### O contexto da conversa

O WhatsApp MCP pode mandar, junto com o áudio, um pouco da conversa para o
servidor escrever melhor nomes e palavras incomuns. Quando o áudio vai para
fora, isso é uma escolha sua, em *Configurações › Transcrição de áudio ›
Mudar o contexto*, em `whatsapp-mcp transcription context` ou em
`WHATSAPP_MCP_TRANSCRIPTION_CONTEXT`:

| Nível | Valor | O que vai junto |
|---|---|---|
| Nada (o padrão) | `none` | só o áudio |
| Só os nomes | `names` | o nome da conversa e das pessoas, sem mensagens |
| Nomes e mensagens recentes | `messages` | os nomes e as últimas mensagens de texto antes do áudio |

Com um servidor seu, **Só os nomes** é uma boa escolha: é o que a
transcrição no próprio computador usa. Com as mensagens recentes, um áudio
numa língua diferente da conversa pode sair traduzido. Detalhes em
[o nível de contexto](transcricao.md#o-nível-de-contexto).

### Se o servidor não responder

O WhatsApp MCP dá 3 segundos para a conexão abrir e até 10 minutos para a
transcrição. Se o servidor está desligado ou dormindo, recusa a chave,
responde erro ou passa do tempo, o áudio é transcrito no próprio computador,
**se a transcrição estiver instalada nele**; se não estiver, a ferramenta de
IA recebe um erro dizendo o que falhou. O `whatsapp-mcp transcription status`
e o painel mostram se o servidor responde.

## 5. Deixar rodando

### macOS (launchd)

Um LaunchAgent sobe o servidor quando você entra no Mac e o reinicia se ele
cair. **Ele só roda com o seu usuário logado**, e um Mac dormindo não
responde: num Mac que fica de servidor, em *Ajustes do Sistema › Bateria*
(ou *Economizador de Energia*), impeça que ele durma com a tela desligada.

Salve como
`~/Library/LaunchAgents/com.brorlandi.whatsapp-mcp-transcription.plist`,
trocando `VOCE` pelo seu usuário (`whoami`):

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key><string>com.brorlandi.whatsapp-mcp-transcription</string>
  <key>ProgramArguments</key>
  <array>
    <string>/Users/VOCE/.local/bin/whatsapp-mcp</string>
    <string>transcription</string>
    <string>serve</string>
  </array>
  <!-- Só com o app instalado, para usar o motor e o modelo dele; sem o app, apague este bloco. -->
  <key>EnvironmentVariables</key>
  <dict>
    <key>WHATSAPP_MCP_DATA</key><string>/Users/VOCE/Library/Application Support/WhatsApp MCP</string>
  </dict>
  <key>RunAtLoad</key><true/>
  <key>KeepAlive</key><true/>
  <key>StandardOutPath</key><string>/Users/VOCE/Library/Logs/whatsapp-mcp-transcription.log</string>
  <key>StandardErrorPath</key><string>/Users/VOCE/Library/Logs/whatsapp-mcp-transcription.log</string>
</dict>
</plist>
```

```sh
plutil -lint ~/Library/LaunchAgents/com.brorlandi.whatsapp-mcp-transcription.plist
launchctl bootstrap gui/$(id -u) ~/Library/LaunchAgents/com.brorlandi.whatsapp-mcp-transcription.plist
tail ~/Library/Logs/whatsapp-mcp-transcription.log
```

Para copiar a chave sem mostrá-la: `pbcopy < ~/.whatsapp-mcp/transcription-server.token`
(ou `pbcopy < "$HOME/Library/Application Support/WhatsApp MCP/transcription-server.token"`
com o app).

Reiniciar: `launchctl kickstart -k gui/$(id -u)/com.brorlandi.whatsapp-mcp-transcription`.
Tirar: `launchctl bootout gui/$(id -u)/com.brorlandi.whatsapp-mcp-transcription`
e apague o arquivo.

### Linux (systemd)

Um serviço do seu usuário, sem `sudo`, como o próprio WhatsApp MCP faz. Salve
como `~/.config/systemd/user/whatsapp-mcp-transcription.service`:

```ini
[Unit]
Description=WhatsApp MCP transcription server
After=network-online.target
Wants=network-online.target

[Service]
ExecStart=%h/.local/bin/whatsapp-mcp transcription serve
# Para outra porta, ou só para um proxy neste computador:
# ExecStart=%h/.local/bin/whatsapp-mcp transcription serve --listen 127.0.0.1:47823
Restart=always
RestartSec=5

[Install]
WantedBy=default.target
```

```sh
systemctl --user daemon-reload
systemctl --user enable --now whatsapp-mcp-transcription.service
loginctl enable-linger "$USER"   # roda também sem você logado, e sobe com o computador
systemctl --user status whatsapp-mcp-transcription.service
```

Os logs: `journalctl --user -u whatsapp-mcp-transcription -f`. Reiniciar:
`systemctl --user restart whatsapp-mcp-transcription`. Tirar:
`systemctl --user disable --now whatsapp-mcp-transcription` e apague o
arquivo.

Se houver firewall, libere a porta só para a sua rede, por exemplo com o ufw:
`sudo ufw allow from 192.168.0.0/24 to any port 47823 proto tcp`.

### Docker

A imagem da versão servidor, `ghcr.io/brorlandi/whatsapp-mcp-local`, roda o
servidor de transcrição também. **No Docker ela só usa o processador**, mesmo
num Mac (um contêiner não alcança a GPU do Mac); vale num Linux forte, não
para ganhar velocidade num Mac. Um `compose.yml` só para ela:

```yaml
services:
  transcricao:
    image: ghcr.io/brorlandi/whatsapp-mcp-local:latest
    command: ["transcription", "serve"]
    environment:
      # A imagem aponta TMPDIR para /data/tmp, que só o serviço do WhatsApp MCP cria;
      # sem esta linha, toda transcrição falha com "stat /data/tmp".
      TMPDIR: /tmp
    ports:
      - "47823:47823"            # atrás de um proxy neste computador: "127.0.0.1:47823:47823"
    volumes:
      - transcricao:/data
    healthcheck:
      disable: true              # o healthcheck da imagem é o do WhatsApp MCP, não deste servidor
    restart: unless-stopped

volumes:
  transcricao:
```

```sh
docker compose run --rm transcricao transcription install   # o motor e o modelo, uma vez, no volume
docker compose up -d
docker compose logs transcricao
docker compose exec transcricao cat /data/mcp/transcription-server.token   # a chave
```

O endereço que o log mostra é o de dentro do contêiner; use o do computador
(`http://<ip-do-computador>:47823`).

## Acesso de fora

O servidor fala HTTP puro. O áudio e a chave vão abertos pela rede, então
escolha o caminho conforme onde está quem manda os áudios:

- **Na mesma rede (casa, escritório)**: use o endereço da rede que o `serve`
  mostra, `http://192.168.0.20:47823`. Dê um IP fixo ao computador do servidor
  no roteador (reserva de DHCP), para o endereço não mudar.
- **Em outra rede, por VPN**: com o [Tailscale](https://tailscale.com) (ou
  outra VPN) nos dois computadores, use o endereço da VPN, como
  `http://meu-mac.exemplo.ts.net:47823`. O tráfego vai cifrado pela VPN, e a
  porta não fica aberta na internet. É o caminho mais simples para um
  WhatsApp MCP num servidor alugado mandar áudios para um Mac em casa.
- **Na internet**: só atrás de HTTPS. Deixe o servidor atendendo só no
  próprio computador (`--listen 127.0.0.1:47823`) e ponha um proxy reverso na
  frente. Com o [Caddy](https://caddyserver.com), que cuida do certificado
  sozinho, o `Caddyfile` inteiro é:

  ```
  transcricao.exemplo.com {
      reverse_proxy 127.0.0.1:47823
  }
  ```

  O endereço no WhatsApp MCP passa a ser `https://transcricao.exemplo.com`.
  Com outro proxy (nginx, por exemplo), aumente o limite de tamanho do corpo
  do pedido (`client_max_body_size 64m`) e o tempo de espera da resposta
  (`proxy_read_timeout 600s`), porque um áudio longo num processador leva
  minutos.

Por que não abrir o HTTP puro na internet: qualquer um no caminho lê os seus
áudios e a chave, e com a chave usa o seu computador para transcrever.

## Atualizar

Baixe a linha de comando de novo (o mesmo comando da
[seção 1](#1-instalar-a-linha-de-comando)) e reinicie o servidor
(`launchctl kickstart -k ...`, `systemctl --user restart ...` ou, no Docker,
`docker compose pull && docker compose up -d`). O motor e o modelo ficam; não
precisa rodar o `install` de novo.

**Use pelo menos a 1.9.6.** Até a 1.9.5, trechos de áudios longos se perdiam
na transcrição (de um áudio de 8 minutos, de um terço a metade sumia, sem
aviso), e é o servidor que transcreve: atualizar só o app do outro lado não
resolve. A versão do servidor aparece no `/v1/health` (`version`) e no
`whatsapp-mcp transcription status` do computador que manda os áudios.

## Problemas comuns

| O que aparece | O que fazer |
|---|---|
| `transcription is not installed on this computer` ao iniciar | Rode `whatsapp-mcp transcription install` com a mesma pasta de dados (`WHATSAPP_MCP_DATA`) que o serviço usa |
| `address already in use` | Outro programa (ou outro `serve`) já usa a porta. Pare-o, ou use `--listen 0.0.0.0:<outra-porta>` |
| `the token must have at least 16 characters` | A chave do arquivo ou de `WHATSAPP_MCP_TRANSCRIPTION_TOKEN` é curta demais |
| O WhatsApp MCP diz que o servidor recusou a chave | A chave informada não é a do arquivo que o servidor usa. Confira se o serviço não usa outra pasta de dados, ou `WHATSAPP_MCP_TRANSCRIPTION_TOKEN` |
| O WhatsApp MCP diz que o servidor não responde | Do computador que manda os áudios: `curl -s -H "Authorization: Bearer $CHAVE" http://<servidor>:47823/v1/health`. Sem resposta: o servidor está de pé? O Mac dormiu? O firewall do macOS (ou o ufw) bloqueia a porta? O `--listen` é `127.0.0.1` (só local)? |
| O app avisa que o endereço não é de um servidor de transcrição | O endereço leva a outro programa. Confira a porta (47823) e o caminho (sem caminho, ou terminando em `/v1`) |
| No Docker: `stat /data/tmp: no such file or directory` | Falta `TMPDIR: /tmp` no `compose.yml` (acima) |
| A transcrição está lenta | Veja a aceleração no `/v1/health` (`accel`): `cpu` quer dizer sem GPU, e um áudio leva mais ou menos o próprio tempo |
| Áudios longos saem cortados | O servidor está numa versão anterior à 1.9.6: [atualize](#atualizar) |

Os logs: no macOS, `~/Library/Logs/whatsapp-mcp-transcription.log` (com o
LaunchAgent acima); no Linux, `journalctl --user -u whatsapp-mcp-transcription`;
no Docker, `docker compose logs transcricao`. O servidor registra pouco além do
início; os detalhes de uma falha voltam na resposta, e o WhatsApp MCP mostra a
última no `whatsapp-mcp transcription status` e no painel.

## Alternativas

O WhatsApp MCP aceita qualquer servidor que fale a API de transcrição da
OpenAI, então também dá para usar outros programas no lugar deste. O deste
projeto aceita o áudio do WhatsApp como vem, aplica o idioma e o contexto do
mesmo jeito que a transcrição local e exige uma chave. Os outros:

- **[whisper.cpp](https://github.com/ggml-org/whisper.cpp)**, o
  `whisper-server`: rode com
  `--inference-path /v1/audio/transcriptions --convert -l auto` (o
  `--convert` para aceitar o `.ogg` do WhatsApp, com o `ffmpeg` do sistema; o
  `-l auto` porque, sem ele, assume inglês). Compilado com CUDA, usa uma placa
  NVIDIA. Não pede chave: deixe-o atrás de uma VPN ou de um proxy que peça.
- **[Speaches](https://github.com/speaches-ai/speaches)**, com o
  faster-whisper, em Docker, com imagens para GPU NVIDIA.
- **[LocalAI](https://github.com/mudler/LocalAI)**, um servidor de vários
  modelos com a API da OpenAI, transcrição incluída.
- **[WhisperKit](https://github.com/argmaxinc/argmax-oss-swift)**, da Argmax,
  feito para os chips Apple, com um servidor local compatível com a OpenAI.

Confira na documentação de cada um como ligar a API da OpenAI e a chave. O
endereço que vai no WhatsApp MCP é o da raiz do servidor ou o que termina em
`/v1`.
