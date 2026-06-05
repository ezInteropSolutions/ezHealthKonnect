// controllers/dlq_controller_test.go
//
// Unit tests for DLQController — pure HTTP handler logic; no database.
// All DLQ operations are intercepted by mockDLQServicer.
//
// Test groups:
//   TC-DLQ-C001..C003  List endpoint
//   TC-DLQ-C004..C005  Stats endpoint
//   TC-DLQ-C006..C007  Get endpoint
//   TC-DLQ-C008..C009  Redrive endpoint
//   TC-DLQ-C010..C011  AbandonOne endpoint
//   TC-DLQ-C012..C013  BulkRedrive endpoint
//   TC-DLQ-C014..C015  BulkAbandon endpoint
//
// Run: go test ./controllers/ -v -run TestDLQController
package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ezhealthkonnect/services/connectors"

	"github.com/gin-gonic/gin"
)

// ─── Mock DLQServicer ────────────────────────────────────────────────────────

type mockDLQServicer struct {
	listRows    []*connectors.DLQRow
	listErr     error
	statsMap    map[string]int
	statsErr    error
	getRow      *connectors.DLQRow
	getErr      error
	redriveErr  error
	abandonErr  error
	bulkAffected int64
	bulkErr     error

	// capture call args
	lastListInterfaceID string
	lastListStatus      string
	lastListLimit       int
	lastListOffset      int
	lastGetID           string
	lastRedriveID       string
	lastRedriveMode     string
	lastAbandonID       string
	lastBulkIDs         []string
}

func (m *mockDLQServicer) ListPending(_ context.Context, interfaceID, _ /*messageID*/, status string, limit, offset int) ([]*connectors.DLQRow, error) {
	m.lastListInterfaceID = interfaceID
	m.lastListStatus = status
	m.lastListLimit = limit
	m.lastListOffset = offset
	return m.listRows, m.listErr
}

func (m *mockDLQServicer) Stats(_ context.Context) (map[string]int, error) {
	return m.statsMap, m.statsErr
}

func (m *mockDLQServicer) Get(_ context.Context, dlqID string) (*connectors.DLQRow, error) {
	m.lastGetID = dlqID
	return m.getRow, m.getErr
}

func (m *mockDLQServicer) ExecuteRedrive(_ context.Context, dlqID, modeOverride string, _ connectors.PipelineRedriver) error {
	m.lastRedriveID = dlqID
	m.lastRedriveMode = modeOverride
	return m.redriveErr
}

func (m *mockDLQServicer) Abandon(_ context.Context, dlqID string) error {
	m.lastAbandonID = dlqID
	return m.abandonErr
}

func (m *mockDLQServicer) BulkAbandon(_ context.Context, dlqIDs []string) (int64, error) {
	m.lastBulkIDs = dlqIDs
	return m.bulkAffected, m.bulkErr
}

func (m *mockDLQServicer) ScheduleRedrive(_ context.Context, _ string, _ string, _ time.Time) error {
	return nil
}

func (m *mockDLQServicer) BulkSchedule(_ context.Context, _ []string, _ string, _ time.Time) (int64, error) {
	return m.bulkAffected, m.bulkErr
}

func (m *mockDLQServicer) InterfaceStats(_ context.Context, _ string) (map[string]int, error) {
	return m.statsMap, m.statsErr
}

// ─── Router helper ───────────────────────────────────────────────────────────

// dlqRouter builds a minimal Gin router wired to dc with all DLQ routes.
func dlqRouter(dc *DLQController) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	rg := r.Group("/dlq")
	dc.RegisterRoutes(rg)
	return r
}

// do is a shorthand to fire a request against the router and return recorder + decoded body.
func do(r *gin.Engine, method, url, body string) (*httptest.ResponseRecorder, map[string]interface{}) {
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, url, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
	} else {
		req = httptest.NewRequest(method, url, nil)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var result map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &result) //nolint:errcheck
	return w, result
}

// ─── TC-DLQ-C001..C003: List ─────────────────────────────────────────────────

