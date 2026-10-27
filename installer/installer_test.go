package main

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

// ── fakes ─────────────────────────────────────────────────────────────────────

type fakePlatform struct {
	mu        sync.Mutex
	calls     []string
	elevated  bool
	startErr  error
	installed bool
	noAdmin   bool     // PostgresAdmin has no stored administrator access (PostgreSQL that was already installed)
	admin     AdminSQL // when set, the administrator EnsurePostgres hands to setup
}

func (f *fakePlatform) log(s string) { f.mu.Lock(); f.calls = append(f.calls, s); f.mu.Unlock() }
func (f *fakePlatform) Name() string { return "test" }
func (f *fakePlatform) DefaultInstallDir() string {
	return filepath.Join(os.TempDir(), "never-used")
}
func (f *fakePlatform) IsElevated() bool                    { return f.elevated }
func (f *fakePlatform) Prerequisites(*Config) []CheckResult { return nil }
func (f *fakePlatform) EnsurePostgres(cfg *Config, _ Reporter) (*PgSetup, error) {
	f.log("ensure-postgres")
	if f.admin != nil {
		return &PgSetup{BinDir: "/pg/bin", Admin: f.admin}, nil
	}
	return &PgSetup{BinDir: "/pg/bin", Admin: fakeAdmin{}}, nil
}
func (f *fakePlatform) PostgresAdmin(string, *Config) AdminSQL {
	if f.noAdmin {
		return nil
	}
	return fakeAdmin{}
}
func (f *fakePlatform) GoAPIPath(dir string) string { return filepath.Join(dir, "go-api") }
func (f *fakePlatform) ExtraEnv(dir string) map[string]string {
	return map[string]string{"XDG_CACHE_HOME": filepath.Join(dir, "cache")}
}
func (f *fakePlatform) PrepareInstall(dir string) error {
	f.log("prepare")
	for _, d := range dataDirs {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o700); err != nil {
			return err
		}
	}
	return nil
}
func (f *fakePlatform) InstallService(string, *Config) error {
	f.log("install-service")
	f.installed = true
	return nil
}
func (f *fakePlatform) StartService() error        { f.log("start"); return f.startErr }
func (f *fakePlatform) StopService() error         { f.log("stop"); return nil }
func (f *fakePlatform) RemoveService(string) error { f.log("remove-service"); return nil }
func (f *fakePlatform) ServiceInstalled() bool     { return f.installed }
func (f *fakePlatform) RegisterUninstall(string, string, string) error {
	f.log("register")
	return nil
}
func (f *fakePlatform) RemoveUninstall()                 {}
func (f *fakePlatform) FreeBytes(string) (uint64, error) { return 100 << 30, nil }

func (f *fakePlatform) callString() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return strings.Join(f.calls, ",")
}

type fakeAdmin struct{}

func (fakeAdmin) Exec(_, sql string) (string, error) {
	if strings.Contains(sql, "pg_database WHERE") {
		return "", nil // database does not exist yet
	}
	return "", nil
}

type fakePg struct {
	mu       sync.Mutex
	calls    []string
	dumpErr  error
	dbState  string // simulated database content, e.g. "v1"
	dumped   string
	restored string
}

func (p *fakePg) log(s string)                    { p.mu.Lock(); p.calls = append(p.calls, s); p.mu.Unlock() }
func (p *fakePg) Ping(PgConn) error               { return nil }
func (p *fakePg) ServerMajor(PgConn) (int, error) { return 17, nil }
func (p *fakePg) ClientMajor() (int, error)       { return 17, nil }
func (p *fakePg) HasTables(PgConn) (bool, error)  { return false, nil }
func (p *fakePg) Dump(_ PgConn, file string) error {
	p.log("dump")
	if p.dumpErr != nil {
		return p.dumpErr
	}
	p.dumped = p.dbState
	return os.WriteFile(file, []byte("DUMP:"+p.dbState), 0o600)
}
func (p *fakePg) VerifyDump(file string) error {
	p.log("verify")
	data, err := os.ReadFile(file)
	if err != nil || !strings.HasPrefix(string(data), "DUMP:") {
		return errors.New("bad dump")
	}
	return nil
}
func (p *fakePg) Restore(_ PgConn, file string) error {
	p.log("restore")
	data, err := os.ReadFile(file)
	if err != nil {
		return err
	}
	p.dbState = strings.TrimPrefix(string(data), "DUMP:")
	p.restored = p.dbState
	return nil
}

