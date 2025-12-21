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

// ===============================================================
// STRATEGY-BASED ENRICHMENT MODELS (NEW)
// ===============================================================
// These models support the Strategy Pattern for pluggable enrichment

// EnrichmentStrategy defines different enrichment approaches
type EnrichmentStrategy string

const (
	// EnrichmentStrategyAPI queries external REST APIs
	EnrichmentStrategyAPI EnrichmentStrategy = "api"

	// EnrichmentStrategyDatabase queries databases (PostgreSQL, MySQL, etc.)
	EnrichmentStrategyDatabase EnrichmentStrategy = "database"

	// EnrichmentStrategyCache queries cache systems (Redis, Memcached)
	EnrichmentStrategyCache EnrichmentStrategy = "cache"

	// EnrichmentStrategyScript executes custom JavaScript for enrichment
	EnrichmentStrategyScript EnrichmentStrategy = "script"
)

// ===============================================================
// API ENRICHMENT CONFIGURATION
// ===============================================================

// APIEnrichmentConfig configuration for API-based enrichment
type APIEnrichmentConfig struct {
	// API endpoint URL (required)
	Endpoint string `json:"endpoint"`

	// HTTP method (GET, POST, etc.)
	Method string `json:"method,omitempty"` // Default: GET

	// Authentication configuration
	AuthType    string `json:"authType,omitempty"` // none, basic, bearer, apikey, oauth2
	Username    string `json:"username,omitempty"`
	Password    string `json:"password,omitempty"`
	APIKey      string `json:"apiKey,omitempty"`
	BearerToken string `json:"bearerToken,omitempty"`

	// OAuth 2.0 configuration (full OAuth flow with automatic token management)
	OAuth2TokenURL     string `json:"oauth2TokenUrl,omitempty"`     // OAuth 2.0 token endpoint URL
	OAuth2ClientID     string `json:"oauth2ClientId,omitempty"`     // OAuth 2.0 client ID
	OAuth2ClientSecret string `json:"oauth2ClientSecret,omitempty"` // OAuth 2.0 client secret
	OAuth2GrantType    string `json:"oauth2GrantType,omitempty"`    // Grant type: client_credentials, password, refresh_token
	OAuth2Scope        string `json:"oauth2Scope,omitempty"`        // OAuth 2.0 scope (space-separated)
	OAuth2Username     string `json:"oauth2Username,omitempty"`     // Username for password grant
	OAuth2Password     string `json:"oauth2Password,omitempty"`     // Password for password grant

	// Request configuration
	Headers     map[string]string      `json:"headers,omitempty"`
	QueryParams map[string]string      `json:"queryParams,omitempty"`
	RequestBody map[string]interface{} `json:"requestBody,omitempty"`

	// Field mapping - which field from message to use in API call
	// Example: "patientId" -> "PID.3" (use patient ID from HL7 message)
	FieldMappings map[string]string `json:"fieldMappings,omitempty"`

	// Response mapping - where to store API response in message
	// Example: "demographics" -> store response at inputData["enriched"]["demographics"]
	TargetPath string `json:"targetPath,omitempty"` // Default: "enriched.api"

	// Timeout in milliseconds
	TimeoutMs int `json:"timeoutMs,omitempty"` // Default: 5000

	// Retry configuration
	RetryCount   int `json:"retryCount,omitempty"`   // Default: 0
	RetryDelayMs int `json:"retryDelayMs,omitempty"` // Default: 1000

	// Error handling
	FailOnError  bool        `json:"failOnError,omitempty"`  // Default: false (continue on failure)
	DefaultValue interface{} `json:"defaultValue,omitempty"` // Value to use if API call fails
}

// ===============================================================
// DATABASE ENRICHMENT CONFIGURATION (EXTENDED)
// ===============================================================

