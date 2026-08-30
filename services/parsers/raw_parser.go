// services/parsers/raw_parser.go
// RawPassthroughParser wraps content that has no dedicated structured parser
// (Unknown, JSON, XML, CSV, EDI — this codebase only ships structured parsers
// for HL7 v2 and CDA/CCDA; FHIR is handled by its own dedicated connector-level
// path, not through this factory at all) in a minimal envelope instead of
// failing outright.
//
// Before this, ParserFactory.GetParser returned an error for any of these
// formats, and MessageParserService.ParseToJSON turned that into a hard
// "parsing failed" that blocked the message before it ever reached the
// transformation pipeline — for EVERY connector except http_fhir_inbound
// (which has its own special-cased bypass in
// processing/engine_message_processor.go). That meant a genuinely
// format-agnostic connector (e.g. http_rest_inbound, file_listener,
// sftp_inbound, kafka_inbound) could never actually accept "any format" the
// way it was meant to — anything that wasn't HL7 v2 or CDA-shaped was
// rejected regardless of which connector received it.
//
// This parser makes the shared pipeline permissive by default: content it
// doesn't have a real structured parser for still flows through (as
// {"_format": "...", "raw": "<original content>"}, unmodified) rather than
// being dropped. Structured, format-aware parsing remains exactly where it
// already was — dedicated parsers for HL7v2/CDA, and http_fhir_inbound's own
// FHIR-specific handling — this only removes the hard failure for everything
// else.
package parsers

import (
	"time"

	"ezhealthkonnect/models"
)

// RawPassthroughParser always succeeds, wrapping raw content in a minimal envelope.
type RawPassthroughParser struct {
	format models.MessageFormat
}

// NewRawPassthroughParser creates a passthrough parser for the given format label.
func NewRawPassthroughParser(format models.MessageFormat) *RawPassthroughParser {
	return &RawPassthroughParser{format: format}
}

// Parse never fails — it wraps rawContent as-is.
func (p *RawPassthroughParser) Parse(rawContent string) *models.ParserResult {
	return &models.ParserResult{
		Success: true,
		Format:  p.format,
		ParsedJSON: map[string]interface{}{
			"raw": rawContent,
		},
		Metadata: models.ParserMetadata{
			ParserVersion: "raw-passthrough-1.0",
			ParsedAt:      time.Now(),
		},
		ValidationResult: models.ValidationResult{IsValid: true},
	}
}

// GetSupportedFormat returns the format this instance was registered for.
func (p *RawPassthroughParser) GetSupportedFormat() models.MessageFormat {
	return p.format
}

// ValidateStructure always reports valid — there is no structure to validate
// for a format this parser doesn't actually understand.
func (p *RawPassthroughParser) ValidateStructure(rawContent string) (*models.ValidationResult, error) {
	return &models.ValidationResult{IsValid: true}, nil
}
