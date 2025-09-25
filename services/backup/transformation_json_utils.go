// services/transformation_json_utils.go
// JSON Transformation Utility Methods
//
// 🎯 PURPOSE: Utility methods for JSON transformation, validation, and processing
package services

import (
	"encoding/json"
	"fmt"
	"log"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// =====================================
// TRANSFORMATION FUNCTION INITIALIZATION
// =====================================

// initializeTransformFunctions sets up transformation functions
func (engine *JSONMappingEngine) initializeTransformFunctions() {
	engine.transformFunctions["uppercase"] = func(value interface{}, params map[string]interface{}) (interface{}, error) {
		return strings.ToUpper(fmt.Sprintf("%v", value)), nil
	}

	engine.transformFunctions["lowercase"] = func(value interface{}, params map[string]interface{}) (interface{}, error) {
		return strings.ToLower(fmt.Sprintf("%v", value)), nil
	}

	engine.transformFunctions["capitalize"] = func(value interface{}, params map[string]interface{}) (interface{}, error) {
		str := fmt.Sprintf("%v", value)
		if len(str) == 0 {
			return str, nil
		}
		return strings.ToUpper(str[:1]) + strings.ToLower(str[1:]), nil
	}

	engine.transformFunctions["trim"] = func(value interface{}, params map[string]interface{}) (interface{}, error) {
		return strings.TrimSpace(fmt.Sprintf("%v", value)), nil
	}

	engine.transformFunctions["sanitize_fhir_id"] = func(value interface{}, params map[string]interface{}) (interface{}, error) {
		str := fmt.Sprintf("%v", value)
		// Remove invalid characters for FHIR ID
		reg := regexp.MustCompile(`[^a-zA-Z0-9\-\.]`)
		sanitized := reg.ReplaceAllString(str, "")
		if len(sanitized) > 64 {
			sanitized = sanitized[:64]
		}
		if len(sanitized) == 0 {
			sanitized = "generated-id"
		}
		return sanitized, nil
	}

	engine.transformFunctions["iso_datetime"] = func(value interface{}, params map[string]interface{}) (interface{}, error) {
		switch v := value.(type) {
		case string:
			// Try to parse various formats and convert to ISO
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				return t.Format(time.RFC3339), nil
			}
			if t, err := time.Parse("2006-01-02 15:04:05", v); err == nil {
				return t.Format(time.RFC3339), nil
			}
			if t, err := time.Parse("20060102150405", v); err == nil {
				return t.Format(time.RFC3339), nil
			}
			return v, nil
		case time.Time:
			return v.Format(time.RFC3339), nil
		default:
			return time.Now().Format(time.RFC3339), nil
		}
	}

	engine.transformFunctions["hl7_timestamp"] = func(value interface{}, params map[string]interface{}) (interface{}, error) {
		switch v := value.(type) {
		case string:
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				return t.Format("20060102150405"), nil
			}
			return v, nil
		case time.Time:
			return v.Format("20060102150405"), nil
		default:
			return time.Now().Format("20060102150405"), nil
		}
	}

	engine.transformFunctions["default"] = func(value interface{}, params map[string]interface{}) (interface{}, error) {
		if value == nil || value == "" {
			if defaultValue, exists := params["defaultValue"]; exists {
				return defaultValue, nil
			}
		}
		return value, nil
	}

	engine.transformFunctions["uuid"] = func(value interface{}, params map[string]interface{}) (interface{}, error) {
		return generateUUID(), nil
	}

	engine.transformFunctions["concat"] = func(value interface{}, params map[string]interface{}) (interface{}, error) {
		var parts []string

		// Add the main value
		parts = append(parts, fmt.Sprintf("%v", value))

		// Add additional values from params
		if additional, exists := params["additional"]; exists {
			if additionalArray, ok := additional.([]interface{}); ok {
				for _, item := range additionalArray {
					parts = append(parts, fmt.Sprintf("%v", item))
				}
			}
		}

		separator := ""
		if sep, exists := params["separator"]; exists {
			separator = fmt.Sprintf("%v", sep)
		}

		return strings.Join(parts, separator), nil
	}

	engine.transformFunctions["substring"] = func(value interface{}, params map[string]interface{}) (interface{}, error) {
		str := fmt.Sprintf("%v", value)

		start := 0
		if startParam, exists := params["start"]; exists {
			if startInt, ok := startParam.(int); ok {
				start = startInt
			}
		}

		length := len(str) - start
		if lengthParam, exists := params["length"]; exists {
			if lengthInt, ok := lengthParam.(int); ok {
				length = lengthInt
			}
		}

		if start < 0 || start >= len(str) {
			return "", nil
		}

		end := start + length
		if end > len(str) {
			end = len(str)
		}

		return str[start:end], nil
	}

	engine.transformFunctions["replace"] = func(value interface{}, params map[string]interface{}) (interface{}, error) {
		str := fmt.Sprintf("%v", value)

		oldValue := ""
		if old, exists := params["old"]; exists {
			oldValue = fmt.Sprintf("%v", old)
		}

		newValue := ""
		if newVal, exists := params["new"]; exists {
			newValue = fmt.Sprintf("%v", newVal)
		}

		return strings.ReplaceAll(str, oldValue, newValue), nil
	}
}

