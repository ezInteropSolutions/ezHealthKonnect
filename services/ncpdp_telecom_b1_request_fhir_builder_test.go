package services

// ─────────────────────────────────────────────────────────────────────────────
// NCPDP Telecommunication D.0 B1 (Claim Billing Request) → FHIR mapping
//
// B1 request → Organization (pharmacy) + Patient + Claim (type=pharmacy),
// one Claim per GS-delimited transaction group (one per drug/line item
// being billed). Mirrors the EDI 835/837 precedent exactly: `fhir.build`'s
// `rowsPath` mode resolves fields ROW-ONLY (confirmed directly in
// fhir_build_executor.go, not assumed), so a small `enrichment.script`
// derive step is needed to copy the ONE header-level value every claim row
// needs (dateOfService) down onto each row before fhir.build ever sees it —
// the same real mechanism gap 835's own "Derive 835 Claim Context" step
// exists to solve, not a design invented fresh for D.0.
//
// Organization/Patient use FIXED literal ids ("organization-pharmacy-1"/
// "patient-1") rather than deriving them from real per-message data — a
// deliberate simplification since D.0's own B1 carries exactly ONE pharmacy
// and ONE patient per transmission (unlike 837's own subscriber/dependent
// multiplicity), so uniqueness WITHIN one message's own Bundle is all that
// matters; this also means Claim's own patient.reference/provider.reference
// need NO row-copied identifier data at all, avoiding the derive script
// growing any larger than the single dateOfService copy-down it actually needs.
//
// Config here is transcribed verbatim into a database migration — matching
// every other *_fhir_builder_test.go's own "config here is the source of
// truth" discipline.
//
// Run: go test ./services/ -v -run TestNCPDPTelecomB1RequestFHIRBuilder
// ─────────────────────────────────────────────────────────────────────────────

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"ezhealthkonnect/models"
	"ezhealthkonnect/ncpdptelecom"
	"ezhealthkonnect/services/executors/payload"
	"ezhealthkonnect/services/executors/validation"
)

const testNCPDPTelecomSchemaDirSvc = "../ncpdptelecom/schemas/telecom_d0"
const ncpdpTelecomParseB1RequestStepAlias = "parse_b1_request"

// buildSelfAuthoredB1Request is the SAME self-authored fixture-construction
// logic as ncpdptelecom/roundtrip_test.go's own buildTestB1Request —
// duplicated here (rather than exported cross-package) since it's small and
// each package's own test fixtures should stay independently readable,
// matching this codebase's own precedent elsewhere (e.g. EDI's per-package
// sample duplication). Structure cross-validated against apiv/dzero and
// eduardonunesp/ncpdp-telecom-fmt-book — see
// ncpdptelecom/schemas/telecom_d0's own sourceRefs for the exact provenance.
func buildSelfAuthoredB1Request() string {
	const rs, fs = "\x1E", "\x1C"
	header := "999999" + "D0" + "B1" +
		"          " + // processorControlNumber
		"1" + "01" +
		"1111111111     " + // serviceProviderId (15)
		"20260919" +
		"          " // software (10)
	body := rs + fs + "AM01" + fs + "CBSMITH" + fs + "CAJOHN" + fs + "C419800101" + fs + "C52" +
		rs + fs + "AM02" + fs + "EY01" + fs + "E91234567890" +
		rs + fs + "AM04" + fs + "C2123456789012" +
		rs + fs + "AM07" + fs + "D2000000123456" + fs + "E103" + fs + "D700003089421" + fs + "E70000030000" + fs + "D301" + fs + "D5030" + fs + "DE20220924" +
		rs + fs + "AM11" + fs + "D90000057A" + fs + "DC0000027E" + fs + "DX0000016B" + fs + "DQ00000000" + fs + "DU0000084F"
	return header + body
}

