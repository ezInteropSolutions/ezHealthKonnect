// services/connectors/oracle_inbound.go
// Oracle Inbound Connector — polls an Oracle table for new rows and delivers
// them as InboundMessages for the pipeline.
//
// Enterprise features (v1.1):
//   - SharedAfterProcess via OracleDialect (:N positional params, unquoted identifiers)
//   - Metrics: IncrementMessagesReceived / RecordError / ClearError on every poll
//   - Watermark persistence: SetMetadata("last_cursor", ...) after each watermark advance
//   - Reconnect with exponential backoff on connection errors
//   - CLOB support: go-ora's Clob implements fmt.Stringer — handled in ConvertRowToMessageWithRecord
//
// Configuration:
//
//	host               string  Oracle host (default: localhost)
//	port               int     Oracle port (default: 1521)
//	database           string  Service name / SID (required)
//	username           string  Oracle user (required)
//	password           string  Oracle password
//	table_name         string  Table to poll (required if query not set)
//	query              string  Custom SQL — overrides table_name/incremental logic
//	incremental_column string  Column for watermark polling (e.g. ID, UPDATED_AT)
//	incremental_type   string  "integer" | "timestamp" | "datetime"
//	polling_interval   int     Seconds between polls (default: 60)
//	max_records        int     FETCH FIRST N ROWS (default: 100)
//	order_by           string  ORDER BY clause (appended verbatim)
//	after_processing   string  "nothing" | "delete" | "update_flag"
//	processed_flag_col string  Column to update for update_flag mode
//	processed_flag_val string  Value to set for update_flag mode
//	ssl_mode           string  "disable" | "require"
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

	_ "github.com/sijms/go-ora/v2" // Registers "oracle" driver
)

const (
	oracleMaxReconnectDelay = 5 * time.Minute
)

// OracleInboundConnector polls an Oracle table for new rows.
type OracleInboundConnector struct {
	*BaseInboundConnector
	config DatabaseInboundConfig
	db     *sql.DB
}

// NewOracleInboundConnector creates a production Oracle inbound connector.
func NewOracleInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "oracle_inbound",
		DisplayName:        "Oracle Database Reader",
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
	return &OracleInboundConnector{
		BaseInboundConnector: NewBaseInboundConnector(metadata),
	}
}

// Initialize parses config, builds the Oracle DSN, opens the DB,
// pings to verify connectivity, and marks the connector ready.
func (o *OracleInboundConnector) Initialize(config []byte) error {
	log.Printf("🏛️  Oracle Inbound: Initializing")

	parsedConfig, err := ParseDatabaseInboundConfig(config)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}
	o.config = *parsedConfig

	db, err := sql.Open("oracle", o.buildDSN())
	if err != nil {
		return fmt.Errorf("failed to open Oracle connection: %w", err)
	}

	ConfigureConnectionPool(db, o.config.MaxOpenConns, o.config.MaxIdleConns)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("failed to ping Oracle at %s:%d: %w", o.config.Host, o.config.Port, err)
	}

	o.db = db
	o.BaseInboundConnector.BaseConnector.initialized = true
	o.BaseInboundConnector.SetState(StateReady)

	log.Printf("✅ Oracle Inbound: Initialized (host=%s, service=%s)", o.config.Host, o.config.Database)
	return nil
}

// buildDSN constructs the go-ora connection URL.
//
//	oracle://user:pass@host:port/service_name
func (o *OracleInboundConnector) buildDSN() string {
	host := o.config.Host
	if host == "" {
		host = "localhost"
	}
	port := o.config.Port
	if port == 0 {
		port = 1521
	}

	dsn := fmt.Sprintf("oracle://%s:%s@%s:%d/%s",
		o.config.Username, o.config.Password, host, port, o.config.Database)

	if o.config.SSLMode == "require" {
		dsn += "?ssl=true&ssl verify=false"
	}

	return dsn
}

// reconnect closes the existing connection and re-opens a fresh one.
func (o *OracleInboundConnector) reconnect() error {
	if o.db != nil {
		_ = o.db.Close()
	}
	db, err := sql.Open("oracle", o.buildDSN())
	if err != nil {
		return err
	}
	ConfigureConnectionPool(db, o.config.MaxOpenConns, o.config.MaxIdleConns)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return err
	}
	o.db = db
	return nil
}

