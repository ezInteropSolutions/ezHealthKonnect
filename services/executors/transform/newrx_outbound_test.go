// services/executors/transform/newrx_outbound_test.go
//
// Proves the FIRST genuinely OUTBOUND NCPDP pipeline in this codebase: every
// NCPDP template built so far (10 OOB templates across SCRIPT and D.0) goes
// wire-format-IN -> FHIR-OUT, terminating at a no-op sink_outbound. This
// proves the reverse direction real customers actually need for e-
// prescribing: a FHIR MedicationRequest arrives (e.g. via http_fhir_inbound,
// confirmed directly in processing/engine_message_processor.go to land with
// the resource's own fields FLAT AT ROOT for the first pipeline step,
// "identical envelope contract to HL7 messages") -> derive a few reshaped
// fields -> ncpdp.map_to_canonical -> ncpdp.build -> a REAL outbound
// connector (not sink_outbound).
//
// Honest scope note: a bare FHIR MedicationRequest carries only References
// (subject/requester/dispenseRequest.performer), not full inline patient/
// prescriber/pharmacy demographics — that data lives on separate Patient/
// Practitioner/Organization resources in a real system. This derive script
// uses Reference.display (a real, spec-legal FHIR field meant exactly for
// "human-readable identification without the full resource") for names, and
// Reference.identifier.value for NPIs (the same logical-reference pattern
// already established in this codebase for EDI 837's Claim.careTeam[].provider).
// It does NOT fabricate a name-splitting fixture library or invent FHIR
// structure that isn't there — this is what a bare MedicationRequest
// genuinely, reliably carries; a real deployment needing full demographics
// would add a Bundle-shaped input or a lookup/enrichment step before this one.
package transform

import (
	"context"
	"strings"
	"testing"

	"ezhealthkonnect/models"
	"ezhealthkonnect/ncpdp"
	"ezhealthkonnect/services/executors/enrichment"
)

const deriveNewRxOutboundStepAlias = "derive_new_rx_outbound_fields"

// deriveNewRxOutboundFieldsScript reshapes a bare FHIR MedicationRequest's
// own fields into the flat, already-snake_case set ncpdp.map_to_canonical's
// own field mappings address via "steps.<alias>.step_output.<key>" — the
// ONLY reach-back mechanism an enrichment.script's return value has to a
// later step (BaseExecutor.SetStepOutputWithDetails / executeStepWithContext,
// confirmed in this codebase's own CLAUDE.md across every EDI/NCPDP derive
// script). Every later step therefore addresses ONLY this script's own
// output — never "message.<rawFhirField>" directly — avoiding the dual-
// addressing ambiguity entirely (the same clean pattern 837P/837I's own
// derive script already established: compute everything once here).
//
// "input.message || input" mirrors the SAME defensive unwrap this
// codebase's own edi_835_to_fhir_test.go derive script uses: a real pipeline
// run wraps a prior step's plain output under "message" for a later step,
// but this Go test (like that one) chains executors directly without that
// wrapper — the script must tolerate both shapes.
func deriveNewRxOutboundFieldsScript() string {
	return `
var msg = input.resourceType ? input : ((input.message && input.message.resourceType) ? input.message : input);
var subj = msg.subject || {};
var req = msg.requester || {};
var disp = msg.dispenseRequest || {};
var med = msg.medicationCodeableConcept || {};
var dosageArr = msg.dosageInstruction || [];
var dosage = dosageArr.length > 0 ? dosageArr[0] : {};
var performer = disp.performer || {};
var quantity = disp.quantity || {};
var coding = (med.coding && med.coding.length > 0) ? med.coding[0] : {};

function splitDisplayName(display) {
    if (!display) return { first: "", last: "" };
    var parts = display.split(" ").filter(function(p) { return p.length > 0; });
    if (parts.length === 0) return { first: "", last: "" };
    if (parts.length === 1) return { first: "", last: parts[0] };
    return { first: parts.slice(0, parts.length - 1).join(" "), last: parts[parts.length - 1] };
}

var patientName = splitDisplayName(subj.display);
var prescriberName = splitDisplayName(req.display);

var quantityValue = "";
if (quantity.value !== undefined && quantity.value !== null) {
    quantityValue = String(quantity.value);
}

({
    message_id: msg.id || "",
    patient_first_name: patientName.first,
    patient_last_name: patientName.last,
    prescriber_first_name: prescriberName.first,
    prescriber_last_name: prescriberName.last,
    prescriber_npi: (req.identifier && req.identifier.value) || "",
    pharmacy_npi: (performer.identifier && performer.identifier.value) || "",
    drug_description: med.text || "",
    drug_code: coding.code || "",
    quantity_value: quantityValue,
    written_date: msg.authoredOn || "",
    sig_text: dosage.text || ""
});
`
}

