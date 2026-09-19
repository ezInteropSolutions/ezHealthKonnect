// services/parsers/ncpdpscript/ncpdp_script_parser_service.go
// NCPDPScriptParserService adapts ncpdp.ParseMessage to this codebase's
// MessageParser interface (services/parser_factory.go), the same role
// services/parsers/edix12/edi_x12_parser_service.go plays for X12 EDI and
// services/parsers/cda/cda_parser_service.go plays for CDA — self-
// initializes from a schema directory at construction time, parses raw
// content into a schema-enriched ParserResult.
package ncpdpscriptparser

import (
	"fmt"
	"strings"
	"time"

	"ezhealthkonnect/models"
	"ezhealthkonnect/ncpdp"
)

// NCPDPScriptParserService parses NCPDP SCRIPT (pharmacy e-prescribing) XML
// content — Phase 1 scope: NewRx, CancelRx, CancelRxResponse,
// RxChangeRequest, RxChangeResponse — into a ParsedJSON/EnhancedFields
// ParserResult.
type NCPDPScriptParserService struct {
	loader *ncpdp.NCPDPSchemaLoader
}

// NewFromSchemaDir builds a fully wired NCPDPScriptParserService from a
// schema directory path (e.g. "./ncpdp/schemas/script_2017071").
func NewFromSchemaDir(schemaDir string) (*NCPDPScriptParserService, error) {
	loader, err := ncpdp.NewNCPDPSchemaLoader(schemaDir)
	if err != nil {
		return nil, fmt.Errorf("ncpdpscript: schema loader init: %w", err)
	}
	return &NCPDPScriptParserService{loader: loader}, nil
}

// Spec exposes the loaded spec directly — used by ncpdp.build/ncpdp.validate
// pipeline steps that need the same *ncpdp.NCPDPSpecDef this parser used,
// without loading the schema directory a second time.
func (s *NCPDPScriptParserService) Spec() *ncpdp.NCPDPSpecDef {
	return s.loader.Spec()
}

// =====================================
// services.MessageParser interface methods
// =====================================

// GetSupportedFormat satisfies the services.MessageParser interface used by ParserFactory.
func (s *NCPDPScriptParserService) GetSupportedFormat() models.MessageFormat {
	return models.FormatNCPDPScript
}

// Parse is the main entry point. Returns Success=false on fatal parse
// errors (missing <Message>/<Body>, unsupported transaction type); like
// edi_x12_parser_service.go, structural validation against the schema's own
// Required flags is a separate ncpdp.validate step, not this one.
func (s *NCPDPScriptParserService) Parse(raw string) *models.ParserResult {
	start := time.Now()

	result := &models.ParserResult{
		Format:         models.FormatNCPDPScript,
		EnhancedFields: make(map[string]*models.EnhancedField),
		FieldOrder:     []string{},
	}

	parsed, err := ncpdp.ParseMessage(s.loader.Spec(), raw)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("ncpdpscript: %v", err)
		return result
	}

	result.Success = true
	result.ParsingTime = time.Since(start)
	result.Metadata = models.ParserMetadata{
		DetectedVersion:  s.loader.Spec().Version,
		MessageType:      parsed.TransactionType,
		MessageControlID: parsed.MessageAttrs["MessageID"],
		FieldCount:       len(parsed.Header) + len(parsed.Body),
		ParsedAt:         time.Now(),
	}

	// _format/transactionType/messageAttrs mirror the same sentinel-key
	// convention cda_parser_service.go's assembleJSON and
	// edi_x12_parser_service.go's own ParsedJSON already use — header/body
	// carry ncpdp.ParseResult's own field names directly, no renaming.
	result.ParsedJSON = map[string]interface{}{
		"_format":         "ncpdpscript",
		"transactionType": parsed.TransactionType,
		"messageAttrs":    parsed.MessageAttrs,
		"header":          parsed.Header,
		"body":            parsed.Body,
	}

	flattenFields("header", parsed.Header, result)
	flattenFields("body", parsed.Body, result)

	return result
}

// flattenFields walks a canonical field/group map into dotted/bracketed
// paths (e.g. "body.medicationPrescribed.quantity.value",
// "body.diagnoses[0].code") for the generic, format-agnostic field-picker UI
// — the same role edi_x12_parser_service.go's flattening of parsed.Fields
// plays, adapted for ncpdp.ParseResult's nested-map shape (which, unlike
// edi.ParseResult, has no separate flat Fields index of its own).
func flattenFields(prefix string, node map[string]interface{}, result *models.ParserResult) {
	for key, value := range node {
		path := prefix + "." + key
		switch v := value.(type) {
		case map[string]interface{}:
			flattenFields(path, v, result)
		case []interface{}:
			for i, item := range v {
				itemPath := fmt.Sprintf("%s[%d]", path, i)
				if m, ok := item.(map[string]interface{}); ok {
					flattenFields(itemPath, m, result)
				} else {
					addLeafField(itemPath, item, result)
				}
			}
		default:
			addLeafField(path, value, result)
		}
	}
}

func addLeafField(path string, value interface{}, result *models.ParserResult) {
	s, _ := value.(string)
	result.EnhancedFields[path] = &models.EnhancedField{
		Path:     path,
		Value:    value,
		DataType: "string",
		HasValue: s != "",
	}
	result.FieldOrder = append(result.FieldOrder, path)
}

// ValidateStructure checks whether the raw content looks like NCPDP SCRIPT
// XML at all (a cheap shape check, NOT full schema validation — that's
// ncpdp.validate's own, separate job, mirroring
// edi_x12_parser_service.go/cda_parser_service.go's identical convention).
func (s *NCPDPScriptParserService) ValidateStructure(rawContent string) (*models.ValidationResult, error) {
	result := &models.ValidationResult{Warnings: []string{}, Errors: []string{}}

	trimmed := strings.TrimSpace(rawContent)
	if len(trimmed) < 10 {
		result.Errors = append(result.Errors, "content too short to be valid NCPDP SCRIPT XML")
		return result, nil
	}
	if !strings.Contains(trimmed, "<Message") {
		result.Errors = append(result.Errors, "content does not contain a <Message> root element")
		return result, nil
	}
	if !strings.Contains(trimmed, "TransactionDomain=\"SCRIPT\"") {
		result.Errors = append(result.Errors, "content's <Message> element does not declare TransactionDomain=\"SCRIPT\"")
		return result, nil
	}

	result.IsValid = true
	return result, nil
}
