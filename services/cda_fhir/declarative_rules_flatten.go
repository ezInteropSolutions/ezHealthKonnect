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
// Only Action=="replace" overrides are honored here. "add"/"remove"/
// "add_section"/"remove_section" remain meaningful for the flat-
// CDAFieldMapping config-browsing API (getCDAFieldMappings/mergeCDAMappings)
// only — the declarative engine's Fields[] rows are the unit of execution,
// not independently addable atoms, so adding a brand-new row at runtime
// would need Scope/SourcePath/Transform semantics with no existing row to
// inherit defaults from. This is out of scope: the override surface this
// function implements is exactly FHIRPath/Transform/ValueMap/Conformance/
// Required on EXISTING fields, matching the Section Field Editor's actual UI.
func ApplyFieldOverrides(rules []MappingRule, overrides []CDAMappingOverride) {
	if len(overrides) == 0 {
		return
	}
	byKey := make(map[string]CDAMappingOverride, len(overrides))
	for _, ov := range overrides {
		if ov.Action != "replace" {
			continue
		}
		byKey[ov.CDAField] = ov
	}
	if len(byKey) == 0 {
		return
	}
	for i := range rules {
		applyOverridesToRows(rules[i].Fields, "", byKey)
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
