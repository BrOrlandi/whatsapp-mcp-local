#!/usr/bin/env bash
# Installs WhatsApp MCP on a server with Docker, asking what it needs to
# know, and leaves it ready for the app on another computer to pair with
# (docs/servidor.md). Running it again updates in place and keeps the data.
#
#   curl -fsSL https://raw.githubusercontent.com/BrOrlandi/whatsapp-mcp-local/main/install-docker.sh -o install-docker.sh
#   bash install-docker.sh
#
# Without questions, for automation, every answer comes from the
# environment and --yes accepts the rest:
#
#   WAMCP_DIR=/opt/whatsapp-mcp WAMCP_HOST=whatsapp-mcp.example.com WAMCP_PROXY=traefik \
#     WAMCP_TRAEFIK_NETWORK=proxy bash install-docker.sh --yes
#
# The remote connection is HTTPS only. This script never publishes a plain
# HTTP port for it: either an existing Traefik on a private Docker network
# reaches the container, or another proxy of yours reaches it on the
# server's loopback only.
set -euo pipefail

REPO="${WAMCP_REPO:-https://github.com/BrOrlandi/whatsapp-mcp-local.git}"
REF="${WAMCP_REF:-main}"
YES=0
for arg in "$@"; do
  case "$arg" in
    --yes|-y) YES=1 ;;
    *) echo "usage: install-docker.sh [--yes]" >&2; exit 2 ;;
  esac
done

say()  { printf '\033[1;32m==>\033[0m %s\n' "$*"; }
warn() { printf '\033[1;33m!!\033[0m %s\n' "$*" >&2; }
die()  { printf '\033[1;31mxx\033[0m %s\n' "$*" >&2; exit 1; }

# ask VAR "question" default: an answer from the environment wins; with
# --yes, the default; otherwise the person types it.
ask() {
  local var="$1" question="$2" default="${3:-}" answer
  if [ -n "${!var:-}" ]; then return; fi
  if [ "$YES" = 1 ] || [ ! -t 0 ]; then
    printf -v "$var" '%s' "$default"
    return
  fi
  read -r -p "$question [${default}]: " answer
  printf -v "$var" '%s' "${answer:-$default}"
}

command -v docker >/dev/null || die "Docker is needed: https://docs.docker.com/engine/install/"
docker compose version >/dev/null 2>&1 || die "Docker Compose v2 is needed (docker compose)"
command -v git >/dev/null || die "git is needed to build the image from the source"

cat <<'EOF'

  WhatsApp MCP num servidor, com Docker.

  O WhatsApp roda num contêiner que fica ligado o tempo todo. O app do seu
  computador vira o controle dele: o acesso é só por HTTPS, com uma camada a
  mais de criptografia de ponta a ponta entre o app e o servidor.

EOF

ask WAMCP_DIR "Pasta da instalação (dados e configuração)" "/opt/whatsapp-mcp"
ask WAMCP_HOST "Endereço HTTPS do servidor para o app (só o nome, sem https://; vazio = sem acesso remoto)" ""
WAMCP_PROXY="${WAMCP_PROXY:-}"
if [ -n "$WAMCP_HOST" ]; then
  default_proxy=other
  if docker ps --format '{{.Image}} {{.Names}}' 2>/dev/null | grep -qi traefik; then default_proxy=traefik; fi
  ask WAMCP_PROXY "Quem faz o HTTPS: traefik (um Traefik já neste Docker) ou other (outro proxy seu, que alcança 127.0.0.1)" "$default_proxy"
  case "$WAMCP_PROXY" in
    traefik)
      ask WAMCP_TRAEFIK_NETWORK "Rede Docker do Traefik" "proxy"
      ask WAMCP_TRAEFIK_ENTRYPOINT "Entrypoint HTTPS do Traefik" "websecure"
      ask WAMCP_TRAEFIK_CERTRESOLVER "certresolver do Traefik (vazio se o certificado já vale para o domínio)" ""
      docker network inspect "$WAMCP_TRAEFIK_NETWORK" >/dev/null 2>&1 || die "the Docker network $WAMCP_TRAEFIK_NETWORK does not exist"
      ;;
    other)
      ask WAMCP_REMOTE_PORT "Porta em 127.0.0.1 deste servidor para o seu proxy (HTTPS) encaminhar" "47822"
      ;;
    *) die "WAMCP_PROXY must be traefik or other" ;;
  esac
fi
ask WAMCP_LANG "Idioma do painel (pt-BR, es, en)" "pt-BR"
ask WAMCP_TZ "Fuso horário" "$(cat /etc/timezone 2>/dev/null || echo America/Sao_Paulo)"

case "$WAMCP_HOST" in
  http://*|https://*) WAMCP_HOST="${WAMCP_HOST#*://}"; WAMCP_HOST="${WAMCP_HOST%%/*}" ;;
esac

DIR="$WAMCP_DIR"
if [ ! -d "$DIR" ]; then
  if ! mkdir -p "$DIR" 2>/dev/null; then
    say "creating $DIR with sudo"
    sudo mkdir -p "$DIR"
    sudo chown "$(id -u):$(id -g)" "$DIR"
  fi