// initializeOperators sets up condition evaluation operators
func (engine *JSONMappingEngine) initializeOperators() {
	engine.conditionEvaluator.operators["eq"] = func(a, b interface{}) bool {
		return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
	}

	engine.conditionEvaluator.operators["ne"] = func(a, b interface{}) bool {
		return fmt.Sprintf("%v", a) != fmt.Sprintf("%v", b)
	}

	engine.conditionEvaluator.operators["gt"] = func(a, b interface{}) bool {
		aNum, aOk := convertToFloat64(a)
		bNum, bOk := convertToFloat64(b)
		if aOk && bOk {
			return aNum > bNum
		}
		return fmt.Sprintf("%v", a) > fmt.Sprintf("%v", b)
	}

	engine.conditionEvaluator.operators["lt"] = func(a, b interface{}) bool {
		aNum, aOk := convertToFloat64(a)
		bNum, bOk := convertToFloat64(b)
		if aOk && bOk {
			return aNum < bNum
		}
		return fmt.Sprintf("%v", a) < fmt.Sprintf("%v", b)
	}

	engine.conditionEvaluator.operators["gte"] = func(a, b interface{}) bool {
		aNum, aOk := convertToFloat64(a)
		bNum, bOk := convertToFloat64(b)
		if aOk && bOk {
			return aNum >= bNum
		}
		return fmt.Sprintf("%v", a) >= fmt.Sprintf("%v", b)
	}

	engine.conditionEvaluator.operators["lte"] = func(a, b interface{}) bool {
		aNum, aOk := convertToFloat64(a)
		bNum, bOk := convertToFloat64(b)
		if aOk && bOk {
			return aNum <= bNum
		}
		return fmt.Sprintf("%v", a) <= fmt.Sprintf("%v", b)
	}

	engine.conditionEvaluator.operators["contains"] = func(a, b interface{}) bool {
		return strings.Contains(fmt.Sprintf("%v", a), fmt.Sprintf("%v", b))
	}

	engine.conditionEvaluator.operators["starts_with"] = func(a, b interface{}) bool {
		return strings.HasPrefix(fmt.Sprintf("%v", a), fmt.Sprintf("%v", b))
	}

	engine.conditionEvaluator.operators["ends_with"] = func(a, b interface{}) bool {
		return strings.HasSuffix(fmt.Sprintf("%v", a), fmt.Sprintf("%v", b))
	}

	engine.conditionEvaluator.operators["exists"] = func(a, b interface{}) bool {
		return a != nil
	}

	engine.conditionEvaluator.operators["not_exists"] = func(a, b interface{}) bool {
		return a == nil
	}
}

