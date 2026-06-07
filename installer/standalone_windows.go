//go:build windows

package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	// WinSW is a reliable GitHub-hosted service wrapper — single exe, no zip extraction needed.
	winswDownload = "https://github.com/winsw/winsw/releases/download/v2.12.0/WinSW-x64.exe"
)

// runStandaloneInstallation is the Windows native (no-Docker) install path.
func runStandaloneInstallation(cfg *Config) {
	success := false
	defer func() {
		if success {
			appURL := fmt.Sprintf("http://localhost:%s", cfg.AppPort)
			emitDoneSignal(appURL)
		} else {
			// Signal done without a URL so the UI shows the log, not the success screen
			emitInstallFailed()
		}
	}()

	emit("info", "╔══════════════════════════════════════════════╗")
	emit("info", "║  ezHealthKonnect — Standalone Installation   ║")
	emit("info", "╚══════════════════════════════════════════════╝")
	emit("info", "")

	// ── Step 1: Directory ──────────────────────────────────────────────────
	emitStep(1, "Preparing install directory")
	if err := os.MkdirAll(cfg.InstallDir, 0755); err != nil {
		emit("error", "Failed to create directory: "+err.Error())
		return
	}
	for _, sub := range []string{"logs", "storage", "tools"} {
		os.MkdirAll(filepath.Join(cfg.InstallDir, sub), 0755) //nolint:errcheck
	}
	emit("ok", "Directory ready: "+cfg.InstallDir)

	// ── Step 2: PostgreSQL ─────────────────────────────────────────────────
	if cfg.DBHost == "" {
		cfg.DBHost = "localhost"
	}
	isLocalDB := cfg.DBHost == "localhost" || cfg.DBHost == "127.0.0.1"

	if isLocalDB {
		emitStep(2, "Installing PostgreSQL 15")
	} else {
		emitStep(2, "Using existing PostgreSQL at "+cfg.DBHost)
	}
	psqlPath, err := installPostgresIfNeeded(isLocalDB)
	if err != nil {
		emit("error", "PostgreSQL install failed: "+err.Error())
		return
	}
	emit("ok", "PostgreSQL ready: "+psqlPath)

	// ── Step 3: Node.js ────────────────────────────────────────────────────
	emitStep(3, "Installing Node.js 20 LTS")
	nodePath, err := installNodeJS()
	if err != nil {
		emit("error", "Node.js install failed: "+err.Error())
		return
	}
	emit("ok", "Node.js ready: "+nodePath)

	// ── Step 4: Download app bundle ────────────────────────────────────────
	emitStep(4, "Downloading ezHealthKonnect")
	if err := downloadAppBundle(cfg.InstallDir, cfg.Version); err != nil {
		emit("error", "Download failed: "+err.Error())
		return
	}
	emit("ok", "Application extracted to "+cfg.InstallDir)

	// ── Step 5: npm install ────────────────────────────────────────────────
	emitStep(5, "Installing Node.js dependencies")
	nodeModulesDir := filepath.Join(cfg.InstallDir, "node_modules")
	if _, err := os.Stat(nodeModulesDir); err == nil {
		emit("ok", "Node.js dependencies already bundled â skipping npm install")
	} else {
		npmPath := filepath.Join(filepath.Dir(nodePath), "npm.cmd")
		npmCmd := exec.Command(npmPath, "install", "--omit=dev", "--silent")
		npmCmd.Dir = cfg.InstallDir
		if err := streamCmd(npmCmd); err != nil {
			emit("warn", "npm install had warnings: "+err.Error())
		} else {
			emit("ok", "Node.js dependencies installed")
		}
	}

	// ── Step 6: Database setup ─────────────────────────────────────────────
	emitStep(6, "Setting up PostgreSQL database")
	if err := setupDatabase(psqlPath, cfg, isLocalDB); err != nil {
		emit("error", "Database setup failed: "+err.Error())
		return
	}
	emit("ok", "Database ready")

	// ── Step 7: Write .env ─────────────────────────────────────────────────
	emitStep(7, "Writing configuration")
	envPath := filepath.Join(cfg.InstallDir, ".env")
	if err := writeStandaloneEnvFile(envPath, cfg); err != nil {
		emit("error", "Failed to write .env: "+err.Error())
		return
	}
	emit("ok", "Configuration saved")

	// ── Step 8: Run migrations ─────────────────────────────────────────────
	emitStep(8, "Running database migrations")
	if err := runMigrations(psqlPath, cfg); err != nil {
		emit("warn", "Migrations had errors (may already be applied): "+err.Error())
	} else {
		emit("ok", "Migrations complete")
	}

	// ── Step 9: Register services ──────────────────────────────────────────
	if cfg.RegisterSvc {
		emitStep(9, "Registering Windows services")
		winswPath, err := ensureWinSW(cfg.InstallDir)
		if err != nil {
			emit("warn", "WinSW download failed — skipping service registration: "+err.Error())
		} else {
			goAPIBin := filepath.Join(cfg.InstallDir, "go-api.exe")
			if err := registerWindowsServices(winswPath, nodePath, goAPIBin, cfg); err != nil {
				emit("warn", "Service registration failed: "+err.Error())
			} else {
				emit("ok", "Windows services registered (auto-start on boot)")
			}
		}
	}

	// ── Step 10: Health check ──────────────────────────────────────────────
	emitStep(10, "Waiting for application to become ready")
	appURL := fmt.Sprintf("http://localhost:%s", cfg.AppPort)
	waitForReady(appURL+"/health", 120*time.Second)

	emit("info", "")
	// ── Registry: Add/Remove Programs entry ───────────────────────────────────
	if installerExe, err := os.Executable(); err == nil {
		writeUninstallRegistry(cfg.InstallDir, cfg.AppPort, installerExe, version)
	}

	emit("info", "╔══════════════════════════════════════════════╗")
	emit("ok",   "║   Installation Complete!                     ║")
	emit("info", "╚══════════════════════════════════════════════╝")
	emit("info", "")
	emit("ok", "Platform URL: "+appURL)
	emit("info", "Default login: admin@ezhealthkonnect.com / admin123")
	emit("warn", "Change the default password immediately after first login.")
	success = true
}

