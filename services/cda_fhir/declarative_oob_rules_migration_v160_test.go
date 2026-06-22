// services/cda_fhir/declarative_oob_rules_migration_v160_test.go
//
// Drift guard for V160 (Coverage/FamilyMemberHistory/Device). Reuses
// migrationV155InsertPattern (declarative_oob_rules_migration_v155_test.go)
// since V160's INSERT column list is identical (document_type..fields,
// flatten_organizers, is_system, is_public) -- same precedent V156 set.
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

func parseSeededV160Rules(t testing.TB) []seededObservationRule {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "database", "migrations", "V160__CDA_Declarative_Mapping_Rules_Coverage_FamilyHistory_Device.sql")

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

func TestV160Migration_MatchesGoLiteralRules_NoDrift(t *testing.T) {
	seeded := parseSeededV160Rules(t)

	var want []cdafhir.MappingRule
	want = append(want, cdafhir.CoverageMappingRules()...)
	want = append(want, cdafhir.FamilyMemberHistoryMappingRules()[0])
	want = append(want, cdafhir.DeviceMappingRules()[0])

	if len(seeded) != len(want) {
		t.Fatalf("got %d seeded rules, want %d (2 Coverage aliases + FamilyMemberHistory + Device)", len(seeded), len(want))
	}

	for i := range want {
		got := seeded[i]
		w := want[i]
		if got.sectionKey != w.SectionKey {
			t.Fatalf("rule[%d]: sectionKey = %q, want %q (migration row order must match Go literal order exactly)", i, got.sectionKey, w.SectionKey)
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
		if !reflect.DeepEqual(got.fields, w.Fields) {
			t.Errorf("rule[%d] (%s/%s): seeded fields drifted from declarative_oob_rules.go's Go literal.\nseeded: %+v\nwant:   %+v", i, got.sectionKey, got.fhirResource, got.fields, w.Fields)
		}
	}
}
