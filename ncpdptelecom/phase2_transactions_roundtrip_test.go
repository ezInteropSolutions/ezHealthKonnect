package ncpdptelecom_test

// Round-trip tests for the 3 transaction codes added on top of Phase 1's B1
// (Claim Billing) build: B2 (Claim Reversal), B3 (Claim Rebill), and E1
// (Eligibility Verification). All 3 are sourced from a single real, official
// payer sheet -- see each schema file's own sourceRefs under
// schemas/telecom_d0/transactions/{B2,B3,E1}_*.json for the exact citation
// (State of North Dakota Medicaid's own published NCPDP D.0 payer sheet,
// read directly via pdftotext, not summarized or guessed).
//
// Self-authored fixtures, same discipline as roundtrip_test.go's own B1
// fixtures (there is no free real D.0 sample of ANY transaction code --
// NCPDP's own Implementation Guide is a paid document).

import (
	"strings"
	"testing"

	"ezhealthkonnect/ncpdptelecom"
	ncpdptelecombuilder "ezhealthkonnect/ncpdptelecom/builder"
)

// --- B2 (Claim Reversal) ---------------------------------------------------

func buildTestB2Request() string {
	header := buildTestHeader("B2")
	body := rs + fs + "AM04" + fs + "C2123456789012" +
		rs + fs + "AM07" + fs + "EM1" + fs + "D2000000123456"
	return header + body
}

func buildTestB2ResponseApproved() string {
	header := buildTestHeader("B2")
	body := rs + fs + "AM21" + fs + "ANA" +
		rs + fs + "AM22" + fs + "EM1" + fs + "D2000000123456"
	return header + body
}

func buildTestB2ResponseRejected() string {
	header := buildTestHeader("B2")
	body := rs + fs + "AM21" + fs + "ANR" + fs + "FA1" + fs + "FB88" +
		rs + fs + "AM22" + fs + "EM1" + fs + "D2000000123456"
	return header + body
}

func TestB2Request_SelfAuthoredSample_ParseAndRoundTrip(t *testing.T) {
	spec := loadRealSpec(t)
	raw := buildTestB2Request()

	result, err := ncpdptelecom.ParseTransmission(spec, "request", raw)
	if err != nil {
		t.Fatalf("ParseTransmission error: %v", err)
	}
	if result.TransactionCode != "B2" {
		t.Errorf("TransactionCode = %q, want \"B2\"", result.TransactionCode)
	}

	insurance, ok := result.TransmissionGroup["Insurance"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected TransmissionGroup.Insurance, got %v", result.TransmissionGroup["Insurance"])
	}
	if insurance["cardholderId"] != "123456789012" {
		t.Errorf("Insurance.cardholderId = %v, want \"123456789012\"", insurance["cardholderId"])
	}
	if _, present := result.TransmissionGroup["Patient"]; present {
		t.Errorf("B2 request must not carry a Patient segment (excluded from the real payer sheet's own template), got %v", result.TransmissionGroup["Patient"])
	}

	if len(result.TransactionGroups) != 1 {
		t.Fatalf("expected 1 transaction group, got %d", len(result.TransactionGroups))
	}
	claim, ok := groupAt(result.TransactionGroups, 0)["Claim"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected TransactionGroups[0].Claim, got %v", groupAt(result.TransactionGroups, 0)["Claim"])
	}
	if claim["prescriptionReferenceNumber"] != "000000123456" {
		t.Errorf("Claim.prescriptionReferenceNumber = %v", claim["prescriptionReferenceNumber"])
	}
	if _, present := claim["quantityDispensed"]; present {
		t.Errorf("B2's own Claim segment must not carry quantityDispensed (excluded from the real payer sheet's own B2 template), got %v", claim["quantityDispensed"])
	}

	built, err := ncpdptelecombuilder.BuildTransmission(spec, "B2", "request", result.Header, result.TransmissionGroup, result.TransactionGroups)
	if err != nil {
		t.Fatalf("BuildTransmission error: %v", err)
	}
	reparsed, err := ncpdptelecom.ParseTransmission(spec, "request", built)
	if err != nil {
		t.Fatalf("re-parsing built transmission failed: %v", err)
	}
	reparsedClaim := groupAt(reparsed.TransactionGroups, 0)["Claim"].(map[string]interface{})
	if reparsedClaim["prescriptionReferenceNumber"] != claim["prescriptionReferenceNumber"] {
		t.Errorf("round trip: Claim.prescriptionReferenceNumber = %v, want %v", reparsedClaim["prescriptionReferenceNumber"], claim["prescriptionReferenceNumber"])
	}
}

