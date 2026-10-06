// External test package (astm_test, not astm) — engine_test.go needs both
// astm itself and astm/builder (for the round-trip test), and astm/builder
// itself imports astm, which would be a real import cycle if this file were
// compiled as part of the internal astm package (an "internal" _test.go file
// is merged into the package it tests). Every identifier referenced below is
// already exported for exactly this reason.
package astm_test

import (
	"strings"
	"testing"

	"ezhealthkonnect/astm"
	"ezhealthkonnect/astm/builder"
)

func testLoader(t *testing.T) *astm.ASTMSchemaLoader {
	t.Helper()
	loader, err := astm.NewASTMSchemaLoader("./schemas/e1394_97")
	if err != nil {
		t.Fatalf("NewASTMSchemaLoader: %v", err)
	}
	return loader
}

func sampleMessage() string {
	records := []string{
		`H|\^&|MSGCTRL001||ANALYZER-1^1.0^SN12345|||||LIS||P|E1394-97|20231004120000`,
		`P|1||MRN12345||Doe^Jane||19800101|F`,
		`O|1|SPEC001||GLU|R`,
		`R|1|GLU|95|mg/dL|70-110|N||F`,
		`L|1|N`,
	}
	return strings.Join(records, "\r") + "\r"
}

func TestSchemaLoader_LoadsAllSevenRecordTypes(t *testing.T) {
	loader := testLoader(t)
	for _, id := range []string{"H", "P", "O", "R", "C", "Q", "L"} {
		if rec := loader.GetRecord(id); rec == nil {
			t.Errorf("GetRecord(%q) returned nil", id)
		}
	}
	if msg := loader.GetMessage("generic_lab_result"); msg == nil {
		t.Error("GetMessage(\"generic_lab_result\") returned nil")
	}
}

func TestDetectDelimiters_FromRealHRecord(t *testing.T) {
	records := astm.SplitRecords(sampleMessage())
	d, found := astm.DetectDelimiters(records)
	if !found {
		t.Fatal("expected H record to be found")
	}
	if d.Field != "|" || d.Repeat != "\\" || d.Component != "^" || d.Escape != "&" {
		t.Errorf("unexpected delimiters: %+v", d)
	}
}

func TestDetectDelimiters_NoHRecord_FallsBackToDefaults(t *testing.T) {
	d, found := astm.DetectDelimiters([]string{"P|1||MRN12345"})
	if found {
		t.Error("expected found=false when no H record present")
	}
	if d.Field != "|" || d.Repeat != "\\" || d.Component != "^" || d.Escape != "&" {
		t.Errorf("unexpected default delimiters: %+v", d)
	}
}

func TestParseMessage_RealSample(t *testing.T) {
	loader := testLoader(t)
	result, err := astm.ParseMessage(loader.Spec(), "generic_lab_result", sampleMessage())
	if err != nil {
		t.Fatalf("ParseMessage: %v", err)
	}

	if got := result.Header["messageControlID"]; got != "MSGCTRL001" {
		t.Errorf("Header.messageControlID = %v, want MSGCTRL001", got)
	}
	sender, ok := result.Header["senderNameID"].(map[string]interface{})
	if !ok {
		t.Fatalf("Header.senderNameID not decoded as component map: %#v", result.Header["senderNameID"])
	}
	if sender["manufacturer"] != "ANALYZER-1" || sender["instrumentID"] != "SN12345" {
		t.Errorf("Header.senderNameID decoded incorrectly: %#v", sender)
	}

	if len(result.PatientBlocks) != 1 {
		t.Fatalf("expected 1 patient block, got %d", len(result.PatientBlocks))
	}
	patient := result.PatientBlocks[0]
	if patient["labPatientID"] != "MRN12345" {
		t.Errorf("patient.labPatientID = %v, want MRN12345", patient["labPatientID"])
	}
	name, ok := patient["patientName"].(map[string]interface{})
	if !ok || name["lastName"] != "Doe" || name["firstName"] != "Jane" {
		t.Errorf("patient.patientName decoded incorrectly: %#v", patient["patientName"])
	}
	if patient["sex"] != "F" {
		t.Errorf("patient.sex = %v, want F", patient["sex"])
	}

	orders, ok := patient["orders"].([]map[string]interface{})
	if !ok || len(orders) != 1 {
		t.Fatalf("expected 1 order, got %#v", patient["orders"])
	}
	order := orders[0]
	if order["specimenID"] != "SPEC001" {
		t.Errorf("order.specimenID = %v, want SPEC001", order["specimenID"])
	}

	results, ok := order["results"].([]map[string]interface{})
	if !ok || len(results) != 1 {
		t.Fatalf("expected 1 result, got %#v", order["results"])
	}
	r := results[0]
	testID, ok := r["universalTestID"].(map[string]interface{})
	if !ok || testID["testID"] != "GLU" {
		t.Errorf("result.universalTestID decoded incorrectly: %#v", r["universalTestID"])
	}
	value, ok := r["dataValue"].(map[string]interface{})
	if !ok || value["value"] != "95" {
		t.Errorf("result.dataValue decoded incorrectly: %#v", r["dataValue"])
	}
	if r["units"] != "mg/dL" {
		t.Errorf("result.units = %v, want mg/dL", r["units"])
	}
	if r["resultStatus"] != "F" {
		t.Errorf("result.resultStatus = %v, want F", r["resultStatus"])
	}

	if result.Trailer["terminationCode"] != "N" {
		t.Errorf("Trailer.terminationCode = %v, want N", result.Trailer["terminationCode"])
	}

	// SegmentPosition/RecordInstances sanity — 5 records total (H,P,O,R,L).
	if len(result.RecordInstances) != 5 {
		t.Errorf("expected 5 RecordInstances, got %d", len(result.RecordInstances))
	}
}

