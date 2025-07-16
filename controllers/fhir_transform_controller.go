// controllers/fhir_transform_controller.go
package controllers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ezhealthkonnect/config"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq" // PostgreSQL driver
)

// =====================================
// SIMPLIFIED FHIR TRANSFORMATION CONTROLLER
// =====================================

type FHIRTransformController struct {
	db     *sql.DB
	config *config.Config
}

// =====================================
// REQUEST/RESPONSE STRUCTURES
// =====================================

type TransformRequest struct {
	HL7Message     string                 `json:"hl7Message" binding:"required"`
	TargetProfile  string                 `json:"targetProfile,omitempty"`
	ValidationMode string                 `json:"validationMode,omitempty"`
	CreateBundle   bool                   `json:"createBundle,omitempty"`
	CustomParams   map[string]interface{} `json:"customParams,omitempty"`
	SourceSystem   string                 `json:"sourceSystem,omitempty"`
	UserID         string                 `json:"userId,omitempty"`
}

type TransformResponse struct {
	Success     bool                     `json:"success"`
	RequestID   string                   `json:"requestId"`
	MessageType string                   `json:"messageType"`
	Resources   []map[string]interface{} `json:"resources"`
	Bundle      map[string]interface{}   `json:"bundle,omitempty"`
	Warnings    []string                 `json:"warnings"`
	Errors      []string                 `json:"errors"`
	Performance map[string]interface{}   `json:"performance"`
	Metadata    map[string]interface{}   `json:"metadata"`
}

type TransformationRule struct {
	ID             int                    `json:"id"`
	HL7Version     string                 `json:"hl7_version"`
	HL7MessageType string                 `json:"hl7_message_type"`
	HL7Segment     string                 `json:"hl7_segment"`
	HL7Field       string                 `json:"hl7_field"`
	HL7Component   *string                `json:"hl7_component"`
	FHIRResource   string                 `json:"fhir_resource"`
	FHIRProfile    string                 `json:"fhir_profile"`
	FHIRPath       string                 `json:"fhir_path"`
	TransformRule  map[string]interface{} `json:"transformation_rule"`
	ConditionExpr  *string                `json:"condition_expression"`
	IsRequired     bool                   `json:"is_required"`
	Priority       int                    `json:"priority"`
	CreatedAt      time.Time              `json:"created_at"`
}

type RuleManagementRequest struct {
	HL7Version         string                 `json:"hl7Version" binding:"required"`
	HL7MessageType     string                 `json:"hl7MessageType" binding:"required"`
	HL7Segment         string                 `json:"hl7Segment" binding:"required"`
	HL7Field           string                 `json:"hl7Field" binding:"required"`
	HL7Component       *string                `json:"hl7Component,omitempty"`
	FHIRResource       string                 `json:"fhirResource" binding:"required"`
	FHIRProfile        string                 `json:"fhirProfile" binding:"required"`
	FHIRPath           string                 `json:"fhirPath" binding:"required"`
	TransformationRule map[string]interface{} `json:"transformationRule" binding:"required"`
	ConditionExpr      *string                `json:"conditionExpression,omitempty"`
	IsRequired         bool                   `json:"isRequired"`
	Priority           int                    `json:"priority"`
}

// =====================================
// INITIALIZATION
// =====================================

func NewFHIRTransformController(database *sql.DB, cfg *config.Config) *FHIRTransformController {
	return &FHIRTransformController{
		db:     database,
		config: cfg,
	}
}

// =====================================
// CORE ENDPOINTS
// =====================================

// GET /api/fhir/transform/status
func (tc *FHIRTransformController) GetStatus(c *gin.Context) {
	// Check database connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var count int
	err := tc.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM hl7_fhir_mappings").Scan(&count)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  fmt.Sprintf("Database connection failed: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status":  "operational",
		"version": "1.0.0",
		"uptime":  time.Since(startTime).String(),
		"database": gin.H{
			"connected": true,
			"rules":     count,
		},
		"statistics": gin.H{
			"totalRules":                count,
			"supportedMessageTypes":     []string{"ADT^A01", "ORU^R01", "ORM^O01"},
			"supportedProfiles":         []string{"base", "us-core"},
			"totalTransformations":      0,
			"successfulTransformations": 0,
		},
		"capabilities": gin.H{
			"ruleManagement":  true,
			"batchProcessing": true,
			"validation":      true,
			"customMappings":  true,
		},
	})
}

