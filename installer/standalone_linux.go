//go:build linux

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

	// ── Step 2: PostgreSQL ─────────────────────────────────────────────────────
	if cfg.DBHost == "" {
		cfg.DBHost = "localhost"
	}
	isLocalDB := cfg.DBHost == "localhost" || cfg.DBHost == "127.0.0.1"

	if isLocalDB {
		emitStep(2, "Installing PostgreSQL 15")
	} else {
		emitStep(2, "Using existing PostgreSQL at "+cfg.DBHost)
	}
	psqlPath, err := installPostgresLinux(isLocalDB)
	if err != nil {
		emit("error", "PostgreSQL install failed: "+err.Error())
		return
	}
	emit("ok", "PostgreSQL ready: "+psqlPath)

	// ── Step 3: Node.js ────────────────────────────────────────────────────────
	emitStep(3, "Installing Node.js 20 LTS")
	nodePath, err := installNodeLinux()
	if err != nil {
		emit("error", "Node.js install failed: "+err.Error())
		return
	}
	emit("ok", "Node.js ready: "+nodePath)

	// ── Step 4: Extract app bundle ─────────────────────────────────────────────
	emitStep(4, "Installing ezHealthKonnect")
	if err := downloadAppBundleLinux(cfg.InstallDir, cfg.Version); err != nil {
		emit("error", "Download failed: "+err.Error())
		return
	}
	emit("ok", "Application extracted to "+cfg.InstallDir)

	// ── Step 5: npm install ────────────────────────────────────────────────────
	emitStep(5, "Installing Node.js dependencies")
	nodeModulesDir := filepath.Join(cfg.InstallDir, "node_modules")
	if _, err := os.Stat(nodeModulesDir); err == nil {
		emit("ok", "Node.js dependencies already bundled — skipping npm install")
	} else {
		npmCmd := exec.Command(nodePath, filepath.Join(filepath.Dir(nodePath), "npm"),
			"install", "--omit=dev", "--silent")
		npmCmd.Dir = cfg.InstallDir
		// npm is usually a script wrapper — try npm directly
		npmDirect := exec.Command("npm", "install", "--omit=dev", "--silent")
		npmDirect.Dir = cfg.InstallDir
		if err := streamCmd(npmDirect); err != nil {
			// Fallback: node path-based npm
			if err2 := streamCmd(npmCmd); err2 != nil {
				emit("warn", "npm install had warnings: "+err2.Error())
			}
		}
		emit("ok", "Node.js dependencies installed")
	}

	// ── Step 6: Database setup ─────────────────────────────────────────────────
	emitStep(6, "Setting up PostgreSQL database")
	if err := setupDatabaseLinux(psqlPath, cfg, isLocalDB); err != nil {
		emit("error", "Database setup failed: "+err.Error())
		return
	}
	emit("ok", "Database ready")

	// ── Step 7: Write .env ─────────────────────────────────────────────────────
	emitStep(7, "Writing configuration")
	envPath := filepath.Join(cfg.InstallDir, ".env")
	if err := writeLinuxEnvFile(envPath, cfg); err != nil {
		emit("error", "Failed to write .env: "+err.Error())
		return
	}
	emit("ok", "Configuration saved")

	// ── Step 8: Run migrations ─────────────────────────────────────────────────
	emitStep(8, "Running database migrations")
	migDir := filepath.Join(cfg.InstallDir, "database", "migrations")
	if err := runMigrationsWithPsql(psqlPath, migDir,
		cfg.DBHost, cfg.DBPort, cfg.DBName, cfg.DBUser, cfg.DBPassword); err != nil {
		emit("warn", "Migrations had errors (may already be applied): "+err.Error())
	} else {
		emit("ok", "Migrations complete")
	}

	// ── Step 9: Register services ──────────────────────────────────────────────
	if cfg.RegisterSvc {
		emitStep(9, "Registering systemd services")
		goAPIBin := filepath.Join(cfg.InstallDir, "go-api")
		if err := registerLinuxServices(nodePath, goAPIBin, envPath, cfg); err != nil {
			emit("warn", "Service registration failed: "+err.Error())
			emit("warn", "Start manually: node server.js & ./go-api &")
		} else {
			emit("ok", "systemd services registered (auto-start on boot)")
		}
	}

	// ── Step 10: Health check ──────────────────────────────────────────────────
	emitStep(10, "Waiting for application to become ready")
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
	success = true
}

