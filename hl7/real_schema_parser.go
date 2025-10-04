// FILE: real_schema_parser.go
// Complete real schema-based HL7 parser with ERROR-ONLY logging
package hl7

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	orderedmap "github.com/wk8/go-ordered-map/v2"
)

// =====================================
// REAL SCHEMA STRUCTURES WITH SEQUENCE SUPPORT
// =====================================

type RealHL7Schema struct {
	Version      string                    `json:"version"`
	MessageType  string                    `json:"messageType"`
	Description  string                    `json:"description"`
	Segments     map[string]RealSegmentDef `json:"segments"`
	SegmentOrder []string                  `json:"segmentOrder"`
	Structure    []string                  `json:"structure"`
	LoadedAt     time.Time                 `json:"-"`
	FilePath     string                    `json:"-"`
}

type RealSegmentDef struct {
	Name          string                  `json:"name"`
	Description   string                  `json:"description"`
	Usage         string                  `json:"usage"`
	MaxUse        string                  `json:"maxUse"`
	Sequence      int                     `json:"sequence"`
	Fields        map[string]RealFieldDef `json:"fields"`
	OrderedFields []string                `json:"orderedFields"`
}

type RealFieldDef struct {
	Name              string                      `json:"name"`
	DataType          string                      `json:"dataType"`
	DataTypeName      string                      `json:"dataTypeName"`
	Length            int                         `json:"length"`
	Usage             string                      `json:"usage"`
	Repeat            string                      `json:"repeat"`
	Description       string                      `json:"description"`
	TableId           string                      `json:"tableId"`
	TableName         string                      `json:"tableName"`
	Components        map[string]RealComponentDef `json:"components"`
	OrderedComponents []string                    `json:"orderedComponents"`
	Values            map[string]string           `json:"values"`
	Position          int                         `json:"position"`
	Sequence          int                         `json:"sequence"`
}

type RealComponentDef struct {
	Name         string            `json:"name"`
	DataType     string            `json:"dataType"`
	DataTypeName string            `json:"dataTypeName"`
	Length       int               `json:"length"`
	Usage        string            `json:"usage"`
	Repeat       string            `json:"repeat"`
	Description  string            `json:"description"`
	TableId      string            `json:"tableId"`
	TableName    string            `json:"tableName"`
	Values       map[string]string `json:"values"`
	Position     int               `json:"position"`
	Sequence     int               `json:"sequence"`
}

// =====================================
// REAL SCHEMA LOADER
// =====================================

type RealSchemaLoader struct {
	schemaDir string
	cache     map[string]*RealHL7Schema
	cacheMux  sync.RWMutex
	stats     RealSchemaStats
	loadErrorMessages []string
}

type RealSchemaStats struct {
	TotalLoads   int       `json:"totalLoads"`
	CacheHits    int       `json:"cacheHits"`
	CacheMisses  int       `json:"cacheMisses"`
	LoadErrors   int       `json:"loadErrors"`
	LastLoaded   string    `json:"lastLoaded"`
	CacheSize    int       `json:"cacheSize"`
	LastLoadTime time.Time `json:"lastLoadTime"`
}

var realSchemaLoader *RealSchemaLoader

// InitRealSchemaLoader initializes the real schema loader
func InitRealSchemaLoader(schemaDirectory string) {
	if _, err := os.Stat(schemaDirectory); os.IsNotExist(err) {
		if err := os.MkdirAll(schemaDirectory, 0755); err != nil {
			fmt.Printf("❌ ERROR: Failed to create schema directory: %v\n", err)
			return
		}
		// Create version directories silently
		versionDirs := []string{"v2.3", "v2.4", "v2.5", "v2.5.1", "v2.6", "v2.7", "v2.8"}
		for _, version := range versionDirs {
			versionPath := filepath.Join(schemaDirectory, version)
			os.MkdirAll(versionPath, 0755) // Silent creation
		}
	}

	realSchemaLoader = &RealSchemaLoader{
		schemaDir: schemaDirectory,
		cache:     make(map[string]*RealHL7Schema),
	}

	// Silent validation - only create sample if no schemas exist
	schemaFiles, err := scanForSchemaFiles(schemaDirectory)
	if err != nil {
		fmt.Printf("❌ ERROR: Failed to scan schema files: %v\n", err)
	} else if len(schemaFiles) == 0 {
		createSampleSchema(schemaDirectory)
	}
}

