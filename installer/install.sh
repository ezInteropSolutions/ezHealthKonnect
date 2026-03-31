#!/usr/bin/env bash
# ezHealthKonnect — Standalone Linux Installer
#
# One-liner:  curl -fsSL https://releases.ezhealthkonnect.com/install.sh | sudo bash
# With flags: sudo bash install.sh --port 3000 --db-password mypass
#
# Options:
#   --install-dir DIR     Installation directory          (default: /opt/ezhealthkonnect)
#   --port PORT           Web UI port                     (default: 3000)
#   --api-port PORT       Go API port                     (default: 8080)
#   --db-port PORT        PostgreSQL port                 (default: 5432)
#   --db-password PASS    Database password               (auto-generated if omitted)
#   --with-ai             Install Ollama AI companion     (default: off)
#   --no-service          Skip systemd service setup      (default: register service)
#   --bundle PATH         Use local .tar.gz instead of downloading
#   --version VERSION     Release version to install      (default: latest)
#   --non-interactive     Never prompt — use all defaults

set -euo pipefail

# ── Colours ───────────────────────────────────────────────────────────────────
RED='\033[0;31m'; GREEN='\033[0;32m'; YELLOW='\033[1;33m'
CYAN='\033[0;36m'; BOLD='\033[1m'; DIM='\033[2m'; NC='\033[0m'

ok()   { echo -e "${GREEN}  ✔ ${NC}${*}"; }
info() { echo -e "${CYAN}  → ${NC}${*}"; }
warn() { echo -e "${YELLOW}  ⚠ ${NC}${*}"; }
err()  { echo -e "${RED}  ✖ ${NC}${*}" >&2; }
step() { echo -e "\n${BOLD}${CYAN}── Step ${1}:${NC} ${BOLD}${2}${NC}"; }
die()  { err "${*}"; exit 1; }

# ── Defaults ──────────────────────────────────────────────────────────────────
INSTALL_DIR=/opt/ezhealthkonnect
APP_PORT=3000
API_PORT=8080
DB_PORT=5432
DB_NAME=ezhealthkonnect
DB_USER=ezhealth_user
DB_PASS=""
WITH_AI=false
REGISTER_SVC=true
LOCAL_BUNDLE=""
VERSION=latest
NON_INTERACTIVE=false
RELEASES_BASE="https://github.com/ezinteropsolutions/ezhealthkonnect/releases"

# ── Arg parsing ───────────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case $1 in
    --install-dir)      INSTALL_DIR="$2";       shift 2 ;;
    --port)             APP_PORT="$2";           shift 2 ;;
    --api-port)         API_PORT="$2";           shift 2 ;;
    --db-port)          DB_PORT="$2";            shift 2 ;;
    --db-password)      DB_PASS="$2";            shift 2 ;;
    --with-ai)          WITH_AI=true;            shift   ;;
    --no-service)       REGISTER_SVC=false;      shift   ;;
    --bundle)           LOCAL_BUNDLE="$2";       shift 2 ;;
    --version)          VERSION="$2";            shift 2 ;;
    --non-interactive)  NON_INTERACTIVE=true;    shift   ;;
    *) die "Unknown option: $1. Run: bash install.sh --help" ;;
  esac
done

# ── Banner ────────────────────────────────────────────────────────────────────
print_banner() {
  echo -e "${CYAN}${BOLD}"
  echo "  ╔══════════════════════════════════════════════╗"
  echo "  ║         ezHealthKonnect Installer            ║"
  echo "  ║    HL7 ↔ FHIR Healthcare Integration        ║"
  echo "  ╚══════════════════════════════════════════════╝"
  echo -e "${NC}"
  echo -e "  ${DIM}Standalone Edition — no Docker required${NC}"
  echo ""
}

# ── Sanity checks ─────────────────────────────────────────────────────────────
check_root() {
  [[ $EUID -eq 0 ]] || die "This installer must be run as root.  Try: sudo bash install.sh"
}

