package enrichment

import (
	"context"
	"ezhealthkonnect/models"
	"testing"
)

// ===============================================================
// SCRIPT ENRICHMENT EXECUTOR TESTS
// ===============================================================

func TestScriptEnrichment_BasicExecution(t *testing.T) {
	executor := NewScriptEnrichmentExecutor()

	step := &models.TransformationStep{
		StepName: "Basic Test",
		StepType: "enrichment.script",
		Enabled:  true,
		Config: map[string]interface{}{
			"script":     `({ result: 42, status: "success" });`,
		},
	}

	inputData := map[string]interface{}{"message_id": "test-123"}
	ctx := context.Background()

	result, err := executor.Execute(ctx, step, inputData)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// P7: data is now in _stepOutput (flat), not enriched.test
	stepOutput, ok := result["_stepOutput"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected _stepOutput map")
	}

	if v, ok := stepOutput["result"].(int64); !ok || v != 42 {
		t.Errorf("Expected result=42, got: %v", stepOutput["result"])
	}
}

func TestScriptEnrichment_CalculateAge(t *testing.T) {
	executor := NewScriptEnrichmentExecutor()

	step := &models.TransformationStep{
		StepName: "Age Calculation",
		StepType: "enrichment.script",
		Enabled:  true,
		Config: map[string]interface{}{
			"script": `
var dob = "19900115";
var age = calculateAge(dob);
var ageGroup = age < 18 ? "pediatric" : (age < 65 ? "adult" : "geriatric");

({ age: age, ageGroup: ageGroup });
			`,
		},
	}

	ctx := context.Background()
	result, err := executor.Execute(ctx, step, map[string]interface{}{})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// P7: data is now in _stepOutput (flat), not enriched.patient
	stepOutput := result["_stepOutput"].(map[string]interface{})

	age, ok := stepOutput["age"].(int64)
	if !ok || age < 30 || age > 40 {
		t.Errorf("Expected age ~34-35, got: %v", age)
	}

	if ageGroup := stepOutput["ageGroup"].(string); ageGroup != "adult" {
		t.Errorf("Expected ageGroup=adult, got: %v", ageGroup)
	}
}

func TestScriptEnrichment_GetNestedValue(t *testing.T) {
	executor := NewScriptEnrichmentExecutor()

	step := &models.TransformationStep{
		StepName: "Extract Values",
		StepType: "enrichment.script",
		Enabled:  true,
		Config: map[string]interface{}{
			"script": `
var patientId = getNestedValue(input, "patient.id");
var msgType = getNestedValue(input, "messageType");

({ patientId: patientId, messageType: msgType });
			`,
		},
	}

	inputData := map[string]interface{}{
		"messageType": "ADT^A01",
		"patient": map[string]interface{}{
			"id": "P123456",
		},
	}

	ctx := context.Background()
	result, err := executor.Execute(ctx, step, inputData)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// P7: data is now in _stepOutput (flat), not enriched.extracted
	stepOutput := result["_stepOutput"].(map[string]interface{})

	if patientId := stepOutput["patientId"].(string); patientId != "P123456" {
		t.Errorf("Expected patientId=P123456, got: %v", patientId)
	}

	if msgType := stepOutput["messageType"].(string); msgType != "ADT^A01" {
		t.Errorf("Expected messageType=ADT^A01, got: %v", msgType)
	}
}

func TestScriptEnrichment_ContextVariables(t *testing.T) {
	executor := NewScriptEnrichmentExecutor()

	step := &models.TransformationStep{
		StepName: "Context Test",
		StepType: "enrichment.script",
		Enabled:  true,
		Config: map[string]interface{}{
			"script": `({ hospitalId: hospitalId, env: environment, combined: hospitalId + "-" + environment });`,
			"context": map[string]interface{}{
				"hospitalId":  "HOSP001",
				"environment": "prod",
			},
		},
	}

	ctx := context.Background()
	result, err := executor.Execute(ctx, step, map[string]interface{}{})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// P7: data is now in _stepOutput (flat), not enriched.context
	stepOutput := result["_stepOutput"].(map[string]interface{})

	if hospitalId := stepOutput["hospitalId"].(string); hospitalId != "HOSP001" {
		t.Errorf("Expected HOSP001, got: %v", hospitalId)
	}

	if combined := stepOutput["combined"].(string); combined != "HOSP001-prod" {
		t.Errorf("Expected HOSP001-prod, got: %v", combined)
	}
}

