package services

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"io"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// tablesDriver is a database driver that answers the monitoring queries
// for three interfaces, records every statement, and answers anything else
// with an error.
type tablesDriver struct {
	mu   sync.Mutex
	seen []string
}

var interfaceTables = []string{
	"messages_intf_6f1b0c1e_0000_4000_8000_000000000001",
	"messages_intf_6f1b0c1e_0000_4000_8000_000000000002",
	"messages_intf_6f1b0c1e_0000_4000_8000_000000000003",
}

func (d *tablesDriver) Open(string) (driver.Conn, error) { return tablesConn{d}, nil }

type tablesConn struct{ d *tablesDriver }

func (c tablesConn) Prepare(q string) (driver.Stmt, error) { return tablesStmt{c.d, q}, nil }
func (c tablesConn) Close() error                          { return nil }
func (c tablesConn) Begin() (driver.Tx, error)             { return nil, errors.New("no transactions") }

type tablesStmt struct {
	d *tablesDriver
	q string
}

func (s tablesStmt) Close() error  { return nil }
func (s tablesStmt) NumInput() int { return -1 }
func (s tablesStmt) Exec([]driver.Value) (driver.Result, error) {
	s.d.record(s.q)
	return nil, errors.New("not run")
}

func (s tablesStmt) Query([]driver.Value) (driver.Rows, error) {
	s.d.record(s.q)
	switch {
	case strings.Contains(s.q, "interface_table_metadata"):
		var rows [][]driver.Value
		for _, t := range interfaceTables {
			rows = append(rows, []driver.Value{t})
		}
		return &fixedRows{cols: []string{"table_name"}, rows: rows}, nil
	case strings.Contains(s.q, "COUNT(*) FILTER (WHERE received_at >= $1)"):
		// Each table: 2 today, and 1, 2, ... 7 in the 7 hours, oldest first.
		return &fixedRows{cols: make([]string, 8), rows: [][]driver.Value{{int64(2), int64(1), int64(2), int64(3), int64(4), int64(5), int64(6), int64(7)}}}, nil
	case strings.Contains(s.q, "ORDER BY received_at DESC LIMIT"):
		return &fixedRows{cols: []string{"message_id", "message_type", "status", "received_at", "processing_time_ms"},
			rows: [][]driver.Value{{intfTable.FindString(s.q), "ADT", "processed", time.Now(), int64(5)}}}, nil
	case strings.Contains(s.q, "FROM interfaces"):
		return &fixedRows{cols: []string{"id", "name"}, rows: [][]driver.Value{{"6f1b0c1e-0000-4000-8000-000000000001", "ADT feed"}}}, nil
	}
	return nil, errors.New("not answered")
}

func (d *tablesDriver) record(q string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seen = append(d.seen, q)
}

type fixedRows struct {
	cols []string
	rows [][]driver.Value
	i    int
}

func (r *fixedRows) Columns() []string { return r.cols }
func (r *fixedRows) Close() error      { return nil }
func (r *fixedRows) Next(dest []driver.Value) error {
	if r.i >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.i])
	r.i++
	return nil
}

var registerTables sync.Once
var tablesDB = &tablesDriver{}

func openTablesDB(t *testing.T) *sql.DB {
	t.Helper()
	registerTables.Do(func() { sql.Register("ezhk-tables", tablesDB) })
	db, err := sql.Open("ezhk-tables", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	tablesDB.mu.Lock()
	tablesDB.seen = nil
	tablesDB.mu.Unlock()
	return db
}

var intfTable = regexp.MustCompile(`messages_intf_[a-f0-9_]+`)

// noStatementSpansTables fails if a statement read more than one
// interface's table, or used a UNION: each would lock every table at once.
func noStatementSpansTables(t *testing.T) int {
	t.Helper()
	tablesDB.mu.Lock()
	defer tablesDB.mu.Unlock()
	perTable := 0
	for _, q := range tablesDB.seen {
		if strings.Contains(strings.ToUpper(q), "UNION") {
			t.Errorf("a statement used a UNION: %s", q)
		}
		found := map[string]bool{}
		for _, m := range intfTable.FindAllString(q, -1) {
			found[m] = true
		}
		if len(found) > 1 {
			t.Errorf("a statement read %d interface tables: %s", len(found), q)
		}
		if len(found) == 1 {
			perTable++
		}
	}
	return perTable
}

// The monitoring summary reads each interface's table in its own
// statement, and adds them up.
func TestMonitoringSummaryReadsOneTablePerStatement(t *testing.T) {
	s := NewAnalyticsService(openTablesDB(t))
	summary, err := s.GetMonitoringSummary(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if n := noStatementSpansTables(t); n != len(interfaceTables) {
		t.Errorf("%d statements on interface tables, want one per table (%d)", n, len(interfaceTables))
	}
	if summary.KPIs.MessagesToday != 6 {
		t.Errorf("messages today: %d, want 6", summary.KPIs.MessagesToday)
	}
	want := []int64{3, 6, 9, 12, 15, 18, 21}
	got := summary.KPITrends.MessagesTodaySparkline
	if len(got) != 7 {
		t.Fatalf("sparkline: %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sparkline: %v, want %v", got, want)
			break
		}
	}
}

// The live feed reads each table's newest messages in its own statement,
// and names them after their interfaces.
func TestLiveMessagesReadOneTablePerStatement(t *testing.T) {
	s := NewAnalyticsService(openTablesDB(t))
	msgs, err := s.GetLiveMessages(context.Background(), 2)
	if err != nil {
		t.Fatal(err)
	}
	if n := noStatementSpansTables(t); n != len(interfaceTables) {
		t.Errorf("%d statements on interface tables, want %d", n, len(interfaceTables))
	}
	if len(msgs) != 2 {
		t.Fatalf("%d messages, want the newest 2", len(msgs))
	}
	// Each is named after its interface, or "Unknown" for one not in the
	// interfaces table.
	for _, m := range msgs {
		want := "Unknown"
		if m.InterfaceID == "6f1b0c1e-0000-4000-8000-000000000001" {
			want = "ADT feed"
		}
		if m.InterfaceName != want {
			t.Errorf("message %s of %s named %q, want %q", m.MessageID, m.InterfaceID, m.InterfaceName, want)
		}
	}
}
