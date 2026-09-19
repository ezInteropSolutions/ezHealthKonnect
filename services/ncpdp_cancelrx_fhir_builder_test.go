package services

// ─────────────────────────────────────────────────────────────────────────────
// NCPDP SCRIPT CancelRx → FHIR mapping
//
// CancelRx carries the SAME body shape as NewRx (Patient/Pharmacy/Prescriber/
// MedicationPrescribed, per ncpdp/schemas/script_2017071/transactions/
// CancelRx.json), so this mapping mirrors ncpdp_newrx_fhir_builder_test.go's
// own Organization/Patient/Practitioner/MedicationRequest shape almost
// exactly — the two real differences are MedicationRequest.status="cancelled"
// (representing the prescriber's own cancellation intent — NCPDP has no
// "pending cancellation" status of its own, and base FHIR has no such state
// either, so the cancellation is modeled as already-effective from the
// sender's perspective, a named simplification) and a business identifier
// (requestReferenceNumber) carried on MedicationRequest.identifier so a
// later CancelRxResponse can be correlated back to it by a real system.
//
// Config here is transcribed verbatim into a database migration — matching
// every other *_fhir_builder_test.go's own "config here is the source of
// truth" discipline.
//
// Run: go test ./services/ -v -run TestNCPDPCancelRxFHIRBuilder
// ─────────────────────────────────────────────────────────────────────────────

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"ezhealthkonnect/models"
	"ezhealthkonnect/ncpdp"
	"ezhealthkonnect/services/executors/payload"
	"ezhealthkonnect/services/executors/validation"
)

const ncpdpParseCancelRxStepAlias = "parse_cancel_rx"

func ncpdpCancelRxData(t *testing.T) map[string]interface{} {
	t.Helper()
	loader, err := ncpdp.NewNCPDPSchemaLoader(testNCPDPSchemaDirSvc)
	if err != nil {
		t.Fatalf("failed to load NCPDP schema: %v", err)
	}
	raw, err := os.ReadFile("../ncpdp/testdata/self_authored/cancel_rx_sample.xml")
	if err != nil {
		t.Fatalf("failed to read self-authored CancelRx sample: %v", err)
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
	svcInjectStepOutput(data, ncpdpParseCancelRxStepAlias, normalized)
	return data
}

func ncpdpCancelRxSrc(path string) string {
	return "steps." + ncpdpParseCancelRxStepAlias + ".step_output.parsed_ncpdp." + path
}

func organizationCancelRxBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Organization",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirPharmacyOrganization",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": ncpdpCancelRxSrc("body.pharmacy.identification.ncpdpid"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "organization-pharmacy-"}},
			map[string]interface{}{"targetPath": "active", "literalValue": "true"},
			map[string]interface{}{"targetPath": "name", "sourcePath": ncpdpCancelRxSrc("body.pharmacy.business_name")},
			map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/ncpdp-provider-id"},
			map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": ncpdpCancelRxSrc("body.pharmacy.identification.ncpdpid")},
		},
	}
}

func patientCancelRxBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Patient",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirPatient",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": ncpdpCancelRxSrc("header.message_id"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "patient-"}},
			map[string]interface{}{"targetPath": "name[0].family", "sourcePath": ncpdpCancelRxSrc("body.patient.human_patient.name.last_name")},
			map[string]interface{}{"targetPath": "name[0].given[0]", "sourcePath": ncpdpCancelRxSrc("body.patient.human_patient.name.first_name")},
			map[string]interface{}{"targetPath": "gender", "literalValue": "male", "condition": map[string]interface{}{"field": ncpdpCancelRxSrc("body.patient.human_patient.gender"), "operator": "equals", "value": "M"}},
			map[string]interface{}{"targetPath": "gender", "literalValue": "female", "condition": map[string]interface{}{"field": ncpdpCancelRxSrc("body.patient.human_patient.gender"), "operator": "equals", "value": "F"}},
			map[string]interface{}{"targetPath": "gender", "literalValue": "unknown", "condition": map[string]interface{}{"field": ncpdpCancelRxSrc("body.patient.human_patient.gender"), "operator": "equals", "value": "U"}},
		},
	}
}

func practitionerCancelRxBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Practitioner",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirPractitioner",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": ncpdpCancelRxSrc("body.prescriber.non_veterinarian.identification.npi"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "practitioner-"}},
			map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://hl7.org/fhir/sid/us-npi"},
			map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": ncpdpCancelRxSrc("body.prescriber.non_veterinarian.identification.npi")},
			map[string]interface{}{"targetPath": "name[0].family", "sourcePath": ncpdpCancelRxSrc("body.prescriber.non_veterinarian.name.last_name")},
			map[string]interface{}{"targetPath": "name[0].given[0]", "sourcePath": ncpdpCancelRxSrc("body.prescriber.non_veterinarian.name.first_name")},
		},
	}
}

func medicationRequestCancelRxBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "MedicationRequest",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirMedicationRequest",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": ncpdpCancelRxSrc("header.message_id"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "medicationrequest-"}},
			map[string]interface{}{"targetPath": "status", "literalValue": "cancelled"},
			map[string]interface{}{"targetPath": "intent", "literalValue": "order"},
			map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/ncpdp-request-reference-number"},
			map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": ncpdpCancelRxSrc("body.request_reference_number")},
			map[string]interface{}{"targetPath": "medicationCodeableConcept.text", "sourcePath": ncpdpCancelRxSrc("body.medication_prescribed.drug_description")},
			map[string]interface{}{"targetPath": "subject.reference", "sourcePath": ncpdpCancelRxSrc("header.message_id"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Patient/patient-"}},
			map[string]interface{}{"targetPath": "requester.reference", "sourcePath": ncpdpCancelRxSrc("body.prescriber.non_veterinarian.identification.npi"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Practitioner/practitioner-"}},
			map[string]interface{}{"targetPath": "dispenseRequest.performer.reference", "sourcePath": ncpdpCancelRxSrc("body.pharmacy.identification.ncpdpid"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Organization/organization-pharmacy-"}},
			map[string]interface{}{"targetPath": "dispenseRequest.quantity.value", "sourcePath": ncpdpCancelRxSrc("body.medication_prescribed.quantity.value"), "transform": "cda_decimal_string_to_number"},
		},
	}
}

func TestNCPDPCancelRxFHIRBuilder_SelfAuthoredSample_BuildsCleanValidatingBundle(t *testing.T) {
	initFHIRRegistrySvc(t)
	data := ncpdpCancelRxData(t)

	data = runFHIRBuild(t, "build_pharmacy_organization_fhir", organizationCancelRxBuildConfig(), data)
	data = runFHIRBuild(t, "build_patient_fhir", patientCancelRxBuildConfig(), data)
	data = runFHIRBuild(t, "build_practitioner_fhir", practitionerCancelRxBuildConfig(), data)
	data = runFHIRBuild(t, "build_medication_request_fhir", medicationRequestCancelRxBuildConfig(), data)

	message := data["message"].(map[string]interface{})
	medReq := message["fhirMedicationRequest"].(map[string]interface{})
	if medReq["status"] != "cancelled" {
		t.Errorf("MedicationRequest.status = %v, want \"cancelled\"", medReq["status"])
	}
	idArr, ok := medReq["identifier"].([]interface{})
	if !ok || len(idArr) == 0 {
		t.Fatalf("expected MedicationRequest.identifier to carry the requestReferenceNumber, got %v", medReq["identifier"])
	}
	if idArr[0].(map[string]interface{})["value"] != "REQ-0001" {
		t.Errorf("MedicationRequest.identifier[0].value = %v, want REQ-0001", idArr[0].(map[string]interface{})["value"])
	}

	assembleAndValidateNCPDPBundle(t, data, "assemble_cancelrx_bundle",
		[]string{"message.fhirPharmacyOrganization", "message.fhirPatient", "message.fhirPractitioner", "message.fhirMedicationRequest"})
}

// assembleAndValidateNCPDPBundle mirrors assembleAndValidateNewRxBundle
// exactly, parameterized by alias so every NCPDP FHIR mapping test in this
// package can share one implementation instead of copy-pasting it per
// transaction type.
func assembleAndValidateNCPDPBundle(t *testing.T, data map[string]interface{}, alias string, resourcePaths []string) string {
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
