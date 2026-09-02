// services/executors/transform/edi_map_to_canonical_executor_test.go
package transform

import (
	"context"
	"testing"

	"ezhealthkonnect/edi"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
)

// newTestEDIMapToCanonicalExecutor mirrors newTestEDIParseExecutor's own fix
// for the schema-directory-relative-to-CWD problem (see that file's comment).
func newTestEDIMapToCanonicalExecutor(t *testing.T) *EDIMapToCanonicalExecutor {
	t.Helper()
	loader, err := edi.NewX12SchemaLoader("../../../edi/schemas/x12_005010")
	if err != nil {
		t.Fatalf("failed to load EDI schema: %v", err)
	}
	return &EDIMapToCanonicalExecutor{
		BaseExecutor: executors.NewBaseExecutor("edi.map_to_canonical", models.ExecutorMetadata{
			Name: "Map to Canonical EDI JSON", Category: "EDI Transform",
		}),
		loader: loader,
	}
}

func runEdiMapToCanonical(t *testing.T, config map[string]interface{}, inputData map[string]interface{}) map[string]interface{} {
	t.Helper()
	executor := newTestEDIMapToCanonicalExecutor(t)
	step := &models.TransformationStep{
		StepName: "Test Map to Canonical EDI", StepType: "edi.map_to_canonical", Enabled: true,
		Config: config,
	}
	output, err := executor.Execute(context.Background(), step, inputData)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	return output
}

// TestEDIMapToCanonical_FlatHeaderFields verifies a simple header mapping
// (BPR's total amount + TRN's trace number) writes into
// canonicalDoc["header"]["<segmentId>"]["<elementKey>"].
func TestEDIMapToCanonical_FlatHeaderFields(t *testing.T) {
	config := map[string]interface{}{
		"outputField": "canonicalEDI",
		"header": []interface{}{
			map[string]interface{}{"segmentId": "BPR", "elementKey": "totalActualProviderPaymentAmount", "sourcePath": "payment.amount"},
			map[string]interface{}{"segmentId": "BPR", "elementKey": "transactionHandlingCode", "literalValue": "I"},
			map[string]interface{}{"segmentId": "TRN", "elementKey": "checkOrEFTTraceNumber", "sourcePath": "payment.traceNumber"},
		},
	}
	input := map[string]interface{}{
		"payment": map[string]interface{}{"amount": "45.75", "traceNumber": "TRACE123"},
	}
	output := runEdiMapToCanonical(t, config, input)

	doc, ok := output["canonicalEDI"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected canonicalEDI to be a map, got %T", output["canonicalEDI"])
	}
	header := doc["header"].(map[string]interface{})
	bpr := header["BPR"].(map[string]interface{})
	if bpr["totalActualProviderPaymentAmount"] != "45.75" {
		t.Errorf("BPR.totalActualProviderPaymentAmount = %v, want 45.75", bpr["totalActualProviderPaymentAmount"])
	}
	if bpr["transactionHandlingCode"] != "I" {
		t.Errorf("BPR.transactionHandlingCode = %v, want I (from literalValue)", bpr["transactionHandlingCode"])
	}
	trn := header["TRN"].(map[string]interface{})
	if trn["checkOrEFTTraceNumber"] != "TRACE123" {
		t.Errorf("TRN.checkOrEFTTraceNumber = %v, want TRACE123", trn["checkOrEFTTraceNumber"])
	}
}

// TestEDIMapToCanonical_SingleInstanceLoop verifies a loop with no rowsPath
// resolves against the SAME context (global inputData at top level) and
// writes one instance map, not an array.
func TestEDIMapToCanonical_SingleInstanceLoop(t *testing.T) {
	config := map[string]interface{}{
		"loops": []interface{}{
			map[string]interface{}{
				"loopId": "1000A",
				"fields": []interface{}{
					map[string]interface{}{"segmentId": "N1", "elementKey": "name", "sourcePath": "payer.name"},
				},
			},
		},
	}
	input := map[string]interface{}{"payer": map[string]interface{}{"name": "ACME PAYER"}}
	output := runEdiMapToCanonical(t, config, input)

	doc := output["canonicalEDI"].(map[string]interface{})
	loops := doc["loops"].(map[string]interface{})
	loop1000A, ok := loops["1000A"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected loop 1000A to be a single instance map, got %T", loops["1000A"])
	}
	n1 := loop1000A["N1"].(map[string]interface{})
	if n1["name"] != "ACME PAYER" {
		t.Errorf("N1.name = %v, want ACME PAYER", n1["name"])
	}
}

