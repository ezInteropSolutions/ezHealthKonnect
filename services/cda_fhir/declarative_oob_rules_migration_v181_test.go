// services/cda_fhir/declarative_oob_rules_migration_v181_test.go
//
// Drift guard for V181 (first-time OOB seed for ClinicalNoteMappingRules()
// -- a Note Activity entry, templateId 2.16.840.1.113883.10.20.22.4.202,
// mapped to one US Core DocumentReference; see declarative_oob_rules.go's
// ClinicalNoteMappingRules() own doc comment for the real "historyOfPresentIllness"/
// Epic "Progress Notes" gap this closes).
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

func parseSeededV181Rules(t testing.TB) []seededObservationRule {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "database", "migrations", "V181__CDA_Declarative_Mapping_Rules_ClinicalNote.sql")

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

func findSeededV181(t testing.TB, seeded []seededObservationRule, sectionKey, fhirResource string) seededObservationRule {
	t.Helper()
	for _, s := range seeded {
		if s.sectionKey == sectionKey && s.fhirResource == fhirResource {
			return s
		}
	}
	t.Fatalf("no seeded rule found in V181 for sectionKey=%q fhirResource=%q", sectionKey, fhirResource)
	return seededObservationRule{}
}

func TestV181Migration_MatchesGoLiteralRules_NoDrift(t *testing.T) {
	seeded := parseSeededV181Rules(t)
	if len(seeded) != 1 {
		t.Fatalf("got %d seeded rules, want 1 (historyOfPresentIllness/DocumentReference)", len(seeded))
	}

	want := cdafhir.ClinicalNoteMappingRules()[0]
	got := findSeededV181(t, seeded, want.SectionKey, want.FHIRResource)
	if got.entryMatch != want.EntryMatch {
		t.Errorf("%s/%s: entry_match = %q, want %q", want.SectionKey, want.FHIRResource, got.entryMatch, want.EntryMatch)
	}
	if got.flattenOrganizers != want.FlattenOrganizers {
		t.Errorf("%s/%s: flatten_organizers = %v, want %v", want.SectionKey, want.FHIRResource, got.flattenOrganizers, want.FlattenOrganizers)
	}
	if !reflect.DeepEqual(got.fields, want.Fields) {
		t.Errorf("%s/%s: seeded fields drifted from declarative_oob_rules.go's Go literal.\nseeded: %+v\nwant:   %+v",
			want.SectionKey, want.FHIRResource, got.fields, want.Fields)
	}
}
