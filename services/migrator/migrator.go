package migrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

// lockKey is the PostgreSQL advisory-lock key that serializes migrators
// ("ezhkmigr" as a big-endian int64).
const lockKey int64 = 0x657a686b6d696772

// Logger receives progress messages.
type Logger interface {
	Printf(format string, args ...any)
}

// Options control a migration run.
type Options struct {
	// DryRun reports what would be applied without changing the database.
	DryRun bool
	// BaselineVersion is required only when the database already has tables
	// but no migration history: migrations up to and including this version
	// are recorded as already applied (Flyway "baseline").
	BaselineVersion string
	// LockTimeout bounds how long to wait for another migrator (default 10m).
	LockTimeout time.Duration
}

// ChecksumMismatch is an applied migration whose file has changed since.
type ChecksumMismatch struct {
	Script           string
	Applied, Current int32
}

// Status describes the database relative to the available migrations.
type Status struct {
	HistoryExists bool
	Applied       []AppliedMigration
	Pending       []Migration
	OutOfOrder    []Migration        // pending, but older than the newest applied version
	Missing       []AppliedMigration // applied, but the file no longer exists
	Modified      []ChecksumMismatch // applied, but the file changed afterwards
	Current       *Version           // newest applied version
}

// Result is the outcome of Migrate.
type Result struct {
	Status
	Executed []Migration
}

// Migrator applies pending migrations from a Source, recording them in History.
type Migrator struct {
	db      *sql.DB
	source  Source
	history History
	log     Logger
}

// New creates a Migrator.
func New(db *sql.DB, source Source, history History, log Logger) *Migrator {
	return &Migrator{db: db, source: source, history: history, log: log}
}

// Status reports applied and pending migrations without changing anything.
func (m *Migrator) Status(ctx context.Context) (*Status, error) {
	migs, err := m.source.Load()
	if err != nil {
		return nil, err
	}
	return m.status(ctx, migs, "")
}

func (m *Migrator) status(ctx context.Context, migs []Migration, baseline string) (*Status, error) {
	st := &Status{}
	exists, err := m.history.Exists(ctx, m.db)
	if err != nil {
		return nil, err
	}
	st.HistoryExists = exists
	var applied []AppliedMigration
	if exists {
		if applied, err = m.history.Applied(ctx, m.db); err != nil {
			return nil, err
		}
	}
	st.Applied = applied

	var baseVer *Version
	appliedByKey := map[string]AppliedMigration{}
	for _, a := range applied {
		appliedByKey[a.Version.key()] = a
		if a.Baseline {
			v := a.Version
			baseVer = &v
		}
		if st.Current == nil || a.Version.Compare(*st.Current) > 0 {
			v := a.Version
			st.Current = &v
		}
	}
	if baseline != "" {
		v, err := ParseVersion(baseline)
		if err != nil {
			return nil, fmt.Errorf("invalid baseline version: %w", err)
		}
		baseVer = &v
	}

	files := map[string]bool{}
	for _, mig := range migs {
		files[mig.Version.key()] = true
		if a, ok := appliedByKey[mig.Version.key()]; ok {
			if a.Checksum != nil && *a.Checksum != mig.Checksum {
				st.Modified = append(st.Modified, ChecksumMismatch{Script: mig.Script, Applied: *a.Checksum, Current: mig.Checksum})
			}
			continue
		}
		if baseVer != nil && mig.Version.Compare(*baseVer) <= 0 {
			continue // covered by the baseline
		}
		st.Pending = append(st.Pending, mig)
		if st.Current != nil && mig.Version.Compare(*st.Current) < 0 {
			st.OutOfOrder = append(st.OutOfOrder, mig)
		}
	}
	for _, a := range applied {
		if !a.Baseline && !files[a.Version.key()] {
			st.Missing = append(st.Missing, a)
		}
	}
	return st, nil
}

// Migrate applies every pending migration in version order, stopping at the
// first failure. A failed migration leaves no trace when it runs in a
// migrator-managed transaction (the default); see ExecutionMode.
func (m *Migrator) Migrate(ctx context.Context, opts Options) (*Result, error) {
	migs, err := m.source.Load()
	if err != nil {
		return nil, err
	}
	// One connection holds the advisory lock for the whole run; work happens
	// on others, so a single-connection pool would wait forever.
	if max := m.db.Stats().MaxOpenConnections; max == 1 {
		return nil, errors.New("migrator needs a connection pool of at least 2 connections")
	}

	lockConn, err := m.acquireLock(ctx, opts.LockTimeout)
	if err != nil {
		return nil, err
	}
	defer func() {
		_, _ = lockConn.ExecContext(context.Background(), `SELECT pg_advisory_unlock($1)`, lockKey)
		lockConn.Close()
	}()

	exists, err := m.history.Exists(ctx, m.db)
	if err != nil {
		return nil, err
	}
	baseline := ""
	if !exists {
		hasTables, err := m.schemaHasTables(ctx)
		if err != nil {
			return nil, err
		}
		if hasTables {
			if opts.BaselineVersion == "" {
				return nil, errors.New("the database already contains tables but has no migration history; " +
					"refusing to run migrations over unknown data. If it is known to match a release, " +
					"rerun with --baseline-version set to that release's last migration version")
			}
			baseline = opts.BaselineVersion
		}
	}

	st, err := m.status(ctx, migs, baseline)
	if err != nil {
		return nil, err
	}
	res := &Result{Status: *st}
	m.report(st)

	if opts.DryRun {
		for _, mig := range st.Pending {
			m.log.Printf("would apply V%s %s (%s)", mig.Version, mig.Description, mig.Analysis.Mode)
		}
		return res, nil
	}

	if !exists {
		if err := m.history.Ensure(ctx, m.db); err != nil {
			return nil, err
		}
		if baseline != "" {
			if err := m.history.Append(ctx, m.db, HistoryEntry{
				Version: baseline, Description: "<< Flyway Baseline >>", Type: "BASELINE", Script: "<< Flyway Baseline >>",
			}); err != nil {
				return nil, err
			}
			m.log.Printf("baselined existing database at version %s", baseline)
		}
	}

	for _, mig := range st.Pending {
		start := time.Now()
		if err := m.apply(ctx, mig); err != nil {
			return res, err
		}
		res.Executed = append(res.Executed, mig)
		m.log.Printf("applied V%s %s (%s, %s)", mig.Version, mig.Description, mig.Analysis.Mode, time.Since(start).Round(time.Millisecond))
	}
	if len(st.Pending) == 0 {
		m.log.Printf("database is up to date")
	} else {
		m.log.Printf("applied %d migration(s)", len(res.Executed))
	}
	return res, nil
}

