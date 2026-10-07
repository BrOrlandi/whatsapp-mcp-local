#!/usr/bin/env bash
# Renders one version of the explainer, with its sound at the loudness of
# spoken web video, and its poster:
#
#   scripts/render.sh curto          # out/curto.mp4 and out/curto.jpg, to watch
#   scripts/render.sh curto --site   # and onto the landing page, in site/assets/video/
set -euo pipefail
cd "$(dirname "$0")/.."
VERSION=${1:?usage: render.sh <version> [--site]}
mkdir -p out

[ "${SKIP_RENDER:-0}" = 1 ] || npx remotion render "$VERSION" "out/$VERSION-raw.mp4"

# Two passes of loudnorm: measure, then correct to -16 LUFS without pumping.
ffmpeg -hide_banner -nostats -i "out/$VERSION-raw.mp4" -vn -af loudnorm=I=-16:TP=-1.5:LRA=11:print_format=json -f null - 2> out/loudness.txt
measured=$(python3 - <<'PY'
import json
text = open("out/loudness.txt").read()
start = text.rindex("{")
d = json.loads(text[start:text.index("}", start) + 1])
print(f"measured_I={d['input_i']}:measured_TP={d['input_tp']}:measured_LRA={d['input_lra']}:measured_thresh={d['input_thresh']}")
PY
)
ffmpeg -hide_banner -loglevel error -y -i "out/$VERSION-raw.mp4" \
  -c:v libx264 -preset slow -crf 23 -tune animation -pix_fmt yuv420p \
  -af "loudnorm=I=-16:TP=-1.5:LRA=11:$measured:linear=true" -ar 48000 -c:a aac -b:a 160k \
  -movflags +faststart "out/$VERSION.mp4"

npx remotion still "$VERSION-poster" "out/$VERSION-poster.png"
ffmpeg -hide_banner -loglevel error -y -i "out/$VERSION-poster.png" -vf scale=1280:-2 -q:v 3 "out/$VERSION.jpg"

if [ "${2:-}" = --site ]; then
  mkdir -p ../site/assets/video
  cp "out/$VERSION.mp4" ../site/assets/video/como-funciona.mp4
  cp "out/$VERSION.jpg" ../site/assets/video/como-funciona.jpg
fi
ls -la "out/$VERSION.mp4" "out/$VERSION.jpg"
