// services/executors/transform/fhir_build_bundle_assembly_test.go
//
// Chains all 10 fhir.build steps used by the real FHIR_Build_Demo pipeline
// (see database/migrations/V200__FHIR_Build_Demo_Round3_Complete_Mapping.sql —
// step configs here are kept byte-for-byte consistent with that migration)
// plus the payload.builder fhir_bundle assembly step, in-process, then
// validates the resulting Bundle against the real fhir/r4 validator. This is
// round 1's "our own tests green" bar before handing a real Bundle to an
// external FHIR validator (see project memory on the CDA precedent).
package transform

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"ezhealthkonnect/fhir/r4"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors/payload"
)

// loadFixtureMessage reads the shared FHIR_Build_Demo sample payload — the
// same file Playwright's FHB-E2-004 test and the live pipeline's Test
// Pipeline panel use, so there is exactly one source of truth for this data.
func loadFixtureMessage(t *testing.T) map[string]interface{} {
	t.Helper()
	raw, err := os.ReadFile("../../../tests/fixtures/fhir_build_demo_sample_message.json")
	if err != nil {
		t.Fatalf("failed to read fixture: %v", err)
	}
	var msg map[string]interface{}
	if err := json.Unmarshal(raw, &msg); err != nil {
		t.Fatalf("failed to parse fixture JSON: %v", err)
	}
	return msg
}

// prefixRef builds a string_prefix field row targeting a *.reference path —
// a small local helper to keep the 10 step configs below readable.
func prefixRef(targetPath, sourcePath, prefix string) map[string]interface{} {
	return map[string]interface{}{
		"targetPath": targetPath, "sourcePath": sourcePath,
		"transform": "string_prefix", "valueMap": map[string]interface{}{"prefix": prefix},
	}
}

func lit(targetPath, value string) map[string]interface{} {
	return map[string]interface{}{"targetPath": targetPath, "literalValue": value}
}

func src(targetPath, sourcePath string) map[string]interface{} {
	return map[string]interface{}{"targetPath": targetPath, "sourcePath": sourcePath}
}

