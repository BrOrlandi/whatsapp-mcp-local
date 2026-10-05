#!/usr/bin/env bash
# Packs what transcription needs on one system into the archive the app
# downloads when transcription is turned on: whisper-cli and ffmpeg, with
# their licences. Prints the archive's name, size and sha256, for
# internal/sidecar/manifest.json.
#
#   scripts/sidecars/package.sh darwin universal <dir with whisper-cli and ffmpeg> <out-dir>
#   scripts/sidecars/package.sh linux amd64 …
#   scripts/sidecars/package.sh windows amd64 <dir with ffmpeg.exe> <out-dir> [cuda]
#
# On Windows, whisper-cli.exe and its DLLs come from whisper.cpp's release.
set -euo pipefail
os="$1" arch="$2" in="$3" out="$(mkdir -p "$4" && cd "$4" && pwd)" variant="${5:-}"
VERSION="${WHISPER_VERSION:-1.9.2}"
stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT
name="transcription_${os}_${arch}${variant:+_$variant}"
if [ "$os" = windows ]; then
  zip="whisper-bin-x64.zip"
  [ "$variant" = cuda ] && zip="whisper-cublas-12.4.0-bin-x64.zip"
  curl -fsSL -o "$stage/w.zip" "https://github.com/ggml-org/whisper.cpp/releases/download/v${VERSION}/${zip}"
  mkdir -p "$stage/x" "$stage/pkg"
  unzip -q "$stage/w.zip" -d "$stage/x"
  dir="$(dirname "$(find "$stage/x" -name whisper-cli.exe | head -1)")"
  cp "$dir"/whisper-cli.exe "$dir"/*.dll "$stage/pkg/"
  cp "$in/ffmpeg.exe" "$stage/pkg/"
  curl -fsSL -o "$stage/pkg/whisper.cpp-LICENSE.txt" "https://raw.githubusercontent.com/ggml-org/whisper.cpp/v${VERSION}/LICENSE"
  (cd "$stage/pkg" && zip -q -r "$out/$name.zip" .)
  file="$out/$name.zip"
else
  mkdir -p "$stage/pkg"
  cp "$in/whisper-cli" "$in/ffmpeg" "$stage/pkg/"
  if [ "$os" = darwin ]; then
    # The app checks the signature after download. SIGN_IDENTITY is the
    # Developer ID; without it, a local build is signed ad hoc.
    if [ -n "${SIGN_IDENTITY:-}" ]; then
      codesign --force --options runtime --timestamp --sign "$SIGN_IDENTITY" "$stage/pkg/whisper-cli" "$stage/pkg/ffmpeg"
    else
      codesign --force --sign - "$stage/pkg/whisper-cli" "$stage/pkg/ffmpeg"
    fi
  fi
  [ -f "$in/whisper.cpp-LICENSE.txt" ] && cp "$in/whisper.cpp-LICENSE.txt" "$stage/pkg/"
  tar -czf "$out/$name.tar.gz" -C "$stage/pkg" .
  file="$out/$name.tar.gz"
fi
if command -v sha256sum >/dev/null; then sum="$(sha256sum "$file" | cut -d' ' -f1)"; else sum="$(shasum -a 256 "$file" | cut -d' ' -f1)"; fi
size="$(wc -c < "$file" | tr -d ' ')"
echo "$(basename "$file") $size $sum"
