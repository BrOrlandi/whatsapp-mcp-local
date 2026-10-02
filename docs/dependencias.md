# Dependências

O que o projeto precisa para funcionar, do mais crítico para o menos, e o que
acontece quando cada peça muda ou falta.

## Visão geral

```
                     ┌──────────────── whatsapp-mcp-v2 (um binário Go) ───────────────┐
Claude Desktop ─stdio─┤ bridge                                                        │
Claude Code ───HTTP───┤ MCP + painel ── supervisor ──► wacli ──► whatsmeow ──► WhatsApp│
                      │      │               │            │                           │
                      │      │               │            ├─ session.db (sessão)      │
                      │      │               │            └─ wacli.db   (mensagens)   │
                      │      └── state.db (transcrições, configurações, clientes)     │
                      │      └── transcrição ──► ffmpeg ──► whisper-cli ──► modelo    │
                      └───────────────────────────────────────────────────────────────┘
          serviço: launchd (macOS) · systemd --user (Linux)
```

## Críticas: sem elas nada funciona

### wacli

| | |
|---|---|
| O que é | Cliente de WhatsApp em linha de comando, de [openclaw/wacli](https://github.com/openclaw/wacli), licença MIT |
| Versão em uso | 0.19.0, pelo Homebrew (`openclaw/tap/wacli`) |
| Para que serve | Pareia o computador como dispositivo conectado, mantém a sessão, recebe e guarda as mensagens, envia, baixa mídia |

É a dependência mais importante, e o projeto depende de várias partes dela, não
só dos comandos:

| Do que o daemon depende | Onde | Se mudar |
|---|---|---|
| Saída `--json` dos comandos (`{success, data, error}`) | todas as tools | uma tool passa a falhar ao ler a resposta |
| Eventos `--events` do `sync` e do `auth` (`connected`, `qr_code`, `pair_code`, `history_sync`, `progress`…) | supervisor, pareamento, painel | o QR code ou o estado do sync deixam de aparecer |
| O socket `.send.sock` que o `sync --follow` abre para receber envios | supervisor | envios esperam o tempo limite em vez de sair |
| O arquivo `LOCK`, com `pid=` | recuperação depois de uma queda | um sync órfão não é encerrado sozinho |
| O esquema do `wacli.db` (tabelas `messages`, `chats`, `contacts`, `groups`…), lido em modo somente leitura | prévias, nomes, contexto de áudio, saúde | a consulta afetada falha com erro de SQL |
| O esquema do `session.db` do whatsmeow (`whatsmeow_contacts`, `whatsmeow_lid_map`, `whatsmeow_device`), lido só para nomes | nomes de pessoas e o seu próprio nome | volta a aparecer número em vez de nome |

O wacli não é fixado numa versão: o `brew upgrade` atualiza. A documentação
dele avisa que o esquema do banco pode mudar entre versões. Depois de atualizar
o wacli, vale abrir o painel e conferir a aba Estado e a prévia de conversas.

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

### Gerenciador de serviço

| Sistema | Como | Arquivo |
|---|---|---|
| macOS | LaunchAgent `com.brorlandi.whatsapp-mcp-v2`, com `KeepAlive` | `~/Library/LaunchAgents/com.brorlandi.whatsapp-mcp-v2.plist` |
| Linux | unit `systemd --user` | `~/.config/systemd/user/whatsapp-mcp-v2.service` |

É o que liga o daemon no login e o reinicia se ele cair.

## Da transcrição: necessárias só para transcrever no computador

| Peça | Versão em uso | Origem | Para que serve |
|---|---|---|---|
| whisper.cpp (`whisper-cli`) | 1.9.4 | Homebrew, fórmula `whisper.cpp` | roda o modelo na GPU do Mac |
| ffmpeg | 8.1 | Homebrew | converte o áudio do WhatsApp (Ogg Opus) para WAV 16 kHz |
| Modelo `ggml-large-v3-turbo-q5_0.bin` | 574.041.195 bytes | Hugging Face, `ggerganov/whisper.cpp` | o Whisper large-v3-turbo quantizado |

Sem qualquer uma delas, o resto continua funcionando, e a tool
`transcribe_audio` usa a OpenAI (se houver chave) ou explica como instalar. O
detalhe do funcionamento está em [transcricao.md](transcricao.md).

Riscos: o Homebrew pode renomear fórmulas (a `whisper-cpp` virou `whisper.cpp`,
e o nome antigo ainda funciona como apelido), e as opções de linha de comando do
`whisper-cli` podem mudar entre versões. O modelo é baixado uma vez e não muda.

## Opcionais

| Peça | Quando entra |
|---|---|
| API da OpenAI (`whisper-1`) | transcrição fora do Mac, ou por escolha, com uma chave salva no painel |
| `claude` (Claude Code) | o botão "Adicionar ao Claude Code" roda `claude mcp add`/`remove` |
| Claude Desktop | o botão "Adicionar ao Claude Desktop" edita `claude_desktop_config.json`, guardando uma cópia antes |
| Homebrew | o instalador usa para instalar wacli, whisper.cpp, ffmpeg e, sem release publicado, o Go |
| Go 1.26+ | só para compilar a partir do código, quando não há release |
| python3 | só nos testes e no modo de demonstração, que usam um wacli falso |

## Bibliotecas Go (compiladas dentro do binário)

| Biblioteca | Para que serve |
|---|---|
| `modernc.org/sqlite` | SQLite em Go puro, sem cgo: lê `wacli.db` e `session.db` e mantém o `state.db` |
| `github.com/skip2/go-qrcode` | desenha o QR code do pareamento no painel |

O resto (servidor HTTP, MCP, painel, supervisor) usa só a biblioteca padrão do
Go. O binário não depende de bibliotecas do sistema.

## Onde ficam os dados

| Caminho | Dono | Conteúdo | Tamanho típico |
|---|---|---|---|
| `~/.wacli/session.db` | wacli/whatsmeow | a sessão e as chaves do dispositivo conectado | pequeno |
| `~/.wacli/wacli.db` | wacli | mensagens, conversas, contatos, grupos | ~130 MB para ~40 mil mensagens |
| `~/.whatsapp-mcp-v2/state.db` | daemon | transcrições, chave da OpenAI, clientes conectados, etapa do setup | menos de 1 MB |
| `~/.whatsapp-mcp-v2/models/` | daemon | o modelo de transcrição | 547 MB |
| `~/.whatsapp-mcp-v2/media/` | daemon | áudios e arquivos baixados para transcrever ou entregar | cresce com o uso, sem limpeza automática |
| `~/Library/Logs/whatsapp-mcp-v2.log` | daemon | log (macOS) | pequeno |

O daemon nunca escreve no `wacli.db` nem no `session.db`. A pasta `media/`
pode ser apagada a qualquer momento: o que for pedido de novo é baixado outra
vez, se o WhatsApp ainda tiver o arquivo.
