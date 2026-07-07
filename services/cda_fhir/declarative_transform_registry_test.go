// services/cda_fhir/declarative_transform_registry_test.go
package cdafhir

import "testing"

// TestDeclarativeTypePairDefaults_AllResolve is a regression guard for the
// exact bug found while building the Add Field modal's transform-inference
// wiring: cda_transform_registry.go's OLDER typePairDefaults table names
// transforms (e.g. "cda_cd_to_code", "boolean_direct") that don't exist
// under those names in THIS registry, so InferTypePair's endpoint would
// silently hand back an unresolvable transform for most type pairs if that
// table were reused here. Every entry in declarativeTypePairDefaults must
// resolve via HasTransform, or InferTransform can return a name Apply()
// then rejects at document-processing time.
func TestDeclarativeTypePairDefaults_AllResolve(t *testing.T) {
	r := NewDeclarativeTransformRegistry()
	for _, tp := range declarativeTypePairDefaults {
		if !r.HasTransform(tp.DefaultTransform) {
			t.Errorf("declarativeTypePairDefaults entry %s→%s names transform %q, which is not registered in DeclarativeTransformRegistry",
				tp.CDADataType, tp.FHIRDataType, tp.DefaultTransform)
		}
	}
}

func TestDeclarativeTransformRegistry_InferTransform(t *testing.T) {
	r := NewDeclarativeTransformRegistry()

	transform, err := r.InferTransform("TS", "dateTime")
	if err != nil {
		t.Fatalf("InferTransform(TS, dateTime) unexpected error: %v", err)
	}
	if transform != "cda_time_to_fhir_datetime" {
		t.Errorf("InferTransform(TS, dateTime) = %q, want cda_time_to_fhir_datetime", transform)
	}

	if _, err := r.InferTransform("NOPE", "NOPE"); err == nil {
		t.Error("InferTransform with an unknown pair should return an error, got nil")
	}
}
