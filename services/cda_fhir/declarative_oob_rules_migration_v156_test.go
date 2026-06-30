// services/cda_fhir/declarative_oob_rules_migration_v156_test.go
//
// Drift guard for V156 (Immunizations). Reuses migrationV155InsertPattern
// (declarative_oob_rules_migration_v155_test.go) since V156's INSERT column
// list is identical (document_type..fields, flatten_organizers, is_system,
// is_public) -- no point recompiling the same regex under a new name. The
// parsing/comparison helpers are still separate per migration file, per
// that file's own doc comment on why these guards stay independent.
//
// V156 itself was then entirely superseded by V176 (performer upgraded to
// PractitionerRole/Practitioner tiers + function, plus new recorded and
// note rows -- see declarative_oob_rules_migration_v176_test.go); only
// structural facts are still checked here.
package cdafhir_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	cdafhir "ezhealthkonnect/services/cda_fhir"
)

func parseSeededV156Rules(t testing.TB) []seededObservationRule {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "database", "migrations", "V156__CDA_Declarative_Mapping_Rules_Immunizations.sql")

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

func TestV156Migration_MatchesGoLiteralRules_NoDrift(t *testing.T) {
	seeded := parseSeededV156Rules(t)
	if len(seeded) != 1 {
		t.Fatalf("got %d seeded rules, want 1 (immunizations)", len(seeded))
	}

	want := cdafhir.ImmunizationMappingRules()[0]
	got := seeded[0]
	if got.sectionKey != want.SectionKey || got.fhirResource != want.FHIRResource {
		t.Fatalf("seeded rule = %s/%s, want %s/%s", got.sectionKey, got.fhirResource, want.SectionKey, want.FHIRResource)
	}
	if got.entryMatch != want.EntryMatch {
		t.Errorf("entry_match = %q, want %q", got.entryMatch, want.EntryMatch)
	}
	if got.flattenOrganizers != want.FlattenOrganizers {
		t.Errorf("flatten_organizers = %v, want %v", got.flattenOrganizers, want.FlattenOrganizers)
	}
	// Fields comparison deliberately omitted -- superseded by V176, see
	// this file's own top doc comment.
}