func TestDetectDelimiters_NonDefaultDelimiters(t *testing.T) {
	// A real-world instrument that declares different special characters —
	// e.g. "!" as the field delimiter, "$" as repeat, "@" as component,
	// "%" as escape — must be read from the H record's own declaration, not
	// assumed to always be the standard "|\^&".
	records := astm.SplitRecords("H!$@%!MSGCTRL1!!SENDER\rL!1!N\r")
	d, found := astm.DetectDelimiters(records)
	if !found {
		t.Fatal("expected the H record to be found")
	}
	if d.Field != "!" || d.Repeat != "$" || d.Component != "@" || d.Escape != "%" {
		t.Errorf("unexpected delimiters: %+v", d)
	}
}

func TestParseMessage_MultiplePatientBlocks_EachKeepsItsOwnOrders(t *testing.T) {
	loader := testLoader(t)
	content := `H|\^&|M1||A^1^S|||||LIS||P|E1394-97|20231004120000` + "\r" +
		"P|1||MRN-A||Doe^Jane||19800101|F\r" +
		"O|1|SPEC-A||GLU\r" +
		"R|1|GLU|90|mg/dL|70-110|N||F\r" +
		"P|2||MRN-B||Smith^Bob||19750505|M\r" +
		"O|1|SPEC-B||BMP\r" +
		"R|1|BMP|140|mmol/L|135-145|N||F\r" +
		"L|1|N\r"

	result, err := astm.ParseMessage(loader.Spec(), "generic_lab_result", content)
	if err != nil {
		t.Fatalf("ParseMessage: %v", err)
	}
	if len(result.PatientBlocks) != 2 {
		t.Fatalf("expected 2 patient blocks, got %d", len(result.PatientBlocks))
	}

	patientA := result.PatientBlocks[0]
	if patientA["labPatientID"] != "MRN-A" {
		t.Errorf("patient A labPatientID = %v, want MRN-A", patientA["labPatientID"])
	}
	ordersA := patientA["orders"].([]map[string]interface{})
	if len(ordersA) != 1 || ordersA[0]["specimenID"] != "SPEC-A" {
		t.Errorf("patient A's own orders got cross-contaminated: %#v", ordersA)
	}

	patientB := result.PatientBlocks[1]
	if patientB["labPatientID"] != "MRN-B" {
		t.Errorf("patient B labPatientID = %v, want MRN-B", patientB["labPatientID"])
	}
	ordersB := patientB["orders"].([]map[string]interface{})
	if len(ordersB) != 1 || ordersB[0]["specimenID"] != "SPEC-B" {
		t.Errorf("patient B's own orders got cross-contaminated: %#v", ordersB)
	}
}

func TestParseMessage_CommentUnderOrder_AttachesToThatOrderOnly(t *testing.T) {
	loader := testLoader(t)
	content := `H|\^&|M1||A^1^S|||||LIS||P|E1394-97|20231004120000` + "\r" +
		"P|1||MRN1||Doe^Jane||19800101|F\r" +
		"O|1|SPEC1||GLU\r" +
		"C|1|I|Specimen hemolyzed|G\r" + // a comment specifically about this order's own result
		"R|1|GLU|90|mg/dL|70-110|N||F\r" +
		"L|1|N\r"

	result, err := astm.ParseMessage(loader.Spec(), "generic_lab_result", content)
	if err != nil {
		t.Fatalf("ParseMessage: %v", err)
	}

	patient := result.PatientBlocks[0]
	if patient["comments"] != nil {
		t.Errorf("the comment should attach to the order, not the patient block (patient[\"comments\"] should be absent entirely when empty): %#v", patient["comments"])
	}
	if len(result.HeaderComments) != 0 {
		t.Error("the comment should attach to the order, not the header")
	}

	order := patient["orders"].([]map[string]interface{})[0]
	comments, ok := order["comments"].([]map[string]interface{})
	if !ok || len(comments) != 1 {
		t.Fatalf("expected exactly 1 comment attached to the order, got %#v", order["comments"])
	}
	if comments[0]["commentText"] != "Specimen hemolyzed" {
		t.Errorf("comment text = %v, want %q", comments[0]["commentText"], "Specimen hemolyzed")
	}
}

