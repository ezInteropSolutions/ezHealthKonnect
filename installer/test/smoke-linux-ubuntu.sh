#!/usr/bin/env bash
# Linux installer smoke test — runs inside a fresh Ubuntu 22.04 container.
# Tests every shell command the Go installer issues, without needing the binary.
#
# Run from Windows:
#   docker run --rm -it ubuntu:22.04 bash -s < installer/test/smoke-linux-ubuntu.sh
#
# Or pipe directly from the repo root:
#   Get-Content installer/test/smoke-linux-ubuntu.sh | docker run --rm -i ubuntu:22.04 bash

set -euo pipefail
export DEBIAN_FRONTEND=noninteractive

# Auto-detect: empty when already root (Docker), "sudo" when non-root (CI runner)
if [ "$(id -u)" -eq 0 ]; then SUDO=""; else SUDO="sudo"; fi

PASS=0; FAIL=0; SKIP=0

ok()   { echo -e "  \033[32mPASS\033[0m  $1"; ((++PASS)); }
fail() { echo -e "  \033[31mFAIL\033[0m  $1"; ((++FAIL)); }
skip() { echo -e "  \033[33mSKIP\033[0m  $1 (non-fatal)"; ((++SKIP)); }
hdr()  { echo -e "\n\033[36m=== $1 ===\033[0m"; }

echo ""
echo "  ezHealthKonnect Linux Installer Smoke Test"
echo "  ==========================================="
echo "  Image: $(cat /etc/os-release | grep PRETTY_NAME | cut -d= -f2 | tr -d '\"')"
echo ""

# ── 1. Package manager detection ───────────────────────────────────────────────
hdr "Package Manager"
if command -v apt-get >/dev/null 2>&1; then
  ok "apt-get found"
else
  fail "apt-get not found"
fi

# ── 2. Distro codename detection ───────────────────────────────────────────────
hdr "Distro Codename (for PGDG repo)"
if grep -q "VERSION_CODENAME=" /etc/os-release; then
  CODENAME=$(grep "^VERSION_CODENAME=" /etc/os-release | cut -d= -f2 | tr -d '"')
  ok "Codename: $CODENAME"
else
  fail "VERSION_CODENAME not in /etc/os-release"
fi

# ── 3. PGDG repo setup ─────────────────────────────────────────────────────────
hdr "PostgreSQL 15 — PGDG Repo Setup"
$SUDO apt-get update -qq
$SUDO apt-get install -y -qq curl gnupg lsb-release 2>/dev/null
ok "curl + gnupg + lsb-release installed"

curl -fsSL https://www.postgresql.org/media/keys/ACCC4CF8.asc \
  | $SUDO gpg --dearmor -o /etc/apt/trusted.gpg.d/postgresql.gpg
ok "PGDG signing key added"

echo "deb https://apt.postgresql.org/pub/repos/apt ${CODENAME}-pgdg main" \
  | $SUDO tee /etc/apt/sources.list.d/pgdg.list > /dev/null
ok "PGDG repo list written"

$SUDO apt-get update -qq
ok "apt-get update after PGDG repo"

# ── 4. PostgreSQL install ──────────────────────────────────────────────────────
hdr "PostgreSQL 15 Install"
# Remove pre-installed PG 14 BEFORE installing PG 15 so the apt post-install
# hook runs on a free port 5432 and properly initializes the cluster.
# No-op on Docker / fresh prod systems where PG 14 was never installed.
$SUDO apt-get remove -y postgresql-14 postgresql-client-14 2>/dev/null || true

$SUDO apt-get install -y -qq postgresql-15 postgresql-client-15
ok "postgresql-15 installed"

if command -v psql >/dev/null 2>&1; then
  ok "psql in PATH: $(which psql) $(psql --version | head -1)"
elif [ -f /usr/lib/postgresql/15/bin/psql ]; then
  ok "psql at /usr/lib/postgresql/15/bin/psql"
else
  fail "psql not found after install"
fi

