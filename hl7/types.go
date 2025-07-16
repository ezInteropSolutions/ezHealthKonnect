// FILE: types.go
// Complete HL7 type definitions with ENHANCED sequence support and position validation
package hl7

import "time"

// =====================================
// API REQUEST/RESPONSE TYPES
// =====================================

// ParseRequest represents an incoming HL7 parse request
type ParseRequest struct {
	RawMessage  string `json:"rawMessage" binding:"required"`
	UseEnhanced bool   `json:"useEnhanced"`
	MessageType string `json:"messageType,omitempty"`
}

// ParseResponse represents the response from HL7 parsing
type ParseResponse struct {
	Success bool                   `json:"success"`
	Data    *EnhancedParsedMessage `json:"data,omitempty"`
	Error   string                 `json:"error,omitempty"`
	Meta    *ParseMeta             `json:"meta,omitempty"`
}

// ParseMeta contains metadata about the parsing process
type ParseMeta struct {
	ParsingTime     time.Duration `json:"parsingTime"`
	DictionaryUsed  bool          `json:"dictionaryUsed"`
	ValidationLevel string        `json:"validationLevel"`
	ParserVersion   string        `json:"parserVersion"`
	SchemaUsed      bool          `json:"schemaUsed"`
	ParseMethod     string        `json:"parseMethod"`
	CacheStats      *CacheStats   `json:"cacheStats,omitempty"`
}

type CacheStats struct {
	CacheHits   int    `json:"cacheHits"`
	CacheMisses int    `json:"cacheMisses"`
	LoadErrors  int    `json:"loadErrors"`
	TotalLoads  int    `json:"totalLoads"`
	CacheSize   int    `json:"cacheSize"`
	LastLoaded  string `json:"lastLoaded"`
}

// =====================================
// CORE HL7 PARSING TYPES
// =====================================

// BasicParsedMessage represents a basic HL7 parsing result
type BasicParsedMessage struct {
	Raw         string                  `json:"raw"`
	MessageType string                  `json:"messageType"`
	Segments    map[string]BasicSegment `json:"segments"`
	ParsedAt    string                  `json:"parsedAt"`
}

// BasicSegment represents a basic HL7 segment
type BasicSegment struct {
	Name   string            `json:"name"`
	Fields map[string]string `json:"fields"` // Key format: "SEGMENT.POSITION" (e.g., "PID.5")
	Raw    string            `json:"raw"`
}

// ✅ UPDATED: EnhancedParsedMessage with map-based segments (keyed by segment name)
type EnhancedParsedMessage struct {
	Raw              string                     `json:"raw"`
	Success          bool                       `json:"success"`
	Error            string                     `json:"error,omitempty"`
	Version          string                     `json:"version"`
	MessageType      MessageTypeInfo            `json:"messageType"`
	BasicSegments    map[string]BasicSegment    `json:"basicSegments,omitempty"`
	EnhancedSegments map[string]EnhancedSegment `json:"enhancedSegments"` // ✅ FIXED: Map keyed by segment name
	SegmentOrder     []string                   `json:"segmentOrder"`     // ✅ NEW: Maintain order separately
	ParsedAt         string                     `json:"parsedAt"`
	DictionaryUsed   bool                       `json:"dictionaryUsed"`
	SchemaLoaded     bool                       `json:"schemaLoaded"`
	ValidationErrors []ValidationError          `json:"validationErrors,omitempty"`
}

// MessageTypeInfo contains information about the HL7 message type
type MessageTypeInfo struct {
	Code        string `json:"code"`        // e.g., "ADT"
	Event       string `json:"event"`       // e.g., "A01"
	Name        string `json:"name"`        // e.g., "ADT^A01"
	Description string `json:"description"` // e.g., "Admit/visit notification"
	Structure   string `json:"structure"`   // Message structure definition
}

// ✅ UPDATED: EnhancedSegment with sequence support and position validation
type EnhancedSegment struct {
	Key              string      `json:"key"` // Segment key (MSH, PID, etc.)
	Raw              string      `json:"raw"`
	Name             string      `json:"name"`
	Description      string      `json:"description"`
	Purpose          string      `json:"purpose"`
	Fields           []FieldInfo `json:"fields"` // Array maintains order, sorted by position
	FieldCount       int         `json:"fieldCount"`
	DictionarySource string      `json:"dictionarySource"`
	Required         bool        `json:"required"`
	Repeating        bool        `json:"repeating"`
	Sequence         int         `json:"sequence,omitempty"` // ✅ Segment sequence for ordering
}

