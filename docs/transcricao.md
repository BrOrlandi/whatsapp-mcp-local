# Transcrição de áudios

Nada é transcrito sem pedido. Um áudio só passa pelo motor de transcrição quando
a sua ferramenta de IA chama a tool `transcribe_audio`, normalmente porque você
pediu para ela ler um áudio. A partir daí a transcrição fica guardada e volta
junto do áudio nas outras tools e no painel.

## O caminho de um áudio

```
você: "o que a Ana disse no áudio?"
  │
  ▼
Claude ──transcribe_audio(message_id)──► daemon
                                           │
  1. acha a mensagem          wacli.db (somente leitura): é um áudio?
  2. já transcrito?           state.db: se sim, devolve na hora
  3. junta o contexto         nome da conversa, quem fala, 12 mensagens antes e 4 depois
  4. baixa o áudio            wacli --read-only media download → <pasta de dados>/media
  5. converte                 ffmpeg: Ogg Opus → WAV 16 kHz mono
  6. transcreve               whisper-cli + large-v3-turbo, com o contexto em --prompt
  7. guarda                   state.db: texto, motor, idioma, data
  │
  ▼
Claude ◄── transcrição + mensagens de contexto + instrução de revisão
  │
  └─ se o contexto mostrar uma palavra mal ouvida ──save_transcript(texto corrigido)──► state.db
                                                     (o texto original fica em raw_text)
```

### 1 e 2. A mensagem e o que já existe

O id vem das tools de leitura. O daemon confere no índice do wacli que é um
áudio (`media_type = audio`). Se já existe uma transcrição guardada, ela volta
na hora, sem rodar nada, a menos que a chamada peça `refresh`.

### 3. O contexto

O contexto é o que torna a transcrição boa em nomes e termos. Ele é montado a
partir do índice:

- o **nome da conversa** e de **quem mandou o áudio**, resolvidos como o
  celular mostra (grupo, agenda, nome de perfil);
- as **pessoas** que aparecem nas mensagens ao redor;
- as **12 mensagens de texto anteriores** e as **4 seguintes**.

Ele é usado duas vezes. Primeiro, vira o `--prompt` do Whisper: os nomes vêm
antes e depois as últimas frases, até cerca de 800 caracteres. O Whisper usa
esse texto para decidir como escrever o que ouve, então "Capuava" sai como a
conversa escreve e não como soa. Depois, volta junto da transcrição para o
Claude revisar.

### 4. O áudio

O áudio é baixado com `wacli --read-only media download`. O modo somente
leitura não pega o lock do WhatsApp, então roda com o sync ligado. O arquivo é
baixado criptografado do servidor de mídia do WhatsApp, decifrado com a chave
que veio na mensagem e guardado em `media/<conversa>/`, na pasta de dados. Um
áudio baixado uma vez não é baixado de novo.

O servidor de mídia do WhatsApp apaga arquivos antigos. Um áudio de meses atrás
pode não estar mais lá: nesse caso o erro explica isso, e
`wacli media retry --chat <jid>` pede ao celular que envie de novo.

### 5 e 6. O motor local

| Etapa | Programa | O que faz |
|---|---|---|
| conversão | `ffmpeg` | transforma o Ogg Opus do WhatsApp em WAV 16 kHz mono, o formato que o whisper.cpp lê |
| transcrição | `whisper-cli` (whisper.cpp) | roda o modelo com `-l auto` (ou o idioma pedido) e `--prompt` com o contexto: na GPU do Mac (Metal), numa placa NVIDIA (CUDA, no Windows) ou no processador, com todos os núcleos menos um |
| modelo | `ggml-large-v3-turbo-q5_0.bin` | Whisper large-v3-turbo quantizado, 574 MB, em `models/` na pasta de dados |

Um áudio de cada vez: a GPU é compartilhada, então um segundo pedido espera o
primeiro terminar.

| Onde roda | Um áudio de 10 segundos |
|---|---|
| Mac com Apple Silicon (M3 Pro, Metal) | cerca de 2 segundos |
| Só processador (Linux arm64, 12 núcleos) | cerca de 13 segundos |

No processador, um áudio leva mais ou menos o próprio tempo, e a seção
Transcrição de áudio das Configurações avisa disso. O primeiro uso depois de ligar o computador leva
alguns segundos a mais para carregar o modelo.

