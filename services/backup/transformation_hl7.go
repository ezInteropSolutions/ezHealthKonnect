// services/transformation_hl7.go
// HL7 Transformation Service for Universal Interface Engine
//
// 🎯 PURPOSE: Comprehensive HL7 message parsing, validation, and transformation
// Supports all HL7 message types with advanced parsing, segment analysis, and format conversion
package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"
	"ezhealthkonnect/hl7"
)

// =====================================
// HL7 TRANSFORMATION SERVICE
// =====================================

// HL7TransformationService handles HL7 message processing
type HL7TransformationService struct {
	db             *sql.DB
	schemaLoader   *hl7.SchemaLoader
	validationRules map[string]ValidationRule
	parsingCache   map[string]*ParsedHL7Message
	performanceMetrics HL7PerformanceMetrics
}

// HL7PerformanceMetrics tracks service performance
type HL7PerformanceMetrics struct {
	MessagesProcessed    int64         `json:"messagesProcessed"`
	AverageParseTime     time.Duration `json:"averageParseTime"`
	AverageValidateTime  time.Duration `json:"averageValidateTime"`
	CacheHitRatio        float64       `json:"cacheHitRatio"`
	ErrorRate            float64       `json:"errorRate"`
	ThroughputPerSecond  float64       `json:"throughputPerSecond"`
}

// HL7TransformRequest defines transformation request structure
type HL7TransformRequest struct {
	MessageID        string                 `json:"messageId"`
	RawHL7Message    []byte                 `json:"rawHL7Message"`
	SourceEncoding   string                 `json:"sourceEncoding,omitempty"`
	ValidationType   string                 `json:"validationType,omitempty"`   // STRICT, LENIENT, NONE
	ParseMode        string                 `json:"parseMode,omitempty"`        // FULL, SEGMENTS_ONLY, HEADERS_ONLY
	OutputFormat     string                 `json:"outputFormat,omitempty"`     // JSON, XML, FHIR_READY
	IncludeMetadata  bool                   `json:"includeMetadata"`
	PreserveBinary   bool                   `json:"preserveBinary"`
	CustomRules      map[string]interface{} `json:"customRules,omitempty"`
	TransformationOptions map[string]interface{} `json:"transformationOptions,omitempty"`
}

// HL7TransformResponse defines transformation response structure
type HL7TransformResponse struct {
	Success              bool                   `json:"success"`
	MessageID            string                 `json:"messageId"`
	MessageType          string                 `json:"messageType"`
	MessageVersion       string                 `json:"messageVersion"`
	SourceSystem         string                 `json:"sourceSystem"`
	ParsedMessage        *ParsedHL7Message      `json:"parsedMessage"`
	ValidationResults    []HL7ValidationResult  `json:"validationResults"`
	TransformationSteps  []TransformationStep   `json:"transformationSteps"`
	OutputContent        map[string]interface{} `json:"outputContent"`
	ProcessingMetrics    ProcessingMetrics      `json:"processingMetrics"`
	Warnings             []string               `json:"warnings"`
	Errors               []string               `json:"errors"`
	QualityScore         float64                `json:"qualityScore"`
	RecommendedActions   []string               `json:"recommendedActions"`
}

// ParsedHL7Message represents comprehensive HL7 message structure
type ParsedHL7Message struct {
	MessageHeader       HL7MessageHeader         `json:"messageHeader"`
	Segments            []HL7Segment             `json:"segments"`
	SegmentGroups       map[string][]HL7Segment  `json:"segmentGroups"`
	MessageStructure    HL7MessageStructure      `json:"messageStructure"`
	FieldDictionary     map[string]HL7Field      `json:"fieldDictionary"`
	EncodingCharacters  HL7EncodingCharacters    `json:"encodingCharacters"`
	MessageStatistics   HL7MessageStatistics     `json:"messageStatistics"`
	ProcessingMetadata  map[string]interface{}   `json:"processingMetadata"`
}

