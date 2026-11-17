#!/usr/bin/env bash
# Installs WhatsApp MCP in server mode with Docker (docs/servidor.md): asks the
# server's public address, the panel's password and how https arrives, writes
# the .env (readable only by you) beside the compose.yml, and starts it.
# Running it again changes the answers and keeps the data.
#
#   curl -fsSL https://raw.githubusercontent.com/BrOrlandi/whatsapp-mcp-local/main/install-docker.sh -o install-docker.sh
#   bash install-docker.sh
#
# Without questions, for automation, the answers come from the environment
# (the password from a file, never from an argument) and --yes accepts the
# rest:
#
#   WAMCP_DIR=/opt/whatsapp-mcp WAMCP_URL=https://whatsapp.example.com WAMCP_PROFILE=caddy \
#     WAMCP_PASSWORD_FILE=/root/panel-password bash install-docker.sh --yes
#
# WAMCP_PROFILE is tailscale (recommended), tunnel, caddy or proxy (a proxy you
# already run). tailscale takes its auth key from WAMCP_TS_AUTHKEY_FILE; tunnel
# takes its token from WAMCP_TUNNEL_TOKEN_FILE (a file, like the password, so
# the secret is not on a command line); caddy takes WAMCP_HTTP_PORT and
# WAMCP_HTTPS_PORT (default 80 and 443); proxy takes WAMCP_PUBLIC_PORT
# (default 47822).
# WAMCP_VERSION picks the image's tag (default latest); WAMCP_PULL=0 uses an
# image already on this computer, as a test of a local build does.
set -euo pipefail

REF="${WAMCP_REF:-main}"
RAW="${WAMCP_RAW:-https://raw.githubusercontent.com/BrOrlandi/whatsapp-mcp-local/$REF}"
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

# ask VAR "question" default: an answer from the environment wins; with --yes
# or without a terminal, the default; otherwise the person types it.
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

# from_file VAR FILE: the first line of FILE, exactly as written.
from_file() {
  local var="$1" file="$2" line
  [ -r "$file" ] || die "cannot read $file"
  IFS= read -r line < "$file" || [ -n "$line" ] || die "$file is empty"
  printf -v "$var" '%s' "$line"
}

# secret VAR "question": like ask, but nothing shows on the screen.
secret() {
  local var="$1" question="$2" answer
  if [ -n "${!var:-}" ]; then return; fi
  [ -t 0 ] || die "$var is needed: set it in the environment"
  # IFS= keeps spaces at either end: the secret is read exactly as typed.
  IFS= read -r -s -p "$question: " answer
  echo
  printf -v "$var" '%s' "$answer"
}

command -v docker >/dev/null || die "Docker is needed: https://docs.docker.com/engine/install/"
docker compose version >/dev/null 2>&1 || die "Docker Compose v2 is needed (docker compose)"
# compose.yml carries its configuration files inline (configs.content), which
# Compose reads from 2.23.1 on.
compose_version="$(docker compose version --short 2>/dev/null | sed 's/^v//')"
if [ "$(printf '%s\n%s\n' 2.23.1 "$compose_version" | sort -V | head -1)" != 2.23.1 ]; then
  die "Docker Compose $compose_version is too old: 2.23.1 or newer is needed"
fi

cat <<'EOF'

  WhatsApp MCP num servidor, com Docker.

  O WhatsApp roda num contêiner que fica ligado o tempo todo. O painel e o
  MCP ficam num endereço https, onde tudo pede a senha do painel, uma chave
  ou um login. Este instalador pergunta o endereço, a senha e como o https
  chega, e sobe tudo.

EOF

ask WAMCP_DIR "Pasta da instalação (configuração; os dados ficam num volume do Docker)" "$HOME/whatsapp-mcp"
mkdir -p "$WAMCP_DIR"
cd "$WAMCP_DIR"

# The compose.yml: from the checkout this script runs from, or from the
# repository at $REF. It is the one place that names the image.
here="$(cd "$(dirname "${BASH_SOURCE[0]}")" 2>/dev/null && pwd || true)"
if [ -n "$here" ] && [ -f "$here/compose.yml" ] && [ "$here" != "$PWD" ]; then
  cp "$here/compose.yml" compose.yml
