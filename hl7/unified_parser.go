// FILE: unified_parser.go
// Fixed unified parser that properly uses existing functions from parser.go
package hl7

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// ✅ Standard container path - where schemas are mounted in Docker
	CONTAINER_SCHEMA_PATH = "/app/schemas"
)

// =====================================
// COMPATIBILITY STUBS FOR MAIN.GO
// =====================================

// Simple schema loader stub to satisfy main.go
type SchemaLoader struct {
	schemaDir string
}

var globalSchemaLoader *SchemaLoader

// InitSchemaLoader - Clean initialization with environment detection
func InitSchemaLoader(schemaDirectory string) {
	var finalSchemaDir string

	// Priority: Environment variable first
	envSchemaDir := os.Getenv("EZHEALTHKONNECT_SCHEMA_DIR")
	if envSchemaDir != "" {
		finalSchemaDir = envSchemaDir
	} else if _, err := os.Stat(CONTAINER_SCHEMA_PATH); err == nil {
		finalSchemaDir = CONTAINER_SCHEMA_PATH
	} else if schemaDirectory != "" {
		finalSchemaDir = schemaDirectory
	} else {
		finalSchemaDir = "./schemas"
	}

	// Ensure directory exists
	if _, err := os.Stat(finalSchemaDir); os.IsNotExist(err) {
		if err := os.MkdirAll(finalSchemaDir, 0755); err != nil {
			fmt.Printf("❌ ERROR: Failed to create schema directory: %v\n", err)
			return
		}
	}

	// Initialize loaders
	globalSchemaLoader = &SchemaLoader{
		schemaDir: finalSchemaDir,
	}

	InitRealSchemaLoader(finalSchemaDir)

	// Verify initialization - only log if failed
	realLoader := GetRealSchemaLoader()
	if realLoader == nil {
		fmt.Printf("❌ ERROR: Schema loader initialization failed\n")
		return
	}

	// Silent validation - only check for empty directory warning
	schemaFiles, err := scanForSchemaFiles(finalSchemaDir)
	if err != nil {
		fmt.Printf("❌ ERROR: Failed to scan schema files: %v\n", err)
	} else if len(schemaFiles) == 0 {
		fmt.Printf("⚠️ WARNING: No schema files found in %s\n", finalSchemaDir)
	}
}

// GetSchemaLoader - Return stub for main.go compatibility
func GetSchemaLoader() *SchemaLoader {
	return globalSchemaLoader
}

// SetMaxCacheSize - Stub function (does nothing)
func (sl *SchemaLoader) SetMaxCacheSize(maxSize int) {
	// No-op
}

// ClearCache - Stub function (does nothing)
func (sl *SchemaLoader) ClearCache() {
	// No-op
}

// GetCacheStats - Return real stats from real loader
func (sl *SchemaLoader) GetCacheStats() CacheStats {
	if realLoader := GetRealSchemaLoader(); realLoader != nil {
		realStats := realLoader.GetStats()
		return CacheStats{
			TotalLoads:  realStats.TotalLoads,
			CacheHits:   realStats.CacheHits,
			CacheMisses: realStats.CacheMisses,
			LoadErrors:  realStats.LoadErrors,
			LastLoaded:  realStats.LastLoaded,
			CacheSize:   realStats.CacheSize,
		}
	}

	return CacheStats{
		TotalLoads:  0,
		CacheHits:   0,
		CacheMisses: 0,
		LoadErrors:  0,
		LastLoaded:  "",
		CacheSize:   0,
	}
}

// ListAvailableSchemas - Return actual available schemas by scanning directory
func (sl *SchemaLoader) ListAvailableSchemas() ([]string, error) {
	schemas := []string{}

	// Use environment variable if available
	schemaDir := sl.schemaDir
	envSchemaDir := os.Getenv("EZHEALTHKONNECT_SCHEMA_DIR")
	if envSchemaDir != "" {
		schemaDir = envSchemaDir
	}

	// Scan the actual schema directory structure
	versionDirs, err := filepath.Glob(filepath.Join(schemaDir, "v*"))
	if err != nil {
		return schemas, fmt.Errorf("failed to scan schema directory: %w", err)
	}

	for _, versionDir := range versionDirs {
		version := filepath.Base(versionDir)

		// Find all .gz files in this version directory
		schemaFiles, err := filepath.Glob(filepath.Join(versionDir, "*.gz"))
		if err != nil {
			fmt.Printf("❌ ERROR: Failed to scan files in %s: %v\n", versionDir, err)
			continue
		}

		for _, schemaFile := range schemaFiles {
			filename := filepath.Base(schemaFile)
			schemaName := strings.TrimSuffix(filename, ".gz")
			fullSchema := fmt.Sprintf("%s_%s", version, schemaName)
			schemas = append(schemas, fullSchema)
		}
	}

	return schemas, nil
}

