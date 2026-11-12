// services/executors/transform/hl7_build_query_answer_test.go
// Tests for the hl7.build options used to answer a query: mshFields,
// segments[].rawPath, fields[].repetition, encoded values, component
// conditions and derivedValues. Synthetic values only.
//
//   TC-HB-Q01  mshFields set MSH-3..6 and MSH-21 from data; MSH-1/2 fixed
//   TC-HB-Q02  rawPath copies a segment; a wrong or injected one is dropped
//   TC-HB-Q03  repetition 2 of PID.5.7 gives "~^^^^^^S"
//   TC-HB-Q04  encoded values keep their encoding but can't add a field
//   TC-HB-Q05  derivedValues: first matching rule, default, chaining, and
//              use in conditions and sourcePaths
//   TC-HB-Q06  component conditions
//   TC-HB-Q07  configs without these options build exactly as before
package transform

import (
	"strings"
	"testing"
)

func rspConfig(extra map[string]interface{}) map[string]interface{} {
	cfg := map[string]interface{}{"messageType": "RSP", "triggerEvent": "K23", "messageStructure": "RSP_K23", "version": "2.5"}
	for k, v := range extra {
		cfg[k] = v
	}
	return cfg
}

func lineFor(lines []string, name string) string {
	for _, l := range lines {
		if l == name || strings.HasPrefix(l, name+"|") {
			return l
		}
	}
	return ""
}

func TestHL7Build_MSHFields(t *testing.T) {
	initHL7Schema(t)
	q := map[string]interface{}{"receiving_application": "PIXMGR^1.2.3^ISO", "receiving_facility": "EZIS",
		"sending_application": "PIXCONSUMER", "sending_facility": "SYNTH_HOSP"}
	cfg := rspConfig(map[string]interface{}{
		"escapeValues": true,
		"mshFields": []interface{}{
			map[string]interface{}{"fieldKey": "MSH.3", "sourcePath": "q.receiving_application", "encoded": true, "literalValue": "EZHK"},
			map[string]interface{}{"fieldKey": "MSH.4", "sourcePath": "q.receiving_facility", "encoded": true},
			map[string]interface{}{"fieldKey": "MSH.5", "sourcePath": "q.sending_application", "encoded": true},
			map[string]interface{}{"fieldKey": "MSH.6", "sourcePath": "q.sending_facility", "encoded": true},
			map[string]interface{}{"fieldKey": "MSH.21", "literalValue": "ITI-9^IHE"},
			map[string]interface{}{"fieldKey": "MSH.2", "literalValue": "XXXX"},
		},
	})
	msh := hl7Lines(t, runHL7Build(t, cfg, map[string]interface{}{"q": q}), "hl7Message")[0]
	f := strings.Split(msh, "|")
	if f[1] != `^~\&` || f[2] != "PIXMGR^1.2.3^ISO" || f[3] != "EZIS" || f[4] != "PIXCONSUMER" || f[5] != "SYNTH_HOSP" {
		t.Fatalf("MSH %q", msh)
	}
	if f[8] != "RSP^K23^RSP_K23" || f[11] != "2.5" || len(f) != 21 || f[20] != "ITI-9^IHE" {
		t.Fatalf("MSH %q (%d fields)", msh, len(f))
	}
	// A missing value falls back to the literal.
	msh = hl7Lines(t, runHL7Build(t, cfg, map[string]interface{}{"q": map[string]interface{}{}}), "hl7Message")[0]
	if strings.Split(msh, "|")[2] != "EZHK" {
		t.Fatalf("fallback MSH %q", msh)
	}
}

