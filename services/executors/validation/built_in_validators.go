package validation

import (
	"fmt"
	"regexp"
	"strings"
)

// ============================================================================
// REQUIRED VALIDATOR
// ============================================================================

type RequiredValidator struct {
	BaseValidator
}

func NewRequiredValidator() *RequiredValidator {
	return &RequiredValidator{
		BaseValidator: BaseValidator{
			validatorType: "required",
			description:   "Validates that a field is not empty",
		},
	}
}

func (v *RequiredValidator) Validate(value interface{}, options map[string]interface{}) (bool, string) {
	if value == nil {
		return false, "Field is required but was not found or is null"
	}

	// Convert to string and check if empty
	strValue := fmt.Sprintf("%v", value)
	strValue = strings.TrimSpace(strValue)

	if strValue == "" {
		return false, "Field is required but is empty"
	}

	return true, ""
}

// ============================================================================
// FORMAT VALIDATOR (with built-in presets)
// ============================================================================

type FormatValidator struct {
	BaseValidator
	presets map[string]string // Built-in format patterns
}

func NewFormatValidator() *FormatValidator {
	return &FormatValidator{
		BaseValidator: BaseValidator{
			validatorType: "format",
			description:   "Validates field format against regex patterns",
		},
		presets: map[string]string{
			"email":        `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`,
			"phone":        `^\d{10}$|^\d{3}-\d{3}-\d{4}$`,
			"ssn":          `^\d{3}-\d{2}-\d{4}$`,
			"date":         `^\d{8}$`,  // YYYYMMDD
			"hl7_date":     `^\d{8}$`,  // HL7 date format (YYYYMMDD)
			"datetime":     `^\d{14}$`, // YYYYMMDDHHMMSS
			"hl7_datetime": `^\d{14}$`, // HL7 datetime format (YYYYMMDDHHMMSS)
			"mrn":          `^[A-Z0-9]{6,12}$`,
			"zip":          `^\d{5}(-\d{4})?$`,
		},
	}
}

func (v *FormatValidator) Validate(value interface{}, options map[string]interface{}) (bool, string) {
	// Handle nil values - should pass (use required validator for presence checks)
	if value == nil {
		return true, ""
	}

	strValue := fmt.Sprintf("%v", value)
	strValue = strings.TrimSpace(strValue)

	// Empty values are handled by required validator
	if strValue == "" {
		return true, ""
	}

	// Get the pattern to use
	var pattern string
	var formatName string

	// Check if there's a preset format
	if format, ok := options["format"].(string); ok && format != "" {
		if presetPattern, exists := v.presets[format]; exists {
			pattern = presetPattern
			formatName = format
		}
	}

	// Override with custom regex if provided
	if regex, ok := options["regex"].(string); ok && regex != "" {
		pattern = regex
		formatName = "custom pattern"
	}

	if pattern == "" {
		return false, "No validation pattern specified"
	}

	fmt.Printf("🔍 [FormatValidator] Validating value: '%s' (type: %T) against %s (pattern: '%s')\n", strValue, value, formatName, pattern)

	// Compile and test regex
	re, err := regexp.Compile(pattern)
	if err != nil {
		return false, fmt.Sprintf("Invalid regex pattern: %v", err)
	}

	if !re.MatchString(strValue) {
		fmt.Printf("❌ [FormatValidator] NO MATCH: '%s' does not match '%s'\n", strValue, pattern)
		return false, fmt.Sprintf("Value '%s' does not match required %s format", strValue, formatName)
	}

	fmt.Printf("✅ [FormatValidator] MATCHED: '%s' matches %s\n", strValue, formatName)
	return true, ""
}

// ============================================================================
// LENGTH VALIDATOR
// ============================================================================

type LengthValidator struct {
	BaseValidator
}

func NewLengthValidator() *LengthValidator {
	return &LengthValidator{
		BaseValidator: BaseValidator{
			validatorType: "length",
			description:   "Validates field length (min, max, or exact)",
		},
	}
}

func (v *LengthValidator) Validate(value interface{}, options map[string]interface{}) (bool, string) {
	strValue := fmt.Sprintf("%v", value)
	length := len(strValue)

	// Check exact length
	if exact, ok := options["exact"].(float64); ok && exact > 0 {
		if length != int(exact) {
			return false, fmt.Sprintf("Field must be exactly %d characters (found %d)", int(exact), length)
		}
		return true, ""
	}

	// Check min length
	if min, ok := options["min"].(float64); ok && min > 0 {
		if length < int(min) {
			return false, fmt.Sprintf("Field must be at least %d characters (found %d)", int(min), length)
		}
	}

	// Check max length
	if max, ok := options["max"].(float64); ok && max > 0 {
		if length > int(max) {
			return false, fmt.Sprintf("Field must be at most %d characters (found %d)", int(max), length)
		}
	}

	return true, ""
}

// ============================================================================
// PATTERN VALIDATOR (Custom Regex)
// ============================================================================

type PatternValidator struct {
	BaseValidator
}

func NewPatternValidator() *PatternValidator {
	return &PatternValidator{
		BaseValidator: BaseValidator{
			validatorType: "pattern",
			description:   "Validates field against custom regex pattern",
		},
	}
}

func (v *PatternValidator) Validate(value interface{}, options map[string]interface{}) (bool, string) {
	// Handle nil values - should pass (use required validator for presence checks)
	if value == nil {
		return true, ""
	}

	strValue := fmt.Sprintf("%v", value)
	strValue = strings.TrimSpace(strValue)

	// Empty values are handled by required validator
	if strValue == "" {
		return true, ""
	}

	regex, ok := options["regex"].(string)
	if !ok || regex == "" {
		return false, "No regex pattern specified"
	}

	fmt.Printf("🔍 [PatternValidator] Validating value: '%s' (type: %T) against regex: '%s'\n", strValue, value, regex)

	re, err := regexp.Compile(regex)
	if err != nil {
		return false, fmt.Sprintf("Invalid regex pattern: %v", err)
	}

	if !re.MatchString(strValue) {
		fmt.Printf("❌ [PatternValidator] NO MATCH: '%s' does not match '%s'\n", strValue, regex)
		return false, fmt.Sprintf("Value '%s' does not match pattern: %s", strValue, regex)
	}

	fmt.Printf("✅ [PatternValidator] MATCHED: '%s' matches '%s'\n", strValue, regex)
	return true, ""
}