// GET /api/fhir/transform/rules
func (tc *FHIRTransformController) GetRules(c *gin.Context) {
	messageType := c.Query("messageType")
	profile := c.Query("profile")
	segment := c.Query("segment")

	if messageType == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "messageType parameter is required",
			"example": "?messageType=ADT^A01&profile=us-core",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Build query dynamically
	query := `
		SELECT id, hl7_version, hl7_message_type, hl7_segment, hl7_field, hl7_component,
		       fhir_resource, fhir_profile, fhir_path, transformation_rule,
		       condition_expression, is_required, priority, created_at
		FROM hl7_fhir_mappings 
		WHERE hl7_message_type = $1
	`
	args := []interface{}{messageType}

	if profile != "" {
		query += " AND (fhir_profile = $2 OR fhir_profile = 'base')"
		args = append(args, profile)
	}

	if segment != "" {
		query += fmt.Sprintf(" AND hl7_segment = $%d", len(args)+1)
		args = append(args, segment)
	}

	query += " ORDER BY priority ASC, id ASC"

	rows, err := tc.db.QueryContext(ctx, query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to query transformation rules",
			"details": err.Error(),
		})
		return
	}
	defer rows.Close()

	var rules []TransformationRule
	for rows.Next() {
		var rule TransformationRule
		var transformRuleJSON []byte

		err := rows.Scan(
			&rule.ID, &rule.HL7Version, &rule.HL7MessageType, &rule.HL7Segment,
			&rule.HL7Field, &rule.HL7Component, &rule.FHIRResource, &rule.FHIRProfile,
			&rule.FHIRPath, &transformRuleJSON, &rule.ConditionExpr, &rule.IsRequired,
			&rule.Priority, &rule.CreatedAt,
		)
		if err != nil {
			if tc.config.VerboseLogging {
				fmt.Printf("⚠️ Error scanning rule: %v\n", err)
			}
			continue
		}

		// Parse JSON transformation rule
		if len(transformRuleJSON) > 0 {
			if err := json.Unmarshal(transformRuleJSON, &rule.TransformRule); err != nil {
				if tc.config.VerboseLogging {
					fmt.Printf("⚠️ Error parsing transformation rule JSON: %v\n", err)
				}
				rule.TransformRule = make(map[string]interface{})
			}
		}

		rules = append(rules, rule)
	}

	c.JSON(http.StatusOK, gin.H{
		"success":     true,
		"messageType": messageType,
		"profile":     profile,
		"segment":     segment,
		"rules":       rules,
		"count":       len(rules),
	})
}

// POST /api/fhir/transform
// Debug version of Transform method with comprehensive error handling
// Replace the existing Transform method in fhir_transform_controller.go

