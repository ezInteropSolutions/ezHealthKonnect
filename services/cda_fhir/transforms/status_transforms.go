package transforms

import "strings"

// ============================================================
// Clinical status transforms
// ============================================================

// AllergyStatusToFHIR converts a CDA allergy act statusCode to a FHIR ClinicalStatus CodeableConcept.
// AllergyIntolerance.clinicalStatus must come from the allergyintolerance-clinical
// CodeSystem — distinct from Condition's condition-clinical CodeSystem — so this
// builds its own CodeableConcept rather than sharing clinicalStatusCC().
func AllergyStatusToFHIR(statusCode string) map[string]interface{} {
	switch normStatus(statusCode) {
	case "55561003", "active":
		return allergyClinicalStatusCC("active", "Active")
	case "73425007", "inactive":
		return allergyClinicalStatusCC("inactive", "Inactive")
	case "413322009", "resolved":
		return allergyClinicalStatusCC("resolved", "Resolved")
	default:
		return allergyClinicalStatusCC("active", "Active")
	}
}

// ConditionStatusToFHIR converts a CDA problem statusCode to a FHIR ClinicalStatus CodeableConcept.
func ConditionStatusToFHIR(statusCode string) map[string]interface{} {
	switch normStatus(statusCode) {
	case "55561003", "active":
		return clinicalStatusCC("active", "Active")
	case "73425007", "inactive":
		return clinicalStatusCC("inactive", "Inactive")
	case "413322009", "resolved":
		return clinicalStatusCC("resolved", "Resolved")
	case "remission":
		return clinicalStatusCC("remission", "Remission")
	default:
		return clinicalStatusCC("active", "Active")
	}
}

// EncounterStatusToFHIR converts a CDA encounter statusCode to a FHIR Encounter.status code.
func EncounterStatusToFHIR(statusCode string) string {
	switch normStatus(statusCode) {
	case "completed":
		return "finished"
	case "active":
		return "in-progress"
	case "cancelled", "canceled", "aborted":
		return "cancelled"
	default:
		return "unknown"
	}
}

// ProcedureStatusToFHIR converts a CDA procedure statusCode to a FHIR Procedure.status code.
func ProcedureStatusToFHIR(statusCode string) string {
	switch normStatus(statusCode) {
	case "completed":
		return "completed"
	case "active":
		return "in-progress"
	case "aborted", "cancelled":
		return "not-done"
	case "held", "suspended":
		return "on-hold"
	default:
		return "unknown"
	}
}

// MedicationRequestStatusToFHIR converts a CDA medication statusCode (moodCode=INT) to
// a FHIR MedicationRequest.status code.
func MedicationRequestStatusToFHIR(statusCode string) string {
	switch normStatus(statusCode) {
	case "active":
		return "active"
	case "completed":
		return "completed"
	case "aborted", "cancelled":
		return "cancelled"
	case "held", "suspended":
		return "on-hold"
	default:
		return "unknown"
	}
}

// MedicationStatusToFHIR converts a CDA medication statusCode to a FHIR MedicationStatement.status code.
func MedicationStatusToFHIR(statusCode string) string {
	switch normStatus(statusCode) {
	case "active":
		return "active"
	case "completed":
		return "completed"
	case "aborted", "cancelled":
		return "stopped"
	case "held", "suspended":
		return "on-hold"
	default:
		return "unknown"
	}
}

// ObservationStatusToFHIR converts a CDA observation statusCode to a FHIR Observation.status code.
func ObservationStatusToFHIR(statusCode string) string {
	switch normStatus(statusCode) {
	case "completed":
		return "final"
	case "active":
		return "preliminary"
	case "aborted", "cancelled":
		return "cancelled"
	case "held":
		return "registered"
	default:
		return "final"
	}
}

