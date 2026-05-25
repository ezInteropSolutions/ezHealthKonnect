//go:build integration

// services/connectors/dlq_integration_test.go
//
// Integration tests for DLQService against a live PostgreSQL database.
// These tests require both the V138 (base table) and V140 (redrive columns)
// migrations to be applied.
//
// Prerequisites:
//   DATABASE_URL="postgres://postgres:postgres@localhost:5432/ezhealthkonnect?sslmode=disable"
//
// Run:
//   go test ./services/connectors/ -v -tags integration -run TestDLQIntegration
//
// Each test creates its own isolated rows using a shared test interface UUID and
// cleans up on completion to leave the database unchanged.
package connectors

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// ─── Shared integration helpers ───────────────────────────────────────────────

const integrationInterfaceID = "11111111-2222-3333-4444-555555555555"

// openIntegrationDB opens a PostgreSQL connection from DATABASE_URL and skips
// the test if the env var is not set or the connection fails.
func openIntegrationDB(t *testing.T) (*sql.DB, func()) {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("skipping integration test — DATABASE_URL not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if err = db.Ping(); err != nil {
		db.Close()
		t.Fatalf("db.Ping: %v", err)
	}
	return db, func() { db.Close() }
}

// ensureTestInterface inserts a minimal interface row so that delivery_dlq
// FK references (interface_id) are satisfied. Safe to call multiple times.
func ensureTestInterface(t *testing.T, db *sql.DB) {
	t.Helper()
	_, err := db.Exec(`
		INSERT INTO interfaces (id, name, type, status, created_at, updated_at)
		VALUES ($1, 'Integration Test Interface (DLQ)', 'HL7', 'active', NOW(), NOW())
		ON CONFLICT (id) DO NOTHING`, integrationInterfaceID)
	if err != nil {
		t.Fatalf("ensureTestInterface: %v", err)
	}
}

// insertDLQRow inserts a raw delivery_dlq row and returns its UUID.
func insertDLQRow(t *testing.T, db *sql.DB, messageID, status string) string {
	t.Helper()
	var id string
	err := db.QueryRow(`
		INSERT INTO delivery_dlq
			(message_id, interface_id, connector_type, payload, content_type,
			 error_message, attempt_count, next_retry_at, status, redrive_mode)
		VALUES ($1, $2::uuid, 'http_outbound', '{"data":1}', 'application/json',
		        'connection refused', 1, NOW() - INTERVAL '1 second', $3, 'from_failed_step')
		RETURNING id`,
		messageID, integrationInterfaceID, status,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insertDLQRow: %v", err)
	}
	return id
}

// cleanupDLQRows deletes all rows created during the test using the given IDs.
func cleanupDLQRows(t *testing.T, db *sql.DB, ids ...string) {
	t.Helper()
	for _, id := range ids {
		if _, err := db.Exec(`DELETE FROM delivery_dlq WHERE id = $1`, id); err != nil {
			t.Logf("cleanupDLQRows: could not delete %s: %v", id, err)
		}
	}
}

// uniqueMsg returns a unique message ID for test isolation.
func uniqueMsg(prefix string) string {
	return fmt.Sprintf("%s-%d", prefix, time.Now().UnixNano())
}

// ─── TC-DLQ-I001: WriteToDLQ insert ─────────────────────────────────────────

func TestDLQIntegration_WriteToDLQ_InsertsRow(t *testing.T) {
	// TC-DLQ-I001: WriteToDLQ inserts a new pending row.
	db, close := openIntegrationDB(t)
	defer close()
	ensureTestInterface(t, db)

	svc := NewDLQService(db)
	ctx := context.Background()
	msgID := uniqueMsg("write-insert")

	err := svc.WriteToDLQ(ctx, WriteDLQParams{
		MessageID:     msgID,
		InterfaceID:   integrationInterfaceID,
		ConnectorType: "http_outbound",
		Payload:       `{"test":true}`,
		ContentType:   "application/json",
		ErrorMessage:  "503 Service Unavailable",
		AttemptCount:  1,
		RedriveMode:   "from_failed_step",
	})
	if err != nil {
		t.Fatalf("TC-DLQ-I001: WriteToDLQ error: %v", err)
	}

	// Verify row exists
	row, err := svc.Get(ctx, "")
	_ = row
	rows, err := svc.ListPending(ctx, integrationInterfaceID, "pending", 10, 0)
	if err != nil {
		t.Fatalf("TC-DLQ-I001: ListPending error: %v", err)
	}
	var found *DLQRow
	for _, r := range rows {
		if r.MessageID == msgID {
			found = r
			break
		}
	}
	if found == nil {
		t.Fatalf("TC-DLQ-I001: inserted row not found in ListPending")
	}
	if found.Status != "pending" {
		t.Errorf("TC-DLQ-I001: status = %q, want \"pending\"", found.Status)
	}
	if found.AttemptCount != 1 {
		t.Errorf("TC-DLQ-I001: attempt_count = %d, want 1", found.AttemptCount)
	}

	cleanupDLQRows(t, db, found.ID)
}

