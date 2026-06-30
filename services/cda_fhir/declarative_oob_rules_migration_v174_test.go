// services/cda_fhir/declarative_oob_rules_migration_v174_test.go
//
// Drift guard for V174 (two IG-driven fixes: MedicationRequest.requester
// priority reversed to author-first per HL7's C-CDA on FHIR IG, and a new
// MedicationRequest.note/MedicationStatement.note row added for Comment
// Activity). Supersedes V170 (medications, both resources), V172
// (medications/MedicationRequest), V171 and V173 (the 4 Plan-of-Care
// aliases) entirely for Fields content -- see those files' own narrowed
// drift tests.
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

func parseSeededV174Rules(t testing.TB) []seededRule {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "database", "migrations", "V174__CDA_Declarative_Mapping_Rules_Medication_Requester_Note.sql")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading migration file: %v", err)
	}

	matches := migrationV155InsertPattern.FindAllStringSubmatch(string(data), -1)
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

func findSeededV174(t testing.TB, seeded []seededRule, sectionKey, fhirResource string) seededRule {
	t.Helper()
	for _, s := range seeded {
		if s.sectionKey == sectionKey && s.fhirResource == fhirResource {
			return s
		}
	}
	t.Fatalf("no seeded rule found in V174 for sectionKey=%q fhirResource=%q", sectionKey, fhirResource)
	return seededRule{}
}

func TestV174Migration_MatchesGoLiteralRules_NoDrift(t *testing.T) {
	seeded := parseSeededV174Rules(t)
	if len(seeded) != 6 {
		t.Fatalf("got %d seeded rules, want 6 (medications x2 + 4 Plan-of-Care aliases, all MedicationRequest except medications/MedicationStatement)", len(seeded))
	}

	var medRequestWant, medStatementWant cdafhir.MappingRule
	for _, rule := range cdafhir.MedicationMappingRules() {
		switch rule.FHIRResource {
		case "MedicationRequest":
			medRequestWant = rule
		case "MedicationStatement":
			medStatementWant = rule
		}
	}

	for _, tc := range []struct {
		sectionKey, fhirResource, entryMatch string
		want                                 cdafhir.MappingRule
	}{
		{"medications", "MedicationRequest", "moodCode=INT", medRequestWant},
		{"medications", "MedicationStatement", "", medStatementWant},
		{"carePlan", "MedicationRequest", "entryType=substanceAdministration", medRequestWant},
		{"planOfCare", "MedicationRequest", "entryType=substanceAdministration", medRequestWant},
		{"assessmentAndPlan", "MedicationRequest", "entryType=substanceAdministration", medRequestWant},
		{"planOfTreatment", "MedicationRequest", "entryType=substanceAdministration", medRequestWant},
	} {
		got := findSeededV174(t, seeded, tc.sectionKey, tc.fhirResource)
		if got.entryMatch != tc.entryMatch {
			t.Errorf("%s/%s: entry_match = %q, want %q", tc.sectionKey, tc.fhirResource, got.entryMatch, tc.entryMatch)
		}
		if !reflect.DeepEqual(got.fields, tc.want.Fields) {
			t.Errorf("%s/%s: seeded fields drifted from declarative_oob_rules.go's Go literal.\nseeded: %+v\nwant:   %+v",
				tc.sectionKey, tc.fhirResource, got.fields, tc.want.Fields)
		}
	}
}
