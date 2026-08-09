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

// TestHL7Build_SegmentCondition_SkipsWholeSegmentWhenFalse verifies a
// "single" segment with a Condition that evaluates false is omitted
// entirely — the MSH-only-message case, not an empty PID segment.
func TestHL7Build_SegmentCondition_SkipsWholeSegmentWhenFalse(t *testing.T) {
	initHL7Schema(t)
	config := map[string]interface{}{
		"messageType":  "ADT",
		"triggerEvent": "A01",
		"version":      "2.5.1",
		"segments": []interface{}{
			map[string]interface{}{
				"segment":   "IN1",
				"condition": map[string]interface{}{"field": "hasInsurance", "operator": "equals", "value": true},
				"fields": []interface{}{
					map[string]interface{}{"fieldKey": "IN1.2", "sourcePath": "planId"},
				},
			},
		},
	}
	output := runHL7Build(t, config, map[string]interface{}{"hasInsurance": false, "planId": "PLAN1"})
	lines := hl7Lines(t, output, "hl7Message")
	if len(lines) != 1 {
		t.Fatalf("expected only MSH (IN1 condition false), got %d lines: %v", len(lines), lines)
	}
}

// TestHL7Build_SegmentCondition_BuildsSegmentWhenTrue is the positive
// counterpart — same config, condition-satisfying data.
func TestHL7Build_SegmentCondition_BuildsSegmentWhenTrue(t *testing.T) {
	initHL7Schema(t)
	config := map[string]interface{}{
		"messageType":  "ADT",
		"triggerEvent": "A01",
		"version":      "2.5.1",
		"segments": []interface{}{
			map[string]interface{}{
				"segment":   "IN1",
				"condition": map[string]interface{}{"field": "hasInsurance", "operator": "equals", "value": true},
				"fields": []interface{}{
					map[string]interface{}{"fieldKey": "IN1.2", "sourcePath": "planId"},
				},
			},
		},
	}
	output := runHL7Build(t, config, map[string]interface{}{"hasInsurance": true, "planId": "PLAN1"})
	lines := hl7Lines(t, output, "hl7Message")
	if len(lines) != 2 || lines[1] != "IN1||PLAN1" {
		t.Fatalf("expected IN1 segment to be built, got lines: %v", lines)
	}
}

// TestHL7Build_RepeatingSegmentCondition_SkipsOnlyMatchingRows verifies a
// per-row Condition on a "repeating" segment excludes individual rows
// without affecting the rest of the array — e.g. skip a cancelled result but
// keep the others.
func TestHL7Build_RepeatingSegmentCondition_SkipsOnlyMatchingRows(t *testing.T) {
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
				"condition":   map[string]interface{}{"field": "status", "operator": "not_equals", "value": "cancelled"},
				"fields": []interface{}{
					map[string]interface{}{"fieldKey": "OBX.3", "sourcePath": "testCode"},
					map[string]interface{}{"fieldKey": "OBX.5", "sourcePath": "value"},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"labResults": []interface{}{
			map[string]interface{}{"testCode": "GLU", "value": "95", "status": "final"},
			map[string]interface{}{"testCode": "K", "value": "4.1", "status": "cancelled"},
			map[string]interface{}{"testCode": "NA", "value": "140", "status": "final"},
		},
	}

	output := runHL7Build(t, config, inputData)
	lines := hl7Lines(t, output, "hl7Message")
	if len(lines) != 3 {
		t.Fatalf("expected MSH + 2 OBX (cancelled row excluded), got %d lines: %v", len(lines), lines)
	}
	if lines[1] != "OBX|||GLU||95" || lines[2] != "OBX|||NA||140" {
		t.Errorf("unexpected OBX lines: %v", lines[1:])
	}
}

