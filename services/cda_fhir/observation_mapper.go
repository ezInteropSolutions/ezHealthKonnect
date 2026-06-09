// services/cda_fhir/observation_mapper.go
// Maps CDA vitalSigns and results section entries → FHIR R4 Observation resources.
// Category is set to "vital-signs" or "laboratory" based on the source section.

package cdafhir

import "fmt"

// ObservationMapper converts CDA vital signs and lab result entries to FHIR Observation
// resources. Call MapVitals for vitalSigns section entries and MapResults for results.
type ObservationMapper struct{}

// MapVitals converts vitalSigns section entries to FHIR Observation resources.
func (m *ObservationMapper) MapVitals(entries []map[string]interface{}, patientRef string) []map[string]interface{} {
	return m.mapEntries(entries, patientRef, "vital-signs", "Vital Signs", "vital")
}

// MapResults converts results (laboratory) section entries to FHIR Observation resources.
func (m *ObservationMapper) MapResults(entries []map[string]interface{}, patientRef string) []map[string]interface{} {
	return m.mapEntries(entries, patientRef, "laboratory", "Laboratory", "lab")
}

func (m *ObservationMapper) mapEntries(
	entries []map[string]interface{},
	patientRef string,
	categoryCode, categoryDisplay, idPrefix string,
) []map[string]interface{} {
	var resources []map[string]interface{}

	for i, entry := range entries {
		resource := m.mapEntry(entry, i, patientRef, categoryCode, categoryDisplay, idPrefix)
		if resource != nil {
			resources = append(resources, resource)
		}
	}

	return resources
}

func (m *ObservationMapper) mapEntry(
	entry map[string]interface{},
	idx int,
	patientRef, categoryCode, categoryDisplay, idPrefix string,
) map[string]interface{} {
	// Vitals use vitalCode; results use testCode
	code := strField(entry, "vitalCode")
	display := strField(entry, "vitalDisplay")
	if code == "" {
		code = strField(entry, "testCode")
		display = strField(entry, "testDisplay")
	}
	system := strField(entry, "testSystem")
	if system == "" {
		system = "LOINC"
	}

	if code == "" && display == "" {
		return nil
	}

	resource := map[string]interface{}{
		"resourceType": "Observation",
		"id":           fmt.Sprintf("%s-%d", idPrefix, idx+1),
		"status":       mapObsStatus(strField(entry, "resultStatus")),
		"category": []interface{}{
			NewCodeableConcept(categoryCode, categoryDisplay,
				"http://terminology.hl7.org/CodeSystem/observation-category"),
		},
		"code":    NewCodeableConcept(code, display, system),
		"subject": map[string]interface{}{"reference": patientRef},
	}

	// Effective time
	if et := strField(entry, "effectiveTime"); et != "" {
		resource["effectiveDateTime"] = FormatDate(et)
	}

	// Value — vitals use "value"/"unit"; labs use "resultValue"/"resultUnit"
	numVal := strField(entry, "value")
	numUnit := strField(entry, "unit")
	if numVal == "" {
		numVal = strField(entry, "resultValue")
		numUnit = strField(entry, "resultUnit")
	}

	if numVal != "" {
		resource["valueQuantity"] = map[string]interface{}{
			"value": numVal,
			"unit":  numUnit,
			"system": "http://unitsofmeasure.org",
		}
	} else if coded := strField(entry, "resultCode"); coded != "" {
		display := strField(entry, "resultDisplay")
		resource["valueCodeableConcept"] = NewCodeableConcept(coded, display, "SNOMED CT")
	} else if text := strField(entry, "resultText"); text != "" {
		resource["valueString"] = text
	} else if vd := strField(entry, "valueDisplay"); vd != "" {
		resource["valueString"] = vd
	}

	// Reference range (labs)
	low := strField(entry, "referenceRangeLow")
	high := strField(entry, "referenceRangeHigh")
	if low != "" || high != "" {
		rr := map[string]interface{}{}
		if low != "" {
			rr["low"] = map[string]interface{}{"value": low}
		}
		if high != "" {
			rr["high"] = map[string]interface{}{"value": high}
		}
		resource["referenceRange"] = []interface{}{rr}
	}

	return resource
}

// mapObsStatus maps a CDA statusCode to a FHIR observation status.
func mapObsStatus(cda string) string {
	switch cda {
	case "completed", "active":
		return "final"
	case "aborted":
		return "cancelled"
	case "new", "":
		return "final"
	default:
		return "final"
	}
}
