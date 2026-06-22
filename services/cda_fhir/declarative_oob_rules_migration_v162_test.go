// services/cda_fhir/declarative_oob_rules_migration_v162_test.go
//
// Drift guard for V162 (LegalAuthenticator). Reuses migrationV161InsertPattern
// (declarative_oob_rules_migration_v161_test.go) since V162's INSERT column
// list is identical to V161's (header_path included) -- same precedent V156
// set reusing V155's pattern.
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

func parseSeededV162Rules(t testing.TB) []seededHeaderRule {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "database", "migrations", "V162__CDA_Declarative_Mapping_Rules_LegalAuthenticator.sql")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading migration file: %v", err)
	}

	matches := migrationV161InsertPattern.FindAllStringSubmatch(string(data), -1)
	if len(matches) == 0 {
		t.Fatal("found zero INSERT...VALUES(...$rules$...) blocks in the migration -- regex or file content drifted")
	}

	rules := make([]seededHeaderRule, 0, len(matches))
	for _, m := range matches {
		var fields []cdafhir.MappingRow
		if err := json.Unmarshal([]byte(m[5]), &fields); err != nil {
			t.Fatalf("unmarshalling fields JSON for %s/%s: %v\nJSON: %s", m[2], m[3], err, m[5])
		}
		rules = append(rules, seededHeaderRule{
			sectionKey:        m[1],
			headerPath:        m[2],
			fhirResource:      m[3],
			entryMatch:        m[4],
			fields:            fields,
			flattenOrganizers: m[6] == "true",
			requiredPaths:     parseSQLStringArray(m[7]),
		})
	}
	return rules
}

func TestV162Migration_MatchesGoLiteralRules_NoDrift(t *testing.T) {
	seeded := parseSeededV162Rules(t)
	want := cdafhir.LegalAuthenticatorMappingRules()

	if len(seeded) != len(want) {
		t.Fatalf("got %d seeded rules, want %d", len(seeded), len(want))
	}

	for i := range want {
		got := seeded[i]
		w := want[i]
		if got.headerPath != w.HeaderPath {
			t.Fatalf("rule[%d]: header_path = %q, want %q", i, got.headerPath, w.HeaderPath)
		}
		if got.fhirResource != w.FHIRResource {
			t.Errorf("rule[%d] (%s): fhirResource = %q, want %q", i, got.headerPath, got.fhirResource, w.FHIRResource)
		}
		if got.entryMatch != w.EntryMatch {
			t.Errorf("rule[%d] (%s): entry_match = %q, want %q", i, got.headerPath, got.entryMatch, w.EntryMatch)
		}
		if got.flattenOrganizers != w.FlattenOrganizers {
			t.Errorf("rule[%d] (%s): flatten_organizers = %v, want %v", i, got.headerPath, got.flattenOrganizers, w.FlattenOrganizers)
		}
		if !reflect.DeepEqual(got.requiredPaths, w.RequiredPaths) {
			t.Errorf("rule[%d] (%s): required_paths = %v, want %v", i, got.headerPath, got.requiredPaths, w.RequiredPaths)
		}
		if !reflect.DeepEqual(got.fields, w.Fields) {
			t.Errorf("rule[%d] (%s): seeded fields drifted from declarative_oob_rules.go's Go literal.\nseeded: %+v\nwant:   %+v", i, got.headerPath, got.fields, w.Fields)
		}
	}
}