detect_os() {
  [[ -f /etc/os-release ]] || die "Cannot detect OS — /etc/os-release not found"
  # shellcheck source=/dev/null
  . /etc/os-release
  OS_ID="${ID:-}"
  OS_ID_LIKE="${ID_LIKE:-}"
  OS_VERSION="${VERSION_ID:-}"

  if [[ "$OS_ID" == "ubuntu" || "$OS_ID" == "debian" || "$OS_ID_LIKE" == *"debian"* ]]; then
    PKG_MGR=apt
  elif [[ "$OS_ID" == "rhel"  || "$OS_ID" == "centos"    || "$OS_ID" == "rocky" || \
          "$OS_ID" == "almalinux" || "$OS_ID_LIKE" == *"rhel"* ]]; then
    PKG_MGR=dnf
    command -v dnf &>/dev/null || PKG_MGR=yum
  else
    die "Unsupported OS: ${OS_ID}. Supported: Ubuntu 20+, Debian 11+, RHEL/Rocky/AlmaLinux/CentOS 8+"
  fi

  ARCH=$(uname -m)
  case "$ARCH" in
    x86_64)  GOARCH=amd64  ;;
    aarch64) GOARCH=arm64  ;;
    *) die "Unsupported architecture: $ARCH" ;;
  esac

  ok "Detected: ${PRETTY_NAME:-$OS_ID} (${ARCH})"
}

check_disk_space() {
  local available
  available=$(df -m "${INSTALL_DIR%/*}" 2>/dev/null | awk 'NR==2{print $4}' || df -m / | awk 'NR==2{print $4}')
  if [[ "$available" -lt 2048 ]]; then
    warn "Low disk space: ${available}MB available (2048MB recommended)"
  else
    ok "Disk space: ${available}MB available"
  fi
}

# ── Interactive config collection ─────────────────────────────────────────────
collect_config() {
  # Already have everything we need
  [[ -n "$DB_PASS" ]] && return
  # Non-interactive or piped (no TTY) — use auto-generated values
  if [[ "$NON_INTERACTIVE" == "true" || ! -t 0 ]]; then
    DB_PASS=$(gen_secret | head -c 32)
    info "Non-interactive mode — using defaults, auto-generated DB password"
    return
  fi

  echo ""
  echo -e "  ${BOLD}Configuration${NC}  (press Enter to accept defaults)"
  echo ""

  local input
  read -rp "  Install directory  [${INSTALL_DIR}]: " input
  [[ -n "$input" ]] && INSTALL_DIR="$input"

  read -rp "  Web UI port        [${APP_PORT}]: " input
  [[ -n "$input" ]] && APP_PORT="$input"

  read -rp "  Go API port        [${API_PORT}]: " input
  [[ -n "$input" ]] && API_PORT="$input"

  read -rp "  PostgreSQL port    [${DB_PORT}]: " input
  [[ -n "$input" ]] && DB_PORT="$input"

  read -rsp "  Database password  [auto-generate]: " input; echo ""
  [[ -n "$input" ]] && DB_PASS="$input"

  read -rp "  Enable AI companion (Ollama)? [y/N]: " input
  [[ "$input" =~ ^[Yy] ]] && WITH_AI=true

  echo ""

  # Auto-generate password if still empty
  if [[ -z "$DB_PASS" ]]; then
    DB_PASS=$(gen_secret | head -c 32)
    info "Auto-generated database password (see .env after install)"
  fi
}

# ── Secrets ───────────────────────────────────────────────────────────────────
gen_secret() {
  openssl rand -base64 48 2>/dev/null || head -c 48 /dev/urandom | base64
}

# ── System packages ───────────────────────────────────────────────────────────
install_base_deps() {
  info "Updating package index..."
  if [[ "$PKG_MGR" == "apt" ]]; then
    DEBIAN_FRONTEND=noninteractive apt-get update -qq
    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq \
      curl wget ca-certificates gnupg lsb-release openssl tar
  else
    $PKG_MGR install -y -q curl wget ca-certificates gnupg openssl tar
  fi
  ok "Base packages ready"
}

install_postgres() {
  # Already installed?
  if command -v psql &>/dev/null; then
    local ver
    ver=$(psql --version | grep -oP '\d+' | head -1)
    if [[ "$ver" -ge 13 ]]; then
      ok "PostgreSQL ${ver} already installed"
      return
    fi
    warn "PostgreSQL ${ver} found but v13+ required — will install v15"
  fi

  info "Installing PostgreSQL 15..."
  if [[ "$PKG_MGR" == "apt" ]]; then
    curl -fsSL https://www.postgresql.org/media/keys/ACCC4CF8.asc \
      | gpg --dearmor -o /usr/share/keyrings/postgresql.gpg
    echo "deb [signed-by=/usr/share/keyrings/postgresql.gpg] \
https://apt.postgresql.org/pub/repos/apt $(lsb_release -cs)-pgdg main" \
      > /etc/apt/sources.list.d/pgdg.list
    DEBIAN_FRONTEND=noninteractive apt-get update -qq
    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq postgresql-15
  else
    local major_ver="${OS_VERSION%%.*}"
    # Install PGDG repo RPM
    $PKG_MGR install -y -q \
      "https://download.postgresql.org/pub/repos/yum/reporpms/EL-${major_ver}-x86_64/pgdg-redhat-repo-latest.noarch.rpm" \
      2>/dev/null || true
    $PKG_MGR module disable -y postgresql 2>/dev/null || true
    $PKG_MGR install -y -q postgresql15-server postgresql15
    /usr/pgsql-15/bin/postgresql-15-setup initdb 2>/dev/null || true
  fi

  systemctl enable --now postgresql   2>/dev/null || \
  systemctl enable --now postgresql-15 2>/dev/null || true

  ok "PostgreSQL installed and started"
}

