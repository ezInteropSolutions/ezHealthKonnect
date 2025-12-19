package enrichment

import (
	"context"
	"database/sql"
	"encoding/json"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
	// Add other database drivers as needed:
	// _ "github.com/go-sql-driver/mysql"
	// _ "github.com/denisenkom/go-mssqldb"
)

// ===============================================================
// DATABASE ENRICHMENT EXECUTOR
// ===============================================================
// Queries databases (PostgreSQL, MySQL, SQL Server, Oracle) to enrich message data
// Implements Strategy Pattern - concrete strategy for database enrichment

type DatabaseEnrichmentExecutor struct {
	*executors.BaseExecutor
	db *sql.DB // Main application database connection
}

// NewDatabaseEnrichmentExecutor creates a new database enrichment executor
func NewDatabaseEnrichmentExecutor(db *sql.DB) *DatabaseEnrichmentExecutor {
	metadata := models.ExecutorMetadata{
		Name:        "Database Enrichment",
		Description: "Enriches messages by querying databases (PostgreSQL, MySQL, SQL Server, Oracle)",
		Version:     "1.0.0",
		Author:      "ezHealthKonnect",
		Category:    "enrichment",
	}

	base := executors.NewBaseExecutor("pre.enrichment.database", metadata)

	return &DatabaseEnrichmentExecutor{
		BaseExecutor: base,
		db:           db,
	}
}

// Execute performs database enrichment
func (e *DatabaseEnrichmentExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	// Pre-execution validation
	if err := e.PreExecute(ctx, step); err != nil {
		return inputData, err
	}

	// Parse configuration
	config, err := e.parseConfig(step)
	if err != nil {
		e.PostExecute(ctx, step, err, time.Since(start))
		return inputData, err
	}

	log.Printf("🗄️  [Database Enrichment] Executing query on %s", config.DatabaseType)

	// Get database connection
	dbConn, err := e.getConnection(config)
	if err != nil {
		if config.FailOnError {
			e.PostExecute(ctx, step, err, time.Since(start))
			return inputData, err
		}
		log.Printf("⚠️  Failed to connect to database: %v", err)
		if config.DefaultValue != nil {
			e.storeResult(inputData, config.TargetPath, config.DefaultValue)
		}
		e.PostExecute(ctx, step, nil, time.Since(start))
		return inputData, nil
	}

	// Build query parameters
	params, err := e.buildQueryParams(config, inputData)
	if err != nil {
		if config.FailOnError {
			e.PostExecute(ctx, step, err, time.Since(start))
			return inputData, err
		}
		log.Printf("⚠️  Failed to build query params: %v", err)
		e.PostExecute(ctx, step, nil, time.Since(start))
		return inputData, nil
	}

	// Execute query with timeout
	queryCtx, cancel := context.WithTimeout(ctx, time.Duration(config.TimeoutMs)*time.Millisecond)
	defer cancel()

	result, err := e.executeQuery(queryCtx, dbConn, config, params)
	if err != nil {
		if config.FailOnError {
			e.PostExecute(ctx, step, err, time.Since(start))
			return inputData, err
		}
		log.Printf("⚠️  Query failed: %v", err)
		if config.DefaultValue != nil {
			e.storeResult(inputData, config.TargetPath, config.DefaultValue)
		}
		e.PostExecute(ctx, step, nil, time.Since(start))
		return inputData, nil
	}

	// Map results if configured
	mappedResult := e.mapResults(result, config.ResultMapping)

	// Store result in target path
	targetPath := config.TargetPath
	if targetPath == "" {
		targetPath = "enriched.database"
	}

	e.storeResult(inputData, targetPath, mappedResult)

	log.Printf("✅ [Database Enrichment] Query successful, %d rows returned", len(result))
	e.PostExecute(ctx, step, nil, time.Since(start))

	return inputData, nil
}

// Validate checks if the step configuration is valid
func (e *DatabaseEnrichmentExecutor) Validate(step *models.TransformationStep) error {
	_, err := e.parseConfig(step)
	return err
}

