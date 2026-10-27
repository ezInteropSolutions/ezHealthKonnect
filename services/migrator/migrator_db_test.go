package migrator

import (
	"context"
	"database/sql"
	"fmt"
	"math/rand"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	_ "github.com/lib/pq"
)

type testLogger struct{ t *testing.T }

func (l testLogger) Printf(format string, args ...any) { l.t.Logf(format, args...) }

// newTestSchema opens DATABASE_URL with search_path set to a fresh schema that
// is dropped afterwards, so tests never touch real tables.
func newTestSchema(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping real-database migrator test")
	}
	admin, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("migtest_%d_%d", time.Now().UnixNano(), rand.Intn(1e6))
	if _, err := admin.Exec(`CREATE SCHEMA ` + schema); err != nil {
		t.Fatalf("create schema: %v", err)
	}
	t.Cleanup(func() {
		admin.Exec(`DROP SCHEMA ` + schema + ` CASCADE`) //nolint:errcheck
		admin.Close()
	})
	sep := " "
	if strings.Contains(dsn, "://") {
		sep = "&"
		if !strings.Contains(dsn, "?") {
			sep = "?"
		}
	}
	param := "search_path=" + schema
	if sep == " " {
		param = "search_path=" + schema
	}
	db, err := sql.Open("postgres", dsn+sep+param)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func migrate(t *testing.T, db *sql.DB, fsys fstest.MapFS, opts Options) (*Result, error) {
	t.Helper()
	m := New(db, FSSource{FS: fsys, Dir: "m"}, PostgresHistory{}, testLogger{t})
	return m.Migrate(context.Background(), opts)
}

func tableExists(t *testing.T, db *sql.DB, name string) bool {
	t.Helper()
	var n int
	if err := db.QueryRow(`SELECT count(*) FROM pg_tables WHERE schemaname = current_schema() AND tablename = $1`, name).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1
}

func historyRows(t *testing.T, db *sql.DB) []string {
	t.Helper()
	rows, err := db.Query(`SELECT installed_rank, COALESCE(version,''), type, script, COALESCE(checksum,0), success FROM ` + HistoryTable + ` ORDER BY installed_rank`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var rank, sum int
		var ver, typ, script string
		var ok bool
		if err := rows.Scan(&rank, &ver, &typ, &script, &sum, &ok); err != nil {
			t.Fatal(err)
		}
		out = append(out, fmt.Sprintf("%d|%s|%s|%s|%d|%v", rank, ver, typ, script, sum, ok))
	}
	return out
}

func checksumOf(s string) int32 {
	c, _ := Checksum(strings.NewReader(s))
	return c
}

