// ─────────────────────────────────────────────────────────────────────────────
// EDI X12 277 → FHIR mapping (EDI Phase 6, claim status response, transform-
// only scope: this engine converts X12 <-> FHIR, it never decides claim
// status facts — it only re-expresses whatever STC status the X12 content
// already carries in FHIR's own Task.status vocabulary, the same class of
// code-system translation 270/271 already does for gender/relationship
// codes, never inventing a status the source didn't report).
//
// FHIR target: Task (see edi_276_fhir_builder_test.go's own header comment
// for why Task, not ClaimResponse, is the right mapping target here).
//
// Run: go test ./services/ -v -run TestEDI277FHIRBuilder
// ─────────────────────────────────────────────────────────────────────────────

package services

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors/payload"
	"ezhealthkonnect/services/executors/validation"
)

// edi277FixtureLoops — one payer, one clearinghouse, one billing provider, a
// subscriber's claim coming back FINALIZED/PAID (STC category F1) and a
// dependent's claim coming back PENDING (STC category A2) — proving the
// status-code-to-Task.status translation picks genuinely different real
// statuses from genuinely different source data, not a hardcoded constant.
func edi277FixtureLoops() map[string]interface{} {
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
}

const derive277StepAlias = "derive_277_claim_status_responses"

// derive277ClaimStatusResponsesScript flattens the parsed loop nesting into
// one row per patient claim status, translating the X12 STC category code
// into FHIR's own Task.status vocabulary — a code-system translation (the
// same class of transform 270/271's own gender/relationship mapping already
// does), never an invented fact: the STATUS itself always comes straight
// from the source STC, this only re-expresses it.
const derive277ClaimStatusResponsesScript = `
var parsed = (input.message && input.message.parsedEDI) || input.parsedEDI || {};
var loops = parsed.loops || {};

if (parsed.transactionSet !== "277") {
  return ({ _claim_status_responses: [] });
}

function arr(v) {
  if (!v) return [];
  return Array.isArray(v) ? v : [v];
}
function first(v) {
  var a = arr(v);
  return a.length > 0 ? a[0] : {};
}
function genderToFHIR(code) {
  if (code === "M") return "male";
  if (code === "F") return "female";
  return "unknown";
}
function stcCategoryToTaskStatus(code) {
  var map = {
    "A1": "received", "A2": "accepted", "A3": "rejected", "A4": "rejected",
    "A6": "rejected", "A7": "rejected", "A8": "rejected",
    "F0": "completed", "F1": "completed", "F2": "completed", "F3": "completed",
    "P0": "in-progress", "P1": "in-progress", "P2": "in-progress",
    "P3": "in-progress", "P4": "in-progress", "P5": "in-progress"
  };
  return map[code] || "in-progress";
}

var infoSourceLevels = arr(loops["2000A"]);
var contexts = [];

for (var srcIdx = 0; srcIdx < infoSourceLevels.length; srcIdx++) {
  var srcLevel = infoSourceLevels[srcIdx];
  var srcNM1 = ((srcLevel.loops || {})["2100A"] || {}).NM1 || {};
  var payerInfo = { payer_id: srcNM1.identificationCode || "", name: srcNM1.nameLastOrOrganizationName || "" };

  var receiverLevels = arr((srcLevel.loops || {})["2000B"]);
  for (var rcvIdx = 0; rcvIdx < receiverLevels.length; rcvIdx++) {
    var rcvLevel = receiverLevels[rcvIdx];

    var providerLevels = arr((rcvLevel.loops || {})["2000C"]);
    for (var provIdx = 0; provIdx < providerLevels.length; provIdx++) {
      var provLevel = providerLevels[provIdx];
      var provNM1 = ((provLevel.loops || {})["2100C"] || {}).NM1 || {};
      var providerInfo = { npi: provNM1.identificationCode || "", name: provNM1.nameLastOrOrganizationName || "" };

      function pushContext(patientInfo, claim) {
        // STC and REF both have maxUse=">1" in their own segment
        // definitions, so the real parser always wraps them as arrays —
        // take the first (claim-level overall status / control number)
        // occurrence, even when only one is actually present.
        var stc = first(claim.STC).healthCareClaimStatus || {};
        var ref = first(claim.REF);
        contexts.push({
          patient_info: patientInfo,
          payer_info: payerInfo,
          provider_info: providerInfo,
          response_identifier: claim.TRN ? claim.TRN.checkOrEFTTraceNumber : "",
          payer_claim_control_number: ref.referenceIdentification || "",
          task_status: stcCategoryToTaskStatus(stc.categoryCode || ""),
          status_category_code: stc.categoryCode || "",
          status_code: stc.statusCode || "",
        });
      }

      var subscriberLevels = arr((provLevel.loops || {})["2000D"]);
      for (var subIdx = 0; subIdx < subscriberLevels.length; subIdx++) {
        var subLevel = subscriberLevels[subIdx];
        var sub2100D = (subLevel.loops || {})["2100D"] || {};
        var subNM1 = sub2100D.NM1 || {};
        var subscriberInfo = { member_id: subNM1.identificationCode || "", first_name: subNM1.nameFirst || "", last_name: subNM1.nameLastOrOrganizationName || "", gender_fhir: "unknown" };

        var claim2200D = arr(sub2100D.loops ? sub2100D.loops["2200D"] : []);
        for (var c = 0; c < claim2200D.length; c++) {
          pushContext(subscriberInfo, claim2200D[c]);
        }

        // "2000E" is a SIBLING of "2100D" within 2000D's OWN "loops" map
        // (matching Dependent's real HL parent being the Subscriber, not
        // nested inside 2100D's own loops, which only holds "2200D") —
        // read from subLevel (the 2000D instance itself), not sub2100D.
        var dependentLevels = arr(subLevel.loops ? subLevel.loops["2000E"] : []);
        for (var depIdx = 0; depIdx < dependentLevels.length; depIdx++) {
          var depLevel = dependentLevels[depIdx];
          var dep2100E = (depLevel.loops || {})["2100E"] || {};
          var depNM1 = dep2100E.NM1 || {};
          var dependentInfo = { member_id: depNM1.identificationCode || subscriberInfo.member_id, first_name: depNM1.nameFirst || "", last_name: depNM1.nameLastOrOrganizationName || "", gender_fhir: "unknown" };
          var claim2200E = arr(dep2100E.loops ? dep2100E.loops["2200E"] : []);
          for (var d = 0; d < claim2200E.length; d++) {
            pushContext(dependentInfo, claim2200E[d]);
          }
        }
      }
    }
  }
}

return ({ _claim_status_responses: contexts });
`

