# Desenvolvimento

Go 1.26 ou mais novo, sem cgo. O wacli precisa estar instalado para o modo de
demonstração.

```sh
go build -o bin/whatsapp-mcp-v2 ./cmd/whatsapp-mcp-v2
go test ./...
```

## Painel com dados de exemplo

```sh
scripts/preview.sh                    # primeira vez: a tela do QR code
PREVIEW_PAIRED=1 scripts/preview.sh   # já pareado, com conversas de exemplo
```

O painel sobe em `http://127.0.0.1:47890` com um wacli falso
(`scripts/preview/fake-wacli`) que repassa as leituras para o wacli de verdade,
sobre um banco preenchido por `scripts/preview/seed.sql`. Só o que depende de
um celular é simulado: pareamento, sync e pedidos de histórico. Com o QR code
na tela, `touch <dir>/store/SCAN` finge que o celular leu o código.

## Testes

Os testes do supervisor usam outro wacli falso (bash + python) que imita o lock
por `flock`, o socket de delegação e o `sync --follow`. Eles cobrem:

- operações exclusivas pausam e religam o sync, e envios concorrentes esperam
  na fila em vez de falhar;
- o pareamento vai do QR code até o sync rodando;
- trocar do QR para o código pelo número não mostra erro;
- depois de conectado, o pareamento passa o histórico para o sync em vez de
  segurar o store.

## Publicar uma versão

Uma tag `v*` roda `.github/workflows/release.yml`, que publica os binários que
o `install.sh` baixa (macOS e Linux, arm64 e amd64). Sem release publicado, o
instalador compila a partir do código.
