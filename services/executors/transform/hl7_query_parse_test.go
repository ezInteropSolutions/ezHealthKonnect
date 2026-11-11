// services/executors/transform/hl7_query_parse_test.go
// Tests for hl7.query_parse and the received-message reader behind it.
// Synthetic values only.
//
//   TC-HQ-001  PIX: identifier by universal ID, domains, the API query
//   TC-HQ-002  PIX: identifier by namespace through the namespaces table
//   TC-HQ-003  PIX: unknown authority / missing ID / unknown domain are
//              problems with IHE's ERR locations, not failures
//   TC-HQ-004  PDQ: @PID parameters mapped, transformed, value-mapped; the
//              identifier takes its system from @PID.3.4; quantity -> _count
//   TC-HQ-005  PDQ: unsupported parameter, unreadable date, identifier with
//              no authority, no parameters at all
//   TC-HQ-006  continuation pointer and QCN's tag in QID-1
//   TC-HQ-007  other delimiters are re-encoded to the standard ones (echo)
//   TC-HQ-008  escaped values are decoded for the search, kept for echo
//   TC-HQ-009  a body that isn't HL7 fails the step
package transform

import (
	"context"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"ezhealthkonnect/models"
)

func runQueryParse(t *testing.T, cfg map[string]interface{}, raw string) map[string]interface{} {
	t.Helper()
	step := &models.TransformationStep{StepName: "Query", StepType: "hl7.query_parse", Enabled: true, Config: cfg}
	out, err := NewHL7QueryParseExecutor().Execute(context.Background(), step, map[string]interface{}{"raw": raw})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	so, _ := out["_stepOutput"].(map[string]interface{})
	if so == nil {
		t.Fatal("no step output")
	}
	return so
}

func seg(lines ...string) string { return strings.Join(lines, "\r") + "\r" }

const pixMSH = "MSH|^~\\&|PIXCONSUMER|SYNTH_HOSP|PIXMGR|EZIS|20261111093000||QBP^Q23^QBP_Q21|CTRL-1|P|2.5"

func pixConfig() map[string]interface{} {
	return map[string]interface{}{
		"identifierField": "QPD.3", "identifierParam": "sourceIdentifier",
		"domainsField": "QPD.4", "domainsParam": "targetSystem",
		"namespaces": map[string]interface{}{"HOSPA": "1.2.840.99999.1", "EID": "https://empi.example.org/eid"},
	}
}

func problemsOf(so map[string]interface{}) []map[string]interface{} {
	var out []map[string]interface{}
	for _, p := range so["problems"].([]interface{}) {
		out = append(out, p.(map[string]interface{}))
	}
	return out
}

func TestQueryParse_PIXByUniversalID(t *testing.T) {
	so := runQueryParse(t, pixConfig(), seg(pixMSH,
		"QPD|IHE PIX Query|TAG-1|000123^^^HOSPA&1.2.840.99999.1&ISO|^^^EID~^^^&2.16.840.99999.7&ISO",
		"RCP|I"))
	if so["query_tag"] != "TAG-1" || so["control_id"] != "CTRL-1" || so["trigger_event"] != "Q23" {
		t.Fatalf("header %v %v %v", so["query_tag"], so["control_id"], so["trigger_event"])
	}
	if so["receiving_application"] != "PIXMGR" || so["sending_facility"] != "SYNTH_HOSP" {
		t.Fatalf("MSH values %v %v", so["receiving_application"], so["sending_facility"])
	}
	if !strings.HasPrefix(so["qpd_segment"].(string), "QPD|IHE PIX Query|TAG-1|") {
		t.Fatalf("qpd %q", so["qpd_segment"])
	}
	id := so["identifier"].(map[string]interface{})
	if id["system"] != "urn:oid:1.2.840.99999.1" || id["value"] != "000123" {
		t.Fatalf("identifier %v", id)
	}
	if so["has_problems"] != false {
		t.Fatalf("problems %v", so["problems"])
	}
	want := url.Values{
		"sourceIdentifier": {"urn:oid:1.2.840.99999.1|000123"},
		"targetSystem":     {"https://empi.example.org/eid", "urn:oid:2.16.840.99999.7"},
	}
	got, _ := url.ParseQuery(so["query"].(string))
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("query %v\nwant %v", got, want)
	}
	if so["domain_count"] != 2 {
		t.Fatalf("domain_count %v", so["domain_count"])
	}
}

