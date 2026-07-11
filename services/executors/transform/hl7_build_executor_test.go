// services/executors/transform/hl7_build_executor_test.go
package transform

import (
	"context"
	"strings"
	"testing"

	"ezhealthkonnect/hl7"
	"ezhealthkonnect/models"
)

const testHL7SchemaDir = "../../../schemas/hl7"

func initHL7Schema(t *testing.T) {
	t.Helper()
	hl7.InitRealSchemaLoader(testHL7SchemaDir)
	if hl7.GetRealSchemaLoader() == nil {
		t.Fatal("expected non-nil HL7 schema loader after InitRealSchemaLoader")
	}
}

func runHL7Build(t *testing.T, config map[string]interface{}, inputData map[string]interface{}) map[string]interface{} {
	t.Helper()
	executor := NewHL7BuildExecutor()
	step := &models.TransformationStep{
		StepName: "Test HL7 Build",
		StepType: "hl7.build",
		Enabled:  true,
		Config:   config,
	}
	output, err := executor.Execute(context.Background(), step, inputData)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	return output
}

func hl7Lines(t *testing.T, output map[string]interface{}, field string) []string {
	t.Helper()
	msg, ok := output[field].(string)
	if !ok {
		t.Fatalf("expected output[%q] to be a string, got %T", field, output[field])
	}
	return strings.Split(strings.TrimRight(msg, "\r"), "\r")
}

// TestHL7Build_SingleSegment_CSVLikeRow verifies a "single" cardinality
// segment resolves fields from top-level inputData, mirroring
// map_to_canonical's own CSV-row on-ramp.
func TestHL7Build_SingleSegment_CSVLikeRow(t *testing.T) {
	initHL7Schema(t)
	config := map[string]interface{}{
		"messageType":  "ADT",
		"triggerEvent": "A01",
		"version":      "2.5.1",
		"segments": []interface{}{
			map[string]interface{}{
				"segment":     "PID",
				"cardinality": "single",
				"fields": []interface{}{
					map[string]interface{}{"fieldKey": "PID.3", "sourcePath": "mrn"},
					map[string]interface{}{"fieldKey": "PID.5.1", "sourcePath": "lastName"},
					map[string]interface{}{"fieldKey": "PID.5.2", "sourcePath": "firstName"},
				},
			},
		},
	}
	inputData := map[string]interface{}{"mrn": "MRN123", "lastName": "Doe", "firstName": "Jane"}

	output := runHL7Build(t, config, inputData)
	lines := hl7Lines(t, output, "hl7Message")

	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (MSH, PID), got %d: %v", len(lines), lines)
	}
	if lines[1] != "PID|||MRN123||Doe^Jane" {
		t.Errorf("PID line = %q, want %q", lines[1], "PID|||MRN123||Doe^Jane")
	}
}

// TestHL7Build_ValueMap_TranslatesRawCode verifies ValueMap translation on an
// HL7 field mapping.
func TestHL7Build_ValueMap_TranslatesRawCode(t *testing.T) {
	initHL7Schema(t)
	config := map[string]interface{}{
		"messageType":  "ADT",
		"triggerEvent": "A01",
		"version":      "2.5.1",
		"segments": []interface{}{
			map[string]interface{}{
				"segment": "PID",
				"fields": []interface{}{
					map[string]interface{}{
						"fieldKey":   "PID.8",
						"sourcePath": "sex",
						"valueMap":   map[string]interface{}{"male": "M", "female": "F"},
					},
				},
			},
		},
	}
	inputData := map[string]interface{}{"sex": "female"}

	output := runHL7Build(t, config, inputData)
	lines := hl7Lines(t, output, "hl7Message")

	if lines[1] != "PID||||||||F" {
		t.Errorf("PID line = %q, want %q", lines[1], "PID||||||||F")
	}
}