func TestDLQController_List_ReturnsRows(t *testing.T) {
	// TC-DLQ-C001: List returns rows from service with success:true and count.
	now := time.Now()
	mock := &mockDLQServicer{
		listRows: []*connectors.DLQRow{
			{ID: "row-1", MessageID: "msg-1", Status: "pending", CreatedAt: now},
			{ID: "row-2", MessageID: "msg-2", Status: "pending", CreatedAt: now},
		},
	}
	dc := NewDLQController(nil, mock)
	r := dlqRouter(dc)

	w, body := do(r, http.MethodGet, "/dlq?status=pending&limit=10&offset=0", "")

	if w.Code != http.StatusOK {
		t.Errorf("TC-DLQ-C001: status = %d, want 200", w.Code)
	}
	if body["success"] != true {
		t.Errorf("TC-DLQ-C001: success = %v, want true", body["success"])
	}
	count, _ := body["count"].(float64)
	if count != 2 {
		t.Errorf("TC-DLQ-C001: count = %v, want 2", count)
	}
	// Verify query params forwarded
	if mock.lastListStatus != "pending" {
		t.Errorf("TC-DLQ-C001: lastListStatus = %q, want \"pending\"", mock.lastListStatus)
	}
	if mock.lastListLimit != 10 {
		t.Errorf("TC-DLQ-C001: lastListLimit = %d, want 10", mock.lastListLimit)
	}
}

func TestDLQController_List_ServiceError_Returns500(t *testing.T) {
	// TC-DLQ-C002: List propagates a service error as HTTP 500.
	mock := &mockDLQServicer{listErr: errors.New("db timeout")}
	dc := NewDLQController(nil, mock)
	r := dlqRouter(dc)

	w, body := do(r, http.MethodGet, "/dlq", "")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("TC-DLQ-C002: status = %d, want 500", w.Code)
	}
	if body["success"] != false {
		t.Errorf("TC-DLQ-C002: success = %v, want false", body["success"])
	}
}

func TestDLQController_List_NilRows_ReturnsEmptyArray(t *testing.T) {
	// TC-DLQ-C003: nil rows from service must become an empty JSON array (not null).
	mock := &mockDLQServicer{listRows: nil}
	dc := NewDLQController(nil, mock)
	r := dlqRouter(dc)

	w, _ := do(r, http.MethodGet, "/dlq", "")

	if w.Code != http.StatusOK {
		t.Errorf("TC-DLQ-C003: status = %d, want 200", w.Code)
	}
	// The body must contain "data":[] not "data":null
	if !strings.Contains(w.Body.String(), `"data":[]`) {
		t.Errorf("TC-DLQ-C003: body does not contain \"data\":[], got: %s", w.Body.String())
	}
}

// ─── TC-DLQ-C004..C005: Stats ────────────────────────────────────────────────

func TestDLQController_Stats_ReturnsCounts(t *testing.T) {
	// TC-DLQ-C004: Stats returns the map from the service.
	mock := &mockDLQServicer{
		statsMap: map[string]int{"pending": 5, "resolved": 12, "abandoned": 3},
	}
	dc := NewDLQController(nil, mock)
	r := dlqRouter(dc)

	w, body := do(r, http.MethodGet, "/dlq/stats", "")

	if w.Code != http.StatusOK {
		t.Errorf("TC-DLQ-C004: status = %d, want 200", w.Code)
	}
	data, _ := body["data"].(map[string]interface{})
	if int(data["pending"].(float64)) != 5 {
		t.Errorf("TC-DLQ-C004: pending = %v, want 5", data["pending"])
	}
}

func TestDLQController_Stats_ServiceError_Returns500(t *testing.T) {
	// TC-DLQ-C005: Stats propagates a service error as HTTP 500.
	mock := &mockDLQServicer{statsErr: errors.New("connection refused")}
	dc := NewDLQController(nil, mock)
	r := dlqRouter(dc)

	w, body := do(r, http.MethodGet, "/dlq/stats", "")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("TC-DLQ-C005: status = %d, want 500", w.Code)
	}
	if body["success"] != false {
		t.Errorf("TC-DLQ-C005: success = %v, want false", body["success"])
	}
}

// ─── TC-DLQ-C006..C007: Get ──────────────────────────────────────────────────

