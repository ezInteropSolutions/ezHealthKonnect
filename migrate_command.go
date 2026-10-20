package main

// migrate_command.go — `go-api migrate`: applies database/migrations with the
// built-in migration engine (services/migrator). Used by the installer
// (install and upgrade) and by the local Docker development stack. The server
// itself never applies migrations on its own, so an upgrade always takes its
// backup first.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"time"

	"ezhealthkonnect/config"
	"ezhealthkonnect/services/migrator"

	"github.com/lib/pq"
)

// migrateStatusJSON is the machine-readable status (`--status --json`).
type migrateStatusJSON struct {
	CurrentVersion string   `json:"currentVersion"`
	Applied        int      `json:"applied"`
	Pending        []string `json:"pending"`
	Missing        []string `json:"missing"`
	Modified       []string `json:"modified"`
	HistoryExists  bool     `json:"historyExists"`
}

// runMigrateCommand implements `go-api migrate`. Returns the process exit code.
func runMigrateCommand(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dir := fs.String("dir", "database/migrations", "directory containing V<version>__<description>.sql files")
	status := fs.Bool("status", false, "report applied and pending migrations without changing anything")
	asJSON := fs.Bool("json", false, "with --status: print machine-readable JSON")
	dryRun := fs.Bool("dry-run", false, "list the migrations that would be applied")
	baseline := fs.String("baseline-version", "", "only for a database that has tables but no migration history: treat migrations up to this version as applied")
	lockTimeout := fs.Duration("lock-timeout", 10*time.Minute, "how long to wait for another migration run to finish")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	logger := log.New(stdout, "migrate: ", 0)
	dsn := config.Load().GetDatabaseURL()
	if dsn == "" {
		fmt.Fprintln(stderr, "migrate: no database configured (set DB_HOST/DB_NAME/DB_USER/DB_PASSWORD or DATABASE_URL)")
		return 1
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Fprintf(stderr, "migrate: %v\n", err)
		return 1
	}
	defer db.Close()

	ctx := context.Background()
	if err := waitForDatabase(ctx, db, 60*time.Second); err != nil {
		fmt.Fprintf(stderr, "migrate: cannot reach the database: %v\n", err)
		return 1
	}

	m := migrator.New(db, migrator.FSSource{FS: os.DirFS("."), Dir: *dir}, migrator.PostgresHistory{}, logger)

	if *status {
		st, err := m.Status(ctx)
		if err != nil {
			fmt.Fprintf(stderr, "migrate: %v\n", err)
			return 1
		}
		if *asJSON {
			out := migrateStatusJSON{Applied: len(st.Applied), HistoryExists: st.HistoryExists,
				Pending: []string{}, Missing: []string{}, Modified: []string{}}
			if st.Current != nil {
				out.CurrentVersion = st.Current.String()
			}
			for _, p := range st.Pending {
				out.Pending = append(out.Pending, p.Script)
			}
			for _, a := range st.Missing {
				out.Missing = append(out.Missing, a.Script)
			}
			for _, mm := range st.Modified {
				out.Modified = append(out.Modified, mm.Script)
			}
			enc := json.NewEncoder(stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(out); err != nil {
				return 1
			}
			return 0
		}
		cur := "none"
		if st.Current != nil {
			cur = st.Current.String()
		}
		logger.Printf("current version: %s; %d applied, %d pending", cur, len(st.Applied), len(st.Pending))
		for _, p := range st.Pending {
			logger.Printf("pending: %s", p.Script)
		}
		return 0
	}

	if _, err := m.Migrate(ctx, migrator.Options{DryRun: *dryRun, BaselineVersion: *baseline, LockTimeout: *lockTimeout}); err != nil {
		fmt.Fprintf(stderr, "migrate: %v\n", err)
		return 1
	}
	return 0
}

// waitForDatabase retries while PostgreSQL is unreachable or still starting
// (it may be booting when the installer or the dev stack invokes migrate).
// Errors the server reports for any other reason — wrong password, missing
// database — are returned at once: waiting would not fix them.
func waitForDatabase(ctx context.Context, db *sql.DB, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		err := db.PingContext(ctx)
		if err == nil || time.Now().After(deadline) {
			return err
		}
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code != "57P03" { // 57P03: cannot_connect_now (starting up)
			return err
		}
		time.Sleep(2 * time.Second)
	}
}
