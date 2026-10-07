# Dependências

O que o projeto precisa para funcionar, do mais crítico para o menos, e o que
acontece quando cada peça muda ou falta.

## Visão geral

```
                       ┌──────────────── WhatsApp MCP (app Wails, um processo) ─────────┐
Claude Desktop ─stdio─► whatsapp-mcp-bridge ─┐                                          │
Claude Code ───HTTP───┤ MCP + painel ── supervisor ──► wacli ──► whatsmeow ──► WhatsApp │
                       │      │               (dentro do app) │                          │
                       │      │                               ├─ session.db (sessão)     │
                       │      │                               └─ wacli.db   (mensagens)  │
                       │      └── state.db (transcrições, clientes)  config.json (porta) │
                       │      └── transcrição ──► ffmpeg ──► whisper-cli ──► modelo      │
                       │           (baixados quando a transcrição é ativada)             │
                       └──────────────────────────────────────────────────────────────────┘
     janela: WKWebView (macOS) · WebView2 (Windows) · WebKitGTK 4.1 (Linux)
     linha de comando: o mesmo núcleo como serviço, launchd (macOS) · systemd --user (Linux)
```

## Críticas: sem elas nada funciona

### wacli

| | |
|---|---|
| O que é | Cliente de WhatsApp em linha de comando, de [openclaw/wacli](https://github.com/openclaw/wacli), licença MIT |
| Versão em uso | 0.20.0, dentro do app, fixada em `build/wacli.env` com o sha256 de cada sistema |
| Para que serve | Pareia o computador como dispositivo conectado, mantém a sessão, recebe e guarda as mensagens, envia, baixa mídia |

É a dependência mais importante, e o projeto depende de várias partes dela, não
só dos comandos:

| Do que o daemon depende | Onde | Se mudar |
|---|---|---|
| Saída `--json` dos comandos (`{success, data, error}`) | todas as tools | uma tool passa a falhar ao ler a resposta |
| Eventos `--events` do `sync` e do `auth` (`connected`, `qr_code`, `pair_code`, `history_sync`, `progress`…) | supervisor, pareamento, painel | o QR code ou o estado do sync deixam de aparecer |
| O socket `.send.sock` que o `sync --follow` abre para receber envios e mudanças de estado das conversas | supervisor | envios esperam o tempo limite em vez de sair; arquivar e marcar como lida passam a pausar o sync |
| O webhook do `sync` (`--webhook`, `--webhook-secret`, `--webhook-events`, `--webhook-allow-private`) e o formato do que ele posta | relay dos webhooks (`internal/webhook`) | os webhooks param de receber; sem as flags, o sync roda sem elas e a API diz `"available": false` |
| O arquivo `LOCK`, com `pid=` | recuperação depois de uma queda | um sync órfão não é encerrado sozinho |
| O esquema do `wacli.db` (tabelas `messages`, `chats`, `contacts`, `groups`…), lido em modo somente leitura | prévias, nomes, contexto de áudio, saúde | a consulta afetada falha com erro de SQL |
| O esquema do `session.db` do whatsmeow (`whatsmeow_contacts`, `whatsmeow_lid_map`, `whatsmeow_device`), lido só para nomes e para o seu próprio número e LID | nomes de pessoas, o seu próprio nome, as menções | volta a aparecer número em vez de nome; as menções deixam de ser achadas |

No app, o wacli é fixado: cada versão do app traz uma versão testada, baixada
dos releases oficiais e conferida pelo sha256 que eles publicam, e ele só muda
com uma versão nova do app. A documentação do wacli avisa que o esquema do banco
pode mudar entre versões; antes de trocar a versão em `build/wacli.env`, vale
rodar os testes e conferir a aba Status e a prévia de conversas. A linha de
comando usa o wacli instalado ao lado dela (pelo `install.sh`) ou o do
Homebrew.

### whatsmeow

| | |
|---|---|
| O que é | A biblioteca que implementa o protocolo do WhatsApp Web, de [tulir/whatsmeow](https://github.com/tulir/whatsmeow), usada por dentro do wacli |
| Risco | É não oficial. Quando o WhatsApp muda o protocolo, a conexão pode parar até sair uma versão nova do whatsmeow e, depois, do wacli |

O projeto não fala com o whatsmeow diretamente: a correção chega atualizando o
wacli.

### WhatsApp

A conta precisa estar ativa no celular. Um dispositivo conectado que fica
semanas sem se conectar é desconectado pelo WhatsApp, e aí é preciso ler o QR
code de novo. O celular também precisa estar com internet para mandar
histórico antigo e para reenviar mídia que expirou.

### O que abre o app, ou o serviço, no login

| | Como |
|---|---|
| App no macOS 13+ | item de início do sistema (SMAppService); no macOS 11 e 12, um LaunchAgent |
| App no Windows | valor `com.brorlandi.whatsapp-mcp` em `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` |
| App no Linux | `~/.config/autostart/` (padrão XDG: GNOME, KDE, XFCE) |
| Linha de comando no macOS | LaunchAgent `com.brorlandi.whatsapp-mcp`, com `KeepAlive` |
| Linha de comando no Linux | unit `systemd --user` `whatsapp-mcp.service` |

### O webview de cada sistema

| Sistema | Webview | Observação |
|---|---|---|
| macOS 11+ | WKWebView | vem no sistema |
| Windows 10 e 11 | WebView2 (Edge) | vem no Windows 11 e no 10 atualizado; o instalador instala se faltar |
| Linux | WebKitGTK 4.1 com GTK 3 | dependência do `.deb`; o AppImage traz junto |

## Da transcrição: necessárias só para transcrever no computador

| Peça | Versão | Origem | Para que serve |
|---|---|---|---|
| whisper.cpp (`whisper-cli`) | 1.9.2 | compilado pelo projeto (macOS, Linux) ou o build oficial (Windows), publicado num release `sidecars-N` | roda o modelo |
| ffmpeg mínimo | 7.1.1 | compilado pelo projeto (`scripts/sidecars/build-ffmpeg.sh`) | converte o áudio do WhatsApp (Ogg Opus, AAC, MP3) para WAV 16 kHz |
| Modelo `ggml-large-v3-turbo-q5_0.bin` | 574.041.195 bytes | Hugging Face, `ggerganov/whisper.cpp`, conferido por sha256 | o Whisper large-v3-turbo quantizado |

Nada disso vem no instalador: o app baixa quando a transcrição é ativada, e
confere tudo por sha256 (veja [transcricao.md](transcricao.md)). Sem elas, o
resto continua funcionando, e a tool `transcribe_audio` explica como instalar.

Riscos: as opções de linha de comando do `whisper-cli` podem mudar entre
versões, por isso a versão é fixa e só muda com um novo `sidecars-N`. O modelo
é baixado uma vez e não muda.

## Opcionais

| Peça | Quando entra |
|---|---|
| `claude` (Claude Code) | o botão "Adicionar ao Claude Code" roda `claude mcp add`/`remove` |
| Claude Desktop | o botão "Adicionar ao Claude Desktop" edita `claude_desktop_config.json`, guardando uma cópia antes |
| Homebrew | só para a linha de comando: o instalador usa para instalar o wacli e, sem release publicado, o Go |
| Go 1.26+ | só para compilar a partir do código |

## Bibliotecas Go

| Biblioteca | Para que serve |
|---|---|
| `github.com/wailsapp/wails/v3` (beta.27, fixada) | a janela, a bandeja, a instância única, as notificações e o item de início do app |
| `golang.org/x/sys` | Job Objects, consoles e registro no Windows |
| `github.com/godbus/dbus/v5` | no Linux, descobrir se há bandeja |
| `modernc.org/sqlite` | SQLite em Go puro: lê `wacli.db` e `session.db` e mantém o `state.db` |
| `github.com/skip2/go-qrcode` | desenha o QR code do pareamento no painel |
| `golang.org/x/image` | reduz as imagens que o `read_media` entrega e lê WebP, TIFF e BMP |
| `golang.org/x/text` | lê arquivos de texto em UTF-16 e Windows-1252 |
| `github.com/klippa-app/go-pdfium` + `github.com/tetratelabs/wazero` | o PDFium compilado para WebAssembly, rodando dentro do processo sem cgo: extrai o texto de PDFs e desenha as páginas de um PDF escaneado. Soma cerca de 10 MB ao binário; o primeiro PDF leva cerca de 1,5 s para preparar o leitor |

A linha de comando e o bridge não usam cgo nem o Wails: são binários Go puros,
que não dependem de bibliotecas do sistema. O app usa cgo no macOS e no Linux
(o webview do sistema); no Windows, não.

## Onde ficam os dados

Na pasta de dados do app (`~/Library/Application Support/WhatsApp MCP` no
macOS, `%LOCALAPPDATA%\WhatsApp MCP` no Windows, `~/.local/share/whatsapp-mcp`
no Linux; na linha de comando, `~/.wacli` e `~/.whatsapp-mcp`):

| Caminho | Dono | Conteúdo | Tamanho típico |
|---|---|---|---|
| `wacli/session.db` | wacli/whatsmeow | a sessão e as chaves do dispositivo conectado | pequeno |
| `wacli/wacli.db` | wacli | mensagens, conversas, contatos, grupos | ~130 MB para ~40 mil mensagens |
| `state.db` | app | transcrições, clientes conectados, etapa da instalação, conversas marcadas como resolvidas ou adiadas, webhooks | menos de 1 MB |
| `config.json` | app | porta, abrir com o sistema, fechar = esconder | pequeno |
| `models/` | app | o modelo de transcrição | 574 MB |
| `bin/` | app | `whisper-cli` e `ffmpeg`, com o sha256 de cada um | poucos MB |
| `media/` | app | áudios e arquivos baixados para transcrever ou entregar | cresce com o uso; a retenção (`POST /api/media/retention`) apaga os antigos |
| `exports/` | app | as exportações do `export_messages`, em NDJSON | o tamanho do que foi exportado |
| `updates/` | app | a versão nova baixada, até ser instalada | o tamanho do instalador |

O app nunca escreve no `wacli.db` nem no `session.db`. A pasta `media/` pode
ser apagada a qualquer momento: o que for pedido de novo é baixado outra vez,
se o WhatsApp ainda tiver o arquivo. Desinstalar o app não apaga a pasta de
dados; **Configurações › Apagar todos os dados deste computador** apaga, depois
de desconectar o dispositivo do WhatsApp.
