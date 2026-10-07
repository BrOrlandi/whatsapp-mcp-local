# Plano: novas ferramentas, segurança e webhook

> Status: implementado em outubro de 2026, menos a segurança, que virou um
> [plano à parte para o futuro](plano-seguranca.md). O que mudou no caminho
> está na seção 9. Nasceu da comparação com o
> [Tauri-EPO/whatsapp-mcp](https://github.com/Tauri-EPO/whatsapp-mcp) (um
> servidor Docker para devs, fork do lharries/whatsapp-mcp). Daquele projeto
> vêm as ideias; a implementação é nossa, sobre o wacli 0.20.

## 0. Resumo

**O que entra.** Cinco grupos de ferramentas (envio, grupos, triagem, leituras
grandes, leitura de mídia), um webhook para mensagens recebidas configurável
pela API local e uma camada de segurança com tela própria.

**O que fica de fora, por decisão.** A "memória" do agente (anotações sobre
conversas, contatos e arquivos) e o arquivo de mensagens apagadas e de
visualização única. Também fica de fora, por ora, expor a mídia como resource
MCP (seção 6).

**Tudo é viável com o que temos.** O wacli 0.20 já faz quase todo o envio e a
administração de grupos; triagem, estatísticas e exportação são consultas ao
banco do wacli, que já lemos em modo só leitura. O que exige mais trabalho é
ler PDF escaneado sem CGO (seção 6) e saber quem foi mencionado, que o wacli
não guarda (seção 4).

---

## 1. Como o wacli limita o desenho

Isto decide o custo de cada ferramenta, por isso vem primeiro.

- O `wacli sync --follow` que o supervisor mantém rodando segura o lock do
  banco o tempo todo. Comandos que só leem funcionam em paralelo.
- Enquanto roda, o sync atende um socket (`<store>/.send.sock`) e executa por
  ele: todos os `send` (texto, arquivo, voz, reação, local, enquete),
  `presence`, `messages edit` e `chats mark-read/mark-unread/archive/pin/mute`
  e os inversos. Esses comandos **não pausam o sync** (no código: o caminho
  delegado, `internal/wacli/supervisor.go`).
- O resto precisa do lock: `messages delete/revoke/forward`, toda alteração de
  grupo, `history backfill`, `media retry`. Esses **pausam o sync** por alguns
  segundos (o supervisor o para, roda o comando e o reinicia), como já fazem
  hoje `delete_message`, `check_numbers` e o `get_group` ao vivo.

Achado lateral: no 0.20, arquivar, fixar e silenciar passaram a ir pelo
socket. O `organise_chat` ainda pausa o sync à toa; dá para trocar já.

**Cada ferramenta nova ganha uma categoria** na definição (ler, enviar, apagar,
grupos, organizar). Custa nada agora e é o que a segurança (seção 8) usa para
esconder e recusar ferramentas.

---

## 2. Envio

| Ferramenta | Como | Pausa o sync? |
|---|---|---|
| Responder citando (`reply_to`) em `send_text_message` e `send_media_message` | `send text/file --reply-to --reply-to-sender` | não |
| @menções (`mentions`) em `send_text_message` | `send text --mention` (o texto precisa ter `@<número>`) | não |
| `dry_run` nos envios | nosso: valida, resolve o destinatário (JID e nome) e devolve o que seria enviado, sem enviar | não |
| `forward_message` | `messages forward`: encaminhamento de verdade, com o selo "Encaminhada" (o do Tauri-EPO reenvia o conteúdo) | sim |
| `mark_chat_read` | `chats mark-read --receipts` (manda confirmação de leitura de até 100 mensagens) | não |
| `send_typing` | `presence typing` / `paused` | não |
| Apagar só para mim | `delete_message` ganha `for_me`: `messages delete --for-me`. Sem ele, continua apagando para todos | sim |

O `dry_run` segue o padrão de prévia que o `delete_message` já tem. Vale
também para `forward_message`.

## 3. Administração de grupos

