// internal/transformers/hl7_transformers.go
// HL7 specific transformers

package transformers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"ezhealthkonnect/processing/pkg"
)

// HL7ToFHIRTransformer converts HL7 messages to FHIR format
type HL7ToFHIRTransformer struct {
	*BaseTransformer
	mappingRules map[string]HL7ToFHIRMapping
	validator    *HL7Validator
}

// FHIRToHL7Transformer converts FHIR resources to HL7 format
type FHIRToHL7Transformer struct {
	*BaseTransformer
	mappingRules map[string]FHIRToHL7Mapping
	validator    *FHIRValidator
}

// HL7ToFHIRMapping defines mapping from HL7 to FHIR
type HL7ToFHIRMapping struct {
	MessageType   string                    `json:"messageType"`
	ResourceType  string                    `json:"resourceType"`
	FieldMappings []HL7ToFHIRFieldMapping   `json:"fieldMappings"`
	Templates     map[string]string         `json:"templates"`
	Rules         map[string]interface{}    `json:"rules"`
}

// HL7ToFHIRFieldMapping defines field-level mapping
type HL7ToFHIRFieldMapping struct {
	HL7Field      string      `json:"hl7Field"`      // e.g., "MSH.3"
	FHIRPath      string      `json:"fhirPath"`      // e.g., "MessageHeader.source.name"
	DataType      string      `json:"dataType"`      // string, date, code, etc.
	Required      bool        `json:"required"`
	Default       interface{} `json:"default"`
	Transformation string     `json:"transformation"` // transformation function name
	Condition     string      `json:"condition"`      // conditional mapping
}

// FHIRToHL7Mapping defines mapping from FHIR to HL7
type FHIRToHL7Mapping struct {
	ResourceType  string                    `json:"resourceType"`
	MessageType   string                    `json:"messageType"`
	FieldMappings []FHIRToHL7FieldMapping   `json:"fieldMappings"`
	Templates     map[string]string         `json:"templates"`
	Rules         map[string]interface{}    `json:"rules"`
}

// FHIRToHL7FieldMapping defines field-level mapping
type FHIRToHL7FieldMapping struct {
	FHIRPath      string      `json:"fhirPath"`
	HL7Field      string      `json:"hl7Field"`
	DataType      string      `json:"dataType"`
	Required      bool        `json:"required"`
	Default       interface{} `json:"default"`
	Transformation string     `json:"transformation"`
	Condition     string      `json:"condition"`
}

// HL7Message represents a parsed HL7 message
type HL7Message struct {
	MessageType string                            `json:"messageType"`
	Segments    map[string][]map[string]string   `json:"segments"`
	Raw         string                            `json:"raw"`
}

// FHIRResource represents a FHIR resource
type FHIRResource struct {
	ResourceType string                 `json:"resourceType"`
	ID           string                 `json:"id,omitempty"`
	Meta         map[string]interface{} `json:"meta,omitempty"`
	Data         map[string]interface{} `json:"data"`
	Raw          string                 `json:"raw"`
}

// HL7Validator validates HL7 messages
type HL7Validator struct {
	strictMode bool
}

// FHIRValidator validates FHIR resources
type FHIRValidator struct {
	strictMode bool
}

// NewHL7ToFHIRTransformer creates a new HL7 to FHIR transformer
func NewHL7ToFHIRTransformer(config TransformerConfig) (pkg.MessageTransformer, error) {
	base := NewBaseTransformer(config)
	base.sourceFormat = "HL7"
	base.targetFormat = "FHIR"

	transformer := &HL7ToFHIRTransformer{
		BaseTransformer: base,
		mappingRules:    make(map[string]HL7ToFHIRMapping),
		validator:       &HL7Validator{strictMode: false},
	}

	// Load mapping rules from config
	if err := transformer.loadMappingRules(config.Rules); err != nil {
		return nil, fmt.Errorf("failed to load mapping rules: %w", err)
	}

	return transformer, nil
}

// NewFHIRToHL7Transformer creates a new FHIR to HL7 transformer
func NewFHIRToHL7Transformer(config TransformerConfig) (pkg.MessageTransformer, error) {
	base := NewBaseTransformer(config)
	base.sourceFormat = "FHIR"
	base.targetFormat = "HL7"

	transformer := &FHIRToHL7Transformer{
		BaseTransformer: base,
		mappingRules:    make(map[string]FHIRToHL7Mapping),
		validator:       &FHIRValidator{strictMode: false},
	}

	// Load mapping rules from config
	if err := transformer.loadMappingRules(config.Rules); err != nil {
		return nil, fmt.Errorf("failed to load mapping rules: %w", err)
	}

	return transformer, nil
}

