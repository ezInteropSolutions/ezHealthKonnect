// services/transformation_json_extended.go
// Extended JSON Transformation Service Methods
//
// 🎯 PURPOSE: Extended JSON transformation, validation, and utility methods
package services

import (
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// =====================================
// JSON TRANSFORMATION LOGIC
// =====================================

// transformJSONData performs the actual JSON transformation
func (s *JSONTransformationService) transformJSONData(request *JSONTransformRequest) (map[string]interface{}, JSONMappingStatistics, error) {
	stats := JSONMappingStatistics{}

	// Determine transformation approach
	switch request.TargetFormat {
	case MessageTypeJSON:
		return s.transformJSONToJSON(request, &stats)
	case MessageTypeFHIR:
		return s.transformJSONToFHIR(request, &stats)
	case MessageTypeHL7:
		return s.transformJSONToHL7(request, &stats)
	case MessageTypeXML:
		return s.transformJSONToXML(request, &stats)
	default:
		return nil, stats, fmt.Errorf("unsupported target format: %s", request.TargetFormat)
	}
}

// transformJSONToJSON performs JSON-to-JSON transformation
func (s *JSONTransformationService) transformJSONToJSON(request *JSONTransformRequest, stats *JSONMappingStatistics) (map[string]interface{}, JSONMappingStatistics, error) {
	sourceData := request.SourceJSON

	// If no mapping rules specified, apply default structure preservation or cleanup
	if len(request.MappingRules) == 0 {
		if request.PreserveStructure {
			return s.preserveJSONStructure(sourceData, request, stats)
		} else {
			return s.normalizeJSONStructure(sourceData, request, stats)
		}
	}

	// Apply custom mapping rules
	return s.applyMappingRules(sourceData, request.MappingRules, stats)
}

// transformJSONToFHIR transforms JSON to FHIR format
func (s *JSONTransformationService) transformJSONToFHIR(request *JSONTransformRequest, stats *JSONMappingStatistics) (map[string]interface{}, JSONMappingStatistics, error) {
	sourceData := request.SourceJSON

	// Use template if available
	if template := s.templateEngine.templates["json-to-fhir-generic"]; template != nil {
		return s.applyTemplate(sourceData, template, stats)
	}

	// Use mapping rules
	mappingRules := s.mappingEngine.mappingRules["JSON_TO_FHIR"]
	if len(mappingRules) > 0 {
		return s.applyMappingRules(sourceData, mappingRules, stats)
	}

	// Fallback to basic FHIR structure
	return s.createBasicFHIRFromJSON(sourceData, stats)
}

// transformJSONToHL7 transforms JSON to HL7 format
func (s *JSONTransformationService) transformJSONToHL7(request *JSONTransformRequest, stats *JSONMappingStatistics) (map[string]interface{}, JSONMappingStatistics, error) {
	sourceData := request.SourceJSON

	// Use template if available
	if template := s.templateEngine.templates["json-to-hl7-generic"]; template != nil {
		return s.applyTemplate(sourceData, template, stats)
	}

	// Use mapping rules
	mappingRules := s.mappingEngine.mappingRules["JSON_TO_HL7"]
	if len(mappingRules) > 0 {
		return s.applyMappingRules(sourceData, mappingRules, stats)
	}

	// Fallback to basic HL7 structure
	return s.createBasicHL7FromJSON(sourceData, stats)
}

// transformJSONToXML transforms JSON to XML format
func (s *JSONTransformationService) transformJSONToXML(request *JSONTransformRequest, stats *JSONMappingStatistics) (map[string]interface{}, JSONMappingStatistics, error) {
	sourceData := request.SourceJSON

	// Create XML structure from JSON
	xmlData := map[string]interface{}{
		"xml": map[string]interface{}{
			"@version":  "1.0",
			"@encoding": "UTF-8",
			"root":      sourceData,
		},
		"format": "XML",
		"metadata": map[string]interface{}{
			"transformedFrom": "JSON",
			"transformedAt":   time.Now().Format(time.RFC3339),
		},
	}

	stats.TotalMappings = 1
	stats.SuccessfulMappings = 1
	stats.MappingSuccessRate = 100.0

	return xmlData, *stats, nil
}

// =====================================
// STRUCTURE TRANSFORMATION METHODS
// =====================================

// preserveJSONStructure preserves the original JSON structure with optional enhancements
func (s *JSONTransformationService) preserveJSONStructure(sourceData map[string]interface{}, request *JSONTransformRequest, stats *JSONMappingStatistics) (map[string]interface{}, JSONMappingStatistics, error) {
	result := make(map[string]interface{})

	// Deep copy source data
	if err := s.deepCopyMap(sourceData, result); err != nil {
		return nil, *stats, fmt.Errorf("failed to copy source data: %v", err)
	}

	// Add metadata if requested
	if request.IncludeMetadata {
		result["_metadata"] = map[string]interface{}{
			"transformedAt":   time.Now().Format(time.RFC3339),
			"transformedBy":   "JSONTransformationService",
			"sourceFormat":    "JSON",
			"targetFormat":    request.TargetFormat,
			"preservedStructure": true,
		}
	}

	// Flatten arrays if requested
	if request.FlattenArrays {
		s.flattenArraysInMap(result)
	}

	stats.TotalMappings = s.countMappableFields(sourceData)
	stats.SuccessfulMappings = stats.TotalMappings
	stats.MappingSuccessRate = 100.0

	return result, *stats, nil
}

// normalizeJSONStructure normalizes and cleans up JSON structure
func (s *JSONTransformationService) normalizeJSONStructure(sourceData map[string]interface{}, request *JSONTransformRequest, stats *JSONMappingStatistics) (map[string]interface{}, JSONMappingStatistics, error) {
	result := make(map[string]interface{})

	stats.TotalMappings = 0
	stats.SuccessfulMappings = 0

	// Normalize each field
	for key, value := range sourceData {
		stats.TotalMappings++

		normalizedKey := s.normalizeFieldName(key)
		normalizedValue, err := s.normalizeFieldValue(value)
		if err != nil {
			stats.FailedMappings++
			log.Printf("⚠️ Warning: Failed to normalize field %s: %v", key, err)
			continue
		}

		result[normalizedKey] = normalizedValue
		stats.SuccessfulMappings++
	}

	// Add metadata
	if request.IncludeMetadata {
		result["_metadata"] = map[string]interface{}{
			"transformedAt":   time.Now().Format(time.RFC3339),
			"transformedBy":   "JSONTransformationService",
			"sourceFormat":    "JSON",
			"targetFormat":    request.TargetFormat,
			"normalized":     true,
		}
	}

	stats.MappingSuccessRate = float64(stats.SuccessfulMappings) / float64(stats.TotalMappings) * 100.0

	return result, *stats, nil
}

// =====================================
// MAPPING RULES APPLICATION
// =====================================

// applyMappingRules applies JSON mapping rules to transform data
func (s *JSONTransformationService) applyMappingRules(sourceData map[string]interface{}, rules []JSONMappingRule, stats *JSONMappingStatistics) (map[string]interface{}, JSONMappingStatistics, error) {
	result := make(map[string]interface{})

	// Sort rules by priority
	sortedRules := s.sortRulesByPriority(rules)

	stats.TotalMappings = len(sortedRules)

	for _, rule := range sortedRules {
		if !rule.Enabled {
			continue
		}

		// Check condition if specified
		if rule.Condition != "" {
			conditionMet, err := s.evaluateCondition(rule.Condition, sourceData)
			if err != nil {
				log.Printf("⚠️ Warning: Failed to evaluate condition for rule %s: %v", rule.ID, err)
				stats.FailedMappings++
				continue
			}
			if !conditionMet {
				stats.ConditionalMappings++
				continue
			}
		}

		// Extract source value
		sourceValue, found := s.extractValueByPath(sourceData, rule.SourcePath)
		if !found {
			if rule.DefaultValue != nil {
				sourceValue = rule.DefaultValue
				stats.DefaultValuesApplied++
			} else {
				stats.FailedMappings++
				continue
			}
		}

		// Apply transformation
		transformedValue, err := s.applyTransformation(sourceValue, rule.Transform, rule.DataType)
		if err != nil {
			log.Printf("⚠️ Warning: Failed to apply transformation for rule %s: %v", rule.ID, err)
			stats.FailedMappings++
			continue
		}

		// Apply data type conversion
		convertedValue, err := s.convertDataType(transformedValue, rule.DataType)
		if err != nil {
			log.Printf("⚠️ Warning: Failed to convert data type for rule %s: %v", rule.ID, err)
			stats.FailedMappings++
			continue
		}

		// Set target value
		if err := s.setValueByPath(result, rule.TargetPath, convertedValue); err != nil {
			log.Printf("⚠️ Warning: Failed to set target value for rule %s: %v", rule.ID, err)
			stats.FailedMappings++
			continue
		}

		stats.SuccessfulMappings++
		if rule.Transform != "" {
			stats.TransformationsApplied++
		}
		if s.requiresDataTypeConversion(transformedValue, rule.DataType) {
			stats.DataTypeConversions++
		}
	}

	stats.MappingSuccessRate = float64(stats.SuccessfulMappings) / float64(stats.TotalMappings) * 100.0

	return result, *stats, nil
}

// =====================================
// TEMPLATE APPLICATION
// =====================================

// applyTemplate applies a JSON template to transform data
func (s *JSONTransformationService) applyTemplate(sourceData map[string]interface{}, template *JSONTemplate, stats *JSONMappingStatistics) (map[string]interface{}, JSONMappingStatistics, error) {
	// Get or compile template
	compiled, err := s.getCompiledTemplate(template)
	if err != nil {
		return nil, *stats, fmt.Errorf("failed to compile template: %v", err)
	}

	// Apply template instructions
	result, err := s.executeTemplate(compiled, sourceData)
	if err != nil {
		return nil, *stats, fmt.Errorf("failed to execute template: %v", err)
	}

	// Update statistics
	stats.TotalMappings = len(compiled.Instructions)
	stats.SuccessfulMappings = stats.TotalMappings // Templates are assumed to succeed
	stats.MappingSuccessRate = 100.0

	return result, *stats, nil
}

// getCompiledTemplate gets or compiles a template
func (s *JSONTransformationService) getCompiledTemplate(template *JSONTemplate) (*CompiledTemplate, error) {
	// Check cache
	if compiled, exists := s.templateEngine.templateCache[template.ID]; exists {
		return compiled, nil
	}

	// Compile template
	compiled := &CompiledTemplate{
		ID:           template.ID,
		Template:     template,
		Instructions: []TemplateInstruction{},
		CompiledAt:   time.Now(),
	}

	// Parse template into instructions
	instructions, err := s.parseTemplateInstructions(template.Template)
	if err != nil {
		return nil, fmt.Errorf("failed to parse template instructions: %v", err)
	}

	compiled.Instructions = instructions

	// Cache compiled template
	s.templateEngine.templateCache[template.ID] = compiled

	return compiled, nil
}

// parseTemplateInstructions parses template into executable instructions
func (s *JSONTransformationService) parseTemplateInstructions(templateData map[string]interface{}) ([]TemplateInstruction, error) {
	var instructions []TemplateInstruction

	for targetPath, templateValue := range templateData {
		instruction := s.parseTemplateValue(targetPath, templateValue)
		instructions = append(instructions, instruction)
	}

	return instructions, nil
}

// parseTemplateValue parses a template value into an instruction
func (s *JSONTransformationService) parseTemplateValue(targetPath string, templateValue interface{}) TemplateInstruction {
	if templateStr, ok := templateValue.(string); ok {
		// Check for template expressions like {{.field | function}}
		if strings.Contains(templateStr, "{{") && strings.Contains(templateStr, "}}") {
			return TemplateInstruction{
				Operation: "transform",
				Target:    targetPath,
				Function:  s.extractTemplateFunction(templateStr),
				Metadata: map[string]interface{}{
					"expression": templateStr,
				},
			}
		}
	}

	// Simple copy operation
	return TemplateInstruction{
		Operation: "copy",
		Source:    s.extractSourcePath(templateValue),
		Target:    targetPath,
	}
}

// executeTemplate executes compiled template instructions
func (s *JSONTransformationService) executeTemplate(compiled *CompiledTemplate, sourceData map[string]interface{}) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	for _, instruction := range compiled.Instructions {
		switch instruction.Operation {
		case "copy":
			value, found := s.extractValueByPath(sourceData, instruction.Source)
			if found {
				s.setValueByPath(result, instruction.Target, value)
			}

		case "transform":
			expression := instruction.Metadata["expression"].(string)
			value, err := s.evaluateTemplateExpression(expression, sourceData)
			if err != nil {
				log.Printf("⚠️ Warning: Failed to evaluate template expression %s: %v", expression, err)
				continue
			}
			s.setValueByPath(result, instruction.Target, value)

		case "condition":
			// Handle conditional logic
			if instruction.Condition != "" {
				conditionMet, err := s.evaluateCondition(instruction.Condition, sourceData)
				if err == nil && conditionMet {
					value, found := s.extractValueByPath(sourceData, instruction.Source)
					if found {
						s.setValueByPath(result, instruction.Target, value)
					}
				}
			}
		}
	}

	return result, nil
}

