package builder

import (
	"strings"
	"testing"

	"ezhealthkonnect/edi"
)

// testSpec mirrors edi/loop_engine_test.go's own synthetic schema (same
// shape, duplicated here since it's small and test-only — see that file's
// own comment for why) plus an envelope, needed only on the build/write side.
func testSpec() *edi.X12SpecDef {
	segments := map[string]*edi.X12SegmentDef{
		"ST": {ID: "ST", Usage: "required", Elements: []*edi.X12ElementDef{
			{Pos: "01", Key: "transactionSetIdentifierCode", DataType: edi.TypeID, FixedValue: "TST"},
			{Pos: "02", Key: "transactionSetControlNumber", DataType: edi.TypeAN},
		}},
		"BPR": {ID: "BPR", Usage: "required", Elements: []*edi.X12ElementDef{
			{Pos: "01", Key: "transactionHandlingCode", DataType: edi.TypeID},
			{Pos: "02", Key: "totalAmount", DataType: edi.TypeN2},
		}},
		"N1": {ID: "N1", Usage: "required", Elements: []*edi.X12ElementDef{
			{Pos: "01", Key: "entityIdentifierCode", DataType: edi.TypeID},
			{Pos: "02", Key: "name", DataType: edi.TypeAN},
		}},
		"LX": {ID: "LX", Usage: "required", Elements: []*edi.X12ElementDef{
			{Pos: "01", Key: "assignedNumber", DataType: edi.TypeN0},
		}},
		"CLP": {ID: "CLP", Usage: "required", Elements: []*edi.X12ElementDef{
			{Pos: "01", Key: "patientControlNumber", DataType: edi.TypeAN},
		}, Repeats: []*edi.X12RepeatDef{
			{Key: "adjustments", StartPos: 2, GroupSize: 2, MaxGroups: 3, GroupFields: []*edi.X12ElementDef{
				{Pos: "1", Key: "reasonCode", DataType: edi.TypeID},
				{Pos: "2", Key: "amount", DataType: edi.TypeN2},
			}},
		}},
		"SE": {ID: "SE", Usage: "required", Elements: []*edi.X12ElementDef{
			{Pos: "01", Key: "count", DataType: edi.TypeN0},
			{Pos: "02", Key: "control", DataType: edi.TypeAN},
		}},
	}

	txSet := &edi.X12TransactionSetDef{
		TransactionSetID: "TST",
		HeaderSegmentIDs: []string{"ST", "BPR"},
		Loops: []*edi.X12LoopDef{
			{ID: "1000A", Repeat: "1", SegmentIDs: []string{"N1"}},
			{ID: "1000B", Repeat: "1", SegmentIDs: []string{"N1"}},
			{ID: "2000", Repeat: ">1", SegmentIDs: []string{"LX"}, Loops: []*edi.X12LoopDef{
				{ID: "2100", Repeat: "1", SegmentIDs: []string{"CLP"}},
			}},
		},
		TrailerSegmentIDs: []string{"SE"},
	}

	envelope := &edi.X12EnvelopeDef{
		ISA: []*edi.X12ElementDef{
			{Pos: "01", Key: "authInfoQualifier", FixedValue: "00", MaxLength: 2},
			{Pos: "02", Key: "authInfo", MaxLength: 10},
			{Pos: "03", Key: "securityInfoQualifier", FixedValue: "00", MaxLength: 2},
			{Pos: "04", Key: "securityInfo", MaxLength: 10},
			{Pos: "05", Key: "senderIdQualifier", FixedValue: "ZZ", MaxLength: 2},
			{Pos: "06", Key: "senderId", MaxLength: 15},
			{Pos: "07", Key: "receiverIdQualifier", FixedValue: "ZZ", MaxLength: 2},
			{Pos: "08", Key: "receiverId", MaxLength: 15},
			{Pos: "09", Key: "date", MaxLength: 6},
			{Pos: "10", Key: "time", MaxLength: 4},
			{Pos: "11", Key: "repetitionSeparator", FixedValue: "^", MaxLength: 1},
			{Pos: "12", Key: "versionNumber", FixedValue: "00501", MaxLength: 5},
			{Pos: "13", Key: "isaControlNumber", MaxLength: 9},
			{Pos: "14", Key: "ackRequested", FixedValue: "0", MaxLength: 1},
			{Pos: "15", Key: "usageIndicator", MaxLength: 1},
			{Pos: "16", Key: "componentSeparator", FixedValue: ":", MaxLength: 1},
		},
		GS: []*edi.X12ElementDef{
			{Pos: "01", Key: "functionalIDCode", FixedValue: "HP"},
			{Pos: "02", Key: "senderId"},
			{Pos: "03", Key: "receiverId"},
			{Pos: "04", Key: "date"},
			{Pos: "05", Key: "time"},
			{Pos: "06", Key: "gsControlNumber"},
			{Pos: "07", Key: "responsibleAgencyCode", FixedValue: "X"},
			{Pos: "08", Key: "versionNumber", FixedValue: "005010X221A1"},
		},
	}

	return &edi.X12SpecDef{
		SpecVersion:     "TEST",
		Segments:        segments,
		TransactionSets: map[string]*edi.X12TransactionSetDef{"TST": txSet},
		Envelope:        envelope,
	}
}

