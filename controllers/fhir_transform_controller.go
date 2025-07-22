// controllers/fhir_transform_controller.go
// Complete FHIR transformation controller with schema integration
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
	"ezhealthkonnect/fhir"
	"ezhealthkonnect/hl7" // Added for HL7 parsing

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq" // PostgreSQL driver
)

// =====================================
// FHIR TRANSFORMATION CONTROLLER
// =====================================

type FHIRTransformController struct {
	db     *sql.DB
	config *config.Config
}

// =====================================
// REQUEST/RESPONSE STRUCTURES
// =====================================

type TransformRequest struct {
	HL7Message     interface{}            `json:"hl7Message" binding:"required"` // Can be string or enhanced JSON
	TargetProfile  string                 `json:"targetProfile,omitempty"`
	FHIRVersion    string                 `json:"fhirVersion,omitempty"`
	ValidationMode string                 `json:"validationMode,omitempty"`
	CreateBundle   bool                   `json:"createBundle,omitempty"`
	CustomParams   map[string]interface{} `json:"customParams,omitempty"`
	SourceSystem   string                 `json:"sourceSystem,omitempty"`
	UserID         string                 `json:"userId,omitempty"`
	RequestID      string                 `json:"requestId,omitempty"`
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
	SchemaInfo  map[string]interface{}   `json:"schemaInfo,omitempty"`
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

var startTime = time.Now()

func NewFHIRTransformController(database *sql.DB, cfg *config.Config) *FHIRTransformController {
	return &FHIRTransformController{
		db:     database,
		config: cfg,
	}
}

// =====================================
// ROUTE REGISTRATION
// =====================================

func (tc *FHIRTransformController) RegisterRoutes(api *gin.RouterGroup) {
	fhirGroup := api.Group("/fhir")
	{
		transformGroup := fhirGroup.Group("/transform")
		{
			// Core transformation endpoints
			transformGroup.GET("/status", tc.GetStatus)
			transformGroup.POST("", tc.Transform)

			// Rule management endpoints
			transformGroup.GET("/rules", tc.GetRules)
			transformGroup.POST("/rules", tc.CreateRule)
			transformGroup.PUT("/rules/:id", tc.UpdateRule)
			transformGroup.DELETE("/rules/:id", tc.DeleteRule)

			// Schema discovery endpoints
			transformGroup.GET("/schemas", tc.GetSchemas)
			transformGroup.GET("/schemas/:resourceType", tc.GetSchemaDetails)

			// Validation and testing endpoints
			transformGroup.POST("/validate", tc.ValidateResources)
			transformGroup.GET("/validation/test", tc.TestValidation)

			// Analytics endpoints
			transformGroup.GET("/analytics", tc.GetAnalytics)
			transformGroup.GET("/logs", tc.GetTransformationLogs)
		}
	}
}

// =====================================
// CORE ENDPOINTS
// =====================================

// GET /api/fhir/transform/status
func (tc *FHIRTransformController) GetStatus(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	status := map[string]interface{}{
		"status":  "operational",
		"version": "1.0.0",
		"uptime":  time.Since(startTime).String(),
	}

	// Database status
	if tc.db != nil {
		var ruleCount int
		err := tc.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM hl7_fhir_mappings WHERE is_active = true").Scan(&ruleCount)
		if err == nil {
			status["database"] = map[string]interface{}{
				"connected": true,
				"rules":     ruleCount,
			}
		} else {
			status["database"] = map[string]interface{}{
				"connected": false,
				"error":     err.Error(),
			}
		}
	}

	// FHIR Schema status
	schemaLoader := fhir.GetFHIRSchemaLoader()
	if schemaLoader != nil {
		schemaStats := schemaLoader.GetStats()
		available, _ := schemaLoader.ListAvailableSchemas()

		status["schemas"] = map[string]interface{}{
			"available":   len(available),
			"loaded":      schemaStats.CacheSize,
			"totalLoads":  schemaStats.TotalLoads,
			"cacheHits":   schemaStats.CacheHits,
			"cacheMisses": schemaStats.CacheMisses,
			"errors":      schemaStats.LoadErrors,
		}
	} else {
		status["schemas"] = map[string]interface{}{
			"status": "not_initialized",
			"note":   "FHIR schema loader not available",
		}
	}

	// Statistics
	status["statistics"] = map[string]interface{}{
		"totalRules":                tc.getStatCount("SELECT COUNT(*) FROM hl7_fhir_mappings WHERE is_active = true"),
		"supportedMessageTypes":     []string{"ADT^A01", "ORU^R01", "ORM^O01"},
		"supportedProfiles":         []string{"base", "us-core"},
		"totalTransformations":      tc.getStatCount("SELECT COUNT(*) FROM fhir_transformation_logs"),
		"successfulTransformations": tc.getStatCount("SELECT COUNT(*) FROM fhir_transformation_logs WHERE transformation_status = 'success'"),
	}

	// Capabilities
	status["capabilities"] = map[string]interface{}{
		"ruleManagement":   true,
		"batchProcessing":  true,
		"validation":       true,
		"customMappings":   true,
		"schemaValidation": schemaLoader != nil,
		"bundleCreation":   true,
		"auditLogging":     true,
	}

	c.JSON(http.StatusOK, status)
}

// POST /api/fhir/transform
func (tc *FHIRTransformController) Transform(c *gin.Context) {
	startTime := time.Now()

	// Initialize response
	response := TransformResponse{
		Success:     false,
		RequestID:   tc.generateRequestID(),
		MessageType: "unknown",
		Resources:   []map[string]interface{}{},
		Warnings:    []string{},
		Errors:      []string{},
		Performance: map[string]interface{}{},
		Metadata:    map[string]interface{}{},
	}

	// Parse request
	var request TransformRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.Errors = append(response.Errors, fmt.Sprintf("Invalid request format: %v", err))
		response.Performance["totalTime"] = time.Since(startTime).String()
		c.JSON(http.StatusBadRequest, response)
		return
	}

	// Set request ID if provided
	if request.RequestID != "" {
		response.RequestID = request.RequestID
	}

	// Set defaults
	if request.TargetProfile == "" {
		request.TargetProfile = "base"
	}
	if request.FHIRVersion == "" {
		request.FHIRVersion = "R4"
	}

	if tc.config.VerboseLogging {
		fmt.Printf("🔄 Processing HL7→FHIR transformation (Request: %s, Profile: %s)\n", response.RequestID, request.TargetProfile)
	}

	// Convert HL7 message to enhanced format if needed
	enhancedHL7, messageType, err := tc.processHL7Input(request.HL7Message)
	if err != nil {
		response.Errors = append(response.Errors, fmt.Sprintf("Failed to process HL7 message: %v", err))
		response.Performance["totalTime"] = time.Since(startTime).String()
		c.JSON(http.StatusBadRequest, response)
		return
	}
	response.MessageType = messageType

	if tc.config.VerboseLogging {
		fmt.Printf("📋 Extracted message type: %s\n", messageType)
	}

	// Load transformation rules
	rules, err := tc.loadTransformationRules(messageType, request.TargetProfile)
	if err != nil {
		response.Errors = append(response.Errors, fmt.Sprintf("Failed to load transformation rules: %v", err))
		response.Performance["totalTime"] = time.Since(startTime).String()
		c.JSON(http.StatusInternalServerError, response)
		return
	}

	if len(rules) == 0 {
		response.Warnings = append(response.Warnings, fmt.Sprintf("No transformation rules found for %s with profile %s", messageType, request.TargetProfile))
	}

	if tc.config.VerboseLogging {
		fmt.Printf("📊 Loaded %d transformation rules from database\n", len(rules))
	}

	// Load FHIR schemas
	schemas := tc.loadRequiredSchemas(rules, request.FHIRVersion, request.TargetProfile)
	response.SchemaInfo = map[string]interface{}{
		"schemasLoaded": len(schemas),
		"version":       request.FHIRVersion,
		"profile":       request.TargetProfile,
	}

	// Apply transformation rules
	resources, warnings, err := tc.applyTransformationRules(enhancedHL7, rules, schemas)
	if err != nil {
		response.Errors = append(response.Errors, fmt.Sprintf("Transformation failed: %v", err))
		response.Performance["totalTime"] = time.Since(startTime).String()
		c.JSON(http.StatusUnprocessableEntity, response)
		return
	}

	response.Resources = resources
	response.Warnings = append(response.Warnings, warnings...)

	// Create bundle if requested
	if request.CreateBundle && len(resources) > 0 {
		bundle := tc.createBundle(resources, response.RequestID)
		response.Bundle = bundle
	}

	// Schema validation if available
	if len(schemas) > 0 && request.ValidationMode != "none" {
		validationErrors := tc.validateResources(resources, schemas)
		if len(validationErrors) > 0 {
			for _, err := range validationErrors {
				response.Warnings = append(response.Warnings, fmt.Sprintf("Validation: %s", err))
			}
		}
	}

	// Set final response metadata
	response.Success = len(response.Errors) == 0
	response.Performance = map[string]interface{}{
		"totalTime":    time.Since(startTime).String(),
		"rulesApplied": len(rules),
		"rulesFound":   len(rules),
	}
	response.Metadata = map[string]interface{}{
		"sourceMessage":    messageType,
		"targetProfile":    request.TargetProfile,
		"fhirVersion":      request.FHIRVersion,
		"transformTime":    time.Now().Format(time.RFC3339),
		"resourcesCreated": len(resources),
		"validationMode":   request.ValidationMode,
		"databaseRules":    true,
		"engine":           "database-driven-v1",
	}

	// Async audit logging
	go tc.logTransformation(request, response)

	// Return response
	status := http.StatusOK
	if !response.Success {
		status = http.StatusUnprocessableEntity
	}

	c.JSON(status, response)
}

