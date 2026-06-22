package mappers

import (
	"testing"

	cdadocument "ezhealthkonnect/cda/document"
)

const testPatientRef = "Patient/patient-1"

// ---- Fix 4: US Core category CodeSystem split ----

func TestCategorySystem_USCoreSpecificCodes(t *testing.T) {
	for _, code := range []string{"functional-status", "cognitive-status", "disability-status", "sdoh"} {
		if got := categorySystem(code); got != usCoreObservationCategorySystem {
			t.Errorf("categorySystem(%q) = %q, want %q", code, got, usCoreObservationCategorySystem)
		}
	}
}

func TestCategorySystem_BaseFHIRCodes(t *testing.T) {
	for _, code := range []string{"vital-signs", "laboratory", "social-history", "survey"} {
		if got := categorySystem(code); got != baseObservationCategorySystem {
			t.Errorf("categorySystem(%q) = %q, want %q", code, got, baseObservationCategorySystem)
		}
	}
}

func TestMapObservations_FunctionalStatusCategory_UsesUSCoreCodeSystem(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{EntryType: "observation", StatusCode: "completed", Value: &cdadocument.CDAValue{Type: "ST", Text: "64"}},
	}
	resources := MapObservations(entries, testPatientRef, "functional-status")
	if len(resources) != 1 {
		t.Fatalf("expected 1 Observation, got %d", len(resources))
	}
	system := categoryCodingSystem(t, resources[0])
	if system != usCoreObservationCategorySystem {
		t.Errorf("category coding system = %q, want %q", system, usCoreObservationCategorySystem)
	}
}

func TestMapObservations_InterpretationCode_DirectChild_SetsInterpretation(t *testing.T) {
	// CONF:1198-7147 -- interpretationCode is a direct child of the
	// observation, a sibling of code/statusCode/value, not nested under a
	// COMP entryRelationship (the prior, IG-incorrect assumption). Real
	// Kareo corpus data has this direct-sibling shape.
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "observation",
			StatusCode: "completed",
			Value:      &cdadocument.CDAValue{Type: "ST", Text: "7.2"},
			InterpretationCode: cdadocument.CDACode{
				Code: "L", CodeSystem: "2.16.840.1.113883.5.83", DisplayName: "Low",
			},
		},
	}
	resources := MapObservations(entries, testPatientRef, "laboratory")
	if len(resources) != 1 {
		t.Fatalf("expected 1 Observation, got %d", len(resources))
	}
	interp, _ := resources[0]["interpretation"].([]interface{})
	if len(interp) != 1 {
		t.Fatalf("expected 1 interpretation entry, got %d", len(interp))
	}
	cc, _ := interp[0].(map[string]interface{})
	codings, _ := cc["coding"].([]interface{})
	coding, _ := codings[0].(map[string]interface{})
	if coding["code"] != "L" {
		t.Errorf("interpretation[0].coding[0].code = %v, want L", coding["code"])
	}
}

func TestMapObservations_NoInterpretationCode_NoInterpretation(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{EntryType: "observation", StatusCode: "completed", Value: &cdadocument.CDAValue{Type: "ST", Text: "98.6"}},
	}
	resources := MapObservations(entries, testPatientRef, "vital-signs")
	if len(resources) != 1 {
		t.Fatalf("expected 1 Observation, got %d", len(resources))
	}
	if _, has := resources[0]["interpretation"]; has {
		t.Error("expected no interpretation when interpretationCode is absent")
	}
}

func categoryCodingSystem(t *testing.T, r map[string]interface{}) string {
	t.Helper()
	cats, _ := r["category"].([]interface{})
	if len(cats) == 0 {
		t.Fatal("no category set on Observation")
	}
	cat, _ := cats[0].(map[string]interface{})
	codings, _ := cat["coding"].([]interface{})
	if len(codings) == 0 {
		t.Fatal("no coding in category")
	}
	coding, _ := codings[0].(map[string]interface{})
	system, _ := coding["system"].(string)
	return system
}

// ---- Fix 5: Blood pressure panel assembly ----

func TestMapObservations_BloodPressurePair_CombinedIntoSinglePanel(t *testing.T) {
	organizer := cdadocument.CDAEntry{
		EntryType: "organizer",
		Components: []cdadocument.CDAEntry{
			{
				EntryType:  "observation",
				StatusCode: "completed",
				Code:       cdadocument.CDACode{Code: loincSystolicBP, CodeSystem: "2.16.840.1.113883.6.1"},
				Value:      &cdadocument.CDAValue{Type: "PQ", Quantity: &cdadocument.CDAQuantity{Value: "124", Unit: "mm[Hg]"}},
			},
			{
				EntryType:  "observation",
				StatusCode: "completed",
				Code:       cdadocument.CDACode{Code: loincDiastolicBP, CodeSystem: "2.16.840.1.113883.6.1"},
				Value:      &cdadocument.CDAValue{Type: "PQ", Quantity: &cdadocument.CDAQuantity{Value: "72", Unit: "mm[Hg]"}},
			},
		},
	}

	resources := MapObservations([]cdadocument.CDAEntry{organizer}, testPatientRef, "vital-signs")
	if len(resources) != 1 {
		t.Fatalf("expected exactly 1 combined BP Observation, got %d", len(resources))
	}

	r := resources[0]
	code, _ := r["code"].(map[string]interface{})
	codings, _ := code["coding"].([]interface{})
	if len(codings) == 0 {
		t.Fatal("BP Observation has no code.coding")
	}
	coding, _ := codings[0].(map[string]interface{})
	if coding["code"] != loincBPPanel {
		t.Errorf("BP Observation.code.coding[0].code = %v, want %q", coding["code"], loincBPPanel)
	}

	components, _ := r["component"].([]interface{})
	if len(components) != 2 {
		t.Fatalf("BP Observation.component length = %d, want 2", len(components))
	}
}

