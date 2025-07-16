// fhir/transformer/rule_engine.go
package transformer

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"ezHealthKonnect/fhir/builder"
	"ezHealthKonnect/fhir/engine"
	"ezHealthKonnect/hl7"
)

// =====================================
// CONFIGURATION-DRIVEN RULE ENGINE
// =====================================

type RuleEngine struct {
	db               *sql.DB
	schemaEngine     *engine.FHIRSchemaEngine
	fhirBuilder      *builder.FHIRBuilder
	cache            *RuleCache
	transformPlugins map[string]TransformPlugin
	validators       map[string]ValidationPlugin
	config           *RuleEngineConfig
}

type RuleEngineConfig struct {
	CacheEnabled     bool          `json:"cacheEnabled"`
	CacheTTL         time.Duration `json:"cacheTTL"`
	StrictValidation bool          `json:"strictValidation"`
	EnableAuditLog   bool          `json:"enableAuditLog"`
	DefaultProfile   string        `json:"defaultProfile"`
	MaxCacheSize     int           `json:"maxCacheSize"`
	PluginDirectory  string        `json:"pluginDirectory"`
}

// =====================================
// RULE DEFINITIONS (FROM DATABASE)
// =====================================

type TransformationRule struct {
	ID             int                    `json:"id" db:"id"`
	HL7Version     string                 `json:"hl7_version" db:"hl7_version"`
	HL7MessageType string                 `json:"hl7_message_type" db:"hl7_message_type"`
	HL7Segment     string                 `json:"hl7_segment" db:"hl7_segment"`
	HL7Field       string                 `json:"hl7_field" db:"hl7_field"`
	HL7Component   *string                `json:"hl7_component" db:"hl7_component"`
	FHIRResource   string                 `json:"fhir_resource" db:"fhir_resource"`
	FHIRProfile    string                 `json:"fhir_profile" db:"fhir_profile"`
	FHIRPath       string                 `json:"fhir_path" db:"fhir_path"`
	TransformRule  map[string]interface{} `json:"transformation_rule" db:"transformation_rule"`
	ConditionExpr  *string                `json:"condition_expression" db:"condition_expression"`
	IsRequired     bool                   `json:"is_required" db:"is_required"`
	Priority       int                    `json:"priority" db:"priority"`
	CreatedAt      time.Time              `json:"created_at" db:"created_at"`
}

type ValueSetMapping struct {
	ID           int                    `json:"id"`
	ElementPath  string                 `json:"element_path"`
	ValueSetURL  string                 `json:"value_set_url"`
	Strength     string                 `json:"strength"`
	CodeMappings map[string]interface{} `json:"code_mappings"`
	IsActive     bool                   `json:"is_active"`
}

type CustomTransformation struct {
	ID              int                    `json:"id"`
	Name            string                 `json:"name"`
	SourceDataType  string                 `json:"source_data_type"`
	TargetDataType  string                 `json:"target_data_type"`
	TransformLogic  map[string]interface{} `json:"transform_logic"`
	ValidationRules map[string]interface{} `json:"validation_rules"`
	IsActive        bool                   `json:"is_active"`
}

// =====================================
// PLUGIN ARCHITECTURE
// =====================================

type TransformPlugin interface {
	GetName() string
	GetVersion() string
	GetSupportedTypes() []TypeMapping
	Transform(input interface{}, params map[string]interface{}) (interface{}, error)
	Validate(input interface{}, output interface{}) []ValidationResult
}

type ValidationPlugin interface {
	GetName() string
	GetSupportedPaths() []string
	Validate(value interface{}, context ValidationContext) []ValidationResult
}

type TypeMapping struct {
	SourceType string `json:"sourceType"`
	TargetType string `json:"targetType"`
}

type ValidationContext struct {
	ElementPath  string                 `json:"elementPath"`
	FHIRProfile  string                 `json:"fhirProfile"`
	HL7Version   string                 `json:"hl7Version"`
	MessageType  string                 `json:"messageType"`
	CustomParams map[string]interface{} `json:"customParams"`
}

