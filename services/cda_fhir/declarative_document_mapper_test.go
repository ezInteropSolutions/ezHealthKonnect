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
	"strconv"
	"strings"
	"testing"

	cdaSchema "ezhealthkonnect/cda"
	cdadocument "ezhealthkonnect/cda/document"
	cdafhir "ezhealthkonnect/services/cda_fhir"
	mappinglog "ezhealthkonnect/services/cda_fhir/mapping_log"
	"ezhealthkonnect/services/executors"

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
		// +3 DocumentReference (chiefComplaint/functionalStatus/planOfTreatment
		// narrative, +3 resources, +3 sections) -- the section-level narrative
		// dispatch pass now produces a DocumentReference for every section
		// with narrative text and no entries, not just a fixed registry;
		// kareo_sample.xml has three such sections. Regenerated from a real
		// run (see TestDeclarativeMapDocument_Corpus_MatchesKnownShape's doc
		// comment); cerner_sample.xml is unaffected (no qualifying sections)
		// and was left untouched as proof of zero drift elsewhere.
		resourceCounts: map[string]int{
			"AllergyIntolerance": 1, "Condition": 2, "DocumentReference": 3, "Encounter": 1, "Immunization": 1,
			"MedicationStatement": 2, "Observation": 6, "Organization": 1, "Patient": 1,
			"Practitioner": 1, "Procedure": 1,
		},
		resourcesProduced: 20,
		successfulCount:   12,
	},
	"mtuitive_sample.xml": {
		// +5 DocumentReference (assistants/dateOfSurgery/operationsPerformed/
		// signature/surgeon narrative sections), same section-level narrative
		// dispatch broadening as kareo_sample.xml above -- mtuitive_sample.xml
		// has no entry-bearing sections at all, so every one of its 5
		// narrative sections now qualifies.
		resourceCounts: map[string]int{
			"DocumentReference": 5, "Organization": 1, "Patient": 1, "Practitioner": 1,
		},
		resourcesProduced: 8,
		successfulCount:   5,
	},
	"practicefusion_sample.xml": {
		// +5 DocumentReference (assessmentAndPlan/chiefComplaint/
		// functionalStatus/instructions/reasonForReferral/vitalSigns narrative
		// sections -- same broadening as above) and +2 Practitioner
		// (ProblemsMappingRules' asserter rows now emit a Practitioner
		// sub-resource per distinct asserter, via EmitAsResource -- see
		// BuildResources' doc comment) relative to this map's last freeze.
		resourceCounts: map[string]int{
			"AllergyIntolerance": 2, "Condition": 2, "DocumentReference": 6, "Encounter": 1, "Immunization": 1,
			"MedicationStatement": 1, "Observation": 1, "Organization": 2, "Patient": 1,
			"Practitioner": 3, "PractitionerRole": 1, "Procedure": 1,
		},
		resourcesProduced: 22,
		successfulCount:   14,
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

	cdaDoc := loadTypedCDADocument(t, filepath.Join("corpus", "kareo_sample.xml"))

	// Built from this fixture's own SectionsByKey (not
	// DeclarativeDispatchedSectionKeys, which only lists entry-rule-registered
	// sections) because DeclarativeMapDocument's narrative-DocumentReference
	// pass runs for every section with narrative text and no entries too --
	// including sections with no MappingRule at all (e.g. kareo_sample.xml's
	// narrative-only chiefComplaint). Restricting EnabledSections must only
	// exclude the one section this test targets, not silently disable those too.
	var enabled []string
	for k := range cdaDoc.SectionsByKey {
		if k != disabled {
			enabled = append(enabled, k)
		}
	}

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

	// The same warning must also land on the specific entry's own EntryTrace
	// (not just the section-level aggregate above) -- this is what lets the
	// Mapping Log UI show the warning directly on the entry row it belongs
	// to, instead of only as a disconnected section-level count. See
	// EntryTrace.Warnings' doc comment (mapping_log/section_log.go).
	if len(fsLog.Entries) != 1 {
		t.Fatalf("MappingLog functionalStatus entries = %v, want exactly 1", fsLog.Entries)
	}
	entryWarnings := fsLog.Entries[0].Warnings
	if len(entryWarnings) != 1 {
		t.Fatalf("MappingLog functionalStatus entry[0].Warnings = %v, want exactly 1", entryWarnings)
	}
	if !strings.Contains(entryWarnings[0], "428171000124102") || !strings.Contains(entryWarnings[0], "Depression screening negative") {
		t.Errorf("entry warning = %q, want it to name the dropped value's code+display", entryWarnings[0])
	}
	if strings.HasPrefix(entryWarnings[0], "entry ") {
		t.Errorf("entry warning = %q, should not repeat the section-level \"entry N, \" prefix -- it's already scoped to this entry", entryWarnings[0])
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

// TestDeclarativeMapDocument_CoverageTracker_RecordsEachEntryDistinctly is
// the regression proof for a bug where every entry beyond the first in a
// section was recorded as "sectionKey#0" no matter its real position,
// because buildOneResource's own entryIdx parameter is only ever correct
// relative to declarative_document_mapper.go's per-entry synthetic
// single-entry wrapper documentMap (see that loop's own comment) -- from
// inside buildOneResource EVERY entry looks like "entry 0", regardless of
// where it actually sits in the section. CDA Coverage Audit reports built
// from this (services/cda_coverage/report.go) reported every entry past the
// first as "never touched" even though mapping succeeded -- exactly what a
// real 99397 CCD sample showed: Functional Status 1/14 entries "found", 13
// false-positive gaps, despite the Mapping Log confirming all 14 were
// mapped to real Observations. Fixed by moving the recording call to
// declarative_document_mapper.go's outer loop, the one place that has the
// real, section-relative entryIdx.
func TestDeclarativeMapDocument_CoverageTracker_RecordsEachEntryDistinctly(t *testing.T) {
	prevOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(prevOutput)

	score0, score1, score2 := 0, 1, 2
	cdaDoc := &cdadocument.CDADocument{
		SectionsByKey: map[string]*cdadocument.CDASection{
			"functionalStatus": {
				TemplateIds: []string{"2.16.840.1.113883.10.20.22.2.14"},
				Title:       "Functional Status",
				Entries: []cdadocument.CDAEntry{
					{
						EntryType:  "observation",
						Code:       cdadocument.CDACode{Code: "54522-8", DisplayName: "Functional status", CodeSystem: "2.16.840.1.113883.6.1"},
						StatusCode: "completed",
						Value:      &cdadocument.CDAValue{Type: "INT", Integer: &score0},
					},
					{
						EntryType:  "observation",
						Code:       cdadocument.CDACode{Code: "54522-8", DisplayName: "Functional status", CodeSystem: "2.16.840.1.113883.6.1"},
						StatusCode: "completed",
						Value:      &cdadocument.CDAValue{Type: "INT", Integer: &score1},
					},
					{
						EntryType:  "observation",
						Code:       cdadocument.CDACode{Code: "54522-8", DisplayName: "Functional status", CodeSystem: "2.16.840.1.113883.6.1"},
						StatusCode: "completed",
						Value:      &cdadocument.CDAValue{Type: "INT", Integer: &score2},
					},
				},
			},
		},
	}

	tracker := executors.NewCDACoverageTracker()
	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	_, err := mapper.DeclarativeMapDocument(context.Background(), cdaDoc, cdafhir.CDAToFHIRConfig{CoverageTracker: tracker})
	if err != nil {
		t.Fatalf("DeclarativeMapDocument: %v", err)
	}

	for i := 0; i < 3; i++ {
		key := executors.CDAEntryKey("functionalStatus", i)
		if !tracker.Touched(key) {
			t.Errorf("tracker.Touched(%q) = false, want true -- entry %d was never recorded as its own distinct key", key, i)
		}
	}
	// Exactly 3 SECTION entry-level keys (no "/elementPath" suffix) -- the
	// specific guarantee this test proves. Element-level keys
	// (functionalStatus#0/code, .../value[0], etc.) are also expected in the
	// snapshot now that CDA Coverage Audit's element-granularity recording
	// (see declarative_engine.go's recordElementCoverage) is wired into the
	// same plain-value applyRow branch this fixture's rows all use --
	// counted here only to confirm they're present and correctly
	// per-entry-distinct too (not asserting an exact total, which would make
	// this test brittle against future rule/field additions to
	// FunctionalStatusMappingRules). "header."-prefixed keys are filtered
	// out entirely -- this fixture's zero-value Header still resolves
	// CDAPatient/CDACustodian (non-pointer struct fields on CDAHeader,
	// always present even zero-valued, unlike EncompassingEncounter/Authors
	// which are pointer/slice fields that correctly produce nothing when
	// absent), so header.patient#0/header.custodian#0 legitimately appear
	// too -- real, correct "record on attempt" behavior (see
	// BuildHeaderResourceWithCoveragePrefix), just outside THIS test's own
	// stated concern (section-entry distinctness only).
	snapshot := tracker.Snapshot()
	entryLevelKeys := 0
	elementLevelKeys := 0
	for key := range snapshot {
		if strings.HasPrefix(key, "header.") {
			continue
		}
		if strings.Contains(key, "/") {
			elementLevelKeys++
			continue
		}
		entryLevelKeys++
	}
	if entryLevelKeys != 3 {
		t.Errorf("tracker recorded %d section entry-level key(s), want exactly 3 (one per entry) -- got %v", entryLevelKeys, snapshot)
	}
	if elementLevelKeys == 0 {
		t.Errorf("tracker recorded 0 element-level keys -- expected at least one per entry (code/statusCode/effectiveTime/value are all plain CDAEntry fields this fixture sets)")
	}
}

// TestDeclarativeMapDocument_CoverageTracker_ElementLevel_VitalSigns is CDA
// Coverage Audit Phase 1's actual proof of the feature this whole effort was
// escalated for (see this session's plan, "element-level tracking"): not
// just "was this entry touched," but "which of its OWN elements were." Vital
// Signs (observationRule(), declarative_oob_rules.go) is the Phase 1 pilot
// section specifically because its rules never cross an entryRelationship/
// component boundary (confirmed by reading the rule) -- clean of the
// data-dependent act-child case Phase 2 is scoped to handle instead.
//
// The negative assertion is the real proof, not the positive ones: this
// fixture's entry carries a genuine, non-empty Performers value, but
// observationRule() only ever maps Authors (via assignedEntityRoleRow) --
// it has no row that reads Performers at all. If element-level recording
// were accidentally too permissive (e.g. recording the whole entry as
// "touched" and letting every field ride along), this would incorrectly
// pass. It must specifically know Performers was never read.
func TestDeclarativeMapDocument_CoverageTracker_ElementLevel_VitalSigns(t *testing.T) {
	prevOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(prevOutput)

	cdaDoc := &cdadocument.CDADocument{
		SectionsByKey: map[string]*cdadocument.CDASection{
			"vitalSigns": {
				TemplateIds: []string{"2.16.840.1.113883.10.20.22.2.4"},
				Title:       "Vital Signs",
				Entries: []cdadocument.CDAEntry{
					{
						EntryType:  "observation",
						Code:       cdadocument.CDACode{Code: "8867-4", DisplayName: "Heart rate", CodeSystem: "2.16.840.1.113883.6.1"},
						StatusCode: "completed",
						Value:      &cdadocument.CDAValue{Type: "PQ", Quantity: &cdadocument.CDAQuantity{Value: "72", Unit: "/min"}},
						// Never read by observationRule() -- see this test's
						// own doc comment. Populated with real, non-empty
						// data specifically so a false "touched" would be
						// possible if element-level recording were too
						// permissive.
						Performers: []cdadocument.CDAPerformer{
							{
								TypeCode: "PRF",
								AssignedEntity: cdadocument.CDAAssignedEntity{
									Code: cdadocument.CDACode{Code: "163W00000X", DisplayName: "Registered Nurse"},
								},
							},
						},
					},
				},
			},
		},
	}

	tracker := executors.NewCDACoverageTracker()
	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	_, err := mapper.DeclarativeMapDocument(context.Background(), cdaDoc, cdafhir.CDAToFHIRConfig{CoverageTracker: tracker})
	if err != nil {
		t.Fatalf("DeclarativeMapDocument: %v", err)
	}

	snapshot := tracker.Snapshot()

	// Positive: fields observationRule() genuinely reads must be recorded
	// at element granularity, each under the entry's own key. Raw XML's
	// <entry> directly wraps this entry's own act tag -- "observation" here
	// (EntryType above) -- which the typed parser flattens away; recorded
	// keys must include that "observation[0]." prefix (executors.
	// CDAEntryActTagPrefix) to match inventory.go's walkEntryElements,
	// which walks the unflattened raw XML and sees the wrapper as a real
	// level (e.g. "observation[0].code[0]", never bare "code[0]").
	for _, wantSuffix := range []string{"observation[0].code", "observation[0].statusCode", "observation[0].value"} {
		found := false
		for key := range snapshot {
			if strings.HasPrefix(key, "vitalSigns#0") && strings.Contains(key, wantSuffix) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("no recorded key matches \"vitalSigns#0...%s\" -- got %v", wantSuffix, snapshot)
		}
	}

	// Negative: the real proof. Performers has genuine data on this entry
	// but no rule in observationRule() ever reads it.
	for key := range snapshot {
		if strings.Contains(key, "performer") {
			t.Errorf("performer element incorrectly recorded as touched: %q (observationRule() never reads Performers) -- full snapshot %v", key, snapshot)
		}
	}
}

// TestDeclarativeMapDocument_CoverageTracker_ElementLevel_RecursiveActChild is
// Phase 2's proof: element-level recording must work irrespective of which
// section it is, including the hard case Phase 1 deliberately avoided --
// entryRelationship-crossing rules where the nested act's own XML tag
// (<observation>/<act>/<substanceAdministration>/...) can only be known from
// the already-resolved node's own EntryType at walk time (xlActChild /
// xlRenamedActChild in cda_element_translation.go), not from the mapping
// path string alone. FunctionalStatusMappingRules()'s
// assessmentScaleSupportingObservationRow (declarative_oob_rules.go) is a
// real OOB rule with exactly this shape: Scope
// "entryRelationships[typeCode=COMP].entry[templateId=...]" +
// CollectAll+EmitAsResource. Negative assertion reuses Performers, the same
// field the Vital Signs test (Phase 1) already proved is never read by any
// rule in this family.
func TestDeclarativeMapDocument_CoverageTracker_ElementLevel_RecursiveActChild(t *testing.T) {
	prevOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(prevOutput)

	nestedScore0 := 3
	nestedScore1 := 4

	cdaDoc := &cdadocument.CDADocument{
		SectionsByKey: map[string]*cdadocument.CDASection{
			"functionalStatus": {
				TemplateIds: []string{"2.16.840.1.113883.10.20.22.2.5.1"},
				Title:       "Functional Status",
				Entries: []cdadocument.CDAEntry{
					{
						EntryType:  "observation",
						Code:       cdadocument.CDACode{Code: "54522-8", DisplayName: "Functional status", CodeSystem: "2.16.840.1.113883.6.1"},
						StatusCode: "completed",
						Value:      &cdadocument.CDAValue{Type: "CD", Code: &cdadocument.CDACode{Code: "LA6115-2", DisplayName: "Able to feed self", CodeSystem: "2.16.840.1.113883.6.1"}},
						EntryRelationships: []cdadocument.CDAEntryRelationship{
							{
								TypeCode: "COMP",
								Entry: cdadocument.CDAEntry{
									EntryType:   "observation",
									TemplateIds: []string{"2.16.840.1.113883.10.20.22.4.86"},
									Code:        cdadocument.CDACode{Code: "72108-1", DisplayName: "Bathing", CodeSystem: "2.16.840.1.113883.6.1"},
									StatusCode:  "completed",
									Value:       &cdadocument.CDAValue{Type: "INT", Integer: &nestedScore0},
									// Genuine data, but assessmentScaleSupportingObservationRow's
									// Fields never include a Performers row -- see this
									// test's negative assertion below (same field Phase
									// 1's Vital Signs test already proved is never read).
									Performers: []cdadocument.CDAPerformer{
										{TypeCode: "PRF", AssignedEntity: cdadocument.CDAAssignedEntity{Code: cdadocument.CDACode{Code: "163W00000X", DisplayName: "Registered Nurse"}}},
									},
								},
							},
							{
								TypeCode: "COMP",
								Entry: cdadocument.CDAEntry{
									EntryType:   "observation",
									TemplateIds: []string{"2.16.840.1.113883.10.20.22.4.86"},
									Code:        cdadocument.CDACode{Code: "72110-7", DisplayName: "Dressing upper body", CodeSystem: "2.16.840.1.113883.6.1"},
									StatusCode:  "completed",
									Value:       &cdadocument.CDAValue{Type: "INT", Integer: &nestedScore1},
									Performers: []cdadocument.CDAPerformer{
										{TypeCode: "PRF", AssignedEntity: cdadocument.CDAAssignedEntity{Code: cdadocument.CDACode{Code: "163W00000X", DisplayName: "Registered Nurse"}}},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	tracker := executors.NewCDACoverageTracker()
	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	_, err := mapper.DeclarativeMapDocument(context.Background(), cdaDoc, cdafhir.CDAToFHIRConfig{CoverageTracker: tracker})
	if err != nil {
		t.Fatalf("DeclarativeMapDocument: %v", err)
	}

	snapshot := tracker.Snapshot()

	// The outer entry's own top-level key must still be recorded --
	// unaffected by anything nested below it.
	if !tracker.Touched(executors.CDAEntryKey("functionalStatus", 0)) {
		t.Errorf("outer entry key functionalStatus#0 not recorded -- got %v", snapshot)
	}

	// Positive: the nested act-child's own fields must resolve through the
	// act-tag insertion (observation[0], data-dependent on the nested
	// entry's own EntryType), under BOTH sibling entryRelationships --
	// proving childCoverageContext's index-zip isn't accidentally only
	// recording the first CollectAll match. Leading "observation[0]." is
	// the OUTER entry's own act-tag wrapper (EntryType="observation" above,
	// see executors.CDAEntryActTagPrefix) -- every key starts there, not at
	// the entry root, since raw XML's <entry> wraps it too.
	for _, idx := range []int{0, 1} {
		prefix := "observation[0].entryRelationship[" + strconv.Itoa(idx) + "].observation[0]"
		for _, field := range []string{"code[0]", "statusCode[0]", "value[0]"} {
			want := prefix + "." + field
			found := false
			for key := range snapshot {
				if strings.HasPrefix(key, "functionalStatus#0") && strings.Contains(key, want) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("no recorded key matches \"functionalStatus#0...%s\" -- got %v", want, snapshot)
			}
		}
	}

	// Negative: the real proof, same shape and same field as the Vital Signs
	// test (Performers) but exercised through the recursive/act-child path --
	// both nested entries carry genuine Performers data but
	// assessmentScaleSupportingObservationRow's Fields never read it. (Note:
	// interpretationCode is deliberately NOT used for this negative --
	// observationRule()'s own top-level fields (declarative_oob_rules.go
	// ~line 1643) DO read interpretationCode on the OUTER entry, so it
	// legitimately shows up as
	// "functionalStatus#0/observation[0].interpretationCode[0]" even
	// though this fixture never sets it there -- Go's encoding/json
	// omitempty is a no-op on non-pointer struct fields, so the zero-value
	// CDACode is still present in the map and the rule still resolves.)
	for key := range snapshot {
		if strings.Contains(key, "performer") {
			t.Errorf("performer element incorrectly recorded as touched: %q (assessmentScaleSupportingObservationRow never reads Performers) -- full snapshot %v", key, snapshot)
		}
	}
}

// TestDeclarativeMapDocument_CoverageTracker_ElementLevel_MultiSection_Encounters
// reproduces a real production report: an Encounters entry showing 0
// elements found at element granularity for FIELDS EncounterMappingRules()
// genuinely reads (code/statusCode/effectiveTime), even though the SAME
// document's mapped Encounter resource has real class/period/status values
// -- proving the gap is in coverage RECORDING, not mapping. Deliberately
// includes a SECOND section (vitalSigns) alongside encounters so
// DeclarativeMapDocument's real per-section goroutine fan-out
// (declarative_document_mapper.go's "go func(...)" per section) is
// genuinely exercised with 2+ concurrent sections -- the single-section
// fixtures every other test in this file uses never trigger real goroutine
// contention, so a concurrency-only bug could hide behind them. Run with
// `go test -race` to catch a real data race directly.
func TestDeclarativeMapDocument_CoverageTracker_ElementLevel_MultiSection_Encounters(t *testing.T) {
	prevOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(prevOutput)

	cdaDoc := &cdadocument.CDADocument{
		SectionsByKey: map[string]*cdadocument.CDASection{
			"encounters": {
				TemplateIds: []string{"2.16.840.1.113883.10.20.22.2.22"},
				Title:       "Encounters",
				Entries: []cdadocument.CDAEntry{
					{
						EntryType:     "encounter",
						ClassCode:     "ENC",
						MoodCode:      "EVN",
						Code:          cdadocument.CDACode{Code: "99213", DisplayName: "Office visit", CodeSystem: "2.16.840.1.113883.6.12"},
						StatusCode:    "completed",
						EffectiveTime: cdadocument.CDATimeRange{Low: cdadocument.CDATime{Value: "20260101"}},
					},
				},
			},
			"vitalSigns": {
				TemplateIds: []string{"2.16.840.1.113883.10.20.22.2.4"},
				Title:       "Vital Signs",
				Entries: []cdadocument.CDAEntry{
					{
						EntryType:  "observation",
						Code:       cdadocument.CDACode{Code: "8867-4", DisplayName: "Heart rate", CodeSystem: "2.16.840.1.113883.6.1"},
						StatusCode: "completed",
						Value:      &cdadocument.CDAValue{Type: "PQ", Quantity: &cdadocument.CDAQuantity{Value: "72", Unit: "/min"}},
					},
				},
			},
		},
	}

	for attempt := 0; attempt < 20; attempt++ {
		tracker := executors.NewCDACoverageTracker()
		mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
		out, err := mapper.DeclarativeMapDocument(context.Background(), cdaDoc, cdafhir.CDAToFHIRConfig{CoverageTracker: tracker})
		if err != nil {
			t.Fatalf("attempt %d: DeclarativeMapDocument: %v", attempt, err)
		}

		// Confirm the mapping side genuinely produced a real Encounter
		// resource with values sourced from code/statusCode/effectiveTime
		// (the exact fields the element-level assertions below check were
		// RECORDED) -- if this fails, the bug is in mapping, not coverage.
		foundEncounter := false
		entries, _ := out.FHIRBundle["entry"].([]interface{})
		for _, e := range entries {
			entryMap, ok := e.(map[string]interface{})
			if !ok {
				continue
			}
			res, ok := entryMap["resource"].(map[string]interface{})
			if !ok || res["resourceType"] != "Encounter" {
				continue
			}
			foundEncounter = true
			if res["class"] == nil {
				t.Errorf("attempt %d: Encounter.class not set -- code[0] apparently not read by mapping either", attempt)
			}
			if res["period"] == nil {
				t.Errorf("attempt %d: Encounter.period not set -- effectiveTime[0] apparently not read by mapping either", attempt)
			}
		}
		if !foundEncounter {
			t.Fatalf("attempt %d: no Encounter resource in output at all", attempt)
		}

		// Raw XML's <entry> directly wraps this entry's own act tag --
		// "encounter" here (EntryType above) -- which the typed parser
		// flattens away; recorded keys must include that "encounter[0]."
		// prefix (executors.CDAEntryActTagPrefix) to match inventory.go's
		// walkEntryElements, which walks the unflattened raw XML and sees
		// the wrapper as a real level. An earlier, weaker version of this
		// assertion (bare "/code" substring match) accidentally matched
		// BOTH the correct shape and the pre-fix buggy bare-"code[0]"
		// shape, so it never actually caught the real production bug this
		// test was written to reproduce -- exact "encounter[0].code"-style
		// substrings close that gap.
		snapshot := tracker.Snapshot()
		for _, wantSuffix := range []string{"encounter[0].code", "encounter[0].statusCode", "encounter[0].effectiveTime"} {
			found := false
			for key := range snapshot {
				if strings.HasPrefix(key, "encounters#0") && strings.Contains(key, wantSuffix) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("attempt %d: no recorded key matches \"encounters#0...%s\" -- got %v", attempt, wantSuffix, snapshot)
			}
		}
	}
}

// TestDeclarativeMapDocument_CoverageTracker_ElementLevel_OrganizerFlattening
// reproduces a second real production bug, found live right after the
// entry-root act-tag fix above: a real Vital Signs Organizer entry (several
// individual vital-sign Observations grouped under one <organizer>, the
// common real-world C-CDA shape observationRule()'s FlattenOrganizers:true
// exists for) showed 0/45 clinical elements found despite genuinely
// mapping several vitals correctly. Cause: flattenOrganizerEntries expands
// the organizer into its component observations, but EVERY flattened
// sibling shared the exact same coveragePrefix+basePath ("vitalSigns#0" +
// "observation[0]"), so distinct real observations (height, weight, heart
// rate, ...) all collapsed onto the IDENTICAL recorded key
// "vitalSigns#0/observation[0].code[0]" -- which could never match the raw
// mirror's real per-component inventory paths
// ("organizer[0].component[N].observation[0].code[0]").
func TestDeclarativeMapDocument_CoverageTracker_ElementLevel_OrganizerFlattening(t *testing.T) {
	prevOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(prevOutput)

	heartRate := "72"
	weight := "68"

	cdaDoc := &cdadocument.CDADocument{
		SectionsByKey: map[string]*cdadocument.CDASection{
			"vitalSigns": {
				TemplateIds: []string{"2.16.840.1.113883.10.20.22.2.4"},
				Title:       "Vital Signs",
				Entries: []cdadocument.CDAEntry{
					{
						EntryType:  "organizer",
						Code:       cdadocument.CDACode{Code: "46680005", DisplayName: "Vital signs", CodeSystem: "2.16.840.1.113883.6.96"},
						StatusCode: "completed",
						Components: []cdadocument.CDAEntry{
							{
								EntryType:  "observation",
								Code:       cdadocument.CDACode{Code: "8867-4", DisplayName: "Heart rate", CodeSystem: "2.16.840.1.113883.6.1"},
								StatusCode: "completed",
								Value:      &cdadocument.CDAValue{Type: "PQ", Quantity: &cdadocument.CDAQuantity{Value: heartRate, Unit: "/min"}},
							},
							{
								EntryType:  "observation",
								Code:       cdadocument.CDACode{Code: "29463-7", DisplayName: "Body weight", CodeSystem: "2.16.840.1.113883.6.1"},
								StatusCode: "completed",
								Value:      &cdadocument.CDAValue{Type: "PQ", Quantity: &cdadocument.CDAQuantity{Value: weight, Unit: "kg"}},
							},
						},
					},
				},
			},
		},
	}

	tracker := executors.NewCDACoverageTracker()
	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	out, err := mapper.DeclarativeMapDocument(context.Background(), cdaDoc, cdafhir.CDAToFHIRConfig{CoverageTracker: tracker})
	if err != nil {
		t.Fatalf("DeclarativeMapDocument: %v", err)
	}

	// Confirm the mapping side genuinely produced TWO distinct Observation
	// resources (one per flattened component) -- if this fails, the bug is
	// in mapping/flattening itself, not coverage.
	entries, _ := out.FHIRBundle["entry"].([]interface{})
	obsCount := 0
	for _, e := range entries {
		entryMap, ok := e.(map[string]interface{})
		if !ok {
			continue
		}
		if res, ok := entryMap["resource"].(map[string]interface{}); ok && res["resourceType"] == "Observation" {
			obsCount++
		}
	}
	if obsCount != 2 {
		t.Fatalf("got %d Observation resources, want 2 (one per flattened organizer component)", obsCount)
	}

	snapshot := tracker.Snapshot()

	// The two flattened components must get DISTINCTLY-indexed keys --
	// "organizer[0].component[0]...." vs "organizer[0].component[1]....",
	// never the same collapsed "observation[0]...." key for both.
	for _, componentIdx := range []int{0, 1} {
		prefix := "organizer[0].component[" + strconv.Itoa(componentIdx) + "].observation[0]"
		for _, field := range []string{"code[0]", "statusCode[0]", "value[0]"} {
			want := prefix + "." + field
			found := false
			for key := range snapshot {
				if strings.HasPrefix(key, "vitalSigns#0") && strings.Contains(key, want) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("no recorded key matches \"vitalSigns#0...%s\" -- got %v", want, snapshot)
			}
		}
	}

	// Negative: the two components must NOT collapse onto a shared bare
	// "observation[0].code[0]" key with no organizer/component prefix --
	// that collapsed shape is exactly the bug this test reproduces.
	for key := range snapshot {
		if strings.Contains(key, "vitalSigns#0/observation[0]") {
			t.Errorf("key collapsed onto the bare (pre-fix, buggy) shape with no organizer/component prefix: %q -- full snapshot %v", key, snapshot)
		}
	}
}

// TestDeclarativeMapDocument_CoverageTracker_Header_Patient is the header-
// field extension's positive+negative proof, matching the Vital Signs
// test's own style: a field PatientMappingRules() genuinely reads (Names)
// must be recorded; a field it never reads (Religion, confirmed by reading
// PatientMappingRules() in full -- it reads ids/names/addresses/telecoms/
// birthDate/gender/deceasedInd/maritalStatus/languages, never race/
// ethnicity/religion) must NOT be, even though this fixture gives it real,
// non-empty data.
func TestDeclarativeMapDocument_CoverageTracker_Header_Patient(t *testing.T) {
	prevOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(prevOutput)

	cdaDoc := &cdadocument.CDADocument{
		Header: cdadocument.CDAHeader{
			Patient: cdadocument.CDAPatient{
				Names:    []cdadocument.CDAName{{Family: "Smith", Given: []string{"Jane"}}},
				Religion: cdadocument.CDACode{Code: "1013", DisplayName: "Catholic", CodeSystem: "2.16.840.1.113883.5.1076"},
			},
		},
	}

	tracker := executors.NewCDACoverageTracker()
	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	_, err := mapper.DeclarativeMapDocument(context.Background(), cdaDoc, cdafhir.CDAToFHIRConfig{CoverageTracker: tracker})
	if err != nil {
		t.Fatalf("DeclarativeMapDocument: %v", err)
	}

	snapshot := tracker.Snapshot()

	if !tracker.Touched(executors.CDAEntryKey("header.patient", 0)) {
		t.Errorf("header.patient#0 not recorded -- got %v", snapshot)
	}

	found := false
	for key := range snapshot {
		if strings.HasPrefix(key, "header.patient#0") && strings.Contains(key, "patient[0].name[0]") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("no recorded key matches \"header.patient#0...patient[0].name[0]\" -- got %v", snapshot)
	}

	// Negative: the real proof -- religiousAffiliationCode has genuine data
	// on this fixture but no row in PatientMappingRules() ever reads it.
	for key := range snapshot {
		if strings.Contains(strings.ToLower(key), "religio") {
			t.Errorf("religion element incorrectly recorded as touched: %q (PatientMappingRules() never reads it) -- full snapshot %v", key, snapshot)
		}
	}
}

// TestDeclarativeMapDocument_CoverageTracker_Header_AuthorSelection is the
// real proof for the author-selection edge case: raw XML can carry several
// <author> elements, but firstAuthorWithPersonIndexed only ever selects the
// first one with a usable assignedPerson (AuthorMappingRules()'s own
// production behavior, unchanged by this session's work). Ground truth
// means the UNSELECTED author's own real content must still show up as a
// genuine gap, not be silently skipped just because the typed side never
// looked at it.
func TestDeclarativeMapDocument_CoverageTracker_Header_AuthorSelection(t *testing.T) {
	prevOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(prevOutput)

	cdaDoc := &cdadocument.CDADocument{
		Header: cdadocument.CDAHeader{
			Authors: []cdadocument.CDAAuthor{
				{
					// Author 0: a device, no person -- firstAuthorWithPersonIndexed
					// must skip this one.
					AssignedAuthor: cdadocument.CDAAssignedAuthor{
						AssignedAuthoringDevice: &cdadocument.CDADevice{SoftwareName: "Epic EHR"},
					},
				},
				{
					// Author 1: has a person -- this is the one that gets selected.
					AssignedAuthor: cdadocument.CDAAssignedAuthor{
						AssignedPerson: &cdadocument.CDAPerson{
							Names: []cdadocument.CDAName{{Family: "Jones", Given: []string{"Robert"}}},
						},
					},
				},
			},
		},
	}

	tracker := executors.NewCDACoverageTracker()
	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	_, err := mapper.DeclarativeMapDocument(context.Background(), cdaDoc, cdafhir.CDAToFHIRConfig{CoverageTracker: tracker})
	if err != nil {
		t.Fatalf("DeclarativeMapDocument: %v", err)
	}

	snapshot := tracker.Snapshot()

	// Author 1 (the selected one) must be recorded, with its own real name
	// element.
	if !tracker.Touched(executors.CDAEntryKey("header.author", 1)) {
		t.Errorf("header.author#1 not recorded -- got %v", snapshot)
	}
	found := false
	for key := range snapshot {
		if strings.HasPrefix(key, "header.author#1") && strings.Contains(key, "author[1]") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("no recorded key matches \"header.author#1...author[1]\" -- got %v", snapshot)
	}

	// Author 0 (never selected) must NOT be recorded at all -- its device
	// name is real content that genuinely was never looked at.
	if tracker.Touched(executors.CDAEntryKey("header.author", 0)) {
		t.Errorf("header.author#0 incorrectly recorded as touched -- it was never selected by firstAuthorWithPersonIndexed -- got %v", snapshot)
	}
	for key := range snapshot {
		if strings.Contains(key, "author[0]") {
			t.Errorf("found a key referencing the unselected author[0]: %q -- got %v", key, snapshot)
		}
	}
}

// TestDeclarativeMapDocument_CoverageTracker_Header_EncompassingEncounterSharing
// proves EncompassingEncounterMappingRules and
// EncompassingEncounterLocationMappingRules -- two separate
// BuildHeaderResourceWithCoveragePrefix calls whose typed roots physically
// overlap (Location resolves to a sub-node of Encounter) -- correctly share
// ONE tracking unit ("header.encompassingEncounter#0"), not two disjoint
// ones. Populates both the Encounter's own fields (Id) and its nested
// Location/HealthCareFacility (Code) so both call sites' recordings are
// exercised in the same run.
func TestDeclarativeMapDocument_CoverageTracker_Header_EncompassingEncounterSharing(t *testing.T) {
	prevOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(prevOutput)

	cdaDoc := &cdadocument.CDADocument{
		Header: cdadocument.CDAHeader{
			EncompassingEncounter: &cdadocument.CDAEncounter{
				Id: cdadocument.CDAII{Root: "2.16.840.1.113883.19", Extension: "ENC-1"},
				Location: &cdadocument.CDALocation{
					HealthCareFacility: cdadocument.CDAHealthCareFacility{
						Code: cdadocument.CDACode{Code: "1160-1", DisplayName: "Urgent Care Center", CodeSystem: "2.16.840.1.113883.6.259"},
					},
				},
			},
		},
	}

	tracker := executors.NewCDACoverageTracker()
	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	_, err := mapper.DeclarativeMapDocument(context.Background(), cdaDoc, cdafhir.CDAToFHIRConfig{CoverageTracker: tracker})
	if err != nil {
		t.Fatalf("DeclarativeMapDocument: %v", err)
	}

	snapshot := tracker.Snapshot()

	sharedKey := executors.CDAEntryKey("header.encompassingEncounter", 0)
	if !tracker.Touched(sharedKey) {
		t.Errorf("%s not recorded -- got %v", sharedKey, snapshot)
	}

	// Both the Encounter's own field (via EncompassingEncounterMappingRules)
	// AND the nested Location field (via
	// EncompassingEncounterLocationMappingRules) must land under the SAME
	// "header.encompassingEncounter#0" prefix -- proving the two call sites
	// share one unit instead of drifting into two independently-numbered
	// ones.
	foundEncounterField := false
	foundLocationField := false
	for key := range snapshot {
		if !strings.HasPrefix(key, "header.encompassingEncounter#0") {
			if strings.HasPrefix(key, "header.encompassingEncounter") {
				t.Errorf("key uses a DIFFERENT encompassingEncounter prefix than #0, proving the two call sites did NOT share one tracking unit: %q -- got %v", key, snapshot)
			}
			continue
		}
		if strings.Contains(key, "componentOf[0].encompassingEncounter[0].id") {
			foundEncounterField = true
		}
		if strings.Contains(key, "healthCareFacility[0].code") {
			foundLocationField = true
		}
	}
	if !foundEncounterField {
		t.Errorf("no recorded key matches the Encounter's own id field under the shared prefix -- got %v", snapshot)
	}
	if !foundLocationField {
		t.Errorf("no recorded key matches the nested Location's healthCareFacility code under the shared prefix -- got %v", snapshot)
	}
}
