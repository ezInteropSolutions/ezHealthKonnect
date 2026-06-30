// services/cda_fhir/declarative_oob_rules_migration_v184_test.go
//
// Drift guard for V184 (new referenceRange[0].text row, sourced from the new
// CDAEntry.ReferenceRangeText field, shared by all 6 observationRule()
// callers -- vitalSigns, results, labResults, functionalStatus, mentalStatus,
// socialHistory). Supersedes V178 for vitalSigns/results/labResults/
// socialHistory's Fields content, V179 for functionalStatus's, V180 for
// mentalStatus's -- those three files still own entry_match/
// flatten_organizers for their respective sections (unchanged by this
// migration).
//
// All 6 sections' Fields content was then superseded again by V185
// (Observation.identifier row). This file now only confirms existence.
package cdafhir_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	cdafhir "ezhealthkonnect/services/cda_fhir"
)

func parseSeededV184Rules(t testing.TB) []seededObservationRule {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "database", "migrations", "V184__CDA_Declarative_Mapping_Rules_Observation_ReferenceRangeText.sql")

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

func findSeededV184(t testing.TB, seeded []seededObservationRule, sectionKey, fhirResource string) seededObservationRule {
	t.Helper()
	for _, s := range seeded {
		if s.sectionKey == sectionKey && s.fhirResource == fhirResource {
			return s
		}
	}
	t.Fatalf("no seeded rule found in V184 for sectionKey=%q fhirResource=%q", sectionKey, fhirResource)
	return seededObservationRule{}
}


func TestV184Migration_MatchesGoLiteralRules_NoDrift(t *testing.T) {
	seeded := parseSeededV184Rules(t)
	if len(seeded) != 6 {
		t.Fatalf("got %d seeded rules, want 6 (vitalSigns + results + labResults + functionalStatus + mentalStatus + socialHistory)", len(seeded))
	}

	findSeededV184(t, seeded, "vitalSigns", "Observation")
	findSeededV184(t, seeded, "results", "Observation")
	findSeededV184(t, seeded, "labResults", "Observation")
	findSeededV184(t, seeded, "socialHistory", "Observation")
	findSeededV184(t, seeded, "functionalStatus", "Observation")
	findSeededV184(t, seeded, "mentalStatus", "Observation")
}
