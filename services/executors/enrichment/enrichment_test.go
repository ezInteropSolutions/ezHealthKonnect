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
		StepType: "pre.enrichment.script",
		Enabled:  true,
		Config: map[string]interface{}{
			"script":     `({ result: 42, status: "success" });`,
			"targetPath": "enriched.test",
		},
	}

	inputData := map[string]interface{}{"message_id": "test-123"}
	ctx := context.Background()

	result, err := executor.Execute(ctx, step, inputData)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	enriched, ok := result["enriched"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected enriched map")
	}

	testData, ok := enriched["test"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected test data")
	}

	if result, ok := testData["result"].(int64); !ok || result != 42 {
		t.Errorf("Expected result=42, got: %v", testData["result"])
	}
}

func TestScriptEnrichment_CalculateAge(t *testing.T) {
	executor := NewScriptEnrichmentExecutor()

	step := &models.TransformationStep{
		StepName: "Age Calculation",
		StepType: "pre.enrichment.script",
		Enabled:  true,
		Config: map[string]interface{}{
			"script": `
var dob = "19900115";
var age = calculateAge(dob);
var ageGroup = age < 18 ? "pediatric" : (age < 65 ? "adult" : "geriatric");

({ age: age, ageGroup: ageGroup });
			`,
			"targetPath": "enriched.patient",
		},
	}

	ctx := context.Background()
	result, err := executor.Execute(ctx, step, map[string]interface{}{})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	enriched := result["enriched"].(map[string]interface{})
	patient := enriched["patient"].(map[string]interface{})

	age, ok := patient["age"].(int64)
	if !ok || age < 30 || age > 40 {
		t.Errorf("Expected age ~34-35, got: %v", age)
	}

	if ageGroup := patient["ageGroup"].(string); ageGroup != "adult" {
		t.Errorf("Expected ageGroup=adult, got: %v", ageGroup)
	}
}

