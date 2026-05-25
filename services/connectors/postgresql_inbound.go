// services/connectors/postgresql_inbound.go
// PostgreSQL Inbound Connector — polls a PostgreSQL table for new rows.
//
// Enterprise features (v1.1):
//   - SharedAfterProcess via PostgreSQLDialect ($N placeholders, "quoted" identifiers)
//   - Metrics: IncrementMessagesReceived / RecordError / ClearError on every poll
//   - Watermark persistence: SetMetadata("last_cursor", ...) after each watermark advance
//   - Reconnect with exponential backoff on connection errors
//   - Polls immediately on Start (consistent with MySQL/SQL Server/Oracle behaviour)
//
// Configuration:
//
//	host               string  PostgreSQL host (default: localhost)
//	port               int     PostgreSQL port (default: 5432)
//	database           string  Database name (required)
//	username           string  PostgreSQL user (required)
//	password           string  PostgreSQL password
//	table_name         string  Table to poll (required if query not set)
//	query              string  Custom SQL — overrides table_name/incremental logic
//	incremental_column string  Column for watermark polling (e.g. id, updated_at)
//	incremental_type   string  "integer" | "bigint" | "timestamp" | "datetime"
//	polling_interval   int     Seconds between polls (default: 60)
//	max_records        int     LIMIT per query (default: 100)
//	order_by           string  ORDER BY clause (appended verbatim)
//	after_processing   string  "nothing" | "delete" | "update_flag"
//	processed_flag_col string  Column to update for update_flag mode
//	processed_flag_val string  Value to set for update_flag mode
//	ssl_mode           string  "disable" | "require" | "verify-ca" | "verify-full"
//	ssl_cert_path      string  Path to root certificate file
//	max_open_conns     int     Connection pool max open (default: 10)
//	max_idle_conns     int     Connection pool max idle (default: 5)

package connectors

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"ezhealthkonnect/models"

	_ "github.com/lib/pq" // PostgreSQL driver
)

const (
	postgresMaxReconnectDelay = 5 * time.Minute
)

// PostgreSQLInboundConnector polls a PostgreSQL table for new rows.
type PostgreSQLInboundConnector struct {
	*BaseInboundConnector
	config DatabaseInboundConfig
	db     *sql.DB
}

// NewPostgreSQLInboundConnector creates a production PostgreSQL inbound connector.
func NewPostgreSQLInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "postgresql_inbound",
		DisplayName:        "PostgreSQL Database Reader",
		Version:            "1.1.0",
		Category:           "inbound",
		Mode:               "pull",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":             true,
			"supports_ssl":              true,
			"supports_incremental":      true,
			"supports_after_processing": true,
			"supports_reconnect":        true,
		},
	}
	return &PostgreSQLInboundConnector{
		BaseInboundConnector: NewBaseInboundConnector(metadata),
	}
}

// Initialize sets up the PostgreSQL connector with configuration.
func (p *PostgreSQLInboundConnector) Initialize(config []byte) error {
	log.Printf("🐘 PostgreSQL Inbound: Initializing")

	parsedConfig, err := ParseDatabaseInboundConfig(config)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}
	p.config = *parsedConfig

	db, err := sql.Open("postgres", p.buildConnectionString())
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	ConfigureConnectionPool(db, p.config.MaxOpenConns, p.config.MaxIdleConns)

	if err := db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("failed to ping PostgreSQL at %s:%d: %w", p.config.Host, p.config.Port, err)
	}

	p.db = db
	p.BaseInboundConnector.BaseConnector.initialized = true
	p.BaseInboundConnector.SetState(StateReady)

	log.Printf("✅ PostgreSQL Inbound: Initialized (host=%s, database=%s, table=%s)",
		p.config.Host, p.config.Database, p.config.TableName)
	return nil
}

// buildConnectionString creates a PostgreSQL libpq-style connection string.
func (p *PostgreSQLInboundConnector) buildConnectionString() string {
	port := p.config.Port
	if port == 0 {
		port = 5432
	}
	sslMode := p.config.SSLMode
	if sslMode == "" {
		sslMode = "disable"
	}
	connStr := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		p.config.Host, port, p.config.Username, p.config.Password, p.config.Database, sslMode)
	if p.config.SSLCertPath != "" && sslMode != "disable" {
		connStr += fmt.Sprintf(" sslrootcert=%s", p.config.SSLCertPath)
	}
	return connStr
}

