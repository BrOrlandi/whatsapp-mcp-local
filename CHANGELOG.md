# Changelog

O que muda para quem usa o WhatsApp MCP, versão a versão. Formato baseado no
[Keep a Changelog](https://keepachangelog.com/pt-BR/1.1.0/), com versões em
[SemVer](https://semver.org/lang/pt-BR/).

## [1.3.1] - 2026-10-09

### Corrigido

- **Reações chegam a quem recebe.** Numa conversa individual, a reação da IA a
  uma mensagem que você recebeu era aceita pelo WhatsApp mas não aparecia para
  ninguém. Num grupo, reagir a uma mensagem sua dava erro ou também não
  aparecia. Agora a reação aparece nos dois casos, como quando você reage pelo
  celular.

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

[1.3.1]: https://github.com/BrOrlandi/whatsapp-mcp-local/releases/tag/v1.3.1
[1.3.0]: https://github.com/BrOrlandi/whatsapp-mcp-local/releases/tag/v1.3.0
[1.2.0]: https://github.com/BrOrlandi/whatsapp-mcp-local/releases/tag/v1.2.0
[1.1.1]: https://github.com/BrOrlandi/whatsapp-mcp-local/releases/tag/v1.1.1
[1.1.0]: https://github.com/BrOrlandi/whatsapp-mcp-local/releases/tag/v1.1.0
[1.0.0]: https://github.com/BrOrlandi/whatsapp-mcp-local/releases/tag/v1.0.0
[0.1.1]: https://github.com/BrOrlandi/whatsapp-mcp-local/releases/tag/v0.1.1
[0.1.0]: https://github.com/BrOrlandi/whatsapp-mcp-local/releases/tag/v0.1.0