// Transform converts HL7 message to FHIR resource
func (h2f *HL7ToFHIRTransformer) Transform(ctx context.Context, message *pkg.UniversalMessage) (*pkg.UniversalMessage, error) {
	start := time.Now()

	// Check cache first
	cacheKey := CreateCacheKey(message.Content, h2f.rules)
	if cached, found := h2f.GetFromCache(cacheKey); found {
		result := pkg.NewUniversalMessage()
		result.Content = cached
		result.ContentType = "FHIR"
		h2f.UpdateStats(true, time.Since(start).Milliseconds())
		return result, nil
	}

	// Validate source
	if err := h2f.ValidateSource(message.Content, message.ContentType); err != nil {
		h2f.UpdateStats(false, time.Since(start).Milliseconds())
		return nil, fmt.Errorf("source validation failed: %w", err)
	}

	// Parse HL7 message
	hl7Msg, err := h2f.parseHL7Message(message.Content)
	if err != nil {
		h2f.UpdateStats(false, time.Since(start).Milliseconds())
		return nil, fmt.Errorf("failed to parse HL7 message: %w", err)
	}

	// Get mapping rules for message type
	mapping, exists := h2f.mappingRules[hl7Msg.MessageType]
	if !exists {
		// Use default mapping
		mapping = h2f.getDefaultMapping(hl7Msg.MessageType)
	}

	// Transform to FHIR
	fhirResource, err := h2f.transformToFHIR(hl7Msg, mapping)
	if err != nil {
		h2f.UpdateStats(false, time.Since(start).Milliseconds())
		return nil, fmt.Errorf("transformation failed: %w", err)
	}

	// Serialize FHIR resource
	fhirJSON, err := json.MarshalIndent(fhirResource.Data, "", "  ")
	if err != nil {
		h2f.UpdateStats(false, time.Since(start).Milliseconds())
		return nil, fmt.Errorf("failed to serialize FHIR resource: %w", err)
	}

	result := pkg.NewUniversalMessage()
	result.Content = string(fhirJSON)
	result.ContentType = "FHIR"
	result.CorrelationID = message.CorrelationID
	result.SourceInterface = message.SourceInterface
	result.TransformationApplied = "HL7_TO_FHIR"

	// Copy metadata and add transformation info
	for key, value := range message.Metadata {
		result.Metadata[key] = value
	}
	result.Metadata["original_format"] = "HL7"
	result.Metadata["hl7_message_type"] = hl7Msg.MessageType
	result.Metadata["fhir_resource_type"] = fhirResource.ResourceType
	result.Metadata["transformation_time_ms"] = time.Since(start).Milliseconds()

	// Cache result
	h2f.PutInCache(cacheKey, result.Content)

	// Validate result
	if err := h2f.ValidateTarget(result.Content, result.ContentType); err != nil {
		h2f.UpdateStats(false, time.Since(start).Milliseconds())
		return nil, fmt.Errorf("target validation failed: %w", err)
	}

	h2f.UpdateStats(true, time.Since(start).Milliseconds())
	return result, nil
}

// Transform converts FHIR resource to HL7 message
func (f2h *FHIRToHL7Transformer) Transform(ctx context.Context, message *pkg.UniversalMessage) (*pkg.UniversalMessage, error) {
	start := time.Now()

	// Check cache first
	cacheKey := CreateCacheKey(message.Content, f2h.rules)
	if cached, found := f2h.GetFromCache(cacheKey); found {
		result := pkg.NewUniversalMessage()
		result.Content = cached
		result.ContentType = "HL7"
		f2h.UpdateStats(true, time.Since(start).Milliseconds())
		return result, nil
	}

	// Validate source
	if err := f2h.ValidateSource(message.Content, message.ContentType); err != nil {
		f2h.UpdateStats(false, time.Since(start).Milliseconds())
		return nil, fmt.Errorf("source validation failed: %w", err)
	}

	// Parse FHIR resource
	fhirResource, err := f2h.parseFHIRResource(message.Content)
	if err != nil {
		f2h.UpdateStats(false, time.Since(start).Milliseconds())
		return nil, fmt.Errorf("failed to parse FHIR resource: %w", err)
	}

	// Get mapping rules for resource type
	mapping, exists := f2h.mappingRules[fhirResource.ResourceType]
	if !exists {
		// Use default mapping
		mapping = f2h.getDefaultMapping(fhirResource.ResourceType)
	}

	// Transform to HL7
	hl7Message, err := f2h.transformToHL7(fhirResource, mapping)
	if err != nil {
		f2h.UpdateStats(false, time.Since(start).Milliseconds())
		return nil, fmt.Errorf("transformation failed: %w", err)
	}

	result := pkg.NewUniversalMessage()
	result.Content = hl7Message
	result.ContentType = "HL7"
	result.CorrelationID = message.CorrelationID
	result.SourceInterface = message.SourceInterface
	result.TransformationApplied = "FHIR_TO_HL7"

	// Copy metadata and add transformation info
	for key, value := range message.Metadata {
		result.Metadata[key] = value
	}
	result.Metadata["original_format"] = "FHIR"
	result.Metadata["fhir_resource_type"] = fhirResource.ResourceType
	result.Metadata["hl7_message_type"] = mapping.MessageType
	result.Metadata["transformation_time_ms"] = time.Since(start).Milliseconds()

	// Cache result
	f2h.PutInCache(cacheKey, result.Content)

	// Validate result
	if err := f2h.ValidateTarget(result.Content, result.ContentType); err != nil {
		f2h.UpdateStats(false, time.Since(start).Milliseconds())
		return nil, fmt.Errorf("target validation failed: %w", err)
	}

	f2h.UpdateStats(true, time.Since(start).Milliseconds())
	return result, nil
}