// TestHL7Build_FieldCondition_RowFallsBackToTopLevel verifies a repeating
// row's field Condition can check a field that only exists at the top level
// (not on the row itself) — the "row data with fallback to top-level"
// semantics conditionMet/mergeWithFallback implement.
func TestHL7Build_FieldCondition_RowFallsBackToTopLevel(t *testing.T) {
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
					map[string]interface{}{
						"fieldKey": "OBX.5", "sourcePath": "value",
						// "includeUnits" only exists at the top level, not on
						// each row — must fall back there to be found.
						"condition": map[string]interface{}{"field": "includeUnits", "operator": "equals", "value": true},
					},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"includeUnits": true,
		"labResults":   []interface{}{map[string]interface{}{"testCode": "GLU", "value": "95"}},
	}

	output := runHL7Build(t, config, inputData)
	lines := hl7Lines(t, output, "hl7Message")
	if len(lines) != 2 || lines[1] != "OBX|||GLU||95" {
		t.Fatalf("expected OBX.5 to be populated via top-level fallback, got: %v", lines)
	}
}

// TestHL7Build_FieldCondition_FalseOmitsOnlyThatField verifies a field-level
// Condition that evaluates false skips just that field, not the whole
// segment (distinct from the segment-level Condition test above).
func TestHL7Build_FieldCondition_FalseOmitsOnlyThatField(t *testing.T) {
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
					map[string]interface{}{
						"fieldKey": "PID.19", "sourcePath": "ssn",
						"condition": map[string]interface{}{"field": "country", "operator": "equals", "value": "US"},
					},
				},
			},
		},
	}
	output := runHL7Build(t, config, map[string]interface{}{"mrn": "MRN123", "ssn": "123-45-6789", "country": "CA"})
	lines := hl7Lines(t, output, "hl7Message")
	// PID.3 populated (no condition), PID.19 omitted (condition false) —
	// segment itself still present since "single" segments always are.
	if lines[1] != "PID|||MRN123" {
		t.Errorf("PID line = %q, want %q (SSN suppressed by false condition)", lines[1], "PID|||MRN123")
	}
}