func TestMigrate_FreshThenUpToDate(t *testing.T) {
	db := newTestSchema(t)
	fsys := fstest.MapFS{
		"m/V1__Create_items.sql":     {Data: []byte("CREATE TABLE items (id int primary key);")},
		"m/V2__Seed.sql":             {Data: []byte("INSERT INTO items VALUES (1); INSERT INTO items VALUES (2);")},
		"m/V3__Own_tx.sql":           {Data: []byte("BEGIN;\nCREATE TABLE notes (id int);\nCOMMIT;\n")},
		"m/V4__Concurrent_index.sql": {Data: []byte("CREATE INDEX CONCURRENTLY items_id_idx ON items (id);")},
	}
	res, err := migrate(t, db, fsys, Options{})
	if err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if len(res.Executed) != 4 || !tableExists(t, db, "items") || !tableExists(t, db, "notes") {
		t.Fatalf("expected 4 migrations applied, got %d", len(res.Executed))
	}
	want := []string{
		fmt.Sprintf("1|1|SQL|V1__Create_items.sql|%d|true", checksumOf("CREATE TABLE items (id int primary key);")),
		fmt.Sprintf("2|2|SQL|V2__Seed.sql|%d|true", checksumOf("INSERT INTO items VALUES (1); INSERT INTO items VALUES (2);")),
		fmt.Sprintf("3|3|SQL|V3__Own_tx.sql|%d|true", checksumOf("BEGIN;\nCREATE TABLE notes (id int);\nCOMMIT;\n")),
		fmt.Sprintf("4|4|SQL|V4__Concurrent_index.sql|%d|true", checksumOf("CREATE INDEX CONCURRENTLY items_id_idx ON items (id);")),
	}
	if got := historyRows(t, db); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("history:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	res, err = migrate(t, db, fsys, Options{})
	if err != nil || len(res.Executed) != 0 || len(res.Pending) != 0 {
		t.Fatalf("second run should do nothing: executed=%d pending=%d err=%v", len(res.Executed), len(res.Pending), err)
	}
}

func TestMigrate_FailureLeavesNoTraceAndStops(t *testing.T) {
	db := newTestSchema(t)
	fsys := fstest.MapFS{
		"m/V1__Ok.sql":     {Data: []byte("CREATE TABLE a (id int);")},
		"m/V2__Broken.sql": {Data: []byte("CREATE TABLE b (id int);\nINSERT INTO b VALUES (1);\nINSERT INTO missing_table VALUES (1);\n")},
		"m/V3__After.sql":  {Data: []byte("CREATE TABLE c (id int);")},
	}
	_, err := migrate(t, db, fsys, Options{})
	if err == nil {
		t.Fatal("expected V2 to fail")
	}
	if !strings.Contains(err.Error(), "V2") || !strings.Contains(err.Error(), "line 3") {
		t.Errorf("error should name the migration and line: %v", err)
	}
	if !tableExists(t, db, "a") || tableExists(t, db, "b") || tableExists(t, db, "c") {
		t.Fatal("V1 must stay applied, V2 must be fully rolled back, V3 must not run")
	}
	if rows := historyRows(t, db); len(rows) != 1 {
		t.Fatalf("only V1 should be recorded, got %v", rows)
	}

	// Fix the file and rerun: continues from V2.
	fsys["m/V2__Broken.sql"] = &fstest.MapFile{Data: []byte("CREATE TABLE b (id int);")}
	res, err := migrate(t, db, fsys, Options{})
	if err != nil || len(res.Executed) != 2 || !tableExists(t, db, "c") {
		t.Fatalf("rerun after fix: executed=%v err=%v", res, err)
	}
}

func TestMigrate_SelfManagedFailureDoesNotPoisonTheConnection(t *testing.T) {
	db := newTestSchema(t)
	db.SetMaxOpenConns(2) // the lock holds one; force reuse of the single worker connection
	fsys := fstest.MapFS{
		"m/V1__Bad_tx.sql": {Data: []byte("BEGIN;\nCREATE TABLE x (id int);\nSELECT 1/0;\nCOMMIT;\n")},
	}
	if _, err := migrate(t, db, fsys, Options{}); err == nil {
		t.Fatal("expected failure")
	}
	if tableExists(t, db, "x") {
		t.Fatal("the failed transaction must be rolled back")
	}
	var one int
	if err := db.QueryRow("SELECT 1").Scan(&one); err != nil {
		t.Fatalf("connection left in an aborted transaction: %v", err)
	}
}

func TestMigrate_RefusesUnknownDatabaseUnlessBaselined(t *testing.T) {
	db := newTestSchema(t)
	if _, err := db.Exec("CREATE TABLE existing_data (id int)"); err != nil {
		t.Fatal(err)
	}
	fsys := fstest.MapFS{
		"m/V1__Old.sql": {Data: []byte("CREATE TABLE existing_data (id int);")},
		"m/V2__New.sql": {Data: []byte("CREATE TABLE added (id int);")},
	}
	if _, err := migrate(t, db, fsys, Options{}); err == nil || !strings.Contains(err.Error(), "no migration history") {
		t.Fatalf("expected a refusal, got %v", err)
	}
	if tableExists(t, db, HistoryTable) {
		t.Fatal("a refused run must not create the history table")
	}
	res, err := migrate(t, db, fsys, Options{BaselineVersion: "1"})
	if err != nil || len(res.Executed) != 1 || res.Executed[0].Script != "V2__New.sql" {
		t.Fatalf("baseline at 1 should apply only V2: %+v %v", res, err)
	}
	if rows := historyRows(t, db); len(rows) != 2 || !strings.Contains(rows[0], "|1|BASELINE|") {
		t.Fatalf("history: %v", rows)
	}
}

func TestStatus_FlywayDeleteRowsModifiedAndMissingFiles(t *testing.T) {
	db := newTestSchema(t)
	fsys := fstest.MapFS{
		"m/V1__A.sql": {Data: []byte("CREATE TABLE a (id int);")},
		"m/V2__B.sql": {Data: []byte("CREATE TABLE b (id int);")},
		"m/V3__C.sql": {Data: []byte("CREATE TABLE c (id int);")},
	}
	if _, err := migrate(t, db, fsys, Options{}); err != nil {
		t.Fatal(err)
	}
	// Flyway `repair` marks a removed file with a DELETE row; V2's file then reappears.
	if _, err := db.Exec(`INSERT INTO ` + HistoryTable + ` (installed_rank, version, description, type, script, checksum, installed_by, execution_time, success)
		VALUES (4, '2', 'B', 'DELETE', 'V2__B.sql', NULL, current_user, 0, true)`); err != nil {
		t.Fatal(err)
	}
	fsys["m/V1__A.sql"] = &fstest.MapFile{Data: []byte("CREATE TABLE a (id int); -- edited after it ran")}
	delete(fsys, "m/V3__C.sql")
	fsys["m/V2__B.sql"] = &fstest.MapFile{Data: []byte("CREATE TABLE IF NOT EXISTS b (id int);")}

	st, err := New(db, FSSource{FS: fsys, Dir: "m"}, PostgresHistory{}, testLogger{t}).Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Modified) != 1 || st.Modified[0].Script != "V1__A.sql" {
		t.Errorf("modified = %+v", st.Modified)
	}
	if len(st.Missing) != 1 || st.Missing[0].Script != "V3__C.sql" {
		t.Errorf("missing = %+v", st.Missing)
	}
	if len(st.Pending) != 1 || st.Pending[0].Script != "V2__B.sql" || len(st.OutOfOrder) != 1 {
		t.Errorf("pending = %+v, outOfOrder = %+v", st.Pending, st.OutOfOrder)
	}
	// Applying never re-runs the edited V1; it applies the re-added V2 out of order.
	res, err := migrate(t, db, fsys, Options{})
	if err != nil || len(res.Executed) != 1 {
		t.Fatalf("executed %+v, err %v", res, err)
	}
}