func TestMapObservations_NonBPVitalSign_NotCombined(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "observation",
			StatusCode: "completed",
			Code:       cdadocument.CDACode{Code: "8302-2"}, // Body height — unrelated to BP
			Value:      &cdadocument.CDAValue{Type: "PQ", Quantity: &cdadocument.CDAQuantity{Value: "162.6", Unit: "cm"}},
		},
	}
	resources := MapObservations(entries, testPatientRef, "vital-signs")
	if len(resources) != 1 {
		t.Fatalf("expected 1 Observation (unchanged), got %d", len(resources))
	}
	if _, hasComponent := resources[0]["component"]; hasComponent {
		t.Error("non-BP vital sign should not have a component array")
	}
}

// ---- Fix 6: MedicationRequest.requester ----

func TestMapMedications_OrderIntent_RequesterFromPerformer(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "substanceAdministration",
			MoodCode:   "INT",
			StatusCode: "active",
			Performers: []cdadocument.CDAPerformer{
				{
					AssignedEntity: cdadocument.CDAAssignedEntity{
						AssignedPerson: &cdadocument.CDAPerson{
							Names: []cdadocument.CDAName{{Given: []string{"Halie"}, Family: "Lower"}},
						},
					},
				},
			},
		},
	}
	resources := MapMedications(entries, testPatientRef)
	if len(resources) != 1 {
		t.Fatalf("expected 1 MedicationRequest, got %d", len(resources))
	}
	requester, ok := resources[0]["requester"].(map[string]interface{})
	if !ok {
		t.Fatal("MedicationRequest.requester not set")
	}
	if requester["display"] != "Halie Lower" {
		t.Errorf("requester.display = %v, want %q", requester["display"], "Halie Lower")
	}
}

func TestMapMedications_OrderIntent_RequesterFallback_NeverEmpty(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{EntryType: "substanceAdministration", MoodCode: "INT", StatusCode: "active"},
	}
	resources := MapMedications(entries, testPatientRef)
	if len(resources) != 1 {
		t.Fatalf("expected 1 MedicationRequest, got %d", len(resources))
	}
	requester, ok := resources[0]["requester"].(map[string]interface{})
	if !ok {
		t.Fatal("MedicationRequest.requester must always be set when intent=order (us-core-21)")
	}
	if display, _ := requester["display"].(string); display == "" {
		t.Error("requester.display must not be empty")
	}
}

// ---- Fix 8: AllergyIntolerance.code fallback ----

func TestMapAllergies_NoKnownAllergies_CodeFallsBackToAssertionValue(t *testing.T) {
	// Mirrors the real-world "No Known Allergies" idiom: the SUBJ-related
	// observation has no CSM substance participant, only its own assertion
	// value, and is negated (negationInd="true" — this is an assertion of
	// absence, not a confirmed allergy).
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "act",
			StatusCode: "active",
			EntryRelationships: []cdadocument.CDAEntryRelationship{
				{
					TypeCode: "SUBJ",
					Entry: cdadocument.CDAEntry{
						EntryType:   "observation",
						StatusCode:  "completed",
						NegationInd: true,
						Value: &cdadocument.CDAValue{
							Type: "CD",
							Code: &cdadocument.CDACode{
								Code:        "419199007",
								CodeSystem:  "2.16.840.1.113883.6.96",
								DisplayName: "Allergy to substance (disorder)",
							},
						},
					},
				},
			},
		},
	}
	resources := MapAllergies(entries, testPatientRef)
	if len(resources) != 1 {
		t.Fatalf("expected 1 AllergyIntolerance, got %d", len(resources))
	}
	if _, hasCode := resources[0]["code"]; !hasCode {
		t.Error("AllergyIntolerance.code must be set (minimum required = 1) even with no CSM substance participant")
	}
	verification, _ := resources[0]["verificationStatus"].(map[string]interface{})
	coding, _ := verification["coding"].([]interface{})
	if len(coding) != 1 {
		t.Fatalf("expected exactly one verificationStatus coding, got %d", len(coding))
	}
	if got := coding[0].(map[string]interface{})["code"]; got != "refuted" {
		t.Errorf("verificationStatus.code = %v, want %q for a negated assertion", got, "refuted")
	}
}