// deriveB1ClaimContextScript flattens each transaction group's own
// Claim/Pricing content into a flat row, copying down header.dateOfService
// (the one value every row needs but can't reach on its own — rowsPath
// field resolution is row-scoped only). Returns already-snake_case property
// names directly (matching the 837P/837I precedent) rather than relying on
// NormalizeStepOutput's own recursive snake-casing of nested content, which
// this project has already been bitten by once (the Universal EDI Receiver
// section's own "embedding raw parsed content" bug).
// Bare top-level statements ending in a trailing parenthesized object-
// literal EXPRESSION (matching pas_fhir_builder_test.go's own
// derivePASFieldsScript convention exactly) -- NOT a "function transform
// (input) {...}" wrapper. script_enrichment_executor.go runs the script
// text UNWRAPPED first; a bare function DECLARATION statement's own
// completion value is undefined (declarations don't produce a runtime
// value) -- an earlier draft of this script defined "function transform"
// but never actually CALLED it, so the executor's own completion-value
// capture saw "undefined", fell through to the (empty) pre-injected
// `output` object, and silently produced a 0-field result with no error at
// any layer. Found only by adding a JS-level debug script that dumped
// Object.keys() at each nesting level, not by static reasoning alone.
const deriveB1ClaimContextScript = `
// The prior ncpdptelecom.parse step's own step_output snapshot is ALWAYS
// reachable via steps.<alias>.step_output (never a plain top-level/
// message-nested field the way a REAL pipeline's execution context would
// additionally expose it) -- confirmed directly by how this test's own
// data fixture is constructed (svcInjectStepOutput only, matching the
// NewRx/CancelRx/RxChangeRequest precedent). That snapshot is ALWAYS
// passed through NormalizeStepOutput first, which recursively snake_cases
// every key at every depth -- so "transactionGroups[i].Claim" becomes
// "transaction_groups[i].claim", not just the top level.
var stepOut = (input.steps && input.steps.parse_b1_request && input.steps.parse_b1_request.step_output) || {};
var parsed = stepOut.parsed_telecom || {};
var header = parsed.header || {};
var groups = parsed.transaction_groups || [];
var claim_rows = [];
for (var i = 0; i < groups.length; i++) {
    var g = groups[i] || {};
    var claim = g.claim || {};
    var pricing = g.pricing || {};
    claim_rows.push({
        prescription_reference_number: claim.prescription_reference_number,
        product_service_id: claim.product_service_id,
        quantity_dispensed: claim.quantity_dispensed,
        gross_amount_due: pricing.gross_amount_due,
        date_of_service: header.date_of_service
    });
}
({ claim_rows: claim_rows });
`

func ncpdpTelecomB1RequestData(t *testing.T) map[string]interface{} {
	t.Helper()
	loader, err := ncpdptelecom.NewTelecomSchemaLoader(testNCPDPTelecomSchemaDirSvc)
	if err != nil {
		t.Fatalf("failed to load D.0 schema: %v", err)
	}
	raw := buildSelfAuthoredB1Request()
	parsed, err := ncpdptelecom.ParseTransmission(loader.Spec(), "request", raw)
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
	svcInjectStepOutput(data, ncpdpTelecomParseB1RequestStepAlias, normalized)
	return data
}

func ncpdpTelecomB1RequestSrc(path string) string {
	return "steps." + ncpdpTelecomParseB1RequestStepAlias + ".step_output.parsed_telecom." + path
}

func organizationB1RequestBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Organization",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirPharmacyOrganization",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "literalValue": "organization-pharmacy-1"},
			map[string]interface{}{"targetPath": "active", "literalValue": "true"},
			map[string]interface{}{"targetPath": "name", "literalValue": "Pharmacy"},
			map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/ncpdp-telecom-provider-id"},
			map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": ncpdpTelecomB1RequestSrc("transmission_group.pharmacy_provider.provider_id")},
		},
	}
}

