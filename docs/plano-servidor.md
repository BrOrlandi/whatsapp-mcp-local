# Plano: modo servidor simples, e o caminho até a 2.0

> Status: proposta de 2026-11-17, nada implementado. Substitui o desenho do
> "modo servidor" da branch `BrOrlandi/cli-headless` (o app do computador como
> controle de um servidor remoto), que foi testado nas `v2.0.0-beta` e ficou
> complexo demais para o que entrega. As decisões marcadas **A decidir** ainda
> dependem do Bruno.

## 0. Resumo

**Objetivo.** Rodar o WhatsApp MCP num servidor Linux ou em Docker, fácil de
instalar e de cuidar, no lugar da versão servidor antiga
(`BrOrlandi/whatsapp-mcp-server`). Quem instala abre o painel no navegador com
uma senha e liga os agentes de IA pelo endereço `https` do servidor, inclusive
o Claude no navegador e no celular.

**O que muda em relação à branch.** Sai o app do computador como controle do
servidor: o pareamento de aparelhos, o canal cifrado próprio, o modo remoto do
app e a mudança do número entre computador e servidor. Entra o que faltava: o
painel na web com senha e o login que o Claude do celular exige.

**Três versões.**

| Versão | O que é |
|---|---|
| 1.8.0 | O que já está pronto no `main` (registro para suporte, wacli 0.26.1, evento `online`, correções) |
| 1.9.0 | O modo servidor simples, deste plano, lançado sem divulgação |
| 2.0.0 | A versão comercial: vários números, controles por conexão, o Orquestrador, licença por e-mail, domínio novo, repositório renomeado |

## 1. O que a pessoa faz

Com Docker, num servidor que já tem um endereço `https`:

```sh
curl -fsSL https://raw.githubusercontent.com/BrOrlandi/whatsapp-mcp-local/main/install-docker.sh -o install-docker.sh
bash install-docker.sh
```

O instalador pergunta o endereço público e a senha do painel, escolhe como o
`https` chega (seção 4), grava o `compose.yaml` e o `.env`, e sobe o
contêiner. Depois:

1. Abre `https://whatsapp.exemplo.com`, entra com a senha e lê o QR code na
   tela (ou `whatsapp-mcp pair` no terminal, como hoje).
2. Em *Conectar ferramenta de IA*, escolhe o agente. O painel mostra o
   endereço do MCP e, conforme o agente, pede para entrar com a senha ou cria
   uma chave.

Sem Docker, o `install.sh` de hoje instala o serviço, e
`whatsapp-mcp server setup` faz as mesmas perguntas.

## 2. Desenho

### Duas escutas

- **Local, como hoje:** `127.0.0.1:47821`, sem senha, para os programas do
  próprio servidor e para a linha de comando. Continua recusando qualquer
  pedido que não seja para `localhost`.
- **Pública, só no modo servidor:** outra porta (`47822`), onde **tudo** pede
  login: painel, API, MCP, mídia e o QR code de conexão. É para ela que o
  proxy ou o túnel aponta.

A separação existe por causa do túnel. Um túnel roda no próprio servidor e
chega pelo `127.0.0.1`; apontado para a escuta local com o cabeçalho `Host`
trocado, ele deixa o painel e as conversas abertos para a internet, sem senha.
Com a escuta pública, não existe configuração de túnel que abra o servidor sem
login.

### Variáveis

| Variável | O que faz |
|---|---|
| `WHATSAPP_MCP_PUBLIC_URL` | O endereço `https` do servidor. Liga o modo servidor |
| `WHATSAPP_MCP_PANEL_PASSWORD_HASH` | A senha do painel, em bcrypt (`$2a$`, `$2b$` ou `$2y$`, custo mínimo 10). Texto puro é recusado |
| `WHATSAPP_MCP_PUBLIC_LISTEN` | Onde a escuta pública atende (padrão `127.0.0.1:47822`; no Docker, `0.0.0.0:47822`) |
| `WHATSAPP_MCP_TRUSTED_PROXY` | Quem pode falar com a escuta pública, quando outros contêineres dividem a rede do proxy: endereços, faixas CIDR ou nomes. Só dele se acreditam o `X-Forwarded-For` e o `X-Forwarded-Proto`. Sem a variável, quem está no loopback é o proxy (um túnel no próprio servidor chega por ali, e o loopback já alcança a escuta local, que não pede nada) |

- Com só uma das duas primeiras, o serviço não sobe e diz qual falta.
- `whatsapp-mcp password hash` pede a senha duas vezes e imprime o hash. No
  `compose.yml`, cada `$` do hash vira `$$`; o instalador grava no `.env`
  entre aspas simples, onde isso não é preciso.
- Fora do Docker, `whatsapp-mcp password set` guarda o hash na pasta de
  dados. A variável vence o arquivo.
- Sem senha definida (nem variável, nem arquivo), o modo servidor não sobe.
  Não existe "criar a senha no primeiro acesso": ninguém chega antes do
  dono.
- Trocar a senha encerra as sessões do painel e os logins dos agentes.

### Painel na web

- Tela de senha, cookie de sessão `Secure`, `HttpOnly` e `SameSite=Lax`
  (decidido na fase 1: com `Strict`, um link para o painel aberto de um app de
  conversa chegaria sem sessão), proteção contra CSRF e checagem de `Origin`
  contra o endereço público.
