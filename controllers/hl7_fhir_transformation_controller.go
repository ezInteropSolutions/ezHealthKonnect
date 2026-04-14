// controllers/hl7_fhir_transformation_controller.go
// Updated controller using new database schema and transformation service
package controllers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"ezhealthkonnect/config"
	"ezhealthkonnect/services"

	"github.com/gin-gonic/gin"
)

// =====================================
// HL7 TO FHIR TRANSFORMATION CONTROLLER
// =====================================

type HL7FHIRTransformationController struct {
	db               *sql.DB
	config           *config.Config
	transformService *services.HL7FHIRTransformServiceV3
}

// =====================================
// CONTROLLER INITIALIZATION
// =====================================

func NewHL7FHIRTransformationController(database *sql.DB, cfg *config.Config) *HL7FHIRTransformationController {
	transformService := services.NewHL7FHIRTransformServiceV3(database)

	return &HL7FHIRTransformationController{
		db:               database,
		config:           cfg,
		transformService: transformService,
	}
}

// =====================================
// ROUTE REGISTRATION
// =====================================

func (c *HL7FHIRTransformationController) RegisterRoutes(router *gin.RouterGroup) {
	fhirGroup := router.Group("/fhir")
	{
		// Core transformation endpoints
		fhirGroup.POST("/transform", c.TransformHL7ToFHIR)
		fhirGroup.GET("/transform/status", c.GetTransformationStatus)

		// Testing and validation endpoints
		fhirGroup.POST("/transform/test", c.TestTransformation)
		fhirGroup.POST("/transform/validate", c.ValidateTransformation)

		// Configuration endpoints
		fhirGroup.GET("/transform/templates", c.GetMessageTemplates)
		fhirGroup.GET("/transform/mappings/:messageType", c.GetFieldMappings)
		fhirGroup.GET("/transform/valuesets", c.GetValueSetMappings)

		// Analytics and monitoring
		fhirGroup.GET("/transform/analytics", c.GetTransformationAnalytics)
		fhirGroup.GET("/transform/logs", c.GetTransformationLogs)
	}
}

// =====================================
// CORE TRANSFORMATION ENDPOINTS
// =====================================

// POST /api/fhir/transform
func (c *HL7FHIRTransformationController) TransformHL7ToFHIR(ctx *gin.Context) {
	var request services.TransformRequest
	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Invalid request format: %v", err),
			"message": "Expected ParsedHL7Data field with parsed HL7 JSON structure",
		})
		return
	}

	// Generate request ID if not provided
	if request.RequestID == "" {
		request.RequestID = fmt.Sprintf("transform_%d", time.Now().UnixNano())
	}

	// Set defaults
	if request.FHIRVersion == "" {
		request.FHIRVersion = "R4"
	}
	if request.TargetProfile == "" {
		request.TargetProfile = "base"
	}

	if c.config.VerboseLogging {
		fmt.Printf("🔄 Processing HL7→FHIR transformation (Request: %s, Profile: %s)\n",
			request.RequestID, request.TargetProfile)
	}

	// Perform transformation
	transformCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	response, err := c.transformService.Transform(transformCtx, &request)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"success":   false,
			"requestId": request.RequestID,
			"error":     fmt.Sprintf("Transformation failed: %v", err),
		})
		return
	}

	// ── FHIR Validation Policy enforcement ───────────────────────────────────
	if len(response.ValidationIssues) > 0 && request.InterfaceID != "" && c.db != nil {
		policy := c.lookupValidationPolicy(transformCtx, request.InterfaceID)
		switch policy {
		case "reject":
			ctx.JSON(http.StatusUnprocessableEntity, gin.H{
				"success":            false,
				"requestId":          request.RequestID,
				"error":              "Message rejected: required FHIR fields are missing",
				"validationFailures": response.ValidationIssues,
			})
			return
		case "queue_review":
			if qErr := c.enqueueForReview(transformCtx, &request, response); qErr != nil {
				fmt.Printf("⚠️  Failed to enqueue message for review: %v\n", qErr)
			}
			ctx.JSON(http.StatusAccepted, gin.H{
				"success":            true,
				"held_for_review":    true,
				"requestId":          request.RequestID,
				"message":            "Message held for manual review due to missing required FHIR fields",
				"validationFailures": response.ValidationIssues,
			})
			return
		case "warn":
			// Add OperationOutcome to bundle — continue to deliver below
			c.attachOperationOutcome(response)
		// "proceed" (default): fall through unchanged
		}
	}
	// ─────────────────────────────────────────────────────────────────────────

	// Return response
	httpStatus := http.StatusOK
	if !response.Success {
		httpStatus = http.StatusUnprocessableEntity
	}

	if c.config.VerboseLogging {
		fmt.Printf("✅ HL7→FHIR transformation completed: %s (%d resources)\n",
			response.RequestID, len(response.FHIRResources))
	}

	ctx.JSON(httpStatus, response)
}

