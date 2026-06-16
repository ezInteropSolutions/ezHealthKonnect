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
	// observation has no CSM substance participant, only its own assertion value.
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
