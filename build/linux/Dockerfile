# The Linux build machine: Ubuntu 22.04, so the .deb and the AppImage run on
# 22.04 and everything newer. scripts/build-linux.sh runs inside it.
FROM ubuntu:22.04
ARG GO_VERSION=1.26.5
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
      build-essential cmake pkg-config curl ca-certificates git file unzip zip xz-utils \
      libgtk-3-dev libwebkit2gtk-4.1-dev dpkg-dev fuse libfuse2 \
    && rm -rf /var/lib/apt/lists/*
RUN arch="$(dpkg --print-architecture)" && \
    curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-${arch}.tar.gz" | tar -xz -C /usr/local
ENV PATH=/usr/local/go/bin:$PATH
RUN git config --global --add safe.directory '*'
