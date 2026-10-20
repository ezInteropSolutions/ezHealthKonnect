package main

// CheckResult is one prerequisite check shown before installing.
type CheckResult struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Version string `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
	Warning string `json:"warning,omitempty"`
}

// PgSetup is PostgreSQL as prepared for the application.
type PgSetup struct {
	BinDir string   // client tools (psql, pg_dump, pg_restore)
	Admin  AdminSQL // superuser access to a LOCAL server; nil for an external database
}

// Platform is everything that differs between operating systems. The
// install/upgrade/rollback logic (Installer) is written once against it.
type Platform interface {
	Name() string
	DefaultInstallDir() string
	IsElevated() bool
	Prerequisites(cfg *Config) []CheckResult

	// EnsurePostgres installs PostgreSQL when needed (local mode) and returns
	// its client tools and admin access.
	EnsurePostgres(cfg *Config, rep Reporter) (*PgSetup, error)
	// PostgresAdmin returns admin access to an already-installed local server
	// (uninstall); nil when unavailable.
	PostgresAdmin(installDir string, cfg *Config) AdminSQL

	GoAPIPath(installDir string) string
	// ExtraEnv returns platform-specific .env entries (paths the app needs).
	ExtraEnv(installDir string) map[string]string

	// PrepareInstall creates data directories and applies ownership and
	// access control to the installation (run after files change).
	PrepareInstall(installDir string) error

	InstallService(installDir string, cfg *Config) error
	StartService() error
	StopService() error
	RemoveService(installDir string) error
	ServiceInstalled() bool

	RegisterUninstall(installDir, version, appPort string) error
	RemoveUninstall()

	FreeBytes(path string) (uint64, error)
}

// dataDirs are created in every installation and owned by the service account.
var dataDirs = []string{"logs", "storage", "schemas", "schema-dist", "uploads", "git-repos", "cache", "backups", "tools"}
