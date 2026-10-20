//go:build linux

package main

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// PostgresMajorToInstall is the PostgreSQL version new installations get.
const PostgresMajorToInstall = 17

const (
	linuxServiceUser = "ezhealthkonnect"
	apiUnit          = "ezhealthkonnect-api"
	webUnit          = "ezhealthkonnect"
	unitDir          = "/etc/systemd/system"
)

// LinuxPlatform implements Platform with systemd and the distribution's
// package manager (apt or dnf/yum, using the official PostgreSQL repositories).
type LinuxPlatform struct{}

func newPlatform() Platform { return LinuxPlatform{} }

func goosName() string { return "linux" }

func defaultInstallDir() string { return "/opt/ezhealthkonnect" }

// Name implements Platform.
func (LinuxPlatform) Name() string { return "Linux" }

// DefaultInstallDir implements Platform.
func (LinuxPlatform) DefaultInstallDir() string { return defaultInstallDir() }

// IsElevated implements Platform.
func (LinuxPlatform) IsElevated() bool { return os.Geteuid() == 0 }

// Prerequisites implements Platform.
func (p LinuxPlatform) Prerequisites(cfg *Config) []CheckResult {
	var out []CheckResult
	osName := "Linux"
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		for _, l := range strings.Split(string(data), "\n") {
			if v, ok := strings.CutPrefix(l, "PRETTY_NAME="); ok {
				osName = strings.Trim(v, `"`)
			}
		}
	}
	out = append(out, CheckResult{Name: "Operating system", OK: true, Version: osName})
	if p.IsElevated() {
		out = append(out, CheckResult{Name: "Administrator rights", OK: true, Version: "running as root"})
	} else {
		out = append(out, CheckResult{Name: "Administrator rights", OK: false, Error: "run the installer with sudo"})
	}
	if _, err := exec.LookPath("systemctl"); err != nil {
		out = append(out, CheckResult{Name: "systemd", OK: false, Error: "systemd is required to run ezHealthKonnect as a service"})
	} else {
		out = append(out, CheckResult{Name: "systemd", OK: true, Version: "available"})
	}
	if pm := packageManager(); pm == "" {
		out = append(out, CheckResult{Name: "Package manager", OK: true, Warning: "apt/dnf/yum not found — PostgreSQL must already be installed"})
	} else {
		out = append(out, CheckResult{Name: "Package manager", OK: true, Version: pm})
	}
	out = append(out, diskCheck(p, "/opt"))
	for _, port := range []string{cfg.AppPort, cfg.APIPort} {
		out = append(out, portCheck(port))
	}
	return out
}

func diskCheck(p Platform, path string) CheckResult {
	free, err := p.FreeBytes(path)
	if err != nil {
		return CheckResult{Name: "Disk space", OK: true, Warning: "could not determine free space"}
	}
	mb := free >> 20
	if mb < 4096 {
		return CheckResult{Name: "Disk space", OK: mb >= 2048, Error: fmt.Sprintf("%d MB free (4 GB recommended)", mb)}
	}
	return CheckResult{Name: "Disk space", OK: true, Version: fmt.Sprintf("%d GB free", mb>>10)}
}

func portCheck(port string) CheckResult {
	name := "Port " + port
	l, err := net.Listen("tcp", ":"+port)
	if err != nil {
		return CheckResult{Name: name, OK: true, Warning: "in use — choose another port in the next step"}
	}
	l.Close()
	return CheckResult{Name: name, OK: true, Version: "available"}
}

// FreeBytes implements Platform.
func (LinuxPlatform) FreeBytes(path string) (uint64, error) {
	for p := path; ; p = filepath.Dir(p) {
		var st syscall.Statfs_t
		if err := syscall.Statfs(p, &st); err == nil {
			return st.Bavail * uint64(st.Bsize), nil
		}
		if p == filepath.Dir(p) {
			return 0, fmt.Errorf("statfs %s failed", path)
		}
	}
}

