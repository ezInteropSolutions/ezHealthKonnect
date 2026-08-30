// services/parsers/raw_parser_test.go
package parsers

import (
	"testing"

	"ezhealthkonnect/models"
)

func TestRawPassthroughParser_AlwaysSucceeds(t *testing.T) {
	p := NewRawPassthroughParser(models.FormatUnknown)
	result := p.Parse("this is plain, non-healthcare-formatted text")
	if !result.Success {
		t.Fatalf("expected Success=true, got false with error: %s", result.Error)
	}
	if result.ParsedJSON["raw"] != "this is plain, non-healthcare-formatted text" {
		t.Errorf("expected raw content preserved, got: %v", result.ParsedJSON["raw"])
	}
	if result.Format != models.FormatUnknown {
		t.Errorf("expected format=unknown, got: %s", result.Format)
	}
}

func TestRawPassthroughParser_ValidateStructure_AlwaysValid(t *testing.T) {
	p := NewRawPassthroughParser(models.FormatJSON)
	v, err := p.ValidateStructure("anything at all")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !v.IsValid {
		t.Error("expected IsValid=true")
	}
}

func TestRawPassthroughParser_GetSupportedFormat(t *testing.T) {
	p := NewRawPassthroughParser(models.FormatCSV)
	if p.GetSupportedFormat() != models.FormatCSV {
		t.Errorf("expected FormatCSV, got: %s", p.GetSupportedFormat())
	}
}
