// services/cda_fhir/declarative_oob_rules_migration_v175_test.go
//
// Drift guard for V175 (three fixes to conditionFields(): a
// barePractitionerRow fallback tier for Condition.recorder when the author
// has no representedOrganization, a new Condition.recordedDate row from
// /author/time, and a new Condition.note row reading C-CDA's Note Activity
// -- all three feed Problems, Health Concerns, and Encounters' nested
// diagnosis, re-seeded together here exactly like V168 before it).
// Supersedes V168 entirely for Fields content -- see that file's own
// narrowed drift test.
//
// The encounters/Encounter row was THEN superseded by V183 (moodCode-aware
// status row + location ScopeFallbacks fix -- see
// declarative_oob_rules_migration_v183_test.go) and is deliberately skipped
// below too, the same way V158's test skips MedicationRequest/ServiceRequest.
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

func parseSeededV175Rules(t testing.TB) []seededRule {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "database", "migrations", "V175__CDA_Declarative_Mapping_Rules_Condition_Note_RecordedDate.sql")

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

func findSeededV175(t testing.TB, seeded []seededRule, sectionKey, fhirResource string) seededRule {
	t.Helper()
	for _, s := range seeded {
		if s.sectionKey == sectionKey && s.fhirResource == fhirResource {
			return s
		}
	}
	t.Fatalf("no seeded rule found in V175 for sectionKey=%q fhirResource=%q", sectionKey, fhirResource)
	return seededRule{}
}

func TestV175Migration_MatchesGoLiteralRules_NoDrift(t *testing.T) {
	seeded := parseSeededV175Rules(t)
	if len(seeded) != 3 {
		t.Fatalf("got %d seeded rules, want 3 (problems + healthConcerns + encounters)", len(seeded))
	}

	for _, tc := range []struct {
		sectionKey, fhirResource string
		want                     cdafhir.MappingRule
	}{
		{"problems", "Condition", cdafhir.ProblemsMappingRules()[0]},
		{"healthConcerns", "Condition", cdafhir.HealthConcernsMappingRules()[1]}, // [0] is now the assessment-scale Observation rule (V186)
		{"encounters", "Encounter", cdafhir.EncounterMappingRules()[0]},
	} {
		got := findSeededV175(t, seeded, tc.sectionKey, tc.fhirResource)
		if got.entryMatch != tc.want.EntryMatch {
			t.Errorf("%s/%s: entry_match = %q, want %q", tc.sectionKey, tc.fhirResource, got.entryMatch, tc.want.EntryMatch)
		}
		if tc.sectionKey == "encounters" && tc.fhirResource == "Encounter" {
			continue // superseded by V183 -- see this file's own top doc comment.
		}
		if !reflect.DeepEqual(got.fields, tc.want.Fields) {
			t.Errorf("%s/%s: seeded fields drifted from declarative_oob_rules.go's Go literal.\nseeded: %+v\nwant:   %+v",
				tc.sectionKey, tc.fhirResource, got.fields, tc.want.Fields)
		}
	}
}
