// services/fhir_narrative/generator.go
// FHIRNarrativeGenerator dispatches FHIR resource narrative generation by resourceType.
// If resource["text"]["div"] is already present it is returned unchanged.
// Generated XHTML uses USCDI v3 labels for all field headings.

package fhirnarrative

import "strings"

// FHIRNarrativeGenerator generates XHTML narrative for FHIR resources that
// lack resource.text.div. One instance is stateless and safe for concurrent use.
type FHIRNarrativeGenerator struct{}

// NewFHIRNarrativeGenerator returns a ready-to-use generator.
func NewFHIRNarrativeGenerator() *FHIRNarrativeGenerator {
	return &FHIRNarrativeGenerator{}
}

// Generate returns an XHTML narrative div for the given FHIR resource.
// If resource["text"]["div"] is already populated it is returned as-is.
// Returns an empty string for unsupported resource types.
func (g *FHIRNarrativeGenerator) Generate(resource map[string]interface{}) string {
	if resource == nil {
		return ""
	}
	// Return existing narrative unchanged — never overwrite what the source set.
	if textMap, ok := resource["text"].(map[string]interface{}); ok {
		if div, ok := textMap["div"].(string); ok && div != "" {
			return div
		}
	}
	switch fhirStr(resource, "resourceType") {
	case "AllergyIntolerance":
		return GenerateAllergyNarrative(resource)
	case "Condition":
		return GenerateConditionNarrative(resource)
	case "MedicationStatement":
		return GenerateMedicationNarrative(resource)
	case "MedicationRequest":
		return GenerateMedicationNarrative(resource)
	case "Observation":
		return GenerateObservationNarrative(resource)
	case "Patient":
		return GeneratePatientNarrative(resource)
	case "Encounter":
		return GenerateEncounterNarrative(resource)
	case "Procedure":
		return GenerateProcedureNarrative(resource)
	case "Immunization":
		return GenerateImmunizationNarrative(resource)
	case "FamilyMemberHistory":
		return GenerateFamilyMemberHistoryNarrative(resource)
	case "Goal":
		return GenerateGoalNarrative(resource)
	case "Coverage":
		return GenerateCoverageNarrative(resource)
	case "DeviceUseStatement":
		return GenerateDeviceNarrative(resource)
	case "CarePlan":
		return GenerateCarePlanNarrative(resource)
	case "Practitioner":
		return GeneratePractitionerNarrative(resource)
	case "Organization":
		return GenerateOrganizationNarrative(resource)
	default:
		return ""
	}
}

// ─── shared field helpers ─────────────────────────────────────────────────────

// fhirStr returns the string value at key in m, or "" if absent or wrong type.
func fhirStr(m map[string]interface{}, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// fhirArr returns the []interface{} value at key in m, or nil.
func fhirArr(m map[string]interface{}, key string) []interface{} {
	if v, ok := m[key].([]interface{}); ok {
		return v
	}
	return nil
}

// fhirMap returns the nested map[string]interface{} at key in m, or nil.
func fhirMap(m map[string]interface{}, key string) map[string]interface{} {
	if v, ok := m[key].(map[string]interface{}); ok {
		return v
	}
	return nil
}

// ccText extracts a human-readable label from a FHIR CodeableConcept.
// Priority: text → first coding display → first coding code.
func ccText(v interface{}) string {
	cc, ok := v.(map[string]interface{})
	if !ok {
		return ""
	}
	if t, ok := cc["text"].(string); ok && t != "" {
		return t
	}
	codings, _ := cc["coding"].([]interface{})
	for _, raw := range codings {
		c, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if d, ok := c["display"].(string); ok && d != "" {
			return d
		}
		if code, ok := c["code"].(string); ok && code != "" {
			return code
		}
	}
	return ""
}

// ccCode extracts the first coding.code from a FHIR CodeableConcept.
func ccCode(v interface{}) string {
	cc, ok := v.(map[string]interface{})
	if !ok {
		return ""
	}
	codings, _ := cc["coding"].([]interface{})
	for _, raw := range codings {
		c, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if code, ok := c["code"].(string); ok && code != "" {
			return code
		}
	}
	return ""
}

// ─── XHTML rendering helpers ──────────────────────────────────────────────────

// escapeXHTML replaces XML special characters so the output is valid XHTML.
func escapeXHTML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	return s
}

// wrapDiv wraps inner content in the FHIR-required XHTML namespace div.
func wrapDiv(inner string) string {
	return `<div xmlns="http://www.w3.org/1999/xhtml">` + inner + `</div>`
}

// heading renders a bold section heading paragraph.
func heading(label string) string {
	return `<p><b>` + escapeXHTML(label) + `</b></p>`
}

// tableRow renders one <tr> with a <th> label and <td> value.
// Returns empty string when value is blank (omits the row entirely).
func tableRow(label, value string) string {
	if value == "" {
		return ""
	}
	return `<tr><th>` + escapeXHTML(label) + `</th><td>` + escapeXHTML(value) + `</td></tr>`
}

// buildTable wraps collected row strings in <table><tbody>.
// Returns empty string when rows is blank.
func buildTable(rows string) string {
	if rows == "" {
		return ""
	}
	return `<table><tbody>` + rows + `</tbody></table>`
}
