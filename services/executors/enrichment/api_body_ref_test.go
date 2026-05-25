package enrichment

// ─────────────────────────────────────────────────────────────────────────────
// bodyRef Tests for APIEnrichmentExecutor
//
// TC-BREF-001  bodyRef resolves a nested object and sends it as request body
// TC-BREF-002  bodyRef resolves a deeply nested FHIR Bundle correctly
// TC-BREF-003  bodyRef nil path falls back to empty body (no panic)
// TC-BREF-004  bodyRef takes priority over requestBody when both are set
// TC-BREF-005  requestBody still works when bodyRef is absent
// TC-BREF-006  bodyRef resolving a scalar wraps it as the body
//
// Run: go test ./services/executors/enrichment/ -v -run TestBodyRef
// ─────────────────────────────────────────────────────────────────────────────

import (
	"context"
	"encoding/json"
	"ezhealthkonnect/models"
	"net/http"
	"net/http/httptest"
	"testing"
)

// captureBodyServer starts a mock HTTP server that decodes the request body
// and stores it in the provided pointer, then returns 200 with {"ok":true}.
func captureBodyServer(t *testing.T, captured *map[string]interface{}) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			// Body might be a non-object — just return ok
			body = map[string]interface{}{"_parse_error": err.Error()}
		}
		*captured = body
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
	}))
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-BREF-001
// ─────────────────────────────────────────────────────────────────────────────