func TestMapAllergies_NotNegated_VerificationStatusConfirmed(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "act",
			StatusCode: "active",
			EntryRelationships: []cdadocument.CDAEntryRelationship{
				{
					TypeCode: "SUBJ",
					Entry: cdadocument.CDAEntry{
						EntryType:  "observation",
						StatusCode: "completed",
						Participants: []cdadocument.CDAParticipant{
							{
								TypeCode: "CSM",
								ParticipantRole: cdadocument.CDAParticipantRole{
									PlayingEntity: &cdadocument.CDAPlayingEntity{
										Code: cdadocument.CDACode{Code: "7980", DisplayName: "Penicillin", CodeSystem: "2.16.840.1.113883.6.88"},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	resources := MapAllergies(entries, testPatientRef)
	if len(resources) != 1 {
		t.Fatalf("expected 1 AllergyIntolerance, got %d", len(resources))
	}
	verification, _ := resources[0]["verificationStatus"].(map[string]interface{})
	coding, _ := verification["coding"].([]interface{})
	if len(coding) != 1 || coding[0].(map[string]interface{})["code"] != "confirmed" {
		t.Errorf("expected verificationStatus.code = confirmed for a non-negated allergy, got %v", coding)
	}
}

// ---- Phase 2: Condition negation -> verificationStatus=refuted + severity ----

func TestMapConditions_NegatedProblem_VerificationStatusRefuted(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "act",
			StatusCode: "active",
			EntryRelationships: []cdadocument.CDAEntryRelationship{
				{
					TypeCode: "SUBJ",
					Entry: cdadocument.CDAEntry{
						EntryType:   "observation",
						StatusCode:  "completed",
						NegationInd: true,
						Value: &cdadocument.CDAValue{
							Type: "CD",
							Code: &cdadocument.CDACode{Code: "64572001", DisplayName: "No known problems", CodeSystem: "2.16.840.1.113883.6.96"},
						},
					},
				},
			},
		},
	}
	resources := MapConditions(entries, testPatientRef, "problem-list-item")
	if len(resources) != 1 {
		t.Fatalf("expected 1 Condition, got %d", len(resources))
	}
	verification, _ := resources[0]["verificationStatus"].(map[string]interface{})
	coding, _ := verification["coding"].([]interface{})
	if len(coding) != 1 || coding[0].(map[string]interface{})["code"] != "refuted" {
		t.Errorf("verificationStatus.code = %v, want refuted for a negated problem", coding)
	}
}

func TestMapConditions_SeverityFromNestedSUBJObservation(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "act",
			StatusCode: "active",
			EntryRelationships: []cdadocument.CDAEntryRelationship{
				{
					TypeCode: "SUBJ",
					Entry: cdadocument.CDAEntry{
						EntryType:  "observation",
						StatusCode: "active",
						Value: &cdadocument.CDAValue{
							Type: "CD",
							Code: &cdadocument.CDACode{Code: "38341003", DisplayName: "Hypertension", CodeSystem: "2.16.840.1.113883.6.96"},
						},
						EntryRelationships: []cdadocument.CDAEntryRelationship{
							{
								TypeCode: "SUBJ",
								Entry: cdadocument.CDAEntry{
									EntryType: "observation",
									Code:      cdadocument.CDACode{Code: "SEV"},
									Value: &cdadocument.CDAValue{
										Type: "CD",
										Code: &cdadocument.CDACode{Code: "24484000", DisplayName: "Severe", CodeSystem: "2.16.840.1.113883.6.96"},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	resources := MapConditions(entries, testPatientRef, "problem-list-item")
	if len(resources) != 1 {
		t.Fatalf("expected 1 Condition, got %d", len(resources))
	}
	severity, ok := resources[0]["severity"].(map[string]interface{})
	if !ok {
		t.Fatal("expected Condition.severity to be set from the nested SEV observation")
	}
	coding, _ := severity["coding"].([]interface{})
	if len(coding) != 1 || coding[0].(map[string]interface{})["code"] != "24484000" {
		t.Errorf("severity coding = %v, want SNOMED 24484000 (Severe)", coding)
	}
}

// ---- Condition.category: problems vs healthConcerns (US Core requires different
// categories per section -- both previously got "problem-list-item" unconditionally) ----

func TestMapConditions_ProblemListItem_UsesBaseCodeSystem(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{EntryType: "act", StatusCode: "active", Value: &cdadocument.CDAValue{Type: "CD", Code: &cdadocument.CDACode{Code: "38341003"}}},
	}
	resources := MapConditions(entries, testPatientRef, "problem-list-item")
	if len(resources) != 1 {
		t.Fatalf("expected 1 Condition, got %d", len(resources))
	}
	cat := categoryCodingSystem(t, resources[0])
	const baseConditionCategorySystem = "http://terminology.hl7.org/CodeSystem/condition-category"
	if cat != baseConditionCategorySystem {
		t.Errorf("category coding system = %q, want %q for problem-list-item", cat, baseConditionCategorySystem)
	}
	coding, _ := resources[0]["category"].([]interface{})[0].(map[string]interface{})["coding"].([]interface{})
	if coding[0].(map[string]interface{})["code"] != "problem-list-item" {
		t.Errorf("category coding code = %v, want problem-list-item", coding[0])
	}
}

func TestMapConditions_HealthConcern_UsesUSCoreCodeSystemAndCode(t *testing.T) {
	// Regression guard for the latent bug found in Phase 0: MapConditions is
	// shared by the "problems" and "healthConcerns" dispatch table entries
	// (document_mapper.go), but used to hardcode category="problem-list-item"
	// unconditionally -- a Health Concern entry would have been misclassified.
	// Per the published US Core "US Core Problem or Health Concern" value set,
	// "health-concern" lives in a DIFFERENT CodeSystem from "problem-list-item",
	// not just a different code in the same one.
	entries := []cdadocument.CDAEntry{
		{EntryType: "act", StatusCode: "active", Value: &cdadocument.CDAValue{Type: "CD", Code: &cdadocument.CDACode{Code: "44261-6"}}},
	}
	resources := MapConditions(entries, testPatientRef, "health-concern")
	if len(resources) != 1 {
		t.Fatalf("expected 1 Condition, got %d", len(resources))
	}
	const usCoreConditionCategorySystem = "http://hl7.org/fhir/us/core/CodeSystem/condition-category"
	cat := categoryCodingSystem(t, resources[0])
	if cat != usCoreConditionCategorySystem {
		t.Errorf("category coding system = %q, want %q for health-concern", cat, usCoreConditionCategorySystem)
	}
	coding, _ := resources[0]["category"].([]interface{})[0].(map[string]interface{})["coding"].([]interface{})
	if coding[0].(map[string]interface{})["code"] != "health-concern" {
		t.Errorf("category coding code = %v, want health-concern", coding[0])
	}
}

// ---- Phase 2: Medication dosing frequency (PIVL_TS) + RSON indication ----

func TestMapMedications_PIVLFrequency_SetsDosageTimingRepeat(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "substanceAdministration",
			MoodCode:   "EVN",
			StatusCode: "active",
			RouteCode:  &cdadocument.CDACode{Code: "C38288", DisplayName: "ORAL"},
			EffectiveTimes: []cdadocument.CDAEffectiveTimeEntry{
				{XSIType: "IVL_TS", Range: cdadocument.CDATimeRange{Low: cdadocument.CDATime{Value: "20240101"}}},
				{XSIType: "PIVL_TS", Period: &cdadocument.CDAQuantity{Value: "12", Unit: "h"}},
			},
		},
	}
	resources := MapMedications(entries, testPatientRef)
	if len(resources) != 1 {
		t.Fatalf("expected 1 MedicationStatement, got %d", len(resources))
	}
	dosages, _ := resources[0]["dosage"].([]interface{})
	if len(dosages) != 1 {
		t.Fatalf("expected 1 dosage entry, got %d", len(dosages))
	}
	dosage := dosages[0].(map[string]interface{})
	timing, ok := dosage["timing"].(map[string]interface{})
	if !ok {
		t.Fatal("expected dosage.timing to be set from the PIVL_TS effectiveTime")
	}
	repeat := timing["repeat"].(map[string]interface{})
	if repeat["period"] != "12" || repeat["periodUnit"] != "h" {
		t.Errorf("repeat = %v, want period=12 periodUnit=h", repeat)
	}
}

func TestMapMedications_RSONIndication_SetsReasonCode(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "substanceAdministration",
			MoodCode:   "EVN",
			StatusCode: "active",
			EntryRelationships: []cdadocument.CDAEntryRelationship{
				{
					TypeCode: "RSON",
					Entry: cdadocument.CDAEntry{
						EntryType: "observation",
						Value: &cdadocument.CDAValue{
							Type: "CD",
							Code: &cdadocument.CDACode{Code: "38341003", DisplayName: "Hypertension", CodeSystem: "2.16.840.1.113883.6.96"},
						},
					},
				},
			},
		},
	}
	resources := MapMedications(entries, testPatientRef)
	if len(resources) != 1 {
		t.Fatalf("expected 1 MedicationStatement, got %d", len(resources))
	}
	reasons, ok := resources[0]["reasonCode"].([]interface{})
	if !ok || len(reasons) != 1 {
		t.Fatalf("expected 1 reasonCode from the RSON indication, got %v", resources[0]["reasonCode"])
	}
	coding, _ := reasons[0].(map[string]interface{})["coding"].([]interface{})
	if len(coding) != 1 || coding[0].(map[string]interface{})["code"] != "38341003" {
		t.Errorf("reasonCode coding = %v, want SNOMED 38341003 (Hypertension)", coding)
	}
}

func TestMapMedications_FreeTextSigAndInstructionV2_SetDosageTextFields(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "substanceAdministration",
			MoodCode:   "INT",
			StatusCode: "active",
			EntryRelationships: []cdadocument.CDAEntryRelationship{
				{
					TypeCode: "COMP",
					Entry: cdadocument.CDAEntry{
						EntryType:   "substanceAdministration",
						TemplateIds: []string{"2.16.840.1.113883.10.20.22.4.147"},
						Text:        "Take one tablet by mouth every morning",
					},
				},
				{
					TypeCode:     "SUBJ",
					InversionInd: true,
					Entry: cdadocument.CDAEntry{
						EntryType:   "act",
						TemplateIds: []string{"2.16.840.1.113883.10.20.22.4.20"},
						Text:        "Take with food",
					},
				},
			},
		},
	}
	resources := MapMedications(entries, testPatientRef)
	if len(resources) != 1 {
		t.Fatalf("expected 1 MedicationRequest, got %d", len(resources))
	}
	dosages, _ := resources[0]["dosageInstruction"].([]interface{})
	if len(dosages) != 1 {
		t.Fatalf("expected 1 dosageInstruction entry, got %d", len(dosages))
	}
	dosage := dosages[0].(map[string]interface{})
	if dosage["text"] != "Take one tablet by mouth every morning" {
		t.Errorf("dosage.text = %v, want Medication Free Text Sig content", dosage["text"])
	}
	if dosage["patientInstruction"] != "Take with food" {
		t.Errorf("dosage.patientInstruction = %v, want Instruction (V2) content", dosage["patientInstruction"])
	}
}

// ---- Fix 9: Encounter.class.display empty-string guard ----

func TestMapEncounters_EmptyDisplayName_OmittedNotEmpty(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{EntryType: "encounter", StatusCode: "completed", Code: cdadocument.CDACode{Code: "AMB", DisplayName: ""}},
	}
	resources := MapEncounters(entries, testPatientRef)
	if len(resources) != 1 {
		t.Fatalf("expected 1 Encounter, got %d", len(resources))
	}
	class, ok := resources[0]["class"].(map[string]interface{})
	if !ok {
		t.Fatal("Encounter.class not set")
	}
	if display, exists := class["display"]; exists {
		t.Errorf("Encounter.class.display must be omitted (not empty string) when source has no displayName, got %q", display)
	}
}

func TestMapEncounters_NonEmptyDisplayName_Kept(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{EntryType: "encounter", StatusCode: "completed", Code: cdadocument.CDACode{Code: "AMB", DisplayName: "Ambulatory"}},
	}
	resources := MapEncounters(entries, testPatientRef)
	class := resources[0]["class"].(map[string]interface{})
	if class["display"] != "Ambulatory" {
		t.Errorf("Encounter.class.display = %v, want %q", class["display"], "Ambulatory")
	}
}

// ---- Plan of Care: per-entry dispatch to ServiceRequest/MedicationRequest/Appointment/Goal ----

func TestMapPlanOfCare_PlannedProcedure_BecomesServiceRequest(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{EntryType: "procedure", MoodCode: "INT", StatusCode: "active", Code: cdadocument.CDACode{Code: "656", DisplayName: "Pap Test"}},
	}
	resources := MapPlanOfCare(entries, testPatientRef)
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	if resources[0]["resourceType"] != "ServiceRequest" {
		t.Errorf("resourceType = %v, want ServiceRequest", resources[0]["resourceType"])
	}
	if resources[0]["intent"] != "plan" {
		t.Errorf("intent = %v, want plan (from moodCode=INT)", resources[0]["intent"])
	}
}

func TestMapPlanOfCare_ProposedObservation_BecomesServiceRequestWithProposalIntent(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{EntryType: "observation", MoodCode: "PRP", StatusCode: "active", Code: cdadocument.CDACode{Code: "20", DisplayName: "Colorectal Cancer Screening"}},
	}
	resources := MapPlanOfCare(entries, testPatientRef)
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	if resources[0]["resourceType"] != "ServiceRequest" {
		t.Errorf("resourceType = %v, want ServiceRequest", resources[0]["resourceType"])
	}
	if resources[0]["intent"] != "proposal" {
		t.Errorf("intent = %v, want proposal (from moodCode=PRP)", resources[0]["intent"])
	}
}

func TestMapPlanOfCare_SubstanceAdministration_ReusesMedicationRequestBuilder(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{EntryType: "substanceAdministration", MoodCode: "INT", StatusCode: "active"},
	}
	resources := MapPlanOfCare(entries, testPatientRef)
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	if resources[0]["resourceType"] != "MedicationRequest" {
		t.Errorf("resourceType = %v, want MedicationRequest", resources[0]["resourceType"])
	}
	if _, hasRequester := resources[0]["requester"]; !hasRequester {
		t.Error("expected requester to be set (reused buildMedicationRequestResource, us-core-21)")
	}
}

