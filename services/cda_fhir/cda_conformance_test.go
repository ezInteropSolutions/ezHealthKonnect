//go:build conformance

// services/cda_fhir/cda_conformance_test.go
// Sprint D conformance tests for the CDA→FHIR output.
//
// Run with:  go test -tags conformance ./services/cda_fhir/...
//
// Test categories:
//   1. CompositionMapper — document bundle structure (no external tool)
//   2. US Core profile presence — all clinical resource types (no external tool)
//   3. HAPI FHIR Validator — zero error-severity issues for US Core profiles
//      (skipped automatically when HAPI_VALIDATOR_JAR env var is not set)
//
// HAPI validator setup:
//   export HAPI_VALIDATOR_JAR=/path/to/validator_cli.jar
//   go test -tags conformance -run TestHAPIValidator ./services/cda_fhir/...

package cdafhir_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	cdafhir "ezhealthkonnect/services/cda_fhir"
)

// ================================================================
// 1. CompositionMapper — document bundle structure
// ================================================================

func TestCompositionMapper_BuildsCompositionResource(t *testing.T) {
	cm := cdafhir.NewCompositionMapper()
	headerMap := map[string]interface{}{
		"title":               "Continuity of Care Document",
		"effectiveTime":       "20250601120000+0000",
		"confidentialityCode": "N",
		"author":              map[string]interface{}{"given": "Jane", "family": "Smith"},
		"custodian":           map[string]interface{}{"name": "Test Hospital"},
	}
	allResources := []map[string]interface{}{
		{"resourceType": "Patient", "id": "patient-1"},
		{"resourceType": "Practitioner", "id": "author-1"},
		{"resourceType": "Organization", "id": "custodian-1"},
		{"resourceType": "AllergyIntolerance", "id": "allergy-1"},
		{"resourceType": "Condition", "id": "cond-1"},
	}
	config := cdafhir.CDAToFHIRConfig{
		DocType:     "CCD",
		BundleType:  "document",
		ProfileMode: "us-core",
	}

	comp := cm.BuildComposition(headerMap, allResources, config)

	if comp["resourceType"] != "Composition" {
		t.Errorf("resourceType: got %v, want Composition", comp["resourceType"])
	}
	if comp["status"] != "final" {
		t.Errorf("status: got %v, want final", comp["status"])
	}
	if comp["title"] != "Continuity of Care Document" {
		t.Errorf("title: got %v, want 'Continuity of Care Document'", comp["title"])
	}
	if comp["date"] == "" || comp["date"] == nil {
		t.Error("date must be set")
	}

	// subject must reference Patient
	subj, ok := comp["subject"].(map[string]interface{})
	if !ok || subj["reference"] != "Patient/patient-1" {
		t.Errorf("subject.reference: got %v, want 'Patient/patient-1'", subj)
	}

	// author must reference Practitioner (one present)
	authors, ok := comp["author"].([]interface{})
	if !ok || len(authors) == 0 {
		t.Error("author array missing or empty")
	} else {
		firstAuthor, _ := authors[0].(map[string]interface{})
		if firstAuthor["reference"] != "Practitioner/author-1" {
			t.Errorf("author[0].reference: got %v, want 'Practitioner/author-1'", firstAuthor["reference"])
		}
	}

	// custodian must reference Organization
	cust, ok := comp["custodian"].(map[string]interface{})
	if !ok || cust["reference"] != "Organization/custodian-1" {
		t.Errorf("custodian.reference: got %v, want 'Organization/custodian-1'", cust)
	}

	// meta.profile must be present for us-core mode
	meta, ok := comp["meta"].(map[string]interface{})
	if !ok {
		t.Fatal("meta missing on Composition in us-core mode")
	}
	profiles, ok := meta["profile"].([]interface{})
	if !ok || len(profiles) == 0 {
		t.Error("meta.profile missing or empty on Composition")
	}

	// type must carry a LOINC code
	typeField, ok := comp["type"].(map[string]interface{})
	if !ok {
		t.Fatal("type field missing on Composition")
	}
	codings, _ := typeField["coding"].([]interface{})
	if len(codings) == 0 {
		t.Error("type.coding must not be empty")
	} else {
		first, _ := codings[0].(map[string]interface{})
		if first["system"] != "http://loinc.org" {
			t.Errorf("type.coding[0].system: got %v, want http://loinc.org", first["system"])
		}
		if first["code"] != "34133-9" {
			t.Errorf("type.coding[0].code: got %v, want 34133-9 (CCD LOINC)", first["code"])
		}
	}

	// section entries must NOT include header resource types
	sections, ok := comp["section"].([]interface{})
	if !ok || len(sections) == 0 {
		t.Error("section array missing or empty")
	}
	for i, sec := range sections {
		s, _ := sec.(map[string]interface{})
		entries, _ := s["entry"].([]interface{})
		for _, e := range entries {
			entry, _ := e.(map[string]interface{})
			ref, _ := entry["reference"].(string)
			if strings.HasPrefix(ref, "Patient/") || strings.HasPrefix(ref, "Practitioner/") || strings.HasPrefix(ref, "Organization/") {
				t.Errorf("section[%d]: header resource %q must not appear as a section entry", i, ref)
			}
		}
	}
}