func (tc *FHIRTransformController) Transform(c *gin.Context) {
	startTime := time.Now()
	fmt.Printf("🚀 DEBUG: Transform endpoint called at %v\n", startTime)

	// Initialize response with error handling
	response := TransformResponse{
		Success:     false,
		RequestID:   generateRequestID(),
		MessageType: "unknown",
		Resources:   []map[string]interface{}{},
		Warnings:    []string{},
		Errors:      []string{},
		Performance: map[string]interface{}{},
		Metadata:    map[string]interface{}{},
	}

	// Ensure we always return a response, even if there's a panic
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("❌ PANIC in Transform: %v\n", r)
			response.Success = false
			response.Errors = append(response.Errors, fmt.Sprintf("Server error: %v", r))
			response.Performance["totalTime"] = time.Since(startTime).String()
			c.JSON(http.StatusInternalServerError, response)
		}
	}()

	fmt.Printf("🔍 DEBUG: Step 1 - Parsing request\n")

	// Parse request with detailed error handling
	var request TransformRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		fmt.Printf("❌ DEBUG: Request binding failed: %v\n", err)
		response.Errors = append(response.Errors, fmt.Sprintf("Invalid request format: %v", err))
		response.Performance["totalTime"] = time.Since(startTime).String()
		c.JSON(http.StatusBadRequest, response)
		return
	}

	fmt.Printf("🔍 DEBUG: Step 2 - Request parsed successfully\n")
	fmt.Printf("   HL7 Message length: %d\n", len(request.HL7Message))
	fmt.Printf("   Target Profile: %s\n", request.TargetProfile)

	// Set defaults
	if request.TargetProfile == "" {
		request.TargetProfile = "us-core"
		fmt.Printf("🔍 DEBUG: Set default target profile: %s\n", request.TargetProfile)
	}

	fmt.Printf("🔍 DEBUG: Step 3 - Checking database connection\n")

	// Test database connection explicitly
	if tc.db == nil {
		fmt.Printf("❌ DEBUG: Database connection is nil\n")
		response.Errors = append(response.Errors, "Database connection not available")
		response.Performance["totalTime"] = time.Since(startTime).String()
		c.JSON(http.StatusServiceUnavailable, response)
		return
	}

	// Test database with ping
	if err := tc.db.Ping(); err != nil {
		fmt.Printf("❌ DEBUG: Database ping failed: %v\n", err)
		response.Errors = append(response.Errors, fmt.Sprintf("Database connection failed: %v", err))
		response.Performance["totalTime"] = time.Since(startTime).String()
		c.JSON(http.StatusServiceUnavailable, response)
		return
	}

	fmt.Printf("✅ DEBUG: Database connection verified\n")

	fmt.Printf("🔍 DEBUG: Step 4 - Parsing HL7 message\n")

	// Parse HL7 message
	messageType, parsedSegments, err := tc.parseHL7Message(request.HL7Message)
	if err != nil {
		fmt.Printf("❌ DEBUG: HL7 parsing failed: %v\n", err)
		response.Errors = append(response.Errors, fmt.Sprintf("Failed to parse HL7 message: %v", err))
		response.Performance["totalTime"] = time.Since(startTime).String()
		c.JSON(http.StatusBadRequest, response)
		return
	}

	fmt.Printf("✅ DEBUG: HL7 message parsed successfully\n")
	fmt.Printf("   Message Type: %s\n", messageType)
	fmt.Printf("   Segments found: %d\n", len(parsedSegments))

	response.MessageType = messageType

	fmt.Printf("🔍 DEBUG: Step 5 - Loading transformation rules\n")

	// Load transformation rules
	rules, err := tc.loadTransformationRules(messageType, request.TargetProfile)
	if err != nil {
		fmt.Printf("❌ DEBUG: Rule loading failed: %v\n", err)
		response.Errors = append(response.Errors, fmt.Sprintf("Failed to load transformation rules: %v", err))
		response.Performance["totalTime"] = time.Since(startTime).String()
		c.JSON(http.StatusInternalServerError, response)
		return
	}

	fmt.Printf("✅ DEBUG: Rules loaded successfully\n")
	fmt.Printf("   Rules found: %d\n", len(rules))

	if len(rules) == 0 {
		fmt.Printf("⚠️ DEBUG: No transformation rules found\n")
		response.Warnings = append(response.Warnings, "No transformation rules found for message type "+messageType)
		response.Errors = append(response.Errors, "No transformation rules available")
		response.Performance["totalTime"] = time.Since(startTime).String()
		response.Performance["rulesFound"] = 0
		c.JSON(http.StatusUnprocessableEntity, response)
		return
	}

	fmt.Printf("🔍 DEBUG: Step 6 - Performing transformation\n")

	// Perform transformation
	resources, warnings, errors := tc.performTransformation(parsedSegments, rules)

	fmt.Printf("✅ DEBUG: Transformation completed\n")
	fmt.Printf("   Resources created: %d\n", len(resources))
	fmt.Printf("   Warnings: %d\n", len(warnings))
	fmt.Printf("   Errors: %d\n", len(errors))

	// Create bundle if requested
	var bundle map[string]interface{}
	if request.CreateBundle && len(resources) > 1 {
		fmt.Printf("🔍 DEBUG: Creating bundle\n")
		bundle = tc.createBundle(resources, response.RequestID)
	}

	fmt.Printf("🔍 DEBUG: Step 7 - Building response\n")

	// Build final response
	response.Success = len(errors) == 0
	response.Resources = resources
	response.Bundle = bundle
	response.Warnings = warnings
	response.Errors = errors
	response.Performance = map[string]interface{}{
		"totalTime":    time.Since(startTime).String(),
		"rulesApplied": len(rules),
		"rulesFound":   len(rules),
	}
	response.Metadata = map[string]interface{}{
		"sourceMessage":    messageType,
		"targetProfile":    request.TargetProfile,
		"transformTime":    time.Now().Format(time.RFC3339),
		"resourcesCreated": len(resources),
		"validationMode":   request.ValidationMode,
	}

	fmt.Printf("✅ DEBUG: Response built successfully\n")
	fmt.Printf("   Success: %v\n", response.Success)
	fmt.Printf("   Response size estimate: %d resources\n", len(response.Resources))

	// Determine status code
	status := http.StatusOK
	if !response.Success {
		status = http.StatusUnprocessableEntity
	}

	fmt.Printf("🔍 DEBUG: Step 8 - Sending JSON response with status %d\n", status)

	// Send response
	c.JSON(status, response)

	fmt.Printf("✅ DEBUG: Transform endpoint completed successfully in %v\n", time.Since(startTime))
}

