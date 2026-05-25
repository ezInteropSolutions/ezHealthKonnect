// processing/phi_safety_integration_test.go
// Integration test for the PHI Safety port-conflict halt mechanism.
//
// Run with:  go test ./processing/ -run TestPortConflictHalt -v -tags integration

//go:build integration

package processing

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"testing"
	"time"

	"ezhealthkonnect/models"
	_ "github.com/lib/pq"
)

const (
	testDBConnStr  = "host=postgres port=5432 user=ezhealth_user password=secure_password_change_me dbname=ezhealthkonnect sslmode=disable"
	conflictPort   = 19876 // use an obscure port so we don't clash with real services
	testInterfaceA = "00000000-0000-0000-0000-aaaaaaaaaaaa"
	testInterfaceB = "00000000-0000-0000-0000-bbbbbbbbbbbb"
)

// TestPortConflictHalt_BothInterfacesHalted verifies the full PHI safety flow:
//
//  1. Spin up a real TCP listener on conflictPort (simulating Interface A already running).
//  2. Call isPortConflictError() with a real bind error from a second Listen attempt.
//  3. Verify the error is correctly classified.
//  4. Verify extractConnectorPort() round-trips correctly.
//  5. (DB assertions skipped in unit build; full flow exercised in integration build.)
func TestPortConflictHalt_HelperPipeline(t *testing.T) {
	// ── Part 1: create a real bind error ────────────────────────────────────
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", conflictPort))
	if err != nil {
		t.Skipf("cannot bind test port %d: %v", conflictPort, err)
	}
	defer ln.Close()

	_, err2 := net.Listen("tcp", fmt.Sprintf(":%d", conflictPort))
	if err2 == nil {
		t.Fatal("expected second bind to fail but it succeeded")
	}

	if !isPortConflictError(err2) {
		t.Errorf("isPortConflictError(%q) = false, want true", err2.Error())
	}
	t.Logf("✅ isPortConflictError correctly identified: %v", err2)

	// ── Part 2: extractConnectorPort round-trip ──────────────────────────────
	cfg := map[string]interface{}{"port": float64(conflictPort), "host": "0.0.0.0"}
	if p := extractConnectorPort(cfg); p != conflictPort {
		t.Errorf("extractConnectorPort = %d, want %d", p, conflictPort)
	}
	t.Logf("✅ extractConnectorPort correctly returns %d", conflictPort)
}

// TestPortConflictHalt_DBAndAudit exercises haltOnPortConflictLocked against a
// real PostgreSQL instance.  Requires TEST_INFRA_HOST to be set in the environment
// (same pattern used by Sprint 8 integration tests).
func TestPortConflictHalt_DBAndAudit(t *testing.T) {
	db, err := sql.Open("postgres", testDBConnStr)
	if err != nil {
		t.Skipf("cannot open test DB: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		t.Skipf("DB not reachable: %v", err)
	}

	// Insert two minimal test interfaces so we have real DB rows to update.
	for _, id := range []string{testInterfaceA, testInterfaceB} {
		_, _ = db.Exec(`DELETE FROM interfaces WHERE id = $1`, id)
		// Grab any real user_id so we satisfy the NOT NULL constraint.
		var userID string
		if err := db.QueryRow(`SELECT id FROM users LIMIT 1`).Scan(&userID); err != nil {
			t.Skipf("no users in DB: %v", err)
		}
		_, err := db.Exec(`
			INSERT INTO interfaces (id, user_id, name, status, interface_status,
			                        source_type, target_type, is_active, created_at, updated_at)
			VALUES ($1, $2, $3, 'active', 'active', 'hl7v2', 'fhir', true, NOW(), NOW())
		`, id, userID, "PHI Safety Test "+id)
		if err != nil {
			t.Skipf("cannot insert test interface %s: %v", id, err)
		}
	}
	defer func() {
		for _, id := range []string{testInterfaceA, testInterfaceB} {
			db.Exec(`DELETE FROM interfaces WHERE id = $1`, id)
			db.Exec(`DELETE FROM audit_logs WHERE entity_id = $1`, id)
		}
	}()

	// Build a minimal engine pointing at the test DB (no connectors, no services).
	engine := &ProcessingEngine{
		db:               db,
		activeInterfaces: map[string]*InterfaceStatus{
			testInterfaceA: {InterfaceID: testInterfaceA, Status: "active"},
			testInterfaceB: {InterfaceID: testInterfaceB, Status: "active"},
		},
		activeConnectors: make(map[string]InputConnector),
		messageChan:      make(map[string]chan *models.InboundMessage),
	}

	// Simulate the conflict: haltOnPortConflictLocked targets testInterfaceA.
	// testInterfaceB should also be stopped if it appears in findActiveInterfacesByPort —
	// but since we have no transformation_steps rows for it, it won't be found;
	// this test only exercises the DB update path for the directly-failing interface.
	engine.haltOnPortConflictLocked(testInterfaceA, conflictPort,
		fmt.Errorf("listen tcp :%d: bind: address already in use", conflictPort))

	// Give the async DB write goroutine time to complete.
	time.Sleep(500 * time.Millisecond)

	// ── Assertion 1: interface status updated to 'error' ────────────────────
	var status string
	if err := db.QueryRow(`SELECT status FROM interfaces WHERE id = $1`, testInterfaceA).Scan(&status); err != nil {
		t.Fatalf("cannot query interface status: %v", err)
	}
	if status != "error" {
		t.Errorf("interface status = %q, want 'error'", status)
	} else {
		t.Logf("✅ Interface A status correctly set to 'error'")
	}

	// ── Assertion 2: HIPAA audit log entry written ──────────────────────────
	var auditAction, riskLevel, result string
	var complianceFlagsJSON []byte
	err = db.QueryRow(`
		SELECT action, risk_level, result, compliance_flags
		FROM audit_logs
		WHERE entity_id = $1 AND action = 'PHI_SAFETY_HALT'
		ORDER BY created_at DESC LIMIT 1
	`, testInterfaceA).Scan(&auditAction, &riskLevel, &result, &complianceFlagsJSON)
	if err != nil {
		t.Fatalf("no PHI_SAFETY_HALT audit log row found for interface %s: %v", testInterfaceA, err)
	}

	if auditAction != "PHI_SAFETY_HALT" {
		t.Errorf("audit action = %q, want 'PHI_SAFETY_HALT'", auditAction)
	}
	if riskLevel != "critical" {
		t.Errorf("audit risk_level = %q, want 'critical'", riskLevel)
	}
	if result != "blocked" {
		t.Errorf("audit result = %q, want 'blocked'", result)
	}
	t.Logf("✅ HIPAA audit log written: action=%s risk_level=%s result=%s", auditAction, riskLevel, result)

	// ── Assertion 3: compliance_flags contain hipaa_safety_halt ─────────────
	var complianceFlags map[string]interface{}
	if err := json.Unmarshal(complianceFlagsJSON, &complianceFlags); err == nil {
		if v, ok := complianceFlags["hipaa_safety_halt"].(bool); !ok || !v {
			t.Errorf("compliance_flags.hipaa_safety_halt not true: %v", complianceFlags)
		} else {
			t.Logf("✅ compliance_flags.hipaa_safety_halt = true")
		}
	}

	// ── Assertion 4: interface removed from activeInterfaces ─────────────────
	if _, stillActive := engine.activeInterfaces[testInterfaceA]; stillActive {
		t.Errorf("interface A still in activeInterfaces after halt")
	} else {
		t.Logf("✅ Interface A removed from activeInterfaces")
	}
}
