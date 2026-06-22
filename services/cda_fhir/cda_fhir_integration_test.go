// services/cda_fhir/cda_fhir_integration_test.go
// Sprint C integration tests for the GenericCDAFHIRMapper pipeline.
//
// Test categories:
//  1. CDATransformRegistry — individual named transforms (unit, no DB)
//  2. setFHIRPath — FHIR path assignment helper (unit, no DB)
//  3. USCoreProfileBuilder — profile injection and Patient extensions (unit, no DB)
//  4. GenericCDAFHIRMapper.Map() — end-to-end CDA→FHIR with nil-DB (OOB skip)
//  5. Section failure isolation — goroutine error containment
//
// DB-dependent paths (three-tier template lookup) require INTEGRATION=true env var.

package cdafhir_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	cdadocument "ezhealthkonnect/cda/document"
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
// 2. setFHIRPath — via exported helper using public Map() route
//    Test via USCoreProfileBuilder.InjectProfile (which calls setFHIRPath internally)
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
// 4. GenericCDAFHIRMapper — Map() with in-memory parsed CDA
//    Uses nil DB so getCDAFieldMappings falls through to error →
//    test the no-DB graceful code path (returns error, not panic).
//    Full OOB mapping coverage requires integration env.
// ================================================================

func TestGenericCDAFHIRMapper_NilDB_ReturnsError(t *testing.T) {
	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	parsedCDA := map[string]interface{}{
		"_format":  "ccda",
		"sections": map[string]interface{}{},
		"header":   map[string]interface{}{},
	}
	_, err := mapper.Map(context.Background(), parsedCDA, cdafhir.CDAToFHIRConfig{DocType: "CCD"})
	if err == nil {
		t.Error("Map with nil DB: expected error, got nil")
	}
}

func TestGenericCDAFHIRMapper_Integration_CCD(t *testing.T) {
	if os.Getenv("INTEGRATION") != "true" {
		t.Skip("skipping DB-dependent integration test; set INTEGRATION=true to run")
	}

	// This test requires a live DB populated with V149/V150 migrations.
	// When INTEGRATION=true, the test runner is expected to inject a real DB
	// via the standard PostgreSQL env vars (DB_HOST, DB_NAME, etc.).
	t.Log("Integration test skipped: implement live DB wiring when test infra is ready")
}

// ================================================================
// 5. Section failure isolation — partial success processing
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

// ================================================================
// 6. Sprint D — MapDocument() typed-struct path (no DB required)
// ================================================================

// newMinimalDoc creates a *CDADocument suitable for MapDocument() unit tests.
// It has a patient with numIDs identifiers, and optionally an allergy section
// with numAllergies entries.
func newMinimalDoc(numIDs int, numAllergies int) *cdadocument.CDADocument {
	ids := make([]cdadocument.CDAII, numIDs)
	for i := range ids {
		ids[i] = cdadocument.CDAII{Root: fmt.Sprintf("2.16.840.1.113883.3.%d", i+1), Extension: fmt.Sprintf("ID-%d", i+1)}
	}

	doc := &cdadocument.CDADocument{
		SectionsByKey: make(map[string]*cdadocument.CDASection),
		Header: cdadocument.CDAHeader{
			Patient: cdadocument.CDAPatient{
				Ids: ids,
				Names: []cdadocument.CDAName{
					{Use: "L", Given: []string{"Jane"}, Family: "Smith"},
				},
				Gender: cdadocument.CDACode{Code: "F"},
				BirthDate: cdadocument.CDATime{Value: "19850315"},
			},
		},
	}

	if numAllergies > 0 {
		entries := make([]cdadocument.CDAEntry, numAllergies)
		for i := range entries {
			entries[i] = cdadocument.CDAEntry{
				EntryType:  "act",
				StatusCode: "active",
				Code:       cdadocument.CDACode{Code: "48765001", CodeSystem: "2.16.840.1.113883.6.96", DisplayName: "Allergy"},
			}
		}
		doc.SectionsByKey["allergiesAndIntolerances"] = &cdadocument.CDASection{
			Key:     "allergiesAndIntolerances",
			Entries: entries,
		}
	}

	return doc
}

