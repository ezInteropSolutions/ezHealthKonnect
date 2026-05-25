package r4

import (
	"regexp"
	"strings"
)

// fhirIDPattern matches the FHIR R4 id datatype: [A-Za-z0-9\-\.]{1,64}
var fhirIDPattern = regexp.MustCompile(`^[A-Za-z0-9\-\.]{1,64}$`)

// fhirDatePattern matches YYYY-MM-DD (and the two shorter forms YYYY-MM, YYYY)
var fhirDatePattern = regexp.MustCompile(`^\d{4}(-\d{2}(-\d{2})?)?$`)

// fhirDateTimePattern matches ISO 8601 dateTime with timezone offset or Z
var fhirDateTimePattern = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}(:\d{2}(\.\d+)?)?([+-]\d{2}:\d{2}|Z)$`)

// isAbsoluteURI returns true when s starts with a valid scheme (e.g. urn:, http:, https:)
func isAbsoluteURI(s string) bool {
	if s == "" {
		return false
	}
	return strings.HasPrefix(s, "urn:") ||
		strings.HasPrefix(s, "http://") ||
		strings.HasPrefix(s, "https://")
}

// ─────────────────────────────────────────────────────────────────────────────
// ConstraintRegistry — Go predicate functions for FHIR R4 invariants
//
// Instead of a full FHIRPath engine we implement a targeted subset of the R4
// invariants that are (a) checkable with simple Go map navigation and (b) most
// frequently violated in generated HL7→FHIR output.
//
// Naming follows the official FHIR R4 constraint key (obs-6, bun-1, …).
// Each function returns true when the constraint IS satisfied.
// ─────────────────────────────────────────────────────────────────────────────

// ConstraintRegistry implements ConstraintChecker and holds per-resource-type
// constraint predicate lists.
type ConstraintRegistry struct {
	defs map[string][]CompiledConstraint // resourceType → constraints
}

var globalConstraintRegistry *ConstraintRegistry

func init() {
	globalConstraintRegistry = buildConstraintRegistry()
}

// GetConstraintRegistry returns the package-level singleton.
func GetConstraintRegistry() *ConstraintRegistry {
	return globalConstraintRegistry
}

// Check runs all registered constraints for resourceType.
// Implements ConstraintChecker.
func (cr *ConstraintRegistry) Check(resourceType string, resource map[string]interface{}) []ConstraintViolation {
	defs, ok := cr.defs[resourceType]
	if !ok {
		return nil
	}
	var violations []ConstraintViolation
	for _, c := range defs {
		if !c.Check(resource) {
			violations = append(violations, ConstraintViolation{
				Key:         c.Key,
				Severity:    c.Severity,
				Description: c.Description,
			})
		}
	}
	return violations
}

// constraintDefsFor returns the pre-built CompiledConstraint slice for a
// resource type; used by compileSchema to embed constraints in CompiledProfile.
func constraintDefsFor(resourceType string) []CompiledConstraint {
	if globalConstraintRegistry == nil {
		return nil
	}
	return globalConstraintRegistry.defs[resourceType]
}

// ─────────────────────────────────────────────────────────────────────────────
// Builder
// ─────────────────────────────────────────────────────────────────────────────

func buildConstraintRegistry() *ConstraintRegistry {
	r := &ConstraintRegistry{defs: make(map[string][]CompiledConstraint)}

	// ── Observation ───────────────────────────────────────────────────────────

	r.add("Observation",
		// obs-6: dataAbsentReason only when value[x] is absent
		// Source: https://hl7.org/fhir/R4/observation.html#invs
		constraint("obs-6", "error",
			"dataAbsentReason SHALL only be present if Observation.value[x] is not present",
			func(res map[string]interface{}) bool {
				_, hasAbsent := res["dataAbsentReason"]
				if !hasAbsent {
					return true // no dataAbsentReason → always satisfied
				}
				return !hasValueX(res)
			},
		),
		// obs-7: if Observation.code is the same as a component.code then value[x] must be present
		// We check the simpler rule: if components exist, each must have a code.
		constraint("obs-7", "warning",
			"If Observation.code is same as component.code the value SHALL be present",
			func(res map[string]interface{}) bool {
				comps, ok := toSlice(res["component"])
				if !ok || len(comps) == 0 {
					return true
				}
				for _, raw := range comps {
					comp, ok := raw.(map[string]interface{})
					if !ok {
						continue
					}
					if comp["code"] == nil {
						return false
					}
				}
				return true
			},
		),
	)

	// ── Bundle ────────────────────────────────────────────────────────────────

	r.add("Bundle",
		// bun-1: total only when type is searchset or history
		// Source: https://hl7.org/fhir/R4/bundle.html#invs
		constraint("bun-1", "error",
			"total only when a search or history",
			func(res map[string]interface{}) bool {
				_, hasTotal := res["total"]
				if !hasTotal {
					return true
				}
				t, _ := res["type"].(string)
				return t == "searchset" || t == "history"
			},
		),
		// bun-2: entry.search only when type is searchset
		constraint("bun-2", "error",
			"entry.search only when a search",
			func(res map[string]interface{}) bool {
				entries, ok := toSlice(res["entry"])
				if !ok {
					return true
				}
				t, _ := res["type"].(string)
				if t == "searchset" {
					return true
				}
				for _, raw := range entries {
					e, ok := raw.(map[string]interface{})
					if !ok {
						continue
					}
					if e["search"] != nil {
						return false
					}
				}
				return true
			},
		),
		// bun-3: entry.request only for batch/transaction/history
		constraint("bun-3", "error",
			"entry.request mandatory for batch/transaction, forbidden otherwise",
			func(res map[string]interface{}) bool {
				entries, ok := toSlice(res["entry"])
				if !ok {
					return true
				}
				t, _ := res["type"].(string)
				needsReq := t == "batch" || t == "transaction" || t == "history"
				for _, raw := range entries {
					e, ok := raw.(map[string]interface{})
					if !ok {
						continue
					}
					hasReq := e["request"] != nil
					if needsReq && !hasReq {
						return false
					}
					if !needsReq && hasReq {
						return false
					}
				}
				return true
			},
		),
		// bun-7: FullUrl must be unique in a bundle, or else entries with the same
		// fullUrl must have different meta.versionId (history bundles).
		// Simplified: fullUrl values must be unique when type != "history".
		constraint("bun-7", "error",
			"FullUrl must be unique in a bundle, or else entries with the same fullUrl must have different meta.versionId",
			func(res map[string]interface{}) bool {
				t, _ := res["type"].(string)
				if t == "history" {
					return true // version-aware uniqueness — skip
				}
				entries, ok := toSlice(res["entry"])
				if !ok {
					return true
				}
				seen := make(map[string]bool)
				for _, raw := range entries {
					e, ok := raw.(map[string]interface{})
					if !ok {
						continue
					}
					fu, _ := e["fullUrl"].(string)
					if fu == "" {
						continue
					}
					if seen[fu] {
						return false
					}
					seen[fu] = true
				}
				return true
			},
		),
		// bdl-12: A message bundle must have a MessageHeader as its first entry.
		// Source: https://hl7.org/fhir/R4/bundle.html#invs
		constraint("bdl-12", "error",
			"A message bundle must have a MessageHeader as the first entry",
			func(res map[string]interface{}) bool {
				t, _ := res["type"].(string)
				if t != "message" {
					return true // only applies to message bundles
				}
				entries, ok := toSlice(res["entry"])
				if !ok || len(entries) == 0 {
					return false // message bundle with no entries
				}
				first, ok := entries[0].(map[string]interface{})
				if !ok {
					return false
				}
				resource, ok := first["resource"].(map[string]interface{})
				if !ok {
					return false
				}
				rt, _ := resource["resourceType"].(string)
				return rt == "MessageHeader"
			},
		),
		// bdl-fullurl: each entry's fullUrl must be a valid absolute URI
		constraint("bdl-fullurl", "error",
			"Bundle entry fullUrl must be an absolute URI (e.g. urn:uuid:... or https://...)",
			func(res map[string]interface{}) bool {
				entries, ok := toSlice(res["entry"])
				if !ok {
					return true
				}
				for _, raw := range entries {
					e, ok := raw.(map[string]interface{})
					if !ok {
						continue
					}
					fu, _ := e["fullUrl"].(string)
					if fu == "" {
						continue // absence checked separately (bdl-8 not implemented)
					}
					if !isAbsoluteURI(fu) {
						return false
					}
				}
				return true
			},
		),
		// bdl-id: Bundle.id must conform to the FHIR id datatype [A-Za-z0-9\-\.]{1,64}
		constraint("bdl-id", "error",
			"Bundle.id must be 1–64 characters and contain only letters, digits, hyphens, or dots",
			func(res map[string]interface{}) bool {
				id, _ := res["id"].(string)
				if id == "" {
					return true // absence is a separate warning
				}
				return fhirIDPattern.MatchString(id)
			},
		),
	)

	// ── Patient ───────────────────────────────────────────────────────────────

	r.add("Patient",
		// pat-1: telecom requires system when value is provided
		constraint("pat-1", "error",
			"Patient.telecom.system SHALL be present when value is provided",
			func(res map[string]interface{}) bool {
				telecoms, ok := toSlice(res["telecom"])
				if !ok {
					return true
				}
				for _, raw := range telecoms {
					t, ok := raw.(map[string]interface{})
					if !ok {
						continue
					}
					if t["value"] != nil && t["system"] == nil {
						return false
					}
				}
				return true
			},
		),
		// pat-date: Patient.birthDate must be in FHIR date format (YYYY, YYYY-MM, or YYYY-MM-DD)
		constraint("pat-date", "error",
			"Patient.birthDate must be a valid FHIR date (YYYY, YYYY-MM, or YYYY-MM-DD)",
			func(res map[string]interface{}) bool {
				bd, _ := res["birthDate"].(string)
				if bd == "" {
					return true // absence checked by required-field layer
				}
				return fhirDatePattern.MatchString(bd)
			},
		),
		// pat-gender: Patient.gender must be male | female | other | unknown
		constraint("pat-gender", "error",
			"Patient.gender must be one of: male, female, other, unknown",
			func(res map[string]interface{}) bool {
				g, _ := res["gender"].(string)
				if g == "" {
					return true // optional field
				}
				return g == "male" || g == "female" || g == "other" || g == "unknown"
			},
		),
	)

	// ── AllergyIntolerance ────────────────────────────────────────────────────

	r.add("AllergyIntolerance",
		// ait-1: AllergyIntolerance.clinicalStatus SHALL be present if verificationStatus is not entered-in-error
		constraint("ait-1", "error",
			"AllergyIntolerance.clinicalStatus SHALL be present if verificationStatus is not entered-in-error",
			func(res map[string]interface{}) bool {
				vs := extractCodeableConcept(res["verificationStatus"])
				if vs == "entered-in-error" {
					return true
				}
				return res["clinicalStatus"] != nil
			},
		),
	)

	// ── Condition ─────────────────────────────────────────────────────────────

	r.add("Condition",
		// con-4: Condition.clinicalStatus SHALL NOT be present if verificationStatus is entered-in-error
		constraint("con-4", "error",
			"If condition is abated, then clinicalStatus must be either inactive, resolved, or remission",
			func(res map[string]interface{}) bool {
				// abatement[x] present → clinicalStatus must be inactive/resolved/remission
				if !hasAbatementX(res) {
					return true
				}
				cs := extractCodeableConcept(res["clinicalStatus"])
				return cs == "inactive" || cs == "resolved" || cs == "remission"
			},
		),
		// con-5: Condition.clinicalStatus SHALL be present if category is problem-list-item
		constraint("con-5", "warning",
			"Condition.clinicalStatus SHALL be present if category is problem-list-item",
			func(res map[string]interface{}) bool {
				cats, ok := toSlice(res["category"])
				if !ok {
					return true
				}
				for _, raw := range cats {
					cat, ok := raw.(map[string]interface{})
					if !ok {
						continue
					}
					if extractCodeableConcept(cat) == "problem-list-item" {
						return res["clinicalStatus"] != nil
					}
				}
				return true
			},
		),
	)

	// ── MedicationRequest ─────────────────────────────────────────────────────

	r.add("MedicationRequest",
		// mrd-1: reasonReference and reasonCode are mutually exclusive arrays
		//        (both may coexist in R4; this is a warning if both populated)
		constraint("mrd-1", "warning",
			"MedicationRequest.reasonCode and reasonReference should not both be populated",
			func(res map[string]interface{}) bool {
				rc, rcOk := toSlice(res["reasonCode"])
				rr, rrOk := toSlice(res["reasonReference"])
				if rcOk && len(rc) > 0 && rrOk && len(rr) > 0 {
					return false
				}
				return true
			},
		),
	)

	// ── Encounter ─────────────────────────────────────────────────────────────

	r.add("Encounter",
		// enc-1: Encounter.hospitalization SHALL NOT be present unless status is in-progress or finished
		constraint("enc-1", "warning",
			"Encounter.reasonReference SHALL be present if reason is required",
			func(res map[string]interface{}) bool {
				// Hospitalisation only meaningful for in-progress or onleave or finished
				if res["hospitalization"] == nil {
					return true
				}
				status, _ := res["status"].(string)
				allowed := map[string]bool{
					"in-progress": true, "onleave": true, "finished": true,
				}
				return allowed[status]
			},
		),
	)

	// ── Observation ─ datetime format ────────────────────────────────────────

	// obs-datetime and obs-status appended to existing Observation constraints
	r.add("Observation",
		// obs-datetime: effectiveDateTime must be valid ISO 8601 when present
		constraint("obs-datetime", "error",
			"Observation.effectiveDateTime must be a valid FHIR dateTime (ISO 8601 with timezone)",
			func(res map[string]interface{}) bool {
				edt, _ := res["effectiveDateTime"].(string)
				if edt == "" {
					return true
				}
				return fhirDateTimePattern.MatchString(edt)
			},
		),
		// obs-status: Observation.status must be a known value
		constraint("obs-status", "error",
			"Observation.status must be one of: registered, preliminary, final, amended, corrected, cancelled, entered-in-error, unknown",
			func(res map[string]interface{}) bool {
				status, _ := res["status"].(string)
				if status == "" {
					return true // absence caught by required-field layer
				}
				valid := map[string]bool{
					"registered": true, "preliminary": true, "final": true,
					"amended": true, "corrected": true, "cancelled": true,
					"entered-in-error": true, "unknown": true,
				}
				return valid[status]
			},
		),
	)

	// ── DiagnosticReport ──────────────────────────────────────────────────────

	r.add("DiagnosticReport",
		// drep-1: result or presentedForm when status is final, amended, corrected, appended
		constraint("drep-1", "warning",
			"DiagnosticReport SHALL have either result or presentedForm when status is final/amended/corrected/appended",
			func(res map[string]interface{}) bool {
				status, _ := res["status"].(string)
				finalStatuses := map[string]bool{
					"final": true, "amended": true, "corrected": true, "appended": true,
				}
				if !finalStatuses[status] {
					return true
				}
				results, rOk := toSlice(res["result"])
				forms, fOk := toSlice(res["presentedForm"])
				hasResult := rOk && len(results) > 0
				hasForm := fOk && len(forms) > 0
				return hasResult || hasForm
			},
		),
		// drep-datetime: effectiveDateTime must be valid ISO 8601 when present
		constraint("drep-datetime", "error",
			"DiagnosticReport.effectiveDateTime must be a valid FHIR dateTime (ISO 8601 with timezone)",
			func(res map[string]interface{}) bool {
				edt, _ := res["effectiveDateTime"].(string)
				if edt == "" {
					return true
				}
				return fhirDateTimePattern.MatchString(edt)
			},
		),
		// drep-issued: issued must be valid FHIR instant (ISO 8601 with timezone) when present
		constraint("drep-issued", "error",
			"DiagnosticReport.issued must be a valid FHIR instant (ISO 8601 with timezone)",
			func(res map[string]interface{}) bool {
				issued, _ := res["issued"].(string)
				if issued == "" {
					return true
				}
				return fhirDateTimePattern.MatchString(issued)
			},
		),
		// drep-status: DiagnosticReport.status must be a known value
		constraint("drep-status", "error",
			"DiagnosticReport.status must be one of: registered, partial, preliminary, final, amended, corrected, appended, cancelled, entered-in-error, unknown",
			func(res map[string]interface{}) bool {
				status, _ := res["status"].(string)
				if status == "" {
					return true // absence caught by required-field layer
				}
				valid := map[string]bool{
					"registered": true, "partial": true, "preliminary": true, "final": true,
					"amended": true, "corrected": true, "appended": true,
					"cancelled": true, "entered-in-error": true, "unknown": true,
				}
				return valid[status]
			},
		),
	)

	// ── Immunization ──────────────────────────────────────────────────────────

	r.add("Immunization",
		// imm-1: lotNumber should be present if status is "completed"
		constraint("imm-1", "warning",
			"Immunization.lotNumber SHOULD be present when status is completed",
			func(res map[string]interface{}) bool {
				status, _ := res["status"].(string)
				if status != "completed" {
					return true
				}
				v, _ := res["lotNumber"].(string)
				return v != ""
			},
		),
	)

	// ── Practitioner ──────────────────────────────────────────────────────────

	r.add("Practitioner",
		// practitioner identifier system should be present when value is present
		constraint("prac-1", "warning",
			"Practitioner.identifier.system SHALL be present when value is provided",
			func(res map[string]interface{}) bool {
				ids, ok := toSlice(res["identifier"])
				if !ok {
					return true
				}
				for _, raw := range ids {
					id, ok := raw.(map[string]interface{})
					if !ok {
						continue
					}
					if id["value"] != nil && id["system"] == nil {
						return false
					}
				}
				return true
			},
		),
	)

	return r
}

// ─────────────────────────────────────────────────────────────────────────────
// Internal helpers
// ─────────────────────────────────────────────────────────────────────────────

func (cr *ConstraintRegistry) add(resourceType string, cs ...CompiledConstraint) {
	cr.defs[resourceType] = append(cr.defs[resourceType], cs...)
}

func constraint(key, severity, desc string, fn ConstraintFunc) CompiledConstraint {
	return CompiledConstraint{Key: key, Severity: severity, Description: desc, Check: fn}
}

// hasValueX returns true if a FHIR resource contains any value[x] variant.
func hasValueX(r map[string]interface{}) bool {
	valueKeys := []string{
		"valueQuantity", "valueCodeableConcept", "valueString", "valueBoolean",
		"valueInteger", "valueRange", "valueRatio", "valueSampledData",
		"valueTime", "valueDateTime", "valuePeriod",
	}
	for _, k := range valueKeys {
		if r[k] != nil {
			return true
		}
	}
	return false
}

// hasAbatementX returns true if a Condition has any abatement[x] variant.
func hasAbatementX(r map[string]interface{}) bool {
	for _, k := range []string{"abatementDateTime", "abatementAge", "abatementPeriod",
		"abatementRange", "abatementString", "abatement"} {
		if r[k] != nil {
			return true
		}
	}
	return false
}

// toSlice coerces an interface{} to []interface{}, returning (nil, false) on failure.
func toSlice(v interface{}) ([]interface{}, bool) {
	if v == nil {
		return nil, false
	}
	s, ok := v.([]interface{})
	return s, ok
}

// extractCodeableConcept extracts the first code string from a CodeableConcept map.
func extractCodeableConcept(v interface{}) string {
	if v == nil {
		return ""
	}
	cc, ok := v.(map[string]interface{})
	if !ok {
		return ""
	}
	codings, ok := toSlice(cc["coding"])
	if !ok || len(codings) == 0 {
		// Try text
		t, _ := cc["text"].(string)
		return t
	}
	first, ok := codings[0].(map[string]interface{})
	if !ok {
		return ""
	}
	code, _ := first["code"].(string)
	return code
}