// GET /api/fhir/transform/status
func (c *HL7FHIRTransformationController) GetTransformationStatus(ctx *gin.Context) {
	status := map[string]interface{}{
		"status":      "operational",
		"service":     "HL7 to FHIR Transformation",
		"version":     "2.0.0",
		"timestamp":   time.Now().Format(time.RFC3339),
		"database":    c.db != nil,
		"environment": c.config.Environment,
	}

	// Check database connectivity and get stats
	if c.db != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var templateCount, mappingCount, valueSetCount int

		c.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM message_fhir_templates WHERE is_default = true").Scan(&templateCount)
		c.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM field_element_mappings").Scan(&mappingCount)
		c.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM value_set_mappings").Scan(&valueSetCount)

		status["database"] = map[string]interface{}{
			"connected":        true,
			"templates":        templateCount,
			"fieldMappings":    mappingCount,
			"valueSetMappings": valueSetCount,
		}
	}

	// Supported features
	status["capabilities"] = map[string]interface{}{
		"messageTypes": []string{"ADT^A01", "ADT^A02", "ADT^A03", "ADT^A04", "ADT^A08", "ORU^R01", "ORM^O01", "SIU^S12", "MDM^T02", "VXU^V04", "RDE^O11", "RDS^O13", "BAR^P01", "DFT^P03"},
		"fhirVersions": []string{"R4"},
		"profiles":     []string{"base", "us-core"},
		"features": []string{
			"Field-level mapping",
			"Value set transformation",
			"Data type conversion",
			"Bundle creation",
			"Validation",
			"Analytics",
		},
	}

	ctx.JSON(http.StatusOK, status)
}

// =====================================
// TESTING AND VALIDATION ENDPOINTS
// =====================================

// POST /api/fhir/transform/test
func (c *HL7FHIRTransformationController) TestTransformation(ctx *gin.Context) {
	var request struct {
		MessageType    string                 `json:"messageType" binding:"required"`
		UseExampleData bool                   `json:"useExampleData"`
		ParsedHL7Data  map[string]interface{} `json:"parsedHL7Data"`
		TargetProfile  string                 `json:"targetProfile"`
		CreateBundle   bool                   `json:"createBundle"`
	}

	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Invalid request: %v", err),
		})
		return
	}

	// Use example data if requested or if no data provided
	var parsedData map[string]interface{}
	if request.UseExampleData || request.ParsedHL7Data == nil {
		parsedData = c.generateExampleParsedHL7(request.MessageType)
	} else {
		parsedData = request.ParsedHL7Data
	}

	// Create transform request
	transformRequest := &services.TransformRequest{
		ParsedHL7Data: parsedData,
		TargetProfile: request.TargetProfile,
		FHIRVersion:   "R4",
		CreateBundle:  request.CreateBundle,
		RequestID:     fmt.Sprintf("test_%d", time.Now().UnixNano()),
	}

	// Perform transformation
	transformCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	response, err := c.transformService.Transform(transformCtx, transformRequest)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Test transformation failed: %v", err),
		})
		return
	}

	// Add test metadata
	testResponse := map[string]interface{}{
		"success":        response.Success,
		"requestId":      response.RequestID,
		"messageType":    response.MessageType,
		"fhirResources":  response.FHIRResources,
		"bundle":         response.Bundle,
		"resourceCounts": response.ResourceCounts,
		"mappingStats":   response.MappingStats,
		"warnings":       response.Warnings,
		"errors":         response.Errors,
		"performance":    response.Performance,
		"testMetadata": map[string]interface{}{
			"usedExampleData": request.UseExampleData || request.ParsedHL7Data == nil,
			"targetProfile":   request.TargetProfile,
			"timestamp":       time.Now().Format(time.RFC3339),
		},
	}

	ctx.JSON(http.StatusOK, testResponse)
}

