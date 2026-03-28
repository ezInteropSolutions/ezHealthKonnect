#!/usr/bin/env bash
# ezHealthKonnect — Docker Edition Installer (Linux / macOS)
#
# Usage:
#   bash install.sh                     # default install to /opt/ezhealthkonnect
#   bash install.sh --dir ~/ezhk        # custom install directory
#   bash install.sh --reconfigure       # re-enter all config values
#   bash install.sh --with-ai           # also start Ollama LLM (~2.3 GB)
#
# Requirements: Docker Engine 24+ with Compose v2 plugin

set -euo pipefail

# ── Defaults ──────────────────────────────────────────────────────────────────
INSTALL_DIR="/opt/ezhealthkonnect"
RECONFIGURE=false
WITH_AI=false

# ── Argument parsing ──────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
    case "$1" in
        --dir)          INSTALL_DIR="$2"; shift 2 ;;
        --reconfigure)  RECONFIGURE=true;  shift   ;;
        --with-ai)      WITH_AI=true;      shift   ;;
        *)              echo "Unknown option: $1"; exit 1 ;;
    esac
done

# ── Colours ───────────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; WHITE='\033[1;37m'; GRAY='\033[0;37m'; RESET='\033[0m'

info()   { echo -e "  ${CYAN}$*${RESET}"; }
ok()     { echo -e "  ${GREEN}[OK]${RESET} $*"; }
warn()   { echo -e "  ${YELLOW}[!] $*${RESET}"; }
fail()   { echo -e "  ${RED}[X] $*${RESET}"; exit 1; }
header() { echo -e "\n${CYAN}$(printf '=%.0s' {1..60})${RESET}\n  ${WHITE}$*${RESET}\n${CYAN}$(printf '=%.0s' {1..60})${RESET}"; }

prompt_required() {
    local val=""
    while [[ -z "$val" ]]; do
        read -rp "  $1: " val
    done
    echo "$val"
}

prompt_secret() {
    local val=""
    while [[ -z "$val" ]]; do
        read -rsp "  $1: " val; echo
    done
    echo "$val"
}

random_secret() {
    # 48 bytes → 64-char base64
    openssl rand -base64 48 2>/dev/null || \
        head -c 48 /dev/urandom | base64 | tr -d '\n'
}

# ── Banner ────────────────────────────────────────────────────────────────────
clear
echo ""
echo -e "${CYAN}  ╔══════════════════════════════════════════════════╗${RESET}"
echo -e "${WHITE}  ║      ezHealthKonnect — Docker Edition            ║${RESET}"
echo -e "${GRAY}  ║      Healthcare Integration Platform             ║${RESET}"
echo -e "${GRAY}  ║      © 2025-2026 ezInterop Solutions             ║${RESET}"
echo -e "${CYAN}  ╚══════════════════════════════════════════════════╝${RESET}"
echo ""

# ── Step 1: Prerequisites ─────────────────────────────────────────────────────
header "Step 1 of 4 — Checking Prerequisites"

# Docker
if ! docker info &>/dev/null; then
    fail "Docker is not running or not installed.\n\n  Install guide: https://docs.docker.com/get-docker/"
fi
DOCKER_VER=$(docker version --format '{{.Server.Version}}' 2>/dev/null || echo "unknown")
ok "Docker Engine $DOCKER_VER"

# Docker Compose v2
if ! docker compose version &>/dev/null; then
    fail "Docker Compose v2 not found. Update Docker or install the compose plugin:\n  https://docs.docker.com/compose/install/"
fi
COMPOSE_VER=$(docker compose version --short 2>/dev/null || echo "unknown")
ok "Docker Compose $COMPOSE_VER"

# openssl (for secret generation)
if ! command -v openssl &>/dev/null; then
    warn "openssl not found — secrets will use /dev/urandom fallback."
fi

# ── Step 2: Install directory ─────────────────────────────────────────────────
header "Step 2 of 4 — Install Location"

mkdir -p "$INSTALL_DIR"
ok "Install directory: $INSTALL_DIR"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

if [[ "$SCRIPT_DIR" != "$INSTALL_DIR" ]]; then
    info "Copying application files to $INSTALL_DIR ..."

    FILES=(Dockerfile docker-compose.prod.yml docker-compose.yml
           package.json package-lock.json go.mod go.sum app.js server.js main.go)
    DIRS=(controllers services middleware routes config models
          processing utils public database schemas)

    for f in "${FILES[@]}"; do
        [[ -f "$SCRIPT_DIR/$f" ]] && cp "$SCRIPT_DIR/$f" "$INSTALL_DIR/"
    done
    for d in "${DIRS[@]}"; do
        [[ -d "$SCRIPT_DIR/$d" ]] && cp -r "$SCRIPT_DIR/$d" "$INSTALL_DIR/"
    done
    ok "Files copied."
fi

cd "$INSTALL_DIR"

# ── Step 3: Configuration ─────────────────────────────────────────────────────
header "Step 3 of 4 — Configuration"

ENV_FILE="$INSTALL_DIR/.env"

if [[ -f "$ENV_FILE" ]] && [[ "$RECONFIGURE" == false ]]; then
    ok ".env already exists — skipping (run with --reconfigure to change)."
