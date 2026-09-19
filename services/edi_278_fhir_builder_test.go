// ─────────────────────────────────────────────────────────────────────────────
// EDI X12 278 → FHIR mapping (EDI Phase 7, Health Care Services Review
// Request/Response — prior authorization). Unlike 276/277 (mapped to Task,
// since a claim status inquiry carries no clinical/adjudication content),
// 278 maps to Claim (use="preauthorization", always built) / ClaimResponse
// (built only when HCR — the real certification decision — is present),
// matching the already-shipped Da Vinci PAS work's own resource choice for
// the same real-world business process (see 278.json's own _sourceRefs for
// the full "PAS vs 278 — different wire formats, same business process"
// rationale). This engine converts X12 <-> FHIR, it never decides a prior
// authorization outcome — HCR01's own action code is only TRANSLATED into
// FHIR's outcome/disposition vocabulary, never invented.
//
// Run: go test ./services/ -v -run TestEDI278FHIRBuilder
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

// edi278FixtureLoops — one UMO, one requester (provider), a subscriber whose
// OWN patient event already carries a real certification decision (HCR
// present — RESPONSE), and a SEPARATE dependent whose own patient event is
// still a pure REQUEST (no HCR yet) — proving both the always-built Claim
// path and the conditionally-built ClaimResponse path from ONE fixture, the
// same "prove both branches in one document" technique 271's own AAA/
// rejection fixture already established.
func edi278FixtureLoops() map[string]interface{} {
	subscriberEvent := map[string]interface{}{
		"HL":  map[string]interface{}{"hierarchicalIdNumber": "7", "hierarchicalParentIdNumber": "3", "hierarchicalLevelCode": "EV", "hierarchicalChildCode": "1"},
		"TRN": map[string]interface{}{"traceTypeCode": "1", "checkOrEFTTraceNumber": "EVENTTRACE001"},
		"UM":  map[string]interface{}{"requestCategoryCode": "HS", "certificationTypeCode": "I", "serviceTypeCode": "1"},
		"HCR": map[string]interface{}{"actionCode": "A1", "certificationNumber": "AUTH99001"},
		"HI": map[string]interface{}{
			"codes": []interface{}{
				map[string]interface{}{"code": map[string]interface{}{"qualifier": "ABK", "code": "M25.561"}},
			},
		},
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

	dependentEvent := map[string]interface{}{
		"HL":  map[string]interface{}{"hierarchicalIdNumber": "5", "hierarchicalParentIdNumber": "4", "hierarchicalLevelCode": "EV", "hierarchicalChildCode": "1"},
		"TRN": map[string]interface{}{"traceTypeCode": "1", "checkOrEFTTraceNumber": "EVENTTRACE002"},
		"UM":  map[string]interface{}{"requestCategoryCode": "HS", "certificationTypeCode": "I", "serviceTypeCode": "1"},
		"HI": map[string]interface{}{
			"codes": []interface{}{
				map[string]interface{}{"code": map[string]interface{}{"qualifier": "ABK", "code": "J45.909"}},
			},
		},
		"loops": map[string]interface{}{
			"2000F": []interface{}{
				map[string]interface{}{
					"HL":  map[string]interface{}{"hierarchicalIdNumber": "6", "hierarchicalParentIdNumber": "5", "hierarchicalLevelCode": "SS", "hierarchicalChildCode": "0"},
					"TRN": map[string]interface{}{"traceTypeCode": "1", "checkOrEFTTraceNumber": "SVCTRACE002"},
					"SV1": map[string]interface{}{"procedureCode": map[string]interface{}{"qualifier": "HC", "code": "90471"}},
				},
			},
		},
	}

	return map[string]interface{}{
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
											"2000D": []interface{}{
												map[string]interface{}{
													"HL": map[string]interface{}{"hierarchicalIdNumber": "4", "hierarchicalParentIdNumber": "3", "hierarchicalLevelCode": "23", "hierarchicalChildCode": "1"},
													"loops": map[string]interface{}{
														"2010D": map[string]interface{}{"NM1": map[string]interface{}{"entityIdentifierCode": "QC", "entityTypeQualifier": "1", "nameLastOrOrganizationName": "SMITH", "nameFirst": "TOMMY", "identificationCodeQualifier": "MI", "identificationCode": "SUB123"}},
														"2000E": dependentEvent,
													},
												},
											},
											"2000E": subscriberEvent,
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

const derive278StepAlias = "derive_278_prior_auth_contexts"

// derive278PriorAuthContextsScript is pure structural reshaping -- it never
// decides a prior authorization outcome. Flattens 278's UMO/requester/
// subscriber/dependent/patient-event hierarchy into one row per patient
// event, computing is_response purely from HCR's own presence (a structural
// fact already in the message), the same "never invented" boundary 271's own
// AAA-based rejection detection already established.
const derive278PriorAuthContextsScript = `
var parsed = (input.message && input.message.parsedEDI) || input.parsedEDI || {};
var loops = parsed.loops || {};

if (parsed.transactionSet !== "278") {
  return ({ _prior_auth_contexts: [], _response_contexts: [] });
}

function arr(v) {
  if (!v) return [];
  return Array.isArray(v) ? v : [v];
}
function genderToFHIR(code) {
  if (code === "M") return "male";
  if (code === "F") return "female";
  return "unknown";
}
function hcrActionToOutcome(code) {
  var map = { "A1": "complete", "A2": "partial", "A3": "complete", "A4": "queued", "A5": "complete", "A6": "complete" };
  return map[code] || "complete";
}
function hcrActionToDisposition(code) {
  var map = {
    "A1": "Certified in total", "A2": "Certified - partial", "A3": "Not Certified",
    "A4": "Pended", "A5": "Upheld", "A6": "Modified"
  };
  return map[code] || "";
}

var umoLevels = arr(loops["2000A"]);
var contexts = [];
var nowIso = new Date().toISOString();

for (var umoIdx = 0; umoIdx < umoLevels.length; umoIdx++) {
  var umoLevel = umoLevels[umoIdx];
  var umoNM1 = ((umoLevel.loops || {})["2010A"] || {}).NM1 || {};
  var umoInfo = { umo_id: umoNM1.identificationCode || "", name: umoNM1.nameLastOrOrganizationName || "" };

  var requesterLevels = arr((umoLevel.loops || {})["2000B"]);
  for (var reqIdx = 0; reqIdx < requesterLevels.length; reqIdx++) {
    var reqLevel = requesterLevels[reqIdx];
    var reqNM1 = (arr((reqLevel.loops || {})["2010B"])[0] || {}).NM1 || {};
    var providerInfo = { npi: reqNM1.identificationCode || "", name: reqNM1.nameLastOrOrganizationName || "" };

    var subscriberLevels = arr((reqLevel.loops || {})["2000C"]);
    for (var subIdx = 0; subIdx < subscriberLevels.length; subIdx++) {
      var subLevel = subscriberLevels[subIdx];
      var sub2010C = (subLevel.loops || {})["2010C"] || {};
      var subNM1 = sub2010C.NM1 || {};
      var subscriberInfo = {
        member_id: subNM1.identificationCode || "",
        first_name: subNM1.nameFirst || "",
        last_name: subNM1.nameLastOrOrganizationName || "",
        gender_fhir: "unknown",
      };

      function extractDiagnoses(event) {
        var out = [];
        var hi = event.HI || {};
        var codes = arr(hi.codes);
        for (var i = 0; i < codes.length; i++) {
          var c = (codes[i] || {}).code || {};
          if (c.code) out.push({ system: "http://hl7.org/fhir/sid/icd-10-cm", code: c.code });
        }
        return out;
      }

      function extractServiceLines(event) {
        var out = [];
        var svcLevels = arr((event.loops || {})["2000F"]);
        for (var i = 0; i < svcLevels.length; i++) {
          var svc = svcLevels[i];
          var sv1 = svc.SV1 || {};
          var proc = sv1.procedureCode || {};
          if (!proc.code) continue;
          out.push({
            code: proc.code,
            hcr_action_code: svc.HCR ? svc.HCR.actionCode : "",
          });
        }
        return out;
      }

      function pushContext(patientInfo, event) {
        var isResponse = !!event.HCR;
        var hcr = event.HCR || {};
        contexts.push({
          patient_info: patientInfo,
          payer_info: umoInfo,
          provider_info: providerInfo,
          created_at: nowIso,
          request_identifier: event.TRN ? event.TRN.checkOrEFTTraceNumber : "",
          diagnoses: extractDiagnoses(event),
          service_lines: extractServiceLines(event),
          is_response: isResponse,
          hcr_action_code: hcr.actionCode || "",
          hcr_certification_number: hcr.certificationNumber || "",
          claim_outcome: isResponse ? hcrActionToOutcome(hcr.actionCode) : "",
          claim_disposition: isResponse ? hcrActionToDisposition(hcr.actionCode) : "",
        });
      }

      var dependentLevels = arr((subLevel.loops || {})["2000D"]);
      for (var depIdx = 0; depIdx < dependentLevels.length; depIdx++) {
        var depLevel = dependentLevels[depIdx];
        var dep2010D = (depLevel.loops || {})["2010D"] || {};
        var depNM1 = dep2010D.NM1 || {};
        var dependentInfo = {
          member_id: depNM1.identificationCode || subscriberInfo.member_id,
          first_name: depNM1.nameFirst || "",
          last_name: depNM1.nameLastOrOrganizationName || "",
          gender_fhir: "unknown",
        };
        var depEvents = arr(depLevel.loops ? depLevel.loops["2000E"] : []);
        for (var e = 0; e < depEvents.length; e++) {
          pushContext(dependentInfo, depEvents[e]);
        }
      }

      // The subscriber's OWN patient event is a SIBLING of 2000D within
      // 2000C's own loops (matching the "Dependent's real HL parent is the
      // Subscriber, never the Provider" rule already learned twice for
      // 276/277 -- 278's own tree has the SAME shape one level deeper).
      var subEvents = arr(subLevel.loops ? subLevel.loops["2000E"] : []);
      for (var se = 0; se < subEvents.length; se++) {
        pushContext(subscriberInfo, subEvents[se]);
      }
    }
  }
}

// fhir.build has no whole-resource-row "condition" gate (Condition only
// exists on individual FIELDS/repeatingGroups, never on the top-level
// config) -- so the ONLY-when-HCR-present ClaimResponse gating must happen
// HERE, as a pre-filtered second array, the same "0-or-1-element array so a
// resource simply isn't built for excluded rows" convention 271's own
// insurance/rejection derive script already established.
var responseContexts = [];
for (var i = 0; i < contexts.length; i++) {
  if (contexts[i].is_response) responseContexts.push(contexts[i]);
}

return ({ _prior_auth_contexts: contexts, _response_contexts: responseContexts });
`

func organization278Config() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Organization",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirOrganization",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "literalValue": "organization-umo"},
			map[string]interface{}{"targetPath": "active", "literalValue": "true"},
			map[string]interface{}{"targetPath": "name", "literalValue": "ACME UMO"},
		},
	}
}