func TestB2Response_SelfAuthoredSample_ApprovedAndRejected_ParseAndRoundTrip(t *testing.T) {
	spec := loadRealSpec(t)

	t.Run("approved", func(t *testing.T) {
		result, err := ncpdptelecom.ParseTransmission(spec, "response", buildTestB2ResponseApproved())
		if err != nil {
			t.Fatalf("ParseTransmission error: %v", err)
		}
		if len(result.TransmissionGroup) != 0 {
			t.Errorf("B2 response has no transmission-group segments, got %v", result.TransmissionGroup)
		}
		status := groupAt(result.TransactionGroups, 0)["ResponseStatus"].(map[string]interface{})
		if status["responseStatus"] != "A" {
			t.Errorf("ResponseStatus.responseStatus = %v, want \"A\"", status["responseStatus"])
		}
		if _, present := groupAt(result.TransactionGroups, 0)["ResponsePricing"]; present {
			t.Error("B2 response must never carry a ResponsePricing segment (a reversal has nothing to price)")
		}

		built, err := ncpdptelecombuilder.BuildTransmission(spec, "B2", "response", result.Header, result.TransmissionGroup, result.TransactionGroups)
		if err != nil {
			t.Fatalf("BuildTransmission error: %v", err)
		}
		reparsed, err := ncpdptelecom.ParseTransmission(spec, "response", built)
		if err != nil {
			t.Fatalf("re-parsing built transmission failed: %v", err)
		}
		if groupAt(reparsed.TransactionGroups, 0)["ResponseStatus"].(map[string]interface{})["responseStatus"] != "A" {
			t.Error("round trip lost ResponseStatus.responseStatus")
		}
	})

	t.Run("rejected", func(t *testing.T) {
		result, err := ncpdptelecom.ParseTransmission(spec, "response", buildTestB2ResponseRejected())
		if err != nil {
			t.Fatalf("ParseTransmission error: %v", err)
		}
		status := groupAt(result.TransactionGroups, 0)["ResponseStatus"].(map[string]interface{})
		if status["responseStatus"] != "R" {
			t.Errorf("ResponseStatus.responseStatus = %v, want \"R\"", status["responseStatus"])
		}
		if status["rejectCode"] != "88" {
			t.Errorf("ResponseStatus.rejectCode = %v, want \"88\"", status["rejectCode"])
		}
	})
}

// --- B3 (Claim Rebill) ------------------------------------------------------
// B3 is confirmed structurally IDENTICAL to B1 (same payer sheet, same
// template, only the transaction code differs) -- these tests reuse B1's own
// fixture builders verbatim with the code swapped, proving the schema
// registration resolves correctly rather than re-deriving B1's own already-
// proven parsing logic a second time.

func buildTestB3Request() string {
	return strings.Replace(buildTestB1Request(), "999999D0B1", "999999D0B3", 1)
}

func buildTestB3Response() string {
	return strings.Replace(buildTestB1Response(), "999999D0B1", "999999D0B3", 1)
}

