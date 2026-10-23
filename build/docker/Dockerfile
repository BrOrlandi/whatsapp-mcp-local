# WhatsApp MCP's command line in a container, for a server without a screen
# (docs/servidor.md). The container is the service: `serve` runs in it, and
# every other command runs beside it with `docker compose exec`.
#
#   docker compose -f build/docker/compose.yaml up -d --build
#   docker compose -f build/docker/compose.yaml exec whatsapp-mcp whatsapp-mcp pair
#
# It ships the wacli pinned in build/wacli.env, checked against its sha256,
# as the app does.

FROM golang:1.26-trixie AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
ARG TARGETARCH
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/whatsapp-mcp ./cmd/whatsapp-mcp \
 && scripts/fetch-wacli.sh linux "${TARGETARCH:-amd64}" /out

FROM debian:trixie-slim
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates tzdata \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --uid 1000 --home-dir /data --no-create-home whatsapp \
 && mkdir -p /data && chown whatsapp:whatsapp /data
COPY --from=build /out/whatsapp-mcp /out/wacli /usr/local/bin/
# Everything that must survive the container lives in /data: the WhatsApp
# session and messages (wacli), and the daemon's state, settings and token.
ENV HOME=/data \
    WHATSAPP_MCP_DATA=/data/mcp \
    WACLI_STORE_DIR=/data/wacli \
    WACLI_BIN=/usr/local/bin/wacli \
    WHATSAPP_MCP_CONTAINER=1
VOLUME /data
USER whatsapp
ENTRYPOINT ["whatsapp-mcp"]
CMD ["serve"]
