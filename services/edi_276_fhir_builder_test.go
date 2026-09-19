// ─────────────────────────────────────────────────────────────────────────────
// EDI X12 276 → FHIR mapping (EDI Phase 6, claim status request, transform-
// only scope: this engine converts X12 <-> FHIR, it never decides claim
// status facts — same "structural transform, not business logic" boundary
// 270/271/837 already established). Config here is transcribed verbatim into
// database/migrations/V251, matching every other *_fhir_builder_test.go's own
// "config here is the source of truth" discipline.
//
// FHIR target: Task (not ClaimResponse) — a claim status REQUEST is an
// administrative "please tell me the status of this claim" ask, which FHIR's
// own Task resource (a generic request-for-work-to-be-done) models directly;
// ClaimResponse is reserved for an actual adjudication response (827/835-
// shaped content this transaction set doesn't carry).
//
// Run: go test ./services/ -v -run TestEDI276FHIRBuilder
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

// edi276FixtureLoops — one payer, one clearinghouse (information receiver),
// one billing provider, a subscriber asking about her own claim, and a
// dependent asking about his own separate claim — exercising both the
// "subscriber is the patient" and "dependent is the patient" branches, same
// shape 270's own fixture already proved for eligibility.
func edi276FixtureLoops() map[string]interface{} {
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
															"DMG": map[string]interface{}{"dateTimePeriodFormatQualifier": "D8", "birthDate": "19800101", "genderCode": "F"},
															"loops": map[string]interface{}{
																"2200D": []interface{}{
																	map[string]interface{}{
																		"TRN": map[string]interface{}{"traceTypeCode": "1", "checkOrEFTTraceNumber": "REQTRACE001"},
																		"REF": map[string]interface{}{"referenceIdentificationQualifier": "EJ", "referenceIdentification": "PCN0001"},
																		"AMT": map[string]interface{}{"amountQualifierCode": "T3", "monetaryAmount": "250.00"},
																		"DTP": map[string]interface{}{"dateTimeQualifier": "472", "dateTimePeriodFormatQualifier": "D8", "date": "20260110"},
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

const derive276StepAlias = "derive_276_claim_status_requests"

// derive276ClaimStatusRequestsScript is pure structural reshaping — it never
// decides claim status facts (a 276 is an inquiry, it carries none). It
// flattens the parsed loop nesting into one row per patient (subscriber or
// dependent) so the Task fhir.build config downstream stays purely
// declarative, same division of labor 270's own derive script established.
const derive276ClaimStatusRequestsScript = `
var parsed = (input.message && input.message.parsedEDI) || input.parsedEDI || {};
var loops = parsed.loops || {};

if (parsed.transactionSet !== "276") {
  return ({ _claim_status_requests: [] });
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

    var providerLevels = arr((rcvLevel.loops || {})["2000C"]);
    for (var provIdx = 0; provIdx < providerLevels.length; provIdx++) {
      var provLevel = providerLevels[provIdx];
      var provNM1 = ((provLevel.loops || {})["2100C"] || {}).NM1 || {};
      var providerInfo = { npi: provNM1.identificationCode || "", name: provNM1.nameLastOrOrganizationName || "" };

      function pushContext(patientInfo, claim) {
        // REF has maxUse=">1" in its own segment definition, so the real
        // parser always wraps it as an array (same reason STC needs
        // first() in the 277 derive script) -- take the first occurrence
        // even when only one REF is actually present.
        var ref = first(claim.REF);
        contexts.push({
          patient_info: patientInfo,
          payer_info: payerInfo,
          provider_info: providerInfo,
          created_at: nowIso,
          request_identifier: claim.TRN ? claim.TRN.checkOrEFTTraceNumber : "",
          patient_control_number: ref.referenceIdentification || "",
        });
      }

      var subscriberLevels = arr((provLevel.loops || {})["2000D"]);
      for (var subIdx = 0; subIdx < subscriberLevels.length; subIdx++) {
        var subLevel = subscriberLevels[subIdx];
        var sub2100D = (subLevel.loops || {})["2100D"] || {};
        var subNM1 = sub2100D.NM1 || {};
        var subDMG = sub2100D.DMG || {};
        var subscriberInfo = {
          member_id: subNM1.identificationCode || "",
          first_name: subNM1.nameFirst || "",
          last_name: subNM1.nameLastOrOrganizationName || "",
          dob: subDMG.birthDate || "",
          gender_fhir: genderToFHIR(subDMG.genderCode),
        };

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
          var depDMG = dep2100E.DMG || {};
          var dependentInfo = {
            member_id: depNM1.identificationCode || subscriberInfo.member_id,
            first_name: depNM1.nameFirst || "",
            last_name: depNM1.nameLastOrOrganizationName || "",
            dob: depDMG.birthDate || "",
            gender_fhir: genderToFHIR(depDMG.genderCode),
          };
          var claim2200E = arr(dep2100E.loops ? dep2100E.loops["2200E"] : []);
          for (var d = 0; d < claim2200E.length; d++) {
            pushContext(dependentInfo, claim2200E[d]);
          }
        }
      }
    }
  }
}

return ({ _claim_status_requests: contexts });
`

func organization276BuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Organization",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirPayerOrganizations",
		"rowsPath":     "steps." + derive276StepAlias + ".step_output._claim_status_requests",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": "payer_info.payer_id", "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "organization-payer-"}},
			map[string]interface{}{"targetPath": "active", "literalValue": "true"},
			map[string]interface{}{"targetPath": "name", "sourcePath": "payer_info.name"},
		},
	}
}

