package services

// ─────────────────────────────────────────────────────────────────────────────
// Da Vinci PAS Template — Zone 2 rebuild on fhir.build + payload.builder
//
// Proves the replacement step chain (derive -> 5x fhir.build -> payload.builder
// fhir_bundle -> stamp profile -> fhir_validation strict) BEFORE it's
// transcribed into database/migrations/V212. Config here is the source of
// truth; V212's SQL must match it exactly, not be independently re-authored
// (independent re-authoring of the same config twice is exactly how bugs
// 1/2/5/6 happened earlier in this template's history).
//
// Run: go test ./services/ -v -run TestPASFHIRBuilder
// ─────────────────────────────────────────────────────────────────────────────

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"ezhealthkonnect/fhir/r4"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors/enrichment"
	"ezhealthkonnect/services/executors/payload"
	"ezhealthkonnect/services/executors/transform"
	"ezhealthkonnect/services/executors/validation"
)

// testFHIRSchemaDirSvc mirrors fhir/r4/r4_test.go's testSchemaDir, adjusted
// for this package's depth (services/ -> repo root is 1 level up).
const testFHIRSchemaDirSvc = "../schemas/fhir"

func initFHIRRegistrySvc(t *testing.T) {
	t.Helper()
	if err := r4.ForceReinit(testFHIRSchemaDirSvc); err != nil {
		t.Fatalf("r4.ForceReinit failed: %v", err)
	}
	if r4.GetRegistry() == nil {
		t.Fatal("expected non-nil registry after ForceReinit")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Fixtures
// ─────────────────────────────────────────────────────────────────────────────

// pasFHIRBuilderInput mirrors the REAL shape a fixed FieldMappingExecutor
// (see field_mapping_executor.go's own SetNestedValue fix) actually produces
// once its output is captured into a later step's "steps.map_to_pas_envelope.step_output"
// snapshot and passed through models.OutputNormalizer.NormalizeStepOutput --
// which snake_cases every camelCase key it finds (firstName -> first_name,
// memberId -> member_id, etc; single-word keys like npi/dob/gender/urgency
// are already snake_case and pass through unchanged). Previously this
// fixture hand-built a flat, ALREADY-camelCase, ALREADY-top-level
// "_pas_envelope" map directly -- convenient for the test, but it meant the
// test could never have caught the real bug (LHS-as-literal-key
// field_mapping output, unreachable except via steps.<alias>.step_output,
// and mangled by normalization once there) found via a real browser Test
// Pipeline run against the actual Da Vinci PAS template (2026-09). Provider
// uses separate first_name/last_name fields (NOT a single combined "name"),
// matching the REAL, currently-deployed shape: V91's original mapping
// config collected one combined "_pas_envelope.provider.name" field, but
// V212's own migration SQL (see its own header comment and its jsonb_set
// patch to the "pas_envelope" step) already REPLACES that with two separate
// "_pas_envelope.provider.firstName"/"lastName" mappings, mirroring how
// patient.firstName/.lastName already work -- confirmed by reading V212's
// full patch, not assumed from V91 alone.
func pasFHIRBuilderInput() map[string]interface{} {
	return map[string]interface{}{
		"steps": map[string]interface{}{
			"map_to_pas_envelope": map[string]interface{}{
				"step_output": map[string]interface{}{
					"_pas_envelope": map[string]interface{}{
						"patient": map[string]interface{}{
							"first_name": "Jane",
							"last_name":  "Smith",
							"dob":        "1975-03-22",
							"gender":     "female",
							"member_id":  "MEM-00123",
						},
						"coverage": map[string]interface{}{
							"payer_id":     "1234567890",
							"plan_id":      "GOLD-PPO",
							"group_number": "GRP-999",
						},
						"provider": map[string]interface{}{
							"npi":          "9876543210",
							"first_name":   "Alice",
							"last_name":    "Johnson",
							"facility_npi": "1111111111",
						},
						"request": map[string]interface{}{
							"service_code":       "99213",
							"diagnosis_codes":    []interface{}{"Z00.00", "J06.9"},
							"urgency":            "routine",
							"service_start_date": "2026-05-01",
							"service_end_date":   "2026-05-01",
							"quantity":           "1",
						},
					},
				},
			},
		},
	}
}

const derivePASFieldsScript = `
// -- Derive PAS Computed Fields --
// Pure data derivation only -- no FHIR resource shape or profile knowledge.
//
// Zone 1 (field_mapping/pas_envelope_mapping, step_alias "pas_envelope",
// step_name "Map to PAS Envelope") writes its own output ONLY into
// inputData["_stepOutput"] (BaseExecutor.SetStepOutputWithDetails) --
// executeStepWithContext extracts that into this step's own "input.steps.<X>.step_output"
// snapshot and DELETES "_stepOutput" from what actually reaches a later
// step, so it is NEVER reachable at input._pas_envelope directly (the
// pre-fix assumption this script made). The snapshot key "<X>" is the
// NORMALIZED STEP NAME ("Map to PAS Envelope" -> "map_to_pas_envelope"),
// NOT the step_alias ("pas_envelope") -- those two happen to be the same
// string for this template's own Zone 2 steps (a naming convention, not a
// guarantee) but genuinely differ for Zone 1, so this is not interchangeable
// with the alias. Bare input._pas_envelope is kept as a fallback purely for
// any Go-level test harness that constructs data flat, without the real
// "steps" wrapper.
var env = (input.steps && input.steps.map_to_pas_envelope && input.steps.map_to_pas_envelope.step_output && input.steps.map_to_pas_envelope.step_output._pas_envelope) || input._pas_envelope || {};
var pat = env.patient || {};
var cov = env.coverage || {};
var req = env.request || {};
var prov = env.provider || {};

var gender = (pat.gender || "unknown").toLowerCase();
if (gender !== "male" && gender !== "female" && gender !== "other" && gender !== "unknown") {
  gender = "unknown";
}

var priority = (req.urgency === "urgent") ? "stat" : "normal";

var diagCodes = req.diagnosis_codes || [];
if (typeof diagCodes === "string") {
  try { diagCodes = JSON.parse(diagCodes); } catch (e) { diagCodes = [diagCodes]; }
}
var diagnosisCodes = diagCodes.map(function(code) { return { code: String(code) }; });

var today = new Date().toISOString().substring(0, 10);
var serviceStartDate = req.service_start_date || today;
var serviceEndDate = req.service_end_date || serviceStartDate;

var quantity = req.quantity ? Number(req.quantity) : 1;

var coverageClasses = [];
if (cov.plan_id) {
  coverageClasses.push({ type: "plan", value: cov.plan_id, name: "Health Plan" });
}
if (cov.group_number) {
  coverageClasses.push({ type: "group", value: cov.group_number, name: "" });
}

var ts = new Date().toISOString();
var claimId = "claim-" + Date.now();
var bundleId = "pas-bundle-" + Date.now();

// Full name fallback for Organization.name when no explicit organization
// name is mapped (V91/V212's own guided config has no "organizationName"
// mapping at all) -- provider.firstName/lastName are separate, real fields
// (see pasFHIRBuilderInput's own doc comment for why), combined here the
// same way the pre-existing script always did.
var providerFullName = ((prov.first_name || "") + " " + (prov.last_name || "")).trim();

return ({
  _pas_derived: {
    gender: gender,
    priority: priority,
    diagnosis_codes: diagnosisCodes,
    service_start_date: serviceStartDate,
    service_end_date: serviceEndDate,
    quantity: quantity,
    coverage_classes: coverageClasses,
    claim_id: claimId,
    bundle_id: bundleId,
    created_at: ts,
    provider_full_name: providerFullName
  }
});
`

// Returns the bundle under the key "fhirBundle", NOT "pas_bundle" -- a real
// bug found via a real browser Test Pipeline run against the Da Vinci PAS
// template (2026-09): models.OutputNormalizer.NormalizeStepOutput recursively
// snake_cases EVERY key in a step's returned object except a small, explicit
// preserve-list ("result", "fhirBundle", "fhirResource"/"fhir_resource") --
// any OTHER key name, including the original "pas_bundle", has its ENTIRE
// nested content mangled (resourceType -> resource_type,
// diagnosisCodeableConcept -> diagnosis_codeable_concept, etc.), silently
// corrupting the real FHIR Bundle this step produces. "fhirBundle" is the
// established, already-documented convention for exactly this situation (a
// step returning a real FHIR resource tree whose internal field names must
// stay camelCase) -- reusing it here is the correct fix, not a new special
// case. This Go-level test bypasses NormalizeStepOutput entirely (see
// stepOutput()/svcInjectStepOutput() below), so it could never have caught
// this on its own; only a real end-to-end run through the actual pipeline
// engine (which DOES normalize) surfaced it.
const stampBundleProfileScript = `
var bundle = JSON.parse(input.steps.assemble_pas_bundle.step_output.payload);
bundle.meta = { profile: ["http://hl7.org/fhir/us/davinci-pas/StructureDefinition/profile-pas-request-bundle"] };
return ({ fhirBundle: bundle });
`

// ─────────────────────────────────────────────────────────────────────────────
// fhir.build configs -- these are transcribed verbatim into V212's SQL.
//
// Every sourcePath/rowsPath/fallbackPaths reference below is prefixed with
// "steps.<normalized-step-name>.step_output." -- a bare "_pas_envelope.*"/
// "_pas_derived.*" (what this file used before) resolves to nothing against
// the real pipeline engine, for the same reason documented on
// derivePASFieldsScript's own "var env = ..." line: a prior enrichment.script
// or field_mapping step's own returned/mapped fields are NEVER merged onto
// plain inputData, only captured into that one step's own
// "steps.<X>.step_output" snapshot. Field segments below are snake_case,
// matching what models.OutputNormalizer.NormalizeStepOutput actually
// produces for every camelCase key inside that snapshot.
// ─────────────────────────────────────────────────────────────────────────────

const pasEnvelopeStepPath = "steps.map_to_pas_envelope.step_output._pas_envelope."
const pasDerivedStepPath = "steps.derive_pas_computed_fields.step_output._pas_derived."

func patientBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Patient",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirPatient",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": pasEnvelopeStepPath + "patient.member_id"},
			map[string]interface{}{"targetPath": "meta.profile[0]", "literalValue": "http://hl7.org/fhir/us/davinci-pas/StructureDefinition/profile-subscriber"},
			map[string]interface{}{"targetPath": "identifier[0].type.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/v2-0203"},
			map[string]interface{}{"targetPath": "identifier[0].type.coding[0].code", "literalValue": "MB"},
			map[string]interface{}{"targetPath": "identifier[0].type.coding[0].display", "literalValue": "Member Number"},
			map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "urn:oid:2.16.840.1.113883.4.6"},
			map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": pasEnvelopeStepPath + "patient.member_id"},
			map[string]interface{}{"targetPath": "name[0].use", "literalValue": "official"},
			map[string]interface{}{"targetPath": "name[0].family", "sourcePath": pasEnvelopeStepPath + "patient.last_name"},
			map[string]interface{}{"targetPath": "name[0].given[0]", "sourcePath": pasEnvelopeStepPath + "patient.first_name"},
			map[string]interface{}{"targetPath": "birthDate", "sourcePath": pasEnvelopeStepPath + "patient.dob"},
			map[string]interface{}{"targetPath": "gender", "sourcePath": pasDerivedStepPath + "gender"},
		},
	}
}

func coverageBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Coverage",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirCoverage",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "literalValue": "coverage-1"},
			map[string]interface{}{"targetPath": "meta.profile[0]", "literalValue": "http://hl7.org/fhir/us/davinci-pas/StructureDefinition/profile-coverage"},
			map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://hl7.org/fhir/sid/us-npi"},
			map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": pasEnvelopeStepPath + "coverage.payer_id"},
			map[string]interface{}{"targetPath": "status", "literalValue": "active"},
			map[string]interface{}{"targetPath": "subscriber.reference", "sourcePath": pasEnvelopeStepPath + "patient.member_id", "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Patient/"}},
			map[string]interface{}{"targetPath": "subscriberId", "sourcePath": pasEnvelopeStepPath + "patient.member_id"},
			map[string]interface{}{"targetPath": "beneficiary.reference", "sourcePath": pasEnvelopeStepPath + "patient.member_id", "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Patient/"}},
			map[string]interface{}{"targetPath": "relationship.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/subscriber-relationship"},
			map[string]interface{}{"targetPath": "relationship.coding[0].code", "literalValue": "self"},
			map[string]interface{}{"targetPath": "payor[0].identifier.system", "literalValue": "http://hl7.org/fhir/sid/us-npi"},
			map[string]interface{}{"targetPath": "payor[0].identifier.value", "sourcePath": pasEnvelopeStepPath + "coverage.payer_id"},
		},
		"repeatingGroups": []interface{}{
			map[string]interface{}{
				"targetPath": "class",
				"rowsPath":   pasDerivedStepPath + "coverage_classes",
				"fields": []interface{}{
					map[string]interface{}{"targetPath": "type.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/coverage-class"},
					map[string]interface{}{"targetPath": "type.coding[0].code", "sourcePath": "type"},
					map[string]interface{}{"targetPath": "value", "sourcePath": "value"},
					map[string]interface{}{"targetPath": "name", "sourcePath": "name"},
				},
			},
		},
	}
}

func practitionerBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Practitioner",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirPractitioner",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "literalValue": "practitioner-1"},
			map[string]interface{}{"targetPath": "meta.profile[0]", "literalValue": "http://hl7.org/fhir/us/davinci-pas/StructureDefinition/profile-practitioner"},
			map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://hl7.org/fhir/sid/us-npi"},
			map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": pasEnvelopeStepPath + "provider.npi"},
			// V212's own migration SQL patches Zone 1 to collect separate
			// firstName/lastName mappings (mirroring patient's own pattern),
			// replacing V91's original single combined "name" field -- see
			// pasFHIRBuilderInput's own doc comment.
			map[string]interface{}{"targetPath": "name[0].family", "sourcePath": pasEnvelopeStepPath + "provider.last_name"},
			map[string]interface{}{"targetPath": "name[0].given[0]", "sourcePath": pasEnvelopeStepPath + "provider.first_name"},
		},
	}
}

func organizationBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Organization",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirOrganization",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "literalValue": "organization-1"},
			map[string]interface{}{"targetPath": "meta.profile[0]", "literalValue": "http://hl7.org/fhir/us/davinci-pas/StructureDefinition/profile-requestor"},
			map[string]interface{}{"targetPath": "active", "literalValue": "true"},
			map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": pasEnvelopeStepPath + "provider.facility_npi", "fallbackPaths": []interface{}{pasEnvelopeStepPath + "provider.npi"}},
			map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://hl7.org/fhir/sid/us-npi"},
			// organization_name is not currently collected by V91's own
			// mapping config (no such lhs is defined there) -- kept as the
			// first-tier fallback so a future guided-mapping addition would
			// work without any further config change here; falls through to
			// the derived (split-from-provider.name) full name, then a
			// final generic literal.
			map[string]interface{}{"targetPath": "name", "sourcePath": pasEnvelopeStepPath + "provider.organization_name", "fallbackPaths": []interface{}{pasDerivedStepPath + "provider_full_name"}, "literalValue": "Requesting Organization"},
		},
	}
}

func claimBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Claim",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirClaim",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": pasDerivedStepPath + "claim_id"},
			map[string]interface{}{"targetPath": "meta.profile[0]", "literalValue": "http://hl7.org/fhir/us/davinci-pas/StructureDefinition/profile-claim"},
			map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "urn:ietf:rfc:3986"},
			map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": pasDerivedStepPath + "claim_id"},
			map[string]interface{}{"targetPath": "status", "literalValue": "active"},
			map[string]interface{}{"targetPath": "type.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/claim-type"},
			map[string]interface{}{"targetPath": "type.coding[0].code", "literalValue": "professional"},
			map[string]interface{}{"targetPath": "use", "literalValue": "preauthorization"},
			map[string]interface{}{"targetPath": "patient.reference", "sourcePath": pasEnvelopeStepPath + "patient.member_id", "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Patient/"}},
			map[string]interface{}{"targetPath": "created", "sourcePath": pasDerivedStepPath + "created_at"},
			map[string]interface{}{"targetPath": "insurer.identifier.system", "literalValue": "http://hl7.org/fhir/sid/us-npi"},
			map[string]interface{}{"targetPath": "insurer.identifier.value", "sourcePath": pasEnvelopeStepPath + "coverage.payer_id"},
			map[string]interface{}{"targetPath": "provider.reference", "literalValue": "Organization/organization-1"},
			map[string]interface{}{"targetPath": "priority.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/processpriority"},
			map[string]interface{}{"targetPath": "priority.coding[0].code", "sourcePath": pasDerivedStepPath + "priority"},
			map[string]interface{}{"targetPath": "insurance[0].sequence", "literalValue": "1"},
			map[string]interface{}{"targetPath": "insurance[0].focal", "literalValue": "true"},
			map[string]interface{}{"targetPath": "insurance[0].coverage.reference", "literalValue": "Coverage/coverage-1"},
			map[string]interface{}{"targetPath": "item[0].sequence", "literalValue": "1"},
			map[string]interface{}{"targetPath": "item[0].extension[0].url", "literalValue": "http://hl7.org/fhir/us/davinci-pas/StructureDefinition/extension-serviceItemRequestedDate"},
			map[string]interface{}{"targetPath": "item[0].extension[0].valuePeriod.start", "sourcePath": pasDerivedStepPath + "service_start_date"},
			map[string]interface{}{"targetPath": "item[0].extension[0].valuePeriod.end", "sourcePath": pasDerivedStepPath + "service_end_date"},
			map[string]interface{}{"targetPath": "item[0].category.coding[0].system", "literalValue": "https://codesystem.x12.org/005010/1365"},
			map[string]interface{}{"targetPath": "item[0].category.coding[0].code", "literalValue": "1"},
			map[string]interface{}{"targetPath": "item[0].category.coding[0].display", "literalValue": "Medical Care"},
			map[string]interface{}{"targetPath": "item[0].productOrService.coding[0].system", "literalValue": "http://www.ama-assn.org/go/cpt"},
			map[string]interface{}{"targetPath": "item[0].productOrService.coding[0].code", "sourcePath": pasEnvelopeStepPath + "request.service_code"},
			map[string]interface{}{"targetPath": "item[0].quantity.value", "sourcePath": pasDerivedStepPath + "quantity", "transform": "cda_decimal_string_to_number"},
			map[string]interface{}{"targetPath": "item[0].provider.reference", "literalValue": "Practitioner/practitioner-1"},
		},
		"repeatingGroups": []interface{}{
			map[string]interface{}{
				"targetPath": "diagnosis",
				"rowsPath":   pasDerivedStepPath + "diagnosis_codes",
				"fields": []interface{}{
					map[string]interface{}{"targetPath": "diagnosisCodeableConcept.coding[0].system", "literalValue": "http://hl7.org/fhir/sid/icd-10-cm"},
					map[string]interface{}{"targetPath": "diagnosisCodeableConcept.coding[0].code", "sourcePath": "code"},
				},
			},
		},
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Test helpers
// ─────────────────────────────────────────────────────────────────────────────

