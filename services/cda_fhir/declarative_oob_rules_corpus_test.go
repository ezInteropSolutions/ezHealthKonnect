// services/cda_fhir/declarative_oob_rules_corpus_test.go
//
// Phase 3 exit criterion: re-run the 4-vendor corpus end-to-end (XML→FHIR
// this time, not just the XML→JSON fidelity pass Phase 0 did) through the
// ported Allergies/Medications/Conditions rules, and spot-check output
// shape. Per the sprint plan's working principle, the corpus is evidence of
// real-world prevalence/shape only — it asserts "did this run without
// panicking and produce well-formed FHIR-shaped resources for whatever this
// vendor's export actually contains", not "this exact value is correct" (no
// vendor sample is treated as a source of truth for correctness).
package cdafhir_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	cdaSchema "ezhealthkonnect/cda"
	cdadocument "ezhealthkonnect/cda/document"
	cdafhir "ezhealthkonnect/services/cda_fhir"

	"github.com/beevik/etree"
)

var corpusFiles = []string{
	"cerner_sample.xml",
	"kareo_sample.xml",
	"mtuitive_sample.xml",
	"practicefusion_sample.xml",
}

func loadCorpusDocumentMap(t testing.TB, fileName string) map[string]interface{} {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")

	data, err := os.ReadFile(filepath.Join(repoRoot, "cda", "document", "testdata", "corpus", fileName))
	if err != nil {
		t.Fatalf("reading corpus file %s: %v", fileName, err)
	}
	raw := string(data)

	doc := etree.NewDocument()
	if err := doc.ReadFromString(raw); err != nil {
		t.Fatalf("parsing XML in %s: %v", fileName, err)
	}

	loader, err := cdaSchema.NewCDASchemaLoader(filepath.Join(repoRoot, "cda", "schemas"))
	if err != nil {
		t.Fatalf("loading CDA schema: %v", err)
	}
	typedDoc := cdadocument.NewCDAParser(loader).ParseDocument(doc.Root(), raw)

	encoded, err := json.Marshal(typedDoc)
	if err != nil {
		t.Fatalf("marshalling typed document for %s: %v", fileName, err)
	}
	var documentMap map[string]interface{}
	if err := json.Unmarshal(encoded, &documentMap); err != nil {
		t.Fatalf("unmarshalling typed document for %s: %v", fileName, err)
	}
	return documentMap
}

// assertWellFormedResource checks the minimal shape every resource this
// engine produces must have, regardless of which vendor/section produced
// it: a resourceType matching what the rule declared, and no field set to
// a literal Go nil/empty placeholder that would indicate a transform wrote
// garbage instead of skipping cleanly.
func assertWellFormedResource(t *testing.T, resource map[string]interface{}, wantResourceType string) {
	t.Helper()
	if rt, _ := resource["resourceType"].(string); rt != wantResourceType {
		t.Errorf("resourceType = %v, want %q", resource["resourceType"], wantResourceType)
	}
	for k, v := range resource {
		if isNilValueForTest(v) {
			t.Errorf("field %q is literally nil -- a transform should have skipped the write, not stored nil", k)
		}
	}
}

