---
name: whatsapp-mcp-servidor
description: Guia a pessoa a colocar o WhatsApp MCP Local no modo servidor, num servidor dela, para que agentes de IA alcancem o WhatsApp por uma URL HTTPS a qualquer hora, enquanto o app do computador continua servindo o MCP no endereço local de sempre, ligado ao servidor por trás. Cobre o contêiner (Dockerfile e compose do repositório), o proxy HTTPS da pessoa, o pareamento do WhatsApp e do app, as chaves de API dos agentes e a verificação final. Use quando a pessoa quiser "rodar o WhatsApp MCP num servidor", "deixar o MCP ligado 24 horas", "conectar o Hermes, o OpenClaw ou outro agente pela URL", "modo servidor" ou "acesso remoto".
---

# Modo servidor do WhatsApp MCP Local

Você vai ajudar a pessoa a montar o modo servidor. Fale com ela na língua dela,
sem jargão, e explique cada passo antes de executá-lo. Este é um uso avançado:
o servidor, o domínio, o HTTPS e a segurança da máquina são responsabilidade
dela, e você só a ajuda a fazer.

## O que é, em uma frase

O WhatsApp passa a rodar num servidor sempre ligado. **Os agentes** (Hermes,
OpenClaw, uma tarefa agendada, um serviço na nuvem) usam o MCP do servidor
**por uma URL HTTPS com chave**. **O computador da pessoa** continua com o MCP
em `http://127.0.0.1:47821/mcp`, como antes: o app vira um controle do
servidor e repassa, por trás, cada pedido ao servidor por uma conexão cifrada.

```
agentes ──https://<endereço>/mcp + chave──┐
                                          ▼
                              proxy HTTPS da pessoa ──► contêiner whatsapp-mcp ──► WhatsApp
                                          ▲
app do computador ──wss /remote/v1 (cifrado)┘
   ▲
Claude Code, Cursor, Codex: http://127.0.0.1:47821/mcp (o mesmo de sempre)
```

Antes de começar, confirme que ela precisa disso: se os agentes rodam só no
computador dela (Claude Code, Cursor, Codex), a versão local basta e não há
servidor para manter.

## É uma configuração avançada: confirme antes de seguir

Diga à pessoa, com clareza, antes de qualquer comando:

- o modo servidor coloca o WhatsApp dela numa máquina que fica ligada e
  alcançável pela rede; quem invadir o servidor lê e manda mensagens como ela;
- a segurança do servidor é dela: atualizações do sistema, firewall, acesso SSH
  só por chave, backup, o certificado HTTPS e quem alcança a URL;
- o projeto entrega o contêiner e as regras de acesso, não a infraestrutura.

Pergunte se ela tem conhecimento técnico para manter um servidor Linux seguro
(SSH, Docker, proxy HTTPS, DNS, firewall). Siga só se ela confirmar. Se não
tiver, recomende ficar na versão local, ou pedir ajuda a alguém que saiba,
e pare aqui. Não tente compensar a falta de conhecimento fazendo tudo por ela
num servidor que ela não saberá manter.

## Regras que você não quebra

- **Só HTTPS com certificado válido** para qualquer acesso de fora do
  servidor. Não monte HTTP exposto, não sugira ignorar erro de certificado, não
  publique a porta 47822 sem o proxy HTTPS na frente.
- **Nunca mostre segredos** (chave de API, token, código de pareamento já
  usado) em logs, em respostas ou em comandos que fiquem no histórico. Grave
  em arquivo com permissão 600 e leia de lá.
- **Só `/remote/v1/` e `/mcp` passam pelo proxy.** O painel e a API do
  servidor ficam no loopback dele.
- **Não exponha o socket do Docker** a nenhum agente.
- **Não apague dados** (`data/`, a pasta do wacli, `remote-server.key`) sem a
  pessoa pedir; faça backup antes de mexer.
- **Não mande mensagem de teste para terceiros** pelo WhatsApp dela.

## 1. Entender o cenário

Pergunte, uma coisa de cada vez, só o que ainda não souber:

1. **Onde fica o servidor** e como você o alcança (SSH, terminal local). Ele
   precisa de Linux e Docker; sem Docker, veja `docs/servidor.md`.
2. **Qual será a URL** (por exemplo `whatsapp.seudominio.com`). Ela precisa de
   um nome com certificado válido.
3. **Como os agentes chegam a ela:** pela internet, ou só por uma rede
   privada (VPN, rede local). A escolha é dela; explique que expor na internet
   deixa o `/mcp` protegido só pela chave de API.
