// services/connectors/sqlserver_inbound.go
// SQL Server Inbound Connector — polls a SQL Server table for new rows and delivers
// them as InboundMessages for the pipeline.
//
// Enterprise features (v1.1):
//   - SharedAfterProcess via SQLServerDialect (@pN placeholders, [bracket] quoting)
//   - Metrics: IncrementMessagesReceived / RecordError / ClearError on every poll
//   - Watermark persistence: SetMetadata("last_cursor", ...) after each watermark advance
//   - Reconnect with exponential backoff on connection errors
//
// Configuration:
//
//	host               string  SQL Server host (default: localhost)
//	port               int     SQL Server port (default: 1433)
//	database           string  Database name (required)
//	username           string  SQL Server login (required)
//	password           string  SQL Server password
//	table_name         string  Table to poll (required if query not set)
//	query              string  Custom SQL — overrides table_name/incremental logic
//	incremental_column string  Column for watermark polling (e.g. id, updated_at)
//	incremental_type   string  "integer" | "timestamp" | "datetime"
//	polling_interval   int     Seconds between polls (default: 60)
//	max_records        int     TOP N per query (default: 100)
//	order_by           string  ORDER BY clause (appended verbatim)
//	after_processing   string  "nothing" | "delete" | "update_flag"
//	processed_flag_col string  Column to update for update_flag mode
//	processed_flag_val string  Value to set for update_flag mode
//	ssl_mode           string  "disable" | "require" | "verify-full"
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

	_ "github.com/microsoft/go-mssqldb" // Registers "sqlserver" driver
)

const (
	sqlserverMaxReconnectDelay = 5 * time.Minute
)

// SQLServerInboundConnector polls a SQL Server table for new rows.
type SQLServerInboundConnector struct {
	*BaseInboundConnector
	config DatabaseInboundConfig
	db     *sql.DB
}

// NewSQLServerInboundConnector creates a production SQL Server inbound connector.
func NewSQLServerInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "sqlserver_inbound",
		DisplayName:        "SQL Server Database Reader",
		Version:            "1.1.0",
		Category:           "inbound",
		Mode:               "pull",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":             true,
			"supports_encryption":       true,
			"supports_incremental":      true,
			"supports_after_processing": true,
			"supports_windows_auth":     true,
			"supports_reconnect":        true,
		},
	}
	return &SQLServerInboundConnector{
		BaseInboundConnector: NewBaseInboundConnector(metadata),
	}
}

// Initialize parses config, builds the SQL Server DSN, opens the DB,
// pings to verify connectivity, and marks the connector ready.
func (s *SQLServerInboundConnector) Initialize(config []byte) error {
	log.Printf("🗄️  SQL Server Inbound: Initializing")

	parsedConfig, err := ParseDatabaseInboundConfig(config)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}
	s.config = *parsedConfig

	db, err := sql.Open("sqlserver", s.buildDSN())
	if err != nil {
		return fmt.Errorf("failed to open SQL Server connection: %w", err)
	}

	ConfigureConnectionPool(db, s.config.MaxOpenConns, s.config.MaxIdleConns)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("failed to ping SQL Server at %s:%d: %w", s.config.Host, s.config.Port, err)
	}

	s.db = db
	s.BaseInboundConnector.BaseConnector.initialized = true
	s.BaseInboundConnector.SetState(StateReady)

	log.Printf("✅ SQL Server Inbound: Initialized (host=%s, database=%s)", s.config.Host, s.config.Database)
	return nil
}

// buildDSN constructs the go-mssqldb URL connection string.
//
//	sqlserver://user:pass@host:port?database=name&encrypt=disable
func (s *SQLServerInboundConnector) buildDSN() string {
	host := s.config.Host
	if host == "" {
		host = "localhost"
	}
	port := s.config.Port
	if port == 0 {
		port = 1433
	}

	dsn := fmt.Sprintf("sqlserver://%s:%s@%s:%d?database=%s",
		s.config.Username, s.config.Password, host, port, s.config.Database)

	switch s.config.SSLMode {
	case "require":
		dsn += "&encrypt=true&TrustServerCertificate=true"
	case "verify-full":
		dsn += "&encrypt=true&TrustServerCertificate=false"
	default: // "disable" or ""
		dsn += "&encrypt=disable"
	}

	return dsn
}

