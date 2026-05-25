package models

import (
	"fmt"
	"strings"
	"sync"
)

// VariableType represents the type of a pipeline variable
type VariableType string

const (
	VarTypeString  VariableType = "string"
	VarTypeNumber  VariableType = "number"
	VarTypeBoolean VariableType = "boolean"
	VarTypeObject  VariableType = "object"
	VarTypeArray   VariableType = "array"
	VarTypeDate    VariableType = "date"
	VarTypeUnknown VariableType = "unknown"
)

// VariableMetadata contains metadata about a pipeline variable
type VariableMetadata struct {
	Name        string       `json:"name"`         // Variable name (e.g., "risk_weights")
	FullPath    string       `json:"full_path"`    // Full path (e.g., "field_mapping.risk_weights")
	Type        VariableType `json:"type"`         // Variable type
	Description string       `json:"description"`  // Human-readable description
	StepName    string       `json:"step_name"`    // Source step name
	StepType    string       `json:"step_type"`    // Source step type
	SampleValue interface{}  `json:"sample_value"` // Example value for preview
	Required    bool         `json:"required"`     // Is this variable always present?
}

// PipelineVariableContext manages variables in a flat namespace
// Design: {step_name}.{variable_name} = value
// Example: field_mapping.risk_weights, database_enrichment.chronic_conditions
type PipelineVariableContext struct {
	Variables map[string]interface{}        `json:"variables"` // Flat key-value store
	Metadata  map[string]VariableMetadata   `json:"metadata"`  // Variable metadata for IntelliSense
	mutex     sync.RWMutex                  // Thread safety
}

// NewPipelineVariableContext creates a new variable context
func NewPipelineVariableContext() *PipelineVariableContext {
	return &PipelineVariableContext{
		Variables: make(map[string]interface{}),
		Metadata:  make(map[string]VariableMetadata),
	}
}

// RegisterVariable registers a variable from a step output
// stepName: normalized step name (e.g., "field_mapping", "database_enrichment_sqlserver")
// varName: variable name (e.g., "risk_weights", "chronic_conditions")
// value: the actual value
// varType: the variable type
func (ctx *PipelineVariableContext) RegisterVariable(stepName, stepType, varName string, value interface{}, varType VariableType) {
	ctx.mutex.Lock()
	defer ctx.mutex.Unlock()

	// Normalize names to snake_case
	normalizedStepName := toSnakeCase(stepName)
	normalizedVarName := toSnakeCase(varName)

	fullPath := fmt.Sprintf("%s.%s", normalizedStepName, normalizedVarName)

	// Store value
	ctx.Variables[fullPath] = value

	// Store metadata
	ctx.Metadata[fullPath] = VariableMetadata{
		Name:        normalizedVarName,
		FullPath:    fullPath,
		Type:        varType,
		StepName:    normalizedStepName,
		StepType:    stepType,
		SampleValue: getSampleValue(value, varType),
		Required:    true, // Default to required
	}
}

// RegisterStepOutput registers all variables from a step's output
// Automatically flattens nested structures into flat namespace
func (ctx *PipelineVariableContext) RegisterStepOutput(stepName, stepType string, output map[string]interface{}) {
	ctx.mutex.Lock()
	defer ctx.mutex.Unlock()

	normalizedStepName := toSnakeCase(stepName)

	for varName, value := range output {
		// Skip internal/metadata fields
		if strings.HasPrefix(varName, "_") {
			continue
		}

		normalizedVarName := toSnakeCase(varName)
		fullPath := fmt.Sprintf("%s.%s", normalizedStepName, normalizedVarName)

		// Detect type
		varType := detectVariableType(value)

		// Store value
		ctx.Variables[fullPath] = value

		// Store metadata
		ctx.Metadata[fullPath] = VariableMetadata{
			Name:        normalizedVarName,
			FullPath:    fullPath,
			Type:        varType,
			StepName:    normalizedStepName,
			StepType:    stepType,
			SampleValue: getSampleValue(value, varType),
			Required:    true,
		}
	}
}

// Get retrieves a variable by full path
func (ctx *PipelineVariableContext) Get(fullPath string) (interface{}, bool) {
	ctx.mutex.RLock()
	defer ctx.mutex.RUnlock()

	value, exists := ctx.Variables[fullPath]
	return value, exists
}

