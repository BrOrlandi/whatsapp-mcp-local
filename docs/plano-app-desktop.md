# Plano: WhatsApp MCP como app desktop

> Status: implementado em outubro de 2026, das fases 0 a 7, com os ajustes
> registrados em [fase0-provas.md](fase0-provas.md) e na seção 14. O que ainda
> depende de uma máquina Windows, de credenciais ou de publicar está na seção 14.

## 0. Resumo

**Objetivo.** Transformar o WhatsApp MCP v2 num aplicativo de verdade para
macOS, Windows e Linux: baixar, abrir, ler o QR code, conectar o Claude. Sem
terminal, sem navegador, sem serviço instalado por linha de comando.

**Como vai funcionar para a pessoa.**

1. Baixa o instalador do seu sistema (`.dmg`, `.exe` ou `.AppImage`/`.deb`).
2. Abre o app. A janela já mostra o QR code e o passo a passo.
3. Lê o QR code no celular. O app mostra o nome e o número conectados.
4. Clica em "Adicionar ao Claude Desktop" (ou Code). Pronto.
5. Enquanto o app estiver aberto, na janela ou só na bandeja do sistema, o
   MCP funciona. Uma opção liga o app junto com o sistema.

**O que muda e o que não muda.**

| Fica igual | Muda |
|---|---|
| O núcleo: supervisor do wacli, MCP, índice, nomes, transcrição sob pedido, painel | O painel abre numa janela do app, não no navegador |
| O MCP em `localhost` (porta 47821 por padrão) e as tools | O app roda o daemon dentro do próprio processo, em vez de um serviço do sistema |
| O wacli como motor do WhatsApp | O wacli passa a vir dentro do app, sem Homebrew |
| | A porta do MCP passa a ser configurável na interface |
| O whisper.cpp + large-v3-turbo, só quando pedido | whisper.cpp e ffmpeg vêm prontos para cada sistema, baixados sob demanda |
| A versão de linha de comando (`whatsapp-mcp-v2 serve`), para servidores e Linux sem tela | Ganha ícone na bandeja, instância única, "iniciar com o sistema", atualização automática |

**Esforço estimado.** De 4 a 6 semanas de trabalho focado, divididas em 8
fases (seção 10). A primeira semana é de provas técnicas, que podem mudar o
restante do plano.

---

## 1. Experiência do usuário

### 1.1 Primeira abertura

```
┌─────────────────────────── WhatsApp MCP ───────────────────────────┐
│  ① WhatsApp ─────────────── ② Claude                               │
│                                                                    │
│  Conecte o seu WhatsApp                                            │
│   1 Abra o WhatsApp no celular                    ┌──────────┐     │
│   2 Configurações › Dispositivos conectados       │ ▓▓ QR ▓▓ │     │
│   3 Conectar um dispositivo                       │ ▓▓▓▓▓▓▓▓ │     │
│   4 Aponte a câmera para o código                 └──────────┘     │
│                                                                    │
│   Prefiro digitar um código no celular                             │
└────────────────────────────────────────────────────────────────────┘
```

- A janela abre direto no passo 1, com o QR code já gerado. Não há botão de
  "começar". É o assistente que já existe no painel (`/instalacao`).
- Ao ler o QR code, o app vai para o passo 2: "WhatsApp conectado: [nome]",
  com o número, e as opções Claude Desktop (primeira), Claude Code e Outra
  ferramenta.
- Quando o Claude se conecta, aparece "Tudo pronto" e o app passa para o
  painel normal.
- Ainda na primeira abertura, o app oferece, com uma caixa marcada por padrão:
  **"Abrir o WhatsApp MCP quando o computador ligar"**.

### 1.2 Uso diário

- **Bandeja do sistema** (barra de menus no macOS, área de notificação no
  Windows, indicador no Linux), com um ícone que muda de cor:
  - verde: conectado e recebendo;
  - amarelo: reconectando, pausado ou recebendo histórico;
  - vermelho: WhatsApp desconectado ou sync parado.
- **Menu da bandeja:**
  - Abrir o WhatsApp MCP
  - Estado: "Conectado · 58 mensagens na última hora"
  - ☑ Iniciar com o sistema
  - Sair do WhatsApp MCP
- **Fechar a janela não encerra o app.** O MCP continua funcionando pela
  bandeja. Na primeira vez em que a janela for fechada, uma notificação avisa:
  "O WhatsApp MCP continua rodando na bandeja. Para encerrar, use Sair."
  (Decidido: fechar esconde, não encerra.)
- **Sair** encerra o daemon com cuidado (para o sync, libera o lock) e o MCP
  para de responder. Os clientes recebem a mensagem "o WhatsApp MCP não está
  aberto".
- **Notificações do sistema** só para o que exige ação: WhatsApp desconectado
  pelo celular, ou nova versão disponível.

### 1.3 Iniciar com o sistema

- Ligado: no login o app abre **só na bandeja**, sem janela (`--hidden`).
- A opção fica na bandeja e numa página "Configurações" do painel.

### 1.4 Atualizações

- O app verifica no GitHub Releases, uma vez por dia, se há versão nova.
- Um aviso no painel (como o banner do v1) e na bandeja: "Versão 1.3
  disponível". Ao clicar, baixa, verifica a assinatura e reinicia.
- O wacli embutido é atualizado junto com o app, numa versão testada. Nunca
  separadamente.

### 1.5 Porta do MCP

O MCP sobe em `localhost`, na porta **47821** por padrão. A página
Configurações deixa trocar:

```
Porta do MCP
[ 47821 ]  [Salvar]
Endereço: http://127.0.0.1:47821/mcp   [Copiar]
```

- **Quando trocar:** se outro programa já usa a porta, ou se a pessoa quiser
  uma porta específica.
- **Validação:** só números de 1024 a 65535. Antes de salvar, o app tenta abrir
  a porta. Se outro programa estiver nela, mostra "A porta 3000 já está em uso
  por outro programa" e não salva.
