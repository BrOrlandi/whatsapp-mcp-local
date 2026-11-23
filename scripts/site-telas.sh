#!/usr/bin/env bash
# Captures the panel's screens for the landing page (site/assets/telas/<lang>/),
# and saves the panel's recipes and tool list (scripts/site-dados/) for
# scripts/site-paginas.mjs, which writes the site's recipe and tool pages. All
# in the light theme and in the three languages, from this checkout, against the
# fake WhatsApp of scripts/preview (seed.sql): no real account, number or name.
#
#   scripts/site-telas.sh                  build ./cmd/whatsapp-mcp and capture everything
#   WMCP_BIN=/path/whatsapp-mcp scripts/site-telas.sh   reuse a binary already built
#   SITE_TELAS_LANGS="pt-BR" scripts/site-telas.sh      only some languages
#   REAL_WACLI=/path/wacli scripts/site-telas.sh        when the wacli on PATH is not the app's
#
# It runs its own instances in a temporary folder, on their own ports, and never
# touches an installed app or another preview. The browser is the browser-proxy
# (a Chromium on another machine, reached at localhost:9223 through its tunnel):
# it cannot open this Mac's 127.0.0.1, so for the length of the capture a small
# forwarder in site-telas.mjs answers on this Mac's network address and passes
# each request to the instance as if it came from 127.0.0.1. Everything stops
# when the script ends, also on an error or Ctrl+C.
set -euo pipefail
cd "$(dirname "$0")/.."
REPO="$PWD"

PROXY="${BROWSER_PROXY:-http://localhost:9223}"
curl -fsS -m 5 "$PROXY/proxy/estado" >/dev/null || {
  echo "the browser-proxy does not answer at $PROXY (is its tunnel up?)" >&2
  exit 1
}
# The real wacli answers the reads, and the Settings screen prints its version:
# it must be the one the app ships (build/wacli.env). REAL_WACLI points at it
# when the one on PATH is another.
. build/wacli.env
REAL_WACLI="${REAL_WACLI:-$(command -v wacli || true)}"
[ -x "$REAL_WACLI" ] || { echo "needs the real wacli $WACLI_VERSION for reads: set REAL_WACLI" >&2; exit 1; }
HAVE="$("$REAL_WACLI" --version 2>/dev/null | awk '{print $NF}' | tr -d v)"
[ "$HAVE" = "$WACLI_VERSION" ] || { echo "$REAL_WACLI is wacli $HAVE, the app ships $WACLI_VERSION: set REAL_WACLI to that one" >&2; exit 1; }
command -v cwebp >/dev/null || { echo "needs cwebp: brew install webp" >&2; exit 1; }

# The address the remote browser reaches this Mac by: the interface of the default route.
IFACE="$(route -n get default 2>/dev/null | awk '/interface:/{print $2}')"
LAN_IP="${SITE_TELAS_IP:-$(ipconfig getifaddr "${IFACE:-en0}" || true)}"
[ -n "$LAN_IP" ] || { echo "could not find this computer's network address; set SITE_TELAS_IP" >&2; exit 1; }

# A short neutral path: the connect screens print the HOME of the instance
# (configuration paths), and this one says nothing about whoever runs it.
W="$(mktemp -d /tmp/wamcp-telas.XXXX)"
PIDS=()
cleanup() {
  for p in "${PIDS[@]+"${PIDS[@]}"}"; do kill "$p" 2>/dev/null || true; done
  wait 2>/dev/null || true
  rm -rf "$W"
}
trap cleanup EXIT INT TERM

BIN="${WMCP_BIN:-}"
if [ -z "$BIN" ]; then
  echo "building ./cmd/whatsapp-mcp"
  # The version the screens print in their footer: the newest in the changelog,
  # or the next patch when the changelog's top entry is still unreleased.
  VERSION="${SITE_TELAS_VERSION:-}"
  if [ -z "$VERSION" ]; then
    VERSION="$(grep -m1 -oE '^## \[[0-9]+\.[0-9]+\.[0-9]+\]' CHANGELOG.md | tr -d '#[] ')"
    if [ "$(grep -m1 -E '^## \[' CHANGELOG.md | grep -cE '[0-9]+\.[0-9]+\.[0-9]+')" = 0 ]; then
      VERSION="${VERSION%.*}.$(( ${VERSION##*.} + 1 ))"
    fi
  fi
  go build -ldflags "-X main.version=$VERSION" -o "$W/whatsapp-mcp" ./cmd/whatsapp-mcp
  BIN="$W/whatsapp-mcp"
fi

LOCAL_PORT=47911
SERVER_PORT=47912
PUBLIC_PORT=47913
# The server mode's panel password: random, typed only by the capture into the
# instance's sign-in page, never printed, thrown away with the instance.
PASSWORD="$(openssl rand -hex 16)"

