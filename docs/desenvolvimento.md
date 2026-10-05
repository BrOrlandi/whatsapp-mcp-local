# Desenvolvimento

Go 1.26 ou mais novo. O app usa cgo no macOS (Xcode) e no Linux (GTK 3 e
WebKitGTK 4.1); a linha de comando e o bridge, não.

```sh
go test ./...
go build -o bin/whatsapp-mcp ./cmd/whatsapp-mcp         # a linha de comando
go build -o bin/whatsapp-mcp-bridge ./cmd/whatsapp-mcp-bridge
```

## Os programas

| Pasta | O que é |
|---|---|
| `cmd/app` | o app (Wails v3): janela, bandeja, ciclo de vida, migração, atualizações |
| `cmd/whatsapp-mcp` | a linha de comando: `serve`, `service install`, `bridge`… |
| `cmd/whatsapp-mcp-bridge` | o stdio que o Claude Desktop inicia, dentro do app |
| `internal/daemon` | o núcleo que o app e a linha de comando rodam: supervisor, índice, MCP, painel, HTTP |
| `internal/platform` | o que muda por sistema: pastas, processos (grupos e sinais; Job Object e `CTRL_BREAK` no Windows) |
| `internal/sidecar` | baixar e conferir os programas da transcrição |
| `internal/updater` | as atualizações do app |

## O app

```sh
scripts/build-macos.sh              # build/bin/WhatsApp MCP.app e WhatsApp-MCP.dmg (universal)
ARCHS=arm64 scripts/build-macos.sh  # só para este Mac, mais rápido
```

Sem `SIGN_IDENTITY`, o app é assinado ad hoc e roda só neste Mac. Com
`SIGN_IDENTITY="Developer ID Application: …"` ele sai assinado, e com
`NOTARY_KEY`, `NOTARY_KEY_ID` e `NOTARY_ISSUER` (uma chave da API do App Store
Connect) o `.dmg` também é notarizado.

O Windows e o Linux compilam em qualquer máquina com Docker:

```sh
# Windows: o instalador NSIS, compilado sem cgo
docker run --rm -v "$PWD:/src" -w /src golang:1.26 bash -c \
  'apt-get update -qq && apt-get install -y -qq nsis unzip && scripts/build-windows.sh'

# Linux: .deb e AppImage, no Ubuntu 22.04 de build/linux/Dockerfile, na arquitetura da máquina
docker build -t wamcp-linux-build build/linux
docker run --rm --privileged -v "$PWD:/src" -w /src wamcp-linux-build scripts/build-linux.sh
```

### Rodar o app com dados de exemplo

Com `WHATSAPP_MCP_DATA` apontando para outra pasta, o app roda separado do que
estiver instalado (inclusive de outra cópia aberta), sem tocar nos dados reais:

```sh
T=$(mktemp -d); mkdir -p $T/store $T/data
WACLI_STORE_DIR=$T/store wacli --json doctor >/dev/null
sqlite3 $T/store/wacli.db < scripts/preview/seed.sql
echo '{"port":47892,"autostart":false,"seen":["first-run"]}' > $T/data/config.json
WHATSAPP_MCP_DATA=$T/data WACLI_STORE_DIR=$T/store WACLI_BIN=$PWD/scripts/preview/fake-wacli \
  "build/bin/WhatsApp MCP.app/Contents/MacOS/WhatsApp MCP"
```

Com o QR code na tela, `touch $T/store/SCAN` finge que o celular leu o código.
O `config.json` com `autostart: false` evita registrar o build de teste para
abrir com o sistema.

## Painel no navegador, com dados de exemplo

```sh
scripts/preview.sh                    # primeira vez: a tela do QR code
PREVIEW_PAIRED=1 scripts/preview.sh   # já pareado, com conversas de exemplo
```

O painel sobe em `http://127.0.0.1:47890` com um wacli falso
(`scripts/preview/fake-wacli`) que repassa as leituras para o wacli de verdade,
sobre um banco preenchido por `scripts/preview/seed.sql`. Só o que depende de
um celular é simulado: pareamento, sync e pedidos de histórico.

## Testes

Os testes do supervisor, do pareamento, da recuperação de órfãos e do núcleo
usam um wacli falso escrito em Go (`internal/wacli/testdata/fakewacli`),
compilado no início dos testes, que imita o lock do store, o socket de
delegação, o `sync --follow` e o `auth`. Por ser Go, roda nos três sistemas, e
o CI (`.github/workflows/ci.yml`) roda tudo no macOS, no Windows e no Linux.
Eles cobrem, entre outros:

- operações exclusivas pausam e religam o sync, e envios concorrentes esperam;
- o pareamento vai do QR code até o sync rodando, e entrega o histórico ao sync;
- parar o núcleo não deixa nenhum wacli rodando;
- trocar a porta abre a nova antes de fechar a antiga, recusa uma porta ocupada
  sem derrubar a atual, e a variável de ambiente vence o `config.json`;
