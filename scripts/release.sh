#!/usr/bin/env bash
# Builds every file of a release on this Mac, into dist/vX.Y.Z: the signed and
# notarised .dmg, the Windows installer and the Linux AppImage and .deb (amd64
# and arm64) in Docker, the command line for each system, and checksums.txt.
# Releases are always built here, never by CI; publishing them with gh is the
# next step (docs/desenvolvimento.md#publicar).
#
#   SIGN_IDENTITY="Developer ID Application: …" NOTARY_KEY=<.p8> NOTARY_KEY_ID=… NOTARY_ISSUER=… \
#     scripts/release.sh X.Y.Z
#
# It builds the tag vX.Y.Z from a fresh clone, so the checkout it runs from is
# left as it is. Needs Go, Xcode's command line tools and Docker.
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${1:?usage: scripts/release.sh X.Y.Z}"
VERSION="${VERSION#v}"
TAG="v$VERSION"
git rev-parse -q --verify "$TAG^{commit}" >/dev/null || { echo "there is no tag $TAG" >&2; exit 1; }
for v in SIGN_IDENTITY NOTARY_KEY NOTARY_KEY_ID NOTARY_ISSUER; do
  [ -n "${!v:-}" ] || { echo "$v is not set: the .dmg must be signed and notarised" >&2; exit 1; }
done

OUT="$PWD/dist/$TAG"
# Under the repository rather than in a temporary folder, so Docker can mount
# it; a clone rather than a worktree, whose .git Docker cannot follow.
src="$PWD/dist/.src-$TAG"
rm -rf "${OUT:?}" "${src:?}"
mkdir -p "$OUT"
git clone -q --branch "$TAG" "$PWD" "$src"
trap 'rm -rf "$src"' EXIT
cd "$src"

echo "==> macOS"
VERSION="$VERSION" scripts/build-macos.sh
xcrun stapler validate -q build/bin/WhatsApp-MCP.dmg
cp build/bin/WhatsApp-MCP.dmg "$OUT/"

echo "==> Windows"
docker run --rm -v "$src:/src" -w /src -e VERSION="$VERSION" golang:1.26 bash -c \
  'git config --global --add safe.directory "*" && apt-get update -qq && apt-get install -y -qq nsis unzip >/dev/null && scripts/build-windows.sh'

for arch in arm64 amd64; do
  echo "==> Linux $arch"
  docker build -q --platform "linux/$arch" -t "wamcp-linux-build:$arch" build/linux >/dev/null
  docker run --rm --privileged --platform "linux/$arch" -v "$src:/src" -w /src -e VERSION="$VERSION" \
    "wamcp-linux-build:$arch" bash -c 'git config --global --add safe.directory "*" && scripts/build-linux.sh'
done
cp build/bin/WhatsApp-MCP-Setup.exe build/bin/WhatsApp-MCP-*.AppImage build/bin/whatsapp-mcp_"$VERSION"_*.deb "$OUT/"

echo "==> command line"
for target in darwin/arm64 darwin/amd64 linux/arm64 linux/amd64 windows/amd64; do
  os="${target%/*}" arch="${target#*/}"
  bin=whatsapp-mcp; [ "$os" = windows ] && bin=whatsapp-mcp.exe
  rm -rf cli && mkdir cli
  CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "-s -w -X main.version=$TAG" -o "cli/$bin" ./cmd/whatsapp-mcp
  if [ "$os" = windows ]; then
    (cd cli && zip -q "$OUT/whatsapp-mcp_${os}_${arch}.zip" "$bin")
  else
    tar -C cli -czf "$OUT/whatsapp-mcp_${os}_${arch}.tar.gz" "$bin"
  fi
done

cd "$OUT"
shasum -a 256 -- * > checksums.txt
echo
echo "==> $OUT"
cat checksums.txt
echo
echo "Version in the app: $(defaults read "$src/build/bin/WhatsApp MCP.app/Contents/Info" CFBundleShortVersionString)"