// =====================================
// RULE CACHE SYSTEM
// =====================================

type RuleCache struct {
	rules           map[string][]*TransformationRule `json:"rules"`
	valueSets       map[string]*ValueSetMapping      `json:"valueSets"`
	transformations map[string]*CustomTransformation `json:"transformations"`
	lastUpdated     map[string]time.Time             `json:"lastUpdated"`
	ttl             time.Duration                    `json:"ttl"`
	maxSize         int                              `json:"maxSize"`
	hitCount        int64                            `json:"hitCount"`
	missCount       int64                            `json:"missCount"`
}

// =====================================
// TRANSFORMATION EXECUTION
// =====================================

type TransformationRequest struct {
	HL7Message     *hl7.EnhancedParsedMessage `json:"hl7Message"`
	TargetProfile  string                     `json:"targetProfile"`
	ValidationMode string                     `json:"validationMode"` // strict | lenient | off
	CustomParams   map[string]interface{}     `json:"customParams"`
	RequestID      string                     `json:"requestId"`
	SourceSystem   string                     `json:"sourceSystem"`
}

type TransformationResponse struct {
	Success           bool                    `json:"success"`
	RequestID         string                  `json:"requestId"`
	Resources         []builder.FHIRResource  `json:"resources"`
	Bundle            *builder.FHIRBundle     `json:"bundle,omitempty"`
	Warnings          []TransformationWarning `json:"warnings"`
	Errors            []TransformationError   `json:"errors"`
	ValidationResults []ValidationResult      `json:"validationResults"`
	Metadata          *TransformationMetadata `json:"metadata"`
	Performance       *PerformanceMetrics     `json:"performance"`
}

type PerformanceMetrics struct {
	TotalTime      time.Duration `json:"totalTime"`
	RuleLoadTime   time.Duration `json:"ruleLoadTime"`
	TransformTime  time.Duration `json:"transformTime"`
	ValidationTime time.Duration `json:"validationTime"`
	RulesApplied   int           `json:"rulesApplied"`
	CacheHits      int           `json:"cacheHits"`
	CacheMisses    int           `json:"cacheMisses"`
}

type ValidationResult struct {
	ElementPath    string `json:"elementPath"`
	Severity       string `json:"severity"`
	Code           string `json:"code"`
	Message        string `json:"message"`
	ExpectedValue  string `json:"expectedValue,omitempty"`
	ActualValue    string `json:"actualValue,omitempty"`
	ValidationRule string `json:"validationRule"`
}

// =====================================
// INITIALIZATION
// =====================================

func NewRuleEngine(db *sql.DB, schemaEngine *engine.FHIRSchemaEngine, fhirBuilder *builder.FHIRBuilder, config *RuleEngineConfig) *RuleEngine {
	if config == nil {
		config = &RuleEngineConfig{
			CacheEnabled:     true,
			CacheTTL:         30 * time.Minute,
			StrictValidation: true,
			EnableAuditLog:   true,
			DefaultProfile:   "us-core",
			MaxCacheSize:     10000,
		}
	}

	engine := &RuleEngine{
		db:               db,
		schemaEngine:     schemaEngine,
		fhirBuilder:      fhirBuilder,
		cache:            NewRuleCache(config.MaxCacheSize, config.CacheTTL),
		transformPlugins: make(map[string]TransformPlugin),
		validators:       make(map[string]ValidationPlugin),
		config:           config,
	}

	// Load built-in transform plugins
	engine.loadBuiltInPlugins()

	return engine
}

func NewRuleCache(maxSize int, ttl time.Duration) *RuleCache {
	return &RuleCache{
		rules:           make(map[string][]*TransformationRule),
		valueSets:       make(map[string]*ValueSetMapping),
		transformations: make(map[string]*CustomTransformation),
		lastUpdated:     make(map[string]time.Time),
		ttl:             ttl,
		maxSize:         maxSize,
		hitCount:        0,
		missCount:       0,
	}
}