// GetRealSchemaLoader returns the real schema loader
func GetRealSchemaLoader() *RealSchemaLoader {
	
	schemaDir := os.Getenv("EZHEALTHKONNECT_SCHEMA_DIR")
    fmt.Printf("🔍 EZHEALTHKONNECT_SCHEMA_DIR: %s\n", schemaDir)
    if schemaDir == "" {
        fmt.Printf("🔍 Error: EZHEALTHKONNECT_SCHEMA_DIR environment variable is not set\n")
        return nil
    }

    fmt.Printf("🔍 Searching for schema files in %s\n", schemaDir)
    files, err := os.ReadDir(schemaDir)
    if err != nil {
        fmt.Printf("🔍 Error reading schema directory: %v\n", err)
        return nil
    }

    fmt.Printf("🔍 Found %d files in schema directory\n", len(files))
    for _, file := range files {
        fmt.Printf("🔍 File: %s\n", file.Name())
        if file.Name() == "ADT_A01.json" {
            fmt.Printf("🔍 Found ADT_A01 schema file: %s\n", file.Name())
            // ... (rest of the code remains the same)
        }
    }

    // ... (rest of the code remains the same)

    // Add some logging to see what's happening when we try to load the schema
    if realSchemaLoader.loadErrorMessages != nil {
        fmt.Printf("🔍 Real schema loader errors:\n")
        for _, err := range realSchemaLoader.loadErrorMessages {
            fmt.Printf("🔍   - %v\n", err)
        }
    }
	return realSchemaLoader
}

// scanForSchemaFiles scans directory for .gz schema files
func scanForSchemaFiles(baseDir string) ([]string, error) {
	var schemaFiles []string
	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if strings.HasSuffix(strings.ToLower(info.Name()), ".gz") {
			schemaFiles = append(schemaFiles, path)
		}
		return nil
	})
	return schemaFiles, err
}

// createSampleSchema creates a sample schema file for testing
func createSampleSchema(baseDir string) {
	versionDir := filepath.Join(baseDir, "v2.5.1")
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		fmt.Printf("❌ ERROR: Failed to create version directory: %v\n", err)
		return
	}

	sampleSchema := map[string]interface{}{
		"version":      "2.5.1",
		"messageType":  "ADT_A04",
		"description":  "Register a Patient",
		"segmentOrder": []string{"MSH", "PID"},
		"segments": map[string]interface{}{
			"MSH": map[string]interface{}{
				"name":          "Message Header",
				"description":   "The MSH segment defines the intent, source, destination, and some specifics of the syntax of a message.",
				"usage":         "R",
				"maxUse":        "1",
				"sequence":      0,
				"orderedFields": []string{"MSH.1", "MSH.2", "MSH.9"},
				"fields": map[string]interface{}{
					"MSH.1": map[string]interface{}{
						"name":         "Field Separator",
						"dataType":     "ST",
						"dataTypeName": "String Data",
						"length":       1,
						"usage":        "R",
						"repeat":       "1",
						"description":  "This field contains the separator between the segment ID and the first real field.",
						"position":     1,
						"sequence":     0,
					},
					"MSH.2": map[string]interface{}{
						"name":         "Encoding Characters",
						"dataType":     "ST",
						"dataTypeName": "String Data",
						"length":       4,
						"usage":        "R",
						"repeat":       "1",
						"description":  "This field contains the four encoding characters.",
						"position":     2,
						"sequence":     1,
					},
					"MSH.9": map[string]interface{}{
						"name":         "Message Type",
						"dataType":     "MSG",
						"dataTypeName": "Message Type",
						"length":       15,
						"usage":        "R",
						"repeat":       "1",
						"description":  "This field contains the message type, trigger event, and the message structure ID for the message.",
						"position":     9,
						"sequence":     2,
					},
				},
			},
			"PID": map[string]interface{}{
				"name":          "Patient Identification",
				"description":   "The PID segment is used by all applications as the primary means of communicating patient identification information.",
				"usage":         "R",
				"maxUse":        "1",
				"sequence":      1,
				"orderedFields": []string{"PID.1", "PID.5", "PID.8"},
				"fields": map[string]interface{}{
					"PID.1": map[string]interface{}{
						"name":         "Set ID - PID",
						"dataType":     "SI",
						"dataTypeName": "Sequence ID",
						"length":       4,
						"usage":        "O",
						"repeat":       "1",
						"description":  "This field contains the number that identifies this patient record among multiple patient records.",
						"position":     1,
						"sequence":     0,
					},
					"PID.5": map[string]interface{}{
						"name":         "Patient Name",
						"dataType":     "XPN",
						"dataTypeName": "Extended Person Name",
						"length":       250,
						"usage":        "B",
						"repeat":       "*",
						"description":  "This field contains the legal name of the patient.",
						"position":     5,
						"sequence":     1,
					},
					"PID.8": map[string]interface{}{
						"name":         "Administrative Sex",
						"dataType":     "IS",
						"dataTypeName": "Coded value for user-defined tables",
						"length":       1,
						"usage":        "O",
						"repeat":       "1",
						"description":  "This field contains the patient's sex.",
						"position":     8,
						"sequence":     2,
						"tableId":      "0001",
						"tableName":    "Administrative Sex",
						"values": map[string]interface{}{
							"F": "Female",
							"M": "Male",
							"O": "Other",
							"U": "Unknown",
						},
					},
				},
			},
		},
	}

	schemaPath := filepath.Join(versionDir, "ADT_A04.gz")
	file, err := os.Create(schemaPath)
	if err != nil {
		fmt.Printf("❌ ERROR: Failed to create sample schema file: %v\n", err)
		return
	}
	defer file.Close()

	gzipWriter := gzip.NewWriter(file)
	defer gzipWriter.Close()

	encoder := json.NewEncoder(gzipWriter)
	encoder.SetIndent("", "  ")

	if err := encoder.Encode(sampleSchema); err != nil {
		fmt.Printf("❌ ERROR: Failed to write sample schema: %v\n", err)
		return
	}
	// Silent success - no logging for sample schema creation
}

