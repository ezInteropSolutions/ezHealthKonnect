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
// Verified against HL7's official C-CDA-on-FHIR ConceptMap (ConceptMap/CF-ProcedureStatus,
// https://github.com/HL7/ccda-on-fhir/blob/master/input/maps/ConceptMap-CF-ProcedureStatus.xml):
// aborted->stopped, active->in-progress, cancelled->not-done, completed->completed.
// "aborted" and "cancelled" are NOT synonyms here even though both come from CDA's single
// v3-ActStatus value set -- the IG maps them to two different Procedure.status codes.
// held/suspended has no official mapping; on-hold is a reasonable, undocumented default.
func ProcedureStatusToFHIR(statusCode string) string {
	switch normStatus(statusCode) {
	case "completed":
		return "completed"
	case "active":
		return "in-progress"
	case "aborted":
		return "stopped"
	case "cancelled":
		return "not-done"
	case "held", "suspended":
		return "on-hold"
	default:
		return "unknown"
	}
}

// MedicationRequestStatusToFHIR converts a CDA medication statusCode (moodCode="INT") to
// a FHIR MedicationRequest.status code. Verified against HL7's official C-CDA-on-FHIR
// ConceptMap (ConceptMap/CF-MedicationStatus, targetUri = ValueSet/medicationrequest-status,
// https://github.com/HL7/ccda-on-fhir/blob/master/input/maps/ConceptMap-CF-MedicationStatus.xml):
// active->active, suspended->on-hold, aborted->stopped, completed->completed,
// nullified->entered-in-error. "cancelled" has no entry in the official table; mapped here
// to FHIR's own "cancelled" code by direct name correspondence (a valid, distinct code in
// MedicationRequest's value set, unlike MedicationStatement's -- see MedicationStatusToFHIR).
func MedicationRequestStatusToFHIR(statusCode string) string {
	switch normStatus(statusCode) {
	case "active":
		return "active"
	case "completed":
		return "completed"
	case "aborted":
		return "stopped"
	case "cancelled":
		return "cancelled"
	case "nullified":
		return "entered-in-error"
	case "held", "suspended":
		return "on-hold"
	default:
		return "unknown"
	}
}

// MedicationStatusToFHIR converts a CDA medication statusCode (moodCode != "INT", i.e. a
// historical/administered medication) to a FHIR MedicationStatement.status code.
//
// UNLIKE MedicationRequestStatusToFHIR above, HL7's C-CDA-on-FHIR IG explicitly has NO
// consensus mapping for this direction: "For mapping histories of medication use from
// CDA, no consensus was established. We welcome feedback on this topic." (CF-medications.md,
// https://github.com/HL7/ccda-on-fhir/blob/master/input/pagecontent/CF-medications.md).
// There is no official ConceptMap to verify against here -- this mirrors the officially-
// specified Request-side source codes as closely as MedicationStatement's own value set
// allows. MedicationStatement.status has no "cancelled" code at all (valid set: active,
// completed, entered-in-error, intended, stopped, on-hold, unknown, not-taken — confirmed
// against http://hl7.org/fhir/R4/valueset-medication-statement-status.html), so CDA
// "cancelled" maps to "stopped" here, same bucket as "aborted" — not literally "cancelled"
// as it does on the Request side.
func MedicationStatusToFHIR(statusCode string) string {
	switch normStatus(statusCode) {
	case "active":
		return "active"
	case "completed":
		return "completed"
	case "aborted", "cancelled":
		return "stopped"
	case "nullified":
		return "entered-in-error"
	case "held", "suspended":
		return "on-hold"
	default:
		return "unknown"
	}
}

// MedicationAdministrationStatusToFHIR converts a CDA substanceAdministration statusCode to a
// FHIR MedicationAdministration.status code. Valid set: in-progress, not-done, on-hold,
// completed, entered-in-error, stopped, unknown (http://hl7.org/fhir/R4/valueset-medication-admin-status.html)
// -- a materially different value set from MedicationStatusToFHIR/MedicationRequestStatusToFHIR
// above (e.g. "active" -> "in-progress" not "active"; "cancelled" -> "not-done" not "stopped"),
// so this is its own switch rather than a shared helper.
func MedicationAdministrationStatusToFHIR(statusCode string) string {
	switch normStatus(statusCode) {
	case "active":
		return "in-progress"
	case "completed":
		return "completed"
	case "aborted":
		return "stopped"
	case "cancelled":
		return "not-done"
	case "nullified":
		return "entered-in-error"
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
// negationInd reflects the entry's @negationInd attribute (C-CDA R2.1 Immunization Activity,
// CONF:1198-8985 — SHALL [1..1]): when true the vaccine was not administered (refused/not given)
// regardless of what statusCode says about the documentation act itself, so it takes priority
// over the statusCode-based mapping below.
func ImmunizationStatusToFHIR(statusCode string, negationInd bool) string {
	if negationInd {
		return "not-done"
	}
	switch normStatus(statusCode) {
	case "completed", "active":
		return "completed"
	case "aborted", "cancelled":
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

// DeviceUseStatementStatusToFHIR converts a CDA Non-Medicinal Supply Activity
// statusCode to a FHIR DeviceUseStatement.status code. CONF:1098-8758 fixes
// this element 1..1, never nullFlavor -- an unrecognized value coerces to
// "active" rather than erroring (device_mapper.go's original deviceStatus()).
func DeviceUseStatementStatusToFHIR(statusCode string) string {
	switch statusCode {
	case "active":
		return "active"
	case "completed":
		return "completed"
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

// AllergyReactionSeverityToFHIR maps a SNOMED reaction-severity observation
// value (Severity Observation V2, templateId 2.16.840.1.113883.10.20.22.4.8,
// CONF:1098-19169 fixes the observation's own code to "SEV") to the FIXED
// mild/moderate/severe enum AllergyIntolerance.reaction[].severity requires
// — unlike Condition.severity, which is a full CodeableConcept (see
// ConditionStatusToFHIR's siblings; that one stays a CodeableConcept on
// purpose). Extracted from allergy_mapper.go's private allergySeverityCode
// (zero behavior change, including the known, already-documented gap: SNOMED
// 255604002 "Very Mild" is unmapped and falls through to the "moderate"
// default — confirmed present in real Kareo corpus data per
// architecture/CDA_FHIR_MAPPING_INVENTORY.md's cross-cutting finding #8;
// fixing that needs its own IG verification, not bundled into this move).
func AllergyReactionSeverityToFHIR(snomedCode string) string {
	switch snomedCode {
	case "371924009", "Mild":
		return "mild"
	case "6736007", "Moderate":
		return "moderate"
	case "24484000", "Severe":
		return "severe"
	default:
		return "moderate"
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

// isFixedIdentifierSystem reports whether root is one of the well-known national
// identifier system OIDs handled explicitly by IdentifierSystem (NPI, SSN, Medicare,
// MBI, DL) — i.e. root is always the system's own fixed OID, never a patient/provider-
// specific identifier value. Used by IIToIdentifier to decide whether falling back to
// root as the Identifier.value (when @extension is absent) is meaningful or garbage.
func isFixedIdentifierSystem(root string) bool {
	sys := IdentifierSystem(root)
	return sys != "" && sys != "urn:oid:"+root
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
