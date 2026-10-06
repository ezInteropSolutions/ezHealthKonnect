// services/cda_coverage/ncpdp_adapter.go
//
// NCPDP SCRIPT and NCPDP Telecom D.0's own Coverage Audit adapters — two
// distinct engines, two distinct format keys ("ncpdpscript"/"ncpdptelecom"),
// same structural problem, so built together in one file.
//
// Unlike CDA/HL7/FHIR, a real fhir.build sourcePath reading NCPDP-parsed
// content does NOT use a short, fixed-shape path — confirmed by grepping a
// REAL OOB migration (V258, NewRx→FHIR), every single sourcePath looks like
// "steps.parse_new_rx.step_output.parsed_ncpdp.body.pharmacy.identification.
// ncpdpid": a "steps.<step-name-derived-alias>.step_output.<outputField>.…"
// address, snake_cased throughout, where <alias> is THIS INTERFACE'S OWN
// ncpdp.parse step's own step_name (normalized via
// models.OutputNormalizer.NormalizeKey — the same rule
// transformation_pipeline_helpers.go's executeStepWithContext already uses
// for the "steps.<key>" snapshot itself, confirmed directly, not assumed).
//
// A second, equally important correction found by reading the real parser
// services directly rather than assuming: `envelope` (this package's own
// BuildInventory parameter — the message's persisted ParsedJSON, produced
// once at ingestion by services/parsers/ncpdpscript and
// services/parsers/ncpdptelecom, entirely independent of whatever the
// PIPELINE's own ncpdp.parse/ncpdptelecom.parse STEP does later) has NO
// "parsedNCPDP"/"parsedTelecom" wrapper key at all — both parser services'
// own Parse() methods write "_format"/"transactionType" (or
// "transactionCode"/"direction")/"header"/"body" (or "transmissionGroup"/
// "transactionGroups") DIRECTLY at envelope's own top level. The
// "parsedNCPDP"/"parsedTelecom" wrapper is an artifact of the SEPARATE
// pipeline STEP's own outputField config — that step re-parses (or reuses)
// the same underlying data and wraps ITS OWN copy of this identical shape
// under outputField before writing it into steps.<alias>.step_output. So
// reconstructing a real tracking key means wrapping the WHOLE envelope under
// the step's outputField key (not looking up a key inside envelope that was
// never there) before normalizing/walking it.
//
// This is a real, user-confirmed increase in scope over CDA/HL7/FHIR's own
// adapters: this one needs a DB lookup PER INTERFACE to discover which
// ncpdp.parse/ncpdptelecom.parse step (if any) that interface's own pipeline
// uses, and must reproduce — not approximate — the exact same
// camelCase-to-snake_case key transformation the real engine applies via
// models.OutputNormalizer.NormalizeStepOutput, reused directly rather than
// reimplemented, to avoid ever drifting from the real behavior.
package cdacoverage

import (
	"context"
	"database/sql"
	"fmt"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services/coverage"
	"ezhealthkonnect/services/executors"
)

// buildNCPDPStyleInventory is the pure, DB-free core: given the already-known
// step candidates and envelope (the message's own persisted ParsedJSON —
// already directly in the parser service's own {_format, ..., header, body}
// shape, see this file's own top doc comment), it builds one InventoryItem
// PER POPULATED LEAF field found under each candidate's own reconstructed
// "steps.<alias>.step_output...." address space — using
// models.OutputNormalizer.NormalizeStepOutput directly (never a
// hand-reimplemented camelCase→snake_case conversion) so the computed key
// can never drift from what the real engine would actually produce.
// skipTopLevelKeys (e.g. "header", "_format", the format's own envelope
// identifier) are excluded entirely, matching every other format's own "the
// whole envelope/header segment is administrative, exclude it upfront rather
// than classify after the fact" convention (CDA's MSH-equivalent, HL7's
// MSH/BHS/BTS/FHS/FTS, FHIR's own resourceType/bookkeeping-key exclusion).
func buildNCPDPStyleInventory(category string, envelope map[string]interface{}, steps []parseStepInfo, skipTopLevelKeys map[string]bool) []InventoryItem {
	if len(steps) == 0 {
		return nil
	}

	normalizer := models.NewOutputNormalizer()
	var items []InventoryItem
	seen := map[string]bool{} // de-dupe when multiple step candidates share the same alias/outputField combo

	skip := map[string]bool{}
	for k := range skipTopLevelKeys {
		skip[normalizer.NormalizeKey(k)] = true
	}

	for _, step := range steps {
		var synthetic map[string]interface{}
		skipDepth := 1
		if step.outputField == "__root__" {
			// __root__ merges envelope's own fields directly into the step's
			// output with no wrapper key at all — so the skip-check applies
			// at THIS level (0), not one level deeper.
			synthetic = envelope
			skipDepth = 0
		} else {
			synthetic = map[string]interface{}{step.outputField: envelope}
		}
		prefix := fmt.Sprintf("steps.%s.step_output", step.alias)
		normalized := normalizer.NormalizeStepOutput(synthetic)

		walkNCPDPLeaves(normalized, prefix, skip, skipDepth, 0, func(fullKey string) {
			if seen[fullKey] {
				return
			}
			seen[fullKey] = true
			items = append(items, InventoryItem{
				Category:     category,
				SectionKey:   fullKey,
				SectionTitle: fullKey,
				EntryIndex:   -1,
			})
		})
	}
	return items
}

