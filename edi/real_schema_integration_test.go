// edi/real_schema_integration_test.go
// Validates the REAL, spec-sourced schema data in edi/schemas/x12_005010/
// (not the synthetic test schema loop_engine_test.go/document_builder_test.go
// use to exercise the engine's mechanisms in isolation) — loads it via
// NewX12SchemaLoader, builds a realistic-shaped 835 from canonical Go data,
// and re-parses it, exercising the real CAS/PLB/SVC composite and repeat-
// group definitions end to end.
package edi_test

import (
	"path/filepath"
	"runtime"
	"testing"

	"ezhealthkonnect/edi"
	"ezhealthkonnect/edi/builder"
)

func realSchemaDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test file location")
	}
	return filepath.Join(filepath.Dir(thisFile), "schemas", "x12_005010")
}

func TestRealSchema_LoadsWithoutError(t *testing.T) {
	loader, err := edi.NewX12SchemaLoader(realSchemaDir(t))
	if err != nil {
		t.Fatalf("NewX12SchemaLoader on real schema data: %v", err)
	}

	txSet := loader.GetTransactionSet("835")
	if txSet == nil {
		t.Fatal("GetTransactionSet(835) returned nil")
	}
	if len(txSet.Loops) != 3 {
		t.Errorf("expected 3 top-level loops (1000A, 1000B, 2000), got %d", len(txSet.Loops))
	}

	for _, id := range []string{"ST", "BPR", "TRN", "CUR", "N1", "N3", "N4", "REF", "PER", "RDM", "LX", "TS3", "TS2",
		"CLP", "NM1", "MIA", "MOA", "DTM", "AMT", "QTY", "CAS", "SVC", "LQ", "PLB", "SE"} {
		if loader.GetSegment(id) == nil {
			t.Errorf("segment %q not found in loaded shared library", id)
		}
	}

	if loader.Envelope() == nil || len(loader.Envelope().ISA) != 16 {
		t.Error("envelope ISA should have exactly 16 elements")
	}
}

