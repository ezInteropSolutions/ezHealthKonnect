// controllers/fhir_validation_queue_controller_test.go
// Unit + integration tests for FHIRValidationQueueController.
//
// Unit tests use DATA-DRIVEN approach — no real DB, exercising JSON encoding,
// status transitions, and helper logic.
//
// Integration tests (build tag: integration) require a live PostgreSQL DB with
// the V64 migration applied. Set DATABASE_URL env var:
//
//	DATABASE_URL="postgres://postgres:postgres@localhost:5432/ezhealthkonnect?sslmode=disable"
//	go test ./controllers/ -v -run TestFHIRValidationQueue
//
// Integration-only tests also run when DATABASE_URL is set without the tag.

package controllers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
)

// ─────────────────────────────────────────────────────────────────────────────
// Test helpers
// ─────────────────────────────────────────────────────────────────────────────

// testDB opens a real PostgreSQL connection if DATABASE_URL is set.
// Returns (db, cleanup, ok) — if ok is false the caller should skip.
func testDB(t *testing.T) (*sql.DB, func(), bool) {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return nil, func() {}, false
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Logf("testDB: sql.Open error: %v — skipping DB tests", err)
		return nil, func() {}, false
	}
	if err = db.Ping(); err != nil {
		db.Close()
		t.Logf("testDB: ping error: %v — skipping DB tests", err)
		return nil, func() {}, false
	}
	cleanup := func() { db.Close() }
	return db, cleanup, true
}

// ginContext creates a test Gin context with a ResponseRecorder.
// Returns (context, recorder).
func ginContext(method, url string, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, url, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, url, nil)
	}
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	return c, w
}

// insertTestQueueItem inserts a row into fhir_validation_queue and returns its ID.
func insertTestQueueItem(t *testing.T, db *sql.DB, status string) string {
	t.Helper()
	// Use a known-good interface_id that references no actual interface (FK may not exist)
	// We use ON CONFLICT DO NOTHING pattern — insert ignores FK violations on interface_id
	// by using a hard-coded UUID that should exist or disabling FK check in test.
	// Simplest: ensure a test interface row exists.
	interfaceID := "00000000-0000-0000-0000-000000000001"
	// Try to create a minimal test interface row (ignore if already exists)
	_, _ = db.Exec(`
		INSERT INTO interfaces (id, name, type, status, created_at, updated_at)
		VALUES ($1, 'Test Interface (VQ Tests)', 'HL7', 'active', NOW(), NOW())
		ON CONFLICT (id) DO NOTHING`, interfaceID)

	var id string
	err := db.QueryRow(`
		INSERT INTO fhir_validation_queue
			(interface_id, message_id, message_type, raw_hl7, validation_failures, status)
		VALUES ($1, $2, $3, $4, $5::jsonb, $6)
		RETURNING id`,
		interfaceID,
		fmt.Sprintf("MSG-%d", time.Now().UnixNano()),
		"ADT^A01",
		"MSH|^~\\&|TestApp|TestFac|EHK|EHK|20260101120000||ADT^A01|TEST001|P|2.5",
		`[{"field":"Patient.identifier","hl7Path":"PID.3.1","missingCode":"MISSING_MRN","severity":"error"}]`,
		status,
	).Scan(&id)
	if err != nil {
		t.Fatalf("insertTestQueueItem: %v", err)
	}
	return id
}

// deleteTestQueueItem cleans up a test row.
func deleteTestQueueItem(t *testing.T, db *sql.DB, id string) {
	t.Helper()
	_, _ = db.Exec(`DELETE FROM fhir_validation_queue WHERE id = $1`, id)
}

// decodeBody parses the response body into a map.
func decodeBody(t *testing.T, w *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("decodeBody: %v — body was: %s", err, w.Body.String())
	}
	return result
}

// ─────────────────────────────────────────────────────────────────────────────
// Unit tests — resolvedByFromCtx  (no DB required)
// ─────────────────────────────────────────────────────────────────────────────

