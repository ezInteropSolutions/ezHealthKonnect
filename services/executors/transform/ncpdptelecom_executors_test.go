// services/executors/transform/ncpdptelecom_executors_test.go
package transform

import (
	"context"
	"testing"

	"ezhealthkonnect/models"
	"ezhealthkonnect/ncpdptelecom"
	"ezhealthkonnect/services/executors"
	ncpdptelecomparser "ezhealthkonnect/services/parsers/ncpdptelecom"

	"github.com/stretchr/testify/require"
)

// newTestNCPDPTelecomExecutors constructs all 4 ncpdptelecom.* executors
// with real schema loaders without going through their New*Executor()'s
// hardcoded "./ncpdptelecom/schemas/telecom_d0" path — that path is
// relative to the running process's CWD (correct at server runtime), not
// this test package's directory (`go test` runs with CWD = the package
// dir). Same fix newTestNCPDPExecutors already applies.
func newTestNCPDPTelecomExecutors(t *testing.T) (*NCPDPTelecomParseExecutor, *NCPDPTelecomValidateExecutor, *NCPDPTelecomBuildExecutor, *NCPDPTelecomMapToCanonicalExecutor) {
	t.Helper()
	const schemaDir = "../../../ncpdptelecom/schemas/telecom_d0"

	parser, err := ncpdptelecomparser.NewFromSchemaDir(schemaDir)
	require.NoError(t, err)
	loader, err := ncpdptelecom.NewTelecomSchemaLoader(schemaDir)
	require.NoError(t, err)

	parseExec := &NCPDPTelecomParseExecutor{
		BaseExecutor: executors.NewBaseExecutor("ncpdptelecom.parse", models.ExecutorMetadata{Name: "NCPDP Telecom D.0 Parser", Category: "NCPDP Telecom Transform"}),
		parser:       parser,
	}
	validateExec := &NCPDPTelecomValidateExecutor{
		BaseExecutor: executors.NewBaseExecutor("ncpdptelecom.validate", models.ExecutorMetadata{Name: "NCPDP Telecom D.0 Validator", Category: "NCPDP Telecom Transform"}),
		loader:       loader,
	}
	buildExec := &NCPDPTelecomBuildExecutor{
		BaseExecutor: executors.NewBaseExecutor("ncpdptelecom.build", models.ExecutorMetadata{Name: "NCPDP Telecom D.0 Builder", Category: "NCPDP Telecom Transform"}),
		loader:       loader,
	}
	mapExec := &NCPDPTelecomMapToCanonicalExecutor{
		BaseExecutor: executors.NewBaseExecutor("ncpdptelecom.map_to_canonical", models.ExecutorMetadata{Name: "Map to Canonical NCPDP Telecom D.0 JSON", Category: "NCPDP Telecom Transform"}),
		loader:       loader,
	}
	return parseExec, validateExec, buildExec, mapExec
}

// buildTestB1RequestRaw is the same self-authored fixture-construction logic
// as ncpdptelecom/roundtrip_test.go's own buildTestB1Request — duplicated
// here (rather than exported cross-package) since it's small and each
// package's own test fixtures should stay independently readable, matching
// this codebase's own precedent elsewhere (e.g. EDI's per-package sample
// duplication).
func buildTestB1RequestRaw() string {
	const rs, fs = "\x1E", "\x1C"
	header := "999999" + "D0" + "B1" +
		"          " + // processorControlNumber
		"1" + "01" +
		"1111111111     " + // serviceProviderId (15)
		"20260919" +
		"          " // software (10)
	body := rs + fs + "AM01" + fs + "CBSMITH" + fs + "CAJOHN" +
		rs + fs + "AM02" + fs + "EY01" + fs + "E91234567890" +
		rs + fs + "AM04" + fs + "C2123456789012" +
		rs + fs + "AM07" + fs + "D2000000123456" + fs + "E103" + fs + "D700003089421" + fs + "E70000030000" + fs + "D301" + fs + "D5030" + fs + "DE20220924" +
		rs + fs + "AM11" + fs + "D90000057A" + fs + "DC0000027E" + fs + "DX0000016B" + fs + "DQ00000000" + fs + "DU0000084F"
	return header + body
}

