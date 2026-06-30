// services/cda_fhir/declarative_document_mapper_test.go
//
// Phase 4 Slice B exit criterion: DeclarativeMapDocument() produces a
// *MapOutput "identical in shape" to MapDocument()'s for the same input —
// proven here at the document-orchestration level (resource-type counts,
// section success/failure sets, total resource count), not re-proving
// field-level correctness Phase 3's per-rule tests already cover.
package cdafhir_test

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	cdaSchema "ezhealthkonnect/cda"
	cdadocument "ezhealthkonnect/cda/document"
	cdafhir "ezhealthkonnect/services/cda_fhir"
	mappinglog "ezhealthkonnect/services/cda_fhir/mapping_log"

	"github.com/beevik/etree"
)

// loadTypedCDADocument parses a CDA testdata fixture into a *cdadocument.CDADocument.
// testing.TB-based (unlike document_mapper_bench_test.go's *testing.B-only
// loadCDADocumentForBench) so the same helper serves both this file's
// correctness tests and any future benchmark in this file.
func loadTypedCDADocument(t testing.TB, relPath string) *cdadocument.CDADocument {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")

	data, err := os.ReadFile(filepath.Join(repoRoot, "cda", "document", "testdata", relPath))
	if err != nil {
		t.Fatalf("reading testdata %s: %v", relPath, err)
	}
	raw := string(data)

	xmlDoc := etree.NewDocument()
	if err := xmlDoc.ReadFromString(raw); err != nil {
		t.Fatalf("parsing XML in %s: %v", relPath, err)
	}

	loader, err := cdaSchema.NewCDASchemaLoader(filepath.Join(repoRoot, "cda", "schemas"))
	if err != nil {
		t.Fatalf("loading CDA schema: %v", err)
	}

	return cdadocument.NewCDAParser(loader).ParseDocument(xmlDoc.Root(), raw)
}

// resourceTypeCounts builds a resourceType -> count histogram from a FHIR
// Bundle's entry[] array -- the comparison unit for this test (exact ids/
// ordering are allowed to differ between the two engines; resource SHAPE
// at the document level is what must match).
func resourceTypeCounts(t *testing.T, bundle map[string]interface{}) map[string]int {
	t.Helper()
	counts := make(map[string]int)
	entries, _ := bundle["entry"].([]interface{})
	for _, e := range entries {
		entryMap, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		resource, ok := entryMap["resource"].(map[string]interface{})
		if !ok {
			continue
		}
		rt, _ := resource["resourceType"].(string)
		counts[rt]++
	}
	return counts
}

func sortedCopy(s []string) []string {
	out := append([]string(nil), s...)
	sort.Strings(out)
	return out
}

// corpusExpectedShape is one corpus file's hardcoded expected output shape --
// the regression baseline this test guards. Captured from
// DeclarativeMapDocument() itself once Phase 4 Slice B's divergence-fixing
// was complete and Slice D's gap-closure (functionalStatus/mentalStatus/
// labResults, none of which any of these 4 vendor files exercise) was done,
// so these numbers reflect the final, decommissioned-Map()-equivalent state.
type corpusExpectedShape struct {
	resourceCounts    map[string]int
	resourcesProduced int
	successfulCount   int
}

var corpusExpectedShapes = map[string]corpusExpectedShape{
	"cerner_sample.xml": {
		resourceCounts: map[string]int{
			"Condition": 5, "MedicationRequest": 6, "Organization": 2, "Patient": 1, "Practitioner": 1,
			"PractitionerRole": 1,
		},
		resourcesProduced: 16,
		successfulCount:   2,
	},
	"kareo_sample.xml": {
		resourceCounts: map[string]int{
			"AllergyIntolerance": 1, "Condition": 2, "Encounter": 1, "Immunization": 1,
			"MedicationStatement": 2, "Observation": 6, "Organization": 1, "Patient": 1,
			"Practitioner": 1, "Procedure": 1,
		},
		resourcesProduced: 17,
		successfulCount:   9,
	},
	"mtuitive_sample.xml": {
		resourceCounts: map[string]int{
			"Organization": 1, "Patient": 1, "Practitioner": 1,
		},
		resourcesProduced: 3,
		successfulCount:   0,
	},
	"practicefusion_sample.xml": {
		// +1 DocumentReference (reasonForReferral/narrative), +1 resource, +1
		// section -- the new section-level narrative dispatch pass (V186 design)
		// now produces a DocumentReference for sections with narrative but no
		// entries; practicefusion_sample.xml has a reasonForReferral section
		// with exactly that shape.
		resourceCounts: map[string]int{
			"AllergyIntolerance": 2, "Condition": 2, "DocumentReference": 1, "Encounter": 1, "Immunization": 1,
			"MedicationStatement": 1, "Observation": 1, "Organization": 2, "Patient": 1,
			"Practitioner": 1, "PractitionerRole": 1, "Procedure": 1,
		},
		resourcesProduced: 15,
		successfulCount:   9,
	},
}

