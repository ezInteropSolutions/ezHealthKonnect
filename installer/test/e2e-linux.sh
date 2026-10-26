#!/usr/bin/env bash
# End-to-end test of the REAL Standalone installer on a fresh Linux host
# (GitHub Actions ubuntu runner, run as root). Exercises: install, first-run
# setup with the one-time token, data creation, upgrade with a new migration,
# an upgrade whose migration fails (must roll back automatically), manual
# rollback, and uninstall — asserting real outcomes at every stage.
#
# usage: sudo installer/test/e2e-linux.sh <installer-binary> <v1-bundle.zip>
set -euo pipefail

INSTALLER=$(readlink -f "$1")
BUNDLE_V1=$(readlink -f "$2")
HERE=$(cd "$(dirname "$0")" && pwd)
WORK=$(mktemp -d)
DIR=/opt/ezhk-e2e
APP_PORT=3000
ADMIN_EMAIL=e2e-admin@example.org
ADMIN_PASSWORD="E2e-$(openssl rand -hex 12)"
COOKIES="$WORK/cookies.txt"

pass() { echo "  PASS  $*"; }
fail() { echo "  FAIL  $*" >&2; diagnostics; exit 1; }
step() { echo; echo "==== $*"; }
diagnostics() {
  echo "---- diagnostics"
  systemctl --no-pager status ezhealthkonnect-api ezhealthkonnect 2>&1 | tail -30 || true
  journalctl -u ezhealthkonnect-api -u ezhealthkonnect --no-pager -n 80 2>&1 || true
  tail -n 60 "$DIR/logs/application/app.log" 2>/dev/null || true
}
envget() { grep "^$1=" "$DIR/.env" | cut -d= -f2-; }
psql_app() { PGPASSWORD="$(envget DB_PASSWORD)" psql -X -q -t -A -h 127.0.0.1 -p "$(envget DB_PORT)" -U "$(envget DB_USER)" -d "$(envget DB_NAME)" -c "$1"; }
health() { curl -fsS "http://127.0.0.1:$APP_PORT/api/system/health" | grep -q '"status":"connected"'; }
version() { "$INSTALLER" status --dir "$DIR" | python3 -c 'import json,sys; print(json.load(sys.stdin)["version"])'; }
login() {
  rm -f "$COOKIES"
  curl -fsS -c "$COOKIES" -H 'Content-Type: application/json' \
    -d "{\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\"}" "http://127.0.0.1:$APP_PORT/api/auth/login" >/dev/null
}

step "Prepare upgrade-test bundles"
python3 "$HERE/make_variant_bundle.py" "$BUNDLE_V1" "$WORK/v2.zip" v0.0.2-ci \
  --add database/migrations/V90001__E2E_Upgrade_Marker.sql \
  "CREATE TABLE e2e_upgrade_marker (id int PRIMARY KEY); INSERT INTO e2e_upgrade_marker VALUES (1);"
python3 "$HERE/make_variant_bundle.py" "$WORK/v2.zip" "$WORK/v3.zip" v0.0.3-ci \
  --add database/migrations/V90002__E2E_Failing_Migration.sql \
  "CREATE TABLE e2e_must_not_exist (id int); SELECT 1/0;"

step "Install (headless)"
cat > "$WORK/answers.json" <<EOF
{"installDir":"$DIR","appPort":"$APP_PORT","apiPort":"8080","dbMode":"local"}
EOF
"$INSTALLER" install --config "$WORK/answers.json" | tee "$WORK/install.log" || fail "install exited non-zero"
health || fail "not healthy after install"
pass "installed and healthy ($(version))"

step "Security of the installation"
[ "$(stat -c '%U:%G %a' "$DIR/.env")" = "root:ezhealthkonnect 640" ] || fail ".env must be root:ezhealthkonnect 640, is $(stat -c '%U:%G %a' "$DIR/.env")"
[ "$(stat -c '%a' "$DIR")" = "750" ] || fail "install dir must be 750"
[ "$(stat -c '%U %a' "$DIR/backups")" = "root 700" ] || fail "backups must be root-only"
[ "$(stat -c '%U' "$DIR/go-api")" = "root" ] || fail "application files must be owned by root"
for exe in go-api node; do
  user=$(ps -o user= -C "$exe" | sort -u | head -1)
  [ "$user" = "ezhealthkonnect" ] || fail "$exe runs as '$user', expected ezhealthkonnect"
