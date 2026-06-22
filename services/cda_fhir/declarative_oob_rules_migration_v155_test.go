// services/cda_fhir/declarative_oob_rules_migration_v155_test.go
//
// Drift guard for V155 (Vital Signs/Results/Social History), mirroring
// declarative_oob_rules_migration_test.go's V154 guard exactly -- kept as a
// SEPARATE test/regex rather than generalizing the V154 one, since V155's
// INSERT column list is genuinely different (adds flatten_organizers before
// is_system/is_public) and the two migrations' drift guards are independent,
// single-purpose checks, not a shared parsing framework.
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

type seededObservationRule struct {
	sectionKey        string
	fhirResource      string
	entryMatch        string
	fields            []cdafhir.MappingRow
	flattenOrganizers bool
}

var migrationV155InsertPattern = regexp.MustCompile(
	`(?s)VALUES\s*\(\s*'[^']*',\s*'[^']*',\s*'[^']*',\s*'([^']*)',\s*'([^']*)',\s*'([^']*)',\s*\d+,\s*\$rules\$(.*?)\$rules\$::jsonb,\s*(true|false)`,
)

func parseSeededV155Rules(t testing.TB) []seededObservationRule {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "database", "migrations", "V155__CDA_Declarative_Mapping_Rules_Observations.sql")

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

func findSeededV155(t testing.TB, seeded []seededObservationRule, sectionKey, fhirResource string) seededObservationRule {
	t.Helper()
	for _, s := range seeded {
		if s.sectionKey == sectionKey && s.fhirResource == fhirResource {
			return s
		}
	}
	t.Fatalf("no seeded rule found in V155 for sectionKey=%q fhirResource=%q", sectionKey, fhirResource)
	return seededObservationRule{}
}

func assertObservationRuleMatchesSeed(t *testing.T, seeded []seededObservationRule, want cdafhir.MappingRule) {
	t.Helper()
	got := findSeededV155(t, seeded, want.SectionKey, want.FHIRResource)
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

func TestV155Migration_MatchesGoLiteralRules_NoDrift(t *testing.T) {
	seeded := parseSeededV155Rules(t)
	if len(seeded) != 3 {
		t.Fatalf("got %d seeded rules, want 3 (vitalSigns + results + socialHistory)", len(seeded))
	}

	for _, rule := range cdafhir.VitalSignsMappingRules() {
		assertObservationRuleMatchesSeed(t, seeded, rule)
	}
	for _, rule := range cdafhir.ResultsMappingRules() {
		assertObservationRuleMatchesSeed(t, seeded, rule)
	}
	for _, rule := range cdafhir.SocialHistoryMappingRules() {
		assertObservationRuleMatchesSeed(t, seeded, rule)
	}
}
