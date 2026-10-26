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
	// The database is now non-empty, so a second install must refuse to reuse it.
	in := &Installer{Rep: ConsoleReporter{Out: &strings.Builder{}}}
	if err := in.refuseExistingData(admin, Config{DBName: db}); err == nil || !strings.Contains(err.Error(), "already exists and contains data") {
		t.Errorf("existing data was not protected: %v", err)
	}
}