// TestDeclarativeMapDocument_Corpus_MatchesKnownShape is a regression guard
// against the now-decommissioned hardcoded Go mapper path (mappers/*.go +
// document_mapper.go, deleted in Phase 4 Slice D): the expected counts below
// are exactly what that Go path produced for these 4 real-world vendor
// samples, confirmed identical to DeclarativeMapDocument()'s own output by
// TestDeclarativeMapDocument_Corpus_MatchesMapDocumentShape before the Go
// path was deleted. Any change here means a real behavioral regression, not
// a comparison-target update (there is no second engine left to compare
// against) — UNLESS the change is deliberate, NEW functionality with no Go
// equivalent to regress from, same as Phase 4's other additive slices
// (LegalAuthenticator, CareTeam's PractitionerRole) already were.
//
// cerner_sample.xml/practicefusion_sample.xml updated for exactly that
// reason: AuthorMappingRules' organizationLinkRow (declarative_oob_rules.go)
// now links the document author's representedOrganization via a new
// PractitionerRole + (when it doesn't dedup with an existing Organization,
// e.g. the custodian, by shared id) a new Organization — data
// patient_mapper.go's MapAuthor never read at all, so there is no prior Go
// count to preserve fidelity to here. mtuitive_sample.xml/kareo_sample.xml
// are unaffected (their authors carry no representedOrganization).
func TestDeclarativeMapDocument_Corpus_MatchesKnownShape(t *testing.T) {
	prevOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(prevOutput)

	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	ctx := context.Background()
	config := cdafhir.CDAToFHIRConfig{}

	for _, file := range corpusFiles {
		t.Run(file, func(t *testing.T) {
			want, ok := corpusExpectedShapes[file]
			if !ok {
				t.Fatalf("no corpusExpectedShapes entry for %q -- add one", file)
			}

			cdaDoc := loadTypedCDADocument(t, filepath.Join("corpus", file))
			out, err := mapper.DeclarativeMapDocument(ctx, cdaDoc, config)
			if err != nil {
				t.Fatalf("DeclarativeMapDocument: %v", err)
			}

			gotCounts := resourceTypeCounts(t, out.FHIRBundle)
			t.Logf("%s: resourceType counts: %v", file, gotCounts)

			for rt, wantCount := range want.resourceCounts {
				if gotCounts[rt] != wantCount {
					t.Errorf("%s: resourceType %q count = %d, want %d", file, rt, gotCounts[rt], wantCount)
				}
			}
			for rt, gotCount := range gotCounts {
				if want.resourceCounts[rt] != gotCount {
					t.Errorf("%s: resourceType %q count = %d, want %d", file, rt, gotCount, want.resourceCounts[rt])
				}
			}

			if out.ProcessingResult.ResourcesProduced != want.resourcesProduced {
				t.Errorf("%s: ResourcesProduced = %d, want %d", file, out.ProcessingResult.ResourcesProduced, want.resourcesProduced)
			}
			if len(out.ProcessingResult.SuccessfulSections) != want.successfulCount {
				t.Errorf("%s: len(SuccessfulSections) = %d, want %d (sections: %v)",
					file, len(out.ProcessingResult.SuccessfulSections), want.successfulCount,
					sortedCopy(out.ProcessingResult.SuccessfulSections))
			}
		})
	}
}

