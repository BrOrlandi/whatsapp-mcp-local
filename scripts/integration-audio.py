#!/usr/bin/env python3
"""Make the voice notes the integration suite transcribes.

Reads integration/testdata/audio/cases.json and, for each case whose file is
missing (or every case with --force), has ElevenLabs say the text and saves it
as a WhatsApp-style voice note: Ogg Opus, mono, 16 kHz. The files are kept in
the repository, so the suite does not need the API; run this only to add or
change a case.

    ELEVENLABS_API_KEY=... scripts/integration-audio.py [--force]

ELEVENLABS_VOICE_ID picks another voice (default: Sarah, a premade voice that
the multilingual model speaks in Portuguese, Spanish and English). Needs ffmpeg.
"""
import json
import os
import subprocess
import sys
import tempfile
import urllib.error
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DIR = ROOT / "integration" / "testdata" / "audio"
MODEL = "eleven_multilingual_v2"
VOICE = os.environ.get("ELEVENLABS_VOICE_ID", "EXAVITQu4vr4xnSDxMaL")


def tts(text):
    """The text said by ElevenLabs, as MP3 bytes."""
    request = urllib.request.Request(
        f"https://api.elevenlabs.io/v1/text-to-speech/{VOICE}?output_format=mp3_44100_128",
        data=json.dumps({
            "text": text,
            "model_id": MODEL,
            "voice_settings": {"stability": 0.5, "similarity_boost": 0.75},
        }).encode(),
        headers={"Content-Type": "application/json", "Accept": "audio/mpeg",
                 "xi-api-key": os.environ["ELEVENLABS_API_KEY"]},
    )
    try:
        return urllib.request.urlopen(request, timeout=300).read()
    except urllib.error.HTTPError as e:
        sys.exit(f"ElevenLabs answered {e.code}: {e.read().decode(errors='replace')[:300]}")


def main():
    if "ELEVENLABS_API_KEY" not in os.environ:
        sys.exit("set ELEVENLABS_API_KEY")
    force = "--force" in sys.argv
    for case in json.loads((DIR / "cases.json").read_text()):
        out = DIR / case["file"]
        if out.exists() and not force:
            continue
        print(f"{case['file']}: {case['text']}")
        with tempfile.NamedTemporaryFile(suffix=".mp3", delete=False) as f:
            f.write(tts(case["text"]))
            mp3 = f.name
        subprocess.run(
            ["ffmpeg", "-loglevel", "error", "-y", "-i", mp3, "-ac", "1", "-ar", "16000",
             "-c:a", "libopus", "-b:a", "24k", "-application", "voip", str(out)],
            check=True,
        )
        os.unlink(mp3)


if __name__ == "__main__":
    main()