func runFHIRBuild(t *testing.T, alias string, config map[string]interface{}, data map[string]interface{}) map[string]interface{} {
	t.Helper()
	exec := transform.NewFHIRBuildExecutor()
	step := &models.TransformationStep{
		StepName:  alias,
		StepAlias: strPtrSvc(alias),
		StepType:  "fhir.build",
		Enabled:   true,
		Config:    config,
	}
	result, err := exec.Execute(context.Background(), step, data)
	if err != nil {
		t.Fatalf("[%s] fhir.build error: %v", alias, err)
	}
	so, _ := result["_stepOutput"].(map[string]interface{})
	svcInjectStepOutput(result, alias, so)
	return result
}

func strPtrSvc(s string) *string { return &s }

func svcStepOutput(t *testing.T, result map[string]interface{}, label string) map[string]interface{} {
	t.Helper()
	out, ok := result["_stepOutput"].(map[string]interface{})
	if !ok {
		t.Fatalf("[%s] _stepOutput missing or not a map", label)
	}
	return out
}

func svcInjectStepOutput(data map[string]interface{}, alias string, output map[string]interface{}) {
	steps, ok := data["steps"].(map[string]interface{})
	if !ok {
		steps = map[string]interface{}{}
		data["steps"] = steps
	}
	steps[alias] = map[string]interface{}{"step_output": output}
}

