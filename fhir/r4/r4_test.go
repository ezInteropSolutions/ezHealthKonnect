package r4_test

import (
	"ezhealthkonnect/fhir/r4"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

const testSchemaDir = "../../schemas/fhir"

// initRegistry initialises the global registry once for all tests in this file.
// Tests that need a real registry call this; tests that use mock dependencies skip it.
func initRegistry(t *testing.T) {
	t.Helper()
	err := r4.ForceReinit(testSchemaDir)
	require.NoError(t, err, "registry initialisation must not return an error")
	require.NotNil(t, r4.GetRegistry(), "GetRegistry must return a non-nil registry")
}

// ─────────────────────────────────────────────────────────────────────────────
// 1. Compiler — parseCardinality
// ─────────────────────────────────────────────────────────────────────────────

func TestParseCardinality(t *testing.T) {
	// parseCardinality is internal; we test it indirectly through a compiled profile.
	// Compile a minimal fake profile by calling ForceReinit with the real schemas
	// and inspecting the resulting CompiledProfile.
	initRegistry(t)
	reg := r4.GetRegistry()
	require.NotNil(t, reg)

	cp, ok := reg.Get("R4", "Patient", "base")
	require.True(t, ok, "R4/Patient/base must be in registry")
	require.NotNil(t, cp)

	// Patient.active is "0..1" → min=0, max=1
	assert.Equal(t, 0, cp.MinCard["Patient.active"], "Patient.active min should be 0")
	assert.Equal(t, 1, cp.MaxCard["Patient.active"], "Patient.active max should be 1")

	// Patient.name is "0..*" → min=0, max=-1
	assert.Equal(t, 0, cp.MinCard["Patient.name"], "Patient.name min should be 0")
	assert.Equal(t, -1, cp.MaxCard["Patient.name"], "Patient.name max should be -1 (unbounded)")
}

// ─────────────────────────────────────────────────────────────────────────────
// 2. Registry — eager load, Get, List
// ─────────────────────────────────────────────────────────────────────────────

func TestRegistry_GetKnownProfiles(t *testing.T) {
	initRegistry(t)
	reg := r4.GetRegistry()

	cases := []struct{ version, rt, profile string }{
		{"R4", "Patient", "base"},
		{"R4", "Observation", "base"},
		{"R4", "Encounter", "base"},
		{"R4", "Bundle", "base"},
		{"R4", "Patient", "us-core"},
		{"R4", "Observation", "us-core"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.version+"/"+tc.rt+"/"+tc.profile, func(t *testing.T) {
			cp, ok := reg.Get(tc.version, tc.rt, tc.profile)
			assert.True(t, ok, "profile should be registered")
			assert.NotNil(t, cp)
			assert.Equal(t, tc.version, cp.Version)
			assert.Equal(t, tc.rt, cp.ResourceType)
			assert.Equal(t, tc.profile, cp.Profile)
			assert.Greater(t, cp.ElementCount, 0, "profile must have at least one element")
		})
	}
}

func TestRegistry_DefaultsEmptyStrings(t *testing.T) {
	initRegistry(t)
	reg := r4.GetRegistry()

	// empty version → R4, empty profile → base
	cp, ok := reg.Get("", "Patient", "")
	assert.True(t, ok)
	assert.Equal(t, "R4", cp.Version)
	assert.Equal(t, "base", cp.Profile)
}

func TestRegistry_MissingProfileReturnsFalse(t *testing.T) {
	initRegistry(t)
	reg := r4.GetRegistry()

	_, ok := reg.Get("R4", "NonExistentResource", "base")
	assert.False(t, ok, "unknown resource type should return false")
}

func TestRegistry_List(t *testing.T) {
	initRegistry(t)
	reg := r4.GetRegistry()

	keys := reg.List("R4")
	assert.Greater(t, len(keys), 100, "R4 should have >100 compiled base profiles")

	// All keys must belong to R4
	for _, k := range keys {
		assert.Equal(t, "R4", k.Version)
	}
}

func TestRegistry_Stats(t *testing.T) {
	initRegistry(t)
	reg := r4.GetRegistry()

	stats := reg.GetStats()
	assert.Greater(t, stats.TotalProfiles, 100)
	assert.Greater(t, stats.VersionCounts["R4"], 100)
	assert.Greater(t, stats.ProfileCounts["us-core"], 0)
	assert.Equal(t, 0, stats.LoadErrors, "no load errors expected")
}

// ─────────────────────────────────────────────────────────────────────────────
// 3. CompiledProfile — US Core Patient specifics
// ─────────────────────────────────────────────────────────────────────────────

func TestUSCorePatient_RequiredFields(t *testing.T) {
	initRegistry(t)
	reg := r4.GetRegistry()

	cp, ok := reg.Get("R4", "Patient", "us-core")
	require.True(t, ok)

	// US Core Patient: identifier, identifier.system, identifier.value, name are required
	assert.True(t, cp.Required["Patient.identifier"], "Patient.identifier must be required in US Core")
	assert.True(t, cp.Required["Patient.name"], "Patient.name must be required in US Core")
}

func TestUSCorePatient_MustSupport(t *testing.T) {
	initRegistry(t)
	reg := r4.GetRegistry()

	cp, ok := reg.Get("R4", "Patient", "us-core")
	require.True(t, ok)
	assert.NotEmpty(t, cp.MustSupport, "US Core Patient must have must-support elements")
}

// TestCompiledProfile_NameDescriptionAllBindings proves the schema-consolidation
// work's additive fields actually carry real data from the .gz files, not just
// compile without error — this is what unblocked migrating
// controllers/cda_schema_controller.go and services/parsers/fhir_parser_service.go
// off the separate legacy loader (fhir/schema_loader.go), which had carried
// this same information independently until now.
func TestCompiledProfile_NameDescriptionAllBindings(t *testing.T) {
	initRegistry(t)
	reg := r4.GetRegistry()

	cp, ok := reg.Get("R4", "Patient", "base")
	require.True(t, ok)

	assert.Equal(t, "Patient", cp.Name, "resource-level Name should come from the StructureDefinition")
	assert.NotEmpty(t, cp.Description, "resource-level Description should be populated")

	assert.NotEmpty(t, cp.ElementNames["Patient.gender"], "per-element Name should be populated")
	assert.NotEmpty(t, cp.ElementDescriptions["Patient.gender"], "per-element Description should be populated")

	// Patient.gender is bound to administrative-gender (required strength) —
	// must appear in both Bindings (enforcement) and AllBindings (display).
	foundInBindings := false
	for _, b := range cp.Bindings {
		if b.Path == "Patient.gender" {
			foundInBindings = true
		}
	}
	assert.True(t, foundInBindings, "Patient.gender should be a required/extensible binding")

	foundInAll := false
	for _, b := range cp.AllBindings {
		if b.Path == "Patient.gender" {
			foundInAll = true
			assert.NotEmpty(t, b.ValueSetURL)
		}
	}
	assert.True(t, foundInAll, "AllBindings should also carry Patient.gender")
}

// ─────────────────────────────────────────────────────────────────────────────
// 4. TerminologyRegistry — Contains
// ─────────────────────────────────────────────────────────────────────────────

func TestTerminology_RequiredBindings(t *testing.T) {
	tr := r4.GetTerminologyRegistry()
	require.NotNil(t, tr)

	cases := []struct {
		vsURL string
		code  string
		want  bool
	}{
		{"http://hl7.org/fhir/ValueSet/observation-status", "final", true},
		{"http://hl7.org/fhir/ValueSet/observation-status", "invalid-code", false},
		{"http://hl7.org/fhir/ValueSet/administrative-gender", "male", true},
		// FHIR codes are case-sensitive by spec; the precompiled ValueSet file
		// confirms caseInsensitive=false for administrative-gender, so "MALE"
		// is not a recognized member of the set.
		{"http://hl7.org/fhir/ValueSet/administrative-gender", "MALE", false},
		{"http://hl7.org/fhir/ValueSet/administrative-gender", "unknown-gender", false},
		{"http://hl7.org/fhir/ValueSet/encounter-status", "in-progress", true},
		{"http://hl7.org/fhir/ValueSet/encounter-status", "bogus", false},
		{"http://hl7.org/fhir/ValueSet/bundle-type", "transaction", true},
		{"http://hl7.org/fhir/ValueSet/bundle-type", "not-a-type", false},
		{"http://hl7.org/fhir/ValueSet/condition-clinical", "active", true},
		{"http://hl7.org/fhir/ValueSet/medicationrequest-status", "active", true},
		{"http://hl7.org/fhir/ValueSet/immunization-status", "completed", true},
		{"http://hl7.org/fhir/ValueSet/allergy-intolerance-category", "food", true},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.vsURL[strings.LastIndex(tc.vsURL, "/")+1:]+"/"+tc.code, func(t *testing.T) {
			inSet, known := tr.Contains(tc.vsURL, tc.code)
			assert.True(t, known, "ValueSet should be known")
			assert.Equal(t, tc.want, inSet)
		})
	}
}