// LoadRealSchema loads and parses actual schema from .gz file
func (rsl *RealSchemaLoader) LoadRealSchema(version, messageType, triggerEvent string) (*RealHL7Schema, error) {
	rsl.cacheMux.Lock()
	defer rsl.cacheMux.Unlock()

	cacheKey := fmt.Sprintf("%s_%s_%s", version, messageType, triggerEvent)

	if schema, exists := rsl.cache[cacheKey]; exists {
		rsl.stats.CacheHits++
		return schema, nil
	}

	rsl.stats.CacheMisses++
	rsl.stats.TotalLoads++

	normalizedVersion := normalizeVersionForSchema(version)
	filename := fmt.Sprintf("%s_%s.gz", messageType, triggerEvent)
	schemaPath := filepath.Join(rsl.schemaDir, normalizedVersion, filename)
	// Around line where it builds the schema path
	fmt.Printf("🔍 DEBUG: Looking for schema file: %s\n", schemaPath)

	if _, err := os.Stat(schemaPath); os.IsNotExist(err) {
		// Try alternatives silently
		alternates := []string{
			fmt.Sprintf("%s^%s.gz", messageType, triggerEvent),
			fmt.Sprintf("%s.gz", messageType),
			fmt.Sprintf("%s_%s_%s.gz", messageType, triggerEvent, normalizedVersion),
		}

		for _, altFilename := range alternates {
			altPath := filepath.Join(rsl.schemaDir, normalizedVersion, altFilename)
			if _, err := os.Stat(altPath); err == nil {
				schemaPath = altPath
				break
			}
		}

		if _, err := os.Stat(schemaPath); os.IsNotExist(err) {
			rsl.stats.LoadErrors++
			return nil, fmt.Errorf("schema file not found: %s", schemaPath)
		}

		
		
	}

	schema, err := rsl.loadAndParseSchemaFile(schemaPath)
	if err != nil {
		rsl.stats.LoadErrors++
		fmt.Printf("❌ ERROR: Schema loading failed for %s: %v\n", cacheKey, err)
    	fmt.Printf("❌ ERROR: Attempted schema path: %s\n", schemaPath)
		return nil, fmt.Errorf("failed to load schema %s: %v", cacheKey, err)
	}

	rsl.cache[cacheKey] = schema
	rsl.stats.LastLoaded = cacheKey
	rsl.stats.CacheSize = len(rsl.cache)
	rsl.stats.LastLoadTime = time.Now()

	return schema, nil
}

// normalizeVersionForSchema normalizes version for file path
func normalizeVersionForSchema(version string) string {
	version = strings.TrimSpace(version)
	if !strings.HasPrefix(version, "v") && !strings.HasPrefix(version, "V") {
		return "v" + version
	}
	return version
}

// loadAndParseSchemaFile loads and parses schema file
func (rsl *RealSchemaLoader) loadAndParseSchemaFile(filePath string) (*RealHL7Schema, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("cannot open schema file %s: %v", filePath, err)
	}
	defer file.Close()

	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return nil, fmt.Errorf("cannot create gzip reader for %s: %v", filePath, err)
	}
	defer gzipReader.Close()

	var rawOrderedSchema *orderedmap.OrderedMap[string, interface{}]
	rawOrderedSchema = orderedmap.New[string, interface{}]()

	decoder := json.NewDecoder(gzipReader)
	if err := decoder.Decode(&rawOrderedSchema); err != nil {
		return nil, fmt.Errorf("cannot decode JSON from %s: %v", filePath, err)
	}

	schema, err := rsl.convertOrderedRawSchema(rawOrderedSchema, filePath)
	if err != nil {
		return nil, fmt.Errorf("cannot convert schema structure: %v", err)
	}

	schema.LoadedAt = time.Now()
	schema.FilePath = filePath

	return schema, nil
}

