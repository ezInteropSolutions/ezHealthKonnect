package ncpdptelecom_test

import (
	"math"
	"strings"
	"testing"

	"ezhealthkonnect/ncpdptelecom"
	ncpdptelecombuilder "ezhealthkonnect/ncpdptelecom/builder"
)

// The B1 request/response fixtures below are SELF-AUTHORED (there is no free
// real B1 sample available — NCPDP's own Implementation Guide is a paid
// document). Their structure is cross-validated against two real,
// structurally-authoritative sources rather than invented from scratch —
// see each schema file's own sourceRefs for the exact provenance. Built
// directly in Go (rather than as a checked-in fixture file) since D.0's own
// separators are non-printable control characters that would render as
// invisible garbage in a normal text-file diff.

func buildTestHeader(transactionCode string) string {
	return "999999" + // binNumber
		"D0" + // version
		transactionCode +
		strings.Repeat(" ", 10) + // processorControlNumber (empty)
		"1" + // transactionCount
		"01" + // serviceProviderIdQualifier
		"1111111111" + strings.Repeat(" ", 5) + // serviceProviderId (15)
		"20260919" + // dateOfService
		strings.Repeat(" ", 10) // softwareVendorCertificationId (empty)
}

const (
	rs = "\x1E"
	fs = "\x1C"
	gs = "\x1D"
)

// groupAt type-asserts TransactionGroups[i] (an interface{}, deliberately —
// see ParseResult.TransactionGroups's own doc comment for why) back to the
// map[string]interface{} every transaction-group cluster actually is.
func groupAt(groups []interface{}, i int) map[string]interface{} {
	m, _ := groups[i].(map[string]interface{})
	return m
}

func buildTestB1Request() string {
	header := buildTestHeader("B1")
	body := rs + fs + "AM01" + fs + "CBSMITH" + fs + "CAJOHN" + fs + "C419800101" + fs + "C52" +
		rs + fs + "AM02" + fs + "EY01" + fs + "E91234567890" +
		rs + fs + "AM04" + fs + "C2123456789012" + fs + "CCJOHN" + fs + "CDSMITH" +
		rs + fs + "AM03" + fs + "EZ01" + fs + "DB1234567893" + fs + "DRWELBY" +
		rs + fs + "AM07" + fs + "D2000000123456" + fs + "E103" + fs + "D700003089421" + fs + "E70000030000" + fs + "D301" + fs + "D5030" + fs + "DE20220924" +
		rs + fs + "AM11" + fs + "D90000057A" + fs + "DC0000027E" + fs + "DX0000016B" + fs + "DQ00000000" + fs + "DU0000084F"
	return header + body
}

func buildTestB1Response() string {
	header := buildTestHeader("B1")
	body := rs + fs + "AM20" + fs + "F4Claim processed successfully" +
		rs + fs + "AM21" + fs + "ANP" + fs + "F31234567890" +
		rs + fs + "AM23" + fs + "F50000084F" + fs + "F60000057A" + fs + "F70000027E" + fs + "F90000084F" + fs + "FI0000016B"
	return header + body
}

