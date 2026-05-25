// services/hl7assembly/uscore_validator.go
//
// In-process US Core R4 profile conformance checker.
//
// Each Validate* function enforces only the SHALL (1..1, 1..*) constraints
// from the named US Core profile. Must-Support (MS) fields that are not SHALL
// are intentionally omitted — they would generate excessive noise for partially
// mapped messages during development.
//
// Profiles implemented:
//
//	USCorePatient              (6.1.0 / 3.1.1 compatible)
//	USCoreObservationLab       (6.1.0 / 3.1.1 compatible)
//	USCoreDiagnosticReportLab  (6.1.0 / 3.1.1 compatible)
//	USCoreImmunization         (6.1.0 / 3.1.1 compatible)
//	USCoreDocumentReference    (6.1.0 / 3.1.1 compatible)
//
// All validators accept generic map[string]interface{} resources — the same
// type produced by every hl7assembly function — so no additional marshalling
// is required.
package hl7assembly

import "strings"

// ─── Types ────────────────────────────────────────────────────────────────────

// ValidationIssue is one US Core profile conformance finding.
type ValidationIssue struct {
	Severity string // "error" | "warning"
	Profile  string // e.g. "USCorePatient"
	Path     string // JSON path, e.g. "Patient.name"
	Message  string
}

// ErrorIssues returns the subset of issues with Severity == "error".
func ErrorIssues(issues []ValidationIssue) []ValidationIssue {
	var errs []ValidationIssue
	for _, i := range issues {
		if i.Severity == "error" {
			errs = append(errs, i)
		}
	}
	return errs
}

// ─── Bundle dispatcher ────────────────────────────────────────────────────────

// ValidateUSCoreBundle validates every resource in the list against its US Core
// profile, dispatching by resourceType. Resources whose type has no registered
// validator are silently skipped (not flagged as errors).
func ValidateUSCoreBundle(resources []map[string]interface{}) []ValidationIssue {
	var issues []ValidationIssue
	for _, r := range resources {
		rt, _ := r["resourceType"].(string)
		switch rt {
		case "Patient":
			issues = append(issues, ValidateUSCorePatient(r)...)
		case "Observation":
			issues = append(issues, ValidateUSCoreObservationLab(r)...)
		case "DiagnosticReport":
			issues = append(issues, ValidateUSCoreDiagnosticReportLab(r)...)
		case "Immunization":
			issues = append(issues, ValidateUSCoreImmunization(r)...)
		case "DocumentReference":
			issues = append(issues, ValidateUSCoreDocumentReference(r)...)
		}
	}
	return issues
}

// ─── Internal helpers ─────────────────────────────────────────────────────────

func vi(severity, profile, path, message string) ValidationIssue {
	return ValidationIssue{Severity: severity, Profile: profile, Path: path, Message: message}
}

// hasString returns true when key is present, is a string, and is non-empty.
func hasString(r map[string]interface{}, key string) bool {
	v, ok := r[key]
	if !ok {
		return false
	}
	s, ok := v.(string)
	return ok && strings.TrimSpace(s) != ""
}

// hasKey returns true when the key exists in the map (value may be nil).
func hasKey(r map[string]interface{}, key string) bool {
	_, ok := r[key]
	return ok
}

// hasNonEmptySlice returns true when key is present and is a non-empty slice.
func hasNonEmptySlice(r map[string]interface{}, key string) bool {
	v, ok := r[key]
	if !ok {
		return false
	}
	switch s := v.(type) {
	case []interface{}:
		return len(s) > 0
	case []map[string]interface{}:
		return len(s) > 0
	}
	return false
}

// hasMap returns true when key is present and is a non-empty map.
func hasMap(r map[string]interface{}, key string) bool {
	v, ok := r[key]
	if !ok {
		return false
	}
	m, ok := v.(map[string]interface{})
	return ok && len(m) > 0
}

// hasValueX returns true when the resource has at least one FHIR value[x] choice.
var valueXKeys = []string{
	"valueQuantity", "valueCodeableConcept", "valueString", "valueBoolean",
	"valueInteger", "valueRange", "valueRatio", "valueSampledData",
	"valueTime", "valueDateTime", "valuePeriod", "valueAttachment",
}

func hasValueX(r map[string]interface{}) bool {
	for _, k := range valueXKeys {
		if _, ok := r[k]; ok {
			return true
		}
	}
	return false
}