// =====================================
// CORE TRANSFORMATION LOGIC
// =====================================

func (re *RuleEngine) Transform(ctx context.Context, request *TransformationRequest) (*TransformationResponse, error) {
	startTime := time.Now()

	response := &TransformationResponse{
		RequestID:         request.RequestID,
		Success:           false,
		Resources:         []builder.FHIRResource{},
		Warnings:          []TransformationWarning{},
		Errors:            []TransformationError{},
		ValidationResults: []ValidationResult{},
		Performance:       &PerformanceMetrics{},
	}

	// 1. Load transformation rules for this message type
	ruleLoadStart := time.Now()
	rules, err := re.loadTransformationRules(ctx, request.HL7Message.MessageType.Code, request.HL7Message.MessageType.Event, request.TargetProfile)
	if err != nil {
		response.Errors = append(response.Errors, TransformationError{
			Severity: "error",
			Code:     "RULE_LOAD_FAILED",
			Message:  fmt.Sprintf("Failed to load transformation rules: %v", err),
		})
		return response, err
	}
	response.Performance.RuleLoadTime = time.Since(ruleLoadStart)
	response.Performance.RulesApplied = len(rules)

	// 2. Execute transformations
	transformStart := time.Now()
	resources, warnings, errors := re.executeTransformations(ctx, request.HL7Message, rules, request.CustomParams)
	response.Resources = resources
	response.Warnings = append(response.Warnings, warnings...)
	response.Errors = append(response.Errors, errors...)
	response.Performance.TransformTime = time.Since(transformStart)

	// 3. Perform validation if enabled
	if request.ValidationMode != "off" {
		validationStart := time.Now()
		validationResults := re.validateResources(ctx, resources, request.TargetProfile, request.ValidationMode == "strict")
		response.ValidationResults = validationResults
		response.Performance.ValidationTime = time.Since(validationStart)
	}

	// 4. Create bundle if requested
	if len(resources) > 1 {
		bundle, err := re.createBundle(resources, request)
		if err != nil {
			response.Warnings = append(response.Warnings, TransformationWarning{
				Severity: "warning",
				Code:     "BUNDLE_CREATION_FAILED",
				Message:  fmt.Sprintf("Failed to create bundle: %v", err),
			})
		} else {
			response.Bundle = bundle
		}
	}

	// 5. Set metadata and performance metrics
	response.Performance.TotalTime = time.Since(startTime)
	response.Performance.CacheHits = int(re.cache.hitCount)
	response.Performance.CacheMisses = int(re.cache.missCount)

	response.Metadata = &TransformationMetadata{
		SourceMessage:    fmt.Sprintf("%s^%s", request.HL7Message.MessageType.Code, request.HL7Message.MessageType.Event),
		TargetProfile:    request.TargetProfile,
		TransformTime:    time.Now(),
		MappingVersion:   "database-driven",
		ResourcesCreated: len(resources),
	}

	response.Success = len(response.Errors) == 0

	return response, nil
}

// =====================================
// RULE LOADING (DATABASE-DRIVEN)
// =====================================

