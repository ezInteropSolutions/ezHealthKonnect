package cdafhir

import (
	"sort"
	"strings"
	"testing"
)

func TestFlattenSectionRules_Allergy_ProducesCoreClinicalFields(t *testing.T) {
	fields := FlattenSectionRules(AllergyMappingRules())

	got := make(map[string]bool, len(fields))
	for _, f := range fields {
		got[f.Key] = true
	}

	// Core clinical FHIR fields that must always be present.
	// recorder / asserter / recordedDate are also present now (V188 IG alignment)
	// but are excluded from this list because their flatten keys vary (nested
	// PractitionerRole rows produce "practitioner.*" and "organization.*" prefixes
	// due to the two-tier recursion in flattenRow -- the important invariant is
	// that the core allergy fields are not disrupted by the new rows).
	mustHave := []string{
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
	for _, key := range mustHave {
		if !got[key] {
			t.Errorf("FlattenSectionRules(AllergyMappingRules()) missing expected key %q; got keys: %v", key, sortedKeys(got))
		}
	}

	// These keys must never appear (stale ccda_2_1.json fields or bare CollectAll parents).
	mustNotHave := []string{"authorGiven", "authorFamily", "authorNPI", "reaction"}
	for _, key := range mustNotHave {
		if got[key] {
			t.Errorf("FlattenSectionRules produced unexpected key %q (stale ccda_2_1.json field or bare CollectAll parent)", key)
		}
	}
}

func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
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

// TestApplyFieldOverrides_RemoveActionDisablesRow verifies Action=="remove"
// sets MappingRow.Disabled on exactly the targeted row (top-level and
// nested), leaving FHIRPath/Transform untouched and every sibling row
// unaffected — applyRow is what actually skips a Disabled row at execution
// time (see declarative_engine_test.go's TestDeclarativeEngine_DisabledRow
// counterpart); this test only verifies the override reaches the right row.
func TestApplyFieldOverrides_RemoveActionDisablesRow(t *testing.T) {
	rules := CloneMappingRules(AllergyMappingRules())

	var codeRow, criticalityRow, reactionRow *MappingRow
	for i := range rules[0].Fields {
		switch rules[0].Fields[i].TargetPath {
		case "code":
			codeRow = &rules[0].Fields[i]
		case "criticality":
			criticalityRow = &rules[0].Fields[i]
		case "reaction":
			reactionRow = &rules[0].Fields[i]
		}
	}
	if codeRow == nil || criticalityRow == nil || reactionRow == nil {
		t.Fatal("AllergyMappingRules() shape changed — expected top-level code, criticality, and reaction rows")
	}
	var severityRow *MappingRow
	for i := range reactionRow.Fields {
		if reactionRow.Fields[i].TargetPath == "severity" {
			severityRow = &reactionRow.Fields[i]
		}
	}
	if severityRow == nil {
		t.Fatal("AllergyMappingRules() shape changed — expected a nested severity row under reaction")
	}

	overrides := []CDAMappingOverride{
		{Action: "remove", SectionKey: "allergiesAndIntolerances", CDAField: "criticality"},
		{Action: "remove", SectionKey: "allergiesAndIntolerances", CDAField: "reaction.severity"},
	}
	ApplyFieldOverrides(rules, overrides)

	if !criticalityRow.Disabled {
		t.Error("criticality.Disabled = false, want true")
	}
	if !severityRow.Disabled {
		t.Error("reaction.severity.Disabled = false, want true")
	}
	if codeRow.Disabled {
		t.Error("code.Disabled = true, want false — code was never targeted by a remove override")
	}
	if criticalityRow.TargetPath != "criticality" {
		t.Errorf("criticality.TargetPath = %q, want unmodified %q (a remove override carries no FHIRPath to apply)", criticalityRow.TargetPath, "criticality")
	}
}

// TestApplyFieldOverrides_AddTopLevelField verifies Action=="add" appends a
// brand-new MappingRow to a rule's top-level Fields, built from the
// override's own Scope/SourcePath/FHIRPath/Transform — there is no existing
// row to match by key, unlike replace/remove.
func TestApplyFieldOverrides_AddTopLevelField(t *testing.T) {
	rules := CloneMappingRules(AllergyMappingRules())
	before := len(rules[0].Fields)

	overrides := []CDAMappingOverride{
		{
			Action:     "add",
			SectionKey: "allergiesAndIntolerances",
			CDAField:   "note",
			FHIRPath:   "note[0]",
			SourcePath: "text",
			Transform:  "cda_text_to_attachment",
		},
	}
	ApplyFieldOverrides(rules, overrides)

	if len(rules[0].Fields) != before+1 {
		t.Fatalf("Fields length = %d, want %d (one row appended)", len(rules[0].Fields), before+1)
	}
	added := rules[0].Fields[len(rules[0].Fields)-1]
	if added.TargetPath != "note[0]" {
		t.Errorf("added.TargetPath = %q, want %q", added.TargetPath, "note[0]")
	}
	if added.SourcePath != "text" {
		t.Errorf("added.SourcePath = %q, want %q", added.SourcePath, "text")
	}
	if added.Transform != "cda_text_to_attachment" {
		t.Errorf("added.Transform = %q, want %q", added.Transform, "cda_text_to_attachment")
	}
}

// TestApplyFieldOverrides_AddNestedField verifies a NestedUnder add appends
// to an EXISTING CollectAll+Fields parent's own Fields (not the rule's
// top-level Fields), leaving sibling nested rows untouched.
func TestApplyFieldOverrides_AddNestedField(t *testing.T) {
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
	before := len(reactionRow.Fields)

	overrides := []CDAMappingOverride{
		{
			Action:      "add",
			SectionKey:  "allergiesAndIntolerances",
			CDAField:    "reaction.onset",
			FHIRPath:    "onset",
			SourcePath:  "effectiveTime",
			NestedUnder: "reaction",
		},
	}
	ApplyFieldOverrides(rules, overrides)

	if len(reactionRow.Fields) != before+1 {
		t.Fatalf("reaction.Fields length = %d, want %d", len(reactionRow.Fields), before+1)
	}
	added := reactionRow.Fields[len(reactionRow.Fields)-1]
	if added.TargetPath != "onset" {
		t.Errorf("added.TargetPath = %q, want bare %q (not the dotted CDAField)", added.TargetPath, "onset")
	}

	// Re-flatten to confirm the new row surfaces under the right flattened key.
	flat := FlattenSectionRules(rules)
	found := false
	for _, f := range flat {
		if f.Key == "reaction.onset" {
			found = true
			if f.NestedUnder != "reaction" {
				t.Errorf("reaction.onset.NestedUnder = %q, want %q", f.NestedUnder, "reaction")
			}
		}
	}
	if !found {
		t.Error("FlattenSectionRules did not produce a \"reaction.onset\" field after the nested add")
	}
}

// TestApplyFieldOverrides_AddNestedField_NoSuchParent_NoOp verifies a
// NestedUnder naming a group that doesn't exist on this rule is a silent
// no-op (real misconfiguration is caught earlier by ValidateAddOverride, not
// here — applyAddOverride stays purely defensive, matching this engine's
// existing "Scope resolves to nothing" convention).
func TestApplyFieldOverrides_AddNestedField_NoSuchParent_NoOp(t *testing.T) {
	rules := CloneMappingRules(AllergyMappingRules())
	before := FlattenSectionRules(CloneMappingRules(AllergyMappingRules()))

	overrides := []CDAMappingOverride{
		{
			Action:      "add",
			SectionKey:  "allergiesAndIntolerances",
			CDAField:    "doesNotExist.onset",
			FHIRPath:    "onset",
			SourcePath:  "effectiveTime",
			NestedUnder: "doesNotExist",
		},
	}
	ApplyFieldOverrides(rules, overrides)

	after := FlattenSectionRules(rules)
	if len(after) != len(before) {
		t.Fatalf("field count = %d, want unchanged %d — add with an unknown NestedUnder must be a no-op", len(after), len(before))
	}
}

// TestApplyFieldOverrides_AddScopedToOneRuleVariant verifies
// TargetFHIRResources narrows an add to only the matching rule variant(s) —
// using the real two-rule Medications section (MedicationRequest vs
// MedicationStatement) rather than a synthetic example.
func TestApplyFieldOverrides_AddScopedToOneRuleVariant(t *testing.T) {
	rules := CloneMappingRules(MedicationMappingRules())
	if len(rules) != 2 {
		t.Fatalf("MedicationMappingRules() returned %d rules, want 2", len(rules))
	}
	var requestRule, statementRule *MappingRule
	for i := range rules {
		switch rules[i].FHIRResource {
		case "MedicationRequest":
			requestRule = &rules[i]
		case "MedicationStatement":
			statementRule = &rules[i]
		}
	}
	if requestRule == nil || statementRule == nil {
		t.Fatal("MedicationMappingRules() shape changed — expected MedicationRequest and MedicationStatement rules")
	}
	requestBefore := len(requestRule.Fields)
	statementBefore := len(statementRule.Fields)

	overrides := []CDAMappingOverride{
		{
			Action:              "add",
			SectionKey:          "medications",
			CDAField:            "priorAuthNumber",
			FHIRPath:            "priorAuthNumber",
			SourcePath:          "id[0].extension",
			TargetFHIRResources: []string{"MedicationRequest"},
		},
	}
	ApplyFieldOverrides(rules, overrides)

	if len(requestRule.Fields) != requestBefore+1 {
		t.Errorf("MedicationRequest.Fields length = %d, want %d (targeted)", len(requestRule.Fields), requestBefore+1)
	}
	if len(statementRule.Fields) != statementBefore {
		t.Errorf("MedicationStatement.Fields length = %d, want unchanged %d (not targeted)", len(statementRule.Fields), statementBefore)
	}
}

// TestApplyFieldOverrides_AddIsIdempotent verifies applying the same add
// override twice never duplicates the row — rowKeyExists is what guards this.
func TestApplyFieldOverrides_AddIsIdempotent(t *testing.T) {
	rules := CloneMappingRules(AllergyMappingRules())
	before := len(rules[0].Fields)

	ov := CDAMappingOverride{
		Action:     "add",
		SectionKey: "allergiesAndIntolerances",
		CDAField:   "note",
		FHIRPath:   "note[0]",
		SourcePath: "text",
	}
	ApplyFieldOverrides(rules, []CDAMappingOverride{ov})
	ApplyFieldOverrides(rules, []CDAMappingOverride{ov})

	if len(rules[0].Fields) != before+1 {
		t.Errorf("Fields length = %d, want %d — applying the same add twice must not duplicate the row", len(rules[0].Fields), before+1)
	}
}

// TestApplyFieldOverrides_AddExtensionShapedTargetPath proves applyAddOverride
// passes an extension-shaped FHIRPath straight through as MappingRow.TargetPath
// with zero special-casing — the Add Field modal's Extension mode composes
// "extension[url=...].value<Type>" client-side and relies on this being a
// verbatim string copy (confirmed by reading applyAddOverride itself), not a
// parsed/validated structure. The actual predicate-matching write behavior is
// covered separately in fhir_path_writer_test.go and
// TestDeclarativeEngine_AddedRow_ExtensionWrite_ProducesNestedExtensionElement.
func TestApplyFieldOverrides_AddExtensionShapedTargetPath(t *testing.T) {
	rules := CloneMappingRules(AllergyMappingRules())
	before := len(rules[0].Fields)

	extensionPath := "extension[url=http://hl7.org/fhir/us/core/StructureDefinition/us-core-race].valueCodeableConcept"
	overrides := []CDAMappingOverride{
		{
			Action:     "add",
			SectionKey: "allergiesAndIntolerances",
			CDAField:   "raceExtension",
			FHIRPath:   extensionPath,
			SourcePath: "value.code",
		},
	}
	ApplyFieldOverrides(rules, overrides)

	if len(rules[0].Fields) != before+1 {
		t.Fatalf("Fields length = %d, want %d", len(rules[0].Fields), before+1)
	}
	added := rules[0].Fields[len(rules[0].Fields)-1]
	if added.TargetPath != extensionPath {
		t.Errorf("added.TargetPath = %q, want the extension path passed through verbatim: %q", added.TargetPath, extensionPath)
	}
}

// TestValidateAddOverride table-drives the error paths ComputeDelta relies
// on to reject a bad "add" before it's ever persisted.
func TestValidateAddOverride(t *testing.T) {
	tests := []struct {
		name    string
		ov      CDAMappingOverride
		wantErr bool
		errSub  string
	}{
		{
			name: "valid top-level add",
			ov: CDAMappingOverride{
				Action: "add", SectionKey: "allergiesAndIntolerances",
				CDAField: "note", FHIRPath: "note[0]", SourcePath: "text",
			},
			wantErr: false,
		},
		{
			name:    "non-add action is always valid (no-op check)",
			ov:      CDAMappingOverride{Action: "replace", SectionKey: "allergiesAndIntolerances", CDAField: "code"},
			wantErr: false,
		},
		{
			name: "missing fhirPath",
			ov: CDAMappingOverride{
				Action: "add", SectionKey: "allergiesAndIntolerances", CDAField: "note",
			},
			wantErr: true,
			errSub:  "fhirPath is required",
		},
		{
			name: "unknown sectionKey",
			ov: CDAMappingOverride{
				Action: "add", SectionKey: "doesNotExist", CDAField: "note", FHIRPath: "note[0]",
			},
			wantErr: true,
			errSub:  "unknown sectionKey",
		},
		{
			name: "unknown target FHIR resource",
			ov: CDAMappingOverride{
				Action: "add", SectionKey: "allergiesAndIntolerances", CDAField: "note", FHIRPath: "note[0]",
				TargetFHIRResources: []string{"NotARealResource"},
			},
			wantErr: true,
			errSub:  "unknown FHIR resource",
		},
		{
			name: "unknown nested group",
			ov: CDAMappingOverride{
				Action: "add", SectionKey: "allergiesAndIntolerances", CDAField: "x.y", FHIRPath: "y",
				NestedUnder: "doesNotExist",
			},
			wantErr: true,
			errSub:  "no nested group",
		},
		{
			name: "known nested group",
			ov: CDAMappingOverride{
				Action: "add", SectionKey: "allergiesAndIntolerances", CDAField: "reaction.onset", FHIRPath: "onset",
				NestedUnder: "reaction",
			},
			wantErr: false,
		},
		{
			name: "collectAll rejected when nested under a group",
			ov: CDAMappingOverride{
				Action: "add", SectionKey: "allergiesAndIntolerances", CDAField: "reaction.manifestationCode", FHIRPath: "manifestationCode",
				NestedUnder: "reaction", CollectAll: true,
			},
			wantErr: true,
			errSub:  "collectAll is only supported for a top-level field",
		},
		{
			name: "collectAll valid at top level",
			ov: CDAMappingOverride{
				Action: "add", SectionKey: "allergiesAndIntolerances", CDAField: "manifestationCodes", FHIRPath: "manifestationCodes",
				CollectAll: true,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAddOverride(tt.ov)
			if tt.wantErr && err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("expected no error, got: %v", err)
			}
			if tt.wantErr && tt.errSub != "" && !strings.Contains(err.Error(), tt.errSub) {
				t.Errorf("error = %q, want it to contain %q", err.Error(), tt.errSub)
			}
		})
	}
}