// parseHL7Message parses an HL7 message string
func (h2f *HL7ToFHIRTransformer) parseHL7Message(content string) (*HL7Message, error) {
	if !strings.HasPrefix(content, "MSH|") {
		return nil, fmt.Errorf("invalid HL7 message: must start with MSH segment")
	}

	lines := strings.Split(strings.ReplaceAll(content, "\r", ""), "\n")
	segments := make(map[string][]map[string]string)

	var messageType string

	for _, line := range lines {
		if line == "" {
			continue
		}

		fields := strings.Split(line, "|")
		if len(fields) < 1 {
			continue
		}

		segmentType := fields[0]

		// Extract message type from MSH segment
		if segmentType == "MSH" && len(fields) >= 9 {
			messageTypeField := fields[8]
			if len(messageTypeField) >= 7 {
				messageType = messageTypeField[:7] // e.g., "ADT^A01"
			}
		}

		// Parse segment fields
		segmentData := make(map[string]string)
		for i, field := range fields {
			segmentData[fmt.Sprintf("%d", i)] = field
		}

		if _, exists := segments[segmentType]; !exists {
			segments[segmentType] = []map[string]string{}
		}
		segments[segmentType] = append(segments[segmentType], segmentData)
	}

	return &HL7Message{
		MessageType: messageType,
		Segments:    segments,
		Raw:         content,
	}, nil
}

// parseFHIRResource parses a FHIR resource JSON
func (f2h *FHIRToHL7Transformer) parseFHIRResource(content string) (*FHIRResource, error) {
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(content), &data); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	resourceType, ok := data["resourceType"].(string)
	if !ok {
		return nil, fmt.Errorf("missing or invalid resourceType")
	}

	resource := &FHIRResource{
		ResourceType: resourceType,
		Data:         data,
		Raw:          content,
	}

	if id, exists := data["id"]; exists {
		if idStr, ok := id.(string); ok {
			resource.ID = idStr
		}
	}

	if meta, exists := data["meta"]; exists {
		if metaMap, ok := meta.(map[string]interface{}); ok {
			resource.Meta = metaMap
		}
	}

	return resource, nil
}

// transformToFHIR transforms HL7 message to FHIR resource
func (h2f *HL7ToFHIRTransformer) transformToFHIR(hl7Msg *HL7Message, mapping HL7ToFHIRMapping) (*FHIRResource, error) {
	fhirData := make(map[string]interface{})
	fhirData["resourceType"] = mapping.ResourceType

	// Apply field mappings
	for _, fieldMapping := range mapping.FieldMappings {
		value, err := h2f.extractHL7Field(hl7Msg, fieldMapping.HL7Field)
		if err != nil && fieldMapping.Required {
			return nil, fmt.Errorf("required field %s not found: %w", fieldMapping.HL7Field, err)
		}

		if value == "" && fieldMapping.Default != nil {
			value = fmt.Sprintf("%v", fieldMapping.Default)
		}

		if value != "" {
			// Apply transformation if specified
			if fieldMapping.Transformation != "" {
				if transformed, err := h2f.applyTransformation(value, fieldMapping.Transformation); err == nil {
					value = transformed
				}
			}

			// Set the value in FHIR data
			if err := h2f.setFHIRField(fhirData, fieldMapping.FHIRPath, value, fieldMapping.DataType); err != nil {
				return nil, fmt.Errorf("failed to set FHIR field %s: %w", fieldMapping.FHIRPath, err)
			}
		}
	}

	return &FHIRResource{
		ResourceType: mapping.ResourceType,
		Data:         fhirData,
	}, nil
}

