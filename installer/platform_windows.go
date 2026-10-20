//go:build windows

package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

// PostgreSQL installer pinned at release build time:
//
//	-ldflags "-X main.pgInstallerURL=... -X main.pgInstallerSHA256=..."
//
// The installer refuses to run a download whose SHA-256 differs.
var (
	pgInstallerURL    string
	pgInstallerSHA256 string
)

// PostgresMajorToInstall is the PostgreSQL version new installations get.
const PostgresMajorToInstall = 17

const (
	windowsServiceID = "ezhealthkonnect"
	uninstallRegKey  = `HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\ezHealthKonnect`
	pgSuperuserFile  = "pg-superuser.secret"
)

// WindowsPlatform implements Platform with WinSW services and the EDB
// PostgreSQL installer.
type WindowsPlatform struct{}

func newPlatform() Platform { return WindowsPlatform{} }

func goosName() string { return "windows" }

func defaultInstallDir() string { return `C:\ezHealthKonnect` }

// Name implements Platform.
func (WindowsPlatform) Name() string { return "Windows" }

// DefaultInstallDir implements Platform.
func (WindowsPlatform) DefaultInstallDir() string { return defaultInstallDir() }

// IsElevated implements Platform (`net session` needs administrator rights).
func (WindowsPlatform) IsElevated() bool {
	return exec.Command("net", "session").Run() == nil
}