// convertOrderedRawSchema converts raw schema to structured format
func (rsl *RealSchemaLoader) convertOrderedRawSchema(raw *orderedmap.OrderedMap[string, interface{}], filePath string) (*RealHL7Schema, error) {
	schema := &RealHL7Schema{
		Segments:     make(map[string]RealSegmentDef),
		SegmentOrder: make([]string, 0),
	}

	if version, exists := raw.Get("version"); exists {
		if v, ok := version.(string); ok {
			schema.Version = v
		}
	}
	if msgType, exists := raw.Get("messageType"); exists {
		if mt, ok := msgType.(string); ok {
			schema.MessageType = mt
		}
	}
	if desc, exists := raw.Get("description"); exists {
		if d, ok := desc.(string); ok {
			schema.Description = d
		}
	}

	segmentCount := 0

	// Look for "structure" key first, then "segments" as fallback
	if structureValue, exists := raw.Get("structure"); exists {
		if structureMap, ok := structureValue.(map[string]interface{}); ok {
			// Process each segment in the structure
			sequenceCounter := 0
			for segKey, segValue := range structureMap {
				if segmentMap, ok := segValue.(map[string]interface{}); ok {
					segmentDef, err := rsl.convertOrderedSegment(segKey, segmentMap, sequenceCounter)
					if err != nil {
						continue // Skip on error, don't log
					}
					schema.Segments[segKey] = segmentDef
					schema.SegmentOrder = append(schema.SegmentOrder, segKey)
					segmentCount++
					sequenceCounter++
				}
			}
		}
	} else if segmentsValue, exists := raw.Get("segments"); exists {
		// Fallback: also support "segments" key for backward compatibility
		if segmentsMap, ok := segmentsValue.(map[string]interface{}); ok {
			// Process segments directly
			sequenceCounter := 0
			for segKey, segValue := range segmentsMap {
				if segmentMap, ok := segValue.(map[string]interface{}); ok {
					segmentDef, err := rsl.convertOrderedSegment(segKey, segmentMap, sequenceCounter)
					if err != nil {
						continue // Skip on error, don't log
					}
					schema.Segments[segKey] = segmentDef
					schema.SegmentOrder = append(schema.SegmentOrder, segKey)
					segmentCount++
					sequenceCounter++
				}
			}
		}
	}

	// Sort segments by sequence
	if len(schema.SegmentOrder) > 0 {
		type segmentForSorting struct {
			key      string
			sequence int
		}

		var segmentsForSorting []segmentForSorting
		for _, segKey := range schema.SegmentOrder {
			if segDef, exists := schema.Segments[segKey]; exists {
				segmentsForSorting = append(segmentsForSorting, segmentForSorting{
					key:      segKey,
					sequence: segDef.Sequence,
				})
			}
		}

		sort.Slice(segmentsForSorting, func(i, j int) bool {
			return segmentsForSorting[i].sequence < segmentsForSorting[j].sequence
		})

		schema.SegmentOrder = make([]string, 0, len(segmentsForSorting))
		for _, seg := range segmentsForSorting {
			schema.SegmentOrder = append(schema.SegmentOrder, seg.key)
		}
	}

	return schema, nil
}