// POST /api/fhir/transform/validate
func (c *HL7FHIRTransformationController) ValidateTransformation(ctx *gin.Context) {
	var request struct {
		FHIRResources []map[string]interface{} `json:"fhirResources" binding:"required"`
		Profile       string                   `json:"profile"`
		Strict        bool                     `json:"strict"`
	}

	if err := ctx.ShouldBindJSON(&request); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid validation request",
		})
		return
	}

	// Basic validation for now - can be enhanced with FHIR schema validation
	issues := []map[string]interface{}{}

	for i, resource := range request.FHIRResources {
		resourceType, hasType := resource["resourceType"].(string)
		if !hasType {
			issues = append(issues, map[string]interface{}{
				"severity":      "error",
				"code":          "missing-resource-type",
				"message":       fmt.Sprintf("Resource at index %d missing resourceType", i),
				"resourceIndex": i,
			})
		}

		if hasType {
			if _, hasId := resource["id"]; !hasId {
				issues = append(issues, map[string]interface{}{
					"severity":      "warning",
					"code":          "missing-id",
					"message":       fmt.Sprintf("%s resource missing id field", resourceType),
					"resourceType":  resourceType,
					"resourceIndex": i,
				})
			}
		}
	}

	isValid := len(issues) == 0 || (request.Strict == false && !c.hasErrorSeverity(issues))

	ctx.JSON(http.StatusOK, gin.H{
		"success":       true,
		"valid":         isValid,
		"resourceCount": len(request.FHIRResources),
		"issues":        issues,
		"profile":       request.Profile,
		"validatedAt":   time.Now().Format(time.RFC3339),
	})
}

// =====================================
// CONFIGURATION ENDPOINTS
// =====================================

// GET /api/fhir/transform/templates
func (c *HL7FHIRTransformationController) GetMessageTemplates(ctx *gin.Context) {
	messageType := ctx.Query("messageType")

	query := `
		SELECT message_type, event_type, fhir_resources, resource_conditions, source, created_at
		FROM message_fhir_templates 
		WHERE is_default = true
	`
	args := []interface{}{}

	if messageType != "" {
		query += " AND message_type = $1"
		args = append(args, messageType)
	}

	query += " ORDER BY message_type, created_at DESC"

	dbCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := c.db.QueryContext(dbCtx, query, args...)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to query message templates",
		})
		return
	}
	defer rows.Close()

	var templates []map[string]interface{}
	for rows.Next() {
		var msgType, source string
		var eventType sql.NullString
		var resourcesJSON, conditionsJSON []byte
		var createdAt time.Time

		err := rows.Scan(&msgType, &eventType, &resourcesJSON, &conditionsJSON, &source, &createdAt)
		if err != nil {
			continue
		}

		var resources []string
		var conditions map[string]interface{}
		json.Unmarshal(resourcesJSON, &resources)
		json.Unmarshal(conditionsJSON, &conditions)

		template := map[string]interface{}{
			"messageType":        msgType,
			"fhirResources":      resources,
			"resourceConditions": conditions,
			"source":             source,
			"createdAt":          createdAt.Format(time.RFC3339),
		}

		if eventType.Valid {
			template["eventType"] = eventType.String
		}

		templates = append(templates, template)
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success":   true,
		"templates": templates,
		"count":     len(templates),
	})
}