// HL7MessageHeader contains MSH segment information
type HL7MessageHeader struct {
	SendingApplication   string    `json:"sendingApplication"`
	SendingFacility     string    `json:"sendingFacility"`
	ReceivingApplication string    `json:"receivingApplication"`
	ReceivingFacility   string    `json:"receivingFacility"`
	MessageDateTime     time.Time `json:"messageDateTime"`
	Security            string    `json:"security,omitempty"`
	MessageType         HL7MessageType `json:"messageType"`
	MessageControlID    string    `json:"messageControlId"`
	ProcessingID        string    `json:"processingId"`
	VersionID           string    `json:"versionId"`
	SequenceNumber      string    `json:"sequenceNumber,omitempty"`
	ContinuationPointer string    `json:"continuationPointer,omitempty"`
	AcceptAckType       string    `json:"acceptAckType,omitempty"`
	ApplicationAckType  string    `json:"applicationAckType,omitempty"`
	CountryCode         string    `json:"countryCode,omitempty"`
	CharacterSet        []string  `json:"characterSet,omitempty"`
	PrincipalLanguage   string    `json:"principalLanguage,omitempty"`
}

// HL7MessageType represents message type and trigger event
type HL7MessageType struct {
	MessageCode     string `json:"messageCode"`     // ADT, ORU, etc.
	TriggerEvent    string `json:"triggerEvent"`    // A01, R01, etc.
	MessageStructure string `json:"messageStructure"` // ADT_A01, ORU_R01, etc.
	Description     string `json:"description"`
}

// HL7Segment represents a parsed HL7 segment
type HL7Segment struct {
	Name            string                 `json:"name"`
	Position        int                    `json:"position"`
	RawContent      string                 `json:"rawContent"`
	Fields          []HL7Field             `json:"fields"`
	FieldCount      int                    `json:"fieldCount"`
	IsOptional      bool                   `json:"isOptional"`
	Cardinality     string                 `json:"cardinality"`
	SegmentMetadata map[string]interface{} `json:"segmentMetadata"`
	ValidationStatus string                `json:"validationStatus"`
	ValidationIssues []ValidationIssue     `json:"validationIssues"`
}

// HL7Field represents a field within a segment
type HL7Field struct {
	Position        int                    `json:"position"`
	Name            string                 `json:"name"`
	Value           string                 `json:"value"`
	Components      []HL7Component         `json:"components"`
	Repetitions     []HL7Repetition        `json:"repetitions"`
	DataType        string                 `json:"dataType"`
	MaxLength       int                    `json:"maxLength"`
	IsRequired      bool                   `json:"isRequired"`
	Table           string                 `json:"table,omitempty"`
	Description     string                 `json:"description"`
	FieldMetadata   map[string]interface{} `json:"fieldMetadata"`
	ValidationStatus string                `json:"validationStatus"`
	ValidationIssues []ValidationIssue     `json:"validationIssues"`
}

// HL7Component represents components within a field
type HL7Component struct {
	Position      int                    `json:"position"`
	Name          string                 `json:"name"`
	Value         string                 `json:"value"`
	SubComponents []HL7SubComponent      `json:"subComponents"`
	DataType      string                 `json:"dataType"`
	Description   string                 `json:"description"`
	Metadata      map[string]interface{} `json:"metadata"`
}

// HL7SubComponent represents sub-components
type HL7SubComponent struct {
	Position    int    `json:"position"`
	Name        string `json:"name"`
	Value       string `json:"value"`
	DataType    string `json:"dataType"`
	Description string `json:"description"`
}

// HL7Repetition represents field repetitions
type HL7Repetition struct {
	Position   int            `json:"position"`
	Components []HL7Component `json:"components"`
	Value      string         `json:"value"`
}

// HL7MessageStructure defines expected message structure
type HL7MessageStructure struct {
	MessageType      string                      `json:"messageType"`
	Version          string                      `json:"version"`
	RequiredSegments []string                    `json:"requiredSegments"`
	OptionalSegments []string                    `json:"optionalSegments"`
	SegmentGroups    map[string]HL7SegmentGroup  `json:"segmentGroups"`
	MaxLength        int                         `json:"maxLength"`
	Conformance      string                      `json:"conformance"`
	Extensions       map[string]interface{}      `json:"extensions"`
}

// HL7SegmentGroup represents grouped segments
type HL7SegmentGroup struct {
	Name        string   `json:"name"`
	Segments    []string `json:"segments"`
	Cardinality string   `json:"cardinality"`
	IsOptional  bool     `json:"isOptional"`
	Description string   `json:"description"`
}

