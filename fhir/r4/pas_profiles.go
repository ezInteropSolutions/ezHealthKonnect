package r4

// ─────────────────────────────────────────────────────────────────────────────
// Da Vinci Prior Authorization Support (PAS) STU2 — Constraint Extensions
//
// Reference: http://hl7.org/fhir/us/davinci-pas/STU2/
//
// This file registers PAS-specific invariants into the globalConstraintRegistry
// via an init() function so they apply automatically when profile = "davinci-pas"
// is passed in ValidationOptions.
//
// Constraints implemented:
//   pas-claim-1  Claim.use must be "preauthorization"
//   pas-claim-2  Claim must have at least one item
//   pas-claim-3  Claim must reference a Coverage resource via Claim.insurance
//   pas-bundle-1 PAS request bundle must contain exactly one Claim resource
//   pas-bundle-2 PAS request bundle must contain at least one Patient resource
//   pas-bundle-3 PAS request bundle must contain at least one Coverage resource
// ─────────────────────────────────────────────────────────────────────────────

func init() {
	registerPASConstraints()
}

func registerPASConstraints() {
	if globalConstraintRegistry == nil {
		return
	}
	r := globalConstraintRegistry

	// ── Claim ─────────────────────────────────────────────────────────────────

	r.add("Claim",
		// pas-claim-1: Da Vinci PAS requires Claim.use = "preauthorization"
		// Source: http://hl7.org/fhir/us/davinci-pas/StructureDefinition/profile-claim
		constraint("pas-claim-1", "error",
			"PAS Claim.use must be 'preauthorization' (Da Vinci PAS STU2 §3.2)",
			func(res map[string]interface{}) bool {
				use, _ := res["use"].(string)
				// Only enforce for resources that declare a PAS profile
				if !hasPASProfile(res, "profile-claim") {
					return true
				}
				return use == "preauthorization"
			},
		),

		// pas-claim-2: PAS Claim must have at least one item
		constraint("pas-claim-2", "error",
			"PAS Claim must have at least one item element (the service being requested)",
			func(res map[string]interface{}) bool {
				if !hasPASProfile(res, "profile-claim") {
					return true
				}
				items, ok := toSlice(res["item"])
				return ok && len(items) > 0
			},
		),

		// pas-claim-3: PAS Claim must include at least one insurance entry referencing coverage
		constraint("pas-claim-3", "error",
			"PAS Claim must have at least one insurance entry with a Coverage reference",
			func(res map[string]interface{}) bool {
				if !hasPASProfile(res, "profile-claim") {
					return true
				}
				insurance, ok := toSlice(res["insurance"])
				if !ok || len(insurance) == 0 {
					return false
				}
				for _, raw := range insurance {
					entry, ok := raw.(map[string]interface{})
					if !ok {
						continue
					}
					cov, _ := entry["coverage"].(map[string]interface{})
					if cov != nil && cov["reference"] != nil {
						return true
					}
				}
				return false
			},
		),
	)

	// ── Bundle ────────────────────────────────────────────────────────────────

	r.add("Bundle",
		// pas-bundle-1: PAS request bundle must contain exactly one Claim
		constraint("pas-bundle-1", "error",
			"PAS request bundle must contain exactly one Claim resource (Da Vinci PAS STU2 §3.1)",
			func(res map[string]interface{}) bool {
				if !hasPASProfile(res, "profile-pas-request-bundle") {
					return true
				}
				return countResourceType(res, "Claim") == 1
			},
		),

		// pas-bundle-2: PAS request bundle must contain at least one Patient
		constraint("pas-bundle-2", "error",
			"PAS request bundle must contain at least one Patient resource",
			func(res map[string]interface{}) bool {
				if !hasPASProfile(res, "profile-pas-request-bundle") {
					return true
				}
				return countResourceType(res, "Patient") >= 1
			},
		),

		// pas-bundle-3: PAS request bundle must contain at least one Coverage
		constraint("pas-bundle-3", "error",
			"PAS request bundle must contain at least one Coverage resource",
			func(res map[string]interface{}) bool {
				if !hasPASProfile(res, "profile-pas-request-bundle") {
					return true
				}
				return countResourceType(res, "Coverage") >= 1
			},
		),
	)
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

// hasPASProfile returns true when the resource's meta.profile array contains a
// URL that ends with the given profileSuffix (e.g. "profile-claim").
// Constraints short-circuit to true when the profile is absent so they don't
// fire against generic Claim / Bundle resources unrelated to PAS.
func hasPASProfile(res map[string]interface{}, profileSuffix string) bool {
	meta, ok := res["meta"].(map[string]interface{})
	if !ok {
		return false
	}
	profiles, ok := toSlice(meta["profile"])
	if !ok {
		return false
	}
	for _, raw := range profiles {
		url, _ := raw.(string)
		if containsSuffix(url, profileSuffix) {
			return true
		}
	}
	return false
}

// countResourceType counts Bundle entries whose resource.resourceType matches rt.
func countResourceType(bundle map[string]interface{}, rt string) int {
	entries, ok := toSlice(bundle["entry"])
	if !ok {
		return 0
	}
	count := 0
	for _, raw := range entries {
		entry, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		resource, ok := entry["resource"].(map[string]interface{})
		if !ok {
			continue
		}
		if resource["resourceType"] == rt {
			count++
		}
	}
	return count
}

// containsSuffix reports whether s contains substr (used for profile URL matching).
func containsSuffix(s, substr string) bool {
	if s == "" || substr == "" {
		return false
	}
	return len(s) >= len(substr) &&
		s[len(s)-len(substr):] == substr
}