type fakeMigrator struct {
	pg   *fakePg
	fail bool
	runs int
}

func (m *fakeMigrator) Migrate(dir, goAPI string, _ Reporter) error {
	m.runs++
	if _, err := os.Stat(goAPI); err != nil {
		return fmt.Errorf("go-api missing: %w", err)
	}
	ver, _ := os.ReadFile(filepath.Join(dir, "VERSION"))
	if m.fail {
		m.pg.dbState = "half-migrated-" + string(ver)
		return errors.New("migration V999 failed")
	}
	m.pg.dbState = string(ver)
	return nil
}

type fakeHealth struct {
	failOnce bool
	checks   int
}

func (h *fakeHealth) WaitHealthy(string, time.Duration) error {
	h.checks++
	if h.failOnce {
		h.failOnce = false
		return errors.New("not healthy")
	}
	return nil
}

// makeBundle writes an application bundle zip for version v with the given
// app files (top-level name -> content).
func makeBundle(t *testing.T, v string, files map[string]string, extraEntries ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "bundle-"+v+".zip")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	entries := map[string]bool{"go-api": true, "VERSION": true, "bundle-manifest.json": true}
	write := func(name, body string, mode os.FileMode) {
		h := &zip.FileHeader{Name: "ezhealthkonnect/" + name, Method: zip.Deflate}
		h.SetMode(mode)
		w, _ := zw.CreateHeader(h)
		io.WriteString(w, body)
	}
	write("go-api", "binary "+v, 0o755)
	write("VERSION", v, 0o644)
	for name, body := range files {
		write(name, body, 0o644)
		entries[strings.SplitN(name, "/", 2)[0]] = true
	}
	for _, e := range extraEntries {
		entries[e] = true
	}
	var list []string
	for e := range entries {
		list = append(list, e)
	}
	info, _ := json.Marshal(BundleInfo{Version: v, OS: goosName(), Arch: runtime.GOARCH, Entries: sortedCopy(list),
		Protected: []string{".env", "storage", "logs"}})
	write("bundle-manifest.json", string(info), 0o644)
	zw.Close()
	f.Close()
	return p
}

type harness struct {
	dir     string
	plat    *fakePlatform
	pg      *fakePg
	migr    *fakeMigrator
	health  *fakeHealth
	clock   time.Time
	console *strings.Builder
}

func newHarness(t *testing.T) *harness {
	pg := &fakePg{}
	return &harness{dir: filepath.Join(t.TempDir(), "ezhk"), plat: &fakePlatform{elevated: true}, pg: pg,
		migr: &fakeMigrator{pg: pg}, health: &fakeHealth{}, clock: time.Date(2026, 10, 20, 9, 0, 0, 0, time.UTC), console: &strings.Builder{}}
}

func (h *harness) installer(bundle string) *Installer {
	return &Installer{Platform: h.plat, Bundle: FileBundle{File: bundle}, Migrator: h.migr, Health: h.health,
		Rep: ConsoleReporter{Out: h.console}, Pg: func(string) PgOps { return h.pg },
		Now: func() time.Time { h.clock = h.clock.Add(time.Minute); return h.clock }, HealthTimeout: time.Second, KeepBackups: 3}
}

func (h *harness) install(t *testing.T, v string, files map[string]string) *InstallResult {
	t.Helper()
	res, err := h.installer(makeBundle(t, v, files)).Install(Config{InstallDir: h.dir})
	if err != nil {
		t.Fatalf("install %s: %v\n%s", v, err, h.console)
	}
	return res
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read %s: %v", p, err)
	}
	return string(b)
}

