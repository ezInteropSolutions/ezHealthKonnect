#!/usr/bin/env bash
# Linux installer smoke test — Rocky Linux 9 (RHEL-compatible).
# Tests the dnf + PGDG RPM path.
#
# Run from Windows:
#   docker run --rm -it rockylinux:9 bash -s < installer/test/smoke-linux-rocky.sh

set -euo pipefail

PASS=0; FAIL=0; SKIP=0
ok()   { echo -e "  \033[32mPASS\033[0m  $1"; ((++PASS)); }
fail() { echo -e "  \033[31mFAIL\033[0m  $1"; ((++FAIL)); }
skip() { echo -e "  \033[33mSKIP\033[0m  $1 (non-fatal)"; ((++SKIP)); }
hdr()  { echo -e "\n\033[36m=== $1 ===\033[0m"; }

echo ""
echo "  ezHealthKonnect Linux Installer Smoke Test (Rocky Linux)"
echo "  =========================================================="
echo "  Image: $(cat /etc/os-release | grep PRETTY_NAME | cut -d= -f2 | tr -d '\"')"
echo ""

# ── 1. Package manager ─────────────────────────────────────────────────────────
hdr "Package Manager"
command -v dnf >/dev/null && ok "dnf found" || fail "dnf not found"

# ── 2. EL version detection ────────────────────────────────────────────────────
hdr "EL Version Detection (for PGDG RPM URL)"
EL_VER="9"
if grep -q "^VERSION_ID=" /etc/os-release; then
  RAW=$(grep "^VERSION_ID=" /etc/os-release | cut -d= -f2 | tr -d '"')
  EL_VER="${RAW%%.*}"  # "9.3" → "9"
  ok "Detected EL version: $EL_VER"
else
  skip "VERSION_ID not found — defaulting to EL 9"
fi

ARCH="x86_64"
PGDG_RPM="https://download.postgresql.org/pub/repos/yum/reporpms/EL-${EL_VER}-${ARCH}/pgdg-redhat-repo-latest.noarch.rpm"
ok "PGDG RPM URL: $PGDG_RPM"

# ── 3. PGDG repo install ───────────────────────────────────────────────────────
hdr "PGDG RPM Repo"
dnf install -y -q "$PGDG_RPM" && ok "PGDG repo RPM installed" || fail "PGDG repo RPM install failed"
dnf -qy module disable postgresql 2>/dev/null && ok "postgresql module disabled" || skip "module disable (not applicable)"

# ── 4. PostgreSQL install ──────────────────────────────────────────────────────
hdr "PostgreSQL 15 Install"
dnf install -y -q postgresql15-server postgresql15-contrib \
  && ok "postgresql15-server installed" || fail "postgresql15-server install failed"

PSQL=/usr/pgsql-15/bin/psql
[ -f "$PSQL" ] && ok "psql at $PSQL" || fail "psql not at $PSQL"

# ── 5. initdb ─────────────────────────────────────────────────────────────────
hdr "PostgreSQL initdb"
PGDATA=/var/lib/pgsql/15/data
# postgresql-15-setup reads PGDATA from the systemd unit file, which does not
# exist in a Docker container. Call initdb directly instead.
if [ -f "$PGDATA/PG_VERSION" ]; then
  ok "initdb already done"
else
  runuser -l postgres -c "/usr/pgsql-15/bin/initdb -D $PGDATA" 2>/dev/null \
    && ok "initdb complete" || fail "initdb failed"
fi

# Start PostgreSQL — use runuser (no PAM needed in minimal containers)
if runuser -l postgres -c "/usr/pgsql-15/bin/pg_ctl -D $PGDATA -l /tmp/pg.log start -w" 2>/dev/null; then
  ok "PostgreSQL started"
else
  fail "PostgreSQL failed to start"
  tail -20 /tmp/pg.log 2>/dev/null || true
fi

sleep 2

# ── 6. Peer auth DB setup ─────────────────────────────────────────────────────
hdr "PostgreSQL Peer Auth"
runuser -l postgres -c "$PSQL -d postgres -c \"CREATE USER ezhealth_user WITH PASSWORD 'testpass';\" -q" 2>/dev/null \
  && ok "CREATE USER" || fail "CREATE USER"
runuser -l postgres -c "$PSQL -d postgres -c \"CREATE DATABASE ezhealthkonnect OWNER ezhealth_user;\" -q" 2>/dev/null \
  && ok "CREATE DATABASE" || fail "CREATE DATABASE"
runuser -l postgres -c "$PSQL -d postgres -c \"GRANT ALL PRIVILEGES ON DATABASE ezhealthkonnect TO ezhealth_user;\" -q" 2>/dev/null \
  && ok "GRANT PRIVILEGES" || skip "GRANT PRIVILEGES"

# ── 7. Node.js ────────────────────────────────────────────────────────────────
hdr "Node.js 20 LTS"
curl -fsSL https://rpm.nodesource.com/setup_20.x -o /tmp/ns-setup.sh && bash /tmp/ns-setup.sh 2>/dev/null \
  && ok "NodeSource RPM repo added" || skip "NodeSource setup (network)"
dnf install -y -q nodejs && ok "nodejs installed" || fail "nodejs install failed"
NODE_VER=$(node --version 2>/dev/null || echo "unknown")
ok "node: $NODE_VER"

# ── 8. Cleanup ────────────────────────────────────────────────────────────────
hdr "DB Cleanup"
runuser -l postgres -c "$PSQL -d postgres -c \"DROP DATABASE IF EXISTS ezhealthkonnect;\" -q" 2>/dev/null \
  && ok "DROP DATABASE" || fail "DROP DATABASE"
runuser -l postgres -c "$PSQL -d postgres -c \"DROP USER IF EXISTS ezhealth_user;\" -q" 2>/dev/null \
  && ok "DROP USER" || fail "DROP USER"

echo ""
echo "  ============================================="
echo -e "  Results: \033[32m${PASS} passed\033[0m  \033[31m${FAIL} failed\033[0m  \033[33m${SKIP} skipped\033[0m"
echo "  ============================================="
echo ""
[ "$FAIL" -eq 0 ] && echo "  Rocky Linux path PASSED." || echo "  FAILURES — review above."
exit $FAIL