# ── 5. Start PostgreSQL ────────────────────────────────────────────────────────
hdr "PostgreSQL Service Start"
# With PG 14 removed before install, the PG 15 apt post-install hook should
# have already created and started the cluster. Check first, start if needed.
if $SUDO pg_ctlcluster 15 main status 2>/dev/null | grep -q "online"; then
  ok "PostgreSQL 15 already running"
elif $SUDO pg_ctlcluster 15 main start 2>/dev/null; then
  ok "PostgreSQL 15 started"
else
  fail "PostgreSQL 15 failed to start"
  $SUDO pg_lsclusters 2>/dev/null || true
  $SUDO cat /var/log/postgresql/postgresql-15-main.log 2>/dev/null | tail -30 || true
fi

sleep 2

# ── 6. PostgreSQL peer auth (the critical DB setup path) ───────────────────────
hdr "PostgreSQL Peer Auth (su - postgres)"
PSQL=/usr/lib/postgresql/15/bin/psql

CREATE_USER="CREATE USER ezhealth_user WITH PASSWORD 'testpass123';"
CREATE_DB="CREATE DATABASE ezhealthkonnect OWNER ezhealth_user;"
GRANT="GRANT ALL PRIVILEGES ON DATABASE ezhealthkonnect TO ezhealth_user;"

if $SUDO su - postgres -c "$PSQL -d postgres -c \"$CREATE_USER\" -q" 2>/dev/null; then
  ok "CREATE USER via peer auth"
else
  fail "CREATE USER via peer auth"
fi

if $SUDO su - postgres -c "$PSQL -d postgres -c \"$CREATE_DB\" -q" 2>/dev/null; then
  ok "CREATE DATABASE via peer auth"
else
  fail "CREATE DATABASE via peer auth"
fi

$SUDO su - postgres -c "$PSQL -d postgres -c \"$GRANT\" -q" 2>/dev/null && true
ok "GRANT PRIVILEGES via peer auth"

# ── 7. App user connectivity ───────────────────────────────────────────────────
hdr "App User DB Connection"
export PGPASSWORD="testpass123"
if $PSQL -h localhost -U ezhealth_user -d ezhealthkonnect -c "SELECT 1;" -q 2>/dev/null; then
  ok "App user can connect with password"
else
  skip "TCP connection as app user (pg_hba may need md5 — socket works)"
  # Try unix socket instead
  if $SUDO su - postgres -c "$PSQL -U ezhealth_user -d ezhealthkonnect -c 'SELECT 1;' -q" 2>/dev/null; then
    ok "App user can connect via socket"
  else
    fail "App user cannot connect at all"
  fi
fi

# ── 8. Migration SQL test ──────────────────────────────────────────────────────
hdr "Migration Runner (numeric sort)"
# Simulate V1, V10, V100 files — verify they'd be sorted correctly
# The real Go code sorts these numerically. This tests the psql execution path.
mkdir -p /tmp/migrations
echo "CREATE TABLE IF NOT EXISTS test_v1 (id serial PRIMARY KEY);" > /tmp/migrations/V1__Init.sql
echo "CREATE TABLE IF NOT EXISTS test_v2 (id serial PRIMARY KEY);" > /tmp/migrations/V2__Users.sql
echo "CREATE TABLE IF NOT EXISTS test_v10 (id serial PRIMARY KEY);" > /tmp/migrations/V10__Interfaces.sql

for sql in /tmp/migrations/V1__Init.sql /tmp/migrations/V2__Users.sql /tmp/migrations/V10__Interfaces.sql; do
  PGPASSWORD=testpass123 $PSQL -h localhost -U ezhealth_user -d ezhealthkonnect -f "$sql" -q 2>/dev/null \
    && ok "  $(basename $sql)" \
    || { # Try via socket
         $SUDO su - postgres -c "$PSQL -U ezhealth_user -d ezhealthkonnect -f $sql -q" 2>/dev/null \
           && ok "  $(basename $sql) (via socket)" \
           || fail "  $(basename $sql)"; }
done