// ✅ ENHANCED: FieldInfo with strict position validation and sequence support
type FieldInfo struct {
	Key          string         `json:"key"`         // Field key format: "SEGMENT.POSITION" (e.g., "PID.5")
	Value        string         `json:"value"`       // Actual field value from HL7 message
	Name         string         `json:"name"`        // Human-readable field name
	Description  string         `json:"description"` // Detailed field description
	DataType     string         `json:"dataType"`    // HL7 data type (XPN, ST, TS, etc.)
	Optionality  string         `json:"optionality"` // R=Required, O=Optional, C=Conditional
	Cardinality  string         `json:"cardinality"` // e.g., "[0..1]", "[1..1]", "[0..*]"
	Position     int            `json:"position"`    // ✅ HL7 field position (1-based, matches HL7 standard)
	HasValue     bool           `json:"hasValue"`    // True if field contains data
	Length       int            `json:"length,omitempty"`
	TableId      string         `json:"tableId,omitempty"`
	Subfields    []SubfieldInfo `json:"subfields,omitempty"` // Array maintains component order
	DefaultValue string         `json:"defaultValue,omitempty"`
	Sequence     int            `json:"sequence,omitempty"` // ✅ Field sequence for array ordering (0-based)
}

// ✅ ENHANCED: SubfieldInfo with position validation and sequence support
type SubfieldInfo struct {
	Key         string `json:"key"`         // Component key format: "SEGMENT.FIELD.COMPONENT" (e.g., "PID.5.1")
	Name        string `json:"name"`        // Component name (e.g., "Family Name")
	DataType    string `json:"dataType"`    // Component data type (e.g., "ST", "FN")
	Usage       string `json:"usage"`       // R, O, C, B (Required, Optional, Conditional, Backward Compatible)
	Length      int    `json:"length"`      // Maximum component length
	Position    int    `json:"position"`    // ✅ Component position within field (1-based)
	Description string `json:"description"` // Component description
	TableId     string `json:"tableId,omitempty"`
	HasValue    bool   `json:"hasValue"`           // True if component contains data
	Value       string `json:"value,omitempty"`    // Actual component value
	Sequence    int    `json:"sequence,omitempty"` // ✅ Component sequence for array ordering (0-based)
}

// ✅ ENHANCED: ValidationError with position-specific information
type ValidationError struct {
	Severity   string `json:"severity"`            // "ERROR", "WARNING", "INFO"
	Code       string `json:"code"`                // Error code (e.g., "POSITION_MISMATCH", "MISSING_REQUIRED_FIELD")
	Message    string `json:"message"`             // Human-readable error message
	Segment    string `json:"segment"`             // Segment where error occurred
	Field      string `json:"field"`               // Field where error occurred (format: "SEGMENT.POSITION")
	Component  string `json:"component,omitempty"` // Component where error occurred (format: "SEGMENT.FIELD.COMPONENT")
	Position   int    `json:"position,omitempty"`  // Position where error occurred
	Suggestion string `json:"suggestion"`          // Suggested fix
	RuleId     string `json:"ruleId,omitempty"`    // Validation rule identifier
}

// =====================================
// POSITION VALIDATION TYPES
// =====================================

// ✅ NEW: PositionValidationResult contains results of field position validation
type PositionValidationResult struct {
	Valid              bool                     `json:"valid"`
	Errors             []ValidationError        `json:"errors"`
	Warnings           []ValidationError        `json:"warnings"`
	PositionMap        map[string]FieldPosition `json:"positionMap"`        // Map of field keys to positions
	DuplicatePositions []DuplicatePosition      `json:"duplicatePositions"` // Fields with duplicate positions
	GapPositions       []int                    `json:"gapPositions"`       // Missing positions in sequence
	MaxPosition        int                      `json:"maxPosition"`        // Highest position found
}