elif [ ! -f compose.yml ] || [ "${WAMCP_UPDATE_COMPOSE:-1}" = 1 ]; then
  command -v curl >/dev/null || die "curl is needed to download compose.yml"
  # A failed download stops here: an old compose.yml is never used silently.
  curl -fsSL -o compose.yml.new "$RAW/compose.yml" || { rm -f compose.yml.new; die "could not download $RAW/compose.yml"; }
  mv compose.yml.new compose.yml
fi
# shellcheck disable=SC2016 # the ${...} is compose's, matched literally
image="$(sed -n 's/.*image: \${WHATSAPP_MCP_IMAGE:-\([^}]*\)}:.*/\1/p' compose.yml | head -1)"
[ -n "$image" ] || die "compose.yml does not name the image"

# The answers kept from a previous run.
old() { [ -f .env ] && sed -n "s/^$1=//p" .env | head -1 | sed "s/^'\(.*\)'$/\1/" || true; }

# The image's tag: the one chosen before, unless WAMCP_VERSION says another.
WAMCP_VERSION="${WAMCP_VERSION:-$(old WHATSAPP_MCP_VERSION)}"

ask WAMCP_URL "Endereço público, com https:// (o do Tailscale, do seu domínio...)" "$(old WHATSAPP_MCP_PUBLIC_URL)"
WAMCP_URL="${WAMCP_URL%/}"
# A strict shape: the address goes into .env and into Caddy's configuration,
# so nothing but a scheme, a host name, an optional port and a plain path.
url_shape='^https?://[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?)*(:[0-9]{1,5})?(/[A-Za-z0-9._~/-]*)?$'
[[ "$WAMCP_URL" =~ $url_shape ]] || die "o endereço precisa ser como https://whatsapp.example.com (sem espaços, aspas ou outros sinais)"
case "$WAMCP_URL" in
  https://*) ;;
  *) warn "http sem s: o navegador manda a senha sem criptografia. Use só para teste." ;;
esac
host="${WAMCP_URL#*://}"
host="${host%%/*}"
hostname_only="${host%%:*}"

# token_shape NAME VALUE: a key or token goes into .env between single
# quotes, so it may hold only the characters such tokens are made of.
token_shape() {
  [[ "$2" =~ ^[A-Za-z0-9._=+/-]+$ ]] || die "$1 has characters a token does not have"
}

current_profile="$(old COMPOSE_PROFILES)"
[ -n "$current_profile" ] || { [ -f compose.override.yml ] && current_profile=proxy; }
ask WAMCP_PROFILE "Como o https chega: tailscale (recomendado), tunnel (Cloudflare), caddy (domínio apontado para cá) ou proxy (um proxy que você já tem)" "${current_profile:-tailscale}"