// TestNCPDPTelecomParseValidateBuild_RealB1Sample_FullChain exercises
// ncpdptelecom.parse -> ncpdptelecom.validate -> ncpdptelecom.build as a
// real pipeline chain, confirming each step's output correctly feeds the
// next via the SAME field-name conventions the real pipeline engine uses.
func TestNCPDPTelecomParseValidateBuild_RealB1Sample_FullChain(t *testing.T) {
	parseExec, validateExec, buildExec, _ := newTestNCPDPTelecomExecutors(t)
	raw := buildTestB1RequestRaw()

	parseStep := &models.TransformationStep{
		StepName: "Parse D.0", StepType: "ncpdptelecom.parse", Enabled: true,
		Config: map[string]interface{}{"sourceField": "raw", "direction": "request", "outputField": "parsedTelecom"},
	}
	afterParse, err := parseExec.Execute(context.Background(), parseStep, map[string]interface{}{"raw": raw})
	require.NoError(t, err)

	parsed, ok := afterParse["parsedTelecom"].(map[string]interface{})
	require.True(t, ok, "expected parsedTelecom to be a map")
	require.Equal(t, "B1", parsed["transactionCode"])
	require.Equal(t, "request", parsed["direction"])

	validateStep := &models.TransformationStep{
		StepName: "Validate D.0", StepType: "ncpdptelecom.validate", Enabled: true,
		Config: map[string]interface{}{"sourceField": "parsedTelecom", "outputField": "telecomValidation"},
	}
	afterValidate, err := validateExec.Execute(context.Background(), validateStep, afterParse)
	require.NoError(t, err)

	validation, ok := afterValidate["telecomValidation"].(map[string]interface{})
	require.True(t, ok, "expected telecomValidation to be a map")
	require.Equal(t, true, validation["valid"], "expected the complete B1 fixture to validate cleanly: %+v", validation["issues"])

	buildStep := &models.TransformationStep{
		StepName: "Build D.0", StepType: "ncpdptelecom.build", Enabled: true,
		Config: map[string]interface{}{"sourceField": "parsedTelecom", "transactionCode": "B1", "direction": "request", "outputField": "ncpdpTelecom"},
	}
	afterBuild, err := buildExec.Execute(context.Background(), buildStep, afterValidate)
	require.NoError(t, err)

	built, ok := afterBuild["ncpdpTelecom"].(string)
	require.True(t, ok, "expected ncpdpTelecom to be a string")
	require.Contains(t, built, "AM01")
	require.Contains(t, built, "SMITH")

	// Round-trip: the built transmission must itself parse back correctly.
	reparsed, err := ncpdptelecom.ParseTransmission(parseExec.parser.Spec(), "request", built)
	require.NoError(t, err)
	require.Equal(t, "B1", reparsed.TransactionCode)
}

// TestNCPDPTelecomValidateExecutor_MessageUnwrapAndRawFallback verifies
// both the inputData["message"] unwrap fallback and the plain-raw-content
// fallback (no prior ncpdptelecom.parse step in the pipeline) work correctly.
func TestNCPDPTelecomValidateExecutor_MessageUnwrapAndRawFallback(t *testing.T) {
	_, validateExec, _, _ := newTestNCPDPTelecomExecutors(t)
	raw := buildTestB1RequestRaw()

	step := &models.TransformationStep{
		StepName: "Validate D.0", StepType: "ncpdptelecom.validate", Enabled: true,
		Config: map[string]interface{}{"sourceField": "raw", "direction": "request", "outputField": "telecomValidation"},
	}
	// No prior ncpdptelecom.parse step ran — sourceField "raw" holds
	// content text, not a ParsedJSON map, exercising the string-fallback
	// branch of resolveNCPDPTelecomParseResult.
	output, err := validateExec.Execute(context.Background(), step, map[string]interface{}{
		"message": map[string]interface{}{"raw": raw},
	})
	require.NoError(t, err)

	validation, ok := output["telecomValidation"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, true, validation["valid"])
}

// TestNCPDPTelecomValidateExecutor_MissingRequiredField_ReturnsInvalidNotError
// verifies a structurally incomplete transmission surfaces as valid=false
// with issues, WITHOUT Execute() itself returning a Go error — the same
// "validation failure is data, not a pipeline fault" contract every other
// *.validate executor in this codebase establishes.
func TestNCPDPTelecomValidateExecutor_MissingRequiredField_ReturnsInvalidNotError(t *testing.T) {
	_, validateExec, _, _ := newTestNCPDPTelecomExecutors(t)

	step := &models.TransformationStep{
		StepName: "Validate D.0", StepType: "ncpdptelecom.validate", Enabled: true,
		Config: map[string]interface{}{"sourceField": "parsedTelecom", "outputField": "telecomValidation"},
	}
	incomplete := map[string]interface{}{
		"parsedTelecom": map[string]interface{}{
			"transactionCode":   "B1",
			"direction":         "request",
			"header":            map[string]interface{}{},
			"transmissionGroup": map[string]interface{}{},
			"transactionGroups": []interface{}{},
		},
	}
	output, err := validateExec.Execute(context.Background(), step, incomplete)
	require.NoError(t, err)

	validation := output["telecomValidation"].(map[string]interface{})
	require.Equal(t, false, validation["valid"])
	issues, ok := validation["issues"].([]map[string]interface{})
	require.True(t, ok)
	require.NotEmpty(t, issues)
}

