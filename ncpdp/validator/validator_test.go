package validator_test

import (
	"os"
	"testing"

	"ezhealthkonnect/ncpdp"
	"ezhealthkonnect/ncpdp/validator"

	"github.com/stretchr/testify/require"
)

func loadSpec(t *testing.T) *ncpdp.NCPDPSpecDef {
	t.Helper()
	loader, err := ncpdp.NewNCPDPSchemaLoader("../schemas/script_2017071")
	require.NoError(t, err)
	return loader.Spec()
}

func TestValidate_NewRxRealSample_NoErrorIssues(t *testing.T) {
	spec := loadSpec(t)
	raw, err := os.ReadFile("../testdata/real_samples/dgoradia_sample_newrx.xml")
	require.NoError(t, err)

	parsed, err := ncpdp.ParseMessage(spec, string(raw))
	require.NoError(t, err)

	result := validator.Validate(spec, parsed)
	require.True(t, result.Valid, "expected the real, complete NewRx sample to validate cleanly, got issues: %+v", result.Issues)
	for _, iss := range result.Issues {
		require.NotEqual(t, validator.SeverityError, iss.Severity, "unexpected error issue: %+v", iss)
	}
}

func TestValidate_MissingRequiredPrescriberName_ReturnsBlockingError(t *testing.T) {
	spec := loadSpec(t)
	parsed := &ncpdp.ParseResult{
		TransactionType: "NewRx",
		Header:          map[string]interface{}{"to": "1", "from": "2", "messageID": "m1", "sentTime": "2026-09-14T00:00:00Z"},
		Body: map[string]interface{}{
			"patient": map[string]interface{}{
				"humanPatient": map[string]interface{}{
					"name": map[string]interface{}{"lastName": "Doe", "firstName": "Jane"},
				},
			},
			"prescriber": map[string]interface{}{
				"nonVeterinarian": map[string]interface{}{
					"identification": map[string]interface{}{"npi": "1234567890"},
					// name deliberately omitted — required
				},
			},
			"medicationPrescribed": map[string]interface{}{
				"drugDescription": "Test Drug",
				"quantity":        map[string]interface{}{"value": "1"},
				"writtenDate":     map[string]interface{}{"date": "2026-09-14"},
				"sig":             map[string]interface{}{"sigText": "Take as directed"},
			},
		},
	}

	result := validator.Validate(spec, parsed)
	require.False(t, result.Valid)

	found := false
	for _, iss := range result.Issues {
		if iss.Severity == validator.SeverityError && iss.Path == "NewRx.prescriber.nonVeterinarian.name" {
			found = true
		}
	}
	require.True(t, found, "expected a blocking error for the missing required prescriber name, got: %+v", result.Issues)
}

func TestValidate_UnknownTransactionType_ReturnsBlockingError(t *testing.T) {
	spec := loadSpec(t)
	parsed := &ncpdp.ParseResult{
		TransactionType: "NotARealTransaction",
		Header:          map[string]interface{}{"to": "1", "from": "2", "messageID": "m1", "sentTime": "2026-09-14T00:00:00Z"},
		Body:            map[string]interface{}{},
	}

	result := validator.Validate(spec, parsed)
	require.False(t, result.Valid)

	found := false
	for _, iss := range result.Issues {
		if iss.Severity == validator.SeverityError && iss.Message == `unknown transaction type "NotARealTransaction"` {
			found = true
		}
	}
	require.True(t, found, "expected an unknown-transaction-type error, got: %+v", result.Issues)
}

func TestValidate_EmptyCancelRxResponse_NoRequiredFields_ValidatesCleanly(t *testing.T) {
	spec := loadSpec(t)
	parsed := &ncpdp.ParseResult{
		TransactionType: "CancelRxResponse",
		Header:          map[string]interface{}{"to": "1", "from": "2", "messageID": "m1", "sentTime": "2026-09-14T00:00:00Z"},
		Body:            map[string]interface{}{},
	}

	result := validator.Validate(spec, parsed)
	require.True(t, result.Valid, "CancelRxResponse has no required body fields/groups by design (an outcome choice, none mandatory in isolation): %+v", result.Issues)
}
