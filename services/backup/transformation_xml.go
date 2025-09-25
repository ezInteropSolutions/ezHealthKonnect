// services/transformation_xml.go
// XML Transformation Service for Universal Interface Engine
//
// 🎯 PURPOSE: XML parsing, validation, and transformation
package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
)

// =====================================
// XML TRANSFORMATION SERVICE
// =====================================

type XMLTransformationService struct {
	db                 *sql.DB
	xmlParser          *XMLParser
	namespaceManager   *NamespaceManager
	xsltProcessor      *XSLTProcessor
	performanceMetrics XMLPerformanceMetrics
}

type XMLPerformanceMetrics struct {
	MessagesProcessed    int64         `json:"messagesProcessed"`
	AverageParseTime     time.Duration `json:"averageParseTime"`
	AverageTransformTime time.Duration `json:"averageTransformTime"`
	ErrorRate            float64       `json:"errorRate"`
}

type XMLTransformRequest struct {
	MessageID     string                 `json:"messageId"`
	RawXML        []byte                 `json:"rawXml"`
	TargetFormat  MessageType            `json:"targetFormat"`
	Namespaces    map[string]string      `json:"namespaces,omitempty"`
	XSLTStylesheet string                `json:"xsltStylesheet,omitempty"`
	Validation    string                 `json:"validation,omitempty"`
	Options       map[string]interface{} `json:"options,omitempty"`
}

type XMLTransformResponse struct {
	Success           bool                   `json:"success"`
	MessageID         string                 `json:"messageId"`
	ParsedXML         map[string]interface{} `json:"parsedXml"`
	TransformedData   map[string]interface{} `json:"transformedData"`
	ValidationResults []XMLValidationResult  `json:"validationResults"`
	Warnings          []string               `json:"warnings"`
	Errors            []string               `json:"errors"`
	ProcessingMetrics ProcessingMetrics      `json:"processingMetrics"`
}

type XMLParser struct {
	preserveWhitespace bool
	namespaceAware     bool
	validating         bool
}

type NamespaceManager struct {
	namespaces map[string]string
	prefixes   map[string]string
}

type XSLTProcessor struct {
	stylesheets map[string]string
}