func TestParseMessage_MissingHRecord_Errors(t *testing.T) {
	loader := testLoader(t)
	_, err := astm.ParseMessage(loader.Spec(), "generic_lab_result", "P|1||MRN12345\rL|1|N\r")
	if err == nil {
		t.Error("expected an error when content has no H record")
	}
}

func TestParseMessage_MissingLRecord_Errors(t *testing.T) {
	loader := testLoader(t)
	_, err := astm.ParseMessage(loader.Spec(), "generic_lab_result", `H|\^&|M1||A^1^S|||||LIS||P|E1394-97|20231004120000`+"\r")
	if err == nil {
		t.Error("expected an error when content has no L record")
	}
}

func TestBuildDocument_RoundTripsRealSample(t *testing.T) {
	loader := testLoader(t)
	spec := loader.Spec()

	parsed, err := astm.ParseMessage(spec, "generic_lab_result", sampleMessage())
	if err != nil {
		t.Fatalf("ParseMessage: %v", err)
	}

	built, err := builder.BuildDocument(spec, builder.BuildInput{
		MessageProfile: "generic_lab_result",
		Header:         parsed.Header,
		HeaderComments: parsed.HeaderComments,
		PatientBlocks:  parsed.PatientBlocks,
		QueryBlocks:    parsed.QueryBlocks,
		Trailer:        parsed.Trailer,
	})
	if err != nil {
		t.Fatalf("BuildDocument: %v", err)
	}

	reparsed, err := astm.ParseMessage(spec, "generic_lab_result", built)
	if err != nil {
		t.Fatalf("ParseMessage(built): %v\nbuilt content:\n%q", err, built)
	}

	if reparsed.Header["messageControlID"] != "MSGCTRL001" {
		t.Errorf("round-tripped messageControlID = %v, want MSGCTRL001", reparsed.Header["messageControlID"])
	}
	if len(reparsed.PatientBlocks) != 1 {
		t.Fatalf("round-tripped patient block count = %d, want 1", len(reparsed.PatientBlocks))
	}
	rp := reparsed.PatientBlocks[0]
	if rp["labPatientID"] != "MRN12345" {
		t.Errorf("round-tripped labPatientID = %v, want MRN12345", rp["labPatientID"])
	}
	orders, ok := rp["orders"].([]map[string]interface{})
	if !ok || len(orders) != 1 {
		t.Fatalf("round-tripped orders = %#v", rp["orders"])
	}
	results, ok := orders[0]["results"].([]map[string]interface{})
	if !ok || len(results) != 1 {
		t.Fatalf("round-tripped results = %#v", orders[0]["results"])
	}
	value, ok := results[0]["dataValue"].(map[string]interface{})
	if !ok || value["value"] != "95" {
		t.Errorf("round-tripped dataValue = %#v, want value=95", results[0]["dataValue"])
	}
	if reparsed.Trailer["terminationCode"] != "N" {
		t.Errorf("round-tripped terminationCode = %v, want N", reparsed.Trailer["terminationCode"])
	}
}

func TestChecksum_KnownValue(t *testing.T) {
	// "1H|\^&|<CR><ETX>" style frame content — a minimal, hand-computed
	// cross-check of the mod-256/hex-uppercase mechanics themselves
	// (independent of any real captured frame, since no readable source
	// found during this feature's Pre-Phase gate showed real checksum byte
	// values — see Checksum's own doc comment).
	content := []byte{'1', 'A', 'B', astm.CR, astm.ETX}
	sum := int('1') + int('A') + int('B') + int(astm.CR) + int(astm.ETX)
	want := sum % 256
	got := astm.Checksum(content)
	gotInt := 0
	for _, c := range got {
		gotInt = gotInt*16 + hexVal(byte(c))
	}
	if gotInt != want {
		t.Errorf("Checksum(%v) = %s (int %d), want int %d", content, got, gotInt, want)
	}
	if len(got) != 2 {
		t.Errorf("Checksum result %q should always be exactly 2 hex characters", got)
	}
}

func hexVal(c byte) int {
	switch {
	case c >= '0' && c <= '9':
		return int(c - '0')
	case c >= 'A' && c <= 'F':
		return int(c-'A') + 10
	}
	return -1
}

