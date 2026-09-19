package ncpdp_test

import (
	"os"
	"testing"

	"ezhealthkonnect/ncpdp"
	ncpdpbuilder "ezhealthkonnect/ncpdp/builder"

	"github.com/stretchr/testify/require"
)

// The four lifecycle-transaction fixtures below are SELF-AUTHORED (not
// sourced from a real message) — there is no free real CancelRx/
// RxChangeRequest/RxChangeResponse sample available (unlike NewRx, which is
// tested against a genuine dgoradia/ncpdp sample). Their structure is
// cross-validated against cosyte/ncpdp's actively-maintained open-source
// SCRIPT parser (src/script/lifecycle.ts) rather than invented from
// scratch — see each transaction schema file's own sourceRefs for the exact
// provenance and named gaps (e.g. RxChangeRequest's own change-reason
// sub-schema, not yet confirmed against any real sample).

func TestCancelRx_SelfAuthoredSample_ParseAndRoundTrip(t *testing.T) {
	spec := loadRealSpec(t)
	raw := readFixture(t, "testdata/self_authored/cancel_rx_sample.xml")

	result, err := ncpdp.ParseMessage(spec, raw)
	require.NoError(t, err)
	require.Equal(t, "CancelRx", result.TransactionType)
	require.Equal(t, "REQ-0001", result.Body["requestReferenceNumber"])

	patient := getMap(t, result.Body, "patient")
	humanPatient := getMap(t, patient, "humanPatient")
	require.Equal(t, "Jenny", getMap(t, humanPatient, "name")["lastName"])

	prescriber := getMap(t, result.Body, "prescriber")
	nonVet := getMap(t, prescriber, "nonVeterinarian")
	require.Equal(t, "1939842031", getMap(t, nonVet, "identification")["npi"])

	med := getMap(t, result.Body, "medicationPrescribed")
	require.Equal(t, "Ondansetron 8 mg Tab Disintegrating", med["drugDescription"])

	built, err := ncpdpbuilder.BuildDocument(spec, "CancelRx", result.Header, result.Body)
	require.NoError(t, err)
	require.Contains(t, built, "<CancelRx>")

	reparsed, err := ncpdp.ParseMessage(spec, built)
	require.NoError(t, err)
	require.Equal(t, result.Header, reparsed.Header)
	require.Equal(t, result.Body, reparsed.Body)
}

func TestCancelRxResponse_SelfAuthoredSample_DeniedOutcome_ParseAndRoundTrip(t *testing.T) {
	spec := loadRealSpec(t)
	raw := readFixture(t, "testdata/self_authored/cancel_rx_response_denied_sample.xml")

	result, err := ncpdp.ParseMessage(spec, raw)
	require.NoError(t, err)
	require.Equal(t, "CancelRxResponse", result.TransactionType)
	require.Equal(t, "app-000000000001", result.Header["relatesToMessageID"])
	require.Equal(t, "REQ-0001", result.Body["requestReferenceNumber"])

	// The 6 outcome choice branches (approved/denied/denyNewToFollow/
	// approvedWithChanges/validated/replace) are nested one level deeper
	// than the transaction root, inside a "response" wrapper group — see
	// ResponseWrapper.json's own sourceRefs for why (a real, confirmed
	// wire-format correction, found via real third-party fixtures).
	response := getMap(t, result.Body, "response")
	// Only the Denied outcome group should be present — the other 5 choice
	// branches must be absent, not present-with-zero-values.
	require.NotContains(t, response, "approved")
	require.NotContains(t, response, "approvedWithChanges")
	denied := getMap(t, response, "denied")
	require.Equal(t, "AB", denied["reasonCode"])
	require.Equal(t, "Already dispensed", denied["denialReason"])
	require.Equal(t, "Patient already picked up medication", denied["note"])

	built, err := ncpdpbuilder.BuildDocument(spec, "CancelRxResponse", result.Header, result.Body)
	require.NoError(t, err)
	require.Contains(t, built, "<Denied>")
	require.NotContains(t, built, "<Approved>") // must not spuriously create sibling outcome elements

	reparsed, err := ncpdp.ParseMessage(spec, built)
	require.NoError(t, err)
	require.Equal(t, result.Header, reparsed.Header)
	require.Equal(t, result.Body, reparsed.Body)
}

func TestRxChangeRequest_SelfAuthoredSample_ParseAndRoundTrip(t *testing.T) {
	spec := loadRealSpec(t)
	raw := readFixture(t, "testdata/self_authored/rx_change_request_sample.xml")

	result, err := ncpdp.ParseMessage(spec, raw)
	require.NoError(t, err)
	require.Equal(t, "RxChangeRequest", result.TransactionType)
	require.Equal(t, "REQ-0002", result.Body["requestReferenceNumber"])

	pharmacy := getMap(t, result.Body, "pharmacy")
	require.Equal(t, "A+ Drugs", pharmacy["businessName"])

	built, err := ncpdpbuilder.BuildDocument(spec, "RxChangeRequest", result.Header, result.Body)
	require.NoError(t, err)
	require.Contains(t, built, "<RxChangeRequest>")

	reparsed, err := ncpdp.ParseMessage(spec, built)
	require.NoError(t, err)
	require.Equal(t, result.Header, reparsed.Header)
	require.Equal(t, result.Body, reparsed.Body)
}

