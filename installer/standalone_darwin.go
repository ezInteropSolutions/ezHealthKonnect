//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// ── Entry points ───────────────────────────────────────────────────────────────

func runStandaloneInstallation(cfg *Config) {
	success := false
	defer func() {
		if success {
			emitDoneSignal(fmt.Sprintf("http://localhost:%s", cfg.AppPort))
		} else {
			emitInstallFailed()
		}
	}()

	emit("info", "================================================")
	emit("info", "  ezHealthKonnect -- Standalone Installation    ")
	emit("info", "================================================")
	emit("info", "")

	// ── Step 1: Directory ──────────────────────────────────────────────────────
	emitStep(1, "Preparing install directory")
	if err := os.MkdirAll(cfg.InstallDir, 0755); err != nil {
		emit("error", "Failed to create directory: "+err.Error())
		return
	}
	for _, sub := range []string{"logs", "storage", "tools"} {
		os.MkdirAll(filepath.Join(cfg.InstallDir, sub), 0755) //nolint:errcheck
	}
	emit("ok", "Directory ready: "+cfg.InstallDir)

	// ── Step 2: Homebrew ───────────────────────────────────────────────────────
	emitStep(2, "Checking Homebrew")
	if err := ensureHomebrew(); err != nil {
		emit("error", "Homebrew setup failed: "+err.Error())
		return
	}
	emit("ok", "Homebrew ready")

	// ── Step 3: PostgreSQL ─────────────────────────────────────────────────────
	if cfg.DBHost == "" {
		cfg.DBHost = "localhost"
	}
	isLocalDB := cfg.DBHost == "localhost" || cfg.DBHost == "127.0.0.1"

	if isLocalDB {
		emitStep(3, "Installing PostgreSQL 15")
	} else {
		emitStep(3, "Using existing PostgreSQL at "+cfg.DBHost)
	}
	psqlPath, err := installPostgresDarwin(isLocalDB)
	if err != nil {
		emit("error", "PostgreSQL install failed: "+err.Error())
		return
	}
	emit("ok", "PostgreSQL ready: "+psqlPath)

	// ── Step 4: Node.js ────────────────────────────────────────────────────────
	emitStep(4, "Installing Node.js 20 LTS")
	nodePath, err := installNodeDarwin()
	if err != nil {
		emit("error", "Node.js install failed: "+err.Error())
		return
	}
	emit("ok", "Node.js ready: "+nodePath)

	// ── Step 5: Extract app bundle ─────────────────────────────────────────────
	emitStep(5, "Installing ezHealthKonnect")
	if err := downloadAppBundleDarwin(cfg.InstallDir, cfg.Version); err != nil {
		emit("error", "Download failed: "+err.Error())
		return
	}
	emit("ok", "Application extracted to "+cfg.InstallDir)

	// ── Step 6: npm install ────────────────────────────────────────────────────
	emitStep(6, "Installing Node.js dependencies")
	npmCmd := exec.Command("npm", "install", "--omit=dev", "--silent")
	npmCmd.Dir = cfg.InstallDir
	if err := streamCmd(npmCmd); err != nil {
		emit("warn", "npm install had warnings: "+err.Error())
	} else {
		emit("ok", "Node.js dependencies installed")
	}

	// ── Step 7: Database setup ─────────────────────────────────────────────────
	emitStep(7, "Setting up PostgreSQL database")
	if err := setupDatabaseDarwin(psqlPath, cfg, isLocalDB); err != nil {
		emit("error", "Database setup failed: "+err.Error())
		return
	}
	emit("ok", "Database ready")

	// ── Step 8: Write .env ─────────────────────────────────────────────────────
	emitStep(8, "Writing configuration")
	envPath := filepath.Join(cfg.InstallDir, ".env")
	if err := writeDarwinEnvFile(envPath, cfg); err != nil {
		emit("error", "Failed to write .env: "+err.Error())
		return
	}
	emit("ok", "Configuration saved")

	// ── Step 9: Run migrations ─────────────────────────────────────────────────
	emitStep(9, "Running database migrations")
	migDir := filepath.Join(cfg.InstallDir, "database", "migrations")
	if err := runMigrationsWithPsql(psqlPath, migDir,
		cfg.DBHost, cfg.DBPort, cfg.DBName, cfg.DBUser, cfg.DBPassword); err != nil {
		emit("warn", "Migrations had errors (may already be applied): "+err.Error())
	} else {
		emit("ok", "Migrations complete")
	}

	// ── Step 10: Register services ─────────────────────────────────────────────
	if cfg.RegisterSvc {
		emitStep(10, "Registering launchd services")
		goAPIBin := filepath.Join(cfg.InstallDir, "go-api")
		os.Chmod(goAPIBin, 0755) //nolint:errcheck
		if err := registerDarwinServices(nodePath, goAPIBin, envPath, cfg); err != nil {
			emit("warn", "Service registration failed: "+err.Error())
			emit("warn", "Start manually: node server.js & ./go-api &")
		} else {
			emit("ok", "launchd services registered (auto-start on login)")
		}
	}

	// ── Step 11: Health check ──────────────────────────────────────────────────
	emitStep(11, "Waiting for application to become ready")
	appURL := fmt.Sprintf("http://localhost:%s", cfg.AppPort)
	waitForReady(appURL+"/health", 120*time.Second)

	emit("info", "")
	emit("info", "================================================")
	emit("ok", "  Installation Complete!")
	emit("info", "================================================")
	emit("info", "")
	emit("ok", "Platform URL: "+appURL)
	emit("info", "Default login: admin@ezhealthkonnect.com / admin123")
	emit("warn", "Change the default password immediately after first login.")
	emit("info", "")
	emit("info", "Note: services will auto-start on next login.")
	emit("info", "To uninstall: run this installer again and click Uninstall.")
	success = true
}

