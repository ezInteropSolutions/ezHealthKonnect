// services/executors/transform/astm_executors_test.go
package transform

import (
	"context"
	"testing"

	"ezhealthkonnect/astm"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	astmparser "ezhealthkonnect/services/parsers/astm"
)

// newTestASTMParseExecutor / newTestASTMBuildExecutor construct the real
// executor directly with a schema path relative to THIS package's own
// directory (go test's CWD), bypassing NewASTMParseExecutor()/
// NewASTMBuildExecutor()'s hardcoded "./astm/schemas/e1394_97" (correct only
// relative to the repo root at server runtime) — same fix
// newTestEDIParseExecutor already applies for edi.parse.
func newTestASTMParseExecutor(t *testing.T) *ASTMParseExecutor {
	t.Helper()
	parser, err := astmparser.NewFromSchemaDir("../../../astm/schemas/e1394_97")
	if err != nil {
		t.Fatalf("failed to load ASTM schema: %v", err)
	}
	return &ASTMParseExecutor{
		BaseExecutor: executors.NewBaseExecutor("astm.parse", models.ExecutorMetadata{
			Name: "ASTM E1394-97 Parser (Raw → ParsedJSON)", Category: "ASTM Transform",
		}),
		parser: parser,
	}
}

func newTestASTMValidateExecutor(t *testing.T) *ASTMValidateExecutor {
	t.Helper()
	loader, err := astm.NewASTMSchemaLoader("../../../astm/schemas/e1394_97")
	if err != nil {
		t.Fatalf("failed to load ASTM schema: %v", err)
	}
	return &ASTMValidateExecutor{
		BaseExecutor: executors.NewBaseExecutor("astm.validate", models.ExecutorMetadata{
			Name: "ASTM E1394-97 Validator", Category: "ASTM Transform",
		}),
		loader: loader,
	}
}

func newTestASTMBuildExecutor(t *testing.T) *ASTMBuildExecutor {
	t.Helper()
	parser, err := astmparser.NewFromSchemaDir("../../../astm/schemas/e1394_97")
	if err != nil {
		t.Fatalf("failed to load ASTM schema: %v", err)
	}
	return &ASTMBuildExecutor{
		BaseExecutor: executors.NewBaseExecutor("astm.build", models.ExecutorMetadata{
			Name: "ASTM E1394-97 Message Builder", Category: "ASTM Transform",
		}),
		parser: parser,
	}
}

const astmSampleContent = "H|\\^&|MSGCTRL001||ANALYZER-1^1.0^SN12345|||||LIS||P|E1394-97|20231004120000\r" +
	"P|1||MRN12345||Doe^Jane||19800101|F\r" +
	"O|1|SPEC001||GLU|R\r" +
	"R|1|GLU|95|mg/dL|70-110|N||F\r" +
	"L|1|N\r"

