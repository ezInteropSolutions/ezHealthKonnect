package main

import (
	"strings"
	"sync"
	"testing"
)

// leftoverAdmin plays a PostgreSQL server that still holds the database of an
// earlier installation (the reported "database ezhealthkonnect already exists").
type leftoverAdmin struct {
	mu       sync.Mutex
	sql      []string
	dropped  bool
	failDrop bool
}

func (a *leftoverAdmin) Exec(db, sql string) (string, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.sql = append(a.sql, sql)
	switch {
	case strings.Contains(sql, "pg_database WHERE"):
		if a.dropped {
			return "", nil
		}
		return "1", nil
	case strings.Contains(sql, "pg_catalog.pg_tables"):
		return "12", nil // the old installation's tables
	case strings.HasPrefix(sql, "DROP DATABASE"):
		if a.failDrop {
			return "ERROR: cannot drop the currently open database", errString("exit status 3")
		}
		a.dropped = true
	}
	return "", nil
}

type errString string

func (e errString) Error() string { return string(e) }

func (a *leftoverAdmin) indexOf(fragment string) int {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i, s := range a.sql {
		if strings.Contains(s, fragment) {
			return i
		}
	}
	return -1
}

func TestInstall_RefusesALeftoverDatabaseUnlessAskedToReplaceIt(t *testing.T) {
	h := newHarness(t)
	admin := &leftoverAdmin{}
	h.plat.admin = admin
	_, err := h.installer(makeBundle(t, "v26.10", nil)).Install(Config{InstallDir: h.dir})
	if err == nil || !strings.Contains(err.Error(), existingDataMarker) {
		t.Fatalf("err = %v, want the refusal naming the existing data", err)
	}
	if admin.indexOf("DROP DATABASE IF EXISTS") >= 0 {
		t.Error("a database holding data was dropped without being asked to")
	}
	if admin.indexOf("CREATE ROLE") >= 0 {
		t.Error("setup went on creating things after refusing")
	}
	if !strings.Contains(err.Error(), `"replaceDatabase": true`) || !strings.Contains(err.Error(), "DROP DATABASE ezhealthkonnect;") {
		t.Errorf("the refusal does not say how to start clean: %v", err)
	}
}

// The reported case, solved without psql: delete the old database on request,
// then create a fresh one.
func TestInstall_ReplaceDatabaseDeletesTheOldOneAndInstallsFresh(t *testing.T) {
	h := newHarness(t)
	admin := &leftoverAdmin{}
	h.plat.admin = admin
	if _, err := h.installer(makeBundle(t, "v26.10", nil)).Install(Config{InstallDir: h.dir, ReplaceDatabase: true}); err != nil {
		t.Fatalf("install: %v\n%s", err, h.console)
	}
	drop := admin.indexOf("DROP DATABASE IF EXISTS ezhealthkonnect WITH (FORCE)")
	create := admin.indexOf("CREATE DATABASE ezhealthkonnect")
	if drop < 0 || create < 0 || drop > create {
		t.Errorf("the old database must be dropped, then a new one created: drop at %d, create at %d; ran %v", drop, create, admin.sql)
	}
	if !strings.Contains(h.console.String(), "Deleting the existing database ezhealthkonnect") {
		t.Errorf("the log does not say the old database was deleted:\n%s", h.console)
	}
	if st, found, _ := LoadState(h.dir); !found || !st.DBCreated {
		t.Errorf("the new database is not recorded as created by setup: %+v", st)
	}
}

func TestInstall_ReplaceDatabaseStopsIfTheOldOneCannotBeDeleted(t *testing.T) {
	h := newHarness(t)
	h.plat.admin = &leftoverAdmin{failDrop: true}
	_, err := h.installer(makeBundle(t, "v26.10", nil)).Install(Config{InstallDir: h.dir, ReplaceDatabase: true})
	if err == nil || !strings.Contains(err.Error(), "could not delete the old database") {
		t.Fatalf("err = %v, want the failure to delete it", err)
	}
}

// Replacing needs administrator access; a database the customer provided is
// never deleted by setup.
func TestConfig_ReplaceDatabaseNeedsAdministratorAccess(t *testing.T) {
	base := Config{InstallDir: t.TempDir() + "/x", ReplaceDatabase: true}

	own := base
	own.DBMode, own.DBHost, own.DBPassword = "external", "db.example", "app-pass"
	own.Normalize()
	if err := own.Validate(); err == nil || !strings.Contains(err.Error(), "administrator") {
		t.Errorf("replace for a database you provided: err = %v, want a refusal", err)
	}

	admin := base
	admin.DBMode, admin.DBHost, admin.DBAdminPassword = "external", "db.example", "adminpw"
	admin.Normalize()
	if err := admin.Validate(); err != nil {
		t.Errorf("replace with an administrator login: %v", err)
	}

	local := base
	local.Normalize()
	if err := local.Validate(); err != nil {
		t.Errorf("replace for PostgreSQL on this machine: %v", err)
	}
}

// The setup window finds the refusal by its text, so the two must agree.
func TestWizardKnowsTheRefusalMarker(t *testing.T) {
	js, err := webFS.ReadFile("web/app.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(js), "const EXISTING_DATA_MARKER = '"+existingDataMarker+"'") {
		t.Errorf("web/app.js does not look for %q: the delete-old-database button would never appear", existingDataMarker)
	}
}