- o bridge segue o app para a porta nova;
- todas as páginas do painel renderizam dentro do app;
- o atualizador recusa um arquivo que não confere com o `checksums.txt` e, no
  macOS, um `.app` com a assinatura quebrada.

A transcrição de ponta a ponta (baixar os pacotes, conferir e transcrever um
áudio) só roda quando pedida, porque executa os programas de verdade:

```sh
WAMCP_E2E_PACKAGES=<pasta com os pacotes e um manifest.json> WAMCP_E2E_AUDIO=voz.ogg \
  WAMCP_E2E_EXPECT=<palavra> WAMCP_E2E_MODEL=<modelo já baixado> \
  go test -run TestInstallAndTranscribe ./internal/localasr
```

Com `WAMCP_E2E_PACKAGES=published`, ele baixa os pacotes fixados em
`internal/sidecar/manifest.json`, do release publicado.

## Publicar

Toda versão é compilada e publicada à mão, neste Mac: o GitHub Actions só roda
os testes. O instalador do Windows não é assinado.

- **Uma versão do app:** com o `CHANGELOG.md` escrito e a tag `vX.Y.Z` criada
  (anotada), `scripts/release.sh X.Y.Z` compila a tag num clone à parte e deixa
  em `dist/vX.Y.Z/` o `.dmg` assinado e notarizado, o instalador do Windows, o
  AppImage e o `.deb` (amd64 e arm64, em Docker), a linha de comando de cada
  sistema e o `checksums.txt`. Ele precisa de `SIGN_IDENTITY`
  (`Developer ID Application: …`), `NOTARY_KEY` (o `.p8` da chave da API do
  App Store Connect), `NOTARY_KEY_ID` e `NOTARY_ISSUER`. Depois:

  ```sh
  git push origin main vX.Y.Z
  gh release create vX.Y.Z --verify-tag --latest --title "WhatsApp MCP Local X.Y.Z" \
    --notes-file <a entrada do CHANGELOG mais os avisos de instalação> -- dist/vX.Y.Z/*
  ```

  e abrir https://whatsapp-mcp.brorlandi.xyz/download para ver a versão nova
  em todos os sistemas.
- **Os programas da transcrição** mudam pouco, e saem num release `sidecars-N`
  à parte, que não é marcado como o mais recente (o atualizador do app não o
  confunde com uma versão). Cada sistema compila com `scripts/sidecars/`
  (`build-whisper.sh` e `build-ffmpeg.sh`; no Mac, precisa do `cmake`):

  ```sh
  # macOS: whisper universal com Metal, ffmpeg de cada arquitetura unido com lipo
  scripts/sidecars/build-whisper.sh out/mac
  ARCH=arm64 scripts/sidecars/build-ffmpeg.sh out/ff-arm64
  ARCH=x86_64 scripts/sidecars/build-ffmpeg.sh out/ff-x86_64
  lipo -create -output out/mac/ffmpeg out/ff-arm64/ffmpeg out/ff-x86_64/ffmpeg
  SIGN_IDENTITY="Developer ID Application: …" scripts/sidecars/package.sh darwin universal out/mac pkg

  # Linux, uma vez por arquitetura (--platform linux/arm64 e linux/amd64)
  docker run --rm --platform linux/$ARCH -v "$PWD:/src" -w /src wamcp-linux-build:$ARCH bash -c \
    'scripts/sidecars/build-whisper.sh out/linux-$ARCH && scripts/sidecars/build-ffmpeg.sh out/linux-$ARCH && scripts/sidecars/package.sh linux $ARCH out/linux-$ARCH pkg'

  # Windows: ffmpeg cruzado com mingw; o whisper vem do release oficial
  docker run --rm -v "$PWD:/src" -w /src wamcp-linux-build:arm64 bash -c \
    'apt-get update -qq && apt-get install -y -qq mingw-w64 && scripts/sidecars/build-ffmpeg.sh out/win x86_64-w64-mingw32- &&
     scripts/sidecars/package.sh windows amd64 out/win pkg && scripts/sidecars/package.sh windows amd64 out/win pkg cuda'

  scripts/sidecars/manifest.py sidecars-N pkg   # fixa os pacotes em internal/sidecar/manifest.json
  gh release create sidecars-N --latest=false --title "Transcrição (sidecars-N)" --notes "…" pkg/*
  ```

  Antes de publicar, `WAMCP_E2E_PACKAGES=pkg` testa a instalação e a
  transcrição a partir deles; depois, `WAMCP_E2E_PACKAGES=published`, a partir
  do release.
- **O wacli:** a versão e os sha256 ficam em `build/wacli.env`. Trocar a versão
  é trocar esse arquivo, rodar os testes e publicar uma versão nova do app.
- **Os ícones** saem de `build/icons/*.svg` com `scripts/icons.sh`.