func patientB1RequestBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Patient",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirPatient",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "literalValue": "patient-1"},
			map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/ncpdp-telecom-cardholder-id"},
			map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": ncpdpTelecomB1RequestSrc("transmission_group.insurance.cardholder_id")},
			map[string]interface{}{"targetPath": "name[0].family", "sourcePath": ncpdpTelecomB1RequestSrc("transmission_group.patient.patient_last_name")},
			map[string]interface{}{"targetPath": "birthDate", "sourcePath": ncpdpTelecomB1RequestSrc("transmission_group.patient.date_of_birth"), "transform": "x12_date_to_fhir_date"},
			map[string]interface{}{"targetPath": "gender", "literalValue": "male", "condition": map[string]interface{}{"field": ncpdpTelecomB1RequestSrc("transmission_group.patient.patient_gender_code"), "operator": "equals", "value": "1"}},
			map[string]interface{}{"targetPath": "gender", "literalValue": "female", "condition": map[string]interface{}{"field": ncpdpTelecomB1RequestSrc("transmission_group.patient.patient_gender_code"), "operator": "equals", "value": "2"}},
		},
	}
}

func claimB1RequestBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Claim",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirClaims",
		"rowsPath":     "steps." + "derive_b1_claim_context" + ".step_output.claim_rows",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": "_rowIndex", "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "claim-"}},
			map[string]interface{}{"targetPath": "status", "literalValue": "active"},
			map[string]interface{}{"targetPath": "type.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/claim-type"},
			map[string]interface{}{"targetPath": "type.coding[0].code", "literalValue": "pharmacy"},
			map[string]interface{}{"targetPath": "use", "literalValue": "claim"},
			map[string]interface{}{"targetPath": "patient.reference", "literalValue": "Patient/patient-1"},
			map[string]interface{}{"targetPath": "provider.reference", "literalValue": "Organization/organization-pharmacy-1"},
			map[string]interface{}{"targetPath": "priority.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/processpriority"},
			map[string]interface{}{"targetPath": "priority.coding[0].code", "literalValue": "normal"},
			map[string]interface{}{"targetPath": "created", "sourcePath": "date_of_service", "transform": "x12_date_to_fhir_date"},
			map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/ncpdp-telecom-prescription-reference-number"},
			map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": "prescription_reference_number"},
			map[string]interface{}{"targetPath": "insurance[0].sequence", "literalValue": "1"},
			map[string]interface{}{"targetPath": "insurance[0].focal", "literalValue": "true"},
			map[string]interface{}{"targetPath": "insurance[0].coverage.display", "literalValue": "Insurance Coverage"},
			map[string]interface{}{"targetPath": "item[0].sequence", "literalValue": "1"},
			map[string]interface{}{"targetPath": "item[0].productOrService.coding[0].system", "literalValue": "http://hl7.org/fhir/sid/ndc"},
			map[string]interface{}{"targetPath": "item[0].productOrService.coding[0].code", "sourcePath": "product_service_id"},
			map[string]interface{}{"targetPath": "item[0].servicedDate", "sourcePath": "date_of_service", "transform": "x12_date_to_fhir_date"},
			map[string]interface{}{"targetPath": "item[0].quantity.value", "sourcePath": "quantity_dispensed"},
			map[string]interface{}{"targetPath": "item[0].net.value", "sourcePath": "gross_amount_due"},
			map[string]interface{}{"targetPath": "item[0].net.currency", "literalValue": "USD"},
		},
	}
}