// initializeCustomValidators sets up custom validation functions
func (validator *JSONValidator) initializeCustomValidators() {
	validator.customValidators["email"] = func(value interface{}, rule JSONValidationRule) (bool, string) {
		str := fmt.Sprintf("%v", value)
		emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
		if emailRegex.MatchString(str) {
			return true, ""
		}
		return false, "Invalid email format"
	}

	validator.customValidators["phone"] = func(value interface{}, rule JSONValidationRule) (bool, string) {
		str := fmt.Sprintf("%v", value)
		// Simple phone validation - digits, spaces, hyphens, parentheses, plus
		phoneRegex := regexp.MustCompile(`^[\d\s\-\(\)\+]+$`)
		if phoneRegex.MatchString(str) && len(strings.ReplaceAll(strings.ReplaceAll(str, " ", ""), "-", "")) >= 10 {
			return true, ""
		}
		return false, "Invalid phone number format"
	}

	validator.customValidators["url"] = func(value interface{}, rule JSONValidationRule) (bool, string) {
		str := fmt.Sprintf("%v", value)
		urlRegex := regexp.MustCompile(`^https?://[^\s/$.?#].[^\s]*$`)
		if urlRegex.MatchString(str) {
			return true, ""
		}
		return false, "Invalid URL format"
	}

	validator.customValidators["date_iso"] = func(value interface{}, rule JSONValidationRule) (bool, string) {
		str := fmt.Sprintf("%v", value)
		if _, err := time.Parse("2006-01-02", str); err == nil {
			return true, ""
		}
		return false, "Invalid ISO date format (YYYY-MM-DD)"
	}

	validator.customValidators["datetime_iso"] = func(value interface{}, rule JSONValidationRule) (bool, string) {
		str := fmt.Sprintf("%v", value)
		if _, err := time.Parse(time.RFC3339, str); err == nil {
			return true, ""
		}
		return false, "Invalid ISO datetime format (RFC3339)"
	}
}

// initializeTemplateFunctions sets up template functions
func (engine *JSONTemplateEngine) initializeTemplateFunctions() {
	// Copy transformation functions to template functions
	engine.functions["uppercase"] = func(value interface{}, params map[string]interface{}) (interface{}, error) {
		return strings.ToUpper(fmt.Sprintf("%v", value)), nil
	}

	engine.functions["lowercase"] = func(value interface{}, params map[string]interface{}) (interface{}, error) {
		return strings.ToLower(fmt.Sprintf("%v", value)), nil
	}

	engine.functions["capitalize"] = func(value interface{}, params map[string]interface{}) (interface{}, error) {
		str := fmt.Sprintf("%v", value)
		if len(str) == 0 {
			return str, nil
		}
		return strings.ToUpper(str[:1]) + strings.ToLower(str[1:]), nil
	}

	engine.functions["default"] = func(value interface{}, params map[string]interface{}) (interface{}, error) {
		if value == nil || value == "" {
			if defaultValue, exists := params["defaultValue"]; exists {
				return defaultValue, nil
			}
			return "Unknown", nil
		}
		return value, nil
	}

	engine.functions["uuid"] = func(value interface{}, params map[string]interface{}) (interface{}, error) {
		return generateUUID(), nil
	}

	engine.functions["now"] = func(value interface{}, params map[string]interface{}) (interface{}, error) {
		format := time.RFC3339
		if formatParam, exists := params["format"]; exists {
			format = fmt.Sprintf("%v", formatParam)
		}
		return time.Now().Format(format), nil
	}
}

// =====================================
// UTILITY METHODS
// =====================================

// detectTargetFormat detects target format from source data or metadata
func (s *JSONTransformationService) detectTargetFormat(sourceData map[string]interface{}, message *UniversalMessage) string {
	// Check for explicit target format in metadata
	if targetFormat, exists := message.CustomFields["targetFormat"]; exists {
		return fmt.Sprintf("%v", targetFormat)
	}

	// Check for FHIR indicators
	if resourceType, exists := sourceData["resourceType"]; exists {
		if resourceTypeStr, ok := resourceType.(string); ok && isFHIRResourceType(resourceTypeStr) {
			return string(MessageTypeFHIR)
		}
	}

	// Check for HL7 indicators
	if messageHeader, exists := sourceData["messageHeader"]; exists {
		if _, ok := messageHeader.(map[string]interface{}); ok {
			return string(MessageTypeHL7)
		}
	}

	// Check for XML indicators
	if xmlData, exists := sourceData["xml"]; exists {
		if _, ok := xmlData.(map[string]interface{}); ok {
			return string(MessageTypeXML)
		}
	}

	// Default to JSON
	return string(MessageTypeJSON)
}