func patient276BuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Patient",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirPatients",
		"rowsPath":     "steps." + derive276StepAlias + ".step_output._claim_status_requests",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": "patient_info.member_id"},
			map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/x12-member-id"},
			map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": "patient_info.member_id"},
			map[string]interface{}{"targetPath": "name[0].family", "sourcePath": "patient_info.last_name"},
			map[string]interface{}{"targetPath": "name[0].given[0]", "sourcePath": "patient_info.first_name"},
			map[string]interface{}{"targetPath": "birthDate", "sourcePath": "patient_info.dob", "transform": "x12_date_to_fhir_date"},
			map[string]interface{}{"targetPath": "gender", "sourcePath": "patient_info.gender_fhir"},
		},
	}
}

func task276BuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Task",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirTasks",
		"rowsPath":     "steps." + derive276StepAlias + ".step_output._claim_status_requests",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": "request_identifier", "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "claim-status-request-"}},
			map[string]interface{}{"targetPath": "status", "literalValue": "requested"},
			map[string]interface{}{"targetPath": "intent", "literalValue": "order"},
			map[string]interface{}{"targetPath": "code.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/task-code"},
			map[string]interface{}{"targetPath": "code.coding[0].code", "literalValue": "fulfill"},
			map[string]interface{}{"targetPath": "description", "literalValue": "Health Care Claim Status Request (X12 276)"},
			map[string]interface{}{"targetPath": "for.reference", "sourcePath": "patient_info.member_id", "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Patient/"}},
			map[string]interface{}{"targetPath": "authoredOn", "sourcePath": "created_at"},
			map[string]interface{}{"targetPath": "requester.identifier.system", "literalValue": "http://hl7.org/fhir/sid/us-npi"},
			map[string]interface{}{"targetPath": "requester.identifier.value", "sourcePath": "provider_info.npi"},
			map[string]interface{}{"targetPath": "owner.identifier.system", "literalValue": "http://ezhealthkonnect.local/x12-payer-id"},
			map[string]interface{}{"targetPath": "owner.identifier.value", "sourcePath": "payer_info.payer_id"},
			map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/x12-patient-control-number"},
			map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": "patient_control_number"},
		},
	}
}

