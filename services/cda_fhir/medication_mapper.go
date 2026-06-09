// services/cda_fhir/medication_mapper.go
// Maps CDA medications section entries → FHIR R4 MedicationStatement resources.

package cdafhir

import "fmt"

// MedicationMapper converts CDA medication entries to FHIR MedicationStatement resources.
type MedicationMapper struct{}

// Map converts a slice of medication entry maps into FHIR MedicationStatement resources.
func (m *MedicationMapper) Map(entries []map[string]interface{}, patientRef string) []map[string]interface{} {
	var resources []map[string]interface{}

	for i, entry := range entries {
		resource := m.mapEntry(entry, i, patientRef)
		if resource != nil {
			resources = append(resources, resource)
		}
	}

	return resources
}

func (m *MedicationMapper) mapEntry(entry map[string]interface{}, idx int, patientRef string) map[string]interface{} {
	code := strField(entry, "drugCode")
	display := strField(entry, "drugDisplay")
	system := strField(entry, "drugSystem")

	if code == "" && display == "" {
		return nil
	}

	resource := map[string]interface{}{
		"resourceType":          "MedicationStatement",
		"id":                    fmt.Sprintf("medstmt-%d", idx+1),
		"subject":               map[string]interface{}{"reference": patientRef},
		"medicationCodeableConcept": NewCodeableConcept(code, display, system),
	}

	// Status
	resource["status"] = mapMedStatus(strField(entry, "status"))

	// Effective period
	start := strField(entry, "startDate")
	stop := strField(entry, "stopDate")
	if start != "" || stop != "" {
		period := map[string]interface{}{}
		if start != "" {
			period["start"] = FormatDate(start)
		}
		if stop != "" {
			period["end"] = FormatDate(stop)
		}
		resource["effectivePeriod"] = period
	}

	// Dosage
	doseVal := strField(entry, "doseValue")
	doseUnit := strField(entry, "doseUnit")
	routeDisplay := strField(entry, "route")
	routeCode := strField(entry, "routeCode")

	if doseVal != "" || routeDisplay != "" {
		dosage := map[string]interface{}{}
		if routeDisplay != "" || routeCode != "" {
			dosage["route"] = NewCodeableConcept(routeCode, routeDisplay, "SNOMED CT")
		}
		if doseVal != "" {
			dosage["doseAndRate"] = []interface{}{
				map[string]interface{}{
					"doseQuantity": map[string]interface{}{
						"value": doseVal,
						"unit":  doseUnit,
					},
				},
			}
		}
		resource["dosage"] = []interface{}{dosage}
	}

	return resource
}

// mapMedStatus maps CDA statusCode to FHIR MedicationStatement status.
func mapMedStatus(cda string) string {
	switch cda {
	case "active":
		return "active"
	case "completed":
		return "completed"
	case "cancelled", "aborted":
		return "stopped"
	case "suspended":
		return "on-hold"
	default:
		return "unknown"
	}
}
