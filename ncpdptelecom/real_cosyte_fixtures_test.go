package ncpdptelecom_test

// Permanent regression guard over real, third-party NCPDP Telecommunication
// D.0 fixtures (cosyte/ncpdp's own MIT-licensed test suite, test/fixtures/
// telecom/*.ncpdp — vendored here for test-only use, same precedent as
// ncpdp/real_cosyte_fixtures_test.go). Unlike the SCRIPT battery, NONE of
// these 3 are complete, spec-conformant transmissions — 2 are shorter than
// the real 56-byte fixed header (pbm-reject-unknown.ncpdp,
// pbm-response-dur.ncpdp — cosyte's own minimal unit-test snippets, not
// full messages), and the 1 that IS byte-length-conformant
// (pbm-person-code.ncpdp) is missing several real-world-required segments
// (PharmacyProvider, Pricing) and fields (patientLastName,
// datePrescriptionWritten) — cosyte's own test focus was narrowly
// "person code" parsing, not a complete realistic claim.
//
// This test exists to PROVE that finding, not just assert it once and
// forget it: the engine must (a) correctly reject the 2 too-short
// transmissions with a clear, graceful error — never a panic/hang — and
// (b) correctly parse AND correctly flag the 1 byte-conformant-but-
// incomplete transmission as invalid (proving the validator, not just the
// parser, works against real third-party data). If a future engine change
// makes any of this silently succeed differently, this test catches it.
//
// Run: go test ./ncpdptelecom/ -run TestRealCosyteTelecomFixtures -v
import (
	"os"
	"testing"

	"ezhealthkonnect/ncpdptelecom"
	"ezhealthkonnect/ncpdptelecom/validator"
)

func TestRealCosyteTelecomFixtures(t *testing.T) {
	loader, err := ncpdptelecom.NewTelecomSchemaLoader("schemas/telecom_d0")
	if err != nil {
		t.Fatalf("schema load: %v", err)
	}
	spec := loader.Spec()

	t.Run("pbm-reject-unknown.ncpdp — too short for the 56-byte header, must fail gracefully", func(t *testing.T) {
		raw, err := os.ReadFile("testdata/real_samples/pbm-reject-unknown.ncpdp")
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		_, err = ncpdptelecom.ParseTransmission(spec, "response", string(raw))
		if err == nil {
			t.Fatal("expected a graceful parse error for a transmission shorter than the 56-byte header, got nil")
		}
	})

	t.Run("pbm-response-dur.ncpdp — too short for the 56-byte header, must fail gracefully", func(t *testing.T) {
		raw, err := os.ReadFile("testdata/real_samples/pbm-response-dur.ncpdp")
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		_, err = ncpdptelecom.ParseTransmission(spec, "response", string(raw))
		if err == nil {
			t.Fatal("expected a graceful parse error for a transmission shorter than the 56-byte header, got nil")
		}
	})

	t.Run("pbm-person-code.ncpdp — byte-conformant header, parses OK, correctly flagged incomplete", func(t *testing.T) {
		raw, err := os.ReadFile("testdata/real_samples/pbm-person-code.ncpdp")
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		parsed, err := ncpdptelecom.ParseTransmission(spec, "request", string(raw))
		if err != nil {
			t.Fatalf("expected this real, byte-conformant fixture to parse successfully, got: %v", err)
		}
		if parsed.TransactionCode != "B1" {
			t.Errorf("TransactionCode = %q, want B1", parsed.TransactionCode)
		}
		result := validator.Validate(spec, parsed)
		if result.Valid {
			t.Error("expected this narrowly-scoped fixture (missing PharmacyProvider/Pricing) to be flagged invalid, got valid=true")
		}
	})
}
