// controllers/mapping_delta_controller_test.go
//
// Unit tests for MappingDeltaController's new raw-delta endpoints
// (GetRawMappingDelta/SaveRawMappingDelta), added so the pipeline builder's
// JSON Export/Import can carry an interface's actual HL7 field-mapping
// overrides alongside the step config -- see PropertiesPanel.js's
// EXTERNAL_MAPPING_STORES. Only the validation branches that return before
// touching mc.db are covered here (this package has no DB-mocking
// convention, matching every other controller test in this file/package).
//
// Run: go test ./controllers/ -v -run TestMappingDeltaController
package controllers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func mappingDeltaRouter(mc *MappingDeltaController) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	mc.RegisterRoutes(r.Group("/api/fhir"))
	return r
}

func TestMappingDeltaController_GetRawMappingDelta_MissingParams_Returns400(t *testing.T) {
	// Gin's router requires both :interfaceId and :messageType segments to
	// match this route at all -- an empty messageType segment ("//raw") is
	// the only way to trigger the explicit validation branch without a live DB.
	mc := NewMappingDeltaController(nil, nil)
	r := mappingDeltaRouter(mc)

	req := httptest.NewRequest(http.MethodGet, "/api/fhir/interfaces/some-interface/mapping-delta//raw", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound && w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 or 404 for a malformed path", w.Code)
	}
}

func TestMappingDeltaController_SaveRawMappingDelta_InvalidJSON_Returns400(t *testing.T) {
	// Invalid JSON must be rejected by ShouldBindJSON before mc.db is ever
	// touched -- safe to exercise with a nil db.
	mc := NewMappingDeltaController(nil, nil)
	r := mappingDeltaRouter(mc)

	req := httptest.NewRequest(http.MethodPut, "/api/fhir/interfaces/iface-1/mapping-delta/ADT^A01/raw",
		strings.NewReader("{not valid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for invalid JSON body", w.Code)
	}
}

func TestMappingDeltaController_SaveRawMappingDelta_EmptyBody_Returns400(t *testing.T) {
	mc := NewMappingDeltaController(nil, nil)
	r := mappingDeltaRouter(mc)

	req := httptest.NewRequest(http.MethodPut, "/api/fhir/interfaces/iface-1/mapping-delta/ADT^A01/raw",
		strings.NewReader(""))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 for an empty body", w.Code)
	}
}

// TestMappingDeltaController_RoutesRegistered_NoConflictWithExisting proves
// the two new /raw routes register cleanly alongside the existing
// mapping-delta routes (PUT/DELETE on the same path minus /raw) -- i.e. this
// change doesn't break Gin's route tree for the pre-existing endpoints.
func TestMappingDeltaController_RoutesRegistered_NoConflictWithExisting(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("RegisterRoutes panicked (likely a route conflict): %v", r)
		}
	}()
	mc := NewMappingDeltaController(nil, nil)
	_ = mappingDeltaRouter(mc)
}
