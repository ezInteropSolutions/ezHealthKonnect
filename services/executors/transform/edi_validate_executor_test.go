// services/executors/transform/edi_validate_executor_test.go
package transform

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ezhealthkonnect/edi"
	"ezhealthkonnect/edi/validator"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
)

// newTestEDIValidateExecutor mirrors newTestEDIParseExecutor's own fix for
// the schema-directory-relative-to-CWD problem (see that file's comment).
func newTestEDIValidateExecutor(t *testing.T) *EDIValidateExecutor {
	t.Helper()
	loader, err := edi.NewX12SchemaLoader("../../../edi/schemas/x12_005010")
	if err != nil {
		t.Fatalf("failed to load EDI schema: %v", err)
	}
	return &EDIValidateExecutor{
		BaseExecutor: executors.NewBaseExecutor("edi.validate", models.ExecutorMetadata{
			Name: "EDI X12 Validator", Category: "EDI Transform",
		}),
		loader: loader,
	}
}

func readValidateSample(t *testing.T, filename string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("../../../edi/testdata/real_samples", filename))
	if err != nil {
		t.Fatalf("failed to read real sample %s: %v", filename, err)
	}
	return string(content)
}

// TestEDIValidateExecutor_RealSamples_NoValueErrors verifies both real,
// unedited 835 samples parse with zero ERROR-severity issues (they're real
// production-shaped data, so any error here would point at a genuine schema
// bug, not a data quality issue).
func TestEDIValidateExecutor_RealSamples_NoValueErrors(t *testing.T) {
	for _, filename := range []string{"blue_cross_nc_sample.txt", "emedny_sample.txt", "united_healthcare_legacy_sample.txt"} {
		t.Run(filename, func(t *testing.T) {
			exec := newTestEDIValidateExecutor(t)
			raw := readValidateSample(t, filename)

			step := &models.TransformationStep{
				StepName: "Test Validate EDI", StepType: "edi.validate", Enabled: true,
				Config: map[string]interface{}{"sourceField": "raw"},
			}
			output, err := exec.Execute(context.Background(), step, map[string]interface{}{"raw": raw})
			if err != nil {
				t.Fatalf("Execute failed: %v", err)
			}

			result, ok := output["ediValidation"].(*validator.Result)
			if !ok {
				t.Fatalf("expected ediValidation to be a *validator.Result, got %T", output["ediValidation"])
			}
			if !result.Valid {
				t.Errorf("expected Valid=true (no ERROR-severity issues) for real sample %s, got issues: %+v", filename, result.Issues)
			}
			for _, issue := range result.Issues {
				if issue.Severity == "error" {
					t.Errorf("unexpected ERROR-severity issue for real sample %s: %+v", filename, issue)
				}
			}
		})
	}
}

// TestEDIValidateExecutor_MalformedValue_IsErrorSeverityAndBlocksValid
// corrupts BPR02 (totalActualProviderPaymentAmount, dataType "R") from a
// real numeric value to a non-numeric string, proving the error path (a
// malformed VALUE) both surfaces as an "error" Issue and flips Valid=false —
// distinct from a SyntaxRule violation, which never does either (see the
// warning-severity test below).
func TestEDIValidateExecutor_MalformedValue_IsErrorSeverityAndBlocksValid(t *testing.T) {
	exec := newTestEDIValidateExecutor(t)
	raw := readValidateSample(t, "blue_cross_nc_sample.txt")
	corrupted := strings.Replace(raw, "BPR*I*1922.86*C*CHK", "BPR*I*NOT_A_NUMBER*C*CHK", 1)
	if corrupted == raw {
		t.Fatal("test setup error: expected BPR substring not found in the sample")
	}

	step := &models.TransformationStep{
		StepName: "Test Validate EDI", StepType: "edi.validate", Enabled: true,
		Config: map[string]interface{}{"sourceField": "raw"},
	}
	output, err := exec.Execute(context.Background(), step, map[string]interface{}{"raw": corrupted})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	result := output["ediValidation"].(*validator.Result)
	if result.Valid {
		t.Fatal("expected Valid=false after corrupting BPR02 to a non-numeric value")
	}
	found := false
	for _, issue := range result.Issues {
		if issue.Severity == "error" && strings.Contains(issue.Path, "BPR") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an ERROR-severity issue on a BPR path, got: %+v", result.Issues)
	}
}

