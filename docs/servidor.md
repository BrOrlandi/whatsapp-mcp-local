# Num servidor

O WhatsApp MCP pode rodar num servidor que fica ligado o tempo todo: um
computador em casa, um mini PC, uma VPS. Há dois jeitos:

- **Modo servidor:** o painel e o MCP ficam num endereço `https` na internet,
  onde tudo pede a senha do painel, uma chave ou um login. É o que permite usar
  o seu WhatsApp no Claude pelo navegador e pelo celular, no ChatGPT e em
  ferramentas de IA de outros computadores. É a maior parte deste guia.
- **Só no próprio servidor:** sem endereço na internet. Quem usa o WhatsApp é
  um programa no próprio servidor (o Claude Code, um agente, um script). Está
  em [Só no próprio servidor](#só-no-próprio-servidor), no fim.

## Modo servidor

> **Chega com a versão 1.9.0.** Este guia já descreve o modo servidor, mas a
> imagem Docker e os comandos `server` só existem a partir dela.

O servidor passa a ter duas portas:

- **A local**, como sempre: `127.0.0.1:47821`, sem senha, só para os
  programas do próprio servidor e para a linha de comando.
- **A pública**, `47822`, onde tudo pede login: o painel, o MCP, as mídias e o
  QR code de conexão. É para ela que o `https` aponta.

O app nunca atende a internet direto nem cuida de certificado: quem faz o
`https` é um programa na frente dele. O instalador oferece quatro opções:

| Opção | Precisa de | Para quem |
|---|---|---|
| **Tailscale Funnel** (recomendada) | Uma conta grátis no Tailscale | Servidor em casa, sem domínio e sem abrir porta. O endereço fica como `https://whatsapp.sua-rede.ts.net` |
| **Túnel da Cloudflare** | Conta na Cloudflare e um domínio nela | Quem quer o próprio domínio sem abrir porta |
| **Caddy** | Um domínio apontado para o servidor, portas 80 e 443 abertas | VPS com IP público. O Caddy tira o certificado sozinho |
| **Um proxy que você já tem** | nginx, Caddy ou outro, instalado no próprio servidor | Quem já tem um servidor organizado |

### A senha do painel

A senha do painel é o que protege as suas conversas na internet: quem a
souber entra no painel e libera qualquer ferramenta de IA. Use uma **senha
longa e aleatória**, gerada por um gerenciador de senhas, com 16 caracteres
ou mais. O app aceita a partir de 10 caracteres: isso é um piso, não uma
recomendação. Uma senha curta ou que já foi usada em outro lugar não serve
aqui.

O porquê: o endereço está na internet, e qualquer pessoa pode tentar adivinhar
a senha. O app dificulta (espera crescente depois de erros, por endereço, e um
limite para todos juntos de cerca de 21 mil tentativas por dia), mas só uma
senha longa e aleatória torna a adivinhação inútil. Quando alguém tenta
demais, o registro mostra o aviso *"someone may be guessing it"*.

Durante uma enxurrada de tentativas, o navegador em que você já entrou com a
senha certa continua entrando em segundos, de qualquer endereço: ele fica
conhecido por um cookie, que a troca de senha encerra. Num navegador novo, a
espera é maior, mas a sua vez chega.

A senha nunca fica gravada: o app guarda só um resumo dela (um hash bcrypt).

## Instalar com Docker

Num servidor Linux com o Docker instalado:

```sh
curl -fsSL https://raw.githubusercontent.com/BrOrlandi/whatsapp-mcp-local/main/install-docker.sh -o install-docker.sh
bash install-docker.sh
```

O instalador pergunta:

1. **A pasta da instalação** (padrão `~/whatsapp-mcp`). Ali ficam o
   `compose.yml` e o `.env`, com as respostas, que só o seu usuário consegue
   ler. Os dados (a sessão do WhatsApp, as mensagens, as chaves) ficam num
   volume do Docker.
2. **O endereço público**, com `https://`.
3. **Como o `https` chega**, uma das quatro opções acima, com o que cada uma
   precisa (veja as seções abaixo).
4. **A senha do painel**, duas vezes. Ela não aparece na tela e não vai para
   nenhum arquivo: só o hash vai para o `.env`.

Para rodar sem perguntas, as respostas vêm do ambiente (`WAMCP_DIR`,
`WAMCP_URL`, `WAMCP_PROFILE`...), e cada segredo vem de um arquivo que só você
lê: `WAMCP_PASSWORD_FILE` para a senha, `WAMCP_TS_AUTHKEY_FILE` para a chave do
Tailscale e `WAMCP_TUNNEL_TOKEN_FILE` para o token do túnel. Não escreva um
segredo direto na linha de comando: ele fica no histórico do shell e, enquanto
o comando roda, aparece para os outros usuários do servidor. O começo do
`install-docker.sh` lista todas as variáveis.

Depois ele baixa a imagem, sobe tudo e diz o que fazer em seguida. Para mudar
uma resposta, rode o instalador de novo: ele mostra as respostas de antes e
mantém os dados. Para trocar só a senha, veja [Trocar a
senha](#trocar-a-senha).

A imagem é `ghcr.io/brorlandi/whatsapp-mcp-local`, para servidores `amd64` e
`arm64` (Raspberry Pi 4 e 5, servidores ARM). No `amd64`, o processador
precisa ter AVX2, uma exigência do wacli; para conferir, `grep -c avx2
/proc/cpuinfo` precisa dar mais que zero.

### Tailscale Funnel

> **A confirmar:** esta opção ainda não foi testada de ponta a ponta com um
> servidor real e os conectores do Claude e do ChatGPT. Os passos abaixo são
> os previstos.

1. Crie uma conta no [Tailscale](https://tailscale.com) (é grátis).
2. No painel do Tailscale, em *Settings › Keys*, gere uma **chave de
   autenticação** (`tskey-auth-...`). Ela entra uma vez e o servidor fica na
   sua rede.
3. Ligue o **Funnel** para a sua rede: em *Access controls*, o atributo
   `funnel` (o painel do Tailscale oferece o botão na primeira vez).
4. Descubra o nome da sua rede (*DNS › Tailnet name*, algo como
   `sua-rede.ts.net`). O endereço público será
   `https://whatsapp.sua-rede.ts.net`, onde `whatsapp` é o nome que você
   escolhe para o servidor.
5. Rode o instalador e escolha `tailscale`. Ele pede o endereço e a chave.

O Funnel tem um limite de banda que o Tailscale não divulga, o que pode pesar
ao enviar e baixar mídias grandes.

### Túnel da Cloudflare

> **A confirmar:** esta opção ainda não foi testada de ponta a ponta com um
> servidor real. Os passos abaixo são os previstos.

1. No painel da Cloudflare, em *Zero Trust › Networks › Tunnels*, crie um
   túnel do tipo **cloudflared** e copie o **token**.
2. No túnel, em *Public hostname*, adicione o endereço que você quer (como
   `whatsapp.example.com`) apontando para o serviço `http://whatsapp-mcp:47822`.
3. Rode o instalador e escolha `tunnel`. Ele pede o endereço e o token.

O túnel rápido da Cloudflare (`trycloudflare.com`, sem conta) troca de
endereço a cada vez que sobe: não serve para um conector fixo.

### Caddy

1. Aponte o seu domínio (como `whatsapp.example.com`) para o IP do servidor.
2. Abra as portas 80 e 443 no firewall.
3. Rode o instalador e escolha `caddy`.

O certificado sai sozinho no primeiro acesso.

> **A confirmar:** testado só com o certificado local do Caddy
> (`https://localhost`). A emissão do certificado para um domínio de verdade
> ainda não foi vista de ponta a ponta.

### Um proxy que você já tem

> **Para um servidor em que você confia em todas as contas.** Nesta opção,
> qualquer conta do próprio servidor alcança a porta pública como se fosse o
> proxy, e pode dizer o endereço que quiser ao app. A porta local
> (`127.0.0.1:47821`) também não pede senha a nenhuma conta do servidor, a
> não ser que você defina um [token](#token). Num
> servidor dividido com outras pessoas, use o Tailscale, o túnel ou o Caddy.

Escolha `proxy` no instalador. A porta pública fica só no `127.0.0.1` do
servidor (padrão `47822`), para o seu proxy encaminhar; nada mais na rede a
alcança. Duas regras, sem as quais a proteção contra quem tenta adivinhar a
senha não vale:

- **O proxy precisa trocar ou completar o `X-Forwarded-For`** com o endereço
  de quem visita (no nginx, `proxy_add_x_forwarded_for`), e mandar
  `X-Forwarded-Proto: https`. O instalador imprime um exemplo para o nginx.
- **Nunca confie numa faixa inteira de rede do Docker** em
  `WHATSAPP_MCP_TRUSTED_PROXY` enquanto a porta também estiver publicada no
  servidor: qualquer um que chegue pela porta publicada pareceria vir dessa
  faixa, e poderia mentir o próprio endereço. Confie só no endereço exato do
  proxy (ou no nome do contêiner dele).

O instalador cobre um proxy instalado no próprio servidor, que alcança
`127.0.0.1:47822`. Um proxy que roda num contêiner (o Traefik, por exemplo)
não chega a esse endereço e pede ajustes que o instalador não faz; esse caso
não está coberto aqui.

## Instalar no Linux sem Docker

> **Para um servidor em que você confia em todas as contas.** Sem Docker, um
> proxy no próprio servidor é reconhecido pelo endereço local, e qualquer conta
> do servidor que fale com a porta pública é tratada como esse proxy, podendo
> dizer o endereço que quiser. A porta local (`127.0.0.1:47821`) também não
> pede senha a nenhuma conta do servidor, a não ser que você defina um
> [token](#token). O arquivo do serviço, quando guarda o
> token, só o seu usuário lê. Num servidor dividido com outras pessoas, prefira
> o Docker com o Tailscale, o túnel ou o Caddy.

```sh
curl -fsSL https://raw.githubusercontent.com/BrOrlandi/whatsapp-mcp-local/main/install.sh | bash
whatsapp-mcp server setup
```

O `server setup` pergunta o endereço público e a senha do painel (duas vezes,
sem mostrar), guarda tudo na pasta de dados e reinstala o serviço, que passa a
atender também a porta pública, em `127.0.0.1:47822`. Aponte para ela o seu
`https`: o Caddy ou o nginx do servidor, o `tailscale funnel 47822`, ou o
`cloudflared` instalados no próprio servidor. Um proxy no próprio servidor é
reconhecido sozinho; um em outra máquina vai em `--trusted-proxy`, mas só por
uma rede privada ou com TLS entre ele e o app: o caminho entre os dois leva a
senha e os tokens, e em HTTP simples numa rede que outros veem eles ficariam
expostos.

> **A confirmar:** o `tailscale funnel` e o `cloudflared` instalados direto no
> servidor, sem Docker, ainda não foram testados com este modo.

| Comando | O que faz |
|---|---|
| `whatsapp-mcp server` | Diz se o modo servidor está ligado, e o endereço |
| `whatsapp-mcp server setup` | Liga, ou muda o endereço (`--url`), a porta (`--listen`) e o proxy (`--trusted-proxy`); `--new-password` troca a senha |
| `whatsapp-mcp server off` | Volta a atender só o próprio servidor |

As variáveis `WHATSAPP_MCP_PUBLIC_URL`, `WHATSAPP_MCP_PUBLIC_LISTEN`,
`WHATSAPP_MCP_TRUSTED_PROXY` e `WHATSAPP_MCP_PANEL_PASSWORD_HASH`, quando
definidas, vencem o que o `server setup` guardou.

## Conectar o WhatsApp

Abra o endereço público no navegador, entre com a senha e siga o painel: o QR
code aparece na tela. No celular, *WhatsApp › Configurações › Dispositivos
conectados › Conectar dispositivo*. O histórico continua chegando em segundo
plano por alguns minutos.

Pelo terminal do servidor também dá:
`docker compose exec whatsapp-mcp whatsapp-mcp pair` (com Docker, na pasta da
instalação) ou `whatsapp-mcp pair` (sem Docker). O QR code sai desenhado no
terminal ([detalhes](#conectar-o-whatsapp-pelo-terminal)).

## Conectar as ferramentas de IA

O endereço do MCP é o endereço público com `/mcp` no fim, como
`https://whatsapp.example.com/mcp`. O painel o mostra em *Conectar MCP*. Há
dois jeitos de uma ferramenta entrar:

- **Pelo login:** você cola o endereço na ferramenta, ela abre uma página do
  seu servidor, você digita a senha do painel e escolhe o que ela pode fazer.
  É o jeito do Claude no navegador e no celular e do ChatGPT.
- **Com uma chave:** você cria uma chave no painel, em *Conexões*, e a
  ferramenta a manda em cada pedido. É o jeito das ferramentas que não fazem
  login: scripts, n8n, alguns agentes.

Na página de autorização, o que aparece em destaque é o endereço para onde
você volta depois de aprovar, como `claude.ai`. O nome que o aplicativo dá a
si mesmo vem embaixo, menor, porque qualquer um pode escolher um nome. Na
primeira vez que um aplicativo pede acesso, a página sempre pede a senha,
mesmo com o painel aberto. Confira o endereço antes de aprovar.

### Claude (navegador, celular e app)

> **A confirmar:** ainda não testado com o Claude de verdade. Os passos
> abaixo são os previstos.

1. No Claude, em *Configurações › Conectores*, *Adicionar conector
   personalizado*.
2. Cole o endereço do MCP (`https://whatsapp.example.com/mcp`).
3. O Claude abre a página do seu servidor: confira que ela diz que você volta
   para `claude.ai`, escolha *Tudo* ou *Só ler*, digite a senha e aprove.

O conector vale no navegador, no celular e no app do Claude ao mesmo tempo.

### ChatGPT

> **A confirmar:** ainda não testado com o ChatGPT de verdade. Os passos
> abaixo são os previstos.

1. No ChatGPT, nas configurações de *Apps e conectores* (é preciso ativar o
   modo de desenvolvedor), crie um conector com o endereço do MCP.
2. Na página do seu servidor, confira que você volta para `chatgpt.com`,
   escolha o que ele pode fazer, digite a senha e aprove.

### Claude Code

Com login (o Claude Code abre o navegador para a senha):

> **A confirmar:** o login pelo Claude Code ainda não foi testado com o
> Claude Code de verdade; o mesmo fluxo foi testado com o cliente oficial do
> SDK do MCP.


```sh
claude mcp add --transport http whatsapp https://whatsapp.example.com/mcp
```

Ou com uma chave criada em *Conexões*:

```sh
claude mcp add --transport http whatsapp https://whatsapp.example.com/mcp \
  --header "Authorization: Bearer wamcp_key_..."
```

### Uma ferramenta que só aceita chave

Crie a chave em *Conexões* (ou no servidor, com
`whatsapp-mcp apikey create --name "Nome da ferramenta"`) e configure a
ferramenta com o endereço do MCP e o cabeçalho
`Authorization: Bearer <chave>`.

Para uma ferramenta que só sabe iniciar um programa (como o Claude Desktop),
o `whatsapp-mcp` do seu computador faz a ponte, sem o WhatsApp instalado nele:

```json
"whatsapp": {
  "command": "whatsapp-mcp",
  "args": ["bridge", "--url", "https://whatsapp.example.com/mcp"],
  "env": { "WHATSAPP_MCP_API_KEY": "wamcp_key_..." }
}
```

A chave nunca vai num argumento: na variável `WHATSAPP_MCP_API_KEY` ou num
arquivo (`--key-file`).

### O que muda para uma ferramenta de fora

- Um arquivo para enviar precisa estar num endereço `https` na internet: o
  servidor não lê arquivos do seu computador.
- Mídias e exportações chegam como um link no endereço público, que vale por
  dez minutos e não pede senha.
- As ferramentas que perguntam ao WhatsApp sobre números e grupos
  (`check_numbers`, a foto de perfil, o grupo atualizado) têm um limite por
  conexão: 20 de uma vez, depois uma por minuto.

## Conexões

A página *Conexões* do painel lista quem entrou pelo endereço público: os
logins e as chaves, com o que cada um pode fazer, quando entrou e quando usou
pela última vez. *Cortar acesso* vale na hora, e o aplicativo que tinha
aquele login passa a pedir a senha de novo se tentar voltar.

- **Só ler:** um login ou uma chave *Só ler* vê e busca as conversas, mas não
  envia, não apaga e não mexe em grupos. Escolha na hora de aprovar o login ou
  de criar a chave.
- **Chave esquecida:** uma chave que ninguém usa nas primeiras 24 horas deixa
  de valer, e a página a mostra como *Expirada*.
- Pelo terminal: `whatsapp-mcp apikey list`, `create` e `revoke`.

## Trocar a senha

Trocar a senha encerra todas as sessões do painel e todos os logins das
ferramentas, que terão de entrar de novo. **As chaves continuam valendo:**
depois de trocar a senha, abra *Conexões* e corte as chaves que não reconhecer
ou não usar mais.

- Com Docker: `WAMCP_NEW_PASSWORD=1 bash install-docker.sh`, respondendo a
  mesma pasta da instalação.
- Sem Docker: `whatsapp-mcp server setup --new-password`.

## Atualizar

- Com Docker, na pasta da instalação:
  `docker compose pull && docker compose up -d`. Os dados ficam.
- Os programas na frente do app (Tailscale, cloudflared, Caddy) estão presos a
  uma versão no `compose.yml` (`tailscale/tailscale:v1.104`,
  `cloudflare/cloudflared:2026.11.1`, `caddy:2.11`). Rodar o instalador de novo
  traz o `compose.yml` da versão atual, com as versões novas deles. Para
  avançar um por conta própria, troque a versão no `compose.yml` (as
  disponíveis estão no Docker Hub de cada um) e rode o mesmo
  `docker compose pull && docker compose up -d`.
- Sem Docker: `whatsapp-mcp update`.

O painel avisa quando sai uma versão nova, num aviso no canto da página e em
**Configurações › Versão e atualizações**, que mostra a versão instalada e o
comando acima para o seu tipo de instalação. Ele só avisa: quem atualiza é
você, no servidor. Se o `.env` fixa uma versão em `WHATSAPP_MCP_VERSION`,
troque-a antes do `docker compose pull`.

**Configurações › Endereços do servidor** mostra o endereço público e o
endereço na rede interna (`http://whatsapp-mcp:47822` no Docker), que é para
onde aponta o programa na frente do app. Os dois são só para consulta.

## Backup

Com Docker, tudo o que importa está no volume `whatsapp-mcp_data`: a sessão
do WhatsApp, as mensagens, as chaves e os logins. Para copiar, com o
contêiner parado:

```sh
docker compose stop
docker run --rm -v whatsapp-mcp_data:/data -v "$PWD":/backup debian:stable-slim \
  tar czf /backup/whatsapp-mcp-backup.tar.gz -C /data .
docker compose start
```

Guarde o arquivo como guardaria uma senha: quem o tiver tem o seu WhatsApp.
Para voltar um backup num servidor novo, crie o volume com o instalador,
pare o contêiner e extraia o arquivo no volume do mesmo jeito (`tar xzf`).

Sem Docker, os dados ficam em `~/.whatsapp-mcp` e na pasta do wacli
(`~/.local/state/wacli` no Linux).

## Problemas

| O que aparece | O que fazer |
|---|---|
| `this request did not arrive over HTTPS: point the tunnel or proxy at this port, or set WHATSAPP_MCP_TRUSTED_PROXY to the proxy's address` | O pedido chegou sem `https`. Abra o endereço com `https://`. Com um proxy seu, confira que ele manda `X-Forwarded-Proto: https` e que o endereço dele está em `WHATSAPP_MCP_TRUSTED_PROXY` |
| `this address answers only through its proxy` | Algo diferente do proxy tentou falar com a porta pública. Confira `WHATSAPP_MCP_TRUSTED_PROXY` |
| O serviço não sobe e diz que o modo servidor precisa da senha do painel | Falta a senha: rode o instalador de novo, ou `whatsapp-mcp server setup` |
| `... está definida, mas WHATSAPP_MCP_PUBLIC_URL não` | Só metade da configuração está no `.env` ou nas variáveis. Rode o instalador de novo |
| `o hash bcrypt da senha do painel tem custo ...` ou `precisa ser um hash bcrypt` | Alguém pôs a senha em texto, ou um hash fraco, no `.env`. Gere de novo com o instalador |
| *Senha errada* | Confira a senha. Depois de algumas erradas, o mesmo endereço espera um pouco mais a cada vez |
| *O servidor está recebendo muitas tentativas de senha agora* | Muita gente (ou alguém) está tentando senhas. Espere alguns segundos e tente de novo. Com uma senha longa e aleatória, não há risco |
| Na página de autorização: *o endereço de retorno deste pedido não é um dos que o aplicativo declarou* | O pedido não veio do aplicativo que diz ser. Não continue |
| Na página de autorização: *não foi possível ler a descrição do aplicativo* | O servidor não conseguiu baixar a descrição do aplicativo. Tente de novo em um minuto |
| A ferramenta recebe `401` com `resource_metadata` | Falta a chave ou o login, ou eles foram cortados em *Conexões*. Conecte de novo |
| A ferramenta recebe *not allowed for this connection* | A conexão é *Só ler*. Para enviar, crie outra com *Tudo* |
| A ferramenta recebe *asked WhatsApp about too many numbers or groups lately* | O limite das consultas ao WhatsApp. Espere o tempo que a mensagem diz |
| O contêiner não fica `healthy` | `docker compose logs whatsapp-mcp` mostra o motivo |
| `CPU ISA level is lower than required` no registro | O processador `amd64` não tem AVX2 (`grep -c avx2 /proc/cpuinfo` dá zero), e o wacli não roda nele |

O registro, com Docker: `docker compose logs -f whatsapp-mcp`. Sem Docker:
`whatsapp-mcp logs -f`.

## Só no próprio servidor

Sem o modo servidor, o MCP responde só em `127.0.0.1`: quem usa o WhatsApp é
um programa no próprio servidor. A linha de comando faz pelo terminal tudo o
que o painel faz: instala o serviço, conecta o WhatsApp, mostra o estado,
troca a porta e o token, configura webhooks, mostra o registro e se atualiza.

### Sem Docker

No macOS e no Linux, sem `sudo`:

```sh
curl -fsSL https://raw.githubusercontent.com/BrOrlandi/whatsapp-mcp-local/main/install.sh | bash
```

O instalador põe o `wacli` e o `whatsapp-mcp` em `~/.local/bin`, registra o
serviço (systemd do usuário no Linux, launchd no macOS) e, sem tela, não tenta
abrir o navegador. Depois:

```sh
whatsapp-mcp pair      # o QR code aparece no terminal
whatsapp-mcp status    # "pronto" quando o WhatsApp estiver conectado
whatsapp-mcp config    # como ligar o Claude Code e outras ferramentas
```

No Linux, um serviço do usuário para quando a última sessão SSH termina, a
menos que o systemd mantenha os serviços desse usuário rodando (o *linger*).
O `service install` liga o linger sozinho quando o sistema deixa; quando não
deixa, ele avisa, e o comando é:

```sh
sudo loginctl enable-linger $USER
```

### Com Docker, só no servidor

Para usar só no servidor, sem modo servidor, a imagem é montada do
repositório com outro compose, que usa a rede do próprio servidor:

```sh
git clone https://github.com/BrOrlandi/whatsapp-mcp-local.git
cd whatsapp-mcp-local
mkdir -p build/docker/data && sudo chown 1000:1000 build/docker/data
docker compose -f build/docker/compose.yaml up -d --build
docker compose -f build/docker/compose.yaml exec whatsapp-mcp whatsapp-mcp pair
```

O que importa no [`build/docker/compose.yaml`](../build/docker/compose.yaml):

- **`network_mode: host`.** O daemon só responde em `127.0.0.1`; com a rede do
  servidor, o MCP fica em `http://127.0.0.1:47821/mcp` no próprio servidor.
- **O volume `/data`** guarda tudo o que precisa sobreviver ao contêiner. A
  pasta `data/`, ao lado do compose, precisa existir e pertencer ao usuário
  1000 do contêiner antes do primeiro `up`.
- **O contêiner é o serviço.** `docker compose up -d`, `restart` e `stop`
  fazem o papel dos comandos `service`, e o registro sai em
  `docker compose logs -f`.
- **Atualizar** é montar a imagem de novo:
  `git pull && docker compose -f build/docker/compose.yaml up -d --build`.

### Conectar o WhatsApp pelo terminal

```sh
whatsapp-mcp pair
```

O QR code aparece no terminal, desenhado com meios-blocos (cerca de 65
colunas por 33 linhas), e é trocado na tela a cada vez que o WhatsApp troca o
código: o primeiro dura um minuto, os seguintes uns 20 segundos. No celular:
*WhatsApp › Configurações › Dispositivos conectados › Conectar dispositivo*.
Quando o celular confirma, o comando termina; o histórico continua chegando
em segundo plano, e o `status` mostra quando acabou.

Abaixo do desenho, o comando imprime **o conteúdo do QR code**, o texto que
ele codifica, para outra interface gerar a imagem: um app, uma página, um bot
do Telegram. Hoje esse conteúdo é um link
(`https://wa.me/settings/linked_devices#2@…`), mas trate-o como um texto
qualquer. Quem tiver o `qrencode` gera a imagem com
`qrencode -t ansiutf8 '<conteúdo>'`. O comando também dá o endereço da imagem
PNG no próprio servidor (`/api/pair/qr.png`), e `--qr-file qr.png` grava a
imagem num arquivo, trocado a cada código novo.

> O conteúdo do QR code liga a conta de quem o ler a este servidor. Não o
> publique nem o mande a um gerador de QR code na internet; passe-o só para
> quem vai ler o código.

| Opção | O que faz |
|---|---|
| `--phone '+55 11 91234-5678'` | Em vez do QR code, um código de 8 caracteres para digitar em *Conectar dispositivo › Conectar com número de telefone*. O número vai com DDI e DDD |
| `--timeout 5m` | Desiste depois desse tempo (saída 5) e cancela a conexão |
| `--no-wait` | Termina assim que mostra o primeiro QR code (ou o código); a conexão continua, e outro programa acompanha pela API |
| `--qr-file <arquivo>` | Grava o QR code em PNG, trocado a cada código novo |
| `--invert` | Sem cores (num `exec -T`, num log), desenha para um terminal de fundo claro |
| `--json` | Uma linha de JSON por acontecimento ([abaixo](#automação)) |

Ctrl+C cancela a conexão. Com o WhatsApp já conectado, `pair` só avisa e
termina com sucesso; para trocar de número, `whatsapp-mcp logout` antes.

### Os comandos

| Comando | O que faz |
|---|---|
| `status` | Onde as coisas estão e o que fazer em seguida |
| `pair` | Conecta o WhatsApp ([acima](#conectar-o-whatsapp-pelo-terminal)) |
| `logout [--yes]` | Desconecta o WhatsApp deste servidor; sem terminal, pede `--yes` |
| `service install` | Instala e inicia o serviço; `uninstall`, `stop`, `start` e `restart` |
| `server [setup\|off]` | O modo servidor sem Docker ([acima](#instalar-no-linux-sem-docker)) |
| `password hash\|set` | A senha do painel do modo servidor: o hash, ou guardar aqui |
| `apikey [list\|create\|revoke]` | As chaves das ferramentas de fora |
| `port [<porta>]` | Mostra a porta do MCP ou troca (de 1024 a 65535), reiniciando o serviço |
| `token [show\|new\|set <token>\|clear]` | A chave que o `/mcp`, o `/health` e as ações da API pedem na porta local ([abaixo](#token)) |
| `webhooks` | Lista; `add <url>`, `remove <id>`, `enable <id>`, `disable <id>`, `test <id>` |
| `logs [-f] [-n 100]` | O registro do serviço (`journalctl` no Linux, o arquivo de log no macOS) |
| `update [--check]` | Baixa a versão mais recente, confere o sha256 e reinicia o serviço |
| `config` | Como configurar o Claude Code e o Claude Desktop |

`webhooks add` aceita `--events message,reaction,receipt,online`, `--include-own` e
`--chats <jid>,<jid>`, os mesmos campos da [API](api.md#webhooks). O segredo
que assina as entregas aparece uma vez, na criação.

A porta e o token ficam na pasta de dados (`~/.whatsapp-mcp/`, ou `/data/mcp`
no Docker): a porta no `config.json`, o token no arquivo `token`, legível só
pelo dono. As variáveis `WHATSAPP_MCP_PORT` e `WHATSAPP_MCP_TOKEN` vencem os
arquivos; quando uma delas está definida, `port` e `token` não trocam o valor
e dizem para mudar a variável. No Docker, a troca vale depois de
`docker compose restart`.

### Token

Sem token, qualquer programa do servidor usa o MCP. `whatsapp-mcp token new`
gera um, reinicia o serviço e o mostra; a partir daí, o `/mcp`, o `/health` e
as ações da API pedem `Authorization: Bearer <token>` na porta local, e os
próprios comandos o usam sozinhos. As leituras da API (`GET`) continuam
abertas para os programas do servidor. `whatsapp-mcp config` já traz o token
na configuração de cada ferramenta. O token não vale na porta pública, onde
quem entra usa uma chave ou um login.

### Automação

Os comandos `status`, `pair`, `logout`, `port`, `token`, `webhooks`, `server`,
`apikey` e `update` aceitam `--json`: o resultado sai como um objeto JSON na
saída padrão, e um erro também, como `{"error": "…", "exit_code": 4}`. O
`status --json` é o mesmo [`GET /api/status`](api.md#get-apistatus).

O `pair --json` escreve uma linha por acontecimento:

```json
{"event":"qr","qr":"https://wa.me/settings/linked_devices#2@…","qr_png":"http://127.0.0.1:47821/api/pair/qr.png","qr_version":1}
{"event":"qr","qr":"https://wa.me/settings/linked_devices#2@…","qr_png":"http://127.0.0.1:47821/api/pair/qr.png","qr_version":2}
{"event":"paired","phone":"+55 (11) 91234-5678","name":"Maria Silva"}
```

Com `--phone`, o primeiro é `{"event":"code","code":"ABCD-EFGH","phone":"…"}`;
com o WhatsApp já conectado, `{"event":"already_paired",…}`.

| Saída | Significa |
|---|---|
| 0 | Feito; no `status`, o WhatsApp está pronto |
| 1 | Algo deu errado; a mensagem diz o quê |
| 2 | O comando foi dado errado |
| 3 | O daemon não responde na porta (o serviço não está rodando) |
| 4 | O WhatsApp não está pronto (`status`: o campo `next` diz por quê) ou a conexão terminou sem ele |
| 5 | O `--timeout` acabou |
| 6 | Não dá para fazer aqui: uma porta ocupada, um valor fixado por variável, um comando que não vale no Docker, um pedido que a API recusou |
| 130 | Interrompido com Ctrl+C |

Um bot que conecta o WhatsApp por outra tela, por exemplo, roda
`whatsapp-mcp pair --json`, manda cada `qr` como imagem para a pessoa e para
quando chega `paired`. Ou pede o código: `pair --phone <número> --json
--no-wait` devolve o código e deixa a conexão correndo, e `status --json`
diz quando terminou.

### De outro computador, sem o modo servidor

A porta local recusa qualquer pedido que não venha de `127.0.0.1`. Para usar
o painel ou o MCP de outro computador sem o modo servidor, um túnel SSH leva a
porta até lá:

```sh
ssh -N -L 47821:127.0.0.1:47821 usuario@servidor
```

Com o túnel aberto, o painel fica em `http://127.0.0.1:47821/` e o MCP em
`http://127.0.0.1:47821/mcp` no seu computador. Para o Claude no navegador e no
celular, ou o ChatGPT, use o [modo servidor](#modo-servidor).

## Transcrição de áudio

No servidor, a transcrição roda só no processador: a mesma
`whatsapp-mcp transcription install` baixa o whisper.cpp e o modelo (cerca de
600 MB). Um áudio leva mais ou menos a sua própria duração para virar texto, ou
mais num processador fraco ([Transcrição](transcricao.md)). Há dois jeitos de
tirar esse trabalho do servidor, escolhidos no `.env` (que vence o painel) ou em
*Configurações › Transcrição de áudio*:

- **Um servidor compatível com a API de transcrição da OpenAI**, em qualquer
  endereço (um serviço na internet, um servidor seu, um computador com GPU):
  `WHATSAPP_MCP_TRANSCRIPTION_URL` com o endereço dele e
  `WHATSAPP_MCP_TRANSCRIPTION_TOKEN` com a chave, quando ele pede uma;
  `WHATSAPP_MCP_TRANSCRIPTION_MODEL` escolhe o modelo quando ele pede um. Um
  exemplo: um Mac com chip Apple rodando `whatsapp-mcp transcription serve`
  transcreve em segundos, com `URL=http://<ip-do-mac>:47823` e a chave do
  arquivo `transcription-server.token` do Mac ([o passo a passo no
  README](../README.md#transcrever-num-servidor-compatível-com-a-openai)). Outro é o
  `whisper-server` do whisper.cpp, rodado com
  `--inference-path /v1/audio/transcriptions --convert -l auto` (sem o
  `-l auto`, ele assume inglês).
- **A OpenAI**, com a sua chave: `WHATSAPP_MCP_TRANSCRIPTION_MODE=openai`,
  `WHATSAPP_MCP_TRANSCRIPTION_OPENAI_KEY=sk-…` e, se quiser,
  `WHATSAPP_MCP_TRANSCRIPTION_MODEL=gpt-4o-mini-transcribe` (padrão
  `whisper-1`). O áudio vai para a OpenAI.

Nos dois, quando o servidor ou a OpenAI não responde, o áudio é transcrito no
próprio servidor, se a transcrição estiver instalada nele
([Os três modos](transcricao.md#os-três-modos)). Do contexto da conversa, por
padrão, nada vai com o áudio para fora; `WHATSAPP_MCP_TRANSCRIPTION_CONTEXT`
escolhe `none` (só o áudio), `names` (o nome da conversa e os das pessoas) ou
`messages` (também as últimas mensagens). Na transcrição no próprio servidor
ela não vale (o log avisa): lá vão sempre só os nomes, que não saem dele. Sem
a variável, dá para escolher no painel, em Contexto da conversa
([o nível de contexto](transcricao.md#o-nível-de-contexto)). Se a sua ferramenta de IA
desiste de esperar antes de 4 minutos, `WHATSAPP_MCP_TRANSCRIPTION_WAIT` diz
em quantos segundos a resposta passa a ser "ainda transcrevendo, chame de
novo". Depois de mudar o `.env`, rode `docker compose up -d`.
