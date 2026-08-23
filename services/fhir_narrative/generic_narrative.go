// services/fhir_narrative/generic_narrative.go
//
// generateGeneric renders an XHTML narrative for ANY FHIR resource type, driven
// by the resource's own JSON shape rather than per-type hand-written logic. It
// is what Generate() falls back to for resource types with no dedicated builder
// in this package, and it is ALSO what Generate() uses whenever a caller
// supplies a NarrativeFieldConfig restriction for that resourceType — even for
// types that DO have a dedicated builder — so per-interface field
// configuration behaves uniformly no matter which resource type it targets.
//
// Because it detects shape (CodeableConcept/Reference/Quantity/Period/
// HumanName/Address) rather than assuming what datatype a field "should" be
// per spec, it also degrades gracefully when an upstream field mapping targets
// the wrong FHIR datatype for a field (e.g. a CE-composite value mistakenly
// mapped into a plain `code` field) — it still renders something meaningful
// instead of silently dropping the value.
package fhirnarrative

import (
	"fmt"
	"sort"
	"strings"

	"ezhealthkonnect/fhir"
)

// NarrativeFieldConfig restricts which top-level fields render per resource
// type. A resourceType key absent from the map means "no restriction — render
// every populated field" (the default). A present key restricts rendering to
// exactly the listed field names.
type NarrativeFieldConfig map[string][]string

// AllowsField reports whether fieldName should render for resourceType, given
// cfg. A nil cfg, or a resourceType absent from cfg, allows everything.
func (cfg NarrativeFieldConfig) AllowsField(resourceType, fieldName string) bool {
	if cfg == nil {
		return true
	}
	allowed, restricted := cfg[resourceType]
	if !restricted {
		return true
	}
	for _, f := range allowed {
		if f == fieldName {
			return true
		}
	}
	return false
}

// Restricts reports whether cfg has an explicit field list for resourceType —
// used by Generate to decide whether to bypass a dedicated per-type builder in
// favor of the generic renderer (so a configured restriction is never silently
// ignored by a resource type that happens to have hand-written logic).
func (cfg NarrativeFieldConfig) Restricts(resourceType string) bool {
	if cfg == nil {
		return false
	}
	_, restricted := cfg[resourceType]
	return restricted
}

// skipFields are structural/administrative — never meaningful narrative content.
var skipFields = map[string]bool{
	"resourceType":  true,
	"id":            true,
	"text":          true,
	"meta":          true,
	"contained":     true,
	"extension":     true,
	"implicitRules": true,
	"language":      true,
}

// IsSkippedField reports whether field is a structural/administrative field
// that the narrative renderer never shows regardless of NarrativeFieldConfig.
// Exported so the narrative field picker's catalog endpoint
// (controllers/narrative_fields_controller.go) can omit them too — otherwise
// the picker would offer checkboxes (e.g. for "id"/"extension") that have no
// visible effect either way.
func IsSkippedField(field string) bool {
	return skipFields[field]
}

// fieldDisplayValueLookup translates specific coded values into friendlier
// display text for specific "ResourceType.field" pairs. A small, explicit,
// reviewed set of exceptions — not a return to per-type functions; every other
// field renders its own coding/text/value as-is.
var fieldDisplayValueLookup = map[string]map[string]string{
	"Encounter.class": {
		"I": "Inpatient", "O": "Outpatient", "E": "Emergency",
		"IMP": "Inpatient", "AMB": "Outpatient", "EMER": "Emergency",
	},
}

// generateGeneric renders resource's populated fields generically, restricted
// to fieldConfig when set for resourceType. schema is optional — when present,
// field labels come from the FHIR spec's element names/descriptions; when nil,
// a label is derived by space-casing the JSON field name.
func generateGeneric(resourceType string, resource map[string]interface{}, schema *fhir.FHIRSchema, fieldConfig NarrativeFieldConfig) string {
	keys := make([]string, 0, len(resource))
	for k := range resource {
		if skipFields[k] {
			continue
		}
		if !fieldConfig.AllowsField(resourceType, k) {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var rows string
	for _, key := range keys {
		value := renderFieldValue(resourceType, key, resource[key])
		if value == "" {
			continue
		}
		rows += tableRow(fieldLabel(resourceType, key, schema), value)
	}

	return wrapDiv(heading(spaceCasePascal(resourceType)) + buildTable(rows))
}

// fieldLabel resolves a human label for a field, preferring the FHIR schema's
// own element name/description when available.
func fieldLabel(resourceType, field string, schema *fhir.FHIRSchema) string {
	if schema != nil {
		if el, ok := schema.Elements[resourceType+"."+field]; ok {
			if el.Name != "" {
				return el.Name
			}
			if el.Description != "" {
				return el.Description
			}
		}
	}
	return spaceCasePascal(field)
}

// spaceCasePascal inserts spaces before capital letters and upper-cases the
// first letter, e.g. "birthDate" -> "Birth Date", "onsetDateTime" -> "Onset Date Time".
func spaceCasePascal(s string) string {
	if s == "" {
		return s
	}
	var b strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte(' ')
		}
		if i == 0 && r >= 'a' && r <= 'z' {
			r = r - 'a' + 'A'
		}
		b.WriteRune(r)
	}
	return b.String()
}

