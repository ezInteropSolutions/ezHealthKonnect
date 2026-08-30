// services/connectors/snowflake_outbound.go
// Snowflake Outbound Connector — writes messages to a Snowflake table using the
// official Snowflake Go driver (github.com/snowflakedb/gosnowflake/v2 — a real
// database/sql driver, not a hand-rolled REST client), so this reuses the exact
// same Exec/ExecContext idioms as postgresql_outbound.go/mysql_outbound.go/
// databricks_outbound.go.
//
// NOTE — same caveat as databricks_outbound.go: there is no way to verify this
// against a real Snowflake account in this environment (cloud-only service, no
// local emulator, no test credentials available). This has been verified via
// unit tests of the pure logic (DSN building, MERGE query construction,
// identifier-quoting/escaping, config validation) and a clean compile — NOT
// against a live Snowflake warehouse. Treat the actual connectivity as
// unverified until tried against a real account.
//
// SCOPE — username/password authentication only. Snowflake's key-pair (JWT)
// authentication requires parsing a base64-encoded PKCS8 RSA private key into
// an *rsa.PrivateKey via the driver's Config struct + a NewConnector/OpenDB
// construction path, rather than a simple DSN string. That's real, more
// complex, and unverifiable without a real key-pair-configured account — so,
// same as the SFTP key-based-auth gap found earlier this session, it is
// explicitly NOT implemented here. Initialize() rejects any config that asks
// for it (auth_type="key_pair" or a non-empty private_key) with a clear error
// rather than silently ignoring those fields or attempting something unverified.
//
// Write modes:
//   - "insert"  — plain INSERT INTO "database"."schema"."table" (...) VALUES (...)
//   - "upsert"  — MERGE INTO ... USING ... ON ... WHEN MATCHED / NOT MATCHED (standard ANSI MERGE, which Snowflake supports natively)
//   - "update"  — alias for upsert
//
// Configuration (DatabaseOutboundConfig fields — reusing the existing shared
// struct's fields already provisioned for cloud warehouses, not new ones):
//
//	account            string  Snowflake account identifier, e.g. "xy12345.us-east-1" (required)
//	username           string  Snowflake username (required)
//	password           string  Snowflake password (required)
//	database           string  Database name
//	schema             string  Schema name within the database
//	warehouse          string  Virtual warehouse to use for the session
//	role               string  Snowflake role to assume
//	table_name         string  Destination table (required)
//	write_mode         string  "insert" | "upsert" | "update" (default: insert)
//	unique_key         string  Merge key column(s) for upsert/update, comma-separated (required when write_mode != insert)
//	batch_size         int     Records per batch (informational only — see SendBatch note)
//
// NOT supported in this pass (present in DatabaseOutboundConfig but rejected
// by Initialize() if set): auth_type="key_pair", private_key, private_key_pass.
package connectors

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"ezhealthkonnect/models"

	_ "github.com/snowflakedb/gosnowflake/v2" // Registers "snowflake" driver
)

// SnowflakeOutboundConnector writes messages to a Snowflake table.
type SnowflakeOutboundConnector struct {
	*BaseOutboundConnector
	config DatabaseOutboundConfig
	db     *sql.DB
}

// NewSnowflakeOutboundConnector creates a Snowflake outbound connector using
// the official Snowflake Go driver.
func NewSnowflakeOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "snowflake_outbound",
		DisplayName:        "Snowflake Data Warehouse Writer",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch":          true,
			"supports_password_auth":  true,
			"supports_merge":          true,
			"supports_pool":           true,
			// supports_oauth, supports_key_pair_auth, supports_stage_copy, and
			// supports_warehouse_mgmt are deliberately omitted — only
			// username/password auth is implemented, and there is no
			// COPY INTO / stage-based bulk loading in this pass. Claiming them
			// would repeat the false-capability problem found in several stubs
			// earlier this session.
		},
	}
	return &SnowflakeOutboundConnector{
		BaseOutboundConnector: NewBaseOutboundConnector(metadata, true),
	}
}

