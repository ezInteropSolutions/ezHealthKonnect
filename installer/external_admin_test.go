package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// recAdmin records the SQL it is given; failOn makes matching statements fail.
type recAdmin struct {
	mu     sync.Mutex
	sql    []string
	failOn string
}

func (a *recAdmin) Exec(_ string, sql string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sql = append(a.sql, sql)
	if a.failOn != "" && strings.Contains(sql, a.failOn) {
		return "ERROR: must be owner of schema public", errors.New("exit status 3")
	}
	return "", nil
}

func (a *recAdmin) saw(fragment string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, s := range a.sql {
		if strings.Contains(s, fragment) {
			return true
		}
	}
	return false
}

func TestConfig_ExternalAcceptsAnAdministratorInsteadOfAnExistingUser(t *testing.T) {
	c := Config{InstallDir: filepath.Join(t.TempDir(), "x"), DBMode: "external", DBHost: "db.example", DBAdminPassword: "s3cret"}
	c.Normalize()
	if c.DBAdminUser != "postgres" {
		t.Errorf("administrator user defaults to %q, want postgres", c.DBAdminUser)
	}
	if err := c.Validate(); err != nil {
		t.Errorf("an administrator login alone must be enough: %v", err)
	}

	none := Config{InstallDir: c.InstallDir, DBMode: "external", DBHost: "db.example"}
	none.Normalize()
	if err := none.Validate(); err == nil || !strings.Contains(err.Error(), "administrator password") {
		t.Errorf("neither login given: err = %v, want a message naming both choices", err)
	}

	own := Config{InstallDir: c.InstallDir, DBMode: "external", DBHost: "db.example", DBPassword: "app-pass"}
	own.Normalize()
	if err := own.Validate(); err != nil {
		t.Errorf("an existing user's password still works: %v", err)
	}
	if own.DBAdminUser != "" {
		t.Errorf("no administrator was given, yet the user is %q", own.DBAdminUser)
	}

	multi := c
	multi.DBAdminPassword = "line1\nline2"
	if err := multi.Validate(); err == nil {
		t.Error("a multi-line password must be refused")
	}
}

func TestPsqlAdmin_PasswordStaysOutOfTheCommandLine(t *testing.T) {
	a := psqlAdmin{Psql: "/pg/bin/psql", Host: "db.example", Port: "5433", User: "dbadmin", Password: "s3cret", SSLMode: "require"}
	args := strings.Join(a.args("postgres", "SELECT 1"), " ")
	for _, want := range []string{"-h db.example", "-p 5433", "-U dbadmin", "-d postgres", "ON_ERROR_STOP=1", "-c SELECT 1"} {
		if !strings.Contains(args, want) {
			t.Errorf("psql arguments %q lack %q", args, want)
		}
	}
	if strings.Contains(args, "s3cret") {
		t.Error("the password is on the command line (visible in the process list)")
	}
	env := strings.Join(a.env(), "\n")
	if !strings.Contains(env, "PGPASSWORD=s3cret") || !strings.Contains(env, "PGSSLMODE=require") {
		t.Errorf("environment lacks the password or TLS mode: %q", env)
	}
}

// The reported case: an existing server, an administrator login, and no
// application user yet. Setup must create the user and the database.
func TestInstall_ExternalAdministratorCreatesTheDatabaseAndUser(t *testing.T) {
	h := newHarness(t)
	admin := &recAdmin{}
	in := h.installer(makeBundle(t, "v26.10", nil))
	in.NewAdmin = func(binDir string, cfg Config) AdminSQL {
		if cfg.DBAdminUser != "postgres" || cfg.DBAdminPassword != "adminpw" || cfg.DBHost != "db.example" {
			t.Errorf("administrator connection got %+v", cfg)
		}
		return admin
	}
	_, err := in.Install(Config{InstallDir: h.dir, DBMode: "external", DBHost: "db.example", DBSSLMode: "disable", DBAdminPassword: "adminpw"})
	if err != nil {
		t.Fatalf("install: %v\n%s", err, h.console)
	}
	for _, want := range []string{"CREATE ROLE ezhealth_user", "CREATE DATABASE ezhealthkonnect", "OWNER ezhealth_user"} {
		if !admin.saw(want) {
			t.Errorf("the administrator never ran %q; ran: %v", want, admin.sql)
		}
	}
	env := readFile(t, filepath.Join(h.dir, ".env"))
	if strings.Contains(env, "adminpw") {
		t.Error("the administrator password was written to .env")
	}
	if !strings.Contains(env, "DB_USER=ezhealth_user") || !strings.Contains(env, "DB_HOST=db.example") {
		t.Errorf(".env does not point at the new user on that server:\n%s", env)
	}
	if st, found, _ := LoadState(h.dir); !found || st.DBMode != "external" {
		t.Errorf("install record: found=%v state=%+v", found, st)
	}
}