Todas pelo wacli, todas pausam o sync (são ações raras, aceitável):

- `manage_group_participants(group_jid, action, participants)`: adicionar,
  remover, promover, rebaixar (`groups participants add|remove|promote|demote`).
- `update_group(group_jid, name?, description?)` (`groups rename`, `groups description`).
- `get_group_invite_link(group_jid, reset?)` (`groups invite link get|revoke`).
  Trocar o link invalida o antigo: conta como ação de grupo, não leitura.
- `leave_group(group_jid)` (`groups leave`).

Remover gente e sair do grupo usam prévia e `confirm`, como o `delete_message`.

## 4. Triagem

| Ferramenta | Como |
|---|---|
| `list_unread` | `chats.unread_count` (o wacli sincroniza com o celular; no 0.20 ler no celular zera a contagem) + as últimas N mensagens recebidas de cada conversa |
| `list_unanswered` | consulta ao banco: conversas cuja última mensagem "de fala" veio do outro lado (ignora reações, mensagens apagadas, votos de enquete, status). Opções: período, idade mínima, incluir grupos, ignorar fechamentos como "ok", "obrigado", 👍 |
| `list_mentions` (o `mentions_me` deles) | **com ressalva:** o wacli não guarda quem foi mencionado. Mas o texto de uma menção sempre traz `@<número>`, então dá para buscar `@<meu telefone>` e `@<meu LID>` nas mensagens de grupo. Os dois vêm do `session.db`, que já lemos para nomes. Alternativa robusta: contribuir no wacli uma coluna com os mencionados |
| `mark_handled` / `snooze_chat` | tabela nova no nosso `state.db`. A conversa some das listas enquanto a marca for mais nova que a última mensagem recebida (ou até o fim do adiamento); mensagem nova a traz de volta. Nada é enviado, o outro lado não vê |

`list_unanswered` ganha `include_group_mentions`, juntando os grupos onde
alguém te mencionou depois da sua última mensagem lá.

## 5. Leituras grandes

Todas por consulta ao banco do wacli, sem pausar nada:

- `get_message_context(chat_jid, message_id, before, after)`: as mensagens em
  volta de um resultado de busca.
- Leituras enxutas em `get_chat_messages`, `search_messages` e `list_chats`:
  `fields` (só os campos pedidos), `max_content_chars` (corta textos longos,
  marcando o corte) e `count_only` (só a contagem).
- `message_stats(group_by: chat|day|month|sender, filtros)`: contagens para o
  agente dimensionar um trabalho ("quem mais fala", "quantas mensagens por mês")
  sem ler o histórico.
- `export_messages(filtros)`: grava NDJSON em `<DataDir>/exports` e devolve o
  caminho, a contagem e o período. O `messages export` do wacli só faz um JSON
  único; escrevemos o NDJSON nós mesmos, linha a linha.

## 6. Leitura de mídia

> **Cancelado.** O `read_media` chegou a ser implementado (pacote
> `internal/mediaread`, com o PDFium em WebAssembly) e foi retirado antes de
> publicar a 1.3.0, por decisão: a função do MCP é dar acesso aos arquivos do
> WhatsApp, e processá-los é trabalho da ferramenta de IA que os usa. A
> transcrição de áudio fica, porque a IA não tem outro jeito de ouvir um áudio;
> uma foto ou um PDF ela já lê. O retirado: cerca de 10 MB no app, cinco
> dependências (`go-pdfium`, `wazero`, `go-commons-pool`, `golang.org/x/image`,
> `golang.org/x/text`) e o app interpretando arquivos mandados por terceiros.
>
> No lugar, o `download_media` entrega o arquivo exatamente como chegou, sem
> reduzir nem converter, e um arquivo que não cabe no resultado (ou que o
> cliente não aceita) vem por um link deste computador (`link: true`, e sempre
> acima de 20 MiB). Ficaram o inventário, a limpeza e a retenção dos arquivos
> baixados.

O plano original, para registro:

`read_media(chat_jid, message_id)` entrega o conteúdo pronto para o modelo ler.
O `download_media` continua existindo para guardar o arquivo ou gerar um link.

- **Imagens:** reduzidas para ~1568 px no lado maior, sem metadados, girando
  pelo EXIF. Em Go puro (`golang.org/x/image`), que lê JPEG, PNG, GIF, WebP,
  TIFF e BMP. HEIC fica de fora (não há decodificador em Go puro; o WhatsApp
  converte fotos para JPEG ao enviar, HEIC só chega como documento).
- **DOCX e XLSX:** texto extraído em Go puro (zip + XML; `excelize` para
  planilhas). Com limites de páginas, linhas e caracteres.
- **PDF com texto:** extração em Go.
- **PDF escaneado** (renderizar páginas como imagem): resolvido com o
  `go-pdfium` no modo WebAssembly (o PDFium rodando via wazero, em Go puro),
  que também extrai o texto de PDFs. Medido: cerca de 10 MB a mais no binário,
  1,5 s para preparar o leitor no primeiro PDF e 75 a 140 ms por página
  desenhada. Compila sem CGO para Windows, Linux e macOS. O pacote é
  `internal/mediaread`.
- **Áudio:** o `download_media` já devolve áudio como áudio, mas o Claude não
  ouve; o caminho é o `transcribe_audio`.
- **Quem ganha:** o Claude Desktop (chat e Cowork), que não lê arquivos do
  disco. O Claude Code já lê imagem e PDF pelo caminho que o `download_media`
  devolve.
- **Inventário e limpeza:** `media_stats` (quanto ocupa, por conversa e por
  tipo, duplicados) e `purge_media` (com prévia antes). Uma opção de retenção
  em dias apaga o que for velho; um arquivo apagado volta a ser baixado quando
  pedido (e, se expirou no WhatsApp, `media retry` pede ao celular de novo).
- **Resource MCP (`whatsapp://media/...`): fora por ora.** Nosso servidor MCP é
  escrito à mão e só fala de ferramentas; `read_media` cobre o mesmo uso.

---

## 7. Webhook de mensagens recebidas

**Para que serve.** Um script neste computador (ou na rede) recebe um POST a
cada mensagem nova e age por conta própria: responder automaticamente,
registrar numa planilha, avisar em outro lugar, acionar uma automação.

**Como funciona por baixo.** O `wacli sync` já tem webhook (`--webhook`,
`--webhook-secret` com assinatura em `X-Wacli-Signature`, `--webhook-events
message,receipt,chat_presence`, `--webhook-allow-private` para localhost). Mas
ele é "melhor esforço": 5 s de prazo, sem novas tentativas, e não conta para
ninguém quando falha. Então:

```
wacli sync ──POST──▶ app (endpoint interno, segredo por execução) ──POST──▶ webhooks configurados
```

- O sync sempre aponta para o próprio app. Configurar, trocar ou apagar
  webhooks não reinicia o sync.
- O app entrega a cada webhook, com assinatura própria (HMAC-SHA256 com o
  segredo daquele webhook), novas tentativas e registro da última entrega.
- O payload é nosso e estável (não o formato interno do wacli): id, conversa e
  nome, remetente e nome, horário, `from_me`, texto, resposta a, reação, tipo e
  nome do arquivo de mídia. Sem os bytes da mídia: o script pede pelo MCP ou
  pela API se quiser.
- Respeita a lista de conversas permitidas da seção 8.
- De brinde, o app passa a saber na hora que chegou mensagem (hoje o `health`
  consulta o banco quando perguntado).

**Fila, novas tentativas e desligamento automático** (decidido):

- Cada webhook tem a sua fila, entregue em ordem. Enquanto uma entrega falha,
  os eventos novos esperam na fila atrás dela.
- Só HTTP 200 conta como entregue. Uma entrega que falha é tentada de novo, em
  intervalos crescentes, até 10 tentativas que cabem em menos de 1 minuto.
- Na 10ª falha o webhook é desligado, com o motivo e a hora guardados, e **a
  fila de pendentes é descartada**. Religar pela API começa com a fila vazia.

