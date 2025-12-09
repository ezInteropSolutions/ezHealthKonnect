// services/connectors/database_base.go
// Base database connector providing common functionality for all database types
// Supports: PostgreSQL, MySQL, SQL Server, Oracle, MongoDB, Snowflake, Databricks

package connectors

import (
	"database/sql"
	"encoding/json"
	"ezhealthkonnect/models"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/lib/pq" // PostgreSQL driver for array support
)

// DatabaseInboundConfig represents common database inbound configuration
type DatabaseInboundConfig struct {
	// Connection
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Database string `json:"database"`
	Username string `json:"username"`
	Password string `json:"password"`

	// SSL/TLS
	SSLMode     string `json:"ssl_mode"`      // disable, require, verify-ca, verify-full
	SSLCertPath string `json:"ssl_cert_path"`

	// Query Configuration
	TableName         string `json:"table_name"`
	Query             string `json:"query"`              // Custom SQL query (optional)
	IncrementalColumn string `json:"incremental_column"` // Column to track (id, updated_at, etc.)
	IncrementalType   string `json:"incremental_type"`   // integer, timestamp, uuid

	// Polling Configuration
	PollingInterval int    `json:"polling_interval"` // seconds
	MaxRecords      int    `json:"max_records"`      // records per poll
	OrderBy         string `json:"order_by"`         // ORDER BY clause

	// After Processing
	AfterProcessing   string `json:"after_processing"`    // update_flag, delete, archive, nothing
	ProcessedFlagCol  string `json:"processed_flag_col"`  // Column to update (for update_flag)
	ProcessedFlagVal  string `json:"processed_flag_val"`  // Value to set (for update_flag)
	ArchiveTable      string `json:"archive_table"`       // Table to move records (for archive)
	DeleteAfterRead   bool   `json:"delete_after_read"`   // Delete records after processing

	// Connection Pool
	MaxOpenConns int `json:"max_open_conns"` // Default: 10
	MaxIdleConns int `json:"max_idle_conns"` // Default: 5

	// Cloud Data Warehouse Specific (Snowflake, Databricks, BigQuery)
	Account        string `json:"account"`         // Snowflake account identifier
	Warehouse      string `json:"warehouse"`       // Snowflake warehouse name
	Schema         string `json:"schema"`          // Schema name
	Role           string `json:"role"`            // Snowflake role
	Region         string `json:"region"`          // Cloud region
	HTTPPath       string `json:"http_path"`       // Databricks HTTP path
	Token          string `json:"token"`           // Access token (Databricks)
	AuthType       string `json:"auth_type"`       // oauth, password, token, key_pair
	PrivateKey     string `json:"private_key"`     // Key pair authentication
	PrivateKeyPass string `json:"private_key_pass"` // Key passphrase
}

// DatabaseOutboundConfig represents common database outbound configuration
type DatabaseOutboundConfig struct {
	// Connection
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Database string `json:"database"`
	Username string `json:"username"`
	Password string `json:"password"`

	// SSL/TLS
	SSLMode     string `json:"ssl_mode"`
	SSLCertPath string `json:"ssl_cert_path"`

	// Write Configuration
	TableName   string `json:"table_name"`
	WriteMode   string `json:"write_mode"`   // insert, upsert, update
	UniqueKey   string `json:"unique_key"`   // Column(s) for upsert (comma-separated)
	BatchSize   int    `json:"batch_size"`   // Records per batch insert

	// Connection Pool
	MaxOpenConns int `json:"max_open_conns"`
	MaxIdleConns int `json:"max_idle_conns"`

	// Cloud Data Warehouse Specific (Snowflake, Databricks, BigQuery)
	Account        string `json:"account"`
	Warehouse      string `json:"warehouse"`
	Schema         string `json:"schema"`
	Role           string `json:"role"`
	Region         string `json:"region"`
	HTTPPath       string `json:"http_path"`
	Token          string `json:"token"`
	AuthType       string `json:"auth_type"`
	PrivateKey     string `json:"private_key"`
	PrivateKeyPass string `json:"private_key_pass"`
}

// BaseDatabaseInbound provides common inbound database functionality
type BaseDatabaseInbound struct {
	*BaseInboundConnector
	config          DatabaseInboundConfig
	db              *sql.DB
	lastProcessed   interface{} // Last incremental value processed
	messageChan     chan<- *models.InboundMessage
	stopChan        chan struct{}
	mu              sync.RWMutex

	// Database-specific methods (implemented by concrete connectors)
	buildConnectionString func(DatabaseInboundConfig) string
	buildIncrementalQuery func(DatabaseInboundConfig, interface{}) string
}

