// services/executors/transform/fhir_build_executor_test.go
package transform

import (
	"context"
	"testing"

	"ezhealthkonnect/fhir/r4"
	"ezhealthkonnect/models"
)

// testFHIRSchemaDir mirrors fhir/r4/r4_test.go's testSchemaDir, adjusted for
// this package's depth (services/executors/transform -> repo root is 3 levels up).
const testFHIRSchemaDir = "../../../schemas/fhir"

func initFHIRRegistry(t *testing.T) {
	t.Helper()
	if err := r4.ForceReinit(testFHIRSchemaDir); err != nil {
		t.Fatalf("r4.ForceReinit failed: %v", err)
	}
	if r4.GetRegistry() == nil {
		t.Fatal("expected non-nil registry after ForceReinit")
	}
}

func runFHIRBuild(t *testing.T, config map[string]interface{}, inputData map[string]interface{}) map[string]interface{} {
	t.Helper()
	executor := NewFHIRBuildExecutor()
	step := &models.TransformationStep{
		StepName: "Test FHIR Build",
		StepType: "fhir.build",
		Enabled:  true,
		Config:   config,
	}
	output, err := executor.Execute(context.Background(), step, inputData)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	return output
}

func fhirResourceFrom(t *testing.T, output map[string]interface{}, field string) map[string]interface{} {
	t.Helper()
	res, ok := output[field].(map[string]interface{})
	if !ok {
		t.Fatalf("expected output[%q] to be a map, got %T", field, output[field])
	}
	return res
}

// TestFHIRBuild_ScalarFields_CSVLikeRow verifies flat scalar fields resolve
// from a CSV-shaped row with no transform.
func TestFHIRBuild_ScalarFields_CSVLikeRow(t *testing.T) {
	initFHIRRegistry(t)
	config := map[string]interface{}{
		"resourceType": "Patient",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "birthDate", "sourcePath": "dob"},
			map[string]interface{}{"targetPath": "gender", "sourcePath": "sex"},
		},
	}
	inputData := map[string]interface{}{"dob": "1980-05-20", "sex": "female"}

	output := runFHIRBuild(t, config, inputData)
	resource := fhirResourceFrom(t, output, "fhirResource")

	if got := resource["resourceType"]; got != "Patient" {
		t.Errorf("resourceType = %v, want Patient", got)
	}
	if got := resource["birthDate"]; got != "1980-05-20" {
		t.Errorf("birthDate = %v, want 1980-05-20", got)
	}
	if got := resource["gender"]; got != "female" {
		t.Errorf("gender = %v, want female", got)
	}
}

// TestFHIRBuild_ValueMapViaStringDirect verifies a "string_direct" transform
// applies ValueMap translation to a raw source value — the pure scalar
// translation case (a source status column that doesn't match FHIR's own
// vocabulary).
func TestFHIRBuild_ValueMapViaStringDirect(t *testing.T) {
	initFHIRRegistry(t)
	config := map[string]interface{}{
		"resourceType": "Patient",
		"fields": []interface{}{
			map[string]interface{}{
				"targetPath": "gender",
				"sourcePath": "sexCode",
				"transform":  "string_direct",
				"valueMap":   map[string]interface{}{"M": "male", "F": "female"},
			},
		},
	}
	inputData := map[string]interface{}{"sexCode": "F"}

	output := runFHIRBuild(t, config, inputData)
	resource := fhirResourceFrom(t, output, "fhirResource")

	if got := resource["gender"]; got != "female" {
		t.Errorf("gender = %v, want female (translated from F via valueMap through string_direct)", got)
	}
}

// TestFHIRBuild_NamedTransform_DecimalStringToNumber verifies a
// DeclarativeTransformRegistry transform that operates on a bare scalar
// (cda_decimal_string_to_number) is reachable from a non-CDA source.
func TestFHIRBuild_NamedTransform_DecimalStringToNumber(t *testing.T) {
	initFHIRRegistry(t)
	config := map[string]interface{}{
		"resourceType": "Observation",
		"fields": []interface{}{
			map[string]interface{}{
				"targetPath": "valueQuantity.value",
				"sourcePath": "result",
				"transform":  "cda_decimal_string_to_number",
			},
		},
	}
	inputData := map[string]interface{}{"result": "98.6"}

	output := runFHIRBuild(t, config, inputData)
	resource := fhirResourceFrom(t, output, "fhirResource")

	vq, ok := resource["valueQuantity"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected valueQuantity to be a map, got %T", resource["valueQuantity"])
	}
	if got, ok := vq["value"].(float64); !ok || got != 98.6 {
		t.Errorf("valueQuantity.value = %v (%T), want 98.6 (float64)", vq["value"], vq["value"])
	}
}

// TestFHIRBuild_FallbackPaths_SecondPathUsedWhenFirstAbsent mirrors
// map_to_canonical_executor_test.go's equivalent test for the shared
// fallback-chain convention.
func TestFHIRBuild_FallbackPaths_SecondPathUsedWhenFirstAbsent(t *testing.T) {
	initFHIRRegistry(t)
	config := map[string]interface{}{
		"resourceType": "Patient",
		"fields": []interface{}{
			map[string]interface{}{
				"targetPath":    "birthDate",
				"sourcePath":    "dateOfBirth",
				"fallbackPaths": []interface{}{"dob"},
			},
		},
	}
	inputData := map[string]interface{}{"dob": "1975-01-01"} // no dateOfBirth

	output := runFHIRBuild(t, config, inputData)
	resource := fhirResourceFrom(t, output, "fhirResource")

	if got := resource["birthDate"]; got != "1975-01-01" {
		t.Errorf("birthDate = %v, want 1975-01-01 (from fallbackPaths)", got)
	}
}

// TestFHIRBuild_LiteralValue_UsedWhenNoPathResolves mirrors
// map_to_canonical_executor_test.go's equivalent test.
func TestFHIRBuild_LiteralValue_UsedWhenNoPathResolves(t *testing.T) {
	initFHIRRegistry(t)
	config := map[string]interface{}{
		"resourceType": "Patient",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "active", "sourcePath": "missingPath", "literalValue": "true"},
		},
	}

	output := runFHIRBuild(t, config, map[string]interface{}{})
	resource := fhirResourceFrom(t, output, "fhirResource")

	if got := resource["active"]; got != "true" {
		t.Errorf("active = %v, want true (from literalValue)", got)
	}
}

