package cdacoverage

import (
	"context"
	"testing"

	"ezhealthkonnect/services/executors"
)

// scriptEnvelope builds a synthetic envelope in the EXACT shape
// services/parsers/ncpdpscript/ncpdp_script_parser_service.go's Parse()
// method actually produces — confirmed by reading that file directly, not
// assumed — so this test exercises the real persisted-content contract, not
// a hand-invented one.
func scriptEnvelope() map[string]interface{} {
	return map[string]interface{}{
		"_format":         "ncpdpscript",
		"raw":             "<Message>...</Message>",
		"transactionType": "NewRx",
		"messageAttrs":    map[string]interface{}{"version": "2017071"},
		"header": map[string]interface{}{
			"to":        "PHARMACY01",
			"from":      "PRESCRIBER01",
			"messageID": "MSG001",
		},
		"body": map[string]interface{}{
			"pharmacy": map[string]interface{}{
				"identification": map[string]interface{}{
					"ncpdpid": "1234567",
				},
			},
			"patient": map[string]interface{}{
				"humanPatient": map[string]interface{}{
					"name": map[string]interface{}{
						"lastName": "DOE",
					},
				},
			},
		},
	}
}

func TestNCPDPScriptAdapter_BuildInventory_ExcludesHeaderAndEnvelopeBookkeeping(t *testing.T) {
	adapter := &ncpdpScriptAdapter{}
	steps := []parseStepInfo{{alias: "parse_new_rx", outputField: "parsedNCPDP"}}

	items := buildNCPDPStyleInventory("NewRx", scriptEnvelope(), steps, ncpdpScriptSkipKeys)

	for _, item := range items {
		if item.SectionKey == "steps.parse_new_rx.step_output.parsed_ncpdp.header.to" {
			t.Errorf("expected the whole 'header' branch to be excluded entirely, found %+v", item)
		}
	}
	// Sanity: real body content IS present.
	found := false
	for _, item := range items {
		if item.SectionKey == "steps.parse_new_rx.step_output.parsed_ncpdp.body.pharmacy.identification.ncpdpid" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a real body field to be in the inventory, items=%v", items)
	}
	_ = adapter
}

// TestNCPDPScriptAdapter_BuildInventory_ExcludesMessageAttrs is a regression
// guard for a real bug this feature's own LIVE verification caught (not a
// Go-level test — the synthetic fixtures here had no reason to include
// messageAttrs until this bug surfaced): a real, persisted
// coverage_audits.category_stats row showed 6 "messageAttrs.*" fields
// (DatatypesVersion/TransportVersion/etc. — the <Message> root element's own
// XML attributes, pure wire-protocol-version bookkeeping) listed as GENUINE
// GAPS, because the first version of ncpdpScriptSkipKeys omitted
// "messageAttrs" entirely.
func TestNCPDPScriptAdapter_BuildInventory_ExcludesMessageAttrs(t *testing.T) {
	steps := []parseStepInfo{{alias: "parse_new_rx", outputField: "parsedNCPDP"}}
	items := buildNCPDPStyleInventory("NewRx", scriptEnvelope(), steps, ncpdpScriptSkipKeys)

	for _, item := range items {
		if item.SectionKey == "steps.parse_new_rx.step_output.parsed_ncpdp.message_attrs.version" {
			t.Errorf("expected the whole 'messageAttrs' branch to be excluded entirely, found %+v", item)
		}
	}
}

// TestNCPDPScriptAdapter_BuildInventory_ExcludesRaw is a regression guard for
// the follow-on to the real "sourceField: raw" parser gap fixed 2026-09-27 in
// ncpdp_script_parser_service.go — once that fix landed, the newly-populated
// "raw" field would otherwise show up as a permanent, meaningless gap (the
// entire original message text as one opaque string) on every report.
func TestNCPDPScriptAdapter_BuildInventory_ExcludesRaw(t *testing.T) {
	steps := []parseStepInfo{{alias: "parse_new_rx", outputField: "parsedNCPDP"}}
	items := buildNCPDPStyleInventory("NewRx", scriptEnvelope(), steps, ncpdpScriptSkipKeys)

	for _, item := range items {
		if item.SectionKey == "steps.parse_new_rx.step_output.parsed_ncpdp.raw" {
			t.Errorf("expected 'raw' to be excluded entirely, found %+v", item)
		}
	}
}

