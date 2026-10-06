// services/coverage/registry.go
//
// The one shared, format-agnostic seam Coverage Audit's per-format engines
// register into. This package is deliberately a true leaf — zero imports
// beyond the standard library — because both sides of the pipeline need to
// reach it without creating an import cycle:
//   - services (top-level, transformation_pipeline_helpers.go) needs only
//     "does this format have a registered coverage adapter at all" before
//     attaching a tracker to a running pipeline.
//   - services/cda_coverage (worker_pool.go) needs the FULL adapter contract
//     (build an inventory, classify a path as administrative vs. clinical)
//     to actually build and persist a report.
//
// services/cda_coverage already imports services (for AlertNotificationService
// et al.), so services cannot import services/cda_coverage back without a
// cycle — and every future format engine (hl7, fhir/r4, edi, ncpdp,
// ncpdptelecom) needs the same reachability from services without wanting to
// depend on cda_coverage's CDA-specific code either. Splitting the minimal
// marker (FormatAdapter) from the rich per-format contract (defined in
// services/cda_coverage as Adapter, a superset) resolves this: Go interface
// satisfaction only requires the concrete type's method set to match, not
// that its package import the interface's package, so a value registered
// here as a bare FormatAdapter can still be recovered and type-asserted up
// to the richer Adapter interface by whichever package defines it.
package coverage

import "sync"

// FormatAdapter is the minimal contract every coverage-audit-capable format
// registers under its own key (e.g. "ccda", "hl7v2", "edi_x12",
// "ncpdp_script", "ncpdp_telecom", "fhir").
type FormatAdapter interface {
	FormatKey() string
}

var (
	mu       sync.RWMutex
	adapters = map[string]FormatAdapter{}
)

// RegisterAdapter registers a (or replaces an existing) adapter under its own
// FormatKey(). Called once per format, either from that format's own init()
// (formats whose schema access is already a package-level singleton — HL7,
// FHIR, EDI, NCPDP, NCPDP Telecom) or explicitly once its constructor-injected
// dependencies exist (CDA, which needs a schema loader + USCDI vocabulary
// instance — see services/cda_coverage/registry.go's RegisterCDAAdapter).
func RegisterAdapter(a FormatAdapter) {
	if a == nil || a.FormatKey() == "" {
		return
	}
	mu.Lock()
	defer mu.Unlock()
	adapters[a.FormatKey()] = a
}

// AdapterFor looks up the registered adapter for formatKey. Callers that need
// more than FormatKey() type-assert the result to their own richer interface
// (e.g. services/cda_coverage.Adapter).
func AdapterFor(formatKey string) (FormatAdapter, bool) {
	if formatKey == "" {
		return nil, false
	}
	mu.RLock()
	defer mu.RUnlock()
	a, ok := adapters[formatKey]
	return a, ok
}

// RegisteredFormatKeys returns every currently-registered format key, sorted
// order not guaranteed — callers that need a stable order (e.g. a UI list)
// should sort the result themselves.
func RegisteredFormatKeys() []string {
	mu.RLock()
	defer mu.RUnlock()
	keys := make([]string, 0, len(adapters))
	for k := range adapters {
		keys = append(keys, k)
	}
	return keys
}