func TestRealSchema_BuildAndRoundTrip_TwoClaims(t *testing.T) {
	loader, err := edi.NewX12SchemaLoader(realSchemaDir(t))
	if err != nil {
		t.Fatalf("NewX12SchemaLoader: %v", err)
	}
	spec := loader.Spec()

	input := builder.BuildInput{
		TransactionSet: "835",
		Interchange: map[string]interface{}{
			"senderId": "PAYERSENDER", "receiverId": "PROVIDERRECV",
		},
		Header: map[string]interface{}{
			"BPR": map[string]interface{}{
				"transactionHandlingCode": "I", "totalActualProviderPaymentAmount": "1250.00",
				"creditOrDebitFlagCode": "C", "paymentMethodCode": "ACH",
			},
			"TRN": map[string]interface{}{"traceTypeCode": "1", "checkOrEFTTraceNumber": "TRACE12345"},
		},
		Loops: map[string]interface{}{
			"1000A": map[string]interface{}{
				"N1": map[string]interface{}{"entityIdentifierCode": "PR", "name": "ACME PAYER"},
			},
			"1000B": map[string]interface{}{
				"N1": map[string]interface{}{"entityIdentifierCode": "PE", "name": "PROVIDER GROUP"},
			},
			// Both 2100 (claim) and 2110 (service line) are schema-declared
			// repeatable (Repeat: ">1", per the sourced Stedi cardinality —
			// see transactionSets/835.json's own _sourceRefs) even though a
			// real 835 typically carries exactly one 2100 per 2000. The
			// engine represents ANY schema-repeatable loop as an array
			// regardless of how many instances a given message actually has,
			// so canonical data must wrap even a single instance — this is
			// what TestRealSchema_BuildAndRoundTrip_TwoClaims's own first
			// draft got wrong (bare maps), caught by this very test panicking
			// on re-parse.
			"2000": []interface{}{
				map[string]interface{}{
					"LX": map[string]interface{}{"assignedNumber": "1"},
					"loops": map[string]interface{}{
						"2100": []interface{}{
							map[string]interface{}{
								"CLP": map[string]interface{}{
									"patientControlNumber": "PC001", "claimStatusCode": "1",
									"totalClaimChargeAmount": "500.00", "claimPaymentAmount": "400.00",
								},
								// CAS is schema maxUse ">1" (a segment can repeat, distinct from
								// its OWN intra-segment adjustments repeat group) — also needs
								// array wrapping, same reasoning as loops 2100/2110 above.
								"CAS": []interface{}{
									map[string]interface{}{
										"claimAdjustmentGroupCode": "CO",
										"adjustments": []interface{}{
											map[string]interface{}{"reasonCode": "45", "amount": "100.00"},
										},
									},
								},
								"loops": map[string]interface{}{
									"2110": []interface{}{
										map[string]interface{}{
											"SVC": map[string]interface{}{
												"procedureCode": map[string]interface{}{"qualifier": "HC", "code": "99213"},
												"chargeAmount":  "150.00", "paidAmount": "120.00",
											},
											"CAS": []interface{}{
												map[string]interface{}{
													"claimAdjustmentGroupCode": "CO",
													"adjustments": []interface{}{
														map[string]interface{}{"reasonCode": "45", "amount": "30.00"},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				},
				map[string]interface{}{
					"LX": map[string]interface{}{"assignedNumber": "2"},
					"loops": map[string]interface{}{
						"2100": []interface{}{
							map[string]interface{}{
								"CLP": map[string]interface{}{
									"patientControlNumber": "PC002", "claimStatusCode": "1",
									"totalClaimChargeAmount": "300.00", "claimPaymentAmount": "300.00",
								},
							},
						},
					},
				},
			},
		},
		Trailer: map[string]interface{}{
			// PLB is also schema maxUse ">1" — array-wrapped for the same
			// reason CAS is above.
			"PLB": []interface{}{
				map[string]interface{}{
					"referenceIdentification": "1234567890", "fiscalPeriodDate": "20261231",
					"adjustments": []interface{}{
						map[string]interface{}{
							"adjustmentIdentifier": map[string]interface{}{"reasonCode": "WO", "referenceIdentification": "REF1"},
							"amount":               "-25.00",
						},
					},
				},
			},
		},
	}

	built, err := builder.BuildDocument(spec, input)
	if err != nil {
		t.Fatalf("BuildDocument: %v", err)
	}

	result, err := edi.ParseTransactionSet(spec, built)
	if err != nil {
		t.Fatalf("re-parsing built 835: %v\nbuilt was:\n%s", err, built)
	}

	if !result.EnvelopePresent {
		t.Error("EnvelopePresent should be true")
	}
	if result.Interchange["senderId"] != "PAYERSENDER" {
		t.Errorf("senderId = %v, want PAYERSENDER", result.Interchange["senderId"])
	}

	loop2000, ok := result.Loops["2000"].([]map[string]interface{})
	if !ok || len(loop2000) != 2 {
		t.Fatalf("expected 2 instances of loop 2000, got %#v", result.Loops["2000"])
	}

	claim1Loops := loop2000[0]["loops"].(map[string]interface{})["2100"].([]map[string]interface{})
	if len(claim1Loops) != 1 {
		t.Fatalf("expected 1 instance of loop 2100 for the first claim, got %d", len(claim1Loops))
	}
	claim1 := claim1Loops[0]
	clp1 := claim1["CLP"].(map[string]interface{})
	if clp1["patientControlNumber"] != "PC001" {
		t.Errorf("first claim patientControlNumber = %v, want PC001", clp1["patientControlNumber"])
	}

	cas1List := claim1["CAS"].([]map[string]interface{})
	if len(cas1List) != 1 {
		t.Fatalf("expected 1 instance of claim-level CAS, got %d", len(cas1List))
	}
	cas1 := cas1List[0]
	if cas1["claimAdjustmentGroupCode"] != "CO" {
		t.Errorf("claim-level CAS group code = %v, want CO", cas1["claimAdjustmentGroupCode"])
	}
	adjustments1 := cas1["adjustments"].([]map[string]interface{})
	if len(adjustments1) != 1 || adjustments1[0]["reasonCode"] != "45" || adjustments1[0]["amount"] != "100.00" {
		t.Errorf("claim-level CAS adjustments = %#v", adjustments1)
	}

	svc1Loops := claim1["loops"].(map[string]interface{})["2110"].([]map[string]interface{})
	if len(svc1Loops) != 1 {
		t.Fatalf("expected 1 instance of loop 2110, got %d", len(svc1Loops))
	}
	svc1 := svc1Loops[0]["SVC"].(map[string]interface{})
	procCode := svc1["procedureCode"].(map[string]interface{})
	if procCode["qualifier"] != "HC" || procCode["code"] != "99213" {
		t.Errorf("SVC composite procedureCode = %#v", procCode)
	}

	trailerPLBList, ok := result.Trailer["PLB"].([]map[string]interface{})
	if !ok || len(trailerPLBList) != 1 {
		t.Fatalf("trailer PLB missing or wrong shape: %#v", result.Trailer["PLB"])
	}
	trailerPLB := trailerPLBList[0]
	plbAdjustments := trailerPLB["adjustments"].([]map[string]interface{})
	if len(plbAdjustments) != 1 {
		t.Fatalf("expected 1 PLB adjustment, got %d", len(plbAdjustments))
	}
	identifier := plbAdjustments[0]["adjustmentIdentifier"].(map[string]interface{})
	if identifier["reasonCode"] != "WO" || identifier["referenceIdentification"] != "REF1" {
		t.Errorf("PLB adjustment identifier composite = %#v", identifier)
	}
	if plbAdjustments[0]["amount"] != "-25.00" {
		t.Errorf("PLB adjustment amount = %v, want -25.00", plbAdjustments[0]["amount"])
	}
}
