package validator

import "testing"

func TestValidateCardinality_ADTA01_DuplicatePID_Flagged(t *testing.T) {
	raw := "MSH|^~\\&|SENDING_APP|SENDING_FAC|RECEIVING_APP|RECEIVING_FAC|20240101120000||ADT^A01|MSG001|P|2.5.1\r" +
		"EVN|A01|20240101120000\r" +
		"PID|1||12345^^^MRN||DOE^JOHN^A||19800115|M\r" +
		"PID|2||99999^^^MRN||DOE^JOHN^A||19800115|M\r" +
		"PV1|1|I|WARD^101^A^Hospital|E||12345^Smith^Jane^A^MD|67890^Jones^Bob^C^MD||MED"
	msg, schema := parseFixture(t, raw)

	errs := validateCardinality(schema, msg)
	if len(errs) != 1 || errs[0].Segment != "PID" {
		t.Fatalf("expected exactly 1 cardinality error for duplicate PID, got: %+v", errs)
	}
	if errs[0].Code != "CARDINALITY_VIOLATION" {
		t.Errorf("expected code CARDINALITY_VIOLATION, got %q", errs[0].Code)
	}
}

func TestValidateCardinality_ADTA01_SinglePID_NoError(t *testing.T) {
	msg, schema := parseFixture(t, validADTA01)
	errs := validateCardinality(schema, msg)
	if len(errs) != 0 {
		t.Errorf("expected zero cardinality errors, got: %+v", errs)
	}
}

// ORU_R01's OBX repeats via its immediate parent OBSERVATION group's
// repeat="*" — builder.SchemaTree's CanRepeat already models this
// correctly; a naive check against OBX's own repeat flag (typically "1")
// would false-positive here.
func TestValidateCardinality_ORUR01_MultipleOBX_NoError(t *testing.T) {
	raw := "MSH|^~\\&|LAB_SYS|LAB_FAC|EHR_SYS|HOSPITAL|20240101130000||ORU^R01|LAB001|P|2.5.1\r" +
		"PID|1||67890^^^MRN||SMITH^JANE^B||19750320|F\r" +
		"OBR|1|ORD001|LAB001|85025^CBC^LN|||20240101125000\r" +
		"OBX|1|NM|718-7^Hemoglobin^LN||14.2|g/dL|12.0-16.0|N|||F\r" +
		"OBX|2|NM|787-2^MCV^LN||89|fL|80-100|N|||F"
	msg, schema := parseFixture(t, raw)

	errs := validateCardinality(schema, msg)
	if len(errs) != 0 {
		t.Errorf("expected zero cardinality errors for repeating OBX, got: %+v", errs)
	}
}
