// services/cda_fhir/us_core_profile_builder_test.go
//
// InjectPatientExtensions unit coverage. Built specifically against
// documentMap["header"]["patient"]'s real shape — json.Marshal(CDAPatient)
// nests race/ethnicity/religion as {"code":..., "displayName":...} objects,
// not flat strings — to guard the regression where strField's plain string
// type-assertion silently returned "" against that shape and these
// extensions never got written at all.
package cdafhir_test

import (
	"encoding/json"
	"testing"

	cdadocument "ezhealthkonnect/cda/document"
	cdafhir "ezhealthkonnect/services/cda_fhir"
)

// patientHeaderMap builds documentMap["header"]["patient"]'s exact JSON
// shape from a typed CDAPatient, mirroring documentMapForHeader's
// marshal/unmarshal round-trip in declarative_oob_rules_test.go.
func patientHeaderMap(t *testing.T, p cdadocument.CDAPatient) map[string]interface{} {
	t.Helper()
	encoded, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("marshalling CDAPatient: %v", err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(encoded, &m); err != nil {
		t.Fatalf("unmarshalling CDAPatient: %v", err)
	}
	return m
}

func TestInjectPatientExtensions_RaceEthnicityReligion(t *testing.T) {
	header := patientHeaderMap(t, cdadocument.CDAPatient{
		Race:      cdadocument.CDACode{Code: "2106-3", DisplayName: "White", CodeSystem: "2.16.840.1.113883.6.238"},
		Ethnicity: cdadocument.CDACode{NullFlavor: "UNK"},
		Religion:  cdadocument.CDACode{Code: "1013", CodeSystem: "2.16.840.1.113883.5.1076"},
	})

	patient := map[string]interface{}{"resourceType": "Patient"}
	builder := cdafhir.NewUSCoreProfileBuilder()
	builder.InjectPatientExtensions(patient, header)

	exts, ok := patient["extension"].([]interface{})
	if !ok {
		t.Fatal("InjectPatientExtensions: no extension array written")
	}

	var race, religion map[string]interface{}
	for _, raw := range exts {
		ext, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		switch ext["url"] {
		case "http://hl7.org/fhir/us/core/StructureDefinition/us-core-race":
			race = ext
		case "http://hl7.org/fhir/us/core/StructureDefinition/us-core-ethnicity":
			t.Error("ethnicity extension written for an all-nullFlavor CDACode (code=\"\"), want none")
		case "http://hl7.org/fhir/StructureDefinition/patient-religion":
			religion = ext
		}
	}

	if race == nil {
		t.Fatal("expected a us-core-race extension, got none")
	}
	raceSub, _ := race["extension"].([]interface{})
	var ombCoding map[string]interface{}
	for _, raw := range raceSub {
		sub, _ := raw.(map[string]interface{})
		if sub["url"] == "ombCategory" {
			ombCoding, _ = sub["valueCoding"].(map[string]interface{})
		}
	}
	if ombCoding == nil || ombCoding["code"] != "2106-3" {
		t.Errorf("race ombCategory valueCoding = %v, want code=2106-3", ombCoding)
	}

	if religion == nil {
		t.Fatal("expected a patient-religion extension, got none")
	}
	cc, _ := religion["valueCodeableConcept"].(map[string]interface{})
	codings, _ := cc["coding"].([]interface{})
	if len(codings) == 0 {
		t.Fatal("religion valueCodeableConcept has no coding[]")
	}
	coding, _ := codings[0].(map[string]interface{})
	if coding["code"] != "1013" {
		t.Errorf("religion coding.code = %v, want 1013", coding["code"])
	}
	if coding["system"] != "http://terminology.hl7.org/CodeSystem/v3-ReligiousAffiliation" {
		t.Errorf("religion coding.system = %v, want v3-ReligiousAffiliation", coding["system"])
	}
	// religiousAffiliationCode commonly has no displayName attribute in real
	// CDA documents -- display must fall back to the bare code, not vanish.
	if cc["text"] != "1013" {
		t.Errorf("religion text = %v, want fallback to code 1013", cc["text"])
	}
}

func TestInjectPatientExtensions_NoHeader_NoOp(t *testing.T) {
	patient := map[string]interface{}{"resourceType": "Patient"}
	builder := cdafhir.NewUSCoreProfileBuilder()
	builder.InjectPatientExtensions(patient, nil)
	if _, ok := patient["extension"]; ok {
		t.Error("expected no extension array when header is nil")
	}
}

func TestInjectPatientExtensions_AlreadyPresent_NotDuplicated(t *testing.T) {
	header := patientHeaderMap(t, cdadocument.CDAPatient{
		Race: cdadocument.CDACode{Code: "2106-3", DisplayName: "White"},
	})
	existing := map[string]interface{}{"url": "http://hl7.org/fhir/us/core/StructureDefinition/us-core-race"}
	patient := map[string]interface{}{
		"resourceType": "Patient",
		"extension":    []interface{}{existing},
	}
	builder := cdafhir.NewUSCoreProfileBuilder()
	builder.InjectPatientExtensions(patient, header)

	exts, _ := patient["extension"].([]interface{})
	if len(exts) != 1 {
		t.Errorf("extension count = %d, want 1 (no duplicate us-core-race)", len(exts))
	}
}

// ---- Condition category-aware profile selection ----
// Regression coverage for the bug found while auditing Encounter/Condition
// linkage: profileEncounterDiagnosis (us-core-condition-encounter-diagnosis)
// was defined but never wired into any lookup, so every Condition --
// including encounter-diagnosis ones -- got the generic
// us-core-condition-problems-health-concerns profile regardless of category.

func conditionWithCategory(code string) map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Condition",
		"category": []interface{}{
			map[string]interface{}{
				"coding": []interface{}{
					map[string]interface{}{
						"system": "http://terminology.hl7.org/CodeSystem/condition-category",
						"code":   code,
					},
				},
			},
		},
	}
}

