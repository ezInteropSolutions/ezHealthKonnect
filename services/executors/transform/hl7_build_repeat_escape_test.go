// services/executors/transform/hl7_build_repeat_escape_test.go
// Tests for hl7.build's repeating fields (repeatPath/components) and value
// escaping (escapeValues).
//
//   TC-HB-R01  repeatPath: one repetition per element, components by position
//   TC-HB-R02  itemCondition filters elements; maxRepetitions caps them
//   TC-HB-R03  scalar array without components repeats the values
//   TC-HB-R04  repeatPath on a component key is refused (no output)
//   TC-HB-R05  empty/missing array writes nothing (and a repeating segment
//              with nothing else skips)
//   TC-HB-E01  escapeValues escapes delimiters and line breaks from data
//   TC-HB-E02  escapeValues leaves literals as written
//   TC-HB-E03  without escapeValues, values pass through unchanged (compat)
//   TC-HB-E04  escaped values round-trip through the real parser
package transform

import (
	"strings"
	"testing"

	"ezhealthkonnect/hl7"
)

func pidLine(t *testing.T, lines []string) string {
	t.Helper()
	for _, l := range lines {
		if l == "PID" || strings.HasPrefix(l, "PID|") {
			return l
		}
	}
	t.Fatalf("no PID segment in %q", lines)
	return ""
}

func identifierPatient() map[string]interface{} {
	return map[string]interface{}{
		"patient": map[string]interface{}{
			"identifier": []interface{}{
				map[string]interface{}{"system": "urn:oid:1.2.3", "value": "EID-1", "use": "official"},
				map[string]interface{}{"system": "urn:oid:9.9", "value": "000123"},
				map[string]interface{}{"system": "urn:oid:8.8", "value": "777"},
			},
		},
	}
}

func repeatingPID3(extra map[string]interface{}) map[string]interface{} {
	field := map[string]interface{}{
		"fieldKey":   "PID.3",
		"repeatPath": "patient.identifier",
		"components": []interface{}{
			map[string]interface{}{"position": "1", "sourcePath": "value"},
			map[string]interface{}{"position": "4.2", "sourcePath": "system"},
			map[string]interface{}{"position": "4.3", "literalValue": "ISO"},
			map[string]interface{}{"position": "5", "literalValue": "MR"},
		},
	}
	for k, v := range extra {
		field[k] = v
	}
	return map[string]interface{}{
		"messageType": "ADT", "triggerEvent": "A31", "version": "2.5.1",
		"segments": []interface{}{
			map[string]interface{}{"segment": "PID", "fields": []interface{}{field}},
		},
	}
}

func TestHL7Build_RepeatingField_OneRepetitionPerElement(t *testing.T) {
	initHL7Schema(t)
	out := runHL7Build(t, repeatingPID3(nil), identifierPatient())
	got := pidLine(t, hl7Lines(t, out, "hl7Message"))
	want := "PID|||EID-1^^^&urn:oid:1.2.3&ISO^MR~000123^^^&urn:oid:9.9&ISO^MR~777^^^&urn:oid:8.8&ISO^MR"
	if got != want {
		t.Fatalf("PID =\n %q\nwant\n %q", got, want)
	}
}

func TestHL7Build_RepeatingField_ItemConditionAndMax(t *testing.T) {
	initHL7Schema(t)
	cfg := repeatingPID3(map[string]interface{}{
		"itemCondition":  map[string]interface{}{"field": "system", "operator": "not_equals", "value": "urn:oid:1.2.3"},
		"maxRepetitions": 1,
	})
	got := pidLine(t, hl7Lines(t, runHL7Build(t, cfg, identifierPatient()), "hl7Message"))
	if got != "PID|||000123^^^&urn:oid:9.9&ISO^MR" {
		t.Fatalf("PID = %q", got)
	}
}

func TestHL7Build_RepeatingField_ScalarArray(t *testing.T) {
	initHL7Schema(t)
	cfg := map[string]interface{}{
		"messageType": "ADT", "triggerEvent": "A31", "version": "2.5.1",
		"segments": []interface{}{map[string]interface{}{"segment": "PID", "fields": []interface{}{
			map[string]interface{}{"fieldKey": "PID.13", "repeatPath": "phones"},
		}}},
	}
	got := pidLine(t, hl7Lines(t, runHL7Build(t, cfg, map[string]interface{}{"phones": []interface{}{"555-0100", "555-0101"}}), "hl7Message"))
	if !strings.HasSuffix(got, "|555-0100~555-0101") {
		t.Fatalf("PID = %q", got)
	}
}