func (re *RuleEngine) loadTransformationRules(ctx context.Context, messageCode, eventCode, profile string) ([]*TransformationRule, error) {
	messageType := fmt.Sprintf("%s^%s", messageCode, eventCode)
	cacheKey := fmt.Sprintf("rules:%s:%s", messageType, profile)

	// Check cache first
	if re.config.CacheEnabled {
		if rules, exists := re.cache.rules[cacheKey]; exists {
			if time.Since(re.cache.lastUpdated[cacheKey]) < re.cache.ttl {
				re.cache.hitCount++
				return rules, nil
			}
		}
		re.cache.missCount++
	}

	// Load from database
	query := `
		SELECT id, hl7_version, hl7_message_type, hl7_segment, hl7_field, hl7_component,
		       fhir_resource, fhir_profile, fhir_path, transformation_rule,
		       condition_expression, is_required, priority, created_at
		FROM hl7_fhir_mappings 
		WHERE hl7_message_type = $1 
		  AND (fhir_profile = $2 OR fhir_profile = 'base')
		ORDER BY priority ASC, id ASC
	`

	rows, err := re.db.QueryContext(ctx, query, messageType, profile)
	if err != nil {
		return nil, fmt.Errorf("failed to query transformation rules: %w", err)
	}
	defer rows.Close()

	var rules []*TransformationRule
	for rows.Next() {
		rule := &TransformationRule{}
		var transformRuleJSON []byte

		err := rows.Scan(
			&rule.ID, &rule.HL7Version, &rule.HL7MessageType, &rule.HL7Segment,
			&rule.HL7Field, &rule.HL7Component, &rule.FHIRResource, &rule.FHIRProfile,
			&rule.FHIRPath, &transformRuleJSON, &rule.ConditionExpr, &rule.IsRequired,
			&rule.Priority, &rule.CreatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan rule: %w", err)
		}

		// Parse JSONB transformation rule
		if len(transformRuleJSON) > 0 {
			if err := json.Unmarshal(transformRuleJSON, &rule.TransformRule); err != nil {
				return nil, fmt.Errorf("failed to parse transformation rule JSON: %w", err)
			}
		}

		rules = append(rules, rule)
	}

	// Cache the results
	if re.config.CacheEnabled {
		re.cache.rules[cacheKey] = rules
		re.cache.lastUpdated[cacheKey] = time.Now()
	}

	return rules, nil
}

// =====================================
// TRANSFORMATION EXECUTION
// =====================================

func (re *RuleEngine) executeTransformations(ctx context.Context, hl7Message *hl7.EnhancedParsedMessage, rules []*TransformationRule, customParams map[string]interface{}) ([]builder.FHIRResource, []TransformationWarning, []TransformationError) {
	var resources []builder.FHIRResource
	var warnings []TransformationWarning
	var errors []TransformationError

	// Group rules by target resource
	resourceRules := make(map[string][]*TransformationRule)
	for _, rule := range rules {
		resourceRules[rule.FHIRResource] = append(resourceRules[rule.FHIRResource], rule)
	}

	// Create resources and apply rules
	for resourceType, rulesForResource := range resourceRules {
		resource, resWarnings, resErrors := re.createResourceWithRules(ctx, hl7Message, resourceType, rulesForResource, customParams)
		if resource != nil {
			resources = append(resources, resource)
		}
		warnings = append(warnings, resWarnings...)
		errors = append(errors, resErrors...)
	}

	return resources, warnings, errors
}

