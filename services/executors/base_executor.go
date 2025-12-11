package executors

import (
	"context"
	"ezhealthkonnect/models"
	"fmt"
	"log"
	"strconv"
	"strings"
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

// GetNestedValue retrieves a value from a nested map using dot notation and array indices
// Supports paths like:
//   - "enhancedSegments.PID.fields[4].subfields[1].value"
//   - "patient.name"
//   - "metadata.timestamp"
func GetNestedValue(data map[string]interface{}, path string) interface{} {
	if path == "" {
		return nil
	}

	// Try direct key first (for simple paths)
	if val, ok := data[path]; ok {
		return val
	}

	// Parse path with dot notation and array indices
	// Example: "enhancedSegments.PID.fields[4].subfields[1].value"
	current := interface{}(data)
	parts := strings.Split(path, ".")

	for _, part := range parts {
		// Check if part has array index like "fields[4]"
		if strings.Contains(part, "[") {
			// Extract key and index: "fields[4]" -> "fields", 4
			openBracket := strings.Index(part, "[")
			closeBracket := strings.Index(part, "]")

			if openBracket == -1 || closeBracket == -1 {
				return nil // Invalid syntax
			}

			key := part[:openBracket]
			indexStr := part[openBracket+1 : closeBracket]
			index, err := strconv.Atoi(indexStr)
			if err != nil {
				return nil // Invalid index
			}

			// Navigate to the key first
			if currentMap, ok := current.(map[string]interface{}); ok {
				arrayVal, exists := currentMap[key]
				if !exists {
					return nil
				}

				// Access array element
				if arraySlice, ok := arrayVal.([]interface{}); ok {
					if index < 0 || index >= len(arraySlice) {
						return nil // Index out of bounds
					}
					current = arraySlice[index]
				} else {
					return nil // Not an array
				}
			} else {
				return nil
			}
		} else {
			// Simple key navigation
			if currentMap, ok := current.(map[string]interface{}); ok {
				val, exists := currentMap[part]
				if !exists {
					return nil
				}
				current = val
			} else {
				return nil
			}
		}
	}

	return current
}

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
