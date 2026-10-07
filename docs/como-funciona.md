# Como funciona

```
WhatsApp MCP (o app: um processo só)
├── janela ──────────── painel, servido em memória, sem passar pela rede
├── ícone na barra de menus / bandeja
├── MCP em 127.0.0.1:47821 ◄── Claude Code (HTTP)
│                          ◄── whatsapp-mcp-bridge (stdio) ◄── Claude Desktop / Cowork
├── supervisor ──► wacli sync --follow   (dono da sessão e do lock)
│        ├─ envia pelo socket de delegação do sync
│        └─ cada mensagem nova ──► relay em 127.0.0.1:<porta aleatória> ──► webhooks
├── leituras ───► wacli --read-only --json + wacli.db (somente leitura)
└── transcrição sob pedido ──► ffmpeg ──► whisper-cli
```

A versão de linha de comando (`whatsapp-mcp serve`) roda o mesmo núcleo, sem a
janela e sem a bandeja, como serviço do sistema.

- **Um único daemon é dono da sessão.** O wacli permite um só processo com o
  lock do store. O daemon mantém o `wacli sync --follow` sempre rodando, e todos
  os clientes falam com ele. Nenhum cliente sobe um segundo WhatsApp.
- **Envios** (texto, arquivo, localização, enquete, reação, edição, "digitando")
  e as mudanças de estado das conversas (marcar como lida, arquivar, fixar,
  silenciar) vão para o sync que já está rodando, sem interrompê-lo. Um wacli
  mais antigo, que não aceita essas mudanças pelo socket, cai na pausa abaixo.
- **Operações que precisam do lock** (histórico, apagar mensagem, encaminhar,
  administrar grupos, verificar números, foto de perfil) pausam o sync por
  alguns segundos, rodam e religam o sync. O WhatsApp entrega na reconexão o
  que chegou nesse intervalo, e os envios feitos durante a pausa esperam na
  fila em vez de falhar.
- **Triagem, estatísticas e exportação** são consultas ao `wacli.db`. As marcas
  de "resolvido" e "adiado" ficam no `state.db` do app, junto das transcrições:
  nada disso chega ao WhatsApp.
