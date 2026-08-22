package enrichment

// ─────────────────────────────────────────────────────────────────────────────
// Da Vinci PAS Script Step Tests
//
// V212 replaced Zone 2's four hand-written resource-building scripts
// (build_patient/build_coverage/build_provider/assemble_pas_bundle) with the
// codebase's generic fhir.build/payload.builder engine — see
// services/pas_fhir_builder_test.go (proves the replacement chain) and
// services/executors/enrichment/pas_integration_test.go's runPASZone2
// (exercises it as part of the full Zone 1-4 integration flow, config loaded
// directly from database/migrations/V212__PAS_Template_Rebuild_On_FHIR_Builder.sql).
// The TestPAS_BuildPatient_*/BuildCoverage_*/BuildProvider_*/AssembleBundle_*
// unit tests that used to live here tested those four scripts directly; they
// were removed along with the scripts rather than left testing dead code.
//
// This file now covers only what's still a hand-written script in the
// deployed template:
//   TC-PAS-023..028  Parse ClaimResponse (AA / AD / CP / PE / unknown / nil)
//
// Run: go test ./services/executors/enrichment/ -v -run TestPAS
// ─────────────────────────────────────────────────────────────────────────────

import (
	"context"
	"ezhealthkonnect/models"
	"testing"
)

const pasClaimResponseScript = `
// -- Parse Da Vinci PAS ClaimResponse --
// Reads from : steps.submit_to_payer.step_output (payer HTTP response body)
// Writes to  : steps.parse_claim_response.step_output.decision
//              steps.parse_claim_response.step_output.review_action
//              steps.parse_claim_response.step_output.denial_reason
//              steps.parse_claim_response.step_output.raw_response
//
// Da Vinci PAS review action codes (X12 005010/306):
//   A1 -> AA   Approved - all items approved as submitted
//   A2 -> CP   Certified Partial - some items approved, some not
//   A3 -> AD   Denied - not medically necessary or plan exclusion
//   A4 -> AD   Denied (modified) - payer changed qty/dates, treated as denial
//   A6 -> PE   Pended - payer needs more time or additional information
//
// 'Route on Decision' step (seq 210) branches on the decision value.

// Pull the payer ClaimResponse from the API step output
var apiResp = (input.steps && input.steps.submit_to_payer &&
               input.steps.submit_to_payer.step_output)
  ? input.steps.submit_to_payer.step_output
  : {};

// API executor wraps the parsed response body under .response;
// fall back to root of the step output for direct assignments.
var claimResp = apiResp.response || apiResp;

var decision     = "unknown";
var reviewAction = "";
var denialReason = "";
var reviewUrl    = "";

// -- Walk ClaimResponse.item[0].extension to find the reviewAction --
if (claimResp && claimResp.item && claimResp.item.length > 0) {
  var firstItem = claimResp.item[0];

  if (firstItem.extension) {
    for (var i = 0; i < firstItem.extension.length; i++) {
      var ext = firstItem.extension[i];

      // reviewAction extension contains sub-extensions: code and reasonCode
      if (ext.url && ext.url.indexOf("reviewAction") !== -1 && ext.extension) {
        for (var j = 0; j < ext.extension.length; j++) {
          var sub = ext.extension[j];

          // 'code' -> X12 review action code (A1/A2/A3/A4/A6)
          if (sub.url === "code" &&
              sub.valueCodeableConcept &&
              sub.valueCodeableConcept.coding &&
              sub.valueCodeableConcept.coding.length > 0) {
            reviewAction = sub.valueCodeableConcept.coding[0].code || "";
          }

          // 'reasonCode' -> denial reason text (present for AD decisions)
          if (sub.url === "reasonCode" &&
              sub.valueCodeableConcept &&
              sub.valueCodeableConcept.coding &&
              sub.valueCodeableConcept.coding.length > 0) {
            denialReason = sub.valueCodeableConcept.coding[0].display ||
                           sub.valueCodeableConcept.coding[0].code    || "";
          }
        }
      }
    }
  }
}

// -- Map X12 action code -> friendly decision label --
if      (reviewAction === "A1")                          { decision = "AA"; }
else if (reviewAction === "A3" || reviewAction === "A4") { decision = "AD"; }
else if (reviewAction === "A2")                          { decision = "CP"; }
else if (reviewAction === "A6")                          { decision = "PE"; }

// Fallback: some payers use ClaimResponse.outcome instead of PAS extensions
else if (claimResp && claimResp.outcome) {
  if      (claimResp.outcome === "complete") { decision = "AA"; }
  else if (claimResp.outcome === "error")    { decision = "AD"; }
  else if (claimResp.outcome === "partial")  { decision = "CP"; }
}

var claimResponseId = claimResp ? (claimResp.id || "") : "";

return ({
  decision:          decision,          // AA | AD | CP | PE | unknown
  review_action:     reviewAction,      // raw X12 code: A1/A2/A3/A4/A6
  denial_reason:     denialReason,      // populated for AD decisions
  review_url:        reviewUrl,         // populated if payer returns review URL
  claim_response_id: claimResponseId,   // ClaimResponse.id from payer
  raw_response:      claimResp          // full ClaimResponse for downstream steps
});
`

