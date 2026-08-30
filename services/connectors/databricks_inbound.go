// services/connectors/databricks_inbound.go
// Databricks Inbound Connector — polls a Delta Lake table via a Databricks SQL
// Warehouse and converts rows to InboundMessages for the pipeline. Uses the
// official Databricks SQL Go driver (github.com/databricks/databricks-sql-go,
// same driver as databricks_outbound.go), reused as a plain database/sql
// driver so this mirrors mysql_inbound.go's polling/incremental/reconnect
// design (watermark tracking, exponential-backoff reconnect, shared
// after-processing) rather than inventing a new pattern for cloud warehouses.
//
// NOTE — same caveat as databricks_outbound.go: there is no way to verify this
// against a real Databricks workspace in this environment (cloud-only
// service, no local emulator, no test credentials available). Verified via
// unit tests of pure logic (DSN building, query construction, config
// validation) and a clean compile — NOT against a live SQL Warehouse. Treat
// actual connectivity as unverified until tried against a real workspace.
//
// Configuration (DatabaseInboundConfig fields — reusing the existing shared
// struct's cloud-warehouse fields, not new ones):
//
//	host               string  Databricks workspace server hostname (required)
//	port               int     Port (default: 443)
//	token              string  Personal access token (required)
//	http_path          string  SQL warehouse HTTP path, e.g. /sql/1.0/warehouses/xxxx (required)
//	database           string  Unity Catalog catalog name
//	schema             string  Schema name within the catalog
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
package connectors

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	"ezhealthkonnect/models"

	_ "github.com/databricks/databricks-sql-go" // Registers "databricks" driver
)

const (
	databricksInboundMaxReconnectDelay = 5 * time.Minute
)

// databricksInboundDialect adapts SharedAfterProcess to Databricks SQL's "?"
// placeholders and backtick identifier quoting (quoteIdent is defined once,
// in databricks_outbound.go, and shared across this package).
type databricksInboundDialect struct{}

func (databricksInboundDialect) Placeholder(_ int) string    { return "?" }
func (databricksInboundDialect) QuoteIdent(name string) string { return quoteIdent(name) }
func (databricksInboundDialect) DriverName() string            { return "databricks" }

var DatabricksDialect SQLDialect = databricksInboundDialect{}

// DatabricksInboundConnector polls a Delta Lake table via a Databricks SQL Warehouse.
type DatabricksInboundConnector struct {
	*BaseInboundConnector
	config DatabaseInboundConfig
	db     *sql.DB
}

// NewDatabricksInboundConnector creates a Databricks inbound connector using
// the official Databricks SQL Go driver.
func NewDatabricksInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "databricks_inbound",
		DisplayName:        "Databricks SQL Warehouse Reader",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "pull",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":             true,
			"supports_pat_auth":         true,
			"supports_incremental":      true,
			"supports_after_processing": true,
			"supports_reconnect":        true,
			"supports_unity_cat":        true,
			// supports_oauth deliberately omitted — only personal access token
			// auth is implemented, same scope decision as databricks_outbound.go.
		},
	}
	return &DatabricksInboundConnector{
		BaseInboundConnector: NewBaseInboundConnector(metadata),
	}
}

// Initialize parses config, opens the Databricks SQL Warehouse connection, and pings to verify.
func (d *DatabricksInboundConnector) Initialize(config []byte) error {
	log.Printf("🧱 Databricks Inbound: Initializing")

	parsedConfig, err := ParseDatabaseInboundConfig(config)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}
	d.config = *parsedConfig

	if d.config.Host == "" {
		return fmt.Errorf("host is required")
	}
	if d.config.Token == "" {
		return fmt.Errorf("token is required")
	}
	if d.config.HTTPPath == "" {
		return fmt.Errorf("http_path is required")
	}

	db, err := sql.Open("databricks", d.buildDSN())
	if err != nil {
		return fmt.Errorf("failed to open Databricks connection: %w", err)
	}

	if d.config.MaxOpenConns > 0 {
		db.SetMaxOpenConns(d.config.MaxOpenConns)
	}
	if d.config.MaxIdleConns > 0 {
		db.SetMaxIdleConns(d.config.MaxIdleConns)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("failed to ping Databricks SQL Warehouse at %s: %w", d.config.Host, err)
	}

	d.db = db
	d.BaseInboundConnector.BaseConnector.initialized = true
	d.BaseInboundConnector.SetState(StateReady)

	log.Printf("✅ Databricks Inbound: Initialized (host=%s, catalog=%s, schema=%s, table=%s)",
		d.config.Host, d.config.Database, d.config.Schema, d.config.TableName)
	return nil
}

// buildDSN constructs the driver's DSN — identical format to databricks_outbound.go.
func (d *DatabricksInboundConnector) buildDSN() string {
	port := d.config.Port
	if port == 0 {
		port = 443
	}
	dsn := fmt.Sprintf("token:%s@%s:%d%s", d.config.Token, d.config.Host, port, d.config.HTTPPath)

	params := []string{}
	if d.config.Database != "" {
		params = append(params, "catalog="+d.config.Database)
	}
	if d.config.Schema != "" {
		params = append(params, "schema="+d.config.Schema)
	}
	if len(params) > 0 {
		dsn += "?" + strings.Join(params, "&")
	}
	return dsn
}