subnet="${WAMCP_SUBNET:-$(old WHATSAPP_MCP_SUBNET)}"
subnet="${subnet:-172.30.247.0/24}"
gateway="${subnet%.*}.1"
profile="" trusted="" extra=()
case "$WAMCP_PROFILE" in
  tailscale)
    profile=tailscale trusted=tailscale
    [ -z "${WAMCP_TS_AUTHKEY_FILE:-}" ] || from_file WAMCP_TS_AUTHKEY "$WAMCP_TS_AUTHKEY_FILE"
    WAMCP_TS_AUTHKEY="${WAMCP_TS_AUTHKEY:-$(old TS_AUTHKEY)}"
    secret WAMCP_TS_AUTHKEY "Chave de autenticação do Tailscale (tskey-auth-..., não aparece na tela)"
    token_shape WAMCP_TS_AUTHKEY "$WAMCP_TS_AUTHKEY"
    extra+=("TS_AUTHKEY='$WAMCP_TS_AUTHKEY'" "TS_HOSTNAME=${hostname_only%%.*}")
    ;;
  tunnel)
    profile=tunnel trusted=cloudflared
    [ -z "${WAMCP_TUNNEL_TOKEN_FILE:-}" ] || from_file WAMCP_TUNNEL_TOKEN "$WAMCP_TUNNEL_TOKEN_FILE"
    WAMCP_TUNNEL_TOKEN="${WAMCP_TUNNEL_TOKEN:-$(old TUNNEL_TOKEN)}"
    secret WAMCP_TUNNEL_TOKEN "Token do túnel da Cloudflare (não aparece na tela)"
    token_shape WAMCP_TUNNEL_TOKEN "$WAMCP_TUNNEL_TOKEN"
    extra+=("TUNNEL_TOKEN='$WAMCP_TUNNEL_TOKEN'")
    ;;
  caddy)
    profile=caddy trusted=caddy
    # The site goes into the Caddyfile: a host name and nothing else.
    [[ "$hostname_only" =~ ^[A-Za-z0-9]([A-Za-z0-9.-]*[A-Za-z0-9])?$ ]] || die "the host name $hostname_only is not one Caddy can take"
    extra+=("WHATSAPP_MCP_SITE=$hostname_only")
    # Ports 80 and 443 by default; other ones only when told.
    http_port="${WAMCP_HTTP_PORT:-$(old WHATSAPP_MCP_HTTP_PORT)}"
    https_port="${WAMCP_HTTPS_PORT:-$(old WHATSAPP_MCP_HTTPS_PORT)}"
    [ -z "$http_port" ] || extra+=("WHATSAPP_MCP_HTTP_PORT=$http_port")
    [ -z "$https_port" ] || extra+=("WHATSAPP_MCP_HTTPS_PORT=$https_port")
    ;;
  proxy)
    trusted="$gateway"
    ask WAMCP_PUBLIC_PORT "Porta em 127.0.0.1 deste servidor para o seu proxy encaminhar" "$(old WHATSAPP_MCP_PUBLIC_PORT || true)"
    WAMCP_PUBLIC_PORT="${WAMCP_PUBLIC_PORT:-47822}"
    extra+=("WHATSAPP_MCP_PUBLIC_PORT=$WAMCP_PUBLIC_PORT")
    ;;
  *) die "WAMCP_PROFILE must be tailscale, tunnel, caddy or proxy" ;;
esac

# The panel's password: kept from before unless a new one is asked for. It is
# never shown, never an argument; only its bcrypt hash is written.
hash="$(old WHATSAPP_MCP_PANEL_PASSWORD_HASH)"
if [ -z "$hash" ] || [ "${WAMCP_NEW_PASSWORD:-0}" = 1 ]; then
  password="" again=""
  if [ -n "${WAMCP_PASSWORD_FILE:-}" ]; then
    from_file password "$WAMCP_PASSWORD_FILE"
  else
    cat <<'EOF'

  A senha do painel protege as suas conversas na internet. Use uma senha
  longa e aleatória, de um gerenciador de senhas (16 caracteres ou mais):
  qualquer pessoa pode tentar adivinhá-la, mesmo que devagar. O mínimo
  aceito, 10 caracteres, é um piso, não uma recomendação.

EOF
    secret password "Senha do painel"
    secret again "De novo, para conferir"
    [ "$password" = "$again" ] || die "as duas senhas não são iguais"
  fi
  [ "${#password}" -ge 10 ] || die "a senha precisa ter ao menos 10 caracteres"
  if [ "${WAMCP_PULL:-1}" = 1 ]; then
    say "pulling $image:${WAMCP_VERSION:-latest}"
    docker pull -q "$image:${WAMCP_VERSION:-latest}" >/dev/null
  fi
  hash="$(printf '%s\n' "$password" | docker run --rm -i "$image:${WAMCP_VERSION:-latest}" password hash)"
  unset password again
fi
case "$hash" in \$2[aby]\$*) ;; *) die "could not make the password's hash" ;; esac

ask WAMCP_LANG "Idioma do painel (pt-BR, es, en)" "$(old WHATSAPP_MCP_LANG || true)"
ask WAMCP_TZ "Fuso horário" "$(old TZ || true)"