func TestMigrate_RejectsSingleConnectionPool(t *testing.T) {
	db := newTestSchema(t)
	db.SetMaxOpenConns(1)
	_, err := migrate(t, db, fstest.MapFS{"m/V1__A.sql": {Data: []byte("SELECT 1;")}}, Options{})
	if err == nil || !strings.Contains(err.Error(), "at least 2") {
		t.Fatalf("expected a clear error instead of a deadlock, got %v", err)
	}
}

func TestMigrate_DryRunChangesNothing(t *testing.T) {
	db := newTestSchema(t)
	fsys := fstest.MapFS{"m/V1__A.sql": {Data: []byte("CREATE TABLE a (id int);")}}
	res, err := migrate(t, db, fsys, Options{DryRun: true})
	if err != nil || len(res.Pending) != 1 || len(res.Executed) != 0 {
		t.Fatalf("dry run: %+v %v", res, err)
	}
	if tableExists(t, db, "a") || tableExists(t, db, HistoryTable) {
		t.Fatal("dry run must not change the database")
	}
}

func TestMigrate_WaitsForAnotherMigratorThenTimesOut(t *testing.T) {
	db := newTestSchema(t)
	holder, err := db.Conn(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	if _, err := holder.ExecContext(context.Background(), `SELECT pg_advisory_lock($1)`, lockKey); err != nil {
		t.Fatal(err)
	}
	defer holder.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, lockKey) //nolint:errcheck

	fsys := fstest.MapFS{"m/V1__A.sql": {Data: []byte("CREATE TABLE a (id int);")}}
	_, err = migrate(t, db, fsys, Options{LockTimeout: 3 * time.Second})
	if err == nil || !strings.Contains(err.Error(), "another migration") {
		t.Fatalf("expected lock timeout, got %v", err)
	}
	if tableExists(t, db, "a") {
		t.Fatal("nothing may run without the lock")
	}
}
