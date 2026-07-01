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
	"context"
	"encoding/base64"
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

// TestDeclarativeEngine_Allergy_NoKnownAllergies_CodeInvertedViaConceptMap
// covers the real "No Known Allergies" idiom as it actually appears in C-CDA
// documents (confirmed against HL7's own C-CDA-Examples "No Known Medication
// Allergies" sample and a real Epic-generated CCD): a CSM participant IS
// present, but its playingEntity/code is nullFlavor="NA" (no real substance
// named) -- distinct from no CSM participant at all, which is the OTHER test
// below. Verified against the HL7 C-CDA on FHIR IG (CF-allergies.md +
// ConceptMap-CF-NoKnownAllergies): code must invert 419199007 "Allergy to
// substance" -> 716186003 "No known allergy", and verificationStatus must
// stay "confirmed" -- the IG's allergy table has no verificationStatus row
// for negation at all, so "refuted" (Go's old behavior) is never correct
// here: FHIR's own "refuted" definition requires "the identified substance",
// which a generic, substance-less NKDA assertion never has.
func TestDeclarativeEngine_Allergy_NoKnownAllergies_CodeInvertedViaConceptMap(t *testing.T) {
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
						Participants: []cdadocument.CDAParticipant{
							{
								TypeCode: "CSM",
								ParticipantRole: cdadocument.CDAParticipantRole{
									PlayingEntity: &cdadocument.CDAPlayingEntity{
										Code: cdadocument.CDACode{NullFlavor: "NA"},
									},
								},
							},
						},
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
	codeCoding := firstCoding(t, resources[0]["code"])
	if codeCoding["code"] != "716186003" {
		t.Errorf("code.coding[0].code = %v, want 716186003 (No known allergy, via ConceptMap-CF-NoKnownAllergies)", codeCoding["code"])
	}
	if codeCoding["system"] != "http://snomed.info/sct" {
		t.Errorf("code.coding[0].system = %v, want http://snomed.info/sct", codeCoding["system"])
	}
	verCoding := firstCoding(t, resources[0]["verificationStatus"])
	if verCoding["code"] != "confirmed" {
		t.Errorf("verificationStatus.code = %v, want \"confirmed\" -- the IG has no verificationStatus row for negation; refuted requires an identified substance this entry doesn't have", verCoding["code"])
	}
}

// TestDeclarativeEngine_Allergy_NoCSMParticipantAtAll_CodeFallsBackToAssertionValue
// covers the OTHER no-substance shape: no CSM participant in the document at
// all (vs. nullFlavor'd, covered above). Without a negationInd=true the IG's
// ConceptMap doesn't apply (nothing to invert), so .code keeps the raw
// assertion value as-is.
func TestDeclarativeEngine_Allergy_NoCSMParticipantAtAll_CodeFallsBackToAssertionValue(t *testing.T) {
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
	documentMap := documentMapForEntries(t, "allergiesAndIntolerances", entries)
	engine := cdafhir.NewDeclarativeEngine()

	resources, errs := engine.BuildResources(documentMap, cdafhir.AllergyMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 AllergyIntolerance, got %d", len(resources))
	}
	codeCoding := firstCoding(t, resources[0]["code"])
	if codeCoding["code"] != "419199007" {
		t.Errorf("code.coding[0].code = %v, want 419199007 (raw assertion value, not negated)", codeCoding["code"])
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

// TestDeclarativeEngine_Allergy_Criticality covers the previously-missing
// Criticality row (see declarative_oob_rules.go's doc comment on it --
// "criticality" had zero references anywhere in services/cda_fhir before
// this fix). Mirrors the real CDA shape: a Criticality Observation
// (code 82606-5) nested under the SUBJ allergy-assertion observation via an
// inverted SUBJ entryRelationship, sibling to the CSM participant.
func TestDeclarativeEngine_Allergy_Criticality(t *testing.T) {
	criticalityEntry := func(valueCode, nullFlavor string) cdadocument.CDAEntry {
		return cdadocument.CDAEntry{
			EntryType: "observation",
			Code:      cdadocument.CDACode{Code: "82606-5", CodeSystem: "2.16.840.1.113883.6.1"},
			Value: &cdadocument.CDAValue{
				Type: "CD",
				Code: &cdadocument.CDACode{Code: valueCode, NullFlavor: nullFlavor},
			},
		}
	}

	t.Run("CRITH maps to high", func(t *testing.T) {
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
									TypeCode:     "SUBJ",
									InversionInd: true,
									Entry:        criticalityEntry("CRITH", ""),
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
		if resources[0]["criticality"] != "high" {
			t.Errorf("criticality = %v, want \"high\" (CRITH via ConceptMap-CF-Criticality)", resources[0]["criticality"])
		}
	})

	t.Run("nullFlavor=UNK produces no criticality field", func(t *testing.T) {
		// The real document this fix was found against: Criticality value
		// is nullFlavor="UNK", not a real CRITH/CRITL/CRITU code -- must
		// stay absent, not default to any value.
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
								Code: &cdadocument.CDACode{Code: "419199007", CodeSystem: "2.16.840.1.113883.6.96"},
							},
							EntryRelationships: []cdadocument.CDAEntryRelationship{
								{
									TypeCode:     "SUBJ",
									InversionInd: true,
									Entry:        criticalityEntry("", "UNK"),
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
		if _, ok := resources[0]["criticality"]; ok {
			t.Errorf("expected no criticality field for nullFlavor=UNK, got %v", resources[0]["criticality"])
		}
	})
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

// TestDeclarativeEngine_Medication_RequesterFromAuthor_RichData_EmitsPractitionerRole
// proves a real production gap found in the same 99397 CCD sample: the
// substanceAdministration's own <author><assignedAuthor> carries an NPI, a
// NUCC specialty code, a work address/phone, AND a representedOrganization
// (id at minimum) -- none of which requesterFromPerformer/requesterFromAuthor
// existed to capture before this fix; only the bare name reached the output.
// Per explicit product decision: build the full PractitionerRole when the
// source has enough structured data to justify it (organization present),
// fall back to display-only when it doesn't (see the sibling
// _NeverEmpty/_RequesterFromPerformer tests above, whose fixtures have no
// organization and must keep resolving to a bare display reference).
func TestDeclarativeEngine_Medication_RequesterFromAuthor_RichData_EmitsPractitionerRole(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "substanceAdministration",
			MoodCode:   "INT",
			StatusCode: "active",
			Authors: []cdadocument.CDAAuthor{
				{
					AssignedAuthor: cdadocument.CDAAssignedAuthor{
						Ids: []cdadocument.CDAII{{Root: "2.16.840.1.113883.4.6", Extension: "1013027903"}},
						Addresses: []cdadocument.CDAAddress{
							{StreetLines: []string{"1000 W SOUTH RD SUITE 110"}, City: "LAFAYETTE", State: "CO", PostalCode: "80026", Country: "USA"},
						},
						Telecoms:       []cdadocument.CDATelecom{{Value: "tel:+1-303-415-4355"}},
						AssignedPerson: &cdadocument.CDAPerson{Names: []cdadocument.CDAName{{Given: []string{"Herman"}, Family: "Damek"}}},
						RepresentedOrganization: &cdadocument.CDAOrganization{
							Ids: []cdadocument.CDAII{{Root: "1.2.840.114350.1.13.549.2.7.2.688879", Extension: "41800"}},
						},
					},
				},
			},
		},
	}
	resources := buildMedicationResources(t, entries)

	var medReq, practitionerRole map[string]interface{}
	for _, r := range resources {
		switch r["resourceType"] {
		case "MedicationRequest":
			medReq = r
		case "PractitionerRole":
			practitionerRole = r
		}
	}
	if medReq == nil {
		t.Fatalf("expected a MedicationRequest among resources, got %+v", resources)
	}
	if practitionerRole == nil {
		t.Fatalf("expected an emitted PractitionerRole among resources (rich author data should produce one), got %+v", resources)
	}
	requesterRef, ok := medReq["requester"].(map[string]interface{})
	if !ok {
		t.Fatalf("MedicationRequest.requester not set: %v", medReq["requester"])
	}
	if _, hasDisplay := requesterRef["display"]; hasDisplay {
		t.Errorf("requester = %v, want a PractitionerRole reference, not a display-only fallback", requesterRef)
	}
	if ref, _ := requesterRef["reference"].(string); ref == "" {
		t.Error("requester.reference must point at the emitted PractitionerRole")
	}
}

// TestDeclarativeEngine_Medication_RequesterPriority_AuthorOverPerformer
// proves the priority swap found by checking HL7's C-CDA on FHIR IG
// directly (CF-medications.html's Medication Activity mapping table lists
// "/author -> .requester", not /performer) -- this codebase originally
// inherited "performer, else author" from medication_mapper.go, which this
// fix reverses. Both performer and author are present here with only a
// name (no organization data, so neither rich PractitionerRole tier fires
// -- see TestDeclarativeEngine_Medication_RequesterFromAuthor_RichData_
// EmitsPractitionerRole above for that case), isolating the display-only
// fallback row's own SourcePath/FallbackPaths order as what this test
// actually exercises.
func TestDeclarativeEngine_Medication_RequesterPriority_AuthorOverPerformer(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "substanceAdministration",
			MoodCode:   "INT",
			StatusCode: "active",
			Performers: []cdadocument.CDAPerformer{
				{
					AssignedEntity: cdadocument.CDAAssignedEntity{
						AssignedPerson: &cdadocument.CDAPerson{
							Names: []cdadocument.CDAName{{Given: []string{"Performer"}, Family: "Person"}},
						},
					},
				},
			},
			Authors: []cdadocument.CDAAuthor{
				{
					AssignedAuthor: cdadocument.CDAAssignedAuthor{
						AssignedPerson: &cdadocument.CDAPerson{
							Names: []cdadocument.CDAName{{Given: []string{"Author"}, Family: "Person"}},
						},
					},
				},
			},
		},
	}
	resources := buildMedicationResources(t, entries)
	if len(resources) != 1 {
		t.Fatalf("expected 1 MedicationRequest, got %d", len(resources))
	}
	requester, ok := resources[0]["requester"].(map[string]interface{})
	if !ok {
		t.Fatal("MedicationRequest.requester not set")
	}
	if requester["display"] != "Author Person" {
		t.Errorf("requester.display = %v, want %q (author must win over performer per the IG)", requester["display"], "Author Person")
	}
}

// TestDeclarativeEngine_Medication_CommentActivity_SetsNote proves the
// MedicationRequest.note / MedicationStatement.note mapping HL7's C-CDA on
// FHIR IG specifies (CF-medications.html: Comment Activity, code 48767-8,
// via entryRelationship -> .note as an Annotation) but this codebase had no
// row for at all. typeCode="COMP" matches this exact 99397 CCD sample's own
// real (non-medication) Comment Activity usage -- see
// commentActivityTemplateID's doc comment.
func TestDeclarativeEngine_Medication_CommentActivity_SetsNote(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "substanceAdministration",
			MoodCode:   "INT",
			StatusCode: "active",
			EntryRelationships: []cdadocument.CDAEntryRelationship{
				{
					TypeCode: "COMP",
					Entry: cdadocument.CDAEntry{
						EntryType:   "act",
						TemplateIds: []string{"2.16.840.1.113883.10.20.22.4.64"},
						Text:        "Patient reports occasional nausea after dosing.",
					},
				},
			},
		},
	}
	resources := buildMedicationResources(t, entries)
	if len(resources) != 1 {
		t.Fatalf("expected 1 MedicationRequest, got %d", len(resources))
	}
	notes, ok := resources[0]["note"].([]interface{})
	if !ok || len(notes) != 1 {
		t.Fatalf("expected 1 note, got %v", resources[0]["note"])
	}
	note, ok := notes[0].(map[string]interface{})
	if !ok {
		t.Fatalf("note[0] not a map: %v", notes[0])
	}
	if note["text"] != "Patient reports occasional nausea after dosing." {
		t.Errorf("note[0].text = %v, want %q", note["text"], "Patient reports occasional nausea after dosing.")
	}
}