func TestMapPlanOfCare_PlannedEncounter_BecomesAppointment(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{EntryType: "encounter", MoodCode: "INT", StatusCode: "active", Code: cdadocument.CDACode{Code: "99213"}},
	}
	resources := MapPlanOfCare(entries, testPatientRef)
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	if resources[0]["resourceType"] != "Appointment" {
		t.Errorf("resourceType = %v, want Appointment", resources[0]["resourceType"])
	}
}

func TestMapPlanOfCare_GoalMood_ReusesGoalBuilder(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{EntryType: "observation", MoodCode: "GOL", StatusCode: "active", Code: cdadocument.CDACode{Code: "8", DisplayName: "Weight loss goal"}},
	}
	resources := MapPlanOfCare(entries, testPatientRef)
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	if resources[0]["resourceType"] != "Goal" {
		t.Errorf("resourceType = %v, want Goal (moodCode=GOL takes priority over EntryType)", resources[0]["resourceType"])
	}
}

func TestMapPlanOfCare_EventMood_Skipped(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{EntryType: "procedure", MoodCode: "EVN", StatusCode: "completed", Code: cdadocument.CDACode{Code: "123"}},
	}
	resources := MapPlanOfCare(entries, testPatientRef)
	if len(resources) != 0 {
		t.Errorf("expected 0 resources for moodCode=EVN (already happened, not a plan entry), got %d", len(resources))
	}
}

