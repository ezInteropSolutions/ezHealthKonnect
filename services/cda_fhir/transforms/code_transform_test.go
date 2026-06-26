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

func TestNormalizeSystem_HL7MaritalStatusOID_GetsCanonicalURL(t *testing.T) {
	got := normalizeSystem("2.16.840.1.113883.5.2")
	want := "http://terminology.hl7.org/CodeSystem/v3-MaritalStatus"
	if got != want {
		t.Errorf("normalizeSystem(MaritalStatus OID) = %q, want %q", got, want)
	}
}

func TestNormalizeSystem_HL7RoleCodeOID_GetsCanonicalURL(t *testing.T) {
	got := normalizeSystem("2.16.840.1.113883.5.88")
	want := "http://terminology.hl7.org/CodeSystem/v3-RoleCode"
	if got != want {
		t.Errorf("normalizeSystem(RoleCode OID) = %q, want %q", got, want)
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

// ---- IIToIdentifier: malformed NPI id (root, no extension) must not fall back to
// root-as-value -- confirmed against real-world PracticeFusion corpus data, where 3
// authors carry exactly this shape. ----

func TestIIToIdentifier_NPIRootWithNoExtension_ReturnsNil(t *testing.T) {
	id := cdadocument.CDAII{Root: "2.16.840.1.113883.4.6"} // NPI system OID, no @extension
	if got := IIToIdentifier(id); got != nil {
		t.Errorf("IIToIdentifier(NPI root, no extension) = %v, want nil (root is the system OID, not an NPI value)", got)
	}
}

func TestIIToIdentifier_SSNRootWithNoExtension_ReturnsNil(t *testing.T) {
	id := cdadocument.CDAII{Root: "2.16.840.1.113883.4.1"} // SSN system OID, no @extension
	if got := IIToIdentifier(id); got != nil {
		t.Errorf("IIToIdentifier(SSN root, no extension) = %v, want nil", got)
	}
}

func TestIIToIdentifier_NPIWithExtension_StillWorks(t *testing.T) {
	id := cdadocument.CDAII{Root: "2.16.840.1.113883.4.6", Extension: "1234567890"}
	got := IIToIdentifier(id)
	if got == nil {
		t.Fatal("IIToIdentifier(NPI root, real extension) returned nil, want a populated Identifier")
	}
	if got["value"] != "1234567890" {
		t.Errorf("Identifier.value = %v, want the actual NPI digits", got["value"])
	}
	if got["system"] != "http://hl7.org/fhir/sid/us-npi" {
		t.Errorf("Identifier.system = %v, want the NPI system URI", got["system"])
	}
}

func TestIIToIdentifier_GenericFacilityOID_RootAsValueFallbackStillWorks(t *testing.T) {
	// Generic/facility-specific OIDs have no fixed national meaning -- some
	// real-world implementations legitimately encode the identifier value only
	// in root. This fallback must remain for those, unlike the fixed-system case.
	id := cdadocument.CDAII{Root: "2.16.840.1.113883.19.5.99999.1"}
	got := IIToIdentifier(id)
	if got == nil {
		t.Fatal("IIToIdentifier(generic facility OID, no extension) returned nil, want root-as-value fallback")
	}
	if got["value"] != "2.16.840.1.113883.19.5.99999.1" {
		t.Errorf("Identifier.value = %v, want root as the fallback value", got["value"])
	}
}
