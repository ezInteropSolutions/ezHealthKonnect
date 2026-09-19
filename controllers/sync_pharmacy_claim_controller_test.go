// controllers/sync_pharmacy_claim_controller_test.go
//
// Integration test for SyncPharmacyClaimController against a real Postgres —
// mirrors sync_claim_status_controller_test.go's own structure exactly,
// reusing that file's/sync_eligibility_controller_test.go's package-level
// helpers (chdirToRepoRoot, openSyncEligibilityTestDB, testCredentialStore —
// all DB-connection/env mechanics unrelated to which endpoint is under test)
// rather than duplicating them.
//
// Run: DATABASE_URL="postgres://ezhealth_user:secure_password_change_me@localhost:5432/ezhealthkonnect?sslmode=disable" \
//        go test ./controllers/ -v -run TestSyncPharmacyClaim
package controllers

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ezhealthkonnect/services"

	"github.com/gin-gonic/gin"
)

// syncB1RequestSample is the same self-authored B1 request fixture
// construction as services/ncpdp_telecom_b1_request_fhir_builder_test.go's
// own buildSelfAuthoredB1Request() — duplicated here (rather than exported
// cross-package) matching this codebase's own per-package sample-fixture
// precedent (e.g. EDI's per-package sample duplication).
func syncB1RequestSample() string {
	const rs, fs = "\x1E", "\x1C"
	header := "999999" + "D0" + "B1" +
		"          " + // processorControlNumber
		"1" + "01" +
		"1111111111     " + // serviceProviderId (15)
		"20260919" +
		"          " // software (10)
	body := rs + fs + "AM01" + fs + "CBSMITH" + fs + "CAJOHN" + fs + "C419800101" + fs + "C52" +
		rs + fs + "AM02" + fs + "EY01" + fs + "E91234567890" +
		rs + fs + "AM04" + fs + "C2123456789012" +
		rs + fs + "AM07" + fs + "D2000000123456" + fs + "E103" + fs + "D700003089421" + fs + "E70000030000" + fs + "D301" + fs + "D5030" + fs + "DE20220924" +
		rs + fs + "AM11" + fs + "D90000057A" + fs + "DC0000027E" + fs + "DX0000016B" + fs + "DQ00000000" + fs + "DU0000084F"
	return header + body
}

