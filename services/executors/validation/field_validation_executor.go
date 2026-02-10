package validation

import (
	"context"
	"encoding/json"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	"fmt"
	"log"
	"strings"
	"time"
)

// FieldValidationExecutor executes validation rules on message fields
type FieldValidationExecutor struct {
	*executors.BaseExecutor
	validators map[string]Validator // Strategy Pattern - registry of validators
}

// NewFieldValidationExecutor creates a new field validation executor
func NewFieldValidationExecutor() *FieldValidationExecutor {
	executor := &FieldValidationExecutor{
		BaseExecutor: executors.NewBaseExecutor("field_validation", models.ExecutorMetadata{
			Name:        "Field Validation",
			Description: "Validates message fields against configurable rules",
			Version:     "1.0.0",
			Author:      "ezHealthKonnect",
			Category:    "Validation",
		}),
		validators: make(map[string]Validator),
	}

	// Register built-in validators
	executor.RegisterValidator(NewRequiredValidator())

	formatValidator := NewFormatValidator()
	executor.RegisterValidator(formatValidator)
	// Map "pattern" to FormatValidator for backward compatibility
	// PatternValidator is redundant since FormatValidator supports custom regex via options["regex"]
	executor.validators["pattern"] = formatValidator

	executor.RegisterValidator(NewLengthValidator())

	return executor
}

// RegisterValidator adds a validator to the registry
func (e *FieldValidationExecutor) RegisterValidator(validator Validator) {
	e.validators[validator.GetType()] = validator
	log.Printf("✅ Registered validator: %s", validator.GetType())
}

// Execute runs the validation step
func (e *FieldValidationExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	if err := e.PreExecute(ctx, step); err != nil {
		return inputData, err
	}

	// Parse configuration
	config, err := e.parseConfig(step)
	if err != nil {
		e.PostExecute(ctx, step, err, time.Since(start))
		return inputData, err
	}

	// Run validations
	result := e.validateRules(config, inputData)

	// Build field-level validation output
	fieldResults := make([]map[string]interface{}, 0)
	for _, rule := range config.Rules {
		fieldResult := map[string]interface{}{
			"field":          rule.Field,
			"validation_type": rule.Type,
			"error_message":   rule.ErrorMessage,
		}

		// Check if this field had errors
		fieldError := ""
		for _, err := range result.Errors {
			if err.Field == rule.Field {
				fieldError = err.Message
				break
			}
		}

		if fieldError != "" {
			fieldResult["valid"] = false
			fieldResult["error"] = fieldError
		} else {
			fieldResult["valid"] = true
		}

		fieldResults = append(fieldResults, fieldResult)
	}

	// Store field-level results for step output tracking
	inputData["_field_validation_results"] = fieldResults

	// Build a cleaner validations map for user output (field -> result)
	validationsOutput := make(map[string]interface{})
	for _, fr := range fieldResults {
		fieldName := fr["field"].(string)
		validationsOutput[fieldName] = map[string]interface{}{
			"valid": fr["valid"],
			"type":  fr["validation_type"],
		}
		if err, ok := fr["error"]; ok && err != "" {
			validationsOutput[fieldName].(map[string]interface{})["error"] = err
		}
	}

	// STANDARDIZED: Separate variables (user-visible output) from execution details (metadata)
	// Variables: What the step produces that users care about
	variables := map[string]interface{}{
		"valid":       result.Valid,
		"validations": validationsOutput, // Per-field validation results
		"errorCount":  len(result.Errors),
	}

	// Execution details: Technical info for debugging/auditing
	executionDetails := map[string]interface{}{
		"fields_validated": len(fieldResults),
		"error_count":      len(result.Errors),
		"field_results":    fieldResults,
	}

	e.SetStepOutputWithDetails(inputData, variables, executionDetails)

	// Handle validation errors based on standard step controls
	if !result.Valid {
		errMsg := e.formatFieldValidationErrors(result.Errors, config.AddFieldNames)

		// Determine behavior based on step-level controls
		// Required=true or OnErrorStrategy="fail" → Return error (NACK sent, pipeline stops)
		// Required=false and OnErrorStrategy="continue" → Return nil (ACK sent, pipeline continues)
		if step.Required || step.OnErrorStrategy == "fail" {
			// STRICT MODE: Fail the pipeline, send NACK
			log.Printf("❌ [Strict Mode] Validation failed - pipeline will stop (Required=%v, OnErrorStrategy=%s)",
				step.Required, step.OnErrorStrategy)
			err := fmt.Errorf("validation failed: %s", errMsg)

			// Add metadata ONLY for ACK/NACK decision (internal use)
			inputData["_validation_status"] = "rejected"
			inputData["_ack_response"] = "NACK"

			// Publish validation feedback (rejected)
			e.publishValidationFeedback(ctx, "rejected", result.Errors, time.Since(start))

			e.PostExecute(ctx, step, err, time.Since(start))
			return inputData, err
		} else {
			// ACCEPT & FLAG MODE: Continue processing with warnings, send ACK
			log.Printf("⚠️  [Accept & Flag] Validation warnings: %s (Required=%v, OnErrorStrategy=%s)",
				errMsg, step.Required, step.OnErrorStrategy)

			// Add metadata ONLY for ACK/NACK decision (internal use)
			inputData["_validation_status"] = "warning"
			inputData["_ack_response"] = "ACK"

			// Publish validation feedback (warnings)
			e.publishValidationFeedback(ctx, "warning", result.Errors, time.Since(start))

			e.PostExecute(ctx, step, nil, time.Since(start))
			return inputData, nil
		}
	}

	// Validation passed
	// Add metadata ONLY for ACK/NACK decision (internal use)
	inputData["_validation_status"] = "passed"
	inputData["_ack_response"] = "ACK"
	e.publishValidationFeedback(ctx, "passed", nil, time.Since(start))

	e.PostExecute(ctx, step, nil, time.Since(start))
	return inputData, nil
}

