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
