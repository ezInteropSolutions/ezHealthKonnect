package cdacoverage

import (
	"context"
	"testing"

	"ezhealthkonnect/services/executors"
)

// edi835Envelope builds a synthetic envelope in the EXACT shape
// services/parsers/edix12/edi_x12_parser_service.go's Parse() method actually
// produces — confirmed by reading that file and edi/loop_engine.go's own
// ParseTransactionSet directly, not assumed. Field names/nesting (ST inside
// header, SE inside trailer, 1000A as a bare non-repeating loop, 2000/2100/
// 2110 as repeating array loops) mirror the real 835-to-FHIR OOB template's
// own "Derive 835 Claim Context" script and "Build PaymentReconciliation"
// step, both read directly from database/migrations/
// V247__EDI_835_To_FHIR_Per_Claim_EOB.sql during this adapter's own design.
func edi835Envelope() map[string]interface{} {
	return map[string]interface{}{
		"_format":         "edi",
		"raw":             "ISA*00*...",
		"transactionSet":  "835",
		"envelopePresent": true,
		"interchange": map[string]interface{}{
			"senderId":         "SENDERID",
			"receiverId":       "RECEIVERID",
			"stControlNumber":  "0001",
			"gsControlNumber":  "1",
			"isaControlNumber": "000000001",
		},
		"header": map[string]interface{}{
			"ST": map[string]interface{}{
				"transactionSetIdentifierCode": "835",
				"transactionSetControlNumber":  "0001",
			},
			"BPR": map[string]interface{}{
				"paymentEffectiveDate":            "20260115",
				"totalActualProviderPaymentAmount": "150.00",
			},
			"TRN": map[string]interface{}{
				"checkOrEFTTraceNumber": "TRACE123456",
			},
		},
		"loops": map[string]interface{}{
			"1000A": map[string]interface{}{
				"N1": map[string]interface{}{
					"name": "ABC INSURANCE",
				},
			},
			"2000": []interface{}{
				map[string]interface{}{
					"loops": map[string]interface{}{
						"2100": []interface{}{
							map[string]interface{}{
								"CLP": map[string]interface{}{
									"patientControlNumber": "PCN001",
									"claimPaymentAmount":   "150.00",
								},
								"NM1": []interface{}{
									map[string]interface{}{
										"entityIdentifierCode":      "QC",
										"nameLastOrOrganizationName": "DOE",
									},
								},
							},
						},
					},
				},
			},
		},
		"trailer": map[string]interface{}{
			"SE": map[string]interface{}{
				"numberOfIncludedSegments": "10",
			},
			"PLB": map[string]interface{}{
				"adjustmentAmount": "5.00",
			},
		},
	}
}

func TestEDIAdapter_BuildInventory_ExcludesEnvelopeBookkeeping(t *testing.T) {
	steps := []parseStepInfo{{alias: "parse_835_json", outputField: "parsedEDI"}}
	items := ediInventoryForTest(edi835Envelope(), steps)

	byPath := map[string]bool{}
	for _, item := range items {
		byPath[item.SectionKey] = true
	}

	for _, excluded := range []string{
		"parsedEDI.interchange.senderId",
		"parsedEDI.header.ST.transactionSetIdentifierCode",
		"parsedEDI.trailer.SE.numberOfIncludedSegments",
	} {
		if byPath[excluded] {
			t.Errorf("expected %q to be excluded (envelope/control bookkeeping), but it was in the inventory", excluded)
		}
	}
	for _, included := range []string{
		"parsedEDI.header.BPR.paymentEffectiveDate",
		"parsedEDI.header.TRN.checkOrEFTTraceNumber",
		"parsedEDI.trailer.PLB.adjustmentAmount",
		"parsedEDI.loops.1000A.N1.name",
		"parsedEDI.loops.2000[0].loops.2100[0].CLP.patientControlNumber",
	} {
		if !byPath[included] {
			t.Errorf("expected %q to be a real inventory item, items=%v", included, byPath)
		}
	}
}

// TestEDIAdapter_TrackingKey_Tier2_MatchesRealOOBMigrationSourcePath is the
// mandatory exact-key proof for Tier 2 (bare "message.<outputField>...."):
// the literal sourcePath string below is copied verbatim from the real,
// shipped edi-835-to-fhir-sftp template's own "Build PaymentReconciliation"
// fhir.build step (database/migrations, "Build PaymentReconciliation" field
// config), not invented. It resolves through resolveJSONPathValue's plain
// dotted walk against the wrapped pipeline envelope (inputData["message"]
// genuinely has "parsedEDI" on it — see this file's own top doc comment on
// why), and is recorded via resolveCoverageTracker's message-wrapped fallback
// (the same fix built for NCPDP's own Phase 3).
func TestEDIAdapter_TrackingKey_Tier2_MatchesRealOOBMigrationSourcePath(t *testing.T) {
	const realSourcePath = "message.parsedEDI.header.TRN.checkOrEFTTraceNumber"

	tracker := executors.NewCDACoverageTracker()
	tracker.Record(realSourcePath)

	steps := []parseStepInfo{{alias: "parse_835_json", outputField: "parsedEDI"}}
	items := ediInventoryForTestWithTracker(edi835Envelope(), steps, tracker)

	var match *InventoryItem
	for i := range items {
		if items[i].SectionKey == "parsedEDI.header.TRN.checkOrEFTTraceNumber" {
			match = &items[i]
		}
	}
	if match == nil {
		t.Fatalf("expected an inventory item for the TRN field, items=%v", items)
	}
	if !tracker.Touched(match.TrackingKey()) {
		t.Errorf("expected the adapter to reconcile the real tier-2 sourcePath hit onto its own canonical key %q, tracker snapshot=%v", match.TrackingKey(), tracker.Snapshot())
	}
}

