#!/usr/bin/env bash
# Serves the panel against fabricated data, so its screens can be worked on
# without pairing a phone. Nothing here talks to WhatsApp.
#
#   scripts/preview.sh            first run: the QR code screen
#   PREVIEW_PAIRED=1 scripts/preview.sh   already paired, with seeded chats
#
# While the QR code shows, `touch $DIR/store/SCAN` pretends the phone scanned it.
set -euo pipefail
cd "$(dirname "$0")/.."
DIR="${PREVIEW_DIR:-$(mktemp -d /tmp/wamcp-preview.XXXX)}"
mkdir -p "$DIR/store" "$DIR/data" "$DIR/home"
command -v wacli >/dev/null || { echo "the preview needs the real wacli for reads: brew install openclaw/tap/wacli" >&2; exit 1; }
WACLI_STORE_DIR="$DIR/store" wacli --json doctor >/dev/null
sqlite3 "$DIR/store/wacli.db" < scripts/preview/seed.sql
[ "${PREVIEW_PAIRED:-0}" = 1 ] && touch "$DIR/store/PAIRED"
go build -o "$DIR/whatsapp-mcp" ./cmd/whatsapp-mcp
echo "preview in $DIR — touch $DIR/store/SCAN to pretend the QR code was scanned"
HOME="$DIR/home" WACLI_BIN="$PWD/scripts/preview/fake-wacli" WACLI_STORE_DIR="$DIR/store" \
  WHATSAPP_MCP_DATA="$DIR/data" WHATSAPP_MCP_PORT="${PREVIEW_PORT:-47890}" exec "$DIR/whatsapp-mcp" serve