**Por que Whisper e não Parakeet V3.** O Handy oferece os dois. O Parakeet é
mais rápido no processador, mas não aceita texto de contexto, e o contexto é o
que corrige nomes e termos já na transcrição. O large-v3-turbo é o melhor
equilíbrio entre velocidade e qualidade em português nesse hardware. Outros
modelos Whisper (`small`, `medium`, `large-v3`) rodariam pelo mesmo caminho
trocando só o arquivo, mas o app hoje usa apenas o turbo.

### 7. O que fica guardado

No `state.db` da pasta de dados, tabela `transcripts`, uma linha por áudio:

| Campo | Conteúdo |
|---|---|
| `text` | a transcrição atual (corrigida, se houve correção) |
| `raw_text` | o que o motor ouviu, preservado quando há correção |
| `source` | `local`, `remote` (outro computador ou servidor), `openai`, `corrected` (revisada) ou `client` (feita pela própria ferramenta de IA) |
| `model` | o motor que gerou |
| `language`, `created_at` | idioma pedido e data |

A partir daí, `get_chat_messages` e `search_messages` devolvem o campo
`transcript` junto do áudio, e a busca também encontra palavras faladas. No
painel, a transcrição aparece embaixo do áudio na prévia da conversa.

### A revisão pelo Claude

A tool devolve a transcrição com as mensagens de contexto e uma instrução: onde
uma palavra for claramente um erro de audição de algo que a conversa menciona
(nome, lugar, produto, termo), corrigir com `save_transcript`, mudando só o que
o contexto deixa certo, sem trocar o jeito de falar nem acrescentar nada. O
daemon guarda a correção e mantém o original em `raw_text`. A seção
Transcrição de áudio das Configurações mostra quantos áudios foram corrigidos
assim.

O daemon não roda um modelo de linguagem próprio para isso: quem corrige é a
ferramenta de IA que pediu a transcrição, que já está no meio da conversa.

## Instalação do motor

Nada da transcrição vem no instalador do app. Quando você clica em **Instalar a
transcrição local** (ou roda `whatsapp-mcp transcription install`):

1. o app baixa o pacote do seu sistema, publicado neste repositório pelo
   workflow `sidecars`: o `whisper-cli` e um `ffmpeg` mínimo, que só sabe
   converter os formatos de áudio do WhatsApp em WAV (cerca de 2 MB, em vez dos
   ~80 MB de um ffmpeg completo);
2. confere o sha256 do pacote contra o fixado em `internal/sidecar/manifest.json`,
   desempacota em `bin/` na pasta de dados e guarda o sha256 de cada programa,
   conferido de novo antes de cada uso; no macOS, confere também a assinatura;
3. baixa o modelo do Hugging Face (`huggingface.co/ggerganov/whisper.cpp`) e
   confere o sha256 dele antes de colocá-lo no lugar.

| Sistema | whisper-cli | Aceleração |
|---|---|---|
| macOS (universal) | compilado pelo projeto | Metal, na GPU |
| Windows | o build oficial do whisper.cpp | CPU; CUDA quando há placa NVIDIA |
| Linux (amd64 e arm64) | compilado pelo projeto, estático | CPU |

Se o `whisper-cli` e o `ffmpeg` já estiverem instalados no computador (pelo
Homebrew, por exemplo), o app usa esses e só baixa o modelo. O motor está
pronto quando os três existem.

## Os três modos

| Modo | Onde o áudio vira texto | Configuração |
|---|---|---|
| `local` (padrão) | neste computador, com o whisper.cpp acima | `whatsapp-mcp transcription install` |
| `remote` | num servidor compatível com a API de transcrição da OpenAI, no endereço que você informar: o `whatsapp-mcp transcription serve` num Mac (abaixo), o `whisper-server` do whisper.cpp, o speaches… | endereço, chave opcional, modelo opcional (padrão `whisper-1`) |
| `openai` | na API da OpenAI (`https://api.openai.com/v1`) | chave da API, modelo (`whisper-1` padrão, `gpt-4o-transcribe`, `gpt-4o-mini-transcribe`) |

