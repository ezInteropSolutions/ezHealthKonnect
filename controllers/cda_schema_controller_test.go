// controllers/cda_schema_controller_test.go
//
// Unit test for CDASchemaController's new GetRawMappingDelta endpoint, added
// so the pipeline builder's JSON Export/Import can carry an interface's
// actual CDA field-mapping overrides alongside the step config -- see
// PropertiesPanel.js's EXTERNAL_MAPPING_STORES. Only the mapper-unavailable
// branch is unit-testable without a real DB connection (this package has no
// DB-mocking convention, matching every other controller test in this file).
//
// Run: go test ./controllers/ -v -run TestCDASchemaController
package controllers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCDASchemaController_GetRawMappingDelta_NoMapper_Returns503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cc := &CDASchemaController{} // mapper is nil
	r := gin.New()
	cc.RegisterRoutes(r.Group("/api/cda"))

	req := httptest.NewRequest(http.MethodGet, "/api/cda/mappings/iface-1/CCD/raw", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 when mapper is nil", w.Code)
	}
}
