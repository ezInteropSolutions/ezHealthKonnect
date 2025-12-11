package executors

import (
	"context"
	"ezhealthkonnect/models"
	"fmt"
	"log"
	"time"
)

// ===============================================================
// BASE EXECUTOR - Template Method Pattern
// ===============================================================

// BaseExecutor provides common functionality for all executors
// Implements Template Method pattern for common pre/post execution logic
type BaseExecutor struct {
	stepType string
	metadata models.ExecutorMetadata
}

// NewBaseExecutor creates a new base executor
func NewBaseExecutor(stepType string, metadata models.ExecutorMetadata) *BaseExecutor {
	return &BaseExecutor{
		stepType: stepType,
		metadata: metadata,
	}
}

// GetStepType returns the executor's step type identifier
func (b *BaseExecutor) GetStepType() string {
	return b.stepType
}

// GetMetadata returns the executor's metadata
func (b *BaseExecutor) GetMetadata() models.ExecutorMetadata {
	return b.metadata
}

// PreExecute performs common pre-execution checks (Template Method)
func (b *BaseExecutor) PreExecute(ctx context.Context, step *models.TransformationStep) error {
	// Check context timeout
	if ctx.Err() != nil {
		return fmt.Errorf("context error: %w", ctx.Err())
	}

	// Validate step is enabled
	if !step.Enabled {
		return fmt.Errorf("step is disabled")
	}

	// Validate step type matches
	if step.StepType != b.stepType {
		return fmt.Errorf("step type mismatch: expected %s, got %s", b.stepType, step.StepType)
	}

	log.Printf("🔄 [%s] Starting execution: %s", b.stepType, step.StepName)
	return nil
}

// PostExecute performs common post-execution logging (Template Method)
func (b *BaseExecutor) PostExecute(ctx context.Context, step *models.TransformationStep, err error, duration time.Duration) {
	if err != nil {
		log.Printf("❌ [%s] Failed after %v: %v", step.StepName, duration, err)
	} else {
		log.Printf("✅ [%s] Completed in %v", step.StepName, duration)
	}
}

// ValidateConfig is a helper for validating required config fields
func (b *BaseExecutor) ValidateConfig(step *models.TransformationStep, requiredFields []string) error {
	if step.Config == nil {
		return fmt.Errorf("config is required")
	}

	for _, field := range requiredFields {
		if _, exists := step.Config[field]; !exists {
			return fmt.Errorf("required config field missing: %s", field)
		}
	}

	return nil
}

// ===============================================================
// HELPER FUNCTIONS
// ===============================================================

// SetNestedValue sets a value in a nested map using dot notation
func SetNestedValue(data map[string]interface{}, path string, value interface{}) error {
	if path == "" {
		return fmt.Errorf("empty path")
	}

	// Simple implementation - just set at top level for now
	// Can be enhanced to handle nested paths like "patient.age"
	data[path] = value
	return nil
}

// EnsureMapExists ensures a nested map path exists
func EnsureMapExists(data map[string]interface{}, path string) map[string]interface{} {
	if existing, ok := data[path]; ok {
		if existingMap, isMap := existing.(map[string]interface{}); isMap {
			return existingMap
		}
	}

	newMap := make(map[string]interface{})
	data[path] = newMap
	return newMap
}
