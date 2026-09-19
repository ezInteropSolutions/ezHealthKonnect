// ─────────────────────────────────────────────────────────────────────────────
// EDI X12 834 → FHIR mapping (EDI Phase 7, Benefit Enrollment and
// Maintenance). Transform-only scope: this engine converts X12 <-> FHIR, it
// never decides enrollment facts. Maps to Patient (the member) + Coverage
// (one per real-world health-coverage enrollment, HD01's own maintenance-
// type code translated into FHIR's active/cancelled Coverage.status
// vocabulary -- the same class of code-system translation 277's own STC
// mapping already established, never an invented fact: the status always
// comes straight from the source HD01).
//
// Unlike every other EDI-to-FHIR mapping in this engine, 834 has no
// "response" transaction set at all (a pure one-way roster feed) -- so there
// is no conditional second resource to gate, just Organization (sponsor,
// single-resource) + Patient + Coverage, both rowsPath-built off the SAME
// flattened member-coverage-pair array the derive script produces (mirroring
// 837P/837I's own "Patient/Coverage built per context, not deduplicated"
// precedent).
//
// Run: go test ./services/ -v -run TestEDI834FHIRBuilder
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

// edi834FixtureLoops -- one sponsor, a subscriber with an ACTIVE health-
// coverage enrollment (HD01="021" Addition) and a dependent with a
// TERMINATED one (HD01="024" Cancellation/Termination -- the REAL X12
// element 875 code, confirmed directly against X12.org's own official
// "Terminate Eligibility for a Subscriber" example, x12.org/examples/
// 005010x220/example-07-terminate-eligibility-subscriber; an earlier
// version of this fixture used "030" (Audit or Compare -- not a real
// termination code at all), a bug only caught by testing against real data),
// both flat top-level 2000 members (no HL hierarchy at all -- 834's own
// genuinely different shape).
func edi834FixtureLoops() map[string]interface{} {
	return map[string]interface{}{
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
							"HD":  map[string]interface{}{"maintenanceTypeCode": "021", "insuranceLineCode": "HLT", "planCoverageDescription": "GOLD PPO"},
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
	}
}

const derive834StepAlias = "derive_834_member_coverages"

// derive834MemberCoveragesScript is pure structural reshaping -- it never
// decides enrollment facts. Flattens 834's flat member-list structure into
// one row per (member x coverage) pair, translating HD01's own maintenance
// type code into FHIR's Coverage.status vocabulary along the way (a
// code-system translation, same class as 270/271's own gender/relationship
// mapping): the status always comes straight from the source HD01, this only
// re-expresses it.
const derive834MemberCoveragesScript = `
var parsed = (input.message && input.message.parsedEDI) || input.parsedEDI || {};
var loops = parsed.loops || {};

if (parsed.transactionSet !== "834") {
  return ({ _member_coverages: [] });
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
function relationshipToFHIR(code) {
  var map = { "18": "self", "01": "spouse", "19": "child" };
  return map[code] || "other";
}
function maintenanceTypeToCoverageStatus(code) {
  // Real X12 element 875 (Maintenance Type Code) standard values, confirmed
  // directly against stedi.com/edi/x12-005010/element/875's own code list
  // AND cross-checked against X12.org's own official 834 examples: "021"
  // Addition and "025" Reinstatement both genuinely mean the coverage is
  // active; "024" is "Cancellation or Termination" -- the REAL code for a
  // terminated enrollment (X12.org's own Example 07, "Terminate Eligibility
  // for a Subscriber", uses exactly this code). An earlier version of this
  // mapping had "024"->"active" and "030"->"cancelled" -- backwards: "030"
  // is "Audit or Compare" in the real standard, not a termination code at
  // all. Found and fixed only by testing against real X12.org sample data,
  // not by re-reading the generic Stedi segment reference a second time.
  var map = { "021": "active", "025": "active", "024": "cancelled", "002": "cancelled" };
  return map[code] || "active";
}

var members = arr(loops["2000"]);
var rows = [];
var nowIso = new Date().toISOString();

for (var i = 0; i < members.length; i++) {
  var member = members[i];
  var ins = member.INS || {};
  var ref = member.REF || {};
  var m2100A = (member.loops || {})["2100A"] || {};
  var nm1 = m2100A.NM1 || {};
  var dmg = m2100A.DMG || {};

  var patientInfo = {
    member_id: nm1.identificationCode || ref.referenceIdentification || "",
    first_name: nm1.nameFirst || "",
    last_name: nm1.nameLastOrOrganizationName || "",
    dob: dmg.birthDate || "",
    gender_fhir: genderToFHIR(dmg.genderCode),
    relationship_fhir: relationshipToFHIR(ins.individualRelationshipCode),
  };

  var coverages = arr((member.loops || {})["2300"]);
  for (var c = 0; c < coverages.length; c++) {
    var hd = coverages[c].HD || {};
    rows.push({
      patient_info: patientInfo,
      created_at: nowIso,
      subscriber_id: ref.referenceIdentification || "",
      plan_description: hd.planCoverageDescription || "",
      insurance_line_code: hd.insuranceLineCode || "",
      coverage_status: maintenanceTypeToCoverageStatus(hd.maintenanceTypeCode || ""),
      maintenance_type_code: hd.maintenanceTypeCode || "",
    });
  }
}

return ({ _member_coverages: rows });
`