func (re *RuleEngine) createResourceWithRules(ctx context.Context, hl7Message *hl7.EnhancedParsedMessage, resourceType string, rules []*TransformationRule, customParams map[string]interface{}) (builder.FHIRResource, []TransformationWarning, []TransformationError) {
	var warnings []TransformationWarning
	var errors []TransformationError

	// Create resource builder
	profile := "base"
	if len(rules) > 0 && rules[0].FHIRProfile != "" {
		profile = rules[0].FHIRProfile
	}

	resourceBuilder := re.fhirBuilder.NewResource(resourceType).WithProfile(profile)

	// Apply each rule
	for _, rule := range rules {
		// Check condition if exists
		if rule.ConditionExpr != nil && *rule.ConditionExpr != "" {
			if !re.evaluateCondition(*rule.ConditionExpr, hl7Message, customParams) {
				continue
			}
		}

		// Get source value from HL7 message
		sourceValue, err := re.extractHL7Value(hl7Message, rule.HL7Segment, rule.HL7Field, rule.HL7Component)
		if err != nil {
			if rule.IsRequired {
				errors = append(errors, TransformationError{
					Severity: "error",
					Code:     "REQUIRED_FIELD_MISSING",
					Message:  fmt.Sprintf("Required field %s.%s not found", rule.HL7Segment, rule.HL7Field),
					HL7Field: fmt.Sprintf("%s.%s", rule.HL7Segment, rule.HL7Field),
					FHIRPath: rule.FHIRPath,
				})
			}
			continue
		}

		// Transform the value
		transformedValue, err := re.transformValue(sourceValue, rule.TransformRule, customParams)
		if err != nil {
			errors = append(errors, TransformationError{
				Severity: "error",
				Code:     "TRANSFORMATION_FAILED",
				Message:  fmt.Sprintf("Failed to transform %s.%s: %v", rule.HL7Segment, rule.HL7Field, err),
				HL7Field: fmt.Sprintf("%s.%s", rule.HL7Segment, rule.HL7Field),
				FHIRPath: rule.FHIRPath,
			})
			continue
		}

		// Set the value in FHIR resource
		err = resourceBuilder.SetField(rule.FHIRPath, transformedValue)
		if err != nil {
			warnings = append(warnings, TransformationWarning{
				Severity: "warning",
				Code:     "FIELD_SET_FAILED",
				Message:  fmt.Sprintf("Failed to set field %s: %v", rule.FHIRPath, err),
				FHIRPath: rule.FHIRPath,
			})
		}
	}

	// Build the resource
	resource, validationErrors, err := resourceBuilder.Build()
	if err != nil {
		errors = append(errors, TransformationError{
			Severity: "error",
			Code:     "RESOURCE_BUILD_FAILED",
			Message:  fmt.Sprintf("Failed to build %s resource: %v", resourceType, err),
		})
		return nil, warnings, errors
	}

	// Handle validation errors
	for _, vErr := range validationErrors {
		if vErr.Severity == "error" {
			errors = append(errors, TransformationError{
				Severity: vErr.Severity,
				Code:     vErr.Code,
				Message:  vErr.Message,
				FHIRPath: vErr.Path,
			})
		} else {
			warnings = append(warnings, TransformationWarning{
				Severity: vErr.Severity,
				Code:     vErr.Code,
				Message:  vErr.Message,
				FHIRPath: vErr.Path,
			})
		}
	}

	return resource, warnings, errors
}

// =====================================
// VALUE TRANSFORMATION LOGIC
// =====================================

func (re *RuleEngine) transformValue(sourceValue interface{}, transformRule map[string]interface{}, customParams map[string]interface{}) (interface{}, error) {
	transformType, exists := transformRule["type"].(string)
	if !exists {
		return sourceValue, nil // No transformation specified
	}

	switch transformType {
	case "direct":
		return sourceValue, nil

	case "code_map":
		return re.transformCodeMap(sourceValue, transformRule)

	case "hl7_name":
		return re.transformHL7Name(sourceValue, transformRule)

	case "hl7_identifier":
		return re.transformHL7Identifier(sourceValue, transformRule)

	case "hl7_date":
		return re.transformHL7Date(sourceValue, transformRule)

	case "hl7_datetime":
		return re.transformHL7DateTime(sourceValue, transformRule)

	case "hl7_coded_element":
		return re.transformHL7CodedElement(sourceValue, transformRule)

	case "custom":
		pluginName, ok := transformRule["plugin"].(string)
		if !ok {
			return nil, fmt.Errorf("custom transformation requires plugin name")
		}
		return re.executeCustomTransformation(pluginName, sourceValue, transformRule, customParams)

	default:
		return nil, fmt.Errorf("unknown transformation type: %s", transformType)
	}
}

func (re *RuleEngine) transformCodeMap(sourceValue interface{}, rule map[string]interface{}) (interface{}, error) {
	codeMap, exists := rule["map"].(map[string]interface{})
	if !exists {
		return sourceValue, nil
	}

	sourceStr := fmt.Sprintf("%v", sourceValue)
	if mappedValue, exists := codeMap[sourceStr]; exists {
		return mappedValue, nil
	}

	// Check for default value
	if defaultValue, exists := rule["default"]; exists {
		return defaultValue, nil
	}

	return sourceValue, nil
}

