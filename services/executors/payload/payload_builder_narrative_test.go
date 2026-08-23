// services/executors/payload/payload_builder_narrative_test.go
// Coverage for loadNarrativeFieldConfig -- the narrative field config loader
// wired into the fhir_bundle assembly path so a fhir.build-produced resource
// shares narrative rendering config with hl7_fhir_transform-produced
// resources of the same type in the same interface (same
// interface_message_mappings storage, GET/PATCH /api/fhir/optional-segments).
// Only the nil-safety branches are unit-testable without a real DB
// connection -- this package has no DB-mocking convention, matching
// services/cda_fhir/generic_mapper_narrative_test.go's own CDA-side
// equivalent. The query itself is exercised end-to-end by
// controllers/optional_segments_controller.go's own save/read round trip.
package payload

import (
	"context"
	"testing"
)

func TestLoadNarrativeFieldConfig_NilDB_ReturnsNil(t *testing.T) {
	e := NewPayloadBuilderExecutor(nil)
	cfg := e.loadNarrativeFieldConfig(context.Background(), "some-interface-id", "ADT^A01")
	if cfg != nil {
		t.Errorf("cfg = %+v, want nil when db is nil", cfg)
	}
}

func TestLoadNarrativeFieldConfig_EmptyInterfaceID_ReturnsNil(t *testing.T) {
	e := NewPayloadBuilderExecutor(nil)
	cfg := e.loadNarrativeFieldConfig(context.Background(), "", "ADT^A01")
	if cfg != nil {
		t.Errorf("cfg = %+v, want nil when interfaceID is empty", cfg)
	}
}
