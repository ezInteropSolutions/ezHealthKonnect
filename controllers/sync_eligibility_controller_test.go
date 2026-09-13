// controllers/sync_eligibility_controller_test.go
//
// Integration test for SyncEligibilityController against a real Postgres —
// GetPipeline/ExecutePipeline both need real DB access (interface/pipeline
// resolution, debug-level/coverage-audit lookups), so a pure in-memory unit
// test can't exercise the happy path. Mirrors the DATABASE_URL-gated skip
// convention already established in services/hl7_fhir_transform_integration_test.go.
//
// Run: DATABASE_URL="postgres://ezhealth_user:secure_password_change_me@localhost:5432/ezhealthkonnect?sslmode=disable" \
//        go test ./controllers/ -v -run TestSyncEligibility
package controllers

import (
	"database/sql"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"ezhealthkonnect/services"
	"ezhealthkonnect/services/logger"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
)

// TestMain initializes the package-level structured logger — ExecutePipeline
// calls logger.Debug/Info directly, and that global is only ever set up by
// main()'s own startup sequence, never by an init() function. Every existing
// EDI test in this codebase calls executors directly (bypassing
// ExecutePipeline entirely), so this is the first test to actually need it.
func TestMain(m *testing.M) {
	logger.Init()
	os.Exit(m.Run())
}

// chdirToRepoRoot temporarily switches the process working directory to the
// repo root and returns a restore func. edi.parse/edi.build (via
// NewExecutorRegistry) self-initialize against a HARDCODED
// "./edi/schemas/x12_005010" relative path, unlike edi_schema_controller_test.go's
// own direct edi.NewX12SchemaLoader("../edi/schemas/x12_005010") call — `go test`
// runs this package's binary with cwd=controllers/, so without this, every
// executor in the registry that self-inits from a repo-root-relative path
// (edi.*, cda.*) silently fails to load and errors on Execute(). Scoped to a
// single test (chdir-then-defer-restore), not a package-wide TestMain chdir,
// so it can't affect edi_schema_controller_test.go's own controllers/-relative
// path in a different test function.
func chdirToRepoRoot(t *testing.T) func() {
	t.Helper()
	original, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed")
	}
	repoRoot := filepath.Dir(filepath.Dir(thisFile)) // controllers/ -> repo root
	if err := os.Chdir(repoRoot); err != nil {
		t.Fatalf("os.Chdir(%s): %v", repoRoot, err)
	}
	return func() { os.Chdir(original) }
}

func openSyncEligibilityTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("skipping integration test: DATABASE_URL not set")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("skipping integration test: cannot reach DB: %v", err)
	}
	return db
}

// sync270Sample is the same self-authored 270 fixture used by this session's
// own edi/testdata/real_samples/self_authored_270_sample.txt — a real,
// schema-valid 270 with a full ISA...IEA envelope.
const sync270Sample = "ISA*00*          *00*          *ZZ*PROVIDER1      *ZZ*PAYER1         *260912*1421*^*00501*889860169*0*P*:~\n" +
	"GS*HS***260912*1421*889893012*X*005010X279A1~\n" +
	"ST*270*4521~\n" +
	"BHT*0022*13*ELIG0001*20260912*1200~\n" +
	"HL*1**20*1~\n" +
	"NM1*PR*2*PAYER1*****PI*PAYER001~\n" +
	"HL*2*1*21*1~\n" +
	"NM1*1P*2*ACME CLINIC*****XX*1234567890~\n" +
	"HL*3*2*22*0~\n" +
	"TRN*1*TRACE001~\n" +
	"NM1*IL*1*SMITH*JANE****MI*SUB123~\n" +
	"DMG*D8*19800101*F~\n" +
	"EQ*30~\n" +
	"SE*12*4521~\n" +
	"GE*1*889893012~\n" +
	"IEA*1*889860169~\n"

