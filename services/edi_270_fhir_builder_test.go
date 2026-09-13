// ─────────────────────────────────────────────────────────────────────────────
// EDI X12 270 → FHIR mapping — Phase 3 (real-time eligibility, transform-only
// scope: this engine converts X12 <-> FHIR, it never decides eligibility facts
// -- same "structural transform, not business logic" boundary 837 already
// established for claims). Config here is transcribed verbatim into
// database/migrations/V242, matching every other *_fhir_builder_test.go's own
// "config here is the source of truth" discipline.
//
// Run: go test ./services/ -v -run TestEDI270FHIRBuilder
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

// ─────────────────────────────────────────────────────────────────────────────
// Fixture — one information source (payer), one information receiver
// (provider), one subscriber asking about her own coverage (service type 30 —
// Health Benefit Plan Coverage), and one dependent asking about a different
// service type (98 — Professional (Physician) Visit) — exercising both the
// "subscriber is the patient" and "dependent is the patient" branches, same
// shape 837P's own derive script already proved for claims.
// ─────────────────────────────────────────────────────────────────────────────

func edi270FixtureLoops() map[string]interface{} {
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
								// Two SEPARATE 2000C (Subscriber) hierarchical branches, matching
								// real X12 usage exactly as 837P's own two-subscriber fixture
								// does: a subscriber loop carrying a 2000D dependent asks ABOUT
								// that dependent (patient = dependent), never about herself AND
								// the dependent in the same branch — proving both derive-script
								// paths needs two distinct 2000C occurrences, not one with both
								// a direct 2110C AND a nested 2000D.
								"2000C": []interface{}{
									// Subscriber 1: asking about her own coverage (self).
									map[string]interface{}{
										"HL":  map[string]interface{}{"hierarchicalIdNumber": "3", "hierarchicalParentIdNumber": "2", "hierarchicalLevelCode": "22", "hierarchicalChildCode": "0"},
										"TRN": map[string]interface{}{"traceTypeCode": "1", "checkOrEFTTraceNumber": "TRACE001"},
										"loops": map[string]interface{}{
											"2100C": map[string]interface{}{
												"NM1": map[string]interface{}{"entityIdentifierCode": "IL", "entityTypeQualifier": "1", "nameLastOrOrganizationName": "SMITH", "nameFirst": "JANE", "identificationCodeQualifier": "MI", "identificationCode": "SUB123"},
												"DMG": map[string]interface{}{"dateTimePeriodFormatQualifier": "D8", "birthDate": "19800101", "genderCode": "F"},
												"loops": map[string]interface{}{
													"2110C": []interface{}{
														map[string]interface{}{"EQ": map[string]interface{}{"serviceTypeCode": "30"}},
													},
												},
											},
										},
									},
									// Subscriber 2: asking about her dependent's coverage instead.
									map[string]interface{}{
										"HL": map[string]interface{}{"hierarchicalIdNumber": "5", "hierarchicalParentIdNumber": "2", "hierarchicalLevelCode": "22", "hierarchicalChildCode": "1"},
										"loops": map[string]interface{}{
											"2100C": map[string]interface{}{
												"NM1": map[string]interface{}{"entityIdentifierCode": "IL", "entityTypeQualifier": "1", "nameLastOrOrganizationName": "DOE", "nameFirst": "MARY", "identificationCodeQualifier": "MI", "identificationCode": "SUB789"},
											},
											"2000D": []interface{}{
												map[string]interface{}{
													"HL":  map[string]interface{}{"hierarchicalIdNumber": "6", "hierarchicalParentIdNumber": "5", "hierarchicalLevelCode": "23", "hierarchicalChildCode": "0"},
													"TRN": map[string]interface{}{"traceTypeCode": "1", "checkOrEFTTraceNumber": "TRACE002"},
													"loops": map[string]interface{}{
														"2100D": map[string]interface{}{
															"NM1": map[string]interface{}{"entityIdentifierCode": "QC", "entityTypeQualifier": "1", "nameLastOrOrganizationName": "DOE", "nameFirst": "TOMMY", "identificationCodeQualifier": "MI", "identificationCode": "SUB456"},
															"DMG": map[string]interface{}{"dateTimePeriodFormatQualifier": "D8", "birthDate": "20100615", "genderCode": "M"},
															"INS": map[string]interface{}{"yesNoConditionResponseCode": "N", "individualRelationshipCode": "19"},
															"loops": map[string]interface{}{
																"2110D": []interface{}{
																	map[string]interface{}{"EQ": map[string]interface{}{"serviceTypeCode": "98"}},
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

const derive270StepAlias = "derive_270_eligibility_contexts"

// derive270EligibilityContextsScript is pure structural reshaping — it never
// decides eligibility facts (there are none to decide in a 270; it's an
// inquiry, not an answer). It only flattens the parsed loop nesting into one
// row per patient (subscriber or dependent) so the CoverageEligibilityRequest
// fhir.build config downstream stays purely declarative, same division of
// labor 837's own derive script already established.
const derive270EligibilityContextsScript = `
var parsed = (input.message && input.message.parsedEDI) || input.parsedEDI || {};
var loops = parsed.loops || {};

if (parsed.transactionSet !== "270") {
  return ({ _eligibility_contexts: [] });
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
function relationshipToFHIR(code) {
  var map = { "18": "self", "01": "spouse", "19": "child", "20": "employee" };
  return map[code] || "other";
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
    var rcvNM1 = ((rcvLevel.loops || {})["2100B"] || {}).NM1 || {};
    var providerInfo = { npi: rcvNM1.identificationCode || "", name: rcvNM1.nameLastOrOrganizationName || "" };

    var subscriberLevels = arr((rcvLevel.loops || {})["2000C"]);
    for (var subIdx = 0; subIdx < subscriberLevels.length; subIdx++) {
      var subLevel = subscriberLevels[subIdx];
      var sub2100C = (subLevel.loops || {})["2100C"] || {};
      var subNM1 = sub2100C.NM1 || {};
      var subDMG = sub2100C.DMG || {};
      var subscriberInfo = {
        member_id: subNM1.identificationCode || "",
        first_name: subNM1.nameFirst || "",
        last_name: subNM1.nameLastOrOrganizationName || "",
        dob: subDMG.birthDate || "",
        gender_fhir: genderToFHIR(subDMG.genderCode),
      };

      var dependentLevels = arr((subLevel.loops || {})["2000D"]);

      function pushContext(patientInfo, relationshipFHIR, eqLoops) {
        var items = [];
        var eqList = arr(eqLoops);
        for (var i = 0; i < eqList.length; i++) {
          var eq = eqList[i].EQ || {};
          if (!eq.serviceTypeCode) continue;
          items.push({ category_code: eq.serviceTypeCode });
        }
        contexts.push({
          patient_info: patientInfo,
          subscriber_info: subscriberInfo,
          payer_info: payerInfo,
          provider_info: providerInfo,
          relationship_fhir: relationshipFHIR,
          created_at: nowIso,
          request_identifier: subLevel.TRN ? subLevel.TRN.checkOrEFTTraceNumber : "",
          items: items,
        });
      }

      if (dependentLevels.length === 0) {
        pushContext(subscriberInfo, "self", sub2100C.loops ? sub2100C.loops["2110C"] : []);
      } else {
        for (var depIdx = 0; depIdx < dependentLevels.length; depIdx++) {
          var depLevel = dependentLevels[depIdx];
          var dep2100D = (depLevel.loops || {})["2100D"] || {};
          var depNM1 = dep2100D.NM1 || {};
          var depDMG = dep2100D.DMG || {};
          var depINS = dep2100D.INS || {};
          var dependentInfo = {
            member_id: depNM1.identificationCode || subscriberInfo.member_id,
            first_name: depNM1.nameFirst || "",
            last_name: depNM1.nameLastOrOrganizationName || "",
            dob: depDMG.birthDate || "",
            gender_fhir: genderToFHIR(depDMG.genderCode),
          };
          pushContext(dependentInfo, relationshipToFHIR(depINS.individualRelationshipCode), dep2100D.loops ? dep2100D.loops["2110D"] : []);
        }
      }
    }
  }
}

return ({ _eligibility_contexts: contexts });
`

func organization270BuildConfig(role string, outputField string) map[string]interface{} {
	sourcePath := "payer_info"
	prefix := "organization-payer-"
	if role == "provider" {
		sourcePath = "provider_info"
		prefix = "organization-provider-"
	}
	idKey := "payer_id"
	nameKey := "name"
	if role == "provider" {
		idKey = "npi"
	}
	return map[string]interface{}{
		"resourceType": "Organization",
		"profile":      "base",
		"version":      "R4",
		"outputField":  outputField,
		"rowsPath":     "steps." + derive270StepAlias + ".step_output._eligibility_contexts",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": sourcePath + "." + idKey, "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": prefix}},
			map[string]interface{}{"targetPath": "active", "literalValue": "true"},
			map[string]interface{}{"targetPath": "name", "sourcePath": sourcePath + "." + nameKey},
		},
	}
}

func patient270BuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Patient",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirPatients",
		"rowsPath":     "steps." + derive270StepAlias + ".step_output._eligibility_contexts",
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

func coverageEligibilityRequestBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "CoverageEligibilityRequest",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirCoverageEligibilityRequests",
		"rowsPath":     "steps." + derive270StepAlias + ".step_output._eligibility_contexts",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": "request_identifier", "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "eligibility-request-"}},
			map[string]interface{}{"targetPath": "status", "literalValue": "active"},
			map[string]interface{}{"targetPath": "purpose[0]", "literalValue": "benefits"},
			map[string]interface{}{"targetPath": "patient.reference", "sourcePath": "patient_info.member_id", "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Patient/"}},
			map[string]interface{}{"targetPath": "created", "sourcePath": "created_at"},
			map[string]interface{}{"targetPath": "provider.identifier.system", "literalValue": "http://hl7.org/fhir/sid/us-npi"},
			map[string]interface{}{"targetPath": "provider.identifier.value", "sourcePath": "provider_info.npi"},
			map[string]interface{}{"targetPath": "insurer.identifier.system", "literalValue": "http://ezhealthkonnect.local/x12-payer-id"},
			map[string]interface{}{"targetPath": "insurer.identifier.value", "sourcePath": "payer_info.payer_id"},
		},
		"repeatingGroups": []interface{}{
			map[string]interface{}{
				"targetPath": "item",
				"rowsPath":   "items",
				"fields": []interface{}{
					map[string]interface{}{"targetPath": "category.coding[0].system", "literalValue": "https://codesystem.x12.org/005010/1365"},
					map[string]interface{}{"targetPath": "category.coding[0].code", "sourcePath": "category_code"},
				},
			},
		},
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-EDI270-FB-001: full chain (derive -> Organization x2 -> Patient ->
// CoverageEligibilityRequest -> payload.builder fhir_bundle -> fhir_validation
// strict) builds a Bundle with 2 patients (subscriber + dependent, proving
// both derive-script branches), 1 payer Organization, 1 provider Organization,
// 2 CoverageEligibilityRequest resources (one per patient, each with its own
// item[] from that patient's own EQ inquiries), and passes strict FHIR
// validation with zero unexpected errors.
// ─────────────────────────────────────────────────────────────────────────────
func TestEDI270FHIRBuilder_SubscriberAndDependent_BuildsCleanValidatingBundle(t *testing.T) {
	initFHIRRegistrySvc(t)

	data := map[string]interface{}{
		"parsedEDI": map[string]interface{}{
			"transactionSet": "270",
			"loops":          edi270FixtureLoops(),
		},
	}

	result := runScriptSvc(t, derive270StepAlias, derive270EligibilityContextsScript, data)
	deriveOut := svcStepOutput(t, result, derive270StepAlias)
	for k, v := range result {
		data[k] = v
	}
	svcInjectStepOutput(data, derive270StepAlias, deriveOut)

	contexts, _ := deriveOut["_eligibility_contexts"].([]interface{})
	if len(contexts) != 2 {
		t.Fatalf("expected 2 eligibility contexts (subscriber's own + dependent's), got %d: %+v", len(contexts), contexts)
	}

	data = runFHIRBuild(t, "build_payer_organization_fhir", organization270BuildConfig("payer", "message.fhirPayerOrganizations"), data)
	data = runFHIRBuild(t, "build_provider_organization_fhir", organization270BuildConfig("provider", "message.fhirProviderOrganizations"), data)
	data = runFHIRBuild(t, "build_patient_fhir", patient270BuildConfig(), data)
	data = runFHIRBuild(t, "build_coverage_eligibility_request_fhir", coverageEligibilityRequestBuildConfig(), data)

	message, ok := data["message"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data[\"message\"] to be a map, got %T", data["message"])
	}
	patients, ok := message["fhirPatients"].([]map[string]interface{})
	if !ok || len(patients) != 2 {
		t.Fatalf("expected 2 Patient resources, got %d (ok=%v)", len(patients), ok)
	}
	requests, ok := message["fhirCoverageEligibilityRequests"].([]map[string]interface{})
	if !ok || len(requests) != 2 {
		t.Fatalf("expected 2 CoverageEligibilityRequest resources, got %d (ok=%v)", len(requests), ok)
	}
	subReq := requests[0]
	items, _ := subReq["item"].([]interface{})
	if len(items) != 1 {
		t.Fatalf("subscriber's own request: expected 1 item, got %d", len(items))
	}
	if code := items[0].(map[string]interface{})["category"].(map[string]interface{})["coding"].([]interface{})[0].(map[string]interface{})["code"]; code != "30" {
		t.Errorf("subscriber's own request item category code = %v, want 30", code)
	}
	depReq := requests[1]
	depItems, _ := depReq["item"].([]interface{})
	if len(depItems) != 1 {
		t.Fatalf("dependent's request: expected 1 item, got %d", len(depItems))
	}
	if code := depItems[0].(map[string]interface{})["category"].(map[string]interface{})["coding"].([]interface{})[0].(map[string]interface{})["code"]; code != "98" {
		t.Errorf("dependent's request item category code = %v, want 98", code)
	}

	payerOrgs, ok := message["fhirPayerOrganizations"].([]map[string]interface{})
	if !ok || len(payerOrgs) != 2 {
		// One payer Organization row is built PER eligibility context (2 contexts,
		// same payer) -- rowsPath is per-patient, not deduplicated across
		// patients sharing the same real-world payer, same "not deduplicated"
		// named simplification 837's own Patient/Coverage building already
		// documents (harmless per FHIR Bundle semantics: same stable id, so a
		// receiver naturally treats the two entries as the same resource).
		t.Fatalf("expected 2 payer Organization rows (one per eligibility context), got %d (ok=%v)", len(payerOrgs), ok)
	}

	assembleAndValidate270Bundle(t, data,
		"message.fhirPayerOrganizations", "message.fhirProviderOrganizations",
		"message.fhirPatients", "message.fhirCoverageEligibilityRequests")
}

// assembleAndValidate270Bundle mirrors assembleAndValidate837Bundle exactly —
// payload.builder fhir_bundle -> fhir_validation strict, asserting zero
// unexpected validation errors.
func assembleAndValidate270Bundle(t *testing.T, data map[string]interface{}, resourcePaths ...string) string {
	t.Helper()

	rp := make([]interface{}, len(resourcePaths))
	for i, p := range resourcePaths {
		rp[i] = p
	}

	pbExec := payload.NewPayloadBuilderExecutor(nil)
	pbStep := &models.TransformationStep{
		StepName:  "Assemble FHIR Bundle",
		StepAlias: strPtrSvc("assemble_270_bundle"),
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
	pbOut := svcStepOutput(t, pbResult, "assemble_270_bundle")
	for k, v := range pbResult {
		data[k] = v
	}
	svcInjectStepOutput(data, "assemble_270_bundle", pbOut)

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
		StepAlias: strPtrSvc("validate_270_bundle"),
		StepType:  "fhir_validation",
		Enabled:   true,
		Config: map[string]interface{}{
			"profile":          "base",
			"validation_level": "strict",
			"fhir_version":     "R4",
			"source_field":     "steps.assemble_270_bundle.step_output.payload",
		},
	}
	vResult, err := vExec.Execute(context.Background(), vStep, data)
	if err != nil {
		t.Fatalf("validate_270_bundle error: %v", err)
	}
	vOut := svcStepOutput(t, vResult, "validate_270_bundle")

	errs := asStringSlice(vOut["errors"])
	if len(errs) > 0 {
		b, _ := json.MarshalIndent(bundle, "", "  ")
		t.Errorf("unexpected validation errors:\n%s\nbundle: %s", strings.Join(errs, "\n"), b)
	}
	return bundleJSON
}
