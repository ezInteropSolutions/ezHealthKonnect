// services/cda_fhir/declarative_oob_rules_migration_v187_test.go
//
// Drift guard for V187 (Coverage IG alignment -- 5 field sources corrected to
// read from the COMP-nested Policy Activity instead of the outer Coverage
// Activity: type, period, payor, relationship, subscriberId). Both section
// key aliases (payersInsurance + payors) are re-seeded with identical fields.
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

func TestV187Migration_MatchesGoLiteralRules_NoDrift(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "database", "migrations", "V187__CDA_Declarative_Mapping_Rules_Coverage_IG_Alignment.sql")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading migration file: %v", err)
	}

	matches := migrationInsertPattern.FindAllStringSubmatch(string(data), -1)
	if len(matches) != 2 {
		t.Fatalf("expected 2 INSERT blocks in V187 (payersInsurance + payors), got %d", len(matches))
	}

	wantFields := cdafhir.CoverageMappingRules()[0].Fields

	for _, m := range matches {
		sectionKey := m[1]
		fhirResource := m[2]

		if fhirResource != "Coverage" {
			t.Errorf("section %s: fhir_resource = %q, want Coverage", sectionKey, fhirResource)
			continue
		}

		var gotFields []cdafhir.MappingRow
		if err := json.Unmarshal([]byte(m[4]), &gotFields); err != nil {
			t.Fatalf("section %s: unmarshalling fields JSON: %v\nJSON: %s", sectionKey, err, m[4])
		}

		if !reflect.DeepEqual(gotFields, wantFields) {
			t.Errorf("section %s: seeded fields drifted from Go literal.\nseeded: %+v\nwant:   %+v",
				sectionKey, gotFields, wantFields)
		}
	}
}