done
grep -q '^SETUP_TOKEN=' "$DIR/.env" || fail "no SETUP_TOKEN"
grep -q "setup.html?token=$(envget SETUP_TOKEN)" "$WORK/install.log" || fail "installer did not print the setup link"
pass "permissions, service account and setup token"

step "First-run setup requires the token"
code=$(curl -s -o /dev/null -w '%{http_code}' -H 'Content-Type: application/json' \
  -d "{\"orgName\":\"E2E\",\"firstName\":\"E\",\"lastName\":\"Tester\",\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\"}" \
  "http://127.0.0.1:$APP_PORT/api/setup/complete")
[ "$code" = "403" ] || fail "setup without token returned $code"
curl -fsS -H 'Content-Type: application/json' \
  -d "{\"orgName\":\"E2E\",\"firstName\":\"E\",\"lastName\":\"Tester\",\"email\":\"$ADMIN_EMAIL\",\"password\":\"$ADMIN_PASSWORD\",\"setupToken\":\"$(envget SETUP_TOKEN)\"}" \
  "http://127.0.0.1:$APP_PORT/api/setup/complete" >/dev/null || fail "setup with token failed"
login || fail "admin cannot log in"
pass "setup refused without token, completed with it, admin logs in"

step "Create data that upgrades must keep"
curl -fsS -b "$COOKIES" -H 'Content-Type: application/json' \
  -d '{"name":"E2E Interface","messageType":"ADT^A01","description":"e2e","sourceType":"","targetType":"","sourceConfig":{},"targetConfig":{}}' \
  "http://127.0.0.1:$APP_PORT/api/interfaces" >/dev/null || fail "create interface"
[ "$(psql_app "SELECT count(*) FROM interfaces WHERE name='E2E Interface'")" = "1" ] || fail "interface not stored"
echo "patient-data" > "$DIR/storage/e2e-object.txt"; chown ezhealthkonnect: "$DIR/storage/e2e-object.txt"
SECRETS_BEFORE=$(grep -E '^(DB_PASSWORD|SESSION_SECRET|JWT_SECRET|APP_CREDENTIAL_KEY)=' "$DIR/.env" | sha256sum)
pass "interface and stored object created"

