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
  6. transcreve               whisper-cli + large-v3-turbo, com o que o nível de contexto deixa em --prompt
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

Ele é usado duas vezes. Primeiro, parte dele vai com o áudio para quem
transcreve, como o `--prompt` do Whisper (ou o campo `prompt` da API, num
servidor ou na OpenAI): o Whisper usa esse texto para decidir como escrever o
que ouve, então "Capuava" sai como a conversa escreve e não como soa. Quanto
vai depende do **nível de contexto** (abaixo). Depois, o contexto inteiro volta
junto da transcrição para a ferramenta de IA revisar: isso fica no computador,
entre o app e a ferramenta, em qualquer nível.

#### O nível de contexto

Em Configurações › Transcrição de áudio › **Contexto da conversa** (desde a
1.9.6), com três níveis:

| Nível | Valor | O que vai com o áudio |
|---|---|---|
| Nada | `none` | só o áudio |
| Só os nomes | `names` | o nome da conversa, de quem mandou e das pessoas que falam nas mensagens ao redor: até 6 nomes, cada um cortado em 4 palavras e 40 letras, sem números de telefone, até 200 caracteres no total. Nenhum texto de mensagem |
| Nomes e mensagens recentes | `messages` | os nomes e as últimas mensagens de texto antes do áudio, até cerca de 600 caracteres delas e 800 no total (o que a 1.9.5 sempre mandava) |

Sem escolha, o nível segue o modo: **Só os nomes** quando a transcrição é neste
computador (nada sai dele de qualquer jeito) e **Nada** num servidor
compatível ou na OpenAI, porque lá esse texto sai do computador. O painel diz
o nível em uso no card e avisa quando o texto sai do computador. Trocar para
um servidor ou para a OpenAI, ou para outro endereço de servidor, pelo painel
ou pela linha de comando, volta ao padrão (Nada), para um nível escolhido
para outro lugar não ir junto sem a pessoa ver; escolher no painel o próprio
padrão do modo também guarda "seguir o modo". Quando o modo vem de
`WHATSAPP_MCP_TRANSCRIPTION_MODE`, essa volta não acontece (o modo não passa
pelo painel): aí só o aviso do card mostra que o texto sai.

Por que não mandar tudo: no whisper.cpp (o motor local e qualquer servidor
feito com ele), as mensagens pesam mais que o idioma. Um áudio em espanhol numa
conversa em português sai traduzido para o português, mesmo com
`language=es`; com só os nomes, mesmo um nome de grupo que é uma frase em
português, ele sai em espanhol (conferido no motor local com
`integration/testdata/audio/envio-es.ogg`). A OpenAI não tem esse viés.

Onde fica: `"context"` no bloco `transcription` do `config.json` (`none`,
`names` ou `messages`; sem o campo, segue o modo, e um `config.json` da 1.9.5
continua valendo), `WHATSAPP_MCP_TRANSCRIPTION_CONTEXT` (vale sozinha, ganha do
`config.json` e trava a escolha no painel) e `whatsapp-mcp transcription
context none|names|messages|auto`. Um valor que não se lê conta como não
definido: cai no padrão do modo, nunca em mandar mais.

O `transcribe_audio` diz o nível usado (`context_level` e `context_sent`)
quando transcreve, e o `whatsapp_status` diz o nível em uso
(`transcription.context`). Uma transcrição guardada não muda quando o nível
muda: pedir de novo devolve a mesma, sem transcrever tudo outra vez, e o nível
com que ela foi feita não fica guardado. Para refazer um áudio com o nível
novo, `transcribe_audio` com `refresh`.

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
| transcrição | `whisper-cli` (whisper.cpp) | roda o modelo com `-l auto` (ou o idioma pedido) e `--prompt` com o contexto do nível escolhido: na GPU do Mac (Metal), numa placa NVIDIA (CUDA, no Windows) ou no processador, com todos os núcleos menos um |
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
| `source` | `local`, `remote` (um servidor compatível com a OpenAI), `openai`, `corrected` (revisada) ou `client` (feita pela própria ferramenta de IA) |
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

1. o app baixa o pacote do seu sistema, publicado à mão num release
   `sidecars-N` deste repositório (montado com `scripts/sidecars/`, veja
   [Desenvolvimento](desenvolvimento.md)): o `whisper-cli` e um `ffmpeg`
   mínimo, que só sabe converter os formatos de áudio do WhatsApp em WAV
   (cerca de 2 MB, em vez dos ~80 MB de um ffmpeg completo);
2. confere o sha256 do pacote contra o fixado em `internal/sidecar/manifest.json`,
   desempacota em `bin/` na pasta de dados e guarda o sha256 de cada programa,
   conferido de novo antes de cada uso; no macOS, confere também a assinatura;
3. no Windows, roda o `whisper-cli -h` uma vez (sem janela, com limite de 10
   s) para ver se ele abre, antes de baixar o modelo (abaixo);
4. baixa o modelo do Hugging Face (`huggingface.co/ggerganov/whisper.cpp`) e
   confere o sha256 dele antes de colocá-lo no lugar.

