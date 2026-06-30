// services/cda_fhir/declarative_oob_rules_migration_v170_test.go
//
// Drift guard for V170 (identifier row added to medicationCommonRows(),
// which feeds both MedicationRequest and MedicationStatement). Supersedes
// V154's "medications" coverage (removed from
// declarative_oob_rules_migration_test.go).
//
// V170's MedicationRequest row was superseded by V172, then both that and
// V170's MedicationStatement row were superseded again by V174 (note row
// added to medicationCommonRows(), feeding both resources -- see
// declarative_oob_rules_migration_v174_test.go). Nothing in this file's
// Fields content is current any more; only structural facts (row count,
// entry_match) are still checked.
package cdafhir_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	cdafhir "ezhealthkonnect/services/cda_fhir"
)

func parseSeededV170Rules(t testing.TB) []seededRule {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "database", "migrations", "V170__CDA_Declarative_Mapping_Rules_Medication_Identifier.sql")

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

func findSeededV170(t testing.TB, seeded []seededRule, sectionKey, fhirResource string) seededRule {
	t.Helper()
	for _, s := range seeded {
		if s.sectionKey == sectionKey && s.fhirResource == fhirResource {
			return s
		}
	}
	t.Fatalf("no seeded rule found in V170 for sectionKey=%q fhirResource=%q", sectionKey, fhirResource)
	return seededRule{}
}

func assertV170RuleMatchesSeed(t *testing.T, seeded []seededRule, want cdafhir.MappingRule) {
	t.Helper()
	got := findSeededV170(t, seeded, want.SectionKey, want.FHIRResource)
	if got.entryMatch != want.EntryMatch {
		t.Errorf("%s/%s: entry_match = %q, want %q", want.SectionKey, want.FHIRResource, got.entryMatch, want.EntryMatch)
	}
	// Fields comparison deliberately omitted -- superseded by V174, see this
	// file's own top doc comment.
}

func TestV170Migration_MatchesGoLiteralRules_NoDrift(t *testing.T) {
	seeded := parseSeededV170Rules(t)
	if len(seeded) != 2 {
		t.Fatalf("got %d seeded rules, want 2 (MedicationRequest + MedicationStatement)", len(seeded))
	}

	for _, rule := range cdafhir.MedicationMappingRules() {
		assertV170RuleMatchesSeed(t, seeded, rule)
	}
}