func TestMapPlanOfCare_OrganizerComponents_AreFlattened(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType: "organizer",
			Components: []cdadocument.CDAEntry{
				{EntryType: "procedure", MoodCode: "INT", StatusCode: "active", Code: cdadocument.CDACode{Code: "1"}},
				{EntryType: "observation", MoodCode: "PRP", StatusCode: "active", Code: cdadocument.CDACode{Code: "2"}},
			},
		},
	}
	resources := MapPlanOfCare(entries, testPatientRef)
	if len(resources) != 2 {
		t.Fatalf("expected 2 resources (organizer flattened into its components), got %d", len(resources))
	}
}

// ---- Care Team: CareTeam + Practitioner participants ----

func TestMapCareTeam_BuildsCareTeamAndPractitioner(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "organizer",
			StatusCode: "active",
			Code:       cdadocument.CDACode{Code: "86744-0", DisplayName: "Care Team"},
			Participants: []cdadocument.CDAParticipant{
				{
					TypeCode:     "PPRF",
					FunctionCode: cdadocument.CDACode{Code: "PCP", DisplayName: "Primary Care Provider"},
					ParticipantRole: cdadocument.CDAParticipantRole{
						Ids: []cdadocument.CDAII{{Root: "2.16.840.1.113883.4.6", Extension: "1013027903"}},
					},
				},
			},
		},
	}
	resources := MapCareTeam(entries, testPatientRef)

	var practitioner, careTeam map[string]interface{}
	for _, r := range resources {
		switch r["resourceType"] {
		case "Practitioner":
			practitioner = r
		case "CareTeam":
			careTeam = r
		}
	}
	if practitioner == nil {
		t.Fatal("expected a Practitioner resource for the care team member")
	}
	if careTeam == nil {
		t.Fatal("expected a CareTeam resource")
	}
	participants, _ := careTeam["participant"].([]interface{})
	if len(participants) != 1 {
		t.Fatalf("CareTeam.participant length = %d, want 1", len(participants))
	}
	p := participants[0].(map[string]interface{})
	member, _ := p["member"].(map[string]interface{})
	wantRef := "Practitioner/" + practitioner["id"].(string)
	if member["reference"] != wantRef {
		t.Errorf("participant.member.reference = %v, want %v", member["reference"], wantRef)
	}
	role, _ := p["role"].([]interface{})
	if len(role) == 0 {
		t.Error("expected participant.role to carry the PCP functionCode")
	}
}