// extractValueByPath extracts value from data using JSONPath-like syntax
func (s *JSONTransformationService) extractValueByPath(data map[string]interface{}, path string) (interface{}, bool) {
	// Handle root path
	if path == "$" || path == "" {
		return data, true
	}

	// Remove $ prefix if present
	if strings.HasPrefix(path, "$.") {
		path = path[2:]
	} else if strings.HasPrefix(path, "$") {
		path = path[1:]
	}

	// Split path by dots
	parts := strings.Split(path, ".")
	current := data

	for i, part := range parts {
		// Handle array access like [0]
		if strings.Contains(part, "[") && strings.Contains(part, "]") {
			arrayName := part[:strings.Index(part, "[")]
			indexStr := part[strings.Index(part, "[")+1 : strings.Index(part, "]")]

			// Get array
			if arrayName != "" {
				if arrayValue, exists := current[arrayName]; exists {
					if array, ok := arrayValue.([]interface{}); ok {
						current = map[string]interface{}{"array": array}
					} else {
						return nil, false
					}
				} else {
					return nil, false
				}
			}

			// Access array element
			if array, exists := current["array"]; exists {
				if arraySlice, ok := array.([]interface{}); ok {
					if index, err := strconv.Atoi(indexStr); err == nil && index < len(arraySlice) {
						if i == len(parts)-1 {
							return arraySlice[index], true
						}
						if nextMap, ok := arraySlice[index].(map[string]interface{}); ok {
							current = nextMap
						} else {
							return nil, false
						}
					} else {
						return nil, false
					}
				} else {
					return nil, false
				}
			} else {
				return nil, false
			}
		} else {
			// Regular field access
			if i == len(parts)-1 {
				value, exists := current[part]
				return value, exists
			}

			if next, exists := current[part]; exists {
				if nextMap, ok := next.(map[string]interface{}); ok {
					current = nextMap
				} else {
					return nil, false
				}
			} else {
				return nil, false
			}
		}
	}

	return nil, false
}

// setValueByPath sets value in data using JSONPath-like syntax
func (s *JSONTransformationService) setValueByPath(data map[string]interface{}, path string, value interface{}) error {
	// Handle root path
	if path == "$" || path == "" {
		return fmt.Errorf("cannot set root value")
	}

	// Remove $ prefix if present
	if strings.HasPrefix(path, "$.") {
		path = path[2:]
	} else if strings.HasPrefix(path, "$") {
		path = path[1:]
	}

	// Split path by dots
	parts := strings.Split(path, ".")
	current := data

	for i, part := range parts {
		if i == len(parts)-1 {
			// Set the final value
			current[part] = value
			return nil
		}

		// Create intermediate objects if they don't exist
		if _, exists := current[part]; !exists {
			current[part] = make(map[string]interface{})
		}

		if nextMap, ok := current[part].(map[string]interface{}); ok {
			current = nextMap
		} else {
			return fmt.Errorf("cannot navigate path %s: intermediate value is not an object", path)
		}
	}

	return nil
}

// countValidationErrors counts validation errors from results
func (s *JSONTransformationService) countValidationErrors(results []JSONValidationResult) int {
	count := 0
	for _, result := range results {
		if !result.Passed && result.Severity == "error" {
			count++
		}
	}
	return count
}

// calculateQualityScore calculates overall quality score
func (s *JSONTransformationService) calculateQualityScore(validationResults []JSONValidationResult, mappingStats JSONMappingStatistics) float64 {
	// Start with mapping success rate
	mappingScore := mappingStats.MappingSuccessRate

	// Factor in validation results
	validationScore := 100.0
	if len(validationResults) > 0 {
		errorCount := 0
		warningCount := 0

		for _, result := range validationResults {
			if !result.Passed {
				if result.Severity == "error" {
					errorCount++
				} else if result.Severity == "warning" {
					warningCount++
				}
			}
		}

		// Calculate validation score (errors are weighted more heavily than warnings)
		totalIssues := float64(errorCount*2 + warningCount)
		totalChecks := float64(len(validationResults))

		if totalChecks > 0 {
			validationScore = ((totalChecks - totalIssues) / totalChecks) * 100.0
			if validationScore < 0 {
				validationScore = 0
			}
		}
	}

	// Weighted average (mapping 70%, validation 30%)
	return (mappingScore*0.7 + validationScore*0.3)
}

// sortRulesByPriority sorts mapping rules by priority
func (s *JSONTransformationService) sortRulesByPriority(rules []JSONMappingRule) []JSONMappingRule {
	// Simple bubble sort by priority
	sorted := make([]JSONMappingRule, len(rules))
	copy(sorted, rules)

	for i := 0; i < len(sorted)-1; i++ {
		for j := i + 1; j < len(sorted); j++ {
			if sorted[i].Priority > sorted[j].Priority {
				sorted[i], sorted[j] = sorted[j], sorted[i]
			}
		}
	}

	return sorted
}