// TestNCPDPScriptAdapter_TrackingKey_MatchesRealOOBMigrationSourcePath is the
// mandatory exact-key proof this whole feature's design doc calls for: the
// literal sourcePath string below is copied verbatim from a real, shipped OOB
// migration (database/migrations/V258__NCPDP_NewRx_To_FHIR_OOB_Pipeline_Template.sql),
// not invented. It must byte-match the TrackingKey() this adapter computes
// for the same field, given a step alias of "parse_new_rx" (the real
// interface's own ncpdp.parse step, named so that
// models.OutputNormalizer.NormalizeKey(stepName) == "parse_new_rx" — the
// exact same address the real engine's own steps.<alias>.step_output
// snapshot uses, confirmed directly in transformation_pipeline_helpers.go,
// not assumed).
//
// This test's design was corrected TWICE before being written this way: the
// first assumption (envelope has a "parsedNCPDP" wrapper key, mirroring the
// pipeline step's own outputField) was wrong — reading
// ncpdp_script_parser_service.go directly showed the message's real,
// persisted ParsedJSON has "header"/"body" at ITS OWN top level, with no
// wrapper at all; the wrapper is an artifact of the SEPARATE ncpdp.parse
// PIPELINE STEP re-wrapping that same shape under its own outputField before
// it reaches steps.<alias>.step_output. The second correction was realizing
// this resolver chain depends on resolveJSONPathValue's tracker hook
// checking BOTH data["_coverageTracker"] AND data["message"]["_coverageTracker"]
// (see field_utils.go's resolveCoverageTracker) — without that fix, a real
// fhir.build step reading a "steps.X.step_output...." sourcePath against the
// raw, still-"message"-wrapped inputData would never see the tracker at all,
// since the tracker lives one level down inside "message", not as a sibling
// of "steps". Both gaps are fixed; this test proves the INVENTORY side of
// the contract (the tracker side is proven by
// TestResolveCoverageTracker_FindsMessageWrappedTracker in
// services/executors/field_utils_coverage_test.go).
func TestNCPDPScriptAdapter_TrackingKey_MatchesRealOOBMigrationSourcePath(t *testing.T) {
	const realSourcePathFromV258 = "steps.parse_new_rx.step_output.parsed_ncpdp.body.pharmacy.identification.ncpdpid"

	steps := []parseStepInfo{{alias: "parse_new_rx", outputField: "parsedNCPDP"}}
	items := buildNCPDPStyleInventory("NewRx", scriptEnvelope(), steps, ncpdpScriptSkipKeys)

	var match *InventoryItem
	for i := range items {
		if items[i].TrackingKey() == realSourcePathFromV258 {
			match = &items[i]
		}
	}
	if match == nil {
		t.Fatalf("expected an inventory item whose TrackingKey() exactly equals the real V258 sourcePath %q, got items=%v", realSourcePathFromV258, items)
	}

	// Now prove the OTHER half: a real tracker.Record() call using that exact
	// literal path (what resolveJSONPathValue would record for a real
	// fhir.build sourcePath read) is recognized as "touched" for this item.
	tracker := executors.NewCDACoverageTracker()
	tracker.Record(realSourcePathFromV258)
	if !tracker.Touched(match.TrackingKey()) {
		t.Errorf("expected the inventory item's own TrackingKey() to match a real tracker.Record() call using the literal V258 sourcePath")
	}
}

func TestNCPDPScriptAdapter_BuildInventory_NoConfiguredParseStep_ReturnsNil(t *testing.T) {
	items := buildNCPDPStyleInventory("NewRx", scriptEnvelope(), nil, ncpdpScriptSkipKeys)
	if items != nil {
		t.Errorf("expected nil inventory when no ncpdp.parse step is configured on this interface, got %v", items)
	}
}

func TestNCPDPScriptAdapter_BuildInventory_RootOutputField_SkipsAtTopLevel(t *testing.T) {
	steps := []parseStepInfo{{alias: "parse_new_rx", outputField: "__root__"}}
	items := buildNCPDPStyleInventory("NewRx", scriptEnvelope(), steps, ncpdpScriptSkipKeys)

	for _, item := range items {
		if item.SectionKey == "steps.parse_new_rx.step_output.header.to" {
			t.Errorf("expected 'header' to be excluded even in __root__ mode, found %+v", item)
		}
	}
	found := false
	for _, item := range items {
		if item.SectionKey == "steps.parse_new_rx.step_output.body.pharmacy.identification.ncpdpid" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected __root__ mode to place body fields one level shallower (no outputField wrapper), items=%v", items)
	}
}