func runStandaloneUninstall(cfg *Config) {
	defer func() { emitDoneSignal("") }()

	emit("info", "================================================")
	emit("info", "  ezHealthKonnect -- Uninstalling")
	emit("info", "================================================")

	// ── 1. Unload launchd services ─────────────────────────────────────────────
	emit("info", "-- Stopping launchd services")
	plists := []string{
		"/Library/LaunchDaemons/com.ezhealthkonnect.api.plist",
		"/Library/LaunchDaemons/com.ezhealthkonnect.plist",
	}
	for _, p := range plists {
		exec.Command("sudo", "launchctl", "unload", p).Run() //nolint:errcheck
		exec.Command("sudo", "rm", "-f", p).Run()            //nolint:errcheck
	}
	emit("ok", "Services stopped and removed")

	// ── 2. Drop database + user ────────────────────────────────────────────────
	emit("info", "-- Dropping PostgreSQL database and user")
	env := readEnvFile(filepath.Join(cfg.InstallDir, ".env"))
	dbHost := env["DB_HOST"]
	dbPort := env["DB_PORT"]
	dbName := env["DB_NAME"]
	dbUser := env["DB_USER"]
	if dbHost == "" { dbHost = "localhost" }
	if dbPort == "" { dbPort = "5432" }
	if dbName == "" { dbName = "ezhealthkonnect" }
	if dbUser == "" { dbUser = "ezhealth_user" }

	if psql := findPsqlDarwin(); psql != "" {
		dropDatabaseDarwin(psql, dbHost, dbPort, dbName, dbUser)
	} else {
		emit("warn", "psql not found — skipping database cleanup")
	}

	// ── 3. Remove app files ────────────────────────────────────────────────────
	emit("info", "-- Removing install directory: "+cfg.InstallDir)
	if err := os.RemoveAll(cfg.InstallDir); err != nil {
		exec.Command("sudo", "rm", "-rf", cfg.InstallDir).Run() //nolint:errcheck
	}
	emit("ok", "Install directory removed")

	emit("ok", "Uninstall complete")
}

// ── Homebrew ───────────────────────────────────────────────────────────────────

func ensureHomebrew() error {
	if _, err := exec.LookPath("brew"); err == nil {
		// Already installed — update index quietly
		exec.Command("brew", "update", "--quiet").Run() //nolint:errcheck
		return nil
	}
	emit("info", "Installing Homebrew (this may take a few minutes)...")
	installCmd := `/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"`
	cmd := exec.Command("/bin/bash", "-c", "NONINTERACTIVE=1 "+installCmd)
	if err := streamCmd(cmd); err != nil {
		return fmt.Errorf("homebrew install: %w", err)
	}
	// Add brew to PATH for the current process (Apple Silicon default path)
	for _, brewPath := range []string{"/opt/homebrew/bin", "/usr/local/bin"} {
		if _, err := os.Stat(filepath.Join(brewPath, "brew")); err == nil {
			os.Setenv("PATH", brewPath+":"+os.Getenv("PATH"))
			break
		}
	}
	return nil
}

