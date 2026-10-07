# Plano: biblioteca de figurinhas

> Status: para o futuro, nada implementado. Nasceu de uma resposta no X em
> outubro de 2026 ("duvido a IA saber usar figurinhas do jeito que eu uso").
> O envio já existe: `send_media_message` com `type: "sticker"` manda um WebP
> como figurinha (`wacli send sticker`, pelo socket do sync, sem pausá-lo).

## 0. Resumo

**Objetivo.** O app guarda as figurinhas da pessoa, mostra todas numa tela,
cada uma com um código, e a IA envia uma figurinha específica pelo código, sem
precisar achar o arquivo nem saber onde ele está.

**Hoje.** A IA só manda uma figurinha se tiver o arquivo: acha uma mensagem com
`media_type: "sticker"`, baixa com `download_media` e passa o caminho ao
`send_media_message`. Funciona, mas custa várias chamadas e não conhece as
favoritas.

## 1. De onde vêm as figurinhas

- **Das conversas.** As enviadas e recebidas já estão no banco do wacli
  (`messages.media_type = 'sticker'`), e o `download_media` baixa o WebP. Mídia
  antiga pode ter expirado no WhatsApp; aí o arquivo só existe se já tiver sido
  baixado.
- **Das favoritas.** O WhatsApp sincroniza as favoritas com os dispositivos
  conectados, junto com as configurações da conta (app state). A whatsmeow
  entende o registro (`StickerAction`, com `isFavorite` e o que é preciso para
  baixar o arquivo), mas o wacli 0.20 não guarda nem tem comando para isso. Os
  caminhos: uma contribuição ao wacli (`wacli stickers favorites`) ou capturar
  nós. A sincronização dessas configurações às vezes falha
  (`app_state_full_sync_failed` no status), o que vale testar antes.

## 2. O que o app guarda

- Uma tabela no `state.db`, uma linha por figurinha: o código, o caminho do
  arquivo, se é animada, de onde veio (conversa e mensagem, ou favorita),
  quantas vezes a pessoa a enviou e quando foi a última.
- **O código é o sha256 do WebP** (encurtado), para a mesma figurinha recebida
  em vinte conversas virar uma só e o código não mudar entre computadores.
- O arquivo numa pasta do app (`<DataDir>/stickers`), fora da limpeza
  automática dos arquivos baixados, para não sumir.

## 3. A tela

- Uma grade com todas as figurinhas, com filtros (favoritas, as que eu mais
  mando, recentes) e o código de cada uma, fácil de copiar.
- Esconder uma figurinha da IA, sem apagar.
- Linguagem simples: "código da figurinha", sem falar em hash nem em app
  state; o detalhe vai num (?).

## 4. As ferramentas

- `list_stickers`: o código, se é animada, quantas vezes a pessoa mandou e de
  onde veio, com filtros iguais aos da tela.
- `send_media_message` aceita `sticker_id` no lugar de `url` quando
  `type: "sticker"`.

## 5. Pontos em aberto

- Entram só as figurinhas que a pessoa enviou (o "jeito dela") ou também as
  recebidas?
- Favoritas: esperar o wacli ou implementar a captura aqui?
- Como a IA sabe o que cada figurinha mostra: olhar a imagem quando precisa
  (o `read_media` já lê WebP, inclusive o primeiro quadro das animadas) ou uma
  legenda que a pessoa escreve na tela.
