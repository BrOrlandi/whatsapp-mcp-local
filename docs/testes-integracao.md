# Testes de integração

Testes de ponta a ponta do app instalado de verdade, com o WhatsApp pareado e as
ferramentas de IA reais (Claude Desktop, por enquanto). Eles **rodam só sob
demanda**: quando o Bruno pede, antes de uma versão que mexe na instalação ou na
conexão com as ferramentas. Não fazem parte do `go test` nem do CI, porque
precisam de outra máquina, de uma conta do WhatsApp e de uma conta do Claude.

Cada roteiro abaixo é para ser seguido à mão ou por um agente, e diz o que
conferir em cada passo. Os comandos usam estes nomes, que cada um define no seu
ambiente (nunca registre aqui endereços, usuários ou IPs reais: o repositório é
público):

| Nome | O que é |
| --- | --- |
| `TEST_MAC` | o Mac de testes, como `usuario@host` para o `ssh` |
| `VM` | o id da VM do Windows no VMPal (`vmpal list` mostra) |
| `vmpal` | o comando do VMPal: `/Applications/VMPal.app/Contents/Helpers/VMPalMachine.app/Contents/MacOS/VMPalMachine --cli` |

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