- Sessões só na memória: um reinício pede a senha de novo.
- Espera crescente depois de senhas erradas, por endereço (o IPv6 pelo seu
  `/64`), de alguns segundos no endereço que todo mundo divide quando o proxy
  não diz quem é. Para todos juntos, um balde de verificações da senha (20 de
  uma vez, depois uma a cada 4 segundos, cerca de 21.600 por dia) e no máximo
  duas verificações de bcrypt ao mesmo tempo. Com o balde vazio, cada vez
  vai para uma das verificações que esperam, sorteada (uma fila estrita com
  desistência em 5 segundos deixa o fim dela sem vez numa enxurrada). O
  navegador em que a senha certa foi digitada fica conhecido (um cookie
  aleatório, só o hash dele no `state.db`, no máximo 20, encerrado pela troca
  de senha) e tem uma reserva própria, de qualquer endereço e depois de um
  reinício, sem a espera por endereço: uma enxurrada atrasa o dono em
  segundos, e a conta nunca é trancada. Sem esse cookie, a vez chega na
  proporção das tentativas. Estar ocupado não conta como senha errada, uma
  vez que não chegou à verificação é devolvida, e o limite batido aparece no
  registro uma vez por minuto.
- É o mesmo painel de hoje. O que só faz sentido no computador da pessoa
  (escrever a configuração do Claude Desktop, abrir no login) some no modo
  servidor.

### MCP por URL: dois jeitos de entrar

O conector do Claude no navegador e no celular, e o do ChatGPT na web, só
aceitam "sem login" ou login por OAuth; não há onde colar uma chave. A versão
servidor antiga, que só tem chave, não funciona com eles (o painel dela avisa
isso). Por isso o `/mcp` público aceita duas credenciais:

| Jeito | Para quem | Como |
|---|---|---|
| **Login (OAuth)** | Claude no navegador e no celular, ChatGPT na web | A pessoa cola o endereço do MCP no conector; abre uma página do servidor que pede a senha do painel e mostra qual agente quer acesso |
| **Chave de API** | Claude Code, Codex, Hermes, Grok Bot, n8n, scripts | `Authorization: Bearer <chave>`, criada no painel ou com `whatsapp-mcp apikey new` |

O login é um servidor OAuth 2.1 mínimo, dentro do daemon:

- descoberta (`/.well-known/oauth-protected-resource` e
  `/.well-known/oauth-authorization-server`);
- o cliente se identifica por um documento de metadados (*Client ID Metadata
  Document*, o preferido na especificação do MCP de 2026-07-28, que o Claude e
  o ChatGPT usam), buscado só na internet, com tempo e tamanho limitados, ou
  pelo registro (`/oauth/register`, mantido para compatibilidade): limitado
  por endereço, no máximo 10 registros sem uso por endereço, apagados depois
  de uma hora;
- código de autorização com PKCE `S256` obrigatório, o `resource` (RFC 8707)
  conferido, o `iss` em toda resposta (RFC 9207) e o `redirect_uri` conferido
  por igualdade, menos a porta num endereço de loopback (RFC 8252);
- a página de autorização mostra em destaque o endereço para onde a pessoa
  volta, porque o nome o aplicativo escolhe; a primeira aprovação de um
  aplicativo para um endereço sempre pede a senha, e nada vem marcado;
- token de acesso curto (1 hora) e token de renovação trocado a cada uso. A
  renovação é automática, feita pelo próprio agente, e não tem prazo: a
  conexão vale até ser revogada em *Conexões* ou até a senha mudar;
- só o hash dos tokens fica no `state.db`; reusar um código ou um token de
  renovação já usado corta a conexão.

As chaves de API, reescritas a partir da branch, ficam em tabelas do
`state.db` (a branch usava um arquivo): só o SHA-256 é guardado, a chave
aparece uma vez, e a que ninguém usa nas primeiras 24 horas deixa de valer.
Trocar a senha não corta as chaves.

O painel lista as duas coisas juntas em *Conexões* (agente, quando entrou,
último uso) e corta qualquer uma na hora.

### Permissões por conexão

Cada login e cada chave é uma conexão com as suas próprias permissões,
guardadas no servidor. A tela de autorização do login é onde a pessoa as
escolhe, na hora de deixar o agente entrar:

| | Na tela de autorização (e ao criar uma chave) |
|---|---|
| **Grátis** | "Este agente só lê": vê e busca, não envia, não apaga, não mexe em grupos |
| **Pro** | Quais conversas este agente alcança (todas, só estas, todas menos estas) e quais tipos de ferramenta ele pode usar |

- As permissões são da conexão, não um pedido do agente: o agente não
  escolhe o que recebe, e não consegue pedir mais depois.
- Dá para mudar depois em *Conexões*, sem o agente entrar de novo. A recusa
  vale na hora; a lista de ferramentas que o agente vê muda quando ele
  reconecta.
- Sem o Pro, a tela mostra as opções pagas desligadas, com o caminho para
  assinar.
- Na escuta local não há login, então não há conexão para marcar: lá o
  controle do Pro vale para todos os programas do computador de uma vez.

### O que muda nas ferramentas para um agente de fora

- `send_media_message` com um caminho de arquivo é recusado: um agente de fora
  não pode mandar ler arquivos do servidor. Vale endereço da internet, buscado
  por um cliente que recusa o próprio servidor e a rede dele
  (`internal/netguard`).
- Os links do `download_media` saem com o endereço público e valem pelo
  próprio segredo (192 bits, dez minutos), sem credencial. O resultado não
  traz o caminho no servidor, e o `export_messages` dá um link em vez do
  caminho.
