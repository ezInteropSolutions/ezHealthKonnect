package controllers

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq" // PostgreSQL driver
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	// Add other database drivers as needed:
	// _ "github.com/go-sql-driver/mysql"
	// _ "github.com/denisenkom/go-mssqldb"
)

// DatabaseTestController handles testing database queries before pipeline execution
type DatabaseTestController struct {
	db *sql.DB
}

// NewDatabaseTestController creates a new database test controller
func NewDatabaseTestController(db *sql.DB) *DatabaseTestController {
	return &DatabaseTestController{db: db}
}

// TestQueryRequest represents the request to test a database query
type TestQueryRequest struct {
	DatabaseType     string            `json:"databaseType"`     // postgresql, mysql, sqlserver, oracle
	ConnectionString string            `json:"connectionString"` // Database connection string
	ConnectionName   string            `json:"connectionName"`   // Named connection (future feature)
	// Individual connection fields (alternative to connectionString)
	DBHost     string `json:"dbHost"`
	DBPort     int    `json:"dbPort"`
	DBName     string `json:"dbName"`
	DBUser     string `json:"dbUser"`
	DBPassword string `json:"dbPassword"`
	Query      string            `json:"query"`      // SQL query to test
	QueryParams      map[string]string `json:"queryParams"`      // Parameter name -> field path mapping
	TestParams       map[string]string `json:"testParams"`       // Parameter name -> test value mapping
}

// TestQuery tests a database query with sample parameters
// POST /api/database/test-query
func (c *DatabaseTestController) TestQuery(ctx *gin.Context) {
	var req TestQueryRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Invalid request: " + err.Error(),
		})
		return
	}

	// Validate required fields
	if req.Query == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Query is required",
		})
		return
	}

	if req.DatabaseType == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Database type is required",
		})
		return
	}

	// Build connection string from individual fields if not provided
	if req.ConnectionString == "" && req.DBHost != "" && req.DBPort > 0 && req.DBName != "" && req.DBUser != "" {
		req.ConnectionString = buildConnectionString(req.DatabaseType, req.DBHost, req.DBPort, req.DBName, req.DBUser, req.DBPassword)
	}

	// Get database connection
	testDB, err := c.getTestConnection(req)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Connection failed: " + err.Error(),
		})
		return
	}
	defer testDB.Close()

	// Build parameter values from test params
	params := make([]interface{}, 0)

	// Sort parameters by key to ensure correct order for positional binding
	// Collect parameter names
	paramNames := make([]string, 0, len(req.QueryParams))
	for paramName := range req.QueryParams {
		paramNames = append(paramNames, paramName)
	}
	// Sort alphabetically (ensures "1", "2", "3" are in correct order)
	sort.Strings(paramNames)

	// Extract values in sorted order
	for _, paramName := range paramNames {
		testValue, exists := req.TestParams[paramName]
		if exists && testValue != "" {
			params = append(params, testValue)
		} else {
			// Use NULL for missing parameters
			params = append(params, nil)
		}
	}

	// Execute query with timeout
	queryCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	rows, err := testDB.QueryContext(queryCtx, req.Query, params...)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   "Query execution failed: " + err.Error(),
		})
		return
	}
	defer rows.Close()

	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Failed to get columns: " + err.Error(),
		})
		return
	}

	// Build result set (limit to 100 rows for safety)
	results := make([]map[string]interface{}, 0)
	rowCount := 0
	maxRows := 100

	for rows.Next() && rowCount < maxRows {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"error":   "Failed to scan row: " + err.Error(),
			})
			return
		}

		row := make(map[string]interface{})
		for i, colName := range columns {
			val := values[i]

			// Convert []byte to string for easier handling in JSON
			if b, ok := val.([]byte); ok {
				row[colName] = string(b)
			} else {
				row[colName] = val
			}
		}

		results = append(results, row)
		rowCount++
	}

	if err := rows.Err(); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   "Error iterating rows: " + err.Error(),
		})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    results,
		"count":   len(results),
		"columns": columns, // Always return column names, even if 0 rows
	})
}

// getTestConnection establishes a database connection for testing
func (c *DatabaseTestController) getTestConnection(req TestQueryRequest) (*sql.DB, error) {
	// If connection name specified, look up from database_connections table
	// TODO: Implement connection registry/manager
	if req.ConnectionName != "" {
		return nil, fmt.Errorf("named connections not yet implemented - please use connection string")
	}

	// Use provided connection string
	if req.ConnectionString == "" {
		return nil, fmt.Errorf("connectionString is required")
	}

	driverName := c.getDriverName(req.DatabaseType)
	db, err := sql.Open(driverName, req.ConnectionString)
	if err != nil {
		return nil, fmt.Errorf("failed to open connection: %w", err)
	}

	// Set connection limits for test queries
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(0)
	db.SetConnMaxLifetime(30 * time.Second)

	// Test connection with ping
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

// getDriverName returns the SQL driver name for the database type
func (c *DatabaseTestController) getDriverName(dbType string) string {
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
func buildConnectionString(dbType, host string, port int, dbName, user, password string) string {
	switch strings.ToLower(dbType) {
	case "postgresql", "postgres":
		return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
			host, port, user, password, dbName)
	case "mysql":
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s",
			user, password, host, port, dbName)
	case "sqlserver", "mssql":
		return fmt.Sprintf("sqlserver://%s:%s@%s:%d?database=%s",
			user, password, host, port, dbName)
	case "oracle":
		return fmt.Sprintf("oracle://%s:%s@%s:%d/%s",
			user, password, host, port, dbName)
	default:
		return fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
			host, port, user, password, dbName)
	}
}

