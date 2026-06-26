// services/cda_fhir/cda_fhir_integration_test.go
// Unit tests for CDA→FHIR transform/profile infrastructure shared by the
// declarative engine.
//
// Test categories:
//  1. CDATransformRegistry — individual named transforms (unit, no DB)
//  2. setFHIRPath — FHIR path assignment helper (unit, no DB)
//  3. USCoreProfileBuilder — profile injection and Patient extensions (unit, no DB)
//  4. CDATransformRegistry.ValidateOOBMappings — OOB template transform-name validation
//
// Phase 4 Slice D removed this file's former sections 4 (GenericCDAFHIRMapper.Map())
// and 6 (MapDocument()) — both tested the decommissioned hardcoded Go mapper
// path (see services/cda_fhir/declarative_document_mapper.go's own header
// comment for what replaced it). The remaining sections here test
// infrastructure declarative_document_mapper.go still depends on directly
// (CDATransformRegistry, USCoreProfileBuilder) and were never Map()/
// MapDocument()-specific.
package cdafhir_test

import (
	"testing"

	cdafhir "ezhealthkonnect/services/cda_fhir"
)

// ================================================================
// 1. CDATransformRegistry — named transform coverage
// ================================================================

func TestCDATransformRegistry_StringDirect(t *testing.T) {
	reg := cdafhir.NewCDATransformRegistry()
	entry := map[string]interface{}{"name": "John Doe"}
	got, err := reg.ApplyTransform("string_direct", entry, "name", nil)
	if err != nil {
		t.Fatalf("string_direct: unexpected error: %v", err)
	}
	if got != "John Doe" {
		t.Errorf("string_direct: got %v, want 'John Doe'", got)
	}
}

func TestCDATransformRegistry_BooleanDirect(t *testing.T) {
	reg := cdafhir.NewCDATransformRegistry()
	entry := map[string]interface{}{"flag": "true"}
	got, err := reg.ApplyTransform("boolean_direct", entry, "flag", nil)
	if err != nil {
		t.Fatalf("boolean_direct: unexpected error: %v", err)
	}
	if got != true {
		t.Errorf("boolean_direct: got %v, want true", got)
	}
}

func TestCDATransformRegistry_TimestampToFHIRDate(t *testing.T) {
	reg := cdafhir.NewCDATransformRegistry()
	entry := map[string]interface{}{"dob": "19800315"}
	got, err := reg.ApplyTransform("cda_timestamp_to_fhir_date", entry, "dob", nil)
	if err != nil {
		t.Fatalf("cda_timestamp_to_fhir_date: unexpected error: %v", err)
	}
	if got != "1980-03-15" {
		t.Errorf("cda_timestamp_to_fhir_date: got %v, want '1980-03-15'", got)
	}
}

func TestCDATransformRegistry_TimestampToFHIRDatetime(t *testing.T) {
	reg := cdafhir.NewCDATransformRegistry()
	entry := map[string]interface{}{"ts": "20250601120000+0000"}
	got, err := reg.ApplyTransform("cda_timestamp_to_fhir_datetime", entry, "ts", nil)
	if err != nil {
		t.Fatalf("cda_timestamp_to_fhir_datetime: unexpected error: %v", err)
	}
	s, ok := got.(string)
	if !ok || len(s) < 10 {
		t.Errorf("cda_timestamp_to_fhir_datetime: got %v, want a datetime string", got)
	}
}

func TestCDATransformRegistry_GenderToFHIR(t *testing.T) {
	reg := cdafhir.NewCDATransformRegistry()
	cases := []struct{ in, want string }{
		{"M", "male"}, {"F", "female"}, {"UN", "unknown"},
	}
	for _, tc := range cases {
		entry := map[string]interface{}{"sex": tc.in}
		got, err := reg.ApplyTransform("cda_gender_to_fhir", entry, "sex", nil)
		if err != nil {
			t.Fatalf("gender_to_fhir(%q): unexpected error: %v", tc.in, err)
		}
		if got != tc.want {
			t.Errorf("gender_to_fhir(%q): got %v, want %q", tc.in, got, tc.want)
		}
	}
}

func TestCDATransformRegistry_ValueMapOverride(t *testing.T) {
	reg := cdafhir.NewCDATransformRegistry()
	entry := map[string]interface{}{"status": "active"}
	vm := map[string]string{"active": "final", "completed": "final"}
	got, err := reg.ApplyTransform("cda_allergy_status_to_fhir", entry, "status", vm)
	if err != nil {
		t.Fatalf("allergy_status_to_fhir with valueMap: unexpected error: %v", err)
	}
	if got != "final" {
		t.Errorf("allergy_status_to_fhir with valueMap: got %v, want 'final'", got)
	}
}

func TestCDATransformRegistry_InferTransform_CD_to_CodeableConcept(t *testing.T) {
	reg := cdafhir.NewCDATransformRegistry()
	name, err := reg.InferTransform("CD", "CodeableConcept")
	if err != nil {
		t.Fatalf("InferTransform CD→CodeableConcept: unexpected error: %v", err)
	}
	if name != "cda_code_to_codeable_concept" {
		t.Errorf("InferTransform: got %q, want 'cda_code_to_codeable_concept'", name)
	}
}

func TestCDATransformRegistry_InferTransform_UnknownPair(t *testing.T) {
	reg := cdafhir.NewCDATransformRegistry()
	_, err := reg.InferTransform("XYZ", "Widget")
	if err == nil {
		t.Error("InferTransform XYZ→Widget: expected error, got nil")
	}
}

