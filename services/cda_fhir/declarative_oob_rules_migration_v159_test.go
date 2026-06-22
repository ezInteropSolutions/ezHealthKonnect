// services/cda_fhir/declarative_oob_rules_migration_v159_test.go
//
// Drift guard for V159 (CareTeam). A separate regex from
// migrationV155InsertPattern, not a reuse: V159's INSERT column list adds
// required_paths (a Postgres TEXT[] literal, "ARRAY['participant']") after
// flatten_organizers -- a genuinely different shape, same precedent V155
// itself set over V154.
package cdafhir_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"

	cdafhir "ezhealthkonnect/services/cda_fhir"
)

type seededCareTeamRule struct {
	sectionKey        string
	fhirResource      string
	entryMatch        string
	fields            []cdafhir.MappingRow
	flattenOrganizers bool
	requiredPaths     []string
}

var migrationV159InsertPattern = regexp.MustCompile(
	`(?s)VALUES\s*\(\s*'[^']*',\s*'[^']*',\s*'[^']*',\s*'([^']*)',\s*'([^']*)',\s*'([^']*)',\s*\d+,\s*\$rules\$(.*?)\$rules\$::jsonb,\s*(true|false),\s*ARRAY\[(.*?)\]`,
)

// parseSQLStringArray parses a Postgres ARRAY['a','b'] literal's inner text
// (already extracted by the regex above) into a []string.
func parseSQLStringArray(inner string) []string {
	inner = strings.TrimSpace(inner)
	if inner == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(inner, ",") {
		out = append(out, strings.Trim(strings.TrimSpace(part), "'"))
	}
	return out
}

func parseSeededV159Rules(t testing.TB) []seededCareTeamRule {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "database", "migrations", "V159__CDA_Declarative_Mapping_Rules_CareTeam.sql")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading migration file: %v", err)
	}

	matches := migrationV159InsertPattern.FindAllStringSubmatch(string(data), -1)
	if len(matches) == 0 {
		t.Fatal("found zero INSERT...VALUES(...$rules$...) blocks in the migration -- regex or file content drifted")
	}

	rules := make([]seededCareTeamRule, 0, len(matches))
	for _, m := range matches {
		var fields []cdafhir.MappingRow
		if err := json.Unmarshal([]byte(m[4]), &fields); err != nil {
			t.Fatalf("unmarshalling fields JSON for %s/%s: %v\nJSON: %s", m[1], m[2], err, m[4])
		}
		rules = append(rules, seededCareTeamRule{
			sectionKey:        m[1],
			fhirResource:      m[2],
			entryMatch:        m[3],
			fields:            fields,
			flattenOrganizers: m[5] == "true",
			requiredPaths:     parseSQLStringArray(m[6]),
		})
	}
	return rules
}

func TestV159Migration_MatchesGoLiteralRules_NoDrift(t *testing.T) {
	seeded := parseSeededV159Rules(t)
	want := cdafhir.CareTeamMappingRules()

	if len(seeded) != len(want) {
		t.Fatalf("got %d seeded rules, want %d (one per Care Team section-key alias)", len(seeded), len(want))
	}

	for i := range want {
		got := seeded[i]
		w := want[i]
		if got.sectionKey != w.SectionKey {
			t.Fatalf("rule[%d]: sectionKey = %q, want %q (migration row order must match CareTeamMappingRules' alias order exactly)", i, got.sectionKey, w.SectionKey)
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
		if !reflect.DeepEqual(got.requiredPaths, w.RequiredPaths) {
			t.Errorf("rule[%d] (%s/%s): required_paths = %v, want %v", i, got.sectionKey, got.fhirResource, got.requiredPaths, w.RequiredPaths)
		}
		if !reflect.DeepEqual(got.fields, w.Fields) {
			t.Errorf("rule[%d] (%s/%s): seeded fields drifted from declarative_oob_rules.go's Go literal.\nseeded: %+v\nwant:   %+v", i, got.sectionKey, got.fhirResource, got.fields, w.Fields)
		}
	}
}