| Sistema | whisper-cli | Aceleração |
|---|---|---|
| macOS (universal) | compilado pelo projeto | Metal, na GPU |
| Windows | o build oficial do whisper.cpp (ainda não funciona num Windows sem o runtime do Visual C++, abaixo) | CPU; CUDA quando há placa NVIDIA |
| Linux (amd64 e arm64) | compilado pelo projeto, estático | CPU |

Se o `whisper-cli` e o `ffmpeg` já estiverem instalados no computador (pelo
Homebrew, por exemplo), o app usa esses e só baixa o modelo. O motor está
pronto quando os três existem e, no Windows, quando o teste abaixo passa.

### Ainda não no Windows

O pacote do Windows usa o build oficial do whisper.cpp, que depende do runtime
do Microsoft Visual C++ 2015 a 2022 (`MSVCP140`, `VCRUNTIME140`, `VCOMP140`).
Um Windows limpo não tem esse runtime, e aí o `whisper-cli.exe` sai com
0xC0000135 (`STATUS_DLL_NOT_FOUND`) antes de escrever qualquer coisa. Muitos
computadores já têm o runtime, instalado por outro programa, e neles a
transcrição local funciona. Refazer o pacote (CRT estático, sem OpenMP, como os
de macOS e Linux) está pedido numa issue no GitHub, aberta para contribuições.

Enquanto isso, desde a 1.9.6:

- **o motor é testado no Windows.** O app roda o `whisper-cli -h` depois de
  baixar o pacote (cerca de 5 MB) e antes do modelo (574 MB), e de novo quando
  as Configurações abrem e no primeiro uso depois que o app abre. O resultado
  fica guardado pelo caminho, tamanho e data do programa; uma falha vale por
  10 minutos ou até as Configurações abrirem de novo, para quem instalar o
  runtime ver a transcrição voltar. Um teste que passa do limite não conta
  como falha. Nos outros sistemas não há teste e nada muda;
- **se ele não abre,** `localasr.Status` traz `ready: false` e `unavailable`
  (`windows-runtime` para 0xC0000135, `engine-failed` para outra falha), com
  `supported` ainda verdadeiro. O card diz **Indisponível** e, em palavras
  simples, que a transcrição neste computador ainda não funciona no Windows,
  apontando para os outros dois modos; o detalhe (o runtime) fica no `(?)` e
  no log. A instalação para antes do modelo, com um erro próprio;
- **as ferramentas de IA também sabem:** `transcribe_audio` responde que a
  transcrição local não está disponível neste computador, com os outros modos
  e o `download_media`; o `whatsapp_status` traz `unavailable` e uma nota; o
  `health` diz "not available" em vez de "not installed". O
  `transcription serve` recusa começar, e o `/v1/health` de um que já rodava
  traz `ready: false` e `unavailable`;
- **nenhum erro vem vazio:** quando o programa não escreve nada, o erro diz
  como ele saiu (o código, e o runtime quando é 0xC0000135).

Quem refizer o pacote do Windows tira esse aviso (o texto do card, do
`transcribe_audio` e do `whatsapp_status`) no mesmo PR.

## Os três modos

| Modo | Onde o áudio vira texto | Configuração |
|---|---|---|
| `local` (padrão) | neste computador, com o whisper.cpp acima | `whatsapp-mcp transcription install` |
| `remote` | em qualquer servidor compatível com a API de transcrição da OpenAI, em qualquer endereço (na internet ou na sua rede): um serviço contratado, o `whisper-server` do whisper.cpp, o speaches, ou o `whatsapp-mcp transcription serve` num computador com GPU (abaixo) | endereço (URL), chave quando o servidor pede, modelo opcional (padrão `whisper-1`) |
| `openai` | na API da OpenAI (`https://api.openai.com/v1`) | chave da API, modelo (`whisper-1` padrão, `gpt-4o-transcribe`, `gpt-4o-mini-transcribe`) |

`remote` e `openai` são o mesmo cliente: só mudam o endereço, a chave e o
modelo. O app manda `POST <base>/audio/transcriptions` em `multipart/form-data`
com `file` (o Ogg Opus do WhatsApp, com nome `.ogg`), `model`,
`response_format=json`, `language` (só quando a chamada pede um idioma; sem
ele, o servidor detecta) e `prompt` (o contexto da conversa, só quando o nível
de contexto não é Nada), com
`Authorization: Bearer <chave>` quando há chave, e lê `text` da resposta. A
`<base>` é o endereço informado; um endereço sem caminho (`http://mac:47823`) é
a raiz do servidor, e a base vira `http://mac:47823/v1`. A OpenAI aceita
arquivos de até 25 MB.

