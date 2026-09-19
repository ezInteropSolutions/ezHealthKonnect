package services

// ─────────────────────────────────────────────────────────────────────────────
// NCPDP Telecommunication D.0 B1 (Claim Billing Response) → FHIR mapping
//
// B1 response → ClaimResponse, one per GS-delimited transaction group. A
// response carries NO patient/pharmacy identity data at all (confirmed
// directly against the schema — ResponseMessage/ResponseStatus/
// ResponseClaim/ResponsePricing carry none), the same real gap that led
// NCPDP SCRIPT's CancelRxResponse/RxChangeResponse to use Task instead of
// MedicationRequest. Since the approved plan for THIS format named
// ClaimResponse as the target (a real, adjudication-bearing resource Task
// can't represent as honestly), ClaimResponse.patient is populated with a
// DISPLAY-ONLY Reference (no `.reference` pointer, since no Patient
// resource exists in this response-only Bundle to point at) — the same
// logical-reference pattern already used for `Claim.careTeam[].provider` in
// the EDI 837 work, satisfying FHIR's structural Reference-object
// requirement without fabricating a resource or writing a dangling pointer.
//
// HCR/AN status code A/C/P (Approved/Captured/Paid) -> outcome "complete";
// D/E/R (Duplicate/Error/Rejected) -> outcome "error" — a code-system
// translation sourced directly from response.md's own confirmed vocabulary,
// never an invented fact.
//
// Config here is transcribed verbatim into a database migration — matching
// every other *_fhir_builder_test.go's own "config here is the source of
// truth" discipline.
//
// Run: go test ./services/ -v -run TestNCPDPTelecomB1ResponseFHIRBuilder
// ─────────────────────────────────────────────────────────────────────────────

import (
	"testing"

	"ezhealthkonnect/models"
	"ezhealthkonnect/ncpdptelecom"
)

const ncpdpTelecomParseB1ResponseStepAlias = "parse_b1_response"

// deriveB1ResponseContextScript flattens each transaction group's own
// ResponseStatus/ResponsePricing content into a flat row — already
// snake_case, matching deriveB1ClaimContextScript's own established
// discipline.
// Bare top-level statements ending in a trailing parenthesized object-
// literal EXPRESSION -- see deriveB1ClaimContextScript's own doc comment
// for the full "function transform(input){...} is never actually called by
// the unwrapped-completion-value execution path" gotcha this fixes.
const deriveB1ResponseContextScript = `
// See deriveB1ClaimContextScript's own doc comment for why this reads
// steps.<alias>.step_output (already recursively snake_cased by
// NormalizeStepOutput) rather than a plain "message.parsedTelecom" field.
var stepOut = (input.steps && input.steps.parse_b1_response && input.steps.parse_b1_response.step_output) || {};
var parsed = stepOut.parsed_telecom || {};
var groups = parsed.transaction_groups || [];
var response_rows = [];
for (var i = 0; i < groups.length; i++) {
    var g = groups[i] || {};
    var status = g.response_status || {};
    var pricing = g.response_pricing || {};
    response_rows.push({
        response_status: status.response_status,
        authorization_number: status.authorization_number,
        reject_code: status.reject_code,
        transaction_reference_number: status.transaction_reference_number,
        total_amount_paid: pricing.total_amount_paid
    });
}
({ response_rows: response_rows });
`

func buildSelfAuthoredB1Response() string {
	const rs, fs = "\x1E", "\x1C"
	header := "999999" + "D0" + "B1" +
		"          " +
		"1" + "01" +
		"1111111111     " +
		"20260919" +
		"          "
	body := rs + fs + "AM20" + fs + "F4Claim processed successfully" +
		rs + fs + "AM21" + fs + "ANP" + fs + "F31234567890" + fs + "K5REQ-0005" +
		rs + fs + "AM23" + fs + "F50000084F" + fs + "F60000057A" + fs + "F70000027E" + fs + "F90000084F" + fs + "FI0000016B"
	return header + body
}