// TestFHIRBuild_RepeatingGroup_Identifier verifies a repeatingGroups entry
// builds one sub-object PER ROW with multiple fields staying aligned
// (system+value from the same row) — the misalignment risk two independent
// CollectAll passes would have, per declarative_schema.go's own documented
// rationale for its Fields-nested-under-CollectAll primitive.
func TestFHIRBuild_RepeatingGroup_Identifier(t *testing.T) {
	initFHIRRegistry(t)
	config := map[string]interface{}{
		"resourceType": "Patient",
		"repeatingGroups": []interface{}{
			map[string]interface{}{
				"targetPath": "identifier",
				"rowsPath":   "patientIdentifiers",
				"fields": []interface{}{
					map[string]interface{}{"targetPath": "system", "sourcePath": "idSystem"},
					map[string]interface{}{"targetPath": "value", "sourcePath": "idValue"},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"patientIdentifiers": []interface{}{
			map[string]interface{}{"idSystem": "http://hospital.example.org/mrn", "idValue": "12345"},
			map[string]interface{}{"idSystem": "http://hl7.org/fhir/sid/us-ssn", "idValue": "999-00-1234"},
		},
	}

	output := runFHIRBuild(t, config, inputData)
	resource := fhirResourceFrom(t, output, "fhirResource")

	idents, ok := resource["identifier"].([]interface{})
	if !ok || len(idents) != 2 {
		t.Fatalf("expected 2 identifiers, got %v (%T)", resource["identifier"], resource["identifier"])
	}
	first, ok := idents[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected identifier[0] to be a map, got %T", idents[0])
	}
	if got := first["system"]; got != "http://hospital.example.org/mrn" {
		t.Errorf("identifier[0].system = %v, want mrn system", got)
	}
	if got := first["value"]; got != "12345" {
		t.Errorf("identifier[0].value = %v, want 12345", got)
	}
	second, ok := idents[1].(map[string]interface{})
	if !ok {
		t.Fatalf("expected identifier[1] to be a map, got %T", idents[1])
	}
	if got := second["value"]; got != "999-00-1234" {
		t.Errorf("identifier[1].value = %v, want 999-00-1234 (rows must stay aligned per-identifier)", got)
	}
}

// TestFHIRBuild_UnknownResourceType_ReturnsError verifies Execute fails
// loudly (not a silently-empty resource) when resourceType/profile/version
// isn't in the compiled registry.
func TestFHIRBuild_UnknownResourceType_ReturnsError(t *testing.T) {
	initFHIRRegistry(t)
	executor := NewFHIRBuildExecutor()
	step := &models.TransformationStep{
		StepName: "Test FHIR Build",
		StepType: "fhir.build",
		Enabled:  true,
		Config:   map[string]interface{}{"resourceType": "NotARealResourceType"},
	}
	_, err := executor.Execute(context.Background(), step, map[string]interface{}{})
	if err == nil {
		t.Fatal("expected an error for an unknown resourceType, got nil")
	}
}

// TestFHIRBuild_MissingResourceType_ReturnsError verifies the required
// resourceType config key is enforced.
func TestFHIRBuild_MissingResourceType_ReturnsError(t *testing.T) {
	initFHIRRegistry(t)
	executor := NewFHIRBuildExecutor()
	step := &models.TransformationStep{
		StepName: "Test FHIR Build",
		StepType: "fhir.build",
		Enabled:  true,
		Config:   map[string]interface{}{},
	}
	_, err := executor.Execute(context.Background(), step, map[string]interface{}{})
	if err == nil {
		t.Fatal("expected an error when resourceType is missing, got nil")
	}
}

// TestFHIRBuild_CustomOutputField verifies outputField is honored.
func TestFHIRBuild_CustomOutputField(t *testing.T) {
	initFHIRRegistry(t)
	config := map[string]interface{}{
		"resourceType": "Patient",
		"outputField":  "customFHIRResource",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "birthDate", "sourcePath": "dob"},
		},
	}
	inputData := map[string]interface{}{"dob": "1990-01-01"}

	output := runFHIRBuild(t, config, inputData)
	if _, present := output["fhirResource"]; present {
		t.Errorf("expected no default fhirResource field when outputField is overridden")
	}
	resource := fhirResourceFrom(t, output, "customFHIRResource")
	if got := resource["birthDate"]; got != "1990-01-01" {
		t.Errorf("birthDate = %v, want 1990-01-01", got)
	}
}

// TestFHIRBuild_NamedTransform_StringPrefix_BuildsNestedReference verifies the
// string_prefix transform composes with SetFHIRPath's nested-path auto-object
// creation to build a real FHIR Reference string (e.g. "Patient/12345") on a
// *.reference target — the mechanism every cross-resource reference in the
// FHIR_Build_Demo pipeline relies on, since no reference-wiring automation
// exists anywhere in this pipeline (payload.builder only concatenates
// resource paths, never inspects references).
func TestFHIRBuild_NamedTransform_StringPrefix_BuildsNestedReference(t *testing.T) {
	initFHIRRegistry(t)
	config := map[string]interface{}{
		"resourceType": "Encounter",
		"fields": []interface{}{
			map[string]interface{}{
				"targetPath": "subject.reference",
				"sourcePath": "patientIdentifiers[0].idValue",
				"transform":  "string_prefix",
				"valueMap":   map[string]interface{}{"prefix": "Patient/"},
			},
		},
	}
	inputData := map[string]interface{}{
		"patientIdentifiers": []interface{}{
			map[string]interface{}{"idValue": "12345"},
		},
	}

	output := runFHIRBuild(t, config, inputData)
	resource := fhirResourceFrom(t, output, "fhirResource")

	subject, ok := resource["subject"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected subject to be a map, got %T", resource["subject"])
	}
	if got := subject["reference"]; got != "Patient/12345" {
		t.Errorf("subject.reference = %v, want Patient/12345", got)
	}
}

// TestFHIRBuild_Encounter_StatusClassAndSubjectReference verifies Encounter
// builds with its class Coding (not a CodeableConcept, unlike most other
// coded elements) and a subject reference wired to the same raw patient id
// field the Patient step uses for its own id — the single-source-of-truth
// convention this round adopts to avoid a second, driftable copy of the id.
func TestFHIRBuild_Encounter_StatusClassAndSubjectReference(t *testing.T) {
	initFHIRRegistry(t)
	config := map[string]interface{}{
		"resourceType": "Encounter",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "status", "literalValue": "finished"},
			map[string]interface{}{"targetPath": "class.system", "literalValue": "http://terminology.hl7.org/CodeSystem/v3-ActCode"},
			map[string]interface{}{"targetPath": "class.code", "literalValue": "AMB"},
			map[string]interface{}{
				"targetPath": "subject.reference", "sourcePath": "patientId",
				"transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Patient/"},
			},
		},
	}
	inputData := map[string]interface{}{"patientId": "12345"}

	output := runFHIRBuild(t, config, inputData)
	resource := fhirResourceFrom(t, output, "fhirResource")

	if got := resource["status"]; got != "finished" {
		t.Errorf("status = %v, want finished", got)
	}
	class, ok := resource["class"].(map[string]interface{})
	if !ok || class["code"] != "AMB" {
		t.Errorf("class = %v, want {code: AMB, ...}", resource["class"])
	}
	subject, ok := resource["subject"].(map[string]interface{})
	if !ok || subject["reference"] != "Patient/12345" {
		t.Errorf("subject.reference = %v, want Patient/12345", resource["subject"])
	}
}

