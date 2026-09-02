// services/executors/transform/edi_parse_executor_test.go
package transform

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	edix12parser "ezhealthkonnect/services/parsers/edix12"
)

// newTestEDIParseExecutor constructs an EDIParseExecutor with a real schema
// loader without going through NewEDIParseExecutor()'s hardcoded
// "./edi/schemas/x12_005010" path -- that path is relative to the running
// process's CWD (correct at server runtime, from the repo root), not this
// test package's directory (`go test` runs with CWD = the package dir).
// Same fix newTestCDABuildExecutor already applies for cda.build.
func newTestEDIParseExecutor(t *testing.T) *EDIParseExecutor {
	t.Helper()
	parser, err := edix12parser.NewFromSchemaDir("../../../edi/schemas/x12_005010")
	if err != nil {
		t.Fatalf("failed to load EDI schema: %v", err)
	}
	return &EDIParseExecutor{
		BaseExecutor: executors.NewBaseExecutor("edi.parse", models.ExecutorMetadata{
			Name: "EDI X12 Parser (Raw → ParsedJSON)", Category: "EDI Transform",
		}),
		parser: parser,
	}
}

func readRealSample(t *testing.T, filename string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join("../../../edi/testdata/real_samples", filename))
	if err != nil {
		t.Fatalf("failed to read real sample %s: %v", filename, err)
	}
	return string(content)
}

// TestEDIParseExecutor_RealSample_BareST_BlueCrossNC exercises the no-ISA-
// envelope shape (the one processing/batch_splitter.go's splitEDITransactions
// produces for each part of a multi-transaction interchange) against a real,
// unedited 835 sourced from keironstoddart/edi-835-parser's own test fixtures.
func TestEDIParseExecutor_RealSample_BareST_BlueCrossNC(t *testing.T) {
	exec := newTestEDIParseExecutor(t)
	raw := readRealSample(t, "blue_cross_nc_sample.txt")

	step := &models.TransformationStep{
		StepName: "Test Parse EDI", StepType: "edi.parse", Enabled: true,
		Config: map[string]interface{}{"sourceField": "raw", "outputField": "parsedEDI"},
	}
	output, err := exec.Execute(context.Background(), step, map[string]interface{}{"raw": raw})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	parsed, ok := output["parsedEDI"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected parsedEDI to be a map, got %T", output["parsedEDI"])
	}
	if parsed["transactionSet"] != "835" {
		t.Errorf("transactionSet = %v, want \"835\"", parsed["transactionSet"])
	}
	if parsed["envelopePresent"] != false {
		t.Errorf("envelopePresent = %v, want false (bare ST, no ISA)", parsed["envelopePresent"])
	}

	loops, ok := parsed["loops"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected loops to be a map, got %T", parsed["loops"])
	}
	if _, ok := loops["2000"]; !ok {
		t.Errorf("expected loop 2000 (claim payment info) to be present in parsed loops: %v", loops)
	}
}