// HL7EncodingCharacters contains HL7 encoding characters
type HL7EncodingCharacters struct {
	FieldSeparator      string `json:"fieldSeparator"`
	ComponentSeparator  string `json:"componentSeparator"`
	RepetitionSeparator string `json:"repetitionSeparator"`
	EscapeCharacter     string `json:"escapeCharacter"`
	SubComponentSeparator string `json:"subComponentSeparator"`
}

// HL7MessageStatistics contains message analysis
type HL7MessageStatistics struct {
	TotalSegments       int     `json:"totalSegments"`
	TotalFields         int     `json:"totalFields"`
	TotalComponents     int     `json:"totalComponents"`
	MessageSize         int64   `json:"messageSize"`
	CompressionRatio    float64 `json:"compressionRatio"`
	ComplexityScore     float64 `json:"complexityScore"`
	DataDensity         float64 `json:"dataDensity"`
	SegmentDistribution map[string]int `json:"segmentDistribution"`
}

// ValidationRule defines validation criteria
type ValidationRule struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Type        string                 `json:"type"`        // REQUIRED, FORMAT, RANGE, CUSTOM
	Severity    string                 `json:"severity"`    // ERROR, WARNING, INFO
	Scope       string                 `json:"scope"`       // MESSAGE, SEGMENT, FIELD, COMPONENT
	Target      string                 `json:"target"`      // Target path (MSH, PID.3, etc.)
	Rule        string                 `json:"rule"`        // Rule expression
	Message     string                 `json:"message"`     // Validation message
	Enabled     bool                   `json:"enabled"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// HL7ValidationResult contains validation outcome
type HL7ValidationResult struct {
	RuleID      string            `json:"ruleId"`
	RuleName    string            `json:"ruleName"`
	Severity    string            `json:"severity"`
	Passed      bool              `json:"passed"`
	Message     string            `json:"message"`
	Location    string            `json:"location"`
	Value       string            `json:"value,omitempty"`
	Expected    string            `json:"expected,omitempty"`
	Suggestion  string            `json:"suggestion,omitempty"`
	Impact      string            `json:"impact"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// TransformationStep tracks individual transformation steps
type TransformationStep struct {
	StepID       string                 `json:"stepId"`
	StepName     string                 `json:"stepName"`
	InputType    string                 `json:"inputType"`
	OutputType   string                 `json:"outputType"`
	StartTime    time.Time              `json:"startTime"`
	EndTime      time.Time              `json:"endTime"`
	Duration     time.Duration          `json:"duration"`
	Success      bool                   `json:"success"`
	InputSize    int64                  `json:"inputSize"`
	OutputSize   int64                  `json:"outputSize"`
	ErrorMessage string                 `json:"errorMessage,omitempty"`
	Metadata     map[string]interface{} `json:"metadata"`
}

// ProcessingMetrics contains performance data
type ProcessingMetrics struct {
	TotalTime       time.Duration `json:"totalTime"`
	ParseTime       time.Duration `json:"parseTime"`
	ValidationTime  time.Duration `json:"validationTime"`
	TransformTime   time.Duration `json:"transformTime"`
	MemoryUsage     int64         `json:"memoryUsage"`
	CacheHits       int           `json:"cacheHits"`
	CacheMisses     int           `json:"cacheMisses"`
}

// =====================================
// SERVICE CONSTRUCTOR AND INITIALIZATION
// =====================================

// NewHL7TransformationService creates a new HL7 transformation service
func NewHL7TransformationService(database *sql.DB) *HL7TransformationService {
	service := &HL7TransformationService{
		db:             database,
		schemaLoader:   hl7.GetSchemaLoader(),
		validationRules: make(map[string]ValidationRule),
		parsingCache:   make(map[string]*ParsedHL7Message),
		performanceMetrics: HL7PerformanceMetrics{},
	}

	// Initialize default validation rules
	service.initializeValidationRules()

	log.Printf("✅ HL7TransformationService initialized")
	return service
}