// BaseDatabaseOutbound provides common outbound database functionality
type BaseDatabaseOutbound struct {
	*BaseOutboundConnector
	config DatabaseOutboundConfig
	db     *sql.DB
	mu     sync.RWMutex

	// Database-specific methods (implemented by concrete connectors)
	buildConnectionString func(DatabaseOutboundConfig) string
	buildUpsertQuery     func(string, map[string]interface{}, string) string
}

// ParseDatabaseInboundConfig parses JSON config into DatabaseInboundConfig
func ParseDatabaseInboundConfig(config []byte) (*DatabaseInboundConfig, error) {
	var cfg DatabaseInboundConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse database config: %w", err)
	}

	// Set defaults
	if cfg.PollingInterval == 0 {
		cfg.PollingInterval = 60 // 1 minute default
	}
	if cfg.MaxRecords == 0 {
		cfg.MaxRecords = 100
	}
	if cfg.MaxOpenConns == 0 {
		cfg.MaxOpenConns = 10
	}
	if cfg.MaxIdleConns == 0 {
		cfg.MaxIdleConns = 5
	}
	if cfg.SSLMode == "" {
		cfg.SSLMode = "disable"
	}
	if cfg.AfterProcessing == "" {
		cfg.AfterProcessing = "nothing"
	}

	return &cfg, nil
}

// ParseDatabaseOutboundConfig parses JSON config into DatabaseOutboundConfig
func ParseDatabaseOutboundConfig(config []byte) (*DatabaseOutboundConfig, error) {
	var cfg DatabaseOutboundConfig
	if err := json.Unmarshal(config, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse database config: %w", err)
	}

	// Set defaults
	if cfg.MaxOpenConns == 0 {
		cfg.MaxOpenConns = 10
	}
	if cfg.MaxIdleConns == 0 {
		cfg.MaxIdleConns = 5
	}
	if cfg.SSLMode == "" {
		cfg.SSLMode = "disable"
	}
	if cfg.WriteMode == "" {
		cfg.WriteMode = "insert"
	}
	if cfg.BatchSize == 0 {
		cfg.BatchSize = 50
	}

	return &cfg, nil
}

// ConfigureConnectionPool sets up database connection pool
func ConfigureConnectionPool(db *sql.DB, maxOpen, maxIdle int) {
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(time.Hour)
}

// HandleAfterProcessing executes after-processing action on records
func HandleAfterProcessing(db *sql.DB, config DatabaseInboundConfig, recordIDs []interface{}) error {
	if len(recordIDs) == 0 {
		return nil
	}

	switch config.AfterProcessing {
	case "update_flag":
		// Update processed flag
		if config.ProcessedFlagCol == "" {
			return fmt.Errorf("processed_flag_col required for update_flag mode")
		}
		if config.TableName == "" {
			return fmt.Errorf("table_name required for update_flag mode")
		}

		query := fmt.Sprintf("UPDATE %s SET %s = $1 WHERE id = ANY($2)",
			config.TableName, config.ProcessedFlagCol)
		result, err := db.Exec(query, config.ProcessedFlagVal, pq.Array(recordIDs))
		if err != nil {
			return fmt.Errorf("update_flag query failed: %w", err)
		}
		rowsAffected, _ := result.RowsAffected()
		log.Printf("✅ After-processing: Updated %d records in %s (set %s = %s)",
			rowsAffected, config.TableName, config.ProcessedFlagCol, config.ProcessedFlagVal)
		return nil

	case "delete":
		// Delete records
		if config.TableName == "" {
			return fmt.Errorf("table_name required for delete mode")
		}
		query := fmt.Sprintf("DELETE FROM %s WHERE id = ANY($1)", config.TableName)
		result, err := db.Exec(query, pq.Array(recordIDs))
		if err != nil {
			return fmt.Errorf("delete query failed: %w", err)
		}
		rowsAffected, _ := result.RowsAffected()
		log.Printf("✅ After-processing: Deleted %d records from %s", rowsAffected, config.TableName)
		return nil

	case "archive":
		// Move to archive table
		if config.ArchiveTable == "" {
			return fmt.Errorf("archive_table required for archive mode")
		}
		if config.TableName == "" {
			return fmt.Errorf("table_name required for archive mode")
		}

		// Insert into archive
		query := fmt.Sprintf("INSERT INTO %s SELECT * FROM %s WHERE id = ANY($1)",
			config.ArchiveTable, config.TableName)
		_, err := db.Exec(query, pq.Array(recordIDs))
		if err != nil {
			return fmt.Errorf("archive insert failed: %w", err)
		}

		// Delete from source
		query = fmt.Sprintf("DELETE FROM %s WHERE id = ANY($1)", config.TableName)
		result, err := db.Exec(query, pq.Array(recordIDs))
		if err != nil {
			return fmt.Errorf("archive delete failed: %w", err)
		}
		rowsAffected, _ := result.RowsAffected()
		log.Printf("✅ After-processing: Archived %d records from %s to %s",
			rowsAffected, config.TableName, config.ArchiveTable)
		return nil

	case "nothing":
		// Do nothing
		return nil

	default:
		return fmt.Errorf("unknown after_processing mode: %s", config.AfterProcessing)
	}
}

