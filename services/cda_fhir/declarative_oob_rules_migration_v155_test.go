// services/cda_fhir/declarative_oob_rules_migration_v155_test.go
//
// V155's own drift-CHECKING test (vitalSigns/results/socialHistory) was
// superseded by V169 (code.text-from-narrative row added to
// observationRule(), the one Go helper feeding all of V155's and V163's
// sections at once -- see declarative_oob_rules_migration_v169_test.go) and
// removed from this file. migrationV155InsertPattern/seededObservationRule/
// parseSeededV155Rules are KEPT here -- V156/V157/V158/V160/V163's own
// drift tests still import these shared package-level symbols for their own
// migrations, unrelated to V155's now-superseded content.
package cdafhir_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"testing"

	cdafhir "ezhealthkonnect/services/cda_fhir"
)

type seededObservationRule struct {
	sectionKey        string
	fhirResource      string
	entryMatch        string
	fields            []cdafhir.MappingRow
	flattenOrganizers bool
}

var migrationV155InsertPattern = regexp.MustCompile(
	`(?s)VALUES\s*\(\s*'[^']*',\s*'[^']*',\s*'[^']*',\s*'([^']*)',\s*'([^']*)',\s*'([^']*)',\s*\d+,\s*\$rules\$(.*?)\$rules\$::jsonb,\s*(true|false)`,
)

func parseSeededV155Rules(t testing.TB) []seededObservationRule {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "database", "migrations", "V155__CDA_Declarative_Mapping_Rules_Observations.sql")

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