// renderFieldValue renders a single field's value by its actual JSON shape.
func renderFieldValue(resourceType, field string, v interface{}) string {
	switch val := v.(type) {
	case string:
		return lookupDisplay(resourceType, field, val)
	case bool:
		return fmt.Sprintf("%v", val)
	case float64:
		return fmt.Sprintf("%g", val)
	case map[string]interface{}:
		return renderObjectValue(resourceType, field, val)
	case []interface{}:
		var parts []string
		for _, item := range val {
			if s := renderArrayItemValue(resourceType, field, item); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, "; ")
	default:
		return ""
	}
}

// renderObjectValue renders a nested object by recognizing common FHIR data
// type shapes structurally — CodeableConcept, Reference, Quantity, HumanName,
// Address, Period — not by which resourceType/field it came from.
func renderObjectValue(resourceType, field string, m map[string]interface{}) string {
	if _, hasCoding := m["coding"]; hasCoding {
		return lookupDisplay(resourceType, field, ccText(m))
	}
	if t, ok := m["text"].(string); ok && t != "" {
		return t
	}
	if d, ok := m["display"].(string); ok && d != "" {
		return d
	}
	if r, ok := m["reference"].(string); ok && r != "" {
		return r
	}
	if num, ok := m["value"]; ok {
		var numStr string
		switch n := num.(type) {
		case float64:
			numStr = fmt.Sprintf("%g", n)
		case string:
			numStr = n
		}
		if numStr != "" {
			if comp, ok := m["comparator"].(string); ok && comp != "" {
				numStr = comp + " " + numStr
			}
			if unit, ok := m["unit"].(string); ok && unit != "" {
				return numStr + " " + unit
			}
			return numStr
		}
	}
	if family, ok := m["family"].(string); ok {
		given := ""
		if gv, ok := m["given"].([]interface{}); ok && len(gv) > 0 {
			if s, ok := gv[0].(string); ok {
				given = s
			}
		}
		if family != "" || given != "" {
			return strings.TrimSpace(strings.TrimSuffix(family+", "+given, ", "))
		}
	}
	if _, hasCity := m["city"]; hasCity {
		var parts []string
		if line, ok := m["line"].([]interface{}); ok {
			for _, l := range line {
				if s, ok := l.(string); ok && s != "" {
					parts = append(parts, s)
				}
			}
		}
		if city, ok := m["city"].(string); ok && city != "" {
			parts = append(parts, city)
		}
		if state, ok := m["state"].(string); ok && state != "" {
			parts = append(parts, state)
		}
		if len(parts) > 0 {
			return strings.Join(parts, ", ")
		}
	}
	if start, hasStart := m["start"]; hasStart {
		startStr, _ := start.(string)
		if endStr, _ := m["end"].(string); endStr != "" {
			return startStr + " – " + endStr
		}
		return startStr
	}
	// Generic fallback: a bare descriptive string on an otherwise-unrecognized
	// nested object (e.g. AllergyIntolerance.reaction[].description).
	if desc, ok := m["description"].(string); ok && desc != "" {
		return desc
	}
	return ""
}

// renderArrayItemValue renders one array entry using the same shape rules as
// scalars/objects, so arrays of CodeableConcept/Reference/HumanName/etc. all
// render sensibly without per-type code.
func renderArrayItemValue(resourceType, field string, item interface{}) string {
	switch v := item.(type) {
	case string:
		return v
	case map[string]interface{}:
		return renderObjectValue(resourceType, field, v)
	default:
		return ""
	}
}

// lookupDisplay applies fieldDisplayValueLookup's small set of known
// code -> friendly-label translations; returns raw unchanged when no entry exists.
func lookupDisplay(resourceType, field, raw string) string {
	if byField, ok := fieldDisplayValueLookup[resourceType+"."+field]; ok {
		if friendly, ok := byField[raw]; ok {
			return friendly
		}
	}
	return raw
}
