// services/cda_fhir/generic_mapper_narrative_test.go
// Coverage for loadNarrativeFieldConfig -- the CDA-side narrative field
// config loader added so cda.to_fhir steps respect a per-(interface,
// document_type) narrative_fields restriction the same way HL7's
// HL7FHIRTransformServiceV3.loadNarrativeFieldConfig already does. Only the
// nil-safety branches are unit-testable without a real DB connection (this
// package has no DB-mocking convention, per generic_mapper_delta_test.go) --
// the query itself is exercised end-to-end by
// controllers/cda_narrative_fields_controller.go's own save/read round trip.
package cdafhir

import (
	"context"
	"testing"
)

func TestLoadNarrativeFieldConfig_NilDB_ReturnsNil(t *testing.T) {
	m := &GenericCDAFHIRMapper{db: nil}
	cfg := m.loadNarrativeFieldConfig(context.Background(), "some-interface-id", "CCD")
	if cfg != nil {
		t.Errorf("cfg = %+v, want nil when db is nil", cfg)
	}
}

func TestLoadNarrativeFieldConfig_EmptyInterfaceID_ReturnsNil(t *testing.T) {
	m := &GenericCDAFHIRMapper{db: nil}
	cfg := m.loadNarrativeFieldConfig(context.Background(), "", "CCD")
	if cfg != nil {
		t.Errorf("cfg = %+v, want nil when interfaceID is empty", cfg)
	}
}
