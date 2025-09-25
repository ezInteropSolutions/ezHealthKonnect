// services/transformation_hl7_extended.go
// Extended HL7 Transformation Service Methods
//
// 🎯 PURPOSE: Extended parsing, validation, and utility methods for HL7 transformation
package services

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// =====================================
// SEGMENT PARSING METHODS
// =====================================

// parseSegment parses an individual HL7 segment
func (s *HL7TransformationService) parseSegment(segmentStr string, position int, encodingChars *HL7EncodingCharacters) (*HL7Segment, error) {
	if len(segmentStr) < 3 {
		return nil, fmt.Errorf("segment too short: %s", segmentStr)
	}

	segmentName := segmentStr[:3]
	fields := strings.Split(segmentStr, encodingChars.FieldSeparator)

	segment := &HL7Segment{
		Name:            segmentName,
		Position:        position,
		RawContent:      segmentStr,
		Fields:          []HL7Field{},
		FieldCount:      len(fields) - 1, // Exclude segment name
		IsOptional:      s.isOptionalSegment(segmentName),
		Cardinality:     s.getSegmentCardinality(segmentName),
		SegmentMetadata: make(map[string]interface{}),
		ValidationStatus: "NOT_VALIDATED",
		ValidationIssues: []ValidationIssue{},
	}

	// Parse fields (skip first element which is segment name)
	for i := 1; i < len(fields); i++ {
		fieldStr := fields[i]

		field, err := s.parseField(fieldStr, i, segmentName, encodingChars)
		if err != nil {
			log.Printf("⚠️ Warning: Failed to parse field %s.%d: %v", segmentName, i, err)
			// Create minimal field for error tracking
			field = &HL7Field{
				Position:        i,
				Name:            fmt.Sprintf("Field_%d", i),
				Value:           fieldStr,
				Components:      []HL7Component{},
				Repetitions:     []HL7Repetition{},
				DataType:        "UNKNOWN",
				IsRequired:      false,
				FieldMetadata:   make(map[string]interface{}),
				ValidationStatus: "PARSE_ERROR",
				ValidationIssues: []ValidationIssue{{
					Severity: "ERROR",
					Code:     "PARSE_FAILED",
					Message:  fmt.Sprintf("Failed to parse field: %v", err),
					Path:     fmt.Sprintf("%s.%d", segmentName, i),
				}},
			}
		}

		segment.Fields = append(segment.Fields, *field)
	}

	// Add segment metadata
	segment.SegmentMetadata["parsedAt"] = time.Now()
	segment.SegmentMetadata["fieldCount"] = len(segment.Fields)
	segment.SegmentMetadata["segmentLength"] = len(segmentStr)

	return segment, nil
}

// parseField parses an individual field within a segment
func (s *HL7TransformationService) parseField(fieldStr string, position int, segmentName string, encodingChars *HL7EncodingCharacters) (*HL7Field, error) {
	field := &HL7Field{
		Position:        position,
		Name:            s.getFieldName(segmentName, position),
		Value:           fieldStr,
		Components:      []HL7Component{},
		Repetitions:     []HL7Repetition{},
		DataType:        s.getFieldDataType(segmentName, position),
		MaxLength:       s.getFieldMaxLength(segmentName, position),
		IsRequired:      s.isRequiredField(segmentName, position),
		Table:           s.getFieldTable(segmentName, position),
		Description:     s.getFieldDescription(segmentName, position),
		FieldMetadata:   make(map[string]interface{}),
		ValidationStatus: "NOT_VALIDATED",
		ValidationIssues: []ValidationIssue{},
	}

	if fieldStr == "" {
		return field, nil
	}

	// Parse repetitions (separated by repetition separator)
	repetitions := strings.Split(fieldStr, encodingChars.RepetitionSeparator)

	for repIndex, repetition := range repetitions {
		if repetition == "" {
			continue
		}

		// Parse components within repetition
		components := strings.Split(repetition, encodingChars.ComponentSeparator)

		repObject := HL7Repetition{
			Position:   repIndex + 1,
			Components: []HL7Component{},
			Value:      repetition,
		}

		for compIndex, component := range components {
			if component == "" {
				continue
			}

			// Parse sub-components
			subComponents := strings.Split(component, encodingChars.SubComponentSeparator)

			compObject := HL7Component{
				Position:      compIndex + 1,
				Name:          s.getComponentName(segmentName, position, compIndex+1),
				Value:         component,
				SubComponents: []HL7SubComponent{},
				DataType:      s.getComponentDataType(segmentName, position, compIndex+1),
				Description:   s.getComponentDescription(segmentName, position, compIndex+1),
				Metadata:      make(map[string]interface{}),
			}

			for subCompIndex, subComponent := range subComponents {
				if subComponent == "" {
					continue
				}

				subCompObject := HL7SubComponent{
					Position:    subCompIndex + 1,
					Name:        s.getSubComponentName(segmentName, position, compIndex+1, subCompIndex+1),
					Value:       subComponent,
					DataType:    s.getSubComponentDataType(segmentName, position, compIndex+1, subCompIndex+1),
					Description: s.getSubComponentDescription(segmentName, position, compIndex+1, subCompIndex+1),
				}

				compObject.SubComponents = append(compObject.SubComponents, subCompObject)
			}

			repObject.Components = append(repObject.Components, compObject)
			field.Components = append(field.Components, compObject)
		}

		field.Repetitions = append(field.Repetitions, repObject)
	}

	// Add field metadata
	field.FieldMetadata["parsedAt"] = time.Now()
	field.FieldMetadata["componentCount"] = len(field.Components)
	field.FieldMetadata["repetitionCount"] = len(field.Repetitions)
	field.FieldMetadata["originalValue"] = fieldStr

	return field, nil
}

