// services/cda_fhir/declarative_oob_rules_test.go
//
// Phase 3 acceptance suite: every assertion ported here mirrors one test in
// services/cda_fhir/mappers/mappers_test.go (plus one from
// cda_fhir_integration_test.go) for the Allergies/Medications/Conditions
// slice this session scoped Phase 3 to — same inputs (the same
// cdadocument.CDAEntry literals, JSON-round-tripped to the documentMap
// shape DeclarativeEngine consumes, exactly like
// declarative_engine_test.go's loadDocumentMapFixture does for real XML
// fixtures), same expected outputs, run against AllergyMappingRules/
// MedicationMappingRules/ProblemsMappingRules/HealthConcernsMappingRules
// instead of the hardcoded Go mappers. 1:1 parity, no partial credit, per
// the sprint plan's Phase 3 exit criteria.
package cdafhir_test

import (
	"encoding/json"
	"testing"

	cdadocument "ezhealthkonnect/cda/document"
	cdafhir "ezhealthkonnect/services/cda_fhir"
	"ezhealthkonnect/services/cda_fhir/assembly"
	"ezhealthkonnect/services/cda_fhir/assembly/rules"
)

// documentMapForEntries builds the {"sectionsByKey": {sectionKey: {"entries":
// [...]}}} shape DeclarativeEngine consumes from real cdadocument.CDAEntry
// struct literals, JSON-round-tripped exactly as cda_parser_service.go does
// for a real parsed document — avoids hand-transcribing nested maps (a
// transcription-error risk) for every ported test.
func documentMapForEntries(t testing.TB, sectionKey string, entries []cdadocument.CDAEntry) map[string]interface{} {
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

func firstCoding(t testing.TB, cc interface{}) map[string]interface{} {
	t.Helper()
	m, ok := cc.(map[string]interface{})
	if !ok {
		t.Fatalf("not a CodeableConcept-shaped map: %v", cc)
	}
	codings, ok := m["coding"].([]interface{})
	if !ok || len(codings) == 0 {
		t.Fatalf("no coding[] in %v", m)
	}
	coding, ok := codings[0].(map[string]interface{})
	if !ok {
		t.Fatalf("coding[0] not a map: %v", codings[0])
	}
	return coding
}

// ---- Allergies ----
// Ports mappers_test.go's TestMapAllergies_NoKnownAllergies_CodeFallsBackToAssertionValue
// and TestMapAllergies_NotNegated_VerificationStatusConfirmed.

func TestDeclarativeEngine_Allergy_NoKnownAllergies_CodeFallsBackToAssertionValue(t *testing.T) {
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
	documentMap := documentMapForEntries(t, "allergiesAndIntolerances", entries)
	engine := cdafhir.NewDeclarativeEngine()

	resources, errs := engine.BuildResources(documentMap, cdafhir.AllergyMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 AllergyIntolerance, got %d", len(resources))
	}
	if _, hasCode := resources[0]["code"]; !hasCode {
		t.Error("AllergyIntolerance.code must be set even with no CSM substance participant")
	}
	coding := firstCoding(t, resources[0]["verificationStatus"])
	if coding["code"] != "refuted" {
		t.Errorf("verificationStatus.code = %v, want \"refuted\" for a negated assertion", coding["code"])
	}
}

func TestDeclarativeEngine_Allergy_NotNegated_VerificationStatusConfirmed(t *testing.T) {
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
	documentMap := documentMapForEntries(t, "allergiesAndIntolerances", entries)
	engine := cdafhir.NewDeclarativeEngine()

	resources, errs := engine.BuildResources(documentMap, cdafhir.AllergyMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 AllergyIntolerance, got %d", len(resources))
	}
	coding := firstCoding(t, resources[0]["verificationStatus"])
	if coding["code"] != "confirmed" {
		t.Errorf("verificationStatus.code = %v, want \"confirmed\" for a non-negated allergy", coding["code"])
	}
}

// TestDeclarativeEngine_Allergy_ReactionsWithAndWithoutSeverity proves
// AllergyMappingRules()'s reaction row (CollectAll+Fields) against the
// production rule itself, not just the generic primitive
// declarative_engine_test.go already covers: two reactions, only the
// second carrying a Severity Observation -- mirrors
// allergy_mapper.go:99-123's own structure (Reaction Observation V2,
// templateId .4.9, MFST; Severity Observation V2, templateId .4.8, nested
// SUBJ) directly, including the inversionInd=true the declarative rows
// check more strictly than the legacy Go mapper does.
func TestDeclarativeEngine_Allergy_ReactionsWithAndWithoutSeverity(t *testing.T) {
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
						EntryRelationships: []cdadocument.CDAEntryRelationship{
							{
								TypeCode:     "MFST",
								InversionInd: true,
								Entry: cdadocument.CDAEntry{
									EntryType:   "observation",
									TemplateIds: []string{"2.16.840.1.113883.10.20.22.4.9"},
									Value: &cdadocument.CDAValue{
										Type: "CD",
										Code: &cdadocument.CDACode{Code: "247472004", DisplayName: "Hives", CodeSystem: "2.16.840.1.113883.6.96"},
									},
								},
							},
							{
								TypeCode:     "MFST",
								InversionInd: true,
								Entry: cdadocument.CDAEntry{
									EntryType:   "observation",
									TemplateIds: []string{"2.16.840.1.113883.10.20.22.4.9"},
									Value: &cdadocument.CDAValue{
										Type: "CD",
										Code: &cdadocument.CDACode{Code: "422587007", DisplayName: "Nausea", CodeSystem: "2.16.840.1.113883.6.96"},
									},
									EntryRelationships: []cdadocument.CDAEntryRelationship{
										{
											TypeCode:     "SUBJ",
											InversionInd: true,
											Entry: cdadocument.CDAEntry{
												EntryType:   "observation",
												TemplateIds: []string{"2.16.840.1.113883.10.20.22.4.8"},
												Code:        cdadocument.CDACode{Code: "SEV"},
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
				},
			},
		},
	}
	documentMap := documentMapForEntries(t, "allergiesAndIntolerances", entries)
	engine := cdafhir.NewDeclarativeEngine()

	resources, errs := engine.BuildResources(documentMap, cdafhir.AllergyMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 AllergyIntolerance, got %d", len(resources))
	}

	reactions, ok := resources[0]["reaction"].([]interface{})
	if !ok || len(reactions) != 2 {
		t.Fatalf("reaction = %v, want a 2-element array", resources[0]["reaction"])
	}

	r0 := reactions[0].(map[string]interface{})
	if _, hasSeverity := r0["severity"]; hasSeverity {
		t.Errorf("reaction[0] (Hives, no Severity Observation) should have no severity key, got %v", r0["severity"])
	}
	m0Coding := firstCoding(t, r0["manifestation"].([]interface{})[0])
	if m0Coding["code"] != "247472004" {
		t.Errorf("reaction[0] manifestation coding = %v, want SNOMED 247472004 (Hives)", m0Coding)
	}

	r1 := reactions[1].(map[string]interface{})
	if r1["severity"] != "severe" {
		t.Errorf("reaction[1] (Nausea, Severity=Severe) severity = %v, want \"severe\" -- if this is missing or "+
			"wrong, the CollectAll+Fields index alignment regressed", r1["severity"])
	}
	m1Coding := firstCoding(t, r1["manifestation"].([]interface{})[0])
	if m1Coding["code"] != "422587007" {
		t.Errorf("reaction[1] manifestation coding = %v, want SNOMED 422587007 (Nausea)", m1Coding)
	}
}

// TestDeclarativeEngine_Allergy_DocumentLevelCount ports
// cda_fhir_integration_test.go's TestMapDocument_AllergyCount_MatchesEntries
// at the rule level: N section entries, each with at least statusCode set
// (the same minimal shape newMinimalDoc builds), must produce N resources.
func TestDeclarativeEngine_Allergy_DocumentLevelCount(t *testing.T) {
	const numAllergies = 4
	entries := make([]cdadocument.CDAEntry, numAllergies)
	for i := range entries {
		entries[i] = cdadocument.CDAEntry{
			EntryType:  "act",
			StatusCode: "active",
			Code:       cdadocument.CDACode{Code: "48765001", CodeSystem: "2.16.840.1.113883.6.96", DisplayName: "Allergy"},
		}
	}
	documentMap := documentMapForEntries(t, "allergiesAndIntolerances", entries)
	engine := cdafhir.NewDeclarativeEngine()

	resources, errs := engine.BuildResources(documentMap, cdafhir.AllergyMappingRules()[0])
	// This fixture's entries carry only a top-level Code (no CSM substance
	// participant, no observation Value at all) -- a synthetic shape neither
	// the original allergy_mapper.go nor this rule's "code" row can find a
	// substance code from (Go's fallback chain only ever checks
	// allergyObs.Value.Code, never the outer entry's own bare Code field; see
	// allergy_mapper.go:93-97). Go's mapper still silently includes the
	// resource (it never gates on "code", only on len(resource)>2);
	// AllergyIntolerance.code is genuinely 1..1 SHALL per US Core, so the
	// declarative row's Required/SHALL flag surfaces that gap as a real
	// error per entry where Go's hardcoded version swallowed it -- a
	// deliberate behavior improvement, not a regression, and exactly why
	// BuildResources still returns the resource alongside the errors rather
	// than dropping it (see buildOneResource's own doc comment).
	if len(errs) != numAllergies {
		t.Fatalf("got %d errors, want %d (one missing-required-code error per entry, matching this minimal "+
			"fixture's lack of CSM/value data): %+v", len(errs), numAllergies, errs)
	}
	for _, e := range errs {
		if e.FieldKey != "code" || e.Severity != "error" {
			t.Errorf("unexpected error shape: %+v", e)
		}
	}
	if len(resources) != numAllergies {
		t.Errorf("AllergyIntolerance count = %d, want %d", len(resources), numAllergies)
	}
}

// ---- Medications ----
// Ports TestMapMedications_OrderIntent_RequesterFromPerformer,
// TestMapMedications_OrderIntent_RequesterFallback_NeverEmpty,
// TestMapMedications_PIVLFrequency_SetsDosageTimingRepeat,
// TestMapMedications_RSONIndication_SetsReasonCode, and
// TestMapMedications_FreeTextSigAndInstructionV2_SetDosageTextFields.

func buildMedicationResources(t testing.TB, entries []cdadocument.CDAEntry) []map[string]interface{} {
	t.Helper()
	documentMap := documentMapForEntries(t, "medications", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResourcesForRules(documentMap, cdafhir.MedicationMappingRules())
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	return resources
}

func TestDeclarativeEngine_Medication_OrderIntent_RequesterFromPerformer(t *testing.T) {
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
	resources := buildMedicationResources(t, entries)
	if len(resources) != 1 || resources[0]["resourceType"] != "MedicationRequest" {
		t.Fatalf("expected 1 MedicationRequest, got %v", resources)
	}
	requester, ok := resources[0]["requester"].(map[string]interface{})
	if !ok {
		t.Fatal("MedicationRequest.requester not set")
	}
	if requester["display"] != "Halie Lower" {
		t.Errorf("requester.display = %v, want %q", requester["display"], "Halie Lower")
	}
}

func TestDeclarativeEngine_Medication_OrderIntent_RequesterFallback_NeverEmpty(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{EntryType: "substanceAdministration", MoodCode: "INT", StatusCode: "active"},
	}
	resources := buildMedicationResources(t, entries)
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

func TestDeclarativeEngine_Medication_PIVLFrequency_SetsDosageTimingRepeat(t *testing.T) {
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
	resources := buildMedicationResources(t, entries)
	if len(resources) != 1 || resources[0]["resourceType"] != "MedicationStatement" {
		t.Fatalf("expected 1 MedicationStatement, got %v", resources)
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

func TestDeclarativeEngine_Medication_RSONIndication_SetsReasonCode(t *testing.T) {
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
	resources := buildMedicationResources(t, entries)
	if len(resources) != 1 {
		t.Fatalf("expected 1 MedicationStatement, got %d", len(resources))
	}
	reasons, ok := resources[0]["reasonCode"].([]interface{})
	if !ok || len(reasons) != 1 {
		t.Fatalf("expected 1 reasonCode from the RSON indication, got %v", resources[0]["reasonCode"])
	}
	coding := firstCoding(t, reasons[0])
	if coding["code"] != "38341003" {
		t.Errorf("reasonCode coding = %v, want SNOMED 38341003 (Hypertension)", coding)
	}
}

func TestDeclarativeEngine_Medication_FreeTextSigAndInstructionV2_SetDosageTextFields(t *testing.T) {
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
	resources := buildMedicationResources(t, entries)
	if len(resources) != 1 || resources[0]["resourceType"] != "MedicationRequest" {
		t.Fatalf("expected 1 MedicationRequest, got %v", resources)
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

// ---- Conditions ----
// Ports TestMapConditions_NegatedProblem_VerificationStatusRefuted,
// TestMapConditions_SeverityFromNestedSUBJObservation,
// TestMapConditions_ProblemListItem_UsesBaseCodeSystem, and
// TestMapConditions_HealthConcern_UsesUSCoreCodeSystemAndCode.

func TestDeclarativeEngine_Condition_NegatedProblem_VerificationStatusRefuted(t *testing.T) {
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
	documentMap := documentMapForEntries(t, "problems", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.ProblemsMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 Condition, got %d", len(resources))
	}
	coding := firstCoding(t, resources[0]["verificationStatus"])
	if coding["code"] != "refuted" {
		t.Errorf("verificationStatus.code = %v, want refuted for a negated problem", coding["code"])
	}
}

func TestDeclarativeEngine_Condition_SeverityFromNestedSUBJObservation(t *testing.T) {
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
	documentMap := documentMapForEntries(t, "problems", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.ProblemsMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 Condition, got %d", len(resources))
	}
	severity, ok := resources[0]["severity"]
	if !ok {
		t.Fatal("expected Condition.severity to be set from the nested SEV observation")
	}
	coding := firstCoding(t, severity)
	if coding["code"] != "24484000" {
		t.Errorf("severity coding = %v, want SNOMED 24484000 (Severe)", coding)
	}
}

func TestDeclarativeEngine_Condition_ProblemListItem_UsesBaseCodeSystem(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{EntryType: "act", StatusCode: "active", Value: &cdadocument.CDAValue{Type: "CD", Code: &cdadocument.CDACode{Code: "38341003"}}},
	}
	documentMap := documentMapForEntries(t, "problems", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.ProblemsMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 Condition, got %d", len(resources))
	}
	cats, _ := resources[0]["category"].([]interface{})
	if len(cats) != 1 {
		t.Fatalf("category = %v, want a 1-element array", resources[0]["category"])
	}
	coding := firstCoding(t, cats[0])
	const baseConditionCategorySystem = "http://terminology.hl7.org/CodeSystem/condition-category"
	if coding["system"] != baseConditionCategorySystem || coding["code"] != "problem-list-item" {
		t.Errorf("category coding = %v, want system=%q code=problem-list-item", coding, baseConditionCategorySystem)
	}
	codeCoding := firstCoding(t, resources[0]["code"])
	if codeCoding["code"] != "38341003" {
		t.Errorf("code coding = %v, want 38341003", codeCoding)
	}
}

func TestDeclarativeEngine_Condition_HealthConcern_UsesUSCoreCodeSystemAndCode(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{EntryType: "act", StatusCode: "active", Value: &cdadocument.CDAValue{Type: "CD", Code: &cdadocument.CDACode{Code: "44261-6"}}},
	}
	documentMap := documentMapForEntries(t, "healthConcerns", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.HealthConcernsMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 Condition, got %d", len(resources))
	}
	cats, _ := resources[0]["category"].([]interface{})
	if len(cats) != 1 {
		t.Fatalf("category = %v, want a 1-element array", resources[0]["category"])
	}
	coding := firstCoding(t, cats[0])
	const usCoreConditionCategorySystem = "http://hl7.org/fhir/us/core/CodeSystem/condition-category"
	if coding["system"] != usCoreConditionCategorySystem || coding["code"] != "health-concern" {
		t.Errorf("category coding = %v, want system=%q code=health-concern", coding, usCoreConditionCategorySystem)
	}
}

// ---- Vital Signs / Results / Social History ----
//
// All three share observationRule() (one Go function, different category
// LiteralValue) -- see declarative_oob_rules.go's own doc comment for what
// this slice deliberately does NOT port (BP-panel combination,
// shell-substitution, interpretationCode) and why.

func TestDeclarativeEngine_VitalSigns_NonBPValue_SetCorrectly(t *testing.T) {
	// Ports mappers_test.go's TestMapObservations_NonBPVitalSign_NotCombined:
	// a single PQ vital sign, no BP pairing involved.
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "observation",
			StatusCode: "completed",
			Code:       cdadocument.CDACode{Code: "8302-2", CodeSystem: "2.16.840.1.113883.6.1", DisplayName: "Height"},
			Value:      &cdadocument.CDAValue{Type: "PQ", Quantity: &cdadocument.CDAQuantity{Value: "162.6", Unit: "cm"}},
		},
	}
	documentMap := documentMapForEntries(t, "vitalSigns", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.VitalSignsMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 Observation, got %d", len(resources))
	}
	r := resources[0]
	if _, hasComponent := r["component"]; hasComponent {
		t.Error("a single non-BP vital sign should not have a component array")
	}
	qty, ok := r["valueQuantity"].(map[string]interface{})
	if !ok || qty["value"] != 162.6 {
		t.Errorf("valueQuantity = %v, want value=162.6 (float64 -- CDAQuantityToFHIR parses the numeric string)", r["valueQuantity"])
	}
	cats, _ := r["category"].([]interface{})
	if len(cats) != 1 {
		t.Fatalf("category = %v, want a 1-element array", r["category"])
	}
	coding := firstCoding(t, cats[0])
	if coding["code"] != "vital-signs" {
		t.Errorf("category coding = %v, want code=vital-signs", coding)
	}
}

func TestDeclarativeEngine_VitalSigns_BPPair_RecombinedByAssemblyLayer(t *testing.T) {
	// Proves the deliberate division of responsibility declarative_oob_rules.go's
	// doc comment describes: this rule's own output keeps Systolic/Diastolic
	// as two SEPARATE Observations (FlattenOrganizers has no BP-pairing
	// logic), but the pre-existing, engine-agnostic
	// assembly/rules.BPPanelSynthesisRule recombines them afterward into the
	// SAME shape Go's mapper-level extractBloodPressurePanels produces
	// directly -- not a silently-dropped gap.
	organizer := cdadocument.CDAEntry{
		EntryType: "organizer",
		Code:      cdadocument.CDACode{Code: "46680005", CodeSystem: "2.16.840.1.113883.6.96", DisplayName: "Vital signs"},
		Components: []cdadocument.CDAEntry{
			{
				EntryType:  "observation",
				StatusCode: "completed",
				Code:       cdadocument.CDACode{Code: "8480-6", CodeSystem: "2.16.840.1.113883.6.1"}, // LOINC Systolic BP
				Value:      &cdadocument.CDAValue{Type: "PQ", Quantity: &cdadocument.CDAQuantity{Value: "124", Unit: "mm[Hg]"}},
			},
			{
				EntryType:  "observation",
				StatusCode: "completed",
				Code:       cdadocument.CDACode{Code: "8462-4", CodeSystem: "2.16.840.1.113883.6.1"}, // LOINC Diastolic BP
				Value:      &cdadocument.CDAValue{Type: "PQ", Quantity: &cdadocument.CDAQuantity{Value: "72", Unit: "mm[Hg]"}},
			},
		},
	}
	documentMap := documentMapForEntries(t, "vitalSigns", []cdadocument.CDAEntry{organizer})
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.VitalSignsMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 2 {
		t.Fatalf("got %d resources, want 2 standalone Observations (FlattenOrganizers, no BP-pairing) before assembly runs", len(resources))
	}

	ctx := &assembly.AssemblyContext{Resources: resources}
	ruleEngine := assembly.NewDefaultRuleEngine()
	ruleEngine.Register(rules.NewBPPanelSynthesisRule())
	if err := ruleEngine.Run(ctx); err != nil {
		t.Fatalf("assembly run: %v", err)
	}

	if len(ctx.Resources) != 3 {
		t.Fatalf("got %d resources after assembly, want 3 (2 originals, still present, + 1 synthesized panel)", len(ctx.Resources))
	}

	var panel map[string]interface{}
	for _, r := range ctx.Resources {
		code, _ := r["code"].(map[string]interface{})
		codings, _ := code["coding"].([]interface{})
		if len(codings) == 0 {
			continue
		}
		if c, _ := codings[0].(map[string]interface{}); c["code"] == "85354-9" {
			panel = r
		}
	}
	if panel == nil {
		t.Fatalf("no synthesized BP panel (code 85354-9) found in ctx.Resources: %+v", ctx.Resources)
	}
	components, _ := panel["component"].([]interface{})
	if len(components) != 2 {
		t.Fatalf("panel.component length = %d, want 2", len(components))
	}

	// BPPanelSynthesisRule keys ctx.Removed by "Observation/<id>". The
	// declarative engine doesn't assign resource "id"s yet (true for every
	// section ported so far, not specific to Vital Signs) -- both
	// candidates resolve to id="", so they collide onto the SAME
	// "Observation/" key here. That's a known, pre-existing gap (Phase 4's
	// job, applied uniformly across all sections, not a one-off patch for
	// this rule), not something this test should paper over: assert the one
	// key the current engine state actually produces, not the two a
	// real-id'd pipeline would.
	if !ctx.Removed["Observation/"] {
		t.Errorf("ctx.Removed = %v, want the standalone Systolic/Diastolic resources marked for exclusion", ctx.Removed)
	}
}

func TestDeclarativeEngine_Results_NullFlavorValue_ProducesDataAbsentReason(t *testing.T) {
	// Mirrors the PracticeFusion real-world case the inventory cites:
	// nullFlavor=NI value -> dataAbsentReason, satisfying us-core-2.
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "observation",
			StatusCode: "completed",
			Code:       cdadocument.CDACode{Code: "26436-6", CodeSystem: "2.16.840.1.113883.6.1"},
			Value:      &cdadocument.CDAValue{NullFlavor: "NI"},
		},
	}
	documentMap := documentMapForEntries(t, "results", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.ResultsMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 Observation, got %d", len(resources))
	}
	if _, has := resources[0]["valueQuantity"]; has {
		t.Error("a nullFlavor value should never produce valueQuantity")
	}
	dar, ok := resources[0]["dataAbsentReason"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected dataAbsentReason to satisfy us-core-2, got %v", resources[0]["dataAbsentReason"])
	}
	coding := firstCoding(t, dar)
	if coding["code"] != "unknown" {
		t.Errorf("dataAbsentReason coding = %v, want code=unknown", coding)
	}
}

func TestDeclarativeEngine_SocialHistory_SmokingStatus_ValueCodeableConcept(t *testing.T) {
	// Mirrors Kareo's real smoking-status entry: SNOMED 266927001 "Unknown
	// if ever smoked" via a CD value.
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "observation",
			StatusCode: "completed",
			Code:       cdadocument.CDACode{Code: "72166-2", CodeSystem: "2.16.840.1.113883.6.1", DisplayName: "Tobacco smoking status"},
			Value: &cdadocument.CDAValue{
				Type: "CD",
				Code: &cdadocument.CDACode{Code: "266927001", DisplayName: "Unknown if ever smoked", CodeSystem: "2.16.840.1.113883.6.96"},
			},
		},
	}
	documentMap := documentMapForEntries(t, "socialHistory", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.SocialHistoryMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 Observation, got %d", len(resources))
	}
	cc, ok := resources[0]["valueCodeableConcept"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected valueCodeableConcept, got %v", resources[0]["valueCodeableConcept"])
	}
	coding := firstCoding(t, cc)
	if coding["code"] != "266927001" {
		t.Errorf("valueCodeableConcept coding = %v, want code=266927001", coding)
	}
	cats, _ := resources[0]["category"].([]interface{})
	catCoding := firstCoding(t, cats[0])
	if catCoding["code"] != "social-history" {
		t.Errorf("category coding = %v, want code=social-history", catCoding)
	}
}

