// services/cda_coverage/hl7_adapter.go
//
// HL7 v2's own Coverage Audit adapter. Two genuinely different HL7→FHIR
// mapping mechanisms coexist in this codebase — fhir.build (config-driven,
// already live-tracked via field_utils.go's resolveJSONPathValue/
// resolveHL7FieldValue hooks, see CLAUDE.md's Coverage Audit generalization
// section) and hl7_fhir_transform (services/hl7_fhir_transform_service_v3.go,
// a single, shared, app-lifetime singleton used exclusively by ORU^R01
// interfaces and a real share of ADT^A01 ones — confirmed via a direct query
// against this project's own real transformation_steps data, not assumed).
//
// A user-confirmed product decision (2026-09-26): hl7_fhir_transform's own
// extraction engine (transform_hl7_extractor.go) is NEVER instrumented — the
// risk of threading per-message state through a live, concurrently-used
// singleton was judged not worth it. Instead, this adapter derives its
// "touched" signal for THAT mechanism statically: it reads the SAME
// field-mapping catalog hl7_fhir_transform itself would resolve for this
// interface/message-type (via HL7FHIRTransformServiceV3.ListConfiguredMappings,
// a read-only accessor added for exactly this purpose) and cross-references
// it against what's genuinely populated in the message. This is an HONEST,
// NAMED trade-off: it answers "does a mapping rule exist for this field,"
// not "did extraction definitely succeed for this specific message" — a
// field could be configured, present, and still silently fail to extract for
// a data-quality reason this approach wouldn't catch. Accepted explicitly in
// exchange for zero risk to the live engine.
//
// Element (component) level (2026-10-02): per-interface opt-in, mirroring
// CDA's/FHIR's own entry-vs-element split — resolveConfiguredFields already
// reads FieldMapping.HL7Component/HL7SubComponent (both had existed, unused,
// since this adapter's original build), and hl7.BuildFieldPresenceWithGranularity
// already walks FieldInfo.Subfields (same "SEGMENT.FIELD.COMPONENT" key shape
// resolveHL7FieldValue already understands) — neither needed new data, only a
// deeper walk + a richer catalog lookup. Purely static-catalog-driven for
// BOTH HL7→FHIR mechanisms (fhir.build and hl7_fhir_transform alike), same as
// the field-level tier above — this adapter never relies on
// resolveHL7FieldValue's own live tracker hook actually having fired, even
// for fhir.build-routed interfaces where it technically does; the adapter's
// own tracker.Record() call is what BuildReportWithClassifier's isCovered
// check actually keys off.
package cdacoverage

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"

	"ezhealthkonnect/hl7"
	"ezhealthkonnect/services"
	"ezhealthkonnect/services/coverage"
	"ezhealthkonnect/services/executors"
)

// hl7AdministrativeSegments are envelope/batch-header segments — never
// clinical content, so their fields are excluded from the inventory entirely
// (not merely "classified administrative") since Coverage Audit's own
// report-building only ever applies clinical/administrative classification at
// element granularity (see element_classifier.go / report.go's
// BuildReportWithClassifier) — a whole-segment exclusion upfront is simpler
// and sufficient here, so Classify() stays unused (see its own doc comment).
var hl7AdministrativeSegments = map[string]bool{
	"MSH": true, "BHS": true, "BTS": true, "FHS": true, "FTS": true,
}

// hl7Adapter implements Adapter for format key "hl7v2".
type hl7Adapter struct {
	transformSvc *services.HL7FHIRTransformServiceV3
}

func (a *hl7Adapter) FormatKey() string { return "hl7v2" }

// Classify always returns false (never administrative) — see
// hl7AdministrativeSegments' own doc comment on why whole-segment exclusion
// happens upfront in BuildInventory instead; kept only to satisfy the Adapter
// interface. Every component this adapter's own element-level tier can ever
// surface belongs to a segment that already passed that whole-segment
// filter, so there's no remaining administrative/clinical split left to make
// here — unlike CDA/FHIR, where individual ELEMENTS within an otherwise-kept
// section can still be administrative (e.g. an id or a nullFlavor).
//
// 2026-10 postmortem: this method's own BODY was accidentally dropped (only
// its doc comment survived) during an earlier same-day edit that updated the
// comment but forgot to re-include the function declaration in the
// replacement text — leaving *hl7Adapter* silently NOT satisfying the
// Adapter interface at all. go build/go vet never caught it because
// coverage.RegisterAdapter's own parameter type is the minimal
// coverage.FormatAdapter (FormatKey() only), not this richer interface — no
// compile-time check anywhere actually asserts *hl7Adapter implements
// Adapter. The failure only surfaced as a runtime type assertion miss
// (adapterFor's own fa.(Adapter) check), manifesting as "no registered
// adapter for source_format=hl7v2" for EVERY real HL7 message, for however
// long this was live. Root-caused via a targeted debug log printing the
// concrete type AND the failed assertion together (see registry.go's own
// adapterFor) — found %T was "*cdacoverage.hl7Adapter" (the right type) but
// the assertion still failed, which can only mean the method set itself was
// incomplete, not a build artifact or caching issue (an earlier, wrong
// hypothesis this session chased first, including two full --no-cache
// rebuilds that understandably changed nothing). No compile-time assertion
// existed anywhere to catch this; none of the OTHER adapters (cdaAdapter,
// fhirAdapter, ediAdapter, ncpdpScriptAdapter, ncpdpTelecomAdapter) were
// missing a method the same way, confirmed by direct inspection of each.
func (a *hl7Adapter) Classify(path string) bool { return false }