func organization277BuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Organization",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirPayerOrganizations",
		"rowsPath":     "steps." + derive277StepAlias + ".step_output._claim_status_responses",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": "payer_info.payer_id", "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "organization-payer-"}},
			map[string]interface{}{"targetPath": "active", "literalValue": "true"},
			map[string]interface{}{"targetPath": "name", "sourcePath": "payer_info.name"},
		},
	}
}

func patient277BuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Patient",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirPatients",
		"rowsPath":     "steps." + derive277StepAlias + ".step_output._claim_status_responses",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": "patient_info.member_id"},
			map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/x12-member-id"},
			map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": "patient_info.member_id"},
			map[string]interface{}{"targetPath": "name[0].family", "sourcePath": "patient_info.last_name"},
			map[string]interface{}{"targetPath": "name[0].given[0]", "sourcePath": "patient_info.first_name"},
			map[string]interface{}{"targetPath": "gender", "sourcePath": "patient_info.gender_fhir"},
		},
	}
}

func task277BuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Task",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirTasks",
		"rowsPath":     "steps." + derive277StepAlias + ".step_output._claim_status_responses",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": "response_identifier", "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "claim-status-response-"}},
			map[string]interface{}{"targetPath": "status", "sourcePath": "task_status"},
			map[string]interface{}{"targetPath": "intent", "literalValue": "order"},
			map[string]interface{}{"targetPath": "code.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/task-code"},
			map[string]interface{}{"targetPath": "code.coding[0].code", "literalValue": "fulfill"},
			map[string]interface{}{"targetPath": "description", "literalValue": "Health Care Claim Status Response (X12 277)"},
			map[string]interface{}{"targetPath": "for.reference", "sourcePath": "patient_info.member_id", "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Patient/"}},
			map[string]interface{}{"targetPath": "owner.identifier.system", "literalValue": "http://ezhealthkonnect.local/x12-payer-id"},
			map[string]interface{}{"targetPath": "owner.identifier.value", "sourcePath": "payer_info.payer_id"},
			map[string]interface{}{"targetPath": "businessStatus.coding[0].system", "literalValue": "https://x12.org/codes/health-care-claim-status-category-codes"},
			map[string]interface{}{"targetPath": "businessStatus.coding[0].code", "sourcePath": "status_category_code"},
			map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/x12-payer-claim-control-number"},
			map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": "payer_claim_control_number"},
		},
	}
}

