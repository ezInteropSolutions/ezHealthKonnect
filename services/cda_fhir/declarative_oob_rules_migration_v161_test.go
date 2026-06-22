// services/cda_fhir/declarative_oob_rules_migration_v161_test.go
//
// Drift guard for V161 (Author/Custodian, header-level rules). A separate
// regex from migrationV155InsertPattern/migrationV159InsertPattern, not a
// reuse: V161's INSERT column list adds header_path between section_key and
// fhir_resource -- a genuinely different shape from every prior migration's
// (this is the first rule that isn't section-keyed at all).
package cdafhir_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"testing"

	cdafhir "ezhealthkonnect/services/cda_fhir"
)

type seededHeaderRule struct {
	sectionKey        string
	headerPath        string
	fhirResource      string
	entryMatch        string
	fields            []cdafhir.MappingRow
	flattenOrganizers bool
	requiredPaths     []string
}

var migrationV161InsertPattern = regexp.MustCompile(
	`(?s)VALUES\s*\(\s*'[^']*',\s*'[^']*',\s*'[^']*',\s*'([^']*)',\s*'([^']*)',\s*'([^']*)',\s*'([^']*)',\s*\d+,\s*\$rules\$(.*?)\$rules\$::jsonb,\s*(true|false),\s*ARRAY\[(.*?)\]`,
)

func parseSeededV161Rules(t testing.TB) []seededHeaderRule {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "database", "migrations", "V161__CDA_Declarative_Mapping_Rules_Author_Custodian.sql")

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

func TestV161Migration_MatchesGoLiteralRules_NoDrift(t *testing.T) {
	seeded := parseSeededV161Rules(t)

	want := []cdafhir.MappingRule{
		cdafhir.AuthorMappingRules()[0],
		cdafhir.CustodianMappingRules()[0],
	}

	if len(seeded) != len(want) {
		t.Fatalf("got %d seeded rules, want %d (Author + Custodian)", len(seeded), len(want))
	}

	for i := range want {
		got := seeded[i]
		w := want[i]
		if got.sectionKey != w.SectionKey {
			t.Errorf("rule[%d]: section_key = %q, want %q", i, got.sectionKey, w.SectionKey)
		}
		if got.headerPath != w.HeaderPath {
			t.Fatalf("rule[%d]: header_path = %q, want %q (migration row order must match Go literal order exactly)", i, got.headerPath, w.HeaderPath)
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