No modo `openai`, o áudio vai para a OpenAI, que cobra por minuto de áudio; no
`remote`, para o servidor informado, onde quer que ele esteja; no `local`, nada
sai do computador. Do contexto da conversa, só vai o que o
[nível de contexto](#o-nível-de-contexto) deixa: por padrão, nos modos
`remote` e `openai`, nada (até a 1.9.5, os nomes e as mensagens ao redor iam
sempre). Se o servidor ou a OpenAI falha e o áudio é transcrito aqui, vale o
mesmo nível.

**Onde se escolhe**, do que vence para o que perde:

1. as variáveis de ambiente, que é como o modo servidor (Docker) configura:

   | Variável | O quê |
   |---|---|
   | `WHATSAPP_MCP_TRANSCRIPTION_MODE` | `local`, `remote` ou `openai`; sem ela, basta `_URL` para ser `remote` |
   | `WHATSAPP_MCP_TRANSCRIPTION_URL` | o endereço do servidor (modo `remote`) |
   | `WHATSAPP_MCP_TRANSCRIPTION_TOKEN` | a chave do servidor (modo `remote`) |
   | `WHATSAPP_MCP_TRANSCRIPTION_MODEL` | o modelo pedido (`remote` e `openai`; padrão `whisper-1`) |
   | `WHATSAPP_MCP_TRANSCRIPTION_OPENAI_KEY` | a chave da API da OpenAI (modo `openai`; sem ela, vale `OPENAI_API_KEY`) |
   | `WHATSAPP_MCP_TRANSCRIPTION_WAIT` | quantos segundos uma chamada espera o texto antes de responder que ainda está transcrevendo (padrão 240; [abaixo](#uma-transcrição-longa-não-se-perde)) |
   | `WHATSAPP_MCP_TRANSCRIPTION_CONTEXT` | o [nível de contexto](#o-nível-de-contexto): `none`, `names` ou `messages`; sem ela, segue o modo |

   Quando `_MODE` ou `_URL` está definida, as variáveis mandam e o painel só
   mostra a configuração, sem deixar trocar; a `_CONTEXT` vale sozinha e trava
   só o Contexto da conversa;
2. o painel, em *Configurações › Transcrição de áudio › Onde transcrever*, ou a
   linha de comando (`whatsapp-mcp transcription local`, `transcription remote
   <endereço> [--model <modelo>]`, `transcription openai [--model <modelo>]`),
   que guardam no `config.json`, em `transcription` (`mode`, `remote_url`,
   `remote_token`, `remote_model`, `openai_key`, `openai_model` e `context`,
   que o painel muda em *Contexto da conversa* e a linha de comando em
   `transcription context`). Cada modo
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
`transcription.context` (o [nível de contexto](#o-nível-de-contexto) em uso),
`transcription.backend` (`remote`, `openai`, cada um com `+local` quando este
computador cobre as falhas, `local` ou `none`) e `transcription.remote` (se
responde, o modelo, a aceleração e a versão quando o servidor diz, o último
erro); `health` ganha a checagem `transcription`, que descreve o destino e só
fica em aviso quando ele não responde e não há transcrição neste computador
para cobrir; `whatsapp-mcp transcription status` testa na hora. A checagem é
um `GET <base>/models`: responder quer dizer que o servidor está de pé e
aceitou a chave. Um servidor que não lista modelos (como o `whisper-server`)
responde 404 ali; então a checagem manda um `POST <base>/audio/transcriptions`
vazio: qualquer resposta que não seja 404 ou 405 (o `whisper-server` reclama da
falta do arquivo) quer dizer que a rota existe, e um 404 ali também quer dizer
que o programa nesse endereço não é um servidor de transcrição (`reason:
not_compatible`, e o painel diz para conferir o endereço). No modo `openai`, a
checagem confere também que a chave pode usar o modelo escolhido.

## Servidor de transcrição

Um computador sem GPU leva minutos num áudio de poucos minutos (num servidor
com 4 núcleos fracos, mais de 300 s para 2 a 3 min de fala). O
`whatsapp-mcp transcription serve` transforma um computador com GPU (um Mac
com chip Apple, por exemplo) num desses servidores compatíveis com a OpenAI,
para o modo `remote` de outra instalação: é o mesmo motor (o `whisper-cli`, o
`ffmpeg` e o modelo que a transcrição local já instalou ali), atrás de um
servidor HTTP com chave que fala a API de transcrição da OpenAI. O passo a
passo para subir no Mac, com o launchd, está no
[README](../README.md#transcrever-num-servidor-compatível-com-a-openai).

Por que o próprio app, e não só o `whisper-server` do whisper.cpp: o Mac que
já tem o app tem também o motor, conferido por sha256, e o modelo; o servidor
do app recebe o áudio do WhatsApp como veio (Ogg Opus) e converte lá, aplica o
contexto e o idioma do mesmo jeito que a transcrição local, e exige uma chave,
o que o `whisper-server` não faz. Como fala o protocolo da OpenAI, o
`whisper-server` serve no lugar dele quando se quer, rodado com
`--inference-path /v1/audio/transcriptions --convert -l auto`: o `--convert`
para ele aceitar Ogg com o `ffmpeg` do sistema, e o `-l auto` porque, sem ele,
o `whisper-server` assume inglês e transcreve um áudio em português como se
fosse inglês; e qualquer cliente da OpenAI (o SDK
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
servidor (num Windows onde o motor não abre, só os outros modos, porque
instalar de novo não resolve), e a ferramenta de
IA pode baixar o áudio com `download_media` (o arquivo fica no disco, com o
caminho na resposta) e transcrever do jeito que preferir.
