package enrichment

// ─────────────────────────────────────────────────────────────────────────────
// Da Vinci PAS Script Step Tests
//
// Tests the four enrichment.script steps that are pre-built inside the
// Da Vinci PAS OOB pipeline template (V91 migration):
//   TC-PAS-001..005  Build FHIR Patient
//   TC-PAS-006..010  Build FHIR Coverage
//   TC-PAS-011..015  Build FHIR Provider (Practitioner + Organization)
//   TC-PAS-016..022  Assemble PAS Claim Bundle
//   TC-PAS-023..028  Parse ClaimResponse (AA / AD / CP / PE / unknown / nil)
//
// Run: go test ./services/executors/enrichment/ -v -run TestPAS
// ─────────────────────────────────────────────────────────────────────────────

import (
	"context"
	"ezhealthkonnect/models"
	"testing"
)

// ─────────────────────────────────────────────────────────────────────────────
// Shared scripts — formatted, readable versions of the V91 migration scripts.
// These constants are the single source of truth for test and production SQL.
// If you change a script here, update V91__DaVinci_PAS_Template.sql to match.
// ─────────────────────────────────────────────────────────────────────────────

const pasPatientScript = `
// -- Build FHIR Patient (Da Vinci PAS Subscriber profile) --
// Reads from : input._pas_envelope.patient.*
// Writes to  : steps.build_patient.step_output.patient
//
// HOW TO USE: Map these fields in 'Map to PAS Envelope':
//   _pas_envelope.patient.firstName  - patient first name       (REQUIRED)
//   _pas_envelope.patient.lastName   - patient last name        (REQUIRED)
//   _pas_envelope.patient.dob        - date of birth YYYY-MM-DD (REQUIRED)
//   _pas_envelope.patient.memberId   - payer member ID          (REQUIRED, becomes FHIR Patient.id)
//   _pas_envelope.patient.gender     - male / female / other    (optional)

var env      = input._pas_envelope || {};
var pat      = env.patient || {};

// memberId becomes both the FHIR Patient.id and the MB identifier value
var memberId = pat.memberId || ("pat-" + Date.now());

// FHIR only accepts: male | female | other | unknown
var gender = (pat.gender || "unknown").toLowerCase();
if (gender !== "male" && gender !== "female" && gender !== "other" && gender !== "unknown") {
  gender = "unknown";
}

var patient = {
  resourceType: "Patient",
  id:           memberId,

  // Da Vinci PAS Subscriber profile URL (required by the IG)
  meta: { profile: ["http://hl7.org/fhir/us/davinci-pas/StructureDefinition/profile-subscriber"] },

  // Member Number identifier (MB type) - links back to the payer member roster
  identifier: [{
    type:   { coding: [{ system: "http://terminology.hl7.org/CodeSystem/v2-0203", code: "MB", display: "Member Number" }] },
    system: "urn:oid:2.16.840.1.113883.4.6",
    value:  memberId
  }],

  // Official legal name as it appears on the insurance card
  name:      [{ use: "official", family: pat.lastName || "", given: [pat.firstName || ""] }],
  birthDate: pat.dob || "",
  gender:    gender
};

return ({ patient: patient });
`