func TestHL7Build_RawSegment(t *testing.T) {
	initHL7Schema(t)
	cfg := rspConfig(map[string]interface{}{"segments": []interface{}{
		map[string]interface{}{"segment": "QAK", "fields": []interface{}{map[string]interface{}{"fieldKey": "QAK.1", "literalValue": "T"}}},
		map[string]interface{}{"segment": "QPD", "rawPath": "q.qpd"},
	}})
	for _, c := range []struct{ qpd, want string }{
		{"QPD|IHE PIX Query|T|000123^^^HOSPA&1.2.3&ISO", "QPD|IHE PIX Query|T|000123^^^HOSPA&1.2.3&ISO"},
		{"QPD|IHE PIX Query|T\rPID|||INJECTED", "QPD|IHE PIX Query|T"},
		{"PID|||NOT A QPD", ""},
		{"", ""},
	} {
		lines := hl7Lines(t, runHL7Build(t, cfg, map[string]interface{}{"q": map[string]interface{}{"qpd": c.qpd}}), "hl7Message")
		if got := lineFor(lines, "QPD"); got != c.want {
			t.Fatalf("QPD from %q = %q, want %q", c.qpd, got, c.want)
		}
		if lineFor(lines, "PID") != "" {
			t.Fatalf("a segment was injected: %q", lines)
		}
	}
}

func TestHL7Build_Repetition(t *testing.T) {
	initHL7Schema(t)
	cfg := rspConfig(map[string]interface{}{"segments": []interface{}{
		map[string]interface{}{"segment": "PID", "fields": []interface{}{
			map[string]interface{}{"fieldKey": "PID.3.1", "literalValue": "000123"},
			map[string]interface{}{"fieldKey": "PID.5.7", "literalValue": "S", "repetition": 2},
		}},
	}})
	if got := lineFor(hl7Lines(t, runHL7Build(t, cfg, map[string]interface{}{}), "hl7Message"), "PID"); got != "PID|||000123||~^^^^^^S" {
		t.Fatalf("PID %q", got)
	}
}

func TestHL7Build_EncodedValues(t *testing.T) {
	initHL7Schema(t)
	cfg := rspConfig(map[string]interface{}{"escapeValues": true, "segments": []interface{}{
		map[string]interface{}{"segment": "MSA", "fields": []interface{}{
			map[string]interface{}{"fieldKey": "MSA.1", "literalValue": "AA"},
			map[string]interface{}{"fieldKey": "MSA.2", "sourcePath": "id", "encoded": true},
		}},
		map[string]interface{}{"segment": "QAK", "fields": []interface{}{
			map[string]interface{}{"fieldKey": "QAK.1", "sourcePath": "id"},
		}},
	}})
	lines := hl7Lines(t, runHL7Build(t, cfg, map[string]interface{}{"id": "A\\S\\B|C\rD"}), "hl7Message")
	if got := lineFor(lines, "MSA"); got != `MSA|AA|A\S\B\F\CD` {
		t.Fatalf("MSA %q", got)
	}
	// Not encoded: escaped as data.
	if got := lineFor(lines, "QAK"); got != `QAK|A\E\S\E\B\F\C\X0D\D` {
		t.Fatalf("QAK %q", got)
	}
}

func pixAnswerConfig() map[string]interface{} {
	status := func(op string, v interface{}) map[string]interface{} {
		return map[string]interface{}{"field": "api.http_status", "operator": op, "value": v}
	}
	return rspConfig(map[string]interface{}{
		"derivedValues": []interface{}{
			map[string]interface{}{"name": "ack", "default": "AE", "rules": []interface{}{
				map[string]interface{}{"condition": map[string]interface{}{"field": "q.has_problems", "operator": "equals", "value": true}, "value": "AE"},
				map[string]interface{}{"condition": status("equals", 200), "value": "AA"},
			}},
			map[string]interface{}{"name": "status", "rules": []interface{}{
				map[string]interface{}{"condition": map[string]interface{}{"field": "derived.ack", "operator": "equals", "value": "AE"}, "value": "AE"},
				map[string]interface{}{"condition": map[string]interface{}{"field": "api.ids", "operator": "exists"}, "value": "OK"},
			}, "default": "NF"},
			map[string]interface{}{"name": "tag", "rules": []interface{}{
				map[string]interface{}{"sourcePath": "q.tag"},
			}},
			map[string]interface{}{"name": "unused"},
		},
		"segments": []interface{}{
			map[string]interface{}{"segment": "MSA", "fields": []interface{}{
				map[string]interface{}{"fieldKey": "MSA.1", "sourcePath": "derived.ack"},
			}},
			map[string]interface{}{"segment": "QAK", "fields": []interface{}{
				map[string]interface{}{"fieldKey": "QAK.1", "sourcePath": "derived.tag"},
				map[string]interface{}{"fieldKey": "QAK.2", "sourcePath": "derived.status"},
			}},
			map[string]interface{}{"segment": "PID", "condition": map[string]interface{}{"field": "derived.status", "operator": "equals", "value": "OK"},
				"fields": []interface{}{map[string]interface{}{"fieldKey": "PID.3", "repeatPath": "api.ids", "components": []interface{}{
					map[string]interface{}{"position": "1", "sourcePath": "value"},
					map[string]interface{}{"position": "4.2", "sourcePath": "system", "transform": "hl7_universal_id"},
					map[string]interface{}{"position": "4.3", "sourcePath": "system", "transform": "hl7_universal_id_type"},
					map[string]interface{}{"position": "5", "literalValue": "MR",
						"condition": map[string]interface{}{"field": "system", "operator": "starts_with", "value": "urn:oid:"}},
				}}}},
		},
	})
}

