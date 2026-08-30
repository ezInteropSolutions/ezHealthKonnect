// services/connectors/snowflake_inbound.go
// Snowflake Inbound Connector — polls a Snowflake table and converts rows to
// InboundMessages for the pipeline. Uses the official Snowflake Go driver
// (github.com/snowflakedb/gosnowflake/v2, same driver as snowflake_outbound.go),
// reused as a plain database/sql driver so this mirrors mysql_inbound.go's
// polling/incremental/reconnect design (watermark tracking, exponential-backoff
// reconnect, shared after-processing) — same pattern databricks_inbound.go
// already follows for the other cloud SQL warehouse.
//
// NOTE — same caveat as snowflake_outbound.go: there is no way to verify this
// against a real Snowflake account in this environment (cloud-only service,
// no local emulator, no test credentials available). Verified via unit tests
// of pure logic (DSN building, query construction, config validation) and a
// clean compile — NOT against a live Snowflake warehouse. Treat actual
// connectivity as unverified until tried against a real account.
//
// SCOPE — username/password authentication only, same scope decision as
// snowflake_outbound.go. Key-pair (JWT) authentication is explicitly NOT
// implemented; Initialize() rejects auth_type="key_pair" or a non-empty
// private_key with a clear error rather than silently ignoring it.
//
// Configuration (DatabaseInboundConfig fields — reusing the existing shared
// struct's cloud-warehouse fields, not new ones):
//
//	account            string  Snowflake account identifier, e.g. "xy12345.us-east-1" (required)
//	username           string  Snowflake username (required)
//	password           string  Snowflake password (required)
//	database           string  Database name
//	schema             string  Schema name within the database
//	warehouse          string  Virtual warehouse to use for the session
//	role               string  Snowflake role to assume
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
//
// NOT supported in this pass (present in DatabaseInboundConfig but rejected by
// Initialize() if set): auth_type="key_pair", private_key, private_key_pass.
package connectors

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"ezhealthkonnect/models"

	_ "github.com/snowflakedb/gosnowflake/v2" // Registers "snowflake" driver
)

const (
	snowflakeInboundMaxReconnectDelay = 5 * time.Minute
)

// snowflakeInboundDialect adapts SharedAfterProcess to Snowflake's "?"
// placeholders and double-quote identifier quoting (quoteIdentSnowflake is
// defined once, in snowflake_outbound.go, and shared across this package).
type snowflakeInboundDialect struct{}

func (snowflakeInboundDialect) Placeholder(_ int) string      { return "?" }
func (snowflakeInboundDialect) QuoteIdent(name string) string { return quoteIdentSnowflake(name) }
func (snowflakeInboundDialect) DriverName() string             { return "snowflake" }

var SnowflakeDialect SQLDialect = snowflakeInboundDialect{}

// SnowflakeInboundConnector polls a Snowflake table for new rows.
type SnowflakeInboundConnector struct {
	*BaseInboundConnector
	config DatabaseInboundConfig
	db     *sql.DB
}

// NewSnowflakeInboundConnector creates a Snowflake inbound connector using the
// official Snowflake Go driver.
func NewSnowflakeInboundConnector() InboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "snowflake_inbound",
		DisplayName:        "Snowflake Data Warehouse Reader",
		Version:            "1.0.0",
		Category:           "inbound",
		Mode:               "pull",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_cron":             true,
			"supports_password_auth":    true,
			"supports_incremental":      true,
			"supports_after_processing": true,
			"supports_reconnect":        true,
			// supports_oauth and supports_key_pair_auth deliberately omitted —
			// same scope decision as snowflake_outbound.go.
		},
	}
	return &SnowflakeInboundConnector{
		BaseInboundConnector: NewBaseInboundConnector(metadata),
	}
}

