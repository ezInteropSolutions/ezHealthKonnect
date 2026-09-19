package services

// ─────────────────────────────────────────────────────────────────────────────
// NCPDP SCRIPT NewRx → FHIR mapping (Phase 1)
//
// Unlike every EDI X12 → FHIR mapping in this codebase, NewRx needs NO
// derive script and NO fhir.build rowsPath: a NewRx message carries exactly
// one patient, one prescriber, one pharmacy, and one medication — there is
// no repeating claim/service-line structure to flatten into row contexts.
// fhir.build's own doc comment confirms this is the intended shape:
// "rowsPath — optional; when set, builds ONE resource PER ROW... instead of
// one resource from inputData directly" — so every field below sources
// straight from steps.parse_new_rx.step_output.parsedNCPDP.{header,body}
// paths. Adding unnecessary row-flattening machinery here would be exactly
// the kind of premature complexity this project's own standards call out.
//
// FHIR targets: Organization (pharmacy), Patient, Practitioner (prescriber),
// MedicationRequest — the same core resource set HL7's own NCPDP SCRIPT <->
// FHIR MedicationRequest mapping guidance names.
//
// Named simplification, not silently omitted: NewRx (as scoped in this
// phase — see ncpdp/schemas/script_2017071's own group definitions) carries
// no patient-identifier field (no MRN/insurance-card-ID segment was
// modeled — that lives in NCPDP's own COO/insurance section, deliberately
// deferred). Patient.id and every cross-resource reference below are
// therefore derived from the message's own Header.messageID — stable within
// one message, but NOT a real patient identifier and NOT deduplicated
// across multiple NewRx messages for the same real-world patient, the same
// "not deduplicated across claims" precedent already established for EDI
// 837P/837I's own Patient/Coverage mapping.
//
// Config here is transcribed verbatim into a database migration — matching
// every other *_fhir_builder_test.go's own "config here is the source of
// truth" discipline.
//
// Run: go test ./services/ -v -run TestNCPDPNewRxFHIRBuilder
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

const ncpdpParseNewRxStepAlias = "parse_new_rx"
const testNCPDPSchemaDirSvc = "../ncpdp/schemas/script_2017071"