// reconnect closes the existing connection and re-opens a fresh one.
func (p *PostgreSQLInboundConnector) reconnect() error {
	if p.db != nil {
		_ = p.db.Close()
	}
	db, err := sql.Open("postgres", p.buildConnectionString())
	if err != nil {
		return err
	}
	ConfigureConnectionPool(db, p.config.MaxOpenConns, p.config.MaxIdleConns)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return err
	}
	p.db = db
	return nil
}

// buildQuery constructs the SELECT query with optional incremental filter.
func (p *PostgreSQLInboundConnector) buildQuery(lastValue interface{}) string {
	if p.config.Query != "" {
		return p.config.Query
	}

	query := fmt.Sprintf("SELECT * FROM %s", p.config.TableName)

	if p.config.IncrementalColumn != "" && lastValue != nil {
		switch p.config.IncrementalType {
		case "timestamp", "datetime":
			query += fmt.Sprintf(" WHERE %s > '%v'", p.config.IncrementalColumn, lastValue)
		default: // integer, bigint, uuid
			query += fmt.Sprintf(" WHERE %s > '%v'", p.config.IncrementalColumn, lastValue)
		}
	}

	if p.config.OrderBy != "" {
		query += fmt.Sprintf(" ORDER BY %s", p.config.OrderBy)
	} else if p.config.IncrementalColumn != "" {
		query += fmt.Sprintf(" ORDER BY %s", p.config.IncrementalColumn)
	}

	if p.config.MaxRecords > 0 {
		query += fmt.Sprintf(" LIMIT %d", p.config.MaxRecords)
	}

	return query
}

// TestConnection pings the database with a context deadline.
func (p *PostgreSQLInboundConnector) TestConnection(ctx context.Context) error {
	if p.db == nil {
		return fmt.Errorf("not initialized")
	}
	return p.db.PingContext(ctx)
}

