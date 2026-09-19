// services/executors/transform/ncpdp_executors_test.go
package transform

import (
	"context"
	"os"
	"testing"

	"ezhealthkonnect/models"
	"ezhealthkonnect/ncpdp"
	"ezhealthkonnect/services/executors"
	ncpdpscriptparser "ezhealthkonnect/services/parsers/ncpdpscript"

	"github.com/stretchr/testify/require"
)

// newTestNCPDPExecutors constructs all 4 ncpdp.* executors with real schema
// loaders without going through their New*Executor()'s hardcoded
// "./ncpdp/schemas/script_2017071" path — that path is relative to the
// running process's CWD (correct at server runtime, from the repo root), not
// this test package's directory (`go test` runs with CWD = the package
// dir). Same fix newTestEDIParseExecutor already applies for edi.parse.
func newTestNCPDPExecutors(t *testing.T) (*NCPDPParseExecutor, *NCPDPValidateExecutor, *NCPDPBuildExecutor, *NCPDPMapToCanonicalExecutor) {
	t.Helper()
	const schemaDir = "../../../ncpdp/schemas/script_2017071"

	parser, err := ncpdpscriptparser.NewFromSchemaDir(schemaDir)
	require.NoError(t, err)
	loader, err := ncpdp.NewNCPDPSchemaLoader(schemaDir)
	require.NoError(t, err)

	parseExec := &NCPDPParseExecutor{
		BaseExecutor: executors.NewBaseExecutor("ncpdp.parse", models.ExecutorMetadata{Name: "NCPDP SCRIPT Parser", Category: "NCPDP Transform"}),
		parser:       parser,
	}
	validateExec := &NCPDPValidateExecutor{
		BaseExecutor: executors.NewBaseExecutor("ncpdp.validate", models.ExecutorMetadata{Name: "NCPDP SCRIPT Validator", Category: "NCPDP Transform"}),
		loader:       loader,
	}
	buildExec := &NCPDPBuildExecutor{
		BaseExecutor: executors.NewBaseExecutor("ncpdp.build", models.ExecutorMetadata{Name: "NCPDP SCRIPT Builder", Category: "NCPDP Transform"}),
		loader:       loader,
	}
	mapExec := &NCPDPMapToCanonicalExecutor{
		BaseExecutor: executors.NewBaseExecutor("ncpdp.map_to_canonical", models.ExecutorMetadata{Name: "Map to Canonical NCPDP SCRIPT JSON", Category: "NCPDP Transform"}),
		loader:       loader,
	}
	return parseExec, validateExec, buildExec, mapExec
}

func readNCPDPFixture(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(data)
}

// TestNCPDPParseValidateBuild_RealNewRxSample_FullChain exercises
// ncpdp.parse -> ncpdp.validate -> ncpdp.build as a real pipeline chain
// against the real, unedited NewRx sample, confirming each step's output
// correctly feeds the next via the SAME field-name conventions the real
// pipeline engine uses (outputField -> a later step's sourceField default).
func TestNCPDPParseValidateBuild_RealNewRxSample_FullChain(t *testing.T) {
	parseExec, validateExec, buildExec, _ := newTestNCPDPExecutors(t)
	raw := readNCPDPFixture(t, "../../../ncpdp/testdata/real_samples/dgoradia_sample_newrx.xml")

	parseStep := &models.TransformationStep{
		StepName: "Parse NCPDP", StepType: "ncpdp.parse", Enabled: true,
		Config: map[string]interface{}{"sourceField": "raw", "outputField": "parsedNCPDP"},
	}
	afterParse, err := parseExec.Execute(context.Background(), parseStep, map[string]interface{}{"raw": raw})
	require.NoError(t, err)

	parsed, ok := afterParse["parsedNCPDP"].(map[string]interface{})
	require.True(t, ok, "expected parsedNCPDP to be a map")
	require.Equal(t, "NewRx", parsed["transactionType"])

	validateStep := &models.TransformationStep{
		StepName: "Validate NCPDP", StepType: "ncpdp.validate", Enabled: true,
		Config: map[string]interface{}{"sourceField": "parsedNCPDP", "outputField": "ncpdpValidation"},
	}
	afterValidate, err := validateExec.Execute(context.Background(), validateStep, afterParse)
	require.NoError(t, err)

	validation, ok := afterValidate["ncpdpValidation"].(map[string]interface{})
	require.True(t, ok, "expected ncpdpValidation to be a map")
	require.Equal(t, true, validation["valid"], "expected the real, complete NewRx sample to validate cleanly: %+v", validation["issues"])

	buildStep := &models.TransformationStep{
		StepName: "Build NCPDP", StepType: "ncpdp.build", Enabled: true,
		Config: map[string]interface{}{"sourceField": "parsedNCPDP", "transactionType": "NewRx", "outputField": "ncpdpScript"},
	}
	afterBuild, err := buildExec.Execute(context.Background(), buildStep, afterValidate)
	require.NoError(t, err)

	built, ok := afterBuild["ncpdpScript"].(string)
	require.True(t, ok, "expected ncpdpScript to be a string")
	require.Contains(t, built, "<NewRx>")
	require.Contains(t, built, "Ondansetron 8 mg Tab Disintegrating")
}

