// services/executors/validation/fhir_validation_executor_test.go
package validation

import (
	"context"
	"encoding/json"
	"testing"

	"ezhealthkonnect/fhir/r4"
	"ezhealthkonnect/models"
)

// testFHIRSchemaDir mirrors services/executors/transform/fhir_build_executor_test.go's
// own constant, adjusted for this package's depth.
const testFHIRSchemaDir = "../../../schemas/fhir"

func initFHIRRegistry(t *testing.T) {
	t.Helper()
	if err := r4.ForceReinit(testFHIRSchemaDir); err != nil {
		t.Fatalf("r4.ForceReinit failed: %v", err)
	}
	if r4.GetRegistry() == nil {
		t.Fatal("expected non-nil registry after ForceReinit")
	}
}

// TestFHIRValidation_SourceField_AcceptsJSONStringBundle pins the fix this
// round added: payload.builder's fhir_bundle mode outputs its assembled
// Bundle as an already-marshaled JSON STRING (its "payload" field) — with no
// separate map-shaped output anywhere in that executor — so source_field
// pointing directly at a payload.builder step's own output previously never
// resolved, forcing an enrichment.script bridge (the exact workaround V212's
// PAS template needed and this project's "no script" platform can't use).
func TestFHIRValidation_SourceField_AcceptsJSONStringBundle(t *testing.T) {
	initFHIRRegistry(t)
	bundle := map[string]interface{}{
		"resourceType": "Bundle",
		"type":         "collection",
		"entry": []interface{}{
			map[string]interface{}{
				"fullUrl":  "urn:uuid:1",
				"resource": map[string]interface{}{"resourceType": "Patient", "id": "12345"},
			},
		},
	}
	raw, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("failed to marshal fixture bundle: %v", err)
	}

	executor := NewFHIRValidationExecutor()
	step := &models.TransformationStep{
		StepName: "Test Validate", StepType: "fhir_validation", Enabled: true,
		Config: map[string]interface{}{"validation_level": "basic", "source_field": "payload"},
	}
	output, err := executor.Execute(context.Background(), step, map[string]interface{}{"payload": string(raw)})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	stepOutput, ok := output["_stepOutput"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected _stepOutput to be a map, got %T", output["_stepOutput"])
	}
	if mode, _ := stepOutput["validation_mode"].(string); mode != "bundle" {
		t.Errorf("validation_mode = %v, want \"bundle\" — the JSON string source_field must resolve as the Bundle it contains", stepOutput["validation_mode"])
	}
	if count, _ := stepOutput["resource_count"].(int); count != 1 {
		t.Errorf("resource_count = %v, want 1", stepOutput["resource_count"])
	}
}

// TestFHIRValidation_SourceField_MalformedJSONString_FallsBackNotPanic
// verifies a source_field resolving to a non-JSON string degrades to the
// default fixed-key search (which then reports "no FHIR data found") rather
// than panicking or silently validating nothing.
func TestFHIRValidation_SourceField_MalformedJSONString_FallsBackNotPanic(t *testing.T) {
	initFHIRRegistry(t)
	executor := NewFHIRValidationExecutor()
	step := &models.TransformationStep{
		StepName: "Test Validate", StepType: "fhir_validation", Enabled: true,
		Config: map[string]interface{}{"validation_level": "basic", "source_field": "payload"},
	}
	output, err := executor.Execute(context.Background(), step, map[string]interface{}{"payload": "not valid json"})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	stepOutput, ok := output["_stepOutput"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected _stepOutput to be a map, got %T", output["_stepOutput"])
	}
	valid, _ := stepOutput["valid"].(bool)
	if valid {
		t.Errorf("expected valid=false when no FHIR data can be found, got true")
	}
}

// TestFHIRValidation_SourceField_StillAcceptsPlainMap is a regression guard:
// the pre-existing map-shaped source_field path (e.g. a fhir.build step's
// own output field) must keep working exactly as before this round's string
// handling was added.
func TestFHIRValidation_SourceField_StillAcceptsPlainMap(t *testing.T) {
	initFHIRRegistry(t)
	executor := NewFHIRValidationExecutor()
	step := &models.TransformationStep{
		StepName: "Test Validate", StepType: "fhir_validation", Enabled: true,
		Config: map[string]interface{}{"validation_level": "basic", "source_field": "message.paymentReconciliation"},
	}
	inputData := map[string]interface{}{
		"message": map[string]interface{}{
			"paymentReconciliation": map[string]interface{}{"resourceType": "PaymentReconciliation", "id": "pr-1"},
		},
	}
	output, err := executor.Execute(context.Background(), step, inputData)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	stepOutput, ok := output["_stepOutput"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected _stepOutput to be a map, got %T", output["_stepOutput"])
	}
	if mode, _ := stepOutput["validation_mode"].(string); mode != "resource" {
		t.Errorf("validation_mode = %v, want \"resource\"", stepOutput["validation_mode"])
	}
}
