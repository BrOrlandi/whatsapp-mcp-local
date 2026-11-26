# Changelog

O que muda para quem usa o WhatsApp MCP, versão a versão. Formato baseado no
[Keep a Changelog](https://keepachangelog.com/pt-BR/1.1.0/), com versões em
[SemVer](https://semver.org/lang/pt-BR/).

## [Não lançado]

### Mudado

- **Servidor: a página de permitir acesso tem a cara do WhatsApp MCP.** Quando
  uma ferramenta de IA pede acesso ao seu WhatsApp pelo servidor, a página
  agora vem no verde escuro do site, com o logo, e mostra o número que vai ser
  conectado, com o meio escondido (como +55 (16) 9••••-6892). A página de
  entrar no painel ganhou o mesmo visual. A tela de "pronto" que aparece
  depois de aprovar é da própria ferramenta de IA, não do servidor.
- **Servidor: a ferramenta que entra pelo login aparece com o próprio nome.**
  Em *Suas conexões*, uma ferramenta conectada pelo login (como o Orca) vem
  com o nome que a página de permitir acesso mostrou, em vez de "mcp". Sem
  nome, ela aparece pelo endereço de onde veio, ou como "Conector". Uma linha
  antiga com o nome "mcp" continua lá até você removê-la em *Gerenciar*.
- **Contexto da conversa só para fora.** Transcrevendo neste computador (ou
  neste servidor), vão sempre só os nomes da conversa e das pessoas, que não
  saem dele, e o card de *Transcrição de áudio* não pede mais essa escolha. O
  *Contexto da conversa* aparece só quando os áudios vão para um servidor
  compatível com a OpenAI ou para a OpenAI, e começa em **Nada**. Um nível
  escolhido antes neste computador deixa de valer aqui, sem ser apagado. No
  terminal, `whatsapp-mcp transcription context` e a variável
  `WHATSAPP_MCP_TRANSCRIPTION_CONTEXT` passam a valer só nesses dois modos.
- **Ajuda › Transcrever áudios** diz numa frase onde os seus áudios são
  transcritos agora, com um botão para trocar (ou instalar), e mantém o
  exemplo de como pedir. A explicação de cada opção foi para *Configurações ›
  Transcrição de áudio › Trocar onde transcrever*, em "Para que serve cada
  opção".
- A versão no rodapé do painel e o novo card **Novidades**, na Ajuda, levam
  ao que mudou em cada versão, em whatsappmcp.com.br/changelog.

### Corrigido

- **Servidor: os passos de conexão usam o endereço do servidor.** Vistos de
  fora do servidor, os passos de *Conectar MCP* mostravam o endereço interno
  (`127.0.0.1`), arquivos de dentro do servidor e um botão que não podia
  funcionar ali. Agora o Claude Code, o ChatGPT / Codex, o Cursor e as outras
  ferramentas usam o endereço público do servidor com uma chave criada em
  *Conexões*, nos arquivos do seu computador, e o Claude Desktop entra como
  conector, com a senha do painel.
- No Linux, o app fechava ao trocar de página (abrir a Ajuda, as
  Configurações ou qualquer aba) com as animações do GNOME ligadas, desde a
  1.9.6. A transição entre as páginas fica desligada só no app do Linux; no
  macOS, no Windows e no navegador ela continua. Obrigado ao Lucas Ogawa, que
  achou a causa e mandou a correção.
- No Linux, o app podia fechar sozinho ao abrir ou quando o estado da
  conexão mudava, por atualizar o menu do ícone da bandeja fora de hora.
- O servidor de transcrição (`whatsapp-mcp transcription serve`) rodando na
  imagem Docker falhava em todo áudio, com "stat /data/tmp". Agora transcreve
  sem configuração extra; até a 1.9.6, o contorno é `TMPDIR=/tmp` no
  contêiner ([Servidor de transcrição](docs/servidor-de-transcricao.md)).

Inclui o wacli 0.26.1.

## [1.9.6] - 2026-11-23

O painel fica mais simples de usar, no computador e no servidor, e a
transcrição de áudios ganha um nome e uma checagem mais claros. Quem usa o app
no computador recebe esta versão pelo atualizador. Num servidor com Docker,
rode o instalador de novo (`install-docker.sh`): ele traz o `compose.yml`
desta versão e mantém o `.env` e os dados. Só trocar a versão e rodar
`docker compose pull && docker compose up -d` atualiza o app, mas não traz o
`compose.yml` novo, e sem ele não chegam a proteção do Docker nem as variáveis
`WHATSAPP_MCP_TRANSCRIPTION_WAIT` e `WHATSAPP_MCP_TRANSCRIPTION_CONTEXT`
descritas abaixo.

O que você pode notar ao atualizar:

- **Se você definiu um token** (`WHATSAPP_MCP_TOKEN`), o painel aberto no
  navegador pelo endereço local pede esse token uma vez, numa página própria.
  O `whatsapp-mcp open` já abre o painel com você dentro, e a janela do app
  não pede nada.
- **No Docker, com o `compose.yml` novo**, o app roda com o sistema só de
  leitura e sem privilégios extras, e escreve só na pasta de dados. As mídias
  enviadas e os áudios sendo transcritos passam por `/data/tmp`, no disco do
  volume de dados (não na memória), que é esvaziada a cada início.
- **Se os seus áudios vão para a OpenAI ou para um servidor de transcrição**,
  o nome da conversa e as mensagens ao redor deixam de ir junto com eles: por
  padrão, vai só o áudio. Para voltar ao que era, escolha **Nomes e mensagens
  recentes** em *Contexto da conversa* (abaixo). Os áudios já transcritos
  continuam como estão.

### Adicionado

- **Contexto da conversa**, em *Configurações › Transcrição de áudio*: você
  escolhe o que vai junto com cada áudio para onde ele é transcrito, para
  acertar nomes e palavras incomuns. **Nada** (só o áudio), **Só os nomes** (o
  nome da conversa e os das pessoas nela, sem nenhuma mensagem) ou **Nomes e
  mensagens recentes** (também as últimas mensagens antes do áudio). O card diz
  o que vai e avisa quando esse texto sai do computador, e o `(?)` e a janela
  explicam cada opção. No Docker, `WHATSAPP_MCP_TRANSCRIPTION_CONTEXT` (`none`,
  `names` ou `messages`); no terminal, `whatsapp-mcp transcription context`. A
  ferramenta de IA vê o nível usado na resposta da transcrição e no status.
  Uma transcrição guardada não muda quando o nível muda; pedir para transcrever
  de novo (`refresh`) refaz com o nível novo.

### Mudado

- **Na OpenAI ou num servidor de transcrição, por padrão vai só o áudio.** Na
  1.9.5, o nome da conversa e as últimas mensagens antes de cada áudio iam
  junto para lá, sempre, e saíam do computador. Agora o padrão nesses dois
  casos é **Nada**; mandar os nomes ou as mensagens é uma escolha sua em
  *Contexto da conversa*, e ao trocar para a OpenAI ou para um servidor a
  escolha volta para Nada. Sem as mensagens, a transcrição pode errar mais um
  nome ou um termo que só a conversa explicava (a sua ferramenta de IA ainda
  confere a transcrição contra a conversa e corrige). Na transcrição neste
  computador, o padrão é **Só os nomes**, que não saem dele.
- **"Em outro computador" agora se chama "Em um servidor compatível com a
  OpenAI"**, em *Configurações › Transcrição de áudio › Onde transcrever*,
  porque vale qualquer servidor que fale a API de transcrição da OpenAI, em
  qualquer endereço, na internet ou na sua rede: você informa o endereço e a
  chave. Um Mac rodando `whatsapp-mcp transcription serve` continua sendo um
  exemplo. As opções de transcrição abrem numa janela, e o card se atualiza
  sem recarregar a página.
- **O card de versão**, em Configurações, mostra a versão instalada, o botão
  **Procurar atualização** e uma linha com o resultado. Quando sai uma versão
  nova, uma caixa verde mostra qual é, o link **Ver o que mudou** e como
  atualizar.
- **Webhooks mais fáceis de entender:** criar e editar usam a mesma janela,
  cada webhook diz em palavras quais eventos recebe, uma chave liga e desliga
  cada um, e apagar fica dentro da janela de edição, com confirmação. A chave
  de um webhook novo aparece com o passo a passo numerado e um botão
  **Finalizar**.
- **Logs num só botão:** ele abre uma janela para escolher o período (três
  opções), baixar o arquivo e, se precisar de ajuda, mandá-lo para
  contato@whatsappmcp.com.br.
- **Conexões com um botão "Gerenciar" por linha**, que reúne desconectar e
  cortar o acesso, cada um com confirmação.
- A porta do MCP e **Criar uma chave** abrem numa janela, e um erro aparece
  dentro dela. Uma chave nova aparece com os passos para usá-la.
- Trocar o idioma do painel não faz mais a página piscar.
- As janelas de confirmação antigas fecham com Esc e devolvem o foco ao botão
  que as abriu.
- As mensagens de erro do painel dizem o que aconteceu em palavras simples e o
  que fazer em seguida.
- Num servidor, o painel diz "neste servidor" onde antes dizia "neste
  computador".

### Corrigido

- **Um áudio em outra língua não sai mais traduzido pela conversa.** Na 1.9.5,
  as últimas mensagens da conversa iam sempre junto com o áudio, e na
  transcrição neste computador (ou num servidor feito com o mesmo motor) elas
  pesavam mais que a língua: um áudio em espanhol numa conversa em português
  saía em português. Agora, sem escolha sua, vão só os nomes quando a
  transcrição é neste computador, e o áudio sai na língua em que foi falado.
- **No Windows, a transcrição neste computador ainda não funciona em todos os
  computadores**, e agora o app diz isso. Ela precisa de um componente da
  Microsoft (o runtime do Visual C++) que um Windows novo não tem, e desde que
  a transcrição existe ela falhava sem avisar. Agora o app testa o programa de
  transcrição antes de baixar o modelo de 574 MB e, se ele não abre, o card
  diz **Indisponível** e aponta as outras opções (um servidor compatível com a
  OpenAI ou a OpenAI); as ferramentas de IA também recebem esse aviso. Quem
  instalar o runtime da Microsoft vê a transcrição voltar ao abrir as
  Configurações de novo. O conserto de vez está pedido na
  [issue #14](https://github.com/BrOrlandi/whatsapp-mcp-local/issues/14).
- **Áudios longos saem inteiros.** Na transcrição neste computador (e num
  servidor feito com o mesmo motor), trechos de um áudio longo se perdiam: de
  um áudio de 8 minutos, de um terço a metade sumia do texto, sem aviso. Agora
  o texto vem completo. Se os seus áudios vão para um Mac rodando
  `whatsapp-mcp transcription serve`, atualize a linha de comando desse Mac
  para a 1.9.6 (rode o `install.sh` de novo) e reinicie o servidor: é ele que
  transcreve.
- Um endereço errado em **Em um servidor compatível com a OpenAI** (um
  programa que não transcreve) é avisado, em vez de aparecer como pronto.
- Uma transcrição pedida enquanto o mesmo áudio já estava sendo transcrito
  volta com a data em que o texto foi guardado, a mesma que um pedido
  seguinte encontra.
- No Docker, `WHATSAPP_MCP_TRANSCRIPTION_WAIT` (quantos segundos uma chamada
  espera a transcrição antes de responder que ainda está em andamento) chega
  ao app; antes o `compose.yml` não a passava.
- O progresso da instalação da transcrição não mostra mais "NaN%".
- `whatsapp-mcp transcription serve --listen 127.0.0.1` mostra só o endereço
  em que ele atende, em vez dos endereços da rede.
- Num servidor, **Procurar atualização** e **Baixar os logs** não faziam nada
  no navegador; agora funcionam.

### Segurança

- **Com um token definido, nenhum programa do computador age pelo painel sem
  ele.** Antes, um programa podia se passar pelo navegador; agora o navegador
  pede o token uma vez e guarda uma sessão.
- O comando sugerido para baixar um arquivo recebido não executa mais nada
  vindo do nome do arquivo, que é escolhido por quem mandou a mensagem.
- Os programas da versão são compilados com o Go 1.26.11, com as correções de
  segurança mais recentes, e a publicação recusa um programa feito com outro.
- Os webhooks não alcançam os endereços internos de nuvem (de onde um servidor
  lê as próprias credenciais), e o teste de um webhook mostra um erro simples.
- Uma chave de API errada repetida não sobrecarrega mais o servidor.
- O painel avisa quando o endereço público do servidor começa com `http://`,
  por onde a senha, as chaves e as mensagens passariam sem criptografia.
- Num endereço `https`, o servidor pede ao navegador para usar sempre `https`
  (HSTS).
- O instalador da linha de comando (`install.sh`) instala a versão do wacli
  testada com o app e confere o sha256 dela e o do próprio WhatsApp MCP antes
  de instalar.
- No Docker, o app roda com o sistema só de leitura, sem privilégios do Linux
  e sem poder ganhar nenhum.

### Removido

- Um arquivo de 26 MB que tinha entrado no repositório por engano (um
  programa compilado, sem efeito no app).

Inclui o wacli 0.26.1.

## [1.9.5] - 2026-11-22

Primeira versão final desde a 1.8.0. Ela traz o **modo servidor**, testado nas
versões 1.9.0-beta.1 e 1.9.0-beta.2 (detalhes nas entradas delas, abaixo), e
três jeitos de transcrever os áudios. Quem usa o app no computador recebe esta
versão pelo atualizador, e nada muda para quem não liga o modo servidor nem
mexe na transcrição. A imagem Docker é
`ghcr.io/brorlandi/whatsapp-mcp-local:1.9.5`, e a partir desta versão também
`latest`; um servidor numa 1.9.0-beta atualiza trocando
`WHATSAPP_MCP_VERSION` no `.env` (ou apagando a linha, para seguir a `latest`)
e rodando `docker compose pull && docker compose up -d`.

### Adicionado

- **Modo servidor.** O WhatsApp MCP pode rodar num servidor ligado o tempo
  todo, com Docker (`install-docker.sh`) ou sem (`whatsapp-mcp server setup`),
  e atender num endereço `https` com senha no painel. Assim o seu WhatsApp
  funciona no Claude pelo navegador e pelo celular, no ChatGPT e em
  ferramentas de IA de outros computadores, por login ou por chave, com
  **Conexões** no painel para ver e cortar cada acesso.
- **Três jeitos de transcrever os áudios**, em *Configurações › Transcrição de
  áudio › Onde transcrever*:
  - **neste computador**, como antes;
  - **em um servidor compatível com a OpenAI**, em qualquer endereço, com o
    endereço e a chave dele. Um exemplo: um Mac com chip Apple rodando
    `whatsapp-mcp transcription serve` transcreve em segundos o que um servidor
    ou um mini PC leva minutos. Vale também o `whisper-server` do whisper.cpp;
  - **na OpenAI**, com a sua chave da API e o modelo que preferir
    (`whisper-1`, `gpt-4o-transcribe` ou `gpt-4o-mini-transcribe`). O áudio e
    as mensagens ao redor dele vão para a OpenAI.

  Se o servidor de transcrição ou a OpenAI não responder (o Mac dormiu, a
  internet caiu), o áudio é transcrito no próprio computador, quando a transcrição está
  instalada nele. O status e a checagem de saúde dizem onde os áudios estão
  sendo transcritos e se o destino está respondendo, e cada transcrição guarda
  qual motor a fez. Pelo terminal: `whatsapp-mcp transcription local`,
  `remote` e `openai`; no Docker, as variáveis
  `WHATSAPP_MCP_TRANSCRIPTION_MODE`, `_URL`, `_TOKEN`, `_MODEL` e
  `_OPENAI_KEY`.
- **O servidor de transcrição fala a API da OpenAI**
  (`POST /v1/audio/transcriptions`), então qualquer programa que use a OpenAI
  consegue transcrever no seu Mac.

### Corrigido

- Um áudio longo num computador lento não fica mais sem transcrição para
  sempre. Quando a ferramenta de IA cansava de esperar, a transcrição parava e
  a próxima tentativa recomeçava do zero. Agora ela continua sozinha e guarda o
  texto; pedir de novo pega o resultado ou espera a mesma transcrição, e depois
  de 4 minutos a resposta diz que ainda está em andamento e quando pedir de
  novo, em vez de estourar o tempo.

### Segurança

- A chave do servidor de transcrição e a chave da OpenAI nunca aparecem na
  tela, no log nem em argumento de linha de comando: o terminal pede a chave
  sem mostrá-la, e o servidor só diz em que arquivo ela está guardada.

Inclui o wacli 0.26.1.

## [1.9.0-beta.2] - 2026-11-20

Segunda versão de teste do modo servidor, só como imagem Docker:
`ghcr.io/brorlandi/whatsapp-mcp-local:1.9.0-beta.2`
(`WHATSAPP_MCP_VERSION=1.9.0-beta.2` no `.env`). A tag `latest` só passa a
existir na versão final. Para atualizar um servidor na 1.9.0-beta.1, troque a
versão no `.env` e rode `docker compose pull && docker compose up -d`; as
conversas, a conexão com o WhatsApp e as chaves continuam. O `compose.yml`
novo acrescenta só `hostname: whatsapp-mcp` ao app, para o painel mostrar o
endereço na rede interna com esse nome.

### Adicionado

- No servidor, **Configurações** mostra a versão instalada, avisa quando sai
  uma versão nova e diz como atualizar (o painel não atualiza sozinho), e
  mostra o endereço público e o da rede interna.

### Corrigido

- Quando o QR code expira ao conectar o WhatsApp, a tela diz isso e pede para
  tentar de novo, em vez de mostrar uma mensagem técnica em inglês.

Inclui o wacli 0.26.1.

## [1.9.0-beta.1] - 2026-11-17

Versão de teste do modo servidor, para quem foi convidado a testar. O app só
atualiza sozinho para versões finais, e quem instalar esta recebe a 1.9.0
final pelo atualizador. O uso com o Claude pelo navegador e pelo celular e com
o ChatGPT ainda não foi confirmado nesta versão de teste. A imagem Docker
desta versão é `ghcr.io/brorlandi/whatsapp-mcp-local:1.9.0-beta.1`
(`WAMCP_VERSION=1.9.0-beta.1` no instalador); a tag `latest` só passa a
existir na versão final.

Quem tem um servidor numa `2.0.0-beta` (o modo remoto de teste) troca a
imagem e mantém os dados: as chaves criadas lá continuam valendo no mesmo
endereço `/mcp`, e aparecem em **Conexões**. O controle de um servidor pelo
app do computador, que só existia nessas versões de teste, não existe mais.

### Adicionado

- **Modo servidor.** O WhatsApp MCP pode rodar num servidor ligado o tempo
  todo e atender num endereço `https`, onde o painel pede uma senha. Assim o
  seu WhatsApp funciona no Claude pelo navegador e pelo celular, no ChatGPT e
  em ferramentas de IA de outros computadores. Quem não liga o modo servidor
  não vê nenhuma diferença: o app e a linha de comando continuam iguais.
- **Instalação com Docker num comando** (`install-docker.sh`), com o `https`
  pelo Tailscale Funnel, por um túnel da Cloudflare, pelo Caddy ou por um
  proxy que você já tenha. A imagem é para servidores `amd64` e `arm64`. Sem
  Docker, `whatsapp-mcp server setup` faz o mesmo no Linux.
- **Duas formas de uma ferramenta entrar:** pelo login, em que você digita a
  senha do painel na página do seu servidor e escolhe o que ela pode fazer, ou
  com uma chave, para as ferramentas que não fazem login.
- **Conexões**, no painel: quem entrou, quando, e o que pode fazer, com um
  botão para cortar o acesso na hora. Um login ou uma chave pode ser *Só ler*.
- **`whatsapp-mcp bridge --url`** liga o Claude Desktop (e outras ferramentas
  que só iniciam um programa) a um servidor, sem o WhatsApp instalado no
  computador.
- Novos comandos: `password`, `apikey`, `server` e `healthcheck`.

### Segurança

- **Um valor que uma ferramenta de IA manda nunca vira opção do wacli.** Um
  número ou termo de busca escrito como opção de linha de comando (por
  exemplo, começando com `--`) podia ser lido pelo wacli como uma opção.
  Nomes de contatos e grupos continuam valendo como destinatário. Isso vale
  também para quem não usa o modo servidor.
- **O arquivo do serviço que guarda o token** (`whatsapp-mcp service install`
  com o token na variável de ambiente) agora só o seu usuário lê.

Inclui o wacli 0.26.1.

## [1.8.0] - 2026-11-17

### Adicionado

- **Aviso de volta online nos webhooks.** Um webhook pode pedir o evento
  `online`, mandado quando o WhatsApp volta a funcionar neste computador: o app
  abriu, o computador acordou ou a conexão voltou depois de pelo menos dois
  minutos fora. Ele chega depois que o histórico mandado pelo celular termina
  de chegar e diz desde quando o número estava fora. Contribuição de
  [@bragueloire](https://github.com/bragueloire).
- **Logs para suporte**, em Configurações: um botão baixa um arquivo de texto
  com o que o app fez nas últimas 24 horas, 3 dias ou 7 dias, com horário
  exato. Números, contatos, grupos e links viram códigos, e o texto das
  mensagens e os nomes não entram no arquivo. O app guarda os últimos 7 dias.

### Corrigido

- **Os webhooks que incluem as suas próprias mensagens recebem também o que a
  IA manda pelas ferramentas**: textos, respostas, reações, mídias,
  localizações, enquetes e encaminhamentos. Antes, só chegava o que era mandado
  pelo celular.
- **Cada envio devolve a conversa como as leituras a conhecem**, para a IA
  achar a mensagem que acabou de mandar ao ler a conversa em seguida.

### Mudou

- Quem estiver numa versão de teste recebe a versão final pelo atualizador.

Inclui o wacli 0.26.1, que mantém o histórico de chamadas ao juntar conversas e corrige falhas de segurança do Go.

## [1.7.6] - 2026-11-09

### Corrigido

- **Quem tem listas de transmissão não fica mais com as mensagens novas
  atrasadas.** Cada mensagem de uma lista de transmissão segurava a chegada das
  outras por mais de um minuto.
- **Responder uma conversa marca ela como lida**, como no celular.
- **Áudios `.m4a` enviados aparecem para quem recebe.** Antes, alguns
  aplicativos do WhatsApp escondiam esses áudios.
- **Anexos de mensagens de empresas** (documento, imagem ou vídeo no topo da
  mensagem) chegam inteiros e podem ser baixados.
- **As confirmações de entrega e leitura não se perdem** quando chega junto
  uma confirmação malformada.
- Correções de segurança nas bibliotecas de rede.

Inclui o wacli 0.26.0.

## [1.7.5] - 2026-11-03

### Mudou

- **Conectar uma ferramenta de IA, um passo por vez.** A tela de conectar
  mostra só o passo atual, com "Passo 1 de 3" e um botão para o próximo (e
  para voltar). Quem ficar travado pode abrir "Ver todos os passos" e ver tudo
  de uma vez, como antes.
- **"Abrir o WhatsApp MCP quando o computador ligar" aparece no fim da
  configuração**, no cartão "Tudo pronto", recomendado, em vez de embaixo do
  QR code. A opção continua em Configurações.

Inclui o wacli 0.25.1.

## [1.7.0] - 2026-11-03

### Novo

- **Tudo pelo terminal, sem tela.** A linha de comando faz o que o painel faz,
  para usar num servidor ou por SSH: `status` diz como as coisas estão e o que
  fazer a seguir, `pair` conecta o WhatsApp com um QR code desenhado no
  terminal (ou com `--phone`, um código para digitar no celular), e há
  `logout`, `port`, `token`, `webhooks`, `logs`, `update` e
  `service stop|start|restart`. Cada comando explica a si mesmo com `--help`;
  para scripts e agentes de IA, os principais respondem em JSON com `--json`,
  e o código de saída diz o que aconteceu.
- **Num servidor Linux, o serviço continua rodando depois que você sai do
  SSH**, e o `service install` não tenta abrir o navegador: ele indica o
  `whatsapp-mcp pair`.
- **`whatsapp-mcp update` atualiza a própria linha de comando** para a versão
  mais nova e reinicia o serviço.
- **Imagem Docker** e um arquivo do Docker Compose para rodar num servidor
  (`docs/servidor.md`).

### Corrigido

- Mudar a porta ou o token pela linha de comando agora reinicia o serviço no
  Linux também.
- O painel não mostra mais mensagens vazias que o wacli grava como
  marcadores.
- Quando o WhatsApp não está conectado, a dica que a IA recebe manda conectar
  pela tela Conectar do app, em vez de um comando interno do wacli (na linha
  de comando, com `whatsapp-mcp pair`).

Inclui o wacli 0.25.1.

## [1.6.2] - 2026-11-02

### Corrigido

- **A conversa com você mesmo mostra as suas mensagens, sem "(message)".** As
  mensagens internas que os aparelhos da sua conta trocam entre si ficam nessa
  conversa, e a IA recebia cada uma delas como uma mensagem vazia,
  "(message)", que escondia o que você escreveu. Ler a conversa, buscar e ver
  o contexto de uma mensagem agora deixam essas mensagens de fora.
- **Claude Desktop no Windows: configuração salva pelo Bloco de Notas ou pelo
  PowerShell.** Esses editores podem gravar o arquivo com uma marca invisível
  no começo, e o app o tomava por inválido. Agora ele lê e grava normalmente.
- **O app não fecha mais o Claude Desktop à toa.** Se a configuração do Claude
  não for um JSON válido, ele avisa qual arquivo corrigir antes de fechar o
  Claude, em vez de fechar e depois não conseguir gravar.

### Mudou

- **"Versão e atualizações" numa linha só.** A versão e o botão de procurar
  atualização ficam lado a lado, e a versão do wacli aparece, curta, no canto.

Inclui o wacli 0.25.1.

## [1.6.1] - 2026-11-02

### Corrigido

- **Claude Desktop no Windows volta a conectar.** As versões novas do Claude
  Desktop guardam a configuração em outra pasta
  (`%LOCALAPPDATA%\Claude-Data`), e o app gravava na antiga, que o Claude não
  lê mais. Agora o app grava em todas as pastas que o Claude usa ou já usou.
- **O botão "Adicionar ao Claude Desktop" fecha e abre o Claude sozinho.**
  Aberto, o Claude Desktop regrava o arquivo de configuração e apagava o
  WhatsApp poucos segundos depois. Agora o app fecha o Claude, grava, confere e
  abre de novo, no Windows e no Mac, e diz em que arquivo gravou.
- **O status não conta mais como "mensagem mais nova" o que não é mensagem.**
  Mensagens internas entre os aparelhos da sua conta, gravadas no pareamento,
  apareciam como a mais recente sem existir em nenhuma conversa.
- A tela de conectar não quebra mais o layout com caminhos longos, e o botão de
  copiar volta a aparecer.

### Mudou

- **Esperando a ferramenta conectar, a tela tem "Checar de novo" e "Pular".**
  Ela também continua checando enquanto estiver aberta, em vez de parar depois
  de dez minutos, e reconhece a aba Code do Claude Desktop.
- **Pedir para o próprio Claude configurar.** Em "Prefiro configurar de outro
  jeito", há um texto para colar no Claude, que testa a conexão e corrige a
  configuração.
- **No celular, o computador aparece como "WhatsApp MCP - Windows (nome do
  PC)"**, em vez de "wacli", nos números conectados a partir desta versão.
- Uma comemoração quando a ferramenta conecta.

Inclui o wacli 0.25.1.

## [1.6.0] - 2026-10-30

### Adicionado

- **A versão do wacli aparece em Configurações › Versão e atualizações.**

### Mudou

- **Envio de arquivo e áudio grandes não falha mais por tempo limite de 45 s.**
  O tempo agora acompanha o tamanho do arquivo.
- **O envio não duplica a mensagem quando a conexão cai no meio.**
- **Arquivar, fixar e marcar como lida não esperam mais o celular.** Antes,
  essas ações podiam levar até cinco minutos quando o celular não respondia.
- **Conversa arquivada volta a aparecer quando chega mensagem nova**, seguindo a
  opção "Manter conversas arquivadas" do WhatsApp.
- **Votos retirados de enquete não voltam mais** depois de reiniciar o app ou
  receber histórico antigo.
- **Prévias de link em alta resolução** por padrão.
- **No Windows, o app reconhece o novo canal de envio do wacli** (named pipe),
  com fallback para o socket anterior.
- **Pareamento que não completava em alguns servidores Linux ARM64.**
  Obrigado @mhdsilva (#10).
- Correções de segurança (Go 1.27.3, x/net).

### Atenção

Na primeira abertura depois de atualizar, o wacli refaz a sincronização de
arquivadas, fixadas e silenciadas. Se ela falhar, essas marcações podem sumir
até o WhatsApp ser aberto no celular. As mensagens não se perdem.

Inclui o wacli 0.25.1.

## [1.5.0] - 2026-10-14

### Adicionado

- **O app em espanhol e inglês, além do português.** O painel, a bandeja, as
  notificações e as mensagens do app aparecem em português, espanhol ou
  inglês. Numa instalação nova, o app segue o idioma do computador.
- **Escolha do idioma com bandeiras.** Na instalação, na tela do QR code e na
  seguinte, e em Configurações › Idioma, cada idioma tem um botão com a sua
  bandeira; ao passar o mouse, aparece o nome do idioma. A troca é na hora e
  não interrompe uma conexão em andamento. A opção "O do sistema" volta a
  seguir o idioma do computador.
- **O instalador do Windows em português, espanhol e inglês**, conforme o
  idioma do Windows. No Linux, o atalho do app também aparece nos três
  idiomas.
- **Na linha de comando,** o idioma do painel se escolhe em Configurações ou
  com `WHATSAPP_MCP_LANG` (`pt-BR`, `es` ou `en`).
- **O site e o README em espanhol e inglês**, com um seletor de idioma no topo.

### Mudou

- **Quem já usa o app continua em português** depois de atualizar, mesmo com o
  computador em inglês ou espanhol. Para mudar, use Configurações › Idioma.
- **O idioma vale só para a tela.** O que a sua ferramenta de IA recebe e os
  avisos dos webhooks, inclusive o aviso de teste, continuam iguais em
  qualquer idioma: as mensagens, os nomes e os valores chegam como sempre.

Inclui o wacli 0.20.0.

## [1.4.0] - 2026-10-10

### Adicionado

- **Um nome para o seu número.** Na aba WhatsApp, dê ao número conectado um
  nome curto, como Pessoal ou Trabalho. A sua ferramenta de IA vê esse nome ao
  usar o WhatsApp (a conexão aparece como "WhatsApp Trabalho" onde a ferramenta
  mostra o título), e ele vem junto quando ela pergunta pelo status.
- **Webhooks dizem de qual número veio o aviso.** Todo aviso traz `account`,
  com o nome que você deu ao número e o telefone dele.

### Mudou

- Por dentro, o app passou a tratar o número conectado como uma unidade
  completa, preparando o caminho para mais de um número no mesmo app. Para
  quem usa um número, nada muda: as mesmas pastas, o mesmo endereço do MCP e a
  mesma conexão nas ferramentas de IA.

Inclui o wacli 0.20.0.

## [1.3.1] - 2026-10-09

### Corrigido

- **Reações chegam a quem recebe.** Numa conversa individual, a reação da IA a
  uma mensagem que você recebeu era aceita pelo WhatsApp mas não aparecia para
  ninguém. Num grupo, reagir a uma mensagem sua dava erro ou também não
  aparecia. Agora a reação aparece nos dois casos, como quando você reage pelo
  celular.

### Mudou

- **Versão e atualizações, em Configurações, ficou mais direta.** Mostra só a
  versão instalada, com um selo ao lado: "Mais recente" quando você já está em
  dia, ou "Nova versão" em destaque quando há uma para instalar.

Inclui o wacli 0.20.0.

## [1.3.0] - 2026-10-07

### Adicionado

- **Responder citando e mencionar.** A IA responde a uma mensagem citando-a,
  como no celular, e menciona pessoas num grupo com @.
- **Mandar figurinhas.** A IA envia uma figurinha (imagem WebP, parada ou
  animada), inclusive uma que você já mandou ou recebeu numa conversa.
- **Rascunho antes de enviar.** Peça para ver a mensagem antes: a IA mostra o
  texto e para quem vai, sem enviar, e só envia depois que você aprovar.
- **Encaminhar, marcar como lida e "digitando…".** O encaminhamento chega como
  no celular, marcado como encaminhado, com a mídia junto.
- **Apagar só para mim**, além de apagar para todos. Os dois pedem confirmação.
- **Administrar grupos:** adicionar, remover e promover pessoas, trocar o nome
  e a descrição, pegar ou trocar o link de convite e sair do grupo. Remover
  alguém, trocar o link e sair pedem confirmação.
- **Quem está esperando sua resposta.** A IA lista as conversas em que a
  última mensagem é da outra pessoa, com há quanto tempo, ignorando um "ok" ou
  "obrigado" no fim, e opcionalmente os grupos em que te mencionaram. Também
  lista as conversas não lidas, como o celular mostra, e as mensagens em que
  te mencionaram.
- **Marcar como resolvido ou adiar.** O que você já resolveu sai da lista até
  alguém escrever de novo; adiar faz a conversa voltar na hora marcada. Fica só
  no seu computador: a outra pessoa não vê nada.
- **Contar e exportar mensagens.** A IA conta mensagens por conversa,
  pessoa, dia ou mês sem ler tudo, lê só o pedaço necessário de conversas
  longas, mostra o que veio antes e depois de uma mensagem e exporta conversas
  inteiras para um arquivo.
- **Conectar outras ferramentas de IA.** Cada uma tem o seu passo a passo:
  Claude Desktop, Claude Code, ChatGPT / Codex, Cursor ou outra. O Codex e o
  Cursor se configuram com um clique, o passo a passo termina sozinho quando a
  ferramenta conecta, e as conexões aparecem com o logo de cada uma.
- **Aba Ajuda**, com as perguntas mais comuns e exemplos do que pedir.
- **Tema claro, escuro ou o do sistema**, em Configurações.
- **Espaço dos arquivos baixados**, em Configurações: quanto ocupam as fotos,
  os áudios e os documentos que a sua ferramenta de IA abriu, um botão para
  apagar, e a opção de apagar sozinho os mais antigos. A IA também mede e apaga
  quando você pede.
- **Webhooks**, em Configurações: um script no seu computador (ou na rede)
  recebe cada mensagem nova e age por conta própria. Na tela você adiciona,
  testa, desliga e apaga cada um, e vê quando foi a última entrega; um webhook
  que para de responder é desligado sozinho. Uma página de documentação mostra
  o JSON de cada tipo de aviso e como conferir que ele veio do app. Também dá
  para configurar pela [API local](docs/api.md#webhooks). A IA sabe que eles
  existem e os sugere quando você pede para ser avisado de mensagens novas.

### Mudou

- **Configurações foi para a engrenagem**, no canto de cima, e reúne a porta,
  abrir e fechar, a transcrição de áudio, as atualizações, a aparência e os
  dados. As abas agora são Conectar MCP, WhatsApp, Status, Funções, Receitas e
  Ajuda.
- No Mac, o `.dmg` mostra para arrastar o app para Aplicativos.
- Arquivar, fixar e silenciar uma conversa não pausam mais o recebimento de
  mensagens.
- **Arquivos grandes chegam por link.** Fotos, áudios e documentos continuam
  chegando à IA inteiros, do jeito que vieram no WhatsApp; um arquivo de mais
  de 20 MB, ou um que a ferramenta não aceite, vem por um link deste
  computador.
- A transcrição de áudio, em Configurações, mostra só se está ativa; como usar
  foi para a Ajuda.

Inclui o wacli 0.20.0.

## [1.2.0] - 2026-10-06

### Adicionado

- **Instalação por uma IA.** O README traz um prompt para colar no Claude Code
  ou em outro agente que rode comandos no seu computador: ele baixa a versão
  certa para o seu sistema, instala, abre o app, te guia no QR code e confere
  se ficou tudo funcionando.
- **API local para agentes de IA.** Um agente no seu computador vê o estado do
  app e do WhatsApp, conecta o WhatsApp (por QR code ou por um código para
  digitar no celular), configura o Claude e abre a janela. Está descrita em
  [docs/api.md](docs/api.md).

### Mudou

- Pedir um código para digitar no celular troca o QR code que estava na tela.

Inclui o wacli 0.20.0.

## [1.1.1] - 2026-10-06

### Corrigido

- **A aba Estado não mostra mais um alerta falso quando o app abre.** Nos
  primeiros segundos, o WhatsApp já está conectado e recebendo enquanto o envio
  de mensagens fica pronto; isso aparecia como um "!" amarelo. Agora aparece
  como "Carregando", sem alerta.
- **O painel se atualiza sozinho.** O alerta da aba Estado e as páginas que
  mostram a conexão acompanham o WhatsApp em segundo plano, sem precisar trocar
  de página. Se você estiver com um diálogo aberto ou digitando, a página
  espera.

Inclui o wacli 0.20.0.

## [1.1.0] - 2026-10-06

### Mudou

- **O app continua rodando quando você fecha a janela, também pelo ⌘Q.** No
  Mac, ⌘Q e "Encerrar" no Dock só fecham a janela: o WhatsApp MCP segue na
  barra de menus e as ferramentas de IA continuam usando o WhatsApp.
- **Encerrar de vez pede confirmação.** "Encerrar o WhatsApp MCP…" fica no
  ícone da barra de menus (área de notificação no Windows), no menu do app e em
  Configurações, e avisa antes: o MCP é desligado e o computador para de
  receber mensagens enquanto o app estiver fechado.

### Corrigido

- No Mac, a janela não aparece mais por cima de outro app em tela cheia: abre
  numa mesa própria ou na própria tela cheia.

Inclui o wacli 0.20.0.

## [1.0.0] - 2026-10-05

A primeira versão pública do WhatsApp MCP Local.

### Adicionado

- **Transcrição de áudio sem instalar nada à parte.** Ao ligar a transcrição,
  o app baixa os programas certos para o seu sistema (macOS, Windows ou Linux)
  e confere cada um antes de usar. No Mac ela usa o Metal; no Windows com placa
  NVIDIA, a versão para CUDA.

### Mudou

- O projeto agora se chama **WhatsApp MCP Local** e mora em
  [BrOrlandi/whatsapp-mcp-local](https://github.com/BrOrlandi/whatsapp-mcp-local).
  Quem tem a 0.1.x recebe esta versão pela atualização automática.
- Linha de comando: os dados ficam em `~/.whatsapp-mcp` e o serviço se chama
  `whatsapp-mcp`. Quem usava o serviço da 0.1.x precisa removê-lo com a versão
  antiga e instalar de novo.

Inclui o wacli 0.20.0.

## [0.1.1] - 2026-10-05

### Adicionado

- **Aviso de versão nova em qualquer tela.** Quando sai uma versão, um aviso no
  canto inferior esquerdo oferece atualizar com um clique. O MCP fica fora do ar
  só pelos segundos em que o app reinicia.
- **Procurar atualização responde na hora:** o botão mostra que está
  procurando e diz se há uma versão nova ou se você já tem a mais recente.

### Mudou

- O app procura versões novas duas vezes por dia, em vez de uma.
- Enquanto o WhatsApp conecta, o app mostra "Carregando" e se atualiza sozinho
  até ficar conectado.
- O botão de conectar e a contagem de ferramentas falam em MCP, e a página
  Estado ficou sem termos técnicos.

### Corrigido

- No Windows, fechar o app nem sempre encerrava a conexão com o WhatsApp
  quando ele rodava sem janela de console.

Inclui o wacli 0.20.0.

## [0.1.0] - 2026-10-05

A primeira versão do WhatsApp MCP como app.

### Adicionado

- **Um app para macOS, Windows e Linux.** Baixe, abra, leia o QR code no
  celular e conecte o Claude, sem terminal e sem navegador.
- **Fica na barra de menus.** Fechar a janela não encerra o app, e o ícone
  mostra se o WhatsApp está conectado. O app abre sozinho quando o computador
  liga.
- **Conectar ferramentas de IA com um clique:** Claude Desktop (chat e Cowork) e
  Claude Code; para as outras que aceitam MCP, como o Codex, um texto pronto
  para colar.
- **Porta configurável.** Se outro programa já usar a 47821, o app avisa e
  troca por uma livre. O Claude Desktop acompanha a troca sozinho.
- **Atualizações.** O app procura uma versão nova uma vez por dia e se atualiza
  com um clique, conferindo o arquivo baixado antes de instalar.
- **Apagar todos os dados deste computador**, em Configurações, desconectando o
  dispositivo do WhatsApp.

### Mudou

- A transcrição de áudios, quando você pede, roda só no computador, com o
  Whisper large-v3-turbo. A opção de usar uma chave da OpenAI volta numa
  versão futura.
- A versão de linha de comando, para servidores e agentes de IA, agora se chama
  `whatsapp-mcp`.

Inclui o wacli 0.20.0.

[1.9.6]: https://github.com/BrOrlandi/whatsapp-mcp-local/releases/tag/v1.9.6
[1.9.5]: https://github.com/BrOrlandi/whatsapp-mcp-local/releases/tag/v1.9.5
[1.8.0]: https://github.com/BrOrlandi/whatsapp-mcp-local/releases/tag/v1.8.0
[1.7.6]: https://github.com/BrOrlandi/whatsapp-mcp-local/releases/tag/v1.7.6
[1.7.5]: https://github.com/BrOrlandi/whatsapp-mcp-local/releases/tag/v1.7.5
[1.7.0]: https://github.com/BrOrlandi/whatsapp-mcp-local/releases/tag/v1.7.0
[1.6.2]: https://github.com/BrOrlandi/whatsapp-mcp-local/releases/tag/v1.6.2
[1.6.1]: https://github.com/BrOrlandi/whatsapp-mcp-local/releases/tag/v1.6.1
[1.6.0]: https://github.com/BrOrlandi/whatsapp-mcp-local/releases/tag/v1.6.0
[1.5.0]: https://github.com/BrOrlandi/whatsapp-mcp-local/releases/tag/v1.5.0
[1.4.0]: https://github.com/BrOrlandi/whatsapp-mcp-local/releases/tag/v1.4.0
[1.3.1]: https://github.com/BrOrlandi/whatsapp-mcp-local/releases/tag/v1.3.1
[1.3.0]: https://github.com/BrOrlandi/whatsapp-mcp-local/releases/tag/v1.3.0
[1.2.0]: https://github.com/BrOrlandi/whatsapp-mcp-local/releases/tag/v1.2.0
[1.1.1]: https://github.com/BrOrlandi/whatsapp-mcp-local/releases/tag/v1.1.1
[1.1.0]: https://github.com/BrOrlandi/whatsapp-mcp-local/releases/tag/v1.1.0
[1.0.0]: https://github.com/BrOrlandi/whatsapp-mcp-local/releases/tag/v1.0.0
[0.1.1]: https://github.com/BrOrlandi/whatsapp-mcp-local/releases/tag/v0.1.1
[0.1.0]: https://github.com/BrOrlandi/whatsapp-mcp-local/releases/tag/v0.1.0