// TestDeclarativeEngine_Medication_OrderIntent_SetsIntentOrder is a
// regression test for a real gap a 747-file sample_ccdas corpus run found:
// medicationRequestFields() never wrote MedicationRequest.intent (1..1
// required in FHIR) at all. medicationRequestRule()'s own EntryMatch
// ("moodCode=INT") already guarantees every matched entry is an order, so
// "order" is the correct literal value for every MedicationRequest this
// rule ever produces.
func TestDeclarativeEngine_Medication_OrderIntent_SetsIntentOrder(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "substanceAdministration",
			MoodCode:   "INT",
			StatusCode: "active",
		},
	}
	resources := buildMedicationResources(t, entries)
	if len(resources) != 1 || resources[0]["resourceType"] != "MedicationRequest" {
		t.Fatalf("expected 1 MedicationRequest, got %v", resources)
	}
	if resources[0]["intent"] != "order" {
		t.Errorf("MedicationRequest.intent = %v, want %q", resources[0]["intent"], "order")
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

// TestDeclarativeEngine_Medication_IdFromEntry_SetsIdentifier proves a real
// production gap found in a 99397 CCD sample (Epic source): every one of
// its 10 medication entries has its own substanceAdministration <id>, but
// neither MedicationRequest nor MedicationStatement ever carried an
// identifier in the output before medicationCommonRows() gained this row.
func TestDeclarativeEngine_Medication_IdFromEntry_SetsIdentifier(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "substanceAdministration",
			MoodCode:   "INT",
			StatusCode: "active",
			Id:         []cdadocument.CDAII{{Root: "1.2.840.114350.1.13.549.2.7.2.798268", Extension: "57530535"}},
		},
	}
	resources := buildMedicationResources(t, entries)
	if len(resources) != 1 {
		t.Fatalf("expected 1 MedicationRequest, got %d", len(resources))
	}
	idents, ok := resources[0]["identifier"].([]interface{})
	if !ok || len(idents) != 1 {
		t.Fatalf("expected 1 identifier, got %v", resources[0]["identifier"])
	}
	ident, ok := idents[0].(map[string]interface{})
	if !ok {
		t.Fatalf("identifier[0] not a map: %v", idents[0])
	}
	if ident["value"] != "57530535" {
		t.Errorf("identifier[0].value = %v, want 57530535", ident["value"])
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
	// period must be a FHIR decimal (JSON number), not the raw CDA attribute
	// string -- a bare string here fails FHIR validation ("the primitive
	// value must be a number").
	if repeat["period"] != float64(12) || repeat["periodUnit"] != "h" {
		t.Errorf("repeat = %v, want period=12 (float64) periodUnit=h", repeat)
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

// TestDeclarativeEngine_Medication_RSONIndication_TextFallbackFromEntryText
// proves a real production gap found in a 99397 CCD sample (Epic source):
// the RSON indication observation's own <value code="67882000"
// codeSystem="2.16.840.1.113883.6.96".../> carried no displayName/
// originalText at all (neither did the entry's own <code>), so
// CDAValueToFHIR's CodeableConcept came back as a bare code with no .text
// -- but the SAME entry's own <text>Vulvar itching<reference
// value="#indication10"/></text> resolves to "Vulvar itching" via the
// standard "#id" reference mechanism (cda/document/section_parser.go),
// sitting unused. declarativeValueOrCodeToCodeableConcept now falls back to
// entry.Text when the value/code branch produced no .text of its own.
func TestDeclarativeEngine_Medication_RSONIndication_TextFallbackFromEntryText(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "substanceAdministration",
			MoodCode:   "INT",
			StatusCode: "active",
			EntryRelationships: []cdadocument.CDAEntryRelationship{
				{
					TypeCode: "RSON",
					Entry: cdadocument.CDAEntry{
						EntryType: "observation",
						Text:      "Vulvar itching", // simulates the post-resolution state; section_parser.go's own resolution is tested separately.
						Value: &cdadocument.CDAValue{
							Type: "CD",
							Code: &cdadocument.CDACode{Code: "67882000", CodeSystem: "2.16.840.1.113883.6.96"}, // no DisplayName/OriginalText, matching the real source.
						},
					},
				},
			},
		},
	}
	resources := buildMedicationResources(t, entries)
	if len(resources) != 1 {
		t.Fatalf("expected 1 MedicationRequest, got %d", len(resources))
	}
	reasons, ok := resources[0]["reasonCode"].([]interface{})
	if !ok || len(reasons) != 1 {
		t.Fatalf("expected 1 reasonCode from the RSON indication, got %v", resources[0]["reasonCode"])
	}
	cc, ok := reasons[0].(map[string]interface{})
	if !ok {
		t.Fatalf("reasonCode[0] not a CodeableConcept-shaped map: %v", reasons[0])
	}
	if cc["text"] != "Vulvar itching" {
		t.Errorf("reasonCode[0].text = %v, want %q (fallback from entry.Text)", cc["text"], "Vulvar itching")
	}
	coding := firstCoding(t, cc)
	if coding["code"] != "67882000" {
		t.Errorf("coding[0].code = %v, want 67882000 unchanged", coding["code"])
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

func TestDeclarativeEngine_MedicationStatement_EffectivePeriod_SuppressesEffectiveDateTime(t *testing.T) {
	// medication_mapper.go:121-124's period-preferred-else-onset shape. Note:
	// given transforms.CDATimeRangeToPeriod/CDATimeRangeToOnset's actual
	// implementations, both are gated by the exact same three fields
	// (Low/High/Value) being empty -- so the "else" branch is structurally
	// unreachable in Go today too (proven, not assumed: if Period returns
	// nil, Onset necessarily returns "" for the same input). This row is
	// ported for byte-for-byte fidelity and as a defensive guard against a
	// future transforms.go change, not because a real input exercises it.
	// What IS verifiable here is the skip itself: a period-bearing entry
	// must never also get effectiveDateTime.
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "substanceAdministration",
			MoodCode:   "EVN",
			StatusCode: "completed",
			EffectiveTime: cdadocument.CDATimeRange{
				Low: cdadocument.CDATime{Value: "20230101"},
			},
		},
	}
	resources := buildMedicationResources(t, entries)
	if len(resources) != 1 || resources[0]["resourceType"] != "MedicationStatement" {
		t.Fatalf("expected 1 MedicationStatement, got %v", resources)
	}
	if _, has := resources[0]["effectivePeriod"]; !has {
		t.Fatal("expected effectivePeriod to be set")
	}
	if _, has := resources[0]["effectiveDateTime"]; has {
		t.Error("effectiveDateTime must not be set when effectivePeriod is already present (SkipIfResourceHasAnyOf)")
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

// TestDeclarativeEngine_Condition_RecorderFromProblemObservationAuthor proves
// the Problem Observation's own <author> (entry.Authors, parsed generically
// for every CDA entry by entry_parser.go, distinct from header.Authors)
// reaches Condition.recorder. Per the HL7 C-CDA on FHIR IG's Problems mapping
// page: "/entryRelationship[@typeCode='SUBJ']/observation/author" ->
// ".recorder" -- the nested observation's author, not the outer Concern
// Act's, which is why Authors is set on the SUBJ-nested entry below, not the
// outer act.
func TestDeclarativeEngine_Condition_RecorderFromProblemObservationAuthor(t *testing.T) {
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
						Authors: []cdadocument.CDAAuthor{
							{
								AssignedAuthor: cdadocument.CDAAssignedAuthor{
									Ids: []cdadocument.CDAII{{Root: "1.2.840.114350.1.13.549.2.7.1.1133", Extension: "534704989"}},
									AssignedPerson: &cdadocument.CDAPerson{
										Names: []cdadocument.CDAName{{Given: []string{"Chloe"}, Family: "Herbst"}},
									},
									RepresentedOrganization: &cdadocument.CDAOrganization{
										Names: []string{"Mumbai Community Health and Affiliates"},
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

	// assignedEntityRoleRow emits PractitionerRole+Practitioner+Organization
	// alongside Condition itself (same multi-resource shape
	// TestDeclarativeEngine_CareTeam_BuildsCareTeamAndPractitioner already
	// exercises for an identical EmitAsResource nesting) -- find each by
	// resourceType rather than asserting a single-element slice.
	var condition, practitionerRole, practitioner, organization map[string]interface{}
	for _, r := range resources {
		switch r["resourceType"] {
		case "Condition":
			condition = r
		case "PractitionerRole":
			practitionerRole = r
		case "Practitioner":
			practitioner = r
		case "Organization":
			organization = r
		}
	}
	if condition == nil {
		t.Fatalf("expected a Condition resource, got %d resources: %v", len(resources), resources)
	}
	if practitioner == nil {
		t.Fatal("expected a Practitioner resource built from the Problem Observation's author")
	}
	if organization == nil {
		t.Fatal("expected an Organization resource built from the author's representedOrganization")
	}
	if practitionerRole == nil {
		t.Fatal("expected a PractitionerRole resource linking the Practitioner and Organization")
	}

	recorderRef, ok := condition["recorder"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected Condition.recorder to be set, got %v", condition["recorder"])
	}
	wantRef := "PractitionerRole/" + practitionerRole["id"].(string)
	if recorderRef["reference"] != wantRef {
		t.Errorf("Condition.recorder.reference = %v, want %v", recorderRef["reference"], wantRef)
	}

	name := firstElement(t, practitioner["name"]).(map[string]interface{})
	if name["family"] != "Herbst" {
		t.Errorf("Practitioner.name[0].family = %v, want Herbst", name["family"])
	}
	if organization["name"] != "Mumbai Community Health and Affiliates" {
		t.Errorf("Organization.name = %v, want %q", organization["name"], "Mumbai Community Health and Affiliates")
	}
}

// TestDeclarativeEngine_Condition_RecorderFallback_BarePractitioner_NoOrganization
// proves a real production gap: a 99397 CCD sample (Epic source) has, on
// every one of its 5 Active Problems' Problem Observation <author>, an NPI
// id and NO representedOrganization at all -- assignedEntityRoleRow's own
// EmitAsResourceRequiredPaths=["organization"] gate discarded the WHOLE
// recorder reference (PractitionerRole and its nested Practitioner) for
// all 5, with no fallback before barePractitionerRow existed. This fixture
// mirrors that exact shape (NPI present, RepresentedOrganization absent).
func TestDeclarativeEngine_Condition_RecorderFallback_BarePractitioner_NoOrganization(t *testing.T) {
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
							Code: &cdadocument.CDACode{Code: "40930008", DisplayName: "Hypothyroidism", CodeSystem: "2.16.840.1.113883.6.96"},
						},
						Authors: []cdadocument.CDAAuthor{
							{
								AssignedAuthor: cdadocument.CDAAssignedAuthor{
									Ids: []cdadocument.CDAII{{Root: "2.16.840.1.113883.4.6", Extension: "1013027903"}},
									AssignedPerson: &cdadocument.CDAPerson{
										Names: []cdadocument.CDAName{{Given: []string{"Herman"}, Family: "Damek"}},
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

	var condition, practitioner map[string]interface{}
	for _, r := range resources {
		switch r["resourceType"] {
		case "Condition":
			condition = r
		case "Practitioner":
			practitioner = r
		case "PractitionerRole":
			t.Fatalf("expected NO PractitionerRole (no organization data) -- got one: %v", r)
		}
	}
	if condition == nil {
		t.Fatalf("expected a Condition resource, got %d resources: %v", len(resources), resources)
	}
	if practitioner == nil {
		t.Fatal("expected a bare Practitioner resource built from the author despite no representedOrganization")
	}

	recorderRef, ok := condition["recorder"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected Condition.recorder to be set via the fallback tier, got %v", condition["recorder"])
	}
	wantRef := "Practitioner/" + practitioner["id"].(string)
	if recorderRef["reference"] != wantRef {
		t.Errorf("Condition.recorder.reference = %v, want %v", recorderRef["reference"], wantRef)
	}
	ident := firstElement(t, practitioner["identifier"]).(map[string]interface{})
	if ident["value"] != "1013027903" {
		t.Errorf("Practitioner.identifier[0].value = %v, want the NPI 1013027903", ident["value"])
	}
}

// TestDeclarativeEngine_Condition_RecordedDate_FromAuthorTime proves the
// Condition.recordedDate mapping HL7's C-CDA on FHIR IG specifies
// (CF-problems.html: "/author/time -> .recordedDate") but this codebase
// never implemented at all, despite every one of the 99397 sample's 5
// Active Problems carrying a real <author><time .../></author>.
func TestDeclarativeEngine_Condition_RecordedDate_FromAuthorTime(t *testing.T) {
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
							Code: &cdadocument.CDACode{Code: "40930008", DisplayName: "Hypothyroidism", CodeSystem: "2.16.840.1.113883.6.96"},
						},
						Authors: []cdadocument.CDAAuthor{
							{
								Time:           cdadocument.CDATime{Value: "20240109124916-0700"},
								AssignedAuthor: cdadocument.CDAAssignedAuthor{},
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
	recordedDate, _ := resources[0]["recordedDate"].(string)
	if recordedDate == "" {
		t.Fatal("expected Condition.recordedDate to be set from the Problem Observation author's <time>")
	}
}

// TestDeclarativeEngine_Condition_NoteActivity_SetsNote proves the
// Condition.note mapping for C-CDA's Note Activity (templateId
// 2.16.840.1.113883.10.20.22.4.202, code 34109-9 "Note"), attached to the
// Concern Act (not the Problem Observation) via entryRelationship
// typeCode=COMP -- a real gap found in the 99397 sample, where 4 of its 5
// Active Problems carry a substantive provider overview note never
// captured anywhere before this row existed.
func TestDeclarativeEngine_Condition_NoteActivity_SetsNote(t *testing.T) {
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
							Code: &cdadocument.CDACode{Code: "40930008", DisplayName: "Hypothyroidism", CodeSystem: "2.16.840.1.113883.6.96"},
						},
					},
				},
				{
					TypeCode: "COMP",
					Entry: cdadocument.CDAEntry{
						EntryType:   "act",
						TemplateIds: []string{"2.16.840.1.113883.10.20.22.4.202"},
						Text:        "last labs looked good, refilled current dosing.",
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
	notes, ok := resources[0]["note"].([]interface{})
	if !ok || len(notes) != 1 {
		t.Fatalf("expected 1 note, got %v", resources[0]["note"])
	}
	note, ok := notes[0].(map[string]interface{})
	if !ok {
		t.Fatalf("note[0] not a map: %v", notes[0])
	}
	if note["text"] != "last labs looked good, refilled current dosing." {
		t.Errorf("note[0].text = %v, want %q", note["text"], "last labs looked good, refilled current dosing.")
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
	// [1] is conditionRule — [0] is the new assessment scale Observation rule.
	resources, errs := engine.BuildResources(documentMap, cdafhir.HealthConcernsMappingRules()[1])
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

func TestDeclarativeEngine_VitalSigns_Performer_OrganizationOnly_EmitsPractitionerRole(t *testing.T) {
	// Real gap found auditing the 99397 sample's Vital Signs section
	// (LOINC 8716-3): every author there has a representedOrganization
	// with a real name + address + id and NO assignedPerson. The old row
	// (cda_name_or_literal_to_display_ref) produced only a bare display
	// string, discarding the organization's identifier/address data.
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "observation",
			StatusCode: "completed",
			Code:       cdadocument.CDACode{Code: "8302-2", CodeSystem: "2.16.840.1.113883.6.1", DisplayName: "Body height"},
			Value:      &cdadocument.CDAValue{Type: "PQ", Quantity: &cdadocument.CDAQuantity{Value: "162.6", Unit: "cm"}},
			Authors: []cdadocument.CDAAuthor{
				{
					AssignedAuthor: cdadocument.CDAAssignedAuthor{
						RepresentedOrganization: &cdadocument.CDAOrganization{
							Ids:   []cdadocument.CDAII{{Root: "1.2.840.114350.1.13.549.2.7.2.688879", Extension: "41800"}},
							Names: []string{"mumbai Community Health and Affiliates"},
						},
					},
				},
			},
		},
	}
	documentMap := documentMapForEntries(t, "vitalSigns", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.VitalSignsMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 3 {
		t.Fatalf("expected 3 resources (Observation + emitted PractitionerRole + emitted Organization), got %d: %+v", len(resources), resources)
	}

	var obs, role, org map[string]interface{}
	for _, r := range resources {
		switch r["resourceType"] {
		case "Observation":
			obs = r
		case "PractitionerRole":
			role = r
		case "Organization":
			org = r
		}
	}
	if obs == nil {
		t.Fatalf("expected an Observation resource, got: %+v", resources)
	}
	if org == nil {
		t.Fatalf("expected an emitted Organization, got: %+v", resources)
	}
	if org["name"] != "mumbai Community Health and Affiliates" {
		t.Errorf("organization.name = %v, want mumbai Community Health and Affiliates", org["name"])
	}
	if role == nil {
		t.Fatalf("expected an emitted PractitionerRole (organization-only degrade), got: %+v", resources)
	}
	if _, hasPractitioner := role["practitioner"]; hasPractitioner {
		t.Errorf("expected NO practitioner field on an organization-only PractitionerRole, got %v", role["practitioner"])
	}
	orgRef, _ := role["organization"].(map[string]interface{})
	wantOrgRef := "Organization/" + org["id"].(string)
	if orgRef["reference"] != wantOrgRef {
		t.Errorf("PractitionerRole.organization.reference = %v, want %v", orgRef, wantOrgRef)
	}

	performer, _ := obs["performer"].([]interface{})
	if len(performer) != 1 {
		t.Fatalf("expected 1 performer, got %v", obs["performer"])
	}
	perfRef, _ := performer[0].(map[string]interface{})
	wantRef := "PractitionerRole/" + role["id"].(string)
	if perfRef["reference"] != wantRef {
		t.Errorf("performer[0].reference = %v, want %v (a real PractitionerRole reference, not a display string)", perfRef, wantRef)
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

func TestDeclarativeEngine_Results_InterpretationCode_DirectChild_SetsInterpretation(t *testing.T) {
	// CONF:1198-7147 -- direct sibling of code/statusCode/value, matching
	// real Kareo corpus data's actual shape (not COMP-nested, the prior
	// IG-incorrect assumption this row replaces).
	entries := []cdadocument.CDAEntry{
		{
			EntryType:          "observation",
			StatusCode:         "completed",
			Code:               cdadocument.CDACode{Code: "2345-7", CodeSystem: "2.16.840.1.113883.6.1"},
			Value:              &cdadocument.CDAValue{Type: "PQ", Quantity: &cdadocument.CDAQuantity{Value: "180", Unit: "mg/dL"}},
			InterpretationCode: cdadocument.CDACode{Code: "H", CodeSystem: "2.16.840.1.113883.5.83", DisplayName: "High"},
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
	interp, _ := resources[0]["interpretation"].([]interface{})
	if len(interp) != 1 {
		t.Fatalf("expected 1 interpretation entry, got %d", len(interp))
	}
	coding := firstCoding(t, interp[0])
	if coding["code"] != "H" {
		t.Errorf("interpretation[0].coding[0].code = %v, want H", coding["code"])
	}
}

func TestDeclarativeEngine_Results_ReferenceRangeText_SetsReferenceRange(t *testing.T) {
	// Real gap found auditing Results (Ascension Wisconsin sample,
	// CCD_06_24_20267.xml): every one of its 46 lab Observations carries a
	// real free-text reference range (e.g. "4.0 - 10.0 Thou/uL") via
	// <referenceRange><observationRange><text>, parsed nowhere before this.
	entries := []cdadocument.CDAEntry{
		{
			EntryType:          "observation",
			StatusCode:         "completed",
			Code:               cdadocument.CDACode{Code: "6690-2", CodeSystem: "2.16.840.1.113883.6.1", DisplayName: "WBC"},
			Value:              &cdadocument.CDAValue{Type: "PQ", Quantity: &cdadocument.CDAQuantity{Value: "1.5", Unit: "Thou/uL"}},
			InterpretationCode: cdadocument.CDACode{Code: "L"},
			ReferenceRangeText: "4.0 - 10.0 Thou/uL",
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
	rr, _ := resources[0]["referenceRange"].([]interface{})
	if len(rr) != 1 {
		t.Fatalf("expected 1 referenceRange entry, got %d (%v)", len(rr), resources[0]["referenceRange"])
	}
	rrMap, _ := rr[0].(map[string]interface{})
	if rrMap["text"] != "4.0 - 10.0 Thou/uL" {
		t.Errorf("referenceRange[0].text = %v, want %q", rrMap["text"], "4.0 - 10.0 Thou/uL")
	}
}

func TestDeclarativeEngine_Results_NullFlavorCodeWithOriginalText_StillEmitted(t *testing.T) {
	// Real gap found auditing Results (Marshfield sample, CCD_05_15_2026.xml):
	// a full cytology/pathology report (real diagnosis text, signing
	// pathologist, date) has code.nullFlavor="UNK" with no LOINC code, but a
	// real <originalText> label -- was being discarded entirely by
	// SkipIfCodeNullFlavor before this fix. Confirmed via a real engine run
	// that this exact shape produced 0 Observations from that file's Results
	// section.
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "observation",
			StatusCode: "completed",
			Code: cdadocument.CDACode{
				NullFlavor:   "UNK",
				OriginalText: "Thin Prep, PAP and HPV if ASCUS",
			},
			Value: &cdadocument.CDAValue{Type: "ST", Text: "Heading: CYTOLOGY REPORT..."},
		},
	}
	documentMap := documentMapForEntries(t, "results", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.ResultsMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 Observation to survive (real content, just no LOINC code), got %d", len(resources))
	}
	code, _ := resources[0]["code"].(map[string]interface{})
	if code["text"] != "Thin Prep, PAP and HPV if ASCUS" {
		t.Errorf("code.text = %v, want the originalText label", code["text"])
	}
}

func TestDeclarativeEngine_Results_NullFlavorCodeNoOriginalText_StillDiscarded(t *testing.T) {
	// Regression guard for the fix above: a genuinely-empty placeholder shell
	// (real shape: Dignity Health's "No data available for this section"
	// Results entry -- bare <code nullFlavor="NI"/>, no originalText at all)
	// must still be discarded, same as the Functional Status .4.69
	// Assessment Scale shells this guard was originally written for (also a
	// bare <code nullFlavor="UNK"/> with no originalText).
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "observation",
			StatusCode: "completed",
			Code:       cdadocument.CDACode{NullFlavor: "NI"},
			Value:      &cdadocument.CDAValue{NullFlavor: "NI"},
		},
	}
	documentMap := documentMapForEntries(t, "results", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.ResultsMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 0 {
		t.Fatalf("expected the code-less, content-less shell to still be discarded, got %d resources", len(resources))
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

// TestDeclarativeEngine_SocialHistory_SDOH_AssessmentScaleFanOut mirrors the
// 99397 sample's real HARK domestic-violence screening shape: a Social
// History Observation shell (templateId .4.38, generic code "8689-2") with
// NO value of its own, SPRT-nesting an Assessment Scale Observation
// (templateId .4.69, "HARK questionnaire", interpretationCode "Low Risk")
// which COMP-nests two Assessment Scale Supporting Observations (templateId
// .4.86, the actual question/answer pairs with their own author). Before
// this fix, only the shell was read -- 14 of 24 real Social History entries
// in that document produced an identical, valueless Observation each. This
// proves all three tiers now emit as independent Observation resources
// linked via hasMember[], per the spec-conformant SPRT/COMP nesting
// confirmed against HL7's C-CDA on FHIR IG and the local CDAR2_IG_CCDA_CLINNOTES
// spec PDF.
//
// The first supporting observation's author (person name, no
// representedOrganization) also exercises the performer tiered upgrade
// applied alongside the Vital Signs performer fix: barePractitionerRow's
// fallback tier fires (assignedEntityRoleRow's own organization-required
// gate has nothing to match), emitting a real Practitioner sub-resource
// instead of the old bare display string -- hence 5 resources, not 4.
func TestDeclarativeEngine_SocialHistory_SDOH_AssessmentScaleFanOut(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "observation",
			StatusCode: "completed",
			Code:       cdadocument.CDACode{Code: "8689-2", DisplayName: "History of Social function", CodeSystem: "2.16.840.1.113883.6.1"},
			EntryRelationships: []cdadocument.CDAEntryRelationship{
				{
					TypeCode: "SPRT",
					Entry: cdadocument.CDAEntry{
						EntryType:         "observation",
						TemplateIds:       []string{"2.16.840.1.113883.10.20.22.4.69"},
						StatusCode:        "completed",
						Code:              cdadocument.CDACode{Code: "76499-3", DisplayName: "Humiliation, Afraid, Rape, and Kick questionnaire [HARK]"},
						Value:             &cdadocument.CDAValue{Type: "CD", Code: &cdadocument.CDACode{NullFlavor: "UNK"}},
						InterpretationCode: cdadocument.CDACode{NullFlavor: "OTH", OriginalText: "Not At Risk"},
						EntryRelationships: []cdadocument.CDAEntryRelationship{
							{
								TypeCode: "COMP",
								Entry: cdadocument.CDAEntry{
									EntryType:   "observation",
									TemplateIds: []string{"2.16.840.1.113883.10.20.22.4.86"},
									StatusCode:  "completed",
									Code:        cdadocument.CDACode{Code: "76501-6", DisplayName: "Within the last year, have you been afraid of your partner or ex-partner?"},
									Value:       &cdadocument.CDAValue{Type: "CD", Code: &cdadocument.CDACode{Code: "LA32-8", DisplayName: "No", CodeSystem: "2.16.840.1.113883.6.1"}},
									Authors: []cdadocument.CDAAuthor{
										{AssignedAuthor: cdadocument.CDAAssignedAuthor{AssignedPerson: &cdadocument.CDAPerson{Names: []cdadocument.CDAName{{Given: []string{"Generic"}, Family: "Mychart"}}}}},
									},
								},
							},
							{
								TypeCode: "COMP",
								Entry: cdadocument.CDAEntry{
									EntryType:   "observation",
									TemplateIds: []string{"2.16.840.1.113883.10.20.22.4.86"},
									StatusCode:  "completed",
									Code:        cdadocument.CDACode{Code: "76500-8", DisplayName: "Within the last year, have you been humiliated or emotionally abused in other ways by your partner or ex-partner?"},
									Value:       &cdadocument.CDAValue{Type: "CD", Code: &cdadocument.CDACode{Code: "LA32-8", DisplayName: "No", CodeSystem: "2.16.840.1.113883.6.1"}},
								},
							},
						},
					},
				},
			},
		},
	}
	documentMap := documentMapForEntries(t, "socialHistory", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.SocialHistoryMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 5 {
		t.Fatalf("expected 5 resources (shell + assessment scale + 2 supporting + 1 emitted Practitioner), got %d: %+v", len(resources), resources)
	}

	var practitioners []map[string]interface{}
	for _, r := range resources {
		if r["resourceType"] == "Practitioner" {
			practitioners = append(practitioners, r)
		}
	}
	if len(practitioners) != 1 {
		t.Fatalf("expected exactly 1 emitted Practitioner (from the first supporting observation's author), got %d: %+v", len(practitioners), practitioners)
	}

	// Identify Observation resources by their code's coding[0].code --
	// robust regardless of emission order.
	byCode := map[string]map[string]interface{}{}
	for _, r := range resources {
		if r["resourceType"] != "Observation" {
			continue
		}
		cc, _ := r["code"].(map[string]interface{})
		coding, _ := cc["coding"].([]interface{})
		if len(coding) == 0 {
			continue
		}
		first, _ := coding[0].(map[string]interface{})
		code, _ := first["code"].(string)
		byCode[code] = r
	}

	shellObs, ok := byCode["8689-2"]
	if !ok {
		t.Fatalf("expected the shell Observation (code 8689-2), got resources: %+v", resources)
	}
	shellMembers, ok := shellObs["hasMember"].([]interface{})
	if !ok || len(shellMembers) != 1 {
		t.Fatalf("shell hasMember = %v, want a 1-element array", shellObs["hasMember"])
	}

	scaleObs, ok := byCode["76499-3"]
	if !ok {
		t.Fatalf("expected the Assessment Scale Observation (code 76499-3, HARK questionnaire), got resources: %+v", resources)
	}
	wantShellRef := "Observation/" + scaleObs["id"].(string)
	shellRef, _ := shellMembers[0].(map[string]interface{})
	if shellRef["reference"] != wantShellRef {
		t.Errorf("shell hasMember[0].reference = %v, want %v", shellRef["reference"], wantShellRef)
	}
	interp, _ := scaleObs["interpretation"].([]interface{})
	if len(interp) == 0 {
		t.Error("expected Assessment Scale Observation.interpretation to be set from interpretationCode")
	}
	if _, hasValue := scaleObs["valueCodeableConcept"]; hasValue {
		t.Errorf("expected NO valueCodeableConcept (source value is nullFlavor=UNK), got %v", scaleObs["valueCodeableConcept"])
	}

	scaleMembers, ok := scaleObs["hasMember"].([]interface{})
	if !ok || len(scaleMembers) != 2 {
		t.Fatalf("Assessment Scale Observation hasMember = %v, want a 2-element array", scaleObs["hasMember"])
	}

	q1, ok := byCode["76501-6"]
	if !ok {
		t.Fatalf("expected the first Assessment Scale Supporting Observation (code 76501-6), got resources: %+v", resources)
	}
	answer, _ := q1["valueCodeableConcept"].(map[string]interface{})
	answerCoding := firstCoding(t, answer)
	if answerCoding["code"] != "LA32-8" {
		t.Errorf("first question's answer coding = %v, want code=LA32-8", answerCoding)
	}
	performer, _ := q1["performer"].([]interface{})
	if len(performer) == 0 {
		t.Fatal("expected the first Assessment Scale Supporting Observation to have a performer from its author")
	}
	perfRef, _ := performer[0].(map[string]interface{})
	wantPerfRef := "Practitioner/" + practitioners[0]["id"].(string)
	if perfRef["reference"] != wantPerfRef {
		t.Errorf("first question's performer[0].reference = %v, want %v (a real Practitioner reference, not a display string)", perfRef, wantPerfRef)
	}

	q2, ok := byCode["76500-8"]
	if !ok {
		t.Fatalf("expected the second Assessment Scale Supporting Observation (code 76500-8), got resources: %+v", resources)
	}
	if q2["category"] == nil {
		t.Error("expected the second question's category to be set")
	}

	wantScaleMember0 := "Observation/" + q1["id"].(string)
	wantScaleMember1 := "Observation/" + q2["id"].(string)
	gotRefs := map[string]bool{}
	for _, m := range scaleMembers {
		ref, _ := m.(map[string]interface{})
		gotRefs[ref["reference"].(string)] = true
	}
	if !gotRefs[wantScaleMember0] || !gotRefs[wantScaleMember1] {
		t.Errorf("Assessment Scale Observation hasMember refs = %v, want both %v and %v", scaleMembers, wantScaleMember0, wantScaleMember1)
	}
}

// ---- Functional Status / Mental Status / labResults alias ----
//
// Phase 4 Slice D gap closure: these 3 had no declarative rule at all before
// this session (see FunctionalStatusMappingRules'/MentalStatusMappingRules'/
// ResultsMappingRules' own doc comments). No corpus file has either section
// — synthetic data only, same convention used elsewhere in this file for
// sections the 4-vendor corpus doesn't exercise.

func TestDeclarativeEngine_FunctionalStatus_UsesUSCoreCategorySystem(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "observation",
			StatusCode: "completed",
			Code:       cdadocument.CDACode{Code: "54521-1", CodeSystem: "2.16.840.1.113883.6.1", DisplayName: "Functional status"},
			Value: &cdadocument.CDAValue{
				Type: "CD",
				Code: &cdadocument.CDACode{Code: "445528004", DisplayName: "Independent", CodeSystem: "2.16.840.1.113883.6.96"},
			},
		},
	}
	documentMap := documentMapForEntries(t, "functionalStatus", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.FunctionalStatusMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 Observation, got %d", len(resources))
	}
	cats, _ := resources[0]["category"].([]interface{})
	catCoding := firstCoding(t, cats[0])
	if catCoding["code"] != "functional-status" {
		t.Errorf("category coding = %v, want code=functional-status", catCoding)
	}
	if catCoding["system"] != "http://hl7.org/fhir/us/core/CodeSystem/us-core-category" {
		t.Errorf("category coding system = %v, want the US Core category system", catCoding["system"])
	}
	if catCoding["display"] != "Functional Status" {
		t.Errorf("category coding display = %v, want 'Functional Status'", catCoding["display"])
	}
}

// TestDeclarativeEngine_FunctionalStatus_CodeTextFromNarrativeOverridesGenericCode
// proves observationRule()'s code.text row fixes a real production gap:
// some EHR vendors (Epic, this session's 99397 CCD corpus) repeat one
// generic LOINC code (54522-8 "Functional status") across many unrelated
// Functional Status entries, so cda_code_to_codeable_concept's own code.text
// (the code's DisplayName) gives no way to tell entries apart. entry.Text
// (resolved by cda/document/section_parser.go's resolveEntryRefs/walkForIDs
// to exactly the text at the referenced "#id" -- the standard, reliable
// part of CDA narrative reference resolution, no inference about narrative
// structure layered on top, deliberately, since that structure isn't
// standardized across vendors) is set here directly to simulate a
// successful resolution; section_parser.go's own resolution mechanics are
// tested separately in that package. Without this row, every such
// Observation's code.text reads "Functional status" regardless of what the
// value actually means.
func TestDeclarativeEngine_FunctionalStatus_CodeTextFromNarrativeOverridesGenericCode(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "observation",
			StatusCode: "completed",
			Code:       cdadocument.CDACode{Code: "54522-8", CodeSystem: "2.16.840.1.113883.6.1", DisplayName: "Functional status"},
			Text:       "Height", // simulates the post-resolution state; section_parser.go's own resolution is tested separately.
			Value: &cdadocument.CDAValue{
				Type: "ST",
				Text: "64",
			},
		},
	}
	documentMap := documentMapForEntries(t, "functionalStatus", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.FunctionalStatusMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 Observation, got %d", len(resources))
	}
	code, ok := resources[0]["code"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected Observation.code to be set, got %v", resources[0]["code"])
	}
	if code["text"] != "Height" {
		t.Errorf("code.text = %v, want %q (the resolved narrative reference, not the generic LOINC display)", code["text"], "Height")
	}
	coding := firstCoding(t, code)
	if coding["code"] != "54522-8" {
		t.Errorf("coding[0].code = %v, want 54522-8 unchanged (only .text should be overridden)", coding["code"])
	}
	if resources[0]["valueString"] != "64" {
		t.Errorf("valueString = %v, want 64", resources[0]["valueString"])
	}
}

// TestDeclarativeEngine_FunctionalStatus_NoNarrativeText_KeepsCodeDisplay
// proves the new row is a no-op (never blanks code.text) when entry.Text is
// empty -- declarative_engine.go's applyRow skips writing anything when
// SourcePath resolves to nothing.
func TestDeclarativeEngine_FunctionalStatus_NoNarrativeText_KeepsCodeDisplay(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "observation",
			StatusCode: "completed",
			Code:       cdadocument.CDACode{Code: "75626-2", CodeSystem: "2.16.840.1.113883.6.1", DisplayName: "Total score [AUDIT-C]"},
			Value: &cdadocument.CDAValue{
				Type: "INT",
				Integer: func() *int { v := 1; return &v }(),
			},
		},
	}
	documentMap := documentMapForEntries(t, "functionalStatus", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.FunctionalStatusMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 Observation, got %d", len(resources))
	}
	code, ok := resources[0]["code"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected Observation.code to be set, got %v", resources[0]["code"])
	}
	if code["text"] != "Total score [AUDIT-C]" {
		t.Errorf("code.text = %v, want the original display unchanged (no narrative text to override with)", code["text"])
	}
}

// TestDeclarativeEngine_FunctionalStatus_AssessmentScaleCompChildren_SurviveNullFlavorShell
// reproduces the real gap found auditing the 99397 sample's Functional
// Status section: its "Alcohol Use" entry is an Assessment Scale Observation
// (templateId .4.69) sitting DIRECTLY as a top-level section entry (unlike
// Social History, where this template nests one level deeper under a
// SPRT-linked shell) with code.nullFlavor="UNK" (non-conformant -- the IG
// requires code+value both [1..1] here) and THREE COMP-nested Assessment
// Scale Supporting Observations (templateId .4.86, AUDIT-C Q1/Q2/Q3) that
// each carry a real LOINC code + value. Before the fix (buildOneResource
// checking SkipIfCodeNullFlavor before Fields ran at all), the entire entry
// -- shell AND all 3 real question/answer children -- was silently
// discarded. confirmed in the actual parsed FHIR output, which had zero
// trace of any AUDIT-C question.
func TestDeclarativeEngine_FunctionalStatus_AssessmentScaleCompChildren_SurviveNullFlavorShell(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "observation",
			StatusCode: "completed",
			TemplateIds: []string{"2.16.840.1.113883.10.20.22.4.69"},
			Code:       cdadocument.CDACode{NullFlavor: "UNK"},
			Value:      &cdadocument.CDAValue{Type: "CD", Code: &cdadocument.CDACode{NullFlavor: "UNK"}},
			EntryRelationships: []cdadocument.CDAEntryRelationship{
				{
					TypeCode: "COMP",
					Entry: cdadocument.CDAEntry{
						EntryType:   "observation",
						TemplateIds: []string{"2.16.840.1.113883.10.20.22.4.86"},
						StatusCode:  "completed",
						Code:        cdadocument.CDACode{Code: "68518-0", DisplayName: "How often do you have a drink containing alcohol?", CodeSystem: "2.16.840.1.113883.6.1"},
						Value:       &cdadocument.CDAValue{Type: "CD", Code: &cdadocument.CDACode{Code: "LA18926-8", DisplayName: "Monthly or less", CodeSystem: "2.16.840.1.113883.6.1"}},
					},
				},
				{
					TypeCode: "COMP",
					Entry: cdadocument.CDAEntry{
						EntryType:   "observation",
						TemplateIds: []string{"2.16.840.1.113883.10.20.22.4.86"},
						StatusCode:  "completed",
						Code:        cdadocument.CDACode{Code: "68519-8", DisplayName: "How many standard drinks containing alcohol do you have on a typical day?", CodeSystem: "2.16.840.1.113883.6.1"},
						Value:       &cdadocument.CDAValue{Type: "CD", Code: &cdadocument.CDACode{Code: "LA15694-5", DisplayName: "1 or 2", CodeSystem: "2.16.840.1.113883.6.1"}},
					},
				},
				{
					TypeCode: "COMP",
					Entry: cdadocument.CDAEntry{
						EntryType:   "observation",
						TemplateIds: []string{"2.16.840.1.113883.10.20.22.4.86"},
						StatusCode:  "completed",
						Code:        cdadocument.CDACode{Code: "68520-6", DisplayName: "How often do you have 6 or more drinks on 1 occasion?", CodeSystem: "2.16.840.1.113883.6.1"},
						Value:       &cdadocument.CDAValue{Type: "CD", Code: &cdadocument.CDACode{Code: "LA6270-8", DisplayName: "Never", CodeSystem: "2.16.840.1.113883.6.1"}},
					},
				},
			},
		},
		// A sibling Functional Status Observation (.4.67, real fixed code
		// 54522-8) in the same section -- proves the fix doesn't disturb
		// the normal, already-working case alongside the null-coded shell.
		{
			EntryType:  "observation",
			StatusCode: "completed",
			Code:       cdadocument.CDACode{Code: "54522-8", CodeSystem: "2.16.840.1.113883.6.1", DisplayName: "Functional status"},
			Value:      &cdadocument.CDAValue{Type: "ST", Text: "64"},
		},
	}
	documentMap := documentMapForEntries(t, "functionalStatus", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.FunctionalStatusMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	// 3 AUDIT-C question Observations + 1 Functional Status Observation (the
	// sibling). The null-coded "Alcohol Use" shell itself must NOT appear.
	if len(resources) != 4 {
		t.Fatalf("expected 4 resources (3 AUDIT-C questions + 1 sibling Height entry, shell excluded), got %d: %+v", len(resources), resources)
	}

	byCode := map[string]map[string]interface{}{}
	for _, r := range resources {
		if r["resourceType"] != "Observation" {
			t.Errorf("unexpected non-Observation resource: %+v", r)
			continue
		}
		cc, _ := r["code"].(map[string]interface{})
		if cc == nil {
			t.Errorf("expected every surviving Observation to carry a real code (the null-coded shell must be excluded), got %+v", r)
			continue
		}
		coding, _ := cc["coding"].([]interface{})
		if len(coding) == 0 {
			continue
		}
		first, _ := coding[0].(map[string]interface{})
		code, _ := first["code"].(string)
		byCode[code] = r
	}

	for wantCode, wantAnswerCode := range map[string]string{
		"68518-0": "LA18926-8",
		"68519-8": "LA15694-5",
		"68520-6": "LA6270-8",
	} {
		q, ok := byCode[wantCode]
		if !ok {
			t.Fatalf("expected AUDIT-C question Observation code=%s to survive, got resources: %+v", wantCode, resources)
		}
		answer, _ := q["valueCodeableConcept"].(map[string]interface{})
		answerCoding := firstCoding(t, answer)
		if answerCoding["code"] != wantAnswerCode {
			t.Errorf("question %s answer coding = %v, want code=%s", wantCode, answerCoding, wantAnswerCode)
		}
		cats, _ := q["category"].([]interface{})
		if len(cats) == 0 {
			t.Errorf("question %s: expected category to be set to functional-status, got none", wantCode)
		} else {
			catCoding := firstCoding(t, cats[0])
			if catCoding["code"] != "functional-status" {
				t.Errorf("question %s category coding = %v, want code=functional-status (not social-history)", wantCode, catCoding)
			}
		}
	}

	if _, stillThere := byCode[""]; stillThere {
		t.Error("the null-coded 'Alcohol Use' shell observation must not survive as its own resource")
	}
	if _, ok := byCode["54522-8"]; !ok {
		t.Errorf("expected the sibling Functional Status Observation (54522-8) to still be mapped, got resources: %+v", resources)
	}
}

func TestDeclarativeEngine_MentalStatus_UsesUSCoreCategorySystem(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "observation",
			StatusCode: "completed",
			Code:       cdadocument.CDACode{Code: "58144-5", CodeSystem: "2.16.840.1.113883.6.1", DisplayName: "Mental status"},
			Value: &cdadocument.CDAValue{
				Type: "CD",
				Code: &cdadocument.CDACode{Code: "248234008", DisplayName: "Alert", CodeSystem: "2.16.840.1.113883.6.96"},
			},
		},
	}
	documentMap := documentMapForEntries(t, "mentalStatus", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.MentalStatusMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 Observation, got %d", len(resources))
	}
	cats, _ := resources[0]["category"].([]interface{})
	catCoding := firstCoding(t, cats[0])
	if catCoding["code"] != "cognitive-status" {
		t.Errorf("category coding = %v, want code=cognitive-status", catCoding)
	}
	if catCoding["system"] != "http://hl7.org/fhir/us/core/CodeSystem/us-core-category" {
		t.Errorf("category coding system = %v, want the US Core category system", catCoding["system"])
	}
	if catCoding["display"] != "Cognitive Status" {
		t.Errorf("category coding display = %v, want 'Cognitive Status'", catCoding["display"])
	}
}

// TestDeclarativeEngine_MentalStatus_AssessmentScaleCompChildren_SurviveNullFlavorShell
// mirrors TestDeclarativeEngine_FunctionalStatus_AssessmentScaleCompChildren_
// SurviveNullFlavorShell exactly (same templateIds, same COMP nesting), using
// an MMSE-style shape -- the IG's own named example for this template
// (CDAR2_IG_CCDA_CLINNOTES_R1_DSTU2.1_2015AUG_Vol2, Table 232: "Mini-Mental
// Status Exam (assesses cognitive function)"). Unlike the Functional Status
// version, this is NOT reproducing a real corpus finding (no real Mental
// Status sample available this session uses Assessment Scale Observation at
// all) -- it's IG-evidence-only, added after explicit user sign-off; see
// MentalStatusMappingRules' own doc comment for the full IG citation.
func TestDeclarativeEngine_MentalStatus_AssessmentScaleCompChildren_SurviveNullFlavorShell(t *testing.T) {
	q1Answer := 5
	q2Answer := 4
	entries := []cdadocument.CDAEntry{
		{
			EntryType:   "observation",
			StatusCode:  "completed",
			TemplateIds: []string{"2.16.840.1.113883.10.20.22.4.69"},
			Code:        cdadocument.CDACode{NullFlavor: "UNK"},
			Value:       &cdadocument.CDAValue{Type: "CD", Code: &cdadocument.CDACode{NullFlavor: "UNK"}},
			EntryRelationships: []cdadocument.CDAEntryRelationship{
				{
					TypeCode: "COMP",
					Entry: cdadocument.CDAEntry{
						EntryType:   "observation",
						TemplateIds: []string{"2.16.840.1.113883.10.20.22.4.86"},
						StatusCode:  "completed",
						Code:        cdadocument.CDACode{Code: "71492-3", DisplayName: "What is the year, season, date, day, month? [MMSE]", CodeSystem: "2.16.840.1.113883.6.1"},
						Value:       &cdadocument.CDAValue{Type: "INT", Integer: &q1Answer},
					},
				},
				{
					TypeCode: "COMP",
					Entry: cdadocument.CDAEntry{
						EntryType:   "observation",
						TemplateIds: []string{"2.16.840.1.113883.10.20.22.4.86"},
						StatusCode:  "completed",
						Code:        cdadocument.CDACode{Code: "71493-1", DisplayName: "What is the state, county, town, hospital, floor? [MMSE]", CodeSystem: "2.16.840.1.113883.6.1"},
						Value:       &cdadocument.CDAValue{Type: "INT", Integer: &q2Answer},
					},
				},
			},
		},
		// A sibling plain Mental Status Observation (.4.74) in the same
		// section -- proves the fix doesn't disturb the normal,
		// already-working case alongside the null-coded shell.
		{
			EntryType:  "observation",
			StatusCode: "completed",
			Code:       cdadocument.CDACode{Code: "58144-5", CodeSystem: "2.16.840.1.113883.6.1", DisplayName: "Mental status"},
			Value:      &cdadocument.CDAValue{Type: "CD", Code: &cdadocument.CDACode{Code: "248234008", DisplayName: "Alert", CodeSystem: "2.16.840.1.113883.6.96"}},
		},
	}
	documentMap := documentMapForEntries(t, "mentalStatus", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.MentalStatusMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	// 2 MMSE question Observations + 1 sibling Mental Status Observation.
	// The null-coded shell itself must NOT appear.
	if len(resources) != 3 {
		t.Fatalf("expected 3 resources (2 MMSE questions + 1 sibling, shell excluded), got %d: %+v", len(resources), resources)
	}

	byCode := map[string]map[string]interface{}{}
	for _, r := range resources {
		if r["resourceType"] != "Observation" {
			t.Errorf("unexpected non-Observation resource: %+v", r)
			continue
		}
		cc, _ := r["code"].(map[string]interface{})
		if cc == nil {
			t.Errorf("expected every surviving Observation to carry a real code (the null-coded shell must be excluded), got %+v", r)
			continue
		}
		coding, _ := cc["coding"].([]interface{})
		if len(coding) == 0 {
			continue
		}
		first, _ := coding[0].(map[string]interface{})
		code, _ := first["code"].(string)
		byCode[code] = r
	}

	for wantCode, wantAnswer := range map[string]float64{
		"71492-3": 5,
		"71493-1": 4,
	} {
		q, ok := byCode[wantCode]
		if !ok {
			t.Fatalf("expected MMSE question Observation code=%s to survive, got resources: %+v", wantCode, resources)
		}
		if q["valueInteger"] != wantAnswer {
			t.Errorf("question %s valueInteger = %v, want %v", wantCode, q["valueInteger"], wantAnswer)
		}
		cats, _ := q["category"].([]interface{})
		if len(cats) == 0 {
			t.Errorf("question %s: expected category to be set to cognitive-status, got none", wantCode)
		} else {
			catCoding := firstCoding(t, cats[0])
			if catCoding["code"] != "cognitive-status" {
				t.Errorf("question %s category coding = %v, want code=cognitive-status (not functional-status/social-history)", wantCode, catCoding)
			}
		}
	}

	if _, stillThere := byCode[""]; stillThere {
		t.Error("the null-coded Assessment Scale shell observation must not survive as its own resource")
	}
	if _, ok := byCode["58144-5"]; !ok {
		t.Errorf("expected the sibling Mental Status Observation (58144-5) to still be mapped, got resources: %+v", resources)
	}
}

func TestDeclarativeEngine_ResultsMappingRules_LabResultsAlias_MatchesResults(t *testing.T) {
	rules := cdafhir.ResultsMappingRules()
	if len(rules) != 2 {
		t.Fatalf("ResultsMappingRules(): got %d rules, want 2 (results + labResults)", len(rules))
	}
	if rules[0].SectionKey != "results" {
		t.Errorf("rules[0].SectionKey = %q, want \"results\"", rules[0].SectionKey)
	}
	if rules[1].SectionKey != "labResults" {
		t.Errorf("rules[1].SectionKey = %q, want \"labResults\"", rules[1].SectionKey)
	}
	if rules[0].FHIRResource != rules[1].FHIRResource {
		t.Errorf("FHIRResource differs between results (%q) and labResults (%q)", rules[0].FHIRResource, rules[1].FHIRResource)
	}

	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "observation",
			StatusCode: "completed",
			Code:       cdadocument.CDACode{Code: "2345-7", CodeSystem: "2.16.840.1.113883.6.1", DisplayName: "Glucose"},
			Value: &cdadocument.CDAValue{
				Type:     "PQ",
				Quantity: &cdadocument.CDAQuantity{Value: "95", Unit: "mg/dL"},
			},
		},
	}
	documentMap := documentMapForEntries(t, "labResults", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, rules[1])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 Observation from labResults, got %d", len(resources))
	}
	qty, ok := resources[0]["valueQuantity"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected valueQuantity, got %v", resources[0]["valueQuantity"])
	}
	if qty["value"] != 95.0 {
		t.Errorf("valueQuantity.value = %v, want 95", qty["value"])
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
	// Updated for the performer PractitionerRole/Practitioner upgrade: a
	// performer with a person name but NO representedOrganization now
	// falls back to a bare Practitioner reference (not a display-only
	// string) -- see ImmunizationMappingRules' barePractitionerRow fallback
	// tier doc comment.
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

	var immunization, practitioner map[string]interface{}
	for _, r := range resources {
		switch r["resourceType"] {
		case "Immunization":
			immunization = r
		case "Practitioner":
			practitioner = r
		case "PractitionerRole":
			t.Fatalf("expected NO PractitionerRole (no organization data) -- got one: %v", r)
		}
	}
	if immunization == nil {
		t.Fatalf("expected an Immunization resource, got %d resources: %v", len(resources), resources)
	}
	if practitioner == nil {
		t.Fatal("expected a bare Practitioner resource built from the performer despite no representedOrganization")
	}

	performers, ok := immunization["performer"].([]interface{})
	if !ok || len(performers) != 1 {
		t.Fatalf("performer = %v, want a 1-element array", immunization["performer"])
	}
	performer0, _ := performers[0].(map[string]interface{})
	actor, _ := performer0["actor"].(map[string]interface{})
	wantRef := "Practitioner/" + practitioner["id"].(string)
	if actor["reference"] != wantRef {
		t.Errorf("performer[0].actor.reference = %v, want %v", actor["reference"], wantRef)
	}
	name := firstElement(t, practitioner["name"]).(map[string]interface{})
	if name["family"] != "Doe" {
		t.Errorf("Practitioner.name[0].family = %v, want %q", name["family"], "Doe")
	}

	function, _ := performer0["function"].(map[string]interface{})
	coding := firstCoding(t, function)
	if coding["code"] != "AP" {
		t.Errorf("performer[0].function coding = %v, want code AP", coding)
	}
}

// TestDeclarativeEngine_Immunization_Performer_OrganizationOnly_EmitsPractitionerRole
// mirrors the COMMON real shape found in 11 of the 99397 sample's 13
// Immunization Activity entries: a /performer/assignedEntity with a
// representedOrganization carrying only a plain <name> (e.g. "COSWI58") and
// NO assignedPerson at all. Per buildEmittedSubResource's len<=1 gate, the
// nested Practitioner sub-resource never gets enough fields set to survive,
// so the PractitionerRole ends up referencing JUST the Organization -- this
// proves that degrade-gracefully path actually produces a usable resource
// (not nothing), which the OLD display-only row would have produced
// nothing for (its Scope required an assignedPerson name to resolve at
// all).
func TestDeclarativeEngine_Immunization_Performer_OrganizationOnly_EmitsPractitionerRole(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "substanceAdministration",
			StatusCode: "completed",
			Consumable: &cdadocument.CDAConsumable{
				ManufacturedProduct: cdadocument.CDAManufacturedProduct{
					ManufacturedMaterial: &cdadocument.CDAMaterial{Code: cdadocument.CDACode{Code: "150", CodeSystem: "2.16.840.1.113883.12.292"}},
				},
			},
			Performers: []cdadocument.CDAPerformer{
				{
					TypeCode: "PRF",
					AssignedEntity: cdadocument.CDAAssignedEntity{
						RepresentedOrganization: &cdadocument.CDAOrganization{Names: []string{"COSWI58"}},
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

	var immunization, practitionerRole, organization map[string]interface{}
	for _, r := range resources {
		switch r["resourceType"] {
		case "Immunization":
			immunization = r
		case "PractitionerRole":
			practitionerRole = r
		case "Organization":
			organization = r
		case "Practitioner":
			t.Fatalf("expected NO Practitioner (no assignedPerson) -- got one: %v", r)
		}
	}
	if immunization == nil {
		t.Fatalf("expected an Immunization resource, got %d resources: %v", len(resources), resources)
	}
	if practitionerRole == nil {
		t.Fatal("expected a PractitionerRole referencing the organization despite no assignedPerson")
	}
	if organization == nil {
		t.Fatal("expected an Organization resource for the representedOrganization")
	}
	if practitionerRole["practitioner"] != nil {
		t.Errorf("PractitionerRole.practitioner = %v, want absent (no assignedPerson)", practitionerRole["practitioner"])
	}
	orgRef, _ := practitionerRole["organization"].(map[string]interface{})
	wantRef := "Organization/" + organization["id"].(string)
	if orgRef["reference"] != wantRef {
		t.Errorf("PractitionerRole.organization.reference = %v, want %v", orgRef["reference"], wantRef)
	}
	if organization["name"] != "COSWI58" {
		t.Errorf("Organization.name = %v, want %q", organization["name"], "COSWI58")
	}

	performers, ok := immunization["performer"].([]interface{})
	if !ok || len(performers) != 1 {
		t.Fatalf("performer = %v, want a 1-element array", immunization["performer"])
	}
	performer0, _ := performers[0].(map[string]interface{})
	actor, _ := performer0["actor"].(map[string]interface{})
	wantActorRef := "PractitionerRole/" + practitionerRole["id"].(string)
	if actor["reference"] != wantActorRef {
		t.Errorf("performer[0].actor.reference = %v, want %v", actor["reference"], wantActorRef)
	}
}

// TestDeclarativeEngine_Immunization_Recorded_FromAuthorTime proves the
// Immunization.recorded mapping HL7's C-CDA on FHIR IG specifies
// (CF-immunizations.md: "/author/time -> .recorded") but this codebase
// never implemented at all, despite every one of the 99397 sample's 13
// Immunization Activity entries carrying a real <author><time .../></author>.
func TestDeclarativeEngine_Immunization_Recorded_FromAuthorTime(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "substanceAdministration",
			StatusCode: "completed",
			Consumable: &cdadocument.CDAConsumable{
				ManufacturedProduct: cdadocument.CDAManufacturedProduct{
					ManufacturedMaterial: &cdadocument.CDAMaterial{Code: cdadocument.CDACode{Code: "150", CodeSystem: "2.16.840.1.113883.12.292"}},
				},
			},
			Authors: []cdadocument.CDAAuthor{
				{Time: cdadocument.CDATime{Value: "20240109094729-0700"}},
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
	recorded, _ := resources[0]["recorded"].(string)
	if recorded == "" {
		t.Fatal("expected Immunization.recorded to be set from the author's <time>")
	}
}

// TestDeclarativeEngine_Immunization_CommentActivity_SetsNote proves the
// Immunization.note mapping for C-CDA's Comment Activity (templateId
// 2.16.840.1.113883.10.20.22.4.64, code 48767-8), attached via
// entryRelationship typeCode=COMP -- an IG-specified mapping
// (CF-immunizations.md) added here with NO real-data evidence in the 99397
// sample (all 13 Immunization Activity entries there have zero
// entryRelationships), unlike Medication.note/Condition.note which were
// both evidenced. This test proves the row works structurally per the IG
// citation alone.
func TestDeclarativeEngine_Immunization_CommentActivity_SetsNote(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "substanceAdministration",
			StatusCode: "completed",
			Consumable: &cdadocument.CDAConsumable{
				ManufacturedProduct: cdadocument.CDAManufacturedProduct{
					ManufacturedMaterial: &cdadocument.CDAMaterial{Code: cdadocument.CDACode{Code: "150", CodeSystem: "2.16.840.1.113883.12.292"}},
				},
			},
			EntryRelationships: []cdadocument.CDAEntryRelationship{
				{
					TypeCode: "COMP",
					Entry: cdadocument.CDAEntry{
						EntryType:   "act",
						TemplateIds: []string{"2.16.840.1.113883.10.20.22.4.64"},
						Text:        "Vaccine administered without incident.",
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
	notes, ok := resources[0]["note"].([]interface{})
	if !ok || len(notes) != 1 {
		t.Fatalf("expected 1 note, got %v", resources[0]["note"])
	}
	note, ok := notes[0].(map[string]interface{})
	if !ok {
		t.Fatalf("note[0] not a map: %v", notes[0])
	}
	if note["text"] != "Vaccine administered without incident." {
		t.Errorf("note[0].text = %v, want %q", note["text"], "Vaccine administered without incident.")
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

// TestDeclarativeEngine_Encounter_Location_FromDirectParticipantLOC covers
// the real gap found auditing a real Ascension Wisconsin CCD's Plan-of-Care
// section: a LOC participant directly on the entry, NOT wrapped in a COMP
// entryRelationship -- the ScopeFallbacks added in V183 must also match
// this shape.
func TestDeclarativeEngine_Encounter_Location_FromDirectParticipantLOC(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "encounter",
			StatusCode: "active",
			Code:       cdadocument.CDACode{Code: "AMB"},
			Participants: []cdadocument.CDAParticipant{
				{
					TypeCode: "LOC",
					ParticipantRole: cdadocument.CDAParticipantRole{
						PlayingEntity: &cdadocument.CDAPlayingEntity{Names: []cdadocument.CDAName{{Family: "Ascension Wisconsin Southeast"}}},
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
	if display["display"] != "Ascension Wisconsin Southeast" {
		t.Errorf("location[0].location.display = %v, want \"Ascension Wisconsin Southeast\"", display["display"])
	}
}

// TestDeclarativeEngine_Encounter_PlannedMood_StatusIsPlanned covers the
// real gap found in the same Ascension file: a moodCode=APT (booked future
// visit) entry with statusCode="active" -- the same statusCode a completed
// encounter would carry -- must resolve to FHIR status="planned", not the
// "in-progress" encounter_status_to_fhir alone would produce.
func TestDeclarativeEngine_Encounter_PlannedMood_StatusIsPlanned(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{EntryType: "encounter", MoodCode: "APT", StatusCode: "active", Code: cdadocument.CDACode{Code: "AMB"}},
	}
	documentMap := documentMapForEntries(t, "encounters", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.EncounterMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if resources[0]["status"] != "planned" {
		t.Errorf("status = %v, want \"planned\" (from moodCode=APT, not statusCode=active)", resources[0]["status"])
	}
}

// TestDeclarativeEngine_Encounter_EventMood_StatusUnchangedFromStatusCode
// confirms the EVN (completed) case is untouched by the new moodCode row --
// it must fall through to the existing statusCode-based mapping exactly as
// before V183.
func TestDeclarativeEngine_Encounter_EventMood_StatusUnchangedFromStatusCode(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{EntryType: "encounter", MoodCode: "EVN", StatusCode: "active", Code: cdadocument.CDACode{Code: "AMB"}},
	}
	documentMap := documentMapForEntries(t, "encounters", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.EncounterMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if resources[0]["status"] != "in-progress" {
		t.Errorf("status = %v, want \"in-progress\" (statusCode=active, moodCode=EVN -- unchanged behavior)", resources[0]["status"])
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

// TestDeclarativeEngine_PlanOfCare_PlannedAct_NullFlavorCode_FallsBackToEntryText
// covers the real gap found auditing 2 independently-sourced real CCDs
// (Marshfield Clinic, Dignity Health): a Planned Act (.4.39) entry's own
// <code> is a bare nullFlavor="UNK" with no <originalText> at all -- the
// real content lives in the entry's own sibling <text><reference .../></text>
// instead, already resolved to plain text by the time the declarative
// engine sees it. Before this fix, every one of these fell through to the
// generic "Unknown" placeholder, discarding real narrative content.
func TestDeclarativeEngine_PlanOfCare_PlannedAct_NullFlavorCode_FallsBackToEntryText(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "act",
			MoodCode:   "INT",
			StatusCode: "active",
			Code:       cdadocument.CDACode{NullFlavor: "UNK"},
			Text:       "Return to Clinic - Endocrinology 6/17/26",
		},
	}
	documentMap := documentMapForEntries(t, "planOfTreatment", entries)
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
	code, _ := resources[0]["code"].(map[string]interface{})
	if code["text"] != "Return to Clinic - Endocrinology 6/17/26" {
		t.Errorf("code.text = %v, want the resolved entry text, not the generic \"Unknown\" placeholder", code["text"])
	}
}

// TestDeclarativeEngine_PlanOfCare_NullFlavorCode_NoEntryText_StillFallsBackToUnknown
// confirms the "Unknown" placeholder still applies when there's truly
// nothing else to use -- the new entry.text row must not turn a genuinely
// empty code into a spurious empty code.text.
func TestDeclarativeEngine_PlanOfCare_NullFlavorCode_NoEntryText_StillFallsBackToUnknown(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "act",
			MoodCode:   "INT",
			StatusCode: "active",
			Code:       cdadocument.CDACode{NullFlavor: "UNK"},
		},
	}
	documentMap := documentMapForEntries(t, "planOfTreatment", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResourcesForRules(documentMap, cdafhir.PlanOfCareMappingRules())
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	code, _ := resources[0]["code"].(map[string]interface{})
	if code["text"] != "Unknown" {
		t.Errorf("code.text = %v, want Unknown (no entry text available either)", code["text"])
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

// covPolicyEntry builds a realistic Policy Activity COMP entry with
// the given SOP code, payer org name, and COV participant details.
func covPolicyEntry(sopCode, payorOrg, memberID, relationship string) cdadocument.CDAEntryRelationship {
	covParticipant := cdadocument.CDAParticipant{
		TypeCode: "COV",
		Time: cdadocument.CDATimeRange{
			Low:  cdadocument.CDATime{Value: "20240101"},
			High: cdadocument.CDATime{NullFlavor: "NA"},
		},
		ParticipantRole: cdadocument.CDAParticipantRole{
			Code: cdadocument.CDACode{Code: relationship},
			Ids:  []cdadocument.CDAII{{Root: "1.2.3.99", Extension: memberID}},
		},
	}
	perf := cdadocument.CDAPerformer{
		AssignedEntity: cdadocument.CDAAssignedEntity{
			Code: cdadocument.CDACode{Code: "PAYOR"},
			RepresentedOrganization: &cdadocument.CDAOrganization{
				Names: []string{payorOrg},
			},
		},
	}
	return cdadocument.CDAEntryRelationship{
		TypeCode: "COMP",
		Entry: cdadocument.CDAEntry{
			Code:         cdadocument.CDACode{Code: sopCode, CodeSystem: "2.16.840.1.113883.3.221.5"},
			Performers:   []cdadocument.CDAPerformer{perf},
			Participants: []cdadocument.CDAParticipant{covParticipant},
		},
	}
}

func TestDeclarativeEngine_Coverage_FullEntry_MedicareEpicPattern(t *testing.T) {
	// Mirrors Epic 99397 corpus structure: SOP code "1" = Medicare, member
	// in COV participant, PAYOR performer with representedOrganization.
	entries := []cdadocument.CDAEntry{
		{
			EntryType:          "act",
			Code:               cdadocument.CDACode{Code: "48768-6"},
			EntryRelationships: []cdadocument.CDAEntryRelationship{covPolicyEntry("1", "MEDICARE", "MBRID001", "SELF")},
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

	// type from SOP code "1" (Medicare), NOT from outer 48768-6
	typ, _ := r["type"].(map[string]interface{})
	if typ == nil {
		t.Fatal("type field absent")
	}
	c0 := firstCoding(t, typ)
	if c0["code"] != "1" {
		t.Errorf("type.coding[0].code = %v, want 1 (Medicare SOP)", c0["code"])
	}
	if c0["codeSystem"] == "http://loinc.org" {
		t.Errorf("type.coding[0].system must not be LOINC — should be SOP system, got %v", c0["codeSystem"])
	}

	// payor from PAYOR performer's org name
	payor := firstElement(t, r["payor"]).(map[string]interface{})
	if payor["display"] != "MEDICARE" {
		t.Errorf("payor[0].display = %v, want MEDICARE", payor["display"])
	}

	// period from COV participant.time
	period, _ := r["period"].(map[string]interface{})
	if period == nil {
		t.Fatal("period field absent")
	}
	if period["start"] != "2024-01-01" {
		t.Errorf("period.start = %v, want 2024-01-01", period["start"])
	}

	// relationship from COV participant code SELF -> "self"
	rel, _ := r["relationship"].(map[string]interface{})
	if rel == nil {
		t.Fatal("relationship field absent")
	}
	if firstCoding(t, rel)["code"] != "self" {
		t.Errorf("relationship.coding[0].code = %v, want self", firstCoding(t, rel)["code"])
	}

	// subscriberId from COV participant.participantRole.ids[0].extension
	if r["subscriberId"] != "MBRID001" {
		t.Errorf("subscriberId = %v, want MBRID001", r["subscriberId"])
	}
}

func TestDeclarativeEngine_Coverage_RelationshipMapping(t *testing.T) {
	// Verify multiple relationship codes map correctly.
	for _, tc := range []struct{ cda, fhir string }{
		{"SELF", "self"},
		{"SPOUSE", "spouse"},
		{"SPS", "spouse"},
		{"CHILD", "child"},
		{"CHLDADOPT", "child"},
		{"MTH", "parent"},
		{"FTH", "parent"},
		{"SIGOTHR", "other"},
		{"DEP", "other"},
		{"UNKNOWN_CODE", "other"},
	} {
		tc := tc
		t.Run(tc.cda, func(t *testing.T) {
			entries := []cdadocument.CDAEntry{
				{
					EntryType: "act",
					Code:      cdadocument.CDACode{Code: "48768-6"},
					EntryRelationships: []cdadocument.CDAEntryRelationship{{
						TypeCode: "COMP",
						Entry: cdadocument.CDAEntry{
							Participants: []cdadocument.CDAParticipant{{
								TypeCode: "COV",
								ParticipantRole: cdadocument.CDAParticipantRole{
									Code: cdadocument.CDACode{Code: tc.cda},
								},
							}},
						},
					}},
				},
			}
			documentMap := documentMapForEntries(t, "payersInsurance", entries)
			resources, errs := cdafhir.NewDeclarativeEngine().BuildResources(documentMap, cdafhir.CoverageMappingRules()[0])
			if len(errs) != 0 {
				t.Fatalf("unexpected errors: %+v", errs)
			}
			rel, _ := resources[0]["relationship"].(map[string]interface{})
			if rel == nil {
				t.Fatal("relationship absent")
			}
			got := firstCoding(t, rel)["code"]
			if got != tc.fhir {
				t.Errorf("cda=%s: relationship.code = %v, want %v", tc.cda, got, tc.fhir)
			}
		})
	}
}

func TestDeclarativeEngine_Coverage_RelationshipFallsBackToSelf_WhenNoCOVParticipant(t *testing.T) {
	// No COV participant -> relationship transform skipped -> fallback literal "self" fires.
	entries := []cdadocument.CDAEntry{
		{
			EntryType: "act",
			Code:      cdadocument.CDACode{Code: "48768-6"},
			EntryRelationships: []cdadocument.CDAEntryRelationship{{
				TypeCode: "COMP",
				Entry:    cdadocument.CDAEntry{Code: cdadocument.CDACode{Code: "1", CodeSystem: "2.16.840.1.113883.3.221.5"}},
			}},
		},
	}
	documentMap := documentMapForEntries(t, "payersInsurance", entries)
	resources, errs := cdafhir.NewDeclarativeEngine().BuildResources(documentMap, cdafhir.CoverageMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	rel, _ := resources[0]["relationship"].(map[string]interface{})
	if rel == nil {
		t.Fatal("relationship absent (even fallback literal failed)")
	}
	if firstCoding(t, rel)["code"] != "self" {
		t.Errorf("relationship.code = %v, want self (fallback)", firstCoding(t, rel)["code"])
	}
}

func TestDeclarativeEngine_Coverage_PayorFromCOMPPerformerOrg(t *testing.T) {
	// Payer org name comes from Policy Activity's performers[0].assignedEntity
	// .representedOrganization.names[0] — the primary path, not a fallback.
	entries := []cdadocument.CDAEntry{
		{
			EntryType: "act",
			Code:      cdadocument.CDACode{Code: "48768-6"},
			EntryRelationships: []cdadocument.CDAEntryRelationship{
				{
					TypeCode: "COMP",
					Entry: cdadocument.CDAEntry{
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
	payor := firstElement(t, resources[0]["payor"]).(map[string]interface{})
	if payor["display"] != "Beta Insurance Co" {
		t.Errorf("payor[0].display = %v, want Beta Insurance Co", payor["display"])
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

func TestDeclarativeEngine_Coverage_SubscriberId_FromCOVParticipantRole(t *testing.T) {
	// subscriberId comes from COV participant.participantRole.ids[0] (member ID).
	// Extension preferred; root used as fallback when no extension.
	for _, tc := range []struct {
		name        string
		ids         []cdadocument.CDAII
		wantSubID   string
	}{
		{"extension preferred", []cdadocument.CDAII{{Root: "1.2.3", Extension: "MEMBER123"}}, "MEMBER123"},
		{"root fallback",       []cdadocument.CDAII{{Root: "1.2.3.4.5"}},                    "1.2.3.4.5"},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			entries := []cdadocument.CDAEntry{
				{
					EntryType: "act",
					Code:      cdadocument.CDACode{Code: "48768-6"},
					EntryRelationships: []cdadocument.CDAEntryRelationship{{
						TypeCode: "COMP",
						Entry: cdadocument.CDAEntry{
							Participants: []cdadocument.CDAParticipant{{
								TypeCode: "COV",
								ParticipantRole: cdadocument.CDAParticipantRole{
									Ids: tc.ids,
								},
							}},
						},
					}},
				},
			}
			documentMap := documentMapForEntries(t, "payersInsurance", entries)
			resources, errs := cdafhir.NewDeclarativeEngine().BuildResources(documentMap, cdafhir.CoverageMappingRules()[0])
			if len(errs) != 0 {
				t.Fatalf("unexpected errors: %+v", errs)
			}
			if resources[0]["subscriberId"] != tc.wantSubID {
				t.Errorf("subscriberId = %v, want %v", resources[0]["subscriberId"], tc.wantSubID)
			}
		})
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

// ---- EncompassingEncounter Location (header-level) ----
// No corpus fixture exercises componentOf/encompassingEncounter at all
// (0/4 of cerner/kareo/mtuitive/practicefusion) -- same situation as
// LegalAuthenticator/Patient, hand-built synthetic data only.

func TestDeclarativeEngine_EncompassingEncounterLocation_BuildsLocation(t *testing.T) {
	header := cdadocument.CDAHeader{
		EncompassingEncounter: &cdadocument.CDAEncounter{
			Location: &cdadocument.CDALocation{
				HealthCareFacility: cdadocument.CDAHealthCareFacility{
					Code: cdadocument.CDACode{
						NullFlavor:  "UNK",
						OriginalText: "Obstetrics and Gynecology",
						Translations: []cdadocument.CDACode{
							{Code: "42", CodeSystem: "1.2.840.114350.1.72.1.7.7.10.688867.4150", CodeSystemName: "Epic.DepartmentSpecialty", DisplayName: "Obstetrics & Gynecology"},
						},
					},
					Location: &cdadocument.CDAPlace{
						Names: []cdadocument.CDAName{{Family: "mumbai Women's Care mira road"}},
						Addresses: []cdadocument.CDAAddress{{
							Use:          "WP",
							StreetLines:  []string{"101 mira road Parkway Ste 201B"},
							City:         "mira road",
							State:        "CO",
							PostalCode:   "80511-4072",
							Country:      "USA",
						}},
					},
					ServiceProviderOrganization: &cdadocument.CDAOrganization{
						Names: []string{"mumbai Women's Care"},
						Ids:   []cdadocument.CDAII{{Root: "1.2.840.114350.1.13.549.2.7.2.696570", Extension: "103001002"}},
					},
				},
			},
		},
	}
	documentMap := documentMapForHeader(t, header)
	engine := cdafhir.NewDeclarativeEngine()
	resource, extra, errs := engine.BuildHeaderResource(documentMap, cdafhir.EncompassingEncounterLocationMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if resource == nil {
		t.Fatal("expected a Location resource")
	}
	if resource["resourceType"] != "Location" {
		t.Errorf("resourceType = %v, want Location", resource["resourceType"])
	}
	// CDAPlace.Names is []CDAName (the HL7 PN datatype), not []string -- the
	// row reads .family directly (see EncompassingEncounterLocationMappingRules'
	// own row comment for why: parsePN lands unstructured facility-name text
	// there, datatypes.go's own fallback comment).
	if resource["name"] != "mumbai Women's Care mira road" {
		t.Errorf("name = %v, want %q", resource["name"], "mumbai Women's Care mira road")
	}
	typeCC := firstElement(t, resource["type"]).(map[string]interface{})
	codings := typeCC["coding"].([]interface{})
	if len(codings) != 1 {
		t.Fatalf("type[0].coding length = %d, want 1", len(codings))
	}
	coding := codings[0].(map[string]interface{})
	if coding["display"] != "Obstetrics & Gynecology" {
		t.Errorf("type[0].coding[0].display = %v, want %q", coding["display"], "Obstetrics & Gynecology")
	}
	addr, ok := resource["address"].(map[string]interface{})
	if !ok {
		t.Fatalf("address = %v, want a single object (Location.address is 0..1, not an array)", resource["address"])
	}
	if addr["city"] != "mira road" {
		t.Errorf("address.city = %v, want %q", addr["city"], "mira road")
	}
	if len(extra) != 1 {
		t.Fatalf("expected exactly 1 extra resource (the emitted managingOrganization), got %d: %+v", len(extra), extra)
	}
	org := extra[0]
	if org["resourceType"] != "Organization" {
		t.Errorf("extra[0].resourceType = %v, want Organization", org["resourceType"])
	}
	if org["name"] != "mumbai Women's Care" {
		t.Errorf("extra[0].name = %v, want %q", org["name"], "mumbai Women's Care")
	}
	managingOrg, ok := resource["managingOrganization"].(map[string]interface{})
	if !ok {
		t.Fatalf("managingOrganization = %v, want a reference object", resource["managingOrganization"])
	}
	orgRef, _ := managingOrg["reference"].(string)
	orgID, _ := org["id"].(string)
	if orgRef != "Organization/"+orgID {
		t.Errorf("managingOrganization.reference = %q, want %q", orgRef, "Organization/"+orgID)
	}
}

func TestDeclarativeEngine_EncompassingEncounterLocation_Absent_NoResource(t *testing.T) {
	documentMap := documentMapForHeader(t, cdadocument.CDAHeader{})
	engine := cdafhir.NewDeclarativeEngine()
	resource, _, errs := engine.BuildHeaderResource(documentMap, cdafhir.EncompassingEncounterLocationMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if resource != nil {
		t.Errorf("expected no resource (no encompassingEncounter present), got %v", resource)
	}
}

func TestDeclarativeEngine_EncompassingEncounterLocation_NoName_NoResource(t *testing.T) {
	header := cdadocument.CDAHeader{
		EncompassingEncounter: &cdadocument.CDAEncounter{
			Location: &cdadocument.CDALocation{
				HealthCareFacility: cdadocument.CDAHealthCareFacility{
					Code: cdadocument.CDACode{Code: "42"},
				},
			},
		},
	}
	documentMap := documentMapForHeader(t, header)
	engine := cdafhir.NewDeclarativeEngine()
	resource, _, errs := engine.BuildHeaderResource(documentMap, cdafhir.EncompassingEncounterLocationMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if resource != nil {
		t.Errorf("expected no resource (RequiredPaths:[name] gate, no location.names), got %v", resource)
	}
}

// ---- EncompassingEncounter -> Encounter (header-level) ----
// Per the HL7 C-CDA on FHIR IG (verified via WebFetch 2026-06-27):
// componentOf/encompassingEncounter always converts to a FHIR Encounter --
// no RequiredPaths gate, unlike most other header rules. Consolidation with
// a matching in-section Encounter is declarative_document_mapper.go's job
// (see EncompassingEncounterMappingRules' own doc comment); these tests
// cover only the per-rule engine mechanics.

func TestDeclarativeEngine_EncompassingEncounter_AllFieldsMapped(t *testing.T) {
	header := cdadocument.CDAHeader{
		EncompassingEncounter: &cdadocument.CDAEncounter{
			Id:                       cdadocument.CDAII{Root: "2.16.840.1.113883.19", Extension: "ENC-1"},
			EffectiveTime:            cdadocument.CDATimeRange{Low: cdadocument.CDATime{Value: "20240101"}, High: cdadocument.CDATime{Value: "20240105"}},
			DischargeDispositionCode: cdadocument.CDACode{Code: "01", DisplayName: "Discharged to home", CodeSystem: "2.16.840.1.113883.6.301.5"},
		},
	}
	documentMap := documentMapForHeader(t, header)
	engine := cdafhir.NewDeclarativeEngine()
	resource, extra, errs := engine.BuildHeaderResource(documentMap, cdafhir.EncompassingEncounterMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(extra) != 0 {
		t.Fatalf("expected no extra resources, got %d", len(extra))
	}
	if resource == nil {
		t.Fatal("expected an Encounter resource")
	}
	if resource["resourceType"] != "Encounter" {
		t.Errorf("resourceType = %v, want Encounter", resource["resourceType"])
	}
	// No statusCode exists on CDAEncounter at all -- always falls through to
	// the transform's own "unknown" default, same as EncounterMappingRules'
	// own row for a nullFlavor entry.
	if resource["status"] != "unknown" {
		t.Errorf("status = %v, want unknown", resource["status"])
	}
	ident := firstElement(t, resource["identifier"]).(map[string]interface{})
	if ident["value"] != "ENC-1" {
		t.Errorf("identifier[0].value = %v, want ENC-1", ident["value"])
	}
	period, ok := resource["period"].(map[string]interface{})
	if !ok || period["start"] == nil {
		t.Errorf("period = %v, want a populated period", resource["period"])
	}
	hosp, ok := resource["hospitalization"].(map[string]interface{})
	if !ok {
		t.Fatalf("hospitalization = %v, want an object", resource["hospitalization"])
	}
	dd, ok := hosp["dischargeDisposition"].(map[string]interface{})
	if !ok {
		t.Fatalf("hospitalization.dischargeDisposition = %v, want a CodeableConcept", hosp["dischargeDisposition"])
	}
	coding := firstElement(t, dd["coding"]).(map[string]interface{})
	if coding["display"] != "Discharged to home" {
		t.Errorf("hospitalization.dischargeDisposition.coding[0].display = %v, want %q", coding["display"], "Discharged to home")
	}
}

func TestDeclarativeEngine_EncompassingEncounter_Absent_NoResource(t *testing.T) {
	documentMap := documentMapForHeader(t, cdadocument.CDAHeader{})
	engine := cdafhir.NewDeclarativeEngine()
	resource, _, errs := engine.BuildHeaderResource(documentMap, cdafhir.EncompassingEncounterMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if resource != nil {
		t.Errorf("expected no resource (no encompassingEncounter present), got %v", resource)
	}
}

func TestDeclarativeEngine_EncompassingEncounter_EmptyElement_StillConvertedPerIG(t *testing.T) {
	// A bare componentOf/encompassingEncounter with no id/effectiveTime/
	// dischargeDispositionCode still produces an Encounter -- the IG's own
	// instruction has no qualifier ("if the document itself contains a
	// componentOf/encompassingEncounter, this should also be converted to a
	// FHIR Encounter resource"), and status always resolves to "unknown"
	// regardless, so the resource is never empty enough to be discarded.
	header := cdadocument.CDAHeader{EncompassingEncounter: &cdadocument.CDAEncounter{}}
	documentMap := documentMapForHeader(t, header)
	engine := cdafhir.NewDeclarativeEngine()
	resource, _, errs := engine.BuildHeaderResource(documentMap, cdafhir.EncompassingEncounterMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if resource == nil {
		t.Fatal("expected an Encounter resource even for a bare encompassingEncounter element")
	}
	if resource["status"] != "unknown" {
		t.Errorf("status = %v, want unknown", resource["status"])
	}
}

// ---- Patient (header-level, Phase 4 Slice A) ----
// No mappers_test.go precedent exists for MapPatient (zero dedicated Go
// tests per the inventory) -- these are new, not ported.

func TestDeclarativeEngine_Patient_AllFieldsMapped(t *testing.T) {
	deceased := false
	header := cdadocument.CDAHeader{
		Patient: cdadocument.CDAPatient{
			Ids:         []cdadocument.CDAII{{Root: "2.16.840.1.113883.4.1", Extension: "999-99-9999"}},
			Names:       []cdadocument.CDAName{{Family: "Doe", Given: []string{"Jane"}, Use: "L"}},
			Addresses:   []cdadocument.CDAAddress{{StreetLines: []string{"1 Main St"}, City: "Boulder", State: "CO", PostalCode: "80301"}},
			Telecoms:    []cdadocument.CDATelecom{{Value: "tel:+1-555-0123"}},
			BirthDate:   cdadocument.CDATime{Value: "19800615"},
			Gender:      cdadocument.CDACode{Code: "F"},
			DeceasedInd: &deceased,
			MaritalStatus: cdadocument.CDACode{
				Code: "M", CodeSystem: "2.16.840.1.113883.5.2", DisplayName: "Married",
			},
			Languages: []cdadocument.CDALanguage{{Code: "en", PreferenceInd: true}},
		},
	}
	documentMap := documentMapForHeader(t, header)
	engine := cdafhir.NewDeclarativeEngine()
	resource, extra, errs := engine.BuildHeaderResource(documentMap, cdafhir.PatientMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(extra) != 0 {
		t.Fatalf("expected no extra resources, got %d", len(extra))
	}
	if resource == nil {
		t.Fatal("expected a Patient resource")
	}
	if resource["resourceType"] != "Patient" {
		t.Errorf("resourceType = %v, want Patient", resource["resourceType"])
	}
	ident := firstElement(t, resource["identifier"]).(map[string]interface{})
	if ident["value"] != "999-99-9999" {
		t.Errorf("identifier[0].value = %v, want 999-99-9999", ident["value"])
	}
	name := firstElement(t, resource["name"]).(map[string]interface{})
	if name["family"] != "Doe" {
		t.Errorf("name[0].family = %v, want Doe", name["family"])
	}
	addr := firstElement(t, resource["address"]).(map[string]interface{})
	if addr["city"] != "Boulder" {
		t.Errorf("address[0].city = %v, want Boulder", addr["city"])
	}
	if resource["birthDate"] != "1980-06-15" {
		t.Errorf("birthDate = %v, want 1980-06-15", resource["birthDate"])
	}
	if resource["gender"] != "female" {
		t.Errorf("gender = %v, want female", resource["gender"])
	}
	if resource["deceasedBoolean"] != false {
		t.Errorf("deceasedBoolean = %v, want false (explicit sdtc:deceasedInd value=\"false\" must be preserved, not omitted)", resource["deceasedBoolean"])
	}
	marital := resource["maritalStatus"].(map[string]interface{})
	if marital["text"] != "Married" {
		t.Errorf("maritalStatus.text = %v, want Married", marital["text"])
	}
	comm := firstElement(t, resource["communication"]).(map[string]interface{})
	if comm["preferred"] != true {
		t.Errorf("communication[0].preferred = %v, want true", comm["preferred"])
	}
	lang := comm["language"].(map[string]interface{})
	if firstCoding(t, lang)["code"] != "en" {
		t.Errorf("communication[0].language.coding[0].code = %v, want en", firstCoding(t, lang)["code"])
	}
}

func TestDeclarativeEngine_Patient_NoDeceasedInd_NoDeceasedBooleanField(t *testing.T) {
	// sdtc:deceasedInd absent entirely (not even value="false") must leave
	// deceasedBoolean unset, not default it to false -- "no data" and
	// "explicitly alive" are different facts.
	header := cdadocument.CDAHeader{
		Patient: cdadocument.CDAPatient{
			Names: []cdadocument.CDAName{{Family: "Doe"}},
		},
	}
	documentMap := documentMapForHeader(t, header)
	engine := cdafhir.NewDeclarativeEngine()
	resource, _, errs := engine.BuildHeaderResource(documentMap, cdafhir.PatientMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if _, ok := resource["deceasedBoolean"]; ok {
		t.Errorf("expected no deceasedBoolean field (sdtc:deceasedInd absent), got %v", resource["deceasedBoolean"])
	}
}

func TestDeclarativeEngine_Patient_NullFlavorGender_NoGenderField(t *testing.T) {
	// Mirrors patient_mapper.go:80-84's exact guard: an explicit nullFlavor
	// with no code means Go skips writing "gender" entirely (the one case
	// the guard changes behavior for -- see declarativeGenderToFHIR's doc
	// comment).
	header := cdadocument.CDAHeader{
		Patient: cdadocument.CDAPatient{
			Names:  []cdadocument.CDAName{{Family: "Doe"}},
			Gender: cdadocument.CDACode{NullFlavor: "UNK"},
		},
	}
	documentMap := documentMapForHeader(t, header)
	engine := cdafhir.NewDeclarativeEngine()
	resource, _, errs := engine.BuildHeaderResource(documentMap, cdafhir.PatientMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if _, ok := resource["gender"]; ok {
		t.Errorf("expected no gender field (explicit nullFlavor, no code), got %v", resource["gender"])
	}
}

func TestDeclarativeEngine_Patient_EmptyGender_DefaultsUnknown(t *testing.T) {
	// An all-empty CDACode (no code, no nullFlavor) still resolves to
	// "unknown" -- transforms.GenderToFHIR("") defaults there, and Go's own
	// guard (Code!="" || NullFlavor=="") passes for this input too.
	header := cdadocument.CDAHeader{
		Patient: cdadocument.CDAPatient{
			Names: []cdadocument.CDAName{{Family: "Doe"}},
		},
	}
	documentMap := documentMapForHeader(t, header)
	engine := cdafhir.NewDeclarativeEngine()
	resource, _, errs := engine.BuildHeaderResource(documentMap, cdafhir.PatientMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if resource["gender"] != "unknown" {
		t.Errorf("gender = %v, want unknown", resource["gender"])
	}
}

// ---- PatientRefPath / EmbedCDAIdentity (Phase 4 Slice A engine mechanics) ----

func TestDeclarativeEngine_PatientRef_SetsCorrectFieldPerResourceType(t *testing.T) {
	entries := []cdadocument.CDAEntry{{EntryType: "act", StatusCode: "active"}}
	documentMap := documentMapForEntries(t, "immunizations", entries)
	engine := cdafhir.NewDeclarativeEngine()
	engine.PatientRef = "Patient/patient-1"
	resources, errs := engine.BuildResources(documentMap, cdafhir.ImmunizationMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 1 {
		t.Fatalf("expected 1 resource, got %d", len(resources))
	}
	patient, ok := resources[0]["patient"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected resource[\"patient\"] to be a reference map, got %v", resources[0]["patient"])
	}
	if patient["reference"] != "Patient/patient-1" {
		t.Errorf("patient.reference = %v, want Patient/patient-1", patient["reference"])
	}
}

func TestDeclarativeEngine_PatientRef_EmptyIsNoOp(t *testing.T) {
	// Zero-value PatientRef (every existing call site before this field
	// existed) must behave exactly as before -- no patient field written.
	entries := []cdadocument.CDAEntry{{EntryType: "act", StatusCode: "active"}}
	documentMap := documentMapForEntries(t, "immunizations", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.ImmunizationMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if _, ok := resources[0]["patient"]; ok {
		t.Errorf("expected no patient field when PatientRef is empty, got %v", resources[0]["patient"])
	}
}

func TestDeclarativeEngine_PatientRef_CoverageSetsBeneficiaryAndSubscriber(t *testing.T) {
	entries := []cdadocument.CDAEntry{{EntryType: "act", StatusCode: "completed", EntryRelationships: []cdadocument.CDAEntryRelationship{
		{TypeCode: "COMP", Entry: cdadocument.CDAEntry{Id: []cdadocument.CDAII{{Root: "1.2.3", Extension: "POL1"}}}},
	}}}
	documentMap := documentMapForEntries(t, "payersInsurance", entries)
	engine := cdafhir.NewDeclarativeEngine()
	engine.PatientRef = "Patient/patient-1"
	resources, errs := engine.BuildResources(documentMap, cdafhir.CoverageMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	beneficiary := resources[0]["beneficiary"].(map[string]interface{})
	subscriber := resources[0]["subscriber"].(map[string]interface{})
	if beneficiary["reference"] != "Patient/patient-1" || subscriber["reference"] != "Patient/patient-1" {
		t.Errorf("beneficiary=%v subscriber=%v, want both Patient/patient-1", beneficiary, subscriber)
	}
}

func TestDeclarativeEngine_EmbedCDAIdentity_AuthorPractitionerGetsCdaIds(t *testing.T) {
	header := cdadocument.CDAHeader{
		Authors: []cdadocument.CDAAuthor{
			{AssignedAuthor: cdadocument.CDAAssignedAuthor{
				Ids:            []cdadocument.CDAII{{Root: "2.16.840.1.113883.4.6", Extension: "1234567890"}},
				AssignedPerson: &cdadocument.CDAPerson{Names: []cdadocument.CDAName{{Family: "Smith"}}},
			}},
		},
	}
	documentMap := documentMapForHeader(t, header)
	engine := cdafhir.NewDeclarativeEngine()
	resource, _, errs := engine.BuildHeaderResource(documentMap, cdafhir.AuthorMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	cdaIds, ok := resource["_cdaIds"].([]interface{})
	if !ok || len(cdaIds) != 1 {
		t.Fatalf("expected one _cdaIds entry, got %v", resource["_cdaIds"])
	}
	entry := cdaIds[0].(map[string]interface{})
	if entry["root"] != "2.16.840.1.113883.4.6" || entry["extension"] != "1234567890" {
		t.Errorf("_cdaIds[0] = %v, want root=2.16.840.1.113883.4.6 extension=1234567890", entry)
	}
}

func TestDeclarativeEngine_EmbedCDAIdentity_SectionResourceNeverGetsCdaIds(t *testing.T) {
	// Parity guard: Go's section-level mappers (Allergy/Condition/Medication/
	// etc.) never call embedCDAIds -- only Author/Custodian/CareTeam's
	// emitted Practitioner do (see MappingRow.EmbedCDAIdentity's doc
	// comment). A section-level resource must NOT get _cdaIds even when its
	// entry carries a top-level "id" -- that would dedup more than Go ever
	// did.
	entries := []cdadocument.CDAEntry{{
		EntryType:  "act",
		StatusCode: "active",
		Id:         []cdadocument.CDAII{{Root: "1.2.3", Extension: "ENTRY1"}},
	}}
	documentMap := documentMapForEntries(t, "immunizations", entries)
	engine := cdafhir.NewDeclarativeEngine()
	resources, errs := engine.BuildResources(documentMap, cdafhir.ImmunizationMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if _, ok := resources[0]["_cdaIds"]; ok {
		t.Errorf("expected no _cdaIds on a section-level resource, got %v", resources[0]["_cdaIds"])
	}
}

// ---- Clinical Note (Note Activity, .4.202) ----
// Real shape found auditing the "historyOfPresentIllness" section (LOINC
// 10164-2, Epic-titled "Progress Notes") of the 99397 sample -- see
// ClinicalNoteMappingRules' own doc comment for the full provenance of every
// field below.

func TestDeclarativeEngine_ClinicalNote_NoteActivity_MapsToDocumentReference(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:   "act",
			ClassCode:   "ACT",
			MoodCode:    "EVN",
			TemplateIds: []string{"2.16.840.1.113883.10.20.22.4.202"},
			Id: []cdadocument.CDAII{
				{Root: "1.2.840.114350.1.13.549.2.7.2.727879", Extension: "457663568"},
			},
			Code: cdadocument.CDACode{
				Code:           "34109-9",
				CodeSystem:     "2.16.840.1.113883.6.1",
				CodeSystemName: "LOINC",
				DisplayName:    "Note",
				OriginalText:   "Progress Notes",
				Translations: []cdadocument.CDACode{
					{Code: "11506-3", CodeSystem: "2.16.840.1.113883.6.1", CodeSystemName: "LOINC", DisplayName: "Progress note"},
				},
			},
			// Already-resolved plain text -- the real entry carries
			// <text><reference value="#Note67"/></text>, resolved against the
			// section's own narrative index by resolveEntryRefs before the
			// declarative engine ever sees this entry. Tests at this layer
			// start from the post-resolution value, same as every other
			// ported test in this file.
			Text:       "RETURN ANNUAL GYNECOLOGY VISIT. Mary is a 66 y.o. female who presents for routine care.",
			StatusCode: "completed",
			EffectiveTime: cdadocument.CDATimeRange{
				Value: cdadocument.CDATime{Value: "20260106093000-0700"},
			},
			Authors: []cdadocument.CDAAuthor{
				{
					AssignedAuthor: cdadocument.CDAAssignedAuthor{
						Ids: []cdadocument.CDAII{{Root: "2.16.840.1.113883.4.6", Extension: "1952640419"}},
						AssignedPerson: &cdadocument.CDAPerson{
							Names: []cdadocument.CDAName{{Given: []string{"Diane"}, Family: "Utz"}},
						},
					},
				},
			},
			Informants: []cdadocument.CDAInformant{
				{
					AssignedEntity: cdadocument.CDAAssignedEntity{
						RepresentedOrganization: &cdadocument.CDAOrganization{
							Names: []string{"mumbai Community Health and Affiliates"},
						},
					},
				},
			},
		},
	}
	documentMap := documentMapForEntries(t, "historyOfPresentIllness", entries)
	engine := cdafhir.NewDeclarativeEngine()
	engine.PatientRef = "Patient/patient-1"

	resources, errs := engine.BuildResources(documentMap, cdafhir.ClinicalNoteMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 3 {
		t.Fatalf("expected 3 resources (DocumentReference + emitted author Practitioner + emitted custodian Organization), got %d: %+v", len(resources), resources)
	}

	var doc map[string]interface{}
	for _, r := range resources {
		if r["resourceType"] == "DocumentReference" {
			doc = r
		}
	}
	if doc == nil {
		t.Fatalf("no DocumentReference among returned resources: %+v", resources)
	}

	if got := doc["subject"].(map[string]interface{})["reference"]; got != "Patient/patient-1" {
		t.Errorf("subject.reference = %v, want Patient/patient-1", got)
	}
	if doc["status"] != "current" {
		t.Errorf("status = %v, want current", doc["status"])
	}
	if doc["date"] != "2026-01-06T09:30:00-07:00" {
		t.Errorf("date = %v, want 2026-01-06T09:30:00-07:00", doc["date"])
	}

	typeCC, ok := doc["type"].(map[string]interface{})
	if !ok {
		t.Fatalf("type is not a CodeableConcept-shaped map: %v", doc["type"])
	}
	typeCodings, _ := typeCC["coding"].([]interface{})
	if len(typeCodings) != 2 {
		t.Fatalf("type.coding length = %d, want 2 (primary 34109-9 + translation 11506-3)", len(typeCodings))
	}
	if got := typeCodings[1].(map[string]interface{})["code"]; got != "11506-3" {
		t.Errorf("type.coding[1].code = %v, want 11506-3 (the standards-based Progress Note code, via the translation fold)", got)
	}

	catCoding := firstCoding(t, doc["category"].([]interface{})[0])
	if catCoding["code"] != "clinical-note" {
		t.Errorf("category[0].coding[0].code = %v, want clinical-note", catCoding["code"])
	}

	attachment, ok := doc["content"].([]interface{})[0].(map[string]interface{})["attachment"].(map[string]interface{})
	if !ok {
		t.Fatalf("content[0].attachment is not a map: %v", doc["content"])
	}
	if attachment["contentType"] != "text/plain" {
		t.Errorf("content[0].attachment.contentType = %v, want text/plain", attachment["contentType"])
	}
	decoded, err := base64.StdEncoding.DecodeString(attachment["data"].(string))
	if err != nil {
		t.Fatalf("content[0].attachment.data is not valid base64: %v", err)
	}
	if string(decoded) != "RETURN ANNUAL GYNECOLOGY VISIT. Mary is a 66 y.o. female who presents for routine care." {
		t.Errorf("decoded attachment data = %q, want the resolved note text", string(decoded))
	}

	idCoding, _ := doc["identifier"].([]interface{})
	if len(idCoding) != 1 {
		t.Fatalf("identifier length = %d, want 1", len(idCoding))
	}

	authorRef, ok := doc["author"].([]interface{})[0].(map[string]interface{})["reference"].(string)
	if !ok || authorRef == "" {
		t.Fatalf("author[0].reference missing: %v", doc["author"])
	}

	custodianRef, ok := doc["custodian"].(map[string]interface{})["reference"].(string)
	if !ok || custodianRef == "" {
		t.Fatalf("custodian.reference missing: %v", doc["custodian"])
	}
}

// TestDeclarativeEngine_ClinicalNote_OtherEntryTypes_NotClaimed confirms
// EntryMatch keeps this rule scoped to ONLY Note Activity (.4.202) entries
// -- a plain entry with no matching templateId in the same section must
// produce zero DocumentReference resources, not a malformed one.
func TestDeclarativeEngine_ClinicalNote_OtherEntryTypes_NotClaimed(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		{
			EntryType:  "observation",
			StatusCode: "completed",
			Code:       cdadocument.CDACode{Code: "10164-2", CodeSystem: "2.16.840.1.113883.6.1"},
		},
	}
	documentMap := documentMapForEntries(t, "historyOfPresentIllness", entries)
	engine := cdafhir.NewDeclarativeEngine()

	resources, errs := engine.BuildResources(documentMap, cdafhir.ClinicalNoteMappingRules()[0])
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %+v", errs)
	}
	if len(resources) != 0 {
		t.Fatalf("expected 0 DocumentReference (no Note Activity templateId present), got %d", len(resources))
	}
}

// ── Section-level narrative tests ────────────────────────────────────────────
// These exercise DeclarativeMapDocument's second dispatch pass for CDA sections
// that have no structured entries and carry content only in their <text>
// narrative block. Unlike the engine-level tests above, these call the full
// mapper so the narrative dispatch pass in the orchestration shell is exercised.

// narrativeDocForSection builds a minimal *cdadocument.CDADocument with one
// narrative-only section (empty Entries) suitable for testing the section-level
// DocumentReference dispatch.
func narrativeDocForSection(sectionKey, loincCode, title, narrativeText string, withEntries bool) *cdadocument.CDADocument {
	section := &cdadocument.CDASection{
		Key:           sectionKey,
		LoincCode:     loincCode,
		Title:         title,
		NarrativeText: narrativeText,
	}
	if withEntries {
		section.Entries = []cdadocument.CDAEntry{
			{EntryType: "observation", StatusCode: "completed",
				Code: cdadocument.CDACode{Code: "55607006", CodeSystem: "2.16.840.1.113883.6.96"}},
		}
	}
	return &cdadocument.CDADocument{
		SectionsByKey: map[string]*cdadocument.CDASection{sectionKey: section},
	}
}

func TestNarrativeSection_AssessmentWithNarrative_ProducesDocumentReference(t *testing.T) {
	// Real gap found auditing Epic 99397 + Ascension: "Visit Diagnoses" section
	// (LOINC 51848-0) is narrative-only in both files. Before this fix the
	// section produced nothing; now it must produce one DocumentReference.
	doc := narrativeDocForSection("assessment", "51848-0", "Visit Diagnoses",
		"<text><table><tbody><tr><td>Osteopenia</td></tr></tbody></table></text>", false)

	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	out, err := mapper.DeclarativeMapDocument(context.Background(), doc, cdafhir.CDAToFHIRConfig{})
	if err != nil {
		t.Fatalf("DeclarativeMapDocument: %v", err)
	}

	var docRefs []map[string]interface{}
	entries, _ := out.FHIRBundle["entry"].([]interface{})
	for _, e := range entries {
		entry, _ := e.(map[string]interface{})
		r, _ := entry["resource"].(map[string]interface{})
		if r["resourceType"] == "DocumentReference" {
			docRefs = append(docRefs, r)
		}
	}
	if len(docRefs) != 1 {
		t.Fatalf("expected 1 DocumentReference for narrative assessment section, got %d", len(docRefs))
	}

	dr := docRefs[0]
	typeCC, _ := dr["type"].(map[string]interface{})
	codings, _ := typeCC["coding"].([]interface{})
	if len(codings) == 0 {
		t.Fatal("DocumentReference.type.coding is empty")
	}
	c0, _ := codings[0].(map[string]interface{})
	if c0["code"] != "51848-0" {
		t.Errorf("type.coding[0].code = %v, want 51848-0", c0["code"])
	}
	if c0["display"] != "Visit Diagnoses" {
		t.Errorf("type.coding[0].display = %v, want Visit Diagnoses (section title wins over fallback)", c0["display"])
	}
	content, _ := dr["content"].([]interface{})
	if len(content) == 0 {
		t.Fatal("DocumentReference.content is empty")
	}
	att, _ := content[0].(map[string]interface{})["attachment"].(map[string]interface{})
	if att["contentType"] != "text/html" {
		t.Errorf("attachment.contentType = %v, want text/html", att["contentType"])
	}
	if att["data"] == "" || att["data"] == nil {
		t.Error("attachment.data must not be empty (base64 of the narrative HTML)")
	}
}

func TestNarrativeSection_AssessmentWithEntries_NoDocumentReference(t *testing.T) {
	// When a section has structured entries, the entry-dispatch loop handles it
	// and the narrative pass must stay silent -- no redundant DocumentReference.
	doc := narrativeDocForSection("assessment", "51848-0", "Visit Diagnoses",
		"<text>narrative</text>", true /* withEntries */)

	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	out, err := mapper.DeclarativeMapDocument(context.Background(), doc, cdafhir.CDAToFHIRConfig{})
	if err != nil {
		t.Fatalf("DeclarativeMapDocument: %v", err)
	}

	entries, _ := out.FHIRBundle["entry"].([]interface{})
	for _, e := range entries {
		entry, _ := e.(map[string]interface{})
		r, _ := entry["resource"].(map[string]interface{})
		if r["resourceType"] == "DocumentReference" {
			t.Fatalf("expected no DocumentReference when section has entries, got one")
		}
	}
}

func TestNarrativeSection_EmptyNarrativeText_NoDocumentReference(t *testing.T) {
	// Sections with NullFlavor-NI or empty narrative must not produce a
	// DocumentReference (nothing to attach as content).
	doc := narrativeDocForSection("assessment", "51848-0", "Visit Diagnoses",
		"" /* empty narrative */, false)

	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	out, err := mapper.DeclarativeMapDocument(context.Background(), doc, cdafhir.CDAToFHIRConfig{})
	if err != nil {
		t.Fatalf("DeclarativeMapDocument: %v", err)
	}

	entries, _ := out.FHIRBundle["entry"].([]interface{})
	for _, e := range entries {
		entry, _ := e.(map[string]interface{})
		r, _ := entry["resource"].(map[string]interface{})
		if r["resourceType"] == "DocumentReference" {
			t.Fatal("expected no DocumentReference for empty narrative, got one")
		}
	}
}

func TestNarrativeSection_DefaultDefsHaveCorrectLOINCCodes(t *testing.T) {
	// Regression guard: confirms all 6 keys and their LOINC fallback codes
	// match the evidence-backed values chosen at implementation time.
	expected := map[string]string{
		"assessment":            "51848-0",
		"hospitalCourse":        "8648-8",
		"dischargeInstructions": "18776-5",
		"reasonForReferral":     "42349-1",
		"reasonForVisit":        "29299-5",
		"clinicalNote":          "34109-9",
	}
	for key, wantCode := range expected {
		def, ok := cdafhir.DefaultNarrativeSectionDefs[key]
		if !ok {
			t.Errorf("DefaultNarrativeSectionDefs missing key %q", key)
			continue
		}
		if def.LoincCode != wantCode {
			t.Errorf("DefaultNarrativeSectionDefs[%q].LoincCode = %q, want %q", key, def.LoincCode, wantCode)
		}
	}
}
