package control

import (
	"encoding/json"
	"log"
	"strings"
)

// ===============================================================
// COLLECTION RESOLVER - OOP Multi-Format Collection Resolution
// ===============================================================
// Strategy Pattern: Each message format has its own resolver that
// knows how to find iterable collections in its data structure.
// The CollectionResolverRegistry auto-detects the format and
// delegates to the appropriate resolver.

// CollectionResolver is the interface that all format-specific resolvers must implement.
// Each resolver knows how to find and extract iterable collections from its format.
type CollectionResolver interface {
	// Name returns the resolver name (e.g., "HL7", "FHIR", "JSON")
	Name() string

	// CanResolve checks if this resolver can handle the given path and data.
	// Returns true if the data format matches this resolver's domain.
	CanResolve(data map[string]interface{}, path string) bool

	// Resolve extracts the collection from the data at the given path.
	// Returns the collection as a slice, or nil if not found.
	Resolve(data map[string]interface{}, path string) []interface{}
}

// CollectionResolverRegistry manages format-specific resolvers and
// auto-detects which resolver to use based on the data structure.
type CollectionResolverRegistry struct {
	resolvers []CollectionResolver // Ordered by priority (first match wins)
}

// NewCollectionResolverRegistry creates a registry with all built-in resolvers.
func NewCollectionResolverRegistry() *CollectionResolverRegistry {
	registry := &CollectionResolverRegistry{
		resolvers: make([]CollectionResolver, 0),
	}

	// Register resolvers in priority order:
	// 1. Nested loop resolver (highest priority - handles loop.item.* paths)
	// 2. HL7 resolver (detects enhancedSegments/segmentGroups)
	// 3. FHIR resolver (detects resourceType)
	// 4. JSON resolver (generic fallback - always matches)
	registry.Register(&NestedLoopResolver{})
	registry.Register(&HL7CollectionResolver{})
	registry.Register(&FHIRCollectionResolver{})
	registry.Register(&JSONCollectionResolver{}) // Always-match fallback

	return registry
}

// Register adds a resolver to the registry.
func (r *CollectionResolverRegistry) Register(resolver CollectionResolver) {
	r.resolvers = append(r.resolvers, resolver)
}

// ResolveCollection tries each registered resolver in order until one succeeds.
// Returns the resolved collection and the name of the resolver that matched.
func (r *CollectionResolverRegistry) ResolveCollection(data map[string]interface{}, path string) ([]interface{}, string) {
	for _, resolver := range r.resolvers {
		if resolver.CanResolve(data, path) {
			result := resolver.Resolve(data, path)
			if result != nil {
				return result, resolver.Name()
			}
		}
	}
	return nil, ""
}

// ===============================================================
// NESTED LOOP RESOLVER - Handles loop.item.* and item.* paths
// ===============================================================
// Highest priority: when inside a nested loop, the collection comes
// from the parent loop's current item.

type NestedLoopResolver struct{}

func (r *NestedLoopResolver) Name() string { return "NestedLoop" }

func (r *NestedLoopResolver) CanResolve(data map[string]interface{}, path string) bool {
	return strings.HasPrefix(path, "loop.item.") ||
		strings.HasPrefix(path, "item.") ||
		path == "observationGroups"
}

func (r *NestedLoopResolver) Resolve(data map[string]interface{}, path string) []interface{} {
	// Handle observationGroups (convenience for OBR-OBX pattern)
	if path == "observationGroups" {
		return r.resolveFromLocations(data, [][]string{
			{"message", "observationGroups"},
			{"parsed_content", "observationGroups"},
			{"observationGroups"},
		})
	}

	// Extract the sub-path after the loop prefix
	var itemPath string
	if strings.HasPrefix(path, "loop.item.") {
		itemPath = strings.TrimPrefix(path, "loop.item.")
	} else if strings.HasPrefix(path, "item.") {
		itemPath = strings.TrimPrefix(path, "item.")
	} else {
		return nil
	}

	// Find the parent loop's current item
	parentItem := r.findParentItem(data)
	if parentItem == nil {
		log.Printf("⚠️  [NestedLoopResolver] No parent loop item found")
		return nil
	}

	// Navigate the sub-path within the parent item
	result := navigatePath(parentItem, strings.Split(itemPath, "."))
	return convertToSlice(result)
}

