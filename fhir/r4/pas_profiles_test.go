package r4_test

// ─────────────────────────────────────────────────────────────────────────────
// Da Vinci PAS FHIR Constraint Tests  (pas_profiles.go)
//
// TC-PASC-001..006  pas-claim-1  (Claim.use = preauthorization)
// TC-PASC-007..012  pas-claim-2  (Claim must have ≥1 item)
// TC-PASC-013..017  pas-claim-3  (Claim must reference Coverage via insurance)
// TC-PASC-018..022  pas-bundle-1 (PAS bundle must have exactly 1 Claim)
// TC-PASC-023..026  pas-bundle-2 (PAS bundle must have ≥1 Patient)
// TC-PASC-027..030  pas-bundle-3 (PAS bundle must have ≥1 Coverage)
// TC-PASC-031..034  Non-PAS resources are NOT affected by PAS constraints
//
// Run: go test ./fhir/r4/ -v -run TestPASC
// ─────────────────────────────────────────────────────────────────────────────

import (
	"ezhealthkonnect/fhir/r4"
	"testing"
)

// ─────────────────────────────────────────────────────────────────────────────
// Fixture helpers
// ─────────────────────────────────────────────────────────────────────────────

const pasClaimProfile    = "http://hl7.org/fhir/us/davinci-pas/StructureDefinition/profile-claim"
const pasBundleProfile   = "http://hl7.org/fhir/us/davinci-pas/StructureDefinition/profile-pas-request-bundle"
const pasCovProfile      = "http://hl7.org/fhir/us/davinci-pas/StructureDefinition/profile-coverage"

func pasClaimMeta(profiles ...string) map[string]interface{} {
	p := make([]interface{}, len(profiles))
	for i, s := range profiles { p[i] = s }
	return map[string]interface{}{"meta": map[string]interface{}{"profile": p}}
}

func minimalPASClaim(use string) map[string]interface{} {
	res := map[string]interface{}{
		"resourceType": "Claim",
		"use":          use,
		"insurance": []interface{}{
			map[string]interface{}{
				"sequence": 1,
				"focal":    true,
				"coverage": map[string]interface{}{"reference": "Coverage/coverage-1"},
			},
		},
		"item": []interface{}{
			map[string]interface{}{
				"sequence":           1,
				"productOrService":   map[string]interface{}{},
			},
		},
	}
	for k, v := range pasClaimMeta(pasClaimProfile) {
		res[k] = v
	}
	return res
}

func pasBundleWithEntries(entries ...map[string]interface{}) map[string]interface{} {
	rawEntries := make([]interface{}, len(entries))
	for i, e := range entries { rawEntries[i] = map[string]interface{}{"resource": e} }
	res := map[string]interface{}{
		"resourceType": "Bundle",
		"type":         "collection",
		"entry":        rawEntries,
	}
	for k, v := range pasClaimMeta(pasBundleProfile) {
		res[k] = v
	}
	return res
}

func patientResource()      map[string]interface{} { return map[string]interface{}{"resourceType": "Patient"} }
func coverageResource()     map[string]interface{} { return map[string]interface{}{"resourceType": "Coverage"} }
func practResource()        map[string]interface{} { return map[string]interface{}{"resourceType": "Practitioner"} }
func validPASClaimResource() map[string]interface{} { return minimalPASClaim("preauthorization") }

func checkConstraints(t *testing.T, resourceType string, res map[string]interface{}) []r4.ConstraintViolation {
	t.Helper()
	cr := r4.GetConstraintRegistry()
	return cr.Check(resourceType, res)
}

func assertNoViolation(t *testing.T, key string, violations []r4.ConstraintViolation) {
	t.Helper()
	for _, v := range violations {
		if v.Key == key {
			t.Errorf("expected no %s violation, but got: %s", key, v.Description)
		}
	}
}