// transformToHL7 transforms FHIR resource to HL7 message
func (f2h *FHIRToHL7Transformer) transformToHL7(fhirResource *FHIRResource, mapping FHIRToHL7Mapping) (string, error) {
	var hl7Lines []string

	// Start with MSH segment
	mshSegment := f2h.buildMSHSegment(mapping.MessageType)
	hl7Lines = append(hl7Lines, mshSegment)

	// Apply field mappings to build other segments
	segmentData := make(map[string]map[string]string)

	for _, fieldMapping := range mapping.FieldMappings {
		value, err := f2h.extractFHIRField(fhirResource, fieldMapping.FHIRPath)
		if err != nil && fieldMapping.Required {
			return "", fmt.Errorf("required field %s not found: %w", fieldMapping.FHIRPath, err)
		}

		if value == "" && fieldMapping.Default != nil {
			value = fmt.Sprintf("%v", fieldMapping.Default)
		}

		if value != "" {
			// Apply transformation if specified
			if fieldMapping.Transformation != "" {
				if transformed, err := f2h.applyTransformation(value, fieldMapping.Transformation); err == nil {
					value = transformed
				}
			}

			// Parse HL7 field to determine segment and position
			segmentType, fieldNum, err := f2h.parseHL7Field(fieldMapping.HL7Field)
			if err != nil {
				continue
			}

			if _, exists := segmentData[segmentType]; !exists {
				segmentData[segmentType] = make(map[string]string)
			}
			segmentData[segmentType][fieldNum] = value
		}
	}

	// Build segments from collected data
	for segmentType, fields := range segmentData {
		if segmentType == "MSH" {
			continue // Already added
		}
		segment := f2h.buildSegment(segmentType, fields)
		hl7Lines = append(hl7Lines, segment)
	}

	return strings.Join(hl7Lines, "\r\n"), nil
}

// Helper methods

func (h2f *HL7ToFHIRTransformer) extractHL7Field(hl7Msg *HL7Message, fieldPath string) (string, error) {
	// Parse field path like "MSH.3" or "PID.5.1"
	parts := strings.Split(fieldPath, ".")
	if len(parts) < 2 {
		return "", fmt.Errorf("invalid field path: %s", fieldPath)
	}

	segmentType := parts[0]
	fieldNum := parts[1]

	segments, exists := hl7Msg.Segments[segmentType]
	if !exists || len(segments) == 0 {
		return "", fmt.Errorf("segment %s not found", segmentType)
	}

	// Use first occurrence of segment
	segment := segments[0]
	if value, exists := segment[fieldNum]; exists {
		return value, nil
	}

	return "", fmt.Errorf("field %s not found in segment %s", fieldNum, segmentType)
}

func (h2f *HL7ToFHIRTransformer) setFHIRField(fhirData map[string]interface{}, fieldPath, value, dataType string) error {
	// Simple implementation - supports basic paths like "source.name"
	parts := strings.Split(fieldPath, ".")
	current := fhirData

	for i, part := range parts {
		if i == len(parts)-1 {
			// Last part - set the value
			switch dataType {
			case "date":
				// Convert HL7 date format to FHIR date format
				current[part] = h2f.convertDate(value)
			case "boolean":
				current[part] = value == "Y" || value == "true" || value == "1"
			case "integer":
				current[part] = value
			default:
				current[part] = value
			}
		} else {
			// Intermediate part - ensure nested object exists
			if _, exists := current[part]; !exists {
				current[part] = make(map[string]interface{})
			}
			if next, ok := current[part].(map[string]interface{}); ok {
				current = next
			} else {
				return fmt.Errorf("field path conflict at %s", part)
			}
		}
	}

	return nil
}

func (h2f *HL7ToFHIRTransformer) convertDate(hl7Date string) string {
	// Convert HL7 date format (YYYYMMDD or YYYYMMDDHHMMSS) to FHIR date format
	if len(hl7Date) >= 8 {
		year := hl7Date[0:4]
		month := hl7Date[4:6]
		day := hl7Date[6:8]
		return fmt.Sprintf("%s-%s-%s", year, month, day)
	}
	return hl7Date
}

func (h2f *HL7ToFHIRTransformer) applyTransformation(value, transformation string) (string, error) {
	functions := GetStandardFunctions()
	if fn, exists := functions[transformation]; exists {
		if result, err := fn(value); err == nil {
			return fmt.Sprintf("%v", result), nil
		}
	}
	return value, nil
}

