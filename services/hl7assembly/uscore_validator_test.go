// services/hl7assembly/uscore_validator_test.go
//
// Two test layers:
//
//  1. Validator unit tests — hand-craft FHIR resources and assert the rule
//     checker catches (or correctly ignores) specific violations.
//
//  2. Assembly integration tests — build minimal HL7 segment data, run it
//     through the hl7assembly functions, then assert the output passes all
//     US Core SHALL rules.  No database or running server required.
package hl7assembly

import (
	"fmt"
	"strings"
	"testing"

	"ezhealthkonnect/hl7"
)

// ─── Segment / field builder helpers ─────────────────────────────────────────

// field builds a minimal hl7.FieldInfo with the given key and value.
func field(key, value string) hl7.FieldInfo {
	return hl7.FieldInfo{Key: key, Value: value, HasValue: value != ""}
}

// subfield builds a hl7.SubfieldInfo for component-level values.
func subfield(key, value string) hl7.SubfieldInfo {
	return hl7.SubfieldInfo{Key: key, Value: value, HasValue: value != ""}
}

// fieldWithSubs builds a FieldInfo that carries named subfields.
func fieldWithSubs(key, value string, subs ...hl7.SubfieldInfo) hl7.FieldInfo {
	return hl7.FieldInfo{Key: key, Value: value, HasValue: value != "", Subfields: subs}
}

// seg builds a minimal hl7.EnhancedSegment.
func seg(key string, fields ...hl7.FieldInfo) hl7.EnhancedSegment {
	return hl7.EnhancedSegment{Key: key, Fields: fields}
}

// segGroups wraps a map of segment name → slice into the parsedHL7Data map
// consumed by ExtractSegmentGroup.
func segGroups(m map[string][]hl7.EnhancedSegment, order ...string) map[string]interface{} {
	d := map[string]interface{}{
		"segmentGroups": m,
	}
	if len(order) > 0 {
		d["segmentOrder"] = order
	}
	return d
}

// ─── Validator unit tests: US Core Patient ────────────────────────────────────

func TestUSCorePatient_Valid(t *testing.T) {
	r := map[string]interface{}{
		"resourceType": "Patient",
		"id":           "p1",
		"name": []interface{}{
			map[string]interface{}{"family": "Smith", "given": []interface{}{"John"}},
		},
		"gender":    "male",
		"birthDate": "1980-01-15",
	}
	issues := ValidateUSCorePatient(r)
	if errs := ErrorIssues(issues); len(errs) != 0 {
		t.Fatalf("expected no errors for valid Patient, got: %v", errs)
	}
}

func TestUSCorePatient_MissingName(t *testing.T) {
	r := map[string]interface{}{
		"resourceType": "Patient",
		"gender":       "female",
	}
	issues := ValidateUSCorePatient(r)
	assertErrorPath(t, issues, "Patient.name")
}

func TestUSCorePatient_EmptyNameSlice(t *testing.T) {
	r := map[string]interface{}{
		"resourceType": "Patient",
		"name":         []interface{}{},
		"gender":       "female",
	}
	issues := ValidateUSCorePatient(r)
	assertErrorPath(t, issues, "Patient.name")
}

func TestUSCorePatient_MissingGender(t *testing.T) {
	r := map[string]interface{}{
		"resourceType": "Patient",
		"name": []interface{}{
			map[string]interface{}{"family": "Doe"},
		},
	}
	issues := ValidateUSCorePatient(r)
	assertErrorPath(t, issues, "Patient.gender")
}

// ─── Validator unit tests: US Core Observation Lab ───────────────────────────

func validObservation() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Observation",
		"id":           "obs-1",
		"status":       "final",
		"category": []interface{}{
			map[string]interface{}{
				"coding": []interface{}{
					map[string]interface{}{
						"system":  "http://terminology.hl7.org/CodeSystem/observation-category",
						"code":    "laboratory",
						"display": "Laboratory",
					},
				},
			},
		},
		"code": map[string]interface{}{
			"coding": []interface{}{
				map[string]interface{}{"system": "http://loinc.org", "code": "718-7", "display": "Hemoglobin"},
			},
			"text": "Hemoglobin",
		},
		"subject":         map[string]interface{}{"reference": "Patient/p1"},
		"valueQuantity":   map[string]interface{}{"value": 14.2, "unit": "g/dL"},
		"effectiveDateTime": "2024-01-01T12:00:00Z",
	}
}

