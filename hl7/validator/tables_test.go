package validator

import (
	"testing"

	"ezhealthkonnect/hl7"
)

func TestCheckTableBinding_ValidAndInvalidCode(t *testing.T) {
	values := map[string]string{"F": "Female", "M": "Male", "O": "Other", "U": "Unknown"}

	if e := checkTableBinding("PID", "PID.8", "", "0001", values, "M", true, hl7.SeverityWarning); e != nil {
		t.Errorf("expected no error for a valid code, got %+v", e)
	}
	e := checkTableBinding("PID", "PID.8", "", "0001", values, "Z", true, hl7.SeverityWarning)
	if e == nil {
		t.Fatal("expected an error for an invalid code, got none")
	}
	if e.Code != "TABLE_VALUE_INVALID" || e.Segment != "PID" || e.Field != "PID.8" {
		t.Errorf("unexpected error shape: %+v", e)
	}
}

// A TableId with an empty Values map is a confirmed-legitimate user-defined
// table (e.g. HL7 table 0010, Physician ID) with no fixed code set in the
// standard — must be silently skipped regardless of value.
func TestCheckTableBinding_EmptyValues_SilentlySkipped(t *testing.T) {
	if e := checkTableBinding("PV1", "PV1.7", "", "0010", map[string]string{}, "ANYTHING", true, hl7.SeverityWarning); e != nil {
		t.Errorf("expected empty-Values table to be silently skipped, got %+v", e)
	}
	if e := checkTableBinding("PV1", "PV1.7", "", "0010", nil, "ANYTHING", true, hl7.SeverityWarning); e != nil {
		t.Errorf("expected nil-Values table to be silently skipped, got %+v", e)
	}
}

func TestCheckTableBinding_NoTableId_Skipped(t *testing.T) {
	if e := checkTableBinding("PID", "PID.5", "", "", nil, "DOE", true, hl7.SeverityWarning); e != nil {
		t.Errorf("expected fields with no TableId to be skipped, got %+v", e)
	}
}

func TestCheckTableBinding_NoValue_Skipped(t *testing.T) {
	values := map[string]string{"M": "Male"}
	if e := checkTableBinding("PID", "PID.8", "", "0001", values, "", false, hl7.SeverityWarning); e != nil {
		t.Errorf("expected an absent value to be skipped, got %+v", e)
	}
}

// Integration-style: PID.8 (Administrative Sex) is confirmed to carry
// tableId "0001" with a real, non-empty code->display map in the compiled
// v2.5.1 ADT_A01 schema.
func TestValidateTableBindings_ADTA01_PID8_RealSchema(t *testing.T) {
	msg, schema := parseFixture(t, validADTA01)
	if errs := validateTableBindings(schema, msg, LevelStandard); len(errs) != 0 {
		t.Errorf("expected zero table errors for a valid PID.8 code, got: %+v", errs)
	}

	raw := "MSH|^~\\&|SENDING_APP|SENDING_FAC|RECEIVING_APP|RECEIVING_FAC|20240101120000||ADT^A01|MSG001|P|2.5.1\r" +
		"EVN|A01|20240101120000\r" +
		"PID|1||12345^^^MRN||DOE^JOHN^A||19800115|Z\r" +
		"PV1|1|I|WARD^101^A^Hospital|E||12345^Smith^Jane^A^MD|67890^Jones^Bob^C^MD||MED"
	badMsg, badSchema := parseFixture(t, raw)
	errs := validateTableBindings(badSchema, badMsg, LevelStandard)
	if len(errs) == 0 {
		t.Fatal("expected a table-binding error for an invalid PID.8 code against the real schema")
	}
	found := false
	for _, e := range errs {
		if e.Field == "PID.8" {
			found = true
			if e.Severity != hl7.SeverityWarning {
				t.Errorf("expected WARNING at Standard level, got %q", e.Severity)
			}
		}
	}
	if !found {
		t.Errorf("expected an error keyed to PID.8, got: %+v", errs)
	}
}

// A composite coded field (CWE) is checked by its code, the first component,
// in every repetition; table 0396's HL7nnnn and 99zzz patterns are codes.
func TestTableCodes(t *testing.T) {
	cases := []struct {
		in        string
		composite bool
		want      []string
	}{
		{"204^Unknown key identifier^HL70357", true, []string{"204"}},
		{"204^x~207^y", true, []string{"204", "207"}},
		{"M", false, []string{"M"}},
		{"A~B", false, []string{"A", "B"}},
	}
	for _, c := range cases {
		got := tableCodes(c.in, c.composite)
		if len(got) != len(c.want) {
			t.Fatalf("tableCodes(%q) = %q", c.in, got)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("tableCodes(%q) = %q, want %q", c.in, got, c.want)
			}
		}
	}
	values := map[string]string{"ISO3166": "Country codes"}
	for _, v := range []string{"HL70357", "HL70396", "99ABC", "ISO3166"} {
		if e := checkTableBinding("ERR", "ERR.3", "ERR.3.3", "0396", values, v, true, hl7.SeverityError); e != nil {
			t.Errorf("%s refused: %+v", v, e)
		}
	}
	for _, v := range []string{"HL7357", "HL703570", "99", "XYZ"} {
		if e := checkTableBinding("ERR", "ERR.3", "ERR.3.3", "0396", values, v, true, hl7.SeverityError); e == nil {
			t.Errorf("%s accepted", v)
		}
	}
}

// End to end on the real schema: an IHE-style ERR-3 is valid at Strict; an
// unknown error code still isn't.
func TestValidateTableBindings_ERR3_CodedValue(t *testing.T) {
	base := "MSH|^~\\&|A|B|C|D|20261111080000||ACK^A01^ACK|X1|P|2.5\rMSA|AE|Y1\rERR||PID^1^3|"
	msg, schema := parseFixture(t, base+"204^Unknown key identifier^HL70357|E")
	if errs := validateTableBindings(schema, msg, LevelStrict); len(errs) != 0 {
		t.Errorf("valid ERR-3 flagged: %+v", errs)
	}
	msg, schema = parseFixture(t, base+"999^Not a code^HL70357|E")
	errs := validateTableBindings(schema, msg, LevelStrict)
	if len(errs) != 1 || errs[0].Field != "ERR.3" {
		t.Errorf("unknown ERR-3 code: %+v", errs)
	}
}
