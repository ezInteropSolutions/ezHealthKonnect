// services/hl7_fhir_transform_response_test.go
//
// populateTransformResponse is the function that finalizes every Transform()
// call's return value: strips a known mapping-engine self-nesting artifact,
// counts resources per type, decides whether to build a Bundle, and sets the
// overall Success flag. Pure logic operating on already-built resources — no
// DB — and previously untested.
package services

import (
	"testing"
	"time"
)

func newTestResponse() *TransformResponse {
	return &TransformResponse{
		RequestID:      "test-req",
		MessageType:    "ADT^A01",
		FHIRResources:  []map[string]interface{}{},
		ResourceCounts: make(map[string]int),
		Warnings:       []string{},
		Errors:         []string{},
	}
}

func TestPopulateTransformResponse_StripsSelfNestingArtifact(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	resp := newTestResponse()
	resources := []map[string]interface{}{
		{"resourceType": "Observation", "id": "obs-1", "Observation": map[string]interface{}{"stray": true}},
	}
	s.populateTransformResponse(resp, resources, MappingStatistics{}, nil, nil, time.Now(), time.Now(), false, nil)

	if _, stillNested := resources[0]["Observation"]; stillNested {
		t.Errorf("resource still has the self-nested %q key: %+v", "Observation", resources[0])
	}
	if resources[0]["id"] != "obs-1" {
		t.Errorf("unrelated field %q was dropped: %+v", "id", resources[0])
	}
}

func TestPopulateTransformResponse_CountsResourcesByType(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	resp := newTestResponse()
	resources := []map[string]interface{}{
		{"resourceType": "Patient"},
		{"resourceType": "AllergyIntolerance"},
		{"resourceType": "AllergyIntolerance"},
	}
	s.populateTransformResponse(resp, resources, MappingStatistics{}, nil, nil, time.Now(), time.Now(), false, nil)

	if resp.ResourceCounts["Patient"] != 1 || resp.ResourceCounts["AllergyIntolerance"] != 2 {
		t.Errorf("ResourceCounts = %+v, want Patient:1, AllergyIntolerance:2", resp.ResourceCounts)
	}
}

func TestPopulateTransformResponse_CreateBundleTrue_WithResources_BuildsBundle(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	resp := newTestResponse()
	resources := []map[string]interface{}{{"resourceType": "MessageHeader", "id": "mh1"}}
	s.populateTransformResponse(resp, resources, MappingStatistics{}, nil, nil, time.Now(), time.Now(), true, nil)

	if resp.Bundle == nil {
		t.Fatal("expected a Bundle to be built when createBundle=true and resources are present")
	}
	if resp.Bundle["resourceType"] != "Bundle" {
		t.Errorf("Bundle.resourceType = %v, want Bundle", resp.Bundle["resourceType"])
	}
}

func TestPopulateTransformResponse_CreateBundleTrue_NoResources_NoBundle(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	resp := newTestResponse()
	s.populateTransformResponse(resp, nil, MappingStatistics{}, nil, nil, time.Now(), time.Now(), true, nil)

	if resp.Bundle != nil {
		t.Errorf("expected no Bundle when there are zero resources, got %+v", resp.Bundle)
	}
}

func TestPopulateTransformResponse_CreateBundleFalse_NoBundle(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	resp := newTestResponse()
	resources := []map[string]interface{}{{"resourceType": "Patient"}}
	s.populateTransformResponse(resp, resources, MappingStatistics{}, nil, nil, time.Now(), time.Now(), false, nil)

	if resp.Bundle != nil {
		t.Errorf("expected no Bundle when createBundle=false, got %+v", resp.Bundle)
	}
}

func TestPopulateTransformResponse_Success_TrueWhenResourcesProduced(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	resp := newTestResponse()
	resources := []map[string]interface{}{{"resourceType": "Patient"}}
	// Even with schema-validation-style warnings/errors present, Success should
	// still be true because resources were produced — errors here are advisory,
	// not "the message was unprocessable" per the function's own doc comment.
	s.populateTransformResponse(resp, resources, MappingStatistics{}, []string{"a warning"}, []string{"a validation error"}, time.Now(), time.Now(), false, nil)

	if !resp.Success {
		t.Errorf("Success = false, want true when resources were produced despite advisory errors")
	}
}

func TestPopulateTransformResponse_Success_FalseWhenNoResourcesAndErrors(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	resp := newTestResponse()
	s.populateTransformResponse(resp, nil, MappingStatistics{}, nil, []string{"fatal error"}, time.Now(), time.Now(), false, nil)

	if resp.Success {
		t.Errorf("Success = true, want false when zero resources were produced AND errors are present")
	}
}

func TestPopulateTransformResponse_Success_TrueWhenNoResourcesButNoErrorsEither(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	resp := newTestResponse()
	s.populateTransformResponse(resp, nil, MappingStatistics{}, nil, nil, time.Now(), time.Now(), false, nil)

	if !resp.Success {
		t.Errorf("Success = false, want true when there are simply no resources to produce and no errors either")
	}
}

func TestPopulateTransformResponse_AppendsWarningsAndErrorsRatherThanOverwriting(t *testing.T) {
	s := &HL7FHIRTransformServiceV3{}
	resp := newTestResponse()
	resp.Warnings = append(resp.Warnings, "pre-existing warning")
	s.populateTransformResponse(resp, nil, MappingStatistics{}, []string{"new warning"}, nil, time.Now(), time.Now(), false, nil)

	if len(resp.Warnings) != 2 || resp.Warnings[0] != "pre-existing warning" || resp.Warnings[1] != "new warning" {
		t.Errorf("Warnings = %v, want both the pre-existing and new warning preserved in order", resp.Warnings)
	}
}
