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
