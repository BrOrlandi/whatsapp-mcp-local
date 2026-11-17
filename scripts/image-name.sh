#!/usr/bin/env bash
# Prints the Docker image's name, as compose.yml names it: the one place it is
# written, so a rename is a change there only.
set -euo pipefail
cd "$(dirname "$0")/.."
# shellcheck disable=SC2016 # the ${...} is compose's, matched literally
name="$(sed -n 's/.*image: \${WHATSAPP_MCP_IMAGE:-\([^}]*\)}:.*/\1/p' compose.yml | head -1)"
[ -n "$name" ] || { echo "compose.yml does not name the image" >&2; exit 1; }
echo "$name"