// initializeValidationRules sets up default validation rules
func (s *HL7TransformationService) initializeValidationRules() {
	defaultRules := []ValidationRule{
		{
			ID:       "MSH_REQUIRED",
			Name:     "MSH Segment Required",
			Type:     "REQUIRED",
			Severity: "ERROR",
			Scope:    "MESSAGE",
			Target:   "MSH",
			Rule:     "exists",
			Message:  "MSH segment is required for all HL7 messages",
			Enabled:  true,
		},
		{
			ID:       "MSH_FIELD_COUNT",
			Name:     "MSH Field Count",
			Type:     "FORMAT",
			Severity: "ERROR",
			Scope:    "SEGMENT",
			Target:   "MSH",
			Rule:     "field_count >= 21",
			Message:  "MSH segment must have at least 21 fields",
			Enabled:  true,
		},
		{
			ID:       "MESSAGE_TYPE_FORMAT",
			Name:     "Message Type Format",
			Type:     "FORMAT",
			Severity: "ERROR",
			Scope:    "FIELD",
			Target:   "MSH.9",
			Rule:     "format: '^[A-Z]{3}\\^[A-Z0-9]{3}$'",
			Message:  "Message type must be in format XXX^YYY",
			Enabled:  true,
		},
		{
			ID:       "CONTROL_ID_UNIQUE",
			Name:     "Control ID Present",
			Type:     "REQUIRED",
			Severity: "WARNING",
			Scope:    "FIELD",
			Target:   "MSH.10",
			Rule:     "not_empty",
			Message:  "Message control ID should not be empty",
			Enabled:  true,
		},
		{
			ID:       "TIMESTAMP_FORMAT",
			Name:     "Timestamp Format",
			Type:     "FORMAT",
			Severity: "WARNING",
			Scope:    "FIELD",
			Target:   "MSH.7",
			Rule:     "format: '^\\d{8}(\\d{6})?$'",
			Message:  "Timestamp should be in YYYYMMDD or YYYYMMDDHHMMSS format",
			Enabled:  true,
		},
	}

	for _, rule := range defaultRules {
		s.validationRules[rule.ID] = rule
	}

	log.Printf("✅ Initialized %d default HL7 validation rules", len(defaultRules))
}

// =====================================
// MAIN TRANSFORMATION INTERFACE
// =====================================

// Transform transforms a UniversalMessage containing HL7 content
func (s *HL7TransformationService) Transform(ctx context.Context, message *UniversalMessage) error {
	// Start transformation tracking
	transformRecord := message.StartTransformation("HL7TransformationService", MessageTypeHL7, MessageTypeJSON)

	startTime := time.Now()
	var outputSize int64 = 0
	var transformError error

	defer func() {
		message.CompleteTransformation(transformError == nil, outputSize, func() string {
			if transformError != nil {
				return transformError.Error()
			}
			return ""
		}())
	}()

	// Update message status
	message.UpdateStatus(StatusParsing, "HL7TransformationService", "Starting HL7 parsing and transformation")

	// Create transformation request
	request := &HL7TransformRequest{
		MessageID:      message.ID,
		RawHL7Message:  message.RawContent,
		SourceEncoding: message.Encoding,
		ValidationType: "STRICT",
		ParseMode:      "FULL",
		OutputFormat:   "JSON",
		IncludeMetadata: true,
		PreserveBinary: false,
		TransformationOptions: map[string]interface{}{
			"preserveOriginalStructure": true,
			"includeValidationResults":  true,
			"generateQualityMetrics":   true,
		},
	}

	// Perform transformation
	response, err := s.TransformHL7Message(ctx, request)
	if err != nil {
		transformError = err
		message.AddError("TRANSFORMATION", "HL7TransformationService", "HL7_TRANSFORM_FAILED",
			"Failed to transform HL7 message", err.Error(), true)
		return err
	}

	if !response.Success {
		transformError = fmt.Errorf("HL7 transformation failed: %v", response.Errors)
		for _, error := range response.Errors {
			message.AddError("TRANSFORMATION", "HL7TransformationService", "HL7_VALIDATION_ERROR",
				"HL7 validation error", error, false)
		}
		return transformError
	}

	// Store parsed content
	message.ParsedContent = response.OutputContent
	message.SubType = response.MessageType

	// Add transformed content
	outputBytes, _ := json.Marshal(response.OutputContent)
	outputSize = int64(len(outputBytes))
	message.AddTransformedContent(MessageTypeJSON, outputBytes, transformRecord.ID)

	// Add validation warnings
	for _, warning := range response.Warnings {
		message.AddWarning("VALIDATION", "HL7TransformationService", "HL7_VALIDATION_WARNING",
			warning, "Data quality impact")
	}

	// Update status to parsed
	message.UpdateStatus(StatusParsed, "HL7TransformationService",
		fmt.Sprintf("HL7 message parsed successfully (Type: %s, Quality: %.2f)",
			response.MessageType, response.QualityScore))

	// Update transformation metadata
	if transformRecord != nil {
		transformRecord.Metadata["messageType"] = response.MessageType
		transformRecord.Metadata["validationResults"] = len(response.ValidationResults)
		transformRecord.Metadata["qualityScore"] = response.QualityScore
		transformRecord.Metadata["segmentCount"] = len(response.ParsedMessage.Segments)
	}

	log.Printf("✅ HL7 transformation completed for message %s (Type: %s, Duration: %v)",
		message.ID, response.MessageType, time.Since(startTime))

	return nil
}