func TestNCPDPTelecomB1RequestFHIRBuilder_SelfAuthoredSample_BuildsCleanValidatingBundle(t *testing.T) {
	initFHIRRegistrySvc(t)
	data := ncpdpTelecomB1RequestData(t)

	deriveResult := runScriptSvc(t, "derive_b1_claim_context", deriveB1ClaimContextScript, data)
	deriveOut := svcStepOutput(t, deriveResult, "derive_b1_claim_context")
	for k, v := range deriveResult {
		data[k] = v
	}
	svcInjectStepOutput(data, "derive_b1_claim_context", deriveOut)

	data = runFHIRBuild(t, "build_pharmacy_organization_fhir", organizationB1RequestBuildConfig(), data)
	data = runFHIRBuild(t, "build_patient_fhir", patientB1RequestBuildConfig(), data)
	data = runFHIRBuild(t, "build_claim_fhir", claimB1RequestBuildConfig(), data)

	message := data["message"].(map[string]interface{})
	claims, ok := message["fhirClaims"].([]map[string]interface{})
	if !ok || len(claims) != 1 {
		t.Fatalf("expected exactly 1 Claim resource, got %v", message["fhirClaims"])
	}
	claim := claims[0]
	if claim["status"] != "active" {
		t.Errorf("Claim.status = %v, want \"active\"", claim["status"])
	}
	idArr := claim["identifier"].([]interface{})
	if idArr[0].(map[string]interface{})["value"] != "000000123456" {
		t.Errorf("Claim.identifier[0].value = %v, want 000000123456", idArr[0].(map[string]interface{})["value"])
	}

	assembleAndValidateNCPDPTelecomBundle(t, data, "assemble_b1_request_bundle",
		[]string{"message.fhirPharmacyOrganization", "message.fhirPatient", "message.fhirClaims"})
}

// assembleAndValidateNCPDPTelecomBundle mirrors assembleAndValidateNCPDPBundle
// (ncpdp_cancelrx_fhir_builder_test.go) exactly, duplicated under its own
// name since D.0's own test files are a genuinely separate format from
// NCPDP SCRIPT despite the shared brand name — matching this project's own
// "separate templates for genuinely different shapes" precedent.
func assembleAndValidateNCPDPTelecomBundle(t *testing.T, data map[string]interface{}, alias string, resourcePaths []string) string {
	t.Helper()

	rp := make([]interface{}, len(resourcePaths))
	for i, p := range resourcePaths {
		rp[i] = p
	}

	pbExec := payload.NewPayloadBuilderExecutor(nil)
	pbStep := &models.TransformationStep{
		StepName:  alias,
		StepAlias: strPtrSvc(alias),
		StepType:  "payload.builder",
		Enabled:   true,
		Config: map[string]interface{}{
			"mode":       "fhir_bundle",
			"fhirBundle": map[string]interface{}{"bundleType": "collection", "resourcePaths": rp},
		},
	}
	pbResult, err := pbExec.Execute(context.Background(), pbStep, data)
	if err != nil {
		t.Fatalf("payload.builder error: %v", err)
	}
	pbOut := svcStepOutput(t, pbResult, alias)
	for k, v := range pbResult {
		data[k] = v
	}
	svcInjectStepOutput(data, alias, pbOut)

	bundleJSON, ok := pbOut["payload"].(string)
	if !ok {
		t.Fatalf("payload.builder did not produce a payload string: %+v", pbOut)
	}
	var bundle map[string]interface{}
	if err := json.Unmarshal([]byte(bundleJSON), &bundle); err != nil {
		t.Fatalf("failed to unmarshal bundle JSON: %v", err)
	}

	valAlias := "validate_" + alias
	vExec := validation.NewFHIRValidationExecutor()
	vStep := &models.TransformationStep{
		StepName:  valAlias,
		StepAlias: strPtrSvc(valAlias),
		StepType:  "fhir_validation",
		Enabled:   true,
		Config: map[string]interface{}{
			"profile":          "base",
			"validation_level": "strict",
			"fhir_version":     "R4",
			"source_field":     "steps." + alias + ".step_output.payload",
		},
	}
	vResult, err := vExec.Execute(context.Background(), vStep, data)
	if err != nil {
		t.Fatalf("%s error: %v", valAlias, err)
	}
	vOut := svcStepOutput(t, vResult, valAlias)

	errs := asStringSlice(vOut["errors"])
	if len(errs) > 0 {
		b, _ := json.MarshalIndent(bundle, "", "  ")
		t.Errorf("unexpected validation errors:\n%s\nbundle: %s", strings.Join(errs, "\n"), b)
	}
	return bundleJSON
}
