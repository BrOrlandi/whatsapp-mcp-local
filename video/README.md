# O vídeo da landing page

O vídeo "Veja como funciona" de https://whatsapp-mcp.brorlandi.xyz, feito com
[Remotion](https://www.remotion.dev): animação em React, narração do Gemini TTS
e legenda gravada na imagem. O que está no site fica em
`site/assets/video/como-funciona.mp4`, com o pôster em `como-funciona.jpg`.

## As versões

| Versão | Duração | Roteiro | Composição |
| --- | --- | --- | --- |
| `longo` | 2:34 | `narration/longo.json` | `longo` |
| `curto` | 1:06 | `narration/curto.json` | `curto` |

As duas contam a mesma história com as mesmas cenas. A curta tem o roteiro
enxuto, a voz acelerada (`"tempo": 1.4`) e cenas com menos respiro. O ritmo de
cada uma e as palavras em que as animações disparam estão em `src/versions.ts`.

## A voz

As duas versões usam a mesma voz, que é a escolhida para os vídeos do projeto:

| | |
| --- | --- |
| Serviço | Gemini TTS, pela API do Google AI Studio (`GEMINI_API_KEY`) |
| Modelo | `gemini-3.1-flash-tts-preview` |
| Voz | `Achernar` (no catálogo do Google, "Soft"; feminina) |
| Idioma | português do Brasil, pelo próprio texto |
| Instrução de estilo | `Say in a soft, calm and warm voice, as a Brazilian Portuguese video narrator, at a natural pace` |

A instrução vai na frente de cada fala, separada por dois-pontos
(`<instrução>: <fala>`), e é o que dá a entonação suave. Ela está no topo de
cada roteiro, junto com o modelo e a voz.

O TTS não repete a mesma gravação: com a mesma voz e a mesma instrução, o
timbre é o mesmo, mas cada geração sai com uma leitura um pouco diferente. Por
isso as gravações usadas ficam no repositório: `public/narration/<versão>/`
(o que toca no vídeo) e, na versão curta, `narration/takes/curto/` (a leitura
original, antes de acelerar). Mudar a velocidade só estica de novo essas
gravações; mudar uma fala gera só aquela fala de novo.

Para acelerar, a versão curta usa o `atempo` do ffmpeg, que mantém o tom da
voz. Pedir ao modelo para falar mais rápido mudaria a entonação.

Outros testes, para referência: o `gemini-3.8-flash-tts` lê a instrução de
estilo em voz alta (não aceita instrução), e o `gemini-2.5-pro-preview-tts` fala
mais rápido e menos suave. As vozes `Vindemiatrix`, `Despina` e `Sulafat` também
foram ouvidas; ficou a `Achernar`.

## Como ele é montado

A narração manda no vídeo. Cada roteiro tem as falas, uma por trecho, cada uma
presa a uma cena. `scripts/narrate.py` gera o áudio de cada fala, corta o
silêncio das pontas, acelera se a versão pedir e transcreve de volta com o
whisper.cpp para saber quando cada palavra é dita; o resultado vai para
`src/narration/<versão>.json`. Cada cena dura o tempo das suas falas
(`src/timeline.ts`), e as animações disparam em deixas, as palavras que
ilustram (`"tools.slack": w("02", "Slack")` em `src/versions.ts`). A legenda
usa o texto do roteiro com esses tempos.

| Cena | Arquivo |
| --- | --- |
| As ferramentas em volta do Claude, o WhatsApp de fora | `src/scenes/Ecosystem.tsx` |
| O nome do app | `src/scenes/Brand.tsx` |
| Passo 1: baixar | `src/scenes/Download.tsx` |
| Passo 2: QR code | `src/scenes/Qr.tsx` |
| Passo 3: conectar a IA | `src/scenes/Choose.tsx` |
| O que a IA faz | `src/scenes/Features.tsx` |
| O aviso sobre disparo em massa | `src/scenes/Warning.tsx` |
| Baixar | `src/scenes/Cta.tsx` |

As telas do app em `public/shots/` foram capturadas do painel com dados de
exemplo (`scripts/preview.sh` na raiz, em 2x, tema escuro). Os logos das
ferramentas em `public/logos/` vêm do [SVG Logos](https://github.com/gilbarbara/logos)
(CC0). A música de fundo, `public/music/bed.mp3`, foi gerada com o Lyria.

## Comandos

Tudo a partir de `video/`, com `npm install` feito uma vez:

```sh
npm run studio                              # abre o editor do Remotion no navegador
GEMINI_API_KEY=... npm run narrate -- curto  # gera só as falas que mudaram
npm run narrate -- curto --force 07         # gera de novo uma fala
npm run render -- curto                     # out/curto.mp4 e out/curto.jpg, para assistir
npm run render -- curto --site              # e põe no site, no lugar do vídeo atual
node scripts/stills.mjs curto out/stills 200 900   # quadros soltos, para conferir
```

O `narrate` precisa do `ffmpeg`, do `whisper-cli` e de um modelo do whisper
(`WHISPER_MODEL`; por padrão, o que o app baixa para transcrever áudios).

Depois de mudar uma fala, confira a cena dela no studio: as animações seguem
as palavras, e uma palavra que saiu do texto quebra a deixa que a usava.
