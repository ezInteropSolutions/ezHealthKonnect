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