- `check_numbers`, a foto de perfil e o `get_group` atualizado perguntam ao
  WhatsApp como o dono: para quem vem de fora, um limite por conexão (20 de
  uma vez, depois um por minuto).

### Programas que só falam por comando

O Claude Desktop liga um MCP por um comando local, não por endereço. Para
eles, `whatsapp-mcp bridge --url https://whatsapp.exemplo.com/mcp` com a chave
numa variável faz a ponte, sem o app instalado.

## 2a. Desenho técnico: quem está chamando

Hoje o código não sabe quem faz cada pedido. A única pergunta é "veio deste
computador?" (`httpserver.Guard`), mais um token único e opcional. A 1.9
precisa de uma etiqueta em cada pedido, o **chamador**, com o que ele pode
fazer. Levantamento do `main` em 2026-11-17.

### O chamador

Um pacote novo (`internal/caller`) com um tipo que viaja no contexto do
pedido:

| Tipo | Nasce em | Alcança |
|---|---|---|
| `local` | Escuta local, depois do `Guard` e do token, como hoje | Tudo |
| `panel` | Escuta pública, com a sessão do painel | Painel e API |
| `oauth` | Escuta pública, com um token de acesso válido | `/mcp` e `/media/`, com as permissões da conexão |
| `apikey` | Escuta pública, com uma chave válida | O mesmo |

O chamador carrega: o tipo, o id e o nome da conexão (para *Conexões* e para
o registro de suporte), se é só leitura, e um espaço para as regras do Pro
(conversas e ferramentas), que o código público só repassa.

Ele substitui as marcas soltas de hoje e da branch (`WithClientHint`,
`WithVia`, `WithDevice`, o `internalKey` da janela do app), que viram campos
dele.

### Onde ele é conferido: quatro pontos

O levantamento mostra que quase tudo já passa por poucos lugares:

1. **As rotas do painel passam por três embrulhos** em
   `internal/panel/panel.go`: `page` (páginas), `form` (formulários) e `api`
   (a API em JSON). Quase todas, não todas (veja o ponto 4). A checagem do
   chamador entra nesses três, não rota por rota, e a classe de cada rota
   está numa tabela só (`internal/httpserver/routes.go`): o que só faz
   sentido no computador do servidor (encerrar, apagar, a porta, as
   atualizações, abrir pasta, escrever a configuração de uma ferramenta de
   IA) é `LocalOnly`, que só o chamador `local` alcança.
2. **O `/mcp`** em `internal/httpserver/server.go`, que hoje confere só o
   token.
3. **A lista e a chamada de ferramentas** em `internal/mcp/server.go`. Cada
   ferramenta já tem uma categoria (`internal/mcp/categories.go`: ler,
   registros locais, enviar, apagar, grupos, organizar), então "só leitura" é
   um filtro por categoria nesses dois pontos.
4. **O que fica fora dos embrulhos** e precisa de checagem própria na escuta
   pública: `/media/`, `/health`, `/api/pair/qr.png` (o QR code de conexão),
   `/diagnostico/logs.txt`, `POST /configuracoes/encerrar` e `/apagar`, os
   redirecionamentos dos endereços antigos (`/estado`, `/documentacao`,
   `/transcricao`, abertos por não terem dado nenhum), `/healthz` e os
   arquivos estáticos (`/assets/`, ícones), que também ficam abertos.

Um cuidado: o embrulho `page` hoje deixa qualquer navegação passar, de
propósito, porque no computador da pessoa outra origem não consegue ler a
página. Na escuta pública, toda página pede sessão e manda para a tela de
senha.

### O que cada chamador alcança

| Rotas | `local` | `panel` | `oauth` / `apikey` |
|---|---|---|---|
| Páginas do painel, formulários | sim | sim | não |
| `/api/*` (estado, conversas, webhooks, mídia, conexão do WhatsApp) | sim, como hoje | sim | não |
| `/mcp` | sim | não | sim |
| `/media/…` (links do `download_media`) | sim | sim | sim, só os da própria conexão |
| `/health` | sim | sim | sim |
| `/healthz` | sim | aberto | aberto |
| Tela de senha, rotas do OAuth, `/.well-known/…` | não existem | abertas (são a porta de entrada) | abertas |

Um teste percorre todas as rotas registradas e exige que cada uma esteja
nesta tabela: uma rota nova sem classificação quebra o teste, em vez de
nascer aberta na internet.

### O que o painel assume do computador da pessoa

Menos do que parecia. O painel já roda sem o app: a linha de comando o serve
com `Host == nil`, e as telas do app (abrir no login, fechar para a bandeja,
abrir pasta, atualizar, encerrar) já somem nesse caso. Sobra:

| Hoje | No modo servidor |
|---|---|
| *Conectar ferramenta de IA* escreve a configuração do Claude Code, Claude Desktop, Codex e Cursor **naquele** computador (`internal/panel/clients.go`) | Mostra o endereço público e o trecho para colar, e o caminho de login ou chave conforme o agente |
| Endereços `http://127.0.0.1:47821/…` nos textos, nos exemplos de webhook e nos links de mídia | O endereço público, quando o pedido vem da escuta pública |
| "Trocar a porta" e "apagar tudo" em Configurações | A porta some (é do compose); apagar tudo pede a senha de novo |
| Detecção dos clientes instalados e a lista de conexões pelo nome do programa | *Conexões* lista logins e chaves |
| Baixar os logs de suporte | Igual, atrás da sessão |

### O Pro se encaixa aqui

Para não refazer os ganchos duas vezes:

- as regras de conversas e ferramentas são campos do chamador que o código
  público consulta nos pontos 2 e 3, e que só o Pro preenche;