install_nodejs() {
  # Already installed at v18+?
  if command -v node &>/dev/null; then
    local major
    major=$(node --version | grep -oP '\d+' | head -1)
    if [[ "$major" -ge 18 ]]; then
      ok "Node.js $(node --version) already installed"
      return
    fi
    warn "Node.js $(node --version) too old (v18+ required) — upgrading via NodeSource"
  fi

  info "Installing Node.js 20 LTS via NodeSource..."
  if [[ "$PKG_MGR" == "apt" ]]; then
    curl -fsSL https://deb.nodesource.com/setup_20.x | bash - >/dev/null 2>&1
    DEBIAN_FRONTEND=noninteractive apt-get install -y -qq nodejs
  else
    curl -fsSL https://rpm.nodesource.com/setup_20.x | bash - >/dev/null 2>&1
    $PKG_MGR install -y -q nodejs
  fi

  ok "Node.js $(node --version) installed"
}

# ── App bundle ────────────────────────────────────────────────────────────────
download_app() {
  mkdir -p "$INSTALL_DIR"

  if [[ -n "$LOCAL_BUNDLE" ]]; then
    info "Using local bundle: $LOCAL_BUNDLE"
    tar -xzf "$LOCAL_BUNDLE" -C "$INSTALL_DIR" --strip-components=1
    ok "Bundle extracted to $INSTALL_DIR"
    return
  fi

  local download_url
  if [[ "$VERSION" == "latest" ]]; then
    download_url="${RELEASES_BASE}/latest/download/ezhealthkonnect-linux-${GOARCH}.tar.gz"
  else
    download_url="${RELEASES_BASE}/download/${VERSION}/ezhealthkonnect-linux-${GOARCH}.tar.gz"
  fi

  local tmp_bundle="/tmp/ezhealthkonnect-bundle.tar.gz"
  info "Downloading ezHealthKonnect ${VERSION}..."
  if command -v curl &>/dev/null; then
    curl -fsSL --progress-bar "$download_url" -o "$tmp_bundle"
  else
    wget -q --show-progress "$download_url" -O "$tmp_bundle"
  fi

  tar -xzf "$tmp_bundle" -C "$INSTALL_DIR" --strip-components=1
  rm -f "$tmp_bundle"
  ok "Application extracted to $INSTALL_DIR"
}

install_npm_deps() {
  info "Installing Node.js dependencies..."
  cd "$INSTALL_DIR"
  npm install --omit=dev --silent 2>/dev/null || npm install --production
  ok "Node.js dependencies installed"
}

# ── Database setup ────────────────────────────────────────────────────────────
setup_database() {
  info "Creating PostgreSQL user and database..."

  # Create role if it doesn't exist
  sudo -u postgres psql -tc \
    "SELECT 1 FROM pg_roles WHERE rolname='${DB_USER}'" \
    | grep -q 1 || \
    sudo -u postgres psql -c \
      "CREATE USER ${DB_USER} WITH PASSWORD '${DB_PASS}';"

  # Create database if it doesn't exist
  sudo -u postgres psql -tc \
    "SELECT 1 FROM pg_database WHERE datname='${DB_NAME}'" \
    | grep -q 1 || \
    sudo -u postgres psql -c \
      "CREATE DATABASE ${DB_NAME} OWNER ${DB_USER};"

  sudo -u postgres psql -c \
    "GRANT ALL PRIVILEGES ON DATABASE ${DB_NAME} TO ${DB_USER};"
  sudo -u postgres psql -d "${DB_NAME}" -c \
    "GRANT ALL ON SCHEMA public TO ${DB_USER};" 2>/dev/null || true

  ok "Database '${DB_NAME}' ready"
}