say "writing $WAMCP_DIR/.env (readable only by you)"
(
  umask 077
  {
    echo "# Written by install-docker.sh; run it again to change the answers."
    echo "COMPOSE_PROFILES=$profile"
    echo "WHATSAPP_MCP_VERSION=${WAMCP_VERSION:-latest}"
    echo "WHATSAPP_MCP_PUBLIC_URL=$WAMCP_URL"
    echo "WHATSAPP_MCP_PANEL_PASSWORD_HASH='$hash'"
    echo "# The only peer whose X-Forwarded-For and -Proto are believed."
    echo "WHATSAPP_MCP_TRUSTED_PROXY=$trusted"
    echo "WHATSAPP_MCP_SUBNET=$subnet"
    echo "WHATSAPP_MCP_GATEWAY=$gateway"
    echo "WHATSAPP_MCP_LANG=${WAMCP_LANG:-pt-BR}"
    echo "TZ=${WAMCP_TZ:-America/Sao_Paulo}"
    for line in "${extra[@]}"; do echo "$line"; done
  } > .env.new
  mv .env.new .env
)
chmod 600 .env

# compose.override.yml publishes the public listener on loopback, for the
# proxy option only. One this script did not write is left alone.
marker="# Written by install-docker.sh for a proxy you already run"
ours=0
{ [ ! -f compose.override.yml ] || head -1 compose.override.yml | grep -qF "$marker"; } && ours=1
if [ "$WAMCP_PROFILE" = proxy ] && [ "$ours" = 1 ]; then
  cat > compose.override.yml <<'EOF'
# Written by install-docker.sh for a proxy you already run: the public
# listener on this server's loopback only, for that proxy to reach. Nothing
# else on the network reaches it.
services:
  whatsapp-mcp:
    ports:
      - 127.0.0.1:${WHATSAPP_MCP_PUBLIC_PORT:-47822}:47822
EOF
elif [ "$ours" = 1 ]; then
  rm -f compose.override.yml
else
  warn "compose.override.yml was not written by this script: left as it is"
fi

# A previous option's container (the tunnel, Caddy...) goes when another is
# chosen.
for pair in tailscale:tailscale tunnel:cloudflared caddy:caddy; do
  if [ "${pair%%:*}" != "$profile" ]; then
    docker compose --profile "${pair%%:*}" rm -sf "${pair#*:}" >/dev/null 2>&1 || true
  fi
done

say "starting"
if [ "${WAMCP_PULL:-1}" = 1 ]; then
  docker compose pull -q
fi
docker compose up -d --remove-orphans
for _ in $(seq 1 60); do
  [ "$(docker compose ps --format '{{.Health}}' whatsapp-mcp 2>/dev/null)" = healthy ] && break
  sleep 1
done
[ "$(docker compose ps --format '{{.Health}}' whatsapp-mcp)" = healthy ] || warn "the container is not healthy yet: docker compose logs whatsapp-mcp"

echo
say "pronto: $WAMCP_URL"
case "$WAMCP_PROFILE" in
  tailscale) cat <<EOF
  No painel do Tailscale, ligue o Funnel para este servidor (Access controls,
  atributo "funnel") e confira que o nome dele é $hostname_only.
EOF
  ;;
  tunnel) cat <<EOF
  No painel da Cloudflare, o hostname público do túnel ($hostname_only)
  precisa apontar para o serviço http://whatsapp-mcp:47822.
EOF
  ;;
  caddy) cat <<EOF
  O domínio $hostname_only precisa apontar para este servidor, com as portas
  80 e 443 abertas; o certificado sai sozinho no primeiro acesso.
EOF
  ;;
  proxy) cat <<EOF
  Aponte o seu proxy para http://127.0.0.1:$WAMCP_PUBLIC_PORT. Ele precisa
  trocar (ou completar) o X-Forwarded-For com o endereço de quem visita e
  mandar X-Forwarded-Proto: https. No nginx:

    location / {
      proxy_pass http://127.0.0.1:$WAMCP_PUBLIC_PORT;
      proxy_set_header Host \$host;
      proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
      proxy_set_header X-Forwarded-Proto https;
      proxy_read_timeout 15m;
    }
EOF
  ;;
esac
cat <<EOF

  Depois:
  1. Abra $WAMCP_URL, entre com a senha e conecte o WhatsApp pelo QR code
     (ou: docker compose exec whatsapp-mcp whatsapp-mcp pair).
  2. Em Conexões, crie uma chave, ou cole $WAMCP_URL/mcp no conector do
     Claude ou do ChatGPT. O guia: docs/servidor.md.
EOF