func TestHL7Build_RepeatingField_ComponentKeyRefused(t *testing.T) {
	initHL7Schema(t)
	cfg := map[string]interface{}{
		"messageType": "ADT", "triggerEvent": "A31", "version": "2.5.1",
		"segments": []interface{}{map[string]interface{}{"segment": "PID", "fields": []interface{}{
			map[string]interface{}{"fieldKey": "PID.3.1", "repeatPath": "patient.identifier",
				"components": []interface{}{map[string]interface{}{"position": "1", "sourcePath": "value"}}},
		}}},
	}
	got := pidLine(t, hl7Lines(t, runHL7Build(t, cfg, identifierPatient()), "hl7Message"))
	if got != "PID" {
		t.Fatalf("a component-level repeatPath wrote %q", got)
	}
}

func TestHL7Build_RepeatingField_EmptyArray(t *testing.T) {
	initHL7Schema(t)
	cfg := map[string]interface{}{
		"messageType": "ADT", "triggerEvent": "A31", "version": "2.5.1",
		"segments": []interface{}{
			map[string]interface{}{"segment": "PID", "fields": []interface{}{
				map[string]interface{}{"fieldKey": "PID.3", "repeatPath": "nothing.here",
					"components": []interface{}{map[string]interface{}{"position": "1", "sourcePath": "value"}}},
			}},
			map[string]interface{}{"segment": "NK1", "cardinality": "repeating", "rowsPath": "kin", "fields": []interface{}{
				map[string]interface{}{"fieldKey": "NK1.2", "repeatPath": "names",
					"components": []interface{}{map[string]interface{}{"position": "1", "sourcePath": "family"}}},
			}},
		},
	}
	lines := hl7Lines(t, runHL7Build(t, cfg, map[string]interface{}{"kin": []interface{}{map[string]interface{}{"names": []interface{}{}}}}), "hl7Message")
	if got := pidLine(t, lines); got != "PID" {
		t.Fatalf("PID = %q", got)
	}
	for _, l := range lines {
		if strings.HasPrefix(l, "NK1") {
			t.Fatalf("a repeating segment with no data was built: %q", l)
		}
	}
}

func escapeConfig(escape bool) map[string]interface{} {
	return map[string]interface{}{
		"messageType": "ADT", "triggerEvent": "A31", "version": "2.5.1", "escapeValues": escape,
		"segments": []interface{}{map[string]interface{}{"segment": "PID", "fields": []interface{}{
			map[string]interface{}{"fieldKey": "PID.5.1", "sourcePath": "family"},
			map[string]interface{}{"fieldKey": "PID.5.2", "sourcePath": "given"},
			map[string]interface{}{"fieldKey": "PID.8", "literalValue": "F^X"},
		}}},
	}
}

// A synthetic, deliberately hostile value: every delimiter, the escape
// character, and a line break that would otherwise start a forged segment.
var hostileName = map[string]interface{}{"family": "O|Brien^Jr~X&Y\\Z\rOBX|1|forged", "given": "Ann\nMarie"}

func TestHL7Build_EscapeValues_EscapesData(t *testing.T) {
	initHL7Schema(t)
	lines := hl7Lines(t, runHL7Build(t, escapeConfig(true), hostileName), "hl7Message")
	for _, l := range lines {
		if strings.HasPrefix(l, "OBX") {
			t.Fatalf("a line break in data forged a segment: %q", lines)
		}
	}
	got := pidLine(t, lines)
	want := `PID|||||O\F\Brien\S\Jr\R\X\T\Y\E\Z\X0D\OBX\F\1\F\forged^Ann\X0A\Marie|||F^X`
	if got != want {
		t.Fatalf("PID =\n %q\nwant\n %q", got, want)
	}
}

func TestHL7Build_EscapeValues_OffKeepsOldBehaviour(t *testing.T) {
	initHL7Schema(t)
	out := runHL7Build(t, escapeConfig(false), map[string]interface{}{"family": "A^B", "given": "C"})
	got := pidLine(t, hl7Lines(t, out, "hl7Message"))
	if got != "PID|||||A^B^C|||F^X" {
		t.Fatalf("PID = %q", got)
	}
}

func TestHL7Build_EscapeValues_RoundTrip(t *testing.T) {
	initHL7Schema(t)
	out := runHL7Build(t, escapeConfig(true), map[string]interface{}{"family": "O|Brien^Jr", "given": "Ann"})
	msg := out["hl7Message"].(string)
	if parsed := hl7.ParseWithRealSchema(msg); parsed == nil {
		t.Fatal("nil parse result")
	}
	pid := pidLine(t, hl7Lines(t, out, "hl7Message"))
	if fields := strings.Split(pid, "|"); len(fields) != 9 {
		t.Fatalf("escaped delimiters shifted PID's fields: %q", pid)
	}
}
