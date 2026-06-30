// services/cda_fhir/declarative_oob_rules_migration_v178_test.go
//
// Drift guard for V178 (Observation.performer upgraded from a bare display
// string to the PractitionerRole/Practitioner tiered pattern, shared by all
// 6 observationRule() callers -- vitalSigns, results, labResults,
// functionalStatus, mentalStatus, socialHistory). socialHistory also
// upgrades the nested Assessment Scale Supporting Observation's own
// performer row (sdohAssessmentScaleSupportingObservationRow()). Supersedes
// V169 for all 6 sections' Fields content, and supersedes V177 for
// socialHistory's Fields content -- see those files' own updated doc
// comments; V169/V177 still own entry_match/flatten_organizers for their
// respective sections (unchanged by this migration).
//
// functionalStatus/Observation's Fields content was THEN superseded by V179
// (Assessment Scale Observation COMP-children fan-out -- see
// declarative_oob_rules_migration_v179_test.go), the same way V178 itself
// superseded V177 for socialHistory above. mentalStatus/Observation's Fields
// content was THEN superseded the same way by V180 (see
// declarative_oob_rules_migration_v180_test.go). This file now only confirms
// functionalStatus and mentalStatus still exist in V178's own migration
// text, and no longer asserts either's Fields.
//
// ALL SIX sections' Fields content was THEN superseded again by V184 (new
// referenceRange[0].text row -- see declarative_oob_rules_migration_v184_test.go).
// This file now only confirms existence for all 6, no longer asserting any
// of their Fields.
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

func parseSeededV178Rules(t testing.TB) []seededObservationRule {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "database", "migrations", "V178__CDA_Declarative_Mapping_Rules_Observation_Performer_PractitionerRole.sql")

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

func findSeededV178(t testing.TB, seeded []seededObservationRule, sectionKey, fhirResource string) seededObservationRule {
	t.Helper()
	for _, s := range seeded {
		if s.sectionKey == sectionKey && s.fhirResource == fhirResource {
			return s
		}
	}
	t.Fatalf("no seeded rule found in V178 for sectionKey=%q fhirResource=%q", sectionKey, fhirResource)
	return seededObservationRule{}
}

func assertV178RuleMatchesSeed(t *testing.T, seeded []seededObservationRule, want cdafhir.MappingRule) {
	t.Helper()
	got := findSeededV178(t, seeded, want.SectionKey, want.FHIRResource)
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

func TestV178Migration_MatchesGoLiteralRules_NoDrift(t *testing.T) {
	seeded := parseSeededV178Rules(t)
	if len(seeded) != 6 {
		t.Fatalf("got %d seeded rules, want 6 (vitalSigns + results + labResults + functionalStatus + mentalStatus + socialHistory)", len(seeded))
	}

	// All 6 sections' Fields content is now superseded by V184 -- existence
	// only, same downgrade V169's test applied to socialHistory once V177
	// superseded it (see this file's own top doc comment).
	findSeededV178(t, seeded, "vitalSigns", "Observation")
	findSeededV178(t, seeded, "results", "Observation")
	findSeededV178(t, seeded, "labResults", "Observation")
	findSeededV178(t, seeded, "functionalStatus", "Observation")
	findSeededV178(t, seeded, "mentalStatus", "Observation")
	findSeededV178(t, seeded, "socialHistory", "Observation")
}