func runStandaloneUninstall(cfg *Config) {
	defer func() { emitDoneSignal("") }()

	emit("info", "================================================")
	emit("info", "  ezHealthKonnect -- Uninstalling")
	emit("info", "================================================")

	// ── 1. Stop and disable services ───────────────────────────────────────────
	emit("info", "-- Stopping systemd services")
	for _, svc := range []string{"ezhealthkonnect", "ezhealthkonnect-api"} {
		sudoRun("systemctl", "stop", svc)
		sudoRun("systemctl", "disable", svc)
	}
	emit("ok", "Services stopped")

	// ── 2. Drop database + user ────────────────────────────────────────────────
	emit("info", "-- Dropping PostgreSQL database and user")
	env := readEnvFile(filepath.Join(cfg.InstallDir, ".env"))
	dbHost := orDefault(env["DB_HOST"], "localhost")
	dbPort := orDefault(env["DB_PORT"], "5432")
	dbName := orDefault(env["DB_NAME"], "ezhealthkonnect")
	dbUser := orDefault(env["DB_USER"], "ezhealth_user")

	psql := findPsqlLinux()
	if psql != "" {
		dropDatabaseLinux(psql, dbHost, dbPort, dbName, dbUser)
	} else {
		emit("warn", "psql not found — skipping database cleanup")
	}

	// ── 3. Remove service unit files ───────────────────────────────────────────
	emit("info", "-- Removing service unit files")
	for _, unit := range []string{
		"/etc/systemd/system/ezhealthkonnect.service",
		"/etc/systemd/system/ezhealthkonnect-api.service",
	} {
		sudoRun("rm", "-f", unit)
	}
	sudoRun("systemctl", "daemon-reload")

	// ── 4. Remove app files ────────────────────────────────────────────────────
	emit("info", "-- Removing install directory: "+cfg.InstallDir)
	if err := sudoRunErr("rm", "-rf", cfg.InstallDir); err != nil {
		emit("warn", "Could not fully remove "+cfg.InstallDir+": "+err.Error())
	} else {
		emit("ok", "Install directory removed")
	}

	emit("ok", "Uninstall complete")
}

// ── PostgreSQL ─────────────────────────────────────────────────────────────────

func installPostgresLinux(isLocal bool) (string, error) {
	if !isLocal {
		if p := findPsqlLinux(); p != "" {
			return p, nil
		}
		return "", fmt.Errorf("psql not found — install PostgreSQL client tools")
	}

	if p := findPsqlLinux(); p != "" {
		emit("info", "PostgreSQL already installed: "+p)
		return p, nil
	}

	pm := detectPackageManager()
	switch pm {
	case "apt-get":
		if err := installPostgresApt(); err != nil {
			return "", err
		}
	case "dnf":
		if err := installPostgresDnf(); err != nil {
			return "", err
		}
	case "yum":
		if err := installPostgresYum(); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("no supported package manager found (apt-get/dnf/yum) — install PostgreSQL 15 manually")
	}

	// Wait for the service to start
	time.Sleep(5 * time.Second)
	sudoRun("systemctl", "start", "postgresql")
	sudoRun("systemctl", "start", "postgresql-15")

	p := findPsqlLinux()
	if p == "" {
		return "", fmt.Errorf("psql not found after install — check PATH and PostgreSQL installation")
	}
	return p, nil
}

func installPostgresApt() error {
	emit("info", "Installing PostgreSQL 15 via apt-get (PGDG repo)...")

	// Step 1: Install prerequisites for adding the repo
	aptGetInstall("curl", "gnupg", "lsb-release") //nolint:errcheck

	// Step 2: Add the PostgreSQL Global Development Group apt repo.
	// postgresql-15 is NOT in the Ubuntu/Debian main repos on most LTS releases.
	addPgdgRepoApt()

	// Step 3: Install
	if err := aptGetInstall("postgresql-15", "postgresql-client-15"); err != nil {
		emit("warn", "postgresql-15 not available — falling back to distro postgresql package")
		if err2 := aptGetInstall("postgresql", "postgresql-client"); err2 != nil {
			return fmt.Errorf("apt-get install postgresql: %w", err2)
		}
	}
	sudoRun("systemctl", "enable", "postgresql")
	return nil
}

