#!/usr/bin/env bash
# Renders the app's icons from build/icons/*.svg: the macOS .icns, the
# Windows .ico, the Linux PNG, and the tray glyph the app tints by status.
# Needs rsvg-convert (brew install librsvg) and, for the .icns, macOS.
set -euo pipefail
cd "$(dirname "$0")/.."
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

png() { rsvg-convert -w "$2" -h "$2" "$1" -o "$3"; }

# macOS
set_dir="$tmp/icon.iconset"
mkdir -p "$set_dir"
for size in 16 32 128 256 512; do
  png build/icons/appicon.svg "$size" "$set_dir/icon_${size}x${size}.png"
  png build/icons/appicon.svg "$((size * 2))" "$set_dir/icon_${size}x${size}@2x.png"
done
if command -v iconutil >/dev/null; then
  iconutil -c icns "$set_dir" -o build/darwin/icon.icns
fi

# Windows and Linux: the full-bleed tile, as the favicon draws it
for size in 16 24 32 48 64 128 256 512; do
  png internal/brand/favicon.svg "$size" "$tmp/tile-$size.png"
done
python3 - "$tmp" build/windows/icon.ico <<'PY'
import struct, sys
tmp, out = sys.argv[1], sys.argv[2]
sizes = [16, 24, 32, 48, 64, 128, 256]
blobs = [open(f"{tmp}/tile-{s}.png", "rb").read() for s in sizes]
header = struct.pack("<HHH", 0, 1, len(sizes))
offset = 6 + 16 * len(sizes)
entries = b""
for s, b in zip(sizes, blobs):
    entries += struct.pack("<BBBBHHII", s % 256, s % 256, 0, 0, 1, 32, len(b), offset)
    offset += len(b)
open(out, "wb").write(header + entries + b"".join(blobs))
PY
cp "$tmp/tile-512.png" build/linux/whatsapp-mcp.png
cp "$tmp/tile-256.png" cmd/app/icons/app.png
png build/icons/tray.svg 64 cmd/app/icons/tray.png
ls -l build/darwin/icon.icns build/windows/icon.ico build/linux/whatsapp-mcp.png cmd/app/icons/*.png