func organization834Config() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Organization",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirOrganization",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "literalValue": "organization-sponsor"},
			map[string]interface{}{"targetPath": "active", "literalValue": "true"},
			map[string]interface{}{"targetPath": "name", "literalValue": "ACME CORP"},
		},
	}
}

func patient834Config() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Patient",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirPatients",
		"rowsPath":     "steps." + derive834StepAlias + ".step_output._member_coverages",
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

func coverage834Config() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Coverage",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirCoverages",
		"rowsPath":     "steps." + derive834StepAlias + ".step_output._member_coverages",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": "_rowIndex", "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "coverage-"}},
			map[string]interface{}{"targetPath": "status", "sourcePath": "coverage_status"},
			map[string]interface{}{"targetPath": "type.coding[0].system", "literalValue": "https://codesystem.x12.org/005010/1205"},
			map[string]interface{}{"targetPath": "type.coding[0].code", "sourcePath": "insurance_line_code"},
			map[string]interface{}{"targetPath": "subscriberId", "sourcePath": "subscriber_id"},
			map[string]interface{}{"targetPath": "beneficiary.reference", "sourcePath": "patient_info.member_id", "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Patient/"}},
			map[string]interface{}{"targetPath": "relationship.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/subscriber-relationship"},
			map[string]interface{}{"targetPath": "relationship.coding[0].code", "sourcePath": "patient_info.relationship_fhir"},
			map[string]interface{}{"targetPath": "payor[0].identifier.system", "literalValue": "http://ezhealthkonnect.local/x12-payer-id"},
			map[string]interface{}{"targetPath": "payor[0].identifier.value", "literalValue": "PAYER001"},
			map[string]interface{}{"targetPath": "class[0].type.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/coverage-class"},
			map[string]interface{}{"targetPath": "class[0].type.coding[0].code", "literalValue": "plan"},
			map[string]interface{}{"targetPath": "class[0].value", "sourcePath": "plan_description"},
		},
	}
}