func TestUSCoreObservationLab_Valid(t *testing.T) {
	issues := ValidateUSCoreObservationLab(validObservation())
	if errs := ErrorIssues(issues); len(errs) != 0 {
		t.Fatalf("expected no errors for valid Observation, got: %v", errs)
	}
}

func TestUSCoreObservationLab_MissingStatus(t *testing.T) {
	r := validObservation()
	delete(r, "status")
	assertErrorPath(t, ValidateUSCoreObservationLab(r), "Observation.status")
}

func TestUSCoreObservationLab_InvalidStatus(t *testing.T) {
	r := validObservation()
	r["status"] = "invalid-code"
	assertErrorPath(t, ValidateUSCoreObservationLab(r), "Observation.status")
}

func TestUSCoreObservationLab_MissingCategory(t *testing.T) {
	r := validObservation()
	delete(r, "category")
	assertErrorPath(t, ValidateUSCoreObservationLab(r), "Observation.category")
}

func TestUSCoreObservationLab_WrongCategorySystem(t *testing.T) {
	r := validObservation()
	r["category"] = []interface{}{
		map[string]interface{}{
			"coding": []interface{}{
				map[string]interface{}{
					"system": "http://wrong.system",
					"code":   "laboratory",
				},
			},
		},
	}
	assertErrorPath(t, ValidateUSCoreObservationLab(r), "Observation.category")
}

func TestUSCoreObservationLab_MissingCode(t *testing.T) {
	r := validObservation()
	delete(r, "code")
	assertErrorPath(t, ValidateUSCoreObservationLab(r), "Observation.code")
}

func TestUSCoreObservationLab_MissingSubject(t *testing.T) {
	r := validObservation()
	delete(r, "subject")
	assertErrorPath(t, ValidateUSCoreObservationLab(r), "Observation.subject")
}

func TestUSCoreObservationLab_MissingValue(t *testing.T) {
	r := validObservation()
	delete(r, "valueQuantity")
	assertErrorPath(t, ValidateUSCoreObservationLab(r), "Observation.value[x]")
}

func TestUSCoreObservationLab_DataAbsentReasonAccepted(t *testing.T) {
	r := validObservation()
	delete(r, "valueQuantity")
	r["dataAbsentReason"] = map[string]interface{}{
		"coding": []interface{}{
			map[string]interface{}{"system": "http://terminology.hl7.org/CodeSystem/data-absent-reason", "code": "unknown"},
		},
	}
	issues := ValidateUSCoreObservationLab(r)
	for _, i := range issues {
		if i.Path == "Observation.value[x]" && i.Severity == "error" {
			t.Fatalf("dataAbsentReason should satisfy value[x] requirement")
		}
	}
}

// ─── Validator unit tests: US Core DiagnosticReport Lab ───────────────────────

func validDiagnosticReport() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "DiagnosticReport",
		"id":           "dr-1",
		"status":       "final",
		"category": []interface{}{
			map[string]interface{}{
				"coding": []interface{}{
					map[string]interface{}{
						"system":  "http://terminology.hl7.org/CodeSystem/v2-0074",
						"code":    "LAB",
						"display": "Laboratory",
					},
				},
			},
		},
		"code":    map[string]interface{}{"text": "CBC"},
		"subject": map[string]interface{}{"reference": "Patient/p1"},
	}
}

func TestUSCoreDiagnosticReportLab_Valid(t *testing.T) {
	issues := ValidateUSCoreDiagnosticReportLab(validDiagnosticReport())
	if errs := ErrorIssues(issues); len(errs) != 0 {
		t.Fatalf("expected no errors for valid DiagnosticReport, got: %v", errs)
	}
}

func TestUSCoreDiagnosticReportLab_MissingStatus(t *testing.T) {
	r := validDiagnosticReport()
	delete(r, "status")
	assertErrorPath(t, ValidateUSCoreDiagnosticReportLab(r), "DiagnosticReport.status")
}