// setupSyncPharmacyClaimTestPipeline creates a real interface + a real
// "B1_REQUEST" pipeline (ncpdptelecom.parse -> ncpdptelecom.build,
// round-tripping the SAME transmission) — enough to prove the CONTROLLER's
// own mechanism (resolve by interface + message type, execute
// synchronously, extract the build step's real output) without needing real
// claim-adjudication logic in the test pipeline itself, which the
// controller is deliberately agnostic to. Mirrors
// setupSyncClaimStatusTestPipeline exactly, transaction type only differs.
func setupSyncPharmacyClaimTestPipeline(t *testing.T, db *sql.DB) (interfaceID string, cleanup func()) {
	t.Helper()

	var userID string
	if err := db.QueryRow(`SELECT id FROM users LIMIT 1`).Scan(&userID); err != nil {
		t.Skipf("no users in DB: %v", err)
	}

	var ifaceID string
	err := db.QueryRow(`
		INSERT INTO interfaces (user_id, name, status, interface_status, source_type, target_type, is_active, created_at, updated_at)
		VALUES ($1, $2, 'active', 'active', 'sync_http', 'sync_http', true, NOW(), NOW())
		RETURNING id
	`, userID, "Sync Pharmacy Claim Controller Test").Scan(&ifaceID)
	if err != nil {
		t.Fatalf("insert test interface: %v", err)
	}

	var pipelineID string
	err = db.QueryRow(`
		INSERT INTO transformation_pipelines (interface_id, message_type, pipeline_name, enabled, version, created_at, updated_at)
		VALUES ($1, 'B1_REQUEST', 'Sync Pharmacy Claim Test Pipeline', true, 1, NOW(), NOW())
		RETURNING id
	`, ifaceID).Scan(&pipelineID)
	if err != nil {
		db.Exec(`DELETE FROM interfaces WHERE id = $1`, ifaceID)
		t.Fatalf("insert test pipeline: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO transformation_steps (pipeline_id, step_name, step_type, sequence, config)
		VALUES
			($1, 'Parse B1 Request', 'ncpdptelecom.parse', 10, '{"direction":"request"}'::jsonb),
			($1, 'Rebuild B1 Request', 'ncpdptelecom.build', 20, '{"sourceField":"parsedTelecom","transactionCode":"B1","direction":"request"}'::jsonb)
	`, pipelineID)
	if err != nil {
		db.Exec(`DELETE FROM interfaces WHERE id = $1`, ifaceID)
		t.Fatalf("insert test steps: %v", err)
	}

	return ifaceID, func() {
		db.Exec(`DELETE FROM interfaces WHERE id = $1`, ifaceID)
	}
}

func newTestSyncPharmacyClaimRouter(t *testing.T, db *sql.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	ctrl := NewSyncPharmacyClaimController(services.NewTransformationPipelineService(db, testCredentialStore(t)))
	group := router.Group("/api/pharmacy-claim")
	ctrl.RegisterRoutes(group)
	return router
}

func TestSyncPharmacyClaimController_HappyPath_B1RequestInB1RequestOut(t *testing.T) {
	defer chdirToRepoRoot(t)()

	db := openSyncEligibilityTestDB(t)
	defer db.Close()

	interfaceID, cleanup := setupSyncPharmacyClaimTestPipeline(t, db)
	defer cleanup()

	router := newTestSyncPharmacyClaimRouter(t, db)

	req := httptest.NewRequest(http.MethodPost, "/api/pharmacy-claim/"+interfaceID+"/submit", strings.NewReader(syncB1RequestSample()))
	req.Header.Set("Content-Type", "application/x-ncpdp-telecom-d0")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.HasPrefix(body, "999999D0B1") {
		t.Errorf("response body should start with the rebuilt 56-byte header (bin=999999, version=D0, transactionCode=B1), got: %q", body[:min(len(body), 20)])
	}
	if !strings.Contains(body, "CBSMITH") {
		t.Errorf("response body should carry the patient's own last name through, got: %s", body)
	}
	if !strings.Contains(body, "D2000000123456") {
		t.Errorf("response body should carry the claim's own prescription reference number through, got: %s", body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/x-ncpdp-telecom-d0" {
		t.Errorf("Content-Type = %q, want application/x-ncpdp-telecom-d0", ct)
	}
}

func TestSyncPharmacyClaimController_NoPipelineConfigured_Returns404(t *testing.T) {
	db := openSyncEligibilityTestDB(t)
	defer db.Close()

	router := newTestSyncPharmacyClaimRouter(t, db)

	req := httptest.NewRequest(http.MethodPost, "/api/pharmacy-claim/00000000-0000-0000-0000-000000000000/submit", strings.NewReader(syncB1RequestSample()))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body: %s", rec.Code, rec.Body.String())
	}
}

func TestSyncPharmacyClaimController_EmptyBody_Returns400(t *testing.T) {
	db := openSyncEligibilityTestDB(t)
	defer db.Close()

	router := newTestSyncPharmacyClaimRouter(t, db)

	req := httptest.NewRequest(http.MethodPost, "/api/pharmacy-claim/00000000-0000-0000-0000-000000000000/submit", strings.NewReader(""))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestSyncPharmacyClaimController_MalformedContent_ReturnsErrorStatus(t *testing.T) {
	defer chdirToRepoRoot(t)()

	db := openSyncEligibilityTestDB(t)
	defer db.Close()

	interfaceID, cleanup := setupSyncPharmacyClaimTestPipeline(t, db)
	defer cleanup()

	router := newTestSyncPharmacyClaimRouter(t, db)

	req := httptest.NewRequest(http.MethodPost, "/api/pharmacy-claim/"+interfaceID+"/submit", strings.NewReader("NOT REAL NCPDP TELECOM CONTENT"))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity && rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 422 or 502 for malformed content; body: %s", rec.Code, rec.Body.String())
	}
}