func TestDLQController_Get_Found_Returns200(t *testing.T) {
	// TC-DLQ-C006: Get returns 200 and the row when found.
	now := time.Now()
	mock := &mockDLQServicer{
		getRow: &connectors.DLQRow{
			ID: "abc-123", MessageID: "msg-42", Status: "pending", CreatedAt: now,
		},
	}
	dc := NewDLQController(nil, mock)
	r := dlqRouter(dc)

	w, body := do(r, http.MethodGet, "/dlq/abc-123", "")

	if w.Code != http.StatusOK {
		t.Errorf("TC-DLQ-C006: status = %d, want 200", w.Code)
	}
	if body["success"] != true {
		t.Errorf("TC-DLQ-C006: success = %v, want true", body["success"])
	}
	if mock.lastGetID != "abc-123" {
		t.Errorf("TC-DLQ-C006: lastGetID = %q, want \"abc-123\"", mock.lastGetID)
	}
}

func TestDLQController_Get_NotFound_Returns404(t *testing.T) {
	// TC-DLQ-C007: Get returns 404 when service returns error.
	mock := &mockDLQServicer{getErr: errors.New("dlq row xyz not found")}
	dc := NewDLQController(nil, mock)
	r := dlqRouter(dc)

	w, body := do(r, http.MethodGet, "/dlq/xyz", "")

	if w.Code != http.StatusNotFound {
		t.Errorf("TC-DLQ-C007: status = %d, want 404", w.Code)
	}
	if body["success"] != false {
		t.Errorf("TC-DLQ-C007: success = %v, want false", body["success"])
	}
}

// ─── TC-DLQ-C008..C009: Redrive ──────────────────────────────────────────────

func TestDLQController_Redrive_Success_Returns200(t *testing.T) {
	// TC-DLQ-C008: Redrive success returns 200 and forwards the mode.
	mock := &mockDLQServicer{}
	dc := NewDLQController(nil, mock)
	r := dlqRouter(dc)

	w, body := do(r, http.MethodPost, "/dlq/row-99/redrive", `{"mode":"from_start"}`)

	if w.Code != http.StatusOK {
		t.Errorf("TC-DLQ-C008: status = %d, want 200", w.Code)
	}
	if body["success"] != true {
		t.Errorf("TC-DLQ-C008: success = %v, want true", body["success"])
	}
	if mock.lastRedriveID != "row-99" {
		t.Errorf("TC-DLQ-C008: lastRedriveID = %q, want \"row-99\"", mock.lastRedriveID)
	}
	if mock.lastRedriveMode != "from_start" {
		t.Errorf("TC-DLQ-C008: lastRedriveMode = %q, want \"from_start\"", mock.lastRedriveMode)
	}
}

func TestDLQController_Redrive_ServiceError_Returns500(t *testing.T) {
	// TC-DLQ-C009: Redrive returns 500 when service fails.
	mock := &mockDLQServicer{redriveErr: errors.New("no pipeline configured")}
	dc := NewDLQController(nil, mock)
	r := dlqRouter(dc)

	w, body := do(r, http.MethodPost, "/dlq/row-1/redrive", `{}`)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("TC-DLQ-C009: status = %d, want 500", w.Code)
	}
	if body["success"] != false {
		t.Errorf("TC-DLQ-C009: success = %v, want false", body["success"])
	}
}

// ─── TC-DLQ-C010..C011: AbandonOne ───────────────────────────────────────────

func TestDLQController_AbandonOne_Success_Returns200(t *testing.T) {
	// TC-DLQ-C010: AbandonOne returns 200 on success.
	mock := &mockDLQServicer{}
	dc := NewDLQController(nil, mock)
	r := dlqRouter(dc)

	w, body := do(r, http.MethodPost, "/dlq/row-55/abandon", "")

	if w.Code != http.StatusOK {
		t.Errorf("TC-DLQ-C010: status = %d, want 200", w.Code)
	}
	if body["success"] != true {
		t.Errorf("TC-DLQ-C010: success = %v, want true", body["success"])
	}
	if mock.lastAbandonID != "row-55" {
		t.Errorf("TC-DLQ-C010: lastAbandonID = %q, want \"row-55\"", mock.lastAbandonID)
	}
}

func TestDLQController_AbandonOne_ServiceError_Returns500(t *testing.T) {
	// TC-DLQ-C011: AbandonOne returns 500 when service fails.
	mock := &mockDLQServicer{abandonErr: errors.New("row not found")}
	dc := NewDLQController(nil, mock)
	r := dlqRouter(dc)

	w, body := do(r, http.MethodPost, "/dlq/row-99/abandon", "")

	if w.Code != http.StatusInternalServerError {
		t.Errorf("TC-DLQ-C011: status = %d, want 500", w.Code)
	}
	if body["success"] != false {
		t.Errorf("TC-DLQ-C011: success = %v, want false", body["success"])
	}
}