func TestMapCareTeam_NoParticipants_ProducesNoCareTeam(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{EntryType: "organizer", StatusCode: "active", Code: cdadocument.CDACode{Code: "86744-0"}},
	}
	resources := MapCareTeam(entries, testPatientRef)
	if len(resources) != 0 {
		t.Errorf("expected 0 resources when the organizer has no usable participants, got %d", len(resources))
	}
}

// ---- Fix 10: Organization custodian active flag ----

func TestMapCustodian_SetsActiveTrue(t *testing.T) {
	custodian := cdadocument.CDACustodian{
		AssignedCustodian: cdadocument.CDAAssignedCustodian{
			RepresentedCustodianOrganization: cdadocument.CDAOrganization{
				Names: []string{"Boulder Community Health and Affiliates"},
			},
		},
	}
	org := MapCustodian(custodian)
	if org == nil {
		t.Fatal("MapCustodian returned nil")
	}
	if active, _ := org["active"].(bool); !active {
		t.Error("Organization.active must be true (required by us-core-organization)")
	}
}

// ---- Immunization negationInd -> status="not-done" + statusReason (CONF:1198-8985) ----

func TestMapImmunizations_NotNegated_StatusCompleted(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "substanceAdministration",
			StatusCode: "completed",
			Consumable: &cdadocument.CDAConsumable{
				ManufacturedProduct: cdadocument.CDAManufacturedProduct{
					ManufacturedMaterial: &cdadocument.CDAMaterial{Code: cdadocument.CDACode{Code: "33", DisplayName: "Pneumococcal (PCV, PPSV)", CodeSystem: "2.16.840.1.113883.12.292"}},
				},
			},
		},
	}
	resources := MapImmunizations(entries, testPatientRef)
	if len(resources) != 1 {
		t.Fatalf("expected 1 Immunization, got %d", len(resources))
	}
	if resources[0]["status"] != "completed" {
		t.Errorf("status = %v, want completed for a non-negated immunization", resources[0]["status"])
	}
	if _, has := resources[0]["statusReason"]; has {
		t.Error("statusReason must not be set for a non-negated immunization")
	}
}