// =====================================
// FALLBACK CREATION METHODS
// =====================================

// createBasicFHIRFromJSON creates basic FHIR structure from JSON
func (s *JSONTransformationService) createBasicFHIRFromJSON(sourceData map[string]interface{}, stats *JSONMappingStatistics) (map[string]interface{}, JSONMappingStatistics, error) {
	result := map[string]interface{}{
		"resourceType": s.inferFHIRResourceType(sourceData),
		"id":          s.generateFHIRID(sourceData),
		"meta": map[string]interface{}{
			"lastUpdated": time.Now().Format(time.RFC3339),
			"source":      "JSONTransformationService",
		},
		"text": map[string]interface{}{
			"status": "generated",
			"div":    "<div xmlns=\"http://www.w3.org/1999/xhtml\">Generated from JSON data</div>",
		},
	}

	// Map common fields
	fieldMappings := map[string]string{
		"name":        "name",
		"title":       "name",
		"description": "text.div",
		"type":        "type",
		"status":      "status",
		"active":      "active",
		"id":          "identifier.value",
		"identifier":  "identifier.value",
		"timestamp":   "meta.lastUpdated",
		"date":        "effectiveDateTime",
	}

	stats.TotalMappings = 0
	stats.SuccessfulMappings = 0

	for sourceField, targetField := range fieldMappings {
		if value, exists := sourceData[sourceField]; exists {
			stats.TotalMappings++
			if err := s.setValueByPath(result, targetField, value); err == nil {
				stats.SuccessfulMappings++
			} else {
				stats.FailedMappings++
			}
		}
	}

	// Add remaining fields as extensions
	for key, value := range sourceData {
		if _, mapped := fieldMappings[key]; !mapped {
			extension := map[string]interface{}{
				"url":   fmt.Sprintf("http://example.com/fhir/extension/%s", key),
				"valueString": fmt.Sprintf("%v", value),
			}

			if extensions, exists := result["extension"]; exists {
				if extArray, ok := extensions.([]interface{}); ok {
					result["extension"] = append(extArray, extension)
				}
			} else {
				result["extension"] = []interface{}{extension}
			}
		}
	}

	stats.MappingSuccessRate = float64(stats.SuccessfulMappings) / float64(stats.TotalMappings) * 100.0

	return result, *stats, nil
}