// ─── TC-DLQ-C012..C013: BulkRedrive ─────────────────────────────────────────

func TestDLQController_BulkRedrive_EmptyIDs_Returns400(t *testing.T) {
	// TC-DLQ-C012: BulkRedrive with empty ids array returns 400.
	mock := &mockDLQServicer{}
	dc := NewDLQController(nil, mock)
	r := dlqRouter(dc)

	w, body := do(r, http.MethodPost, "/dlq/bulk-redrive", `{"ids":[]}`)

	if w.Code != http.StatusBadRequest {
		t.Errorf("TC-DLQ-C012: status = %d, want 400", w.Code)
	}
	if body["success"] != false {
		t.Errorf("TC-DLQ-C012: success = %v, want false", body["success"])
	}
}

func TestDLQController_BulkRedrive_MultipleIDs_ReportsEach(t *testing.T) {
	// TC-DLQ-C013: BulkRedrive calls ExecuteRedrive for each ID and reports results.
	// First ID succeeds, second ID fails.
	callCount := 0
	var callIDs []string
	mock := &mockDLQServicer{}
	mock.redriveErr = nil // default success

	// We need a custom mock that returns error for specific IDs.
	// Use a closure-based approach by wrapping — but since mockDLQServicer is a simple
	// struct, we set a sequence of errors via a counter tracked in the captured variable.
	dc := NewDLQController(nil, &mockDLQServicerSequenced{
		redriveErrs: map[string]error{
			"id-fail": errors.New("timeout"),
		},
		callIDs: &callIDs,
		callCount: &callCount,
	})
	r := dlqRouter(dc)

	w, body := do(r, http.MethodPost, "/dlq/bulk-redrive", `{"ids":["id-ok","id-fail"],"mode":"from_start"}`)

	if w.Code != http.StatusOK {
		t.Errorf("TC-DLQ-C013: status = %d, want 200", w.Code)
	}
	if body["success"] != true {
		t.Errorf("TC-DLQ-C013: success = %v, want true", body["success"])
	}
	data, _ := body["data"].([]interface{})
	if len(data) != 2 {
		t.Errorf("TC-DLQ-C013: data len = %d, want 2", len(data))
	}
	// Verify id-ok succeeded and id-fail failed
	for _, entry := range data {
		row, _ := entry.(map[string]interface{})
		if row["id"] == "id-ok" && row["success"] != true {
			t.Errorf("TC-DLQ-C013: id-ok should succeed, got success=%v", row["success"])
		}
		if row["id"] == "id-fail" && row["success"] != false {
			t.Errorf("TC-DLQ-C013: id-fail should fail, got success=%v", row["success"])
		}
	}
}

// mockDLQServicerSequenced lets different IDs return different errors for BulkRedrive testing.
type mockDLQServicerSequenced struct {
	redriveErrs map[string]error
	callIDs     *[]string
	callCount   *int
}

func (m *mockDLQServicerSequenced) ListPending(_ context.Context, _, _, _ string, _, _ int) ([]*connectors.DLQRow, error) {
	return nil, nil
}
func (m *mockDLQServicerSequenced) Stats(_ context.Context) (map[string]int, error) {
	return nil, nil
}
func (m *mockDLQServicerSequenced) Get(_ context.Context, _ string) (*connectors.DLQRow, error) {
	return nil, nil
}
func (m *mockDLQServicerSequenced) ExecuteRedrive(_ context.Context, dlqID, _ string, _ connectors.PipelineRedriver) error {
	*m.callCount++
	*m.callIDs = append(*m.callIDs, dlqID)
	if err, ok := m.redriveErrs[dlqID]; ok {
		return err
	}
	return nil
}
func (m *mockDLQServicerSequenced) Abandon(_ context.Context, _ string) error    { return nil }
func (m *mockDLQServicerSequenced) BulkAbandon(_ context.Context, _ []string) (int64, error) {
	return 0, nil
}
func (m *mockDLQServicerSequenced) ScheduleRedrive(_ context.Context, _ string, _ string, _ time.Time) error {
	return nil
}