// TC-EDI276-FB-001: full chain (derive -> Organization -> Patient -> Task ->
// payload.builder fhir_bundle -> fhir_validation strict) builds a Bundle
// with 2 patients (subscriber + dependent, proving both derive-script
// branches) and 2 Task resources (one per claim status inquiry), and passes
// strict FHIR validation with zero unexpected errors.
func TestEDI276FHIRBuilder_SubscriberAndDependent_BuildsCleanValidatingBundle(t *testing.T) {
	initFHIRRegistrySvc(t)

	data := map[string]interface{}{
		"parsedEDI": map[string]interface{}{
			"transactionSet": "276",
			"loops":          edi276FixtureLoops(),
		},
	}

	result := runScriptSvc(t, derive276StepAlias, derive276ClaimStatusRequestsScript, data)
	deriveOut := svcStepOutput(t, result, derive276StepAlias)
	for k, v := range result {
		data[k] = v
	}
	svcInjectStepOutput(data, derive276StepAlias, deriveOut)

	contexts, _ := deriveOut["_claim_status_requests"].([]interface{})
	if len(contexts) != 2 {
		t.Fatalf("expected 2 claim status contexts (subscriber's own + dependent's), got %d: %+v", len(contexts), contexts)
	}

	data = runFHIRBuild(t, "build_payer_organization_fhir", organization276BuildConfig(), data)
	data = runFHIRBuild(t, "build_patient_fhir", patient276BuildConfig(), data)
	data = runFHIRBuild(t, "build_task_fhir", task276BuildConfig(), data)

	message, ok := data["message"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data[\"message\"] to be a map, got %T", data["message"])
	}
	patients, ok := message["fhirPatients"].([]map[string]interface{})
	if !ok || len(patients) != 2 {
		t.Fatalf("expected 2 Patient resources, got %d (ok=%v)", len(patients), ok)
	}
	tasks, ok := message["fhirTasks"].([]map[string]interface{})
	if !ok || len(tasks) != 2 {
		t.Fatalf("expected 2 Task resources, got %d (ok=%v)", len(tasks), ok)
	}
	if tasks[0]["status"] != "requested" || tasks[0]["intent"] != "order" {
		t.Errorf("Task[0] = %#v, want status=requested/intent=order", tasks[0])
	}
	if id0 := tasks[0]["identifier"].([]interface{})[0].(map[string]interface{})["value"]; id0 != "PCN0001" {
		t.Errorf("Task[0] identifier value = %v, want PCN0001", id0)
	}
	if id1 := tasks[1]["identifier"].([]interface{})[0].(map[string]interface{})["value"]; id1 != "PCN0002" {
		t.Errorf("Task[1] identifier value = %v, want PCN0002 (dependent's own claim, genuinely distinct from the subscriber's)", id1)
	}

	assembleAndValidate276Bundle(t, data, "message.fhirPayerOrganizations", "message.fhirPatients", "message.fhirTasks")
}

// assembleAndValidate276Bundle mirrors assembleAndValidate270Bundle exactly —
// payload.builder fhir_bundle -> fhir_validation strict, asserting zero
// unexpected validation errors.
func assembleAndValidate276Bundle(t *testing.T, data map[string]interface{}, resourcePaths ...string) string {
	t.Helper()

	rp := make([]interface{}, len(resourcePaths))
	for i, p := range resourcePaths {
		rp[i] = p
	}

	pbExec := payload.NewPayloadBuilderExecutor(nil)
	pbStep := &models.TransformationStep{
		StepName:  "Assemble FHIR Bundle",
		StepAlias: strPtrSvc("assemble_276_bundle"),
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
	pbOut := svcStepOutput(t, pbResult, "assemble_276_bundle")
	for k, v := range pbResult {
		data[k] = v
	}
	svcInjectStepOutput(data, "assemble_276_bundle", pbOut)

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
		StepAlias: strPtrSvc("validate_276_bundle"),
		StepType:  "fhir_validation",
		Enabled:   true,
		Config: map[string]interface{}{
			"profile":          "base",
			"validation_level": "strict",
			"fhir_version":     "R4",
			"source_field":     "steps.assemble_276_bundle.step_output.payload",
		},
	}
	vResult, err := vExec.Execute(context.Background(), vStep, data)
	if err != nil {
		t.Fatalf("validate_276_bundle error: %v", err)
	}
	vOut := svcStepOutput(t, vResult, "validate_276_bundle")

	errs := asStringSlice(vOut["errors"])
	if len(errs) > 0 {
		b, _ := json.MarshalIndent(bundle, "", "  ")
		t.Errorf("unexpected validation errors:\n%s\nbundle: %s", strings.Join(errs, "\n"), b)
	}
	return bundleJSON
}
