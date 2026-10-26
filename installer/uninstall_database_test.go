package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
)

// Reported: after "Existing database" + an administrator login, uninstall left
// the database behind and the next install said "database already exists".
// These cover who created the database and how uninstall may remove it.

// externalInstall installs with an administrator login and returns the
// installer; uninstallAdmin receives the connection uninstall opens.
func externalInstall(t *testing.T, h *harness) (in *Installer, uninstallAdmin *recAdmin, seen *[]Config) {
	t.Helper()
	provision, drop := &recAdmin{}, &recAdmin{}
	var cfgs []Config
	in = h.installer(makeBundle(t, "v26.10", nil))
	in.NewAdmin = func(_ string, cfg Config) AdminSQL {
		cfgs = append(cfgs, cfg)
		if cfg.DBAdminPassword == "adminpw" {
			return provision
		}
		return drop
	}
	if _, err := in.Install(Config{InstallDir: h.dir, DBMode: "external", DBHost: "db.example", DBSSLMode: "disable", DBAdminPassword: "adminpw"}); err != nil {
		t.Fatalf("install: %v\n%s", err, h.console)
	}
	return in, drop, &cfgs
}

func TestUninstall_DropsTheDatabaseSetupCreatedOnAnExistingServer(t *testing.T) {
	h := newHarness(t)
	in, drop, seen := externalInstall(t, h)

	st, _, _ := LoadState(h.dir)
	if !st.DBCreated || !st.OwnsDatabase() {
		t.Fatalf("setup created the database but did not record it: %+v", st)
	}
	env, _ := ReadEnvFile(filepath.Join(h.dir, ".env"))
	appPassword := env.Get("DB_PASSWORD")

	if err := in.Uninstall(h.dir, false); err != nil {
		t.Fatalf("uninstall: %v\n%s", err, h.console)
	}
	if !drop.saw("DROP DATABASE IF EXISTS ezhealthkonnect WITH (FORCE)") {
		t.Errorf("the database was not dropped; ran: %v", drop.sql)
	}
	// It signs in as the application user (the owner) — no administrator password is needed.
	last := (*seen)[len(*seen)-1]
	if last.DBAdminUser != "ezhealth_user" || last.DBAdminPassword != appPassword || last.DBHost != "db.example" || last.DBAdminPassword == "adminpw" {
		t.Errorf("uninstall connected as %q to %q (password reused from the install: %v)", last.DBAdminUser, last.DBHost, last.DBAdminPassword == "adminpw")
	}
	if !strings.Contains(h.console.String(), "Database ezhealthkonnect removed") {
		t.Errorf("uninstall did not say the database was removed:\n%s", h.console)
	}
	if _, err := os.Stat(h.dir); !os.IsNotExist(err) {
		t.Errorf("install folder still there (stat err = %v)", err)
	}
}

func TestUninstall_KeepDatabaseKeepsIt(t *testing.T) {
	h := newHarness(t)
	in, drop, _ := externalInstall(t, h)
	if err := in.Uninstall(h.dir, true); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if drop.saw("DROP DATABASE") {
		t.Errorf("--keep-database still dropped it: %v", drop.sql)
	}
	if !strings.Contains(h.console.String(), "Keeping database") {
		t.Errorf("uninstall did not say it kept the database:\n%s", h.console)
	}
}

// A database the customer provided is theirs: never dropped.
func TestUninstall_NeverDropsADatabaseYouProvided(t *testing.T) {
	h := newHarness(t)
	in := h.installer(makeBundle(t, "v26.10", nil))
	in.NewAdmin = func(string, Config) AdminSQL {
		t.Error("an administrator or owner connection was opened for a database setup did not create")
		return &recAdmin{}
	}
	if _, err := in.Install(Config{InstallDir: h.dir, DBMode: "external", DBHost: "db.example", DBSSLMode: "disable", DBPassword: "app-pass"}); err != nil {
		t.Fatalf("install: %v", err)
	}
	if st, _, _ := LoadState(h.dir); st.DBCreated || st.OwnsDatabase() {
		t.Fatalf("a provided database was recorded as created by setup: %+v", st)
	}
	if err := in.Uninstall(h.dir, false); err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if !strings.Contains(h.console.String(), "provided by you") {
		t.Errorf("uninstall did not say the database was left in place:\n%s", h.console)
	}
}