// TestNCPDPValidateExecutor_MessageUnwrapAndRawFallback verifies both the
// inputData["message"] unwrap fallback and the plain-raw-XML fallback (no
// prior ncpdp.parse step in the pipeline) work correctly.
func TestNCPDPValidateExecutor_MessageUnwrapAndRawFallback(t *testing.T) {
	_, validateExec, _, _ := newTestNCPDPExecutors(t)
	raw := readNCPDPFixture(t, "../../../ncpdp/testdata/self_authored/cancel_rx_sample.xml")

	step := &models.TransformationStep{
		StepName: "Validate NCPDP", StepType: "ncpdp.validate", Enabled: true,
		Config: map[string]interface{}{"sourceField": "raw", "outputField": "ncpdpValidation"},
	}
	// No prior ncpdp.parse step ran — sourceField "raw" holds XML text, not
	// a ParsedJSON map, exercising the string-fallback branch of
	// resolveNCPDPParseResult.
	output, err := validateExec.Execute(context.Background(), step, map[string]interface{}{
		"message": map[string]interface{}{"raw": raw},
	})
	require.NoError(t, err)

	validation, ok := output["ncpdpValidation"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, true, validation["valid"])
}

// TestNCPDPValidateExecutor_MissingRequiredField_ReturnsInvalidNotError
// verifies a structurally incomplete message surfaces as valid=false with
// issues, WITHOUT Execute() itself returning a Go error — the same
// "validation failure is data, not a pipeline fault" contract edi.validate
// establishes (a pipeline can branch on the result via
// control.if_then_else/control.switch_case).
func TestNCPDPValidateExecutor_MissingRequiredField_ReturnsInvalidNotError(t *testing.T) {
	_, validateExec, _, _ := newTestNCPDPExecutors(t)

	step := &models.TransformationStep{
		StepName: "Validate NCPDP", StepType: "ncpdp.validate", Enabled: true,
		Config: map[string]interface{}{"sourceField": "parsedNCPDP", "outputField": "ncpdpValidation"},
	}
	incomplete := map[string]interface{}{
		"parsedNCPDP": map[string]interface{}{
			"transactionType": "NewRx",
			"header":          map[string]interface{}{},
			"body":            map[string]interface{}{},
		},
	}
	output, err := validateExec.Execute(context.Background(), step, incomplete)
	require.NoError(t, err)

	validation := output["ncpdpValidation"].(map[string]interface{})
	require.Equal(t, false, validation["valid"])
	issues, ok := validation["issues"].([]map[string]interface{})
	require.True(t, ok)
	require.NotEmpty(t, issues)
}