// ─────────────────────────────────────────────────────────────────────────────
// Test helpers
// ─────────────────────────────────────────────────────────────────────────────

func newScriptStep(name, script string) *models.TransformationStep {
	return &models.TransformationStep{
		StepName: name,
		StepType: "enrichment.script",
		Enabled:  true,
		Config:   map[string]interface{}{"script": script},
	}
}

func mustExecScript(t *testing.T, name, script string, input map[string]interface{}) map[string]interface{} {
	t.Helper()
	exec := NewScriptEnrichmentExecutor()
	result, err := exec.Execute(context.Background(), newScriptStep(name, script), input)
	if err != nil {
		t.Fatalf("[%s] script execution failed: %v", name, err)
	}
	out, ok := result["_stepOutput"].(map[string]interface{})
	if !ok {
		t.Fatalf("[%s] _stepOutput missing or not a map", name)
	}
	return out
}

// fullPASInput builds the standard _pas_envelope used across multiple tests.
// provider.firstName/.lastName (not a single combined .name) since V212 —
// mirrors how patient.firstName/.lastName already worked, and removes the
// need for a name-splitting transform in the fhir.build Practitioner config.
func fullPASInput() map[string]interface{} {
	return map[string]interface{}{
		"_pas_envelope": map[string]interface{}{
			"patient": map[string]interface{}{
				"firstName": "Jane",
				"lastName":  "Smith",
				"dob":       "1975-03-22",
				"gender":    "female",
				"memberId":  "MEM-00123",
			},
			"coverage": map[string]interface{}{
				"payerId":     "1234567890",
				"planId":      "GOLD-PPO",
				"groupNumber": "GRP-999",
			},
			"provider": map[string]interface{}{
				"npi":         "9876543210",
				"firstName":   "Alice",
				"lastName":    "Johnson",
				"facilityNpi": "1111111111",
			},
			"request": map[string]interface{}{
				"serviceCode":      "99213",
				"diagnosisCodes":   []interface{}{"Z00.00", "J06.9"},
				"urgency":          "routine",
				"serviceStartDate": "2026-05-01",
				"serviceEndDate":   "2026-05-01",
				"quantity":         "1",
			},
		},
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-PAS-023..028  Parse ClaimResponse
// ─────────────────────────────────────────────────────────────────────────────

func claimResponseInput(reviewActionCode, denialReasonDisplay string) map[string]interface{} {
	ext := []interface{}{
		map[string]interface{}{
			"url": "http://hl7.org/fhir/us/davinci-pas/StructureDefinition/extension-reviewAction",
			"extension": []interface{}{
				map[string]interface{}{
					"url": "code",
					"valueCodeableConcept": map[string]interface{}{
						"coding": []interface{}{
							map[string]interface{}{"code": reviewActionCode},
						},
					},
				},
				map[string]interface{}{
					"url": "reasonCode",
					"valueCodeableConcept": map[string]interface{}{
						"coding": []interface{}{
							map[string]interface{}{
								"code":    "NOT_MED_NECESSARY",
								"display": denialReasonDisplay,
							},
						},
					},
				},
			},
		},
	}

	return map[string]interface{}{
		"steps": map[string]interface{}{
			"submit_to_payer": map[string]interface{}{
				"step_output": map[string]interface{}{
					"resourceType": "ClaimResponse",
					"id":           "cr-001",
					"item": []interface{}{
						map[string]interface{}{"extension": ext},
					},
				},
			},
		},
	}
}

// TC-PAS-023: reviewAction A1 → decision=AA
func TestPAS_ParseClaimResponse_Approved(t *testing.T) {
	out := mustExecScript(t, "TC-PAS-023", pasClaimResponseScript, claimResponseInput("A1", ""))
	if out["decision"] != "AA" {
		t.Errorf("expected AA, got %v", out["decision"])
	}
}

// TC-PAS-024: reviewAction A3 → decision=AD (denied)
func TestPAS_ParseClaimResponse_Denied_A3(t *testing.T) {
	out := mustExecScript(t, "TC-PAS-024", pasClaimResponseScript, claimResponseInput("A3", "Not Medically Necessary"))
	if out["decision"] != "AD" {
		t.Errorf("expected AD for A3, got %v", out["decision"])
	}
	if out["denial_reason"] != "Not Medically Necessary" {
		t.Errorf("expected denial_reason populated, got %v", out["denial_reason"])
	}
}

// TC-PAS-025: reviewAction A4 → decision=AD (denied — modified)
func TestPAS_ParseClaimResponse_Denied_A4(t *testing.T) {
	out := mustExecScript(t, "TC-PAS-025", pasClaimResponseScript, claimResponseInput("A4", ""))
	if out["decision"] != "AD" {
		t.Errorf("expected AD for A4, got %v", out["decision"])
	}
}

// TC-PAS-026: reviewAction A2 → decision=CP (partial approval)
func TestPAS_ParseClaimResponse_PartialApproval(t *testing.T) {
	out := mustExecScript(t, "TC-PAS-026", pasClaimResponseScript, claimResponseInput("A2", ""))
	if out["decision"] != "CP" {
		t.Errorf("expected CP, got %v", out["decision"])
	}
}

// TC-PAS-027: reviewAction A6 → decision=PE (pended)
func TestPAS_ParseClaimResponse_Pended(t *testing.T) {
	out := mustExecScript(t, "TC-PAS-027", pasClaimResponseScript, claimResponseInput("A6", ""))
	if out["decision"] != "PE" {
		t.Errorf("expected PE, got %v", out["decision"])
	}
}

// TC-PAS-028: Fallback to ClaimResponse.outcome when no reviewAction extension
func TestPAS_ParseClaimResponse_OutcomeFallback(t *testing.T) {
	cases := []struct {
		outcome  string
		expected string
	}{
		{"complete", "AA"},
		{"error", "AD"},
		{"partial", "CP"},
		{"queued", "unknown"},
	}
	for _, tc := range cases {
		input := map[string]interface{}{
			"steps": map[string]interface{}{
				"submit_to_payer": map[string]interface{}{
					"step_output": map[string]interface{}{
						"resourceType": "ClaimResponse",
						"id":           "cr-fallback",
						"outcome":      tc.outcome,
					},
				},
			},
		}
		out := mustExecScript(t, "TC-PAS-028-"+tc.outcome, pasClaimResponseScript, input)
		if out["decision"] != tc.expected {
			t.Errorf("outcome=%s: expected %s, got %v", tc.outcome, tc.expected, out["decision"])
		}
	}
}
