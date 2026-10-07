# Plano: instalar o app pelo Homebrew

> Status: planejado em outubro de 2026, ainda não executado. As decisões em
> aberto estão na seção 7.

## 0. Resumo

**Objetivo.** Instalar o app do macOS com um comando:

```sh
brew install --cask brorlandi/tap/whatsapp-mcp
```

Vale sobretudo para quem já usa o terminal e para o prompt do README que pede a
uma IA para instalar: um comando no lugar de baixar o `.dmg`, montar, copiar e
desmontar. A página de download continua sendo o caminho principal para quem
não é técnico.

**Por que um tap próprio.** O homebrew-cask oficial exige notabilidade: 30
forks, 30 watchers ou 75 estrelas, e o triplo (90, 90 ou 225) quando o próprio
autor submete. Em outubro de 2026 o repositório tinha 3 forks, 1 watcher e 9
estrelas. Um tap próprio não tem essa exigência, e dá para migrar para o
oficial depois. O app já é assinado e notarizado, como o Homebrew passou a
exigir dos casks.

## 1. O tap

Um repositório público `BrOrlandi/homebrew-tap`, com:

- `Casks/whatsapp-mcp.rb`, o cask (seção 2);
- um `README.md` curto, com o comando de instalação e um link para o projeto.

## 2. O cask

```ruby
cask "whatsapp-mcp" do
  version "1.2.0"
  sha256 "c9ce263e49e58d0baebda434881ee4b033de8f5198038d9867fcfde5231a0c10"

  url "https://github.com/BrOrlandi/whatsapp-mcp-local/releases/download/v#{version}/WhatsApp-MCP.dmg"
  name "WhatsApp MCP"
  desc "Connect WhatsApp to Claude and other AI tools over MCP, on your own computer"
  homepage "https://whatsapp-mcp.brorlandi.xyz/"

  livecheck do
    url :url
    strategy :github_latest
  end

  auto_updates true
  depends_on macos: ">= :big_sur"

  app "WhatsApp MCP.app"

  uninstall quit: "com.brorlandi.whatsapp-mcp"

  zap trash: [
    "~/Library/Application Support/WhatsApp MCP",
    "~/Library/Caches/com.brorlandi.whatsapp-mcp",
    "~/Library/Logs/WhatsApp MCP",
    "~/Library/WebKit/com.brorlandi.whatsapp-mcp",
  ]
end
```

- **`auto_updates true`:** o app se atualiza sozinho (`internal/updater`), então
  o `brew upgrade` deixa a atualização com ele, e os dois não brigam. Depois de
  uma atualização feita pelo app, o brew continua registrando a versão antiga.
  Isso é esperado e não atrapalha o `brew uninstall`.
- **`sha256`:** é o do `.dmg` da 1.2.0 com o fundo de "Arraste para instalar",
  que substituiu o original na release em 6 de outubro de 2026.
- **`depends_on`:** vem do `LSMinimumSystemVersion` 11.0 em
  `build/darwin/Info.plist`.
- **`uninstall quit`:** encerra o app pelo bundle id antes de removê-lo. O item
  de login fica registrado no sistema, mas sem efeito, porque o app não existe
  mais.
- **`zap`:** só roda com `brew uninstall --zap`, e apaga a conta conectada, o
  histórico e os logs. Um `brew uninstall` normal remove só o app. Os caminhos
  vêm de `internal/platform/paths_darwin.go` e do que o WebView cria com o
  bundle id.
- **A linha de comando** e o serviço dela (`com.brorlandi.whatsapp-mcp`, em
  `~/Library/LaunchAgents`) não fazem parte do cask.

## 3. Validar antes de anunciar

1. `brew style` e `brew audit --cask --strict --online brorlandi/tap/whatsapp-mcp`.
2. Instalar num Mac que já tem o app em `/Applications`, instalado pelo `.dmg`:
   `brew install --cask --force brorlandi/tap/whatsapp-mcp`. Os dados ficam em
   `~/Library`, então nada se perde.
3. Abrir o app e conferir que `http://127.0.0.1:47821/api/status` responde.
4. `brew uninstall --cask whatsapp-mcp` (sem `--zap`), conferir que os dados
   continuam em `~/Library/Application Support/WhatsApp MCP`, e instalar de novo.

## 4. O tap no processo de release

Um script novo, `scripts/homebrew.sh X.Y.Z`, roda depois do `gh release create`,
porque a URL do `.dmg` precisa existir. Ele:

1. lê o sha256 do `WhatsApp-MCP.dmg` em `dist/vX.Y.Z/checksums.txt`;
2. clona o tap numa pasta temporária e troca `version` e `sha256` no cask;
3. roda `brew style` no cask;
4. faz o commit "whatsapp-mcp X.Y.Z" e o push no `main` do tap.

O mesmo passo entra em:

- `docs/desenvolvimento.md#publicar`, logo depois do `gh release create`;
- a seção "Release & Changelog" do `CLAUDE.md`;
- `.claude/release.json`, como um canal `homebrew-tap` em `publish`.

Se uma release trocar o `.dmg` no lugar, como aconteceu com a 1.2.0, o script
roda de novo com a mesma versão, só para atualizar o `sha256`.

## 5. O README

- **"Como instalar", no item do macOS:** acrescentar "ou, com Homebrew:
  `brew install --cask brorlandi/tap/whatsapp-mcp`".
- **Prompt da IA, passo 2 do macOS:** se `brew --version` responder, instalar
  com `brew install --cask brorlandi/tap/whatsapp-mcp`, com `--force` quando já
  houver um `WhatsApp MCP.app` em `/Applications` instalado pelo `.dmg`, e abrir
  com `open -a "WhatsApp MCP"`. Sem o brew, segue o caminho do `.dmg` de hoje.
- **"No dia a dia":** dizer que, instalado pelo brew, o app continua se
  atualizando sozinho e o `brew upgrade` não é necessário.

## 6. Fora deste plano

- **Um formula da linha de comando.** A linha de comando procura o `wacli` no
  PATH, então o formula dependeria de um `wacli` também no brew. Vale olhar se
  existe um tap do `wacli` antes de prometer isso.
- **O homebrew-cask oficial,** enquanto o projeto não tiver a notabilidade da
  seção 0.
- **O Linux.** O Homebrew no Linux não tem casks.

## 7. Decisões em aberto

Cada item traz a recomendação primeiro.

1. **Nome do tap:** `BrOrlandi/homebrew-tap`, genérico, que serve para outros
   projetos. A alternativa é `BrOrlandi/homebrew-whatsapp-mcp`.
2. **Nome do cask:** `whatsapp-mcp`, que o Homebrew deriva do nome do app. A
   alternativa é `whatsapp-mcp-local`, que repete o nome do produto.
3. **Atualização do tap a cada release:** o script faz push direto no `main` do
   tap, sem abrir PR.
4. **Página de download:** pôr o comando do brew também no cartão do macOS em
   `site/download/index.html`. Isso muda `site/` e publica o site no push.