// ConvertRowToMessageWithRecord converts database row to InboundMessage and returns the raw record map
// This allows access to the record ID for after-processing
func ConvertRowToMessageWithRecord(rows *sql.Rows, tableName string) (*models.InboundMessage, map[string]interface{}, error) {
	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		return nil, nil, err
	}

	// Create slice for values
	values := make([]interface{}, len(columns))
	valuePtrs := make([]interface{}, len(columns))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	// Scan row
	if err := rows.Scan(valuePtrs...); err != nil {
		return nil, nil, err
	}

	// Build map
	record := make(map[string]interface{})
	for i, col := range columns {
		record[col] = values[i]
	}

	// Common column names for raw message content (in priority order)
	messageColumns := []string{
		"raw_message", "message", "content", "message_content",
		"data", "payload", "hl7_message", "hl7", "raw_data",
		"message_body", "body",
	}

	// Common column names for message type
	typeColumns := []string{
		"message_type", "msg_type", "type", "format", "content_type",
	}

	// Common column names for message ID
	idColumns := []string{
		"message_id", "msg_id", "id", "correlation_id", "uuid",
	}

	// Extract raw message content
	var content string
	var contentType string = "application/json" // Default
	var messageType string = ""
	var recordMessageID string = ""
	var foundRawContent bool = false

	// Find raw message content
	for _, col := range messageColumns {
		if val, ok := record[col]; ok && val != nil {
			// Convert to string
			switch v := val.(type) {
			case string:
				content = v
				foundRawContent = true
			case []byte:
				content = string(v)
				foundRawContent = true
			}
			if foundRawContent {
				break
			}
		}
	}

	// Find message type
	for _, col := range typeColumns {
		if val, ok := record[col]; ok && val != nil {
			switch v := val.(type) {
			case string:
				messageType = v
			case []byte:
				messageType = string(v)
			}
			if messageType != "" {
				break
			}
		}
	}

	// Find message ID
	for _, col := range idColumns {
		if val, ok := record[col]; ok && val != nil {
			switch v := val.(type) {
			case string:
				recordMessageID = v
			case []byte:
				recordMessageID = string(v)
			case int:
				recordMessageID = fmt.Sprintf("%d", v)
			case int64:
				recordMessageID = fmt.Sprintf("%d", v)
			case float64:
				recordMessageID = fmt.Sprintf("%.0f", v)
			}
			if recordMessageID != "" {
				break
			}
		}
	}

	// If no raw content column found, fall back to full JSON
	if !foundRawContent {
		contentBytes, err := json.Marshal(record)
		if err != nil {
			return nil, nil, err
		}
		content = string(contentBytes)
	} else {
		// Detect content type based on content
		if len(content) > 0 {
			// Check for HL7 v2 (starts with MSH|)
			if len(content) >= 4 && content[:4] == "MSH|" {
				contentType = "application/hl7-v2"
				if messageType == "" {
					messageType = "hl7v2"
				}
			} else if len(content) > 0 && content[0] == '{' {
				contentType = "application/json"
			} else if len(content) > 0 && content[0] == '<' {
				contentType = "application/xml"
			} else {
				contentType = "text/plain"
			}
		}
	}

	// Create message ID (prefer from record, fallback to generated)
	messageID := fmt.Sprintf("db_%s_%d", tableName, time.Now().UnixNano())
	if recordMessageID != "" {
		messageID = fmt.Sprintf("db_%s_%s", tableName, recordMessageID)
	}

	return &models.InboundMessage{
		MessageID:      messageID,
		CorrelationID:  "",
		Content:        content,
		ContentType:    contentType,
		SourceType:     "database",
		SourceEndpoint: tableName,
		MessageType:    messageType,
		MessageSize:    len(content),
		ReceivedAt:     time.Now(),
		Headers: map[string]string{
			"Table-Name":    tableName,
			"Record-Count":  "1",
			"Database-Type": "relational",
		},
	}, record, nil
}

