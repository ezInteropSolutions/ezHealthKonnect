// services/cda_coverage/edi_adapter.go
//
// EDI X12's own Coverage Audit adapter — the hardest of the four formats in
// this generalization effort, because REAL EDI OOB templates use THREE
// genuinely different addressing conventions for the SAME underlying parsed
// content, not one, confirmed by reading real, shipped migration config
// directly (database/migrations/V247__EDI_835_To_FHIR_Per_Claim_EOB.sql and
// its own predecessors) rather than assuming NCPDP's single-tier pattern
// would simply carry over unchanged:
//
//  1. "steps.<parse_alias>.step_output.<normalizedOutputField>.<snake_path>"
//     — a fhir.build sourcePath addressing a prior step's OWN step_output
//     snapshot directly (NCPDP's V258 NewRx template's own, only, tier).
//     Snake_cased throughout, via models.OutputNormalizer.NormalizeStepOutput.
//  2. "message.<outputField>.<original_case_path>" — a fhir.build sourcePath
//     reading edi.parse's own output through the RUNNING MESSAGE ENVELOPE
//     directly (e.g. "message.parsedEDI.header.TRN.checkOrEFTTraceNumber",
//     copied verbatim from the real 835-to-FHIR template's own
//     "Build PaymentReconciliation" step). This works because
//     transformation_pipeline_helpers.go's executeStepWithContext has a
//     GENERIC "preserve every other top-level field the step computed" loop
//     (confirmed by reading it directly, not assumed from NCPDP's own
//     Phase-3 investigation, which stopped short of finding this) that
//     copies edi.parse's bare outputField key (e.g. "parsedEDI") straight
//     into execCtx.Message — so a later step's inputData["message"] (==
//     execCtx.Message) genuinely has "parsedEDI" on it, reachable via a
//     PLAIN dotted sourcePath through resolveJSONPathValue's generic walk
//     (no "steps." prefix needed at all). ORIGINAL field-name casing (no
//     snake_casing — NormalizeStepOutput is never involved on this path).
//  3. Bare "<outputField>.<original_case_path>" (no "message." prefix) — an
//     enrichment.script DERIVE step's own internal read of
//     "input.message.parsedEDI...." (confirmed via the real 835 template's
//     own "Derive 835 Claim Context" script, and via the near-identical,
//     already-documented 837P/837I derive-script gotcha in CLAUDE.md).
//     Tracked by the goja coverage-tracking wrapper (coverage_script_tracking.go),
//     which wraps inputData["message"] directly — so the recorded path never
//     carries the "message." prefix a raw sourcePath string would (see that
//     file's own doc comment). Same original-casing rule as tier 2 — no
//     normalization is ever applied to a script's own property reads.
//
// This is THE format with the heaviest enrichment.script usage in this whole
// codebase (837P/837I/278/834/276/277 all derive-reshape claim/service-line
// content before fhir.build ever sees it) — tier 3 is where most of the real
// coverage signal for those transaction sets will actually come from, not
// tier 1 or 2.
//
// Because a single real field could be addressed via ANY of these 3 forms
// depending on which template/step actually reads it, BuildInventory computes
// all 3 candidate keys per real field and, if the tracker shows ANY of them
// touched, re-records the hit onto ONE canonical key (tier 3's own bare form)
// before returning — producing exactly one InventoryItem per real field
// regardless of which convention a given interface's pipeline happens to use.
// This mirrors hl7_adapter.go's own "adapter self-records into the tracker"
// precedent, for the same reason: no single literal tracker key can be
// asserted as *the* right one across every real template shape.
package cdacoverage

import (
	"context"
	"database/sql"
	"fmt"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services/coverage"
	"ezhealthkonnect/services/executors"
)

// ediAdminTopLevelKeys are envelope-identification/bookkeeping fields excluded
// from the walk entirely: "_format" (pipeline plumbing, matches every other
// adapter's own convention), "raw" (the entire original X12 text as one
// opaque string, added once edi_x12_parser_service.go's own Parse() started
// setting it — see that file's own doc comment on the real, live-delivery
// parser gap this closed), "transactionSet"/"envelopePresent" (this format's
// own envelope-identification fields, analogous to HL7's MSH.9 or FHIR's
// resourceType), and "interchange" (the ISA/GS-level control data —
// sender/receiver IDs, control numbers, usage indicator — pure
// interchange-envelope bookkeeping, never clinical/business content, the
// EDI-level equivalent of HL7's whole-MSH exclusion).
var ediAdminTopLevelKeys = map[string]bool{
	"_format": true, "raw": true, "transactionSet": true,
	"envelopePresent": true, "interchange": true,
}

// ediEnvelopeSegmentKeys are pure envelope-control segments that appear
// WITHIN header/trailer alongside genuine business content — confirmed
// directly from edi/loop_engine.go's own ParseTransactionSet, which walks
// txSet.HeaderSegmentIDs STARTING FROM the ST token itself (so header["ST"]
// is populated, duplicating what interchange["stControlNumber"] already
// captures more directly) and txSet.TrailerSegmentIDs ending at SE (so
// trailer["SE"] is likewise populated). Unlike HL7's whole-MSH exclusion or
// NCPDP's whole-header exclusion, EDI's own header/trailer are a genuine MIX
// — 835's own header also carries BPR (payment amount/date)/TRN (trace
// number)/CUR/REF/DTM, and trailer carries PLB (provider-level balance,
// real financial adjustment content) — so only ST/SE, never the whole
// branch, are excluded here.
var ediEnvelopeSegmentKeys = map[string]bool{"ST": true, "SE": true}

