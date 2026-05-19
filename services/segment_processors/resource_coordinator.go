// services/segment_processors/resource_coordinator.go
//
// ResourceCoordinator provides indexed, logical-ref access to the assembled
// FHIR resource slice during the segment processor stage.
//
// # Design rationale
//
// Raw slice mutation (append / index) has three failure modes in a multi-stage
// pipeline: (1) stale pointer after reallocation, (2) processor A overwriting
// data set by processor B due to ordering assumptions, (3) UUID placeholders
// generated before the bundle assembler assigns real ones.
//
// The coordinator solves all three:
//   - Every resource is addressable by its logical reference ("ResourceType/id"),
//     so processors never need to scan the slice or remember positions.
//   - Update() applies a mutation function in-place; processors don't need to
//     know how many times a resource has been touched or in what order.
//   - UUID assignment is deferred entirely to the bundle assembler.
//     Processors write "ResourceType/id" relative references;
//     rewriteReferences() in the assembler converts them to urn:uuid: after
//     all entries have been assigned a fullUrl.
//
// # Usage in a SegmentProcessor
//
//	func (p myProcessor) Process(ctx *SegmentProcessorContext) error {
//	    patient := ctx.Coordinator.FindFirst("Patient")
//	    ...
//	    ctx.Coordinator.Update("Patient/"+patientID, func(r map[string]interface{}) {
//	        r["someField"] = "value"
//	    })
//	    newRes := map[string]interface{}{"resourceType": "RelatedPerson", "id": newID, ...}
//	    ctx.Coordinator.Add(newRes)  // returns "RelatedPerson/<newID>"
//	    return nil
//	}
package segment_processors

import "fmt"

// ResourceCoordinator wraps the assembled resource slice with indexed access.
// It is created once per transform request and threaded through all segment
// processors via SegmentProcessorContext.
type ResourceCoordinator struct {
	resources []map[string]interface{}
	index     map[string]int // "ResourceType/id" → position in resources
}

// NewCoordinator wraps resources in a coordinator and builds the index.
// The coordinator and the caller share the same underlying array; mutations via
// Update are immediately visible through All().
func NewCoordinator(resources []map[string]interface{}) *ResourceCoordinator {
	c := &ResourceCoordinator{
		resources: resources,
		index:     make(map[string]int, len(resources)),
	}
	for i, r := range resources {
		if ref := LogicalRef(r); ref != "" {
			c.index[ref] = i
		}
	}
	return c
}

// Add appends a new resource, indexes it, and returns its logical reference
// ("ResourceType/id"). Panics when the resource has no non-empty id field —
// every FHIR resource must have an id before entering the bundle.
func (c *ResourceCoordinator) Add(r map[string]interface{}) string {
	ref := LogicalRef(r)
	if ref == "" {
		panic("ResourceCoordinator.Add: resource must have non-empty resourceType and id")
	}
	c.index[ref] = len(c.resources)
	c.resources = append(c.resources, r)
	return ref
}

// Get returns the resource identified by logicalRef (e.g. "Patient/patient-123"),
// or nil when not found.
func (c *ResourceCoordinator) Get(logicalRef string) map[string]interface{} {
	if i, ok := c.index[logicalRef]; ok {
		return c.resources[i]
	}
	return nil
}

// Update applies fn to the resource at logicalRef. It is a no-op when the
// reference is not found. Use this instead of Get + direct mutation to make
// the intent clear and avoid accidentally holding a stale pointer.
func (c *ResourceCoordinator) Update(logicalRef string, fn func(map[string]interface{})) {
	if i, ok := c.index[logicalRef]; ok {
		fn(c.resources[i])
	}
}

// FindFirst returns the first resource whose resourceType matches rt, or nil.
func (c *ResourceCoordinator) FindFirst(rt string) map[string]interface{} {
	for _, r := range c.resources {
		if t, _ := r["resourceType"].(string); t == rt {
			return r
		}
	}
	return nil
}

// AllOfType returns all resources whose resourceType matches rt in insertion order.
func (c *ResourceCoordinator) AllOfType(rt string) []map[string]interface{} {
	var out []map[string]interface{}
	for _, r := range c.resources {
		if t, _ := r["resourceType"].(string); t == rt {
			out = append(out, r)
		}
	}
	return out
}

// All returns the full resource slice in insertion order.
// This is the value to hand back to the pipeline after processors run.
func (c *ResourceCoordinator) All() []map[string]interface{} {
	return c.resources
}

// LogicalRef returns the "ResourceType/id" string for a resource.
// Returns "" when either field is absent or empty.
func LogicalRef(r map[string]interface{}) string {
	rt, _ := r["resourceType"].(string)
	id, _ := r["id"].(string)
	if rt == "" || id == "" {
		return ""
	}
	return fmt.Sprintf("%s/%s", rt, id)
}