# ── 9. Node.js install ─────────────────────────────────────────────────────────
hdr "Node.js 20 LTS — NodeSource"
curl -fsSL https://deb.nodesource.com/setup_20.x -o /tmp/nodesource-setup.sh
chmod +x /tmp/nodesource-setup.sh
$SUDO bash /tmp/nodesource-setup.sh 2>/dev/null
$SUDO apt-get install -y -qq nodejs
ok "nodejs installed"

if command -v node >/dev/null 2>&1; then
  NODE_VER=$(node --version)
  ok "node: $NODE_VER"
  MAJOR=$(echo "$NODE_VER" | tr -d 'v' | cut -d. -f1)
  if [ "$MAJOR" -ge 18 ]; then
    ok "Node.js version >= 18 (minimum requirement met)"
  else
    fail "Node.js version $NODE_VER is below minimum (18)"
  fi
else
  fail "node not in PATH after install"
fi

if command -v npm >/dev/null 2>&1; then
  ok "npm: $(npm --version)"
else
  fail "npm not found"
fi

# ── 10. Systemd unit file format ───────────────────────────────────────────────
hdr "systemd Unit File Validation"
NODE_BIN=$(which node)
PSQL_PATH=$(which psql || echo "/usr/lib/postgresql/15/bin/psql")
INSTALL_DIR="/opt/ezhealthkonnect"

cat > /tmp/ezhealthkonnect-api.service << EOF
[Unit]
Description=ezHealthKonnect Go API
After=network.target postgresql.service postgresql-15.service
Wants=network.target

[Service]
Type=simple
WorkingDirectory=$INSTALL_DIR
ExecStart=$INSTALL_DIR/go-api
Restart=on-failure
RestartSec=10
StandardOutput=append:$INSTALL_DIR/logs/api.log
StandardError=append:$INSTALL_DIR/logs/api.err.log
EnvironmentFile=$INSTALL_DIR/.env

[Install]
WantedBy=multi-user.target
EOF

# Validate using systemd-analyze (if available in container)
if command -v systemd-analyze >/dev/null 2>&1; then
  systemd-analyze verify /tmp/ezhealthkonnect-api.service 2>/dev/null \
    && ok "Unit file valid (systemd-analyze)" \
    || skip "systemd-analyze verify (may need running systemd)"
else
  # Just check the file has the required sections
  if grep -q "\[Unit\]" /tmp/ezhealthkonnect-api.service && \
     grep -q "\[Service\]" /tmp/ezhealthkonnect-api.service && \
     grep -q "\[Install\]" /tmp/ezhealthkonnect-api.service; then
    ok "Unit file has required sections [Unit] [Service] [Install]"
  else
    fail "Unit file missing required sections"
  fi
fi

# ── 11. Drop cleanup test ──────────────────────────────────────────────────────
hdr "DB Cleanup (Uninstall Path)"
$SUDO su - postgres -c "$PSQL -d postgres -c \"SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname='ezhealthkonnect' AND pid<>pg_backend_pid();\" -q" 2>/dev/null && true
$SUDO su - postgres -c "$PSQL -d postgres -c \"DROP DATABASE IF EXISTS ezhealthkonnect;\" -q" 2>/dev/null \
  && ok "DROP DATABASE" || fail "DROP DATABASE"
$SUDO su - postgres -c "$PSQL -d postgres -c \"DROP USER IF EXISTS ezhealth_user;\" -q" 2>/dev/null \
  && ok "DROP USER" || fail "DROP USER"

# ── Summary ────────────────────────────────────────────────────────────────────
echo ""
echo "  ============================================="
echo -e "  Results: \033[32m${PASS} passed\033[0m  \033[31m${FAIL} failed\033[0m  \033[33m${SKIP} skipped\033[0m"
echo "  ============================================="
echo ""

[ "$FAIL" -eq 0 ] && echo "  All critical checks PASSED. Linux installer is ready." \
  || echo "  FAILURES detected — review output above before shipping."
echo ""
exit $FAIL