func TestB3Request_SelfAuthoredSample_ParseAndRoundTrip(t *testing.T) {
	spec := loadRealSpec(t)
	raw := buildTestB3Request()

	result, err := ncpdptelecom.ParseTransmission(spec, "request", raw)
	if err != nil {
		t.Fatalf("ParseTransmission error: %v", err)
	}
	if result.TransactionCode != "B3" {
		t.Errorf("TransactionCode = %q, want \"B3\"", result.TransactionCode)
	}
	patient, ok := result.TransmissionGroup["Patient"].(map[string]interface{})
	if !ok || patient["patientLastName"] != "SMITH" {
		t.Fatalf("expected Patient.patientLastName = SMITH, got %v", result.TransmissionGroup["Patient"])
	}
	if len(result.TransactionGroups) != 1 {
		t.Fatalf("expected 1 transaction group, got %d", len(result.TransactionGroups))
	}
	if _, ok := groupAt(result.TransactionGroups, 0)["Pricing"].(map[string]interface{}); !ok {
		t.Fatal("expected TransactionGroups[0].Pricing (B3 uses the full B1 shape, including Pricing)")
	}

	built, err := ncpdptelecombuilder.BuildTransmission(spec, "B3", "request", result.Header, result.TransmissionGroup, result.TransactionGroups)
	if err != nil {
		t.Fatalf("BuildTransmission error: %v", err)
	}
	reparsed, err := ncpdptelecom.ParseTransmission(spec, "request", built)
	if err != nil {
		t.Fatalf("re-parsing built transmission failed: %v", err)
	}
	if reparsed.TransactionCode != "B3" {
		t.Errorf("round trip: TransactionCode = %q, want \"B3\"", reparsed.TransactionCode)
	}
}

func TestB3Response_SelfAuthoredSample_ParseAndRoundTrip(t *testing.T) {
	spec := loadRealSpec(t)
	raw := buildTestB3Response()

	result, err := ncpdptelecom.ParseTransmission(spec, "response", raw)
	if err != nil {
		t.Fatalf("ParseTransmission error: %v", err)
	}
	if result.TransactionCode != "B3" {
		t.Errorf("TransactionCode = %q, want \"B3\"", result.TransactionCode)
	}
	status, ok := groupAt(result.TransactionGroups, 0)["ResponseStatus"].(map[string]interface{})
	if !ok || status["responseStatus"] != "P" {
		t.Fatalf("expected ResponseStatus.responseStatus = P, got %v", groupAt(result.TransactionGroups, 0)["ResponseStatus"])
	}

	built, err := ncpdptelecombuilder.BuildTransmission(spec, "B3", "response", result.Header, result.TransmissionGroup, result.TransactionGroups)
	if err != nil {
		t.Fatalf("BuildTransmission error: %v", err)
	}
	if _, err := ncpdptelecom.ParseTransmission(spec, "response", built); err != nil {
		t.Fatalf("re-parsing built transmission failed: %v", err)
	}
}

// --- E1 (Eligibility Verification) -----------------------------------------
// E1 has NO transaction-group/claim-line-item concept at all -- confirmed
// directly from the real payer sheet (no Claim/Pricing segment appears
// anywhere in its own template) and from this engine's own schema
// (E1_request.json/E1_response.json both declare an empty
// transactionGroupSegments list). These fixtures therefore carry only
// transmission-group segments and zero GS separators.

func buildTestE1Request() string {
	header := buildTestHeader("E1")
	body := rs + fs + "AM04" + fs + "C2123456789012" + fs + "C61" +
		rs + fs + "AM01" + fs + "CBSMITH" + fs + "CAJOHN" + fs + "C419800101" +
		rs + fs + "AM03" + fs + "EZ01" + fs + "DB1234567893" + fs + "DRWELBY"
	return header + body
}

func buildTestE1ResponseApproved() string {
	header := buildTestHeader("E1")
	body := rs + fs + "AM21" + fs + "ANA"
	return header + body
}

func buildTestE1ResponseRejected() string {
	header := buildTestHeader("E1")
	body := rs + fs + "AM21" + fs + "ANR" + fs + "FA1" + fs + "FB65"
	return header + body
}

