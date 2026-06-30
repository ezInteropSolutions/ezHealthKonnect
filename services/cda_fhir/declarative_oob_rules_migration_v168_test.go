// services/cda_fhir/declarative_oob_rules_migration_v168_test.go
//
// Drift guard for V168 (Condition.recorder added to conditionFields(), which
// feeds Problems, Health Concerns, and Encounters' nested diagnosis -- all
// three are re-seeded together in V168 and checked here). Supersedes
// V154's "problems"/"healthConcerns" coverage and V167's "encounters"
// coverage in declarative_oob_rules_migration_test.go and the now-deleted
// declarative_oob_rules_migration_v167_test.go respectively.
//
// V168 itself was then entirely superseded by V175 (recorder fallback
// tier, recordedDate, and note rows added to the same conditionFields() --
// see declarative_oob_rules_migration_v175_test.go); only structural facts
// are still checked here.
package cdafhir_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	cdafhir "ezhealthkonnect/services/cda_fhir"
)

func parseSeededV168Rules(t testing.TB) []seededRule {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "database", "migrations", "V168__CDA_Declarative_Mapping_Rules_Condition_Recorder.sql")

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

func TestV168Migration_MatchesGoLiteralRules_NoDrift(t *testing.T) {
	seeded := parseSeededV168Rules(t)
	if len(seeded) != 3 {
		t.Fatalf("got %d seeded rules, want 3 (problems + healthConcerns + encounters)", len(seeded))
	}

	for _, s := range seeded {
		if s.sectionKey != "problems" && s.sectionKey != "healthConcerns" && s.sectionKey != "encounters" {
			t.Fatalf("unexpected seeded sectionKey %q in V168", s.sectionKey)
		}
	}

	// Fields comparison deliberately omitted for all three rows --
	// superseded by V175, see this file's own top doc comment. Only
	// confirm each row still exists with the expected shape.
	findSeededIn(t, seeded, "problems", "Condition")
	findSeededIn(t, seeded, "healthConcerns", "Condition")
	findSeededIn(t, seeded, "encounters", "Encounter")
}

func findSeededIn(t testing.TB, seeded []seededRule, sectionKey, fhirResource string) seededRule {
	t.Helper()
	for _, s := range seeded {
		if s.sectionKey == sectionKey && s.fhirResource == fhirResource {
			return s
		}
	}
	t.Fatalf("no seeded rule found in V168 for sectionKey=%q fhirResource=%q", sectionKey, fhirResource)
	return seededRule{}
}