**API local** (mesmo acesso das rotas de agente que já existem):

| Rota | O que faz |
|---|---|
| `GET /api/webhooks` | lista, com estado (ligado/desligado e motivo) e última entrega |
| `POST /api/webhooks` | cria: `url`, `events` (mensagem, reação, confirmação de leitura), `include_own` (incluir as mensagens que você manda), conversas opcionais. Devolve o segredo uma única vez |
| `PATCH /api/webhooks/{id}` | altera; `enabled: true` religa |
| `DELETE /api/webhooks/{id}` | apaga |
| `POST /api/webhooks/{id}/test` | manda um evento de teste e diz o que voltou |

Uma seção na tela de Configurações mostra os webhooks e o estado de cada um.

---

## 8. Segurança

Saiu deste plano: está em [plano-seguranca.md](plano-seguranca.md), para o
futuro, com listas de conversas permitidas e bloqueadas. A base ficou pronta
aqui: cada ferramenta tem uma categoria (`internal/mcp/categories.go`).

---

## 9. O que foi feito, e o que mudou no caminho

Tudo das seções 2 a 7 foi implementado, com testes (o fake wacli de
`internal/wacli/testdata` aprendeu os comandos novos, e
`internal/daemon/tools_test.go` passa as ferramentas pelo `/mcp` de ponta a
ponta). Diferenças em relação ao plano:

- **`delete_message` com `for_me`** também pede prévia e `confirm`, como o
  apagar para todos.
- **`organise_chat` e `mark_chat_read` tentam o socket do sync primeiro** e,
  se o wacli em uso for mais antigo e recusar, caem na pausa. A linha de
  comando pode rodar um wacli instalado à parte.
- **Uma resposta só cita mensagem da mesma conversa.** Uma de outra conversa é
  recusada antes de chegar ao wacli, que também recusaria.
- **Uma menção precisa estar no texto** como `@<número>`; a ferramenta recusa
  em vez de inserir sozinha, para não mudar o texto que a pessoa aprovou.
- **`snooze_chat` substitui um "resolvido"** anterior: a conversa volta na hora
  marcada.
- **Retenção da mídia e webhooks** ganharam cards em Configurações (Webhooks
  logo depois das opções do app, Arquivos baixados perto de Dados), que usam a
  mesma API local dos scripts (`internal/panel/assets/settings.js`). O card de
  webhooks leva a `/webhooks/documentacao`, com o JSON de cada tipo de aviso
  gerado dos próprios tipos que o app envia (`internal/panel/webhookdocs.go`). A
  varredura da retenção roda um minuto depois de abrir e a cada seis horas.
- **Qualquer resposta 2xx conta como entregue**, não só 200: um script que
  responde 204 está funcionando. Cada tentativa espera até 3 segundos; as 10
  tentativas cabem em menos de um minuto.
- **O relay do webhook fica sempre ligado**, numa porta própria de loopback:
  criar, mudar ou apagar webhooks não reinicia o sync. Se o wacli em uso não
  tiver webhook, o sync roda sem ele e a API responde `"available": false`.
- **`read_media` foi implementado e retirado antes da 1.3.0** (seção 6): o
  MCP entrega o arquivo original, e a IA processa.
- **Mídia expirada no WhatsApp não é pedida de novo ao celular
  automaticamente.** O `media retry` do wacli escolhe sozinho quais mídias
  pedir, pausando o sync por até meio minuto; a mensagem de erro agora explica
  que o arquivo pode estar no celular.

## 10. Decisões dos pontos em aberto

- Desligamento do webhook: depois de 10 tentativas em menos de um minuto, com
  a fila descartada (decisão do Bruno).
- `read_media`: cancelado; o `download_media` entrega o arquivo original, com
  um link local quando ele não cabe no resultado.
- `list_mentions` busca `@<número>` e `@<LID>` no texto; uma coluna de
  mencionados no wacli continua sendo a alternativa robusta.