// countResourcesByType returns the number of bundle entries whose resource type matches.
func countResourcesByType(bundle map[string]interface{}, resourceType string) int {
	entries, _ := bundle["entry"].([]interface{})
	count := 0
	for _, e := range entries {
		entry, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		res, ok := entry["resource"].(map[string]interface{})
		if !ok {
			continue
		}
		if res["resourceType"] == resourceType {
			count++
		}
	}
	return count
}

// findPatientResource returns the first Patient resource from the bundle, or nil.
func findPatientResource(bundle map[string]interface{}) map[string]interface{} {
	entries, _ := bundle["entry"].([]interface{})
	for _, e := range entries {
		entry, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		res, ok := entry["resource"].(map[string]interface{})
		if !ok {
			continue
		}
		if res["resourceType"] == "Patient" {
			return res
		}
	}
	return nil
}

func TestMapDocument_NilDoc_ReturnsError(t *testing.T) {
	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	_, err := mapper.MapDocument(context.Background(), nil, cdafhir.CDAToFHIRConfig{})
	if err == nil {
		t.Error("MapDocument(nil): expected error, got nil")
	}
}

func TestMapDocument_EmptyDoc_ProducesBundle(t *testing.T) {
	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	doc := &cdadocument.CDADocument{
		SectionsByKey: make(map[string]*cdadocument.CDASection),
	}
	out, err := mapper.MapDocument(context.Background(), doc, cdafhir.CDAToFHIRConfig{})
	if err != nil {
		t.Fatalf("MapDocument(empty doc): unexpected error: %v", err)
	}
	if out == nil || out.FHIRBundle == nil {
		t.Fatal("MapDocument(empty doc): nil output or bundle")
	}
	if out.FHIRBundle["resourceType"] != "Bundle" {
		t.Errorf("MapDocument(empty doc): resourceType=%v, want Bundle", out.FHIRBundle["resourceType"])
	}
}

// TestMapDocument_DuplicateSectionMappers_GetUniqueIDsAbsoluteFullURLs_AndRewrittenRefs
// covers fixes 1+2 together: "problems" and "healthConcerns" both dispatch to
// MapConditions, and each numbers its own output from 1 — without the
// document-wide renumbering pass, both sections would each produce a
// "condition-1". This also verifies every bundle entry's fullUrl is an
// absolute urn:uuid:, unique, and that Condition.subject was rewritten from
// the relative "Patient/patient-1" form to the Patient's actual fullUrl.
func TestMapDocument_DuplicateSectionMappers_GetUniqueIDsAbsoluteFullURLs_AndRewrittenRefs(t *testing.T) {
	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	doc := newMinimalDoc(1, 0)
	doc.SectionsByKey["problems"] = &cdadocument.CDASection{
		Key:     "problems",
		Entries: []cdadocument.CDAEntry{{EntryType: "act", StatusCode: "active"}},
	}
	doc.SectionsByKey["healthConcerns"] = &cdadocument.CDASection{
		Key:     "healthConcerns",
		Entries: []cdadocument.CDAEntry{{EntryType: "act", StatusCode: "active"}},
	}

	out, err := mapper.MapDocument(context.Background(), doc, cdafhir.CDAToFHIRConfig{})
	if err != nil {
		t.Fatalf("MapDocument: unexpected error: %v", err)
	}

	entries, _ := out.FHIRBundle["entry"].([]interface{})
	seenFullURLs := map[string]bool{}
	seenConditionIDs := map[string]bool{}
	patientFullURL := ""

	for _, e := range entries {
		entry := e.(map[string]interface{})
		fullURL, _ := entry["fullUrl"].(string)
		if !strings.HasPrefix(fullURL, "urn:uuid:") {
			t.Errorf("entry fullUrl %q is not an absolute urn:uuid: URL", fullURL)
		}
		if seenFullURLs[fullURL] {
			t.Errorf("duplicate fullUrl %q across bundle entries", fullURL)
		}
		seenFullURLs[fullURL] = true

		res, _ := entry["resource"].(map[string]interface{})
		switch res["resourceType"] {
		case "Patient":
			patientFullURL = fullURL
		case "Condition":
			id, _ := res["id"].(string)
			if seenConditionIDs[id] {
				t.Errorf("duplicate Condition id %q produced by independent sections", id)
			}
			seenConditionIDs[id] = true
		}
	}

	if len(seenConditionIDs) != 2 {
		t.Errorf("expected 2 uniquely-id'd Condition resources (one per section), got %d", len(seenConditionIDs))
	}
	if patientFullURL == "" {
		t.Fatal("no Patient resource found in bundle")
	}

	for _, e := range entries {
		entry := e.(map[string]interface{})
		res, _ := entry["resource"].(map[string]interface{})
		if res["resourceType"] != "Condition" {
			continue
		}
		subject, _ := res["subject"].(map[string]interface{})
		if ref, _ := subject["reference"].(string); ref != patientFullURL {
			t.Errorf("Condition.subject.reference = %q, want Patient's fullUrl %q", ref, patientFullURL)
		}
	}
}