// ImmunizationStatusToFHIR converts a CDA substanceAdministration statusCode to a FHIR Immunization.status.
func ImmunizationStatusToFHIR(statusCode string) string {
	switch normStatus(statusCode) {
	case "completed", "active":
		return "completed"
	case "aborted", "cancelled", "refused":
		return "not-done"
	default:
		return "completed"
	}
}

// CarePlanStatusToFHIR converts a CDA care plan act statusCode to a FHIR CarePlan.status code.
func CarePlanStatusToFHIR(statusCode string) string {
	switch normStatus(statusCode) {
	case "active":
		return "active"
	case "completed":
		return "completed"
	case "cancelled", "aborted":
		return "revoked"
	case "held", "suspended":
		return "on-hold"
	default:
		return "active"
	}
}

// GoalStatusToFHIR converts a CDA goal act statusCode to a FHIR Goal.lifecycleStatus code.
func GoalStatusToFHIR(statusCode string) string {
	switch normStatus(statusCode) {
	case "active":
		return "active"
	case "completed":
		return "completed"
	case "cancelled", "aborted":
		return "cancelled"
	case "held", "suspended":
		return "on-hold"
	default:
		return "active"
	}
}

// ============================================================
// Plan of Care / Care Team transforms (Planned Procedure, Planned Observation,
// Planned Act, Planned Encounter, Planned Supply, Care Team Organizer)
// ============================================================

// ServiceRequestIntentFromMood converts a CDA Act moodCode on a Plan of Care
// entry (Planned Procedure/Observation/Act) to a FHIR ServiceRequest.intent
// code. Per the C-CDA on FHIR IG, moodCode is what distinguishes a "planned"
// entry from an "event" one within the same template.
func ServiceRequestIntentFromMood(moodCode string) string {
	switch strings.ToUpper(strings.TrimSpace(moodCode)) {
	case "PRP":
		return "proposal"
	case "ARQ", "RQO":
		return "order"
	case "INT":
		return "plan"
	default:
		return "plan"
	}
}

// ServiceRequestStatusToFHIR converts a CDA Plan of Care entry statusCode to a
// FHIR ServiceRequest.status code.
func ServiceRequestStatusToFHIR(statusCode string) string {
	switch normStatus(statusCode) {
	case "active":
		return "active"
	case "completed":
		return "completed"
	case "cancelled", "aborted":
		return "revoked"
	case "held", "suspended":
		return "on-hold"
	default:
		return "active"
	}
}

// AppointmentStatusToFHIR converts a CDA Planned Encounter statusCode to a
// FHIR Appointment.status code.
func AppointmentStatusToFHIR(statusCode string) string {
	switch normStatus(statusCode) {
	case "active":
		return "booked"
	case "completed":
		return "fulfilled"
	case "cancelled", "aborted":
		return "cancelled"
	case "held", "suspended":
		return "pending"
	default:
		return "proposed"
	}
}

// SupplyRequestStatusToFHIR converts a CDA Planned Supply statusCode to a
// FHIR SupplyRequest.status code.
func SupplyRequestStatusToFHIR(statusCode string) string {
	switch normStatus(statusCode) {
	case "active":
		return "active"
	case "completed":
		return "completed"
	case "cancelled", "aborted":
		return "cancelled"
	case "held", "suspended":
		return "suspended"
	default:
		return "active"
	}
}

// CareTeamStatusToFHIR converts a CDA Care Team Organizer statusCode to a
// FHIR CareTeam.status code.
func CareTeamStatusToFHIR(statusCode string) string {
	switch normStatus(statusCode) {
	case "active":
		return "active"
	case "completed":
		return "inactive"
	case "cancelled", "aborted":
		return "entered-in-error"
	case "held", "suspended":
		return "suspended"
	default:
		return "active"
	}
}

// ============================================================
// Type/category helpers
// ============================================================

