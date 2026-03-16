// services/connectors/mysql_inbound.go
// MySQL Inbound Connector — polls a MySQL/MariaDB table for new rows and converts them
// to InboundMessages for the pipeline.
//
// Enterprise features (v1.1):
//   - SharedAfterProcess via MySQLDialect (? placeholders, backtick quoting)
//   - Metrics: IncrementMessagesReceived / RecordError / ClearError on every poll
//   - Watermark persistence: SetMetadata("last_cursor", ...) after each watermark advance
//   - Reconnect with exponential backoff (1s, 4s, 9s … up to 5 min) on connection errors
//
// Configuration:
//
//	host               string  MySQL host (default: localhost)
//	port               int     MySQL port (default: 3306)
//	database           string  Database name (required)
//	username           string  MySQL user (required)
//	password           string  MySQL password
//	table_name         string  Table to poll (required if query not set)
//	query              string  Custom SQL — overrides table_name/incremental logic
//	incremental_column string  Column for incremental polling (e.g. id, updated_at)
//	incremental_type   string  "integer" | "timestamp" | "datetime"
//	polling_interval   int     Seconds between polls (default: 60)
//	max_records        int     LIMIT per query (default: 100)
//	order_by           string  ORDER BY clause
//	after_processing   string  "nothing" | "delete" | "update_flag"
//	processed_flag_col string  Column to set for update_flag
//	processed_flag_val string  Value to set for update_flag
//	ssl_mode           string  "disable" | "require"
//	max_open_conns     int     Connection pool max open (default: 10)
//	max_idle_conns     int     Connection pool max idle (default: 5)

package connectors

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"ezhealthkonnect/models"

	_ "github.com/go-sql-driver/mysql" // MySQL driver
)

const (
	mysqlMaxReconnectDelay = 5 * time.Minute
)

// MySQLInboundConnector polls a MySQL table for new rows.
type MySQLInboundConnector struct {
	*BaseInboundConnector
	config DatabaseInboundConfig
	db     *sql.DB
}

// NewMySQLInboundConnector creates a production MySQL inbound connector.
func NewMySQLInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "mysql_inbound",
		DisplayName:        "MySQL Database Reader",
		Version:            "1.1.0",
		Category:           "inbound",
		Mode:               "pull",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":             true,
			"supports_tls":              true,
			"supports_incremental":      true,
			"supports_after_processing": true,
			"supports_reconnect":        true,
		},
	}
	return &MySQLInboundConnector{
		BaseInboundConnector: NewBaseInboundConnector(metadata),
	}
}

// Initialize configures the connector and opens the MySQL connection.
func (m *MySQLInboundConnector) Initialize(config []byte) error {
	log.Printf("🐬 MySQL Inbound: Initializing")

	parsedConfig, err := ParseDatabaseInboundConfig(config)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}
	m.config = *parsedConfig

	db, err := sql.Open("mysql", m.buildDSN())
	if err != nil {
		return fmt.Errorf("failed to open MySQL connection: %w", err)
	}

	ConfigureConnectionPool(db, m.config.MaxOpenConns, m.config.MaxIdleConns)

	if err := db.Ping(); err != nil {
		_ = db.Close()
		return fmt.Errorf("failed to ping MySQL at %s:%d: %w", m.config.Host, m.config.Port, err)
	}

	m.db = db
	m.BaseInboundConnector.BaseConnector.initialized = true
	m.BaseInboundConnector.SetState(StateReady)

	log.Printf("✅ MySQL Inbound: Initialized (host=%s, database=%s)", m.config.Host, m.config.Database)
	return nil
}

// buildDSN creates a MySQL DSN from the parsed config.
func (m *MySQLInboundConnector) buildDSN() string {
	host := m.config.Host
	if host == "" {
		host = "localhost"
	}
	port := m.config.Port
	if port == 0 {
		port = 3306
	}

	params := []string{"parseTime=true", "charset=utf8mb4"}
	switch m.config.SSLMode {
	case "require", "verify-ca", "verify-full":
		params = append(params, "tls=true")
	default:
		params = append(params, "tls=false")
	}

	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?%s",
		m.config.Username, m.config.Password,
		host, port,
		m.config.Database,
		strings.Join(params, "&"),
	)
}

