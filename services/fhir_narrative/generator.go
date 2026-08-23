// services/fhir_narrative/generator.go
// FHIRNarrativeGenerator dispatches FHIR resource narrative generation by resourceType.
// If resource["text"]["div"] is already present it is returned unchanged.
// Generated XHTML uses USCDI v3 labels for all field headings.

package fhirnarrative

import (
	"strings"

	"ezhealthkonnect/fhir"
)

// FHIRNarrativeGenerator generates XHTML narrative for FHIR resources that
// lack resource.text.div. One instance is stateless and safe for concurrent use.
type FHIRNarrativeGenerator struct{}

// NewFHIRNarrativeGenerator returns a ready-to-use generator.
func NewFHIRNarrativeGenerator() *FHIRNarrativeGenerator {
	return &FHIRNarrativeGenerator{}
}

// Generate returns an XHTML narrative div for the given FHIR resource.
// If resource["text"]["div"] is already populated it is returned as-is.
//
// schema is optional (nil-safe) — passed through to the generic renderer for
// field labels; dedicated per-type builders below don't need it.
//
// fieldConfig is optional (nil-safe) — when it has an explicit field-list
// restriction for this resource's type (fieldConfig.Restricts(resourceType)),
// the generic renderer is used EVEN for a resource type that has a dedicated
// builder below, so a per-interface field configuration is never silently
// ignored just because that type happens to have hand-written logic. With no
// restriction, dedicated builders are preferred (they carry real, tested,
// spec-informed behavior — e.g. Observation's USCDI category-driven labeling —
// that a purely generic renderer can't replicate); anything with no dedicated
// builder falls back to the generic renderer instead of returning "".
func (g *FHIRNarrativeGenerator) Generate(resource map[string]interface{}, schema *fhir.FHIRSchema, fieldConfig NarrativeFieldConfig) string {
	if resource == nil {
		return ""
	}
	// Return existing narrative unchanged — never overwrite what the source set.
	if textMap, ok := resource["text"].(map[string]interface{}); ok {
		if div, ok := textMap["div"].(string); ok && div != "" {
			return div
		}
	}

	resourceType := fhirStr(resource, "resourceType")
	if fieldConfig.Restricts(resourceType) {
		return generateGeneric(resourceType, resource, schema, fieldConfig)
	}

	switch resourceType {
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
	case "CareTeam":
		return GenerateCareTeamNarrative(resource)
	case "ServiceRequest":
		return GenerateServiceRequestNarrative(resource)
	case "Location":
		return GenerateLocationNarrative(resource)
	default:
		return generateGeneric(resourceType, resource, schema, fieldConfig)
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

// telecomSystemLabel converts a FHIR ContactPoint.system code to a
// human-readable table-row label (e.g. "phone" -> "Phone"), the same
// code-to-label convention patient_narrative.go's patientGenderLabel already
// uses for Patient.gender. Falls back to the raw system value for anything
// outside the standard ContactPointSystem ValueSet, so an unexpected value is
// still shown rather than silently dropped.
func telecomSystemLabel(system string) string {
	switch system {
	case "phone":
		return "Phone"
	case "fax":
		return "Fax"
	case "email":
		return "Email"
	case "pager":
		return "Pager"
	case "url":
		return "URL"
	case "sms":
		return "SMS"
	case "other":
		return "Other"
	default:
		return system
	}
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

// anyText extracts display text from a field regardless of whether it arrived
// as a plain string (the spec-correct shape for e.g. AllergyIntolerance.criticality,
// a `code` type) or, due to an upstream field-mapping datatype mismatch, as a
// CodeableConcept-shaped object instead. Dedicated builders in this package use
// this instead of a bare string-type-assertion wherever a field's real-world
// value has been observed to sometimes arrive mistyped — so the narrative still
// shows something meaningful instead of silently dropping the value.
func anyText(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ccText(v)
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