// =====================================
// VALIDATION METHODS
// =====================================

// validateParsedMessage validates a parsed HL7 message
func (s *HL7TransformationService) validateParsedMessage(parsedMessage *ParsedHL7Message, validationType string) ([]HL7ValidationResult, error) {
	var results []HL7ValidationResult

	// Validate message structure
	structureResults := s.validateMessageStructure(parsedMessage, validationType)
	results = append(results, structureResults...)

	// Validate individual segments
	for _, segment := range parsedMessage.Segments {
		segmentResults := s.validateSegment(&segment, parsedMessage, validationType)
		results = append(results, segmentResults...)
	}

	// Validate cross-segment relationships
	if validationType == "STRICT" {
		relationshipResults := s.validateCrossSegmentRelationships(parsedMessage)
		results = append(results, relationshipResults...)
	}

	// Apply custom validation rules
	customResults := s.applyCustomValidationRules(parsedMessage, validationType)
	results = append(results, customResults...)

	log.Printf("✅ HL7 validation completed: %d validation checks performed", len(results))
	return results, nil
}

// validateMessageStructure validates overall message structure
func (s *HL7TransformationService) validateMessageStructure(parsedMessage *ParsedHL7Message, validationType string) []HL7ValidationResult {
	var results []HL7ValidationResult

	// Rule: MSH segment must be first
	if len(parsedMessage.Segments) == 0 || parsedMessage.Segments[0].Name != "MSH" {
		results = append(results, HL7ValidationResult{
			RuleID:   "MSH_FIRST",
			RuleName: "MSH Segment First",
			Severity: "ERROR",
			Passed:   false,
			Message:  "MSH segment must be the first segment",
			Location: "Message Structure",
			Impact:   "Message cannot be processed without proper header",
		})
	}

	// Rule: Check for required segments based on message type
	requiredSegments := s.getRequiredSegments(parsedMessage.MessageHeader.MessageType.MessageStructure)
	presentSegments := make(map[string]bool)
	for _, segment := range parsedMessage.Segments {
		presentSegments[segment.Name] = true
	}

	for _, required := range requiredSegments {
		if !presentSegments[required] {
			results = append(results, HL7ValidationResult{
				RuleID:   "REQUIRED_SEGMENT",
				RuleName: "Required Segment Present",
				Severity: "ERROR",
				Passed:   false,
				Message:  fmt.Sprintf("Required segment %s is missing", required),
				Location: "Message Structure",
				Expected: required,
				Impact:   "Message may not be fully processable",
			})
		}
	}

	// Rule: Check segment order
	if validationType == "STRICT" {
		orderResults := s.validateSegmentOrder(parsedMessage)
		results = append(results, orderResults...)
	}

	return results
}

// validateSegment validates an individual segment
func (s *HL7TransformationService) validateSegment(segment *HL7Segment, parsedMessage *ParsedHL7Message, validationType string) []HL7ValidationResult {
	var results []HL7ValidationResult

	segmentName := segment.Name
	location := fmt.Sprintf("Segment %s (Position %d)", segmentName, segment.Position)

	// Rule: Check minimum field count
	minFields := s.getMinimumFieldCount(segmentName)
	if len(segment.Fields) < minFields {
		results = append(results, HL7ValidationResult{
			RuleID:   "MIN_FIELD_COUNT",
			RuleName: "Minimum Field Count",
			Severity: "ERROR",
			Passed:   false,
			Message:  fmt.Sprintf("Segment %s must have at least %d fields, found %d", segmentName, minFields, len(segment.Fields)),
			Location: location,
			Expected: fmt.Sprintf("Minimum %d fields", minFields),
			Value:    fmt.Sprintf("%d fields", len(segment.Fields)),
			Impact:   "Segment may be incomplete",
		})
	}

	// Validate individual fields
	for _, field := range segment.Fields {
		fieldResults := s.validateField(&field, segmentName, validationType)
		results = append(results, fieldResults...)
	}

	return results
}

