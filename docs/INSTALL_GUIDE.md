# Installing ezHealthKonnect

ezHealthKonnect installs natively on Windows and Linux servers as system
services. Docker is not needed and is not used. (The repository's
`docker-compose.yml` is for developing ezHealthKonnect itself and is not a
supported way to run it in production.)

Each release has two installers and a checksum file:

| File | For |
|------|-----|
| `ezHealthKonnect-Setup-Win64.exe` | Windows Server 2019+ / Windows 10 and 11, x64 |
| `ezHealthKonnect-Setup-Linux-x64` | Linux x86_64 with systemd: Ubuntu 22.04/24.04, Debian 11+, RHEL/Rocky/AlmaLinux 8 and 9 |
| `SHA256SUMS.txt` | Checksums — verify before running |

Each installer contains the whole application, including its own Node.js
runtime. During the install it needs internet access only to obtain
PostgreSQL, and only when PostgreSQL is not already on the machine.

Every release's installers are tested end to end on fresh Ubuntu 22.04,
Ubuntu 24.04, AlmaLinux 9 and Windows hosts before they are published. The
tests cover install, an upgrade, a failed upgrade that must roll back, a
manual rollback and uninstall.

---

## 1. Verify the download

```bash
sha256sum -c SHA256SUMS.txt --ignore-missing          # Linux
```
```powershell
Get-FileHash .\ezHealthKonnect-Setup-Win64.exe -Algorithm SHA256   # compare with SHA256SUMS.txt
```

## 2. Install

### Windows

Run `ezHealthKonnect-Setup-Win64.exe` as an administrator. A setup window
opens. It checks the machine, asks for the ports and the database, and
installs. If you close the window before finishing, setup exits on its own
about two minutes later; an install that is already running always
completes first. Run from a command prompt, the installer also prints the
setup address there, and the headless commands (`install --config`,
`upgrade`, `uninstall`) report progress in that window.

### Linux

```bash
chmod +x ezHealthKonnect-Setup-Linux-x64
sudo ./ezHealthKonnect-Setup-Linux-x64
```

On a server without a desktop, the installer prints the setup address
together with an `ssh -L` command for opening it from your own machine. You
can also install without a browser using an answer file (see below).

### What the installer does

- **Database.** It uses PostgreSQL on this machine. If PostgreSQL is missing,
  the installer adds PostgreSQL 17: from the official PostgreSQL repositories
  on Linux, or with the official EDB installer on Windows (downloaded only
  when needed, and run only if its SHA-256 checksum matches). It creates a
  dedicated database user with a generated password. Alternatively, you can
  point it at an existing database, for example a managed cloud PostgreSQL.
  PostgreSQL 15 or newer is required.
- **Database schema.** It creates and upgrades the database schema with the
  application's built-in migration tool (`go-api migrate`).
- **Secrets.** It writes `.env` with freshly generated secrets: the session
  and JWT keys, the internal proxy secret, the key that encrypts connector
  credentials (`APP_CREDENTIAL_KEY`), and a one-time setup token.
- **Services.**
  - Linux: two systemd services, `ezhealthkonnect` (web) and
    `ezhealthkonnect-api` (engine). Both run as the unprivileged system user
    `ezhealthkonnect`.
  - Windows: one service, `ezhealthkonnect`. It runs as its own virtual
    account, `NT SERVICE\ezhealthkonnect`, not as LocalSystem. The account can
    read the application, write only the data folders, and cannot access
    `backups`. Otherwise only SYSTEM and Administrators can access the install
    directory.
- **Health check.** It waits until the application reports healthy before
  saying it is done.

Default locations: `/opt/ezhealthkonnect` (Linux) and `C:\ezHealthKonnect`
(Windows).

## 3. Create the first administrator

When the install finishes, the installer shows a one-time link:
`http://<server>:3000/setup.html?token=…`. Open it to create the first
administrator account. There is no default password. Until setup is
completed, the setup page refuses anyone who does not have the token, which
is stored as `SETUP_TOKEN` in `.env`. Keep the link private.

---

## Installing without a browser (answer file)

```bash
sudo ./ezHealthKonnect-Setup-Linux-x64 install --config answers.json
```
```powershell
.\ezHealthKonnect-Setup-Win64.exe install --config answers.json   # elevated prompt
```

`answers.json`, with defaults shown. Every field is optional.

