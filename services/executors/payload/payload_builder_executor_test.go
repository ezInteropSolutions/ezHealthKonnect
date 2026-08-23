// services/executors/payload/payload_builder_executor_test.go
package payload

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"ezhealthkonnect/models"
)

func runPayloadBuilder(t *testing.T, config map[string]interface{}, inputData map[string]interface{}) map[string]interface{} {
	t.Helper()
	executor := NewPayloadBuilderExecutor(nil)
	step := &models.TransformationStep{
		StepName: "Test Payload Builder",
		StepType: "payload.builder",
		Enabled:  true,
		Config:   config,
	}
	output, err := executor.Execute(context.Background(), step, inputData)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	return output
}

func bundleFromOutput(t *testing.T, output map[string]interface{}) map[string]interface{} {
	t.Helper()
	payload, ok := output["payload"].(string)
	if !ok || payload == "" {
		t.Fatalf("expected output[\"payload\"] to be a non-empty string, got %v (%T)", output["payload"], output["payload"])
	}
	var bundle map[string]interface{}
	if err := json.Unmarshal([]byte(payload), &bundle); err != nil {
		t.Fatalf("failed to unmarshal payload as JSON: %v", err)
	}
	return bundle
}

// TestBuildFHIRBundle_CollectionType_OmitsTotal is a regression guard for the
// bun-1 constraint bug: total should only be present for searchset/history
// Bundle types. This executor only supports collection|transaction|batch|
// document (searchset/history are rejected by its own validTypes check), so
// for every bundleType it actually accepts, total must never appear.
func TestBuildFHIRBundle_CollectionType_OmitsTotal(t *testing.T) {
	config := map[string]interface{}{
		"mode": "fhir_bundle",
		"fhirBundle": map[string]interface{}{
			"bundleType":    "collection",
			"resourcePaths": []interface{}{"fhir.patient"},
		},
	}
	inputData := map[string]interface{}{
		"fhir": map[string]interface{}{
			"patient": map[string]interface{}{"resourceType": "Patient", "id": "12345"},
		},
	}

	output := runPayloadBuilder(t, config, inputData)
	bundle := bundleFromOutput(t, output)

	if _, present := bundle["total"]; present {
		t.Errorf("collection-type Bundle has a \"total\" key (%v) — bun-1 requires it be absent for non-searchset/history types", bundle["total"])
	}
}

// TestBuildFHIRBundle_TransactionType_OmitsTotal verifies the same bun-1 fix
// holds for transaction bundles (which additionally get request blocks).
func TestBuildFHIRBundle_TransactionType_OmitsTotal(t *testing.T) {
	config := map[string]interface{}{
		"mode": "fhir_bundle",
		"fhirBundle": map[string]interface{}{
			"bundleType":    "transaction",
			"resourcePaths": []interface{}{"fhir.patient"},
		},
	}
	inputData := map[string]interface{}{
		"fhir": map[string]interface{}{
			"patient": map[string]interface{}{"resourceType": "Patient", "id": "12345"},
		},
	}

	output := runPayloadBuilder(t, config, inputData)
	bundle := bundleFromOutput(t, output)

	if _, present := bundle["total"]; present {
		t.Errorf("transaction-type Bundle has a \"total\" key (%v), want absent", bundle["total"])
	}
	entries, ok := bundle["entry"].([]interface{})
	if !ok || len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %v", bundle["entry"])
	}
	entry := entries[0].(map[string]interface{})
	if _, present := entry["request"]; !present {
		t.Errorf("transaction-type entry missing request block")
	}
}