// recoverPanic turns a panic inside fn into a test failure with a clear
// message, instead of crashing the whole test binary — the actual assertion
// these adversarial-input tests care about.
func mustNotPanic(t *testing.T, name string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("%s: ParseMessage panicked on adversarial input: %v", name, r)
		}
	}()
	fn()
}

func TestParseMessage_EmptyContent_ErrorsNotPanics(t *testing.T) {
	loader := testLoader(t)
	mustNotPanic(t, "empty content", func() {
		_, err := astm.ParseMessage(loader.Spec(), "generic_lab_result", "")
		if err == nil {
			t.Error("expected an error for empty content")
		}
	})
}

func TestParseMessage_GarbageBinaryContent_ErrorsNotPanics(t *testing.T) {
	loader := testLoader(t)
	garbage := string([]byte{0x00, 0xFF, 0xFE, 0x01, 0x02, 0x80, 0x81, '\r', 0x00, 0x00})
	mustNotPanic(t, "garbage binary content", func() {
		_, err := astm.ParseMessage(loader.Spec(), "generic_lab_result", garbage)
		if err == nil {
			t.Error("expected an error for garbage binary content with no real H record")
		}
	})
}

func TestParseMessage_ExtremelyLongFieldValue_DoesNotPanic(t *testing.T) {
	loader := testLoader(t)
	hugeComment := strings.Repeat("X", 500_000) // half a megabyte in one field — a hostile or corrupted frame
	content := `H|\^&|M1||A^1^S|||||LIS||P|E1394-97|20231004120000` + "\r" +
		"C|1|I|" + hugeComment + "|G\r" +
		"L|1|N\r"

	mustNotPanic(t, "extremely long field value", func() {
		result, err := astm.ParseMessage(loader.Spec(), "generic_lab_result", content)
		if err != nil {
			t.Fatalf("expected a huge-but-well-formed field to still parse: %v", err)
		}
		if len(result.HeaderComments) != 1 {
			t.Fatalf("expected 1 header comment, got %d", len(result.HeaderComments))
		}
		got, _ := result.HeaderComments[0]["commentText"].(string)
		if len(got) != len(hugeComment) {
			t.Errorf("comment text length = %d, want %d", len(got), len(hugeComment))
		}
	})
}

func TestParseMessage_ManyRepeatDelimitedItems_DoesNotPanic(t *testing.T) {
	loader := testLoader(t)
	var items []string
	for i := 0; i < 1000; i++ {
		items = append(items, "TUBE")
	}
	manyRepeats := strings.Join(items, "\\") // 1000 repeat-delimited instrumentSpecimenID values on one O record
	content := `H|\^&|M1||A^1^S|||||LIS||P|E1394-97|20231004120000` + "\r" +
		"P|1||MRN1||Doe^Jane||19800101|F\r" +
		"O|1|SPEC1|" + manyRepeats + "|GLU\r" +
		"L|1|N\r"

	mustNotPanic(t, "many repeat-delimited items", func() {
		result, err := astm.ParseMessage(loader.Spec(), "generic_lab_result", content)
		if err != nil {
			t.Fatalf("ParseMessage: %v", err)
		}
		order := result.PatientBlocks[0]["orders"].([]map[string]interface{})[0]
		got, ok := order["instrumentSpecimenID"].([]interface{})
		if !ok {
			t.Fatalf("expected instrumentSpecimenID to decode as []interface{}, got %T", order["instrumentSpecimenID"])
		}
		if len(got) != 1000 {
			t.Errorf("expected 1000 repeat items, got %d", len(got))
		}
	})
}

func TestParseMessage_UnterminatedMessage_ErrorsNotPanics(t *testing.T) {
	loader := testLoader(t)
	// An H record with no L (Terminator) at all — a connection that dropped
	// mid-transmission, a real and common failure mode for a serial/TCP link.
	mustNotPanic(t, "unterminated message", func() {
		_, err := astm.ParseMessage(loader.Spec(), "generic_lab_result",
			`H|\^&|M1||A^1^S|||||LIS||P|E1394-97|20231004120000`+"\r"+
				"P|1||MRN1||Doe^Jane||19800101|F\r")
		if err == nil {
			t.Error("expected an error for a message with no L (Terminator) record")
		}
	})
}

func TestNextFrameNumber_WrapsAtSeven(t *testing.T) {
	if astm.NextFrameNumber(7) != 0 {
		t.Errorf("NextFrameNumber(7) = %d, want 0", astm.NextFrameNumber(7))
	}
	if astm.NextFrameNumber(3) != 4 {
		t.Errorf("NextFrameNumber(3) = %d, want 4", astm.NextFrameNumber(3))
	}
}
