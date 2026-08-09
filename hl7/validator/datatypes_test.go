package validator

import (
	"testing"

	"ezhealthkonnect/hl7"
)

func TestCheckDataTypeFormat_NM(t *testing.T) {
	cases := []struct {
		value   string
		wantErr bool
	}{
		{"123", false},
		{"12.5", false},
		{"-4", false},
		{"+3.14", false},
		{"12a", true},
		{"1.2.3", true},
		{"", false}, // empty handled by hasValue=false in caller; not reached here since we pass hasValue=true, "" still short-circuits
	}
	for _, c := range cases {
		hasValue := c.value != ""
		errs := checkDataTypeFormat("OBX", "OBX.5", "", hl7.DataTypeNM, c.value, hasValue, 0, hl7.SeverityWarning)
		if c.wantErr && len(errs) == 0 {
			t.Errorf("NM %q: expected an error, got none", c.value)
		}
		if !c.wantErr && len(errs) != 0 {
			t.Errorf("NM %q: expected no error, got %+v", c.value, errs)
		}
	}
}

func TestCheckDataTypeFormat_SI(t *testing.T) {
	cases := []struct {
		value   string
		wantErr bool
	}{
		{"5", false},
		{"0", false},
		{"-1", true},
		{"5.5", true},
		{"abc", true},
	}
	for _, c := range cases {
		errs := checkDataTypeFormat("OBX", "OBX.1", "", hl7.DataTypeSI, c.value, true, 0, hl7.SeverityWarning)
		if c.wantErr && len(errs) == 0 {
			t.Errorf("SI %q: expected an error, got none", c.value)
		}
		if !c.wantErr && len(errs) != 0 {
			t.Errorf("SI %q: expected no error, got %+v", c.value, errs)
		}
	}
}

func TestCheckDataTypeFormat_DT_TM_TS(t *testing.T) {
	dtCases := map[string]bool{"2024": true, "202401": true, "20240115": true, "2024-01-15": false, "abc": false}
	for v, want := range dtCases {
		errs := checkDataTypeFormat("PID", "PID.7", "", hl7.DataTypeDT, v, true, 0, hl7.SeverityWarning)
		if got := len(errs) == 0; got != want {
			t.Errorf("DT %q: valid=%v, want %v", v, got, want)
		}
	}

	tmCases := map[string]bool{"12": true, "1230": true, "123045": true, "123045.1234": true, "1230+0500": true, "9": false}
	for v, want := range tmCases {
		errs := checkDataTypeFormat("OBR", "OBR.7", "", hl7.DataTypeTM, v, true, 0, hl7.SeverityWarning)
		if got := len(errs) == 0; got != want {
			t.Errorf("TM %q: valid=%v, want %v", v, got, want)
		}
	}

	tsCases := map[string]bool{"20240101120000": true, "2024": true, "notadate": false}
	for v, want := range tsCases {
		errs := checkDataTypeFormat("MSH", "MSH.7", "", hl7.DataTypeTS, v, true, 0, hl7.SeverityWarning)
		if got := len(errs) == 0; got != want {
			t.Errorf("TS %q: valid=%v, want %v", v, got, want)
		}
	}
}

func TestCheckDataTypeFormat_LengthOverflow_STOnly(t *testing.T) {
	// MSH.1 (field separator) is a real, short (Length=1) ST field.
	if errs := checkDataTypeFormat("MSH", "MSH.1", "", hl7.DataTypeST, "|", true, 1, hl7.SeverityWarning); len(errs) != 0 {
		t.Errorf("expected no error for a value within length, got %+v", errs)
	}
	if errs := checkDataTypeFormat("MSH", "MSH.1", "", hl7.DataTypeST, "||", true, 1, hl7.SeverityWarning); len(errs) == 0 {
		t.Error("expected a length-overflow error, got none")
	}
}

// ID/IS format is owned entirely by table-binding validation — this
// category must never emit anything for them, even with an obviously
// "wrong-looking" value, to avoid double-reporting the same bad code.
func TestCheckDataTypeFormat_IDAndIS_AlwaysSkipped(t *testing.T) {
	if errs := checkDataTypeFormat("PID", "PID.8", "", hl7.DataTypeID, "NOT_A_REAL_CODE", true, 0, hl7.SeverityWarning); len(errs) != 0 {
		t.Errorf("expected ID to be skipped by data-type checking, got %+v", errs)
	}
	if errs := checkDataTypeFormat("PID", "PID.8", "", hl7.DataTypeIS, "NOT_A_REAL_CODE", true, 0, hl7.SeverityWarning); len(errs) != 0 {
		t.Errorf("expected IS to be skipped by data-type checking, got %+v", errs)
	}
}

func TestCheckDataTypeFormat_SeverityRespected(t *testing.T) {
	errs := checkDataTypeFormat("OBX", "OBX.5", "", hl7.DataTypeNM, "abc", true, 0, hl7.SeverityError)
	if len(errs) != 1 || errs[0].Severity != hl7.SeverityError {
		t.Fatalf("expected severity ERROR to be passed through, got %+v", errs)
	}
}