// TestBuildFHIRBundle_AssemblesEntriesInResourcePathOrder verifies multiple
// resourcePaths resolve into entries in the configured order.
func TestBuildFHIRBundle_AssemblesEntriesInResourcePathOrder(t *testing.T) {
	config := map[string]interface{}{
		"mode": "fhir_bundle",
		"fhirBundle": map[string]interface{}{
			"bundleType":    "collection",
			"resourcePaths": []interface{}{"fhir.patient", "fhir.encounter"},
		},
	}
	inputData := map[string]interface{}{
		"fhir": map[string]interface{}{
			"patient":   map[string]interface{}{"resourceType": "Patient", "id": "12345"},
			"encounter": map[string]interface{}{"resourceType": "Encounter", "id": "enc-001"},
		},
	}

	output := runPayloadBuilder(t, config, inputData)
	bundle := bundleFromOutput(t, output)

	entries, ok := bundle["entry"].([]interface{})
	if !ok || len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %v", bundle["entry"])
	}
	first := entries[0].(map[string]interface{})["resource"].(map[string]interface{})
	if first["resourceType"] != "Patient" {
		t.Errorf("entry[0].resource.resourceType = %v, want Patient (resourcePaths order)", first["resourceType"])
	}
	second := entries[1].(map[string]interface{})["resource"].(map[string]interface{})
	if second["resourceType"] != "Encounter" {
		t.Errorf("entry[1].resource.resourceType = %v, want Encounter (resourcePaths order)", second["resourceType"])
	}
}

// TestBuildFHIRBundle_EveryEntryGetsFullURL is a regression guard for a real
// external-validator finding: every Bundle entry SHALL have a fullUrl, and an
// absent one also makes any relative "ResourceType/id" reference elsewhere in
// the Bundle unresolvable per FHIR's reference-resolution rules. fullUrl
// assignment now delegates to fhir/r4.AssembleEntries (the same
// already-validated logic services/cda_fhir's Bundle assembly uses) rather
// than a second, parallel {baseUrl}/ResourceType/id implementation — every
// entry, regardless of whether its resource has its own "id", gets a
// urn:uuid: fullUrl.
func TestBuildFHIRBundle_EveryEntryGetsFullURL(t *testing.T) {
	config := map[string]interface{}{
		"mode": "fhir_bundle",
		"fhirBundle": map[string]interface{}{
			"bundleType":    "collection",
			"resourcePaths": []interface{}{"fhir.patient", "fhir.condition"},
		},
	}
	inputData := map[string]interface{}{
		"fhir": map[string]interface{}{
			"patient":   map[string]interface{}{"resourceType": "Patient", "id": "12345"},
			"condition": map[string]interface{}{"resourceType": "Condition"}, // no id
		},
	}

	output := runPayloadBuilder(t, config, inputData)
	bundle := bundleFromOutput(t, output)

	entries, ok := bundle["entry"].([]interface{})
	if !ok || len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %v", bundle["entry"])
	}
	patientEntry := entries[0].(map[string]interface{})
	if got, _ := patientEntry["fullUrl"].(string); !strings.HasPrefix(got, "urn:uuid:") {
		t.Errorf("Patient entry fullUrl = %v, want urn:uuid: prefix", got)
	}
	conditionEntry := entries[1].(map[string]interface{})
	if got, _ := conditionEntry["fullUrl"].(string); !strings.HasPrefix(got, "urn:uuid:") {
		t.Errorf("Condition (no id) entry fullUrl = %v, want urn:uuid: prefix", got)
	}
}