func TestTerminology_VersionSuffixStripped(t *testing.T) {
	tr := r4.GetTerminologyRegistry()
	// observation-status URL with version suffix should still resolve
	inSet, known := tr.Contains("http://hl7.org/fhir/ValueSet/observation-status|4.0.1", "final")
	assert.True(t, known)
	assert.True(t, inSet)
}

func TestTerminology_UnknownValueSet(t *testing.T) {
	tr := r4.GetTerminologyRegistry()
	inSet, known := tr.Contains("http://example.com/custom-valueset", "someCode")
	assert.False(t, known, "custom ValueSet should not be known")
	assert.False(t, inSet)
}

// ─────────────────────────────────────────────────────────────────────────────
// 5. ConstraintRegistry — predicate functions
// ─────────────────────────────────────────────────────────────────────────────

// obs-6 and bun-1 were retired from ConstraintRegistry once real spec data
// covered them generically (see fhir/r4/compiler.go's compileSpecConstraints
// and fhir/r4/fhirpath) — their old direct-registry tests moved to
// fhir/r4/fhirpath/spec_rules_test.go's TestSpecRules_Obs6_* /
// TestSpecRules_Bdl1_* (same pass/fail fixtures, testing the rule directly).
// End-to-end coverage that the retirement didn't change observable validator
// behavior remains right here: TestValidator_Strict_ObsDataAbsentViolation
// and TestValidator_Bundle_StrictBdl1Violation below.

