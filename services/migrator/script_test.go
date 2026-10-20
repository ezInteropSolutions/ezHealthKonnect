package migrator

import (
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

func TestSplitStatements(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		want []string
	}{
		{"simple", "SELECT 1; SELECT 2;", []string{"SELECT 1", "SELECT 2"}},
		{"no trailing semicolon", "SELECT 1;\nSELECT 2", []string{"SELECT 1", "SELECT 2"}},
		{"semicolon in string", "INSERT INTO t VALUES ('a;b'); SELECT 1;", []string{"INSERT INTO t VALUES ('a;b')", "SELECT 1"}},
		{"doubled quote", "SELECT 'it''s; fine'; SELECT 2;", []string{"SELECT 'it''s; fine'", "SELECT 2"}},
		{"escape string", `SELECT E'a\';b'; SELECT 2;`, []string{`SELECT E'a\';b'`, "SELECT 2"}},
		{"quoted identifier", `CREATE TABLE "a;b" (x int); SELECT 1;`, []string{`CREATE TABLE "a;b" (x int)`, "SELECT 1"}},
		{"dollar body", "CREATE FUNCTION f() RETURNS int AS $$ BEGIN RETURN 1; END; $$ LANGUAGE plpgsql; SELECT 1;",
			[]string{"CREATE FUNCTION f() RETURNS int AS $$ BEGIN RETURN 1; END; $$ LANGUAGE plpgsql", "SELECT 1"}},
		{"tagged dollar body", "DO $body$ BEGIN PERFORM 1; END $body$; SELECT 1;",
			[]string{"DO $body$ BEGIN PERFORM 1; END $body$", "SELECT 1"}},
		{"nested $$ inside tagged", "DO $o$ BEGIN EXECUTE $$SELECT 1;$$; END $o$;", []string{"DO $o$ BEGIN EXECUTE $$SELECT 1;$$; END $o$"}},
		{"positional param is not a dollar quote", "PREPARE p AS SELECT $1; SELECT 2;", []string{"PREPARE p AS SELECT $1", "SELECT 2"}},
		{"line comment", "SELECT 1; -- a; comment\nSELECT 2;", []string{"SELECT 1", "-- a; comment\nSELECT 2"}},
		{"nested block comment", "/* a /* b; */ c; */ SELECT 1;", []string{"/* a /* b; */ c; */ SELECT 1"}},
		{"comment-only tail dropped", "SELECT 1;\n-- done\n", []string{"SELECT 1"}},
		{"begin atomic body", "CREATE FUNCTION f() RETURNS int LANGUAGE sql BEGIN ATOMIC SELECT 1; SELECT 2; END; SELECT 3;",
			[]string{"CREATE FUNCTION f() RETURNS int LANGUAGE sql BEGIN ATOMIC SELECT 1; SELECT 2; END", "SELECT 3"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := splitStatements(c.sql); !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %q\nwant %q", got, c.want)
			}
		})
	}
}

func TestAnalyzeScript_Mode(t *testing.T) {
	cases := []struct {
		name string
		sql  string
		want ExecutionMode
	}{
		{"plain DDL", "CREATE TABLE t (id int); INSERT INTO t VALUES (1);", ModeTransactional},
		{"plpgsql BEGIN is not transaction control", "DO $$ BEGIN RAISE NOTICE 'x'; END $$;", ModeTransactional},
		{"own transaction", "BEGIN;\nCREATE TABLE t (id int);\nCOMMIT;", ModeSelfManaged},
		{"lowercase + comments", "-- header\nbegin;\nselect 1;\ncommit;", ModeSelfManaged},
		{"start transaction", "START TRANSACTION; SELECT 1; END;", ModeSelfManaged},
		{"begin atomic is not transaction control", "CREATE FUNCTION f() RETURNS int LANGUAGE sql BEGIN ATOMIC SELECT 1; END;", ModeTransactional},
		{"concurrent index", "CREATE INDEX CONCURRENTLY i ON t (x);", ModeNonTransactional},
		{"concurrent unique index", "CREATE UNIQUE INDEX CONCURRENTLY IF NOT EXISTS i ON t (x);", ModeNonTransactional},
		{"drop index concurrently", "DROP INDEX CONCURRENTLY i;", ModeNonTransactional},
		{"vacuum", "VACUUM ANALYZE t;", ModeNonTransactional},
		{"concurrently only mentioned in a comment", "-- could use CONCURRENTLY\nCREATE INDEX i ON t (x);", ModeTransactional},
		{"concurrently only in a string", "INSERT INTO notes VALUES ('CREATE INDEX CONCURRENTLY');", ModeTransactional},
	}
	for _, c := range cases {
		if got := AnalyzeScript(c.sql).Mode; got != c.want {
			t.Errorf("%s: mode %v, want %v", c.name, got, c.want)
		}
	}
}

// Runs the analyzer over every real migration and cross-checks it against an
// independent line-based scan: the 25 files that wrap themselves in
// BEGIN;/COMMIT; must be self-managed, and no real migration needs
// statement-by-statement execution today.
func TestAnalyzeScript_RealMigrations(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(realMigrationsDir, "V*.sql"))
	if err != nil || len(files) < 250 {
		t.Fatalf("expected the real migrations, got %d (%v)", len(files), err)
	}
	txLine := regexp.MustCompile(`(?im)^\s*(BEGIN|COMMIT)\s*;`)
	var bySelfScan, byAnalyzer, nonTx []string
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(f)
		if txLine.Match(data) {
			bySelfScan = append(bySelfScan, name)
		}
		s := AnalyzeScript(string(data))
		switch s.Mode {
		case ModeSelfManaged:
			byAnalyzer = append(byAnalyzer, name)
		case ModeNonTransactional:
			nonTx = append(nonTx, name)
		}
		if len(s.Statements) == 0 && strings.TrimSpace(string(data)) != "" && !onlyComments(string(data)) {
			t.Errorf("%s: no statements found", name)
		}
	}
	sort.Strings(bySelfScan)
	sort.Strings(byAnalyzer)
	if len(byAnalyzer) != 25 || !reflect.DeepEqual(bySelfScan, byAnalyzer) {
		t.Errorf("self-managed migrations\nanalyzer (%d): %v\nline scan (%d): %v", len(byAnalyzer), byAnalyzer, len(bySelfScan), bySelfScan)
	}
	if len(nonTx) != 0 {
		t.Errorf("unexpected non-transactional migrations: %v", nonTx)
	}
}
