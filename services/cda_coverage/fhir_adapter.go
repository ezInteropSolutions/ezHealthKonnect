// services/cda_coverage/fhir_adapter.go
//
// FHIR's own Coverage Audit adapter — scoped to FHIR-as-INBOUND-SOURCE only
// (e.g. an http_fhir_inbound-triggered pipeline that maps a received FHIR
// resource onward to something else). FHIR-as-BUILD-TARGET (the common
// fhir.build output-construction direction) is explicitly out of scope —
// that direction is already served by fhir_validation's required-field
// checks, and "coverage audit" is fundamentally a source-completeness
// question, symmetric with why CDA/HL7/EDI/NCPDP builds-as-outbound-targets
// aren't audited either (see CLAUDE.md's Coverage Audit generalization
// section).
//
// The lowest-effort of the four remaining formats by a wide margin: tracking
// is ALREADY live — every fhir.build/field_mapping/etc. sourcePath reading an
// inbound FHIR resource resolves through resolveFHIRFieldValue, which
// delegates straight to resolveJSONPathValue (field_utils.go), already
// hooked in Phase 0. Only the inventory + classifier + registration were
// missing.
package cdacoverage

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"ezhealthkonnect/services/coverage"
	"ezhealthkonnect/services/executors"
)

// fhirAdministrativeFields are pure FHIR-infrastructure elements — never
// clinical content. extension/modifierExtension are DELIBERATELY not in this
// set: US Core profiles routinely carry real clinical content in extensions
// (e.g. us-core-race/us-core-ethnicity), so blanket-excluding them would hide
// genuine gaps — the same judgment call CDA's own classifier had to make for
// "id" (never excluded, despite living in an otherwise-administrative-shaped
// neighborhood). Flagged as likely to need a user correction once this sees
// real US Core traffic, matching that exact precedent.
var fhirAdministrativeFields = map[string]bool{
	"id": true, "meta": true, "implicitRules": true, "language": true, "text": true,
}

// isFHIREnvelopeBookkeepingKey reports whether key is pipeline/parser
// plumbing rather than real resource content — never a clinical fact, must be
// excluded from the inventory walk the same way CDA's own inventory never
// treats its own envelope plumbing as a clinical fact.
//
// Every reserved key this whole pipeline engine adds to a message envelope
// (_format, _coverageTracker, _semantic_index, _sensitivity_map,
// _interfaceId, _variableContext, _stepOutput, ...) is, by this codebase's
// own established convention, underscore-prefixed — confirmed by grepping
// every such key referenced across services/transformation_pipeline_helpers.go,
// field_utils.go, and this package's own worker_pool.go/hl7_adapter.go, not
// assumed. A blanket underscore-prefix rule is deliberately used here instead
// of an explicit, ever-growing list of known reserved keys (the same
// anti-hardcoding principle this whole codebase already applies elsewhere) —
// the one honest, named trade-off is that a genuine FHIR "_elementName"
// primitive-extension field (a real but uncommon FHIR JSON convention, e.g.
// "_birthDate" carrying extensions for "birthDate") would also be excluded
// under this rule. Accepted for v1 given how much more likely a pipeline's
// own reserved key is to appear in real traffic.
func isFHIREnvelopeBookkeepingKey(key string) bool {
	if key == "resourceType" || key == "raw" || key == "parsedAt" || key == "schemaLoaded" || key == "enhancedFields" {
		return true
	}
	return strings.HasPrefix(key, "_")
}

// fhirAdapter implements Adapter for format key "fhir".
type fhirAdapter struct{}

func (a *fhirAdapter) FormatKey() string { return "fhir" }

// Classify always returns false — like hl7Adapter, this adapter's v1 scope is
// field-level only (no ElementPath), and administrative fields are excluded
// from the inventory entirely upfront (see fhirAdministrativeFields) rather
// than classified after the fact, since report.go's clinical/administrative
// split only ever applies at element granularity.
func (a *fhirAdapter) Classify(path string) bool { return false }