func exists(p string) bool { _, err := os.Stat(p); return err == nil }

// ── install ───────────────────────────────────────────────────────────────────

func TestInstall_FreshInstallation(t *testing.T) {
	h := newHarness(t)
	res := h.install(t, "v26.10", map[string]string{"server.js": "web", "public/index.html": "<html>"})

	for _, f := range []string{"go-api", "server.js", "public/index.html", "VERSION", ".env", stateFileName} {
		if !exists(filepath.Join(h.dir, f)) {
			t.Errorf("missing %s after install", f)
		}
	}
	env, _ := ReadEnvFile(filepath.Join(h.dir, ".env"))
	for _, k := range []string{"DB_PASSWORD", "SESSION_SECRET", "JWT_SECRET", credentialKeyEnvVar, "SETUP_TOKEN", "LOCAL_STORAGE_PATH", "GIT_REPOS_BASE_DIR", "XDG_CACHE_HOME"} {
		if !env.Has(k) {
			t.Errorf(".env lacks %s", k)
		}
	}
	if env.Get("NODE_ENV") != "production" || env.Get("SESSION_SECRET") == env.Get("JWT_SECRET") {
		t.Error("secrets must be production and distinct")
	}
	if !strings.Contains(res.SetupURL, "token="+env.Get("SETUP_TOKEN")) {
		t.Errorf("setup URL %q must carry the .env setup token", res.SetupURL)
	}
	if runtime.GOOS != "windows" {
		st, _ := os.Stat(filepath.Join(h.dir, ".env"))
		if st.Mode().Perm() != 0o600 {
			t.Errorf(".env mode %v, want 0600 before the platform adjusts it", st.Mode().Perm())
		}
	}
	state, found, _ := LoadState(h.dir)
	if !found || state.Version != "v26.10" || !state.Service || len(state.AppEntries) == 0 {
		t.Fatalf("state = %+v", state)
	}
	if got := h.plat.callString(); got != "ensure-postgres,prepare,register,install-service,start" {
		t.Errorf("platform calls = %s", got)
	}
	if h.migr.runs != 1 || h.health.checks != 1 {
		t.Errorf("migrate runs %d, health checks %d", h.migr.runs, h.health.checks)
	}
	entries, _ := os.ReadDir(h.dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".staging-") {
			t.Errorf("staging directory %s left behind", e.Name())
		}
	}
}

func TestInstall_Refusals(t *testing.T) {
	h := newHarness(t)
	bundle := makeBundle(t, "v1", nil)

	h.plat.elevated = false
	if _, err := h.installer(bundle).Install(Config{InstallDir: h.dir}); err == nil || !strings.Contains(err.Error(), "administrator") {
		t.Errorf("non-elevated install: %v", err)
	}
	h.plat.elevated = true

	os.MkdirAll(h.dir, 0o755)
	os.WriteFile(filepath.Join(h.dir, "someone-elses-file.txt"), []byte("x"), 0o644)
	if _, err := h.installer(bundle).Install(Config{InstallDir: h.dir}); err == nil || !strings.Contains(err.Error(), "not empty") {
		t.Errorf("non-empty dir: %v", err)
	}
	os.Remove(filepath.Join(h.dir, "someone-elses-file.txt"))

	h.install(t, "v1", nil)
	if _, err := h.installer(bundle).Install(Config{InstallDir: h.dir}); err == nil || !strings.Contains(err.Error(), "already installed") {
		t.Errorf("second install: %v", err)
	}
	if _, err := h.installer(bundle).Install(Config{InstallDir: "relative/path"}); err == nil {
		t.Error("relative install dir must be refused")
	}
}

// ── upgrade ───────────────────────────────────────────────────────────────────