// =====================================
// UNIFIED PARSING - USING EXISTING FUNCTIONS
// =====================================

// ParseHL7Enhanced - Enhanced version that tries schema first
func ParseHL7Enhanced(rawMessage string) *EnhancedParsedMessage {
	fmt.Printf("🔍 ParseHL7Enhanced called\n")
	// Validate input
	if strings.TrimSpace(rawMessage) == "" {
		return &EnhancedParsedMessage{
			Raw:      rawMessage,
			Success:  false,
			Error:    "Empty message provided",
			ParsedAt: time.Now().Format(time.RFC3339),
		}
	}

	// Try schema parsing first
	realLoader := GetRealSchemaLoader()
	fmt.Printf("🔍 Real schema loader available: %v\n", realLoader != nil)

	if realLoader != nil {
		fmt.Printf("🔍 Attempting real schema parsing...\n")
		realResult := ParseWithRealSchema(rawMessage)

		if realResult != nil && realResult.Success {
			fmt.Printf("🔍 Real schema parsing success: %v, SchemaLoaded: %v\n", 
                realResult.Success, realResult.SchemaLoaded)
			// Check if we actually got schema-enhanced segments
			schemaSegmentCount := 0
			for _, segment := range realResult.EnhancedSegments {
				if strings.Contains(segment.DictionarySource, "RealSchemaLoader") ||
					strings.Contains(segment.DictionarySource, "Schema") {
					schemaSegmentCount++
				}
			}
			fmt.Printf("🔍 Schema-enhanced segments count: %d\n", schemaSegmentCount)

			// Use schema result if we have schema segments or explicit schema loading
			if schemaSegmentCount > 0 || realResult.SchemaLoaded {
				validatedResult := validateAndFixFieldPositioning(realResult)
				validatedResult.SchemaLoaded = true
				validatedResult.DictionaryUsed = true
				return validatedResult
			}
		}
	} else {
		fmt.Printf("🔍 Real schema parsing failed or returned nil\n")
		// Only log if environment should have schemas but doesn't
		envSchemaDir := os.Getenv("EZHEALTHKONNECT_SCHEMA_DIR")
		if envSchemaDir != "" {
			if _, err := os.Stat(envSchemaDir); os.IsNotExist(err) {
				fmt.Printf("❌ ERROR: Schema directory does not exist: %s\n", envSchemaDir)
			}
		}
	}

	// Fallback to basic parsing using CORRECT function name
	version, messageType, triggerEvent := extractMessageInfoForParsing(rawMessage)
	basicResult := ParseHL7MessageEnhanced(rawMessage) // ✅ CORRECT function name
	if basicResult == nil {
		return &EnhancedParsedMessage{
			Raw:      rawMessage,
			Success:  false,
			Error:    "Both schema and basic parsing failed",
			ParsedAt: time.Now().Format(time.RFC3339),
		}
	}

	// Convert using CORRECT function name
	enhancedSegments, segmentOrder := convertBasicToMapFormatFixed(basicResult.Segments)

	result := &EnhancedParsedMessage{
		Raw:              rawMessage,
		Success:          true,
		Version:          version,
		MessageType:      createMessageTypeInfoForParsing(messageType, triggerEvent),
		BasicSegments:    basicResult.Segments,
		EnhancedSegments: enhancedSegments,
		SegmentOrder:     segmentOrder,
		ParsedAt:         time.Now().Format(time.RFC3339),
		DictionaryUsed:   false,
		SchemaLoaded:     false,
		ValidationErrors: []ValidationError{},
	}

	validatedResult := validateAndFixFieldPositioning(result)
	return validatedResult
}

// ParseHL7EnhancedWithOptions parses an HL7 message and applies optional post-processing.
// Currently supports opts.EscapeHandling: "decode" | "passthrough" (default).
func ParseHL7EnhancedWithOptions(rawMessage string, opts ParseOptions) *EnhancedParsedMessage {
	result := ParseHL7Enhanced(rawMessage)
	if result == nil || !result.Success {
		return result
	}
	if opts.EscapeHandling == "decode" {
		encodingChars := ExtractEncodingChars(rawMessage)
		applyEscapeDecoding(result, encodingChars)
	}
	return result
}

// applyEscapeDecoding walks all field/subfield values in an EnhancedParsedMessage
// and replaces HL7 escape sequences with their literal characters.
func applyEscapeDecoding(result *EnhancedParsedMessage, encodingChars string) {
	decodeFields := func(fields []FieldInfo) {
		for i := range fields {
			fields[i].Value = DecodeHL7Escapes(fields[i].Value, encodingChars)
			for j := range fields[i].Subfields {
				fields[i].Subfields[j].Value = DecodeHL7Escapes(fields[i].Subfields[j].Value, encodingChars)
			}
		}
	}

	for k, seg := range result.EnhancedSegments {
		decodeFields(seg.Fields)
		result.EnhancedSegments[k] = seg
	}

	for segName, instances := range result.SegmentGroups {
		for i := range instances {
			decodeFields(instances[i].Fields)
		}
		result.SegmentGroups[segName] = instances
	}
}