// ── PostgreSQL ─────────────────────────────────────────────────────────────

// postgresDirectURL is the official EDB silent installer for PostgreSQL 15.
// Using a pinned URL avoids winget source index issues entirely.
const postgresDirectURL = "https://get.enterprisedb.com/postgresql/postgresql-15.13-1-windows-x64.exe"

func installPostgresIfNeeded(isLocal bool) (string, error) {
	if !isLocal {
		// External/cloud DB — only need the psql client for running migrations.
		if p := findPsql(); p != "" {
			emit("info", "Using existing PostgreSQL client: "+p)
			return p, nil
		}
		return "", fmt.Errorf("psql.exe not found — install PostgreSQL client tools or add psql.exe to PATH")
	}
	return installPostgres()
}

func installPostgres() (string, error) {
	if p := findPsql(); p != "" {
		emit("info", "PostgreSQL already installed: "+p)
		return p, nil
	}

	// Try winget first — available on Windows 10 1709+ and all Windows 11.
	// Faster and avoids HTTP/2 CDN issues with large EDB installer.
	if err := installViaWinget("PostgreSQL.PostgreSQL.15", "PostgreSQL 15"); err == nil {
		emit("info", "Waiting for PostgreSQL service to start...")
		time.Sleep(10 * time.Second)
		exec.Command("net", "start", "postgresql-x64-15").Run() //nolint:errcheck
		if p := findPsql(); p != "" {
			return p, nil
		}
	}

	// Fall back to direct EDB download.
	if err := installPostgresDirect(); err != nil {
		return "", fmt.Errorf("PostgreSQL install failed: %w", err)
	}

	emit("info", "Waiting for PostgreSQL service to start...")
	time.Sleep(15 * time.Second)

	exec.Command("net", "start", "postgresql-x64-15").Run() //nolint:errcheck
	exec.Command("net", "start", "postgresql").Run()        //nolint:errcheck

	p := findPsql()
	if p == "" {
		return "", fmt.Errorf("psql.exe not found after install — check PostgreSQL installation")
	}
	return p, nil
}