- as rotas `/mcp/<slug>` dos números extras recebem o chamador da mesma
  forma que o `/mcp`;
- a tela de autorização e a de criar chave têm um espaço que o Pro preenche
  com as suas opções.

## 2b. Sem quebrar nada na 1.9

Regra da 1.9.0: quem atualiza da 1.8 não muda nada e não percebe nada. O modo
servidor só existe para quem define `WHATSAPP_MCP_PUBLIC_URL`. Fica igual:

- o endereço e a porta locais (`http://127.0.0.1:47821`, `/mcp`, `/api`,
  `/health`), e a regra de só responder a `localhost`;
- o token opcional (`WHATSAPP_MCP_TOKEN`, `whatsapp-mcp token`) e o que ele
  protege;
- o nome da conexão (`whatsapp`), as ferramentas do MCP e o formato das suas
  respostas;
- os webhooks: os eventos, o corpo, os cabeçalhos e a assinatura;
- a pasta de dados e os bancos (`state.db` só ganha tabelas, não muda as que
  existem);
- os comandos da linha de comando, as suas saídas em `--json` e os códigos de
  saída;
- o compose de hoje (`network_mode: host`, só local) continua funcionando;
- os nomes dos arquivos da release, que o atualizador procura.

O que entra é só adição: variáveis, comandos (`password`, `apikey`,
`server setup`, `bridge --url`), a escuta pública e as rotas dela. Os testes
de integração de hoje passam sem alteração, e ganham uma etapa de servidor.

Quem está numa `v2.0.0-beta` é a exceção, por ser versão de teste: o modo
remoto do app deixa de existir (fase 4).

Na 2.0.0, uma quebra só entra se for mesmo necessária e trouxer vantagem
clara, e vai escrita no changelog com o caminho de migração.

## 3. O que sai, e o que fica, da branch `cli-headless`

| Parte | Destino |
|---|---|
| Chaves de API (`internal/daemon/apikeys.go`, páginas *Agentes remotos*) | Fica |
| Busca de mídia só na internet (`internal/mcp/remotefetch.go`) | Fica |
| Proxy confiável (`internal/remote/trust.go`) | Fica, fora do pacote `remote` |
| `install-docker.sh` | Fica, reescrito para o desenho novo |
| Textos em espanhol e inglês dessas telas | Ficam os que servem |
| Pareamento de aparelhos e canal cifrado (`internal/remote`: `channel`, `mux`, `client`, `server`) | Sai |
| Modo remoto do app (`cmd/app`, telas do app) | Sai |
| Mudança do número entre computador e servidor (`internal/transfer`) | Sai; fica o passo a passo à mão que já está em `servidor.md` |
| `docs/acesso-remoto.md`, `modo-servidor.md`, `arquitetura-servidor.md`, a skill | Reescritos |

A branch fica guardada com uma tag (`arquivo/modo-remoto`), e o trabalho novo
parte do `main`, trazendo só os arquivos da primeira metade da tabela.

## 4. Como o `https` chega

O daemon nunca atende a internet direto nem cuida de certificado. O
`compose.yaml` oferece estes perfis, e o instalador pergunta qual:

| Perfil | Precisa de | Observação |
|---|---|---|
| `tailscale` (**recomendado**) | Uma conta grátis no Tailscale | O Tailscale Funnel dá um endereço fixo `https://<servidor>.<rede>.ts.net`, com certificado, sem domínio próprio e sem abrir porta. Serve para servidor em casa |
| `tunnel` | Conta na Cloudflare e um domínio nela | Um `cloudflared` ao lado, com o token do túnel. Para quem quer o próprio domínio sem abrir porta |
| `caddy` | Um domínio apontado para o servidor, portas 80 e 443 | O Caddy tira o certificado sozinho |
| nenhum | Um proxy que já existe (Traefik, nginx) | O instalador imprime os rótulos do Traefik ou o trecho do nginx |

Sobre o Funnel, pela documentação do Tailscale em 2026-11:

- existe em todos os planos, e ainda é marcado como beta;
- só publica nas portas 443, 8443 e 10000;
- tem um limite de banda que o Tailscale não divulga nem deixa mudar, o que
  pode pesar no envio e no download de mídia;
- precisa ser ligado em segundo plano (`--bg`) para voltar depois de um
  reinício.

A conferir na fase 2, antes de o guia recomendar: se o conector do Claude e o
do ChatGPT aceitam um endereço `ts.net`, e como o Funnel sobe num contêiner
ao lado do nosso.

O túnel rápido da Cloudflare (`trycloudflare.com`, sem conta) troca de
endereço a cada vez que sobe, então não serve para um conector fixo. O guia o
mostra só como teste, e sempre apontado para a escuta pública.

## 5. Imagem Docker publicada

Hoje a imagem é montada do repositório. Para instalar com um comando, ela
precisa estar publicada:

- `ghcr.io/brorlandi/whatsapp-mcp-local`, para `amd64` e `arm64`, com as
  tags `X.Y.Z` e `latest` (decidido em 2026-11-17).
- O nome `ghcr.io/brorlandi/whatsapp-mcp` é o da imagem da versão servidor
  antiga, e os servidores antigos puxam as tags `edge` e `latest` dela. Com
  um nome próprio, nenhum deles recebe este app por engano.
- A imagem passa para `ghcr.io/brorlandi/whatsapp-mcp` junto com a troca de
  nome do repositório. Até lá, o `compose.yml` é o único lugar que cita o
  nome (o `install-docker.sh` e o `scripts/release.sh` o leem de lá), para a
  troca ser em um ponto só.
