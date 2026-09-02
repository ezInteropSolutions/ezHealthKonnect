// services/executors/transform/edi_pipeline_integration_test.go
// End-to-end coverage chaining edi.parse -> edi.build as two real,
// registered pipeline steps (not just edi/builder's own internal round-trip
// test) against a real, unedited 835 sample -- proves the executors' own
// config/field wiring (sourceField/outputField, ISA/GS config merge) works
// together, not just each engine layer in isolation.
package transform

import (
	"context"
	"testing"

	"ezhealthkonnect/edi"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
)

func newTestEDIBuildExecutor(t *testing.T) *EDIBuildExecutor {
	t.Helper()
	loader, err := edi.NewX12SchemaLoader("../../../edi/schemas/x12_005010")
	if err != nil {
		t.Fatalf("failed to load EDI schema: %v", err)
	}
	return &EDIBuildExecutor{
		BaseExecutor: executors.NewBaseExecutor("edi.build", models.ExecutorMetadata{
			Name: "EDI X12 Document Builder", Category: "EDI Transform",
		}),
		loader: loader,
	}
}

// TestEDIPipeline_ParseThenBuild_EMedNY_RoundTrips chains edi.parse's real
// output directly into edi.build's sourceField (default "parsedEDI") with
// zero reshaping -- proving the "no required reshaping" claim in
// edi_build_executor.go's own doc comment, not just asserting it in prose.
//
// isaSenderId/isaReceiverId are configured here too, but the ORIGINAL
// eMedNY sender/receiver ("EMEDNYBAT"/"ETIN") must survive, not the config
// values -- edi.build's mergeInterchangeConfig deliberately never overwrites
// a value the source data already supplies (see that function's own doc
// comment). That "don't clobber real data" behavior is exactly what this
// assertion is proving, not an oversight.
func TestEDIPipeline_ParseThenBuild_EMedNY_RoundTrips(t *testing.T) {
	parseExec := newTestEDIParseExecutor(t)
	buildExec := newTestEDIBuildExecutor(t)
	raw := readRealSample(t, "emedny_sample.txt")

	parseStep := &models.TransformationStep{
		StepName: "Test Parse EDI", StepType: "edi.parse", Enabled: true,
		Config: map[string]interface{}{"sourceField": "raw", "outputField": "parsedEDI"},
	}
	parsedOutput, err := parseExec.Execute(context.Background(), parseStep, map[string]interface{}{"raw": raw})
	if err != nil {
		t.Fatalf("edi.parse Execute failed: %v", err)
	}

	buildStep := &models.TransformationStep{
		StepName: "Test Build EDI", StepType: "edi.build", Enabled: true,
		Config: map[string]interface{}{
			"sourceField": "parsedEDI", "transactionSet": "835", "outputField": "ediX12",
			"isaSenderId": "OUTBOUNDSENDER", "isaReceiverId": "OUTBOUNDRECVR",
		},
	}
	builtOutput, err := buildExec.Execute(context.Background(), buildStep, parsedOutput)
	if err != nil {
		t.Fatalf("edi.build Execute failed: %v", err)
	}

	built, ok := builtOutput["ediX12"].(string)
	if !ok || built == "" {
		t.Fatalf("expected a non-empty ediX12 string output, got %T", builtOutput["ediX12"])
	}

	// Re-parse the rebuilt document from scratch and confirm the data that
	// matters survived the parse -> build round trip.
	reparsed, err := edi.ParseTransactionSet(buildExec.loader.Spec(), built)
	if err != nil {
		t.Fatalf("re-parsing built document failed: %v\nbuilt was:\n%s", err, built)
	}
	if !reparsed.EnvelopePresent {
		t.Error("expected the built document to carry a real ISA envelope")
	}
	if reparsed.Interchange["senderId"] != "EMEDNYBAT" {
		t.Errorf("re-parsed senderId = %v, want the original eMedNY ISA06 value EMEDNYBAT (config must not clobber real source data)", reparsed.Interchange["senderId"])
	}
	if reparsed.Interchange["receiverId"] != "ETIN" {
		t.Errorf("re-parsed receiverId = %v, want the original eMedNY ISA08 value ETIN", reparsed.Interchange["receiverId"])
	}

	header := reparsed.Header["BPR"].(map[string]interface{})
	if header["totalActualProviderPaymentAmount"] != "45.75" {
		t.Errorf("re-parsed BPR total amount = %v, want 45.75 (original eMedNY value)", header["totalActualProviderPaymentAmount"])
	}

	// One loop 2000 instance (one LX group); the 3 original claims are 3
	// instances of the CHILD loop 2100 nested inside it — see the sibling
	// edi.parse test's own comment for why.
	loop2000 := reparsed.Loops["2000"].([]map[string]interface{})
	if len(loop2000) != 1 {
		t.Fatalf("re-parsed loop 2000 count = %d, want 1 (one LX group)", len(loop2000))
	}
	claims := loop2000[0]["loops"].(map[string]interface{})["2100"].([]map[string]interface{})
	if len(claims) != 3 {
		t.Fatalf("re-parsed loop 2100 count = %d, want 3 (original eMedNY claim count)", len(claims))
	}

	clp1 := claims[0]["CLP"].(map[string]interface{})
	if clp1["totalClaimChargeAmount"] != "34.25" {
		t.Errorf("re-parsed first claim charge amount = %v, want 34.25", clp1["totalClaimChargeAmount"])
	}
}