// ---- Immunizations ----
//
// Ports all 4 status/statusReason assertions from mappers_test.go (the
// negationInd-takes-priority behavior, already fixed before this session,
// per ImmunizationMappingRules' own top doc comment) plus the new
// performer-field regression this session's investigation found and fixed
// in Go.

func TestDeclarativeEngine_Immunization_NotNegated_StatusCompleted(t *testing.T) {
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
	documentMap := documentMapForEntries(t, "immunizations", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.ImmunizationMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
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

func TestDeclarativeEngine_Immunization_NegationInd_StatusNotDone(t *testing.T) {
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
	documentMap := documentMapForEntries(t, "immunizations", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.ImmunizationMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 Immunization, got %d", len(resources))
	}
	if resources[0]["status"] != "not-done" {
		t.Errorf("status = %v, want not-done for a negated (refused) immunization -- if this regressed to "+
			"\"completed\", the Condition's ThenTransform=string_direct override broke", resources[0]["status"])
	}
}

func TestDeclarativeEngine_Immunization_NegationIndWithRefusalReason_SetsStatusReason(t *testing.T) {
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
						Code:      cdadocument.CDACode{Code: "PATOBJ", DisplayName: "Patient objection", CodeSystem: "2.16.840.1.113883.5.8"},
					},
				},
			},
		},
	}
	documentMap := documentMapForEntries(t, "immunizations", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.ImmunizationMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
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
	coding := firstCoding(t, reason)
	if coding["code"] != "PATOBJ" {
		t.Errorf("statusReason coding = %v, want PATOBJ (Patient objection)", coding)
	}
}

