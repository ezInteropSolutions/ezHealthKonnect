// services/parser_factory.go
// Factory for creating message parsers (OOB pattern)

package services

import (
	"fmt"
	"log"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services/parsers"
	cdaparser "ezhealthkonnect/services/parsers/cda"
	edix12parser "ezhealthkonnect/services/parsers/edix12"
	ncpdpscriptparser "ezhealthkonnect/services/parsers/ncpdpscript"
	ncpdptelecomparser "ezhealthkonnect/services/parsers/ncpdptelecom"
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

	// Register X12 EDI parser (835 phase 1) — gracefully skipped if schema
	// dir is missing, same convention as the CDA parser above.
	pf.RegisterEDIX12Parser("./edi/schemas/x12_005010")

	// Register NCPDP SCRIPT (pharmacy e-prescribing) parser (Phase 1: NewRx/
	// CancelRx/CancelRxResponse/RxChangeRequest/RxChangeResponse) —
	// gracefully skipped if schema dir is missing, same convention as CDA/EDI above.
	pf.RegisterNCPDPScriptParser("./ncpdp/schemas/script_2017071")

	// Register NCPDP Telecommunication D.0 (real-time pharmacy claims)
	// parser (Phase 1: B1 Claim Billing request/response) — gracefully
	// skipped if schema dir is missing, same convention as CDA/EDI/NCPDP
	// SCRIPT above.
	pf.RegisterNCPDPTelecomParser("./ncpdptelecom/schemas/telecom_d0")

	// Raw passthrough for every format without a dedicated structured parser.
	// FHIR is deliberately NOT included here — it's handled entirely by
	// http_fhir_inbound's own connector-level path (processing/
	// engine_message_processor.go's sourceType=="http_fhir" branch), which is
	// where FHIR-specific validation belongs; registering a passthrough for
	// FormatFHIR here would never actually be reached by that path, but could
	// be misleading if some other caller ever looked it up expecting real
	// FHIR parsing. Everything else (Unknown/JSON/XML/CSV) previously had
	// no parser at all, meaning GetParser failed outright for any connector
	// that isn't format-locked to HL7v2/CDA/FHIR/EDI — see raw_parser.go's
	// file header for the full rationale.
	for _, format := range []models.MessageFormat{
		models.FormatUnknown, models.FormatJSON, models.FormatXML,
		models.FormatCSV, models.FormatHL7v3,
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

// RegisterEDIX12Parser initialises and registers the X12 EDI parser (835
// phase 1) using schema files from schemaDir (e.g. "./edi/schemas/x12_005010").
// Returns nil and logs a warning if the schema directory is missing or invalid —
// existing HL7/CDA/FHIR processing is unaffected. Mirrors RegisterCDAParser exactly.
func (pf *ParserFactory) RegisterEDIX12Parser(schemaDir string) error {
	svc, err := edix12parser.NewFromSchemaDir(schemaDir)
	if err != nil {
		log.Printf("⚠️  EDI X12 parser not registered: %v", err)
		return err
	}
	pf.parsers[models.FormatEDI] = svc
	log.Printf("✅ EDI X12 parser registered (schema: %s)", schemaDir)
	return nil
}

// RegisterNCPDPScriptParser initialises and registers the NCPDP SCRIPT
// (pharmacy e-prescribing) parser using schema files from schemaDir (e.g.
// "./ncpdp/schemas/script_2017071"). Returns nil and logs a warning if the
// schema directory is missing or invalid — existing HL7/CDA/EDI/FHIR
// processing is unaffected. Mirrors RegisterEDIX12Parser/RegisterCDAParser exactly.
func (pf *ParserFactory) RegisterNCPDPScriptParser(schemaDir string) error {
	svc, err := ncpdpscriptparser.NewFromSchemaDir(schemaDir)
	if err != nil {
		log.Printf("⚠️  NCPDP SCRIPT parser not registered: %v", err)
		return err
	}
	pf.parsers[models.FormatNCPDPScript] = svc
	log.Printf("✅ NCPDP SCRIPT parser registered (schema: %s)", schemaDir)
	return nil
}

// RegisterNCPDPTelecomParser initialises and registers the NCPDP
// Telecommunication D.0 (real-time pharmacy claims) parser using schema
// files from schemaDir (e.g. "./ncpdptelecom/schemas/telecom_d0"). Returns
// nil and logs a warning if the schema directory is missing or invalid —
// existing HL7/CDA/EDI/NCPDP SCRIPT/FHIR processing is unaffected. Mirrors
// RegisterNCPDPScriptParser exactly.
func (pf *ParserFactory) RegisterNCPDPTelecomParser(schemaDir string) error {
	svc, err := ncpdptelecomparser.NewFromSchemaDir(schemaDir)
	if err != nil {
		log.Printf("⚠️  NCPDP Telecommunication D.0 parser not registered: %v", err)
		return err
	}
	pf.parsers[models.FormatNCPDPTelecom] = svc
	log.Printf("✅ NCPDP Telecommunication D.0 parser registered (schema: %s)", schemaDir)
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