// TestEDIMapToCanonical_NestedRepeatingLoops_2000_2100_2110 verifies the
// real 835 3-level nesting (2000 -> 2100 -> 2110), each level repeating,
// with rowsPath resolved relative to the PARENT's own current row (not
// global inputData) at every nested level -- the core claim this step's own
// doc comment makes about handling arbitrary depth with one recursive
// function.
func TestEDIMapToCanonical_NestedRepeatingLoops_2000_2100_2110(t *testing.T) {
	config := map[string]interface{}{
		"loops": []interface{}{
			map[string]interface{}{
				"loopId":   "2000",
				"rowsPath": "headerGroups",
				"fields": []interface{}{
					map[string]interface{}{"segmentId": "LX", "elementKey": "assignedNumber", "sourcePath": "seq"},
				},
				"loops": []interface{}{
					map[string]interface{}{
						"loopId":   "2100",
						"rowsPath": "claims",
						"fields": []interface{}{
							map[string]interface{}{"segmentId": "CLP", "elementKey": "totalClaimChargeAmount", "sourcePath": "chargeAmount"},
						},
						"loops": []interface{}{
							map[string]interface{}{
								"loopId":   "2110",
								"rowsPath": "services",
								"fields": []interface{}{
									map[string]interface{}{"segmentId": "SVC", "elementKey": "monetaryAmount", "sourcePath": "amount"},
								},
							},
						},
					},
				},
			},
		},
	}
	input := map[string]interface{}{
		"headerGroups": []interface{}{
			map[string]interface{}{
				"seq": "1",
				"claims": []interface{}{
					map[string]interface{}{
						"chargeAmount": "100.00",
						"services": []interface{}{
							map[string]interface{}{"amount": "50.00"},
							map[string]interface{}{"amount": "50.00"},
						},
					},
					map[string]interface{}{
						"chargeAmount": "25.00",
						"services":     []interface{}{map[string]interface{}{"amount": "25.00"}},
					},
				},
			},
		},
	}
	output := runEdiMapToCanonical(t, config, input)

	doc := output["canonicalEDI"].(map[string]interface{})
	loops := doc["loops"].(map[string]interface{})

	loop2000, ok := loops["2000"].([]map[string]interface{})
	if !ok {
		t.Fatalf("expected loop 2000 to be []map[string]interface{}, got %T", loops["2000"])
	}
	if len(loop2000) != 1 {
		t.Fatalf("expected 1 loop 2000 instance, got %d", len(loop2000))
	}

	childLoops, ok := loop2000[0]["loops"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected loop 2000's own nested loops map, got %T", loop2000[0]["loops"])
	}
	claims, ok := childLoops["2100"].([]map[string]interface{})
	if !ok {
		t.Fatalf("expected loop 2100 to be []map[string]interface{}, got %T", childLoops["2100"])
	}
	if len(claims) != 2 {
		t.Fatalf("expected 2 claim (2100) instances, got %d", len(claims))
	}

	clp1 := claims[0]["CLP"].(map[string]interface{})
	if clp1["totalClaimChargeAmount"] != "100.00" {
		t.Errorf("first claim charge amount = %v, want 100.00", clp1["totalClaimChargeAmount"])
	}

	svcLoops1, ok := claims[0]["loops"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected first claim's own nested loops map, got %T", claims[0]["loops"])
	}
	services1, ok := svcLoops1["2110"].([]map[string]interface{})
	if !ok || len(services1) != 2 {
		t.Fatalf("expected 2 service (2110) instances under the first claim, got %v (%T)", services1, svcLoops1["2110"])
	}

	svcLoops2, ok := claims[1]["loops"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected second claim's own nested loops map, got %T", claims[1]["loops"])
	}
	services2, ok := svcLoops2["2110"].([]map[string]interface{})
	if !ok || len(services2) != 1 {
		t.Fatalf("expected 1 service (2110) instance under the second claim, got %v (%T)", services2, svcLoops2["2110"])
	}
}

