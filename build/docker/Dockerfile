# WhatsApp MCP's command line in a container, for a server without a screen
# (docs/servidor.md). The container is the service: `serve` runs in it, and
# every other command runs beside it with `docker compose exec`.
#
# Two ways to use it:
#   - the server mode, with the published image and the root compose.yml
#     (install-docker.sh): a public listener behind a proxy or a tunnel;
#   - this computer only, built from the source with build/docker/compose.yaml:
#       docker compose -f build/docker/compose.yaml up -d --build
#
# It ships the wacli pinned in build/wacli.env, checked against its sha256,
# as the app does. Built for linux/amd64 and linux/arm64 (scripts/release.sh):
# the Go build runs on the builder's platform and cross-compiles.

FROM --platform=$BUILDPLATFORM golang:1.26-trixie AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG TAGS=""
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=linux GOARCH="${TARGETARCH:-amd64}" go build -tags "${TAGS}" -trimpath \
      -ldflags "-s -w -X main.version=${VERSION}" -o /out/whatsapp-mcp ./cmd/whatsapp-mcp \
 && scripts/fetch-wacli.sh linux "${TARGETARCH:-amd64}" /out

FROM debian:trixie-slim
ARG VERSION=dev
LABEL org.opencontainers.image.title="WhatsApp MCP Local" \
      org.opencontainers.image.description="WhatsApp for AI tools over MCP, on your own server" \
      org.opencontainers.image.source="https://github.com/BrOrlandi/whatsapp-mcp-local" \
      org.opencontainers.image.licenses="MIT" \
      org.opencontainers.image.version="${VERSION}"
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates tzdata \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --uid 1000 --home-dir /data --no-create-home whatsapp \
 && mkdir -p /data && chown whatsapp:whatsapp /data
COPY --from=build /out/whatsapp-mcp /out/wacli /usr/local/bin/
# Everything that must survive the container lives in /data: the WhatsApp
# session and messages (wacli), and the daemon's state, settings and keys.
# The public listener answers on every interface of the container, for the
# proxy beside it; it only starts with WHATSAPP_MCP_PUBLIC_URL set (the
# server mode), and the local listener stays on the container's loopback.
ENV HOME=/data \
    WHATSAPP_MCP_DATA=/data/mcp \
    WACLI_STORE_DIR=/data/wacli \
    WACLI_BIN=/usr/local/bin/wacli \
    WHATSAPP_MCP_CONTAINER=1 \
    WHATSAPP_MCP_PUBLIC_LISTEN=0.0.0.0:47822
VOLUME /data
EXPOSE 47822
USER whatsapp
# Whether the daemon answers; it says nothing of WhatsApp, which may still be
# waiting to be paired.
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD ["whatsapp-mcp", "healthcheck"]
ENTRYPOINT ["whatsapp-mcp"]
CMD ["serve"]
