#!/usr/bin/env bash
# Builds the smallest ffmpeg that turns a WhatsApp voice note into the 16 kHz
# mono WAV whisper.cpp reads: Ogg/Opus mostly, plus the M4A/AAC and MP3 that
# arrive as audio from other apps. Nothing else is compiled in, which keeps it
# to a few megabytes instead of the usual eighty.
#
#   scripts/sidecars/build-ffmpeg.sh <out-dir> [cross-prefix]
#
# cross-prefix builds for Windows from Linux, e.g. x86_64-w64-mingw32-.
# MACOSX_DEPLOYMENT_TARGET and ARCH (arm64, x86_64) are honoured on macOS.
set -euo pipefail

VERSION="${FFMPEG_VERSION:-7.1.1}"
OUT="$(mkdir -p "$1" && cd "$1" && pwd)"
CROSS="${2:-}"
WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT

curl -fsSL "https://ffmpeg.org/releases/ffmpeg-${VERSION}.tar.xz" | tar -xJ -C "$WORK"
cd "$WORK/ffmpeg-${VERSION}"

flags=(
  --disable-everything --disable-autodetect --disable-doc --disable-debug
  --disable-network --disable-ffplay --disable-ffprobe --enable-small
  --enable-static --disable-shared --disable-x86asm
  --disable-avdevice --disable-swscale --disable-postproc
  --enable-protocol=file --enable-protocol=pipe
  --enable-demuxer=ogg,mov,mp3,wav,matroska
  --enable-parser=opus,vorbis,aac,mpegaudio
  --enable-decoder=opus,vorbis,aac,mp3float,mp3,pcm_s16le,pcm_f32le
  --enable-muxer=wav --enable-encoder=pcm_s16le
  --enable-filter=aresample,aformat,anull,abuffer,abuffersink
)
case "$(uname -s)" in
  Darwin)
    arch="${ARCH:-$(uname -m)}"
    export MACOSX_DEPLOYMENT_TARGET="${MACOSX_DEPLOYMENT_TARGET:-11.0}"
    flags+=(--arch="$arch" --cc="clang -arch $arch" --extra-cflags="-mmacosx-version-min=$MACOSX_DEPLOYMENT_TARGET" --extra-ldflags="-mmacosx-version-min=$MACOSX_DEPLOYMENT_TARGET")
    [ "$arch" != "$(uname -m)" ] && flags+=(--enable-cross-compile --target-os=darwin)
    ;;
esac
if [ -n "$CROSS" ]; then
  flags+=(--enable-cross-compile --cross-prefix="$CROSS" --target-os=mingw32 --arch=x86_64 --extra-ldflags=-static)
elif [ "$(uname -s)" = Linux ]; then
  flags+=(--extra-ldflags=-static)
fi

# Decoding a voice note takes milliseconds without hand-written assembly,
# and leaving it out spares every build machine an assembler.
./configure --prefix="$WORK/prefix" "${flags[@]}" >"$WORK/configure.log" 2>&1 || { tail -20 "$WORK/configure.log" >&2; exit 1; }
make -j"$(getconf _NPROCESSORS_ONLN 2>/dev/null || echo 4)" >"$WORK/make.log" 2>&1 || { tail -20 "$WORK/make.log" >&2; exit 1; }
bin=ffmpeg
[ -n "$CROSS" ] && bin=ffmpeg.exe
"${CROSS}strip" "$bin" 2>/dev/null || strip "$bin" 2>/dev/null || true
cp "$bin" "$OUT/"
ls -l "$OUT/$bin"
