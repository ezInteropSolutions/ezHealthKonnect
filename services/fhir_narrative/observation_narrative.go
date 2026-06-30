// services/fhir_narrative/observation_narrative.go
// Generates XHTML narrative for a FHIR R4 Observation resource.
// Labels are selected by Observation.category (USCDI v3 classes: Vital
// Signs, Laboratory, Social History, plus the US Core "functional-status"/
// "cognitive-status" categories used by the CDA->FHIR declarative engine).

package fhirnarrative

import "fmt"

// observationNarrativeLabels holds the per-category field headings used by
// GenerateObservationNarrative -- one switch, not a chain of isVital/isLab
// booleans, so adding a 6th category later is a single new case, not a new
// boolean threaded through every branch.
type observationNarrativeLabels struct {
	sectionHeading string
	testLabel      string
	valueLabel     string
	dateLabel      string
	// showLabExtras gates the Result Unit/Reference Range/Result Status
	// rows -- concepts that only make sense for an actual lab test, not a
	// vital sign reading or a non-quantitative finding (social history,
	// functional/cognitive status). Previously gated on "!isVital", which
	// silently mislabeled three other categories as "Laboratory Test" and
	// would have shown lab-specific rows for them too had any of them ever
	// carried a referenceRange/valueQuantity. Real gap found auditing
	// Mental Status: a "Cognitive function" entry's narrative rendered
	// "Laboratory Test" with no lab anywhere in that section.
	showLabExtras bool
}

func observationNarrativeLabelsForCategory(categoryCode string) observationNarrativeLabels {
	switch categoryCode {
	case "vital-signs":
		return observationNarrativeLabels{
			sectionHeading: "Vital Sign", testLabel: "Vital Sign",
			valueLabel: "Measurement Value", dateLabel: "Measurement Date",
		}
	case "laboratory":
		return observationNarrativeLabels{
			sectionHeading: "Laboratory Test", testLabel: "Laboratory Test",
			valueLabel: "Result Value", dateLabel: "Date Performed", showLabExtras: true,
		}
	case "social-history":
		return observationNarrativeLabels{
			sectionHeading: "Social History", testLabel: "Social History Finding",
			valueLabel: "Finding", dateLabel: "Date Recorded",
		}
	case "functional-status":
		return observationNarrativeLabels{
			sectionHeading: "Functional Status", testLabel: "Functional Status Finding",
			valueLabel: "Finding", dateLabel: "Date Recorded",
		}
	case "cognitive-status":
		return observationNarrativeLabels{
			sectionHeading: "Cognitive Status", testLabel: "Cognitive Status Finding",
			valueLabel: "Finding", dateLabel: "Date Recorded",
		}
	default:
		// Unknown/future category -- a generic, honest label instead of
		// guessing "Laboratory Test" for something that may not be a lab
		// result at all.
		return observationNarrativeLabels{
			sectionHeading: "Observation", testLabel: "Observation",
			valueLabel: "Value", dateLabel: "Date Recorded",
		}
	}
}

// GenerateObservationNarrative produces FHIR-compliant XHTML narrative for an
// Observation resource. Field headings are selected by Observation.category.
func GenerateObservationNarrative(r map[string]interface{}) string {
	labels := observationNarrativeLabelsForCategory(observationCategoryCode(r))

	rows := tableRow(labels.testLabel, ccText(r["code"]))
	rows += tableRow("Status", fhirStr(r, "status"))
	rows += tableRow(labels.dateLabel, fhirStr(r, "effectiveDateTime"))

	// Value — try valueQuantity first, then valueString, then valueCodeableConcept.
	if vq := fhirMap(r, "valueQuantity"); vq != nil {
		val := fhirStr(vq, "value")
		unit := fhirStr(vq, "unit")
		if val != "" {
			display := val
			if unit != "" {
				display = fmt.Sprintf("%s %s", val, unit)
			}
			rows += tableRow(labels.valueLabel, display)
			if labels.showLabExtras {
				rows += tableRow("Result Unit", unit)
			}
		}
	} else if vs := fhirStr(r, "valueString"); vs != "" {
		rows += tableRow(labels.valueLabel, vs)
	} else if vcc := ccText(r["valueCodeableConcept"]); vcc != "" {
		rows += tableRow(labels.valueLabel, vcc)
	}

	// Reference range + duplicate Result Status row (labs only).
	if labels.showLabExtras {
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

	return wrapDiv(heading(labels.sectionHeading) + buildTable(rows))
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