// BuildInventory walks every top-level, non-nil field of the inbound FHIR
// resource (already flat at envelope's own root) and builds one InventoryItem
// per field — no schema lookup needed to build the inventory itself (unlike
// HL7's, which needs the mapping catalog to know what's "touched"; here the
// SAME resolveJSONPathValue hook that already tracks every other JSON-shaped
// format's direct reads already does the touching for us — this method only
// needs to enumerate what's really there).
//
// SectionKey is the BARE field key (e.g. "name", "birthDate"), never
// "ResourceType.field" — confirmed by directly checking every real caller of
// GetFieldValue/DetectPathType in this codebase (grepped for IsFHIRPath/
// PathTypeFHIR): NOTHING actually constructs a "ResourceType.field"-prefixed
// sourcePath against a flat inbound resource — resolveFHIRFieldValue exists
// in the dispatch table but has zero real callers today. A real sourcePath
// reading this resource's own field (e.g. "name") has no HL7/FHIR/CDA path
// shape, so DetectPathType falls through to the generic PathTypeJSON branch —
// resolveJSONPathValue, already hooked in Phase 0 — and records the LITERAL
// bare key. This was caught by this file's own mandatory exact-key test
// (TestFHIRAdapter_TrackingKey_MatchesRealLiveTrackingHook) failing against
// an earlier, "ResourceType.field"-prefixed design — exactly the kind of
// mismatch that test exists to catch.
//
// elementLevel (opt-in, per interface — see worker_pool.go's
// resolveElementGranularity) additionally decomposes any top-level field
// whose value is itself a nested object/array (e.g. "name": [{"family":...,
// "given":[...]}]) into one InventoryItem per genuinely-populated LEAF path
// (e.g. "name[0].family", "name[0].given[0]"), using the identical
// dotted+bracket convention both resolveJSONPathValue (field_utils.go) and
// the goja script-tracking wrapper (coverage_script_tracking.go) already
// record real reads under — so no new tracking hook is needed, only a deeper
// walk of what already exists. A field whose value is already a scalar (e.g.
// "birthDate") has nothing to decompose into and is still reported exactly
// as the entry-level tier does — the coarse and fine tiers are never BOTH
// emitted for the same field, which would double-count one real data point
// as two separate "entries" in the report (FHIR's flat EntryIndex:-1 key
// space has no grouping mechanism to merge a parent item with its own
// children the way CDA's (SectionKey, EntryIndex) entryGroup does).
//
// Because a leaf's own literal key (e.g. "name[0].given[0]") is a DIFFERENT
// string than a coarser sourcePath that legitimately reads the whole
// container at once (e.g. "name", copying the array through unprocessed) —
// and isCovered's (report.go) ancestor-prefix rollup only activates for
// CDA/HL7-style "sectionKey#N/elementPath" keys (those containing a "/"),
// never FHIR's flat, slash-less ones — this method reconciles that itself
// (reconcileFHIRAncestorCoverage) before returning, mirroring the same
// "adapter self-canonicalizes into tracker.Record() before the report ever
// runs" pattern edi_adapter.go already established for its own, differently
// -shaped reconciliation need.
func (a *fhirAdapter) BuildInventory(ctx context.Context, interfaceID string, envelope map[string]interface{}, elementLevel bool, tracker *executors.CDACoverageTracker) ([]InventoryItem, error) {
	resourceType, _ := envelope["resourceType"].(string)
	if resourceType == "" {
		return nil, nil // not a FHIR resource
	}

	items := make([]InventoryItem, 0, len(envelope))
	for key, value := range envelope {
		if isFHIREnvelopeBookkeepingKey(key) || fhirAdministrativeFields[key] || value == nil {
			continue
		}
		if elementLevel {
			if leaves := fhirLeafPaths(value, key); len(leaves) > 0 {
				for _, leaf := range leaves {
					if tracker != nil {
						reconcileFHIRAncestorCoverage(leaf, tracker)
					}
					items = append(items, InventoryItem{
						Category:     resourceType,
						SectionKey:   leaf,
						SectionTitle: leaf,
						EntryIndex:   -1,
					})
				}
				continue // fully decomposed into leaves -- don't also emit the coarse item
			}
			// value is already a scalar (or an empty container) -- nothing to
			// decompose, fall through to the same coarse item entry-level
			// reporting always produces.
		}
		items = append(items, InventoryItem{
			Category:     resourceType,
			SectionKey:   key,
			SectionTitle: key,
			EntryIndex:   -1, // see InventoryItem.TrackingKey's own doc comment on this sentinel
		})
	}
	return items, nil
}