func (a *hl7Adapter) BuildInventory(ctx context.Context, interfaceID string, envelope map[string]interface{}, elementLevel bool, tracker *executors.CDACoverageTracker) ([]InventoryItem, error) {
	msg, err := decodeEnhancedParsedMessage(envelope)
	if err != nil || msg == nil || len(msg.SegmentGroups) == 0 {
		return nil, nil // not an HL7 message, or nothing parsed
	}

	messageType := msg.MessageType.Name
	if messageType == "" {
		return nil, nil
	}

	cat := a.resolveConfiguredFields(ctx, interfaceID, messageType)

	presence := hl7.BuildFieldPresenceWithGranularity(msg, elementLevel)
	items := make([]InventoryItem, 0, len(presence))
	for _, p := range presence {
		if hl7AdministrativeSegments[p.SegmentName] {
			continue
		}
		item := InventoryItem{
			Category:     p.SegmentName,
			SectionKey:   p.FieldPath,
			SectionTitle: p.FieldPath,
			EntryIndex:   p.SegmentIndex,
		}
		items = append(items, item)
		if tracker == nil {
			continue
		}
		var covered bool
		if parent, ok := hl7ParentFieldKey(p.FieldPath); ok {
			// p.FieldPath is a component-level item (e.g. "PID.5.1") --
			// covered either by a mapping targeting that EXACT component, or
			// by a mapping targeting its WHOLE parent field with NO
			// component specified ("PID.5") -- reading the whole composite
			// field necessarily reads everything inside it. A mapping that
			// only targets a DIFFERENT sibling component (e.g. "PID.5.2")
			// must NOT cover this one -- anyFieldMapping (below) is
			// deliberately not consulted here, only wholeFieldOnly.
			covered = cat.componentLevel[p.FieldPath] || cat.wholeFieldOnly[parent]
		} else {
			// p.FieldPath is a field-level item (elementLevel disabled, or
			// this field had no populated subfields to decompose into) --
			// ANY mapping touching this field at all, whole-field or
			// component-specific, counts, matching the ORIGINAL (pre-
			// element-level) field-level semantic: a human deliberately
			// mapped something inside this field, so the field itself isn't
			// a gap, even though a SPECIFIC component's own mapping
			// wouldn't be precise enough to prove by itself.
			covered = cat.anyFieldMapping[p.FieldPath]
		}
		// Live tracking (2026-10): an OR, never a replacement -- strictly
		// refines the static-catalog signal above, can only ADD coverage,
		// never remove it. Checked against the BARE "SEGMENT.FIELD
		// [.COMPONENT]" key (p.FieldPath), not item.TrackingKey()'s own
		// "#N"-segment-instance-suffixed form: extractHL7ValueAtomic
		// (transform_hl7_extractor.go) has no way to know WHICH instance
		// of a repeating segment it just read from (FieldMapping carries
		// no instance index at all) -- the same pre-existing limitation
		// resolveHL7FieldValue's own live tracking already has for
		// fhir.build-routed repeating segments (it resolves against
		// enhancedSegments, the one-map-per-type view, never a specific
		// instance), not a new gap this introduces. A field genuinely
		// read live by either HL7->FHIR mechanism (hl7_fhir_transform via
		// this new ctx-threaded hook, or fhir.build via its own
		// already-live resolveHL7FieldValue hook) is strictly stronger
		// evidence of real coverage than "a rule exists in the catalog."
		if tracker.Touched(p.FieldPath) {
			covered = true
		}
		if covered {
			tracker.Record(item.TrackingKey())
		}
	}
	return items, nil
}

// hl7ParentFieldKey returns ("SEGMENT.FIELD", true) for a component-level key
// ("SEGMENT.FIELD.COMPONENT", exactly 3 dot-separated parts) or ("", false)
// for anything else (a field-level key has only 2 parts) -- used to check
// whether a mapping targeting the WHOLE field also covers one of its own
// components.
func hl7ParentFieldKey(path string) (string, bool) {
	parts := strings.Split(path, ".")
	if len(parts) != 3 {
		return "", false
	}
	return parts[0] + "." + parts[1], true
}

