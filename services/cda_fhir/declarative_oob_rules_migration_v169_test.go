// services/cda_fhir/declarative_oob_rules_migration_v169_test.go
//
// Drift guard for V169 (code.text-from-narrative row added to
// observationRule(), which feeds vitalSigns, results, socialHistory,
// labResults, functionalStatus, and mentalStatus -- all 6 were re-seeded
// together in V169). Supersedes V155's vitalSigns/results/socialHistory
// coverage (removed from declarative_oob_rules_migration_v155_test.go) and
// V163's labResults/functionalStatus/mentalStatus coverage (the now-deleted
// declarative_oob_rules_migration_v163_test.go).
//
// socialHistory/Observation's Fields content was THEN superseded by V177
// (SDOH Assessment Scale fan-out), and ALL 6 sections' Fields content was
// THEN superseded again by V178 (Observation.performer upgraded to the
// PractitionerRole/Practitioner tiered pattern -- see
// declarative_oob_rules_migration_v178_test.go). This file now only
// confirms all 6 rows still exist in V169's own migration text (a content
// drift guard on the file itself, not on the live Go literal), and no
// longer asserts Fields for any of them.
package cdafhir_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	cdafhir "ezhealthkonnect/services/cda_fhir"
)

func parseSeededV169Rules(t testing.TB) []seededObservationRule {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "database", "migrations", "V169__CDA_Declarative_Mapping_Rules_Observation_CodeText_Narrative.sql")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading migration file: %v", err)
	}

	matches := migrationV155InsertPattern.FindAllStringSubmatch(string(data), -1)
	if len(matches) == 0 {
		t.Fatal("found zero INSERT...VALUES(...$rules$...) blocks in the migration -- regex or file content drifted")
	}

	rules := make([]seededObservationRule, 0, len(matches))
	for _, m := range matches {
		var fields []cdafhir.MappingRow
		if err := json.Unmarshal([]byte(m[4]), &fields); err != nil {
			t.Fatalf("unmarshalling fields JSON for %s/%s: %v\nJSON: %s", m[1], m[2], err, m[4])
		}
		rules = append(rules, seededObservationRule{
			sectionKey:        m[1],
			fhirResource:      m[2],
			entryMatch:        m[3],
			fields:            fields,
			flattenOrganizers: m[5] == "true",
		})
	}
	return rules
}

func findSeededV169(t testing.TB, seeded []seededObservationRule, sectionKey, fhirResource string) seededObservationRule {
	t.Helper()
	for _, s := range seeded {
		if s.sectionKey == sectionKey && s.fhirResource == fhirResource {
			return s
		}
	}
	t.Fatalf("no seeded rule found in V169 for sectionKey=%q fhirResource=%q", sectionKey, fhirResource)
	return seededObservationRule{}
}

func TestV169Migration_MatchesGoLiteralRules_NoDrift(t *testing.T) {
	seeded := parseSeededV169Rules(t)
	if len(seeded) != 6 {
		t.Fatalf("got %d seeded rules, want 6 (vitalSigns + results + socialHistory + labResults + functionalStatus + mentalStatus)", len(seeded))
	}

	// Fields content for all 6 sections is now superseded by V178 (and
	// socialHistory was already superseded by V177 before that) -- only
	// existence is checked here, see this file's own top doc comment.
	findSeededV169(t, seeded, "vitalSigns", "Observation")
	findSeededV169(t, seeded, "results", "Observation")
	findSeededV169(t, seeded, "labResults", "Observation")
	findSeededV169(t, seeded, "functionalStatus", "Observation")
	findSeededV169(t, seeded, "mentalStatus", "Observation")
	findSeededV169(t, seeded, "socialHistory", "Observation")
}
