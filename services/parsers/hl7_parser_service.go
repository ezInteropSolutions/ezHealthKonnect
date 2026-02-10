// services/parsers/hl7_parser_service.go
// HL7 Parser Service - Adapter wrapping existing HL7 parser
// Follows MVC pattern and reuses existing hl7.ParseWithRealSchema()

package parsers

import (
	"fmt"
	"log"
	"strings"
	"time"

	"ezhealthkonnect/hl7"
	"ezhealthkonnect/models"
)

// HL7ParserService wraps existing HL7 parser in standardized interface
type HL7ParserService struct {
	version string
}

// NewHL7ParserService creates new HL7 parser service
func NewHL7ParserService() *HL7ParserService {
	return &HL7ParserService{
		version: "1.0.0",
	}
}

// Parse converts HL7 message to standard JSON format
// Reuses existing hl7.ParseWithRealSchema() - NO CODE DUPLICATION
func (hp *HL7ParserService) Parse(rawContent string) (*models.ParserResult, error) {
	startTime := time.Now()

	// ✅ REUSE EXISTING PARSER - Call existing schema-based parser
	enhancedResult := hl7.ParseWithRealSchema(rawContent)

	if !enhancedResult.Success {
		return &models.ParserResult{
			Success:     false,
			Format:      models.FormatHL7v2,
			Error:       enhancedResult.Error,
			ParsingTime: time.Since(startTime),
		}, fmt.Errorf("HL7 parsing failed: %s", enhancedResult.Error)
	}

	// Convert to standard JSON structure (preserves FULL enhanced schema)
	parsedJSON := convertToStandardJSON(enhancedResult)

	// Extract metadata
	metadata := models.ParserMetadata{
		ParserVersion:    hp.version,
		DetectedVersion:  enhancedResult.Version,
		MessageType:      enhancedResult.MessageType.Name,
		MessageControlID: extractControlID(enhancedResult),
		SegmentCount:     len(enhancedResult.SegmentOrder),
		FieldCount:       countTotalFields(enhancedResult),
		ParsedAt:         time.Now(),
	}

	// DEBUG: Log what messageType we extracted
	log.Printf("🔍 HL7 Parser: MessageType struct=%+v, MessageType.Name='%s'", enhancedResult.MessageType, enhancedResult.MessageType.Name)

	// Extract validation results
	validationResult := models.ValidationResult{
		IsValid:  enhancedResult.Success && len(enhancedResult.ValidationErrors) == 0,
		Errors:   extractErrors(enhancedResult.ValidationErrors),
		Warnings: extractWarnings(enhancedResult.ValidationErrors),
	}

	return &models.ParserResult{
		Success:          true,
		Format:           models.FormatHL7v2,
		ParsedJSON:       parsedJSON,
		Metadata:         metadata,
		ValidationResult: validationResult,
		ParsingTime:      time.Since(startTime),
	}, nil
}

// convertToStandardJSON converts EnhancedParsedMessage to JSON
// IMPORTANT: We store the FULL enhanced structure from hl7.ParseWithRealSchema()
// This preserves all schema information, field metadata, validation results, etc.
func convertToStandardJSON(enhanced *hl7.EnhancedParsedMessage) map[string]interface{} {
	// Convert the entire EnhancedParsedMessage to JSON
	// This preserves all the rich schema-based parsing data
	result := make(map[string]interface{})

	// Core message data
	result["raw"] = enhanced.Raw
	result["success"] = enhanced.Success
	result["error"] = enhanced.Error
	result["version"] = enhanced.Version
	result["messageType"] = enhanced.MessageType

	// Enhanced segments with full schema metadata
	result["enhancedSegments"] = enhanced.EnhancedSegments
	result["segmentGroups"] = enhanced.SegmentGroups           // ALL instances of each segment type (for loops over IN1, OBX, etc.)
	result["observationGroups"] = enhanced.ObservationGroups  // OBR-OBX grouped relationships for nested loops
	result["segmentOrder"] = enhanced.SegmentOrder

	// Basic segments (if available)
	if len(enhanced.BasicSegments) > 0 {
		result["basicSegments"] = enhanced.BasicSegments
	}

	// Parsing metadata
	result["parsedAt"] = enhanced.ParsedAt
	result["dictionaryUsed"] = enhanced.DictionaryUsed
	result["schemaLoaded"] = enhanced.SchemaLoaded

	// Validation results
	if len(enhanced.ValidationErrors) > 0 {
		result["validationErrors"] = enhanced.ValidationErrors
	}

	return result
}

// extractControlID extracts message control ID from parsed message
func extractControlID(enhanced *hl7.EnhancedParsedMessage) string {
	if msh, exists := enhanced.EnhancedSegments["MSH"]; exists {
		// Find MSH.10 field (Message Control ID)
		for _, field := range msh.Fields {
			if field.Position == 10 || field.Key == "MSH.10" {
				return field.Value
			}
		}
	}
	return ""
}

// countTotalFields counts all fields in all segments
func countTotalFields(enhanced *hl7.EnhancedParsedMessage) int {
	count := 0
	for _, segment := range enhanced.EnhancedSegments {
		count += len(segment.Fields)
	}
	return count
}

// extractWarnings extracts warning messages
func extractWarnings(errors []hl7.ValidationError) []string {
	warnings := []string{}
	for _, err := range errors {
		if err.Severity == "warning" {
			warnings = append(warnings, err.Message)
		}
	}
	return warnings
}

// extractErrors extracts error messages
func extractErrors(errors []hl7.ValidationError) []string {
	errorMessages := []string{}
	for _, err := range errors {
		if err.Severity == "error" || err.Severity == "" {
			errorMessages = append(errorMessages, err.Message)
		}
	}
	return errorMessages
}

// GetSupportedVersions returns supported HL7 versions
func (hp *HL7ParserService) GetSupportedVersions() []string {
	return []string{"2.3", "2.3.1", "2.4", "2.5", "2.5.1", "2.6", "2.7", "2.8"}
}

// ValidateMessage performs basic HL7 v2 validation
func (hp *HL7ParserService) ValidateMessage(rawContent string) (bool, []string) {
	errors := []string{}

	// Check for MSH segment
	if !strings.HasPrefix(rawContent, "MSH|") &&
		!strings.Contains(rawContent, "\rMSH|") &&
		!strings.Contains(rawContent, "\nMSH|") {
		errors = append(errors, "Missing MSH segment at start")
		return false, errors
	}

	// Basic length check
	if len(rawContent) < 20 {
		errors = append(errors, "Message too short to be valid HL7")
		return false, errors
	}

	return len(errors) == 0, errors
}

// GetSupportedFormat returns the message format this parser supports
func (hp *HL7ParserService) GetSupportedFormat() models.MessageFormat {
	return models.FormatHL7v2
}

// ValidateStructure performs structural validation of the message
func (hp *HL7ParserService) ValidateStructure(rawContent string) (*models.ValidationResult, error) {
	isValid, errors := hp.ValidateMessage(rawContent)

	return &models.ValidationResult{
		IsValid:  isValid,
		Errors:   errors,
		Warnings: []string{},
	}, nil
}