// TestEDIValidateExecutor_SyntaxRuleViolation_IsWarningNeverBlocking proves
// the product-decided severity split at the executor level (not just inside
// edi/validator's own unit tests): CAS's syntaxRules only cover its 2nd-6th
// reason/amount/quantity trios (CAS05-19; trio 1, CAS02-04, has none — see
// CAS.json's own sourceRefs), so this populates trio 2's amount (CAS06)
// without its own anchor reason code (CAS05), violating the "L050607" rule
// (positions 05,06,07). Must surface as a warning and Valid must stay
// true — this is the concrete case the whole SyntaxRule mechanism was built
// for (see edi.SyntaxRule's doc comment and this project's "flexible, not
// rigid" product decision).
func TestEDIValidateExecutor_SyntaxRuleViolation_IsWarningNeverBlocking(t *testing.T) {
	exec := newTestEDIValidateExecutor(t)
	raw := readValidateSample(t, "blue_cross_nc_sample.txt")
	// "CAS*CO*42*34.6~" -> "CAS*CO*42*34.6***50~" adds an amount at CAS06
	// (2nd trio) while leaving CAS05 (2nd trio's own reason code) empty.
	corrupted := strings.Replace(raw, "CAS*CO*42*34.6~", "CAS*CO*42*34.6***50~", 1)
	if corrupted == raw {
		t.Fatal("test setup error: expected CAS substring not found in the sample")
	}

	step := &models.TransformationStep{
		StepName: "Test Validate EDI", StepType: "edi.validate", Enabled: true,
		Config: map[string]interface{}{"sourceField": "raw"},
	}
	output, err := exec.Execute(context.Background(), step, map[string]interface{}{"raw": corrupted})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	result := output["ediValidation"].(*validator.Result)
	if !result.Valid {
		t.Fatalf("expected Valid=true — a SyntaxRule violation must never block, got issues: %+v", result.Issues)
	}
	foundWarning := false
	for _, issue := range result.Issues {
		if issue.Severity == "warning" && issue.Path == "CAS" {
			foundWarning = true
		}
	}
	if !foundWarning {
		t.Errorf("expected a warning-severity CAS SyntaxRule issue, got: %+v", result.Issues)
	}
}

// TestEDIValidateExecutor_EmptySourceField_ReturnsError verifies a genuine
// execution failure (no content anywhere) surfaces as a Go error.
func TestEDIValidateExecutor_EmptySourceField_ReturnsError(t *testing.T) {
	exec := newTestEDIValidateExecutor(t)
	step := &models.TransformationStep{
		StepName: "Test Validate EDI", StepType: "edi.validate", Enabled: true,
		Config: map[string]interface{}{"sourceField": "raw"},
	}
	_, err := exec.Execute(context.Background(), step, map[string]interface{}{})
	if err == nil {
		t.Fatal("expected an error when the source field has no content, got nil")
	}
}