fi
mkdir -p "$DIR/data"
# The container runs as the user 1000; the data folder is that user's, and
# nobody else's.
if [ "$(stat -c %u "$DIR/data" 2>/dev/null || stat -f %u "$DIR/data")" != 1000 ]; then
  sudo chown 1000:1000 "$DIR/data" 2>/dev/null || chown 1000:1000 "$DIR/data" || warn "set $DIR/data to the user 1000 (sudo chown 1000:1000 $DIR/data)"
fi
chmod 700 "$DIR/data" 2>/dev/null || sudo chmod 700 "$DIR/data"

say "fetching the source ($REF)"
if [ -d "$DIR/src/.git" ]; then
  git -C "$DIR/src" fetch --quiet --tags origin
  git -C "$DIR/src" checkout --quiet "$REF"
  git -C "$DIR/src" pull --quiet --ff-only origin "$REF" 2>/dev/null || true
else
  git clone --quiet "$REPO" "$DIR/src"
  git -C "$DIR/src" checkout --quiet "$REF"
fi
VERSION="$(git -C "$DIR/src" describe --tags --always 2>/dev/null || echo dev)"

say "writing $DIR/compose.yaml"
{
  cat <<EOF
# Written by install-docker.sh; run it again to change the answers.
services:
  whatsapp-mcp:
    build:
      context: ./src
      dockerfile: build/docker/Dockerfile
      args:
        VERSION: "$VERSION"
    image: whatsapp-mcp-local:$VERSION
    container_name: whatsapp-mcp
    restart: unless-stopped
    volumes:
      - ./data:/data
    environment:
      TZ: "$WAMCP_TZ"
      WHATSAPP_MCP_LANG: "$WAMCP_LANG"
EOF
  if [ -n "$WAMCP_HOST" ]; then
    cat <<EOF
      # The remote routes only, for the HTTPS proxy; the panel, the API and
      # the MCP stay on the container's loopback.
      WHATSAPP_MCP_REMOTE_LISTEN: "0.0.0.0:47822"
EOF
  fi
  cat <<'EOF'
    healthcheck:
      # 0 is ready and 4 not ready yet: the daemon answers. 3 is down.
      test: ["CMD-SHELL", "whatsapp-mcp status --json >/dev/null 2>&1; [ $$? -ne 3 ]"]
      interval: 30s
      timeout: 10s
      retries: 3
      start_period: 20s
EOF
  case "$WAMCP_PROXY" in
    traefik)
      cat <<EOF
    networks: [default, $WAMCP_TRAEFIK_NETWORK]
    labels:
      - "traefik.enable=true"
      - "traefik.docker.network=$WAMCP_TRAEFIK_NETWORK"
      - "traefik.http.routers.whatsapp-mcp.rule=Host(\`$WAMCP_HOST\`) && PathPrefix(\`/remote/v1/\`)"
      - "traefik.http.routers.whatsapp-mcp.entrypoints=$WAMCP_TRAEFIK_ENTRYPOINT"
      - "traefik.http.routers.whatsapp-mcp.tls=true"
EOF
      if [ -n "${WAMCP_TRAEFIK_CERTRESOLVER:-}" ]; then
        echo "      - \"traefik.http.routers.whatsapp-mcp.tls.certresolver=$WAMCP_TRAEFIK_CERTRESOLVER\""
      fi
      cat <<EOF
      - "traefik.http.services.whatsapp-mcp.loadbalancer.server.port=47822"

networks:
  $WAMCP_TRAEFIK_NETWORK:
    external: true
EOF
      ;;
    other)
      cat <<EOF
    # Only this server's loopback: your HTTPS proxy forwards to it and must
    # set X-Forwarded-Proto: https. Never publish it on another address.
    ports:
      - "127.0.0.1:$WAMCP_REMOTE_PORT:47822"
EOF
      ;;
  esac
} > "$DIR/compose.yaml"

say "building the image and starting the container (the first build takes a few minutes)"
(cd "$DIR" && docker compose up -d --build)

say "waiting for the daemon"
for _ in $(seq 1 60); do
  code=0
  docker exec whatsapp-mcp whatsapp-mcp status --json >/dev/null 2>&1 || code=$?
  [ "$code" != 3 ] && break
  sleep 2
done
[ "$code" != 3 ] || die "the daemon did not answer: (cd $DIR && docker compose logs)"

cat <<EOF

  Pronto. O WhatsApp MCP está rodando no contêiner whatsapp-mcp.

  1. Conecte o WhatsApp (QR code no terminal, ou --phone com DDI e DDD):
       docker exec -it whatsapp-mcp whatsapp-mcp pair
     Já tem os dados de outro computador? Siga docs/servidor.md, "Trazer os
     dados de outro computador", antes de conectar.
  2. Confira:
       docker exec whatsapp-mcp whatsapp-mcp status
EOF
if [ -n "$WAMCP_HOST" ]; then
  cat <<EOF
  3. Gere o código para o app (vale uma vez, por 10 minutos):
       docker exec whatsapp-mcp whatsapp-mcp remote code --name "Meu Mac"
     No app: Configurações › Acesso remoto › Usar um servidor, com
     https://$WAMCP_HOST e o código.
EOF
fi
cat <<EOF

  Atualizar depois: rode este script de novo.

EOF
