package services

// ─────────────────────────────────────────────────────────────────────────────
// NCPDP SCRIPT RxChangeRequest → FHIR mapping
//
// RxChangeRequest carries the SAME body shape as NewRx/CancelRx (Patient/
// Pharmacy/Prescriber/MedicationPrescribed), so this mirrors
// ncpdp_cancelrx_fhir_builder_test.go's own Organization/Patient/
// Practitioner/MedicationRequest shape almost exactly. The one real semantic
// difference: MedicationRequest.status="draft" + intent="proposal" (both
// real base-FHIR MedicationRequest enum values, chosen deliberately — a
// pharmacy's PROPOSED change is not yet prescriber-approved, unlike NewRx's
// own status="active"/intent="order" or CancelRx's status="cancelled") —
// RxChangeResponse's own outcome then represents the prescriber's actual
// decision (see ncpdp_rxchangeresponse_fhir_builder_test.go).
//
// Config here is transcribed verbatim into a database migration — matching
// every other *_fhir_builder_test.go's own "config here is the source of
// truth" discipline.
//
// Run: go test ./services/ -v -run TestNCPDPRxChangeRequestFHIRBuilder
// ─────────────────────────────────────────────────────────────────────────────

import (
	"os"
	"testing"

	"ezhealthkonnect/models"
	"ezhealthkonnect/ncpdp"
)

const ncpdpParseRxChangeRequestStepAlias = "parse_rx_change_request"

func ncpdpRxChangeRequestData(t *testing.T) map[string]interface{} {
	t.Helper()
	loader, err := ncpdp.NewNCPDPSchemaLoader(testNCPDPSchemaDirSvc)
	if err != nil {
		t.Fatalf("failed to load NCPDP schema: %v", err)
	}
	raw, err := os.ReadFile("../ncpdp/testdata/self_authored/rx_change_request_sample.xml")
	if err != nil {
		t.Fatalf("failed to read self-authored RxChangeRequest sample: %v", err)
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
	svcInjectStepOutput(data, ncpdpParseRxChangeRequestStepAlias, normalized)
	return data
}

func ncpdpRxChangeRequestSrc(path string) string {
	return "steps." + ncpdpParseRxChangeRequestStepAlias + ".step_output.parsed_ncpdp." + path
}

func organizationRxChangeRequestBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Organization",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirPharmacyOrganization",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": ncpdpRxChangeRequestSrc("body.pharmacy.identification.ncpdpid"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "organization-pharmacy-"}},
			map[string]interface{}{"targetPath": "active", "literalValue": "true"},
			map[string]interface{}{"targetPath": "name", "sourcePath": ncpdpRxChangeRequestSrc("body.pharmacy.business_name")},
			map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/ncpdp-provider-id"},
			map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": ncpdpRxChangeRequestSrc("body.pharmacy.identification.ncpdpid")},
		},
	}
}

func patientRxChangeRequestBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Patient",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirPatient",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": ncpdpRxChangeRequestSrc("header.message_id"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "patient-"}},
			map[string]interface{}{"targetPath": "name[0].family", "sourcePath": ncpdpRxChangeRequestSrc("body.patient.human_patient.name.last_name")},
			map[string]interface{}{"targetPath": "name[0].given[0]", "sourcePath": ncpdpRxChangeRequestSrc("body.patient.human_patient.name.first_name")},
			map[string]interface{}{"targetPath": "gender", "literalValue": "male", "condition": map[string]interface{}{"field": ncpdpRxChangeRequestSrc("body.patient.human_patient.gender"), "operator": "equals", "value": "M"}},
			map[string]interface{}{"targetPath": "gender", "literalValue": "female", "condition": map[string]interface{}{"field": ncpdpRxChangeRequestSrc("body.patient.human_patient.gender"), "operator": "equals", "value": "F"}},
			map[string]interface{}{"targetPath": "gender", "literalValue": "unknown", "condition": map[string]interface{}{"field": ncpdpRxChangeRequestSrc("body.patient.human_patient.gender"), "operator": "equals", "value": "U"}},
		},
	}
}

func practitionerRxChangeRequestBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Practitioner",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirPractitioner",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": ncpdpRxChangeRequestSrc("body.prescriber.non_veterinarian.identification.npi"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "practitioner-"}},
			map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://hl7.org/fhir/sid/us-npi"},
			map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": ncpdpRxChangeRequestSrc("body.prescriber.non_veterinarian.identification.npi")},
			map[string]interface{}{"targetPath": "name[0].family", "sourcePath": ncpdpRxChangeRequestSrc("body.prescriber.non_veterinarian.name.last_name")},
			map[string]interface{}{"targetPath": "name[0].given[0]", "sourcePath": ncpdpRxChangeRequestSrc("body.prescriber.non_veterinarian.name.first_name")},
		},
	}
}

func medicationRequestRxChangeRequestBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "MedicationRequest",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirMedicationRequest",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": ncpdpRxChangeRequestSrc("header.message_id"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "medicationrequest-"}},
			map[string]interface{}{"targetPath": "status", "literalValue": "draft"},
			map[string]interface{}{"targetPath": "intent", "literalValue": "proposal"},
			map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/ncpdp-request-reference-number"},
			map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": ncpdpRxChangeRequestSrc("body.request_reference_number")},
			map[string]interface{}{"targetPath": "medicationCodeableConcept.text", "sourcePath": ncpdpRxChangeRequestSrc("body.medication_prescribed.drug_description")},
			map[string]interface{}{"targetPath": "subject.reference", "sourcePath": ncpdpRxChangeRequestSrc("header.message_id"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Patient/patient-"}},
			map[string]interface{}{"targetPath": "requester.reference", "sourcePath": ncpdpRxChangeRequestSrc("body.prescriber.non_veterinarian.identification.npi"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Practitioner/practitioner-"}},
			map[string]interface{}{"targetPath": "dispenseRequest.performer.reference", "sourcePath": ncpdpRxChangeRequestSrc("body.pharmacy.identification.ncpdpid"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Organization/organization-pharmacy-"}},
			map[string]interface{}{"targetPath": "dispenseRequest.quantity.value", "sourcePath": ncpdpRxChangeRequestSrc("body.medication_prescribed.quantity.value"), "transform": "cda_decimal_string_to_number"},
		},
	}
}

func TestNCPDPRxChangeRequestFHIRBuilder_SelfAuthoredSample_BuildsCleanValidatingBundle(t *testing.T) {
	initFHIRRegistrySvc(t)
	data := ncpdpRxChangeRequestData(t)

	data = runFHIRBuild(t, "build_pharmacy_organization_fhir", organizationRxChangeRequestBuildConfig(), data)
	data = runFHIRBuild(t, "build_patient_fhir", patientRxChangeRequestBuildConfig(), data)
	data = runFHIRBuild(t, "build_practitioner_fhir", practitionerRxChangeRequestBuildConfig(), data)
	data = runFHIRBuild(t, "build_medication_request_fhir", medicationRequestRxChangeRequestBuildConfig(), data)

	message := data["message"].(map[string]interface{})
	medReq := message["fhirMedicationRequest"].(map[string]interface{})
	if medReq["status"] != "draft" {
		t.Errorf("MedicationRequest.status = %v, want \"draft\"", medReq["status"])
	}
	if medReq["intent"] != "proposal" {
		t.Errorf("MedicationRequest.intent = %v, want \"proposal\"", medReq["intent"])
	}

	assembleAndValidateNCPDPBundle(t, data, "assemble_rxchangerequest_bundle",
		[]string{"message.fhirPharmacyOrganization", "message.fhirPatient", "message.fhirPractitioner", "message.fhirMedicationRequest"})
}