// Initialize parses config, rejects unimplemented auth modes, opens the
// Snowflake connection, and pings to verify.
func (s *SnowflakeInboundConnector) Initialize(config []byte) error {
	log.Printf("❄️  Snowflake Inbound: Initializing")

	parsedConfig, err := ParseDatabaseInboundConfig(config)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}
	s.config = *parsedConfig

	if s.config.AuthType == "key_pair" || s.config.PrivateKey != "" {
		return fmt.Errorf("key-pair authentication is not yet implemented for Snowflake inbound — set auth_type to \"password\" (or leave it blank) and provide username/password instead")
	}
	if s.config.Account == "" {
		return fmt.Errorf("account is required")
	}
	if s.config.Username == "" {
		return fmt.Errorf("username is required")
	}
	if s.config.Password == "" {
		return fmt.Errorf("password is required")
	}

	db, err := sql.Open("snowflake", s.buildDSN())
	if err != nil {
		return fmt.Errorf("failed to open Snowflake connection: %w", err)
	}

	if s.config.MaxOpenConns > 0 {
		db.SetMaxOpenConns(s.config.MaxOpenConns)
	}
	if s.config.MaxIdleConns > 0 {
		db.SetMaxIdleConns(s.config.MaxIdleConns)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("failed to ping Snowflake account %s: %w", s.config.Account, err)
	}

	s.db = db
	s.BaseInboundConnector.BaseConnector.initialized = true
	s.BaseInboundConnector.SetState(StateReady)

	log.Printf("✅ Snowflake Inbound: Initialized (account=%s, database=%s, schema=%s, table=%s)",
		s.config.Account, s.config.Database, s.config.Schema, s.config.TableName)
	return nil
}

// buildDSN constructs the driver's DSN — identical format to snowflake_outbound.go.
func (s *SnowflakeInboundConnector) buildDSN() string {
	userInfo := fmt.Sprintf("%s:%s", url.QueryEscape(s.config.Username), url.QueryEscape(s.config.Password))

	path := s.config.Account
	if s.config.Database != "" {
		path += "/" + s.config.Database
		if s.config.Schema != "" {
			path += "/" + s.config.Schema
		}
	}

	dsn := fmt.Sprintf("%s@%s", userInfo, path)

	params := []string{}
	if s.config.Warehouse != "" {
		params = append(params, "warehouse="+url.QueryEscape(s.config.Warehouse))
	}
	if s.config.Role != "" {
		params = append(params, "role="+url.QueryEscape(s.config.Role))
	}
	if len(params) > 0 {
		dsn += "?" + strings.Join(params, "&")
	}
	return dsn
}

// reconnect closes the existing connection and re-opens a fresh one.
func (s *SnowflakeInboundConnector) reconnect() error {
	if s.db != nil {
		_ = s.db.Close()
	}
	db, err := sql.Open("snowflake", s.buildDSN())
	if err != nil {
		return err
	}
	if s.config.MaxOpenConns > 0 {
		db.SetMaxOpenConns(s.config.MaxOpenConns)
	}
	if s.config.MaxIdleConns > 0 {
		db.SetMaxIdleConns(s.config.MaxIdleConns)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return err
	}
	s.db = db
	return nil
}

// qualifiedTable returns database.schema.table, each part double-quoted
// (Snowflake ANSI-SQL convention), omitting empty parts — same convention as
// snowflake_outbound.go's qualifiedTable.
func (s *SnowflakeInboundConnector) qualifiedTable() string {
	parts := []string{}
	if s.config.Database != "" {
		parts = append(parts, quoteIdentSnowflake(s.config.Database))
	}
	if s.config.Schema != "" {
		parts = append(parts, quoteIdentSnowflake(s.config.Schema))
	}
	parts = append(parts, quoteIdentSnowflake(s.config.TableName))
	return strings.Join(parts, ".")
}

