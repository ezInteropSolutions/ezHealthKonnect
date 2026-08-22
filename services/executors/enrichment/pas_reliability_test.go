package enrichment

// ─────────────────────────────────────────────────────────────────────────────
// Da Vinci PAS Pipeline — Reliability Test Suite
//
// Closes the test-coverage gap that let three real bugs ship undetected in
// the davinci-pas-r4 template: validate_pas_bundle, route_on_decision, and
// deliver_decision were never exercised by pas_integration_test.go's
// runPASPipeline (it stops after submit_to_payer / parse_claim_response).
// Fixed in V210 (validate_pas_bundle config keys) and V211 (route_on_decision
// action type, deliver_decision contentField + retry), plus a Go-level fix
// to fhir_validation_executor.go's source_field resolution.
//
// Run: go test ./services/executors/enrichment/ -v -run TestPASReliability
// ─────────────────────────────────────────────────────────────────────────────

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors/control"
	"ezhealthkonnect/services/executors/transform"
	"ezhealthkonnect/services/executors/validation"
)

// buildAssembledBundle runs V212's Zone 2 chain (derive -> 5x fhir.build ->
// payload.builder assemble -> stamp profile, config loaded directly from
// database/migrations/V212__PAS_Template_Rebuild_On_FHIR_Builder.sql via
// runPASZone2 in pas_integration_test.go) and returns the flat pipeline data
// with steps.stamp_bundle_profile.step_output.pas_bundle populated — the
// same addressing scheme downstream steps use in production (confirmed by
// tracing executors.GetNestedValue's generic dot-path traversal, which walks
// the original un-wrapped data for multi-part paths regardless of whether
// the caller also wraps it under "message").
func buildAssembledBundle(t *testing.T) map[string]interface{} {
	t.Helper()
	data := map[string]interface{}{}
	for k, v := range fullPASInput() {
		data[k] = v
	}
	return runPASZone2(t, data)
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-PAS-REL-001: validate_pas_bundle finds the bundle and the PAS
// constraints actually evaluate (Bug 4 — source_field resolution).
// ─────────────────────────────────────────────────────────────────────────────
func TestPASReliability_ValidatePASBundle_FindsBundleAndValidates(t *testing.T) {
	data := buildAssembledBundle(t)

	exec := validation.NewFHIRValidationExecutor()
	step := &models.TransformationStep{
		StepName:  "Validate PAS Bundle",
		StepAlias: strPtr("validate_pas_bundle"),
		StepType:  "fhir_validation",
		Enabled:   true,
		Config: map[string]interface{}{
			"profile":             "davinci-pas",
			"validation_level":    "strict",
			"source_field":        "steps.stamp_bundle_profile.step_output.pas_bundle",
			"fail_on_error":       false,
			"required_resources":  []interface{}{"Claim", "Patient", "Coverage"},
		},
	}

	result, err := exec.Execute(context.Background(), step, data)
	if err != nil {
		t.Fatalf("validate_pas_bundle returned an error: %v", err)
	}
	out := stepOutput(t, result, "validate_pas_bundle")

	// Before the fix, this step always short-circuited with exactly one
	// error: "No FHIR data found...". Assert that specific failure mode is
	// gone, then assert the step actually found and evaluated resources.
	// NOTE: out["errors"] is a raw []string here (this test calls Execute()
	// directly, bypassing the pipeline engine's JSON round-trip) -- a plain
	// `.([]interface{})` assertion fails silently on that concrete type,
	// which would make this whole check a silent no-op. asStringSlicePAS
	// handles both shapes.
	for _, s := range asStringSlicePAS(out["errors"]) {
		if strings.Contains(s, "No FHIR data found") {
			t.Fatalf("validate_pas_bundle still can't find the bundle: %v", out["errors"])
		}
	}

	resourceCount, _ := out["resource_count"].(int)
	if resourceCount == 0 {
		// json round-trip in some code paths turns this into float64
		if f, ok := out["resource_count"].(float64); ok {
			resourceCount = int(f)
		}
	}
	if resourceCount == 0 {
		t.Fatalf("expected resource_count > 0 (bundle should contain Claim/Patient/Coverage/Practitioner/Organization), got 0. Full output: %+v", out)
	}

	// Assert the 6 Da Vinci PAS constraints themselves (pas-claim-1/2/3,
	// pas-bundle-1/2/3 — fhir/r4/pas_profiles.go) evaluated and passed: this
	// is what bug 1 (level=strict) + bug 4 (source_field) together are
	// supposed to guarantee. Deliberately NOT asserting overall valid==true
	// here — the assembled bundle has a separate, pre-existing FHIR
	// reference-form defect (bundle entries use urn:uuid: fullUrls but
	// internal references use ResourceType/id form, which the base FHIR
	// Bundle spec requires to match) that strict-level validation now
	// correctly catches for the first time. That's a real finding, but a
	// different one from what this test is scoped to prove.
	for _, s := range asStringSlicePAS(out["errors"]) {
		if strings.Contains(s, "pas-claim") || strings.Contains(s, "pas-bundle") {
			t.Errorf("a PAS-specific constraint failed (should all pass for this spec-conformant bundle): %s", s)
		}
	}
}

// asStringSlicePAS handles both raw []string (calling an executor's
// Execute() directly, as this test file does -- no JSON round-trip) and
// []interface{} (the shape after going through encoding/json). A plain
// `.([]interface{})` type assertion silently fails (ok=false, nil result) on
// a genuine []string -- which is what fhir_validation_executor.go's
// buildOutput actually stores in variables["errors"] -- so using that
// assertion directly makes any check against the error list a silent no-op.
func asStringSlicePAS(v interface{}) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-PAS-REL-002/003: route_on_decision's set_value actions actually set
// _pa_status / _pa_status_label (Bug 5 — set_variable was a silent no-op).
// Config mirrors V211's fixed shape exactly (action: set_value, targetField).
// ─────────────────────────────────────────────────────────────────────────────
func routeOnDecisionConfig() map[string]interface{} {
	mkActions := func(status, label string) []interface{} {
		return []interface{}{
			map[string]interface{}{"action": "set_value", "targetField": "_pa_status", "value": status},
			map[string]interface{}{"action": "set_value", "targetField": "_pa_status_label", "value": label},
		}
	}
	return map[string]interface{}{
		"field": "steps.parse_claim_response.step_output.decision",
		"cases": []interface{}{
			map[string]interface{}{"value": "AA", "actions": mkActions("approved", "Prior Authorization Approved")},
			map[string]interface{}{"value": "AD", "actions": mkActions("denied", "Prior Authorization Denied")},
			map[string]interface{}{"value": "CP", "actions": mkActions("partial", "Prior Authorization Partially Approved")},
			map[string]interface{}{"value": "PE", "actions": mkActions("pended", "Prior Authorization Pended")},
		},
		"default": map[string]interface{}{
			"actions": mkActions("unknown", "Prior Authorization Decision Unknown"),
		},
	}
}

func TestPASReliability_RouteOnDecision_ApprovedSetsStatus(t *testing.T) {
	exec := control.NewSwitchCaseExecutor()
	step := &models.TransformationStep{
		StepName:  "Route on Decision",
		StepAlias: strPtr("route_on_decision"),
		StepType:  "switch_case",
		Enabled:   true,
		Config:    routeOnDecisionConfig(),
	}
	input := map[string]interface{}{
		"steps": map[string]interface{}{
			"parse_claim_response": map[string]interface{}{
				"step_output": map[string]interface{}{"decision": "AA"},
			},
		},
	}

	result, err := exec.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("route_on_decision returned an error: %v", err)
	}

	if got := result["_pa_status"]; got != "approved" {
		t.Errorf("expected _pa_status=approved, got %v (before the fix this was never set at all)", got)
	}
	if got := result["_pa_status_label"]; got != "Prior Authorization Approved" {
		t.Errorf("expected _pa_status_label='Prior Authorization Approved', got %v", got)
	}
}