// Debug version of parseHL7Message with better error handling

// POST /api/fhir/transform/rules
func (tc *FHIRTransformController) CreateRule(c *gin.Context) {
	var request RuleManagementRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid rule format",
			"details": err.Error(),
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Convert transformation rule to JSON
	transformRuleJSON, err := json.Marshal(request.TransformationRule)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid transformation rule JSON",
			"details": err.Error(),
		})
		return
	}

	// Insert new rule
	query := `
		INSERT INTO hl7_fhir_mappings 
		(hl7_version, hl7_message_type, hl7_segment, hl7_field, hl7_component,
		 fhir_resource, fhir_profile, fhir_path, transformation_rule,
		 condition_expression, is_required, priority)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id
	`

	var ruleID int
	err = tc.db.QueryRowContext(ctx, query,
		request.HL7Version, request.HL7MessageType, request.HL7Segment,
		request.HL7Field, request.HL7Component, request.FHIRResource,
		request.FHIRProfile, request.FHIRPath, transformRuleJSON,
		request.ConditionExpr, request.IsRequired, request.Priority,
	).Scan(&ruleID)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to create transformation rule",
			"details": err.Error(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"success": true,
		"message": "Transformation rule created successfully",
		"ruleId":  ruleID,
		"rule": gin.H{
			"id":                 ruleID,
			"hl7MessageType":     request.HL7MessageType,
			"hl7Field":           fmt.Sprintf("%s.%s", request.HL7Segment, request.HL7Field),
			"fhirPath":           request.FHIRPath,
			"transformationType": request.TransformationRule["type"],
		},
	})
}

// =====================================
// HELPER FUNCTIONS
// =====================================

func (tc *FHIRTransformController) parseHL7Message(hl7Message string) (string, map[string][]string, error) {
	// Simple HL7 parsing - extract message type and segments
	lines := strings.Split(strings.ReplaceAll(hl7Message, "\r", "\n"), "\n")
	segments := make(map[string][]string)
	messageType := ""

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) < 3 {
			continue
		}

		segmentType := line[:3]
		fields := strings.Split(line, "|")
		segments[segmentType] = fields

		// Extract message type from MSH segment
		if segmentType == "MSH" && len(fields) > 8 {
			messageType = fields[8] // MSH.9 - Message Type
		}
	}

	if messageType == "" {
		return "", nil, fmt.Errorf("MSH segment not found or invalid")
	}

	return messageType, segments, nil
}

func (tc *FHIRTransformController) loadTransformationRules(messageType, profile string) ([]TransformationRule, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := `
		SELECT id, hl7_version, hl7_message_type, hl7_segment, hl7_field, hl7_component,
		       fhir_resource, fhir_profile, fhir_path, transformation_rule,
		       condition_expression, is_required, priority, created_at
		FROM hl7_fhir_mappings 
		WHERE hl7_message_type = $1 AND (fhir_profile = $2 OR fhir_profile = 'base')
		ORDER BY priority ASC, id ASC
	`

	rows, err := tc.db.QueryContext(ctx, query, messageType, profile)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []TransformationRule
	for rows.Next() {
		var rule TransformationRule
		var transformRuleJSON []byte

		err := rows.Scan(
			&rule.ID, &rule.HL7Version, &rule.HL7MessageType, &rule.HL7Segment,
			&rule.HL7Field, &rule.HL7Component, &rule.FHIRResource, &rule.FHIRProfile,
			&rule.FHIRPath, &transformRuleJSON, &rule.ConditionExpr, &rule.IsRequired,
			&rule.Priority, &rule.CreatedAt,
		)
		if err != nil {
			continue
		}

		if len(transformRuleJSON) > 0 {
			json.Unmarshal(transformRuleJSON, &rule.TransformRule)
		}

		rules = append(rules, rule)
	}

	return rules, nil
}

