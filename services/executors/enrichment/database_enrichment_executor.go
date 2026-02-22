package enrichment

import (
	"context"
	"database/sql"
	"encoding/json"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	// SQL database drivers
	_ "github.com/lib/pq"                // PostgreSQL
	_ "github.com/go-sql-driver/mysql"   // MySQL
	_ "github.com/microsoft/go-mssqldb"  // SQL Server
	_ "github.com/sijms/go-ora/v2"       // Oracle

	// NoSQL drivers
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/bson"
	"github.com/redis/go-redis/v9"
)

// Global connection pool with cache and reuse
var (
	dbConnectionPool = make(map[string]*sql.DB)
	dbPoolMutex      sync.RWMutex
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

	base := executors.NewBaseExecutor("enrichment.database", metadata)

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
	log.Printf("   🔍 DEBUG: DatabaseType = %s", config.DatabaseType)
	log.Printf("   🔍 DEBUG: TargetPath = %s", config.TargetPath)

	// Execute query with timeout
	queryCtx, cancel := context.WithTimeout(ctx, time.Duration(config.TimeoutMs)*time.Millisecond)
	defer cancel()

	var result interface{}

	// Route to appropriate database type handler
	dbType := strings.ToLower(config.DatabaseType)
	switch dbType {
	case "mongodb", "mongo":
		result, err = e.executeMongoQuery(queryCtx, config, inputData)
	case "redis":
		result, err = e.executeRedisQuery(queryCtx, config, inputData)
	case "postgresql", "postgres", "mysql", "sqlserver", "mssql", "oracle":
		result, err = e.executeSQLQuery(queryCtx, config, inputData)
	default:
		err = fmt.Errorf("unsupported database type: %s", config.DatabaseType)
	}

	if err != nil {
		log.Printf("⚠️  Query failed: %v", err)

		// Apply default value if configured (executor-level fallback)
		if config.DefaultValue != nil {
			e.storeResult(inputData, config.TargetPath, config.DefaultValue)
			// STANDARDIZED: Default value used + execution details
			e.SetStepOutputWithDetails(inputData, map[string]interface{}{
				"value": config.DefaultValue,
			}, map[string]interface{}{
				"database_type":      config.DatabaseType,
				"default_value_used": true,
				"error":              err.Error(),
				"target_path":        config.TargetPath,
			})
		} else {
			// STANDARDIZED: No variables + execution details with error
			e.SetStepOutputWithDetails(inputData, map[string]interface{}{}, map[string]interface{}{
				"database_type": config.DatabaseType,
				"error":         err.Error(),
				"target_path":   config.TargetPath,
			})
		}

		// Always return error — pipeline service decides retry/catch behavior
		e.PostExecute(ctx, step, err, time.Since(start))
		return inputData, err
	}

	// Map results if configured (for SQL results, NoSQL results come pre-mapped)
	var mappedResult interface{}
	if resultArray, ok := result.([]map[string]interface{}); ok {
		mappedResult = e.mapResults(resultArray, config.ResultMapping)
	} else {
		mappedResult = result // Already in correct format (MongoDB, Redis, etc.)
	}

	// Store result in target path
	targetPath := config.TargetPath
	if targetPath == "" {
		targetPath = "enriched.database"
	}

	e.storeResult(inputData, targetPath, mappedResult)

	// Build variables (the enriched data) and execution details
	// CONSISTENT STRUCTURE: Always use "rows" array format for predictable output
	var variables map[string]interface{}
	var rowCount int

	if resultArray, ok := mappedResult.([]map[string]interface{}); ok {
		// Multiple rows - use as-is
		variables = map[string]interface{}{"rows": resultArray}
		rowCount = len(resultArray)
	} else if resultMap, ok := mappedResult.(map[string]interface{}); ok {
		// Single row - wrap in array for consistency
		variables = map[string]interface{}{"rows": []map[string]interface{}{resultMap}}
		rowCount = 1
	} else {
		// Other types - wrap in result
		variables = map[string]interface{}{"rows": []interface{}{mappedResult}}
		rowCount = 1
	}

	// STANDARDIZED: Variables (query result) + execution details
	e.SetStepOutputWithDetails(inputData, variables, map[string]interface{}{
		"database_type": config.DatabaseType,
		"target_path":   targetPath,
		"rows_returned": rowCount,
	})

	// Log success
	if resultArray, ok := result.([]map[string]interface{}); ok {
		log.Printf("✅ [Database Enrichment] Query successful, %d rows returned", len(resultArray))
	} else {
		log.Printf("✅ [Database Enrichment] Query successful")
	}
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

	// Validate required fields based on database type
	dbType := strings.ToLower(config.DatabaseType)
	switch dbType {
	case "mongodb", "mongo":
		if config.Collection == "" {
			return nil, fmt.Errorf("collection is required for MongoDB")
		}
	case "redis":
		if config.RedisKey == "" {
			return nil, fmt.Errorf("redisKey is required for Redis")
		}
	default:
		// SQL databases require query
		if config.Query == "" {
			return nil, fmt.Errorf("query is required for SQL databases")
		}
	}

	// Build connection string from individual fields if not provided
	// Skip for Redis and MongoDB - they handle connections internally
	if config.ConnectionString == "" && config.ConnectionName == "" && dbType != "redis" && dbType != "mongodb" && dbType != "mongo" {
		if config.DBHost != "" && config.DBPort > 0 && config.DBName != "" && config.DBUser != "" {
			// Build connection string from individual fields (SQL databases only)
			config.ConnectionString = e.buildConnectionString(&config)
			log.Printf("   ℹ️  Built connection string from individual fields: %s:****@%s:%d/%s",
				config.DBUser, config.DBHost, config.DBPort, config.DBName)
		} else {
			// Use main application database if no connection specified
			log.Printf("   ℹ️  No connection specified, using main application database")
		}
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

// getConnection gets the database connection with connection pooling
func (e *DatabaseEnrichmentExecutor) getConnection(config *models.DatabaseEnrichmentConfigV2) (*sql.DB, error) {
	// If connection name is specified, look it up from stored connections
	if config.ConnectionName != "" {
		// TODO: Implement connection pool/registry for named connections
		log.Printf("   ⚠️  Named connections not yet implemented, using main DB")
		return e.db, nil
	}

	// If connection string is specified, check connection pool first
	if config.ConnectionString != "" {
		// Generate cache key from connection string + database type
		cacheKey := fmt.Sprintf("%s::%s", config.DatabaseType, config.ConnectionString)

		// Check if connection already exists in pool (READ lock)
		dbPoolMutex.RLock()
		if db, exists := dbConnectionPool[cacheKey]; exists {
			dbPoolMutex.RUnlock()
			// Verify connection is still alive
			if err := db.Ping(); err == nil {
				log.Printf("   ♻️  Reusing cached database connection for %s", config.DatabaseType)
				return db, nil
			}
			// Connection dead, need to recreate
			log.Printf("   ⚠️  Cached connection dead, recreating for %s", config.DatabaseType)
		} else {
			dbPoolMutex.RUnlock()
		}

		// Create new connection (WRITE lock)
		dbPoolMutex.Lock()
		defer dbPoolMutex.Unlock()

		// Double-check in case another goroutine just created it
		if db, exists := dbConnectionPool[cacheKey]; exists {
			if err := db.Ping(); err == nil {
				return db, nil
			}
		}

		log.Printf("   🔌 Creating new database connection for %s", config.DatabaseType)
		db, err := sql.Open(e.getDriverName(config.DatabaseType), config.ConnectionString)
		if err != nil {
			return nil, fmt.Errorf("failed to open database connection: %w", err)
		}

		// Configure connection pool settings for performance
		db.SetMaxOpenConns(10)                 // Max 10 concurrent connections per database
		db.SetMaxIdleConns(5)                  // Keep 5 idle connections ready
		db.SetConnMaxLifetime(5 * time.Minute) // Recycle connections every 5 minutes

		// Test connection with timeout
		pingCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		if err := db.PingContext(pingCtx); err != nil {
			db.Close()
			return nil, fmt.Errorf("failed to ping database: %w", err)
		}

		// Store in pool
		dbConnectionPool[cacheKey] = db
		log.Printf("   ✅ Database connection cached for reuse (%s)", config.DatabaseType)

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

// buildConnectionString constructs a connection string from individual fields
func (e *DatabaseEnrichmentExecutor) buildConnectionString(config *models.DatabaseEnrichmentConfigV2) string {
	switch strings.ToLower(config.DatabaseType) {
	case "postgresql", "postgres":
		// PostgreSQL format: host=X port=Y user=Z password=W dbname=N sslmode=disable
		return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
			config.DBHost, config.DBPort, config.DBUser, config.DBPassword, config.DBName)
	case "mysql":
		// MySQL format: user:password@tcp(host:port)/database
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s",
			config.DBUser, config.DBPassword, config.DBHost, config.DBPort, config.DBName)
	case "sqlserver", "mssql":
		// SQL Server format: sqlserver://user:password@host:port?database=dbname
		return fmt.Sprintf("sqlserver://%s:%s@%s:%d?database=%s",
			config.DBUser, config.DBPassword, config.DBHost, config.DBPort, config.DBName)
	case "oracle":
		// Oracle format: oracle://user:password@host:port/service_name
		return fmt.Sprintf("oracle://%s:%s@%s:%d/%s",
			config.DBUser, config.DBPassword, config.DBHost, config.DBPort, config.DBName)
	case "mongodb":
		// MongoDB format: mongodb://user:password@host:port/database?authSource=admin
		if config.DBUser != "" && config.DBPassword != "" {
			return fmt.Sprintf("mongodb://%s:%s@%s:%d/%s?authSource=admin",
				config.DBUser, config.DBPassword, config.DBHost, config.DBPort, config.DBName)
		}
		// MongoDB without authentication
		return fmt.Sprintf("mongodb://%s:%d/%s",
			config.DBHost, config.DBPort, config.DBName)
	case "redis":
		// Redis format: redis://[:password@]host:port/db
		dbNum := 0 // Default database
		if config.DBName != "" {
			fmt.Sscanf(config.DBName, "%d", &dbNum)
		}
		if config.DBPassword != "" {
			return fmt.Sprintf("redis://:%s@%s:%d/%d",
				config.DBPassword, config.DBHost, config.DBPort, dbNum)
		}
		return fmt.Sprintf("redis://%s:%d/%d",
			config.DBHost, config.DBPort, dbNum)
	default:
		// Default to PostgreSQL format
		return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
			config.DBHost, config.DBPort, config.DBUser, config.DBPassword, config.DBName)
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

	// For MySQL/PostgreSQL with positional parameters, we need to maintain order
	// Parameters should be named as "1", "2", "3", etc. for positional binding

	// Collect and sort parameter names to ensure consistent order (1, 2, 3, ... or alphabetically)
	paramNames := make([]string, 0, len(config.QueryParams))
	for paramName := range config.QueryParams {
		paramNames = append(paramNames, paramName)
	}

	// Sort parameter names to ensure consistent order
	// This ensures "1", "2", "3" are processed in the correct order for MySQL ?
	sort.Strings(paramNames)

	params := make([]interface{}, 0, len(paramNames))

	for _, paramName := range paramNames {
		fieldPath := config.QueryParams[paramName]
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

// ===============================================================
// DATABASE-SPECIFIC EXECUTION METHODS
// ===============================================================

// executeSQLQuery executes queries for SQL databases (PostgreSQL, MySQL, SQL Server, Oracle)
func (e *DatabaseEnrichmentExecutor) executeSQLQuery(
	ctx context.Context,
	config *models.DatabaseEnrichmentConfigV2,
	inputData map[string]interface{},
) ([]map[string]interface{}, error) {
	log.Printf("   🔍 DEBUG: SQL Query = %s", config.Query)
	log.Printf("   🔍 DEBUG: QueryParams = %+v", config.QueryParams)

	// Get database connection
	dbConn, err := e.getConnection(config)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Build query parameters
	params, err := e.buildQueryParams(config, inputData)
	if err != nil {
		return nil, fmt.Errorf("failed to build query params: %w", err)
	}

	// Execute query
	return e.executeQuery(ctx, dbConn, config, params)
}

// executeMongoQuery executes queries for MongoDB
func (e *DatabaseEnrichmentExecutor) executeMongoQuery(
	ctx context.Context,
	config *models.DatabaseEnrichmentConfigV2,
	inputData map[string]interface{},
) (interface{}, error) {
	log.Printf("   🔍 DEBUG: MongoDB Collection = %s", config.Collection)
	log.Printf("   🔍 DEBUG: MongoDB Filter = %+v", config.Filter)

	// Build connection string
	connectionString := config.ConnectionString
	if connectionString == "" && config.DBHost != "" {
		// Build MongoDB connection string: mongodb://user:pass@host:port/database
		if config.DBUser != "" && config.DBPassword != "" {
			connectionString = fmt.Sprintf("mongodb://%s:%s@%s:%d/%s",
				config.DBUser, config.DBPassword, config.DBHost, config.DBPort, config.DBName)
		} else {
			connectionString = fmt.Sprintf("mongodb://%s:%d/%s",
				config.DBHost, config.DBPort, config.DBName)
		}
	}

	// Connect to MongoDB
	clientOptions := options.Client().ApplyURI(connectionString)
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}
	defer client.Disconnect(ctx)

	// Ping to verify connection
	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	// Get database and collection
	dbName := config.DBName
	if dbName == "" {
		return nil, fmt.Errorf("database name is required for MongoDB")
	}
	if config.Collection == "" {
		return nil, fmt.Errorf("collection name is required for MongoDB")
	}

	collection := client.Database(dbName).Collection(config.Collection)

	var results []map[string]interface{}

	// Check if using aggregation pipeline (advanced mode) or simple find (visual mode)
	if config.MongoDBQueryMode == "advanced" && len(config.AggregationPipeline) > 0 {
		log.Printf("   ⚡ MongoDB Advanced Mode: Using aggregation pipeline with %d stages", len(config.AggregationPipeline))

		// Build aggregation pipeline with field placeholder substitution
		pipeline := e.buildAggregationPipeline(config.AggregationPipeline, inputData)
		log.Printf("   📋 MongoDB aggregation pipeline after substitution: %+v", pipeline)

		// Execute aggregation
		cursor, err := collection.Aggregate(ctx, pipeline)
		if err != nil {
			return nil, fmt.Errorf("MongoDB aggregation failed: %w", err)
		}
		defer cursor.Close(ctx)

		// Decode results
		for cursor.Next(ctx) {
			var result map[string]interface{}
			if err := cursor.Decode(&result); err != nil {
				return nil, fmt.Errorf("failed to decode MongoDB aggregation result: %w", err)
			}
			results = append(results, result)
		}

		if err := cursor.Err(); err != nil {
			return nil, fmt.Errorf("MongoDB aggregation cursor error: %w", err)
		}

		log.Printf("   ✅ MongoDB aggregation returned %d documents", len(results))

		// Return single document or array
		if len(results) == 1 {
			return results[0], nil
		}
		return results, nil
	}

	// Visual mode: Use simple find query
	log.Printf("   👁️ MongoDB Visual Mode: Using simple find query")

	// Build filter - replace field placeholders with actual values
	filter := e.buildMongoFilter(config.Filter, inputData)
	log.Printf("   📋 MongoDB filter after substitution: %+v", filter)

	// Build find options with projection if specified
	findOptions := options.Find()
	if config.Projection != nil && len(config.Projection) > 0 {
		findOptions.SetProjection(config.Projection)
	}

	cursor, err := collection.Find(ctx, filter, findOptions)
	if err != nil {
		return nil, fmt.Errorf("MongoDB find failed: %w", err)
	}
	defer cursor.Close(ctx)

	// Decode results
	for cursor.Next(ctx) {
		var result map[string]interface{}
		if err := cursor.Decode(&result); err != nil {
			return nil, fmt.Errorf("failed to decode MongoDB result: %w", err)
		}
		results = append(results, result)
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("MongoDB cursor error: %w", err)
	}

	log.Printf("   ✅ MongoDB query returned %d documents", len(results))

	// Return single document or array
	if len(results) == 1 {
		return results[0], nil
	}
	return results, nil
}

// executeRedisQuery executes queries for Redis
func (e *DatabaseEnrichmentExecutor) executeRedisQuery(
	ctx context.Context,
	config *models.DatabaseEnrichmentConfigV2,
	inputData map[string]interface{},
) (interface{}, error) {
	log.Printf("   🔍 DEBUG: Redis Key = %s", config.RedisKey)
	log.Printf("   🔍 DEBUG: Redis Command = %s", config.RedisCommand)

	// Build connection string
	connectionString := config.ConnectionString
	if connectionString == "" && config.DBHost != "" {
		// Build Redis connection string: redis://host:port/db
		dbNum := 0 // Default database
		if config.DBName != "" {
			// Redis DB name is a number (0-15), try to parse
			fmt.Sscanf(config.DBName, "%d", &dbNum)
		}

		if config.DBPassword != "" {
			connectionString = fmt.Sprintf("redis://:%s@%s:%d/%d",
				config.DBPassword, config.DBHost, config.DBPort, dbNum)
		} else {
			connectionString = fmt.Sprintf("redis://%s:%d/%d",
				config.DBHost, config.DBPort, dbNum)
		}
	}

	// Parse Redis connection string
	opt, err := redis.ParseURL(connectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Redis connection string: %w", err)
	}

	// Connect to Redis
	client := redis.NewClient(opt)
	defer client.Close()

	// Ping to verify connection
	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to ping Redis: %w", err)
	}

	// Build Redis key - replace placeholders with field values
	redisKey := e.buildRedisKey(config.RedisKey, inputData)

	log.Printf("   📋 Redis key after substitution: %s", redisKey)

	// Execute Redis command
	command := strings.ToUpper(config.RedisCommand)
	if command == "" {
		command = "GET" // Default command
	}

	var result interface{}

	switch command {
	case "GET":
		val, err := client.Get(ctx, redisKey).Result()
		if err == redis.Nil {
			return nil, fmt.Errorf("key not found: %s", redisKey)
		} else if err != nil {
			return nil, fmt.Errorf("GET failed: %w", err)
		}

		// Try to parse as JSON
		var jsonResult interface{}
		if err := json.Unmarshal([]byte(val), &jsonResult); err == nil {
			result = jsonResult
		} else {
			result = val // Return as string if not JSON
		}

	case "HGETALL":
		val, err := client.HGetAll(ctx, redisKey).Result()
		if err != nil {
			return nil, fmt.Errorf("HGETALL failed: %w", err)
		}

		// Convert map[string]string to map[string]interface{}
		resultMap := make(map[string]interface{})
		for k, v := range val {
			resultMap[k] = v
		}
		result = resultMap

	case "SMEMBERS":
		val, err := client.SMembers(ctx, redisKey).Result()
		if err != nil {
			return nil, fmt.Errorf("SMEMBERS failed: %w", err)
		}

		// Convert []string to []interface{}
		resultArray := make([]interface{}, len(val))
		for i, v := range val {
			resultArray[i] = v
		}
		result = resultArray

	case "LRANGE":
		val, err := client.LRange(ctx, redisKey, 0, -1).Result()
		if err != nil {
			return nil, fmt.Errorf("LRANGE failed: %w", err)
		}

		// Convert []string to []interface{}
		resultArray := make([]interface{}, len(val))
		for i, v := range val {
			resultArray[i] = v
		}
		result = resultArray

	default:
		return nil, fmt.Errorf("unsupported Redis command: %s", command)
	}

	log.Printf("   ✅ Redis query successful")
	return result, nil
}

// buildMongoFilter replaces field placeholders in MongoDB filter with actual values
func (e *DatabaseEnrichmentExecutor) buildMongoFilter(
	filter map[string]interface{},
	inputData map[string]interface{},
) bson.M {
	result := bson.M{}

	for key, value := range filter {
		// If value is a string starting with {field}, replace it
		if strValue, ok := value.(string); ok && strings.HasPrefix(strValue, "{") && strings.HasSuffix(strValue, "}") {
			fieldPath := strings.Trim(strValue, "{}")
			actualValue := executors.GetNestedValue(inputData, fieldPath)
			result[key] = actualValue
		} else {
			result[key] = value
		}
	}

	return result
}

// buildRedisKey replaces placeholders in Redis key pattern with actual values
func (e *DatabaseEnrichmentExecutor) buildRedisKey(
	keyPattern string,
	inputData map[string]interface{},
) string {
	result := keyPattern

	// Find all placeholders - support both {{ fieldPath }} and {fieldPath}
	for {
		// Try to find double-brace placeholder first {{ }}
		start := strings.Index(result, "{{")
		isDoubleBrace := false
		if start != -1 {
			isDoubleBrace = true
		} else {
			// Try single brace {
			start = strings.Index(result, "{")
			if start == -1 {
				break
			}
		}

		// Find matching closing brace
		var end int
		if isDoubleBrace {
			end = strings.Index(result[start:], "}}")
			if end == -1 {
				break
			}
			// Extract field path (trim {{ }} and spaces)
			fieldPath := strings.TrimSpace(result[start+2 : start+end])
			value := executors.GetNestedValue(inputData, fieldPath)

			// Convert value to string
			var strValue string
			if value != nil {
				strValue = fmt.Sprintf("%v", value)
			}

			// Replace {{ fieldPath }} with value
			result = result[:start] + strValue + result[start+end+2:]
		} else {
			end = strings.Index(result[start:], "}")
			if end == -1 {
				break
			}
			// Extract field path (trim { } and spaces)
			fieldPath := strings.TrimSpace(result[start+1 : start+end])
			value := executors.GetNestedValue(inputData, fieldPath)

			// Convert value to string
			var strValue string
			if value != nil {
				strValue = fmt.Sprintf("%v", value)
			}

			// Replace {fieldPath} with value
			result = result[:start] + strValue + result[start+end+1:]
		}
	}

	return result
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
	}
}

// buildAggregationPipeline replaces field placeholders in MongoDB aggregation pipeline
func (e *DatabaseEnrichmentExecutor) buildAggregationPipeline(
	pipeline []map[string]interface{},
	inputData map[string]interface{},
) []interface{} {
	result := make([]interface{}, len(pipeline))

	for i, stage := range pipeline {
		result[i] = e.substituteFieldPlaceholders(stage, inputData)
	}

	return result
}

// substituteFieldPlaceholders recursively substitutes {fieldPath} placeholders with actual values
func (e *DatabaseEnrichmentExecutor) substituteFieldPlaceholders(
	obj interface{},
	inputData map[string]interface{},
) interface{} {
	switch v := obj.(type) {
	case map[string]interface{}:
		// Recursively process map
		result := make(map[string]interface{})
		for key, value := range v {
			result[key] = e.substituteFieldPlaceholders(value, inputData)
		}
		return result

	case []interface{}:
		// Recursively process array
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = e.substituteFieldPlaceholders(item, inputData)
		}
		return result

	case string:
		// Check if string contains {fieldPath} placeholder
		if strings.HasPrefix(v, "{") && strings.HasSuffix(v, "}") {
			fieldPath := strings.Trim(v, "{}")
			actualValue := executors.GetNestedValue(inputData, fieldPath)
			if actualValue != nil {
				return actualValue
			}
		}
		return v

	default:
		// Return as-is (numbers, booleans, etc.)
		return v
	}
}

