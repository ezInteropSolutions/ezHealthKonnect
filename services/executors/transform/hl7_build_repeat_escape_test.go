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
	"context"
	"strings"
	"testing"

	"ezhealthkonnect/hl7"
	"ezhealthkonnect/models"
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

func TestHL7Build_TriggerFromData(t *testing.T) {
	initHL7Schema(t)
	cfg := map[string]interface{}{
		"messageType": "ADT", "version": "2.5.1",
		"triggerEventPath":    "type",
		"triggerEventMap":     map[string]interface{}{"identity.merged": "A40", "identity.created": "A28"},
		"messageStructureMap": map[string]interface{}{"A40": "ADT_A39", "A28": "ADT_A05"},
		"segments":            []interface{}{map[string]interface{}{"segment": "EVN", "fields": []interface{}{map[string]interface{}{"fieldKey": "EVN.1", "literalValue": "x"}}}},
	}
	lines := hl7Lines(t, runHL7Build(t, cfg, map[string]interface{}{"type": "identity.merged"}), "hl7Message")
	if f := strings.Split(lines[0], "|"); f[8] != "ADT^A40^ADT_A39" {
		t.Fatalf("MSH.9 = %q", f[8])
	}

	step := NewHL7BuildExecutor()
	_, err := step.Execute(context.Background(), &models.TransformationStep{StepType: "hl7.build", Enabled: true, Config: cfg},
		map[string]interface{}{"type": "task.created"})
	if err == nil {
		t.Fatal("an unmapped trigger built a message")
	}
}

func TestHL7ValueTransforms(t *testing.T) {
	cases := []struct{ name, in, want string; ok bool }{
		{"hl7_date", "1987-03-14", "19870314", true},
		{"hl7_date", "1987-03", "198703", true},
		{"hl7_date", "1987", "1987", true},
		{"hl7_date", "1987-03-14T09:30:00Z", "19870314", true},
		{"hl7_date", "14/03/1987", "", false},
		{"hl7_datetime", "2026-11-10T11:11:33.452998Z", "20261110111133+0000", true},
		{"hl7_datetime", "2026-11-10T16:41:33+05:30", "20261110164133+0530", true},
		{"hl7_datetime", "2026-11-10", "20261110", true},
		{"hl7_datetime", "yesterday", "", false},
		{"uppercase", "male", "MALE", true},
		{"iso3166_alpha3", "US", "USA", true},
		{"iso3166_alpha3", "de", "DEU", true},
		{"iso3166_alpha3", "CN", "CHN", true},
		{"iso3166_alpha3", "GBR", "GBR", true},
		{"iso3166_alpha3", "EU", "", false},
		{"iso3166_alpha3", "Springfield", "", false},
	}
	for _, c := range cases {
		got, ok, known := applyHL7Transform(c.name, c.in)
		if !known || ok != c.ok || (ok && got != c.want) {
			t.Errorf("%s(%q) = %q,%v,%v want %q,%v", c.name, c.in, got, ok, known, c.want, c.ok)
		}
	}
	if _, _, known := applyHL7Transform("nope", "x"); known {
		t.Error("unknown transform reported as known")
	}
}

func TestHL7Build_TransformInMapping(t *testing.T) {
	initHL7Schema(t)
	cfg := map[string]interface{}{
		"messageType": "ADT", "triggerEvent": "A31", "version": "2.5.1",
		"segments": []interface{}{map[string]interface{}{"segment": "PID", "fields": []interface{}{
			map[string]interface{}{"fieldKey": "PID.7", "sourcePath": "birthDate", "transform": "hl7_date"},
			map[string]interface{}{"fieldKey": "PID.8", "sourcePath": "gender", "valueMap": map[string]interface{}{"female": "F"}},
		}}},
	}
	got := pidLine(t, hl7Lines(t, runHL7Build(t, cfg, map[string]interface{}{"birthDate": "2008-02-20", "gender": "female"}), "hl7Message"))
	if got != "PID|||||||20080220|F" {
		t.Fatalf("PID = %q", got)
	}
}