func (re *RuleEngine) transformHL7Name(sourceValue interface{}, rule map[string]interface{}) (interface{}, error) {
	// Transform HL7 XPN to FHIR HumanName
	sourceStr := fmt.Sprintf("%v", sourceValue)
	components := strings.Split(sourceStr, "^")

	humanName := map[string]interface{}{}

	if len(components) > 0 && components[0] != "" {
		humanName["family"] = components[0]
	}
	if len(components) > 1 && components[1] != "" {
		humanName["given"] = []string{components[1]}
	}
	if len(components) > 2 && components[2] != "" {
		if given, exists := humanName["given"].([]string); exists {
			humanName["given"] = append(given, components[2])
		} else {
			humanName["given"] = []string{components[2]}
		}
	}

	// Add text representation
	var nameParts []string
	if given, exists := humanName["given"]; exists {
		if givenSlice, ok := given.([]string); ok {
			nameParts = append(nameParts, strings.Join(givenSlice, " "))
		}
	}
	if family, exists := humanName["family"]; exists {
		nameParts = append(nameParts, fmt.Sprintf("%v", family))
	}
	if len(nameParts) > 0 {
		humanName["text"] = strings.Join(nameParts, " ")
	}

	return humanName, nil
}

// =====================================
// UTILITY FUNCTIONS
// =====================================

func (re *RuleEngine) extractHL7Value(hl7Message *hl7.EnhancedParsedMessage, segmentCode, fieldCode string, componentCode *string) (interface{}, error) {
	// Find the segment
	for _, segment := range hl7Message.Segments {
		if segment.SegmentType == segmentCode {
			// Find the field
			for _, field := range segment.Fields {
				if field.Key == fieldCode {
					if componentCode == nil {
						return field.Value, nil
					}
					// Extract component if specified
					components := strings.Split(field.Value, "^")
					if componentIndex, err := parseComponentIndex(*componentCode); err == nil && componentIndex < len(components) {
						return components[componentIndex], nil
					}
				}
			}
		}
	}
	return nil, fmt.Errorf("field %s.%s not found", segmentCode, fieldCode)
}

func parseComponentIndex(component string) (int, error) {
	// Convert component like "1" to zero-based index
	if idx := strings.TrimSpace(component); idx != "" {
		if i, err := strconv.Atoi(idx); err == nil && i > 0 {
			return i - 1, nil
		}
	}
	return -1, fmt.Errorf("invalid component: %s", component)
}

func (re *RuleEngine) evaluateCondition(condition string, hl7Message *hl7.EnhancedParsedMessage, customParams map[string]interface{}) bool {
	// Simple condition evaluation - this would be expanded with a proper expression evaluator
	// For now, support basic conditions like "PV1 segment exists"
	if strings.Contains(condition, "segment exists") {
		segmentType := strings.Fields(condition)[0]
		for _, segment := range hl7Message.Segments {
			if segment.SegmentType == segmentType {
				return true
			}
		}
		return false
	}
	return true // Default to true for unknown conditions
}

// =====================================
// PLUGIN MANAGEMENT
// =====================================

func (re *RuleEngine) RegisterPlugin(plugin TransformPlugin) {
	re.transformPlugins[plugin.GetName()] = plugin
}

func (re *RuleEngine) RegisterValidator(validator ValidationPlugin) {
	re.validators[validator.GetName()] = validator
}

func (re *RuleEngine) executeCustomTransformation(pluginName string, sourceValue interface{}, rule map[string]interface{}, customParams map[string]interface{}) (interface{}, error) {
	plugin, exists := re.transformPlugins[pluginName]
	if !exists {
		return nil, fmt.Errorf("plugin not found: %s", pluginName)
	}

	// Merge rule parameters with custom parameters
	params := make(map[string]interface{})
	for k, v := range customParams {
		params[k] = v
	}
	if ruleParams, exists := rule["parameters"].(map[string]interface{}); exists {
		for k, v := range ruleParams {
			params[k] = v
		}
	}

	return plugin.Transform(sourceValue, params)
}

func (re *RuleEngine) loadBuiltInPlugins() {
	// Register built-in transformation plugins
	// This would load plugins from the configured plugin directory
}

