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
  4. baixa o áudio            wacli --read-only media download → ~/.whatsapp-mcp-v2/media
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
que veio na mensagem e guardado em `~/.whatsapp-mcp-v2/media/<conversa>/`. Um
áudio baixado uma vez não é baixado de novo.

O servidor de mídia do WhatsApp apaga arquivos antigos. Um áudio de meses atrás
pode não estar mais lá: nesse caso o erro explica isso, e
`wacli media retry --chat <jid>` pede ao celular que envie de novo.

### 5 e 6. O motor local

| Etapa | Programa | O que faz |
|---|---|---|
| conversão | `ffmpeg` | transforma o Ogg Opus do WhatsApp em WAV 16 kHz mono, o formato que o whisper.cpp lê |
| transcrição | `whisper-cli` (whisper.cpp) | roda o modelo na GPU do Mac (Metal), com `-l auto` (ou o idioma pedido), `--prompt` com o contexto e metade dos núcleos do processador |
| modelo | `ggml-large-v3-turbo-q5_0.bin` | Whisper large-v3-turbo quantizado, 574 MB, em `~/.whatsapp-mcp-v2/models/` |

Um áudio de cada vez: a GPU é compartilhada, então um segundo pedido espera o
primeiro terminar. Num M3 Pro, um áudio de 6 segundos leva cerca de 2 segundos,
e um de 3 minutos, uns 15. O primeiro uso depois de ligar o computador leva
alguns segundos a mais para carregar o modelo.

**Por que Whisper e não Parakeet V3.** O Handy oferece os dois. O Parakeet é
mais rápido no processador, mas não aceita texto de contexto, e o contexto é o
que corrige nomes e termos já na transcrição. O large-v3-turbo é o melhor
equilíbrio entre velocidade e qualidade em português nesse hardware. Outros
modelos Whisper (`small`, `medium`, `large-v3`) rodariam pelo mesmo caminho
trocando só o arquivo, mas o app hoje usa apenas o turbo.

### 7. O que fica guardado

Em `~/.whatsapp-mcp-v2/state.db`, tabela `transcripts`, uma linha por áudio:

| Campo | Conteúdo |
|---|---|
| `text` | a transcrição atual (corrigida, se houve correção) |
| `raw_text` | o que o motor ouviu, preservado quando há correção |
| `source` | `local`, `openai`, `corrected` (revisada) ou `client` (feita pela própria ferramenta de IA) |
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

Em Macs com Apple Silicon, o `install.sh` roda
`whatsapp-mcp-v2 transcription install`, que:

1. instala `whisper.cpp` e `ffmpeg` pelo Homebrew, se faltarem;
2. baixa o modelo do Hugging Face
   (`huggingface.co/ggerganov/whisper.cpp`), confere o tamanho e só então o
   coloca no lugar.

A página **Transcrição** do painel faz o mesmo com um botão e mostra o
progresso. O motor está pronto quando os três existem: `whisper-cli`, `ffmpeg`
e o modelo.

## Sem o motor local

Fora de um Mac com Apple Silicon, ou com `engine: "openai"`, a transcrição usa
a API da OpenAI (`whisper-1`) com a chave salva na página Transcrição ou pela
tool `set_transcription_key`. O áudio é enviado para a OpenAI e cobrado na conta
da chave (US$ 0,006 por minuto). Sem motor local e sem chave, a tool devolve o
passo a passo para configurar um dos dois.
