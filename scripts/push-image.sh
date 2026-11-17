#!/usr/bin/env bash
# Pushes the Docker image scripts/release.sh built, for both architectures, to
# its registry: the tags X.Y.Z and latest. A step of its own, run by hand after
# the release is checked; the release build never pushes.
#
#   docker login ghcr.io          # once, with a token that can write packages
#   scripts/push-image.sh X.Y.Z
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${1:?usage: scripts/push-image.sh X.Y.Z}"
VERSION="${VERSION#v}"
IMAGE="$(scripts/image-name.sh)"

# A test version (1.9.0-beta.1) is pushed under its own tag only.
TAGS_TO_PUSH=("$VERSION")
case "$VERSION" in *-*) ;; *) TAGS_TO_PUSH+=(latest) ;; esac

for tag in "${TAGS_TO_PUSH[@]}"; do
  docker image inspect "$IMAGE:$tag" >/dev/null 2>&1 || { echo "$IMAGE:$tag is not on this computer: run scripts/release.sh $VERSION first" >&2; exit 1; }
done
platforms="$(docker image inspect "$IMAGE:$VERSION" --platform linux/amd64 --format '{{.Architecture}}' 2>/dev/null || true) $(docker image inspect "$IMAGE:$VERSION" --platform linux/arm64 --format '{{.Architecture}}' 2>/dev/null || true)"
[ "$platforms" = "amd64 arm64" ] || { echo "$IMAGE:$VERSION does not hold both amd64 and arm64 (got: $platforms)" >&2; exit 1; }

read -r -p "Push $IMAGE with the tags ${TAGS_TO_PUSH[*]} (amd64, arm64)? [y/N] " answer
[ "$answer" = y ] || [ "$answer" = Y ] || { echo "nothing pushed"; exit 1; }
# With the containerd image store, a push sends the index with both platforms.
for tag in "${TAGS_TO_PUSH[@]}"; do
  docker push "$IMAGE:$tag"
done
echo "==> pushed $IMAGE: ${TAGS_TO_PUSH[*]}"
