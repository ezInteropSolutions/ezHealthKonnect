// services/connectors/postgresql_inbound.go
// PostgreSQL database inbound connector implementation
// Polls PostgreSQL database for new records and converts them to messages

package connectors

import (
	"context"
	"database/sql"
	"ezhealthkonnect/models"
	"fmt"
	"log"
	"time"

	_ "github.com/lib/pq" // PostgreSQL driver
)

// PostgreSQLInboundConnector handles reading from PostgreSQL databases
type PostgreSQLInboundConnector struct {
	*BaseInboundConnector
	config DatabaseInboundConfig
	db     *sql.DB
}

// NewPostgreSQLInboundConnector creates a new PostgreSQL inbound connector
func NewPostgreSQLInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "postgresql_inbound",
		DisplayName:        "PostgreSQL Database Reader",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "pull",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":             true,
			"supports_ssl":              true,
			"supports_incremental":      true,
			"supports_after_processing": true,
		},
	}

	connector := &PostgreSQLInboundConnector{
		BaseInboundConnector: NewBaseInboundConnector(metadata),
	}

	return connector
}

// Initialize sets up the PostgreSQL connector with configuration
func (p *PostgreSQLInboundConnector) Initialize(config []byte) error {
	log.Printf("🐘 PostgreSQL Inbound: Initializing connector")

	// Parse configuration
	parsedConfig, err := ParseDatabaseInboundConfig(config)
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
	ConfigureConnectionPool(db, p.config.MaxOpenConns, p.config.MaxIdleConns)

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("failed to ping database: %w", err)
	}

	p.db = db

	// Mark as initialized
	p.BaseInboundConnector.BaseConnector.initialized = true

	log.Printf("✅ PostgreSQL Inbound: Initialized successfully (host=%s, database=%s, table=%s)",
		p.config.Host, p.config.Database, p.config.TableName)

	return nil
}

// buildConnectionString creates PostgreSQL connection string
func (p *PostgreSQLInboundConnector) buildConnectionString() string {
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

// buildQuery constructs the SQL query for polling
func (p *PostgreSQLInboundConnector) buildQuery(lastValue interface{}) string {
	var query string

	// Use custom query if provided
	if p.config.Query != "" {
		query = p.config.Query
	} else if p.config.IncrementalColumn != "" {
		// Build incremental query
		query = fmt.Sprintf("SELECT * FROM %s", p.config.TableName)

		// Add WHERE clause for incremental polling
		if lastValue != nil {
			switch p.config.IncrementalType {
			case "timestamp", "datetime":
				query += fmt.Sprintf(" WHERE %s > '%v'", p.config.IncrementalColumn, lastValue)
			case "integer", "bigint":
				query += fmt.Sprintf(" WHERE %s > %v", p.config.IncrementalColumn, lastValue)
			default:
				query += fmt.Sprintf(" WHERE %s > '%v'", p.config.IncrementalColumn, lastValue)
			}
		}

		// Add ORDER BY
		if p.config.OrderBy != "" {
			query += fmt.Sprintf(" ORDER BY %s", p.config.OrderBy)
		} else {
			query += fmt.Sprintf(" ORDER BY %s", p.config.IncrementalColumn)
		}
	} else {
		// Simple full table scan
		query = fmt.Sprintf("SELECT * FROM %s", p.config.TableName)
		if p.config.OrderBy != "" {
			query += fmt.Sprintf(" ORDER BY %s", p.config.OrderBy)
		}
	}

	// Add LIMIT
	if p.config.MaxRecords > 0 {
		query += fmt.Sprintf(" LIMIT %d", p.config.MaxRecords)
	}

	return query
}

// Start begins polling the PostgreSQL database
func (p *PostgreSQLInboundConnector) Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	if !p.initialized {
		return fmt.Errorf("connector not initialized")
	}

	log.Printf("🐘 PostgreSQL Inbound: Starting connector (polling interval: %ds)", p.config.PollingInterval)

	// Start polling loop
	go p.pollLoop(ctx, messageChan)

	return nil
}