func TestDeclarativeEngine_Immunization_Performer_ReadFromPerformersField(t *testing.T) {
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
		},
	}
	documentMap := documentMapForEntries(t, "immunizations", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.ImmunizationMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 Immunization, got %d", len(resources))
	}
	performers, ok := resources[0]["performer"].([]interface{})
	if !ok || len(performers) != 1 {
		t.Fatalf("performer = %v, want a 1-element array", resources[0]["performer"])
	}
	actor, _ := performers[0].(map[string]interface{})["actor"].(map[string]interface{})
	if actor["display"] != "Jane Doe" {
		t.Errorf("performer[0].actor.display = %v, want \"Jane Doe\"", actor["display"])
	}
}

func TestDeclarativeEngine_Immunization_NotNegated_RSONNotGated_KnownDivergenceFromGo(t *testing.T) {
	// Documents (does not silently hide) ImmunizationMappingRules' own
	// top-doc-comment divergence: Go's mappers_test.go has
	// TestMapImmunizations_NotNegated_RSONIgnored proving Go suppresses
	// statusReason when NOT negated, even if an RSON relationship exists.
	// The declarative statusReason row has no primitive to gate on
	// negationInd (a field outside its own Scope root) and isn't given one
	// for a zero-corpus-evidence, non-conformant-data edge case -- so THIS
	// engine, today, DOES populate statusReason here. If this test ever
	// starts failing because someone "fixed" the row to suppress it, update
	// ImmunizationMappingRules' doc comment too (the divergence would no
	// longer exist, which is fine, just keep the documentation honest).
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "substanceAdministration",
			StatusCode: "completed",
			Consumable: &cdadocument.CDAConsumable{
				ManufacturedProduct: cdadocument.CDAManufacturedProduct{
					ManufacturedMaterial: &cdadocument.CDAMaterial{Code: cdadocument.CDACode{Code: "33", CodeSystem: "2.16.840.1.113883.12.292"}},
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
	documentMap := documentMapForEntries(t, "immunizations", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.ImmunizationMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if resources[0]["status"] != "completed" {
		t.Errorf("status = %v, want completed (NOT negated)", resources[0]["status"])
	}
	if _, has := resources[0]["statusReason"]; !has {
		t.Error("this test documents that statusReason IS currently set even though NOT negated -- " +
			"if this now fails, the divergence from Go was closed; update the doc comment, don't just delete this test")
	}
}

// ---- Encounters ----
//
// Ports both existing mappers_test.go assertions (the class.display
// empty-string guard) plus new tests for participant/location -- zero
// dedicated tests existed for those in Go (per the inventory), so these are
// new coverage, not ports.

func TestDeclarativeEngine_Encounter_EmptyDisplayName_OmittedNotEmpty(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{EntryType: "encounter", StatusCode: "completed", Code: cdadocument.CDACode{Code: "AMB", DisplayName: ""}},
	}
	documentMap := documentMapForEntries(t, "encounters", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.EncounterMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
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

func TestDeclarativeEngine_Encounter_NonEmptyDisplayName_Kept(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{EntryType: "encounter", StatusCode: "completed", Code: cdadocument.CDACode{Code: "AMB", DisplayName: "Ambulatory"}},
	}
	documentMap := documentMapForEntries(t, "encounters", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.EncounterMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	class := resources[0]["class"].(map[string]interface{})
	if class["display"] != "Ambulatory" {
		t.Errorf("class.display = %v, want %q", class["display"], "Ambulatory")
	}
	typeArr, ok := resources[0]["type"].([]interface{})
	if !ok || len(typeArr) != 1 {
		t.Fatalf("type = %v, want a 1-element array (same code as class, dual-mapped)", resources[0]["type"])
	}
	coding := firstCoding(t, typeArr[0])
	if coding["code"] != "AMB" {
		t.Errorf("type[0] coding = %v, want code=AMB", coding)
	}
}

func TestDeclarativeEngine_Encounter_Participant_TypeAndIndividualSet(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "encounter",
			StatusCode: "completed",
			Code:       cdadocument.CDACode{Code: "AMB"},
			Participants: []cdadocument.CDAParticipant{
				{
					TypeCode: "ATND",
					ParticipantRole: cdadocument.CDAParticipantRole{
						PlayingEntity: &cdadocument.CDAPlayingEntity{Names: []cdadocument.CDAName{{Given: []string{"Jane"}, Family: "Doe"}}},
					},
				},
			},
		},
	}
	documentMap := documentMapForEntries(t, "encounters", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.EncounterMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	participants, ok := resources[0]["participant"].([]interface{})
	if !ok || len(participants) != 1 {
		t.Fatalf("participant = %v, want a 1-element array", resources[0]["participant"])
	}
	part, _ := participants[0].(map[string]interface{})
	typeArr, _ := part["type"].([]interface{})
	if len(typeArr) != 1 {
		t.Fatalf("participant[0].type = %v, want a 1-element array", part["type"])
	}
	coding := firstCoding(t, typeArr[0])
	if coding["code"] != "ATND" {
		t.Errorf("participant[0].type coding = %v, want code=ATND", coding)
	}
	individual, _ := part["individual"].(map[string]interface{})
	if individual["display"] != "Jane Doe" {
		t.Errorf("participant[0].individual.display = %v, want \"Jane Doe\"", individual["display"])
	}
}

func TestDeclarativeEngine_Encounter_Location_FromCOMPParticipantLOC(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "encounter",
			StatusCode: "completed",
			Code:       cdadocument.CDACode{Code: "AMB"},
			EntryRelationships: []cdadocument.CDAEntryRelationship{
				{
					TypeCode: "COMP",
					Entry: cdadocument.CDAEntry{
						EntryType: "encounter",
						Participants: []cdadocument.CDAParticipant{
							{
								TypeCode: "LOC",
								ParticipantRole: cdadocument.CDAParticipantRole{
									PlayingEntity: &cdadocument.CDAPlayingEntity{Names: []cdadocument.CDAName{{Family: "Main Street Clinic"}}},
								},
							},
						},
					},
				},
			},
		},
	}
	documentMap := documentMapForEntries(t, "encounters", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.EncounterMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	locArr, ok := resources[0]["location"].([]interface{})
	if !ok || len(locArr) != 1 {
		t.Fatalf("location = %v, want a 1-element array", resources[0]["location"])
	}
	loc, _ := locArr[0].(map[string]interface{})
	display, _ := loc["location"].(map[string]interface{})
	if display["display"] != "Main Street Clinic" {
		t.Errorf("location[0].location.display = %v, want \"Main Street Clinic\"", display["display"])
	}
}

// ---- Procedures ----
//
// Ports all 3 existing mappers_test.go assertions (the body-site structural
// fix, already landed before this session -- see ProcedureMappingRules' own
// top doc comment) plus a new performer test (zero dedicated tests existed
// for that in Go, per the inventory).

func TestDeclarativeEngine_Procedure_TargetSiteCode_SetsBodySite(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:      "procedure",
			StatusCode:     "completed",
			Code:           cdadocument.CDACode{Code: "44950", DisplayName: "Appendectomy", CodeSystem: "2.16.840.1.113883.6.12"},
			TargetSiteCode: &cdadocument.CDACode{Code: "66754008", DisplayName: "Appendix structure", CodeSystem: "2.16.840.1.113883.6.96"},
		},
	}
	documentMap := documentMapForEntries(t, "procedures", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.ProcedureMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 Procedure, got %d", len(resources))
	}
	bodySite, ok := resources[0]["bodySite"].([]interface{})
	if !ok || len(bodySite) != 1 {
		t.Fatalf("expected Procedure.bodySite to be set from TargetSiteCode, got %v", resources[0]["bodySite"])
	}
	coding := firstCoding(t, bodySite[0])
	if coding["code"] != "66754008" {
		t.Errorf("bodySite coding = %v, want SNOMED 66754008 (Appendix structure)", coding)
	}
}

