package response

import (
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"

	"ezhealthkonnect/models"
)

// ResponseExtractorService handles extraction and transformation of API response data
type ResponseExtractorService struct{}

// NewResponseExtractorService creates a new response extractor service
func NewResponseExtractorService() *ResponseExtractorService {
	return &ResponseExtractorService{}
}

// ApplyMappingRules applies a set of mapping rules to an API response
func (s *ResponseExtractorService) ApplyMappingRules(apiResponse interface{}, rules []models.ResponseMappingRule) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	for _, rule := range rules {
		value, err := s.ExtractField(apiResponse, rule)
		if err != nil {
			if rule.Required {
				return nil, fmt.Errorf("required field %s: %w", rule.TargetField, err)
			}
			log.Printf("⚠️  Failed to extract field %s: %v", rule.TargetField, err)

			// Use default value if available
			if rule.DefaultValue != nil {
				result[rule.TargetField] = rule.DefaultValue
				log.Printf("📝 Using default value for %s", rule.TargetField)
			}
			continue
		}

		if value != nil {
			result[rule.TargetField] = value
		} else if rule.DefaultValue != nil {
			result[rule.TargetField] = rule.DefaultValue
		}
	}

	log.Printf("✅ Applied %d mapping rules, extracted %d fields", len(rules), len(result))
	return result, nil
}

// ExtractField extracts a single field from API response based on a rule
func (s *ResponseExtractorService) ExtractField(apiResponse interface{}, rule models.ResponseMappingRule) (interface{}, error) {
	// Step 1: Extract value using JSONPath
	var rawValue interface{}
	var err error

	if rule.SourcePath != "" {
		rawValue, err = s.extractByJSONPath(apiResponse, rule.SourcePath)
		if err != nil {
			return nil, fmt.Errorf("JSONPath extraction failed: %w", err)
		}
	}

	// Step 2: Apply transformation if configured
	switch rule.TransformType {
	case models.TransformNone, "":
		return rawValue, nil

	case models.TransformCombine:
		return s.transformCombine(apiResponse, rule.TransformConfig)

	case models.TransformFilter:
		return s.transformFilter(rawValue, rule.TransformConfig)

	case models.TransformFormat:
		return s.transformFormat(rawValue, rule.TransformConfig)

	case models.TransformConditional:
		return s.transformConditional(apiResponse, rule.TransformConfig)

	default:
		log.Printf("⚠️  Unknown transform type: %s, returning raw value", rule.TransformType)
		return rawValue, nil
	}
}

// extractByJSONPath extracts a value using JSONPath expression
// Custom implementation to avoid dependency on broken external library
func (s *ResponseExtractorService) extractByJSONPath(data interface{}, path string) (interface{}, error) {
	// Convert data to JSON-compatible format
	var jsonData interface{}

	// If data is already a map or slice, use it directly
	switch v := data.(type) {
	case map[string]interface{}, []interface{}:
		jsonData = v
	default:
		// Marshal and unmarshal to ensure JSON compatibility
		jsonBytes, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal data: %w", err)
		}
		if err := json.Unmarshal(jsonBytes, &jsonData); err != nil {
			return nil, fmt.Errorf("failed to unmarshal data: %w", err)
		}
	}

	// Apply custom JSONPath resolver
	result, err := s.resolveJSONPath(jsonData, path)
	if err != nil {
		return nil, fmt.Errorf("JSONPath lookup failed for %s: %w", path, err)
	}

	return result, nil
}

// resolveJSONPath is a custom JSONPath implementation supporting common patterns:
// - $.field or field - direct field access
// - $.field.nested - nested field access
// - $.array[0] - array index access
// - $.array[*] - all array elements
// - $.field[0].nested - combined patterns
func (s *ResponseExtractorService) resolveJSONPath(data interface{}, path string) (interface{}, error) {
	// Remove leading $. or $ if present
	path = strings.TrimPrefix(path, "$.")
	path = strings.TrimPrefix(path, "$")

	if path == "" {
		return data, nil
	}

	// Parse path into segments
	segments := s.parseJSONPathSegments(path)

	current := data
	for _, segment := range segments {
		var err error
		current, err = s.resolvePathSegment(current, segment)
		if err != nil {
			return nil, err
		}
		if current == nil {
			return nil, nil
		}
	}

	return current, nil
}