// =====================================
// CONFIGURATION MANAGEMENT APIS
// =====================================

func (re *RuleEngine) AddTransformationRule(ctx context.Context, rule *TransformationRule) error {
	transformRuleJSON, err := json.Marshal(rule.TransformRule)
	if err != nil {
		return fmt.Errorf("failed to marshal transformation rule: %w", err)
	}

	query := `
		INSERT INTO hl7_fhir_mappings 
		(hl7_version, hl7_message_type, hl7_segment, hl7_field, hl7_component,
		 fhir_resource, fhir_profile, fhir_path, transformation_rule,
		 condition_expression, is_required, priority)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
		RETURNING id
	`

	err = re.db.QueryRowContext(ctx, query,
		rule.HL7Version, rule.HL7MessageType, rule.HL7Segment, rule.HL7Field, rule.HL7Component,
		rule.FHIRResource, rule.FHIRProfile, rule.FHIRPath, transformRuleJSON,
		rule.ConditionExpr, rule.IsRequired, rule.Priority).Scan(&rule.ID)

	if err != nil {
		return fmt.Errorf("failed to insert transformation rule: %w", err)
	}

	// Invalidate cache
	re.invalidateCache()
	return nil
}

func (re *RuleEngine) UpdateTransformationRule(ctx context.Context, id int, rule *TransformationRule) error {
	transformRuleJSON, err := json.Marshal(rule.TransformRule)
	if err != nil {
		return fmt.Errorf("failed to marshal transformation rule: %w", err)
	}

	query := `
		UPDATE hl7_fhir_mappings 
		SET hl7_version = $2, hl7_message_type = $3, hl7_segment = $4, 
		    hl7_field = $5, hl7_component = $6, fhir_resource = $7, 
		    fhir_profile = $8, fhir_path = $9, transformation_rule = $10,
		    condition_expression = $11, is_required = $12, priority = $13
		WHERE id = $1
	`

	_, err = re.db.ExecContext(ctx, query, id,
		rule.HL7Version, rule.HL7MessageType, rule.HL7Segment, rule.HL7Field, rule.HL7Component,
		rule.FHIRResource, rule.FHIRProfile, rule.FHIRPath, transformRuleJSON,
		rule.ConditionExpr, rule.IsRequired, rule.Priority)

	if err != nil {
		return fmt.Errorf("failed to update transformation rule: %w", err)
	}

	// Invalidate cache
	re.invalidateCache()
	return nil
}

func (re *RuleEngine) DeleteTransformationRule(ctx context.Context, id int) error {
	query := `DELETE FROM hl7_fhir_mappings WHERE id = $1`
	_, err := re.db.ExecContext(ctx, query, id)
	if err != nil {
		return fmt.Errorf("failed to delete transformation rule: %w", err)
	}

	// Invalidate cache
	re.invalidateCache()
	return nil
}

func (re *RuleEngine) invalidateCache() {
	re.cache.rules = make(map[string][]*TransformationRule)
	re.cache.lastUpdated = make(map[string]time.Time)
}

// =====================================
// REMAINING TRANSFORMATION IMPLEMENTATIONS
// =====================================

func (re *RuleEngine) transformHL7Identifier(sourceValue interface{}, rule map[string]interface{}) (interface{}, error) {
	// Transform HL7 CX to FHIR Identifier
	sourceStr := fmt.Sprintf("%v", sourceValue)
	components := strings.Split(sourceStr, "^")

	identifier := map[string]interface{}{}

	if len(components) > 0 && components[0] != "" {
		identifier["value"] = components[0]
	}
	if len(components) > 3 && components[3] != "" {
		identifier["system"] = re.mapAssigningAuthority(components[3])
	}
	if len(components) > 4 && components[4] != "" {
		identifier["type"] = map[string]interface{}{
			"coding": []map[string]interface{}{{
				"system": "http://terminology.hl7.org/CodeSystem/v2-0203",
				"code":   components[4],
			}},
		}
	}

	return identifier, nil
}

