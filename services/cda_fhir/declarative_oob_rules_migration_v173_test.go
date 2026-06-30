// services/cda_fhir/declarative_oob_rules_migration_v173_test.go
//
// Drift guard for V173 (requesterFromPerformer/requesterFromAuthor --
// PractitionerRole-emitting requester tiers -- added to
// medicationRequestFields(), which PlanOfCareMappingRules()'s
// substanceAdministration branch reuses verbatim for all 4 Plan-of-Care
// section aliases: carePlan, planOfCare, assessmentAndPlan,
// planOfTreatment). Supersedes V171's coverage entirely.
//
// V173 itself was then entirely superseded by V174 (requester priority
// reversed to author-first per the IG, plus a new note row -- see
// declarative_oob_rules_migration_v174_test.go); only structural facts are
// still checked here.
package cdafhir_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	cdafhir "ezhealthkonnect/services/cda_fhir"
)

func parseSeededV173Rules(t testing.TB) []seededObservationRule {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "database", "migrations", "V173__CDA_Declarative_Mapping_Rules_PlanOfCare_Medication_Requester.sql")

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

func findSeededV173(t testing.TB, seeded []seededObservationRule, sectionKey, fhirResource string) seededObservationRule {
	t.Helper()
	for _, s := range seeded {
		if s.sectionKey == sectionKey && s.fhirResource == fhirResource {
			return s
		}
	}
	t.Fatalf("no seeded rule found in V173 for sectionKey=%q fhirResource=%q", sectionKey, fhirResource)
	return seededObservationRule{}
}

func TestV173Migration_MatchesGoLiteralRules_NoDrift(t *testing.T) {
	seeded := parseSeededV173Rules(t)
	if len(seeded) != 4 {
		t.Fatalf("got %d seeded rules, want 4 (carePlan + planOfCare + assessmentAndPlan + planOfTreatment, all MedicationRequest)", len(seeded))
	}

	for _, sectionKey := range []string{"carePlan", "planOfCare", "assessmentAndPlan", "planOfTreatment"} {
		findSeededV173(t, seeded, sectionKey, "MedicationRequest")
		// entry_match/Fields comparison deliberately omitted -- superseded
		// by V174, see this file's own top doc comment.
	}
}
