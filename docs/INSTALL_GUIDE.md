# ezHealthKonnect — Installation Guide

**Version:** 1.0  
**Platforms:** Windows 10/11, Linux (Ubuntu/Debian/RHEL/Rocky)

---

## Table of Contents

1. [Overview](#overview)
2. [System Requirements](#system-requirements)
3. [Windows Installation](#windows-installation)
4. [Linux Installation](#linux-installation)
5. [First Login](#first-login)
6. [Uninstalling](#uninstalling)
7. [Troubleshooting](#troubleshooting)

---

## Overview

ezHealthKonnect is an AI-powered healthcare integration platform that transforms HL7 messages to FHIR format. The installer sets up all required components automatically:

- **PostgreSQL 15** — primary database
- **Node.js 20 LTS** — web application runtime
- **ezHealthKonnect Go API** — high-performance HL7/FHIR transformation engine
- **HL7 schemas** — v2.3, v2.5, v2.5.1 (core) + FHIR R4

After installation, the platform is accessible from your browser at `http://localhost:3000`.

---

## System Requirements

| Component | Minimum |
|---|---|
| RAM | 4 GB (8 GB recommended) |
| Disk space | 3 GB free |
| Network | Internet connection required during installation |

### Windows
- Windows 10 (version 1709 or later) or Windows 11
- Windows Server 2019 or later
- winget (App Installer) — pre-installed on Windows 11; available via Microsoft Store on Windows 10

### Linux
- Ubuntu 20.04 LTS or later
- Debian 11 or later
- RHEL / Rocky Linux / AlmaLinux 8 or later
- CentOS Stream 9 or later
- Run as root or a user with `sudo` privileges

---

## Windows Installation

### Step 1 — Download the installer

Download `ezHealthKonnect-Setup-Win64.exe` from the ezInterOp Solutions releases page.

> The installer is a single self-contained file (~350 MB). No additional files are needed.

### Step 2 — Run the installer

Double-click `ezHealthKonnect-Setup-Win64.exe`. Windows may display a SmartScreen warning on first run — click **More info**, then **Run anyway**.

The installer opens a wizard in your default browser:

```
http://127.0.0.1:<random-port>
```

> If the browser does not open automatically, the installer window shows the URL — copy and paste it manually.

### Step 3 — Choose installation type

Select **Standalone Edition** (recommended for single-server deployments that do not have Docker).

Select **Docker Edition** if Docker Desktop is already installed and you prefer container-based deployment.

### Step 4 — Run prerequisite checks

Click **Check Prerequisites**. The installer verifies:

- Available disk space (3 GB minimum)
- Network ports 3000, 8080, and 5432 are not in use
- winget package manager availability

> If a port is in use, you can change it in the Configuration step — no action required at this stage.

### Step 5 — Configure (optional)

The defaults work for most installations:

| Setting | Default | Notes |
|---|---|---|
| Install directory | `C:\ezHealthKonnect` | Must not already exist |
| Web port | `3000` | Browser access URL |
| API port | `8080` | Internal — no firewall rule needed |
| Database host | `localhost` | Change only for external PostgreSQL |
| Database port | `5432` | Standard PostgreSQL port |

> **External database**: If you have an existing PostgreSQL server, enter its host address, port, and the superuser password. The installer will create the application database and user automatically.

### Step 6 — Install

Click **Install**. The installer runs up to 10 steps and shows live progress:

1. Prepares the install directory
2. Installs PostgreSQL 15 (via winget, or direct download as fallback ~350 MB)
3. Installs Node.js 20 LTS (via winget, or direct download ~30 MB)
4. Extracts the application
5. Installs Node.js dependencies
6. Creates the database and user
7. Writes the configuration file
8. Runs database migrations
9. Registers Windows services (auto-start on boot)
10. Waits for the application to become ready

**Typical installation time:** 5–15 minutes depending on internet speed.

### Step 7 — Complete

When installation completes, the wizard shows a **Go to Platform** button. Click it to open ezHealthKonnect.

The platform is now registered as a single Windows service, `ezhealthkonnect`, which runs both
the Node.js web frontend and the Go API backend (via a small start-app.ps1 wrapper).

The service starts automatically when Windows boots.

---

## Linux Installation

### Step 1 — Download the installer

Download `ezHealthKonnect-Setup-Linux-x64` from the ezInterOp Solutions releases page.

### Step 2 — Make the file executable

Open a terminal and run:

```bash
chmod +x ezHealthKonnect-Setup-Linux-x64
```

### Step 3 — Run the installer

The installer must run with root or sudo privileges to install system packages and register services:

```bash
sudo ./ezHealthKonnect-Setup-Linux-x64
```

The installer opens a wizard in your default browser. On headless servers, copy the URL printed to the terminal and open it from another machine:

```
http://<server-ip>:<port>
```

> **Firewall note:** The installer wizard runs on a random local port. If accessing remotely, you may need to temporarily open that port or use SSH port forwarding:
> ```bash
> ssh -L 7788:localhost:7788 user@server
> ```
> Then open `http://localhost:7788` in your local browser.

### Step 4 — Choose Standalone Edition

Select **Standalone Edition**. The Linux installer automatically detects your package manager (apt-get, dnf, or yum) and installs the required components.

### Step 5 — Configure (optional)

The defaults are recommended for first-time installations:

| Setting | Default |
|---|---|
| Install directory | `/opt/ezhealthkonnect` |
| Web port | `3000` |
| API port | `8080` |

### Step 6 — Install

The installer runs the following steps automatically:

1. Creates `/opt/ezhealthkonnect/`
2. Installs PostgreSQL 15 from the PGDG repository
3. Installs Node.js 20 LTS from NodeSource repository
4. Extracts the application files
5. Installs Node.js dependencies
6. Creates the PostgreSQL database and user
7. Writes `/opt/ezhealthkonnect/.env`
8. Runs database migrations
9. Registers `ezhealthkonnect` and `ezhealthkonnect-api` systemd services
10. Waits for health check

**Typical installation time:** 5–15 minutes.

### Step 7 — Verify services are running

```bash
systemctl status ezhealthkonnect
systemctl status ezhealthkonnect-api
```

Both should show `active (running)`.

### Accessing the platform

From the server itself:
```
http://localhost:3000
```

From another machine on the same network (replace `<server-ip>` with the server's IP address):
```
http://<server-ip>:3000
```

> To allow remote access, open port 3000 in your firewall:
> ```bash
> # Ubuntu/Debian (UFW)
> sudo ufw allow 3000/tcp
>
> # RHEL/Rocky (firewalld)
> sudo firewall-cmd --add-port=3000/tcp --permanent
> sudo firewall-cmd --reload
> ```

---

## First Login

After installation on any platform, open a browser and navigate to:

```
http://localhost:3000
```

Log in with the default administrator credentials:

| Field | Value |
|---|---|
| Email | `admin@ezhealthkonnect.com` |
| Password | `admin123` |

> **Security:** Change the default password immediately after your first login.
> Click your profile icon (top right) → **Profile** → **Change Password**.

### Post-installation steps

1. **Change the admin password** — required before connecting any real data sources
2. **Create additional user accounts** — navigate to **Admin → User Management**
3. **Configure your first interface** — click **New Interface** on the dashboard
4. **Test with a sample HL7 message** — use the built-in message tester

---

## Uninstalling

### Windows

**Option 1 — Add/Remove Programs:**
Open **Windows Settings → Apps → Installed apps**, search for "ezHealthKonnect", click **Uninstall**.

**Option 2 — Re-run the installer:**
Run `ezHealthKonnect-Setup-Win64.exe` again. If the application is already installed, the wizard shows an **Uninstall** option.

The uninstaller stops all services, drops the PostgreSQL database and user, and removes all application files.

> PostgreSQL itself is not uninstalled (it may be used by other applications). To remove PostgreSQL, use **Add/Remove Programs** and uninstall "PostgreSQL 15".

### Linux

Re-run the installer and click **Uninstall**, or run manually:

```bash
sudo systemctl stop ezhealthkonnect ezhealthkonnect-api
sudo systemctl disable ezhealthkonnect ezhealthkonnect-api
sudo rm /etc/systemd/system/ezhealthkonnect*.service
sudo systemctl daemon-reload
sudo -u postgres psql -c "DROP DATABASE IF EXISTS ezhealthkonnect;"
sudo -u postgres psql -c "DROP USER IF EXISTS ezhealth_user;"
sudo rm -rf /opt/ezhealthkonnect
```

---

## Troubleshooting

### The browser does not open automatically

The installer prints the URL to its console window. Copy and paste it into any browser.
On Linux servers without a desktop, use the URL from another machine (see [Linux Installation](#linux-installation)).

### Port 3000 or 8080 is already in use

During configuration (Step 5), change the **Web port** or **API port** to a free port (e.g., 3001, 8081). The installer will configure all components to use the new port.

### PostgreSQL installation fails (Windows)

The installer tries winget first, then a direct download. If the direct download also fails:

1. Download and install [PostgreSQL 15](https://www.enterprisedb.com/downloads/postgres-postgresql-downloads) manually
2. During installation, set the superuser password to `postgres`
3. Re-run the ezHealthKonnect installer — it will detect the existing PostgreSQL and skip installation

### PostgreSQL installation fails (Linux — permission denied)

Ensure you are running the installer with `sudo`:
```bash
sudo ./ezHealthKonnect-Setup-Linux-x64
```

### "Database already exists" during migration

This is a warning, not an error. The installer is safe to re-run. Existing migrations are detected and skipped automatically.

### Application does not start after installation

Check the service logs:

**Windows:**
```
C:\ezHealthKonnect\logs\
```

**Linux:**
```bash
journalctl -u ezhealthkonnect -n 50
journalctl -u ezhealthkonnect-api -n 50
```

### Checking service status

**Windows (PowerShell):**
```powershell
Get-Service ezhealthkonnect
```

**Linux:**
```bash
systemctl status ezhealthkonnect ezhealthkonnect-api
```

### Restarting services manually

**Windows:**
```powershell
Restart-Service ezhealthkonnect
```

**Linux:**
```bash
sudo systemctl restart ezhealthkonnect ezhealthkonnect-api
```

---

## Support

For assistance, contact ezInterOp Solutions:

- **Website:** [https://www.ezInterOpSolutions.com](https://www.ezInterOpSolutions.com)
- **Email:** support@ezinteropsolutions.com

---

*ezHealthKonnect is a proprietary product of ezInterOp Solutions. All rights reserved.*
