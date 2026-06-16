// fhir/r4/bundle_assembler.go
// Shared FHIR R4 bundle-entry assembly: assigns urn:uuid: fullUrls and rewrites
// internal "ResourceType/id" references to match, so every entry satisfies the
// R4 bundle constraints this package's validator already enforces:
//   - bdl-fullurl: each entry's fullUrl must be a valid absolute URI
//   - bdl-7: fullUrl must be unique in a bundle
// and so relative references resolve correctly per FHIR spec §3.3.2.1: when an
// entry's fullUrl is urn:uuid:, references to it MUST also use the urn:uuid: form.
//
// Originally implemented (and still independently useful as a worked example) in
// services/hl7_fhir_transform_service_v3.go's createBundle(); extracted here so the
// CDA→FHIR pipeline (services/cda_fhir) can reuse the exact same, already-validated
// logic instead of re-implementing it.
package r4

import (
	"github.com/google/uuid"
)

// AssembleEntries assigns a urn:uuid: fullUrl to every resource, rewrites every
// "ResourceType/id" reference found anywhere in any resource to the matching
// urn:uuid:, and returns the ready-to-use Bundle.entry array (each entry shaped
// as {"fullUrl": ..., "resource": ...}).
//
// Resources are not reordered — callers that need a specific first entry (e.g.
// MessageHeader for message bundles, Composition for document bundles) must sort
// resources before calling this function.
func AssembleEntries(resources []map[string]interface{}) []interface{} {
	shortToUUID := make(map[string]string, len(resources)) // "Patient/patient-1" → "urn:uuid:..."
	entries := make([]interface{}, len(resources))
	built := make([]map[string]interface{}, len(resources))

	for i, resource := range resources {
		fullURL := "urn:uuid:" + uuid.New().String()
		rt, _ := resource["resourceType"].(string)
		id, _ := resource["id"].(string)
		if rt != "" && id != "" {
			shortToUUID[rt+"/"+id] = fullURL
		}
		built[i] = map[string]interface{}{
			"fullUrl":  fullURL,
			"resource": resource,
		}
	}

	for i, entry := range built {
		if res, ok := entry["resource"].(map[string]interface{}); ok {
			RewriteReferences(res, shortToUUID)
		}
		entries[i] = entry
	}

	return entries
}

// RewriteReferences walks any FHIR resource map/slice and replaces "reference"
// values of the form "ResourceType/id" with their urn:uuid: equivalents from the
// lookup. Values not present in the lookup are left untouched. Safe to call on
// any JSON-shaped value (map[string]interface{} / []interface{} / scalars).
func RewriteReferences(node interface{}, lookup map[string]string) interface{} {
	switch v := node.(type) {
	case map[string]interface{}:
		if ref, ok := v["reference"].(string); ok {
			if u, found := lookup[ref]; found {
				v["reference"] = u
			}
		}
		for k, child := range v {
			v[k] = RewriteReferences(child, lookup)
		}
	case []interface{}:
		for i, item := range v {
			v[i] = RewriteReferences(item, lookup)
		}
	}
	return node
}