- **O que acontece ao salvar:** o app troca só o servidor do MCP para a porta
  nova, sem reiniciar o WhatsApp nem o sync. Uma conversa já aberta no Claude
  perde a conexão e reconecta no próximo uso.
- **Os clientes conectados são atualizados junto:**
  - **Claude Desktop (e Cowork):** nada a fazer. O bridge lê a porta da
    configuração do app a cada vez que inicia, então basta reabrir o Claude
    Desktop.
  - **Claude Code:** o endereço tem a porta dentro. O app oferece "Atualizar o
    Claude Code agora", que refaz o `claude mcp add` com o endereço novo.
  - **Outras ferramentas:** o app mostra o endereço novo e avisa que elas
    precisam ser configuradas de novo, com o prompt pronto da aba "Outra
    ferramenta".
- **Porta ocupada ao abrir o app:** se, ao ligar, a porta configurada estiver
  em uso por outro programa (não por outra cópia do app), a janela abre com o
  aviso e sugere uma porta livre, já testada, com um botão "Usar a porta 47822".
  O WhatsApp continua conectado enquanto isso; só o MCP fica fora até resolver.
- **Linha de comando:** a variável `WHATSAPP_MCP_PORT` continua funcionando e,
  quando definida, vale mais que a configuração da interface (a página mostra
  "definida por variável de ambiente" e bloqueia o campo).

### 1.6 Desinstalar

- macOS: arrastar para o Lixo. Windows: "Adicionar ou remover programas".
  Linux: o gerenciador de pacotes, ou apagar o AppImage.
- O desinstalador **não** apaga os dados (mensagens, sessão, modelo). A página
  Configurações tem "Apagar todos os dados deste computador", com
  confirmação, que também desconecta o dispositivo no WhatsApp.

---

## 2. Decisões técnicas

### 2.1 Framework do app: Wails v3

| Opção | Prós | Contras | Veredito |
|---|---|---|---|
| **Wails v3** (Go + webview do sistema) | Mesmo Go do projeto, sem reescrever nada; reaproveita o painel HTML; bandeja, instância única e várias janelas nativas; binário pequeno (~15 MB) | Ainda em beta (API estável, usado em produção); precisa compilar em cada sistema (cgo) | **Escolhido** |
| Wails v2 | Estável há anos | Sem bandeja nativa (precisaria de outra biblioteca); menos recursos de janela | Plano B, se o v3 travar em algo |
| Tauri (Rust) | Maduro, ótimo empacotamento e atualização | O núcleo em Go viraria um processo separado; duas linguagens | Não |
| Electron | Maduro | ~150 MB; Node junto; o Go também separado | Não |
| Fyne / Gio (UI nativa em Go) | Um binário só | Reescrever todo o painel; visual diferente do v1 | Não |

O webview de cada sistema:

| Sistema | Webview | Observação |
|---|---|---|
| macOS 11+ | WKWebView | já vem no sistema |
| Windows 10/11 | WebView2 (Edge) | já vem no Windows 11 e em Windows 10 atualizados; o instalador traz o instalador do WebView2 por garantia |
| Linux | WebKitGTK 4.1 | dependência do pacote (`libwebkit2gtk-4.1-0`); o AppImage a inclui |

### 2.2 Um processo só

Hoje são dois processos: o daemon (serviço do sistema) e o navegador. No app,
o daemon roda **dentro** do processo do app:

```
WhatsApp MCP (processo do app)
├── janela (Wails) ──── painel, servido em memória, sem rede
├── bandeja
├── servidor MCP HTTP em 127.0.0.1:47821 ◄── Claude Code
│                                       ◄── bridge (stdio) ◄── Claude Desktop / Cowork
├── supervisor ──► wacli sync --follow (processo filho)
└── transcrição sob pedido ──► ffmpeg ──► whisper-cli (processos filhos)
```

- O código de `cmd/whatsapp-mcp-v2/main.go` (`serve`) vira um pacote
  reutilizável, `internal/daemon`, com `Start(ctx, cfg) (*Daemon, error)` e
  `Stop()`. Ele é usado pelo app e pela linha de comando.
- **A janela não usa a rede.** O Wails serve o painel por um esquema interno
  (`wails://`), e o handler é o mesmo `http.Handler` do painel de hoje,
  chamado em memória. O servidor HTTP em `127.0.0.1:47821` continua só para o
  MCP (o Claude Code precisa dele) e para o `/health`.
- **A janela não fala com a internet.** O painel só carrega recursos
  embutidos, como hoje.

### 2.3 Três executáveis

| Executável | Subsistema | Para quê |
|---|---|---|
| `WhatsApp MCP` (o app) | janela | o que a pessoa abre |
| `whatsapp-mcp-bridge` | console | o processo stdio que o Claude Desktop inicia; lê a porta em `config.json` e repassa para `127.0.0.1:<porta>` |
| `whatsapp-mcp` (opcional, mesmo código de hoje) | console | linha de comando e servidores sem tela |

O bridge é separado porque, no Windows, um executável de janela
(`-H windowsgui`) não tem stdio confiável. O bridge é minúsculo (~5 MB) e vai
dentro do pacote do app.

### 2.4 O que vai dentro do app e o que é baixado depois

| Peça | Como chega | Tamanho |
|---|---|---|
| App + bridge | no instalador | ~20 MB |
| wacli (versão fixa e testada) | no instalador | ~25 MB |
| ffmpeg mínimo (só Ogg/Opus → WAV) | baixado ao ativar a transcrição | ~3 MB |
| whisper-cli do sistema | baixado ao ativar a transcrição | ~5–15 MB (CPU) ou mais (CUDA) |
| Modelo large-v3-turbo | baixado ao ativar a transcrição | 574 MB |

Instalador total: **~45 MB**. A transcrição continua opcional e sob pedido,
como hoje.

---

## 3. Arquitetura do código

### 3.1 Estrutura nova