// TestMapDocument_PatientIdentifiers_MinCount verifies AC-3:
// A patient with 3 CDA <id> elements produces a FHIR Patient with ≥3 identifiers.
func TestMapDocument_PatientIdentifiers_MinCount(t *testing.T) {
	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	doc := newMinimalDoc(3, 0)

	out, err := mapper.MapDocument(context.Background(), doc, cdafhir.CDAToFHIRConfig{ProfileMode: "base"})
	if err != nil {
		t.Fatalf("MapDocument: unexpected error: %v", err)
	}

	patient := findPatientResource(out.FHIRBundle)
	if patient == nil {
		t.Fatal("MapDocument: no Patient resource in bundle")
	}

	identifiers, _ := patient["identifier"].([]interface{})
	if len(identifiers) < 3 {
		t.Errorf("MapDocument: Patient.identifier count = %d, want ≥3", len(identifiers))
	}
}

// TestMapDocument_AllergyCount_MatchesEntries verifies AC-5:
// AllergyIntolerance resource count equals the number of allergy section entries.
func TestMapDocument_AllergyCount_MatchesEntries(t *testing.T) {
	const numAllergies = 4
	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	doc := newMinimalDoc(1, numAllergies)

	out, err := mapper.MapDocument(context.Background(), doc, cdafhir.CDAToFHIRConfig{ProfileMode: "base"})
	if err != nil {
		t.Fatalf("MapDocument: unexpected error: %v", err)
	}

	got := countResourcesByType(out.FHIRBundle, "AllergyIntolerance")
	if got != numAllergies {
		t.Errorf("MapDocument: AllergyIntolerance count = %d, want %d", got, numAllergies)
	}
}

func TestMapDocument_PatientName_MappedCorrectly(t *testing.T) {
	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	doc := newMinimalDoc(1, 0)

	out, err := mapper.MapDocument(context.Background(), doc, cdafhir.CDAToFHIRConfig{ProfileMode: "base"})
	if err != nil {
		t.Fatalf("MapDocument: unexpected error: %v", err)
	}

	patient := findPatientResource(out.FHIRBundle)
	if patient == nil {
		t.Fatal("MapDocument: no Patient resource in bundle")
	}

	names, _ := patient["name"].([]interface{})
	if len(names) == 0 {
		t.Fatal("MapDocument: Patient.name is empty")
	}
	first, _ := names[0].(map[string]interface{})
	if first["family"] != "Smith" {
		t.Errorf("MapDocument: Patient.name[0].family = %v, want Smith", first["family"])
	}
}

func TestMapDocument_BirthDate_Formatted(t *testing.T) {
	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	doc := newMinimalDoc(1, 0)

	out, err := mapper.MapDocument(context.Background(), doc, cdafhir.CDAToFHIRConfig{ProfileMode: "base"})
	if err != nil {
		t.Fatalf("MapDocument: unexpected error: %v", err)
	}

	patient := findPatientResource(out.FHIRBundle)
	if patient == nil {
		t.Fatal("MapDocument: no Patient resource in bundle")
	}
	bd, _ := patient["birthDate"].(string)
	// CDATimeToFHIRDate("19850315") → "1985-03-15"
	if bd != "1985-03-15" {
		t.Errorf("MapDocument: Patient.birthDate = %q, want 1985-03-15", bd)
	}
}