4. **Que proxy HTTPS já existe** no servidor (Caddy, Traefik, nginx, outro) ou
   se é preciso instalar um. O Caddy é o mais simples: pede e renova o
   certificado sozinho.
5. **Se o WhatsApp já roda no app do computador** dela. Nesse caso, **não
   conecte o WhatsApp no servidor**: ao parear (passo 5), o app leva a sessão
   e as conversas para o servidor, com *Levar as conversas deste computador
   para o servidor* marcado, e apaga a cópia do computador depois que o
   servidor conecta. O servidor precisa estar sem WhatsApp, rodar no Docker
   ou como serviço, e usar a mesma versão do wacli que o app.

## 2. Subir o contêiner

Use o `Dockerfile` e o `compose.yaml` de `build/docker/` do repositório
`BrOrlandi/whatsapp-mcp-local`, na última versão publicada (a tag da release
mais recente). Copie o compose para a pasta da instalação (por exemplo
`/opt/whatsapp-mcp`) e ajuste:

- `context` apontando para o clone do repositório;
- a pasta `data/` criada antes do primeiro `up` e dada ao usuário 1000
  (`mkdir -p data && sudo chown 1000:1000 data`);
- `TZ` e `WHATSAPP_MCP_LANG`;
- o acesso remoto, que vem desligado:
  - `WHATSAPP_MCP_REMOTE_LISTEN`: onde o proxy alcança o contêiner;
  - `WHATSAPP_MCP_REMOTE_TRUSTED_PROXY`: quem é o proxy (IP, faixa ou nome do
    contêiner). Com ele, qualquer outro que alcance a porta é recusado.

Os dois casos comuns:

| Proxy | Rede do contêiner | `REMOTE_LISTEN` | `REMOTE_TRUSTED_PROXY` |
|---|---|---|---|
| no próprio servidor (Caddy, nginx) | `network_mode: host`, como no compose | `127.0.0.1:47822` | `127.0.0.1` |
| em outro contêiner (Traefik) | a rede Docker do proxy, sem `network_mode: host` | `0.0.0.0:47822` | o nome do contêiner do proxy |

No segundo caso, não publique a porta 47822 no host: só o proxy, pela rede do
Docker, a alcança.

Suba e confira:

```sh
docker compose up -d --build
docker compose exec whatsapp-mcp whatsapp-mcp status
docker compose logs --tail 50   # procure "serving remote access"
```

## 3. Configurar o proxy HTTPS

O que o proxy da pessoa precisa fazer, seja qual for:

- terminar o HTTPS na URL escolhida, com certificado válido;
- encaminhar **só** `/remote/v1/*` e `/mcp` para a escuta remota; o resto
  responde 404;
- aceitar WebSocket em `/remote/v1/session` e `/remote/v1/pair`;
- mandar `X-Forwarded-Proto: https` (Caddy e Traefik mandam sozinhos; no
  nginx, `proxy_set_header X-Forwarded-Proto $scheme;` e os cabeçalhos
  `Upgrade` e `Connection`);
- não redirecionar nada para `http://`.

Caddy, com o contêiner em `127.0.0.1:47822`:

```
whatsapp.seudominio.com {
	@mcp path /remote/v1/* /mcp
	handle @mcp {
		reverse_proxy 127.0.0.1:47822
	}
	respond 404
}
```

Traefik, como labels no serviço do compose:

```yaml
labels:
  - traefik.enable=true
  - traefik.docker.network=<rede do traefik>
  - traefik.http.routers.whatsapp-mcp.rule=Host(`whatsapp.seudominio.com`) && (PathPrefix(`/remote/v1/`) || Path(`/mcp`))
  - traefik.http.routers.whatsapp-mcp.entrypoints=<entrypoint https>
  - traefik.http.routers.whatsapp-mcp.tls.certresolver=<resolver>
  - traefik.http.services.whatsapp-mcp.loadbalancer.server.port=47822
```

Confira de fora do servidor: `curl -fsS https://<endereço>/remote/v1/health`
responde `ok`, e `curl -s -o /dev/null -w '%{http_code}' https://<endereço>/`
responde 404.

## 4. Conectar o WhatsApp

Pule este passo se o WhatsApp já roda no app do computador da pessoa: o app
leva a sessão no passo 5. Senão, conecte pelo app (passo 5 mostra o QR code
do servidor) ou pelo terminal:

```sh
docker compose exec whatsapp-mcp whatsapp-mcp pair
```

O QR aparece no terminal; no celular, *WhatsApp › Configurações ›
Dispositivos conectados › Conectar um dispositivo*.

## 5. Ligar o app do computador da pessoa

No servidor, gere um código de pareamento (vale uma vez, por 10 minutos; gere
só na hora de colar):