// jsonPathSegment represents a single segment in a JSONPath
type jsonPathSegment struct {
	field    string
	isArray  bool
	index    int
	isWild   bool // [*] selector
}

// parseJSONPathSegments parses a JSONPath into segments
// Examples:
//   "field.nested" -> [{field: "field"}, {field: "nested"}]
//   "array[0].field" -> [{field: "array", isArray: true, index: 0}, {field: "field"}]
//   "items[*].name" -> [{field: "items", isArray: true, isWild: true}, {field: "name"}]
func (s *ResponseExtractorService) parseJSONPathSegments(path string) []jsonPathSegment {
	var segments []jsonPathSegment

	// Regex to match field names with optional array index
	// Matches: fieldName, fieldName[0], fieldName[*]
	re := regexp.MustCompile(`([^.\[\]]+)(?:\[(\d+|\*)\])?`)

	matches := re.FindAllStringSubmatch(path, -1)
	for _, match := range matches {
		if len(match) < 2 || match[1] == "" {
			continue
		}

		seg := jsonPathSegment{field: match[1]}

		if len(match) >= 3 && match[2] != "" {
			seg.isArray = true
			if match[2] == "*" {
				seg.isWild = true
			} else {
				idx, _ := strconv.Atoi(match[2])
				seg.index = idx
			}
		}

		segments = append(segments, seg)
	}

	return segments
}

// resolvePathSegment resolves a single path segment against data
func (s *ResponseExtractorService) resolvePathSegment(data interface{}, seg jsonPathSegment) (interface{}, error) {
	// First, get the field value
	var fieldValue interface{}

	switch v := data.(type) {
	case map[string]interface{}:
		var exists bool
		fieldValue, exists = v[seg.field]
		if !exists {
			return nil, fmt.Errorf("field '%s' not found", seg.field)
		}
	case []interface{}:
		// If data is already an array and we have a field name,
		// extract that field from each array element (for wildcard results)
		if seg.field != "" {
			results := []interface{}{}
			for _, item := range v {
				if itemMap, ok := item.(map[string]interface{}); ok {
					if val, exists := itemMap[seg.field]; exists {
						results = append(results, val)
					}
				}
			}
			if len(results) == 0 {
				return nil, fmt.Errorf("field '%s' not found in array elements", seg.field)
			}
			if !seg.isArray {
				return results, nil
			}
			fieldValue = results
		} else {
			fieldValue = v
		}
	default:
		return nil, fmt.Errorf("cannot navigate into type %T", data)
	}

	// If no array access needed, return the field value
	if !seg.isArray {
		return fieldValue, nil
	}

	// Array access required
	arr, ok := fieldValue.([]interface{})
	if !ok {
		return nil, fmt.Errorf("field '%s' is not an array", seg.field)
	}

	// Wildcard access - return all elements
	if seg.isWild {
		return arr, nil
	}

	// Specific index access
	if seg.index < 0 || seg.index >= len(arr) {
		return nil, fmt.Errorf("array index %d out of bounds (length %d)", seg.index, len(arr))
	}

	return arr[seg.index], nil
}

// transformCombine combines multiple fields into a single value
func (s *ResponseExtractorService) transformCombine(data interface{}, configRaw map[string]interface{}) (interface{}, error) {
	// Parse config
	configJSON, _ := json.Marshal(configRaw)
	var config models.CombineTransformConfig
	if err := json.Unmarshal(configJSON, &config); err != nil {
		return nil, fmt.Errorf("invalid combine config: %w", err)
	}

	// Extract all values
	values := []string{}
	for _, path := range config.AdditionalPaths {
		value, err := s.extractByJSONPath(data, path)
		if err != nil {
			log.Printf("⚠️  Failed to extract path %s: %v", path, err)
			continue
		}
		values = append(values, fmt.Sprintf("%v", value))
	}

	if len(values) == 0 {
		return nil, fmt.Errorf("no values extracted for combine")
	}

	// Apply format if specified
	if config.Format != "" {
		// Simple format string replacement {0}, {1}, etc.
		result := config.Format
		for i, value := range values {
			placeholder := fmt.Sprintf("{%d}", i)
			result = strings.ReplaceAll(result, placeholder, value)
		}
		return result, nil
	}

	// Default: join with separator
	return strings.Join(values, config.Separator), nil
}

