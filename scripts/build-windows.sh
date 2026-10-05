#!/usr/bin/env bash
# Builds WhatsApp-MCP-Setup.exe: the app, the bridge and the pinned wacli for
# Windows (amd64), in a per-user NSIS installer. Runs on Linux, macOS or the
# Windows runner's bash; needs Go and makensis (apt install nsis, brew install
# makensis).
#
# VERSION   the version to stamp (default: git describe)
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo 0.0.0)}"
VERSION="${VERSION#v}"
# Windows wants four numbers; anything after them (a commit, -rc1) is dropped.
num="$(printf '%s' "$VERSION" | sed -E 's/^([0-9]+(\.[0-9]+){0,3}).*/\1/')"
[[ "$num" =~ ^[0-9] ]] || num=0.0.0
VERSION4="$(printf '%s.0.0.0' "$num" | cut -d. -f1-4)"
OUT="build/bin"
work="$(mktemp -d)"
trap 'rm -rf "$work" cmd/app/rsrc_windows_*.syso cmd/whatsapp-mcp-bridge/rsrc_windows_*.syso' EXIT
stage="$work/stage"
mkdir -p "$stage/bin" "$OUT"

winres() {
  go run github.com/tc-hib/go-winres@v0.3.3 make --in winres.json \
    --out "$1/rsrc" --arch amd64 --product-version "$VERSION4" --file-version "$VERSION4"
}
echo "==> building $VERSION ($VERSION4)"
(cd build/windows && winres ../../cmd/app && winres ../../cmd/whatsapp-mcp-bridge)
ldflags="-s -w -X main.version=$VERSION"
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "$ldflags -H windowsgui" -o "$stage/WhatsApp MCP.exe" ./cmd/app
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -ldflags "$ldflags" -o "$stage/whatsapp-mcp-bridge.exe" ./cmd/whatsapp-mcp-bridge
scripts/fetch-wacli.sh windows amd64 "$stage/bin" >/dev/null
scripts/notices.sh "$stage/bin/wacli-LICENSE.txt" > "$stage/THIRD-PARTY-NOTICES.txt"
rm -f "$stage/bin/wacli-LICENSE.txt"

curl -fsSL -o "$work/MicrosoftEdgeWebview2Setup.exe" 'https://go.microsoft.com/fwlink/p/?LinkId=2124703'
cp build/windows/installer.nsi build/windows/icon.ico "$work/"
makensis -V2 -DVERSION="$VERSION" -DVERSION4="$VERSION4" -DSRC="$stage" -DOUT="$PWD/$OUT/WhatsApp-MCP-Setup.exe" "$work/installer.nsi"
ls -l "$OUT/WhatsApp-MCP-Setup.exe"