// TestNCPDPMapToCanonical_ThenBuild_ProducesValidCancelRx proves the no-code
// mapping step end-to-end: arbitrary "source row" data (as if from a CSV/DB
// query) mapped into canonical NCPDP JSON, then built into a real CancelRx
// XML message, exercising nested (non-repeatable) group mapping (patient,
// prescriber, medicationPrescribed).
func TestNCPDPMapToCanonical_ThenBuild_ProducesValidCancelRx(t *testing.T) {
	_, _, buildExec, mapExec := newTestNCPDPExecutors(t)

	sourceRow := map[string]interface{}{
		"patient_last_name":  "Doe",
		"patient_first_name": "Jane",
		"prescriber_npi":     "1234567890",
		"prescriber_last":    "Smith",
		"prescriber_first":   "Alice",
		"drug_desc":          "Amoxicillin 500 mg Capsule",
		"qty_value":          "30",
		"written_date":       "2026-09-01",
		"sig_text":           "Take one capsule by mouth three times daily",
		"request_ref":        "REQ-9001",
	}

	mapStep := &models.TransformationStep{
		StepName: "Map to Canonical NCPDP", StepType: "ncpdp.map_to_canonical", Enabled: true,
		Config: map[string]interface{}{
			"outputField":     "parsedNCPDP",
			"transactionType": "CancelRx",
			"headerFields": []map[string]interface{}{
				{"fieldKey": "to", "literalValue": "1111111"},
				{"fieldKey": "from", "literalValue": "2222222"},
				{"fieldKey": "messageID", "literalValue": "test-msg-1"},
				{"fieldKey": "sentTime", "literalValue": "2026-09-14T12:00:00Z"},
			},
			"bodyFields": []map[string]interface{}{
				{"fieldKey": "requestReferenceNumber", "sourcePath": "request_ref"},
			},
			"bodyGroups": []map[string]interface{}{
				{
					"groupKey": "patient",
					"groups": []map[string]interface{}{
						{
							"groupKey": "humanPatient",
							"groups": []map[string]interface{}{
								{"groupKey": "name", "fields": []map[string]interface{}{
									{"fieldKey": "lastName", "sourcePath": "patient_last_name"},
									{"fieldKey": "firstName", "sourcePath": "patient_first_name"},
								}},
							},
						},
					},
				},
				{
					"groupKey": "prescriber",
					"groups": []map[string]interface{}{
						{
							"groupKey": "nonVeterinarian",
							"groups": []map[string]interface{}{
								{"groupKey": "identification", "fields": []map[string]interface{}{
									{"fieldKey": "npi", "sourcePath": "prescriber_npi"},
								}},
								{"groupKey": "name", "fields": []map[string]interface{}{
									{"fieldKey": "lastName", "sourcePath": "prescriber_last"},
									{"fieldKey": "firstName", "sourcePath": "prescriber_first"},
								}},
							},
						},
					},
				},
				{
					"groupKey": "medicationPrescribed",
					"fields": []map[string]interface{}{
						{"fieldKey": "drugDescription", "sourcePath": "drug_desc"},
					},
					"groups": []map[string]interface{}{
						{"groupKey": "quantity", "fields": []map[string]interface{}{
							{"fieldKey": "value", "sourcePath": "qty_value"},
						}},
						{"groupKey": "writtenDate", "fields": []map[string]interface{}{
							{"fieldKey": "date", "sourcePath": "written_date"},
						}},
						{"groupKey": "sig", "fields": []map[string]interface{}{
							{"fieldKey": "sigText", "sourcePath": "sig_text"},
						}},
					},
				},
			},
		},
	}

	afterMap, err := mapExec.Execute(context.Background(), mapStep, sourceRow)
	require.NoError(t, err)

	canonical, ok := afterMap["parsedNCPDP"].(map[string]interface{})
	require.True(t, ok)
	body := canonical["body"].(map[string]interface{})
	require.Equal(t, "REQ-9001", body["requestReferenceNumber"])
	patient := body["patient"].(map[string]interface{})
	humanPatient := patient["humanPatient"].(map[string]interface{})
	name := humanPatient["name"].(map[string]interface{})
	require.Equal(t, "Doe", name["lastName"])

	buildStep := &models.TransformationStep{
		StepName: "Build NCPDP", StepType: "ncpdp.build", Enabled: true,
		Config: map[string]interface{}{"sourceField": "parsedNCPDP", "transactionType": "CancelRx", "outputField": "ncpdpScript"},
	}
	afterBuild, err := buildExec.Execute(context.Background(), buildStep, afterMap)
	require.NoError(t, err)

	built := afterBuild["ncpdpScript"].(string)
	require.Contains(t, built, "<CancelRx>")
	require.Contains(t, built, "Amoxicillin 500 mg Capsule")
	require.Contains(t, built, "<LastName>Doe</LastName>")
	require.Contains(t, built, "<NPI>1234567890</NPI>")

	// Round-trip: the built XML must itself parse back correctly through
	// the real parser, proving the mapped-then-built document is genuinely
	// well-formed NCPDP SCRIPT, not just string-contains-the-right-bits.
	parser, err := ncpdpscriptparser.NewFromSchemaDir("../../../ncpdp/schemas/script_2017071")
	require.NoError(t, err)
	reparsed := parser.Parse(built)
	require.True(t, reparsed.Success)
	require.Equal(t, "CancelRx", reparsed.ParsedJSON["transactionType"])
}