// ✅ NEW: FieldPosition represents field position information
type FieldPosition struct {
	SegmentName string `json:"segmentName"` // Segment name (MSH, PID, etc.)
	Position    int    `json:"position"`    // HL7 field position (1-based)
	FieldKey    string `json:"fieldKey"`    // Field key (SEGMENT.POSITION)
	ArrayIndex  int    `json:"arrayIndex"`  // Array index in fields slice (0-based)
	HasValue    bool   `json:"hasValue"`    // True if field has data
}

// ✅ NEW: DuplicatePosition represents fields with duplicate positions
type DuplicatePosition struct {
	Position  int      `json:"position"`  // Duplicated position
	FieldKeys []string `json:"fieldKeys"` // All field keys with this position
}

// =====================================
// PARSING OPTIONS
// =====================================

// ParsingOptions defines how HL7 messages should be parsed
type ParsingOptions struct {
	UseEnhanced       bool   `json:"useEnhanced"`
	UseDictionary     bool   `json:"useDictionary"`
	ValidationLevel   string `json:"validationLevel"` // "none", "basic", "strict"
	IncludeRaw        bool   `json:"includeRaw"`
	IncludeMetadata   bool   `json:"includeMetadata"`
	CustomSeparators  bool   `json:"customSeparators"`
	PreserveCase      bool   `json:"preserveCase"`
	FallbackToBasic   bool   `json:"fallbackToBasic"`
	PerformanceLog    bool   `json:"performanceLog"`
	ValidatePositions bool   `json:"validatePositions"` // ✅ NEW: Enable position validation
	StrictPositioning bool   `json:"strictPositioning"` // ✅ NEW: Enforce strict HL7 positioning rules
	FillMissingFields bool   `json:"fillMissingFields"` // ✅ NEW: Create placeholder fields for missing positions
}

// =====================================
// SCHEMA VALIDATION TYPES
// =====================================

// ✅ NEW: SchemaValidationResult contains results of schema-based validation
type SchemaValidationResult struct {
	Valid                 bool              `json:"valid"`
	SchemaUsed            string            `json:"schemaUsed"`            // Schema identifier used for validation
	FieldsValidated       int               `json:"fieldsValidated"`       // Number of fields validated
	FieldsMissing         []string          `json:"fieldsMissing"`         // Required fields not found in message
	FieldsExtra           []string          `json:"fieldsExtra"`           // Fields in message not defined in schema
	DataTypeErrors        []ValidationError `json:"dataTypeErrors"`        // Data type validation errors
	CardinalityErrors     []ValidationError `json:"cardinalityErrors"`     // Cardinality validation errors
	TableValidationErrors []ValidationError `json:"tableValidationErrors"` // Table value validation errors
}

// =====================================
// DICTIONARY/LEGACY TYPES
// =====================================

// DictionaryResponse represents response from dictionary service (legacy)
type DictionaryResponse struct {
	Success          bool                       `json:"success"`
	EnhancedSegments map[string]EnhancedSegment `json:"enhancedSegments"`
	MessageStructure MessageStructure           `json:"messageStructure"`
	ValidationRules  []ValidationRule           `json:"validationRules"`
	Error            string                     `json:"error,omitempty"`
}

// MessageStructure defines the expected structure of an HL7 message
type MessageStructure struct {
	MessageType      string              `json:"messageType"`
	RequiredSegments []string            `json:"requiredSegments"`
	OptionalSegments []string            `json:"optionalSegments"`
	SegmentOrder     []string            `json:"segmentOrder"`
	RepeatingGroups  map[string][]string `json:"repeatingGroups"`
}

// ValidationRule represents a validation rule for HL7 messages
type ValidationRule struct {
	RuleID      string `json:"ruleId"`
	Description string `json:"description"`
	Severity    string `json:"severity"`
	Condition   string `json:"condition"`
	Action      string `json:"action"`
}

// =====================================
// UTILITY HELPER TYPES
// =====================================

// ✅ NEW: FieldPositionHelper provides utilities for field position management
type FieldPositionHelper struct {
	SegmentName string
	Fields      []FieldInfo
}

// ✅ NEW: ComponentPositionHelper provides utilities for component position management
type ComponentPositionHelper struct {
	FieldKey   string
	Components []SubfieldInfo
}

