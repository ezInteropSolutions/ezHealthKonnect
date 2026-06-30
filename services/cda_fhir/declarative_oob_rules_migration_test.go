// services/cda_fhir/declarative_oob_rules_migration_test.go
//
// Drift guard for Phase 3's V154 migration: declarative_oob_rules.go is the
// single source of truth for the OOB rule content (see that file's top doc
// comment); the migration's dollar-quoted JSON is hand-synced to match it.
// This test reads database/migrations/V154__CDA_Declarative_Mapping_Rules.sql
// straight off disk, extracts each INSERT's (section_key, fhir_resource,
// entry_match, fields JSON), unmarshals fields into []MappingRow, and
// deep-compares the result against the corresponding Go literal function's
// output. Any future hand-edit to one side without the other fails here
// instead of shipping silently — the same kind of regression floor Phase 1/2
// established with benchmark/test parity, applied to a migration seed
// instead of code.
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

type seededRule struct {
	sectionKey   string
	fhirResource string
	entryMatch   string
	fields       []cdafhir.MappingRow
}

var migrationInsertPattern = regexp.MustCompile(
	`(?s)VALUES\s*\(\s*'[^']*',\s*'[^']*',\s*'[^']*',\s*'([^']*)',\s*'([^']*)',\s*'([^']*)',\s*\d+,\s*\$rules\$(.*?)\$rules\$::jsonb`,
)

func parseSeededRules(t testing.TB) []seededRule {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "database", "migrations", "V154__CDA_Declarative_Mapping_Rules.sql")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading migration file: %v", err)
	}

	matches := migrationInsertPattern.FindAllStringSubmatch(string(data), -1)
	if len(matches) == 0 {
		t.Fatal("found zero INSERT...VALUES(...$rules$...) blocks in the migration -- regex or file content drifted")
	}

	rules := make([]seededRule, 0, len(matches))
	for _, m := range matches {
		var fields []cdafhir.MappingRow
		if err := json.Unmarshal([]byte(m[4]), &fields); err != nil {
			t.Fatalf("unmarshalling fields JSON for %s/%s: %v\nJSON: %s", m[1], m[2], err, m[4])
		}
		rules = append(rules, seededRule{
			sectionKey:   m[1],
			fhirResource: m[2],
			entryMatch:   m[3],
			fields:       fields,
		})
	}
	return rules
}

func findSeeded(t testing.TB, seeded []seededRule, sectionKey, fhirResource string) seededRule {
	t.Helper()
	for _, s := range seeded {
		if s.sectionKey == sectionKey && s.fhirResource == fhirResource {
			return s
		}
	}
	t.Fatalf("no seeded rule found in V154 for sectionKey=%q fhirResource=%q", sectionKey, fhirResource)
	return seededRule{}
}

func assertRuleMatchesSeed(t *testing.T, seeded []seededRule, want cdafhir.MappingRule) {
	t.Helper()
	got := findSeeded(t, seeded, want.SectionKey, want.FHIRResource)
	if got.entryMatch != want.EntryMatch {
		t.Errorf("%s/%s: entry_match = %q, want %q", want.SectionKey, want.FHIRResource, got.entryMatch, want.EntryMatch)
	}
	if !reflect.DeepEqual(got.fields, want.Fields) {
		t.Errorf("%s/%s: seeded fields drifted from declarative_oob_rules.go's Go literal.\nseeded: %+v\nwant:   %+v",
			want.SectionKey, want.FHIRResource, got.fields, want.Fields)
	}
}

// TestV154Migration_MatchesGoLiteralRules_NoDrift only checks the row V154
// still owns (allergy). V154's own "problems"/"healthConcerns" rows were
// superseded by V168 (Condition.recorder added to conditionFields() -- see
// declarative_oob_rules_migration_v168_test.go) and its "medications" rows
// were superseded by V170 (identifier added to medicationCommonRows() --
// see declarative_oob_rules_migration_v170_test.go), so this file's on-disk
// content for those sections is now stale BY DESIGN and is deliberately not
// asserted against the Go literal here.
func TestV154Migration_MatchesGoLiteralRules_NoDrift(t *testing.T) {
	seeded := parseSeededRules(t)
	if len(seeded) != 5 {
		t.Fatalf("got %d seeded rules, want 5 (1 allergy + 2 medication + 2 condition)", len(seeded))
	}

	for _, rule := range cdafhir.AllergyMappingRules() {
		assertRuleMatchesSeed(t, seeded, rule)
	}
}