const pasCoverageScript = `
// -- Build FHIR Coverage (payer insurance card) --
// Reads from : input._pas_envelope.coverage.* and .patient.*
// Writes to  : steps.build_coverage.step_output.coverage
//
// HOW TO USE: Map these fields in 'Map to PAS Envelope':
//   _pas_envelope.coverage.payerId      - payer NPI or OID       (REQUIRED)
//   _pas_envelope.coverage.planId       - plan ID, e.g. GOLD-PPO (optional)
//   _pas_envelope.coverage.groupNumber  - employer group number   (optional)
//
// Subscriber relationship defaults to 'self' (patient = subscriber).

var env      = input._pas_envelope || {};
var cov      = env.coverage || {};
var pat      = env.patient  || {};
var memberId = pat.memberId || "patient-1";

var coverage = {
  resourceType: "Coverage",
  id:           "coverage-1",

  // Da Vinci PAS Coverage profile URL
  meta:         { profile: ["http://hl7.org/fhir/us/davinci-pas/StructureDefinition/profile-coverage"] },
  status:       "active",

  // Subscriber = Patient (self scenario, most common for prior auth)
  subscriber:   { reference: "Patient/" + memberId },
  subscriberId: memberId,
  beneficiary:  { reference: "Patient/" + memberId },
  relationship: { coding: [{ system: "http://terminology.hl7.org/CodeSystem/subscriber-relationship", code: "self" }] },

  // Payer identified by NPI - must match Claim.insurer
  payor: [{ identifier: { system: "http://hl7.org/fhir/sid/us-npi", value: cov.payerId || "" } }]
};

// Optional: add plan class (e.g. GOLD-PPO) - required by some payer APIs
if (cov.planId) {
  coverage.class = [{
    type:  { coding: [{ system: "http://terminology.hl7.org/CodeSystem/coverage-class", code: "plan" }] },
    value: cov.planId,
    name:  "Health Plan"
  }];
}

// Optional: add employer group number
if (cov.groupNumber) {
  coverage.class = coverage.class || [];
  coverage.class.push({
    type:  { coding: [{ system: "http://terminology.hl7.org/CodeSystem/coverage-class", code: "group" }] },
    value: cov.groupNumber
  });
}

return ({ coverage: coverage });
`

const pasProviderScript = `
// -- Build Provider Resources (Practitioner + Organization) --
// Reads from : input._pas_envelope.provider.*
// Writes to  : steps.build_provider.step_output.practitioner
//              steps.build_provider.step_output.organization
//
// HOW TO USE: Map these fields in 'Map to PAS Envelope':
//   _pas_envelope.provider.npi              - requesting clinician NPI  (REQUIRED)
//   _pas_envelope.provider.name             - full name e.g. 'Alice Johnson' (optional)
//   _pas_envelope.provider.facilityNpi      - facility NPI (falls back to clinician NPI)
//   _pas_envelope.provider.organizationName - org display name (optional)
//   _pas_envelope.provider.phone            - org contact phone (optional)
//
// Claim references Organization as provider (billing entity).
// Line item references Practitioner (individual ordering clinician).

var env  = input._pas_envelope || {};
var prov = env.provider || {};

// Split 'Alice Johnson' -> family='Johnson', given=['Alice']
var nameParts  = (prov.name || "Unknown Provider").split(" ");
var lastName   = nameParts.length > 1 ? nameParts[nameParts.length - 1] : nameParts[0];
var firstNames = nameParts.length > 1 ? nameParts.slice(0, nameParts.length - 1) : [];

// Practitioner = the individual ordering / requesting clinician
var practitioner = {
  resourceType: "Practitioner",
  id:           "practitioner-1",
  meta:         { profile: ["http://hl7.org/fhir/us/davinci-pas/StructureDefinition/profile-practitioner"] },
  identifier:   [{ system: "http://hl7.org/fhir/sid/us-npi", value: prov.npi || "" }],
  name:         [{ family: lastName, given: firstNames }]
};

// Organization = the requesting facility / billing entity on the Claim
// Falls back to clinician NPI when no separate facilityNpi is provided
var facilityNpi = prov.facilityNpi || prov.npi || "";

var organization = {
  resourceType: "Organization",
  id:           "organization-1",
  meta:         { profile: ["http://hl7.org/fhir/us/davinci-pas/StructureDefinition/profile-requestor"] },
  identifier:   [{ system: "http://hl7.org/fhir/sid/us-npi", value: facilityNpi }],
  name:         prov.organizationName || prov.name || "Requesting Organization"
};

// Optional contact phone on the organization
if (prov.phone) {
  organization.telecom = [{ system: "phone", value: prov.phone, use: "work" }];
}

return ({ practitioner: practitioner, organization: organization });
`

