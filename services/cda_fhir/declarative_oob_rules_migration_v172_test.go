// services/cda_fhir/declarative_oob_rules_migration_v172_test.go
//
// Drift guard for V172 (requesterFromPerformer/requesterFromAuthor --
// PractitionerRole-emitting requester tiers -- added to
// medicationRequestFields() ahead of the pre-existing display-only
// fallback). Supersedes V170's MedicationRequest coverage.
//
// V172 itself was then entirely superseded by V174 (requester priority
// reversed to author-first per the IG, plus a new note row -- see
// declarative_oob_rules_migration_v174_test.go); only structural facts are
// still checked here.
package cdafhir_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	cdafhir "ezhealthkonnect/services/cda_fhir"
)

func parseSeededV172Rules(t testing.TB) []seededRule {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "database", "migrations", "V172__CDA_Declarative_Mapping_Rules_Medication_Requester.sql")

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

func TestV172Migration_MatchesGoLiteralRules_NoDrift(t *testing.T) {
	seeded := parseSeededV172Rules(t)
	if len(seeded) != 1 {
		t.Fatalf("got %d seeded rules, want 1 (medications/MedicationRequest)", len(seeded))
	}

	var want cdafhir.MappingRule
	for _, rule := range cdafhir.MedicationMappingRules() {
		if rule.FHIRResource == "MedicationRequest" {
			want = rule
		}
	}

	got := seeded[0]
	if got.sectionKey != want.SectionKey || got.fhirResource != want.FHIRResource {
		t.Fatalf("seeded rule = %s/%s, want %s/%s", got.sectionKey, got.fhirResource, want.SectionKey, want.FHIRResource)
	}
	// entry_match/Fields comparison deliberately omitted -- superseded by
	// V174, see this file's own top doc comment.
}