func TestPASReliability_RouteOnDecision_DeniedSetsStatus(t *testing.T) {
	exec := control.NewSwitchCaseExecutor()
	step := &models.TransformationStep{
		StepName:  "Route on Decision",
		StepAlias: strPtr("route_on_decision"),
		StepType:  "switch_case",
		Enabled:   true,
		Config:    routeOnDecisionConfig(),
	}
	input := map[string]interface{}{
		"steps": map[string]interface{}{
			"parse_claim_response": map[string]interface{}{
				"step_output": map[string]interface{}{"decision": "AD"},
			},
		},
	}

	result, err := exec.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("route_on_decision returned an error: %v", err)
	}

	if got := result["_pa_status"]; got != "denied" {
		t.Errorf("expected _pa_status=denied, got %v", got)
	}
	if got := result["_pa_status_label"]; got != "Prior Authorization Denied" {
		t.Errorf("expected _pa_status_label='Prior Authorization Denied', got %v", got)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-PAS-REL-004: deliver_decision resolves the parsed decision, not the
// raw inbound envelope (Bug 6 — contentField pointed at a nonexistent alias
// "parse_response" instead of "parse_claim_response", so it silently fell
// back to sending the whole original message).
// ─────────────────────────────────────────────────────────────────────────────
func TestPASReliability_DeliverDecision_SendsParsedDecisionNotRawEnvelope(t *testing.T) {
	exec := transform.NewOutboundConnectorExecutor()
	step := &models.TransformationStep{
		StepName:  "Deliver PA Decision",
		StepAlias: strPtr("deliver_decision"),
		StepType:  "connector.outbound",
		Enabled:   true,
		Config: map[string]interface{}{
			"connectorType": "http_outbound",
			"config": map[string]interface{}{
				"endpoint":             "https://example.invalid/callback",
				"method":               "POST",
				"authentication_type":  "none",
			},
			"contentType":  "application/fhir+json",
			"contentField": "steps.parse_claim_response.step_output", // V211-fixed alias
		},
	}

	// A distinguishable marker in the raw inbound envelope — if the executor
	// falls back to sending the whole message (the bug), this string will
	// show up in the resolved payload; if it correctly resolves contentField,
	// it will not.
	input := map[string]interface{}{
		"_pas_envelope": map[string]interface{}{
			"patient": map[string]interface{}{"firstName": "RAW-ENVELOPE-MARKER-SHOULD-NOT-BE-SENT"},
		},
		"steps": map[string]interface{}{
			"parse_claim_response": map[string]interface{}{
				"step_output": map[string]interface{}{
					"decision":      "AA",
					"review_action": "A1",
				},
			},
		},
	}

	ctx := models.WithTestMode(context.Background()) // dry-run: resolves + serializes, doesn't send over the network
	result, err := exec.Execute(ctx, step, input)
	if err != nil {
		t.Fatalf("deliver_decision returned an error: %v", err)
	}

	out := stepOutput(t, result, "deliver_decision")
	payload, _ := out["payload_full"].(string)
	if payload == "" {
		t.Fatalf("expected a non-empty resolved payload, got empty. Full step output: %+v", out)
	}

	if strings.Contains(payload, "RAW-ENVELOPE-MARKER-SHOULD-NOT-BE-SENT") {
		t.Fatalf("deliver_decision sent the raw inbound envelope instead of the parsed decision — contentField alias is still wrong. payload: %s", payload)
	}
	if !strings.Contains(payload, "\"decision\":\"AA\"") && !strings.Contains(payload, "\"decision\": \"AA\"") {
		t.Errorf("expected the resolved payload to contain the parsed decision (decision=AA), got: %s", payload)
	}

	// Sanity-check the payload really is the parse_claim_response step_output
	// object, not some other coincidental match.
	var parsed map[string]interface{}
	if jsonErr := json.Unmarshal([]byte(payload), &parsed); jsonErr == nil {
		if parsed["review_action"] != "A1" {
			t.Errorf("expected review_action=A1 in resolved payload, got %v (payload: %s)", parsed["review_action"], payload)
		}
	}
}