// validateField validates an individual field
func (s *HL7TransformationService) validateField(field *HL7Field, segmentName, validationType string) []HL7ValidationResult {
	var results []HL7ValidationResult

	location := fmt.Sprintf("%s.%d", segmentName, field.Position)

	// Rule: Required field validation
	if field.IsRequired && field.Value == "" {
		results = append(results, HL7ValidationResult{
			RuleID:   "REQUIRED_FIELD",
			RuleName: "Required Field Present",
			Severity: "ERROR",
			Passed:   false,
			Message:  fmt.Sprintf("Required field %s is empty", location),
			Location: location,
			Expected: "Non-empty value",
			Value:    "Empty",
			Impact:   "Required data is missing",
		})
	}

	// Rule: Maximum length validation
	if field.MaxLength > 0 && len(field.Value) > field.MaxLength {
		results = append(results, HL7ValidationResult{
			RuleID:   "MAX_LENGTH",
			RuleName: "Maximum Length Check",
			Severity: "WARNING",
			Passed:   false,
			Message:  fmt.Sprintf("Field %s exceeds maximum length", location),
			Location: location,
			Expected: fmt.Sprintf("Maximum %d characters", field.MaxLength),
			Value:    fmt.Sprintf("%d characters", len(field.Value)),
			Impact:   "Data may be truncated by receiving system",
		})
	}

	// Rule: Data type validation
	if validationType == "STRICT" && field.DataType != "" && field.Value != "" {
		isValid, validationMsg := s.validateDataType(field.Value, field.DataType)
		if !isValid {
			results = append(results, HL7ValidationResult{
				RuleID:   "DATA_TYPE",
				RuleName: "Data Type Validation",
				Severity: "ERROR",
				Passed:   false,
				Message:  fmt.Sprintf("Field %s has invalid data type: %s", location, validationMsg),
				Location: location,
				Expected: field.DataType,
				Value:    field.Value,
				Impact:   "Data format may cause processing errors",
			})
		}
	}

	// Rule: Table validation (for coded values)
	if field.Table != "" && field.Value != "" {
		isValid, suggestion := s.validateTableValue(field.Value, field.Table)
		if !isValid {
			severity := "WARNING"
			if validationType == "STRICT" {
				severity = "ERROR"
			}

			results = append(results, HL7ValidationResult{
				RuleID:   "TABLE_VALUE",
				RuleName: "Table Value Validation",
				Severity: severity,
				Passed:   false,
				Message:  fmt.Sprintf("Field %s contains invalid table value", location),
				Location: location,
				Expected: fmt.Sprintf("Valid value from table %s", field.Table),
				Value:    field.Value,
				Suggestion: suggestion,
				Impact:   "Value may not be recognized by receiving system",
			})
		}
	}

	return results
}

// validateCrossSegmentRelationships validates relationships between segments
func (s *HL7TransformationService) validateCrossSegmentRelationships(parsedMessage *ParsedHL7Message) []HL7ValidationResult {
	var results []HL7ValidationResult

	// Example: PID segment should come before PV1
	pidFound := false
	pv1Position := -1
	pidPosition := -1

	for i, segment := range parsedMessage.Segments {
		if segment.Name == "PID" {
			pidFound = true
			pidPosition = i
		} else if segment.Name == "PV1" {
			pv1Position = i
			if !pidFound {
				results = append(results, HL7ValidationResult{
					RuleID:   "PID_BEFORE_PV1",
					RuleName: "PID Before PV1 Segment Order",
					Severity: "ERROR",
					Passed:   false,
					Message:  "PV1 segment found before PID segment",
					Location: fmt.Sprintf("Segments PID:%d, PV1:%d", pidPosition, pv1Position),
					Impact:   "Patient information must be established before visit information",
				})
			}
		}
	}

	// Add more cross-segment validation rules here...

	return results
}

// applyCustomValidationRules applies user-defined validation rules
func (s *HL7TransformationService) applyCustomValidationRules(parsedMessage *ParsedHL7Message, validationType string) []HL7ValidationResult {
	var results []HL7ValidationResult

	for _, rule := range s.validationRules {
		if !rule.Enabled {
			continue
		}

		// Apply rule based on scope
		switch rule.Scope {
		case "MESSAGE":
			result := s.applyMessageRule(rule, parsedMessage)
			if result != nil {
				results = append(results, *result)
			}
		case "SEGMENT":
			for _, segment := range parsedMessage.Segments {
				if s.ruleTargetMatches(rule.Target, segment.Name, "", "") {
					result := s.applySegmentRule(rule, &segment)
					if result != nil {
						results = append(results, *result)
					}
				}
			}
		case "FIELD":
			for _, segment := range parsedMessage.Segments {
				for _, field := range segment.Fields {
					target := fmt.Sprintf("%s.%d", segment.Name, field.Position)
					if s.ruleTargetMatches(rule.Target, segment.Name, target, "") {
						result := s.applyFieldRule(rule, &field, target)
						if result != nil {
							results = append(results, *result)
						}
					}
				}
			}
		}
	}

	return results
}