- Montada pelo `scripts/release.sh`, no Mac de quem publica, como o resto da
  release, para `amd64` e `arm64`, no Docker local; publicada num passo à
  parte, `scripts/push-image.sh`, que pede confirmação.
- `whatsapp-mcp update` no Docker passa a dizer o comando certo
  (`docker compose pull && docker compose up -d`).

## 6. Grátis ou paga

Decidido em 2026-11-17 (em 2026-11-03 o modo servidor estava no pacote pago):

- **Grátis, no repositório público:** o modo servidor com um número, senha,
  login e chaves de API. Ele substitui uma versão que já é grátis e tem gente
  usando, e o guia de migração dela já manda as pessoas para cá.
- **Pago (Pro):** vários números, conversas permitidas e bloqueadas,
  ferramentas permitidas, e o Orquestrador (fase 6d).

Decidido no mesmo dia, sobre o [plano de segurança](plano-seguranca.md):

- **Grátis:** "só leitura" por conexão. Um login ou uma chave marcado assim
  vê e busca, mas não envia, não apaga e não mexe em grupos. Com o MCP na
  internet, é proteção básica.
- **Pro:** quais conversas cada conexão alcança e quais ferramentas ela pode
  usar.

## 7. Fases

Cada fase termina com algo que roda e pode ser testado. As fases 1 a 4 são a
1.9.0.

### Fase 0: fechar a 1.8.0 e limpar o terreno

1. A 1.8.0 leva o evento `online` dos webhooks, o registro para suporte e o
   wacli 0.26.1. O que só existe na `beta/1.8.0-beta.2` fica de fora, a menos
   que o retorno de quem testa as betas peça o contrário.
2. `scripts/integration.sh` verde no Mac de testes, e publicar a 1.8.0.
3. Commitar o que está parado no `main` sem commit e não é do domínio: os
   links para `whatsapp-mcp-server` e a pendência do `send_contact` no
   roadmap.
4. Guardar numa branch as mudanças do domínio novo (site, READMEs, docs e os
   três vídeos) e tirá-las da pasta de trabalho. Elas voltam na fase 6.
5. Conferir o guia de migração no repositório da versão servidor: a tabela
   diz que ela funciona no claude.ai e no celular, e o painel dela diz que
   não.
6. Apagar as branches e worktrees sem uso (seção 9).

### Fase 0b: o chamador, sem mudar nada

**Feita** (2026-11-17).

Preparação, no `main`, antes de existir escuta pública. Pode sair numa
versão de correção, porque não muda comportamento.

1. O pacote `internal/caller` e o chamador no contexto de cada pedido. Todo
   pedido de hoje nasce `local`.
2. Os três embrulhos do painel, o `/mcp` e a lista e a chamada de
   ferramentas passam a consultar o chamador. Para `local`, a resposta é
   sempre "pode", então nada muda.
3. As marcas soltas (`WithClientHint`, a janela do app) viram campos dele.
4. O teste que exige toda rota classificada (seção 2a).
5. "Só leitura" como filtro por categoria, ainda sem tela: só o mecanismo e
   os testes.

Pronto quando: os testes de unidade e a suíte de integração passam sem
nenhuma alteração neles, e a saída de `tools/list` e das rotas da API é
idêntica à de antes.

### Fase 1: escuta pública e painel com senha

**Feita** (2026-11-17). O `SameSite` ficou `Lax`; a espera depois de erros
ganhou o desenho da fase 2b.

1. `WHATSAPP_MCP_PUBLIC_URL`, `_PUBLIC_LISTEN`, `_PANEL_PASSWORD_HASH` e
   `_TRUSTED_PROXY`; o serviço recusa subir com a configuração pela metade.
2. A escuta pública, com o filtro de proxy confiável e de `https` da branch.
3. Login do painel: tela de senha, sessão, CSRF, espera depois de erros.
4. `whatsapp-mcp password hash` e `password set`.
5. O painel no modo servidor: esconder o que é só do computador da pessoa, e
   o QR code de conexão atrás do login.
6. Testes: nenhuma rota da escuta pública responde sem sessão; a escuta local
   continua igual, byte a byte, para quem não liga o modo servidor.

Pronto quando: num contêiner atrás de um proxy, dá para abrir o painel pelo
navegador, entrar com a senha e conectar o WhatsApp.

### Fase 2: MCP por URL

**Feita** (2026-11-17), menos os itens 6 e 7, os testes com o Tailscale
Funnel e os conectores de verdade, que ficam para a fase 4. A página
*Agentes remotos* virou *Conexões*. Depois veio a **fase 2b**, também feita:
as correções de uma revisão de segurança (a página de autorização, a espera
depois de erros, limites para o que um estranho faz o servidor guardar, a
validação dos argumentos que chegam ao wacli, o limite das consultas ao
WhatsApp por conexão).

1. Trazer as chaves de API da branch para a escuta pública, com as páginas
   *Agentes remotos* e os comandos `apikey`.
2. O servidor OAuth: descoberta, registro, autorização com a senha do painel,
   tokens, renovação e revogação.
3. *Conexões* no painel, com logins e chaves juntos.
4. `send_media_message` só com endereço da internet para quem vem de fora, e
   os links do `download_media` com o endereço público.
5. `whatsapp-mcp bridge --url`.
6. Conferir o Tailscale Funnel: num contêiner ao lado, e aceito pelos
   conectores do Claude e do ChatGPT.