// parseConfig parses and validates the step configuration
func (e *DatabaseEnrichmentExecutor) parseConfig(step *models.TransformationStep) (*models.DatabaseEnrichmentConfigV2, error) {
	if step.Config == nil {
		return nil, fmt.Errorf("database enrichment requires configuration")
	}

	// Marshal to JSON then unmarshal to struct for type safety
	configJSON, err := json.Marshal(step.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	var config models.DatabaseEnrichmentConfigV2
	if err := json.Unmarshal(configJSON, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate required fields
	if config.Query == "" {
		return nil, fmt.Errorf("query is required for database enrichment")
	}

	if config.ConnectionString == "" && config.ConnectionName == "" {
		// Use main application database if no connection specified
		log.Printf("   ℹ️  No connection specified, using main application database")
	}

	if config.DatabaseType == "" {
		return nil, fmt.Errorf("databaseType is required")
	}

	// Set defaults
	if config.TimeoutMs == 0 {
		config.TimeoutMs = 3000
	}

	return &config, nil
}

// getConnection gets the database connection
func (e *DatabaseEnrichmentExecutor) getConnection(config *models.DatabaseEnrichmentConfigV2) (*sql.DB, error) {
	// If connection name is specified, look it up from stored connections
	if config.ConnectionName != "" {
		// TODO: Implement connection pool/registry for named connections
		log.Printf("   ⚠️  Named connections not yet implemented, using main DB")
		return e.db, nil
	}

	// If connection string is specified, create a new connection
	if config.ConnectionString != "" {
		db, err := sql.Open(e.getDriverName(config.DatabaseType), config.ConnectionString)
		if err != nil {
			return nil, fmt.Errorf("failed to open database connection: %w", err)
		}

		// Test connection
		if err := db.Ping(); err != nil {
			return nil, fmt.Errorf("failed to ping database: %w", err)
		}

		return db, nil
	}

	// Use main application database as fallback
	return e.db, nil
}

// getDriverName returns the SQL driver name for the database type
func (e *DatabaseEnrichmentExecutor) getDriverName(dbType string) string {
	switch strings.ToLower(dbType) {
	case "postgresql", "postgres":
		return "postgres"
	case "mysql":
		return "mysql"
	case "sqlserver", "mssql":
		return "sqlserver"
	case "oracle":
		return "oracle"
	default:
		return "postgres" // Default to PostgreSQL
	}
}

// buildQueryParams extracts parameter values from the input message
func (e *DatabaseEnrichmentExecutor) buildQueryParams(
	config *models.DatabaseEnrichmentConfigV2,
	inputData map[string]interface{},
) ([]interface{}, error) {
	if len(config.QueryParams) == 0 {
		return nil, nil
	}

	params := make([]interface{}, 0)

	// PostgreSQL uses $1, $2, etc. - extract parameter numbers
	// MySQL uses ? - parameters are positional
	// Named parameters are stored as {"paramName": "fieldPath"}

	for paramName, fieldPath := range config.QueryParams {
		value := executors.GetNestedValue(inputData, fieldPath)
		if value == nil {
			log.Printf("   ⚠️  Parameter %s (field %s) is null", paramName, fieldPath)
		}
		params = append(params, value)
		log.Printf("   📋 Parameter %s = %v (from %s)", paramName, value, fieldPath)
	}

	return params, nil
}

// executeQuery executes the SQL query
func (e *DatabaseEnrichmentExecutor) executeQuery(
	ctx context.Context,
	db *sql.DB,
	config *models.DatabaseEnrichmentConfigV2,
	params []interface{},
) ([]map[string]interface{}, error) {

	// Execute query
	rows, err := db.QueryContext(ctx, config.Query, params...)
	if err != nil {
		return nil, fmt.Errorf("query execution failed: %w", err)
	}
	defer rows.Close()

	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	// Build result set
	results := make([]map[string]interface{}, 0)

	for rows.Next() {
		// Create a slice of interface{}'s to represent each column
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}

		// Scan the result into the column pointers
		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		// Create a map for this row
		row := make(map[string]interface{})
		for i, colName := range columns {
			val := values[i]

			// Convert []byte to string for easier handling
			if b, ok := val.([]byte); ok {
				row[colName] = string(b)
			} else {
				row[colName] = val
			}
		}

		results = append(results, row)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return results, nil
}

// mapResults maps database column names to output field names
func (e *DatabaseEnrichmentExecutor) mapResults(
	results []map[string]interface{},
	mapping map[string]string,
) interface{} {
	if len(mapping) == 0 {
		// No mapping, return results as-is
		if len(results) == 1 {
			return results[0] // Single row, return as object
		}
		return results // Multiple rows, return as array
	}

	// Apply mapping
	mappedResults := make([]map[string]interface{}, 0, len(results))
	for _, row := range results {
		mappedRow := make(map[string]interface{})
		for dbColumn, outputField := range mapping {
			if value, exists := row[dbColumn]; exists {
				mappedRow[outputField] = value
			}
		}
		mappedResults = append(mappedResults, mappedRow)
	}

	if len(mappedResults) == 1 {
		return mappedResults[0] // Single row, return as object
	}
	return mappedResults // Multiple rows, return as array
}

// storeResult stores the enrichment result at the target path
func (e *DatabaseEnrichmentExecutor) storeResult(
	inputData map[string]interface{},
	targetPath string,
	result interface{},
) {
	executors.SetNestedValue(inputData, targetPath, result)
}

// GetConfigSchema returns the JSON schema for configuration
func (e *DatabaseEnrichmentExecutor) GetConfigSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":     "object",
		"required": []string{"databaseType", "query"},
		"properties": map[string]interface{}{
			"databaseType": map[string]interface{}{
				"type":        "string",
				"description": "Database type",
				"enum":        []string{"postgresql", "mysql", "sqlserver", "oracle"},
			},
			"connectionString": map[string]interface{}{
				"type":        "string",
				"description": "Database connection string (optional if using main DB)",
			},
			"query": map[string]interface{}{
				"type":        "string",
				"description": "SQL query with parameter placeholders ($1, $2, etc.)",
			},
			"queryParams": map[string]interface{}{
				"type":        "object",
				"description": "Map parameter names to message field paths",
			},
			"targetPath": map[string]interface{}{
				"type":        "string",
				"description": "Where to store the query results",
				"default":     "enriched.database",
			},
			"resultMapping": map[string]interface{}{
				"type":        "object",
				"description": "Map database column names to output field names",
			},
			"timeoutMs": map[string]interface{}{
				"type":        "integer",
				"description": "Query timeout in milliseconds",
				"default":     3000,
			},
			"cacheResults": map[string]interface{}{
				"type":        "boolean",
				"description": "Cache query results",
				"default":     false,
			},
			"failOnError": map[string]interface{}{
				"type":        "boolean",
				"description": "Stop pipeline if query fails",
				"default":     false,
			},
		},
	}
}

// GetConfigExample returns an example configuration
func (e *DatabaseEnrichmentExecutor) GetConfigExample() map[string]interface{} {
	return map[string]interface{}{
		"databaseType": "postgresql",
		"query":        "SELECT provider_id, first_name, last_name, npi, specialty FROM providers WHERE provider_id = $1",
		"queryParams": map[string]string{
			"1": "PV1.7", // Provider ID from HL7 message
		},
		"targetPath": "enriched.provider",
		"resultMapping": map[string]string{
			"provider_id": "id",
			"first_name":  "firstName",
			"last_name":   "lastName",
			"npi":         "nationalProviderId",
			"specialty":   "specialty",
		},
		"timeoutMs":    3000,
		"cacheResults": true,
		"cacheTTL":     3600,
		"failOnError":  false,
	}
}
