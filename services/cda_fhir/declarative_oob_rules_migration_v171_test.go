// services/cda_fhir/declarative_oob_rules_migration_v171_test.go
//
// Drift guard for V171 (identifier row added to medicationCommonRows(),
// which PlanOfCareMappingRules()'s substanceAdministration branch reuses
// verbatim for all 4 Plan-of-Care section aliases: carePlan, planOfCare,
// assessmentAndPlan, planOfTreatment). Supersedes V158's MedicationRequest
// coverage for these 4 sections (skipped in
// declarative_oob_rules_migration_v158_test.go).
//
// V171 itself was, in turn, entirely superseded by V173 (requester rows --
// see declarative_oob_rules_migration_v173_test.go): every one of the 4
// rows V171 seeded got a Fields-list change, so this file's Fields
// comparison is gone BY DESIGN; only structural facts (row count,
// entry_match) that V173 doesn't independently re-verify are still checked.
package cdafhir_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	cdafhir "ezhealthkonnect/services/cda_fhir"
)

func parseSeededV171Rules(t testing.TB) []seededObservationRule {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "database", "migrations", "V171__CDA_Declarative_Mapping_Rules_PlanOfCare_Medication_Identifier.sql")

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

func findSeededV171(t testing.TB, seeded []seededObservationRule, sectionKey, fhirResource string) seededObservationRule {
	t.Helper()
	for _, s := range seeded {
		if s.sectionKey == sectionKey && s.fhirResource == fhirResource {
			return s
		}
	}
	t.Fatalf("no seeded rule found in V171 for sectionKey=%q fhirResource=%q", sectionKey, fhirResource)
	return seededObservationRule{}
}

func TestV171Migration_MatchesGoLiteralRules_NoDrift(t *testing.T) {
	seeded := parseSeededV171Rules(t)
	if len(seeded) != 4 {
		t.Fatalf("got %d seeded rules, want 4 (carePlan + planOfCare + assessmentAndPlan + planOfTreatment, all MedicationRequest)", len(seeded))
	}

	for _, sectionKey := range []string{"carePlan", "planOfCare", "assessmentAndPlan", "planOfTreatment"} {
		got := findSeededV171(t, seeded, sectionKey, "MedicationRequest")
		if got.entryMatch != "entryType=substanceAdministration" {
			t.Errorf("%s/MedicationRequest: entry_match = %q, want %q", sectionKey, got.entryMatch, "entryType=substanceAdministration")
		}
		// Fields comparison deliberately omitted -- superseded by V173, see
		// this file's own top doc comment.
	}
}
