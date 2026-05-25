package services

import "fmt"

// getString is a nil-safe helper to extract a string from a map.
func getString(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return v
}

// getMap is a nil-safe helper to extract a map from a map.
func getMap(m map[string]interface{}, key string) map[string]interface{} {
	v, _ := m[key].(map[string]interface{})
	return v
}

// Helper function to get keys from map
func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// Helper function to get map keys for debugging
func getMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// wireConditionsToEncounterDiagnosis wires Condition resources into
// Encounter.diagnosis[].condition references.
//
// The OOB template creates an Encounter.diagnosis slot with a null condition
// because at mapping time the Condition's urn:uuid is not yet known.  This
// post-normalizer pass:
//  1. Collects all Condition IDs and builds "Condition/<id>" relative references.
//     rewriteReferences() in bundle assembly will convert these to urn:uuid: form.
//  2. Finds every Encounter with a diagnosis array.
//  3. For each null-condition slot it injects the next available Condition reference.
//  4. If the Encounter has no diagnosis array yet, injects one slot per Condition.
//  5. Removes any slots that still have no condition reference.
func (s *HL7FHIRTransformServiceV3) wireConditionsToEncounterDiagnosis(
	resources []map[string]interface{},
) []map[string]interface{} {
	// Collect Condition ids in document order (first = principal/admitting diagnosis).
	var conditionRefs []string
	for _, r := range resources {
		if rt, _ := r["resourceType"].(string); rt != "Condition" {
			continue
		}
		if id, _ := r["id"].(string); id != "" {
			conditionRefs = append(conditionRefs, "Condition/"+id)
		}
	}
	if len(conditionRefs) == 0 {
		return resources
	}

	for _, r := range resources {
		if rt, _ := r["resourceType"].(string); rt != "Encounter" {
			continue
		}
		diags, hasDiags := r["diagnosis"].([]interface{})
		if !hasDiags || len(diags) == 0 {
			// No slots yet — inject one per Condition.
			var newDiags []interface{}
			for _, ref := range conditionRefs {
				newDiags = append(newDiags, map[string]interface{}{
					"condition": map[string]interface{}{
						"reference": ref,
					},
				})
			}
			r["diagnosis"] = newDiags
			continue
		}

		// Wire existing null slots in order; keep slots that are already wired.
		condIdx := 0
		var kept []interface{}
		for _, dRaw := range diags {
			d, ok2 := dRaw.(map[string]interface{})
			if !ok2 {
				continue
			}
			if cond := d["condition"]; cond != nil {
				kept = append(kept, d)
				condIdx++
				continue
			}
			if condIdx < len(conditionRefs) {
				d["condition"] = map[string]interface{}{
					"reference": conditionRefs[condIdx],
				}
				kept = append(kept, d)
				condIdx++
			}
			// Slots with no available Condition are discarded.
		}
		if len(kept) == 0 {
			delete(r, "diagnosis")
		} else {
			r["diagnosis"] = kept
		}
	}
	return resources
}

// wireSubjectReferences is a universal post-normalizer pass that wires the
// Patient reference into every resource type that requires it according to
// FHIR R4.  The OOB template engine assembles resources independently; cross-
// resource references such as subject/patient/beneficiary cannot be filled in
// at mapping time because the Patient's urn:uuid is not yet known.
//
// Rules (never overwrites an existing non-empty reference):
//   - subject → Patient:  Encounter, Condition, Observation, DiagnosticReport,
//     ChargeItem, AllergyIntolerance, DocumentReference, ServiceRequest,
//     MedicationRequest, MedicationAdministration, Procedure, ImagingStudy,
//     CarePlan, CareTeam, RiskAssessment, DetectedIssue, Goal, QuestionnaireResponse
//   - patient → Patient:  Immunization, MedicationDispense, MedicationStatement
//   - beneficiary → Patient: Coverage
//   - subject[0] → Patient: Account (Account.subject is a list)
func (s *HL7FHIRTransformServiceV3) wireSubjectReferences(
	resources []map[string]interface{},
) []map[string]interface{} {
	// Find the Patient resource id.
	patientID := ""
	for _, r := range resources {
		if rt, _ := r["resourceType"].(string); rt == "Patient" {
			if id, _ := r["id"].(string); id != "" {
				patientID = id
				break
			}
		}
	}
	if patientID == "" {
		return resources // no Patient in bundle → nothing to wire
	}
	patientRef := map[string]interface{}{
		"reference": "Patient/" + patientID,
	}

	// resourceType → field name for "subject"-style single reference
	subjectFields := map[string]string{
		"Encounter":              "subject",
		"Condition":              "subject",
		"Observation":            "subject",
		"DiagnosticReport":       "subject",
		"ChargeItem":             "subject",
		"AllergyIntolerance":     "patient",
		"DocumentReference":      "subject",
		"ServiceRequest":         "subject",
		"MedicationRequest":      "subject",
		"MedicationAdministration": "subject",
		"Procedure":              "subject",
		"ImagingStudy":           "subject",
		"CarePlan":               "subject",
		"CareTeam":               "subject",
		"RiskAssessment":         "subject",
		"DetectedIssue":          "subject",
		"Goal":                   "subject",
		"QuestionnaireResponse":  "subject",
		"Immunization":           "patient",
		"MedicationDispense":     "subject",
		"MedicationStatement":    "subject",
		"Coverage":               "beneficiary",
	}

	for _, r := range resources {
		rt, _ := r["resourceType"].(string)

		// Account.subject is a list (0..*) — inject Patient as first element if absent.
		if rt == "Account" {
			switch existing := r["subject"].(type) {
			case []interface{}:
				if len(existing) == 0 {
					r["subject"] = []interface{}{patientRef}
				}
			case nil:
				r["subject"] = []interface{}{patientRef}
			}
			continue
		}

		field, ok := subjectFields[rt]
		if !ok {
			continue
		}

		existing := r[field]
		if existing != nil {
			// Do not overwrite a reference that is already set.
			if m, isMap := existing.(map[string]interface{}); isMap {
				if ref, _ := m["reference"].(string); ref != "" {
					continue
				}
			}
		}
		r[field] = patientRef
	}
	return resources
}

// synthesizeMissingRequiredFields fills in FHIR R4 required fields that the
// template engine cannot populate at mapping time (cardinality 1..1 fields
// whose value depends on business rules rather than a direct HL7 source).
//
// Current rules:
//   - DiagnosticReport.status: set to "unknown" when absent (valid FHIR R4 code).
//   - ChargeItemDefinition.url: synthesize a canonical URI from the resource id
//     when absent (required 1..1 by FHIR R4).
//   - ChargeItemDefinition.status: set to "unknown" when absent.
func (s *HL7FHIRTransformServiceV3) synthesizeMissingRequiredFields(
	resources []map[string]interface{},
) []map[string]interface{} {
	for _, r := range resources {
		rt, _ := r["resourceType"].(string)

		switch rt {
		case "DiagnosticReport":
			if status, _ := r["status"].(string); status == "" {
				r["status"] = "unknown"
			}

		case "ChargeItemDefinition":
			if status, _ := r["status"].(string); status == "" {
				r["status"] = "unknown"
			}
			if url, _ := r["url"].(string); url == "" {
				id, _ := r["id"].(string)
				if id == "" {
					id = "unknown"
				}
				// Synthesize a deterministic canonical URI from the resource id.
				r["url"] = fmt.Sprintf("urn:chargeitemdefinition:%s", id)
			}
		}
	}
	return resources
}