// TestNCPDPTelecomMapToCanonical_ThenBuild_ProducesValidB1 proves the
// no-code mapping step end-to-end: arbitrary "source row" data (as if from
// a CSV/DB query) mapped into canonical D.0 JSON, then built into a real B1
// transmission, exercising the transaction-group rowsPath mechanism with
// TWO claims in one transmission.
func TestNCPDPTelecomMapToCanonical_ThenBuild_ProducesValidB1(t *testing.T) {
	_, _, buildExec, mapExec := newTestNCPDPTelecomExecutors(t)

	sourceRow := map[string]interface{}{
		"patient_last_name": "DOE",
		"cardholder_id":     "987654321",
		"claims": []map[string]interface{}{
			{"rx_ref": "000000000001", "ndc": "00003089421", "qty": "30", "fill_num": "1", "days_supply": "30", "written_date": "20220101"},
			{"rx_ref": "000000000002", "ndc": "00003089999", "qty": "60", "fill_num": "2", "days_supply": "60", "written_date": "20220201"},
		},
	}

	mapStep := &models.TransformationStep{
		StepName: "Map to Canonical D.0", StepType: "ncpdptelecom.map_to_canonical", Enabled: true,
		Config: map[string]interface{}{
			"outputField":     "parsedTelecom",
			"transactionCode": "B1",
			"direction":       "request",
			"transmissionGroupSegments": []map[string]interface{}{
				{"segmentKey": "Patient", "fields": []map[string]interface{}{
					{"fieldKey": "patientLastName", "sourcePath": "patient_last_name"},
				}},
				{"segmentKey": "Insurance", "fields": []map[string]interface{}{
					{"fieldKey": "cardholderId", "sourcePath": "cardholder_id"},
				}},
			},
			"transactionGroupRowsPath": "claims",
			"transactionGroupSegments": []map[string]interface{}{
				{"segmentKey": "Claim", "fields": []map[string]interface{}{
					{"fieldKey": "prescriptionReferenceNumber", "sourcePath": "rx_ref"},
					{"fieldKey": "productServiceIdQualifier", "literalValue": "03"},
					{"fieldKey": "productServiceId", "sourcePath": "ndc"},
					{"fieldKey": "quantityDispensed", "sourcePath": "qty"},
					{"fieldKey": "fillNumber", "sourcePath": "fill_num"},
					{"fieldKey": "daysSupply", "sourcePath": "days_supply"},
					{"fieldKey": "datePrescriptionWritten", "sourcePath": "written_date"},
				}},
			},
		},
	}

	afterMap, err := mapExec.Execute(context.Background(), mapStep, sourceRow)
	require.NoError(t, err)

	canonical, ok := afterMap["parsedTelecom"].(map[string]interface{})
	require.True(t, ok)
	transmissionGroup := canonical["transmissionGroup"].(map[string]interface{})
	patient := transmissionGroup["Patient"].(map[string]interface{})
	require.Equal(t, "DOE", patient["patientLastName"])

	transactionGroups := canonical["transactionGroups"].([]interface{})
	require.Len(t, transactionGroups, 2, "expected 2 transaction groups, one per claim row")
	firstClaim := transactionGroups[0].(map[string]interface{})["Claim"].(map[string]interface{})
	require.Equal(t, "000000000001", firstClaim["prescriptionReferenceNumber"])
	secondClaim := transactionGroups[1].(map[string]interface{})["Claim"].(map[string]interface{})
	require.Equal(t, "000000000002", secondClaim["prescriptionReferenceNumber"])

	buildStep := &models.TransformationStep{
		StepName: "Build D.0", StepType: "ncpdptelecom.build", Enabled: true,
		Config: map[string]interface{}{"sourceField": "parsedTelecom", "outputField": "ncpdpTelecom"},
	}
	afterBuild, err := buildExec.Execute(context.Background(), buildStep, afterMap)
	require.NoError(t, err)

	built := afterBuild["ncpdpTelecom"].(string)
	require.Contains(t, built, "DOE")
	require.Contains(t, built, "000000000001")
	require.Contains(t, built, "000000000002")

	// Round-trip: the built transmission must itself parse back correctly
	// through the real parser, proving the mapped-then-built document is
	// genuinely well-formed D.0, not just string-contains-the-right-bits.
	loader, err := ncpdptelecom.NewTelecomSchemaLoader("../../../ncpdptelecom/schemas/telecom_d0")
	require.NoError(t, err)
	reparsed, err := ncpdptelecom.ParseTransmission(loader.Spec(), "request", built)
	require.NoError(t, err)
	require.Equal(t, "B1", reparsed.TransactionCode)
	require.Len(t, reparsed.TransactionGroups, 2)
}
