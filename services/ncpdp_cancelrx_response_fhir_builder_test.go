package services

// ─────────────────────────────────────────────────────────────────────────────
// NCPDP SCRIPT CancelRxResponse → FHIR mapping
//
// FHIR target: Task (NOT MedicationRequest) — a real, deliberate design
// correction from this phase's original plan draft. CancelRxResponse (per
// ncpdp/schemas/script_2017071/transactions/CancelRxResponse.json) carries
// ONLY requestReferenceNumber + one of 6 outcome choices (each: reasonCode,
// referenceNumber, denialReason, note) + an optional medicationPrescribed —
// NO Patient/Prescriber/Pharmacy fields at all (confirmed directly against
// the schema file, not assumed). Base FHIR MedicationRequest.subject is 1..1
// required; fabricating a Patient reference this message never carries would
// violate this project's own no-invented-structure discipline. Task.for is
// 0..1 optional, so Task can honestly represent "the response to a
// cancellation request, correlated by its own business identifier" without
// inventing patient linkage — the same class of correction 837P/837I's own
// "no separate Practitioner resource" design note already established
// (avoid fabricating a resource the source data can't honestly support).
//
// Config here is transcribed verbatim into a database migration — matching
// every other *_fhir_builder_test.go's own "config here is the source of
// truth" discipline.
//
// Run: go test ./services/ -v -run TestNCPDPCancelRxResponseFHIRBuilder
// ─────────────────────────────────────────────────────────────────────────────

import (
	"os"
	"testing"

	"ezhealthkonnect/models"
	"ezhealthkonnect/ncpdp"
)

const ncpdpParseCancelRxResponseStepAlias = "parse_cancel_rx_response"

func ncpdpCancelRxResponseData(t *testing.T, fixture string) map[string]interface{} {
	t.Helper()
	loader, err := ncpdp.NewNCPDPSchemaLoader(testNCPDPSchemaDirSvc)
	if err != nil {
		t.Fatalf("failed to load NCPDP schema: %v", err)
	}
	raw, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("failed to read fixture %s: %v", fixture, err)
	}
	parsed, err := ncpdp.ParseMessage(loader.Spec(), string(raw))
	if err != nil {
		t.Fatalf("ncpdp.ParseMessage failed: %v", err)
	}

	rawStepOutput := map[string]interface{}{
		"parsedNCPDP": map[string]interface{}{
			"transactionType": parsed.TransactionType,
			"header":          parsed.Header,
			"body":            parsed.Body,
		},
	}
	normalized := models.NewOutputNormalizer().NormalizeStepOutput(rawStepOutput)

	data := map[string]interface{}{}
	svcInjectStepOutput(data, ncpdpParseCancelRxResponseStepAlias, normalized)
	return data
}

func ncpdpCancelRxResponseSrc(path string) string {
	return "steps." + ncpdpParseCancelRxResponseStepAlias + ".step_output.parsed_ncpdp." + path
}

