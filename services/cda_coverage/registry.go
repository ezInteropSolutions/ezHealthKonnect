// services/cda_coverage/registry.go
//
// The rich, per-format Coverage Audit contract, plus CDA's own registration
// under it. See services/coverage/registry.go's doc comment for why the
// minimal FormatAdapter marker and this richer Adapter interface live in two
// different packages (an import-cycle constraint, not a design preference).
package cdacoverage

import (
	"context"

	cdaSchema "ezhealthkonnect/cda"
	"ezhealthkonnect/services/coverage"
	"ezhealthkonnect/services/executors"
	"ezhealthkonnect/uscdi"
)

// Adapter is the full per-format Coverage Audit contract: build the
// ground-truth inventory for one message's parsed content, and classify a
// given path as administrative/structural noise vs. clinical content. A
// concrete type satisfies this by implementing all three methods — it need
// not import this package to do so, only services/coverage (for FormatKey)
// or, like cdaAdapter below, this package directly since it's defined here.
type Adapter interface {
	coverage.FormatAdapter
	// BuildInventory enumerates every real field/entry path this format's
	// own schema says could exist against envelope (that format's own
	// parsed-content shape, e.g. CDA's ParsedJSON — each adapter knows which
	// key(s) of envelope it needs). Returns (nil, nil) when envelope isn't
	// this adapter's format after all (e.g. the fidelity mirror key is
	// missing) — not an error, just nothing to audit.
	//
	// interfaceID and tracker exist for adapters (currently only HL7's — see
	// hl7_adapter.go) whose "touched" signal can't come from a live runtime
	// tracker at all (see CLAUDE.md's Coverage Audit generalization section
	// on why hl7_fhir_transform's own extraction engine is deliberately never
	// instrumented) and instead derive it statically, from that interface's
	// own configured field-mapping catalog — such an adapter calls
	// tracker.Record(...) itself, as a side effect of building the inventory,
	// BEFORE the caller takes tracker.Snapshot(). CDA's own adapter (and any
	// future adapter whose format IS live-tracked, e.g. via the
	// resolveJSONPathValue/GetNestedValue hooks) ignores both parameters —
	// its tracking already happened earlier, during the real pipeline run.
	BuildInventory(ctx context.Context, interfaceID string, envelope map[string]interface{}, elementLevel bool, tracker *executors.CDACoverageTracker) ([]InventoryItem, error)
	// Classify reports whether path is administrative/structural noise
	// (true) rather than genuine clinical content (false) for this format's
	// own vocabulary — see element_classifier.go for CDA's own table.
	Classify(path string) bool
}

// Compile-time interface-satisfaction assertions for every concrete adapter
// type. Added 2026-10 after a real bug this exact gap allowed: hl7Adapter
// silently stopped satisfying Adapter (a method's own body was dropped
// during an edit, see hl7_adapter.go's own postmortem comment on its
// Classify method) and NOTHING caught it at compile time — coverage.
// RegisterAdapter's own parameter type is the minimal coverage.FormatAdapter
// (FormatKey() only), so a richer-interface mismatch only ever surfaced as a
// runtime type-assertion failure inside adapterFor, manifesting as "no
// registered adapter" for every real message of that format. These blank
// assignments cost nothing at runtime (eliminated by the compiler) and
// would have caught that exact bug immediately as a compile error instead
// of requiring live debugging to find.
var (
	_ Adapter = (*cdaAdapter)(nil)
	_ Adapter = (*hl7Adapter)(nil)
	_ Adapter = (*fhirAdapter)(nil)
	_ Adapter = (*ediAdapter)(nil)
	_ Adapter = (*ncpdpScriptAdapter)(nil)
	_ Adapter = (*ncpdpTelecomAdapter)(nil)
)

// adapterFor resolves formatKey to its full Adapter, if one is registered and
// implements the richer contract (every registered adapter should, in
// practice — this package is the only caller of Adapter's own methods).
func adapterFor(formatKey string) (Adapter, bool) {
	fa, ok := coverage.AdapterFor(formatKey)
	if !ok {
		return nil, false
	}
	a, ok := fa.(Adapter)
	return a, ok
}

// cdaAdapter implements Adapter for format key "ccda" as a pure routing shim
// around this package's own pre-existing BuildInventoryWithGranularity/
// isAdministrativeElementPath — registering it changes no CDA behavior.
type cdaAdapter struct {
	schemaLoader *cdaSchema.CDASchemaLoader
	vocabulary   *uscdi.USCDIVocabulary
}

func (a *cdaAdapter) FormatKey() string { return "ccda" }

func (a *cdaAdapter) BuildInventory(ctx context.Context, interfaceID string, envelope map[string]interface{}, elementLevel bool, tracker *executors.CDACoverageTracker) ([]InventoryItem, error) {
	// interfaceID/tracker unused: CDA's tracking already happened live, during
	// the real pipeline run (declarative_engine.go's own Record calls) — see
	// this method's own interface doc comment.
	xmlMirror, _ := envelope["xml"].(map[string]interface{})
	if xmlMirror == nil {
		return nil, nil // not a CDA message, or parse didn't reach the fidelity mirror
	}
	return BuildInventoryWithGranularity(xmlMirror, a.schemaLoader, a.vocabulary, elementLevel), nil
}

func (a *cdaAdapter) Classify(path string) bool {
	return isAdministrativeElementPath(path)
}

// RegisterCDAAdapter registers the CDA/CCD coverage-audit adapter under
// format key "ccda". Called once from NewWorkerPool (worker_pool.go), which
// already receives schemaLoader/vocabulary as constructor-injected
// dependencies — CDA can't self-register via a bare init() the way a format
// whose schema access is already a package-level getter (HL7, FHIR, EDI,
// NCPDP, NCPDP Telecom) will, since these two are per-consumer constructed,
// not package-level singletons (see NewWorkerPool's own doc comment on why).
func RegisterCDAAdapter(schemaLoader *cdaSchema.CDASchemaLoader, vocabulary *uscdi.USCDIVocabulary) {
	coverage.RegisterAdapter(&cdaAdapter{schemaLoader: schemaLoader, vocabulary: vocabulary})
}
