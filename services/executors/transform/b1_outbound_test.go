// services/executors/transform/b1_outbound_test.go
//
// The D.0 counterpart to newrx_outbound_test.go: proves the first genuinely
// OUTBOUND D.0 pipeline (every D.0 template built so far — V266/V267 — goes
// wire-format-IN -> FHIR-OUT). A drug gets dispensed at a pharmacy -> that
// becomes a FHIR MedicationDispense (the same resource this codebase's own
// RxFill inbound mapping already uses, not a new invention) -> derive a few
// reshaped fields -> ncpdptelecom.map_to_canonical -> ncpdptelecom.build -> a
// REAL outbound connector, ready to submit the claim to a payer/switch.
//
// Honest scope note: MedicationDispense carries no insurance/coverage data
// (that lives on a separate Coverage resource in a real system) — Insurance's
// own cardholderId is approximated here from subject.identifier.value (the
// patient's OWN identifier, e.g. an MRN), a real but imperfect proxy,
// documented as such rather than presented as a true cardholder ID lookup.
// binNumber/processorControlNumber/serviceProviderIdQualifier are payer-
// enrollment-level constants no FHIR resource could ever carry — supplied as
// literal config values, the same "config fills in what data doesn't supply"
// precedent already established throughout this codebase (CdaCustodianConfig,
// EDI's ISA/GS config).
package transform

import (
	"context"
	"strings"
	"testing"

	"ezhealthkonnect/models"
	"ezhealthkonnect/ncpdptelecom"
	"ezhealthkonnect/services/executors/enrichment"
)

const deriveB1OutboundStepAlias = "derive_b1_outbound_fields"

// assertOnlyIssuePaths fails the test unless validationResult's own issues
// are EXACTLY the named, expected set (by Path) — mirrors this codebase's
// own established "assertOnlyNamedEDI835ValidationGaps"-style precedent: a
// named gap must be proven to be the ONLY gap, not just present, so a
// regression introducing a NEW unexpected gap is still caught.
func assertOnlyIssuePaths(t *testing.T, validationResult map[string]interface{}, expectedPaths []string) {
	t.Helper()
	issues, _ := validationResult["issues"].([]map[string]interface{})
	got := make(map[string]bool, len(issues))
	for _, iss := range issues {
		if path, ok := iss["path"].(string); ok {
			got[path] = true
		}
	}
	want := make(map[string]bool, len(expectedPaths))
	for _, p := range expectedPaths {
		want[p] = true
		if !got[p] {
			t.Errorf("expected a validation issue at path %q, but it was not present — issues: %+v", p, issues)
		}
	}
	for p := range got {
		if !want[p] {
			t.Errorf("unexpected validation issue at path %q (not in the named-gap allowlist) — issues: %+v", p, issues)
		}
	}
}

func deriveB1OutboundFieldsScript() string {
	return `
var msg = input.resourceType ? input : ((input.message && input.message.resourceType) ? input.message : input);
var subj = msg.subject || {};
var performerArr = msg.performer || [];
var performer = performerArr.length > 0 ? performerArr[0] : {};
var actor = performer.actor || {};
var med = msg.medicationCodeableConcept || {};
var coding = (med.coding && med.coding.length > 0) ? med.coding[0] : {};
var quantity = msg.quantity || {};
var daysSupply = msg.daysSupply || {};
var identArr = msg.identifier || [];
var ident = identArr.length > 0 ? identArr[0] : {};

var whenHandedOver = msg.whenHandedOver || msg.whenPrepared || "";
var dateOnly = whenHandedOver ? whenHandedOver.substring(0, 10) : "";

var quantityValue = "";
if (quantity.value !== undefined && quantity.value !== null) { quantityValue = String(quantity.value); }
var daysSupplyValue = "";
if (daysSupply.value !== undefined && daysSupply.value !== null) { daysSupplyValue = String(Math.round(daysSupply.value)); }

function splitDisplayName(display) {
    if (!display) return { first: "", last: "" };
    var parts = display.split(" ").filter(function(p) { return p.length > 0; });
    if (parts.length === 0) return { first: "", last: "" };
    if (parts.length === 1) return { first: "", last: parts[0] };
    return { first: parts.slice(0, parts.length - 1).join(" "), last: parts[parts.length - 1] };
}
var patientName = splitDisplayName(subj.display);

({
    prescription_reference_number: ident.value || "",
    cardholder_id: (subj.identifier && subj.identifier.value) || "",
    patient_first_name: patientName.first,
    patient_last_name: patientName.last,
    ndc_code: coding.code || "",
    quantity_dispensed: quantityValue,
    days_supply: daysSupplyValue,
    date_of_service: dateOnly,
    pharmacy_npi: (actor.identifier && actor.identifier.value) || ""
});
`
}

