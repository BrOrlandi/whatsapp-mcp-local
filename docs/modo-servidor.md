# Modo servidor: o WhatsApp MCP ligado o tempo todo

Um passo a passo para quem nunca montou um servidor: contratar a máquina,
instalar o WhatsApp MCP nela (com Docker, o recomendado, ou sem) e ligar o app
do seu computador a ela. Os detalhes de cada comando estão em
[Num servidor sem tela](servidor.md), e os de segurança, em
[Acesso remoto](acesso-remoto.md).

> **Uso avançado.** O modo servidor pede conhecimento técnico para montar e
> manter o servidor: atualizações, backup, HTTPS e a segurança da máquina
> ficam com você. O app pede essa confirmação antes de seguir.

## Precisa mesmo de um servidor?

- **Sim**, se você quer conectar **agentes autônomos**, que trabalham sozinhos
  a qualquer hora, como o OpenClaw e o Hermes Agent, entre outros, mesmo com
  o seu computador desligado.
- **Não**, se você usa o WhatsApp com ferramentas do seu computador, como o
  Claude Code, o Cursor e o Codex: a versão local atende perfeitamente, sem
  servidor nenhum para cuidar.

## Por que existe

Normalmente, o WhatsApp MCP roda no seu computador. Quando ele desliga, ou o
notebook fecha, tudo para: as mensagens deixam de chegar ao WhatsApp MCP, os
webhooks não disparam e nenhum agente de IA alcança o seu WhatsApp.

No modo servidor, o WhatsApp MCP roda numa máquina que fica ligada o tempo
todo. Assim:

- **os seus agentes de IA alcançam o seu WhatsApp pelo MCP mesmo com o seu
  computador desligado**: um agente que roda no próprio servidor (o Claude
  Code, uma tarefa agendada, uma automação) usa o MCP de lá a qualquer hora;
- **os webhooks disparam a qualquer hora**, para cada mensagem nova;
- **o histórico fica completo**: nenhuma mensagem se perde porque o
  computador estava desligado;
- **o app do seu computador vira o controle do servidor**: mostra o painel
  dele, e as ferramentas de IA desse computador continuam no mesmo endereço
  de sempre (`http://127.0.0.1:47821/mcp`), agora lendo do servidor.

O MCP do servidor não fica aberto na internet: ele atende os programas do
próprio servidor e os apps pareados com ele, por uma conexão HTTPS cifrada de
ponta a ponta. O modo servidor é opcional; sem ele, o app continua rodando
tudo no seu computador.

## Do que você precisa

- **Um servidor Linux sempre ligado**, com acesso por SSH. Pode ser um
  servidor virtual (VPS) contratado ou um computador em casa que não desliga.
- **Um endereço (domínio ou subdomínio)** apontando para ele, como
  `whatsapp.seudominio.com`. O app só conecta por HTTPS, com certificado
  válido, e o certificado precisa de um nome.
- **O celular com o seu WhatsApp**, para conectar o servidor.

## 1. Contratar o servidor

Qualquer provedor de VPS serve (Hetzner, DigitalOcean, Contabo, Hostinger,
Magalu Cloud, entre outros). O que pedir:

| | Mínimo | Com transcrição de áudio no servidor |
|---|---|---|
| Processador | 1 vCPU | 2 vCPU |
| Memória | 2 GB | 4 GB |
| Disco | 20 GB | 30 GB |
| Sistema | Ubuntu 24.04 LTS | Ubuntu 24.04 LTS |

A transcrição de áudio no servidor usa só o processador (o modelo ocupa uns
600 MB); sem ela, o plano mínimo sobra.

Depois de contratar:

1. Anote o IP do servidor e entre nele por SSH: `ssh root@<ip>`. Se o
   provedor pedir, cadastre a sua chave SSH em vez de usar senha.
2. Crie um usuário para não trabalhar como root:
   ```sh
   adduser whatsapp && usermod -aG sudo whatsapp
   ```
   e entre de novo com ele: `ssh whatsapp@<ip>`.
3. No painel de DNS do seu domínio, crie um registro **A** com o nome
   escolhido (por exemplo, `whatsapp`) apontando para o IP do servidor.

## 2. Escolher como instalar

| | Com Docker (recomendado) | Sem Docker |
|---|---|---|
| Instalação | um script pergunta tudo e monta o contêiner | um script instala o programa e o serviço do usuário |
| Acesso pelo app (HTTPS) | o script prepara a ligação com o proxy HTTPS | você configura o serviço à mão |
| Atualizar | rodar o script de novo | `whatsapp-mcp update` |
| Backup | a pasta `data/` inteira | `~/.whatsapp-mcp` e a pasta do wacli |
| Isolamento | o WhatsApp MCP roda separado do resto do servidor | roda como um programa do seu usuário |

O Docker é o recomendado: o instalador faz mais por você, e tudo o que importa
fica numa pasta só.

## 3. Instalar com Docker (recomendado)

**Instale o Docker**, se o servidor ainda não tiver:

```sh
curl -fsSL https://get.docker.com | sudo sh
sudo usermod -aG docker $USER
```

Saia do SSH e entre de novo, para o grupo valer.

**Instale o HTTPS.** Se o servidor ainda não tem um proxy HTTPS, o Caddy é o
mais simples: ele pede e renova o certificado sozinho.

```sh
sudo apt install -y caddy
```

Troque `whatsapp.seudominio.com` pelo seu endereço e grave o
`/etc/caddy/Caddyfile`:

```
whatsapp.seudominio.com {
	handle /remote/v1/* {
		reverse_proxy 127.0.0.1:47822
	}
	respond 404
}
```

```sh
sudo systemctl reload caddy
```

Só o caminho do acesso remoto passa pelo Caddy; o resto responde 404. A porta
47822 fica só no `127.0.0.1` do servidor, e o Caddy é quem a alcança.

**Rode o instalador do WhatsApp MCP:**

```sh
curl -fsSL https://raw.githubusercontent.com/BrOrlandi/whatsapp-mcp-local/main/install-docker.sh -o install-docker.sh
bash install-docker.sh
```

Ele pergunta:

- **Pasta da instalação:** aceite `/opt/whatsapp-mcp`.
- **Endereço HTTPS do servidor:** o seu, sem `https://`
  (`whatsapp.seudominio.com`).
- **Quem faz o HTTPS:** `other` para o Caddy. Se o servidor já tem um
  Traefik no Docker, responda `traefik`, e o script liga o contêiner a ele.
- **Porta para o seu proxy:** aceite `47822`, a mesma do Caddyfile.
- **Idioma e fuso horário:** aceite ou ajuste.

No fim, o contêiner `whatsapp-mcp` está rodando, e o script mostra os próximos
passos.

**Conecte o WhatsApp** pelo app, no passo 4, ou já aqui, pelo terminal:

```sh
docker exec -it whatsapp-mcp whatsapp-mcp pair
```

O QR code aparece no terminal. No celular: *WhatsApp › Configurações ›
Dispositivos conectados › Conectar um dispositivo*. Prefere digitar um código
no celular? `docker exec -it whatsapp-mcp whatsapp-mcp pair --phone '+55 11 91234-5678'`.

Já usa o WhatsApp MCP no seu computador e quer levar as conversas e a conexão
para o servidor? Antes de conectar, siga
[Trazer os dados de outro computador](servidor.md#trazer-os-dados-de-outro-computador).

## 3. (alternativa) Instalar sem Docker

```sh
curl -fsSL https://raw.githubusercontent.com/BrOrlandi/whatsapp-mcp-local/main/install.sh | bash
sudo loginctl enable-linger $USER   # o serviço segue rodando depois que o SSH fecha
whatsapp-mcp pair                    # ou conecte pelo app, no passo 4
```

Para o app do seu computador alcançar este servidor, o serviço precisa ouvir o
acesso remoto no `127.0.0.1`, para o Caddy encaminhar:

```sh
WHATSAPP_MCP_REMOTE_LISTEN=127.0.0.1:47822 whatsapp-mcp service install
```

O serviço guarda essa configuração e a mantém nas próximas reinstalações (uma
porta nova, um token novo).

O Caddy é o mesmo do caminho com Docker, acima.

## 4. Ligar o app do seu computador

No servidor, gere um código de pareamento (vale uma vez, por 10 minutos):

```sh
docker exec whatsapp-mcp whatsapp-mcp remote code --name "Meu Mac"
# sem Docker: whatsapp-mcp remote code --name "Meu Mac"
```

No app: *Configurações › Acesso remoto › Usar um servidor* (ou, na primeira
vez que abre o app, *Usar um servidor* abaixo do QR code). Confirme o aviso de
uso avançado, informe `https://whatsapp.seudominio.com` e o código, e conecte.

Se o WhatsApp do servidor ainda não estiver conectado, o app mostra a
instalação do servidor, com o QR code dele: leia com o celular, como na
versão local. A partir daí, o WhatsApp fica conectado no servidor, e o app
mostra o painel dele.

O app passa a mostrar, no topo de cada tela, que está no modo remoto, qual
servidor usa e como estão as duas conexões: a do app com o servidor e a do
servidor com o WhatsApp.

## 5. Ligar os agentes do próprio servidor

Um agente que roda no servidor usa o MCP de lá, sem passar pelo app:

- **Sem Docker:** `whatsapp-mcp config` mostra a configuração de cada
  ferramenta; o MCP fica em `http://127.0.0.1:47821/mcp`.
- **Com Docker:** o MCP fica dentro do contêiner. Configure a ferramenta para
  rodar `docker exec -i whatsapp-mcp whatsapp-mcp bridge` como servidor MCP
  (stdio); para o Claude Code:
  ```sh
  claude mcp add whatsapp -- docker exec -i whatsapp-mcp whatsapp-mcp bridge
  ```

## Se o servidor ficar fora de alcance

O app mostra *Reconectando* e, depois de um minuto, *Indisponível*, na bandeja
e no topo das telas, e tenta de novo sozinho. Enquanto isso, o WhatsApp
continua conectado no servidor e nada se perde. Para usar o WhatsApp sem o
servidor, a tela de conexão do app tem *Voltar ao modo local*.

## Manter

- **Atualizar:** com Docker, rode o `install-docker.sh` de novo; sem Docker,
  `whatsapp-mcp update`.
- **Backup:** com Docker, pare o contêiner e copie a pasta `data/` da
  instalação (`docker compose stop`, `tar`, `docker compose start`, em
  `/opt/whatsapp-mcp`).
- **Ver o que acontece:** `docker exec whatsapp-mcp whatsapp-mcp status` e
  `docker compose logs -f`.
- **Cortar um app:** `whatsapp-mcp remote revoke <id>` (a lista sai em
  `whatsapp-mcp remote`).
