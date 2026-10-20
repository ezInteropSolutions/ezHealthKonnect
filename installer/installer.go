package main

// installer.go — install, upgrade (with backup + automatic rollback),
// rollback and uninstall, written once against the Platform interface.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// MigrationRunner applies database migrations with the installed go-api.
type MigrationRunner interface {
	Migrate(installDir, goAPI string, rep Reporter) error
}

// GoAPIMigrator runs `go-api migrate` in the installation directory (go-api
// reads .env from its working directory).
type GoAPIMigrator struct{}

// Migrate implements MigrationRunner.
func (GoAPIMigrator) Migrate(installDir, goAPI string, rep Reporter) error {
	cmd := exec.Command(goAPI, "migrate")
	cmd.Dir = installDir
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	for _, line := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if line = strings.TrimSpace(line); line != "" && !strings.Contains(line, "No .env file found") {
			rep.Info("  %s", line)
		}
	}
	if err != nil {
		return fmt.Errorf("database migration failed: %w", err)
	}
	return nil
}

// HealthChecker waits until the application reports healthy.
type HealthChecker interface {
	WaitHealthy(appPort string, timeout time.Duration) error
}

// HTTPHealthChecker requires both the web server and the backend's database
// connection to be healthy.
type HTTPHealthChecker struct{}

// WaitHealthy implements HealthChecker.
func (HTTPHealthChecker) WaitHealthy(appPort string, timeout time.Duration) error {
	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		last = checkHealthOnce(client, appPort)
		if last == nil {
			return nil
		}
		time.Sleep(3 * time.Second)
	}
	return fmt.Errorf("the application did not become healthy within %s: %v", timeout, last)
}

