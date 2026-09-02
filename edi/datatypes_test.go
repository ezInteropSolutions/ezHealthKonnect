package edi

import "testing"

func TestValidate(t *testing.T) {
	cases := []struct {
		name    string
		t       X12DataType
		raw     string
		wantErr bool
	}{
		{"AN any string ok", TypeAN, "hello world", false},
		{"ID any string ok", TypeID, "PR", false},
		{"DT valid date", TypeDT, "20260828", false},
		{"DT wrong length", TypeDT, "2026828", true},
		{"DT non-numeric", TypeDT, "2026082X", true},
		{"TM 4 digits ok", TypeTM, "1204", false},
		{"TM 6 digits ok", TypeTM, "120405", false},
		{"TM wrong length", TypeTM, "12", true},
		{"R valid decimal", TypeR, "1250.00", false},
		{"R invalid", TypeR, "not-a-number", true},
		{"N2 valid integer", TypeN2, "125000", false},
		{"N2 negative valid", TypeN2, "-125000", false},
		{"N2 invalid", TypeN2, "12.50", true},
		{"unknown type", X12DataType("ZZ"), "anything", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Validate(c.t, c.raw, 0, 0)
			if (err != nil) != c.wantErr {
				t.Errorf("Validate(%s, %q) error = %v, wantErr %v", c.t, c.raw, err, c.wantErr)
			}
		})
	}
}

func TestValidate_LengthBounds(t *testing.T) {
	if err := Validate(TypeAN, "ab", 3, 5); err == nil {
		t.Error("expected error for value shorter than minLength")
	}
	if err := Validate(TypeAN, "abcdef", 3, 5); err == nil {
		t.Error("expected error for value longer than maxLength")
	}
	if err := Validate(TypeAN, "abcd", 3, 5); err != nil {
		t.Errorf("unexpected error for in-bounds value: %v", err)
	}
}

func TestParseAndFormat_ImpliedDecimal(t *testing.T) {
	v, err := Parse(TypeN2, "125000")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	f, ok := v.(float64)
	if !ok || f != 1250.00 {
		t.Errorf("Parse(N2, 125000) = %#v, want 1250.00", v)
	}

	back, err := Format(TypeN2, 1250.00)
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	if back != "125000" {
		t.Errorf("Format(N2, 1250.00) = %q, want 125000", back)
	}
}

func TestParseAndFormat_NegativeImpliedDecimal(t *testing.T) {
	v, err := Parse(TypeN2, "-500")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if v.(float64) != -5.00 {
		t.Errorf("Parse(N2, -500) = %#v, want -5.00", v)
	}

	back, err := Format(TypeN2, -5.00)
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	if back != "-500" {
		t.Errorf("Format(N2, -5.00) = %q, want -500", back)
	}
}

func TestParseAndFormat_RDecimal(t *testing.T) {
	v, err := Parse(TypeR, "1250.5")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if v.(float64) != 1250.5 {
		t.Errorf("Parse(R, 1250.5) = %#v, want 1250.5", v)
	}
	back, err := Format(TypeR, 1250.5)
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	if back != "1250.5" {
		t.Errorf("Format(R, 1250.5) = %q, want 1250.5", back)
	}
}

func TestParseAndFormat_ANPassthrough(t *testing.T) {
	v, err := Parse(TypeAN, "hello")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if v != "hello" {
		t.Errorf("Parse(AN, hello) = %#v, want hello", v)
	}
	back, err := Format(TypeAN, "hello")
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	if back != "hello" {
		t.Errorf("Format(AN, hello) = %q, want hello", back)
	}
}
