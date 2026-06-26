// services/cda_fhir/declarative_oob_rules_migration_v163_test.go
//
// Drift guard for V163 (labResults / functionalStatus / mentalStatus, the 3
// section-coverage gaps closed in Phase 4 Slice D). Reuses
// migrationV155InsertPattern -- same INSERT...VALUES(...) column shape
// (section_key, fhir_resource, entry_match, rule_order, fields,
// flatten_organizers, then skip_if_code_null_flavor/is_system/is_public,
// none of which the regex needs to capture).
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

func parseSeededV163Rules(t testing.TB) []seededObservationRule {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "database", "migrations", "V163__CDA_Declarative_Mapping_Rules_FunctionalMentalStatus_LabResults.sql")

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

func findSeededV163(t testing.TB, seeded []seededObservationRule, sectionKey, fhirResource string) seededObservationRule {
	t.Helper()
	for _, s := range seeded {
		if s.sectionKey == sectionKey && s.fhirResource == fhirResource {
			return s
		}
	}
	t.Fatalf("no seeded rule found in V163 for sectionKey=%q fhirResource=%q", sectionKey, fhirResource)
	return seededObservationRule{}
}

func assertV163RuleMatchesSeed(t *testing.T, seeded []seededObservationRule, want cdafhir.MappingRule) {
	t.Helper()
	got := findSeededV163(t, seeded, want.SectionKey, want.FHIRResource)
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

func TestV163Migration_MatchesGoLiteralRules_NoDrift(t *testing.T) {
	seeded := parseSeededV163Rules(t)
	if len(seeded) != 3 {
		t.Fatalf("got %d seeded rules, want 3 (labResults + functionalStatus + mentalStatus)", len(seeded))
	}

	// ResultsMappingRules()[1] is "labResults" -- see that function's own doc
	// comment for why it's index 1, not 0.
	assertV163RuleMatchesSeed(t, seeded, cdafhir.ResultsMappingRules()[1])
	for _, rule := range cdafhir.FunctionalStatusMappingRules() {
		assertV163RuleMatchesSeed(t, seeded, rule)
	}
	for _, rule := range cdafhir.MentalStatusMappingRules() {
		assertV163RuleMatchesSeed(t, seeded, rule)
	}
}