func TestE1Request_SelfAuthoredSample_ParseAndRoundTrip(t *testing.T) {
	spec := loadRealSpec(t)
	raw := buildTestE1Request()

	result, err := ncpdptelecom.ParseTransmission(spec, "request", raw)
	if err != nil {
		t.Fatalf("ParseTransmission error: %v", err)
	}
	if result.TransactionCode != "E1" {
		t.Errorf("TransactionCode = %q, want \"E1\"", result.TransactionCode)
	}
	if len(result.TransactionGroups) != 0 {
		t.Errorf("E1 has no transaction-group concept, expected 0 groups, got %d", len(result.TransactionGroups))
	}

	insurance, ok := result.TransmissionGroup["Insurance"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected TransmissionGroup.Insurance, got %v", result.TransmissionGroup["Insurance"])
	}
	if insurance["patientRelationshipCode"] != "1" {
		t.Errorf("Insurance.patientRelationshipCode = %v, want \"1\" (Self/Cardholder)", insurance["patientRelationshipCode"])
	}

	patient, ok := result.TransmissionGroup["Patient"].(map[string]interface{})
	if !ok || patient["patientLastName"] != "SMITH" {
		t.Fatalf("expected Patient.patientLastName = SMITH, got %v", result.TransmissionGroup["Patient"])
	}

	prescriber, ok := result.TransmissionGroup["Prescriber"].(map[string]interface{})
	if !ok || prescriber["prescriberLastName"] != "WELBY" {
		t.Fatalf("expected Prescriber.prescriberLastName = WELBY, got %v", result.TransmissionGroup["Prescriber"])
	}

	if _, present := result.TransmissionGroup["PharmacyProvider"]; present {
		t.Error("E1 request must not carry a Pharmacy Provider segment (excluded from the real payer sheet's own template)")
	}

	built, err := ncpdptelecombuilder.BuildTransmission(spec, "E1", "request", result.Header, result.TransmissionGroup, result.TransactionGroups)
	if err != nil {
		t.Fatalf("BuildTransmission error: %v", err)
	}
	reparsed, err := ncpdptelecom.ParseTransmission(spec, "request", built)
	if err != nil {
		t.Fatalf("re-parsing built transmission failed: %v", err)
	}
	reparsedPatient := reparsed.TransmissionGroup["Patient"].(map[string]interface{})
	if reparsedPatient["patientLastName"] != patient["patientLastName"] {
		t.Errorf("round trip: Patient.patientLastName = %v, want %v", reparsedPatient["patientLastName"], patient["patientLastName"])
	}
}

func TestE1Response_SelfAuthoredSample_ApprovedAndRejected_ParseAndRoundTrip(t *testing.T) {
	spec := loadRealSpec(t)

	t.Run("approved", func(t *testing.T) {
		result, err := ncpdptelecom.ParseTransmission(spec, "response", buildTestE1ResponseApproved())
		if err != nil {
			t.Fatalf("ParseTransmission error: %v", err)
		}
		status, ok := result.TransmissionGroup["ResponseStatus"].(map[string]interface{})
		if !ok || status["responseStatus"] != "A" {
			t.Fatalf("expected ResponseStatus.responseStatus = A, got %v", result.TransmissionGroup["ResponseStatus"])
		}
		if len(result.TransactionGroups) != 0 {
			t.Errorf("E1 response has no transaction-group concept, expected 0 groups, got %d", len(result.TransactionGroups))
		}

		built, err := ncpdptelecombuilder.BuildTransmission(spec, "E1", "response", result.Header, result.TransmissionGroup, result.TransactionGroups)
		if err != nil {
			t.Fatalf("BuildTransmission error: %v", err)
		}
		if _, err := ncpdptelecom.ParseTransmission(spec, "response", built); err != nil {
			t.Fatalf("re-parsing built transmission failed: %v", err)
		}
	})

	t.Run("rejected", func(t *testing.T) {
		result, err := ncpdptelecom.ParseTransmission(spec, "response", buildTestE1ResponseRejected())
		if err != nil {
			t.Fatalf("ParseTransmission error: %v", err)
		}
		status := result.TransmissionGroup["ResponseStatus"].(map[string]interface{})
		if status["responseStatus"] != "R" {
			t.Errorf("ResponseStatus.responseStatus = %v, want \"R\"", status["responseStatus"])
		}
		if status["rejectCode"] != "65" {
			t.Errorf("ResponseStatus.rejectCode = %v, want \"65\"", status["rejectCode"])
		}
	})
}