// reconnect closes the existing connection and re-opens a fresh one.
func (s *SQLServerInboundConnector) reconnect() error {
	if s.db != nil {
		_ = s.db.Close()
	}
	db, err := sql.Open("sqlserver", s.buildDSN())
	if err != nil {
		return err
	}
	ConfigureConnectionPool(db, s.config.MaxOpenConns, s.config.MaxIdleConns)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return err
	}
	s.db = db
	return nil
}

// buildQuery constructs the SELECT statement.
// SQL Server uses TOP N (not LIMIT) and [bracket] quoting.
func (s *SQLServerInboundConnector) buildQuery(lastValue interface{}) string {
	if s.config.Query != "" {
		return s.config.Query
	}

	top := s.config.MaxRecords
	if top <= 0 {
		top = 100
	}

	query := fmt.Sprintf("SELECT TOP %d * FROM [%s]", top, s.config.TableName)

	if s.config.IncrementalColumn != "" && lastValue != nil {
		switch s.config.IncrementalType {
		case "timestamp", "datetime":
			query += fmt.Sprintf(" WHERE [%s] > '%v'", s.config.IncrementalColumn, lastValue)
		default:
			query += fmt.Sprintf(" WHERE [%s] > %v", s.config.IncrementalColumn, lastValue)
		}
	}

	if s.config.OrderBy != "" {
		query += fmt.Sprintf(" ORDER BY %s", s.config.OrderBy)
	} else if s.config.IncrementalColumn != "" {
		query += fmt.Sprintf(" ORDER BY [%s] ASC", s.config.IncrementalColumn)
	}

	return query
}

// TestConnection pings the SQL Server database with a context deadline.
func (s *SQLServerInboundConnector) TestConnection(ctx context.Context) error {
	if s.db == nil {
		return fmt.Errorf("not initialized")
	}
	return s.db.PingContext(ctx)
}

// Validate checks that the connector is initialized and has required fields.
func (s *SQLServerInboundConnector) Validate() error {
	if !s.BaseInboundConnector.BaseConnector.initialized {
		return fmt.Errorf("connector not initialized")
	}
	if s.config.TableName == "" && s.config.Query == "" {
		return fmt.Errorf("either table_name or query must be specified")
	}
	if s.config.Database == "" {
		return fmt.Errorf("database is required")
	}
	return nil
}

// SupportsCron returns true — SQL Server poll mode supports cron scheduling.
func (s *SQLServerInboundConnector) SupportsCron() bool { return true }

// Start launches the poll goroutine.
func (s *SQLServerInboundConnector) Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	if !s.BaseInboundConnector.BaseConnector.initialized {
		return fmt.Errorf("connector not initialized")
	}
	s.BaseInboundConnector.SetState(StateRunning)
	log.Printf("🗄️  SQL Server Inbound: Starting (interval=%ds)", s.config.PollingInterval)
	go s.pollLoop(ctx, messageChan)
	return nil
}

// pollLoop polls immediately on start then on every ticker interval, with
// exponential backoff reconnect on connection errors.
func (s *SQLServerInboundConnector) pollLoop(ctx context.Context, messageChan chan<- *models.InboundMessage) {
	defer s.BaseInboundConnector.SetState(StateReady)

	interval := s.config.PollingInterval
	if interval == 0 {
		interval = 60
	}
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	var lastValue interface{}
	reconnectAttempts := 0

	// Poll immediately on start
	if newLast, err := s.poll(messageChan, lastValue); err != nil {
		s.BaseInboundConnector.RecordError(err)
		log.Printf("❌ SQL Server Inbound: Initial poll error: %v", err)
	} else {
		if newLast != nil {
			lastValue = newLast
			s.BaseInboundConnector.SetMetadata("last_cursor", fmt.Sprintf("%v", lastValue))
		}
		s.BaseInboundConnector.ClearError()
	}

	for {
		select {
		case <-ctx.Done():
			log.Printf("🗄️  SQL Server Inbound: Context cancelled")
			return
		case <-s.BaseInboundConnector.stopCh:
			log.Printf("🗄️  SQL Server Inbound: Stop signal received")
			return
		case <-ticker.C:
			newLast, err := s.poll(messageChan, lastValue)
			if err != nil {
				s.BaseInboundConnector.RecordError(err)
				log.Printf("❌ SQL Server Inbound: Poll error: %v", err)

				if IsConnectionError(err) {
					reconnectAttempts++
					delay := time.Duration(reconnectAttempts*reconnectAttempts) * time.Second
					if delay > sqlserverMaxReconnectDelay {
						delay = sqlserverMaxReconnectDelay
					}
					log.Printf("🔄 SQL Server Inbound: Connection lost — reconnecting in %v (attempt %d)", delay, reconnectAttempts)
					s.BaseInboundConnector.SetState(StateError)
					select {
					case <-time.After(delay):
					case <-ctx.Done():
						return
					case <-s.BaseInboundConnector.stopCh:
						return
					}
					if err := s.reconnect(); err != nil {
						log.Printf("❌ SQL Server Inbound: Reconnect failed: %v", err)
					} else {
						log.Printf("✅ SQL Server Inbound: Reconnected successfully")
						s.BaseInboundConnector.SetState(StateRunning)
						reconnectAttempts = 0
					}
				}
			} else {
				if newLast != nil {
					lastValue = newLast
					s.BaseInboundConnector.SetMetadata("last_cursor", fmt.Sprintf("%v", lastValue)) // P3
				}
				s.BaseInboundConnector.ClearError()
				reconnectAttempts = 0
			}
		}
	}
}

