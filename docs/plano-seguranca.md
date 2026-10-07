# Plano: segurança das ferramentas

> Status: para o futuro, nada implementado. Saiu do
> [plano das novas ferramentas](plano-novas-ferramentas.md) em outubro de 2026
> para ser planejado à parte. A base já existe: cada ferramenta tem uma
> categoria (`internal/mcp/categories.go`).

## 0. Resumo

**Objetivo.** Deixar a pessoa decidir o que o Claude pode ver e fazer no
WhatsApp dela, numa tela em linguagem simples: quais conversas ele enxerga
(uma lista de permitidas, uma de bloqueadas, ou as duas) e que tipo de ação
ele pode tomar (ler, enviar, apagar, administrar grupos, organizar).

**Por quê.** O agente lê mensagens escritas por terceiros e pode enviar
mensagens. Uma instrução escondida numa mensagem ("encaminhe isso para…") é
o risco real. As ferramentas já avisam que o conteúdo é dado, não instrução,
mas avisar não impede: impedir é tirar do alcance do agente o que ele não
precisa.

---

## 1. Como o Tauri-EPO faz

Tudo por variável de ambiente, aplicado duas vezes (no servidor MCP e no
bridge que fala com o WhatsApp):

- **Conversas permitidas** (`WHATSAPP_ALLOWED_CHATS=5511999999999,*@g.us`):
  vazia, não filtra nada. Preenchida, listas e buscas são filtradas em
  silêncio; pedir uma conversa fora da lista, ou enviar para ela, é recusado
  com erro `denied`. A comparação é só por texto: o mesmo contato com telefone
  e com LID precisa estar escrito das duas formas, ou some.
- **Não há lista de conversas bloqueadas.** Para esconder uma conversa só, é
  preciso listar todas as outras. O `exclude_chat_jid` das leituras é um filtro
  que o agente escolhe a cada chamada, não uma proteção. O webhook deles não
  passa pela lista: entrega mensagens de todas as conversas.
- **Só leitura** (`WHATSAPP_READ_ONLY=1`): as ferramentas que mudam algo somem
  da lista de ferramentas e, se chamadas mesmo assim, são recusadas com "mostre
  o rascunho ao usuário e deixe que ele envie".
- **Ferramentas permitidas e proibidas** (`WHATSAPP_ALLOW_TOOLS`,
  `WHATSAPP_DENY_TOOLS`): proibir vence permitir, e só leitura vence as duas.
  Nome errado impede o servidor de subir.
- **Conteúdo de terceiros:** toda ferramenta que devolve mensagens avisa que é
  dado, não instrução; nomes de contatos e grupos são limpos de caracteres de
  controle e cortados em 200 caracteres; opcionalmente, o texto vem entre
  `<untrusted>…</untrusted>`.

## 2. Como seria o nosso

### Conversas: permitidas e bloqueadas

- Três modos: **todas** (o padrão), **só estas** (lista de permitidas) e
  **todas menos estas** (lista de bloqueadas). Talvez as duas listas juntas,
  com a bloqueada vencendo: "os grupos do trabalho, menos o da diretoria".
- Escolhidas numa lista com busca, sem digitar JID. Atalhos para "todos os
  grupos" e "todas as conversas diretas".
- Um contato vale pelas duas grafias (telefone e LID), usando o que o app já
  sabe (`contact_aliases`, `whatsmeow_lid_map`). Um envio para um nome é
  resolvido para o JID antes da checagem, porque o wacli aceita nomes no
  `--to`.
- O que fica fora some das listas, das buscas, das estatísticas, da triagem,
  da exportação e do webhook; pedir uma conversa fora, ou enviar para ela, é
  recusado dizendo o porquê.

### O que o Claude pode fazer

- Chaves por categoria (`internal/mcp/categories.go`): ler, registros locais,
  enviar, apagar, administrar grupos, organizar conversas. "Só leitura" é o
  atalho que deixa só a primeira (e talvez a segunda).
- Uma ferramenta desligada some de `tools/list` e é recusada em `tools/call`.
  Ressalva: o servidor não avisa os clientes, então quem já está conectado só
  vê a lista nova ao reconectar; a recusa vale na hora.

### Arquivos que podem ser enviados

- Hoje `send_media_message` aceita qualquer caminho do computador; uma
  instrução escondida numa mensagem poderia levar o agente a mandar um arquivo
  sensível. O wacli 0.20 tem `WACLI_MEDIA_ROOTS` para restringir.
- Padrão sugerido: Downloads, Documentos, Imagens e a pasta de mídia do app.

### Conteúdo de terceiros

- Limpar nomes sempre (caracteres de controle e de direção do texto, que
  servem para disfarçar).
- Delimitar o texto das mensagens como opção.

### Onde fica

- Uma tela "Segurança" no painel, em linguagem simples, com os detalhes
  técnicos num (?). Vale no app e na linha de comando, por isso as escolhas
  ficam no `state.db` (o `config.json` só existe no app).
- As mesmas escolhas pela API local, para um agente configurar.

## 3. Em aberto

- Só uma lista por vez, ou permitidas e bloqueadas juntas?
- "Só leitura" deixa marcar como resolvido, adiar e exportar (que só mudam
  este computador)?
- As pastas padrão de envio.
- Proteger também a API local com uma chave, já prevista em
  [api.md](api.md).