// createBasicHL7FromJSON creates basic HL7 structure from JSON
func (s *JSONTransformationService) createBasicHL7FromJSON(sourceData map[string]interface{}, stats *JSONMappingStatistics) (map[string]interface{}, JSONMappingStatistics, error) {
	result := map[string]interface{}{
		"messageHeader": map[string]interface{}{
			"sendingApplication":   s.extractStringValue(sourceData, []string{"source", "sender", "from"}, "JSON_APP"),
			"sendingFacility":     s.extractStringValue(sourceData, []string{"facility", "organization"}, "JSON_FACILITY"),
			"receivingApplication": s.extractStringValue(sourceData, []string{"target", "recipient", "to"}, "TARGET_APP"),
			"messageType": map[string]interface{}{
				"messageCode":      s.extractStringValue(sourceData, []string{"messageType", "type"}, "ADT"),
				"triggerEvent":     s.extractStringValue(sourceData, []string{"triggerEvent", "event"}, "A01"),
				"messageStructure": "ADT_A01",
			},
			"messageControlID": s.extractStringValue(sourceData, []string{"id", "messageId", "controlId"}, uuid.New().String()),
			"messageDateTime":  time.Now().Format("20060102150405"),
			"versionID":        "2.5",
		},
		"segments": []map[string]interface{}{
			{
				"name": "MSH",
				"fields": []map[string]interface{}{
					{"position": 1, "value": "|"},
					{"position": 2, "value": "^~\\&"},
				},
			},
		},
	}

	stats.TotalMappings = len(sourceData)
	stats.SuccessfulMappings = stats.TotalMappings
	stats.MappingSuccessRate = 100.0

	return result, *stats, nil
}

