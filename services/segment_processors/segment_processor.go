// services/segment_processors/segment_processor.go
//
// SegmentProcessor interface and registry.
//
// Segment processors handle HL7 segments that have no standard FHIR path
// target in any mapping template (MRG, TXA, ZSG, NTE-level enrichment, etc.).
// They run after all normalizers and sanitizeFHIRResources, just before bundle
// assembly, and receive a ResourceCoordinator for safe indexed resource access.
//
// Registering a new processor: implement SegmentProcessor and call
// Register() from the processor file's init() function.  The engine
// resolves processors by name from the manifest's SegmentProcessors list.
package segment_processors

import (
	"fmt"
	"sync"
)

// SegmentProcessorContext carries all state a processor needs to read
// source segments and modify the assembled resource slice.
type SegmentProcessorContext struct {
	// Coordinator is the indexed resource store for this transform request.
	// Processors use Add / Get / Update / FindFirst instead of raw slice ops.
	// UUID assignment is deferred to the bundle assembler — write
	// "ResourceType/id" relative references; rewriteReferences() resolves them.
	Coordinator *ResourceCoordinator

	// ParsedHL7Data is the full enhanced-schema HL7 parse result.
	ParsedHL7Data map[string]interface{}

	// MessageType is the full HL7 message type string (e.g. "ADT^A40").
	MessageType string

	// InterfaceID is the interface UUID, empty for ad-hoc transforms.
	InterfaceID string
}

// SegmentProcessor is the strategy interface for post-assembly segment handling.
type SegmentProcessor interface {
	// Name is the unique identifier used in MessageTypeManifest.SegmentProcessors.
	Name() string
	// Process runs the processor.  Returning a non-nil error causes a warning
	// to be appended to the transform response; it does not abort the pipeline.
	Process(ctx *SegmentProcessorContext) error
}

// registry holds all registered processors, safe for concurrent reads.
var (
	mu       sync.RWMutex
	registry = map[string]SegmentProcessor{}
)

// Register adds a processor to the registry.  Call from processor init().
// Panics on duplicate name — prevents silent shadowing.
func Register(p SegmentProcessor) {
	mu.Lock()
	defer mu.Unlock()
	if _, exists := registry[p.Name()]; exists {
		panic(fmt.Sprintf("segment_processors: duplicate registration for %q", p.Name()))
	}
	registry[p.Name()] = p
}

// Get returns the processor registered under name, or nil if not found.
func Get(name string) SegmentProcessor {
	mu.RLock()
	defer mu.RUnlock()
	return registry[name]
}

// RunAll resolves each name from the manifest's SegmentProcessors list,
// runs it, and collects any errors as warnings (non-fatal).
func RunAll(names []string, ctx *SegmentProcessorContext) []string {
	var warnings []string
	for _, name := range names {
		p := Get(name)
		if p == nil {
			warnings = append(warnings, fmt.Sprintf("segment processor %q not found", name))
			continue
		}
		if err := p.Process(ctx); err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", name, err))
		}
	}
	return warnings
}