func installPostgresDirect() error {
	tmp := filepath.Join(os.TempDir(), "postgresql-installer.exe")
	emit("info", "Downloading PostgreSQL 15 installer (~350 MB)...")
	if err := downloadFileProgress(postgresDirectURL, tmp); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer os.Remove(tmp)

	emit("info", "Running PostgreSQL silent installer...")
	// EDB installer flags: unattended mode, no shortcuts, no stack builder
	cmd := exec.Command(tmp,
		"--mode", "unattended",
		"--unattendedmodeui", "none",
		"--superpassword", "postgres",
		"--serverport", "5432",
		"--servicename", "postgresql-x64-15",
		"--disable-components", "stackbuilder",
	)
	out, err := cmd.CombinedOutput()
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) != "" {
			emit("info", "  pg: "+line)
		}
	}
	if err != nil {
		return fmt.Errorf("installer exited: %w — %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// findPsql searches known PostgreSQL install paths and PATH.
func findPsql() string {
	// Check PATH first
	if p, err := exec.LookPath("psql"); err == nil {
		return p
	}
	// Search common install dirs
	programFiles := os.Getenv("ProgramFiles")
	if programFiles == "" {
		programFiles = `C:\Program Files`
	}
	for _, ver := range []string{"17", "16", "15", "14", "13"} {
		p := filepath.Join(programFiles, "PostgreSQL", ver, "bin", "psql.exe")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func wingetInstall(pkg string) error {
	return wingetInstallWithFlags(pkg, false)
}

func wingetInstallWithFlags(pkg string, ignoreHash bool) error {
	args := []string{
		"install", "--id", pkg,
		"--silent",
		"--accept-package-agreements",
		"--accept-source-agreements",
		"--scope", "machine",
	}
	if ignoreHash {
		args = append(args, "--ignore-security-hash")
	}
	cmd := exec.Command("winget", args...)
	out, err := cmd.CombinedOutput()
	outStr := string(out)

	// Stream output to the UI log
	for _, line := range strings.Split(outStr, "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) != "" {
			emit("info", "  winget: "+line)
		}
	}

	if err != nil {
		// Already installed is not an error
		if strings.Contains(outStr, "already installed") || strings.Contains(outStr, "No applicable upgrade") {
			emit("info", pkg+" is already installed")
			return nil
		}
		// Source data missing/corrupt (0x8a15000f) — the winget package index is
		// broken. `source update` is not enough; we must reset and rebuild it.
		if !ignoreHash && strings.Contains(outStr, "0x8a15000f") {
			emit("warn", "winget source index corrupt (0x8a15000f) — resetting and rebuilding package index...")
			exec.Command("winget", "source", "reset", "--force").Run()                                    //nolint:errcheck
			exec.Command("winget", "source", "update", "--accept-source-agreements").Run()               //nolint:errcheck
			// Small pause to let the index finish writing before the next search
			time.Sleep(5 * time.Second)
			emit("info", "winget sources reset — retrying install")
			return wingetInstallWithFlags(pkg, false)
		}
		// Hash mismatch — enable InstallerHashOverride then retry with --ignore-security-hash.
		// The flag requires the setting to be enabled first; otherwise winget prints help and exits non-zero.
		if !ignoreHash && (strings.Contains(outStr, "InstallerHashOverride") || strings.Contains(outStr, "hash mismatch") || strings.Contains(outStr, "installer hash")) {
			emit("warn", "Hash mismatch — enabling InstallerHashOverride and retrying")
			if settingsOut, settingsErr := exec.Command("winget", "settings", "--enable", "InstallerHashOverride").CombinedOutput(); settingsErr != nil {
				emit("warn", "Could not enable InstallerHashOverride: "+strings.TrimSpace(string(settingsOut)))
			} else {
				emit("info", "InstallerHashOverride enabled")
			}
			return wingetInstallWithFlags(pkg, true)
		}
		return fmt.Errorf("winget exited with error: %s", strings.TrimSpace(outStr))
	}
	return nil
}

// ── Node.js ────────────────────────────────────────────────────────────────

// nodejs20DirectURL is the official Node.js 20 LTS MSI for Windows x64.
const nodejs20DirectURL = "https://nodejs.org/dist/v20.19.0/node-v20.19.0-x64.msi"

func installNodeJS() (string, error) {
	if p := findNode(); p != "" {
		emit("info", "Node.js already installed: "+p)
		return p, nil
	}

	// Try winget first.
	if err := installViaWinget("OpenJS.NodeJS.LTS", "Node.js LTS"); err == nil {
		time.Sleep(5 * time.Second)
		if p := findNode(); p != "" {
			return p, nil
		}
	}

	// Fall back to direct MSI download.
	if err := installNodeJSDirect(); err != nil {
		return "", fmt.Errorf("Node.js install failed: %w", err)
	}

	time.Sleep(5 * time.Second)

	p := findNode()
	if p == "" {
		return "", fmt.Errorf("node.exe not found after install")
	}
	return p, nil
}

func installNodeJSDirect() error {
	tmp := filepath.Join(os.TempDir(), "nodejs-installer.msi")
	emit("info", "Downloading Node.js 20 LTS MSI (~30 MB)...")
	if err := downloadFileProgress(nodejs20DirectURL, tmp); err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer os.Remove(tmp)

	emit("info", "Running Node.js silent installer...")
	cmd := exec.Command("msiexec", "/i", tmp, "/qn", "/norestart",
		"ADDLOCAL=ALL",
	)
	out, err := cmd.CombinedOutput()
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) != "" {
			emit("info", "  node: "+line)
		}
	}
	if err != nil {
		return fmt.Errorf("msiexec exited: %w — %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// installViaWinget installs a package by winget ID silently.
// Returns an error if winget is unavailable or the install fails.
func installViaWinget(packageID, displayName string) error {
	winget, err := exec.LookPath("winget")
	if err != nil {
		return fmt.Errorf("winget not found")
	}
	emit("info", fmt.Sprintf("Installing %s via winget (id: %s)...", displayName, packageID))
	cmd := exec.Command(winget,
		"install", "-e",
		"--id", packageID,
		"--accept-source-agreements",
		"--accept-package-agreements",
		"--silent",
	)
	if err := streamCmd(cmd); err != nil {
		return fmt.Errorf("winget install %s: %w", packageID, err)
	}
	emit("ok", displayName+" installed via winget")
	return nil
}

func findNode() string {
	if p, err := exec.LookPath("node"); err == nil {
		return p
	}
	for _, base := range []string{
		`C:\Program Files\nodejs`,
		`C:\Program Files (x86)\nodejs`,
		filepath.Join(os.Getenv("ProgramFiles"), "nodejs"),
	} {
		p := filepath.Join(base, "node.exe")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// ── App bundle ─────────────────────────────────────────────────────────────


func downloadAppBundle(installDir, cfgVersion string) error {
	// Fast path: bundle was embedded at build time (built with -tags embedded).
	if len(embeddedWindowsBundle) > 0 {
		emit("info", "Extracting embedded bundle...")
		tmp := filepath.Join(os.TempDir(), "ezhealthkonnect-bundle.zip")
		if err := os.WriteFile(tmp, embeddedWindowsBundle, 0644); err != nil {
			return fmt.Errorf("failed to write embedded bundle: %w", err)
		}
		defer os.Remove(tmp)
		return extractZip(tmp, installDir, true)
	}

	// Side-by-side bundle: if a zip sits next to the installer exe, use it.
	// Supports offline installs and dev testing without a GitHub release.
	if exePath, err := os.Executable(); err == nil {
		sidecar := filepath.Join(filepath.Dir(exePath), "ezhealthkonnect-windows-amd64.zip")
		if _, statErr := os.Stat(sidecar); statErr == nil {
			emit("info", "Using local bundle: ezhealthkonnect-windows-amd64.zip")
			emit("info", "Extracting...")
			return extractZip(sidecar, installDir, true)
		}
	}

	// Slow path: download from GitHub releases.
	// Priority: 1) explicit version from config UI  2) build-time version
	// injected via -X main.version=...  3) GitHub API latest release.
	tag := cfgVersion
	if tag == "" || tag == "latest" {
		tag = version // build-time variable set by release workflow
	}
	if tag == "" || tag == "latest" {
		emit("info", "Resolving latest release from GitHub...")
		resolved, err := resolveLatestTag()
		if err != nil {
			return fmt.Errorf("could not resolve latest version from GitHub API: %w", err)
		}
		tag = resolved
		emit("info", "Latest release: "+tag)
	}

	url := releasesBase + "/download/" + tag + "/ezhealthkonnect-windows-amd64.zip"
	emit("info", "Downloading bundle "+tag+" from GitHub releases...")
	tmp := filepath.Join(os.TempDir(), "ezhealthkonnect-bundle.zip")
	if err := downloadFileProgress(url, tmp); err != nil {
		return fmt.Errorf("download failed (%s): %w", url, err)
	}
	defer os.Remove(tmp)

	emit("info", "Extracting...")
	return extractZip(tmp, installDir, true)
}


// ── Database setup ─────────────────────────────────────────────────────────

func setupDatabase(psqlPath string, cfg *Config, isLocal bool) error {
	pgBin := filepath.Dir(psqlPath)
	pgPass := pgAdminPassword()

	runPsql := func(db, query string) error {
		cmd := exec.Command(
			filepath.Join(pgBin, "psql.exe"),
			"-h", cfg.DBHost,
			"-U", "postgres",
			"-p", cfg.DBPort,
			"-d", db,
			"-c", query,
			"-q",
		)
		cmd.Env = append(os.Environ(), "PGPASSWORD="+pgPass)
		out, err := cmd.CombinedOutput()
		if err != nil && !strings.Contains(string(out), "already exists") {
			return fmt.Errorf("psql: %s: %w", strings.TrimSpace(string(out)), err)
		}
		return nil
	}

	if !isLocal {
		// External DB — user/database must already exist; only verify connectivity.
		emit("info", "Skipping user/database creation (external PostgreSQL)")
		verifyCmd := exec.Command(
			filepath.Join(pgBin, "psql.exe"),
			"-h", cfg.DBHost,
			"-U", cfg.DBUser,
			"-p", cfg.DBPort,
			"-d", cfg.DBName,
			"-c", "SELECT 1;",
			"-q",
		)
		verifyCmd.Env = append(os.Environ(), "PGPASSWORD="+cfg.DBPassword)
		if out, err := verifyCmd.CombinedOutput(); err != nil {
			return fmt.Errorf("cannot connect to external database at %s:%s/%s: %s",
				cfg.DBHost, cfg.DBPort, cfg.DBName, strings.TrimSpace(string(out)))
		}
		emit("ok", "Connected to external database")
		return nil
	}

	if err := runPsql("postgres",
		fmt.Sprintf("CREATE USER %s WITH PASSWORD '%s';", cfg.DBUser, cfg.DBPassword)); err != nil {
		emit("info", "User may already exist — continuing")
	}
	if err := runPsql("postgres",
		fmt.Sprintf("CREATE DATABASE %s OWNER %s;", cfg.DBName, cfg.DBUser)); err != nil {
		emit("info", "Database may already exist — continuing")
	}
	runPsql("postgres", //nolint:errcheck
		fmt.Sprintf("GRANT ALL PRIVILEGES ON DATABASE %s TO %s;", cfg.DBName, cfg.DBUser))

	return nil
}


func runMigrations(psqlPath string, cfg *Config) error {
	pgBin := filepath.Dir(psqlPath)
	psqlBin := filepath.Join(pgBin, "psql.exe")
	migDir := filepath.Join(cfg.InstallDir, "database", "migrations")
	return runMigrationsWithPsql(psqlBin, migDir,
		cfg.DBHost, cfg.DBPort, cfg.DBName, cfg.DBUser, cfg.DBPassword)
}

// ── .env ───────────────────────────────────────────────────────────────────

func writeStandaloneEnvFile(path string, cfg *Config) error {
	session := randomSecret(48)
	jwt := randomSecret(48)

	content := fmt.Sprintf(`# ezHealthKonnect - Production Environment
# Generated by installer on %s
# Keep this file private.

PORT=%s
API_PORT=%s

DB_HOST=%s
DB_PORT=%s
DB_NAME=%s
DB_USER=%s
DB_PASSWORD=%s
DB_SSL=false

SESSION_SECRET=%s
JWT_SECRET=%s
# Standalone installs run over plain HTTP — disable secure-only cookie flag.
SESSION_COOKIE_SECURE=false

NODE_ENV=production
LOG_LEVEL=info

OBJECT_STORAGE_DRIVER=local
LOCAL_STORAGE_PATH=%s\storage

# HL7/FHIR schema files — bundled with the installer
EZHEALTHKONNECT_SCHEMA_DIR=%s\schemas
FHIR_SCHEMA_DIR=%s\schemas\fhir

AI_ENABLED=%v
OLLAMA_URL=http://localhost:11434
OLLAMA_CHAT_MODEL=llama3.2:3b
OLLAMA_EMBED_MODEL=nomic-embed-text
`,
		time.Now().Format("2006-01-02 15:04"),
		cfg.AppPort, cfg.APIPort,
		cfg.DBHost, cfg.DBPort, cfg.DBName, cfg.DBUser, cfg.DBPassword,
		session, jwt,
		cfg.InstallDir,
		cfg.InstallDir,
		cfg.InstallDir,
		cfg.WithAI,
	)
	return os.WriteFile(path, []byte(content), 0600)
}

// ── WinSW + Windows Services ───────────────────────────────────────────────
// WinSW wraps any executable as a Windows service using an XML config file.
// Single exe download from GitHub — no zip extraction needed.

func ensureWinSW(installDir string) (string, error) {
	toolsDir := filepath.Join(installDir, "tools")
	winswExe := filepath.Join(toolsDir, "winsw.exe")

	if _, err := os.Stat(winswExe); err == nil {
		return winswExe, nil
	}

	emit("info", "Downloading WinSW service manager...")
	if err := downloadFileProgress(winswDownload, winswExe); err != nil {
		return "", fmt.Errorf("download WinSW: %w", err)
	}
	emit("ok", "WinSW downloaded")
	return winswExe, nil
}

func writeWinSWConfig(path, id, name, desc, exe, args, workDir, logFile string) error {
	content := fmt.Sprintf(`<service>
  <id>%s</id>
  <name>%s</name>
  <description>%s</description>
  <executable>%s</executable>
  <arguments>%s</arguments>
  <workingdirectory>%s</workingdirectory>
  <logpath>%s</logpath>
  <log mode="roll-by-size"><sizeThreshold>10240</sizeThreshold><keepFiles>3</keepFiles></log>
  <startmode>Automatic</startmode>
  <onfailure action="restart" delay="10 sec"/>
</service>`, id, name, desc, exe, args, workDir, logFile)
	return os.WriteFile(path, []byte(content), 0644)
}

// installWinSWService copies the WinSW binary to tools/<id>.exe, writes the XML
// config alongside it (WinSW v2 locates config by replacing .exe→.xml in its own path),
// removes any previous instance of the service, then runs "<id>.exe install".
// Returns the path to the service executable (used later to start/stop).
func installWinSWService(winswPath, toolsDir, id, name, desc, exe, args, workDir, logsDir string) (string, error) {
	svcExe := filepath.Join(toolsDir, id+".exe")
	svcCfg := filepath.Join(toolsDir, id+".xml")

	// Copy winsw.exe → <id>.exe so WinSW picks up <id>.xml automatically.
	src, err := os.Open(winswPath)
	if err != nil {
		return "", fmt.Errorf("open winsw: %w", err)
	}
	dst, err := os.OpenFile(svcExe, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		src.Close()
		return "", fmt.Errorf("create %s: %w", svcExe, err)
	}
	_, err = io.Copy(dst, src)
	src.Close()
	dst.Close()
	if err != nil {
		return "", fmt.Errorf("copy winsw to %s: %w", svcExe, err)
	}

	if err := writeWinSWConfig(svcCfg, id, name, desc, exe, args, workDir, logsDir); err != nil {
		return "", fmt.Errorf("write config %s: %w", svcCfg, err)
	}

	removeService(id)

	// WinSW v2: run "<id>.exe install" with no arguments — it finds <id>.xml itself.
	if out, err := exec.Command(svcExe, "install").CombinedOutput(); err != nil {
		return "", fmt.Errorf("winsw install %s: %s: %w", id, strings.TrimSpace(string(out)), err)
	}
	return svcExe, nil
}

func registerWindowsServices(winswPath, nodePath, goAPIBin string, cfg *Config) error {
	toolsDir := filepath.Join(cfg.InstallDir, "tools")
	logsDir := filepath.Join(cfg.InstallDir, "logs")
	os.MkdirAll(logsDir, 0755) //nolint:errcheck

	// ── Go API service ──────────────────────────────────────────────────────
	apiExe, err := installWinSWService(winswPath, toolsDir,
		"ezhealthkonnect-api",
		"ezHealthKonnect Go API", "ezHealthKonnect HL7/FHIR Go Backend",
		goAPIBin, "",
		cfg.InstallDir, logsDir,
	)
	if err != nil {
		return err
	}

	// ── Node.js frontend service ────────────────────────────────────────────
	serverJS := filepath.Join(cfg.InstallDir, "server.js")
	appExe, err := installWinSWService(winswPath, toolsDir,
		"ezhealthkonnect",
		"ezHealthKonnect Web", "ezHealthKonnect Web Frontend",
		nodePath, serverJS,
		cfg.InstallDir, logsDir,
	)
	if err != nil {
		return err
	}

	// Start services
	emit("info", "Starting services...")
	if out, err := exec.Command(apiExe, "start").CombinedOutput(); err != nil {
		emit("warn", fmt.Sprintf("Could not start go-api: %s", strings.TrimSpace(string(out))))
	}
	time.Sleep(3 * time.Second)
	if out, err := exec.Command(appExe, "start").CombinedOutput(); err != nil {
		emit("warn", fmt.Sprintf("Could not start node: %s", strings.TrimSpace(string(out))))
	}

	return nil
}

// ── Windows Registry (Add/Remove Programs) ─────────────────────────────────

const uninstallRegKey = `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\ezHealthKonnect`

// writeUninstallRegistry creates an entry in Add/Remove Programs so the user
// can uninstall from the Windows Settings app or control panel.
func writeUninstallRegistry(installDir, appPort, installerExe, ver string) {
	uninstallCmd := fmt.Sprintf(`"%s" --uninstall`, installerExe)
	pairs := [][2]string{
		{"DisplayName", "ezHealthKonnect"},
		{"DisplayVersion", ver},
		{"Publisher", "ezInterOp Solutions"},
		{"InstallLocation", installDir},
		{"UninstallString", uninstallCmd},
		{"QuietUninstallString", uninstallCmd},
		{"DisplayIcon", filepath.Join(installDir, "go-api.exe")},
		{"URLInfoAbout", "https://www.ezInterOpSolutions.com"},
		{"Comments", "Healthcare HL7 to FHIR Integration Platform"},
		{"NoModify", "1"},
		{"NoRepair", "1"},
	}
	for _, p := range pairs {
		t := "REG_SZ"
		if p[0] == "NoModify" || p[0] == "NoRepair" {
			t = "REG_DWORD"
		}
		exec.Command("reg", "add", `HKLM\`+uninstallRegKey, "/v", p[0], "/t", t, "/d", p[1], "/f").Run() //nolint:errcheck
	}
}

func removeUninstallRegistry() {
	exec.Command("reg", "delete", `HKLM\`+uninstallRegKey, "/f").Run() //nolint:errcheck
}

func removeService(name string) {
	exec.Command("sc", "stop", name).Run()   //nolint:errcheck
	exec.Command("sc", "delete", name).Run() //nolint:errcheck
}

// runStandaloneUninstall stops services, drops the database, and removes all files.
func runStandaloneUninstall(cfg *Config) {
	const svcAPI = "ezhealthkonnect-api"
	const svcApp = "ezhealthkonnect"

	// ── 1. Stop services before touching anything else ─────────────────────
	emit("info", "── Stopping and removing Windows services")
	removeService(svcAPI)
	removeService(svcApp)
	emit("ok", "Services stopped and removed")

	// ── 2. Drop database + user ────────────────────────────────────────────
	// Read credentials from the installed .env so we use exactly what was
	// configured at install time, even if defaults were changed.
	emit("info", "── Dropping PostgreSQL database and user")
	dbCfg := loadEnvForUninstall(cfg.InstallDir)
	if err := dropDatabaseAndUser(dbCfg); err != nil {
		emit("warn", "Database cleanup had issues (may already be removed): "+err.Error())
	} else {
		emit("ok", fmt.Sprintf("Database '%s' and user '%s' dropped", dbCfg.dbName, dbCfg.dbUser))
	}

	// ── 3. Remove app files ────────────────────────────────────────────────
	emit("info", "── Removing install directory: "+cfg.InstallDir)
	if err := os.RemoveAll(cfg.InstallDir); err != nil {
		emit("warn", "Could not fully remove "+cfg.InstallDir+": "+err.Error())
	} else {
		emit("ok", "Install directory removed")
	}

	// ── 4. Remove Add/Remove Programs entry ────────────────────────────────
	emit("info", "── Removing Add/Remove Programs entry")
	removeUninstallRegistry()
	emit("ok", "Registry entry removed")

	emit("ok", "Uninstall complete — you may close this window")
}

// dbUninstallConfig holds the minimal DB info needed to drop during uninstall.
type dbUninstallConfig struct {
	host     string
	port     string
	dbName   string
	dbUser   string
	psqlPath string
}

// loadEnvForUninstall reads the .env file from installDir and extracts DB settings.
// Falls back to installer defaults if the file cannot be read.
func loadEnvForUninstall(installDir string) dbUninstallConfig {
	cfg := dbUninstallConfig{
		host:     "localhost",
		port:     "5432",
		dbName:   "ezhealthkonnect",
		dbUser:   "ezhealth_user",
		psqlPath: findPsql(),
	}
	env := readEnvFile(filepath.Join(installDir, ".env"))
	if v, ok := env["DB_HOST"]; ok { cfg.host = v }
	if v, ok := env["DB_PORT"]; ok { cfg.port = v }
	if v, ok := env["DB_NAME"]; ok { cfg.dbName = v }
	if v, ok := env["DB_USER"]; ok { cfg.dbUser = v }
	return cfg
}

// dropDatabaseAndUser terminates active connections then drops the DB and user.
func dropDatabaseAndUser(cfg dbUninstallConfig) error {
	if cfg.psqlPath == "" {
		return fmt.Errorf("psql.exe not found — PostgreSQL may already be removed")
	}

	pgBin := filepath.Dir(cfg.psqlPath)
	psql := filepath.Join(pgBin, "psql.exe")
	adminPw := pgAdminPassword()

	run := func(db, sql string) error {
		cmd := exec.Command(psql, "-h", cfg.host, "-U", "postgres", "-p", cfg.port, "-d", db, "-c", sql, "-q")
		cmd.Env = append(os.Environ(), "PGPASSWORD="+adminPw)
		out, err := cmd.CombinedOutput()
		if err != nil {
			msg := strings.TrimSpace(string(out))
			if strings.Contains(msg, "does not exist") {
				return nil // already gone — not an error
			}
			return fmt.Errorf("%s: %s", sql, msg)
		}
		return nil
	}

	// Terminate any active connections so DROP DATABASE doesn't fail.
	_ = run("postgres", fmt.Sprintf(
		`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s' AND pid <> pg_backend_pid();`,
		cfg.dbName))

	if err := run("postgres", fmt.Sprintf("DROP DATABASE IF EXISTS %s;", cfg.dbName)); err != nil {
		return err
	}
	if err := run("postgres", fmt.Sprintf("DROP USER IF EXISTS %s;", cfg.dbUser)); err != nil {
		return err
	}
	return nil
}