// TestEDIValidateExecutor_CustomRule_PairedViolation_IsWarningWithCustomSource
// adds a user-defined Paired rule on N3 (address line 1 / line 2 -- a
// segment with NO OOB syntaxRules of its own, see N3.json) and validates it
// against the real, unmodified blueCrossNC sample, whose N3 line
// ("N3*P O BOX 2291~") only ever populates address line 1. Proves: (1) a
// step-config-supplied rule actually gets checked, layered onto the OOB
// rules without needing to touch edi/validator.go itself; (2) it's tagged
// Source="custom" so the UI can distinguish it from a spec rule; (3) it
// stays warning-severity and never blocks Valid, same as every OOB rule.
func TestEDIValidateExecutor_CustomRule_PairedViolation_IsWarningWithCustomSource(t *testing.T) {
	exec := newTestEDIValidateExecutor(t)
	raw := readValidateSample(t, "blue_cross_nc_sample.txt")

	step := &models.TransformationStep{
		StepName: "Test Validate EDI", StepType: "edi.validate", Enabled: true,
		Config: map[string]interface{}{
			"sourceField": "raw",
			"customRules": []interface{}{
				map[string]interface{}{
					"segmentId": "N3", "type": "P",
					"positions": []interface{}{"addressInformation1", "addressInformation2"},
				},
			},
		},
	}
	output, err := exec.Execute(context.Background(), step, map[string]interface{}{"raw": raw})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	result := output["ediValidation"].(*validator.Result)
	if !result.Valid {
		t.Fatalf("expected Valid=true — a custom rule violation must never block, got issues: %+v", result.Issues)
	}
	found := false
	for _, issue := range result.Issues {
		if issue.Path == "N3" && issue.Source == "custom" {
			found = true
			if issue.Severity != "warning" {
				t.Errorf("expected custom rule issue to be warning-severity, got %q", issue.Severity)
			}
		}
	}
	if !found {
		t.Errorf("expected a custom-source N3 issue (address line 2 missing while line 1 present), got: %+v", result.Issues)
	}
}

// TestEDIValidateExecutor_CustomRule_UnknownSegment_SkippedGracefully
// verifies a custom rule referencing a segment ID that doesn't exist in the
// schema is silently skipped (logged, not a crash or an Execute() error) —
// config can drift (a rule authored against a schema that later changes),
// and this step degrades gracefully the same way applyCanonicalTransform's
// unknown-transform handling already does elsewhere in this codebase.
func TestEDIValidateExecutor_CustomRule_UnknownSegment_SkippedGracefully(t *testing.T) {
	exec := newTestEDIValidateExecutor(t)
	raw := readValidateSample(t, "blue_cross_nc_sample.txt")

	step := &models.TransformationStep{
		StepName: "Test Validate EDI", StepType: "edi.validate", Enabled: true,
		Config: map[string]interface{}{
			"sourceField": "raw",
			"customRules": []interface{}{
				map[string]interface{}{"segmentId": "ZZZ", "type": "P", "positions": []interface{}{"a", "b"}},
			},
		},
	}
	output, err := exec.Execute(context.Background(), step, map[string]interface{}{"raw": raw})
	if err != nil {
		t.Fatalf("expected no error for an unknown-segment custom rule, got: %v", err)
	}
	result := output["ediValidation"].(*validator.Result)
	for _, issue := range result.Issues {
		if issue.Path == "ZZZ" {
			t.Errorf("did not expect any issue for the unknown segment ZZZ, got: %+v", issue)
		}
	}
}

// TestEDIValidateExecutor_CustomRule_UnknownElementKey_SkippedGracefully
// verifies a custom rule on a REAL segment but with a stale/nonexistent
// element key resolves to zero positions and is skipped entirely, rather
// than producing a malformed SyntaxRule with empty Positions.
func TestEDIValidateExecutor_CustomRule_UnknownElementKey_SkippedGracefully(t *testing.T) {
	exec := newTestEDIValidateExecutor(t)
	raw := readValidateSample(t, "blue_cross_nc_sample.txt")

	step := &models.TransformationStep{
		StepName: "Test Validate EDI", StepType: "edi.validate", Enabled: true,
		Config: map[string]interface{}{
			"sourceField": "raw",
			"customRules": []interface{}{
				map[string]interface{}{"segmentId": "N3", "type": "P", "positions": []interface{}{"thisKeyDoesNotExist"}},
			},
		},
	}
	output, err := exec.Execute(context.Background(), step, map[string]interface{}{"raw": raw})
	if err != nil {
		t.Fatalf("expected no error for an unknown-element-key custom rule, got: %v", err)
	}
	result := output["ediValidation"].(*validator.Result)
	for _, issue := range result.Issues {
		if issue.Source == "custom" {
			t.Errorf("did not expect any custom-source issue when the rule's element key doesn't resolve, got: %+v", issue)
		}
	}
}