// GET /api/fhir/transform/mappings/:messageType
func (c *HL7FHIRTransformationController) GetFieldMappings(ctx *gin.Context) {
	messageType := ctx.Param("messageType")
	profile := ctx.DefaultQuery("profile", "base")
	segment := ctx.Query("segment")

	if messageType == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "messageType is required",
		})
		return
	}

	query := `
		SELECT fem.segment_name, fem.hl7_field, fem.hl7_component,
		       fem.fhir_resource_type, fem.fhir_element_path, fem.data_type_transform,
		       fem.is_required, fem.cardinality, fem.transformation_rules
		FROM field_element_mappings fem
		WHERE EXISTS (
			SELECT 1 FROM message_fhir_templates mft 
			WHERE mft.message_type = $1 
			AND mft.fhir_resources::jsonb ? fem.fhir_resource_type
		)
	`
	args := []interface{}{messageType}

	if segment != "" {
		query += " AND fem.segment_name = $2"
		args = append(args, segment)
	}

	query += " ORDER BY fem.segment_name, fem.hl7_field"

	dbCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := c.db.QueryContext(dbCtx, query, args...)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to query field mappings",
		})
		return
	}
	defer rows.Close()

	var mappings []map[string]interface{}
	for rows.Next() {
		var segmentName, hl7Field, fhirResourceType, fhirElementPath, cardinality string
		var hl7Component, dataTypeTransform sql.NullString
		var isRequired bool
		var rulesJSON []byte

		err := rows.Scan(&segmentName, &hl7Field, &hl7Component, &fhirResourceType,
			&fhirElementPath, &dataTypeTransform, &isRequired, &cardinality, &rulesJSON)
		if err != nil {
			continue
		}

		mapping := map[string]interface{}{
			"segmentName":      segmentName,
			"hl7Field":         hl7Field,
			"fhirResourceType": fhirResourceType,
			"fhirElementPath":  fhirElementPath,
			"isRequired":       isRequired,
			"cardinality":      cardinality,
		}

		if hl7Component.Valid {
			mapping["hl7Component"] = hl7Component.String
		}
		if dataTypeTransform.Valid {
			mapping["dataTypeTransform"] = dataTypeTransform.String
		}

		if len(rulesJSON) > 0 {
			var rules map[string]interface{}
			json.Unmarshal(rulesJSON, &rules)
			mapping["transformationRules"] = rules
		}

		mappings = append(mappings, mapping)
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success":     true,
		"messageType": messageType,
		"profile":     profile,
		"segment":     segment,
		"mappings":    mappings,
		"count":       len(mappings),
	})
}

// GET /api/fhir/transform/valuesets
func (c *HL7FHIRTransformationController) GetValueSetMappings(ctx *gin.Context) {
	mappingName := ctx.Query("mappingName")
	hl7Table := ctx.Query("hl7Table")

	query := `
		SELECT mapping_name, hl7_table, hl7_value, fhir_system, fhir_code, fhir_display, mapping_type
		FROM value_set_mappings
		WHERE 1=1
	`
	args := []interface{}{}

	if mappingName != "" {
		query += " AND mapping_name = $1"
		args = append(args, mappingName)
	}
	if hl7Table != "" {
		if len(args) > 0 {
			query += " AND hl7_table = $2"
		} else {
			query += " AND hl7_table = $1"
		}
		args = append(args, hl7Table)
	}

	query += " ORDER BY mapping_name, hl7_value"

	dbCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := c.db.QueryContext(dbCtx, query, args...)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to query value set mappings",
		})
		return
	}
	defer rows.Close()

	var mappings []map[string]interface{}
	for rows.Next() {
		var mappingName, hl7Table, hl7Value, fhirSystem, fhirCode, fhirDisplay, mappingType string

		err := rows.Scan(&mappingName, &hl7Table, &hl7Value, &fhirSystem, &fhirCode, &fhirDisplay, &mappingType)
		if err != nil {
			continue
		}

		mappings = append(mappings, map[string]interface{}{
			"mappingName": mappingName,
			"hl7Table":    hl7Table,
			"hl7Value":    hl7Value,
			"fhirSystem":  fhirSystem,
			"fhirCode":    fhirCode,
			"fhirDisplay": fhirDisplay,
			"mappingType": mappingType,
		})
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success":  true,
		"mappings": mappings,
		"count":    len(mappings),
	})
}