// =====================================
// RULE MANAGEMENT ENDPOINTS
// =====================================

// GET /api/fhir/transform/rules
func (tc *FHIRTransformController) GetRules(c *gin.Context) {
	messageType := c.Query("messageType")
	profile := c.DefaultQuery("profile", "base")
	segment := c.Query("segment")

	if messageType == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "messageType parameter is required",
			"example": "?messageType=ADT^A01&profile=base",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Build query
	query := `
		SELECT id, hl7_version, hl7_message_type, hl7_segment, hl7_field, hl7_component,
		       fhir_resource, fhir_profile, fhir_path, transformation_rule,
		       condition_expression, is_required, priority, created_at
		FROM hl7_fhir_mappings 
		WHERE hl7_message_type = $1 AND (fhir_profile = $2 OR fhir_profile = 'base')
		  AND is_active = true
	`
	args := []interface{}{messageType, profile}

	if segment != "" {
		query += " AND hl7_segment = $3"
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
		var component, condition sql.NullString

		err := rows.Scan(
			&rule.ID, &rule.HL7Version, &rule.HL7MessageType, &rule.HL7Segment,
			&rule.HL7Field, &component, &rule.FHIRResource, &rule.FHIRProfile,
			&rule.FHIRPath, &transformRuleJSON, &condition, &rule.IsRequired,
			&rule.Priority, &rule.CreatedAt,
		)
		if err != nil {
			continue
		}

		// Parse optional fields
		if component.Valid {
			rule.HL7Component = &component.String
		}
		if condition.Valid {
			rule.ConditionExpr = &condition.String
		}

		// Parse transformation logic JSON
		if len(transformRuleJSON) > 0 {
			json.Unmarshal(transformRuleJSON, &rule.TransformRule)
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

	// Validate rule against FHIR schema if available
	schemaValidated := tc.validateRuleAgainstSchema(request)

	transformRuleJSON, err := json.Marshal(request.TransformationRule)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid transformation rule JSON",
			"details": err.Error(),
		})
		return
	}

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
		"success":         true,
		"ruleId":          ruleID,
		"schemaValidated": schemaValidated,
		"message":         "Transformation rule created successfully",
	})
}