// AllergyTypeToFHIR maps a SNOMED allergy type observation value to a FHIR type code.
func AllergyTypeToFHIR(snomedCode string) string {
	switch snomedCode {
	case "416098002", "59037007": // Drug allergy / Drug hypersensitivity
		return "allergy"
	case "235719002", "420134006": // Food intolerance / Propensity to adverse reactions
		return "intolerance"
	default:
		return "allergy"
	}
}

// AllergyCategoryFromSubstanceSystem returns FHIR category based on the substance code system.
func AllergyCategoryFromSubstanceSystem(substanceCodeSystem string) []interface{} {
	norm := normalizeSystem(substanceCodeSystem)
	switch norm {
	case "http://www.nlm.nih.gov/research/umls/rxnorm",
		"http://hl7.org/fhir/sid/ndc":
		return []interface{}{"medication"}
	case "http://snomed.info/sct",
		"http://ncithesaurus.nci.nih.gov":
		return []interface{}{"environment"}
	default:
		return []interface{}{"medication"}
	}
}

// GenderToFHIR converts a CDA administrativeGenderCode to a FHIR gender string.
func GenderToFHIR(code string) string {
	switch strings.ToUpper(strings.TrimSpace(code)) {
	case "M", "MALE":
		return "male"
	case "F", "FEMALE":
		return "female"
	case "UN", "UNK", "UNKNOWN":
		return "unknown"
	case "O", "OTHER":
		return "other"
	default:
		return "unknown"
	}
}

// ============================================================
// Identifier system helpers (shared with patient mapper)
// ============================================================

// IdentifierSystem maps a well-known CDA OID root to a FHIR system URI.
func IdentifierSystem(root string) string {
	switch root {
	case "2.16.840.1.113883.4.1":
		return "http://hl7.org/fhir/sid/us-ssn"
	case "2.16.840.1.113883.4.6":
		return "http://hl7.org/fhir/sid/us-npi"
	case "2.16.840.1.113883.4.572":
		return "http://hl7.org/fhir/sid/us-medicare"
	case "2.16.840.1.113883.4.927":
		return "http://hl7.org/fhir/sid/us-mbi"
	case "2.16.840.1.113883.4.3":
		return "http://hl7.org/fhir/sid/us-dl"
	default:
		if root != "" {
			return "urn:oid:" + root
		}
		return ""
	}
}

// IdentifierTypeCode maps a well-known OID root to an HL7 v2 table 0203 type code.
// Facility-specific OID subtrees (2.16.840.1.113883.3.*) are treated as MRNs.
func IdentifierTypeCode(root string) string {
	switch root {
	case "2.16.840.1.113883.4.1":
		return "SS"
	case "2.16.840.1.113883.4.6":
		return "NPI"
	case "2.16.840.1.113883.4.572":
		return "MC"
	case "2.16.840.1.113883.4.927":
		return "MA"
	case "2.16.840.1.113883.4.3":
		return "DL"
	default:
		if strings.HasPrefix(root, "2.16.840.1.113883.3.") {
			return "MR"
		}
		return "PI"
	}
}

// ============================================================
// private helpers
// ============================================================

func clinicalStatusCC(code, display string) map[string]interface{} {
	return map[string]interface{}{
		"coding": []interface{}{
			map[string]interface{}{
				"system":  "http://terminology.hl7.org/CodeSystem/condition-clinical",
				"code":    code,
				"display": display,
			},
		},
		"text": display,
	}
}

// allergyClinicalStatusCC builds an AllergyIntolerance.clinicalStatus
// CodeableConcept from the allergyintolerance-clinical CodeSystem (NOT
// condition-clinical — that value set only applies to Condition resources).
func allergyClinicalStatusCC(code, display string) map[string]interface{} {
	return map[string]interface{}{
		"coding": []interface{}{
			map[string]interface{}{
				"system":  "http://terminology.hl7.org/CodeSystem/allergyintolerance-clinical",
				"code":    code,
				"display": display,
			},
		},
		"text": display,
	}
}

// normStatus lower-cases and trims a status code for switch comparison.
func normStatus(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
