#!/usr/bin/env bash
# Installs WhatsApp MCP v2 on this computer and opens its panel in the browser.
#
#   curl -fsSL https://raw.githubusercontent.com/BrOrlandi/whatsapp-mcp-v2/main/install.sh | bash
#
# or, from a clone of the repository: ./install.sh
#
# It installs wacli (the WhatsApp client) and whatsapp-mcp-v2 into
# ~/.local/bin, registers the daemon as a login service, and opens
# http://127.0.0.1:47821/, where WhatsApp is connected by QR code. It never
# needs sudo, and running it again updates both in place.
set -euo pipefail

REPO="BrOrlandi/whatsapp-mcp-v2"
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
install_wacli() {
  if command -v wacli >/dev/null 2>&1; then
    say "wacli already installed: $(wacli --version 2>/dev/null | head -1)"
    return
  fi
  if command -v brew >/dev/null 2>&1; then
    say "installing wacli with Homebrew"
    brew install openclaw/tap/wacli
    return
  fi
  say "installing wacli from its GitHub release"
  local tag asset
  tag="$(curl -fsSL https://api.github.com/repos/openclaw/wacli/releases/latest | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)"
  [ -n "$tag" ] || die "could not find the latest wacli release"
  asset="wacli_${tag#v}_${OS}_${ARCH}.tar.gz"
  curl -fsSL -o "$TMP/wacli.tgz" "https://github.com/openclaw/wacli/releases/download/$tag/$asset"
  tar -xzf "$TMP/wacli.tgz" -C "$TMP"
  install -m 0755 "$(find "$TMP" -type f -name wacli | head -1)" "$BIN_DIR/wacli"
}

# ---- whatsapp-mcp-v2 ---------------------------------------------------------
ensure_go() {
  command -v go >/dev/null 2>&1 && return
  if command -v brew >/dev/null 2>&1; then
    say "installing Go with Homebrew (needed to build from source)"
    brew install go
    return
  fi
  die "Go is needed to build whatsapp-mcp-v2 from source: install it from https://go.dev/dl/ and run this again"
}

build_from() {
  local src="$1"
  ensure_go
  say "building whatsapp-mcp-v2 from source"
  local version
  version="$(git -C "$src" describe --tags --always --dirty 2>/dev/null || echo dev)"
  (cd "$src" && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$TMP/whatsapp-mcp-v2" ./cmd/whatsapp-mcp-v2)
}

install_mcp() {
  local here=""
  if [ -n "${BASH_SOURCE[0]:-}" ] && [ -f "${BASH_SOURCE[0]}" ]; then
    here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
  fi
  if [ -n "$here" ] && [ -f "$here/go.mod" ] && grep -q "module github.com/$REPO" "$here/go.mod"; then
    build_from "$here"
  elif curl -fsSL -o "$TMP/mcp.tgz" "https://github.com/$REPO/releases/latest/download/whatsapp-mcp-v2_${OS}_${ARCH}.tar.gz" 2>/dev/null; then
    say "downloading the latest release"
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
  install -m 0755 "$TMP/whatsapp-mcp-v2" "$BIN_DIR/whatsapp-mcp-v2"
  if [ "$OS" = darwin ]; then
    # A binary built or downloaded here is not notarised; clear the
    # quarantine flag so launchd can start it.
    xattr -d com.apple.quarantine "$BIN_DIR/whatsapp-mcp-v2" 2>/dev/null || true
  fi
  say "installed $BIN_DIR/whatsapp-mcp-v2 ($("$BIN_DIR/whatsapp-mcp-v2" version))"
}

install_wacli
install_mcp

case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) warn "$BIN_DIR is not on your PATH; add it to use whatsapp-mcp-v2 from a terminal" ;;
esac

WACLI_BIN="$(command -v wacli || echo "$BIN_DIR/wacli")"
say "starting the service"
WACLI_BIN="$WACLI_BIN" WHATSAPP_MCP_PORT="$PORT" "$BIN_DIR/whatsapp-mcp-v2" service install

cat <<EOF

  Pronto. O painel abriu no navegador: http://127.0.0.1:$PORT/

  1. Clique em "Conectar WhatsApp" e escaneie o QR code pelo celular
     (WhatsApp > Dispositivos conectados > Conectar dispositivo).
  2. Em "Conectar ao Claude", clique para adicionar ao Claude Code e/ou ao Claude Desktop.

  Para abrir o painel de novo: whatsapp-mcp-v2 open

EOF
