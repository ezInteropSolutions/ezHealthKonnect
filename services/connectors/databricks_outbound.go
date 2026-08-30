// services/connectors/databricks_outbound.go
// Databricks Outbound Connector — writes messages to a Delta Lake table via a
// Databricks SQL Warehouse, using the official Databricks SQL Go driver
// (github.com/databricks/databricks-sql-go — a real database/sql driver, not
// a hand-rolled REST client) so this reuses the exact same Exec/ExecContext
// idioms as postgresql_outbound.go/mysql_outbound.go/sqlserver_outbound.go.
//
// NOTE — unlike the other DB outbound connectors added this session, there is
// no way to verify this against a real Databricks workspace in this
// environment (cloud-only service, no local emulator, no test credentials
// available). This has been verified via unit tests of the pure logic (DSN
// building, MERGE query construction, config validation) and a clean compile
// — NOT against a live Databricks SQL Warehouse. Treat the actual
// connectivity as unverified until tried against a real workspace.
//
// Write modes:
//   - "insert"  — plain INSERT INTO `catalog`.`schema`.`table` (...) VALUES (...)
//   - "upsert"  — MERGE INTO ... USING ... ON ... WHEN MATCHED / NOT MATCHED (Delta Lake MERGE)
//   - "update"  — alias for upsert
//
// Configuration (DatabaseOutboundConfig fields — reusing the existing shared
// struct's fields already provisioned for cloud warehouses, not new ones):
//
//	host               string  Databricks workspace server hostname (required)
//	port               int     Port (default: 443)
//	token              string  Personal access token (required)
//	http_path          string  SQL warehouse HTTP path, e.g. /sql/1.0/warehouses/xxxx (required)
//	database           string  Unity Catalog catalog name
//	schema             string  Schema name within the catalog
//	table_name         string  Destination table (required)
//	write_mode         string  "insert" | "upsert" | "update" (default: insert)
//	unique_key         string  Merge key column(s) for upsert/update, comma-separated (required when write_mode != insert)
//	batch_size         int     Records per batch transaction (default: 50)
package connectors

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"ezhealthkonnect/models"

	_ "github.com/databricks/databricks-sql-go" // Registers "databricks" driver
)

// DatabricksOutboundConnector writes messages to a Delta Lake table via a
// Databricks SQL Warehouse.
type DatabricksOutboundConnector struct {
	*BaseOutboundConnector
	config DatabaseOutboundConfig
	db     *sql.DB
}

// NewDatabricksOutboundConnector creates a Databricks outbound connector using
// the official Databricks SQL Go driver.
func NewDatabricksOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "databricks_outbound",
		DisplayName:        "Databricks SQL Warehouse Writer",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch":      true,
			"supports_pat_auth":   true,
			"supports_merge":      true,
			"supports_pool":       true,
			"supports_delta_lake": true,
			"supports_unity_cat":  true,
			// supports_oauth deliberately omitted — only personal access token
			// auth is implemented; claiming OAuth support without building the
			// client-credentials flow would repeat the false-capability problem
			// found in several stubs earlier this session.
		},
	}
	return &DatabricksOutboundConnector{
		BaseOutboundConnector: NewBaseOutboundConnector(metadata, true),
	}
}

// Initialize parses config, opens the Databricks SQL Warehouse connection,
// and pings to verify.
func (d *DatabricksOutboundConnector) Initialize(config []byte) error {
	log.Printf("🧱 Databricks Outbound: Initializing")

	parsedConfig, err := ParseDatabaseOutboundConfig(config)
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
	d.BaseOutboundConnector.BaseConnector.initialized = true

	log.Printf("✅ Databricks Outbound: Initialized (host=%s, catalog=%s, schema=%s, table=%s, mode=%s)",
		d.config.Host, d.config.Database, d.config.Schema, d.config.TableName, d.config.WriteMode)
	return nil
}

