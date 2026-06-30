// services/cda_fhir/declarative_oob_rules_migration_v186_test.go
//
// Drift guard for V186 (healthConcerns/Observation rule for Assessment Scale
// Observations -- PHQ-9, PHQ-2, AUDIT-C -- nested via REFR inside Health
// Concern Acts). The existing healthConcerns/Condition row (V175) is unchanged.
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

func TestV186Migration_MatchesGoLiteralRules_NoDrift(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "database", "migrations", "V186__CDA_Declarative_Mapping_Rules_HealthConcern_AssessmentScale.sql")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading migration file: %v", err)
	}

	matches := migrationInsertPattern.FindAllStringSubmatch(string(data), -1)
	if len(matches) != 1 {
		t.Fatalf("expected 1 INSERT block in V186, got %d", len(matches))
	}

	m := matches[0]
	var gotFields []cdafhir.MappingRow
	if err := json.Unmarshal([]byte(m[4]), &gotFields); err != nil {
		t.Fatalf("unmarshalling fields JSON: %v\nJSON: %s", err, m[4])
	}
	got := seededRule{
		sectionKey:   m[1],
		fhirResource: m[2],
		entryMatch:   m[3],
		fields:       gotFields,
	}

	want := cdafhir.HealthConcernsMappingRules()[0] // assessment scale Observation rule

	if got.sectionKey != want.SectionKey {
		t.Errorf("section_key = %q, want %q", got.sectionKey, want.SectionKey)
	}
	if got.fhirResource != want.FHIRResource {
		t.Errorf("fhir_resource = %q, want %q", got.fhirResource, want.FHIRResource)
	}
	if got.entryMatch != want.EntryMatch {
		t.Errorf("entry_match = %q, want %q", got.entryMatch, want.EntryMatch)
	}
	if !reflect.DeepEqual(got.fields, want.Fields) {
		t.Errorf("seeded fields drifted from Go literal.\nseeded: %+v\nwant:   %+v", got.fields, want.Fields)
	}
}