// Initialize parses config, rejects unimplemented auth modes, opens the
// Snowflake connection, and pings to verify.
func (s *SnowflakeOutboundConnector) Initialize(config []byte) error {
	log.Printf("❄️  Snowflake Outbound: Initializing")

	parsedConfig, err := ParseDatabaseOutboundConfig(config)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}
	s.config = *parsedConfig

	if s.config.AuthType == "key_pair" || s.config.PrivateKey != "" {
		return fmt.Errorf("key-pair authentication is not yet implemented for Snowflake outbound — set auth_type to \"password\" (or leave it blank) and provide username/password instead")
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
	if s.config.TableName == "" {
		return fmt.Errorf("table_name is required")
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
	s.BaseOutboundConnector.BaseConnector.initialized = true

	log.Printf("✅ Snowflake Outbound: Initialized (account=%s, database=%s, schema=%s, table=%s, mode=%s)",
		s.config.Account, s.config.Database, s.config.Schema, s.config.TableName, s.config.WriteMode)
	return nil
}

// buildDSN constructs the driver's DSN: "user:password@account/database/schema?warehouse=X&role=Y"
// — confirmed format from the official driver's own documentation. Username
// and password are URL-escaped since passwords commonly contain characters
// (@, /, ?, &) that would otherwise corrupt the DSN.
func (s *SnowflakeOutboundConnector) buildDSN() string {
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

// quoteIdentSnowflake quotes an identifier using Snowflake's ANSI-SQL
// double-quote convention (NOT backticks like MySQL/Databricks). Any embedded
// double quote is escaped by doubling it, per the ANSI SQL standard.
func quoteIdentSnowflake(name string) string {
	escaped := strings.ReplaceAll(name, `"`, `""`)
	return `"` + escaped + `"`
}

// qualifiedTable returns database.schema.table, each part quoted, omitting
// empty parts.
func (s *SnowflakeOutboundConnector) qualifiedTable() string {
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

// Send writes a single message to the Snowflake table.
func (s *SnowflakeOutboundConnector) Send(ctx context.Context, message *models.OutboundMessage) (*DeliveryResult, error) {
	if !s.BaseOutboundConnector.BaseConnector.initialized {
		return nil, fmt.Errorf("connector not initialized")
	}

	startTime := time.Now()

	var dataMap map[string]interface{}
	if err := json.Unmarshal([]byte(message.Content), &dataMap); err != nil {
		return &DeliveryResult{
			Success:      false,
			MessageID:    message.MessageID,
			Timestamp:    time.Now(),
			ErrorMessage: fmt.Sprintf("failed to parse message content as JSON: %v", err),
			DurationMs:   int64(time.Since(startTime).Milliseconds()),
		}, err
	}

	query, values := s.buildWriteQuery(dataMap)
	log.Printf("❄️  Snowflake Outbound: Executing: %s", query)

	result, err := s.db.ExecContext(ctx, query, values...)
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

// SendBatch writes multiple messages. Same independent-per-message approach as
// databricks_outbound.go: no explicit multi-statement transaction wrapping,
// since the driver's transaction support against a real Snowflake session is
// not confirmed in this environment (see file header on the no-live-account
// caveat) — one message failing must not risk an unverified BeginTx/Commit
// path silently failing the whole batch. batch_size is accepted in config for
// forward compatibility but not yet used to chunk a single bulk statement.
func (s *SnowflakeOutboundConnector) SendBatch(ctx context.Context, messages []*models.OutboundMessage) ([]*DeliveryResult, error) {
	if !s.BaseOutboundConnector.BaseConnector.initialized {
		return nil, fmt.Errorf("connector not initialized")
	}

	startTime := time.Now()
	results := make([]*DeliveryResult, 0, len(messages))
	successCount, failureCount := 0, 0

	for _, message := range messages {
		result, err := s.Send(ctx, message)
		if err != nil && result == nil {
			result = &DeliveryResult{
				Success:      false,
				MessageID:    message.MessageID,
				Timestamp:    time.Now(),
				ErrorMessage: err.Error(),
			}
		}
		results = append(results, result)
		if result.Success {
			successCount++
		} else {
			failureCount++
		}
	}

	log.Printf("✅ Snowflake Outbound: Batch complete — success: %d, failed: %d, elapsed: %v",
		successCount, failureCount, time.Since(startTime))
	return results, nil
}

// buildWriteQuery builds either INSERT or a MERGE INTO statement depending on
// write_mode — same shape as databricks_outbound.go's buildWriteQuery, adapted
// to Snowflake's "?" positional placeholders (confirmed supported by the
// official driver) and double-quote identifier quoting.
func (s *SnowflakeOutboundConnector) buildWriteQuery(dataMap map[string]interface{}) (string, []interface{}) {
	columns := make([]string, 0, len(dataMap))
	values := make([]interface{}, 0, len(dataMap))
	for k, v := range dataMap {
		columns = append(columns, k)
		values = append(values, v)
	}

	switch s.config.WriteMode {
	case "upsert", "update":
		if s.config.UniqueKey != "" {
			return s.buildMergeQuery(columns, values)
		}
		// Fallback to INSERT when no unique key configured.
	}

	quotedCols := make([]string, len(columns))
	placeholders := make([]string, len(columns))
	for i, col := range columns {
		quotedCols[i] = quoteIdentSnowflake(col)
		placeholders[i] = "?"
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		s.qualifiedTable(),
		strings.Join(quotedCols, ", "),
		strings.Join(placeholders, ", "),
	)
	return query, values
}

// buildMergeQuery builds a standard ANSI MERGE INTO ... USING ... statement for
// upsert. uniqueKey may be a comma-separated list of columns (same convention
// as the other DB outbound connectors' UniqueKey field), all of which must
// match for a row to be considered "matched".
func (s *SnowflakeOutboundConnector) buildMergeQuery(columns []string, values []interface{}) (string, []interface{}) {
	sourceSelects := make([]string, len(columns))
	for i, col := range columns {
		sourceSelects[i] = fmt.Sprintf("? AS %s", quoteIdentSnowflake(col))
	}

	keyCols := make(map[string]bool)
	onClauses := make([]string, 0)
	for _, k := range strings.Split(s.config.UniqueKey, ",") {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		keyCols[k] = true
		onClauses = append(onClauses, fmt.Sprintf("target.%s = source.%s", quoteIdentSnowflake(k), quoteIdentSnowflake(k)))
	}

	updateClauses := make([]string, 0, len(columns))
	for _, col := range columns {
		if !keyCols[col] {
			q := quoteIdentSnowflake(col)
			updateClauses = append(updateClauses, fmt.Sprintf("target.%s = source.%s", q, q))
		}
	}

	insertCols := make([]string, len(columns))
	insertSrc := make([]string, len(columns))
	for i, col := range columns {
		insertCols[i] = quoteIdentSnowflake(col)
		insertSrc[i] = "source." + quoteIdentSnowflake(col)
	}

	query := fmt.Sprintf(
		"MERGE INTO %s AS target USING (SELECT %s) AS source ON %s "+
			"WHEN MATCHED THEN UPDATE SET %s "+
			"WHEN NOT MATCHED THEN INSERT (%s) VALUES (%s)",
		s.qualifiedTable(),
		strings.Join(sourceSelects, ", "),
		strings.Join(onClauses, " AND "),
		strings.Join(updateClauses, ", "),
		strings.Join(insertCols, ", "),
		strings.Join(insertSrc, ", "),
	)
	return query, values
}

// SupportsBatch returns true.
func (s *SnowflakeOutboundConnector) SupportsBatch() bool { return true }

// TestConnection pings the Snowflake account.
func (s *SnowflakeOutboundConnector) TestConnection(ctx context.Context) error {
	if s.db == nil {
		return fmt.Errorf("not initialized")
	}
	return s.db.PingContext(ctx)
}

// Validate checks required configuration fields.
func (s *SnowflakeOutboundConnector) Validate() error {
	if !s.BaseOutboundConnector.BaseConnector.initialized {
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
	if s.config.TableName == "" {
		return fmt.Errorf("table_name is required")
	}
	if (s.config.WriteMode == "upsert" || s.config.WriteMode == "update") && s.config.UniqueKey == "" {
		return fmt.Errorf("unique_key is required when write_mode is upsert or update")
	}
	return nil
}

// Close shuts down the connector and releases the DB connection.
func (s *SnowflakeOutboundConnector) Close() error {
	log.Printf("❄️  Snowflake Outbound: Closing")
	if s.db != nil {
		if err := s.db.Close(); err != nil {
			return fmt.Errorf("failed to close Snowflake connection: %w", err)
		}
		s.db = nil
	}
	log.Printf("✅ Snowflake Outbound: Closed")
	return nil
}

// GetStatus returns connector status with Snowflake-specific metadata.
func (s *SnowflakeOutboundConnector) GetStatus() ConnectorStatus {
	status := s.BaseOutboundConnector.GetStatus()
	if status.Metadata == nil {
		status.Metadata = map[string]string{}
	}
	status.Metadata["account"] = s.config.Account
	status.Metadata["database"] = s.config.Database
	status.Metadata["schema"] = s.config.Schema
	status.Metadata["warehouse"] = s.config.Warehouse
	status.Metadata["table"] = s.config.TableName
	status.Metadata["write_mode"] = s.config.WriteMode
	return status
}