// GetAll returns all variables (for script context injection)
func (ctx *PipelineVariableContext) GetAll() map[string]interface{} {
	ctx.mutex.RLock()
	defer ctx.mutex.RUnlock()

	// Return a copy to prevent external modification
	result := make(map[string]interface{}, len(ctx.Variables))
	for k, v := range ctx.Variables {
		result[k] = v
	}
	return result
}

// GetMetadata returns metadata for all variables (for IntelliSense)
func (ctx *PipelineVariableContext) GetMetadata() []VariableMetadata {
	ctx.mutex.RLock()
	defer ctx.mutex.RUnlock()

	result := make([]VariableMetadata, 0, len(ctx.Metadata))
	for _, meta := range ctx.Metadata {
		result = append(result, meta)
	}
	return result
}

// GetByStepName returns all variables from a specific step
func (ctx *PipelineVariableContext) GetByStepName(stepName string) map[string]interface{} {
	ctx.mutex.RLock()
	defer ctx.mutex.RUnlock()

	normalizedStepName := toSnakeCase(stepName)
	prefix := normalizedStepName + "."

	result := make(map[string]interface{})
	for fullPath, value := range ctx.Variables {
		if strings.HasPrefix(fullPath, prefix) {
			// Remove prefix to get variable name only
			varName := strings.TrimPrefix(fullPath, prefix)
			result[varName] = value
		}
	}
	return result
}

// ToNestedStructure converts flat namespace back to nested structure
// Used for backward compatibility with existing code
func (ctx *PipelineVariableContext) ToNestedStructure() map[string]interface{} {
	ctx.mutex.RLock()
	defer ctx.mutex.RUnlock()

	result := make(map[string]interface{})

	for fullPath, value := range ctx.Variables {
		parts := strings.Split(fullPath, ".")
		if len(parts) != 2 {
			continue // Skip invalid paths
		}

		stepName := parts[0]
		varName := parts[1]

		// Ensure step object exists
		if _, exists := result[stepName]; !exists {
			result[stepName] = make(map[string]interface{})
		}

		// Add variable to step object
		if stepObj, ok := result[stepName].(map[string]interface{}); ok {
			stepObj[varName] = value
		}
	}

	return result
}

// detectVariableType automatically detects the type of a value
func detectVariableType(value interface{}) VariableType {
	if value == nil {
		return VarTypeUnknown
	}

	switch v := value.(type) {
	case string:
		return VarTypeString
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64, float32, float64:
		return VarTypeNumber
	case bool:
		return VarTypeBoolean
	case map[string]interface{}:
		return VarTypeObject
	case []interface{}, []map[string]interface{}:
		return VarTypeArray
	default:
		// Check if it's a slice/array
		if strings.HasPrefix(fmt.Sprintf("%T", v), "[]") {
			return VarTypeArray
		}
		return VarTypeObject
	}
}

// getSampleValue returns a safe sample value for metadata
func getSampleValue(value interface{}, varType VariableType) interface{} {
	switch varType {
	case VarTypeString:
		if str, ok := value.(string); ok && len(str) > 50 {
			return str[:50] + "..."
		}
		return value
	case VarTypeArray:
		return "[array]"
	case VarTypeObject:
		return "{object}"
	default:
		return value
	}
}

// toSnakeCase converts a string to snake_case
func toSnakeCase(s string) string {
	var result strings.Builder
	prevWasUnderscore := false

	for i, r := range s {
		// Handle spaces, hyphens, arrows (→)
		if r == ' ' || r == '-' || r == '→' {
			if !prevWasUnderscore && i > 0 {
				result.WriteRune('_')
				prevWasUnderscore = true
			}
			continue
		}

		// Handle uppercase letters
		if r >= 'A' && r <= 'Z' {
			if i > 0 && !prevWasUnderscore {
				result.WriteRune('_')
			}
			result.WriteRune(r + 32) // Convert to lowercase
			prevWasUnderscore = false
		} else {
			result.WriteRune(r)
			prevWasUnderscore = r == '_'
		}
	}

	return result.String()
}