// addPgdgRepoApt adds the official PostgreSQL apt repository.
func addPgdgRepoApt() {
	// Detect distro codename from /etc/os-release (always present on modern Debian/Ubuntu)
	codename := ""
	data, err := os.ReadFile("/etc/os-release")
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if after, ok := strings.CutPrefix(line, "VERSION_CODENAME="); ok {
				codename = strings.Trim(after, `"'`)
				break
			}
		}
	}
	if codename == "" {
		// Fallback: lsb_release command
		if out, err := exec.Command("lsb_release", "-cs").Output(); err == nil {
			codename = strings.TrimSpace(string(out))
		}
	}
	if codename == "" {
		emit("warn", "  Could not detect distro codename — skipping PGDG repo setup")
		return
	}

	emit("info", fmt.Sprintf("  Adding PGDG apt repo for %s...", codename))

	// Add signing key
	keyCmd := exec.Command("bash", "-c",
		`curl -fsSL https://www.postgresql.org/media/keys/ACCC4CF8.asc | gpg --dearmor -o /etc/apt/trusted.gpg.d/postgresql.gpg`)
	if !isRoot() {
		keyCmd = exec.Command("bash", "-c",
			`curl -fsSL https://www.postgresql.org/media/keys/ACCC4CF8.asc | sudo gpg --dearmor -o /etc/apt/trusted.gpg.d/postgresql.gpg`)
	}
	keyCmd.CombinedOutput() //nolint:errcheck

	// Add repo list entry
	repoLine := fmt.Sprintf("deb https://apt.postgresql.org/pub/repos/apt %s-pgdg main", codename)
	writeFileWithSudo("/etc/apt/sources.list.d/pgdg.list", []byte(repoLine+"\n"), 0644) //nolint:errcheck

	// Update package index
	sudoRun("apt-get", "update", "-y", "-q")
}

func installPostgresDnf() error {
	emit("info", "Installing PostgreSQL 15 via dnf...")
	pgdgRpm := pgdgRPMURL()
	sudoRun("dnf", "install", "-y", pgdgRpm)
	sudoRun("dnf", "-qy", "module", "disable", "postgresql")
	if err := sudoRunErr("dnf", "install", "-y", "postgresql15-server", "postgresql15-contrib"); err != nil {
		return fmt.Errorf("dnf install postgresql15-server: %w", err)
	}
	sudoRun("/usr/pgsql-15/bin/postgresql-15-setup", "initdb")
	sudoRun("systemctl", "enable", "postgresql-15")
	return nil
}

func installPostgresYum() error {
	emit("info", "Installing PostgreSQL 15 via yum...")
	pgdgRpm := pgdgRPMURL()
	sudoRun("yum", "install", "-y", pgdgRpm)
	if err := sudoRunErr("yum", "install", "-y", "postgresql15-server", "postgresql15-contrib"); err != nil {
		return fmt.Errorf("yum install postgresql15-server: %w", err)
	}
	sudoRun("/usr/pgsql-15/bin/postgresql-15-setup", "initdb")
	sudoRun("systemctl", "enable", "postgresql-15")
	return nil
}

// pgdgRPMURL builds the correct PGDG RPM repo URL for the current distro and arch.
func pgdgRPMURL() string {
	// Detect EL major version from /etc/os-release
	elVer := "9"
	data, err := os.ReadFile("/etc/os-release")
	if err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			if after, ok := strings.CutPrefix(line, "VERSION_ID="); ok {
				v := strings.Trim(after, `"'`)
				if idx := strings.Index(v, "."); idx >= 0 {
					v = v[:idx] // "8.6" → "8"
				}
				if v == "7" || v == "8" || v == "9" {
					elVer = v
				}
				break
			}
		}
	}

	arch := "x86_64"
	if runtime.GOARCH == "arm64" {
		arch = "aarch64"
	}

	return fmt.Sprintf(
		"https://download.postgresql.org/pub/repos/yum/reporpms/EL-%s-%s/pgdg-redhat-repo-latest.noarch.rpm",
		elVer, arch)
}

