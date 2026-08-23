// services/transform_narrative.go
//
// generateNarrative renders a resource's FHIR-spec-compliant XHTML narrative
// (resource.text.div). Delegates entirely to the unified, resource-shape-driven
// engine in services/fhir_narrative — no per-resource-type Go function lives
// here anymore. See that package's generateGeneric for the generic rendering
// logic, and its dedicated builders (Observation, AllergyIntolerance, etc.) for
// the small set of resource types with real, tested, spec-informed narrative
// logic (e.g. Observation's USCDI category-driven labeling) beyond generic
// field listing.
package services

import (
	"ezhealthkonnect/fhir"
	fhirnarrative "ezhealthkonnect/services/fhir_narrative"
)

// sharedNarrativeGenerator is stateless and safe for concurrent use — one
// instance for the whole service, matching fhirnarrative.FHIRNarrativeGenerator's
// own doc comment ("safe for concurrent use").
var sharedNarrativeGenerator = fhirnarrative.NewFHIRNarrativeGenerator()

// generateNarrative creates a FHIR-spec compliant XHTML narrative (text.div).
// schema is optional (nil-safe) — improves field labels when present.
// fieldConfig is optional (nil-safe) — restricts which fields render when the
// interface has a per-resource-type narrative field configuration; nil means
// "render every populated field" (the default).
func (s *HL7FHIRTransformServiceV3) generateNarrative(
	resourceType string,
	resource map[string]interface{},
	schema *fhir.FHIRSchema,
	fieldConfig fhirnarrative.NarrativeFieldConfig,
) string {
	return sharedNarrativeGenerator.Generate(resource, schema, fieldConfig)
}