// GoAPIPath implements Platform.
func (LinuxPlatform) GoAPIPath(dir string) string { return filepath.Join(dir, "go-api") }

// ExtraEnv implements Platform: the service account has no home directory,
// so caches (e.g. the Snowflake driver's) go inside the installation.
func (LinuxPlatform) ExtraEnv(dir string) map[string]string {
	return map[string]string{
		"XDG_CACHE_HOME": filepath.Join(dir, "cache"),
		"HOME":           filepath.Join(dir, "cache"),
	}
}

// ── PostgreSQL ────────────────────────────────────────────────────────────────

// linuxAdmin runs SQL as the postgres OS user (peer authentication).
type linuxAdmin struct {
	psql string
	port string
}

func (a linuxAdmin) Exec(database, sql string) (string, error) {
	cmd := exec.Command("runuser", "-u", "postgres", "--", a.psql, "-X", "-q", "-t", "-A",
		"-v", "ON_ERROR_STOP=1", "-p", a.port, "-d", database, "-c", sql)
	cmd.Dir = "/"
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
}

// EnsurePostgres implements Platform.
func (p LinuxPlatform) EnsurePostgres(cfg *Config, rep Reporter) (*PgSetup, error) {
	if cfg.DBMode == "external" {
		bin := newestPgBinDir()
		if bin == "" {
			rep.Info("Installing PostgreSQL %d client tools (needed for backups)", PostgresMajorToInstall)
			if err := installPostgresPackages(true, rep); err != nil {
				return nil, err
			}
			bin = newestPgBinDir()
		}
		if bin == "" {
			return nil, fmt.Errorf("PostgreSQL client tools (psql, pg_dump) not found")
		}
		return &PgSetup{BinDir: bin}, nil
	}

	if bin := newestPgBinDir(); bin != "" {
		admin := linuxAdmin{psql: filepath.Join(bin, "psql"), port: cfg.DBPort}
		if out, err := admin.Exec("postgres", "SHOW server_version_num"); err == nil {
			n, _ := strconv.Atoi(strings.TrimSpace(out))
			if n/10000 < MinPostgresMajor {
				return nil, fmt.Errorf("PostgreSQL %d is running on port %s; version %d or newer is required", n/10000, cfg.DBPort, MinPostgresMajor)
			}
			rep.OK("Using the PostgreSQL %d server already running on port %s", n/10000, cfg.DBPort)
			return &PgSetup{BinDir: bin, Admin: admin}, nil
		}
	}

	rep.Info("Installing PostgreSQL %d from the official PostgreSQL repository", PostgresMajorToInstall)
	if err := installPostgresPackages(false, rep); err != nil {
		return nil, err
	}
	bin := pgBinDirFor(PostgresMajorToInstall)
	if bin == "" {
		return nil, fmt.Errorf("PostgreSQL %d was installed but its programs were not found", PostgresMajorToInstall)
	}
	port, err := clusterPort(PostgresMajorToInstall)
	if err == nil && port != "" && port != cfg.DBPort {
		rep.Info("PostgreSQL %d is listening on port %s", PostgresMajorToInstall, port)
		cfg.DBPort = port
	}
	admin := linuxAdmin{psql: filepath.Join(bin, "psql"), port: cfg.DBPort}
	var lastErr error
	var lastOut string
	for i := 0; i < 30; i++ {
		if lastOut, lastErr = admin.Exec("postgres", "SELECT 1"); lastErr == nil {
			rep.OK("PostgreSQL %d installed and running", PostgresMajorToInstall)
			return &PgSetup{BinDir: bin, Admin: admin}, nil
		}
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("PostgreSQL %d is not reachable on port %s: %s", PostgresMajorToInstall, cfg.DBPort, firstLine(lastOut, lastErr))
}

// PostgresAdmin implements Platform.
func (LinuxPlatform) PostgresAdmin(_ string, cfg *Config) AdminSQL {
	bin := newestPgBinDir()
	if bin == "" {
		return nil
	}
	return linuxAdmin{psql: filepath.Join(bin, "psql"), port: cfg.DBPort}
}

func packageManager() string {
	for _, pm := range []string{"apt-get", "dnf", "yum"} {
		if _, err := exec.LookPath(pm); err == nil {
			return pm
		}
	}
	return ""
}

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		lines := strings.Split(strings.TrimSpace(out.String()), "\n")
		if len(lines) > 8 {
			lines = lines[len(lines)-8:]
		}
		return fmt.Errorf("%s %s failed: %s", name, strings.Join(args, " "), strings.Join(lines, " | "))
	}
	return nil
}