func TestNCPDPScriptAdapter_BuildInventory_WrongFormat_ReturnsNil(t *testing.T) {
	adapter := &ncpdpScriptAdapter{}
	items, err := adapter.BuildInventory(context.Background(), "", map[string]interface{}{"_format": "ncpdptelecom"}, false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if items != nil {
		t.Errorf("expected nil for a non-ncpdpscript envelope, got %v", items)
	}
}

// ─── NCPDP Telecom D.0 ───────────────────────────────────────────────────

func telecomEnvelope() map[string]interface{} {
	return map[string]interface{}{
		"_format":         "ncpdptelecom",
		"raw":             "raw D.0 transmission bytes",
		"transactionCode": "B1",
		"direction":       "request",
		"header": map[string]interface{}{
			"binNumber": "610011",
			"version":   "D0",
		},
		"transmissionGroup": map[string]interface{}{
			"patient": map[string]interface{}{
				"dateOfBirth": "19800101",
			},
		},
		"transactionGroups": []interface{}{
			map[string]interface{}{
				"claim": map[string]interface{}{
					"prescriptionServiceReferenceNumber": "0000001",
				},
			},
		},
	}
}

func TestNCPDPTelecomAdapter_BuildInventory_ExcludesHeaderAndEnvelopeBookkeeping(t *testing.T) {
	steps := []parseStepInfo{{alias: "parse_b1_request", outputField: "parsedTelecom"}}
	items := buildNCPDPStyleInventory("B1", telecomEnvelope(), steps, ncpdpTelecomSkipKeys)

	for _, item := range items {
		if item.SectionKey == "steps.parse_b1_request.step_output.parsed_telecom.header.bin_number" {
			t.Errorf("expected the whole 'header' branch to be excluded, found %+v", item)
		}
	}

	wantKeys := map[string]bool{
		"steps.parse_b1_request.step_output.parsed_telecom.transmission_group.patient.date_of_birth": false,
		"steps.parse_b1_request.step_output.parsed_telecom.transaction_groups[0].claim.prescription_service_reference_number": false,
	}
	for _, item := range items {
		if _, ok := wantKeys[item.SectionKey]; ok {
			wantKeys[item.SectionKey] = true
		}
	}
	for key, ok := range wantKeys {
		if !ok {
			t.Errorf("expected inventory item %q, got items=%v", key, items)
		}
	}
}

// TestNCPDPTelecomAdapter_BuildInventory_ExcludesRaw mirrors
// TestNCPDPScriptAdapter_BuildInventory_ExcludesRaw for D.0's own parser gap
// fix (ncpdp_telecom_parser_service.go, 2026-09-27).
func TestNCPDPTelecomAdapter_BuildInventory_ExcludesRaw(t *testing.T) {
	steps := []parseStepInfo{{alias: "parse_b1_request", outputField: "parsedTelecom"}}
	items := buildNCPDPStyleInventory("B1", telecomEnvelope(), steps, ncpdpTelecomSkipKeys)

	for _, item := range items {
		if item.SectionKey == "steps.parse_b1_request.step_output.parsed_telecom.raw" {
			t.Errorf("expected 'raw' to be excluded entirely, found %+v", item)
		}
	}
}

func TestNCPDPTelecomAdapter_BuildInventory_WrongFormat_ReturnsNil(t *testing.T) {
	adapter := &ncpdpTelecomAdapter{}
	items, err := adapter.BuildInventory(context.Background(), "", map[string]interface{}{"_format": "ncpdpscript"}, false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if items != nil {
		t.Errorf("expected nil for a non-ncpdptelecom envelope, got %v", items)
	}
}

func TestNCPDPTelecomAdapter_FormatKey(t *testing.T) {
	if (&ncpdpTelecomAdapter{}).FormatKey() != "ncpdptelecom" {
		t.Errorf("unexpected FormatKey")
	}
	if (&ncpdpScriptAdapter{}).FormatKey() != "ncpdpscript" {
		t.Errorf("unexpected FormatKey")
	}
}