func checkHealthOnce(client *http.Client, appPort string) error {
	resp, err := client.Get("http://127.0.0.1:" + appPort + "/api/system/health")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned HTTP %d", resp.StatusCode)
	}
	var body struct {
		Database struct {
			Status string `json:"status"`
		} `json:"database"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return fmt.Errorf("health check: %w", err)
	}
	if body.Database.Status != "connected" {
		return fmt.Errorf("database status is %q", body.Database.Status)
	}
	return nil
}

// Installer orchestrates installs and upgrades.
type Installer struct {
	Platform Platform
	Bundle   BundleSource
	Migrator MigrationRunner
	Health   HealthChecker
	Rep      Reporter
	Now      func() time.Time
	// Pg returns client tools for a bin dir (injectable for tests).
	Pg func(binDir string) PgOps
	// HealthTimeout bounds the post-start health wait.
	HealthTimeout time.Duration
	// KeepBackups is how many upgrade backups to keep.
	KeepBackups int
}

// PgOps is the subset of PgTools the orchestrator needs.
type PgOps interface {
	Ping(c PgConn) error
	ServerMajor(c PgConn) (int, error)
	ClientMajor() (int, error)
	HasTables(c PgConn) (bool, error)
	Dump(c PgConn, file string) error
	VerifyDump(file string) error
	Restore(c PgConn, file string) error
}

// NewInstaller wires the production implementations.
func NewInstaller(p Platform, b BundleSource, rep Reporter) *Installer {
	return &Installer{
		Platform: p, Bundle: b, Migrator: GoAPIMigrator{}, Health: HTTPHealthChecker{}, Rep: rep,
		Now: time.Now, Pg: func(bin string) PgOps { return PgTools{BinDir: bin} },
		HealthTimeout: 3 * time.Minute, KeepBackups: 3,
	}
}

// InstallResult tells the person installing where to go next.
type InstallResult struct {
	AppURL   string `json:"appUrl"`
	SetupURL string `json:"setupUrl,omitempty"`
	Version  string `json:"version"`
}

// ── Install ───────────────────────────────────────────────────────────────────

// Install performs a fresh installation.
func (in *Installer) Install(cfg Config) (*InstallResult, error) {
	cfg.Normalize()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	rep := in.Rep
	dir := cfg.InstallDir

	rep.Step(1, "Checking this machine")
	if _, found, err := LoadState(dir); err != nil {
		return nil, err
	} else if found {
		return nil, errors.New("ezHealthKonnect is already installed here — use Upgrade instead")
	}
	if err := requireEmptyInstallDir(dir); err != nil {
		return nil, err
	}
	if !in.Platform.IsElevated() {
		return nil, errors.New("the installer must run as administrator (Windows) or root (Linux)")
	}
	rep.OK("Installing into %s", dir)

	rep.Step(2, "Unpacking ezHealthKonnect")
	staged, err := StageBundle(in.Bundle, dir, goosName())
	if err != nil {
		return nil, err
	}
	defer staged.Remove()
	rep.OK("Version %s ready (%s)", staged.Info.Version, in.Bundle.Describe())

	rep.Step(3, "Preparing PostgreSQL")
	pg, err := in.Platform.EnsurePostgres(&cfg, rep)
	if err != nil {
		return nil, err
	}
	conn := PgConn{Host: cfg.DBHost, Port: cfg.DBPort, DB: cfg.DBName, User: cfg.DBUser, Password: cfg.DBPassword, SSLMode: cfg.DBSSLMode}
	tools := in.Pg(pg.BinDir)
	if cfg.DBMode == "local" {
		conn.Password = randomPassword()
		if err := in.refuseExistingData(pg.Admin, cfg); err != nil {
			return nil, err
		}
		if err := EnsureRoleAndDatabase(pg.Admin, cfg.DBName, cfg.DBUser, conn.Password); err != nil {
			return nil, err
		}
		rep.OK("Database %s and user %s ready", cfg.DBName, cfg.DBUser)
	}
	if err := tools.Ping(conn); err != nil {
		return nil, fmt.Errorf("cannot connect to the database as %s: %w", cfg.DBUser, err)
	}
	if err := in.checkPgVersions(tools, conn); err != nil {
		return nil, err
	}
	if cfg.DBMode == "external" {
		if has, err := tools.HasTables(conn); err != nil {
			return nil, err
		} else if has {
			return nil, fmt.Errorf("database %s on %s already contains tables — use an empty database (or Upgrade an existing installation)", cfg.DBName, cfg.DBHost)
		}
	}

	rep.Step(4, "Installing application files")
	if err := moveEntries(staged.Dir, dir, staged.Info.Entries); err != nil {
		return nil, err
	}
	state := &InstallState{Version: staged.Info.Version, InstalledAt: in.Now(), UpdatedAt: in.Now(),
		AppEntries: sortedCopy(staged.Info.Entries), DBMode: cfg.DBMode, PGBinDir: pg.BinDir, Service: true,
		History: []StateChange{{Action: "install", To: staged.Info.Version, At: in.Now()}}}
	if err := state.Save(dir); err != nil {
		return nil, err
	}
	setupToken := randomSecret(24)
	env := in.newEnv(&cfg, conn, setupToken)
	if err := env.WriteFile(filepath.Join(dir, ".env")); err != nil {
		return nil, err
	}
	if err := in.Platform.PrepareInstall(dir); err != nil {
		return nil, fmt.Errorf("secure installation directory: %w", err)
	}
	rep.OK("Files installed; configuration written to %s", filepath.Join(dir, ".env"))

	rep.Step(5, "Creating the database schema")
	if err := in.Migrator.Migrate(dir, in.Platform.GoAPIPath(dir), rep); err != nil {
		return nil, err
	}
	rep.OK("Database schema up to date")

	appURL := "http://localhost:" + cfg.AppPort
	result := &InstallResult{AppURL: appURL, Version: staged.Info.Version,
		SetupURL: appURL + "/setup.html?token=" + setupToken}
	rep.Step(6, "Starting ezHealthKonnect")
	if err := in.Platform.InstallService(dir, &cfg); err != nil {
		return nil, err
	}
	if err := in.Platform.StartService(); err != nil {
		return nil, err
	}
	if err := in.Health.WaitHealthy(cfg.AppPort, in.HealthTimeout); err != nil {
		return nil, fmt.Errorf("%w (logs: %s)", err, filepath.Join(dir, "logs"))
	}
	rep.OK("ezHealthKonnect is running at %s", appURL)
	if err := in.Platform.RegisterUninstall(dir, staged.Info.Version, cfg.AppPort); err != nil {
		rep.Warn("Could not register in the system's program list: %v", err)
	}
	return result, nil
}

func (in *Installer) refuseExistingData(admin AdminSQL, cfg Config) error {
	if admin == nil {
		return nil
	}
	exists, err := admin.Exec("postgres", fmt.Sprintf("SELECT 1 FROM pg_database WHERE datname = %s", sqlLiteral(cfg.DBName)))
	if err != nil {
		return fmt.Errorf("inspect PostgreSQL: %s", firstLine(exists, err))
	}
	if strings.TrimSpace(exists) != "1" {
		return nil
	}
	tables, err := admin.Exec(cfg.DBName, "SELECT count(*) FROM pg_catalog.pg_tables WHERE schemaname NOT IN ('pg_catalog','information_schema')")
	if err != nil {
		return fmt.Errorf("inspect database %s: %s", cfg.DBName, firstLine(tables, err))
	}
	if strings.TrimSpace(tables) != "0" {
		return fmt.Errorf("database %s already exists and contains data — it may belong to an earlier installation. "+
			"Restore that installation and use Upgrade, or choose a different database name", cfg.DBName)
	}
	return nil
}

func (in *Installer) checkPgVersions(tools PgOps, conn PgConn) error {
	server, err := tools.ServerMajor(conn)
	if err != nil {
		return err
	}
	if server < MinPostgresMajor {
		return fmt.Errorf("PostgreSQL %d is too old — version %d or newer is required", server, MinPostgresMajor)
	}
	client, err := tools.ClientMajor()
	if err != nil {
		return err
	}
	if client < server {
		return fmt.Errorf("pg_dump %d cannot back up a PostgreSQL %d server — install PostgreSQL %d client tools", client, server, server)
	}
	return nil
}

func requireEmptyInstallDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".staging-") {
			os.RemoveAll(filepath.Join(dir, e.Name())) // leftover from an interrupted run
			continue
		}
		return fmt.Errorf("install directory %s is not empty — choose an empty folder", dir)
	}
	return nil
}

// newEnv builds the .env for a fresh installation.
func (in *Installer) newEnv(cfg *Config, conn PgConn, setupToken string) *EnvFile {
	e := &EnvFile{}
	e.Comment("ezHealthKonnect configuration — generated by the installer on " + in.Now().Format("2006-01-02 15:04"))
	e.Comment("Contains secrets: readable only by administrators and the service.")
	e.Comment("Upgrades keep every value here; they only add new settings.")
	e.Comment("")
	e.Set("NODE_ENV", "production")
	e.Set("PORT", cfg.AppPort)
	e.Set("API_PORT", cfg.APIPort)
	e.Set("LOG_LEVEL", "info")
	e.Comment("")
	e.Set("DB_HOST", conn.Host)
	e.Set("DB_PORT", conn.Port)
	e.Set("DB_NAME", conn.DB)
	e.Set("DB_USER", conn.User)
	e.Set("DB_PASSWORD", conn.Password)
	e.Set("DB_SSL", conn.SSLMode)
	e.Comment("")
	e.Set("SESSION_SECRET", randomSecret(48))
	e.Set("JWT_SECRET", randomSecret(48))
	e.Comment("Encrypts stored connector credentials. Never change it: stored credentials would become unreadable.")
	e.Set(credentialKeyEnvVar, newCredentialKey())
	e.Comment("One-time token for the first-run setup page (creating the first administrator).")
	e.Set("SETUP_TOKEN", setupToken)
	e.Comment("Plain HTTP by default. Behind an HTTPS reverse proxy set TRUST_PROXY and SESSION_COOKIE_SECURE=auto.")
	e.Set("SESSION_COOKIE_SECURE", "false")
	e.Comment("")
	in.applyPathDefaults(e, cfg.InstallDir, true)
	e.Set("AI_ENABLED", fmt.Sprint(cfg.WithAI))
	e.Set("OLLAMA_URL", "http://localhost:11434")
	e.Set("OLLAMA_CHAT_MODEL", "llama3.2:3b")
	e.Set("OLLAMA_EMBED_MODEL", "nomic-embed-text")
	return e
}

// applyPathDefaults sets the paths every release needs. With force=false
// (upgrade) only missing keys are added.
func (in *Installer) applyPathDefaults(e *EnvFile, dir string, force bool) []string {
	paths := map[string]string{
		"OBJECT_STORAGE_DRIVER":      "local",
		"LOCAL_STORAGE_PATH":         filepath.Join(dir, "storage"),
		"EZHEALTHKONNECT_SCHEMA_DIR": filepath.Join(dir, "schemas"),
		"FHIR_SCHEMA_DIR":            filepath.Join(dir, "schemas", "fhir"),
		"SCHEMA_DIST_PATH":           filepath.Join(dir, "schema-dist"),
		"GIT_REPOS_BASE_DIR":         filepath.Join(dir, "git-repos"),
	}
	for k, v := range in.Platform.ExtraEnv(dir) {
		paths[k] = v
	}
	keys := make([]string, 0, len(paths))
	for k := range paths {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var added []string
	for _, k := range keys {
		if force {
			e.Set(k, paths[k])
			added = append(added, k)
		} else if e.SetIfMissing(k, paths[k]) {
			added = append(added, k)
		}
	}
	return added
}

// ── Upgrade ───────────────────────────────────────────────────────────────────

// backupManifest describes one upgrade backup folder.
type backupManifest struct {
	FromVersion string    `json:"fromVersion"`
	ToVersion   string    `json:"toVersion"`
	CreatedAt   time.Time `json:"createdAt"`
	AppEntries  []string  `json:"appEntries"`
	DBDump      string    `json:"dbDump"`
}

// Upgrade replaces the application with the bundle's version, keeping all
// data and configuration. Before anything changes it backs up the database,
// .env and the application files; if any later step fails it restores them
// and restarts the previous version.
func (in *Installer) Upgrade(installDir string) (*InstallResult, error) {
	rep := in.Rep
	dir := filepath.Clean(installDir)

	rep.Step(1, "Checking the current installation")
	state, found, err := LoadState(dir)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, fmt.Errorf("no ezHealthKonnect installation found in %s", dir)
	}
	if !in.Platform.IsElevated() {
		return nil, errors.New("the installer must run as administrator (Windows) or root (Linux)")
	}
	env, err := ReadEnvFile(filepath.Join(dir, ".env"))
	if err != nil {
		return nil, err
	}
	conn := connFromEnv(env)
	appPort := env.Get("PORT")
	rep.OK("Installed version: %s", state.Version)

	rep.Step(2, "Unpacking the new version")
	staged, err := StageBundle(in.Bundle, dir, goosName())
	if err != nil {
		return nil, err
	}
	defer staged.Remove()
	cmp := compareVersions(staged.Info.Version, state.Version)
	if cmp < 0 {
		return nil, fmt.Errorf("this installer contains %s, which is older than the installed %s — downgrades are not supported (use Rollback to return to the version before the last upgrade)", staged.Info.Version, state.Version)
	}
	rep.OK("New version: %s", staged.Info.Version)

	rep.Step(3, "Pre-flight checks")
	tools := in.Pg(state.PGBinDir)
	if err := tools.Ping(conn); err != nil {
		return nil, fmt.Errorf("cannot connect to the database: %w", err)
	}
	if err := in.checkPgVersions(tools, conn); err != nil {
		return nil, err
	}
	if free, err := in.Platform.FreeBytes(dir); err == nil && free < 2<<30 {
		return nil, fmt.Errorf("only %d MB free in %s — at least 2 GB is needed for the backup and the new version", free>>20, dir)
	}
	rep.OK("Database reachable, backup tools present, enough disk space")

	// Nothing has changed so far. From here on, every failure restores.
	rep.Step(4, "Stopping ezHealthKonnect")
	if state.Service {
		if err := in.Platform.StopService(); err != nil {
			return nil, fmt.Errorf("could not stop the service (nothing was changed): %w", err)
		}
	}
	rep.OK("Stopped")

	rep.Step(5, "Backing up the database, configuration and application")
	backupDir := filepath.Join(dir, "backups", in.Now().Format("20060102-150405")+"-"+sanitizeVersion(state.Version))
	bm := &backupManifest{FromVersion: state.Version, ToVersion: staged.Info.Version, CreatedAt: in.Now(),
		AppEntries: state.AppEntries, DBDump: "database.dump"}
	if err := in.backup(dir, backupDir, tools, conn, bm); err != nil {
		in.restartAfterAbort(state, appPort)
		return nil, fmt.Errorf("backup failed — nothing was changed: %w", err)
	}
	rep.OK("Backup saved to %s", backupDir)

	if err := in.applyUpgrade(dir, staged, state, env, appPort); err != nil {
		rep.Error("Upgrade failed: %v", err)
		rep.Warn("Restoring the previous version from the backup...")
		if rbErr := in.restore(dir, backupDir, state.AppEntries, staged.Info.Entries, tools, conn, state); rbErr != nil {
			rep.Error("AUTOMATIC ROLLBACK FAILED: %v", rbErr)
			rep.Error("Your data is safe in %s — contact support before changing anything.", backupDir)
			return nil, fmt.Errorf("upgrade failed (%v) and the automatic rollback also failed (%v); backup: %s", err, rbErr, backupDir)
		}
		rep.OK("Rolled back: version %s is running again with its data restored", state.Version)
		// This backup has served its purpose; a later manual rollback must
		// target the last SUCCESSFUL upgrade, not this failed attempt.
		_ = os.Rename(backupDir, backupDir+"-rolledback")
		return nil, fmt.Errorf("upgrade to %s failed and was rolled back: %w", staged.Info.Version, err)
	}

	state.History = append(state.History, StateChange{Action: "upgrade", From: state.Version, To: staged.Info.Version, At: in.Now(), Backup: backupDir})
	state.Version = staged.Info.Version
	state.AppEntries = sortedCopy(staged.Info.Entries)
	state.UpdatedAt = in.Now()
	if err := state.Save(dir); err != nil {
		rep.Warn("Could not record the new version: %v", err)
	}
	if err := in.Platform.RegisterUninstall(dir, state.Version, appPort); err != nil {
		rep.Warn("Could not update the program list entry: %v", err)
	}
	in.pruneBackups(dir)
	rep.OK("Upgraded %s → %s", bm.FromVersion, state.Version)
	return &InstallResult{AppURL: "http://localhost:" + appPort, Version: state.Version}, nil
}

func (in *Installer) backup(dir, backupDir string, tools PgOps, conn PgConn, bm *backupManifest) error {
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return err
	}
	discard := func(err error) error { os.RemoveAll(backupDir); return err }
	dump := filepath.Join(backupDir, bm.DBDump)
	if err := tools.Dump(conn, dump); err != nil {
		return discard(err)
	}
	if err := tools.VerifyDump(dump); err != nil {
		return discard(err)
	}
	for _, f := range []string{".env", stateFileName} {
		if err := copyFileTo(filepath.Join(dir, f), filepath.Join(backupDir, f)); err != nil {
			return discard(fmt.Errorf("back up %s: %w", f, err))
		}
	}
	data, _ := json.MarshalIndent(bm, "", "  ")
	if err := os.WriteFile(filepath.Join(backupDir, "backup.json"), data, 0o600); err != nil {
		return discard(err)
	}
	// Application files last: once moved, the old version is out of place.
	if err := moveEntries(dir, filepath.Join(backupDir, "app"), bm.AppEntries); err != nil {
		// Put back whatever was already moved so "nothing was changed" holds.
		if undo := moveEntries(filepath.Join(backupDir, "app"), dir, bm.AppEntries); undo != nil {
			return fmt.Errorf("%w (and putting the files back failed: %v — they are in %s)", err, undo, backupDir)
		}
		os.RemoveAll(backupDir)
		return err
	}
	return nil
}

// applyUpgrade swaps in the new files, migrates and starts. Any error is
// returned for the caller to roll back.
func (in *Installer) applyUpgrade(dir string, staged *StagedBundle, state *InstallState, env *EnvFile, appPort string) error {
	rep := in.Rep
	rep.Step(6, "Installing the new version")
	if err := moveEntries(staged.Dir, dir, staged.Info.Entries); err != nil {
		return err
	}
	added := in.applyPathDefaults(env, dir, false)
	if err := env.WriteFile(filepath.Join(dir, ".env")); err != nil {
		return err
	}
	if len(added) > 0 {
		rep.Info("Added new settings to .env: %s", strings.Join(added, ", "))
	}
	if err := in.Platform.PrepareInstall(dir); err != nil {
		return err
	}

	rep.Step(7, "Updating the database schema")
	if err := in.Migrator.Migrate(dir, in.Platform.GoAPIPath(dir), rep); err != nil {
		return err
	}

	rep.Step(8, "Starting the new version")
	if state.Service {
		cfg := configFromEnv(dir, env)
		if err := in.Platform.InstallService(dir, cfg); err != nil {
			return err
		}
		if err := in.Platform.StartService(); err != nil {
			return err
		}
		if err := in.Health.WaitHealthy(appPort, in.HealthTimeout); err != nil {
			return err
		}
		rep.OK("Healthy")
	}
	return nil
}

// restore puts the previous version back: files, configuration and database.
func (in *Installer) restore(dir, backupDir string, oldEntries, newEntries []string, tools PgOps, conn PgConn, state *InstallState) error {
	rep := in.Rep
	if state.Service {
		_ = in.Platform.StopService()
	}
	// Move the failed version aside (kept for diagnosis), then the old one back.
	failed := filepath.Join(backupDir, "failed-"+in.Now().Format("150405"))
	if err := moveEntries(dir, failed, newEntries); err != nil {
		return fmt.Errorf("remove the new files: %w", err)
	}
	if err := moveEntries(filepath.Join(backupDir, "app"), dir, oldEntries); err != nil {
		return fmt.Errorf("put back the previous files: %w", err)
	}
	for _, f := range []string{".env", stateFileName} {
		if err := copyFileTo(filepath.Join(backupDir, f), filepath.Join(dir, f)); err != nil {
			return fmt.Errorf("restore %s: %w", f, err)
		}
	}
	if err := tools.Restore(conn, filepath.Join(backupDir, "database.dump")); err != nil {
		return fmt.Errorf("restore the database: %w", err)
	}
	rep.OK("Previous files, configuration and database restored")
	if err := in.Platform.PrepareInstall(dir); err != nil {
		return err
	}
	if state.Service {
		env, err := ReadEnvFile(filepath.Join(dir, ".env"))
		if err != nil {
			return err
		}
		if err := in.Platform.InstallService(dir, configFromEnv(dir, env)); err != nil {
			return err
		}
		if err := in.Platform.StartService(); err != nil {
			return err
		}
		if err := in.Health.WaitHealthy(env.Get("PORT"), in.HealthTimeout); err != nil {
			return fmt.Errorf("the previous version did not come back healthy: %w", err)
		}
	}
	return nil
}

// restartAfterAbort restarts the untouched installation after a failure
// before any change was made.
func (in *Installer) restartAfterAbort(state *InstallState, appPort string) {
	if !state.Service {
		return
	}
	if err := in.Platform.StartService(); err != nil {
		in.Rep.Error("Could not restart the service: %v", err)
		return
	}
	if err := in.Health.WaitHealthy(appPort, in.HealthTimeout); err != nil {
		in.Rep.Error("The service restarted but is not healthy: %v", err)
	}
}

// Rollback returns to the version before the most recent upgrade, restoring
// that upgrade's backup. Data written since that upgrade is lost — callers
// must confirm.
func (in *Installer) Rollback(installDir string) error {
	rep := in.Rep
	dir := filepath.Clean(installDir)
	state, found, err := LoadState(dir)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("no ezHealthKonnect installation found in %s", dir)
	}
	if !in.Platform.IsElevated() {
		return errors.New("the installer must run as administrator (Windows) or root (Linux)")
	}
	backupDir, bm, err := latestBackup(dir)
	if err != nil {
		return err
	}
	if bm.ToVersion != state.Version {
		return fmt.Errorf("the latest backup was taken for the upgrade to %s, but %s is installed — refusing to guess", bm.ToVersion, state.Version)
	}
	env, err := ReadEnvFile(filepath.Join(dir, ".env"))
	if err != nil {
		return err
	}
	tools := in.Pg(state.PGBinDir)
	if err := tools.VerifyDump(filepath.Join(backupDir, bm.DBDump)); err != nil {
		return err
	}
	rep.Step(1, fmt.Sprintf("Rolling back %s → %s", state.Version, bm.FromVersion))
	if err := in.restore(dir, backupDir, bm.AppEntries, state.AppEntries, tools, connFromEnv(env), state); err != nil {
		return err
	}
	restored, _, err := LoadState(dir)
	if err == nil && restored != nil {
		restored.History = append(restored.History, StateChange{Action: "rollback", From: state.Version, To: bm.FromVersion, At: in.Now(), Backup: backupDir})
		_ = restored.Save(dir)
		_ = in.Platform.RegisterUninstall(dir, restored.Version, env.Get("PORT"))
	}
	// The backup has been consumed; keep it for the record but mark it.
	_ = os.Rename(backupDir, backupDir+"-restored")
	rep.OK("Rolled back to %s", bm.FromVersion)
	return nil
}

func latestBackup(dir string) (string, *backupManifest, error) {
	root := filepath.Join(dir, "backups")
	entries, err := os.ReadDir(root)
	if err != nil {
		return "", nil, fmt.Errorf("no upgrade backups found in %s", root)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasSuffix(e.Name(), "-restored") && !strings.HasSuffix(e.Name(), "-rolledback") {
			if _, err := os.Stat(filepath.Join(root, e.Name(), "backup.json")); err == nil {
				names = append(names, e.Name())
			}
		}
	}
	if len(names) == 0 {
		return "", nil, fmt.Errorf("no upgrade backups found in %s", root)
	}
	sort.Strings(names)
	p := filepath.Join(root, names[len(names)-1])
	data, err := os.ReadFile(filepath.Join(p, "backup.json"))
	if err != nil {
		return "", nil, err
	}
	var bm backupManifest
	if err := json.Unmarshal(data, &bm); err != nil {
		return "", nil, err
	}
	return p, &bm, nil
}

func (in *Installer) pruneBackups(dir string) {
	root := filepath.Join(dir, "backups")
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	keep := in.KeepBackups
	if keep < 1 {
		keep = 1
	}
	for len(names) > keep {
		os.RemoveAll(filepath.Join(root, names[0]))
		in.Rep.Info("Removed old backup %s", names[0])
		names = names[1:]
	}
}

// ── Uninstall ─────────────────────────────────────────────────────────────────

// Uninstall stops and removes the service, drops the local database and
// removes the installation directory.
func (in *Installer) Uninstall(installDir string, keepDatabase bool) error {
	rep := in.Rep
	dir := filepath.Clean(installDir)
	if !in.Platform.IsElevated() {
		return errors.New("the installer must run as administrator (Windows) or root (Linux)")
	}
	state, found, _ := LoadState(dir)
	env, _ := ReadEnvFile(filepath.Join(dir, ".env"))

	rep.Step(1, "Stopping and removing the service")
	if err := in.Platform.RemoveService(dir); err != nil {
		rep.Warn("Service removal: %v", err)
	}

	if found && state.DBMode == "local" && !keepDatabase {
		rep.Step(2, "Removing the database")
		cfg := configFromEnv(dir, env)
		if admin := in.Platform.PostgresAdmin(dir, cfg); admin != nil {
			if err := DropRoleAndDatabase(admin, cfg.DBName, cfg.DBUser); err != nil {
				rep.Warn("Database removal: %v", err)
			} else {
				rep.OK("Database %s removed", cfg.DBName)
			}
		} else {
			rep.Warn("No administrator access to PostgreSQL — database %s was left in place", cfg.DBName)
		}
	}

	rep.Step(3, "Removing files")
	in.Platform.RemoveUninstall()
	if err := removeInstallDir(dir); err != nil {
		return fmt.Errorf("remove %s: %w", dir, err)
	}
	rep.OK("ezHealthKonnect has been removed")
	return nil
}

// ── helpers ───────────────────────────────────────────────────────────────────

func connFromEnv(e *EnvFile) PgConn {
	ssl := e.Get("DB_SSL")
	switch strings.ToLower(ssl) {
	case "", "false", "0", "no":
		ssl = "disable"
	case "true", "1", "yes":
		ssl = "require"
	}
	return PgConn{Host: e.Get("DB_HOST"), Port: e.Get("DB_PORT"), DB: e.Get("DB_NAME"), User: e.Get("DB_USER"),
		Password: e.Get("DB_PASSWORD"), SSLMode: ssl}
}

func configFromEnv(dir string, e *EnvFile) *Config {
	return &Config{InstallDir: dir, AppPort: e.Get("PORT"), APIPort: e.Get("API_PORT"), DBHost: e.Get("DB_HOST"),
		DBPort: e.Get("DB_PORT"), DBName: e.Get("DB_NAME"), DBUser: e.Get("DB_USER"),
		WithAI: strings.EqualFold(e.Get("AI_ENABLED"), "true")}
}

func copyFileTo(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o600)
}

func sanitizeVersion(v string) string {
	var b strings.Builder
	for _, r := range v {
		if r == '.' || r == '-' || r == '_' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "unknown"
	}
	return b.String()
}

// compareVersions compares release versions like v26.10, v26.10.1, v26.11-rc1.
func compareVersions(a, b string) int {
	pa, pb := versionParts(a), versionParts(b)
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			if x < y {
				return -1
			}
			return 1
		}
	}
	return 0
}

func versionParts(v string) []int {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	var out []int
	for _, p := range strings.Split(v, ".") {
		n := 0
		for _, c := range p {
			if c < '0' || c > '9' {
				break
			}
			n = n*10 + int(c-'0')
		}
		out = append(out, n)
	}
	return out
}
