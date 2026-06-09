// services/fhir_narrative/medication_narrative.go
// Generates XHTML narrative for a FHIR R4 MedicationStatement resource.
// USCDI v3 class: Medications.

package fhirnarrative

import "fmt"

// GenerateMedicationNarrative produces FHIR-compliant XHTML narrative for a
// MedicationStatement resource. USCDI v3 labels are used for all field headings.
func GenerateMedicationNarrative(r map[string]interface{}) string {
	rows := tableRow("Medication", ccText(r["medicationCodeableConcept"]))
	rows += tableRow("Status", fhirStr(r, "status"))

	// Effective period (start / end dates)
	if period := fhirMap(r, "effectivePeriod"); period != nil {
		rows += tableRow("Start Date", fhirStr(period, "start"))
		rows += tableRow("End Date", fhirStr(period, "end"))
	}

	// Dosage (first entry — covers the primary administration instruction)
	if dosages := fhirArr(r, "dosage"); len(dosages) > 0 {
		if d, ok := dosages[0].(map[string]interface{}); ok {
			rows += tableRow("Route", ccText(d["route"]))

			// Dose quantity nested under doseAndRate[0].doseQuantity
			if dar := fhirArr(d, "doseAndRate"); len(dar) > 0 {
				if darEntry, ok := dar[0].(map[string]interface{}); ok {
					if dq := fhirMap(darEntry, "doseQuantity"); dq != nil {
						val := fhirStr(dq, "value")
						unit := fhirStr(dq, "unit")
						if val != "" {
							dose := val
							if unit != "" {
								dose = fmt.Sprintf("%s %s", val, unit)
							}
							rows += tableRow("Dose", dose)
						}
					}
				}
			}
		}
	}

	return wrapDiv(heading("Medication") + buildTable(rows))
}