// =====================================
// OUTPUT FORMAT TRANSFORMATION
// =====================================

// transformToOutputFormat converts parsed message to specified output format
func (s *HL7TransformationService) transformToOutputFormat(parsedMessage *ParsedHL7Message, outputFormat string, includeMetadata bool) (map[string]interface{}, error) {
	switch strings.ToUpper(outputFormat) {
	case "JSON":
		return s.transformToJSON(parsedMessage, includeMetadata)
	case "XML":
		return s.transformToXML(parsedMessage, includeMetadata)
	case "FHIR_READY":
		return s.transformToFHIRReady(parsedMessage, includeMetadata)
	default:
		return nil, fmt.Errorf("unsupported output format: %s", outputFormat)
	}
}

// transformToJSON converts parsed message to JSON format
func (s *HL7TransformationService) transformToJSON(parsedMessage *ParsedHL7Message, includeMetadata bool) (map[string]interface{}, error) {
	result := map[string]interface{}{
		"messageType":    parsedMessage.MessageHeader.MessageType.MessageStructure,
		"messageHeader":  parsedMessage.MessageHeader,
		"segments":       []map[string]interface{}{},
		"segmentGroups":  parsedMessage.SegmentGroups,
	}

	// Transform segments
	for _, segment := range parsedMessage.Segments {
		segmentData := map[string]interface{}{
			"name":       segment.Name,
			"position":   segment.Position,
			"fields":     []map[string]interface{}{},
		}

		// Transform fields
		for _, field := range segment.Fields {
			fieldData := map[string]interface{}{
				"position": field.Position,
				"name":     field.Name,
				"value":    field.Value,
				"dataType": field.DataType,
			}

			// Add components if present
			if len(field.Components) > 0 {
				components := []map[string]interface{}{}
				for _, comp := range field.Components {
					compData := map[string]interface{}{
						"position": comp.Position,
						"name":     comp.Name,
						"value":    comp.Value,
						"dataType": comp.DataType,
					}

					// Add sub-components if present
					if len(comp.SubComponents) > 0 {
						subComponents := []map[string]interface{}{}
						for _, subComp := range comp.SubComponents {
							subCompData := map[string]interface{}{
								"position": subComp.Position,
								"name":     subComp.Name,
								"value":    subComp.Value,
								"dataType": subComp.DataType,
							}
							subComponents = append(subComponents, subCompData)
						}
						compData["subComponents"] = subComponents
					}

					components = append(components, compData)
				}
				fieldData["components"] = components
			}

			// Add repetitions if present
			if len(field.Repetitions) > 0 {
				repetitions := []map[string]interface{}{}
				for _, rep := range field.Repetitions {
					repData := map[string]interface{}{
						"position": rep.Position,
						"value":    rep.Value,
					}
					repetitions = append(repetitions, repData)
				}
				fieldData["repetitions"] = repetitions
			}

			segmentData["fields"] = append(segmentData["fields"].([]map[string]interface{}), fieldData)
		}

		result["segments"] = append(result["segments"].([]map[string]interface{}), segmentData)
	}

	// Add metadata if requested
	if includeMetadata {
		result["metadata"] = map[string]interface{}{
			"encodingCharacters": parsedMessage.EncodingCharacters,
			"messageStructure":   parsedMessage.MessageStructure,
			"statistics":         parsedMessage.MessageStatistics,
			"processingMetadata": parsedMessage.ProcessingMetadata,
		}
	}

	return result, nil
}

// transformToXML converts parsed message to XML format
func (s *HL7TransformationService) transformToXML(parsedMessage *ParsedHL7Message, includeMetadata bool) (map[string]interface{}, error) {
	// For now, return JSON structure with XML marker
	// TODO: Implement actual XML generation
	jsonResult, err := s.transformToJSON(parsedMessage, includeMetadata)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"format": "XML",
		"xmlData": jsonResult,
		"note":   "XML transformation not yet implemented - returning structured data",
	}, nil
}