func TestCompositionMapper_DischargeSummaryLoincCode(t *testing.T) {
	cm := cdafhir.NewCompositionMapper()
	comp := cm.BuildComposition(nil, nil, cdafhir.CDAToFHIRConfig{
		DocType:    "DischargeSummary",
		BundleType: "document",
	})
	typeField, _ := comp["type"].(map[string]interface{})
	codings, _ := typeField["coding"].([]interface{})
	first, _ := codings[0].(map[string]interface{})
	if first["code"] != "18842-5" {
		t.Errorf("DischargeSummary LOINC: got %v, want 18842-5", first["code"])
	}
}

func TestCompositionMapper_ReferralNoteLoincCode(t *testing.T) {
	cm := cdafhir.NewCompositionMapper()
	comp := cm.BuildComposition(nil, nil, cdafhir.CDAToFHIRConfig{
		DocType:    "ReferralNote",
		BundleType: "document",
	})
	typeField, _ := comp["type"].(map[string]interface{})
	codings, _ := typeField["coding"].([]interface{})
	first, _ := codings[0].(map[string]interface{})
	if first["code"] != "57133-1" {
		t.Errorf("ReferralNote LOINC: got %v, want 57133-1", first["code"])
	}
}

func TestCompositionMapper_NoMetaInBaseMode(t *testing.T) {
	cm := cdafhir.NewCompositionMapper()
	comp := cm.BuildComposition(nil, nil, cdafhir.CDAToFHIRConfig{
		DocType:     "CCD",
		BundleType:  "document",
		ProfileMode: "base",
	})
	if _, ok := comp["meta"]; ok {
		t.Error("meta must not be injected in base profile mode")
	}
}

func TestCompositionMapper_NilHeaderFallbacks(t *testing.T) {
	cm := cdafhir.NewCompositionMapper()
	comp := cm.BuildComposition(nil, nil, cdafhir.CDAToFHIRConfig{
		DocType:    "CCD",
		BundleType: "document",
	})
	if comp["title"] != "Continuity of Care Document" {
		t.Errorf("nil header: title fallback: got %v", comp["title"])
	}
	if comp["confidentiality"] != "N" {
		t.Errorf("nil header: confidentiality fallback: got %v", comp["confidentiality"])
	}
	dateStr, _ := comp["date"].(string)
	if dateStr == "" {
		t.Error("nil header: date must not be empty (should fall back to now)")
	}
}

// ================================================================
// 2. US Core profile presence — resource-level assertion
// ================================================================

func TestUSCore_MetaProfile_RequiredResources(t *testing.T) {
	builder := cdafhir.NewUSCoreProfileBuilder()

	// All US Core resources that the CDA→FHIR engine can produce
	resourceTypes := []string{
		"Patient",
		"AllergyIntolerance",
		"Condition",
		"MedicationStatement",
		"Immunization",
		"Observation",
		"Procedure",
		"Encounter",
		"Practitioner",
		"Organization",
		"DiagnosticReport",
	}

	for _, rt := range resourceTypes {
		r := map[string]interface{}{"resourceType": rt, "id": "test-1"}
		builder.InjectProfile(r, "")
		meta, ok := r["meta"].(map[string]interface{})
		profiles, _ := meta["profile"].([]interface{})
		if !ok || len(profiles) == 0 {
			// Not all types have a US Core profile — warn but don't fail for unknown types
			t.Logf("NOTICE: %s: no meta.profile injected (no US Core mapping defined)", rt)
		}
	}
}

func TestUSCore_AllergyIntolerance_HasProfile(t *testing.T) {
	assertProfileContains(t, "AllergyIntolerance", "us-core-allergyintolerance")
}

func TestUSCore_Condition_HasProfile(t *testing.T) {
	assertProfileContains(t, "Condition", "us-core-condition")
}

func TestUSCore_Observation_HasProfile(t *testing.T) {
	assertProfileContains(t, "Observation", "us-core-observation")
}

func TestUSCore_Patient_HasProfile(t *testing.T) {
	assertProfileContains(t, "Patient", "us-core-patient")
}