// ncpdpRealNewRxData parses the real, unedited SCRIPT v2017071 NewRx sample
// (same fixture ncpdp/newrx_roundtrip_test.go uses) and injects it as
// parse_new_rx's own step_output, exactly the shape a real ncpdp.parse step
// leaves behind for a later fhir.build step's sourcePath to read.
//
// Critical: the injected map is passed through the REAL
// models.OutputNormalizer.NormalizeStepOutput — the exact same call
// transformation_pipeline_helpers.go's executeStepWithContext applies to
// every step's own step_output snapshot before a later step's sourcePath can
// see it. A first version of this test skipped that call (mirroring
// svcInjectStepOutput's own plain pass-through, which is only safe for a
// script step whose RETURNED keys are already deliberately snake_case) and
// passed cleanly — but a real Test Pipeline run against the live app found
// EVERY sourcePath-driven field on every fhir.build step resolving empty,
// because NormalizeStepOutput recursively snake-cases every key ncpdp's own
// (deliberately camelCase, matching its own schema field.Key convention)
// ParsedJSON carries — "parsedNCPDP" becomes "parsed_ncpdp",
// "medicationPrescribed" becomes "medication_prescribed", and so on, all the
// way down. Calling the real normalizer here — instead of hand-transcribing
// the snake_case shape a second time — is what makes this test actually
// prove the real engine's behavior, not just its own internal consistency.
func ncpdpRealNewRxData(t *testing.T) map[string]interface{} {
	t.Helper()
	loader, err := ncpdp.NewNCPDPSchemaLoader(testNCPDPSchemaDirSvc)
	if err != nil {
		t.Fatalf("failed to load NCPDP schema: %v", err)
	}
	raw, err := os.ReadFile("../ncpdp/testdata/real_samples/dgoradia_sample_newrx.xml")
	if err != nil {
		t.Fatalf("failed to read real NewRx sample: %v", err)
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
	svcInjectStepOutput(data, ncpdpParseNewRxStepAlias, normalized)
	return data
}

// ncpdpSrc builds a sourcePath addressing parse_new_rx's own NORMALIZED
// (snake_case) step_output — path must already be given in snake_case,
// matching what NormalizeStepOutput actually produces (see
// ncpdpRealNewRxData's own doc comment for why this isn't the schema's own
// camelCase field.Key convention).
func ncpdpSrc(path string) string {
	return "steps." + ncpdpParseNewRxStepAlias + ".step_output.parsed_ncpdp." + path
}

func organizationNewRxBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Organization",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirPharmacyOrganization",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": ncpdpSrc("body.pharmacy.identification.ncpdpid"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "organization-pharmacy-"}},
			map[string]interface{}{"targetPath": "active", "literalValue": "true"},
			map[string]interface{}{"targetPath": "name", "sourcePath": ncpdpSrc("body.pharmacy.business_name")},
			map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://ezhealthkonnect.local/ncpdp-provider-id"},
			map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": ncpdpSrc("body.pharmacy.identification.ncpdpid")},
			map[string]interface{}{"targetPath": "identifier[1].system", "literalValue": "http://hl7.org/fhir/sid/us-npi"},
			map[string]interface{}{"targetPath": "identifier[1].value", "sourcePath": ncpdpSrc("body.pharmacy.identification.npi")},
			map[string]interface{}{"targetPath": "address[0].line[0]", "sourcePath": ncpdpSrc("body.pharmacy.address.address_line1")},
			map[string]interface{}{"targetPath": "address[0].city", "sourcePath": ncpdpSrc("body.pharmacy.address.city")},
			map[string]interface{}{"targetPath": "address[0].state", "sourcePath": ncpdpSrc("body.pharmacy.address.state_province")},
			map[string]interface{}{"targetPath": "address[0].postalCode", "sourcePath": ncpdpSrc("body.pharmacy.address.postal_code")},
			map[string]interface{}{"targetPath": "telecom[0].system", "literalValue": "phone"},
			map[string]interface{}{"targetPath": "telecom[0].value", "sourcePath": ncpdpSrc("body.pharmacy.communication_numbers.primary_telephone.number")},
		},
	}
}

func patientNewRxBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Patient",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirPatient",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": ncpdpSrc("header.message_id"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "patient-"}},
			map[string]interface{}{"targetPath": "name[0].family", "sourcePath": ncpdpSrc("body.patient.human_patient.name.last_name")},
			map[string]interface{}{"targetPath": "name[0].given[0]", "sourcePath": ncpdpSrc("body.patient.human_patient.name.first_name")},
			// cda_gender_to_fhir (services/cda_fhir's DeclarativeTransformRegistry)
			// expects a CDA-shaped {"code": "..."} object, not NCPDP's own bare
			// "M"/"F"/"U" string — remarshalInto would silently zero it out and
			// the transform would skip the write. Three conditional literals
			// map NCPDP's own gender vocabulary directly instead, the same
			// condition-gated-literal pattern EDI 837I's own onAdmission field uses.
			map[string]interface{}{"targetPath": "gender", "literalValue": "male", "condition": map[string]interface{}{"field": ncpdpSrc("body.patient.human_patient.gender"), "operator": "equals", "value": "M"}},
			map[string]interface{}{"targetPath": "gender", "literalValue": "female", "condition": map[string]interface{}{"field": ncpdpSrc("body.patient.human_patient.gender"), "operator": "equals", "value": "F"}},
			map[string]interface{}{"targetPath": "gender", "literalValue": "unknown", "condition": map[string]interface{}{"field": ncpdpSrc("body.patient.human_patient.gender"), "operator": "equals", "value": "U"}},
			map[string]interface{}{"targetPath": "birthDate", "sourcePath": ncpdpSrc("body.patient.human_patient.date_of_birth.date")},
			map[string]interface{}{"targetPath": "address[0].line[0]", "sourcePath": ncpdpSrc("body.patient.human_patient.address.address_line1")},
			map[string]interface{}{"targetPath": "address[0].city", "sourcePath": ncpdpSrc("body.patient.human_patient.address.city")},
			map[string]interface{}{"targetPath": "address[0].state", "sourcePath": ncpdpSrc("body.patient.human_patient.address.state_province")},
			map[string]interface{}{"targetPath": "address[0].postalCode", "sourcePath": ncpdpSrc("body.patient.human_patient.address.postal_code")},
			map[string]interface{}{"targetPath": "telecom[0].system", "literalValue": "phone"},
			map[string]interface{}{"targetPath": "telecom[0].value", "sourcePath": ncpdpSrc("body.patient.human_patient.communication_numbers.primary_telephone.number")},
		},
	}
}

func practitionerNewRxBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Practitioner",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirPractitioner",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": ncpdpSrc("body.prescriber.non_veterinarian.identification.npi"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "practitioner-"}},
			map[string]interface{}{"targetPath": "identifier[0].system", "literalValue": "http://hl7.org/fhir/sid/us-npi"},
			map[string]interface{}{"targetPath": "identifier[0].value", "sourcePath": ncpdpSrc("body.prescriber.non_veterinarian.identification.npi")},
			map[string]interface{}{"targetPath": "identifier[1].system", "literalValue": "http://ezhealthkonnect.local/dea-number"},
			map[string]interface{}{"targetPath": "identifier[1].value", "sourcePath": ncpdpSrc("body.prescriber.non_veterinarian.identification.dea_number")},
			map[string]interface{}{"targetPath": "name[0].family", "sourcePath": ncpdpSrc("body.prescriber.non_veterinarian.name.last_name")},
			map[string]interface{}{"targetPath": "name[0].given[0]", "sourcePath": ncpdpSrc("body.prescriber.non_veterinarian.name.first_name")},
			map[string]interface{}{"targetPath": "name[0].suffix[0]", "sourcePath": ncpdpSrc("body.prescriber.non_veterinarian.name.suffix")},
			map[string]interface{}{"targetPath": "address[0].line[0]", "sourcePath": ncpdpSrc("body.prescriber.non_veterinarian.address.address_line1")},
			map[string]interface{}{"targetPath": "address[0].city", "sourcePath": ncpdpSrc("body.prescriber.non_veterinarian.address.city")},
			map[string]interface{}{"targetPath": "address[0].state", "sourcePath": ncpdpSrc("body.prescriber.non_veterinarian.address.state_province")},
			map[string]interface{}{"targetPath": "address[0].postalCode", "sourcePath": ncpdpSrc("body.prescriber.non_veterinarian.address.postal_code")},
			map[string]interface{}{"targetPath": "telecom[0].system", "literalValue": "phone"},
			map[string]interface{}{"targetPath": "telecom[0].value", "sourcePath": ncpdpSrc("body.prescriber.non_veterinarian.communication_numbers.primary_telephone.number")},
		},
	}
}