func TestUSCoreDiagnosticReportLab_WrongCategory(t *testing.T) {
	r := validDiagnosticReport()
	r["category"] = []interface{}{
		map[string]interface{}{
			"coding": []interface{}{
				map[string]interface{}{"system": "http://terminology.hl7.org/CodeSystem/v2-0074", "code": "RAD"},
			},
		},
	}
	assertErrorPath(t, ValidateUSCoreDiagnosticReportLab(r), "DiagnosticReport.category")
}

func TestUSCoreDiagnosticReportLab_MissingCode(t *testing.T) {
	r := validDiagnosticReport()
	delete(r, "code")
	assertErrorPath(t, ValidateUSCoreDiagnosticReportLab(r), "DiagnosticReport.code")
}

func TestUSCoreDiagnosticReportLab_MissingSubject(t *testing.T) {
	r := validDiagnosticReport()
	delete(r, "subject")
	assertErrorPath(t, ValidateUSCoreDiagnosticReportLab(r), "DiagnosticReport.subject")
}

// ─── Validator unit tests: US Core Immunization ───────────────────────────────

func validImmunization() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Immunization",
		"id":           "imm-1",
		"status":       "completed",
		"vaccineCode": map[string]interface{}{
			"coding": []interface{}{
				map[string]interface{}{"system": "http://hl7.org/fhir/sid/cvx", "code": "08"},
			},
			"text": "Hepatitis B",
		},
		"patient":             map[string]interface{}{"reference": "Patient/p1"},
		"occurrenceDateTime":  "2024-06-01",
	}
}

func TestUSCoreImmunization_Valid(t *testing.T) {
	issues := ValidateUSCoreImmunization(validImmunization())
	if errs := ErrorIssues(issues); len(errs) != 0 {
		t.Fatalf("expected no errors for valid Immunization, got: %v", errs)
	}
}

func TestUSCoreImmunization_InvalidStatus(t *testing.T) {
	r := validImmunization()
	r["status"] = "active"
	assertErrorPath(t, ValidateUSCoreImmunization(r), "Immunization.status")
}

func TestUSCoreImmunization_MissingVaccineCode(t *testing.T) {
	r := validImmunization()
	delete(r, "vaccineCode")
	assertErrorPath(t, ValidateUSCoreImmunization(r), "Immunization.vaccineCode")
}

func TestUSCoreImmunization_MissingPatient(t *testing.T) {
	r := validImmunization()
	delete(r, "patient")
	assertErrorPath(t, ValidateUSCoreImmunization(r), "Immunization.patient")
}

func TestUSCoreImmunization_MissingOccurrence(t *testing.T) {
	r := validImmunization()
	delete(r, "occurrenceDateTime")
	assertErrorPath(t, ValidateUSCoreImmunization(r), "Immunization.occurrence[x]")
}

func TestUSCoreImmunization_OccurrenceStringAccepted(t *testing.T) {
	r := validImmunization()
	delete(r, "occurrenceDateTime")
	r["occurrenceString"] = "approximately June 2024"
	issues := ValidateUSCoreImmunization(r)
	for _, i := range issues {
		if i.Path == "Immunization.occurrence[x]" && i.Severity == "error" {
			t.Fatalf("occurrenceString should satisfy occurrence[x] requirement")
		}
	}
}

// ─── Validator unit tests: US Core DocumentReference ─────────────────────────

func validDocumentReference() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "DocumentReference",
		"id":           "docref-1",
		"status":       "current",
		"content": []interface{}{
			map[string]interface{}{
				"attachment": map[string]interface{}{
					"contentType": "text/plain",
					"data":        "VGVzdA==",
				},
			},
		},
	}
}

func TestUSCoreDocumentReference_Valid(t *testing.T) {
	issues := ValidateUSCoreDocumentReference(validDocumentReference())
	if errs := ErrorIssues(issues); len(errs) != 0 {
		t.Fatalf("expected no errors for valid DocumentReference, got: %v", errs)
	}
}

