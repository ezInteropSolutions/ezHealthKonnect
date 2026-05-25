// services/segment_processors/txa_processor.go
//
// TXAProcessor handles MDM^T01/T02/T11 structural assembly.
//
// It calls AssembleMDMDocument which:
//   - Drops the incorrectly template-mapped DocumentReference produced from
//     raw TXA field mappings and replaces it with a properly-built one.
//   - Drops any Observation resources assembled from OBX segments (OBX in
//     MDM carries document content, not clinical observations).
//   - Maps TXA.17 (Document Completion Status, Table 0271) → docStatus.
//   - Maps TXA.19 (Document Availability Status, Table 0273) → status.
//   - Concatenates OBX TX/FT/ST text lines → content[0].attachment.data (base64).
//   - Promotes OBX ED binary / RP external references to content[0].attachment.
//   - Sets MessageHeader.focus → the new DocumentReference.
//
// The processor runs after all template field-mapping and post-normalizer passes
// so that Patient / Encounter resources are already present and their ids can be
// used to wire DocumentReference.subject.
package segment_processors

import (
	"ezhealthkonnect/services/hl7assembly"
)

type txaProcessor struct{}

func (txaProcessor) Name() string { return "TXAProcessor" }

func (txaProcessor) Process(ctx *SegmentProcessorContext) error {
	// Structural assembly: build a correct FHIR R4 DocumentReference from TXA
	// and OBX segments. The assembled result replaces any existing DocumentReference
	// and Observation resources produced by the generic template mapper.
	allResources, _ := hl7assembly.AssembleMDMDocument(
		ctx.ParsedHL7Data,
		ctx.Coordinator.All(),
	)
	ctx.Coordinator.ReplaceAll(allResources)
	return nil
}

func init() {
	Register(txaProcessor{})
}
