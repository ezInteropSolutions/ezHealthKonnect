package transforms

import "testing"

const allergyClinicalSystem = "http://terminology.hl7.org/CodeSystem/allergyintolerance-clinical"
const conditionClinicalSystem = "http://terminology.hl7.org/CodeSystem/condition-clinical"

func TestAllergyStatusToFHIR_UsesAllergyClinicalCodeSystem(t *testing.T) {
	cc := AllergyStatusToFHIR("active")
	codings, _ := cc["coding"].([]interface{})
	if len(codings) == 0 {
		t.Fatal("AllergyStatusToFHIR produced no coding")
	}
	coding := codings[0].(map[string]interface{})
	if coding["system"] != allergyClinicalSystem {
		t.Errorf("AllergyIntolerance.clinicalStatus system = %v, want %q (not Condition's %q)",
			coding["system"], allergyClinicalSystem, conditionClinicalSystem)
	}
}

func TestConditionStatusToFHIR_StillUsesConditionClinicalCodeSystem(t *testing.T) {
	cc := ConditionStatusToFHIR("active")
	codings, _ := cc["coding"].([]interface{})
	coding := codings[0].(map[string]interface{})
	if coding["system"] != conditionClinicalSystem {
		t.Errorf("Condition.clinicalStatus system = %v, want %q", coding["system"], conditionClinicalSystem)
	}
}

// TestAllergyReactionSeverityToFHIR_MatchesExtractedGoBehavior pins the
// exact case table allergy_mapper.go's now-deleted private allergySeverityCode
// switch used to hold, including the already-documented, deliberately-NOT-
// fixed gap (255604002 "Very Mild" falls through to "moderate" -- see
// architecture/CDA_FHIR_MAPPING_INVENTORY.md's cross-cutting finding #8).
// This is an extract-method move, not a behavior change; this test is what
// proves that.
func TestAllergyReactionSeverityToFHIR_MatchesExtractedGoBehavior(t *testing.T) {
	cases := map[string]string{
		"371924009": "mild",
		"Mild":      "mild",
		"6736007":   "moderate",
		"Moderate":  "moderate",
		"24484000":  "severe",
		"Severe":    "severe",
		// Known, already-flagged gap -- preserved verbatim, not fixed here.
		"255604002": "moderate",
		"":          "moderate",
	}
	for input, want := range cases {
		if got := AllergyReactionSeverityToFHIR(input); got != want {
			t.Errorf("AllergyReactionSeverityToFHIR(%q) = %q, want %q", input, got, want)
		}
	}
}

// ---- Verified against HL7's official C-CDA-on-FHIR ConceptMaps (see doc comments on
// each function for the exact source) -- not assumed, not derived from sample documents. ----

func TestProcedureStatusToFHIR_MatchesOfficialConceptMap(t *testing.T) {
	cases := map[string]string{
		"completed": "completed",
		"active":    "in-progress",
		"aborted":   "stopped",  // distinct from cancelled -- this was the bug
		"cancelled": "not-done", // distinct from aborted
	}
	for cdaStatus, want := range cases {
		if got := ProcedureStatusToFHIR(cdaStatus); got != want {
			t.Errorf("ProcedureStatusToFHIR(%q) = %q, want %q (per ConceptMap/CF-ProcedureStatus)", cdaStatus, got, want)
		}
	}
}

func TestMedicationRequestStatusToFHIR_MatchesOfficialConceptMap(t *testing.T) {
	cases := map[string]string{
		"active":    "active",
		"suspended": "on-hold",
		"aborted":   "stopped", // was incorrectly "cancelled" before this fix
		"completed": "completed",
		"nullified": "entered-in-error",
		"cancelled": "cancelled", // no official entry; valid distinct code on this resource only
	}
	for cdaStatus, want := range cases {
		if got := MedicationRequestStatusToFHIR(cdaStatus); got != want {
			t.Errorf("MedicationRequestStatusToFHIR(%q) = %q, want %q (per ConceptMap/CF-MedicationStatus)", cdaStatus, got, want)
		}
	}
}

func TestMedicationStatusToFHIR_NoOfficialMapping_UsesValidValueSetOnly(t *testing.T) {
	// MedicationStatement.status has no "cancelled" code at all (R4 valueset:
	// active/completed/entered-in-error/intended/stopped/on-hold/unknown/not-taken).
	// Both CDA "aborted" and "cancelled" must land on a code that actually exists
	// in this value set -- "stopped", not "cancelled".
	if got := MedicationStatusToFHIR("cancelled"); got != "stopped" {
		t.Errorf(`MedicationStatusToFHIR("cancelled") = %q, want "stopped" -- MedicationStatement has no "cancelled" status code`, got)
	}
	if got := MedicationStatusToFHIR("nullified"); got != "entered-in-error" {
		t.Errorf(`MedicationStatusToFHIR("nullified") = %q, want "entered-in-error"`, got)
	}
}