// poll executes one query and sends matching rows as InboundMessages.
func (s *SQLServerInboundConnector) poll(messageChan chan<- *models.InboundMessage, lastValue interface{}) (interface{}, error) {
	// Take a local snapshot of db to avoid racing with Stop() that sets db=nil
	db := s.db
	if db == nil {
		return nil, fmt.Errorf("db is nil — connector may be stopping")
	}

	query := s.buildQuery(lastValue)
	log.Printf("🔍 SQL Server Inbound: Executing: %s", query)

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	recordCount := 0
	var newLastValue interface{}

	for rows.Next() {
		msg, record, err := ConvertRowToMessageWithRecord(rows, s.config.TableName)
		if err != nil {
			log.Printf("❌ SQL Server Inbound: Row conversion failed: %v", err)
			continue
		}
		msg.SourceType = "sqlserver"

		if s.config.IncrementalColumn != "" {
			if val, ok := record[s.config.IncrementalColumn]; ok {
				newLastValue = val
			}
		}

		messageChan <- msg
		s.BaseInboundConnector.IncrementMessagesReceived() // P2
		recordCount++

		// After-processing via shared dialect-aware helper (P1)
		if s.config.AfterProcessing != "nothing" && s.config.AfterProcessing != "" {
			if id, ok := record["id"]; ok && id != nil {
				if err := SharedAfterProcess(db, s.config, id, SQLServerDialect); err != nil {
					log.Printf("⚠️  SQL Server Inbound: After-process failed: %v", err)
				}
			}
		}
	}

	if err := rows.Err(); err != nil {
		return newLastValue, fmt.Errorf("rows error: %w", err)
	}

	if recordCount > 0 {
		log.Printf("✅ SQL Server Inbound: Polled %d rows from [%s]", recordCount, s.config.TableName)
	}
	return newLastValue, nil
}

// Stop halts polling and closes the DB connection.
func (s *SQLServerInboundConnector) Stop() error {
	log.Printf("🗄️  SQL Server Inbound: Stopping")
	if err := s.BaseInboundConnector.Stop(); err != nil {
		return err
	}
	if s.db != nil {
		if err := s.db.Close(); err != nil {
			return fmt.Errorf("failed to close SQL Server connection: %w", err)
		}
		s.db = nil
	}
	log.Printf("✅ SQL Server Inbound: Stopped")
	return nil
}

// Close is an alias for Stop.
func (s *SQLServerInboundConnector) Close() error { return s.Stop() }

// GetStatus returns connector status enriched with SQL Server-specific metadata.
func (s *SQLServerInboundConnector) GetStatus() ConnectorStatus {
	status := s.BaseInboundConnector.GetStatus()
	if s.db != nil {
		if err := s.db.Ping(); err == nil {
			status.Connected = true
		} else {
			status.Connected = false
			status.LastError = err.Error()
		}
	}
	status.Metadata["database"] = s.config.Database
	status.Metadata["host"] = s.config.Host
	status.Metadata["table"] = s.config.TableName
	return status
}