// installV1WithData installs v1 and adds customer data that an upgrade must
// never touch.
func installV1WithData(t *testing.T, h *harness) map[string]string {
	t.Helper()
	h.install(t, "v26.10", map[string]string{"server.js": "web v1", "old-only.js": "removed in v2"})
	h.pg.dbState = "v26.10+customer-rows"
	data := map[string]string{
		"storage/raw/msg-1.hl7":       "MSH|patient data",
		"logs/application/app.log":    "log line",
		"schemas/hl7/v2.5.1/ADT.json": "{}",
		"git-repos/repo/README":       "repo",
		"uploads/file.csv":            "a,b",
	}
	for p, body := range data {
		full := filepath.Join(h.dir, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(full), 0o700)
		os.WriteFile(full, []byte(body), 0o600)
	}
	return data
}

func assertDataIntact(t *testing.T, h *harness, data map[string]string) {
	t.Helper()
	for p, body := range data {
		if got := readFile(t, filepath.Join(h.dir, filepath.FromSlash(p))); got != body {
			t.Errorf("customer data %s changed: %q", p, got)
		}
	}
}

func TestUpgrade_SwapsApplicationKeepsDataAndConfig(t *testing.T) {
	h := newHarness(t)
	data := installV1WithData(t, h)
	envBefore, _ := ReadEnvFile(filepath.Join(h.dir, ".env"))
	// Simulate an older .env that lacks a setting newer releases need.
	envBefore.Set("GIT_REPOS_BASE_DIR", "")
	envBefore.WriteFile(filepath.Join(h.dir, ".env"))
	h.plat.calls = nil

	res, err := h.installer(makeBundle(t, "v26.11", map[string]string{"server.js": "web v2", "new-only.js": "added in v2"})).Upgrade(h.dir)
	if err != nil {
		t.Fatalf("upgrade: %v\n%s", err, h.console)
	}
	if res.Version != "v26.11" {
		t.Errorf("version = %s", res.Version)
	}
	if readFile(t, filepath.Join(h.dir, "server.js")) != "web v2" || !exists(filepath.Join(h.dir, "new-only.js")) {
		t.Error("new application files not installed")
	}
	if exists(filepath.Join(h.dir, "old-only.js")) {
		t.Error("a file removed in the new version must not linger")
	}
	assertDataIntact(t, h, data)

	envAfter, _ := ReadEnvFile(filepath.Join(h.dir, ".env"))
	for _, k := range []string{"DB_PASSWORD", "SESSION_SECRET", "JWT_SECRET", credentialKeyEnvVar, "SETUP_TOKEN"} {
		if envAfter.Get(k) != envBefore.Get(k) {
			t.Errorf("upgrade changed secret %s", k)
		}
	}
	if !envAfter.Has("GIT_REPOS_BASE_DIR") {
		t.Error("upgrade must add settings the new version needs")
	}

	// Backup taken before anything changed, then services stopped/started.
	if got := strings.Join(h.pg.calls, ","); !strings.HasPrefix(got, "dump,verify") {
		t.Errorf("pg calls = %s", got)
	}
	if h.pg.dumped != "v26.10+customer-rows" {
		t.Errorf("backup captured %q", h.pg.dumped)
	}
	if got := h.plat.callString(); got != "stop,prepare,install-service,start,register" {
		t.Errorf("platform calls = %s", got)
	}
	_, bm, err := latestBackup(h.dir)
	if err != nil || bm.FromVersion != "v26.10" || bm.ToVersion != "v26.11" {
		t.Fatalf("backup manifest %+v %v", bm, err)
	}
	backupDir, _, _ := latestBackup(h.dir)
	if readFile(t, filepath.Join(backupDir, "app", "server.js")) != "web v1" || !exists(filepath.Join(backupDir, "app", "old-only.js")) {
		t.Error("previous application files must be in the backup")
	}
	state, _, _ := LoadState(h.dir)
	if state.Version != "v26.11" || len(state.History) != 2 {
		t.Errorf("state = %+v", state)
	}
}

