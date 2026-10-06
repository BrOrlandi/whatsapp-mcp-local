#!/usr/bin/env bash
# Builds WhatsApp MCP.app for macOS (arm64 and amd64 in one universal app),
# with the bridge and the pinned wacli inside, signs it, and wraps it in a
# .dmg that opens with a shortcut to Applications over a background that says
# to drag the app onto it (build/darwin/dmg-settings.py; dmgbuild runs through
# uv, brew install uv).
#
#   scripts/build-macos.sh                 build/bin/WhatsApp MCP.app and .dmg
#
# VERSION       the version to stamp (default: git describe)
# ARCHS         "arm64 amd64" (default) or one of them, for a quicker local build
# SIGN_IDENTITY a "Developer ID Application: …" identity; without it the app is
#               signed ad hoc, which runs here but not on other Macs
# NOTARY_KEY, NOTARY_KEY_ID, NOTARY_ISSUER
#               an App Store Connect API key (.p8 path, its id, the issuer) to
#               notarise and staple the .dmg; skipped when unset
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"
VERSION="${VERSION#v}"
ARCHS="${ARCHS:-arm64 amd64}"
OUT="build/bin"
APP="$OUT/WhatsApp MCP.app"
export MACOSX_DEPLOYMENT_TARGET=11.0
export CGO_CFLAGS="-mmacosx-version-min=11.0" CGO_LDFLAGS="-mmacosx-version-min=11.0"
ldflags="-s -w -X main.version=$VERSION"

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT
apps=() bridges=()
for arch in $ARCHS; do
  cc_arch="$arch"; [ "$arch" = amd64 ] && cc_arch=x86_64
  echo "==> building for $arch"
  CGO_ENABLED=1 GOOS=darwin GOARCH="$arch" CC="clang -arch $cc_arch" \
    go build -trimpath -ldflags "$ldflags" -o "$work/app-$arch" ./cmd/app 2> >(grep -v "ld: warning" >&2)
  CGO_ENABLED=0 GOOS=darwin GOARCH="$arch" go build -trimpath -ldflags "$ldflags" -o "$work/bridge-$arch" ./cmd/whatsapp-mcp-bridge
  apps+=("$work/app-$arch"); bridges+=("$work/bridge-$arch")
done

rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources/bin"
lipo -create -output "$APP/Contents/MacOS/WhatsApp MCP" "${apps[@]}"
lipo -create -output "$APP/Contents/MacOS/whatsapp-mcp-bridge" "${bridges[@]}"
scripts/fetch-wacli.sh darwin universal "$APP/Contents/Resources/bin" >/dev/null
mv "$APP/Contents/Resources/bin/wacli-LICENSE.txt" "$work/wacli-LICENSE.txt"
sed "s/@VERSION@/$VERSION/g" build/darwin/Info.plist > "$APP/Contents/Info.plist"
cp build/darwin/icon.icns "$APP/Contents/Resources/icon.icns"
scripts/notices.sh "$work/wacli-LICENSE.txt" > "$APP/Contents/Resources/THIRD-PARTY-NOTICES.txt"

# Inside out: the bridge, then the bundle, which seals everything else. wacli
# keeps its own Developer ID signature and notarisation from OpenClaw.
if [ -n "${SIGN_IDENTITY:-}" ]; then
  sign=(codesign --force --options runtime --timestamp --sign "$SIGN_IDENTITY")
else
  echo "==> SIGN_IDENTITY not set: signing ad hoc (this Mac only)"
  sign=(codesign --force --sign -)
fi
"${sign[@]}" "$APP/Contents/MacOS/whatsapp-mcp-bridge"
"${sign[@]}" "$APP"
codesign --verify --deep --strict "$APP"

echo "==> making the .dmg"
DMG="$OUT/WhatsApp-MCP.dmg"
rm -f "$DMG"
uv tool run dmgbuild==1.6.7 -s build/darwin/dmg-settings.py -D "app=$APP" "WhatsApp MCP" "$DMG" >/dev/null
if [ -n "${SIGN_IDENTITY:-}" ]; then
  codesign --force --timestamp --sign "$SIGN_IDENTITY" "$DMG"
fi
if [ -n "${NOTARY_KEY:-}" ] && [ -n "${SIGN_IDENTITY:-}" ]; then
  echo "==> notarising"
  xcrun notarytool submit "$DMG" --key "$NOTARY_KEY" --key-id "$NOTARY_KEY_ID" --issuer "$NOTARY_ISSUER" --wait
  xcrun stapler staple "$DMG"
  xcrun stapler validate "$DMG"
fi
ls -l "$DMG"
