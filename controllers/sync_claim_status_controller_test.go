// controllers/sync_claim_status_controller_test.go
//
// Integration test for SyncClaimStatusController against a real Postgres —
// mirrors sync_eligibility_controller_test.go's own structure exactly,
// reusing that file's package-level helpers (chdirToRepoRoot,
// openSyncEligibilityTestDB, testCredentialStore — all DB-connection/env
// mechanics unrelated to which endpoint is under test) rather than
// duplicating them.
//
// Run: DATABASE_URL="postgres://ezhealth_user:secure_password_change_me@localhost:5432/ezhealthkonnect?sslmode=disable" \
//        go test ./controllers/ -v -run TestSyncClaimStatus
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

// sync276Sample is the same self-authored 276 fixture used by this session's
// own edi/testdata/real_samples/self_authored_276_sample.txt — a real,
// schema-valid 276 with a full ISA...IEA envelope.
const sync276Sample = "ISA*00*          *00*          *ZZ*PROVIDER1      *ZZ*PAYER1         *260914*0900*^*00501*889860170*0*P*:~\n" +
	"GS*HR***260914*0900*889893013*X*005010X212~\n" +
	"ST*276*4522~\n" +
	"BHT*0010*13*CLMSTAT01*20260914*0900~\n" +
	"HL*1**20*1~\n" +
	"NM1*PR*2*PAYER1*****PI*PAYER001~\n" +
	"HL*2*1*21*1~\n" +
	"NM1*41*2*ACME CLEARINGHOUSE*****46*CLR001~\n" +
	"HL*3*2*19*1~\n" +
	"NM1*1P*2*ACME CLINIC*****XX*1234567890~\n" +
	"HL*4*3*22*1~\n" +
	"NM1*IL*1*SMITH*JANE****MI*SUB123~\n" +
	"DMG*D8*19800101*F~\n" +
	"TRN*1*REQTRACE001~\n" +
	"REF*EJ*PCN0001~\n" +
	"AMT*T3*250.00~\n" +
	"DTP*472*D8*20260110~\n" +
	"SVC*HC:99213**250.00~\n" +
	"HL*5*4*23*0~\n" +
	"NM1*QC*1*SMITH*TOMMY****MI*SUB123~\n" +
	"DMG*D8*20100601*M~\n" +
	"TRN*1*REQTRACE002~\n" +
	"REF*EJ*PCN0002~\n" +
	"SVC*HC:90460**75.00~\n" +
	"SE*23*4522~\n" +
	"GE*1*889893013~\n" +
	"IEA*1*889860170~\n"

// setupSyncClaimStatusTestPipeline creates a real interface + a real "276"
// pipeline (edi.parse -> edi.build, round-tripping the SAME transaction set)
// — enough to prove the CONTROLLER's own mechanism (resolve by interface +
// message type, execute synchronously, extract the build step's real
// output) without needing real claim-adjudication logic in the test
// pipeline itself, which the controller is deliberately agnostic to. Mirrors
// setupSyncEligibilityTestPipeline exactly, transaction set only differs.
func setupSyncClaimStatusTestPipeline(t *testing.T, db *sql.DB) (interfaceID string, cleanup func()) {
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
	`, userID, "Sync Claim Status Controller Test").Scan(&ifaceID)
	if err != nil {
		t.Fatalf("insert test interface: %v", err)
	}

	var pipelineID string
	err = db.QueryRow(`
		INSERT INTO transformation_pipelines (interface_id, message_type, pipeline_name, enabled, version, created_at, updated_at)
		VALUES ($1, '276', 'Sync Claim Status Test Pipeline', true, 1, NOW(), NOW())
		RETURNING id
	`, ifaceID).Scan(&pipelineID)
	if err != nil {
		db.Exec(`DELETE FROM interfaces WHERE id = $1`, ifaceID)
		t.Fatalf("insert test pipeline: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO transformation_steps (pipeline_id, step_name, step_type, sequence, config)
		VALUES
			($1, 'Parse 276', 'edi.parse', 10, '{}'::jsonb),
			($1, 'Rebuild 276', 'edi.build', 20, '{"sourceField":"parsedEDI","transactionSet":"276"}'::jsonb)
	`, pipelineID)
	if err != nil {
		db.Exec(`DELETE FROM interfaces WHERE id = $1`, ifaceID)
		t.Fatalf("insert test steps: %v", err)
	}

	return ifaceID, func() {
		db.Exec(`DELETE FROM interfaces WHERE id = $1`, ifaceID)
	}
}

func newTestSyncClaimStatusRouter(t *testing.T, db *sql.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	ctrl := NewSyncClaimStatusController(services.NewTransformationPipelineService(db, testCredentialStore(t)))
	group := router.Group("/api/claim-status")
	ctrl.RegisterRoutes(group)
	return router
}

func TestSyncClaimStatusController_HappyPath_276In276Out(t *testing.T) {
	defer chdirToRepoRoot(t)()

	db := openSyncEligibilityTestDB(t)
	defer db.Close()

	interfaceID, cleanup := setupSyncClaimStatusTestPipeline(t, db)
	defer cleanup()

	router := newTestSyncClaimStatusRouter(t, db)

	req := httptest.NewRequest(http.MethodPost, "/api/claim-status/"+interfaceID+"/check", strings.NewReader(sync276Sample))
	req.Header.Set("Content-Type", "application/edi-x12")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "ST*276*") {
		t.Errorf("response body should contain a rebuilt ST*276* segment, got: %s", body)
	}
	if !strings.Contains(body, "NM1*IL*1*SMITH*JANE") {
		t.Errorf("response body should carry the subscriber's own data through, got: %s", body)
	}
	if !strings.Contains(body, "NM1*QC*1*SMITH*TOMMY") {
		t.Errorf("response body should carry the dependent's own data through, got: %s", body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/edi-x12" {
		t.Errorf("Content-Type = %q, want application/edi-x12", ct)
	}
}

func TestSyncClaimStatusController_NoPipelineConfigured_Returns404(t *testing.T) {
	db := openSyncEligibilityTestDB(t)
	defer db.Close()

	router := newTestSyncClaimStatusRouter(t, db)

	req := httptest.NewRequest(http.MethodPost, "/api/claim-status/00000000-0000-0000-0000-000000000000/check", strings.NewReader(sync276Sample))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body: %s", rec.Code, rec.Body.String())
	}
}

func TestSyncClaimStatusController_EmptyBody_Returns400(t *testing.T) {
	db := openSyncEligibilityTestDB(t)
	defer db.Close()

	router := newTestSyncClaimStatusRouter(t, db)

	req := httptest.NewRequest(http.MethodPost, "/api/claim-status/00000000-0000-0000-0000-000000000000/check", strings.NewReader(""))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestSyncClaimStatusController_MalformedEDI_ReturnsErrorStatus(t *testing.T) {
	defer chdirToRepoRoot(t)()

	db := openSyncEligibilityTestDB(t)
	defer db.Close()

	interfaceID, cleanup := setupSyncClaimStatusTestPipeline(t, db)
	defer cleanup()

	router := newTestSyncClaimStatusRouter(t, db)

	req := httptest.NewRequest(http.MethodPost, "/api/claim-status/"+interfaceID+"/check", strings.NewReader("NOT REAL EDI CONTENT"))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity && rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 422 or 502 for malformed EDI content; body: %s", rec.Code, rec.Body.String())
	}
}
