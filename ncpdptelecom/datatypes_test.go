package ncpdptelecom_test

import (
	"math"
	"testing"

	"ezhealthkonnect/ncpdptelecom"
)

// Worked examples transcribed verbatim from
// eduardonunesp/ncpdp-telecom-fmt-book's docs/data-types.md — the primary
// source for the signed-overpunch encoding this test proves both directions
// of.
func TestDecodeOverpunch_WorkedExamples(t *testing.T) {
	cases := []struct {
		name   string
		raw    string
		places int
		want   float64
	}{
		{"positive gross amount due", "0000084F", 2, 8.46},
		{"negative amount", "0000084J", 2, -8.41},
		{"positive zero", "0000000000{", 2, 0.00},
		{"ingredient cost", "0000057A", 2, 5.71},
		{"dispensing fee", "0000027E", 2, 2.75},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ncpdptelecom.DecodeOverpunch(c.raw, c.places)
			if err != nil {
				t.Fatalf("DecodeOverpunch(%q, %d) error: %v", c.raw, c.places, err)
			}
			if math.Abs(got-c.want) > 0.001 {
				t.Errorf("DecodeOverpunch(%q, %d) = %v, want %v", c.raw, c.places, got, c.want)
			}
		})
	}
}

func TestEncodeOverpunch_RoundTripsWorkedExamples(t *testing.T) {
	cases := []struct {
		name   string
		value  float64
		places int
		width  int
		want   string
	}{
		{"positive gross amount due", 8.46, 2, 8, "0000084F"},
		{"negative amount", -8.41, 2, 8, "0000084J"},
		{"positive zero", 0.00, 2, 8, "0000000{"},
		{"ingredient cost", 5.71, 2, 8, "0000057A"},
		{"dispensing fee", 2.75, 2, 8, "0000027E"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ncpdptelecom.EncodeOverpunch(c.value, c.places, c.width)
			if err != nil {
				t.Fatalf("EncodeOverpunch(%v, %d, %d) error: %v", c.value, c.places, c.width, err)
			}
			if got != c.want {
				t.Errorf("EncodeOverpunch(%v, %d, %d) = %q, want %q", c.value, c.places, c.width, got, c.want)
			}
			// Round trip: decoding what we just encoded must recover the
			// original value.
			decoded, err := ncpdptelecom.DecodeOverpunch(got, c.places)
			if err != nil {
				t.Fatalf("DecodeOverpunch(%q, %d) error: %v", got, c.places, err)
			}
			if math.Abs(decoded-c.value) > 0.001 {
				t.Errorf("round trip: decoded %v, want %v", decoded, c.value)
			}
		})
	}
}

func TestValidate_OverpunchRejectsInvalidLetter(t *testing.T) {
	if err := ncpdptelecom.Validate(ncpdptelecom.TypeOverpunch, "0000084Z", 8); err == nil {
		t.Error("expected an error for an invalid overpunch letter, got nil")
	}
}

func TestFormat_IntegerZeroPads(t *testing.T) {
	got, err := ncpdptelecom.Format(ncpdptelecom.TypeInteger, float64(1), 0, 3)
	if err != nil {
		t.Fatalf("Format error: %v", err)
	}
	if got != "001" {
		t.Errorf("Format(1, width=3) = %q, want \"001\"", got)
	}
}

func TestParse_DecimalAppliesImpliedPlaces(t *testing.T) {
	got, err := ncpdptelecom.Parse(ncpdptelecom.TypeDecimal, "00000030", 2)
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	f, ok := got.(float64)
	if !ok || math.Abs(f-0.30) > 0.001 {
		t.Errorf("Parse(\"00000030\", places=2) = %v, want 0.30", got)
	}
}
