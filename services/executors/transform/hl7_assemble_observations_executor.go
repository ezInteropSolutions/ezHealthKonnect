// services/executors/transform/hl7_assemble_observations_executor.go
//
// HL7AssembleObservationsExecutor — pipeline step "hl7.assemble_observations"
//
// Purpose:
//   Converts all OBX segments in an HL7 ORU^R01 message into individual FHIR
//   Observation resources and links them to the DiagnosticReport produced by
//   the preceding hl7_fhir_transform step.
//
// Why a step and not baked into hl7_fhir_transform:
//   - Visible in the pipeline builder — user can see it, understand it
//   - Per-interface control — disable for interfaces that handle observations differently
//   - Extensible — user can add enrichment.script steps after to customise specific fields
//
// Step type:  hl7.assemble_observations
// Sequence:   110  (after hl7_fhir_transform at 100, before fhir_validation at 200)
//
// Expected pipeline context (written by hl7_fhir_transform at seq 100):
//   inputData["fhirResources"]  — []map[string]interface{} individual FHIR resources
//   inputData["fhirBundle"]     — map[string]interface{}   FHIR Bundle wrapping those resources
//   inputData["message"]        — map[string]interface{}   parsed HL7 data with segmentGroups
//
// Output (merged into pipeline context):
//   outputData["fhirResources"]  — updated resources (Observations replaced, DR linked)
//   outputData["fhirBundle"]     — rebuilt bundle from updated resources
package transform

import (
	"context"
	"encoding/json"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	"ezhealthkonnect/services/hl7assembly"
	"fmt"
	"log"
	"time"
)

// HL7AssembleObservationsExecutor implements the hl7.assemble_observations pipeline step.
type HL7AssembleObservationsExecutor struct {
	*executors.BaseExecutor
}

func NewHL7AssembleObservationsExecutor() *HL7AssembleObservationsExecutor {
	return &HL7AssembleObservationsExecutor{
		BaseExecutor: executors.NewBaseExecutor("hl7.assemble_observations", models.ExecutorMetadata{
			Name:        "Assemble Observations from OBX",
			Description: "Converts each OBX segment into a FHIR Observation and links all to DiagnosticReport.result[]. Adds standard laboratory category, reference ranges, and interpretation flags.",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "HL7 Transform",
		}),
	}
}

func (e *HL7AssembleObservationsExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	startTime := time.Now()
	log.Printf("  🔬 hl7.assemble_observations: starting")

	// ── 1. Resolve parsed HL7 data (contains segmentGroups) ──────────────────
	parsedHL7Data := e.resolveHL7Data(inputData)
	if parsedHL7Data == nil {
		return nil, fmt.Errorf("hl7.assemble_observations: could not locate parsed HL7 data (segmentGroups) in pipeline context")
	}

	// ── 2. Resolve fhirResources from pipeline context ────────────────────────
	fhirResources, err := e.resolveFHIRResources(inputData)
	if err != nil {
		return nil, fmt.Errorf("hl7.assemble_observations: %w", err)
	}
	if len(fhirResources) == 0 {
		log.Printf("  ⚠️  hl7.assemble_observations: no fhirResources in context — skipping")
		return inputData, nil
	}

	// ── 3. Run assembly ───────────────────────────────────────────────────────
	assembled, warnings := hl7assembly.AssembleORUObservations(parsedHL7Data, fhirResources)

	// ── 4. Rebuild bundle ─────────────────────────────────────────────────────
	bundle := hl7assembly.RebuildFHIRBundle(assembled)

	// ── 5. Write back into pipeline context ──────────────────────────────────
	outputData := make(map[string]interface{}, len(inputData)+4)
	for k, v := range inputData {
		outputData[k] = v
	}
	outputData["fhirResources"] = assembled
	outputData["fhirBundle"] = bundle

	// Keep fhirBundle accessible under message key too (for downstream FHIR validators)
	if msg, ok := outputData["message"].(map[string]interface{}); ok {
		msg["fhirBundle"] = bundle
	}

	// Count by resource type for logging / _stepOutput
	typeCounts := map[string]int{}
	for _, r := range assembled {
		if rt, ok := r["resourceType"].(string); ok {
			typeCounts[rt]++
		}
	}

	durationMs := time.Since(startTime).Milliseconds()
	log.Printf("  ✅ hl7.assemble_observations: %d resources assembled in %dms — %v", len(assembled), durationMs, typeCounts)

	e.SetStepOutputWithDetails(outputData,
		map[string]interface{}{
			"resource_count": len(assembled),
			"resource_types": typeCounts,
			"warnings":       warnings,
		},
		map[string]interface{}{
			"duration_ms":    durationMs,
			"success":        true,
			"resource_count": len(assembled),
			"resource_types": typeCounts,
		},
	)

	return outputData, nil
}

