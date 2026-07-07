// services/cda_fhir/generic_mapper_delta_test.go
// Coverage for LoadCDAMappingDeltaPublic/loadCDAMappingDelta -- the raw
// (unmerged) delta lookup added so the pipeline builder's JSON Export/Import
// can carry an interface's actual field-mapping overrides, not just its
// step config. Only the nil-safety branches are unit-testable without a
// real DB connection (this package has no DB-mocking convention); the
// query itself reuses loadCDAMappingOverrides' already-proven SQL verbatim.
package cdafhir

import (
	"context"
	"testing"
)

func TestLoadCDAMappingDeltaPublic_NilDB_ReturnsNil(t *testing.T) {
	m := &GenericCDAFHIRMapper{db: nil}
	delta := m.LoadCDAMappingDeltaPublic(context.Background(), "some-interface-id", "CCD")
	if delta != nil {
		t.Errorf("delta = %+v, want nil when db is nil", delta)
	}
}

func TestLoadCDAMappingDeltaPublic_EmptyInterfaceID_ReturnsNil(t *testing.T) {
	m := &GenericCDAFHIRMapper{db: nil}
	delta := m.LoadCDAMappingDeltaPublic(context.Background(), "", "CCD")
	if delta != nil {
		t.Errorf("delta = %+v, want nil when interfaceID is empty", delta)
	}
}

// TestLoadCDAMappingOverrides_StillMatchesLoadCDAMappingDelta is a regression
// test proving the refactor (loadCDAMappingOverrides now delegates to the new
// loadCDAMappingDelta instead of running its own query) preserves the
// original nil-on-no-DB behavior exactly.
func TestLoadCDAMappingOverrides_StillMatchesLoadCDAMappingDelta(t *testing.T) {
	m := &GenericCDAFHIRMapper{db: nil}
	overrides := m.loadCDAMappingOverrides(context.Background(), "some-interface-id", "CCD")
	if overrides != nil {
		t.Errorf("overrides = %+v, want nil when db is nil", overrides)
	}
}