// convertOrderedSegment converts segment definition
func (rsl *RealSchemaLoader) convertOrderedSegment(segmentName string, raw map[string]interface{}, defaultSequence int) (RealSegmentDef, error) {
	extractedSequence := extractSequenceFromRaw(raw, defaultSequence)

	segment := RealSegmentDef{
		Name:          segmentName,
		Fields:        make(map[string]RealFieldDef),
		OrderedFields: make([]string, 0),
		Sequence:      extractedSequence,
	}

	if name, ok := raw["name"].(string); ok {
		segment.Name = name
	}
	if desc, ok := raw["description"].(string); ok {
		segment.Description = desc
	}
	if usage, ok := raw["usage"].(string); ok {
		segment.Usage = usage
	}
	if maxUse, ok := raw["maxUse"].(string); ok {
		segment.MaxUse = maxUse
	}

	// Check if fields exist and process them with proper positioning
	if fields, ok := raw["fields"].(map[string]interface{}); ok {
		// Create field position map to handle gaps
		fieldPositions := make(map[int]RealFieldDef)
		maxPosition := 0

		// First pass: collect all fields and their positions
		fieldSequence := 0
		for fieldKey, fieldValue := range fields {
			if fieldData, ok := fieldValue.(map[string]interface{}); ok {
				fieldDef, err := rsl.convertOrderedField(fieldKey, fieldData, fieldSequence)
				if err != nil {
					continue // Skip on error
				}

				// Extract and validate position
				if pos, err := extractFieldPosition(fieldKey); err == nil {
					fieldDef.Position = pos
					if pos > maxPosition {
						maxPosition = pos
					}
					fieldPositions[pos] = fieldDef
				} else {
					// Use sequence as fallback position
					fieldDef.Position = fieldSequence + 1
					fieldPositions[fieldSequence+1] = fieldDef
					if fieldSequence+1 > maxPosition {
						maxPosition = fieldSequence + 1
					}
				}

				fieldSequence++
			}
		}

		// Generate ordered fields for ALL positions 1 to maxPosition
		for position := 1; position <= maxPosition; position++ {
			fieldKey := fmt.Sprintf("%s.%d", segmentName, position)

			if fieldDef, exists := fieldPositions[position]; exists {
				// Use the actual field definition
				segment.Fields[fieldKey] = fieldDef
				segment.OrderedFields = append(segment.OrderedFields, fieldKey)
			} else {
				// Create placeholder field for missing positions to maintain alignment
				placeholderField := RealFieldDef{
					Name:        fmt.Sprintf("Field %d", position),
					DataType:    "ST",
					Usage:       "O",
					Description: fmt.Sprintf("%s field %d (not defined in schema)", segmentName, position),
					Position:    position,
					Sequence:    position - 1,
					Components:  make(map[string]RealComponentDef),
					Values:      make(map[string]string),
				}
				segment.Fields[fieldKey] = placeholderField
				segment.OrderedFields = append(segment.OrderedFields, fieldKey)
			}
		}
	}

	return segment, nil
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// convertOrderedField converts field definition
func (rsl *RealSchemaLoader) convertOrderedField(fieldKey string, raw map[string]interface{}, defaultSequence int) (RealFieldDef, error) {
	field := RealFieldDef{
		Components:        make(map[string]RealComponentDef),
		OrderedComponents: make([]string, 0),
		Values:            make(map[string]string),
		Sequence:          extractSequenceFromRaw(raw, defaultSequence),
	}

	if name, ok := raw["name"].(string); ok {
		field.Name = name
	}
	if dataType, ok := raw["dataType"].(string); ok {
		field.DataType = dataType
	}
	if dataTypeName, ok := raw["dataTypeName"].(string); ok {
		field.DataTypeName = dataTypeName
	}
	if length, ok := raw["length"].(float64); ok {
		field.Length = int(length)
	}
	if usage, ok := raw["usage"].(string); ok {
		field.Usage = usage
	}
	if repeat, ok := raw["repeat"].(string); ok {
		field.Repeat = repeat
	}
	if desc, ok := raw["description"].(string); ok && desc != "" && desc != "null" {
		field.Description = desc
	} else {
		field.Description = fmt.Sprintf("%s field", field.Name)
	}
	if tableId, ok := raw["tableId"].(string); ok && tableId != "" && tableId != "null" {
		field.TableId = tableId
	}
	if tableName, ok := raw["tableName"].(string); ok && tableName != "" && tableName != "null" {
		field.TableName = tableName
	}

	if components, ok := raw["components"].(map[string]interface{}); ok {
		orderedComponents := orderedmap.New[string, interface{}]()
		componentsJSON, _ := json.Marshal(components)
		json.Unmarshal(componentsJSON, &orderedComponents)

		compSequence := 0
		for pair := orderedComponents.Oldest(); pair != nil; pair = pair.Next() {
			compKey := pair.Key
			compValue := pair.Value

			if compData, ok := compValue.(map[string]interface{}); ok {
				compDef := rsl.convertComponent(compKey, compData, compSequence)

				if pos, err := extractComponentPosition(compKey); err == nil {
					compDef.Position = pos
				}

				field.Components[compKey] = compDef
				field.OrderedComponents = append(field.OrderedComponents, compKey)
				compSequence++
			}
		}
	}

	if values, ok := raw["values"].(map[string]interface{}); ok {
		for key, value := range values {
			if strValue, ok := value.(string); ok {
				field.Values[key] = strValue
			}
		}
	}

	return field, nil
}

// convertComponent converts component definition
func (rsl *RealSchemaLoader) convertComponent(compKey string, raw map[string]interface{}, defaultSequence int) RealComponentDef {
	comp := RealComponentDef{
		Values:   make(map[string]string),
		Sequence: extractSequenceFromRaw(raw, defaultSequence),
	}

	if name, ok := raw["name"].(string); ok {
		comp.Name = name
	}
	if dataType, ok := raw["dataType"].(string); ok {
		comp.DataType = dataType
	}
	if dataTypeName, ok := raw["dataTypeName"].(string); ok {
		comp.DataTypeName = dataTypeName
	}
	if length, ok := raw["length"].(float64); ok {
		comp.Length = int(length)
	}
	if usage, ok := raw["usage"].(string); ok {
		comp.Usage = usage
	}
	if repeat, ok := raw["repeat"].(string); ok {
		comp.Repeat = repeat
	}

	if desc, ok := raw["description"].(string); ok && desc != "" && desc != "null" {
		comp.Description = desc
	} else {
		comp.Description = fmt.Sprintf("%s component", comp.Name)
	}

	if tableId, ok := raw["tableId"].(string); ok && tableId != "" && tableId != "null" {
		comp.TableId = tableId
	}
	if tableName, ok := raw["tableName"].(string); ok && tableName != "" && tableName != "null" {
		comp.TableName = tableName
	}

	if values, ok := raw["values"].(map[string]interface{}); ok {
		for key, value := range values {
			if strValue, ok := value.(string); ok {
				comp.Values[key] = strValue
			}
		}
	}

	return comp
}

// Helper functions
func extractSequenceFromRaw(raw map[string]interface{}, defaultSeq int) int {
	if sequenceValue, exists := raw["sequence"]; exists {
		if sequence, ok := sequenceValue.(float64); ok {
			return int(sequence)
		}
		if sequenceStr, ok := sequenceValue.(string); ok {
			if parsed, err := strconv.Atoi(sequenceStr); err == nil {
				return parsed
			}
		}
		if sequence, ok := sequenceValue.(int); ok {
			return sequence
		}
	}
	return defaultSeq
}

func extractComponentPosition(compKey string) (int, error) {
	parts := strings.Split(compKey, ".")
	if len(parts) >= 3 {
		return strconv.Atoi(parts[len(parts)-1])
	}
	return 0, fmt.Errorf("invalid component key format: %s", compKey)
}

func extractComponentValue(fieldValue string, componentPosition int) string {
	if fieldValue == "" {
		return ""
	}
	components := strings.Split(fieldValue, "^")
	if componentPosition > 0 && componentPosition <= len(components) {
		return strings.TrimSpace(components[componentPosition-1])
	}
	return ""
}

func (rsl *RealSchemaLoader) GetStats() RealSchemaStats {
	rsl.cacheMux.RLock()
	defer rsl.cacheMux.RUnlock()
	stats := rsl.stats
	stats.CacheSize = len(rsl.cache)
	return stats
}

// =====================================
// PARSING FUNCTIONS WITH MAP-BASED SEGMENTS
// =====================================

func ParseWithRealSchema(rawMessage string) *EnhancedParsedMessage {
	version, messageType, triggerEvent := extractMessageInfoSimple(rawMessage)

	if realSchemaLoader == nil {
		return &EnhancedParsedMessage{
			Raw:      rawMessage,
			Success:  false,
			Error:    "Schema loader not initialized",
			ParsedAt: time.Now().Format(time.RFC3339),
		}
	}

	schema, err := realSchemaLoader.LoadRealSchema(version, messageType, triggerEvent)
	if err != nil {
		return &EnhancedParsedMessage{
			Raw:      rawMessage,
			Success:  false,
			Error:    fmt.Sprintf("Schema loading failed: %v", err),
			ParsedAt: time.Now().Format(time.RFC3339),
		}
	}

	// Parse the message with basic parser first
	basicResult := ParseHL7MessageEnhanced(rawMessage)
	if basicResult == nil {
		return &EnhancedParsedMessage{
			Raw:      rawMessage,
			Success:  false,
			Error:    "Basic parsing failed",
			ParsedAt: time.Now().Format(time.RFC3339),
		}
	}

	// Enhance segments with loaded schema
	enhancedSegments, segmentOrder := enhanceWithRealSchema(basicResult.Segments, schema)

	// Add validation for missing required fields
	validationErrors := validateRequiredFields(enhancedSegments, schema)

	result := &EnhancedParsedMessage{
		Raw:              rawMessage,
		Success:          true,
		Version:          version,
		MessageType:      createMessageTypeInfo(messageType, triggerEvent),
		BasicSegments:    basicResult.Segments,
		EnhancedSegments: enhancedSegments,
		SegmentOrder:     segmentOrder,
		ParsedAt:         time.Now().Format(time.RFC3339),
		DictionaryUsed:   true,
		SchemaLoaded:     true,
		ValidationErrors: validationErrors,
	}

	return result
}

// enhanceWithRealSchema - Enhanced with proper position handling
func enhanceWithRealSchema(basicSegments map[string]BasicSegment, schema *RealHL7Schema) (map[string]EnhancedSegment, []string) {
	enhancedSegments := make(map[string]EnhancedSegment)
	var segmentOrder []string

	// Determine segment order from schema or use basic order
	if schema != nil && len(schema.SegmentOrder) > 0 {
		segmentOrder = schema.SegmentOrder
	} else {
		segmentOrder = getBasicHL7Order(basicSegments)
	}

	// Process ALL basic segments, ensuring none are missed
	processedSegments := make(map[string]bool)

	// First pass: Process segments in the determined order
	for _, segmentName := range segmentOrder {
		if basicSeg, exists := basicSegments[segmentName]; exists {
			enhancedSeg := createEnhancedSegmentFromBasic(segmentName, basicSeg, schema)
			enhancedSegments[segmentName] = enhancedSeg
			processedSegments[segmentName] = true
		}
	}

	// Second pass: Process any remaining basic segments not in the order
	for segmentName, basicSeg := range basicSegments {
		if !processedSegments[segmentName] {
			enhancedSeg := createEnhancedSegmentFromBasic(segmentName, basicSeg, schema)
			enhancedSegments[segmentName] = enhancedSeg
			segmentOrder = append(segmentOrder, segmentName)
		}
	}

	return enhancedSegments, segmentOrder
}

// Helper function to create enhanced segment (uses schema if available, falls back to basic conversion)
func createEnhancedSegmentFromBasic(segmentName string, basicSeg BasicSegment, schema *RealHL7Schema) EnhancedSegment {
	// If we have schema definition, use it
	if schema != nil {
		if segmentDef, hasSchema := schema.Segments[segmentName]; hasSchema {
			return createEnhancedSegmentWithSchema(segmentName, basicSeg, segmentDef)
		}
	}

	// Otherwise, fall back to existing basic conversion logic
	enhancedArray := ConvertBasicToEnhancedWithDelimiters(map[string]BasicSegment{segmentName: basicSeg})
	if len(enhancedArray) > 0 {
		enhanced := enhancedArray[0]
		return enhanced
	}

	// Last resort: create minimal enhanced segment
	return EnhancedSegment{
		Key:              segmentName,
		Raw:              basicSeg.Raw,
		Name:             fmt.Sprintf("%s Segment", segmentName),
		Description:      fmt.Sprintf("%s segment", segmentName),
		Purpose:          "Standard HL7 segment (basic conversion)",
		DictionarySource: "BasicConverter_v1.0",
		Fields:           []FieldInfo{},
		FieldCount:       0,
	}
}

// Helper function to create enhanced segment using schema definition with FIXED positioning
func createEnhancedSegmentWithSchema(segmentName string, basicSeg BasicSegment, segmentDef RealSegmentDef) EnhancedSegment {
	enhancedSeg := EnhancedSegment{
		Key:              segmentName,
		Raw:              basicSeg.Raw,
		Name:             segmentDef.Name,
		Description:      segmentDef.Description,
		Purpose:          fmt.Sprintf("%s (HL7 Schema Enhanced)", segmentDef.Description),
		DictionarySource: "RealSchemaLoader_v1.0",
		Required:         (segmentDef.Usage == "R"),
		Sequence:         segmentDef.Sequence,
		Fields:           make([]FieldInfo, 0),
	}

	// Create fields for ALL positions to maintain order and use schema descriptions
	maxPosition := 0
	schemaFieldPositions := make(map[int]RealFieldDef)

	// First, map all schema fields by position
	for _, fieldDef := range segmentDef.Fields {
		if fieldDef.Position > maxPosition {
			maxPosition = fieldDef.Position
		}
		schemaFieldPositions[fieldDef.Position] = fieldDef
	}

	// Process ALL positions from 1 to maxPosition using schema definitions
	for position := 1; position <= maxPosition; position++ {
		fieldKey := fmt.Sprintf("%s.%d", segmentName, position)

		// Get field value from basic parsing (may be empty)
		fieldValue := ""
		hasValue := false
		if value, exists := basicSeg.Fields[fieldKey]; exists {
			fieldValue = value
			hasValue = (value != "")
		}

		// Use schema definition if available
		if schemaDef, hasSchema := schemaFieldPositions[position]; hasSchema {
			fieldInfo := FieldInfo{
				Key:         fieldKey,
				Value:       fieldValue,
				Name:        schemaDef.Name,
				Description: schemaDef.Description,
				DataType:    schemaDef.DataType,
				Optionality: schemaDef.Usage,
				Cardinality: fmt.Sprintf("[%s]", schemaDef.Repeat),
				Position:    position,
				Sequence:    schemaDef.Sequence,
				HasValue:    hasValue,
				Length:      schemaDef.Length,
				TableId:     schemaDef.TableId,
			}

			// Add rich subfields with schema component information
			if len(schemaDef.OrderedComponents) > 0 {
				subfields := make([]SubfieldInfo, 0, len(schemaDef.OrderedComponents))
				for _, compKey := range schemaDef.OrderedComponents {
					if comp, exists := schemaDef.Components[compKey]; exists {
						componentValue := ""
						componentHasValue := false
						if hasValue {
							componentValue = extractComponentValue(fieldValue, comp.Position)
							componentHasValue = (componentValue != "")
						}

						subfield := SubfieldInfo{
							Key:         compKey,
							Name:        comp.Name,
							DataType:    comp.DataType,
							Usage:       comp.Usage,
							Length:      comp.Length,
							Position:    comp.Position,
							Sequence:    comp.Sequence,
							Description: comp.Description,
							TableId:     comp.TableId,
							HasValue:    componentHasValue,
							Value:       componentValue,
						}
						subfields = append(subfields, subfield)
					}
				}
				fieldInfo.Subfields = subfields
			} else {
				fieldInfo.Subfields = []SubfieldInfo{}
			}

			enhancedSeg.Fields = append(enhancedSeg.Fields, fieldInfo)
		} else if hasValue {
			// Field has value but no schema definition - create basic field
			fieldInfo := FieldInfo{
				Key:         fieldKey,
				Value:       fieldValue,
				Name:        fmt.Sprintf("Field %d", position),
				Description: fmt.Sprintf("%s field %d (no schema definition)", segmentName, position),
				DataType:    "ST",
				Optionality: "O",
				Position:    position,
				HasValue:    true,
				Subfields:   []SubfieldInfo{},
			}
			enhancedSeg.Fields = append(enhancedSeg.Fields, fieldInfo)
		}
	}

	enhancedSeg.FieldCount = len(enhancedSeg.Fields)
	return enhancedSeg
}

func getBasicHL7Order(basicSegments map[string]BasicSegment) []string {
	standardOrder := []string{
		"MSH", "EVN", "PID", "NK1", "PV1", "PV2", "ROL",
		"OBX", "OBR", "ORC", "RXO", "RXR", "RXC", "RXA", "RXE",
		"DG1", "AL1", "PR1", "GT1", "IN1", "IN2", "IN3",
		"ACC", "UB1", "UB2", "NTE", "DSC",
	}

	var orderedSegments []string
	segmentExists := make(map[string]bool)

	for _, segmentName := range standardOrder {
		if _, exists := basicSegments[segmentName]; exists {
			orderedSegments = append(orderedSegments, segmentName)
			segmentExists[segmentName] = true
		}
	}

	var remainingSegments []string
	for segmentName := range basicSegments {
		if !segmentExists[segmentName] {
			remainingSegments = append(remainingSegments, segmentName)
		}
	}

	sort.Strings(remainingSegments)
	orderedSegments = append(orderedSegments, remainingSegments...)

	return orderedSegments
}

func createMessageTypeInfo(messageType, triggerEvent string) MessageTypeInfo {
	fullName := fmt.Sprintf("%s^%s", messageType, triggerEvent)

	descriptions := map[string]string{
		"ADT^A01": "Admit/visit notification",
		"ADT^A03": "Discharge/end visit",
		"ADT^A04": "Register a patient",
		"ADT^A08": "Update patient information",
		"ORU^R01": "Observation result",
		"ORM^O01": "Order message",
		"MDM^T02": "Medical document management",
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

func extractMessageInfoSimple(rawMessage string) (version, messageType, triggerEvent string) {
	version = "2.5.1"
	messageType = "ADT"
	triggerEvent = "A01"

	// Handle both \r\n (CRLF), \r (CR), and \n (LF) as segment separators
	// Replace all \r\n with \n first, then \r with \n
	normalizedMessage := strings.ReplaceAll(rawMessage, "\r\n", "\n")
	normalizedMessage = strings.ReplaceAll(normalizedMessage, "\r", "\n")

	lines := strings.Split(normalizedMessage, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "MSH|") {
			fields := strings.Split(line, "|")

			// MSH-12 (field index 11) contains the version
			if len(fields) >= 12 {
				if v := strings.TrimSpace(fields[11]); v != "" {
					version = v
				}
			}

			// MSH-9 (field index 8) contains message type^trigger event
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

// Validate required fields and create specific validation errors
func validateRequiredFields(enhancedSegments map[string]EnhancedSegment, schema *RealHL7Schema) []ValidationError {
	var validationErrors []ValidationError

	if schema == nil {
		return validationErrors
	}

	for segmentName, enhancedSegment := range enhancedSegments {
		if schemaDef, hasSchema := schema.Segments[segmentName]; hasSchema {
			// Check each schema field for required status
			for fieldKey, fieldDef := range schemaDef.Fields {
				if fieldDef.Usage == "R" { // Required field
					// Find this field in the enhanced segment
					fieldFound := false
					fieldHasValue := false

					for _, field := range enhancedSegment.Fields {
						if field.Key == fieldKey {
							fieldFound = true
							fieldHasValue = field.HasValue
							break
						}
					}

					if !fieldFound {
						validationErrors = append(validationErrors, ValidationError{
							Severity:   SeverityError,
							Code:       ErrorCodeMissingRequiredField,
							Message:    fmt.Sprintf("Required field %s (%s) is missing from segment %s", fieldKey, fieldDef.Name, segmentName),
							Segment:    segmentName,
							Field:      fieldKey,
							Position:   fieldDef.Position,
							Suggestion: fmt.Sprintf("Add %s field to %s segment", fieldDef.Name, segmentName),
							RuleId:     fmt.Sprintf("REQ_%s_%d", segmentName, fieldDef.Position),
						})
					} else if !fieldHasValue {
						validationErrors = append(validationErrors, ValidationError{
							Severity:   SeverityWarning,
							Code:       "EMPTY_REQUIRED_FIELD",
							Message:    fmt.Sprintf("Required field %s (%s) is empty in segment %s", fieldKey, fieldDef.Name, segmentName),
							Segment:    segmentName,
							Field:      fieldKey,
							Position:   fieldDef.Position,
							Suggestion: fmt.Sprintf("Provide a value for %s", fieldDef.Name),
							RuleId:     fmt.Sprintf("EMPTY_%s_%d", segmentName, fieldDef.Position),
						})
					}
				}
			}
		}
	}

	return validationErrors
}