func TestDeclarativeEngine_Procedure_NoTargetSiteCode_NoBodySiteEvenWithUnrelatedCOMP(t *testing.T) {
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
	documentMap := documentMapForEntries(t, "procedures", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.ProcedureMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if _, has := resources[0]["bodySite"]; has {
		t.Error("bodySite must not be inferred from an unrelated COMP entryRelationship")
	}
}

func TestDeclarativeEngine_Procedure_ObservationVariant_NoTargetSiteCode_NoBodySite(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{EntryType: "observation", StatusCode: "completed", Code: cdadocument.CDACode{Code: "44950"}},
	}
	documentMap := documentMapForEntries(t, "procedures", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.ProcedureMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if _, has := resources[0]["bodySite"]; has {
		t.Error("bodySite must not be set when TargetSiteCode is nil")
	}
}

func TestDeclarativeEngine_Procedure_Performer_PRFOrSPRF_BothMatch(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "procedure",
			StatusCode: "completed",
			Code:       cdadocument.CDACode{Code: "44950"},
			Participants: []cdadocument.CDAParticipant{
				{
					TypeCode:        "REF",
					ParticipantRole: cdadocument.CDAParticipantRole{PlayingEntity: &cdadocument.CDAPlayingEntity{Names: []cdadocument.CDAName{{Given: []string{"Not"}, Family: "APerformer"}}}},
				},
				{
					TypeCode:        "PRF",
					ParticipantRole: cdadocument.CDAParticipantRole{PlayingEntity: &cdadocument.CDAPlayingEntity{Names: []cdadocument.CDAName{{Given: []string{"Primary"}, Family: "Surgeon"}}}},
				},
				{
					TypeCode:        "SPRF",
					ParticipantRole: cdadocument.CDAParticipantRole{PlayingEntity: &cdadocument.CDAPlayingEntity{Names: []cdadocument.CDAName{{Given: []string{"Assisting"}, Family: "Surgeon"}}}},
				},
			},
		},
	}
	documentMap := documentMapForEntries(t, "procedures", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.ProcedureMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	performers, ok := resources[0]["performer"].([]interface{})
	if !ok || len(performers) != 2 {
		t.Fatalf("performer = %v, want a 2-element array (PRF and SPRF; REF must be excluded)", resources[0]["performer"])
	}
	first, _ := performers[0].(map[string]interface{})
	actor, _ := first["actor"].(map[string]interface{})
	if actor["display"] != "Primary Surgeon" {
		t.Errorf("performer[0].actor.display = %v, want \"Primary Surgeon\"", actor["display"])
	}
	second, _ := performers[1].(map[string]interface{})
	actor2, _ := second["actor"].(map[string]interface{})
	if actor2["display"] != "Assisting Surgeon" {
		t.Errorf("performer[1].actor.display = %v, want \"Assisting Surgeon\"", actor2["display"])
	}
}