func TestScriptEnrichment_ErrorHandling(t *testing.T) {
	executor := NewScriptEnrichmentExecutor()

	// ScriptEnrichmentConfig has no failOnError field — a thrown script error
	// is always surfaced (see Execute()'s own comment: "Always return error —
	// pipeline service decides retry/catch behavior" via step.config.errorHandling).
	step := &models.TransformationStep{
		StepName: "Error Test",
		StepType: "enrichment.script",
		Enabled:  true,
		Config: map[string]interface{}{
			"script": `throw new Error("Test error");`,
		},
	}

	ctx := context.Background()
	result, err := executor.Execute(ctx, step, map[string]interface{}{"original": "data"})

	if err == nil {
		t.Fatal("Expected a thrown script error to be returned")
	}

	// Original data should be preserved alongside the error
	if original := result["original"].(string); original != "data" {
		t.Error("Original data should be preserved")
	}
	if _, ok := result["_script_error"]; !ok {
		t.Error("Expected _script_error to be set in the output for debugging")
	}
}

func TestScriptEnrichment_ParseHL7Date(t *testing.T) {
	executor := NewScriptEnrichmentExecutor()

	step := &models.TransformationStep{
		StepName: "Date Parsing",
		StepType: "enrichment.script",
		Enabled:  true,
		Config: map[string]interface{}{
			"script": `
var date1 = parseHL7Date("20231215");
var date2 = parseHL7Date("20231215143045");

({ date1: date1, date2: date2 });
			`,
		},
	}

	ctx := context.Background()
	result, err := executor.Execute(ctx, step, map[string]interface{}{})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// P7: data is now in _stepOutput (flat), not enriched.dates
	stepOutput := result["_stepOutput"].(map[string]interface{})

	if date1 := stepOutput["date1"].(string); date1 != "2023-12-15" {
		t.Errorf("Expected 2023-12-15, got: %v", date1)
	}

	if date2 := stepOutput["date2"].(string); date2 != "2023-12-15T14:30:45" {
		t.Errorf("Expected 2023-12-15T14:30:45, got: %v", date2)
	}
}

// ===============================================================
// METADATA ENRICHMENT EXECUTOR TESTS
// ===============================================================
// REMOVED: MetadataEnrichmentExecutor was consolidated into FieldMappingExecutor.
// These tests referenced NewMetadataEnrichmentExecutor() which no longer exists.
// Metadata functionality is now tested via FieldMapping with metadata config.

// ===============================================================
// OUTPUT VARIABLE MUTATION TESTS
// ===============================================================
// The executor pre-injects an empty `output` object into the VM.
// Scripts can write to it (e.g. `output.patientId = "123"`) and omit
// an explicit return — the VM reads back the mutated `output` value.