// TestHL7Build_RepeatingSegment_OBXPerRow verifies a "repeating" cardinality
// segment builds one instance per row, keeping multiple fields from the
// same row aligned.
func TestHL7Build_RepeatingSegment_OBXPerRow(t *testing.T) {
	initHL7Schema(t)
	config := map[string]interface{}{
		"messageType":  "ORU",
		"triggerEvent": "R01",
		"version":      "2.5.1",
		"segments": []interface{}{
			map[string]interface{}{
				"segment":     "OBX",
				"cardinality": "repeating",
				"rowsPath":    "labResults",
				"fields": []interface{}{
					map[string]interface{}{"fieldKey": "OBX.3", "sourcePath": "testCode"},
					map[string]interface{}{"fieldKey": "OBX.5", "sourcePath": "value"},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"labResults": []interface{}{
			map[string]interface{}{"testCode": "GLU", "value": "95"},
			map[string]interface{}{"testCode": "K", "value": "4.1"},
		},
	}

	output := runHL7Build(t, config, inputData)
	lines := hl7Lines(t, output, "hl7Message")

	if len(lines) != 3 {
		t.Fatalf("expected 3 lines (MSH, OBX, OBX), got %d: %v", len(lines), lines)
	}
	if lines[1] != "OBX|||GLU||95" {
		t.Errorf("OBX[0] line = %q, want %q", lines[1], "OBX|||GLU||95")
	}
	if lines[2] != "OBX|||K||4.1" {
		t.Errorf("OBX[1] line = %q, want %q (must stay aligned with its own row)", lines[2], "OBX|||K||4.1")
	}
}

// TestHL7Build_MSHDefaults verifies MSH auto-populates sensible defaults
// (sending application/facility) when not overridden.
func TestHL7Build_MSHDefaults(t *testing.T) {
	initHL7Schema(t)
	config := map[string]interface{}{
		"messageType":  "ADT",
		"triggerEvent": "A01",
		"version":      "2.5.1",
	}
	output := runHL7Build(t, config, map[string]interface{}{})
	lines := hl7Lines(t, output, "hl7Message")

	if !strings.HasPrefix(lines[0], "MSH|^~\\&|ezHealthKonnect|EHK|") {
		t.Errorf("unexpected MSH line: %q", lines[0])
	}
	if !strings.Contains(lines[0], "|ADT^A01|") {
		t.Errorf("expected MSH.9 = ADT^A01, got: %q", lines[0])
	}
}

// TestHL7Build_UnknownMessageType_ReturnsError verifies Execute fails loudly
// for a messageType/triggerEvent/version combination the schema doesn't define.
func TestHL7Build_UnknownMessageType_ReturnsError(t *testing.T) {
	initHL7Schema(t)
	executor := NewHL7BuildExecutor()
	step := &models.TransformationStep{
		StepName: "Test HL7 Build",
		StepType: "hl7.build",
		Enabled:  true,
		Config:   map[string]interface{}{"messageType": "ZZZ", "triggerEvent": "Z99", "version": "2.5.1"},
	}
	_, err := executor.Execute(context.Background(), step, map[string]interface{}{})
	if err == nil {
		t.Fatal("expected an error for an unknown messageType/triggerEvent, got nil")
	}
}

// TestHL7Build_MissingMessageType_ReturnsError verifies the required
// messageType config key is enforced.
func TestHL7Build_MissingMessageType_ReturnsError(t *testing.T) {
	initHL7Schema(t)
	executor := NewHL7BuildExecutor()
	step := &models.TransformationStep{
		StepName: "Test HL7 Build",
		StepType: "hl7.build",
		Enabled:  true,
		Config:   map[string]interface{}{},
	}
	_, err := executor.Execute(context.Background(), step, map[string]interface{}{})
	if err == nil {
		t.Fatal("expected an error when messageType is missing, got nil")
	}
}

// TestHL7Build_RoundTripsThroughRealParser feeds this executor's own output
// back through the existing hl7.ParseWithRealSchema parser — the same
// independent-oracle check hl7/builder/message_builder_test.go performs, run
// here through the full executor (config -> message) instead of the
// lower-level Segment API directly.
func TestHL7Build_RoundTripsThroughRealParser(t *testing.T) {
	initHL7Schema(t)
	config := map[string]interface{}{
		"messageType":  "ADT",
		"triggerEvent": "A01",
		"version":      "2.5.1",
		"segments": []interface{}{
			map[string]interface{}{
				"segment": "PID",
				"fields": []interface{}{
					map[string]interface{}{"fieldKey": "PID.3", "sourcePath": "mrn"},
				},
			},
		},
	}
	output := runHL7Build(t, config, map[string]interface{}{"mrn": "MRN555"})
	msg, _ := output["hl7Message"].(string)

	parsed := hl7.ParseWithRealSchema(msg)
	if !parsed.Success {
		t.Fatalf("ParseWithRealSchema failed on hl7.build's own output: %v", parsed.Error)
	}
	pidSeg, ok := parsed.BasicSegments["PID"]
	if !ok {
		t.Fatal("expected parsed message to contain a PID segment")
	}
	if got := pidSeg.Fields["PID.3"]; got != "MRN555" {
		t.Errorf("parsed PID.3 = %q, want MRN555", got)
	}
}

// TestHL7Build_CustomOutputField verifies outputField is honored.
func TestHL7Build_CustomOutputField(t *testing.T) {
	initHL7Schema(t)
	config := map[string]interface{}{
		"messageType":  "ADT",
		"triggerEvent": "A01",
		"version":      "2.5.1",
		"outputField":  "customHL7Message",
	}
	output := runHL7Build(t, config, map[string]interface{}{})
	if _, present := output["hl7Message"]; present {
		t.Errorf("expected no default hl7Message field when outputField is overridden")
	}
	if _, ok := output["customHL7Message"].(string); !ok {
		t.Errorf("expected output[\"customHL7Message\"] to be a string")
	}
}