step "Upgrade to v0.0.2-ci (adds a migration)"
"$INSTALLER" upgrade --dir "$DIR" --bundle "$WORK/v2.zip" || fail "upgrade to v2 failed"
[ "$(version)" = "v0.0.2-ci" ] || fail "version after upgrade: $(version)"
health || fail "not healthy after upgrade"
[ "$(psql_app 'SELECT count(*) FROM e2e_upgrade_marker')" = "1" ] || fail "new migration not applied"
[ "$(psql_app "SELECT count(*) FROM interfaces WHERE name='E2E Interface'")" = "1" ] || fail "data lost in upgrade"
[ "$(cat "$DIR/storage/e2e-object.txt")" = "patient-data" ] || fail "stored object lost"
[ "$(grep -E '^(DB_PASSWORD|SESSION_SECRET|JWT_SECRET|APP_CREDENTIAL_KEY)=' "$DIR/.env" | sha256sum)" = "$SECRETS_BEFORE" ] || fail "secrets changed"
login || fail "admin cannot log in after upgrade"
ls "$DIR/backups"/*/database.dump >/dev/null || fail "no upgrade backup"
pass "upgraded with data, secrets and login intact"

step "Upgrade to v0.0.3-ci with a failing migration must roll back"
if "$INSTALLER" upgrade --dir "$DIR" --bundle "$WORK/v3.zip" > "$WORK/v3.log" 2>&1; then
  cat "$WORK/v3.log"; fail "an upgrade with a failing migration must fail"
fi
grep -q "rolled back" "$WORK/v3.log" || { cat "$WORK/v3.log"; fail "failure was not rolled back"; }
[ "$(version)" = "v0.0.2-ci" ] || fail "version after failed upgrade: $(version)"
health || fail "previous version not healthy after automatic rollback"
[ "$(psql_app "SELECT to_regclass('e2e_must_not_exist') IS NULL")" = "t" ] || fail "failed migration left objects behind"
[ "$(psql_app 'SELECT count(*) FROM e2e_upgrade_marker')" = "1" ] || fail "data lost in automatic rollback"
login || fail "admin cannot log in after automatic rollback"
pass "failed upgrade rolled back automatically"

step "Manual rollback to v0.0.1-ci"
"$INSTALLER" rollback --dir "$DIR" --yes || fail "rollback failed"
[ "$(version)" = "v0.0.1-ci" ] || fail "version after rollback: $(version)"
health || fail "not healthy after rollback"
[ "$(psql_app "SELECT to_regclass('e2e_upgrade_marker') IS NULL")" = "t" ] || fail "database not restored to before the upgrade"
[ "$(psql_app "SELECT count(*) FROM interfaces WHERE name='E2E Interface'")" = "1" ] || fail "data from before the upgrade lost"
login || fail "admin cannot log in after rollback"
pass "rolled back to the previous version and database"

step "Uninstall"
DB_NAME=$(envget DB_NAME); DB_PORT=$(envget DB_PORT)
"$INSTALLER" uninstall --dir "$DIR" --yes || fail "uninstall failed"
[ ! -e "$DIR" ] || fail "install dir still present"
[ ! -e /etc/systemd/system/ezhealthkonnect-api.service ] || fail "service unit still present"
! id ezhealthkonnect >/dev/null 2>&1 || fail "service account still present"
[ "$(runuser -u postgres -- psql -X -t -A -p "$DB_PORT" -c "SELECT count(*) FROM pg_database WHERE datname='$DB_NAME'")" = "0" ] || fail "database not dropped"
pass "uninstalled completely"

# Uninstall followed by install must work: no leftover database in the way
# ("database ezhealthkonnect already exists").
step "Reinstall after uninstall"
"$INSTALLER" install --config "$WORK/answers.json" | tee "$WORK/reinstall.log" || fail "reinstall exited non-zero"
health || fail "not healthy after reinstall"
pass "clean reinstall works, no leftover database"
"$INSTALLER" uninstall --dir "$DIR" --yes || fail "uninstall after reinstall failed"
[ ! -e "$DIR" ] || fail "install dir still present after the second uninstall"
[ "$(runuser -u postgres -- psql -X -t -A -p "$DB_PORT" -c "SELECT count(*) FROM pg_database WHERE datname='$DB_NAME'")" = "0" ] || fail "database not dropped by the second uninstall"
pass "second uninstall removed the database too"

# A database left behind by an earlier installation (an older build, a deleted
# folder) must neither be overwritten nor leave the person stuck: install
# refuses, and installs fresh only when asked to replace it.
step "Leftover database from an earlier installation"
runuser -u postgres -- psql -X -q -p "$DB_PORT" -c "CREATE DATABASE $DB_NAME" || fail "could not create the leftover database"
runuser -u postgres -- psql -X -q -p "$DB_PORT" -d "$DB_NAME" -c "CREATE TABLE leftover_marker (id int); INSERT INTO leftover_marker VALUES (42)" || fail "could not fill the leftover database"
if "$INSTALLER" install --config "$WORK/answers.json" >"$WORK/refused.log" 2>&1; then fail "install went ahead over a database that holds data"; fi
grep -q "already exists and contains data" "$WORK/refused.log" || fail "refusal does not name the leftover data: $(cat "$WORK/refused.log")"
[ "$(runuser -u postgres -- psql -X -t -A -p "$DB_PORT" -d "$DB_NAME" -c "SELECT count(*) FROM leftover_marker")" = "1" ] || fail "the refused install touched the leftover data"
pass "install refuses to overwrite it and leaves it untouched"
cat > "$WORK/answers-replace.json" <<EOF
{"installDir":"$DIR","appPort":"$APP_PORT","apiPort":"8080","dbMode":"local","replaceDatabase":true}
EOF
"$INSTALLER" install --config "$WORK/answers-replace.json" | tee "$WORK/replace.log" || fail "install with replaceDatabase failed"
grep -q "Deleting the existing database" "$WORK/replace.log" || fail "log does not say the old database was deleted"
health || fail "not healthy after replacing the leftover database"
[ "$(runuser -u postgres -- psql -X -t -A -p "$DB_PORT" -d "$DB_NAME" -c "SELECT to_regclass('leftover_marker') IS NULL")" = "t" ] || fail "the leftover data survived the replacement"
pass "replaceDatabase deleted the old database and installed fresh"
"$INSTALLER" uninstall --dir "$DIR" --yes || fail "final uninstall failed"

echo; echo "ALL INSTALLER E2E CHECKS PASSED"