// PostgreSQL that was already installed: setup typed no stored password, so it
// has no administrator access — it must still remove the database it created.
func TestUninstall_WithoutAdministratorAccessDropsAsTheOwner(t *testing.T) {
	h := newHarness(t)
	h.plat.noAdmin = true
	drop := &recAdmin{}
	var as Config
	in := h.installer(makeBundle(t, "v26.10", nil))
	in.NewAdmin = func(_ string, cfg Config) AdminSQL { as = cfg; return drop }
	if _, err := in.Install(Config{InstallDir: h.dir}); err != nil {
		t.Fatalf("install: %v\n%s", err, h.console)
	}
	if err := in.Uninstall(h.dir, false); err != nil {
		t.Fatalf("uninstall: %v\n%s", err, h.console)
	}
	if !drop.saw("DROP DATABASE IF EXISTS ezhealthkonnect") || as.DBAdminUser != "ezhealth_user" {
		t.Errorf("owner removal: user=%q ran=%v", as.DBAdminUser, drop.sql)
	}
	if strings.Contains(h.console.String(), "left in place") {
		t.Errorf("the database was reported as left in place:\n%s", h.console)
	}
}

// Failing to drop must not block the uninstall, and must say how to finish.
func TestUninstall_FailedDropStillUninstallsAndExplains(t *testing.T) {
	h := newHarness(t)
	in, drop, _ := externalInstall(t, h)
	drop.failOn = "DROP DATABASE"
	if err := in.Uninstall(h.dir, false); err != nil {
		t.Fatalf("uninstall must still finish: %v", err)
	}
	if !strings.Contains(h.console.String(), "Remove it yourself with: DROP DATABASE ezhealthkonnect;") {
		t.Errorf("no instruction for removing the database by hand:\n%s", h.console)
	}
	if _, err := os.Stat(h.dir); !os.IsNotExist(err) {
		t.Error("the install folder was not removed")
	}
}

func TestInstallState_OwnsDatabaseAcrossVersions(t *testing.T) {
	cases := map[string]bool{
		`{"dbMode":"local"}`:                      true,  // recorded before dbCreated existed
		`{"dbMode":"external"}`:                   false, // recorded before dbCreated existed: provided by the customer
		`{"dbMode":"external","dbCreated":true}`:  true,
		`{"dbMode":"external","dbCreated":false}`: false,
		`{"dbMode":"local","dbCreated":true}`:     true,
	}
	for in, want := range cases {
		var s InstallState
		if err := json.Unmarshal([]byte(in), &s); err != nil {
			t.Fatal(err)
		}
		if got := s.OwnsDatabase(); got != want {
			t.Errorf("%s: OwnsDatabase = %v, want %v", in, got, want)
		}
	}
}

// The wizard's confirmation text depends on this.
func TestUIServer_StateSaysWhetherUninstallRemovesTheDatabase(t *testing.T) {
	web := fstest.MapFS{"index.html": {Data: []byte("<html>")}}
	state := func(dir string) StateResponse {
		s := NewUIServer(&fakePlatform{elevated: true}, nil, web, 41234)
		s.Locate = func() []string { return []string{dir} }
		req := httptest.NewRequest(http.MethodGet, "/api/state", nil)
		req.Host = "127.0.0.1:41234"
		req.Header.Set("X-Installer-Token", s.Token)
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, req)
		var st StateResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil {
			t.Fatal(err)
		}
		return st
	}
	provided := installed(t, filepath.Join(t.TempDir(), "provided")) // external, not created by setup
	if state(provided).RemovesDatabase {
		t.Error("the wizard would promise to drop a database the customer provided")
	}
	created := filepath.Join(t.TempDir(), "created")
	_ = os.MkdirAll(created, 0o755)
	if err := (&InstallState{Version: "v26.10", DBMode: "external", DBCreated: true}).Save(created); err != nil {
		t.Fatal(err)
	}
	if !state(created).RemovesDatabase {
		t.Error("the wizard does not say uninstall removes the database setup created")
	}
}