// evaluateCondition evaluates a condition string
func (s *JSONTransformationService) evaluateCondition(condition string, data map[string]interface{}) (bool, error) {
	// Simple condition evaluation
	// Format: "field operator value" e.g., "$.type eq 'Patient'"

	parts := strings.Fields(condition)
	if len(parts) != 3 {
		return false, fmt.Errorf("invalid condition format: %s", condition)
	}

	field := parts[0]
	operator := parts[1]
	expectedValue := strings.Trim(parts[2], "'\"")

	// Extract field value
	actualValue, found := s.extractValueByPath(data, field)
	if !found {
		return false, nil
	}

	// Apply operator
	if opFunc, exists := s.mappingEngine.conditionEvaluator.operators[operator]; exists {
		return opFunc(actualValue, expectedValue), nil
	}

	return false, fmt.Errorf("unknown operator: %s", operator)
}

// applyTransformation applies a transformation function
func (s *JSONTransformationService) applyTransformation(value interface{}, transform, dataType string) (interface{}, error) {
	if transform == "" {
		return value, nil
	}

	if transformFunc, exists := s.mappingEngine.transformFunctions[transform]; exists {
		return transformFunc(value, map[string]interface{}{})
	}

	return value, fmt.Errorf("unknown transformation: %s", transform)
}

// convertDataType converts value to specified data type
func (s *JSONTransformationService) convertDataType(value interface{}, dataType string) (interface{}, error) {
	switch dataType {
	case "string":
		return fmt.Sprintf("%v", value), nil
	case "number", "integer":
		if num, ok := convertToFloat64(value); ok {
			if dataType == "integer" {
				return int(num), nil
			}
			return num, nil
		}
		return nil, fmt.Errorf("cannot convert %v to number", value)
	case "boolean":
		if boolVal, ok := convertToBoolean(value); ok {
			return boolVal, nil
		}
		return nil, fmt.Errorf("cannot convert %v to boolean", value)
	case "array":
		if array, ok := value.([]interface{}); ok {
			return array, nil
		}
		// Convert single value to array
		return []interface{}{value}, nil
	case "object":
		if obj, ok := value.(map[string]interface{}); ok {
			return obj, nil
		}
		return nil, fmt.Errorf("cannot convert %v to object", value)
	default:
		return value, nil
	}
}

// Helper methods for data manipulation and validation
func (s *JSONTransformationService) requiresDataTypeConversion(value interface{}, targetType string) bool {
	switch targetType {
	case "string":
		_, ok := value.(string)
		return !ok
	case "number":
		_, ok := value.(float64)
		if !ok {
			_, ok = value.(int)
		}
		return !ok
	case "boolean":
		_, ok := value.(bool)
		return !ok
	case "array":
		_, ok := value.([]interface{})
		return !ok
	case "object":
		_, ok := value.(map[string]interface{})
		return !ok
	default:
		return false
	}
}

func (s *JSONTransformationService) deepCopyMap(source, target map[string]interface{}) error {
	for key, value := range source {
		switch v := value.(type) {
		case map[string]interface{}:
			target[key] = make(map[string]interface{})
			if err := s.deepCopyMap(v, target[key].(map[string]interface{})); err != nil {
				return err
			}
		case []interface{}:
			target[key] = make([]interface{}, len(v))
			copy(target[key].([]interface{}), v)
		default:
			target[key] = v
		}
	}
	return nil
}

func (s *JSONTransformationService) flattenArraysInMap(data map[string]interface{}) {
	for key, value := range data {
		if array, ok := value.([]interface{}); ok && len(array) == 1 {
			data[key] = array[0]
		} else if subMap, ok := value.(map[string]interface{}); ok {
			s.flattenArraysInMap(subMap)
		}
	}
}

func (s *JSONTransformationService) countMappableFields(data map[string]interface{}) int {
	count := 0
	for _, value := range data {
		count++
		if subMap, ok := value.(map[string]interface{}); ok {
			count += s.countMappableFields(subMap)
		}
	}
	return count
}