func TestCDATransformRegistry_UnknownTransform(t *testing.T) {
	reg := cdafhir.NewCDATransformRegistry()
	_, err := reg.ApplyTransform("does_not_exist", map[string]interface{}{}, "key", nil)
	if err == nil {
		t.Error("ApplyTransform(does_not_exist): expected error, got nil")
	}
}

// ================================================================
// 2. setFHIRPath — via exported helper using USCoreProfileBuilder.InjectProfile
//    (which calls setFHIRPath internally)
// ================================================================

func TestSetFHIRPath_ViaProfileInjection(t *testing.T) {
	builder := cdafhir.NewUSCoreProfileBuilder()
	r := map[string]interface{}{"resourceType": "Patient"}
	builder.InjectProfile(r, "")
	meta, ok := r["meta"].(map[string]interface{})
	if !ok {
		t.Fatal("InjectProfile: no meta key in resource")
	}
	profiles, ok := meta["profile"].([]interface{})
	if !ok || len(profiles) == 0 {
		t.Fatal("InjectProfile: meta.profile is missing or empty")
	}
	want := "http://hl7.org/fhir/us/core/StructureDefinition/us-core-patient"
	if profiles[0] != want {
		t.Errorf("InjectProfile: got %v, want %q", profiles[0], want)
	}
}

// ================================================================
// 3. USCoreProfileBuilder — profile injection correctness
// ================================================================

func TestUSCoreProfileBuilder_KnownResources(t *testing.T) {
	builder := cdafhir.NewUSCoreProfileBuilder()
	knownTypes := []struct {
		resourceType string
		wantFragment string
	}{
		{"AllergyIntolerance", "us-core-allergyintolerance"},
		{"Condition", "us-core-condition"},
		{"Observation", "us-core-observation"},
		// MedicationStatement intentionally has no entry: US Core dropped that
		// profile in 5.0+ in favor of MedicationRequest (see
		// resourceTypeToProfile's comment in us_core_profile_builder.go).
		{"MedicationRequest", "us-core-medicationrequest"},
		{"Immunization", "us-core-immunization"},
		{"Procedure", "us-core-procedure"},
		{"Encounter", "us-core-encounter"},
	}

	for _, tc := range knownTypes {
		r := map[string]interface{}{"resourceType": tc.resourceType}
		builder.InjectProfile(r, "")
		meta, ok := r["meta"].(map[string]interface{})
		if !ok {
			t.Errorf("%s: no meta after InjectProfile", tc.resourceType)
			continue
		}
		profiles, _ := meta["profile"].([]interface{})
		if len(profiles) == 0 {
			t.Errorf("%s: empty profile slice", tc.resourceType)
			continue
		}
		profileURL, _ := profiles[0].(string)
		if len(profileURL) == 0 {
			t.Errorf("%s: profile URL is empty", tc.resourceType)
		}
	}
}

func TestUSCoreProfileBuilder_TemplateProfilePreferred(t *testing.T) {
	builder := cdafhir.NewUSCoreProfileBuilder()
	custom := "http://example.org/MyCustomProfile"
	r := map[string]interface{}{"resourceType": "Patient"}
	builder.InjectProfile(r, custom)
	meta, _ := r["meta"].(map[string]interface{})
	profiles, _ := meta["profile"].([]interface{})
	found := false
	for _, p := range profiles {
		if p == custom {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("template profile %q not found in %v", custom, profiles)
	}
}

func TestUSCoreProfileBuilder_UnknownResourceType_NoMeta(t *testing.T) {
	builder := cdafhir.NewUSCoreProfileBuilder()
	r := map[string]interface{}{"resourceType": "UnknownType"}
	builder.InjectProfile(r, "")
	// Unknown type with no template profile → meta should not be injected
	if _, ok := r["meta"]; ok {
		t.Error("InjectProfile: should not inject meta for unknown resource with no template profile")
	}
}

// ================================================================
// 4. CDATransformRegistry.ValidateOOBMappings — OOB template transform-name validation
// ================================================================

func TestCDATransformRegistry_ValidateOOBMappings_EmptySlice(t *testing.T) {
	reg := cdafhir.NewCDATransformRegistry()
	warnings := reg.ValidateOOBMappings(nil)
	if len(warnings) != 0 {
		t.Errorf("ValidateOOBMappings(nil): expected 0 warnings, got %d: %v", len(warnings), warnings)
	}
}

func TestCDATransformRegistry_ValidateOOBMappings_ValidTransform(t *testing.T) {
	reg := cdafhir.NewCDATransformRegistry()
	mappings := []cdafhir.CDAFieldMapping{
		{CDAField: "f1", Transform: "string_direct"},
		{CDAField: "f2", Transform: "cda_timestamp_to_fhir_date"},
	}
	warnings := reg.ValidateOOBMappings(mappings)
	if len(warnings) != 0 {
		t.Errorf("ValidateOOBMappings(valid): expected 0 warnings, got %d: %v", len(warnings), warnings)
	}
}

func TestCDATransformRegistry_ValidateOOBMappings_InvalidTransform(t *testing.T) {
	reg := cdafhir.NewCDATransformRegistry()
	mappings := []cdafhir.CDAFieldMapping{
		{CDAField: "f1", Transform: "nonexistent_transform"},
	}
	warnings := reg.ValidateOOBMappings(mappings)
	if len(warnings) == 0 {
		t.Error("ValidateOOBMappings(invalid transform): expected warning, got none")
	}
}
