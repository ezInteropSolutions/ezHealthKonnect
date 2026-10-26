package main

import (
	"os"
	"strings"
	"testing"
)

// Runs psqlAdmin and EnsureRoleAndDatabase against a REAL PostgreSQL server.
// Skipped unless EZHK_TEST_PG_HOST is set, for example (a throwaway server):
//
//	EZHK_TEST_PG_HOST=localhost EZHK_TEST_PG_PORT=5432 EZHK_TEST_PG_ADMIN=postgres \
//	EZHK_TEST_PG_ADMIN_PASSWORD=... EZHK_TEST_PSQL_DIR=/usr/bin go test -run RealServer ./...
//
// It creates, uses and removes the database ezhk_it_db and the role ezhk_it_user.
func TestPsqlAdmin_RealServer_CreatesTheDatabaseAndUser(t *testing.T) {
	host := os.Getenv("EZHK_TEST_PG_HOST")
	if host == "" {
		t.Skip("EZHK_TEST_PG_HOST not set: no real PostgreSQL server to test against")
	}
	port := os.Getenv("EZHK_TEST_PG_PORT")
	if port == "" {
		port = "5432"
	}
	admin := psqlAdmin{
		Psql: psqlPath(os.Getenv("EZHK_TEST_PSQL_DIR")), Host: host, Port: port,
		User: os.Getenv("EZHK_TEST_PG_ADMIN"), Password: os.Getenv("EZHK_TEST_PG_ADMIN_PASSWORD"), SSLMode: "disable",
	}
	const db, user, pw = "ezhk_it_db", "ezhk_it_user", "it-generated-pw"
	t.Cleanup(func() { _ = DropRoleAndDatabase(admin, db, user) })
	_ = DropRoleAndDatabase(admin, db, user) // a previous aborted run

	if out, err := admin.Exec("postgres", "SELECT 1"); err != nil || out != "1" {
		t.Fatalf("administrator sign-in: out=%q err=%v", out, err)
	}
	if err := EnsureRoleAndDatabase(admin, db, user, pw); err != nil {
		t.Fatalf("EnsureRoleAndDatabase: %v", err)
	}
	// Running it again (the role and database now exist) must still succeed.
	if err := EnsureRoleAndDatabase(admin, db, user, pw); err != nil {
		t.Fatalf("second EnsureRoleAndDatabase: %v", err)
	}

	// The new user signs in with the generated password and can create objects.
	app := psqlAdmin{Psql: admin.Psql, Host: host, Port: port, User: user, Password: pw, SSLMode: "disable"}
	if out, err := app.Exec(db, "CREATE TABLE it_probe (id int PRIMARY KEY); INSERT INTO it_probe VALUES (1); SELECT count(*) FROM it_probe"); err != nil || !strings.HasSuffix(out, "1") {
		t.Fatalf("application user could not use its database: out=%q err=%v", out, err)
	}
	// A wrong password is refused.
	bad := app
	bad.Password = "wrong"
	if _, err := bad.Exec(db, "SELECT 1"); err == nil {
		t.Error("the application user signed in with a wrong password")
	}
	// Uninstall drops the database as its owner, with no administrator password:
	// first prove the existing-data protection, then the removal.
	// The database is now non-empty, so a second install must refuse to reuse it.
	in := &Installer{Rep: ConsoleReporter{Out: &strings.Builder{}}}
	if err := in.refuseExistingData(admin, Config{DBName: db}); err == nil || !strings.Contains(err.Error(), "already exists and contains data") {
		t.Errorf("existing data was not protected: %v", err)
	}

	// The owner (the application user, connected to "postgres") drops its own
	// database while a connection to it is still open; the role stays.
	if err := DropDatabaseAsOwner(app, db); err != nil {
		t.Fatalf("owner could not drop its database: %v", err)
	}
	if out, err := admin.Exec("postgres", "SELECT 1 FROM pg_database WHERE datname = 'ezhk_it_db'"); err != nil || out != "" {
		t.Errorf("the database still exists after the owner dropped it: out=%q err=%v", out, err)
	}
	if out, err := admin.Exec("postgres", "SELECT 1 FROM pg_roles WHERE rolname = 'ezhk_it_user'"); err != nil || out != "1" {
		t.Errorf("the role should remain (a role cannot drop itself): out=%q err=%v", out, err)
	}
	// A reinstall on top of that must work: role exists, database does not.
	if err := EnsureRoleAndDatabase(admin, db, user, "second-pw"); err != nil {
		t.Errorf("reinstall after the owner's drop: %v", err)
	}

	// An old database full of data is refused, and replaced only when asked.
	app2 := psqlAdmin{Psql: admin.Psql, Host: host, Port: port, User: user, Password: "second-pw", SSLMode: "disable"}
	if _, err := app2.Exec(db, "CREATE TABLE old_data (id int); INSERT INTO old_data VALUES (1)"); err != nil {
		t.Fatalf("filling the old database: %v", err)
	}
	if err := in.refuseExistingData(admin, Config{DBName: db}); err == nil {
		t.Fatal("a database with data was accepted without being asked to replace it")
	}
	if out, _ := app2.Exec(db, "SELECT count(*) FROM old_data"); out != "1" {
		t.Errorf("the refused install touched the data: %q", out)
	}
	if err := in.refuseExistingData(admin, Config{DBName: db, ReplaceDatabase: true}); err != nil {
		t.Fatalf("replacing the old database: %v", err)
	}
	if out, err := admin.Exec("postgres", "SELECT 1 FROM pg_database WHERE datname = 'ezhk_it_db'"); err != nil || out != "" {
		t.Errorf("the old database survived the replacement: out=%q err=%v", out, err)
	}
	if err := EnsureRoleAndDatabase(admin, db, user, "third-pw"); err != nil {
		t.Errorf("creating the fresh database after the replacement: %v", err)
	}
}