// =====================================
// CORE HL7 TRANSFORMATION LOGIC
// =====================================

// TransformHL7Message performs complete HL7 message transformation
func (s *HL7TransformationService) TransformHL7Message(ctx context.Context, request *HL7TransformRequest) (*HL7TransformResponse, error) {
	startTime := time.Now()

	response := &HL7TransformResponse{
		Success:              false,
		MessageID:            request.MessageID,
		ValidationResults:    []HL7ValidationResult{},
		TransformationSteps:  []TransformationStep{},
		Warnings:             []string{},
		Errors:               []string{},
		ProcessingMetrics:    ProcessingMetrics{},
	}

	// Step 1: Parse HL7 message
	parseStep := s.startTransformationStep("PARSE_HL7", "RAW", "PARSED")
	parsedMessage, parseErr := s.parseHL7Message(request.RawHL7Message, request.SourceEncoding)
	s.completeTransformationStep(&parseStep, parseErr, len(request.RawHL7Message), func() int {
		if parsedMessage != nil {
			return len(parsedMessage.Segments)
		}
		return 0
	}())
	response.TransformationSteps = append(response.TransformationSteps, parseStep)

	if parseErr != nil {
		response.Errors = append(response.Errors, fmt.Sprintf("Parse error: %v", parseErr))
		return response, parseErr
	}

	response.ParsedMessage = parsedMessage
	response.MessageType = parsedMessage.MessageHeader.MessageType.MessageStructure
	response.MessageVersion = parsedMessage.MessageHeader.VersionID
	response.SourceSystem = parsedMessage.MessageHeader.SendingApplication

	// Step 2: Validation (if requested)
	if request.ValidationType != "NONE" {
		validationStep := s.startTransformationStep("VALIDATE_HL7", "PARSED", "VALIDATED")
		validationResults, validationErr := s.validateParsedMessage(parsedMessage, request.ValidationType)
		s.completeTransformationStep(&validationStep, validationErr, 0, len(validationResults))
		response.TransformationSteps = append(response.TransformationSteps, validationStep)

		response.ValidationResults = validationResults

		// Count errors and warnings
		errorCount := 0
		warningCount := 0
		for _, result := range validationResults {
			if !result.Passed {
				if result.Severity == "ERROR" {
					errorCount++
					response.Errors = append(response.Errors,
						fmt.Sprintf("Validation error at %s: %s", result.Location, result.Message))
				} else if result.Severity == "WARNING" {
					warningCount++
					response.Warnings = append(response.Warnings,
						fmt.Sprintf("Validation warning at %s: %s", result.Location, result.Message))
				}
			}
		}

		// Calculate quality score
		totalValidations := len(validationResults)
		if totalValidations > 0 {
			passed := totalValidations - errorCount - warningCount
			response.QualityScore = float64(passed) / float64(totalValidations) * 100.0
		} else {
			response.QualityScore = 100.0
		}

		// Fail if strict validation and errors found
		if request.ValidationType == "STRICT" && errorCount > 0 {
			return response, fmt.Errorf("validation failed with %d errors", errorCount)
		}
	}

	// Step 3: Transform to output format
	transformStep := s.startTransformationStep("TRANSFORM_OUTPUT", "PARSED", request.OutputFormat)
	outputContent, transformErr := s.transformToOutputFormat(parsedMessage, request.OutputFormat, request.IncludeMetadata)
	outputSize := 0
	if outputContent != nil {
		if outputBytes, err := json.Marshal(outputContent); err == nil {
			outputSize = len(outputBytes)
		}
	}
	s.completeTransformationStep(&transformStep, transformErr, 0, outputSize)
	response.TransformationSteps = append(response.TransformationSteps, transformStep)

	if transformErr != nil {
		response.Errors = append(response.Errors, fmt.Sprintf("Transform error: %v", transformErr))
		return response, transformErr
	}

	response.OutputContent = outputContent
	response.Success = true

	// Step 4: Calculate processing metrics
	response.ProcessingMetrics = ProcessingMetrics{
		TotalTime:      time.Since(startTime),
		ParseTime:      parseStep.Duration,
		ValidationTime: func() time.Duration {
			if len(response.TransformationSteps) > 1 {
				return response.TransformationSteps[1].Duration
			}
			return 0
		}(),
		TransformTime: transformStep.Duration,
		MemoryUsage:   int64(len(request.RawHL7Message) + outputSize),
	}

	// Update service metrics
	s.updatePerformanceMetrics(response.ProcessingMetrics)

	log.Printf("✅ HL7 transformation completed (Message: %s, Type: %s, Quality: %.2f%%)",
		response.MessageID, response.MessageType, response.QualityScore)

	return response, nil
}

