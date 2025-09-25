// services/transformation_custom.go
// Custom Transformation Service for Universal Interface Engine
//
// 🎯 PURPOSE: User-defined transformation rules and custom logic
package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

// =====================================
// CUSTOM TRANSFORMATION SERVICE
// =====================================

type CustomTransformationService struct {
	db                 *sql.DB
	ruleEngine         *CustomRuleEngine
	scriptEngine       *ScriptEngine
	userDefinedRules   map[string]CustomRule
	performanceMetrics CustomPerformanceMetrics
}

type CustomPerformanceMetrics struct {
	RulesExecuted     int64         `json:"rulesExecuted"`
	AverageExecTime   time.Duration `json:"averageExecTime"`
	SuccessRate       float64       `json:"successRate"`
	CustomFunctions   int           `json:"customFunctions"`
}

type CustomRule struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Type        string                 `json:"type"` // script, mapping, validation, condition
	Language    string                 `json:"language"` // javascript, lua, regex, jsonpath
	Script      string                 `json:"script"`
	Conditions  []CustomCondition      `json:"conditions"`
	Actions     []CustomAction         `json:"actions"`
	Priority    int                    `json:"priority"`
	Enabled     bool                   `json:"enabled"`
	Metadata    map[string]interface{} `json:"metadata"`
	CreatedAt   time.Time              `json:"createdAt"`
	UpdatedAt   time.Time              `json:"updatedAt"`
}

type CustomCondition struct {
	Field    string      `json:"field"`
	Operator string      `json:"operator"`
	Value    interface{} `json:"value"`
	Logic    string      `json:"logic"` // AND, OR, NOT
}