// TestHL7Build_GroupBy_GroupsFlatCSVIntoOBRWithChildOBX is the exact
// real-world scenario that motivated GroupBy/ChildSegments: a single flat
// CSV of (orderId, testName, analyte, value) rows for two lab panels (CBC,
// CMP) must become OBR(CBC) followed by ITS OWN OBX results, then OBR(CMP)
// followed by ITS results — not all OBRs then all OBXs, which a flat
// independently-looped segment model would produce.
func TestHL7Build_GroupBy_GroupsFlatCSVIntoOBRWithChildOBX(t *testing.T) {
	initHL7Schema(t)
	config := map[string]interface{}{
		"messageType":  "ORU",
		"triggerEvent": "R01",
		"version":      "2.5.1",
		"segments": []interface{}{
			map[string]interface{}{
				"segment": "OBR", "cardinality": "repeating", "rowsPath": "orders",
				"groupBy": []interface{}{"orderId"},
				"fields": []interface{}{
					map[string]interface{}{"fieldKey": "OBR.1", "sourcePath": "orderId"},
					map[string]interface{}{"fieldKey": "OBR.2", "sourcePath": "testName"},
				},
				"childSegments": []interface{}{
					map[string]interface{}{
						"segment": "OBX", "cardinality": "repeating", "rowsPath": "_rows",
						"fields": []interface{}{
							map[string]interface{}{"fieldKey": "OBX.3", "sourcePath": "analyte"},
							map[string]interface{}{"fieldKey": "OBX.5", "sourcePath": "value"},
						},
					},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"orders": []interface{}{
			map[string]interface{}{"orderId": "1", "testName": "CBC", "analyte": "WBC", "value": "5.4"},
			map[string]interface{}{"orderId": "1", "testName": "CBC", "analyte": "RBC", "value": "4.8"},
			map[string]interface{}{"orderId": "2", "testName": "CMP", "analyte": "Glucose", "value": "95"},
			map[string]interface{}{"orderId": "2", "testName": "CMP", "analyte": "Sodium", "value": "140"},
		},
	}

	output := runHL7Build(t, config, inputData)
	lines := hl7Lines(t, output, "hl7Message")
	want := []string{
		"OBR|1|CBC",
		"OBX|||WBC||5.4",
		"OBX|||RBC||4.8",
		"OBR|2|CMP",
		"OBX|||Glucose||95",
		"OBX|||Sodium||140",
	}
	if len(lines) != len(want)+1 { // +1 for MSH
		t.Fatalf("expected MSH + %d lines, got %d: %v", len(want), len(lines), lines)
	}
	for i, w := range want {
		if lines[i+1] != w {
			t.Errorf("line %d = %q, want %q (full: %v)", i+1, lines[i+1], w, lines)
		}
	}
}

// TestHL7Build_GroupBy_RowMissingKeyBecomesSingletonNotDropped verifies a
// row missing the GroupBy column becomes its own bucket rather than being
// silently excluded — mirrors map_to_canonical's own rule for the identical
// situation.
func TestHL7Build_GroupBy_RowMissingKeyBecomesSingletonNotDropped(t *testing.T) {
	initHL7Schema(t)
	config := map[string]interface{}{
		"messageType":  "ORU",
		"triggerEvent": "R01",
		"version":      "2.5.1",
		"segments": []interface{}{
			map[string]interface{}{
				"segment": "OBR", "cardinality": "repeating", "rowsPath": "orders",
				"groupBy": []interface{}{"orderId"},
				"fields": []interface{}{
					map[string]interface{}{"fieldKey": "OBR.2", "sourcePath": "testName"},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"orders": []interface{}{
			map[string]interface{}{"orderId": "1", "testName": "CBC"},
			map[string]interface{}{"testName": "UNGROUPED"}, // no orderId at all
		},
	}
	output := runHL7Build(t, config, inputData)
	lines := hl7Lines(t, output, "hl7Message")
	if len(lines) != 3 { // MSH + 2 OBR (one per bucket, including the singleton)
		t.Fatalf("expected MSH + 2 OBR (singleton not dropped), got %d lines: %v", len(lines), lines)
	}
	if lines[1] != "OBR||CBC" || lines[2] != "OBR||UNGROUPED" {
		t.Errorf("unexpected OBR lines: %v", lines[1:])
	}
}

// TestHL7Build_ChildSegments_AlreadyNestedData_NoGroupBy verifies a
// ChildSegment's RowsPath resolves relative to its parent's OWN current row
// when the source data already arrives pre-nested (e.g. from an API) — no
// GroupBy needed at all in that case.
func TestHL7Build_ChildSegments_AlreadyNestedData_NoGroupBy(t *testing.T) {
	initHL7Schema(t)
	config := map[string]interface{}{
		"messageType":  "ORU",
		"triggerEvent": "R01",
		"version":      "2.5.1",
		"segments": []interface{}{
			map[string]interface{}{
				"segment": "OBR", "cardinality": "repeating", "rowsPath": "orders",
				"fields": []interface{}{
					map[string]interface{}{"fieldKey": "OBR.1", "sourcePath": "orderId"},
					map[string]interface{}{"fieldKey": "OBR.2", "sourcePath": "testName"},
				},
				"childSegments": []interface{}{
					map[string]interface{}{
						"segment": "OBX", "cardinality": "repeating", "rowsPath": "results",
						"fields": []interface{}{
							map[string]interface{}{"fieldKey": "OBX.3", "sourcePath": "analyte"},
							map[string]interface{}{"fieldKey": "OBX.5", "sourcePath": "value"},
						},
					},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"orders": []interface{}{
			map[string]interface{}{"orderId": "1", "testName": "CBC", "results": []interface{}{
				map[string]interface{}{"analyte": "WBC", "value": "5.4"},
			}},
			map[string]interface{}{"orderId": "2", "testName": "CMP", "results": []interface{}{
				map[string]interface{}{"analyte": "Glucose", "value": "95"},
			}},
		},
	}

	output := runHL7Build(t, config, inputData)
	lines := hl7Lines(t, output, "hl7Message")
	want := []string{"OBR|1|CBC", "OBX|||WBC||5.4", "OBR|2|CMP", "OBX|||Glucose||95"}
	if len(lines) != len(want)+1 {
		t.Fatalf("expected MSH + %d lines, got %d: %v", len(want), len(lines), lines)
	}
	for i, w := range want {
		if lines[i+1] != w {
			t.Errorf("line %d = %q, want %q (full: %v)", i+1, lines[i+1], w, lines)
		}
	}
}

// TestHL7Build_ChildSegments_MultipleChildrenBuildInConfiguredOrder verifies
// two different ChildSegments under the same parent both build, in the
// order they're configured, for every parent instance.
func TestHL7Build_ChildSegments_MultipleChildrenBuildInConfiguredOrder(t *testing.T) {
	initHL7Schema(t)
	config := map[string]interface{}{
		"messageType":  "ORU",
		"triggerEvent": "R01",
		"version":      "2.5.1",
		"segments": []interface{}{
			map[string]interface{}{
				"segment": "OBR", "cardinality": "repeating", "rowsPath": "orders",
				"fields": []interface{}{map[string]interface{}{"fieldKey": "OBR.1", "sourcePath": "orderId"}},
				"childSegments": []interface{}{
					map[string]interface{}{
						"segment": "NTE", "cardinality": "single",
						"fields": []interface{}{map[string]interface{}{"fieldKey": "NTE.3", "sourcePath": "note"}},
					},
					map[string]interface{}{
						"segment": "OBX", "cardinality": "repeating", "rowsPath": "results",
						"fields": []interface{}{map[string]interface{}{"fieldKey": "OBX.3", "sourcePath": "analyte"}},
					},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"orders": []interface{}{
			map[string]interface{}{"orderId": "1", "note": "STAT", "results": []interface{}{
				map[string]interface{}{"analyte": "WBC"},
			}},
		},
	}
	output := runHL7Build(t, config, inputData)
	lines := hl7Lines(t, output, "hl7Message")
	want := []string{"OBR|1", "NTE|||STAT", "OBX|||WBC"}
	if len(lines) != len(want)+1 {
		t.Fatalf("expected MSH + %d lines, got %d: %v", len(want), len(lines), lines)
	}
	for i, w := range want {
		if lines[i+1] != w {
			t.Errorf("line %d = %q, want %q (full: %v)", i+1, lines[i+1], w, lines)
		}
	}
}

// TestHL7Build_ThreeLevelChain_ORCOwnsGroupBy_OBRPassesThroughToOBX verifies
// GroupBy/ChildSegments generalizes beyond a single parent+children level:
// real ORU_R01 order groups are ORC (order control) THEN OBR (the request),
// with OBX results nested under OBR — i.e. the segment that must own the
// GroupBy bucketing (ORC, since it's first) is NOT the same segment whose
// children (OBX) actually need the bucket's rows. This only works if a
// "single" cardinality child (OBR) transparently passes its own context
// (including the GroupedItemsKey) through to ITS children unchanged —
// verified here rather than just reasoned about, per this session's own
// "verify, don't assume" lesson from the CanRepeat bug.
func TestHL7Build_ThreeLevelChain_ORCOwnsGroupBy_OBRPassesThroughToOBX(t *testing.T) {
	initHL7Schema(t)
	config := map[string]interface{}{
		"messageType":  "ORU",
		"triggerEvent": "R01",
		"version":      "2.5.1",
		"segments": []interface{}{
			map[string]interface{}{
				"segment": "ORC", "cardinality": "repeating", "rowsPath": "orders",
				"groupBy": []interface{}{"orderId"},
				"fields":  []interface{}{map[string]interface{}{"fieldKey": "ORC.1", "sourcePath": "orderId"}},
				"childSegments": []interface{}{
					map[string]interface{}{
						"segment": "OBR", "cardinality": "single",
						"fields": []interface{}{map[string]interface{}{"fieldKey": "OBR.2", "sourcePath": "testName"}},
						"childSegments": []interface{}{
							map[string]interface{}{
								"segment": "OBX", "cardinality": "repeating", "rowsPath": "_rows",
								"fields": []interface{}{map[string]interface{}{"fieldKey": "OBX.3", "sourcePath": "analyte"}},
							},
						},
					},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"orders": []interface{}{
			map[string]interface{}{"orderId": "1", "testName": "CBC", "analyte": "WBC"},
			map[string]interface{}{"orderId": "1", "testName": "CBC", "analyte": "RBC"},
			map[string]interface{}{"orderId": "2", "testName": "CMP", "analyte": "Glucose"},
		},
	}
	output := runHL7Build(t, config, inputData)
	lines := hl7Lines(t, output, "hl7Message")
	want := []string{"ORC|1", "OBR||CBC", "OBX|||WBC", "OBX|||RBC", "ORC|2", "OBR||CMP", "OBX|||Glucose"}
	if len(lines) != len(want)+1 {
		t.Fatalf("expected MSH + %d lines, got %d: %v", len(want), len(lines), lines)
	}
	for i, w := range want {
		if lines[i+1] != w {
			t.Errorf("line %d = %q, want %q (full: %v)", i+1, lines[i+1], w, lines)
		}
	}
}

// TestHL7Build_GroupBy_MultipleSiblingChildren_SingleAndRepeatingMixed
// verifies a DIFFERENT real grouping shape than OBR/OBX: ADT_A01's
// INSURANCE group is IN1 (once per insurance block) + IN2 (optional,
// single) + IN3 (repeating certifications) + ROL (repeating roles) — i.e.
// ONE GroupBy parent with MULTIPLE children of DIFFERENT cardinalities,
// not a single nested child. Confirmed against the real compiled ADT_A01
// schema before writing this test (IN2 repeat="1", IN3 repeat="*").
func TestHL7Build_GroupBy_MultipleSiblingChildren_SingleAndRepeatingMixed(t *testing.T) {
	initHL7Schema(t)
	config := map[string]interface{}{
		"messageType":  "ADT",
		"triggerEvent": "A01",
		"version":      "2.5.1",
		"segments": []interface{}{
			map[string]interface{}{
				"segment": "IN1", "cardinality": "repeating", "rowsPath": "coverages",
				"groupBy": []interface{}{"planId"},
				"fields":  []interface{}{map[string]interface{}{"fieldKey": "IN1.2", "sourcePath": "planId"}},
				"childSegments": []interface{}{
					map[string]interface{}{
						"segment": "IN2", "cardinality": "single",
						"fields": []interface{}{map[string]interface{}{"fieldKey": "IN2.1", "sourcePath": "groupNumber"}},
					},
					map[string]interface{}{
						"segment": "IN3", "cardinality": "repeating", "rowsPath": "_rows",
						"fields": []interface{}{map[string]interface{}{"fieldKey": "IN3.1", "sourcePath": "certNumber"}},
					},
				},
			},
		},
	}
	inputData := map[string]interface{}{
		"coverages": []interface{}{
			map[string]interface{}{"planId": "P1", "groupNumber": "G100", "certNumber": "C1"},
			map[string]interface{}{"planId": "P1", "groupNumber": "G100", "certNumber": "C2"},
			map[string]interface{}{"planId": "P2", "groupNumber": "G200", "certNumber": "C3"},
		},
	}
	output := runHL7Build(t, config, inputData)
	lines := hl7Lines(t, output, "hl7Message")
	want := []string{"IN1||P1", "IN2|G100", "IN3|C1", "IN3|C2", "IN1||P2", "IN2|G200", "IN3|C3"}
	if len(lines) != len(want)+1 {
		t.Fatalf("expected MSH + %d lines, got %d: %v", len(want), len(lines), lines)
	}
	for i, w := range want {
		if lines[i+1] != w {
			t.Errorf("line %d = %q, want %q (full: %v)", i+1, lines[i+1], w, lines)
		}
	}
}
