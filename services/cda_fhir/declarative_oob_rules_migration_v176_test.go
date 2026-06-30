// services/cda_fhir/declarative_oob_rules_migration_v176_test.go
//
// Drift guard for V176 (three fixes to ImmunizationMappingRules():
// performer upgraded from a display-only string to a tiered
// assignedEntityRoleRow -> barePractitionerRow fallback emitting
// PractitionerRole/Practitioner/Organization, plus a new
// performer[0].function="AP" row, a new recorded row from authors[0].time,
// and a new note row from Comment Activity). Reuses migrationV155InsertPattern
// since V176's INSERT column list is identical to V156's (document_type..
// fields, flatten_organizers, is_system, is_public). Supersedes V156
// entirely for Fields content -- see that file's own narrowed drift test.
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

func parseSeededV176Rules(t testing.TB) []seededObservationRule {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "database", "migrations", "V176__CDA_Declarative_Mapping_Rules_Immunization_Performer_Recorded_Note.sql")

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

func TestV176Migration_MatchesGoLiteralRules_NoDrift(t *testing.T) {
	seeded := parseSeededV176Rules(t)
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
	if !reflect.DeepEqual(got.fields, want.Fields) {
		t.Errorf("seeded fields drifted from declarative_oob_rules.go's Go literal.\nseeded: %+v\nwant:   %+v", got.fields, want.Fields)
	}
}
