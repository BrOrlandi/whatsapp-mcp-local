#!/usr/bin/env bash
# Builds WhatsApp MCP for Linux on the machine's own architecture: a .deb for
# Ubuntu and Debian, and an AppImage that carries WebKitGTK and runs on any
# distribution. Needs Go, a C compiler, libgtk-3-dev, libwebkit2gtk-4.1-dev,
# dpkg-deb and file.
#
# VERSION   the version to stamp (default: git describe)
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo 0.0.0)}"
VERSION="${VERSION#v}"
case "$(uname -m)" in
  x86_64) arch=amd64 appimage_arch=x86_64 ;;
  aarch64|arm64) arch=arm64 appimage_arch=aarch64 ;;
  *) echo "unsupported machine $(uname -m)" >&2; exit 1 ;;
esac
repo="$PWD"
OUT="$repo/build/bin"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
mkdir -p "$OUT"
ldflags="-s -w -X main.version=$VERSION"

echo "==> building $VERSION for linux/$arch"
# GTK 3 and WebKitGTK 4.1 are in every supported Ubuntu (22.04 and later)
# and Debian 12; Wails defaults to GTK 4, which 22.04 lacks.
CGO_ENABLED=1 go build -tags gtk3 -trimpath -ldflags "$ldflags" -o "$work/whatsapp-mcp-app" ./cmd/app
CGO_ENABLED=0 go build -trimpath -ldflags "$ldflags" -o "$work/whatsapp-mcp-bridge" ./cmd/whatsapp-mcp-bridge
scripts/fetch-wacli.sh linux "$arch" "$work" >/dev/null
scripts/notices.sh "$work/wacli-LICENSE.txt" > "$work/THIRD-PARTY-NOTICES.txt"

# ---- .deb -------------------------------------------------------------------
deb="$work/deb"
mkdir -p "$deb/DEBIAN" "$deb/opt/whatsapp-mcp/bin" "$deb/usr/bin" \
  "$deb/usr/share/applications" "$deb/usr/share/icons/hicolor/512x512/apps" "$deb/usr/share/doc/whatsapp-mcp"
install -m 0755 "$work/whatsapp-mcp-app" "$work/whatsapp-mcp-bridge" "$deb/opt/whatsapp-mcp/"
install -m 0755 "$work/wacli" "$deb/opt/whatsapp-mcp/bin/"
ln -s /opt/whatsapp-mcp/whatsapp-mcp-app "$deb/usr/bin/whatsapp-mcp-app"
sed 's|^Exec=.*|Exec=/opt/whatsapp-mcp/whatsapp-mcp-app|' build/linux/whatsapp-mcp.desktop > "$deb/usr/share/applications/whatsapp-mcp.desktop"
install -m 0644 build/linux/whatsapp-mcp.png "$deb/usr/share/icons/hicolor/512x512/apps/whatsapp-mcp.png"
install -m 0644 "$work/THIRD-PARTY-NOTICES.txt" "$deb/usr/share/doc/whatsapp-mcp/copyright"
size="$(du -sk "$deb" | cut -f1)"
cat > "$deb/DEBIAN/control" <<CONTROL
Package: whatsapp-mcp
Version: ${VERSION}
Architecture: ${arch}
Maintainer: Bruno Orlandi <https://github.com/BrOrlandi>
Installed-Size: ${size}
Depends: libgtk-3-0, libwebkit2gtk-4.1-0
Section: net
Priority: optional
Homepage: https://github.com/BrOrlandi/whatsapp-mcp-v2
Description: WhatsApp for AI tools, on this computer
 WhatsApp MCP connects your WhatsApp to Claude and other AI tools through
 the Model Context Protocol. Everything runs on this computer.
CONTROL
dpkg-deb --root-owner-group --build "$deb" "$OUT/whatsapp-mcp_${VERSION}_${arch}.deb" >/dev/null

# ---- AppImage ---------------------------------------------------------------
# The way Wails builds one: linuxdeploy with its GTK plugin, plus WebKit's
# helper processes, which no library dependency pulls in.
appdir="$work/WhatsApp-MCP.AppDir"
mkdir -p "$appdir/usr/bin"
install -m 0755 "$work/whatsapp-mcp-app" "$work/whatsapp-mcp-bridge" "$appdir/usr/bin/"
mkdir -p "$appdir/usr/bin/bin" && install -m 0755 "$work/wacli" "$appdir/usr/bin/bin/"
install -m 0644 "$work/THIRD-PARTY-NOTICES.txt" "$appdir/usr/bin/"
for f in WebKitWebProcess WebKitNetworkProcess libwebkit2gtkinjectedbundle.so; do
  src="$(find /usr/lib -name "$f" -path '*webkit2gtk-4.1*' 2>/dev/null | head -1)"
  [ -n "$src" ] || { echo "WebKitGTK file $f not found" >&2; exit 1; }
  mkdir -p "$appdir$(dirname "$src")"
  cp "$src" "$appdir$src"
done
tools="$work/tools"
mkdir -p "$tools"
curl -fsSL -o "$tools/linuxdeploy" "https://github.com/linuxdeploy/linuxdeploy/releases/download/continuous/linuxdeploy-${appimage_arch}.AppImage"
curl -fsSL -o "$appdir/AppRun" "https://github.com/AppImage/AppImageKit/releases/download/continuous/AppRun-${appimage_arch}"
cp "$(go env GOMODCACHE)/github.com/wailsapp/wails/v3@$(go list -m -f '{{.Version}}' github.com/wailsapp/wails/v3)/internal/commands/linuxdeploy-plugin-gtk.sh" "$tools/"
chmod +x "$tools/linuxdeploy" "$tools/linuxdeploy-plugin-gtk.sh" "$appdir/AppRun"
(
  cd "$tools"
  export DEPLOY_GTK_VERSION=3 NO_STRIP=1 PATH="$tools:$PATH"
  deploy() {
    ./linuxdeploy --appimage-extract-and-run --appdir "$appdir" --executable "$appdir/usr/bin/whatsapp-mcp-app" \
      --desktop-file "$repo/build/linux/whatsapp-mcp.desktop" --icon-file "$repo/build/linux/whatsapp-mcp.png" \
      "$@" >>"$work/linuxdeploy.log" 2>&1 || { tail -30 "$work/linuxdeploy.log" >&2; exit 1; }
  }
  deploy --plugin gtk
  # WebKitGTK starts its helper processes from a path compiled into it,
  # /usr/lib/<arch>/webkit2gtk-4.1. Rewritten as ././/lib/…, the same length,
  # it is relative to the folder AppRun runs the app from, $APPDIR/usr,
  # where the helpers were copied.
  find "$appdir/usr/lib" -name 'libwebkit2gtk-4.1.so*' -type f -exec sed -i 's|/usr/lib/|././/lib/|g' {} +
  OUTPUT="WhatsApp-MCP-${appimage_arch}.AppImage" deploy --output appimage
  mv "WhatsApp-MCP-${appimage_arch}.AppImage" "$OUT/"
)
ls -l "$OUT"/whatsapp-mcp_*_"$arch".deb "$OUT/WhatsApp-MCP-${appimage_arch}.AppImage"
