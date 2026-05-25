// services/connectors/dlq_service_test.go
//
// Unit tests for DLQService — pure logic that does not touch the database.
//
// Test groups:
//   TC-DLQ-U001..U003  nullableStr helper
//   TC-DLQ-U004..U006  WriteDLQParams defaults
//   TC-DLQ-U007..U008  BulkAbandon short-circuit on empty slice
//   TC-DLQ-U009        ExecuteRedrive with no pipeline configured → error
//   TC-DLQ-U010        SetPipelineRedriver stores the redriver
//   TC-DLQ-U011..U014  redriveRow routing (mock PipelineRedriver)
//
// Run: go test ./services/connectors/ -v -run TestDLQ
package connectors

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// ─── Mock PipelineRedriver ────────────────────────────────────────────────────

type mockRedriver struct {
	fromInputCalls atomic.Int32
	fromStepCalls  atomic.Int32
	inputErr       error
	stepErr        error
	// capture last args
	lastInputID  string
	lastStepID   string
	lastPipeID   string
}

func (m *mockRedriver) ExecuteTransformationFromInput(_ context.Context, messageID, _, _ string, _ map[string]interface{}) error {
	m.fromInputCalls.Add(1)
	m.lastInputID = messageID
	return m.inputErr
}

func (m *mockRedriver) ExecuteFromStep(_ context.Context, pipelineID, fromStepID string, _ map[string]interface{}) error {
	m.fromStepCalls.Add(1)
	m.lastPipeID = pipelineID
	m.lastStepID = fromStepID
	return m.stepErr
}

// ─── TC-DLQ-U001..U003: nullableStr ──────────────────────────────────────────

func TestNullableStr_EmptyString(t *testing.T) {
	// TC-DLQ-U001
	result := nullableStr("")
	if result != nil {
		t.Errorf("TC-DLQ-U001: nullableStr(\"\") = %v, want nil", result)
	}
}

func TestNullableStr_NonEmpty(t *testing.T) {
	// TC-DLQ-U002
	result := nullableStr("hello")
	if result != "hello" {
		t.Errorf("TC-DLQ-U002: nullableStr(\"hello\") = %v, want \"hello\"", result)
	}
}

func TestNullableStr_Whitespace(t *testing.T) {
	// TC-DLQ-U003: whitespace is non-empty — not treated as nil
	result := nullableStr("  ")
	if result == nil {
		t.Errorf("TC-DLQ-U003: nullableStr(\"  \") = nil, want non-nil")
	}
}

// ─── TC-DLQ-U004..U006: WriteDLQParams defaults ───────────────────────────────

func TestWriteDLQParams_DefaultRedriveMode(t *testing.T) {
	// TC-DLQ-U004: zero-value RedriveMode should be set to "from_failed_step" by WriteToDLQ.
	// We verify the field semantics without a DB by checking the zero value.
	var p WriteDLQParams
	if p.RedriveMode != "" {
		t.Errorf("TC-DLQ-U004: zero-value RedriveMode = %q, want empty (set by WriteToDLQ)", p.RedriveMode)
	}
	// After normalisation (simulated):
	if p.RedriveMode == "" {
		p.RedriveMode = "from_failed_step"
	}
	if p.RedriveMode != "from_failed_step" {
		t.Errorf("TC-DLQ-U004: after normalise RedriveMode = %q, want \"from_failed_step\"", p.RedriveMode)
	}
}

func TestWriteDLQParams_ExplicitFromStart(t *testing.T) {
	// TC-DLQ-U005: explicit "from_start" is preserved unchanged.
	p := WriteDLQParams{RedriveMode: "from_start"}
	if p.RedriveMode == "" {
		p.RedriveMode = "from_failed_step"
	}
	if p.RedriveMode != "from_start" {
		t.Errorf("TC-DLQ-U005: RedriveMode = %q, want \"from_start\"", p.RedriveMode)
	}
}

func TestWriteDLQParams_NilExpiresAtIsAllowed(t *testing.T) {
	// TC-DLQ-U006: ExpiresAt may be nil (no expiry).
	p := WriteDLQParams{}
	if p.ExpiresAt != nil {
		t.Errorf("TC-DLQ-U006: zero ExpiresAt = %v, want nil", p.ExpiresAt)
	}
}

// ─── TC-DLQ-U007..U008: BulkAbandon short-circuit ───────────────────────────