// ✅ NEW: MessageStatistics provides statistical information about parsed message
type MessageStatistics struct {
	TotalSegments        int                      `json:"totalSegments"`
	TotalFields          int                      `json:"totalFields"`
	TotalComponents      int                      `json:"totalComponents"`
	FieldsWithValues     int                      `json:"fieldsWithValues"`
	ComponentsWithValues int                      `json:"componentsWithValues"`
	SegmentStatistics    map[string]SegmentStats  `json:"segmentStatistics"`
	PositionStatistics   PositionValidationResult `json:"positionStatistics"`
	ValidationSummary    ValidationSummary        `json:"validationSummary"`
}

// ✅ NEW: SegmentStats provides statistics for individual segments
type SegmentStats struct {
	SegmentName           string `json:"segmentName"`
	FieldCount            int    `json:"fieldCount"`
	FieldsWithValues      int    `json:"fieldsWithValues"`
	ComponentCount        int    `json:"componentCount"`
	MaxPosition           int    `json:"maxPosition"`
	HasPositionGaps       bool   `json:"hasPositionGaps"`
	HasDuplicatePositions bool   `json:"hasDuplicatePositions"`
}

// ✅ NEW: ValidationSummary provides overall validation statistics
type ValidationSummary struct {
	TotalErrors    int            `json:"totalErrors"`
	TotalWarnings  int            `json:"totalWarnings"`
	TotalInfo      int            `json:"totalInfo"`
	ErrorsByType   map[string]int `json:"errorsByType"`
	WarningsByType map[string]int `json:"warningsByType"`
}

// =====================================
// CONSTANTS FOR VALIDATION
// =====================================

// ✅ NEW: Validation error codes
const (
	ErrorCodePositionMismatch     = "POSITION_MISMATCH"
	ErrorCodeDuplicatePosition    = "DUPLICATE_POSITION"
	ErrorCodeMissingRequiredField = "MISSING_REQUIRED_FIELD"
	ErrorCodeInvalidFieldKey      = "INVALID_FIELD_KEY"
	ErrorCodeInvalidComponentKey  = "INVALID_COMPONENT_KEY"
	ErrorCodePositionGap          = "POSITION_GAP"
	ErrorCodeInvalidDataType      = "INVALID_DATA_TYPE"
	ErrorCodeCardinalityViolation = "CARDINALITY_VIOLATION"
	ErrorCodeTableValueInvalid    = "TABLE_VALUE_INVALID"
	ErrorCodeSequenceOutOfOrder   = "SEQUENCE_OUT_OF_ORDER"
)

// ✅ NEW: Validation severity levels
const (
	SeverityError   = "ERROR"
	SeverityWarning = "WARNING"
	SeverityInfo    = "INFO"
)

// ✅ NEW: Field optionality types
const (
	OptionalityRequired     = "R" // Required
	OptionalityOptional     = "O" // Optional
	OptionalityConditional  = "C" // Conditional
	OptionalityBackward     = "B" // Backward Compatible
	OptionalityNotSupported = "X" // Not Supported
)

// ✅ NEW: Common HL7 data types
const (
	DataTypeST  = "ST"  // String
	DataTypeID  = "ID"  // Coded Value
	DataTypeIS  = "IS"  // Coded value for user-defined tables
	DataTypeCE  = "CE"  // Coded Element
	DataTypeTS  = "TS"  // Timestamp
	DataTypeXPN = "XPN" // Extended Person Name
	DataTypeXAD = "XAD" // Extended Address
	DataTypeXTN = "XTN" // Extended Telecommunication Number
	DataTypeCX  = "CX"  // Extended Composite ID
	DataTypeHD  = "HD"  // Hierarchic Designator
	DataTypeMSG = "MSG" // Message Type
	DataTypePT  = "PT"  // Processing Type
	DataTypeVID = "VID" // Version Identifier
	DataTypePL  = "PL"  // Person Location
	DataTypeXCN = "XCN" // Extended Composite Name
	DataTypeFN  = "FN"  // Family Name
	DataTypeSI  = "SI"  // Sequence ID
)

// ✅ NEW: Helper functions for position validation (to be implemented)
type PositionValidator interface {
	ValidateFieldPositions(segments map[string]EnhancedSegment) PositionValidationResult
	ValidateComponentPositions(field FieldInfo) PositionValidationResult
	CheckForDuplicatePositions(fields []FieldInfo) []DuplicatePosition
	FindPositionGaps(fields []FieldInfo) []int
	SuggestPositionFixes(errors []ValidationError) []string
}