// =====================================
// HELPER FUNCTIONS WITH ERROR-ONLY LOGGING
// =====================================

// convertBasicToMapFormatFixed - Convert using existing ConvertBasicToEnhanced function
func convertBasicToMapFormatFixed(basicSegments map[string]BasicSegment) (map[string]EnhancedSegment, []string) {
	// Use the CORRECT function name from parser.go
	enhancedArray := ConvertBasicToEnhancedWithDelimiters(basicSegments) // ✅ CORRECT function name

	enhancedMap := make(map[string]EnhancedSegment)
	segmentOrder := make([]string, 0, len(enhancedArray))

	for _, enhancedSeg := range enhancedArray {
		enhancedSeg.Fields = sortFieldsByPosition(enhancedSeg.Fields)
		enhancedMap[enhancedSeg.Key] = enhancedSeg
		segmentOrder = append(segmentOrder, enhancedSeg.Key)
	}

	return enhancedMap, segmentOrder
}

// sortFieldsByPosition - Sort fields by their HL7 position to ensure correct order
func sortFieldsByPosition(fields []FieldInfo) []FieldInfo {
	sortedFields := make([]FieldInfo, len(fields))
	copy(sortedFields, fields)

	// Simple bubble sort by position (primary) and sequence (secondary)
	for i := 0; i < len(sortedFields)-1; i++ {
		for j := i + 1; j < len(sortedFields); j++ {
			shouldSwap := false

			if sortedFields[i].Position > sortedFields[j].Position {
				shouldSwap = true
			} else if sortedFields[i].Position == sortedFields[j].Position {
				if sortedFields[i].Sequence > sortedFields[j].Sequence {
					shouldSwap = true
				}
			}

			if shouldSwap {
				sortedFields[i], sortedFields[j] = sortedFields[j], sortedFields[i]
			}
		}
	}

	return sortedFields
}

// validateAndFixFieldPositioning - Validate and fix field positioning issues
func validateAndFixFieldPositioning(result *EnhancedParsedMessage) *EnhancedParsedMessage {
	if result == nil || result.EnhancedSegments == nil {
		return result
	}

	validationErrors := []ValidationError{}

	for segName, segment := range result.EnhancedSegments {
		positionMap := make(map[int][]string)
		maxPosition := 0

		for i, field := range segment.Fields {
			// Validate field key format
			if !isValidFieldKey(field.Key, segName) {
				validationErrors = append(validationErrors, ValidationError{
					Severity:   SeverityWarning,
					Code:       ErrorCodeInvalidFieldKey,
					Message:    fmt.Sprintf("Invalid field key format: %s", field.Key),
					Segment:    segName,
					Field:      field.Key,
					Position:   field.Position,
					Suggestion: fmt.Sprintf("Expected format: %s.{position}", segName),
					RuleId:     fmt.Sprintf("KEY_%s_%d", segName, i),
				})
			}

			// Check for invalid positions
			if field.Position < 1 {
				validationErrors = append(validationErrors, ValidationError{
					Severity:   SeverityError,
					Code:       ErrorCodePositionMismatch,
					Message:    fmt.Sprintf("Invalid position %d for field %s (must be >= 1)", field.Position, field.Key),
					Segment:    segName,
					Field:      field.Key,
					Position:   field.Position,
					Suggestion: "Ensure field positions are 1-based",
					RuleId:     fmt.Sprintf("POS_%s_%d", segName, field.Position),
				})
			}

			positionMap[field.Position] = append(positionMap[field.Position], field.Key)
			if field.Position > maxPosition {
				maxPosition = field.Position
			}
		}

		// Check for duplicate positions - ERROR level
		for position, fieldKeys := range positionMap {
			if len(fieldKeys) > 1 {
				validationErrors = append(validationErrors, ValidationError{
					Severity:   SeverityError,
					Code:       ErrorCodeDuplicatePosition,
					Message:    fmt.Sprintf("Duplicate position %d in segment %s: %v", position, segName, fieldKeys),
					Segment:    segName,
					Position:   position,
					Suggestion: "Each field must have a unique position",
					RuleId:     fmt.Sprintf("DUP_%s_%d", segName, position),
				})
			}
		}

		// Fix field ordering
		fixedSegment := segment
		fixedSegment.Fields = sortFieldsByPosition(segment.Fields)
		result.EnhancedSegments[segName] = fixedSegment
	}

	// Add validation errors to result
	result.ValidationErrors = append(result.ValidationErrors, validationErrors...)

	// Only log if there are actual errors (not warnings)
	errorCount := countErrorsBySeverity(validationErrors, SeverityError)
	if errorCount > 0 {
		fmt.Printf("❌ Validation errors found: %d errors\n", errorCount)
	}

	return result
}