run_migrations() {
  info "Running database migrations..."
  local conn="postgresql://${DB_USER}:${DB_PASS}@localhost:${DB_PORT}/${DB_NAME}"

  # Run each V*.sql file in version order via psql
  local migration_dir="${INSTALL_DIR}/database/migrations"
  if [[ -d "$migration_dir" ]]; then
    local count=0
    while IFS= read -r -d '' f; do
      psql "$conn" -f "$f" -q 2>/dev/null && count=$((count + 1)) || true
    done < <(find "$migration_dir" -name "V*.sql" -print0 | sort -zV)
    ok "Applied ${count} migration file(s)"
  else
    warn "No migrations directory found — skipping"
  fi
}

# ── Config ────────────────────────────────────────────────────────────────────
write_env() {
  local session_secret jwt_secret
  session_secret=$(gen_secret)
  jwt_secret=$(gen_secret)

  mkdir -p "$INSTALL_DIR"
  cat > "${INSTALL_DIR}/.env" <<EOF
# ezHealthKonnect — Production Environment
# Generated by installer on $(date '+%Y-%m-%d %H:%M')
# Keep this file private — it contains secrets.

PORT=${APP_PORT}
API_PORT=${API_PORT}

DB_HOST=localhost
DB_PORT=${DB_PORT}
DB_NAME=${DB_NAME}
DB_USER=${DB_USER}
DB_PASSWORD=${DB_PASS}
DB_SSL=false

SESSION_SECRET=${session_secret}
JWT_SECRET=${jwt_secret}

NODE_ENV=production
LOG_LEVEL=info

OBJECT_STORAGE_DRIVER=local
LOCAL_STORAGE_PATH=${INSTALL_DIR}/storage

AI_ENABLED=${WITH_AI}
OLLAMA_URL=http://localhost:11434
OLLAMA_CHAT_MODEL=llama3.2:3b
OLLAMA_EMBED_MODEL=nomic-embed-text
EOF

  chmod 600 "${INSTALL_DIR}/.env"
  ok "Configuration written to ${INSTALL_DIR}/.env"
}

# ── System user ───────────────────────────────────────────────────────────────
create_system_user() {
  if id ezhealthkonnect &>/dev/null; then
    ok "System user 'ezhealthkonnect' already exists"
  else
    useradd --system --no-create-home --shell /usr/sbin/nologin ezhealthkonnect
    ok "System user 'ezhealthkonnect' created"
  fi

  mkdir -p "${INSTALL_DIR}/storage" "${INSTALL_DIR}/logs"
  chown -R ezhealthkonnect:ezhealthkonnect "$INSTALL_DIR"
}

# ── Systemd ───────────────────────────────────────────────────────────────────
install_service() {
  local node_bin
  node_bin=$(command -v node)
  local go_api_bin="${INSTALL_DIR}/go-api"
  chmod +x "$go_api_bin" 2>/dev/null || true

  # Go API backend
  cat > /etc/systemd/system/ezhealthkonnect-api.service <<EOF
[Unit]
Description=ezHealthKonnect Go API Backend
After=network-online.target postgresql.service
Wants=network-online.target

[Service]
Type=simple
User=ezhealthkonnect
WorkingDirectory=${INSTALL_DIR}
EnvironmentFile=${INSTALL_DIR}/.env
ExecStart=${go_api_bin}
Restart=on-failure
RestartSec=5
StandardOutput=append:${INSTALL_DIR}/logs/api.log
StandardError=append:${INSTALL_DIR}/logs/api.log

[Install]
WantedBy=multi-user.target
EOF

  # Node.js frontend
  cat > /etc/systemd/system/ezhealthkonnect.service <<EOF
[Unit]
Description=ezHealthKonnect Web Frontend
After=network-online.target postgresql.service ezhealthkonnect-api.service
Requires=ezhealthkonnect-api.service

[Service]
Type=simple
User=ezhealthkonnect
WorkingDirectory=${INSTALL_DIR}
EnvironmentFile=${INSTALL_DIR}/.env
ExecStart=${node_bin} server.js
Restart=on-failure
RestartSec=5
StandardOutput=append:${INSTALL_DIR}/logs/app.log
StandardError=append:${INSTALL_DIR}/logs/app.log

[Install]
WantedBy=multi-user.target
EOF

  systemctl daemon-reload
  systemctl enable ezhealthkonnect-api ezhealthkonnect
  ok "Systemd services registered (auto-start on boot)"
}

