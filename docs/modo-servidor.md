# Modo servidor: o WhatsApp MCP ligado o tempo todo

Um passo a passo para quem nunca montou um servidor: contratar a máquina,
instalar o WhatsApp MCP nela (com Docker, o recomendado, ou sem) e ligar o app
do seu computador a ela. Os detalhes de cada comando estão em
[Num servidor sem tela](servidor.md), e os de segurança, em
[Acesso remoto](acesso-remoto.md), e o mapa das peças, em
[Arquitetura do modo servidor](arquitetura-servidor.md).

> **Uso avançado.** O modo servidor pede conhecimento técnico para montar e
> manter o servidor: atualizações, backup, HTTPS e a segurança da máquina
> ficam com você. O app pede essa confirmação antes de seguir.
>
> Quer ajuda de um agente de IA para montar? A skill
> [`skills/whatsapp-mcp-servidor`](../skills/whatsapp-mcp-servidor/SKILL.md)
> guia o Claude Code, o Codex ou outro agente pelo passo a passo, com as
> regras de segurança. Copie a pasta para as skills do seu agente, ou cole o
> `SKILL.md` na conversa como instrução.

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
próprio servidor, os apps pareados com ele, por uma conexão HTTPS cifrada de
ponta a ponta, e os agentes de fora a quem você der uma chave (passo 6). Sem
nenhuma chave criada, o endereço do MCP responde 404. O modo servidor é
opcional; sem ele, o app continua rodando tudo no seu computador.

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
	@whatsapp path /remote/v1/* /mcp
	handle @whatsapp {
		reverse_proxy 127.0.0.1:47822
	}
	respond 404
}
```

```sh
sudo systemctl reload caddy
```

Só dois caminhos passam pelo Caddy: o do app (`/remote/v1/`) e o dos agentes
de fora (`/mcp`, que só responde a quem tem uma chave). O resto responde 404.
A porta 47822 fica só no `127.0.0.1` do servidor, e o Caddy é quem a alcança.

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
  A regra que ele gera só leva o caminho do app; para os agentes de fora do
  passo 6, troque a regra no `compose.yaml` por
  ``Host(`whatsapp.seudominio.com`) && (PathPrefix(`/remote/v1/`) || Path(`/mcp`))``
  e rode `docker compose up -d`.
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

O código só serve para esse primeiro pareamento. Depois, o app conecta sozinho
toda vez, com a chave que guardou no Keychain do computador.

**Outro computador também vai gerenciar o servidor?** No app que já está
conectado: *Configurações › Acesso remoto › Gerenciar a conexão com o servidor
› Parear outro computador como gerenciador*. O código aparece ali, e o outro
computador o usa como acima. Pelo terminal do servidor também dá:
`remote code`, como no começo deste passo.

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

## 6. Ligar agentes de fora do servidor

Um agente que roda em outro lugar, como o OpenClaw, o Hermes Agent ou um
agente na nuvem, usa o MCP pelo endereço do servidor
(`https://whatsapp.seudominio.com/mcp`), com uma chave de API só dele. Pelo
app:

1. Abra *Conectar ferramenta de IA* e desça até *Agentes remotos*. Escolha o
   agente; se ele não está na lista, *Conectar outro agente remoto*.
2. **Crie uma chave para o agente.** O nome já vem preenchido com o do
   agente. A chave aparece uma vez só: depois que a tela muda, não há como
   vê-la de novo.
3. **Configure o agente.** A tela mostra um texto para mandar no chat do
   agente, para ele mesmo se configurar, ou o endereço e o cabeçalho para pôr
   na configuração de servidores MCP dele. Guarde a chave no lugar de segredos
   do agente e não a cole em outras conversas.
4. **Confira a conexão.** Clique em *Já configurei no agente*: a tela espera
   o primeiro uso do MCP com essa chave e avisa quando ele chega.

Pelo terminal do servidor, a mesma chave sai de
`docker exec whatsapp-mcp whatsapp-mcp apikey new "Hermes"` (sem Docker,
`whatsapp-mcp apikey new "Hermes"`).

Uma chave que nenhum agente usar em 24 horas deixa de valer sozinha, para não
sobrar chave esquecida. Depois do primeiro uso, ela vale até ser revogada.
Dê uma chave a cada agente: assim dá para cortar um só.

As chaves aparecem em *Configurações › Acesso remoto › Gerenciar a conexão com
o servidor › Chaves de agentes remotos*, com o agente de cada uma, quando foi
criada e se já está em uso. *Revogar* corta a chave na hora; o agente que a
usava precisa de uma chave nova para voltar.

## Se o servidor ficar fora de alcance

O app mostra *Reconectando* e, depois de um minuto, *Indisponível*, na bandeja
e no topo das telas, e tenta de novo sozinho. Enquanto isso, o WhatsApp
continua conectado no servidor e nada se perde. Para usar o WhatsApp sem o
servidor, a tela de conexão do app tem *Voltar ao modo local*.

## Perdi ou formatei o computador

O WhatsApp e as conversas continuam no servidor: não é preciso ler o QR code
de novo nem migrar nada. O que se perdeu foi a chave do app daquele
computador, que ficava no Keychain dele. Para gerenciar o servidor de outro
computador:

1. **Entre no terminal do servidor**, por SSH ou pelo console do provedor. Se
   outro computador seu já gerencia o servidor, pode pular para o passo 3 e
   gerar o código nele.
2. **Gere um código de pareamento:**
   ```sh
   docker exec whatsapp-mcp whatsapp-mcp remote code --name "Mac novo"
   # sem Docker: whatsapp-mcp remote code --name "Mac novo"
   ```
3. **No computador novo**, instale o app e vá em *Configurações › Acesso
   remoto › Usar um servidor*, com o endereço `https://` do servidor e o
   código.
4. **Revogue o computador antigo.** Veja a lista com os ids e corte o antigo:
   ```sh
   docker exec whatsapp-mcp whatsapp-mcp remote
   docker exec whatsapp-mcp whatsapp-mcp remote revoke <id>
   ```
   ou, no app novo, *Gerenciar a conexão com o servidor › Revogar* ao lado do
   nome dele. Se o computador foi **perdido ou roubado**, faça isso primeiro:
   a chave continua no Keychain dele até ser revogada. Se foi formatado,
   revogar só limpa a lista.

Sem acesso ao terminal do servidor e sem outro computador pareado, não há como
parear um novo: guarde o acesso SSH do servidor junto com as outras senhas
importantes.

## Manter

- **Atualizar:** com Docker, rode o `install-docker.sh` de novo; sem Docker,
  `whatsapp-mcp update`.
- **Backup:** com Docker, pare o contêiner e copie a pasta `data/` da
  instalação (`docker compose stop`, `tar`, `docker compose start`, em
  `/opt/whatsapp-mcp`).
- **Ver o que acontece:** `docker exec whatsapp-mcp whatsapp-mcp status` e
  `docker compose logs -f`.
- **Cortar um app:** `whatsapp-mcp remote revoke <id>` (a lista sai em
  `whatsapp-mcp remote`), ou *Revogar* na tela *Gerenciar a conexão com o
  servidor* de outro app. Trocar de computador:
  [Perdi ou formatei o computador](#perdi-ou-formatei-o-computador).
- **Cortar um agente de fora:** *Revogar* ao lado da chave dele, em
  *Gerenciar a conexão com o servidor › Chaves de agentes remotos*, ou
  `whatsapp-mcp apikey revoke <id>` (a lista sai em `whatsapp-mcp apikey`).