func findPsqlLinux() string {
	if p, err := exec.LookPath("psql"); err == nil {
		return p
	}
	candidates := []string{
		"/usr/bin/psql",
		"/usr/lib/postgresql/15/bin/psql",
		"/usr/lib/postgresql/16/bin/psql",
		"/usr/lib/postgresql/14/bin/psql",
		"/usr/pgsql-15/bin/psql",
		"/usr/pgsql-16/bin/psql",
		"/usr/local/bin/psql",
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// setupDatabaseLinux creates the app database and user.
// Uses peer authentication (runs psql as the postgres OS user).
func setupDatabaseLinux(psqlPath string, cfg *Config, isLocal bool) error {
	if !isLocal {
		// External DB: just verify connectivity
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

	// Local DB: use peer auth as the postgres OS user
	runAdmin := func(sql string) {
		runPsqlAdmin(psqlPath, "postgres", sql)
	}
	runAdmin(fmt.Sprintf("CREATE USER %s WITH PASSWORD '%s';", cfg.DBUser, cfg.DBPassword))
	runAdmin(fmt.Sprintf("CREATE DATABASE %s OWNER %s;", cfg.DBName, cfg.DBUser))
	runAdmin(fmt.Sprintf("GRANT ALL PRIVILEGES ON DATABASE %s TO %s;", cfg.DBName, cfg.DBUser))
	return nil
}

// runPsqlAdmin runs a SQL command as the postgres OS user using peer authentication.
func runPsqlAdmin(psqlPath, db, sql string) {
	var cmd *exec.Cmd
	if isRoot() {
		// su switches to postgres OS user (peer auth via unix socket)
		inner := fmt.Sprintf(`%s -d %s -c %s -q 2>&1`, psqlPath, db, shellQuote(sql))
		cmd = exec.Command("su", "-", "postgres", "-c", inner)
	} else {
		cmd = exec.Command("sudo", "-u", "postgres", psqlPath,
			"-d", db, "-c", sql, "-q")
	}
	out, err := cmd.CombinedOutput()
	if err != nil && !strings.Contains(string(out), "already exists") {
		emit("warn", fmt.Sprintf("  psql admin: %s", strings.TrimSpace(string(out))))
	}
}

func dropDatabaseLinux(psqlPath, host, port, dbName, dbUser string) {
	runAdmin := func(sql string) {
		runPsqlAdmin(psqlPath, "postgres", sql)
	}
	runAdmin(fmt.Sprintf(
		`SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = '%s' AND pid <> pg_backend_pid();`,
		dbName))
	runAdmin("DROP DATABASE IF EXISTS " + dbName + ";")
	runAdmin("DROP USER IF EXISTS " + dbUser + ";")
	emit("ok", fmt.Sprintf("Database '%s' and user '%s' dropped", dbName, dbUser))
}

func isRoot() bool { return os.Getuid() == 0 }

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// ── Node.js ────────────────────────────────────────────────────────────────────

func installNodeLinux() (string, error) {
	if p := findNodeLinux(); p != "" && nodeVersionOK(p) {
		emit("info", "Node.js already installed: "+p)
		return p, nil
	}

	pm := detectPackageManager()
	switch pm {
	case "apt-get":
		if err := installNodeApt(); err != nil {
			emit("warn", "apt install failed: "+err.Error()+" — falling back to tarball")
		} else {
			if p := findNodeLinux(); p != "" {
				return p, nil
			}
		}
	case "dnf", "yum":
		if err := installNodeDnfYum(pm); err != nil {
			emit("warn", pm+" install failed: "+err.Error()+" — falling back to tarball")
		} else {
			if p := findNodeLinux(); p != "" {
				return p, nil
			}
		}
	}

	// Tarball fallback: works on any Linux distro
	return installNodeTarball()
}

func installNodeApt() error {
	emit("info", "Adding NodeSource repository for Node.js 20 LTS...")
	setupTmp := filepath.Join(os.TempDir(), "nodesource-setup.sh")
	if err := downloadFileProgress("https://deb.nodesource.com/setup_20.x", setupTmp); err != nil {
		// Try plain package as a simpler fallback
		emit("warn", "NodeSource download failed — trying apt-get install nodejs directly")
		return aptGetInstall("nodejs", "npm")
	}
	defer os.Remove(setupTmp)
	os.Chmod(setupTmp, 0755) //nolint:errcheck
	if isRoot() {
		sudoRun("bash", setupTmp)
	} else {
		sudoRun("sudo", "bash", setupTmp)
	}
	return aptGetInstall("nodejs")
}

func installNodeDnfYum(pm string) error {
	emit("info", fmt.Sprintf("Installing Node.js 20 LTS via %s...", pm))
	// Try NodeSource RPM setup
	setupTmp := filepath.Join(os.TempDir(), "nodesource-setup.sh")
	if err := downloadFileProgress("https://rpm.nodesource.com/setup_20.x", setupTmp); err == nil {
		defer os.Remove(setupTmp)
		os.Chmod(setupTmp, 0755) //nolint:errcheck
		sudoRun("bash", setupTmp)
		return sudoRunErr(pm, "install", "-y", "nodejs")
	}
	// Fallback: distro packages
	return sudoRunErr(pm, "install", "-y", "nodejs", "npm")
}

func installNodeTarball() (string, error) {
	arch := "x64"
	if runtime.GOARCH == "arm64" {
		arch = "arm64"
	}
	url := fmt.Sprintf("https://nodejs.org/dist/v20.19.0/node-v20.19.0-linux-%s.tar.xz", arch)
	tmp := filepath.Join(os.TempDir(), "node-linux.tar.xz")
	emit("info", fmt.Sprintf("Downloading Node.js 20 LTS tarball (%s)...", arch))
	if err := downloadFileProgress(url, tmp); err != nil {
		return "", fmt.Errorf("tarball download: %w", err)
	}
	defer os.Remove(tmp)
	emit("info", "Extracting Node.js to /usr/local...")
	var cmd *exec.Cmd
	if isRoot() {
		cmd = exec.Command("tar", "-xJf", tmp, "--strip-components=1", "-C", "/usr/local")
	} else {
		cmd = exec.Command("sudo", "tar", "-xJf", tmp, "--strip-components=1", "-C", "/usr/local")
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("tar extract: %s: %w", strings.TrimSpace(string(out)), err)
	}
	p := "/usr/local/bin/node"
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("node not found at %s after tarball install", p)
	}
	return p, nil
}

func findNodeLinux() string {
	if p, err := exec.LookPath("node"); err == nil {
		return p
	}
	for _, p := range []string{"/usr/local/bin/node", "/usr/bin/node", "/opt/homebrew/bin/node"} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

func nodeVersionOK(nodePath string) bool {
	out, err := exec.Command(nodePath, "--version").Output()
	if err != nil {
		return false
	}
	// v20.x.x → major = 20, we accept >= 18
	v := strings.TrimPrefix(strings.TrimSpace(string(out)), "v")
	major := 0
	fmt.Sscanf(v, "%d", &major)
	return major >= 18
}

// ── App bundle ─────────────────────────────────────────────────────────────────

func downloadAppBundleLinux(installDir, cfgVersion string) error {
	// Embedded bundle (built with -tags embedded)
	if len(embeddedLinuxBundle) > 0 {
		emit("info", "Extracting embedded bundle...")
		tmp := filepath.Join(os.TempDir(), "ezhealthkonnect-bundle.zip")
		if err := os.WriteFile(tmp, embeddedLinuxBundle, 0644); err != nil {
			return fmt.Errorf("failed to write embedded bundle: %w", err)
		}
		defer os.Remove(tmp)
		return extractZip(tmp, installDir, true)
	}

	// Side-car bundle next to the installer binary
	if exePath, err := os.Executable(); err == nil {
		for _, name := range []string{"ezhealthkonnect-linux-amd64.zip", "ezhealthkonnect-linux-arm64.zip"} {
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

	arch := "amd64"
	if runtime.GOARCH == "arm64" {
		arch = "arm64"
	}
	url := fmt.Sprintf("%s/download/%s/ezhealthkonnect-linux-%s.zip", releasesBase, tag, arch)
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

func writeLinuxEnvFile(path string, cfg *Config) error {
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

// ── systemd services ───────────────────────────────────────────────────────────

func registerLinuxServices(nodePath, goAPIBin, envPath string, cfg *Config) error {
	logsDir := filepath.Join(cfg.InstallDir, "logs")
	os.MkdirAll(logsDir, 0755) //nolint:errcheck

	// Make the go-api binary executable
	os.Chmod(goAPIBin, 0755) //nolint:errcheck

	units := []struct {
		name    string
		content string
	}{
		{
			"ezhealthkonnect-api",
			fmt.Sprintf(`[Unit]
Description=ezHealthKonnect Go API
After=network.target postgresql.service postgresql-15.service
Wants=network.target

[Service]
Type=simple
WorkingDirectory=%s
ExecStart=%s
Restart=on-failure
RestartSec=10
StandardOutput=append:%s/api.log
StandardError=append:%s/api.err.log
EnvironmentFile=%s

[Install]
WantedBy=multi-user.target
`, cfg.InstallDir, goAPIBin, logsDir, logsDir, envPath),
		},
		{
			"ezhealthkonnect",
			fmt.Sprintf(`[Unit]
Description=ezHealthKonnect Web Frontend
After=ezhealthkonnect-api.service
Requires=ezhealthkonnect-api.service

[Service]
Type=simple
WorkingDirectory=%s
ExecStart=%s %s/server.js
Restart=on-failure
RestartSec=10
StandardOutput=append:%s/node.log
StandardError=append:%s/node.err.log
EnvironmentFile=%s

[Install]
WantedBy=multi-user.target
`, cfg.InstallDir, nodePath, cfg.InstallDir, logsDir, logsDir, envPath),
		},
	}

	for _, u := range units {
		unitPath := "/etc/systemd/system/" + u.name + ".service"
		if err := writeFileWithSudo(unitPath, []byte(u.content), 0644); err != nil {
			return fmt.Errorf("write unit %s: %w", u.name, err)
		}
	}

	sudoRun("systemctl", "daemon-reload")

	// Enable and start in dependency order: API first, then web
	for _, svc := range []string{"ezhealthkonnect-api", "ezhealthkonnect"} {
		sudoRun("systemctl", "enable", svc)
		if out, err := sudoRunOutput("systemctl", "start", svc); err != nil {
			emit("warn", fmt.Sprintf("Could not start %s: %s", svc, strings.TrimSpace(string(out))))
		} else {
			emit("ok", "Started: "+svc)
		}
		time.Sleep(2 * time.Second)
	}
	return nil
}

// ── Package manager helpers ────────────────────────────────────────────────────

func detectPackageManager() string {
	for _, pm := range []string{"apt-get", "dnf", "yum", "zypper"} {
		if _, err := exec.LookPath(pm); err == nil {
			return pm
		}
	}
	return ""
}

func aptGetInstall(pkgs ...string) error {
	args := append([]string{"install", "-y"}, pkgs...)
	return sudoRunErr("apt-get", args...)
}

// ── sudo/root helpers ──────────────────────────────────────────────────────────

// sudoRun runs a command, prepending sudo if not already root. Ignores errors.
func sudoRun(name string, args ...string) {
	var cmd *exec.Cmd
	if isRoot() {
		cmd = exec.Command(name, args...)
	} else {
		cmd = exec.Command("sudo", append([]string{name}, args...)...)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		emit("warn", fmt.Sprintf("  %s %s: %s", name, strings.Join(args, " "), strings.TrimSpace(string(out))))
	}
}

// sudoRunErr runs a command with sudo and returns any error.
func sudoRunErr(name string, args ...string) error {
	var cmd *exec.Cmd
	if isRoot() {
		cmd = exec.Command(name, args...)
	} else {
		cmd = exec.Command("sudo", append([]string{name}, args...)...)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", name, strings.TrimSpace(string(out)))
	}
	return nil
}

// sudoRunOutput runs a command with sudo and returns combined output.
func sudoRunOutput(name string, args ...string) ([]byte, error) {
	var cmd *exec.Cmd
	if isRoot() {
		cmd = exec.Command(name, args...)
	} else {
		cmd = exec.Command("sudo", append([]string{name}, args...)...)
	}
	return cmd.CombinedOutput()
}

// writeFileWithSudo writes content to path, using sudo if not root.
func writeFileWithSudo(path string, content []byte, perm os.FileMode) error {
	if isRoot() {
		return os.WriteFile(path, content, perm)
	}
	// Write to a temp file then sudo cp
	tmp, err := os.CreateTemp("", "ezhk-*.unit")
	if err != nil {
		return err
	}
	tmp.Write(content) //nolint:errcheck
	tmp.Close()
	defer os.Remove(tmp.Name())
	cmd := exec.Command("sudo", "cp", tmp.Name(), path)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("sudo cp to %s: %s: %w", path, strings.TrimSpace(string(out)), err)
	}
	exec.Command("sudo", "chmod", fmt.Sprintf("%o", perm), path).Run() //nolint:errcheck
	return nil
}

// ── Utilities ──────────────────────────────────────────────────────────────────

func orDefault(val, def string) string {
	if val == "" {
		return def
	}
	return val
}