// buildQuery constructs the SELECT query with optional incremental filter —
// same shape as databricks_inbound.go's buildQuery, adapted to double-quote
// identifier quoting.
func (s *SnowflakeInboundConnector) buildQuery(lastValue interface{}) string {
	if s.config.Query != "" {
		return s.config.Query
	}

	query := fmt.Sprintf("SELECT * FROM %s", s.qualifiedTable())

	if s.config.IncrementalColumn != "" && lastValue != nil {
		switch s.config.IncrementalType {
		case "timestamp", "datetime":
			query += fmt.Sprintf(" WHERE %s > '%v'", quoteIdentSnowflake(s.config.IncrementalColumn), lastValue)
		default:
			query += fmt.Sprintf(" WHERE %s > %v", quoteIdentSnowflake(s.config.IncrementalColumn), lastValue)
		}
	}

	if s.config.OrderBy != "" {
		query += fmt.Sprintf(" ORDER BY %s", s.config.OrderBy)
	} else if s.config.IncrementalColumn != "" {
		query += fmt.Sprintf(" ORDER BY %s", quoteIdentSnowflake(s.config.IncrementalColumn))
	}

	if s.config.MaxRecords > 0 {
		query += fmt.Sprintf(" LIMIT %d", s.config.MaxRecords)
	}

	return query
}

// TestConnection pings the Snowflake account.
func (s *SnowflakeInboundConnector) TestConnection(ctx context.Context) error {
	if s.db == nil {
		return fmt.Errorf("not initialized")
	}
	return s.db.PingContext(ctx)
}

// Validate checks required fields.
func (s *SnowflakeInboundConnector) Validate() error {
	if !s.BaseInboundConnector.BaseConnector.initialized {
		return fmt.Errorf("connector not initialized")
	}
	if s.config.Account == "" {
		return fmt.Errorf("account is required")
	}
	if s.config.Username == "" {
		return fmt.Errorf("username is required")
	}
	if s.config.Password == "" {
		return fmt.Errorf("password is required")
	}
	if s.config.TableName == "" && s.config.Query == "" {
		return fmt.Errorf("either table_name or query must be specified")
	}
	return nil
}

// SupportsCron returns true — Snowflake poll mode supports cron scheduling.
func (s *SnowflakeInboundConnector) SupportsCron() bool { return true }

// Start launches the polling goroutine.
func (s *SnowflakeInboundConnector) Start(ctx context.Context, messageChan chan<- *models.InboundMessage) error {
	if !s.BaseInboundConnector.BaseConnector.initialized {
		return fmt.Errorf("connector not initialized")
	}
	s.BaseInboundConnector.SetState(StateRunning)
	log.Printf("❄️  Snowflake Inbound: Starting (interval=%ds)", s.config.PollingInterval)
	go s.pollLoop(ctx, messageChan)
	return nil
}

// pollLoop runs polls on a ticker with exponential backoff reconnect on
// connection errors — same structure as databricks_inbound.go's pollLoop.
func (s *SnowflakeInboundConnector) pollLoop(ctx context.Context, messageChan chan<- *models.InboundMessage) {
	defer s.BaseInboundConnector.SetState(StateReady)

	interval := s.config.PollingInterval
	if interval == 0 {
		interval = 60
	}
	ticker := time.NewTicker(time.Duration(interval) * time.Second)
	defer ticker.Stop()

	var lastValue interface{}
	reconnectAttempts := 0

	if newLast, err := s.poll(messageChan, lastValue); err != nil {
		s.BaseInboundConnector.RecordError(err)
		log.Printf("❌ Snowflake Inbound: Initial poll error: %v", err)
	} else {
		if newLast != nil {
			lastValue = newLast
			s.BaseInboundConnector.SetMetadata("last_cursor", fmt.Sprintf("%v", lastValue))
		}
		s.BaseInboundConnector.ClearError()
		reconnectAttempts = 0
	}

	for {
		select {
		case <-ctx.Done():
			log.Printf("❄️  Snowflake Inbound: Context cancelled")
			return
		case <-s.BaseInboundConnector.stopCh:
			log.Printf("❄️  Snowflake Inbound: Stop signal received")
			return
		case <-ticker.C:
			newLast, err := s.poll(messageChan, lastValue)
			if err != nil {
				s.BaseInboundConnector.RecordError(err)
				log.Printf("❌ Snowflake Inbound: Poll error: %v", err)

				if IsConnectionError(err) {
					reconnectAttempts++
					delay := time.Duration(reconnectAttempts*reconnectAttempts) * time.Second
					if delay > snowflakeInboundMaxReconnectDelay {
						delay = snowflakeInboundMaxReconnectDelay
					}
					log.Printf("🔄 Snowflake Inbound: Connection lost — reconnecting in %v (attempt %d)", delay, reconnectAttempts)
					s.BaseInboundConnector.SetState(StateError)
					select {
					case <-time.After(delay):
					case <-ctx.Done():
						return
					case <-s.BaseInboundConnector.stopCh:
						return
					}
					if err := s.reconnect(); err != nil {
						log.Printf("❌ Snowflake Inbound: Reconnect failed: %v", err)
					} else {
						log.Printf("✅ Snowflake Inbound: Reconnected successfully")
						s.BaseInboundConnector.SetState(StateRunning)
						reconnectAttempts = 0
					}
				}
			} else {
				if newLast != nil {
					lastValue = newLast
					s.BaseInboundConnector.SetMetadata("last_cursor", fmt.Sprintf("%v", lastValue))
				}
				s.BaseInboundConnector.ClearError()
				reconnectAttempts = 0
			}
		}
	}
}

