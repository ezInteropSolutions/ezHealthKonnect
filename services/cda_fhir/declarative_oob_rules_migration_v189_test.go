// services/cda_fhir/declarative_oob_rules_migration_v189_test.go
//
// Drift guard for V189 (CareTeam.managingOrganization from organizer author's
// representedOrganization, aligned with C-CDA on FHIR IG). Real corpus
// evidence: Epic 99397 CCD's care team organizer has
// <author><assignedAuthor><representedOrganization> "Boulder Community Health
// and Affiliates" — previously unmapped.
//
// V189 supersedes V165 for the careTeam / careTeams rows only. V165's
// Author/LegalAuthenticator header rows remain live and unmodified.
//
// Drift guard pattern: read V189 SQL, parse each INSERT's fields JSON,
// deep-compare against CareTeamMappingRules().
package cdafhir_test

import (
	"encoding/json"
	"reflect"
	"testing"

	cdafhir "ezhealthkonnect/services/cda_fhir"
)

func parseSeededV189Rules(t testing.TB) []seededCareTeamRuleV165 {
	t.Helper()
	data := readMigrationFile(t, "V189__CDA_Declarative_Mapping_Rules_CareTeam_ManagingOrganization.sql")

	matches := migrationV165CareTeamInsertPattern.FindAllStringSubmatch(data, -1)
	if len(matches) == 0 {
		t.Fatal("found zero CareTeam-shaped INSERT...VALUES(...$rules$...) blocks in V189 -- regex or file content drifted")
	}

	rules := make([]seededCareTeamRuleV165, 0, len(matches))
	for _, m := range matches {
		var fields []cdafhir.MappingRow
		if err := json.Unmarshal([]byte(m[4]), &fields); err != nil {
			t.Fatalf("unmarshalling fields JSON for %s/%s: %v\nJSON: %s", m[1], m[2], err, m[4])
		}
		rules = append(rules, seededCareTeamRuleV165{
			sectionKey:        m[1],
			fhirResource:      m[2],
			entryMatch:        m[3],
			fields:            fields,
			flattenOrganizers: m[5] == "true",
			requiredPaths:     parseSQLStringArray(m[6]),
		})
	}
	return rules
}

// TestV189Migration_CareTeamRules_MatchGoLiteral_NoDrift verifies that V189's
// careTeam and careTeams INSERTs match CareTeamMappingRules() exactly.
// Supersedes V165's TestV165Migration_CareTeamRules_MatchGoLiteral_NoDrift.
func TestV189Migration_CareTeamRules_MatchGoLiteral_NoDrift(t *testing.T) {
	seeded := parseSeededV189Rules(t)
	want := cdafhir.CareTeamMappingRules()

	if len(seeded) != len(want) {
		t.Fatalf("got %d seeded CareTeam rules, want %d (one per Care Team section-key alias)", len(seeded), len(want))
	}

	for i := range want {
		got := seeded[i]
		w := want[i]
		if got.sectionKey != w.SectionKey {
			t.Fatalf("rule[%d]: sectionKey = %q, want %q (migration row order must match CareTeamMappingRules' alias order exactly)", i, got.sectionKey, w.SectionKey)
		}
		if got.fhirResource != w.FHIRResource {
			t.Errorf("rule[%d] (%s): fhirResource = %q, want %q", i, got.sectionKey, got.fhirResource, w.FHIRResource)
		}
		if got.entryMatch != w.EntryMatch {
			t.Errorf("rule[%d] (%s/%s): entry_match = %q, want %q", i, got.sectionKey, got.fhirResource, got.entryMatch, w.EntryMatch)
		}
		if got.flattenOrganizers != w.FlattenOrganizers {
			t.Errorf("rule[%d] (%s/%s): flatten_organizers = %v, want %v", i, got.sectionKey, got.fhirResource, got.flattenOrganizers, w.FlattenOrganizers)
		}
		if !reflect.DeepEqual(got.requiredPaths, w.RequiredPaths) {
			t.Errorf("rule[%d] (%s/%s): required_paths = %v, want %v", i, got.sectionKey, got.fhirResource, got.requiredPaths, w.RequiredPaths)
		}
		if !reflect.DeepEqual(got.fields, w.Fields) {
			t.Errorf("rule[%d] (%s/%s): seeded fields drifted from declarative_oob_rules.go's Go literal.\nseeded: %+v\nwant:   %+v",
				i, got.sectionKey, got.fhirResource, got.fields, w.Fields)
		}
	}
}