// Validate validates the step configuration
func (e *FieldValidationExecutor) Validate(step *models.TransformationStep) error {
	_, err := e.parseConfig(step)
	return err
}

// parseConfig parses the step configuration
func (e *FieldValidationExecutor) parseConfig(step *models.TransformationStep) (*models.ValidationConfig, error) {
	var config models.ValidationConfig

	configBytes, err := json.Marshal(step.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := json.Unmarshal(configBytes, &config); err != nil {
		return nil, fmt.Errorf("failed to parse validation config: %w", err)
	}

	if len(config.Rules) == 0 {
		return nil, fmt.Errorf("no validation rules configured")
	}

	return &config, nil
}

// validateRules executes all validation rules
func (e *FieldValidationExecutor) validateRules(
	config *models.ValidationConfig,
	inputData map[string]interface{},
) models.FieldValidationResult {
	result := models.FieldValidationResult{
		Valid:  true,
		Errors: []models.FieldValidationError{},
	}

	for _, rule := range config.Rules {
		// Get the field value
		fieldValue := executors.GetNestedValue(inputData, rule.Field)

		// Get the validator for this rule type
		validator, exists := e.validators[rule.Type]
		if !exists {
			result.Valid = false
			result.Errors = append(result.Errors, NewFieldValidationError(
				rule.Field,
				e.extractFieldName(rule.Field),
				rule.Type,
				fmt.Sprintf("Unknown validation type: %s", rule.Type),
				"",
				"",
			))
			continue
		}

		// Execute validation
		valid, errorMsg := validator.Validate(fieldValue, rule.Options)

		if !valid {
			result.Valid = false

			// Extract field name from path for better error messages
			fieldName := e.extractFieldName(rule.Field)

			// Use custom error message if provided, otherwise use validator's message
			finalErrorMsg := rule.ErrorMessage
			if finalErrorMsg == "" {
				finalErrorMsg = errorMsg
			}

			// Add field name to error message if configured
			if config.AddFieldNames && fieldName != "" {
				finalErrorMsg = fmt.Sprintf("[%s] %s", fieldName, finalErrorMsg)
			}

			actualValue := fmt.Sprintf("%v", fieldValue)
			if fieldValue == nil {
				actualValue = "<null>"
			}

			result.Errors = append(result.Errors, NewFieldValidationError(
				rule.Field,
				fieldName,
				rule.Type,
				finalErrorMsg,
				actualValue,
				e.getExpectedFormat(rule),
			))

			// Stop on first error if configured
			if config.StopOnFirstError {
				break
			}
		}
	}

	return result
}

// extractFieldName extracts a human-readable field name from the field path
// Example: "enhancedSegments.PID.fields[4].subfields[1].value" -> "Email Address" (PID.13.4)
func (e *FieldValidationExecutor) extractFieldName(fieldPath string) string {
	// Try to extract from the path structure
	// Pattern: enhancedSegments.SEGMENT.fields[INDEX].value
	// or: enhancedSegments.SEGMENT.fields[INDEX].subfields[SUBINDEX].value

	parts := strings.Split(fieldPath, ".")
	if len(parts) < 3 {
		return fieldPath
	}

	// Extract segment name (e.g., "PID")
	segment := parts[1]

	// Try to extract field position from fields[INDEX]
	var fieldPos string
	var subfieldPos string

	for i, part := range parts {
		if strings.HasPrefix(part, "fields[") {
			// Extract index
			idx := strings.TrimSuffix(strings.TrimPrefix(part, "fields["), "]")
			// Field position is INDEX + 1 (array is 0-based, HL7 is 1-based)
			// But we need to look at the actual field key in the data
			fieldPos = idx
		}
		if strings.HasPrefix(part, "subfields[") {
			idx := strings.TrimSuffix(strings.TrimPrefix(part, "subfields["), "]")
			subfieldPos = idx

			// If we have both, construct HL7 field notation
			if fieldPos != "" {
				// This would be like PID.13.4
				// But we don't have the actual field numbers without parsing the data
				// So return a descriptive name
				if i > 0 && parts[i-1] == "subfields" {
					return fmt.Sprintf("%s subfield[%s]", segment, subfieldPos)
				}
			}
		}
	}

	if fieldPos != "" {
		return fmt.Sprintf("%s field[%s]", segment, fieldPos)
	}

	return fieldPath
}

// getExpectedFormat returns the expected format for the validation rule
func (e *FieldValidationExecutor) getExpectedFormat(rule models.ValidationRule) string {
	switch rule.Type {
	case "format":
		if format, ok := rule.Options["format"].(string); ok {
			return format
		}
		if regex, ok := rule.Options["regex"].(string); ok {
			return regex
		}
	case "pattern":
		if regex, ok := rule.Options["regex"].(string); ok {
			return regex
		}
	case "length":
		if exact, ok := rule.Options["exact"].(float64); ok {
			return fmt.Sprintf("exactly %d characters", int(exact))
		}
		var parts []string
		if min, ok := rule.Options["min"].(float64); ok {
			parts = append(parts, fmt.Sprintf("min: %d", int(min)))
		}
		if max, ok := rule.Options["max"].(float64); ok {
			parts = append(parts, fmt.Sprintf("max: %d", int(max)))
		}
		if len(parts) > 0 {
			return strings.Join(parts, ", ")
		}
	}
	return ""
}

// formatFieldValidationErrors formats validation errors for output
func (e *FieldValidationExecutor) formatFieldValidationErrors(errors []models.FieldValidationError, includeDetails bool) string {
	if len(errors) == 0 {
		return ""
	}

	var messages []string
	for _, err := range errors {
		messages = append(messages, err.Message)
	}

	return strings.Join(messages, "; ")
}

// GetStepType returns the step type identifier
func (e *FieldValidationExecutor) GetStepType() string {
	return "pre.validation"
}

// publishValidationFeedback publishes validation results to the ProcessingEngine
func (e *FieldValidationExecutor) publishValidationFeedback(
	ctx context.Context,
	status string,
	errors []models.FieldValidationError,
	processingTime time.Duration,
) {
	pe, ok := ctx.Value("processing_engine").(interface {
		PublishValidationFeedback(*models.ValidationFeedback)
	})
	if !ok || pe == nil {
		log.Printf("⚠️  ProcessingEngine not found in context - cannot send validation feedback")
		return
	}

	messageID, _ := ctx.Value("message_id").(string)
	interfaceID, _ := ctx.Value("interface_id").(string)

	if messageID == "" || interfaceID == "" {
		log.Printf("⚠️  Missing messageID or interfaceID in context")
		return
	}

	// Mode is no longer a separate parameter - determined by status
	var mode models.ValidationACKMode
	if status == "rejected" {
		mode = models.ValidationModeStrictReject
	} else {
		mode = models.ValidationModeAcceptAndFlag
	}

	feedback := models.NewValidationFeedback(
		messageID,
		interfaceID,
		mode,
		status,
		errors,
		processingTime.Milliseconds(),
		"tcp_mllp_inbound",
	)

	log.Printf("📤 Publishing validation feedback: %s (status: %s)", messageID, status)
	pe.PublishValidationFeedback(feedback)
}
