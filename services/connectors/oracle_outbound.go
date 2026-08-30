// services/connectors/oracle_outbound.go
// Oracle Outbound Connector — writes messages to an Oracle table using
// INSERT or MERGE INTO (upsert).
//
// Write modes:
//   - "insert"  — INSERT INTO table (col, ...) VALUES (:1, :2, ...)
//   - "upsert"  — MERGE INTO table t USING (SELECT :1 c1, :2 c2, ... FROM DUAL)
//                   s ON (t.key = s.key)
//                   WHEN MATCHED THEN UPDATE ...
//                   WHEN NOT MATCHED THEN INSERT ...
//   - "update"  — alias for upsert
//
// Oracle uses :N positional bind variables (go-ora/v2) and no quoting for
// standard identifiers (they are case-insensitive unless double-quoted).
//
// Configuration (DatabaseOutboundConfig fields):
//
//	host               string  Oracle host (default: localhost)
//	port               int     Oracle port (default: 1521)
//	database           string  Service name / SID (required)
//	username           string  Oracle user (required)
//	password           string  Oracle password
//	table_name         string  Destination table (required)
//	write_mode         string  "insert" | "upsert" | "update" (default: insert)
//	unique_key         string  Merge key column for upsert/update
//	batch_size         int     Records per batch transaction (default: 50)
//	ssl_mode           string  "disable" | "require"
//	max_open_conns     int     Connection pool max open (default: 10)
//	max_idle_conns     int     Connection pool max idle (default: 5)

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

	go_ora "github.com/sijms/go-ora/v2"
)

// OracleOutboundConnector writes messages to an Oracle table.
type OracleOutboundConnector struct {
	*BaseOutboundConnector
	config DatabaseOutboundConfig
	db     *sql.DB
}

// NewOracleOutboundConnector creates a production Oracle outbound connector.
func NewOracleOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "oracle_outbound",
		DisplayName:        "Oracle Database Writer",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch":        true,
			"supports_ssl":          true,
			"supports_merge":        true,
			"supports_pool":         true,
			"supports_windows_auth": true, // go-ora's native AUTH TYPE=OS — see buildWindowsAuthDSN
		},
	}
	return &OracleOutboundConnector{
		BaseOutboundConnector: NewBaseOutboundConnector(metadata, true),
	}
}

// Initialize parses config, opens the Oracle connection, and pings to verify.
func (o *OracleOutboundConnector) Initialize(config []byte) error {
	log.Printf("🏛️  Oracle Outbound: Initializing")

	parsedConfig, err := ParseDatabaseOutboundConfig(config)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}
	o.config = *parsedConfig

	db, err := sql.Open("oracle", o.buildDSN())
	if err != nil {
		return fmt.Errorf("failed to open Oracle connection: %w", err)
	}

	if o.config.MaxOpenConns > 0 {
		db.SetMaxOpenConns(o.config.MaxOpenConns)
	}
	if o.config.MaxIdleConns > 0 {
		db.SetMaxIdleConns(o.config.MaxIdleConns)
	}
	db.SetConnMaxLifetime(time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("failed to ping Oracle at %s:%d: %w", o.config.Host, o.config.Port, err)
	}

	o.db = db
	o.BaseOutboundConnector.BaseConnector.initialized = true

	log.Printf("✅ Oracle Outbound: Initialized (host=%s, service=%s, table=%s, mode=%s)",
		o.config.Host, o.config.Database, o.config.TableName, o.config.WriteMode)
	return nil
}

// buildDSN constructs the go-ora connection URL.
//
//	oracle://user:pass@host:port/service_name
//
// When auth_type is "windows", delegates to buildWindowsAuthDSN instead —
// see oracle_inbound.go's buildWindowsAuthDSN for the full rationale and the
// same "unverified against a real domain" caveat (identical mechanism,
// duplicated here since outbound/inbound don't share a base DSN builder).
func (o *OracleOutboundConnector) buildDSN() string {
	host := o.config.Host
	if host == "" {
		host = "localhost"
	}
	port := o.config.Port
	if port == 0 {
		port = 1521
	}

	if o.config.AuthType == "windows" {
		return o.buildWindowsAuthDSN(host, port)
	}

	dsn := fmt.Sprintf("oracle://%s:%s@%s:%d/%s",
		o.config.Username, o.config.Password, host, port, o.config.Database)

	if o.config.SSLMode == "require" {
		dsn += "?ssl=true&ssl verify=false"
	}

	return dsn
}