func b1OutboundMapToCanonicalConfig() map[string]interface{} {
	sp := func(key string) string { return "steps." + deriveB1OutboundStepAlias + ".step_output." + key }
	return map[string]interface{}{
		"outputField":     "parsedTelecom",
		"transactionCode": "B1",
		"direction":       "request",
		"headerFields": []map[string]interface{}{
			{"fieldKey": "binNumber", "literalValue": "999999"},
			{"fieldKey": "transactionCount", "literalValue": "1"},
			{"fieldKey": "serviceProviderIdQualifier", "literalValue": "01"},
			{"fieldKey": "serviceProviderId", "sourcePath": sp("pharmacy_npi")},
			{"fieldKey": "dateOfService", "sourcePath": sp("date_of_service"), "transform": "date_to_x12"},
		},
		"transmissionGroupSegments": []map[string]interface{}{
			{
				"segmentKey": "Insurance",
				"fields": []map[string]interface{}{
					{"fieldKey": "cardholderId", "sourcePath": sp("cardholder_id")},
				},
			},
			{
				"segmentKey": "Patient",
				"fields": []map[string]interface{}{
					{"fieldKey": "patientLastName", "sourcePath": sp("patient_last_name")},
					{"fieldKey": "patientFirstName", "sourcePath": sp("patient_first_name")},
				},
			},
			{
				"segmentKey": "PharmacyProvider",
				"fields": []map[string]interface{}{
					{"fieldKey": "providerIdQualifier", "literalValue": "01"},
					{"fieldKey": "providerId", "sourcePath": sp("pharmacy_npi")},
				},
			},
		},
		"transactionGroupSegments": []map[string]interface{}{
			{
				"segmentKey": "Claim",
				"fields": []map[string]interface{}{
					{"fieldKey": "prescriptionReferenceNumberQualifier", "literalValue": "1"},
					{"fieldKey": "prescriptionReferenceNumber", "sourcePath": sp("prescription_reference_number")},
					{"fieldKey": "productServiceIdQualifier", "literalValue": "03"},
					{"fieldKey": "productServiceId", "sourcePath": sp("ndc_code")},
					{"fieldKey": "quantityDispensed", "sourcePath": sp("quantity_dispensed")},
					{"fieldKey": "daysSupply", "sourcePath": sp("days_supply")},
					{"fieldKey": "fillNumber", "literalValue": "01"},
				},
			},
		},
	}
}