// =====================================
// ANALYTICS AND MONITORING
// =====================================

// GET /api/fhir/transform/analytics
func (c *HL7FHIRTransformationController) GetTransformationAnalytics(ctx *gin.Context) {
	// For now, return basic analytics - can be enhanced with actual logging
	analytics := map[string]interface{}{
		"totalTransformations":  0,
		"successRate":           "100%",
		"averageProcessingTime": "250ms",
		"topMessageTypes": []map[string]interface{}{
			{"messageType": "ADT^A01", "count": 150, "percentage": 45},
			{"messageType": "ORU^R01", "count": 100, "percentage": 30},
			{"messageType": "ORM^O01", "count": 80, "percentage": 25},
		},
		"resourceTypeCounts": map[string]int{
			"Patient":          230,
			"Encounter":        180,
			"Observation":      120,
			"DiagnosticReport": 100,
		},
		"lastUpdated": time.Now().Format(time.RFC3339),
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success":   true,
		"analytics": analytics,
	})
}

// GET /api/fhir/transform/logs
func (c *HL7FHIRTransformationController) GetTransformationLogs(ctx *gin.Context) {
	// Basic log structure - implement actual logging as needed
	logs := []map[string]interface{}{
		{
			"requestId":        "transform_123456789",
			"messageType":      "ADT^A01",
			"status":           "success",
			"resourcesCreated": 5,
			"processingTime":   "245ms",
			"timestamp":        time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
		},
		{
			"requestId":        "transform_123456788",
			"messageType":      "ORU^R01",
			"status":           "success",
			"resourcesCreated": 8,
			"processingTime":   "312ms",
			"timestamp":        time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
		},
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"logs":    logs,
		"count":   len(logs),
	})
}

// =====================================
// HELPER METHODS
// =====================================

// lookupValidationPolicy fetches fhir_validation_policy for the given interface.
// Returns "proceed" on any error so the transform is never silently blocked.
func (c *HL7FHIRTransformationController) lookupValidationPolicy(ctx context.Context, interfaceID string) string {
	var policy sql.NullString
	err := c.db.QueryRowContext(ctx,
		`SELECT fhir_validation_policy FROM interfaces WHERE id = $1`, interfaceID,
	).Scan(&policy)
	if err != nil || !policy.Valid || policy.String == "" {
		return "proceed"
	}
	return policy.String
}

// enqueueForReview inserts a row into fhir_validation_queue so an operator can
// resolve the missing fields before the message is delivered.
func (c *HL7FHIRTransformationController) enqueueForReview(
	ctx context.Context,
	req *services.TransformRequest,
	resp *services.TransformResponse,
) error {
	failuresJSON, _ := json.Marshal(resp.ValidationIssues)
	partialFHIRJSON, _ := json.Marshal(resp.FHIRResources)

	var rawHL7 *string
	if raw, ok := req.ParsedHL7Data["raw"].(string); ok && raw != "" {
		rawHL7 = &raw
	}
	var msgID *string
	if mid, ok := req.ParsedHL7Data["messageId"].(string); ok && mid != "" {
		msgID = &mid
	}
	msgType := resp.MessageType
	if msgType == "" {
		msgType = req.MessageType
	}

	_, err := c.db.ExecContext(ctx, `
		INSERT INTO fhir_validation_queue
			(interface_id, message_id, message_type, raw_hl7, partial_fhir, validation_failures, status)
		VALUES ($1, $2, $3, $4, $5, $6, 'pending_review')`,
		req.InterfaceID,
		msgID,
		nullableString(msgType),
		rawHL7,
		string(partialFHIRJSON),
		string(failuresJSON),
	)
	return err
}

