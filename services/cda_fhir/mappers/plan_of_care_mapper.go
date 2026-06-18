// services/cda_fhir/mappers/plan_of_care_mapper.go
//
// Per the C-CDA on FHIR IG, a Plan of Care / Plan of Treatment section's
// "interventions" are NOT a single FHIR resource type — each entry maps to
// whichever resource its own template + moodCode calls for:
//
//	Planned Procedure   (procedure)             -> ServiceRequest
//	Planned Observation (observation)           -> ServiceRequest
//	Planned Act         (act)                   -> ServiceRequest
//	Planned Encounter   (encounter)              -> Appointment
//	Planned Supply       (supply)                 -> SupplyRequest
//	Planned Medication  (substanceAdministration) -> MedicationRequest
//	                                                  (reuses the exact same
//	                                                  builder as the Medications
//	                                                  section — same template,
//	                                                  same target resource)
//	Goal observation     (moodCode == "GOL")       -> Goal
//	                                                  (reuses the exact same
//	                                                  builder as the Goals section)
//
// Event-mood ("EVN") entries are skipped: something that already happened
// belongs in Procedures/Encounters/Results, not the plan/intervention list.
//
// This same dispatcher serves every section-title variant of "Plan of Care"
// seen across C-CDA documents (Plan of Treatment, Plan of Care, Assessment
// and Plan, Care Plan) — see the sectionKey aliases wired in document_mapper.go.
package mappers

import (
	"fmt"

	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/services/cda_fhir/transforms"
)

// MapPlanOfCare converts typed CDA Plan of Care section entries to FHIR
// resources, dispatching per-entry as described above.
func MapPlanOfCare(entries []cdadocument.CDAEntry, patientRef string) []map[string]interface{} {
	var resources []map[string]interface{}
	for _, entry := range flattenPlanOfCareEntries(entries) {
		r := dispatchPlanOfCareEntry(len(resources)+1, entry, patientRef)
		if r != nil && len(r) > 2 {
			resources = append(resources, r)
		}
	}
	return resources
}

// flattenPlanOfCareEntries expands organizer components (a Plan of Care
// section can group several planned activities under one organizer) into the
// top-level slice so each is dispatched individually.
func flattenPlanOfCareEntries(entries []cdadocument.CDAEntry) []cdadocument.CDAEntry {
	var flat []cdadocument.CDAEntry
	for _, e := range entries {
		if e.EntryType == "organizer" && len(e.Components) > 0 {
			flat = append(flat, e.Components...)
		} else {
			flat = append(flat, e)
		}
	}
	return flat
}

func dispatchPlanOfCareEntry(idx int, entry cdadocument.CDAEntry, patientRef string) map[string]interface{} {
	if entry.MoodCode == "EVN" {
		return nil // already happened — not a plan/intervention entry
	}
	if entry.MoodCode == "GOL" {
		return buildGoalResource(idx, entry, patientRef)
	}
	switch entry.EntryType {
	case "substanceAdministration":
		return buildMedicationRequestResource(idx, entry, patientRef)
	case "encounter":
		return buildPlannedAppointmentResource(idx, entry, patientRef)
	case "supply":
		return buildSupplyRequestResource(idx, entry, patientRef)
	case "procedure", "observation", "act":
		return buildServiceRequestResource(idx, entry, patientRef)
	default:
		return nil
	}
}

// buildServiceRequestResource maps a Planned Procedure/Observation/Act entry
// to a FHIR ServiceRequest.
func buildServiceRequestResource(idx int, entry cdadocument.CDAEntry, patientRef string) map[string]interface{} {
	r := map[string]interface{}{
		"resourceType": "ServiceRequest",
		"id":           fmt.Sprintf("serviceRequest-%d", idx),
		"status":       transforms.ServiceRequestStatusToFHIR(entry.StatusCode),
		"intent":       transforms.ServiceRequestIntentFromMood(entry.MoodCode),
	}
	if patientRef != "" {
		r["subject"] = ref(patientRef)
	}
	if cc := transforms.CDACodeToCodeableConcept(entry.Code); cc != nil {
		r["code"] = cc
	} else {
		// us-core-servicerequest requires code (1..1). When the CDA entry carries a
		// nullFlavor code (EHR didn't provide a coded procedure), emit a text-only
		// CodeableConcept so the resource is at least structurally valid.
		r["code"] = map[string]interface{}{"text": "Unknown"}
	}
	if period := transforms.CDATimeRangeToPeriod(entry.EffectiveTime); period != nil {
		r["occurrencePeriod"] = period
	} else if dt := transforms.CDATimeRangeToOnset(entry.EffectiveTime); dt != "" {
		r["occurrenceDateTime"] = dt
	}

	// Reason from the RSON entryRelationship (the clinical justification for
	// the planned act — e.g. "Diagnosis" linking a planned Pap test to its indication).
	for _, rel := range entry.EntryRelationships {
		if rel.TypeCode != "RSON" {
			continue
		}
		if cc := transforms.CDACodeToCodeableConcept(rel.Entry.Code); cc != nil {
			r["reasonCode"] = []interface{}{cc}
		}
		break
	}

	return r
}

// buildPlannedAppointmentResource maps a Planned Encounter entry to a FHIR Appointment.
func buildPlannedAppointmentResource(idx int, entry cdadocument.CDAEntry, patientRef string) map[string]interface{} {
	r := map[string]interface{}{
		"resourceType": "Appointment",
		"id":           fmt.Sprintf("appointment-%d", idx),
		"status":       transforms.AppointmentStatusToFHIR(entry.StatusCode),
	}
	if cc := transforms.CDACodeToCodeableConcept(entry.Code); cc != nil {
		r["serviceType"] = []interface{}{cc}
	}
	if period := transforms.CDATimeRangeToPeriod(entry.EffectiveTime); period != nil {
		if start, ok := period["start"]; ok {
			r["start"] = start
		}
		if end, ok := period["end"]; ok {
			r["end"] = end
		}
	}
	if patientRef != "" {
		r["participant"] = []interface{}{
			map[string]interface{}{
				"actor":  ref(patientRef),
				"status": "accepted",
			},
		}
	}
	return r
}

// buildSupplyRequestResource maps a Planned Supply entry to a FHIR SupplyRequest.
// CDA's Planned Supply template doesn't currently have a typed quantity field
// on CDAEntry (only substanceAdministration's DoseQuantity does); since
// SupplyRequest.quantity is required, we default to 1 when the source doesn't
// carry one rather than emit an invalid resource.
func buildSupplyRequestResource(idx int, entry cdadocument.CDAEntry, patientRef string) map[string]interface{} {
	r := map[string]interface{}{
		"resourceType": "SupplyRequest",
		"id":           fmt.Sprintf("supplyRequest-%d", idx),
		"status":       transforms.SupplyRequestStatusToFHIR(entry.StatusCode),
		"quantity":     map[string]interface{}{"value": 1},
	}
	if cc := transforms.CDACodeToCodeableConcept(entry.Code); cc != nil {
		r["itemCodeableConcept"] = cc
	}
	if patientRef != "" {
		r["deliverFor"] = ref(patientRef)
	}
	if dt := transforms.CDATimeRangeToOnset(entry.EffectiveTime); dt != "" {
		r["authoredOn"] = dt
	}
	return r
}
