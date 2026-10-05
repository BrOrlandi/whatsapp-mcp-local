#!/usr/bin/env bash
# Downloads the wacli pinned in build/wacli.env for one system, checks it
# against the pinned checksum, and puts the program in <out-dir>.
#
#   scripts/fetch-wacli.sh darwin universal build/out
#   scripts/fetch-wacli.sh windows amd64 build/out
set -euo pipefail
cd "$(dirname "$0")/.."
os="$1" arch="$2" out="$3"
# shellcheck source=/dev/null
. build/wacli.env
var="WACLI_SHA256_${os}_${arch}"
want="${!var:?no pinned checksum for $os/$arch}"
ext=tar.gz
[ "$os" = windows ] && ext=zip
name="wacli_${WACLI_VERSION}_${os}_${arch}.${ext}"
[ "$os/$arch" = darwin/universal ] && name="wacli_${WACLI_VERSION}_universal_darwin_all.tar.gz"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
curl -fsSL -o "$tmp/$name" "https://github.com/openclaw/wacli/releases/download/v${WACLI_VERSION}/${name}"
if command -v sha256sum >/dev/null; then got="$(sha256sum "$tmp/$name" | cut -d' ' -f1)"; else got="$(shasum -a 256 "$tmp/$name" | cut -d' ' -f1)"; fi
[ "$got" = "$want" ] || { echo "checksum mismatch for $name: got $got, want $want" >&2; exit 1; }
mkdir -p "$tmp/x" "$out"
if [ "$ext" = zip ]; then unzip -q "$tmp/$name" -d "$tmp/x"; else tar -xzf "$tmp/$name" -C "$tmp/x"; fi
bin=wacli
[ "$os" = windows ] && bin=wacli.exe
src="$(find "$tmp/x" -type f -name "$bin" | head -1)"
[ -n "$src" ] || { echo "no $bin in $name" >&2; exit 1; }
cp "$src" "$out/$bin"
chmod 0755 "$out/$bin"
# The licence travels with the program.
lic="$(find "$tmp/x" -maxdepth 3 -type f -iname 'LICENSE*' | head -1)"
[ -n "$lic" ] && cp "$lic" "$out/wacli-LICENSE.txt"
echo "$out/$bin"