// ─── TC-DLQ-I002: WriteToDLQ upsert ─────────────────────────────────────────

func TestDLQIntegration_WriteToDLQ_Upsert_IncrementsAttemptCount(t *testing.T) {
	// TC-DLQ-I002: Writing a second failure for the same (message_id, interface_id)
	// upserts rather than inserting a duplicate active row.
	db, close := openIntegrationDB(t)
	defer close()
	ensureTestInterface(t, db)

	svc := NewDLQService(db)
	ctx := context.Background()
	msgID := uniqueMsg("write-upsert")

	params := WriteDLQParams{
		MessageID:    msgID,
		InterfaceID:  integrationInterfaceID,
		ErrorMessage: "timeout",
		AttemptCount: 1,
	}

	if err := svc.WriteToDLQ(ctx, params); err != nil {
		t.Fatalf("TC-DLQ-I002: first write error: %v", err)
	}
	if err := svc.WriteToDLQ(ctx, params); err != nil {
		t.Fatalf("TC-DLQ-I002: second write (upsert) error: %v", err)
	}

	rows, err := svc.ListPending(ctx, integrationInterfaceID, "pending", 50, 0)
	if err != nil {
		t.Fatalf("TC-DLQ-I002: ListPending error: %v", err)
	}
	var matches []*DLQRow
	for _, r := range rows {
		if r.MessageID == msgID {
			matches = append(matches, r)
		}
	}
	if len(matches) != 1 {
		t.Errorf("TC-DLQ-I002: expected 1 row, got %d (upsert should prevent duplicates)", len(matches))
	}
	if len(matches) > 0 && matches[0].AttemptCount < 2 {
		t.Errorf("TC-DLQ-I002: attempt_count = %d, want >= 2 after upsert", matches[0].AttemptCount)
	}

	if len(matches) > 0 {
		cleanupDLQRows(t, db, matches[0].ID)
	}
}

// ─── TC-DLQ-I003: Get by ID ──────────────────────────────────────────────────

func TestDLQIntegration_Get_ByID(t *testing.T) {
	// TC-DLQ-I003: Get retrieves a specific row by its UUID.
	db, close := openIntegrationDB(t)
	defer close()
	ensureTestInterface(t, db)

	ctx := context.Background()
	msgID := uniqueMsg("get-by-id")
	id := insertDLQRow(t, db, msgID, "pending")
	defer cleanupDLQRows(t, db, id)

	svc := NewDLQService(db)
	row, err := svc.Get(ctx, id)
	if err != nil {
		t.Fatalf("TC-DLQ-I003: Get error: %v", err)
	}
	if row.ID != id {
		t.Errorf("TC-DLQ-I003: ID = %q, want %q", row.ID, id)
	}
	if row.MessageID != msgID {
		t.Errorf("TC-DLQ-I003: MessageID = %q, want %q", row.MessageID, msgID)
	}
}

func TestDLQIntegration_Get_NotFound_ReturnsError(t *testing.T) {
	// TC-DLQ-I004: Get with a non-existent UUID returns an error.
	db, close := openIntegrationDB(t)
	defer close()

	svc := NewDLQService(db)
	_, err := svc.Get(context.Background(), "00000000-0000-0000-0000-000000000000")
	if err == nil {
		t.Error("TC-DLQ-I004: expected error for missing ID, got nil")
	}
}

// ─── TC-DLQ-I005: Resolve ────────────────────────────────────────────────────

