// services/connectors/postgresql_outbound.go
// PostgreSQL database outbound connector implementation
// Writes messages to PostgreSQL database using UPSERT (ON CONFLICT)

package connectors

import (
	"context"
	"database/sql"
	"encoding/json"
	"ezhealthkonnect/models"
	"fmt"
	"log"
	"strings"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
)

// PostgreSQLOutboundConnector handles writing to PostgreSQL databases
type PostgreSQLOutboundConnector struct {
	*BaseOutboundConnector
	config DatabaseOutboundConfig
	db     *sql.DB
}

// NewPostgreSQLOutboundConnector creates a new PostgreSQL outbound connector
func NewPostgreSQLOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "postgresql_outbound",
		DisplayName:        "PostgreSQL Database Writer",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch":  true,
			"supports_ssl":    true,
			"supports_upsert": true,
			"supports_pool":   true,
		},
	}

	connector := &PostgreSQLOutboundConnector{
		BaseOutboundConnector: NewBaseOutboundConnector(metadata, true),
	}

	return connector
}

// Initialize sets up the PostgreSQL connector with configuration
func (p *PostgreSQLOutboundConnector) Initialize(config []byte) error {
	log.Printf("🐘 PostgreSQL Outbound: Initializing connector")

	// Parse configuration
	parsedConfig, err := ParseDatabaseOutboundConfig(config)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}
	p.config = *parsedConfig

	// Build connection string
	connStr := p.buildConnectionString()

	// Open database connection
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	if p.config.MaxOpenConns > 0 {
		db.SetMaxOpenConns(p.config.MaxOpenConns)
	}
	if p.config.MaxIdleConns > 0 {
		db.SetMaxIdleConns(p.config.MaxIdleConns)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("failed to ping database: %w", err)
	}

	p.db = db

	// Mark as initialized
	p.BaseOutboundConnector.BaseConnector.initialized = true

	log.Printf("✅ PostgreSQL Outbound: Initialized successfully (host=%s, database=%s, table=%s)",
		p.config.Host, p.config.Database, p.config.TableName)

	return nil
}

// buildConnectionString creates PostgreSQL connection string
func (p *PostgreSQLOutboundConnector) buildConnectionString() string {
	// Default port
	port := p.config.Port
	if port == 0 {
		port = 5432
	}

	// Default SSL mode
	sslMode := p.config.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}

	// Build connection string
	connStr := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		p.config.Host,
		port,
		p.config.Username,
		p.config.Password,
		p.config.Database,
		sslMode,
	)

	// Add SSL certificate path if provided
	if p.config.SSLCertPath != "" && sslMode != "disable" {
		connStr += fmt.Sprintf(" sslrootcert=%s", p.config.SSLCertPath)
	}

	return connStr
}

// Send sends a single message to PostgreSQL
func (p *PostgreSQLOutboundConnector) Send(ctx context.Context, message *models.OutboundMessage) (*DeliveryResult, error) {
	if !p.initialized {
		return nil, fmt.Errorf("connector not initialized")
	}

	startTime := time.Now()

	// Parse message content as JSON map
	var dataMap map[string]interface{}
	if err := json.Unmarshal([]byte(message.Content), &dataMap); err != nil {
		return &DeliveryResult{
			Success:      false,
			MessageID:    message.MessageID,
			Timestamp:    time.Now(),
			ErrorMessage: fmt.Sprintf("failed to parse message content: %v", err),
			DurationMs:   int64(time.Since(startTime).Milliseconds()),
		}, err
	}

	// Build and execute INSERT/UPSERT query
	query, values := p.buildUpsertQuery(dataMap)

	log.Printf("🐘 PostgreSQL Outbound: Executing query: %s", query)

	result, err := p.db.Exec(query, values...)
	if err != nil {
		return &DeliveryResult{
			Success:      false,
			MessageID:    message.MessageID,
			Timestamp:    time.Now(),
			ErrorMessage: fmt.Sprintf("query execution failed: %v", err),
			DurationMs:   int64(time.Since(startTime).Milliseconds()),
		}, err
	}

	rowsAffected, _ := result.RowsAffected()

	return &DeliveryResult{
		Success:        true,
		MessageID:      message.MessageID,
		Timestamp:      time.Now(),
		Acknowledgment: fmt.Sprintf("Rows affected: %d", rowsAffected),
		DurationMs:     int64(time.Since(startTime).Milliseconds()),
	}, nil
}