7. Testes com clientes de verdade: Claude no navegador e no celular, ChatGPT
   na web, Claude Code, Hermes e Grok Bot.

Pronto quando: o Claude do celular lê e envia mensagens pelo servidor, e uma
chave revogada para de funcionar na hora.

### Fase 3: instalar e atualizar

**Feita** (2026-11-17), menos: a publicação da imagem, que é um passo à mão
na release (`scripts/push-image.sh`); o guia "vindo da versão servidor"; a
skill `whatsapp-mcp-servidor`. O `compose.yml` tem os perfis `tailscale`,
`tunnel` e `caddy`, e a opção de um proxy existente. As partes não testadas
de verdade (Funnel, túnel, conectores) estão marcadas "a confirmar" no
`servidor.md`. A etapa de servidor da suíte de integração (fase 4, item 2)
também já existe: `go test -tags integration_server ./integration/`.

1. Imagem publicada no `ghcr.io`, montada pelo `scripts/release.sh`.
2. `compose.yaml` com os perfis `tailscale`, `tunnel` e `caddy`, e o
   `install-docker.sh` novo.
3. `whatsapp-mcp server setup` para quem instala sem Docker.
4. Docs: `servidor.md` e `modo-servidor.md` reescritos, um passo a passo do
   Tailscale Funnel e outro do túnel da Cloudflare, e o guia "vindo da versão servidor".
5. A skill `whatsapp-mcp-servidor` reescrita, para um agente conduzir a
   instalação.

Pronto quando: numa máquina limpa, do comando de instalação ao Claude do
celular respondendo, sem editar arquivo à mão.

### Fase 4: revisar, migrar e lançar a 1.9.0

A 1.9.0 sai sem divulgação: release, changelog e página de download, e só. A
divulgação do modo servidor fica para a 2.0.

1. Revisão de segurança do que ficou na internet: login, OAuth, chaves,
   sessão, proxy confiável, mídia.
2. Uma etapa de servidor em `integration/`: o contêiner de pé, login, uma
   chave, o fluxo OAuth e uma chamada de ferramenta pela escuta pública.
3. Migrar o servidor de casa do Bruno, hoje numa `2.0.0-beta` em modo remoto,
   para a imagem nova; o app do Mac volta ao modo local ou sai de uso.
4. Um testador de fora instala num Linux dele, só com a documentação, antes
   de a 1.9.0 ser publicada: o que travar nele vira correção ou texto novo
   no guia.
5. Avisar quem instalou uma `v2.0.0-beta`: o atualizador não oferece a 1.9.0
   para quem está numa `2.0.0-beta` (o número é maior), então a troca é à
   mão.
6. Site: a página `/servidor` passa a ser a do modo servidor desta versão,
   com o aviso de migração para quem usa a antiga.
7. No repositório da versão servidor: o guia de migração deixa de dizer que
   claude.ai e celular "ainda não" existem aqui.
8. Publicar a 1.9.0.

### Fase 5: segurança das ferramentas

O [plano de segurança](plano-seguranca.md), agora mais urgente porque o MCP
pode estar na internet:

1. Só leitura por conexão (um login ou uma chave que só lê).
2. Conversas permitidas e bloqueadas, sobre os ganchos da branch
   `BrOrlandi/ignore-list-local`.
3. Ferramentas permitidas por categoria.
4. Pastas de onde um arquivo pode ser enviado.

O item 1 é público e pode sair antes; os itens 2 a 4 são do Pro e saem com a
2.0 (seção 6).

### Fase 6: a 2.0, versão comercial

1. **Vários números** ([plano](plano-multiplas-contas.md)): refazer os ganchos
   da `BrOrlandi/pro-hooks` sobre o `main` depois da 1.9.0. A branch nasceu em
   cima da `cli-headless`, então carrega o modo remoto que sai. No servidor,
   cada número ganha o seu `/mcp/<slug>`, com os mesmos login e chaves.
2. **Licença por e-mail e conta no site** (fase 6b). Falta o deploy do
   Worker, a chave de produção e o envio do código.
3. **Pagamento:** Pix Automático primeiro, cartão depois.
4. **Site:** as páginas de preços, assinatura, termos e privacidade
   (`BrOrlandi/pro-site-pages`), e a troca para o domínio novo, com o
   endereço antigo redirecionando e os três cortes do vídeo já renderizados.
5. **Repositório:** `whatsapp-mcp-local` vira `whatsapp-mcp` na data que o
   Bruno marcar (de 1 a 2 meses depois da última versão do servidor antigo,
   de 2026-11-17). A imagem Docker ganha o nome definitivo nessa hora.
6. Preço, textos jurídicos e divulgação.

### Fase 6a: lista de e-mails, antes da 2.0

Pode sair a qualquer momento, sem depender das outras fases. Serve para
avisar do lançamento do Pro e, depois, para a newsletter.

1. Um campo de e-mail no site ("Avise-me das novidades"), em pt, es e en, na
   home e na página de download.
2. Confirmação em duas etapas: a pessoa recebe um e-mail e só entra na lista
   depois de clicar. Evita endereço errado e inscrição feita por terceiros.
3. Onde fica: o Worker de assinaturas recebe o formulário e manda a
   confirmação. O envio é pelo useSend, o serviço de e-mail de código aberto
   que o Bruno já hospeda (por cima do Amazon SES); os códigos de entrar da
   fase 6b saem por ele também. O site continua estático.
4. A página `/privacidade` precisa existir antes do formulário: o que é
   guardado (o e-mail, o idioma e a data), para quê, e como sair. Todo e-mail
   leva o link de sair da lista (LGPD).