// ---- Goals (standalone "goals" section) ----
//
// Zero dedicated tests exist for MapGoals in mappers_test.go (per the
// inventory) -- these are new coverage, not ports.

func TestDeclarativeEngine_Goal_DescriptionFromValueCode(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "observation",
			StatusCode: "active",
			Code:       cdadocument.CDACode{Code: "8", DisplayName: "Goal observation type"},
			Value: &cdadocument.CDAValue{
				Type: "CD",
				Code: &cdadocument.CDACode{Code: "162673000", DisplayName: "Weight loss goal", CodeSystem: "2.16.840.1.113883.6.96"},
			},
		},
	}
	documentMap := documentMapForEntries(t, "goals", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.GoalMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 Goal, got %d", len(resources))
	}
	if resources[0]["lifecycleStatus"] != "active" {
		t.Errorf("lifecycleStatus = %v, want active", resources[0]["lifecycleStatus"])
	}
	coding := firstCoding(t, resources[0]["description"])
	if coding["code"] != "162673000" {
		t.Errorf("description coding = %v, want 162673000 (from Value.Code, preferred over entry.Code)", coding)
	}
}

func TestDeclarativeEngine_Goal_NoValue_DescriptionFallsBackToCode(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{EntryType: "observation", StatusCode: "active", Code: cdadocument.CDACode{Code: "8", DisplayName: "Weight loss goal"}},
	}
	documentMap := documentMapForEntries(t, "goals", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.GoalMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	coding := firstCoding(t, resources[0]["description"])
	if coding["code"] != "8" {
		t.Errorf("description coding = %v, want code=8 (fallback to entry.Code)", coding)
	}
}

func TestDeclarativeEngine_Goal_NoValueNoCode_DescriptionPlaceholder(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{EntryType: "observation", StatusCode: "active"},
	}
	documentMap := documentMapForEntries(t, "goals", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.GoalMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	desc, ok := resources[0]["description"].(map[string]interface{})
	if !ok || desc["text"] != "Goal" {
		t.Errorf("description = %v, want the {\"text\":\"Goal\"} placeholder", resources[0]["description"])
	}
}

// ---- CarePlan / Goal (Plan of Care) ----
//
// Ports all 7 dedicated mappers_test.go assertions for MapPlanOfCare's
// per-entry dispatch. Tests run against the "planOfCare" section-key alias;
// the migration drift-guard test verifies the other 3 aliases are
// byte-identical rule sets, so this isn't re-tested per alias.

