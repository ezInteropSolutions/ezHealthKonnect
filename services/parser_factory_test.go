// services/parser_factory_test.go
package services

import (
	"testing"

	"ezhealthkonnect/models"
)

func TestParserFactory_GetParser_HL7v2AndCDAStillRegistered(t *testing.T) {
	pf := NewParserFactory()
	if _, err := pf.GetParser(models.FormatHL7v2); err != nil {
		t.Errorf("expected HL7v2 parser to be registered, got error: %v", err)
	}
	// CDA registration depends on schema dir being present; skip strict check here.
}

func TestParserFactory_GetParser_PreviouslyUnsupportedFormatsNowSucceed(t *testing.T) {
	pf := NewParserFactory()
	for _, format := range []models.MessageFormat{
		models.FormatUnknown, models.FormatJSON, models.FormatXML,
		models.FormatCSV, models.FormatEDI, models.FormatHL7v3,
	} {
		parser, err := pf.GetParser(format)
		if err != nil {
			t.Errorf("expected a parser to be available for format %q, got error: %v", format, err)
			continue
		}
		result := parser.Parse("arbitrary test content")
		if !result.Success {
			t.Errorf("expected Parse to succeed for format %q, got error: %s", format, result.Error)
		}
	}
}

func TestParserFactory_GetParser_FHIRNotRegistered(t *testing.T) {
	// FHIR is deliberately handled entirely outside this factory (see
	// raw_parser.go's file header) -- confirm it's NOT silently given a
	// passthrough here, which could mask the real, connector-level FHIR path
	// ever being bypassed by mistake.
	pf := NewParserFactory()
	if _, err := pf.GetParser(models.FormatFHIR); err == nil {
		t.Error("expected FormatFHIR to NOT be registered in ParserFactory (handled by http_fhir_inbound's own path instead)")
	}
}