// fhirBuildDemoStepConfigs returns the 10 fhir.build step configs, in the
// order they run in the real pipeline. Keep in sync with V200's UPDATEs.
//
// Round 3 (cross-resource reference wiring): Practitioner/Organization/Location
// gained their own "id" fields (sourced from the same natural identifier
// already used elsewhere — practitionerNPI/orgNPI/a dedicated locationId —
// exactly like Patient's own id already reused patientIdentifiers[0].idValue)
// specifically so Encounter.participant/.serviceProvider/.location,
// Condition/Observation/MedicationRequest/AllergyIntolerance/Immunization's
// recorder/performer/requester, and Location.managingOrganization can all
// reference them by a resolvable "ResourceType/id" — before this round those
// three resources were built but never referenced by anything else in the
// Bundle.
func fhirBuildDemoStepConfigs() []map[string]interface{} {
	return []map[string]interface{}{
		{ // Patient
			"resourceType": "Patient", "outputField": "message.fhirPatient",
			"fields": []interface{}{
				src("id", "message.patientIdentifiers[0].idValue"),
				src("birthDate", "message.dob"),
				map[string]interface{}{"targetPath": "gender", "sourcePath": "message.sexCode", "transform": "string_direct", "valueMap": map[string]interface{}{"F": "female", "M": "male"}},
				src("name[0].family", "message.patientFamily"),
				src("name[0].given[0]", "message.patientGiven"),
				lit("telecom[0].system", "phone"),
				src("telecom[0].value", "message.patientPhone"),
				src("address[0].line[0]", "message.patientAddressLine"),
				src("address[0].city", "message.patientAddressCity"),
				src("address[0].state", "message.patientAddressState"),
				src("address[0].postalCode", "message.patientAddressPostalCode"),
			},
			"repeatingGroups": []interface{}{
				map[string]interface{}{
					"targetPath": "identifier", "rowsPath": "message.patientIdentifiers",
					"fields": []interface{}{src("system", "idSystem"), src("value", "idValue")},
				},
			},
		},
		{ // Encounter
			"resourceType": "Encounter", "outputField": "message.fhirEncounter",
			"fields": []interface{}{
				src("id", "message.encounterId"),
				lit("status", "finished"),
				lit("class.system", "http://terminology.hl7.org/CodeSystem/v3-ActCode"),
				lit("class.code", "AMB"),
				prefixRef("subject.reference", "message.patientIdentifiers[0].idValue", "Patient/"),
				src("period.start", "message.encounterPeriodStart"),
				src("period.end", "message.encounterPeriodEnd"),
				src("reasonCode[0].coding[0].system", "message.conditionCodeSystem"),
				src("reasonCode[0].coding[0].code", "message.conditionCode"),
				prefixRef("participant[0].individual.reference", "message.practitionerNPI", "Practitioner/"),
				prefixRef("serviceProvider.reference", "message.orgNPI", "Organization/"),
				prefixRef("location[0].location.reference", "message.locationId", "Location/"),
			},
		},
		{ // Condition
			"resourceType": "Condition", "outputField": "message.fhirCondition",
			"fields": []interface{}{
				lit("clinicalStatus.coding[0].system", "http://terminology.hl7.org/CodeSystem/condition-clinical"),
				lit("clinicalStatus.coding[0].code", "active"),
				src("code.coding[0].system", "message.conditionCodeSystem"),
				src("code.coding[0].code", "message.conditionCode"),
				prefixRef("subject.reference", "message.patientIdentifiers[0].idValue", "Patient/"),
				prefixRef("recorder.reference", "message.practitionerNPI", "Practitioner/"),
				lit("category[0].coding[0].system", "http://terminology.hl7.org/CodeSystem/condition-category"),
				lit("category[0].coding[0].code", "problem-list-item"),
				lit("verificationStatus.coding[0].system", "http://terminology.hl7.org/CodeSystem/condition-ver-status"),
				lit("verificationStatus.coding[0].code", "confirmed"),
			},
		},
		{ // Observation
			"resourceType": "Observation", "outputField": "message.fhirObservation",
			"fields": []interface{}{
				lit("status", "final"),
				lit("code.coding[0].system", "http://loinc.org"),
				src("code.coding[0].code", "message.observationCode"),
				// category + effectiveDateTime + valueQuantity.system/code:
				// round-2 additions — LOINC 8310-5 (Body Temperature) auto-
				// selects FHIR's built-in "bodytemp" vital-signs profile,
				// which requires all four (confirmed via a real external
				// validator run against round 1's output).
				lit("category[0].coding[0].system", "http://terminology.hl7.org/CodeSystem/observation-category"),
				lit("category[0].coding[0].code", "vital-signs"),
				lit("category[0].coding[0].display", "Vital Signs"),
				src("effectiveDateTime", "message.observationDate"),
				map[string]interface{}{"targetPath": "valueQuantity.value", "sourcePath": "message.observationValue", "transform": "cda_decimal_string_to_number"},
				src("valueQuantity.unit", "message.observationUnit"),
				lit("valueQuantity.system", "http://unitsofmeasure.org"),
				src("valueQuantity.code", "message.observationUnit"),
				prefixRef("subject.reference", "message.patientIdentifiers[0].idValue", "Patient/"),
				prefixRef("encounter.reference", "message.encounterId", "Encounter/"),
				prefixRef("performer[0].reference", "message.practitionerNPI", "Practitioner/"),
				lit("interpretation[0].coding[0].system", "http://terminology.hl7.org/CodeSystem/v3-ObservationInterpretation"),
				lit("interpretation[0].coding[0].code", "N"),
			},
		},
		{ // MedicationRequest
			"resourceType": "MedicationRequest", "outputField": "message.fhirMedicationRequest",
			"fields": []interface{}{
				lit("status", "active"),
				lit("intent", "order"),
				lit("medicationCodeableConcept.coding[0].system", "http://www.nlm.nih.gov/research/umls/rxnorm"),
				src("medicationCodeableConcept.coding[0].code", "message.drugCode"),
				prefixRef("subject.reference", "message.patientIdentifiers[0].idValue", "Patient/"),
				prefixRef("requester.reference", "message.practitionerNPI", "Practitioner/"),
				src("authoredOn", "message.encounterPeriodStart"),
				lit("dosageInstruction[0].text", "Take 1 tablet by mouth once daily"),
			},
		},
		{ // AllergyIntolerance
			"resourceType": "AllergyIntolerance", "outputField": "message.fhirAllergyIntolerance",
			"fields": []interface{}{
				lit("clinicalStatus.coding[0].system", "http://terminology.hl7.org/CodeSystem/allergyintolerance-clinical"),
				lit("clinicalStatus.coding[0].code", "active"),
				lit("code.coding[0].system", "http://snomed.info/sct"),
				src("code.coding[0].code", "message.allergenCode"),
				prefixRef("patient.reference", "message.patientIdentifiers[0].idValue", "Patient/"),
				prefixRef("recorder.reference", "message.practitionerNPI", "Practitioner/"),
				lit("type", "allergy"),
				lit("category[0]", "medication"),
				lit("criticality", "low"),
			},
		},
		{ // Immunization
			"resourceType": "Immunization", "outputField": "message.fhirImmunization",
			"fields": []interface{}{
				lit("status", "completed"),
				lit("vaccineCode.coding[0].system", "http://hl7.org/fhir/sid/cvx"),
				src("vaccineCode.coding[0].code", "message.vaccineCode"),
				src("occurrenceDateTime", "message.immunizationDate"),
				prefixRef("patient.reference", "message.patientIdentifiers[0].idValue", "Patient/"),
				prefixRef("performer[0].actor.reference", "message.practitionerNPI", "Practitioner/"),
				prefixRef("location.reference", "message.locationId", "Location/"),
				lit("lotNumber", "LOT12345"),
			},
		},
		{ // Practitioner
			"resourceType": "Practitioner", "outputField": "message.fhirPractitioner",
			"fields": []interface{}{
				src("id", "message.practitionerNPI"),
				lit("identifier[0].system", "http://hl7.org/fhir/sid/us-npi"),
				src("identifier[0].value", "message.practitionerNPI"),
				src("name[0].family", "message.practitionerFamily"),
				src("name[0].given[0]", "message.practitionerGiven"),
				src("gender", "message.practitionerGender"),
				lit("telecom[0].system", "phone"),
				src("telecom[0].value", "message.practitionerPhone"),
			},
		},
		{ // Organization
			"resourceType": "Organization", "outputField": "message.fhirOrganization",
			"fields": []interface{}{
				src("id", "message.orgNPI"),
				lit("identifier[0].system", "http://hl7.org/fhir/sid/us-npi"),
				src("identifier[0].value", "message.orgNPI"),
				src("name", "message.orgName"),
				lit("type[0].coding[0].system", "http://terminology.hl7.org/CodeSystem/organization-type"),
				lit("type[0].coding[0].code", "prov"),
				lit("telecom[0].system", "phone"),
				src("telecom[0].value", "message.orgPhone"),
			},
		},
		{ // Location
			"resourceType": "Location", "outputField": "message.fhirLocation",
			"fields": []interface{}{
				src("id", "message.locationId"),
				lit("status", "active"),
				src("name", "message.locationName"),
				prefixRef("managingOrganization.reference", "message.orgNPI", "Organization/"),
				lit("physicalType.coding[0].system", "http://terminology.hl7.org/CodeSystem/location-physical-type"),
				lit("physicalType.coding[0].code", "ro"),
			},
		},
	}
}

