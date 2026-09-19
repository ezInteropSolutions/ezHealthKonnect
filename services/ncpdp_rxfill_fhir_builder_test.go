package services

// ─────────────────────────────────────────────────────────────────────────────
// NCPDP SCRIPT RxFill → FHIR mapping
//
// FHIR target: MedicationDispense — a pharmacy's own notification that a
// prescription was Filled/NotFilled/PartialFill'd, per
// ncpdp/schemas/script_2017071/transactions/RxFill.json (sourced from the
// SCRIPT v10.6 XSD — see that file's own sourceRefs). Base FHIR
// MedicationDispense.required is only status + medication[x]
// unconditionally (performer.actor and substitution.wasSubstituted are
// conditional on those substructures being present at all — confirmed
// directly from schemas/fhir/R4/resources/MedicationDispense.gz's own
// `required` list, not assumed), so a Patient/Organization-only Bundle
// (no separate Practitioner resource — RxFill's own schema carries no
// individual dispensing pharmacist name, only the pharmacy itself) is fully
// spec-conformant.
//
// FillStatus's 3-way choice (Filled/NotFilled/PartialFill) maps onto FHIR's
// own medicationdispense-status ValueSet: Filled -> "completed", PartialFill
// -> "in-progress" (a partial fill is, by definition, not yet complete),
// NotFilled -> "declined" — a code-system translation, never an invented
// fact; the raw outcome type itself always comes straight from the source.
//
// Config here is transcribed verbatim into a database migration — matching
// every other *_fhir_builder_test.go's own "config here is the source of
// truth" discipline.
//
// Run: go test ./services/ -v -run TestNCPDPRxFillFHIRBuilder
// ─────────────────────────────────────────────────────────────────────────────

import (
	"os"
	"testing"

	"ezhealthkonnect/models"
	"ezhealthkonnect/ncpdp"
)

const ncpdpParseRxFillStepAlias = "parse_rx_fill"

func ncpdpRxFillData(t *testing.T, fixture string) map[string]interface{} {
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
	svcInjectStepOutput(data, ncpdpParseRxFillStepAlias, normalized)
	return data
}

func ncpdpRxFillSrc(path string) string {
	return "steps." + ncpdpParseRxFillStepAlias + ".step_output.parsed_ncpdp." + path
}

func organizationRxFillBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Organization",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirPharmacyOrganization",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": ncpdpRxFillSrc("body.pharmacy.identification.ncpdpid"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "organization-pharmacy-"}},
			map[string]interface{}{"targetPath": "active", "literalValue": "true"},
			map[string]interface{}{"targetPath": "name", "sourcePath": ncpdpRxFillSrc("body.pharmacy.business_name")},
			map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/ncpdp-provider-id"},
			map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": ncpdpRxFillSrc("body.pharmacy.identification.ncpdpid")},
		},
	}
}

func patientRxFillBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Patient",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirPatient",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": ncpdpRxFillSrc("header.message_id"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "patient-"}},
			map[string]interface{}{"targetPath": "name[0].family", "sourcePath": ncpdpRxFillSrc("body.patient.human_patient.name.last_name")},
			map[string]interface{}{"targetPath": "name[0].given[0]", "sourcePath": ncpdpRxFillSrc("body.patient.human_patient.name.first_name")},
			map[string]interface{}{"targetPath": "gender", "literalValue": "male", "condition": map[string]interface{}{"field": ncpdpRxFillSrc("body.patient.human_patient.gender"), "operator": "equals", "value": "M"}},
			map[string]interface{}{"targetPath": "gender", "literalValue": "female", "condition": map[string]interface{}{"field": ncpdpRxFillSrc("body.patient.human_patient.gender"), "operator": "equals", "value": "F"}},
			map[string]interface{}{"targetPath": "gender", "literalValue": "unknown", "condition": map[string]interface{}{"field": ncpdpRxFillSrc("body.patient.human_patient.gender"), "operator": "equals", "value": "U"}},
		},
	}
}

func medicationDispenseRxFillBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "MedicationDispense",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirMedicationDispense",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": ncpdpRxFillSrc("header.message_id"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "medicationdispense-"}},
			map[string]interface{}{"targetPath": "status", "literalValue": "completed", "condition": map[string]interface{}{"field": ncpdpRxFillSrc("body.filled"), "operator": "exists"}},
			map[string]interface{}{"targetPath": "status", "literalValue": "in-progress", "condition": map[string]interface{}{"field": ncpdpRxFillSrc("body.partial_fill"), "operator": "exists"}},
			map[string]interface{}{"targetPath": "status", "literalValue": "declined", "condition": map[string]interface{}{"field": ncpdpRxFillSrc("body.not_filled"), "operator": "exists"}},
			map[string]interface{}{"targetPath": "medicationCodeableConcept.text", "sourcePath": ncpdpRxFillSrc("body.medication_dispensed.drug_description"), "fallbackPaths": []interface{}{ncpdpRxFillSrc("body.medication_prescribed.drug_description")}},
			map[string]interface{}{"targetPath": "subject.reference", "sourcePath": ncpdpRxFillSrc("header.message_id"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Patient/patient-"}},
			map[string]interface{}{"targetPath": "performer[0].actor.reference", "sourcePath": ncpdpRxFillSrc("body.pharmacy.identification.ncpdpid"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Organization/organization-pharmacy-"}},
			map[string]interface{}{"targetPath": "quantity.value", "sourcePath": ncpdpRxFillSrc("body.medication_dispensed.quantity.value"), "transform": "cda_decimal_string_to_number"},
			map[string]interface{}{"targetPath": "whenHandedOver", "sourcePath": ncpdpRxFillSrc("body.medication_dispensed.last_fill_date.date")},
			map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/ncpdp-fill-reference-number"},
			map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": ncpdpRxFillSrc("body.filled.reference_number")},
			map[string]interface{}{"targetPath": "note[0].text", "sourcePath": ncpdpRxFillSrc("body.filled.note"), "fallbackPaths": []interface{}{
				ncpdpRxFillSrc("body.partial_fill.note"),
				ncpdpRxFillSrc("body.not_filled.note"),
			}},
		},
	}
}

func TestNCPDPRxFillFHIRBuilder_FilledOutcome_BuildsCleanValidatingBundle(t *testing.T) {
	initFHIRRegistrySvc(t)
	data := ncpdpRxFillData(t, "../ncpdp/testdata/self_authored/rx_fill_filled_sample.xml")

	data = runFHIRBuild(t, "build_pharmacy_organization_fhir", organizationRxFillBuildConfig(), data)
	data = runFHIRBuild(t, "build_patient_fhir", patientRxFillBuildConfig(), data)
	data = runFHIRBuild(t, "build_medication_dispense_fhir", medicationDispenseRxFillBuildConfig(), data)

	message := data["message"].(map[string]interface{})
	medDisp := message["fhirMedicationDispense"].(map[string]interface{})
	if medDisp["status"] != "completed" {
		t.Errorf("MedicationDispense.status = %v, want \"completed\" (Filled outcome)", medDisp["status"])
	}
	if medDisp["medicationCodeableConcept"].(map[string]interface{})["text"] != "Ondansetron 8 mg Tab Disintegrating" {
		t.Errorf("MedicationDispense.medicationCodeableConcept.text = %v", medDisp["medicationCodeableConcept"])
	}
	idArr, ok := medDisp["identifier"].([]interface{})
	if !ok || len(idArr) == 0 || idArr[0].(map[string]interface{})["value"] != "REQ-0004" {
		t.Fatalf("expected MedicationDispense.identifier[0].value = REQ-0004, got %v", medDisp["identifier"])
	}
	noteArr, ok := medDisp["note"].([]interface{})
	if !ok || len(noteArr) == 0 || noteArr[0].(map[string]interface{})["text"] != "Dispensed as written" {
		t.Errorf("MedicationDispense.note[0].text = %v, want the real note text", medDisp["note"])
	}

	assembleAndValidateNCPDPBundle(t, data, "assemble_rxfill_bundle",
		[]string{"message.fhirPharmacyOrganization", "message.fhirPatient", "message.fhirMedicationDispense"})
}