// buildDSN constructs the driver's DSN: "token:<pat>@<host>:<port>/<http_path>?catalog=X&schema=Y"
// — confirmed format from the official driver's own documentation.
func (d *DatabricksOutboundConnector) buildDSN() string {
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

// quoteIdent quotes an identifier using Databricks SQL's backtick convention
// (same as MySQL/BigQuery — Databricks SQL, built on Spark SQL/Delta Lake,
// uses backticks to quote identifiers containing special characters or
// reserved words). Unverified against a live workspace — see file header.
func quoteIdent(name string) string {
	return "`" + name + "`"
}

// qualifiedTable returns catalog.schema.table (Unity Catalog three-level
// namespace), each part quoted, omitting empty parts.
func (d *DatabricksOutboundConnector) qualifiedTable() string {
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

// Send writes a single message to the Delta Lake table.
func (d *DatabricksOutboundConnector) Send(ctx context.Context, message *models.OutboundMessage) (*DeliveryResult, error) {
	if !d.BaseOutboundConnector.BaseConnector.initialized {
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

	query, values := d.buildWriteQuery(dataMap)
	log.Printf("🧱 Databricks Outbound: Executing: %s", query)

	result, err := d.db.ExecContext(ctx, query, values...)
	if err != nil {
		return &DeliveryResult{
			Success:      false,
			MessageID:    message.MessageID,
			Timestamp:    time.Now(),
			ErrorMessage: fmt.Sprintf("query execution failed: %v", err),
			DurationMs:   int64(time.Since(startTime).Milliseconds()),
		}, err
	}

	// RowsAffected may be unsupported by some Databricks driver versions for
	// MERGE statements — treat that as informational only, not a failure.
	rowsAffected, _ := result.RowsAffected()

	return &DeliveryResult{
		Success:        true,
		MessageID:      message.MessageID,
		Timestamp:      time.Now(),
		Acknowledgment: fmt.Sprintf("Rows affected: %d", rowsAffected),
		DurationMs:     int64(time.Since(startTime).Milliseconds()),
	}, nil
}

// SendBatch writes multiple messages. Note: unlike the pq/mssql/mysql
// drivers, no explicit multi-statement transaction wrapping is used here —
// Databricks SQL Warehouses commonly run each statement as its own implicit
// transaction over the REST-based Thrift protocol, and the driver's
// transaction support against a SQL Warehouse endpoint is not confirmed in
// this environment (see file header on the no-live-workspace caveat) — each
// message is executed independently so a single failure doesn't risk an
// unverified BeginTx/Commit path silently failing the whole batch.
func (d *DatabricksOutboundConnector) SendBatch(ctx context.Context, messages []*models.OutboundMessage) ([]*DeliveryResult, error) {
	if !d.BaseOutboundConnector.BaseConnector.initialized {
		return nil, fmt.Errorf("connector not initialized")
	}

	startTime := time.Now()
	results := make([]*DeliveryResult, 0, len(messages))
	successCount, failureCount := 0, 0

	for _, message := range messages {
		result, err := d.Send(ctx, message)
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

	log.Printf("✅ Databricks Outbound: Batch complete — success: %d, failed: %d, elapsed: %v",
		successCount, failureCount, time.Since(startTime))
	return results, nil
}

// buildWriteQuery builds either INSERT or a Delta Lake MERGE INTO statement
// depending on write_mode — same shape as sqlserver_outbound.go's
// buildWriteQuery/buildMergeQuery, adapted to Databricks SQL's "?" positional
// placeholders (confirmed supported by the official driver) and backtick
// identifier quoting.
func (d *DatabricksOutboundConnector) buildWriteQuery(dataMap map[string]interface{}) (string, []interface{}) {
	columns := make([]string, 0, len(dataMap))
	values := make([]interface{}, 0, len(dataMap))
	for k, v := range dataMap {
		columns = append(columns, k)
		values = append(values, v)
	}

	switch d.config.WriteMode {
	case "upsert", "update":
		if d.config.UniqueKey != "" {
			return d.buildMergeQuery(columns, values)
		}
		// Fallback to INSERT when no unique key configured.
	}

	quotedCols := make([]string, len(columns))
	placeholders := make([]string, len(columns))
	for i, col := range columns {
		quotedCols[i] = quoteIdent(col)
		placeholders[i] = "?"
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		d.qualifiedTable(),
		strings.Join(quotedCols, ", "),
		strings.Join(placeholders, ", "),
	)
	return query, values
}

// buildMergeQuery builds a MERGE INTO ... USING ... statement for upsert.
// uniqueKey may be a comma-separated list of columns (same convention as the
// other DB outbound connectors' UniqueKey field), all of which must match for
// a row to be considered "matched".
func (d *DatabricksOutboundConnector) buildMergeQuery(columns []string, values []interface{}) (string, []interface{}) {
	sourceSelects := make([]string, len(columns))
	for i, col := range columns {
		sourceSelects[i] = fmt.Sprintf("? AS %s", quoteIdent(col))
	}

	keyCols := make(map[string]bool)
	onClauses := make([]string, 0)
	for _, k := range strings.Split(d.config.UniqueKey, ",") {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		keyCols[k] = true
		onClauses = append(onClauses, fmt.Sprintf("target.%s = source.%s", quoteIdent(k), quoteIdent(k)))
	}

	updateClauses := make([]string, 0, len(columns))
	for _, col := range columns {
		if !keyCols[col] {
			q := quoteIdent(col)
			updateClauses = append(updateClauses, fmt.Sprintf("target.%s = source.%s", q, q))
		}
	}

	insertCols := make([]string, len(columns))
	insertSrc := make([]string, len(columns))
	for i, col := range columns {
		insertCols[i] = quoteIdent(col)
		insertSrc[i] = "source." + quoteIdent(col)
	}

	query := fmt.Sprintf(
		"MERGE INTO %s AS target USING (SELECT %s) AS source ON %s "+
			"WHEN MATCHED THEN UPDATE SET %s "+
			"WHEN NOT MATCHED THEN INSERT (%s) VALUES (%s)",
		d.qualifiedTable(),
		strings.Join(sourceSelects, ", "),
		strings.Join(onClauses, " AND "),
		strings.Join(updateClauses, ", "),
		strings.Join(insertCols, ", "),
		strings.Join(insertSrc, ", "),
	)
	return query, values
}

// SupportsBatch returns true.
func (d *DatabricksOutboundConnector) SupportsBatch() bool { return true }

// TestConnection pings the Databricks SQL Warehouse with a generous deadline
// — SQL Warehouses can take longer to respond than a traditional DB if the
// warehouse is currently stopped/starting up.
func (d *DatabricksOutboundConnector) TestConnection(ctx context.Context) error {
	if d.db == nil {
		return fmt.Errorf("not initialized")
	}
	return d.db.PingContext(ctx)
}

// Validate checks required configuration fields.
func (d *DatabricksOutboundConnector) Validate() error {
	if !d.BaseOutboundConnector.BaseConnector.initialized {
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
	if d.config.TableName == "" {
		return fmt.Errorf("table_name is required")
	}
	if (d.config.WriteMode == "upsert" || d.config.WriteMode == "update") && d.config.UniqueKey == "" {
		return fmt.Errorf("unique_key is required when write_mode is upsert or update")
	}
	return nil
}

// Close shuts down the connector and releases the DB connection.
func (d *DatabricksOutboundConnector) Close() error {
	log.Printf("🧱 Databricks Outbound: Closing")
	if d.db != nil {
		if err := d.db.Close(); err != nil {
			return fmt.Errorf("failed to close Databricks connection: %w", err)
		}
		d.db = nil
	}
	log.Printf("✅ Databricks Outbound: Closed")
	return nil
}

// GetStatus returns connector status with Databricks-specific metadata.
func (d *DatabricksOutboundConnector) GetStatus() ConnectorStatus {
	status := d.BaseOutboundConnector.GetStatus()
	if status.Metadata == nil {
		status.Metadata = map[string]string{}
	}
	status.Metadata["host"] = d.config.Host
	status.Metadata["catalog"] = d.config.Database
	status.Metadata["schema"] = d.config.Schema
	status.Metadata["table"] = d.config.TableName
	status.Metadata["write_mode"] = d.config.WriteMode
	return status
}
