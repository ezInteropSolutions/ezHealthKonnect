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

	// Raw passthrough for every format without a dedicated structured parser.
	// FHIR is deliberately NOT included here — it's handled entirely by
	// http_fhir_inbound's own connector-level path (processing/
	// engine_message_processor.go's sourceType=="http_fhir" branch), which is
	// where FHIR-specific validation belongs; registering a passthrough for
	// FormatFHIR here would never actually be reached by that path, but could
	// be misleading if some other caller ever looked it up expecting real
	// FHIR parsing. Everything else (Unknown/JSON/XML/CSV/EDI) previously had
	// no parser at all, meaning GetParser failed outright for any connector
	// that isn't format-locked to HL7v2/CDA/FHIR — see raw_parser.go's file
	// header for the full rationale.
	for _, format := range []models.MessageFormat{
		models.FormatUnknown, models.FormatJSON, models.FormatXML,
		models.FormatCSV, models.FormatEDI, models.FormatHL7v3,
	} {
		pf.parsers[format] = parsers.NewRawPassthroughParser(format)
	}

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