func (tc *FHIRTransformController) performTransformation(segments map[string][]string, rules []TransformationRule) ([]map[string]interface{}, []string, []string) {
	var resources []map[string]interface{}
	var warnings []string
	var errors []string

	// Group rules by target resource
	resourceRules := make(map[string][]TransformationRule)
	for _, rule := range rules {
		resourceRules[rule.FHIRResource] = append(resourceRules[rule.FHIRResource], rule)
	}

	// Create resources
	for resourceType, rulesForResource := range resourceRules {
		resource := map[string]interface{}{
			"resourceType": resourceType,
			"id":           fmt.Sprintf("%s-%d", strings.ToLower(resourceType), time.Now().Unix()),
		}

		// Apply each rule
		for _, rule := range rulesForResource {
			// Check condition if exists
			if rule.ConditionExpr != nil && *rule.ConditionExpr != "" {
				if !tc.evaluateCondition(*rule.ConditionExpr, segments) {
					continue
				}
			}

			// Extract value from HL7
			value, err := tc.extractHL7Value(segments, rule.HL7Segment, rule.HL7Field, rule.HL7Component)
			if err != nil {
				if rule.IsRequired {
					errors = append(errors, fmt.Sprintf("Required field %s.%s not found: %v", rule.HL7Segment, rule.HL7Field, err))
				} else {
					warnings = append(warnings, fmt.Sprintf("Optional field %s.%s not found: %v", rule.HL7Segment, rule.HL7Field, err))
				}
				continue
			}

			// Transform the value
			transformedValue, err := tc.transformValue(value, rule.TransformRule)
			if err != nil {
				warnings = append(warnings, fmt.Sprintf("Transformation failed for %s.%s: %v", rule.HL7Segment, rule.HL7Field, err))
				continue
			}

			// Set value in FHIR resource
			tc.setFHIRValue(resource, rule.FHIRPath, transformedValue)
		}

		// Only include resource if it has more than just resourceType and id
		if len(resource) > 2 {
			resources = append(resources, resource)
		}
	}

	return resources, warnings, errors
}

func (tc *FHIRTransformController) extractHL7Value(segments map[string][]string, segmentType, fieldCode string, component *string) (string, error) {
	fields, exists := segments[segmentType]
	if !exists {
		return "", fmt.Errorf("segment %s not found", segmentType)
	}

	// Handle special static values
	if fieldCode == "static" {
		return "static_value", nil
	}

	// Parse field position (e.g., "PID.5" -> position 5)
	parts := strings.Split(fieldCode, ".")
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid field code: %s", fieldCode)
	}

	position, err := strconv.Atoi(parts[1])
	if err != nil {
		return "", fmt.Errorf("invalid field position: %s", parts[1])
	}

	if position >= len(fields) {
		return "", fmt.Errorf("field position %d out of range (segment has %d fields)", position, len(fields))
	}

	value := fields[position]

	// Extract component if specified
	if component != nil && *component != "" {
		components := strings.Split(value, "^")
		compIndex, err := strconv.Atoi(*component)
		if err == nil && compIndex > 0 && compIndex <= len(components) {
			value = components[compIndex-1] // Convert to 0-based index
		}
	}

	return value, nil
}

func (tc *FHIRTransformController) transformValue(value string, rule map[string]interface{}) (interface{}, error) {
	if rule == nil {
		return value, nil
	}

	transformType, exists := rule["type"].(string)
	if !exists {
		return value, nil
	}

	switch transformType {
	case "direct":
		if staticValue, exists := rule["value"]; exists {
			return staticValue, nil
		}
		return value, nil

	case "hl7_name":
		return tc.transformHL7Name(value), nil

	case "hl7_identifier":
		return tc.transformHL7Identifier(value), nil

	case "hl7_date":
		return tc.transformHL7Date(value), nil

	case "hl7_datetime":
		return tc.transformHL7DateTime(value), nil

	case "code_map":
		if mapping, exists := rule["map"].(map[string]interface{}); exists {
			if mappedValue, exists := mapping[value]; exists {
				return mappedValue, nil
			}
		}
		if defaultValue, exists := rule["default"]; exists {
			return defaultValue, nil
		}
		return value, nil

	default:
		return value, fmt.Errorf("unknown transformation type: %s", transformType)
	}
}

func (tc *FHIRTransformController) transformHL7Name(value string) map[string]interface{} {
	components := strings.Split(value, "^")
	name := map[string]interface{}{
		"use": "official",
	}

	if len(components) > 0 && components[0] != "" {
		name["family"] = components[0]
	}
	if len(components) > 1 && components[1] != "" {
		given := []string{components[1]}
		if len(components) > 2 && components[2] != "" {
			given = append(given, components[2])
		}
		name["given"] = given
	}

	// Create text representation
	var parts []string
	if given, exists := name["given"]; exists {
		if givenSlice, ok := given.([]string); ok {
			parts = append(parts, strings.Join(givenSlice, " "))
		}
	}
	if family, exists := name["family"]; exists {
		parts = append(parts, fmt.Sprintf("%v", family))
	}
	if len(parts) > 0 {
		name["text"] = strings.Join(parts, " ")
	}

	return name
}

