// services/parsers/hl7_parser_service.go
// HL7 Parser Service - adapter wrapping hl7.ParseWithRealSchema.
// Implements MessageParser so it slots into ParserRegistry alongside FHIR, CDA, etc.

package parsers

import (
	"log"
	"strings"
	"time"

	"ezhealthkonnect/hl7"
	"ezhealthkonnect/models"
)

// HL7ParserService wraps the existing HL7 parser and adapts its output to
// models.ParserResult so the pipeline controller is format-agnostic.
type HL7ParserService struct{}

// NewHL7ParserService creates an HL7ParserService.
func NewHL7ParserService() *HL7ParserService { return &HL7ParserService{} }

// Format implements MessageParser.
func (hp *HL7ParserService) Format() string { return string(models.FormatHL7v2) }

// Parse implements MessageParser.
// ParsedJSON preserves the full enhanced HL7 structure (enhancedSegments,
// segmentGroups, observationGroups) so existing executor code keeps working.
// EnhancedFields provides the format-agnostic flat field map for new consumers.
func (hp *HL7ParserService) Parse(rawContent string) *models.ParserResult {
	startTime := time.Now()

	enhanced := hl7.ParseWithRealSchema(rawContent)
	if !enhanced.Success {
		return &models.ParserResult{
			Success:     false,
			Format:      models.FormatHL7v2,
			Error:       enhanced.Error,
			ParsingTime: time.Since(startTime),
		}
	}

	log.Printf("🔍 HL7 Parser: type=%s version=%s segments=%d schemaLoaded=%v",
		enhanced.MessageType.Name, enhanced.Version,
		len(enhanced.SegmentOrder), enhanced.SchemaLoaded)

	parsedJSON := buildHL7ParsedJSON(enhanced)
	enhancedFields, fieldOrder := flattenHL7EnhancedFields(enhanced)

	return &models.ParserResult{
		Success:    true,
		Format:     models.FormatHL7v2,
		ParsedJSON: parsedJSON,
		Metadata: models.ParserMetadata{
			DetectedVersion:  enhanced.Version,
			MessageType:      enhanced.MessageType.Name,
			MessageControlID: extractHL7ControlID(enhanced),
			SegmentCount:     len(enhanced.SegmentOrder),
			FieldCount:       len(enhancedFields),
			ParsedAt:         time.Now(),
		},
		ValidationResult: models.ValidationResult{
			IsValid:  enhanced.Success && len(enhanced.ValidationErrors) == 0,
			Errors:   extractHL7Errors(enhanced.ValidationErrors),
			Warnings: extractHL7Warnings(enhanced.ValidationErrors),
		},
		EnhancedFields:  enhancedFields,
		FieldOrder:      fieldOrder,
		TypeName:        enhanced.MessageType.Name,
		TypeDescription: enhanced.MessageType.Description,
		ParsingTime:     time.Since(startTime),
	}
}

// =====================================
// HELPERS
// =====================================

// buildHL7ParsedJSON builds the ParsedJSON map preserving the full enhanced
// HL7 structure that existing executor code depends on.
func buildHL7ParsedJSON(enhanced *hl7.EnhancedParsedMessage) map[string]interface{} {
	result := map[string]interface{}{
		"raw":              enhanced.Raw,
		"success":          enhanced.Success,
		"version":          enhanced.Version,
		"messageType":      enhanced.MessageType,
		"enhancedSegments": enhanced.EnhancedSegments,
		"segmentGroups":    enhanced.SegmentGroups,
		"observationGroups": enhanced.ObservationGroups,
		"segmentOrder":     enhanced.SegmentOrder,
		"parsedAt":         enhanced.ParsedAt,
		"dictionaryUsed":   enhanced.DictionaryUsed,
		"schemaLoaded":     enhanced.SchemaLoaded,
		"_format":          "hl7v2",
	}
	if len(enhanced.BasicSegments) > 0 {
		result["basicSegments"] = enhanced.BasicSegments
	}
	if len(enhanced.ValidationErrors) > 0 {
		result["validationErrors"] = enhanced.ValidationErrors
	}
	return result
}

// flattenHL7EnhancedFields converts enhancedSegments into the format-agnostic
// models.EnhancedField map keyed by "SEG.n" (e.g. "PID.5", "MSH.9").
// Segment order from the message is preserved in fieldOrder.
func flattenHL7EnhancedFields(enhanced *hl7.EnhancedParsedMessage) (map[string]*models.EnhancedField, []string) {
	fields := make(map[string]*models.EnhancedField)
	var order []string

	for _, segName := range enhanced.SegmentOrder {
		seg, ok := enhanced.EnhancedSegments[segName]
		if !ok {
			continue
		}
		for _, f := range seg.Fields {
			field := &models.EnhancedField{
				Path:        f.Key,
				Value:       f.Value,
				Name:        f.Name,
				Description: f.Description,
				DataType:    f.DataType,
				Cardinality: f.Cardinality,
				Required:    f.Optionality == "R",
				HasValue:    f.HasValue,
				SchemaFound: f.Name != "",
			}
			if len(f.Subfields) > 0 {
				field.SubFields = make([]*models.EnhancedField, 0, len(f.Subfields))
				for _, sf := range f.Subfields {
					field.SubFields = append(field.SubFields, &models.EnhancedField{
						Path:        sf.Key,
						Value:       sf.Value,
						Name:        sf.Name,
						Description: sf.Description,
						DataType:    sf.DataType,
						Required:    sf.Usage == "R",
						HasValue:    sf.HasValue,
						SchemaFound: sf.Name != "",
					})
				}
			}
			fields[f.Key] = field
			order = append(order, f.Key)
		}
	}
	return fields, order
}

func extractHL7ControlID(enhanced *hl7.EnhancedParsedMessage) string {
	if msh, exists := enhanced.EnhancedSegments["MSH"]; exists {
		for _, field := range msh.Fields {
			if field.Position == 10 || field.Key == "MSH.10" {
				return field.Value
			}
		}
	}
	return ""
}

func extractHL7Warnings(errs []hl7.ValidationError) []string {
	var out []string
	for _, e := range errs {
		if e.Severity == "warning" {
			out = append(out, e.Message)
		}
	}
	return out
}

func extractHL7Errors(errs []hl7.ValidationError) []string {
	var out []string
	for _, e := range errs {
		if e.Severity == "error" || e.Severity == "" {
			out = append(out, e.Message)
		}
	}
	return out
}

// =====================================
// EXTRA METHODS (non-interface, kept for direct callers)
// =====================================

func (hp *HL7ParserService) GetSupportedVersions() []string {
	return []string{"2.3", "2.3.1", "2.4", "2.5", "2.5.1", "2.6", "2.7", "2.8"}
}

func (hp *HL7ParserService) ValidateMessage(rawContent string) (bool, []string) {
	var errs []string
	if !strings.HasPrefix(rawContent, "MSH|") &&
		!strings.Contains(rawContent, "\rMSH|") &&
		!strings.Contains(rawContent, "\nMSH|") {
		errs = append(errs, "Missing MSH segment at start")
		return false, errs
	}
	if len(rawContent) < 20 {
		errs = append(errs, "Message too short to be valid HL7")
		return false, errs
	}
	return true, errs
}

func (hp *HL7ParserService) GetSupportedFormat() models.MessageFormat {
	return models.FormatHL7v2
}

func (hp *HL7ParserService) ValidateStructure(rawContent string) (*models.ValidationResult, error) {
	isValid, errs := hp.ValidateMessage(rawContent)
	return &models.ValidationResult{IsValid: isValid, Errors: errs, Warnings: []string{}}, nil
}