func TestDeclarativeEngine_PlanOfCare_PlannedProcedure_BecomesServiceRequest(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{EntryType: "procedure", MoodCode: "INT", StatusCode: "active", Code: cdadocument.CDACode{Code: "656", DisplayName: "Pap Test"}},
	}
	documentMap := documentMapForEntries(t, "planOfCare", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResourcesForRules(documentMap, cdafhir.PlanOfCareMappingRules())
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
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

func TestDeclarativeEngine_PlanOfCare_ProposedObservation_BecomesServiceRequestWithProposalIntent(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{EntryType: "observation", MoodCode: "PRP", StatusCode: "active", Code: cdadocument.CDACode{Code: "20", DisplayName: "Colorectal Cancer Screening"}},
	}
	documentMap := documentMapForEntries(t, "planOfCare", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResourcesForRules(documentMap, cdafhir.PlanOfCareMappingRules())
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
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

func TestDeclarativeEngine_PlanOfCare_SubstanceAdministration_ReusesMedicationRequestFields(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{EntryType: "substanceAdministration", MoodCode: "INT", StatusCode: "active"},
	}
	documentMap := documentMapForEntries(t, "planOfCare", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResourcesForRules(documentMap, cdafhir.PlanOfCareMappingRules())
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	if resources[0]["resourceType"] != "MedicationRequest" {
		t.Errorf("resourceType = %v, want MedicationRequest", resources[0]["resourceType"])
	}
	if _, hasRequester := resources[0]["requester"]; !hasRequester {
		t.Error("expected requester to be set (reused medicationRequestFields, us-core-21)")
	}
}

func TestDeclarativeEngine_PlanOfCare_PlannedEncounter_BecomesAppointment(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{EntryType: "encounter", MoodCode: "INT", StatusCode: "active", Code: cdadocument.CDACode{Code: "99213"}},
	}
	documentMap := documentMapForEntries(t, "planOfCare", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResourcesForRules(documentMap, cdafhir.PlanOfCareMappingRules())
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	if resources[0]["resourceType"] != "Appointment" {
		t.Errorf("resourceType = %v, want Appointment", resources[0]["resourceType"])
	}
}

func TestDeclarativeEngine_PlanOfCare_PlannedSupply_BecomesSupplyRequestWithHardcodedQuantity(t *testing.T) {
	// Zero dedicated Go tests exist for this branch (per the inventory) --
	// new coverage, not a port. quantity=1 is hardcoded in both Go and this
	// rule (CDA's Planned Supply template has no typed quantity field on
	// CDAEntry; SupplyRequest.quantity is required) -- a workaround, not a
	// real value, per plan_of_care_mapper.go's own doc comment.
	entries := []cdadocument.CDAEntry{
		{EntryType: "supply", MoodCode: "INT", StatusCode: "active", Code: cdadocument.CDACode{Code: "337388004", DisplayName: "Wheelchair"}},
	}
	documentMap := documentMapForEntries(t, "planOfCare", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResourcesForRules(documentMap, cdafhir.PlanOfCareMappingRules())
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	if resources[0]["resourceType"] != "SupplyRequest" {
		t.Errorf("resourceType = %v, want SupplyRequest", resources[0]["resourceType"])
	}
	qty, ok := resources[0]["quantity"].(map[string]interface{})
	if !ok || qty["value"] != float64(1) {
		t.Errorf("quantity = %v, want {\"value\":1} (hardcoded)", resources[0]["quantity"])
	}
	coding := firstCoding(t, resources[0]["itemCodeableConcept"])
	if coding["code"] != "337388004" {
		t.Errorf("itemCodeableConcept coding = %v, want code=337388004", coding)
	}
}

func TestDeclarativeEngine_PlanOfCare_GoalMood_ReusesGoalFields(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{EntryType: "observation", MoodCode: "GOL", StatusCode: "active", Code: cdadocument.CDACode{Code: "8", DisplayName: "Weight loss goal"}},
	}
	documentMap := documentMapForEntries(t, "planOfCare", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResourcesForRules(documentMap, cdafhir.PlanOfCareMappingRules())
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	if resources[0]["resourceType"] != "Goal" {
		t.Errorf("resourceType = %v, want Goal (moodCode=GOL takes priority over EntryType)", resources[0]["resourceType"])
	}
}

func TestDeclarativeEngine_PlanOfCare_EventMood_Skipped(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{EntryType: "procedure", MoodCode: "EVN", StatusCode: "completed", Code: cdadocument.CDACode{Code: "123"}},
	}
	documentMap := documentMapForEntries(t, "planOfCare", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResourcesForRules(documentMap, cdafhir.PlanOfCareMappingRules())
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 0 {
		t.Errorf("expected 0 resources for moodCode=EVN (already happened, not a plan entry), got %d", len(resources))
	}
}

func TestDeclarativeEngine_PlanOfCare_OrganizerComponents_AreFlattened(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType: "organizer",
			Components: []cdadocument.CDAEntry{
				{EntryType: "procedure", MoodCode: "INT", StatusCode: "active", Code: cdadocument.CDACode{Code: "1"}},
				{EntryType: "observation", MoodCode: "PRP", StatusCode: "active", Code: cdadocument.CDACode{Code: "2"}},
			},
		},
	}
	documentMap := documentMapForEntries(t, "planOfCare", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResourcesForRules(documentMap, cdafhir.PlanOfCareMappingRules())
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 2 {
		t.Fatalf("expected 2 resources (organizer flattened into its components), got %d", len(resources))
	}
}

// ---- CareTeam / Practitioner (EmitAsResource + RequiredPaths) ----

func TestDeclarativeEngine_CareTeam_BuildsCareTeamAndPractitioner(t *testing.T) {
	// Ported 1:1 from mappers_test.go's TestMapCareTeam_BuildsCareTeamAndPractitioner.
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
	documentMap := documentMapForEntries(t, "careTeam", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.CareTeamMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}

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
	role := firstCoding(t, firstElement(t, p["role"]))
	if role["code"] != "PCP" {
		t.Errorf("participant.role coding = %v, want code=PCP", role)
	}
	// Practitioner built from identifier only -- no playingEntity name in
	// this entry, and component-performer enrichment is deliberately not
	// ported (see CareTeamMappingRules' doc comment).
	if _, hasName := practitioner["name"]; hasName {
		t.Errorf("expected no Practitioner.name (no playingEntity, enrichment not ported), got %v", practitioner["name"])
	}
	ident := firstElement(t, practitioner["identifier"]).(map[string]interface{})
	if ident["value"] != "1013027903" {
		t.Errorf("Practitioner.identifier[0].value = %v, want 1013027903", ident["value"])
	}
}

func TestDeclarativeEngine_CareTeam_NoParticipants_ProducesNoCareTeam(t *testing.T) {
	// Ported 1:1 from mappers_test.go's TestMapCareTeam_NoParticipants_ProducesNoCareTeam --
	// proves MappingRule.RequiredPaths actually discards the resource even
	// though status/category would otherwise populate it.
	entries := []cdadocument.CDAEntry{
		{EntryType: "organizer", StatusCode: "active", Code: cdadocument.CDACode{Code: "86744-0"}},
	}
	documentMap := documentMapForEntries(t, "careTeam", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.CareTeamMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 0 {
		t.Errorf("expected 0 resources when the organizer has no usable participants, got %d", len(resources))
	}
}

func TestDeclarativeEngine_CareTeam_PlayingEntityName_BuildsPractitionerWithName(t *testing.T) {
	// New coverage (no Go precedent needed beyond the existing
	// buildPractitionerResource fields): proves the EmitAsResource Fields
	// pull name/qualification/telecom from participantRole, not just identifier.
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "organizer",
			StatusCode: "active",
			Participants: []cdadocument.CDAParticipant{
				{
					ParticipantRole: cdadocument.CDAParticipantRole{
						Code:     cdadocument.CDACode{Code: "207Q00000X", DisplayName: "Family Medicine"},
						Telecoms: []cdadocument.CDATelecom{{Value: "tel:+1-555-0100", Use: "WP"}},
						PlayingEntity: &cdadocument.CDAPlayingEntity{
							Names: []cdadocument.CDAName{{Family: "Smith", Given: []string{"Jane"}}},
						},
					},
				},
			},
		},
	}
	documentMap := documentMapForEntries(t, "careTeam", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.CareTeamMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	var practitioner map[string]interface{}
	for _, r := range resources {
		if r["resourceType"] == "Practitioner" {
			practitioner = r
		}
	}
	if practitioner == nil {
		t.Fatal("expected a Practitioner resource")
	}
	name := firstElement(t, practitioner["name"]).(map[string]interface{})
	if name["family"] != "Smith" {
		t.Errorf("Practitioner.name[0].family = %v, want Smith", name["family"])
	}
	telecom := firstElement(t, practitioner["telecom"]).(map[string]interface{})
	if telecom["system"] != "phone" {
		t.Errorf("Practitioner.telecom[0].system = %v, want phone", telecom["system"])
	}
	qual, ok := practitioner["qualification"].([]interface{})
	if !ok || len(qual) != 1 {
		t.Fatalf("Practitioner.qualification = %v, want 1 element", practitioner["qualification"])
	}
}

func TestDeclarativeEngine_CareTeam_MultipleParticipants_EachGetsDistinctCrossReferencedPractitioner(t *testing.T) {
	// Proves buildEmittedSubResource's idx-based id synthesis keeps multiple
	// participants within ONE entry distinct (not all colliding on the same
	// "practitioner-1") and that each CareTeam.participant[i].member points
	// at the matching, not some other, Practitioner.
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "organizer",
			StatusCode: "active",
			Participants: []cdadocument.CDAParticipant{
				{
					FunctionCode: cdadocument.CDACode{Code: "PCP"},
					ParticipantRole: cdadocument.CDAParticipantRole{
						PlayingEntity: &cdadocument.CDAPlayingEntity{Names: []cdadocument.CDAName{{Family: "Alpha"}}},
					},
				},
				{
					FunctionCode: cdadocument.CDACode{Code: "NURSE"},
					ParticipantRole: cdadocument.CDAParticipantRole{
						PlayingEntity: &cdadocument.CDAPlayingEntity{Names: []cdadocument.CDAName{{Family: "Beta"}}},
					},
				},
			},
		},
	}
	documentMap := documentMapForEntries(t, "careTeam", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.CareTeamMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}

	var practitioners []map[string]interface{}
	var careTeam map[string]interface{}
	for _, r := range resources {
		switch r["resourceType"] {
		case "Practitioner":
			practitioners = append(practitioners, r)
		case "CareTeam":
			careTeam = r
		}
	}
	if len(practitioners) != 2 {
		t.Fatalf("expected 2 distinct Practitioner resources, got %d", len(practitioners))
	}
	if practitioners[0]["id"] == practitioners[1]["id"] {
		t.Fatalf("expected distinct ids, both practitioners got %v", practitioners[0]["id"])
	}

	participants, _ := careTeam["participant"].([]interface{})
	if len(participants) != 2 {
		t.Fatalf("CareTeam.participant length = %d, want 2", len(participants))
	}
	byFamily := map[string]string{}
	for _, pr := range practitioners {
		name := firstElement(t, pr["name"]).(map[string]interface{})
		byFamily[name["family"].(string)] = pr["id"].(string)
	}
	for i, want := range []string{"PCP", "NURSE"} {
		p := participants[i].(map[string]interface{})
		role := firstCoding(t, firstElement(t, p["role"]))
		if role["code"] != want {
			t.Errorf("participant[%d].role = %v, want code=%s", i, role, want)
		}
	}
	// Cross-reference correctness: participant[0] (Alpha/PCP) must point at
	// Alpha's Practitioner, not Beta's.
	member0 := participants[0].(map[string]interface{})["member"].(map[string]interface{})
	if member0["reference"] != "Practitioner/"+byFamily["Alpha"] {
		t.Errorf("participant[0].member.reference = %v, want Practitioner/%s", member0["reference"], byFamily["Alpha"])
	}
	member1 := participants[1].(map[string]interface{})["member"].(map[string]interface{})
	if member1["reference"] != "Practitioner/"+byFamily["Beta"] {
		t.Errorf("participant[1].member.reference = %v, want Practitioner/%s", member1["reference"], byFamily["Beta"])
	}
}

// ---- Coverage ----

