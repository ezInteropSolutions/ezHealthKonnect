// services/parsers/edix12/edi_x12_parser_service.go
// EDIX12ParserService adapts edi.ParseTransactionSet to this codebase's
// MessageParser interface (services/parser_factory.go), the same role
// services/parsers/cda/cda_parser_service.go plays for CDA — self-
// initializes from a schema directory at construction time, parses raw
// content into a schema-enriched ParserResult.
package edix12parser

import (
	"fmt"
	"strings"
	"time"

	"ezhealthkonnect/edi"
	"ezhealthkonnect/models"
)

// EDIX12ParserService parses X12 EDI content (currently 835 only — phase 1
// scope) into a ParsedJSON/EnhancedFields ParserResult.
type EDIX12ParserService struct {
	loader *edi.X12SchemaLoader
}

// NewFromSchemaDir builds a fully wired EDIX12ParserService from a schema
// directory path (e.g. "./edi/schemas/x12_005010").
func NewFromSchemaDir(schemaDir string) (*EDIX12ParserService, error) {
	loader, err := edi.NewX12SchemaLoader(schemaDir)
	if err != nil {
		return nil, fmt.Errorf("edi: schema loader init: %w", err)
	}
	return &EDIX12ParserService{loader: loader}, nil
}

// Spec exposes the loaded spec directly — used by edi.build/edi.validate
// pipeline steps that need the same *edi.X12SpecDef this parser used,
// without loading the schema directory a second time.
func (s *EDIX12ParserService) Spec() *edi.X12SpecDef {
	return s.loader.Spec()
}

// =====================================
// services.MessageParser interface methods
// =====================================

// GetSupportedFormat satisfies the services.MessageParser interface used by ParserFactory.
func (s *EDIX12ParserService) GetSupportedFormat() models.MessageFormat {
	return models.FormatEDI
}

// Parse is the main entry point. Accepts raw X12 EDI content — with or
// without an ISA envelope, see edi/segment_reader.go's DetectDelimiters —
// and returns a fully populated ParserResult. Returns Success=false on
// fatal parse errors (e.g. a required segment missing, an unknown
// transaction set); parsing itself is otherwise permissive by design (see
// edi/loop_engine.go's own doc comments) — malformed VALUES still parse,
// they're caught by a separate edi.validate step, not here.
func (s *EDIX12ParserService) Parse(raw string) *models.ParserResult {
	start := time.Now()

	result := &models.ParserResult{
		Format:         models.FormatEDI,
		EnhancedFields: make(map[string]*models.EnhancedField),
		FieldOrder:     []string{},
	}

	parsed, err := edi.ParseTransactionSet(s.loader.Spec(), raw)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("edi: %v", err)
		return result
	}

	result.Success = true
	result.ParsingTime = time.Since(start)

	controlID := ""
	if v, ok := parsed.Interchange["stControlNumber"].(string); ok {
		controlID = v
	}
	result.Metadata = models.ParserMetadata{
		DetectedVersion:  s.loader.Spec().SpecVersion,
		MessageType:      parsed.TransactionSet,
		MessageControlID: controlID,
		SegmentCount:     len(parsed.SegmentInstances),
		FieldCount:       len(parsed.Fields),
		ParsedAt:         time.Now(),
	}

	// _format/transactionSet/envelopePresent are the same sentinel-key
	// convention cda_parser_service.go's assembleJSON uses ("_format" at the
	// root) — interchange/header/loops/trailer mirror edi.ParseResult's own
	// field names directly, no renaming.
	result.ParsedJSON = map[string]interface{}{
		"_format":         "edi",
		"transactionSet":  parsed.TransactionSet,
		"envelopePresent": parsed.EnvelopePresent,
		"interchange":     parsed.Interchange,
		"header":          parsed.Header,
		"loops":           parsed.Loops,
		"trailer":         parsed.Trailer,
	}

	for path, field := range parsed.Fields {
		result.EnhancedFields[path] = &models.EnhancedField{
			Path:     path,
			Value:    field.Value,
			DataType: field.DataType,
			HasValue: field.Value != "",
		}
		result.FieldOrder = append(result.FieldOrder, path)
	}

	return result
}

// ValidateStructure checks whether the raw content looks like X12 EDI at
// all (a cheap shape check, NOT full schema validation — that's
// edi.validate's own, separate job, mirroring how cda_parser_service.go's
// own ValidateStructure only checks for a ClinicalDocument root, not full
// CDA schema conformance).
func (s *EDIX12ParserService) ValidateStructure(rawContent string) (*models.ValidationResult, error) {
	result := &models.ValidationResult{Warnings: []string{}, Errors: []string{}}

	trimmed := strings.TrimSpace(rawContent)
	if len(trimmed) < 4 {
		result.Errors = append(result.Errors, "content too short to be valid X12 EDI")
		return result, nil
	}
	if !strings.HasPrefix(trimmed, "ISA") && !strings.HasPrefix(trimmed, "ST") {
		result.Errors = append(result.Errors, "content does not start with ISA (full envelope) or ST (bare transaction set)")
		return result, nil
	}

	result.IsValid = true
	return result, nil
}