// TestEDIBuildExecutor_ISAConfig_AppliesWhenSourceHasNoInterchange proves
// the other half of mergeInterchangeConfig's contract: when the source data
// carries no interchange values at all (e.g. canonical JSON assembled by an
// upstream mapping step, not a prior edi.parse), the step's own
// isaSenderId/isaReceiverId/gsSenderCode/gsReceiverCode config DOES apply --
// this is the "deployment-level trading-partner identifiers" case the
// config fields exist for.
func TestEDIBuildExecutor_ISAConfig_AppliesWhenSourceHasNoInterchange(t *testing.T) {
	buildExec := newTestEDIBuildExecutor(t)

	buildStep := &models.TransformationStep{
		StepName: "Test Build EDI", StepType: "edi.build", Enabled: true,
		Config: map[string]interface{}{
			"sourceField": "canonicalEDI", "transactionSet": "835", "outputField": "ediX12",
			"isaSenderId": "OUTBOUNDSENDER", "isaReceiverId": "OUTBOUNDRECVR",
		},
	}
	input := map[string]interface{}{
		"canonicalEDI": map[string]interface{}{
			"header": map[string]interface{}{
				// BPR and TRN are the header's own two "required" segments
				// (see 835.json's headerSegmentIds + each segment's own usage
				// flag) -- omitting either would fail BuildDocument's own
				// required-segment check before ever reaching ISA/GS output.
				"BPR": map[string]interface{}{"transactionHandlingCode": "I", "totalActualProviderPaymentAmount": "100.00"},
				"TRN": map[string]interface{}{"traceTypeCode": "1", "checkOrEFTTraceNumber": "TESTTRACE1"},
			},
			"loops": map[string]interface{}{},
		},
	}
	builtOutput, err := buildExec.Execute(context.Background(), buildStep, input)
	if err != nil {
		t.Fatalf("edi.build Execute failed: %v", err)
	}
	built := builtOutput["ediX12"].(string)

	reparsed, err := edi.ParseTransactionSet(buildExec.loader.Spec(), built)
	if err != nil {
		t.Fatalf("re-parsing built document failed: %v\nbuilt was:\n%s", err, built)
	}
	if reparsed.Interchange["senderId"] != "OUTBOUNDSENDER" {
		t.Errorf("re-parsed senderId = %v, want the config value OUTBOUNDSENDER (no source data to preserve instead)", reparsed.Interchange["senderId"])
	}
	if reparsed.Interchange["receiverId"] != "OUTBOUNDRECVR" {
		t.Errorf("re-parsed receiverId = %v, want the config value OUTBOUNDRECVR", reparsed.Interchange["receiverId"])
	}
}
