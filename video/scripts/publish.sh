#!/usr/bin/env bash
# Renders the explainer and puts it on the landing page, in site/assets/video/:
# the MP4 with its sound at the loudness of spoken web video, and the poster.
set -euo pipefail
cd "$(dirname "$0")/.."
OUT=../site/assets/video
mkdir -p out "$OUT"

[ "${SKIP_RENDER:-0}" = 1 ] || npx remotion render Explainer out/raw.mp4

# Two passes of loudnorm: measure, then correct to -16 LUFS without pumping.
ffmpeg -hide_banner -nostats -i out/raw.mp4 -vn -af loudnorm=I=-16:TP=-1.5:LRA=11:print_format=json -f null - 2> out/loudness.txt
measured=$(python3 - <<'PY'
import json
text = open("out/loudness.txt").read()
start = text.rindex("{")
d = json.loads(text[start:text.index("}", start) + 1])
print(f"measured_I={d['input_i']}:measured_TP={d['input_tp']}:measured_LRA={d['input_lra']}:measured_thresh={d['input_thresh']}")
PY
)
ffmpeg -hide_banner -loglevel error -y -i out/raw.mp4 \
  -c:v libx264 -preset slow -crf 23 -tune animation -pix_fmt yuv420p \
  -af "loudnorm=I=-16:TP=-1.5:LRA=11:$measured:linear=true" -ar 48000 -c:a aac -b:a 160k \
  -movflags +faststart "$OUT/como-funciona.mp4"

# The poster: every tool around Claude, the WhatsApp among them, before the
# first subtitle of the next scene.
npx remotion still Explainer --frame=905 out/poster.png
ffmpeg -hide_banner -loglevel error -y -i out/poster.png -vf scale=1280:-2 -q:v 3 "$OUT/como-funciona.jpg"

ls -la "$OUT"