// walkNCPDPLeaves recursively visits every populated scalar leaf of node,
// calling emit(fullDottedPath) for each. Maps recurse by key; arrays recurse
// by index (bracketed, matching this codebase's own established
// "arr[N]" convention elsewhere in Coverage Audit and the pipeline engine at
// large). skipKeys (already normalized) are excluded at exactly skipDepth
// levels deep from the root — 0 means "skip at this very level," used for the
// "__root__" output-field case where header/body sit directly at top level.
func walkNCPDPLeaves(node interface{}, prefix string, skipKeys map[string]bool, skipDepth, currentDepth int, emit func(string)) {
	switch v := node.(type) {
	case map[string]interface{}:
		for key, val := range v {
			if currentDepth == skipDepth && skipKeys[key] {
				continue
			}
			walkNCPDPLeaves(val, prefix+"."+key, skipKeys, skipDepth, currentDepth+1, emit)
		}
	case []interface{}:
		for i, val := range v {
			walkNCPDPLeaves(val, fmt.Sprintf("%s[%d]", prefix, i), skipKeys, skipDepth, currentDepth+1, emit)
		}
	case nil:
		// absent — nothing to record, matching every other adapter's own
		// "only what's genuinely populated" inventory philosophy.
	case string:
		if v != "" {
			emit(prefix)
		}
	default:
		// Any other populated scalar (number, bool) — always a real value.
		emit(prefix)
	}
}

// ─── NCPDP SCRIPT ────────────────────────────────────────────────────────

// ncpdpScriptSkipKeys are envelope-identification/bookkeeping fields — never
// clinical content: "_format" (pipeline plumbing, matches every other
// adapter's own convention), "transactionType" (this format's own envelope
// routing field, analogous to HL7's MSH.9 or FHIR's resourceType — both of
// which are excluded from THEIR OWN adapters' walks the same way), "header"
// (the whole <Header> element — MessageID/RelatesToMessageID/SentTime/
// To/From/sender+receiver software identifiers, all administrative per this
// feature's own plan doc, excluded wholesale rather than field-by-field,
// mirroring HL7's whole-MSH exclusion), and "messageAttrs" (the <Message>
// root element's own XML attributes — DatatypesVersion/TransportVersion/
// TransactionVersion/StructuresVersion/ECLVersion/TransactionDomain, pure
// wire-protocol-version bookkeeping, never clinical content).
//
// A real, live-verification-caught bug: the first version of this set
// omitted "messageAttrs" entirely — caught by inspecting a REAL, live
// coverage_audits row's own category_stats (not a Go-level test, which had
// no reason to include messageAttrs in its own synthetic fixture), which
// showed 6 messageAttrs.* fields listed as GENUINE GAPS. Fixed here; a
// regression test (TestNCPDPScriptAdapter_BuildInventory_ExcludesMessageAttrs)
// guards against reintroducing this.
//
// "raw" (added once ncpdp_script_parser_service.go's own Parse() started
// setting it — see that file's own doc comment on the real, live-delivery
// parser gap this closed) is excluded the same way: it's the entire original
// message text as one opaque string, never a field any real mapping rule
// would address as a single unit, so leaving it in the walk would produce a
// permanent, meaningless "gap" on every single report.
var ncpdpScriptSkipKeys = map[string]bool{"_format": true, "transactionType": true, "header": true, "messageAttrs": true, "raw": true}