func TestDeclarativeEngine_Coverage_PayorFromOuterParticipant(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "act",
			Code:       cdadocument.CDACode{Code: "48768-6", DisplayName: "Payment sources"},
			StatusCode: "completed",
			Participants: []cdadocument.CDAParticipant{
				{
					TypeCode: "HLD",
					ParticipantRole: cdadocument.CDAParticipantRole{
						ScopingEntity: &cdadocument.CDAEntity{Desc: "Acme Health Plan"},
					},
				},
			},
		},
	}
	documentMap := documentMapForEntries(t, "payersInsurance", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.CoverageMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	r := resources[0]
	if r["status"] != "active" {
		t.Errorf("status = %v, want active", r["status"])
	}
	typ := r["type"].(map[string]interface{})
	if typ["text"] != "Payment sources Document" {
		t.Errorf("type.text = %v, want corrected display 'Payment sources Document'", typ["text"])
	}
	payor := firstElement(t, r["payor"]).(map[string]interface{})
	if payor["display"] != "Acme Health Plan" {
		t.Errorf("payor[0].display = %v, want Acme Health Plan", payor["display"])
	}
	rel := r["relationship"].(map[string]interface{})
	if firstCoding(t, rel)["code"] != "self" {
		t.Errorf("relationship coding = %v, want code=self", firstCoding(t, rel))
	}
}

func TestDeclarativeEngine_Coverage_PayorFallsBackToCOMPPerformerOrg(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType: "act",
			Code:      cdadocument.CDACode{Code: "48768-6"},
			EntryRelationships: []cdadocument.CDAEntryRelationship{
				{
					TypeCode: "COMP",
					Entry: cdadocument.CDAEntry{
						Id: []cdadocument.CDAII{{Root: "1.2.3", Extension: "MEMBER123"}},
						Performers: []cdadocument.CDAPerformer{
							{AssignedEntity: cdadocument.CDAAssignedEntity{
								RepresentedOrganization: &cdadocument.CDAOrganization{Names: []string{"Beta Insurance Co"}},
							}},
						},
					},
				},
			},
		},
	}
	documentMap := documentMapForEntries(t, "payersInsurance", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.CoverageMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	r := resources[0]
	payor := firstElement(t, r["payor"]).(map[string]interface{})
	if payor["display"] != "Beta Insurance Co" {
		t.Errorf("payor[0].display = %v, want Beta Insurance Co (fallback tier)", payor["display"])
	}
	if r["subscriberId"] != "MEMBER123" {
		t.Errorf("subscriberId = %v, want MEMBER123 (extension preferred over root)", r["subscriberId"])
	}
}

func TestDeclarativeEngine_Coverage_NoPayorInfo_UsesUnknownPlaceholder(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{EntryType: "act", Code: cdadocument.CDACode{Code: "48768-6"}},
	}
	documentMap := documentMapForEntries(t, "payersInsurance", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.CoverageMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	payor := firstElement(t, resources[0]["payor"]).(map[string]interface{})
	if payor["display"] != "Unknown" {
		t.Errorf("payor[0].display = %v, want Unknown placeholder", payor["display"])
	}
}

func TestDeclarativeEngine_Coverage_SubscriberId_RootFallbackWhenNoExtension(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType: "act",
			Code:      cdadocument.CDACode{Code: "48768-6"},
			EntryRelationships: []cdadocument.CDAEntryRelationship{
				{
					TypeCode: "COMP",
					Entry: cdadocument.CDAEntry{
						Id: []cdadocument.CDAII{{Root: "1.2.3.4.5"}},
					},
				},
			},
		},
	}
	documentMap := documentMapForEntries(t, "payersInsurance", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.CoverageMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if resources[0]["subscriberId"] != "1.2.3.4.5" {
		t.Errorf("subscriberId = %v, want root fallback 1.2.3.4.5", resources[0]["subscriberId"])
	}
}

// ---- FamilyMemberHistory ----

func TestDeclarativeEngine_FamilyMemberHistory_RelationshipNameAndConditions(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType: "organizer",
			Participants: []cdadocument.CDAParticipant{
				{
					TypeCode: "SBJ",
					ParticipantRole: cdadocument.CDAParticipantRole{
						Code: cdadocument.CDACode{Code: "MTH", DisplayName: "Mother"},
						PlayingEntity: &cdadocument.CDAPlayingEntity{
							Names: []cdadocument.CDAName{{Family: "Doe", Given: []string{"Jane"}}},
						},
					},
				},
			},
			Components: []cdadocument.CDAEntry{
				{
					Value:         &cdadocument.CDAValue{Type: "CD", Code: &cdadocument.CDACode{Code: "59621000", DisplayName: "Hypertension"}},
					EffectiveTime: cdadocument.CDATimeRange{Low: cdadocument.CDATime{Value: "20100101"}},
				},
			},
		},
	}
	documentMap := documentMapForEntries(t, "familyHistory", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.FamilyMemberHistoryMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	r := resources[0]
	if r["status"] != "completed" {
		t.Errorf("status = %v, want completed", r["status"])
	}
	if r["name"] != "Doe" {
		t.Errorf("name = %v, want Doe (lossy: family only, given discarded)", r["name"])
	}
	rel := firstCoding(t, r["relationship"])
	if rel["code"] != "MTH" {
		t.Errorf("relationship coding = %v, want code=MTH", rel)
	}
	conditions, ok := r["condition"].([]interface{})
	if !ok || len(conditions) != 1 {
		t.Fatalf("condition = %v, want 1 element", r["condition"])
	}
	cond := conditions[0].(map[string]interface{})
	code := firstCoding(t, cond["code"])
	if code["code"] != "59621000" {
		t.Errorf("condition[0].code coding = %v, want code=59621000", code)
	}
	if cond["onsetDateTime"] == "" || cond["onsetDateTime"] == nil {
		t.Errorf("condition[0].onsetDateTime not set")
	}
}

func TestDeclarativeEngine_FamilyMemberHistory_NoSBJParticipant_NoRelationshipOrName(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{EntryType: "organizer"},
	}
	documentMap := documentMapForEntries(t, "familyHistory", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.FamilyMemberHistoryMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource (status alone is enough to be non-empty), got %d", len(resources))
	}
	if _, has := resources[0]["relationship"]; has {
		t.Errorf("expected no relationship, got %v", resources[0]["relationship"])
	}
	if _, has := resources[0]["name"]; has {
		t.Errorf("expected no name, got %v", resources[0]["name"])
	}
}

// ---- Device (DeviceUseStatement) ----

func TestDeclarativeEngine_Device_PRDParticipantCode_PreferredOverEntryCode(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "supply",
			StatusCode: "active",
			Code:       cdadocument.CDACode{Code: "999999", DisplayName: "Generic Equipment"},
			Participants: []cdadocument.CDAParticipant{
				{
					TypeCode: "PRD",
					ParticipantRole: cdadocument.CDAParticipantRole{
						PlayingEntity: &cdadocument.CDAPlayingEntity{
							Code: cdadocument.CDACode{Code: "58938008", DisplayName: "Wheelchair"},
						},
					},
				},
			},
			EffectiveTime: cdadocument.CDATimeRange{Low: cdadocument.CDATime{Value: "20200101"}, High: cdadocument.CDATime{Value: "20210101"}},
		},
	}
	documentMap := documentMapForEntries(t, "medicalEquipment", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.DeviceMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	r := resources[0]
	if r["status"] != "active" {
		t.Errorf("status = %v, want active", r["status"])
	}
	device := r["device"].(map[string]interface{})
	coding := firstCoding(t, device["codeableConcept"])
	if coding["code"] != "58938008" {
		t.Errorf("device.codeableConcept coding = %v, want PRD participant code 58938008 (not entry code 999999)", coding)
	}
	if _, has := r["timingPeriod"]; !has {
		t.Errorf("expected timingPeriod to be set")
	}
}

func TestDeclarativeEngine_Device_NoPRDParticipant_FallsBackToEntryCode(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "supply",
			StatusCode: "unrecognized-status",
			Code:       cdadocument.CDACode{Code: "999999", DisplayName: "Generic Equipment"},
		},
	}
	documentMap := documentMapForEntries(t, "medicalEquipment", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.DeviceMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	r := resources[0]
	if r["status"] != "active" {
		t.Errorf("status = %v, want active (unrecognized statusCode coerces to active)", r["status"])
	}
	device := r["device"].(map[string]interface{})
	coding := firstCoding(t, device["codeableConcept"])
	if coding["code"] != "999999" {
		t.Errorf("device.codeableConcept coding = %v, want entry code 999999 (no PRD participant present)", coding)
	}
}

func firstElement(t testing.TB, v interface{}) interface{} {
	t.Helper()
	arr, ok := v.([]interface{})
	if !ok || len(arr) == 0 {
		t.Fatalf("expected a non-empty array, got %v", v)
	}
	return arr[0]
}