// categoryHasCode returns true when the resource's category[] array contains at
// least one Coding with the given system and code.
func categoryHasCode(r map[string]interface{}, system, code string) bool {
	raw, ok := r["category"]
	if !ok {
		return false
	}
	cats, ok := raw.([]interface{})
	if !ok {
		return false
	}
	for _, c := range cats {
		cm, ok := c.(map[string]interface{})
		if !ok {
			continue
		}
		codings, ok := cm["coding"].([]interface{})
		if !ok {
			continue
		}
		for _, coding := range codings {
			cdm, ok := coding.(map[string]interface{})
			if !ok {
				continue
			}
			sys, _ := cdm["system"].(string)
			cod, _ := cdm["code"].(string)
			if sys == system && cod == code {
				return true
			}
		}
	}
	return false
}

// ─── US Core Patient ──────────────────────────────────────────────────────────

// ValidateUSCorePatient checks SHALL constraints from the US Core Patient profile.
//
// Rules:
//   - Patient.name  (1..*) — at least one name entry must be present
//   - Patient.gender (1..1) — must be present and non-empty
func ValidateUSCorePatient(r map[string]interface{}) []ValidationIssue {
	const profile = "USCorePatient"
	var issues []ValidationIssue

	if !hasNonEmptySlice(r, "name") {
		issues = append(issues, vi("error", profile, "Patient.name",
			"US Core Patient requires at least one name entry (1..*)"))
	}
	if !hasString(r, "gender") {
		issues = append(issues, vi("error", profile, "Patient.gender",
			"US Core Patient requires gender (1..1)"))
	}
	return issues
}

// ─── US Core Observation Lab ──────────────────────────────────────────────────

// ValidateUSCoreObservationLab checks SHALL constraints from the US Core
// Laboratory Result Observation profile.
//
// Rules:
//   - Observation.status   (1..1) — must be a valid FHIR observation-status code
//   - Observation.category (1..*) — must include the "laboratory" code from
//     http://terminology.hl7.org/CodeSystem/observation-category
//   - Observation.code     (1..1) — must be present
//   - Observation.subject  (1..1) — must be present (Patient reference)
//   - Observation.value[x] or Observation.dataAbsentReason — one must be present
func ValidateUSCoreObservationLab(r map[string]interface{}) []ValidationIssue {
	const profile = "USCoreObservationLab"
	var issues []ValidationIssue

	validStatuses := map[string]bool{
		"registered": true, "preliminary": true, "final": true,
		"amended": true, "corrected": true, "cancelled": true,
		"entered-in-error": true, "unknown": true,
	}
	status, _ := r["status"].(string)
	if status == "" {
		issues = append(issues, vi("error", profile, "Observation.status",
			"US Core Observation Lab requires status (1..1)"))
	} else if !validStatuses[status] {
		issues = append(issues, vi("error", profile, "Observation.status",
			"Observation.status '"+status+"' is not a valid FHIR observation-status code"))
	}

	const obsCatSystem = "http://terminology.hl7.org/CodeSystem/observation-category"
	if !categoryHasCode(r, obsCatSystem, "laboratory") {
		issues = append(issues, vi("error", profile, "Observation.category",
			"US Core Observation Lab requires category[].coding with system '"+obsCatSystem+"' code 'laboratory'"))
	}

	if !hasKey(r, "code") {
		issues = append(issues, vi("error", profile, "Observation.code",
			"US Core Observation Lab requires code (1..1)"))
	}

	if !hasMap(r, "subject") {
		issues = append(issues, vi("error", profile, "Observation.subject",
			"US Core Observation Lab requires subject (Patient reference) (1..1)"))
	}

	if !hasValueX(r) && !hasMap(r, "dataAbsentReason") {
		issues = append(issues, vi("error", profile, "Observation.value[x]",
			"US Core Observation Lab requires either value[x] or dataAbsentReason"))
	}

	return issues
}

// ─── US Core DiagnosticReport Lab ────────────────────────────────────────────

