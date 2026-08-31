# Installing ezHealthKonnect

ezHealthKonnect ships as two editions. Pick the one that fits your environment:

| Edition | Best for | Requires |
|---------|----------|---------|
| **Standalone** | Servers, VMs, bare-metal | Nothing — installer handles everything |
| **Docker** | Developer workstations, teams already using Docker | Docker Desktop / Docker Engine |

---

## Linux — Standalone Edition (recommended for servers)

One command. The script installs PostgreSQL, Node.js, downloads the app, and registers systemd services.

```bash
curl -fsSL https://github.com/ezinteropsolutions/ezhealthkonnect/releases/latest/download/install.sh | sudo bash
```

**Supported distros:** Ubuntu 20.04+, Debian 11+, RHEL/Rocky/AlmaLinux/CentOS Stream 8+
**Architecture:** x86_64, arm64

### Options

Pass flags after `--` when piping, or as arguments when running directly:

```bash
# With custom ports and password
sudo bash install.sh --port 3000 --api-port 8080 --db-password MyStr0ngPass

# Enable AI companion (downloads ~2.3 GB of Ollama models)
sudo bash install.sh --with-ai

# Skip systemd service registration (start manually)
sudo bash install.sh --no-service

# Use a specific version
sudo bash install.sh --version v1.2.0
```

| Flag | Default | Description |
|------|---------|-------------|
| `--install-dir DIR` | `/opt/ezhealthkonnect` | Where app files are stored |
| `--port PORT` | `3000` | Web UI port |
| `--api-port PORT` | `8080` | Go API port |
| `--db-port PORT` | `5432` | PostgreSQL port |
| `--db-password PASS` | *(auto-generated)* | Database password |
| `--with-ai` | off | Install Ollama AI companion |
| `--no-service` | *(registers service)* | Skip systemd registration |
| `--version VERSION` | `latest` | Release version to install |

### After install

```bash
# Check service status
systemctl status ezhealthkonnect
systemctl status ezhealthkonnect-api

# View live logs
journalctl -u ezhealthkonnect -f

# Restart
systemctl restart ezhealthkonnect ezhealthkonnect-api

# Configuration file
nano /opt/ezhealthkonnect/.env
```

Open `http://<server-ip>:3000` in your browser.
Default login: `admin@ezhealthkonnect.com` / `admin123` — **change this immediately.**

---

## Windows — Setup Wizard

The installer is a single `.exe` — no dependencies, no admin rights needed to launch.

1. Go to the [Releases page](https://github.com/ezinteropsolutions/ezhealthkonnect/releases/latest)
2. Download `ezhealthkonnect-installer.exe`
3. Double-click it — a browser window opens automatically
4. Choose your edition:
   - **Docker Edition** — requires Docker Desktop (offered for auto-install if missing)
   - **Standalone Edition** — installs PostgreSQL and Node.js natively via winget (Windows 10/11 only)
5. Follow the wizard: prerequisites → configuration → install

**Requirements for Standalone Edition:**
Windows 10 version 1809 or later (winget must be available). If winget is missing, install [App Installer](https://apps.microsoft.com/store/detail/app-installer/9NBLGGH4NNS1) from the Microsoft Store.

### After install (Windows Standalone)

Open **Services** (`services.msc`) — you'll see one service, **ezHealthKonnect**, running
both the Go backend and the Node.js frontend (a small start-app.ps1 wrapper starts both and
keeps them tied together — see `installer/assets/start-app.ps1`).

Or via PowerShell:
```powershell
Get-Service ezhealthkonnect
Restart-Service ezhealthkonnect
```

Logs: `C:\ezHealthKonnect\logs\ezhealthkonnect.out.log` and `ezhealthkonnect.err.log`

---

## Linux — Docker Edition

Use this if you already have Docker on your Linux machine (e.g. a developer workstation).

1. Download the Linux installer:
```bash
curl -fsSL -o ezhealthkonnect-installer \
  https://github.com/ezinteropsolutions/ezhealthkonnect/releases/latest/download/ezhealthkonnect-installer-linux
chmod +x ezhealthkonnect-installer
./ezhealthkonnect-installer
```

2. A browser window opens (via `xdg-open`). If you're on a headless server, copy the URL printed in the terminal and open it on any machine that can reach this server.

---

## Verifying Downloads

Each release includes a `SHA256SUMS.txt` file. To verify:

```bash
# Linux
sha256sum -c SHA256SUMS.txt --ignore-missing

# PowerShell
Get-FileHash ezhealthkonnect-installer.exe -Algorithm SHA256
# compare against SHA256SUMS.txt
```

---

## Uninstalling

**Linux Standalone:**
```bash
sudo systemctl stop ezhealthkonnect ezhealthkonnect-api
sudo systemctl disable ezhealthkonnect ezhealthkonnect-api
sudo rm /etc/systemd/system/ezhealthkonnect*.service
sudo systemctl daemon-reload
sudo rm -rf /opt/ezhealthkonnect
# Optional: remove PostgreSQL database
sudo -u postgres psql -c "DROP DATABASE ezhealthkonnect;"
sudo -u postgres psql -c "DROP USER ezhealth_user;"
```

**Windows Standalone:**

Prefer the installer's own uninstall (`ezHealthKonnect-Setup-Win64.exe --uninstall`, or via
Add/Remove Programs) — it also drops the database and removes the Add/Remove Programs entry.
Manual equivalent, if needed:
```powershell
C:\ezHealthKonnect\tools\ezhealthkonnect.exe stop
C:\ezHealthKonnect\tools\ezhealthkonnect.exe uninstall
Remove-Item -Recurse -Force C:\ezHealthKonnect
```

**Docker Edition (any platform):**
```bash
cd /opt/ezhealthkonnect   # or wherever you installed
docker compose -f docker-compose.prod.yml down -v --remove-orphans
rm -rf /opt/ezhealthkonnect
```

---

## Upgrading

Upgrade support is not yet built into the installer. For now:
1. Stop services
2. Back up your `.env` and database
3. Run the installer again into the same directory — it will overwrite app files and re-run migrations
4. Restart services

---

## Troubleshooting

**Port already in use:**
Use `--port` / `--api-port` / `--db-port` flags to choose different ports, or stop the conflicting process first.

**Linux: "Permission denied" running install.sh:**
The script must run as root: `sudo bash install.sh`

**Windows: winget not found:**
Install [App Installer](https://apps.microsoft.com/store/detail/9NBLGGH4NNS1) from the Microsoft Store, then re-run the installer.

**App not starting (Linux):**
```bash
journalctl -u ezhealthkonnect -n 50 --no-pager
journalctl -u ezhealthkonnect-api -n 50 --no-pager
```

**Database connection errors:**
Check that the password in `.env` matches the PostgreSQL user password:
```bash
cat /opt/ezhealthkonnect/.env | grep DB_PASSWORD
sudo -u postgres psql -c "\du"
```

**Need help?**
Open an issue at [github.com/ezinteropsolutions/ezhealthkonnect/issues](https://github.com/ezinteropsolutions/ezhealthkonnect/issues)