func TestScriptEnrichment_OutputMutation_NoReturn(t *testing.T) {
	executor := NewScriptEnrichmentExecutor()

	step := &models.TransformationStep{
		StepName: "Output Mutation",
		StepType: "enrichment.script",
		Enabled:  true,
		Config: map[string]interface{}{
			// Script writes to `output` but does NOT return a value
			"script": `
output.patientId = "P12345";
output.status = "active";
`,
		},
	}

	result, err := executor.Execute(context.Background(), step, map[string]interface{}{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	stepOutput, ok := result["_stepOutput"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected _stepOutput map")
	}
	if stepOutput["patientId"] != "P12345" {
		t.Errorf("Expected patientId=P12345, got: %v", stepOutput["patientId"])
	}
	if stepOutput["status"] != "active" {
		t.Errorf("Expected status=active, got: %v", stepOutput["status"])
	}
}

func TestScriptEnrichment_ExplicitReturnTakesPrecedence(t *testing.T) {
	executor := NewScriptEnrichmentExecutor()

	step := &models.TransformationStep{
		StepName: "Return Precedence",
		StepType: "enrichment.script",
		Enabled:  true,
		Config: map[string]interface{}{
			// Script mutates output AND returns a value — return wins
			"script": `
output.shouldBeIgnored = "yes";
({ returnedValue: 99 });
`,
		},
	}

	result, err := executor.Execute(context.Background(), step, map[string]interface{}{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	stepOutput, ok := result["_stepOutput"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected _stepOutput map")
	}
	// The explicit return value should be used
	if v, _ := stepOutput["returnedValue"].(int64); v != 99 {
		t.Errorf("Expected returnedValue=99, got: %v", stepOutput["returnedValue"])
	}
	// The output mutation should NOT appear (return value takes precedence)
	if _, found := stepOutput["shouldBeIgnored"]; found {
		t.Error("Expected shouldBeIgnored to be absent when explicit return is present")
	}
}

func TestScriptEnrichment_OutputMutation_ComputedFields(t *testing.T) {
	executor := NewScriptEnrichmentExecutor()

	step := &models.TransformationStep{
		StepName: "Computed via output",
		StepType: "enrichment.script",
		Enabled:  true,
		Config: map[string]interface{}{
			"script": `
var dob = "19900115";
output.age = calculateAge(dob);
output.ageGroup = output.age < 18 ? "pediatric" : "adult";
`,
		},
	}

	result, err := executor.Execute(context.Background(), step, map[string]interface{}{})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	stepOutput := result["_stepOutput"].(map[string]interface{})
	age, ok := stepOutput["age"].(int64)
	if !ok || age < 30 || age > 40 {
		t.Errorf("Expected age ~34-35 via output mutation, got: %v", stepOutput["age"])
	}
	if stepOutput["ageGroup"] != "adult" {
		t.Errorf("Expected ageGroup=adult, got: %v", stepOutput["ageGroup"])
	}
}

func TestScriptEnrichment_TopLevelReturnStatement(t *testing.T) {
	executor := NewScriptEnrichmentExecutor()

	step := &models.TransformationStep{
		StepName: "Explicit Return Statement",
		StepType: "enrichment.script",
		Enabled:  true,
		Config: map[string]interface{}{
			// A bare top-level `return` is a SyntaxError outside a function —
			// this can only run via the IIFE-wrapped fallback path.
			"script": `
if (input.skip) {
	return { skipped: true };
}
return { value: 7 };
`,
		},
	}

	result, err := executor.Execute(context.Background(), step, map[string]interface{}{"skip": false})
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	stepOutput, ok := result["_stepOutput"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected _stepOutput map")
	}
	if v, _ := stepOutput["value"].(int64); v != 7 {
		t.Errorf("Expected value=7 from explicit return, got: %v", stepOutput["value"])
	}
}

// ===============================================================
// BENCHMARK TESTS
// ===============================================================

func BenchmarkScriptEnrichment_Simple(b *testing.B) {
	executor := NewScriptEnrichmentExecutor()

	step := &models.TransformationStep{
		StepName: "Benchmark",
		StepType: "enrichment.script",
		Enabled:  true,
		Config: map[string]interface{}{
			"script": `({ result: 42 });`,
		},
	}

	ctx := context.Background()
	inputData := map[string]interface{}{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = executor.Execute(ctx, step, inputData)
	}
}

func BenchmarkScriptEnrichment_Complex(b *testing.B) {
	executor := NewScriptEnrichmentExecutor()

	step := &models.TransformationStep{
		StepName: "Complex Benchmark",
		StepType: "enrichment.script",
		Enabled:  true,
		Config: map[string]interface{}{
			"script": `
var dob = "19900115";
var age = calculateAge(dob);
var patientId = getNestedValue(input, "patient.id");

({ age: age, ageGroup: age < 18 ? "pediatric" : "adult", patientId: patientId });
			`,
		},
	}

	ctx := context.Background()
	inputData := map[string]interface{}{
		"patient": map[string]interface{}{"id": "P123456"},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = executor.Execute(ctx, step, inputData)
	}
}

// BenchmarkMetadataEnrichment removed — MetadataEnrichmentExecutor consolidated into FieldMapping
