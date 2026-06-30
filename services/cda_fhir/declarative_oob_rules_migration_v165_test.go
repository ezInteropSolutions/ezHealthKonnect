// services/cda_fhir/declarative_oob_rules_migration_v165_test.go
//
// Drift guard for V165, which retargets THREE prior migrations' own drift
// guards:
//   - Author/LegalAuthenticator (previously V161/V162): those seeds never
//     gained the organizationLinkRow PractitionerRole this slice adds, and
//     V161's row was, additionally, unrecoverably corrupted by V162's
//     ON CONFLICT collision (see V165's own doc comment) -- there is no
//     correct V161 state left to drift-check against.
//   - CareTeam (previously V164): gained
//     "emitAsResourceRequiredPaths": ["organization"] on its existing
//     PractitionerRole row.
//
// skip_empty_check is checked here for header rules (Author/
// LegalAuthenticator) even though V161's/V162's own drift tests never did --
// exactly the column whose leaked value (Author's true surviving onto
// LegalAuthenticator's row after the collision) was this slice's first
// symptom of the corruption.
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

type seededHeaderRuleV165 struct {
	sectionKey        string
	headerPath        string
	fhirResource      string
	entryMatch        string
	fields            []cdafhir.MappingRow
	flattenOrganizers bool
	requiredPaths     []string
	skipEmptyCheck    bool
}

type seededCareTeamRuleV165 struct {
	sectionKey        string
	fhirResource      string
	entryMatch        string
	fields            []cdafhir.MappingRow
	flattenOrganizers bool
	requiredPaths     []string
}

// parseSQLStringArray parses a Postgres ARRAY['a','b'] literal's inner text
// (already extracted by a regex below) into a []string. Shared across every
// declarative_oob_rules_migration_v*_test.go file in this package that still
// needs it (v155/156/157/158/160/163's own parseSeededVNNNRules functions
// all call this one, defined here since this is currently the latest
// migration test file to need it) -- do not rename or remove without
// updating every caller.
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

var migrationV165HeaderInsertPattern = regexp.MustCompile(
	`(?s)VALUES\s*\(\s*'[^']*',\s*'[^']*',\s*'[^']*',\s*'([^']*)',\s*'([^']*)',\s*'([^']*)',\s*'([^']*)',\s*\d+,\s*\$rules\$(.*?)\$rules\$::jsonb,\s*(true|false),\s*ARRAY\[(.*?)\](?:::text\[\])?,\s*(true|false)`,
)

var migrationV165CareTeamInsertPattern = regexp.MustCompile(
	`(?s)VALUES\s*\(\s*'[^']*',\s*'[^']*',\s*'[^']*',\s*'([^']*)',\s*'([^']*)',\s*'([^']*)',\s*\d+,\s*\$rules\$(.*?)\$rules\$::jsonb,\s*(true|false),\s*ARRAY\[(.*?)\]`,
)

func readMigrationFile(t testing.TB, filename string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "database", "migrations", filename)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading migration file: %v", err)
	}
	return string(data)
}

func parseSeededV165HeaderRules(t testing.TB) []seededHeaderRuleV165 {
	t.Helper()
	data := readMigrationFile(t, "V165__CDA_Declarative_Mapping_Rules_Header_Constraint_Fix_And_PractitionerRole.sql")

	matches := migrationV165HeaderInsertPattern.FindAllStringSubmatch(data, -1)
	if len(matches) == 0 {
		t.Fatal("found zero header-shaped INSERT...VALUES(...$rules$...) blocks -- regex or file content drifted")
	}

	rules := make([]seededHeaderRuleV165, 0, len(matches))
	for _, m := range matches {
		var fields []cdafhir.MappingRow
		if err := json.Unmarshal([]byte(m[5]), &fields); err != nil {
			t.Fatalf("unmarshalling fields JSON for %s/%s: %v\nJSON: %s", m[2], m[3], err, m[5])
		}
		rules = append(rules, seededHeaderRuleV165{
			sectionKey:        m[1],
			headerPath:        m[2],
			fhirResource:      m[3],
			entryMatch:        m[4],
			fields:            fields,
			flattenOrganizers: m[6] == "true",
			requiredPaths:     parseSQLStringArray(m[7]),
			skipEmptyCheck:    m[8] == "true",
		})
	}
	return rules
}

func parseSeededV165CareTeamRules(t testing.TB) []seededCareTeamRuleV165 {
	t.Helper()
	data := readMigrationFile(t, "V165__CDA_Declarative_Mapping_Rules_Header_Constraint_Fix_And_PractitionerRole.sql")

	matches := migrationV165CareTeamInsertPattern.FindAllStringSubmatch(data, -1)
	if len(matches) == 0 {
		t.Fatal("found zero CareTeam-shaped INSERT...VALUES(...$rules$...) blocks -- regex or file content drifted")
	}

	rules := make([]seededCareTeamRuleV165, 0, len(matches))
	for _, m := range matches {
		var fields []cdafhir.MappingRow
		if err := json.Unmarshal([]byte(m[4]), &fields); err != nil {
			t.Fatalf("unmarshalling fields JSON for %s/%s: %v\nJSON: %s", m[1], m[2], err, m[4])
		}
		rules = append(rules, seededCareTeamRuleV165{
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

func TestV165Migration_HeaderRules_MatchGoLiteral_NoDrift(t *testing.T) {
	seeded := parseSeededV165HeaderRules(t)

	want := []cdafhir.MappingRule{
		cdafhir.AuthorMappingRules()[0],
		cdafhir.LegalAuthenticatorMappingRules()[0],
	}

	if len(seeded) != len(want) {
		t.Fatalf("got %d seeded header rules, want %d (Author + LegalAuthenticator)", len(seeded), len(want))
	}

	for i := range want {
		got := seeded[i]
		w := want[i]
		if got.headerPath != w.HeaderPath {
			t.Fatalf("rule[%d]: header_path = %q, want %q (migration row order must match Go literal order exactly)", i, got.headerPath, w.HeaderPath)
		}
		if got.sectionKey != w.SectionKey {
			t.Errorf("rule[%d] (%s): section_key = %q, want %q", i, got.headerPath, got.sectionKey, w.SectionKey)
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
		if got.skipEmptyCheck != w.SkipEmptyCheck {
			t.Errorf("rule[%d] (%s): skip_empty_check = %v, want %v", i, got.headerPath, got.skipEmptyCheck, w.SkipEmptyCheck)
		}
		if !reflect.DeepEqual(got.requiredPaths, w.RequiredPaths) {
			t.Errorf("rule[%d] (%s): required_paths = %v, want %v", i, got.headerPath, got.requiredPaths, w.RequiredPaths)
		}
		if !reflect.DeepEqual(got.fields, w.Fields) {
			t.Errorf("rule[%d] (%s): seeded fields drifted from declarative_oob_rules.go's Go literal.\nseeded: %+v\nwant:   %+v", i, got.headerPath, got.fields, w.Fields)
		}
	}
}

func TestV165Migration_CareTeamRules_MatchGoLiteral_NoDrift(t *testing.T) {
	seeded := parseSeededV165CareTeamRules(t)
	want := cdafhir.CareTeamMappingRules()

	if len(seeded) != len(want) {
		t.Fatalf("got %d seeded CareTeam rules, want %d (one per Care Team section-key alias)", len(seeded), len(want))
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