const pasBundleScript = `
// -- Assemble Da Vinci PAS Request Bundle --
// Reads from : steps.build_patient / build_coverage / build_provider step outputs
//              input._pas_envelope.request.*
// Writes to  : steps.assemble_pas_bundle.step_output.pas_bundle  (POSTed to payer)
//              steps.assemble_pas_bundle.step_output.claim_id / bundle_id
//
// HOW TO USE: No edits needed here. The bundle assembles automatically from the
// three Build steps above. To add SupportingInfo, authRefNumber, or extra line
// items, add a script step AFTER this one that modifies pas_bundle.
//
// Urgency: _pas_envelope.request.urgency = 'urgent' -> Claim.priority = stat
//          Anything else (or omitted)               -> Claim.priority = normal

var env = input._pas_envelope || {};
var req = env.request || {};
var pat = env.patient || {};

var ts       = new Date().toISOString();
var claimId  = "claim-"      + Date.now();
var bundleId = "pas-bundle-" + Date.now();

// -- Pull FHIR resources from the preceding build steps --
// Each falls back to a minimal stub if a build step was skipped.
var patient = (input.steps && input.steps.build_patient &&
               input.steps.build_patient.step_output &&
               input.steps.build_patient.step_output.patient)
  ? input.steps.build_patient.step_output.patient
  : { resourceType: "Patient", id: pat.memberId || "patient-1" };

var coverage = (input.steps && input.steps.build_coverage &&
                input.steps.build_coverage.step_output &&
                input.steps.build_coverage.step_output.coverage)
  ? input.steps.build_coverage.step_output.coverage
  : { resourceType: "Coverage", id: "coverage-1" };

var practitioner = (input.steps && input.steps.build_provider &&
                    input.steps.build_provider.step_output &&
                    input.steps.build_provider.step_output.practitioner)
  ? input.steps.build_provider.step_output.practitioner
  : { resourceType: "Practitioner", id: "practitioner-1" };

var organization = (input.steps && input.steps.build_provider &&
                    input.steps.build_provider.step_output &&
                    input.steps.build_provider.step_output.organization)
  ? input.steps.build_provider.step_output.organization
  : { resourceType: "Organization", id: "organization-1" };

// -- Diagnosis codes -> Claim.diagnosis array --
// Accepts a JS array or a JSON-encoded string: ["J18.9", "Z87.891"]
var diagCodes = req.diagnosisCodes || [];
if (typeof diagCodes === "string") {
  try { diagCodes = JSON.parse(diagCodes); } catch (e) { diagCodes = [diagCodes]; }
}

var diagnoses = diagCodes.map(function(code, idx) {
  return {
    sequence: idx + 1,
    diagnosisCodeableConcept: {
      coding: [{ system: "http://hl7.org/fhir/sid/icd-10-cm", code: String(code) }]
    }
  };
});

// -- Priority and service dates --
var priority    = (req.urgency === "urgent") ? "stat" : "normal";
var serviceDate = req.serviceStartDate || ts.substring(0, 10);
var endDate     = req.serviceEndDate   || serviceDate;

// -- Build the Claim --
// Claim.use MUST be 'preauthorization' per the Da Vinci PAS IG.
// Enforced by the FHIR Validate step (seq 140).
var claim = {
  resourceType: "Claim",
  id:           claimId,
  meta:         { profile: ["http://hl7.org/fhir/us/davinci-pas/StructureDefinition/profile-claim"] },
  status:       "active",
  type:         { coding: [{ system: "http://terminology.hl7.org/CodeSystem/claim-type", code: "professional" }] },
  use:          "preauthorization",

  patient:      { reference: "Patient/" + (pat.memberId || "patient-1") },
  created:      ts,

  // Insurer = the payer, identified by NPI
  insurer:      { identifier: { system: "http://hl7.org/fhir/sid/us-npi", value: (env.coverage || {}).payerId || "" } },

  // Provider = requesting organisation (billing entity, not individual clinician)
  provider:     { reference: "Organization/organization-1" },

  priority:     { coding: [{ system: "http://terminology.hl7.org/CodeSystem/processpriority", code: priority }] },
  insurance:    [{ sequence: 1, focal: true, coverage: { reference: "Coverage/coverage-1" } }],
  diagnosis:    diagnoses,

  // Line item: the service being requested for authorisation
  item: [{
    sequence: 1,

    // PAS extension: the date range the service is requested for
    extension: [{
      url:         "http://hl7.org/fhir/us/davinci-pas/StructureDefinition/extension-serviceItemRequestedDate",
      valuePeriod: { start: serviceDate, end: endDate }
    }],

    category:         { coding: [{ system: "https://codesystem.x12.org/005010/1365", code: "1", display: "Medical Care" }] },
    productOrService: { coding: [{ system: "http://www.ama-assn.org/go/cpt", code: req.serviceCode || "" }] },
    quantity:         { value: req.quantity ? Number(req.quantity) : 1 },
    provider:         { reference: "Practitioner/practitioner-1" }
  }]
};

// -- Wrap in a Da Vinci PAS Request Bundle --
var bundle = {
  resourceType: "Bundle",
  id:           bundleId,
  meta:         { profile: ["http://hl7.org/fhir/us/davinci-pas/StructureDefinition/profile-pas-request-bundle"] },
  type:         "collection",
  timestamp:    ts,
  entry: [
    { fullUrl: "urn:uuid:" + claimId,         resource: claim         },
    { fullUrl: "urn:uuid:" + patient.id,      resource: patient       },
    { fullUrl: "urn:uuid:coverage-1",         resource: coverage      },
    { fullUrl: "urn:uuid:practitioner-1",     resource: practitioner  },
    { fullUrl: "urn:uuid:organization-1",     resource: organization  }
  ]
};

return ({ pas_bundle: bundle, claim_id: claimId, bundle_id: bundleId, created_at: ts });
`

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
				"name":        "Alice Johnson",
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
// TC-PAS-001..005  Build FHIR Patient
// ─────────────────────────────────────────────────────────────────────────────