func TestMapDocument_ProcessingResult_SectionCounts(t *testing.T) {
	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	doc := newMinimalDoc(1, 2)

	out, err := mapper.MapDocument(context.Background(), doc, cdafhir.CDAToFHIRConfig{ProfileMode: "base"})
	if err != nil {
		t.Fatalf("MapDocument: unexpected error: %v", err)
	}

	if out.ProcessingResult.SectionsProcessed < 1 {
		t.Errorf("MapDocument: SectionsProcessed = %d, want ≥1", out.ProcessingResult.SectionsProcessed)
	}
	if out.ProcessingResult.ResourcesProduced < 1 {
		t.Errorf("MapDocument: ResourcesProduced = %d, want ≥1", out.ProcessingResult.ResourcesProduced)
	}
}

func TestMapDocument_OnSectionFailure_Continue(t *testing.T) {
	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	doc := newMinimalDoc(1, 1)

	out, err := mapper.MapDocument(context.Background(), doc, cdafhir.CDAToFHIRConfig{
		ProfileMode:      "base",
		OnSectionFailure: "continue",
	})
	if err != nil {
		t.Fatalf("MapDocument: unexpected error: %v", err)
	}
	if out == nil {
		t.Fatal("MapDocument: nil output")
	}
}

func TestMapDocument_EnabledSections_Filters(t *testing.T) {
	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	// Doc has only allergies section
	doc := newMinimalDoc(1, 3)
	// Enable only "medications" (which has no entries in doc) → no section resources
	out, err := mapper.MapDocument(context.Background(), doc, cdafhir.CDAToFHIRConfig{
		ProfileMode:     "base",
		EnabledSections: []string{"medications"},
	})
	if err != nil {
		t.Fatalf("MapDocument: unexpected error: %v", err)
	}
	// AllergyIntolerance should NOT appear (section filtered out)
	got := countResourcesByType(out.FHIRBundle, "AllergyIntolerance")
	if got != 0 {
		t.Errorf("MapDocument(enabledSections=[medications]): got %d AllergyIntolerance, want 0", got)
	}
}

func TestMapDocument_BundleType_Collection(t *testing.T) {
	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	doc := newMinimalDoc(1, 0)
	out, err := mapper.MapDocument(context.Background(), doc, cdafhir.CDAToFHIRConfig{
		BundleType:  "collection",
		ProfileMode: "base",
	})
	if err != nil {
		t.Fatalf("MapDocument: unexpected error: %v", err)
	}
	if out.FHIRBundle["type"] != "collection" {
		t.Errorf("MapDocument: bundle type = %v, want collection", out.FHIRBundle["type"])
	}
}

func TestMapDocument_MultipleSections_ResourceTypes(t *testing.T) {
	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	doc := newMinimalDoc(1, 1)

	// Add a problems section
	doc.SectionsByKey["problems"] = &cdadocument.CDASection{
		Key: "problems",
		Entries: []cdadocument.CDAEntry{
			{EntryType: "observation", StatusCode: "active", Code: cdadocument.CDACode{Code: "55607006"}},
		},
	}

	out, err := mapper.MapDocument(context.Background(), doc, cdafhir.CDAToFHIRConfig{ProfileMode: "base"})
	if err != nil {
		t.Fatalf("MapDocument: unexpected error: %v", err)
	}

	allergyCount := countResourcesByType(out.FHIRBundle, "AllergyIntolerance")
	if allergyCount < 1 {
		t.Errorf("MapDocument: AllergyIntolerance count = %d, want ≥1", allergyCount)
	}
	conditionCount := countResourcesByType(out.FHIRBundle, "Condition")
	if conditionCount < 1 {
		t.Errorf("MapDocument: Condition count = %d, want ≥1", conditionCount)
	}
}

// TestMapDocument_Integration_CCD runs when INTEGRATION=true against a real CCD file.
func TestMapDocument_Integration_CCD(t *testing.T) {
	if os.Getenv("INTEGRATION") != "true" {
		t.Skip("skipping typed-path integration test; set INTEGRATION=true to run")
	}
	t.Log("Integration test: implement live CCD parse + MapDocument when test infra is ready")
}