func patient278Config() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Patient",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirPatients",
		"rowsPath":     "steps." + derive278StepAlias + ".step_output._prior_auth_contexts",
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

func claim278Config() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Claim",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirClaims",
		"rowsPath":     "steps." + derive278StepAlias + ".step_output._prior_auth_contexts",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": "request_identifier", "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "prior-auth-request-"}},
			map[string]interface{}{"targetPath": "status", "literalValue": "active"},
			map[string]interface{}{"targetPath": "type.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/claim-type"},
			map[string]interface{}{"targetPath": "type.coding[0].code", "literalValue": "professional"},
			map[string]interface{}{"targetPath": "use", "literalValue": "preauthorization"},
			map[string]interface{}{"targetPath": "patient.reference", "sourcePath": "patient_info.member_id", "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Patient/"}},
			map[string]interface{}{"targetPath": "created", "sourcePath": "created_at"},
			map[string]interface{}{"targetPath": "provider.identifier.system", "literalValue": "http://hl7.org/fhir/sid/us-npi"},
			map[string]interface{}{"targetPath": "provider.identifier.value", "sourcePath": "provider_info.npi"},
			map[string]interface{}{"targetPath": "priority.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/processpriority"},
			map[string]interface{}{"targetPath": "priority.coding[0].code", "literalValue": "normal"},
			map[string]interface{}{"targetPath": "insurance[0].sequence", "literalValue": "1"},
			map[string]interface{}{"targetPath": "insurance[0].focal", "literalValue": "true"},
			map[string]interface{}{"targetPath": "insurance[0].coverage.identifier.system", "literalValue": "http://ezhealthkonnect.local/x12-umo-id"},
			map[string]interface{}{"targetPath": "insurance[0].coverage.identifier.value", "sourcePath": "payer_info.umo_id"},
		},
		"repeatingGroups": []interface{}{
			map[string]interface{}{
				"rowsPath":   "diagnoses",
				"targetPath": "diagnosis",
				"fields": []interface{}{
					map[string]interface{}{"targetPath": "sequence", "sourcePath": "_rowIndex", "transform": "cda_decimal_string_to_number"},
					map[string]interface{}{"targetPath": "diagnosisCodeableConcept.coding[0].system", "sourcePath": "system"},
					map[string]interface{}{"targetPath": "diagnosisCodeableConcept.coding[0].code", "sourcePath": "code"},
				},
			},
			map[string]interface{}{
				"rowsPath":   "service_lines",
				"targetPath": "item",
				"fields": []interface{}{
					map[string]interface{}{"targetPath": "sequence", "sourcePath": "_rowIndex", "transform": "cda_decimal_string_to_number"},
					map[string]interface{}{"targetPath": "productOrService.coding[0].system", "literalValue": "http://www.ama-assn.org/go/cpt"},
					map[string]interface{}{"targetPath": "productOrService.coding[0].code", "sourcePath": "code"},
				},
			},
		},
	}
}

