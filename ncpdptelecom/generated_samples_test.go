package ncpdptelecom_test

// Permanent regression guard over a diverse, byte-accurate B1 request/
// response sample battery (18 scenarios: multi-claim, minimal/fully
// populated, fractional quantities, prior auth, controlled substances,
// zero/large dollar amounts, and every response outcome family —
// approved/captured/plain/rejected/duplicate/error, with and without
// pricing, single and multi-claim). Every sample is generated once via
// ncpdptelecom/sample_generator (BuildTransmission itself), so byte-
// accuracy is guaranteed by construction — this test only proves every
// one PARSES and VALIDATES cleanly against the real engine, closing the
// loop a throwaway generator script can't (a script runs once and is
// deleted; this stays as a standing guard against a future schema/engine
// regression on any of these real-world-shaped scenarios).
//
// Found and fixed a real bug via this exact battery (2026-09-19):
// ResponsePricing's grossAmountDue/totalAmountPaid were modeled
// required=true, so every rejected/duplicate/error response sample (which
// realistically carries no pricing segment at all) failed validation with
// "required segment ResponsePricing is missing" — see
// schemas/telecom_d0/segments/ResponsePricing.json's own sourceRefs for
// the full correction record.
//
// Run: go test ./ncpdptelecom/ -run TestGeneratedSampleBattery_AllParseAndValidateCleanly -v
import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ezhealthkonnect/ncpdptelecom"
	"ezhealthkonnect/ncpdptelecom/validator"
)

func TestGeneratedSampleBattery_AllParseAndValidateCleanly(t *testing.T) {
	loader, err := ncpdptelecom.NewTelecomSchemaLoader("schemas/telecom_d0")
	if err != nil {
		t.Fatalf("failed to load D.0 schema: %v", err)
	}
	spec := loader.Spec()

	dir := "testdata/generated_samples"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read %s: %v", dir, err)
	}
	if len(entries) == 0 {
		t.Fatalf("no generated samples found in %s", dir)
	}

	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".txt") {
			continue
		}
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}

			var direction string
			switch {
			case strings.HasPrefix(name, "request_"):
				direction = "request"
			case strings.HasPrefix(name, "response_"):
				direction = "response"
			default:
				t.Fatalf("sample file %q doesn't start with request_/response_ — can't infer direction", name)
			}

			parsed, err := ncpdptelecom.ParseTransmission(spec, direction, string(raw))
			if err != nil {
				t.Fatalf("ParseTransmission failed: %v", err)
			}

			result := validator.Validate(spec, parsed)
			if !result.Valid {
				t.Errorf("expected %s to validate cleanly, got issues: %+v", name, result.Issues)
			}
		})
	}
}
