// fhir/builder/canonical_field_catalog.go
//
// Public, read-only catalog of every FHIR resource type/profile/field key the
// fhir.build pipeline step (services/executors/transform/fhir_build_executor.go)
// can write to — sourced live from fhir/r4.SchemaRegistry, the SAME compiled
// profile catalog services/executors/validation/fhir_validation_executor.go
// already validates against. This is the single source of truth the
// fhir.build step's no-code field-mapping UI is built from (via a small API
// endpoint), mirroring cda/builder/canonical_field_catalog.go's role for
// cda.build/cda.map_to_canonical — so a user configuring a mapping only ever
// sees target keys fhir.build genuinely consumes, never a stale or invented
// vocabulary.
//
// Deliberately never caches r4.GetRegistry()'s result: InitRegistry() can run
// in a background goroutine (see main.go), so the registry may still be
// populating when these functions are first called. Every call re-fetches
// the live registry, exactly as fhir_validation_executor.go already does.
package builder

import (
	"sort"
	"strings"

	"ezhealthkonnect/fhir/r4"
)

// CanonicalFieldInfo describes one canonical field key available as a
// fhir.build mapping target.
type CanonicalFieldInfo struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	DataType string `json:"dataType,omitempty"`
	Required bool   `json:"required,omitempty"`
	// ValueSetURL/BindingStrength surface fhir/r4's compiled terminology
	// bindings (previously validator-internal only — fhir/r4/validator.go's
	// validateTerminology was the sole consumer) to the fhir.build config UI,
	// so a user can see a field is coded against a specific ValueSet before
	// running a Test Pipeline or waiting on an external validator round.
	// Only "required"/"extensible" bindings are ever populated — compiler.go
	// deliberately drops "preferred"/"example" (no normative obligation).
	ValueSetURL     string `json:"valueSetUrl,omitempty"`
	BindingStrength string `json:"bindingStrength,omitempty"`
}

// ResourceTypeCatalog returns every distinct FHIR resource type compiled for
// version (e.g. "R4"), for the fhir.build step's resource-type picker.
// Returns nil if the registry isn't populated yet.
func ResourceTypeCatalog(version string) []string {
	reg := r4.GetRegistry()
	if reg == nil {
		return nil
	}
	seen := make(map[string]bool)
	var out []string
	for _, k := range reg.List(version) {
		if !seen[k.ResourceType] {
			seen[k.ResourceType] = true
			out = append(out, k.ResourceType)
		}
	}
	sort.Strings(out)
	return out
}

// ProfileCatalog returns every profile name (e.g. "base", "us-core") compiled
// for one resourceType+version, for the fhir.build step's profile picker.
func ProfileCatalog(version, resourceType string) []string {
	reg := r4.GetRegistry()
	if reg == nil {
		return nil
	}
	var out []string
	for _, k := range reg.List(version) {
		if k.ResourceType == resourceType {
			out = append(out, k.Profile)
		}
	}
	sort.Strings(out)
	return out
}

// FieldCatalog returns every element path fhir.build can write for one
// resourceType+profile+version, sourced live from the compiled profile's
// DataTypes/Required maps (fhir/r4.CompiledProfile) — the same maps
// fhir_validation_executor.go validates a built resource against. Element
// paths are compiled as "ResourceType.field" (see fhir/r4/compiler.go's
// compileSchema); the ResourceType prefix is stripped here since
// fhir_build_executor.go's TargetPath is always relative to the resource
// being built, matching CDAFieldDef.Key's flat convention on the CDA side.
// Returns nil if the registry isn't populated yet or resourceType/profile is
// unknown for version.
func FieldCatalog(version, resourceType, profile string) []CanonicalFieldInfo {
	reg := r4.GetRegistry()
	if reg == nil {
		return nil
	}
	cp, ok := reg.Get(version, resourceType, profile)
	if !ok {
		return nil
	}

	bindingByPath := make(map[string]r4.CompiledBinding, len(cp.Bindings))
	for _, b := range cp.Bindings {
		bindingByPath[b.Path] = b
	}

	prefix := resourceType + "."
	out := make([]CanonicalFieldInfo, 0, len(cp.DataTypes))
	for path, dataType := range cp.DataTypes {
		key := strings.TrimPrefix(path, prefix)
		info := CanonicalFieldInfo{
			Key:      key,
			Label:    key,
			DataType: dataType,
			Required: cp.Required[path],
		}
		if b, ok := bindingByPath[path]; ok {
			info.ValueSetURL = b.ValueSetURL
			info.BindingStrength = b.BindingStrength
		}
		out = append(out, info)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}
