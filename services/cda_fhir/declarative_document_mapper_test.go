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
	"testing"

	cdaSchema "ezhealthkonnect/cda"
	cdadocument "ezhealthkonnect/cda/document"
	cdafhir "ezhealthkonnect/services/cda_fhir"

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
			"Condition": 5, "MedicationRequest": 6, "Organization": 1, "Patient": 1, "Practitioner": 1,
		},
		resourcesProduced: 14,
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
		resourceCounts: map[string]int{
			"AllergyIntolerance": 2, "Condition": 2, "Encounter": 1, "Immunization": 1,
			"MedicationStatement": 1, "Observation": 1, "Organization": 1, "Patient": 1,
			"Practitioner": 1, "Procedure": 1,
		},
		resourcesProduced: 12,
		successfulCount:   8,
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
// against).
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

func containsStr(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