func assertHasViolation(t *testing.T, key string, violations []r4.ConstraintViolation) {
	t.Helper()
	for _, v := range violations {
		if v.Key == key { return }
	}
	t.Errorf("expected %s violation but none found (violations: %v)", key, violations)
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-PASC-001..006  pas-claim-1  Claim.use = "preauthorization"
// ─────────────────────────────────────────────────────────────────────────────

// TC-PASC-001: Correct use=preauthorization passes
func TestPASC_Claim1_PreauthorizationPasses(t *testing.T) {
	violations := checkConstraints(t, "Claim", minimalPASClaim("preauthorization"))
	assertNoViolation(t, "pas-claim-1", violations)
}

// TC-PASC-002: use=claim on PAS-profiled resource fails
func TestPASC_Claim1_WrongUseFails(t *testing.T) {
	violations := checkConstraints(t, "Claim", minimalPASClaim("claim"))
	assertHasViolation(t, "pas-claim-1", violations)
}

// TC-PASC-003: use=predetermination on PAS-profiled resource fails
func TestPASC_Claim1_Predetermination(t *testing.T) {
	violations := checkConstraints(t, "Claim", minimalPASClaim("predetermination"))
	assertHasViolation(t, "pas-claim-1", violations)
}

// TC-PASC-004: Claim without PAS profile — pas-claim-1 does NOT fire regardless of use
func TestPASC_Claim1_NonPASClaimIgnored(t *testing.T) {
	res := map[string]interface{}{
		"resourceType": "Claim",
		"use":          "claim", // would fail if PAS profile present
	}
	violations := checkConstraints(t, "Claim", res)
	assertNoViolation(t, "pas-claim-1", violations)
}

// TC-PASC-005: Empty use value on PAS-profiled resource fails
func TestPASC_Claim1_EmptyUse(t *testing.T) {
	violations := checkConstraints(t, "Claim", minimalPASClaim(""))
	assertHasViolation(t, "pas-claim-1", violations)
}

// TC-PASC-006: pas-claim-1 violation is severity=error
func TestPASC_Claim1_SeverityIsError(t *testing.T) {
	violations := checkConstraints(t, "Claim", minimalPASClaim("claim"))
	for _, v := range violations {
		if v.Key == "pas-claim-1" {
			if v.Severity != "error" {
				t.Errorf("expected severity=error, got %s", v.Severity)
			}
			return
		}
	}
	t.Error("pas-claim-1 not found in violations")
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-PASC-007..012  pas-claim-2  Claim must have ≥1 item
// ─────────────────────────────────────────────────────────────────────────────

// TC-PASC-007: Claim with one item passes
func TestPASC_Claim2_OneItemPasses(t *testing.T) {
	violations := checkConstraints(t, "Claim", minimalPASClaim("preauthorization"))
	assertNoViolation(t, "pas-claim-2", violations)
}

// TC-PASC-008: PAS Claim with no item array fails
func TestPASC_Claim2_NoItemFails(t *testing.T) {
	res := map[string]interface{}{
		"resourceType": "Claim",
		"use":          "preauthorization",
		"insurance": []interface{}{
			map[string]interface{}{"coverage": map[string]interface{}{"reference": "Coverage/c1"}},
		},
	}
	for k, v := range pasClaimMeta(pasClaimProfile) { res[k] = v }
	violations := checkConstraints(t, "Claim", res)
	assertHasViolation(t, "pas-claim-2", violations)
}

// TC-PASC-009: PAS Claim with empty item array fails
func TestPASC_Claim2_EmptyItemFails(t *testing.T) {
	res := minimalPASClaim("preauthorization")
	res["item"] = []interface{}{}
	violations := checkConstraints(t, "Claim", res)
	assertHasViolation(t, "pas-claim-2", violations)
}

// TC-PASC-010: Non-PAS Claim without item does not trigger pas-claim-2
func TestPASC_Claim2_NonPASIgnored(t *testing.T) {
	res := map[string]interface{}{"resourceType": "Claim", "use": "claim"}
	violations := checkConstraints(t, "Claim", res)
	assertNoViolation(t, "pas-claim-2", violations)
}

// TC-PASC-011: Multiple items all pass
func TestPASC_Claim2_MultipleItems(t *testing.T) {
	res := minimalPASClaim("preauthorization")
	res["item"] = []interface{}{
		map[string]interface{}{"sequence": 1},
		map[string]interface{}{"sequence": 2},
	}
	violations := checkConstraints(t, "Claim", res)
	assertNoViolation(t, "pas-claim-2", violations)
}

// TC-PASC-012: pas-claim-2 violation is severity=error
func TestPASC_Claim2_SeverityIsError(t *testing.T) {
	res := minimalPASClaim("preauthorization")
	res["item"] = []interface{}{}
	for _, v := range checkConstraints(t, "Claim", res) {
		if v.Key == "pas-claim-2" && v.Severity != "error" {
			t.Errorf("expected severity=error, got %s", v.Severity)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-PASC-013..017  pas-claim-3  Claim.insurance must reference Coverage
// ─────────────────────────────────────────────────────────────────────────────

// TC-PASC-013: Claim with valid insurance.coverage.reference passes
func TestPASC_Claim3_ValidInsurancePasses(t *testing.T) {
	violations := checkConstraints(t, "Claim", minimalPASClaim("preauthorization"))
	assertNoViolation(t, "pas-claim-3", violations)
}

// TC-PASC-014: PAS Claim with no insurance array fails
func TestPASC_Claim3_NoInsuranceFails(t *testing.T) {
	res := minimalPASClaim("preauthorization")
	delete(res, "insurance")
	violations := checkConstraints(t, "Claim", res)
	assertHasViolation(t, "pas-claim-3", violations)
}

// TC-PASC-015: Insurance entry with no coverage.reference fails
func TestPASC_Claim3_NoCoverageRefFails(t *testing.T) {
	res := minimalPASClaim("preauthorization")
	res["insurance"] = []interface{}{
		map[string]interface{}{"sequence": 1, "focal": true},
	}
	violations := checkConstraints(t, "Claim", res)
	assertHasViolation(t, "pas-claim-3", violations)
}

// TC-PASC-016: Insurance entry with empty coverage block fails
func TestPASC_Claim3_EmptyCoverageBlockFails(t *testing.T) {
	res := minimalPASClaim("preauthorization")
	res["insurance"] = []interface{}{
		map[string]interface{}{"coverage": map[string]interface{}{}},
	}
	violations := checkConstraints(t, "Claim", res)
	assertHasViolation(t, "pas-claim-3", violations)
}

// TC-PASC-017: Non-PAS Claim without insurance does not trigger pas-claim-3
func TestPASC_Claim3_NonPASIgnored(t *testing.T) {
	res := map[string]interface{}{"resourceType": "Claim", "use": "claim"}
	violations := checkConstraints(t, "Claim", res)
	assertNoViolation(t, "pas-claim-3", violations)
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-PASC-018..022  pas-bundle-1  Bundle must have exactly 1 Claim
// ─────────────────────────────────────────────────────────────────────────────

// TC-PASC-018: Bundle with exactly one Claim passes
func TestPASC_Bundle1_OneClaimPasses(t *testing.T) {
	bundle := pasBundleWithEntries(
		validPASClaimResource(), patientResource(), coverageResource(),
	)
	violations := checkConstraints(t, "Bundle", bundle)
	assertNoViolation(t, "pas-bundle-1", violations)
}

// TC-PASC-019: Bundle with no Claim fails
func TestPASC_Bundle1_NoClaimFails(t *testing.T) {
	bundle := pasBundleWithEntries(patientResource(), coverageResource())
	violations := checkConstraints(t, "Bundle", bundle)
	assertHasViolation(t, "pas-bundle-1", violations)
}

// TC-PASC-020: Bundle with two Claims fails
func TestPASC_Bundle1_TwoClaimsFails(t *testing.T) {
	bundle := pasBundleWithEntries(
		validPASClaimResource(), validPASClaimResource(), patientResource(),
	)
	violations := checkConstraints(t, "Bundle", bundle)
	assertHasViolation(t, "pas-bundle-1", violations)
}

// TC-PASC-021: Non-PAS Bundle without Claim does not trigger pas-bundle-1
func TestPASC_Bundle1_NonPASIgnored(t *testing.T) {
	bundle := map[string]interface{}{
		"resourceType": "Bundle",
		"type":         "searchset",
		"entry":        []interface{}{map[string]interface{}{"resource": patientResource()}},
	}
	violations := checkConstraints(t, "Bundle", bundle)
	assertNoViolation(t, "pas-bundle-1", violations)
}

// TC-PASC-022: pas-bundle-1 violation is severity=error
func TestPASC_Bundle1_SeverityIsError(t *testing.T) {
	bundle := pasBundleWithEntries(patientResource())
	for _, v := range checkConstraints(t, "Bundle", bundle) {
		if v.Key == "pas-bundle-1" && v.Severity != "error" {
			t.Errorf("expected severity=error, got %s", v.Severity)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-PASC-023..026  pas-bundle-2  Bundle must have ≥1 Patient
// ─────────────────────────────────────────────────────────────────────────────

// TC-PASC-023: Bundle with one Patient passes
func TestPASC_Bundle2_OnePatientPasses(t *testing.T) {
	bundle := pasBundleWithEntries(validPASClaimResource(), patientResource(), coverageResource())
	violations := checkConstraints(t, "Bundle", bundle)
	assertNoViolation(t, "pas-bundle-2", violations)
}

// TC-PASC-024: Bundle without Patient fails
func TestPASC_Bundle2_NoPatientFails(t *testing.T) {
	bundle := pasBundleWithEntries(validPASClaimResource(), coverageResource())
	violations := checkConstraints(t, "Bundle", bundle)
	assertHasViolation(t, "pas-bundle-2", violations)
}

// TC-PASC-025: Bundle with multiple Patients passes
func TestPASC_Bundle2_MultiplePatientsPasses(t *testing.T) {
	bundle := pasBundleWithEntries(
		validPASClaimResource(), patientResource(), patientResource(), coverageResource(),
	)
	violations := checkConstraints(t, "Bundle", bundle)
	assertNoViolation(t, "pas-bundle-2", violations)
}

// TC-PASC-026: Non-PAS Bundle without Patient does not trigger pas-bundle-2
func TestPASC_Bundle2_NonPASIgnored(t *testing.T) {
	bundle := map[string]interface{}{
		"resourceType": "Bundle",
		"type":         "batch",
		"entry":        []interface{}{map[string]interface{}{"resource": coverageResource()}},
	}
	violations := checkConstraints(t, "Bundle", bundle)
	assertNoViolation(t, "pas-bundle-2", violations)
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-PASC-027..030  pas-bundle-3  Bundle must have ≥1 Coverage
// ─────────────────────────────────────────────────────────────────────────────

// TC-PASC-027: Bundle with Coverage passes
func TestPASC_Bundle3_CoveragePasses(t *testing.T) {
	bundle := pasBundleWithEntries(validPASClaimResource(), patientResource(), coverageResource())
	violations := checkConstraints(t, "Bundle", bundle)
	assertNoViolation(t, "pas-bundle-3", violations)
}

// TC-PASC-028: Bundle without Coverage fails
func TestPASC_Bundle3_NoCoverageFails(t *testing.T) {
	bundle := pasBundleWithEntries(validPASClaimResource(), patientResource())
	violations := checkConstraints(t, "Bundle", bundle)
	assertHasViolation(t, "pas-bundle-3", violations)
}

// TC-PASC-029: Bundle with multiple Coverage resources passes
func TestPASC_Bundle3_MultipleCoveragePasses(t *testing.T) {
	bundle := pasBundleWithEntries(
		validPASClaimResource(), patientResource(), coverageResource(), coverageResource(),
	)
	violations := checkConstraints(t, "Bundle", bundle)
	assertNoViolation(t, "pas-bundle-3", violations)
}

// TC-PASC-030: Non-PAS Bundle without Coverage does not trigger pas-bundle-3
func TestPASC_Bundle3_NonPASIgnored(t *testing.T) {
	bundle := map[string]interface{}{
		"resourceType": "Bundle",
		"type":         "searchset",
		"entry":        []interface{}{map[string]interface{}{"resource": patientResource()}},
	}
	violations := checkConstraints(t, "Bundle", bundle)
	assertNoViolation(t, "pas-bundle-3", violations)
}

// ─────────────────────────────────────────────────────────────────────────────
// TC-PASC-031..034  Non-PAS resources unaffected
// ─────────────────────────────────────────────────────────────────────────────

// TC-PASC-031: Generic Claim (no PAS profile) does not trigger any PAS constraints
func TestPASC_GenericClaimNoViolations(t *testing.T) {
	res := map[string]interface{}{
		"resourceType": "Claim",
		"use":          "claim",
		// no meta.profile
	}
	for _, v := range checkConstraints(t, "Claim", res) {
		if v.Key == "pas-claim-1" || v.Key == "pas-claim-2" || v.Key == "pas-claim-3" {
			t.Errorf("unexpected PAS constraint %s on generic Claim", v.Key)
		}
	}
}

// TC-PASC-032: Non-FHIR resource type returns empty violations (no panic)
func TestPASC_UnknownResourceType_NoPanic(t *testing.T) {
	cr := r4.GetConstraintRegistry()
	violations := cr.Check("UnknownFHIRResource", map[string]interface{}{"resourceType": "UnknownFHIRResource"})
	if violations != nil && len(violations) > 0 {
		t.Errorf("expected no violations for unknown type, got %v", violations)
	}
}

// TC-PASC-033: Standard obs-6 constraint still fires alongside PAS constraints
func TestPASC_ExistingConstraintsStillWork(t *testing.T) {
	res := map[string]interface{}{
		"resourceType":      "Observation",
		"dataAbsentReason":  map[string]interface{}{},
		"valueString":       "some value",
	}
	violations := checkConstraints(t, "Observation", res)
	assertHasViolation(t, "obs-6", violations)
}

// TC-PASC-034: Full valid PAS bundle (all 5 resources) has zero violations
func TestPASC_FullValidBundle_NoViolations(t *testing.T) {
	bundle := pasBundleWithEntries(
		validPASClaimResource(),
		patientResource(),
		coverageResource(),
		practResource(),
		map[string]interface{}{"resourceType": "Organization"},
	)
	for _, v := range checkConstraints(t, "Bundle", bundle) {
		if v.Key == "pas-bundle-1" || v.Key == "pas-bundle-2" || v.Key == "pas-bundle-3" {
			t.Errorf("unexpected PAS bundle violation %s on valid bundle", v.Key)
		}
	}
}