// transformFilter filters an array and extracts a specific field
func (s *ResponseExtractorService) transformFilter(data interface{}, configRaw map[string]interface{}) (interface{}, error) {
	// Parse config
	configJSON, _ := json.Marshal(configRaw)
	var config models.FilterTransformConfig
	if err := json.Unmarshal(configJSON, &config); err != nil {
		return nil, fmt.Errorf("invalid filter config: %w", err)
	}

	// Ensure data is an array
	array, ok := data.([]interface{})
	if !ok {
		return nil, fmt.Errorf("filter transform requires an array, got %T", data)
	}

	// Filter array
	for _, item := range array {
		itemMap, ok := item.(map[string]interface{})
		if !ok {
			continue
		}

		// Check if item matches filter
		if s.matchesFilter(itemMap, config.FilterField, config.FilterOperator, config.FilterValue) {
			// Extract the target field
			if config.ExtractField != "" {
				if value, exists := itemMap[config.ExtractField]; exists {
					return value, nil
				}
			} else {
				return item, nil
			}
		}
	}

	return nil, fmt.Errorf("no matching item found in array")
}

// matchesFilter checks if a map matches the filter criteria
func (s *ResponseExtractorService) matchesFilter(item map[string]interface{}, field, operator string, expectedValue interface{}) bool {
	actualValue, exists := item[field]
	if !exists {
		return false
	}

	actualStr := fmt.Sprintf("%v", actualValue)
	expectedStr := fmt.Sprintf("%v", expectedValue)

	switch operator {
	case "equals":
		return actualStr == expectedStr
	case "contains":
		return strings.Contains(actualStr, expectedStr)
	case "startsWith":
		return strings.HasPrefix(actualStr, expectedStr)
	case "endsWith":
		return strings.HasSuffix(actualStr, expectedStr)
	case "gt":
		return s.compareNumbers(actualValue, expectedValue) > 0
	case "lt":
		return s.compareNumbers(actualValue, expectedValue) < 0
	case "gte":
		return s.compareNumbers(actualValue, expectedValue) >= 0
	case "lte":
		return s.compareNumbers(actualValue, expectedValue) <= 0
	default:
		log.Printf("⚠️  Unknown filter operator: %s", operator)
		return false
	}
}

// compareNumbers compares two values as numbers
func (s *ResponseExtractorService) compareNumbers(a, b interface{}) int {
	aFloat := s.toFloat64(a)
	bFloat := s.toFloat64(b)
	if aFloat < bFloat {
		return -1
	} else if aFloat > bFloat {
		return 1
	}
	return 0
}

// toFloat64 converts a value to float64
func (s *ResponseExtractorService) toFloat64(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case string:
		f, _ := strconv.ParseFloat(val, 64)
		return f
	default:
		return 0
	}
}

// transformFormat formats a value (currently supports date formatting)
func (s *ResponseExtractorService) transformFormat(data interface{}, configRaw map[string]interface{}) (interface{}, error) {
	// Parse config
	configJSON, _ := json.Marshal(configRaw)
	var config models.FormatTransformConfig
	if err := json.Unmarshal(configJSON, &config); err != nil {
		return nil, fmt.Errorf("invalid format config: %w", err)
	}

	dataStr := fmt.Sprintf("%v", data)

	// Date formatting
	if config.FormatType == "date" || (config.InputFormat != "" && config.OutputFormat != "") {
		return s.formatDate(dataStr, config.InputFormat, config.OutputFormat)
	}

	// Default: return as-is
	return data, nil
}

// formatDate converts date from input format to output format
func (s *ResponseExtractorService) formatDate(dateStr, inputFormat, outputFormat string) (string, error) {
	// Convert Go format strings from common patterns
	inputLayout := s.convertDateFormat(inputFormat)
	outputLayout := s.convertDateFormat(outputFormat)

	// Parse input date
	parsedDate, err := time.Parse(inputLayout, dateStr)
	if err != nil {
		return "", fmt.Errorf("failed to parse date %s with format %s: %w", dateStr, inputFormat, err)
	}

	// Format to output
	return parsedDate.Format(outputLayout), nil
}