func (m *Migrator) report(st *Status) {
	cur := "none"
	if st.Current != nil {
		cur = st.Current.String()
	}
	m.log.Printf("current version: %s; %d applied, %d pending", cur, len(st.Applied), len(st.Pending))
	for _, mm := range st.Modified {
		m.log.Printf("warning: %s changed after it was applied (checksum %d, now %d); it will not be re-run", mm.Script, mm.Applied, mm.Current)
	}
	for _, a := range st.Missing {
		m.log.Printf("warning: applied migration %s is no longer present", a.Script)
	}
	for _, mig := range st.OutOfOrder {
		m.log.Printf("note: %s is older than the current version and will be applied out of order", mig.Script)
	}
}

// apply runs one migration and records it in the history.
func (m *Migrator) apply(ctx context.Context, mig Migration) error {
	conn, err := m.db.Conn(ctx)
	if err != nil {
		return fmt.Errorf("V%s: open connection: %w", mig.Version, err)
	}
	defer conn.Close()

	checksum := mig.Checksum
	entry := func(start time.Time) HistoryEntry {
		return HistoryEntry{
			Version: mig.Version.String(), Description: mig.Description, Type: "SQL",
			Script: mig.Script, Checksum: &checksum, ExecutionTime: time.Since(start),
		}
	}
	start := time.Now()

	switch mig.Analysis.Mode {
	case ModeTransactional:
		tx, err := conn.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("V%s: begin: %w", mig.Version, err)
		}
		if _, err := tx.ExecContext(ctx, mig.SQL); err != nil {
			_ = tx.Rollback()
			return describeError(mig, mig.SQL, err)
		}
		if err := m.history.Append(ctx, tx, entry(start)); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("V%s: commit: %w", mig.Version, err)
		}
	case ModeSelfManaged:
		if _, err := conn.ExecContext(ctx, mig.SQL); err != nil {
			// Leave no aborted transaction behind on this pooled connection.
			_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
			return describeError(mig, mig.SQL, err)
		}
		if err := m.history.Append(ctx, conn, entry(start)); err != nil {
			return err
		}
	case ModeNonTransactional:
		for _, stmt := range mig.Analysis.Statements {
			if _, err := conn.ExecContext(ctx, stmt); err != nil {
				return describeError(mig, stmt, err)
			}
		}
		if err := m.history.Append(ctx, conn, entry(start)); err != nil {
			return err
		}
	}
	return nil
}

// describeError wraps a database error with the migration and, when
// PostgreSQL reports a position, the line it points at.
func describeError(mig Migration, sqlText string, err error) error {
	msg := fmt.Sprintf("V%s (%s) failed", mig.Version, mig.Script)
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		if pos := atoiSafe(pqErr.Position); pos > 0 && pos <= len(sqlText)+1 {
			line := 1 + strings.Count(sqlText[:pos-1], "\n")
			if idx := strings.Index(mig.SQL, sqlText); idx >= 0 {
				line += strings.Count(mig.SQL[:idx], "\n")
			}
			msg += fmt.Sprintf(" at line %d", line)
		}
		if pqErr.Detail != "" {
			return fmt.Errorf("%s: %w (detail: %s)", msg, err, pqErr.Detail)
		}
	}
	return fmt.Errorf("%s: %w", msg, err)
}

func atoiSafe(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

func (m *Migrator) acquireLock(ctx context.Context, timeout time.Duration) (*sql.Conn, error) {
	if timeout <= 0 {
		timeout = 10 * time.Minute
	}
	conn, err := m.db.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("connect: %w", err)
	}
	deadline := time.Now().Add(timeout)
	announced := false
	for {
		var got bool
		if err := conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock($1)`, lockKey).Scan(&got); err != nil {
			conn.Close()
			return nil, fmt.Errorf("acquire migration lock: %w", err)
		}
		if got {
			return conn, nil
		}
		if time.Now().After(deadline) {
			conn.Close()
			return nil, fmt.Errorf("another migration is still running after %s; try again later", timeout)
		}
		if !announced {
			m.log.Printf("waiting for another migration run to finish...")
			announced = true
		}
		select {
		case <-ctx.Done():
			conn.Close()
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
		}
	}
}

func (m *Migrator) schemaHasTables(ctx context.Context) (bool, error) {
	var n int
	err := m.db.QueryRowContext(ctx, `
		SELECT count(*) FROM pg_catalog.pg_tables
		 WHERE schemaname = current_schema() AND tablename <> $1`, HistoryTable).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("inspect schema: %w", err)
	}
	return n > 0, nil
}
