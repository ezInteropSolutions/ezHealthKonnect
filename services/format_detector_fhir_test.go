package services

import (
	"testing"

	"ezhealthkonnect/models"
)

// FHIR is recognised by a top-level resourceType; JSON that only carries a
// resource inside it (an event naming a Patient) is plain JSON, which has a
// parser, rather than FHIR, which in ParserFactory has none.
func TestFormatDetector_FHIRByTopLevelResourceType(t *testing.T) {
	fd := NewFormatDetector()
	cases := []struct {
		name    string
		content string
		want    models.MessageFormat
	}{
		{"resource", `{"resourceType":"Patient","id":"p1"}`, models.FormatFHIR},
		{"resource, type not first", `{"id":"p1","meta":{"versionId":"1"},"resourceType":"Practitioner"}`, models.FormatFHIR},
		{"bundle with nested resources", `{"resourceType":"Bundle","entry":[{"resource":{"resourceType":"Patient"}}]}`, models.FormatFHIR},
		{"array of resources", `[{"resourceType":"Patient"},{"resourceType":"Observation"}]`, models.FormatFHIR},
		{"leading whitespace", "\n  {\"resourceType\":\"Patient\"}", models.FormatFHIR},
		{"event carrying a resource", `{"topic":"patient-created","focus":{"id":"EID-1","resource":{"resourceType":"Patient","id":"EID-1"}}}`, models.FormatJSON},
		{"array with a non-resource", `[{"resourceType":"Patient"},{"note":"x"}]`, models.FormatJSON},
		{"resourceType only in a string value", `{"note":"\"resourceType\" is a FHIR key"}`, models.FormatJSON},
		{"plain JSON", `{"a":1}`, models.FormatJSON},
	}
	for _, tc := range cases {
		if got := fd.DetectFormat(tc.content).DetectedFormat; got != tc.want {
			t.Errorf("%s: detected %q, want %q", tc.name, got, tc.want)
		}
	}
}

// An event message carrying a FHIR resource parses, with its fields
// addressable, instead of failing for want of a FHIR parser.
func TestMessageParser_EventCarryingResourceParsesAsJSON(t *testing.T) {
	fd := NewFormatDetector()
	pf := NewParserFactory()
	content := `{"topic":"patient-created","focus":{"id":"EID-1","resource":{"resourceType":"Patient","birthDate":"1979-05-21"}}}`
	parser, err := pf.GetParser(fd.DetectFormat(content).DetectedFormat)
	if err != nil {
		t.Fatalf("no parser: %v", err)
	}
	res := parser.Parse(content)
	if !res.Success {
		t.Fatalf("parse failed: %s", res.Error)
	}
	if res.ParsedJSON["topic"] != "patient-created" {
		t.Errorf("topic not addressable: %v", res.ParsedJSON["topic"])
	}
}