func TestConstraint_bun7_DuplicateFullUrl(t *testing.T) {
	cr := r4.GetConstraintRegistry()

	entries := []interface{}{
		map[string]interface{}{"fullUrl": "urn:uuid:1"},
		map[string]interface{}{"fullUrl": "urn:uuid:2"},
	}
	pass := map[string]interface{}{"resourceType": "Bundle", "type": "transaction", "entry": entries}
	assert.Empty(t, filterKey(cr.Check("Bundle", pass), "bun-7"))

	dupEntries := []interface{}{
		map[string]interface{}{"fullUrl": "urn:uuid:1"},
		map[string]interface{}{"fullUrl": "urn:uuid:1"}, // duplicate
	}
	fail := map[string]interface{}{"resourceType": "Bundle", "type": "transaction", "entry": dupEntries}
	viols := filterKey(cr.Check("Bundle", fail), "bun-7")
	assert.NotEmpty(t, viols)
}

func TestConstraint_pat1_TelecomSystem(t *testing.T) {
	cr := r4.GetConstraintRegistry()

	// PASS: telecom has system + value
	pass := map[string]interface{}{
		"resourceType": "Patient",
		"telecom":      []interface{}{map[string]interface{}{"system": "phone", "value": "555-1234"}},
	}
	assert.Empty(t, filterKey(cr.Check("Patient", pass), "pat-1"))

	// FAIL: telecom has value but no system
	fail := map[string]interface{}{
		"resourceType": "Patient",
		"telecom":      []interface{}{map[string]interface{}{"value": "555-1234"}},
	}
	viols := filterKey(cr.Check("Patient", fail), "pat-1")
	assert.NotEmpty(t, viols)
}

// ─────────────────────────────────────────────────────────────────────────────
// 6. FHIRR4Validator — ValidateResource
// ─────────────────────────────────────────────────────────────────────────────

// stubProfile builds a minimal CompiledProfile for use in unit tests without
// touching the real registry.
type stubProfileProvider struct {
	profile *r4.CompiledProfile
}

