// services/cda_fhir/declarative_rules_flatten.go
//
// Bridges the OOB declarative rule literals (declarative_oob_rules.go,
// declarative_schema.go) with the CDA mapping *configuration* layer
// (generic_mapper.go, controllers/cda_schema_controller.go, the
// pipeline-builder Section field editor). Until this file existed, that
// configuration layer had its own, unrelated notion of "what fields exist in
// a section" (a static ccda_2_1.json USCDI schema) and "what the OOB default
// is" (cda_fhir_templates.template_config) — neither matched what
// DeclarativeMapDocument actually executes. This file is the single place
// that turns a real []MappingRule into the flat, addressable field list both
// the config-browsing API and the runtime override-patch path need, so both
// sides finally agree on one source of truth.
package cdafhir

import "fmt"

// FlattenedField is one addressable, editable field within a section's
// MappingRule set — the unit the Section Field Editor UI lists one row per,
// and the unit CDAMappingOverride.CDAField identifies.
type FlattenedField struct {
	Key              string // TargetPath, dot-joined for nested rows (e.g. "reaction.manifestation[0]")
	SectionKey       string
	FHIRResource     string
	CDASourceDisplay string // Scope+SourcePath joined, or literal/condition description
	TargetPath       string
	Transform        string
	ValueMap         map[string]string
	Conformance      string
	Required         bool
	NestedUnder      string // "" for top-level; parent TargetPath for nested rows
}

// FlattenSectionRules walks every MappingRule in rules (all rules sharing one
// SectionKey, e.g. both Medication rules) and returns one FlattenedField per
// top-level Fields[] row, plus one per row nested under a CollectAll+Fields
// parent (exactly one level — no current *MappingRules() function nests
// deeper). A CollectAll+Fields PARENT row itself produces no FlattenedField
// of its own: per MappingRow.Fields' own doc comment, "this row's OWN
// SourcePath/Transform/... are ignored when Fields is set" — it has no
// scalar value to expose for editing, only its children do.
func FlattenSectionRules(rules []MappingRule) []FlattenedField {
	var out []FlattenedField
	for _, rule := range rules {
		for _, row := range rule.Fields {
			out = append(out, flattenRow(rule, row, "", "")...)
		}
	}
	return out
}

// flattenRow emits the FlattenedField for one row (recursing one level into
// row.Fields when set) and threads the parent's Scope down so nested rows
// without their own Scope still get a non-blank CDA source description.
func flattenRow(rule MappingRule, row MappingRow, nestedUnder, inheritedScope string) []FlattenedField {
	key := row.TargetPath
	if nestedUnder != "" {
		key = nestedUnder + "." + row.TargetPath
	}

	if len(row.Fields) > 0 {
		// CollectAll+Fields parent: no FlattenedField of its own, only children.
		var out []FlattenedField
		for _, child := range row.Fields {
			out = append(out, flattenRow(rule, child, row.TargetPath, row.Scope)...)
		}
		return out
	}

	return []FlattenedField{{
		Key:              key,
		SectionKey:       rule.SectionKey,
		FHIRResource:     rule.FHIRResource,
		CDASourceDisplay: describeCDASource(row, inheritedScope),
		TargetPath:       row.TargetPath,
		Transform:        row.Transform,
		ValueMap:         row.ValueMap,
		Conformance:      row.Conformance,
		Required:         row.Required,
		NestedUnder:      nestedUnder,
	}}
}

// describeCDASource renders the effective CDA-side path for display.
// inheritedScope is the parent row's own Scope, used as a prefix when this
// row is nested and has no Scope of its own (e.g. Allergy's
// reaction.manifestation[0]/reaction.severity rows, whose real CDA root is
// the parent "reaction" CollectAll row's Scope).
func describeCDASource(row MappingRow, inheritedScope string) string {
	scope := row.Scope
	if scope == "" {
		scope = inheritedScope
	}

	if row.SourcePath != "" {
		if scope != "" {
			return scope + "." + row.SourcePath
		}
		return row.SourcePath
	}

	if row.Condition != nil && row.Condition.WhenPath != "" {
		whenScope := scope
		desc := fmt.Sprintf("condition: %s=%s", row.Condition.WhenPath, row.Condition.Equals)
		if whenScope != "" {
			desc = fmt.Sprintf("condition: %s.%s=%s", whenScope, row.Condition.WhenPath, row.Condition.Equals)
		}
		return desc
	}

	if row.LiteralValue != nil {
		return fmt.Sprintf("literal: %v", row.LiteralValue)
	}

	return scope
}