// SendBatch sends multiple messages in a batch transaction
func (p *PostgreSQLOutboundConnector) SendBatch(ctx context.Context, messages []*models.OutboundMessage) ([]*DeliveryResult, error) {
	if !p.initialized {
		return nil, fmt.Errorf("connector not initialized")
	}

	startTime := time.Now()
	results := make([]*DeliveryResult, 0, len(messages))

	// Begin transaction
	tx, err := p.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	// Track success/failure
	successCount := 0
	failureCount := 0

	// Process each message
	for _, message := range messages {
		messageStart := time.Now()

		// Parse message content
		var dataMap map[string]interface{}
		if err := json.Unmarshal([]byte(message.Content), &dataMap); err != nil {
			results = append(results, &DeliveryResult{
				Success:      false,
				MessageID:    message.MessageID,
				Timestamp:    time.Now(),
				ErrorMessage: fmt.Sprintf("failed to parse message: %v", err),
				DurationMs:   int64(time.Since(messageStart).Milliseconds()),
			})
			failureCount++
			continue
		}

		// Build and execute query
		query, values := p.buildUpsertQuery(dataMap)
		result, err := tx.Exec(query, values...)

		if err != nil {
			results = append(results, &DeliveryResult{
				Success:      false,
				MessageID:    message.MessageID,
				Timestamp:    time.Now(),
				ErrorMessage: fmt.Sprintf("query failed: %v", err),
				DurationMs:   int64(time.Since(messageStart).Milliseconds()),
			})
			failureCount++
			continue
		}

		rowsAffected, _ := result.RowsAffected()
		results = append(results, &DeliveryResult{
			Success:        true,
			MessageID:      message.MessageID,
			Timestamp:      time.Now(),
			Acknowledgment: fmt.Sprintf("Rows affected: %d", rowsAffected),
			DurationMs:     int64(time.Since(messageStart).Milliseconds()),
		})
		successCount++
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		// Rollback on commit failure
		tx.Rollback()
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	duration := time.Since(startTime)
	log.Printf("✅ PostgreSQL Outbound: Batch sent %d messages (success: %d, failed: %d) in %v",
		len(messages), successCount, failureCount, duration)

	return results, nil
}

// buildUpsertQuery constructs PostgreSQL INSERT ... ON CONFLICT query
func (p *PostgreSQLOutboundConnector) buildUpsertQuery(dataMap map[string]interface{}) (string, []interface{}) {
	// Extract column names and values
	columns := make([]string, 0, len(dataMap))
	placeholders := make([]string, 0, len(dataMap))
	values := make([]interface{}, 0, len(dataMap))

	i := 1
	for key, value := range dataMap {
		columns = append(columns, key)
		placeholders = append(placeholders, fmt.Sprintf("$%d", i))
		values = append(values, value)
		i++
	}

	// Build base INSERT query
	query := fmt.Sprintf(
		"INSERT INTO %s (%s) VALUES (%s)",
		p.config.TableName,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
	)

	// Add ON CONFLICT clause if write mode is upsert and unique key is specified
	if (p.config.WriteMode == "upsert" || p.config.WriteMode == "update") && p.config.UniqueKey != "" {
		// Build UPDATE clause for conflict resolution
		updateClauses := make([]string, 0, len(columns))
		for _, col := range columns {
			if col != p.config.UniqueKey {
				updateClauses = append(updateClauses, fmt.Sprintf("%s = EXCLUDED.%s", col, col))
			}
		}

		query += fmt.Sprintf(
			" ON CONFLICT (%s) DO UPDATE SET %s",
			p.config.UniqueKey,
			strings.Join(updateClauses, ", "),
		)
	}

	return query, values
}

// SupportsBatch indicates this connector supports batch operations
func (p *PostgreSQLOutboundConnector) SupportsBatch() bool {
	return true
}

// Close cleans up resources
func (p *PostgreSQLOutboundConnector) Close() error {
	log.Printf("🐘 PostgreSQL Outbound: Closing connector")

	if p.db != nil {
		if err := p.db.Close(); err != nil {
			return fmt.Errorf("failed to close database: %w", err)
		}
	}

	log.Printf("✅ PostgreSQL Outbound: Closed successfully")
	return nil
}

// Validate checks if the configuration is valid
func (p *PostgreSQLOutboundConnector) Validate() error {
	if !p.initialized {
		return fmt.Errorf("connector not initialized")
	}

	if p.config.TableName == "" {
		return fmt.Errorf("table_name is required")
	}

	if p.config.Host == "" {
		return fmt.Errorf("host is required")
	}

	if p.config.Database == "" {
		return fmt.Errorf("database is required")
	}

	if p.config.Username == "" {
		return fmt.Errorf("username is required")
	}

	if (p.config.WriteMode == "upsert" || p.config.WriteMode == "update") && p.config.UniqueKey == "" {
		return fmt.Errorf("unique_key is required when write_mode is upsert or update")
	}

	return nil
}

// GetStatus returns connector status
func (p *PostgreSQLOutboundConnector) GetStatus() ConnectorStatus {
	// Get base status
	status := p.BaseOutboundConnector.GetStatus()

	// Add database-specific metadata
	if p.initialized {
		if p.db != nil {
			if err := p.db.Ping(); err == nil {
				status.Connected = true
				p.SetMetadata("database", p.config.Database)
				p.SetMetadata("host", p.config.Host)
				p.SetMetadata("table", p.config.TableName)
				if p.config.WriteMode != "" {
					p.SetMetadata("write_mode", p.config.WriteMode)
				}
			} else {
				status.Connected = false
				status.LastError = fmt.Sprintf("Connection error: %v", err)
			}
		}
	}

	return status
}