// TestASTMExecutorChain_ParseValidateBuild_RoundTrips exercises the full
// astm.parse -> astm.validate -> astm.build chain the way a real pipeline
// runs them in sequence, each step's output data feeding the next.
func TestASTMExecutorChain_ParseValidateBuild_RoundTrips(t *testing.T) {
	parseExec := newTestASTMParseExecutor(t)
	validateExec := newTestASTMValidateExecutor(t)
	buildExec := newTestASTMBuildExecutor(t)

	parseStep := &models.TransformationStep{
		StepName: "Parse ASTM", StepType: "astm.parse", Enabled: true,
		Config: map[string]interface{}{"sourceField": "raw", "outputField": "parsedASTM"},
	}
	afterParse, err := parseExec.Execute(context.Background(), parseStep, map[string]interface{}{"raw": astmSampleContent})
	if err != nil {
		t.Fatalf("astm.parse Execute: %v", err)
	}
	parsedASTM, ok := afterParse["parsedASTM"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected parsedASTM to be a map, got %T", afterParse["parsedASTM"])
	}
	if parsedASTM["header"] == nil {
		t.Fatal("expected a non-nil header in parsedASTM")
	}

	validateStep := &models.TransformationStep{
		StepName: "Validate ASTM", StepType: "astm.validate", Enabled: true,
		Config: map[string]interface{}{"sourceField": "parsedASTM", "outputField": "astmValidation"},
	}
	afterValidate, err := validateExec.Execute(context.Background(), validateStep, afterParse)
	if err != nil {
		t.Fatalf("astm.validate Execute: %v", err)
	}
	validation, ok := afterValidate["astmValidation"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected astmValidation to be a map, got %T", afterValidate["astmValidation"])
	}
	if validation["valid"] != true {
		t.Errorf("expected a well-formed sample to validate cleanly, got: %#v", validation)
	}

	buildStep := &models.TransformationStep{
		StepName: "Build ASTM", StepType: "astm.build", Enabled: true,
		Config: map[string]interface{}{"sourceField": "parsedASTM", "outputField": "astmMessage"},
	}
	afterBuild, err := buildExec.Execute(context.Background(), buildStep, afterValidate)
	if err != nil {
		t.Fatalf("astm.build Execute: %v", err)
	}
	built, ok := afterBuild["astmMessage"].(string)
	if !ok || built == "" {
		t.Fatalf("expected a non-empty built ASTM message string, got %T %q", afterBuild["astmMessage"], built)
	}

	// Re-parse the built output and confirm the key fields survived the
	// full chain intact.
	reparsed, err := parseExec.Execute(context.Background(), parseStep, map[string]interface{}{"raw": built})
	if err != nil {
		t.Fatalf("re-parsing the built output: %v", err)
	}
	reparsedASTM := reparsed["parsedASTM"].(map[string]interface{})
	header := reparsedASTM["header"].(map[string]interface{})
	if header["messageControlID"] != "MSGCTRL001" {
		t.Errorf("round-tripped messageControlID = %v, want MSGCTRL001", header["messageControlID"])
	}
}

// TestASTMParseExecutor_NilParser_ReturnsClearError (BE-051) — when the
// schema directory failed to load at construction time, Execute() must
// return a clear Go error, never a nil-pointer panic.
func TestASTMParseExecutor_NilParser_ReturnsClearError(t *testing.T) {
	exec := &ASTMParseExecutor{
		BaseExecutor: executors.NewBaseExecutor("astm.parse", models.ExecutorMetadata{Name: "ASTM Parser", Category: "ASTM Transform"}),
		parser:       nil,
	}
	step := &models.TransformationStep{StepName: "Parse ASTM", StepType: "astm.parse", Enabled: true, Config: map[string]interface{}{}}
	_, err := exec.Execute(context.Background(), step, map[string]interface{}{"raw": astmSampleContent})
	if err == nil {
		t.Fatal("expected a clear error when the ASTM parser failed to initialise, got nil")
	}
}

func TestASTMValidateExecutor_NilLoader_ReturnsClearError(t *testing.T) {
	exec := &ASTMValidateExecutor{
		BaseExecutor: executors.NewBaseExecutor("astm.validate", models.ExecutorMetadata{Name: "ASTM Validator", Category: "ASTM Transform"}),
		loader:       nil,
	}
	step := &models.TransformationStep{StepName: "Validate ASTM", StepType: "astm.validate", Enabled: true, Config: map[string]interface{}{}}
	_, err := exec.Execute(context.Background(), step, map[string]interface{}{"raw": astmSampleContent})
	if err == nil {
		t.Fatal("expected a clear error when the ASTM schema failed to load, got nil")
	}
}

func TestASTMBuildExecutor_NilParser_ReturnsClearError(t *testing.T) {
	exec := &ASTMBuildExecutor{
		BaseExecutor: executors.NewBaseExecutor("astm.build", models.ExecutorMetadata{Name: "ASTM Builder", Category: "ASTM Transform"}),
		parser:       nil,
	}
	step := &models.TransformationStep{StepName: "Build ASTM", StepType: "astm.build", Enabled: true, Config: map[string]interface{}{}}
	_, err := exec.Execute(context.Background(), step, map[string]interface{}{"parsedASTM": map[string]interface{}{}})
	if err == nil {
		t.Fatal("expected a clear error when the ASTM schema failed to load, got nil")
	}
}