// DescribeAddedSource renders a CDA-source display string for a saved
// Action=="add" override — the config-browsing API's equivalent of
// describeCDASource for a field that has no MappingRow at all (only the
// override's own raw Scope/SourcePath; an "add" override has no Condition or
// LiteralValue in its schema, so this doesn't need describeCDASource's fuller
// logic, just its same "scope.sourcePath" display convention).
func DescribeAddedSource(scope, sourcePath string) string {
	if sourcePath != "" {
		if scope != "" {
			return scope + "." + sourcePath
		}
		return sourcePath
	}
	return scope
}

// CloneMappingRules deep-copies rules (and recursively their Fields,
// Condition, ValueMap, RequiredPaths, PatientRefPath) so callers can patch
// the copy without mutating declarativeSectionRuleGroupsCache, which is read
// concurrently by every in-flight DeclarativeMapDocument call across its own
// per-section goroutines. Callers MUST use this before any in-place mutation
// of a rule slice obtained from the cache or from DeclarativeSectionRules.
func CloneMappingRules(rules []MappingRule) []MappingRule {
	out := make([]MappingRule, len(rules))
	for i, rule := range rules {
		out[i] = rule
		if rule.RequiredPaths != nil {
			out[i].RequiredPaths = append([]string(nil), rule.RequiredPaths...)
		}
		if rule.PatientRefPath != nil {
			out[i].PatientRefPath = append([]string(nil), rule.PatientRefPath...)
		}
		out[i].Fields = cloneMappingRows(rule.Fields)
	}
	return out
}

func cloneMappingRows(rows []MappingRow) []MappingRow {
	if rows == nil {
		return nil
	}
	out := make([]MappingRow, len(rows))
	for i, row := range rows {
		out[i] = row
		if row.ScopeFallbacks != nil {
			out[i].ScopeFallbacks = append([]string(nil), row.ScopeFallbacks...)
		}
		if row.FallbackPaths != nil {
			out[i].FallbackPaths = append([]string(nil), row.FallbackPaths...)
		}
		if row.ValueMap != nil {
			vm := make(map[string]string, len(row.ValueMap))
			for k, v := range row.ValueMap {
				vm[k] = v
			}
			out[i].ValueMap = vm
		}
		if row.SkipIfResourceHasAnyOf != nil {
			out[i].SkipIfResourceHasAnyOf = append([]string(nil), row.SkipIfResourceHasAnyOf...)
		}
		if row.Condition != nil {
			cond := *row.Condition
			if row.Condition.WhenPaths != nil {
				cond.WhenPaths = append([]string(nil), row.Condition.WhenPaths...)
			}
			if row.Condition.ThenValueMap != nil {
				tvm := make(map[string]string, len(row.Condition.ThenValueMap))
				for k, v := range row.Condition.ThenValueMap {
					tvm[k] = v
				}
				cond.ThenValueMap = tvm
			}
			out[i].Condition = &cond
		}
		out[i].Fields = cloneMappingRows(row.Fields)
	}
	return out
}

