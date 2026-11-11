// services/hl7assembly/datatypes_repetitions_test.go
// XTN, XPN and XAD builders: one FHIR value per repetition, components read
// by their meaning. Synthetic values only.
package hl7assembly

import (
	"reflect"
	"testing"
)

func TestBuildContactPointsFromXTN(t *testing.T) {
	cases := []struct {
		name, in string
		want     []map[string]interface{}
	}{
		{"v2.5 composed number and an email repetition", "^PRN^PH^^1^555^0100999~^NET^Internet^petra@example.net",
			[]map[string]interface{}{
				{"system": "phone", "use": "home", "value": "+1 555 0100999"},
				{"system": "email", "value": "petra@example.net"},
			}},
		{"XTN.1 number", "(555)010-0123^WPN^PH", []map[string]interface{}{{"system": "phone", "use": "work", "value": "(555)010-0123"}}},
		{"mobile", "^PRS^CP^^44^20^79460000^12", []map[string]interface{}{{"system": "phone", "use": "mobile", "value": "+44 20 79460000 x12"}}},
		{"unformatted XTN.12", "^PRN^PH^^^^^^^^^5550100777", []map[string]interface{}{{"system": "phone", "use": "home", "value": "5550100777"}}},
		{"fax", "^WPN^FX^^1^555^0100555", []map[string]interface{}{{"system": "fax", "use": "work", "value": "+1 555 0100555"}}},
		{"plain email", "someone@example.org", []map[string]interface{}{{"system": "email", "use": "home", "value": "someone@example.org"}}},
		{"plain number", "5550100", []map[string]interface{}{{"system": "phone", "use": "home", "value": "5550100"}}},
		{"cellular, no use code", "^^CP^^1^555^0100333", []map[string]interface{}{{"system": "phone", "use": "mobile", "value": "+1 555 0100333"}}},
		{"unmapped use code ORN sets no use", "^ORN^PH^^44^20^79460000", []map[string]interface{}{{"system": "phone", "value": "+44 20 79460000"}}},
		{"TTY is other", "^PRN^TTY^^1^555^0100444", []map[string]interface{}{{"system": "other", "use": "home", "value": "+1 555 0100444"}}},
		{"no equipment type, address in XTN.4", "^^^ann@example.net", []map[string]interface{}{{"system": "email", "use": "home", "value": "ann@example.net"}}},
		{"use and type only: nothing to send", "^PRN^PH", nil},
		{"empty repetitions skipped", "~~^PRN^PH^^1^555^0100999~", []map[string]interface{}{{"system": "phone", "use": "home", "value": "+1 555 0100999"}}},
	}
	for _, c := range cases {
		got := BuildContactPointsFromXTN(c.in, nil)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v\nwant %v", c.name, got, c.want)
		}
	}
	// A component is never a contact point of its own.
	for _, cp := range BuildContactPointsFromXTN("^PRN^PH^^1^555^0100999", nil) {
		if v := cp["value"]; v == "PRN" || v == "PH" || v == "1" || v == "555" {
			t.Fatalf("component emitted as a contact point: %v", cp)
		}
	}
}

func TestBuildHumanNamesFromXPN(t *testing.T) {
	got := BuildHumanNamesFromXPN("Synthwell^Petra^Jo^^^^L~Brightwell^Petra^^^^^M~^^^^^^A", nil)
	want := []map[string]interface{}{
		{"use": "official", "family": "Synthwell", "given": []string{"Petra", "Jo"}},
		{"use": "maiden", "family": "Brightwell", "given": []string{"Petra"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v\nwant %v", got, want)
	}
	// No name type code: official, as before.
	if n := BuildHumanNameFromXPN("Doe^Jane", nil); n["use"] != "official" {
		t.Fatalf("default use %v", n["use"])
	}
	// Rules still win over XPN.7.
	if n := BuildHumanNameFromXPN("Doe^Jane^^^^^M", map[string]interface{}{"use": "usual"}); n["use"] != "usual" {
		t.Fatalf("rules not applied: %v", n["use"])
	}
}

func TestNameUseFromCode(t *testing.T) {
	for in, want := range map[string]string{
		"L": "official", "official": "official", "M": "maiden", "maiden": "maiden",
		"D": "usual", "N": "nickname", "S": "anonymous", "old": "old", "zz": "usual",
	} {
		if got := NameUseFromCode(in); got != want {
			t.Errorf("NameUseFromCode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBuildAddressesFromXAD(t *testing.T) {
	got := BuildAddressesFromXAD("12 Synthetic Rd^Unit 2^Springfield^MA^01101^USA^H~~1 Work Pl^^Shelbyville^MA^01102^USA^B", nil)
	if len(got) != 2 || got[0]["city"] != "Springfield" || got[1]["city"] != "Shelbyville" {
		t.Fatalf("got %v", got)
	}
}
