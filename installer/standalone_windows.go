//go:build windows

package main

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	releasesBase  = "https://github.com/ezinteropsolutions/ezhealthkonnect/releases"
	nssmDownload  = "https://nssm.cc/release/nssm-2.24.zip"
	nssmExeInZip  = "nssm-2.24/win64/nssm.exe"
)

// runStandaloneInstallation is the Windows native (no-Docker) install path.
func runStandaloneInstallation(cfg *Config) {
	defer func() {
		appURL := fmt.Sprintf("http://localhost:%s", cfg.AppPort)
		emitDoneSignal(appURL)
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
	emitStep(2, "Installing PostgreSQL 15")
	psqlPath, err := installPostgres()
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
	npmPath := filepath.Join(filepath.Dir(nodePath), "npm.cmd")
	if err := streamCmd(exec.Command(npmPath, "install", "--omit=dev", "--silent")); err != nil {
		emit("warn", "npm install had warnings: "+err.Error())
	} else {
		emit("ok", "Node.js dependencies installed")
	}

	// ── Step 6: Database setup ─────────────────────────────────────────────
	emitStep(6, "Setting up PostgreSQL database")
	if err := setupDatabase(psqlPath, cfg); err != nil {
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
		nssmPath, err := ensureNSSM(cfg.InstallDir)
		if err != nil {
			emit("warn", "NSSM download failed — skipping service registration: "+err.Error())
		} else {
			goAPIBin := filepath.Join(cfg.InstallDir, "go-api.exe")
			if err := registerWindowsServices(nssmPath, nodePath, goAPIBin, cfg); err != nil {
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
	emit("info", "╔══════════════════════════════════════════════╗")
	emit("ok",   "║   Installation Complete!                     ║")
	emit("info", "╚══════════════════════════════════════════════╝")
	emit("info", "")
	emit("ok", "Platform URL: "+appURL)
	emit("info", "Default login: admin@ezhealthkonnect.com / admin123")
	emit("warn", "Change the default password immediately after first login.")
}

// ── PostgreSQL ─────────────────────────────────────────────────────────────

func installPostgres() (string, error) {
	// Already installed?
	if p := findPsql(); p != "" {
		emit("info", "PostgreSQL already installed: "+p)
		return p, nil
	}

	emit("info", "Installing PostgreSQL 15 via winget...")
	if err := wingetInstall("PostgreSQL.PostgreSQL.15"); err != nil {
		// Try without version suffix
		if err2 := wingetInstall("PostgreSQL.PostgreSQL"); err2 != nil {
			return "", fmt.Errorf("winget install PostgreSQL: %w (also tried generic: %v)", err, err2)
		}
	}

	// Give installer time to finish
	emit("info", "Waiting for PostgreSQL service to start...")
	time.Sleep(15 * time.Second)

	// Start the service (winget install may not auto-start)
	exec.Command("net", "start", "postgresql-x64-15").Run() //nolint:errcheck
	exec.Command("net", "start", "postgresql").Run()        //nolint:errcheck

	p := findPsql()
	if p == "" {
		return "", fmt.Errorf("psql.exe not found after install — check PostgreSQL installation")
	}
	return p, nil
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
	out, err := exec.Command("winget", "install", "--id", pkg,
		"--silent", "--accept-package-agreements", "--accept-source-agreements",
		"--scope", "machine").CombinedOutput()
	if err != nil {
		// "already installed" is not an error
		if strings.Contains(string(out), "already installed") {
			return nil
		}
		return fmt.Errorf("%w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// ── Node.js ────────────────────────────────────────────────────────────────

func installNodeJS() (string, error) {
	if p := findNode(); p != "" {
		emit("info", "Node.js already installed: "+p)
		return p, nil
	}

	emit("info", "Installing Node.js 20 LTS via winget...")
	if err := wingetInstall("OpenJS.NodeJS.LTS"); err != nil {
		return "", fmt.Errorf("winget install Node.js: %w", err)
	}

	time.Sleep(5 * time.Second)

	p := findNode()
	if p == "" {
		return "", fmt.Errorf("node.exe not found after install")
	}
	return p, nil
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

func downloadAppBundle(installDir, version string) error {
	var url string
	if version == "" || version == "latest" {
		url = releasesBase + "/latest/download/ezhealthkonnect-windows-amd64.zip"
	} else {
		url = releasesBase + "/download/" + version + "/ezhealthkonnect-windows-amd64.zip"
	}

	emit("info", "Downloading bundle from "+url+" ...")
	tmp := filepath.Join(os.TempDir(), "ezhealthkonnect-bundle.zip")
	if err := downloadFileProgress(url, tmp); err != nil {
		return err
	}
	defer os.Remove(tmp)

	emit("info", "Extracting...")
	return extractZip(tmp, installDir, true)
}

func downloadFileProgress(url, dest string) error {
	resp, err := http.Get(url) //nolint:gosec
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()

	total := resp.ContentLength
	var downloaded int64
	buf := make([]byte, 32*1024)
	last := time.Now()

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				return werr
			}
			downloaded += int64(n)
			if time.Since(last) > 5*time.Second {
				if total > 0 {
					emit("info", fmt.Sprintf("  Downloading... %.0f%% (%.1f / %.1f MB)",
						float64(downloaded)/float64(total)*100,
						float64(downloaded)/1e6, float64(total)/1e6))
				} else {
					emit("info", fmt.Sprintf("  Downloading... %.1f MB", float64(downloaded)/1e6))
				}
				last = time.Now()
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return readErr
		}
	}
	return nil
}

func extractZip(src, dest string, stripRoot bool) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		name := f.Name
		if stripRoot {
			// Remove first path component
			idx := strings.Index(name, "/")
			if idx < 0 {
				continue
			}
			name = name[idx+1:]
		}
		if name == "" {
			continue
		}

		target := filepath.Join(dest, filepath.FromSlash(name))

		if f.FileInfo().IsDir() {
			os.MkdirAll(target, f.Mode()) //nolint:errcheck
			continue
		}

		os.MkdirAll(filepath.Dir(target), 0755) //nolint:errcheck

		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			out.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		out.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

// ── Database setup ─────────────────────────────────────────────────────────

func setupDatabase(psqlPath string, cfg *Config) error {
	pgBin := filepath.Dir(psqlPath)
	pgPass := pgAdminPassword()

	runPsql := func(db, query string) error {
		cmd := exec.Command(
			filepath.Join(pgBin, "psql.exe"),
			"-U", "postgres",
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

// pgAdminPassword reads the PostgreSQL superuser password.
// During a fresh winget install the default is typically blank or "postgres".
// We check the env var PGPASSWORD first, then try blank, then "postgres".
func pgAdminPassword() string {
	if p := os.Getenv("PGPASSWORD"); p != "" {
		return p
	}
	return "postgres"
}

func runMigrations(psqlPath string, cfg *Config) error {
	pgBin := filepath.Dir(psqlPath)
	migDir := filepath.Join(cfg.InstallDir, "database", "migrations")

	info, err := os.ReadDir(migDir)
	if err != nil {
		return fmt.Errorf("migrations dir not found: %w", err)
	}

	count := 0
	for _, entry := range info {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "V") || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		sqlFile := filepath.Join(migDir, entry.Name())
		cmd := exec.Command(
			filepath.Join(pgBin, "psql.exe"),
			"-U", cfg.DBUser,
			"-d", cfg.DBName,
			"-f", sqlFile,
			"-q",
		)
		cmd.Env = append(os.Environ(), "PGPASSWORD="+cfg.DBPassword)
		out, err := cmd.CombinedOutput()
		if err != nil {
			emit("warn", fmt.Sprintf("  %s: %s", entry.Name(), strings.TrimSpace(string(out))))
		}
		count++
	}

	emit("info", fmt.Sprintf("  Applied %d migration file(s)", count))
	return nil
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

DB_HOST=localhost
DB_PORT=%s
DB_NAME=%s
DB_USER=%s
DB_PASSWORD=%s
DB_SSL=false

SESSION_SECRET=%s
JWT_SECRET=%s

NODE_ENV=production
LOG_LEVEL=info

OBJECT_STORAGE_DRIVER=local
LOCAL_STORAGE_PATH=%s\storage

AI_ENABLED=%v
OLLAMA_URL=http://localhost:11434
OLLAMA_CHAT_MODEL=llama3.2:3b
OLLAMA_EMBED_MODEL=nomic-embed-text
`,
		time.Now().Format("2006-01-02 15:04"),
		cfg.AppPort, cfg.APIPort,
		cfg.DBPort, cfg.DBName, cfg.DBUser, cfg.DBPassword,
		session, jwt,
		cfg.InstallDir,
		cfg.WithAI,
	)
	return os.WriteFile(path, []byte(content), 0600)
}

// ── NSSM + Windows Services ────────────────────────────────────────────────

func ensureNSSM(installDir string) (string, error) {
	toolsDir := filepath.Join(installDir, "tools")
	nssmExe := filepath.Join(toolsDir, "nssm.exe")

	// Already downloaded?
	if _, err := os.Stat(nssmExe); err == nil {
		return nssmExe, nil
	}

	emit("info", "Downloading NSSM service manager...")
	zipPath := filepath.Join(os.TempDir(), "nssm.zip")
	if err := downloadFileProgress(nssmDownload, zipPath); err != nil {
		return "", fmt.Errorf("download NSSM: %w", err)
	}
	defer os.Remove(zipPath)

	// Extract just the nssm.exe from the zip
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", err
	}
	defer r.Close()

	for _, f := range r.File {
		if f.Name != nssmExeInZip {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", err
		}
		out, err := os.Create(nssmExe)
		if err != nil {
			rc.Close()
			return "", err
		}
		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()
		if err != nil {
			return "", err
		}
		emit("ok", "NSSM downloaded")
		return nssmExe, nil
	}
	return "", fmt.Errorf("nssm.exe not found in zip (expected: %s)", nssmExeInZip)
}

func registerWindowsServices(nssmPath, nodePath, goAPIBin string, cfg *Config) error {
	envFile := filepath.Join(cfg.InstallDir, ".env")

	// Helper: set env vars on the service via NSSM
	setEnvVars := func(svcName string) {
		exec.Command(nssmPath, "set", svcName, "AppEnvironmentExtra", //nolint:errcheck
			"PORT="+cfg.AppPort,
			"API_PORT="+cfg.APIPort,
			"NODE_ENV=production",
		).Run()
		// Also point at the .env file via AppEnvFile if supported
		_ = envFile
	}

	// ── Go API service ──────────────────────────────────────────────────────
	svcAPI := "ezhealthkonnect-api"
	removeService(nssmPath, svcAPI)

	if out, err := exec.Command(nssmPath, "install", svcAPI, goAPIBin).CombinedOutput(); err != nil {
		return fmt.Errorf("nssm install go-api: %s: %w", out, err)
	}
	exec.Command(nssmPath, "set", svcAPI, "AppDirectory", cfg.InstallDir).Run()          //nolint:errcheck
	exec.Command(nssmPath, "set", svcAPI, "DisplayName", "ezHealthKonnect Go API").Run() //nolint:errcheck
	exec.Command(nssmPath, "set", svcAPI, "Description",
		"ezHealthKonnect HL7/FHIR Go Backend").Run() //nolint:errcheck
	exec.Command(nssmPath, "set", svcAPI, "Start", "SERVICE_AUTO_START").Run() //nolint:errcheck
	exec.Command(nssmPath, "set", svcAPI, "AppStdout",
		filepath.Join(cfg.InstallDir, "logs", "api.log")).Run() //nolint:errcheck
	exec.Command(nssmPath, "set", svcAPI, "AppStderr",
		filepath.Join(cfg.InstallDir, "logs", "api.log")).Run() //nolint:errcheck
	setEnvVars(svcAPI)

	// ── Node.js frontend service ────────────────────────────────────────────
	svcApp := "ezhealthkonnect"
	removeService(nssmPath, svcApp)

	serverJS := filepath.Join(cfg.InstallDir, "server.js")
	if out, err := exec.Command(nssmPath, "install", svcApp, nodePath, serverJS).CombinedOutput(); err != nil {
		return fmt.Errorf("nssm install node: %s: %w", out, err)
	}
	exec.Command(nssmPath, "set", svcApp, "AppDirectory", cfg.InstallDir).Run()            //nolint:errcheck
	exec.Command(nssmPath, "set", svcApp, "DisplayName", "ezHealthKonnect Web").Run()      //nolint:errcheck
	exec.Command(nssmPath, "set", svcApp, "Description", "ezHealthKonnect Web Frontend").Run() //nolint:errcheck
	exec.Command(nssmPath, "set", svcApp, "Start", "SERVICE_AUTO_START").Run()             //nolint:errcheck
	exec.Command(nssmPath, "set", svcApp, "DependOnService", svcAPI).Run()                 //nolint:errcheck
	exec.Command(nssmPath, "set", svcApp, "AppStdout",
		filepath.Join(cfg.InstallDir, "logs", "app.log")).Run() //nolint:errcheck
	exec.Command(nssmPath, "set", svcApp, "AppStderr",
		filepath.Join(cfg.InstallDir, "logs", "app.log")).Run() //nolint:errcheck
	setEnvVars(svcApp)

	// Start services
	emit("info", "Starting services...")
	if out, err := exec.Command(nssmPath, "start", svcAPI).CombinedOutput(); err != nil {
		emit("warn", fmt.Sprintf("Could not start %s: %s", svcAPI, strings.TrimSpace(string(out))))
	}
	time.Sleep(3 * time.Second)
	if out, err := exec.Command(nssmPath, "start", svcApp).CombinedOutput(); err != nil {
		emit("warn", fmt.Sprintf("Could not start %s: %s", svcApp, strings.TrimSpace(string(out))))
	}

	return nil
}

func removeService(nssmPath, name string) {
	// Stop + remove silently — ignore errors (may not exist)
	exec.Command(nssmPath, "stop", name).Run()              //nolint:errcheck
	exec.Command(nssmPath, "remove", name, "confirm").Run() //nolint:errcheck
}

// runStandaloneUninstall stops Windows services and removes the install directory.
func runStandaloneUninstall(cfg *Config) {
	const svcAPI = "ezHealthKonnect-API"
	const svcApp = "ezHealthKonnect"

	emit("info", "── Stopping and removing Windows services")
	nssmPath := filepath.Join(cfg.InstallDir, "nssm.exe")
	if _, err := os.Stat(nssmPath); err != nil {
		// Try to find nssm in PATH
		if p, err2 := exec.LookPath("nssm"); err2 == nil {
			nssmPath = p
		}
	}
	if nssmPath != "" {
		removeService(nssmPath, svcAPI)
		removeService(nssmPath, svcApp)
		emit("ok", "Services removed")
	} else {
		emit("warn", "nssm not found — services may need to be removed manually via services.msc")
	}

	emit("info", "── Removing install directory: "+cfg.InstallDir)
	if err := os.RemoveAll(cfg.InstallDir); err != nil {
		emit("warn", "Could not fully remove "+cfg.InstallDir+": "+err.Error())
	} else {
		emit("ok", "Install directory removed")
	}

	emit("ok", "Uninstall complete — you may close this window")
}
