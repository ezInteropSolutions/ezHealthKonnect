// controllers/sync_prior_auth_controller_test.go
//
// Integration test for SyncPriorAuthController against a real Postgres —
// mirrors sync_claim_status_controller_test.go's own structure exactly,
// reusing that file's package-level helpers (chdirToRepoRoot,
// openSyncEligibilityTestDB, testCredentialStore — all DB-connection/env
// mechanics unrelated to which endpoint is under test) rather than
// duplicating them.
//
// Run: DATABASE_URL="postgres://ezhealth_user:secure_password_change_me@localhost:5432/ezhealthkonnect?sslmode=disable" \
//        go test ./controllers/ -v -run TestSyncPriorAuth
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

// sync278Sample is a self-authored 278 request (no HCR — a pure request; the
// resolved test pipeline below just re-parses/re-builds it unchanged, same
// "prove the controller's own mechanism, not real business logic" scope as
// sync276Sample).
const sync278Sample = "ISA*00*          *00*          *ZZ*ACMECLINIC     *ZZ*ACMEUMO        *260914*0900*^*00501*889860180*0*P*:~\n" +
	"GS*HI***260914*0900*889893020*X*005010X217~\n" +
	"ST*278*4530~\n" +
	"BHT*0078*13*PA0001*20260914*0900~\n" +
	"HL*1**20*1~\n" +
	"NM1*X3*2*ACME UMO*****PI*UMO001~\n" +
	"HL*2*1*21*1~\n" +
	"NM1*1P*2*ACME CLINIC*****XX*1234567890~\n" +
	"HL*3*2*22*1~\n" +
	"NM1*IL*1*SMITH*JANE****MI*SUB123~\n" +
	"HL*4*3*EV*0~\n" +
	"TRN*1*EVENTTRACE001~\n" +
	"UM*HS*I*1~\n" +
	"SE*13*4530~\n" +
	"GE*1*889893020~\n" +
	"IEA*1*889860180~\n"

// setupSyncPriorAuthTestPipeline creates a real interface + a real "278"
// pipeline (edi.parse -> edi.build, round-tripping the SAME transaction set)
// — enough to prove the CONTROLLER's own mechanism (resolve by interface +
// message type, execute synchronously, extract the build step's real
// output) without needing real prior-authorization decision logic in the
// test pipeline itself, which the controller is deliberately agnostic to.
// Mirrors setupSyncClaimStatusTestPipeline exactly, transaction set only
// differs.
func setupSyncPriorAuthTestPipeline(t *testing.T, db *sql.DB) (interfaceID string, cleanup func()) {
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
	`, userID, "Sync Prior Auth Controller Test").Scan(&ifaceID)
	if err != nil {
		t.Fatalf("insert test interface: %v", err)
	}

	var pipelineID string
	err = db.QueryRow(`
		INSERT INTO transformation_pipelines (interface_id, message_type, pipeline_name, enabled, version, created_at, updated_at)
		VALUES ($1, '278', 'Sync Prior Auth Test Pipeline', true, 1, NOW(), NOW())
		RETURNING id
	`, ifaceID).Scan(&pipelineID)
	if err != nil {
		db.Exec(`DELETE FROM interfaces WHERE id = $1`, ifaceID)
		t.Fatalf("insert test pipeline: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO transformation_steps (pipeline_id, step_name, step_type, sequence, config)
		VALUES
			($1, 'Parse 278', 'edi.parse', 10, '{}'::jsonb),
			($1, 'Rebuild 278', 'edi.build', 20, '{"sourceField":"parsedEDI","transactionSet":"278"}'::jsonb)
	`, pipelineID)
	if err != nil {
		db.Exec(`DELETE FROM interfaces WHERE id = $1`, ifaceID)
		t.Fatalf("insert test steps: %v", err)
	}

	return ifaceID, func() {
		db.Exec(`DELETE FROM interfaces WHERE id = $1`, ifaceID)
	}
}

func newTestSyncPriorAuthRouter(t *testing.T, db *sql.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	ctrl := NewSyncPriorAuthController(services.NewTransformationPipelineService(db, testCredentialStore(t)))
	group := router.Group("/api/prior-auth")
	ctrl.RegisterRoutes(group)
	return router
}

func TestSyncPriorAuthController_HappyPath_278In278Out(t *testing.T) {
	defer chdirToRepoRoot(t)()

	db := openSyncEligibilityTestDB(t)
	defer db.Close()

	interfaceID, cleanup := setupSyncPriorAuthTestPipeline(t, db)
	defer cleanup()

	router := newTestSyncPriorAuthRouter(t, db)

	req := httptest.NewRequest(http.MethodPost, "/api/prior-auth/"+interfaceID+"/check", strings.NewReader(sync278Sample))
	req.Header.Set("Content-Type", "application/edi-x12")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "ST*278*") {
		t.Errorf("response body should contain a rebuilt ST*278* segment, got: %s", body)
	}
	if !strings.Contains(body, "NM1*IL*1*SMITH*JANE") {
		t.Errorf("response body should carry the subscriber's own data through, got: %s", body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/edi-x12" {
		t.Errorf("Content-Type = %q, want application/edi-x12", ct)
	}
}

func TestSyncPriorAuthController_NoPipelineConfigured_Returns404(t *testing.T) {
	db := openSyncEligibilityTestDB(t)
	defer db.Close()

	router := newTestSyncPriorAuthRouter(t, db)

	req := httptest.NewRequest(http.MethodPost, "/api/prior-auth/00000000-0000-0000-0000-000000000000/check", strings.NewReader(sync278Sample))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body: %s", rec.Code, rec.Body.String())
	}
}

func TestSyncPriorAuthController_EmptyBody_Returns400(t *testing.T) {
	db := openSyncEligibilityTestDB(t)
	defer db.Close()

	router := newTestSyncPriorAuthRouter(t, db)

	req := httptest.NewRequest(http.MethodPost, "/api/prior-auth/00000000-0000-0000-0000-000000000000/check", strings.NewReader(""))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestSyncPriorAuthController_MalformedEDI_ReturnsErrorStatus(t *testing.T) {
	defer chdirToRepoRoot(t)()

	db := openSyncEligibilityTestDB(t)
	defer db.Close()

	interfaceID, cleanup := setupSyncPriorAuthTestPipeline(t, db)
	defer cleanup()

	router := newTestSyncPriorAuthRouter(t, db)

	req := httptest.NewRequest(http.MethodPost, "/api/prior-auth/"+interfaceID+"/check", strings.NewReader("NOT REAL EDI CONTENT"))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity && rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 422 or 502 for malformed EDI content; body: %s", rec.Code, rec.Body.String())
	}
}
