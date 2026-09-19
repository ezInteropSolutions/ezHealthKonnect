package services

// ─────────────────────────────────────────────────────────────────────────────
// NCPDP SCRIPT RxChangeResponse → FHIR mapping
//
// FHIR target: Task, same reasoning as CancelRxResponse (see
// ncpdp_cancelrx_response_fhir_builder_test.go's own doc comment) — the
// schema carries no Patient/Prescriber identification for response
// messages, so a base-FHIR-required MedicationRequest.subject can't be
// honestly populated. A separate Go file (rather than a shared parameterized
// helper) matches this project's "separate templates for genuinely
// different shapes" precedent even where the FHIR shape is largely
// identical — RxChangeResponse's own OOB template is a distinct migration.
//
// Unlike CancelRxResponse, RxChangeResponse's own optional medicationPrescribed
// (present, typically, alongside ApprovedWithChanges) represents real new
// prescribing content — Task.description sources it when present, falling
// back to a generic label, rather than building a second, patient-less
// MedicationRequest resource (the same "don't fabricate a resource the
// source data can't honestly support" call already made for CancelRxResponse).
//
// Config here is transcribed verbatim into a database migration — matching
// every other *_fhir_builder_test.go's own "config here is the source of
// truth" discipline.
//
// Run: go test ./services/ -v -run TestNCPDPRxChangeResponseFHIRBuilder
// ─────────────────────────────────────────────────────────────────────────────

import (
	"os"
	"testing"

	"ezhealthkonnect/models"
	"ezhealthkonnect/ncpdp"
)

const ncpdpParseRxChangeResponseStepAlias = "parse_rx_change_response"

func ncpdpRxChangeResponseData(t *testing.T) map[string]interface{} {
	t.Helper()
	loader, err := ncpdp.NewNCPDPSchemaLoader(testNCPDPSchemaDirSvc)
	if err != nil {
		t.Fatalf("failed to load NCPDP schema: %v", err)
	}
	raw, err := os.ReadFile("../ncpdp/testdata/self_authored/rx_change_response_approved_with_changes_sample.xml")
	if err != nil {
		t.Fatalf("failed to read self-authored RxChangeResponse sample: %v", err)
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
	svcInjectStepOutput(data, ncpdpParseRxChangeResponseStepAlias, normalized)
	return data
}

func ncpdpRxChangeResponseSrc(path string) string {
	return "steps." + ncpdpParseRxChangeResponseStepAlias + ".step_output.parsed_ncpdp." + path
}

func taskRxChangeResponseBuildConfig() map[string]interface{} {
	outcomeBranches := []string{"denied", "deny_new_to_follow", "approved", "approved_with_changes", "validated", "replace"}
	outcomeLiterals := map[string]string{
		"denied": "Denied", "deny_new_to_follow": "DenyNewToFollow", "approved": "Approved",
		"approved_with_changes": "ApprovedWithChanges", "validated": "Validated", "replace": "Replace",
	}
	rejectedBranches := map[string]bool{"denied": true, "deny_new_to_follow": true}

	fields := []interface{}{
		map[string]interface{}{"targetPath": "id", "sourcePath": ncpdpRxChangeResponseSrc("header.message_id"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "task-rxchangeresponse-"}},
		map[string]interface{}{"targetPath": "intent", "literalValue": "order"},
		map[string]interface{}{"targetPath": "code.coding[0].system", "literalValue": "http://terminology.hl7.org/CodeSystem/task-code"},
		map[string]interface{}{"targetPath": "code.coding[0].code", "literalValue": "fulfill"},
		// description sources the approved/modified medication's own drug
		// description when medicationPrescribed is present (typically
		// alongside ApprovedWithChanges); literalValue is the fallback for
		// every response that carries no medication content at all.
		map[string]interface{}{"targetPath": "description", "sourcePath": ncpdpRxChangeResponseSrc("body.medication_prescribed.drug_description"), "literalValue": "NCPDP SCRIPT RxChangeRequest Response"},
		map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/ncpdp-request-reference-number"},
		map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": ncpdpRxChangeResponseSrc("body.request_reference_number")},
		map[string]interface{}{"targetPath": "businessStatus.coding[0].system", "literalValue": "http://ezhealthkonnect.local/ncpdp-rxchangeresponse-outcome"},
	}

	for _, branch := range outcomeBranches {
		status := "completed"
		if rejectedBranches[branch] {
			status = "rejected"
		}
		fields = append(fields,
			map[string]interface{}{"targetPath": "status", "literalValue": status, "condition": map[string]interface{}{"field": ncpdpRxChangeResponseSrc("body.response." + branch), "operator": "exists"}},
			map[string]interface{}{"targetPath": "businessStatus.coding[0].code", "literalValue": outcomeLiterals[branch], "condition": map[string]interface{}{"field": ncpdpRxChangeResponseSrc("body.response." + branch), "operator": "exists"}},
		)
	}

	noteField := map[string]interface{}{"targetPath": "note[0].text", "sourcePath": ncpdpRxChangeResponseSrc("body.response.denied.note")}
	fallbacks := make([]interface{}, 0, len(outcomeBranches)-1)
	for _, branch := range outcomeBranches[1:] {
		fallbacks = append(fallbacks, ncpdpRxChangeResponseSrc("body.response."+branch+".note"))
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

func TestNCPDPRxChangeResponseFHIRBuilder_ApprovedWithChangesOutcome_BuildsCleanValidatingTask(t *testing.T) {
	initFHIRRegistrySvc(t)
	data := ncpdpRxChangeResponseData(t)

	data = runFHIRBuild(t, "build_rxchangeresponse_task_fhir", taskRxChangeResponseBuildConfig(), data)

	message := data["message"].(map[string]interface{})
	task := message["fhirTask"].(map[string]interface{})
	if task["status"] != "completed" {
		t.Errorf("Task.status = %v, want \"completed\" (ApprovedWithChanges outcome)", task["status"])
	}
	bsCoding := task["businessStatus"].(map[string]interface{})["coding"].([]interface{})[0].(map[string]interface{})
	if bsCoding["code"] != "ApprovedWithChanges" {
		t.Errorf("Task.businessStatus.coding[0].code = %v, want \"ApprovedWithChanges\"", bsCoding["code"])
	}
	if task["description"] != "Ondansetron ODT 4 mg" {
		t.Errorf("Task.description = %v, want the real approved drug description", task["description"])
	}
	noteArr := task["note"].([]interface{})
	if noteArr[0].(map[string]interface{})["text"] != "Approved generic substitution" {
		t.Errorf("Task.note[0].text = %v, want the real note text", noteArr[0].(map[string]interface{})["text"])
	}

	assembleAndValidateNCPDPBundle(t, data, "assemble_rxchangeresponse_bundle", []string{"message.fhirTask"})
}