// reconnect closes the existing connection and re-opens a fresh one.
func (d *DatabricksInboundConnector) reconnect() error {
	if d.db != nil {
		_ = d.db.Close()
	}
	db, err := sql.Open("databricks", d.buildDSN())
	if err != nil {
		return err
	}
	if d.config.MaxOpenConns > 0 {
		db.SetMaxOpenConns(d.config.MaxOpenConns)
	}
	if d.config.MaxIdleConns > 0 {
		db.SetMaxIdleConns(d.config.MaxIdleConns)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return err
	}
	d.db = db
	return nil
}

// qualifiedTable returns catalog.schema.table (Unity Catalog three-level
// namespace), each part backtick-quoted, omitting empty parts — same
// convention as databricks_outbound.go's qualifiedTable.
func (d *DatabricksInboundConnector) qualifiedTable() string {
	parts := []string{}
	if d.config.Database != "" {
		parts = append(parts, quoteIdent(d.config.Database))
	}
	if d.config.Schema != "" {
		parts = append(parts, quoteIdent(d.config.Schema))
	}
	parts = append(parts, quoteIdent(d.config.TableName))
	return strings.Join(parts, ".")
}

// buildQuery constructs the SELECT query with optional incremental filter —
// same shape as mysql_inbound.go's buildQuery, adapted to backtick quoting
// and the catalog.schema.table namespace.
func (d *DatabricksInboundConnector) buildQuery(lastValue interface{}) string {
	if d.config.Query != "" {
		return d.config.Query
	}

	query := fmt.Sprintf("SELECT * FROM %s", d.qualifiedTable())

	if d.config.IncrementalColumn != "" && lastValue != nil {
		switch d.config.IncrementalType {
		case "timestamp", "datetime":
			query += fmt.Sprintf(" WHERE %s > '%v'", quoteIdent(d.config.IncrementalColumn), lastValue)
		default:
			query += fmt.Sprintf(" WHERE %s > %v", quoteIdent(d.config.IncrementalColumn), lastValue)
		}
	}

	if d.config.OrderBy != "" {
		query += fmt.Sprintf(" ORDER BY %s", d.config.OrderBy)
	} else if d.config.IncrementalColumn != "" {
		query += fmt.Sprintf(" ORDER BY %s", quoteIdent(d.config.IncrementalColumn))
	}

	if d.config.MaxRecords > 0 {
		query += fmt.Sprintf(" LIMIT %d", d.config.MaxRecords)
	}

	return query
}

// TestConnection pings the Databricks SQL Warehouse with a generous deadline
// — SQL Warehouses can take longer to respond than a traditional DB if the
// warehouse is currently stopped/starting up (same rationale as
// databricks_outbound.go's TestConnection).
func (d *DatabricksInboundConnector) TestConnection(ctx context.Context) error {
	if d.db == nil {
		return fmt.Errorf("not initialized")
	}
	return d.db.PingContext(ctx)
}

// Validate checks required fields.
func (d *DatabricksInboundConnector) Validate() error {
	if !d.BaseInboundConnector.BaseConnector.initialized {
		return fmt.Errorf("connector not initialized")
	}
	if d.config.Host == "" {
		return fmt.Errorf("host is required")
	}
	if d.config.Token == "" {
		return fmt.Errorf("token is required")
	}
	if d.config.HTTPPath == "" {
		return fmt.Errorf("http_path is required")
	}
	if d.config.TableName == "" && d.config.Query == "" {
		return fmt.Errorf("either table_name or query must be specified")
	}
	return nil
}

// SupportsCron returns true — Databricks poll mode supports cron scheduling.
func (d *DatabricksInboundConnector) SupportsCron() bool { return true }

// Start launches the polling goroutine.
func (d *DatabricksInboundConnector) Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	if !d.BaseInboundConnector.BaseConnector.initialized {
		return fmt.Errorf("connector not initialized")
	}
	d.BaseInboundConnector.SetState(StateRunning)
	log.Printf("🧱 Databricks Inbound: Starting (interval=%ds)", d.config.PollingInterval)
	go d.pollLoop(ctx, messageChan)
	return nil
}

