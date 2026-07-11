// services/ai/mapping_suggester_service_test.go
package ai

import (
	"context"
	"testing"
)

func TestMappingSuggesterService_SuggestFieldMappings_ConstrainedToCatalog(t *testing.T) {
	// The mock LLM proposes a mapping to a target field NOT in the catalog
	// ("Patient.notARealField") alongside a valid one ("birthDate") — only
	// the valid one must survive constrainToCatalog's enforcement.
	llm := NewMockProvider(map[string]string{
		"": `[{"source_field":"dob","target_field":"birthDate","confidence":0.9,"reasoning":"date of birth"},
		     {"source_field":"foo","target_field":"Patient.notARealField","confidence":0.5,"reasoning":"guess"}]`,
	})

	svc := newMappingSuggesterService()
	input := FieldMappingSuggestInput{
		StepType:     "fhir.build",
		SampleRows:   []map[string]interface{}{{"dob": "1980-05-20", "foo": "bar"}},
		TargetFields: []TargetFieldInfo{{Key: "birthDate", Label: "Birth Date", DataType: "date"}},
	}

	suggestions, err := svc.SuggestFieldMappings(context.Background(), llm, input)
	if err != nil {
		t.Fatalf("SuggestFieldMappings failed: %v", err)
	}
	if len(suggestions) != 1 {
		t.Fatalf("expected 1 suggestion after catalog constraint, got %d: %+v", len(suggestions), suggestions)
	}
	if suggestions[0].TargetField != "birthDate" {
		t.Errorf("TargetField = %q, want birthDate", suggestions[0].TargetField)
	}
	if suggestions[0].SourceField != "dob" {
		t.Errorf("SourceField = %q, want dob", suggestions[0].SourceField)
	}
}

func TestMappingSuggesterService_SuggestFieldMappings_NoSampleRows_ReturnsError(t *testing.T) {
	svc := newMappingSuggesterService()
	llm := NewMockProvider(nil)
	_, err := svc.SuggestFieldMappings(context.Background(), llm, FieldMappingSuggestInput{
		StepType:     "fhir.build",
		TargetFields: []TargetFieldInfo{{Key: "birthDate"}},
	})
	if err == nil {
		t.Fatal("expected an error when no sample rows are provided")
	}
}

func TestMappingSuggesterService_SuggestFieldMappings_NoTargetFields_ReturnsError(t *testing.T) {
	svc := newMappingSuggesterService()
	llm := NewMockProvider(nil)
	_, err := svc.SuggestFieldMappings(context.Background(), llm, FieldMappingSuggestInput{
		StepType:   "fhir.build",
		SampleRows: []map[string]interface{}{{"dob": "1980-05-20"}},
	})
	if err == nil {
		t.Fatal("expected an error when the target field catalog is empty")
	}
}

func TestMappingSuggesterService_SuggestFieldMappings_UnparsableLLMResponse_ReturnsError(t *testing.T) {
	llm := NewMockProvider(map[string]string{"": "not json at all, sorry"})
	svc := newMappingSuggesterService()
	_, err := svc.SuggestFieldMappings(context.Background(), llm, FieldMappingSuggestInput{
		StepType:     "fhir.build",
		SampleRows:   []map[string]interface{}{{"dob": "1980-05-20"}},
		TargetFields: []TargetFieldInfo{{Key: "birthDate"}},
	})
	if err == nil {
		t.Fatal("expected an error when the LLM response contains no extractable JSON")
	}
}

func TestConstrainToCatalog_DropsUnknownTargets(t *testing.T) {
	suggestions := []MappingSuggestion{
		{SourceField: "a", TargetField: "known"},
		{SourceField: "b", TargetField: "unknown"},
	}
	targetFields := []TargetFieldInfo{{Key: "known"}}

	out := constrainToCatalog(suggestions, targetFields)
	if len(out) != 1 || out[0].TargetField != "known" {
		t.Errorf("expected only the known-target suggestion to survive, got %+v", out)
	}
}