func medicationRequestNewRxBuildConfig() map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "MedicationRequest",
		"profile":      "base",
		"version":      "R4",
		"outputField":  "message.fhirMedicationRequest",
		"fields": []interface{}{
			map[string]interface{}{"targetPath": "id", "sourcePath": ncpdpSrc("header.message_id"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "medicationrequest-"}},
			map[string]interface{}{"targetPath": "status", "literalValue": "active"},
			map[string]interface{}{"targetPath": "intent", "literalValue": "order"},
			map[string]interface{}{"targetPath": "medicationCodeableConcept.text", "sourcePath": ncpdpSrc("body.medication_prescribed.drug_description")},
			map[string]interface{}{"targetPath": "medicationCodeableConcept.coding[0].system", "literalValue": "http://hl7.org/fhir/sid/ndc"},
			map[string]interface{}{"targetPath": "medicationCodeableConcept.coding[0].code", "sourcePath": ncpdpSrc("body.medication_prescribed.drug_coded.product_code.code")},
			map[string]interface{}{"targetPath": "subject.reference", "sourcePath": ncpdpSrc("header.message_id"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Patient/patient-"}},
			map[string]interface{}{"targetPath": "requester.reference", "sourcePath": ncpdpSrc("body.prescriber.non_veterinarian.identification.npi"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Practitioner/practitioner-"}},
			map[string]interface{}{"targetPath": "authoredOn", "sourcePath": ncpdpSrc("body.medication_prescribed.written_date.date")},
			map[string]interface{}{"targetPath": "dosageInstruction[0].text", "sourcePath": ncpdpSrc("body.medication_prescribed.sig.sig_text")},
			map[string]interface{}{"targetPath": "dispenseRequest.quantity.value", "sourcePath": ncpdpSrc("body.medication_prescribed.quantity.value"), "transform": "cda_decimal_string_to_number"},
			map[string]interface{}{"targetPath": "dispenseRequest.numberOfRepeatsAllowed", "sourcePath": ncpdpSrc("body.medication_prescribed.number_of_refills"), "transform": "cda_decimal_string_to_number"},
			map[string]interface{}{"targetPath": "dispenseRequest.performer.reference", "sourcePath": ncpdpSrc("body.pharmacy.identification.ncpdpid"), "transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": "Organization/organization-pharmacy-"}},
		},
	}
}

// TC-NCPDP-NewRx-FB-001: full chain (Organization -> Patient -> Practitioner
// -> MedicationRequest -> payload.builder fhir_bundle -> fhir_validation
// strict) against the real, unedited NewRx sample, asserting real field
// values and zero unexpected validation errors.
func TestNCPDPNewRxFHIRBuilder_RealSample_BuildsCleanValidatingBundle(t *testing.T) {
	initFHIRRegistrySvc(t)
	data := ncpdpRealNewRxData(t)

	data = runFHIRBuild(t, "build_pharmacy_organization_fhir", organizationNewRxBuildConfig(), data)
	data = runFHIRBuild(t, "build_patient_fhir", patientNewRxBuildConfig(), data)
	data = runFHIRBuild(t, "build_practitioner_fhir", practitionerNewRxBuildConfig(), data)
	data = runFHIRBuild(t, "build_medication_request_fhir", medicationRequestNewRxBuildConfig(), data)

	message, ok := data["message"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected data[\"message\"] to be a map, got %T", data["message"])
	}

	org, ok := message["fhirPharmacyOrganization"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected fhirPharmacyOrganization to be a map, got %T", message["fhirPharmacyOrganization"])
	}
	if org["name"] != "A+ Drugs" {
		t.Errorf("Organization.name = %v, want \"A+ Drugs\"", org["name"])
	}

	patient, ok := message["fhirPatient"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected fhirPatient to be a map, got %T", message["fhirPatient"])
	}
	patientName := patient["name"].([]interface{})[0].(map[string]interface{})
	if patientName["family"] != "Jenny" {
		t.Errorf("Patient.name[0].family = %v, want \"Jenny\"", patientName["family"])
	}
	if patient["gender"] != "female" {
		t.Errorf("Patient.gender = %v, want \"female\"", patient["gender"])
	}
	if patient["birthDate"] != "1984-09-09" {
		t.Errorf("Patient.birthDate = %v, want \"1984-09-09\"", patient["birthDate"])
	}

	practitioner, ok := message["fhirPractitioner"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected fhirPractitioner to be a map, got %T", message["fhirPractitioner"])
	}
	practName := practitioner["name"].([]interface{})[0].(map[string]interface{})
	if practName["family"] != "Bless" {
		t.Errorf("Practitioner.name[0].family = %v, want \"Bless\"", practName["family"])
	}

	medReq, ok := message["fhirMedicationRequest"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected fhirMedicationRequest to be a map, got %T", message["fhirMedicationRequest"])
	}
	if medReq["status"] != "active" || medReq["intent"] != "order" {
		t.Errorf("MedicationRequest status/intent = %v/%v, want active/order", medReq["status"], medReq["intent"])
	}
	medCC := medReq["medicationCodeableConcept"].(map[string]interface{})
	if medCC["text"] != "Ondansetron 8 mg Tab Disintegrating" {
		t.Errorf("MedicationRequest.medicationCodeableConcept.text = %v, want the real drug description", medCC["text"])
	}
	subjectRef := medReq["subject"].(map[string]interface{})["reference"]
	patientRef := "Patient/" + patient["id"].(string)
	if subjectRef != patientRef {
		t.Errorf("MedicationRequest.subject.reference = %v, want %v (matching the real Patient resource's own id)", subjectRef, patientRef)
	}
	dispenseQty := medReq["dispenseRequest"].(map[string]interface{})["quantity"].(map[string]interface{})["value"]
	if numToFloat(t, dispenseQty) != 15 {
		t.Errorf("MedicationRequest.dispenseRequest.quantity.value = %v, want 15", dispenseQty)
	}

	assembleAndValidateNewRxBundle(t, data, "message.fhirPharmacyOrganization", "message.fhirPatient", "message.fhirPractitioner", "message.fhirMedicationRequest")
}

func numToFloat(t *testing.T, v interface{}) float64 {
	t.Helper()
	switch n := v.(type) {
	case float64:
		return n
	case int:
		return float64(n)
	default:
		t.Fatalf("expected a numeric value, got %T (%v)", v, v)
		return 0
	}
}

// assembleAndValidateNewRxBundle mirrors assembleAndValidate276Bundle exactly
// — payload.builder fhir_bundle -> fhir_validation strict, asserting zero
// unexpected validation errors.
func assembleAndValidateNewRxBundle(t *testing.T, data map[string]interface{}, resourcePaths ...string) string {
	t.Helper()

	rp := make([]interface{}, len(resourcePaths))
	for i, p := range resourcePaths {
		rp[i] = p
	}

	pbExec := payload.NewPayloadBuilderExecutor(nil)
	pbStep := &models.TransformationStep{
		StepName:  "Assemble FHIR Bundle",
		StepAlias: strPtrSvc("assemble_newrx_bundle"),
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
	pbOut := svcStepOutput(t, pbResult, "assemble_newrx_bundle")
	for k, v := range pbResult {
		data[k] = v
	}
	svcInjectStepOutput(data, "assemble_newrx_bundle", pbOut)

	bundleJSON, ok := pbOut["payload"].(string)
	if !ok {
		t.Fatalf("payload.builder did not produce a payload string: %+v", pbOut)
	}
	var bundle map[string]interface{}
	if err := json.Unmarshal([]byte(bundleJSON), &bundle); err != nil {
		t.Fatalf("failed to unmarshal bundle JSON: %v", err)
	}

	vExec := validation.NewFHIRValidationExecutor()
	vStep := &models.TransformationStep{
		StepName:  "Validate FHIR Bundle",
		StepAlias: strPtrSvc("validate_newrx_bundle"),
		StepType:  "fhir_validation",
		Enabled:   true,
		Config: map[string]interface{}{
			"profile":          "base",
			"validation_level": "strict",
			"fhir_version":     "R4",
			"source_field":     "steps.assemble_newrx_bundle.step_output.payload",
		},
	}
	vResult, err := vExec.Execute(context.Background(), vStep, data)
	if err != nil {
		t.Fatalf("validate_newrx_bundle error: %v", err)
	}
	vOut := svcStepOutput(t, vResult, "validate_newrx_bundle")

	errs := asStringSlice(vOut["errors"])
	if len(errs) > 0 {
		b, _ := json.MarshalIndent(bundle, "", "  ")
		t.Errorf("unexpected validation errors:\n%s\nbundle: %s", strings.Join(errs, "\n"), b)
	}
	return bundleJSON
}
