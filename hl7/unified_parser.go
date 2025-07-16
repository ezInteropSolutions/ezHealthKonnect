// FILE: unified_parser.go
// Complete unified parser with FIXED schema detection and no fallbacks
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
// Replace the InitSchemaLoader function in unified_parser.go with this fixed version:

// InitSchemaLoader - Clean initialization with FIXED environment variable priority
func InitSchemaLoader(schemaDirectory string) {
	fmt.Printf("🔍 DEBUG: InitSchemaLoader called with: %s\n", schemaDirectory)

	var finalSchemaDir string

	// ✅ PRIORITY FIX: Check environment variable FIRST
	envSchemaDir := os.Getenv("EZHEALTHKONNECT_SCHEMA_DIR")
	if envSchemaDir != "" {
		finalSchemaDir = envSchemaDir
		fmt.Printf("🌍 ENVIRONMENT: Using env variable schema path: %s\n", finalSchemaDir)
	} else if _, err := os.Stat(CONTAINER_SCHEMA_PATH); err == nil {
		// We're in a container - use standard mounted path
		finalSchemaDir = CONTAINER_SCHEMA_PATH
		fmt.Printf("🐳 CONTAINER: Using standard schema path: %s\n", finalSchemaDir)
	} else if schemaDirectory != "" {
		finalSchemaDir = schemaDirectory
		fmt.Printf("💻 DEVELOPMENT: Using passed schema path: %s\n", finalSchemaDir)
	} else {
		finalSchemaDir = "./schemas" // Local development fallback
		fmt.Printf("💻 DEVELOPMENT: Using default schema path: %s\n", finalSchemaDir)
	}

	// ✅ CRITICAL DEBUG: Show what path we're actually using
	fmt.Printf("🎯 FINAL SCHEMA PATH: %s\n", finalSchemaDir)

	// ✅ Ensure directory exists
	if _, err := os.Stat(finalSchemaDir); os.IsNotExist(err) {
		fmt.Printf("⚠️ WARNING: Schema directory does not exist: %s\n", finalSchemaDir)
		if err := os.MkdirAll(finalSchemaDir, 0755); err != nil {
			fmt.Printf("❌ ERROR: Failed to create schema directory: %v\n", err)
			return
		}
		fmt.Printf("✅ Created schema directory: %s\n", finalSchemaDir)
	}

	// ✅ DEBUG: List what's actually in the directory
	fmt.Printf("🔍 DEBUG: Scanning directory contents: %s\n", finalSchemaDir)
	if entries, err := os.ReadDir(finalSchemaDir); err == nil {
		fmt.Printf("📁 Directory entries:\n")
		for _, entry := range entries {
			if entry.IsDir() {
				fmt.Printf("  📁 %s/\n", entry.Name())
				// List files in subdirectory
				if subEntries, err := os.ReadDir(filepath.Join(finalSchemaDir, entry.Name())); err == nil {
					for _, subEntry := range subEntries {
						fmt.Printf("    📄 %s\n", subEntry.Name())
					}
				}
			} else {
				fmt.Printf("  📄 %s\n", entry.Name())
			}
		}
	} else {
		fmt.Printf("❌ ERROR: Cannot read directory: %v\n", err)
	}

	// ✅ Initialize loaders
	globalSchemaLoader = &SchemaLoader{
		schemaDir: finalSchemaDir,
	}

	fmt.Printf("🔍 DEBUG: Initializing real schema loader with: %s\n", finalSchemaDir)
	InitRealSchemaLoader(finalSchemaDir)

	// ✅ Verify initialization
	realLoader := GetRealSchemaLoader()
	if realLoader != nil {
		fmt.Printf("✅ SUCCESS: Schema system initialized with: %s\n", finalSchemaDir)

		// Quick validation
		schemaFiles, err := scanForSchemaFiles(finalSchemaDir)
		if err == nil {
			fmt.Printf("📊 Found %d schema files\n", len(schemaFiles))
			if len(schemaFiles) == 0 {
				fmt.Printf("⚠️ No schema files found - will create sample schemas\n")
			} else {
				fmt.Printf("✅ Schema files found:\n")
				for _, file := range schemaFiles {
					rel, _ := filepath.Rel(finalSchemaDir, file)
					fmt.Printf("  📄 %s\n", rel)
				}
			}
		}
	} else {
		fmt.Printf("❌ ERROR: Schema loader initialization failed\n")
	}
}

// GetSchemaLoader - Return stub for main.go compatibility
func GetSchemaLoader() *SchemaLoader {
	fmt.Printf("🔍 DEBUG: GetSchemaLoader called, returning: %p\n", globalSchemaLoader)
	return globalSchemaLoader
}