// findParentItem looks for the parent loop's current item in various locations
func (r *NestedLoopResolver) findParentItem(data map[string]interface{}) map[string]interface{} {
	// Check "loop" context (set by parent loop)
	if loopCtx, ok := data["loop"].(map[string]interface{}); ok {
		if item, ok := loopCtx["item"].(map[string]interface{}); ok {
			return item
		}
	}
	// Check direct "item" (shorthand set by parent loop)
	if item, ok := data["item"].(map[string]interface{}); ok {
		return item
	}
	// Check "_loop" context (internal storage)
	if loopCtx, ok := data["_loop"].(map[string]interface{}); ok {
		if item, ok := loopCtx["item"].(map[string]interface{}); ok {
			return item
		}
	}
	return nil
}

func (r *NestedLoopResolver) resolveFromLocations(data map[string]interface{}, locations [][]string) []interface{} {
	for _, pathParts := range locations {
		result := navigatePath(data, pathParts)
		if result != nil {
			return convertToSlice(result)
		}
	}
	return nil
}

// ===============================================================
// HL7 COLLECTION RESOLVER - Handles HL7 segment iteration
// ===============================================================
// Knows the HL7 data structure: enhancedSegments, segmentGroups,
// observationGroups, and segment shorthand (e.g., "OBX", "IN1").

type HL7CollectionResolver struct{}

func (r *HL7CollectionResolver) Name() string { return "HL7" }

func (r *HL7CollectionResolver) CanResolve(data map[string]interface{}, path string) bool {
	// Can resolve if:
	// 1. Path looks like an HL7 segment name (3 uppercase letters)
	// 2. Data contains HL7-specific keys (enhancedSegments, segmentGroups)
	if isHL7SegmentName(path) {
		return true
	}

	// Check if data has HL7 structure
	if msg := getMessageMap(data); msg != nil {
		if _, hasEnhanced := msg["enhancedSegments"]; hasEnhanced {
			return strings.HasPrefix(path, "segmentGroups.") ||
				strings.HasPrefix(path, "enhancedSegments.")
		}
	}

	return false
}

func (r *HL7CollectionResolver) Resolve(data map[string]interface{}, path string) []interface{} {
	msg := getMessageMap(data)
	if msg == nil {
		// Try data directly (in case message IS the data)
		msg = data
	}

	// For HL7 segment shorthand (e.g., "IN1", "OBX", "NK1")
	if isHL7SegmentName(path) {
		return r.resolveSegment(data, msg, path)
	}

	// For explicit paths like "segmentGroups.OBX"
	if strings.HasPrefix(path, "segmentGroups.") {
		segName := strings.TrimPrefix(path, "segmentGroups.")
		return r.resolveSegment(data, msg, segName)
	}

	// For explicit paths like "enhancedSegments.OBX"
	if strings.HasPrefix(path, "enhancedSegments.") {
		segName := strings.TrimPrefix(path, "enhancedSegments.")
		return r.resolveFromEnhancedSegments(msg, segName)
	}

	return nil
}

// resolveSegment tries all possible locations to find a segment collection
func (r *HL7CollectionResolver) resolveSegment(data, msg map[string]interface{}, segmentName string) []interface{} {
	// Priority 1: segmentGroups (contains ALL instances as an array)
	// Try in message, data, and top-level
	segGroupLocations := []map[string]interface{}{msg, data}
	for _, location := range segGroupLocations {
		if location == nil {
			continue
		}
		sgRaw := location["segmentGroups"]
		if sgRaw == nil {
			continue
		}
		sg, ok := sgRaw.(map[string]interface{})
		if !ok {
			// Handle typed Go maps (e.g., map[string][]hl7.EnhancedSegment) via JSON round-trip
			sg = toGenericMap(sgRaw)
		}
		if sg != nil {
			if segments := sg[segmentName]; segments != nil {
				result := convertToSlice(segments)
				if result != nil && len(result) > 0 {
					return result
				}
			}
		}
	}

	// Priority 2: enhancedSegments (single instance per type - wrap in array)
	result := r.resolveFromEnhancedSegments(msg, segmentName)
	if result != nil {
		return result
	}

	// Priority 3: Try top-level data (for cases where HL7 was merged into inputData)
	result = r.resolveFromEnhancedSegments(data, segmentName)
	if result != nil {
		return result
	}

	// Priority 4: Check if the segment exists as a field array in the message
	// (handles custom parsers that store segments differently)
	if segments := msg[segmentName]; segments != nil {
		result := convertToSlice(segments)
		if result != nil && len(result) > 0 {
			return result
		}
	}

	return nil
}

