package models

import (
	"time"
)

// ===============================================================
// ENRICHMENT CONFIGURATION MODELS
// ===============================================================

// MetadataEnrichmentConfig defines configuration for metadata enrichment
type MetadataEnrichmentConfig struct {
	AddTimestamp     bool              `json:"addTimestamp"`
	AddCorrelationID bool              `json:"addCorrelationId"`
	AddInterfaceID   bool              `json:"addInterfaceId"`
	AddMessageID     bool              `json:"addMessageId"`
	CustomMetadata   map[string]string `json:"customMetadata,omitempty"`
}

// CalculatedEnrichmentConfig defines configuration for calculated field enrichment
type CalculatedEnrichmentConfig struct {
	Calculations []Calculation `json:"calculations"`
}

// Calculation represents a single calculation operation
type Calculation struct {
	Type        string                 `json:"type"`        // age_from_dob, bmi, full_name
	SourceField interface{}            `json:"sourceField"` // string or []string for multiple fields
	TargetField string                 `json:"targetField"`
	Options     map[string]interface{} `json:"options,omitempty"`
}

// DatabaseEnrichmentConfig defines configuration for database lookup enrichment
type DatabaseEnrichmentConfig struct {
	Query         string            `json:"query"`
	SourceField   string            `json:"sourceField"`
	TargetMapping map[string]string `json:"targetMapping"`
	CacheResults  bool              `json:"cacheResults,omitempty"`
	CacheTTL      int               `json:"cacheTTL,omitempty"` // seconds
}

// ===============================================================
// ENRICHMENT RESULT MODELS
// ===============================================================

// EnrichmentResult represents the result of an enrichment operation
type EnrichmentResult struct {
	Success       bool                   `json:"success"`
	FieldsAdded   []string               `json:"fieldsAdded"`
	FieldsUpdated []string               `json:"fieldsUpdated"`
	EnrichedData  map[string]interface{} `json:"enrichedData"`
	Error         string                 `json:"error,omitempty"`
	ExecutionTime time.Duration          `json:"executionTime"`
}

// ===============================================================
// EXECUTOR METADATA
// ===============================================================

// ExecutorMetadata provides information about an executor
type ExecutorMetadata struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
	Author      string `json:"author"`
	Category    string `json:"category"` // validation, enrichment, mapping, transformation
}

// ===============================================================
// ENRICHMENT ERROR TYPES
// ===============================================================

// EnrichmentError represents an error during enrichment
type EnrichmentError struct {
	StepName string
	StepType string
	Code     string
	Message  string
	Cause    error
}

func (e *EnrichmentError) Error() string {
	if e.Cause != nil {
		return e.Message + ": " + e.Cause.Error()
	}
	return e.Message
}

// Error codes
const (
	ErrConfigInvalid     = "CONFIG_INVALID"
	ErrFieldNotFound     = "FIELD_NOT_FOUND"
	ErrDatabaseQuery     = "DATABASE_QUERY_FAILED"
	ErrCalculationFailed = "CALCULATION_FAILED"
	ErrAPICallFailed     = "API_CALL_FAILED"
	ErrValidationFailed  = "VALIDATION_FAILED"
)