func TestScriptEnrichment_GetNestedValue(t *testing.T) {
	executor := NewScriptEnrichmentExecutor()

	step := &models.TransformationStep{
		StepName: "Extract Values",
		StepType: "pre.enrichment.script",
		Enabled:  true,
		Config: map[string]interface{}{
			"script": `
var patientId = getNestedValue(input, "patient.id");
var msgType = getNestedValue(input, "messageType");

({ patientId: patientId, messageType: msgType });
			`,
			"targetPath": "enriched.extracted",
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

	enriched := result["enriched"].(map[string]interface{})
	extracted := enriched["extracted"].(map[string]interface{})

	if patientId := extracted["patientId"].(string); patientId != "P123456" {
		t.Errorf("Expected patientId=P123456, got: %v", patientId)
	}

	if msgType := extracted["messageType"].(string); msgType != "ADT^A01" {
		t.Errorf("Expected messageType=ADT^A01, got: %v", msgType)
	}
}

func TestScriptEnrichment_ContextVariables(t *testing.T) {
	executor := NewScriptEnrichmentExecutor()

	step := &models.TransformationStep{
		StepName: "Context Test",
		StepType: "pre.enrichment.script",
		Enabled:  true,
		Config: map[string]interface{}{
			"script": `({ hospitalId: hospitalId, env: environment, combined: hospitalId + "-" + environment });`,
			"context": map[string]interface{}{
				"hospitalId":  "HOSP001",
				"environment": "prod",
			},
			"targetPath": "enriched.context",
		},
	}

	ctx := context.Background()
	result, err := executor.Execute(ctx, step, map[string]interface{}{})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	enriched := result["enriched"].(map[string]interface{})
	contextData := enriched["context"].(map[string]interface{})

	if hospitalId := contextData["hospitalId"].(string); hospitalId != "HOSP001" {
		t.Errorf("Expected HOSP001, got: %v", hospitalId)
	}

	if combined := contextData["combined"].(string); combined != "HOSP001-prod" {
		t.Errorf("Expected HOSP001-prod, got: %v", combined)
	}
}

func TestScriptEnrichment_ErrorHandling(t *testing.T) {
	executor := NewScriptEnrichmentExecutor()

	// Test with failOnError=false
	step := &models.TransformationStep{
		StepName: "Error Test",
		StepType: "pre.enrichment.script",
		Enabled:  true,
		Config: map[string]interface{}{
			"script":      `throw new Error("Test error");`,
			"targetPath":  "enriched.error",
			"failOnError": false,
		},
	}

	ctx := context.Background()
	result, err := executor.Execute(ctx, step, map[string]interface{}{"original": "data"})

	// Should NOT return error
	if err != nil {
		t.Errorf("Expected no error with failOnError=false, got: %v", err)
	}

	// Original data should be preserved
	if original := result["original"].(string); original != "data" {
		t.Error("Original data should be preserved")
	}

	// Test with failOnError=true
	step.Config["failOnError"] = true
	_, err = executor.Execute(ctx, step, map[string]interface{}{})

	// Should return error
	if err == nil {
		t.Error("Expected error with failOnError=true")
	}
}

func TestScriptEnrichment_ParseHL7Date(t *testing.T) {
	executor := NewScriptEnrichmentExecutor()

	step := &models.TransformationStep{
		StepName: "Date Parsing",
		StepType: "pre.enrichment.script",
		Enabled:  true,
		Config: map[string]interface{}{
			"script": `
var date1 = parseHL7Date("20231215");
var date2 = parseHL7Date("20231215143045");

({ date1: date1, date2: date2 });
			`,
			"targetPath": "enriched.dates",
		},
	}

	ctx := context.Background()
	result, err := executor.Execute(ctx, step, map[string]interface{}{})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	enriched := result["enriched"].(map[string]interface{})
	dates := enriched["dates"].(map[string]interface{})

	if date1 := dates["date1"].(string); date1 != "2023-12-15" {
		t.Errorf("Expected 2023-12-15, got: %v", date1)
	}

	if date2 := dates["date2"].(string); date2 != "2023-12-15T14:30:45" {
		t.Errorf("Expected 2023-12-15T14:30:45, got: %v", date2)
	}
}

// ===============================================================
// METADATA ENRICHMENT EXECUTOR TESTS
// ===============================================================

func TestMetadataEnrichment_Timestamps(t *testing.T) {
	executor := NewMetadataEnrichmentExecutor()

	step := &models.TransformationStep{
		StepName: "Add Timestamps",
		StepType: "pre.enrichment.metadata",
		Enabled:  true,
		Config: map[string]interface{}{
			"addTimestamp":     true,
			"addCorrelationId": false,
		},
	}

	ctx := context.Background()
	result, err := executor.Execute(ctx, step, map[string]interface{}{})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	metadata, ok := result["metadata"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected metadata map")
	}

	if _, ok := metadata["receivedAt"]; !ok {
		t.Error("Expected receivedAt timestamp")
	}
}

func TestMetadataEnrichment_CorrelationID(t *testing.T) {
	executor := NewMetadataEnrichmentExecutor()

	step := &models.TransformationStep{
		StepName: "Add Correlation ID",
		StepType: "pre.enrichment.metadata",
		Enabled:  true,
		Config: map[string]interface{}{
			"addCorrelationId": true,
			"addTimestamp":     false,
		},
	}

	ctx := context.Background()
	result, err := executor.Execute(ctx, step, map[string]interface{}{})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	metadata := result["metadata"].(map[string]interface{})

	correlationID, ok := metadata["correlationId"].(string)
	if !ok || correlationID == "" {
		t.Error("Expected non-empty correlationId")
	}

	// UUID should be 36 characters (with hyphens)
	if len(correlationID) != 36 {
		t.Errorf("Expected UUID length 36, got: %d", len(correlationID))
	}
}

func TestMetadataEnrichment_CustomMetadata(t *testing.T) {
	executor := NewMetadataEnrichmentExecutor()

	step := &models.TransformationStep{
		StepName: "Custom Metadata",
		StepType: "pre.enrichment.metadata",
		Enabled:  true,
		Config: map[string]interface{}{
			"customMetadata": map[string]interface{}{
				"environment": "test",
				"version":     "1.0",
			},
		},
	}

	ctx := context.Background()
	result, err := executor.Execute(ctx, step, map[string]interface{}{})

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	metadata := result["metadata"].(map[string]interface{})

	if env := metadata["environment"].(string); env != "test" {
		t.Errorf("Expected environment=test, got: %v", env)
	}

	if version := metadata["version"].(string); version != "1.0" {
		t.Errorf("Expected version=1.0, got: %v", version)
	}
}

// ===============================================================
// BENCHMARK TESTS
// ===============================================================

func BenchmarkScriptEnrichment_Simple(b *testing.B) {
	executor := NewScriptEnrichmentExecutor()

	step := &models.TransformationStep{
		StepName: "Benchmark",
		StepType: "pre.enrichment.script",
		Enabled:  true,
		Config: map[string]interface{}{
			"script":     `({ result: 42 });`,
			"targetPath": "enriched.bench",
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
		StepType: "pre.enrichment.script",
		Enabled:  true,
		Config: map[string]interface{}{
			"script": `
var dob = "19900115";
var age = calculateAge(dob);
var patientId = getNestedValue(input, "patient.id");

({ age: age, ageGroup: age < 18 ? "pediatric" : "adult", patientId: patientId });
			`,
			"targetPath": "enriched.bench",
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

func BenchmarkMetadataEnrichment(b *testing.B) {
	executor := NewMetadataEnrichmentExecutor()

	step := &models.TransformationStep{
		StepName: "Metadata Benchmark",
		StepType: "pre.enrichment.metadata",
		Enabled:  true,
		Config: map[string]interface{}{
			"addTimestamp":     true,
			"addCorrelationId": true,
		},
	}

	ctx := context.Background()
	inputData := map[string]interface{}{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = executor.Execute(ctx, step, inputData)
	}
}