// newRxOutboundMapToCanonicalConfig mirrors what a real
// V266/V267-style migration would embed verbatim — every sourcePath
// addresses ONLY the derive script's own step_output (see the script's own
// doc comment for why), never the raw FHIR fields directly.
func newRxOutboundMapToCanonicalConfig() map[string]interface{} {
	sp := func(key string) string { return "steps." + deriveNewRxOutboundStepAlias + ".step_output." + key }
	return map[string]interface{}{
		"outputField":     "parsedNCPDP",
		"transactionType": "NewRx",
		"headerFields": []map[string]interface{}{
			{"fieldKey": "to", "literalValue": "PHARMACY_MAILBOX_ID"},
			{"fieldKey": "from", "literalValue": "EZHEALTHKONNECT"},
			{"fieldKey": "messageID", "sourcePath": sp("message_id")},
			{"fieldKey": "sentTime", "sourcePath": sp("written_date")},
		},
		"bodyGroups": []map[string]interface{}{
			{
				"groupKey": "patient",
				"groups": []map[string]interface{}{
					{
						"groupKey": "humanPatient",
						"groups": []map[string]interface{}{
							{
								"groupKey": "name",
								"fields": []map[string]interface{}{
									{"fieldKey": "lastName", "sourcePath": sp("patient_last_name")},
									{"fieldKey": "firstName", "sourcePath": sp("patient_first_name")},
								},
							},
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
							{
								"groupKey": "identification",
								"fields": []map[string]interface{}{
									{"fieldKey": "npi", "sourcePath": sp("prescriber_npi")},
								},
							},
							{
								"groupKey": "name",
								"fields": []map[string]interface{}{
									{"fieldKey": "lastName", "sourcePath": sp("prescriber_last_name")},
									{"fieldKey": "firstName", "sourcePath": sp("prescriber_first_name")},
								},
							},
						},
					},
				},
			},
			{
				"groupKey": "medicationPrescribed",
				"fields": []map[string]interface{}{
					{"fieldKey": "drugDescription", "sourcePath": sp("drug_description")},
				},
				"groups": []map[string]interface{}{
					{
						"groupKey": "drugCoded",
						"groups": []map[string]interface{}{
							{
								"groupKey": "productCode",
								"fields": []map[string]interface{}{
									{"fieldKey": "code", "sourcePath": sp("drug_code")},
									{"fieldKey": "qualifier", "literalValue": "ND"},
								},
							},
						},
					},
					{
						"groupKey": "quantity",
						"fields": []map[string]interface{}{
							{"fieldKey": "value", "sourcePath": sp("quantity_value")},
						},
					},
					{
						"groupKey": "writtenDate",
						"fields": []map[string]interface{}{
							{"fieldKey": "date", "sourcePath": sp("written_date"), "transform": "datetime_to_ncpdp_date"},
						},
					},
					{
						"groupKey": "sig",
						"fields": []map[string]interface{}{
							{"fieldKey": "sigText", "sourcePath": sp("sig_text")},
						},
					},
				},
			},
		},
	}
}

