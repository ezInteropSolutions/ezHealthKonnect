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
	p, ok := extractConnectorPort(cfg)
	if !ok || p != 6610 {
		t.Errorf("expected (6610, true), got (%d, %v)", p, ok)
	}
}

func TestExtractConnectorPort_Int(t *testing.T) {
	cfg := map[string]interface{}{"port": 2575}
	p, ok := extractConnectorPort(cfg)
	if !ok || p != 2575 {
		t.Errorf("expected (2575, true), got (%d, %v)", p, ok)
	}
}

func TestExtractConnectorPort_Int64(t *testing.T) {
	cfg := map[string]interface{}{"port": int64(1433)}
	p, ok := extractConnectorPort(cfg)
	if !ok || p != 1433 {
		t.Errorf("expected (1433, true), got (%d, %v)", p, ok)
	}
}

func TestExtractConnectorPort_Missing(t *testing.T) {
	// No "port" key at all (e.g. a file_listener config) must never be treated as a
	// real port — this is the exact false-positive that let unrelated file_listener
	// interfaces get swept into someone else's port-conflict halt.
	cfg := map[string]interface{}{"host": "localhost"}
	p, ok := extractConnectorPort(cfg)
	if ok {
		t.Errorf("expected ok=false for missing port, got (%d, %v)", p, ok)
	}
}

func TestExtractConnectorPort_NilConfig(t *testing.T) {
	p, ok := extractConnectorPort(nil)
	if ok {
		t.Errorf("expected ok=false for nil config, got (%d, %v)", p, ok)
	}
}

func TestExtractConnectorPort_StringValue(t *testing.T) {
	// tcp_mllp_inbound accepts a quoted-string port (via ConnectorConfig.GetInt's
	// strconv.Atoi fallback), so a numeric string must round-trip as a real port —
	// otherwise two real MLLP interfaces sharing a string-typed port would silently
	// stop triggering the conflict halt at all.
	cfg := map[string]interface{}{"port": "6610"}
	p, ok := extractConnectorPort(cfg)
	if !ok || p != 6610 {
		t.Errorf("expected (6610, true) for numeric string port, got (%d, %v)", p, ok)
	}
}

func TestExtractConnectorPort_EmptyStringValue(t *testing.T) {
	// http_rest_inbound configs have been seen in the wild with "port": "" (a real
	// misconfiguration, e.g. Da Vinci PAS) — this must not be treated as port 0.
	cfg := map[string]interface{}{"port": ""}
	p, ok := extractConnectorPort(cfg)
	if ok {
		t.Errorf("expected ok=false for empty string port, got (%d, %v)", p, ok)
	}
}
