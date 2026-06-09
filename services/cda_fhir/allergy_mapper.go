// services/cda_fhir/allergy_mapper.go
// Maps CDA allergiesAndIntolerances section entries → FHIR R4 AllergyIntolerance resources.

package cdafhir

import "fmt"

// AllergyMapper converts CDA allergy entries to FHIR AllergyIntolerance resources.
type AllergyMapper struct{}

// Map converts a slice of allergy entry maps (from the allergiesAndIntolerances
// section processor) into FHIR AllergyIntolerance resources.
func (m *AllergyMapper) Map(entries []map[string]interface{}, patientRef string) []map[string]interface{} {
	var resources []map[string]interface{}

	for i, entry := range entries {
		resource := m.mapEntry(entry, i, patientRef)
		if resource != nil {
			resources = append(resources, resource)
		}
	}

	return resources
}

func (m *AllergyMapper) mapEntry(entry map[string]interface{}, idx int, patientRef string) map[string]interface{} {
	code := strField(entry, "medicationAllergyCode")
	display := strField(entry, "medicationAllergyDisplay")
	system := strField(entry, "medicationAllergySystem")

	if code == "" && display == "" {
		return nil
	}

	resource := map[string]interface{}{
		"resourceType": "AllergyIntolerance",
		"id":           fmt.Sprintf("allergy-%d", idx+1),
		"type":         "allergy",
		"category":     []interface{}{"medication"},
		"patient":      map[string]interface{}{"reference": patientRef},
	}

	// Substance (allergen)
	resource["code"] = NewCodeableConcept(code, display, system)

	// Clinical status
	resource["clinicalStatus"] = mapAllergyStatus(strField(entry, "status"))

	// Verification status — mark as confirmed for CDA sourced data
	resource["verificationStatus"] = NewCodeableConcept(
		"confirmed", "Confirmed",
		"http://terminology.hl7.org/CodeSystem/allergyintolerance-verification",
	)

	// Onset date
	if onset := strField(entry, "onsetDate"); onset != "" {
		resource["onsetDateTime"] = FormatDate(onset)
	}

	// Reaction
	reaction := strField(entry, "reaction")
	reactionCode := strField(entry, "reactionCode")
	if reaction != "" || reactionCode != "" {
		reactionEntry := map[string]interface{}{
			"manifestation": []interface{}{
				NewCodeableConcept(reactionCode, reaction, "SNOMED CT"),
			},
		}
		if sev := strField(entry, "severity"); sev != "" {
			reactionEntry["severity"] = mapSeverity(sev)
		}
		resource["reaction"] = []interface{}{reactionEntry}
	}

	return resource
}

// mapAllergyStatus converts a CDA status display name to a FHIR clinical status
// CodeableConcept.
func mapAllergyStatus(cdaStatus string) map[string]interface{} {
	code := "active"
	display := "Active"

	switch cdaStatus {
	case "Inactive", "inactive", "Resolved", "resolved":
		code = "inactive"
		display = "Inactive"
	case "Refuted", "refuted":
		code = "refuted"
		display = "Refuted"
	}

	return NewCodeableConcept(code, display,
		"http://terminology.hl7.org/CodeSystem/allergyintolerance-clinical")
}

// mapSeverity converts a CDA severity display to a FHIR severity code.
func mapSeverity(cda string) string {
	switch cda {
	case "Mild", "mild":
		return "mild"
	case "Moderate", "moderate":
		return "moderate"
	case "Severe", "severe", "Life threatening", "Fatal":
		return "severe"
	default:
		return "moderate"
	}
}
