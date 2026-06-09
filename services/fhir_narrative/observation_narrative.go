// services/fhir_narrative/observation_narrative.go
// Generates XHTML narrative for a FHIR R4 Observation resource.
// Detects vital-signs vs laboratory category and applies the correct USCDI v3 labels.
// USCDI v3 classes: Vital Signs · Laboratory (Tests, Values, Result Status).

package fhirnarrative

import "fmt"

// GenerateObservationNarrative produces FHIR-compliant XHTML narrative for an
// Observation resource. USCDI v3 labels are used for all field headings.
func GenerateObservationNarrative(r map[string]interface{}) string {
	// Determine whether this is a vital sign or a lab result.
	categoryCode := observationCategoryCode(r)
	isVital := categoryCode == "vital-signs"

	var sectionHeading, testLabel, valueLabel, dateLabel string
	if isVital {
		sectionHeading = "Vital Sign"
		testLabel = "Vital Sign"
		valueLabel = "Measurement Value"
		dateLabel = "Measurement Date"
	} else {
		sectionHeading = "Laboratory Test"
		testLabel = "Laboratory Test"
		valueLabel = "Result Value"
		dateLabel = "Date Performed"
	}

	rows := tableRow(testLabel, ccText(r["code"]))
	rows += tableRow("Status", fhirStr(r, "status"))
	rows += tableRow(dateLabel, fhirStr(r, "effectiveDateTime"))

	// Value — try valueQuantity first, then valueString, then valueCodeableConcept.
	if vq := fhirMap(r, "valueQuantity"); vq != nil {
		val := fhirStr(vq, "value")
		unit := fhirStr(vq, "unit")
		if val != "" {
			display := val
			if unit != "" {
				display = fmt.Sprintf("%s %s", val, unit)
			}
			rows += tableRow(valueLabel, display)
			if !isVital {
				rows += tableRow("Result Unit", unit)
			}
		}
	} else if vs := fhirStr(r, "valueString"); vs != "" {
		rows += tableRow(valueLabel, vs)
	} else if vcc := ccText(r["valueCodeableConcept"]); vcc != "" {
		rows += tableRow(valueLabel, vcc)
	}

	// Reference range (labs only)
	if !isVital {
		if rrs := fhirArr(r, "referenceRange"); len(rrs) > 0 {
			if rr, ok := rrs[0].(map[string]interface{}); ok {
				low := ""
				high := ""
				if lowMap := fhirMap(rr, "low"); lowMap != nil {
					low = fhirStr(lowMap, "value")
				}
				if highMap := fhirMap(rr, "high"); highMap != nil {
					high = fhirStr(highMap, "value")
				}
				if low != "" || high != "" {
					refRange := low + " – " + high
					if low == "" {
						refRange = "< " + high
					} else if high == "" {
						refRange = "> " + low
					}
					rows += tableRow("Reference Range", refRange)
				}
			}
		}
		rows += tableRow("Result Status", fhirStr(r, "status"))
	}

	return wrapDiv(heading(sectionHeading) + buildTable(rows))
}

// observationCategoryCode extracts the first category coding code from an
// Observation resource (e.g. "vital-signs" or "laboratory").
func observationCategoryCode(r map[string]interface{}) string {
	cats := fhirArr(r, "category")
	for _, raw := range cats {
		cat, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		code := ccCode(cat)
		if code != "" {
			return code
		}
	}
	return ""
}
