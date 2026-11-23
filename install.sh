#!/usr/bin/env bash
# Installs WhatsApp MCP's command line on this computer and opens its panel in
# the browser. For a computer with a screen, the desktop app is simpler: see
# the README. This is for servers, computers that stay on, and AI agents.
#
#   curl -fsSL https://raw.githubusercontent.com/BrOrlandi/whatsapp-mcp-local/main/install.sh | bash
#
# or, from a clone of the repository: ./install.sh
#
# It installs wacli (the WhatsApp client) and whatsapp-mcp into
# ~/.local/bin, registers the daemon as a login service, and opens
# http://127.0.0.1:47821/, where WhatsApp is connected by QR code. It never
# needs sudo, and running it again updates both in place.
set -euo pipefail

REPO="BrOrlandi/whatsapp-mcp-local"
BIN_DIR="${WHATSAPP_MCP_BIN_DIR:-$HOME/.local/bin}"
PORT="${WHATSAPP_MCP_PORT:-47821}"

say()  { printf '\033[1;32m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m!!\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31mxx\033[0m %s\n' "$*" >&2; exit 1; }

case "$(uname -s)" in
  Darwin) OS=darwin ;;
  Linux)  OS=linux ;;
  *) die "this installer supports macOS and Linux; on Windows use WSL" ;;
esac
case "$(uname -m)" in
  arm64|aarch64) ARCH=arm64 ;;
  x86_64|amd64)  ARCH=amd64 ;;
  *) die "unsupported processor $(uname -m)" ;;
esac

mkdir -p "$BIN_DIR"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# ---- wacli -------------------------------------------------------------------
# The wacli this release is tested with, and the sha256 of each system's file
# from its checksums.txt: the same pin as build/wacli.env, which a test keeps
# in step with these lines.
WACLI_VERSION=0.26.1
WACLI_SHA256_darwin_arm64=20898523a70fb3ad992f8a20d786f70484bb53221df017cdafdcb7f7fc882d8f
WACLI_SHA256_darwin_amd64=d5561a41534ec83be8bca611d1fcef12bbfb9967a3661dcc7d0328b929224a7d
WACLI_SHA256_linux_amd64=515405c5a33c7f651a368484740db662736f50c275be5a0504fb0a84dce47013
WACLI_SHA256_linux_arm64=9efe789b01e12462140089749d8b5ce0ad292521893f4166e13eca81f0acac4c

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then sha256sum "$1" | cut -d' ' -f1; else shasum -a 256 "$1" | cut -d' ' -f1; fi
}

install_wacli() {
  if command -v wacli >/dev/null 2>&1; then
    say "wacli already installed: $(wacli --version 2>/dev/null | head -1)"
    return
  fi
  say "installing wacli $WACLI_VERSION from its GitHub release"
  local asset pin want got
  asset="wacli_${WACLI_VERSION}_${OS}_${ARCH}.tar.gz"
  pin="WACLI_SHA256_${OS}_${ARCH}"
  want="${!pin:-}"
  [ -n "$want" ] || die "no checked wacli for $OS/$ARCH"
  curl -fsSL -o "$TMP/wacli.tgz" "https://github.com/openclaw/wacli/releases/download/v$WACLI_VERSION/$asset"
  got="$(sha256_of "$TMP/wacli.tgz")"
  [ "$got" = "$want" ] || die "the wacli download does not match its checksum (got $got, want $want)"
  tar -xzf "$TMP/wacli.tgz" -C "$TMP"
  install -m 0755 "$(find "$TMP" -type f -name wacli | head -1)" "$BIN_DIR/wacli"
}

# ---- whatsapp-mcp ------------------------------------------------------------
ensure_go() {
  command -v go >/dev/null 2>&1 && return
  if command -v brew >/dev/null 2>&1; then
    say "installing Go with Homebrew (needed to build from source)"
    brew install go
    return
  fi
  die "Go is needed to build whatsapp-mcp from source: install it from https://go.dev/dl/ and run this again"
}