func claimResponse278Config() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "ClaimResponse",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirClaimResponses",
		"rowsPath":     "steps." + derive278StepAlias + ".step_output._response_contexts",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": "request_identifier", "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "prior-auth-response-"}},
			map[string]interface{}{"targetPath": "status", "literalValue": "active"},
			map[string]interface{}{"targetPath": "type.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/claim-type"},
			map[string]interface{}{"targetPath": "type.coding[0].code", "literalValue": "professional"},
			map[string]interface{}{"targetPath": "use", "literalValue": "preauthorization"},
			map[string]interface{}{"targetPath": "patient.reference", "sourcePath": "patient_info.member_id", "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Patient/"}},
			map[string]interface{}{"targetPath": "created", "sourcePath": "created_at"},
			map[string]interface{}{"targetPath": "insurer.identifier.system", "literalValue": "http://ezhealthkonnect.local/x12-umo-id"},
			map[string]interface{}{"targetPath": "insurer.identifier.value", "sourcePath": "payer_info.umo_id"},
			map[string]interface{}{"targetPath": "outcome", "sourcePath": "claim_outcome"},
			map[string]interface{}{"targetPath": "disposition", "sourcePath": "claim_disposition"},
			map[string]interface{}{"targetPath": "preAuthRef", "sourcePath": "hcr_certification_number"},
			map[string]interface{}{"targetPath": "request.identifier.system", "literalValue": "http://ezhealthkonnect.local/x12-prior-auth-trace"},
			map[string]interface{}{"targetPath": "request.identifier.value", "sourcePath": "request_identifier"},
		},
	}
}