// =====================================
// VALIDATION METHODS
// =====================================

// validateSourceJSON validates source JSON data
func (s *JSONTransformationService) validateSourceJSON(sourceData map[string]interface{}, validationLevel string) ([]JSONValidationResult, error) {
	var results []JSONValidationResult

	// Basic structure validation
	structuralResults := s.validateJSONStructure(sourceData)
	results = append(results, structuralResults...)

	// Schema validation if available
	if schema := s.schemaRegistry.defaultSchemas[MessageTypeJSON]; schema != nil {
		schemaResults := s.validateAgainstSchema(sourceData, schema, validationLevel)
		results = append(results, schemaResults...)
	}

	// Data type validation
	typeResults := s.validateDataTypes(sourceData)
	results = append(results, typeResults...)

	return results, nil
}

// validateTransformedData validates transformed data against target schema
func (s *JSONTransformationService) validateTransformedData(transformedData map[string]interface{}, targetSchema, validationLevel string) ([]JSONValidationResult, error) {
	var results []JSONValidationResult

	// Load target schema
	schema, exists := s.schemaRegistry.schemas[targetSchema]
	if !exists {
		return results, fmt.Errorf("target schema not found: %s", targetSchema)
	}

	// Validate against schema
	schemaResults := s.validateAgainstSchema(transformedData, schema, validationLevel)
	results = append(results, schemaResults...)

	return results, nil
}