func TestB1Request_SelfAuthoredSample_ParseAndRoundTrip(t *testing.T) {
	spec := loadRealSpec(t)
	raw := buildTestB1Request()

	result, err := ncpdptelecom.ParseTransmission(spec, "request", raw)
	if err != nil {
		t.Fatalf("ParseTransmission error: %v", err)
	}
	if result.TransactionCode != "B1" {
		t.Errorf("TransactionCode = %q, want \"B1\"", result.TransactionCode)
	}
	if result.Header["binNumber"] != "999999" {
		t.Errorf("Header.binNumber = %v, want \"999999\"", result.Header["binNumber"])
	}
	if result.Header["dateOfService"] != "20260919" {
		t.Errorf("Header.dateOfService = %v, want \"20260919\"", result.Header["dateOfService"])
	}

	patient, ok := result.TransmissionGroup["Patient"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected TransmissionGroup.Patient, got %v", result.TransmissionGroup["Patient"])
	}
	if patient["patientLastName"] != "SMITH" {
		t.Errorf("Patient.patientLastName = %v, want \"SMITH\"", patient["patientLastName"])
	}
	if patient["dateOfBirth"] != "19800101" {
		t.Errorf("Patient.dateOfBirth = %v, want \"19800101\"", patient["dateOfBirth"])
	}

	insurance, ok := result.TransmissionGroup["Insurance"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected TransmissionGroup.Insurance, got %v", result.TransmissionGroup["Insurance"])
	}
	if insurance["cardholderId"] != "123456789012" {
		t.Errorf("Insurance.cardholderId = %v, want \"123456789012\"", insurance["cardholderId"])
	}

	if len(result.TransactionGroups) != 1 {
		t.Fatalf("expected 1 transaction group, got %d", len(result.TransactionGroups))
	}
	claim, ok := groupAt(result.TransactionGroups, 0)["Claim"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected TransactionGroups[0].Claim, got %v", groupAt(result.TransactionGroups, 0)["Claim"])
	}
	if qty, ok := claim["quantityDispensed"].(float64); !ok || math.Abs(qty-30.0) > 0.001 {
		t.Errorf("Claim.quantityDispensed = %v, want 30.0", claim["quantityDispensed"])
	}
	if days, ok := claim["daysSupply"].(float64); !ok || days != 30 {
		t.Errorf("Claim.daysSupply = %v, want 30", claim["daysSupply"])
	}

	pricing, ok := groupAt(result.TransactionGroups, 0)["Pricing"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected TransactionGroups[0].Pricing, got %v", groupAt(result.TransactionGroups, 0)["Pricing"])
	}
	if cost, ok := pricing["ingredientCostSubmitted"].(float64); !ok || math.Abs(cost-5.71) > 0.001 {
		t.Errorf("Pricing.ingredientCostSubmitted = %v, want 5.71 (decoded from overpunch)", pricing["ingredientCostSubmitted"])
	}
	if usc, ok := pricing["usualAndCustomaryCharge"].(float64); !ok || usc != 0 {
		t.Errorf("Pricing.usualAndCustomaryCharge = %v, want 0 (plain decimal, no overpunch)", pricing["usualAndCustomaryCharge"])
	}

	built, err := ncpdptelecombuilder.BuildTransmission(spec, "B1", "request", result.Header, result.TransmissionGroup, result.TransactionGroups)
	if err != nil {
		t.Fatalf("BuildTransmission error: %v", err)
	}

	reparsed, err := ncpdptelecom.ParseTransmission(spec, "request", built)
	if err != nil {
		t.Fatalf("re-parsing built transmission failed: %v", err)
	}
	if reparsed.Header["binNumber"] != result.Header["binNumber"] {
		t.Errorf("round trip: Header.binNumber = %v, want %v", reparsed.Header["binNumber"], result.Header["binNumber"])
	}
	reparsedPatient := reparsed.TransmissionGroup["Patient"].(map[string]interface{})
	if reparsedPatient["patientLastName"] != patient["patientLastName"] {
		t.Errorf("round trip: Patient.patientLastName = %v, want %v", reparsedPatient["patientLastName"], patient["patientLastName"])
	}
	reparsedClaim := groupAt(reparsed.TransactionGroups, 0)["Claim"].(map[string]interface{})
	if math.Abs(reparsedClaim["quantityDispensed"].(float64)-claim["quantityDispensed"].(float64)) > 0.001 {
		t.Errorf("round trip: Claim.quantityDispensed = %v, want %v", reparsedClaim["quantityDispensed"], claim["quantityDispensed"])
	}
	reparsedPricing := groupAt(reparsed.TransactionGroups, 0)["Pricing"].(map[string]interface{})
	if math.Abs(reparsedPricing["ingredientCostSubmitted"].(float64)-pricing["ingredientCostSubmitted"].(float64)) > 0.001 {
		t.Errorf("round trip: Pricing.ingredientCostSubmitted = %v, want %v", reparsedPricing["ingredientCostSubmitted"], pricing["ingredientCostSubmitted"])
	}
}

