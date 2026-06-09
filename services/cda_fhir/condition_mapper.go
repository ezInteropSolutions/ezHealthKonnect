// services/cda_fhir/condition_mapper.go
// Maps CDA problems section entries → FHIR R4 Condition resources.

package cdafhir

import "fmt"

// ConditionMapper converts CDA problem entries to FHIR Condition resources.
type ConditionMapper struct{}

// Map converts a slice of problem entry maps into FHIR Condition resources.
func (m *ConditionMapper) Map(entries []map[string]interface{}, patientRef string) []map[string]interface{} {
	var resources []map[string]interface{}

	for i, entry := range entries {
		resource := m.mapEntry(entry, i, patientRef)
		if resource != nil {
			resources = append(resources, resource)
		}
	}

	return resources
}

func (m *ConditionMapper) mapEntry(entry map[string]interface{}, idx int, patientRef string) map[string]interface{} {
	code := strField(entry, "conditionCode")
	display := strField(entry, "conditionDisplay")
	system := strField(entry, "conditionSystem")

	if code == "" && display == "" {
		return nil
	}

	resource := map[string]interface{}{
		"resourceType": "Condition",
		"id":           fmt.Sprintf("condition-%d", idx+1),
		"subject":      map[string]interface{}{"reference": patientRef},
		"code":         NewCodeableConcept(code, display, system),
	}

	// Clinical status
	resource["clinicalStatus"] = mapConditionStatus(strField(entry, "status"))

	// Verification status — confirmed for CDA sourced data
	resource["verificationStatus"] = NewCodeableConcept(
		"confirmed", "Confirmed",
		"http://terminology.hl7.org/CodeSystem/condition-ver-status",
	)

	// Category — problem-list-item
	resource["category"] = []interface{}{
		NewCodeableConcept("problem-list-item", "Problem List Item",
			"http://terminology.hl7.org/CodeSystem/condition-category"),
	}

	// Onset and resolution dates
	if onset := strField(entry, "onsetDate"); onset != "" {
		resource["onsetDateTime"] = FormatDate(onset)
	}
	if resolution := strField(entry, "resolutionDate"); resolution != "" {
		resource["abatementDateTime"] = FormatDate(resolution)
	}

	return resource
}

// mapConditionStatus converts a CDA problem status display to FHIR clinical status.
func mapConditionStatus(cda string) map[string]interface{} {
	code := "active"
	display := "Active"

	switch cda {
	case "Resolved", "resolved", "Inactive", "inactive":
		code = "resolved"
		display = "Resolved"
	case "Remission", "remission", "In Remission":
		code = "remission"
		display = "Remission"
	}

	return NewCodeableConcept(code, display,
		"http://terminology.hl7.org/CodeSystem/condition-clinical")
}