// installPostgresPackages installs PostgreSQL (server, or client tools only)
// from the official PostgreSQL (PGDG) repositories.
func installPostgresPackages(clientOnly bool, rep Reporter) error {
	major := strconv.Itoa(PostgresMajorToInstall)
	switch packageManager() {
	case "apt-get":
		if err := addPgdgApt(rep); err != nil {
			return err
		}
		pkgs := []string{"postgresql-client-" + major}
		if !clientOnly {
			pkgs = append(pkgs, "postgresql-"+major)
		}
		if err := run("apt-get", append([]string{"install", "-y", "-q"}, pkgs...)...); err != nil {
			return err
		}
		if clientOnly {
			return nil
		}
		port, _ := clusterPort(PostgresMajorToInstall)
		for _, c := range debianClusterCommands(major, port != "") {
			if err := run(c[0], c[1:]...); err != nil {
				return fmt.Errorf("start PostgreSQL %s: %w", major, err)
			}
		}
		return nil
	case "dnf", "yum":
		pm := packageManager()
		if err := run(pm, "install", "-y", "-q", pgdgRPMURL()); err != nil && !strings.Contains(err.Error(), "already installed") {
			rep.Warn("PostgreSQL repository package: %v", err)
		}
		if err := configurePgdgRepos(PostgresMajorToInstall); err != nil {
			return err
		}
		_ = run(pm, "-qy", "module", "disable", "postgresql")
		pkgs := []string{"postgresql" + major}
		if !clientOnly {
			pkgs = append(pkgs, "postgresql"+major+"-server", "postgresql"+major+"-contrib")
		}
		if err := run(pm, append([]string{"install", "-y", "-q"}, pkgs...)...); err != nil {
			return err
		}
		if clientOnly {
			return nil
		}
		setup := fmt.Sprintf("/usr/pgsql-%s/bin/postgresql-%s-setup", major, major)
		if _, err := os.Stat(fmt.Sprintf("/var/lib/pgsql/%s/data/PG_VERSION", major)); os.IsNotExist(err) {
			cmd := exec.Command(setup, "initdb")
			// Explicit: peer locally, password (SCRAM) over TCP — never the
			// distribution's historical "ident" default, which the app cannot use.
			cmd.Env = append(os.Environ(), "PGSETUP_INITDB_OPTIONS=--auth-local=peer --auth-host=scram-sha-256 --encoding=UTF8")
			if out, err := cmd.CombinedOutput(); err != nil {
				return fmt.Errorf("initialise PostgreSQL: %s", strings.TrimSpace(string(out)))
			}
		}
		return run("systemctl", "enable", "--now", "postgresql-"+major)
	default:
		return fmt.Errorf("no supported package manager (apt-get, dnf, yum): install PostgreSQL %d yourself and choose 'existing database'", PostgresMajorToInstall)
	}
}

// addPgdgApt adds the official PostgreSQL apt repository with a dedicated
// signing key (not a system-wide trusted key).
func addPgdgApt(rep Reporter) error {
	codename := osReleaseValue("VERSION_CODENAME")
	if codename == "" {
		return fmt.Errorf("cannot determine the distribution codename")
	}
	if err := run("apt-get", "install", "-y", "-q", "ca-certificates", "curl"); err != nil {
		return err
	}
	keyring := "/usr/share/keyrings/ezhk-postgresql.asc"
	if err := run("curl", "-fsSL", "-o", keyring, "https://www.postgresql.org/media/keys/ACCC4CF8.asc"); err != nil {
		return err
	}
	line := fmt.Sprintf("deb [signed-by=%s] https://apt.postgresql.org/pub/repos/apt %s-pgdg main\n", keyring, codename)
	if err := os.WriteFile("/etc/apt/sources.list.d/ezhk-pgdg.list", []byte(line), 0o644); err != nil {
		return err
	}
	return run("apt-get", "update", "-q")
}

