# Plano: vários números de WhatsApp

> Status: fase 1 implementada (o número como unidade e o nome do número);
> fase 2 em preparação. Nasceu de pedidos de usuários em outubro
> de 2026 e foi fechado numa conversa com o Bruno em 2026-10-10. É um recurso
> **pago**: cada número além do primeiro tem mensalidade. O código que gerencia
> mais de um número fica num repositório privado.

## 0. Resumo

**Objetivo.** Conectar mais de um número no mesmo app (pessoal, trabalho,
loja), todos ativos ao mesmo tempo, cada um com a sua conexão MCP.

**Regra principal.** Quem tem um número só não percebe nada: o nome da
conexão (`whatsapp`), a URL, as pastas, os webhooks e as configurações dos
clientes continuam iguais, e atualizar o app não pede nada.

**Como a IA escolhe o número.** Pelo nome da conexão. Cada número é um
servidor MCP separado, com as mesmas ferramentas; nenhuma ferramenta recebe a
conta e nenhuma junta os números. Para "o que tenho de não lido nos dois", o
agente chama as duas conexões.

## 1. Conexões e rótulo

- **Mesmo processo, mesma porta, uma URL por número.** O principal fica em
  `/mcp`; cada extra em `/mcp/<slug>` (ex.: `/mcp/trabalho`).
- **O slug nasce do rótulo e não muda.** Renomear o rótulo depois não muda a
  URL nem o nome da conexão, para não quebrar a config dos clientes. Para
  trocar a URL, remove e adiciona o número de novo.
- **Rótulo simples e opcional, para todos**, inclusive quem tem um número só:
  "Pessoal", "Trabalho". Sem rótulo, vale o nome de perfil do WhatsApp.
- **Onde o rótulo aparece:**
  - nome técnico da conexão nos clientes: `whatsapp-trabalho` (Claude Code,
    Codex e Cursor não aceitam espaço). O principal continua `whatsapp`;
  - `serverInfo.title = "WhatsApp Trabalho"`, que os clientes mais novos
    mostram;
  - no painel, na lista de conexões: "Claude Code · WhatsApp Trabalho";
  - no `whatsapp_status`, que ganha `account: {label, phone, name}`. Hoje o
    número só aparece como `linked_jid` dentro da saída crua do `wacli doctor`
    (`internal/mcp/status.go`), e o nome nem isso;
  - nas instructions do servidor: uma frase com o rótulo e o telefone, **só
    quando houver dois números ou mais**. Com um número, o texto fica igual.

## 2. Telas, bandeja e linha de comando

- **Adicionar número fica na aba WhatsApp.** A tela inicial e o fluxo de
  instalação não mudam; lista e seletor de números só aparecem com dois ou
  mais.