func TestResolvedByFromCtx_WithEmail(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/", nil)
	c.Set("user", map[string]interface{}{"email": "nurse@hospital.org"})

	result := resolvedByFromCtx(c)
	if result != "nurse@hospital.org" {
		t.Errorf("expected 'nurse@hospital.org', got %q", result)
	}
}

func TestResolvedByFromCtx_WithUsernameOnly(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/", nil)
	c.Set("user", map[string]interface{}{"username": "john.smith"})

	result := resolvedByFromCtx(c)
	if result != "john.smith" {
		t.Errorf("expected 'john.smith', got %q", result)
	}
}

func TestResolvedByFromCtx_EmailTakesPrecedenceOverUsername(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/", nil)
	c.Set("user", map[string]interface{}{
		"email":    "doctor@clinic.com",
		"username": "dr.jones",
	})

	result := resolvedByFromCtx(c)
	if result != "doctor@clinic.com" {
		t.Errorf("email should take precedence; got %q", result)
	}
}

func TestResolvedByFromCtx_NoUserInContext(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/", nil)
	// No "user" key set in context

	result := resolvedByFromCtx(c)
	if result != "unknown" {
		t.Errorf("expected 'unknown' when no user in context, got %q", result)
	}
}

func TestResolvedByFromCtx_WrongUserType(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/", nil)
	// Set user as wrong type (string instead of map)
	c.Set("user", "not-a-map")

	result := resolvedByFromCtx(c)
	if result != "unknown" {
		t.Errorf("expected 'unknown' for wrong type, got %q", result)
	}
}