// isValidFieldKey - Validate field key format
func isValidFieldKey(fieldKey, segmentName string) bool {
	parts := strings.Split(fieldKey, ".")
	if len(parts) != 2 {
		return false
	}

	if parts[0] != segmentName {
		return false
	}

	_, err := extractFieldPosition(fieldKey)
	return err == nil
}

// countErrorsBySeverity - Count errors by severity
func countErrorsBySeverity(errors []ValidationError, severity string) int {
	count := 0
	for _, err := range errors {
		if err.Severity == severity {
			count++
		}
	}
	return count
}

// extractMessageInfoForParsing - Extract message info for parsing
func extractMessageInfoForParsing(rawMessage string) (version, messageType, triggerEvent string) {
	version = "2.5.1"
	messageType = "ADT"
	triggerEvent = "A01"

	lines := strings.Split(rawMessage, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "MSH|") {
			fields := strings.Split(line, "|")

			if len(fields) >= 12 {
				if v := strings.TrimSpace(fields[11]); v != "" {
					version = v
				}
			}

			if len(fields) >= 9 {
				msgField := strings.TrimSpace(fields[8])
				if msgField != "" {
					parts := strings.Split(msgField, "^")
					if len(parts) >= 1 {
						messageType = strings.TrimSpace(parts[0])
					}
					if len(parts) >= 2 {
						triggerEvent = strings.TrimSpace(parts[1])
					}
				}
			}
			break
		}
	}

	return version, messageType, triggerEvent
}

func createMessageTypeInfoForParsing(messageType, triggerEvent string) MessageTypeInfo {
	fullName := fmt.Sprintf("%s^%s", messageType, triggerEvent)

	descriptions := map[string]string{
		"ADT^A01": "Admit/visit notification",
		"ADT^A03": "Discharge/end visit",
		"ADT^A04": "Register a patient",
		"ADT^A08": "Update patient information",
		"ORU^R01": "Observation result",
		"ORM^O01": "Order message",
		"MDM^T02": "Medical document management",
		"SIU^S12": "Notification of new appointment booking",
		"SIU^S13": "Notification of appointment rescheduling",
		"SIU^S15": "Notification of appointment cancellation",
		"DFT^P03": "Post detail financial transaction",
	}

	description := descriptions[fullName]
	if description == "" {
		description = fmt.Sprintf("%s %s message", messageType, triggerEvent)
	}

	return MessageTypeInfo{
		Code:        messageType,
		Event:       triggerEvent,
		Name:        fullName,
		Description: description,
		Structure:   fmt.Sprintf("%s_%s", messageType, triggerEvent),
	}
}

// =====================================
// COMPATIBILITY FUNCTIONS
// =====================================

// ConvertBasicToEnhancedMap - Wrapper for existing function with map format conversion
func ConvertBasicToEnhancedMap(basicSegments map[string]BasicSegment) (map[string]EnhancedSegment, []string) {
	return convertBasicToMapFormatFixed(basicSegments)
}

// REMOVED ParseHL7MessageEnhanced from this file to avoid duplicate declaration
// The function already exists in parser.go and should be used from there

// =====================================
// DEBUGGING AND STATUS FUNCTIONS
// =====================================

// GetSchemaLoaderStatus returns current status of schema loader
func GetSchemaLoaderStatus() map[string]interface{} {
	envSchemaDir := os.Getenv("EZHEALTHKONNECT_SCHEMA_DIR")

	status := map[string]interface{}{
		"globalLoader":   globalSchemaLoader != nil,
		"realLoader":     GetRealSchemaLoader() != nil,
		"envVariable":    envSchemaDir,
		"envVariableSet": envSchemaDir != "",
	}

	if globalSchemaLoader != nil {
		status["configuredDir"] = globalSchemaLoader.schemaDir
	}

	if realLoader := GetRealSchemaLoader(); realLoader != nil {
		realStats := realLoader.GetStats()
		status["realStats"] = map[string]interface{}{
			"totalLoads":  realStats.TotalLoads,
			"cacheHits":   realStats.CacheHits,
			"cacheMisses": realStats.CacheMisses,
			"loadErrors":  realStats.LoadErrors,
			"cacheSize":   realStats.CacheSize,
			"lastLoaded":  realStats.LastLoaded,
		}
	}

	// Check if schema directory exists
	if envSchemaDir != "" {
		if _, err := os.Stat(envSchemaDir); err == nil {
			status["schemaDirectoryExists"] = true
		} else {
			status["schemaDirectoryExists"] = false
			status["schemaDirectoryError"] = err.Error()
		}
	}

	return status
}