func TestDLQIntegration_Resolve_SetsStatusResolved(t *testing.T) {
	// TC-DLQ-I005: Resolve marks the row as resolved.
	db, close := openIntegrationDB(t)
	defer close()
	ensureTestInterface(t, db)

	ctx := context.Background()
	id := insertDLQRow(t, db, uniqueMsg("resolve"), "pending")
	defer cleanupDLQRows(t, db, id)

	svc := NewDLQService(db)
	if err := svc.Resolve(ctx, id); err != nil {
		t.Fatalf("TC-DLQ-I005: Resolve error: %v", err)
	}

	var status string
	if err := db.QueryRow(`SELECT status FROM delivery_dlq WHERE id = $1`, id).Scan(&status); err != nil {
		t.Fatalf("TC-DLQ-I005: status query error: %v", err)
	}
	if status != "resolved" {
		t.Errorf("TC-DLQ-I005: status = %q, want \"resolved\"", status)
	}
}

// ─── TC-DLQ-I006: Abandon ────────────────────────────────────────────────────

func TestDLQIntegration_Abandon_SetsStatusAbandoned(t *testing.T) {
	// TC-DLQ-I006: Abandon marks the row as abandoned.
	db, close := openIntegrationDB(t)
	defer close()
	ensureTestInterface(t, db)

	ctx := context.Background()
	id := insertDLQRow(t, db, uniqueMsg("abandon"), "pending")
	defer cleanupDLQRows(t, db, id)

	svc := NewDLQService(db)
	if err := svc.Abandon(ctx, id); err != nil {
		t.Fatalf("TC-DLQ-I006: Abandon error: %v", err)
	}

	var status string
	if err := db.QueryRow(`SELECT status FROM delivery_dlq WHERE id = $1`, id).Scan(&status); err != nil {
		t.Fatalf("TC-DLQ-I006: status query: %v", err)
	}
	if status != "abandoned" {
		t.Errorf("TC-DLQ-I006: status = %q, want \"abandoned\"", status)
	}
}

// ─── TC-DLQ-I007: Fail state machine ─────────────────────────────────────────

func TestDLQIntegration_Fail_BelowMaxAttempts_RemainsPending(t *testing.T) {
	// TC-DLQ-I007: Fail below maxAttempts sets status back to pending with a new next_retry_at.
	db, close := openIntegrationDB(t)
	defer close()
	ensureTestInterface(t, db)

	ctx := context.Background()
	id := insertDLQRow(t, db, uniqueMsg("fail-below"), "pending")
	defer cleanupDLQRows(t, db, id)

	svc := NewDLQService(db)
	if err := svc.Fail(ctx, id, "connection refused", 10, 5*time.Second); err != nil {
		t.Fatalf("TC-DLQ-I007: Fail error: %v", err)
	}

	var status string
	var attempts int
	if err := db.QueryRow(`SELECT status, attempt_count FROM delivery_dlq WHERE id = $1`, id).Scan(&status, &attempts); err != nil {
		t.Fatalf("TC-DLQ-I007: query error: %v", err)
	}
	if status != "pending" {
		t.Errorf("TC-DLQ-I007: status = %q, want \"pending\"", status)
	}
	if attempts < 2 {
		t.Errorf("TC-DLQ-I007: attempt_count = %d, want >= 2", attempts)
	}
}

func TestDLQIntegration_Fail_AtMaxAttempts_Abandons(t *testing.T) {
	// TC-DLQ-I008: Fail at maxAttempts transitions status to abandoned.
	db, close := openIntegrationDB(t)
	defer close()
	ensureTestInterface(t, db)

	ctx := context.Background()
	id := insertDLQRow(t, db, uniqueMsg("fail-max"), "pending")
	defer cleanupDLQRows(t, db, id)

	svc := NewDLQService(db)
	// maxAttempts=1 means the very first failure abandons
	if err := svc.Fail(ctx, id, "fatal", 1, 5*time.Second); err != nil {
		t.Fatalf("TC-DLQ-I008: Fail error: %v", err)
	}

	var status string
	if err := db.QueryRow(`SELECT status FROM delivery_dlq WHERE id = $1`, id).Scan(&status); err != nil {
		t.Fatalf("TC-DLQ-I008: query error: %v", err)
	}
	if status != "abandoned" {
		t.Errorf("TC-DLQ-I008: status = %q, want \"abandoned\"", status)
	}
}

// ─── TC-DLQ-I009: Stats ──────────────────────────────────────────────────────