// =====================================
// HL7 PARSING LOGIC
// =====================================

// parseHL7Message parses raw HL7 content into structured format
func (s *HL7TransformationService) parseHL7Message(rawMessage []byte, encoding string) (*ParsedHL7Message, error) {
	if len(rawMessage) == 0 {
		return nil, fmt.Errorf("empty HL7 message")
	}

	messageStr := string(rawMessage)

	// Parse encoding characters from MSH segment
	encodingChars, err := s.parseEncodingCharacters(messageStr)
	if err != nil {
		return nil, fmt.Errorf("failed to parse encoding characters: %v", err)
	}

	// Split into segments
	segments := s.splitIntoSegments(messageStr)
	if len(segments) == 0 {
		return nil, fmt.Errorf("no segments found in HL7 message")
	}

	// Parse MSH segment first
	mshSegment := segments[0]
	if !strings.HasPrefix(mshSegment, "MSH") {
		return nil, fmt.Errorf("first segment must be MSH, found: %s", mshSegment[:3])
	}

	messageHeader, err := s.parseMessageHeader(mshSegment, encodingChars)
	if err != nil {
		return nil, fmt.Errorf("failed to parse message header: %v", err)
	}

	// Parse all segments
	parsedSegments := make([]HL7Segment, 0, len(segments))
	segmentGroups := make(map[string][]HL7Segment)
	fieldDictionary := make(map[string]HL7Field)

	for i, segmentStr := range segments {
		segment, err := s.parseSegment(segmentStr, i, encodingChars)
		if err != nil {
			log.Printf("⚠️ Warning: Failed to parse segment %d (%s): %v", i, segmentStr[:3], err)
			continue
		}

		parsedSegments = append(parsedSegments, *segment)

		// Group segments by type
		segmentName := segment.Name
		if _, exists := segmentGroups[segmentName]; !exists {
			segmentGroups[segmentName] = []HL7Segment{}
		}
		segmentGroups[segmentName] = append(segmentGroups[segmentName], *segment)

		// Build field dictionary
		for _, field := range segment.Fields {
			key := fmt.Sprintf("%s.%d", segmentName, field.Position)
			fieldDictionary[key] = field
		}
	}

	// Build message structure
	messageStructure := s.buildMessageStructure(messageHeader.MessageType, parsedSegments)

	// Calculate statistics
	statistics := s.calculateMessageStatistics(parsedSegments, len(rawMessage))

	parsedMessage := &ParsedHL7Message{
		MessageHeader:      *messageHeader,
		Segments:           parsedSegments,
		SegmentGroups:      segmentGroups,
		MessageStructure:   *messageStructure,
		FieldDictionary:    fieldDictionary,
		EncodingCharacters: *encodingChars,
		MessageStatistics:  *statistics,
		ProcessingMetadata: map[string]interface{}{
			"parseTime":    time.Now(),
			"encoding":     encoding,
			"messageSize":  len(rawMessage),
			"segmentCount": len(parsedSegments),
		},
	}

	return parsedMessage, nil
}

