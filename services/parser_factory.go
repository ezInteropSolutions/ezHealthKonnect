// services/parser_factory.go
// Factory for creating message parsers (OOB pattern)

package services

import (
	"fmt"
	"log"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services/parsers"
)

// MessageParser interface - all parsers implement this
type MessageParser interface {
	Parse(rawContent string) *models.ParserResult
	GetSupportedFormat() models.MessageFormat
	ValidateStructure(rawContent string) (*models.ValidationResult, error)
}

// ParserFactory creates and manages parsers
type ParserFactory struct {
	parsers map[models.MessageFormat]MessageParser
}

// NewParserFactory creates factory with all parsers registered (OOB)
func NewParserFactory() *ParserFactory {
	factory := &ParserFactory{
		parsers: make(map[models.MessageFormat]MessageParser),
	}

	// OOB: Auto-register all available parsers
	factory.registerParsers()

	return factory
}

// registerParsers registers all built-in parsers (OOB pattern)
func (pf *ParserFactory) registerParsers() {
	// Register HL7 v2 parser (REUSES EXISTING CODE)
	pf.parsers[models.FormatHL7v2] = parsers.NewHL7ParserService()

	// TODO: Register other parsers as they're implemented
	// pf.parsers[models.FormatXML] = parsers.NewXMLParser()
	// pf.parsers[models.FormatJSON] = parsers.NewJSONParser()
	// pf.parsers[models.FormatFHIR] = parsers.NewFHIRParser()
	// pf.parsers[models.FormatEDI] = parsers.NewEDIParser()
	// pf.parsers[models.FormatCSV] = parsers.NewCSVParser()

	log.Printf("✅ ParserFactory registered %d parsers", len(pf.parsers))
}

// GetParser returns appropriate parser for format (OOB selection)
func (pf *ParserFactory) GetParser(format models.MessageFormat) (MessageParser, error) {
	parser, exists := pf.parsers[format]
	if !exists {
		return nil, fmt.Errorf("no parser available for format: %s", format)
	}
	return parser, nil
}


// GetAvailableParsers returns list of supported formats
func (pf *ParserFactory) GetAvailableParsers() []models.MessageFormat {
	formats := make([]models.MessageFormat, 0, len(pf.parsers))
	for format := range pf.parsers {
		formats = append(formats, format)
	}
	return formats
}

// IsFormatSupported checks if a format is supported
func (pf *ParserFactory) IsFormatSupported(format models.MessageFormat) bool {
	_, exists := pf.parsers[format]
	return exists
}
