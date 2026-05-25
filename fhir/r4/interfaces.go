// Package r4 provides compile-once, lookup-fast FHIR validation for R4 and future versions.
//
// All schemas are loaded and compiled at startup via InitRegistry(); validation on the hot
// path is a single O(1) map lookup followed by O(n) rule application with zero file I/O.
//
// Adding a new FHIR version (R5, R6, …) requires only placing schema .gz files under
// schemas/fhir/R5/resources/ — no code changes.
package r4

// ─────────────────────────────────────────────────────────────────────────────
// Core interfaces (depend only on types defined in this package)
// ─────────────────────────────────────────────────────────────────────────────

// ProfileProvider returns pre-compiled FHIR profiles keyed by version, resource
// type, and profile name.
type ProfileProvider interface {
	// Get returns the CompiledProfile for the given triple.
	// version="" → "R4", profile="" → "base".
	// Returns (nil, false) when the profile is not registered.
	Get(version, resourceType, profile string) (*CompiledProfile, bool)

	// List returns every RegistryKey registered for the given version.
	List(version string) []RegistryKey

	// GetStats returns loader diagnostics (counts, errors, timing).
	GetStats() RegistryStats
}

// TerminologyLookup checks whether a code is a member of a known ValueSet.
type TerminologyLookup interface {
	// Contains returns (inSet, valueSetKnown).
	//   inSet=true  → code is a member of the ValueSet.
	//   known=false → ValueSet is not embedded; caller should treat as pass-through.
	Contains(valueSetURL, code string) (inSet, known bool)
}

// ConstraintChecker evaluates resource-specific FHIR R4 invariants expressed as
// Go predicate functions (a targeted subset of FHIRPath semantics).
type ConstraintChecker interface {
	// Check runs all registered constraints for resourceType and returns violations.
	// Returns nil if there are no violations or no constraints are registered for the type.
	Check(resourceType string, resource map[string]interface{}) []ConstraintViolation
}

// ResourceValidator is the top-level FHIR validation interface used by pipeline
// executors and test harnesses.
type ResourceValidator interface {
	// ValidateResource validates a standalone (non-Bundle) FHIR resource.
	ValidateResource(resource map[string]interface{}, opts ValidationOptions) ResourceResult

	// ValidateBundle validates a FHIR Bundle and every resource entry within it.
	ValidateBundle(bundle map[string]interface{}, opts ValidationOptions) BundleResult
}