func TestDLQIntegration_Stats_CountsByStatus(t *testing.T) {
	// TC-DLQ-I009: Stats returns correct counts including rows created in this test.
	db, close := openIntegrationDB(t)
	defer close()
	ensureTestInterface(t, db)

	ctx := context.Background()
	id1 := insertDLQRow(t, db, uniqueMsg("stats-pending"), "pending")
	id2 := insertDLQRow(t, db, uniqueMsg("stats-abandoned"), "abandoned")
	defer cleanupDLQRows(t, db, id1, id2)

	svc := NewDLQService(db)
	counts, err := svc.Stats(ctx)
	if err != nil {
		t.Fatalf("TC-DLQ-I009: Stats error: %v", err)
	}

	if counts["pending"] < 1 {
		t.Errorf("TC-DLQ-I009: pending count = %d, want >= 1", counts["pending"])
	}
	if counts["abandoned"] < 1 {
		t.Errorf("TC-DLQ-I009: abandoned count = %d, want >= 1", counts["abandoned"])
	}
}

// ─── TC-DLQ-I010: BulkAbandon ────────────────────────────────────────────────

func TestDLQIntegration_BulkAbandon_AffectsAllIDs(t *testing.T) {
	// TC-DLQ-I010: BulkAbandon marks all given IDs as abandoned and returns affected count.
	db, close := openIntegrationDB(t)
	defer close()
	ensureTestInterface(t, db)

	ctx := context.Background()
	id1 := insertDLQRow(t, db, uniqueMsg("bulk-1"), "pending")
	id2 := insertDLQRow(t, db, uniqueMsg("bulk-2"), "pending")
	id3 := insertDLQRow(t, db, uniqueMsg("bulk-3"), "pending")
	defer cleanupDLQRows(t, db, id1, id2, id3)

	svc := NewDLQService(db)
	affected, err := svc.BulkAbandon(ctx, []string{id1, id2, id3})
	if err != nil {
		t.Fatalf("TC-DLQ-I010: BulkAbandon error: %v", err)
	}
	if affected != 3 {
		t.Errorf("TC-DLQ-I010: affected = %d, want 3", affected)
	}

	for _, id := range []string{id1, id2, id3} {
		var status string
		if err := db.QueryRow(`SELECT status FROM delivery_dlq WHERE id = $1`, id).Scan(&status); err != nil {
			t.Fatalf("TC-DLQ-I010: status query for %s: %v", id, err)
		}
		if status != "abandoned" {
			t.Errorf("TC-DLQ-I010: id=%s status = %q, want \"abandoned\"", id, status)
		}
	}
}

// ─── TC-DLQ-I011: ListPending filters ────────────────────────────────────────