type ncpdpScriptAdapter struct {
	db *sql.DB
}

func (a *ncpdpScriptAdapter) FormatKey() string { return "ncpdpscript" }

// Classify always returns false — like the FHIR/HL7 adapters, administrative
// content is excluded from the inventory entirely upfront (ncpdpScriptSkipKeys)
// rather than classified after the fact.
func (a *ncpdpScriptAdapter) Classify(path string) bool { return false }

func (a *ncpdpScriptAdapter) BuildInventory(ctx context.Context, interfaceID string, envelope map[string]interface{}, elementLevel bool, tracker *executors.CDACoverageTracker) ([]InventoryItem, error) {
	format, _ := envelope["_format"].(string)
	if format != "ncpdpscript" {
		return nil, nil // not an NCPDP SCRIPT message
	}
	transactionType, _ := envelope["transactionType"].(string)
	if transactionType == "" {
		transactionType = "NCPDP SCRIPT"
	}
	steps := resolveParseSteps(ctx, a.db, interfaceID, "ncpdp.parse", "parsedNCPDP")
	return buildNCPDPStyleInventory(transactionType, envelope, steps, ncpdpScriptSkipKeys), nil
}

// RegisterNCPDPScriptAdapter registers the NCPDP SCRIPT coverage-audit
// adapter under format key "ncpdpscript". Needs a per-interface DB lookup
// (see resolveParseSteps), so — like HL7's own adapter — it can't
// self-register via a bare init() the way FHIR's does.
func RegisterNCPDPScriptAdapter(db *sql.DB) {
	coverage.RegisterAdapter(&ncpdpScriptAdapter{db: db})
}

// ─── NCPDP Telecom D.0 ───────────────────────────────────────────────────

// ncpdpTelecomSkipKeys mirrors ncpdpScriptSkipKeys for D.0's own envelope
// shape: "_format"/"direction" are pipeline plumbing, "transactionCode" is
// this format's own envelope routing field, "header" is the fixed 56-byte
// transmission header (BIN, version, transaction code, processor control
// number, service provider identity, date of service) — entirely
// administrative per this feature's own plan doc — and "raw" (added once
// ncpdp_telecom_parser_service.go's own Parse() started setting it) is the
// entire original message text as one opaque string, excluded for the same
// reason ncpdpScriptSkipKeys excludes it.
var ncpdpTelecomSkipKeys = map[string]bool{"_format": true, "direction": true, "transactionCode": true, "header": true, "raw": true}

type ncpdpTelecomAdapter struct {
	db *sql.DB
}

func (a *ncpdpTelecomAdapter) FormatKey() string { return "ncpdptelecom" }

func (a *ncpdpTelecomAdapter) Classify(path string) bool { return false }

func (a *ncpdpTelecomAdapter) BuildInventory(ctx context.Context, interfaceID string, envelope map[string]interface{}, elementLevel bool, tracker *executors.CDACoverageTracker) ([]InventoryItem, error) {
	format, _ := envelope["_format"].(string)
	if format != "ncpdptelecom" {
		return nil, nil // not an NCPDP Telecom D.0 message
	}
	transactionCode, _ := envelope["transactionCode"].(string)
	if transactionCode == "" {
		transactionCode = "NCPDP Telecom D.0"
	}
	steps := resolveParseSteps(ctx, a.db, interfaceID, "ncpdptelecom.parse", "parsedTelecom")
	return buildNCPDPStyleInventory(transactionCode, envelope, steps, ncpdpTelecomSkipKeys), nil
}

// RegisterNCPDPTelecomAdapter registers the NCPDP Telecom D.0 coverage-audit
// adapter under format key "ncpdptelecom". Same per-interface-DB-lookup
// reasoning as RegisterNCPDPScriptAdapter above.
func RegisterNCPDPTelecomAdapter(db *sql.DB) {
	coverage.RegisterAdapter(&ncpdpTelecomAdapter{db: db})
}
