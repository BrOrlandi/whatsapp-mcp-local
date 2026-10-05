# Fase 0 · Provas técnicas

> Feitas em 4 de outubro de 2026, num Mac com Apple Silicon (macOS 26.5), com
> Docker (OrbStack) para o Linux. Não havia máquina Windows: o que depende
> dela está marcado como não testado.

## Resumo

| Pergunta | Resultado | Muda o plano? |
|---|---|---|
| Wails v3 nos três sistemas | macOS: funciona. Windows: compila daqui, sem cgo. Linux: precisa de cgo e WebKitGTK, então compila no Docker ou no CI | Sim, em dois pontos (abaixo) |
| wacli no Windows | Não testado | Não, mas a Fase 3 começa por isso |
| Transcrição fora do Mac | Funciona no Linux. Em CPU, o turbo leva cerca de 1,15 vez a duração do áudio | Sim: o whisper do Linux passa a ser compilado por nós |
| Bridge `.exe` com o Claude Desktop | Não testado | Não |

## 1. Wails v3 (beta.27)

- **Janela com o painel em memória (macOS):** funciona. A janela carrega
  `wails://localhost/` e o Wails entrega cada requisição a um `http.Handler`
  comum, o mesmo do painel.
- **Cabeçalhos que o webview manda:** nenhum `Sec-Fetch-Site`; um `fetch` com
  POST não manda `Origin` (só `Referer: wails://localhost/`); um formulário
  manda `Origin: wails://localhost`. A checagem de mesma origem do painel
  recusaria as chamadas da própria janela.
  **Ajuste:** o handler em memória marca a requisição como interna, e o painel
  aceita o que vem por ele. Nada fora da janela alcança esse caminho, porque
  ele não passa pela rede. O servidor em `127.0.0.1` continua com a checagem
  de sempre.
- **Windows:** `GOOS=windows CGO_ENABLED=0` gera um `.exe` de 16 MB a partir do
  Mac. Não foi executado.
- **Linux:** o Wails exige cgo com `libwebkit2gtk-4.1-dev`. Compilação no
  Docker e no CI (Fase 4).
- **O Wails já traz** bandeja, instância única, notificações e "iniciar com o
  sistema" (SMAppService no macOS 13+, LaunchAgent antes disso, `HKCU\…\Run`
  no Windows, `~/.config/autostart` no Linux).
  **Ajuste:** o `internal/platform/autostart_*` do plano fica com o Wails.
  Um detalhe: o `SMAppService.mainApp` não aceita argumentos, então no macOS o
  `--hidden` não chega. O app descobre se foi aberto como item de início pelo
  evento de abertura do sistema (`keyAELaunchedAsLogInItem`).
- **O Wails também traz um atualizador**, mas ele troca só o executável (ou o
  `.app` a partir de um zip). Não serve para o Windows, onde o app, o bridge e
  o wacli precisam ser trocados juntos, nem para o AppImage.
  Fica o `internal/updater` próprio, como o plano previa.

## 2. wacli no Windows

Não testado. O que ficou pronto para testar na Fase 3:

- parar com elegância por `CTRL_BREAK`: o app, que não tem console, se anexa
  ao console oculto do wacli pelo tempo do envio;
- um Job Object com `KILL_ON_JOB_CLOSE`, que encerra o wacli se o app morrer;
- os testes do supervisor, do pareamento e da recuperação de órfãos usam
  agora um wacli falso escrito em Go, que compila para Windows, em vez do
  falso em bash e python. Rodam no macOS e no Linux; no Windows, rodam no CI.

Ainda por confirmar numa máquina Windows: o socket Unix de envio do wacli, o
lock do store e se o wacli encerra limpo ao receber `CTRL_BREAK`.

## 3. Transcrição fora do Mac

- **ffmpeg mínimo** (`scripts/sidecars/build-ffmpeg.sh`): 1,7 MB no macOS arm64
  e 2,5 MB no Linux arm64 (estático). Converte a nota de voz Ogg/Opus no WAV de
  16 kHz mono que o whisper lê. Entraram também AAC, MP3 e Vorbis, para os
  áudios que chegam de outros apps; custa poucos KB.
- **Decodificar Opus em Go** e dispensar o ffmpeg: não há decodificador
  completo em Go puro (o `pion/opus` só cobre SILK). Fica o ffmpeg.
- **whisper.cpp oficial** (v1.9.2): publica binários para Ubuntu (x64 e arm64)
  e Windows (x64 CPU, CUDA 11.8 e 12.4). Para macOS, não.
  Os de Ubuntu são dinâmicos e dependem de `libgomp1`, que não vem nas
  instalações mínimas.
  **Ajuste:** o whisper-cli do Linux passa a ser compilado no nosso CI,
  estático e sem OpenMP, como o do macOS. O do Windows continua o oficial.
- **Tempo do large-v3-turbo q5_0 em CPU** (Linux arm64 numa VM, 12 núcleos de
  Apple M): 11,3 s para um áudio de 9,8 s com 12 threads, e 12,7 s com 6. No
  mesmo Mac, com Metal: 1,4 s. Confirma a estimativa do plano: em CPU, o
  tempo do próprio áudio. A transcrição saiu correta nos dois, nomes inclusive.

## 4. Bridge com o Claude Desktop no Windows

Não testado. Uma mudança feita aqui vale para os três sistemas: o bridge relê
a porta no `config.json` quando o app não responde, então uma porta trocada
no app passa a valer sem reabrir o Claude Desktop.