// pollLoop continuously polls the database for new records
func (p *PostgreSQLInboundConnector) pollLoop(ctx context.Context, messageChan chan<- *models.InboundMessage) {
	// Default polling interval
	interval := p.config.PollingInterval
	if interval == 0 {
		interval = 60 // 60 seconds default
	}

	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	var lastValue interface{}

	for {
		select {
		case <-ctx.Done():
			log.Printf("🐘 PostgreSQL Inbound: Context cancelled, stopping polling loop")
			return
		case <-p.stopCh:
			log.Printf("🐘 PostgreSQL Inbound: Stopping polling loop")
			return
		case <-ticker.C:
			// Poll for new records
			newLastValue, err := p.poll(messageChan, lastValue)
			if err != nil {
				log.Printf("❌ PostgreSQL Inbound: Poll error: %v", err)
			} else if newLastValue != nil {
				lastValue = newLastValue
			}
		}
	}
}

// poll executes a single poll operation
func (p *PostgreSQLInboundConnector) poll(messageChan chan<- *models.InboundMessage, lastValue interface{}) (interface{}, error) {
	startTime := time.Now()

	// Build query
	query := p.buildQuery(lastValue)
	log.Printf("🔍 PostgreSQL Inbound: Executing query: %s", query)

	// Execute query
	rows, err := p.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	recordCount := 0
	var newLastValue interface{}

	// Process each row
	for rows.Next() {
		// Convert row to message and get the raw record for after-processing
		message, record, err := ConvertRowToMessageWithRecord(rows, p.config.TableName)
		if err != nil {
			log.Printf("❌ PostgreSQL Inbound: Failed to convert row: %v", err)
			continue
		}

		// Track last incremental value from record
		if p.config.IncrementalColumn != "" {
			if val, ok := record[p.config.IncrementalColumn]; ok {
				newLastValue = val
			}
		}

		// Send to channel
		messageChan <- message
		recordCount++

		// Handle after-processing (requires record IDs)
		if p.config.AfterProcessing != "nothing" && p.config.AfterProcessing != "" {
			// Extract record ID for after-processing
			var recordIDs []interface{}
			if id, ok := record["id"]; ok && id != nil {
				recordIDs = append(recordIDs, id)
			}
			if len(recordIDs) > 0 {
				if err := HandleAfterProcessing(p.db, p.config, recordIDs); err != nil {
					log.Printf("❌ PostgreSQL Inbound: After-processing failed: %v", err)
				}
			} else {
				log.Printf("⚠️ PostgreSQL Inbound: No 'id' column found for after-processing")
			}
		}
	}

	if err := rows.Err(); err != nil {
		return newLastValue, fmt.Errorf("rows iteration error: %w", err)
	}

	duration := time.Since(startTime)
	if recordCount > 0 {
		log.Printf("✅ PostgreSQL Inbound: Polled %d records in %v", recordCount, duration)
	}

	return newLastValue, nil
}

// Stop halts the connector
func (p *PostgreSQLInboundConnector) Stop() error {
	log.Printf("🐘 PostgreSQL Inbound: Stopping connector")

	// Stop polling loop via base connector
	if err := p.BaseInboundConnector.Stop(); err != nil {
		return err
	}

	// Close database connection
	if p.db != nil {
		if err := p.db.Close(); err != nil {
			return fmt.Errorf("failed to close database: %w", err)
		}
	}

	log.Printf("✅ PostgreSQL Inbound: Stopped successfully")
	return nil
}

// Close cleans up resources
func (p *PostgreSQLInboundConnector) Close() error {
	return p.Stop()
}

// Validate checks if the configuration is valid
func (p *PostgreSQLInboundConnector) Validate() error {
	if !p.initialized {
		return fmt.Errorf("connector not initialized")
	}

	if p.config.TableName == "" && p.config.Query == "" {
		return fmt.Errorf("either table_name or query must be specified")
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

	return nil
}

// GetStatus returns connector status
func (p *PostgreSQLInboundConnector) GetStatus() ConnectorStatus {
	// Get base status
	status := p.BaseInboundConnector.GetStatus()

	// Add database-specific metadata
	if p.initialized {
		if p.db != nil {
			if err := p.db.Ping(); err == nil {
				status.Connected = true
				p.SetMetadata("database", p.config.Database)
				p.SetMetadata("host", p.config.Host)
				p.SetMetadata("table", p.config.TableName)
			} else {
				status.Connected = false
				status.LastError = fmt.Sprintf("Connection error: %v", err)
			}
		}
	}

	return status
}

// SupportsCron indicates this connector supports cron-based scheduling
func (p *PostgreSQLInboundConnector) SupportsCron() bool {
	return true
}
