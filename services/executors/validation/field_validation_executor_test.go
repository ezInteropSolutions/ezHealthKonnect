package validation

import (
	"context"
	"ezhealthkonnect/models"
	"testing"
)

// TestFieldValidationExecutor_GetStepType_MatchesRealUsage guards against the exact bug found
// 2026-09-06: a stale GetStepType() override on FieldValidationExecutor returned "pre.validation"
// while its own embedded BaseExecutor was constructed with "field_validation" — the name every
// real caller actually uses (ToolboxManager.js, PropertiesPanel.js, PipelineModels.js, and 4 real
// saved steps in production at the time this was found). The override shadowed the correct
// embedded value via Go's own-method-wins-over-embedded-method rule, so services/executor_registry.go's
// autoRegisterExecutors() never registered a "field_validation" key at all — every field_validation
// step in the system silently failed with "no executor available for step type: field_validation"
// (or, worse, the executor_registry.go's own backward-compat alias lines for "pre.validation"/
// "core.validation" read that same never-registered key and overwrote the correctly self-registered
// "pre.validation" entry with nil too, breaking that name as well). Fixed by deleting the override
// and letting the embedded BaseExecutor's own GetStepType() (matching its constructor call) take over.
func TestFieldValidationExecutor_GetStepType_MatchesRealUsage(t *testing.T) {
	executor := NewFieldValidationExecutor()
	if got := executor.GetStepType(); got != "field_validation" {
		t.Fatalf("GetStepType() = %q, want %q (the name every real caller uses)", got, "field_validation")
	}
}

// TestFieldValidationExecutor_Execute_RequiredField_PassAndFail is a basic Execute()-level
// smoke test (this package had zero prior test coverage) proving the required-validator itself
// correctly distinguishes a present vs. absent field once the step actually runs.
func TestFieldValidationExecutor_Execute_RequiredField_PassAndFail(t *testing.T) {
	executor := NewFieldValidationExecutor()

	step := &models.TransformationStep{
		StepType: "field_validation",
		Enabled:  true,
		Config: map[string]interface{}{
			"rules": []interface{}{
				map[string]interface{}{
					"field":        "message.trn",
					"type":         "required",
					"errorMessage": "trn is required",
				},
			},
		},
	}

	t.Run("field present", func(t *testing.T) {
		input := map[string]interface{}{
			"message": map[string]interface{}{"trn": "12345"},
		}
		output, err := executor.Execute(context.Background(), step, input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		stepOutput, _ := output["_stepOutput"].(map[string]interface{})
		if valid, _ := stepOutput["valid"].(bool); !valid {
			t.Fatalf("expected valid=true when the required field is present, got step_output=%+v", stepOutput)
		}
	})

	t.Run("field absent, non-strict step continues with a warning", func(t *testing.T) {
		input := map[string]interface{}{
			"message": map[string]interface{}{},
		}
		output, err := executor.Execute(context.Background(), step, input)
		if err != nil {
			t.Fatalf("unexpected error (step.Required is false, should warn-and-continue): %v", err)
		}
		stepOutput, _ := output["_stepOutput"].(map[string]interface{})
		if valid, _ := stepOutput["valid"].(bool); valid {
			t.Fatalf("expected valid=false when the required field is absent, got step_output=%+v", stepOutput)
		}
	})

	t.Run("field absent, step.Required=true rejects the pipeline", func(t *testing.T) {
		strictStep := &models.TransformationStep{
			StepType: "field_validation",
			Enabled:  true,
			Required: true,
			Config:   step.Config,
		}
		input := map[string]interface{}{
			"message": map[string]interface{}{},
		}
		_, err := executor.Execute(context.Background(), strictStep, input)
		if err == nil {
			t.Fatal("expected an error when a required rule fails on a step.Required=true step")
		}
	})
}
