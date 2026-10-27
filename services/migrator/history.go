package migrator

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// HistoryTable is Flyway's history table; reusing it keeps databases migrated
// by Flyway and by this package interchangeable.
const HistoryTable = "flyway_schema_history"

// Execer is satisfied by *sql.DB, *sql.Conn and *sql.Tx.
type Execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// AppliedMigration is the effective state of one version in the history.
type AppliedMigration struct {
	Version     Version
	Description string
	Script      string
	Checksum    *int32
	InstalledOn time.Time
	Baseline    bool
}

// HistoryEntry is a row to append to the history table.
type HistoryEntry struct {
	Version       string
	Description   string
	Type          string // "SQL" or "BASELINE"
	Script        string
	Checksum      *int32
	ExecutionTime time.Duration
}

// History reads and appends migration history.
type History interface {
	// Exists reports whether the history table exists.
	Exists(ctx context.Context, q Execer) (bool, error)
	// Ensure creates the history table if it does not exist.
	Ensure(ctx context.Context, q Execer) error
	// Applied returns the effective applied migrations (in history order,
	// with Flyway DELETE markers applied and failed rows ignored).
	Applied(ctx context.Context, db *sql.DB) ([]AppliedMigration, error)
	// Append records a successful migration (or baseline).
	Append(ctx context.Context, q Execer, e HistoryEntry) error
}

// PostgresHistory implements History on flyway_schema_history.
type PostgresHistory struct{}

// Exists implements History.
func (PostgresHistory) Exists(ctx context.Context, q Execer) (bool, error) {
	var name sql.NullString
	if err := q.QueryRowContext(ctx, `SELECT to_regclass($1)::text`, HistoryTable).Scan(&name); err != nil {
		return false, fmt.Errorf("check %s: %w", HistoryTable, err)
	}
	return name.Valid, nil
}

// Ensure implements History. The DDL matches Flyway's own for PostgreSQL.
func (PostgresHistory) Ensure(ctx context.Context, q Execer) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS ` + HistoryTable + ` (
			installed_rank INT NOT NULL,
			version VARCHAR(50),
			description VARCHAR(200) NOT NULL,
			type VARCHAR(20) NOT NULL,
			script VARCHAR(1000) NOT NULL,
			checksum INTEGER,
			installed_by VARCHAR(100) NOT NULL,
			installed_on TIMESTAMP NOT NULL DEFAULT now(),
			execution_time INTEGER NOT NULL,
			success BOOLEAN NOT NULL,
			CONSTRAINT ` + HistoryTable + `_pk PRIMARY KEY (installed_rank)
		)`,
		`CREATE INDEX IF NOT EXISTS ` + HistoryTable + `_s_idx ON ` + HistoryTable + ` (success)`,
	}
	for _, s := range stmts {
		if _, err := q.ExecContext(ctx, s); err != nil {
			return fmt.Errorf("create %s: %w", HistoryTable, err)
		}
	}
	return nil
}

// Applied implements History. Rows are replayed in installed_rank order: a
// successful SQL/BASELINE row marks its version applied, a DELETE row (written
// by `flyway repair` for a removed file) marks it not applied.
func (PostgresHistory) Applied(ctx context.Context, db *sql.DB) ([]AppliedMigration, error) {
	rows, err := db.QueryContext(ctx, `
		SELECT version, description, type, script, checksum, installed_on, success
		  FROM `+HistoryTable+`
		 WHERE version IS NOT NULL
		 ORDER BY installed_rank`)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", HistoryTable, err)
	}
	defer rows.Close()

	var order []string
	state := map[string]AppliedMigration{}
	for rows.Next() {
		var (
			ver, desc, typ, script string
			checksum               sql.NullInt32
			on                     time.Time
			success                bool
		)
		if err := rows.Scan(&ver, &desc, &typ, &script, &checksum, &on, &success); err != nil {
			return nil, fmt.Errorf("read %s: %w", HistoryTable, err)
		}
		v, err := ParseVersion(ver)
		if err != nil {
			return nil, fmt.Errorf("%s contains an invalid version %q: %w", HistoryTable, ver, err)
		}
		switch {
		case typ == "DELETE":
			delete(state, v.key())
		case success:
			am := AppliedMigration{Version: v, Description: desc, Script: script, InstalledOn: on, Baseline: typ == "BASELINE"}
			if checksum.Valid {
				c := checksum.Int32
				am.Checksum = &c
			}
			if _, seen := state[v.key()]; !seen {
				order = append(order, v.key())
			}
			state[v.key()] = am
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	var applied []AppliedMigration
	emitted := map[string]bool{}
	for _, k := range order {
		if am, ok := state[k]; ok && !emitted[k] {
			applied = append(applied, am)
			emitted[k] = true
		}
	}
	return applied, nil

}

// Append implements History.
func (PostgresHistory) Append(ctx context.Context, q Execer, e HistoryEntry) error {
	var checksum any
	if e.Checksum != nil {
		checksum = *e.Checksum
	}
	_, err := q.ExecContext(ctx, `
		INSERT INTO `+HistoryTable+`
		       (installed_rank, version, description, type, script, checksum,
		        installed_by, installed_on, execution_time, success)
		VALUES ((SELECT COALESCE(MAX(installed_rank), 0) + 1 FROM `+HistoryTable+`),
		        $1, $2, $3, $4, $5, current_user, now(), $6, true)`,
		e.Version, truncate(e.Description, 200), e.Type, truncate(e.Script, 1000), checksum,
		int(e.ExecutionTime/time.Millisecond))
	if err != nil {
		return fmt.Errorf("record %s in %s: %w", e.Script, HistoryTable, err)
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