// resolveFromEnhancedSegments finds a segment in enhancedSegments and wraps in array
func (r *HL7CollectionResolver) resolveFromEnhancedSegments(source map[string]interface{}, segmentName string) []interface{} {
	if source == nil {
		return nil
	}
	esRaw := source["enhancedSegments"]
	es, ok := esRaw.(map[string]interface{})
	if !ok && esRaw != nil {
		// Handle typed Go maps (e.g., map[string]hl7.EnhancedSegment) via JSON round-trip
		es = toGenericMap(esRaw)
		if es != nil {
			ok = true
		}
	}
	if !ok || es == nil {
		return nil
	}
	segment := es[segmentName]
	if segment == nil {
		return nil
	}

	// If it's already a slice (multiple instances), return as-is
	if slice := convertToSlice(segment); slice != nil && len(slice) > 0 {
		return slice
	}

	// Single segment (map) - wrap in array for iteration
	if _, isMap := segment.(map[string]interface{}); isMap {
		return []interface{}{segment}
	}

	return nil
}

// ===============================================================
// FHIR COLLECTION RESOLVER - Handles FHIR resource iteration
// ===============================================================
// Knows the FHIR data structure: Bundle.entry, resource arrays,
// and FHIR path notation (e.g., "Patient.name", "Bundle.entry").

type FHIRCollectionResolver struct{}

func (r *FHIRCollectionResolver) Name() string { return "FHIR" }

func (r *FHIRCollectionResolver) CanResolve(data map[string]interface{}, path string) bool {
	// Can resolve if data contains FHIR markers
	msg := getMessageMap(data)
	if msg == nil {
		msg = data
	}

	// Check for FHIR resource structure
	if _, ok := msg["resourceType"]; ok {
		return true
	}
	if _, ok := msg["fhirBundle"]; ok {
		return true
	}

	// Check for FHIR path notation (ResourceType.field)
	fhirResources := []string{
		"Patient", "Observation", "Encounter", "Condition",
		"Medication", "DiagnosticReport", "Procedure", "AllergyIntolerance",
		"Bundle", "Practitioner", "Organization", "Location",
	}
	for _, resource := range fhirResources {
		if strings.HasPrefix(path, resource+".") || path == resource {
			return true
		}
	}

	return false
}

func (r *FHIRCollectionResolver) Resolve(data map[string]interface{}, path string) []interface{} {
	msg := getMessageMap(data)
	if msg == nil {
		msg = data
	}

	// Handle Bundle.entry (most common FHIR iteration)
	if path == "Bundle.entry" || path == "entry" {
		return r.resolveBundleEntries(msg)
	}

	// Handle filtering by resource type (e.g., "Observation" in a Bundle)
	fhirResources := []string{
		"Patient", "Observation", "Encounter", "Condition",
		"Medication", "DiagnosticReport", "Procedure", "AllergyIntolerance",
		"Practitioner", "Organization", "Location",
	}
	for _, resource := range fhirResources {
		if path == resource {
			return r.filterBundleByResourceType(msg, resource)
		}
	}

	// Handle nested FHIR paths (e.g., "Patient.name", "Observation.component")
	parts := strings.SplitN(path, ".", 2)
	if len(parts) == 2 {
		// Try to find the resource and navigate to the field
		return r.resolveResourceField(msg, parts[0], parts[1])
	}

	return nil
}

// resolveBundleEntries extracts entries from a FHIR Bundle
func (r *FHIRCollectionResolver) resolveBundleEntries(msg map[string]interface{}) []interface{} {
	// Direct Bundle
	if entries, ok := msg["entry"].([]interface{}); ok {
		return entries
	}
	// Nested in fhirBundle
	if bundle, ok := msg["fhirBundle"].(map[string]interface{}); ok {
		if entries, ok := bundle["entry"].([]interface{}); ok {
			return entries
		}
	}
	return nil
}

// filterBundleByResourceType filters Bundle entries by resource type
func (r *FHIRCollectionResolver) filterBundleByResourceType(msg map[string]interface{}, resourceType string) []interface{} {
	entries := r.resolveBundleEntries(msg)
	if entries == nil {
		return nil
	}

	var filtered []interface{}
	for _, entry := range entries {
		entryMap, ok := entry.(map[string]interface{})
		if !ok {
			continue
		}
		resource, ok := entryMap["resource"].(map[string]interface{})
		if !ok {
			continue
		}
		if rt, ok := resource["resourceType"].(string); ok && rt == resourceType {
			filtered = append(filtered, resource)
		}
	}

	return filtered
}