// TestDeclarativeMapDocument_Corpus_AllResourcesLinkedToPatient is Phase 4
// Slice A+B's combined proof at the orchestration level: every resource
// DeclarativeMapDocument() produces that has a PatientRefPath (everything
// except Patient/Author/Custodian/CareTeam's emitted Practitioner) actually
// carries a resolved "Patient/patient-1" reference once run through the
// real document-level orchestrator -- not just the per-rule engine-mechanics
// proof declarative_oob_rules_corpus_test.go already has, but confirmation
// the orchestrator wires PatientRef through correctly end-to-end.
func TestDeclarativeMapDocument_Corpus_AllResourcesLinkedToPatient(t *testing.T) {
	prevOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(prevOutput)

	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	ctx := context.Background()
	config := cdafhir.CDAToFHIRConfig{}

	patientLinkFields := []string{"subject", "patient", "beneficiary", "deliverFor"}

	for _, file := range corpusFiles {
		t.Run(file, func(t *testing.T) {
			cdaDoc := loadTypedCDADocument(t, filepath.Join("corpus", file))
			output, err := mapper.DeclarativeMapDocument(ctx, cdaDoc, config)
			if err != nil {
				t.Fatalf("DeclarativeMapDocument: %v", err)
			}

			entries, _ := output.FHIRBundle["entry"].([]interface{})
			checked := 0
			for _, e := range entries {
				entryMap, _ := e.(map[string]interface{})
				resource, _ := entryMap["resource"].(map[string]interface{})
				if resource == nil {
					continue
				}
				rt, _ := resource["resourceType"].(string)
				if rt == "Patient" || rt == "Practitioner" || rt == "Organization" {
					continue
				}
				for _, field := range patientLinkFields {
					if ref, ok := resource[field].(map[string]interface{}); ok {
						checked++
						if _, hasRef := ref["reference"]; !hasRef {
							t.Errorf("%s: %s/%s.%s has no reference key: %v", file, rt, resource["id"], field, ref)
						}
					}
				}
			}
			t.Logf("%s: checked %d patient-link fields across non-header resources", file, checked)
		})
	}
}

// TestDeclarativeMapDocument_EnabledSectionsSkipsDisabledSection guards the
// isSectionEnabled gate config.EnabledSections feeds (generic_mapper.go) —
// the part of the Section Field Editor redesign that was already wired
// correctly end-to-end; the bug being fixed was purely the frontend shape
// mismatch (CDAStepBuilder.js collectConfig), not this gate itself. Proves
// that explicitly omitting one dispatched section from EnabledSections
// removes every resource of that section's type from the output bundle while
// leaving every other dispatched section's resources untouched.
func TestDeclarativeMapDocument_EnabledSectionsSkipsDisabledSection(t *testing.T) {
	prevOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(prevOutput)

	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	ctx := context.Background()

	const disabled = "allergiesAndIntolerances"
	var enabled []string
	for _, k := range cdafhir.DeclarativeDispatchedSectionKeys() {
		if k != disabled {
			enabled = append(enabled, k)
		}
	}

	cdaDoc := loadTypedCDADocument(t, filepath.Join("corpus", "kareo_sample.xml"))

	baseline, err := mapper.DeclarativeMapDocument(ctx, cdaDoc, cdafhir.CDAToFHIRConfig{})
	if err != nil {
		t.Fatalf("DeclarativeMapDocument (baseline): %v", err)
	}
	baselineCounts := resourceTypeCounts(t, baseline.FHIRBundle)
	if baselineCounts["AllergyIntolerance"] == 0 {
		t.Fatal("baseline (EnabledSections unset) produced zero AllergyIntolerance resources — fixture no longer exercises this test's premise")
	}

	restricted, err := mapper.DeclarativeMapDocument(ctx, cdaDoc, cdafhir.CDAToFHIRConfig{EnabledSections: enabled})
	if err != nil {
		t.Fatalf("DeclarativeMapDocument (restricted): %v", err)
	}
	restrictedCounts := resourceTypeCounts(t, restricted.FHIRBundle)

	if restrictedCounts["AllergyIntolerance"] != 0 {
		t.Errorf("AllergyIntolerance count = %d, want 0 (section excluded from EnabledSections)", restrictedCounts["AllergyIntolerance"])
	}
	for rt, count := range baselineCounts {
		if rt == "AllergyIntolerance" {
			continue
		}
		if restrictedCounts[rt] != count {
			t.Errorf("resourceType %q count = %d, want %d (unaffected by excluding %q)", rt, restrictedCounts[rt], count, disabled)
		}
	}
}

