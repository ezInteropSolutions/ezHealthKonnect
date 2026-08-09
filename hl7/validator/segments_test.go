package validator

import "testing"

const oruR01WithoutPID = "MSH|^~\\&|LAB_SYS|LAB_FAC|EHR_SYS|HOSPITAL|20240101130000||ORU^R01|LAB001|P|2.5.1\r" +
	"OBR|1|ORD001|LAB001|85025^CBC^LN|||20240101125000\r" +
	"OBX|1|NM|718-7^Hemoglobin^LN||14.2|g/dL|12.0-16.0|N|||F"

func TestValidateRequiredSegments_ADTA01_MissingPID_Flagged(t *testing.T) {
	raw := "MSH|^~\\&|SENDING_APP|SENDING_FAC|RECEIVING_APP|RECEIVING_FAC|20240101120000||ADT^A01|MSG001|P|2.5.1\r" +
		"EVN|A01|20240101120000\r" +
		"PV1|1|I|WARD^101^A^Hospital|E||12345^Smith^Jane^A^MD|67890^Jones^Bob^C^MD||MED|||N|||67890^Jones^Bob^C^MD|INS|V001|||||||||||||||||||ADM|A0|20240101120000"
	msg, schema := parseFixture(t, raw)

	errs := validateRequiredSegments(schema, msg)
	if len(errs) != 1 {
		t.Fatalf("expected exactly 1 missing-segment error, got %d: %+v", len(errs), errs)
	}
	if errs[0].Segment != "PID" {
		t.Errorf("expected the missing segment to be PID, got %q", errs[0].Segment)
	}
	if errs[0].Code != "MISSING_REQUIRED_SEGMENT" {
		t.Errorf("expected code MISSING_REQUIRED_SEGMENT, got %q", errs[0].Code)
	}
}

func TestValidateRequiredSegments_ADTA01_AllPresent_NoErrors(t *testing.T) {
	msg, schema := parseFixture(t, validADTA01)
	errs := validateRequiredSegments(schema, msg)
	if len(errs) != 0 {
		t.Errorf("expected zero missing-segment errors, got: %+v", errs)
	}
}

// ORU_R01's PID sits inside the optional PATIENT group (confirmed by
// hl7/builder.RequiredSpine's own ORU_R01 test: RequiredSpine == ["OBR"]
// only), so a PID-less ORU^R01 must NOT be flagged — this is the key edge
// case RequiredSpine's transitive-AND logic exists to get right.
func TestValidateRequiredSegments_ORUR01_MissingPID_NotFlagged(t *testing.T) {
	msg, schema := parseFixture(t, oruR01WithoutPID)
	errs := validateRequiredSegments(schema, msg)
	if len(errs) != 0 {
		t.Errorf("expected zero errors — PID is optional in ORU_R01, got: %+v", errs)
	}
}

func TestValidateRequiredSegments_ORUR01_MissingOBR_Flagged(t *testing.T) {
	raw := "MSH|^~\\&|LAB_SYS|LAB_FAC|EHR_SYS|HOSPITAL|20240101130000||ORU^R01|LAB001|P|2.5.1\r" +
		"PID|1||67890^^^MRN||SMITH^JANE^B||19750320|F"
	msg, schema := parseFixture(t, raw)
	errs := validateRequiredSegments(schema, msg)
	if len(errs) != 1 || errs[0].Segment != "OBR" {
		t.Errorf("expected exactly 1 error for missing OBR, got: %+v", errs)
	}
}