// ApplyFieldOverrides mutates rules IN PLACE — callers MUST pass a
// CloneMappingRules result, never a slice obtained directly from
// declarativeSectionRuleGroupsCache/DeclarativeSectionRules.
//
// Action=="replace" and Action=="remove" are matched-by-key against EXISTING
// rows via applyOverridesToRows (FHIRPath/Transform/ValueMap/Conformance/
// Required/Disabled — matching the Section Field Editor's Reset/Remove UI).
// Action=="add" is different in kind, not just degree: there is no existing
// row to match, so applyAddOverride below builds a brand-new MappingRow from
// the override's own Scope/SourcePath/TargetPath/Transform and inserts it —
// see that function's doc comment for exactly where. "add_section"/
// "remove_section" remain meaningful only for the flat-CDAFieldMapping
// config-browsing API (getCDAFieldMappings/mergeCDAMappings), not here —
// section-level add/remove has no MappingRow-insertion analogue.
func ApplyFieldOverrides(rules []MappingRule, overrides []CDAMappingOverride) {
	if len(overrides) == 0 {
		return
	}
	byKey := make(map[string]CDAMappingOverride, len(overrides))
	var addOverrides []CDAMappingOverride
	for _, ov := range overrides {
		switch ov.Action {
		case "replace", "remove":
			byKey[ov.CDAField] = ov
		case "add":
			addOverrides = append(addOverrides, ov)
		}
	}
	if len(byKey) > 0 {
		for i := range rules {
			applyOverridesToRows(rules[i].Fields, "", byKey)
		}
	}
	for _, ov := range addOverrides {
		applyAddOverride(rules, ov)
	}
}

func applyOverridesToRows(rows []MappingRow, nestedUnder string, byKey map[string]CDAMappingOverride) {
	for i := range rows {
		row := &rows[i]
		if len(row.Fields) > 0 {
			applyOverridesToRows(row.Fields, row.TargetPath, byKey)
			continue
		}
		key := row.TargetPath
		if nestedUnder != "" {
			key = nestedUnder + "." + row.TargetPath
		}
		ov, ok := byKey[key]
		if !ok {
			continue
		}
		if ov.Action == "remove" {
			// A removed field's FHIRPath/Transform/etc. are moot — applyRow
			// skips the row entirely before ever reading them.
			row.Disabled = true
			continue
		}
		if ov.FHIRPath != "" {
			row.TargetPath = ov.FHIRPath
		}
		if ov.Transform != "" {
			row.Transform = ov.Transform
		}
		if ov.ValueMap != nil {
			row.ValueMap = ov.ValueMap
		}
		if ov.Conformance != "" {
			row.Conformance = ov.Conformance
		}
		if ov.IsRequired {
			row.Required = ov.IsRequired
		}
	}
}

// applyAddOverride inserts one brand-new MappingRow (built from ov's own
// Scope/SourcePath/TargetPath/Transform/ValueMap/Conformance/IsRequired)
// into every rule variant ov.TargetFHIRResources targets (all variants when
// unset — the same indiscriminate fan-out replace/remove already have).
// newRow.TargetPath is always the BARE property name (e.g. "onset", never
// "reaction.onset") — matching how every existing nested row already works;
// the dotted lookup key only exists at FlattenSectionRules' flattened-key
// layer (nestedUnder + "." + row.TargetPath), never on the row itself.
//
// Defensive, not fatal, throughout: a rule this override doesn't target, or
// a NestedUnder parent that doesn't exist on some targeted rule, is a silent
// no-op for that rule — this engine's existing convention is that a Scope/
// target resolving to nothing is normal, not an error (see MappingRow's own
// doc comment on Scope). Real misconfiguration is caught earlier, at
// save time, by ValidateAddOverride — this function never needs to error.
func applyAddOverride(rules []MappingRule, ov CDAMappingOverride) {
	newRow := MappingRow{
		Scope:       ov.Scope,
		SourcePath:  ov.SourcePath,
		TargetPath:  ov.FHIRPath,
		Transform:   ov.Transform,
		ValueMap:    ov.ValueMap,
		Conformance: ov.Conformance,
		Required:    ov.IsRequired,
		CollectAll:  ov.CollectAll,
	}
	for i := range rules {
		rule := &rules[i]
		if !ruleIsTargeted(ov.TargetFHIRResources, rule.FHIRResource) {
			continue
		}
		if ov.NestedUnder == "" {
			if rowKeyExists(rule.Fields, newRow.TargetPath) {
				continue // idempotent — already present, don't duplicate
			}
			rule.Fields = append(rule.Fields, newRow)
			continue
		}
		parent := findCollectAllParent(rule.Fields, ov.NestedUnder)
		if parent == nil {
			continue // this rule variant has no such nested group — no-op
		}
		if rowKeyExists(parent.Fields, newRow.TargetPath) {
			continue
		}
		parent.Fields = append(parent.Fields, newRow)
	}
}

