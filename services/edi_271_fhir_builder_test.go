// ─────────────────────────────────────────────────────────────────────────────
// EDI X12 271 → FHIR mapping — Phase 3 (real-time eligibility, transform-only
// scope, same boundary as edi_270_fhir_builder_test.go's own note). Config
// here is transcribed verbatim into database/migrations/V243.
//
// Run: go test ./services/ -v -run TestEDI271FHIRBuilder
// ─────────────────────────────────────────────────────────────────────────────

package services

import (
	"testing"
)

// ─────────────────────────────────────────────────────────────────────────────
// Fixture — one subscriber getting a real active-coverage EB answer (with a
// nested 2115C benefit-additional-info III), and a dependent getting an AAA
// rejection instead (subscriber/dependent not found) — proving both the
// "complete" and "error" outcome branches on the SAME transaction, and
// exercising the genuinely 3-level nested insurance[].item[].benefit[]
// FHIR shape via fhir.build's own nested repeatingGroups mechanism.
// ─────────────────────────────────────────────────────────────────────────────

func edi271FixtureLoops() map[string]interface{} {
	return map[string]interface{}{
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
								// Two SEPARATE 2000C branches — same real-X12-usage reasoning as
								// edi_270_fhir_builder_test.go's own fixture: a subscriber loop
								// carrying a 2000D dependent answers ABOUT that dependent, never
								// about herself simultaneously.
								"2000C": []interface{}{
									// Subscriber 1: a real active-coverage answer about herself.
									map[string]interface{}{
										"HL":  map[string]interface{}{"hierarchicalIdNumber": "3", "hierarchicalParentIdNumber": "2", "hierarchicalLevelCode": "22", "hierarchicalChildCode": "0"},
										"TRN": map[string]interface{}{"traceTypeCode": "2", "checkOrEFTTraceNumber": "TRACE001"},
										"loops": map[string]interface{}{
											"2100C": map[string]interface{}{
												"NM1": map[string]interface{}{"entityIdentifierCode": "IL", "entityTypeQualifier": "1", "nameLastOrOrganizationName": "SMITH", "nameFirst": "JANE", "identificationCodeQualifier": "MI", "identificationCode": "SUB123"},
												"DMG": map[string]interface{}{"dateTimePeriodFormatQualifier": "D8", "birthDate": "19800101", "genderCode": "F"},
												"loops": map[string]interface{}{
													"2110C": []interface{}{
														map[string]interface{}{
															"EB": map[string]interface{}{"eligibilityBenefitInformationCode": "1", "coverageLevelCode": "IND", "serviceTypeCode": "30", "planCoverageDescription": "PPO GOLD", "monetaryAmount": "25.00"},
															"loops": map[string]interface{}{
																"2115C": []interface{}{
																	map[string]interface{}{"III": map[string]interface{}{"codeListQualifierCode": "ZZ", "industryCode": "REMAINING VISITS: 5"}},
																},
															},
														},
													},
												},
											},
										},
									},
									// Subscriber 2: her dependent's answer is a rejection instead.
									map[string]interface{}{
										"HL": map[string]interface{}{"hierarchicalIdNumber": "5", "hierarchicalParentIdNumber": "2", "hierarchicalLevelCode": "22", "hierarchicalChildCode": "1"},
										"loops": map[string]interface{}{
											"2100C": map[string]interface{}{
												"NM1": map[string]interface{}{"entityIdentifierCode": "IL", "entityTypeQualifier": "1", "nameLastOrOrganizationName": "DOE", "nameFirst": "MARY", "identificationCodeQualifier": "MI", "identificationCode": "SUB789"},
											},
											"2000D": []interface{}{
												map[string]interface{}{
													"HL":  map[string]interface{}{"hierarchicalIdNumber": "6", "hierarchicalParentIdNumber": "5", "hierarchicalLevelCode": "23", "hierarchicalChildCode": "0"},
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
	}
}

const derive271StepAlias = "derive_271_eligibility_contexts"

// derive271EligibilityContextsScript reshapes the parsed 271 into one row per
// patient (subscriber or dependent), pre-computing the "complete" vs "error"
// outcome directly from whether that patient's own 2100C/2100D carries an AAA
// segment — a structural fact already present in the message, never invented.
// EB->benefit mapping is likewise a pure code/value carry-through: EB01 (the
// real eligibility/benefit determination some real payer system already
// made) becomes benefit.type, EB07 becomes benefit.allowedMoney.
const derive271EligibilityContextsScript = `
var parsed = (input.message && input.message.parsedEDI) || input.parsedEDI || {};
var loops = parsed.loops || {};

if (parsed.transactionSet !== "271") {
  return ({ _coverage_contexts: [] });
}

function arr(v) {
  if (!v) return [];
  return Array.isArray(v) ? v : [v];
}

var infoSourceLevels = arr(loops["2000A"]);
var contexts = [];
var nowIso = new Date().toISOString();

for (var srcIdx = 0; srcIdx < infoSourceLevels.length; srcIdx++) {
  var srcLevel = infoSourceLevels[srcIdx];
  var srcNM1 = ((srcLevel.loops || {})["2100A"] || {}).NM1 || {};
  var payerInfo = { payer_id: srcNM1.identificationCode || "", name: srcNM1.nameLastOrOrganizationName || "" };

  var receiverLevels = arr((srcLevel.loops || {})["2000B"]);
  for (var rcvIdx = 0; rcvIdx < receiverLevels.length; rcvIdx++) {
    var rcvLevel = receiverLevels[rcvIdx];

    var subscriberLevels = arr((rcvLevel.loops || {})["2000C"]);
    for (var subIdx = 0; subIdx < subscriberLevels.length; subIdx++) {
      var subLevel = subscriberLevels[subIdx];
      var sub2100C = (subLevel.loops || {})["2100C"] || {};
      var subNM1 = sub2100C.NM1 || {};
      var subscriberInfo = {
        member_id: subNM1.identificationCode || "",
        first_name: subNM1.nameFirst || "",
        last_name: subNM1.nameLastOrOrganizationName || "",
      };

      var dependentLevels = arr((subLevel.loops || {})["2000D"]);

      // Builds the item[] rows (one per real 2110C/2110D EB occurrence) that
      // sit INSIDE the one insurance[] entry -- FHIR's own shape is ONE
      // coverage (insurance[]) carrying MULTIPLE reported benefits (item[]),
      // not one insurance entry per benefit fact.
      function buildInsuranceItemRows(eb2110List) {
        var out = [];
        var ebList = arr(eb2110List);
        for (var i = 0; i < ebList.length; i++) {
          var eb = ebList[i].EB || {};
          if (!eb.eligibilityBenefitInformationCode) continue;
          var benefits = [{
            eb_code: eb.eligibilityBenefitInformationCode,
            allowed_amount: eb.monetaryAmount || "",
            plan_description: eb.planCoverageDescription || "",
          }];
          out.push({ category_code: eb.serviceTypeCode || "", benefits: benefits });
        }
        return out;
      }

      function pushContext(patientInfo, requestId, aaaList, eb2110List) {
        var rejections = arr(aaaList);
        var isRejected = rejections.length > 0;
        var errorItems = [];
        for (var i = 0; i < rejections.length; i++) {
          errorItems.push({ reject_reason_code: rejections[i].rejectReasonCode || "" });
        }
        // "insurance" is a 0-or-1-element array (this mapping reports exactly
        // one real-world coverage per patient) so the fhir.build repeatingGroup
        // naturally produces zero insurance[] entries for a rejected patient --
        // the same "empty rowsPath array -> zero repeatingGroup rows" convention
        // 837's own diagnosis/item/careTeam repeatingGroups already rely on,
        // rather than a separate conditional mechanism.
        var insurance = isRejected ? [] : [{
          member_id: patientInfo.member_id,
          items: buildInsuranceItemRows(eb2110List),
        }];
        contexts.push({
          patient_info: patientInfo,
          payer_info: payerInfo,
          created_at: nowIso,
          request_identifier: requestId || "",
          outcome: isRejected ? "error" : "complete",
          insurance: insurance,
          error_items: errorItems,
        });
      }

      if (dependentLevels.length === 0) {
        pushContext(
          subscriberInfo,
          subLevel.TRN ? subLevel.TRN.checkOrEFTTraceNumber : "",
          sub2100C.AAA,
          sub2100C.loops ? sub2100C.loops["2110C"] : []
        );
      } else {
        for (var depIdx = 0; depIdx < dependentLevels.length; depIdx++) {
          var depLevel = dependentLevels[depIdx];
          var dep2100D = (depLevel.loops || {})["2100D"] || {};
          var depNM1 = dep2100D.NM1 || {};
          var dependentInfo = {
            member_id: depNM1.identificationCode || subscriberInfo.member_id,
            first_name: depNM1.nameFirst || "",
            last_name: depNM1.nameLastOrOrganizationName || "",
          };
          pushContext(
            dependentInfo,
            depLevel.TRN ? depLevel.TRN.checkOrEFTTraceNumber : "",
            dep2100D.AAA,
            dep2100D.loops ? dep2100D.loops["2110D"] : []
          );
        }
      }
    }
  }
}

return ({ _coverage_contexts: contexts });
`

func coverageEligibilityResponseBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "CoverageEligibilityResponse",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirCoverageEligibilityResponses",
		"rowsPath":     "steps." + derive271StepAlias + ".step_output._coverage_contexts",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": "request_identifier", "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "eligibility-response-"}},
			map[string]interface{}{"targetPath": "status", "literalValue": "active"},
			map[string]interface{}{"targetPath": "purpose[0]", "literalValue": "benefits"},
			map[string]interface{}{"targetPath": "patient.reference", "sourcePath": "patient_info.member_id", "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Patient/"}},
			map[string]interface{}{"targetPath": "created", "sourcePath": "created_at"},
			map[string]interface{}{"targetPath": "outcome", "sourcePath": "outcome"},
			// request: a logical (identifier-only) reference, same pattern 837's
			// own careTeam[].provider/Claim.facility already established -- a 271
			// processed independently has no real prior CoverageEligibilityRequest
			// resource in the same Bundle to point at.
			map[string]interface{}{"targetPath": "request.identifier.system", "literalValue": "http://ezhealthkonnect.local/x12-trace-number"},
			map[string]interface{}{"targetPath": "request.identifier.value", "sourcePath": "request_identifier"},
			map[string]interface{}{"targetPath": "insurer.identifier.system", "literalValue": "http://ezhealthkonnect.local/x12-payer-id"},
			map[string]interface{}{"targetPath": "insurer.identifier.value", "sourcePath": "payer_info.payer_id"},
		},
		"repeatingGroups": []interface{}{
			map[string]interface{}{
				"targetPath": "insurance",
				// "insurance" is pre-flattened by the derive script to a 0-or-1-
				// element array (one real-world coverage per patient, or none for a
				// rejected patient) -- an empty array here means zero insurance[]
				// entries get written, the exact same "empty rowsPath -> zero rows"
				// convention 837's own diagnosis/item/careTeam repeatingGroups
				// already rely on, rather than a separate groupCondition mechanism.
				"rowsPath": "insurance",
				"fields": []interface{}{
					map[string]interface{}{"targetPath": "coverage.identifier.system", "literalValue": "http://ezhealthkonnect.local/x12-member-id"},
					map[string]interface{}{"targetPath": "coverage.identifier.value", "sourcePath": "member_id"},
				},
				"repeatingGroups": []interface{}{
					map[string]interface{}{
						"targetPath": "item",
						"rowsPath":   "items",
						"fields": []interface{}{
							map[string]interface{}{"targetPath": "category.coding[0].system", "literalValue": "https://codesystem.x12.org/005010/1365"},
							map[string]interface{}{"targetPath": "category.coding[0].code", "sourcePath": "category_code"},
						},
						"repeatingGroups": []interface{}{
							map[string]interface{}{
								"targetPath": "benefit",
								"rowsPath":   "benefits",
								"fields": []interface{}{
									map[string]interface{}{"targetPath": "type.coding[0].system", "literalValue": "https://codesystem.x12.org/005010/1338"},
									map[string]interface{}{"targetPath": "type.coding[0].code", "sourcePath": "eb_code"},
									map[string]interface{}{"targetPath": "allowedMoney.value", "sourcePath": "allowed_amount"},
									map[string]interface{}{"targetPath": "allowedMoney.currency", "literalValue": "USD"},
								},
							},
						},
					},
				},
			},
			map[string]interface{}{
				"targetPath": "error",
				"rowsPath":   "error_items",
				"fields": []interface{}{
					map[string]interface{}{"targetPath": "code.coding[0].system", "literalValue": "https://codesystem.x12.org/005010/901"},
					map[string]interface{}{"targetPath": "code.coding[0].code", "sourcePath": "reject_reason_code"},
				},
			},
		},
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-EDI271-FB-001: full chain (derive -> Patient -> CoverageEligibilityResponse
// -> payload.builder fhir_bundle -> fhir_validation strict) builds a Bundle
// with 2 CoverageEligibilityResponse resources (subscriber outcome=complete
// with a real nested insurance[0].item[0].benefit[0], dependent
// outcome=error with a real error[0]), and passes strict FHIR validation
// with zero unexpected errors.
// ─────────────────────────────────────────────────────────────────────────────
func TestEDI271FHIRBuilder_ActiveCoverageAndRejection_BuildsCleanValidatingBundle(t *testing.T) {
	initFHIRRegistrySvc(t)

	data := map[string]interface{}{
		"parsedEDI": map[string]interface{}{
			"transactionSet": "271",
			"loops":          edi271FixtureLoops(),
		},
	}

	result := runScriptSvc(t, derive271StepAlias, derive271EligibilityContextsScript, data)
	deriveOut := svcStepOutput(t, result, derive271StepAlias)
	for k, v := range result {
		data[k] = v
	}
	svcInjectStepOutput(data, derive271StepAlias, deriveOut)

	contexts, _ := deriveOut["_coverage_contexts"].([]interface{})
	if len(contexts) != 2 {
		t.Fatalf("expected 2 coverage contexts (subscriber's own + dependent's), got %d: %+v", len(contexts), contexts)
	}
	subCtx := contexts[0].(map[string]interface{})
	if subCtx["outcome"] != "complete" {
		t.Errorf("subscriber outcome = %v, want complete", subCtx["outcome"])
	}
	depCtx := contexts[1].(map[string]interface{})
	if depCtx["outcome"] != "error" {
		t.Errorf("dependent outcome = %v, want error", depCtx["outcome"])
	}

	data = runFHIRBuild(t, "build_patient_fhir", patient271PatientBuildConfig(), data)
	data = runFHIRBuild(t, "build_coverage_eligibility_response_fhir", coverageEligibilityResponseBuildConfig(), data)

	message, ok := data["message"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data[\"message\"] to be a map, got %T", data["message"])
	}
	responses, ok := message["fhirCoverageEligibilityResponses"].([]map[string]interface{})
	if !ok || len(responses) != 2 {
		t.Fatalf("expected 2 CoverageEligibilityResponse resources, got %d (ok=%v)", len(responses), ok)
	}

	subResp := responses[0]
	if subResp["outcome"] != "complete" {
		t.Errorf("subscriber response outcome = %v, want complete", subResp["outcome"])
	}
	insurance, _ := subResp["insurance"].([]interface{})
	if len(insurance) != 1 {
		t.Fatalf("subscriber response: expected 1 insurance entry, got %d: %+v", len(insurance), subResp["insurance"])
	}
	items, _ := insurance[0].(map[string]interface{})["item"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("subscriber response: expected 1 item, got %d", len(items))
	}
	item0 := items[0].(map[string]interface{})
	if code := item0["category"].(map[string]interface{})["coding"].([]interface{})[0].(map[string]interface{})["code"]; code != "30" {
		t.Errorf("subscriber response item category code = %v, want 30", code)
	}
	benefits, _ := item0["benefit"].([]interface{})
	if len(benefits) != 1 {
		t.Fatalf("subscriber response: expected 1 benefit, got %d", len(benefits))
	}
	benefit0 := benefits[0].(map[string]interface{})
	if code := benefit0["type"].(map[string]interface{})["coding"].([]interface{})[0].(map[string]interface{})["code"]; code != "1" {
		t.Errorf("subscriber response benefit type code = %v, want 1 (active coverage)", code)
	}
	if amt := benefit0["allowedMoney"].(map[string]interface{})["value"]; amt != "25.00" {
		t.Errorf("subscriber response benefit allowedMoney = %v, want 25.00", amt)
	}

	depResp := responses[1]
	if depResp["outcome"] != "error" {
		t.Errorf("dependent response outcome = %v, want error", depResp["outcome"])
	}
	if _, hasInsurance := depResp["insurance"]; hasInsurance {
		t.Errorf("dependent response (rejected) should have NO insurance entry, got %+v", depResp["insurance"])
	}
	depErrors, _ := depResp["error"].([]interface{})
	if len(depErrors) != 1 {
		t.Fatalf("dependent response: expected 1 error entry, got %d", len(depErrors))
	}
	if code := depErrors[0].(map[string]interface{})["code"].(map[string]interface{})["coding"].([]interface{})[0].(map[string]interface{})["code"]; code != "72" {
		t.Errorf("dependent response error code = %v, want 72", code)
	}

	assembleAndValidate270Bundle(t, data, "message.fhirPatients", "message.fhirCoverageEligibilityResponses")
}

func patient271PatientBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Patient",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirPatients",
		"rowsPath":     "steps." + derive271StepAlias + ".step_output._coverage_contexts",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": "patient_info.member_id"},
			map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/x12-member-id"},
			map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": "patient_info.member_id"},
			map[string]interface{}{"targetPath": "name[0].family", "sourcePath": "patient_info.last_name"},
			map[string]interface{}{"targetPath": "name[0].given[0]", "sourcePath": "patient_info.first_name"},
		},
	}
}