// TC-PAS-001: Patient resource has correct resourceType and PAS profile URL
func TestPAS_BuildPatient_ResourceType(t *testing.T) {
	out := mustExecScript(t, "TC-PAS-001", pasPatientScript, fullPASInput())

	patient, ok := out["patient"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'patient' key in step output")
	}
	if patient["resourceType"] != "Patient" {
		t.Errorf("expected resourceType=Patient, got %v", patient["resourceType"])
	}
	meta, _ := patient["meta"].(map[string]interface{})
	profiles, _ := meta["profile"].([]interface{})
	if len(profiles) == 0 {
		t.Fatal("expected at least one profile URL")
	}
	if profiles[0] != "http://hl7.org/fhir/us/davinci-pas/StructureDefinition/profile-subscriber" {
		t.Errorf("unexpected PAS profile URL: %v", profiles[0])
	}
}

// TC-PAS-002: Patient name, dob, and gender are mapped correctly
func TestPAS_BuildPatient_Demographics(t *testing.T) {
	out := mustExecScript(t, "TC-PAS-002", pasPatientScript, fullPASInput())
	patient := out["patient"].(map[string]interface{})

	names, _ := patient["name"].([]interface{})
	if len(names) == 0 {
		t.Fatal("expected name array")
	}
	nameEntry := names[0].(map[string]interface{})
	if nameEntry["family"] != "Smith" {
		t.Errorf("expected family=Smith, got %v", nameEntry["family"])
	}
	given, _ := nameEntry["given"].([]interface{})
	if len(given) == 0 || given[0] != "Jane" {
		t.Errorf("expected given=[Jane], got %v", given)
	}
	if patient["birthDate"] != "1975-03-22" {
		t.Errorf("expected birthDate=1975-03-22, got %v", patient["birthDate"])
	}
	if patient["gender"] != "female" {
		t.Errorf("expected gender=female, got %v", patient["gender"])
	}
}

