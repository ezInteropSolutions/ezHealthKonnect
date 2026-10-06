// services/parsers/astm/astm_parser_service.go
// ASTMParserService adapts astm.ParseMessage to this codebase's MessageParser
// interface (services/parser_factory.go) — mirrors
// services/parsers/ncpdptelecom/ncpdp_telecom_parser_service.go's own role
// exactly, self-initializing from a schema directory at construction time.
package astmparser

import (
	"fmt"
	"time"

	"ezhealthkonnect/astm"
	"ezhealthkonnect/models"
)

// ASTMParserService parses ASTM E1394-97 (lab instrument host-interface)
// content — already destuffed of its ENQ/STX/ETX/EOT transport framing by
// the owning connector (services/connectors/astm_framing.go) before it ever
// reaches this parser, the same convention every other connector-owned
// framing layer in this codebase follows.
type ASTMParserService struct {
	loader *astm.ASTMSchemaLoader
}

// NewFromSchemaDir builds a fully wired ASTMParserService from a schema
// directory path (e.g. "./astm/schemas/e1394_97").
func NewFromSchemaDir(schemaDir string) (*ASTMParserService, error) {
	loader, err := astm.NewASTMSchemaLoader(schemaDir)
	if err != nil {
		return nil, fmt.Errorf("astmparser: schema loader init: %w", err)
	}
	return &ASTMParserService{loader: loader}, nil
}

// Spec exposes the loaded spec directly — used by astm.parse/build/validate
// pipeline steps that need the same *astm.ASTMSpecDef this parser used,
// without loading the schema directory a second time.
func (s *ASTMParserService) Spec() *astm.ASTMSpecDef {
	return s.loader.Spec()
}

// =====================================
// services.MessageParser interface methods
// =====================================

// GetSupportedFormat satisfies the services.MessageParser interface used by ParserFactory.
func (s *ASTMParserService) GetSupportedFormat() models.MessageFormat {
	return models.FormatASTM
}

// Parse is the main entry point for the generic auto-detection pathway.
// ASTM doesn't version by transaction set the way X12/NCPDP Telecom do, so
// there's no direction/variant to sniff — "generic_lab_result" is the one
// message profile phase 1 registers (see astm/schema_types.go's own doc
// comment on why this is a fixed shape, not a schema-driven variable tree).
func (s *ASTMParserService) Parse(raw string) *models.ParserResult {
	start := time.Now()

	result := &models.ParserResult{
		Format:         models.FormatASTM,
		EnhancedFields: make(map[string]*models.EnhancedField),
		FieldOrder:     []string{},
	}

	parsed, err := astm.ParseMessage(s.loader.Spec(), "generic_lab_result", raw)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("astmparser: %v", err)
		return result
	}

	result.Success = true
	result.ParsingTime = time.Since(start)

	messageControlID := ""
	if v, ok := parsed.Header["messageControlID"].(string); ok {
		messageControlID = v
	}
	result.Metadata = models.ParserMetadata{
		DetectedVersion:  s.loader.Spec().SpecVersion,
		MessageType:      "ASTM",
		MessageControlID: messageControlID,
		SegmentCount:     len(parsed.RecordInstances),
		FieldCount:       len(parsed.Fields),
		ParsedAt:         time.Now(),
	}

	// "raw" mirrors the same convention every other format parser in this
	// codebase uses — the original message text, so a later mid-pipeline
	// astm.parse step (default sourceField "raw") can find it on a genuinely
	// live message, not just inside a Test Pipeline run's own envelope
	// construction. See CLAUDE.md's Coverage Audit generalization section
	// for the earlier, identical gap found and fixed for EDI/NCPDP.
	result.ParsedJSON = map[string]interface{}{
		"_format":        "astm",
		"raw":            raw,
		"messageProfile": parsed.MessageProfile,
		"header":         parsed.Header,
		"headerComments": parsed.HeaderComments,
		"patientBlocks":  parsed.PatientBlocks,
		"queryBlocks":    parsed.QueryBlocks,
		"trailer":        parsed.Trailer,
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

// ValidateStructure checks whether the raw content looks like ASTM E1394-97
// at all (a cheap shape check, NOT full schema validation — that's
// astm.validate's own, separate job, mirroring every other format parser's
// identical convention).
func (s *ASTMParserService) ValidateStructure(rawContent string) (*models.ValidationResult, error) {
	result := &models.ValidationResult{Warnings: []string{}, Errors: []string{}}

	records := astm.SplitRecords(rawContent)
	if len(records) == 0 {
		result.Errors = append(result.Errors, "no records found in content")
		return result, nil
	}
	if _, found := astm.DetectDelimiters(records); !found {
		result.Errors = append(result.Errors, "no H (Header) record found")
		return result, nil
	}

	result.IsValid = true
	return result, nil
}
