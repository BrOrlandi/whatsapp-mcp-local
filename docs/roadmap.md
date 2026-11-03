# Roadmap

O que está na fila para depois, sem data. Cada item com plano próprio aponta
para ele; os demais são só a ideia, registrada para não se perder.

## Com plano

- **Segurança das ferramentas**: a pessoa escolhe quais conversas a IA vê e
  que tipo de ação ela pode fazer. [Plano](plano-seguranca.md).
- **Biblioteca de figurinhas**: o app guarda as figurinhas, mostra todas numa
  tela e a IA envia uma pelo código. [Plano](plano-figurinhas.md).
- **Instalar pelo Homebrew** (`brew install --cask`). [Plano](plano-homebrew.md).
- **Vários números de WhatsApp no mesmo app**, como recurso pago: uma
  conexão MCP por número, e quem tem um número só não percebe nada.
  [Plano](plano-multiplas-contas.md).

## A revisar

- **Onde roda a transcrição dos áudios.** Hoje ela é conferida só no Mac.
  Falta:
  - Testar o whisper.cpp no Windows e no Linux. O Handy, app em que a
    transcrição se baseou, funciona bem em todos.
  - Medir quanto CPU e tempo um áudio custa. A tela de Transcrição deve dizer,
    com um teste rápido no próprio computador, se ele dá conta, antes de
    ligar.
  - Escolher onde transcrever quando o WhatsApp MCP roda num servidor (2.0),
    numa configuração do app:
    - no servidor;
    - no app do computador da pessoa, que está conectado ao servidor e
      transcreve com o motor local dele;
    - em nenhum dos dois: a ferramenta entrega o áudio, e o agente de IA que
      consome o MCP transcreve.

    Nem todo servidor compensa rodar o Whisper.
  - Na versão local, continua transcrevendo no próprio computador, depois de
    passar no teste de capacidade.

  O fluxo de hoje está em [transcricao.md](transcricao.md).
