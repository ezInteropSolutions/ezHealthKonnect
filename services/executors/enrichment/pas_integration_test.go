package enrichment

// ─────────────────────────────────────────────────────────────────────────────
// Da Vinci PAS Pipeline Integration Test
//
// Runs all four pipeline zones end-to-end WITHOUT a real database or Go backend:
//   Zone 1  PAS envelope mapping      (FieldMappingExecutor — pas_envelope_mapping alias)
//   Zone 2  Build FHIR resources      (3× ScriptEnrichmentExecutor)
//   Zone 2  Assemble PAS bundle       (ScriptEnrichmentExecutor)
//   Zone 3  Submit to payer           (APIEnrichmentExecutor with mock httptest server)
//   Zone 4  Parse ClaimResponse       (ScriptEnrichmentExecutor)
//
// The mock payer server returns realistic ClaimResponse bundles so that the
// decision-parsing script can be exercised in a fully realistic data flow.
//
// Run: go test ./services/executors/enrichment/ -v -run TestPASIntegration
// ─────────────────────────────────────────────────────────────────────────────

import (
	"context"
	"encoding/json"
	"ezhealthkonnect/models"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ─────────────────────────────────────────────────────────────────────────────
// Mock payer server fixtures
// ─────────────────────────────────────────────────────────────────────────────

// mockPayerResponse builds a minimal Da Vinci PAS ClaimResponse with the given
// reviewAction code (A1=AA, A2=CP, A3=AD, A4=AD-modified, A6=PE).
func mockPayerResponse(reviewActionCode string) map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "ClaimResponse",
		"id":           "cr-mock-001",
		"status":       "active",
		"use":          "preauthorization",
		"item": []interface{}{
			map[string]interface{}{
				"itemSequence": 1,
				"extension": []interface{}{
					map[string]interface{}{
						"url": "http://hl7.org/fhir/us/davinci-pas/StructureDefinition/extension-reviewAction",
						"extension": []interface{}{
							map[string]interface{}{
								"url": "code",
								"valueCodeableConcept": map[string]interface{}{
									"coding": []interface{}{
										map[string]interface{}{
											"system": "https://codesystem.x12.org/005010/306",
											"code":   reviewActionCode,
										},
									},
								},
							},
							map[string]interface{}{
								"url": "reasonCode",
								"valueCodeableConcept": map[string]interface{}{
									"coding": []interface{}{
										map[string]interface{}{
											"system":  "https://codesystem.x12.org/005010/901",
											"code":    "4",
											"display": "Not medically necessary",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Test helpers
// ─────────────────────────────────────────────────────────────────────────────

// stepOutput safely retrieves _stepOutput from executor result.
func stepOutput(t *testing.T, result map[string]interface{}, label string) map[string]interface{} {
	t.Helper()
	out, ok := result["_stepOutput"].(map[string]interface{})
	if !ok {
		t.Fatalf("[%s] _stepOutput missing or not a map", label)
	}
	return out
}

// runScript executes a ScriptEnrichmentExecutor step and returns the full result.
func runScript(t *testing.T, stepName, script string, input map[string]interface{}) map[string]interface{} {
	t.Helper()
	exec := NewScriptEnrichmentExecutor()
	step := &models.TransformationStep{
		StepName: stepName,
		StepAlias: strPtr(toAlias(stepName)),
		StepType: "enrichment.script",
		Enabled:  true,
		Config:   map[string]interface{}{"script": script},
	}
	result, err := exec.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("[%s] script error: %v", stepName, err)
	}
	return result
}

func strPtr(s string) *string { return &s }

// toAlias converts a display name to a step alias (lower, spaces→underscores).
func toAlias(name string) string {
	out := ""
	for _, c := range name {
		switch {
		case c >= 'A' && c <= 'Z':
			out += string(rune(c + 32))
		case c == ' ':
			out += "_"
		default:
			out += string(c)
		}
	}
	return out
}

// injectStepOutput merges a step's _stepOutput into the pipeline's `steps` map
// so downstream steps can reference it as steps.<alias>.step_output.<field>.
func injectStepOutput(data map[string]interface{}, alias string, output map[string]interface{}) {
	steps, ok := data["steps"].(map[string]interface{})
	if !ok {
		steps = map[string]interface{}{}
		data["steps"] = steps
	}
	steps[alias] = map[string]interface{}{"step_output": output}
}

// ─────────────────────────────────────────────────────────────────────────────
// Integration test scenarios
// ─────────────────────────────────────────────────────────────────────────────

// runPASPipeline runs Zones 1-3 of the PAS pipeline and returns the data map
// after the payer submission step (ready for Zone 4 parsing).
// payerServer should be a mock httptest.Server.
func runPASPipeline(t *testing.T, inbound map[string]interface{}, payerServer *httptest.Server) map[string]interface{} {
	t.Helper()
	ctx := context.Background()

	// ── Zone 1: PAS Envelope Mapping ──────────────────────────────────────────
	// Simulate field_mapping step: the user has pre-mapped literal values
	// (in a real pipeline these come from the source connector's message).
	// We inject _pas_envelope directly to replicate what field_mapping writes.
	data := map[string]interface{}{}
	for k, v := range inbound {
		data[k] = v
	}

	// ── Zone 2: Build FHIR Patient ────────────────────────────────────────────
	result := runScript(t, "Build FHIR Patient", pasPatientScript, data)
	patOut := stepOutput(t, result, "Build FHIR Patient")
	for k, v := range result {
		data[k] = v
	}
	injectStepOutput(data, "build_patient", patOut)

	// ── Zone 2: Build FHIR Coverage ───────────────────────────────────────────
	result = runScript(t, "Build FHIR Coverage", pasCoverageScript, data)
	covOut := stepOutput(t, result, "Build FHIR Coverage")
	for k, v := range result {
		data[k] = v
	}
	injectStepOutput(data, "build_coverage", covOut)

	// ── Zone 2: Build Provider Resources ─────────────────────────────────────
	result = runScript(t, "Build Provider Resources", pasProviderScript, data)
	provOut := stepOutput(t, result, "Build Provider Resources")
	for k, v := range result {
		data[k] = v
	}
	injectStepOutput(data, "build_provider", provOut)

	// ── Zone 2: Assemble PAS Bundle ────────────────────────────────────────────
	result = runScript(t, "Assemble PAS Bundle", pasBundleScript, data)
	bundleOut := stepOutput(t, result, "Assemble PAS Bundle")
	for k, v := range result {
		data[k] = v
	}
	injectStepOutput(data, "assemble_pas_bundle", bundleOut)

	// ── Zone 3: Submit to Payer (enrichment.api with bodyRef) ─────────────────
	apiExec := NewAPIEnrichmentExecutor()
	apiStep := &models.TransformationStep{
		StepName: "Submit to Payer",
		StepAlias: strPtr("submit_to_payer"),
		StepType: "enrichment.api",
		Enabled:  true,
		ID:       "step-submit-payer",
		Config: map[string]interface{}{
			"method":   "POST",
			"endpoint": payerServer.URL + "/fhir/R4/$submit",
			"headers": map[string]interface{}{
				"Content-Type": "application/fhir+json",
				"Accept":       "application/fhir+json",
			},
			"bodyRef":   "steps.assemble_pas_bundle.step_output.pas_bundle",
			"timeoutMs": 10000,
		},
	}
	data, err := apiExec.Execute(ctx, apiStep, data)
	if err != nil {
		t.Fatalf("[Submit to Payer] API call failed: %v", err)
	}
	payerOut := stepOutput(t, data, "Submit to Payer")
	injectStepOutput(data, "submit_to_payer", payerOut)

	return data
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-PAS-INT-001: Happy path — routine request, payer returns AA (Approved)
// ─────────────────────────────────────────────────────────────────────────────
func TestPASIntegration_HappyPath_Approved(t *testing.T) {
	// Mock payer returns A1 = Approved
	payer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/fhir/R4/$submit" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}

		// Verify Content-Type header
		ct := r.Header.Get("Content-Type")
		if ct != "application/fhir+json" {
			t.Errorf("expected Content-Type application/fhir+json, got %s", ct)
		}

		// Verify request body contains a FHIR Bundle
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("could not decode request body: %v", err)
		}
		if body["resourceType"] != "Bundle" {
			t.Errorf("expected Bundle in request body, got %v", body["resourceType"])
		}

		w.Header().Set("Content-Type", "application/fhir+json")
		json.NewEncoder(w).Encode(mockPayerResponse("A1"))
	}))
	defer payer.Close()

	data := runPASPipeline(t, fullPASInput(), payer)

	// ── Zone 4: Parse ClaimResponse ────────────────────────────────────────────
	result := runScript(t, "Parse Claim Response", pasClaimResponseScript, data)
	parsed := stepOutput(t, result, "Parse Claim Response")

	if parsed["decision"] != "AA" {
		t.Errorf("expected decision=AA, got %v", parsed["decision"])
	}
	if parsed["review_action"] != "A1" {
		t.Errorf("expected review_action=A1, got %v", parsed["review_action"])
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-PAS-INT-002: Payer denies (A3)
// ─────────────────────────────────────────────────────────────────────────────
func TestPASIntegration_Denied(t *testing.T) {
	payer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/fhir+json")
		json.NewEncoder(w).Encode(mockPayerResponse("A3"))
	}))
	defer payer.Close()

	data := runPASPipeline(t, fullPASInput(), payer)
	result := runScript(t, "Parse Claim Response", pasClaimResponseScript, data)
	parsed := stepOutput(t, result, "Parse Claim Response")

	if parsed["decision"] != "AD" {
		t.Errorf("expected decision=AD, got %v", parsed["decision"])
	}
	// Denial reason should be populated
	if parsed["denial_reason"] == "" || parsed["denial_reason"] == nil {
		t.Error("expected denial_reason to be populated for AD decision")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-PAS-INT-003: Payer partially approves (A2 = CP)
// ─────────────────────────────────────────────────────────────────────────────
func TestPASIntegration_PartialApproval(t *testing.T) {
	payer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/fhir+json")
		json.NewEncoder(w).Encode(mockPayerResponse("A2"))
	}))
	defer payer.Close()

	data := runPASPipeline(t, fullPASInput(), payer)
	result := runScript(t, "Parse Claim Response", pasClaimResponseScript, data)
	parsed := stepOutput(t, result, "Parse Claim Response")

	if parsed["decision"] != "CP" {
		t.Errorf("expected decision=CP, got %v", parsed["decision"])
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-PAS-INT-004: Payer pends review (A6 = PE)
// ─────────────────────────────────────────────────────────────────────────────
func TestPASIntegration_Pended(t *testing.T) {
	payer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/fhir+json")
		json.NewEncoder(w).Encode(mockPayerResponse("A6"))
	}))
	defer payer.Close()

	data := runPASPipeline(t, fullPASInput(), payer)
	result := runScript(t, "Parse Claim Response", pasClaimResponseScript, data)
	parsed := stepOutput(t, result, "Parse Claim Response")

	if parsed["decision"] != "PE" {
		t.Errorf("expected decision=PE, got %v", parsed["decision"])
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-PAS-INT-005: Request body sent to payer contains Claim with use=preauthorization
// ─────────────────────────────────────────────────────────────────────────────
func TestPASIntegration_RequestBodyIsConformantBundle(t *testing.T) {
	var capturedBody map[string]interface{}

	payer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/fhir+json")
		json.NewEncoder(w).Encode(mockPayerResponse("A1"))
	}))
	defer payer.Close()

	runPASPipeline(t, fullPASInput(), payer)

	if capturedBody == nil {
		t.Fatal("payer received no request body")
	}
	if capturedBody["resourceType"] != "Bundle" {
		t.Errorf("expected Bundle in request, got %v", capturedBody["resourceType"])
	}

	// Verify the bundle contains a Claim with use=preauthorization
	entries, _ := capturedBody["entry"].([]interface{})
	claimFound := false
	for _, e := range entries {
		entry := e.(map[string]interface{})
		res, _ := entry["resource"].(map[string]interface{})
		if res["resourceType"] == "Claim" {
			claimFound = true
			if res["use"] != "preauthorization" {
				t.Errorf("Claim.use must be preauthorization, got %v", res["use"])
			}
		}
	}
	if !claimFound {
		t.Error("no Claim resource found in submitted bundle")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-PAS-INT-006: Urgent request produces stat priority in submitted Claim
// ─────────────────────────────────────────────────────────────────────────────
func TestPASIntegration_UrgentRequest_StatPriority(t *testing.T) {
	var capturedBody map[string]interface{}

	payer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&capturedBody)
		w.Header().Set("Content-Type", "application/fhir+json")
		json.NewEncoder(w).Encode(mockPayerResponse("A1"))
	}))
	defer payer.Close()

	input := fullPASInput()
	input["_pas_envelope"].(map[string]interface{})["request"].(map[string]interface{})["urgency"] = "urgent"

	runPASPipeline(t, input, payer)

	entries, _ := capturedBody["entry"].([]interface{})
	for _, e := range entries {
		entry := e.(map[string]interface{})
		res, _ := entry["resource"].(map[string]interface{})
		if res["resourceType"] != "Claim" {
			continue
		}
		priority, _ := res["priority"].(map[string]interface{})
		codings, _ := priority["coding"].([]interface{})
		if len(codings) == 0 {
			t.Fatal("expected priority coding on Claim")
		}
		c := codings[0].(map[string]interface{})
		if c["code"] != "stat" {
			t.Errorf("expected stat priority for urgent request, got %v", c["code"])
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-PAS-INT-007: Payer returns 500 — pipeline returns error, data intact
// ─────────────────────────────────────────────────────────────────────────────
func TestPASIntegration_PayerServerError(t *testing.T) {
	payer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))
	defer payer.Close()

	ctx := context.Background()
	data := fullPASInput()

	// Run zones 1-2 manually to build the bundle
	result := runScript(t, "Build FHIR Patient",    pasPatientScript,  data)
	patOut := stepOutput(t, result, "Build FHIR Patient")
	for k, v := range result { data[k] = v }
	injectStepOutput(data, "build_patient", patOut)

	result = runScript(t, "Build FHIR Coverage", pasCoverageScript, data)
	covOut := stepOutput(t, result, "Build FHIR Coverage")
	for k, v := range result { data[k] = v }
	injectStepOutput(data, "build_coverage", covOut)

	result = runScript(t, "Build Provider Resources", pasProviderScript, data)
	provOut := stepOutput(t, result, "Build Provider Resources")
	for k, v := range result { data[k] = v }
	injectStepOutput(data, "build_provider", provOut)

	result = runScript(t, "Assemble PAS Bundle", pasBundleScript, data)
	bundleOut := stepOutput(t, result, "Assemble PAS Bundle")
	for k, v := range result { data[k] = v }
	injectStepOutput(data, "assemble_pas_bundle", bundleOut)

	// Zone 3 — expect error from 500 response
	apiExec := NewAPIEnrichmentExecutor()
	apiStep := &models.TransformationStep{
		StepName: "Submit to Payer",
		StepType: "enrichment.api",
		Enabled:  true,
		ID:       "step-submit-error",
		Config: map[string]interface{}{
			"method":   "POST",
			"endpoint": payer.URL + "/fhir/R4/$submit",
			"bodyRef":  "steps.assemble_pas_bundle.step_output.pas_bundle",
		},
	}
	_, err := apiExec.Execute(ctx, apiStep, data)
	if err == nil {
		t.Error("expected error when payer returns 500, got nil")
	}

	// Original PAS envelope should still be intact (data not wiped on error)
	if data["_pas_envelope"] == nil {
		t.Error("_pas_envelope wiped on API error — original data should be preserved")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-PAS-INT-008: Minimal required fields only — no optional fields
// ─────────────────────────────────────────────────────────────────────────────
func TestPASIntegration_MinimalRequiredFields(t *testing.T) {
	payer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/fhir+json")
		json.NewEncoder(w).Encode(mockPayerResponse("A1"))
	}))
	defer payer.Close()

	// Only required fields — no gender, planId, groupNumber, facilityNpi, urgency, dates, quantity
	minInput := map[string]interface{}{
		"_pas_envelope": map[string]interface{}{
			"patient": map[string]interface{}{
				"firstName": "John",
				"lastName":  "Doe",
				"dob":       "1985-01-15",
				"memberId":  "MEM-MIN-001",
			},
			"coverage": map[string]interface{}{
				"payerId": "0000000001",
			},
			"provider": map[string]interface{}{
				"npi": "1234567890",
			},
			"request": map[string]interface{}{
				"serviceCode":    "99214",
				"diagnosisCodes": []interface{}{"J18.9"},
			},
		},
	}

	data := runPASPipeline(t, minInput, payer)
	result := runScript(t, "Parse Claim Response", pasClaimResponseScript, data)
	parsed := stepOutput(t, result, "Parse Claim Response")

	if parsed["decision"] != "AA" {
		t.Errorf("expected AA even with minimal fields, got %v", parsed["decision"])
	}
}