// TestEDIMapToCanonical_ThenBuild_RoundTrips chains edi.map_to_canonical's
// output directly into edi.build's default sourceField ("parsedEDI" is
// edi.build's own default, but any field works — this test uses the
// step's own configured outputField) with zero reshaping, then re-parses
// the built document to confirm the mapped data survived a real round trip
// through edi/builder.BuildDocument -- not just an in-memory shape check.
func TestEDIMapToCanonical_ThenBuild_RoundTrips(t *testing.T) {
	mapConfig := map[string]interface{}{
		"outputField": "canonicalEDI",
		"header": []interface{}{
			map[string]interface{}{"segmentId": "BPR", "elementKey": "totalActualProviderPaymentAmount", "sourcePath": "amount"},
			map[string]interface{}{"segmentId": "TRN", "elementKey": "checkOrEFTTraceNumber", "sourcePath": "trace"},
		},
		"loops": []interface{}{
			map[string]interface{}{
				"loopId":   "2000",
				"rowsPath": "claims",
				"fields": []interface{}{
					map[string]interface{}{"segmentId": "LX", "elementKey": "assignedNumber", "sourcePath": "seq"},
				},
				"loops": []interface{}{
					map[string]interface{}{
						"loopId":   "2100",
						"fields": []interface{}{
							map[string]interface{}{"segmentId": "CLP", "elementKey": "totalClaimChargeAmount", "sourcePath": "amount"},
						},
					},
				},
			},
		},
	}
	input := map[string]interface{}{
		"amount": "100.00", "trace": "TR1",
		"claims": []interface{}{map[string]interface{}{"seq": "1", "amount": "75.50"}},
	}
	mapped := runEdiMapToCanonical(t, mapConfig, input)

	buildExec := newTestEDIBuildExecutor(t)
	buildStep := &models.TransformationStep{
		StepName: "Test Build EDI", StepType: "edi.build", Enabled: true,
		Config: map[string]interface{}{
			"sourceField": "canonicalEDI", "transactionSet": "835", "outputField": "ediX12",
			"isaSenderId": "MAPSENDER", "isaReceiverId": "MAPRECEIVER",
		},
	}
	built, err := buildExec.Execute(context.Background(), buildStep, mapped)
	if err != nil {
		t.Fatalf("edi.build Execute failed: %v", err)
	}

	reparsed, err := edi.ParseTransactionSet(buildExec.loader.Spec(), built["ediX12"].(string))
	if err != nil {
		t.Fatalf("re-parsing built document failed: %v\nbuilt was:\n%s", err, built["ediX12"])
	}
	header := reparsed.Header["BPR"].(map[string]interface{})
	if header["totalActualProviderPaymentAmount"] != "100.00" {
		t.Errorf("re-parsed BPR amount = %v, want 100.00", header["totalActualProviderPaymentAmount"])
	}
	loop2000 := reparsed.Loops["2000"].([]map[string]interface{})
	if len(loop2000) != 1 {
		t.Fatalf("re-parsed loop 2000 count = %d, want 1", len(loop2000))
	}
	// 2100 is schema-repeating (Repeat=">1") even though this test's own
	// mapping config set no rowsPath for it -- exactly the case this whole
	// executor rewrite exists for: the OUTPUT shape must still be a
	// []map[string]interface{}, matching what edi/builder.writeLoops
	// requires and what re-parsing a real repeating loop always produces.
	claims2100 := loop2000[0]["loops"].(map[string]interface{})["2100"].([]map[string]interface{})
	if len(claims2100) != 1 {
		t.Fatalf("re-parsed loop 2100 count = %d, want 1", len(claims2100))
	}
	clp := claims2100[0]["CLP"].(map[string]interface{})
	if clp["totalClaimChargeAmount"] != "75.50" {
		t.Errorf("re-parsed claim charge amount = %v, want 75.50", clp["totalClaimChargeAmount"])
	}
}
