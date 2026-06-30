// services/cda_fhir/declarative_oob_rules_migration_v158_test.go
//
// Drift guard for V158 (Goals + Plan-of-Care's 4 section-key aliases).
// Reuses migrationV155InsertPattern -- every one of the 25 rows here
// (1 goals + 4 aliases x 6 rules) is its own independent
// INSERT...VALUES(...) statement, same shape as V155/V157, specifically so
// this regex (which requires a literal "VALUES" immediately before each
// tuple) finds all 25.
//
// The 4 MedicationRequest rows (one per alias) were superseded by V171
// (identifier added to medicationCommonRows(), which
// PlanOfCareMappingRules()'s substanceAdministration branch reuses verbatim
// -- see declarative_oob_rules_migration_v171_test.go) and are
// deliberately skipped below; this file's on-disk content for them is now
// stale BY DESIGN.
//
// The 4 ServiceRequest rows (one per alias) were THEN superseded by V182
// (entry.text -> code.text fallback row -- see
// declarative_oob_rules_migration_v182_test.go) and are deliberately
// skipped below too, the same way.
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

func parseSeededV158Rules(t testing.TB) []seededObservationRule {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "database", "migrations", "V158__CDA_Declarative_Mapping_Rules_CarePlan_Goal.sql")

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

func TestV158Migration_MatchesGoLiteralRules_NoDrift(t *testing.T) {
	seeded := parseSeededV158Rules(t)

	var want []cdafhir.MappingRule
	want = append(want, cdafhir.GoalMappingRules()[0])
	want = append(want, cdafhir.PlanOfCareMappingRules()...)

	if len(seeded) != len(want) {
		t.Fatalf("got %d seeded rules, want %d (1 goals + 4 aliases x 6 Plan-of-Care rules)", len(seeded), len(want))
	}

	for i := range want {
		got := seeded[i]
		w := want[i]
		if got.sectionKey != w.SectionKey {
			t.Fatalf("rule[%d]: sectionKey = %q, want %q (migration row order must match PlanOfCareMappingRules' alias/rule order exactly)", i, got.sectionKey, w.SectionKey)
		}
		if got.fhirResource != w.FHIRResource {
			t.Errorf("rule[%d] (%s): fhirResource = %q, want %q", i, got.sectionKey, got.fhirResource, w.FHIRResource)
		}
		if got.entryMatch != w.EntryMatch {
			t.Errorf("rule[%d] (%s/%s): entry_match = %q, want %q", i, got.sectionKey, got.fhirResource, got.entryMatch, w.EntryMatch)
		}
		if got.flattenOrganizers != w.FlattenOrganizers {
			t.Errorf("rule[%d] (%s/%s): flatten_organizers = %v, want %v", i, got.sectionKey, got.fhirResource, got.flattenOrganizers, w.FlattenOrganizers)
		}
		if w.FHIRResource == "MedicationRequest" || w.FHIRResource == "ServiceRequest" {
			continue // superseded by V171/V182 respectively -- see this file's own top doc comment.
		}
		if !reflect.DeepEqual(got.fields, w.Fields) {
			t.Errorf("rule[%d] (%s/%s): seeded fields drifted from declarative_oob_rules.go's Go literal.\nseeded: %+v\nwant:   %+v", i, got.sectionKey, got.fhirResource, got.fields, w.Fields)
		}
	}
}