func TestDLQIntegration_ListPending_FiltersByInterfaceID(t *testing.T) {
	// TC-DLQ-I011: ListPending with interface_id filter only returns rows for that interface.
	db, close := openIntegrationDB(t)
	defer close()
	ensureTestInterface(t, db)

	ctx := context.Background()
	msgID := uniqueMsg("list-filter")
	id := insertDLQRow(t, db, msgID, "pending")
	defer cleanupDLQRows(t, db, id)

	svc := NewDLQService(db)
	// Filter by the test interface — should include our row
	rows, err := svc.ListPending(ctx, integrationInterfaceID, "pending", 100, 0)
	if err != nil {
		t.Fatalf("TC-DLQ-I011: ListPending error: %v", err)
	}
	var found bool
	for _, r := range rows {
		if r.ID == id {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("TC-DLQ-I011: inserted row %s not found in ListPending for interface %s", id, integrationInterfaceID)
	}

	// Filter by a different interface — should NOT include our row
	otherRows, err := svc.ListPending(ctx, "aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee", "pending", 100, 0)
	if err != nil {
		t.Fatalf("TC-DLQ-I011: ListPending (other iface) error: %v", err)
	}
	for _, r := range otherRows {
		if r.ID == id {
			t.Errorf("TC-DLQ-I011: row %s appeared under wrong interface filter", id)
		}
	}
}

func TestDLQIntegration_ListPending_FiltersByStatus(t *testing.T) {
	// TC-DLQ-I012: ListPending with status filter excludes rows of other statuses.
	db, close := openIntegrationDB(t)
	defer close()
	ensureTestInterface(t, db)

	ctx := context.Background()
	pendingID := insertDLQRow(t, db, uniqueMsg("list-status-p"), "pending")
	abandonedID := insertDLQRow(t, db, uniqueMsg("list-status-a"), "abandoned")
	defer cleanupDLQRows(t, db, pendingID, abandonedID)

	svc := NewDLQService(db)
	rows, err := svc.ListPending(ctx, "", "pending", 100, 0)
	if err != nil {
		t.Fatalf("TC-DLQ-I012: ListPending error: %v", err)
	}
	for _, r := range rows {
		if r.ID == abandonedID {
			t.Errorf("TC-DLQ-I012: abandoned row appeared in pending filter results")
		}
	}
}

// ─── TC-DLQ-I013: Expiry filtering ───────────────────────────────────────────

func TestDLQIntegration_WriteToDLQ_WithExpiresAt(t *testing.T) {
	// TC-DLQ-I013: WriteToDLQ stores expires_at and the poller query respects it.
	db, close := openIntegrationDB(t)
	defer close()
	ensureTestInterface(t, db)

	ctx := context.Background()
	msgID := uniqueMsg("expires")
	expiresAt := time.Now().Add(24 * time.Hour)

	svc := NewDLQService(db)
	if err := svc.WriteToDLQ(ctx, WriteDLQParams{
		MessageID:    msgID,
		InterfaceID:  integrationInterfaceID,
		ErrorMessage: "test",
		AttemptCount: 1,
		ExpiresAt:    &expiresAt,
	}); err != nil {
		t.Fatalf("TC-DLQ-I013: WriteToDLQ error: %v", err)
	}

	rows, err := svc.ListPending(ctx, integrationInterfaceID, "pending", 100, 0)
	if err != nil {
		t.Fatalf("TC-DLQ-I013: ListPending error: %v", err)
	}
	var found *DLQRow
	for _, r := range rows {
		if r.MessageID == msgID {
			found = r
			break
		}
	}
	if found == nil {
		t.Fatalf("TC-DLQ-I013: row with expires_at not found")
	}
	if found.ExpiresAt == nil {
		t.Errorf("TC-DLQ-I013: ExpiresAt is nil, want non-nil")
	} else if found.ExpiresAt.Before(time.Now()) {
		t.Errorf("TC-DLQ-I013: ExpiresAt = %v is in the past", found.ExpiresAt)
	}

	cleanupDLQRows(t, db, found.ID)
}

// ─── TC-DLQ-I014: Snapshot round-trip ────────────────────────────────────────

func TestDLQIntegration_WriteToDLQ_SnapshotsRoundTrip(t *testing.T) {
	// TC-DLQ-I014: pipeline_input_snapshot and pipeline_data_snapshot survive a DB round-trip.
	db, close := openIntegrationDB(t)
	defer close()
	ensureTestInterface(t, db)

	ctx := context.Background()
	msgID := uniqueMsg("snapshots")

	svc := NewDLQService(db)
	if err := svc.WriteToDLQ(ctx, WriteDLQParams{
		MessageID:   msgID,
		InterfaceID: integrationInterfaceID,
		AttemptCount: 1,
		PipelineInputSnapshot: map[string]interface{}{"raw": "MSH|^~\\&|..."},
		PipelineDataSnapshot:  map[string]interface{}{"step_result": "ok", "count": float64(3)},
	}); err != nil {
		t.Fatalf("TC-DLQ-I014: WriteToDLQ error: %v", err)
	}

	rows, err := svc.ListPending(ctx, integrationInterfaceID, "pending", 100, 0)
	if err != nil {
		t.Fatalf("TC-DLQ-I014: ListPending error: %v", err)
	}
	var found *DLQRow
	for _, r := range rows {
		if r.MessageID == msgID {
			found = r
			break
		}
	}
	if found == nil {
		t.Fatalf("TC-DLQ-I014: row not found after insert")
	}

	if found.PipelineInputSnapshot == nil {
		t.Error("TC-DLQ-I014: PipelineInputSnapshot is nil after round-trip")
	} else if found.PipelineInputSnapshot["raw"] != "MSH|^~\\&|..." {
		t.Errorf("TC-DLQ-I014: PipelineInputSnapshot[raw] = %v, want \"MSH|^~\\&|...\"", found.PipelineInputSnapshot["raw"])
	}
	if found.PipelineDataSnapshot == nil {
		t.Error("TC-DLQ-I014: PipelineDataSnapshot is nil after round-trip")
	} else if found.PipelineDataSnapshot["step_result"] != "ok" {
		t.Errorf("TC-DLQ-I014: PipelineDataSnapshot[step_result] = %v, want \"ok\"", found.PipelineDataSnapshot["step_result"])
	}

	cleanupDLQRows(t, db, found.ID)
}
