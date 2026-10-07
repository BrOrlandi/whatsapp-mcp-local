# O vídeo da landing page

O vídeo "Veja como funciona" de https://whatsapp-mcp.brorlandi.xyz, feito com
[Remotion](https://www.remotion.dev): animação em React, narração do Gemini TTS
e legenda gravada na imagem. Fica em `site/assets/video/como-funciona.mp4`,
com o pôster em `como-funciona.jpg`.

## Como ele é montado

A narração manda no vídeo. `narration/script.json` tem as falas, uma por
trecho, cada uma presa a uma cena. `scripts/narrate.py` gera o áudio de cada
fala no Gemini TTS, corta o silêncio das pontas e transcreve de volta com o
whisper.cpp para saber quando cada palavra é dita; o resultado vai para
`src/narration.json`. Cada cena dura o tempo das suas falas
(`src/timeline.ts`), e as animações disparam nas palavras que ilustram
(`wordAt("02", "Slack")`). A legenda usa o texto do roteiro com esses tempos.

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
npm run studio                      # abre o editor do Remotion no navegador
GEMINI_API_KEY=... npm run narrate  # gera de novo só as falas que mudaram
npm run narrate -- --force 07       # gera de novo uma fala
npm run publish                     # renderiza e põe o vídeo e o pôster no site
node scripts/stills.mjs . out/stills 200 905   # quadros soltos, para conferir
```

O `narrate` precisa do `ffmpeg`, do `whisper-cli` e de um modelo do whisper
(`WHISPER_MODEL`; por padrão, o que o app baixa para transcrever áudios). A
voz, o modelo e o tom estão no topo de `narration/script.json`; trocar a voz
gera todas as falas de novo.

Depois de mudar uma fala, confira a cena dela no studio: as animações seguem
as palavras, e uma palavra que saiu do texto quebra o `wordAt` que a usava.