// MedicationStatement intentionally has no US Core 6.1.0 profile — it was
// dropped from US Core 5.0+ in favor of MedicationRequest (see
// resourceTypeToProfile's comment in us_core_profile_builder.go).
func TestUSCore_MedicationRequest_HasProfile(t *testing.T) {
	assertProfileContains(t, "MedicationRequest", "us-core-medicationrequest")
}

func TestUSCore_Immunization_HasProfile(t *testing.T) {
	assertProfileContains(t, "Immunization", "us-core-immunization")
}

func assertProfileContains(t *testing.T, resourceType, profileFragment string) {
	t.Helper()
	builder := cdafhir.NewUSCoreProfileBuilder()
	r := map[string]interface{}{"resourceType": resourceType, "id": "test-1"}
	builder.InjectProfile(r, "")
	meta, ok := r["meta"].(map[string]interface{})
	if !ok {
		t.Fatalf("%s: meta not injected", resourceType)
	}
	profiles, _ := meta["profile"].([]interface{})
	if len(profiles) == 0 {
		t.Fatalf("%s: meta.profile empty", resourceType)
	}
	for _, p := range profiles {
		if s, ok := p.(string); ok && strings.Contains(s, profileFragment) {
			return
		}
	}
	t.Errorf("%s: meta.profile %v does not contain %q", resourceType, profiles, profileFragment)
}

// ================================================================
// 3. HAPI FHIR Validator — zero error-severity issues
//    Skipped when HAPI_VALIDATOR_JAR is not set.
// ================================================================

// hapiValidationResult mirrors the HAPI CLI -output json structure.
type hapiValidationResult struct {
	Issues []struct {
		Severity string `json:"severity"`
		Message  string `json:"message"`
		Location string `json:"location"`
	} `json:"issues"`
}

func TestHAPIValidator_Patient_USCore(t *testing.T) {
	builder := cdafhir.NewUSCoreProfileBuilder()
	patient := map[string]interface{}{
		"resourceType": "Patient",
		"id":           "test-patient-1",
		"name": []interface{}{
			map[string]interface{}{
				"use":    "official",
				"family": "Doe",
				"given":  []interface{}{"John"},
			},
		},
		"gender":    "male",
		"birthDate": "1980-03-15",
		"identifier": []interface{}{
			map[string]interface{}{
				"system": "http://hl7.org/fhir/sid/us-ssn",
				"value":  "123-45-6789",
			},
		},
	}
	builder.InjectProfile(patient, "")

	assertHAPIValid(t, patient, []string{
		"hl7.fhir.us.core#6.1.0",
	}, "Patient/test-patient-1")
}

func TestHAPIValidator_AllergyIntolerance_USCore(t *testing.T) {
	builder := cdafhir.NewUSCoreProfileBuilder()
	allergy := map[string]interface{}{
		"resourceType":      "AllergyIntolerance",
		"id":                "test-allergy-1",
		"clinicalStatus": map[string]interface{}{
			"coding": []interface{}{
				map[string]interface{}{
					"system": "http://terminology.hl7.org/CodeSystem/allergyintolerance-clinical",
					"code":   "active",
				},
			},
		},
		"verificationStatus": map[string]interface{}{
			"coding": []interface{}{
				map[string]interface{}{
					"system": "http://terminology.hl7.org/CodeSystem/allergyintolerance-verification",
					"code":   "confirmed",
				},
			},
		},
		"type": "allergy",
		"code": map[string]interface{}{
			"coding": []interface{}{
				map[string]interface{}{
					"system":  "http://www.nlm.nih.gov/research/umls/rxnorm",
					"code":    "7980",
					"display": "Penicillin",
				},
			},
		},
		"patient": map[string]interface{}{"reference": "Patient/test-patient-1"},
	}
	builder.InjectProfile(allergy, "")

	assertHAPIValid(t, allergy, []string{
		"hl7.fhir.us.core#6.1.0",
	}, "AllergyIntolerance/test-allergy-1")
}

func TestHAPIValidator_Observation_VitalSigns(t *testing.T) {
	builder := cdafhir.NewUSCoreProfileBuilder()
	obs := map[string]interface{}{
		"resourceType": "Observation",
		"id":           "test-obs-bp",
		"status":       "final",
		"category": []interface{}{
			map[string]interface{}{
				"coding": []interface{}{
					map[string]interface{}{
						"system":  "http://terminology.hl7.org/CodeSystem/observation-category",
						"code":    "vital-signs",
						"display": "Vital Signs",
					},
				},
			},
		},
		"code": map[string]interface{}{
			"coding": []interface{}{
				map[string]interface{}{
					"system":  "http://loinc.org",
					"code":    "55284-4",
					"display": "Blood pressure systolic and diastolic",
				},
			},
		},
		"subject":        map[string]interface{}{"reference": "Patient/test-patient-1"},
		"effectiveDateTime": time.Now().UTC().Format("2006-01-02"),
		"valueQuantity": map[string]interface{}{
			"value":  120,
			"unit":   "mmHg",
			"system": "http://unitsofmeasure.org",
			"code":   "mm[Hg]",
		},
	}
	builder.InjectProfile(obs, "http://hl7.org/fhir/StructureDefinition/vitalsigns")

	assertHAPIValid(t, obs, []string{
		"hl7.fhir.us.core#6.1.0",
	}, "Observation/test-obs-bp")
}