func runScriptSvc(t *testing.T, alias, script string, input map[string]interface{}) map[string]interface{} {
	t.Helper()
	exec := enrichment.NewScriptEnrichmentExecutor()
	step := &models.TransformationStep{
		StepName:  alias,
		StepAlias: strPtrSvc(alias),
		StepType:  "enrichment.script",
		Enabled:   true,
		Config:    map[string]interface{}{"script": script},
	}
	result, err := exec.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("[%s] script error: %v", alias, err)
	}
	return result
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-PAS-FB-001: full chain builds a Bundle that validates clean (zero errors,
// not just zero PAS-constraint errors) -- the acid test that this redesign
// structurally fixes the fullUrl/reference-form bug, not just relocates it.
// ─────────────────────────────────────────────────────────────────────────────
func TestPASFHIRBuilder_FullChain_BuildsCleanValidatingBundle(t *testing.T) {
	initFHIRRegistrySvc(t)
	data := pasFHIRBuilderInput()

	// Derive. Step alias here is "derive_pas_computed_fields" -- the
	// NORMALIZED STEP NAME ("Derive PAS Computed Fields"), matching what a
	// real pipeline run actually keys "steps.<X>.step_output" by (see
	// derivePASFieldsScript's own doc comment) -- NOT V212's real, shorter
	// step_alias "derive_pas_fields", which is a different string.
	result := runScriptSvc(t, "derive_pas_computed_fields", derivePASFieldsScript, data)
	deriveOut := svcStepOutput(t, result, "derive_pas_computed_fields")
	for k, v := range result {
		data[k] = v
	}
	svcInjectStepOutput(data, "derive_pas_computed_fields", deriveOut)

	// 5x fhir.build
	data = runFHIRBuild(t, "build_patient_fhir", patientBuildConfig(), data)
	data = runFHIRBuild(t, "build_coverage_fhir", coverageBuildConfig(), data)
	data = runFHIRBuild(t, "build_practitioner_fhir", practitionerBuildConfig(), data)
	data = runFHIRBuild(t, "build_organization_fhir", organizationBuildConfig(), data)
	data = runFHIRBuild(t, "build_claim_fhir", claimBuildConfig(), data)

	// payload.builder fhir_bundle
	pbExec := payload.NewPayloadBuilderExecutor(nil)
	pbStep := &models.TransformationStep{
		StepName:  "Assemble PAS Bundle",
		StepAlias: strPtrSvc("assemble_pas_bundle"),
		StepType:  "payload.builder",
		Enabled:   true,
		Config: map[string]interface{}{
			"mode": "fhir_bundle",
			"fhirBundle": map[string]interface{}{
				"bundleType": "collection",
				"resourcePaths": []interface{}{
					"message.fhirClaim", "message.fhirPatient", "message.fhirCoverage",
					"message.fhirPractitioner", "message.fhirOrganization",
				},
			},
		},
	}
	pbResult, err := pbExec.Execute(context.Background(), pbStep, data)
	if err != nil {
		t.Fatalf("payload.builder error: %v", err)
	}
	pbOut := svcStepOutput(t, pbResult, "assemble_pas_bundle")
	for k, v := range pbResult {
		data[k] = v
	}
	svcInjectStepOutput(data, "assemble_pas_bundle", pbOut)

	// Stamp bundle profile (also converts JSON string -> map for downstream)
	result = runScriptSvc(t, "stamp_bundle_profile", stampBundleProfileScript, data)
	stampOut := svcStepOutput(t, result, "stamp_bundle_profile")
	for k, v := range result {
		data[k] = v
	}
	svcInjectStepOutput(data, "stamp_bundle_profile", stampOut)

	pasBundle, ok := stampOut["fhirBundle"].(map[string]interface{})
	if !ok {
		t.Fatalf("stamp_bundle_profile did not produce a fhirBundle map: %+v", stampOut)
	}
	if pasBundle["resourceType"] != "Bundle" {
		t.Fatalf("expected resourceType=Bundle, got %v", pasBundle["resourceType"])
	}

	// fhir_validation at strict level, source_field pointing at the stamped bundle
	vExec := validation.NewFHIRValidationExecutor()
	vStep := &models.TransformationStep{
		StepName:  "Validate PAS Bundle",
		StepAlias: strPtrSvc("validate_pas_bundle"),
		StepType:  "fhir_validation",
		Enabled:   true,
		Config: map[string]interface{}{
			"profile":             "davinci-pas",
			"validation_level":    "strict",
			"source_field":        "steps.stamp_bundle_profile.step_output.fhirBundle",
			"fail_on_error":       false,
			"required_resources":  []interface{}{"Claim", "Patient", "Coverage"},
		},
	}
	vResult, err := vExec.Execute(context.Background(), vStep, data)
	if err != nil {
		t.Fatalf("validate_pas_bundle error: %v", err)
	}
	vOut := svcStepOutput(t, vResult, "validate_pas_bundle")

	// The acid test: zero wrong-reference-form errors -- the specific defect
	// (bundle entries' urn:uuid: fullUrls not matching internal
	// ResourceType/id references) that motivated rebuilding Zone 2 on
	// fhir.build + payload.builder's AssembleEntries in the first place.
	errs := asStringSlice(vOut["errors"])
	var unexpected []string
	for _, s := range errs {
		// KNOWN, OUT-OF-SCOPE, VERIFIED-REAL gap: schemas/fhir/R4/valuesets/
		// ClaimTypes.gz is compiled with "codes": [] and "fetchFailed": true
		// (confirmed by directly inspecting the file -- a build_fhir_schemas.py
		// data gap, unrelated to this template). Any Claim.type code will
		// spuriously fail "invalid-code" until that ValueSet is recompiled
		// with real data -- reproduced on a stable Docker daemon, so this is
		// not transient flakiness. Tolerated explicitly, not silently: every
		// OTHER error is still required to be absent.
		if strings.Contains(s, "invalid-code") && strings.Contains(s, "Claim.type") {
			continue
		}
		unexpected = append(unexpected, s)
	}
	if len(unexpected) > 0 {
		b, _ := json.MarshalIndent(pasBundle, "", "  ")
		t.Errorf("unexpected validation errors beyond the known ClaimTypes ValueSet gap:\n%s\nbundle: %s",
			strings.Join(unexpected, "\n"), b)
	}
}

// asStringSlice handles both raw []string (calling an executor's Execute()
// directly, as this test does -- no JSON round-trip) and []interface{}
// (the shape after going through encoding/json, as the real pipeline engine
// and any HTTP-based test would see it). A plain `.([]interface{})` type
// assertion silently fails (ok=false, nil result) on a genuine []string --
// which is what fhir_validation_executor.go's buildOutput actually stores in
// variables["errors"] -- so using that assertion directly here would make
// every check against the error list a silent no-op instead of a real test.
func asStringSlice(v interface{}) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}
