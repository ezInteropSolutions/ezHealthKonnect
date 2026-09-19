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
	"ezhealthkonnect/edi/validator"
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

// TestRealSchema_999_LoadsAndResolves proves the Phase 2 architecture fixes
// (composite lookup fallback, X12LoopDef.TrailerSegmentIDs) work against the
// real, spec-sourced 999 schema data, and that 835 registering under its own
// bare key is unaffected by 999 sharing the same manifest/envelope/ST files.
func TestRealSchema_999_LoadsAndResolves(t *testing.T) {
	loader, err := edi.NewX12SchemaLoader(realSchemaDir(t))
	if err != nil {
		t.Fatalf("NewX12SchemaLoader on real schema data: %v", err)
	}

	txSet := loader.GetTransactionSet("999")
	if txSet == nil {
		t.Fatal("GetTransactionSet(999) returned nil")
	}
	if txSet.FunctionalIdentifierCode != "FA" {
		t.Errorf("999 FunctionalIdentifierCode = %q, want FA", txSet.FunctionalIdentifierCode)
	}
	if txSet.VersionReleaseIndustryCode != "005010X231A1" {
		t.Errorf("999 VersionReleaseIndustryCode = %q, want 005010X231A1", txSet.VersionReleaseIndustryCode)
	}
	if txSet.EffectiveST01() != "999" {
		t.Errorf("999 EffectiveST01() = %q, want 999 (single-variant set, TransactionSetID doubles as ST01)", txSet.EffectiveST01())
	}
	if len(txSet.Loops) != 1 || txSet.Loops[0].ID != "2000" {
		t.Fatalf("expected exactly 1 top-level loop (2000), got %#v", txSet.Loops)
	}
	if len(txSet.Loops[0].TrailerSegmentIDs) != 1 || txSet.Loops[0].TrailerSegmentIDs[0] != "IK5" {
		t.Errorf("loop 2000 TrailerSegmentIDs = %#v, want [IK5]", txSet.Loops[0].TrailerSegmentIDs)
	}

	// 835 must still resolve under its own bare key, proving dual
	// registration and the shared envelope/ST files are unaffected by 999
	// now sharing them.
	if loader.GetTransactionSet("835") == nil {
		t.Fatal("GetTransactionSet(835) returned nil after adding 999 — regression in shared schema files")
	}

	for _, id := range []string{"AK1", "AK2", "IK3", "IK4", "IK5", "AK9", "CTX"} {
		if loader.GetSegment(id) == nil {
			t.Errorf("segment %q not found in loaded shared library", id)
		}
	}
}

// TestRealSchema_999_BuildAndRoundTrip_AcceptedWithOneError proves the full
// engine — build, GS01/GS08 defaulting from X12TransactionSetDef (Fix 1),
// bare-key parse-time resolution (Fix 2's fallback tier), the new loop-level
// TrailerSegmentIDs mechanism (IK5 after 2100, AK9/SE after 2000) — against
// a realistically-shaped 999 acknowledging one 835 with one segment-level
// error, round-tripped through edi/validator too (proving GS01/GS08 don't
// falsely flag on a message this engine built itself).
func TestRealSchema_999_BuildAndRoundTrip_AcceptedWithOneError(t *testing.T) {
	loader, err := edi.NewX12SchemaLoader(realSchemaDir(t))
	if err != nil {
		t.Fatalf("NewX12SchemaLoader: %v", err)
	}
	spec := loader.Spec()

	input := builder.BuildInput{
		TransactionSet: "999",
		Interchange: map[string]interface{}{
			"senderId": "EZHEALTHKONNECT", "receiverId": "PAYERSENDER",
		},
		Header: map[string]interface{}{
			"AK1": map[string]interface{}{
				"functionalIdentifierCode": "HP", "groupControlNumber": "100001", "versionReleaseIndustryCode": "005010X221A1",
			},
		},
		Loops: map[string]interface{}{
			"2000": []interface{}{
				map[string]interface{}{
					"AK2": map[string]interface{}{
						"transactionSetIdentifierCode": "835", "transactionSetControlNumber": "0001",
					},
					"loops": map[string]interface{}{
						"2100": []interface{}{
							map[string]interface{}{
								"IK3": map[string]interface{}{
									"segmentIdCode": "CLP", "segmentPosition": "12", "loopIdentifierCode": "2100", "segmentSyntaxErrorCode": "8",
								},
							},
						},
					},
					"IK5": map[string]interface{}{
						"transactionSetAcknowledgmentCode": "E", "syntaxErrorCode1": "5",
					},
				},
			},
		},
		Trailer: map[string]interface{}{
			"AK9": map[string]interface{}{
				"functionalGroupAcknowledgeCode": "E", "numberOfTransactionSetsIncluded": "1",
				"numberOfReceivedTransactionSets": "1", "numberOfAcceptedTransactionSets": "0",
			},
		},
	}

	built, err := builder.BuildDocument(spec, input)
	if err != nil {
		t.Fatalf("BuildDocument: %v", err)
	}

	result, err := edi.ParseTransactionSet(spec, built)
	if err != nil {
		t.Fatalf("re-parsing built 999: %v\nbuilt was:\n%s", err, built)
	}

	if result.TransactionSet != "999" {
		t.Errorf("TransactionSet = %q, want 999 (single-variant — bare-key fallback tier)", result.TransactionSet)
	}
	if result.Interchange["functionalIdentifierCode"] != "FA" {
		t.Errorf("GS01 = %v, want FA (defaulted from X12TransactionSetDef, Fix 1)", result.Interchange["functionalIdentifierCode"])
	}
	if result.Interchange["versionReleaseIndustryCode"] != "005010X231A1" {
		t.Errorf("GS08 = %v, want 005010X231A1", result.Interchange["versionReleaseIndustryCode"])
	}

	ak1 := result.Header["AK1"].(map[string]interface{})
	if ak1["functionalIdentifierCode"] != "HP" || ak1["groupControlNumber"] != "100001" {
		t.Errorf("AK1 = %#v", ak1)
	}

	// 2000 is schema-declared repeat ">1" — always an array regardless of
	// how many instances a given message actually has, same convention as
	// 835's own 2000/2100/2110 (see TestRealSchema_BuildAndRoundTrip_TwoClaims's
	// own comment above).
	loop2000List, ok := result.Loops["2000"].([]map[string]interface{})
	if !ok || len(loop2000List) != 1 {
		t.Fatalf("expected exactly 1 instance of loop 2000, got %#v", result.Loops["2000"])
	}
	loop2000 := loop2000List[0]
	ak2 := loop2000["AK2"].(map[string]interface{})
	if ak2["transactionSetIdentifierCode"] != "835" {
		t.Errorf("AK2 transactionSetIdentifierCode = %v, want 835", ak2["transactionSetIdentifierCode"])
	}

	// The real point of this test: IK5 must round-trip as a sibling of AK2
	// and "loops" WITHIN the same 2000 instance — proving TrailerSegmentIDs
	// is written after the nested 2100 loop on build and re-parsed back into
	// the same instance map, not lost or misplaced.
	ik5, ok := loop2000["IK5"].(map[string]interface{})
	if !ok {
		t.Fatalf("IK5 missing from 2000 loop instance — TrailerSegmentIDs round-trip broken: %#v", loop2000)
	}
	if ik5["transactionSetAcknowledgmentCode"] != "E" {
		t.Errorf("IK5 transactionSetAcknowledgmentCode = %v, want E", ik5["transactionSetAcknowledgmentCode"])
	}

	loop2100List := loop2000["loops"].(map[string]interface{})["2100"].([]map[string]interface{})
	if len(loop2100List) != 1 {
		t.Fatalf("expected exactly 1 instance of loop 2100, got %d", len(loop2100List))
	}
	ik3 := loop2100List[0]["IK3"].(map[string]interface{})
	if ik3["segmentIdCode"] != "CLP" || ik3["segmentSyntaxErrorCode"] != "8" {
		t.Errorf("IK3 = %#v", ik3)
	}

	ak9 := result.Trailer["AK9"].(map[string]interface{})
	if ak9["functionalGroupAcknowledgeCode"] != "E" {
		t.Errorf("AK9 functionalGroupAcknowledgeCode = %v, want E", ak9["functionalGroupAcknowledgeCode"])
	}

	// checkEnvelopeIdentity (Fix 1's validator counterpart) must NOT flag a
	// message this engine built itself from the resolved txSet's own values.
	valResult := validator.Validate(spec, result)
	for _, issue := range valResult.Issues {
		if issue.Severity == "error" {
			t.Errorf("unexpected error-severity issue on a self-built, correctly-enveloped 999: %+v", issue)
		}
	}
}