func (s *JSONTransformationService) normalizeFieldName(name string) string {
	// Convert to camelCase
	words := strings.Fields(strings.ReplaceAll(strings.ReplaceAll(name, "_", " "), "-", " "))
	if len(words) == 0 {
		return name
	}

	result := strings.ToLower(words[0])
	for i := 1; i < len(words); i++ {
		if len(words[i]) > 0 {
			result += strings.ToUpper(words[i][:1]) + strings.ToLower(words[i][1:])
		}
	}
	return result
}

func (s *JSONTransformationService) normalizeFieldValue(value interface{}) (interface{}, error) {
	switch v := value.(type) {
	case string:
		trimmed := strings.TrimSpace(v)
		// Try to convert string numbers
		if num, err := strconv.ParseFloat(trimmed, 64); err == nil && !strings.Contains(trimmed, ".") {
			return int(num), nil
		} else if num, err := strconv.ParseFloat(trimmed, 64); err == nil {
			return num, nil
		}
		// Try to convert string booleans
		if strings.ToLower(trimmed) == "true" {
			return true, nil
		} else if strings.ToLower(trimmed) == "false" {
			return false, nil
		}
		return trimmed, nil
	case []interface{}:
		normalized := make([]interface{}, len(v))
		for i, item := range v {
			if normalizedItem, err := s.normalizeFieldValue(item); err == nil {
				normalized[i] = normalizedItem
			} else {
				normalized[i] = item
			}
		}
		return normalized, nil
	case map[string]interface{}:
		normalized := make(map[string]interface{})
		for key, val := range v {
			normalizedKey := s.normalizeFieldName(key)
			if normalizedVal, err := s.normalizeFieldValue(val); err == nil {
				normalized[normalizedKey] = normalizedVal
			} else {
				normalized[normalizedKey] = val
			}
		}
		return normalized, nil
	default:
		return value, nil
	}
}

// Template processing utility methods
func (s *JSONTransformationService) extractTemplateFunction(expression string) string {
	// Extract function from template expression like {{.field | function}}
	if strings.Contains(expression, "|") {
		parts := strings.Split(expression, "|")
		if len(parts) > 1 {
			return strings.TrimSpace(strings.Trim(parts[1], " }}"))
		}
	}
	return ""
}

func (s *JSONTransformationService) extractSourcePath(templateValue interface{}) string {
	if str, ok := templateValue.(string); ok {
		if strings.HasPrefix(str, "{{.") && strings.HasSuffix(str, "}}") {
			// Extract path from {{.field}} or {{.field | function}}
			content := str[3 : len(str)-2] // Remove {{. and }}
			if strings.Contains(content, "|") {
				parts := strings.Split(content, "|")
				return "$." + strings.TrimSpace(parts[0])
			}
			return "$." + strings.TrimSpace(content)
		}
	}
	return ""
}

func (s *JSONTransformationService) evaluateTemplateExpression(expression string, data map[string]interface{}) (interface{}, error) {
	// Simple template expression evaluation
	// Format: {{.field | function}}

	if !strings.HasPrefix(expression, "{{") || !strings.HasSuffix(expression, "}}") {
		return expression, nil
	}

	content := expression[2 : len(expression)-2] // Remove {{ and }}
	content = strings.TrimSpace(content)

	if strings.HasPrefix(content, ".") {
		content = content[1:] // Remove leading dot
	}

	var fieldPath string
	var functionName string

	if strings.Contains(content, "|") {
		parts := strings.Split(content, "|")
		fieldPath = strings.TrimSpace(parts[0])
		functionName = strings.TrimSpace(parts[1])
	} else {
		fieldPath = content
	}

	// Extract value
	value, found := s.extractValueByPath(data, "$." + fieldPath)
	if !found {
		return nil, fmt.Errorf("field not found: %s", fieldPath)
	}

	// Apply function if specified
	if functionName != "" {
		if templateFunc, exists := s.templateEngine.functions[functionName]; exists {
			return templateFunc(value, map[string]interface{}{})
		}
		return nil, fmt.Errorf("unknown template function: %s", functionName)
	}

	return value, nil
}

// Inference and detection utility methods
func (s *JSONTransformationService) inferFHIRResourceType(data map[string]interface{}) string {
	// Check for explicit resourceType
	if resourceType, exists := data["resourceType"]; exists {
		return fmt.Sprintf("%v", resourceType)
	}

	// Infer from common patterns
	if _, exists := data["name"]; exists {
		if _, existsGender := data["gender"]; existsGender {
			return "Patient"
		}
		return "Organization"
	}

	if _, exists := data["status"]; exists {
		if _, existsCode := data["code"]; existsCode {
			return "Observation"
		}
		return "Encounter"
	}

	if _, exists := data["messageType"]; exists {
		return "MessageHeader"
	}

	return "Resource"
}