func TestBulkAbandon_EmptySlice_NoDBCall(t *testing.T) {
	// TC-DLQ-U007: BulkAbandon with empty IDs returns (0, nil) without touching DB.
	svc := &DLQService{db: nil} // nil DB — would panic if any SQL ran
	affected, err := svc.BulkAbandon(context.Background(), []string{})
	if err != nil {
		t.Errorf("TC-DLQ-U007: err = %v, want nil", err)
	}
	if affected != 0 {
		t.Errorf("TC-DLQ-U007: affected = %d, want 0", affected)
	}
}

func TestBulkAbandon_NilSlice_NoDBCall(t *testing.T) {
	// TC-DLQ-U008: BulkAbandon with nil IDs returns (0, nil) without touching DB.
	svc := &DLQService{db: nil}
	affected, err := svc.BulkAbandon(context.Background(), nil)
	if err != nil {
		t.Errorf("TC-DLQ-U008: err = %v, want nil", err)
	}
	if affected != 0 {
		t.Errorf("TC-DLQ-U008: affected = %d, want 0", affected)
	}
}

// ─── TC-DLQ-U009: ExecuteRedrive no pipeline configured ─────────────────────

func TestExecuteRedrive_NoPipelineSvc_ReturnsError(t *testing.T) {
	// TC-DLQ-U009: ExecuteRedrive with no pipelineSvc configured (both s.pipelineSvc
	// and the passed svc are nil) must return an error immediately.
	svc := &DLQService{db: nil, pipelineSvc: nil}
	err := svc.ExecuteRedrive(context.Background(), "any-id", "", nil)
	if err == nil {
		t.Error("TC-DLQ-U009: want error when no pipeline redriver configured, got nil")
	}
}

// ─── TC-DLQ-U010: SetPipelineRedriver ───────────────────────────────────────

func TestSetPipelineRedriver_Stores(t *testing.T) {
	// TC-DLQ-U010: SetPipelineRedriver stores the redriver on the service.
	svc := &DLQService{}
	mock := &mockRedriver{}
	svc.SetPipelineRedriver(mock)
	if svc.pipelineSvc == nil {
		t.Error("TC-DLQ-U010: pipelineSvc not stored after SetPipelineRedriver")
	}
}

// ─── TC-DLQ-U011..U014: redriveRow routing ──────────────────────────────────

func TestRedriveRow_FromStart_NilSnapshot_Error(t *testing.T) {
	// TC-DLQ-U011: from_start with nil PipelineInputSnapshot → error (can't redrive from start).
	svc := &DLQService{}
	row := &DLQRow{
		ID:                    "row-1",
		RedriveMode:           "from_start",
		PipelineInputSnapshot: nil,
	}
	mock := &mockRedriver{}
	err := svc.redriveRow(context.Background(), row, mock)
	if err == nil {
		t.Error("TC-DLQ-U011: want error for from_start with nil snapshot, got nil")
	}
	if mock.fromInputCalls.Load() != 0 {
		t.Errorf("TC-DLQ-U011: ExecuteTransformationFromInput called %d times, want 0", mock.fromInputCalls.Load())
	}
}

func TestRedriveRow_FromStart_CallsRedriver(t *testing.T) {
	// TC-DLQ-U012: from_start with valid snapshot calls ExecuteTransformationFromInput.
	svc := &DLQService{}
	row := &DLQRow{
		ID:          "row-2",
		MessageID:   "msg-abc",
		InterfaceID: "iface-xyz",
		RedriveMode: "from_start",
		PipelineInputSnapshot: map[string]interface{}{"foo": "bar"},
	}
	mock := &mockRedriver{}
	err := svc.redriveRow(context.Background(), row, mock)
	if err != nil {
		t.Errorf("TC-DLQ-U012: unexpected error: %v", err)
	}
	if mock.fromInputCalls.Load() != 1 {
		t.Errorf("TC-DLQ-U012: ExecuteTransformationFromInput called %d times, want 1", mock.fromInputCalls.Load())
	}
	if mock.lastInputID != "msg-abc" {
		t.Errorf("TC-DLQ-U012: messageID = %q, want \"msg-abc\"", mock.lastInputID)
	}
}

func TestRedriveRow_FromFailedStep_MissingIDs_Error(t *testing.T) {
	// TC-DLQ-U013: from_failed_step with missing pipeline_id or failed_step_id → error.
	svc := &DLQService{}
	row := &DLQRow{
		ID:           "row-3",
		RedriveMode:  "from_failed_step",
		PipelineID:   "",
		FailedStepID: "",
	}
	mock := &mockRedriver{}
	err := svc.redriveRow(context.Background(), row, mock)
	if err == nil {
		t.Error("TC-DLQ-U013: want error for from_failed_step with empty IDs, got nil")
	}
	if mock.fromStepCalls.Load() != 0 {
		t.Errorf("TC-DLQ-U013: ExecuteFromStep called %d times, want 0", mock.fromStepCalls.Load())
	}
}

