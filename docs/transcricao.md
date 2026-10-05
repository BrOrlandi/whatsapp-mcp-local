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

No processador, um áudio leva mais ou menos o próprio tempo, e a página
Transcrição avisa disso. O primeiro uso depois de ligar o computador leva
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
| `source` | `local`, `corrected` (revisada) ou `client` (feita pela própria ferramenta de IA); transcrições antigas podem ter `openai` |
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
daemon guarda a correção e mantém o original em `raw_text`. A página
Transcrição mostra quantos áudios foram corrigidos assim.

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

## Sem o motor local

A primeira versão do app transcreve só no computador. Sem o motor instalado, a
tool `transcribe_audio` devolve o passo a passo para instalar, e a ferramenta de
IA pode baixar o áudio com `download_media` (o arquivo fica no disco, com o
caminho na resposta) e transcrever do jeito que preferir. A opção de usar uma
chave da OpenAI, útil em computadores sem GPU, volta numa versão futura.