func (m *mockDLQServicerSequenced) BulkSchedule(_ context.Context, _ []string, _ string, _ time.Time) (int64, error) {
	return 0, nil
}

func (m *mockDLQServicerSequenced) InterfaceStats(_ context.Context, _ string) (map[string]int, error) {
	return nil, nil
}

// ─── TC-DLQ-C014..C015: BulkAbandon ─────────────────────────────────────────

func TestDLQController_BulkAbandon_EmptyIDs_Returns400(t *testing.T) {
	// TC-DLQ-C014: BulkAbandon with empty ids returns 400.
	mock := &mockDLQServicer{}
	dc := NewDLQController(nil, mock)
	r := dlqRouter(dc)

	w, body := do(r, http.MethodPost, "/dlq/bulk-abandon", `{"ids":[]}`)

	if w.Code != http.StatusBadRequest {
		t.Errorf("TC-DLQ-C014: status = %d, want 400", w.Code)
	}
	if body["success"] != false {
		t.Errorf("TC-DLQ-C014: success = %v, want false", body["success"])
	}
}

func TestDLQController_BulkAbandon_ValidIDs_ReturnsAffected(t *testing.T) {
	// TC-DLQ-C015: BulkAbandon with valid ids returns affected count.
	mock := &mockDLQServicer{bulkAffected: 3}
	dc := NewDLQController(nil, mock)
	r := dlqRouter(dc)

	w, body := do(r, http.MethodPost, "/dlq/bulk-abandon", `{"ids":["a","b","c"]}`)

	if w.Code != http.StatusOK {
		t.Errorf("TC-DLQ-C015: status = %d, want 200", w.Code)
	}
	if body["success"] != true {
		t.Errorf("TC-DLQ-C015: success = %v, want true", body["success"])
	}
	affected, _ := body["affected"].(float64)
	if affected != 3 {
		t.Errorf("TC-DLQ-C015: affected = %v, want 3", affected)
	}
	if len(mock.lastBulkIDs) != 3 {
		t.Errorf("TC-DLQ-C015: lastBulkIDs len = %d, want 3", len(mock.lastBulkIDs))
	}
}

func TestDLQController_BulkAbandon_ServiceError_Returns500(t *testing.T) {
	// TC-DLQ-C016: BulkAbandon returns 500 when service fails.
	mock := &mockDLQServicer{bulkErr: errors.New("db error")}
	dc := NewDLQController(nil, mock)
	r := dlqRouter(dc)

	w, body := do(r, http.MethodPost, "/dlq/bulk-abandon", `{"ids":["x","y"]}`)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("TC-DLQ-C016: status = %d, want 500", w.Code)
	}
	if body["success"] != false {
		t.Errorf("TC-DLQ-C016: success = %v, want false", body["success"])
	}
}

// ─── TC-DLQ-C017: List default params ────────────────────────────────────────

func TestDLQController_List_DefaultStatus_IsPending(t *testing.T) {
	// TC-DLQ-C017: List without ?status= defaults to "pending".
	mock := &mockDLQServicer{listRows: []*connectors.DLQRow{}}
	dc := NewDLQController(nil, mock)
	r := dlqRouter(dc)

	do(r, http.MethodGet, "/dlq", "")

	if mock.lastListStatus != "pending" {
		t.Errorf("TC-DLQ-C017: default status = %q, want \"pending\"", mock.lastListStatus)
	}
	if mock.lastListLimit != 50 {
		t.Errorf("TC-DLQ-C017: default limit = %d, want 50", mock.lastListLimit)
	}
}

func TestDLQController_List_InterfaceIDFilter_Forwarded(t *testing.T) {
	// TC-DLQ-C018: List with ?interface_id= forwards the value to service.
	mock := &mockDLQServicer{listRows: []*connectors.DLQRow{}}
	dc := NewDLQController(nil, mock)
	r := dlqRouter(dc)

	do(r, http.MethodGet, "/dlq?interface_id=iface-abc", "")

	if mock.lastListInterfaceID != "iface-abc" {
		t.Errorf("TC-DLQ-C018: lastListInterfaceID = %q, want \"iface-abc\"", mock.lastListInterfaceID)
	}
}