func (re *RuleEngine) transformHL7Date(sourceValue interface{}, rule map[string]interface{}) (interface{}, error) {
	// Transform HL7 date (YYYYMMDD) to FHIR date (YYYY-MM-DD)
	sourceStr := strings.TrimSpace(fmt.Sprintf("%v", sourceValue))
	if len(sourceStr) >= 8 {
		year := sourceStr[0:4]
		month := sourceStr[4:6]
		day := sourceStr[6:8]
		return fmt.Sprintf("%s-%s-%s", year, month, day), nil
	}
	return sourceValue, nil
}

func (re *RuleEngine) transformHL7DateTime(sourceValue interface{}, rule map[string]interface{}) (interface{}, error) {
	// Transform HL7 timestamp to FHIR dateTime
	sourceStr := strings.TrimSpace(fmt.Sprintf("%v", sourceValue))
	if len(sourceStr) >= 8 {
		year := sourceStr[0:4]
		month := sourceStr[4:6]
		day := sourceStr[6:8]
		result := fmt.Sprintf("%s-%s-%s", year, month, day)

		if len(sourceStr) >= 14 {
			hour := sourceStr[8:10]
			minute := sourceStr[10:12]
			second := sourceStr[12:14]
			result += fmt.Sprintf("T%s:%s:%s", hour, minute, second)
		}

		return result, nil
	}
	return sourceValue, nil
}

func (re *RuleEngine) transformHL7CodedElement(sourceValue interface{}, rule map[string]interface{}) (interface{}, error) {
	// Transform HL7 CE to FHIR CodeableConcept
	sourceStr := fmt.Sprintf("%v", sourceValue)
	components := strings.Split(sourceStr, "^")

	concept := map[string]interface{}{}

	if len(components) > 1 && components[1] != "" {
		concept["text"] = components[1]
	}

	coding := []map[string]interface{}{}
	if len(components) > 0 && components[0] != "" {
		codingEntry := map[string]interface{}{
			"code": components[0],
		}
		if len(components) > 1 && components[1] != "" {
			codingEntry["display"] = components[1]
		}
		if len(components) > 2 && components[2] != "" {
			codingEntry["system"] = re.mapCodingSystem(components[2])
		}
		coding = append(coding, codingEntry)
	}

	if len(coding) > 0 {
		concept["coding"] = coding
	}

	return concept, nil
}

func (re *RuleEngine) mapAssigningAuthority(authority string) string {
	// This would typically come from a configuration table
	mapping := map[string]string{
		"HOSPITAL": "http://hospital.smarthealthit.org",
		"SSN":      "http://hl7.org/fhir/sid/us-ssn",
		"NPI":      "http://hl7.org/fhir/sid/us-npi",
	}
	if system, exists := mapping[authority]; exists {
		return system
	}
	return fmt.Sprintf("urn:oid:%s", authority)
}

func (re *RuleEngine) mapCodingSystem(hl7System string) string {
	// This would typically come from a configuration table
	mapping := map[string]string{
		"L":     "http://loinc.org",
		"LN":    "http://loinc.org",
		"SNM":   "http://snomed.info/sct",
		"ICD9":  "http://hl7.org/fhir/sid/icd-9-cm",
		"ICD10": "http://hl7.org/fhir/sid/icd-10-cm",
	}
	if system, exists := mapping[hl7System]; exists {
		return system
	}
	return hl7System
}

func (re *RuleEngine) validateResources(ctx context.Context, resources []builder.FHIRResource, profile string, strict bool) []ValidationResult {
	var results []ValidationResult
	// Implementation would validate resources against FHIR profiles
	// This is a placeholder for comprehensive validation logic
	return results
}

func (re *RuleEngine) createBundle(resources []builder.FHIRResource, request *TransformationRequest) (*builder.FHIRBundle, error) {
	// Implementation would create a FHIR Bundle from the resources
	// This is a placeholder for bundle creation logic
	return nil, nil
}