// attachOperationOutcome appends an OperationOutcome FHIR resource to the
// response bundle describing every validation failure.  Used for policy=warn.
func (c *HL7FHIRTransformationController) attachOperationOutcome(resp *services.TransformResponse) {
	issues := make([]map[string]interface{}, 0, len(resp.ValidationIssues))
	for _, vf := range resp.ValidationIssues {
		issues = append(issues, map[string]interface{}{
			"severity":    vf.Severity,
			"code":        vf.Code,
			"diagnostics": fmt.Sprintf("%s (path: %s)", vf.Message, vf.Path),
			"details": map[string]interface{}{
				"coding": []map[string]interface{}{{
					"system":  "https://ezhealthkonnect.io/fhir/validation-codes",
					"code":    vf.Code,
					"display": vf.Path,
				}},
			},
		})
	}
	oo := map[string]interface{}{
		"resourceType": "OperationOutcome",
		"id":           fmt.Sprintf("validation-warn-%d", time.Now().UnixNano()),
		"issue":        issues,
	}
	resp.FHIRResources = append(resp.FHIRResources, oo)
	if resp.Bundle != nil {
		if entries, ok := resp.Bundle["entry"].([]interface{}); ok {
			resp.Bundle["entry"] = append(entries, map[string]interface{}{
				"resource": oo,
				"search":   map[string]interface{}{"mode": "outcome"},
			})
		}
	}
}

// nullableString converts an empty string to nil for SQL nullable columns.
func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func (c *HL7FHIRTransformationController) hasErrorSeverity(issues []map[string]interface{}) bool {
	for _, issue := range issues {
		if severity, ok := issue["severity"].(string); ok && severity == "error" {
			return true
		}
	}
	return false
}