func (s *JSONTransformationService) generateFHIRID(data map[string]interface{}) string {
	// Try to use existing ID
	if id, exists := data["id"]; exists {
		return sanitizeFHIRID(fmt.Sprintf("%v", id))
	}

	// Generate from other fields
	if name, exists := data["name"]; exists {
		return sanitizeFHIRID(fmt.Sprintf("%v", name))
	}

	// Generate UUID
	return generateUUID()
}

func (s *JSONTransformationService) extractStringValue(data map[string]interface{}, keys []string, defaultValue string) string {
	for _, key := range keys {
		if value, exists := data[key]; exists {
			return fmt.Sprintf("%v", value)
		}
	}
	return defaultValue
}

// Validation utility methods
func (s *JSONTransformationService) countNullValues(data map[string]interface{}) int {
	count := 0
	for _, value := range data {
		if value == nil {
			count++
		} else if subMap, ok := value.(map[string]interface{}); ok {
			count += s.countNullValues(subMap)
		}
	}
	return count
}

func (s *JSONTransformationService) calculateNestingDepth(data map[string]interface{}) int {
	maxDepth := 0
	for _, value := range data {
		depth := 1
		if subMap, ok := value.(map[string]interface{}); ok {
			depth += s.calculateNestingDepth(subMap)
		}
		if depth > maxDepth {
			maxDepth = depth
		}
	}
	return maxDepth
}

func (s *JSONTransformationService) hasFieldAtPath(data map[string]interface{}, path string) bool {
	_, found := s.extractValueByPath(data, path)
	return found
}

func (s *JSONTransformationService) isValidType(value interface{}, expectedType interface{}) bool {
	switch expected := expectedType.(type) {
	case string:
		return s.checkSingleType(value, expected)
	case []interface{}:
		for _, t := range expected {
			if typeStr, ok := t.(string); ok && s.checkSingleType(value, typeStr) {
				return true
			}
		}
		return false
	default:
		return true
	}
}

func (s *JSONTransformationService) checkSingleType(value interface{}, expectedType string) bool {
	switch expectedType {
	case "string":
		_, ok := value.(string)
		return ok
	case "number":
		_, ok1 := value.(float64)
		_, ok2 := value.(int)
		return ok1 || ok2
	case "integer":
		_, ok := value.(int)
		return ok
	case "boolean":
		_, ok := value.(bool)
		return ok
	case "array":
		_, ok := value.([]interface{})
		return ok
	case "object":
		_, ok := value.(map[string]interface{})
		return ok
	case "null":
		return value == nil
	default:
		return true
	}
}

func (s *JSONTransformationService) convertToFloat64(value interface{}) (float64, bool) {
	return convertToFloat64(value)
}

func (s *JSONTransformationService) isInEnum(value interface{}, enum []interface{}) bool {
	valueStr := fmt.Sprintf("%v", value)
	for _, enumValue := range enum {
		if fmt.Sprintf("%v", enumValue) == valueStr {
			return true
		}
	}
	return false
}

func (s *JSONTransformationService) hasConsistentTypes(array []interface{}) bool {
	if len(array) <= 1 {
		return true
	}

	firstType := reflect.TypeOf(array[0])
	for i := 1; i < len(array); i++ {
		if reflect.TypeOf(array[i]) != firstType {
			return false
		}
	}
	return true
}

func (s *JSONTransformationService) looksLikeNumber(str string) bool {
	_, err := strconv.ParseFloat(str, 64)
	return err == nil
}

func (s *JSONTransformationService) looksLikeBoolean(str string) bool {
	lower := strings.ToLower(str)
	return lower == "true" || lower == "false" || lower == "yes" || lower == "no" || lower == "1" || lower == "0"
}

// Performance and utility helper methods
func (s *JSONTransformationService) startTransformationStep(stepName, inputType, outputType string) TransformationStep {
	return TransformationStep{
		StepID:     fmt.Sprintf("step_%d", time.Now().UnixNano()),
		StepName:   stepName,
		InputType:  inputType,
		OutputType: outputType,
		StartTime:  time.Now(),
		Success:    false,
		Metadata:   make(map[string]interface{}),
	}
}

