// processing/phi_safety_test.go
// Unit tests for the PHI Safety port-conflict halt helpers.

package processing

import (
	"errors"
	"testing"
)

// ─── isPortConflictError ─────────────────────────────────────────────────────

func TestIsPortConflictError_BindError(t *testing.T) {
	err := errors.New("listen tcp :6610: bind: address already in use")
	if !isPortConflictError(err) {
		t.Error("expected true for 'address already in use'")
	}
}

func TestIsPortConflictError_EADDRINUSEVariant(t *testing.T) {
	err := errors.New("listen tcp :2575: EADDRINUSE")
	if !isPortConflictError(err) {
		t.Error("expected true for EADDRINUSE")
	}
}

func TestIsPortConflictError_BindAddressVariant(t *testing.T) {
	err := errors.New("bind: address in use")
	if !isPortConflictError(err) {
		t.Error("expected true for 'bind: address'")
	}
}

func TestIsPortConflictError_NilError(t *testing.T) {
	if isPortConflictError(nil) {
		t.Error("expected false for nil error")
	}
}

func TestIsPortConflictError_UnrelatedError(t *testing.T) {
	err := errors.New("connection refused")
	if isPortConflictError(err) {
		t.Error("expected false for 'connection refused'")
	}
}

func TestIsPortConflictError_TimeoutError(t *testing.T) {
	err := errors.New("i/o timeout")
	if isPortConflictError(err) {
		t.Error("expected false for timeout error")
	}
}

// ─── extractConnectorPort ────────────────────────────────────────────────────

func TestExtractConnectorPort_Float64(t *testing.T) {
	cfg := map[string]interface{}{"port": float64(6610)}
	if p := extractConnectorPort(cfg); p != 6610 {
		t.Errorf("expected 6610, got %d", p)
	}
}

func TestExtractConnectorPort_Int(t *testing.T) {
	cfg := map[string]interface{}{"port": 2575}
	if p := extractConnectorPort(cfg); p != 2575 {
		t.Errorf("expected 2575, got %d", p)
	}
}

func TestExtractConnectorPort_Int64(t *testing.T) {
	cfg := map[string]interface{}{"port": int64(1433)}
	if p := extractConnectorPort(cfg); p != 1433 {
		t.Errorf("expected 1433, got %d", p)
	}
}

func TestExtractConnectorPort_Missing(t *testing.T) {
	cfg := map[string]interface{}{"host": "localhost"}
	if p := extractConnectorPort(cfg); p != 0 {
		t.Errorf("expected 0 for missing port, got %d", p)
	}
}

func TestExtractConnectorPort_NilConfig(t *testing.T) {
	if p := extractConnectorPort(nil); p != 0 {
		t.Errorf("expected 0 for nil config, got %d", p)
	}
}

func TestExtractConnectorPort_StringValue(t *testing.T) {
	// String port values should return 0 (unsupported type)
	cfg := map[string]interface{}{"port": "6610"}
	if p := extractConnectorPort(cfg); p != 0 {
		t.Errorf("expected 0 for string port, got %d", p)
	}
}
