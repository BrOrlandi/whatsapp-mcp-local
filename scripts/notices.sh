#!/usr/bin/env bash
# Prints the licence notices that ship with the app: its own, and those of
# the programs and libraries inside it.
#
#   scripts/notices.sh [wacli-LICENSE.txt]
set -euo pipefail
cd "$(dirname "$0")/.."
cat <<TXT
WhatsApp MCP
============
$(cat LICENSE)


wacli (https://github.com/openclaw/wacli), shipped inside the app
=================================================================
$( [ -n "${1:-}" ] && [ -f "$1" ] && cat "$1" || echo "MIT License. See https://github.com/openclaw/wacli/blob/main/LICENSE" )


whatsmeow (https://github.com/tulir/whatsmeow), compiled into wacli
===================================================================
Mozilla Public License 2.0. The source code of whatsmeow is available at
https://github.com/tulir/whatsmeow, and the MPL-2.0 text at
https://www.mozilla.org/MPL/2.0/.


Wails (https://wails.io)
========================
MIT License. Copyright (c) 2018-Present Lea Anthony.
https://github.com/wailsapp/wails/blob/master/LICENSE


modernc.org/sqlite
==================
BSD 3-Clause License. https://gitlab.com/cznic/sqlite/-/blob/master/LICENSE


PDFium (https://pdfium.googlesource.com/pdfium/), compiled to WebAssembly
=========================================================================
Apache License 2.0 and BSD 3-Clause License, with the libraries built into it
(FreeType, libjpeg-turbo, OpenJPEG, Little CMS, zlib) under their own licences.
https://pdfium.googlesource.com/pdfium/+/main/LICENSE
Built for WebAssembly by go-pdfium from https://github.com/bblanchon/pdfium-binaries.


go-pdfium (https://github.com/klippa-app/go-pdfium)
===================================================
MIT License. https://github.com/klippa-app/go-pdfium/blob/main/LICENSE


wazero (https://wazero.io)
==========================
Apache License 2.0. https://github.com/tetratelabs/wazero/blob/main/LICENSE


Go extended libraries (golang.org/x/image, golang.org/x/text, golang.org/x/sys)
==============================================================================
BSD 3-Clause License. https://go.dev/LICENSE


whisper.cpp and ffmpeg (downloaded only when transcription is turned on)
=======================================================================
whisper.cpp: MIT License, https://github.com/ggml-org/whisper.cpp
FFmpeg: LGPL 2.1 or later, built without GPL components;
source at https://ffmpeg.org/releases/ and the build script in
scripts/sidecars/build-ffmpeg.sh of this repository.
TXT