func (s *stubProfileProvider) Get(_, rt, _ string) (*r4.CompiledProfile, bool) {
	if s.profile != nil && s.profile.ResourceType == rt {
		return s.profile, true
	}
	return nil, false
}
func (s *stubProfileProvider) List(_ string) []r4.RegistryKey { return nil }
func (s *stubProfileProvider) GetStats() r4.RegistryStats      { return r4.RegistryStats{} }

func TestValidator_Basic_UnknownType(t *testing.T) {
	v := r4.NewFHIRR4Validator(nil, nil, nil)
	res := v.ValidateResource(
		map[string]interface{}{"resourceType": "NotARealType"},
		r4.ValidationOptions{Level: r4.LevelBasic},
	)
	assert.False(t, res.Valid)
	assert.NotEmpty(t, res.Errors)
}

func TestValidator_Basic_MissingResourceType(t *testing.T) {
	v := r4.NewFHIRR4Validator(nil, nil, nil)
	res := v.ValidateResource(
		map[string]interface{}{"id": "123"},
		r4.ValidationOptions{Level: r4.LevelBasic},
	)
	assert.False(t, res.Valid)
	require.NotEmpty(t, res.Errors)
	assert.Equal(t, "missing-resource-type", res.Errors[0].Code)
}

func TestValidator_Standard_RequiredField_Missing(t *testing.T) {
	initRegistry(t)
	v := r4.NewFHIRR4Validator(r4.GetRegistry(), r4.GetTerminologyRegistry(), r4.GetConstraintRegistry())

	// Observation without status (required)
	obs := map[string]interface{}{
		"resourceType": "Observation",
		"id":           "obs-1",
		"code":         map[string]interface{}{"coding": []interface{}{}},
		// status intentionally omitted
	}
	res := v.ValidateResource(obs, r4.ValidationOptions{Level: r4.LevelStandard})
	errorCodes := collectCodes(res.Errors)
	assert.Contains(t, errorCodes, "required-field",
		"missing Observation.status should produce required-field error")
}

func TestValidator_Standard_ValidPatient_NoErrors(t *testing.T) {
	initRegistry(t)
	v := r4.NewFHIRR4Validator(r4.GetRegistry(), r4.GetTerminologyRegistry(), r4.GetConstraintRegistry())

	patient := map[string]interface{}{
		"resourceType": "Patient",
		"id":           "pat-1",
	}
	// Patient base profile has no strictly required fields → should pass standard
	res := v.ValidateResource(patient, r4.ValidationOptions{Level: r4.LevelStandard})
	assert.True(t, res.Valid, "valid Patient should pass standard validation")
	assert.Empty(t, res.Errors)
}

func TestValidator_Strict_InvalidGenderCode(t *testing.T) {
	initRegistry(t)
	v := r4.NewFHIRR4Validator(r4.GetRegistry(), r4.GetTerminologyRegistry(), r4.GetConstraintRegistry())

	patient := map[string]interface{}{
		"resourceType": "Patient",
		"id":           "pat-2",
		"gender":       "INVALID_GENDER",
	}
	res := v.ValidateResource(patient, r4.ValidationOptions{Level: r4.LevelStrict})
	errorCodes := collectCodes(res.Errors)
	assert.Contains(t, errorCodes, "invalid-code",
		"invalid gender code should produce invalid-code error in strict mode")
}

func TestValidator_Strict_ValidGenderCode(t *testing.T) {
	initRegistry(t)
	v := r4.NewFHIRR4Validator(r4.GetRegistry(), r4.GetTerminologyRegistry(), r4.GetConstraintRegistry())

	patient := map[string]interface{}{
		"resourceType": "Patient",
		"id":           "pat-3",
		"gender":       "female",
	}
	res := v.ValidateResource(patient, r4.ValidationOptions{Level: r4.LevelStrict})
	assert.True(t, res.Valid)
	assert.Empty(t, res.Errors)
}

