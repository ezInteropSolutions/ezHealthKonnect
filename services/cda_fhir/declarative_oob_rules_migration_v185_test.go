// services/cda_fhir/declarative_oob_rules_migration_v185_test.go
//
// Drift guard for V185 (Observation.identifier row added to all 6
// observationRule() callers). Supersedes V184 for all 6 sections' Fields
// content -- V178 still owns entry_match/flatten_organizers.
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

func parseSeededV185Rules(t testing.TB) []seededObservationRule {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "database", "migrations", "V185__CDA_Declarative_Mapping_Rules_Observation_Identifier.sql")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading migration file: %v", err)
	}
	matches := migrationV155InsertPattern.FindAllStringSubmatch(string(data), -1)
	if len(matches) == 0 {
		t.Fatal("found zero INSERT...VALUES(...$rules$...) blocks in the migration")
	}
	rules := make([]seededObservationRule, 0, len(matches))
	for _, m := range matches {
		var fields []cdafhir.MappingRow
		if err := json.Unmarshal([]byte(m[4]), &fields); err != nil {
			t.Fatalf("unmarshalling fields JSON for %s/%s: %v", m[1], m[2], err)
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

func findSeededV185(t testing.TB, seeded []seededObservationRule, sectionKey, fhirResource string) seededObservationRule {
	t.Helper()
	for _, s := range seeded {
		if s.sectionKey == sectionKey && s.fhirResource == fhirResource {
			return s
		}
	}
	t.Fatalf("no seeded rule found in V185 for sectionKey=%q fhirResource=%q", sectionKey, fhirResource)
	return seededObservationRule{}
}

func assertV185RuleMatchesSeed(t *testing.T, seeded []seededObservationRule, want cdafhir.MappingRule) {
	t.Helper()
	got := findSeededV185(t, seeded, want.SectionKey, want.FHIRResource)
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

func TestV185Migration_MatchesGoLiteralRules_NoDrift(t *testing.T) {
	seeded := parseSeededV185Rules(t)
	if len(seeded) != 6 {
		t.Fatalf("got %d seeded rules, want 6", len(seeded))
	}
	for _, rule := range cdafhir.VitalSignsMappingRules() {
		assertV185RuleMatchesSeed(t, seeded, rule)
	}
	assertV185RuleMatchesSeed(t, seeded, cdafhir.ResultsMappingRules()[0])
	assertV185RuleMatchesSeed(t, seeded, cdafhir.ResultsMappingRules()[1])
	assertV185RuleMatchesSeed(t, seeded, cdafhir.SocialHistoryMappingRules()[0])
	assertV185RuleMatchesSeed(t, seeded, cdafhir.FunctionalStatusMappingRules()[0])
	assertV185RuleMatchesSeed(t, seeded, cdafhir.MentalStatusMappingRules()[0])
}