func brewBin() string {
	for _, p := range []string{"/opt/homebrew/bin/brew", "/usr/local/bin/brew"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if p, _ := exec.LookPath("brew"); p != "" {
		return p
	}
	return "brew"
}

// ── PostgreSQL ─────────────────────────────────────────────────────────────────

func installPostgresDarwin(isLocal bool) (string, error) {
	if !isLocal {
		if p := findPsqlDarwin(); p != "" {
			return p, nil
		}
		return "", fmt.Errorf("psql not found — install PostgreSQL client via Homebrew: brew install libpq")
	}

	if p := findPsqlDarwin(); p != "" {
		emit("info", "PostgreSQL already installed: "+p)
		// Ensure the service is running
		exec.Command(brewBin(), "services", "start", "postgresql@15").Run() //nolint:errcheck
		return p, nil
	}

	emit("info", "Installing PostgreSQL 15 via Homebrew...")
	if err := streamCmd(exec.Command(brewBin(), "install", "postgresql@15")); err != nil {
		return "", fmt.Errorf("brew install postgresql@15: %w", err)
	}

	emit("info", "Starting PostgreSQL service...")
	exec.Command(brewBin(), "services", "start", "postgresql@15").Run() //nolint:errcheck
	time.Sleep(8 * time.Second)

	p := findPsqlDarwin()
	if p == "" {
		return "", fmt.Errorf("psql not found after brew install — try: brew link postgresql@15 --force")
	}
	return p, nil
}

func findPsqlDarwin() string {
	if p, _ := exec.LookPath("psql"); p != "" {
		return p
	}
	candidates := []string{
		// Apple Silicon (arm64)
		"/opt/homebrew/opt/postgresql@15/bin/psql",
		"/opt/homebrew/opt/postgresql@16/bin/psql",
		"/opt/homebrew/bin/psql",
		// Intel (amd64)
		"/usr/local/opt/postgresql@15/bin/psql",
		"/usr/local/opt/postgresql@16/bin/psql",
		"/usr/local/bin/psql",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func setupDatabaseDarwin(psqlPath string, cfg *Config, isLocal bool) error {
	if !isLocal {
		cmd := exec.Command(psqlPath,
			"-h", cfg.DBHost, "-U", cfg.DBUser, "-p", cfg.DBPort,
			"-d", cfg.DBName, "-c", "SELECT 1;", "-q")
		cmd.Env = append(os.Environ(), "PGPASSWORD="+cfg.DBPassword)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("cannot connect to database at %s:%s/%s: %s",
				cfg.DBHost, cfg.DBPort, cfg.DBName, strings.TrimSpace(string(out)))
		}
		emit("ok", "Connected to external database")
		return nil
	}

	// Homebrew PostgreSQL on macOS defaults to trust auth for local socket connections.
	// We can run psql without a password as the current user if the postgres role exists.
	runAdmin := func(sql string) {
		// Try socket connection (trust auth — Homebrew default)
		cmd := exec.Command(psqlPath, "-d", "postgres", "-c", sql, "-q")
		out, err := cmd.CombinedOutput()
		if err != nil && !strings.Contains(string(out), "already exists") {
			// Fallback: PGPASSWORD auth
			cmd2 := exec.Command(psqlPath, "-h", "localhost", "-U", "postgres",
				"-d", "postgres", "-c", sql, "-q")
			cmd2.Env = append(os.Environ(), "PGPASSWORD="+pgAdminPassword())
			if out2, err2 := cmd2.CombinedOutput(); err2 != nil &&
				!strings.Contains(string(out2), "already exists") {
				emit("warn", fmt.Sprintf("  psql admin: %s", strings.TrimSpace(string(out2))))
			}
		}
	}

	runAdmin(fmt.Sprintf("CREATE USER %s WITH PASSWORD '%s';", cfg.DBUser, cfg.DBPassword))
	runAdmin(fmt.Sprintf("CREATE DATABASE %s OWNER %s;", cfg.DBName, cfg.DBUser))
	runAdmin(fmt.Sprintf("GRANT ALL PRIVILEGES ON DATABASE %s TO %s;", cfg.DBName, cfg.DBUser))
	return nil
}

func dropDatabaseDarwin(psqlPath, host, port, dbName, dbUser string) {
	runAdmin := func(sql string) {
		cmd := exec.Command(psqlPath, "-d", "postgres", "-c", sql, "-q")
		cmd.CombinedOutput() //nolint:errcheck
	}
	runAdmin(fmt.Sprintf(
		`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s' AND pid <> pg_backend_pid();`,
		dbName))
	runAdmin("DROP DATABASE IF EXISTS " + dbName + ";")
	runAdmin("DROP USER IF EXISTS " + dbUser + ";")
	emit("ok", fmt.Sprintf("Database '%s' and user '%s' dropped", dbName, dbUser))
}

// ── Node.js ────────────────────────────────────────────────────────────────────

func installNodeDarwin() (string, error) {
	if p := findNodeDarwin(); p != "" {
		emit("info", "Node.js already installed: "+p)
		return p, nil
	}
	emit("info", "Installing Node.js 20 LTS via Homebrew...")
	if err := streamCmd(exec.Command(brewBin(), "install", "node@20")); err != nil {
		// Fallback: generic node
		if err2 := streamCmd(exec.Command(brewBin(), "install", "node")); err2 != nil {
			return "", fmt.Errorf("brew install node: %w", err2)
		}
	}
	time.Sleep(3 * time.Second)
	p := findNodeDarwin()
	if p == "" {
		return "", fmt.Errorf("node not found after brew install — try: brew link node@20 --force")
	}
	return p, nil
}

func findNodeDarwin() string {
	if p, _ := exec.LookPath("node"); p != "" {
		return p
	}
	candidates := []string{
		"/opt/homebrew/opt/node@20/bin/node",
		"/opt/homebrew/bin/node",
		"/usr/local/opt/node@20/bin/node",
		"/usr/local/bin/node",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// ── App bundle ─────────────────────────────────────────────────────────────────

func downloadAppBundleDarwin(installDir, cfgVersion string) error {
	if len(embeddedDarwinBundle) > 0 {
		emit("info", "Extracting embedded bundle...")
		tmp := filepath.Join(os.TempDir(), "ezhealthkonnect-bundle.zip")
		if err := os.WriteFile(tmp, embeddedDarwinBundle, 0644); err != nil {
			return fmt.Errorf("failed to write embedded bundle: %w", err)
		}
		defer os.Remove(tmp)
		return extractZip(tmp, installDir, true)
	}

	// Side-car bundle next to the installer binary
	if exePath, err := os.Executable(); err == nil {
		for _, name := range []string{
			"ezhealthkonnect-darwin-arm64.zip",
			"ezhealthkonnect-darwin-amd64.zip",
		} {
			sidecar := filepath.Join(filepath.Dir(exePath), name)
			if _, statErr := os.Stat(sidecar); statErr == nil {
				emit("info", "Using local bundle: "+name)
				return extractZip(sidecar, installDir, true)
			}
		}
	}

	// GitHub releases fallback
	tag := cfgVersion
	if tag == "" || tag == "latest" {
		tag = version
	}
	if tag == "" || tag == "latest" {
		emit("info", "Resolving latest release from GitHub...")
		resolved, err := resolveLatestTag()
		if err != nil {
			return fmt.Errorf("could not resolve latest version: %w", err)
		}
		tag = resolved
		emit("info", "Latest release: "+tag)
	}

	arch := "arm64"
	if runtime.GOARCH == "amd64" {
		arch = "amd64"
	}
	url := fmt.Sprintf("%s/download/%s/ezhealthkonnect-darwin-%s.zip", releasesBase, tag, arch)
	emit("info", fmt.Sprintf("Downloading bundle %s from GitHub releases...", tag))
	tmp := filepath.Join(os.TempDir(), "ezhealthkonnect-bundle.zip")
	if err := downloadFileProgress(url, tmp); err != nil {
		return fmt.Errorf("download failed (%s): %w", url, err)
	}
	defer os.Remove(tmp)
	emit("info", "Extracting...")
	return extractZip(tmp, installDir, true)
}

// ── .env ───────────────────────────────────────────────────────────────────────

func writeDarwinEnvFile(path string, cfg *Config) error {
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
SESSION_COOKIE_SECURE=false

NODE_ENV=production
LOG_LEVEL=info

OBJECT_STORAGE_DRIVER=local
LOCAL_STORAGE_PATH=%s/storage

# HL7/FHIR schema files -- bundled with the installer
EZHEALTHKONNECT_SCHEMA_DIR=%s/schemas
FHIR_SCHEMA_DIR=%s/schemas/fhir

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

// ── launchd services ───────────────────────────────────────────────────────────

func registerDarwinServices(nodePath, goAPIBin, envPath string, cfg *Config) error {
	logsDir := filepath.Join(cfg.InstallDir, "logs")
	os.MkdirAll(logsDir, 0755) //nolint:errcheck

	// Write wrapper scripts that source .env before starting the process.
	// launchd does not support EnvironmentFile natively.
	scripts := []struct {
		path    string
		content string
	}{
		{
			filepath.Join(cfg.InstallDir, "tools", "start-api.sh"),
			fmt.Sprintf("#!/bin/bash\nset -a; source %s; set +a; exec %s\n", envPath, goAPIBin),
		},
		{
			filepath.Join(cfg.InstallDir, "tools", "start-node.sh"),
			fmt.Sprintf("#!/bin/bash\nset -a; source %s; set +a; exec %s %s/server.js\n",
				envPath, nodePath, cfg.InstallDir),
		},
	}
	for _, s := range scripts {
		if err := os.WriteFile(s.path, []byte(s.content), 0755); err != nil {
			return fmt.Errorf("write wrapper script %s: %w", s.path, err)
		}
	}

	type plistDef struct {
		label    string
		plistPath string
		script   string
		logOut   string
		logErr   string
	}
	plists := []plistDef{
		{
			"com.ezhealthkonnect.api",
			"/Library/LaunchDaemons/com.ezhealthkonnect.api.plist",
			filepath.Join(cfg.InstallDir, "tools", "start-api.sh"),
			filepath.Join(logsDir, "api.out.log"),
			filepath.Join(logsDir, "api.err.log"),
		},
		{
			"com.ezhealthkonnect",
			"/Library/LaunchDaemons/com.ezhealthkonnect.plist",
			filepath.Join(cfg.InstallDir, "tools", "start-node.sh"),
			filepath.Join(logsDir, "node.out.log"),
			filepath.Join(logsDir, "node.err.log"),
		},
	}

	for _, p := range plists {
		content := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>%s</string>
    <key>ProgramArguments</key>
    <array>
        <string>/bin/bash</string>
        <string>%s</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>%s</string>
    <key>StandardErrorPath</key>
    <string>%s</string>
</dict>
</plist>
`, p.label, p.script, p.logOut, p.logErr)

		if err := writeDarwinPlist(p.plistPath, content); err != nil {
			return fmt.Errorf("write plist %s: %w", p.plistPath, err)
		}
	}

	// Load services via launchctl
	for _, p := range plists {
		cmd := exec.Command("sudo", "launchctl", "load", "-w", p.plistPath)
		if out, err := cmd.CombinedOutput(); err != nil {
			emit("warn", fmt.Sprintf("Could not load %s: %s", p.label, strings.TrimSpace(string(out))))
		} else {
			emit("ok", "Loaded: "+p.label)
		}
		time.Sleep(2 * time.Second)
	}
	return nil
}

func writeDarwinPlist(path, content string) error {
	// Try direct write first (root), fall back to sudo
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		tmp, terr := os.CreateTemp("", "ezhk-*.plist")
		if terr != nil {
			return terr
		}
		tmp.WriteString(content) //nolint:errcheck
		tmp.Close()
		defer os.Remove(tmp.Name())
		cmd := exec.Command("sudo", "cp", tmp.Name(), path)
		if out, err2 := cmd.CombinedOutput(); err2 != nil {
			return fmt.Errorf("sudo cp to %s: %s: %w", path, strings.TrimSpace(string(out)), err2)
		}
		exec.Command("sudo", "chmod", "644", path).Run() //nolint:errcheck
	}
	return nil
}