// hl7ConfiguredCatalog is resolveConfiguredFields' own return shape — three
// maps answering three genuinely different questions, not one:
type hl7ConfiguredCatalog struct {
	// anyFieldMapping["SEGMENT.FIELD"] is true when ANY mapping touches this
	// field at all, whole-field or component-specific — the semantic a
	// FIELD-LEVEL presence item needs (elementLevel disabled, or a field with
	// no populated subfields to decompose into): a human deliberately mapped
	// SOMETHING inside this field, so reporting the whole field as a gap
	// would be wrong, even though that alone isn't precise enough to cover
	// any ONE specific component.
	anyFieldMapping map[string]bool
	// wholeFieldOnly["SEGMENT.FIELD"] is true only when a mapping targets the
	// field with NO component specified — the semantic a COMPONENT-LEVEL
	// presence item's ancestor-rollup needs: reading the whole composite
	// field necessarily reads everything inside it, but a mapping that only
	// targets ONE sibling component must never cover another.
	wholeFieldOnly map[string]bool
	// componentLevel["SEGMENT.FIELD.COMPONENT"] is true for a mapping
	// targeting that EXACT component.
	componentLevel map[string]bool
}

// resolveConfiguredFields statically resolves which HL7 paths this
// interface/message-type actually maps, per HL7FHIRTransformServiceV3's OWN
// resolution priority (embedded_mappings → interface_message_mappings →
// delta → legacy → hl7_fhir_templates OOB) — see ListConfiguredMappings' own
// doc comment. FieldMapping.HL7SubComponent is deliberately folded into its
// own owning component's key rather than given a 4th tier, since
// SubfieldInfo (hl7/types.go) itself has no sub-sub-component level to match
// it against — the inventory can never represent anything deeper than
// "SEGMENT.FIELD.COMPONENT", so a mapping naming a sub-component still only
// needs to mark that component's own presence item covered. Never fails the
// caller: an error or empty result here just means BuildInventory reports
// every populated field/component as a gap, the same fail-closed posture
// every other best-effort lookup in this feature already takes.
func (a *hl7Adapter) resolveConfiguredFields(ctx context.Context, interfaceID, messageType string) hl7ConfiguredCatalog {
	cat := hl7ConfiguredCatalog{
		anyFieldMapping: map[string]bool{},
		wholeFieldOnly:  map[string]bool{},
		componentLevel:  map[string]bool{},
	}
	if a.transformSvc == nil {
		return cat
	}
	req := &services.TransformRequest{InterfaceID: interfaceID, MessageType: messageType}
	mappings, err := a.transformSvc.ListConfiguredMappings(ctx, messageType, "", req)
	if err != nil {
		return cat
	}
	for _, m := range mappings {
		if m.SegmentName == "" || m.HL7Field == "" {
			continue
		}
		fieldKey := m.SegmentName + "." + m.HL7Field
		cat.anyFieldMapping[fieldKey] = true
		if m.HL7Component == "" {
			cat.wholeFieldOnly[fieldKey] = true
			continue
		}
		cat.componentLevel[fieldKey+"."+m.HL7Component] = true
	}
	return cat
}

// decodeEnhancedParsedMessage converts envelope (the generic, JSON-decoded
// ParsedJSON map every format's worker-pool job hands to BuildInventory) into
// a typed *hl7.EnhancedParsedMessage via a JSON round-trip — envelope
// originated FROM marshaling exactly this struct at parse time, so its own
// json tags ("enhancedSegments", "segmentGroups", "messageType", ...) already
// match. A direct type assertion is never attempted here (unlike some
// in-process-only callers elsewhere in this codebase) since envelope always
// comes from persisted object storage, never a live Go value.
func decodeEnhancedParsedMessage(envelope map[string]interface{}) (*hl7.EnhancedParsedMessage, error) {
	raw, err := json.Marshal(envelope)
	if err != nil {
		return nil, err
	}
	var msg hl7.EnhancedParsedMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil, err
	}
	return &msg, nil
}

// RegisterHL7Adapter registers the HL7 v2 coverage-audit adapter under format
// key "hl7v2". Called once from NewWorkerPool (worker_pool.go), which already
// receives db as a constructor-injected dependency. Deliberately constructs
// its OWN, dedicated *services.HL7FHIRTransformServiceV3 instance rather than
// sharing the one executor_registry.go uses for real message processing —
// zero shared mutable state (that instance's own assemblyRules cache etc.),
// zero risk of interfering with live processing, matching this whole
// adapter's "never touch the live engine" design.
func RegisterHL7Adapter(db *sql.DB) {
	coverage.RegisterAdapter(&hl7Adapter{transformSvc: services.NewHL7FHIRTransformServiceV3(db)})
}