// TestValidator_CodingSystemCheck_ResourceLevelCodeField_NotFlaggedAsCoding
// regression-covers a false positive in walkCodingSystems: Condition.code,
// Observation.code, and AllergyIntolerance.code are each a CodeableConcept
// assigned to a resource field literally named "code" — that collided with
// the walker's existence-only "has a code key => treat as a Coding" heuristic,
// firing a bogus "Coding has a code but no system" warning on the resource
// itself (path == the resource type, not a real element path) every time,
// even when every actual Coding nested inside had a proper system.
func TestValidator_CodingSystemCheck_ResourceLevelCodeField_NotFlaggedAsCoding(t *testing.T) {
	initRegistry(t)
	v := r4.NewFHIRR4Validator(r4.GetRegistry(), r4.GetTerminologyRegistry(), r4.GetConstraintRegistry())

	condition := map[string]interface{}{
		"resourceType": "Condition",
		"id":           "cond-1",
		"subject":      map[string]interface{}{"reference": "Patient/12345"},
		"code": map[string]interface{}{
			"coding": []interface{}{
				map[string]interface{}{"system": "http://snomed.info/sct", "code": "44054006"},
			},
		},
	}
	res := v.ValidateResource(condition, r4.ValidationOptions{Level: r4.LevelStrict})
	warningCodes := collectCodes(res.Warnings)
	assert.NotContains(t, warningCodes, "coding-no-system",
		"Condition.code (a CodeableConcept) must not be misidentified as a bare Coding missing its system")
}

// TestValidator_CodingSystemCheck_RealCodingMissingSystem_StillFlagged ensures
// the fix didn't remove real detection: an actual Coding (system+code siblings,
// nested under a field that is NOT itself named "code") with no system must
// still be flagged.
func TestValidator_CodingSystemCheck_RealCodingMissingSystem_StillFlagged(t *testing.T) {
	initRegistry(t)
	v := r4.NewFHIRR4Validator(r4.GetRegistry(), r4.GetTerminologyRegistry(), r4.GetConstraintRegistry())

	condition := map[string]interface{}{
		"resourceType": "Condition",
		"id":           "cond-2",
		"subject":      map[string]interface{}{"reference": "Patient/12345"},
		"clinicalStatus": map[string]interface{}{
			"coding": []interface{}{
				map[string]interface{}{"code": "active"},
			},
		},
	}
	res := v.ValidateResource(condition, r4.ValidationOptions{Level: r4.LevelStrict})
	warningCodes := collectCodes(res.Warnings)
	assert.Contains(t, warningCodes, "coding-no-system",
		"a genuine Coding with a code but no system must still be flagged")
}

func TestValidator_Strict_ObsDataAbsentViolation(t *testing.T) {
	initRegistry(t)
	v := r4.NewFHIRR4Validator(r4.GetRegistry(), r4.GetTerminologyRegistry(), r4.GetConstraintRegistry())

	obs := map[string]interface{}{
		"resourceType":     "Observation",
		"id":               "obs-2",
		"status":           "final",
		"code":             map[string]interface{}{},
		"dataAbsentReason": map[string]interface{}{"coding": []interface{}{}},
		"valueString":      "some value", // violates obs-6
	}
	res := v.ValidateResource(obs, r4.ValidationOptions{Level: r4.LevelStrict})
	constraintCodes := collectCodes(append(res.Errors, res.Warnings...))
	assert.Contains(t, constraintCodes, "obs-6")
}

// ─────────────────────────────────────────────────────────────────────────────
// 7. FHIRR4Validator — ValidateBundle
// ─────────────────────────────────────────────────────────────────────────────

func TestValidator_Bundle_NotABundle(t *testing.T) {
	v := r4.NewFHIRR4Validator(nil, nil, nil)
	result := v.ValidateBundle(
		map[string]interface{}{"resourceType": "Patient"},
		r4.ValidationOptions{},
	)
	assert.False(t, result.Valid)
	assert.NotEmpty(t, result.BundleIssues)
}

func TestValidator_Bundle_MissingType(t *testing.T) {
	v := r4.NewFHIRR4Validator(nil, nil, nil)
	result := v.ValidateBundle(
		map[string]interface{}{"resourceType": "Bundle"},
		r4.ValidationOptions{},
	)
	assert.False(t, result.Valid)
	codes := collectIssueCodes(result.BundleIssues)
	assert.Contains(t, codes, "missing-bundle-type")
}