// pollLoop runs polls on a ticker with exponential backoff reconnect on
// connection errors — same structure as mysql_inbound.go's pollLoop.
func (d *DatabricksInboundConnector) pollLoop(ctx context.Context, messageChan chan<- *models.InboundMessage) {
	defer d.BaseInboundConnector.SetState(StateReady)

	interval := d.config.PollingInterval
	if interval == 0 {
		interval = 60
	}
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	var lastValue interface{}
	reconnectAttempts := 0

	if newLast, err := d.poll(messageChan, lastValue); err != nil {
		d.BaseInboundConnector.RecordError(err)
		log.Printf("❌ Databricks Inbound: Initial poll error: %v", err)
	} else {
		if newLast != nil {
			lastValue = newLast
			d.BaseInboundConnector.SetMetadata("last_cursor", fmt.Sprintf("%v", lastValue))
		}
		d.BaseInboundConnector.ClearError()
		reconnectAttempts = 0
	}

	for {
		select {
		case <-ctx.Done():
			log.Printf("🧱 Databricks Inbound: Context cancelled")
			return
		case <-d.BaseInboundConnector.stopCh:
			log.Printf("🧱 Databricks Inbound: Stop signal received")
			return
		case <-ticker.C:
			newLast, err := d.poll(messageChan, lastValue)
			if err != nil {
				d.BaseInboundConnector.RecordError(err)
				log.Printf("❌ Databricks Inbound: Poll error: %v", err)

				if IsConnectionError(err) {
					reconnectAttempts++
					delay := time.Duration(reconnectAttempts*reconnectAttempts) * time.Second
					if delay > databricksInboundMaxReconnectDelay {
						delay = databricksInboundMaxReconnectDelay
					}
					log.Printf("🔄 Databricks Inbound: Connection lost — reconnecting in %v (attempt %d)", delay, reconnectAttempts)
					d.BaseInboundConnector.SetState(StateError)
					select {
					case <-time.After(delay):
					case <-ctx.Done():
						return
					case <-d.BaseInboundConnector.stopCh:
						return
					}
					if err := d.reconnect(); err != nil {
						log.Printf("❌ Databricks Inbound: Reconnect failed: %v", err)
					} else {
						log.Printf("✅ Databricks Inbound: Reconnected successfully")
						d.BaseInboundConnector.SetState(StateRunning)
						reconnectAttempts = 0
					}
				}
			} else {
				if newLast != nil {
					lastValue = newLast
					d.BaseInboundConnector.SetMetadata("last_cursor", fmt.Sprintf("%v", lastValue))
				}
				d.BaseInboundConnector.ClearError()
				reconnectAttempts = 0
			}
		}
	}
}

// poll executes one query and sends matching rows as messages.
func (d *DatabricksInboundConnector) poll(messageChan chan<- *models.InboundMessage, lastValue interface{}) (interface{}, error) {
	db := d.db
	if db == nil {
		return nil, fmt.Errorf("db is nil — connector may be stopping")
	}

	query := d.buildQuery(lastValue)
	log.Printf("🔍 Databricks Inbound: Executing: %s", query)

	rows, err := db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	recordCount := 0
	var newLastValue interface{}

	for rows.Next() {
		msg, record, err := ConvertRowToMessageWithRecord(rows, d.config.TableName)
		if err != nil {
			log.Printf("❌ Databricks Inbound: Row conversion failed: %v", err)
			continue
		}
		msg.SourceType = "databricks"

		if d.config.IncrementalColumn != "" {
			if val, ok := record[d.config.IncrementalColumn]; ok {
				newLastValue = val
			}
		}

		messageChan <- msg
		d.BaseInboundConnector.IncrementMessagesReceived()
		recordCount++

		if d.config.AfterProcessing != "nothing" && d.config.AfterProcessing != "" {
			if id, ok := record["id"]; ok && id != nil {
				if err := SharedAfterProcess(db, d.config, id, DatabricksDialect); err != nil {
					log.Printf("⚠️  Databricks Inbound: After-process failed: %v", err)
				}
			}
		}
	}

	if err := rows.Err(); err != nil {
		return newLastValue, fmt.Errorf("rows error: %w", err)
	}

	if recordCount > 0 {
		log.Printf("✅ Databricks Inbound: Polled %d rows from %s", recordCount, d.config.TableName)
	}
	return newLastValue, nil
}

// Stop halts the connector and closes the DB connection.
func (d *DatabricksInboundConnector) Stop() error {
	log.Printf("🧱 Databricks Inbound: Stopping")
	if err := d.BaseInboundConnector.Stop(); err != nil {
		return err
	}
	if d.db != nil {
		if err := d.db.Close(); err != nil {
			return fmt.Errorf("failed to close Databricks connection: %w", err)
		}
		d.db = nil
	}
	log.Printf("✅ Databricks Inbound: Stopped")
	return nil
}

// Close is an alias for Stop.
func (d *DatabricksInboundConnector) Close() error { return d.Stop() }

// GetStatus returns connector status with Databricks-specific metadata.
func (d *DatabricksInboundConnector) GetStatus() ConnectorStatus {
	status := d.BaseInboundConnector.GetStatus()
	if d.db != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := d.db.PingContext(ctx); err == nil {
			status.Connected = true
		} else {
			status.Connected = false
			status.LastError = err.Error()
		}
	}
	if status.Metadata == nil {
		status.Metadata = map[string]string{}
	}
	status.Metadata["host"] = d.config.Host
	status.Metadata["catalog"] = d.config.Database
	status.Metadata["schema"] = d.config.Schema
	status.Metadata["table"] = d.config.TableName
	return status
}
