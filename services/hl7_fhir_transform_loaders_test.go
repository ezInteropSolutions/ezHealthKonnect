// services/hl7_fhir_transform_loaders_test.go
//
// Nil-safety coverage for the DB-touching config loaders on
// HL7FHIRTransformServiceV3 — same convention used throughout this codebase
// for DB-backed code with no mocking library (see e.g.
// services/cda_fhir/generic_mapper_narrative_test.go): only the nil-DB/empty-
// interfaceID guard branches are unit-testable without a real DB; the query
// itself is exercised end-to-end via the real HTTP endpoints that read/write
// these same config rows.
//
// isInterfacePureOOB and getInterfaceMessageType previously had NO such guard
// (their only production caller already checks s.db != nil before calling, so
// this was never reachable in the real server — every real construction path
// passes a live *sql.DB) — the guard was added here for consistency with
// every sibling loader in this file and to make them safely unit-testable.
package services

import (
	"context"
	"testing"
)

func TestLoadResourcePolicy_NilDB_ReturnsNil(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	if got := s.loadResourcePolicy(context.Background(), "some-interface-id", "ADT^A01"); got != nil {
		t.Errorf("got %+v, want nil when db is nil", got)
	}
}

func TestLoadResourcePolicy_EmptyInterfaceID_ReturnsNil(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	if got := s.loadResourcePolicy(context.Background(), "", "ADT^A01"); got != nil {
		t.Errorf("got %+v, want nil when interfaceID is empty", got)
	}
}

func TestLoadNarrativeFieldConfig_NilDB_ReturnsNil(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	if got := s.loadNarrativeFieldConfig(context.Background(), "some-interface-id", "ADT^A01"); got != nil {
		t.Errorf("got %+v, want nil when db is nil", got)
	}
}

func TestLoadNarrativeFieldConfig_EmptyInterfaceID_ReturnsNil(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	if got := s.loadNarrativeFieldConfig(context.Background(), "", "ADT^A01"); got != nil {
		t.Errorf("got %+v, want nil when interfaceID is empty", got)
	}
}

func TestLoadOptionalSegmentConfig_NilDB_ReturnsNil(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	if got := s.loadOptionalSegmentConfig(context.Background(), "some-interface-id", "ADT^A01"); got != nil {
		t.Errorf("got %+v, want nil when db is nil", got)
	}
}

func TestLoadOptionalSegmentConfig_EmptyInterfaceID_ReturnsNil(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	if got := s.loadOptionalSegmentConfig(context.Background(), "", "ADT^A01"); got != nil {
		t.Errorf("got %+v, want nil when interfaceID is empty", got)
	}
}

func TestIsInterfacePureOOB_NilDB_ReturnsFalseNotPanic(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	if got := s.isInterfacePureOOB(context.Background(), "some-interface-id", "ADT^A01"); got != false {
		t.Errorf("got %v, want false (conservative default) when db is nil — must not panic", got)
	}
}

func TestGetInterfaceMessageType_NilDB_ReturnsEmptyNotPanic(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	if got := s.getInterfaceMessageType(context.Background(), "some-interface-id"); got != "" {
		t.Errorf("got %q, want empty string when db is nil — must not panic", got)
	}
}