// TC-PAS-003: Patient identifier uses memberId with MB type
func TestPAS_BuildPatient_MemberIdentifier(t *testing.T) {
	out := mustExecScript(t, "TC-PAS-003", pasPatientScript, fullPASInput())
	patient := out["patient"].(map[string]interface{})

	identifiers, _ := patient["identifier"].([]interface{})
	if len(identifiers) == 0 {
		t.Fatal("expected identifier array")
	}
	ident := identifiers[0].(map[string]interface{})
	if ident["value"] != "MEM-00123" {
		t.Errorf("expected identifier value=MEM-00123, got %v", ident["value"])
	}
}

// TC-PAS-004: Unknown gender value is normalised to "unknown"
func TestPAS_BuildPatient_GenderNormalisation(t *testing.T) {
	input := fullPASInput()
	input["_pas_envelope"].(map[string]interface{})["patient"].(map[string]interface{})["gender"] = "MALE"
	out := mustExecScript(t, "TC-PAS-004", pasPatientScript, input)
	patient := out["patient"].(map[string]interface{})
	if patient["gender"] != "male" {
		t.Errorf("expected normalised gender=male, got %v", patient["gender"])
	}

	input["_pas_envelope"].(map[string]interface{})["patient"].(map[string]interface{})["gender"] = "nonbinary"
	out = mustExecScript(t, "TC-PAS-004b", pasPatientScript, input)
	patient = out["patient"].(map[string]interface{})
	if patient["gender"] != "unknown" {
		t.Errorf("expected unrecognised gender→unknown, got %v", patient["gender"])
	}
}