func TestValidator_Bundle_ValidTransactionBundle(t *testing.T) {
	initRegistry(t)
	v := r4.NewFHIRR4Validator(r4.GetRegistry(), r4.GetTerminologyRegistry(), r4.GetConstraintRegistry())

	bundle := map[string]interface{}{
		"resourceType": "Bundle",
		"type":         "transaction",
		"entry": []interface{}{
			map[string]interface{}{
				"fullUrl": "urn:uuid:patient-1",
				"resource": map[string]interface{}{
					"resourceType": "Patient",
					"id":           "patient-1",
					"gender":       "male",
				},
			},
			map[string]interface{}{
				"fullUrl": "urn:uuid:enc-1",
				"resource": map[string]interface{}{
					"resourceType": "Encounter",
					"id":           "enc-1",
					"status":       "finished",
					"class": map[string]interface{}{
						"system": "http://terminology.hl7.org/CodeSystem/v3-ActCode",
						"code":   "AMB",
					},
					"subject": map[string]interface{}{"reference": "urn:uuid:patient-1"},
				},
			},
		},
	}
	result := v.ValidateBundle(bundle, r4.ValidationOptions{
		Level:        r4.LevelStandard,
		ValidateRefs: true,
	})
	assert.Equal(t, 0, result.ErrorCount, "valid transaction bundle should have zero errors")
}

func TestValidator_Bundle_RequiredTypesMissing(t *testing.T) {
	v := r4.NewFHIRR4Validator(nil, nil, nil)
	bundle := map[string]interface{}{
		"resourceType": "Bundle",
		"type":         "collection",
		"entry":        []interface{}{},
	}
	result := v.ValidateBundle(bundle, r4.ValidationOptions{
		RequiredTypes: []string{"Patient"},
	})
	assert.False(t, result.Valid)
	codes := collectIssueCodes(result.BundleIssues)
	assert.Contains(t, codes, "missing-required-type")
}

// The retired hand-written predicate this test originally exercised was
// registered under the key "bun-1" — not a real FHIR constraint key at all
// (the actual R4 invariant is "bdl-1"; confirmed directly against
// hl7.org/fhir/R4/bundle.profile.json while building the spec-driven
// replacement). The new engine reports the real key, so this test now
// asserts on "bdl-1" — the same rule, correctly named.
func TestValidator_Bundle_StrictBdl1Violation(t *testing.T) {
	initRegistry(t)
	v := r4.NewFHIRR4Validator(r4.GetRegistry(), r4.GetTerminologyRegistry(), r4.GetConstraintRegistry())

	bundle := map[string]interface{}{
		"resourceType": "Bundle",
		"type":         "transaction",
		"total":        42, // bdl-1: total only allowed for searchset/history
		"entry":        []interface{}{},
	}
	result := v.ValidateBundle(bundle, r4.ValidationOptions{Level: r4.LevelStrict})
	codes := collectIssueCodes(result.BundleIssues)
	assert.Contains(t, codes, "bdl-1")
}

// ─────────────────────────────────────────────────────────────────────────────
// 8. ParseLevel
// ─────────────────────────────────────────────────────────────────────────────

func TestParseLevel(t *testing.T) {
	assert.Equal(t, r4.LevelBasic, r4.ParseLevel("basic"))
	assert.Equal(t, r4.LevelBasic, r4.ParseLevel("BASIC"))
	assert.Equal(t, r4.LevelStandard, r4.ParseLevel("standard"))
	assert.Equal(t, r4.LevelStandard, r4.ParseLevel(""))    // unknown → standard
	assert.Equal(t, r4.LevelStrict, r4.ParseLevel("strict"))
}

// ─────────────────────────────────────────────────────────────────────────────
// Test helpers
// ─────────────────────────────────────────────────────────────────────────────

func filterKey(viols []r4.ConstraintViolation, key string) []r4.ConstraintViolation {
	var out []r4.ConstraintViolation
	for _, v := range viols {
		if v.Key == key {
			out = append(out, v)
		}
	}
	return out
}

func collectCodes(issues []r4.ValidationIssue) []string {
	codes := make([]string, len(issues))
	for i, iss := range issues {
		codes[i] = iss.Code
	}
	return codes
}

func collectIssueCodes(issues []r4.ValidationIssue) []string {
	return collectCodes(issues)
}