func TestQueryParse_PIXByNamespace(t *testing.T) {
	so := runQueryParse(t, pixConfig(), seg(pixMSH, "QPD|IHE PIX Query|TAG-2|000123^^^HOSPA"))
	id := so["identifier"].(map[string]interface{})
	if id["system"] != "urn:oid:1.2.840.99999.1" || id["universal_id"] != "1.2.840.99999.1" || id["universal_id_type"] != "ISO" {
		t.Fatalf("identifier %v", id)
	}
	got, _ := url.ParseQuery(so["query"].(string))
	if got.Get("sourceIdentifier") != "urn:oid:1.2.840.99999.1|000123" || got.Has("targetSystem") {
		t.Fatalf("query %v", got)
	}
}

func TestQueryParse_PIXProblems(t *testing.T) {
	cases := []struct {
		qpd       string
		locations []string
		code      string
	}{
		{"QPD|IHE PIX Query|T|000123^^^NOPE", []string{"QPD^1^3^1^4"}, hl7ErrUnknownKey},
		{"QPD|IHE PIX Query|T|^^^HOSPA", []string{"QPD^1^3^1^1"}, hl7ErrRequiredMissing},
		{"QPD|IHE PIX Query|T|000123^^^HOSPA|^^^EID~^^^NOPE~^^^ALSO_NOPE", []string{"QPD^1^4^2", "QPD^1^4^3"}, hl7ErrUnknownKey},
	}
	for _, c := range cases {
		so := runQueryParse(t, pixConfig(), seg(pixMSH, c.qpd))
		ps := problemsOf(so)
		if len(ps) != len(c.locations) || so["has_problems"] != true || so["problem_count"] != len(c.locations) {
			t.Fatalf("%s: problems %v", c.qpd, ps)
		}
		for i, p := range ps {
			if p["location"] != c.locations[i] || p["code"] != c.code || p["text"] == "" {
				t.Fatalf("%s: problem %d = %v", c.qpd, i, p)
			}
		}
	}
}

func pdqConfig() map[string]interface{} {
	return map[string]interface{}{
		"parametersField": "QPD.3", "domainsField": "QPD.8",
		"countParam": "_count", "defaultCount": 10, "quantityField": "RCP.2", "continuationField": "DSC.1",
		"namespaces": map[string]interface{}{"HOSPA": "1.2.840.99999.1"},
		"parameters": []interface{}{
			map[string]interface{}{"field": "PID.5.1.1", "param": "family"},
			map[string]interface{}{"field": "PID.5.1", "param": "family"},
			map[string]interface{}{"field": "PID.5.2", "param": "given"},
			map[string]interface{}{"field": "PID.7", "param": "birthdate", "transform": "fhir_date"},
			map[string]interface{}{"field": "PID.8", "param": "gender", "valueMap": map[string]interface{}{"M": "male", "F": "female"}},
			map[string]interface{}{"field": "PID.11.6", "param": "address-country", "transform": "iso3166_alpha2"},
			map[string]interface{}{"field": "PID.3.1", "param": "identifier", "domainField": "PID.3.4"},
		},
	}
}

const pdqMSH = "MSH|^~\\&|PDQCONSUMER|SYNTH_HOSP|PDQSUPPLIER|EZIS|20261111093000||QBP^Q22^QBP_Q21|CTRL-9|P|2.5"