```
cmd/
  whatsapp-mcp/          linha de comando (o main.go de hoje, enxuto)
  whatsapp-mcp-bridge/   bridge stdio
  app/                   app Wails: janela, bandeja, ciclo de vida
internal/
  daemon/          NOVO: monta índice, estado, supervisor, MCP, painel, HTTP; Start/Stop
  platform/        NOVO: caminhos, autostart, processos e abertura de URL por sistema
    paths_{darwin,windows,linux}.go
    autostart_{darwin,windows,linux}.go
    proc_{unix,windows}.go       grupo de processos / Job Object, sinais, órfãos
  sidecar/         NOVO: localiza e verifica os binários embutidos (wacli, ffmpeg, whisper)
  updater/         NOVO: verifica e aplica atualizações
  wacli/           supervisor e pareamento (processos via platform)
  localasr/        transcrição (binários via sidecar, não Homebrew)
  panel/           painel (origem permitida também wails://; página Configurações)
  mcp/, index/, state/, bridge/, httpserver/, brand/   sem mudanças grandes
build/
  darwin/   Info.plist, entitlements, ícone .icns
  windows/  manifesto, ícone .ico, script NSIS
  linux/    .desktop, ícones, AppImage
```

### 3.2 Ciclo de vida do app

```
abrir app
 ├─ já existe uma instância? → foca a janela dela e sai   (instância única)
 ├─ migra dados da instalação antiga, se houver           (seção 8)
 ├─ daemon.Start()
 │    ├─ porta configurada ocupada por outro programa? → avisa e sugere uma livre (1.5)
 │    └─ supervisor sobe o wacli (ou espera o pareamento)
 ├─ cria bandeja
 └─ cria a janela (a menos que --hidden)
        └─ abre em /instalacao se não pareado, senão em /

fechar janela → esconde (app segue na bandeja)
sair (bandeja / Cmd+Q / desligar o sistema)
 └─ daemon.Stop(): cancela pareamento, para o sync com elegância (até 20 s), fecha bancos
```

O desligamento do sistema também precisa encerrar o sync com elegância:
`applicationShouldTerminate` no macOS, `WM_QUERYENDSESSION`/`WM_ENDSESSION`
no Windows (o Wails expõe ambos) e `SIGTERM` no Linux.

---

## 4. Por sistema

### 4.1 macOS

| Tema | Plano |
|---|---|
| Arquiteturas | arm64 e amd64, como `.app` universal |
| Pacote | `WhatsApp MCP.app` dentro de um `.dmg` com atalho para Aplicativos |
| Binários embutidos | `Contents/MacOS/WhatsApp MCP`, `Contents/MacOS/whatsapp-mcp-bridge`, `Contents/Resources/bin/wacli` |
| Dados | `~/Library/Application Support/WhatsApp MCP/` (`wacli/`, `state.db`, `models/`, `media/`) |
| Logs | `~/Library/Logs/WhatsApp MCP/` |
| Iniciar com o sistema | `SMAppService.mainApp.register()` (macOS 13+; aparece em Ajustes › Itens de início); fallback para LaunchAgent no macOS 11–12 |
| Assinatura | Developer ID Application + hardened runtime + notarização (`notarytool`) + `stapler`. **Todos** os binários dentro do `.app` (wacli, whisper-cli, ffmpeg) são assinados, inclusive os baixados depois |
| Transcrição | whisper-cli compilado no nosso CI com Metal (o whisper.cpp não publica binário de macOS); ffmpeg mínimo compilado no CI |
| Bridge no Claude Desktop | `"command": "/Applications/WhatsApp MCP.app/Contents/MacOS/whatsapp-mcp-bridge"` |
| Pegadinhas | Sem notarização, o Gatekeeper diz que o app "está danificado". O SMAppService se comporta melhor com o app em /Applications, e o `.dmg` orienta a arrastar para lá. Binários baixados depois ganham o atributo de quarentena, que deve ser removido e verificado |

### 4.2 Windows

