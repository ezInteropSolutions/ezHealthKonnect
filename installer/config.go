package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Config is what the person installing chooses (web UI or answer file).
type Config struct {
	InstallDir string `json:"installDir"`
	AppPort    string `json:"appPort"`
	APIPort    string `json:"apiPort"`

	// DBMode "local": the installer installs/uses PostgreSQL on this machine and
	// creates the database and role itself (password generated).
	// DBMode "external": the database and role already exist (cloud/managed
	// PostgreSQL); the installer only connects.
	DBMode     string `json:"dbMode"`
	DBHost     string `json:"dbHost"`
	DBPort     string `json:"dbPort"`
	DBName     string `json:"dbName"`
	DBUser     string `json:"dbUser"`
	DBPassword string `json:"dbPassword"` // external mode only
	DBSSLMode  string `json:"dbSslMode"`  // external mode: disable | require | verify-full

	// External mode, alternative to DBPassword: an administrator account on
	// that server (user defaults to "postgres"). With a password, setup signs
	// in once as the administrator and creates the database and the
	// application's user itself (generated password) — nothing has to exist
	// beforehand. The administrator password is used for this and never stored.
	DBAdminUser     string `json:"dbAdminUser"`
	DBAdminPassword string `json:"dbAdminPassword"`

	// Only needed when PostgreSQL is already installed on this machine and
	// cannot be administered through the OS (Windows): its superuser password.
	PGAdminPassword string `json:"pgAdminPassword"`

	WithAI bool `json:"withAI"`
}

var identRe = regexp.MustCompile(`^[a-z_][a-z0-9_]{0,62}$`)

// Normalize fills defaults.
func (c *Config) Normalize() {
	if c.InstallDir == "" {
		c.InstallDir = defaultInstallDir()
	}
	c.InstallDir = filepath.Clean(c.InstallDir)
	def := func(p *string, v string) {
		if strings.TrimSpace(*p) == "" {
			*p = v
		}
		*p = strings.TrimSpace(*p)
	}
	def(&c.AppPort, "3000")
	def(&c.APIPort, "8080")
	def(&c.DBMode, "local")
	def(&c.DBPort, "5432")
	def(&c.DBName, "ezhealthkonnect")
	def(&c.DBUser, "ezhealth_user")
	if c.DBMode == "external" && c.DBAdminPassword != "" {
		def(&c.DBAdminUser, "postgres")
	}
	if c.DBMode == "local" {
		c.DBHost = "localhost"
		c.DBSSLMode = "disable"
	}
	def(&c.DBSSLMode, "require")
}

// Validate rejects configurations that cannot work or are unsafe.
func (c *Config) Validate() error {
	if !filepath.IsAbs(c.InstallDir) {
		return fmt.Errorf("install directory must be an absolute path")
	}
	for name, p := range map[string]string{"web port": c.AppPort, "API port": c.APIPort, "database port": c.DBPort} {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("%s %q is not a valid port", name, p)
		}
	}
	if c.AppPort == c.APIPort {
		return fmt.Errorf("the web port and the API port must differ")
	}
	if !identRe.MatchString(c.DBName) || !identRe.MatchString(c.DBUser) {
		return fmt.Errorf("database name and user must be lowercase letters, digits and underscores")
	}
	switch c.DBMode {
	case "local":
	case "external":
		if c.DBHost == "" {
			return fmt.Errorf("an existing PostgreSQL server needs a host")
		}
		if c.DBAdminPassword == "" && c.DBPassword == "" {
			return fmt.Errorf("enter an administrator password (setup then creates the database and its user), " +
				"or the password of a database user that already exists")
		}
		for _, p := range []string{c.DBPassword, c.DBAdminPassword, c.DBAdminUser} {
			if strings.ContainsAny(p, "\r\n") {
				return fmt.Errorf("database user names and passwords must be a single line")
			}
		}
		switch c.DBSSLMode {
		case "disable", "require", "verify-ca", "verify-full":
		default:
			return fmt.Errorf("unsupported SSL mode %q", c.DBSSLMode)
		}
	default:
		return fmt.Errorf("database mode must be local or external")
	}
	return nil
}

// ── Install state ─────────────────────────────────────────────────────────────

const stateFileName = "install-state.json"

// InstallState records what is installed. It is how an upgrade knows which
// top-level entries belong to the application (and may be replaced) and
// everything else is customer data.
type InstallState struct {
	Version     string        `json:"version"`
	InstalledAt time.Time     `json:"installedAt"`
	UpdatedAt   time.Time     `json:"updatedAt"`
	AppEntries  []string      `json:"appEntries"`
	DBMode      string        `json:"dbMode"`
	PGBinDir    string        `json:"pgBinDir"`
	Service     bool          `json:"service"`
	History     []StateChange `json:"history"`
}

// StateChange is one install/upgrade/rollback event.
type StateChange struct {
	Action string    `json:"action"`
	From   string    `json:"from,omitempty"`
	To     string    `json:"to"`
	At     time.Time `json:"at"`
	Backup string    `json:"backup,omitempty"`
}

// LoadState reads install-state.json; ok=false when there is no installation.
func LoadState(installDir string) (*InstallState, bool, error) {
	data, err := os.ReadFile(filepath.Join(installDir, stateFileName))
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var s InstallState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, false, fmt.Errorf("%s is corrupt: %w", stateFileName, err)
	}
	return &s, true, nil
}

// Save writes install-state.json atomically.
func (s *InstallState) Save(installDir string) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(installDir, "."+stateFileName+".tmp")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(installDir, stateFileName))
}