5. O formulário marca de onde veio a inscrição (home, download, página do
   Pro), para saber quem pediu aviso do Pro.

Decidido em 2026-11-17:

- **Remetente:** `contato@` no domínio novo, com um redirecionamento de
  e-mail na Cloudflare para as respostas chegarem à caixa do Bruno. O e-mail
  do domínio pode funcionar antes de o site mudar para ele: são registros de
  DNS separados (os do SES para enviar, os do redirecionamento para
  receber).
- **Uma lista só.** Quem pediu aviso do Pro e quem quer a newsletter estão na
  mesma lista; a origem da inscrição (item 5) separa os grupos quando for
  preciso.

A conferir antes de montar:

- se o useSend desta versão tem lista de contatos e envio em massa, com o
  link de sair, ou se a lista fica no banco do Worker e o useSend só envia;
- que o serviço tem monitoramento: ele já travou uma vez, e os códigos de
  entrar passam a depender dele.

### Fase 6b: conta no site

O plano dos vários números já tem "entrar por e-mail com código" e uma página
`/assinatura` para cancelar sem o app. A conta no site é isso crescido:

1. **Entrar:** e-mail e código de 6 dígitos, sem senha. É a mesma identidade
   do app e do painel do servidor: um e-mail, uma conta.
2. **A página da conta** (`/conta`), servida pelo Worker de assinaturas:
   - o plano, o valor, a próxima cobrança e o histórico de pagamentos;
   - quantos números extras, e mudar a quantidade;
   - trocar a forma de pagamento e cancelar;
   - onde a licença está em uso (cada app ou servidor que entrou com o
     e-mail), com "sair deste aparelho";
   - a inscrição na newsletter.
3. **Assinar pelo site:** `/precos` leva ao checkout, e a compra cria a conta
   com o e-mail usado. No app, "Já assinei" continua pedindo só o e-mail.
4. **Licença liberada pelo Bruno:** o painel de administração libera um
   e-mail sem pagamento (cortesia, teste, suporte). A pessoa entra com esse
   e-mail e a licença já está lá.
5. **Apagar a conta:** a pessoa pede pela própria página, e o Worker apaga o
   e-mail e o vínculo com a assinatura (LGPD).

A tela de assinatura do app passa a mostrar o resumo e um botão que abre a
página da conta, em vez de repetir tudo lá.

### Fase 6c: a licença conta números, não aparelhos

A licença não pode ser comprada por um e dividida entre vários. O que ela
limita é a quantidade de **números de WhatsApp diferentes** na conta, onde
quer que rodem:

- Uma conta com 2 números pode tê-los no mesmo servidor, em dois servidores,
  ou um no computador e outro num servidor. Em todos os casos, são 2.
- O mesmo número em dois lugares conta uma vez (e o WhatsApp já não deixa a
  mesma sessão rodar em dois lugares ao mesmo tempo).
- Um terceiro número, em qualquer instalação que entrou com aquele e-mail, é
  recusado: "esta conta já usa os 2 números do plano", com a lista de quais
  são e o botão para liberar um.

Como funciona:

1. Ao entrar com o e-mail, e a cada validação diária, a instalação diz ao
   Worker quais números ela tem. O Worker guarda uma **vaga** por número na
   conta.
2. O token assinado que o Worker devolve vale para aqueles números, não para
   a instalação. Um número fora do token roda sem os recursos pagos.
3. Liberar uma vaga é pela página da conta ou pelo app. Para ninguém rodar
   números em rodízio, uma vaga liberada só aceita outro número depois de um
   intervalo (por exemplo, 3 trocas por mês sem espera, depois 7 dias).
4. Os 14 dias de tolerância sem internet continuam, por instalação.
5. O Worker recebe o número em forma de código, não o número em si. Um
   telefone tem poucos dígitos e o código não é segredo absoluto para quem
   opera o Worker; a página de privacidade diz isso com clareza.

O que precisa ser acertado com o plano dos vários números, que hoje fala em
"números extras" e deixa o principal de fora de qualquer checagem:

- Quem usa um número só, sem recurso pago, continua sem conta e sem checagem
  nenhuma.
- Com o Pro, o controle de conversas e de ferramentas vale também para quem
  tem um número só. Então esse número precisa de uma vaga.
- **A decidir:** o plano passa a ser vendido por total de números (1, 2,
  3…), ou continua "o principal mais N extras", com o principal ocupando uma
  vaga que vem no plano?

### Fase 6d: o Orquestrador, lançado junto com o Pro

Decidido em 2026-11-17 (em 2026-11-03 ele vinha depois, como um segundo
plano): o Orquestrador sai com a 2.0.

**O que é.** Um serviço na internet, próprio, feito para este app: ele recebe
os webhooks do WhatsApp MCP e os redireciona. A pessoa monta os fluxos numa
tela: as mensagens desta conversa vão para este destino, as daquele grupo
para aquele outro. Assim conversas diferentes caem em agentes de IA
diferentes, por exemplo um bot do Grok Bot para os clientes e um agente do
Hermes para a família, e cada agente responde pelo MCP.

**O que já existe.** Uma primeira versão roda na casa do Bruno, para um
usuário só: regras num arquivo, entrega a agentes do Hermes, as mensagens de
uma conversa juntadas por alguns segundos para acordar o agente uma vez por
rajada, novas tentativas com espera crescente e descarte de repetições.

**O que falta para virar produto.**

1. Vários usuários: cada conta com os seus fluxos, destinos e segredos, com
   o mesmo login por e-mail da conta do site (fase 6b).
