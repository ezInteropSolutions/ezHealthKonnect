package cdafhir

import (
	"sort"
	"testing"
)

func TestFlattenSectionRules_Allergy_ProducesAll9RealFields(t *testing.T) {
	fields := FlattenSectionRules(AllergyMappingRules())

	got := make([]string, 0, len(fields))
	for _, f := range fields {
		got = append(got, f.Key)
	}
	sort.Strings(got)

	want := []string{
		"category",
		"clinicalStatus",
		"code",
		"criticality",
		"onsetDateTime",
		"reaction.manifestation[0]",
		"reaction.severity",
		"type",
		"verificationStatus",
	}
	sort.Strings(want)

	if len(got) != len(want) {
		t.Fatalf("FlattenSectionRules(AllergyMappingRules()) = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("FlattenSectionRules(AllergyMappingRules()) = %v, want %v", got, want)
		}
	}

	for _, key := range []string{"authorGiven", "authorFamily", "authorNPI", "reaction"} {
		for _, f := range fields {
			if f.Key == key {
				t.Errorf("FlattenSectionRules produced unexpected key %q (stale ccda_2_1.json field or bare CollectAll parent)", key)
			}
		}
	}
}

func TestFlattenSectionRules_NestedFieldsHaveCorrectNestedUnderAndSource(t *testing.T) {
	fields := FlattenSectionRules(AllergyMappingRules())

	var manifestation, severity *FlattenedField
	for i := range fields {
		switch fields[i].Key {
		case "reaction.manifestation[0]":
			manifestation = &fields[i]
		case "reaction.severity":
			severity = &fields[i]
		}
	}

	if manifestation == nil {
		t.Fatal("expected a reaction.manifestation[0] field")
	}
	if manifestation.NestedUnder != "reaction" {
		t.Errorf("manifestation.NestedUnder = %q, want %q", manifestation.NestedUnder, "reaction")
	}

	if severity == nil {
		t.Fatal("expected a reaction.severity field")
	}
	if severity.NestedUnder != "reaction" {
		t.Errorf("severity.NestedUnder = %q, want %q", severity.NestedUnder, "reaction")
	}
	if severity.CDASourceDisplay == "" {
		t.Error("severity.CDASourceDisplay is empty — expected parent Scope to be inherited")
	}
}

func TestCloneMappingRules_DoesNotAliasOriginal(t *testing.T) {
	clone := CloneMappingRules(AllergyMappingRules())
	mutateSeverityRow(clone)

	fresh := AllergyMappingRules()
	if got := severityTargetPath(fresh); got != "severity" {
		t.Errorf("AllergyMappingRules() (constructor) affected by clone mutation: severity TargetPath = %q", got)
	}

	cacheClone := CloneMappingRules(DeclarativeSectionRules("allergiesAndIntolerances"))
	mutateSeverityRow(cacheClone)

	if got := severityTargetPath(DeclarativeSectionRules("allergiesAndIntolerances")); got != "severity" {
		t.Errorf("declarativeSectionRuleGroupsCache singleton mutated via clone: severity TargetPath = %q", got)
	}
}

func mutateSeverityRow(rules []MappingRule) {
	for i := range rules {
		for j := range rules[i].Fields {
			for k := range rules[i].Fields[j].Fields {
				if rules[i].Fields[j].Fields[k].TargetPath == "severity" {
					rules[i].Fields[j].Fields[k].TargetPath = "MUTATED"
				}
			}
		}
	}
}

func severityTargetPath(rules []MappingRule) string {
	for _, rule := range rules {
		for _, row := range rule.Fields {
			for _, child := range row.Fields {
				if child.TargetPath == "severity" || child.TargetPath == "MUTATED" {
					return child.TargetPath
				}
			}
		}
	}
	return ""
}