// validateJSONStructure performs basic JSON structure validation
func (s *JSONTransformationService) validateJSONStructure(data map[string]interface{}) []JSONValidationResult {
	var results []JSONValidationResult

	// Check for empty data
	if len(data) == 0 {
		results = append(results, JSONValidationResult{
			RuleID:    "structure-empty",
			RuleName:  "Non-empty Structure",
			Path:      "$",
			Severity:  "warning",
			Passed:    false,
			Message:   "JSON structure is empty",
			Timestamp: time.Now(),
		})
	}

	// Check for null values
	nullCount := s.countNullValues(data)
	if nullCount > 0 {
		results = append(results, JSONValidationResult{
			RuleID:    "structure-nulls",
			RuleName:  "Null Values Check",
			Path:      "$",
			Severity:  "info",
			Passed:    true,
			Message:   fmt.Sprintf("Found %d null values", nullCount),
			Value:     nullCount,
			Timestamp: time.Now(),
		})
	}

	// Check nesting depth
	depth := s.calculateNestingDepth(data)
	if depth > 10 {
		results = append(results, JSONValidationResult{
			RuleID:    "structure-depth",
			RuleName:  "Nesting Depth Check",
			Path:      "$",
			Severity:  "warning",
			Passed:    false,
			Message:   fmt.Sprintf("Deep nesting detected (depth: %d)", depth),
			Value:     depth,
			Expected:  10,
			Timestamp: time.Now(),
		})
	}

	return results
}

// validateAgainstSchema validates data against JSON schema
func (s *JSONTransformationService) validateAgainstSchema(data map[string]interface{}, schema *JSONSchema, validationLevel string) []JSONValidationResult {
	var results []JSONValidationResult

	// Check required fields
	for _, required := range schema.Required {
		if !s.hasFieldAtPath(data, required) {
			severity := "error"
			if validationLevel == "LENIENT" {
				severity = "warning"
			}

			results = append(results, JSONValidationResult{
				RuleID:    "schema-required",
				RuleName:  "Required Field",
				Path:      required,
				Severity:  severity,
				Passed:    false,
				Message:   fmt.Sprintf("Required field missing: %s", required),
				Expected:  "non-null value",
				Timestamp: time.Now(),
			})
		}
	}

	// Validate properties
	for fieldName, property := range schema.Properties {
		if value, exists := data[fieldName]; exists {
			propertyResults := s.validateProperty(value, property, fieldName, validationLevel)
			results = append(results, propertyResults...)
		}
	}

	return results
}