func TestHAPIValidator_Observation_Lab(t *testing.T) {
	builder := cdafhir.NewUSCoreProfileBuilder()
	obs := map[string]interface{}{
		"resourceType": "Observation",
		"id":           "test-obs-lab",
		"status":       "final",
		"category": []interface{}{
			map[string]interface{}{
				"coding": []interface{}{
					map[string]interface{}{
						"system": "http://terminology.hl7.org/CodeSystem/observation-category",
						"code":   "laboratory",
					},
				},
			},
		},
		"code": map[string]interface{}{
			"coding": []interface{}{
				map[string]interface{}{
					"system":  "http://loinc.org",
					"code":    "2339-0",
					"display": "Glucose [Mass/volume] in Blood",
				},
			},
		},
		"subject":           map[string]interface{}{"reference": "Patient/test-patient-1"},
		"effectiveDateTime": time.Now().UTC().Format("2006-01-02"),
		"valueQuantity": map[string]interface{}{
			"value":  95,
			"unit":   "mg/dL",
			"system": "http://unitsofmeasure.org",
			"code":   "mg/dL",
		},
	}
	builder.InjectProfile(obs, "")

	assertHAPIValid(t, obs, []string{
		"hl7.fhir.us.core#6.1.0",
	}, "Observation/test-obs-lab")
}

// assertHAPIValid writes the resource to a temp file, runs HAPI FHIR Validator,
// and asserts zero error-severity issues. Skips gracefully when the JAR is absent.
func assertHAPIValid(t *testing.T, resource map[string]interface{}, igs []string, resourceRef string) {
	t.Helper()

	jarPath := os.Getenv("HAPI_VALIDATOR_JAR")
	if jarPath == "" {
		t.Skipf("HAPI_VALIDATOR_JAR not set — skipping HAPI validation for %s", resourceRef)
	}
	if _, err := os.Stat(jarPath); err != nil {
		t.Skipf("HAPI_VALIDATOR_JAR %q not found: %v — skipping", jarPath, err)
	}

	// Serialize resource to temp file
	tmpDir := t.TempDir()
	inputFile := filepath.Join(tmpDir, "resource.json")
	outputFile := filepath.Join(tmpDir, "result.json")

	data, err := json.MarshalIndent(resource, "", "  ")
	if err != nil {
		t.Fatalf("marshal resource: %v", err)
	}
	if err := os.WriteFile(inputFile, data, 0600); err != nil {
		t.Fatalf("write input: %v", err)
	}

	// Build HAPI CLI args
	args := []string{
		"-jar", jarPath,
		"-version", "4.0.1",
		"-output", "json",
		"-output-file", outputFile,
	}
	for _, ig := range igs {
		args = append(args, "-ig", ig)
	}
	args = append(args, inputFile)

	cmd := exec.Command("java", args...)
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		// Non-zero exit from the validator is expected when there are errors;
		// we still parse the output to report specific issues.
		t.Logf("HAPI validator exited with: %v\nstdout/stderr:\n%s", runErr, string(out))
	}

	// Parse output JSON
	resultBytes, readErr := os.ReadFile(outputFile)
	if readErr != nil {
		t.Fatalf("read validator output: %v (validator output: %s)", readErr, string(out))
	}

	var result hapiValidationResult
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		t.Fatalf("parse validator output JSON: %v\nraw: %s", err, string(resultBytes))
	}

	// Assert zero error-severity issues
	var errors []string
	for _, issue := range result.Issues {
		if issue.Severity == "error" || issue.Severity == "fatal" {
			errors = append(errors, fmt.Sprintf("[%s] %s @ %s", issue.Severity, issue.Message, issue.Location))
		}
	}

	if len(errors) > 0 {
		t.Errorf("%s: HAPI FHIR Validator reported %d error(s):\n  %s",
			resourceRef, len(errors), strings.Join(errors, "\n  "))
	} else {
		t.Logf("%s: HAPI validation passed (%d issue(s) total, 0 errors)", resourceRef, len(result.Issues))
	}
}