// ===============================================================
// VARIABLE PROVIDER INTERFACE IMPLEMENTATION
// ===============================================================

// GetOutputVariables returns the list of variables this executor will produce
// Implements the VariableProvider interface for automatic variable discovery
func (e *DatabaseEnrichmentExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	variables := []models.VariableDefinition{}

	// Parse config to extract database query and determine output variables
	log.Printf("🔧 [GetOutputVariables] RAW step.Config: %+v", step.Config)

	config, err := e.parseConfig(step)
	if err != nil {
		log.Printf("⚠️  [DatabaseEnrichment] Failed to parse config for variable discovery: %v", err)
		return variables
	}

	// Determine base path where results will be stored
	basePath := config.TargetPath
	if basePath == "" {
		basePath = "enriched.database"
	}

	// DEBUG: Log the config to see what we're working with
	log.Printf("🔍 [GetOutputVariables] Database type: %s, Query: %s, ResultMapping count: %d",
		config.DatabaseType, config.Query, len(config.ResultMapping))
	if len(config.ResultMapping) > 0 {
		log.Printf("   ResultMapping entries:")
		for dbCol, outField := range config.ResultMapping {
			log.Printf("      %s → %s", dbCol, outField)
		}
	}

	// Extract variables based on database type
	dbType := strings.ToLower(config.DatabaseType)
	switch dbType {
	case "mongodb", "mongo":
		// MongoDB: Extract fields from projection if specified
		if config.Projection != nil && len(config.Projection) > 0 {
			for fieldName := range config.Projection {
				if fieldName == "_id" {
					continue // Skip MongoDB internal ID
				}
				variables = append(variables, models.VariableDefinition{
					Name:         fieldName,
					Path:         fmt.Sprintf("%s.%s", basePath, fieldName),
					DataType:     "string", // Default to string, actual type varies
					Description:  fmt.Sprintf("MongoDB field from %s collection", config.Collection),
					UsageExample: fmt.Sprintf(`getNestedValue(input, "%s.%s")`, basePath, fieldName),
					Category:     "Database",
				})
			}
		} else {
			// No projection specified, return generic enriched_data variable
			variables = append(variables, models.VariableDefinition{
				Name:         "enriched_data",
				Path:         basePath,
				DataType:     "object",
				Description:  fmt.Sprintf("MongoDB document from %s collection", config.Collection),
				UsageExample: fmt.Sprintf(`getNestedValue(input, "%s.fieldName")`, basePath),
				Category:     "Database",
			})
		}

	case "redis":
		// Redis: Single variable based on command type
		command := strings.ToUpper(config.RedisCommand)
		if command == "" {
			command = "GET"
		}

		dataType := "string"
		if command == "HGETALL" {
			dataType = "object"
		} else if command == "SMEMBERS" || command == "LRANGE" {
			dataType = "array"
		}

		variables = append(variables, models.VariableDefinition{
			Name:         "redis_value",
			Path:         basePath,
			DataType:     dataType,
			Description:  fmt.Sprintf("Redis %s result for key: %s", command, config.RedisKey),
			UsageExample: fmt.Sprintf(`getNestedValue(input, "%s")`, basePath),
			Category:     "Database",
		})

	default:
		// SQL databases: Check for Result Mapping first (user-defined output fields)
		if len(config.ResultMapping) > 0 {
			// Use mapped field names as variable names
			log.Printf("✅ [GetOutputVariables] Found %d result mappings for %s", len(config.ResultMapping), config.DatabaseType)
			for dbCol, outputField := range config.ResultMapping {
				log.Printf("   • Mapping: %s → %s", dbCol, outputField)
				variables = append(variables, models.VariableDefinition{
					Name:         outputField,
					Path:         fmt.Sprintf("%s.%s", basePath, outputField),
					DataType:     "string", // Default to string, actual type varies
					Description:  fmt.Sprintf("Database column '%s' from %s query", dbCol, config.DatabaseType),
					UsageExample: fmt.Sprintf(`getNestedValue(input, "%s.%s")`, basePath, outputField),
					Category:     "Database",
					SourceField:  dbCol,
				})
			}
		} else {
			// No result mapping - try to extract column names from SELECT query
			columnNames := e.extractColumnsFromQuery(config.Query)

			if len(columnNames) > 0 {
				// Use original column names as variable names
				log.Printf("✅ [GetOutputVariables] Extracted %d columns from query for %s", len(columnNames), config.DatabaseType)
				for _, colName := range columnNames {
					varDef := e.BuildVariableDefinition(
						colName,
						basePath,
						fmt.Sprintf("Database column from %s query", config.DatabaseType),
						executors.WithCategory("Database"),
					)
					variables = append(variables, varDef)
				}
			} else {
				// Fallback to generic enriched_data variable (SELECT * or unparseable query)
				log.Printf("⚠️  [GetOutputVariables] No result mapping and couldn't parse query - using generic variable for %s", config.DatabaseType)
				variables = append(variables, models.VariableDefinition{
					Name:         "enriched_data",
					Path:         basePath,
					DataType:     "object",
					Description:  fmt.Sprintf("Database enrichment results from %s", config.DatabaseType),
					UsageExample: fmt.Sprintf(`getNestedValue(input, "%s.fieldName")`, basePath),
					Category:     "Database",
				})
			}
		}
	}

	return variables
}