type CustomAction struct {
	Type       string                 `json:"type"` // set, delete, transform, validate
	Target     string                 `json:"target"`
	Value      interface{}            `json:"value"`
	Function   string                 `json:"function,omitempty"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

type CustomRuleEngine struct {
	compiledRules map[string]*CompiledRule
	functions     map[string]CustomFunction
}

type CompiledRule struct {
	Rule        *CustomRule
	Compiled    interface{}
	CompiledAt  time.Time
}

type CustomFunction func(interface{}, map[string]interface{}) (interface{}, error)

type ScriptEngine struct {
	supportedLanguages map[string]LanguageExecutor
}

type LanguageExecutor interface {
	Execute(script string, context map[string]interface{}) (interface{}, error)
}

type CustomTransformRequest struct {
	MessageID   string                 `json:"messageId"`
	SourceData  map[string]interface{} `json:"sourceData"`
	RuleIDs     []string               `json:"ruleIds,omitempty"`
	Context     map[string]interface{} `json:"context,omitempty"`
	Options     map[string]interface{} `json:"options,omitempty"`
}

type CustomTransformResponse struct {
	Success           bool                   `json:"success"`
	MessageID         string                 `json:"messageId"`
	TransformedData   map[string]interface{} `json:"transformedData"`
	RulesExecuted     []RuleExecutionResult  `json:"rulesExecuted"`
	Warnings          []string               `json:"warnings"`
	Errors            []string               `json:"errors"`
	ProcessingMetrics ProcessingMetrics      `json:"processingMetrics"`
}

type RuleExecutionResult struct {
	RuleID      string        `json:"ruleId"`
	RuleName    string        `json:"ruleName"`
	Success     bool          `json:"success"`
	ExecutionTime time.Duration `json:"executionTime"`
	Changes     []string      `json:"changes"`
	Error       string        `json:"error,omitempty"`
}

// NewCustomTransformationService creates a new custom transformation service
func NewCustomTransformationService(database *sql.DB) *CustomTransformationService {
	service := &CustomTransformationService{
		db:               database,
		ruleEngine:       NewCustomRuleEngine(),
		scriptEngine:     NewScriptEngine(),
		userDefinedRules: make(map[string]CustomRule),
		performanceMetrics: CustomPerformanceMetrics{},
	}

	service.initializeDefaultRules()
	return service
}

func NewCustomRuleEngine() *CustomRuleEngine {
	engine := &CustomRuleEngine{
		compiledRules: make(map[string]*CompiledRule),
		functions:     make(map[string]CustomFunction),
	}

	engine.initializeFunctions()
	return engine
}

func NewScriptEngine() *ScriptEngine {
	return &ScriptEngine{
		supportedLanguages: make(map[string]LanguageExecutor),
	}
}

// Transform transforms a UniversalMessage using custom rules
func (s *CustomTransformationService) Transform(ctx context.Context, message *UniversalMessage) error {
	transformRecord := message.StartTransformation("CustomTransformationService", message.MessageType, message.MessageType)

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

	request := &CustomTransformRequest{
		MessageID:  message.ID,
		SourceData: message.ParsedContent,
		Context: map[string]interface{}{
			"messageType": message.MessageType,
			"sourceSystem": message.SourceSystem,
			"timestamp": time.Now(),
		},
	}

	response, err := s.ExecuteCustomRules(ctx, request)
	if err != nil {
		transformError = err
		message.AddError("TRANSFORMATION", "CustomTransformationService", "CUSTOM_RULE_FAILED",
			"Custom rule execution failed", err.Error(), true)
		return err
	}

	if !response.Success {
		transformError = fmt.Errorf("custom rule execution failed")
		return transformError
	}

	message.ParsedContent = response.TransformedData
	outputBytes, _ := json.Marshal(response.TransformedData)
	outputSize = int64(len(outputBytes))
	message.AddTransformedContent(message.MessageType, outputBytes, transformRecord.ID)

	message.UpdateStatus(StatusTransformed, "CustomTransformationService",
		fmt.Sprintf("Custom transformation completed (%d rules executed)", len(response.RulesExecuted)))

	log.Printf("✅ Custom transformation completed for message %s (Duration: %v)",
		message.ID, time.Since(startTime))
	return nil
}

// ExecuteCustomRules executes custom transformation rules
func (s *CustomTransformationService) ExecuteCustomRules(ctx context.Context, request *CustomTransformRequest) (*CustomTransformResponse, error) {
	startTime := time.Now()

	response := &CustomTransformResponse{
		Success:         false,
		MessageID:       request.MessageID,
		TransformedData: make(map[string]interface{}),
		RulesExecuted:   []RuleExecutionResult{},
		Warnings:        []string{},
		Errors:          []string{},
	}

	// Copy source data
	for k, v := range request.SourceData {
		response.TransformedData[k] = v
	}

	// Get applicable rules
	rules := s.getApplicableRules(request.RuleIDs)

	// Execute rules in priority order
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}

		ruleStart := time.Now()
		ruleResult := RuleExecutionResult{
			RuleID:   rule.ID,
			RuleName: rule.Name,
			Success:  false,
			Changes:  []string{},
		}

		// Check conditions
		if s.evaluateConditions(rule.Conditions, response.TransformedData, request.Context) {
			// Execute actions
			changes, err := s.executeActions(rule.Actions, response.TransformedData, request.Context)
			if err != nil {
				ruleResult.Error = err.Error()
				response.Errors = append(response.Errors, fmt.Sprintf("Rule %s failed: %v", rule.Name, err))
			} else {
				ruleResult.Success = true
				ruleResult.Changes = changes
			}
		}

		ruleResult.ExecutionTime = time.Since(ruleStart)
		response.RulesExecuted = append(response.RulesExecuted, ruleResult)
	}

	response.Success = len(response.Errors) == 0
	response.ProcessingMetrics = ProcessingMetrics{
		TotalTime: time.Since(startTime),
	}

	return response, nil
}

// initializeDefaultRules sets up default custom rules
func (s *CustomTransformationService) initializeDefaultRules() {
	// Data cleanup rule
	cleanupRule := CustomRule{
		ID:          "data-cleanup",
		Name:        "Data Cleanup",
		Description: "Remove null values and empty strings",
		Type:        "mapping",
		Language:    "javascript",
		Script:      "function cleanup(data) { /* cleanup logic */ return data; }",
		Actions: []CustomAction{
			{
				Type:     "transform",
				Target:   "$",
				Function: "removeNulls",
			},
		},
		Priority:  1,
		Enabled:   true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Validation rule
	validationRule := CustomRule{
		ID:          "data-validation",
		Name:        "Data Validation",
		Description: "Validate required fields",
		Type:        "validation",
		Conditions: []CustomCondition{
			{
				Field:    "$.id",
				Operator: "exists",
				Logic:    "AND",
			},
		},
		Actions: []CustomAction{
			{
				Type:   "validate",
				Target: "$.id",
				Value:  "required",
			},
		},
		Priority:  2,
		Enabled:   true,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	s.userDefinedRules[cleanupRule.ID] = cleanupRule
	s.userDefinedRules[validationRule.ID] = validationRule
}

// initializeFunctions sets up custom functions
func (engine *CustomRuleEngine) initializeFunctions() {
	engine.functions["removeNulls"] = func(value interface{}, params map[string]interface{}) (interface{}, error) {
		if dataMap, ok := value.(map[string]interface{}); ok {
			cleaned := make(map[string]interface{})
			for k, v := range dataMap {
				if v != nil && v != "" {
					cleaned[k] = v
				}
			}
			return cleaned, nil
		}
		return value, nil
	}

	engine.functions["uppercase"] = func(value interface{}, params map[string]interface{}) (interface{}, error) {
		if str, ok := value.(string); ok {
			return strings.ToUpper(str), nil
		}
		return value, nil
	}

	engine.functions["generateId"] = func(value interface{}, params map[string]interface{}) (interface{}, error) {
		return uuid.New().String(), nil
	}
}

// Helper methods
func (s *CustomTransformationService) getApplicableRules(ruleIDs []string) []CustomRule {
	var rules []CustomRule

	if len(ruleIDs) == 0 {
		// Return all enabled rules
		for _, rule := range s.userDefinedRules {
			if rule.Enabled {
				rules = append(rules, rule)
			}
		}
	} else {
		// Return specified rules
		for _, ruleID := range ruleIDs {
			if rule, exists := s.userDefinedRules[ruleID]; exists && rule.Enabled {
				rules = append(rules, rule)
			}
		}
	}

	// Sort by priority
	for i := 0; i < len(rules)-1; i++ {
		for j := i + 1; j < len(rules); j++ {
			if rules[i].Priority > rules[j].Priority {
				rules[i], rules[j] = rules[j], rules[i]
			}
		}
	}

	return rules
}

func (s *CustomTransformationService) evaluateConditions(conditions []CustomCondition, data, context map[string]interface{}) bool {
	if len(conditions) == 0 {
		return true
	}

	for _, condition := range conditions {
		if !s.evaluateCondition(condition, data, context) {
			return false
		}
	}

	return true
}

func (s *CustomTransformationService) evaluateCondition(condition CustomCondition, data, context map[string]interface{}) bool {
	// Simple condition evaluation
	value := s.extractValueByPath(data, condition.Field)

	switch condition.Operator {
	case "exists":
		return value != nil
	case "equals":
		return fmt.Sprintf("%v", value) == fmt.Sprintf("%v", condition.Value)
	case "not_equals":
		return fmt.Sprintf("%v", value) != fmt.Sprintf("%v", condition.Value)
	case "contains":
		if str, ok := value.(string); ok {
			return strings.Contains(str, fmt.Sprintf("%v", condition.Value))
		}
	}

	return true
}

func (s *CustomTransformationService) executeActions(actions []CustomAction, data, context map[string]interface{}) ([]string, error) {
	var changes []string

	for _, action := range actions {
		switch action.Type {
		case "set":
			s.setValueByPath(data, action.Target, action.Value)
			changes = append(changes, fmt.Sprintf("Set %s = %v", action.Target, action.Value))

		case "delete":
			s.deleteValueByPath(data, action.Target)
			changes = append(changes, fmt.Sprintf("Deleted %s", action.Target))

		case "transform":
			if action.Function != "" {
				if fn, exists := s.ruleEngine.functions[action.Function]; exists {
					oldValue := s.extractValueByPath(data, action.Target)
					newValue, err := fn(oldValue, action.Parameters)
					if err == nil {
						s.setValueByPath(data, action.Target, newValue)
						changes = append(changes, fmt.Sprintf("Transformed %s", action.Target))
					}
				}
			}
		}
	}

	return changes, nil
}

func (s *CustomTransformationService) extractValueByPath(data map[string]interface{}, path string) interface{} {
	// Simple path extraction
	if path == "$" {
		return data
	}

	if strings.HasPrefix(path, "$.") {
		field := path[2:]
		return data[field]
	}

	return data[path]
}

func (s *CustomTransformationService) setValueByPath(data map[string]interface{}, path string, value interface{}) {
	if strings.HasPrefix(path, "$.") {
		field := path[2:]
		data[field] = value
	} else {
		data[path] = value
	}
}

func (s *CustomTransformationService) deleteValueByPath(data map[string]interface{}, path string) {
	if strings.HasPrefix(path, "$.") {
		field := path[2:]
		delete(data, field)
	} else {
		delete(data, path)
	}
}

// Public API methods
func (s *CustomTransformationService) AddRule(rule CustomRule) {
	rule.UpdatedAt = time.Now()
	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = time.Now()
	}
	s.userDefinedRules[rule.ID] = rule
}

func (s *CustomTransformationService) RemoveRule(ruleID string) {
	delete(s.userDefinedRules, ruleID)
}

func (s *CustomTransformationService) GetRule(ruleID string) (CustomRule, bool) {
	rule, exists := s.userDefinedRules[ruleID]
	return rule, exists
}

func (s *CustomTransformationService) ListRules() []CustomRule {
	rules := make([]CustomRule, 0, len(s.userDefinedRules))
	for _, rule := range s.userDefinedRules {
		rules = append(rules, rule)
	}
	return rules
}

func (s *CustomTransformationService) GetPerformanceMetrics() CustomPerformanceMetrics {
	return s.performanceMetrics
}