`remote` e `openai` são o mesmo cliente: só mudam o endereço, a chave e o
modelo. O app manda `POST <base>/audio/transcriptions` em `multipart/form-data`
com `file` (o Ogg Opus do WhatsApp, com nome `.ogg`), `model`,
`response_format=json`, `language` (só quando a chamada pede um idioma; sem
ele, o servidor detecta) e `prompt` (o contexto da conversa), com
`Authorization: Bearer <chave>` quando há chave, e lê `text` da resposta. A
`<base>` é o endereço informado; um endereço sem caminho (`http://mac:47823`) é
a raiz do servidor, e a base vira `http://mac:47823/v1`. A OpenAI aceita
arquivos de até 25 MB.

No modo `openai`, o áudio **e o contexto da conversa** (nomes e as mensagens
ao redor) vão para a OpenAI, que cobra por minuto de áudio. Os outros dois
modos não mandam nada para a internet.

**Onde se escolhe**, do que vence para o que perde:

1. as variáveis de ambiente, que é como o modo servidor (Docker) configura:

   | Variável | O quê |
   |---|---|
   | `WHATSAPP_MCP_TRANSCRIPTION_MODE` | `local`, `remote` ou `openai`; sem ela, basta `_URL` para ser `remote` |
   | `WHATSAPP_MCP_TRANSCRIPTION_URL` | o endereço do servidor (modo `remote`) |
   | `WHATSAPP_MCP_TRANSCRIPTION_TOKEN` | a chave do servidor (modo `remote`) |
   | `WHATSAPP_MCP_TRANSCRIPTION_MODEL` | o modelo pedido (`remote` e `openai`; padrão `whisper-1`) |
   | `WHATSAPP_MCP_TRANSCRIPTION_OPENAI_KEY` | a chave da API da OpenAI (modo `openai`; sem ela, vale `OPENAI_API_KEY`) |

   Quando `_MODE` ou `_URL` está definida, as variáveis mandam e o painel só
   mostra a configuração, sem deixar trocar;
2. o painel, em *Configurações › Transcrição de áudio › Onde transcrever*, ou a
   linha de comando (`whatsapp-mcp transcription local`, `transcription remote
   <endereço> [--model <modelo>]`, `transcription openai [--model <modelo>]`),
   que guardam no `config.json`, em `transcription` (`mode`, `remote_url`,
   `remote_token`, `remote_model`, `openai_key`, `openai_model`). Cada modo
   guarda o seu, então voltar para um modo não pede tudo de novo. Um
   `config.json` da 1.9.0-beta, só com `remote_url`, vale como `remote`.

Segredos nunca entram em argumento de linha de comando, que outros usuários
do computador conseguem ver, nem em log: a linha de comando lê a chave da
entrada padrão (digitada sem aparecer, ou de um arquivo com `<`), o painel
nunca a mostra de volta, e no Docker ela vem de variável de ambiente. O
`config.json` só pode ser lido pelo dono.

**Quando o servidor ou a OpenAI falha**: a conexão tem 3 s para abrir, e a
transcrição lá tem até 10 min. Se a conexão não abre (o Mac dorme ou está
desligado), a chave é recusada, o servidor responde erro ou passa do tempo, o
áudio é transcrito neste computador, se a transcrição estiver instalada aqui;
se não estiver, o erro diz o que falhou. A resposta de `transcribe_audio` diz
`backend` (`remote`, `openai` ou `local`) e, quando caiu para cá, `fallback`
com o motivo. A transcrição guardada diz em `source` qual motor a fez
(`remote`, `openai` ou `local`) e em `model` o nome dele (por exemplo
`OpenAI whisper-1` ou `whisper.cpp large-v3-turbo (transcription server
192.168.0.10:47823)`).

**Onde ver**: `whatsapp_status` mostra `transcription.mode`,
`transcription.backend` (`remote`, `openai`, cada um com `+local` quando este
computador cobre as falhas, `local` ou `none`) e `transcription.remote` (se
responde, o modelo, a aceleração e a versão quando o servidor diz, o último
erro); `health` ganha a checagem `transcription`, que descreve o destino e só
fica em aviso quando ele não responde e não há transcrição neste computador
para cobrir; `whatsapp-mcp transcription status` testa na hora. A checagem é
um `GET <base>/models`: responder (mesmo 404, como o `whisper-server`, que não
lista modelos) quer dizer que o servidor está de pé e aceitou a chave; no modo
`openai`, ela confere também que a chave pode usar o modelo escolhido.

## Servidor de transcrição