build_from() {
  local src="$1"
  ensure_go
  say "building whatsapp-mcp from source"
  local version
  version="$(git -C "$src" describe --tags --always --dirty 2>/dev/null || echo dev)"
  (cd "$src" && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$TMP/whatsapp-mcp" ./cmd/whatsapp-mcp)
}

install_mcp() {
  local here=""
  if [ -n "${BASH_SOURCE[0]:-}" ] && [ -f "${BASH_SOURCE[0]}" ]; then
    here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  fi
  # The latest release's tag, resolved once, so the file and its checksums
  # come from the same release even if a new one is published meanwhile.
  local tag=""
  if [ -z "$here" ] || [ ! -f "$here/go.mod" ]; then
    tag="$(curl -fsSLI -o /dev/null -w '%{url_effective}' "https://github.com/$REPO/releases/latest" 2>/dev/null || true)"
    case "$tag" in */releases/tag/*) tag="${tag##*/releases/tag/}" ;; *) tag="" ;; esac
  fi
  if [ -n "$here" ] && [ -f "$here/go.mod" ] && grep -q "module github.com/$REPO" "$here/go.mod"; then
    build_from "$here"
  elif [ -n "$tag" ] && curl -fsSL -o "$TMP/mcp.tgz" "https://github.com/$REPO/releases/download/$tag/whatsapp-mcp_${OS}_${ARCH}.tar.gz" 2>/dev/null; then
    say "downloading the latest release ($tag)"
    # Checked against the release's own checksums.txt before it is unpacked.
    curl -fsSL -o "$TMP/checksums.txt" "https://github.com/$REPO/releases/download/$tag/checksums.txt" ||
      die "could not download the release's checksums.txt"
    local want got
    want="$(awk -v f="whatsapp-mcp_${OS}_${ARCH}.tar.gz" '$2 == f {print $1}' "$TMP/checksums.txt")"
    got="$(sha256_of "$TMP/mcp.tgz")"
    [ -n "$want" ] && [ "$got" = "$want" ] || die "the whatsapp-mcp download does not match the release's checksums.txt"
    tar -xzf "$TMP/mcp.tgz" -C "$TMP"
  else
    say "no published release to download; fetching the source"
    if command -v gh >/dev/null 2>&1 && gh auth status >/dev/null 2>&1; then
      gh repo clone "$REPO" "$TMP/src" -- --depth 1 >/dev/null
    else
      git clone --depth 1 "https://github.com/$REPO.git" "$TMP/src" >/dev/null
    fi
    build_from "$TMP/src"
  fi
  install -m 0755 "$TMP/whatsapp-mcp" "$BIN_DIR/whatsapp-mcp"
  if [ "$OS" = darwin ]; then
    # A binary built or downloaded here is not notarised; clear the
    # quarantine flag so launchd can start it.
    xattr -d com.apple.quarantine "$BIN_DIR/whatsapp-mcp" 2>/dev/null || true
  fi
  say "installed $BIN_DIR/whatsapp-mcp ($("$BIN_DIR/whatsapp-mcp" version))"
}

install_wacli
install_mcp

case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) warn "$BIN_DIR is not on your PATH; add it to use whatsapp-mcp from a terminal" ;;
esac

# Voice notes are transcribed on the computer itself on Apple Silicon:
# whisper.cpp and a ~600 MB model, set up once. Not fatal: the panel can set
# it up later, and OpenAI remains an option.
if [ "$OS" = darwin ] && [ "$ARCH" = arm64 ]; then
  say "setting up local voice-note transcription (whisper.cpp, about 600 MB)"
  "$BIN_DIR/whatsapp-mcp" transcription install || warn "local transcription was not set up; the panel's Configurações › Transcrição de áudio can do it later"
fi

WACLI_BIN="$(command -v wacli || echo "$BIN_DIR/wacli")"
say "starting the service"
# The port is passed on only when given: running this again keeps one
# changed with `whatsapp-mcp port`.
if [ -n "${WHATSAPP_MCP_PORT:-}" ]; then
  WACLI_BIN="$WACLI_BIN" "$BIN_DIR/whatsapp-mcp" service install
else
  env -u WHATSAPP_MCP_PORT WACLI_BIN="$WACLI_BIN" "$BIN_DIR/whatsapp-mcp" service install
fi
PORT="$("$BIN_DIR/whatsapp-mcp" port 2>/dev/null || echo "$PORT")"

# A server has no screen to open the panel on: WhatsApp is connected from
# the terminal instead (docs/servidor.md).
if [ -n "${SSH_CONNECTION:-}" ] || { [ "$OS" = linux ] && [ -z "${DISPLAY:-}" ] && [ -z "${WAYLAND_DISPLAY:-}" ]; }; then
  cat <<EOF

  Pronto. O serviço está rodando em http://127.0.0.1:$PORT/

  1. Conecte o WhatsApp por aqui: whatsapp-mcp pair
     (o QR code aparece no terminal; com --phone, um código para digitar
     no celular).
  2. Confira com: whatsapp-mcp status
  3. Configure as ferramentas de IA: whatsapp-mcp config

EOF
  exit 0
fi

cat <<EOF

  Pronto. O painel abriu no navegador: http://127.0.0.1:$PORT/

  1. Escaneie o QR code que aparece lá pelo celular
     (WhatsApp > Dispositivos conectados > Conectar dispositivo).
  2. Confira se as suas conversas recentes batem com as do celular.
  3. Conecte o Claude Desktop ou o Claude Code com um clique.

  Para abrir o painel de novo: whatsapp-mcp open
  Sem navegador, pelo terminal: whatsapp-mcp pair, whatsapp-mcp status

EOF