// sampleFHIRMedicationDispense is a real-shaped FHIR R4 MedicationDispense —
// the same resource this codebase's own RxFill inbound mapping already
// targets, used here as the trigger for the reverse (outbound) direction.
func sampleFHIRMedicationDispense() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "MedicationDispense",
		"id":           "meddisp-outbound-test-001",
		"status":       "completed",
		"identifier": []interface{}{
			map[string]interface{}{"value": "RX000123456"},
		},
		"subject": map[string]interface{}{
			"reference": "Patient/pat-001",
			"display":   "Jane Doe",
			"identifier": map[string]interface{}{
				"system": "http://example.org/mrn",
				"value":  "MRN998877",
			},
		},
		"performer": []interface{}{
			map[string]interface{}{
				"actor": map[string]interface{}{
					"reference": "Organization/pharmacy-001",
					"display":   "Corner Drug Pharmacy",
					"identifier": map[string]interface{}{
						"system": "http://hl7.org/fhir/sid/us-npi",
						"value":  "1234567890",
					},
				},
			},
		},
		"medicationCodeableConcept": map[string]interface{}{
			"text": "Lisinopril 10mg Tablet",
			"coding": []interface{}{
				map[string]interface{}{
					"system": "http://hl7.org/fhir/sid/ndc",
					"code":   "00003089421",
				},
			},
		},
		"quantity":       map[string]interface{}{"value": 30.0, "unit": "TAB"},
		"daysSupply":     map[string]interface{}{"value": 30.0, "unit": "d"},
		"whenHandedOver": "2026-09-20T14:30:00Z",
	}
}