func TestQueryParse_PDQParameters(t *testing.T) {
	so := runQueryParse(t, pdqConfig(), seg(pdqMSH,
		"QPD|IHE PDQ Query|Q-77|@PID.5.1.1^SYNTHWELL*~@PID.5.2^Petra~@PID.7^19870314~@PID.8^F~@PID.11.6^USA~@PID.3.1^000123~@PID.3.4.1^HOSPA|||||^^^HOSPA",
		"RCP|I|5^RD"))
	got, _ := url.ParseQuery(so["query"].(string))
	want := url.Values{
		"family": {"SYNTHWELL"}, "given": {"Petra"}, "birthdate": {"1987-03-14"}, "gender": {"female"},
		"address-country": {"US"}, "identifier": {"urn:oid:1.2.840.99999.1|000123"}, "_count": {"5"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("query %v\nwant %v", got, want)
	}
	if so["has_problems"] != false || so["quantity"] != 5 {
		t.Fatalf("problems %v quantity %v", so["problems"], so["quantity"])
	}
	if ds := so["domain_systems"].([]interface{}); len(ds) != 1 || ds[0] != "urn:oid:1.2.840.99999.1" {
		t.Fatalf("domains %v", ds)
	}
	// A whole HD in one parameter works too.
	so = runQueryParse(t, pdqConfig(), seg(pdqMSH, "QPD|IHE PDQ Query|Q-78|@PID.3.1^000123~@PID.3.4^&2.16.840.99999.3&ISO"))
	if got, _ := url.ParseQuery(so["query"].(string)); got.Get("identifier") != "urn:oid:2.16.840.99999.3|000123" || got.Get("_count") != "10" {
		t.Fatalf("query %v", got)
	}
}

func TestQueryParse_PDQProblems(t *testing.T) {
	cases := []struct {
		qpd, location, code string
	}{
		{"QPD|IHE PDQ Query|Q|@PID.19^123-45-6789", "QPD^1^3^1", hl7ErrInternal},
		{"QPD|IHE PDQ Query|Q|@PID.5.1.1^X~@PID.7^notadate", "QPD^1^3^2", hl7ErrDataType},
		{"QPD|IHE PDQ Query|Q|@PID.3.1^000123", "QPD^1^3^1", hl7ErrUnknownKey},
		{"QPD|IHE PDQ Query|Q|", "QPD^1^3", hl7ErrRequiredMissing},
		{"QPD|IHE PDQ Query|Q|@PID.5.1.1^X|||||^^^NOPE", "QPD^1^8^1", hl7ErrUnknownKey},
	}
	for _, c := range cases {
		ps := problemsOf(runQueryParse(t, pdqConfig(), seg(pdqMSH, c.qpd)))
		if len(ps) != 1 || ps[0]["location"] != c.location || ps[0]["code"] != c.code {
			t.Fatalf("%s: problems %v", c.qpd, ps)
		}
	}
	// The unsupported parameter is never sent: the search isn't widened.
	so := runQueryParse(t, pdqConfig(), seg(pdqMSH, "QPD|IHE PDQ Query|Q|@PID.19^123-45-6789~@PID.5.1.1^X"))
	if q := so["query"].(string); strings.Contains(q, "123-45") {
		t.Fatalf("unsupported parameter sent: %s", q)
	}
}

func TestQueryParse_ContinuationAndCancel(t *testing.T) {
	so := runQueryParse(t, pdqConfig(), seg(pdqMSH, "QPD|IHE PDQ Query|Q-77|@PID.5.1.1^X", "RCP|I|5^RD", "DSC|abc123|I"))
	if so["continuation"] != "abc123" {
		t.Fatalf("continuation %v", so["continuation"])
	}
	so = runQueryParse(t, map[string]interface{}{"tagField": "QID.1"}, seg(
		"MSH|^~\\&|PDQCONSUMER|SYNTH_HOSP|PDQSUPPLIER|EZIS|20261111093000||QCN^J01^QCN_J01|CTRL-10|P|2.5",
		"QID|Q-77|IHE PDQ Query"))
	if so["query_tag"] != "Q-77" || so["message_type"] != "QCN" || so["trigger_event"] != "J01" {
		t.Fatalf("cancel %v %v %v", so["query_tag"], so["message_type"], so["trigger_event"])
	}
}

func TestQueryParse_ReencodesOtherDelimiters(t *testing.T) {
	// "#" fields, "$" components, "%" repetitions, "*" escape, "@" subcomponents;
	// a literal "^" in a value must come out escaped.
	raw := seg("MSH#$%*@#PIXCONSUMER#SYNTH_HOSP#PIXMGR#EZIS#20261111093000##QBP$Q23$QBP_Q21#CTRL-2#P#2.5",
		"QPD#IHE PIX Query#TAG^3#000123$$$HOSPA@1.2.840.99999.1@ISO#$$$EID%$$$@2.16.840.99999.7@ISO")
	so := runQueryParse(t, pixConfig(), raw)
	if so["qpd_segment"] != `QPD|IHE PIX Query|TAG\S\3|000123^^^HOSPA&1.2.840.99999.1&ISO|^^^EID~^^^&2.16.840.99999.7&ISO` {
		t.Fatalf("qpd %q", so["qpd_segment"])
	}
	if so["query_tag"] != `TAG\S\3` || so["trigger_event"] != "Q23" {
		t.Fatalf("tag %v trigger %v", so["query_tag"], so["trigger_event"])
	}
	if id := so["identifier"].(map[string]interface{}); id["system"] != "urn:oid:1.2.840.99999.1" {
		t.Fatalf("identifier %v", id)
	}
}

func TestQueryParse_EscapedValues(t *testing.T) {
	so := runQueryParse(t, pdqConfig(), seg(pdqMSH, `QPD|IHE PDQ Query|Q\T\1|@PID.5.1.1^O\S\BRIEN~@PID.5.2^Ann\X41\`))
	got, _ := url.ParseQuery(so["query"].(string))
	if got.Get("family") != "O^BRIEN" || got.Get("given") != "AnnA" {
		t.Fatalf("decoded %v", got)
	}
	if so["query_tag"] != `Q\T\1` {
		t.Fatalf("tag kept encoded: %v", so["query_tag"])
	}
}

func TestQueryParse_NotHL7(t *testing.T) {
	step := &models.TransformationStep{StepName: "Query", StepType: "hl7.query_parse", Enabled: true, Config: map[string]interface{}{}}
	if _, err := NewHL7QueryParseExecutor().Execute(context.Background(), step, map[string]interface{}{"raw": `{"a":1}`}); err == nil {
		t.Fatal("a non-HL7 body was read")
	}
}

func TestUnescapeHL7(t *testing.T) {
	for in, want := range map[string]string{
		`a\F\b\S\c\T\d\R\e\E\f`: `a|b^c&d~e\f`,
		`x\.br\y`:              "xy",
		`trailing\`:            `trailing\`,
		`\X4142\`:              "AB",
		"plain":                "plain",
	} {
		if got := unescapeHL7(in); got != want {
			t.Errorf("unescapeHL7(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestQueryTransforms(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"hl7_universal_id", "urn:oid:1.2.840.99999.1", "1.2.840.99999.1"},
		{"hl7_universal_id", "urn:uuid:0f8fad5b-d9cb-469f-a165-70867728950e", "0f8fad5b-d9cb-469f-a165-70867728950e"},
		{"hl7_universal_id", "https://empi.example.org/eid", "https://empi.example.org/eid"},
		{"hl7_universal_id_type", "urn:oid:1.2.840.99999.1", "ISO"},
		{"hl7_universal_id_type", "urn:uuid:0f8fad5b-d9cb-469f-a165-70867728950e", "UUID"},
		{"hl7_universal_id_type", "https://empi.example.org/eid", "URI"},
		{"fhir_date", "19870314", "1987-03-14"},
		{"fhir_date", "198703", "1987-03"},
		{"fhir_date", "1987", "1987"},
		{"fhir_date", "19870314093000-0500", "1987-03-14"},
		{"iso3166_alpha2", "USA", "US"},
		{"iso3166_alpha2", "gb", "GB"},
	}
	for _, c := range cases {
		got, ok, known := applyHL7Transform(c.name, c.in)
		if !known || !ok || got != c.want {
			t.Errorf("%s(%q) = %q ok=%v known=%v, want %q", c.name, c.in, got, ok, known, c.want)
		}
	}
	for _, c := range []struct{ name, in string }{{"fhir_date", "March"}, {"iso3166_alpha2", "ZZZ"}, {"hl7_universal_id_type", ""}} {
		if _, ok, _ := applyHL7Transform(c.name, c.in); ok {
			t.Errorf("%s(%q) accepted", c.name, c.in)
		}
	}
}
