# ezHealthKonnect Installer — Architecture

For engineers who maintain the installer. The user-facing guide is
[docs/INSTALL_GUIDE.md](../docs/INSTALL_GUIDE.md).

## Decisions (v26.10)

- **Production installs are Standalone only.** Docker is a development tool
  for this repository (`docker-compose.yml`) and is not shipped to customers.
  This removes a dependency on Docker Desktop licensing and engine changes.
- **The database schema is managed by the application's own migration
  engine** (`go-api migrate`, `services/migrator`), not by Flyway. It reads
  and writes Flyway's `flyway_schema_history` table and uses Flyway's
  checksum algorithm, so existing databases continue unchanged.
- **New installs get PostgreSQL 17.** Existing servers must run PostgreSQL 15
  or newer.
- **No default administrator.** The first administrator is created through a
  one-time, token-protected setup link.

## Components

| Path | Role |
|------|------|
| `cmd/bundle-builder` | Builds the application bundle zip from **git-tracked files only**, as listed in `packaging/bundle-manifest.json`. It adds a pinned, SHA-256-verified Node.js runtime and production `node_modules`, plus WinSW on Windows. A test fails if the bundle stops covering any runtime `COPY` in the Dockerfile. |
| `packaging/bundle-manifest.json` | What the bundle contains, the customer-data entries that must never appear in it (`protected`), and the pinned downloads (Node.js, WinSW, the PostgreSQL Windows installer). |
| `installer/` (Go module `ezhealthkonnect-installer`) | The installer. It is built with `-tags embedded` and the bundle compiled in, so each release ships one file per OS. |
| `installer/test/` | Real end-to-end scripts, run by `.github/workflows/installer.yml`. |

## Installer structure

- **`Installer`** (`installer.go`) — the orchestrator: Install, Upgrade,
  Rollback, Uninstall. Its dependencies are all injected (`Platform`,
  `BundleSource`, `Reporter`, the PostgreSQL tools, the migrator and the
  health checker), so `installer_test.go` exercises every path, including
  each failure-and-rollback path, with fakes.
- **`Platform`** (`platform.go`) — OS-specific work: elevation, installing
  PostgreSQL, PostgreSQL admin access, services, file permissions and the
  uninstall registration. `platform_linux.go` uses systemd with apt or
  dnf/yum and the official PGDG repositories. `platform_windows.go` uses
  WinSW, the EDB installer and the registry. Supporting a new OS means
  adding one Platform implementation.
- **`BundleSource`** (`bundle.go`) — the embedded bundle or a `--bundle`
  file. Zip extraction rejects paths that would escape the target directory.
- **`UIServer`** (`server.go`, `web/`) — the setup wizard. It listens on
  `127.0.0.1` only, behind a 256-bit per-run token, with Host and Origin
  checks, JSON-only POST requests, progress over SSE and a CSP of
  `script-src 'self'`.
- **Headless commands** (`main.go`): `install`, `upgrade`, `rollback`,
  `uninstall`, `status`, `version`.

## Install layout and data protection

The application is installed flat into the install directory. The bundle's
top-level entries are recorded in `install-state.json` (`appEntries`). An
upgrade replaces only those entries. Everything else is customer data and is
never moved or deleted by an upgrade. The always-protected set is `.env`,
`install-state.json`, `logs`, `storage`, `schemas`, `schema-dist`,
`uploads`, `backups`, `git-repos`, `cache` and `tools`.

## Upgrade transaction

1. Checks: an installation exists, the new version is not older (downgrades
   are refused), the database is reachable, `pg_dump` is present, and at
   least 2 GB is free.
2. Stop the services.
3. Back up into `backups/<timestamp>-<old version>`: a `pg_dump -Fc` (checked
   to contain `flyway_schema_history`), `.env`, the state file and the old
   application entries. A partial backup is removed if this step fails, so
   nothing has changed yet.
4. Install the new entries and add any new settings to `.env` without
   changing existing values.
5. `go-api migrate`.
6. Start the services and wait for `/api/system/health` to report the
   database as connected.

From step 2 onward, any failure restores the old files and the database
(one transaction: `DROP OWNED BY CURRENT_USER` followed by the restored dump)
and starts the old version again. The backup of a failed attempt is renamed
`-rolledback`, so a later manual rollback cannot pick it up. Three backups
are kept.

**Manual rollback** restores the newest backup whose target version matches
the installed version, then renames that backup `-restored`.

## Security

- `.env` gets generated `SESSION_SECRET`, `JWT_SECRET`,
  `INTERNAL_PROXY_SECRET`, `APP_CREDENTIAL_KEY` (reused on reinstall, so
  stored credentials stay readable), `DB_PASSWORD` and `SETUP_TOKEN`.
- Linux: the services run as the system user `ezhealthkonnect` under hardened
  systemd units; `CAP_NET_BIND_SERVICE` lets them use ports below 1024.
  `backups` and the state file are root-only.
- Windows: the install directory's ACL is restricted to SYSTEM and
  Administrators. A generated PostgreSQL superuser password is stored in
  `tools\pg-superuser.secret`.
- The PostgreSQL Windows installer runs only if its SHA-256 matches the value
  compiled into the installer.

## Known limitations

- The Windows installer is not code-signed yet, so SmartScreen warns on first
  run.
- The Windows service runs as LocalSystem. This is mitigated by the
  directory ACL; a dedicated virtual service account is planned.
- On Windows, `start-app.ps1` stops `go-api` forcefully when the service
  stops, so there is no graceful drain of in-flight messages.
- Linux builds are x86_64 only.
- On Windows with a PostgreSQL that existed before installation, the
  customer's superuser password is used during install and deliberately not
  stored. Uninstall therefore leaves the database in place, and the E2E
  asserts this.

## Testing

- **Unit tests:** `cd installer && go test ./...` (also compile for Windows:
  `GOOS=windows go test -c`) and `go test ./cmd/bundle-builder`.
- **End to end:** `.github/workflows/installer.yml` builds both installers,
  then runs `installer/test/e2e-linux.sh` on Ubuntu 22.04, Ubuntu 24.04 and
  an AlmaLinux 9 systemd container, and `installer/test/e2e-windows.ps1` on
  Windows with a fresh and with an existing PostgreSQL. Each run covers:
  - install, the setup token and creating data;
  - an upgrade to a variant bundle carrying a marker migration;
  - an upgrade with a deliberately failing migration, which must roll back
    with the data intact;
  - a manual rollback;
  - uninstall.
- `release.yml` publishes only the installers this workflow built and tested.