// DatabaseEnrichmentConfigV2 extends the existing configuration with more options
type DatabaseEnrichmentConfigV2 struct {
	// Database connection (required if not using connection name)
	ConnectionString string `json:"connectionString,omitempty"`
	ConnectionName   string `json:"connectionName,omitempty"` // Reference to stored connection

	// Database type
	DatabaseType string `json:"databaseType"` // postgresql, mysql, sqlserver, oracle

	// Query configuration
	Query       string            `json:"query"`       // SQL query with parameter placeholders
	QueryParams map[string]string `json:"queryParams,omitempty"` // Map parameter name to field path

	// Example:
	// Query: "SELECT * FROM patients WHERE patient_id = $1"
	// QueryParams: {"patientId": "PID.3"}

	// Response mapping
	TargetPath    string            `json:"targetPath,omitempty"`    // Default: "enriched.database"
	ResultMapping map[string]string `json:"resultMapping,omitempty"` // Map DB columns to output fields

	// Timeout in milliseconds
	TimeoutMs int `json:"timeoutMs,omitempty"` // Default: 3000

	// Caching
	CacheResults bool `json:"cacheResults,omitempty"`
	CacheTTL     int  `json:"cacheTTL,omitempty"` // seconds

	// Error handling
	FailOnError  bool        `json:"failOnError,omitempty"` // Default: false
	DefaultValue interface{} `json:"defaultValue,omitempty"`
}

// ===============================================================
// CACHE ENRICHMENT CONFIGURATION
// ===============================================================

// CacheEnrichmentConfig configuration for cache-based enrichment
type CacheEnrichmentConfig struct {
	// Cache connection
	ConnectionString string `json:"connectionString,omitempty"`
	ConnectionName   string `json:"connectionName,omitempty"`

	// Cache type
	CacheType string `json:"cacheType"` // redis, memcached

	// Key configuration - build cache key from message fields
	// Example: "patient:{patientId}" where {patientId} is replaced with PID.3 value
	KeyTemplate string            `json:"keyTemplate"`
	KeyMappings map[string]string `json:"keyMappings,omitempty"`

	// Response mapping
	TargetPath string `json:"targetPath,omitempty"` // Default: "enriched.cache"

	// Timeout in milliseconds
	TimeoutMs int `json:"timeoutMs,omitempty"` // Default: 1000

	// Error handling
	FailOnError  bool        `json:"failOnError,omitempty"` // Default: false
	DefaultValue interface{} `json:"defaultValue,omitempty"`

	// Optional: Write back to cache
	WriteBack  bool `json:"writeBack,omitempty"`
	TTLSeconds int  `json:"ttlSeconds,omitempty"` // Time to live for cache entries
}

// ===============================================================
// SCRIPT ENRICHMENT CONFIGURATION
// ===============================================================

// ScriptEnrichmentConfig configuration for custom JavaScript enrichment
type ScriptEnrichmentConfig struct {
	// JavaScript code to execute
	Script string `json:"script"`

	// Context variables to pass to script
	// These will be available as global variables in the script
	Context map[string]interface{} `json:"context,omitempty"`

	// Response mapping
	TargetPath string `json:"targetPath,omitempty"` // Default: "enriched.script"

	// Timeout in milliseconds
	TimeoutMs int `json:"timeoutMs,omitempty"` // Default: 5000

	// Error handling
	FailOnError bool `json:"failOnError,omitempty"` // Default: false
}

// ===============================================================
// UNIFIED ENRICHMENT CONFIGURATION
// ===============================================================

// UnifiedEnrichmentConfig is a unified configuration that can hold any enrichment type
// Only one of the strategy-specific configs should be populated
type UnifiedEnrichmentConfig struct {
	// Strategy type
	Strategy EnrichmentStrategy `json:"strategy"`

	// Strategy-specific configurations (only one should be set)
	APIConfig      *APIEnrichmentConfig       `json:"apiConfig,omitempty"`
	DatabaseConfig *DatabaseEnrichmentConfigV2 `json:"databaseConfig,omitempty"`
	CacheConfig    *CacheEnrichmentConfig     `json:"cacheConfig,omitempty"`
	ScriptConfig   *ScriptEnrichmentConfig    `json:"scriptConfig,omitempty"`

	// Common settings
	Enabled     bool   `json:"enabled,omitempty"`     // Default: true
	StopOnError bool   `json:"stopOnError,omitempty"` // Stop pipeline if enrichment fails
	LogLevel    string `json:"logLevel,omitempty"`    // debug, info, warn, error
}
