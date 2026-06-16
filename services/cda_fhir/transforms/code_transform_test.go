package transforms

import (
	"testing"

	cdadocument "ezhealthkonnect/cda/document"
)

func TestNormalizeSystem_KnownOID(t *testing.T) {
	got := normalizeSystem("2.16.840.1.113883.6.1") // LOINC
	want := "http://loinc.org"
	if got != want {
		t.Errorf("normalizeSystem(known LOINC OID) = %q, want %q", got, want)
	}
}

func TestNormalizeSystem_UnmappedOID_GetsURNOIDPrefix(t *testing.T) {
	// ICD-9-CM — not in the explicit lookup table.
	got := normalizeSystem("2.16.840.1.113883.6.103")
	want := "urn:oid:2.16.840.1.113883.6.103"
	if got != want {
		t.Errorf("normalizeSystem(unmapped OID) = %q, want %q (must be an absolute URI)", got, want)
	}
}

func TestNormalizeSystem_HL7MaritalStatusOID_GetsURNOIDPrefix(t *testing.T) {
	got := normalizeSystem("2.16.840.1.113883.5.2")
	want := "urn:oid:2.16.840.1.113883.5.2"
	if got != want {
		t.Errorf("normalizeSystem(MaritalStatus OID) = %q, want %q", got, want)
	}
}

func TestNormalizeSystem_AlreadyAbsoluteURI_Unchanged(t *testing.T) {
	got := normalizeSystem("http://example.org/custom-system")
	want := "http://example.org/custom-system"
	if got != want {
		t.Errorf("normalizeSystem(absolute URI) = %q, want unchanged %q", got, want)
	}
}

func TestNormalizeSystem_FreeTextName_Unchanged(t *testing.T) {
	got := normalizeSystem("Some Local Registry")
	want := "Some Local Registry"
	if got != want {
		t.Errorf("normalizeSystem(non-OID free text) = %q, want unchanged %q", got, want)
	}
}

func TestCDACodeToCoding_UnmappedOID_ProducesAbsoluteSystem(t *testing.T) {
	code := cdadocument.CDACode{
		Code:        "244.9",
		DisplayName: "Hypothyroidism",
		CodeSystem:  "2.16.840.1.113883.6.103", // ICD-9-CM, not in the lookup table
	}
	coding := CDACodeToCoding(code)
	if coding == nil {
		t.Fatal("CDACodeToCoding returned nil")
	}
	system, _ := coding["system"].(string)
	if system != "urn:oid:2.16.840.1.113883.6.103" {
		t.Errorf("coding[system] = %q, want urn:oid: form (absolute URI)", system)
	}
}
