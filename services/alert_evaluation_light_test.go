package services

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"strings"
	"sync"
	"testing"
)

// recordingDriver is a database driver that records every statement and
// answers each with an error, to see what a piece of code asks the database
// without one.
type recordingDriver struct {
	mu   sync.Mutex
	seen []string
}

func (d *recordingDriver) Open(string) (driver.Conn, error) { return recordingConn{d}, nil }

func (d *recordingDriver) record(q string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.seen = append(d.seen, q)
}

type recordingConn struct{ d *recordingDriver }

func (c recordingConn) Prepare(q string) (driver.Stmt, error) { return recordingStmt{c.d, q}, nil }
func (c recordingConn) Close() error                          { return nil }
func (c recordingConn) Begin() (driver.Tx, error)             { return nil, errors.New("no transactions") }

type recordingStmt struct {
	d *recordingDriver
	q string
}

var errRecorded = errors.New("recorded, not run")

func (s recordingStmt) Close() error  { return nil }
func (s recordingStmt) NumInput() int { return -1 }
func (s recordingStmt) Exec([]driver.Value) (driver.Result, error) {
	s.d.record(s.q)
	return nil, errRecorded
}
func (s recordingStmt) Query([]driver.Value) (driver.Rows, error) {
	s.d.record(s.q)
	return nil, errRecorded
}

var registerRecorder sync.Once
var recorder = &recordingDriver{}

// The background alert check reads only the figures the rules compare
// against, never the monitoring summary's counts across every interface's
// message table: those lock every such table in one statement, and run every
// minute they filled PostgreSQL's shared lock table.
func TestAlertEvaluationNeverQueriesEveryInterfaceTable(t *testing.T) {
	registerRecorder.Do(func() { sql.Register("ezhk-recorder", recorder) })
	db, err := sql.Open("ezhk-recorder", "")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	recorder.mu.Lock()
	recorder.seen = nil
	recorder.mu.Unlock()

	s := NewAnalyticsService(db)
	_ = s.EvaluateAlertRules(context.Background())

	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	if len(recorder.seen) == 0 {
		t.Fatal("the alert check asked the database nothing")
	}
	var kpis int
	for _, q := range recorder.seen {
		lower := strings.ToLower(q)
		if strings.Contains(lower, "messages_intf") || strings.Contains(lower, "union") || strings.Contains(lower, "information_schema") {
			t.Errorf("the alert check read every interface's table: %s", q)
		}
		if strings.Contains(lower, "transformation_executions") || strings.Contains(lower, "dead_letter_queue") {
			kpis++
		}
	}
	if kpis != 3 {
		t.Errorf("the alert check read %d of the 3 figures the rules use: %v", kpis, recorder.seen)
	}
}
