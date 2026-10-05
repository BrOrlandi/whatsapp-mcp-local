#!/usr/bin/env python3
"""Pins a set of transcription archives in internal/sidecar/manifest.json.

    scripts/sidecars/manifest.py sidecars-N <dir with the five archives>

Writes the manifest with each archive's sha256 and size and its address in the
release sidecars-N of this repository, and copies it into the folder, to be
published with the archives.
"""
import hashlib
import json
import os
import shutil
import sys

REPO = "BrOrlandi/whatsapp-mcp-local"
ARCHIVES = {
    "transcription_darwin_universal.tar.gz": ("darwin/universal", "metal"),
    "transcription_linux_amd64.tar.gz": ("linux/amd64", "cpu"),
    "transcription_linux_arm64.tar.gz": ("linux/arm64", "cpu"),
    "transcription_windows_amd64.zip": ("windows/amd64", "cpu"),
    "transcription_windows_amd64_cuda.zip": ("windows/amd64/cuda", "cuda"),
}

tag, folder = sys.argv[1], sys.argv[2]
packages = {}
for name, (key, accel) in ARCHIVES.items():
    with open(os.path.join(folder, name), "rb") as f:
        body = f.read()
    packages[key] = {
        "url": f"https://github.com/{REPO}/releases/download/{tag}/{name}",
        "sha256": hashlib.sha256(body).hexdigest(),
        "size": len(body),
        "accel": accel,
    }
path = os.path.join(os.path.dirname(__file__), "..", "..", "internal", "sidecar", "manifest.json")
with open(path, "w") as f:
    json.dump({"version": tag, "packages": packages}, f, indent=2)
    f.write("\n")
shutil.copy(path, os.path.join(folder, "manifest.json"))
print(os.path.normpath(path))