```json
{
  "installDir": "/opt/ezhealthkonnect",
  "appPort": "3000",
  "apiPort": "8080",
  "dbMode": "local",
  "dbPort": "5432",
  "dbName": "ezhealthkonnect",
  "dbUser": "ezhealth_user",
  "withAI": false
}
```

To use an existing PostgreSQL server (another machine, a managed cloud
service, or one on this machine addressed by host), set `"dbMode": "external"`
together with `dbHost`, `dbPort`, `dbName`, `dbUser` and `dbSslMode`
(`require` by default; `disable`, `verify-ca` and `verify-full` are also
accepted; a server on this machine normally needs `disable`), and then
either:

- **an administrator login**: `dbAdminUser` (default `postgres`) and
  `dbAdminPassword`. Setup signs in once, creates the database and the
  application user (with a generated password, kept only in `.env`) and
  never stores the administrator password. Nothing has to exist beforehand.
  An existing database of that name that already holds tables is refused,
  never reused. On managed services (RDS, Azure, Cloud SQL) the
  administrator is not a superuser; that is supported, provided it may
  create roles and databases.
- **an existing user**: `dbPassword`, for a database and user that already
  exist. The database must be empty and owned by that user. Setup only
  connects to it.

In the wizard this is the **Existing database** option: choose *Create the
database and user for me* or *They already exist*.

On Windows, if PostgreSQL is already installed on the machine, also set
`pgAdminPassword` (the `postgres` superuser password). The installer needs it
to create the database and user.

The installer prints the one-time setup link when it finishes.

---

## Upgrading

Run the newer installer on the same machine. The setup window detects the
existing installation and offers **Upgrade**. Headless:

```bash
sudo ./ezHealthKonnect-Setup-Linux-x64 upgrade        # --dir if not the default
```

Before changing anything, an upgrade checks three things: the database is
reachable, the PostgreSQL backup tools are present, and there are at least
2 GB of free disk space. If any check fails, the upgrade stops and nothing is
changed. Then the upgrade:

1. Stops the services.
2. Backs up the database (`pg_dump`) and the application files into
   `<install dir>/backups/<timestamp>-<old version>`.
3. Installs the new application files. New settings are added to `.env`;
   existing values are never changed.
4. Applies database migrations.
5. Starts the services and waits for a healthy response.

**If any step fails, the installer restores the database and the files from
that backup automatically and starts the previous version again.**

Your data is never touched by an upgrade: `.env`, `storage`, `logs`,
`uploads`, `schemas`, `backups` and the other data folders. Installing an
older version over a newer one is refused. The three most recent upgrade
backups are kept.

## Rolling back

To return to the version before the last successful upgrade:

```bash
sudo ./ezHealthKonnect-Setup-Linux-x64 rollback --yes
```

This restores the database exactly as it was before that upgrade. **Messages
and configuration changes made since that upgrade are lost**, which is why
`--yes` is required. The setup window offers the same action.

## Status

```bash
sudo ./ezHealthKonnect-Setup-Linux-x64 status     # installed version, history, backups
./ezHealthKonnect-Setup-Linux-x64 version         # versions this installer contains
```

## Uninstalling

Use the setup window, **Add or remove programs** (Windows), or:

```bash
sudo ./ezHealthKonnect-Setup-Linux-x64 uninstall --yes                  # also drops the database
sudo ./ezHealthKonnect-Setup-Linux-x64 uninstall --yes --keep-database  # keep the data
```

Setup finds an existing installation wherever it was installed (from the
Add or remove programs entry and the service), so Manage and Uninstall are
offered even for a folder other than the default. Pass `--dir <folder>` only
to point at a different one. Uninstall refuses a folder that is not an
ezHealthKonnect installation (it has no `install-state.json`), so it can
never delete an unrelated folder. An installation left by an older
installer has no such record: stop and delete the `ezhealthkonnect` service
(`sc.exe stop ezhealthkonnect` then `sc.exe delete ezhealthkonnect` on
Windows) and remove its folder by hand. The Add or remove programs entry is
created before the service is started, so even an install that failed
part-way can be removed from there.