else
    echo ""
    echo -e "  ${WHITE}A few questions to configure your installation.${RESET}"
    echo -e "  ${GRAY}Press Enter to accept the default shown in [brackets].${RESET}"
    echo ""

    # App port
    read -rp "  Web UI port [3000]: " APP_PORT_RAW
    APP_PORT="${APP_PORT_RAW:-3000}"

    # Database password
    echo ""
    echo -e "  ${WHITE}Choose a strong database password:${RESET}"
    DB_PASSWORD=$(prompt_secret "Database password")

    # MinIO password
    echo ""
    echo -e "  ${WHITE}Choose a storage password (MinIO — for message archiving):${RESET}"
    MINIO_PASSWORD=$(prompt_secret "Storage password")

    # Auto-generate secrets
    SESSION_SECRET=$(random_secret)
    JWT_SECRET=$(random_secret)

    cat > "$ENV_FILE" <<EOF
# ezHealthKonnect — Production Environment
# Generated by install.sh on $(date '+%Y-%m-%d %H:%M')
# Keep this file private — it contains secrets.

# ── Application ───────────────────────────────────────
APP_IMAGE=ezhealthkonnect/app:latest
APP_PORT=${APP_PORT}
API_PORT=8080

# ── Database (PostgreSQL) ─────────────────────────────
DB_USER=ezhealth_user
DB_PASSWORD=${DB_PASSWORD}
DB_PORT=5432
DB_SSL=false

# ── Object Storage (MinIO) ────────────────────────────
MINIO_USER=ezhealth_user
MINIO_PASSWORD=${MINIO_PASSWORD}
MINIO_API_PORT=9000
MINIO_CONSOLE_PORT=9001

# ── Secrets (auto-generated — do not share) ───────────
SESSION_SECRET=${SESSION_SECRET}
JWT_SECRET=${JWT_SECRET}

# ── AI / Ollama (only used with --with-ai flag) ────────
OLLAMA_PORT=11434
OLLAMA_CHAT_MODEL=llama3.2:3b
OLLAMA_EMBED_MODEL=nomic-embed-text
EOF

    chmod 600 "$ENV_FILE"
    ok ".env written to $ENV_FILE"
fi

# Read port back from .env for the final message
APP_PORT=$(grep -E '^APP_PORT=' "$ENV_FILE" | cut -d= -f2 | tr -d ' ' || echo "3000")

# ── Step 4: Build and Start ───────────────────────────────────────────────────
header "Step 4 of 4 — Building and Starting ezHealthKonnect"

echo ""
info "Building application image (3–6 minutes on first run)..."
echo ""

docker build -t ezhealthkonnect/app:latest "$INSTALL_DIR"

echo ""
info "Starting services..."
echo ""

COMPOSE_ARGS=(-f "$INSTALL_DIR/docker-compose.prod.yml" --env-file "$ENV_FILE")
if [[ "$WITH_AI" == true ]]; then
    COMPOSE_ARGS+=(--profile ai)
fi

docker compose "${COMPOSE_ARGS[@]}" up -d --remove-orphans

# ── Done ──────────────────────────────────────────────────────────────────────
URL="http://localhost:${APP_PORT}"

echo ""
echo -e "${GREEN}$(printf '=%.0s' {1..60})${RESET}"
echo -e "${GREEN}  ezHealthKonnect is starting up!${RESET}"
echo -e "${GREEN}$(printf '=%.0s' {1..60})${RESET}"
echo ""
echo -e "  Services are initialising (usually 20–30 seconds)."
echo ""
echo -e "  ${CYAN}Platform URL :${RESET} $URL"
echo -e "  ${CYAN}Install dir  :${RESET} $INSTALL_DIR"
echo -e "  ${CYAN}Config file  :${RESET} $ENV_FILE"
echo ""
echo -e "  On first visit you will be guided through a short setup"
echo -e "  wizard to create your administrator account."
echo ""

if [[ "$WITH_AI" == true ]]; then
    echo -e "  ${YELLOW}AI (ezCompanion) is enabled. Pulling model in background...${RESET}"
    echo -e "  ${YELLOW}This is a ~2.3 GB download and may take several minutes.${RESET}"
    echo ""
    (sleep 30 && \
     docker exec ezhk-ollama ollama pull llama3.2:3b && \
     docker exec ezhk-ollama ollama pull nomic-embed-text) &>/dev/null &
fi

echo -e "  ${GRAY}To stop:    docker compose -f docker-compose.prod.yml down${RESET}"
echo -e "  ${GRAY}To restart: docker compose -f docker-compose.prod.yml up -d${RESET}"
echo -e "  ${GRAY}Logs:       docker compose -f docker-compose.prod.yml logs -f app${RESET}"
echo ""

# Try to open browser (best-effort)
sleep 5
if command -v xdg-open &>/dev/null; then
    xdg-open "$URL" &>/dev/null &
elif command -v open &>/dev/null; then
    open "$URL" &>/dev/null &
else
    echo -e "  Open your browser and navigate to: ${CYAN}$URL${RESET}"
    echo ""
fi