type XMLValidationResult struct {
	IsValid  bool     `json:"isValid"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
}

// NewXMLTransformationService creates a new XML transformation service
func NewXMLTransformationService(database *sql.DB) *XMLTransformationService {
	return &XMLTransformationService{
		db: database,
		xmlParser: &XMLParser{
			preserveWhitespace: false,
			namespaceAware:     true,
			validating:         true,
		},
		namespaceManager: &NamespaceManager{
			namespaces: make(map[string]string),
			prefixes:   make(map[string]string),
		},
		xsltProcessor:      &XSLTProcessor{stylesheets: make(map[string]string)},
		performanceMetrics: XMLPerformanceMetrics{},
	}
}

// Transform transforms a UniversalMessage containing XML content
func (s *XMLTransformationService) Transform(ctx context.Context, message *UniversalMessage) error {
	transformRecord := message.StartTransformation("XMLTransformationService", MessageTypeXML, MessageTypeJSON)

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

	request := &XMLTransformRequest{
		MessageID:    message.ID,
		RawXML:       message.RawContent,
		TargetFormat: MessageTypeJSON,
		Validation:   "MODERATE",
	}

	response, err := s.TransformXML(ctx, request)
	if err != nil {
		transformError = err
		message.AddError("TRANSFORMATION", "XMLTransformationService", "XML_TRANSFORM_FAILED",
			"Failed to transform XML", err.Error(), true)
		return err
	}

	if !response.Success {
		transformError = fmt.Errorf("XML transformation failed")
		return transformError
	}

	message.ParsedContent = response.TransformedData
	outputBytes, _ := json.Marshal(response.TransformedData)
	outputSize = int64(len(outputBytes))
	message.AddTransformedContent(MessageTypeJSON, outputBytes, transformRecord.ID)

	message.UpdateStatus(StatusTransformed, "XMLTransformationService", "XML transformation completed")

	log.Printf("✅ XML transformation completed for message %s (Duration: %v)",
		message.ID, time.Since(startTime))
	return nil
}

// TransformXML performs XML transformation
func (s *XMLTransformationService) TransformXML(ctx context.Context, request *XMLTransformRequest) (*XMLTransformResponse, error) {
	startTime := time.Now()

	response := &XMLTransformResponse{
		Success:           false,
		MessageID:         request.MessageID,
		ValidationResults: []XMLValidationResult{},
		Warnings:          []string{},
		Errors:            []string{},
	}

	// Parse XML
	parsedXML, err := s.parseXML(request.RawXML)
	if err != nil {
		response.Errors = append(response.Errors, fmt.Sprintf("XML parse error: %v", err))
		return response, err
	}

	response.ParsedXML = parsedXML

	// Transform based on target format
	switch request.TargetFormat {
	case MessageTypeJSON:
		response.TransformedData = s.xmlToJSON(parsedXML)
	case MessageTypeFHIR:
		response.TransformedData = s.xmlToFHIR(parsedXML)
	case MessageTypeHL7:
		response.TransformedData = s.xmlToHL7(parsedXML)
	default:
		response.TransformedData = parsedXML
	}

	response.Success = true
	response.ProcessingMetrics = ProcessingMetrics{
		TotalTime: time.Since(startTime),
	}

	return response, nil
}

// parseXML parses XML content into a map structure
func (s *XMLTransformationService) parseXML(xmlData []byte) (map[string]interface{}, error) {
	xmlStr := string(xmlData)

	// Simple XML parsing - in production would use proper XML parser
	result := map[string]interface{}{
		"xml": map[string]interface{}{
			"root": s.extractXMLElements(xmlStr),
		},
		"metadata": map[string]interface{}{
			"parsedAt": time.Now().Format(time.RFC3339),
			"size":     len(xmlData),
		},
	}

	return result, nil
}

// extractXMLElements extracts XML elements (simplified)
func (s *XMLTransformationService) extractXMLElements(xmlStr string) map[string]interface{} {
	// Simplified XML parsing - extract basic structure
	elements := make(map[string]interface{})

	// Extract root element
	rootRegex := regexp.MustCompile(`<(\w+)[^>]*>(.*)</\1>`)
	matches := rootRegex.FindStringSubmatch(xmlStr)

	if len(matches) > 1 {
		elements["rootElement"] = matches[1]
		elements["content"] = s.extractTextContent(matches[2])
		elements["attributes"] = s.extractAttributes(matches[0])
	}

	return elements
}

// extractTextContent extracts text content from XML
func (s *XMLTransformationService) extractTextContent(content string) string {
	// Remove XML tags and return text content
	tagRegex := regexp.MustCompile(`<[^>]*>`)
	return strings.TrimSpace(tagRegex.ReplaceAllString(content, " "))
}

// extractAttributes extracts XML attributes
func (s *XMLTransformationService) extractAttributes(element string) map[string]string {
	attributes := make(map[string]string)

	attrRegex := regexp.MustCompile(`(\w+)="([^"]*)"`)
	matches := attrRegex.FindAllStringSubmatch(element, -1)

	for _, match := range matches {
		if len(match) == 3 {
			attributes[match[1]] = match[2]
		}
	}

	return attributes
}

// xmlToJSON converts XML structure to JSON
func (s *XMLTransformationService) xmlToJSON(xmlData map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"format":        "JSON",
		"convertedFrom": "XML",
		"data":          xmlData,
		"convertedAt":   time.Now().Format(time.RFC3339),
	}
}

// xmlToFHIR converts XML to FHIR format
func (s *XMLTransformationService) xmlToFHIR(xmlData map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"resourceType": "Bundle",
		"id":           uuid.New().String(),
		"type":         "document",
		"entry": []map[string]interface{}{
			{
				"resource": map[string]interface{}{
					"resourceType": "DocumentReference",
					"id":           uuid.New().String(),
					"status":       "current",
					"content": []map[string]interface{}{
						{
							"attachment": map[string]interface{}{
								"contentType": "application/xml",
								"data":        xmlData,
							},
						},
					},
				},
			},
		},
		"meta": map[string]interface{}{
			"lastUpdated": time.Now().Format(time.RFC3339),
			"source":      "XMLTransformationService",
		},
	}
}

// xmlToHL7 converts XML to HL7 format
func (s *XMLTransformationService) xmlToHL7(xmlData map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"messageHeader": map[string]interface{}{
			"sendingApplication": "XML_CONVERTER",
			"messageType": map[string]interface{}{
				"messageCode":      "ADT",
				"triggerEvent":     "A01",
				"messageStructure": "ADT_A01",
			},
			"messageControlID": uuid.New().String(),
			"messageDateTime":  time.Now().Format("20060102150405"),
		},
		"segments": []map[string]interface{}{
			{
				"name": "MSH",
				"fields": []map[string]interface{}{
					{"position": 1, "value": "|"},
					{"position": 2, "value": "^~\\&"},
				},
			},
		},
		"metadata": map[string]interface{}{
			"convertedFrom": "XML",
			"originalData":  xmlData,
		},
	}
}

func (s *XMLTransformationService) GetPerformanceMetrics() XMLPerformanceMetrics {
	return s.performanceMetrics
}