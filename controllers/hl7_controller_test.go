// controllers/hl7_controller_test.go
//
// Integration tests for the hl7/validator conformance checks wired into
// HL7Controller.ParseMessage (/api/hl7/parse) and ValidateMessage
// (/api/hl7/validate) — confirms the new validator's findings actually reach
// the HTTP response's validationErrors array at each ValidationLevel, using
// the same real compiled schema fixtures as hl7/validator's own tests.
//
// Run: go test ./controllers/ -v -run TestHL7Controller
package controllers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"ezhealthkonnect/config"
	"ezhealthkonnect/hl7"

	"github.com/gin-gonic/gin"
)

const testHL7SchemaDirForController = "../schemas/hl7"

// adtA01MissingPID omits the required PID segment from an otherwise valid
// ADT^A01 message — the case hl7/validator/segments_test.go also exercises
// directly against the package, here confirmed end-to-end over HTTP.
const adtA01MissingPID = "MSH|^~\\&|SENDING_APP|SENDING_FAC|RECEIVING_APP|RECEIVING_FAC|20240101120000||ADT^A01|MSG001|P|2.5.1\r" +
	"EVN|A01|20240101120000\r" +
	"PV1|1|I|WARD^101^A^Hospital|E||12345^Smith^Jane^A^MD|67890^Jones^Bob^C^MD||MED"

func newHL7TestController() *HL7Controller {
	hl7.InitRealSchemaLoader(testHL7SchemaDirForController)
	return NewHL7Controller(&config.Config{HL7ValidationLevel: "basic"})
}

func postHL7Parse(t *testing.T, ctrl *HL7Controller, body map[string]interface{}) map[string]interface{} {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/hl7/parse", ctrl.ParseMessage)

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/hl7/parse", strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	return resp
}

func validationErrorCodes(t *testing.T, resp map[string]interface{}) []string {
	t.Helper()
	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("response has no data object: %+v", resp)
	}
	raw, _ := data["validationErrors"].([]interface{})
	codes := make([]string, 0, len(raw))
	for _, v := range raw {
		if m, ok := v.(map[string]interface{}); ok {
			if code, ok := m["code"].(string); ok {
				codes = append(codes, code)
			}
		}
	}
	return codes
}

func TestHL7Controller_ParseMessage_BasicLevel_DoesNotRunNewValidator(t *testing.T) {
	ctrl := newHL7TestController()
	resp := postHL7Parse(t, ctrl, map[string]interface{}{
		"rawMessage": adtA01MissingPID,
		"useEnhanced": true,
	})
	codes := validationErrorCodes(t, resp)
	for _, c := range codes {
		if c == hl7.ErrorCodeMissingRequiredSegment {
			t.Errorf("expected no MISSING_REQUIRED_SEGMENT at basic level (request omitted validationLevel), got codes: %v", codes)
		}
	}
}

func TestHL7Controller_ParseMessage_StandardLevel_FlagsMissingSegment(t *testing.T) {
	ctrl := newHL7TestController()
	resp := postHL7Parse(t, ctrl, map[string]interface{}{
		"rawMessage":      adtA01MissingPID,
		"useEnhanced":     true,
		"validationLevel": "standard",
	})
	codes := validationErrorCodes(t, resp)
	found := false
	for _, c := range codes {
		if c == hl7.ErrorCodeMissingRequiredSegment {
			found = true
		}
	}
	if !found {
		t.Errorf("expected MISSING_REQUIRED_SEGMENT at standard level, got codes: %v", codes)
	}
}

func TestHL7Controller_ValidateMessage_StandardLevel_FlagsMissingSegment(t *testing.T) {
	ctrl := newHL7TestController()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/hl7/validate", ctrl.ValidateMessage)

	payload, _ := json.Marshal(map[string]interface{}{
		"rawMessage":      adtA01MissingPID,
		"validationLevel": "standard",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/hl7/validate", strings.NewReader(string(payload)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if valid, _ := resp["valid"].(bool); valid {
		t.Error("expected valid=false — PID is missing and validationLevel=standard was requested")
	}
	errs, _ := resp["errors"].([]interface{})
	found := false
	for _, v := range errs {
		if m, ok := v.(map[string]interface{}); ok && m["code"] == hl7.ErrorCodeMissingRequiredSegment {
			found = true
		}
	}
	if !found {
		t.Errorf("expected MISSING_REQUIRED_SEGMENT in /api/hl7/validate response, got: %+v", errs)
	}
}