Uninstall removes the services and **the whole install directory, including
`storage`, `logs`, `uploads` and `backups`**. Copy anything you want to keep
before running it. It drops the database **that setup created** (PostgreSQL
on this machine, or an existing server where you gave an administrator
login) unless `--keep-database` is given, and the setup window's
confirmation says whether it will. A database you provided yourself ("They
already exist") is never dropped. PostgreSQL itself is not removed.

Setup never stores an administrator password you typed. It does not need it
to uninstall: it created the database with the application user as its
owner, and an owner may drop its own database. When setup installed
PostgreSQL itself it also removes the `ezhealth_user` role; when it dropped
the database as the owner, the role stays (a role cannot remove itself), and
a reinstall simply reuses it. If the drop fails, uninstall still finishes and
prints the one command to run (`DROP DATABASE ezhealthkonnect;`).

## Reinstalling

Uninstall, then run setup again: this works without any database password.
On Windows, when setup installed PostgreSQL itself it generates the
`postgres` password and also keeps it in `C:\ProgramData\ezHealthKonnect`
(readable by administrators and SYSTEM only), because PostgreSQL stays after
an uninstall. A reinstall reuses it and says so.

Two situations still need a hand:

- **"database ezhealthkonnect already exists and contains data".** An earlier
  installation left its database behind (it was uninstalled by an older
  version, or its folder was deleted by hand instead of using Uninstall).
  Setup never overwrites data on its own. In the setup window the failure
  offers **Delete the old database and install fresh**: it asks you to
  confirm, then deletes that database and all its data and installs a new
  one. In an answer file, set `"replaceDatabase": true`. It needs the
  administrator access you already gave setup (the `postgres` password on an
  existing server, or the one setup generated); it is not available for a
  database and user you provided yourself. To keep the data instead, restore
  that installation and choose Upgrade. Or drop it yourself:

  ```powershell
  $env:PGPASSWORD = "<the postgres password>"
  & "C:\Program Files\PostgreSQL\17\bin\psql.exe" -U postgres -h localhost `
     -c "DROP DATABASE IF EXISTS ezhealthkonnect" -c "DROP ROLE IF EXISTS ezhealth_user"
  ```

- **PostgreSQL is already installed and nobody knows the `postgres`
  password** (an install made by an older installer, or its files deleted by
  hand). If another application uses that PostgreSQL, ask its owner for the
  password. If nothing else does, set a new one through a temporary local
  trust rule (run PowerShell as administrator; change `17` to your version):

  ```powershell
  $data = "C:\Program Files\PostgreSQL\17\data"
  Stop-Service postgresql-x64-17
  Copy-Item "$data\pg_hba.conf" "$data\pg_hba.conf.bak"
  (Get-Content "$data\pg_hba.conf") -replace '(host\s+all\s+all\s+(127\.0\.0\.1/32|::1/128)\s+)scram-sha-256','$1trust' | Set-Content "$data\pg_hba.conf"
  Start-Service postgresql-x64-17
  & "C:\Program Files\PostgreSQL\17\bin\psql.exe" -U postgres -h localhost -c "ALTER USER postgres PASSWORD '<new password>'"
  Stop-Service postgresql-x64-17
  Move-Item -Force "$data\pg_hba.conf.bak" "$data\pg_hba.conf"
  Start-Service postgresql-x64-17
  ```

  Then enter the new password in setup's "PostgreSQL superuser password"
  box. This replaces the `postgres` login for every application on that
  server.

---

## Day-to-day operations

**Linux**
```bash
systemctl status ezhealthkonnect ezhealthkonnect-api
systemctl restart ezhealthkonnect ezhealthkonnect-api
journalctl -u ezhealthkonnect-api -n 100 --no-pager
```
Configuration is in `/opt/ezhealthkonnect/.env`, readable only by root and
the service user. Restart both services after changing it.

**Windows**
```powershell
Get-Service ezhealthkonnect
Restart-Service ezhealthkonnect
```
Logs are in `C:\ezHealthKonnect\logs`. Configuration is in
`C:\ezHealthKonnect\.env`.

## Troubleshooting

- **A port is in use.** The setup window checks the ports before installing.
  Choose different `appPort`/`apiPort` values, or stop the program that is
  using the port.
- **An upgrade failed.** The installer has already rolled back. It prints the
  step that failed and the backup it restored. The services' logs explain
  migration and startup errors.
- **PostgreSQL could not be installed** (no internet or a blocked
  repository). Install PostgreSQL 15 or newer yourself, then choose an
  existing database (`dbMode: external`).
- **Help:** [github.com/ezinteropsolutions/ezhealthkonnect/issues](https://github.com/ezinteropsolutions/ezhealthkonnect/issues)