func TestUSCoreDocumentReference_InvalidStatus(t *testing.T) {
	r := validDocumentReference()
	r["status"] = "active"
	assertErrorPath(t, ValidateUSCoreDocumentReference(r), "DocumentReference.status")
}

func TestUSCoreDocumentReference_MissingContent(t *testing.T) {
	r := validDocumentReference()
	delete(r, "content")
	assertErrorPath(t, ValidateUSCoreDocumentReference(r), "DocumentReference.content")
}

func TestUSCoreDocumentReference_EmptyContent(t *testing.T) {
	r := validDocumentReference()
	r["content"] = []interface{}{}
	assertErrorPath(t, ValidateUSCoreDocumentReference(r), "DocumentReference.content")
}

// ─── Assembly integration test: ORU^R01 ──────────────────────────────────────
//
// Builds minimal HL7 segment data, runs it through AssembleORUObservations,
// and asserts every Observation and DiagnosticReport in the output passes
// US Core SHALL checks.

func TestORUAssemblyUSCoreConformance(t *testing.T) {
	patient := map[string]interface{}{
		"resourceType": "Patient",
		"id":           "patient-1",
		"name":         []interface{}{map[string]interface{}{"family": "GARCIA", "given": []interface{}{"MARIA"}}},
		"gender":       "female",
	}
	dr := map[string]interface{}{
		"resourceType": "DiagnosticReport",
		"id":           "dr-1",
	}

	hl7Data := segGroups(
		map[string][]hl7.EnhancedSegment{
			"MSH": {
				seg("MSH", field("MSH.4", "HOSP")),
			},
			"OBR": {
				seg("OBR",
					field("OBR.4", "85022^CBC^L"),
					field("OBR.7", "20240101120000"),
					field("OBR.25", "F"),
				),
			},
			"OBX": {
				seg("OBX",
					field("OBX.1", "1"),
					field("OBX.2", "NM"),
					field("OBX.3", "718-7^Hemoglobin^LN"),
					field("OBX.5", "14.2"),
					field("OBX.6", "g/dL"),
					field("OBX.11", "F"),
					field("OBX.14", "20240101120000"),
				),
				seg("OBX",
					field("OBX.1", "2"),
					field("OBX.2", "NM"),
					field("OBX.3", "789-8^Erythrocytes^LN"),
					field("OBX.5", "4.5"),
					field("OBX.6", "10*6/uL"),
					field("OBX.11", "F"),
					field("OBX.14", "20240101120000"),
				),
			},
		},
		"MSH", "OBR", "OBX", "OBX",
	)

	resources, warnings := AssembleORUObservations(hl7Data, []map[string]interface{}{patient, dr})
	t.Logf("assembly warnings: %v", warnings)

	issues := ValidateUSCoreBundle(resources)
	reportAndFail(t, issues)
}

// TestORUAssembly_NoSubject verifies that an Observation produced without a
// Patient in the resource list correctly triggers the US Core subject error.
// This guards against regressions where the validator is accidentally bypassed.
func TestORUAssembly_MissingPatientFlagged(t *testing.T) {
	dr := map[string]interface{}{"resourceType": "DiagnosticReport", "id": "dr-1"}

	hl7Data := segGroups(
		map[string][]hl7.EnhancedSegment{
			"MSH": {seg("MSH", field("MSH.4", "HOSP"))},
			"OBR": {seg("OBR",
				field("OBR.4", "85022^CBC^L"),
				field("OBR.25", "F"),
			)},
			"OBX": {seg("OBX",
				field("OBX.1", "1"),
				field("OBX.2", "NM"),
				field("OBX.3", "718-7^Hemoglobin^LN"),
				field("OBX.5", "14.2"),
				field("OBX.11", "F"),
			)},
		},
		"MSH", "OBR", "OBX",
	)

	resources, _ := AssembleORUObservations(hl7Data, []map[string]interface{}{dr})
	issues := ValidateUSCoreBundle(resources)
	found := false
	for _, i := range issues {
		if i.Path == "Observation.subject" && i.Severity == "error" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected Observation.subject error when no Patient in resources, but none was reported")
	}
}

