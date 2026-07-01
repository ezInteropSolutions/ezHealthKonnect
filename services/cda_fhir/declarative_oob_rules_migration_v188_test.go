// services/cda_fhir/declarative_oob_rules_migration_v188_test.go
//
// Drift guard for V188 (Condition.asserter + Condition.onsetAge additions
// to conditionFields() via C-CDA on FHIR IG alignment, plus
// AllergyIntolerance.recorder, AllergyIntolerance.recordedDate, and
// AllergyIntolerance.asserter).
//
// V188 supersedes:
//   - V175 for problems/Condition and healthConcerns/Condition Fields content
//   - V154 for allergiesAndIntolerances/AllergyIntolerance Fields content
//   - V183 for encounters/Encounter Fields content (which in turn superseded V175)
//
// All four Go literal functions below still return the single source of truth;
// V188's SQL is hand-synced to match them and this test is the automated
// drift guard.
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

func parseSeededV188Rules(t testing.TB) []seededRule {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "database", "migrations", "V188__CDA_Declarative_Mapping_Rules_Condition_Allergy_Asserter_OnsetAge.sql")

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

func findSeededV188(t testing.TB, seeded []seededRule, sectionKey, fhirResource string) seededRule {
	t.Helper()
	for _, s := range seeded {
		if s.sectionKey == sectionKey && s.fhirResource == fhirResource {
			return s
		}
	}
	t.Fatalf("no seeded rule found in V188 for sectionKey=%q fhirResource=%q", sectionKey, fhirResource)
	return seededRule{}
}

func TestV188Migration_MatchesGoLiteralRules_NoDrift(t *testing.T) {
	seeded := parseSeededV188Rules(t)
	if len(seeded) != 4 {
		t.Fatalf("got %d seeded rules, want 4 (problems + healthConcerns + allergiesAndIntolerances + encounters)", len(seeded))
	}

	for _, tc := range []struct {
		sectionKey, fhirResource string
		want                     cdafhir.MappingRule
	}{
		// problems/Condition: V188 supersedes V175 (added asserter + onsetAge)
		{"problems", "Condition", cdafhir.ProblemsMappingRules()[0]},
		// healthConcerns/Condition: V188 supersedes V175
		{"healthConcerns", "Condition", cdafhir.HealthConcernsMappingRules()[1]},
		// allergiesAndIntolerances/AllergyIntolerance: V188 supersedes V154
		{"allergiesAndIntolerances", "AllergyIntolerance", cdafhir.AllergyMappingRules()[0]},
		// encounters/Encounter: V188 supersedes V183 (which superseded V175)
		{"encounters", "Encounter", cdafhir.EncounterMappingRules()[0]},
	} {
		got := findSeededV188(t, seeded, tc.sectionKey, tc.fhirResource)
		if got.entryMatch != tc.want.EntryMatch {
			t.Errorf("%s/%s: entry_match = %q, want %q",
				tc.sectionKey, tc.fhirResource, got.entryMatch, tc.want.EntryMatch)
		}
		if !reflect.DeepEqual(got.fields, tc.want.Fields) {
			t.Errorf("%s/%s: seeded fields drifted from declarative_oob_rules.go's Go literal.\nseeded: %+v\nwant:   %+v",
				tc.sectionKey, tc.fhirResource, got.fields, tc.want.Fields)
		}
	}
}