// transformToFHIRReady converts parsed message to FHIR-ready format
func (s *HL7TransformationService) transformToFHIRReady(parsedMessage *ParsedHL7Message, includeMetadata bool) (map[string]interface{}, error) {
	result := map[string]interface{}{
		"resourceType": "Bundle",
		"id":          parsedMessage.MessageHeader.MessageControlID,
		"type":        "message",
		"timestamp":   parsedMessage.MessageHeader.MessageDateTime,
		"entry":       []map[string]interface{}{},
	}

	// Create MessageHeader resource
	messageHeaderResource := map[string]interface{}{
		"resourceType": "MessageHeader",
		"id":          fmt.Sprintf("msg-%s", parsedMessage.MessageHeader.MessageControlID),
		"eventCoding": map[string]interface{}{
			"system": "http://terminology.hl7.org/CodeSystem/v2-0003",
			"code":   parsedMessage.MessageHeader.MessageType.MessageCode,
			"display": parsedMessage.MessageHeader.MessageType.Description,
		},
		"source": map[string]interface{}{
			"name":     parsedMessage.MessageHeader.SendingApplication,
			"endpoint": fmt.Sprintf("hl7://%s", parsedMessage.MessageHeader.SendingFacility),
		},
		"destination": []map[string]interface{}{
			{
				"name":     parsedMessage.MessageHeader.ReceivingApplication,
				"endpoint": fmt.Sprintf("hl7://%s", parsedMessage.MessageHeader.ReceivingFacility),
			},
		},
	}

	result["entry"] = append(result["entry"].([]map[string]interface{}), map[string]interface{}{
		"resource": messageHeaderResource,
	})

	// Add FHIR-ready segment data
	if includeMetadata {
		result["hl7Metadata"] = map[string]interface{}{
			"originalSegments": len(parsedMessage.Segments),
			"messageStructure": parsedMessage.MessageStructure,
			"statistics":       parsedMessage.MessageStatistics,
		}
	}

	return result, nil
}

// =====================================
// UTILITY AND HELPER METHODS
// =====================================

// buildMessageStructure builds the expected message structure
func (s *HL7TransformationService) buildMessageStructure(messageType HL7MessageType, segments []HL7Segment) *HL7MessageStructure {
	structure := &HL7MessageStructure{
		MessageType:      messageType.MessageStructure,
		Version:          "2.5", // Default version
		RequiredSegments: s.getRequiredSegments(messageType.MessageStructure),
		OptionalSegments: s.getOptionalSegments(messageType.MessageStructure),
		SegmentGroups:    make(map[string]HL7SegmentGroup),
		MaxLength:        0,
		Conformance:      "Standard",
		Extensions:       make(map[string]interface{}),
	}

	// Calculate total message length
	totalLength := 0
	for _, segment := range segments {
		totalLength += len(segment.RawContent)
	}
	structure.MaxLength = totalLength

	return structure
}

// calculateMessageStatistics calculates message statistics
func (s *HL7TransformationService) calculateMessageStatistics(segments []HL7Segment, messageSize int) *HL7MessageStatistics {
	statistics := &HL7MessageStatistics{
		MessageSize:         int64(messageSize),
		SegmentDistribution: make(map[string]int),
	}

	totalFields := 0
	totalComponents := 0

	for _, segment := range segments {
		statistics.TotalSegments++
		statistics.SegmentDistribution[segment.Name]++

		for _, field := range segment.Fields {
			totalFields++
			totalComponents += len(field.Components)
		}
	}

	statistics.TotalFields = totalFields
	statistics.TotalComponents = totalComponents

	// Calculate derived metrics
	if statistics.TotalSegments > 0 {
		statistics.DataDensity = float64(totalFields) / float64(statistics.TotalSegments)
	}

	// Simple complexity score based on structure
	statistics.ComplexityScore = float64(statistics.TotalSegments)*1.0 +
								float64(totalFields)*0.5 +
								float64(totalComponents)*0.1

	return statistics
}