// ─── Assembly integration test: VXU^V04 ──────────────────────────────────────

func TestVXUAssemblyUSCoreConformance(t *testing.T) {
	patient := map[string]interface{}{
		"resourceType": "Patient",
		"id":           "patient-1",
		"name":         []interface{}{map[string]interface{}{"family": "JONES"}},
		"gender":       "unknown",
	}

	hl7Data := segGroups(
		map[string][]hl7.EnhancedSegment{
			"MSH": {seg("MSH", field("MSH.4", "CLINIC"))},
			"RXA": {
				seg("RXA",
					field("RXA.1", "1"),
					field("RXA.3", "20240601100000"),
					field("RXA.5", "08^Hepatitis B^CVX"),
					field("RXA.6", "1"),
					field("RXA.7", "mL"),
					field("RXA.20", "CP"),
				),
			},
		},
		"MSH", "RXA",
	)

	resources, warnings := AssembleVXUImmunizations(hl7Data, []map[string]interface{}{patient})
	t.Logf("assembly warnings: %v", warnings)

	issues := ValidateUSCoreBundle(resources)
	reportAndFail(t, issues)
}

// ─── Assembly integration test: MDM^T02 ──────────────────────────────────────

func TestMDMAssemblyUSCoreConformance(t *testing.T) {
	patient := map[string]interface{}{
		"resourceType": "Patient",
		"id":           "patient-1",
		"name":         []interface{}{map[string]interface{}{"family": "BROWN"}},
		"gender":       "male",
	}

	hl7Data := segGroups(
		map[string][]hl7.EnhancedSegment{
			"MSH": {seg("MSH", field("MSH.4", "HOSP"))},
			"TXA": {
				seg("TXA",
					field("TXA.17", "LA"),
					field("TXA.19", "AV"),
				),
			},
			"OBX": {
				seg("OBX",
					field("OBX.1", "1"),
					field("OBX.2", "TX"),
					field("OBX.5", "Patient presents with chest pain."),
				),
			},
		},
		"MSH", "TXA", "OBX",
	)

	resources, warnings := AssembleMDMDocument(hl7Data, []map[string]interface{}{patient})
	t.Logf("assembly warnings: %v", warnings)

	issues := ValidateUSCoreBundle(resources)
	reportAndFail(t, issues)
}

// ─── Bundle-level dispatcher test ────────────────────────────────────────────

func TestValidateUSCoreBundle_SkipsUnknownResourceTypes(t *testing.T) {
	resources := []map[string]interface{}{
		{"resourceType": "Bundle", "id": "b1"},
		{"resourceType": "MessageHeader", "id": "mh1"},
		{"resourceType": "Practitioner", "id": "prac1"},
	}
	issues := ValidateUSCoreBundle(resources)
	if len(issues) != 0 {
		t.Fatalf("expected no issues for resource types without a US Core validator, got: %v", issues)
	}
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// assertErrorPath fails the test when no error-severity issue with the given
// path is present in issues.
func assertErrorPath(t *testing.T, issues []ValidationIssue, path string) {
	t.Helper()
	for _, i := range issues {
		if i.Path == path && i.Severity == "error" {
			return
		}
	}
	t.Errorf("expected an error issue at path %q, but none was found.\nAll issues: %s", path, formatIssues(issues))
}

// reportAndFail logs all validation issues and fails if any have severity
// "error". Warnings are logged but do not fail the test.
func reportAndFail(t *testing.T, issues []ValidationIssue) {
	t.Helper()
	errs := ErrorIssues(issues)
	if len(issues) > 0 {
		t.Logf("validation issues:\n%s", formatIssues(issues))
	}
	if len(errs) > 0 {
		t.Errorf("%d US Core error(s) found — see issues above", len(errs))
	}
}

// formatIssues renders a ValidationIssue slice as a multi-line string for test output.
func formatIssues(issues []ValidationIssue) string {
	var sb strings.Builder
	for _, i := range issues {
		sb.WriteString(fmt.Sprintf("  [%s] %s @ %s: %s\n", i.Severity, i.Profile, i.Path, i.Message))
	}
	return sb.String()
}