// TC-PAS-005: Empty envelope produces a Patient with fallback id (no panic)
func TestPAS_BuildPatient_EmptyEnvelope(t *testing.T) {
	out := mustExecScript(t, "TC-PAS-005", pasPatientScript, map[string]interface{}{})
	patient, ok := out["patient"].(map[string]interface{})
	if !ok {
		t.Fatal("expected patient even with empty envelope")
	}
	if patient["resourceType"] != "Patient" {
		t.Errorf("expected Patient resourceType, got %v", patient["resourceType"])
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-PAS-006..010  Build FHIR Coverage
// ─────────────────────────────────────────────────────────────────────────────

// TC-PAS-006: Coverage has correct profile and status=active
func TestPAS_BuildCoverage_ProfileAndStatus(t *testing.T) {
	out := mustExecScript(t, "TC-PAS-006", pasCoverageScript, fullPASInput())
	cov, ok := out["coverage"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'coverage' key")
	}
	if cov["resourceType"] != "Coverage" {
		t.Errorf("expected Coverage, got %v", cov["resourceType"])
	}
	if cov["status"] != "active" {
		t.Errorf("expected status=active, got %v", cov["status"])
	}
}

// TC-PAS-007: Coverage subscriber and beneficiary reference the member ID
func TestPAS_BuildCoverage_PatientRef(t *testing.T) {
	out := mustExecScript(t, "TC-PAS-007", pasCoverageScript, fullPASInput())
	cov := out["coverage"].(map[string]interface{})

	sub, _ := cov["subscriber"].(map[string]interface{})
	if sub["reference"] != "Patient/MEM-00123" {
		t.Errorf("expected subscriber ref=Patient/MEM-00123, got %v", sub["reference"])
	}
	ben, _ := cov["beneficiary"].(map[string]interface{})
	if ben["reference"] != "Patient/MEM-00123" {
		t.Errorf("expected beneficiary ref=Patient/MEM-00123, got %v", ben["reference"])
	}
}

// TC-PAS-008: Coverage payor identifier uses payerId
func TestPAS_BuildCoverage_PayorId(t *testing.T) {
	out := mustExecScript(t, "TC-PAS-008", pasCoverageScript, fullPASInput())
	cov := out["coverage"].(map[string]interface{})
	payors, _ := cov["payor"].([]interface{})
	if len(payors) == 0 {
		t.Fatal("expected payor array")
	}
	payor := payors[0].(map[string]interface{})
	ident, _ := payor["identifier"].(map[string]interface{})
	if ident["value"] != "1234567890" {
		t.Errorf("expected payerId=1234567890, got %v", ident["value"])
	}
}

// TC-PAS-009: planId and groupNumber produce class entries
func TestPAS_BuildCoverage_ClassEntries(t *testing.T) {
	out := mustExecScript(t, "TC-PAS-009", pasCoverageScript, fullPASInput())
	cov := out["coverage"].(map[string]interface{})
	classes, _ := cov["class"].([]interface{})
	if len(classes) < 2 {
		t.Errorf("expected 2 class entries (plan + group), got %d", len(classes))
	}
}

// TC-PAS-010: Missing planId / groupNumber produces no class entries
func TestPAS_BuildCoverage_NoOptionalFields(t *testing.T) {
	input := fullPASInput()
	input["_pas_envelope"].(map[string]interface{})["coverage"] = map[string]interface{}{
		"payerId": "9999999999",
	}
	out := mustExecScript(t, "TC-PAS-010", pasCoverageScript, input)
	cov := out["coverage"].(map[string]interface{})
	if _, hasClass := cov["class"]; hasClass {
		t.Error("expected no class when planId and groupNumber absent")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-PAS-011..015  Build Provider Resources
// ─────────────────────────────────────────────────────────────────────────────

// TC-PAS-011: Practitioner has correct NPI identifier
func TestPAS_BuildProvider_PractitionerNPI(t *testing.T) {
	out := mustExecScript(t, "TC-PAS-011", pasProviderScript, fullPASInput())
	pract, ok := out["practitioner"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'practitioner' key")
	}
	ids, _ := pract["identifier"].([]interface{})
	if len(ids) == 0 {
		t.Fatal("expected identifier on Practitioner")
	}
	id := ids[0].(map[string]interface{})
	if id["value"] != "9876543210" {
		t.Errorf("expected NPI=9876543210, got %v", id["value"])
	}
}

// TC-PAS-012: Practitioner name is split correctly (given / family)
func TestPAS_BuildProvider_PractitionerNameSplit(t *testing.T) {
	out := mustExecScript(t, "TC-PAS-012", pasProviderScript, fullPASInput())
	pract := out["practitioner"].(map[string]interface{})
	names, _ := pract["name"].([]interface{})
	if len(names) == 0 {
		t.Fatal("expected name array")
	}
	n := names[0].(map[string]interface{})
	if n["family"] != "Johnson" {
		t.Errorf("expected family=Johnson, got %v", n["family"])
	}
	given, _ := n["given"].([]interface{})
	if len(given) == 0 || given[0] != "Alice" {
		t.Errorf("expected given=[Alice], got %v", given)
	}
}

// TC-PAS-013: Organization has correct profile and name
func TestPAS_BuildProvider_OrganizationProfile(t *testing.T) {
	out := mustExecScript(t, "TC-PAS-013", pasProviderScript, fullPASInput())
	org, ok := out["organization"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'organization' key")
	}
	if org["resourceType"] != "Organization" {
		t.Errorf("expected Organization, got %v", org["resourceType"])
	}
	ids, _ := org["identifier"].([]interface{})
	if len(ids) == 0 {
		t.Fatal("expected identifier on Organization")
	}
	id := ids[0].(map[string]interface{})
	if id["value"] != "1111111111" {
		t.Errorf("expected facilityNpi=1111111111 on org, got %v", id["value"])
	}
}

// TC-PAS-014: Organization falls back to provider NPI when facilityNpi absent
func TestPAS_BuildProvider_FacilityNPIFallback(t *testing.T) {
	input := fullPASInput()
	input["_pas_envelope"].(map[string]interface{})["provider"] = map[string]interface{}{
		"npi":  "5555555555",
		"name": "Bob Doctor",
	}
	out := mustExecScript(t, "TC-PAS-014", pasProviderScript, input)
	org := out["organization"].(map[string]interface{})
	ids, _ := org["identifier"].([]interface{})
	id := ids[0].(map[string]interface{})
	if id["value"] != "5555555555" {
		t.Errorf("expected org NPI to fall back to provider NPI=5555555555, got %v", id["value"])
	}
}

// TC-PAS-015: Phone number adds telecom to Organization when present
func TestPAS_BuildProvider_OrgPhone(t *testing.T) {
	input := fullPASInput()
	input["_pas_envelope"].(map[string]interface{})["provider"].(map[string]interface{})["phone"] = "555-867-5309"
	out := mustExecScript(t, "TC-PAS-015", pasProviderScript, input)
	org := out["organization"].(map[string]interface{})
	telecoms, _ := org["telecom"].([]interface{})
	if len(telecoms) == 0 {
		t.Fatal("expected telecom entry when phone present")
	}
	telecom := telecoms[0].(map[string]interface{})
	if telecom["value"] != "555-867-5309" {
		t.Errorf("expected phone value=555-867-5309, got %v", telecom["value"])
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-PAS-016..022  Assemble PAS Claim Bundle
// ─────────────────────────────────────────────────────────────────────────────

// buildBundleInput runs the three prior scripts and returns the combined
// inputData that the bundle assembly script expects.
func buildBundleInput(t *testing.T) map[string]interface{} {
	t.Helper()
	base := fullPASInput()
	patOut  := mustExecScript(t, "build_patient",  pasPatientScript,  base)
	covOut  := mustExecScript(t, "build_coverage", pasCoverageScript, base)
	provOut := mustExecScript(t, "build_provider", pasProviderScript, base)

	base["steps"] = map[string]interface{}{
		"build_patient": map[string]interface{}{
			"step_output": patOut,
		},
		"build_coverage": map[string]interface{}{
			"step_output": covOut,
		},
		"build_provider": map[string]interface{}{
			"step_output": provOut,
		},
	}
	return base
}

// TC-PAS-016: Bundle has resourceType=Bundle, type=collection, and PAS profile
func TestPAS_AssembleBundle_Structure(t *testing.T) {
	out := mustExecScript(t, "TC-PAS-016", pasBundleScript, buildBundleInput(t))
	bundle, ok := out["pas_bundle"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'pas_bundle' key")
	}
	if bundle["resourceType"] != "Bundle" {
		t.Errorf("expected Bundle, got %v", bundle["resourceType"])
	}
	if bundle["type"] != "collection" {
		t.Errorf("expected type=collection, got %v", bundle["type"])
	}
	meta, _ := bundle["meta"].(map[string]interface{})
	profiles, _ := meta["profile"].([]interface{})
	if len(profiles) == 0 {
		t.Fatal("expected PAS profile on bundle")
	}
	found := false
	for _, p := range profiles {
		if p == "http://hl7.org/fhir/us/davinci-pas/StructureDefinition/profile-pas-request-bundle" {
			found = true
		}
	}
	if !found {
		t.Error("missing profile-pas-request-bundle profile URL")
	}
}

// TC-PAS-017: Bundle contains exactly 5 entries (Claim, Patient, Coverage, Practitioner, Organization)
func TestPAS_AssembleBundle_EntryCount(t *testing.T) {
	out := mustExecScript(t, "TC-PAS-017", pasBundleScript, buildBundleInput(t))
	bundle := out["pas_bundle"].(map[string]interface{})
	entries, _ := bundle["entry"].([]interface{})
	if len(entries) != 5 {
		t.Errorf("expected 5 bundle entries, got %d", len(entries))
	}
}

// TC-PAS-018: Claim inside bundle has use=preauthorization
func TestPAS_AssembleBundle_ClaimPreauthorization(t *testing.T) {
	out := mustExecScript(t, "TC-PAS-018", pasBundleScript, buildBundleInput(t))
	bundle := out["pas_bundle"].(map[string]interface{})
	entries := bundle["entry"].([]interface{})
	var claim map[string]interface{}
	for _, e := range entries {
		entry := e.(map[string]interface{})
		res, _ := entry["resource"].(map[string]interface{})
		if res["resourceType"] == "Claim" {
			claim = res
			break
		}
	}
	if claim == nil {
		t.Fatal("Claim resource not found in bundle")
	}
	if claim["use"] != "preauthorization" {
		t.Errorf("expected Claim.use=preauthorization, got %v", claim["use"])
	}
}

// TC-PAS-019: Diagnosis codes are mapped to ICD-10 codings correctly
func TestPAS_AssembleBundle_DiagnosisCodes(t *testing.T) {
	out := mustExecScript(t, "TC-PAS-019", pasBundleScript, buildBundleInput(t))
	bundle := out["pas_bundle"].(map[string]interface{})
	entries := bundle["entry"].([]interface{})
	var claim map[string]interface{}
	for _, e := range entries {
		entry := e.(map[string]interface{})
		res, _ := entry["resource"].(map[string]interface{})
		if res["resourceType"] == "Claim" {
			claim = res
		}
	}
	diags, _ := claim["diagnosis"].([]interface{})
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnoses (Z00.00 + J06.9), got %d", len(diags))
	}
	firstDiag := diags[0].(map[string]interface{})
	dc, _ := firstDiag["diagnosisCodeableConcept"].(map[string]interface{})
	codings, _ := dc["coding"].([]interface{})
	if len(codings) == 0 {
		t.Fatal("expected coding on first diagnosis")
	}
	c := codings[0].(map[string]interface{})
	if c["code"] != "Z00.00" {
		t.Errorf("expected first diagnosis code=Z00.00, got %v", c["code"])
	}
}

// TC-PAS-020: Urgent request sets priority.coding[0].code = "stat"
func TestPAS_AssembleBundle_UrgentPriority(t *testing.T) {
	input := buildBundleInput(t)
	input["_pas_envelope"].(map[string]interface{})["request"].(map[string]interface{})["urgency"] = "urgent"
	out := mustExecScript(t, "TC-PAS-020", pasBundleScript, input)
	bundle := out["pas_bundle"].(map[string]interface{})
	entries := bundle["entry"].([]interface{})
	for _, e := range entries {
		entry := e.(map[string]interface{})
		res, _ := entry["resource"].(map[string]interface{})
		if res["resourceType"] != "Claim" {
			continue
		}
		priority, _ := res["priority"].(map[string]interface{})
		codings, _ := priority["coding"].([]interface{})
		if len(codings) == 0 {
			t.Fatal("expected priority coding")
		}
		c := codings[0].(map[string]interface{})
		if c["code"] != "stat" {
			t.Errorf("expected priority=stat for urgent, got %v", c["code"])
		}
	}
}

// TC-PAS-021: Routine request sets priority = "normal"
func TestPAS_AssembleBundle_RoutinePriority(t *testing.T) {
	out := mustExecScript(t, "TC-PAS-021", pasBundleScript, buildBundleInput(t))
	bundle := out["pas_bundle"].(map[string]interface{})
	entries := bundle["entry"].([]interface{})
	for _, e := range entries {
		entry := e.(map[string]interface{})
		res, _ := entry["resource"].(map[string]interface{})
		if res["resourceType"] != "Claim" {
			continue
		}
		priority, _ := res["priority"].(map[string]interface{})
		codings, _ := priority["coding"].([]interface{})
		c := codings[0].(map[string]interface{})
		if c["code"] != "normal" {
			t.Errorf("expected priority=normal for routine, got %v", c["code"])
		}
	}
}

// TC-PAS-022: Output also contains claim_id, bundle_id, created_at
func TestPAS_AssembleBundle_Metadata(t *testing.T) {
	out := mustExecScript(t, "TC-PAS-022", pasBundleScript, buildBundleInput(t))
	for _, key := range []string{"claim_id", "bundle_id", "created_at"} {
		if out[key] == nil {
			t.Errorf("expected '%s' in bundle assembly output", key)
		}
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