// TC-EDI278-FB-001: full chain (derive -> Organization -> Patient -> Claim ->
// ClaimResponse -> payload.builder fhir_bundle -> fhir_validation strict)
// proves 2 patients (subscriber + dependent), 2 Claims (always built), but
// exactly 1 ClaimResponse (only the subscriber's own event carries HCR) --
// the "empty rowsPath row for a request-only context produces no
// ClaimResponse" gate working correctly -- and passes strict FHIR
// validation with zero unexpected errors.
func TestEDI278FHIRBuilder_RequestAndResponse_BuildsCleanValidatingBundle(t *testing.T) {
	initFHIRRegistrySvc(t)

	data := map[string]interface{}{
		"parsedEDI": map[string]interface{}{
			"transactionSet": "278",
			"loops":          edi278FixtureLoops(),
		},
	}

	result := runScriptSvc(t, derive278StepAlias, derive278PriorAuthContextsScript, data)
	deriveOut := svcStepOutput(t, result, derive278StepAlias)
	for k, v := range result {
		data[k] = v
	}
	svcInjectStepOutput(data, derive278StepAlias, deriveOut)

	contexts, _ := deriveOut["_prior_auth_contexts"].([]interface{})
	if len(contexts) != 2 {
		t.Fatalf("expected 2 prior auth contexts (subscriber's own response + dependent's own request), got %d: %+v", len(contexts), contexts)
	}

	data = runFHIRBuild(t, "build_umo_organization_fhir", organization278Config(), data)
	data = runFHIRBuild(t, "build_patient_fhir", patient278Config(), data)
	data = runFHIRBuild(t, "build_claim_fhir", claim278Config(), data)
	data = runFHIRBuild(t, "build_claim_response_fhir", claimResponse278Config(), data)

	message, ok := data["message"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data[\"message\"] to be a map, got %T", data["message"])
	}
	patients, ok := message["fhirPatients"].([]map[string]interface{})
	if !ok || len(patients) != 2 {
		t.Fatalf("expected 2 Patient resources, got %d (ok=%v)", len(patients), ok)
	}
	claims, ok := message["fhirClaims"].([]map[string]interface{})
	if !ok || len(claims) != 2 {
		t.Fatalf("expected 2 Claim resources (Claim is ALWAYS built, request or response), got %d (ok=%v)", len(claims), ok)
	}
	for _, c := range claims {
		if c["use"] != "preauthorization" {
			t.Errorf("Claim.use = %v, want preauthorization", c["use"])
		}
	}
	claimResponses, ok := message["fhirClaimResponses"].([]map[string]interface{})
	if !ok || len(claimResponses) != 1 {
		t.Fatalf("expected EXACTLY 1 ClaimResponse (only the subscriber's own event carries HCR -- the dependent's own request has none), got %d (ok=%v): %+v", len(claimResponses), ok, claimResponses)
	}
	if claimResponses[0]["outcome"] != "complete" {
		t.Errorf("ClaimResponse.outcome = %v, want complete (HCR01=A1 Certified in total)", claimResponses[0]["outcome"])
	}
	if claimResponses[0]["preAuthRef"] != "AUTH99001" {
		t.Errorf("ClaimResponse.preAuthRef = %v, want AUTH99001", claimResponses[0]["preAuthRef"])
	}
	if claimResponses[0]["disposition"] != "Certified in total" {
		t.Errorf("ClaimResponse.disposition = %v, want 'Certified in total'", claimResponses[0]["disposition"])
	}

	assembleAndValidate278Bundle(t, data, "message.fhirOrganization", "message.fhirPatients", "message.fhirClaims", "message.fhirClaimResponses")
}

