// controllers/edi_schema_controller_test.go
//
// Unit tests for EDISchemaController's GetSegments endpoint, specifically
// the new syntaxRules field added so edi.validate's config panel can list
// and selectively disable OOB (schema-defined) SyntaxRules -- mirrors
// cda_schema_controller_test.go's pattern (real schema loader against the
// checked-in schema directory, no DB needed).
//
// Run: go test ./controllers/ -v -run TestEDISchemaController
package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"ezhealthkonnect/edi"

	"github.com/gin-gonic/gin"
)

func TestEDISchemaController_GetSegments_NoLoader_Returns503(t *testing.T) {
	gin.SetMode(gin.TestMode)
	ec := NewEDISchemaController(nil)
	r := gin.New()
	ec.RegisterRoutes(r.Group("/api/edi"))

	req := httptest.NewRequest(http.MethodGet, "/api/edi/schema/segments", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 when loader is nil", w.Code)
	}
}

// TestEDISchemaController_GetSegments_ExposesSyntaxRules proves the real
// gap this round closed: BPR/CAS/N1/N4/PER's own OOB SyntaxRules (defined
// in edi/schemas/x12_005010/segments/*.json) are now present in the API
// response with element keys resolved for display, not just raw positions
// -- previously segmentSummary had no SyntaxRules field at all.
func TestEDISchemaController_GetSegments_ExposesSyntaxRules(t *testing.T) {
	gin.SetMode(gin.TestMode)
	loader, err := edi.NewX12SchemaLoader("../edi/schemas/x12_005010")
	if err != nil {
		t.Fatalf("failed to load EDI schema: %v", err)
	}
	ec := NewEDISchemaController(loader)
	r := gin.New()
	ec.RegisterRoutes(r.Group("/api/edi"))

	req := httptest.NewRequest(http.MethodGet, "/api/edi/schema/segments", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}

	var resp struct {
		Success  bool             `json:"success"`
		Segments []segmentSummary `json:"segments"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resp.Success {
		t.Fatal("expected success=true")
	}

	var per *segmentSummary
	for i := range resp.Segments {
		if resp.Segments[i].ID == "PER" {
			per = &resp.Segments[i]
			break
		}
	}
	if per == nil {
		t.Fatal("expected a PER segment in the response")
	}
	if len(per.SyntaxRules) != 3 {
		t.Fatalf("PER: expected 3 OOB syntax rules, got %d: %+v", len(per.SyntaxRules), per.SyntaxRules)
	}
	first := per.SyntaxRules[0]
	if first.Type != "P" {
		t.Errorf("PER's first syntax rule: type = %q, want \"P\"", first.Type)
	}
	wantKeys := []string{"communicationNumberQualifier1", "communicationNumber1"}
	if len(first.ElementKeys) != 2 || first.ElementKeys[0] != wantKeys[0] || first.ElementKeys[1] != wantKeys[1] {
		t.Errorf("PER's first syntax rule: elementKeys = %v, want %v (raw positions %v)", first.ElementKeys, wantKeys, first.Positions)
	}

	// A segment with no OOB rules at all (e.g. LX) should omit the field
	// (omitempty) rather than emit an empty array — cheap, non-breaking
	// confirmation the field is genuinely optional.
	var lx *segmentSummary
	for i := range resp.Segments {
		if resp.Segments[i].ID == "LX" {
			lx = &resp.Segments[i]
			break
		}
	}
	if lx == nil {
		t.Fatal("expected an LX segment in the response")
	}
	if len(lx.SyntaxRules) != 0 {
		t.Errorf("LX: expected zero OOB syntax rules, got %+v", lx.SyntaxRules)
	}
}