type ediAdapter struct {
	db *sql.DB
}

func (a *ediAdapter) FormatKey() string { return "edi" }

// Classify always returns false — administrative content is excluded from
// the inventory entirely upfront (ediAdminTopLevelKeys/ediEnvelopeSegmentKeys)
// rather than classified after the fact, matching every other field-level
// adapter in this package.
func (a *ediAdapter) Classify(path string) bool { return false }

// BuildInventory walks envelope's own header/loops/trailer (excluding
// interchange and the ST/SE envelope-control segments) and, for each real,
// populated leaf field, reconciles all 3 real addressing conventions named in
// this file's own doc comment into exactly one InventoryItem.
func (a *ediAdapter) BuildInventory(ctx context.Context, interfaceID string, envelope map[string]interface{}, elementLevel bool, tracker *executors.CDACoverageTracker) ([]InventoryItem, error) {
	format, _ := envelope["_format"].(string)
	if format != "edi" {
		return nil, nil // not an EDI X12 message
	}
	transactionSet, _ := envelope["transactionSet"].(string)
	if transactionSet == "" {
		transactionSet = "EDI X12"
	}

	steps := resolveParseSteps(ctx, a.db, interfaceID, "edi.parse", "parsedEDI")
	if len(steps) == 0 {
		return nil, nil
	}

	return buildEDIInventoryPure(transactionSet, envelope, steps, tracker), nil
}

// buildEDIInventoryPure is the pure, DB-free core: given the already-known
// step candidates, envelope, and (optionally nil) tracker, it walks
// header/loops/trailer and reconciles the 3 real addressing conventions (see
// this file's own top doc comment) into exactly one InventoryItem per real
// field. Split out from BuildInventory so tests can exercise it directly
// without a database, mirroring buildNCPDPStyleInventory's identical split in
// ncpdp_adapter.go.
func buildEDIInventoryPure(transactionSet string, envelope map[string]interface{}, steps []parseStepInfo, tracker *executors.CDACoverageTracker) []InventoryItem {
	normalizer := models.NewOutputNormalizer()
	var items []InventoryItem
	seen := map[string]bool{}

	sections := []struct {
		key  string
		skip map[string]bool
	}{
		{"header", ediEnvelopeSegmentKeys},
		{"loops", nil},
		{"trailer", ediEnvelopeSegmentKeys},
	}

	for _, step := range steps {
		for _, section := range sections {
			sub, ok := envelope[section.key].(map[string]interface{})
			if !ok {
				continue
			}
			walkEDILeaves(sub, section.key, section.key, section.skip, 0, normalizer, func(rawPath, normPath string) {
				bareKey := step.outputField + "." + rawPath
				if seen[bareKey] {
					return
				}
				seen[bareKey] = true

				if tracker != nil {
					messageKey := "message." + bareKey
					stepsKey := fmt.Sprintf("steps.%s.step_output.%s.%s",
						step.alias, normalizer.NormalizeKey(step.outputField), normPath)
					if tracker.Touched(bareKey) || tracker.Touched(messageKey) || tracker.Touched(stepsKey) {
						tracker.Record(bareKey)
					}
				}

				items = append(items, InventoryItem{
					Category:     transactionSet,
					SectionKey:   bareKey,
					SectionTitle: bareKey,
					EntryIndex:   -1,
				})
			})
		}
	}
	return items
}

// walkEDILeaves recursively visits every populated scalar leaf of node,
// building BOTH the raw (original field-name casing, as the parser actually
// produced it) and normalized (models.OutputNormalizer-snake_cased) path in
// lockstep, calling emit(rawPath, normPath) for each. Building both in one
// pass — rather than walking twice against a separately-normalized copy —
// avoids ever needing to "zip" two independently-ordered map iterations back
// together. skipKeys (raw, un-normalized) are excluded at exactly depth 0
// relative to node's own root — used to drop the ST/SE envelope-control
// segments that appear as DIRECT children of header/trailer, never at any
// deeper level (a real field happening to be named "ST" three levels down
// inside a loop is not this segment and must not be excluded).
func walkEDILeaves(node interface{}, rawPrefix, normPrefix string, skipKeys map[string]bool, depth int, normalizer *models.OutputNormalizer, emit func(rawPath, normPath string)) {
	switch v := node.(type) {
	case map[string]interface{}:
		for key, val := range v {
			if depth == 0 && skipKeys[key] {
				continue
			}
			normKey := normalizer.NormalizeKey(key)
			walkEDILeaves(val, rawPrefix+"."+key, normPrefix+"."+normKey, skipKeys, depth+1, normalizer, emit)
		}
	case []interface{}:
		for i, val := range v {
			idx := fmt.Sprintf("[%d]", i)
			walkEDILeaves(val, rawPrefix+idx, normPrefix+idx, skipKeys, depth+1, normalizer, emit)
		}
	case nil:
		// absent — nothing to record, matching every other adapter's own
		// "only what's genuinely populated" inventory philosophy.
	case string:
		if v != "" {
			emit(rawPrefix, normPrefix)
		}
	default:
		// Any other populated scalar (number, bool) — always a real value.
		emit(rawPrefix, normPrefix)
	}
}

// RegisterEDIAdapter registers the EDI X12 coverage-audit adapter under
// format key "edi". Needs a per-interface DB lookup (see resolveParseSteps),
// so — like HL7's and NCPDP's own adapters — it can't self-register via a
// bare init() the way FHIR's does.
func RegisterEDIAdapter(db *sql.DB) {
	coverage.RegisterAdapter(&ediAdapter{db: db})
}
