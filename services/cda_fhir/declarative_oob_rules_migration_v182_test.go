// services/cda_fhir/declarative_oob_rules_migration_v182_test.go
//
// Drift guard for V182 (entry.text -> code.text fallback row added to
// PlanOfCareMappingRules()'s ServiceRequest dispatch branch, across all 4
// section-key aliases -- see PlanOfCareMappingRules' own ServiceRequest row
// doc comment in declarative_oob_rules.go for the real "Marshfield
// Clinic"/"Dignity Health" gap this closes: a Planned Act .4.39 entry's
// bare nullFlavor code, with the real content only in the entry's own
// sibling <text> reference, fell through to a generic "Unknown" placeholder).
// Supersedes V158 for these 4 ServiceRequest rows' Fields content ONLY --
// V158 still owns entry_match/flatten_organizers for them (unchanged by
// this migration), and remains the owner of every other Plan-of-Care rule
// across all 4 aliases (the no-op EVN rule, Goal, Appointment,
// SupplyRequest; MedicationRequest was already superseded by V171).
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

func parseSeededV182Rules(t testing.TB) []seededObservationRule {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "database", "migrations", "V182__CDA_Declarative_Mapping_Rules_PlanOfCare_ServiceRequest_TextFallback.sql")

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

func findSeededV182(t testing.TB, seeded []seededObservationRule, sectionKey, fhirResource string) seededObservationRule {
	t.Helper()
	for _, s := range seeded {
		if s.sectionKey == sectionKey && s.fhirResource == fhirResource {
			return s
		}
	}
	t.Fatalf("no seeded rule found in V182 for sectionKey=%q fhirResource=%q", sectionKey, fhirResource)
	return seededObservationRule{}
}

func TestV182Migration_MatchesGoLiteralRules_NoDrift(t *testing.T) {
	seeded := parseSeededV182Rules(t)
	if len(seeded) != 4 {
		t.Fatalf("got %d seeded rules, want 4 (carePlan + planOfCare + assessmentAndPlan + planOfTreatment, all ServiceRequest)", len(seeded))
	}

	for _, rule := range cdafhir.PlanOfCareMappingRules() {
		if rule.FHIRResource != "ServiceRequest" {
			continue
		}
		got := findSeededV182(t, seeded, rule.SectionKey, rule.FHIRResource)
		if got.entryMatch != rule.EntryMatch {
			t.Errorf("%s/%s: entry_match = %q, want %q", rule.SectionKey, rule.FHIRResource, got.entryMatch, rule.EntryMatch)
		}
		if got.flattenOrganizers != rule.FlattenOrganizers {
			t.Errorf("%s/%s: flatten_organizers = %v, want %v", rule.SectionKey, rule.FHIRResource, got.flattenOrganizers, rule.FlattenOrganizers)
		}
		if !reflect.DeepEqual(got.fields, rule.Fields) {
			t.Errorf("%s/%s: seeded fields drifted from declarative_oob_rules.go's Go literal.\nseeded: %+v\nwant:   %+v",
				rule.SectionKey, rule.FHIRResource, got.fields, rule.Fields)
		}
	}
}