// TestFHIRBuild_Condition_ClinicalStatusAndCode verifies Condition builds its
// clinicalStatus CodeableConcept and code, plus a subject reference.
func TestFHIRBuild_Condition_ClinicalStatusAndCode(t *testing.T) {
	initFHIRRegistry(t)
	config := map[string]interface{}{
		"resourceType": "Condition",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "clinicalStatus.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/condition-clinical"},
			map[string]interface{}{"targetPath": "clinicalStatus.coding[0].code", "literalValue": "active"},
			map[string]interface{}{"targetPath": "code.coding[0].system", "sourcePath": "conditionCodeSystem"},
			map[string]interface{}{"targetPath": "code.coding[0].code", "sourcePath": "conditionCode"},
			map[string]interface{}{
				"targetPath": "subject.reference", "sourcePath": "patientId",
				"transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Patient/"},
			},
		},
	}
	inputData := map[string]interface{}{
		"patientId": "12345", "conditionCode": "44054006", "conditionCodeSystem": "http://snomed.info/sct",
	}

	output := runFHIRBuild(t, config, inputData)
	resource := fhirResourceFrom(t, output, "fhirResource")

	clinicalStatus, ok := resource["clinicalStatus"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected clinicalStatus to be a map, got %T", resource["clinicalStatus"])
	}
	codings, ok := clinicalStatus["coding"].([]interface{})
	if !ok || len(codings) != 1 {
		t.Fatalf("expected clinicalStatus.coding to have 1 entry, got %v", clinicalStatus["coding"])
	}
	if coding := codings[0].(map[string]interface{}); coding["code"] != "active" {
		t.Errorf("clinicalStatus.coding[0].code = %v, want active", coding["code"])
	}
	subject, ok := resource["subject"].(map[string]interface{})
	if !ok || subject["reference"] != "Patient/12345" {
		t.Errorf("subject.reference = %v, want Patient/12345", resource["subject"])
	}
}

// TestFHIRBuild_Observation_CategoryStatusSubjectAndEncounterReference
// extends the existing valueQuantity scalar test with subject+encounter
// references, the two-reference case no existing test covers.
func TestFHIRBuild_Observation_CategoryStatusSubjectAndEncounterReference(t *testing.T) {
	initFHIRRegistry(t)
	config := map[string]interface{}{
		"resourceType": "Observation",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "status", "literalValue": "final"},
			map[string]interface{}{"targetPath": "code.coding[0].system", "literalValue": "http://loinc.org"},
			map[string]interface{}{"targetPath": "code.coding[0].code", "sourcePath": "obsCode"},
			map[string]interface{}{"targetPath": "valueQuantity.value", "sourcePath": "obsValue", "transform": "cda_decimal_string_to_number"},
			map[string]interface{}{"targetPath": "valueQuantity.unit", "sourcePath": "obsUnit"},
			map[string]interface{}{
				"targetPath": "subject.reference", "sourcePath": "patientId",
				"transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Patient/"},
			},
			map[string]interface{}{
				"targetPath": "encounter.reference", "sourcePath": "encounterId",
				"transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Encounter/"},
			},
		},
	}
	inputData := map[string]interface{}{
		"patientId": "12345", "encounterId": "enc-001",
		"obsCode": "8310-5", "obsValue": "37.2", "obsUnit": "Cel",
	}

	output := runFHIRBuild(t, config, inputData)
	resource := fhirResourceFrom(t, output, "fhirResource")

	subject, ok := resource["subject"].(map[string]interface{})
	if !ok || subject["reference"] != "Patient/12345" {
		t.Errorf("subject.reference = %v, want Patient/12345", resource["subject"])
	}
	encounter, ok := resource["encounter"].(map[string]interface{})
	if !ok || encounter["reference"] != "Encounter/enc-001" {
		t.Errorf("encounter.reference = %v, want Encounter/enc-001", resource["encounter"])
	}
}

