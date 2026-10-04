# Plano: WhatsApp MCP como app desktop

> Status: proposta, para revisão. Nada deste plano está implementado.

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
| O endereço do MCP (`http://127.0.0.1:47821/mcp`) e as tools | O app roda o daemon dentro do próprio processo, em vez de um serviço do sistema |
| O wacli como motor do WhatsApp | O wacli passa a vir dentro do app, sem Homebrew |
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
  (Decisão em aberto, seção 12: a alternativa é fechar = sair.)
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

### 1.5 Desinstalar

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
| `whatsapp-mcp-bridge` | console | o processo stdio que o Claude Desktop inicia; só repassa para `127.0.0.1:47821` |
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
 │    ├─ porta 47821 ocupada por outro? → mostra erro claro na janela
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
| Assinatura | Certificado de assinatura de código. Opções: Azure Trusted Signing (~US$ 10/mês, reputação boa no SmartScreen) ou certificado OV/EV. Sem assinatura, o SmartScreen mostra "O Windows protegeu o computador" |
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
  próprio áudio. A página Transcrição avisa e oferece o modelo `small` (~470
  MB) como opção mais rápida. Decisão em aberto, seção 12.
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

### 6.3 `internal/daemon`

- Extrair do `main.go` a montagem: índice, estado, supervisor, motor de
  transcrição, servidor MCP, painel e HTTP.
- `Start` retorna erros que a janela consegue mostrar (porta em uso, wacli
  ausente ou corrompido, pasta sem permissão).
- Expor um canal de eventos de estado (conectado, reconectando, desconectado
  pelo WhatsApp, histórico chegando) para a bandeja e as notificações, em vez
  de a bandeja ficar consultando.

### 6.4 `internal/panel`

- Aceitar a origem `wails://wails` (e o equivalente de cada sistema), além de
  `http://127.0.0.1:47821`, na verificação de mesma origem das ações.
- Página nova **Configurações**: iniciar com o sistema, fechar = esconder ou
  sair, pasta de dados (abrir no Finder/Explorer), versão e atualizações, apagar
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
| sidecars | os três | whisper-cli (macOS), ffmpeg mínimo (os três), sha256 |

Todos os jobs rodam `go test ./...` antes de empacotar. O release no GitHub é
criado por tag `v*`, com `checksums.txt`.

### 7.2 Segredos necessários

| Segredo | Para quê | Custo |
|---|---|---|
| Certificado Developer ID + senha do app para notarização | macOS | Apple Developer Program, US$ 99/ano |
| Azure Trusted Signing (ou certificado OV/EV) | Windows | ~US$ 10/mês (ou US$ 200–400/ano) |
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

### 9.2 Ponta a ponta (manual, antes de cada release)

| Cenário | macOS | Windows | Linux |
|---|---|---|---|
| Instalação limpa, QR code, Claude conectado | ☐ | ☐ | ☐ |
| Código pelo número de telefone | ☐ | ☐ | ☐ |
| Fechar a janela: MCP segue respondendo | ☐ | ☐ | ☐ |
| Sair: nenhum wacli fica rodando | ☐ | ☐ | ☐ |
| Matar o app à força: na volta, assume o sync | ☐ | ☐ | ☐ |
| Iniciar com o sistema: reinicia o computador e o app sobe na bandeja | ☐ | ☐ | ☐ |
| Desligado 30 min: mensagens recuperadas ao reabrir | ☐ | ☐ | ☐ |
| Transcrição: instalar, transcrever um áudio sob pedido | ☐ | ☐ | ☐ |
| Atualização automática de uma versão para a seguinte | ☐ | ☐ | ☐ |
| Migração da instalação v2 atual (só macOS) | ☐ | — | — |
| Gatekeeper / SmartScreen sem avisos | ☐ | ☐ | — |

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

Janela, bandeja, primeira abertura no QR code, Configurações, autostart com
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

- README: em vez do prompt + `curl`, "Baixe o app para o seu sistema", com os
  três botões e o prompt como alternativa para quem quer a linha de comando.
- Capturas novas, `docs/` atualizados (dependências, caminhos por sistema).
- Beta com algumas pessoas em cada sistema antes do anúncio.

---

## 11. Riscos

| Risco | Impacto | Mitigação |
|---|---|---|
| Wails v3 ainda em beta | um bug de janela ou bandeja sem correção rápida | prova na Fase 0; Wails v2 + biblioteca de bandeja como plano B; fixar a versão do Wails |
| wacli no Windows menos testado pelo projeto deles | sync instável ou encerramento ruim no Windows | Fase 0 dedicada; reportar e contribuir com correções no wacli; Job Object garante ao menos que nada fique órfão |
| Custo e burocracia de assinatura | sem assinatura, avisos assustadores no macOS e no Windows | decidir cedo (seção 12); dá para publicar um beta sem assinatura, com instruções de "abrir mesmo assim" |
| WhatsApp mudar o protocolo | a conexão para em todos os sistemas | o app avisa na bandeja; release rápido com o wacli atualizado; o updater entrega em horas |
| Antivírus no Windows estranhar o app que baixa e roda binários | bloqueio da transcrição | assinar todos os binários; baixar só do nosso release; verificar sha256 |
| Transcrição lenta em CPU | experiência ruim no Windows e Linux sem GPU | aviso claro; modelo `small` como opção; CUDA quando houver NVIDIA |
| Duas cópias rodando (app e serviço antigo) | brigam pela sessão do WhatsApp | migração remove o LaunchAgent; o app detecta a porta e o lock ocupados e explica |
| GNOME sem bandeja | o app "some" ao fechar a janela | detectar a falta de bandeja e só minimizar |

---

## 12. Decisões em aberto

1. **Fechar a janela:** esconde na bandeja (proposta) ou encerra o app?
2. **Assinatura de código:** pagar Apple (US$ 99/ano) e Azure Trusted Signing
   (~US$ 10/mês) desde a primeira versão pública, ou começar com um beta sem
   assinatura?
3. **Nome do app:** "WhatsApp MCP"? Usar "WhatsApp" no nome de um app pode
   esbarrar nas regras de marca da Meta. Alternativas como "Zap MCP" ou
   "Conversas MCP" evitam o problema.
4. **Transcrição em CPU:** manter o turbo como único modelo, com aviso, ou
   oferecer o `small` automaticamente quando não houver GPU?
5. **Linha de comando:** manter o `whatsapp-mcp` (serve, service install) como
   produto suportado para servidores, ou só como ferramenta de desenvolvimento?
6. **Repositório:** o app nasce neste repositório (proposta) ou num novo?

## 13. Fora do escopo

- Acesso pelo claude.ai na web e pelo app do celular (exige servidor exposto
  na internet; é o papel da v1).
- Várias contas de WhatsApp no mesmo app.
- Interface de leitura e envio de mensagens dentro do app (o app é a ponte
  para a IA, não um cliente de WhatsApp).
- Lojas de aplicativos (Mac App Store, Microsoft Store): o sandbox delas
  conflita com rodar o wacli e abrir uma porta local.