// TestEDIAdapter_TrackingKey_Tier3_ScriptTracking is the mandatory exact-key
// proof for Tier 3 (goja script tracking, no "message." prefix at all): the
// real "Derive 835 Claim Context" script (same migration) reads
// "input.message.parsedEDI.header.BPR.paymentEffectiveDate" — the
// coverage_script_tracking.go wrapper records this as the BARE
// "parsedEDI.header.BPR.paymentEffectiveDate" (no "message." prefix), since
// it wraps inputData["message"] directly, not the whole script input. See
// this file's own top doc comment for the full mechanism trace.
func TestEDIAdapter_TrackingKey_Tier3_ScriptTracking(t *testing.T) {
	const scriptTrackedKey = "parsedEDI.header.BPR.paymentEffectiveDate"

	tracker := executors.NewCDACoverageTracker()
	tracker.Record(scriptTrackedKey)

	steps := []parseStepInfo{{alias: "parse_835_json", outputField: "parsedEDI"}}
	items := ediInventoryForTestWithTracker(edi835Envelope(), steps, tracker)

	var match *InventoryItem
	for i := range items {
		if items[i].SectionKey == scriptTrackedKey {
			match = &items[i]
		}
	}
	if match == nil {
		t.Fatalf("expected an inventory item for the BPR payment date field, items=%v", items)
	}
	if !tracker.Touched(match.TrackingKey()) {
		t.Errorf("expected the adapter to reconcile the real tier-3 script-tracked hit onto its own canonical key %q", match.TrackingKey())
	}
}

// TestEDIAdapter_TrackingKey_Tier1_StepsAddressing proves the defensive third
// tier: an interface whose fhir.build step reads a prior step's output via
// the explicit "steps.<alias>.step_output...." address (NCPDP's V258 own,
// only, convention — not directly observed in the 835 template itself, which
// uses tier 2/3 instead, but a real, proven-working mechanism this adapter
// must also support since some other EDI template could use it).
func TestEDIAdapter_TrackingKey_Tier1_StepsAddressing(t *testing.T) {
	const stepsKey = "steps.parse_835_json.step_output.parsed_edi.header.bpr.payment_effective_date"

	tracker := executors.NewCDACoverageTracker()
	tracker.Record(stepsKey)

	steps := []parseStepInfo{{alias: "parse_835_json", outputField: "parsedEDI"}}
	items := ediInventoryForTestWithTracker(edi835Envelope(), steps, tracker)

	var match *InventoryItem
	for i := range items {
		if items[i].SectionKey == "parsedEDI.header.BPR.paymentEffectiveDate" {
			match = &items[i]
		}
	}
	if match == nil {
		t.Fatalf("expected an inventory item for the BPR payment date field, items=%v", items)
	}
	if !tracker.Touched(match.TrackingKey()) {
		t.Errorf("expected the adapter to reconcile the tier-1 steps.X.step_output hit onto its own canonical key %q", match.TrackingKey())
	}
}

func TestEDIAdapter_BuildInventory_UntouchedFieldIsAGenuineGap(t *testing.T) {
	tracker := executors.NewCDACoverageTracker()
	// Touch nothing.
	steps := []parseStepInfo{{alias: "parse_835_json", outputField: "parsedEDI"}}
	items := ediInventoryForTestWithTracker(edi835Envelope(), steps, tracker)

	var match *InventoryItem
	for i := range items {
		if items[i].SectionKey == "parsedEDI.trailer.PLB.adjustmentAmount" {
			match = &items[i]
		}
	}
	if match == nil {
		t.Fatalf("expected an inventory item for PLB.adjustmentAmount, items=%v", items)
	}
	if tracker.Touched(match.TrackingKey()) {
		t.Errorf("expected an untouched field to remain a genuine gap, not spuriously marked touched")
	}
}

func TestEDIAdapter_BuildInventory_WrongFormat_ReturnsNil(t *testing.T) {
	adapter := &ediAdapter{}
	items, err := adapter.BuildInventory(context.Background(), "", map[string]interface{}{"_format": "ncpdpscript"}, false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if items != nil {
		t.Errorf("expected nil for a non-edi envelope, got %v", items)
	}
}

func TestEDIAdapter_FormatKey(t *testing.T) {
	if (&ediAdapter{}).FormatKey() != "edi" {
		t.Errorf("unexpected FormatKey")
	}
}

// ─── test-only helpers, mirroring the pure-core/DB-lookup split every other
// adapter in this package already uses (buildNCPDPStyleInventory being the
// pure core called directly, bypassing the DB, in ncpdp_adapter_test.go) ───

func ediInventoryForTest(envelope map[string]interface{}, steps []parseStepInfo) []InventoryItem {
	return ediInventoryForTestWithTracker(envelope, steps, nil)
}

func ediInventoryForTestWithTracker(envelope map[string]interface{}, steps []parseStepInfo, tracker *executors.CDACoverageTracker) []InventoryItem {
	transactionSet, _ := envelope["transactionSet"].(string)
	if transactionSet == "" {
		transactionSet = "EDI X12"
	}
	return buildEDIInventoryPure(transactionSet, envelope, steps, tracker)
}
