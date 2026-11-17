# Testes de integração

Testes de ponta a ponta do app instalado de verdade, com o WhatsApp pareado.
Não fazem parte do `go test ./...` nem do CI, porque precisam de outra máquina e
de uma conta do WhatsApp. São duas partes:

- **A suíte automática** (`scripts/integration.sh`), que **roda antes de toda
  versão**, no Mac de testes: o painel, a API local, todas as ferramentas do
  MCP, os webhooks, a ponte do Claude Desktop e um reinício do app.
- **Os roteiros com as ferramentas de IA reais** (Claude Desktop no Mac e na VM
  do Windows), mais abaixo, seguidos à mão ou por um agente quando o Bruno pede,
  antes de uma versão que mexe na instalação ou na conexão com as ferramentas.

Os comandos usam estes nomes, que cada um define no seu ambiente (nunca
registre aqui endereços, usuários, números ou IPs reais: o repositório é
público):

| Nome | O que é |
| --- | --- |
| `TEST_MAC` | o Mac de testes, como `usuario@host` para o `ssh` |
| `VM` | o id da VM do Windows no VMPal (`vmpal list` mostra) |
| `vmpal` | o comando do VMPal: `/Applications/VMPal.app/Contents/Helpers/VMPalMachine.app/Contents/MacOS/VMPalMachine --cli` |

## Suíte automática

```sh
scripts/integration.sh                    # compila este checkout, instala no Mac de testes e testa
scripts/integration.sh --no-build         # instala o app que já está em build/bin
scripts/integration.sh --no-install       # testa o app que está instalado lá, como está
scripts/integration.sh -run 'TestApp/messaging'   # o resto vai para o go test
```

O script compila o app para arm64 com a versão `<git describe>-it`, troca o app
do Mac de testes, espera o WhatsApp conectar, abre dois túneis SSH (o app do Mac
de testes numa porta daqui e, no sentido contrário, um receptor de webhooks
daqui numa porta de lá) e roda `go test -tags integration ./integration/`. O
log fica em `build/integration/`. Tudo verde é condição para publicar.

**Configuração.** Em `.env.integration`, na raiz (ignorado pelo git):

| Variável | O que é |
| --- | --- |
| `TEST_MAC` | o Mac de testes (`usuario@host`), pareado com a conta de teste |
| `WMCP_TO` | o número (com DDI) ou JID que recebe as mensagens de teste. O wacli não deixa a conta mandar para si mesma. Tudo o que a suíte manda é apagado para todos no fim. Sem ele, a parte que envia é pulada |
| `WMCP_GROUP` | opcional: um grupo só de teste que a conta administra. A suíte muda a descrição e o nome dele e põe de volta, e pede o link de convite |
| `WMCP_TOKEN` | a `WHATSAPP_MCP_TOKEN` do app, se ele tiver uma |

**O que ela confere**, na ordem:

1. **HTTP:** `/healthz`, `/api/status` (a versão é a compilada, `next` é
   `ready`), `/health` (só a verificação `receiving` pode falhar, porque a conta
   de teste recebe pouco), as páginas do painel sem sobra de template, as
   leituras da API, os logs de suporte e a recusa de um pedido de outra origem.
2. **Protocolo MCP:** `initialize` com instruções, `ping` e `tools/list`, que
   tem de ser exatamente a lista de `expectedTools` em
   `integration/app_test.go`. Uma ferramenta nova falha aqui até ganhar o seu
   teste.
3. **Leituras:** `health`, `whatsapp_status`, `list_chats` (com `fields`),
   `get_chat_messages` (com `count_only`), `get_message_context`,
   `search_messages`, `list_contacts`, `message_stats`, `list_unread`,
   `list_unanswered`, `list_mentions`, `media_stats`, `export_messages` (o
   arquivo existe lá), `check_numbers` e `get_profile_picture`.
4. **API de webhooks:** criar, ler, alterar, a entrega de teste assinada, uma
   URL inválida recusada, apagar.