// parseEncodingCharacters extracts HL7 encoding characters from MSH segment
func (s *HL7TransformationService) parseEncodingCharacters(messageStr string) (*HL7EncodingCharacters, error) {
	if len(messageStr) < 8 {
		return nil, fmt.Errorf("message too short to contain MSH segment")
	}

	if !strings.HasPrefix(messageStr, "MSH") {
		return nil, fmt.Errorf("message must start with MSH segment")
	}

	// Standard HL7 encoding: MSH|^~\&|
	if len(messageStr) < 8 {
		return nil, fmt.Errorf("MSH segment too short")
	}

	return &HL7EncodingCharacters{
		FieldSeparator:        string(messageStr[3]),  // |
		ComponentSeparator:    string(messageStr[4]),  // ^
		RepetitionSeparator:   string(messageStr[5]),  // ~
		EscapeCharacter:       string(messageStr[6]),  // \
		SubComponentSeparator: string(messageStr[7]),  // &
	}, nil
}

// splitIntoSegments splits HL7 message into individual segments
func (s *HL7TransformationService) splitIntoSegments(messageStr string) []string {
	// Handle different line endings
	messageStr = strings.ReplaceAll(messageStr, "\r\n", "\n")
	messageStr = strings.ReplaceAll(messageStr, "\r", "\n")

	segments := strings.Split(messageStr, "\n")

	// Filter out empty segments
	validSegments := make([]string, 0, len(segments))
	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if len(segment) > 3 && isValidSegmentName(segment[:3]) {
			validSegments = append(validSegments, segment)
		}
	}

	return validSegments
}

// isValidSegmentName checks if a string is a valid HL7 segment name
func isValidSegmentName(name string) bool {
	if len(name) != 3 {
		return false
	}

	// Common HL7 segment names
	validSegments := map[string]bool{
		"MSH": true, "EVN": true, "PID": true, "PD1": true, "NK1": true,
		"PV1": true, "PV2": true, "OBX": true, "OBR": true, "ORC": true,
		"SPM": true, "SAC": true, "NTE": true, "AL1": true, "DG1": true,
		"PR1": true, "GT1": true, "IN1": true, "IN2": true, "IN3": true,
		"ACC": true, "UB1": true, "UB2": true, "MRG": true, "PDA": true,
		"QRD": true, "QRF": true, "DSC": true, "DSP": true, "ERR": true,
		"MSA": true, "QAK": true, "QPD": true, "RDF": true, "RDT": true,
		"FHS": true, "BHS": true, "BTS": true, "FTS": true,
	}

	return validSegments[name]
}

// parseMessageHeader parses the MSH segment
func (s *HL7TransformationService) parseMessageHeader(mshSegment string, encodingChars *HL7EncodingCharacters) (*HL7MessageHeader, error) {
	fields := strings.Split(mshSegment, encodingChars.FieldSeparator)
	if len(fields) < 12 {
		return nil, fmt.Errorf("MSH segment must have at least 12 fields, found %d", len(fields))
	}

	header := &HL7MessageHeader{}

	// Parse basic fields
	if len(fields) > 2 {
		header.SendingApplication = fields[2]
	}
	if len(fields) > 3 {
		header.SendingFacility = fields[3]
	}
	if len(fields) > 4 {
		header.ReceivingApplication = fields[4]
	}
	if len(fields) > 5 {
		header.ReceivingFacility = fields[5]
	}
	if len(fields) > 6 {
		if timestamp, err := s.parseHL7Timestamp(fields[6]); err == nil {
			header.MessageDateTime = timestamp
		}
	}
	if len(fields) > 7 {
		header.Security = fields[7]
	}
	if len(fields) > 8 {
		messageType, err := s.parseMessageType(fields[8], encodingChars)
		if err != nil {
			return nil, fmt.Errorf("failed to parse message type: %v", err)
		}
		header.MessageType = *messageType
	}
	if len(fields) > 9 {
		header.MessageControlID = fields[9]
	}
	if len(fields) > 10 {
		header.ProcessingID = fields[10]
	}
	if len(fields) > 11 {
		header.VersionID = fields[11]
	}

	// Parse optional fields
	if len(fields) > 12 {
		header.SequenceNumber = fields[12]
	}
	if len(fields) > 13 {
		header.ContinuationPointer = fields[13]
	}
	if len(fields) > 14 {
		header.AcceptAckType = fields[14]
	}
	if len(fields) > 15 {
		header.ApplicationAckType = fields[15]
	}
	if len(fields) > 16 {
		header.CountryCode = fields[16]
	}
	if len(fields) > 17 && fields[17] != "" {
		header.CharacterSet = strings.Split(fields[17], encodingChars.RepetitionSeparator)
	}
	if len(fields) > 18 {
		header.PrincipalLanguage = fields[18]
	}

	return header, nil
}