func documentMapForHeader(t testing.TB, header cdadocument.CDAHeader) map[string]interface{} {
	t.Helper()
	doc := map[string]interface{}{"header": header}
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

// ---- Author / Custodian (header-level) ----

func TestDeclarativeEngine_Author_FirstAuthorWithPerson_BuildsPractitioner(t *testing.T) {
	header := cdadocument.CDAHeader{
		Authors: []cdadocument.CDAAuthor{
			{AssignedAuthor: cdadocument.CDAAssignedAuthor{
				AssignedAuthoringDevice: &cdadocument.CDADevice{SoftwareName: "EHR System"},
			}},
			{AssignedAuthor: cdadocument.CDAAssignedAuthor{
				Ids:            []cdadocument.CDAII{{Root: "2.16.840.1.113883.4.6", Extension: "1234567890"}},
				Telecoms:       []cdadocument.CDATelecom{{Value: "tel:+1-555-0101"}},
				AssignedPerson: &cdadocument.CDAPerson{Names: []cdadocument.CDAName{{Family: "Smith", Given: []string{"Alice"}}}},
			}},
		},
	}
	documentMap := documentMapForHeader(t, header)
	engine := cdafhir.NewDeclarativeEngine()
	resource, extra, errs := engine.BuildHeaderResource(documentMap, cdafhir.AuthorMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(extra) != 0 {
		t.Fatalf("expected no extra resources, got %d", len(extra))
	}
	if resource == nil {
		t.Fatal("expected a Practitioner resource")
	}
	if resource["resourceType"] != "Practitioner" {
		t.Errorf("resourceType = %v, want Practitioner", resource["resourceType"])
	}
	name := firstElement(t, resource["name"]).(map[string]interface{})
	if name["family"] != "Smith" {
		t.Errorf("name[0].family = %v, want Smith (the device-authoring first author has no person, skipped)", name["family"])
	}
	ident := firstElement(t, resource["identifier"]).(map[string]interface{})
	if ident["value"] != "1234567890" {
		t.Errorf("identifier[0].value = %v, want 1234567890", ident["value"])
	}
}

func TestDeclarativeEngine_Author_NoAuthorWithPerson_NoResource(t *testing.T) {
	header := cdadocument.CDAHeader{
		Authors: []cdadocument.CDAAuthor{
			{AssignedAuthor: cdadocument.CDAAssignedAuthor{
				AssignedAuthoringDevice: &cdadocument.CDADevice{SoftwareName: "EHR System"},
			}},
		},
	}
	documentMap := documentMapForHeader(t, header)
	engine := cdafhir.NewDeclarativeEngine()
	resource, _, errs := engine.BuildHeaderResource(documentMap, cdafhir.AuthorMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if resource != nil {
		t.Errorf("expected no resource (only author is device-authored, no person), got %v", resource)
	}
}

func TestDeclarativeEngine_Author_NoAuthorsAtAll_NoResource(t *testing.T) {
	documentMap := documentMapForHeader(t, cdadocument.CDAHeader{})
	engine := cdafhir.NewDeclarativeEngine()
	resource, _, errs := engine.BuildHeaderResource(documentMap, cdafhir.AuthorMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if resource != nil {
		t.Errorf("expected no resource, got %v", resource)
	}
}

func TestDeclarativeEngine_Custodian_SetsActiveTrue(t *testing.T) {
	// Ported 1:1 from mappers_test.go's TestMapCustodian_SetsActiveTrue.
	header := cdadocument.CDAHeader{
		Custodian: cdadocument.CDACustodian{
			AssignedCustodian: cdadocument.CDAAssignedCustodian{
				RepresentedCustodianOrganization: cdadocument.CDAOrganization{
					Names: []string{"Boulder Community Health and Affiliates"},
				},
			},
		},
	}
	documentMap := documentMapForHeader(t, header)
	engine := cdafhir.NewDeclarativeEngine()
	resource, _, errs := engine.BuildHeaderResource(documentMap, cdafhir.CustodianMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if resource == nil {
		t.Fatal("expected an Organization resource")
	}
	if active, _ := resource["active"].(bool); !active {
		t.Error("Organization.active must be true (required by us-core-organization)")
	}
	if resource["name"] != "Boulder Community Health and Affiliates" {
		t.Errorf("name = %v, want Boulder Community Health and Affiliates", resource["name"])
	}
}

func TestDeclarativeEngine_Custodian_NoName_NoResource(t *testing.T) {
	// RequiredPaths:["name"] proof -- mirrors patient_mapper.go's
	// `if len(org.Names) == 0 { return nil }`.
	documentMap := documentMapForHeader(t, cdadocument.CDAHeader{})
	engine := cdafhir.NewDeclarativeEngine()
	resource, _, errs := engine.BuildHeaderResource(documentMap, cdafhir.CustodianMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if resource != nil {
		t.Errorf("expected no resource (no custodian name), got %v", resource)
	}
}

func TestDeclarativeEngine_Custodian_IdentifiersAndAddresses(t *testing.T) {
	header := cdadocument.CDAHeader{
		Custodian: cdadocument.CDACustodian{
			AssignedCustodian: cdadocument.CDAAssignedCustodian{
				RepresentedCustodianOrganization: cdadocument.CDAOrganization{
					Names:     []string{"Get Well Clinic"},
					Ids:       []cdadocument.CDAII{{Root: "1.2.3.4", Extension: "ORG123"}},
					Addresses: []cdadocument.CDAAddress{{StreetLines: []string{"100 Main St"}, City: "Boulder", State: "CO", PostalCode: "80301"}},
				},
			},
		},
	}
	documentMap := documentMapForHeader(t, header)
	engine := cdafhir.NewDeclarativeEngine()
	resource, _, errs := engine.BuildHeaderResource(documentMap, cdafhir.CustodianMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	ident := firstElement(t, resource["identifier"]).(map[string]interface{})
	if ident["value"] != "ORG123" {
		t.Errorf("identifier[0].value = %v, want ORG123", ident["value"])
	}
	addr := firstElement(t, resource["address"]).(map[string]interface{})
	if addr["city"] != "Boulder" {
		t.Errorf("address[0].city = %v, want Boulder", addr["city"])
	}
}

// ---- LegalAuthenticator (header-level, new functionality) ----

func TestDeclarativeEngine_LegalAuthenticator_BuildsPractitioner(t *testing.T) {
	header := cdadocument.CDAHeader{
		LegalAuthenticator: &cdadocument.CDALegalAuthenticator{
			Time:          cdadocument.CDATime{Value: "20230615120000"},
			SignatureCode: "S",
			AssignedEntity: cdadocument.CDAAssignedEntity{
				Ids:      []cdadocument.CDAII{{Root: "2.16.840.1.113883.4.6", Extension: "9988776655"}},
				Telecoms: []cdadocument.CDATelecom{{Value: "tel:+1-555-0199"}},
				AssignedPerson: &cdadocument.CDAPerson{
					Names: []cdadocument.CDAName{{Family: "Howser", Given: []string{"Douglas"}}},
				},
			},
		},
	}
	documentMap := documentMapForHeader(t, header)
	engine := cdafhir.NewDeclarativeEngine()
	resource, extra, errs := engine.BuildHeaderResource(documentMap, cdafhir.LegalAuthenticatorMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(extra) != 0 {
		t.Fatalf("expected no extra resources, got %d", len(extra))
	}
	if resource == nil {
		t.Fatal("expected a Practitioner resource")
	}
	if resource["resourceType"] != "Practitioner" {
		t.Errorf("resourceType = %v, want Practitioner", resource["resourceType"])
	}
	name := firstElement(t, resource["name"]).(map[string]interface{})
	if name["family"] != "Howser" {
		t.Errorf("name[0].family = %v, want Howser", name["family"])
	}
	ident := firstElement(t, resource["identifier"]).(map[string]interface{})
	if ident["value"] != "9988776655" {
		t.Errorf("identifier[0].value = %v, want 9988776655", ident["value"])
	}
}

func TestDeclarativeEngine_LegalAuthenticator_Absent_NoResource(t *testing.T) {
	documentMap := documentMapForHeader(t, cdadocument.CDAHeader{})
	engine := cdafhir.NewDeclarativeEngine()
	resource, _, errs := engine.BuildHeaderResource(documentMap, cdafhir.LegalAuthenticatorMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if resource != nil {
		t.Errorf("expected no resource (no legalAuthenticator present), got %v", resource)
	}
}

func TestDeclarativeEngine_LegalAuthenticator_NoAssignedPersonName_NoResource(t *testing.T) {
	header := cdadocument.CDAHeader{
		LegalAuthenticator: &cdadocument.CDALegalAuthenticator{
			Time:           cdadocument.CDATime{Value: "20230615120000"},
			SignatureCode:  "S",
			AssignedEntity: cdadocument.CDAAssignedEntity{},
		},
	}
	documentMap := documentMapForHeader(t, header)
	engine := cdafhir.NewDeclarativeEngine()
	resource, _, errs := engine.BuildHeaderResource(documentMap, cdafhir.LegalAuthenticatorMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if resource != nil {
		t.Errorf("expected no resource (RequiredPaths:[name] gate, no assignedPerson), got %v", resource)
	}
}
