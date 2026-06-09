// services/parsers/message_parser.go
// Format-agnostic MessageParser interface and registry.
//
// Adding a new format (CDA, X12, …) requires only a new MessageParser
// implementation — no changes to callers.
//
// Pattern mirrors services/executors/control/collection_resolver.go.

package parsers

import (
	"encoding/json"
	"time"

	"ezhealthkonnect/models"
)

// =====================================
// INTERFACE
// =====================================

// MessageParser is the common contract for all format-specific schema parsers.
type MessageParser interface {
	// Format returns the canonical format identifier matching models.MessageFormat.
	Format() string
	// Parse enriches a raw message with schema metadata and returns a
	// format-agnostic ParserResult. Never returns nil — Success=false on error.
	Parse(raw string) *models.ParserResult
}

// =====================================
// REGISTRY
// =====================================

// ParserRegistry resolves parsers by format string and falls back to the
// PassthroughParser for unknown formats so callers never receive nil.
type ParserRegistry struct {
	parsers map[string]MessageParser
}

// NewParserRegistry creates a registry pre-loaded with all built-in parsers.
func NewParserRegistry() *ParserRegistry {
	r := &ParserRegistry{parsers: make(map[string]MessageParser)}
	r.Register(NewHL7ParserService())
	r.Register(NewFHIRParserService())
	return r
}

// Register adds or replaces a parser for its format.
func (r *ParserRegistry) Register(p MessageParser) {
	r.parsers[p.Format()] = p
}

// Get returns the parser for the given format. Aliases ("hl7" → "hl7v2") are
// resolved before lookup. Falls back to PassthroughParser for unknown formats.
func (r *ParserRegistry) Get(format string) MessageParser {
	resolved := resolveFormatAlias(format)
	if p, ok := r.parsers[resolved]; ok {
		return p
	}
	return &PassthroughParser{format: format}
}

// resolveFormatAlias normalises common format aliases.
func resolveFormatAlias(format string) string {
	switch format {
	case "hl7", "hl7v2":
		return string(models.FormatHL7v2)
	case "fhir":
		return string(models.FormatFHIR)
	case "cda", "ccd", "c32", "hitsp":
		return string(models.FormatCCDA)
	// "json" is intentionally NOT remapped to "fhir" — plain JSON goes to
	// PassthroughParser which does a best-effort field merge without claiming
	// FHIR semantics. FHIRParserService itself detects FHIR via resourceType.
	default:
		return format
	}
}

// =====================================
// PASSTHROUGH PARSER
// =====================================

// PassthroughParser handles formats with no dedicated schema parser (XML, CCDA,
// EDI, plain JSON, etc.). It preserves the raw message and does a best-effort
// JSON merge for JSON-like payloads.
type PassthroughParser struct {
	format string
}

func (p *PassthroughParser) Format() string { return p.format }

func (p *PassthroughParser) Parse(raw string) *models.ParserResult {
	result := &models.ParserResult{
		Success:        true,
		Format:         models.MessageFormat(p.format),
		ParsingTime:    0,
		EnhancedFields: make(map[string]*models.EnhancedField),
		FieldOrder:     []string{},
		Metadata: models.ParserMetadata{
			DetectedVersion: p.format,
			ParsedAt:        time.Now(),
		},
	}

	parsed := map[string]interface{}{
		"raw":      raw,
		"_format":  p.format,
		"parsedAt": time.Now().Format(time.RFC3339),
	}

	// Best-effort JSON parse for json/fhir fallback — if it has no resourceType
	// the FHIR parser fell back here; just merge the fields.
	if p.format == "json" || p.format == "fhir" {
		var obj map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &obj); err == nil {
			for k, v := range obj {
				parsed[k] = v
			}
		}
	}

	// XML-based formats: signal content type for downstream steps
	if p.format == "xml" || p.format == "ccda" || p.format == "hl7v3" {
		parsed["content_type"] = "text/xml"
	}

	result.ParsedJSON = parsed
	return result
}