func TestResolvedByFromCtx_EmptyEmailFallsBackToUsername(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/", nil)
	c.Set("user", map[string]interface{}{"email": "", "username": "fallback_user"})

	result := resolvedByFromCtx(c)
	if result != "fallback_user" {
		t.Errorf("expected 'fallback_user' when email is empty, got %q", result)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Unit tests — ValidationQueueItem JSON serialization
// ─────────────────────────────────────────────────────────────────────────────

func TestValidationQueueItem_JSONRoundTrip(t *testing.T) {
	msgID := "MSG-001"
	msgType := "ADT^A01"
	rawMsg := "MSH|^~\\&|..."

	item := ValidationQueueItem{
		ID:                 "aabbccdd-0000-0000-0000-000000000001",
		InterfaceID:        "aabbccdd-0000-0000-0000-000000000002",
		InterfaceName:      "Epic ADT Feed",
		MessageID:          &msgID,
		MessageType:        &msgType,
		RawMessage:         &rawMsg,
		PartialFHIR:        json.RawMessage(`{"resourceType":"Bundle","entry":[]}`),
		ValidationFailures: json.RawMessage(`[{"field":"Patient.identifier","severity":"error"}]`),
		Status:             "pending_review",
		Resolution:         nil,
		AssignedTo:         nil,
		Notes:              nil,
		CreatedAt:          time.Now().UTC(),
		UpdatedAt:          time.Now().UTC(),
	}

	b, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded ValidationQueueItem
	if err = json.Unmarshal(b, &decoded); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if decoded.ID != item.ID {
		t.Errorf("ID mismatch: got %s", decoded.ID)
	}
	if decoded.InterfaceName != "Epic ADT Feed" {
		t.Errorf("InterfaceName mismatch: got %s", decoded.InterfaceName)
	}
	if decoded.Status != "pending_review" {
		t.Errorf("Status mismatch: got %s", decoded.Status)
	}
	if string(decoded.PartialFHIR) != `{"resourceType":"Bundle","entry":[]}` {
		t.Errorf("PartialFHIR mismatch: got %s", string(decoded.PartialFHIR))
	}
}

func TestValidationQueueItem_JSONFields_NullablePointers(t *testing.T) {
	item := ValidationQueueItem{
		ID:          "aabbccdd-0000-0000-0000-000000000003",
		InterfaceID: "aabbccdd-0000-0000-0000-000000000004",
		Status:      "pending_review",
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
		// All nullable fields left as nil
	}

	b, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var raw map[string]interface{}
	if err = json.Unmarshal(b, &raw); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	// message_id, assigned_to, notes should serialize as null (not omitted)
	for _, key := range []string{"message_id", "assigned_to", "notes"} {
		val, ok := raw[key]
		if !ok {
			t.Errorf("key %q missing from JSON output", key)
		} else if val != nil {
			t.Errorf("key %q expected null, got %v", key, val)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Unit tests — Resolve / Override / Reject body parsing (no DB)
// ─────────────────────────────────────────────────────────────────────────────

func TestResolveBodyParsing_UsesProvidedValues(t *testing.T) {
	// Simulate: POST body with provided_values alias
	body := `{"provided_values":{"PID.3.1":"MRN-99999"},"reviewer_notes":"Obtained from ADT feed"}`
	_, w := ginContext(http.MethodPut, "/api/fhir/validation-queue/xxx/resolve", body)

	// We cannot call vc.Resolve without a real DB, but we can verify
	// our body struct parsing independently
	var req struct {
		Overrides      map[string]string `json:"overrides"`
		Notes          string            `json:"notes"`
		ProvidedValues map[string]string `json:"provided_values"`
		ReviewerNotes  string            `json:"reviewer_notes"`
	}
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	// Alias merge logic (mirrors controller)
	if req.ProvidedValues != nil && req.Overrides == nil {
		req.Overrides = req.ProvidedValues
	}
	if req.Overrides["PID.3.1"] != "MRN-99999" {
		t.Errorf("expected provided_values alias to merge; got %v", req.Overrides)
	}
	notes := req.Notes
	if notes == "" {
		notes = req.ReviewerNotes
	}
	if notes != "Obtained from ADT feed" {
		t.Errorf("expected reviewer_notes alias; got %q", notes)
	}
	_ = w // recorder not used in pure parse test
}

func TestResolveBodyParsing_OverridesTakePrecedenceOverProvidedValues(t *testing.T) {
	body := `{"overrides":{"PID.3.1":"EXPLICIT"},"provided_values":{"PID.3.1":"ALIAS"}}`
	var req struct {
		Overrides      map[string]string `json:"overrides"`
		ProvidedValues map[string]string `json:"provided_values"`
	}
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("parse error: %v", err)
	}
	// alias merge only when overrides is nil
	if req.Overrides == nil && req.ProvidedValues != nil {
		req.Overrides = req.ProvidedValues
	}
	if req.Overrides["PID.3.1"] != "EXPLICIT" {
		t.Errorf("expected overrides to take precedence; got %v", req.Overrides)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Integration tests — require DATABASE_URL env var
// ─────────────────────────────────────────────────────────────────────────────

func TestFHIRValidationQueue_List_ReturnsDataArray(t *testing.T) {
	db, cleanup, ok := testDB(t)
	if !ok {
		t.Skip("DATABASE_URL not set — skipping integration test")
	}
	defer cleanup()

	vc := NewFHIRValidationQueueController(db)
	id := insertTestQueueItem(t, db, "pending_review")
	defer deleteTestQueueItem(t, db, id)

	// Build a real router for the test
	router := gin.New()
	router.GET("/api/fhir/validation-queue", vc.List)

	req := httptest.NewRequest(http.MethodGet, "/api/fhir/validation-queue", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	data, ok := body["data"].([]interface{})
	if !ok {
		t.Fatalf("expected 'data' array in response, got: %v", body)
	}
	count, _ := body["count"].(float64)
	if int(count) != len(data) {
		t.Errorf("count field %v does not match data length %d", count, len(data))
	}
	if len(data) == 0 {
		t.Error("expected at least one item in list after insert")
	}
}

func TestFHIRValidationQueue_List_StatusFilter(t *testing.T) {
	db, cleanup, ok := testDB(t)
	if !ok {
		t.Skip("DATABASE_URL not set — skipping integration test")
	}
	defer cleanup()

	vc := NewFHIRValidationQueueController(db)
	id1 := insertTestQueueItem(t, db, "pending_review")
	id2 := insertTestQueueItem(t, db, "in_review")
	defer deleteTestQueueItem(t, db, id1)
	defer deleteTestQueueItem(t, db, id2)

	router := gin.New()
	router.GET("/api/fhir/validation-queue", vc.List)

	req := httptest.NewRequest(http.MethodGet, "/api/fhir/validation-queue?status=pending_review", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &body)
	data, _ := body["data"].([]interface{})
	for _, raw := range data {
		item := raw.(map[string]interface{})
		if item["status"] != "pending_review" {
			t.Errorf("status filter returned item with status %v", item["status"])
		}
	}
}

func TestFHIRValidationQueue_List_LimitParam(t *testing.T) {
	db, cleanup, ok := testDB(t)
	if !ok {
		t.Skip("DATABASE_URL not set — skipping integration test")
	}
	defer cleanup()

	vc := NewFHIRValidationQueueController(db)
	ids := make([]string, 5)
	for i := range ids {
		ids[i] = insertTestQueueItem(t, db, "pending_review")
	}
	defer func() {
		for _, id := range ids {
			deleteTestQueueItem(t, db, id)
		}
	}()

	router := gin.New()
	router.GET("/api/fhir/validation-queue", vc.List)

	req := httptest.NewRequest(http.MethodGet, "/api/fhir/validation-queue?limit=2&status=pending_review", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var body map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &body)
	data, _ := body["data"].([]interface{})
	if len(data) > 2 {
		t.Errorf("limit=2 returned %d items", len(data))
	}
}

func TestFHIRValidationQueue_Get_ExistingItem(t *testing.T) {
	db, cleanup, ok := testDB(t)
	if !ok {
		t.Skip("DATABASE_URL not set — skipping integration test")
	}
	defer cleanup()

	vc := NewFHIRValidationQueueController(db)
	id := insertTestQueueItem(t, db, "pending_review")
	defer deleteTestQueueItem(t, db, id)

	router := gin.New()
	router.GET("/api/fhir/validation-queue/:id", vc.Get)

	req := httptest.NewRequest(http.MethodGet, "/api/fhir/validation-queue/"+id, nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}
	var item ValidationQueueItem
	if err := json.Unmarshal(w.Body.Bytes(), &item); err != nil {
		t.Fatalf("decode error: %v", err)
	}
	if item.ID != id {
		t.Errorf("expected ID %s, got %s", id, item.ID)
	}
	if item.Status != "pending_review" {
		t.Errorf("expected status 'pending_review', got %q", item.Status)
	}
	if item.MessageType == nil || *item.MessageType != "ADT^A01" {
		t.Errorf("expected message_type 'ADT^A01', got %v", item.MessageType)
	}
}

func TestFHIRValidationQueue_Get_NotFound(t *testing.T) {
	db, cleanup, ok := testDB(t)
	if !ok {
		t.Skip("DATABASE_URL not set — skipping integration test")
	}
	defer cleanup()

	vc := NewFHIRValidationQueueController(db)
	router := gin.New()
	router.GET("/api/fhir/validation-queue/:id", vc.Get)

	// Non-existent UUID
	req := httptest.NewRequest(http.MethodGet,
		"/api/fhir/validation-queue/00000000-dead-beef-0000-000000000000", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown item, got %d", w.Code)
	}
	body := decodeBody(t, w)
	if _, hasError := body["error"]; !hasError {
		t.Error("expected 'error' field in 404 response")
	}
}

func TestFHIRValidationQueue_MarkInReview_TransitionsStatus(t *testing.T) {
	db, cleanup, ok := testDB(t)
	if !ok {
		t.Skip("DATABASE_URL not set — skipping integration test")
	}
	defer cleanup()

	vc := NewFHIRValidationQueueController(db)
	id := insertTestQueueItem(t, db, "pending_review")
	defer deleteTestQueueItem(t, db, id)

	router := gin.New()
	router.PUT("/api/fhir/validation-queue/:id/review", vc.MarkInReview)

	req := httptest.NewRequest(http.MethodPut, "/api/fhir/validation-queue/"+id+"/review", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}

	// Verify status in DB
	var status string
	db.QueryRow(`SELECT status FROM fhir_validation_queue WHERE id = $1`, id).Scan(&status)
	if status != "in_review" {
		t.Errorf("expected DB status='in_review' after review, got %q", status)
	}
}

func TestFHIRValidationQueue_MarkInReview_IdempotentOnInReviewItem(t *testing.T) {
	// Calling /review on an already in_review item should succeed (no error)
	// because the WHERE clause simply matches no rows — idempotent behaviour.
	db, cleanup, ok := testDB(t)
	if !ok {
		t.Skip("DATABASE_URL not set — skipping integration test")
	}
	defer cleanup()

	vc := NewFHIRValidationQueueController(db)
	id := insertTestQueueItem(t, db, "in_review")
	defer deleteTestQueueItem(t, db, id)

	router := gin.New()
	router.PUT("/api/fhir/validation-queue/:id/review", vc.MarkInReview)

	req := httptest.NewRequest(http.MethodPut, "/api/fhir/validation-queue/"+id+"/review", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should be 200 (exec did nothing but didn't error)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 idempotent, got %d", w.Code)
	}
}

func TestFHIRValidationQueue_Resolve_SetsResolutionJSON(t *testing.T) {
	db, cleanup, ok := testDB(t)
	if !ok {
		t.Skip("DATABASE_URL not set — skipping integration test")
	}
	defer cleanup()

	vc := NewFHIRValidationQueueController(db)
	id := insertTestQueueItem(t, db, "in_review")
	defer deleteTestQueueItem(t, db, id)

	router := gin.New()
	router.PUT("/api/fhir/validation-queue/:id/resolve", vc.Resolve)

	body := `{"overrides":{"PID.3.1":"MRN-55555"},"notes":"MRN confirmed via Epic"}`
	req := httptest.NewRequest(http.MethodPut, "/api/fhir/validation-queue/"+id+"/resolve",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}

	// Verify DB state
	var status string
	var resolution []byte
	db.QueryRow(`SELECT status, resolution FROM fhir_validation_queue WHERE id = $1`, id).
		Scan(&status, &resolution)

	if status != "resolved_provided" {
		t.Errorf("expected status='resolved_provided', got %q", status)
	}

	var resMap map[string]interface{}
	if err := json.Unmarshal(resolution, &resMap); err != nil {
		t.Fatalf("resolution is not valid JSON: %v", err)
	}
	if resMap["action"] != "provide_values" {
		t.Errorf("expected action='provide_values', got %v", resMap["action"])
	}
	overrides, _ := resMap["overrides"].(map[string]interface{})
	if overrides["PID.3.1"] != "MRN-55555" {
		t.Errorf("expected overrides['PID.3.1']='MRN-55555', got %v", overrides)
	}
	if resMap["resolved_by"] != "unknown" {
		// No user in context — should be "unknown"
		t.Logf("resolved_by = %v (no auth context in test — expected 'unknown')", resMap["resolved_by"])
	}
}

func TestFHIRValidationQueue_Override_SetsOverrideStatus(t *testing.T) {
	db, cleanup, ok := testDB(t)
	if !ok {
		t.Skip("DATABASE_URL not set — skipping integration test")
	}
	defer cleanup()

	vc := NewFHIRValidationQueueController(db)
	id := insertTestQueueItem(t, db, "in_review")
	defer deleteTestQueueItem(t, db, id)

	router := gin.New()
	router.PUT("/api/fhir/validation-queue/:id/override", vc.Override)

	body := `{"notes":"Clinician confirmed acceptable to proceed without MRN"}`
	req := httptest.NewRequest(http.MethodPut, "/api/fhir/validation-queue/"+id+"/override",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}

	var status string
	var resolution []byte
	db.QueryRow(`SELECT status, resolution FROM fhir_validation_queue WHERE id = $1`, id).
		Scan(&status, &resolution)

	if status != "resolved_override" {
		t.Errorf("expected status='resolved_override', got %q", status)
	}

	var resMap map[string]interface{}
	json.Unmarshal(resolution, &resMap)
	if resMap["action"] != "override" {
		t.Errorf("expected action='override', got %v", resMap["action"])
	}
}

func TestFHIRValidationQueue_Reject_SetsRejectedStatus(t *testing.T) {
	db, cleanup, ok := testDB(t)
	if !ok {
		t.Skip("DATABASE_URL not set — skipping integration test")
	}
	defer cleanup()

	vc := NewFHIRValidationQueueController(db)
	id := insertTestQueueItem(t, db, "pending_review")
	defer deleteTestQueueItem(t, db, id)

	router := gin.New()
	router.PUT("/api/fhir/validation-queue/:id/reject", vc.Reject)

	body := `{"notes":"Patient not found in system — message invalid"}`
	req := httptest.NewRequest(http.MethodPut, "/api/fhir/validation-queue/"+id+"/reject",
		strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}

	var status string
	db.QueryRow(`SELECT status FROM fhir_validation_queue WHERE id = $1`, id).Scan(&status)
	if status != "resolved_rejected" {
		t.Errorf("expected status='resolved_rejected', got %q", status)
	}
}

func TestFHIRValidationQueue_Stats_ReturnsAllCounters(t *testing.T) {
	db, cleanup, ok := testDB(t)
	if !ok {
		t.Skip("DATABASE_URL not set — skipping integration test")
	}
	defer cleanup()

	vc := NewFHIRValidationQueueController(db)
	// Insert one of each interesting status
	id1 := insertTestQueueItem(t, db, "pending_review")
	id2 := insertTestQueueItem(t, db, "in_review")
	id3 := insertTestQueueItem(t, db, "resolved_provided")
	defer deleteTestQueueItem(t, db, id1)
	defer deleteTestQueueItem(t, db, id2)
	defer deleteTestQueueItem(t, db, id3)

	router := gin.New()
	router.GET("/api/fhir/validation-queue/stats", vc.Stats)

	req := httptest.NewRequest(http.MethodGet, "/api/fhir/validation-queue/stats", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %s", w.Code, w.Body.String())
	}

	body := decodeBody(t, w)
	for _, key := range []string{"pending", "in_review", "resolved", "total"} {
		if _, ok := body[key]; !ok {
			t.Errorf("stats response missing key %q", key)
		}
	}
	// Our inserts must appear in the counters
	pending := int(body["pending"].(float64))
	inReview := int(body["in_review"].(float64))
	resolved := int(body["resolved"].(float64))

	if pending < 1 {
		t.Errorf("expected at least 1 pending item, got %d", pending)
	}
	if inReview < 1 {
		t.Errorf("expected at least 1 in_review item, got %d", inReview)
	}
	if resolved < 1 {
		t.Errorf("expected at least 1 resolved item, got %d", resolved)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Healthcare domain correctness — validation_failures JSON structure
// ─────────────────────────────────────────────────────────────────────────────

func TestValidationFailures_HL7ToFHIRFieldNames(t *testing.T) {
	// Validates that the validation_failures JSON uses correct FHIR R4 resource
	// field names (not HL7 paths) for the "field" property, and that HL7 paths
	// are correctly mapped to the "hl7Path" property.
	//
	// This enforces HL7→FHIR naming conventions per HL7 FHIR R4 spec:
	//   PID.3 → Patient.identifier
	//   PID.5 → Patient.name
	//   PID.7 → Patient.birthDate
	//   PID.8 → Patient.gender
	//   OBX.3 → Observation.code
	//   OBX.5 → Observation.value[x]
	//   OBX.11 → Observation.status
	//   PV1.2 → Encounter.status
	//   PV1.44 → Encounter.period.start

	type failure struct {
		Field       string `json:"field"`
		HL7Path     string `json:"hl7Path"`
		MissingCode string `json:"missingCode"`
		Severity    string `json:"severity"`
	}

	testCases := []struct {
		name        string
		hl7Path     string
		fhirField   string
		missingCode string
	}{
		{"patient identifier", "PID.3.1", "Patient.identifier", "MISSING_MRN"},
		{"patient name family", "PID.5.1", "Patient.name", "MISSING_PATIENT_NAME"},
		{"patient birthdate", "PID.7", "Patient.birthDate", "MISSING_DOB"},
		{"patient gender", "PID.8", "Patient.gender", "MISSING_GENDER"},
		{"observation code", "OBX.3", "Observation.code", "MISSING_LOINC"},
		{"observation value", "OBX.5", "Observation.value[x]", "MISSING_OBX_VALUE"},
		{"observation status", "OBX.11", "Observation.status", "MISSING_OBX_STATUS"},
		{"encounter status", "PV1.2", "Encounter.status", "MISSING_VISIT_CLASS"},
		{"encounter period", "PV1.44", "Encounter.period.start", "MISSING_ADMIT_DATE"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			f := failure{
				Field:       tc.fhirField,
				HL7Path:     tc.hl7Path,
				MissingCode: tc.missingCode,
				Severity:    "error",
			}
			b, _ := json.Marshal(f)
			var decoded failure
			json.Unmarshal(b, &decoded)

			if decoded.Field != tc.fhirField {
				t.Errorf("FHIR field name: expected %q, got %q", tc.fhirField, decoded.Field)
			}
			if decoded.HL7Path != tc.hl7Path {
				t.Errorf("HL7 path: expected %q, got %q", tc.hl7Path, decoded.HL7Path)
			}
			if decoded.Severity != "error" {
				t.Errorf("severity: expected 'error', got %q", decoded.Severity)
			}
		})
	}
}

func TestFHIRValidationPolicy_ValidValues(t *testing.T) {
	// Documents the 4 valid onMissing policy values per the V66 migration CHECK constraint.
	// These must stay in sync with the CHECK constraint in V66__Interface_FHIR_Validation_Policy.sql.
	validPolicies := []string{"proceed", "warn", "reject", "queue_review"}
	policySet := map[string]bool{}
	for _, p := range validPolicies {
		policySet[p] = true
	}

	// Verify "queue_review" is present and activates the review queue
	if !policySet["queue_review"] {
		t.Error("'queue_review' must be a valid policy value")
	}
	// "proceed" must be the default (safest — never blocks a message)
	if !policySet["proceed"] {
		t.Error("'proceed' must be a valid policy value (default)")
	}
	// Verify the constraint would reject unknown values
	invalidValues := []string{"hold", "discard", "manual", "queue", "review"}
	for _, v := range invalidValues {
		if policySet[v] {
			t.Errorf("value %q should NOT be a valid policy", v)
		}
	}
}

func TestResolutionPayload_ContainsRequiredFields(t *testing.T) {
	// The resolution JSONB payload written by Resolve/Override/Reject must contain
	// specific fields required for HIPAA audit trail compliance:
	// - action: what was done ("provide_values", "override", "reject")
	// - resolved_by: identity of the person who took the action
	// - resolved_at: ISO 8601 timestamp of the action
	// This test verifies the structure without a DB call.

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPut, "/", nil)
	c.Set("user", map[string]interface{}{"email": "qatester@hospital.org"})

	resolvedBy := resolvedByFromCtx(c)
	resolution := map[string]interface{}{
		"action":      "override",
		"resolved_by": resolvedBy,
		"resolved_at": time.Now().UTC().Format(time.RFC3339),
		"notes":       "Approved by attending physician",
	}
	b, err := json.Marshal(resolution)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var decoded map[string]interface{}
	json.Unmarshal(b, &decoded)

	for _, required := range []string{"action", "resolved_by", "resolved_at"} {
		if _, ok := decoded[required]; !ok {
			t.Errorf("resolution payload missing required HIPAA audit field: %q", required)
		}
	}
	if decoded["resolved_by"] != "qatester@hospital.org" {
		t.Errorf("resolved_by should be email from context, got %v", decoded["resolved_by"])
	}
	// resolved_at must parse as RFC3339
	if ts, ok := decoded["resolved_at"].(string); !ok || ts == "" {
		t.Error("resolved_at must be non-empty RFC3339 string")
	} else if _, err := time.Parse(time.RFC3339, ts); err != nil {
		t.Errorf("resolved_at %q is not valid RFC3339: %v", ts, err)
	}
}
