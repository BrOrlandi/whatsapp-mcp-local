#!/usr/bin/env bash
# Builds whisper-cli from whisper.cpp as one self-contained program: Metal on
# macOS (arm64 and x86_64 in one universal binary), plain CPU on Linux,
# without OpenMP or shared libraries, so it runs on any machine of its kind.
# Windows uses whisper.cpp's own release (see package.sh).
#
#   scripts/sidecars/build-whisper.sh <out-dir>
set -euo pipefail

VERSION="${WHISPER_VERSION:-1.9.2}"
OUT="$(mkdir -p "$1" && cd "$1" && pwd)"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
curl -fsSL "https://github.com/ggml-org/whisper.cpp/archive/refs/tags/v${VERSION}.tar.gz" | tar -xz -C "$WORK"
src="$WORK/whisper.cpp-${VERSION}"

flags=(
  -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=OFF
  -DWHISPER_BUILD_TESTS=OFF -DWHISPER_BUILD_SERVER=OFF -DWHISPER_SDL2=OFF -DWHISPER_CURL=OFF
  -DGGML_NATIVE=OFF -DGGML_OPENMP=OFF
)
case "$(uname -s)" in
  Darwin)
    flags+=(-DGGML_METAL=ON -DGGML_METAL_EMBED_LIBRARY=ON -DCMAKE_OSX_ARCHITECTURES="arm64;x86_64" -DCMAKE_OSX_DEPLOYMENT_TARGET=11.0)
    ;;
  Linux)
    # Without -march=native, the CPU features a 2013-or-later x86 machine
    # has; arm64 always has NEON.
    if [ "$(uname -m)" = x86_64 ]; then
      flags+=(-DGGML_AVX=ON -DGGML_AVX2=ON -DGGML_FMA=ON -DGGML_F16C=ON)
    fi
    flags+=(-DCMAKE_EXE_LINKER_FLAGS="-static-libgcc -static-libstdc++")
    ;;
esac
cmake -S "$src" -B "$WORK/build" "${flags[@]}" >/dev/null
cmake --build "$WORK/build" --config Release -j "$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 4)" --target whisper-cli >/dev/null
cp "$WORK/build/bin/whisper-cli" "$OUT/"
strip "$OUT/whisper-cli" 2>/dev/null || true
cp "$src/LICENSE" "$OUT/whisper.cpp-LICENSE.txt"
ls -l "$OUT/whisper-cli"