// Prerequisites implements Platform.
func (p WindowsPlatform) Prerequisites(cfg *Config) []CheckResult {
	var out []CheckResult
	ver, _ := exec.Command("cmd", "/c", "ver").Output()
	out = append(out, CheckResult{Name: "Operating system", OK: true, Version: strings.TrimSpace(string(ver))})
	if p.IsElevated() {
		out = append(out, CheckResult{Name: "Administrator rights", OK: true, Version: "running as administrator"})
	} else {
		out = append(out, CheckResult{Name: "Administrator rights", OK: false, Error: "right-click the installer and choose 'Run as administrator'"})
	}
	if _, err := exec.LookPath("powershell.exe"); err != nil {
		out = append(out, CheckResult{Name: "PowerShell", OK: false, Error: "powershell.exe is required to run the service"})
	} else {
		out = append(out, CheckResult{Name: "PowerShell", OK: true, Version: "available"})
	}
	if bin, major := existingPostgres(); bin != "" {
		out = append(out, CheckResult{Name: "PostgreSQL", OK: true, Version: fmt.Sprintf("%d installed — its superuser password will be needed", major)})
	} else if pgInstallerURL == "" {
		out = append(out, CheckResult{Name: "PostgreSQL", OK: true, Warning: "not installed, and this installer build includes no PostgreSQL package — install PostgreSQL 15+ first"})
	} else {
		out = append(out, CheckResult{Name: "PostgreSQL", OK: true, Version: fmt.Sprintf("will install PostgreSQL %d", PostgresMajorToInstall)})
	}
	out = append(out, diskCheck(p, `C:\`))
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
func (WindowsPlatform) FreeBytes(path string) (uint64, error) {
	k32 := syscall.NewLazyDLL("kernel32.dll")
	proc := k32.NewProc("GetDiskFreeSpaceExW")
	for p := path; ; p = filepath.Dir(p) {
		if _, err := os.Stat(p); err == nil {
			ptr, err := syscall.UTF16PtrFromString(p)
			if err != nil {
				return 0, err
			}
			var avail, total, free uint64
			r, _, callErr := proc.Call(uintptr(unsafe.Pointer(ptr)), uintptr(unsafe.Pointer(&avail)),
				uintptr(unsafe.Pointer(&total)), uintptr(unsafe.Pointer(&free)))
			if r == 0 {
				return 0, callErr
			}
			return avail, nil
		}
		if p == filepath.Dir(p) {
			return 0, fmt.Errorf("no existing directory in %s", path)
		}
	}
}

// GoAPIPath implements Platform.
func (WindowsPlatform) GoAPIPath(dir string) string { return filepath.Join(dir, "go-api.exe") }

// ExtraEnv implements Platform.
func (WindowsPlatform) ExtraEnv(string) map[string]string { return nil }

// ── PostgreSQL ────────────────────────────────────────────────────────────────

type windowsAdmin struct {
	psql, port, password string
}

func (a windowsAdmin) Exec(database, sql string) (string, error) {
	cmd := exec.Command(a.psql, "-X", "-q", "-t", "-A", "-v", "ON_ERROR_STOP=1",
		"-h", "localhost", "-p", a.port, "-U", "postgres", "-d", database, "-c", sql)
	cmd.Env = append(os.Environ(), "PGPASSWORD="+a.password, "PGCONNECT_TIMEOUT=10")
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return strings.TrimSpace(out.String()), err
}

func programFiles() string {
	if pf := os.Getenv("ProgramFiles"); pf != "" {
		return pf
	}
	return `C:\Program Files`
}

// existingPostgres returns the bin dir and major version of the newest
// installed PostgreSQL server.
func existingPostgres() (string, int) {
	best, bestDir := 0, ""
	matches, _ := filepath.Glob(filepath.Join(programFiles(), "PostgreSQL", "*", "bin", "pg_ctl.exe"))
	for _, m := range matches {
		n, err := strconv.Atoi(filepath.Base(filepath.Dir(filepath.Dir(m))))
		if err == nil && n > best {
			best, bestDir = n, filepath.Dir(m)
		}
	}
	return bestDir, best
}

func existingClientTools() string {
	best, bestDir := 0, ""
	matches, _ := filepath.Glob(filepath.Join(programFiles(), "PostgreSQL", "*", "bin", "pg_dump.exe"))
	for _, m := range matches {
		n, err := strconv.Atoi(filepath.Base(filepath.Dir(filepath.Dir(m))))
		if err == nil && n > best {
			best, bestDir = n, filepath.Dir(m)
		}
	}
	return bestDir
}

// EnsurePostgres implements Platform.
func (p WindowsPlatform) EnsurePostgres(cfg *Config, rep Reporter) (*PgSetup, error) {
	if cfg.DBMode == "external" {
		bin := existingClientTools()
		if bin == "" {
			rep.Info("Installing PostgreSQL %d command-line tools (needed for backups)", PostgresMajorToInstall)
			if err := runEDBInstaller(rep, "", cfg.DBPort, true); err != nil {
				return nil, err
			}
			bin = existingClientTools()
		}
		if bin == "" {
			return nil, fmt.Errorf("PostgreSQL client tools (psql, pg_dump) not found")
		}
		return &PgSetup{BinDir: bin}, nil
	}

	if bin, major := existingPostgres(); bin != "" {
		if major < MinPostgresMajor {
			return nil, fmt.Errorf("PostgreSQL %d is installed; version %d or newer is required", major, MinPostgresMajor)
		}
		if cfg.PGAdminPassword == "" {
			return nil, fmt.Errorf("PostgreSQL %d is already installed on this machine — enter its superuser (postgres) password so the installer can create the database", major)
		}
		admin := windowsAdmin{psql: filepath.Join(bin, "psql.exe"), port: cfg.DBPort, password: cfg.PGAdminPassword}
		if out, err := admin.Exec("postgres", "SELECT 1"); err != nil {
			return nil, fmt.Errorf("cannot sign in to PostgreSQL as postgres on port %s: %s", cfg.DBPort, firstLine(out, err))
		}
		rep.OK("Using the installed PostgreSQL %d", major)
		return &PgSetup{BinDir: bin, Admin: admin}, nil
	}

	superPassword := randomPassword()
	if err := runEDBInstaller(rep, superPassword, cfg.DBPort, false); err != nil {
		return nil, err
	}
	bin, major := existingPostgres()
	if bin == "" || major != PostgresMajorToInstall {
		return nil, fmt.Errorf("PostgreSQL %d did not install correctly", PostgresMajorToInstall)
	}
	// Kept (inside the administrators-only install directory) so uninstall can
	// remove the database; never printed or logged.
	if err := os.MkdirAll(filepath.Join(cfg.InstallDir, "tools"), 0o700); err == nil {
		_ = os.WriteFile(filepath.Join(cfg.InstallDir, "tools", pgSuperuserFile), []byte(superPassword), 0o600)
	}
	admin := windowsAdmin{psql: filepath.Join(bin, "psql.exe"), port: cfg.DBPort, password: superPassword}
	var lastErr error
	for i := 0; i < 30; i++ {
		var out string
		if out, lastErr = admin.Exec("postgres", "SELECT 1"); lastErr == nil {
			rep.OK("PostgreSQL %d installed and running", PostgresMajorToInstall)
			return &PgSetup{BinDir: bin, Admin: admin}, nil
		} else {
			lastErr = fmt.Errorf("%s", firstLine(out, lastErr))
		}
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("PostgreSQL %d did not start: %v", PostgresMajorToInstall, lastErr)
}

// PostgresAdmin implements Platform.
func (WindowsPlatform) PostgresAdmin(dir string, cfg *Config) AdminSQL {
	bin, _ := existingPostgres()
	pw, err := os.ReadFile(filepath.Join(dir, "tools", pgSuperuserFile))
	if bin == "" || err != nil {
		return nil
	}
	return windowsAdmin{psql: filepath.Join(bin, "psql.exe"), port: cfg.DBPort, password: strings.TrimSpace(string(pw))}
}

// runEDBInstaller downloads the pinned PostgreSQL installer, verifies its
// SHA-256 and runs it unattended.
func runEDBInstaller(rep Reporter, superPassword, port string, clientOnly bool) error {
	if pgInstallerURL == "" || len(pgInstallerSHA256) != 64 {
		return fmt.Errorf("this installer build does not include a PostgreSQL package — install PostgreSQL %d+ first, then run the installer again", MinPostgresMajor)
	}
	tmp := filepath.Join(os.TempDir(), "ezhk-postgresql-installer.exe")
	defer os.Remove(tmp)
	rep.Info("Downloading PostgreSQL %d (~370 MB)…", PostgresMajorToInstall)
	if err := downloadVerified(pgInstallerURL, pgInstallerSHA256, tmp); err != nil {
		return err
	}
	rep.Info("Installing PostgreSQL (this can take a few minutes)…")
	args := []string{"--mode", "unattended", "--unattendedmodeui", "none"}
	if clientOnly {
		args = append(args, "--enable-components", "commandlinetools", "--disable-components", "server,pgAdmin,stackbuilder")
	} else {
		args = append(args, "--enable-components", "server,commandlinetools", "--disable-components", "pgAdmin,stackbuilder",
			"--superpassword", superPassword, "--serverport", port,
			"--servicename", fmt.Sprintf("postgresql-x64-%d", PostgresMajorToInstall))
	}
	out, err := exec.Command(tmp, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("PostgreSQL installer failed: %v %s", err, firstLine(string(out), err))
	}
	return nil
}

func downloadVerified(url, wantSHA, dest string) error {
	client := &http.Client{Timeout: 30 * time.Minute}
	resp, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	h := sha256.New()
	_, err = io.Copy(io.MultiWriter(f, h), resp.Body)
	f.Close()
	if err != nil {
		return fmt.Errorf("download %s: %w", url, err)
	}
	if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, wantSHA) {
		os.Remove(dest)
		return fmt.Errorf("the PostgreSQL download failed its integrity check (SHA-256 %s, expected %s) — not running it", got, wantSHA)
	}
	return nil
}

// ── Files and access control ──────────────────────────────────────────────────

// PrepareInstall implements Platform: creates data directories and restricts
// the whole installation to SYSTEM and Administrators. Without this, the
// folder inherits C:\'s permissions, which let ordinary signed-in users
// modify it — and the service runs as LocalSystem from it.
func (WindowsPlatform) PrepareInstall(dir string) error {
	for _, d := range dataDirs {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o700); err != nil {
			return err
		}
	}
	out, err := exec.Command("icacls", dir, "/inheritance:r",
		"/grant:r", "*S-1-5-18:(OI)(CI)F", // SYSTEM
		"/grant:r", "*S-1-5-32-544:(OI)(CI)F", // Administrators
		"/Q").CombinedOutput()
	if err != nil {
		return fmt.Errorf("restrict access to %s: %s", dir, strings.TrimSpace(string(out)))
	}
	return nil
}

// ── Service (WinSW) ───────────────────────────────────────────────────────────

func serviceExe(dir string) string { return filepath.Join(dir, "tools", windowsServiceID+".exe") }

// InstallService implements Platform: one service runs start-app.ps1, which
// keeps the Go backend and the Node.js frontend together.
func (p WindowsPlatform) InstallService(dir string, _ *Config) error {
	if p.ServiceInstalled() {
		_ = exec.Command(serviceExe(dir), "stop").Run()
		_ = exec.Command(serviceExe(dir), "uninstall").Run()
	}
	if err := copyFileTo(filepath.Join(dir, "runtime", "winsw", "WinSW-x64.exe"), serviceExe(dir)); err != nil {
		return fmt.Errorf("install service wrapper: %w", err)
	}
	ps, err := exec.LookPath("powershell.exe")
	if err != nil {
		return err
	}
	xml := fmt.Sprintf(`<service>
  <id>%[1]s</id>
  <name>ezHealthKonnect</name>
  <description>ezHealthKonnect healthcare integration engine</description>
  <executable>%[2]s</executable>
  <arguments>-NoProfile -ExecutionPolicy Bypass -File "%[3]s\start-app.ps1" -NodePath "%[3]s\runtime\node\node.exe"</arguments>
  <workingdirectory>%[3]s</workingdirectory>
  <logpath>%[3]s\logs</logpath>
  <log mode="roll-by-size"><sizeThreshold>10240</sizeThreshold><keepFiles>5</keepFiles></log>
  <startmode>Automatic</startmode>
  <delayedAutoStart>true</delayedAutoStart>
  <stoptimeout>60 sec</stoptimeout>
  <onfailure action="restart" delay="10 sec"/>
  <onfailure action="restart" delay="30 sec"/>
  <resetfailure>1 hour</resetfailure>
  <stopparentprocessfirst>false</stopparentprocessfirst>
</service>
`, windowsServiceID, ps, dir)
	if err := os.WriteFile(filepath.Join(dir, "tools", windowsServiceID+".xml"), []byte(xml), 0o600); err != nil {
		return err
	}
	if out, err := exec.Command(serviceExe(dir), "install").CombinedOutput(); err != nil {
		return fmt.Errorf("register service: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func serviceDirFromRegistry() string {
	out, err := exec.Command("reg", "query", uninstallRegKey, "/v", "InstallLocation").Output()
	if err != nil {
		return ""
	}
	for _, l := range strings.Split(string(out), "\n") {
		if i := strings.Index(l, "REG_SZ"); i >= 0 {
			return strings.TrimSpace(l[i+len("REG_SZ"):])
		}
	}
	return ""
}

func scRun(args ...string) error {
	out, err := exec.Command("sc.exe", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("sc %s: %s", strings.Join(args, " "), strings.TrimSpace(string(out)))
	}
	return nil
}

// StartService implements Platform.
func (WindowsPlatform) StartService() error { return scRun("start", windowsServiceID) }

// StopService implements Platform; waits until the service has stopped.
func (p WindowsPlatform) StopService() error {
	if !p.ServiceInstalled() {
		return nil
	}
	if dir := serviceDirFromRegistry(); dir != "" {
		if _, err := os.Stat(serviceExe(dir)); err == nil {
			out, err := exec.Command(serviceExe(dir), "stop").CombinedOutput()
			if err != nil && !strings.Contains(string(out), "not started") {
				return fmt.Errorf("stop service: %s", strings.TrimSpace(string(out)))
			}
		}
	}
	_ = scRun("stop", windowsServiceID)
	for i := 0; i < 60; i++ {
		out, _ := exec.Command("sc.exe", "query", windowsServiceID).Output()
		if strings.Contains(string(out), "STOPPED") {
			return nil
		}
		time.Sleep(time.Second)
	}
	return fmt.Errorf("the service did not stop within 60 seconds")
}

// ServiceInstalled implements Platform.
func (WindowsPlatform) ServiceInstalled() bool {
	return exec.Command("sc.exe", "query", windowsServiceID).Run() == nil
}

// RemoveService implements Platform.
func (p WindowsPlatform) RemoveService(dir string) error {
	if !p.ServiceInstalled() {
		return nil
	}
	_ = p.StopService()
	if _, err := os.Stat(serviceExe(dir)); err == nil {
		if out, err := exec.Command(serviceExe(dir), "uninstall").CombinedOutput(); err == nil {
			return nil
		} else {
			return fmt.Errorf("remove service: %s", strings.TrimSpace(string(out)))
		}
	}
	return scRun("delete", windowsServiceID)
}

// RegisterUninstall implements Platform: an Add/Remove Programs entry that
// runs the installer copy kept in tools\.
func (WindowsPlatform) RegisterUninstall(dir, ver, _ string) error {
	if err := keepInstallerCopy(dir, "ezHealthKonnect-Setup.exe"); err != nil {
		return err
	}
	setup := filepath.Join(dir, "tools", "ezHealthKonnect-Setup.exe")
	values := [][3]string{
		{"DisplayName", "REG_SZ", "ezHealthKonnect"},
		{"DisplayVersion", "REG_SZ", ver},
		{"Publisher", "REG_SZ", "ezInterOp Solutions"},
		{"InstallLocation", "REG_SZ", dir},
		{"UninstallString", "REG_SZ", fmt.Sprintf(`"%s" --uninstall`, setup)},
		{"DisplayIcon", "REG_SZ", setup},
		{"URLInfoAbout", "REG_SZ", "https://www.ezInterOpSolutions.com"},
		{"NoModify", "REG_DWORD", "1"},
		{"NoRepair", "REG_DWORD", "1"},
	}
	for _, v := range values {
		if out, err := exec.Command("reg", "add", uninstallRegKey, "/v", v[0], "/t", v[1], "/d", v[2], "/f").CombinedOutput(); err != nil {
			return fmt.Errorf("registry %s: %s", v[0], strings.TrimSpace(string(out)))
		}
	}
	return nil
}

// RemoveUninstall implements Platform.
func (WindowsPlatform) RemoveUninstall() {
	_ = exec.Command("reg", "delete", uninstallRegKey, "/f").Run()
}

func embeddedBundle() []byte { return embeddedWindowsBundle }