// setupSyncEligibilityTestPipeline creates a real interface + a real "270"
// pipeline (edi.parse -> edi.build, round-tripping the SAME transaction set)
// — enough to prove the CONTROLLER's own mechanism (resolve by interface +
// message type, execute synchronously, extract the build step's real
// output) without needing real eligibility business logic in the test
// pipeline itself, which the controller is deliberately agnostic to.
func setupSyncEligibilityTestPipeline(t *testing.T, db *sql.DB) (interfaceID string, cleanup func()) {
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
	`, userID, "Sync Eligibility Controller Test").Scan(&ifaceID)
	if err != nil {
		t.Fatalf("insert test interface: %v", err)
	}

	var pipelineID string
	err = db.QueryRow(`
		INSERT INTO transformation_pipelines (interface_id, message_type, pipeline_name, enabled, version, created_at, updated_at)
		VALUES ($1, '270', 'Sync Eligibility Test Pipeline', true, 1, NOW(), NOW())
		RETURNING id
	`, ifaceID).Scan(&pipelineID)
	if err != nil {
		db.Exec(`DELETE FROM interfaces WHERE id = $1`, ifaceID)
		t.Fatalf("insert test pipeline: %v", err)
	}

	_, err = db.Exec(`
		INSERT INTO transformation_steps (pipeline_id, step_name, step_type, sequence, config)
		VALUES
			($1, 'Parse 270', 'edi.parse', 10, '{}'::jsonb),
			($1, 'Rebuild 270', 'edi.build', 20, '{"sourceField":"parsedEDI","transactionSet":"270"}'::jsonb)
	`, pipelineID)
	if err != nil {
		db.Exec(`DELETE FROM interfaces WHERE id = $1`, ifaceID)
		t.Fatalf("insert test steps: %v", err)
	}

	return ifaceID, func() {
		db.Exec(`DELETE FROM interfaces WHERE id = $1`, ifaceID)
	}
}

// testCredentialStore builds a real (non-nil) CredentialStore with a
// throwaway all-zero key — the executors this test exercises (edi.parse,
// edi.build) never touch credential decryption, but ExecutorRegistry binds
// a credStore method value for every registered executor unconditionally at
// construction time, so a real instance avoids relying on nil-receiver
// method-value semantics.
func testCredentialStore(t *testing.T) *services.CredentialStore {
	t.Helper()
	key := base64.StdEncoding.EncodeToString(make([]byte, 32))
	store, err := services.NewCredentialStore(key)
	if err != nil {
		t.Fatalf("NewCredentialStore: %v", err)
	}
	return store
}

func newTestSyncEligibilityRouter(t *testing.T, db *sql.DB) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	ctrl := NewSyncEligibilityController(services.NewTransformationPipelineService(db, testCredentialStore(t)))
	group := router.Group("/api/eligibility")
	ctrl.RegisterRoutes(group)
	return router
}

func TestSyncEligibilityController_HappyPath_270In270Out(t *testing.T) {
	defer chdirToRepoRoot(t)()

	db := openSyncEligibilityTestDB(t)
	defer db.Close()

	interfaceID, cleanup := setupSyncEligibilityTestPipeline(t, db)
	defer cleanup()

	router := newTestSyncEligibilityRouter(t, db)

	req := httptest.NewRequest(http.MethodPost, "/api/eligibility/"+interfaceID+"/check", strings.NewReader(sync270Sample))
	req.Header.Set("Content-Type", "application/edi-x12")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "ST*270*") {
		t.Errorf("response body should contain a rebuilt ST*270* segment, got: %s", body)
	}
	if !strings.Contains(body, "NM1*IL*1*SMITH*JANE") {
		t.Errorf("response body should carry the subscriber's own data through, got: %s", body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/edi-x12" {
		t.Errorf("Content-Type = %q, want application/edi-x12", ct)
	}
}

func TestSyncEligibilityController_NoPipelineConfigured_Returns404(t *testing.T) {
	db := openSyncEligibilityTestDB(t)
	defer db.Close()

	router := newTestSyncEligibilityRouter(t, db)

	req := httptest.NewRequest(http.MethodPost, "/api/eligibility/00000000-0000-0000-0000-000000000000/check", strings.NewReader(sync270Sample))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404; body: %s", rec.Code, rec.Body.String())
	}
}

func TestSyncEligibilityController_EmptyBody_Returns400(t *testing.T) {
	db := openSyncEligibilityTestDB(t)
	defer db.Close()

	router := newTestSyncEligibilityRouter(t, db)

	req := httptest.NewRequest(http.MethodPost, "/api/eligibility/00000000-0000-0000-0000-000000000000/check", strings.NewReader(""))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400; body: %s", rec.Code, rec.Body.String())
	}
}

func TestSyncEligibilityController_MalformedEDI_ReturnsErrorStatus(t *testing.T) {
	defer chdirToRepoRoot(t)()

	db := openSyncEligibilityTestDB(t)
	defer db.Close()

	interfaceID, cleanup := setupSyncEligibilityTestPipeline(t, db)
	defer cleanup()

	router := newTestSyncEligibilityRouter(t, db)

	req := httptest.NewRequest(http.MethodPost, "/api/eligibility/"+interfaceID+"/check", strings.NewReader("NOT REAL EDI CONTENT"))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity && rec.Code != http.StatusBadGateway {
		t.Errorf("status = %d, want 422 or 502 for malformed EDI content; body: %s", rec.Code, rec.Body.String())
	}
}