// MongoDBSchemaRequest represents the request to fetch MongoDB collection schema
type MongoDBSchemaRequest struct {
	DBHost     string `json:"dbHost"`
	DBPort     int    `json:"dbPort"`
	DBName     string `json:"dbName"`
	DBUser     string `json:"dbUser"`
	DBPassword string `json:"dbPassword"`
	Collection string `json:"collection"`
}

// MongoDBField represents a field in the MongoDB collection
type MongoDBField struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category,omitempty"`
}

// GetMongoDBCollectionSchema fetches field names from a MongoDB collection by sampling documents
// POST /api/database/mongodb-schema
func (c *DatabaseTestController) GetMongoDBCollectionSchema(ctx *gin.Context) {
	var req MongoDBSchemaRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Invalid request: %v", err),
		})
		return
	}

	// Build MongoDB connection string with authSource=admin for root users
	var connectionString string
	if req.DBUser != "" && req.DBPassword != "" {
		connectionString = fmt.Sprintf("mongodb://%s:%s@%s:%d/%s?authSource=admin",
			req.DBUser, req.DBPassword, req.DBHost, req.DBPort, req.DBName)
	} else {
		connectionString = fmt.Sprintf("mongodb://%s:%d/%s",
			req.DBHost, req.DBPort, req.DBName)
	}

	// Connect to MongoDB
	clientOptions := options.Client().ApplyURI(connectionString)
	mongoCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(mongoCtx, clientOptions)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Failed to connect to MongoDB: %v", err),
		})
		return
	}
	defer client.Disconnect(mongoCtx)

	// Ping to verify connection
	if err := client.Ping(mongoCtx, nil); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Failed to ping MongoDB: %v", err),
		})
		return
	}

	// Get collection
	collection := client.Database(req.DBName).Collection(req.Collection)

	// Sample documents to extract schema (sample first 100 documents)
	cursor, err := collection.Find(mongoCtx, bson.M{}, options.Find().SetLimit(100))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Failed to query collection: %v", err),
		})
		return
	}
	defer cursor.Close(mongoCtx)

	// Collect all unique field names
	fieldMap := make(map[string]bool)
	for cursor.Next(mongoCtx) {
		var document bson.M
		if err := cursor.Decode(&document); err != nil {
			continue
		}

		// Extract all field names recursively
		extractFieldNames(document, "", fieldMap)
	}

	// Convert to sorted list of fields
	fields := make([]MongoDBField, 0, len(fieldMap))
	for fieldName := range fieldMap {
		fields = append(fields, MongoDBField{
			Name:     fieldName,
			Type:     "string",  // Type detection can be enhanced later
			Category: categorizeField(fieldName),
		})
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"fields":  fields,
		"count":   len(fields),
	})
}

// extractFieldNames recursively extracts all field names from a BSON document
func extractFieldNames(doc bson.M, prefix string, fieldMap map[string]bool) {
	for key, value := range doc {
		fieldName := key
		if prefix != "" {
			fieldName = prefix + "." + key
		}

		fieldMap[fieldName] = true

		// Recursively extract nested object fields
		if nestedDoc, ok := value.(bson.M); ok {
			extractFieldNames(nestedDoc, fieldName, fieldMap)
		}
	}
}

// categorizeField attempts to categorize a field based on its name
func categorizeField(fieldName string) string {
	lowerName := strings.ToLower(fieldName)

	// Identity fields
	if strings.Contains(lowerName, "mrn") || strings.Contains(lowerName, "ssn") ||
		strings.Contains(lowerName, "id") {
		return "Identity"
	}

	// Demographics
	if strings.Contains(lowerName, "name") || strings.Contains(lowerName, "dob") ||
		strings.Contains(lowerName, "dateofbirth") || strings.Contains(lowerName, "gender") ||
		strings.Contains(lowerName, "age") {
		return "Demographics"
	}

	// Contact
	if strings.Contains(lowerName, "phone") || strings.Contains(lowerName, "email") ||
		strings.Contains(lowerName, "address") || strings.Contains(lowerName, "city") ||
		strings.Contains(lowerName, "state") || strings.Contains(lowerName, "zip") {
		return "Contact"
	}

	// Insurance
	if strings.Contains(lowerName, "insurance") || strings.Contains(lowerName, "provider") ||
		strings.Contains(lowerName, "member") || strings.Contains(lowerName, "group") {
		return "Insurance"
	}

	// Clinical
	if strings.Contains(lowerName, "allerg") || strings.Contains(lowerName, "condition") ||
		strings.Contains(lowerName, "diagnosis") || strings.Contains(lowerName, "medication") ||
		strings.Contains(lowerName, "vital") {
		return "Clinical"
	}

	// Administrative
	if strings.Contains(lowerName, "status") || strings.Contains(lowerName, "facility") ||
		strings.Contains(lowerName, "visit") || strings.Contains(lowerName, "admission") {
		return "Administrative"
	}

	return "Custom"
}