# $1 = folder, $2 = language, $3 = paired (1) or showing the QR code (0)
prepare() {
  mkdir -p "$1/store" "$1/data" "$1/home"
  WACLI_STORE_DIR="$1/store" HOME="$1/home" "$REAL_WACLI" --json doctor >/dev/null
  sqlite3 "$1/store/wacli.db" < scripts/preview/seed.sql
  [ "$3" = 1 ] && touch "$1/store/PAIRED"
  # Claude Desktop and Cursor "installed" in the instance's HOME, so the connect
  # screens show their normal state rather than a download link.
  mkdir -p "$1/home/Library/Application Support/Claude" "$1/home/.cursor"
  echo '{"mcpServers":{}}' > "$1/home/Library/Application Support/Claude/claude_desktop_config.json"
  # Stand-ins for the claude and codex commands, found where the panel looks:
  # they only answer --version, so the screens show the normal "add" button and
  # nothing is ever configured.
  mkdir -p "$1/home/.local/bin"
  for c in claude codex; do
    printf '#!/bin/sh\n[ "$1" = --version ] && { echo "1.0.0"; exit 0; }\nexit 1\n' > "$1/home/.local/bin/$c"
    chmod +x "$1/home/.local/bin/$c"
  done
  echo "{\"autostart\":false,\"seen\":[\"first-run\"],\"language\":\"$2\"}" > "$1/data/config.json"
}

instance_env() {
  printf '%s\n' "HOME=$1/home" "WACLI_BIN=$REPO/scripts/preview/fake-wacli" "REAL_WACLI=$REAL_WACLI" \
    "WACLI_STORE_DIR=$1/store" "WHATSAPP_MCP_DATA=$1/data" "PREVIEW_QR_SECONDS=30" \
    "PATH=$1/home/.local/bin:/usr/bin:/bin:/usr/sbin:/sbin:/opt/homebrew/bin"
}

wait_up() {
  for _ in $(seq 1 60); do curl -fsS -m 2 -o /dev/null "$1" 2>/dev/null && return 0; sleep 0.5; done
  echo "the instance at $1 did not start" >&2
  return 1
}

for LANG_CODE in ${SITE_TELAS_LANGS:-pt-BR es en}; do
  echo "== $LANG_CODE"
  L="$W/$LANG_CODE"
  prepare "$L/local" "$LANG_CODE" 0
  prepare "$L/server" "$LANG_CODE" 1
  HASH="$(printf '%s\n' "$PASSWORD" | env -i $(instance_env "$L/server") "$BIN" password hash)"

  (cd "$L/local" && exec env -i $(instance_env "$L/local") "WHATSAPP_MCP_PORT=$LOCAL_PORT" \
    "WHATSAPP_MCP_LANG=$LANG_CODE" "$BIN" serve >"$L/local.log" 2>&1 </dev/null) &
  PIDS+=($!)
  (cd "$L/server" && exec env -i $(instance_env "$L/server") "WHATSAPP_MCP_PORT=$SERVER_PORT" \
    "WHATSAPP_MCP_PUBLIC_URL=http://localhost:$PUBLIC_PORT" "WHATSAPP_MCP_PUBLIC_LISTEN=127.0.0.1:$PUBLIC_PORT" \
    "WHATSAPP_MCP_PANEL_PASSWORD_HASH=$HASH" "WHATSAPP_MCP_LANG=$LANG_CODE" \
    "$BIN" serve >"$L/server.log" 2>&1 </dev/null) &
  PIDS+=($!)
  wait_up "http://127.0.0.1:$LOCAL_PORT/api/status"
  wait_up "http://127.0.0.1:$SERVER_PORT/api/status"

  OUT="site/assets/telas/$(echo "$LANG_CODE" | cut -c1-2)"
  mkdir -p "$OUT"
  # Two connections for the Conexões page, as someone would name them.
  case "$LANG_CODE" in
    es) K1="n8n de la oficina"; K2="Informe semanal" ;;
    en) K1="Office n8n"; K2="Weekly report" ;;
    *) K1="n8n do escritório"; K2="Relatório semanal" ;;
  esac
  env -i $(instance_env "$L/server") "WHATSAPP_MCP_PORT=$SERVER_PORT" "$BIN" apikey create --name "$K1" >/dev/null
  env -i $(instance_env "$L/server") "WHATSAPP_MCP_PORT=$SERVER_PORT" "$BIN" apikey create --name "$K2" --read-only >/dev/null

  SITE_TELAS_PASSWORD="$PASSWORD" node scripts/site-telas.mjs --proxy "$PROXY" --ip "$LAN_IP" --lang "$LANG_CODE" --out "$W/png-$LANG_CODE" \
    --local "127.0.0.1:$LOCAL_PORT" --server "127.0.0.1:$SERVER_PORT" --public "localhost:$PUBLIC_PORT" --scan "$L/local/store/SCAN" --data scripts/site-dados --bin "$BIN" --home "$L/local/home" \
    ${SITE_TELAS_ONLY:+--only "$SITE_TELAS_ONLY"}

  for png in "$W/png-$LANG_CODE"/*.png; do
    cwebp -quiet -q 82 -m 6 "$png" -o "$OUT/$(basename "${png%.png}").webp"
  done
  for p in "${PIDS[@]}"; do kill "$p" 2>/dev/null || true; done
  wait 2>/dev/null || true
  PIDS=()
done
du -sh site/assets/telas
# The recipe and tool pages carry the panel's words: write them again.
node scripts/site-paginas.mjs