// TestDeclarativeMapDocument_PatientExtensions_EndToEnd is the regression
// guard for the bug that prompted this whole group of changes: race/
// ethnicity/religion extensions and deceasedBoolean never reached the
// FHIR Patient resource DeclarativeMapDocument produces, because
// USCoreProfileBuilder.InjectPatientExtensions was never called from this
// orchestrator at all (only InjectProfile, meta.profile-only, was). None of
// the 4 vendor corpus fixtures exercise these fields, so this uses a minimal
// synthetic CDADocument instead of a testdata file.
func TestDeclarativeMapDocument_PatientExtensions_EndToEnd(t *testing.T) {
	prevOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(prevOutput)

	deceased := false
	cdaDoc := &cdadocument.CDADocument{
		Header: cdadocument.CDAHeader{
			Patient: cdadocument.CDAPatient{
				Names:       []cdadocument.CDAName{{Family: "Lelo", Given: []string{"Mary"}}},
				Gender:      cdadocument.CDACode{Code: "F"},
				DeceasedInd: &deceased,
				Race:        cdadocument.CDACode{Code: "2106-3", DisplayName: "White", CodeSystem: "2.16.840.1.113883.6.238"},
				Ethnicity:   cdadocument.CDACode{NullFlavor: "UNK"},
				Religion:    cdadocument.CDACode{Code: "1013", CodeSystem: "2.16.840.1.113883.5.1076"},
			},
		},
	}

	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	out, err := mapper.DeclarativeMapDocument(context.Background(), cdaDoc, cdafhir.CDAToFHIRConfig{})
	if err != nil {
		t.Fatalf("DeclarativeMapDocument: %v", err)
	}

	var patient map[string]interface{}
	entries, _ := out.FHIRBundle["entry"].([]interface{})
	for _, e := range entries {
		entryMap, _ := e.(map[string]interface{})
		resource, _ := entryMap["resource"].(map[string]interface{})
		if resource["resourceType"] == "Patient" {
			patient = resource
			break
		}
	}
	if patient == nil {
		t.Fatal("no Patient resource in bundle")
	}

	if patient["deceasedBoolean"] != false {
		t.Errorf("deceasedBoolean = %v, want false", patient["deceasedBoolean"])
	}

	exts, ok := patient["extension"].([]interface{})
	if !ok || len(exts) != 2 {
		t.Fatalf("extension = %v, want exactly 2 (race + religion; ethnicity skipped -- nullFlavor UNK has no code)", exts)
	}
	var urls []string
	for _, raw := range exts {
		if ext, ok := raw.(map[string]interface{}); ok {
			if url, ok := ext["url"].(string); ok {
				urls = append(urls, url)
			}
		}
	}
	if !containsStr(urls, "http://hl7.org/fhir/us/core/StructureDefinition/us-core-race") {
		t.Errorf("extension urls = %v, missing us-core-race", urls)
	}
	if !containsStr(urls, "http://hl7.org/fhir/StructureDefinition/patient-religion") {
		t.Errorf("extension urls = %v, missing patient-religion", urls)
	}
}

// TestDeclarativeMapDocument_EncompassingEncounterLocation_LinksToEncounter
// is the document-orchestration-level proof for
// EncompassingEncounterLocationMappingRules (declarative_oob_rules.go):
// componentOf/encompassingEncounter describes ONE overarching visit for the
// whole document, not a specific section entry, so the resulting Location
// resource gets linked from every Encounter resource that doesn't already
// carry a more specific location of its own -- a cross-cutting header-to-
// section wiring step this engine has no other example of yet, hence the
// dedicated end-to-end test (no corpus fixture exercises
// componentOf/encompassingEncounter at all).
func TestDeclarativeMapDocument_EncompassingEncounterLocation_LinksToEncounter(t *testing.T) {
	prevOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(prevOutput)

	header := cdadocument.CDAHeader{
		EncompassingEncounter: &cdadocument.CDAEncounter{
			Location: &cdadocument.CDALocation{
				HealthCareFacility: cdadocument.CDAHealthCareFacility{
					Location: &cdadocument.CDAPlace{
						Names: []cdadocument.CDAName{{Family: "Mumbai Women's Care"}},
					},
				},
			},
		},
	}

	cdaDoc := &cdadocument.CDADocument{
		Header: header,
		SectionsByKey: map[string]*cdadocument.CDASection{
			"encounters": {
				Entries: []cdadocument.CDAEntry{
					{EntryType: "encounter"}, // no location of its own
				},
			},
		},
	}

	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	out, err := mapper.DeclarativeMapDocument(context.Background(), cdaDoc, cdafhir.CDAToFHIRConfig{})
	if err != nil {
		t.Fatalf("DeclarativeMapDocument: %v", err)
	}

	var location, encounter map[string]interface{}
	var locationFullURL string
	entries, _ := out.FHIRBundle["entry"].([]interface{})
	for _, e := range entries {
		entryMap, _ := e.(map[string]interface{})
		resource, _ := entryMap["resource"].(map[string]interface{})
		switch resource["resourceType"] {
		case "Location":
			location = resource
			locationFullURL, _ = entryMap["fullUrl"].(string)
		case "Encounter":
			encounter = resource
		}
	}

	if location == nil {
		t.Fatal("no Location resource in bundle")
	}
	if location["id"] != "location-1" {
		t.Errorf("Location.id = %v, want location-1", location["id"])
	}
	if location["name"] != "Mumbai Women's Care" {
		t.Errorf("Location.name = %v, want %q", location["name"], "Mumbai Women's Care")
	}

	if encounter == nil {
		t.Fatal("no Encounter resource in bundle")
	}
	locArr, ok := encounter["location"].([]interface{})
	if !ok || len(locArr) != 1 {
		t.Fatalf("Encounter.location = %v, want a 1-element array", encounter["location"])
	}
	locEntry, _ := locArr[0].(map[string]interface{})
	ref, _ := locEntry["location"].(map[string]interface{})
	// "Location/location-1" is only the intermediate form -- final bundle
	// assembly (fhir/r4/bundle_assembler.go's RewriteReferences) rewrites
	// every "ResourceType/id" reference to the matching urn:uuid: fullUrl
	// (FHIR R4 §3.3.2.1: a urn:uuid: fullUrl entry's incoming references
	// must use the same urn:uuid: form), so the only stable assertion here
	// is "matches the Location entry's own fullUrl", not the literal string.
	if ref["reference"] != locationFullURL {
		t.Errorf("Encounter.location[0].location.reference = %v, want %q (the Location entry's fullUrl)", ref["reference"], locationFullURL)
	}
}

