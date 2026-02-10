package executors

import (
	"fmt"
	"log"
	"regexp"
	"strings"
)

// ===============================================================
// SHARED TRANSFORMATION UTILITIES
// ===============================================================
// These utilities provide common transformation functions that can be
// reused across different executors (FieldMapping, SwitchCase, IfThenElse, etc.)
// Following OOP principles - DRY (Don't Repeat Yourself)

// TransformValue applies a single transformation to a value
// Supported transformations:
// - trim: Remove leading/trailing whitespace
// - upper/uppercase: Convert to uppercase
// - lower/lowercase: Convert to lowercase
// - capitalize: First letter uppercase, rest lowercase
// - regex:pattern: Extract first regex match
// - substring:start:end: Extract substring
// - replace:old:new: Replace all occurrences
func TransformValue(value interface{}, transformation string) interface{} {
	if value == nil {
		return nil
	}

	valueStr := fmt.Sprintf("%v", value)
	transformation = strings.TrimSpace(transformation)

	if transformation == "" {
		return value
	}

	log.Printf("      🔄 TransformValue: input='%v', transformation='%s'", value, transformation)

	var result string

	// Simple transformations (exact match)
	switch transformation {
	case "trim":
		result = strings.TrimSpace(valueStr)
	case "upper", "uppercase":
		result = strings.ToUpper(valueStr)
	case "lower", "lowercase":
		result = strings.ToLower(valueStr)
	case "capitalize":
		if len(valueStr) > 0 {
			result = strings.ToUpper(string(valueStr[0])) + strings.ToLower(valueStr[1:])
		} else {
			result = valueStr
		}
	default:
		// Parameterized transformations
		if strings.HasPrefix(transformation, "regex:") {
			pattern := strings.TrimPrefix(transformation, "regex:")
			re, err := regexp.Compile(pattern)
			if err == nil {
				matches := re.FindStringSubmatch(valueStr)
				if len(matches) > 0 {
					result = matches[0]
				} else {
					result = valueStr
				}
			} else {
				log.Printf("      ⚠️ Invalid regex pattern: %s", pattern)
				result = valueStr
			}
		} else if strings.HasPrefix(transformation, "substring:") {
			params := strings.TrimPrefix(transformation, "substring:")
			parts := strings.Split(params, ":")
			if len(parts) == 2 {
				var start, end int
				fmt.Sscanf(parts[0], "%d", &start)
				fmt.Sscanf(parts[1], "%d", &end)
				if start >= 0 && end <= len(valueStr) && start < end {
					result = valueStr[start:end]
				} else {
					result = valueStr
				}
			} else {
				result = valueStr
			}
		} else if strings.HasPrefix(transformation, "replace:") {
			params := strings.TrimPrefix(transformation, "replace:")
			parts := strings.Split(params, ":")
			if len(parts) >= 2 {
				result = strings.ReplaceAll(valueStr, parts[0], parts[1])
			} else {
				result = valueStr
			}
		} else {
			log.Printf("      ⚠️ Unknown transformation: %s", transformation)
			result = valueStr
		}
	}

	log.Printf("      🔄 TransformValue: result='%v'", result)
	return result
}

// ApplyTransformations applies multiple comma-separated transformations to a value
// Example: "trim,upper" will first trim whitespace, then convert to uppercase
func ApplyTransformations(value interface{}, transforms string) interface{} {
	if value == nil || transforms == "" {
		return value
	}

	result := value
	transformList := strings.Split(transforms, ",")

	for _, transform := range transformList {
		result = TransformValue(result, transform)
	}

	return result
}

// GetAvailableTransformations returns a list of available transformation types
// Useful for UI dropdowns and documentation
func GetAvailableTransformations() []map[string]string {
	return []map[string]string{
		{"value": "uppercase", "label": "UPPERCASE", "description": "Convert to uppercase"},
		{"value": "lowercase", "label": "lowercase", "description": "Convert to lowercase"},
		{"value": "trim", "label": "Trim whitespace", "description": "Remove leading/trailing whitespace"},
		{"value": "capitalize", "label": "Capitalize", "description": "First letter uppercase"},
		{"value": "regex:", "label": "Regex extract", "description": "Extract using regex pattern (e.g., regex:\\d+)"},
		{"value": "substring:", "label": "Substring", "description": "Extract substring (e.g., substring:0:5)"},
		{"value": "replace:", "label": "Replace", "description": "Replace text (e.g., replace:old:new)"},
	}
}