| Tema | Plano |
|---|---|
| Arquiteturas | amd64 (o wacli não publica arm64; Windows on ARM roda amd64 emulado) |
| Pacote | Instalador NSIS por usuário (sem pedir administrador) em `%LOCALAPPDATA%\Programs\WhatsApp MCP\`. Avaliar MSIX depois |
| Binários embutidos | `WhatsApp MCP.exe`, `whatsapp-mcp-bridge.exe`, `bin\wacli.exe` |
| Dados | `%LOCALAPPDATA%\WhatsApp MCP\` |
| Iniciar com o sistema | valor em `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` com `"...\WhatsApp MCP.exe" --hidden` |
| Assinatura | Começa sem assinatura (decisão 7, seção 12): o SmartScreen mostra "O Windows protegeu o computador", e o README ensina o "Executar assim mesmo". Depois, SignPath Foundation (grátis, se o projeto se qualificar) ou um certificado OV de autoridade certificadora |
| WebView2 | presente no Windows 11 e no 10 atualizado; o instalador inclui o bootstrapper |
| Transcrição | whisper.cpp publica `whisper-bin-x64.zip` (CPU) e `whisper-cublas-12.x-bin-x64.zip` (NVIDIA). Padrão: CPU; CUDA se detectar placa NVIDIA. ffmpeg mínimo do nosso CI |
| Bridge no Claude Desktop | `%APPDATA%\Claude\claude_desktop_config.json`, `"command": "C:\\Users\\…\\WhatsApp MCP\\whatsapp-mcp-bridge.exe"` |
| Pegadinhas | ver 6.1: grupos de processos e sinais funcionam diferente; o wacli usa socket Unix (suportado no Windows 10 1803+); antivírus podem estranhar um binário baixando e rodando outros (assinar todos) |

### 4.3 Linux

| Tema | Plano |
|---|---|
| Arquiteturas | amd64 e arm64 |
| Pacotes | AppImage (roda em qualquer distro) e `.deb` (Ubuntu/Debian). Flatpak depois, se houver pedido |
| Binários embutidos | dentro do AppImage/`.deb`: app, bridge, wacli |
| Dados | `$XDG_DATA_HOME/whatsapp-mcp` (`~/.local/share/whatsapp-mcp`) |
| Iniciar com o sistema | `~/.config/autostart/whatsapp-mcp.desktop` (padrão XDG; GNOME, KDE, XFCE) |
| Bandeja | StatusNotifierItem/AppIndicator. No GNOME puro precisa da extensão AppIndicator, então fechar a janela sem bandeja disponível **não** pode esconder o app: nesse caso a janela minimiza |
| Webview | `libwebkit2gtk-4.1` (o `.deb` declara; o AppImage inclui) |
| Assinatura | não há exigência; publicar checksums e assinatura GPG dos arquivos |
| Transcrição | whisper.cpp publica `whisper-bin-ubuntu-x64/arm64.tar.gz` (CPU); Vulkan/CUDA depois |
| Bridge no Claude Desktop | o mesmo esquema dos outros sistemas, se houver Claude Desktop para Linux na época; senão, o foco no Linux é o Claude Code |

---

## 5. Dependências embarcadas

### 5.1 wacli

- **Versão fixa por release do app.** O app é testado com uma versão do wacli
  (hoje 0.20.0) e a traz dentro do pacote. Isso resolve o maior risco atual (o
  `brew upgrade` mudar o wacli por baixo do app).
- **Origem:** os releases oficiais de `openclaw/wacli` para darwin
  (arm64/amd64), linux (arm64/amd64) e windows (amd64), baixados no CI com
  verificação do `checksums.txt` publicado por eles.
- **Licença:** MIT. O app inclui o aviso de licença do wacli e do whatsmeow
  (MPL-2.0: distribuir o binário é permitido, e o código-fonte já está
  disponível no repositório deles).
- **Atualizar o wacli** = novo release do app, depois de rodar a bateria de
  testes da seção 9 com a versão nova.

### 5.2 Transcrição

| Peça | macOS | Windows | Linux |
|---|---|---|---|
| whisper-cli | compilado no nosso CI (Metal) | release oficial (CPU ou CUDA) | release oficial (CPU) |
| ffmpeg | build mínimo nosso | build mínimo nosso | build mínimo nosso |
| modelo | large-v3-turbo q5_0 (574 MB) | o mesmo | o mesmo |

- **ffmpeg mínimo:** compilado com
  `--disable-everything --enable-demuxer=ogg --enable-decoder=opus --enable-muxer=wav --enable-encoder=pcm_s16le --enable-filter=aresample --enable-protocol=file`.
  Fica com ~3 MB em vez de ~80 MB. Alternativa a avaliar no spike: decodificar
  Opus em Go e dispensar o ffmpeg.
- **Onde ficam:** `<dados>/bin/` com um `manifest.json` (versão, sha256). O
  `sidecar` verifica o sha256 antes de executar.
- **Em CPU** (Windows e Linux sem GPU), o turbo leva mais ou menos o tempo do
  próprio áudio. Na primeira versão do app, a página Transcrição só avisa
  disso; o modelo continua sendo o turbo.
- **Sem transcrição pela OpenAI na primeira versão do app.** A opção de usar
  uma chave da OpenAI (Whisper por API), útil justamente em computadores sem
  GPU, fica para uma versão futura. Até lá, quem não puder ou não quiser usar
  o motor local tem o caminho de sempre: a ferramenta de IA baixa o áudio com
  `download_media` (o arquivo já fica no disco, com o caminho na resposta) e o
  transcreve como preferir. Na prática, a página Transcrição do app mostra só o
  motor local, e a tool `set_transcription_key` e o uso da chave pela
  `transcribe_audio` saem da primeira versão do app, voltando junto com a
  opção.
- **O que não muda:** a transcrição só roda quando pedida pela tool
  `transcribe_audio`, e o contexto da conversa continua no `--prompt`.

### 5.3 Bibliotecas Go novas

- `github.com/wailsapp/wails/v3`: janela, bandeja, instância única,
  notificações e eventos de desligamento.
- `golang.org/x/sys/windows`: Job Objects e registro do Windows.

---

## 6. Mudanças no código, por pacote

### 6.1 `internal/platform/proc_*` e o supervisor (o ponto mais delicado)

Hoje o supervisor usa recursos de Unix: `Setpgid`, `syscall.Kill(-pid)`, SIGINT
para o grupo e recuperação de órfãos lendo o `pid` do arquivo `LOCK`.

| Função | Unix (macOS/Linux) | Windows |
|---|---|---|
| Isolar o filho | `Setpgid` | `CREATE_NEW_PROCESS_GROUP` + Job Object |
| Parar com elegância | SIGINT para o grupo | `CTRL_BREAK_EVENT` para o grupo do filho, que o Go do wacli recebe como `os.Interrupt` (a validar no spike) |
| Forçar | SIGKILL | `TerminateJobObject` |
| Filho morre com o app | não é automático, daí a recuperação de órfãos | **Job Object com `KILL_ON_JOB_CLOSE`**: se o app morrer, o Windows mata o wacli. Resolve o problema dos órfãos na origem |
| Órfão de uma execução anterior | lê `pid` do `LOCK`, confere o pai e encerra | o mesmo, com `OpenProcess` e conferindo o nome do executável |

No Linux, aplicar `PR_SET_PDEATHSIG` no filho para ele morrer junto com o app,
como reforço da recuperação de órfãos. No macOS não existe equivalente, então
a recuperação de órfãos continua lá.

### 6.2 `internal/platform/paths_*`

- Uma função `DataDir()` por sistema (seção 4).
- `WACLI_STORE_DIR` passa a ser `<dados>/wacli`, dentro da pasta do app, em vez
  do `~/.wacli` global, para o app ser autocontido.
- As variáveis de ambiente atuais continuam valendo, para quem roda a linha de
  comando.

- **`config.json`** em `<dados>/config.json`, para o que precisa ser lido
  antes de qualquer outra coisa e também pelo bridge:

  ```json
  { "port": 47821, "close_to_tray": true, "autostart": true }
  ```

  Lido na abertura do app e pelo `whatsapp-mcp-bridge` a cada início. Escrito
  de forma atômica (arquivo temporário e `rename`). O que é estado do daemon
  (transcrições, clientes, etapa do setup) continua no `state.db`.

### 6.3 `internal/daemon`

- Extrair do `main.go` a montagem: índice, estado, supervisor, motor de
  transcrição, servidor MCP, painel e HTTP.
- `Start` retorna erros que a janela consegue mostrar (porta em uso, wacli
  ausente ou corrompido, pasta sem permissão).
- `SetPort(port)`: abre o listener novo antes de fechar o antigo, para nunca
  ficar sem nenhum; se o novo falhar, mantém o antigo e devolve o erro. Atualiza
  o endereço que o painel mostra e o que os botões de cliente gravam. Não mexe
  no supervisor nem no sync.
- Expor um canal de eventos de estado (conectado, reconectando, desconectado
  pelo WhatsApp, histórico chegando) para a bandeja e as notificações, em vez
  de a bandeja ficar consultando.

### 6.4 `internal/panel`

- Aceitar a origem `wails://wails` (e o equivalente de cada sistema), além de
  `http://127.0.0.1:47821`, na verificação de mesma origem das ações.