```sh
docker compose exec whatsapp-mcp whatsapp-mcp remote code --name "Meu Mac"
```

No app: *Configurações › Acesso remoto › Usar um servidor*, confirmar o aviso,
colar `https://<endereço>` e o código. A partir daí:

- a janela do app mostra o painel do servidor;
- o MCP do computador continua em `http://127.0.0.1:47821/mcp`, e as
  ferramentas já configuradas não mudam nada.

O código só serve para o primeiro pareamento; depois o app conecta sozinho.
Outro computador que também vai gerenciar o servidor recebe um código pelo app
já conectado (*Gerenciar a conexão com o servidor › Parear outro computador
como gerenciador*) ou pelo mesmo `remote code`. Se a pessoa perder ou formatar
o computador, siga `docs/modo-servidor.md#perdi-ou-formatei-o-computador`:
código novo pelo terminal do servidor, e revogar o antigo. Lembre a pessoa de
guardar o acesso SSH do servidor: sem ele e sem outro computador pareado, não
há como parear um novo.

Para outro computador que só deve usar o MCP, sem administrar o servidor, use
`--scope tools`. A lista e a revogação: `whatsapp-mcp remote` e
`whatsapp-mcp remote revoke <id>`.

## 6. Dar uma chave de API a cada agente

O `/mcp` pela URL fica desligado (404) enquanto não houver chave. O servidor
guarda só o SHA-256 de cada chave, uma por linha, no arquivo `api-keys` da
pasta de dados (`data/mcp/api-keys`), com um nome depois de um espaço. Ele lê
o arquivo a cada pedido: não precisa reiniciar, e apagar a linha revoga na
hora.

Se a pessoa tem o app pareado, o caminho mais simples é ela mesma: *Conectar
ferramenta de IA › Agentes remotos*, escolher o agente e seguir a tela, que
cria a chave, mostra o texto para o agente e espera o primeiro uso.

Pelo terminal, gere a chave sem mostrá-la:

```sh
umask 077
docker exec whatsapp-mcp whatsapp-mcp apikey new hermes --json \
  | grep -o '"key":"[^"]*"' | cut -d'"' -f4 > hermes-api-key   # a chave, só para o agente
```

Uma chave feita assim (ou pela tela) que ninguém usar em 24 horas deixa de
valer sozinha; o teste do passo 7 já conta como uso. Para uma chave que nunca
expira, escreva a linha à mão, sem a marca `unused`:

```sh
umask 077
openssl rand -hex 32 > hermes-api-key
printf '%s hermes\n' "$(sha256sum < hermes-api-key | cut -d' ' -f1)" >> data/mcp/api-keys
```

Entregue o arquivo da chave ao agente pelo canal de segredos dele (um `.env`
com permissão 600, um gerenciador de segredos), nunca colado na conversa. A
configuração do agente:

```json
{
  "mcpServers": {
    "whatsapp": {
      "url": "https://<endereço>/mcp",
      "headers": { "Authorization": "Bearer <conteúdo do arquivo da chave>" }
    }
  }
}
```

Uma chave por agente (ou por grupo de agentes) facilita revogar um só.

Um agente que roda **no próprio servidor** não precisa de chave nem de URL:
`docker exec -i whatsapp-mcp whatsapp-mcp bridge` como servidor MCP (stdio).

## 7. Verificar

Sem mostrar a chave na tela:

```sh
# sem chave: 401
curl -s -o /dev/null -w '%{http_code}\n' -X POST https://<endereço>/mcp
# com chave: 200 e a lista de ferramentas
curl -s -X POST https://<endereço>/mcp \
  -H "Authorization: Bearer $(cat hermes-api-key)" \
  -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-03-26","capabilities":{},"clientInfo":{"name":"teste","version":"0"}}}' \
  | head -c 300
```

Depois, peça ao agente para listar as conversas (`list_chats`), só leitura.
No painel, em *Conexões*, ele aparece em "Direto no servidor, pelo endereço
com chave", e o computador da pessoa em "No computador …, pelo app".

## 8. Entregar

Resuma para a pessoa, sem segredos:

- a URL do MCP para agentes e onde está cada arquivo de chave;
- que o MCP do computador dela segue em `http://127.0.0.1:47821/mcp`;
- como atualizar (novo `git pull` na tag e `docker compose up -d --build`),
  fazer backup (parar o contêiner e copiar `data/`) e revogar um app ou uma
  chave;
- que manter o servidor, o certificado e o acesso de rede é com ela.

Referências no repositório: `docs/modo-servidor.md`, `docs/servidor.md`,
`docs/acesso-remoto.md` e `docs/arquitetura-servidor.md`.
