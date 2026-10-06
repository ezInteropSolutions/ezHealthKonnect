// services/executors/transform/dicom_parse_executor_test.go
package transform

import (
	"context"
	"testing"

	dicompkg "ezhealthkonnect/dicom"
	"ezhealthkonnect/models"

	"github.com/amrshadid/go-dicom/dataelem"
	"github.com/amrshadid/go-dicom/dataset"
)

// buildTestPart10 reuses dicom.SerializeToPart10 directly — the same real
// Part 10 construction path the live receive connector uses — rather than
// hand-building fixture bytes a second time.
func buildTestPart10(t *testing.T) string {
	t.Helper()
	ds := dataset.NewDataset()
	must := func(err error) {
		if err != nil {
			t.Fatalf("buildTestPart10: %v", err)
		}
	}
	must(ds.AddByKeyword("SOPClassUID", dataelem.UI, []byte("1.2.840.10008.5.1.4.1.1.1")))
	must(ds.AddByKeyword("SOPInstanceUID", dataelem.UI, []byte("1.2.3.4.5.55")))
	must(ds.AddByKeyword("PatientID", dataelem.LO, []byte("PID055")))

	out, err := dicompkg.SerializeToPart10("1.2.840.10008.5.1.4.1.1.1", "1.2.3.4.5.55", ds)
	if err != nil {
		t.Fatalf("SerializeToPart10: %v", err)
	}
	return string(out)
}

func TestDICOMParseExecutor_RealFixture(t *testing.T) {
	exec := NewDICOMParseExecutor()
	raw := buildTestPart10(t)

	step := &models.TransformationStep{
		StepName: "Test Parse DICOM", StepType: "dicom.parse", Enabled: true,
		Config: map[string]interface{}{"sourceField": "raw", "outputField": "parsedDICOM"},
	}
	output, err := exec.Execute(context.Background(), step, map[string]interface{}{"raw": raw})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	parsed, ok := output["parsedDICOM"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected parsedDICOM to be a map, got %T", output["parsedDICOM"])
	}
	if parsed["sopInstanceUID"] != "1.2.3.4.5.55" {
		t.Errorf("sopInstanceUID = %v, want 1.2.3.4.5.55", parsed["sopInstanceUID"])
	}
	if parsed["patientId"] != "PID055" {
		t.Errorf("patientId = %v, want PID055", parsed["patientId"])
	}
}

// TestDICOMParseExecutor_OutputFieldConfigured_RootMerge verifies the
// "__root__" outputField escape hatch, mirroring edi.parse's own.
func TestDICOMParseExecutor_OutputFieldConfigured_RootMerge(t *testing.T) {
	exec := NewDICOMParseExecutor()
	raw := buildTestPart10(t)

	step := &models.TransformationStep{
		StepName: "Test Parse DICOM", StepType: "dicom.parse", Enabled: true,
		Config: map[string]interface{}{"sourceField": "raw", "outputField": "__root__"},
	}
	output, err := exec.Execute(context.Background(), step, map[string]interface{}{"raw": raw})
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if output["sopInstanceUID"] != "1.2.3.4.5.55" {
		t.Errorf("expected sopInstanceUID merged at root, got: %v", output["sopInstanceUID"])
	}
	if _, ok := output["parsedDICOM"]; ok {
		t.Error("did not expect a nested parsedDICOM field when outputField is __root__")
	}
}

// TestDICOMParseExecutor_MessageUnwrapFallback verifies the
// inputData["message"] unwrap fallback, mirroring edi.parse's own.
func TestDICOMParseExecutor_MessageUnwrapFallback(t *testing.T) {
	exec := NewDICOMParseExecutor()
	raw := buildTestPart10(t)

	step := &models.TransformationStep{
		StepName: "Test Parse DICOM", StepType: "dicom.parse", Enabled: true,
		Config: map[string]interface{}{"sourceField": "raw", "outputField": "parsedDICOM"},
	}
	input := map[string]interface{}{
		"message": map[string]interface{}{"raw": raw},
	}
	output, err := exec.Execute(context.Background(), step, input)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if output["parsedDICOM"] == nil {
		t.Fatal("expected parsedDICOM to be populated from the wrapped message.raw fallback")
	}
}

// TestDICOMParseExecutor_EmptySourceField_ReturnsError verifies a genuine
// execution failure (no content anywhere) surfaces as a Go error, not a
// silently empty result.
func TestDICOMParseExecutor_EmptySourceField_ReturnsError(t *testing.T) {
	exec := NewDICOMParseExecutor()
	step := &models.TransformationStep{
		StepName: "Test Parse DICOM", StepType: "dicom.parse", Enabled: true,
		Config: map[string]interface{}{"sourceField": "raw"},
	}
	_, err := exec.Execute(context.Background(), step, map[string]interface{}{})
	if err == nil {
		t.Fatal("expected an error when the source field has no content, got nil")
	}
}

// TestDICOMParseExecutor_GarbageContent_ReturnsErrorNotPanic verifies a
// malformed, non-DICOM source field surfaces as a clean error.
func TestDICOMParseExecutor_GarbageContent_ReturnsErrorNotPanic(t *testing.T) {
	exec := NewDICOMParseExecutor()
	step := &models.TransformationStep{
		StepName: "Test Parse DICOM", StepType: "dicom.parse", Enabled: true,
		Config: map[string]interface{}{"sourceField": "raw"},
	}
	_, err := exec.Execute(context.Background(), step, map[string]interface{}{"raw": "not a dicom file"})
	if err == nil {
		t.Fatal("expected an error for non-DICOM content, got nil")
	}
}