// buildWindowsAuthDSN builds a DSN using go-ora's native OS-authentication
// support (AUTH TYPE=OS via NTLM/NTS) — see oracle_inbound.go's
// buildWindowsAuthDSN for the full doc comment; kept in sync with it.
func (o *OracleOutboundConnector) buildWindowsAuthDSN(host string, port int) string {
	options := map[string]string{
		"AUTH TYPE": "OS",
		"AUTH SERV": "NTS",
	}
	if o.config.OSUser != "" {
		options["OS USER"] = o.config.OSUser
	}
	if o.config.OSPassword != "" {
		options["OS PASS"] = o.config.OSPassword
	}
	if o.config.Domain != "" {
		options["DOMAIN"] = o.config.Domain
	}
	if o.config.SSLMode == "require" {
		options["SSL"] = "true"
		options["SSL VERIFY"] = "false"
	}
	return go_ora.BuildUrl(host, port, o.config.Database, "", "", options)
}

// Send writes a single message to Oracle.
func (o *OracleOutboundConnector) Send(ctx context.Context, message *models.OutboundMessage) (*DeliveryResult, error) {
	if !o.BaseOutboundConnector.BaseConnector.initialized {
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

	query, values := o.buildWriteQuery(dataMap)
	log.Printf("🏛️  Oracle Outbound: Executing: %s", query)

	result, err := o.db.ExecContext(ctx, query, values...)
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

// SendBatch writes multiple messages inside a single transaction.
func (o *OracleOutboundConnector) SendBatch(ctx context.Context, messages []*models.OutboundMessage) ([]*DeliveryResult, error) {
	if !o.BaseOutboundConnector.BaseConnector.initialized {
		return nil, fmt.Errorf("connector not initialized")
	}

	startTime := time.Now()
	results := make([]*DeliveryResult, 0, len(messages))

	tx, err := o.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	successCount, failureCount := 0, 0

	for _, message := range messages {
		msgStart := time.Now()

		var dataMap map[string]interface{}
		if err := json.Unmarshal([]byte(message.Content), &dataMap); err != nil {
			results = append(results, &DeliveryResult{
				Success:      false,
				MessageID:    message.MessageID,
				Timestamp:    time.Now(),
				ErrorMessage: fmt.Sprintf("failed to parse message: %v", err),
				DurationMs:   int64(time.Since(msgStart).Milliseconds()),
			})
			failureCount++
			continue
		}

		query, values := o.buildWriteQuery(dataMap)
		result, err := tx.ExecContext(ctx, query, values...)
		if err != nil {
			results = append(results, &DeliveryResult{
				Success:      false,
				MessageID:    message.MessageID,
				Timestamp:    time.Now(),
				ErrorMessage: fmt.Sprintf("query failed: %v", err),
				DurationMs:   int64(time.Since(msgStart).Milliseconds()),
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
			DurationMs:     int64(time.Since(msgStart).Milliseconds()),
		})
		successCount++
	}

	if err := tx.Commit(); err != nil {
		tx.Rollback()
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	log.Printf("✅ Oracle Outbound: Batch complete — success: %d, failed: %d, elapsed: %v",
		successCount, failureCount, time.Since(startTime))
	return results, nil
}

// buildWriteQuery builds either INSERT or MERGE INTO depending on write_mode.
//
// MERGE INTO syntax (Oracle):
//
//	MERGE INTO table t
//	USING (SELECT :1 AS col1, :2 AS col2, ... FROM DUAL) s
//	ON (t.key = s.key)
//	WHEN MATCHED THEN UPDATE SET t.col = s.col, ...
//	WHEN NOT MATCHED THEN INSERT (col, ...) VALUES (s.col, ...)
func (o *OracleOutboundConnector) buildWriteQuery(dataMap map[string]interface{}) (string, []interface{}) {
	columns := make([]string, 0, len(dataMap))
	values := make([]interface{}, 0, len(dataMap))

	for k, v := range dataMap {
		columns = append(columns, k)
		values = append(values, v)
	}

	switch o.config.WriteMode {
	case "upsert", "update":
		if o.config.UniqueKey != "" {
			return o.buildMergeQuery(columns, values)
		}
	}

	// Default: INSERT — Oracle uses :N positional params
	placeholders := make([]string, len(columns))
	for i := range columns {
		placeholders[i] = fmt.Sprintf(":%d", i+1)
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		o.config.TableName,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "),
	)
	return query, values
}

// buildMergeQuery builds MERGE INTO ... USING (SELECT ... FROM DUAL) for Oracle upsert.
func (o *OracleOutboundConnector) buildMergeQuery(columns []string, values []interface{}) (string, []interface{}) {
	// Build source SELECT: SELECT :1 AS col1, :2 AS col2 FROM DUAL
	sourceSelects := make([]string, len(columns))
	for i, col := range columns {
		sourceSelects[i] = fmt.Sprintf(":%d AS %s", i+1, col)
	}

	// UPDATE clauses: t.col = s.col (skip the key column)
	updateClauses := make([]string, 0, len(columns))
	for _, col := range columns {
		if col != o.config.UniqueKey {
			updateClauses = append(updateClauses, fmt.Sprintf("t.%s = s.%s", col, col))
		}
	}

	// INSERT columns and source values
	insertSrc := make([]string, len(columns))
	for i, col := range columns {
		insertSrc[i] = "s." + col
	}

	query := fmt.Sprintf(
		"MERGE INTO %s t USING (SELECT %s FROM DUAL) s ON (t.%s = s.%s) "+
			"WHEN MATCHED THEN UPDATE SET %s "+
			"WHEN NOT MATCHED THEN INSERT (%s) VALUES (%s)",
		o.config.TableName,
		strings.Join(sourceSelects, ", "),
		o.config.UniqueKey, o.config.UniqueKey,
		strings.Join(updateClauses, ", "),
		strings.Join(columns, ", "),
		strings.Join(insertSrc, ", "),
	)
	return query, values
}

// SupportsBatch returns true — Oracle outbound supports batching.
func (o *OracleOutboundConnector) SupportsBatch() bool { return true }

// TestConnection pings the Oracle database with a context deadline.
func (o *OracleOutboundConnector) TestConnection(ctx context.Context) error {
	if o.db == nil {
		return fmt.Errorf("not initialized")
	}
	return o.db.PingContext(ctx)
}

// Validate checks required configuration fields.
func (o *OracleOutboundConnector) Validate() error {
	if !o.BaseOutboundConnector.BaseConnector.initialized {
		return fmt.Errorf("connector not initialized")
	}
	if o.config.TableName == "" {
		return fmt.Errorf("table_name is required")
	}
	if o.config.Database == "" {
		return fmt.Errorf("database (service name) is required")
	}
	if (o.config.WriteMode == "upsert" || o.config.WriteMode == "update") && o.config.UniqueKey == "" {
		return fmt.Errorf("unique_key is required when write_mode is upsert or update")
	}
	if o.config.AuthType == "windows" && o.config.OSPassword == "" {
		return fmt.Errorf("os_password is required when auth_type is 'windows'")
	}
	return nil
}

// Close shuts down the connector and releases the DB connection.
func (o *OracleOutboundConnector) Close() error {
	log.Printf("🏛️  Oracle Outbound: Closing")
	if o.db != nil {
		if err := o.db.Close(); err != nil {
			return fmt.Errorf("failed to close Oracle connection: %w", err)
		}
		o.db = nil
	}
	log.Printf("✅ Oracle Outbound: Closed")
	return nil
}

// GetStatus returns connector status with Oracle-specific metadata.
func (o *OracleOutboundConnector) GetStatus() ConnectorStatus {
	status := o.BaseOutboundConnector.GetStatus()
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
	status.Metadata["write_mode"] = o.config.WriteMode
	return status
}
