package models

import (
	"encoding/json"
	"time"
)

// ===============================================================
// ENRICHMENT CONFIGURATION MODELS
// ===============================================================

// MetadataEnrichmentConfig defines configuration for metadata enrichment
type MetadataEnrichmentConfig struct {
	AddTimestamp     bool                   `json:"addTimestamp"`
	AddCorrelationID bool                   `json:"addCorrelationId"`
	AddInterfaceID   bool                   `json:"addInterfaceId"`
	AddMessageID     bool                   `json:"addMessageId"`
	CustomMetadata   map[string]interface{} `json:"customMetadata,omitempty"` // Changed to interface{} to support nested objects
}

// FieldMappingConfig defines configuration for field mapping transformations
type FieldMappingConfig struct {
	Mappings []FieldMapping         `json:"mappings"`
	Metadata map[string]interface{} `json:"metadata,omitempty"` // Optional metadata to add (supports nested objects)
}

// FieldMapping represents a single field mapping with transformations
type FieldMapping struct {
	LHS        string `json:"lhs"`        // Target variable name
	RHS        string `json:"rhs"`        // Source field path or system variable
	Transforms string `json:"transforms"` // Comma-separated transformations
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
// NOTE: This is a legacy/simple version. For full features, see DatabaseEnrichmentConfigV2 below.
type DatabaseEnrichmentConfig struct {
	Query         string            `json:"query"`
	SourceField   string            `json:"sourceField"`
	TargetMapping map[string]string `json:"targetMapping"`
	CacheResults  bool              `json:"cacheResults,omitempty"`
	CacheTTL      int               `json:"cacheTTL,omitempty"` // seconds
	TargetPath    string            `json:"targetPath,omitempty"` // Where to store results (e.g., "enriched.database")
	OutputSchema  []OutputField     `json:"outputSchema,omitempty"` // Declared output fields
}

// NOTE: APIEnrichmentConfig and ScriptEnrichmentConfig are defined below in the
// "STRATEGY-BASED ENRICHMENT MODELS" section with extended configurations

// OutputField declares a field that will be produced by an enrichment step
type OutputField struct {
	Name        string `json:"name"`        // Field name (e.g., "chronicConditions")
	Description string `json:"description"` // Human-readable description
	Type        string `json:"type"`        // Data type: string, number, boolean, array, object
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
	OAuth2Audience     string `json:"oauth2Audience,omitempty"`     // OAuth 2.0 audience (required by Auth0 and some providers)
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

	// Default value fallback (executor-level; error handling is at pipeline/step level)
	DefaultValue interface{} `json:"defaultValue,omitempty"` // Value to use if API call fails

	// Circuit breaker — prevents hammering a failing downstream API
	FailureThreshold    int `json:"failureThreshold,omitempty"`    // consecutive failures before opening (default 5)
	OpenDurationSeconds int `json:"openDurationSeconds,omitempty"` // seconds to stay open before half-open probe (default 60)

	// Result caching — in-memory LRU+TTL cache to avoid redundant API calls
	CacheTtlSeconds int `json:"cacheTtlSeconds,omitempty"` // 0 = disabled; default 0
	CacheMaxEntries int `json:"cacheMaxEntries,omitempty"` // max cache size; default 1000
}

// ===============================================================
// DATABASE ENRICHMENT CONFIGURATION (EXTENDED)
// ===============================================================

// DatabaseEnrichmentConfigV2 extends the existing configuration with more options
type DatabaseEnrichmentConfigV2 struct {
	// Database connection (required if not using connection name)
	ConnectionString string `json:"connectionString,omitempty"`
	ConnectionName   string `json:"connectionName,omitempty"` // Reference to stored connection

	// Individual connection fields (alternative to connectionString)
	DBHost     string `json:"dbHost,omitempty"`
	DBPort     int    `json:"dbPort,omitempty"`
	DBName     string `json:"dbName,omitempty"`
	DBUser     string `json:"dbUser,omitempty"`
	DBPassword string `json:"dbPassword,omitempty"`

	// Database type (SQL and NoSQL)
	DatabaseType string `json:"databaseType"` // postgresql, mysql, sqlserver, oracle, mongodb, redis, dynamodb, snowflake, databricks, cassandra

	// Query configuration (SQL databases)
	Query       string            `json:"query"`       // SQL query with parameter placeholders
	QueryParams map[string]string `json:"queryParams,omitempty"` // Map parameter name to field path

	// Example:
	// Query: "SELECT * FROM patients WHERE patient_id = $1"
	// QueryParams: {"patientId": "PID.3"}

	// NoSQL-specific configuration
	Collection          string                   `json:"collection,omitempty"`          // MongoDB collection name
	Filter              map[string]interface{}   `json:"filter,omitempty"`              // MongoDB filter query
	Projection          map[string]interface{}   `json:"projection,omitempty"`          // MongoDB projection
	MongoDBQueryMode    string                   `json:"mongodbQueryMode,omitempty"`    // visual or advanced
	AggregationPipeline []map[string]interface{} `json:"-"`                             // MongoDB aggregation pipeline (advanced mode) - custom unmarshaling
	AggregationPipelineRaw json.RawMessage       `json:"aggregationPipeline,omitempty"` // Raw JSON for custom unmarshaling
	RedisKey            string                   `json:"redisKey,omitempty"`            // Redis key pattern (supports {field} placeholders)
	RedisCommand        string                   `json:"redisCommand,omitempty"`        // GET, HGETALL, SMEMBERS, etc.
	DynamoDBTable   string                 `json:"dynamodbTable,omitempty"`   // DynamoDB table name
	KeyCondition    string                 `json:"keyCondition,omitempty"`    // DynamoDB key condition expression
	FilterExpression string                `json:"filterExpression,omitempty"` // DynamoDB filter expression
	CassandraKeyspace string               `json:"cassandraKeyspace,omitempty"` // Cassandra keyspace

	// Cloud database specific
	AWSRegion        string `json:"awsRegion,omitempty"`        // For DynamoDB
	AWSAccessKey     string `json:"awsAccessKey,omitempty"`     // For DynamoDB
	AWSSecretKey     string `json:"awsSecretKey,omitempty"`     // For DynamoDB
	SnowflakeAccount string `json:"snowflakeAccount,omitempty"` // For Snowflake
	SnowflakeWarehouse string `json:"snowflakeWarehouse,omitempty"` // For Snowflake
	SnowflakeSchema  string `json:"snowflakeSchema,omitempty"`  // For Snowflake
	DatabricksHTTPPath string `json:"databricksHttpPath,omitempty"` // For Databricks
	DatabricksCatalog  string `json:"databricksCatalog,omitempty"`  // For Databricks
	DatabricksToken    string `json:"databricksToken,omitempty"`    // For Databricks access token

	// Response mapping
	TargetPath    string            `json:"targetPath,omitempty"`    // Default: "enriched.database"
	ResultMapping map[string]string `json:"resultMapping,omitempty"` // Map DB columns to output fields

	// Timeout in milliseconds
	TimeoutMs int `json:"timeoutMs,omitempty"` // Default: 3000

	// Caching — in-memory LRU+TTL cache to avoid redundant DB queries
	CacheResults    bool `json:"cacheResults,omitempty"`
	CacheTTL        int  `json:"cacheTTL,omitempty"`        // seconds (0 = disabled)
	CacheMaxEntries int  `json:"cacheMaxEntries,omitempty"` // max cache size; default 1000

	// Connection pool tuning (optional; registry defaults used when zero)
	PoolMaxOpen            int `json:"poolMaxOpen,omitempty"`            // default 10
	PoolMaxIdle            int `json:"poolMaxIdle,omitempty"`            // default 5
	PoolMaxLifetimeMinutes int `json:"poolMaxLifetimeMinutes,omitempty"` // default 5

	// Default value fallback (executor-level; error handling is at pipeline/step level)
	DefaultValue interface{} `json:"defaultValue,omitempty"`
}

// UnmarshalJSON custom unmarshaler to handle aggregationPipeline as both string and array
func (d *DatabaseEnrichmentConfigV2) UnmarshalJSON(data []byte) error {
	// Create a type alias to avoid infinite recursion
	type Alias DatabaseEnrichmentConfigV2

	// Unmarshal into alias
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(d),
	}

	if err := json.Unmarshal(data, aux); err != nil {
		return err
	}

	// Handle aggregationPipeline - convert string to array if needed
	if len(d.AggregationPipelineRaw) > 0 {
		// Try to unmarshal as array first
		var pipeline []map[string]interface{}
		if err := json.Unmarshal(d.AggregationPipelineRaw, &pipeline); err == nil {
			d.AggregationPipeline = pipeline
		} else {
			// Try as string (legacy format)
			var pipelineStr string
			if err := json.Unmarshal(d.AggregationPipelineRaw, &pipelineStr); err == nil {
				// If it's an empty string, leave AggregationPipeline as empty array
				if pipelineStr != "" {
					// Try to parse the string as JSON
					if err := json.Unmarshal([]byte(pipelineStr), &pipeline); err == nil {
						d.AggregationPipeline = pipeline
					}
				}
			}
		}
	}

	return nil
}