// TestFHIRBuild_MedicationRequest_StatusAndSubjectReference verifies
// MedicationRequest builds status/intent/medicationCodeableConcept plus a
// subject reference.
func TestFHIRBuild_MedicationRequest_StatusAndSubjectReference(t *testing.T) {
	initFHIRRegistry(t)
	config := map[string]interface{}{
		"resourceType": "MedicationRequest",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "status", "literalValue": "active"},
			map[string]interface{}{"targetPath": "intent", "literalValue": "order"},
			map[string]interface{}{"targetPath": "medicationCodeableConcept.coding[0].system", "literalValue": "http://www.nlm.nih.gov/research/umls/rxnorm"},
			map[string]interface{}{"targetPath": "medicationCodeableConcept.coding[0].code", "sourcePath": "drugCode"},
			map[string]interface{}{
				"targetPath": "subject.reference", "sourcePath": "patientId",
				"transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Patient/"},
			},
		},
	}
	inputData := map[string]interface{}{"patientId": "12345", "drugCode": "197361"}

	output := runFHIRBuild(t, config, inputData)
	resource := fhirResourceFrom(t, output, "fhirResource")

	if got := resource["status"]; got != "active" {
		t.Errorf("status = %v, want active", got)
	}
	if got := resource["intent"]; got != "order" {
		t.Errorf("intent = %v, want order", got)
	}
	subject, ok := resource["subject"].(map[string]interface{})
	if !ok || subject["reference"] != "Patient/12345" {
		t.Errorf("subject.reference = %v, want Patient/12345", resource["subject"])
	}
}

// TestFHIRBuild_AllergyIntolerance_ClinicalStatusAndPatientReference verifies
// AllergyIntolerance builds correctly — note the reference field is "patient",
// not "subject" (unlike Condition/Observation/MedicationRequest).
func TestFHIRBuild_AllergyIntolerance_ClinicalStatusAndPatientReference(t *testing.T) {
	initFHIRRegistry(t)
	config := map[string]interface{}{
		"resourceType": "AllergyIntolerance",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "clinicalStatus.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/allergyintolerance-clinical"},
			map[string]interface{}{"targetPath": "clinicalStatus.coding[0].code", "literalValue": "active"},
			map[string]interface{}{"targetPath": "code.coding[0].system", "literalValue": "http://snomed.info/sct"},
			map[string]interface{}{"targetPath": "code.coding[0].code", "sourcePath": "allergenCode"},
			map[string]interface{}{
				"targetPath": "patient.reference", "sourcePath": "patientId",
				"transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Patient/"},
			},
		},
	}
	inputData := map[string]interface{}{"patientId": "12345", "allergenCode": "7980"}

	output := runFHIRBuild(t, config, inputData)
	resource := fhirResourceFrom(t, output, "fhirResource")

	patient, ok := resource["patient"].(map[string]interface{})
	if !ok || patient["reference"] != "Patient/12345" {
		t.Errorf("patient.reference = %v, want Patient/12345", resource["patient"])
	}
}

// TestFHIRBuild_Immunization_StatusAndPatientReference verifies Immunization
// builds status/vaccineCode plus a patient reference.
func TestFHIRBuild_Immunization_StatusAndPatientReference(t *testing.T) {
	initFHIRRegistry(t)
	config := map[string]interface{}{
		"resourceType": "Immunization",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "status", "literalValue": "completed"},
			map[string]interface{}{"targetPath": "vaccineCode.coding[0].system", "literalValue": "http://hl7.org/fhir/sid/cvx"},
			map[string]interface{}{"targetPath": "vaccineCode.coding[0].code", "sourcePath": "vaccineCode"},
			map[string]interface{}{
				"targetPath": "patient.reference", "sourcePath": "patientId",
				"transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Patient/"},
			},
		},
	}
	inputData := map[string]interface{}{"patientId": "12345", "vaccineCode": "141"}

	output := runFHIRBuild(t, config, inputData)
	resource := fhirResourceFrom(t, output, "fhirResource")

	if got := resource["status"]; got != "completed" {
		t.Errorf("status = %v, want completed", got)
	}
	patient, ok := resource["patient"].(map[string]interface{})
	if !ok || patient["reference"] != "Patient/12345" {
		t.Errorf("patient.reference = %v, want Patient/12345", resource["patient"])
	}
}

// TestFHIRBuild_Practitioner_NameAndIdentifier verifies Practitioner builds
// its repeating name/identifier arrays via literal bracket-index paths
// (Practitioner has no repeatingGroups need here — a single known name and
// identifier — so this exercises SetFHIRPath's array-index creation directly).
func TestFHIRBuild_Practitioner_NameAndIdentifier(t *testing.T) {
	initFHIRRegistry(t)
	config := map[string]interface{}{
		"resourceType": "Practitioner",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://hl7.org/fhir/sid/us-npi"},
			map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": "practitionerNPI"},
			map[string]interface{}{"targetPath": "name[0].family", "sourcePath": "practitionerFamily"},
			map[string]interface{}{"targetPath": "name[0].given[0]", "sourcePath": "practitionerGiven"},
		},
	}
	inputData := map[string]interface{}{
		"practitionerNPI": "1234567890", "practitionerFamily": "Smith", "practitionerGiven": "Jane",
	}

	output := runFHIRBuild(t, config, inputData)
	resource := fhirResourceFrom(t, output, "fhirResource")

	names, ok := resource["name"].([]interface{})
	if !ok || len(names) != 1 {
		t.Fatalf("expected 1 name entry, got %v", resource["name"])
	}
	name := names[0].(map[string]interface{})
	if got := name["family"]; got != "Smith" {
		t.Errorf("name[0].family = %v, want Smith", got)
	}
}

// TestFHIRBuild_Organization_NameAndIdentifier verifies Organization builds
// its plain-string name field (unlike Practitioner's repeating HumanName).
func TestFHIRBuild_Organization_NameAndIdentifier(t *testing.T) {
	initFHIRRegistry(t)
	config := map[string]interface{}{
		"resourceType": "Organization",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://hl7.org/fhir/sid/us-npi"},
			map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": "orgNPI"},
			map[string]interface{}{"targetPath": "name", "sourcePath": "orgName"},
		},
	}
	inputData := map[string]interface{}{"orgNPI": "9876543210", "orgName": "General Hospital"}

	output := runFHIRBuild(t, config, inputData)
	resource := fhirResourceFrom(t, output, "fhirResource")

	if got := resource["name"]; got != "General Hospital" {
		t.Errorf("name = %v, want General Hospital", got)
	}
}