// TestBuildFHIRBundle_RewritesReferencesToMatchingFullURL proves
// AssembleEntries' reference-rewriting is actually wired up through
// Execute(): a relative "ResourceType/id" reference anywhere in a resource
// must end up equal to the referenced resource's own (urn:uuid:) fullUrl,
// not left as the original relative string — this is what makes cross-entry
// references in a freshly-assembled Bundle (not yet persisted to any FHIR
// server, so no real {base}/ResourceType/id exists yet) actually resolvable.
func TestBuildFHIRBundle_RewritesReferencesToMatchingFullURL(t *testing.T) {
	config := map[string]interface{}{
		"mode": "fhir_bundle",
		"fhirBundle": map[string]interface{}{
			"bundleType":    "collection",
			"resourcePaths": []interface{}{"fhir.patient", "fhir.condition"},
		},
	}
	inputData := map[string]interface{}{
		"fhir": map[string]interface{}{
			"patient":   map[string]interface{}{"resourceType": "Patient", "id": "12345"},
			"condition": map[string]interface{}{"resourceType": "Condition", "subject": map[string]interface{}{"reference": "Patient/12345"}},
		},
	}

	output := runPayloadBuilder(t, config, inputData)
	bundle := bundleFromOutput(t, output)

	entries := bundle["entry"].([]interface{})
	patientFullURL, _ := entries[0].(map[string]interface{})["fullUrl"].(string)
	condition := entries[1].(map[string]interface{})["resource"].(map[string]interface{})
	subjectRef, _ := condition["subject"].(map[string]interface{})["reference"].(string)

	if subjectRef != patientFullURL {
		t.Errorf("Condition.subject.reference = %q, want it rewritten to Patient's fullUrl %q", subjectRef, patientFullURL)
	}
}

// TestBuildFHIRBundle_GeneratesNarrativeWhenAbsent verifies the narrative
// auto-generation step (leveraging services/fhir_narrative, the same
// generator services/cda_fhir already uses) actually runs for a supported
// resource type that has no text.div of its own.
func TestBuildFHIRBundle_GeneratesNarrativeWhenAbsent(t *testing.T) {
	config := map[string]interface{}{
		"mode": "fhir_bundle",
		"fhirBundle": map[string]interface{}{
			"bundleType":    "collection",
			"resourcePaths": []interface{}{"fhir.patient"},
		},
	}
	inputData := map[string]interface{}{
		"fhir": map[string]interface{}{
			"patient": map[string]interface{}{"resourceType": "Patient", "id": "12345", "gender": "female"},
		},
	}

	output := runPayloadBuilder(t, config, inputData)
	bundle := bundleFromOutput(t, output)

	entries := bundle["entry"].([]interface{})
	patient := entries[0].(map[string]interface{})["resource"].(map[string]interface{})
	text, ok := patient["text"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected Patient.text to be set, got %v", patient["text"])
	}
	if div, _ := text["div"].(string); div == "" {
		t.Errorf("expected Patient.text.div to be a non-empty narrative, got %q", div)
	}
	if status, _ := text["status"].(string); status != "generated" {
		t.Errorf("Patient.text.status = %q, want \"generated\"", status)
	}
}

// TestBuildFHIRBundle_PreservesExistingNarrative verifies narrative
// generation never overwrites a text.div the source data already set —
// matching FHIRNarrativeGenerator.Generate's own documented contract.
func TestBuildFHIRBundle_PreservesExistingNarrative(t *testing.T) {
	config := map[string]interface{}{
		"mode": "fhir_bundle",
		"fhirBundle": map[string]interface{}{
			"bundleType":    "collection",
			"resourcePaths": []interface{}{"fhir.patient"},
		},
	}
	existingDiv := `<div xmlns="http://www.w3.org/1999/xhtml">Custom narrative</div>`
	inputData := map[string]interface{}{
		"fhir": map[string]interface{}{
			"patient": map[string]interface{}{
				"resourceType": "Patient", "id": "12345",
				"text": map[string]interface{}{"status": "additional", "div": existingDiv},
			},
		},
	}

	output := runPayloadBuilder(t, config, inputData)
	bundle := bundleFromOutput(t, output)

	entries := bundle["entry"].([]interface{})
	patient := entries[0].(map[string]interface{})["resource"].(map[string]interface{})
	text := patient["text"].(map[string]interface{})
	if div, _ := text["div"].(string); div != existingDiv {
		t.Errorf("Patient.text.div = %q, want the pre-existing narrative preserved unchanged", div)
	}
	if status, _ := text["status"].(string); status != "additional" {
		t.Errorf("Patient.text.status = %q, want \"additional\" (pre-existing value) preserved", status)
	}
}