5. **Mensagens**, para `WMCP_TO`, com um webhook (`include_own`) recebendo tudo:
   `dry_run`, `send_typing`, texto (o webhook chega, assinado, e a busca acha),
   resposta citando, reação (e o webhook dela), edição, o recibo de entrega do
   outro celular, localização, enquete e `get_poll_results`, encaminhar (com e
   sem `dry_run`), imagem, documento e áudio feitos no Mac de testes (`sips`,
   `say`), `download_media` com link, `transcribe_audio`, `save_transcript` (a busca
   acha o texto salvo), `purge_media` da conversa, `sync_history`,
   `mark_chat_read` e a prévia de `delete_message`, que não apaga. Cada envio
   devolve em `chat_jid` a conversa como as leituras a conhecem. No fim, tudo
   o que foi mandado é apagado para todos.
6. **Transcrição:** os áudios de `integration/testdata/audio/` (feitos com a
   ElevenLabs por `scripts/integration-audio.py`, e
   guardados no repositório) vão como áudio para `WMCP_TO` e são transcritos no
   Mac de testes. Cada transcrição tem de trazer as palavras de `expect` em
   `cases.json` (com alternativas, como `quinze|15`), com uma falta tolerada
   por áudio; a segunda chamada devolve a transcrição guardada. Precisa da
   transcrição instalada lá (Configurações › Transcrição de áudio); para um
   caso novo, acrescente-o a `cases.json` e rode
   `ELEVENLABS_API_KEY=… scripts/integration-audio.py`, que só gera os que faltam
   (`--force` refaz todos).
7. **Estado local:** `organise_chat` (fixar, silenciar e arquivar, e desfazer),
   `mark_handled` e `snooze_chat`, e limpar as marcas.
8. **Grupos:** `list_groups` e `get_group`; com `WMCP_GROUP`, `update_group`,
   `get_group_invite_link` (e o `reset` sem `confirm`, que não troca o link) e
   `leave_group` sem `confirm`, que não sai.
9. **API de mídia:** a retenção (mudada e posta de volta) e a prévia de
   `purge_media`.
10. **Ponte:** o `whatsapp-mcp-bridge` do app, rodado lá como o Claude Desktop
   roda, responde `initialize`, `tools/list` e uma chamada.
11. **Reinício:** o app é fechado e aberto de novo, volta a `ready`, manda o
    aviso `online` com `reason` `started` e `offline_since` de pouco antes, e
    fica um só `wacli sync` rodando.