func TestInstall_ExternalWithAnExistingUserNeverUsesAnAdministrator(t *testing.T) {
	h := newHarness(t)
	in := h.installer(makeBundle(t, "v26.10", nil))
	in.NewAdmin = func(string, Config) AdminSQL {
		t.Error("an administrator connection was opened although none was given")
		return &recAdmin{}
	}
	if _, err := in.Install(Config{InstallDir: h.dir, DBMode: "external", DBHost: "db.example", DBSSLMode: "disable", DBPassword: "app-pass"}); err != nil {
		t.Fatalf("install: %v\n%s", err, h.console)
	}
	if env := readFile(t, filepath.Join(h.dir, ".env")); !strings.Contains(env, "app-pass") {
		t.Errorf("the existing user's password was not used:\n%s", env)
	}
}

// Failing to sign in as the administrator must stop before anything is created.
func TestInstall_ExternalAdministratorCannotSignIn(t *testing.T) {
	h := newHarness(t)
	admin := &recAdmin{failOn: "SELECT 1"}
	in := h.installer(makeBundle(t, "v26.10", nil))
	in.NewAdmin = func(string, Config) AdminSQL { return admin }
	_, err := in.Install(Config{InstallDir: h.dir, DBMode: "external", DBHost: "db.example", DBSSLMode: "disable", DBAdminPassword: "wrong"})
	if err == nil || !strings.Contains(err.Error(), "cannot sign in") {
		t.Fatalf("err = %v, want a sign-in failure", err)
	}
	if admin.saw("CREATE ROLE") {
		t.Error("a role was created although the administrator could not sign in")
	}
}

// failingPing makes the application user's connection fail like a missing role.
type failingPing struct{ *fakePg }

func (failingPing) Ping(PgConn) error {
	return errors.New(`FATAL: role "ezhealth_user" does not exist`)
}

func TestInstall_MissingUserSuggestsGivingAnAdministrator(t *testing.T) {
	h := newHarness(t)
	in := h.installer(makeBundle(t, "v26.10", nil))
	in.Pg = func(string) PgOps { return failingPing{h.pg} }
	_, err := in.Install(Config{InstallDir: h.dir, DBMode: "external", DBHost: "db.example", DBSSLMode: "disable", DBPassword: "pw"})
	if err == nil || !strings.Contains(err.Error(), "enter an administrator") {
		t.Fatalf("err = %v, want advice to enter an administrator login", err)
	}
	if _, statErr := os.Stat(filepath.Join(h.dir, ".env")); statErr == nil {
		t.Error("files were written although the database could not be reached")
	}
}

func TestEnsureRoleAndDatabase_ManagedServerFallbacks(t *testing.T) {
	a := &recAdmin{failOn: "ALTER SCHEMA"}
	if err := EnsureRoleAndDatabase(a, "ezhealthkonnect", "ezhealth_user", "pw"); err != nil {
		t.Fatalf("a refused schema-owner change must fall back to a grant, got %v", err)
	}
	if !a.saw("GRANT ALL ON SCHEMA public TO ezhealth_user") {
		t.Errorf("no grant fallback; ran: %v", a.sql)
	}
	if !a.saw("GRANT ezhealth_user TO CURRENT_USER") {
		t.Errorf("a non-superuser administrator was never made a member of the new role; ran: %v", a.sql)
	}

	both := &recAdmin{failOn: "SCHEMA public"}
	if err := EnsureRoleAndDatabase(both, "ezhealthkonnect", "ezhealth_user", "pw"); err == nil {
		t.Error("when neither the owner change nor the grant works, the install must stop")
	}
}