Um computador sem GPU leva minutos num áudio de poucos minutos (num servidor
com 4 núcleos fracos, mais de 300 s para 2 a 3 min de fala). O
`whatsapp-mcp transcription serve` deixa outro computador, um Mac com chip
Apple, fazer esse trabalho pela rede: é o mesmo motor (o `whisper-cli`, o
`ffmpeg` e o modelo que a transcrição local já instalou ali), atrás de um
servidor HTTP com chave que fala a API de transcrição da OpenAI. O passo a
passo para subir no Mac, com o launchd, está no
[README](../README.md#transcrever-em-outro-computador).

Por que o próprio app, e não só o `whisper-server` do whisper.cpp: o Mac que
já tem o app tem também o motor, conferido por sha256, e o modelo; o servidor
do app recebe o áudio do WhatsApp como veio (Ogg Opus) e converte lá, aplica o
contexto e o idioma do mesmo jeito que a transcrição local, e exige uma chave,
o que o `whisper-server` não faz. Como fala o protocolo da OpenAI, o
`whisper-server` serve no lugar dele quando se quer (rodado com
`--inference-path /v1/audio/transcriptions --convert`, o `--convert` para ele
aceitar Ogg com o `ffmpeg` do sistema), e qualquer cliente da OpenAI (o SDK
oficial, por exemplo, com `base_url=http://<mac>:47823/v1`) usa o Mac.

O protocolo:

| Pedido | Resposta |
|---|---|
| `POST /v1/audio/transcriptions`, `multipart/form-data` com `file`, `model` (qualquer valor), `language`, `prompt`, `response_format` | `json` (padrão): `{"text"}`; `text`: o texto puro; `verbose_json`: `{"text", "language", "duration"}`. Os três levam também `engine` |
| `GET /v1/models` | a lista da OpenAI (`whisper-1` e `whisper-large-v3-turbo`, vazia quando o motor não está instalado), mais `ready`, `accel`, `engine` e `version`: é a checagem que o painel e o `whatsapp_status` usam |
| `GET /v1/health` | `{"ready", "accel", "engine", "version"}`, para quem monitora |

Erros vêm no formato da OpenAI, `{"error": {"message", "type", "code"}}`: `401`
com `invalid_api_key` para chave errada, `400` para um pedido sem `file` ou com
outro `response_format`, `413` para um arquivo grande demais, `503` quando o
motor não está instalado. Toda requisição leva `Authorization: Bearer
<chave>`, comparada em tempo constante. A chave vem de
`WHATSAPP_MCP_TRANSCRIPTION_TOKEN` ou do arquivo de `--token-file` (padrão:
`transcription-server.token` na pasta de dados), criado na primeira vez, com
permissão só para o dono; o servidor nunca a imprime, só diz em que arquivo
está. O áudio vai sem TLS pela rede local: use numa rede de confiança (ou por
uma VPN como o Tailscale). O endpoint antigo da 1.9.0-beta (`/v1/transcribe`,
com o campo `audio`) não existe mais.

## Uma transcrição longa não se perde

Os clientes de MCP desistem de uma chamada depois de alguns minutos (5 min é
comum). Antes, a transcrição era morta quando o cliente desistia, e a próxima
tentativa recomeçava do zero, sem nunca terminar num computador lento. Agora:

- a transcrição roda fora da chamada, com limite próprio (o motor tem até 1 h
  depois que pega a vez), e guarda o texto quando termina;
- uma chamada para a mesma mensagem enquanto ela roda se junta a ela, em vez de
  começar outra; depois que termina, a chamada acha o texto guardado;
- depois de 240 s (ou um pouco antes do prazo da chamada, se for menor; a
  variável `WHATSAPP_MCP_TRANSCRIPTION_WAIT`, em segundos, muda esse limite
  para clientes que desistem antes) a resposta é `status: in_progress` com
  `retry_after_seconds`, e não um erro: a ferramenta de IA chama de novo e
  recebe o texto;
- a fila do motor (um áudio por vez) espera a vez respeitando o prazo de quem
  pediu, e áudios diferentes não ficam presos atrás de uma chamada.

## Sem o motor local

Sem o motor instalado e sem um servidor de transcrição ou a OpenAI, a tool
`transcribe_audio` devolve o passo a passo para instalar ou apontar para um
servidor, e a ferramenta de
IA pode baixar o áudio com `download_media` (o arquivo fica no disco, com o
caminho na resposta) e transcrever do jeito que preferir.
