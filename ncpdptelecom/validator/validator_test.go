package validator_test

import (
	"testing"

	"ezhealthkonnect/ncpdptelecom"
	"ezhealthkonnect/ncpdptelecom/validator"
)

func loadSpec(t *testing.T) *ncpdptelecom.TelecomSpecDef {
	t.Helper()
	loader, err := ncpdptelecom.NewTelecomSchemaLoader("../schemas/telecom_d0")
	if err != nil {
		t.Fatalf("failed to load D.0 schema: %v", err)
	}
	return loader.Spec()
}

func TestValidate_CompleteB1Request_NoErrorIssues(t *testing.T) {
	spec := loadSpec(t)
	result := &ncpdptelecom.ParseResult{
		TransactionCode: "B1",
		Direction:       "request",
		Header: map[string]interface{}{
			"binNumber": "999999", "version": "D0", "transactionCode": "B1",
			"transactionCount": float64(1), "serviceProviderIdQualifier": "01",
			"serviceProviderId": "1111111111", "dateOfService": "20260919",
		},
		TransmissionGroup: map[string]interface{}{
			"Patient":          map[string]interface{}{"patientLastName": "SMITH"},
			"PharmacyProvider": map[string]interface{}{"providerIdQualifier": "01", "providerId": "1234567890"},
			"Insurance":        map[string]interface{}{"cardholderId": "123456789012"},
		},
		TransactionGroups: []interface{}{
			map[string]interface{}{
				"Claim": map[string]interface{}{
					"prescriptionReferenceNumber": "000000123456", "productServiceIdQualifier": "03",
					"productServiceId": "00003089421", "quantityDispensed": float64(30),
					"fillNumber": float64(1), "daysSupply": float64(30), "datePrescriptionWritten": "20220924",
				},
				"Pricing": map[string]interface{}{
					"ingredientCostSubmitted": float64(5.71), "usualAndCustomaryCharge": float64(0), "grossAmountDue": float64(8.46),
				},
			},
		},
	}

	got := validator.Validate(spec, result)
	if !got.Valid {
		t.Errorf("expected a complete B1 request to validate cleanly, got issues: %+v", got.Issues)
	}
}

func TestValidate_MissingRequiredHeaderField_ReturnsBlockingError(t *testing.T) {
	spec := loadSpec(t)
	result := &ncpdptelecom.ParseResult{
		TransactionCode:   "B1",
		Direction:         "request",
		Header:            map[string]interface{}{"version": "D0", "transactionCode": "B1"}, // binNumber deliberately omitted
		TransmissionGroup: map[string]interface{}{},
		TransactionGroups: nil,
	}

	got := validator.Validate(spec, result)
	if got.Valid {
		t.Fatal("expected validation to fail for a missing required header field")
	}
	found := false
	for _, iss := range got.Issues {
		if iss.Severity == validator.SeverityError && iss.Path == "Header.binNumber" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a blocking error for the missing binNumber header field, got: %+v", got.Issues)
	}
}

func TestValidate_MissingRequiredSegment_ReturnsBlockingError(t *testing.T) {
	spec := loadSpec(t)
	result := &ncpdptelecom.ParseResult{
		TransactionCode: "B1",
		Direction:       "request",
		Header: map[string]interface{}{
			"binNumber": "999999", "version": "D0", "transactionCode": "B1",
			"transactionCount": float64(1), "serviceProviderIdQualifier": "01",
			"serviceProviderId": "1111111111", "dateOfService": "20260919",
		},
		TransmissionGroup: map[string]interface{}{"Patient": map[string]interface{}{"patientLastName": "SMITH"}}, // Insurance deliberately omitted — required
		TransactionGroups: []interface{}{map[string]interface{}{
			"Claim": map[string]interface{}{
				"prescriptionReferenceNumber": "000000123456", "productServiceIdQualifier": "03",
				"productServiceId": "00003089421", "quantityDispensed": float64(30),
				"fillNumber": float64(1), "daysSupply": float64(30), "datePrescriptionWritten": "20220924",
			},
			"Pricing": map[string]interface{}{"ingredientCostSubmitted": float64(5.71), "usualAndCustomaryCharge": float64(0), "grossAmountDue": float64(8.46)},
		}},
	}

	got := validator.Validate(spec, result)
	if got.Valid {
		t.Fatal("expected validation to fail for a missing required Insurance segment")
	}
	found := false
	for _, iss := range got.Issues {
		if iss.Severity == validator.SeverityError && iss.Path == "TransmissionGroup.Insurance" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a blocking error for the missing Insurance segment, got: %+v", got.Issues)
	}
}

func TestValidate_UnknownTransaction_ReturnsBlockingError(t *testing.T) {
	spec := loadSpec(t)
	result := &ncpdptelecom.ParseResult{TransactionCode: "ZZ", Direction: "request", Header: map[string]interface{}{}}

	got := validator.Validate(spec, result)
	if got.Valid {
		t.Fatal("expected validation to fail for an unknown transaction")
	}
}

func TestValidate_NoTransactionGroupsButSegmentsRequired_ReturnsBlockingError(t *testing.T) {
	spec := loadSpec(t)
	result := &ncpdptelecom.ParseResult{
		TransactionCode: "B1",
		Direction:       "request",
		Header: map[string]interface{}{
			"binNumber": "999999", "version": "D0", "transactionCode": "B1",
			"transactionCount": float64(1), "serviceProviderIdQualifier": "01",
			"serviceProviderId": "1111111111", "dateOfService": "20260919",
		},
		TransmissionGroup: map[string]interface{}{
			"Patient": map[string]interface{}{"patientLastName": "SMITH"}, "Insurance": map[string]interface{}{"cardholderId": "1"},
		},
		TransactionGroups: nil, // no claim at all — Claim/Pricing are required transaction-group segments
	}

	got := validator.Validate(spec, result)
	if got.Valid {
		t.Fatal("expected validation to fail when no transaction groups are present but Claim/Pricing are required")
	}
}