// isNilValueForTest mirrors declarative_engine.go's isNilValue (unexported,
// not visible from this cdafhir_test package) -- a plain `v == nil` check
// here used to miss exactly the bug that check exists to catch: a typed nil
// (e.g. a nil map[string]interface{} returned by
// transforms.CDACodeToCodeableConcept) boxed into interface{} is NOT == nil,
// so this corpus test's own "no literal null fields" guard would have
// silently passed on the real PracticeFusion/Kareo Results data that
// exposed the bug. Caught the regression once, worth keeping permanently.
func isNilValueForTest(v interface{}) bool {
	if v == nil {
		return true
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

// assertWellFormedReactions spot-checks AllergyIntolerance.reaction[] shape
// against real vendor data — the CollectAll+Fields row this exercises
// (AllergyMappingRules) is proven against real Kareo/Cerner reaction+
// severity data per architecture/CDA_FHIR_MAPPING_INVENTORY.md's allergy
// section, not just synthetic fixtures.
func assertWellFormedReactions(t *testing.T, resourceIdx int, reactions interface{}) {
	t.Helper()
	arr, ok := reactions.([]interface{})
	if !ok || len(arr) == 0 {
		t.Errorf("resource[%d].reaction = %v, want a non-empty array (the row only writes when len(subObj)>0)", resourceIdx, reactions)
		return
	}
	for i, item := range arr {
		rxn, ok := item.(map[string]interface{})
		if !ok || len(rxn) == 0 {
			t.Errorf("resource[%d].reaction[%d] = %v, want a non-empty map -- an empty reaction should have been dropped, not written", resourceIdx, i, item)
			continue
		}
		if _, hasManifestation := rxn["manifestation"]; !hasManifestation {
			if _, hasSeverity := rxn["severity"]; !hasSeverity {
				t.Errorf("resource[%d].reaction[%d] = %v has neither manifestation nor severity -- should have been dropped", resourceIdx, i, rxn)
			}
		}
	}
}

func TestDeclarativeEngine_Corpus_AllergiesEndToEnd(t *testing.T) {
	engine := cdafhir.NewDeclarativeEngine()
	rule := cdafhir.AllergyMappingRules()[0]

	for _, file := range corpusFiles {
		t.Run(file, func(t *testing.T) {
			documentMap := loadCorpusDocumentMap(t, file)
			resources, errs := engine.BuildResources(documentMap, rule)
			t.Logf("%s: %d AllergyIntolerance resources, %d field-level notices", file, len(resources), len(errs))
			for _, e := range errs {
				t.Logf("  notice: entry=%d field=%s severity=%s err=%s", e.EntryIndex, e.FieldKey, e.Severity, e.Error)
			}
			for i, r := range resources {
				// AllergyMappingRules' recorder/asserter rows emit sub-resources
				// (Practitioner/PractitionerRole/Organization) via EmitAsResource,
				// concatenated into this SAME slice by BuildResources (see its
				// own doc comment) -- the identical mixed-type slice the
				// production Bundle assembler consumes. Skip anything that
				// isn't this section's own AllergyIntolerance, same convention
				// TestDeclarativeEngine_CareTeam_BuildsCareTeamAndPractitioner
				// (declarative_oob_rules_test.go) already established.
				if rt, _ := r["resourceType"].(string); rt != "AllergyIntolerance" {
					continue
				}
				assertWellFormedResource(t, r, "AllergyIntolerance")
				if reactions, has := r["reaction"]; has {
					assertWellFormedReactions(t, i, reactions)
				}
				pretty, _ := json.MarshalIndent(r, "    ", "  ")
				t.Logf("  resource[%d]:\n    %s", i, pretty)
			}
		})
	}
}

func TestDeclarativeEngine_Corpus_MedicationsEndToEnd(t *testing.T) {
	engine := cdafhir.NewDeclarativeEngine()
	rules := cdafhir.MedicationMappingRules()

	for _, file := range corpusFiles {
		t.Run(file, func(t *testing.T) {
			documentMap := loadCorpusDocumentMap(t, file)
			resources, errs := engine.BuildResourcesForRules(documentMap, rules)
			t.Logf("%s: %d Medication* resources, %d field-level notices", file, len(resources), len(errs))
			for _, e := range errs {
				t.Logf("  notice: entry=%d field=%s severity=%s err=%s", e.EntryIndex, e.FieldKey, e.Severity, e.Error)
			}
			for i, r := range resources {
				rt, _ := r["resourceType"].(string)
				// MedicationMappingRules' requester-tier rows emit sub-resources
				// (Practitioner/Organization) via EmitAsResource, concatenated
				// into this same slice by BuildResourcesForRules (see its doc
				// comment) -- skip anything that isn't this section's own
				// Medication* resource.
				if rt != "MedicationRequest" && rt != "MedicationStatement" {
					continue
				}
				pretty, _ := json.MarshalIndent(r, "    ", "  ")
				t.Logf("  resource[%d] (%s):\n    %s", i, rt, pretty)
			}
		})
	}
}

func TestDeclarativeEngine_Corpus_ConditionsEndToEnd(t *testing.T) {
	engine := cdafhir.NewDeclarativeEngine()
	problemsRule := cdafhir.ProblemsMappingRules()[0]
	healthConcernsRule := cdafhir.HealthConcernsMappingRules()[0]

	for _, file := range corpusFiles {
		t.Run(file, func(t *testing.T) {
			documentMap := loadCorpusDocumentMap(t, file)

			resources, errs := engine.BuildResources(documentMap, problemsRule)
			t.Logf("%s: %d Condition resources from \"problems\", %d field-level notices", file, len(resources), len(errs))
			for i, r := range resources {
				// ProblemsMappingRules' recorder/asserter rows emit sub-resources
				// (Practitioner/PractitionerRole/Organization) via EmitAsResource,
				// concatenated into this same slice by BuildResources (see its
				// own doc comment) -- skip anything that isn't this section's
				// own Condition resource.
				if rt, _ := r["resourceType"].(string); rt != "Condition" {
					continue
				}
				assertWellFormedResource(t, r, "Condition")
				pretty, _ := json.MarshalIndent(r, "    ", "  ")
				t.Logf("  problems[%d]:\n    %s", i, pretty)
			}

			hcResources, hcErrs := engine.BuildResources(documentMap, healthConcernsRule)
			t.Logf("%s: %d Condition resources from \"healthConcerns\", %d field-level notices", file, len(hcResources), len(hcErrs))
			for i, r := range hcResources {
				if rt, _ := r["resourceType"].(string); rt != "Condition" {
					continue
				}
				assertWellFormedResource(t, r, "Condition")
				pretty, _ := json.MarshalIndent(r, "    ", "  ")
				t.Logf("  healthConcerns[%d]:\n    %s", i, pretty)
			}
		})
	}
}

// runObservationCorpusEndToEnd is shared by the three corpus tests below --
// VitalSignsMappingRules/ResultsMappingRules/SocialHistoryMappingRules are
// all one rule, FlattenOrganizers, against documentMap, with the same
// well-formed-resource + us-core-2 (value[x]-or-dataAbsentReason) spot-check.
func runObservationCorpusEndToEnd(t *testing.T, sectionLabel string, rule cdafhir.MappingRule) {
	engine := cdafhir.NewDeclarativeEngine()
	for _, file := range corpusFiles {
		t.Run(file, func(t *testing.T) {
			documentMap := loadCorpusDocumentMap(t, file)
			resources, errs := engine.BuildResources(documentMap, rule)
			t.Logf("%s: %d Observation resources from %q, %d field-level notices", file, len(resources), sectionLabel, len(errs))
			for _, e := range errs {
				t.Logf("  notice: entry=%d field=%s severity=%s err=%s", e.EntryIndex, e.FieldKey, e.Severity, e.Error)
			}
			for i, r := range resources {
				assertWellFormedResource(t, r, "Observation")
				if !hasAnyObservationValueXOrAbsentReason(r) {
					t.Errorf("resource[%d] has none of value[x]/dataAbsentReason -- violates us-core-2", i)
				}
				pretty, _ := json.MarshalIndent(r, "    ", "  ")
				t.Logf("  resource[%d]:\n    %s", i, pretty)
			}
		})
	}
}

// hasAnyObservationValueXOrAbsentReason checks the same us-core-2 invariant
// the SkipIfResourceHasAnyOf-gated dataAbsentReason row exists to satisfy --
// a corpus-level proof the gate actually fires for every real resource this
// rule produces, not just the hand-built fixtures in declarative_oob_rules_test.go.
func hasAnyObservationValueXOrAbsentReason(r map[string]interface{}) bool {
	for _, key := range append([]string{"dataAbsentReason"}, observationValueXFHIRKeysForTest...) {
		if _, ok := r[key]; ok {
			return true
		}
	}
	return false
}

// observationValueXFHIRKeysForTest mirrors declarative_oob_rules.go's
// observationValueXFHIRKeys (unexported, package cdafhir, not visible from
// this cdafhir_test package) -- kept in sync by
// TestDeclarativeEngine_VitalSigns_NonBPValue_SetCorrectly and friends
// actually exercising each key, not by a shared symbol across the package
// boundary.
var observationValueXFHIRKeysForTest = []string{
	"valueQuantity", "valueCodeableConcept", "valueString", "valueBoolean", "valueInteger", "valuePeriod",
}

func TestDeclarativeEngine_Corpus_VitalSignsEndToEnd(t *testing.T) {
	runObservationCorpusEndToEnd(t, "vitalSigns", cdafhir.VitalSignsMappingRules()[0])
}

func TestDeclarativeEngine_Corpus_ResultsEndToEnd(t *testing.T) {
	runObservationCorpusEndToEnd(t, "results", cdafhir.ResultsMappingRules()[0])
}

func TestDeclarativeEngine_Corpus_SocialHistoryEndToEnd(t *testing.T) {
	runObservationCorpusEndToEnd(t, "socialHistory", cdafhir.SocialHistoryMappingRules()[0])
}

func TestDeclarativeEngine_Corpus_ImmunizationsEndToEnd(t *testing.T) {
	engine := cdafhir.NewDeclarativeEngine()
	rule := cdafhir.ImmunizationMappingRules()[0]

	for _, file := range corpusFiles {
		t.Run(file, func(t *testing.T) {
			documentMap := loadCorpusDocumentMap(t, file)
			resources, errs := engine.BuildResources(documentMap, rule)
			t.Logf("%s: %d Immunization resources, %d field-level notices", file, len(resources), len(errs))
			for _, e := range errs {
				t.Logf("  notice: entry=%d field=%s severity=%s err=%s", e.EntryIndex, e.FieldKey, e.Severity, e.Error)
			}
			for i, r := range resources {
				assertWellFormedResource(t, r, "Immunization")
				// Kareo's real immunization entry has negationInd="true" --
				// the exact case ImmunizationMappingRules' status row exists
				// to handle correctly.
				if r["status"] == "not-done" && file != "kareo_sample.xml" {
					t.Errorf("resource[%d].status = not-done in %s -- only Kareo's corpus sample has a negated immunization", i, file)
				}
				pretty, _ := json.MarshalIndent(r, "    ", "  ")
				t.Logf("  resource[%d]:\n    %s", i, pretty)
			}
		})
	}
}

func TestDeclarativeEngine_Corpus_EncountersEndToEnd(t *testing.T) {
	engine := cdafhir.NewDeclarativeEngine()
	rule := cdafhir.EncounterMappingRules()[0]

	for _, file := range corpusFiles {
		t.Run(file, func(t *testing.T) {
			documentMap := loadCorpusDocumentMap(t, file)
			resources, errs := engine.BuildResources(documentMap, rule)
			t.Logf("%s: %d Encounter resources, %d field-level notices", file, len(resources), len(errs))
			for _, e := range errs {
				t.Logf("  notice: entry=%d field=%s severity=%s err=%s", e.EntryIndex, e.FieldKey, e.Severity, e.Error)
			}
			for i, r := range resources {
				assertWellFormedResource(t, r, "Encounter")
				pretty, _ := json.MarshalIndent(r, "    ", "  ")
				t.Logf("  resource[%d]:\n    %s", i, pretty)
			}
		})
	}
}

func TestDeclarativeEngine_Corpus_ProceduresEndToEnd(t *testing.T) {
	engine := cdafhir.NewDeclarativeEngine()
	rule := cdafhir.ProcedureMappingRules()[0]

	for _, file := range corpusFiles {
		t.Run(file, func(t *testing.T) {
			documentMap := loadCorpusDocumentMap(t, file)
			resources, errs := engine.BuildResources(documentMap, rule)
			t.Logf("%s: %d Procedure resources, %d field-level notices", file, len(resources), len(errs))
			for _, e := range errs {
				t.Logf("  notice: entry=%d field=%s severity=%s err=%s", e.EntryIndex, e.FieldKey, e.Severity, e.Error)
			}
			for i, r := range resources {
				assertWellFormedResource(t, r, "Procedure")
				pretty, _ := json.MarshalIndent(r, "    ", "  ")
				t.Logf("  resource[%d]:\n    %s", i, pretty)
			}
		})
	}
}

func TestDeclarativeEngine_Corpus_GoalsEndToEnd(t *testing.T) {
	engine := cdafhir.NewDeclarativeEngine()
	rule := cdafhir.GoalMappingRules()[0]

	for _, file := range corpusFiles {
		t.Run(file, func(t *testing.T) {
			documentMap := loadCorpusDocumentMap(t, file)
			resources, errs := engine.BuildResources(documentMap, rule)
			t.Logf("%s: %d Goal resources, %d field-level notices", file, len(resources), len(errs))
			for _, e := range errs {
				t.Logf("  notice: entry=%d field=%s severity=%s err=%s", e.EntryIndex, e.FieldKey, e.Severity, e.Error)
			}
			for i, r := range resources {
				assertWellFormedResource(t, r, "Goal")
				pretty, _ := json.MarshalIndent(r, "    ", "  ")
				t.Logf("  resource[%d]:\n    %s", i, pretty)
			}
		})
	}
}

func TestDeclarativeEngine_Corpus_PlanOfCareEndToEnd(t *testing.T) {
	engine := cdafhir.NewDeclarativeEngine()
	rules := cdafhir.PlanOfCareMappingRules()
	validTypes := map[string]bool{"ServiceRequest": true, "Appointment": true, "SupplyRequest": true, "MedicationRequest": true, "Goal": true}

	for _, file := range corpusFiles {
		t.Run(file, func(t *testing.T) {
			documentMap := loadCorpusDocumentMap(t, file)
			resources, errs := engine.BuildResourcesForRules(documentMap, rules)
			t.Logf("%s: %d Plan-of-Care resources, %d field-level notices", file, len(resources), len(errs))
			for _, e := range errs {
				t.Logf("  notice: entry=%d field=%s severity=%s err=%s", e.EntryIndex, e.FieldKey, e.Severity, e.Error)
			}
			for i, r := range resources {
				rt, _ := r["resourceType"].(string)
				if !validTypes[rt] {
					t.Errorf("resource[%d].resourceType = %v, want one of ServiceRequest/Appointment/SupplyRequest/MedicationRequest/Goal", i, r["resourceType"])
				}
				assertWellFormedResource(t, r, rt)
				pretty, _ := json.MarshalIndent(r, "    ", "  ")
				t.Logf("  resource[%d] (%s):\n    %s", i, rt, pretty)
			}
		})
	}
}

func TestDeclarativeEngine_Corpus_CareTeamEndToEnd(t *testing.T) {
	engine := cdafhir.NewDeclarativeEngine()
	rules := cdafhir.CareTeamMappingRules()
	validTypes := map[string]bool{"CareTeam": true, "Practitioner": true}

	for _, file := range corpusFiles {
		t.Run(file, func(t *testing.T) {
			documentMap := loadCorpusDocumentMap(t, file)
			var resources []map[string]interface{}
			var errs []cdafhir.SectionError
			for _, rule := range rules {
				ruleResources, ruleErrs := engine.BuildResources(documentMap, rule)
				resources = append(resources, ruleResources...)
				errs = append(errs, ruleErrs...)
			}
			t.Logf("%s: %d CareTeam/Practitioner resources, %d field-level notices", file, len(resources), len(errs))
			for _, e := range errs {
				t.Logf("  notice: entry=%d field=%s severity=%s err=%s", e.EntryIndex, e.FieldKey, e.Severity, e.Error)
			}
			for i, r := range resources {
				rt, _ := r["resourceType"].(string)
				if !validTypes[rt] {
					t.Errorf("resource[%d].resourceType = %v, want CareTeam or Practitioner", i, r["resourceType"])
				}
				assertWellFormedResource(t, r, rt)
				pretty, _ := json.MarshalIndent(r, "    ", "  ")
				t.Logf("  resource[%d] (%s):\n    %s", i, rt, pretty)
			}
		})
	}
}

func TestDeclarativeEngine_Corpus_CoverageEndToEnd(t *testing.T) {
	engine := cdafhir.NewDeclarativeEngine()
	rules := cdafhir.CoverageMappingRules()

	for _, file := range corpusFiles {
		t.Run(file, func(t *testing.T) {
			documentMap := loadCorpusDocumentMap(t, file)
			var resources []map[string]interface{}
			var errs []cdafhir.SectionError
			for _, rule := range rules {
				ruleResources, ruleErrs := engine.BuildResources(documentMap, rule)
				resources = append(resources, ruleResources...)
				errs = append(errs, ruleErrs...)
			}
			t.Logf("%s: %d Coverage resources, %d field-level notices", file, len(resources), len(errs))
			for _, e := range errs {
				t.Logf("  notice: entry=%d field=%s severity=%s err=%s", e.EntryIndex, e.FieldKey, e.Severity, e.Error)
			}
			for i, r := range resources {
				assertWellFormedResource(t, r, "Coverage")
				pretty, _ := json.MarshalIndent(r, "    ", "  ")
				t.Logf("  resource[%d]:\n    %s", i, pretty)
			}
		})
	}
}

func TestDeclarativeEngine_Corpus_FamilyMemberHistoryEndToEnd(t *testing.T) {
	engine := cdafhir.NewDeclarativeEngine()
	rule := cdafhir.FamilyMemberHistoryMappingRules()[0]

	for _, file := range corpusFiles {
		t.Run(file, func(t *testing.T) {
			documentMap := loadCorpusDocumentMap(t, file)
			resources, errs := engine.BuildResources(documentMap, rule)
			t.Logf("%s: %d FamilyMemberHistory resources, %d field-level notices", file, len(resources), len(errs))
			for _, e := range errs {
				t.Logf("  notice: entry=%d field=%s severity=%s err=%s", e.EntryIndex, e.FieldKey, e.Severity, e.Error)
			}
			for i, r := range resources {
				assertWellFormedResource(t, r, "FamilyMemberHistory")
				pretty, _ := json.MarshalIndent(r, "    ", "  ")
				t.Logf("  resource[%d]:\n    %s", i, pretty)
			}
		})
	}
}

func TestDeclarativeEngine_Corpus_DeviceEndToEnd(t *testing.T) {
	engine := cdafhir.NewDeclarativeEngine()
	rule := cdafhir.DeviceMappingRules()[0]

	for _, file := range corpusFiles {
		t.Run(file, func(t *testing.T) {
			documentMap := loadCorpusDocumentMap(t, file)
			resources, errs := engine.BuildResources(documentMap, rule)
			t.Logf("%s: %d DeviceUseStatement resources, %d field-level notices", file, len(resources), len(errs))
			for _, e := range errs {
				t.Logf("  notice: entry=%d field=%s severity=%s err=%s", e.EntryIndex, e.FieldKey, e.Severity, e.Error)
			}
			for i, r := range resources {
				assertWellFormedResource(t, r, "DeviceUseStatement")
				pretty, _ := json.MarshalIndent(r, "    ", "  ")
				t.Logf("  resource[%d]:\n    %s", i, pretty)
			}
		})
	}
}

func TestDeclarativeEngine_Corpus_AuthorEndToEnd(t *testing.T) {
	engine := cdafhir.NewDeclarativeEngine()
	rule := cdafhir.AuthorMappingRules()[0]

	for _, file := range corpusFiles {
		t.Run(file, func(t *testing.T) {
			documentMap := loadCorpusDocumentMap(t, file)
			resource, extra, errs := engine.BuildHeaderResource(documentMap, rule)
			t.Logf("%s: author resource present=%v, %d extra, %d field-level notices", file, resource != nil, len(extra), len(errs))
			for _, e := range errs {
				t.Logf("  notice: entry=%d field=%s severity=%s err=%s", e.EntryIndex, e.FieldKey, e.Severity, e.Error)
			}
			if resource != nil {
				assertWellFormedResource(t, resource, "Practitioner")
				pretty, _ := json.MarshalIndent(resource, "    ", "  ")
				t.Logf("  resource:\n    %s", pretty)
			}
		})
	}
}

func TestDeclarativeEngine_Corpus_LegalAuthenticatorEndToEnd(t *testing.T) {
	engine := cdafhir.NewDeclarativeEngine()
	rule := cdafhir.LegalAuthenticatorMappingRules()[0]

	for _, file := range corpusFiles {
		t.Run(file, func(t *testing.T) {
			documentMap := loadCorpusDocumentMap(t, file)
			resource, extra, errs := engine.BuildHeaderResource(documentMap, rule)
			t.Logf("%s: legalAuthenticator resource present=%v, %d extra, %d field-level notices", file, resource != nil, len(extra), len(errs))
			for _, e := range errs {
				t.Logf("  notice: entry=%d field=%s severity=%s err=%s", e.EntryIndex, e.FieldKey, e.Severity, e.Error)
			}
			if resource != nil {
				assertWellFormedResource(t, resource, "Practitioner")
				pretty, _ := json.MarshalIndent(resource, "    ", "  ")
				t.Logf("  resource:\n    %s", pretty)
			}
		})
	}
}

func TestDeclarativeEngine_Corpus_CustodianEndToEnd(t *testing.T) {
	engine := cdafhir.NewDeclarativeEngine()
	rule := cdafhir.CustodianMappingRules()[0]

	for _, file := range corpusFiles {
		t.Run(file, func(t *testing.T) {
			documentMap := loadCorpusDocumentMap(t, file)
			resource, extra, errs := engine.BuildHeaderResource(documentMap, rule)
			t.Logf("%s: custodian resource present=%v, %d extra, %d field-level notices", file, resource != nil, len(extra), len(errs))
			for _, e := range errs {
				t.Logf("  notice: entry=%d field=%s severity=%s err=%s", e.EntryIndex, e.FieldKey, e.Severity, e.Error)
			}
			if resource != nil {
				assertWellFormedResource(t, resource, "Organization")
				pretty, _ := json.MarshalIndent(resource, "    ", "  ")
				t.Logf("  resource:\n    %s", pretty)
			}
		})
	}
}

// TestDeclarativeEngine_Corpus_PatientEndToEnd is Phase 4 Slice A's corpus
// proof: PatientMappingRules() against all 4 real vendor documents. A real
// C-CDA document always has a patient (unlike Author/Custodian/
// LegalAuthenticator, which are genuinely optional per the corpus results
// above), so this asserts a resource is actually produced for every file,
// not just that IF one is produced it's well-formed.
func TestDeclarativeEngine_Corpus_PatientEndToEnd(t *testing.T) {
	engine := cdafhir.NewDeclarativeEngine()
	rule := cdafhir.PatientMappingRules()[0]

	for _, file := range corpusFiles {
		t.Run(file, func(t *testing.T) {
			documentMap := loadCorpusDocumentMap(t, file)
			resource, extra, errs := engine.BuildHeaderResource(documentMap, rule)
			t.Logf("%s: patient resource present=%v, %d extra, %d field-level notices", file, resource != nil, len(extra), len(errs))
			for _, e := range errs {
				t.Logf("  notice: entry=%d field=%s severity=%s err=%s", e.EntryIndex, e.FieldKey, e.Severity, e.Error)
			}
			if resource == nil {
				t.Fatalf("%s: expected a Patient resource (every real C-CDA document has one)", file)
			}
			assertWellFormedResource(t, resource, "Patient")
			pretty, _ := json.MarshalIndent(resource, "    ", "  ")
			t.Logf("  resource:\n    %s", pretty)
		})
	}
}

// TestDeclarativeEngine_Corpus_PatientLinkage_AllSectionsReferencePatient is
// Phase 4 Slice A's PatientRefPath end-to-end proof: every resource every
// already-shipped section rule produces, when given a real engine.PatientRef,
// carries a non-empty patient-link field -- the gap this slice closes (every
// *MappingRules() function used to skip this entirely). Runs against every
// section that has BuildResources/BuildResourcesForRules wiring AND real
// corpus data for at least one vendor (skips silently otherwise, same
// "evidence of prevalence, not correctness" convention every other corpus
// test in this file already follows).
func TestDeclarativeEngine_Corpus_PatientLinkage_AllSectionsReferencePatient(t *testing.T) {
	const patientRef = "Patient/patient-1"

	type linkedRule struct {
		name          string
		path          string // the field expected to carry the reference
		rules         []cdafhir.MappingRule
		multi         bool     // true => use BuildResourcesForRules
		resourceTypes []string // allow-list: BuildResources/BuildResourcesForRules
		// may also emit EmitAsResource sub-resources (Practitioner, etc.) into
		// this same slice -- see BuildResources' doc comment. Only the section's
		// own resource type(s) are expected to carry the patient/subject link.
	}
	cases := []linkedRule{
		{"Allergy", "patient", cdafhir.AllergyMappingRules(), false, []string{"AllergyIntolerance"}},
		{"Medication", "subject", cdafhir.MedicationMappingRules(), true, []string{"MedicationRequest", "MedicationStatement"}},
		{"Problems", "subject", cdafhir.ProblemsMappingRules(), false, []string{"Condition"}},
		{"VitalSigns", "subject", cdafhir.VitalSignsMappingRules(), false, []string{"Observation"}},
		{"Immunization", "patient", cdafhir.ImmunizationMappingRules(), false, []string{"Immunization"}},
		{"Procedure", "subject", cdafhir.ProcedureMappingRules(), false, []string{"Procedure"}},
		{"CareTeam", "subject", cdafhir.CareTeamMappingRules(), false, []string{"CareTeam"}},
		{"Device", "subject", cdafhir.DeviceMappingRules(), false, []string{"DeviceUseStatement"}},
	}

	for _, file := range corpusFiles {
		documentMap := loadCorpusDocumentMap(t, file)
		for _, c := range cases {
			engine := cdafhir.NewDeclarativeEngine()
			engine.PatientRef = patientRef

			var resources []map[string]interface{}
			if c.multi {
				resources, _ = engine.BuildResourcesForRules(documentMap, c.rules)
			} else {
				resources, _ = engine.BuildResources(documentMap, c.rules[0])
			}
			for i, r := range resources {
				if rt, _ := r["resourceType"].(string); !containsStr(c.resourceTypes, rt) {
					continue
				}
				ref, ok := r[c.path].(map[string]interface{})
				if !ok || ref["reference"] != patientRef {
					t.Errorf("%s/%s[%d]: %s = %v, want {reference: %s}", file, c.name, i, c.path, r[c.path], patientRef)
				}
			}
		}
	}
}