func TestMapImmunizations_NegationInd_StatusNotDone(t *testing.T) {
	// Mirrors the real Kareo corpus pattern: statusCode="completed" (the recording
	// act completed normally) but negationInd="true" (the vaccine itself was refused).
	// Before this fix, ImmunizationStatusToFHIR ignored NegationInd entirely and
	// reported "completed" -- i.e. asserted a vaccine was given when it was refused.
	entries := []cdadocument.CDAEntry{
		{
			EntryType:   "substanceAdministration",
			StatusCode:  "completed",
			NegationInd: true,
			Consumable: &cdadocument.CDAConsumable{
				ManufacturedProduct: cdadocument.CDAManufacturedProduct{
					ManufacturedMaterial: &cdadocument.CDAMaterial{Code: cdadocument.CDACode{Code: "33", DisplayName: "Pneumococcal (PCV, PPSV)", CodeSystem: "2.16.840.1.113883.12.292"}},
				},
			},
		},
	}
	resources := MapImmunizations(entries, testPatientRef)
	if len(resources) != 1 {
		t.Fatalf("expected 1 Immunization, got %d", len(resources))
	}
	if resources[0]["status"] != "not-done" {
		t.Errorf("status = %v, want not-done for a negated (refused) immunization", resources[0]["status"])
	}
}

func TestMapImmunizations_NegationIndWithRefusalReason_SetsStatusReason(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:   "substanceAdministration",
			StatusCode:  "completed",
			NegationInd: true,
			Consumable: &cdadocument.CDAConsumable{
				ManufacturedProduct: cdadocument.CDAManufacturedProduct{
					ManufacturedMaterial: &cdadocument.CDAMaterial{Code: cdadocument.CDACode{Code: "33", DisplayName: "Pneumococcal (PCV, PPSV)", CodeSystem: "2.16.840.1.113883.12.292"}},
				},
			},
			EntryRelationships: []cdadocument.CDAEntryRelationship{
				{
					TypeCode: "RSON",
					Entry: cdadocument.CDAEntry{
						EntryType: "observation",
						Code: cdadocument.CDACode{
							Code: "PATOBJ", DisplayName: "Patient objection", CodeSystem: "2.16.840.1.113883.5.8",
						},
					},
				},
			},
		},
	}
	resources := MapImmunizations(entries, testPatientRef)
	if len(resources) != 1 {
		t.Fatalf("expected 1 Immunization, got %d", len(resources))
	}
	if resources[0]["status"] != "not-done" {
		t.Errorf("status = %v, want not-done", resources[0]["status"])
	}
	reason, ok := resources[0]["statusReason"].(map[string]interface{})
	if !ok {
		t.Fatal("expected statusReason to be set from the RSON refusal-reason relationship")
	}
	coding, _ := reason["coding"].([]interface{})
	if len(coding) != 1 || coding[0].(map[string]interface{})["code"] != "PATOBJ" {
		t.Errorf("statusReason coding = %v, want PATOBJ (Patient objection)", coding)
	}
}

func TestMapImmunizations_NotNegated_RSONIgnored(t *testing.T) {
	// An RSON relationship without NegationInd is not a refusal reason in this
	// template family -- statusReason should only ever be populated alongside
	// status="not-done".
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "substanceAdministration",
			StatusCode: "completed",
			Consumable: &cdadocument.CDAConsumable{
				ManufacturedProduct: cdadocument.CDAManufacturedProduct{
					ManufacturedMaterial: &cdadocument.CDAMaterial{Code: cdadocument.CDACode{Code: "33", DisplayName: "Pneumococcal (PCV, PPSV)", CodeSystem: "2.16.840.1.113883.12.292"}},
				},
			},
			EntryRelationships: []cdadocument.CDAEntryRelationship{
				{
					TypeCode: "RSON",
					Entry: cdadocument.CDAEntry{
						EntryType: "observation",
						Code:      cdadocument.CDACode{Code: "PATOBJ", DisplayName: "Patient objection", CodeSystem: "2.16.840.1.113883.5.8"},
					},
				},
			},
		},
	}
	resources := MapImmunizations(entries, testPatientRef)
	if len(resources) != 1 {
		t.Fatalf("expected 1 Immunization, got %d", len(resources))
	}
	if _, has := resources[0]["statusReason"]; has {
		t.Error("statusReason must not be set when the immunization was not negated")
	}
}

// ---- Immunization performer: <performer>, not <participant> ----