// TC-EDI277-FB-001: full chain (derive -> Organization -> Patient -> Task ->
// payload.builder fhir_bundle -> fhir_validation strict) builds a Bundle
// with 2 patients, 2 Task resources with GENUINELY DIFFERENT statuses
// (subscriber's own claim: F1 -> "completed"; dependent's own claim: A2 ->
// "accepted"), proving the status-code translation isn't a hardcoded
// constant, and passes strict FHIR validation with zero unexpected errors.
func TestEDI277FHIRBuilder_FinalizedAndPendingStatus_BuildsCleanValidatingBundle(t *testing.T) {
	initFHIRRegistrySvc(t)

	data := map[string]interface{}{
		"parsedEDI": map[string]interface{}{
			"transactionSet": "277",
			"loops":          edi277FixtureLoops(),
		},
	}

	result := runScriptSvc(t, derive277StepAlias, derive277ClaimStatusResponsesScript, data)
	deriveOut := svcStepOutput(t, result, derive277StepAlias)
	for k, v := range result {
		data[k] = v
	}
	svcInjectStepOutput(data, derive277StepAlias, deriveOut)

	contexts, _ := deriveOut["_claim_status_responses"].([]interface{})
	if len(contexts) != 2 {
		t.Fatalf("expected 2 claim status response contexts, got %d: %+v", len(contexts), contexts)
	}

	data = runFHIRBuild(t, "build_payer_organization_fhir", organization277BuildConfig(), data)
	data = runFHIRBuild(t, "build_patient_fhir", patient277BuildConfig(), data)
	data = runFHIRBuild(t, "build_task_fhir", task277BuildConfig(), data)

	message, ok := data["message"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data[\"message\"] to be a map, got %T", data["message"])
	}
	tasks, ok := message["fhirTasks"].([]map[string]interface{})
	if !ok || len(tasks) != 2 {
		t.Fatalf("expected 2 Task resources, got %d (ok=%v)", len(tasks), ok)
	}
	if tasks[0]["status"] != "completed" {
		t.Errorf("Task[0] (subscriber, STC F1) status = %v, want completed", tasks[0]["status"])
	}
	if tasks[1]["status"] != "accepted" {
		t.Errorf("Task[1] (dependent, STC A2) status = %v, want accepted (genuinely distinct from the subscriber's completed)", tasks[1]["status"])
	}
	bs0 := tasks[0]["businessStatus"].(map[string]interface{})["coding"].([]interface{})[0].(map[string]interface{})
	if bs0["code"] != "F1" {
		t.Errorf("Task[0] businessStatus code = %v, want F1", bs0["code"])
	}

	assembleAndValidate277Bundle(t, data, "message.fhirPayerOrganizations", "message.fhirPatients", "message.fhirTasks")
}

// assembleAndValidate277Bundle mirrors assembleAndValidate276Bundle exactly.
func assembleAndValidate277Bundle(t *testing.T, data map[string]interface{}, resourcePaths ...string) string {
	t.Helper()

	rp := make([]interface{}, len(resourcePaths))
	for i, p := range resourcePaths {
		rp[i] = p
	}

	pbExec := payload.NewPayloadBuilderExecutor(nil)
	pbStep := &models.TransformationStep{
		StepName:  "Assemble FHIR Bundle",
		StepAlias: strPtrSvc("assemble_277_bundle"),
		StepType:  "payload.builder",
		Enabled:   true,
		Config: map[string]interface{}{
			"mode":       "fhir_bundle",
			"fhirBundle": map[string]interface{}{"bundleType": "collection", "resourcePaths": rp},
		},
	}
	pbResult, err := pbExec.Execute(context.Background(), pbStep, data)
	if err != nil {
		t.Fatalf("payload.builder error: %v", err)
	}
	pbOut := svcStepOutput(t, pbResult, "assemble_277_bundle")
	for k, v := range pbResult {
		data[k] = v
	}
	svcInjectStepOutput(data, "assemble_277_bundle", pbOut)

	bundleJSON, ok := pbOut["payload"].(string)
	if !ok {
		t.Fatalf("payload.builder did not produce a payload string: %+v", pbOut)
	}
	var bundle map[string]interface{}
	if err := json.Unmarshal([]byte(bundleJSON), &bundle); err != nil {
		t.Fatalf("failed to unmarshal bundle JSON: %v", err)
	}

	vExec := validation.NewFHIRValidationExecutor()
	vStep := &models.TransformationStep{
		StepName:  "Validate FHIR Bundle",
		StepAlias: strPtrSvc("validate_277_bundle"),
		StepType:  "fhir_validation",
		Enabled:   true,
		Config: map[string]interface{}{
			"profile":          "base",
			"validation_level": "strict",
			"fhir_version":     "R4",
			"source_field":     "steps.assemble_277_bundle.step_output.payload",
		},
	}
	vResult, err := vExec.Execute(context.Background(), vStep, data)
	if err != nil {
		t.Fatalf("validate_277_bundle error: %v", err)
	}
	vOut := svcStepOutput(t, vResult, "validate_277_bundle")

	errs := asStringSlice(vOut["errors"])
	if len(errs) > 0 {
		b, _ := json.MarshalIndent(bundle, "", "  ")
		t.Errorf("unexpected validation errors:\n%s\nbundle: %s", strings.Join(errs, "\n"), b)
	}
	return bundleJSON
}