func profileURLs(t *testing.T, r map[string]interface{}) []string {
	t.Helper()
	meta, _ := r["meta"].(map[string]interface{})
	profiles, _ := meta["profile"].([]interface{})
	var urls []string
	for _, p := range profiles {
		if s, ok := p.(string); ok {
			urls = append(urls, s)
		}
	}
	return urls
}

func TestInjectProfile_Condition_EncounterDiagnosis_GetsItsOwnProfile(t *testing.T) {
	cond := conditionWithCategory("encounter-diagnosis")
	cdafhir.NewUSCoreProfileBuilder().InjectProfile(cond, "")

	urls := profileURLs(t, cond)
	if len(urls) != 1 || urls[0] != "http://hl7.org/fhir/us/core/StructureDefinition/us-core-condition-encounter-diagnosis" {
		t.Errorf("profile = %v, want exactly [us-core-condition-encounter-diagnosis]", urls)
	}
}

func TestInjectProfile_Condition_ProblemListItem_GetsGenericProfile(t *testing.T) {
	cond := conditionWithCategory("problem-list-item")
	cdafhir.NewUSCoreProfileBuilder().InjectProfile(cond, "")

	urls := profileURLs(t, cond)
	if len(urls) != 1 || urls[0] != "http://hl7.org/fhir/us/core/StructureDefinition/us-core-condition-problems-health-concerns" {
		t.Errorf("profile = %v, want exactly [us-core-condition-problems-health-concerns] (no category override for this category)", urls)
	}
}

func TestInjectProfile_Condition_NoCategory_GetsGenericProfile(t *testing.T) {
	cond := map[string]interface{}{"resourceType": "Condition"}
	cdafhir.NewUSCoreProfileBuilder().InjectProfile(cond, "")

	urls := profileURLs(t, cond)
	if len(urls) != 1 || urls[0] != "http://hl7.org/fhir/us/core/StructureDefinition/us-core-condition-problems-health-concerns" {
		t.Errorf("profile = %v, want exactly [us-core-condition-problems-health-concerns]", urls)
	}
}

func TestInjectProfile_Condition_TemplateOverride_TakesPrecedence(t *testing.T) {
	cond := conditionWithCategory("encounter-diagnosis")
	cdafhir.NewUSCoreProfileBuilder().InjectProfile(cond, "http://example.org/custom-profile")

	urls := profileURLs(t, cond)
	if len(urls) != 1 || urls[0] != "http://example.org/custom-profile" {
		t.Errorf("profile = %v, want exactly [http://example.org/custom-profile] (explicit templateProfile must win over category inference)", urls)
	}
}