// TestApplyFieldOverrides_AddWithCollectAll proves applyAddOverride actually
// wires CollectAll through onto the new MappingRow — the real gap
// declarative_rules_flatten.go had before this feature (newRow's literal
// simply never read ov.CollectAll at all, so a "capture all as a list"
// checkbox would have silently saved a singular row regardless of what the
// user checked).
func TestApplyFieldOverrides_AddWithCollectAll(t *testing.T) {
	rules := CloneMappingRules(AllergyMappingRules())
	before := len(rules[0].Fields)

	overrides := []CDAMappingOverride{
		{
			Action:     "add",
			SectionKey: "allergiesAndIntolerances",
			CDAField:   "manifestationCodes",
			FHIRPath:   "manifestationCodes",
			Scope:      "entryRelationships[typeCode=MFST].entry",
			SourcePath: "code.code",
			CollectAll: true,
		},
	}
	ApplyFieldOverrides(rules, overrides)

	if len(rules[0].Fields) != before+1 {
		t.Fatalf("Fields length = %d, want %d", len(rules[0].Fields), before+1)
	}
	added := rules[0].Fields[len(rules[0].Fields)-1]
	if !added.CollectAll {
		t.Error("added.CollectAll = false, want true — ov.CollectAll was not wired onto the new row")
	}
}