2. A tela dos fluxos: escolher conversas (contato, grupo, "todas as outras"),
   o destino e o que fazer com a rajada.
3. Destinos prontos: Hermes, Grok Bot (a rotina que começa por webhook) e um
   webhook genérico assinado, para n8n e scripts.
4. Ligação com o app em um passo: o app cria sozinho o webhook que aponta
   para o Orquestrador, com o segredo, quando a pessoa entra com a conta.
5. Privacidade: o desenho em que o Orquestrador roteia sem ler o texto das
   mensagens (o app cifra o texto para o destino e deixa em claro só a
   conversa, o tipo e a hora). Onde o destino é de terceiros, a tela avisa
   que ele lê as mensagens.
6. Registro das entregas (entregue, falhou, tentando), sem guardar o texto.
7. Limites por plano: fluxos, destinos, mensagens por mês.

**O que isso pede do app, e pode entrar antes da 2.0:**

- carimbo de tempo e id na assinatura dos webhooks, contra o reenvio de uma
  entrega capturada, de preferência no formato Standard Webhooks. Entra como
  cabeçalhos novos ao lado dos atuais, para não quebrar quem já recebe;
- o `account` no corpo, que já existe, para o Orquestrador saber de qual
  número veio.

**A decidir:**

- O Orquestrador entra no preço do Pro ou é um plano acima.
- Onde ele roda (um Worker com fila e banco, ou um serviço próprio).
- Se a primeira versão pública já sai sem ler as mensagens, ou se isso vem
  numa segunda etapa.
- Se quem roda o próprio servidor pode hospedar o Orquestrador também, ou se
  ele é só o serviço do Bruno.

### Depois da 2.0

- **Acesso de fora ligado no próprio app.** Para quem não tem servidor e quer
  o Claude do celular alcançando o WhatsApp que roda no computador: uma tela
  em Configurações liga a escuta pública, define a senha e mostra o
  endereço. Não é o modo remoto das `v2.0.0-beta` (o app como controle de um
  servidor separado); aqui o WhatsApp continua no computador, e é o app que
  passa a atender de fora. Usa a mesma base da 1.9; só falta a tela.

- `send_contact`, quando o `wacli send contact` sair numa release do wacli.
- Onde a transcrição roda no servidor ([roadmap](roadmap.md)).
- Homebrew e biblioteca de figurinhas, que já têm plano.

## 8. Riscos

- **OAuth feito em casa.** É a parte nova mais sensível. Fica pequeno de
  propósito (um usuário, uma senha, sem escopos) e passa pela revisão da
  fase 4 antes de sair. Se o Claude ou o ChatGPT mudarem o que exigem do
  conector, é aqui que quebra; os testes da fase 2 com os clientes de verdade
  precisam virar um roteiro repetível.
- **Senha fraca.** A senha do painel passa a proteger o WhatsApp inteiro na
  internet. O `password hash` recusa senha curta, e as tentativas erradas
  esperam cada vez mais.
- **Instrução escondida numa mensagem.** Com o agente do celular lendo e
  enviando, o risco do plano de segurança cresce. Por isso a fase 5 vem logo
  depois.
- **Quem está na `2.0.0-beta`.** São poucos, mas ficam fora do atualizador
  até a 2.0.0 final.

## 9. Branches e worktrees: o que fazer com cada uma

| Branch | Situação | Destino |
|---|---|---|
| `BrOrlandi/cli-headless` (48 commits) | O modo remoto das `v2.0.0-beta` | Tag de arquivo; aproveitar os arquivos da seção 3 |
| `BrOrlandi/cli-headless-on-1.6.2` | A mesma, rebaseada numa versão velha | Apagar |
| `BrOrlandi/headless-client-wip`, `BrOrlandi/agents-listener-wip` | Ideias postas de lado: cliente sem tela e escuta com token para rede privada | Apagar; viraram o `bridge --url` e a escuta pública |
| `BrOrlandi/pro-hooks` (55 commits, só local) | Ganchos do Pro, em cima da `cli-headless` | Refazer sobre o `main` na fase 6 |
| `BrOrlandi/ignore-list-local` | Ganchos para esconder conversas dos agentes | Base da fase 5 |
| `BrOrlandi/pro-site-pages` | Páginas de preços, assinatura, termos e privacidade | Fase 6 |
| `beta/1.8.0-beta.2` | Build de teste do reenvio de mensagens | Decidir na fase 0 |
| `BrOrlandi/i18n-on-main`, `backup/main-antes-sync`, `pr2`, `pr3`, `review/*`, `pr-5-idiomas` | Revisões e cópias antigas | Conferir se tudo já está no `main` e apagar |
| `BrOrlandi/release-1.5.0`, `release-1.6.0`, `review-mark-unread`, `site-browser-language`, `windows-claude` | Sem nada à frente do `main` | Apagar com as worktrees |

## 10. A decidir

1. A data do rename do repositório.
2. O plano pago é vendido por total de números ou por números extras
   (fase 6c).
3. O intervalo para reaproveitar uma vaga liberada.
4. O Orquestrador: preço, onde roda, e se já nasce sem ler as mensagens
   (fase 6d).

Decidido para a 1.9 em 2026-11-17: o modo servidor só existe na linha de
comando (Linux e Docker), não no app do computador; a senha por variável ou por comando; sem
senha, não sobe; login dos agentes sem prazo; a transcrição no servidor fica
como hoje (o Whisper instalado no servidor, se a pessoa quiser).