// buildQuery constructs the SELECT statement using Oracle 12c+ syntax.
//
// Oracle uses FETCH FIRST N ROWS ONLY (not LIMIT).
func (o *OracleInboundConnector) buildQuery(lastValue interface{}) string {
	if o.config.Query != "" {
		return o.config.Query
	}

	top := o.config.MaxRecords
	if top <= 0 {
		top = 100
	}

	query := fmt.Sprintf("SELECT * FROM %s", o.config.TableName)

	if o.config.IncrementalColumn != "" && lastValue != nil {
		switch o.config.IncrementalType {
		case "timestamp", "datetime":
			query += fmt.Sprintf(" WHERE %s > TIMESTAMP '%v'", o.config.IncrementalColumn, lastValue)
		default:
			query += fmt.Sprintf(" WHERE %s > :1", o.config.IncrementalColumn)
		}
	}

	if o.config.OrderBy != "" {
		query += fmt.Sprintf(" ORDER BY %s", o.config.OrderBy)
	} else if o.config.IncrementalColumn != "" {
		query += fmt.Sprintf(" ORDER BY %s ASC", o.config.IncrementalColumn)
	}

	query += fmt.Sprintf(" FETCH FIRST %d ROWS ONLY", top)
	return query
}

// TestConnection pings the Oracle database with a context deadline.
func (o *OracleInboundConnector) TestConnection(ctx context.Context) error {
	if o.db == nil {
		return fmt.Errorf("not initialized")
	}
	return o.db.PingContext(ctx)
}

// Validate checks that the connector is initialized and has required fields.
func (o *OracleInboundConnector) Validate() error {
	if !o.BaseInboundConnector.BaseConnector.initialized {
		return fmt.Errorf("connector not initialized")
	}
	if o.config.TableName == "" && o.config.Query == "" {
		return fmt.Errorf("either table_name or query must be specified")
	}
	if o.config.Database == "" {
		return fmt.Errorf("database (service name) is required")
	}
	return nil
}

// SupportsCron returns true — Oracle poll mode supports cron scheduling.
func (o *OracleInboundConnector) SupportsCron() bool { return true }

// Start launches the poll goroutine.
func (o *OracleInboundConnector) Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	if !o.BaseInboundConnector.BaseConnector.initialized {
		return fmt.Errorf("connector not initialized")
	}
	o.BaseInboundConnector.SetState(StateRunning)
	log.Printf("🏛️  Oracle Inbound: Starting (interval=%ds)", o.config.PollingInterval)
	go o.pollLoop(ctx, messageChan)
	return nil
}

// pollLoop polls immediately on start then on every ticker interval, with
// exponential backoff reconnect on connection errors.
func (o *OracleInboundConnector) pollLoop(ctx context.Context, messageChan chan<- *models.InboundMessage) {
	defer o.BaseInboundConnector.SetState(StateReady)

	interval := o.config.PollingInterval
	if interval == 0 {
		interval = 60
	}
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	var lastValue interface{}
	reconnectAttempts := 0

	// Poll immediately on start
	if newLast, err := o.poll(messageChan, lastValue); err != nil {
		o.BaseInboundConnector.RecordError(err)
		log.Printf("❌ Oracle Inbound: Initial poll error: %v", err)
	} else {
		if newLast != nil {
			lastValue = newLast
			o.BaseInboundConnector.SetMetadata("last_cursor", fmt.Sprintf("%v", lastValue))
		}
		o.BaseInboundConnector.ClearError()
	}

	for {
		select {
		case <-ctx.Done():
			log.Printf("🏛️  Oracle Inbound: Context cancelled")
			return
		case <-o.BaseInboundConnector.stopCh:
			log.Printf("🏛️  Oracle Inbound: Stop signal received")
			return
		case <-ticker.C:
			newLast, err := o.poll(messageChan, lastValue)
			if err != nil {
				o.BaseInboundConnector.RecordError(err)
				log.Printf("❌ Oracle Inbound: Poll error: %v", err)

				if IsConnectionError(err) {
					reconnectAttempts++
					delay := time.Duration(reconnectAttempts*reconnectAttempts) * time.Second
					if delay > oracleMaxReconnectDelay {
						delay = oracleMaxReconnectDelay
					}
					log.Printf("🔄 Oracle Inbound: Connection lost — reconnecting in %v (attempt %d)", delay, reconnectAttempts)
					o.BaseInboundConnector.SetState(StateError)
					select {
					case <-time.After(delay):
					case <-ctx.Done():
						return
					case <-o.BaseInboundConnector.stopCh:
						return
					}
					if err := o.reconnect(); err != nil {
						log.Printf("❌ Oracle Inbound: Reconnect failed: %v", err)
					} else {
						log.Printf("✅ Oracle Inbound: Reconnected successfully")
						o.BaseInboundConnector.SetState(StateRunning)
						reconnectAttempts = 0
					}
				}
			} else {
				if newLast != nil {
					lastValue = newLast
					o.BaseInboundConnector.SetMetadata("last_cursor", fmt.Sprintf("%v", lastValue)) // P3
				}
				o.BaseInboundConnector.ClearError()
				reconnectAttempts = 0
			}
		}
	}
}

