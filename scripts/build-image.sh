#!/usr/bin/env bash
# Builds the server mode's Docker image for linux/amd64 and linux/arm64 into
# this computer's Docker store, tagged X.Y.Z and latest (a test version such
# as X.Y.Z-beta.1 is not tagged latest), and never pushes it
# (scripts/push-image.sh does, as its own step). scripts/release.sh runs it
# for every release; it also runs alone:
#
#   scripts/build-image.sh --check    only checks that Docker can hold the image
#   scripts/build-image.sh X.Y.Z      checks, builds, and prints each arch's version
set -euo pipefail
cd "$(dirname "$0")/.."

die() { echo "$*" >&2; exit 1; }

# check stops with a plain message when Docker cannot build and keep an
# image of two platforms: it is not running, buildx is missing, or its image
# store is the classic one (which keeps one platform per tag).
check() {
  command -v docker >/dev/null || die "Docker is not installed: the release builds the server image with it"
  docker info >/dev/null 2>&1 || die "Docker is not running: start it and run this again"
  docker buildx version >/dev/null 2>&1 || die "docker buildx is missing: the server image is built with it"
  docker info -f '{{.DriverStatus}}' 2>/dev/null | grep -q 'io.containerd.snapshotter' ||
    die "Docker's image store cannot keep an image of two platforms: turn on the containerd image store (Docker Desktop: Settings › General › \"Use containerd for pulling and storing images\"; OrbStack has it) and run this again"
}

VERSION="${1:?usage: scripts/build-image.sh --check | X.Y.Z}"
check
[ "$VERSION" = --check ] && { echo "Docker can build and keep the server image"; exit 0; }
VERSION="${VERSION#v}"
IMAGE="$(scripts/image-name.sh)"
echo "==> image $IMAGE:$VERSION"
# A test version (1.9.0-beta.1) is never latest: compose.yml installs latest.
tags=(-t "$IMAGE:$VERSION")
case "$VERSION" in *-*) ;; *) tags+=(-t "$IMAGE:latest") ;; esac
docker buildx build --platform linux/amd64,linux/arm64 -f build/docker/Dockerfile \
  --build-arg VERSION="v$VERSION" --build-arg TAGS="${TAGS:-}" \
  "${tags[@]}" --load .
for arch in amd64 arm64; do
  echo "    $arch: $(docker run --rm --platform "linux/$arch" "$IMAGE:$VERSION" version)"
done
