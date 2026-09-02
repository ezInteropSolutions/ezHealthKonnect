package edi

import (
	"strings"
	"testing"
)

// testSpec builds a small SYNTHETIC schema — not real 835 spec data,
// deliberately shaped like it (header, two sibling loops sharing a trigger
// segment, a nested repeating loop, an intra-segment repeat group, and a
// composite element) so the engine's generic mechanisms are all exercised
// without depending on spec-verified schema content.
func testSpec() *X12SpecDef {
	segments := map[string]*X12SegmentDef{
		"ST": {ID: "ST", Usage: "required", Elements: []*X12ElementDef{
			{Pos: "01", Key: "transactionSetIdentifierCode", DataType: TypeID, FixedValue: "TST"},
			{Pos: "02", Key: "transactionSetControlNumber", DataType: TypeAN},
		}},
		"BPR": {ID: "BPR", Usage: "required", Elements: []*X12ElementDef{
			{Pos: "01", Key: "transactionHandlingCode", DataType: TypeID},
			{Pos: "02", Key: "totalAmount", DataType: TypeN2},
		}},
		"N1": {ID: "N1", Usage: "required", Elements: []*X12ElementDef{
			{Pos: "01", Key: "entityIdentifierCode", DataType: TypeID},
			{Pos: "02", Key: "name", DataType: TypeAN},
		}},
		"LX": {ID: "LX", Usage: "required", Elements: []*X12ElementDef{
			{Pos: "01", Key: "assignedNumber", DataType: TypeN0},
		}},
		"CLP": {ID: "CLP", Usage: "required", Elements: []*X12ElementDef{
			{Pos: "01", Key: "patientControlNumber", DataType: TypeAN},
		}, Repeats: []*X12RepeatDef{
			{Key: "adjustments", StartPos: 2, GroupSize: 2, MaxGroups: 3, GroupFields: []*X12ElementDef{
				{Pos: "1", Key: "reasonCode", DataType: TypeID},
				{Pos: "2", Key: "amount", DataType: TypeN2},
			}},
		}},
		"SVC": {ID: "SVC", Usage: "situational", Elements: []*X12ElementDef{
			{Pos: "01", Key: "procedureCode", Composite: &X12CompositeDef{SubElements: []*X12ElementDef{
				{Pos: "1", Key: "qualifier", DataType: TypeID},
				{Pos: "2", Key: "code", DataType: TypeAN},
			}}},
		}},
		"SE": {ID: "SE", Usage: "required", Elements: []*X12ElementDef{
			{Pos: "01", Key: "count", DataType: TypeN0},
			{Pos: "02", Key: "control", DataType: TypeAN},
		}},
	}

	txSet := &X12TransactionSetDef{
		TransactionSetID: "TST",
		HeaderSegmentIDs: []string{"ST", "BPR"},
		Loops: []*X12LoopDef{
			{ID: "1000A", Repeat: "1", SegmentIDs: []string{"N1"}},
			{ID: "1000B", Repeat: "1", SegmentIDs: []string{"N1"}},
			{ID: "2000", Repeat: ">1", SegmentIDs: []string{"LX"}, Loops: []*X12LoopDef{
				{ID: "2100", Repeat: "1", SegmentIDs: []string{"CLP", "SVC"}},
			}},
		},
		TrailerSegmentIDs: []string{"SE"},
	}

	return &X12SpecDef{
		SpecVersion:     "TEST",
		Segments:        segments,
		TransactionSets: map[string]*X12TransactionSetDef{"TST": txSet},
	}
}

func sampleContent() string {
	segs := []string{
		"ST*TST*0001",
		"BPR*C*1250.00",
		"N1*PR*ACME PAYER",
		"N1*PE*PROVIDER GROUP",
		"LX*1",
		"CLP*PC001*A1*10.00*A2*5.00",
		"SVC*HC:99213",
		"LX*2",
		"CLP*PC002",
		"SE*9*0001",
	}
	return strings.Join(segs, "~\n") + "~\n"
}

func TestParseTransactionSet_HeaderAndSiblingLoops(t *testing.T) {
	spec := testSpec()
	result, err := ParseTransactionSet(spec, sampleContent())
	if err != nil {
		t.Fatalf("ParseTransactionSet: %v", err)
	}

	if result.TransactionSet != "TST" {
		t.Errorf("TransactionSet = %q, want TST", result.TransactionSet)
	}

	bpr, ok := result.Header["BPR"].(map[string]interface{})
	if !ok {
		t.Fatalf("header BPR missing or wrong type: %#v", result.Header["BPR"])
	}
	if bpr["totalAmount"] != "1250.00" {
		t.Errorf("BPR totalAmount = %v, want 1250.00", bpr["totalAmount"])
	}

	loopA, ok := result.Loops["1000A"].(map[string]interface{})
	if !ok {
		t.Fatalf("loop 1000A missing or wrong type: %#v", result.Loops["1000A"])
	}
	if loopA["N1"].(map[string]interface{})["name"] != "ACME PAYER" {
		t.Errorf("1000A.N1.name = %v, want ACME PAYER", loopA["N1"])
	}

	loopB, ok := result.Loops["1000B"].(map[string]interface{})
	if !ok {
		t.Fatalf("loop 1000B missing or wrong type: %#v", result.Loops["1000B"])
	}
	if loopB["N1"].(map[string]interface{})["name"] != "PROVIDER GROUP" {
		t.Errorf("1000B.N1.name = %v, want PROVIDER GROUP — sibling-loop disambiguation failed", loopB["N1"])
	}
}