// reconnect closes the existing connection and re-opens a fresh one.
func (m *MySQLInboundConnector) reconnect() error {
	if m.db != nil {
		_ = m.db.Close()
	}
	db, err := sql.Open("mysql", m.buildDSN())
	if err != nil {
		return err
	}
	ConfigureConnectionPool(db, m.config.MaxOpenConns, m.config.MaxIdleConns)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return err
	}
	m.db = db
	return nil
}

// buildQuery constructs the SELECT query with optional incremental filter.
func (m *MySQLInboundConnector) buildQuery(lastValue interface{}) string {
	if m.config.Query != "" {
		return m.config.Query
	}

	query := fmt.Sprintf("SELECT * FROM `%s`", m.config.TableName)

	if m.config.IncrementalColumn != "" && lastValue != nil {
		switch m.config.IncrementalType {
		case "timestamp", "datetime":
			query += fmt.Sprintf(" WHERE `%s` > '%v'", m.config.IncrementalColumn, lastValue)
		default:
			query += fmt.Sprintf(" WHERE `%s` > %v", m.config.IncrementalColumn, lastValue)
		}
	}

	if m.config.OrderBy != "" {
		query += fmt.Sprintf(" ORDER BY %s", m.config.OrderBy)
	} else if m.config.IncrementalColumn != "" {
		query += fmt.Sprintf(" ORDER BY `%s`", m.config.IncrementalColumn)
	}

	if m.config.MaxRecords > 0 {
		query += fmt.Sprintf(" LIMIT %d", m.config.MaxRecords)
	}

	return query
}

// TestConnection pings the database.
func (m *MySQLInboundConnector) TestConnection(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("not initialized")
	}
	return m.db.PingContext(ctx)
}

// Validate checks required fields.
func (m *MySQLInboundConnector) Validate() error {
	if !m.BaseInboundConnector.BaseConnector.initialized {
		return fmt.Errorf("connector not initialized")
	}
	if m.config.TableName == "" && m.config.Query == "" {
		return fmt.Errorf("either table_name or query must be specified")
	}
	if m.config.Database == "" {
		return fmt.Errorf("database is required")
	}
	return nil
}

// SupportsCron returns true — MySQL poll mode supports cron scheduling.
func (m *MySQLInboundConnector) SupportsCron() bool { return true }

// Start launches the polling goroutine.
func (m *MySQLInboundConnector) Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	if !m.BaseInboundConnector.BaseConnector.initialized {
		return fmt.Errorf("connector not initialized")
	}
	m.BaseInboundConnector.SetState(StateRunning)
	log.Printf("🐬 MySQL Inbound: Starting (interval=%ds)", m.config.PollingInterval)
	go m.pollLoop(ctx, messageChan)
	return nil
}

// pollLoop runs polls on a ticker with exponential backoff reconnect on connection errors.
func (m *MySQLInboundConnector) pollLoop(ctx context.Context, messageChan chan<- *models.InboundMessage) {
	defer m.BaseInboundConnector.SetState(StateReady)

	interval := m.config.PollingInterval
	if interval == 0 {
		interval = 60
	}
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	var lastValue interface{}
	reconnectAttempts := 0

	// Poll immediately on start
	if newLast, err := m.poll(messageChan, lastValue); err != nil {
		m.BaseInboundConnector.RecordError(err)
		log.Printf("❌ MySQL Inbound: Initial poll error: %v", err)
	} else {
		if newLast != nil {
			lastValue = newLast
			m.BaseInboundConnector.SetMetadata("last_cursor", fmt.Sprintf("%v", lastValue))
		}
		m.BaseInboundConnector.ClearError()
		reconnectAttempts = 0
	}

	for {
		select {
		case <-ctx.Done():
			log.Printf("🐬 MySQL Inbound: Context cancelled")
			return
		case <-m.BaseInboundConnector.stopCh:
			log.Printf("🐬 MySQL Inbound: Stop signal received")
			return
		case <-ticker.C:
			newLast, err := m.poll(messageChan, lastValue)
			if err != nil {
				m.BaseInboundConnector.RecordError(err)
				log.Printf("❌ MySQL Inbound: Poll error: %v", err)

				if IsConnectionError(err) {
					reconnectAttempts++
					delay := time.Duration(reconnectAttempts*reconnectAttempts) * time.Second
					if delay > mysqlMaxReconnectDelay {
						delay = mysqlMaxReconnectDelay
					}
					log.Printf("🔄 MySQL Inbound: Connection lost — reconnecting in %v (attempt %d)", delay, reconnectAttempts)
					m.BaseInboundConnector.SetState(StateError)
					select {
					case <-time.After(delay):
					case <-ctx.Done():
						return
					case <-m.BaseInboundConnector.stopCh:
						return
					}
					if err := m.reconnect(); err != nil {
						log.Printf("❌ MySQL Inbound: Reconnect failed: %v", err)
					} else {
						log.Printf("✅ MySQL Inbound: Reconnected successfully")
						m.BaseInboundConnector.SetState(StateRunning)
						reconnectAttempts = 0
					}
				}
			} else {
				if newLast != nil {
					lastValue = newLast
					m.BaseInboundConnector.SetMetadata("last_cursor", fmt.Sprintf("%v", lastValue)) // P3
				}
				m.BaseInboundConnector.ClearError()
				reconnectAttempts = 0
			}
		}
	}
}