// ruleIsTargeted reports whether fhirResource is one of targets, or targets
// is empty (meaning "every rule variant in the section").
func ruleIsTargeted(targets []string, fhirResource string) bool {
	if len(targets) == 0 {
		return true
	}
	for _, t := range targets {
		if t == fhirResource {
			return true
		}
	}
	return false
}

// findCollectAllParent locates an existing nested-group parent row by its
// TargetPath — a row counts as a "parent" only when it has its own Fields
// (a CollectAll+Fields row), matching flattenRow's own definition of one.
func findCollectAllParent(rows []MappingRow, targetPath string) *MappingRow {
	for i := range rows {
		if rows[i].TargetPath == targetPath && len(rows[i].Fields) > 0 {
			return &rows[i]
		}
	}
	return nil
}

// rowKeyExists reports whether a LEAF row (no Fields of its own) with this
// bare TargetPath already exists among rows — the idempotency guard so
// re-applying the same "add" override twice never duplicates a row.
func rowKeyExists(rows []MappingRow, targetPath string) bool {
	for _, r := range rows {
		if len(r.Fields) == 0 && r.TargetPath == targetPath {
			return true
		}
	}
	return false
}

// ValidateAddOverride checks an Action=="add" override against the section's
// real rule shape BEFORE it's persisted — called from
// CDASchemaController.ComputeDelta, never from the runtime engine (which
// stays purely defensive — see applyAddOverride's own doc comment). Returns
// nil for non-"add" overrides.
//
// Nested adds are validated strictly: the named group must exist in EVERY
// targeted resource, or this returns an error naming the gap. Top-level adds
// are NOT held to that same strictness — no per-resource existence check —
// because "replace"/"remove" already apply indiscriminately across every
// rule variant in a section today, and a top-level field resolving to
// nothing on some variant is this engine's normal "Scope matched nothing"
// behavior, not a configuration error. Forcing strict validation there too
// would be inconsistent with existing behavior and needlessly naggy for the
// common case (most sections have exactly one rule variant).
func ValidateAddOverride(ov CDAMappingOverride) error {
	if ov.Action != "add" {
		return nil
	}
	if ov.FHIRPath == "" {
		return fmt.Errorf("cda mapping add: %q: fhirPath is required", ov.CDAField)
	}
	// CollectAll is only ever exercised, in the whole OOB rule set, as a
	// top-level row (services/cda_fhir/declarative_oob_rules.go's plain-loop
	// CollectAll rows — e.g. reasonCode — have no parent group). Nesting a
	// CollectAll row under an existing CollectAll+Fields group is
	// unprecedented and applyAddOverride's Fields-append logic doesn't
	// special-case it, so reject it explicitly rather than silently produce
	// an unproven shape. The frontend already enforces this via mutual
	// exclusivity in the UI; this is the defensive server-side backstop.
	if ov.CollectAll && ov.NestedUnder != "" {
		return fmt.Errorf("cda mapping add: %q: collectAll is only supported for a top-level field, not nested under %q", ov.CDAField, ov.NestedUnder)
	}
	rules := DeclarativeSectionRules(ov.SectionKey)
	if len(rules) == 0 {
		return fmt.Errorf("cda mapping add: unknown sectionKey %q", ov.SectionKey)
	}

	targets := ov.TargetFHIRResources
	if len(targets) == 0 {
		for _, r := range rules {
			targets = append(targets, r.FHIRResource)
		}
	}
	byResource := make(map[string]MappingRule, len(rules))
	for _, r := range rules {
		byResource[r.FHIRResource] = r
	}
	for _, t := range targets {
		rule, ok := byResource[t]
		if !ok {
			return fmt.Errorf("cda mapping add: %q: unknown FHIR resource %q for section %q", ov.CDAField, t, ov.SectionKey)
		}
		if ov.NestedUnder != "" && findCollectAllParent(rule.Fields, ov.NestedUnder) == nil {
			return fmt.Errorf("cda mapping add: %q: resource %q has no nested group %q", ov.CDAField, t, ov.NestedUnder)
		}
	}
	return nil
}