func TestParseTransactionSet_NestedRepeatingLoopsAndRepeatGroup(t *testing.T) {
	spec := testSpec()
	result, err := ParseTransactionSet(spec, sampleContent())
	if err != nil {
		t.Fatalf("ParseTransactionSet: %v", err)
	}

	loop2000, ok := result.Loops["2000"].([]map[string]interface{})
	if !ok {
		t.Fatalf("loop 2000 wrong type: %#v", result.Loops["2000"])
	}
	if len(loop2000) != 2 {
		t.Fatalf("expected 2 instances of loop 2000, got %d", len(loop2000))
	}

	first2100 := loop2000[0]["loops"].(map[string]interface{})["2100"].(map[string]interface{})
	clp := first2100["CLP"].(map[string]interface{})
	if clp["patientControlNumber"] != "PC001" {
		t.Errorf("first CLP patientControlNumber = %v, want PC001", clp["patientControlNumber"])
	}
	adjustments, ok := clp["adjustments"].([]map[string]interface{})
	if !ok || len(adjustments) != 2 {
		t.Fatalf("expected 2 adjustments, got %#v", clp["adjustments"])
	}
	if adjustments[0]["reasonCode"] != "A1" || adjustments[0]["amount"] != "10.00" {
		t.Errorf("first adjustment = %#v", adjustments[0])
	}
	if adjustments[1]["reasonCode"] != "A2" || adjustments[1]["amount"] != "5.00" {
		t.Errorf("second adjustment = %#v", adjustments[1])
	}

	svc := first2100["SVC"].(map[string]interface{})
	procCode := svc["procedureCode"].(map[string]interface{})
	if procCode["qualifier"] != "HC" || procCode["code"] != "99213" {
		t.Errorf("composite procedureCode = %#v", procCode)
	}

	second2100 := loop2000[1]["loops"].(map[string]interface{})["2100"].(map[string]interface{})
	clp2 := second2100["CLP"].(map[string]interface{})
	if clp2["patientControlNumber"] != "PC002" {
		t.Errorf("second CLP patientControlNumber = %v, want PC002", clp2["patientControlNumber"])
	}
	if _, hasAdjustments := clp2["adjustments"]; hasAdjustments {
		t.Errorf("second CLP should have no adjustments, got %#v", clp2["adjustments"])
	}
	if _, hasSVC := second2100["SVC"]; hasSVC {
		t.Errorf("second 2100 instance should have no situational SVC, got %#v", second2100["SVC"])
	}
}

func TestParseTransactionSet_FlatFieldAddressing(t *testing.T) {
	spec := testSpec()
	result, err := ParseTransactionSet(spec, sampleContent())
	if err != nil {
		t.Fatalf("ParseTransactionSet: %v", err)
	}

	cases := map[string]string{
		"BPR.02":               "1250.00",
		"1000A.N1.02":          "ACME PAYER",
		"1000B.N1.02":          "PROVIDER GROUP",
		"2000[1].2100.CLP.01":  "PC001",
		"2000[2].2100.CLP.01":  "PC002",
	}
	for path, want := range cases {
		field, ok := result.Fields[path]
		if !ok {
			t.Errorf("missing flat field %q (have %d fields)", path, len(result.Fields))
			continue
		}
		if field.Value != want {
			t.Errorf("field %q = %q, want %q", path, field.Value, want)
		}
	}

	// 1000A/1000B never repeat in this message, so their addressing must
	// NOT carry a "[1]" suffix — the "don't clutter the common case" rule.
	if _, wrongPath := result.Fields["1000A[1].N1.02"]; wrongPath {
		t.Errorf("1000A should not be bracketed since it never repeats")
	}
}

func TestParseTransactionSet_RequiredSegmentMissingIsError(t *testing.T) {
	spec := testSpec()
	content := "ST*TST*0001~\nN1*PR*ACME PAYER~\nSE*9*0001~\n" // missing required BPR
	if _, err := ParseTransactionSet(spec, content); err == nil {
		t.Fatal("expected an error for missing required BPR segment, got nil")
	}
}

func TestParseTransactionSet_BareSTNoEnvelope(t *testing.T) {
	spec := testSpec()
	content := "ST*TST*0001~BPR*C*100.00~N1*PR*ACME~N1*PE*PROVIDER~SE*5*0001~"
	result, err := ParseTransactionSet(spec, content)
	if err != nil {
		t.Fatalf("ParseTransactionSet (bare ST): %v", err)
	}
	if result.EnvelopePresent {
		t.Error("EnvelopePresent should be false for bare-ST content")
	}
	if result.TransactionSet != "TST" {
		t.Errorf("TransactionSet = %q, want TST", result.TransactionSet)
	}
}
