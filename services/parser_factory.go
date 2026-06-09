// services/parser_factory.go
// Factory for creating message parsers (OOB pattern)

package services

import (
	"fmt"
	"log"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services/parsers"
	cdaparser "ezhealthkonnect/services/parsers/cda"
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

	// Register CDA/CCD parser — gracefully skipped if schema dir is missing.
	pf.RegisterCDAParser("./cda/schemas")

	log.Printf("✅ ParserFactory registered %d parsers", len(pf.parsers))
}

// RegisterCDAParser initialises and registers the CDA/CCD parser using
// schema files from schemaDir (e.g. "./cda/schemas").
// Returns nil and logs a warning if the schema directory is missing or invalid —
// existing HL7/FHIR processing is unaffected.
func (pf *ParserFactory) RegisterCDAParser(schemaDir string) error {
	svc, err := cdaparser.NewFromSchemaDir(schemaDir)
	if err != nil {
		log.Printf("⚠️  CDA parser not registered: %v", err)
		return err
	}
	pf.parsers[models.FormatCCDA] = svc
	log.Printf("✅ CDA parser registered (schema: %s)", schemaDir)
	return nil
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
