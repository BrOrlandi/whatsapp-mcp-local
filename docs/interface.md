# Interface do painel

As decisões de desenho do painel (o app no computador e a versão do servidor
usam o mesmo). Leia antes de mudar qualquer tela. Cada regra traz o porquê e
um exemplo do próprio painel; os nomes de classe estão em
`internal/panel/style.go`, os templates em `internal/panel/templates.go` e os
scripts em `internal/panel/assets/`. O que o código ainda não segue está na
lista do fim: lá, a regra vale, e o código é que muda.

## Texto

- **Linguagem de quem não é técnico.** Nada de endereços de API, nomes de
  ferramentas do MCP, jargão ou explicação de como funciona por dentro no texto
  visível. O detalhe técnico vai num `(?)`, o `tip`. Quem abre o painel quer
  ligar o WhatsApp à IA, não entender a arquitetura. Exemplo: a introdução dos
  Webhooks diz "Avisam um programa seu a cada mensagem nova…", e o POST, a
  assinatura e as 10 tentativas ficam no `tip` ao lado.
- **As peças internas não têm nome.** Uma mensagem nunca cita o wacli nem
  outra dependência. Um erro diz o que aconteceu e o que clicar em seguida:
  "O QR code expirou. Clique em Tentar de novo para gerar outro." Um erro
  desconhecido vira uma frase genérica ("Não foi possível conectar agora.
  Clique em Tentar de novo."), e o texto original vai só para o log. Modelo:
  `pairingError` em `internal/panel/panel.go`.
- **A boa notícia é dita.** Depois de uma verificação que não encontra nada, a
  tela diz que está tudo bem; um espaço vazio não é estado. Exemplo: "Você já
  tem a versão mais recente.", na `update-status--ok` do card de versão.
- **pt-BR é a língua de origem.** O texto em português é a chave, e todo texto
  tem tradução em `es.json` e `en.json`; chave que o código não usa mais sai do
  catálogo. O `go test ./internal/i18n` cobra as três coisas. Como funciona:
  [Idiomas](desenvolvimento.md#idiomas).

## Cor

- **Nada de azul.** Avisos e destaques usam o dourado que combina com o verde
  (`--accent`, `--accent-soft`, `pill--accent`); estados bons usam os verdes
  (`--ok`, `--ok-bg`, `--ok-border`, `--brand`); o vermelho (`--danger`,
  `--danger-bg`, `--danger-border`) é só para ação destrutiva e erro. A paleta
  é a da marca, e o azul brigava com ela. Cor nova sai desses tokens do
  `:root`, nunca de um hex solto.
- **Claro, escuro e celular.** Tudo tem de ser legível nos dois temas (os
  tokens já trocam em `darkPalette`) e na largura de um celular (os `@media
  (max-width:520px)` e `460px` do `style.go`).

## Cards

Um card (`card`, `card__head`, `card__title`, `card__body`) tem esta ordem:

1. **No alto, o fato principal e no máximo uma ação principal ao lado.** Fatos
   secundários ficam discretos (`muted`). Exemplo: o card de versão
   (`updatecard`) mostra "Versão instalada" com o número grande
   (`version__number`) e, ao lado, só "Procurar atualização".
2. **Logo abaixo, uma linha de estado** para o momento atual: procurando, em
   dia, erro (`update-status`, com `--ok` e `--error`).
3. **O que pede a atenção da pessoa vem numa caixa separada, abaixo, de fundo
   verde, com tudo sobre o assunto, em ordem** (`update-found`): o título que
   diz o que aconteceu ("Uma nova versão foi encontrada",
   `update-found__title`), o nome da coisa ("WhatsApp MCP 1.8.0",
   `update-found__name`), um botão para saber mais ("Ver o que mudou", que abre
   as notas da versão pelo site, `brand.ChangelogURL`) e a ação, ou o comando com "Copiar" quando é
   um comando (`update-found__how`). Ação relacionada nunca fica solta ao lado
   de botões que não têm nada a ver com ela.

E também:

- **Informação só de leitura que a pessoa precisa copiar ou conferir ganha o
  seu próprio card, como valor, não como campo editável.** Exemplo: "Endereços
  do servidor" (`#enderecos`), com o endereço público e o interno em `snippet`
  com `data-copy`.
- **Card não tem formulário embutido.** Formulário abre num modal (abaixo).
- **O app e o servidor usam a mesma estrutura de card; só a ação muda.** O
  mesmo `updatecard` serve aos dois: no computador, o botão "Instalar e
  reiniciar"; no servidor, o comando em `update-found__how`.

## Avisos no canto

Uma novidade (versão nova) aparece num aviso no canto, em todas as páginas
(`update-banner`), que leva ao card que a explica (`/configuracoes#atualizacoes`).
O card é onde ficam o detalhe e a ação. O aviso pode ser dispensado
(`update-banner__close`).

## Adicionar e editar: modais

- **Adicionar é um botão no card que abre um modal com o formulário.** O botão
  do estado vazio abre o mesmo modal. Exemplo: "Adicionar webhook" e
  "Configurar o primeiro webhook" (no `empty`) abrem `#webhook-dialog`.
- **Editar abre o mesmo modal, preenchido, com título e botão próprios:**
  "Editar webhook" e "Salvar".
- **Um grupo de opções tem um título que diz o que se escolhe, depois uma
  linha de dica, depois as opções.** Exemplo: "Eventos" (`field__label`),
  "Avisar quando" (`field__hint`) e os `check`, num `role="group"`.
- **Erros de validação e do servidor aparecem dentro do modal** (o `alert` de
  `data-webhook-error`), não na página atrás dele.
- **Esc, × e Cancelar fecham o modal, e o foco volta para o que o abriu.**
  Enquanto ele está aberto, o Tab não sai dele. Modelo: a função `modal()` de
  `settings.js`, que o modal de webhooks e o dos logs usam. Um modal com um
  formulário comum, que vai para o servidor, só precisa de `data-modal` no
  `overlay` e de × e Cancelar apontando para o card (`href="#id-do-card"`,
  nunca `#`, que sobe a página): `settings.js` (carregado em todas as
  páginas) dá a ele o mesmo teclado e devolve a página para onde estava.
  Modelos: "Gerenciar" em Suas conexões, "Trocar a porta".
- **Escolher e baixar também é um modal:** o card tem um só botão com contorno
  ("Baixar os logs"); o modal tem as opções em `check` de rádio, com título e
  dica, e um "Baixar" cheio. O formulário é um GET comum, para baixar mesmo
  sem script. Depois de baixar, o modal fica aberto com uma linha de
  confirmação, e Cancelar vira Fechar. Modelo: `#logs-dialog`.
- **Ação destrutiva fica dentro do modal de edição**, como botão vermelho
  (`btn--danger`), separada de Salvar e Cancelar (`dialog__actions` a põe
  sozinha à esquerda), e pede confirmação (`confirm`: "Apagar este webhook?",
  "Não apagar", "Apagar mesmo"). Nunca como mais um botão na linha. Num
  modal `data-modal`, o botão vermelho leva `data-confirm="#id"`, o id da
  caixa `confirm` (escondida) que pergunta; o "Não…" dela tem
  `data-confirm-no`. Sem script, o botão age direto: o modal foi o passo que
  perguntou.

Peças: `overlay`, `dialog`, `dialog__head`, `dialog__close`, `dialog__body`,
`dialog__actions`, `field`, `field__label`, `field__hint`, `check`,
`check__hint`, `confirm`.

## Linhas e botões

- **A linha mostra a configuração do item em palavras, sem abrir nada.**
  Exemplo: "Eventos: Mensagens · Reações · inclui as suas", no `row__meta`.
- **Nunca uma fileira de botões iguais.** A aparência de cada controle
  corresponde ao tipo e ao peso dele:
  - ligar e desligar é um interruptor com o estado escrito ao lado, "Ligado" ou
    "Desligado" (`switch`, `switch__track`, com `aria-checked`);
  - a ação principal da linha é o único botão com contorno ("Editar",
    `btn--ghost btn--small`);
  - ação ocasional é discreta ("Testar", `btn--quiet btn--small`);
  - ação destrutiva é vermelha e fica no modal.
- **Item desligado parece desligado de relance**, apagado, mas ainda mostra o
  que é (`hook--off` deixa o título em `--muted`).
- **O botão cheio (`btn`) é para o passo que termina um fluxo:** "Salvar",
  "Adicionar webhook" no modal, "Finalizar", "Ir para o painel".

## Depois de criar algo que mostra um segredo

Uma caixa (`secret`) com passos numerados (`secret__steps`, `secret__step`):

1. **"Guarde a chave"**: a chave (`secret__value`), "Copiar a chave" e o aviso
   de que ela não aparece de novo.
2. **"Veja a documentação"**: uma frase sobre o porquê e o botão, que abre numa
   aba nova para a chave continuar na tela.

Depois, um "Finalizar" cheio (`btn`). Enquanto a caixa está aberta, o botão
"Adicionar…" some; ele volta quando a caixa fecha. Modelo: `data-webhook-secret`.

## Movimento

Mudar uma configuração não pode fazer a página pular ou piscar: o que a pessoa
clicou fica no mesmo lugar da tela, e o que carrega depois não o empurra.
Exemplos: o card Aparência é marcado antes da primeira pintura
(`[data-themes]`), para não surgir depois e empurrar o resto; ao trocar o
idioma, `theme.js` segura o botão escolhido na mesma altura enquanto a página
nova se monta; números que chegam depois ocupam o lugar com "…" ou `skeleton`.
Transições são curtas (0,12 a 0,2 s) e respeitam `prefers-reduced-motion`.

## Peças para reaproveitar

| Para | Use |
|---|---|
| Um bloco da página | `card`, `card__head`, `card__title` (com `{{icon}}`), `card__body`; `stack` espaça os filhos |
| Ação principal que termina um fluxo | `btn` |
| A ação principal de um card ou linha | `btn btn--ghost btn--small` |
| Ação ocasional, discreta | `btn btn--quiet btn--small` |
| Ação destrutiva (dentro de modal) | `btn btn--danger` |
| Estado curto ao lado de um título | `pill` com `--ok`, `--warn`, `--off`, `--accent`; `pill--new` para novidade |
| Detalhe técnico ou explicação | `tip` (`tip__body`); `tip--quiet` para o que quase ninguém precisa |
| Modal | `overlay`, `dialog`, `dialog__head`, `dialog__close`, `dialog__body`, `dialog__actions` |
| Confirmar algo que não se desfaz | `confirm`, `confirm__title`, `confirm__text` |
| Campo de formulário | `field`, `field__label`, `field__hint` |
| Opção marcável | `check`, `check__hint` |
| Ligar e desligar um item | `switch`, `switch__track` |
| Lista de itens | `rows`, `row`, `row__main`, `row__title`, `row__meta` |
| Lista vazia | `empty`, `empty__title`, com o botão que abre o modal de adicionar |
| Valor para copiar | `snippet` com `<pre data-copy>`; o `app.js` põe o botão "Copiar" |
| Texto ou endereço para copiar (não é comando nem chave) | `<pre class="plain" data-copy>`, caixa clara; o bloco escuro fica só para comandos e chaves. E-mail: `snippet--mail`, com a cara de um `fact` |
| Segredo recém-criado | `secret`, `secret__steps`, `secret__step`, `secret__value` |
| Estado de uma verificação | `update-status` (`--ok`, `--error`) |
| Algo que pede atenção, com a ação | `update-found` (`__title`, `__name`, `__note`, `__how`) |
| Novidade em todas as páginas | `update-banner` |
| Resultado de uma ação, com spinner | `busy` (`role="status"`), `spinner` |
| Números lado a lado | `facts`, `fact` |
| Erro / sucesso numa página | `alert`, `alert--ok` |

## Convenções que já existiam

- **O ícone da barra de menus do macOS é monocromático**, uma imagem template
  que o sistema pinta como os outros ícones; o estado aparece pela forma (um
  ponto, o glifo apagado). No Windows e no Linux o ícone tem cor.
  `templateIcon` em `cmd/app/tray.go`.
- **Sem travessão no texto que a pessoa lê:** ponto, dois-pontos ou vírgula.
- **Ao dar exemplos de ferramentas de IA,** "Claude, Codex e outras", sem
  destacar uma; onde o MCP entra, ele é nomeado ("Conectar ferramenta de IA ao
  MCP").
- **Endereços só onde a pessoa precisa copiá-los** (Configurações, conectar
  uma ferramenta, Endereços do servidor). A página Funções é a exceção que
  existe para listar as funções pelo nome.
- **A aba só ganha marca quando algo está errado** (`nav__alert`): uma marca
  sempre verde vira decoração e ninguém repara quando muda.
- **Os modais abrem pelo endereço** (`#id`, `:target`): o Voltar do navegador
  fecha, e a atualização periódica da página espera enquanto um está aberto.
- **Ação de conta que não se desfaz** ("Desconectar este computador do
  WhatsApp", "Apagar todos os dados") abre um modal de confirmação que diz o
  que se perde e o que fica; "Apagar tudo" pede para digitar uma palavra.

## Como conferir

Toda tela é conferida no app do computador e na versão do servidor, nos dois
temas e na largura de um celular. `scripts/preview.sh` sobe o painel da linha
de comando com um WhatsApp falso, e o app roda com dados de exemplo sem tocar
nos reais ([painel no navegador](desenvolvimento.md#painel-no-navegador-com-dados-de-exemplo),
[app com dados de exemplo](desenvolvimento.md#rodar-o-app-com-dados-de-exemplo)).

## Antes de entregar uma tela

- [ ] Uma pessoa que não é técnica entende cada frase? O detalhe técnico foi
      para um `(?)`?
- [ ] Nenhuma mensagem cita o wacli ou outra peça interna; todo erro diz o que
      clicar, e o desconhecido é genérico, com o original no log.
- [ ] O estado "está tudo bem" é dito com uma frase.
- [ ] Todo texto novo tem es e en, e não sobrou chave sem uso
      (`go test ./internal/i18n`).
- [ ] Nenhum azul; vermelho só em erro e ação destrutiva.
- [ ] Legível em claro, escuro e na largura do celular.
- [ ] O card tem o fato e uma ação no alto, a linha de estado e, se for o
      caso, a caixa verde com tudo sobre o assunto.
- [ ] Adicionar e editar usam o mesmo modal; Esc, × e Cancelar fecham; o foco
      volta; o erro aparece dentro dele.
- [ ] Apagar está no modal, em vermelho, separado e com confirmação.
- [ ] Na linha: a configuração em palavras, um interruptor com o estado
      escrito, um único botão com contorno, o resto discreto.
- [ ] Nada pula nem pisca ao mudar uma configuração; com
      `prefers-reduced-motion`, nada anima.
- [ ] Conferido no app e no servidor.

## Ainda não segue

O que hoje contraria as regras acima. Quem mexer nessas telas corrige.

- **O wacli na linha da versão:** o card de versão mostra "wacli 0.x" ao lado
  da versão instalada.
