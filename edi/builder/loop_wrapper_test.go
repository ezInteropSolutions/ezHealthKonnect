package builder

import (
	"strings"
	"testing"

	"ezhealthkonnect/edi"
)

// wrapperTestSpec is a SYNTHETIC schema exercising X12LoopDef.Wrapper — a
// single Start/End segment pair (X12's own generic "LS"/"LE" loop header/
// trailer) bracketing a WHOLE repeating loop set, rather than each
// instance's own leading/trailing segments (SegmentIDs/TrailerSegmentIDs
// already cover that shape). This is the wire shape 271's own loop 2120
// needs — see X12LoopDef.Wrapper's own doc comment. Isolated from this
// file's sibling testSpec() (document_builder_test.go) so it can't
// interfere with that suite's own exact-output assertions.
func wrapperTestSpec() *edi.X12SpecDef {
	segments := map[string]*edi.X12SegmentDef{
		"ST": {ID: "ST", Usage: "required", Elements: []*edi.X12ElementDef{
			{Pos: "01", Key: "transactionSetIdentifierCode", DataType: edi.TypeID, FixedValue: "TST"},
			{Pos: "02", Key: "transactionSetControlNumber", DataType: edi.TypeAN},
		}},
		"N1": {ID: "N1", Usage: "required", MaxUse: ">1", Elements: []*edi.X12ElementDef{
			{Pos: "01", Key: "entityIdentifierCode", DataType: edi.TypeID},
			{Pos: "02", Key: "name", DataType: edi.TypeAN},
		}},
		"SE": {ID: "SE", Usage: "required", Elements: []*edi.X12ElementDef{
			{Pos: "01", Key: "count", DataType: edi.TypeN0},
			{Pos: "02", Key: "control", DataType: edi.TypeAN},
		}},
	}

	txSet := &edi.X12TransactionSetDef{
		TransactionSetID: "TST",
		HeaderSegmentIDs: []string{"ST"},
		Loops: []*edi.X12LoopDef{
			// An ordinary, unwrapped loop sharing the SAME trigger segment
			// (N1) as the wrapped loop below — proves loopMatchTrigger
			// correctly tells them apart (this one's own trigger is "N1"
			// itself; the wrapped one's is its Wrapper.Start, "LS").
			{ID: "1000", Repeat: "1", SegmentIDs: []string{"N1"}},
			{
				ID: "3000", Repeat: ">1", SegmentIDs: []string{"N1"},
				Wrapper: &edi.X12LoopWrapperDef{Start: "LS", End: "LE"},
			},
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

func TestBuildDocument_WrapperLoop_RoundTrip(t *testing.T) {
	spec := wrapperTestSpec()
	input := BuildInput{
		TransactionSet: "TST",
		Interchange: map[string]interface{}{
			"senderId": "SENDERID", "receiverId": "RECEIVERID",
		},
		Loops: map[string]interface{}{
			"1000": map[string]interface{}{"N1": map[string]interface{}{"entityIdentifierCode": "PR", "name": "ACME PAYER"}},
			"3000": []interface{}{
				map[string]interface{}{"N1": map[string]interface{}{"entityIdentifierCode": "41", "name": "RELATED A"}},
				map[string]interface{}{"N1": map[string]interface{}{"entityIdentifierCode": "41", "name": "RELATED B"}},
			},
		},
	}

	out, err := BuildDocument(spec, input)
	if err != nil {
		t.Fatalf("BuildDocument: %v", err)
	}
	if !strings.Contains(out, "LS*3000~") {
		t.Errorf("output should contain the wrapper Start segment LS*3000~, got:\n%s", out)
	}
	if !strings.Contains(out, "LE*3000~") {
		t.Errorf("output should contain the wrapper End segment LE*3000~, got:\n%s", out)
	}

	lsIdx := strings.Index(out, "LS*3000~")
	leIdx := strings.Index(out, "LE*3000~")
	relatedAIdx := strings.Index(out, "RELATED A")
	relatedBIdx := strings.Index(out, "RELATED B")
	if !(lsIdx >= 0 && lsIdx < relatedAIdx && relatedAIdx < relatedBIdx && relatedBIdx < leIdx) {
		t.Fatalf("expected LS < RELATED A < RELATED B < LE, got LS@%d A@%d B@%d LE@%d in:\n%s",
			lsIdx, relatedAIdx, relatedBIdx, leIdx, out)
	}

	result, err := edi.ParseTransactionSet(spec, out)
	if err != nil {
		t.Fatalf("re-parsing built document: %v\noutput was:\n%s", err, out)
	}

	loop1000, ok := result.Loops["1000"].(map[string]interface{})
	if !ok {
		t.Fatalf("loop 1000 missing or wrong type: %#v", result.Loops["1000"])
	}
	if loop1000["N1"].(map[string]interface{})["name"] != "ACME PAYER" {
		t.Errorf("1000.N1.name = %v, want ACME PAYER", loop1000["N1"])
	}

	loop3000, ok := result.Loops["3000"].([]map[string]interface{})
	if !ok {
		t.Fatalf("loop 3000 (wrapper) missing or wrong type: %#v", result.Loops["3000"])
	}
	if len(loop3000) != 2 {
		t.Fatalf("expected 2 instances of wrapped loop 3000, got %d", len(loop3000))
	}
	if loop3000[0]["N1"].(map[string]interface{})["name"] != "RELATED A" {
		t.Errorf("3000[0].N1.name = %v, want RELATED A", loop3000[0]["N1"])
	}
	if loop3000[1]["N1"].(map[string]interface{})["name"] != "RELATED B" {
		t.Errorf("3000[1].N1.name = %v, want RELATED B", loop3000[1]["N1"])
	}
}

func TestParseTransactionSet_WrapperLoop_MissingEndSegmentIsError(t *testing.T) {
	spec := wrapperTestSpec()
	// LS present but LE omitted entirely — a genuinely malformed bracket.
	content := "ST*TST*0001~LS*3000~N1*41*RELATED A~SE*3*0001~"
	if _, err := edi.ParseTransactionSet(spec, content); err == nil {
		t.Fatal("expected an error for a wrapper loop missing its required End segment, got nil")
	}
}

func TestParseTransactionSet_WrapperLoop_EmptyBracketProducesNoInstances(t *testing.T) {
	spec := wrapperTestSpec()
	// A wrapper loop that legitimately has zero instances still carries its
	// own empty LS...LE bracket per X12's own generic loop-header/trailer
	// convention — proves the mechanism doesn't require at least one
	// instance to correctly consume the bracket, and that a present-but-
	// empty bracket doesn't leave a spurious "3000" key behind.
	content := "ST*TST*0001~LS*3000~LE*3000~SE*3*0001~"
	result, err := edi.ParseTransactionSet(spec, content)
	if err != nil {
		t.Fatalf("ParseTransactionSet (empty wrapper bracket): %v", err)
	}
	if _, present := result.Loops["3000"]; present {
		t.Errorf("expected no 3000 entry for an empty bracket, got %#v", result.Loops["3000"])
	}
}