func (c *HL7FHIRTransformationController) generateExampleParsedHL7(messageType string) map[string]interface{} {
	// Generate example based on the structure shown in the parsedhl7.json
	switch messageType {
	case "ADT^A01", "ADT^A04":
		return map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"messageType": map[string]interface{}{
					"code":        "ADT",
					"event":       "A01",
					"name":        messageType,
					"description": "Admit/visit notification",
				},
				"enhancedSegments": map[string]interface{}{
					"MSH": map[string]interface{}{
						"key":  "MSH",
						"name": "MSH",
						"fields": []interface{}{
							map[string]interface{}{
								"key":   "MSH.3",
								"value": "SENDING_APPLICATION",
								"name":  "Sending Application",
							},
							map[string]interface{}{
								"key":   "MSH.7",
								"value": "20231215143022",
								"name":  "Date/Time of Message",
							},
							map[string]interface{}{
								"key":   "MSH.9",
								"value": messageType,
								"name":  "Message Type",
							},
							map[string]interface{}{
								"key":   "MSH.10",
								"value": "12345678",
								"name":  "Message Control ID",
							},
						},
					},
					"PID": map[string]interface{}{
						"key":  "PID",
						"name": "PID",
						"fields": []interface{}{
							map[string]interface{}{
								"key":   "PID.3",
								"value": "12345^^^MR^MR",
								"name":  "Patient Identifier List",
							},
							map[string]interface{}{
								"key":   "PID.5",
								"value": "Doe^John^M",
								"name":  "Patient Name",
							},
							map[string]interface{}{
								"key":   "PID.7",
								"value": "19900101",
								"name":  "Date of Birth",
							},
							map[string]interface{}{
								"key":   "PID.8",
								"value": "M",
								"name":  "Administrative Sex",
							},
							map[string]interface{}{
								"key":   "PID.11",
								"value": "123 Main St^^Springfield^IL^62701",
								"name":  "Patient Address",
							},
							map[string]interface{}{
								"key":   "PID.13",
								"value": "555-1234^^^john.doe@email.com",
								"name":  "Phone Number - Home",
							},
						},
					},
					"PV1": map[string]interface{}{
						"key":  "PV1",
						"name": "PV1",
						"fields": []interface{}{
							map[string]interface{}{
								"key":   "PV1.2",
								"value": "I",
								"name":  "Patient Class",
							},
							map[string]interface{}{
								"key":   "PV1.3",
								"value": "ICU^101^A",
								"name":  "Assigned Patient Location",
							},
							map[string]interface{}{
								"key":   "PV1.44",
								"value": "20231215140000",
								"name":  "Admit Date/Time",
							},
						},
					},
				},
			},
		}
	case "ORU^R01":
		return map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"messageType": map[string]interface{}{
					"code":        "ORU",
					"event":       "R01",
					"name":        messageType,
					"description": "Observation result",
				},
				"enhancedSegments": map[string]interface{}{
					"MSH": map[string]interface{}{
						"key":  "MSH",
						"name": "MSH",
						"fields": []interface{}{
							map[string]interface{}{
								"key":   "MSH.3",
								"value": "LAB_SYSTEM",
								"name":  "Sending Application",
							},
							map[string]interface{}{
								"key":   "MSH.9",
								"value": messageType,
								"name":  "Message Type",
							},
						},
					},
					"PID": map[string]interface{}{
						"key":  "PID",
						"name": "PID",
						"fields": []interface{}{
							map[string]interface{}{
								"key":   "PID.3",
								"value": "67890^^^MR^MR",
								"name":  "Patient Identifier List",
							},
							map[string]interface{}{
								"key":   "PID.5",
								"value": "Smith^Jane^A",
								"name":  "Patient Name",
							},
						},
					},
					"OBR": map[string]interface{}{
						"key":  "OBR",
						"name": "OBR",
						"fields": []interface{}{
							map[string]interface{}{
								"key":   "OBR.4",
								"value": "CBC^Complete Blood Count^LN",
								"name":  "Universal Service Identifier",
							},
						},
					},
					"OBX": map[string]interface{}{
						"key":  "OBX",
						"name": "OBX",
						"fields": []interface{}{
							map[string]interface{}{
								"key":   "OBX.3",
								"value": "WBC^White Blood Cell Count^LN",
								"name":  "Observation Identifier",
							},
							map[string]interface{}{
								"key":   "OBX.5",
								"value": "7.5",
								"name":  "Observation Value",
							},
							map[string]interface{}{
								"key":   "OBX.11",
								"value": "F",
								"name":  "Observation Result Status",
							},
						},
					},
				},
			},
		}
	default:
		// Generic example
		return map[string]interface{}{
			"success": true,
			"data": map[string]interface{}{
				"messageType": map[string]interface{}{
					"name": messageType,
				},
				"enhancedSegments": map[string]interface{}{
					"MSH": map[string]interface{}{
						"key":  "MSH",
						"name": "MSH",
						"fields": []interface{}{
							map[string]interface{}{
								"key":   "MSH.9",
								"value": messageType,
								"name":  "Message Type",
							},
						},
					},
					"PID": map[string]interface{}{
						"key":  "PID",
						"name": "PID",
						"fields": []interface{}{
							map[string]interface{}{
								"key":   "PID.3",
								"value": "TEST123",
								"name":  "Patient Identifier List",
							},
						},
					},
				},
			},
		}
	}
}
