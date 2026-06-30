// services/cda_fhir/declarative_oob_rules_migration_v177_test.go
//
// Drift guard for V177 (SDOH Assessment Scale fan-out added to
// SocialHistoryMappingRules() -- shell -> Assessment Scale Observation
// (SPRT) -> Assessment Scale Supporting Observation (COMP, CollectAll),
// each emitted as its own Observation linked via hasMember[]). Reuses
// migrationV155InsertPattern since V177's INSERT column list is identical
// to V169's (document_type..fields, flatten_organizers,
// skip_if_code_null_flavor, is_system, is_public). Supersedes V169 for
// socialHistory/Observation's Fields content.
//
// socialHistory/Observation's Fields content was THEN superseded again by
// V178 (Observation.performer upgraded to the PractitionerRole/Practitioner
// tiered pattern, including the nested Assessment Scale Supporting
// Observation's own performer row -- see
// declarative_oob_rules_migration_v178_test.go). This file now only
// confirms the row still exists in V177's own migration text, and no
// longer asserts Fields.
package cdafhir_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	cdafhir "ezhealthkonnect/services/cda_fhir"
)

func parseSeededV177Rules(t testing.TB) []seededObservationRule {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "database", "migrations", "V177__CDA_Declarative_Mapping_Rules_SocialHistory_SDOH_AssessmentScale.sql")

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

func TestV177Migration_MatchesGoLiteralRules_NoDrift(t *testing.T) {
	seeded := parseSeededV177Rules(t)
	if len(seeded) != 1 {
		t.Fatalf("got %d seeded rules, want 1 (socialHistory)", len(seeded))
	}

	want := cdafhir.SocialHistoryMappingRules()[0]
	got := seeded[0]
	if got.sectionKey != want.SectionKey || got.fhirResource != want.FHIRResource {
		t.Fatalf("seeded rule = %s/%s, want %s/%s", got.sectionKey, got.fhirResource, want.SectionKey, want.FHIRResource)
	}
	// Fields content is now superseded by V178 -- see this file's own top
	// doc comment. entry_match/flatten_organizers are unchanged by V178 but
	// are no longer asserted here either, since the live row is fully
	// owned by V178 going forward.
}
