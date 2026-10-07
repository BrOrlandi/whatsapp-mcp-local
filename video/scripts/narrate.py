#!/usr/bin/env python3
"""Generates the narration: one clip per line of narration/script.json.

Each clip is spoken by Gemini TTS, trimmed of the silence at its ends, kept as
an MP3 (so the repository holds the takes the video was cut to), and
transcribed back with whisper.cpp to time every word, which is what the
subtitles and the scenes follow. A clip whose text, voice, model and style did
not change is not generated again, so editing one line costs one request.

    GEMINI_API_KEY=... python3 scripts/narrate.py          # from video/
    python3 scripts/narrate.py --force 07                  # redo one clip

Needs ffmpeg and whisper-cli on the PATH, and a whisper model (WHISPER_MODEL;
by default the one the app installs for transcription).
"""
import base64
import difflib
import hashlib
import json
import os
import re
import subprocess
import sys
import tempfile
import unicodedata
import urllib.request
import wave
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
SCRIPT = ROOT / "narration" / "script.json"
CACHE = ROOT / "narration" / "cache.json"
OUT_DIR = ROOT / "public" / "narration"
TIMELINE = ROOT / "src" / "narration.json"
MODEL = os.environ.get(
    "WHISPER_MODEL",
    str(Path.home() / "Library/Application Support/WhatsApp MCP/models/ggml-large-v3-turbo-q5_0.bin"),
)


def tts(model, voice, style, text):
    body = {
        "contents": [{"parts": [{"text": f"{style}: {text}"}]}],
        "generationConfig": {
            "responseModalities": ["AUDIO"],
            "speechConfig": {"voiceConfig": {"prebuiltVoiceConfig": {"voiceName": voice}}},
        },
    }
    url = f"https://generativelanguage.googleapis.com/v1beta/models/{model}:generateContent?key={os.environ['GEMINI_API_KEY']}"
    request = urllib.request.Request(url, data=json.dumps(body).encode(), headers={"Content-Type": "application/json"})
    answer = json.load(urllib.request.urlopen(request, timeout=300))
    data = base64.b64decode(answer["candidates"][0]["content"]["parts"][0]["inlineData"]["data"])
    if data[:4] == b"RIFF":
        return data
    # Raw 16-bit PCM at 24 kHz: wrap it in a WAV header.
    with tempfile.NamedTemporaryFile(suffix=".wav", delete=False) as f:
        path = f.name
    with wave.open(path, "wb") as w:
        w.setnchannels(1)
        w.setsampwidth(2)
        w.setframerate(24000)
        w.writeframes(data)
    return Path(path).read_bytes()


def trim(raw: bytes, out: Path):
    with tempfile.NamedTemporaryFile(suffix=".wav", delete=False) as f:
        f.write(raw)
        src = f.name
    edge = "silenceremove=start_periods=1:start_silence=0.04:start_threshold=-45dB"
    subprocess.run(
        ["ffmpeg", "-loglevel", "error", "-y", "-i", src, "-af", f"{edge},areverse,{edge},areverse",
         "-ar", "48000", "-ac", "1", "-c:a", "libmp3lame", "-b:a", "192k", str(out)],
        check=True,
    )


def duration_ms(path: Path) -> int:
    probe = subprocess.run(
        ["ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", str(path)],
        check=True, capture_output=True, text=True,
    )
    return round(float(probe.stdout) * 1000)


def transcribe(path: Path):
    """Word timings from whisper.cpp, one segment per word."""
    with tempfile.TemporaryDirectory() as tmp:
        wav16 = Path(tmp) / "a.wav"
        subprocess.run(["ffmpeg", "-loglevel", "error", "-y", "-i", str(path), "-ar", "16000", "-ac", "1", str(wav16)], check=True)
        subprocess.run(
            ["whisper-cli", "-m", MODEL, "-l", "pt", "-ml", "1", "-sow", "-oj", "-of", str(Path(tmp) / "t"), "-np", "-f", str(wav16)],
            check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
        )
        segments = json.loads((Path(tmp) / "t.json").read_text())["transcription"]
    words = []
    for s in segments:
        text = s["text"].strip()
        if text:
            words.append({"text": text, "startMs": s["offsets"]["from"], "endMs": s["offsets"]["to"]})
    return words