func ncpdpTelecomB1ResponseData(t *testing.T) map[string]interface{} {
	t.Helper()
	loader, err := ncpdptelecom.NewTelecomSchemaLoader(testNCPDPTelecomSchemaDirSvc)
	if err != nil {
		t.Fatalf("failed to load D.0 schema: %v", err)
	}
	raw := buildSelfAuthoredB1Response()
	parsed, err := ncpdptelecom.ParseTransmission(loader.Spec(), "response", raw)
	if err != nil {
		t.Fatalf("ncpdptelecom.ParseTransmission failed: %v", err)
	}

	rawStepOutput := map[string]interface{}{
		"parsedTelecom": map[string]interface{}{
			"transactionCode":   parsed.TransactionCode,
			"direction":         parsed.Direction,
			"header":            parsed.Header,
			"transmissionGroup": parsed.TransmissionGroup,
			"transactionGroups": parsed.TransactionGroups,
		},
	}
	normalized := models.NewOutputNormalizer().NormalizeStepOutput(rawStepOutput)

	data := map[string]interface{}{}
	svcInjectStepOutput(data, ncpdpTelecomParseB1ResponseStepAlias, normalized)
	return data
}

func claimResponseB1BuildConfig() map[string]interface{} {
	outcomeApproved := []string{"A", "C", "P"}
	outcomeError := []string{"D", "E", "R"}

	fields := []interface{}{
		map[string]interface{}{"targetPath": "id", "sourcePath": "_rowIndex", "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "claimresponse-"}},
		map[string]interface{}{"targetPath": "status", "literalValue": "active"},
		map[string]interface{}{"targetPath": "type.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/claim-type"},
		map[string]interface{}{"targetPath": "type.coding[0].code", "literalValue": "pharmacy"},
		map[string]interface{}{"targetPath": "use", "literalValue": "claim"},
		map[string]interface{}{"targetPath": "patient.display", "literalValue": "Patient information not included in claim status response"},
		map[string]interface{}{"targetPath": "created", "literalValue": "2026-09-19"},
		map[string]interface{}{"targetPath": "insurer.display", "literalValue": "Payer"},
		map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/ncpdp-telecom-transaction-reference-number"},
		map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": "transaction_reference_number"},
		map[string]interface{}{"targetPath": "disposition", "sourcePath": "authorization_number", "fallbackPaths": []interface{}{"reject_code"}},
	}
	for _, code := range outcomeApproved {
		fields = append(fields, map[string]interface{}{"targetPath": "outcome", "literalValue": "complete", "condition": map[string]interface{}{"field": "response_status", "operator": "equals", "value": code}})
	}
	for _, code := range outcomeError {
		fields = append(fields, map[string]interface{}{"targetPath": "outcome", "literalValue": "error", "condition": map[string]interface{}{"field": "response_status", "operator": "equals", "value": code}})
	}

	return map[string]interface{}{
		"resourceType": "ClaimResponse",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirClaimResponses",
		"rowsPath":     "steps.derive_b1_response_context.step_output.response_rows",
		"fields":       fields,
	}
}

func TestNCPDPTelecomB1ResponseFHIRBuilder_ApprovedOutcome_BuildsCleanValidatingBundle(t *testing.T) {
	initFHIRRegistrySvc(t)
	data := ncpdpTelecomB1ResponseData(t)

	deriveResult := runScriptSvc(t, "derive_b1_response_context", deriveB1ResponseContextScript, data)
	deriveOut := svcStepOutput(t, deriveResult, "derive_b1_response_context")
	for k, v := range deriveResult {
		data[k] = v
	}
	svcInjectStepOutput(data, "derive_b1_response_context", deriveOut)

	data = runFHIRBuild(t, "build_claim_response_fhir", claimResponseB1BuildConfig(), data)

	message := data["message"].(map[string]interface{})
	responses, ok := message["fhirClaimResponses"].([]map[string]interface{})
	if !ok || len(responses) != 1 {
		t.Fatalf("expected exactly 1 ClaimResponse resource, got %v", message["fhirClaimResponses"])
	}
	cr := responses[0]
	if cr["outcome"] != "complete" {
		t.Errorf("ClaimResponse.outcome = %v, want \"complete\" (Paid status)", cr["outcome"])
	}
	idArr := cr["identifier"].([]interface{})
	if idArr[0].(map[string]interface{})["value"] != "REQ-0005" {
		t.Errorf("ClaimResponse.identifier[0].value = %v, want REQ-0005", idArr[0].(map[string]interface{})["value"])
	}
	if cr["disposition"] != "1234567890" {
		t.Errorf("ClaimResponse.disposition = %v, want the real authorization number 1234567890", cr["disposition"])
	}

	assembleAndValidateNCPDPTelecomBundle(t, data, "assemble_b1_response_bundle", []string{"message.fhirClaimResponses"})
}