// TestDeclarativeMapDocument_EncompassingEncounterLocation_DoesNotOverwriteEntrySpecificLocation
// proves the more-specific section-entry-level LOC participant (Encounter's
// own "entryRelationships[typeCode=COMP].entry.participants[typeCode=LOC]"
// row) wins over the document-wide encompassingEncounter facility — the
// header-level link is a fallback, not an override. Also exercises the
// EncompassingEncounterMappingRules consolidation gate (this fixture's
// EncompassingEncounter carries only a location, no id/period/
// hospitalization): asserting exactly one Encounter resource here is what
// caught the gate's absence in the first place -- without it, this header
// data would have spawned a near-duplicate, no-new-information standalone
// Encounter alongside the real one.
func TestDeclarativeMapDocument_EncompassingEncounterLocation_DoesNotOverwriteEntrySpecificLocation(t *testing.T) {
	prevOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(prevOutput)

	header := cdadocument.CDAHeader{
		EncompassingEncounter: &cdadocument.CDAEncounter{
			Location: &cdadocument.CDALocation{
				HealthCareFacility: cdadocument.CDAHealthCareFacility{
					Location: &cdadocument.CDAPlace{
						Names: []cdadocument.CDAName{{Family: "Mumbai Women's Care"}},
					},
				},
			},
		},
	}

	cdaDoc := &cdadocument.CDADocument{
		Header: header,
		SectionsByKey: map[string]*cdadocument.CDASection{
			"encounters": {
				Entries: []cdadocument.CDAEntry{
					{
						EntryType: "encounter",
						EntryRelationships: []cdadocument.CDAEntryRelationship{
							{
								TypeCode: "COMP",
								Entry: cdadocument.CDAEntry{
									Participants: []cdadocument.CDAParticipant{
										{
											TypeCode: "LOC",
											ParticipantRole: cdadocument.CDAParticipantRole{
												PlayingEntity: &cdadocument.CDAPlayingEntity{
													Names: []cdadocument.CDAName{{Family: "Specific Visit Clinic"}},
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

	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	out, err := mapper.DeclarativeMapDocument(context.Background(), cdaDoc, cdafhir.CDAToFHIRConfig{})
	if err != nil {
		t.Fatalf("DeclarativeMapDocument: %v", err)
	}

	var encounters []map[string]interface{}
	entries, _ := out.FHIRBundle["entry"].([]interface{})
	for _, e := range entries {
		entryMap, _ := e.(map[string]interface{})
		resource, _ := entryMap["resource"].(map[string]interface{})
		if resource["resourceType"] == "Encounter" {
			encounters = append(encounters, resource)
		}
	}
	if len(encounters) != 1 {
		t.Fatalf("got %d Encounter resources, want exactly 1 (the EncompassingEncounterMappingRules consolidation gate should have suppressed a near-duplicate standalone Encounter for a header with only a location)", len(encounters))
	}
	encounter := encounters[0]
	locArr, ok := encounter["location"].([]interface{})
	if !ok || len(locArr) != 1 {
		t.Fatalf("Encounter.location = %v, want a 1-element array", encounter["location"])
	}
	locEntry, _ := locArr[0].(map[string]interface{})
	ref, _ := locEntry["location"].(map[string]interface{})
	display, _ := ref["display"].(string)
	if display != "Specific Visit Clinic" {
		t.Errorf("Encounter.location[0].location = %v, want the entry-specific display %q (not the document-wide facility)", ref, "Specific Visit Clinic")
	}
}

// TestDeclarativeMapDocument_EncounterDiagnosis_ConditionGetsEncounterBackReference
// is the document-orchestration-level proof for the Condition.encounter
// back-reference added to DeclarativeMapDocument (the gap found while
// auditing Encounter/Condition linkage): Encounter.diagnosis[].condition
// already pointed forward at the Condition; this proves the reverse link
// gets set too, and only after both resources have their final ids.
func TestDeclarativeMapDocument_EncounterDiagnosis_ConditionGetsEncounterBackReference(t *testing.T) {
	prevOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(prevOutput)

	cdaDoc := &cdadocument.CDADocument{
		SectionsByKey: map[string]*cdadocument.CDASection{
			"encounters": {
				Entries: []cdadocument.CDAEntry{
					{
						EntryType: "encounter",
						EntryRelationships: []cdadocument.CDAEntryRelationship{
							{
								TypeCode: "SUBJ",
								Entry: cdadocument.CDAEntry{ // Problem Concern Act
									EntryRelationships: []cdadocument.CDAEntryRelationship{
										{
											TypeCode: "SUBJ",
											Entry: cdadocument.CDAEntry{ // Problem Observation
												Code: cdadocument.CDACode{
													Code: "38341003", DisplayName: "Hypertension",
													CodeSystem: "2.16.840.1.113883.6.96",
												},
												StatusCode: "completed",
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

	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	out, err := mapper.DeclarativeMapDocument(context.Background(), cdaDoc, cdafhir.CDAToFHIRConfig{})
	if err != nil {
		t.Fatalf("DeclarativeMapDocument: %v", err)
	}

	var encounter, condition map[string]interface{}
	var encounterFullURL, conditionFullURL string
	entries, _ := out.FHIRBundle["entry"].([]interface{})
	for _, e := range entries {
		entryMap, _ := e.(map[string]interface{})
		resource, _ := entryMap["resource"].(map[string]interface{})
		switch resource["resourceType"] {
		case "Encounter":
			encounter = resource
			encounterFullURL, _ = entryMap["fullUrl"].(string)
		case "Condition":
			condition = resource
			conditionFullURL, _ = entryMap["fullUrl"].(string)
		}
	}
	if encounter == nil {
		t.Fatal("no Encounter resource in bundle")
	}
	if condition == nil {
		t.Fatal("no Condition resource in bundle")
	}

	// "Condition/<id>"/"Encounter/<id>" are only the intermediate form --
	// final bundle assembly (fhir/r4/bundle_assembler.go's
	// RewriteReferences) rewrites every "ResourceType/id" reference to the
	// matching urn:uuid: fullUrl, so the only stable assertion is against
	// each entry's own fullUrl (see the EncompassingEncounterLocation test
	// above for the same lesson).
	locArr, ok := encounter["diagnosis"].([]interface{})
	if !ok || len(locArr) != 1 {
		t.Fatalf("Encounter.diagnosis = %v, want a 1-element array", encounter["diagnosis"])
	}
	diagEntry, _ := locArr[0].(map[string]interface{})
	condRef, _ := diagEntry["condition"].(map[string]interface{})
	if condRef["reference"] != conditionFullURL {
		t.Fatalf("Encounter.diagnosis[0].condition.reference = %v, want %q (the Condition entry's fullUrl)", condRef["reference"], conditionFullURL)
	}

	encRef, ok := condition["encounter"].(map[string]interface{})
	if !ok {
		t.Fatalf("Condition.encounter = %v, want a reference object", condition["encounter"])
	}
	if encRef["reference"] != encounterFullURL {
		t.Errorf("Condition.encounter.reference = %v, want %q (the Encounter entry's fullUrl)", encRef["reference"], encounterFullURL)
	}
}

// TestDeclarativeMapDocument_EncompassingEncounter_ConsolidatesWithMatchingInSectionEncounter
// proves the HL7 C-CDA on FHIR IG's consolidation rule (verified via
// WebFetch 2026-06-27): when componentOf/encompassingEncounter shares the
// same CDA <id> as an in-section Encounter Activity, exactly ONE Encounter
// resource results, carrying the in-section side's own fields (status,
// class, etc.) AND the header-only field (hospitalization) neither side
// alone has.
func TestDeclarativeMapDocument_EncompassingEncounter_ConsolidatesWithMatchingInSectionEncounter(t *testing.T) {
	prevOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(prevOutput)

	sharedID := cdadocument.CDAII{Root: "2.16.840.1.113883.19", Extension: "ENC-1"}

	cdaDoc := &cdadocument.CDADocument{
		Header: cdadocument.CDAHeader{
			EncompassingEncounter: &cdadocument.CDAEncounter{
				Id:                       sharedID,
				DischargeDispositionCode: cdadocument.CDACode{Code: "01", DisplayName: "Discharged to home", CodeSystem: "2.16.840.1.113883.6.301.5"},
			},
		},
		SectionsByKey: map[string]*cdadocument.CDASection{
			"encounters": {
				Entries: []cdadocument.CDAEntry{
					{
						EntryType:  "encounter",
						Id:         []cdadocument.CDAII{sharedID},
						StatusCode: "completed",
					},
				},
			},
		},
	}

	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	out, err := mapper.DeclarativeMapDocument(context.Background(), cdaDoc, cdafhir.CDAToFHIRConfig{})
	if err != nil {
		t.Fatalf("DeclarativeMapDocument: %v", err)
	}

	var encounters []map[string]interface{}
	entries, _ := out.FHIRBundle["entry"].([]interface{})
	for _, e := range entries {
		entryMap, _ := e.(map[string]interface{})
		resource, _ := entryMap["resource"].(map[string]interface{})
		if resource["resourceType"] == "Encounter" {
			encounters = append(encounters, resource)
		}
	}
	if len(encounters) != 1 {
		t.Fatalf("got %d Encounter resources, want exactly 1 (consolidated, not duplicated)", len(encounters))
	}
	enc := encounters[0]

	// The in-section side's own status must be authoritative -- the header
	// candidate's row deliberately is NOT copied across on a match (see
	// EncompassingEncounterMappingRules' own doc comment). "finished" (not
	// "completed") is encounter_status_to_fhir's correct mapping for CDA's
	// "completed" -- a different transform's output, not this test's
	// concern, but the right value to assert against.
	if enc["status"] != "finished" {
		t.Errorf("status = %v, want finished (the in-section Encounter's own status, not overwritten by the header candidate)", enc["status"])
	}

	hosp, ok := enc["hospitalization"].(map[string]interface{})
	if !ok {
		t.Fatalf("hospitalization = %v, want an object (the one field only encompassingEncounter provides)", enc["hospitalization"])
	}
	dd, _ := hosp["dischargeDisposition"].(map[string]interface{})
	coding := firstElement(t, dd["coding"]).(map[string]interface{})
	if coding["display"] != "Discharged to home" {
		t.Errorf("hospitalization.dischargeDisposition display = %v, want %q", coding["display"], "Discharged to home")
	}

	identArr, _ := enc["identifier"].([]interface{})
	if len(identArr) != 1 {
		t.Errorf("identifier count = %d, want exactly 1 (both sides resolve the SAME CDA <id> -- must not duplicate)", len(identArr))
	}
}

// TestDeclarativeMapDocument_EncompassingEncounter_NoMatch_BecomesStandaloneEncounter
// proves the IG's other instruction applies when no in-section Encounter
// shares the candidate's id: componentOf/encompassingEncounter still
// becomes its own Encounter resource rather than being dropped.
func TestDeclarativeMapDocument_EncompassingEncounter_NoMatch_BecomesStandaloneEncounter(t *testing.T) {
	prevOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(prevOutput)

	cdaDoc := &cdadocument.CDADocument{
		Header: cdadocument.CDAHeader{
			EncompassingEncounter: &cdadocument.CDAEncounter{
				Id: cdadocument.CDAII{Root: "2.16.840.1.113883.19", Extension: "HEADER-ENC"},
			},
		},
		// No "encounters" section at all -- nothing for the header
		// candidate to consolidate with.
	}

	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	out, err := mapper.DeclarativeMapDocument(context.Background(), cdaDoc, cdafhir.CDAToFHIRConfig{})
	if err != nil {
		t.Fatalf("DeclarativeMapDocument: %v", err)
	}

	var encounters []map[string]interface{}
	entries, _ := out.FHIRBundle["entry"].([]interface{})
	for _, e := range entries {
		entryMap, _ := e.(map[string]interface{})
		resource, _ := entryMap["resource"].(map[string]interface{})
		if resource["resourceType"] == "Encounter" {
			encounters = append(encounters, resource)
		}
	}
	if len(encounters) != 1 {
		t.Fatalf("got %d Encounter resources, want exactly 1 (standalone, from encompassingEncounter alone)", len(encounters))
	}
	if encounters[0]["status"] != "unknown" {
		t.Errorf("status = %v, want unknown", encounters[0]["status"])
	}
	ident := firstElement(t, encounters[0]["identifier"]).(map[string]interface{})
	if ident["value"] != "HEADER-ENC" {
		t.Errorf("identifier[0].value = %v, want HEADER-ENC", ident["value"])
	}
}

func containsStr(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}

// TestDeclarativeMapDocument_AdditionalValues_SurfacesInMappingLog is the
// document-orchestration-level proof for the warning -> Mapping Log wiring
// (declarative_document_mapper.go's per-entry loop, sb.AddWarning) added
// alongside additionalValuesWarningMessages (declarative_engine.go) and
// SectionLog.Warnings (mapping_log/section_log.go). The engine-level proof
// (declarative_engine_test.go's TestDeclarativeEngine_AdditionalValues_
// SurfacesWarning_KeepsFirstValue) only exercises BuildResources directly
// against a raw map and never reaches this orchestrator at all -- this test
// is what confirms a real CDADocument's warning actually lands on
// MappingLog.Sections[i].Warnings, the exact field the message detail
// screen's Mapping Log tab reads (public/js/messages.js's
// _renderMappingLog) for LIVE production messages, not just manual Test
// Pipeline runs.
func TestDeclarativeMapDocument_AdditionalValues_SurfacesInMappingLog(t *testing.T) {
	prevOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(prevOutput)

	score := 0
	cdaDoc := &cdadocument.CDADocument{
		SectionsByKey: map[string]*cdadocument.CDASection{
			"functionalStatus": {
				TemplateIds: []string{"2.16.840.1.113883.10.20.22.2.14"},
				Title:       "Functional Status",
				Entries: []cdadocument.CDAEntry{
					{
						EntryType:  "observation",
						Code:       cdadocument.CDACode{Code: "44261-6", DisplayName: "Patient Health Questionnaire 2 item total score", CodeSystem: "2.16.840.1.113883.6.1"},
						StatusCode: "completed",
						Value:      &cdadocument.CDAValue{Type: "INT", Integer: &score},
						AdditionalValues: []cdadocument.CDAValue{
							{Type: "CD", Code: &cdadocument.CDACode{Code: "428171000124102", DisplayName: "Depression screening negative (finding)"}},
						},
					},
				},
			},
		},
	}

	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	out, err := mapper.DeclarativeMapDocument(context.Background(), cdaDoc, cdafhir.CDAToFHIRConfig{})
	if err != nil {
		t.Fatalf("DeclarativeMapDocument: %v", err)
	}

	var fsLog *mappinglog.SectionLog
	for i, s := range out.MappingLog.Sections {
		if s.SectionKey == "functionalStatus" {
			fsLog = &out.MappingLog.Sections[i]
			break
		}
	}
	if fsLog == nil {
		t.Fatal("no functionalStatus entry in MappingLog.Sections")
	}
	if len(fsLog.Warnings) != 1 {
		t.Fatalf("MappingLog functionalStatus warnings = %v, want exactly 1", fsLog.Warnings)
	}
	if !strings.Contains(fsLog.Warnings[0], "428171000124102") || !strings.Contains(fsLog.Warnings[0], "Depression screening negative") {
		t.Errorf("warning = %q, want it to name the dropped value's code+display", fsLog.Warnings[0])
	}
	if len(fsLog.Errors) != 0 {
		t.Errorf("MappingLog functionalStatus errors = %v, want none -- a warning must never populate Errors", fsLog.Errors)
	}

	// The bundle itself must be unaffected -- first value still wins.
	var fsObs map[string]interface{}
	entries, _ := out.FHIRBundle["entry"].([]interface{})
	for _, e := range entries {
		entryMap, _ := e.(map[string]interface{})
		resource, _ := entryMap["resource"].(map[string]interface{})
		if resource["resourceType"] != "Observation" {
			continue
		}
		if cat, ok := resource["category"].([]interface{}); ok {
			for _, c := range cat {
				cm, _ := c.(map[string]interface{})
				codings, _ := cm["coding"].([]interface{})
				for _, co := range codings {
					com, _ := co.(map[string]interface{})
					if com["code"] == "functional-status" {
						fsObs = resource
					}
				}
			}
		}
	}
	if fsObs == nil {
		t.Fatal("no functional-status category Observation in bundle")
	}
	if fsObs["valueInteger"] != float64(0) {
		t.Errorf("valueInteger = %v, want 0 (first value kept, unchanged behavior)", fsObs["valueInteger"])
	}
}