func TestB1Response_SelfAuthoredSample_ApprovedOutcome_ParseAndRoundTrip(t *testing.T) {
	spec := loadRealSpec(t)
	raw := buildTestB1Response()

	result, err := ncpdptelecom.ParseTransmission(spec, "response", raw)
	if err != nil {
		t.Fatalf("ParseTransmission error: %v", err)
	}
	if result.TransactionCode != "B1" {
		t.Errorf("TransactionCode = %q, want \"B1\"", result.TransactionCode)
	}

	msg, ok := result.TransmissionGroup["ResponseMessage"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected TransmissionGroup.ResponseMessage, got %v", result.TransmissionGroup["ResponseMessage"])
	}
	if msg["message"] != "Claim processed successfully" {
		t.Errorf("ResponseMessage.message = %v", msg["message"])
	}

	if len(result.TransactionGroups) != 1 {
		t.Fatalf("expected 1 transaction group (single-claim response), got %d", len(result.TransactionGroups))
	}
	status, ok := groupAt(result.TransactionGroups, 0)["ResponseStatus"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected TransactionGroups[0].ResponseStatus, got %v", groupAt(result.TransactionGroups, 0)["ResponseStatus"])
	}
	if status["responseStatus"] != "P" {
		t.Errorf("ResponseStatus.responseStatus = %v, want \"P\" (Paid)", status["responseStatus"])
	}
	if status["authorizationNumber"] != "1234567890" {
		t.Errorf("ResponseStatus.authorizationNumber = %v", status["authorizationNumber"])
	}

	pricing, ok := groupAt(result.TransactionGroups, 0)["ResponsePricing"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected TransactionGroups[0].ResponsePricing, got %v", groupAt(result.TransactionGroups, 0)["ResponsePricing"])
	}
	if total, ok := pricing["totalAmountPaid"].(float64); !ok || math.Abs(total-8.46) > 0.001 {
		t.Errorf("ResponsePricing.totalAmountPaid = %v, want 8.46", pricing["totalAmountPaid"])
	}

	built, err := ncpdptelecombuilder.BuildTransmission(spec, "B1", "response", result.Header, result.TransmissionGroup, result.TransactionGroups)
	if err != nil {
		t.Fatalf("BuildTransmission error: %v", err)
	}
	reparsed, err := ncpdptelecom.ParseTransmission(spec, "response", built)
	if err != nil {
		t.Fatalf("re-parsing built transmission failed: %v", err)
	}
	reparsedStatus := groupAt(reparsed.TransactionGroups, 0)["ResponseStatus"].(map[string]interface{})
	if reparsedStatus["responseStatus"] != status["responseStatus"] {
		t.Errorf("round trip: ResponseStatus.responseStatus = %v, want %v", reparsedStatus["responseStatus"], status["responseStatus"])
	}
}

// TestB1Response_MultipleTransactionGroups proves the GS-delimited
// repeating-cluster mechanism directly, mirroring apiv/dzero's own real
// fixture (two separate Response Status segments, one per claim, GS-
// separated) byte-for-byte.
func TestSniffDirection_DistinguishesRequestFromResponse(t *testing.T) {
	spec := loadRealSpec(t)

	if got := ncpdptelecom.SniffDirection(spec, buildTestB1Request()); got != "request" {
		t.Errorf("SniffDirection(request fixture) = %q, want \"request\"", got)
	}
	if got := ncpdptelecom.SniffDirection(spec, buildTestB1Response()); got != "response" {
		t.Errorf("SniffDirection(response fixture) = %q, want \"response\"", got)
	}
}

func TestB1Response_MultipleTransactionGroups_SplitsOnGS(t *testing.T) {
	spec := loadRealSpec(t)
	header := buildTestHeader("B1")
	body := rs + fs + "AM20" + fs + "F4TEST MESSAGE" + gs +
		rs + fs + "AM21" + fs + "ANC" + gs +
		rs + fs + "AM21" + fs + "ANR"
	raw := header + body

	result, err := ncpdptelecom.ParseTransmission(spec, "response", raw)
	if err != nil {
		t.Fatalf("ParseTransmission error: %v", err)
	}
	if len(result.TransactionGroups) != 2 {
		t.Fatalf("expected 2 transaction groups, got %d", len(result.TransactionGroups))
	}
	first := groupAt(result.TransactionGroups, 0)["ResponseStatus"].(map[string]interface{})
	if first["responseStatus"] != "C" {
		t.Errorf("TransactionGroups[0].ResponseStatus.responseStatus = %v, want \"C\"", first["responseStatus"])
	}
	second := groupAt(result.TransactionGroups, 1)["ResponseStatus"].(map[string]interface{})
	if second["responseStatus"] != "R" {
		t.Errorf("TransactionGroups[1].ResponseStatus.responseStatus = %v, want \"R\"", second["responseStatus"])
	}
}