- Página nova **Configurações**: porta do MCP (1.5), iniciar com o sistema,
  fechar = esconder ou sair, pasta de dados (abrir no Finder/Explorer), versão e atualizações, apagar
  dados.
- O botão "Adicionar ao Claude Desktop" passa a gravar o caminho do
  `whatsapp-mcp-bridge` do app, com o arquivo de configuração certo de cada
  sistema.
- Tirar do painel o que só fazia sentido no navegador (o texto "abra
  http://127.0.0.1:47821", o comando `whatsapp-mcp-v2 open`).
- Links externos (doação, GitHub, OpenAI) abrem no navegador do sistema, não
  dentro da janela.

### 6.5 `internal/localasr` e `internal/sidecar`

- Trocar a instalação pelo Homebrew pelo download dos binários do nosso release
  (seção 5.2), com sha256 verificado e progresso na tela.
- Escolher o pacote certo: macOS Metal; Windows CPU ou CUDA (detectando NVIDIA
  com `nvidia-smi`); Linux CPU.
- No macOS, remover a quarentena e verificar a assinatura dos binários baixados.

### 6.6 Autostart (`internal/platform/autostart_*`)

`Enable()`, `Disable()` e `Enabled()` por sistema (seção 4). O antigo
`service install` (LaunchAgent/systemd) fica só na linha de comando.

### 6.7 `cmd/app`

- Wails v3: janela principal (1000×760, mínimo 720×560), bandeja com ícones
  claro e escuro, instância única, `--hidden`, eventos de desligamento.
- Ícone da bandeja reflete o `health`: verde, amarelo, vermelho.
- Menu do macOS: Sobre, Configurações (Cmd+,), Sair (Cmd+Q).

### 6.8 `internal/updater`

- Consulta `api.github.com/repos/BrOrlandi/whatsapp-mcp-v2/releases/latest`
  uma vez por dia.
- Baixa o pacote do sistema, confere sha256 e assinatura, e então:
  - macOS: troca o `.app` e reabre;
  - Windows: roda o novo instalador em modo silencioso;
  - Linux: troca o AppImage ou orienta o `apt`.
- Usar o mecanismo de atualização do Wails v3 se ele estiver pronto até lá;
  senão, este pacote.

---

## 7. Build, assinatura e publicação

### 7.1 CI (GitHub Actions)

O Wails usa cgo e o webview de cada sistema, então cada sistema compila na
própria máquina do CI:

| Job | Máquina | Saída |
|---|---|---|
| macos | `macos-14` (arm64), com cross para amd64 | `.app` universal assinado e notarizado, dentro de `.dmg` |
| windows | `windows-latest` | `WhatsApp-MCP-Setup.exe` assinado |
| linux-amd64 | `ubuntu-22.04` | `.AppImage`, `.deb` |
| linux-arm64 | `ubuntu-22.04-arm` | `.AppImage`, `.deb` |
| cli | os três | `whatsapp-mcp_<sistema>_<arquitetura>.tar.gz` (ou `.zip` no Windows), a linha de comando sem janela, que o `install.sh` baixa |
| sidecars | os três | whisper-cli (macOS), ffmpeg mínimo (os três), sha256 |

Todos os jobs rodam `go test ./...` antes de empacotar. O release no GitHub é
criado por tag `v*`, com `checksums.txt`.

### 7.2 Segredos necessários

| Segredo | Para quê | Custo |
|---|---|---|
| Certificado **Developer ID Application** (exportado como `.p12`) + chave da API do App Store Connect para a notarização | macOS | já coberto pela conta Apple Developer do Bruno |
| Nenhum na primeira versão; depois, token do SignPath ou certificado OV | Windows | grátis (SignPath) ou ~US$ 100–300/ano (OV) |
| Chave GPG | assinatura dos pacotes Linux | grátis |

### 7.3 Versões

- SemVer, recomeçando a contagem do app em `v1.0.0`.
- O changelog cita a versão do wacli embutida.

---

## 8. Migração da instalação atual

Hoje (no Mac do Bruno) existem: o LaunchAgent `com.brorlandi.whatsapp-mcp-v2`,
o store do wacli em `~/.wacli`, e `~/.whatsapp-mcp-v2` (estado, modelo,
mídia). Ao abrir o app pela primeira vez:

1. **Detectar** o LaunchAgent e os dados antigos.
2. **Perguntar:** "Encontramos uma instalação anterior do WhatsApp MCP, já
   conectada como [nome]. Usar a mesma conexão?" Assim não é preciso ler o QR
   code de novo.
3. Se sim:
   - parar e remover o LaunchAgent;
   - **mover** `~/.wacli` para `<dados>/wacli`;
   - mover `state.db`, `models/` e `media/`;
   - atualizar a entrada do Claude Desktop para o bridge novo (o Claude Code
     não muda, porque o endereço é o mesmo).
4. Se não: começar do zero, sem apagar nada.

Mover, e não copiar, porque dois processos com a mesma sessão do WhatsApp
brigariam pelo dispositivo.

---

## 9. Testes

### 9.1 Automáticos (em cada sistema, no CI)

- Os testes atuais do supervisor, do pareamento e da recuperação de órfãos,
  rodando também no Windows com um wacli falso equivalente (o atual usa bash e
  python; reescrever em Go, compilado como binário de teste, para rodar nos
  três sistemas).
- `internal/platform`: caminhos, autostart (escreve e lê de volta numa pasta
  temporária), encerramento de processos (Job Object no Windows).
- `internal/daemon`: sobe, responde `/health`, para sem deixar processos.
- O painel com origem `wails://`.
- Troca de porta: o listener novo sobe antes de o antigo fechar; porta ocupada
  é recusada sem derrubar a atual; o bridge passa a usar a porta nova no
  próximo início; a variável de ambiente vence a configuração.

### 9.2 Ponta a ponta (manual, antes de cada release)

| Cenário | macOS | Windows | Linux |
|---|---|---|---|
| Instalação limpa, QR code, Claude conectado | ☐ | ☐ | ☐ |
| Código pelo número de telefone | ☐ | ☐ | ☐ |
| Trocar a porta: Claude Desktop e Claude Code voltam a funcionar | ☐ | ☐ | ☐ |
| Porta ocupada ao abrir: aviso e sugestão de porta livre | ☐ | ☐ | ☐ |
| Fechar a janela: MCP segue respondendo | ☐ | ☐ | ☐ |
| Sair: nenhum wacli fica rodando | ☐ | ☐ | ☐ |
| Matar o app à força: na volta, assume o sync | ☐ | ☐ | ☐ |
| Iniciar com o sistema: reinicia o computador e o app sobe na bandeja | ☐ | ☐ | ☐ |
| Desligado 30 min: mensagens recuperadas ao reabrir | ☐ | ☐ | ☐ |
| Transcrição: instalar, transcrever um áudio sob pedido | ☐ | ☐ | ☐ |
| Atualização automática de uma versão para a seguinte | ☐ | ☐ | ☐ |
| Migração da instalação v2 atual (só macOS) | ☐ | — | — |
| Gatekeeper / SmartScreen sem avisos | ☐ | ☐ | — |
| Linha de comando: `install.sh` num servidor sem tela, QR code pelo painel no navegador | ☐ | — | ☐ |

---

## 10. Fases

Cada fase termina com algo que roda e pode ser testado.

### Fase 0 · Provas técnicas (3–4 dias)

Respondem às perguntas que podem mudar o plano:

1. **Wails v3 nos três sistemas:** janela com o painel atual servido em
   memória, bandeja, instância única, `--hidden`, eventos de desligamento.
2. **wacli no Windows:** pareia, roda `sync --follow`, o socket de envio
   funciona, o `CTRL_BREAK` encerra com elegância, o Job Object mata o filho
   quando o app morre, o lock é liberado.
3. **Transcrição fora do Mac:** whisper-cli oficial + ffmpeg mínimo no Windows e
   no Linux, com tempo de transcrição do turbo em CPU.
4. **Bridge `.exe` com o Claude Desktop no Windows.**

**Entrega:** um relatório curto com o que funcionou, o que não, e os ajustes
no plano.

### Fase 1 · Núcleo como biblioteca (2–3 dias)

`internal/daemon`, `internal/platform` (caminhos e processos), os binários da
seção 2.3 separados. A linha de comando continua funcionando igual.

**Aceite:** testes atuais passam; `whatsapp-mcp serve` se comporta como hoje.

### Fase 2 · App no macOS (4–5 dias)

Janela, bandeja, primeira abertura no QR code, Configurações (com a porta), autostart com
SMAppService, wacli embutido, migração da instalação atual.

**Aceite:** a sua instalação migra sem novo QR code; os cenários da 9.2 no
macOS passam (sem assinatura ainda).

### Fase 3 · App no Windows (4–6 dias)

Processos com Job Object, `CTRL_BREAK`, caminhos, autostart no registro,
WebView2, instalador NSIS, bridge `.exe`, configuração do Claude Desktop no
Windows.

**Aceite:** cenários da 9.2 no Windows passam numa máquina Windows 11 limpa.

### Fase 4 · App no Linux (2–3 dias)

AppImage e `.deb`, autostart XDG, bandeja com fallback para GNOME sem extensão.

**Aceite:** cenários da 9.2 no Ubuntu 24.04 (GNOME) e no Kubuntu (KDE).

### Fase 5 · Transcrição nos três sistemas (3–4 dias)

`internal/sidecar`, builds do whisper-cli (macOS) e do ffmpeg mínimo no CI,
download com sha256, escolha CPU/CUDA, aviso de desempenho em CPU.

**Aceite:** transcrever um áudio sob pedido nos três sistemas, com contexto.

### Fase 6 · Empacotamento, assinatura, publicação e atualização (4–6 dias)

CI da seção 7, notarização, assinatura no Windows, checksums, `updater`.

**Aceite:** uma tag gera os instaladores assinados; o app na versão N se
atualiza para a N+1 sozinho nos três sistemas.

### Fase 7 · Documentação e beta (2–3 dias)

- README: "Baixe o app para o seu sistema" primeiro, com os três arquivos; logo
  depois, "Prefere o terminal ou um servidor?", com o prompt e o `install.sh`
  da linha de comando.
- Capturas novas, `docs/` atualizados (dependências, caminhos por sistema).
- Beta com algumas pessoas em cada sistema antes do anúncio.

---

## 11. Riscos

| Risco | Impacto | Mitigação |
|---|---|---|
| Wails v3 ainda em beta | um bug de janela ou bandeja sem correção rápida | prova na Fase 0; Wails v2 + biblioteca de bandeja como plano B; fixar a versão do Wails |
| wacli no Windows menos testado pelo projeto deles | sync instável ou encerramento ruim no Windows | Fase 0 dedicada; reportar e contribuir com correções no wacli; Job Object garante ao menos que nada fique órfão |
| Windows sem assinatura | o SmartScreen assusta quem baixa | instruções com imagens no README; SignPath Foundation ou certificado OV depois (decisão 8) |
| Notarização da Apple recusar o app | o `.dmg` passa a abrir com aviso | assinar todos os binários embutidos com *hardened runtime*; testar a notarização já na Fase 2, não só na Fase 6 |
| WhatsApp mudar o protocolo | a conexão para em todos os sistemas | o app avisa na bandeja; release rápido com o wacli atualizado; o updater entrega em horas |
| Antivírus no Windows estranhar o app que baixa e roda binários | bloqueio da transcrição | assinar todos os binários; baixar só do nosso release; verificar sha256 |
| Transcrição lenta em CPU | experiência ruim no Windows e Linux sem GPU | aviso claro; CUDA quando houver NVIDIA; a IA pode baixar o áudio e transcrever de outra forma; chave da OpenAI numa versão futura |
| Duas cópias rodando (app e serviço antigo) | brigam pela sessão do WhatsApp | migração remove o LaunchAgent; o app detecta a porta e o lock ocupados e explica |
| Outro programa na porta 47821 | o MCP não sobe | aviso na abertura com uma porta livre sugerida; porta configurável (1.5) |
| Porta trocada e cliente com o endereço antigo | a IA "perde" o WhatsApp | o bridge lê a porta sozinho; o Claude Code é atualizado com um clique; as outras ferramentas recebem o prompt pronto |
| GNOME sem bandeja | o app "some" ao fechar a janela | detectar a falta de bandeja e só minimizar |

---

## 12. Decisões

### Tomadas

1. **Fechar a janela esconde o app na bandeja; não encerra.** Encerrar é só
   pelo "Sair" (bandeja, menu ou Cmd+Q). Exceção: no GNOME sem bandeja, a
   janela minimiza em vez de esconder (seção 4.3).
2. **Distribuição só pelo GitHub Releases, com um arquivo por sistema.** Nada
   de Mac App Store nem Microsoft Store. Na página de releases:

   | Sistema | Arquivo | O que a pessoa faz |
   |---|---|---|
   | macOS | `WhatsApp-MCP.dmg` | abre e arrasta o app para Aplicativos |
   | Windows | `WhatsApp-MCP-Setup.exe` | roda o instalador, que põe o app numa pasta fixa e cria o atalho no Menu Iniciar |
   | Linux | `WhatsApp-MCP.AppImage` e `.deb` | roda o AppImage, ou instala o `.deb` |

   O `.exe` é um instalador, e não um executável portátil, porque o caminho do
   app precisa ser estável: é ele que fica gravado na configuração do Claude
   Desktop e no "iniciar com o sistema". A atualização automática busca a
   versão nova na mesma página.
3. **O nome é "WhatsApp MCP".** O risco de marca da Meta é conhecido e aceito.
4. **Transcrição na primeira versão do app: só o motor local.** Em computador
   sem GPU, o turbo roda com um aviso de que é mais lento. A chave da OpenAI
   para usar o Whisper por API fica para uma versão futura. Enquanto isso, a
   ferramenta de IA pode baixar o áudio pelo `download_media` e transcrever do
   jeito que achar melhor (seção 5.2).
5. **O app nasce neste repositório** (`BrOrlandi/whatsapp-mcp-v2`).
6. **A linha de comando continua como produto (opção A, abaixo).** O
   `whatsapp-mcp` sem janela segue publicado em cada release, ao lado do app,
   com o `install.sh` e o prompt de instalação no README, e entra na matriz de
   testes. Ele é o caminho para servidores sem tela, máquinas ligadas 24 horas
   e instalação por agente de IA.

### Assinatura (decidida)

As duas últimas decisões, sobre assinatura:

7. **Assinatura no macOS: com a conta Apple Developer que o Bruno já tem.** A
   conta serve para distribuir fora da App Store: o certificado **Developer ID
   Application** existe exatamente para apps baixados de sites e do GitHub, e a
   **notarização** (a Apple examina o app e o "carimba") é feita pela mesma
   conta. Com os dois, o `.dmg` abre sem nenhum aviso. Passos:
   - criar o certificado Developer ID Application em developer.apple.com
     (só o "Account Holder" da conta pode criar, se a conta for de empresa);
   - criar uma chave da API do App Store Connect para o `notarytool` rodar no CI;
   - assinar **todos** os binários dentro do `.app` (o app, o bridge, o wacli e,
     depois de baixados, o whisper-cli e o ffmpeg) com *hardened runtime*;
     notarizar o `.dmg` e "grampear" o carimbo nele (`stapler`), para abrir
     mesmo sem internet.
   - Nada disso coloca o app na App Store nem passa pela revisão dela.

8. **Assinatura no Windows: a primeira versão sai sem assinatura.** Não existe
   assinatura gratuita *e* imediata que evite o SmartScreen:

   | Opção | Custo | Situação |
   |---|---|---|
   | Certificado autoassinado | grátis | **não serve**: o Windows não confia nele, e o aviso continua (ou piora) |
   | Azure Trusted Signing | ~US$ 10/mês | **indisponível no Brasil**: pessoa física só nos EUA e no Canadá; empresa, numa lista de países sem o Brasil |
   | [SignPath Foundation](https://signpath.org) | grátis | para projetos de código aberto: exige repositório **público**, licença aprovada pela OSI (MIT serve), build no CI público e aprovação manual de cada assinatura. **Pontos de atenção:** hoje o repositório é privado, e o programa restringe binários de terceiros dentro do pacote (o wacli vai junto). Precisa de consulta a eles |
   | Certificado OV de uma autoridade (Certum, Sectigo…) | ~US$ 100–300/ano | funciona no Brasil; desde 2023 a chave fica num token físico ou na nuvem da autoridade, o que exige integrar com o CI |

   Mesmo assinado, um app novo ainda pode receber o aviso do SmartScreen nas
   primeiras semanas: a Microsoft dá "reputação" aos poucos, conforme as
   pessoas baixam sem problemas. Desde 2024, nem certificado EV pula essa etapa.

   **Plano:** lançar o `.exe` sem assinatura, com o passo a passo do
   "Mais informações › Executar assim mesmo" no README (duas imagens). Quando o
   repositório ficar público, pedir o SignPath Foundation, consultando antes
   sobre o wacli embutido; se não der, comprar um OV.

#### Sobre a linha de comando (decidido: opção A)

Hoje, a v2 é **só** linha de comando: o `install.sh` instala o executável
`whatsapp-mcp-v2`, que roda como serviço do sistema (`service install`) e não
tem janela. O app desktop substitui isso para quem usa um computador com tela.

A pergunta é o que fazer com esse modo sem janela depois que o app existir:

| Opção | O que significa | Para quem serve |
|---|---|---|
| **A. Manter como produto** | Continua publicado junto com o app (um arquivo a mais no release), com o `install.sh` e o prompt de instalação no README, documentado e testado a cada versão | Servidores Linux sem tela, um Mac mini ligado 24 h, quem prefere terminal ou quer automatizar a instalação com um agente de IA |
| **B. Só ferramenta de desenvolvimento** | O código continua no repositório (o app e os testes dependem dele), mas não é publicado nem documentado para o público; o README passa a falar só do app | Só quem desenvolve o projeto |

O custo de manter a opção A é pequeno, porque o app e a linha de comando usam o
mesmo núcleo (`internal/daemon`): é mais um arquivo no release e mais uma
coluna na lista de testes. O benefício é não perder quem roda em servidor.
**Decidido: opção A.**

## 13. Fora do escopo

- Acesso pelo claude.ai na web e pelo app do celular (exige servidor exposto
  na internet; é o papel da v1).
- Várias contas de WhatsApp no mesmo app.
- Interface de leitura e envio de mensagens dentro do app (o app é a ponte
  para a IA, não um cliente de WhatsApp).
- Lojas de aplicativos (Mac App Store, Microsoft Store): o sandbox delas
  conflita com rodar o wacli e abrir uma porta local.

## 14. Execução

### Ajustes em relação ao plano

- **Autostart:** o Wails v3 já traz (SMAppService, LaunchAgent, `HKCU\…\Run`,
  XDG), então não há `internal/platform/autostart_*`. No macOS o item de início
  não passa argumentos; o app descobre que foi aberto no login pelo evento de
  abertura do sistema. No AppImage, o app escreve a entrada XDG apontando para o
  próprio arquivo `.AppImage`.
- **Atualizador próprio** (`internal/updater`): o do Wails troca só o
  executável, o que não serve para o Windows (app, bridge e wacli juntos) nem
  para o AppImage.
- **Painel na janela:** o webview não manda `Sec-Fetch-Site` nem `Origin` num
  `fetch`, e não segue redirecionamentos do handler em memória. As requisições
  da janela são marcadas como internas, e os redirecionamentos viram uma página
  que navega sozinha.
- **Linux:** GTK 3 com WebKitGTK 4.1 (o padrão do Wails é GTK 4, que o Ubuntu
  22.04 não tem). O AppImage traz o WebKitGTK e seus processos auxiliares, com o
  caminho deles reescrito dentro da biblioteca. Dentro de um AppImage, o Claude
  Desktop inicia o próprio AppImage com `bridge`.
- **whisper-cli no Linux:** compilado pelo projeto, estático e sem OpenMP; o
  oficial depende de `libgomp1`. Em CPU ele fica uns 20% mais lento que o
  oficial com OpenMP, em troca de rodar em qualquer máquina, inclusive Raspberry
  Pi 4.
- **ffmpeg mínimo:** além de Ogg/Opus, lê AAC, MP3 e Vorbis; 1,7 a 2,5 MB.
- **Ícone da barra de menus:** monocromático no macOS (o sistema pinta), com o
  estado na forma: ícone cheio, com um ponto, ou apagado com o ponto. Colorido
  no Windows e no Linux.
- **A linha de comando** passou a se chamar `whatsapp-mcp`; o `install.sh` deixa
  um link `whatsapp-mcp-v2` para as configurações antigas.

### Verificado

- macOS: instalação do zero, QR code, conexão do Claude pelo bridge, troca de
  porta (inclusive porta ocupada ao abrir), fechar e sair pela bandeja, e a
  migração da instalação real, sem novo QR code.
- Linux: o `.deb` no Ubuntu 24.04 e o AppImage num Debian 12 sem WebKit, numa
  tela virtual; transcrição de ponta a ponta no Linux arm64.
- Windows: compila, e o instalador é gerado; o ffmpeg mínimo roda sob Wine.

### Pendências

- **Windows numa máquina de verdade** (o CI roda os testes no
  `windows-latest`, mas a interface, o `CTRL_BREAK` no wacli real, o socket de
  envio e o bridge com o Claude Desktop ainda não foram vistos rodando).
- **Notarização:** o build local sai assinado com o Developer ID; notarizar
  precisa do Issuer ID da chave da API do App Store Connect, e os segredos do CI
  da seção 7.2.
- **Primeiro `sidecars-1`:** rodar o workflow para publicar os pacotes da
  transcrição e fixá-los em `internal/sidecar/manifest.json`. Até lá, o app usa
  um whisper-cli e um ffmpeg já instalados (como os do Homebrew).
- **Repositório público:** com o repositório privado, os downloads das versões e
  dos pacotes não funcionam para quem não tem acesso.
- **Beta** com algumas pessoas em cada sistema antes do anúncio.