// TC-BREF-001: bodyRef resolves a nested object and it arrives at the server
func TestBodyRef_ResolvesNestedObject(t *testing.T) {
	var received map[string]interface{}
	server := captureBodyServer(t, &received)
	defer server.Close()

	executor := NewAPIEnrichmentExecutor()
	step := &models.TransformationStep{
		ID:       "step-bref-001",
		StepName: "BodyRef Test",
		StepType: "enrichment.api",
		Enabled:  true,
		Config: map[string]interface{}{
			"method":   "POST",
			"endpoint": server.URL + "/submit",
			"bodyRef":  "payload.fhir_bundle",
		},
	}

	fhirBundle := map[string]interface{}{
		"resourceType": "Bundle",
		"type":         "collection",
		"id":           "test-bundle-001",
	}
	inputData := map[string]interface{}{
		"payload": map[string]interface{}{
			"fhir_bundle": fhirBundle,
		},
	}

	_, err := executor.Execute(context.Background(), step, inputData)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if received == nil {
		t.Fatal("server received no body")
	}
	if received["resourceType"] != "Bundle" {
		t.Errorf("expected Bundle in body, got %v", received["resourceType"])
	}
	if received["id"] != "test-bundle-001" {
		t.Errorf("expected id=test-bundle-001, got %v", received["id"])
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-BREF-002
// ─────────────────────────────────────────────────────────────────────────────

// TC-BREF-002: bodyRef using the exact PAS pipeline path pattern
func TestBodyRef_PASSiblingStepPath(t *testing.T) {
	var received map[string]interface{}
	server := captureBodyServer(t, &received)
	defer server.Close()

	executor := NewAPIEnrichmentExecutor()
	step := &models.TransformationStep{
		ID:       "step-bref-002",
		StepName: "PAS Submit",
		StepType: "enrichment.api",
		Enabled:  true,
		Config: map[string]interface{}{
			"method":   "POST",
			"endpoint": server.URL + "/fhir/R4/$submit",
			"bodyRef":  "steps.assemble_pas_bundle.step_output.pas_bundle",
		},
	}

	pasBundle := map[string]interface{}{
		"resourceType": "Bundle",
		"type":         "collection",
		"meta": map[string]interface{}{
			"profile": []interface{}{
				"http://hl7.org/fhir/us/davinci-pas/StructureDefinition/profile-pas-request-bundle",
			},
		},
		"entry": []interface{}{
			map[string]interface{}{
				"resource": map[string]interface{}{
					"resourceType": "Claim",
					"use":          "preauthorization",
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"steps": map[string]interface{}{
			"assemble_pas_bundle": map[string]interface{}{
				"step_output": map[string]interface{}{
					"pas_bundle": pasBundle,
				},
			},
		},
	}

	_, err := executor.Execute(context.Background(), step, inputData)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if received["resourceType"] != "Bundle" {
		t.Errorf("expected Bundle via deep path, got %v", received["resourceType"])
	}
	// Entry should be present
	entries, _ := received["entry"].([]interface{})
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-BREF-003
// ─────────────────────────────────────────────────────────────────────────────

// TC-BREF-003: bodyRef that resolves to nil sends empty body without panicking
func TestBodyRef_NilPathNoError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Accept any body
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
	}))
	defer server.Close()

	executor := NewAPIEnrichmentExecutor()
	step := &models.TransformationStep{
		ID:       "step-bref-003",
		StepName: "Nil BodyRef",
		StepType: "enrichment.api",
		Enabled:  true,
		Config: map[string]interface{}{
			"method":   "POST",
			"endpoint": server.URL + "/submit",
			"bodyRef":  "this.path.does.not.exist",
		},
	}

	// Should not panic or hard-error on missing path
	_, err := executor.Execute(context.Background(), step, map[string]interface{}{})
	if err != nil {
		t.Errorf("expected no error for nil bodyRef, got: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-BREF-004
// ─────────────────────────────────────────────────────────────────────────────

// TC-BREF-004: bodyRef takes priority over requestBody when both present
func TestBodyRef_TakesPriorityOverRequestBody(t *testing.T) {
	var received map[string]interface{}
	server := captureBodyServer(t, &received)
	defer server.Close()

	executor := NewAPIEnrichmentExecutor()
	step := &models.TransformationStep{
		ID:       "step-bref-004",
		StepName: "Priority Test",
		StepType: "enrichment.api",
		Enabled:  true,
		Config: map[string]interface{}{
			"method":   "POST",
			"endpoint": server.URL + "/submit",
			// Both bodyRef and requestBody — bodyRef must win
			"bodyRef": "preferred_body",
			"requestBody": map[string]interface{}{
				"source": "requestBody",
			},
		},
	}

	inputData := map[string]interface{}{
		"preferred_body": map[string]interface{}{
			"source": "bodyRef",
		},
	}

	_, err := executor.Execute(context.Background(), step, inputData)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if received["source"] != "bodyRef" {
		t.Errorf("expected source=bodyRef (bodyRef priority), got %v", received["source"])
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-BREF-005
// ─────────────────────────────────────────────────────────────────────────────

// TC-BREF-005: requestBody still works when bodyRef absent (no regression)
func TestBodyRef_RequestBodyStillWorksWhenNoBodyRef(t *testing.T) {
	var received map[string]interface{}
	server := captureBodyServer(t, &received)
	defer server.Close()

	executor := NewAPIEnrichmentExecutor()
	step := &models.TransformationStep{
		ID:       "step-bref-005",
		StepName: "requestBody Fallback",
		StepType: "enrichment.api",
		Enabled:  true,
		Config: map[string]interface{}{
			"method":   "POST",
			"endpoint": server.URL + "/submit",
			"requestBody": map[string]interface{}{
				"source": "requestBody",
				"value":  "hello",
			},
		},
	}

	_, err := executor.Execute(context.Background(), step, map[string]interface{}{})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if received["source"] != "requestBody" {
		t.Errorf("expected source=requestBody, got %v", received["source"])
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-BREF-006
// ─────────────────────────────────────────────────────────────────────────────

// TC-BREF-006: bodyRef that resolves to a non-map value (string) is sent as body
func TestBodyRef_ScalarValueSentAsBody(t *testing.T) {
	// Use a raw capture since body won't be JSON-decodable to map
	var capturedRaw string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		capturedRaw = string(buf[:n])
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
	}))
	defer server.Close()

	executor := NewAPIEnrichmentExecutor()
	step := &models.TransformationStep{
		ID:       "step-bref-006",
		StepName: "Scalar BodyRef",
		StepType: "enrichment.api",
		Enabled:  true,
		Config: map[string]interface{}{
			"method":   "POST",
			"endpoint": server.URL + "/submit",
			"bodyRef":  "raw_payload",
		},
	}

	_, err := executor.Execute(context.Background(), step, map[string]interface{}{
		"raw_payload": "HL7-raw-message-here",
	})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if capturedRaw == "" {
		t.Error("expected non-empty body for scalar bodyRef")
	}
}