// TestFHIRBuildDemo_FullPipeline_AssemblesValidBundle chains all 10 fhir.build
// steps + the bundle assembly step and validates the result, asserting zero
// errors and zero unresolved-reference warnings.
func TestFHIRBuildDemo_FullPipeline_AssemblesValidBundle(t *testing.T) {
	initFHIRRegistry(t)

	data := map[string]interface{}{"message": loadFixtureMessage(t)}

	buildExecutor := NewFHIRBuildExecutor()
	for _, cfg := range fhirBuildDemoStepConfigs() {
		step := &models.TransformationStep{
			StepName: cfg["resourceType"].(string), StepType: "fhir.build", Enabled: true, Config: cfg,
		}
		output, err := buildExecutor.Execute(context.Background(), step, data)
		if err != nil {
			t.Fatalf("fhir.build failed for %s: %v", cfg["resourceType"], err)
		}
		data = output
	}

	bundleStep := &models.TransformationStep{
		StepName: "Assemble Bundle", StepType: "payload.builder", Enabled: true,
		Config: map[string]interface{}{
			"mode": "fhir_bundle",
			"fhirBundle": map[string]interface{}{
				"bundleType": "collection",
				"resourcePaths": []interface{}{
					"message.fhirPatient", "message.fhirEncounter", "message.fhirCondition",
					"message.fhirObservation", "message.fhirMedicationRequest", "message.fhirAllergyIntolerance",
					"message.fhirImmunization", "message.fhirPractitioner", "message.fhirOrganization", "message.fhirLocation",
				},
			},
		},
	}
	payloadExecutor := payload.NewPayloadBuilderExecutor()
	output, err := payloadExecutor.Execute(context.Background(), bundleStep, data)
	if err != nil {
		t.Fatalf("payload.builder (fhir_bundle) failed: %v", err)
	}

	bundleJSON, ok := output["payload"].(string)
	if !ok || bundleJSON == "" {
		t.Fatalf("expected output[\"payload\"] to be a non-empty Bundle JSON string, got %v", output["payload"])
	}
	var bundle map[string]interface{}
	if err := json.Unmarshal([]byte(bundleJSON), &bundle); err != nil {
		t.Fatalf("failed to unmarshal assembled Bundle: %v", err)
	}

	entries, ok := bundle["entry"].([]interface{})
	if !ok || len(entries) != 10 {
		t.Fatalf("expected 10 Bundle entries, got %v", bundle["entry"])
	}

	// Round 3: fullUrl/reference assembly now delegates to
	// fhir/r4.AssembleEntries (the same already-validated logic
	// services/cda_fhir's Bundle assembly uses) instead of a second, parallel
	// implementation — every entry gets a fresh urn:uuid: fullUrl (never the
	// "{base}/ResourceType/id" form round 2 used), and every
	// "ResourceType/id" reference anywhere in any resource is rewritten to
	// match. fullUrls are therefore random per run: assert format + uniqueness
	// (bdl-7) rather than exact values, and prove the rewrite actually
	// happened by checking specific cross-references resolve to the exact
	// urn:uuid: of the resource they point to.
	seenFullURLs := make(map[string]bool, len(entries))
	resourceByType := make(map[string]map[string]interface{}, len(entries))
	fullURLByType := make(map[string]string, len(entries))
	for i, e := range entries {
		entry := e.(map[string]interface{})
		resource := entry["resource"].(map[string]interface{})
		rt := resource["resourceType"].(string)
		fullURL, _ := entry["fullUrl"].(string)
		if fullURL == "" || !strings.HasPrefix(fullURL, "urn:uuid:") {
			t.Errorf("entry[%d] (%s) fullUrl = %q, want a non-empty urn:uuid: value", i, rt, fullURL)
		}
		if seenFullURLs[fullURL] {
			t.Errorf("entry[%d] (%s) fullUrl %q is a duplicate — violates bdl-7 (fullUrl must be unique)", i, rt, fullURL)
		}
		seenFullURLs[fullURL] = true
		resourceByType[rt] = resource
		fullURLByType[rt] = fullURL
	}

	assertRefResolvesTo := func(refHolder map[string]interface{}, refKey, targetType string) {
		ref, _ := refHolder["reference"].(string)
		want := fullURLByType[targetType]
		if ref != want {
			t.Errorf("%s reference = %q, want it rewritten to the %s entry's fullUrl %q", refKey, ref, targetType, want)
		}
	}

	condition := resourceByType["Condition"]
	assertRefResolvesTo(condition["subject"].(map[string]interface{}), "Condition.subject", "Patient")
	encounter := resourceByType["Encounter"]
	assertRefResolvesTo(encounter["subject"].(map[string]interface{}), "Encounter.subject", "Patient")
	assertRefResolvesTo(encounter["serviceProvider"].(map[string]interface{}), "Encounter.serviceProvider", "Organization")

	validator := r4.GetValidator()
	result := validator.ValidateBundle(bundle, r4.ValidationOptions{
		Version: "R4", Profile: "base", Level: r4.LevelStandard, ValidateRefs: true,
	})

	if !result.Valid {
		for _, iss := range result.BundleIssues {
			t.Logf("Bundle issue [%s] %s: %s", iss.Severity, iss.Code, iss.Message)
		}
		for _, rr := range result.Resources {
			for _, e := range rr.Errors {
				t.Logf("%s error [%s] %s: %s", rr.ResourceType, e.Code, e.Path, e.Message)
			}
		}
		t.Errorf("expected Bundle to validate cleanly (0 errors), got ErrorCount=%d", result.ErrorCount)
	}

	// Every cross-resource reference this round wires (Patient/*, Encounter/*)
	// must resolve within the Bundle — an unresolved-reference warning here
	// means the reference-wiring convention (same raw id field for both the
	// referenced resource's own id and every reference to it) has drifted.
	for _, rr := range result.Resources {
		for _, w := range rr.Warnings {
			if w.Code == "unresolved-reference" {
				t.Errorf("%s has an unresolved reference: %s", rr.ResourceType, w.Message)
			}
		}
	}
}