// ValidateUSCoreDiagnosticReportLab checks SHALL constraints from the US Core
// Laboratory DiagnosticReport profile.
//
// Rules:
//   - DiagnosticReport.status   (1..1) — must be present
//   - DiagnosticReport.category (1..*) — must include "LAB" from
//     http://terminology.hl7.org/CodeSystem/v2-0074
//   - DiagnosticReport.code     (1..1) — must be present
//   - DiagnosticReport.subject  (1..1) — must be present
func ValidateUSCoreDiagnosticReportLab(r map[string]interface{}) []ValidationIssue {
	const profile = "USCoreDiagnosticReportLab"
	var issues []ValidationIssue

	if !hasString(r, "status") {
		issues = append(issues, vi("error", profile, "DiagnosticReport.status",
			"US Core DiagnosticReport Lab requires status (1..1)"))
	}

	const drCatSystem = "http://terminology.hl7.org/CodeSystem/v2-0074"
	if !categoryHasCode(r, drCatSystem, "LAB") {
		issues = append(issues, vi("error", profile, "DiagnosticReport.category",
			"US Core DiagnosticReport Lab requires category[].coding with system '"+drCatSystem+"' code 'LAB'"))
	}

	if !hasKey(r, "code") {
		issues = append(issues, vi("error", profile, "DiagnosticReport.code",
			"US Core DiagnosticReport Lab requires code (1..1)"))
	}

	if !hasMap(r, "subject") {
		issues = append(issues, vi("error", profile, "DiagnosticReport.subject",
			"US Core DiagnosticReport Lab requires subject (1..1)"))
	}

	return issues
}

// ─── US Core Immunization ─────────────────────────────────────────────────────

// ValidateUSCoreImmunization checks SHALL constraints from the US Core
// Immunization profile.
//
// Rules:
//   - Immunization.status       (1..1) — completed | entered-in-error | not-done
//   - Immunization.vaccineCode  (1..1) — must be present
//   - Immunization.patient      (1..1) — must be present (Patient reference)
//   - Immunization.occurrence[x] (1..1) — occurrenceDateTime or occurrenceString
func ValidateUSCoreImmunization(r map[string]interface{}) []ValidationIssue {
	const profile = "USCoreImmunization"
	var issues []ValidationIssue

	validStatuses := map[string]bool{
		"completed": true, "entered-in-error": true, "not-done": true,
	}
	status, _ := r["status"].(string)
	if status == "" {
		issues = append(issues, vi("error", profile, "Immunization.status",
			"US Core Immunization requires status (1..1)"))
	} else if !validStatuses[status] {
		issues = append(issues, vi("error", profile, "Immunization.status",
			"Immunization.status '"+status+"' must be completed|entered-in-error|not-done"))
	}

	if !hasKey(r, "vaccineCode") {
		issues = append(issues, vi("error", profile, "Immunization.vaccineCode",
			"US Core Immunization requires vaccineCode (1..1)"))
	}

	if !hasMap(r, "patient") {
		issues = append(issues, vi("error", profile, "Immunization.patient",
			"US Core Immunization requires patient reference (1..1)"))
	}

	_, hasDT := r["occurrenceDateTime"]
	_, hasStr := r["occurrenceString"]
	if !hasDT && !hasStr {
		issues = append(issues, vi("error", profile, "Immunization.occurrence[x]",
			"US Core Immunization requires occurrenceDateTime or occurrenceString (1..1)"))
	}

	return issues
}

// ─── US Core DocumentReference ───────────────────────────────────────────────

// ValidateUSCoreDocumentReference checks SHALL constraints from the US Core
// DocumentReference profile.
//
// Rules:
//   - DocumentReference.status  (1..1) — current | superseded | entered-in-error
//   - DocumentReference.content (1..*) — at least one content entry with attachment
func ValidateUSCoreDocumentReference(r map[string]interface{}) []ValidationIssue {
	const profile = "USCoreDocumentReference"
	var issues []ValidationIssue

	validStatuses := map[string]bool{
		"current": true, "superseded": true, "entered-in-error": true,
	}
	status, _ := r["status"].(string)
	if status == "" {
		issues = append(issues, vi("error", profile, "DocumentReference.status",
			"US Core DocumentReference requires status (1..1)"))
	} else if !validStatuses[status] {
		issues = append(issues, vi("error", profile, "DocumentReference.status",
			"DocumentReference.status '"+status+"' must be current|superseded|entered-in-error"))
	}

	if !hasNonEmptySlice(r, "content") {
		issues = append(issues, vi("error", profile, "DocumentReference.content",
			"US Core DocumentReference requires at least one content entry (1..*)"))
	}

	return issues
}