// ---- Observation category-aware profile selection ----
// Regression coverage for the bug found while auditing Mental Status:
// profileObservationVitals/profileObservationLab (us-core-vital-signs /
// us-core-observation-lab) were declared from the start but never wired
// into any lookup, so EVERY Observation -- vitals, labs, social history,
// functional status, cognitive status alike -- got the generic
// us-core-observation-clinical-result profile regardless of category.

func observationWithCategoryCode(code string) map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Observation",
		"category": []interface{}{
			map[string]interface{}{
				"coding": []interface{}{
					map[string]interface{}{
						"system": "http://terminology.hl7.org/CodeSystem/observation-category",
						"code":   code,
					},
				},
			},
		},
	}
}

func TestInjectProfile_Observation_VitalSigns_GetsVitalSignsProfile(t *testing.T) {
	obs := observationWithCategoryCode("vital-signs")
	cdafhir.NewUSCoreProfileBuilder().InjectProfile(obs, "")

	urls := profileURLs(t, obs)
	if len(urls) != 1 || urls[0] != "http://hl7.org/fhir/us/core/StructureDefinition/us-core-vital-signs" {
		t.Errorf("profile = %v, want exactly [us-core-vital-signs]", urls)
	}
}

func TestInjectProfile_Observation_Laboratory_GetsLabProfile(t *testing.T) {
	obs := observationWithCategoryCode("laboratory")
	cdafhir.NewUSCoreProfileBuilder().InjectProfile(obs, "")

	urls := profileURLs(t, obs)
	if len(urls) != 1 || urls[0] != "http://hl7.org/fhir/us/core/StructureDefinition/us-core-observation-lab" {
		t.Errorf("profile = %v, want exactly [us-core-observation-lab]", urls)
	}
}

func TestInjectProfile_Observation_ScreeningAssessmentCategories_GetScreeningAssessmentProfile(t *testing.T) {
	for _, code := range []string{"social-history", "functional-status", "cognitive-status"} {
		t.Run(code, func(t *testing.T) {
			obs := observationWithCategoryCode(code)
			cdafhir.NewUSCoreProfileBuilder().InjectProfile(obs, "")

			urls := profileURLs(t, obs)
			if len(urls) != 1 || urls[0] != "http://hl7.org/fhir/us/core/StructureDefinition/us-core-observation-screening-assessment" {
				t.Errorf("category=%s: profile = %v, want exactly [us-core-observation-screening-assessment]", code, urls)
			}
		})
	}
}

func TestInjectProfile_Observation_NoCategory_GetsGenericClinicalResultProfile(t *testing.T) {
	obs := map[string]interface{}{"resourceType": "Observation"}
	cdafhir.NewUSCoreProfileBuilder().InjectProfile(obs, "")

	urls := profileURLs(t, obs)
	if len(urls) != 1 || urls[0] != "http://hl7.org/fhir/us/core/StructureDefinition/us-core-observation-clinical-result" {
		t.Errorf("profile = %v, want exactly [us-core-observation-clinical-result] (fallback for an uncategorized Observation)", urls)
	}
}

func TestInjectProfile_Observation_UnrecognizedCategory_GetsGenericClinicalResultProfile(t *testing.T) {
	obs := observationWithCategoryCode("exam")
	cdafhir.NewUSCoreProfileBuilder().InjectProfile(obs, "")

	urls := profileURLs(t, obs)
	if len(urls) != 1 || urls[0] != "http://hl7.org/fhir/us/core/StructureDefinition/us-core-observation-clinical-result" {
		t.Errorf("profile = %v, want exactly [us-core-observation-clinical-result] (no override defined for this category)", urls)
	}
}

func TestInjectProfile_Observation_TemplateOverride_TakesPrecedence(t *testing.T) {
	obs := observationWithCategoryCode("vital-signs")
	cdafhir.NewUSCoreProfileBuilder().InjectProfile(obs, "http://example.org/custom-profile")

	urls := profileURLs(t, obs)
	if len(urls) != 1 || urls[0] != "http://example.org/custom-profile" {
		t.Errorf("profile = %v, want exactly [http://example.org/custom-profile] (explicit templateProfile must win over category inference)", urls)
	}
}