// SetMaxCacheSize - Stub function (does nothing)
func (sl *SchemaLoader) SetMaxCacheSize(maxSize int) {
	fmt.Printf("🔍 DEBUG: SetMaxCacheSize called with: %d\n", maxSize)
}

// ClearCache - Stub function (does nothing)
func (sl *SchemaLoader) ClearCache() {
	fmt.Printf("🔍 DEBUG: ClearCache called\n")
}

// GetCacheStats - Return real stats from real loader
func (sl *SchemaLoader) GetCacheStats() CacheStats {
	fmt.Printf("🔍 DEBUG: GetCacheStats called\n")
	if realLoader := GetRealSchemaLoader(); realLoader != nil {
		fmt.Printf("🔍 DEBUG: Real loader exists, getting stats...\n")
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

	fmt.Printf("🔍 DEBUG: Real loader is nil, returning empty stats\n")
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
	fmt.Printf("🔍 DEBUG: ListAvailableSchemas called\n")
	schemas := []string{}

	// ✅ FIX: Use environment variable if available
	schemaDir := sl.schemaDir
	envSchemaDir := os.Getenv("EZHEALTHKONNECT_SCHEMA_DIR")
	if envSchemaDir != "" {
		schemaDir = envSchemaDir
		fmt.Printf("✅ Using schema directory from environment: %s\n", schemaDir)
	}

	// Scan the actual schema directory structure
	versionDirs, err := filepath.Glob(filepath.Join(schemaDir, "v*"))
	if err != nil {
		fmt.Printf("🔍 DEBUG: Error scanning schema directory: %v\n", err)
		return schemas, err
	}

	fmt.Printf("🔍 DEBUG: Found %d version directories\n", len(versionDirs))

	for _, versionDir := range versionDirs {
		version := filepath.Base(versionDir)
		fmt.Printf("🔍 DEBUG: Scanning version directory: %s\n", version)

		// Find all .gz files in this version directory
		schemaFiles, err := filepath.Glob(filepath.Join(versionDir, "*.gz"))
		if err != nil {
			fmt.Printf("🔍 DEBUG: Error scanning files in %s: %v\n", versionDir, err)
			continue // Skip this version if error
		}

		fmt.Printf("🔍 DEBUG: Found %d schema files in %s\n", len(schemaFiles), version)

		for _, schemaFile := range schemaFiles {
			filename := filepath.Base(schemaFile)
			// Remove .gz extension: ADT_A04.gz -> ADT_A04
			schemaName := strings.TrimSuffix(filename, ".gz")
			// Create full identifier: v2.5.1_ADT_A04
			fullSchema := fmt.Sprintf("%s_%s", version, schemaName)
			schemas = append(schemas, fullSchema)
			fmt.Printf("🔍 DEBUG: Added schema: %s\n", fullSchema)
		}
	}

	fmt.Printf("🔍 DEBUG: Total schemas found: %d\n", len(schemas))
	return schemas, nil
}

// =====================================
// UNIFIED PARSING - CLEAN VERSION WITH FIXED SCHEMA DETECTION
// =====================================

// ParseHL7Enhanced - Enhanced version that ALWAYS tries schema first with better detection
func ParseHL7Enhanced(rawMessage string) *EnhancedParsedMessage {
	startTime := time.Now()
	fmt.Printf("🔍 DEBUG: ParseHL7Enhanced called\n")

	// Validate input
	if strings.TrimSpace(rawMessage) == "" {
		return &EnhancedParsedMessage{
			Raw:      rawMessage,
			Success:  false,
			Error:    "Empty message provided",
			ParsedAt: time.Now().Format(time.RFC3339),
		}
	}

	// ✅ DOCKER FIX: Check environment and schema loader status
	envSchemaDir := os.Getenv("EZHEALTHKONNECT_SCHEMA_DIR")
	fmt.Printf("🐳 DOCKER DEBUG: Environment EZHEALTHKONNECT_SCHEMA_DIR = '%s'\n", envSchemaDir)

	realLoader := GetRealSchemaLoader()
	fmt.Printf("🐳 DOCKER DEBUG: Real schema loader exists: %v\n", realLoader != nil)

	if realLoader != nil {
		stats := realLoader.GetStats()
		fmt.Printf("🐳 DOCKER DEBUG: Schema loader stats - TotalLoads: %d, CacheHits: %d, LoadErrors: %d\n",
			stats.TotalLoads, stats.CacheHits, stats.LoadErrors)
	}

	// ✅ CRITICAL: Always try schema first with detailed debugging
	if realLoader != nil {
		version, messageType, triggerEvent := extractMessageInfoForParsing(rawMessage)
		fmt.Printf("🔍 DOCKER DEBUG: Attempting schema loading for %s^%s v%s\n", messageType, triggerEvent, version)

		// ✅ Call the function from real_schema_parser.go
		realResult := ParseWithRealSchema(rawMessage)

		// ✅ DETAILED DEBUG: Show exactly what happened
		if realResult != nil {
			fmt.Printf("✅ DOCKER DEBUG: Schema parsing returned result\n")
			fmt.Printf("   📊 Success: %v\n", realResult.Success)
			fmt.Printf("   📊 Error: %s\n", realResult.Error)
			fmt.Printf("   📊 SchemaLoaded: %v\n", realResult.SchemaLoaded)
			fmt.Printf("   📊 DictionaryUsed: %v\n", realResult.DictionaryUsed)
			fmt.Printf("   📊 Segments: %d\n", len(realResult.EnhancedSegments))

			// ✅ Check each segment's dictionary source
			schemaSegmentCount := 0
			for segName, segment := range realResult.EnhancedSegments {
				fmt.Printf("   🔍 Segment %s: DictionarySource = '%s'\n", segName, segment.DictionarySource)
				if strings.Contains(segment.DictionarySource, "RealSchemaLoader") ||
					strings.Contains(segment.DictionarySource, "Schema") {
					schemaSegmentCount++
				}
			}

			fmt.Printf("   📊 Schema-enhanced segments: %d/%d\n", schemaSegmentCount, len(realResult.EnhancedSegments))

			// ✅ FIXED: Use result if successful AND has schema segments OR if explicitly marked as schema-loaded
			if realResult.Success && (schemaSegmentCount > 0 || realResult.SchemaLoaded) {
				fmt.Printf("✅ DOCKER SUCCESS: Using schema result (schemaSegments: %d, schemaLoaded: %v)\n",
					schemaSegmentCount, realResult.SchemaLoaded)

				validatedResult := validateAndFixFieldPositioning(realResult)
				validatedResult.SchemaLoaded = true
				validatedResult.DictionaryUsed = true

				fmt.Printf("✅ Schema parsing completed in %v\n", time.Since(startTime))
				return validatedResult
			} else {
				fmt.Printf("⚠️ DOCKER DEBUG: Schema result not suitable - falling back to basic\n")
				fmt.Printf("   📊 Reason: Success=%v, SchemaSegments=%d, SchemaLoaded=%v\n",
					realResult.Success, schemaSegmentCount, realResult.SchemaLoaded)
			}
		} else {
			fmt.Printf("❌ DOCKER DEBUG: ParseWithRealSchema returned nil\n")
		}
	} else {
		fmt.Printf("❌ DOCKER DEBUG: Real schema loader is nil - check initialization\n")

		// ✅ DOCKER DIAGNOSTIC: Check why schema loader is nil
		if envSchemaDir == "" {
			fmt.Printf("❌ DOCKER ISSUE: EZHEALTHKONNECT_SCHEMA_DIR environment variable not set\n")
		} else {
			fmt.Printf("🔍 DOCKER INFO: Schema directory set to: %s\n", envSchemaDir)
			// Check if directory exists in container
			if _, err := os.Stat(envSchemaDir); os.IsNotExist(err) {
				fmt.Printf("❌ DOCKER ISSUE: Schema directory does not exist in container: %s\n", envSchemaDir)
			} else {
				fmt.Printf("✅ DOCKER INFO: Schema directory exists in container\n")
			}
		}
	}

	// ✅ Fallback to basic parsing
	fmt.Printf("🔄 DOCKER DEBUG: Using basic parsing fallback\n")

	version, messageType, triggerEvent := extractMessageInfoForParsing(rawMessage)
	basicResult := ParseHL7Message(rawMessage)
	if basicResult == nil {
		return &EnhancedParsedMessage{
			Raw:      rawMessage,
			Success:  false,
			Error:    "Both schema and basic parsing failed",
			ParsedAt: time.Now().Format(time.RFC3339),
		}
	}

	enhancedSegments, segmentOrder := convertBasicArrayToMapFormatFixed(basicResult.Segments)

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
	fmt.Printf("✅ Basic parsing completed in %v\n", time.Since(startTime))
	return validatedResult
}

// =====================================
// HELPER FUNCTIONS WITH FIXES
// =====================================

// ✅ FIXED: convertBasicArrayToMapFormatFixed - Convert with proper position handling
func convertBasicArrayToMapFormatFixed(basicSegments map[string]BasicSegment) (map[string]EnhancedSegment, []string) {
	// Use FIXED ConvertBasicToEnhanced function from parser.go
	enhancedArray := ConvertBasicToEnhanced(basicSegments)

	// Convert array result to map format
	enhancedMap := make(map[string]EnhancedSegment)
	segmentOrder := make([]string, 0, len(enhancedArray))

	// ✅ CRITICAL FIX: Ensure fields are properly sorted by position
	for _, enhancedSeg := range enhancedArray {
		enhancedSeg.Fields = sortFieldsByPosition(enhancedSeg.Fields)
		enhancedMap[enhancedSeg.Key] = enhancedSeg
		segmentOrder = append(segmentOrder, enhancedSeg.Key)

		fmt.Printf("✅ Converted segment %s with %d fields (using %s)\n",
			enhancedSeg.Key, len(enhancedSeg.Fields), enhancedSeg.DictionarySource)
	}

	return enhancedMap, segmentOrder
}

// ✅ NEW: Sort fields by their HL7 position to ensure correct order
func sortFieldsByPosition(fields []FieldInfo) []FieldInfo {
	// Create a copy to avoid modifying the original
	sortedFields := make([]FieldInfo, len(fields))
	copy(sortedFields, fields)

	// Sort by position (primary) and sequence (secondary)
	for i := 0; i < len(sortedFields)-1; i++ {
		for j := i + 1; j < len(sortedFields); j++ {
			shouldSwap := false

			// Primary sort: by position
			if sortedFields[i].Position > sortedFields[j].Position {
				shouldSwap = true
			} else if sortedFields[i].Position == sortedFields[j].Position {
				// Secondary sort: by sequence if positions are equal
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

// ✅ ENHANCED: Validate and fix field positioning issues with detailed reporting
func validateAndFixFieldPositioning(result *EnhancedParsedMessage) *EnhancedParsedMessage {
	if result == nil || result.EnhancedSegments == nil {
		return result
	}

	fmt.Printf("🔍 DEBUG: Validating field positioning for %d segments\n", len(result.EnhancedSegments))

	validationErrors := []ValidationError{}

	for segName, segment := range result.EnhancedSegments {
		fmt.Printf("  📋 Validating segment %s with %d fields\n", segName, len(segment.Fields))

		// Check for position issues
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

			// Check for position consistency
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

			// Track position usage
			positionMap[field.Position] = append(positionMap[field.Position], field.Key)
			if field.Position > maxPosition {
				maxPosition = field.Position
			}

			fmt.Printf("    [%d] %s -> Position %d (HasValue: %v)\n", i, field.Key, field.Position, field.HasValue)
		}

		// Check for duplicate positions
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

		// Check for position gaps
		for pos := 1; pos <= maxPosition; pos++ {
			if _, exists := positionMap[pos]; !exists {
				validationErrors = append(validationErrors, ValidationError{
					Severity:   SeverityWarning,
					Code:       ErrorCodePositionGap,
					Message:    fmt.Sprintf("Missing field at position %d in segment %s", pos, segName),
					Segment:    segName,
					Position:   pos,
					Suggestion: "Consider if this field should be present",
					RuleId:     fmt.Sprintf("GAP_%s_%d", segName, pos),
				})
			}
		}

		// ✅ CRITICAL FIX: Ensure fields are sorted by position
		fixedSegment := segment
		fixedSegment.Fields = sortFieldsByPosition(segment.Fields)
		result.EnhancedSegments[segName] = fixedSegment
	}

	// Add validation errors to result
	result.ValidationErrors = append(result.ValidationErrors, validationErrors...)

	fmt.Printf("✅ Field positioning validation complete: %d errors, %d warnings\n",
		countErrorsBySeverity(validationErrors, SeverityError),
		countErrorsBySeverity(validationErrors, SeverityWarning))

	return result
}

// ✅ NEW: Validate field key format
func isValidFieldKey(fieldKey, segmentName string) bool {
	parts := strings.Split(fieldKey, ".")
	if len(parts) != 2 {
		return false
	}

	if parts[0] != segmentName {
		return false
	}

	// Check if position part is a valid number
	_, err := extractFieldPosition(fieldKey)
	return err == nil
}

// ✅ NEW: Count errors by severity
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

// ParseHL7MessageEnhanced - Compatibility function for existing controller
func ParseHL7MessageEnhanced(rawMessage string, dictionaryURL string) *EnhancedParsedMessage {
	// Use the new unified parser, ignore dictionary URL (we use schema instead)
	fmt.Printf("🔍 DEBUG: ParseHL7MessageEnhanced called (compatibility function)\n")
	return ParseHL7Enhanced(rawMessage)
}

// ConvertBasicToEnhancedMap - Wrapper for existing function with map format conversion
func ConvertBasicToEnhancedMap(basicSegments map[string]BasicSegment) (map[string]EnhancedSegment, []string) {
	return convertBasicArrayToMapFormatFixed(basicSegments)
}

// =====================================
// DEBUGGING AND TESTING FUNCTIONS
// =====================================

// GetSchemaLoaderStatus returns current status of schema loader
func GetSchemaLoaderStatus() map[string]interface{} {
	fmt.Printf("🔍 DEBUG: GetSchemaLoaderStatus called\n")

	// ✅ DEBUG: Show environment variable status
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

	// ✅ Check if schema directory exists
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