func TestRxChangeResponse_SelfAuthoredSample_ApprovedWithChangesOutcome_ParseAndRoundTrip(t *testing.T) {
	spec := loadRealSpec(t)
	raw := readFixture(t, "testdata/self_authored/rx_change_response_approved_with_changes_sample.xml")

	result, err := ncpdp.ParseMessage(spec, raw)
	require.NoError(t, err)
	require.Equal(t, "RxChangeResponse", result.TransactionType)

	response := getMap(t, result.Body, "response")
	require.NotContains(t, response, "denied")
	awc := getMap(t, response, "approvedWithChanges")
	require.Equal(t, "G", awc["reasonCode"])
	require.Equal(t, "Approved generic substitution", awc["note"])

	med := getMap(t, result.Body, "medicationPrescribed")
	require.Equal(t, "Ondansetron ODT 4 mg", med["drugDescription"])

	built, err := ncpdpbuilder.BuildDocument(spec, "RxChangeResponse", result.Header, result.Body)
	require.NoError(t, err)
	require.Contains(t, built, "<ApprovedWithChanges>")

	reparsed, err := ncpdp.ParseMessage(spec, built)
	require.NoError(t, err)
	require.Equal(t, result.Header, reparsed.Header)
	require.Equal(t, result.Body, reparsed.Body)
}

func TestRxRenewalRequest_SelfAuthoredSample_ParseAndRoundTrip(t *testing.T) {
	spec := loadRealSpec(t)
	raw := readFixture(t, "testdata/self_authored/rx_renewal_request_sample.xml")

	result, err := ncpdp.ParseMessage(spec, raw)
	require.NoError(t, err)
	require.Equal(t, "RxRenewalRequest", result.TransactionType)
	require.Equal(t, "REQ-0003", result.Body["requestReferenceNumber"])

	patient := getMap(t, result.Body, "patient")
	humanPatient := getMap(t, patient, "humanPatient")
	require.Equal(t, "Jenny", getMap(t, humanPatient, "name")["lastName"])

	pharmacy := getMap(t, result.Body, "pharmacy")
	require.Equal(t, "A+ Drugs", pharmacy["businessName"])

	med := getMap(t, result.Body, "medicationPrescribed")
	require.Equal(t, "Ondansetron 8 mg Tab Disintegrating", med["drugDescription"])

	built, err := ncpdpbuilder.BuildDocument(spec, "RxRenewalRequest", result.Header, result.Body)
	require.NoError(t, err)
	require.Contains(t, built, "<RxRenewalRequest>")

	reparsed, err := ncpdp.ParseMessage(spec, built)
	require.NoError(t, err)
	require.Equal(t, result.Header, reparsed.Header)
	require.Equal(t, result.Body, reparsed.Body)
}

func TestRxRenewalResponse_SelfAuthoredSample_ApprovedOutcome_ParseAndRoundTrip(t *testing.T) {
	spec := loadRealSpec(t)
	raw := readFixture(t, "testdata/self_authored/rx_renewal_response_approved_sample.xml")

	result, err := ncpdp.ParseMessage(spec, raw)
	require.NoError(t, err)
	require.Equal(t, "RxRenewalResponse", result.TransactionType)
	require.Equal(t, "app-000000000005", result.Header["relatesToMessageID"])
	require.Equal(t, "REQ-0003", result.Body["requestReferenceNumber"])

	response := getMap(t, result.Body, "response")
	require.NotContains(t, response, "denied")
	require.NotContains(t, response, "approvedWithChanges")
	approved := getMap(t, response, "approved")
	require.Equal(t, "A", approved["reasonCode"])
	require.Equal(t, "Renewal approved as written", approved["note"])

	built, err := ncpdpbuilder.BuildDocument(spec, "RxRenewalResponse", result.Header, result.Body)
	require.NoError(t, err)
	require.Contains(t, built, "<Approved>")
	require.NotContains(t, built, "<Denied>")

	reparsed, err := ncpdp.ParseMessage(spec, built)
	require.NoError(t, err)
	require.Equal(t, result.Header, reparsed.Header)
	require.Equal(t, result.Body, reparsed.Body)
}

