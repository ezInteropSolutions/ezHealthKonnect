// services/cda_fhir/declarative_oob_rules_internal_test.go
//
// Internal-package tests (mirrors declarative_rules_flatten_test.go's own
// "package cdafhir" style) for applyPlanOfCareEncounterTarget and
// isPlanOfCareSectionKey -- both unexported, only ever called by
// DeclarativeMapDocument on an already-cloned []MappingRule (see that
// function's planOfCareEncounterTarget resolution in
// declarative_document_mapper.go), so they can't be exercised from the
// external cdafhir_test package the rest of this file's sibling tests use.
package cdafhir

import (
	"encoding/json"
	"testing"

	cdadocument "ezhealthkonnect/cda/document"
)

// internalDocumentMapForEntries mirrors declarative_oob_rules_test.go's own
// documentMapForEntries -- duplicated here (not exported/shared) because
// that helper lives in the external cdafhir_test package and this file
// needs package-internal access to applyPlanOfCareEncounterTarget.
func internalDocumentMapForEntries(t testing.TB, sectionKey string, entries []cdadocument.CDAEntry) map[string]interface{} {
	t.Helper()
	doc := map[string]interface{}{
		"sectionsByKey": map[string]interface{}{
			sectionKey: map[string]interface{}{"entries": entries},
		},
	}
	encoded, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("marshalling test document: %v", err)
	}
	var documentMap map[string]interface{}
	if err := json.Unmarshal(encoded, &documentMap); err != nil {
		t.Fatalf("unmarshalling test document: %v", err)
	}
	return documentMap
}

func TestApplyPlanOfCareEncounterTarget_SwapsAppointmentRuleToEncounter(t *testing.T) {
	rules := CloneMappingRules(PlanOfCareMappingRules())
	swapped := applyPlanOfCareEncounterTarget(rules)

	found := false
	for _, r := range swapped {
		if r.EntryMatch != "entryType=encounter" {
			continue
		}
		found = true
		if r.FHIRResource != "Encounter" {
			t.Errorf("FHIRResource = %q, want \"Encounter\"", r.FHIRResource)
		}
		if len(r.Fields) != len(EncounterMappingRules()[0].Fields) {
			t.Errorf("got %d fields, want the same %d as encounterFields()", len(r.Fields), len(EncounterMappingRules()[0].Fields))
		}
		// Real bug caught via live verification against the Ascension file
		// (not by this suite's other tests, which call BuildResourcesForRules
		// directly and never set DeclarativeEngine.PatientRef -- so a missing
		// PatientRefPath never showed up as a missing "subject" there): the
		// swap must also carry over PatientRefPath, or a swapped Encounter
		// never gets its required (US Core 1..1) subject reference.
		if len(r.PatientRefPath) != 1 || r.PatientRefPath[0] != "subject" {
			t.Errorf("PatientRefPath = %v, want [\"subject\"] (same as EncounterMappingRules())", r.PatientRefPath)
		}
	}
	if !found {
		t.Fatal("no entryType=encounter rule found after swap")
	}
}

func TestApplyPlanOfCareEncounterTarget_NotCalled_KeepsAppointmentByDefault(t *testing.T) {
	// PlanOfCareMappingRules()'s own Go literal is untouched by V183/this
	// feature -- it must still return Appointment when the swap function is
	// never invoked (i.e. the resolved runtime target was "Appointment").
	for _, r := range PlanOfCareMappingRules() {
		if r.EntryMatch == "entryType=encounter" {
			if r.FHIRResource != "Appointment" {
				t.Errorf("FHIRResource = %q, want \"Appointment\" (Go literal default, unchanged)", r.FHIRResource)
			}
			return
		}
	}
	t.Fatal("no entryType=encounter rule found in PlanOfCareMappingRules()")
}

func TestIsPlanOfCareSectionKey(t *testing.T) {
	for _, k := range []string{"carePlan", "planOfCare", "assessmentAndPlan", "planOfTreatment"} {
		if !isPlanOfCareSectionKey(k) {
			t.Errorf("isPlanOfCareSectionKey(%q) = false, want true", k)
		}
	}
	if isPlanOfCareSectionKey("encounters") {
		t.Error("isPlanOfCareSectionKey(\"encounters\") = true, want false")
	}
}

func TestDeclarativeEngine_PlanOfCare_EncounterTarget_Encounter_ProducesEncounterResource(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{EntryType: "encounter", MoodCode: "APT", StatusCode: "active", Code: cdadocument.CDACode{Code: "AMB"}},
	}
	documentMap := internalDocumentMapForEntries(t, "planOfTreatment", entries)
	rules := CloneMappingRules(PlanOfCareMappingRules())
	rules = applyPlanOfCareEncounterTarget(rules)

	engine := NewDeclarativeEngine()
	resources, errs := engine.BuildResourcesForRules(documentMap, rules)
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	if resources[0]["resourceType"] != "Encounter" {
		t.Errorf("resourceType = %v, want Encounter", resources[0]["resourceType"])
	}
	if resources[0]["status"] != "planned" {
		t.Errorf("status = %v, want planned", resources[0]["status"])
	}
}