// poll executes one query and sends matching rows as messages.
func (s *SnowflakeInboundConnector) poll(messageChan chan<- *models.InboundMessage, lastValue interface{}) (interface{}, error) {
	db := s.db
	if db == nil {
		return nil, fmt.Errorf("db is nil — connector may be stopping")
	}

	query := s.buildQuery(lastValue)
	log.Printf("🔍 Snowflake Inbound: Executing: %s", query)

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
			log.Printf("❌ Snowflake Inbound: Row conversion failed: %v", err)
			continue
		}
		msg.SourceType = "snowflake"

		if s.config.IncrementalColumn != "" {
			if val, ok := record[s.config.IncrementalColumn]; ok {
				newLastValue = val
			}
		}

		messageChan <- msg
		s.BaseInboundConnector.IncrementMessagesReceived()
		recordCount++

		if s.config.AfterProcessing != "nothing" && s.config.AfterProcessing != "" {
			if id, ok := record["id"]; ok && id != nil {
				if err := SharedAfterProcess(db, s.config, id, SnowflakeDialect); err != nil {
					log.Printf("⚠️  Snowflake Inbound: After-process failed: %v", err)
				}
			}
		}
	}

	if err := rows.Err(); err != nil {
		return newLastValue, fmt.Errorf("rows error: %w", err)
	}

	if recordCount > 0 {
		log.Printf("✅ Snowflake Inbound: Polled %d rows from %s", recordCount, s.config.TableName)
	}
	return newLastValue, nil
}

// Stop halts the connector and closes the DB connection.
func (s *SnowflakeInboundConnector) Stop() error {
	log.Printf("❄️  Snowflake Inbound: Stopping")
	if err := s.BaseInboundConnector.Stop(); err != nil {
		return err
	}
	if s.db != nil {
		if err := s.db.Close(); err != nil {
			return fmt.Errorf("failed to close Snowflake connection: %w", err)
		}
		s.db = nil
	}
	log.Printf("✅ Snowflake Inbound: Stopped")
	return nil
}

// Close is an alias for Stop.
func (s *SnowflakeInboundConnector) Close() error { return s.Stop() }

// GetStatus returns connector status with Snowflake-specific metadata.
func (s *SnowflakeInboundConnector) GetStatus() ConnectorStatus {
	status := s.BaseInboundConnector.GetStatus()
	if s.db != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := s.db.PingContext(ctx); err == nil {
			status.Connected = true
		} else {
			status.Connected = false
			status.LastError = err.Error()
		}
	}
	if status.Metadata == nil {
		status.Metadata = map[string]string{}
	}
	status.Metadata["account"] = s.config.Account
	status.Metadata["database"] = s.config.Database
	status.Metadata["schema"] = s.config.Schema
	status.Metadata["warehouse"] = s.config.Warehouse
	status.Metadata["table"] = s.config.TableName
	return status
}
