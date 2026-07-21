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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	cdaSchema "ezhealthkonnect/cda"

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

// TestCDASchemaController_DocumentTypeEndpoints_NoSchemaLoader_Returns503
// covers the guided-configuration endpoints' schemaLoader-nil guard —
// mirrors every other endpoint in this file (503 when the schema directory
// is unavailable), no DB needed.
func TestCDASchemaController_DocumentTypeEndpoints_NoSchemaLoader_Returns503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cc := &CDASchemaController{} // schemaLoader is nil
	r := gin.New()
	cc.RegisterRoutes(r.Group("/api/cda"))

	for _, path := range []string{"/api/cda/document-types", "/api/cda/document-types/CCD/requirements"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusServiceUnavailable {
			t.Errorf("%s: status = %d, want 503 when schemaLoader is nil", path, w.Code)
		}
	}
}

func TestCDASchemaController_DocumentTypeEndpoints_HappyPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	loader, err := cdaSchema.NewCDASchemaLoader("../cda/schemas")
	if err != nil {
		t.Fatalf("failed to load CDA schema: %v", err)
	}
	cc := &CDASchemaController{schemaLoader: loader}
	r := gin.New()
	cc.RegisterRoutes(r.Group("/api/cda"))

	req := httptest.NewRequest(http.MethodGet, "/api/cda/document-types", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("/document-types: status = %d, body = %s", w.Code, w.Body.String())
	}
	var listResp struct {
		Success       bool     `json:"success"`
		DocumentTypes []string `json:"documentTypes"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("failed to decode /document-types response: %v", err)
	}
	if !listResp.Success || len(listResp.DocumentTypes) == 0 {
		t.Fatalf("expected a non-empty document type list, got: %+v", listResp)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/cda/document-types/CCD/requirements", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("/document-types/CCD/requirements: status = %d, body = %s", w.Code, w.Body.String())
	}
	var reqResp struct {
		Success      bool `json:"success"`
		Requirements struct {
			DocumentType string           `json:"documentType"`
			Sections     []map[string]any `json:"sections"`
			HeaderGroups map[string]any   `json:"headerGroups"`
		} `json:"requirements"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &reqResp); err != nil {
		t.Fatalf("failed to decode /requirements response: %v", err)
	}
	if !reqResp.Success || reqResp.Requirements.DocumentType != "CCD" || len(reqResp.Requirements.Sections) == 0 {
		t.Fatalf("expected populated CCD requirements, got: %+v", reqResp)
	}
	for _, group := range []string{"patient", "author", "custodian"} {
		if _, ok := reqResp.Requirements.HeaderGroups[group]; !ok {
			t.Errorf("expected headerGroups to include %q", group)
		}
	}

	req = httptest.NewRequest(http.MethodGet, "/api/cda/document-types/Not-A-Real-Type/requirements", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("unknown document type: status = %d, want 404", w.Code)
	}
}