// ConvertRowToMessage converts database row to InboundMessage
// Intelligently extracts raw message content from common column names
func ConvertRowToMessage(rows *sql.Rows, tableName string) (*models.InboundMessage, error) {
	// Get column names
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	// Create slice for values
	values := make([]interface{}, len(columns))
	valuePtrs := make([]interface{}, len(columns))
	for i := range values {
		valuePtrs[i] = &values[i]
	}

	// Scan row
	if err := rows.Scan(valuePtrs...); err != nil {
		return nil, err
	}

	// Build map
	record := make(map[string]interface{})
	for i, col := range columns {
		record[col] = values[i]
	}

	// Common column names for raw message content (in priority order)
	messageColumns := []string{
		"raw_message", "message", "content", "message_content",
		"data", "payload", "hl7_message", "hl7", "raw_data",
		"message_body", "body",
	}

	// Common column names for message type
	typeColumns := []string{
		"message_type", "msg_type", "type", "format", "content_type",
	}

	// Common column names for message ID
	idColumns := []string{
		"message_id", "msg_id", "id", "correlation_id", "uuid",
	}

	// Extract raw message content
	var content string
	var contentType string = "application/json" // Default
	var messageType string = ""
	var recordMessageID string = ""
	var foundRawContent bool = false

	// Find raw message content
	for _, col := range messageColumns {
		if val, ok := record[col]; ok && val != nil {
			// Convert to string
			switch v := val.(type) {
			case string:
				content = v
				foundRawContent = true
			case []byte:
				content = string(v)
				foundRawContent = true
			}
			if foundRawContent {
				log.Printf("🔍 ConvertRowToMessage: Found raw content in column '%s' (length: %d)", col, len(content))
				break
			}
		}
	}

	// Find message type
	for _, col := range typeColumns {
		if val, ok := record[col]; ok && val != nil {
			switch v := val.(type) {
			case string:
				messageType = v
			case []byte:
				messageType = string(v)
			}
			if messageType != "" {
				log.Printf("🔍 ConvertRowToMessage: Found message type '%s' in column '%s'", messageType, col)
				break
			}
		}
	}

	// Find message ID
	for _, col := range idColumns {
		if val, ok := record[col]; ok && val != nil {
			switch v := val.(type) {
			case string:
				recordMessageID = v
			case []byte:
				recordMessageID = string(v)
			case int:
				recordMessageID = fmt.Sprintf("%d", v)
			case int64:
				recordMessageID = fmt.Sprintf("%d", v)
			case float64:
				recordMessageID = fmt.Sprintf("%.0f", v)
			}
			if recordMessageID != "" {
				break
			}
		}
	}

	// If no raw content column found, fall back to full JSON
	if !foundRawContent {
		contentBytes, err := json.Marshal(record)
		if err != nil {
			return nil, err
		}
		content = string(contentBytes)
		log.Printf("⚠️  ConvertRowToMessage: No raw content column found, using full JSON row")
	} else {
		// Detect content type based on content
		if len(content) > 0 {
			// Check for HL7 v2 (starts with MSH|)
			if len(content) >= 4 && content[:4] == "MSH|" {
				contentType = "application/hl7-v2"
				if messageType == "" {
					messageType = "hl7v2"
				}
			} else if len(content) > 0 && content[0] == '{' {
				contentType = "application/json"
			} else if len(content) > 0 && content[0] == '<' {
				contentType = "application/xml"
			} else {
				contentType = "text/plain"
			}
		}
	}

	// Create message ID (prefer from record, fallback to generated)
	messageID := fmt.Sprintf("db_%s_%d", tableName, time.Now().UnixNano())
	if recordMessageID != "" {
		messageID = fmt.Sprintf("db_%s_%s", tableName, recordMessageID)
	}

	return &models.InboundMessage{
		MessageID:      messageID,
		CorrelationID:  "",
		Content:        content,
		ContentType:    contentType,
		SourceType:     "database",
		SourceEndpoint: tableName,
		MessageType:    messageType,
		MessageSize:    len(content),
		ReceivedAt:     time.Now(),
		Headers: map[string]string{
			"Table-Name":    tableName,
			"Record-Count":  "1",
			"Database-Type": "relational",
		},
	}, nil
}

// LogDatabaseStats logs connection pool statistics
func LogDatabaseStats(db *sql.DB, dbType string) {
	stats := db.Stats()
	log.Printf("📊 %s Connection Pool Stats: Open=%d, InUse=%d, Idle=%d",
		dbType, stats.OpenConnections, stats.InUse, stats.Idle)
}