// Performance and utility helper methods
func (s *HL7TransformationService) startTransformationStep(stepName, inputType, outputType string) TransformationStep {
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

func (s *HL7TransformationService) completeTransformationStep(step *TransformationStep, err error, inputSize, outputSize int) {
	step.EndTime = time.Now()
	step.Duration = step.EndTime.Sub(step.StartTime)
	step.Success = err == nil
	step.InputSize = int64(inputSize)
	step.OutputSize = int64(outputSize)

	if err != nil {
		step.ErrorMessage = err.Error()
	}
}

func (s *HL7TransformationService) updatePerformanceMetrics(metrics ProcessingMetrics) {
	s.performanceMetrics.MessagesProcessed++

	// Update running averages
	totalMessages := float64(s.performanceMetrics.MessagesProcessed)
	s.performanceMetrics.AverageParseTime = time.Duration(
		(float64(s.performanceMetrics.AverageParseTime)*(totalMessages-1) +
		 float64(metrics.ParseTime)) / totalMessages)

	s.performanceMetrics.AverageValidateTime = time.Duration(
		(float64(s.performanceMetrics.AverageValidateTime)*(totalMessages-1) +
		 float64(metrics.ValidationTime)) / totalMessages)
}

// GetPerformanceMetrics returns current performance metrics
func (s *HL7TransformationService) GetPerformanceMetrics() HL7PerformanceMetrics {
	return s.performanceMetrics
}

// ResetPerformanceMetrics resets performance tracking
func (s *HL7TransformationService) ResetPerformanceMetrics() {
	s.performanceMetrics = HL7PerformanceMetrics{}
}

// =====================================
// SCHEMA AND DATA TYPE HELPERS
// =====================================

// Data type, field, and validation helper methods
func (s *HL7TransformationService) getFieldName(segmentName string, position int) string {
	// Define known field names for common segments
	fieldNames := map[string]map[int]string{
		"MSH": {
			1: "Field Separator", 2: "Encoding Characters", 3: "Sending Application",
			4: "Sending Facility", 5: "Receiving Application", 6: "Receiving Facility",
			7: "Date/Time of Message", 8: "Security", 9: "Message Type",
			10: "Message Control ID", 11: "Processing ID", 12: "Version ID",
		},
		"PID": {
			1: "Set ID", 2: "Patient ID", 3: "Patient Identifier List",
			5: "Patient Name", 7: "Date/Time of Birth", 8: "Administrative Sex",
			11: "Patient Address", 13: "Phone Number - Home", 14: "Phone Number - Business",
		},
		"PV1": {
			1: "Set ID", 2: "Patient Class", 3: "Assigned Patient Location",
			4: "Admission Type", 7: "Attending Doctor", 19: "Visit Number",
		},
	}

	if segmentFields, exists := fieldNames[segmentName]; exists {
		if fieldName, exists := segmentFields[position]; exists {
			return fieldName
		}
	}

	return fmt.Sprintf("Field_%d", position)
}

func (s *HL7TransformationService) getFieldDataType(segmentName string, position int) string {
	// Define known data types for common fields
	dataTypes := map[string]map[int]string{
		"MSH": {
			3: "HD", 4: "HD", 5: "HD", 6: "HD", 7: "TS", 9: "MSG", 10: "ST", 11: "PT", 12: "VID",
		},
		"PID": {
			3: "CX", 5: "XPN", 7: "TS", 8: "IS", 11: "XAD", 13: "XTN", 14: "XTN",
		},
		"PV1": {
			2: "IS", 3: "PL", 4: "IS", 7: "XCN", 19: "CX",
		},
	}

	if segmentTypes, exists := dataTypes[segmentName]; exists {
		if dataType, exists := segmentTypes[position]; exists {
			return dataType
		}
	}

	return "ST" // Default to string
}

func (s *HL7TransformationService) isRequiredField(segmentName string, position int) bool {
	// Define required fields for common segments
	requiredFields := map[string][]int{
		"MSH": {1, 2, 9, 10, 11, 12},
		"PID": {1, 3, 5},
		"PV1": {1, 2},
	}

	if required, exists := requiredFields[segmentName]; exists {
		for _, reqPos := range required {
			if reqPos == position {
				return true
			}
		}
	}

	return false
}

func (s *HL7TransformationService) getFieldMaxLength(segmentName string, position int) int {
	// Define maximum lengths for fields
	maxLengths := map[string]map[int]int{
		"MSH": {3: 227, 4: 227, 5: 227, 6: 227, 9: 15, 10: 20, 11: 3, 12: 60},
		"PID": {5: 250, 8: 1, 11: 250},
	}

	if segmentLengths, exists := maxLengths[segmentName]; exists {
		if maxLength, exists := segmentLengths[position]; exists {
			return maxLength
		}
	}

	return 0 // No limit
}

func (s *HL7TransformationService) getFieldTable(segmentName string, position int) string {
	// Define table numbers for coded fields
	tables := map[string]map[int]string{
		"PID": {8: "0001"}, // Administrative Sex
		"PV1": {2: "0004", 4: "0007"}, // Patient Class, Admission Type
	}

	if segmentTables, exists := tables[segmentName]; exists {
		if table, exists := segmentTables[position]; exists {
			return table
		}
	}

	return ""
}

func (s *HL7TransformationService) getFieldDescription(segmentName string, position int) string {
	return fmt.Sprintf("%s field %d", segmentName, position)
}

func (s *HL7TransformationService) getComponentName(segmentName string, fieldPosition, componentPosition int) string {
	return fmt.Sprintf("%s.%d.%d", segmentName, fieldPosition, componentPosition)
}

func (s *HL7TransformationService) getComponentDataType(segmentName string, fieldPosition, componentPosition int) string {
	return "ST" // Default
}

func (s *HL7TransformationService) getComponentDescription(segmentName string, fieldPosition, componentPosition int) string {
	return fmt.Sprintf("Component %d of %s.%d", componentPosition, segmentName, fieldPosition)
}

func (s *HL7TransformationService) getSubComponentName(segmentName string, fieldPosition, componentPosition, subComponentPosition int) string {
	return fmt.Sprintf("%s.%d.%d.%d", segmentName, fieldPosition, componentPosition, subComponentPosition)
}

func (s *HL7TransformationService) getSubComponentDataType(segmentName string, fieldPosition, componentPosition, subComponentPosition int) string {
	return "ST" // Default
}

func (s *HL7TransformationService) getSubComponentDescription(segmentName string, fieldPosition, componentPosition, subComponentPosition int) string {
	return fmt.Sprintf("Sub-component %d of %s.%d.%d", subComponentPosition, segmentName, fieldPosition, componentPosition)
}

func (s *HL7TransformationService) isOptionalSegment(segmentName string) bool {
	optionalSegments := map[string]bool{
		"EVN": true, "PD1": true, "NK1": true, "PV2": true, "OBX": true,
		"NTE": true, "AL1": true, "DG1": true, "PR1": true,
	}
	return optionalSegments[segmentName]
}

func (s *HL7TransformationService) getSegmentCardinality(segmentName string) string {
	// Define cardinalities for segments
	cardinalities := map[string]string{
		"MSH": "[1..1]",
		"EVN": "[0..1]",
		"PID": "[1..1]",
		"PD1": "[0..1]",
		"NK1": "[0..*]",
		"PV1": "[1..1]",
		"PV2": "[0..1]",
		"OBX": "[0..*]",
		"OBR": "[0..*]",
		"NTE": "[0..*]",
	}

	if cardinality, exists := cardinalities[segmentName]; exists {
		return cardinality
	}

	return "[0..*]" // Default
}

func (s *HL7TransformationService) getRequiredSegments(messageType string) []string {
	requirements := map[string][]string{
		"ADT_A01": {"MSH", "EVN", "PID", "PV1"},
		"ADT_A02": {"MSH", "EVN", "PID", "PV1"},
		"ADT_A03": {"MSH", "EVN", "PID", "PV1"},
		"ADT_A04": {"MSH", "EVN", "PID", "PV1"},
		"ADT_A08": {"MSH", "EVN", "PID", "PV1"},
		"ORU_R01": {"MSH", "PID", "OBR", "OBX"},
		"ORM_O01": {"MSH", "PID", "ORC"},
	}

	if required, exists := requirements[messageType]; exists {
		return required
	}

	return []string{"MSH"} // Minimum requirement
}

func (s *HL7TransformationService) getOptionalSegments(messageType string) []string {
	optionals := map[string][]string{
		"ADT_A01": {"PD1", "NK1", "PV2", "OBX", "AL1", "DG1", "PR1", "GT1", "IN1", "ACC", "UB1", "UB2"},
		"ORU_R01": {"PD1", "NK1", "PV1", "PV2", "NTE"},
		"ORM_O01": {"PD1", "NK1", "PV1", "PV2", "IN1", "GT1", "AL1", "NTE"},
	}

	if optional, exists := optionals[messageType]; exists {
		return optional
	}

	return []string{} // No optionals defined
}

func (s *HL7TransformationService) getMinimumFieldCount(segmentName string) int {
	minimums := map[string]int{
		"MSH": 12,
		"EVN": 2,
		"PID": 3,
		"PV1": 2,
		"OBR": 4,
		"OBX": 5,
		"ORC": 1,
	}

	if minimum, exists := minimums[segmentName]; exists {
		return minimum
	}

	return 1 // Default minimum
}

func (s *HL7TransformationService) validateDataType(value, dataType string) (bool, string) {
	switch dataType {
	case "TS": // Timestamp
		formats := []string{"20060102", "20060102150405", "200601021504"}
		for _, format := range formats {
			if _, err := time.Parse(format, value); err == nil {
				return true, ""
			}
		}
		return false, "Invalid timestamp format"
	case "IS": // Integer String
		if _, err := strconv.Atoi(value); err != nil {
			return false, "Must be a valid integer"
		}
		return true, ""
	case "ST": // String
		return true, "" // Strings are always valid
	default:
		return true, "" // Unknown types pass by default
	}
}

func (s *HL7TransformationService) validateTableValue(value, table string) (bool, string) {
	// Simple table validation - in production, this would check against actual HL7 tables
	tables := map[string]map[string]string{
		"0001": {"F": "Female", "M": "Male", "O": "Other", "U": "Unknown"},
		"0004": {"E": "Emergency", "I": "Inpatient", "O": "Outpatient", "P": "Preadmit"},
	}

	if tableValues, exists := tables[table]; exists {
		if _, valid := tableValues[value]; valid {
			return true, ""
		}

		// Suggest valid values
		validValues := make([]string, 0, len(tableValues))
		for key := range tableValues {
			validValues = append(validValues, key)
		}
		return false, fmt.Sprintf("Valid values: %v", validValues)
	}

	return true, "" // Unknown tables pass by default
}

func (s *HL7TransformationService) validateSegmentOrder(parsedMessage *ParsedHL7Message) []HL7ValidationResult {
	var results []HL7ValidationResult

	// Define expected segment orders for common message types
	orders := map[string][]string{
		"ADT_A01": {"MSH", "EVN", "PID", "PD1", "NK1", "PV1", "PV2"},
		"ORU_R01": {"MSH", "PID", "PD1", "NK1", "PV1", "PV2", "ORC", "OBR", "NTE", "OBX"},
	}

	messageType := parsedMessage.MessageHeader.MessageType.MessageStructure
	if expectedOrder, exists := orders[messageType]; exists {
		segmentPositions := make(map[string]int)
		for i, segment := range parsedMessage.Segments {
			segmentPositions[segment.Name] = i
		}

		for i := 1; i < len(expectedOrder); i++ {
			current := expectedOrder[i]
			previous := expectedOrder[i-1]

			currentPos, currentExists := segmentPositions[current]
			previousPos, previousExists := segmentPositions[previous]

			if currentExists && previousExists && currentPos < previousPos {
				results = append(results, HL7ValidationResult{
					RuleID:   "SEGMENT_ORDER",
					RuleName: "Segment Order Validation",
					Severity: "WARNING",
					Passed:   false,
					Message:  fmt.Sprintf("Segment %s should come after %s", current, previous),
					Location: fmt.Sprintf("Segments %s:%d, %s:%d", previous, previousPos, current, currentPos),
					Impact:   "Message structure may not conform to standard",
				})
			}
		}
	}

	return results
}

func (s *HL7TransformationService) ruleTargetMatches(target, segmentName, fieldPath, componentPath string) bool {
	if target == segmentName {
		return true
	}
	if target == fieldPath {
		return true
	}
	if strings.HasPrefix(fieldPath, target) {
		return true
	}
	return false
}

func (s *HL7TransformationService) applyMessageRule(rule ValidationRule, parsedMessage *ParsedHL7Message) *HL7ValidationResult {
	// Apply message-level validation rules
	switch rule.ID {
	case "MSH_REQUIRED":
		if len(parsedMessage.Segments) == 0 || parsedMessage.Segments[0].Name != "MSH" {
			return &HL7ValidationResult{
				RuleID:   rule.ID,
				RuleName: rule.Name,
				Severity: rule.Severity,
				Passed:   false,
				Message:  rule.Message,
				Location: "Message",
				Impact:   "Message cannot be processed",
			}
		}
	}
	return nil
}

func (s *HL7TransformationService) applySegmentRule(rule ValidationRule, segment *HL7Segment) *HL7ValidationResult {
	// Apply segment-level validation rules
	return nil
}

func (s *HL7TransformationService) applyFieldRule(rule ValidationRule, field *HL7Field, location string) *HL7ValidationResult {
	// Apply field-level validation rules
	return nil
}

// =====================================
// PUBLIC API METHODS
// =====================================

// GetSupportedMessageTypes returns list of supported HL7 message types
func (s *HL7TransformationService) GetSupportedMessageTypes() []string {
	return []string{
		"ADT_A01", "ADT_A02", "ADT_A03", "ADT_A04", "ADT_A05", "ADT_A08",
		"ORU_R01", "ORM_O01", "SIU_S12", "MDM_T02",
	}
}

// GetValidationRules returns current validation rules
func (s *HL7TransformationService) GetValidationRules() map[string]ValidationRule {
	return s.validationRules
}

// AddValidationRule adds a custom validation rule
func (s *HL7TransformationService) AddValidationRule(rule ValidationRule) {
	s.validationRules[rule.ID] = rule
}

// RemoveValidationRule removes a validation rule
func (s *HL7TransformationService) RemoveValidationRule(ruleID string) {
	delete(s.validationRules, ruleID)
}

// ClearCache clears the parsing cache
func (s *HL7TransformationService) ClearCache() {
	s.parsingCache = make(map[string]*ParsedHL7Message)
}

// GetCacheStats returns cache statistics
func (s *HL7TransformationService) GetCacheStats() map[string]interface{} {
	return map[string]interface{}{
		"cacheSize":  len(s.parsingCache),
		"cacheLimit": 1000, // Could be configurable
		"hitRatio":   s.performanceMetrics.CacheHitRatio,
	}
}