func (s *JSONTransformationService) completeTransformationStep(step *TransformationStep, err error, inputSize, outputSize int) {
	step.EndTime = time.Now()
	step.Duration = step.EndTime.Sub(step.StartTime)
	step.Success = err == nil
	step.InputSize = int64(inputSize)
	step.OutputSize = int64(outputSize)

	if err != nil {
		step.ErrorMessage = err.Error()
	}
}

func (s *JSONTransformationService) updatePerformanceMetrics(processingMetrics ProcessingMetrics) {
	s.performanceMetrics.MessagesProcessed++

	// Update running averages
	totalMessages := float64(s.performanceMetrics.MessagesProcessed)
	s.performanceMetrics.AverageTransformTime = time.Duration(
		(float64(s.performanceMetrics.AverageTransformTime)*(totalMessages-1) +
		 float64(processingMetrics.TransformTime)) / totalMessages)
}

// Global utility functions
func convertToFloat64(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	case string:
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

func convertToBoolean(value interface{}) (bool, bool) {
	switch v := value.(type) {
	case bool:
		return v, true
	case string:
		lower := strings.ToLower(v)
		if lower == "true" || lower == "yes" || lower == "1" {
			return true, true
		} else if lower == "false" || lower == "no" || lower == "0" {
			return false, true
		}
	case int:
		return v != 0, true
	case float64:
		return v != 0, true
	}
	return false, false
}

func isFHIRResourceType(resourceType string) bool {
	fhirResourceTypes := []string{
		"Patient", "Organization", "Practitioner", "Encounter", "Observation",
		"DiagnosticReport", "MessageHeader", "Bundle", "Composition",
		"Condition", "Procedure", "Medication", "MedicationRequest",
		"AllergyIntolerance", "Immunization", "CarePlan", "Goal",
	}

	for _, validType := range fhirResourceTypes {
		if resourceType == validType {
			return true
		}
	}
	return false
}

func sanitizeFHIRID(id string) string {
	// Remove invalid characters for FHIR ID
	reg := regexp.MustCompile(`[^a-zA-Z0-9\-\.]`)
	sanitized := reg.ReplaceAllString(id, "")
	if len(sanitized) > 64 {
		sanitized = sanitized[:64]
	}
	if len(sanitized) == 0 {
		sanitized = generateUUID()
	}
	return sanitized
}

func generateUUID() string {
	// Simple UUID generation for demonstration
	return fmt.Sprintf("id-%d", time.Now().UnixNano())
}

// Public API methods
func (s *JSONTransformationService) GetPerformanceMetrics() JSONPerformanceMetrics {
	return s.performanceMetrics
}

func (s *JSONTransformationService) ResetPerformanceMetrics() {
	s.performanceMetrics = JSONPerformanceMetrics{}
}

func (s *JSONTransformationService) GetSupportedTransformations() []string {
	transformations := make([]string, 0, len(s.mappingEngine.transformFunctions))
	for name := range s.mappingEngine.transformFunctions {
		transformations = append(transformations, name)
	}
	return transformations
}

func (s *JSONTransformationService) AddMappingRule(ruleSetName string, rule JSONMappingRule) {
	if _, exists := s.mappingEngine.mappingRules[ruleSetName]; !exists {
		s.mappingEngine.mappingRules[ruleSetName] = []JSONMappingRule{}
	}
	s.mappingEngine.mappingRules[ruleSetName] = append(s.mappingEngine.mappingRules[ruleSetName], rule)
}

func (s *JSONTransformationService) RemoveMappingRule(ruleSetName, ruleID string) {
	if rules, exists := s.mappingEngine.mappingRules[ruleSetName]; exists {
		for i, rule := range rules {
			if rule.ID == ruleID {
				s.mappingEngine.mappingRules[ruleSetName] = append(rules[:i], rules[i+1:]...)
				break
			}
		}
	}
}

func (s *JSONTransformationService) GetAvailableSchemas() []string {
	schemas := make([]string, 0, len(s.schemaRegistry.schemas))
	for id := range s.schemaRegistry.schemas {
		schemas = append(schemas, id)
	}
	return schemas
}

func (s *JSONTransformationService) ClearCache() {
	s.schemaRegistry.schemaCache = make(map[string]*CompiledSchema)
	s.templateEngine.templateCache = make(map[string]*CompiledTemplate)
}