- **Bandeja:** um ícone só, com o tom do pior estado entre os números (no
  macOS continua template). O menu mostra uma linha por número ("Trabalho ·
  conectado") e a notificação de desconexão diz qual número caiu.
- **Linha de comando, com a mesma licença:** `whatsapp-mcp entrar <email>`,
  `whatsapp-mcp numero adicionar trabalho` (QR no terminal ou código pelo
  telefone), `numero listar`, `numero remover trabalho`,
  `whatsapp-mcp config --numero trabalho`. O `serve` sobe todos os números no
  mesmo serviço (`com.brorlandi.whatsapp-mcp`) e na mesma porta.
- **Limite: 10 números no total** (o principal mais 9 extras). Ao tentar
  passar disso, o app mostra o contato whatsappmcp@brorlandi.xyz.

## 3. Dados de cada número

- O principal não sai do lugar: `<DataDir>/wacli`, `state.db`, `media/`,
  `exports/`.
- Cada extra ganha `<DataDir>/numeros/<slug>/` com o seu `wacli/` (store e
  sessão do wacli, um `wacli sync` por número), `state.db` (triagem,
  transcrições, webhooks, configurações), `media/` e `exports/`. Remover um
  número é deslogar e apagar a pasta.
- Compartilhados: `config.json`, os modelos e programas de transcrição
  (`models/`, `bin/`), `updates/` e a licença.
- **Webhooks separados por número.** Todo payload ganha
  `account: {slug, label, phone}`, inclusive o do principal; é campo novo, não
  quebra script.

## 4. Open core: o que é aberto e o que é pago

- **Repositório público (MIT), de graça:** a refatoração para "a conta como
  unidade" (um supervisor do wacli, um store e um `state.db` por conta), o
  rótulo, o `account` no `whatsapp_status` e nos webhooks.
- **Repositório privado** (`BrOrlandi/whatsapp-mcp-pro`): o que gerencia mais
  de uma conta (adicionar número, as rotas `/mcp/<slug>`, escrever as
  conexões `whatsapp-<slug>` nos clientes), a licença, as telas de assinatura
  e o código do Worker de assinaturas.
- **Build:** o código privado é **sobreposto** à cópia do código público que
  o `scripts/release.sh` clona, e tudo compila com `-tags pro` (`.dmg`,
  instalador do Windows, AppImage, `.deb`, linha de comando). Não dá para usar
  um `go.work` com um módulo separado: o Go não deixa outro módulo importar os
  pacotes `internal/` deste, e é neles que está tudo. A sobreposição põe
  `internal/pro/` e um arquivo `cmd/app/zz_pro.go` (com `//go:build pro`) na
  árvore clonada; os dois caminhos ficam no `.gitignore` do público, para o
  desenvolvimento local com o privado ao lado. O público não cita o privado: o
  privado se registra num pacote de ganchos (`internal/extend`). O CI público
  compila e testa sem nada disso; sem o privado, a aba WhatsApp mostra
  "disponível na versão oficial". Os nomes dos arquivos da release não mudam.
- A trava é razoável, não absoluta: alguém determinado reescreve o
  gerenciador. O que não pode é a verificação morar no código público, onde
  bastaria apagá-la.

## 5. Assinatura

- **Worker de assinaturas na Cloudflare**, `whatsapp-mcp-assinaturas`, em
  `assinatura.whatsapp-mcp.brorlandi.xyz` (o nome evita confusão com o
  `whatsapp-mcp-license-worker`, que ativa o Evolution Go da versão servidor).
  Tem o **seu próprio painel de administração** em `/admin` (lista de
  clientes, conceder, mudar e tirar números extras, histórico), atrás de um
  token e do Cloudflare Access. Conceder pelo painel é o que permite
  desenvolver e testar tudo antes do gateway de pagamento, e depois serve para
  cortesias e suporte. O código de entrar chega pelo Resend.
  Ele guarda as chaves dos provedores e recebe os webhooks deles; o app nunca
  fala com o provedor, porque teria de levar a chave secreta no binário.
- **Token assinado.** O Worker responde com um token Ed25519 ("este e-mail tem
  N números extras até a data D"); o app confere com a chave pública embutida.
- **Entrar por e-mail com código.** "Já assinei" pede o e-mail da compra; o
  Worker manda um código de 6 dígitos e soma as assinaturas ativas daquele
  e-mail. Sem chave de licença para perder.
- **Validação diária, 14 dias de tolerância offline.** Assinatura vencida ou
  cancelada: os extras pausam (o sync para e a conexão responde "assinatura
  vencida, renove em…"), os dados ficam, e **o principal nunca é afetado**.
- **Provedores:**
  - lançamento: **Asaas, só cartão**, pelo checkout recorrente, numa conta PJ
    (CNPJ 28.910.643/0001-91);
  - depois: **Pix Automático pela Woovi**, com valor variável, no mesmo Worker;
  - fora: o Pix comum do Asaas (fatura manual todo mês, R$ 1,99 por Pix).
- **Preço por quantidade, tabela no Worker.** Mensal e regressivo (ideia
  inicial: R$ 20 por 1 extra, R$ 30 por 2); os valores ficam para depois. O
  Worker calcula o valor pela quantidade e grava na assinatura.
- **Mudar a quantidade sem proporcional.** Aumentar libera o número na hora e
  o valor novo vale a partir da próxima cobrança. Diminuir pede para escolher
  qual número sai, e ele fica até o fim do mês pago.
- **Sem período de teste**; o número principal já é a amostra grátis.
- **A tela de assinatura fica na aba WhatsApp** ("Assinatura · 2 números
  extras · R$ 30/mês", mudar quantidade, trocar cartão, cancelar). Nenhum dos
  provedores tem portal do cliente, então essa tela é nossa.

## 6. Site e termos

- `/precos`: a tabela e o botão de assinar (checkout do Asaas pelo Worker).
- `/termos`: sem afiliação com a Meta; o Bruno não responde por número banido
  por mau uso ou disparo automático. O app pede o "aceito" antes da primeira
  compra.
- `/privacidade`: o que o Worker guarda (e-mail, id da assinatura), por causa
  da LGPD.
- `/assinatura`: cancelar sem o app.
- O Bruno aceita o risco de cobrar por um cliente não oficial (whatsmeow).

## 7. Fases

1. **Aberta e grátis:** conta como unidade, rótulo, `account` no
   `whatsapp_status` e nos webhooks. Release minor, nada muda para quem usa.
2. **Paga:** vários números, Worker com Asaas, telas de assinatura, páginas do
   site.
3. **Pix:** Woovi no Worker.

## 7a. Fase 2, passo a passo

Feito na fase 1: `daemon.Account` (store, sync, índice, `state.db`, servidor
MCP e webhooks de um número), `account.Info` (rótulo, telefone, nome), o nome
na aba WhatsApp, `account` no `whatsapp_status`, nos webhooks e no
`/api/status`, o título da conexão e a frase nas instructions com dois números
ou mais.

No público (ganchos, sem lógica paga):
1. `internal/extend`: ganchos que o privado preenche no `init` (ao subir o
   daemon, rotas extras, seção da aba WhatsApp, comandos da linha de comando,
   itens da bandeja).
2. `Daemon.AddAccount(spec)` e `RemoveAccount(slug)`, e um handler
   `/mcp/{slug}` que procura o número na hora; o `Stop` e o `SetPort` já
   percorrem todos.
3. A bandeja e o `daemon.Status` com o pior estado entre os números e uma
   linha por número.
4. `scripts/release.sh`: com `PRO_DIR`, sobrepõe o privado e compila com
   `GOFLAGS=-tags=pro` (também nos builds em Docker); `.gitignore` com
   `internal/pro/` e `cmd/*/zz_pro.go`.

No privado (`BrOrlandi/whatsapp-mcp-pro`):
5. A lista de números extras (`<DataDir>/numeros.json`), subir cada um ao
   abrir o app, adicionar (pareamento por QR ou código), remover (deslogar e
   apagar a pasta), o slug a partir do rótulo e o limite de 10.
6. Escrever as conexões `whatsapp-<slug>` no Claude Code, Claude Desktop,
   Codex e Cursor, e "Claude Code · WhatsApp Trabalho" na lista do painel.
7. A licença: entrar por e-mail com código, guardar o token, conferir a
   assinatura Ed25519 com a chave pública embutida, renovar uma vez por dia,
   14 dias sem internet, pausar os extras quando vence.
8. As telas na aba WhatsApp: lista de números, adicionar, assinatura
   (quantidade, valor, trocar cartão e cancelar, estes dois ligados ao gateway
   depois) e o aviso do limite com o e-mail de contato.
9. Linha de comando: `entrar`, `numero adicionar|listar|remover`,
   `config --numero`.
10. O Worker (`worker/`): login por código, direitos de uso no D1, token
    assinado, painel `/admin`, tabela de preços; webhooks e checkout do Asaas
    ficam como stubs.
11. Site: `/precos`, `/termos`, `/privacidade`, `/assinatura`, com o botão de
    assinar desligado até o gateway.

Fica para o gateway: checkout do Asaas, webhooks de pagamento, mudança de
valor e cancelamento; depois, Pix pela Woovi.

## 8. O que o código assume hoje (pontos de mudança)

- O painel recusa um segundo pareamento: "este computador já está conectado a
  um WhatsApp" (`internal/panel/panel.go`, `/api/pair`).
- Um `*daemon.Daemon` no app, um `wacli.Supervisor` com um pareador, um
  `mcp.Server` com estado em memória (histórico, links de mídia, clientes).
- `ServerName = "whatsapp"` fixo em `internal/panel/clients.go`, que escreve as
  configs de Claude Code, Claude Desktop, Codex e Cursor.
- `state.db` único (`transcripts`, `settings`, `clients`, `triage`,
  `webhooks`), com a triagem chaveada só por `chat_jid`.
- Um relay de webhook por daemon; payloads sem conta
  (`internal/webhook/webhook.go`).
- Bandeja e `daemon.Status` sem conta (`cmd/app/tray.go`,
  `internal/daemon/status.go`).
- `EraseEverything` desloga uma sessão e apaga o `DataDir` inteiro.

## 9. Pendências fora do código

- Conta PJ no Asaas: cadastro em andamento (e-mail asaas@brorlandi.xyz).
  Depois: chaves de sandbox e produção, NFS-e automática, e pedir ao gerente a
  tokenização de cartão (sem ela não dá para mudar o valor de uma assinatura
  de cartão).
- Testar no sandbox se `PUT /v3/subscriptions/{id}` aceita `value` (o FAQ diz
  que sim, o schema publicado não mostra o campo).
- whatsappmcp@brorlandi.xyz hoje não chega no Gmail: a regra de e-mail manda
  esse endereço para o `whatsapp-mcp-license-worker`, que descarta o que não é
  `whatsappmcp+<id>@` porque `FALLBACK_ADDRESS` não está definido.
- Woovi PJ travada em "Aguardando verificação" do BC Protege+.
