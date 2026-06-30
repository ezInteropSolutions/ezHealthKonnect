// services/cda_fhir/declarative_oob_rules_migration_v157_test.go
//
// Drift guard for V157 (Encounters / Procedures). Reuses
// migrationV155InsertPattern -- two separate INSERT...VALUES(...) statements
// (one per section), each with its own literal "VALUES" token, same shape
// the regex already expects (see V155's own migration file, also written as
// independent statements rather than one INSERT with comma-separated
// tuples -- a multi-tuple single VALUES clause would only have "VALUES"
// before the FIRST tuple, which this regex can't find a second match
// after).
//
// Encounters' own seed was retargeted to V166 (see that migration's doc
// comment and declarative_oob_rules_migration_v166_test.go): V157's
// "encounters" row never gained assignedEntityRoleRow's PractitionerRole on
// performers[*], the same gap CareTeam/Author/LegalAuthenticator already
// closed elsewhere. This test now only checks "procedures" against V157,
// which V166 doesn't touch and remains the authoritative seed for.
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

func parseSeededV157Rules(t testing.TB) []seededObservationRule {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")
	path := filepath.Join(repoRoot, "database", "migrations", "V157__CDA_Declarative_Mapping_Rules_Encounters_Procedures.sql")

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

func TestV157Migration_MatchesGoLiteralRules_NoDrift(t *testing.T) {
	seeded := parseSeededV157Rules(t)
	if len(seeded) != 2 {
		t.Fatalf("got %d seeded rules, want 2 (encounters, procedures)", len(seeded))
	}

	// "encounters" deliberately excluded here -- see this file's own top
	// doc comment: that row's drift check moved to V166.
	wantRules := map[string]cdafhir.MappingRule{
		"procedures": cdafhir.ProcedureMappingRules()[0],
	}

	for _, got := range seeded {
		want, ok := wantRules[got.sectionKey]
		if !ok {
			if got.sectionKey == "encounters" {
				continue
			}
			t.Errorf("seeded an unexpected sectionKey %q", got.sectionKey)
			continue
		}
		if got.fhirResource != want.FHIRResource {
			t.Errorf("%s: fhirResource = %q, want %q", got.sectionKey, got.fhirResource, want.FHIRResource)
		}
		if got.entryMatch != want.EntryMatch {
			t.Errorf("%s: entry_match = %q, want %q", got.sectionKey, got.entryMatch, want.EntryMatch)
		}
		if got.flattenOrganizers != want.FlattenOrganizers {
			t.Errorf("%s: flatten_organizers = %v, want %v", got.sectionKey, got.flattenOrganizers, want.FlattenOrganizers)
		}
		if !reflect.DeepEqual(got.fields, want.Fields) {
			t.Errorf("%s: seeded fields drifted from declarative_oob_rules.go's Go literal.\nseeded: %+v\nwant:   %+v", got.sectionKey, got.fields, want.Fields)
		}
	}
}
