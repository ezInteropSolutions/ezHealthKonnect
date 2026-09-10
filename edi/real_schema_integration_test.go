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
			"HI": map[string]interface{}{
				"codes": []interface{}{
					map[string]interface{}{"code": map[string]interface{}{"qualifier": "ABK", "code": "R51"}},
				},
			},
			"loops": map[string]interface{}{
				"2400": []interface{}{
					map[string]interface{}{
						"LX": map[string]interface{}{"assignedNumber": "1"},
						"SV1": map[string]interface{}{
							"procedureCode": map[string]interface{}{"qualifier": "HC", "code": "99213"},
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
				"transactionSetCreationDate": "20260115", "transactionSetCreationTime": "1200",
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
	subHI := subscriberClaims[0]["HI"].(map[string]interface{})["codes"].([]map[string]interface{})
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
				"transactionSetCreationDate": "20260115", "transactionSetCreationTime": "1200",
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
											"HI": map[string]interface{}{
												"codes": []interface{}{
													map[string]interface{}{"code": map[string]interface{}{"qualifier": "ABK", "code": "I219"}},
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
