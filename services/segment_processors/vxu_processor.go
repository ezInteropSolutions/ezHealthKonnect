package segment_processors

import "ezhealthkonnect/services/hl7assembly"

type vxuProcessor struct{}

func (vxuProcessor) Name() string { return "VXUProcessor" }

func (vxuProcessor) Process(ctx *SegmentProcessorContext) error {
	updated, _ := hl7assembly.AssembleVXUImmunizations(
		ctx.ParsedHL7Data,
		ctx.Coordinator.All(),
	)
	ctx.Coordinator.ReplaceAll(updated)
	return nil
}

func init() { Register(vxuProcessor{}) }