// parseMessageType parses the message type field (MSH.9)
func (s *HL7TransformationService) parseMessageType(messageTypeField string, encodingChars *HL7EncodingCharacters) (*HL7MessageType, error) {
	components := strings.Split(messageTypeField, encodingChars.ComponentSeparator)
	if len(components) < 2 {
		return nil, fmt.Errorf("message type must have at least 2 components")
	}

	messageType := &HL7MessageType{
		MessageCode:  components[0],
		TriggerEvent: components[1],
	}

	if len(components) > 2 {
		messageType.MessageStructure = components[2]
	} else {
		messageType.MessageStructure = fmt.Sprintf("%s_%s", messageType.MessageCode, messageType.TriggerEvent)
	}

	// Get description from known message types
	messageType.Description = s.getMessageTypeDescription(messageType.MessageCode, messageType.TriggerEvent)

	return messageType, nil
}

// parseHL7Timestamp parses HL7 timestamp format
func (s *HL7TransformationService) parseHL7Timestamp(timestampStr string) (time.Time, error) {
	if timestampStr == "" {
		return time.Time{}, fmt.Errorf("empty timestamp")
	}

	// HL7 timestamp formats: YYYY[MM[DD[HH[MM[SS[.S[S[S[S]]]]]]]]]
	formats := []string{
		"20060102150405",    // YYYYMMDDHHMMSS
		"200601021504",      // YYYYMMDDHHMM
		"2006010215",        // YYYYMMDDHH
		"20060102",          // YYYYMMDD
		"200601",            // YYYYMM
		"2006",              // YYYY
		"20060102150405.000", // With milliseconds
	}

	for _, format := range formats {
		if len(timestampStr) == len(format) {
			if t, err := time.Parse(format, timestampStr); err == nil {
				return t, nil
			}
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse timestamp: %s", timestampStr)
}

// getMessageTypeDescription returns description for known message types
func (s *HL7TransformationService) getMessageTypeDescription(messageCode, triggerEvent string) string {
	descriptions := map[string]string{
		"ADT_A01": "Admit/Visit Notification",
		"ADT_A02": "Transfer a Patient",
		"ADT_A03": "Discharge/End Visit",
		"ADT_A04": "Register a Patient",
		"ADT_A05": "Pre-admit a Patient",
		"ADT_A08": "Update Patient Information",
		"ORU_R01": "Observation Result",
		"ORM_O01": "Order Message",
		"ACK":     "General Acknowledgment",
		"QRY_A19": "Patient Query",
		"SIU_S12": "Schedule Information - New Appointment Booking",
		"MDM_T02": "Medical Document Management - Original Document Notification",
	}

	key := fmt.Sprintf("%s_%s", messageCode, triggerEvent)
	if desc, exists := descriptions[key]; exists {
		return desc
	}

	// Generic descriptions
	switch messageCode {
	case "ADT":
		return "Admission, Discharge, Transfer"
	case "ORU":
		return "Observation Result"
	case "ORM":
		return "Order Message"
	case "SIU":
		return "Schedule Information"
	case "MDM":
		return "Medical Document Management"
	default:
		return fmt.Sprintf("%s Message", messageCode)
	}
}

// Continue with remaining methods...
// [This is part 1 of the HL7 transformation service - continuing with remaining methods in next part]