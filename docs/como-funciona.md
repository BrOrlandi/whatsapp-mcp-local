# Como funciona

```
Claude Code ──HTTP──┐
                    ├──► whatsapp-mcp-v2 serve (127.0.0.1:47821)
Claude Desktop ─┐   │        │
  (Chat, Cowork)│   │        ├─ supervisiona  wacli sync --follow   (dono da sessão e do lock)
  stdio: bridge ┘───┘        ├─ envia via     socket de delegação do sync
                             ├─ lê            wacli --read-only --json + wacli.db (somente leitura)
                             └─ guarda        ~/.whatsapp-mcp-v2 (transcrições, chave OpenAI, mídia)
```

- **Um único daemon é dono da sessão.** O wacli permite um só processo com o
  lock do store. O daemon mantém o `wacli sync --follow` sempre rodando, e todos
  os clientes falam com ele. Nenhum cliente sobe um segundo WhatsApp.
- **Envios** (texto, arquivo, localização, enquete, reação, edição) vão para o
  sync que já está rodando, sem interrompê-lo.
- **Operações que precisam do lock** (histórico, revogar mensagem,
  arquivar/fixar/silenciar, verificar números, foto de perfil) pausam o sync
  por alguns segundos, rodam e religam o sync. O WhatsApp entrega na reconexão
  o que chegou nesse intervalo, e os envios feitos durante a pausa esperam na
  fila em vez de falhar.
- **Leituras** usam `wacli --read-only` e uma conexão SQLite somente leitura,
  que o wacli documenta como segura durante o sync. Nada escreve no `wacli.db`.
- **Nomes** vêm, em ordem: do nome do grupo, do apelido, da agenda do celular
  (lida só em leitura da sessão do WhatsApp), dos nomes guardados nas mensagens
  e do nome de perfil. Sem nenhum deles, aparece o número.
- **O Claude Desktop** só inicia servidores locais por stdio, então ele roda o
  `whatsapp-mcp-v2 bridge`, que repassa cada mensagem para o mesmo daemon. O
  Cowork usa os servidores do Claude Desktop.

## Pareamento e histórico

O painel roda o `wacli auth` e mostra o QR code, ou um código de 8 caracteres
para digitar no celular. Depois que o celular conecta o dispositivo, o celular
começa a mandar o histórico. Numa conta grande isso leva bem mais de meia hora,
e o sync recebe esse histórico em segundo plano enquanto você já usa o Claude.

Para ir mais para trás, o painel e a tool `sync_history` pedem ao celular as
mensagens anteriores à mais antiga que o computador já tem de cada conversa. O
celular precisa estar com internet, e cada pedido recua mais um trecho.

## Transcrição de áudios

Em Macs com Apple Silicon, as notas de voz são transcritas no próprio
computador com o [whisper.cpp](https://github.com/ggml-org/whisper.cpp) e o
modelo `large-v3-turbo` (cerca de 600 MB), rodando na GPU, como no
[Handy](https://github.com/cjpais/Handy). O instalador prepara tudo; a página
Transcrição do painel também instala com um clique.

A conversa ajuda em duas etapas:

1. **Na transcrição.** O Whisper recebe como contexto o nome da conversa, as
   pessoas e as últimas mensagens de texto, e escreve nomes e termos como a
   conversa os escreve. É por isso que o motor é o Whisper e não o Parakeet,
   que não aceita contexto.
2. **Na revisão.** A tool `transcribe_audio` devolve a transcrição com as
   mensagens ao redor e pede ao Claude que corrija o que o contexto mostra que
   foi mal ouvido. A correção é guardada com `save_transcript`, e o texto
   original fica guardado ao lado.

Nada é transcrito sem pedido: um áudio só passa pelo Whisper quando você pede
à sua ferramenta de IA para lê-lo. Depois disso a transcrição fica guardada, e
as tools de leitura e o painel a mostram junto do áudio. Fora do Mac, ou por
escolha, a transcrição pode ser feita pela OpenAI com uma chave salva no
painel.

## Segurança

- O daemon escuta só em `127.0.0.1`.
- Ele recusa `Host` que não seja loopback (proteção contra DNS rebinding) e
  qualquer `Origin` que não seja o próprio servidor, então nem outra aba de um
  `localhost` em outra porta consegue chamar as tools.
- As ações do painel (conectar o WhatsApp, editar a configuração do Claude) só
  aceitam chamadas da própria página, verificadas pelos headers `Sec-Fetch-Site`
  e `Origin`.
- Com `WHATSAPP_MCP_TOKEN` definido, toda chamada precisa de
  `Authorization: Bearer <token>`, o que fecha o endpoint também para outros
  programas deste computador.
- O conteúdo das mensagens é escrito por terceiros. Os resultados das tools
  avisam o modelo para tratá-lo como dado, nunca como instrução.

## Configuração

| Variável | Padrão |
|---|---|
| `WHATSAPP_MCP_PORT` | `47821` |
| `WHATSAPP_MCP_TOKEN` | vazio (sem token) |
| `WACLI_BIN` | `wacli` no PATH ou no Homebrew |
| `WACLI_STORE_DIR` | o padrão do wacli (`~/.wacli` no macOS) |
| `WHATSAPP_MCP_DATA` | `~/.whatsapp-mcp-v2` |

Comandos: `whatsapp-mcp-v2 serve | bridge | service install | service uninstall | open | config | version`.
Logs: `~/Library/Logs/whatsapp-mcp-v2.log` (macOS) ou
`journalctl --user -u whatsapp-mcp-v2` (Linux).

## Tools

A aba **Documentação** do painel lista as tools direto do servidor. Em relação
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