start_services() {
  info "Starting Go API..."
  systemctl start ezhealthkonnect-api
  sleep 2
  info "Starting web frontend..."
  systemctl start ezhealthkonnect
  ok "Services started"
}

# ── AI companion ──────────────────────────────────────────────────────────────
install_ollama() {
  [[ "$WITH_AI" == "true" ]] || return
  info "Installing Ollama (AI companion)..."
  if curl -fsSL https://ollama.ai/install.sh | sh >/dev/null 2>&1; then
    systemctl enable --now ollama 2>/dev/null || true
    # Pull models in background — these are large, don't block install
    nohup ollama pull llama3.2:3b       >/dev/null 2>&1 &
    nohup ollama pull nomic-embed-text  >/dev/null 2>&1 &
    ok "Ollama installed — models downloading in background (~2.3 GB)"
  else
    warn "Ollama install failed — AI features won't be available. Install manually: https://ollama.ai"
  fi
}

# ── Health check ──────────────────────────────────────────────────────────────
health_check() {
  info "Waiting for application to become ready..."
  local url="http://localhost:${APP_PORT}/health"
  local deadline=$((SECONDS + 90))
  while [[ $SECONDS -lt $deadline ]]; do
    if curl -sf "$url" -o /dev/null 2>/dev/null; then
      ok "Application is ready"
      return
    fi
    sleep 3
  done
  warn "Health check timed out — the app may still be initialising"
  warn "Check: journalctl -u ezhealthkonnect -f"
}

# ── Done ──────────────────────────────────────────────────────────────────────
print_done() {
  local ip
  ip=$(hostname -I 2>/dev/null | awk '{print $1}' || echo "localhost")

  echo ""
  echo -e "${GREEN}${BOLD}"
  echo "  ╔══════════════════════════════════════════════╗"
  echo "  ║        Installation Complete!                ║"
  echo "  ╚══════════════════════════════════════════════╝"
  echo -e "${NC}"
  echo -e "  ${BOLD}Local URL:${NC}     ${CYAN}http://localhost:${APP_PORT}${NC}"
  [[ "$ip" != "localhost" ]] && \
  echo -e "  ${BOLD}Network URL:${NC}   ${CYAN}http://${ip}:${APP_PORT}${NC}"
  echo ""
  echo -e "  ${DIM}Default login:  admin@ezhealthkonnect.com / admin123${NC}"
  echo -e "  ${DIM}Change this immediately after first login.${NC}"
  echo ""
  echo -e "  ${DIM}Service commands:${NC}"
  echo -e "  ${DIM}  systemctl {start|stop|restart|status} ezhealthkonnect${NC}"
  echo -e "  ${DIM}  systemctl {start|stop|restart|status} ezhealthkonnect-api${NC}"
  echo -e "  ${DIM}  journalctl -u ezhealthkonnect -f        (live log)${NC}"
  echo ""
  echo -e "  ${YELLOW}⚠  Database password: ${BOLD}${DB_PASS}${NC}"
  echo -e "  ${YELLOW}   Stored in: ${INSTALL_DIR}/.env${NC}"
  echo ""
  if [[ "$WITH_AI" == "true" ]]; then
    echo -e "  ${DIM}AI models downloading in background (~2.3 GB) — AI features available once done.${NC}"
    echo ""
  fi
}

# ── Main ──────────────────────────────────────────────────────────────────────
main() {
  print_banner
  check_root
  detect_os
  check_disk_space
  collect_config

  step "1/8" "Installing system dependencies"
  install_base_deps
  install_postgres
  install_nodejs

  step "2/8" "Downloading application"
  download_app

  step "3/8" "Installing Node.js dependencies"
  install_npm_deps

  step "4/8" "Setting up database"
  setup_database

  step "5/8" "Writing configuration"
  write_env

  step "6/8" "Running database migrations"
  run_migrations

  step "7/8" "Creating system user and setting permissions"
  create_system_user

  step "8/8" "Registering system services"
  if [[ "$REGISTER_SVC" == "true" ]]; then
    install_service
    install_ollama
    start_services
    health_check
  else
    install_ollama
    info "Skipping service registration (--no-service flag)"
    info "To start manually:"
    info "  cd ${INSTALL_DIR} && source .env && ./go-api &"
    info "  cd ${INSTALL_DIR} && source .env && node server.js"
  fi

  print_done
}

main "$@"