func TestRxFill_SelfAuthoredSample_FilledOutcome_ParseAndRoundTrip(t *testing.T) {
	spec := loadRealSpec(t)
	raw := readFixture(t, "testdata/self_authored/rx_fill_filled_sample.xml")

	result, err := ncpdp.ParseMessage(spec, raw)
	require.NoError(t, err)
	require.Equal(t, "RxFill", result.TransactionType)

	require.NotContains(t, result.Body, "notFilled")
	require.NotContains(t, result.Body, "partialFill")
	filled := getMap(t, result.Body, "filled")
	require.Equal(t, "REQ-0004", filled["referenceNumber"])
	require.Equal(t, "Dispensed as written", filled["note"])

	pharmacy := getMap(t, result.Body, "pharmacy")
	require.Equal(t, "A+ Drugs", pharmacy["businessName"])

	dispensed := getMap(t, result.Body, "medicationDispensed")
	require.Equal(t, "Ondansetron 8 mg Tab Disintegrating", dispensed["drugDescription"])
	lastFillDate := getMap(t, dispensed, "lastFillDate")
	require.Equal(t, "2026-09-19", lastFillDate["date"])

	built, err := ncpdpbuilder.BuildDocument(spec, "RxFill", result.Header, result.Body)
	require.NoError(t, err)
	require.Contains(t, built, "<Filled>")
	require.NotContains(t, built, "<NotFilled>")

	reparsed, err := ncpdp.ParseMessage(spec, built)
	require.NoError(t, err)
	require.Equal(t, result.Header, reparsed.Header)
	require.Equal(t, result.Body, reparsed.Body)
}

func TestStatus_SelfAuthoredSample_ParseAndRoundTrip(t *testing.T) {
	spec := loadRealSpec(t)
	raw := readFixture(t, "testdata/self_authored/status_sample.xml")

	result, err := ncpdp.ParseMessage(spec, raw)
	require.NoError(t, err)
	require.Equal(t, "Status", result.TransactionType)
	require.Equal(t, "000", result.Body["code"])
	require.Equal(t, "Message received and processed successfully", result.Body["description"])

	built, err := ncpdpbuilder.BuildDocument(spec, "Status", result.Header, result.Body)
	require.NoError(t, err)
	require.Contains(t, built, "<Status>")

	reparsed, err := ncpdp.ParseMessage(spec, built)
	require.NoError(t, err)
	require.Equal(t, result.Header, reparsed.Header)
	require.Equal(t, result.Body, reparsed.Body)
}

func TestError_SelfAuthoredSample_ParseAndRoundTrip(t *testing.T) {
	spec := loadRealSpec(t)
	raw := readFixture(t, "testdata/self_authored/error_sample.xml")

	result, err := ncpdp.ParseMessage(spec, raw)
	require.NoError(t, err)
	require.Equal(t, "Error", result.TransactionType)
	require.Equal(t, "900", result.Body["code"])
	require.Equal(t, "001", result.Body["descriptionCode"])
	require.Equal(t, "Unable to process transaction: unknown patient", result.Body["description"])

	built, err := ncpdpbuilder.BuildDocument(spec, "Error", result.Header, result.Body)
	require.NoError(t, err)
	require.Contains(t, built, "<Error>")

	reparsed, err := ncpdp.ParseMessage(spec, built)
	require.NoError(t, err)
	require.Equal(t, result.Header, reparsed.Header)
	require.Equal(t, result.Body, reparsed.Body)
}

func TestVerify_SelfAuthoredSample_ParseAndRoundTrip(t *testing.T) {
	spec := loadRealSpec(t)
	raw := readFixture(t, "testdata/self_authored/verify_sample.xml")

	result, err := ncpdp.ParseMessage(spec, raw)
	require.NoError(t, err)
	require.Equal(t, "Verify", result.TransactionType)
	require.Equal(t, "000", result.Body["code"])
	require.Equal(t, "Prescriber identity verified", result.Body["description"])

	built, err := ncpdpbuilder.BuildDocument(spec, "Verify", result.Header, result.Body)
	require.NoError(t, err)
	require.Contains(t, built, "<Verify>")

	reparsed, err := ncpdp.ParseMessage(spec, built)
	require.NoError(t, err)
	require.Equal(t, result.Header, reparsed.Header)
	require.Equal(t, result.Body, reparsed.Body)
}

func TestGetMessage_SelfAuthoredSample_ParseAndRoundTrip(t *testing.T) {
	spec := loadRealSpec(t)
	raw := readFixture(t, "testdata/self_authored/get_message_sample.xml")

	result, err := ncpdp.ParseMessage(spec, raw)
	require.NoError(t, err)
	require.Equal(t, "GetMessage", result.TransactionType)
	require.Empty(t, result.Body)

	built, err := ncpdpbuilder.BuildDocument(spec, "GetMessage", result.Header, result.Body)
	require.NoError(t, err)
	require.Contains(t, built, "<GetMessage")

	reparsed, err := ncpdp.ParseMessage(spec, built)
	require.NoError(t, err)
	require.Equal(t, result.Header, reparsed.Header)
	require.Equal(t, result.Body, reparsed.Body)
}

func readFixture(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}