// resolveHL7Data walks nested "message" keys to find the map containing segmentGroups.
func (e *HL7AssembleObservationsExecutor) resolveHL7Data(inputData map[string]interface{}) map[string]interface{} {
	current := inputData
	for i := 0; i < 5; i++ {
		if _, has := current["segmentGroups"]; has {
			return current
		}
		nested, ok := current["message"].(map[string]interface{})
		if !ok {
			break
		}
		current = nested
	}
	// Last try: top-level may itself have segmentGroups via direct injection
	if _, has := inputData["segmentGroups"]; has {
		return inputData
	}
	return nil
}

// resolveFHIRResources extracts the fhirResources slice from the pipeline context.
// Tries several locations in priority order:
//  1. inputData["fhirResources"] — set by hl7_fhir_transform executor (direct)
//  2. inputData["fhirBundle"]["entry"][*]["resource"] — extract from bundle
func (e *HL7AssembleObservationsExecutor) resolveFHIRResources(inputData map[string]interface{}) ([]map[string]interface{}, error) {
	// Priority 1: direct fhirResources key
	if raw, ok := inputData["fhirResources"]; ok {
		switch v := raw.(type) {
		case []map[string]interface{}:
			return v, nil
		case []interface{}:
			resources := make([]map[string]interface{}, 0, len(v))
			for _, item := range v {
				if m, ok := item.(map[string]interface{}); ok {
					resources = append(resources, m)
				}
			}
			return resources, nil
		}
	}

	// Priority 2: extract from fhirBundle.entry[].resource
	if bundleRaw, ok := inputData["fhirBundle"]; ok {
		bundle, ok := bundleRaw.(map[string]interface{})
		if !ok {
			// Try JSON round-trip
			b, _ := json.Marshal(bundleRaw)
			json.Unmarshal(b, &bundle)
		}
		if bundle != nil {
			if entries, ok := bundle["entry"].([]interface{}); ok {
				resources := make([]map[string]interface{}, 0, len(entries))
				for _, entry := range entries {
					if entryMap, ok := entry.(map[string]interface{}); ok {
						if res, ok := entryMap["resource"].(map[string]interface{}); ok {
							resources = append(resources, res)
						}
					}
				}
				if len(resources) > 0 {
					return resources, nil
				}
			}
		}
	}

	return nil, nil // not an error — prior step may have produced no resources
}

func (e *HL7AssembleObservationsExecutor) Validate(step *models.TransformationStep) error {
	return nil // no required config — step works on whatever fhirResources are present
}

func (e *HL7AssembleObservationsExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	return []models.VariableDefinition{
		e.BuildVariableDefinition("resource_count", "_stepOutput.resource_count", "Total FHIR resources after assembly"),
		e.BuildVariableDefinition("resource_types", "_stepOutput.resource_types", "Map of resourceType → count"),
		e.BuildVariableDefinition("warnings", "_stepOutput.warnings", "Advisory messages from the assembly step"),
	}
}