- **Webhooks.** O sync entrega cada mensagem nova a um relay do próprio app, num
  endereço de loopback com porta aleatória e um segredo novo a cada execução.
  O app repassa a cada webhook configurado, com fila, novas tentativas e
  desligamento automático ([api.md](api.md#webhooks)).
- **Leituras** usam `wacli --read-only` e uma conexão SQLite somente leitura,
  que o wacli documenta como segura durante o sync. Nada escreve no `wacli.db`.
- **Nomes** vêm, em ordem: do nome do grupo, do apelido, da agenda do celular
  (lida só em leitura da sessão do WhatsApp), dos nomes guardados nas mensagens
  e do nome de perfil. Sem nenhum deles, aparece o número.
- **O Claude Desktop** só inicia servidores locais por stdio, então ele roda o
  `whatsapp-mcp-bridge` que vem dentro do app (na linha de comando,
  `whatsapp-mcp bridge`), que repassa cada mensagem para o mesmo processo. O
  bridge lê a porta no `config.json` do app e lê de novo quando o app não
  responde, então trocar a porta não exige mexer no Claude Desktop. O Cowork usa
  os servidores do Claude Desktop.

## O app

- **Um processo só.** O núcleo (`internal/daemon`) roda dentro do app. Fechar a
  janela esconde o app na barra de menus; **Sair** para o sync com cuidado (até
  20 segundos para o wacli fechar o banco) e o MCP deixa de responder. O mesmo
  acontece quando o computador desliga ou a sessão do usuário termina.
- **Instância única.** Abrir o app de novo só traz a janela da instância que já
  está rodando.
- **O wacli vem dentro do app**, numa versão fixa e testada (hoje a 0.20.0). Ele
  só muda quando sai uma versão nova do app.
- **Abrir com o sistema.** No macOS 13 ou mais novo, um item de início
  (SMAppService), que aparece em *Ajustes › Geral › Itens de Início*; antes
  disso, um LaunchAgent. No Windows, um valor em
  `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`. No Linux, um arquivo em
  `~/.config/autostart`. Em todos, o app abre só na bandeja.
- **Linux sem bandeja.** O GNOME puro não mostra ícones de bandeja sem a
  extensão AppIndicator. Sem ela, fechar a janela só a minimiza, para o app
  nunca ficar rodando sem um jeito de voltar a ele.
- **Atualizações.** Duas vezes por dia o app consulta a versão mais recente no
  GitHub. Ao instalar, ele baixa o arquivo do sistema, confere o sha256 contra o
  `checksums.txt` da versão (e, no macOS, que a assinatura é do mesmo
  desenvolvedor), troca o app e reinicia nele. O `.deb` é atualizado pelo
  pacote: o app só avisa e aponta a página da versão.

## Pareamento e histórico

O painel roda o `wacli auth` e mostra o QR code, ou um código de 8 caracteres
para digitar no celular. Depois que o celular conecta o dispositivo, o celular
começa a mandar o histórico. Numa conta grande isso leva bem mais de meia hora,
e o sync recebe esse histórico em segundo plano enquanto você já usa o Claude.

Para ir mais para trás, o painel e a tool `sync_history` pedem ao celular as
mensagens anteriores à mais antiga que o computador já tem de cada conversa. O
celular precisa estar com internet, e cada pedido recua mais um trecho.

## Transcrição de áudios

Os áudios são transcritos no próprio computador com o whisper.cpp e o modelo
large-v3-turbo, usando a conversa como contexto, e só quando você pede: na GPU
do Mac, numa placa NVIDIA no Windows, ou no processador. O caminho completo, do pedido à revisão pelo Claude, está em
[transcricao.md](transcricao.md).

## Dependências

O que é crítico, o que é opcional, as versões em uso e onde ficam os dados estão
em [dependencias.md](dependencias.md).

## Segurança

- O MCP escuta só em `127.0.0.1`.
- A janela do app carrega o painel em memória, sem passar pela rede, e abre
  qualquer link externo no navegador do sistema.
- Ele recusa `Host` que não seja loopback (proteção contra DNS rebinding) e
  qualquer `Origin` que não seja o próprio servidor, então nem outra aba de um
  `localhost` em outra porta consegue chamar as tools.
- As ações do painel (conectar o WhatsApp, editar a configuração do Claude,
  trocar a porta) só aceitam chamadas da própria página: pela rede, verificadas
  pelos headers `Sec-Fetch-Site` e `Origin`; no app, porque só a janela alcança
  o painel em memória.
- Com `WHATSAPP_MCP_TOKEN` definido, toda chamada precisa de
  `Authorization: Bearer <token>`, o que fecha o endpoint também para outros
  programas deste computador.
- O conteúdo das mensagens é escrito por terceiros. Os resultados das tools
  avisam o modelo para tratá-lo como dado, nunca como instrução.

## Configuração

O app guarda em `config.json`, na pasta de dados, o que é lido antes de tudo e
pelo bridge: a porta, se abre com o sistema e se fechar a janela esconde ou
encerra. O resto (transcrições, clientes, etapa da instalação) fica no
`state.db`. A página **Configurações** muda tudo isso.

| Pasta de dados | |
|---|---|
| macOS | `~/Library/Application Support/WhatsApp MCP` (log em `~/Library/Logs/WhatsApp MCP`) |
| Windows | `%LOCALAPPDATA%\WhatsApp MCP` (log em `logs\`) |
| Linux | `~/.local/share/whatsapp-mcp` (log em `~/.local/state/whatsapp-mcp`) |

As variáveis de ambiente valem para o app e para a linha de comando, e vencem o
`config.json`:

| Variável | Padrão |
|---|---|
| `WHATSAPP_MCP_PORT` | `47821` (no app, a da página Configurações) |
| `WHATSAPP_MCP_TOKEN` | vazio (sem token) |
| `WHATSAPP_MCP_PROXY` | o campo `whatsapp_proxy` do `config.json`; vazio herda o ambiente do wacli ([proxy e Tailscale](proxy-tailscale.md)) |
| `WACLI_BIN` | o wacli de dentro do app; na linha de comando, o instalado ao lado ou no PATH |
| `WACLI_STORE_DIR` | `<pasta de dados>/wacli`; na linha de comando, o padrão do wacli (`~/.wacli`) |
| `WHATSAPP_MCP_DATA` | a pasta de dados acima; na linha de comando, `~/.whatsapp-mcp` |

Linha de comando: `whatsapp-mcp serve | bridge | service install | service uninstall | service stop | service start | open | config | transcription install | version`.
Logs do serviço: `~/Library/Logs/whatsapp-mcp.log` (macOS) ou
`journalctl --user -u whatsapp-mcp` (Linux).

## Tools

A aba **Funções** do painel lista as tools direto do servidor. Em relação
ao [WhatsApp MCP hospedado](https://github.com/BrOrlandi/whatsapp-mcp):

| Diferença | Por quê |
|---|---|
| `send_contact` não existe | o wacli não envia cartão de contato (vCard) |
| `sync_history` não tem `before`; ganhou `rounds` | o wacli sempre parte da mensagem mais antiga de cada conversa; `rounds` recua mais vezes na mesma chamada |
| `download_media` devolve `path` | o arquivo já está no disco, e um cliente com acesso a arquivos lê direto dali |
| `health` | um veredito (ok, warn ou fail) sobre o daemon, o pareamento, o sync e a chegada de mensagens; o mesmo relatório responde em `GET /health`, com 503 quando algo falha |

## Limitações de rodar localmente

- **O índice só cresce com o daemon rodando.** Com o computador desligado, o
  WhatsApp guarda uma fila para o dispositivo e entrega na reconexão. Para
  períodos longos essa entrega não é garantida, e um dispositivo inativo por
  semanas é desconectado.
- **Buracos só se preenchem para trás.** Não existe como pedir "o que veio
  depois de X". Um período perdido numa conversa que ficou quieta só pode ser
  buscado quando chegar uma mensagem nova nela.
- **Mídia antiga expira no CDN do WhatsApp.** Nesse caso,
  `wacli media retry --chat <jid>` pede ao celular que envie o arquivo de novo.
