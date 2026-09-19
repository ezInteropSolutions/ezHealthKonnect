package services

// ─────────────────────────────────────────────────────────────────────────────
// NCPDP SCRIPT RxRenewalResponse → FHIR mapping
//
// FHIR target: Task, same reasoning as CancelRxResponse/RxChangeResponse
// (see ncpdp_cancelrx_response_fhir_builder_test.go's own doc comment) — the
// schema carries no Patient/Prescriber identification for response
// messages, so a base-FHIR-required MedicationRequest.subject can't be
// honestly populated. Cross-validated against cosyte/ncpdp's own
// LifecycleResponseFields, which RxRenewalResponse extends with ZERO
// additional fields of its own — the same 6 outcome elements, same
// fail-safe denial-first precedence, shared verbatim across all 3 lifecycle
// response kinds (see RxRenewalResponse.json's own sourceRefs).
//
// Unlike CancelRxResponse (but like RxChangeResponse), RxRenewalResponse's
// own optional medicationPrescribed represents real content worth carrying
// forward on an ApprovedWithChanges outcome — Task.description sources it
// when present, falling back to a generic label.
//
// Config here is transcribed verbatim into a database migration — matching
// every other *_fhir_builder_test.go's own "config here is the source of
// truth" discipline.
//
// Run: go test ./services/ -v -run TestNCPDPRxRenewalResponseFHIRBuilder
// ─────────────────────────────────────────────────────────────────────────────

import (
	"os"
	"testing"

	"ezhealthkonnect/models"
	"ezhealthkonnect/ncpdp"
)

const ncpdpParseRxRenewalResponseStepAlias = "parse_rx_renewal_response"

func ncpdpRxRenewalResponseData(t *testing.T, fixture string) map[string]interface{} {
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
	svcInjectStepOutput(data, ncpdpParseRxRenewalResponseStepAlias, normalized)
	return data
}

func ncpdpRxRenewalResponseSrc(path string) string {
	return "steps." + ncpdpParseRxRenewalResponseStepAlias + ".step_output.parsed_ncpdp." + path
}

func taskRxRenewalResponseBuildConfig() map[string]interface{} {
	outcomeBranches := []string{"denied", "deny_new_to_follow", "approved", "approved_with_changes", "validated", "replace"}
	outcomeLiterals := map[string]string{
		"denied": "Denied", "deny_new_to_follow": "DenyNewToFollow", "approved": "Approved",
		"approved_with_changes": "ApprovedWithChanges", "validated": "Validated", "replace": "Replace",
	}
	rejectedBranches := map[string]bool{"denied": true, "deny_new_to_follow": true}

	fields := []interface{}{
		map[string]interface{}{"targetPath": "id", "sourcePath": ncpdpRxRenewalResponseSrc("header.message_id"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "task-rxrenewalresponse-"}},
		map[string]interface{}{"targetPath": "intent", "literalValue": "order"},
		map[string]interface{}{"targetPath": "code.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/task-code"},
		map[string]interface{}{"targetPath": "code.coding[0].code", "literalValue": "fulfill"},
		map[string]interface{}{"targetPath": "description", "sourcePath": ncpdpRxRenewalResponseSrc("body.medication_prescribed.drug_description"), "literalValue": "NCPDP SCRIPT RxRenewalRequest Response"},
		map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/ncpdp-request-reference-number"},
		map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": ncpdpRxRenewalResponseSrc("body.request_reference_number")},
		map[string]interface{}{"targetPath": "businessStatus.coding[0].system", "literalValue": "http://ezhealthkonnect.local/ncpdp-rxrenewalresponse-outcome"},
	}

	for _, branch := range outcomeBranches {
		status := "completed"
		if rejectedBranches[branch] {
			status = "rejected"
		}
		fields = append(fields,
			map[string]interface{}{"targetPath": "status", "literalValue": status, "condition": map[string]interface{}{"field": ncpdpRxRenewalResponseSrc("body.response." + branch), "operator": "exists"}},
			map[string]interface{}{"targetPath": "businessStatus.coding[0].code", "literalValue": outcomeLiterals[branch], "condition": map[string]interface{}{"field": ncpdpRxRenewalResponseSrc("body.response." + branch), "operator": "exists"}},
		)
	}

	noteField := map[string]interface{}{"targetPath": "note[0].text", "sourcePath": ncpdpRxRenewalResponseSrc("body.response.denied.note")}
	fallbacks := make([]interface{}, 0, len(outcomeBranches)-1)
	for _, branch := range outcomeBranches[1:] {
		fallbacks = append(fallbacks, ncpdpRxRenewalResponseSrc("body.response."+branch+".note"))
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

func TestNCPDPRxRenewalResponseFHIRBuilder_ApprovedOutcome_BuildsCleanValidatingTask(t *testing.T) {
	initFHIRRegistrySvc(t)
	data := ncpdpRxRenewalResponseData(t, "../ncpdp/testdata/self_authored/rx_renewal_response_approved_sample.xml")

	data = runFHIRBuild(t, "build_rxrenewalresponse_task_fhir", taskRxRenewalResponseBuildConfig(), data)

	message := data["message"].(map[string]interface{})
	task := message["fhirTask"].(map[string]interface{})
	if task["status"] != "completed" {
		t.Errorf("Task.status = %v, want \"completed\" (Approved outcome)", task["status"])
	}
	bsCoding := task["businessStatus"].(map[string]interface{})["coding"].([]interface{})[0].(map[string]interface{})
	if bsCoding["code"] != "Approved" {
		t.Errorf("Task.businessStatus.coding[0].code = %v, want \"Approved\"", bsCoding["code"])
	}
	noteArr := task["note"].([]interface{})
	if noteArr[0].(map[string]interface{})["text"] != "Renewal approved as written" {
		t.Errorf("Task.note[0].text = %v, want the real note text", noteArr[0].(map[string]interface{})["text"])
	}
	idArr := task["identifier"].([]interface{})
	if idArr[0].(map[string]interface{})["value"] != "REQ-0003" {
		t.Errorf("Task.identifier[0].value = %v, want REQ-0003", idArr[0].(map[string]interface{})["value"])
	}

	assembleAndValidateNCPDPBundle(t, data, "assemble_rxrenewalresponse_bundle", []string{"message.fhirTask"})
}
