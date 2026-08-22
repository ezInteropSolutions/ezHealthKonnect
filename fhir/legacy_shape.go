// fhir/legacy_shape.go
//
// FHIRSchema/FHIRElement were originally populated by fhir/schema_loader.go's
// FHIRSchemaLoader — an independent second implementation of "decode a .gz
// schema file into a Go struct" that duplicated fhir/r4/registry.go's
// SchemaRegistry. That duplication (two systems separately opening,
// decoding, and caching the same files) has been eliminated — deleted 2026-08-16.
//
// services/transform_fhir_setter.go still expects this exact shape across
// ~950 lines of business logic (Elements[path] keyed lookups, the
// strings.Contains(element.Cardinality, "*") array-vs-scalar idiom). Rather
// than rewrite that untested, deeply-woven file to a different shape — real
// risk for zero behavioral benefit — these two struct types are kept
// verbatim, and BuildLegacySchemaShape below produces one from
// r4.CompiledProfile (the single real schema source now) instead of from a
// second file decode. Every consumer of *FHIRSchema is unaffected; only
// where the data comes from changed.
package fhir

import "ezhealthkonnect/fhir/r4"

type FHIRSchema struct {
	ResourceType string                  `json:"resourceType"`
	Version      string                  `json:"version"`
	Name         string                  `json:"name"`
	Description  string                  `json:"description"`
	Profile      string                  `json:"profile,omitempty"`
	Elements     map[string]*FHIRElement `json:"elements"`
	Required     []string                `json:"required"`
	MustSupport  []string                `json:"mustSupport,omitempty"`
}

type FHIRElement struct {
	Path            string `json:"path"`
	Name            string `json:"name"`
	Description     string `json:"description"`
	DataType        string `json:"dataType"`
	Cardinality     string `json:"cardinality"`
	Required        bool   `json:"required"`
	MustSupport     bool   `json:"mustSupport"`
	IsModifier      bool   `json:"isModifier"`
	IsSummary       bool   `json:"isSummary"`
	ValueSet        string `json:"valueSet,omitempty"`
	BindingStrength string `json:"bindingStrength,omitempty"`
	// Constraints intentionally omitted: CompiledProfile only carries
	// resource-root invariants (SpecConstraints), not per-element ones, and
	// no real consumer of this shape ever reads FHIRElement.Constraints
	// (confirmed by direct grep of transform_fhir_setter.go before this
	// migration) — nothing to populate it from, nothing that needs it.
}

// BuildLegacySchemaShape reconstructs a *FHIRSchema from cp — the one
// remaining bridge between the real schema source (r4.SchemaRegistry) and
// services/transform_fhir_setter.go's existing, unchanged consuming code.
// Returns nil for a nil cp (mirrors the old loader's "not found" behavior).
func BuildLegacySchemaShape(cp *r4.CompiledProfile) *FHIRSchema {
	if cp == nil {
		return nil
	}

	schema := &FHIRSchema{
		ResourceType: cp.ResourceType,
		Version:      cp.Version,
		Name:         cp.Name,
		Description:  cp.Description,
		Profile:      cp.Profile,
		Elements:     make(map[string]*FHIRElement, len(cp.MinCard)),
		MustSupport:  cp.MustSupport,
	}

	for path := range cp.Required {
		schema.Required = append(schema.Required, path)
	}

	for path := range cp.MinCard {
		valueSet, bindingStrength, _ := cp.Binding(path)
		schema.Elements[path] = &FHIRElement{
			Path:            path,
			Name:            cp.ElementNames[path],
			Description:     cp.ElementDescriptions[path],
			DataType:        cp.DataTypes[path],
			Cardinality:     cp.Cardinality(path),
			Required:        cp.Required[path],
			MustSupport:     cp.IsMustSupport(path),
			IsModifier:      cp.IsModifier[path],
			IsSummary:       cp.IsSummary[path],
			ValueSet:        valueSet,
			BindingStrength: bindingStrength,
		}
	}

	return schema
}