func osReleaseValue(key string) string {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	for _, l := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(l, key+"="); ok {
			return strings.Trim(v, `"'`)
		}
	}
	return ""
}

func pgdgRPMURL() string {
	el := osReleaseValue("VERSION_ID")
	if i := strings.Index(el, "."); i >= 0 {
		el = el[:i]
	}
	if el != "8" && el != "9" && el != "10" {
		el = "9"
	}
	arch := "x86_64"
	if runtime.GOARCH == "arm64" {
		arch = "aarch64"
	}
	return fmt.Sprintf("https://download.postgresql.org/pub/repos/yum/reporpms/EL-%s-%s/pgdg-redhat-repo-latest.noarch.rpm", el, arch)
}

// configurePgdgRepos keeps only the PostgreSQL version we install (plus
// pgdg-common) and points them at PGDG's major-version directory: the repo
// package enables every version (one broken mirror then fails every dnf
// command) and uses OS minor-version paths that 404 on servers not on the
// newest minor release. Same fix as installer/install.sh (verified on Rocky 9.3).
func configurePgdgRepos(major int) error {
	files, _ := filepath.Glob("/etc/yum.repos.d/pgdg-redhat*.repo")
	for _, f := range files {
		if err := rewritePgdgRepoFile(f, major); err != nil {
			return err
		}
	}
	return nil
}

// rewritePgdgRepoFile disables every pgdgNN repo except the one we install
// and switches baseurls to the major-version directory.
func rewritePgdgRepoFile(f string, major int) error {
	keep := "pgdg" + strconv.Itoa(major)
	data, err := os.ReadFile(f)
	if err != nil {
		return err
	}
	var out []string
	section := ""
	for _, line := range strings.Split(string(data), "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			section = strings.Trim(t, "[]")
		}
		if isVersionRepo(section) && section != keep && strings.HasPrefix(t, "enabled") {
			line = "enabled=0"
		}
		if strings.HasPrefix(t, "baseurl") {
			line = strings.ReplaceAll(line, "$releasever_major.$releasever_minor", "$releasever_major")
		}
		out = append(out, line)
	}
	return os.WriteFile(f, []byte(strings.Join(out, "\n")), 0o644)
}

func isVersionRepo(section string) bool {
	rest, ok := strings.CutPrefix(section, "pgdg")
	if !ok || rest == "" {
		return false
	}
	_, err := strconv.Atoi(rest)
	return err == nil
}

func pgBinDirFor(major int) string {
	for _, d := range []string{
		fmt.Sprintf("/usr/lib/postgresql/%d/bin", major), // Debian/Ubuntu
		fmt.Sprintf("/usr/pgsql-%d/bin", major),          // RHEL family (PGDG)
	} {
		if _, err := os.Stat(filepath.Join(d, "pg_dump")); err == nil {
			return d
		}
	}
	return ""
}

// newestPgBinDir returns the newest installed PostgreSQL client tool dir.
func newestPgBinDir() string {
	var majors []int
	for _, pattern := range []string{"/usr/lib/postgresql/*/bin/pg_dump", "/usr/pgsql-*/bin/pg_dump"} {
		matches, _ := filepath.Glob(pattern)
		for _, m := range matches {
			v := filepath.Base(filepath.Dir(filepath.Dir(m)))
			v = strings.TrimPrefix(v, "pgsql-")
			if n, err := strconv.Atoi(v); err == nil {
				majors = append(majors, n)
			}
		}
	}
	if len(majors) == 0 {
		if p, err := exec.LookPath("pg_dump"); err == nil {
			return filepath.Dir(p)
		}
		return ""
	}
	sort.Ints(majors)
	return pgBinDirFor(majors[len(majors)-1])
}