// TestRealSchema_837P_BuildAndRoundTrip_LoopRefResolvesIndependently proves
// the Phase 2 837P schema — GS08 composite disambiguation (Fix 2), the
// loop-level TrailerSegmentIDs mechanism at the transaction-set's own SE
// (unrelated loop-level use from 999, exercised here at the top level via
// HeaderSegmentIDs/TrailerSegmentIDs), and above all Fix 3's loopRef sharing.
// The real point of this test: 2300 (claim) is referenced from BOTH 2000B's
// own position AND 2000C's own position within ONE document — this only
// proves the mechanism if each occurrence carries genuinely DIFFERENT data
// and both round-trip correctly, not just that the loop resolves at all.
func TestRealSchema_837P_BuildAndRoundTrip_LoopRefResolvesIndependently(t *testing.T) {
	loader, err := edi.NewX12SchemaLoader(realSchemaDir(t))
	if err != nil {
		t.Fatalf("NewX12SchemaLoader: %v", err)
	}
	spec := loader.Spec()

	txSet := loader.GetTransactionSet("837P")
	if txSet == nil {
		t.Fatal("GetTransactionSet(837P) returned nil")
	}
	if txSet.EffectiveST01() != "837" {
		t.Errorf("837P EffectiveST01() = %q, want 837 (real wire value, distinct from the friendly schema id)", txSet.EffectiveST01())
	}

	claimLoop := func(patientControlNumber string) map[string]interface{} {
		return map[string]interface{}{
			"CLM": map[string]interface{}{
				"patientControlNumber": patientControlNumber, "totalClaimChargeAmount": "250.00",
				"healthCareServiceLocation": map[string]interface{}{
					"placeOfServiceCode": "11", "facilityCodeQualifier": "B", "claimFrequencyCode": "1",
				},
				"providerSignatureIndicator": "Y", "assignmentOrPlanParticipationCode": "A",
				"benefitsAssignmentCertificationIndicator": "Y", "releaseOfInformationCode": "Y",
			},
			"HI": []interface{}{
				map[string]interface{}{
					"codes": []interface{}{
						map[string]interface{}{"code": map[string]interface{}{"qualifier": "ABK", "code": "R51"}},
					},
				},
			},
			"loops": map[string]interface{}{
				"2400": []interface{}{
					map[string]interface{}{
						"LX": map[string]interface{}{"assignedNumber": "1"},
						"SV1": map[string]interface{}{
							"procedureCode":        map[string]interface{}{"qualifier": "HC", "code": "99213"},
							"lineItemChargeAmount": "250.00", "unitOfMeasurementCode": "UN", "serviceUnitCount": "1",
							"diagnosisCodePointer": map[string]interface{}{"pointer1": "1"},
						},
						"DTP": []interface{}{
							map[string]interface{}{"dateTimeQualifier": "472", "dateTimePeriodFormatQualifier": "D8", "datePeriod": "20260115"},
						},
					},
				},
			},
		}
	}

	input := builder.BuildInput{
		TransactionSet: "837P",
		Interchange:    map[string]interface{}{"senderId": "PROVIDER1", "receiverId": "PAYER1"},
		Header: map[string]interface{}{
			"BHT": map[string]interface{}{
				"hierarchicalStructureCode": "0019", "transactionSetPurposeCode": "00",
				"originatorApplicationTransactionIdentifier": "TX0001",
				"transactionSetCreationDate":                 "20260115", "transactionSetCreationTime": "1200",
				"claimOrEncounterIdentifier": "CH",
			},
		},
		Loops: map[string]interface{}{
			"1000A": map[string]interface{}{"NM1": map[string]interface{}{"entityIdentifierCode": "41", "entityTypeQualifier": "2", "nameLastOrOrganizationName": "ACME BILLING", "identificationCodeQualifier": "46", "identificationCode": "SUB001"}},
			"1000B": map[string]interface{}{"NM1": map[string]interface{}{"entityIdentifierCode": "40", "entityTypeQualifier": "2", "nameLastOrOrganizationName": "PAYER1", "identificationCodeQualifier": "46", "identificationCode": "RECV001"}},
			"2000A": []interface{}{
				map[string]interface{}{
					"HL": map[string]interface{}{"hierarchicalIdNumber": "1", "hierarchicalLevelCode": "20", "hierarchicalChildCode": "1"},
					"loops": map[string]interface{}{
						"2010AA": map[string]interface{}{"NM1": map[string]interface{}{"entityIdentifierCode": "85", "entityTypeQualifier": "2", "nameLastOrOrganizationName": "ACME CLINIC", "identificationCodeQualifier": "XX", "identificationCode": "1234567890"}},
						"2000B": []interface{}{
							map[string]interface{}{
								"HL":  map[string]interface{}{"hierarchicalIdNumber": "2", "hierarchicalParentIdNumber": "1", "hierarchicalLevelCode": "22", "hierarchicalChildCode": "1"},
								"SBR": map[string]interface{}{"payerResponsibilitySequenceNumberCode": "P", "individualRelationshipCode": "18"},
								"loops": map[string]interface{}{
									"2010BA": map[string]interface{}{"NM1": map[string]interface{}{"entityIdentifierCode": "IL", "entityTypeQualifier": "1", "nameLastOrOrganizationName": "SMITH", "nameFirst": "JANE", "identificationCodeQualifier": "MI", "identificationCode": "SUB123"}},
									"2010BB": map[string]interface{}{"NM1": map[string]interface{}{"entityIdentifierCode": "PR", "entityTypeQualifier": "2", "nameLastOrOrganizationName": "PAYER1", "identificationCodeQualifier": "PI", "identificationCode": "PAYER001"}},
									// Subscriber's OWN claim — first 2300 occurrence, via loopRef.
									"2300": []interface{}{claimLoop("SUBSCRIBER-CLAIM-001")},
									// A dependent patient — second 2300 occurrence, via the SAME
									// loopRef, nested one level deeper under 2000C, with its OWN
									// distinct patientControlNumber proving independent resolution.
									"2000C": []interface{}{
										map[string]interface{}{
											"HL":  map[string]interface{}{"hierarchicalIdNumber": "3", "hierarchicalParentIdNumber": "2", "hierarchicalLevelCode": "23", "hierarchicalChildCode": "0"},
											"PAT": map[string]interface{}{"unitOfMeasurementCode": "01"},
											"loops": map[string]interface{}{
												"2010CA": map[string]interface{}{"NM1": map[string]interface{}{"entityIdentifierCode": "QC", "entityTypeQualifier": "1", "nameLastOrOrganizationName": "SMITH", "nameFirst": "JUNIOR"}},
												"2300":   []interface{}{claimLoop("DEPENDENT-CLAIM-001")},
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
	}

	built, err := builder.BuildDocument(spec, input)
	if err != nil {
		t.Fatalf("BuildDocument: %v", err)
	}

	result, err := edi.ParseTransactionSet(spec, built)
	if err != nil {
		t.Fatalf("re-parsing built 837P: %v\nbuilt was:\n%s", err, built)
	}

	if result.TransactionSet != "837P" {
		t.Errorf("TransactionSet = %q, want 837P (composite ST01+GS08 lookup, Fix 2)", result.TransactionSet)
	}
	if result.Interchange["functionalIdentifierCode"] != "HC" {
		t.Errorf("GS01 = %v, want HC", result.Interchange["functionalIdentifierCode"])
	}
	if result.Interchange["versionReleaseIndustryCode"] != "005010X222A1" {
		t.Errorf("GS08 = %v, want 005010X222A1", result.Interchange["versionReleaseIndustryCode"])
	}

	// Drill down: 2000A -> 2000B -> both 2300 occurrences (subscriber's own,
	// and the nested 2000C dependent's own), confirming each carries its OWN
	// distinct patientControlNumber — proving loopRef gave each occurrence an
	// independently-owned tree, not a shared/aliased one. 2000A/2000B/2000C
	// are all schema-repeat ">1", so each parses as an array regardless of
	// how many instances this message actually has (same convention as
	// 835's own 2000/2100/2110 — see TestRealSchema_BuildAndRoundTrip_TwoClaims).
	loop2000AList := result.Loops["2000A"].([]map[string]interface{})
	if len(loop2000AList) != 1 {
		t.Fatalf("expected 1 instance of loop 2000A, got %d", len(loop2000AList))
	}
	loop2000BList := loop2000AList[0]["loops"].(map[string]interface{})["2000B"].([]map[string]interface{})
	if len(loop2000BList) != 1 {
		t.Fatalf("expected 1 instance of loop 2000B, got %d", len(loop2000BList))
	}
	loop2000B := loop2000BList[0]

	subscriberClaims := loop2000B["loops"].(map[string]interface{})["2300"].([]map[string]interface{})
	if len(subscriberClaims) != 1 {
		t.Fatalf("expected 1 subscriber-level claim, got %d", len(subscriberClaims))
	}
	subClaim := subscriberClaims[0]["CLM"].(map[string]interface{})
	if subClaim["patientControlNumber"] != "SUBSCRIBER-CLAIM-001" {
		t.Errorf("subscriber claim patientControlNumber = %v, want SUBSCRIBER-CLAIM-001", subClaim["patientControlNumber"])
	}

	loop2000CList := loop2000B["loops"].(map[string]interface{})["2000C"].([]map[string]interface{})
	if len(loop2000CList) != 1 {
		t.Fatalf("expected 1 instance of loop 2000C, got %d", len(loop2000CList))
	}
	loop2000C := loop2000CList[0]
	dependentClaims := loop2000C["loops"].(map[string]interface{})["2300"].([]map[string]interface{})
	if len(dependentClaims) != 1 {
		t.Fatalf("expected 1 dependent-level claim, got %d", len(dependentClaims))
	}
	depClaim := dependentClaims[0]["CLM"].(map[string]interface{})
	if depClaim["patientControlNumber"] != "DEPENDENT-CLAIM-001" {
		t.Errorf("dependent claim patientControlNumber = %v, want DEPENDENT-CLAIM-001", depClaim["patientControlNumber"])
	}

	// Both claims' own HI (12x repeat-group, see segments/HI.json) and 2400
	// service-line data must also have round-tripped correctly, independently.
	// HI is itself maxUse ">1" (a claim can carry several HI occurrences, one
	// per qualifier-group — see segments/HI.json's own maxUse-correction
	// note), so the segment itself is an array; this fixture only supplies
	// one occurrence.
	subHIInstances := subscriberClaims[0]["HI"].([]map[string]interface{})
	if len(subHIInstances) != 1 {
		t.Fatalf("expected 1 HI occurrence, got %d", len(subHIInstances))
	}
	subHI := subHIInstances[0]["codes"].([]map[string]interface{})
	if len(subHI) != 1 || subHI[0]["code"].(map[string]interface{})["code"] != "R51" {
		t.Errorf("subscriber claim HI codes = %#v", subHI)
	}
	subSV1 := subscriberClaims[0]["loops"].(map[string]interface{})["2400"].([]map[string]interface{})[0]["SV1"].(map[string]interface{})
	if subSV1["procedureCode"].(map[string]interface{})["code"] != "99213" {
		t.Errorf("subscriber claim SV1 procedure code = %#v", subSV1["procedureCode"])
	}

	// checkEnvelopeIdentity must not falsely flag this self-built 837P.
	valResult := validator.Validate(spec, result)
	for _, issue := range valResult.Issues {
		if issue.Severity == "error" {
			t.Errorf("unexpected error-severity issue on a self-built, correctly-enveloped 837P: %+v", issue)
		}
	}
}

// TestRealSchema_837I_BuildAndRoundTrip_DistinctFrom837P proves the 837I
// schema resolves independently of 837P (same raw ST01="837", disambiguated
// purely by GS08 — Fix 2's whole reason for existing), and exercises
// institutional-only segments 837P has no equivalent for: CL1, SV2, and a
// 2320 Other Subscriber occurrence carrying BOTH MIA and MOA together.
func TestRealSchema_837I_BuildAndRoundTrip_DistinctFrom837P(t *testing.T) {
	loader, err := edi.NewX12SchemaLoader(realSchemaDir(t))
	if err != nil {
		t.Fatalf("NewX12SchemaLoader: %v", err)
	}
	spec := loader.Spec()

	txSet := loader.GetTransactionSet("837I")
	if txSet == nil {
		t.Fatal("GetTransactionSet(837I) returned nil")
	}
	if txSet.EffectiveST01() != "837" {
		t.Errorf("837I EffectiveST01() = %q, want 837", txSet.EffectiveST01())
	}
	if txSet.VersionReleaseIndustryCode != "005010X223A2" {
		t.Errorf("837I VersionReleaseIndustryCode = %q, want 005010X223A2", txSet.VersionReleaseIndustryCode)
	}

	input := builder.BuildInput{
		TransactionSet: "837I",
		Interchange:    map[string]interface{}{"senderId": "HOSPITAL1", "receiverId": "PAYER1"},
		Header: map[string]interface{}{
			"BHT": map[string]interface{}{
				"hierarchicalStructureCode": "0019", "transactionSetPurposeCode": "00",
				"originatorApplicationTransactionIdentifier": "TX0002",
				"transactionSetCreationDate":                 "20260115", "transactionSetCreationTime": "1200",
				"claimOrEncounterIdentifier": "CH",
			},
		},
		Loops: map[string]interface{}{
			"1000A": map[string]interface{}{"NM1": map[string]interface{}{"entityIdentifierCode": "41", "entityTypeQualifier": "2", "nameLastOrOrganizationName": "GENERAL HOSPITAL", "identificationCodeQualifier": "46", "identificationCode": "SUB002"}},
			"1000B": map[string]interface{}{"NM1": map[string]interface{}{"entityIdentifierCode": "40", "entityTypeQualifier": "2", "nameLastOrOrganizationName": "PAYER1", "identificationCodeQualifier": "46", "identificationCode": "RECV001"}},
			"2000A": []interface{}{
				map[string]interface{}{
					"HL": map[string]interface{}{"hierarchicalIdNumber": "1", "hierarchicalLevelCode": "20", "hierarchicalChildCode": "1"},
					"loops": map[string]interface{}{
						"2010AA": map[string]interface{}{"NM1": map[string]interface{}{"entityIdentifierCode": "85", "entityTypeQualifier": "2", "nameLastOrOrganizationName": "GENERAL HOSPITAL", "identificationCodeQualifier": "XX", "identificationCode": "9876543210"}},
						"2000B": []interface{}{
							map[string]interface{}{
								"HL":  map[string]interface{}{"hierarchicalIdNumber": "2", "hierarchicalParentIdNumber": "1", "hierarchicalLevelCode": "22", "hierarchicalChildCode": "0"},
								"SBR": map[string]interface{}{"payerResponsibilitySequenceNumberCode": "P", "individualRelationshipCode": "18"},
								"loops": map[string]interface{}{
									"2010BA": map[string]interface{}{"NM1": map[string]interface{}{"entityIdentifierCode": "IL", "entityTypeQualifier": "1", "nameLastOrOrganizationName": "DOE", "nameFirst": "JOHN", "identificationCodeQualifier": "MI", "identificationCode": "SUB456"}},
									"2010BB": map[string]interface{}{"NM1": map[string]interface{}{"entityIdentifierCode": "PR", "entityTypeQualifier": "2", "nameLastOrOrganizationName": "PAYER1", "identificationCodeQualifier": "PI", "identificationCode": "PAYER001"}},
									"2300": []interface{}{
										map[string]interface{}{
											"CLM": map[string]interface{}{
												"patientControlNumber": "INST-CLAIM-001", "totalClaimChargeAmount": "5000.00",
												"healthCareServiceLocation": map[string]interface{}{
													"placeOfServiceCode": "11", "facilityCodeQualifier": "A", "claimFrequencyCode": "1",
												},
												"providerSignatureIndicator": "Y", "assignmentOrPlanParticipationCode": "A",
												"benefitsAssignmentCertificationIndicator": "Y", "releaseOfInformationCode": "Y",
											},
											"CL1": map[string]interface{}{
												"admissionTypeCode": "1", "admissionSourceCode": "1", "patientStatusCode": "01",
											},
											"HI": []interface{}{
												map[string]interface{}{
													"codes": []interface{}{
														map[string]interface{}{"code": map[string]interface{}{"qualifier": "ABK", "code": "I219"}},
													},
												},
											},
											"loops": map[string]interface{}{
												"2320": []interface{}{
													map[string]interface{}{
														"SBR": map[string]interface{}{"payerResponsibilitySequenceNumberCode": "S", "individualRelationshipCode": "01"},
														"OI":  map[string]interface{}{"benefitsAssignmentCertificationIndicator": "Y", "releaseOfInformationCode": "Y"},
														// Both MIA (inpatient) and MOA (outpatient) present together —
														// institutional's own real shape, unlike 837P which only ever has MOA.
														"MIA": map[string]interface{}{"coveredDaysCount": "5", "claimDRGAmount": "4500.00"},
														"MOA": map[string]interface{}{"reimbursementRate": "0.8"},
														"loops": map[string]interface{}{
															"2330A": map[string]interface{}{"NM1": map[string]interface{}{"entityIdentifierCode": "IL", "entityTypeQualifier": "1", "nameLastOrOrganizationName": "DOE", "nameFirst": "JOHN"}},
															"2330B": map[string]interface{}{"NM1": map[string]interface{}{"entityIdentifierCode": "PR", "entityTypeQualifier": "2", "nameLastOrOrganizationName": "SECONDARY PAYER"}},
														},
													},
												},
												"2400": []interface{}{
													map[string]interface{}{
														"LX": map[string]interface{}{"assignedNumber": "1"},
														"SV2": map[string]interface{}{
															"serviceLineRevenueCode": "0450",
															"procedureCode":          map[string]interface{}{"qualifier": "HC", "code": "99284"},
															"lineItemChargeAmount":   "5000.00", "unitOfMeasurementCode": "UN", "serviceUnitCount": "1",
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
		t.Fatalf("re-parsing built 837I: %v\nbuilt was:\n%s", err, built)
	}

	if result.TransactionSet != "837I" {
		t.Errorf("TransactionSet = %q, want 837I — confirms GS08=005010X223A2 correctly disambiguates from 837P's own GS08, not just falling back to bare '837'", result.TransactionSet)
	}
	if result.Interchange["versionReleaseIndustryCode"] != "005010X223A2" {
		t.Errorf("GS08 = %v, want 005010X223A2", result.Interchange["versionReleaseIndustryCode"])
	}

	loop2000A := result.Loops["2000A"].([]map[string]interface{})[0]
	loop2000B := loop2000A["loops"].(map[string]interface{})["2000B"].([]map[string]interface{})[0]
	claims := loop2000B["loops"].(map[string]interface{})["2300"].([]map[string]interface{})
	if len(claims) != 1 {
		t.Fatalf("expected 1 claim, got %d", len(claims))
	}
	claim := claims[0]

	cl1 := claim["CL1"].(map[string]interface{})
	if cl1["patientStatusCode"] != "01" {
		t.Errorf("CL1 patientStatusCode = %v, want 01", cl1["patientStatusCode"])
	}

	other2320 := claim["loops"].(map[string]interface{})["2320"].([]map[string]interface{})
	if len(other2320) != 1 {
		t.Fatalf("expected 1 instance of loop 2320, got %d", len(other2320))
	}
	mia, hasMIA := other2320[0]["MIA"].(map[string]interface{})
	moa, hasMOA := other2320[0]["MOA"].(map[string]interface{})
	if !hasMIA || !hasMOA {
		t.Fatalf("expected BOTH MIA and MOA present on the same 2320 instance, got MIA=%v MOA=%v", hasMIA, hasMOA)
	}
	if mia["coveredDaysCount"] != "5" || mia["claimDRGAmount"] != "4500.00" {
		t.Errorf("MIA = %#v", mia)
	}
	if moa["reimbursementRate"] != "0.8" {
		t.Errorf("MOA = %#v", moa)
	}

	sv2 := claim["loops"].(map[string]interface{})["2400"].([]map[string]interface{})[0]["SV2"].(map[string]interface{})
	if sv2["serviceLineRevenueCode"] != "0450" {
		t.Errorf("SV2 serviceLineRevenueCode = %v, want 0450", sv2["serviceLineRevenueCode"])
	}
	if sv2["procedureCode"].(map[string]interface{})["code"] != "99284" {
		t.Errorf("SV2 procedureCode = %#v", sv2["procedureCode"])
	}

	valResult := validator.Validate(spec, result)
	for _, issue := range valResult.Issues {
		if issue.Severity == "error" {
			t.Errorf("unexpected error-severity issue on a self-built, correctly-enveloped 837I: %+v", issue)
		}
	}
}

// TestRealSchema_270_BuildAndRoundTrip_SubscriberAndDependent proves the real,
// spec-sourced 270 schema (EDI X12 Phase 3) round-trips correctly through the
// SAME generic engine 835/837/999 already use — a subscriber asking about one
// service type, plus a dependent asking about a different one, exercising the
// full 2000A->2100A / 2000B->2100B / 2000C->2100C->2110C /
// 2000D->2100D->2110D tree and the HL03 trigger-discriminator mechanism.
func TestRealSchema_270_BuildAndRoundTrip_SubscriberAndDependent(t *testing.T) {
	loader, err := edi.NewX12SchemaLoader(realSchemaDir(t))
	if err != nil {
		t.Fatalf("NewX12SchemaLoader: %v", err)
	}
	spec := loader.Spec()

	txSet := loader.GetTransactionSet("270")
	if txSet == nil {
		t.Fatal("GetTransactionSet(270) returned nil")
	}
	if txSet.FunctionalIdentifierCode != "HS" {
		t.Errorf("270 FunctionalIdentifierCode = %q, want HS", txSet.FunctionalIdentifierCode)
	}
	if txSet.VersionReleaseIndustryCode != "005010X279A1" {
		t.Errorf("270 VersionReleaseIndustryCode = %q, want 005010X279A1", txSet.VersionReleaseIndustryCode)
	}

	input := builder.BuildInput{
		TransactionSet: "270",
		Interchange:    map[string]interface{}{"senderId": "PROVIDER1", "receiverId": "PAYER1"},
		Header: map[string]interface{}{
			"BHT": map[string]interface{}{
				"hierarchicalStructureCode": "0022", "transactionSetPurposeCode": "13",
				"originatorApplicationTransactionIdentifier": "ELIG0001",
				"transactionSetCreationDate":                 "20260115", "transactionSetCreationTime": "1200",
			},
		},
		Loops: map[string]interface{}{
			"2000A": []interface{}{
				map[string]interface{}{
					"HL": map[string]interface{}{"hierarchicalIdNumber": "1", "hierarchicalLevelCode": "20", "hierarchicalChildCode": "1"},
					"loops": map[string]interface{}{
						"2100A": map[string]interface{}{"NM1": map[string]interface{}{"entityIdentifierCode": "PR", "entityTypeQualifier": "2", "nameLastOrOrganizationName": "PAYER1", "identificationCodeQualifier": "PI", "identificationCode": "PAYER001"}},
						"2000B": []interface{}{
							map[string]interface{}{
								"HL": map[string]interface{}{"hierarchicalIdNumber": "2", "hierarchicalParentIdNumber": "1", "hierarchicalLevelCode": "21", "hierarchicalChildCode": "1"},
								"loops": map[string]interface{}{
									"2100B": map[string]interface{}{"NM1": map[string]interface{}{"entityIdentifierCode": "1P", "entityTypeQualifier": "2", "nameLastOrOrganizationName": "ACME CLINIC", "identificationCodeQualifier": "XX", "identificationCode": "1234567890"}},
									"2000C": []interface{}{
										map[string]interface{}{
											"HL":  map[string]interface{}{"hierarchicalIdNumber": "3", "hierarchicalParentIdNumber": "2", "hierarchicalLevelCode": "22", "hierarchicalChildCode": "1"},
											"TRN": map[string]interface{}{"traceTypeCode": "1", "checkOrEFTTraceNumber": "TRACE001"},
											"loops": map[string]interface{}{
												"2100C": map[string]interface{}{
													"NM1": map[string]interface{}{"entityIdentifierCode": "IL", "entityTypeQualifier": "1", "nameLastOrOrganizationName": "SMITH", "nameFirst": "JANE", "identificationCodeQualifier": "MI", "identificationCode": "SUB123"},
													"DMG": map[string]interface{}{"dateTimePeriodFormatQualifier": "D8", "birthDate": "19800101", "genderCode": "F"},
													"loops": map[string]interface{}{
														"2110C": []interface{}{
															map[string]interface{}{
																"EQ": map[string]interface{}{"serviceTypeCode": "30"},
															},
														},
													},
												},
												"2000D": []interface{}{
													map[string]interface{}{
														"HL":  map[string]interface{}{"hierarchicalIdNumber": "4", "hierarchicalParentIdNumber": "3", "hierarchicalLevelCode": "23", "hierarchicalChildCode": "0"},
														"TRN": map[string]interface{}{"traceTypeCode": "1", "checkOrEFTTraceNumber": "TRACE002"},
														"loops": map[string]interface{}{
															"2100D": map[string]interface{}{
																"NM1": map[string]interface{}{"entityIdentifierCode": "QC", "entityTypeQualifier": "1", "nameLastOrOrganizationName": "SMITH", "nameFirst": "TOMMY", "identificationCodeQualifier": "MI", "identificationCode": "SUB123"},
																"INS": map[string]interface{}{"yesNoConditionResponseCode": "N", "individualRelationshipCode": "19"},
																"loops": map[string]interface{}{
																	"2110D": []interface{}{
																		map[string]interface{}{
																			"EQ": map[string]interface{}{"serviceTypeCode": "98"},
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
								},
							},
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
		t.Fatalf("re-parsing built 270: %v\nbuilt was:\n%s", err, built)
	}

	if result.TransactionSet != "270" {
		t.Errorf("TransactionSet = %q, want 270", result.TransactionSet)
	}

	loop2000A := result.Loops["2000A"].([]map[string]interface{})[0]
	loop2000B := loop2000A["loops"].(map[string]interface{})["2000B"].([]map[string]interface{})[0]
	loop2000C := loop2000B["loops"].(map[string]interface{})["2000C"].([]map[string]interface{})[0]

	trn := loop2000C["TRN"].(map[string]interface{})
	if trn["checkOrEFTTraceNumber"] != "TRACE001" {
		t.Errorf("2000C TRN = %#v, want checkOrEFTTraceNumber=TRACE001", trn)
	}

	loop2100C := loop2000C["loops"].(map[string]interface{})["2100C"].(map[string]interface{})
	nm1 := loop2100C["NM1"].(map[string]interface{})
	if nm1["nameLastOrOrganizationName"] != "SMITH" || nm1["identificationCode"] != "SUB123" {
		t.Errorf("2100C NM1 = %#v", nm1)
	}

	loop2110C := loop2100C["loops"].(map[string]interface{})["2110C"].([]map[string]interface{})
	if len(loop2110C) != 1 {
		t.Fatalf("expected 1 instance of loop 2110C, got %d", len(loop2110C))
	}
	if loop2110C[0]["EQ"].(map[string]interface{})["serviceTypeCode"] != "30" {
		t.Errorf("2110C EQ = %#v, want serviceTypeCode=30", loop2110C[0]["EQ"])
	}

	loop2000D := loop2000C["loops"].(map[string]interface{})["2000D"].([]map[string]interface{})[0]
	loop2100D := loop2000D["loops"].(map[string]interface{})["2100D"].(map[string]interface{})
	if loop2100D["NM1"].(map[string]interface{})["nameFirst"] != "TOMMY" {
		t.Errorf("2100D NM1 = %#v, want nameFirst=TOMMY (proving the dependent tier is genuinely distinct from the subscriber tier)", loop2100D["NM1"])
	}
	if loop2100D["INS"].(map[string]interface{})["individualRelationshipCode"] != "19" {
		t.Errorf("2100D INS = %#v, want individualRelationshipCode=19", loop2100D["INS"])
	}
	loop2110D := loop2100D["loops"].(map[string]interface{})["2110D"].([]map[string]interface{})
	if len(loop2110D) != 1 || loop2110D[0]["EQ"].(map[string]interface{})["serviceTypeCode"] != "98" {
		t.Errorf("2110D EQ = %#v, want serviceTypeCode=98", loop2110D)
	}

	valResult := validator.Validate(spec, result)
	for _, issue := range valResult.Issues {
		if issue.Severity == "error" {
			t.Errorf("unexpected error-severity issue on a self-built, correctly-enveloped 270: %+v", issue)
		}
	}
}

// TestRealSchema_271_BuildAndRoundTrip_ActiveCoverageAndRejection proves the
// real, spec-sourced 271 schema round-trips correctly — a subscriber getting
// a real active-coverage EB answer (with a nested 2115C benefit-additional-
// info III), and a dependent getting an AAA rejection instead (subscriber/
// dependent not found), on the SAME transaction.
func TestRealSchema_271_BuildAndRoundTrip_ActiveCoverageAndRejection(t *testing.T) {
	loader, err := edi.NewX12SchemaLoader(realSchemaDir(t))
	if err != nil {
		t.Fatalf("NewX12SchemaLoader: %v", err)
	}
	spec := loader.Spec()

	txSet := loader.GetTransactionSet("271")
	if txSet == nil {
		t.Fatal("GetTransactionSet(271) returned nil")
	}
	if txSet.FunctionalIdentifierCode != "HB" {
		t.Errorf("271 FunctionalIdentifierCode = %q, want HB", txSet.FunctionalIdentifierCode)
	}

	input := builder.BuildInput{
		TransactionSet: "271",
		Interchange:    map[string]interface{}{"senderId": "PAYER1", "receiverId": "PROVIDER1"},
		Header: map[string]interface{}{
			"BHT": map[string]interface{}{
				"hierarchicalStructureCode": "0022", "transactionSetPurposeCode": "11",
				"originatorApplicationTransactionIdentifier": "ELIG0001",
				"transactionSetCreationDate":                 "20260115", "transactionSetCreationTime": "1201",
			},
		},
		Loops: map[string]interface{}{
			"2000A": []interface{}{
				map[string]interface{}{
					"HL": map[string]interface{}{"hierarchicalIdNumber": "1", "hierarchicalLevelCode": "20", "hierarchicalChildCode": "1"},
					"loops": map[string]interface{}{
						"2100A": map[string]interface{}{"NM1": map[string]interface{}{"entityIdentifierCode": "PR", "entityTypeQualifier": "2", "nameLastOrOrganizationName": "PAYER1", "identificationCodeQualifier": "PI", "identificationCode": "PAYER001"}},
						"2000B": []interface{}{
							map[string]interface{}{
								"HL": map[string]interface{}{"hierarchicalIdNumber": "2", "hierarchicalParentIdNumber": "1", "hierarchicalLevelCode": "21", "hierarchicalChildCode": "1"},
								"loops": map[string]interface{}{
									"2100B": map[string]interface{}{"NM1": map[string]interface{}{"entityIdentifierCode": "1P", "entityTypeQualifier": "2", "nameLastOrOrganizationName": "ACME CLINIC", "identificationCodeQualifier": "XX", "identificationCode": "1234567890"}},
									"2000C": []interface{}{
										map[string]interface{}{
											"HL":  map[string]interface{}{"hierarchicalIdNumber": "3", "hierarchicalParentIdNumber": "2", "hierarchicalLevelCode": "22", "hierarchicalChildCode": "1"},
											"TRN": map[string]interface{}{"traceTypeCode": "2", "checkOrEFTTraceNumber": "TRACE001"},
											"loops": map[string]interface{}{
												"2100C": map[string]interface{}{
													"NM1": map[string]interface{}{"entityIdentifierCode": "IL", "entityTypeQualifier": "1", "nameLastOrOrganizationName": "SMITH", "nameFirst": "JANE", "identificationCodeQualifier": "MI", "identificationCode": "SUB123"},
													"loops": map[string]interface{}{
														"2110C": []interface{}{
															map[string]interface{}{
																"EB": map[string]interface{}{"eligibilityBenefitInformationCode": "1", "coverageLevelCode": "IND", "serviceTypeCode": "30", "planCoverageDescription": "PPO GOLD"},
																"loops": map[string]interface{}{
																	"2115C": []interface{}{
																		map[string]interface{}{"III": map[string]interface{}{"codeListQualifierCode": "ZZ", "industryCode": "REMAINING VISITS: 5"}},
																	},
																	"2120C": []interface{}{
																		map[string]interface{}{"NM1": map[string]interface{}{"entityIdentifierCode": "P3", "entityTypeQualifier": "2", "nameLastOrOrganizationName": "CASE MANAGEMENT ORG"}},
																	},
																},
															},
														},
													},
												},
												"2000D": []interface{}{
													map[string]interface{}{
														"HL":  map[string]interface{}{"hierarchicalIdNumber": "4", "hierarchicalParentIdNumber": "3", "hierarchicalLevelCode": "23", "hierarchicalChildCode": "0"},
														"TRN": map[string]interface{}{"traceTypeCode": "2", "checkOrEFTTraceNumber": "TRACE002"},
														"loops": map[string]interface{}{
															"2100D": map[string]interface{}{
																"NM1": map[string]interface{}{"entityIdentifierCode": "QC", "entityTypeQualifier": "1", "nameLastOrOrganizationName": "UNKNOWN", "nameFirst": "DEP", "identificationCodeQualifier": "MI", "identificationCode": "SUB999"},
																"AAA": map[string]interface{}{"yesNoConditionResponseCode": "N", "rejectReasonCode": "72", "followUpActionCode": "C"},
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
		t.Fatalf("re-parsing built 271: %v\nbuilt was:\n%s", err, built)
	}

	if result.TransactionSet != "271" {
		t.Errorf("TransactionSet = %q, want 271", result.TransactionSet)
	}

	loop2000A := result.Loops["2000A"].([]map[string]interface{})[0]
	loop2000B := loop2000A["loops"].(map[string]interface{})["2000B"].([]map[string]interface{})[0]
	loop2000C := loop2000B["loops"].(map[string]interface{})["2000C"].([]map[string]interface{})[0]
	loop2100C := loop2000C["loops"].(map[string]interface{})["2100C"].(map[string]interface{})

	loop2110C := loop2100C["loops"].(map[string]interface{})["2110C"].([]map[string]interface{})
	if len(loop2110C) != 1 {
		t.Fatalf("expected 1 instance of loop 2110C, got %d", len(loop2110C))
	}
	eb := loop2110C[0]["EB"].(map[string]interface{})
	if eb["eligibilityBenefitInformationCode"] != "1" || eb["planCoverageDescription"] != "PPO GOLD" {
		t.Errorf("2110C EB = %#v", eb)
	}
	loop2115C := loop2110C[0]["loops"].(map[string]interface{})["2115C"].([]map[string]interface{})
	if len(loop2115C) != 1 || loop2115C[0]["III"].(map[string]interface{})["industryCode"] != "REMAINING VISITS: 5" {
		t.Errorf("2115C III = %#v", loop2115C)
	}
	// 2120C proves the real (not synthetic) 271 schema's own LS/LE-wrapped
	// Related Entity loop round-trips correctly — the whole point of adding
	// X12LoopDef.Wrapper — sibling to, not nested inside, 2115C above.
	loop2120C := loop2110C[0]["loops"].(map[string]interface{})["2120C"].([]map[string]interface{})
	if len(loop2120C) != 1 || loop2120C[0]["NM1"].(map[string]interface{})["nameLastOrOrganizationName"] != "CASE MANAGEMENT ORG" {
		t.Errorf("2120C NM1 = %#v", loop2120C)
	}

	loop2000D := loop2000C["loops"].(map[string]interface{})["2000D"].([]map[string]interface{})[0]
	loop2100D := loop2000D["loops"].(map[string]interface{})["2100D"].(map[string]interface{})
	aaaList, hasAAA := loop2100D["AAA"].([]map[string]interface{})
	if !hasAAA || len(aaaList) != 1 {
		t.Fatalf("expected 2100D to carry exactly 1 AAA rejection, got %#v", loop2100D)
	}
	aaa := aaaList[0]
	if aaa["yesNoConditionResponseCode"] != "N" || aaa["rejectReasonCode"] != "72" {
		t.Errorf("2100D AAA = %#v, want N/72 (subscriber/insured not found)", aaa)
	}
	// The rejected dependent tier correctly has NO 2110D benefit loop at all —
	// a structural fact this engine reflects, not decides.
	if _, has2110D := loop2100D["loops"]; has2110D {
		t.Errorf("expected no loops under a rejected 2100D, got %#v", loop2100D["loops"])
	}

	valResult := validator.Validate(spec, result)
	for _, issue := range valResult.Issues {
		if issue.Severity == "error" {
			t.Errorf("unexpected error-severity issue on a self-built, correctly-enveloped 271: %+v", issue)
		}
	}
}

// TestRealSchema_276_BuildAndRoundTrip_SubscriberAndDependentClaimStatus
// proves the real, spec-sourced 276 schema (EDI Phase 6) round-trips
// correctly through the same generic engine — a 5-level HL tree one level
// deeper than 270/271's own 4-level tree (276/277 add a genuine Service
// Provider HL level, 2000C, that 270/271 fold into a plain PRV segment
// instead). Exercises 2000A->2100A / 2000B->2100B / 2000C->2100C /
// 2000D->2100D->2200D->2210D / 2000E->2100E->2200E->2210E and the HL03
// trigger-discriminator mechanism at all 5 levels (20/21/19/22/23).
func TestRealSchema_276_BuildAndRoundTrip_SubscriberAndDependentClaimStatus(t *testing.T) {
	loader, err := edi.NewX12SchemaLoader(realSchemaDir(t))
	if err != nil {
		t.Fatalf("NewX12SchemaLoader: %v", err)
	}
	spec := loader.Spec()

	txSet := loader.GetTransactionSet("276")
	if txSet == nil {
		t.Fatal("GetTransactionSet(276) returned nil")
	}
	if txSet.FunctionalIdentifierCode != "HR" {
		t.Errorf("276 FunctionalIdentifierCode = %q, want HR", txSet.FunctionalIdentifierCode)
	}
	if txSet.VersionReleaseIndustryCode != "005010X212" {
		t.Errorf("276 VersionReleaseIndustryCode = %q, want 005010X212", txSet.VersionReleaseIndustryCode)
	}

	input := builder.BuildInput{
		TransactionSet: "276",
		Interchange:    map[string]interface{}{"senderId": "PROVIDER1", "receiverId": "PAYER1"},
		Header: map[string]interface{}{
			"BHT": map[string]interface{}{
				"hierarchicalStructureCode": "0010", "transactionSetPurposeCode": "13",
				"originatorApplicationTransactionIdentifier": "CLMSTAT01",
				"transactionSetCreationDate":                 "20260115", "transactionSetCreationTime": "1200",
			},
		},
		Loops: map[string]interface{}{
			"2000A": []interface{}{
				map[string]interface{}{
					"HL": map[string]interface{}{"hierarchicalIdNumber": "1", "hierarchicalLevelCode": "20", "hierarchicalChildCode": "1"},
					"loops": map[string]interface{}{
						"2100A": map[string]interface{}{"NM1": map[string]interface{}{"entityIdentifierCode": "PR", "entityTypeQualifier": "2", "nameLastOrOrganizationName": "PAYER1", "identificationCodeQualifier": "PI", "identificationCode": "PAYER001"}},
						"2000B": []interface{}{
							map[string]interface{}{
								"HL": map[string]interface{}{"hierarchicalIdNumber": "2", "hierarchicalParentIdNumber": "1", "hierarchicalLevelCode": "21", "hierarchicalChildCode": "1"},
								"loops": map[string]interface{}{
									"2100B": map[string]interface{}{"NM1": map[string]interface{}{"entityIdentifierCode": "41", "entityTypeQualifier": "2", "nameLastOrOrganizationName": "ACME CLEARINGHOUSE", "identificationCodeQualifier": "46", "identificationCode": "CLR001"}},
									"2000C": []interface{}{
										map[string]interface{}{
											"HL": map[string]interface{}{"hierarchicalIdNumber": "3", "hierarchicalParentIdNumber": "2", "hierarchicalLevelCode": "19", "hierarchicalChildCode": "1"},
											"loops": map[string]interface{}{
												"2100C": map[string]interface{}{"NM1": map[string]interface{}{"entityIdentifierCode": "1P", "entityTypeQualifier": "2", "nameLastOrOrganizationName": "ACME CLINIC", "identificationCodeQualifier": "XX", "identificationCode": "1234567890"}},
												"2000D": []interface{}{
													map[string]interface{}{
														"HL": map[string]interface{}{"hierarchicalIdNumber": "4", "hierarchicalParentIdNumber": "3", "hierarchicalLevelCode": "22", "hierarchicalChildCode": "1"},
														"loops": map[string]interface{}{
															"2100D": map[string]interface{}{
																"NM1": map[string]interface{}{"entityIdentifierCode": "IL", "entityTypeQualifier": "1", "nameLastOrOrganizationName": "SMITH", "nameFirst": "JANE", "identificationCodeQualifier": "MI", "identificationCode": "SUB123"},
																"DMG": map[string]interface{}{"dateTimePeriodFormatQualifier": "D8", "birthDate": "19800101", "genderCode": "F"},
																"loops": map[string]interface{}{
																	"2200D": []interface{}{
																		map[string]interface{}{
																			"TRN": map[string]interface{}{"traceTypeCode": "1", "checkOrEFTTraceNumber": "REQTRACE001"},
																			"REF": map[string]interface{}{"referenceIdentificationQualifier": "EJ", "referenceIdentification": "PCN0001"},
																			"AMT": map[string]interface{}{"amountQualifierCode": "T3", "monetaryAmount": "250.00"},
																			"DTP": map[string]interface{}{"dateTimeQualifier": "472", "dateTimePeriodFormatQualifier": "D8", "date": "20260110"},
																			"loops": map[string]interface{}{
																				"2210D": []interface{}{
																					map[string]interface{}{
																						"SVC": map[string]interface{}{"procedureCode": map[string]interface{}{"qualifier": "HC", "code": "99213"}, "chargeAmount": "250.00"},
																					},
																				},
																			},
																		},
																	},
																},
															},
															"2000E": []interface{}{
																map[string]interface{}{
																	"HL": map[string]interface{}{"hierarchicalIdNumber": "5", "hierarchicalParentIdNumber": "4", "hierarchicalLevelCode": "23", "hierarchicalChildCode": "0"},
																	"loops": map[string]interface{}{
																		"2100E": map[string]interface{}{
																			"NM1": map[string]interface{}{"entityIdentifierCode": "QC", "entityTypeQualifier": "1", "nameLastOrOrganizationName": "SMITH", "nameFirst": "TOMMY", "identificationCodeQualifier": "MI", "identificationCode": "SUB123"},
																			"DMG": map[string]interface{}{"dateTimePeriodFormatQualifier": "D8", "birthDate": "20100601", "genderCode": "M"},
																			"loops": map[string]interface{}{
																				"2200E": []interface{}{
																					map[string]interface{}{
																						"TRN": map[string]interface{}{"traceTypeCode": "1", "checkOrEFTTraceNumber": "REQTRACE002"},
																						"REF": map[string]interface{}{"referenceIdentificationQualifier": "EJ", "referenceIdentification": "PCN0002"},
																						"loops": map[string]interface{}{
																							"2210E": []interface{}{
																								map[string]interface{}{
																									"SVC": map[string]interface{}{"procedureCode": map[string]interface{}{"qualifier": "HC", "code": "90460"}, "chargeAmount": "75.00"},
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
		t.Fatalf("re-parsing built 276: %v\nbuilt was:\n%s", err, built)
	}

	if result.TransactionSet != "276" {
		t.Errorf("TransactionSet = %q, want 276", result.TransactionSet)
	}

	loop2000A := result.Loops["2000A"].([]map[string]interface{})[0]
	loop2000B := loop2000A["loops"].(map[string]interface{})["2000B"].([]map[string]interface{})[0]
	loop2000C := loop2000B["loops"].(map[string]interface{})["2000C"].([]map[string]interface{})[0]

	loop2100C := loop2000C["loops"].(map[string]interface{})["2100C"].(map[string]interface{})
	if loop2100C["NM1"].(map[string]interface{})["identificationCode"] != "1234567890" {
		t.Errorf("2100C NM1 = %#v, want identificationCode=1234567890 (proving the Service Provider level is genuinely distinct from Receiver/Payer)", loop2100C["NM1"])
	}

	loop2000D := loop2000C["loops"].(map[string]interface{})["2000D"].([]map[string]interface{})[0]
	loop2100D := loop2000D["loops"].(map[string]interface{})["2100D"].(map[string]interface{})
	if loop2100D["NM1"].(map[string]interface{})["identificationCode"] != "SUB123" {
		t.Errorf("2100D NM1 = %#v, want identificationCode=SUB123", loop2100D["NM1"])
	}

	loop2200D := loop2100D["loops"].(map[string]interface{})["2200D"].([]map[string]interface{})
	if len(loop2200D) != 1 {
		t.Fatalf("expected 1 instance of loop 2200D, got %d", len(loop2200D))
	}
	if loop2200D[0]["TRN"].(map[string]interface{})["checkOrEFTTraceNumber"] != "REQTRACE001" {
		t.Errorf("2200D TRN = %#v, want checkOrEFTTraceNumber=REQTRACE001", loop2200D[0]["TRN"])
	}
	loop2210D := loop2200D[0]["loops"].(map[string]interface{})["2210D"].([]map[string]interface{})
	if len(loop2210D) != 1 {
		t.Fatalf("expected 1 instance of loop 2210D, got %d", len(loop2210D))
	}
	svc := loop2210D[0]["SVC"].(map[string]interface{})
	procCode := svc["procedureCode"].(map[string]interface{})
	if procCode["code"] != "99213" {
		t.Errorf("2210D SVC.procedureCode = %#v, want code=99213", procCode)
	}

	// The dependent tier (2000E) is genuinely distinct from the subscriber
	// tier (2000D), its own HL parent points at the subscriber's own HL id
	// (proving the "Dependent's parent is always Subscriber, never Provider"
	// hierarchy semantics), and carries its own independent claim status
	// inquiry (2200E/2210E), not a copy of the subscriber's.
	loop2000E := loop2000D["loops"].(map[string]interface{})["2000E"].([]map[string]interface{})[0]
	loop2100E := loop2000E["loops"].(map[string]interface{})["2100E"].(map[string]interface{})
	if loop2100E["NM1"].(map[string]interface{})["nameFirst"] != "TOMMY" {
		t.Errorf("2100E NM1 = %#v, want nameFirst=TOMMY", loop2100E["NM1"])
	}
	loop2200E := loop2100E["loops"].(map[string]interface{})["2200E"].([]map[string]interface{})
	if len(loop2200E) != 1 || loop2200E[0]["TRN"].(map[string]interface{})["checkOrEFTTraceNumber"] != "REQTRACE002" {
		t.Errorf("2200E TRN = %#v, want checkOrEFTTraceNumber=REQTRACE002", loop2200E)
	}

	valResult := validator.Validate(spec, result)
	for _, issue := range valResult.Issues {
		if issue.Severity == "error" {
			t.Errorf("unexpected error-severity issue on a self-built, correctly-enveloped 276: %+v", issue)
		}
	}
}

// TestRealSchema_277_BuildAndRoundTrip_AcceptedAndFinalizedStatus proves the
// real, spec-sourced 277 schema round-trips correctly — a subscriber's claim
// coming back with an in-process status (STC category+status composite) and
// a dependent's claim coming back finalized/paid, on the SAME transaction,
// including 277's own extra receiver/provider-level trace+status loops
// (2200B/2200C) that 276 has no equivalent of.
func TestRealSchema_277_BuildAndRoundTrip_AcceptedAndFinalizedStatus(t *testing.T) {
	loader, err := edi.NewX12SchemaLoader(realSchemaDir(t))
	if err != nil {
		t.Fatalf("NewX12SchemaLoader: %v", err)
	}
	spec := loader.Spec()

	txSet := loader.GetTransactionSet("277")
	if txSet == nil {
		t.Fatal("GetTransactionSet(277) returned nil")
	}
	if txSet.FunctionalIdentifierCode != "HN" {
		t.Errorf("277 FunctionalIdentifierCode = %q, want HN", txSet.FunctionalIdentifierCode)
	}

	input := builder.BuildInput{
		TransactionSet: "277",
		Interchange:    map[string]interface{}{"senderId": "PAYER1", "receiverId": "PROVIDER1"},
		Header: map[string]interface{}{
			"BHT": map[string]interface{}{
				"hierarchicalStructureCode": "0010", "transactionSetPurposeCode": "08",
				"originatorApplicationTransactionIdentifier": "CLMSTAT01",
				"transactionSetCreationDate":                 "20260116", "transactionSetCreationTime": "0900",
			},
		},
		Loops: map[string]interface{}{
			"2000A": []interface{}{
				map[string]interface{}{
					"HL": map[string]interface{}{"hierarchicalIdNumber": "1", "hierarchicalLevelCode": "20", "hierarchicalChildCode": "1"},
					"loops": map[string]interface{}{
						"2100A": map[string]interface{}{"NM1": map[string]interface{}{"entityIdentifierCode": "PR", "entityTypeQualifier": "2", "nameLastOrOrganizationName": "PAYER1", "identificationCodeQualifier": "PI", "identificationCode": "PAYER001"}},
						"2000B": []interface{}{
							map[string]interface{}{
								"HL": map[string]interface{}{"hierarchicalIdNumber": "2", "hierarchicalParentIdNumber": "1", "hierarchicalLevelCode": "21", "hierarchicalChildCode": "1"},
								"loops": map[string]interface{}{
									"2100B": map[string]interface{}{
										"NM1": map[string]interface{}{"entityIdentifierCode": "41", "entityTypeQualifier": "2", "nameLastOrOrganizationName": "ACME CLEARINGHOUSE", "identificationCodeQualifier": "46", "identificationCode": "CLR001"},
										"loops": map[string]interface{}{
											"2200B": map[string]interface{}{
												"TRN": map[string]interface{}{"traceTypeCode": "2", "checkOrEFTTraceNumber": "RCVTRACE001"},
												"STC": map[string]interface{}{"healthCareClaimStatus": map[string]interface{}{"categoryCode": "A1", "statusCode": "20"}, "statusInformationEffectiveDate": "20260116"},
											},
										},
									},
									"2000C": []interface{}{
										map[string]interface{}{
											"HL": map[string]interface{}{"hierarchicalIdNumber": "3", "hierarchicalParentIdNumber": "2", "hierarchicalLevelCode": "19", "hierarchicalChildCode": "1"},
											"loops": map[string]interface{}{
												"2100C": map[string]interface{}{
													"NM1": map[string]interface{}{"entityIdentifierCode": "1P", "entityTypeQualifier": "2", "nameLastOrOrganizationName": "ACME CLINIC", "identificationCodeQualifier": "XX", "identificationCode": "1234567890"},
													"loops": map[string]interface{}{
														"2200C": map[string]interface{}{
															"TRN": map[string]interface{}{"traceTypeCode": "2", "checkOrEFTTraceNumber": "PRVTRACE001"},
															"STC": map[string]interface{}{"healthCareClaimStatus": map[string]interface{}{"categoryCode": "A1", "statusCode": "20"}, "statusInformationEffectiveDate": "20260116"},
														},
													},
												},
												"2000D": []interface{}{
													map[string]interface{}{
														"HL": map[string]interface{}{"hierarchicalIdNumber": "4", "hierarchicalParentIdNumber": "3", "hierarchicalLevelCode": "22", "hierarchicalChildCode": "1"},
														"loops": map[string]interface{}{
															"2100D": map[string]interface{}{
																"NM1": map[string]interface{}{"entityIdentifierCode": "IL", "entityTypeQualifier": "1", "nameLastOrOrganizationName": "SMITH", "nameFirst": "JANE", "identificationCodeQualifier": "MI", "identificationCode": "SUB123"},
																"loops": map[string]interface{}{
																	"2200D": []interface{}{
																		map[string]interface{}{
																			"TRN": map[string]interface{}{"traceTypeCode": "1", "checkOrEFTTraceNumber": "REQTRACE001"},
																			"STC": map[string]interface{}{"healthCareClaimStatus": map[string]interface{}{"categoryCode": "F1", "statusCode": "1"}, "statusInformationEffectiveDate": "20260116", "totalSubmittedChargeAmount": "250.00", "totalPaidAmount": "200.00"},
																			"REF": map[string]interface{}{"referenceIdentificationQualifier": "1K", "referenceIdentification": "PAYERCLM001"},
																		},
																	},
																},
															},
															"2000E": []interface{}{
																map[string]interface{}{
																	"HL": map[string]interface{}{"hierarchicalIdNumber": "5", "hierarchicalParentIdNumber": "4", "hierarchicalLevelCode": "23", "hierarchicalChildCode": "0"},
																	"loops": map[string]interface{}{
																		"2100E": map[string]interface{}{
																			"NM1": map[string]interface{}{"entityIdentifierCode": "QC", "entityTypeQualifier": "1", "nameLastOrOrganizationName": "SMITH", "nameFirst": "TOMMY", "identificationCodeQualifier": "MI", "identificationCode": "SUB123"},
																			"loops": map[string]interface{}{
																				"2200E": []interface{}{
																					map[string]interface{}{
																						"TRN": map[string]interface{}{"traceTypeCode": "1", "checkOrEFTTraceNumber": "REQTRACE002"},
																						"STC": map[string]interface{}{"healthCareClaimStatus": map[string]interface{}{"categoryCode": "A2", "statusCode": "35"}, "statusInformationEffectiveDate": "20260116"},
																						"REF": map[string]interface{}{"referenceIdentificationQualifier": "1K", "referenceIdentification": "PAYERCLM002"},
																						"loops": map[string]interface{}{
																							"2220E": []interface{}{
																								map[string]interface{}{
																									"SVC": map[string]interface{}{"procedureCode": map[string]interface{}{"qualifier": "HC", "code": "90460"}, "chargeAmount": "75.00", "paidAmount": "60.00"},
																									"STC": map[string]interface{}{"healthCareClaimStatus": map[string]interface{}{"categoryCode": "F1", "statusCode": "1"}},
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
		t.Fatalf("re-parsing built 277: %v\nbuilt was:\n%s", err, built)
	}

	if result.TransactionSet != "277" {
		t.Errorf("TransactionSet = %q, want 277", result.TransactionSet)
	}

	loop2000A := result.Loops["2000A"].([]map[string]interface{})[0]
	loop2000B := loop2000A["loops"].(map[string]interface{})["2000B"].([]map[string]interface{})[0]

	// 2200B (Information Receiver Trace) — an STC loop 276 has no equivalent of.
	loop2100B := loop2000B["loops"].(map[string]interface{})["2100B"].(map[string]interface{})
	loop2200B := loop2100B["loops"].(map[string]interface{})["2200B"].(map[string]interface{})
	// STC has maxUse=">1" in its own segment definition, so the parser always
	// wraps it as an array — schema-driven cardinality, not observed-count-driven,
	// matching this engine's established precedent (see edi.map_to_canonical's
	// own "schema decides wrapping" rule) — even a single real occurrence here.
	stc2200B := loop2200B["STC"].([]map[string]interface{})[0]["healthCareClaimStatus"].(map[string]interface{})
	if stc2200B["categoryCode"] != "A1" {
		t.Errorf("2200B STC = %#v, want categoryCode=A1", stc2200B)
	}

	loop2000C := loop2000B["loops"].(map[string]interface{})["2000C"].([]map[string]interface{})[0]
	loop2000D := loop2000C["loops"].(map[string]interface{})["2000D"].([]map[string]interface{})[0]
	loop2100D := loop2000D["loops"].(map[string]interface{})["2100D"].(map[string]interface{})
	loop2200D := loop2100D["loops"].(map[string]interface{})["2200D"].([]map[string]interface{})
	if len(loop2200D) != 1 {
		t.Fatalf("expected 1 instance of loop 2200D, got %d", len(loop2200D))
	}
	stc2200Dseg := loop2200D[0]["STC"].([]map[string]interface{})[0]
	stc2200D := stc2200Dseg["healthCareClaimStatus"].(map[string]interface{})
	if stc2200D["categoryCode"] != "F1" || stc2200D["statusCode"] != "1" {
		t.Errorf("2200D STC = %#v, want categoryCode=F1/statusCode=1 (finalized/paid)", stc2200D)
	}
	if stc2200Dseg["totalPaidAmount"] != "200.00" {
		t.Errorf("2200D STC.totalPaidAmount = %#v, want 200.00", stc2200Dseg)
	}

	// Dependent tier's own independent claim status, including its own
	// 2220E service-line-level STC — genuinely distinct from the
	// subscriber's own 2200D status, proving STC repeats correctly at both
	// the claim and service-line levels on the same built document.
	loop2000E := loop2000D["loops"].(map[string]interface{})["2000E"].([]map[string]interface{})[0]
	loop2100E := loop2000E["loops"].(map[string]interface{})["2100E"].(map[string]interface{})
	loop2200E := loop2100E["loops"].(map[string]interface{})["2200E"].([]map[string]interface{})
	if len(loop2200E) != 1 {
		t.Fatalf("expected 1 instance of loop 2200E, got %d", len(loop2200E))
	}
	stc2200E := loop2200E[0]["STC"].([]map[string]interface{})[0]["healthCareClaimStatus"].(map[string]interface{})
	if stc2200E["categoryCode"] != "A2" {
		t.Errorf("2200E STC = %#v, want categoryCode=A2 (pending/in-process, genuinely distinct from the subscriber's finalized F1)", stc2200E)
	}

	loop2220E := loop2200E[0]["loops"].(map[string]interface{})["2220E"].([]map[string]interface{})
	if len(loop2220E) != 1 {
		t.Fatalf("expected 1 instance of loop 2220E, got %d", len(loop2220E))
	}
	svcStc := loop2220E[0]["STC"].([]map[string]interface{})[0]["healthCareClaimStatus"].(map[string]interface{})
	if svcStc["categoryCode"] != "F1" {
		t.Errorf("2220E STC = %#v, want categoryCode=F1 (service line finalized even though the claim-level status is still A2/pending)", svcStc)
	}

	valResult := validator.Validate(spec, result)
	for _, issue := range valResult.Issues {
		if issue.Severity == "error" {
			t.Errorf("unexpected error-severity issue on a self-built, correctly-enveloped 277: %+v", issue)
		}
	}
}

// TestRealSchema_278_BuildAndRoundTrip_RequestAndResponseOnSameUnifiedSchema
// proves the real, spec-sourced 278 schema (EDI Phase 7, a SINGLE unified
// schema serving both directions -- see 278.json's own _sourceRefs) round-
// trips correctly for BOTH a pure REQUEST (the dependent's own patient event,
// no HCR present) and a RESPONSE carrying a real certification decision (the
// subscriber's own patient event, HCR present) on the SAME transaction --
// mirroring 271's own "prove both branches in one document" technique.
func TestRealSchema_278_BuildAndRoundTrip_RequestAndResponseOnSameUnifiedSchema(t *testing.T) {
	loader, err := edi.NewX12SchemaLoader(realSchemaDir(t))
	if err != nil {
		t.Fatalf("NewX12SchemaLoader: %v", err)
	}
	spec := loader.Spec()

	txSet := loader.GetTransactionSet("278")
	if txSet == nil {
		t.Fatal("GetTransactionSet(278) returned nil")
	}
	if txSet.FunctionalIdentifierCode != "HI" {
		t.Errorf("278 FunctionalIdentifierCode = %q, want HI", txSet.FunctionalIdentifierCode)
	}
	if txSet.VersionReleaseIndustryCode != "005010X217" {
		t.Errorf("278 VersionReleaseIndustryCode = %q, want 005010X217", txSet.VersionReleaseIndustryCode)
	}

	input := builder.BuildInput{
		TransactionSet: "278",
		Interchange:    map[string]interface{}{"senderId": "ACMEUMO", "receiverId": "ACMECLINIC"},
		Header: map[string]interface{}{
			"BHT": map[string]interface{}{
				"hierarchicalStructureCode": "0078", "transactionSetPurposeCode": "13",
				"originatorApplicationTransactionIdentifier": "PA0001",
				"transactionSetCreationDate":                 "20260914", "transactionSetCreationTime": "0900",
			},
		},
		Loops: map[string]interface{}{
			"2000A": []interface{}{
				map[string]interface{}{
					"HL": map[string]interface{}{"hierarchicalIdNumber": "1", "hierarchicalLevelCode": "20", "hierarchicalChildCode": "1"},
					"loops": map[string]interface{}{
						"2010A": map[string]interface{}{"NM1": map[string]interface{}{"entityIdentifierCode": "X3", "entityTypeQualifier": "2", "nameLastOrOrganizationName": "ACME UMO", "identificationCodeQualifier": "PI", "identificationCode": "UMO001"}},
						"2000B": []interface{}{
							map[string]interface{}{
								"HL": map[string]interface{}{"hierarchicalIdNumber": "2", "hierarchicalParentIdNumber": "1", "hierarchicalLevelCode": "21", "hierarchicalChildCode": "1"},
								"loops": map[string]interface{}{
									"2010B": []interface{}{
										map[string]interface{}{"NM1": map[string]interface{}{"entityIdentifierCode": "1P", "entityTypeQualifier": "2", "nameLastOrOrganizationName": "ACME CLINIC", "identificationCodeQualifier": "XX", "identificationCode": "1234567890"}},
									},
									"2000C": []interface{}{
										map[string]interface{}{
											"HL": map[string]interface{}{"hierarchicalIdNumber": "3", "hierarchicalParentIdNumber": "2", "hierarchicalLevelCode": "22", "hierarchicalChildCode": "1"},
											"loops": map[string]interface{}{
												"2010C": map[string]interface{}{"NM1": map[string]interface{}{"entityIdentifierCode": "IL", "entityTypeQualifier": "1", "nameLastOrOrganizationName": "SMITH", "nameFirst": "JANE", "identificationCodeQualifier": "MI", "identificationCode": "SUB123"}},
												// 2000E here is the SUBSCRIBER's own patient event -- a
												// SIBLING of 2000D within 2000C's own loops (matching the
												// "Dependent's real HL parent is the Subscriber, never the
												// Provider" rule this session already learned twice for
												// 276/277 -- 278's own tree has the SAME shape one level
												// deeper). This one carries HCR: it is a RESPONSE.
												"2000D": []interface{}{
													map[string]interface{}{
														"HL": map[string]interface{}{"hierarchicalIdNumber": "4", "hierarchicalParentIdNumber": "3", "hierarchicalLevelCode": "23", "hierarchicalChildCode": "1"},
														"loops": map[string]interface{}{
															"2010D": map[string]interface{}{"NM1": map[string]interface{}{"entityIdentifierCode": "QC", "entityTypeQualifier": "1", "nameLastOrOrganizationName": "SMITH", "nameFirst": "TOMMY", "identificationCodeQualifier": "MI", "identificationCode": "SUB123"}},
															// The DEPENDENT's own patient event -- no HCR: a
															// pure REQUEST, no decision made yet.
															"2000E": map[string]interface{}{
																"HL": map[string]interface{}{"hierarchicalIdNumber": "5", "hierarchicalParentIdNumber": "4", "hierarchicalLevelCode": "EV", "hierarchicalChildCode": "1"},
																"TRN": map[string]interface{}{"traceTypeCode": "1", "checkOrEFTTraceNumber": "EVENTTRACE002"},
																"UM":  map[string]interface{}{"requestCategoryCode": "HS", "certificationTypeCode": "I", "serviceTypeCode": "1"},
																"loops": map[string]interface{}{
																	"2000F": []interface{}{
																		map[string]interface{}{
																			"HL":  map[string]interface{}{"hierarchicalIdNumber": "6", "hierarchicalParentIdNumber": "5", "hierarchicalLevelCode": "SS", "hierarchicalChildCode": "0"},
																			"TRN": map[string]interface{}{"traceTypeCode": "1", "checkOrEFTTraceNumber": "SVCTRACE002"},
																			"SV1": map[string]interface{}{"procedureCode": map[string]interface{}{"qualifier": "HC", "code": "90471"}},
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
								},
							},
						},
					},
				},
			},
		},
	}
	// Splice the subscriber's own 2000E (a RESPONSE, HCR present) in as a
	// sibling of 2000D within 2000C's own loops -- done here via direct map
	// manipulation (rather than inline above) purely to keep the literal
	// above readable; the resulting shape is identical to writing it inline.
	loop2000CInner := input.Loops["2000A"].([]interface{})[0].(map[string]interface{})["loops"].(map[string]interface{})["2000B"].([]interface{})[0].(map[string]interface{})["loops"].(map[string]interface{})["2000C"].([]interface{})[0].(map[string]interface{})["loops"].(map[string]interface{})
	loop2000CInner["2000E"] = map[string]interface{}{
		"HL":  map[string]interface{}{"hierarchicalIdNumber": "7", "hierarchicalParentIdNumber": "3", "hierarchicalLevelCode": "EV", "hierarchicalChildCode": "1"},
		"TRN": map[string]interface{}{"traceTypeCode": "1", "checkOrEFTTraceNumber": "EVENTTRACE001"},
		"UM":  map[string]interface{}{"requestCategoryCode": "HS", "certificationTypeCode": "I", "serviceTypeCode": "1"},
		"HCR": map[string]interface{}{"actionCode": "A1", "certificationNumber": "AUTH99001"},
		"loops": map[string]interface{}{
			"2000F": []interface{}{
				map[string]interface{}{
					"HL":  map[string]interface{}{"hierarchicalIdNumber": "8", "hierarchicalParentIdNumber": "7", "hierarchicalLevelCode": "SS", "hierarchicalChildCode": "0"},
					"TRN": map[string]interface{}{"traceTypeCode": "1", "checkOrEFTTraceNumber": "SVCTRACE001"},
					"SV1": map[string]interface{}{"procedureCode": map[string]interface{}{"qualifier": "HC", "code": "99213"}},
					"HCR": map[string]interface{}{"actionCode": "A1", "certificationNumber": "AUTH99001-SVC"},
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
		t.Fatalf("re-parsing built 278: %v\nbuilt was:\n%s", err, built)
	}
	if result.TransactionSet != "278" {
		t.Errorf("TransactionSet = %q, want 278", result.TransactionSet)
	}

	loop2000A := result.Loops["2000A"].([]map[string]interface{})[0]
	loop2000B := loop2000A["loops"].(map[string]interface{})["2000B"].([]map[string]interface{})[0]
	loop2000C := loop2000B["loops"].(map[string]interface{})["2000C"].([]map[string]interface{})[0]

	// The subscriber's own 2000E (sibling of 2000D) is the RESPONSE.
	loop2000ESub := loop2000C["loops"].(map[string]interface{})["2000E"].(map[string]interface{})
	if loop2000ESub["TRN"].(map[string]interface{})["checkOrEFTTraceNumber"] != "EVENTTRACE001" {
		t.Errorf("subscriber 2000E TRN = %#v, want EVENTTRACE001", loop2000ESub["TRN"])
	}
	if loop2000ESub["HCR"] == nil {
		t.Fatal("subscriber 2000E should carry HCR (a certification decision) -- it's the RESPONSE side")
	}
	if loop2000ESub["HCR"].(map[string]interface{})["actionCode"] != "A1" {
		t.Errorf("subscriber 2000E HCR = %#v, want actionCode=A1", loop2000ESub["HCR"])
	}
	loop2000FSub := loop2000ESub["loops"].(map[string]interface{})["2000F"].([]map[string]interface{})[0]
	if loop2000FSub["SV1"].(map[string]interface{})["procedureCode"].(map[string]interface{})["code"] != "99213" {
		t.Errorf("subscriber 2000F SV1 = %#v, want procedureCode.code=99213", loop2000FSub["SV1"])
	}

	// The dependent's own 2000E (nested inside 2000D) is the pure REQUEST --
	// no HCR present anywhere in this branch.
	loop2000D := loop2000C["loops"].(map[string]interface{})["2000D"].([]map[string]interface{})[0]
	if loop2000D["loops"].(map[string]interface{})["2010D"].(map[string]interface{})["NM1"].(map[string]interface{})["nameFirst"] != "TOMMY" {
		t.Errorf("2010D NM1 = %#v, want nameFirst=TOMMY", loop2000D["loops"].(map[string]interface{})["2010D"])
	}
	loop2000EDep := loop2000D["loops"].(map[string]interface{})["2000E"].(map[string]interface{})
	if loop2000EDep["TRN"].(map[string]interface{})["checkOrEFTTraceNumber"] != "EVENTTRACE002" {
		t.Errorf("dependent 2000E TRN = %#v, want EVENTTRACE002", loop2000EDep["TRN"])
	}
	if loop2000EDep["HCR"] != nil {
		t.Errorf("dependent 2000E should carry NO HCR (it's a pure REQUEST, no decision made yet), got %#v", loop2000EDep["HCR"])
	}
	loop2000FDep := loop2000EDep["loops"].(map[string]interface{})["2000F"].([]map[string]interface{})[0]
	if loop2000FDep["SV1"].(map[string]interface{})["procedureCode"].(map[string]interface{})["code"] != "90471" {
		t.Errorf("dependent 2000F SV1 = %#v, want procedureCode.code=90471", loop2000FDep["SV1"])
	}
	if loop2000FDep["HCR"] != nil {
		t.Errorf("dependent 2000F should carry NO HCR, got %#v", loop2000FDep["HCR"])
	}

	valResult := validator.Validate(spec, result)
	for _, issue := range valResult.Issues {
		if issue.Severity == "error" {
			t.Errorf("unexpected error-severity issue on a self-built, correctly-enveloped 278: %+v", issue)
		}
	}
}

// TestRealSchema_834_BuildAndRoundTrip_ActiveAndTerminatedMembers proves the
// real, spec-sourced 834 schema (EDI Phase 7, a genuinely different FLAT
// shape -- no HL hierarchy, every member is its own top-level 2000 instance
// triggered by INS) round-trips correctly for two members on the same file:
// a subscriber with an ACTIVE health-coverage enrollment (HD01="021"
// Addition) and a dependent with a TERMINATED one (HD01="024" Cancellation/
// Termination -- the real X12 element 875 code, confirmed against X12.org's
// own official 834 examples) -- proving the maintenance-type-code detail the FHIR mapping's own
// Coverage.status translation depends on survives a real round trip.
func TestRealSchema_834_BuildAndRoundTrip_ActiveAndTerminatedMembers(t *testing.T) {
	loader, err := edi.NewX12SchemaLoader(realSchemaDir(t))
	if err != nil {
		t.Fatalf("NewX12SchemaLoader: %v", err)
	}
	spec := loader.Spec()

	txSet := loader.GetTransactionSet("834")
	if txSet == nil {
		t.Fatal("GetTransactionSet(834) returned nil")
	}
	if txSet.FunctionalIdentifierCode != "BE" {
		t.Errorf("834 FunctionalIdentifierCode = %q, want BE", txSet.FunctionalIdentifierCode)
	}
	if txSet.VersionReleaseIndustryCode != "005010X220" {
		t.Errorf("834 VersionReleaseIndustryCode = %q, want 005010X220", txSet.VersionReleaseIndustryCode)
	}

	input := builder.BuildInput{
		TransactionSet: "834",
		Interchange:    map[string]interface{}{"senderId": "ACMECORP", "receiverId": "PAYER1"},
		Header: map[string]interface{}{
			"BGN": map[string]interface{}{"transactionSetPurposeCode": "00", "referenceIdentification": "ENROLL0001", "date": "20260914"},
		},
		Loops: map[string]interface{}{
			"1000A": map[string]interface{}{"N1": map[string]interface{}{"entityIdentifierCode": "P5", "name": "ACME CORP", "identificationCodeQualifier": "FI", "identificationCode": "111223333"}},
			"1000B": map[string]interface{}{"N1": map[string]interface{}{"entityIdentifierCode": "IN", "name": "PAYER1", "identificationCodeQualifier": "PI", "identificationCode": "PAYER001"}},
			"2000": []interface{}{
				map[string]interface{}{
					"INS": map[string]interface{}{"yesNoConditionResponseCode": "Y", "individualRelationshipCode": "18", "maintenanceTypeCode": "021", "maintenanceReasonCode": "XN"},
					"REF": map[string]interface{}{"referenceIdentificationQualifier": "0F", "referenceIdentification": "SUB123"},
					"loops": map[string]interface{}{
						"2100A": map[string]interface{}{
							"NM1": map[string]interface{}{"entityIdentifierCode": "IL", "entityTypeQualifier": "1", "nameLastOrOrganizationName": "SMITH", "nameFirst": "JANE", "identificationCodeQualifier": "34", "identificationCode": "999001234"},
							"DMG": map[string]interface{}{"dateTimePeriodFormatQualifier": "D8", "birthDate": "19800101", "genderCode": "F"},
						},
						"2300": []interface{}{
							map[string]interface{}{
								"HD": map[string]interface{}{"maintenanceTypeCode": "021", "insuranceLineCode": "HLT", "planCoverageDescription": "GOLD PPO"},
								"DTP": map[string]interface{}{"dateTimeQualifier": "348", "dateTimePeriodFormatQualifier": "D8", "datePeriod": "20260101"},
							},
						},
					},
				},
				map[string]interface{}{
					"INS": map[string]interface{}{"yesNoConditionResponseCode": "N", "individualRelationshipCode": "19", "maintenanceTypeCode": "024", "maintenanceReasonCode": "XT"},
					"REF": map[string]interface{}{"referenceIdentificationQualifier": "0F", "referenceIdentification": "SUB123"},
					"loops": map[string]interface{}{
						"2100A": map[string]interface{}{
							"NM1": map[string]interface{}{"entityIdentifierCode": "IL", "entityTypeQualifier": "1", "nameLastOrOrganizationName": "SMITH", "nameFirst": "TOMMY", "identificationCodeQualifier": "34", "identificationCode": "999001234"},
							"DMG": map[string]interface{}{"dateTimePeriodFormatQualifier": "D8", "birthDate": "20100601", "genderCode": "M"},
						},
						"2300": []interface{}{
							map[string]interface{}{
								"HD":  map[string]interface{}{"maintenanceTypeCode": "024", "insuranceLineCode": "HLT", "planCoverageDescription": "GOLD PPO"},
								"DTP": map[string]interface{}{"dateTimeQualifier": "349", "dateTimePeriodFormatQualifier": "D8", "datePeriod": "20260901"},
							},
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
		t.Fatalf("re-parsing built 834: %v\nbuilt was:\n%s", err, built)
	}
	if result.TransactionSet != "834" {
		t.Errorf("TransactionSet = %q, want 834", result.TransactionSet)
	}

	loop1000A := result.Loops["1000A"].(map[string]interface{})
	if loop1000A["N1"].(map[string]interface{})["name"] != "ACME CORP" {
		t.Errorf("1000A N1 = %#v, want name=ACME CORP", loop1000A["N1"])
	}
	loop1000B := result.Loops["1000B"].(map[string]interface{})
	if loop1000B["N1"].(map[string]interface{})["entityIdentifierCode"] != "IN" {
		t.Errorf("1000B N1 = %#v, want entityIdentifierCode=IN (Insurer -- the real X12 code 834 uses, confirmed against X12.org's own official examples)", loop1000B["N1"])
	}

	members := result.Loops["2000"].([]map[string]interface{})
	if len(members) != 2 {
		t.Fatalf("expected 2 flat member instances (no HL hierarchy), got %d", len(members))
	}

	subscriber := members[0]
	if subscriber["INS"].(map[string]interface{})["individualRelationshipCode"] != "18" {
		t.Errorf("member[0] INS = %#v, want individualRelationshipCode=18 (self)", subscriber["INS"])
	}
	sub2100A := subscriber["loops"].(map[string]interface{})["2100A"].(map[string]interface{})
	if sub2100A["NM1"].(map[string]interface{})["nameFirst"] != "JANE" {
		t.Errorf("member[0] 2100A NM1 = %#v, want nameFirst=JANE", sub2100A["NM1"])
	}
	sub2300 := subscriber["loops"].(map[string]interface{})["2300"].([]map[string]interface{})
	if len(sub2300) != 1 || sub2300[0]["HD"].(map[string]interface{})["maintenanceTypeCode"] != "021" {
		t.Errorf("member[0] 2300 HD = %#v, want maintenanceTypeCode=021 (Addition -- active, real X12 element 875 code)", sub2300)
	}

	dependent := members[1]
	if dependent["INS"].(map[string]interface{})["individualRelationshipCode"] != "19" {
		t.Errorf("member[1] INS = %#v, want individualRelationshipCode=19 (child), proving members are flat siblings, not HL-nested", dependent["INS"])
	}
	dep2100A := dependent["loops"].(map[string]interface{})["2100A"].(map[string]interface{})
	if dep2100A["NM1"].(map[string]interface{})["nameFirst"] != "TOMMY" {
		t.Errorf("member[1] 2100A NM1 = %#v, want nameFirst=TOMMY", dep2100A["NM1"])
	}
	dep2300 := dependent["loops"].(map[string]interface{})["2300"].([]map[string]interface{})
	if len(dep2300) != 1 || dep2300[0]["HD"].(map[string]interface{})["maintenanceTypeCode"] != "024" {
		t.Errorf("member[1] 2300 HD = %#v, want maintenanceTypeCode=024 (Cancellation/Termination, the real X12 element 875 code -- confirmed against X12.org's own official 834 examples), genuinely distinct from the subscriber's own Addition", dep2300)
	}

	valResult := validator.Validate(spec, result)
	for _, issue := range valResult.Issues {
		if issue.Severity == "error" {
			t.Errorf("unexpected error-severity issue on a self-built, correctly-enveloped 834: %+v", issue)
		}
	}
}
