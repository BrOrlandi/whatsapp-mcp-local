#!/usr/bin/env bash
# Integration tests: builds the macOS app from this checkout, installs it on
# the test Mac, and runs the suite in integration/ against it, through SSH
# tunnels. Run it before every release (docs/desenvolvimento.md#publicar).
#
#   scripts/integration.sh                 build, install and test
#   scripts/integration.sh --no-build      install the app already in build/bin
#   scripts/integration.sh --no-install    test the app installed on the test Mac as it is
#   scripts/integration.sh -run 'TestApp/messaging'   extra arguments go to go test
#
# Settings come from the environment or from .env.integration at the root
# (ignored by git; never commit addresses or numbers, the repository is public):
#
#   TEST_MAC     user@host of the test Mac, paired with the test WhatsApp account
#   WMCP_TO      number (with country code) or JID the test messages go to; they
#                are deleted for everyone at the end. Without it, nothing is sent.
#   WMCP_GROUP   optional group JID the test account administers, made for the
#                tests: it gets renamed and its description changed and restored
#   WMCP_TOKEN   the app's WHATSAPP_MCP_TOKEN, if it has one
#
# docs/testes-integracao.md explains what it covers and how to prepare the Mac.
set -euo pipefail
cd "$(dirname "$0")/.."

build=1 install=1
args=()
for a in "$@"; do
  case "$a" in
    --no-build) build=0 ;;
    --no-install) build=0 install=0 ;;
    *) args+=("$a") ;;
  esac
done

if [ -f .env.integration ]; then set -a; . ./.env.integration; set +a; fi
: "${TEST_MAC:?set TEST_MAC (user@host of the test Mac) in .env.integration}"
APP="/Applications/WhatsApp MCP.app"
LOCAL_PORT="${WMCP_LOCAL_PORT:-57821}"   # the app, seen from here
HOOK_LOCAL="${WMCP_HOOK_LOCAL:-57822}"   # the webhook receiver here
HOOK_REMOTE="${WMCP_HOOK_REMOTE:-57823}" # the same receiver, seen from the test Mac
logdir="build/integration"; mkdir -p "$logdir"
ssh_() { ssh -o BatchMode=yes -o ConnectTimeout=10 "$TEST_MAC" "$@"; }

ssh_ true || { echo "the test Mac ($TEST_MAC) does not answer over ssh" >&2; exit 1; }
# Keep it awake for the run and a while after. Never kill it at the end: the
# test Mac sleeps then and stops answering ssh until someone wakes it.
ssh_ 'nohup caffeinate -dimsu -t 7200 >/dev/null 2>&1 &'

if [ "$build" = 1 ]; then
  VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null | sed 's/^v//')-it}"
  echo "==> building $VERSION"
  ARCHS=arm64 VERSION="$VERSION" scripts/build-macos.sh >"$logdir/build.log" 2>&1 \
    || { tail -20 "$logdir/build.log"; exit 1; }
fi
if [ "$install" = 1 ]; then
  [ -d "build/bin/WhatsApp MCP.app" ] || { echo "no build/bin/WhatsApp MCP.app to install" >&2; exit 1; }
  VERSION="${VERSION:-$(defaults read "$PWD/build/bin/WhatsApp MCP.app/Contents/Info" CFBundleShortVersionString 2>/dev/null || true)}"
  echo "==> installing on the test Mac"
  ssh_ 'pkill -TERM -x "WhatsApp MCP"; for i in $(seq 1 20); do pgrep -x "WhatsApp MCP" >/dev/null || break; sleep 1; done
        pkill -KILL -x "WhatsApp MCP" 2>/dev/null; rm -rf "'"$APP"'"'
  tar -C build/bin -cf - "WhatsApp MCP.app" | ssh_ 'tar -C /Applications -xf - && open -a "'"$APP"'"'
fi

port=$(ssh_ 'f="$HOME/Library/Application Support/WhatsApp MCP/config.json"; [ -f "$f" ] && /usr/bin/plutil -extract port raw -o - "$f" 2>/dev/null || echo 47821')
echo "==> waiting for the app on the test Mac (port $port) to be ready"
status=""
for _ in $(seq 1 60); do
  status=$(ssh_ "curl -s -m3 http://127.0.0.1:$port/api/status" || true)
  [ "$(printf '%s' "$status" | jq -r '.next // empty' 2>/dev/null)" = ready ] && break
  sleep 3
done
printf '%s' "$status" | jq -c '{version, next, whatsapp: .whatsapp.state}' 2>/dev/null || echo "$status"
[ "$install" = 1 ] || VERSION=$(printf '%s' "$status" | jq -r '.version // empty')

echo "==> tunnels: app on 127.0.0.1:$LOCAL_PORT, webhooks back to 127.0.0.1:$HOOK_LOCAL"
ssh -o BatchMode=yes -o ExitOnForwardFailure=yes -N \
  -L "$LOCAL_PORT:127.0.0.1:$port" -R "$HOOK_REMOTE:127.0.0.1:$HOOK_LOCAL" "$TEST_MAC" &
tunnel=$!
trap 'kill $tunnel 2>/dev/null || true' EXIT
for _ in $(seq 1 20); do curl -s -m2 "http://127.0.0.1:$LOCAL_PORT/healthz" >/dev/null && break; sleep 0.5; done

log="$logdir/$(date +%Y%m%d-%H%M%S).log"
echo "==> go test (log in $log)"
set +e
WMCP_URL="http://127.0.0.1:$LOCAL_PORT" WMCP_HOOK_LISTEN="127.0.0.1:$HOOK_LOCAL" \
  WMCP_HOOK_URL="http://127.0.0.1:$HOOK_REMOTE" WMCP_VERSION="${VERSION:-}" \
  TEST_MAC="$TEST_MAC" WMCP_TO="${WMCP_TO:-}" WMCP_GROUP="${WMCP_GROUP:-}" WMCP_TOKEN="${WMCP_TOKEN:-}" \
  go test -tags integration -count=1 -v -timeout 30m ./integration/ ${args[@]+"${args[@]}"} 2>&1 | tee "$log"
code=${PIPESTATUS[0]}
set -e
echo
grep -E '^(\s*--- (PASS|FAIL|SKIP)|ok|FAIL)' "$log" || true
exit "$code"