// clusterPort reads the port of the Debian/Ubuntu cluster (pg_lsclusters);
// on RHEL the default 5432 applies.
// debianClusterCommands makes sure the Debian/Ubuntu cluster "<major>/main"
// exists, starts at boot and is running. apt normally creates it, but not when
// /etc/postgresql-common/createcluster.conf disables that, and it does not
// start it when the postgresql service is disabled — common on hosts that
// already had another PostgreSQL version (whose cluster then also keeps 5432,
// so the new one gets the next free port; clusterPort reads it afterwards).
func debianClusterCommands(major string, exists bool) [][]string {
	var cmds [][]string
	if !exists {
		cmds = append(cmds, []string{"pg_createcluster", major, "main", "--",
			"--auth-local=peer", "--auth-host=scram-sha-256", "--encoding=UTF8"})
	}
	return append(cmds, []string{"systemctl", "enable", "--now", "postgresql@" + major + "-main"})
}

func clusterPort(major int) (string, error) {
	out, err := exec.Command("pg_lsclusters", "--no-header").Output()
	if err != nil {
		return "", err
	}
	for _, l := range strings.Split(string(out), "\n") {
		f := strings.Fields(l)
		if len(f) >= 3 && f[0] == strconv.Itoa(major) {
			return f[2], nil
		}
	}
	return "", nil
}

// ── Files, user and permissions ───────────────────────────────────────────────

// PrepareInstall implements Platform. Application files stay owned by root
// (the service cannot modify its own code); the service account owns only the
// data directories it writes; backups and the state file are root-only; .env
// is readable by the service but writable only by root.
func (p LinuxPlatform) PrepareInstall(dir string) error {
	uid, gid, err := ensureServiceUser()
	if err != nil {
		return err
	}
	if err := os.Chown(dir, 0, gid); err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o750); err != nil {
		return err
	}
	for _, d := range dataDirs {
		path := filepath.Join(dir, d)
		created := false
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if err := os.MkdirAll(path, 0o700); err != nil {
				return err
			}
			created = true
		}
		owner, group := uid, gid
		if d == "backups" || d == "tools" {
			owner, group = 0, 0
		}
		if err := os.Chown(path, owner, group); err != nil {
			return err
		}
		if err := os.Chmod(path, 0o700); err != nil {
			return err
		}
		_ = created // existing contents keep their owner; new files are created by the service itself
	}
	if err := os.Chown(filepath.Join(dir, ".env"), 0, gid); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Chmod(filepath.Join(dir, ".env"), 0o640); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := os.Chmod(filepath.Join(dir, stateFileName), 0o600); err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, exe := range []string{"go-api", "runtime/node/bin/node"} {
		_ = os.Chmod(filepath.Join(dir, exe), 0o755)
	}
	return nil
}

func ensureServiceUser() (int, int, error) {
	if _, err := user.Lookup(linuxServiceUser); err != nil {
		nologin := "/usr/sbin/nologin"
		if _, err := os.Stat(nologin); err != nil {
			nologin = "/sbin/nologin"
		}
		if out, err := exec.Command("useradd", "--system", "--user-group", "--no-create-home",
			"--home-dir", "/nonexistent", "--shell", nologin, linuxServiceUser).CombinedOutput(); err != nil {
			return 0, 0, fmt.Errorf("create service account: %s", strings.TrimSpace(string(out)))
		}
	}
	// Serial devices (RS232 medical devices) belong to the dialout group.
	if _, err := user.LookupGroup("dialout"); err == nil {
		_ = exec.Command("usermod", "-a", "-G", "dialout", linuxServiceUser).Run()
	}
	u, err := user.Lookup(linuxServiceUser)
	if err != nil {
		return 0, 0, err
	}
	uid, _ := strconv.Atoi(u.Uid)
	gid, _ := strconv.Atoi(u.Gid)
	return uid, gid, nil
}