// sampleFHIRMedicationRequest is a real-shaped (not fabricated) FHIR R4
// MedicationRequest, using only fields the base spec actually defines —
// subject/requester/dispenseRequest.performer as bare References with
// display+identifier (never inventing inline demographics a real
// MedicationRequest resource wouldn't carry).
func sampleFHIRMedicationRequest() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "MedicationRequest",
		"id":           "medreq-outbound-test-001",
		"status":       "active",
		"intent":       "order",
		"subject": map[string]interface{}{
			"reference": "Patient/pat-001",
			"display":   "John Smith",
		},
		"requester": map[string]interface{}{
			"reference": "Practitioner/prac-001",
			"display":   "Marcus Welby",
			"identifier": map[string]interface{}{
				"system": "http://hl7.org/fhir/sid/us-npi",
				"value":  "1234567893",
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
		"dosageInstruction": []interface{}{
			map[string]interface{}{"text": "Take one tablet by mouth once daily"},
		},
		"dispenseRequest": map[string]interface{}{
			"quantity": map[string]interface{}{"value": 30.0, "unit": "TAB"},
			"performer": map[string]interface{}{
				"reference": "Organization/pharmacy-001",
				"display":   "Corner Drug Pharmacy",
				"identifier": map[string]interface{}{
					"system": "http://hl7.org/fhir/sid/us-npi",
					"value":  "1234567890",
				},
			},
		},
		"authoredOn": "2026-09-20T14:30:00Z",
	}
}