// TestApplyFieldOverrides_ReplaceChangesTargetPathAndTransform verifies the
// override by holding a pointer to the row taken BEFORE patching, located by
// its original TargetPath. Re-flattening AFTER ApplyFieldOverrides and
// looking up by the new Key would be circular: Key is derived from
// TargetPath for top-level rows (by design — see FlattenSectionRules' own
// doc comment), and overriding FHIRPath is exactly what changes TargetPath —
// production code never re-flattens a rule slice ApplyFieldOverrides has
// already patched (DeclarativeMapDocument passes the patched rules straight
// into BuildResourcesForRules; FlattenSectionRules is only ever called on
// the un-patched OOB cache), so this is the correct verification shape, not
// a workaround.
func TestApplyFieldOverrides_ReplaceChangesTargetPathAndTransform(t *testing.T) {
	rules := CloneMappingRules(AllergyMappingRules())

	var codeRow, clinicalStatusRow *MappingRow
	for i := range rules[0].Fields {
		switch rules[0].Fields[i].TargetPath {
		case "code":
			codeRow = &rules[0].Fields[i]
		case "clinicalStatus":
			clinicalStatusRow = &rules[0].Fields[i]
		}
	}
	if codeRow == nil || clinicalStatusRow == nil {
		t.Fatal("AllergyMappingRules() shape changed — expected top-level code and clinicalStatus rows")
	}

	overrides := []CDAMappingOverride{
		{
			Action:     "replace",
			SectionKey: "allergiesAndIntolerances",
			CDAField:   "code",
			FHIRPath:   "code.coding[1].code",
			Transform:  "custom_transform",
		},
	}
	ApplyFieldOverrides(rules, overrides)

	if codeRow.TargetPath != "code.coding[1].code" {
		t.Errorf("code.TargetPath = %q, want %q", codeRow.TargetPath, "code.coding[1].code")
	}
	if codeRow.Transform != "custom_transform" {
		t.Errorf("code.Transform = %q, want %q", codeRow.Transform, "custom_transform")
	}
	if clinicalStatusRow.TargetPath != "clinicalStatus" {
		t.Errorf("clinicalStatus.TargetPath = %q, want unmodified %q", clinicalStatusRow.TargetPath, "clinicalStatus")
	}
}

// TestApplyFieldOverrides_NestedFieldOverride exercises the recursive branch
// of applyOverridesToRows — see the doc comment above for why this holds
// pointers taken before patching rather than re-flattening afterward.
func TestApplyFieldOverrides_NestedFieldOverride(t *testing.T) {
	rules := CloneMappingRules(AllergyMappingRules())

	var reactionRow *MappingRow
	for i := range rules[0].Fields {
		if rules[0].Fields[i].TargetPath == "reaction" {
			reactionRow = &rules[0].Fields[i]
		}
	}
	if reactionRow == nil {
		t.Fatal("AllergyMappingRules() shape changed — expected a top-level reaction row")
	}

	var severityRow, manifestationRow *MappingRow
	for i := range reactionRow.Fields {
		switch reactionRow.Fields[i].TargetPath {
		case "severity":
			severityRow = &reactionRow.Fields[i]
		case "manifestation[0]":
			manifestationRow = &reactionRow.Fields[i]
		}
	}
	if severityRow == nil || manifestationRow == nil {
		t.Fatal("AllergyMappingRules() shape changed — expected nested severity and manifestation[0] rows under reaction")
	}

	overrides := []CDAMappingOverride{
		{
			Action:     "replace",
			SectionKey: "allergiesAndIntolerances",
			CDAField:   "reaction.severity",
			FHIRPath:   "reaction[0].severity",
			Transform:  "custom_severity_transform",
		},
	}
	ApplyFieldOverrides(rules, overrides)

	if severityRow.TargetPath != "reaction[0].severity" {
		t.Errorf("severity.TargetPath = %q, want %q", severityRow.TargetPath, "reaction[0].severity")
	}
	if severityRow.Transform != "custom_severity_transform" {
		t.Errorf("severity.Transform = %q, want %q", severityRow.Transform, "custom_severity_transform")
	}
	if manifestationRow.TargetPath != "manifestation[0]" {
		t.Errorf("manifestation.TargetPath = %q, want unmodified %q", manifestationRow.TargetPath, "manifestation[0]")
	}
}