// Validate checks required fields.
func (p *PostgreSQLInboundConnector) Validate() error {
	if !p.BaseInboundConnector.BaseConnector.initialized {
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

// SupportsCron returns true — PostgreSQL poll mode supports cron scheduling.
func (p *PostgreSQLInboundConnector) SupportsCron() bool { return true }

// Start launches the polling goroutine.
func (p *PostgreSQLInboundConnector) Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	if !p.BaseInboundConnector.BaseConnector.initialized {
		return fmt.Errorf("connector not initialized")
	}
	p.BaseInboundConnector.SetState(StateRunning)
	log.Printf("🐘 PostgreSQL Inbound: Starting (interval=%ds)", p.config.PollingInterval)
	go p.pollLoop(ctx, messageChan)
	return nil
}

// pollLoop polls immediately on start, then on each ticker interval, with
// exponential backoff reconnect on connection errors.
func (p *PostgreSQLInboundConnector) pollLoop(ctx context.Context, messageChan chan<- *models.InboundMessage) {
	defer p.BaseInboundConnector.SetState(StateReady)

	interval := p.config.PollingInterval
	if interval == 0 {
		interval = 60
	}
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	var lastValue interface{}
	reconnectAttempts := 0

	// Poll immediately on start
	if newLast, err := p.poll(messageChan, lastValue); err != nil {
		p.BaseInboundConnector.RecordError(err)
		log.Printf("❌ PostgreSQL Inbound: Initial poll error: %v", err)
	} else {
		if newLast != nil {
			lastValue = newLast
			p.BaseInboundConnector.SetMetadata("last_cursor", fmt.Sprintf("%v", lastValue))
		}
		p.BaseInboundConnector.ClearError()
	}

	for {
		select {
		case <-ctx.Done():
			log.Printf("🐘 PostgreSQL Inbound: Context cancelled")
			return
		case <-p.stopCh:
			log.Printf("🐘 PostgreSQL Inbound: Stop signal received")
			return
		case <-ticker.C:
			newLast, err := p.poll(messageChan, lastValue)
			if err != nil {
				p.BaseInboundConnector.RecordError(err)
				log.Printf("❌ PostgreSQL Inbound: Poll error: %v", err)

				if IsConnectionError(err) {
					reconnectAttempts++
					delay := time.Duration(reconnectAttempts*reconnectAttempts) * time.Second
					if delay > postgresMaxReconnectDelay {
						delay = postgresMaxReconnectDelay
					}
					log.Printf("🔄 PostgreSQL Inbound: Connection lost — reconnecting in %v (attempt %d)", delay, reconnectAttempts)
					p.BaseInboundConnector.SetState(StateError)
					select {
					case <-time.After(delay):
					case <-ctx.Done():
						return
					case <-p.stopCh:
						return
					}
					if err := p.reconnect(); err != nil {
						log.Printf("❌ PostgreSQL Inbound: Reconnect failed: %v", err)
					} else {
						log.Printf("✅ PostgreSQL Inbound: Reconnected successfully")
						p.BaseInboundConnector.SetState(StateRunning)
						reconnectAttempts = 0
					}
				}
			} else {
				if newLast != nil {
					lastValue = newLast
					p.BaseInboundConnector.SetMetadata("last_cursor", fmt.Sprintf("%v", lastValue)) // P3
				}
				p.BaseInboundConnector.ClearError()
				reconnectAttempts = 0
			}
		}
	}
}

// poll executes one query and sends matching rows as InboundMessages.
func (p *PostgreSQLInboundConnector) poll(messageChan chan<- *models.InboundMessage, lastValue interface{}) (interface{}, error) {
	// Take a local snapshot of db to avoid racing with Stop() that sets db=nil
	db := p.db
	if db == nil {
		return nil, fmt.Errorf("db is nil — connector may be stopping")
	}

	startTime := time.Now()
	query := p.buildQuery(lastValue)
	log.Printf("🔍 PostgreSQL Inbound: Executing: %s", query)

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	recordCount := 0
	var newLastValue interface{}

	for rows.Next() {
		msg, record, err := ConvertRowToMessageWithRecord(rows, p.config.TableName)
		if err != nil {
			log.Printf("❌ PostgreSQL Inbound: Row conversion failed: %v", err)
			continue
		}
		msg.SourceType = "postgresql"

		if p.config.IncrementalColumn != "" {
			if val, ok := record[p.config.IncrementalColumn]; ok {
				newLastValue = val
			}
		}

		messageChan <- msg
		p.BaseInboundConnector.IncrementMessagesReceived() // P2
		recordCount++

		// After-processing via shared dialect-aware helper (P1)
		if p.config.AfterProcessing != "nothing" && p.config.AfterProcessing != "" {
			if id, ok := record["id"]; ok && id != nil {
				if err := SharedAfterProcess(db, p.config, id, PostgreSQLDialect); err != nil {
					log.Printf("⚠️  PostgreSQL Inbound: After-process failed: %v", err)
				}
			}
		}
	}

	if err := rows.Err(); err != nil {
		return newLastValue, fmt.Errorf("rows error: %w", err)
	}

	duration := time.Since(startTime)
	if recordCount > 0 {
		log.Printf("✅ PostgreSQL Inbound: Polled %d records in %v", recordCount, duration)
	}
	return newLastValue, nil
}

// Stop halts the connector and closes the DB connection.
func (p *PostgreSQLInboundConnector) Stop() error {
	log.Printf("🐘 PostgreSQL Inbound: Stopping")
	if err := p.BaseInboundConnector.Stop(); err != nil {
		return err
	}
	if p.db != nil {
		if err := p.db.Close(); err != nil {
			return fmt.Errorf("failed to close PostgreSQL connection: %w", err)
		}
		p.db = nil
	}
	log.Printf("✅ PostgreSQL Inbound: Stopped")
	return nil
}

// Close is an alias for Stop.
func (p *PostgreSQLInboundConnector) Close() error { return p.Stop() }

// GetStatus returns connector status with PostgreSQL-specific metadata.
func (p *PostgreSQLInboundConnector) GetStatus() ConnectorStatus {
	status := p.BaseInboundConnector.GetStatus()
	if p.db != nil {
		if err := p.db.Ping(); err == nil {
			status.Connected = true
		} else {
			status.Connected = false
			status.LastError = err.Error()
		}
	}
	status.Metadata["database"] = p.config.Database
	status.Metadata["host"] = p.config.Host
	status.Metadata["table"] = p.config.TableName
	return status
}