// resolveResourceField navigates into a FHIR resource to find an array field
func (r *FHIRCollectionResolver) resolveResourceField(msg map[string]interface{}, resourceType, fieldPath string) []interface{} {
	// Find the resource (could be direct or in Bundle)
	var resource map[string]interface{}

	// Check if msg IS the resource
	if rt, ok := msg["resourceType"].(string); ok && rt == resourceType {
		resource = msg
	}

	// Check in Bundle entries
	if resource == nil {
		resources := r.filterBundleByResourceType(msg, resourceType)
		if len(resources) > 0 {
			if rm, ok := resources[0].(map[string]interface{}); ok {
				resource = rm
			}
		}
	}

	if resource == nil {
		return nil
	}

	// Navigate to the field
	result := navigatePath(resource, strings.Split(fieldPath, "."))
	return convertToSlice(result)
}

// ===============================================================
// JSON COLLECTION RESOLVER - Generic JSON path resolution
// ===============================================================
// Fallback resolver that handles standard dot-notation paths
// and array indexing (e.g., "data.items", "results[0].values").

type JSONCollectionResolver struct{}

func (r *JSONCollectionResolver) Name() string { return "JSON" }

func (r *JSONCollectionResolver) CanResolve(data map[string]interface{}, path string) bool {
	return true // Always matches as fallback
}

func (r *JSONCollectionResolver) Resolve(data map[string]interface{}, path string) []interface{} {
	// Try the path from the data root
	parts := strings.Split(path, ".")
	result := navigatePath(data, parts)
	if slice := convertToSlice(result); slice != nil {
		return slice
	}

	// Try under "message" key
	if msg := getMessageMap(data); msg != nil {
		result := navigatePath(msg, parts)
		if slice := convertToSlice(result); slice != nil {
			return slice
		}
	}

	return nil
}

// ===============================================================
// SHARED UTILITIES
// ===============================================================

// isHL7SegmentName checks if the path looks like an HL7 segment name (2-3 uppercase letters + optional digit)
func isHL7SegmentName(path string) bool {
	if len(path) < 2 || len(path) > 4 {
		return false
	}
	// Common HL7 segments: MSH, PID, PV1, OBX, IN1, NK1, etc.
	for i, c := range path {
		if i < len(path)-1 {
			// First chars must be uppercase letters
			if c < 'A' || c > 'Z' {
				return false
			}
		} else {
			// Last char can be letter or digit (e.g., PV1, IN1, NK1)
			if (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
				return false
			}
		}
	}
	return true
}

// getMessageMap extracts the "message" sub-map from data
func getMessageMap(data map[string]interface{}) map[string]interface{} {
	if msg, ok := data["message"].(map[string]interface{}); ok {
		return msg
	}
	return nil
}

// navigatePath traverses nested maps using path parts
func navigatePath(data interface{}, parts []string) interface{} {
	current := data
	for _, part := range parts {
		if current == nil {
			return nil
		}
		switch v := current.(type) {
		case map[string]interface{}:
			current = v[part]
		default:
			return nil
		}
	}
	return current
}

// toGenericMap converts a typed Go value (struct, typed map) to map[string]interface{}
// via JSON round-trip. Returns nil if conversion fails. This is essential for handling
// typed Go structs (e.g., map[string]hl7.EnhancedSegment) that come from executors
// that pass typed data through the pipeline context instead of JSON-serialized data.
func toGenericMap(value interface{}) map[string]interface{} {
	if value == nil {
		return nil
	}
	// Already the right type
	if m, ok := value.(map[string]interface{}); ok {
		return m
	}
	// JSON round-trip to convert typed structs to generic maps
	jsonBytes, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var result map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return nil
	}
	return result
}

// toGenericSlice converts a typed Go slice to []interface{} via JSON round-trip.
// Handles typed slices like []hl7.EnhancedSegment that can't be directly asserted.
func toGenericSlice(value interface{}) []interface{} {
	if value == nil {
		return nil
	}
	// Already the right type
	if s, ok := value.([]interface{}); ok {
		return s
	}
	// JSON round-trip
	jsonBytes, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var result []interface{}
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return nil
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// convertToSlice converts various types to []interface{}
// Handles both generic (map[string]interface{}) and typed Go slices/maps via JSON round-trip
func convertToSlice(value interface{}) []interface{} {
	if value == nil {
		return nil
	}

	switch v := value.(type) {
	case []interface{}:
		if len(v) > 0 {
			return v
		}
		return nil
	case []map[string]interface{}:
		if len(v) == 0 {
			return nil
		}
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = item
		}
		return result
	default:
		// Fallback: try JSON round-trip for typed Go slices (e.g., []hl7.EnhancedSegment)
		return toGenericSlice(value)
	}
}