// TestEDIParseExecutor_RealSample_FullEnvelope_MultiClaim_EMedNY exercises
// full ISA...IEA envelope parsing with 3 CLP claims in one 2000 loop --
// real, unedited eMedNY 835 sample.
func TestEDIParseExecutor_RealSample_FullEnvelope_MultiClaim_EMedNY(t *testing.T) {
	exec := newTestEDIParseExecutor(t)
	raw := readRealSample(t, "emedny_sample.txt")

	step := &models.TransformationStep{
		StepName: "Test Parse EDI", StepType: "edi.parse", Enabled: true,
		Config: map[string]interface{}{"sourceField": "raw", "outputField": "parsedEDI"},
	}
	output, err := exec.Execute(context.Background(), step, map[string]interface{}{"raw": raw})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	parsed := output["parsedEDI"].(map[string]interface{})
	if parsed["envelopePresent"] != true {
		t.Errorf("envelopePresent = %v, want true (ISA present)", parsed["envelopePresent"])
	}

	interchange, ok := parsed["interchange"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected interchange to be a map, got %T", parsed["interchange"])
	}
	if interchange["senderId"] == nil || interchange["senderId"] == "" {
		t.Errorf("expected a non-empty ISA sender id, got %v", interchange["senderId"])
	}

	// The eMedNY sample has exactly one "LX*1" header-number group (loop
	// 2000) containing all 3 CLP claims nested inside it as 3 instances of
	// the CHILD loop 2100 -- not 3 separate 2000 instances. Confirmed
	// directly against 835.json's loop tree (2000 wraps LX/TS3/TS2; CLP
	// lives under 2100, nested one level deeper) and the raw sample itself
	// (a single "LX*1~" token, never repeated).
	loops := parsed["loops"].(map[string]interface{})
	loop2000, ok := loops["2000"].([]map[string]interface{})
	if !ok {
		t.Fatalf("expected loop 2000 to be a []map[string]interface{}, got %T", loops["2000"])
	}
	if len(loop2000) != 1 {
		t.Fatalf("expected 1 loop 2000 instance (one LX group), got %d", len(loop2000))
	}

	childLoops, ok := loop2000[0]["loops"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected loop 2000's own nested loops map, got %T", loop2000[0]["loops"])
	}
	claims, ok := childLoops["2100"].([]map[string]interface{})
	if !ok {
		t.Fatalf("expected loop 2100 to repeat (3 claims) as a []map[string]interface{}, got %T", childLoops["2100"])
	}
	if len(claims) != 3 {
		t.Errorf("expected 3 claim (CLP) instances, got %d", len(claims))
	}
}

// TestEDIParseExecutor_OutputFieldConfigured_RootMerge verifies the
// "__root__" outputField escape hatch merges ParsedJSON directly into the
// step's output data instead of nesting it under a named field.
func TestEDIParseExecutor_OutputFieldConfigured_RootMerge(t *testing.T) {
	exec := newTestEDIParseExecutor(t)
	raw := readRealSample(t, "blue_cross_nc_sample.txt")

	step := &models.TransformationStep{
		StepName: "Test Parse EDI", StepType: "edi.parse", Enabled: true,
		Config: map[string]interface{}{"sourceField": "raw", "outputField": "__root__"},
	}
	output, err := exec.Execute(context.Background(), step, map[string]interface{}{"raw": raw})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if output["transactionSet"] != "835" {
		t.Errorf("expected transactionSet merged at root, got: %v", output["transactionSet"])
	}
	if _, ok := output["parsedEDI"]; ok {
		t.Errorf("did not expect a nested parsedEDI field when outputField is __root__")
	}
}

// TestEDIParseExecutor_MessageUnwrapFallback verifies the
// inputData["message"] unwrap fallback (mirrors cda_parse_executor.go's own)
// so edi.parse works when it's not the pipeline's first step.
func TestEDIParseExecutor_MessageUnwrapFallback(t *testing.T) {
	exec := newTestEDIParseExecutor(t)
	raw := readRealSample(t, "blue_cross_nc_sample.txt")

	step := &models.TransformationStep{
		StepName: "Test Parse EDI", StepType: "edi.parse", Enabled: true,
		Config: map[string]interface{}{"sourceField": "raw", "outputField": "parsedEDI"},
	}
	input := map[string]interface{}{
		"message": map[string]interface{}{"raw": raw},
	}
	output, err := exec.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if output["parsedEDI"] == nil {
		t.Fatal("expected parsedEDI to be populated from the wrapped message.raw fallback")
	}
}

// TestEDIParseExecutor_EmptySourceField_ReturnsError verifies a genuine
// execution failure (no content anywhere) surfaces as a Go error, not a
// silently empty result.
func TestEDIParseExecutor_EmptySourceField_ReturnsError(t *testing.T) {
	exec := newTestEDIParseExecutor(t)
	step := &models.TransformationStep{
		StepName: "Test Parse EDI", StepType: "edi.parse", Enabled: true,
		Config: map[string]interface{}{"sourceField": "raw"},
	}
	_, err := exec.Execute(context.Background(), step, map[string]interface{}{})
	if err == nil {
		t.Fatal("expected an error when the source field has no content, got nil")
	}
}