// PUT /api/fhir/transform/rules/:id
func (tc *FHIRTransformController) UpdateRule(c *gin.Context) {
	ruleID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid rule ID",
		})
		return
	}

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

	transformRuleJSON, err := json.Marshal(request.TransformationRule)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "Invalid transformation rule JSON",
			"details": err.Error(),
		})
		return
	}

	query := `
		UPDATE hl7_fhir_mappings 
		SET hl7_version = $2, hl7_message_type = $3, hl7_segment = $4,
		    hl7_field = $5, hl7_component = $6, fhir_resource = $7,
		    fhir_profile = $8, fhir_path = $9, transformation_rule = $10,
		    condition_expression = $11, is_required = $12, priority = $13,
		    updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND is_active = true
	`

	result, err := tc.db.ExecContext(ctx, query, ruleID,
		request.HL7Version, request.HL7MessageType, request.HL7Segment,
		request.HL7Field, request.HL7Component, request.FHIRResource,
		request.FHIRProfile, request.FHIRPath, transformRuleJSON,
		request.ConditionExpr, request.IsRequired, request.Priority,
	)

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to update transformation rule",
			"details": err.Error(),
		})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Rule not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Transformation rule updated successfully",
		"ruleId":  ruleID,
	})
}

// DELETE /api/fhir/transform/rules/:id
func (tc *FHIRTransformController) DeleteRule(c *gin.Context) {
	ruleID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid rule ID",
		})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Soft delete by setting is_active = false
	query := `
		UPDATE hl7_fhir_mappings 
		SET is_active = false, updated_at = CURRENT_TIMESTAMP
		WHERE id = $1 AND is_active = true
	`

	result, err := tc.db.ExecContext(ctx, query, ruleID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to delete transformation rule",
			"details": err.Error(),
		})
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Rule not found",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Transformation rule deleted successfully",
		"ruleId":  ruleID,
	})
}

// =====================================
// SCHEMA DISCOVERY ENDPOINTS
// =====================================