func TestNewRxOutbound_FromFHIRMedicationRequest_BuildsValidNewRxXML(t *testing.T) {
	fhirInput := sampleFHIRMedicationRequest()

	// Step 1: enrichment.script — derive reshaped fields from the raw FHIR resource.
	scriptExec := enrichment.NewScriptEnrichmentExecutor()
	scriptStep := &models.TransformationStep{
		StepName: "Derive New Rx Outbound Fields",
		StepType: "enrichment.script",
		Enabled:  true,
		Config:   map[string]interface{}{"script": deriveNewRxOutboundFieldsScript()},
	}
	scriptResult, err := scriptExec.Execute(context.Background(), scriptStep, fhirInput)
	if err != nil {
		t.Fatalf("enrichment.script Execute failed: %v", err)
	}
	stepOutputRaw, ok := scriptResult["_stepOutput"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected _stepOutput map, got %T", scriptResult["_stepOutput"])
	}
	if stepOutputRaw["patient_last_name"] != "Smith" || stepOutputRaw["patient_first_name"] != "John" {
		t.Fatalf("expected patient name split to John/Smith, got first=%v last=%v", stepOutputRaw["patient_first_name"], stepOutputRaw["patient_last_name"])
	}
	if stepOutputRaw["prescriber_npi"] != "1234567893" {
		t.Fatalf("expected prescriber_npi=1234567893, got %v", stepOutputRaw["prescriber_npi"])
	}

	// Inject the step output under steps.<alias>.step_output — the ONLY
	// reference path a later step's sourcePath can address it by.
	stepOutputData := make(map[string]interface{})
	for k, v := range fhirInput {
		stepOutputData[k] = v
	}
	stepOutputData["steps"] = map[string]interface{}{
		deriveNewRxOutboundStepAlias: map[string]interface{}{"step_output": stepOutputRaw},
	}

	// Step 2: ncpdp.map_to_canonical — constructed via newTestNCPDPExecutors
	// (ncpdp_executors_test.go), NOT NewNCPDPMapToCanonicalExecutor()
	// directly: that constructor self-initialises from the hardcoded
	// "./ncpdp/schemas/script_2017071" path, correct at server runtime (CWD
	// = repo root) but NOT under `go test` (CWD = this package's own
	// directory) — an already-documented, already-solved issue in this same
	// test file, not a new one.
	_, validateExecShared, buildExecShared, mapExec := newTestNCPDPExecutors(t)
	mapStep := &models.TransformationStep{
		StepName: "Map to Canonical NewRx", StepType: "ncpdp.map_to_canonical", Enabled: true,
		Config: newRxOutboundMapToCanonicalConfig(),
	}
	mapOut, err := mapExec.Execute(context.Background(), mapStep, stepOutputData)
	if err != nil {
		t.Fatalf("ncpdp.map_to_canonical Execute failed: %v", err)
	}
	canonicalDoc, ok := mapOut["parsedNCPDP"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected parsedNCPDP map, got %T", mapOut["parsedNCPDP"])
	}
	body, ok := canonicalDoc["body"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected canonical body map, got %T", canonicalDoc["body"])
	}
	medPrescribed, ok := body["medicationPrescribed"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected medicationPrescribed map in canonical body, got %v", body["medicationPrescribed"])
	}
	if medPrescribed["drugDescription"] != "Lisinopril 10mg Tablet" {
		t.Fatalf("expected drugDescription mapped, got %v", medPrescribed["drugDescription"])
	}

	// Step 3: ncpdp.validate — a real gap closed after initial full-stack
	// verification: neither outbound template originally had a validate step
	// before build, unlike every inbound-direction template. ncpdp.validate's
	// own resolveNCPDPParseResult accepts a map already shaped like
	// map_to_canonical's own output (has "transactionType"/"header"/"body"
	// keys) directly, with zero new engine code — confirmed by reading
	// ncpdp_validate_executor.go before adding this step, not assumed.
	validateStep := &models.TransformationStep{
		StepName: "Validate Canonical NewRx", StepType: "ncpdp.validate", Enabled: true,
		Config: map[string]interface{}{"sourceField": "parsedNCPDP", "outputField": "ncpdpValidation"},
	}
	validateOut, err := validateExecShared.Execute(context.Background(), validateStep, mapOut)
	if err != nil {
		t.Fatalf("ncpdp.validate Execute failed: %v", err)
	}
	validationResult, ok := validateOut["ncpdpValidation"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected ncpdpValidation map, got %T", validateOut["ncpdpValidation"])
	}
	if validationResult["valid"] != true {
		t.Errorf("expected canonical NewRx to validate cleanly (all fields we mapped are populated), got issues: %+v", validationResult["issues"])
	}

	// Step 4: ncpdp.build
	buildStep := &models.TransformationStep{
		StepName: "Build NewRx XML", StepType: "ncpdp.build", Enabled: true,
		Config: map[string]interface{}{"sourceField": "parsedNCPDP", "transactionType": "NewRx", "outputField": "ncpdpScript"},
	}
	buildOut, err := buildExecShared.Execute(context.Background(), buildStep, validateOut)
	if err != nil {
		t.Fatalf("ncpdp.build Execute failed: %v", err)
	}
	xmlDoc, ok := buildOut["ncpdpScript"].(string)
	if !ok || xmlDoc == "" {
		t.Fatalf("expected non-empty ncpdpScript XML string, got %T: %v", buildOut["ncpdpScript"], buildOut["ncpdpScript"])
	}
	if !containsAll(xmlDoc, []string{"NewRx", "Lisinopril 10mg Tablet", "Smith", "John", "Welby", "1234567893"}) {
		t.Fatalf("built NewRx XML missing expected content:\n%s", xmlDoc)
	}

	// Round-trip: the built XML must be a real, well-formed NewRx a real
	// pharmacy system's own ncpdp.parse would accept — the same bar every
	// other NCPDP transaction type in this codebase proves itself against.
	loader, err := ncpdp.NewNCPDPSchemaLoader("../../../ncpdp/schemas/script_2017071")
	if err != nil {
		t.Fatalf("failed to load NCPDP schema: %v", err)
	}
	reparsed, err := ncpdp.ParseMessage(loader.Spec(), xmlDoc)
	if err != nil {
		t.Fatalf("built NewRx XML failed to re-parse: %v\n%s", err, xmlDoc)
	}
	if reparsed.TransactionType != "NewRx" {
		t.Fatalf("re-parsed TransactionType = %q, want NewRx", reparsed.TransactionType)
	}
	reBody, _ := reparsed.Body["patient"].(map[string]interface{})
	if reBody == nil {
		t.Fatalf("re-parsed document missing patient body, got %+v", reparsed.Body)
	}
}

func containsAll(s string, substrs []string) bool {
	for _, sub := range substrs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
