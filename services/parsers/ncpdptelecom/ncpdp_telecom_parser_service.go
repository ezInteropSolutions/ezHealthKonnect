// services/parsers/ncpdptelecom/ncpdp_telecom_parser_service.go
// NCPDPTelecomParserService adapts ncpdptelecom.ParseTransmission to this
// codebase's MessageParser interface (services/parser_factory.go), the same
// role services/parsers/ncpdpscript/ncpdp_script_parser_service.go plays
// for NCPDP SCRIPT — self-initializes from a schema directory at
// construction time, parses raw content into a schema-enriched
// ParserResult.
package ncpdptelecomparser

import (
	"fmt"
	"strings"
	"time"

	"ezhealthkonnect/models"
	"ezhealthkonnect/ncpdptelecom"
)

// NCPDPTelecomParserService parses NCPDP Telecommunication D.0 (real-time
// pharmacy claims) content — Phase 1 scope: B1 (Claim Billing) request and
// response.
type NCPDPTelecomParserService struct {
	loader *ncpdptelecom.TelecomSchemaLoader
}

// NewFromSchemaDir builds a fully wired NCPDPTelecomParserService from a
// schema directory path (e.g. "./ncpdptelecom/schemas/telecom_d0").
func NewFromSchemaDir(schemaDir string) (*NCPDPTelecomParserService, error) {
	loader, err := ncpdptelecom.NewTelecomSchemaLoader(schemaDir)
	if err != nil {
		return nil, fmt.Errorf("ncpdptelecomparser: schema loader init: %w", err)
	}
	return &NCPDPTelecomParserService{loader: loader}, nil
}

// Spec exposes the loaded spec directly — used by ncpdptelecom.parse/build/
// validate pipeline steps that need the same *ncpdptelecom.TelecomSpecDef
// this parser used, without loading the schema directory a second time.
func (s *NCPDPTelecomParserService) Spec() *ncpdptelecom.TelecomSpecDef {
	return s.loader.Spec()
}

// =====================================
// services.MessageParser interface methods
// =====================================

// GetSupportedFormat satisfies the services.MessageParser interface used by ParserFactory.
func (s *NCPDPTelecomParserService) GetSupportedFormat() models.MessageFormat {
	return models.FormatNCPDPTelecom
}

// Parse is the main entry point for the generic auto-detection pathway,
// where no caller-supplied direction is available. Direction is guessed via
// ncpdptelecom.SniffDirection — see that function's own doc comment for why
// this is a best-effort heuristic, not a certainty (request and response
// share the identical wire transaction code). A pipeline step that knows
// its own direction (ncpdptelecom.parse, configured explicitly) should call
// ncpdptelecom.ParseTransmission directly instead of going through this
// generic entry point. Structural validation against the schema's own
// Required flags is a separate ncpdptelecom.validate step, not this one.
func (s *NCPDPTelecomParserService) Parse(raw string) *models.ParserResult {
	start := time.Now()

	result := &models.ParserResult{
		Format:         models.FormatNCPDPTelecom,
		EnhancedFields: make(map[string]*models.EnhancedField),
		FieldOrder:     []string{},
	}

	direction := ncpdptelecom.SniffDirection(s.loader.Spec(), raw)
	parsed, err := ncpdptelecom.ParseTransmission(s.loader.Spec(), direction, raw)
	if err != nil {
		result.Success = false
		result.Error = fmt.Sprintf("ncpdptelecomparser: %v", err)
		return result
	}

	result.Success = true
	result.ParsingTime = time.Since(start)
	result.Metadata = models.ParserMetadata{
		DetectedVersion:  s.loader.Spec().Version,
		MessageType:      parsed.TransactionCode + "_" + parsed.Direction,
		MessageControlID: fmt.Sprintf("%v", parsed.Header["serviceProviderId"]),
		FieldCount:       len(parsed.Header) + len(parsed.TransmissionGroup) + len(parsed.TransactionGroups),
		ParsedAt:         time.Now(),
	}

	result.ParsedJSON = map[string]interface{}{
		"_format":           "ncpdptelecom",
		"transactionCode":   parsed.TransactionCode,
		"direction":         parsed.Direction,
		"header":            parsed.Header,
		"transmissionGroup": parsed.TransmissionGroup,
		"transactionGroups": parsed.TransactionGroups,
	}

	flattenFields("header", parsed.Header, result)
	flattenFields("transmissionGroup", parsed.TransmissionGroup, result)
	for i, rawCluster := range parsed.TransactionGroups {
		if cluster, ok := rawCluster.(map[string]interface{}); ok {
			flattenFields(fmt.Sprintf("transactionGroups[%d]", i), cluster, result)
		}
	}

	return result
}

// flattenFields walks a canonical field/group map into dotted/bracketed
// paths for the generic, format-agnostic field-picker UI — mirroring
// ncpdp_script_parser_service.go's identical helper.
func flattenFields(prefix string, node map[string]interface{}, result *models.ParserResult) {
	for key, value := range node {
		path := prefix + "." + key
		switch v := value.(type) {
		case map[string]interface{}:
			flattenFields(path, v, result)
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
		HasValue: value != nil && s != "",
	}
	result.FieldOrder = append(result.FieldOrder, path)
}

// ValidateStructure checks whether the raw content looks like an NCPDP
// Telecommunication D.0 transmission at all (a cheap shape check, NOT full
// schema validation — that's ncpdptelecom.validate's own, separate job,
// mirroring every other format parser's identical convention).
func (s *NCPDPTelecomParserService) ValidateStructure(rawContent string) (*models.ValidationResult, error) {
	result := &models.ValidationResult{Warnings: []string{}, Errors: []string{}}

	if len(rawContent) < 56 {
		result.Errors = append(result.Errors, "content shorter than the 56-byte fixed D.0 header")
		return result, nil
	}
	if rawContent[6:8] != "D0" {
		result.Errors = append(result.Errors, "content's header Version/Release field is not \"D0\"")
		return result, nil
	}
	if !strings.ContainsRune(rawContent, '\x1E') {
		result.Errors = append(result.Errors, "content contains no RS (0x1E) segment separator")
		return result, nil
	}

	result.IsValid = true
	return result, nil
}
