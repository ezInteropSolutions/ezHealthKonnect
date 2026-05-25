// services/segment_processors/obr_processor.go
//
// OBRProcessor handles ORU^R01 cross-resource wiring that field-mapping
// templates cannot express.
//
// It calls AssembleORUObservations which:
//   - Discards the single placeholder Observation the mapping engine produced
//     from enhancedSegments and rebuilds one Observation per OBX segment using
//     the full segmentGroups list (all 14 OBX lines in a typical CBC result).
//   - Maps raw HL7 status codes to FHIR values:
//       OBX.11 "F" → Observation.status "final"
//       OBR.25 "F" → DiagnosticReport.status "final"
//   - Formats HL7 TS timestamps to FHIR instant (e.g. "201204101630" →
//     "2012-04-10T16:30:00+00:00") for DiagnosticReport.issued.
//   - Applies LOINC / UCUM / facility-namespace coding systems to OBX.3 codes
//     and OBR.4, eliminating the "Coding has no system" validator warnings.
//   - Wires DiagnosticReport.result[] → every Observation.
//   - Wires DiagnosticReport.subject and Observation.subject → Patient.
//
// In addition, OBRProcessor wires DiagnosticReport.encounter → Encounter
// (when PV1 produced an Encounter — AssembleORUObservations does not do this).
//
// MessageHeader.focus → DiagnosticReport is handled by the manifest
// FocusTypes augmentation pass that runs after all segment processors.
package segment_processors

import (
	"ezhealthkonnect/services/hl7assembly"
)

type obrProcessor struct{}

func (obrProcessor) Name() string { return "OBRProcessor" }

func (obrProcessor) Process(ctx *SegmentProcessorContext) error {
	// Rebuild all Observations from OBX segments and wire the DiagnosticReport.
	// ctx.ParsedHL7Data carries segmentGroups from the HL7 parser so all OBX
	// instances are available (not just the last one stored in enhancedSegments).
	allResources, _ := hl7assembly.AssembleORUObservations(
		ctx.ParsedHL7Data,
		ctx.Coordinator.All(),
	)
	ctx.Coordinator.ReplaceAll(allResources)

	// Wire DiagnosticReport.encounter → Encounter.
	// AssembleORUObservations does not wire this reference; the ORU^R01
	// manifest permits Encounter and PV1 may produce one via the template.
	dr := ctx.Coordinator.FindFirst("DiagnosticReport")
	enc := ctx.Coordinator.FindFirst("Encounter")
	if dr != nil && enc != nil {
		if encID, _ := enc["id"].(string); encID != "" {
			dr["encounter"] = map[string]interface{}{
				"reference": "Encounter/" + encID,
			}
		}
	}

	return nil
}

func init() {
	Register(obrProcessor{})
}