// convertDateFormat converts common date format patterns to Go time layout
func (s *ResponseExtractorService) convertDateFormat(format string) string {
	replacements := map[string]string{
		"YYYY": "2006",
		"YY":   "06",
		"MM":   "01",
		"DD":   "02",
		"HH":   "15",
		"mm":   "04",
		"ss":   "05",
	}

	layout := format
	for pattern, goPattern := range replacements {
		layout = strings.ReplaceAll(layout, pattern, goPattern)
	}

	return layout
}

// transformConditional applies conditional logic to extract values
func (s *ResponseExtractorService) transformConditional(data interface{}, configRaw map[string]interface{}) (interface{}, error) {
	// Parse config
	configJSON, _ := json.Marshal(configRaw)
	var config models.ConditionalTransformConfig
	if err := json.Unmarshal(configJSON, &config); err != nil {
		return nil, fmt.Errorf("invalid conditional config: %w", err)
	}

	// Evaluate conditions in order
	for _, condition := range config.Conditions {
		// Check if condition matches
		conditionValue, err := s.extractByJSONPath(data, condition.If.Field)
		if err != nil {
			log.Printf("⚠️  Failed to extract condition field %s: %v", condition.If.Field, err)
			continue
		}

		if s.evaluateCondition(conditionValue, condition.If.Operator, condition.If.Value) {
			// Condition matched - return the "then" value
			if condition.Then.Field != "" {
				return s.extractByJSONPath(data, condition.Then.Field)
			}
			return condition.Then.Value, nil
		}
	}

	// No conditions matched - return default
	return config.Default, nil
}

// evaluateCondition evaluates a single condition
func (s *ResponseExtractorService) evaluateCondition(actual interface{}, operator string, expected interface{}) bool {
	actualStr := fmt.Sprintf("%v", actual)
	expectedStr := fmt.Sprintf("%v", expected)

	switch operator {
	case "equals":
		return actualStr == expectedStr
	case "contains":
		return strings.Contains(actualStr, expectedStr)
	case "gt":
		return s.compareNumbers(actual, expected) > 0
	case "lt":
		return s.compareNumbers(actual, expected) < 0
	case "gte":
		return s.compareNumbers(actual, expected) >= 0
	case "lte":
		return s.compareNumbers(actual, expected) <= 0
	default:
		log.Printf("⚠️  Unknown condition operator: %s", operator)
		return false
	}
}

// CountFields recursively counts the number of fields in a map
func (s *ResponseExtractorService) CountFields(data interface{}) int {
	count := 0

	switch v := data.(type) {
	case map[string]interface{}:
		for _, value := range v {
			count++
			if nestedMap, ok := value.(map[string]interface{}); ok {
				count += s.CountFields(nestedMap)
			}
		}
	case []interface{}:
		for _, item := range v {
			count += s.CountFields(item)
		}
	default:
		count = 1
	}

	return count
}

// ValidateMappingRule validates a mapping rule configuration
func (s *ResponseExtractorService) ValidateMappingRule(rule models.ResponseMappingRule) error {
	if rule.TargetField == "" {
		return fmt.Errorf("targetField is required")
	}

	switch rule.TransformType {
	case models.TransformNone, "":
		if rule.SourcePath == "" {
			return fmt.Errorf("sourcePath is required for transform type 'none'")
		}

	case models.TransformCombine:
		if rule.TransformConfig == nil {
			return fmt.Errorf("transformConfig is required for transform type 'combine'")
		}
		// Validate combine config
		configJSON, _ := json.Marshal(rule.TransformConfig)
		var config models.CombineTransformConfig
		if err := json.Unmarshal(configJSON, &config); err != nil {
			return fmt.Errorf("invalid combine config: %w", err)
		}
		if len(config.AdditionalPaths) == 0 {
			return fmt.Errorf("additionalPaths is required for combine transform")
		}

	case models.TransformFilter:
		if rule.SourcePath == "" {
			return fmt.Errorf("sourcePath is required for transform type 'filter'")
		}
		if rule.TransformConfig == nil {
			return fmt.Errorf("transformConfig is required for transform type 'filter'")
		}

	case models.TransformFormat:
		if rule.SourcePath == "" {
			return fmt.Errorf("sourcePath is required for transform type 'format'")
		}

	case models.TransformConditional:
		if rule.TransformConfig == nil {
			return fmt.Errorf("transformConfig is required for transform type 'conditional'")
		}
	}

	return nil
}