func TestMapImmunizations_Performer_ReadFromPerformersNotParticipants(t *testing.T) {
	// Regression: this loop used to read entry.Participants[typeCode=PRF],
	// which a real C-CDA Immunization Activity's <performer typeCode="PRF">
	// element never populates (entry_parser.go's parsePerformers parses
	// <performer> into entry.Performers, a completely different field from
	// <participant>/entry.Participants) -- so performer could never be set
	// from real data. Fixed to read entry.Performers, the same field
	// medication_mapper.go's requesterReference already reads correctly.
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "substanceAdministration",
			StatusCode: "completed",
			Consumable: &cdadocument.CDAConsumable{
				ManufacturedProduct: cdadocument.CDAManufacturedProduct{
					ManufacturedMaterial: &cdadocument.CDAMaterial{Code: cdadocument.CDACode{Code: "33", CodeSystem: "2.16.840.1.113883.12.292"}},
				},
			},
			Performers: []cdadocument.CDAPerformer{
				{
					TypeCode: "PRF",
					AssignedEntity: cdadocument.CDAAssignedEntity{
						AssignedPerson: &cdadocument.CDAPerson{Names: []cdadocument.CDAName{{Given: []string{"Jane"}, Family: "Doe"}}},
					},
				},
			},
			// A Participant typed PRF (the OLD, wrong field) must NOT be the source.
			Participants: []cdadocument.CDAParticipant{
				{TypeCode: "PRF", ParticipantRole: cdadocument.CDAParticipantRole{PlayingEntity: &cdadocument.CDAPlayingEntity{Names: []cdadocument.CDAName{{Given: []string{"Wrong"}, Family: "Field"}}}}},
			},
		},
	}
	resources := MapImmunizations(entries, testPatientRef)
	if len(resources) != 1 {
		t.Fatalf("expected 1 Immunization, got %d", len(resources))
	}
	performers, ok := resources[0]["performer"].([]interface{})
	if !ok || len(performers) != 1 {
		t.Fatalf("performer = %v, want a 1-element array", resources[0]["performer"])
	}
	actor, _ := performers[0].(map[string]interface{})["actor"].(map[string]interface{})
	if actor["display"] != "Jane Doe" {
		t.Errorf("performer[0].actor.display = %v, want \"Jane Doe\" (from Performers, not the wrong Participants field)", actor["display"])
	}
}

// ---- Procedure bodySite: read from <targetSiteCode> directly, not a COMP entryRelationship ----

func TestMapProcedures_TargetSiteCode_SetsBodySite(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:      "procedure",
			StatusCode:     "completed",
			Code:           cdadocument.CDACode{Code: "44950", DisplayName: "Appendectomy", CodeSystem: "2.16.840.1.113883.6.12"},
			TargetSiteCode: &cdadocument.CDACode{Code: "66754008", DisplayName: "Appendix structure", CodeSystem: "2.16.840.1.113883.6.96"},
		},
	}
	resources := MapProcedures(entries, testPatientRef)
	if len(resources) != 1 {
		t.Fatalf("expected 1 Procedure, got %d", len(resources))
	}
	bodySite, ok := resources[0]["bodySite"].([]interface{})
	if !ok || len(bodySite) != 1 {
		t.Fatalf("expected Procedure.bodySite to be set from TargetSiteCode, got %v", resources[0]["bodySite"])
	}
	cc, _ := bodySite[0].(map[string]interface{})
	coding, _ := cc["coding"].([]interface{})
	if len(coding) != 1 || coding[0].(map[string]interface{})["code"] != "66754008" {
		t.Errorf("bodySite coding = %v, want SNOMED 66754008 (Appendix structure)", coding)
	}
}

func TestMapProcedures_NoTargetSiteCode_NoBodySiteEvenWithUnrelatedCOMP(t *testing.T) {
	// Regression guard for the original bug: bodySite must come from
	// TargetSiteCode only, never be inferred from an unrelated COMP
	// entryRelationship (e.g. an indication or reason sub-act).
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "procedure",
			StatusCode: "completed",
			Code:       cdadocument.CDACode{Code: "44950", DisplayName: "Appendectomy", CodeSystem: "2.16.840.1.113883.6.12"},
			EntryRelationships: []cdadocument.CDAEntryRelationship{
				{
					TypeCode: "COMP",
					Entry: cdadocument.CDAEntry{
						EntryType: "observation",
						Code:      cdadocument.CDACode{Code: "385536008", DisplayName: "Acute appendicitis", CodeSystem: "2.16.840.1.113883.6.96"},
					},
				},
			},
		},
	}
	resources := MapProcedures(entries, testPatientRef)
	if len(resources) != 1 {
		t.Fatalf("expected 1 Procedure, got %d", len(resources))
	}
	if _, has := resources[0]["bodySite"]; has {
		t.Error("bodySite must not be inferred from an unrelated COMP entryRelationship")
	}
}

func TestMapProcedures_ObservationVariant_NoTargetSiteCode_NoBodySite(t *testing.T) {
	// Procedure Activity Observation (.4.13) uses an <observation> element,
	// which has no targetSiteCode in the base CDA R2 schema.
	entries := []cdadocument.CDAEntry{
		{EntryType: "observation", StatusCode: "completed", Code: cdadocument.CDACode{Code: "44950"}},
	}
	resources := MapProcedures(entries, testPatientRef)
	if len(resources) != 1 {
		t.Fatalf("expected 1 Procedure, got %d", len(resources))
	}
	if _, has := resources[0]["bodySite"]; has {
		t.Error("bodySite must not be set when TargetSiteCode is nil")
	}
}