// GET /api/fhir/transform/schemas
func (tc *FHIRTransformController) GetSchemas(c *gin.Context) {
	schemaLoader := fhir.GetFHIRSchemaLoader()
	if schemaLoader == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "FHIR schema loader not initialized",
			"help":    "Ensure FHIR schemas are available in schemas/fhir/R4/ directory",
		})
		return
	}

	available, err := schemaLoader.ListAvailableSchemas()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to list schemas",
			"details": err.Error(),
		})
		return
	}

	// Parse and structure schema information
	schemas := make([]map[string]interface{}, 0, len(available))
	versionCount := make(map[string]int)
	profileCount := make(map[string]int)

	for _, schemaID := range available {
		parts := strings.Split(schemaID, "_")
		if len(parts) >= 3 {
			version := parts[0]
			profile := parts[1]
			resourceType := parts[2]

			versionCount[version]++
			profileCount[profile]++

			schemas = append(schemas, map[string]interface{}{
				"id":           schemaID,
				"version":      version,
				"profile":      profile,
				"resourceType": resourceType,
				"displayName":  fmt.Sprintf("%s %s (%s)", resourceType, profile, version),
			})
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"schemas": schemas,
		"summary": gin.H{
			"total":    len(schemas),
			"versions": versionCount,
			"profiles": profileCount,
		},
		"loader": gin.H{
			"status": "operational",
			"stats":  schemaLoader.GetStats(),
		},
	})
}

// GET /api/fhir/transform/schemas/:resourceType
func (tc *FHIRTransformController) GetSchemaDetails(c *gin.Context) {
	resourceType := c.Param("resourceType")
	profile := c.DefaultQuery("profile", "base")
	version := c.DefaultQuery("version", "R4")

	schemaLoader := fhir.GetFHIRSchemaLoader()
	if schemaLoader == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"success": false,
			"error":   "FHIR schema loader not initialized",
		})
		return
	}

	schema, err := schemaLoader.LoadFHIRSchema(resourceType, profile, version)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success":      false,
			"error":        "Schema not found",
			"resourceType": resourceType,
			"profile":      profile,
			"version":      version,
			"details":      err.Error(),
			"suggestion":   "Try with profile=base or check available schemas at /schemas",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"schema":  schema,
		"info": gin.H{
			"resourceType": resourceType,
			"profile":      profile,
			"version":      version,
			"elements":     len(schema.Elements),
			"required":     len(schema.Required),
			"mustSupport":  len(schema.MustSupport),
			"loadTime":     schema.LoadTime.String(),
		},
	})
}

// =====================================
// VALIDATION ENDPOINTS
// =====================================