func TestBuildDocument_ISAIsExactly106CharsAndReparses(t *testing.T) {
	spec := testSpec()
	input := BuildInput{
		TransactionSet: "TST",
		Interchange: map[string]interface{}{
			"senderId": "SENDERID", "receiverId": "RECEIVERID",
		},
		Header: map[string]interface{}{
			"BPR": map[string]interface{}{"transactionHandlingCode": "C", "totalAmount": "1250.00"},
		},
		Loops: map[string]interface{}{
			"1000A": map[string]interface{}{"N1": map[string]interface{}{"entityIdentifierCode": "PR", "name": "ACME PAYER"}},
			"1000B": map[string]interface{}{"N1": map[string]interface{}{"entityIdentifierCode": "PE", "name": "PROVIDER GROUP"}},
			"2000": []interface{}{
				map[string]interface{}{
					"LX": map[string]interface{}{"assignedNumber": "1"},
					"loops": map[string]interface{}{
						"2100": map[string]interface{}{"CLP": map[string]interface{}{
							"patientControlNumber": "PC001",
							"adjustments": []interface{}{
								map[string]interface{}{"reasonCode": "A1", "amount": "10.00"},
							},
						}},
					},
				},
			},
		},
	}

	out, err := BuildDocument(spec, input)
	if err != nil {
		t.Fatalf("BuildDocument: %v", err)
	}
	if !strings.HasPrefix(out, "ISA") {
		t.Fatalf("output doesn't start with ISA: %q", out[:min(40, len(out))])
	}

	isaEnd := strings.Index(out, "~\n")
	if isaEnd != 105 {
		t.Errorf("ISA segment length = %d, want 105 (106 with terminator)", isaEnd)
	}

	result, err := edi.ParseTransactionSet(spec, out)
	if err != nil {
		t.Fatalf("re-parsing built document: %v\noutput was:\n%s", err, out)
	}
	if !result.EnvelopePresent {
		t.Error("EnvelopePresent should be true after re-parsing a built document with a real ISA envelope")
	}
	if result.Interchange["senderId"] != "SENDERID" {
		t.Errorf("re-parsed senderId = %v, want SENDERID", result.Interchange["senderId"])
	}
}



func TestBuildDocument_RoundTripFromParse(t *testing.T) {
	spec := testSpec()
	original := strings.Join([]string{
		"ST*TST*0001",
		"BPR*C*1250.00",
		"N1*PR*ACME PAYER",
		"N1*PE*PROVIDER GROUP",
		"LX*1",
		"CLP*PC001*A1*10.00*A2*5.00",
		"LX*2",
		"CLP*PC002",
		"SE*9*0001",
	}, "~\n") + "~\n"

	parsed, err := edi.ParseTransactionSet(spec, original)
	if err != nil {
		t.Fatalf("parsing original: %v", err)
	}

	input := BuildInput{
		TransactionSet: parsed.TransactionSet,
		Interchange:    parsed.Interchange,
		Header:         parsed.Header,
		Loops:          parsed.Loops,
	}

	built, err := BuildDocument(spec, input)
	if err != nil {
		t.Fatalf("BuildDocument: %v", err)
	}

	reparsed, err := edi.ParseTransactionSet(spec, built)
	if err != nil {
		t.Fatalf("re-parsing built document: %v\nbuilt was:\n%s", err, built)
	}

	loopA := reparsed.Loops["1000A"].(map[string]interface{})["N1"].(map[string]interface{})
	if loopA["name"] != "ACME PAYER" {
		t.Errorf("round-tripped 1000A.N1.name = %v, want ACME PAYER", loopA["name"])
	}
	loopB := reparsed.Loops["1000B"].(map[string]interface{})["N1"].(map[string]interface{})
	if loopB["name"] != "PROVIDER GROUP" {
		t.Errorf("round-tripped 1000B.N1.name = %v, want PROVIDER GROUP", loopB["name"])
	}

	loop2000 := reparsed.Loops["2000"].([]map[string]interface{})
	if len(loop2000) != 2 {
		t.Fatalf("round-tripped loop 2000 count = %d, want 2", len(loop2000))
	}
	clp1 := loop2000[0]["loops"].(map[string]interface{})["2100"].(map[string]interface{})["CLP"].(map[string]interface{})
	if clp1["patientControlNumber"] != "PC001" {
		t.Errorf("round-tripped first CLP patientControlNumber = %v, want PC001", clp1["patientControlNumber"])
	}
	adjustments := clp1["adjustments"].([]map[string]interface{})
	if len(adjustments) != 2 || adjustments[0]["reasonCode"] != "A1" || adjustments[1]["amount"] != "5.00" {
		t.Errorf("round-tripped adjustments = %#v", adjustments)
	}
}
