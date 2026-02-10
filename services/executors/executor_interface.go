package executors

import (
	"context"
	"ezhealthkonnect/models"
)

// ===============================================================
// EXECUTOR INTERFACE - Strategy Pattern
// ===============================================================

// Executor defines the contract for all pipeline step executors
// This is the core Strategy interface that all executors must implement
type Executor interface {
	// Execute performs the executor's operation on input data
	Execute(ctx context.Context, step *models.TransformationStep, inputData map[string]interface{}) (map[string]interface{}, error)

	// GetStepType returns the unique step type identifier (e.g., "pre.validation", "pre.enrichment.metadata")
	GetStepType() string
}

// ===============================================================
// OPTIONAL INTERFACES - Interface Segregation Principle
// ===============================================================

// Validatable interface for executors that can validate their configuration
type Validatable interface {
	Validate(step *models.TransformationStep) error
}

// MetadataProvider interface for executors that provide metadata
type MetadataProvider interface {
	GetMetadata() models.ExecutorMetadata
}

// Cacheable interface for executors that support result caching
type Cacheable interface {
	GetCacheKey(step *models.TransformationStep, inputData map[string]interface{}) string
	GetCacheTTL() int
}

// Retryable interface for executors that support retry logic
type Retryable interface {
	ShouldRetry(err error) bool
	GetMaxRetries() int
}

// Describable interface for executors that can describe their configuration
type Describable interface {
	GetConfigSchema() map[string]interface{}
	GetConfigExample() map[string]interface{}
}

// VariableProvider interface for executors that declare their output variables
// This enables automatic variable discovery for downstream steps (IntelliSense/autocomplete)
type VariableProvider interface {
	// GetOutputVariables returns the list of variables this executor will produce
	// based on its configuration. Returns variable definitions that can be used
	// for autocomplete, validation, and reference documentation.
	GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition
}