func (tc *FHIRTransformController) transformHL7Identifier(value string) map[string]interface{} {
	components := strings.Split(value, "^")
	identifier := map[string]interface{}{
		"use": "usual",
	}

	if len(components) > 0 && components[0] != "" {
		identifier["value"] = components[0]
	}
	if len(components) > 3 && components[3] != "" {
		identifier["system"] = fmt.Sprintf("http://hospital.smarthealthit.org/%s", strings.ToLower(components[3]))
	}

	return identifier
}

func (tc *FHIRTransformController) transformHL7Date(value string) string {
	if len(value) >= 8 {
		year := value[0:4]
		month := value[4:6]
		day := value[6:8]
		return fmt.Sprintf("%s-%s-%s", year, month, day)
	}
	return value
}

func (tc *FHIRTransformController) transformHL7DateTime(value string) string {
	if len(value) >= 8 {
		year := value[0:4]
		month := value[4:6]
		day := value[6:8]
		result := fmt.Sprintf("%s-%s-%s", year, month, day)

		if len(value) >= 14 {
			hour := value[8:10]
			minute := value[10:12]
			second := value[12:14]
			result += fmt.Sprintf("T%s:%s:%s", hour, minute, second)
		}

		return result
	}
	return value
}

func (tc *FHIRTransformController) setFHIRValue(resource map[string]interface{}, path string, value interface{}) {
	// Simple path setting (e.g., "Patient.name" -> resource["name"] = value)
	parts := strings.Split(path, ".")
	if len(parts) < 2 {
		return
	}

	fieldName := parts[1]

	// Handle array fields that can have multiple values
	arrayFields := map[string]bool{
		"name": true, "identifier": true, "address": true, "telecom": true,
	}

	if arrayFields[fieldName] {
		if existing, exists := resource[fieldName]; exists {
			if existingArray, ok := existing.([]interface{}); ok {
				resource[fieldName] = append(existingArray, value)
			} else {
				resource[fieldName] = []interface{}{existing, value}
			}
		} else {
			resource[fieldName] = []interface{}{value}
		}
	} else {
		resource[fieldName] = value
	}
}

func (tc *FHIRTransformController) evaluateCondition(condition string, segments map[string][]string) bool {
	// Simple condition evaluation
	if strings.Contains(condition, "segment exists") {
		segmentType := strings.Fields(condition)[0]
		_, exists := segments[segmentType]
		return exists
	}
	return true // Default to true for unknown conditions
}

func (tc *FHIRTransformController) createBundle(resources []map[string]interface{}, requestID string) map[string]interface{} {
	entries := make([]map[string]interface{}, len(resources))
	for i, resource := range resources {
		resourceType := resource["resourceType"].(string)
		resourceID := resource["id"].(string)

		entries[i] = map[string]interface{}{
			"fullUrl":  fmt.Sprintf("%s/%s", resourceType, resourceID),
			"resource": resource,
			"request": map[string]interface{}{
				"method": "PUT",
				"url":    fmt.Sprintf("%s/%s", resourceType, resourceID),
			},
		}
	}

	return map[string]interface{}{
		"resourceType": "Bundle",
		"id":           fmt.Sprintf("bundle-%s", requestID),
		"type":         "transaction",
		"entry":        entries,
	}
}

// =====================================
// ROUTE REGISTRATION
// =====================================

// In fhir_transform_controller.go, find the RegisterRoutes method and change:

func (tc *FHIRTransformController) RegisterRoutes(api *gin.RouterGroup) {
	fhirGroup := api.Group("/fhir")
	{
		transformGroup := fhirGroup.Group("/transform")
		{
			transformGroup.GET("/status", tc.GetStatus)
			transformGroup.GET("/rules", tc.GetRules)
			// CHANGE THIS LINE:
			// transformGroup.POST("/", tc.Transform)  // OLD - creates /api/fhir/transform/
			transformGroup.POST("/", tc.Transform) // NEW - creates /api/fhir/transform
			transformGroup.POST("/rules", tc.CreateRule)
		}
	}
}

// =====================================
// UTILITY FUNCTIONS
// =====================================

func generateRequestID() string {
	return fmt.Sprintf("tf_%d", time.Now().UnixNano())
}

var startTime = time.Now()
