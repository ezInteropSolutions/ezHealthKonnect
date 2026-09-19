package services

// ─────────────────────────────────────────────────────────────────────────────
// NCPDP SCRIPT RxRenewalRequest → FHIR mapping
//
// RxRenewalRequest carries the SAME body shape as NewRx/CancelRx/
// RxChangeRequest (Patient/Pharmacy/Prescriber/MedicationPrescribed, per
// ncpdp/schemas/script_2017071/transactions/RxRenewalRequest.json —
// cross-validated against cosyte/ncpdp's own LifecycleRequestFields, which
// RxRenewalRequest extends with ZERO additional fields), so this mirrors
// ncpdp_cancelrx_fhir_builder_test.go's own Organization/Patient/
// Practitioner/MedicationRequest shape almost exactly. The one real semantic
// difference: MedicationRequest.status="active" + intent="order" (a renewal
// request represents the pharmacy asking to CONTINUE an existing, currently
// active prescription — unlike RxChangeRequest's own "draft"/"proposal",
// there is no pharmacy-proposed alteration here, just a request to keep
// dispensing the same medication) with a business identifier
// (requestReferenceNumber) so a later RxRenewalResponse can be correlated
// back to it.
//
// Config here is transcribed verbatim into a database migration — matching
// every other *_fhir_builder_test.go's own "config here is the source of
// truth" discipline.
//
// Run: go test ./services/ -v -run TestNCPDPRxRenewalRequestFHIRBuilder
// ─────────────────────────────────────────────────────────────────────────────

import (
	"os"
	"testing"

	"ezhealthkonnect/models"
	"ezhealthkonnect/ncpdp"
)

const ncpdpParseRxRenewalRequestStepAlias = "parse_rx_renewal_request"

func ncpdpRxRenewalRequestData(t *testing.T) map[string]interface{} {
	t.Helper()
	loader, err := ncpdp.NewNCPDPSchemaLoader(testNCPDPSchemaDirSvc)
	if err != nil {
		t.Fatalf("failed to load NCPDP schema: %v", err)
	}
	raw, err := os.ReadFile("../ncpdp/testdata/self_authored/rx_renewal_request_sample.xml")
	if err != nil {
		t.Fatalf("failed to read self-authored RxRenewalRequest sample: %v", err)
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
	svcInjectStepOutput(data, ncpdpParseRxRenewalRequestStepAlias, normalized)
	return data
}

func ncpdpRxRenewalRequestSrc(path string) string {
	return "steps." + ncpdpParseRxRenewalRequestStepAlias + ".step_output.parsed_ncpdp." + path
}

func organizationRxRenewalRequestBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Organization",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirPharmacyOrganization",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": ncpdpRxRenewalRequestSrc("body.pharmacy.identification.ncpdpid"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "organization-pharmacy-"}},
			map[string]interface{}{"targetPath": "active", "literalValue": "true"},
			map[string]interface{}{"targetPath": "name", "sourcePath": ncpdpRxRenewalRequestSrc("body.pharmacy.business_name")},
			map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/ncpdp-provider-id"},
			map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": ncpdpRxRenewalRequestSrc("body.pharmacy.identification.ncpdpid")},
		},
	}
}

func patientRxRenewalRequestBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Patient",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirPatient",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": ncpdpRxRenewalRequestSrc("header.message_id"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "patient-"}},
			map[string]interface{}{"targetPath": "name[0].family", "sourcePath": ncpdpRxRenewalRequestSrc("body.patient.human_patient.name.last_name")},
			map[string]interface{}{"targetPath": "name[0].given[0]", "sourcePath": ncpdpRxRenewalRequestSrc("body.patient.human_patient.name.first_name")},
			map[string]interface{}{"targetPath": "gender", "literalValue": "male", "condition": map[string]interface{}{"field": ncpdpRxRenewalRequestSrc("body.patient.human_patient.gender"), "operator": "equals", "value": "M"}},
			map[string]interface{}{"targetPath": "gender", "literalValue": "female", "condition": map[string]interface{}{"field": ncpdpRxRenewalRequestSrc("body.patient.human_patient.gender"), "operator": "equals", "value": "F"}},
			map[string]interface{}{"targetPath": "gender", "literalValue": "unknown", "condition": map[string]interface{}{"field": ncpdpRxRenewalRequestSrc("body.patient.human_patient.gender"), "operator": "equals", "value": "U"}},
		},
	}
}

func practitionerRxRenewalRequestBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Practitioner",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirPractitioner",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": ncpdpRxRenewalRequestSrc("body.prescriber.non_veterinarian.identification.npi"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "practitioner-"}},
			map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://hl7.org/fhir/sid/us-npi"},
			map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": ncpdpRxRenewalRequestSrc("body.prescriber.non_veterinarian.identification.npi")},
			map[string]interface{}{"targetPath": "name[0].family", "sourcePath": ncpdpRxRenewalRequestSrc("body.prescriber.non_veterinarian.name.last_name")},
			map[string]interface{}{"targetPath": "name[0].given[0]", "sourcePath": ncpdpRxRenewalRequestSrc("body.prescriber.non_veterinarian.name.first_name")},
		},
	}
}

func medicationRequestRxRenewalRequestBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "MedicationRequest",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirMedicationRequest",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": ncpdpRxRenewalRequestSrc("header.message_id"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "medicationrequest-"}},
			map[string]interface{}{"targetPath": "status", "literalValue": "active"},
			map[string]interface{}{"targetPath": "intent", "literalValue": "order"},
			map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/ncpdp-request-reference-number"},
			map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": ncpdpRxRenewalRequestSrc("body.request_reference_number")},
			map[string]interface{}{"targetPath": "medicationCodeableConcept.text", "sourcePath": ncpdpRxRenewalRequestSrc("body.medication_prescribed.drug_description")},
			map[string]interface{}{"targetPath": "subject.reference", "sourcePath": ncpdpRxRenewalRequestSrc("header.message_id"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Patient/patient-"}},
			map[string]interface{}{"targetPath": "requester.reference", "sourcePath": ncpdpRxRenewalRequestSrc("body.prescriber.non_veterinarian.identification.npi"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Practitioner/practitioner-"}},
			map[string]interface{}{"targetPath": "dispenseRequest.performer.reference", "sourcePath": ncpdpRxRenewalRequestSrc("body.pharmacy.identification.ncpdpid"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Organization/organization-pharmacy-"}},
			map[string]interface{}{"targetPath": "dispenseRequest.quantity.value", "sourcePath": ncpdpRxRenewalRequestSrc("body.medication_prescribed.quantity.value"), "transform": "cda_decimal_string_to_number"},
		},
	}
}

func TestNCPDPRxRenewalRequestFHIRBuilder_SelfAuthoredSample_BuildsCleanValidatingBundle(t *testing.T) {
	initFHIRRegistrySvc(t)
	data := ncpdpRxRenewalRequestData(t)

	data = runFHIRBuild(t, "build_pharmacy_organization_fhir", organizationRxRenewalRequestBuildConfig(), data)
	data = runFHIRBuild(t, "build_patient_fhir", patientRxRenewalRequestBuildConfig(), data)
	data = runFHIRBuild(t, "build_practitioner_fhir", practitionerRxRenewalRequestBuildConfig(), data)
	data = runFHIRBuild(t, "build_medication_request_fhir", medicationRequestRxRenewalRequestBuildConfig(), data)

	message := data["message"].(map[string]interface{})
	medReq := message["fhirMedicationRequest"].(map[string]interface{})
	if medReq["status"] != "active" {
		t.Errorf("MedicationRequest.status = %v, want \"active\"", medReq["status"])
	}
	if medReq["intent"] != "order" {
		t.Errorf("MedicationRequest.intent = %v, want \"order\"", medReq["intent"])
	}
	idArr, ok := medReq["identifier"].([]interface{})
	if !ok || len(idArr) == 0 {
		t.Fatalf("expected MedicationRequest.identifier to carry the requestReferenceNumber, got %v", medReq["identifier"])
	}
	if idArr[0].(map[string]interface{})["value"] != "REQ-0003" {
		t.Errorf("MedicationRequest.identifier[0].value = %v, want REQ-0003", idArr[0].(map[string]interface{})["value"])
	}

	assembleAndValidateNCPDPBundle(t, data, "assemble_rxrenewalrequest_bundle",
		[]string{"message.fhirPharmacyOrganization", "message.fhirPatient", "message.fhirPractitioner", "message.fhirMedicationRequest"})
}
