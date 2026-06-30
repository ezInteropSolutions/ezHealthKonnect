// services/cda_fhir/declarative_oob_rules_migration_v179_test.go
//
// Drift guard for V179 (Assessment Scale Observation COMP-children fan-out
// for functionalStatus -- see declarative_oob_rules.go's
// FunctionalStatusMappingRules() own doc comment, and V179's migration file
// doc comment, for the real gap this closes: AUDIT-C/PHQ-2 question/answer
// Observations nested under a code.nullFlavor="UNK" Assessment Scale
// Observation shell were silently discarded entirely, not just the shell).
// Supersedes V178 for functionalStatus/Observation's Fields content ONLY --
// V178 still owns entry_match/flatten_organizers for functionalStatus
// (unchanged by this migration), and remains the owner of the other 5
// observationRule() sections (vitalSigns/results/labResults/mentalStatus/
// socialHistory).
//
// functionalStatus/Observation's Fields content was THEN superseded again
// by V184 (new referenceRange[0].text row -- see
// declarative_oob_rules_migration_v184_test.go). This file now only confirms
// existence, no longer asserting Fields.
package cdafhir_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	cdafhir "ezhealthkonnect/services/cda_fhir"
)

func parseSeededV179Rules(t testing.TB) []seededObservationRule {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "database", "migrations", "V179__CDA_Declarative_Mapping_Rules_FunctionalStatus_AssessmentScale_COMP.sql")

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

func findSeededV179(t testing.TB, seeded []seededObservationRule, sectionKey, fhirResource string) seededObservationRule {
	t.Helper()
	for _, s := range seeded {
		if s.sectionKey == sectionKey && s.fhirResource == fhirResource {
			return s
		}
	}
	t.Fatalf("no seeded rule found in V179 for sectionKey=%q fhirResource=%q", sectionKey, fhirResource)
	return seededObservationRule{}
}

func TestV179Migration_MatchesGoLiteralRules_NoDrift(t *testing.T) {
	seeded := parseSeededV179Rules(t)
	if len(seeded) != 1 {
		t.Fatalf("got %d seeded rules, want 1 (functionalStatus)", len(seeded))
	}

	findSeededV179(t, seeded, "functionalStatus", "Observation")
}
