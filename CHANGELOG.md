# Changelog

O que muda para quem usa o WhatsApp MCP, versão a versão. Formato baseado no
[Keep a Changelog](https://keepachangelog.com/pt-BR/1.1.0/), com versões em
[SemVer](https://semver.org/lang/pt-BR/).

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

[0.1.0]: https://github.com/BrOrlandi/whatsapp-mcp-v2/releases/tag/v0.1.0