// POST /api/fhir/transform/validate
func (tc *FHIRTransformController) ValidateResources(c *gin.Context) {
	var request struct {
		Resources []map[string]interface{} `json:"resources" binding:"required"`
		Profile   string                   `json:"profile,omitempty"`
		Version   string                   `json:"version,omitempty"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request format",
			"details": err.Error(),
		})
		return
	}

	profile := request.Profile
	if profile == "" {
		profile = "base"
	}
	version := request.Version
	if version == "" {
		version = "R4"
	}

	// Load schemas for validation
	schemas := tc.loadSchemasForResources(request.Resources, version, profile)
	validationErrors := tc.validateResources(request.Resources, schemas)

	c.JSON(http.StatusOK, gin.H{
		"success":          len(validationErrors) == 0,
		"resourcesCount":   len(request.Resources),
		"schemasLoaded":    len(schemas),
		"validationErrors": validationErrors,
		"profile":          profile,
		"version":          version,
	})
}

// GET /api/fhir/transform/validation/test
func (tc *FHIRTransformController) TestValidation(c *gin.Context) {
	resourceType := c.DefaultQuery("resourceType", "Patient")
	profile := c.DefaultQuery("profile", "base")

	// Create a sample FHIR resource for testing
	sampleResource := map[string]interface{}{
		"resourceType": resourceType,
		"id":           "test-123",
	}

	if resourceType == "Patient" {
		sampleResource["name"] = []map[string]interface{}{
			{
				"use":    "official",
				"family": "Test",
				"given":  []string{"Patient"},
			},
		}
		sampleResource["gender"] = "unknown"
	}

	schemaLoader := fhir.GetFHIRSchemaLoader()
	if schemaLoader == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "FHIR schema loader not initialized",
		})
		return
	}

	schema, err := schemaLoader.LoadFHIRSchema(resourceType, profile, "R4")
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error":        "Schema not found for validation",
			"resourceType": resourceType,
			"profile":      profile,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":        true,
		"message":        "Schema validation test",
		"sampleResource": sampleResource,
		"schema": gin.H{
			"resourceType": schema.ResourceType,
			"elements":     len(schema.Elements),
			"required":     schema.Required,
		},
		"note": "This demonstrates schema loading capability. Full validation is available via the /validate endpoint.",
	})
}

// =====================================
// ANALYTICS ENDPOINTS
// =====================================

// GET /api/fhir/transform/analytics
func (tc *FHIRTransformController) GetAnalytics(c *gin.Context) {
	days := c.DefaultQuery("days", "7")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	analytics := map[string]interface{}{
		"period": fmt.Sprintf("Last %s days", days),
	}

	// Total transformations
	var totalTransformations int
	tc.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM fhir_transformation_logs WHERE created_at >= CURRENT_DATE - INTERVAL '"+days+" days'").Scan(&totalTransformations)
	analytics["totalTransformations"] = totalTransformations

	// Success rate
	var successfulTransformations int
	tc.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM fhir_transformation_logs WHERE transformation_status = 'success' AND created_at >= CURRENT_DATE - INTERVAL '"+days+" days'").Scan(&successfulTransformations)

	successRate := 0.0
	if totalTransformations > 0 {
		successRate = float64(successfulTransformations) / float64(totalTransformations) * 100
	}
	analytics["successRate"] = fmt.Sprintf("%.1f%%", successRate)

	// Average processing time
	var avgProcessingTime sql.NullFloat64
	tc.db.QueryRowContext(ctx,
		"SELECT AVG(processing_time_ms) FROM fhir_transformation_logs WHERE created_at >= CURRENT_DATE - INTERVAL '"+days+" days'").Scan(&avgProcessingTime)

	if avgProcessingTime.Valid {
		analytics["avgProcessingTime"] = fmt.Sprintf("%.0f ms", avgProcessingTime.Float64)
	} else {
		analytics["avgProcessingTime"] = "N/A"
	}

	c.JSON(http.StatusOK, gin.H{
		"success":   true,
		"analytics": analytics,
	})
}

// GET /api/fhir/transform/logs
func (tc *FHIRTransformController) GetTransformationLogs(c *gin.Context) {
	limit := c.DefaultQuery("limit", "50")
	status := c.Query("status")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := `
		SELECT request_id, hl7_message_type, transformation_status, resources_created,
		       processing_time_ms, warnings_count, errors_count, created_at
		FROM fhir_transformation_logs
	`
	args := []interface{}{}

	if status != "" {
		query += " WHERE transformation_status = $1"
		args = append(args, status)
	}

	query += " ORDER BY created_at DESC LIMIT $" + fmt.Sprintf("%d", len(args)+1)
	args = append(args, limit)

	rows, err := tc.db.QueryContext(ctx, query, args...)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to query transformation logs",
			"details": err.Error(),
		})
		return
	}
	defer rows.Close()

	var logs []map[string]interface{}
	for rows.Next() {
		var log map[string]interface{} = make(map[string]interface{})
		var requestID, messageType, status string
		var resourcesCreated, processingTime, warningsCount, errorsCount int
		var createdAt time.Time

		err := rows.Scan(&requestID, &messageType, &status, &resourcesCreated,
			&processingTime, &warningsCount, &errorsCount, &createdAt)
		if err != nil {
			continue
		}

		log["requestId"] = requestID
		log["messageType"] = messageType
		log["status"] = status
		log["resourcesCreated"] = resourcesCreated
		log["processingTime"] = fmt.Sprintf("%d ms", processingTime)
		log["warningsCount"] = warningsCount
		log["errorsCount"] = errorsCount
		log["createdAt"] = createdAt.Format(time.RFC3339)

		logs = append(logs, log)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"logs":    logs,
		"count":   len(logs),
	})
}

// =====================================
// HELPER METHODS
// =====================================

func (tc *FHIRTransformController) generateRequestID() string {
	return fmt.Sprintf("tf_%d", time.Now().UnixNano())
}

func (tc *FHIRTransformController) getStatCount(query string) int {
	var count int
	if tc.db != nil {
		tc.db.QueryRow(query).Scan(&count)
	}
	return count
}

// =====================================
// ACTUAL IMPLEMENTATION METHODS
// =====================================

// Process HL7 input (string or object) to enhanced format
func (tc *FHIRTransformController) processHL7Input(hl7Input interface{}) (map[string]interface{}, string, error) {
	var hl7Data map[string]interface{}
	var messageType string = "UNKNOWN"

	switch v := hl7Input.(type) {
	case string:
		// Parse HL7 string using existing parser
		parsed := hl7.ParseHL7Enhanced(v)
		if parsed == nil || !parsed.Success {
			return nil, "", fmt.Errorf("invalid HL7 format: %s", parsed.Error)
		}

		// Convert to map format
		hl7Data = make(map[string]interface{})
		for segName, segment := range parsed.EnhancedSegments {
			segmentData := make(map[string]interface{})
			for i, field := range segment.Fields {
				fieldKey := fmt.Sprintf("%d", i+1)
				segmentData[fieldKey] = field.Value
			}
			hl7Data[segName] = segmentData
		}
		messageType = parsed.MessageType.Name

	case map[string]interface{}:
		hl7Data = v
		// Extract message type from MSH segment
		if msh, exists := hl7Data["MSH"].(map[string]interface{}); exists {
			if msgTypeField, exists := msh["9"].(string); exists {
				messageType = msgTypeField
			}
		}

	default:
		return nil, "", fmt.Errorf("HL7 message must be either a string or structured object")
	}

	return hl7Data, messageType, nil
}

// Load transformation rules from database
func (tc *FHIRTransformController) loadTransformationRules(messageType, profile string) ([]TransformationRule, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	query := `
		SELECT id, hl7_version, hl7_message_type, hl7_segment, hl7_field, hl7_component,
		       fhir_resource, fhir_profile, fhir_path, transformation_rule,
		       condition_expression, is_required, priority, created_at
		FROM hl7_fhir_mappings 
		WHERE hl7_message_type = $1 AND (fhir_profile = $2 OR fhir_profile = 'base')
		  AND is_active = true
		ORDER BY priority ASC, id ASC
	`

	rows, err := tc.db.QueryContext(ctx, query, messageType, profile)
	if err != nil {
		return nil, fmt.Errorf("database query failed: %v", err)
	}
	defer rows.Close()

	var rules []TransformationRule
	for rows.Next() {
		var rule TransformationRule
		var transformRuleJSON []byte
		var component, condition sql.NullString

		err := rows.Scan(
			&rule.ID, &rule.HL7Version, &rule.HL7MessageType, &rule.HL7Segment,
			&rule.HL7Field, &component, &rule.FHIRResource, &rule.FHIRProfile,
			&rule.FHIRPath, &transformRuleJSON, &condition, &rule.IsRequired,
			&rule.Priority, &rule.CreatedAt,
		)
		if err != nil {
			continue
		}

		// Parse optional fields
		if component.Valid {
			rule.HL7Component = &component.String
		}
		if condition.Valid {
			rule.ConditionExpr = &condition.String
		}

		// Parse transformation logic JSON
		if len(transformRuleJSON) > 0 {
			json.Unmarshal(transformRuleJSON, &rule.TransformRule)
		}

		rules = append(rules, rule)
	}

	return rules, nil
}

// Load required FHIR schemas based on rules
func (tc *FHIRTransformController) loadRequiredSchemas(rules []TransformationRule, version, profile string) map[string]*fhir.FHIRSchema {
	schemaLoader := fhir.GetFHIRSchemaLoader()
	if schemaLoader == nil {
		return map[string]*fhir.FHIRSchema{}
	}

	schemas := make(map[string]*fhir.FHIRSchema)
	uniqueResources := make(map[string]bool)

	// Collect unique resource types from rules
	for _, rule := range rules {
		uniqueResources[rule.FHIRResource] = true
	}

	// Load schema for each resource type
	for resourceType := range uniqueResources {
		schema, err := schemaLoader.LoadFHIRSchema(resourceType, profile, version)
		if err != nil {
			// Try fallback to base profile
			if profile != "base" {
				schema, err = schemaLoader.LoadFHIRSchema(resourceType, "base", version)
			}
			if err != nil {
				if tc.config.VerboseLogging {
					fmt.Printf("⚠️ Could not load FHIR schema for %s/%s/%s: %v\n", resourceType, profile, version, err)
				}
				continue
			}
		}
		schemas[resourceType] = schema
	}

	return schemas
}

// Apply transformation rules to create FHIR resources
func (tc *FHIRTransformController) applyTransformationRules(hl7Message map[string]interface{}, rules []TransformationRule, schemas map[string]*fhir.FHIRSchema) ([]map[string]interface{}, []string, error) {
	resourcesMap := make(map[string]map[string]interface{})
	warnings := []string{}

	// Group rules by resource type
	rulesByResource := make(map[string][]TransformationRule)
	for _, rule := range rules {
		rulesByResource[rule.FHIRResource] = append(rulesByResource[rule.FHIRResource], rule)
	}

	// Process each resource type
	for resourceType, resourceRules := range rulesByResource {
		resource := tc.initializeFHIRResource(resourceType)
		appliedRules := 0

		// Apply each rule to the resource
		for _, rule := range resourceRules {
			err := tc.applyRule(hl7Message, rule, resource)
			if err != nil {
				if rule.IsRequired {
					warnings = append(warnings, fmt.Sprintf("Required field %s.%s failed: %v", rule.HL7Segment, rule.HL7Field, err))
				}
				continue
			}
			appliedRules++
		}

		// Only include resources that have data beyond the basic structure
		if appliedRules > 0 && tc.hasContent(resource) {
			resourcesMap[resourceType] = resource
		}
	}

	// Convert map to array
	var fhirResources []map[string]interface{}
	for _, resource := range resourcesMap {
		fhirResources = append(fhirResources, resource)
	}

	return fhirResources, warnings, nil
}

// Initialize a FHIR resource with basic structure
func (tc *FHIRTransformController) initializeFHIRResource(resourceType string) map[string]interface{} {
	return map[string]interface{}{
		"resourceType": resourceType,
		"id":           fmt.Sprintf("%s-%d", strings.ToLower(resourceType), time.Now().Unix()),
		"meta": map[string]interface{}{
			"profile": []string{fmt.Sprintf("http://hl7.org/fhir/R4/StructureDefinition/%s", resourceType)},
		},
	}
}

// Apply a single mapping rule to a FHIR resource
func (tc *FHIRTransformController) applyRule(hl7Message map[string]interface{}, rule TransformationRule, resource map[string]interface{}) error {
	// Extract HL7 value
	hl7Value, found := tc.extractHL7Value(hl7Message, rule.HL7Segment, rule.HL7Field, rule.HL7Component)
	if !found || hl7Value == "" {
		if rule.IsRequired {
			return fmt.Errorf("required field not found or empty")
		}
		return fmt.Errorf("field not found")
	}

	// Transform value based on rule
	transformedValue, err := tc.transformValue(hl7Value, rule.TransformRule)
	if err != nil {
		return fmt.Errorf("transformation failed: %v", err)
	}

	// Set value in FHIR resource
	err = tc.setFHIRPath(resource, rule.FHIRPath, transformedValue)
	if err != nil {
		return fmt.Errorf("failed to set FHIR path: %v", err)
	}

	if tc.config.VerboseLogging {
		fmt.Printf("✅ Applied rule: %s.%s → %s.%s\n", rule.HL7Segment, rule.HL7Field, rule.FHIRResource, rule.FHIRPath)
	}

	return nil
}

// Extract HL7 value from parsed data
func (tc *FHIRTransformController) extractHL7Value(hl7Message map[string]interface{}, segment, field string, component *string) (string, bool) {
	segmentData, exists := hl7Message[segment].(map[string]interface{})
	if !exists {
		return "", false
	}

	fieldValue, exists := segmentData[field].(string)
	if !exists {
		return "", false
	}

	// Handle component extraction if specified
	if component != nil && *component != "" {
		// For components, split by ^ and extract the specific component
		components := strings.Split(fieldValue, "^")
		compIndex, err := strconv.Atoi(*component)
		if err != nil || compIndex < 1 || compIndex > len(components) {
			return "", false
		}
		return components[compIndex-1], true
	}

	return fieldValue, true
}

// Transform HL7 value based on transformation rule
func (tc *FHIRTransformController) transformValue(hl7Value string, transformRule map[string]interface{}) (interface{}, error) {
	if transformRule == nil {
		return hl7Value, nil // Direct mapping
	}

	transformType, exists := transformRule["type"].(string)
	if !exists {
		return hl7Value, nil
	}

	switch transformType {
	case "direct":
		return hl7Value, nil

	case "identifier":
		return []map[string]interface{}{
			{
				"use":   "usual",
				"value": hl7Value,
			},
		}, nil

	case "humanName":
		components := strings.Split(hl7Value, "^")
		name := map[string]interface{}{"use": "official"}

		if len(components) > 0 && components[0] != "" {
			name["family"] = components[0]
		}
		if len(components) > 1 && components[1] != "" {
			name["given"] = []string{components[1]}
		}

		return []map[string]interface{}{name}, nil

	case "date", "hl7_date_to_fhir_date":
		// Convert YYYYMMDD to YYYY-MM-DD
		if len(hl7Value) >= 8 {
			return fmt.Sprintf("%s-%s-%s", hl7Value[0:4], hl7Value[4:6], hl7Value[6:8]), nil
		}
		return hl7Value, nil

	case "code", "hl7_code_to_fhir_code":
		// Handle coded values with value set mapping
		if valueSet, exists := transformRule["valueSet"].(string); exists {
			// Look up code mapping in database
			mappedCode := tc.lookupCodeMapping(hl7Value, valueSet)
			if mappedCode != "" {
				return mappedCode, nil
			}
		}
		return hl7Value, nil

	default:
		return hl7Value, nil
	}
}

// Set value in FHIR resource at specified path
func (tc *FHIRTransformController) setFHIRPath(resource map[string]interface{}, path string, value interface{}) error {
	// Simple path setting - can be enhanced for complex paths like "name[0].given"
	resource[path] = value
	return nil
}

// Check if resource has content beyond basic structure
func (tc *FHIRTransformController) hasContent(resource map[string]interface{}) bool {
	// Check if resource has content beyond resourceType, id, and meta
	return len(resource) > 3
}

// Look up code mapping in custom value sets
func (tc *FHIRTransformController) lookupCodeMapping(hl7Code, valueSet string) string {
	if tc.db == nil {
		return ""
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var fhirCode string
	query := `
		SELECT fhir_code 
		FROM fhir_custom_value_sets 
		WHERE value_set_name = $1 AND hl7_code = $2 AND is_active = true
		ORDER BY mapping_confidence DESC, is_default_mapping DESC
		LIMIT 1
	`

	err := tc.db.QueryRowContext(ctx, query, valueSet, hl7Code).Scan(&fhirCode)
	if err != nil {
		return "" // No mapping found
	}

	return fhirCode
}

// Validate resources against FHIR schemas
func (tc *FHIRTransformController) validateResources(resources []map[string]interface{}, schemas map[string]*fhir.FHIRSchema) []string {
	var validationErrors []string

	// Basic validation - check resource type exists and has required fields
	for _, resource := range resources {
		resourceType, exists := resource["resourceType"].(string)
		if !exists {
			validationErrors = append(validationErrors, "Resource missing resourceType")
			continue
		}

		schema, hasSchema := schemas[resourceType]
		if !hasSchema {
			continue // Skip validation if no schema
		}

		// Check required fields
		for _, requiredField := range schema.Required {
			if _, exists := resource[requiredField]; !exists {
				validationErrors = append(validationErrors, fmt.Sprintf("%s missing required field: %s", resourceType, requiredField))
			}
		}
	}

	return validationErrors
}

// Validate rule against FHIR schema
func (tc *FHIRTransformController) validateRuleAgainstSchema(request RuleManagementRequest) bool {
	// Basic validation - can be enhanced with actual schema checking
	return request.FHIRResource != "" && request.FHIRPath != ""
}

// Load schemas for specific resources
func (tc *FHIRTransformController) loadSchemasForResources(resources []map[string]interface{}, version, profile string) map[string]*fhir.FHIRSchema {
	schemaLoader := fhir.GetFHIRSchemaLoader()
	if schemaLoader == nil {
		return map[string]*fhir.FHIRSchema{}
	}

	schemas := make(map[string]*fhir.FHIRSchema)
	uniqueResources := make(map[string]bool)

	// Collect unique resource types
	for _, resource := range resources {
		if resourceType, exists := resource["resourceType"].(string); exists {
			uniqueResources[resourceType] = true
		}
	}

	// Load schema for each resource type
	for resourceType := range uniqueResources {
		schema, err := schemaLoader.LoadFHIRSchema(resourceType, profile, version)
		if err != nil {
			continue
		}
		schemas[resourceType] = schema
	}

	return schemas
}

// Create FHIR Bundle from resources
func (tc *FHIRTransformController) createBundle(resources []map[string]interface{}, requestID string) map[string]interface{} {
	entries := make([]map[string]interface{}, len(resources))

	for i, resource := range resources {
		entries[i] = map[string]interface{}{
			"resource": resource,
		}
	}

	return map[string]interface{}{
		"resourceType": "Bundle",
		"id":           fmt.Sprintf("bundle-%s", requestID),
		"type":         "message",
		"timestamp":    time.Now().Format(time.RFC3339),
		"entry":        entries,
	}
}

// Log transformation for audit purposes
func (tc *FHIRTransformController) logTransformation(request TransformRequest, response TransformResponse) {
	if tc.db == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	status := "success"
	if !response.Success {
		status = "error"
	}

	query := `
		INSERT INTO fhir_transformation_logs 
		(request_id, hl7_message_type, transformation_status, resources_created,
		 processing_time_ms, warnings_count, errors_count, source_system, user_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	// Extract processing time in milliseconds
	processingTimeMs := 0 // Default value
	if totalTimeStr, exists := response.Performance["totalTime"].(string); exists {
		// Parse duration string like "123.456ms" or "1.23s"
		if strings.HasSuffix(totalTimeStr, "ms") {
			if ms, err := strconv.ParseFloat(strings.TrimSuffix(totalTimeStr, "ms"), 64); err == nil {
				processingTimeMs = int(ms)
			}
		} else if strings.HasSuffix(totalTimeStr, "s") {
			if s, err := strconv.ParseFloat(strings.TrimSuffix(totalTimeStr, "s"), 64); err == nil {
				processingTimeMs = int(s * 1000) // Convert seconds to milliseconds
			}
		}
	}

	tc.db.ExecContext(ctx, query,
		response.RequestID,
		response.MessageType,
		status,
		len(response.Resources),
		processingTimeMs,
		len(response.Warnings),
		len(response.Errors),
		request.SourceSystem,
		request.UserID,
	)
}