func assertRolledBack(t *testing.T, h *harness, data map[string]string, envBefore string) {
	t.Helper()
	if readFile(t, filepath.Join(h.dir, "server.js")) != "web v1" || !exists(filepath.Join(h.dir, "old-only.js")) {
		t.Error("previous application files were not restored")
	}
	if exists(filepath.Join(h.dir, "new-only.js")) {
		t.Error("files of the failed version must be removed")
	}
	if readFile(t, filepath.Join(h.dir, ".env")) != envBefore {
		t.Error(".env was not restored exactly")
	}
	if h.pg.dbState != "v26.10+customer-rows" {
		t.Errorf("database not restored: %q", h.pg.dbState)
	}
	state, _, _ := LoadState(h.dir)
	if state.Version != "v26.10" {
		t.Errorf("state version %s after rollback", state.Version)
	}
	assertDataIntact(t, h, data)
}

func TestUpgrade_MigrationFailureRestoresEverything(t *testing.T) {
	h := newHarness(t)
	data := installV1WithData(t, h)
	envBefore := readFile(t, filepath.Join(h.dir, ".env"))
	h.migr.fail = true

	_, err := h.installer(makeBundle(t, "v26.11", map[string]string{"server.js": "web v2", "new-only.js": "x"})).Upgrade(h.dir)
	if err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected a rolled-back failure, got %v\n%s", err, h.console)
	}
	assertRolledBack(t, h, data, envBefore)
	if !strings.Contains(strings.Join(h.pg.calls, ","), "restore") {
		t.Error("database must be restored from the backup")
	}
	if !strings.HasSuffix(h.plat.callString(), "start") {
		t.Errorf("the previous version must be started again: %s", h.plat.callString())
	}
}

func TestUpgrade_UnhealthyNewVersionRestoresEverything(t *testing.T) {
	h := newHarness(t)
	data := installV1WithData(t, h)
	envBefore := readFile(t, filepath.Join(h.dir, ".env"))
	h.health.failOnce = true // new version unhealthy; restored one healthy

	_, err := h.installer(makeBundle(t, "v26.11", map[string]string{"server.js": "web v2", "new-only.js": "x"})).Upgrade(h.dir)
	if err == nil || !strings.Contains(err.Error(), "rolled back") {
		t.Fatalf("expected rollback, got %v", err)
	}
	assertRolledBack(t, h, data, envBefore)
}

func TestUpgrade_BackupFailureChangesNothing(t *testing.T) {
	h := newHarness(t)
	data := installV1WithData(t, h)
	envBefore := readFile(t, filepath.Join(h.dir, ".env"))
	h.pg.dumpErr = errors.New("disk full")
	h.plat.calls = nil

	_, err := h.installer(makeBundle(t, "v26.11", map[string]string{"server.js": "web v2"})).Upgrade(h.dir)
	if err == nil || !strings.Contains(err.Error(), "nothing was changed") {
		t.Fatalf("expected an abort, got %v", err)
	}
	if readFile(t, filepath.Join(h.dir, "server.js")) != "web v1" || readFile(t, filepath.Join(h.dir, ".env")) != envBefore {
		t.Error("files changed although the backup failed")
	}
	if h.migr.runs != 1 {
		t.Error("migrations must not run without a backup")
	}
	if got := h.plat.callString(); got != "stop,start" {
		t.Errorf("service must be restarted untouched: %s", got)
	}
	assertDataIntact(t, h, data)
}

func TestUpgrade_RefusesDowngradeBeforeStopping(t *testing.T) {
	h := newHarness(t)
	installV1WithData(t, h)
	h.plat.calls = nil
	_, err := h.installer(makeBundle(t, "v26.09", nil)).Upgrade(h.dir)
	if err == nil || !strings.Contains(err.Error(), "older") {
		t.Fatalf("expected downgrade refusal, got %v", err)
	}
	if h.plat.callString() != "" {
		t.Errorf("nothing may happen before the refusal: %s", h.plat.callString())
	}
}