// validateProperty validates a single property against schema
func (s *JSONTransformationService) validateProperty(value interface{}, property *JSONProperty, path, validationLevel string) []JSONValidationResult {
	var results []JSONValidationResult

	// Type validation
	if !s.isValidType(value, property.Type) {
		results = append(results, JSONValidationResult{
			RuleID:    "schema-type",
			RuleName:  "Data Type Validation",
			Path:      path,
			Severity:  "error",
			Passed:    false,
			Message:   fmt.Sprintf("Invalid type for field %s", path),
			Value:     fmt.Sprintf("%T", value),
			Expected:  property.Type,
			Timestamp: time.Now(),
		})
	}

	// String validations
	if strValue, ok := value.(string); ok {
		// Pattern validation
		if property.Pattern != "" {
			if matched, _ := regexp.MatchString(property.Pattern, strValue); !matched {
				results = append(results, JSONValidationResult{
					RuleID:    "schema-pattern",
					RuleName:  "Pattern Validation",
					Path:      path,
					Severity:  "error",
					Passed:    false,
					Message:   fmt.Sprintf("Value does not match pattern: %s", property.Pattern),
					Value:     strValue,
					Expected:  property.Pattern,
					Timestamp: time.Now(),
				})
			}
		}

		// Length validations
		if property.MinLength != nil && len(strValue) < *property.MinLength {
			results = append(results, JSONValidationResult{
				RuleID:    "schema-minlength",
				RuleName:  "Minimum Length",
				Path:      path,
				Severity:  "error",
				Passed:    false,
				Message:   fmt.Sprintf("String too short (min: %d, actual: %d)", *property.MinLength, len(strValue)),
				Value:     len(strValue),
				Expected:  *property.MinLength,
				Timestamp: time.Now(),
			})
		}

		if property.MaxLength != nil && len(strValue) > *property.MaxLength {
			results = append(results, JSONValidationResult{
				RuleID:    "schema-maxlength",
				RuleName:  "Maximum Length",
				Path:      path,
				Severity:  "error",
				Passed:    false,
				Message:   fmt.Sprintf("String too long (max: %d, actual: %d)", *property.MaxLength, len(strValue)),
				Value:     len(strValue),
				Expected:  *property.MaxLength,
				Timestamp: time.Now(),
			})
		}
	}

	// Numeric validations
	if numValue, ok := s.convertToFloat64(value); ok {
		if property.Minimum != nil && numValue < *property.Minimum {
			results = append(results, JSONValidationResult{
				RuleID:    "schema-minimum",
				RuleName:  "Minimum Value",
				Path:      path,
				Severity:  "error",
				Passed:    false,
				Message:   fmt.Sprintf("Value below minimum (min: %f, actual: %f)", *property.Minimum, numValue),
				Value:     numValue,
				Expected:  *property.Minimum,
				Timestamp: time.Now(),
			})
		}

		if property.Maximum != nil && numValue > *property.Maximum {
			results = append(results, JSONValidationResult{
				RuleID:    "schema-maximum",
				RuleName:  "Maximum Value",
				Path:      path,
				Severity:  "error",
				Passed:    false,
				Message:   fmt.Sprintf("Value above maximum (max: %f, actual: %f)", *property.Maximum, numValue),
				Value:     numValue,
				Expected:  *property.Maximum,
				Timestamp: time.Now(),
			})
		}
	}

	// Enum validation
	if len(property.Enum) > 0 {
		if !s.isInEnum(value, property.Enum) {
			results = append(results, JSONValidationResult{
				RuleID:    "schema-enum",
				RuleName:  "Enumeration Value",
				Path:      path,
				Severity:  "error",
				Passed:    false,
				Message:   fmt.Sprintf("Value not in allowed enumeration"),
				Value:     value,
				Expected:  property.Enum,
				Timestamp: time.Now(),
			})
		}
	}

	return results
}

// validateDataTypes performs data type validation
func (s *JSONTransformationService) validateDataTypes(data map[string]interface{}) []JSONValidationResult {
	var results []JSONValidationResult

	for key, value := range data {
		// Check for inconsistent types in arrays
		if array, ok := value.([]interface{}); ok {
			if !s.hasConsistentTypes(array) {
				results = append(results, JSONValidationResult{
					RuleID:    "datatype-array-consistency",
					RuleName:  "Array Type Consistency",
					Path:      key,
					Severity:  "warning",
					Passed:    false,
					Message:   "Array contains inconsistent data types",
					Value:     fmt.Sprintf("%d items", len(array)),
					Timestamp: time.Now(),
				})
			}
		}

		// Check for potential data type issues
		if strValue, ok := value.(string); ok {
			if s.looksLikeNumber(strValue) {
				results = append(results, JSONValidationResult{
					RuleID:    "datatype-string-number",
					RuleName:  "String Number Detection",
					Path:      key,
					Severity:  "info",
					Passed:    true,
					Message:   "String value looks like a number",
					Value:     strValue,
					Suggestion: "Consider converting to numeric type",
					Timestamp: time.Now(),
				})
			}

			if s.looksLikeBoolean(strValue) {
				results = append(results, JSONValidationResult{
					RuleID:    "datatype-string-boolean",
					RuleName:  "String Boolean Detection",
					Path:      key,
					Severity:  "info",
					Passed:    true,
					Message:   "String value looks like a boolean",
					Value:     strValue,
					Suggestion: "Consider converting to boolean type",
					Timestamp: time.Now(),
				})
			}
		}
	}

	return results
}

// Continue with remaining utility methods in next part...