def norm(word: str) -> str:
    word = unicodedata.normalize("NFKD", word.lower())
    return re.sub(r"[^a-z0-9]", "", "".join(c for c in word if not unicodedata.combining(c)))


def align(text: str, heard: list, total_ms: int):
    """Times each word of the script from the words whisper heard.

    Words whisper spelled differently (Jira, MCP) are matched by position
    between the words that do match, so the script's own spelling is what the
    subtitles show.
    """
    words = text.split()
    a = [norm(w) for w in words]
    b = [norm(h["text"]) for h in heard]
    times = [None] * len(words)
    for block in difflib.SequenceMatcher(None, a, b, autojunk=False).get_matching_blocks():
        for k in range(block.size):
            h = heard[block.b + k]
            times[block.a + k] = (h["startMs"], h["endMs"])
    # Fill the gaps by spreading them evenly between the known neighbours.
    i = 0
    while i < len(words):
        if times[i] is not None:
            i += 1
            continue
        j = i
        while j < len(words) and times[j] is None:
            j += 1
        start = times[i - 1][1] if i > 0 else 0
        end = times[j][0] if j < len(words) else total_ms
        step = max(end - start, 1) / (j - i)
        for k in range(i, j):
            times[k] = (round(start + (k - i) * step), round(start + (k - i + 1) * step))
        i = j
    return [{"text": w, "startMs": t[0], "endMs": t[1]} for w, t in zip(words, times)]


def similar(text: str, heard: list) -> float:
    return difflib.SequenceMatcher(None, norm(text), norm("".join(h["text"] for h in heard))).ratio()


def main():
    script = json.loads(SCRIPT.read_text())
    cache = json.loads(CACHE.read_text()) if CACHE.exists() else {}
    force = set(sys.argv[sys.argv.index("--force") + 1:]) if "--force" in sys.argv else set()
    OUT_DIR.mkdir(parents=True, exist_ok=True)
    timeline = []
    for clip in script["clips"]:
        key = hashlib.sha1(json.dumps([script["model"], script["voice"], script["style"], clip["text"]]).encode()).hexdigest()
        out = OUT_DIR / f"{clip['id']}.mp3"
        entry = cache.get(clip["id"])
        if clip["id"] in force or not out.exists() or not entry or entry["key"] != key:
            for attempt in range(5):
                trim(tts(script["model"], script["voice"], script["style"], clip["text"]), out)
                heard = transcribe(out)
                score = similar(clip["text"], heard)
                print(f"{clip['id']} attempt {attempt + 1}: {score:.2f} {' '.join(h['text'] for h in heard)}")
                # The model sometimes reads the style instruction aloud, or
                # drops a word: try again until what it said is the script.
                if score >= 0.8 and not norm(heard[0]["text"]).startswith("say"):
                    break
            else:
                sys.exit(f"clip {clip['id']} never matched its text")
            entry = {"key": key, "heard": heard}
            cache[clip["id"]] = entry
            CACHE.write_text(json.dumps(cache, ensure_ascii=False, indent=1) + "\n")
        total = duration_ms(out)
        timeline.append({
            "id": clip["id"], "scene": clip["scene"], "text": clip["text"], "file": f"narration/{clip['id']}.mp3",
            "durationMs": total, "words": align(clip["text"], entry["heard"], total),
        })
    TIMELINE.write_text(json.dumps(timeline, ensure_ascii=False, indent=1) + "\n")
    print(f"{len(timeline)} clips, {sum(c['durationMs'] for c in timeline) / 1000:.1f}s of narration")


if __name__ == "__main__":
    main()