func TestHL7Build_DerivedValues(t *testing.T) {
	initHL7Schema(t)
	ids := []interface{}{
		map[string]interface{}{"system": "urn:oid:1.2.3", "value": "A1"},
		map[string]interface{}{"system": "https://empi.example.org/eid", "value": "E9"},
	}
	cases := []struct {
		name string
		data map[string]interface{}
		msa  string
		qak  string
		pid  string
	}{
		{"found", map[string]interface{}{"q": map[string]interface{}{"tag": "T1", "has_problems": false},
			"api": map[string]interface{}{"http_status": 200, "ids": ids}},
			"MSA|AA", "QAK|T1|OK", "PID|||A1^^^&1.2.3&ISO^MR~E9^^^&https://empi.example.org/eid&URI"},
		{"no ids", map[string]interface{}{"q": map[string]interface{}{"tag": "T2", "has_problems": false},
			"api": map[string]interface{}{"http_status": 200}}, "MSA|AA", "QAK|T2|NF", ""},
		{"not found", map[string]interface{}{"q": map[string]interface{}{"tag": "T3", "has_problems": false},
			"api": map[string]interface{}{"http_status": 404}}, "MSA|AE", "QAK|T3|AE", ""},
		{"problems first", map[string]interface{}{"q": map[string]interface{}{"tag": "T4", "has_problems": true},
			"api": map[string]interface{}{"http_status": 200, "ids": ids}}, "MSA|AE", "QAK|T4|AE", ""},
		{"no answer", map[string]interface{}{"q": map[string]interface{}{"tag": "T5"}}, "MSA|AE", "QAK|T5|AE", ""},
	}
	for _, c := range cases {
		lines := hl7Lines(t, runHL7Build(t, pixAnswerConfig(), c.data), "hl7Message")
		if lineFor(lines, "MSA") != c.msa || lineFor(lines, "QAK") != c.qak || lineFor(lines, "PID") != c.pid {
			t.Fatalf("%s: %q", c.name, lines)
		}
	}
	// Derived values don't leak into the step's data.
	out := runHL7Build(t, pixAnswerConfig(), map[string]interface{}{"q": map[string]interface{}{"tag": "T"}})
	if _, leaked := out["derived"]; leaked {
		t.Fatal("derived values written to the output data")
	}
}

func TestHL7Build_QueryOptionsOffByDefault(t *testing.T) {
	initHL7Schema(t)
	cfg := map[string]interface{}{"messageType": "ADT", "triggerEvent": "A31", "version": "2.5.1",
		"sendingApplication": "APP", "segments": []interface{}{
			map[string]interface{}{"segment": "PID", "fields": []interface{}{
				map[string]interface{}{"fieldKey": "PID.3.1", "sourcePath": "id"},
				map[string]interface{}{"fieldKey": "PID.5.1", "sourcePath": "family", "valueMap": map[string]interface{}{"X": ""}},
			}},
		}}
	lines := hl7Lines(t, runHL7Build(t, cfg, map[string]interface{}{"id": "A^B", "family": "X"}), "hl7Message")
	f := strings.Split(lines[0], "|")
	if f[2] != "APP" || len(f) != 12 {
		t.Fatalf("MSH changed: %q", lines[0])
	}
	if got := lineFor(lines, "PID"); got != "PID|||A^B||" {
		t.Fatalf("PID %q", got)
	}
}