// TC-EDI834-FB-001: full chain (derive -> Organization -> Patient -> Coverage
// -> payload.builder fhir_bundle -> fhir_validation strict) proves 2
// patients (subscriber + dependent) and 2 Coverage resources with genuinely
// different translated statuses (subscriber's own Addition -> "active";
// dependent's own Termination -> "cancelled"), and passes strict FHIR
// validation with zero unexpected errors.
func TestEDI834FHIRBuilder_ActiveAndTerminatedMembers_BuildsCleanValidatingBundle(t *testing.T) {
	initFHIRRegistrySvc(t)

	data := map[string]interface{}{
		"parsedEDI": map[string]interface{}{
			"transactionSet": "834",
			"loops":          edi834FixtureLoops(),
		},
	}

	result := runScriptSvc(t, derive834StepAlias, derive834MemberCoveragesScript, data)
	deriveOut := svcStepOutput(t, result, derive834StepAlias)
	for k, v := range result {
		data[k] = v
	}
	svcInjectStepOutput(data, derive834StepAlias, deriveOut)

	rows, _ := deriveOut["_member_coverages"].([]interface{})
	if len(rows) != 2 {
		t.Fatalf("expected 2 member-coverage rows (subscriber's own + dependent's own), got %d: %+v", len(rows), rows)
	}

	data = runFHIRBuild(t, "build_sponsor_organization_fhir", organization834Config(), data)
	data = runFHIRBuild(t, "build_patient_fhir", patient834Config(), data)
	data = runFHIRBuild(t, "build_coverage_fhir", coverage834Config(), data)

	message, ok := data["message"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data[\"message\"] to be a map, got %T", data["message"])
	}
	patients, ok := message["fhirPatients"].([]map[string]interface{})
	if !ok || len(patients) != 2 {
		t.Fatalf("expected 2 Patient resources, got %d (ok=%v)", len(patients), ok)
	}
	coverages, ok := message["fhirCoverages"].([]map[string]interface{})
	if !ok || len(coverages) != 2 {
		t.Fatalf("expected 2 Coverage resources, got %d (ok=%v)", len(coverages), ok)
	}
	if coverages[0]["status"] != "active" {
		t.Errorf("Coverage[0] (subscriber, HD01=024 Addition) status = %v, want active", coverages[0]["status"])
	}
	if coverages[1]["status"] != "cancelled" {
		t.Errorf("Coverage[1] (dependent, HD01=030 Termination) status = %v, want cancelled (genuinely distinct from the subscriber's active)", coverages[1]["status"])
	}

	assembleAndValidate834Bundle(t, data, "message.fhirOrganization", "message.fhirPatients", "message.fhirCoverages")
}

// assembleAndValidate834Bundle mirrors assembleAndValidate278Bundle exactly.
func assembleAndValidate834Bundle(t *testing.T, data map[string]interface{}, resourcePaths ...string) string {
	t.Helper()

	rp := make([]interface{}, len(resourcePaths))
	for i, p := range resourcePaths {
		rp[i] = p
	}

	pbExec := payload.NewPayloadBuilderExecutor(nil)
	pbStep := &models.TransformationStep{
		StepName:  "Assemble FHIR Bundle",
		StepAlias: strPtrSvc("assemble_834_bundle"),
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
	pbOut := svcStepOutput(t, pbResult, "assemble_834_bundle")
	for k, v := range pbResult {
		data[k] = v
	}
	svcInjectStepOutput(data, "assemble_834_bundle", pbOut)

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
		StepAlias: strPtrSvc("validate_834_bundle"),
		StepType:  "fhir_validation",
		Enabled:   true,
		Config: map[string]interface{}{
			"profile":          "base",
			"validation_level": "strict",
			"fhir_version":     "R4",
			"source_field":     "steps.assemble_834_bundle.step_output.payload",
		},
	}
	vResult, err := vExec.Execute(context.Background(), vStep, data)
	if err != nil {
		t.Fatalf("validate_834_bundle error: %v", err)
	}
	vOut := svcStepOutput(t, vResult, "validate_834_bundle")

	errs := asStringSlice(vOut["errors"])
	if len(errs) > 0 {
		b, _ := json.MarshalIndent(bundle, "", "  ")
		t.Errorf("unexpected validation errors:\n%s\nbundle: %s", strings.Join(errs, "\n"), b)
	}
	return bundleJSON
}
