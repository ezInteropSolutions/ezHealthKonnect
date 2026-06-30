// services/cda_fhir/declarative_oob_rules_migration_v183_test.go
//
// Drift guard for V183 (moodCode-aware status row + location ScopeFallbacks
// fix to encounterFields() -- see that function's own doc comment in
// declarative_oob_rules.go for the real Ascension Wisconsin Plan-of-Care
// gap this closes). Supersedes V175 for the encounters/Encounter row's
// Fields content only -- V175 remains the owner of the problems/Condition
// and healthConcerns/Condition rows seeded alongside it.
//
// No parallel row is seeded anywhere for PlanOfCareMappingRules() itself:
// the Appointment->Encounter swap for Plan-of-Care's entryType=encounter
// branch is a per-document RUNTIME decision (declarative_document_mapper.go's
// planOfCareEncounterTarget resolution + applyPlanOfCareEncounterTarget),
// not a change to any OOB rule's hardcoded definition -- seeding a second
// static row here would misleadingly imply two rules coexist statically
// when only one is ever active per document, chosen at runtime.
package cdafhir_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	cdafhir "ezhealthkonnect/services/cda_fhir"
)

func parseSeededV183Rules(t testing.TB) []seededRule {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "database", "migrations", "V183__CDA_Declarative_Mapping_Rules_Encounter_PlannedMood_Location.sql")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading migration file: %v", err)
	}

	matches := migrationInsertPattern.FindAllStringSubmatch(string(data), -1)
	if len(matches) == 0 {
		t.Fatal("found zero INSERT...VALUES(...$rules$...) blocks in the migration -- regex or file content drifted")
	}

	rules := make([]seededRule, 0, len(matches))
	for _, m := range matches {
		var fields []cdafhir.MappingRow
		if err := json.Unmarshal([]byte(m[4]), &fields); err != nil {
			t.Fatalf("unmarshalling fields JSON for %s/%s: %v\nJSON: %s", m[1], m[2], err, m[4])
		}
		rules = append(rules, seededRule{
			sectionKey:   m[1],
			fhirResource: m[2],
			entryMatch:   m[3],
			fields:       fields,
		})
	}
	return rules
}

func TestV183Migration_MatchesGoLiteralRules_NoDrift(t *testing.T) {
	seeded := parseSeededV183Rules(t)
	if len(seeded) != 1 {
		t.Fatalf("got %d seeded rules, want 1 (encounters/Encounter)", len(seeded))
	}

	want := cdafhir.EncounterMappingRules()[0]
	got := seeded[0]
	if got.sectionKey != want.SectionKey || got.fhirResource != want.FHIRResource {
		t.Fatalf("seeded rule = %s/%s, want %s/%s", got.sectionKey, got.fhirResource, want.SectionKey, want.FHIRResource)
	}
	if got.entryMatch != want.EntryMatch {
		t.Errorf("entry_match = %q, want %q", got.entryMatch, want.EntryMatch)
	}
	if !reflect.DeepEqual(got.fields, want.Fields) {
		t.Errorf("seeded fields drifted from declarative_oob_rules.go's Go literal.\nseeded: %+v\nwant:   %+v", got.fields, want.Fields)
	}
}
