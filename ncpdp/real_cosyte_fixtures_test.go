package ncpdp_test

// Permanent regression guard over real, third-party NCPDP SCRIPT fixtures
// (cosyte/ncpdp's own MIT-licensed test suite — real fixtures, not
// self-authored — vendored here for test-only use, matching this
// codebase's own established "real sample, test-only, sourceRefs
// documented" precedent already proven for EDI X12's own real-sample
// rounds). Every fixture is parsed against the real ncpdp.ParseMessage
// engine — a genuine "process every message" proof across all 12
// transaction types this engine supports (a coverage this engine's own
// self-authored fixtures alone could never provide, since they only ever
// exercise the ONE structural shape their own author assumed).
//
// A subset of these fixtures is DELIBERATELY minimal/edge-case by design
// (their own filenames say so — e.g. "newrx-sig-text-only", "status-no-code",
// "rxrenewal-response-no-outcome") and correctly fail Validate — that's
// proof the validator works, not a bug. cleanFixtures below is the
// explicit, individually-confirmed allowlist of fixtures that SHOULD
// validate cleanly; everything else is parsed (asserting zero parse
// failures) but only logged, not asserted valid.
//
// Found and fixed 3 real bugs via this exact battery (2026-09-19):
//  1. CodedElement's Code/Qualifier alternate wire shape (attribute+text
//     content vs. nested <Code>/<Qualifier> sub-elements) — see
//     CodedElement.json's own sourceRefs.
//  2. MedicationPrescribed's quantity/writtenDate/sig required=true was
//     applied uniformly across NewRx AND CancelRx/RxChangeRequest/
//     RxRenewalRequest/RxFill, but a real CancelRx legitimately omits them
//     — see MedicationPrescribed.json's own sourceRefs.
//  3. CancelRxResponse/RxChangeResponse/RxRenewalResponse's own 6 outcome
//     groups (Approved/Denied/etc.) are nested one level deeper than
//     originally modeled, inside a `<Response>` wrapper element — see
//     ResponseWrapper.json's own sourceRefs.
//
// Run: go test ./ncpdp/ -run TestRealCosyteFixtures_AllParseCleanly -v
import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ezhealthkonnect/ncpdp"
	"ezhealthkonnect/ncpdp/validator"
)

// cleanFixtures lists every fixture individually confirmed (2026-09-19, post
// all 3 fixes above) to validate with zero issues — a real, complete,
// well-formed message for its own transaction type.
var cleanFixtures = map[string]bool{
	"cancelrx-request.xml":                                true,
	"cancelrx-response-approved.xml":                      true,
	"cancelrx-response-denied.xml":                        true,
	"error-response.xml":                                  true,
	"newrx-basic.xml":                                      true,
	"newrx-coded-and-strength.xml":                         true,
	"rxchange-request.xml":                                 true,
	"rxchange-response-denied.xml":                         true,
	"rxchange-response-validated.xml":                      true,
	"rxrenewal-request.xml":                                true,
	"rxrenewal-response-ambiguous.xml":                     true,
	"rxrenewal-response-approved-with-changes-nested.xml":  true,
	"rxrenewal-response-approved-with-changes.xml":         true,
	"rxrenewal-response-approved.xml":                      true,
	"rxrenewal-response-denied.xml":                        true,
	"status-response.xml":                                  true,
	"surescripts-routing.xml":                               true,
	"verify-response.xml":                                   true,
}

func TestRealCosyteFixtures_AllParseCleanly(t *testing.T) {
	loader, err := ncpdp.NewNCPDPSchemaLoader("schemas/script_2017071")
	if err != nil {
		t.Fatalf("schema load: %v", err)
	}
	spec := loader.Spec()

	dir := "testdata/real_samples/cosyte"
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatalf("no real fixtures found in %s", dir)
	}

	parseFailures := 0
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".xml") {
			continue
		}
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			parsed, err := ncpdp.ParseMessage(spec, string(raw))
			if err != nil {
				parseFailures++
				t.Errorf("ParseMessage failed (this should NEVER happen for well-formed XML, even an incomplete one): %v", err)
				return
			}
			result := validator.Validate(spec, parsed)
			if cleanFixtures[name] {
				if !result.Valid {
					t.Errorf("expected %s (transactionType=%s) to validate cleanly, got issues: %+v", name, parsed.TransactionType, result.Issues)
				}
			} else {
				t.Logf("%s (transactionType=%s) valid=%v issues=%+v (not asserted — a deliberately minimal/edge-case fixture)", name, parsed.TransactionType, result.Valid, result.Issues)
			}
		})
	}
}