// extractColumnsFromQuery parses a SQL query and extracts column names from SELECT
// This is reused from the controller logic to maintain consistency
func (e *DatabaseEnrichmentExecutor) extractColumnsFromQuery(query string) []string {
	if query == "" {
		return nil
	}

	// Convert to lowercase for easier parsing
	lowerQuery := strings.ToLower(strings.TrimSpace(query))

	// Find SELECT and FROM positions
	selectPos := strings.Index(lowerQuery, "select")
	fromPos := strings.Index(lowerQuery, "from")

	if selectPos == -1 || fromPos == -1 || fromPos <= selectPos {
		return nil
	}

	// Extract column list between SELECT and FROM
	columnsPart := query[selectPos+6 : fromPos] // +6 for "select"
	columnsPart = strings.TrimSpace(columnsPart)

	// Handle SELECT *
	if strings.TrimSpace(columnsPart) == "*" {
		return nil // Can't determine columns from SELECT *
	}

	// Split by comma and extract column names/aliases
	columns := []string{}
	parts := strings.Split(columnsPart, ",")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		// Check for AS alias
		asPos := strings.LastIndex(strings.ToLower(part), " as ")
		if asPos != -1 {
			// Use the alias
			alias := strings.TrimSpace(part[asPos+4:])
			columns = append(columns, alias)
			continue
		}

		// Check for space-separated alias (e.g., "column_name alias")
		spaceParts := strings.Fields(part)
		if len(spaceParts) > 1 {
			// Last part is likely the alias
			columns = append(columns, spaceParts[len(spaceParts)-1])
		} else {
			// Extract column name after last dot (for table.column)
			dotPos := strings.LastIndex(part, ".")
			if dotPos != -1 {
				columns = append(columns, part[dotPos+1:])
			} else {
				columns = append(columns, part)
			}
		}
	}

	return columns
}