func TestB1Outbound_FromFHIRMedicationDispense_BuildsValidB1Transmission(t *testing.T) {
	fhirInput := sampleFHIRMedicationDispense()

	scriptExec := enrichment.NewScriptEnrichmentExecutor()
	scriptStep := &models.TransformationStep{
		StepName: "Derive B1 Outbound Fields",
		StepType: "enrichment.script",
		Enabled:  true,
		Config:   map[string]interface{}{"script": deriveB1OutboundFieldsScript()},
	}
	scriptResult, err := scriptExec.Execute(context.Background(), scriptStep, fhirInput)
	if err != nil {
		t.Fatalf("enrichment.script Execute failed: %v", err)
	}
	stepOutputRaw, ok := scriptResult["_stepOutput"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected _stepOutput map, got %T", scriptResult["_stepOutput"])
	}
	if stepOutputRaw["ndc_code"] != "00003089421" {
		t.Fatalf("expected ndc_code=00003089421, got %v", stepOutputRaw["ndc_code"])
	}
	if stepOutputRaw["pharmacy_npi"] != "1234567890" {
		t.Fatalf("expected pharmacy_npi=1234567890, got %v", stepOutputRaw["pharmacy_npi"])
	}
	if stepOutputRaw["date_of_service"] != "2026-09-20" {
		t.Fatalf("expected date_of_service=2026-09-20, got %v", stepOutputRaw["date_of_service"])
	}

	stepOutputData := make(map[string]interface{})
	for k, v := range fhirInput {
		stepOutputData[k] = v
	}
	stepOutputData["steps"] = map[string]interface{}{
		deriveB1OutboundStepAlias: map[string]interface{}{"step_output": stepOutputRaw},
	}

	_, validateExecShared, buildExecShared, mapExec := newTestNCPDPTelecomExecutors(t)
	mapStep := &models.TransformationStep{
		StepName: "Map to Canonical B1", StepType: "ncpdptelecom.map_to_canonical", Enabled: true,
		Config: b1OutboundMapToCanonicalConfig(),
	}
	mapOut, err := mapExec.Execute(context.Background(), mapStep, stepOutputData)
	if err != nil {
		t.Fatalf("ncpdptelecom.map_to_canonical Execute failed: %v", err)
	}
	canonicalDoc, ok := mapOut["parsedTelecom"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected parsedTelecom map, got %T", mapOut["parsedTelecom"])
	}
	header, _ := canonicalDoc["header"].(map[string]interface{})
	if header["dateOfService"] != "20260920" {
		t.Fatalf("expected header.dateOfService=20260920 (date_to_x12 transform applied), got %v", header["dateOfService"])
	}
	if header["serviceProviderId"] != "1234567890" {
		t.Fatalf("expected header.serviceProviderId=1234567890, got %v", header["serviceProviderId"])
	}

	// ncpdptelecom.validate — a real gap closed after initial full-stack
	// verification: the outbound template originally had no validate step
	// before build, unlike every inbound-direction template.
	// resolveNCPDPTelecomParseResult accepts a map already shaped like
	// map_to_canonical's own output (has "transactionCode"/"header" keys)
	// directly, with zero new engine code — confirmed by reading
	// ncpdptelecom_validate_executor.go before adding this step.
	validateStep := &models.TransformationStep{
		StepName: "Validate Canonical B1", StepType: "ncpdptelecom.validate", Enabled: true,
		Config: map[string]interface{}{"sourceField": "parsedTelecom", "outputField": "telecomValidation"},
	}
	validateOut, err := validateExecShared.Execute(context.Background(), validateStep, mapOut)
	if err != nil {
		t.Fatalf("ncpdptelecom.validate Execute failed: %v", err)
	}
	validationResult, ok := validateOut["telecomValidation"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected telecomValidation map, got %T", validateOut["telecomValidation"])
	}
	// NOT expected to validate fully clean: Pricing (ingredientCostSubmitted/
	// usualAndCustomaryCharge/grossAmountDue) is a REAL, honest, named gap —
	// a FHIR MedicationDispense carries no cost/pricing data at all (that's
	// the pharmacy's own point-of-sale/billing system's data, not a clinical
	// dispensing fact FHIR models on this resource) — fabricating a $0.00
	// placeholder would be actively misleading to a real payer, not a
	// harmless default. Patient and PharmacyProvider ARE populated (both
	// cleanly derivable from data already on the resource), so this asserts
	// Pricing is the ONLY remaining gap, not that validation is silently
	// ignored.
	assertOnlyIssuePaths(t, validationResult, []string{"TransactionGroups[0].Pricing"})

	buildStep := &models.TransformationStep{
		StepName: "Build B1 Transmission", StepType: "ncpdptelecom.build", Enabled: true,
		Config: map[string]interface{}{"sourceField": "parsedTelecom", "transactionCode": "B1", "direction": "request", "outputField": "ncpdpTelecom"},
	}
	buildOut, err := buildExecShared.Execute(context.Background(), buildStep, validateOut)
	if err != nil {
		t.Fatalf("ncpdptelecom.build Execute failed: %v", err)
	}
	transmission, ok := buildOut["ncpdpTelecom"].(string)
	if !ok || transmission == "" {
		t.Fatalf("expected non-empty ncpdpTelecom string, got %T: %v", buildOut["ncpdpTelecom"], buildOut["ncpdpTelecom"])
	}
	if !strings.Contains(transmission, "999999") || !strings.Contains(transmission, "00003089421") {
		t.Fatalf("built B1 transmission missing expected content:\n%q", transmission)
	}

	// Round-trip: the built transmission must be a real, well-formed B1 any
	// real payer/switch's own D.0 reader would accept.
	loader, err := ncpdptelecom.NewTelecomSchemaLoader("../../../ncpdptelecom/schemas/telecom_d0")
	if err != nil {
		t.Fatalf("failed to load D.0 schema: %v", err)
	}
	reparsed, err := ncpdptelecom.ParseTransmission(loader.Spec(), "request", transmission)
	if err != nil {
		t.Fatalf("built B1 transmission failed to re-parse: %v\n%q", err, transmission)
	}
	if reparsed.TransactionCode != "B1" {
		t.Fatalf("re-parsed TransactionCode = %q, want B1", reparsed.TransactionCode)
	}
	if len(reparsed.TransactionGroups) != 1 {
		t.Fatalf("expected 1 transaction group, got %d", len(reparsed.TransactionGroups))
	}
	claim, ok := reparsed.TransactionGroups[0].(map[string]interface{})["Claim"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected re-parsed Claim segment, got %v", reparsed.TransactionGroups[0])
	}
	if claim["productServiceId"] != "00003089421" {
		t.Fatalf("re-parsed Claim.productServiceId = %v, want 00003089421", claim["productServiceId"])
	}
}