func TestRollback_ReturnsToPreviousVersion(t *testing.T) {
	h := newHarness(t)
	data := installV1WithData(t, h)
	envBefore := readFile(t, filepath.Join(h.dir, ".env"))
	if _, err := h.installer(makeBundle(t, "v26.11", map[string]string{"server.js": "web v2", "new-only.js": "x"})).Upgrade(h.dir); err != nil {
		t.Fatal(err)
	}
	if err := h.installer(makeBundle(t, "v26.11", nil)).Rollback(h.dir); err != nil {
		t.Fatalf("rollback: %v\n%s", err, h.console)
	}
	assertRolledBack(t, h, data, envBefore)
	if err := h.installer(makeBundle(t, "v26.11", nil)).Rollback(h.dir); err == nil {
		t.Error("a consumed backup must not be restored twice")
	}
}

func TestRollback_AfterAFailedUpgradeTargetsTheLastSuccessfulOne(t *testing.T) {
	h := newHarness(t)
	data := installV1WithData(t, h)
	envBefore := readFile(t, filepath.Join(h.dir, ".env"))
	if _, err := h.installer(makeBundle(t, "v26.11", map[string]string{"server.js": "web v2", "new-only.js": "x"})).Upgrade(h.dir); err != nil {
		t.Fatal(err)
	}
	h.migr.fail = true
	if _, err := h.installer(makeBundle(t, "v26.12", map[string]string{"server.js": "web v3"})).Upgrade(h.dir); err == nil {
		t.Fatal("v26.12 upgrade should fail")
	}
	h.migr.fail = false
	if state, _, _ := LoadState(h.dir); state.Version != "v26.11" {
		t.Fatalf("after the failed upgrade, v26.11 must still be installed (got %s)", state.Version)
	}
	if err := h.installer(makeBundle(t, "v26.12", nil)).Rollback(h.dir); err != nil {
		t.Fatalf("manual rollback after a failed upgrade: %v\n%s", err, h.console)
	}
	assertRolledBack(t, h, data, envBefore)
}

func TestUpgrade_PrunesOldBackups(t *testing.T) {
	h := newHarness(t)
	installV1WithData(t, h)
	for _, v := range []string{"v26.11", "v26.12", "v27.1", "v27.2"} {
		if _, err := h.installer(makeBundle(t, v, map[string]string{"server.js": v})).Upgrade(h.dir); err != nil {
			t.Fatal(err)
		}
	}
	entries, _ := os.ReadDir(filepath.Join(h.dir, "backups"))
	if len(entries) != 3 {
		t.Errorf("kept %d backups, want 3", len(entries))
	}
}

// ── bundle safety ─────────────────────────────────────────────────────────────

func TestStageBundle_RejectsBundleClaimingCustomerData(t *testing.T) {
	for _, bad := range []string{".env", "storage", "logs", "backups", stateFileName} {
		dir := t.TempDir()
		b := makeBundle(t, "v1", map[string]string{bad + "x/placeholder": "x"}, bad)
		if _, err := StageBundle(FileBundle{File: b}, dir, goosName()); err == nil {
			t.Errorf("bundle listing %q must be rejected", bad)
		}
	}
}

func TestMoveEntries_RefusesProtectedNames(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(src, ".env"), []byte("SECRET=1"), 0o600)
	if err := moveEntries(src, dst, []string{".env"}); err == nil {
		t.Fatal("moving .env must be refused")
	}
	if !exists(filepath.Join(src, ".env")) {
		t.Fatal(".env was moved")
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"v26.10", "v26.9", 1}, {"v26.10", "v26.10", 0}, {"v26.10.1", "v26.10", 1},
		{"v27.1", "v26.12", 1}, {"v26.10", "v26.11", -1}, {"26.11-rc1", "v26.11", 0},
	}
	for _, c := range cases {
		if got := compareVersions(c.a, c.b); got != c.want {
			t.Errorf("compareVersions(%s, %s) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