// fhirLeafPaths recursively decomposes value (a map or slice found under a
// FHIR field) into every genuinely-populated leaf path, prefixed with
// prefix — e.g. fhirLeafPaths(nameValue, "name") might return
// ["name[0].family", "name[0].given[0]"]. Returns nil for a value with
// nothing further to decompose (a scalar, or an empty/all-nil container) —
// the caller's own signal to fall back to reporting the coarse, top-level
// field as-is. Mirrors walkEDILeaves'/walkNCPDPLeaves' identical recursive
// shape in this same package, just keyed to FHIR's own dotted+bracket path
// convention instead of those formats' step-output-prefixed one.
func fhirLeafPaths(value interface{}, prefix string) []string {
	switch v := value.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var out []string
		for _, k := range keys {
			child := v[k]
			if child == nil {
				continue
			}
			childPrefix := prefix + "." + k
			if leaves := fhirLeafPaths(child, childPrefix); len(leaves) > 0 {
				out = append(out, leaves...)
			} else if isFHIRScalarLeaf(child) {
				out = append(out, childPrefix)
			}
		}
		return out
	case []interface{}:
		var out []string
		for i, item := range v {
			if item == nil {
				continue
			}
			childPrefix := fmt.Sprintf("%s[%d]", prefix, i)
			if leaves := fhirLeafPaths(item, childPrefix); len(leaves) > 0 {
				out = append(out, leaves...)
			} else if isFHIRScalarLeaf(item) {
				out = append(out, childPrefix)
			}
		}
		return out
	default:
		return nil // scalar at the top of this call -- caller falls back to the coarse item
	}
}

// isFHIRScalarLeaf reports whether v is a genuinely-populated scalar (never
// an empty string, matching every other adapter's "only what's really there"
// inventory philosophy) — used once fhirLeafPaths has already established v
// is not itself a further-decomposable map/slice.
func isFHIRScalarLeaf(v interface{}) bool {
	if s, ok := v.(string); ok {
		return s != ""
	}
	return v != nil
}

// fhirAncestorCandidates returns path plus every progressively-shorter
// ancestor prefix of it, most-specific first, by stripping exactly one
// trailing segment at a time — either a "[N]" bracket-index segment or a
// ".key" dot segment, whichever path's own tail actually is. Mirrors
// joinCoveragePath's/parseJSONPath's own bracket+dot convention (see
// coverage_script_tracking.go / field_utils.go) walked in reverse — e.g.
// "name[0].given[0]" -> ["name[0].given[0]", "name[0].given", "name[0]", "name"].
func fhirAncestorCandidates(path string) []string {
	out := []string{path}
	for {
		if strings.HasSuffix(path, "]") {
			if idx := strings.LastIndexByte(path, '['); idx >= 0 {
				path = path[:idx]
				out = append(out, path)
				continue
			}
		}
		if idx := strings.LastIndexByte(path, '.'); idx >= 0 {
			path = path[:idx]
			out = append(out, path)
			continue
		}
		break
	}
	return out
}

// reconcileFHIRAncestorCoverage marks leaf as touched (tracker.Record) when
// any of its own ancestor containers was genuinely read as one opaque value
// (e.g. a sourcePath or script reading "name" wholesale, never drilling into
// "name[0].given") — see BuildInventory's own doc comment on why this can't
// rely on report.go's isCovered ancestor rollup, which only ever activates
// for CDA/HL7-style "/"-delimited keys, not FHIR's flat ones.
func reconcileFHIRAncestorCoverage(leaf string, tracker *executors.CDACoverageTracker) {
	for _, ancestor := range fhirAncestorCandidates(leaf) {
		if tracker.Touched(ancestor) {
			tracker.Record(leaf)
			return
		}
	}
}

// init self-registers the FHIR adapter — safe as a bare init() (unlike CDA's
// and HL7's own adapters) since this one needs no constructor-injected
// dependencies at all; nothing to construct per-consumer.
func init() {
	coverage.RegisterAdapter(&fhirAdapter{})
}
