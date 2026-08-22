// fhir/legacy_shape_test.go
//
// Regression coverage for BuildLegacySchemaShape — the one new function
// introduced when fhir/schema_loader.go (an independent second .gz-decoding
// system) was deleted in favor of a single schema source, fhir/r4.SchemaRegistry.
// See legacy_shape.go's own doc comment for the full rationale.
//
// This does NOT re-verify compileSchema's own correctness (already covered by
// fhir/r4's own tests) — it verifies BuildLegacySchemaShape is a lossless,
// field-by-field transcription of a *r4.CompiledProfile into the legacy
// FHIRSchema/FHIRElement shape services/transform_fhir_setter.go still
// consumes. Runs against every real resource/profile the registry has
// compiled from schemas/fhir/R4 on disk — not a hardcoded resource list — so
// it stays correct as schemas are added or regenerated.
package fhir_test

import (
	"testing"

	"ezhealthkonnect/fhir"
	"ezhealthkonnect/fhir/r4"
)

func TestBuildLegacySchemaShape_Nil(t *testing.T) {
	if got := fhir.BuildLegacySchemaShape(nil); got != nil {
		t.Errorf("BuildLegacySchemaShape(nil) = %+v, want nil", got)
	}
}

func TestBuildLegacySchemaShape_MatchesCompiledProfile(t *testing.T) {
	if err := r4.InitRegistry("../schemas/fhir"); err != nil {
		t.Fatalf("InitRegistry: %v", err)
	}
	reg := r4.GetRegistry()
	if reg == nil {
		t.Fatal("registry is nil after InitRegistry")
	}

	keys := reg.List("R4")
	if len(keys) == 0 {
		t.Fatal("registry has zero R4 schemas loaded — cannot verify adapter against real data")
	}

	checked := 0
	for _, key := range keys {
		cp, ok := reg.Get(key.Version, key.ResourceType, key.Profile)
		if !ok || cp == nil {
			t.Fatalf("registry.List returned %v but Get failed", key)
		}

		schema := fhir.BuildLegacySchemaShape(cp)
		if schema == nil {
			t.Fatalf("BuildLegacySchemaShape(%v) returned nil for a non-nil profile", key)
		}

		if schema.ResourceType != cp.ResourceType || schema.Version != cp.Version ||
			schema.Name != cp.Name || schema.Description != cp.Description || schema.Profile != cp.Profile {
			t.Errorf("%v: top-level fields mismatch: got %+v", key, schema)
		}

		if len(schema.Elements) != len(cp.MinCard) {
			t.Errorf("%v: Elements count = %d, want %d (len(cp.MinCard))", key, len(schema.Elements), len(cp.MinCard))
		}

		for path := range cp.MinCard {
			elem, ok := schema.Elements[path]
			if !ok {
				t.Errorf("%v: missing element %q in adapter output", key, path)
				continue
			}
			wantVS, wantStrength, _ := cp.Binding(path)
			if elem.Path != path {
				t.Errorf("%v %q: Path = %q", key, path, elem.Path)
			}
			if elem.Name != cp.ElementNames[path] {
				t.Errorf("%v %q: Name = %q, want %q", key, path, elem.Name, cp.ElementNames[path])
			}
			if elem.Description != cp.ElementDescriptions[path] {
				t.Errorf("%v %q: Description mismatch", key, path)
			}
			if elem.DataType != cp.DataTypes[path] {
				t.Errorf("%v %q: DataType = %q, want %q", key, path, elem.DataType, cp.DataTypes[path])
			}
			if elem.Cardinality != cp.Cardinality(path) {
				t.Errorf("%v %q: Cardinality = %q, want %q", key, path, elem.Cardinality, cp.Cardinality(path))
			}
			if elem.Required != cp.Required[path] {
				t.Errorf("%v %q: Required = %v, want %v", key, path, elem.Required, cp.Required[path])
			}
			if elem.MustSupport != cp.IsMustSupport(path) {
				t.Errorf("%v %q: MustSupport = %v, want %v", key, path, elem.MustSupport, cp.IsMustSupport(path))
			}
			if elem.IsModifier != cp.IsModifier[path] {
				t.Errorf("%v %q: IsModifier mismatch", key, path)
			}
			if elem.IsSummary != cp.IsSummary[path] {
				t.Errorf("%v %q: IsSummary mismatch", key, path)
			}
			if elem.ValueSet != wantVS {
				t.Errorf("%v %q: ValueSet = %q, want %q", key, path, elem.ValueSet, wantVS)
			}
			if elem.BindingStrength != wantStrength {
				t.Errorf("%v %q: BindingStrength = %q, want %q", key, path, elem.BindingStrength, wantStrength)
			}
		}

		// Required is rebuilt from a map (cp.Required), so slice ORDER is not
		// deterministic — compare as sets, not exact slice equality.
		gotRequired := make(map[string]bool, len(schema.Required))
		for _, p := range schema.Required {
			gotRequired[p] = true
		}
		for p, want := range cp.Required {
			if want && !gotRequired[p] {
				t.Errorf("%v: Required missing path %q", key, p)
			}
		}
		for p := range gotRequired {
			if !cp.Required[p] {
				t.Errorf("%v: Required has extra path %q not in cp.Required", key, p)
			}
		}

		if len(schema.MustSupport) != len(cp.MustSupport) {
			t.Errorf("%v: MustSupport length = %d, want %d", key, len(schema.MustSupport), len(cp.MustSupport))
		}

		checked++
	}

	t.Logf("verified BuildLegacySchemaShape against %d real compiled profiles", checked)
}