// ── systemd services ──────────────────────────────────────────────────────────

const unitHardening = `NoNewPrivileges=yes
ProtectSystem=full
ProtectHome=yes
PrivateTmp=yes
ProtectKernelTunables=yes
ProtectKernelModules=yes
ProtectControlGroups=yes
RestrictSUIDSGID=yes
LockPersonality=yes
UMask=0077
LimitNOFILE=65536`

// InstallService implements Platform.
func (LinuxPlatform) InstallService(dir string, _ *Config) error {
	groups := ""
	if _, err := user.LookupGroup("dialout"); err == nil {
		groups = "SupplementaryGroups=dialout\n"
	}
	api := fmt.Sprintf(`[Unit]
Description=ezHealthKonnect processing engine
After=network-online.target postgresql.service postgresql-%[4]d.service
Wants=network-online.target

[Service]
Type=simple
User=%[1]s
Group=%[1]s
%[3]sWorkingDirectory=%[2]s
EnvironmentFile=%[2]s/.env
ExecStart=%[2]s/go-api
Restart=on-failure
RestartSec=5
# Graceful shutdown drains in-flight messages (up to ~45 s).
TimeoutStopSec=60
# Lets device listeners bind ports below 1024 (e.g. DICOM 104) without root.
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
%[5]s

[Install]
WantedBy=multi-user.target
`, linuxServiceUser, dir, groups, PostgresMajorToInstall, unitHardening)

	web := fmt.Sprintf(`[Unit]
Description=ezHealthKonnect web application
After=%[3]s.service
Requires=%[3]s.service
PartOf=%[3]s.service

[Service]
Type=simple
User=%[1]s
Group=%[1]s
WorkingDirectory=%[2]s
EnvironmentFile=%[2]s/.env
ExecStart=%[2]s/runtime/node/bin/node %[2]s/server.js
Restart=on-failure
RestartSec=5
TimeoutStopSec=30
CapabilityBoundingSet=
%[4]s

[Install]
WantedBy=multi-user.target
`, linuxServiceUser, dir, apiUnit, unitHardening)

	for name, content := range map[string]string{apiUnit: api, webUnit: web} {
		if err := os.WriteFile(filepath.Join(unitDir, name+".service"), []byte(content), 0o644); err != nil {
			return fmt.Errorf("write %s unit: %w", name, err)
		}
	}
	if err := run("systemctl", "daemon-reload"); err != nil {
		return err
	}
	return run("systemctl", "enable", apiUnit, webUnit)
}

// StartService implements Platform.
func (LinuxPlatform) StartService() error {
	return run("systemctl", "start", apiUnit, webUnit)
}

// StopService implements Platform (systemctl waits until both have exited).
func (LinuxPlatform) StopService() error {
	if !(LinuxPlatform{}).ServiceInstalled() {
		return nil
	}
	return run("systemctl", "stop", webUnit, apiUnit)
}

// ServiceInstalled implements Platform.
func (LinuxPlatform) ServiceInstalled() bool {
	_, err := os.Stat(filepath.Join(unitDir, apiUnit+".service"))
	return err == nil
}

// RemoveService implements Platform.
func (p LinuxPlatform) RemoveService(string) error {
	_ = run("systemctl", "disable", "--now", webUnit, apiUnit)
	for _, u := range []string{webUnit, apiUnit} {
		os.Remove(filepath.Join(unitDir, u+".service"))
	}
	_ = run("systemctl", "daemon-reload")
	_ = exec.Command("userdel", linuxServiceUser).Run()
	return nil
}

// RegisterUninstall implements Platform: keeps a copy of this installer in
// tools/ so upgrade, rollback and uninstall work without the original file.
func (LinuxPlatform) RegisterUninstall(dir, _, _ string) error {
	return keepInstallerCopy(dir, "ezhealthkonnect-setup")
}

// RemoveUninstall implements Platform.
func (LinuxPlatform) RemoveUninstall() {}

func embeddedBundle() []byte { return embeddedLinuxBundle }