func (f2h *FHIRToHL7Transformer) extractFHIRField(fhirResource *FHIRResource, fieldPath string) (string, error) {
	// Navigate through nested JSON structure
	parts := strings.Split(fieldPath, ".")
	current := fhirResource.Data

	for _, part := range parts {
		if next, exists := current[part]; exists {
			if nextMap, ok := next.(map[string]interface{}); ok {
				current = nextMap
			} else {
				return fmt.Sprintf("%v", next), nil
			}
		} else {
			return "", fmt.Errorf("field %s not found", part)
		}
	}

	return "", fmt.Errorf("unexpected end of field path")
}

func (f2h *FHIRToHL7Transformer) parseHL7Field(fieldPath string) (string, string, error) {
	parts := strings.Split(fieldPath, ".")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid HL7 field path: %s", fieldPath)
	}
	return parts[0], parts[1], nil
}

func (f2h *FHIRToHL7Transformer) buildMSHSegment(messageType string) string {
	timestamp := time.Now().Format("20060102150405")
	return fmt.Sprintf("MSH|^~\\&|SENDER||RECEIVER||%s||%s||P|2.5", timestamp, messageType)
}

func (f2h *FHIRToHL7Transformer) buildSegment(segmentType string, fields map[string]string) string {
	// Simple segment building - would need more sophisticated logic for real implementation
	var parts []string
	parts = append(parts, segmentType)

	// Add fields in order
	for i := 1; i <= 20; i++ { // Assuming max 20 fields
		fieldNum := fmt.Sprintf("%d", i)
		if value, exists := fields[fieldNum]; exists {
			parts = append(parts, value)
		} else {
			parts = append(parts, "")
		}
	}

	return strings.Join(parts, "|")
}

func (f2h *FHIRToHL7Transformer) applyTransformation(value, transformation string) (string, error) {
	functions := GetStandardFunctions()
	if fn, exists := functions[transformation]; exists {
		if result, err := fn(value); err == nil {
			return fmt.Sprintf("%v", result), nil
		}
	}
	return value, nil
}

// loadMappingRules loads mapping rules from configuration
func (h2f *HL7ToFHIRTransformer) loadMappingRules(rules map[string]interface{}) error {
	// Load from rules configuration
	// This would typically load from database or configuration files
	// For now, add default mappings

	h2f.mappingRules["ADT^A01"] = HL7ToFHIRMapping{
		MessageType:  "ADT^A01",
		ResourceType: "Patient",
		FieldMappings: []HL7ToFHIRFieldMapping{
			{HL7Field: "PID.3", FHIRPath: "identifier.value", DataType: "string", Required: true},
			{HL7Field: "PID.5.1", FHIRPath: "name.family", DataType: "string", Required: true},
			{HL7Field: "PID.5.2", FHIRPath: "name.given", DataType: "string"},
			{HL7Field: "PID.7", FHIRPath: "birthDate", DataType: "date"},
			{HL7Field: "PID.8", FHIRPath: "gender", DataType: "string"},
		},
	}

	return nil
}

func (f2h *FHIRToHL7Transformer) loadMappingRules(rules map[string]interface{}) error {
	// Load FHIR to HL7 mappings
	f2h.mappingRules["Patient"] = FHIRToHL7Mapping{
		ResourceType: "Patient",
		MessageType:  "ADT^A01",
		FieldMappings: []FHIRToHL7FieldMapping{
			{FHIRPath: "identifier.value", HL7Field: "PID.3", DataType: "string", Required: true},
			{FHIRPath: "name.family", HL7Field: "PID.5.1", DataType: "string", Required: true},
			{FHIRPath: "name.given", HL7Field: "PID.5.2", DataType: "string"},
			{FHIRPath: "birthDate", HL7Field: "PID.7", DataType: "date"},
			{FHIRPath: "gender", HL7Field: "PID.8", DataType: "string"},
		},
	}

	return nil
}

func (h2f *HL7ToFHIRTransformer) getDefaultMapping(messageType string) HL7ToFHIRMapping {
	return HL7ToFHIRMapping{
		MessageType:   messageType,
		ResourceType:  "Bundle",
		FieldMappings: []HL7ToFHIRFieldMapping{},
	}
}

func (f2h *FHIRToHL7Transformer) getDefaultMapping(resourceType string) FHIRToHL7Mapping {
	return FHIRToHL7Mapping{
		ResourceType:  resourceType,
		MessageType:   "ADT^A01",
		FieldMappings: []FHIRToHL7FieldMapping{},
	}
}