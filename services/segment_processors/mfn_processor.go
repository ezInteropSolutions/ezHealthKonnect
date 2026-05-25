package segment_processors

import "ezhealthkonnect/services/hl7assembly"

type mfnProcessor struct{}

func (mfnProcessor) Name() string { return "MFNProcessor" }

func (mfnProcessor) Process(ctx *SegmentProcessorContext) error {
	updated, _ := hl7assembly.AssembleMFNResources(
		ctx.ParsedHL7Data,
		ctx.Coordinator.All(),
	)
	ctx.Coordinator.ReplaceAll(updated)
	return nil
}

func init() { Register(mfnProcessor{}) }