// poll executes one query and sends matching rows as InboundMessages.
// Returns the last incremental value seen (for next poll's watermark).
func (o *OracleInboundConnector) poll(messageChan chan<- *models.InboundMessage, lastValue interface{}) (interface{}, error) {
	// Take a local snapshot of db to avoid racing with Stop() that sets db=nil
	db := o.db
	if db == nil {
		return nil, fmt.Errorf("db is nil — connector may be stopping")
	}

	query := o.buildQuery(lastValue)
	log.Printf("🔍 Oracle Inbound: Executing: %s", query)

	// Pass bind variable when incremental integer watermark is active
	var rows *sql.Rows
	var err error
	if o.config.IncrementalColumn != "" && lastValue != nil &&
		o.config.IncrementalType != "timestamp" && o.config.IncrementalType != "datetime" {
		rows, err = db.Query(query, lastValue)
	} else {
		rows, err = db.Query(query)
	}
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	recordCount := 0
	var newLastValue interface{}

	for rows.Next() {
		msg, record, err := ConvertRowToMessageWithRecord(rows, o.config.TableName)
		if err != nil {
			log.Printf("❌ Oracle Inbound: Row conversion failed: %v", err)
			continue
		}
		msg.SourceType = "oracle"

		if o.config.IncrementalColumn != "" {
			if val, ok := record[o.config.IncrementalColumn]; ok {
				newLastValue = val
			}
		}

		messageChan <- msg
		o.BaseInboundConnector.IncrementMessagesReceived() // P2
		recordCount++

		// After-processing via shared dialect-aware helper (P1)
		if o.config.AfterProcessing != "nothing" && o.config.AfterProcessing != "" {
			if id, ok := record["id"]; ok && id != nil {
				if err := SharedAfterProcess(db, o.config, id, OracleDialect); err != nil {
					log.Printf("⚠️  Oracle Inbound: After-process failed: %v", err)
				}
			}
		}
	}

	if err := rows.Err(); err != nil {
		return newLastValue, fmt.Errorf("rows error: %w", err)
	}

	if recordCount > 0 {
		log.Printf("✅ Oracle Inbound: Polled %d rows from %s", recordCount, o.config.TableName)
	}
	return newLastValue, nil
}

// Stop halts polling and closes the DB connection.
func (o *OracleInboundConnector) Stop() error {
	log.Printf("🏛️  Oracle Inbound: Stopping")
	if err := o.BaseInboundConnector.Stop(); err != nil {
		return err
	}
	if o.db != nil {
		if err := o.db.Close(); err != nil {
			return fmt.Errorf("failed to close Oracle connection: %w", err)
		}
		o.db = nil
	}
	log.Printf("✅ Oracle Inbound: Stopped")
	return nil
}

// Close is an alias for Stop.
func (o *OracleInboundConnector) Close() error { return o.Stop() }

// GetStatus returns connector status enriched with Oracle-specific metadata.
func (o *OracleInboundConnector) GetStatus() ConnectorStatus {
	status := o.BaseInboundConnector.GetStatus()
	if o.db != nil {
		if err := o.db.Ping(); err == nil {
			status.Connected = true
		} else {
			status.Connected = false
			status.LastError = err.Error()
		}
	}
	status.Metadata["service_name"] = o.config.Database
	status.Metadata["host"] = o.config.Host
	status.Metadata["table"] = o.config.TableName
	return status
}