// assembleAndValidate278Bundle mirrors assembleAndValidate276Bundle exactly.
func assembleAndValidate278Bundle(t *testing.T, data map[string]interface{}, resourcePaths ...string) string {
	t.Helper()

	rp := make([]interface{}, len(resourcePaths))
	for i, p := range resourcePaths {
		rp[i] = p
	}

	pbExec := payload.NewPayloadBuilderExecutor(nil)
	pbStep := &models.TransformationStep{
		StepName:  "Assemble FHIR Bundle",
		StepAlias: strPtrSvc("assemble_278_bundle"),
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
	pbOut := svcStepOutput(t, pbResult, "assemble_278_bundle")
	for k, v := range pbResult {
		data[k] = v
	}
	svcInjectStepOutput(data, "assemble_278_bundle", pbOut)

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
		StepAlias: strPtrSvc("validate_278_bundle"),
		StepType:  "fhir_validation",
		Enabled:   true,
		Config: map[string]interface{}{
			"profile":          "base",
			"validation_level": "strict",
			"fhir_version":     "R4",
			"source_field":     "steps.assemble_278_bundle.step_output.payload",
		},
	}
	vResult, err := vExec.Execute(context.Background(), vStep, data)
	if err != nil {
		t.Fatalf("validate_278_bundle error: %v", err)
	}
	vOut := svcStepOutput(t, vResult, "validate_278_bundle")

	errs := asStringSlice(vOut["errors"])
	if len(errs) > 0 {
		b, _ := json.MarshalIndent(bundle, "", "  ")
		t.Errorf("unexpected validation errors:\n%s\nbundle: %s", strings.Join(errs, "\n"), b)
	}
	return bundleJSON
}