// ===============================================================
// CACHE ENRICHMENT CONFIGURATION
// ===============================================================
// DEPRECATED: Use DatabaseEnrichmentConfigV2 with Redis instead.
// This struct is kept for backward compatibility only.

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

	// Default value fallback (executor-level; error handling is at pipeline/step level)
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

	// DEPRECATED: Context variables - use Metadata Enrichment or Database Enrichment steps instead
	// Kept for backward compatibility only
	Context map[string]interface{} `json:"context,omitempty"`

	// Response mapping
	TargetPath string `json:"targetPath,omitempty"` // Default: "enriched.script"

	// Timeout in milliseconds
	TimeoutMs int `json:"timeoutMs,omitempty"` // Default: 5000

}

// ===============================================================
// FILE PARSER CONFIGURATION
// ===============================================================

// FileParserConfig defines configuration for the file parser executor
type FileParserConfig struct {
	SourceField string      `json:"sourceField"`            // field containing raw file content
	FileFormat  string      `json:"fileFormat"`             // csv, tsv, fixed_width, xlsx
	Delimiter   string      `json:"delimiter,omitempty"`    // for delimited files (default: ",")
	HasHeader   bool        `json:"hasHeader"`              // first row = column names
	Columns     []ColumnDef `json:"columns,omitempty"`      // for fixed_width: name + position
	Encoding    string      `json:"encoding,omitempty"`     // utf-8, latin1, etc.
	SkipRows    int         `json:"skipRows,omitempty"`     // skip N rows from top
	MaxRecords    int `json:"maxRecords,omitempty"`    // limit (0 = unlimited)
	MaxFileSizeMB int `json:"maxFileSizeMB,omitempty"` // file size gate in MB (0 = default 100 MB, hard cap 500 MB)
	TrimFields      bool        `json:"trimFields"`                     // trim whitespace from values
	QuoteChar       string      `json:"quoteChar,omitempty"`            // quote character (default: `"`)
	SheetName       string      `json:"sheetName,omitempty"`            // xlsx/xls: which sheet to parse (default: first sheet)
	SheetIndex      int         `json:"sheetIndex,omitempty"`           // xlsx/xls: 0-based sheet index (used if sheetName is empty)
	ContentEncoding string      `json:"contentEncoding,omitempty"`      // "base64" if content is base64-encoded binary
	AutoDetect      bool        `json:"autoDetect,omitempty"`           // auto-detect format, delimiter, header
	Template        string      `json:"template,omitempty"`             // OOB template name (e.g., "cclf1", "nacha_entry")

	// Source configuration — WHERE to get the file content
	// "field" (default): read raw file content from SourceField pipeline variable
	// "local_path":      read from FilePath on the server's local file system (or container volume mount)
	// "field_as_path":   read a file URI from SourceField, then resolve it:
	//                      s3://bucket/key  → AWS S3 (credentials from interface connectivity config or IAM role)
	//                      https://...      → HTTP GET
	//                      file:///...      → local filesystem
	SourceType  string `json:"sourceType,omitempty"`   // "field" | "local_path" | "field_as_path"
	FilePath    string `json:"filePath,omitempty"`      // absolute path or glob (e.g. /data/claims/*.csv)
	BatchMode   bool   `json:"batchMode,omitempty"`     // true = process all files matching FilePath/FilePattern
	FilePattern string `json:"filePattern,omitempty"`   // sub-glob when FilePath is a directory (e.g. "*.csv")
}

// ColumnDef defines a column for fixed-width file parsing
type ColumnDef struct {
	Name   string `json:"name"`   // column/field name
	Start  int    `json:"start"`  // 1-based start position
	Length int    `json:"length"` // character length
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