**A conta de teste não pode estar em outro lugar.** Uma cópia da mesma sessão
do wacli (outro computador, um container) faz os dois se derrubarem a cada
poucos segundos: o log de diagnóstico mostra `stream_replaced` ("another client
took over this session"), e os envios falham com `not connected`. Pare a cópia
ou pareie o Mac de testes de novo.

**O Mac de testes não pode dormir.** O script deixa um `caffeinate` de duas
horas rodando lá. Se ele dormir mesmo assim, o SSH para de responder (o ping
continua) e só acorda com alguém na máquina.

## Modo servidor, com Docker

O modo servidor tem uma etapa própria, que roda neste computador, com o Docker
e sem conta do WhatsApp (o wacli da imagem, sem conectar). Ela não faz parte da
suíte do Mac de testes, que não muda:

```sh
go test -tags integration_server -count=1 -v ./integration/
```

Ela monta a imagem a partir deste código (ou usa a que `WMCP_SERVER_IMAGE`
nomear, como `ghcr.io/brorlandi/whatsapp-mcp-local:1.9.0`), sobe o
`compose.yml` com a escuta pública no loopback, como para um proxy que já
existe, e passa pelo que uma pessoa e uma ferramenta fazem pelo endereço
público: a senha errada e a certa, uma chave criada em *Conexões*, uma chamada
de ferramenta com ela, o login OAuth com PKCE (só leitura, e um envio
recusado) e a chave valendo depois de `docker compose down` e `up`. No fim,
apaga o volume.

## Preparar o Mac de testes

Um Mac separado na rede, para não fechar o Claude Desktop de quem desenvolve.

1. **Acesso remoto (SSH)** ligado em Ajustes › Geral › Compartilhamento, e a
   chave de quem vai testar em `~/.ssh/authorized_keys`.
2. A sessão do usuário aberta na tela: o Claude Desktop e o app precisam de uma
   sessão gráfica. Para ele não dormir durante o teste:
   `ssh $TEST_MAC 'nohup caffeinate -dimsu -t 14400 >/dev/null 2>&1 &'`.
3. **Claude Desktop** instalado e logado. O download do site recusa `curl`;
   copie o app de outro Mac:
   `tar -C /Applications -cf - Claude.app | ssh $TEST_MAC 'tar -C /Applications -xf -'`.
4. **VMPal** instalado, do mesmo jeito, para rodar a VM do Windows nele (abaixo).

Pelo SSH não se vê a tela (o `screencapture` só pega o papel de parede sem a
permissão de Gravação de Tela); os testes conferem pelos logs, pelos processos
e pela API do app. A tela da VM do Windows, essa sim, o VMPal mostra.

## Instalar a versão a testar

**Mac:** `ARCHS=arm64 VERSION=X.Y.Z-dev scripts/build-macos.sh` gera
`build/bin/WhatsApp MCP.app` (assinado ad hoc, que roda copiado por `ssh`, sem
quarentena). Feche o app que estiver lá e troque:

```sh
ssh $TEST_MAC 'osascript -e "quit app \"WhatsApp MCP\""; sleep 3; rm -rf "/Applications/WhatsApp MCP.app"'
tar -C build/bin -cf - "WhatsApp MCP.app" | ssh $TEST_MAC 'tar -C /Applications -xf - && open -a "/Applications/WhatsApp MCP.app"'
ssh $TEST_MAC 'curl -s http://127.0.0.1:47821/api/status'   # "version" é a de teste
```

**Windows:** o instalador sai em Docker, de qualquer Mac:

```sh
docker run --rm -v "$PWD:/src" -w /src -e VERSION=X.Y.Z-dev golang:1.26 bash -c \
  'git config --global --add safe.directory /src; apt-get update -qq && apt-get install -y -qq nsis unzip >/dev/null && scripts/build-windows.sh'
```

e vai para a VM (veja "VM do Windows" para rodar o `vmpal` no Mac de testes):

```sh
vmpal send-files $VM build/bin/WhatsApp-MCP-Setup.exe     # cai na área de trabalho da VM
vmpal exec $VM -- powershell.exe -Command '
  Get-Process "WhatsApp MCP",wacli -EA 0 | Stop-Process -Force; Start-Sleep 2
  Start-Process "$HOME\Desktop\WhatsApp-MCP-Setup.exe" -ArgumentList "/S" -Wait
  explorer.exe "$env:LOCALAPPDATA\Programs\WhatsApp MCP\WhatsApp MCP.exe"'
```

## VM do Windows

Uma VM Windows 11 (ARM) no VMPal, com o WhatsApp MCP pareado e o Claude Desktop
da Microsoft Store logado. Ela roda no Mac de testes; o Mac de quem desenvolve
a comanda pelo SSH, rodando lá o `vmpal`, ou pelo VMPal Remote (no VMPal do Mac
de testes, Ajustes › Controle remoto ligado; no outro, Adicionar computador).

```sh
ssh $TEST_MAC "$vmpal list"                       # o id e o estado
ssh $TEST_MAC "$vmpal start $VM --wait"
ssh $TEST_MAC "$vmpal exec $VM -- powershell.exe -Command '…'"
ssh $TEST_MAC "$vmpal ui $VM screenshot -o /tmp/vm.png" && scp $TEST_MAC:/tmp/vm.png .
ssh $TEST_MAC "$vmpal ui $VM click X Y"           # nas coordenadas da captura
ssh $TEST_MAC "$vmpal stop $VM"                   # desliga com segurança
```

- Programas com janela, para durarem além do `exec`: `explorer.exe "<caminho>"`.
  O Claude da Store: `explorer.exe "shell:AppsFolder\Claude_<código>!Claude"`
  (`Get-StartApps Claude` mostra o id).
- Instalar um pacote MSIX (`Add-AppxPackage`) precisa de `exec --admin`.
- O Windows sozinho ocupa uns 35 GB: deixe o limite do disco da VM em 40 GB ou
  mais (`vmpal set $VM --disk 40`). No limite, a VM pausa e o `vmpal info`
  diz por quê.
- Mover a VM para outro Mac: com ela desligada, copie
  `~/Library/Application Support/VMPal/Virtual Machines/<nome>.vmpal`.

## Roteiro: conectar o Claude Desktop

O que a 1.6.1 corrigiu, e o que mais quebra quando o Claude Desktop muda: onde
ele guarda a configuração, e que ele regrava o arquivo enquanto está aberto.

**Onde fica cada coisa**

| | Windows (Claude 4, Store) | macOS |
| --- | --- | --- |
| Configuração | `%LOCALAPPDATA%\Claude-Data\claude_<código>\claude_desktop_config.json` | `~/Library/Application Support/Claude/claude_desktop_config.json` |
| Log principal | `%LOCALAPPDATA%\Claude\logs\main.log` | `~/Library/Logs/Claude/main.log` |
| Ponte | `whatsapp-mcp-bridge.exe` | `whatsapp-mcp-bridge` |

**1. Começar do zero.** Feche o Claude, tire a entrada `whatsapp` da
configuração (com o Claude fechado, senão ele desfaz) e esqueça as conexões no
app, para a tela não achar que já conectou:

```sh
# no app (Windows: Invoke-WebRequest com os mesmos dados)
curl -s -X POST -H "Origin: http://127.0.0.1:47821" -d client=claude-desktop http://127.0.0.1:47821/conexoes/remover
curl -s -X POST -H "Origin: http://127.0.0.1:47821" -d client=client:local-agent-mode-whatsapp http://127.0.0.1:47821/conexoes/remover
curl -s http://127.0.0.1:47821/api/state     # "clients_live": 0
```

Abra o Claude de novo e deixe-o aberto: é o caso real.

**2. O botão.** Em Conectar › Claude Desktop, clique em "Adicionar ao Claude
Desktop" (ou chame a mesma rota que ele chama:
`curl -X POST -H "Origin: http://127.0.0.1:47821" -d '{}' http://127.0.0.1:47821/api/clients/claude-desktop`).
Confira:

- a resposta traz `"ok":true`, `"reopened":true` e `"where"` com o arquivo do
  quadro acima;
- o Claude fecha e abre de novo (no log principal, `beforeQuit` e logo depois a
  abertura; um processo novo do Claude);
- em segundos, a ponte roda e o log principal diz
  `Connected to whatsapp (N tools)`;
- `api/state` lista o cliente `claude-ai` (as conversas) e, quando a aba Code
  é usada, `local-agent-mode-whatsapp`;
- a tela vai sozinha para "Claude Desktop conectado", com a comemoração;
- um minuto depois, a entrada `whatsapp` continua no arquivo, mesmo depois de
  o Claude registrar `Config file written` no log.

**3. A espera.** Sem clicar no botão, a tela de conectar mostra "Checar de
novo" e "Pular e ir para o painel"; os dois funcionam, e a tela continua
checando sozinha por mais de dez minutos.

**4. O prompt.** Em "Prefiro configurar de outro jeito", copie o texto, cole
numa conversa da aba Code do Claude Desktop e siga o que ele pedir. Ele deve
testar a ponte, gravar a configuração e pedir para fechar e abrir o Claude; no
fim, o Claude responde com o número conectado.

**5. Ponte órfã.** Com a ponte rodando, encerre o Claude à força (Windows:
`Stop-Process` nos processos do Claude; macOS: `kill -9`). A ponte deve sair
junto em segundos.

## Linux

Ainda sem roteiro: o Claude Desktop não tem versão para Linux. Quando houver um
teste a fazer lá (o app, o AppImage, o `.deb`, o Claude Code), a VM entra no
VMPal do mesmo jeito.