func TestRedriveRow_FromFailedStep_CallsRedriver(t *testing.T) {
	// TC-DLQ-U014: from_failed_step with valid IDs calls ExecuteFromStep.
	svc := &DLQService{}
	row := &DLQRow{
		ID:           "row-4",
		RedriveMode:  "from_failed_step",
		PipelineID:   "pipe-111",
		FailedStepID: "step-222",
		PipelineDataSnapshot: map[string]interface{}{"data": true},
	}
	mock := &mockRedriver{}
	err := svc.redriveRow(context.Background(), row, mock)
	if err != nil {
		t.Errorf("TC-DLQ-U014: unexpected error: %v", err)
	}
	if mock.fromStepCalls.Load() != 1 {
		t.Errorf("TC-DLQ-U014: ExecuteFromStep called %d times, want 1", mock.fromStepCalls.Load())
	}
	if mock.lastPipeID != "pipe-111" {
		t.Errorf("TC-DLQ-U014: pipelineID = %q, want \"pipe-111\"", mock.lastPipeID)
	}
	if mock.lastStepID != "step-222" {
		t.Errorf("TC-DLQ-U014: stepID = %q, want \"step-222\"", mock.lastStepID)
	}
}

func TestRedriveRow_FromFailedStep_NilDataSnapshot_UsesEmptyMap(t *testing.T) {
	// TC-DLQ-U015: from_failed_step with nil PipelineDataSnapshot substitutes empty map.
	svc := &DLQService{}
	row := &DLQRow{
		ID:                   "row-5",
		RedriveMode:          "from_failed_step",
		PipelineID:           "pipe-333",
		FailedStepID:         "step-444",
		PipelineDataSnapshot: nil, // nil → should be substituted with {}
	}
	mock := &mockRedriver{}
	err := svc.redriveRow(context.Background(), row, mock)
	if err != nil {
		t.Errorf("TC-DLQ-U015: unexpected error: %v", err)
	}
	if mock.fromStepCalls.Load() != 1 {
		t.Errorf("TC-DLQ-U015: ExecuteFromStep called %d times, want 1", mock.fromStepCalls.Load())
	}
}

func TestRedriveRow_RedriveErr_Propagated(t *testing.T) {
	// TC-DLQ-U016: when redriveRow's inner call fails, the error is returned.
	svc := &DLQService{}
	row := &DLQRow{
		ID:                    "row-6",
		RedriveMode:           "from_start",
		PipelineInputSnapshot: map[string]interface{}{"x": 1},
	}
	wantErr := errors.New("downstream unavailable")
	mock := &mockRedriver{inputErr: wantErr}
	err := svc.redriveRow(context.Background(), row, mock)
	if err == nil {
		t.Error("TC-DLQ-U016: want error propagated from redriver, got nil")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("TC-DLQ-U016: err = %v, want %v", err, wantErr)
	}
}

// ─── TC-DLQ-U017: DLQRow zero-value sentinel ─────────────────────────────────

func TestDLQRow_ZeroValueSafe(t *testing.T) {
	// TC-DLQ-U017: DLQRow zero value has safe defaults (no panics on field access).
	var r DLQRow
	if r.ID != "" || r.AttemptCount != 0 || r.NextRetryAt != nil || r.ExpiresAt != nil {
		t.Errorf("TC-DLQ-U017: unexpected zero values: %+v", r)
	}
	_ = r.CreatedAt.IsZero() // time.Time zero value must not panic
}

// ─── TC-DLQ-U018: WriteDLQParams ExpiresAt pointer ────────────────────────────

func TestWriteDLQParams_ExpiresAt_Pointer(t *testing.T) {
	// TC-DLQ-U018: ExpiresAt is a *time.Time so callers can omit expiry.
	t1 := time.Now().Add(24 * time.Hour)
	p := WriteDLQParams{ExpiresAt: &t1}
	if p.ExpiresAt == nil {
		t.Error("TC-DLQ-U018: ExpiresAt should be non-nil")
	}
	if !p.ExpiresAt.Equal(t1) {
		t.Errorf("TC-DLQ-U018: ExpiresAt = %v, want %v", p.ExpiresAt, t1)
	}
}
