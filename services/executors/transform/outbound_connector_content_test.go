package transform

import (
	"encoding/json"
	"strings"
	"testing"
)

// Regression (v26.10 UAT): the shipped CDA -> FHIR template builds a FHIR Bundle, but the delivery step sent the
// ORIGINAL CDA XML. resolveAndSerializeContent inferred the output format from the INPUT message's _format ("cda")
// and, with no top-level contentType on the step, serialised the selected Bundle straight back to CDA. When a step
// explicitly selects a FHIR resource with contentField, the content must decide the wire format.

func cdaInput(extra map[string]interface{}) map[string]interface{} {
	msg := map[string]interface{}{
		"_format": "cda",
		"raw":     "<ClinicalDocument>original</ClinicalDocument>",
	}
	for k, v := range extra {
		msg[k] = v
	}
	return map[string]interface{}{"message": msg}
}

func TestResolveAndSerializeContent_ExplicitFHIRBundleFromCDAInput_IsDeliveredAsFHIRJSON(t *testing.T) {
	bundle := map[string]interface{}{"resourceType": "Bundle", "type": "collection",
		"entry": []interface{}{map[string]interface{}{"resource": map[string]interface{}{"resourceType": "Patient", "id": "p1"}}}}
	body, ct := resolveAndSerializeContent(cdaInput(map[string]interface{}{"fhirBundle": bundle}),
		outboundConnectorConfig{ContentField: "fhirBundle", ContentType: "application/json"})

	if strings.Contains(body, "ClinicalDocument") || ct == "application/hl7-cda+xml" {
		t.Fatalf("delivered the CDA document instead of the FHIR Bundle (ct=%q body=%.80q)", ct, body)
	}
	if ct != "application/fhir+json" {
		t.Errorf("content type = %q, want application/fhir+json", ct)
	}
	var got map[string]interface{}
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if got["resourceType"] != "Bundle" {
		t.Errorf("resourceType = %v, want Bundle", got["resourceType"])
	}
}

func TestResolveAndSerializeContent_ExplicitContentTypeStillWins(t *testing.T) {
	// A user who explicitly asks for CDA output keeps getting it: only an UNSET/generic type lets the content decide.
	bundle := map[string]interface{}{"resourceType": "Bundle", "entry": []interface{}{}}
	_, ct := resolveAndSerializeContent(cdaInput(map[string]interface{}{"fhirBundle": bundle}),
		outboundConnectorConfig{ContentField: "fhirBundle", ContentType: "application/fhir+json"})
	if ct != "application/fhir+json" {
		t.Errorf("explicit application/fhir+json: content type = %q", ct)
	}
}

func TestResolveAndSerializeContent_StringContentIsStillSentAsIs(t *testing.T) {
	body, _ := resolveAndSerializeContent(cdaInput(map[string]interface{}{"payload": "{\"resourceType\":\"Bundle\"}"}),
		outboundConnectorConfig{ContentField: "payload", ContentType: "application/fhir+json"})
	if body != "{\"resourceType\":\"Bundle\"}" {
		t.Errorf("string content was altered: %q", body)
	}
}

func TestIsFHIRResource(t *testing.T) {
	cases := []struct {
		name string
		in   interface{}
		want bool
	}{
		{"bundle", map[string]interface{}{"resourceType": "Bundle"}, true},
		{"patient", map[string]interface{}{"resourceType": "Patient"}, true},
		{"empty resourceType", map[string]interface{}{"resourceType": ""}, false},
		{"non-string resourceType", map[string]interface{}{"resourceType": 5}, false},
		{"plain map", map[string]interface{}{"a": 1}, false},
		{"string", "Bundle", false},
		{"nil", nil, false},
	}
	for _, c := range cases {
		if got := isFHIRResource(c.in); got != c.want {
			t.Errorf("%s: isFHIRResource = %v, want %v", c.name, got, c.want)
		}
	}
}