// poll executes one query and sends matching rows as messages.
func (m *MySQLInboundConnector) poll(messageChan chan<- *models.InboundMessage, lastValue interface{}) (interface{}, error) {
	// Take a local snapshot of db to avoid racing with Stop() that sets db=nil
	db := m.db
	if db == nil {
		return nil, fmt.Errorf("db is nil — connector may be stopping")
	}

	query := m.buildQuery(lastValue)
	log.Printf("🔍 MySQL Inbound: Executing: %s", query)

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	recordCount := 0
	var newLastValue interface{}

	for rows.Next() {
		msg, record, err := ConvertRowToMessageWithRecord(rows, m.config.TableName)
		if err != nil {
			log.Printf("❌ MySQL Inbound: Row conversion failed: %v", err)
			continue
		}
		msg.SourceType = "mysql"

		if m.config.IncrementalColumn != "" {
			if val, ok := record[m.config.IncrementalColumn]; ok {
				newLastValue = val
			}
		}

		messageChan <- msg
		m.BaseInboundConnector.IncrementMessagesReceived() // P2
		recordCount++

		// After-processing via shared dialect-aware helper (P1)
		if m.config.AfterProcessing != "nothing" && m.config.AfterProcessing != "" {
			if id, ok := record["id"]; ok && id != nil {
				if err := SharedAfterProcess(db, m.config, id, MySQLDialect); err != nil {
					log.Printf("⚠️  MySQL Inbound: After-process failed: %v", err)
				}
			}
		}
	}

	if err := rows.Err(); err != nil {
		return newLastValue, fmt.Errorf("rows error: %w", err)
	}

	if recordCount > 0 {
		log.Printf("✅ MySQL Inbound: Polled %d rows from %s", recordCount, m.config.TableName)
	}
	return newLastValue, nil
}

// Stop halts the connector and closes the DB connection.
func (m *MySQLInboundConnector) Stop() error {
	log.Printf("🐬 MySQL Inbound: Stopping")
	if err := m.BaseInboundConnector.Stop(); err != nil {
		return err
	}
	if m.db != nil {
		if err := m.db.Close(); err != nil {
			return fmt.Errorf("failed to close MySQL connection: %w", err)
		}
		m.db = nil
	}
	log.Printf("✅ MySQL Inbound: Stopped")
	return nil
}

// Close is an alias for Stop.
func (m *MySQLInboundConnector) Close() error { return m.Stop() }

// GetStatus returns connector status with MySQL-specific information.
func (m *MySQLInboundConnector) GetStatus() ConnectorStatus {
	status := m.BaseInboundConnector.GetStatus()
	if m.db != nil {
		if err := m.db.Ping(); err == nil {
			status.Connected = true
		} else {
			status.Connected = false
			status.LastError = err.Error()
		}
	}
	status.Metadata["database"] = m.config.Database
	status.Metadata["host"] = m.config.Host
	status.Metadata["table"] = m.config.TableName
	return status
}