// taskCancelRxResponseBuildConfig is reused (with a different step-alias
// prefix baked into ncpdpCancelRxResponseSrc) for RxChangeResponse too — see
// ncpdp_rxchangeresponse_fhir_builder_test.go's own doc comment for why that
// file defines its own copy instead of a shared parameterized function
// (each transaction type gets its own dedicated migration/template, matching
// this project's "separate templates for genuinely different shapes"
// precedent, even when the FHIR shape happens to be identical).
func taskCancelRxResponseBuildConfig() map[string]interface{} {
	outcomeBranches := []string{"denied", "deny_new_to_follow", "approved", "approved_with_changes", "validated", "replace"}
	outcomeLiterals := map[string]string{
		"denied": "Denied", "deny_new_to_follow": "DenyNewToFollow", "approved": "Approved",
		"approved_with_changes": "ApprovedWithChanges", "validated": "Validated", "replace": "Replace",
	}
	rejectedBranches := map[string]bool{"denied": true, "deny_new_to_follow": true}

	fields := []interface{}{
		map[string]interface{}{"targetPath": "id", "sourcePath": ncpdpCancelRxResponseSrc("header.message_id"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "task-cancelrx-response-"}},
		map[string]interface{}{"targetPath": "intent", "literalValue": "order"},
		map[string]interface{}{"targetPath": "code.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/task-code"},
		map[string]interface{}{"targetPath": "code.coding[0].code", "literalValue": "fulfill"},
		map[string]interface{}{"targetPath": "description", "literalValue": "NCPDP SCRIPT CancelRx Response"},
		map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/ncpdp-request-reference-number"},
		map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": ncpdpCancelRxResponseSrc("body.request_reference_number")},
		map[string]interface{}{"targetPath": "businessStatus.coding[0].system", "literalValue": "http://ezhealthkonnect.local/ncpdp-cancelrx-outcome"},
	}

	// Exactly one of the 6 outcome groups is ever present on a real message
	// (this schema doesn't structurally enforce mutual exclusion, matching
	// the CDA choice-constraint scope boundary precedent) -- every field
	// below only resolves for whichever branch is actually present, so
	// stacking all 6 as ordinary conditional/fallback entries is safe.
	for _, branch := range outcomeBranches {
		status := "completed"
		if rejectedBranches[branch] {
			status = "rejected"
		}
		fields = append(fields,
			map[string]interface{}{"targetPath": "status", "literalValue": status, "condition": map[string]interface{}{"field": ncpdpCancelRxResponseSrc("body.response." + branch), "operator": "exists"}},
			map[string]interface{}{"targetPath": "businessStatus.coding[0].code", "literalValue": outcomeLiterals[branch], "condition": map[string]interface{}{"field": ncpdpCancelRxResponseSrc("body.response." + branch), "operator": "exists"}},
		)
	}

	// note[0].text sources the first PRESENT branch's own note text --
	// fallbackPaths checked in order, "first present value wins" (fhir.build's
	// own convention, same as sourcePath/literalValue elsewhere in this file).
	noteField := map[string]interface{}{"targetPath": "note[0].text", "sourcePath": ncpdpCancelRxResponseSrc("body.response.denied.note")}
	fallbacks := make([]interface{}, 0, len(outcomeBranches)-1)
	for _, branch := range outcomeBranches[1:] {
		fallbacks = append(fallbacks, ncpdpCancelRxResponseSrc("body.response."+branch+".note"))
	}
	noteField["fallbackPaths"] = fallbacks
	fields = append(fields, noteField)

	return map[string]interface{}{
		"resourceType": "Task",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirTask",
		"fields":       fields,
	}
}

func TestNCPDPCancelRxResponseFHIRBuilder_DeniedOutcome_BuildsCleanValidatingTask(t *testing.T) {
	initFHIRRegistrySvc(t)
	data := ncpdpCancelRxResponseData(t, "../ncpdp/testdata/self_authored/cancel_rx_response_denied_sample.xml")

	data = runFHIRBuild(t, "build_cancelrx_response_task_fhir", taskCancelRxResponseBuildConfig(), data)

	message := data["message"].(map[string]interface{})
	task := message["fhirTask"].(map[string]interface{})
	if task["status"] != "rejected" {
		t.Errorf("Task.status = %v, want \"rejected\" (Denied outcome)", task["status"])
	}
	bsCoding := task["businessStatus"].(map[string]interface{})["coding"].([]interface{})[0].(map[string]interface{})
	if bsCoding["code"] != "Denied" {
		t.Errorf("Task.businessStatus.coding[0].code = %v, want \"Denied\"", bsCoding["code"])
	}
	noteArr := task["note"].([]interface{})
	if noteArr[0].(map[string]interface{})["text"] != "Patient already picked up medication" {
		t.Errorf("Task.note[0].text = %v, want the real note text", noteArr[0].(map[string]interface{})["text"])
	}
	idArr := task["identifier"].([]interface{})
	if idArr[0].(map[string]interface{})["value"] != "REQ-0001" {
		t.Errorf("Task.identifier[0].value = %v, want REQ-0001", idArr[0].(map[string]interface{})["value"])
	}

	assembleAndValidateNCPDPBundle(t, data, "assemble_cancelrx_response_bundle", []string{"message.fhirTask"})
}