// ===============================================================
// NESTED repeatingGroups TESTS (EDI Phase 5)
//
// Added to build ExplanationOfBenefit.item[].adjudication[] from an X12 835
// service line with no script: two fixed entries (submitted/benefit amounts)
// written as ordinary indexed fields on the item row, followed by N further
// entries built from a NESTED repeatingGroup whose rows come from CAS
// occurrences' own adjustment trios (field_utils.go's "[*]" wildcard-flatten
// path support, added alongside this) — appended after the fixed entries,
// never overwriting them.
// ===============================================================

// TestFHIRBuild_NestedRepeatingGroup_AppendsAfterFixedIndexedFields verifies
// a nested repeatingGroup's own rows are appended after entries the parent
// row's ordinary Fields already wrote at the SAME targetPath, rather than
// overwriting them — the mechanism startingIndex/applyRepeatingGroup provide.
func TestFHIRBuild_NestedRepeatingGroup_AppendsAfterFixedIndexedFields(t *testing.T) {
	initFHIRRegistry(t)
	config := map[string]interface{}{
		"resourceType": "ExplanationOfBenefit",
		"repeatingGroups": []interface{}{
			map[string]interface{}{
				"targetPath": "item",
				"rowsPath":   "items",
				"fields": []interface{}{
					// Two FIXED entries written directly via literal bracket
					// indices — the "submitted"/"benefit" adjudication entries
					// every item gets regardless of adjustments.
					map[string]interface{}{"targetPath": "adjudication[0].category.coding[0].code", "literalValue": "submitted"},
					map[string]interface{}{"targetPath": "adjudication[0].amount.value", "sourcePath": "chargeAmount", "transform": "cda_decimal_string_to_number"},
					map[string]interface{}{"targetPath": "adjudication[1].category.coding[0].code", "literalValue": "benefit"},
					map[string]interface{}{"targetPath": "adjudication[1].amount.value", "sourcePath": "paidAmount", "transform": "cda_decimal_string_to_number"},
				},
				"repeatingGroups": []interface{}{
					map[string]interface{}{
						"targetPath": "adjudication",
						"rowsPath":   "extraAdjustments", // plain rowsPath, no wildcard -- isolates the append mechanism from wildcard-flatten
						"fields": []interface{}{
							map[string]interface{}{"targetPath": "category.coding[0].code", "sourcePath": "reasonCode"},
							map[string]interface{}{"targetPath": "amount.value", "sourcePath": "amount", "transform": "cda_decimal_string_to_number"},
						},
					},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{
				"chargeAmount": "100.00",
				"paidAmount":   "80.00",
				"extraAdjustments": []interface{}{
					map[string]interface{}{"reasonCode": "45", "amount": "20.00"},
				},
			},
		},
	}

	output := runFHIRBuild(t, config, inputData)
	resource := fhirResourceFrom(t, output, "fhirResource")

	items, ok := resource["item"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("expected 1 item, got %v", resource["item"])
	}
	item := items[0].(map[string]interface{})
	adjudication, ok := item["adjudication"].([]interface{})
	if !ok || len(adjudication) != 3 {
		t.Fatalf("expected 3 adjudication entries (2 fixed + 1 appended), got %v", item["adjudication"])
	}

	first := adjudication[0].(map[string]interface{})
	if code := first["category"].(map[string]interface{})["coding"].([]interface{})[0].(map[string]interface{})["code"]; code != "submitted" {
		t.Errorf("adjudication[0].category.coding[0].code = %v, want submitted (must survive being overwritten by the nested group)", code)
	}
	second := adjudication[1].(map[string]interface{})
	if code := second["category"].(map[string]interface{})["coding"].([]interface{})[0].(map[string]interface{})["code"]; code != "benefit" {
		t.Errorf("adjudication[1].category.coding[0].code = %v, want benefit", code)
	}
	third := adjudication[2].(map[string]interface{})
	if code := third["category"].(map[string]interface{})["coding"].([]interface{})[0].(map[string]interface{})["code"]; code != "45" {
		t.Errorf("adjudication[2].category.coding[0].code = %v, want 45 (the nested group's own row, appended after the 2 fixed entries)", code)
	}
}

// TestFHIRBuild_NestedRepeatingGroup_WildcardFlattenSource is the real EDI
// Phase 5 shape: a service line with 2 CAS occurrences (2 and 1 adjustment
// trios respectively) flattens, via "CAS[*].adjustments", into 3 further
// adjudication entries appended after the 2 fixed submitted/benefit ones --
// end to end, no script, no second RowsPath lookup needed.
func TestFHIRBuild_NestedRepeatingGroup_WildcardFlattenSource(t *testing.T) {
	initFHIRRegistry(t)
	config := map[string]interface{}{
		"resourceType": "ExplanationOfBenefit",
		"repeatingGroups": []interface{}{
			map[string]interface{}{
				"targetPath": "item",
				"rowsPath":   "items",
				"fields": []interface{}{
					map[string]interface{}{"targetPath": "adjudication[0].category.coding[0].code", "literalValue": "submitted"},
					map[string]interface{}{"targetPath": "adjudication[0].amount.value", "sourcePath": "chargeAmount", "transform": "cda_decimal_string_to_number"},
					map[string]interface{}{"targetPath": "adjudication[1].category.coding[0].code", "literalValue": "benefit"},
					map[string]interface{}{"targetPath": "adjudication[1].amount.value", "sourcePath": "paidAmount", "transform": "cda_decimal_string_to_number"},
				},
				"repeatingGroups": []interface{}{
					map[string]interface{}{
						"targetPath": "adjudication",
						"rowsPath":   "CAS[*].adjustments",
						"fields": []interface{}{
							map[string]interface{}{"targetPath": "category.coding[0].code", "sourcePath": "reasonCode"},
							map[string]interface{}{"targetPath": "amount.value", "sourcePath": "amount", "transform": "cda_decimal_string_to_number"},
						},
					},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{
				"chargeAmount": "100.00",
				"paidAmount":   "80.00",
				"CAS": []interface{}{
					map[string]interface{}{
						"claimAdjustmentGroupCode": "CO",
						"adjustments": []interface{}{
							map[string]interface{}{"reasonCode": "45", "amount": "15.00"},
							map[string]interface{}{"reasonCode": "97", "amount": "5.00"},
						},
					},
					map[string]interface{}{
						"claimAdjustmentGroupCode": "PR",
						"adjustments": []interface{}{
							map[string]interface{}{"reasonCode": "1", "amount": "20.00"},
						},
					},
				},
			},
		},
	}

	output := runFHIRBuild(t, config, inputData)
	resource := fhirResourceFrom(t, output, "fhirResource")

	items := resource["item"].([]interface{})
	item := items[0].(map[string]interface{})
	adjudication, ok := item["adjudication"].([]interface{})
	if !ok || len(adjudication) != 5 {
		t.Fatalf("expected 5 adjudication entries (2 fixed + 3 flattened from 2 CAS occurrences), got %d: %v", len(adjudication), item["adjudication"])
	}

	wantReasonAt := map[int]string{2: "45", 3: "97", 4: "1"}
	for idx, wantReason := range wantReasonAt {
		entry := adjudication[idx].(map[string]interface{})
		code := entry["category"].(map[string]interface{})["coding"].([]interface{})[0].(map[string]interface{})["code"]
		if code != wantReason {
			t.Errorf("adjudication[%d].category.coding[0].code = %v, want %q", idx, code, wantReason)
		}
	}
}

// TestFHIRBuild_NestedRepeatingGroup_NoFixedEntries_StartsAtZero is a
// regression guard: when the parent row's Fields never touch the nested
// group's TargetPath at all, startingIndex must still return 0 (not panic on
// a missing key), same as top-level repeatingGroups behaved before nesting
// was added.
func TestFHIRBuild_NestedRepeatingGroup_NoFixedEntries_StartsAtZero(t *testing.T) {
	initFHIRRegistry(t)
	config := map[string]interface{}{
		"resourceType": "ExplanationOfBenefit",
		"repeatingGroups": []interface{}{
			map[string]interface{}{
				"targetPath": "item",
				"rowsPath":   "items",
				"repeatingGroups": []interface{}{
					map[string]interface{}{
						"targetPath": "adjudication",
						"rowsPath":   "adjustments",
						"fields": []interface{}{
							map[string]interface{}{"targetPath": "category.coding[0].code", "sourcePath": "reasonCode"},
						},
					},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"items": []interface{}{
			map[string]interface{}{
				"adjustments": []interface{}{
					map[string]interface{}{"reasonCode": "2"},
				},
			},
		},
	}

	output := runFHIRBuild(t, config, inputData)
	resource := fhirResourceFrom(t, output, "fhirResource")
	items := resource["item"].([]interface{})
	item := items[0].(map[string]interface{})
	adjudication, ok := item["adjudication"].([]interface{})
	if !ok || len(adjudication) != 1 {
		t.Fatalf("expected exactly 1 adjudication entry starting at index 0, got %v", item["adjudication"])
	}
}

// TestFHIRBuild_Location_NameAndStatus verifies Location builds its
// plain-string name and status fields.
func TestFHIRBuild_Location_NameAndStatus(t *testing.T) {
	initFHIRRegistry(t)
	config := map[string]interface{}{
		"resourceType": "Location",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "status", "literalValue": "active"},
			map[string]interface{}{"targetPath": "name", "sourcePath": "locationName"},
		},
	}
	inputData := map[string]interface{}{"locationName": "Emergency Department"}

	output := runFHIRBuild(t, config, inputData)
	resource := fhirResourceFrom(t, output, "fhirResource")

	if got := resource["status"]; got != "active" {
		t.Errorf("status = %v, want active", got)
	}
	if got := resource["name"]; got != "Emergency Department" {
		t.Errorf("name = %v, want Emergency Department", got)
	}
}

// TestFHIRBuild_GetOutputVariables_ReflectsConfiguredOutputField proves the
// field picker (payload.builder's Resource Paths, wired via
// StepVariablesProvider -> GET /api/pipeline/reference-variables ->
// GetOutputVariables) reports THIS step's own real outputField/resourceType,
// not the hardcoded "fhirResource" default -- a step author who renames
// outputField (e.g. the EDI 835 template's "message.paymentReconciliation")
// would otherwise have the picker silently suggest a path that doesn't
// exist anywhere in the pipeline's actual data.
func TestFHIRBuild_GetOutputVariables_ReflectsConfiguredOutputField(t *testing.T) {
	executor := NewFHIRBuildExecutor()

	step := &models.TransformationStep{
		StepName: "Build PaymentReconciliation",
		StepType: "fhir.build",
		Enabled:  true,
		Config: map[string]interface{}{
			"resourceType": "PaymentReconciliation",
			"outputField":  "message.paymentReconciliation",
		},
	}
	vars := executor.GetOutputVariables(step)
	if len(vars) != 1 {
		t.Fatalf("expected exactly 1 declared variable, got %d: %+v", len(vars), vars)
	}
	if vars[0].Path != "message.paymentReconciliation" {
		t.Errorf("Path = %q, want %q", vars[0].Path, "message.paymentReconciliation")
	}
	if vars[0].Name != "PaymentReconciliation Resource" {
		t.Errorf("Name = %q, want it to mention the configured resourceType", vars[0].Name)
	}
}

// TestFHIRBuild_GetOutputVariables_DefaultsWhenConfigEmpty covers a fresh
// step with no config yet (e.g. just dragged onto the canvas) -- must fall
// back to the same "fhirResource" default Execute() itself uses, not panic
// or return an empty path.
func TestFHIRBuild_GetOutputVariables_DefaultsWhenConfigEmpty(t *testing.T) {
	executor := NewFHIRBuildExecutor()
	step := &models.TransformationStep{StepName: "New Step", StepType: "fhir.build", Enabled: true}

	vars := executor.GetOutputVariables(step)
	if len(vars) != 1 || vars[0].Path != "fhirResource" {
		t.Fatalf("expected default path %q, got %+v", "fhirResource", vars)
	}
}

// ── Conditional field/row population ────────────────────────────────────────

func TestFHIRBuild_FieldCondition_FalseOmitsOnlyThatField(t *testing.T) {
	initFHIRRegistry(t)
	config := map[string]interface{}{
		"resourceType": "Patient",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "birthDate", "sourcePath": "dob"},
			map[string]interface{}{
				"targetPath": "deceasedBoolean", "literalValue": "true",
				"condition": map[string]interface{}{"field": "status", "operator": "equals", "value": "deceased"},
			},
		},
	}
	inputData := map[string]interface{}{"dob": "1980-01-01", "status": "active"}

	output := runFHIRBuild(t, config, inputData)
	resource := fhirResourceFrom(t, output, "fhirResource")

	if resource["birthDate"] != "1980-01-01" {
		t.Errorf("birthDate = %v, want 1980-01-01 (unconditional field must still be written)", resource["birthDate"])
	}
	if _, present := resource["deceasedBoolean"]; present {
		t.Errorf("deceasedBoolean should be absent when its condition is false, got %v", resource["deceasedBoolean"])
	}
}

func TestFHIRBuild_FieldCondition_RowFallsBackToTopLevel(t *testing.T) {
	initFHIRRegistry(t)
	config := map[string]interface{}{
		"resourceType": "ExplanationOfBenefit",
		"repeatingGroups": []interface{}{
			map[string]interface{}{
				"targetPath": "item",
				"rowsPath":   "items",
				"fields": []interface{}{
					map[string]interface{}{"targetPath": "sequence", "sourcePath": "seq", "transform": "cda_decimal_string_to_number"},
					map[string]interface{}{
						// "country" exists only on the ROOT inputData, not on
						// each item row -- proves conditionMet's topLevel
						// fallback (mergeWithFallback) actually works here.
						"targetPath": "category.coding[0].code", "literalValue": "us-only",
						"condition": map[string]interface{}{"field": "country", "operator": "equals", "value": "US"},
					},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"country": "US",
		"items":   []interface{}{map[string]interface{}{"seq": "1"}},
	}

	output := runFHIRBuild(t, config, inputData)
	resource := fhirResourceFrom(t, output, "fhirResource")

	items, ok := resource["item"].([]interface{})
	if !ok || len(items) != 1 {
		t.Fatalf("expected 1 item, got %v", resource["item"])
	}
	item := items[0].(map[string]interface{})
	code := item["category"].(map[string]interface{})["coding"].([]interface{})[0].(map[string]interface{})["code"]
	if code != "us-only" {
		t.Errorf("category.coding[0].code = %v, want us-only (condition should resolve 'country' from topLevel, not the row)", code)
	}
}

// TestFHIRBuild_RepeatingGroup_RowCondition_SkipsCASTrioWithEmptyReasonCode is
// the real motivating case: an EDI 835 CAS segment has up to 6 adjustment
// trios, most of which are blank padding in any real claim. Condition lets
// the blank ones be dropped while the populated ones stay in the same
// adjudication[] list, in order.
func TestFHIRBuild_RepeatingGroup_RowCondition_SkipsCASTrioWithEmptyReasonCode(t *testing.T) {
	initFHIRRegistry(t)
	config := map[string]interface{}{
		"resourceType": "ExplanationOfBenefit",
		"repeatingGroups": []interface{}{
			map[string]interface{}{
				"targetPath": "adjudication",
				"rowsPath":   "trios",
				// A blank X12 element resolves as an empty string, not a
				// missing key, so "not_equals" against "" (not "not_exists")
				// is the rule that matches real EDI data.
				"condition": map[string]interface{}{"field": "reasonCode", "operator": "not_equals", "value": ""},
				"fields": []interface{}{
					map[string]interface{}{"targetPath": "category.coding[0].code", "sourcePath": "reasonCode"},
					map[string]interface{}{"targetPath": "amount.value", "sourcePath": "amount", "transform": "cda_decimal_string_to_number"},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"trios": []interface{}{
			map[string]interface{}{"reasonCode": "1", "amount": "50.00"},  // populated -- keep
			// reasonCode blank but amount non-empty: without the new
			// Condition gate, applyFieldRow would still write amount.value
			// (0 is not isEmptyFieldValue), leaving a non-empty subObj that
			// the pre-existing "skip if len(subObj)==0" check would NOT
			// catch on its own -- this is what actually proves Condition is
			// doing the work, not the old empty-row fallback.
			map[string]interface{}{"reasonCode": "", "amount": "0.00"},
			map[string]interface{}{"reasonCode": "45", "amount": "10.00"}, // populated -- keep
			map[string]interface{}{"reasonCode": "", "amount": "0.00"},
		},
	}

	output := runFHIRBuild(t, config, inputData)
	resource := fhirResourceFrom(t, output, "fhirResource")

	adjudication, ok := resource["adjudication"].([]interface{})
	if !ok || len(adjudication) != 2 {
		t.Fatalf("expected 2 adjudication entries (blank trios dropped), got %v", resource["adjudication"])
	}
	first := adjudication[0].(map[string]interface{})
	if code := first["category"].(map[string]interface{})["coding"].([]interface{})[0].(map[string]interface{})["code"]; code != "1" {
		t.Errorf("adjudication[0] reasonCode = %v, want 1", code)
	}
	second := adjudication[1].(map[string]interface{})
	if code := second["category"].(map[string]interface{})["coding"].([]interface{})[0].(map[string]interface{})["code"]; code != "45" {
		t.Errorf("adjudication[1] reasonCode = %v, want 45 (order preserved, blank rows skipped in place)", code)
	}
}

func TestFHIRBuild_RepeatingGroup_GroupCondition_FalseSkipsEntireListNotJustRows(t *testing.T) {
	initFHIRRegistry(t)
	config := map[string]interface{}{
		"resourceType": "PaymentReconciliation",
		"repeatingGroups": []interface{}{
			map[string]interface{}{
				"targetPath":     "detail",
				"rowsPath":       "claims",
				"groupCondition": map[string]interface{}{"field": "isMultiClaim", "operator": "equals", "value": true},
				"fields": []interface{}{
					map[string]interface{}{"targetPath": "amount.value", "sourcePath": "amount", "transform": "cda_decimal_string_to_number"},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"isMultiClaim": false,
		"claims":       []interface{}{map[string]interface{}{"amount": "100.00"}},
	}

	output := runFHIRBuild(t, config, inputData)
	resource := fhirResourceFrom(t, output, "fhirResource")

	if _, present := resource["detail"]; present {
		t.Errorf("expected 'detail' to be entirely absent when groupCondition is false, got %v", resource["detail"])
	}
}

// TestFHIRBuild_RepeatingGroup_GroupConditionAndRowCondition_BothApplyTogether
// proves the two-field design is load-bearing: a single condition field
// (HL7-style) could not express "only build this list when a claim-level
// flag holds, AND within it, drop any row whose amount is zero" -- that's a
// conjunction of two independent checks at two independent points.
func TestFHIRBuild_RepeatingGroup_GroupConditionAndRowCondition_BothApplyTogether(t *testing.T) {
	initFHIRRegistry(t)
	config := map[string]interface{}{
		"resourceType": "PaymentReconciliation",
		"repeatingGroups": []interface{}{
			map[string]interface{}{
				"targetPath":     "detail",
				"rowsPath":       "claims",
				"groupCondition": map[string]interface{}{"field": "isMultiClaim", "operator": "equals", "value": true},
				"condition":      map[string]interface{}{"field": "amount", "operator": "not_equals", "value": "0.00"},
				"fields": []interface{}{
					map[string]interface{}{"targetPath": "amount.value", "sourcePath": "amount", "transform": "cda_decimal_string_to_number"},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"isMultiClaim": true,
		"claims": []interface{}{
			map[string]interface{}{"amount": "100.00"},
			map[string]interface{}{"amount": "0.00"}, // group applies, but this row is filtered
		},
	}

	output := runFHIRBuild(t, config, inputData)
	resource := fhirResourceFrom(t, output, "fhirResource")

	detail, ok := resource["detail"].([]interface{})
	if !ok || len(detail) != 1 {
		t.Fatalf("expected exactly 1 detail entry (group built, one row filtered), got %v", resource["detail"])
	}
}

// TestFHIRBuild_NestedRepeatingGroup_ConditionSeesGenuineTopLevelField proves
// topLevel is threaded unchanged through recursion: a condition on the
// INNERMOST of two nested repeatingGroups can still see a field that exists
// only on the true root inputData -- not on the outer row, not on the inner
// row -- which was structurally impossible before topLevel was threaded
// through applyFieldRow/applyRepeatingGroup.
func TestFHIRBuild_NestedRepeatingGroup_ConditionSeesGenuineTopLevelField(t *testing.T) {
	initFHIRRegistry(t)
	config := map[string]interface{}{
		"resourceType": "ExplanationOfBenefit",
		"repeatingGroups": []interface{}{
			map[string]interface{}{
				"targetPath": "item",
				"rowsPath":   "items",
				"fields": []interface{}{
					map[string]interface{}{"targetPath": "sequence", "sourcePath": "seq", "transform": "cda_decimal_string_to_number"},
				},
				"repeatingGroups": []interface{}{
					map[string]interface{}{
						"targetPath": "adjudication",
						"rowsPath":   "adjustments",
						"condition":  map[string]interface{}{"field": "region", "operator": "equals", "value": "NC"},
						"fields": []interface{}{
							map[string]interface{}{"targetPath": "amount.value", "sourcePath": "amount", "transform": "cda_decimal_string_to_number"},
						},
					},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"region": "NC", // ROOT-only field -- absent from both items[] and adjustments[]
		"items": []interface{}{
			map[string]interface{}{
				"seq":         "1",
				"adjustments": []interface{}{map[string]interface{}{"amount": "20.00"}},
			},
		},
	}

	output := runFHIRBuild(t, config, inputData)
	resource := fhirResourceFrom(t, output, "fhirResource")

	items := resource["item"].([]interface{})
	item := items[0].(map[string]interface{})
	adjudication, ok := item["adjudication"].([]interface{})
	if !ok || len(adjudication) != 1 {
		t.Fatalf("expected 1 adjudication entry -- innermost condition should resolve 'region' via topLevel fallback, got %v", item["adjudication"])
	}
}

// TestFHIRBuild_Condition_PredicateBracketSourcePath_ResolvesConsistentlyWithFieldMapping
// is the test that would have caught the original GetNestedValue/GetFieldValue
// mismatch: native []map[string]interface{} data (the shape EDI's parser
// actually produces, not []interface{}) with a predicate-bracket path used as
// a CONDITION field, proving conditionMet resolves it the same way an
// ordinary sourcePath on the identical shape already does.
func TestFHIRBuild_Condition_PredicateBracketSourcePath_ResolvesConsistentlyWithFieldMapping(t *testing.T) {
	initFHIRRegistry(t)
	config := map[string]interface{}{
		"resourceType": "ExplanationOfBenefit",
		"fields": []interface{}{
			map[string]interface{}{
				"targetPath": "provider.display",
				"sourcePath": "NM1[entityIdentifierCode=82].nameLastOrOrganizationName",
				"condition":  map[string]interface{}{"field": "NM1[entityIdentifierCode=82].nameLastOrOrganizationName", "operator": "exists"},
			},
		},
	}
	inputData := map[string]interface{}{
		"NM1": []map[string]interface{}{
			{"entityIdentifierCode": "QC", "nameLastOrOrganizationName": "Doe"},
			{"entityIdentifierCode": "82", "nameLastOrOrganizationName": "Smith Clinic"},
		},
	}

	output := runFHIRBuild(t, config, inputData)
	resource := fhirResourceFrom(t, output, "fhirResource")

	display := resource["provider"].(map[string]interface{})["display"]
	if display != "Smith Clinic" {
		t.Errorf("provider.display = %v, want Smith Clinic (condition's predicate-bracket path must resolve the same as the ordinary sourcePath does)", display)
	}
}
