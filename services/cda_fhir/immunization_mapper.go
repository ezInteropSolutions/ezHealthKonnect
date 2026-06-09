// services/cda_fhir/immunization_mapper.go
// Maps CDA immunizations section entries → FHIR R4 Immunization resources.

package cdafhir

import "fmt"

// ImmunizationMapper converts CDA immunization entries to FHIR Immunization resources.
type ImmunizationMapper struct{}

// Map converts a slice of immunization entry maps into FHIR Immunization resources.
func (m *ImmunizationMapper) Map(entries []map[string]interface{}, patientRef string) []map[string]interface{} {
	var resources []map[string]interface{}

	for i, entry := range entries {
		resource := m.mapEntry(entry, i, patientRef)
		if resource != nil {
			resources = append(resources, resource)
		}
	}

	return resources
}

func (m *ImmunizationMapper) mapEntry(entry map[string]interface{}, idx int, patientRef string) map[string]interface{} {
	code := strField(entry, "vaccineCode")
	display := strField(entry, "vaccineDisplay")
	system := strField(entry, "vaccineSystem")

	if code == "" && display == "" {
		return nil
	}

	resource := map[string]interface{}{
		"resourceType":  "Immunization",
		"id":            fmt.Sprintf("immunization-%d", idx+1),
		"patient":       map[string]interface{}{"reference": patientRef},
		"vaccineCode":   NewCodeableConcept(code, display, system),
		"status":        mapImmunizationStatus(strField(entry, "status")),
		"primarySource": true,
	}

	// Administration date
	if date := strField(entry, "administrationDate"); date != "" {
		resource["occurrenceDateTime"] = FormatDate(date)
	}

	// Lot number
	if lot := strField(entry, "lotNumber"); lot != "" {
		resource["lotNumber"] = lot
	}

	return resource
}

// mapImmunizationStatus converts a CDA statusCode to a FHIR immunization status.
func mapImmunizationStatus(cda string) string {
	switch cda {
	case "completed", "active", "":
		return "completed"
	case "aborted", "nullified":
		return "not-done"
	default:
		return "completed"
	}
}
