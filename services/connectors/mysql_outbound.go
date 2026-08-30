// services/connectors/mysql_outbound.go
// MySQL Outbound Connector — writes messages to a MySQL/MariaDB table using
// INSERT or INSERT ... ON DUPLICATE KEY UPDATE (upsert).
//
// Write modes:
//   - "insert"  — plain INSERT INTO `table` (...) VALUES (...)
//   - "upsert"  — INSERT ... ON DUPLICATE KEY UPDATE ... (requires a unique/primary key on unique_key)
//   - "update"  — alias for upsert
//
// ON DUPLICATE KEY UPDATE is used instead of REPLACE INTO on purpose: REPLACE
// INTO deletes and re-inserts the conflicting row, which resets auto-increment
// ids and can fire ON DELETE CASCADE — ON DUPLICATE KEY UPDATE updates in place.
//
// Configuration (DatabaseOutboundConfig fields):
//
//	host               string  MySQL host (default: localhost)
//	port               int     MySQL port (default: 3306)
//	database           string  Database name (required)
//	username           string  MySQL user (required)
//	password           string  MySQL password
//	table_name         string  Destination table (required)
//	write_mode         string  "insert" | "upsert" | "update" (default: insert)
//	unique_key         string  Unique/primary key column for upsert/update (required when write_mode != insert)
//	batch_size         int     Records per batch transaction (default: 50)
//	ssl_mode           string  "disable" | "require" | "verify-ca" | "verify-full"
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

	_ "github.com/go-sql-driver/mysql" // Registers "mysql" driver
)

// MySQLOutboundConnector writes messages to a MySQL/MariaDB table.
type MySQLOutboundConnector struct {
	*BaseOutboundConnector
	config DatabaseOutboundConfig
	db     *sql.DB
}

// NewMySQLOutboundConnector creates a production MySQL outbound connector.
func NewMySQLOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "mysql_outbound",
		DisplayName:        "MySQL Database Writer",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch":  true,
			"supports_tls":    true,
			"supports_upsert": true,
			"supports_pool":   true,
		},
	}
	return &MySQLOutboundConnector{
		BaseOutboundConnector: NewBaseOutboundConnector(metadata, true),
	}
}

// Initialize parses config, opens the MySQL connection, and pings to verify.
func (m *MySQLOutboundConnector) Initialize(config []byte) error {
	log.Printf("🐬 MySQL Outbound: Initializing")

	parsedConfig, err := ParseDatabaseOutboundConfig(config)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}
	m.config = *parsedConfig

	db, err := sql.Open("mysql", m.buildDSN())
	if err != nil {
		return fmt.Errorf("failed to open MySQL connection: %w", err)
	}

	ConfigureConnectionPool(db, m.config.MaxOpenConns, m.config.MaxIdleConns)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("failed to ping MySQL at %s:%d: %w", m.config.Host, m.config.Port, err)
	}

	m.db = db
	m.BaseOutboundConnector.BaseConnector.initialized = true

	log.Printf("✅ MySQL Outbound: Initialized (host=%s, database=%s, table=%s, mode=%s)",
		m.config.Host, m.config.Database, m.config.TableName, m.config.WriteMode)
	return nil
}

// buildDSN creates a MySQL DSN from the parsed config — same format as
// mysql_inbound.go's buildDSN, so both directions behave identically for the
// same connection settings.
func (m *MySQLOutboundConnector) buildDSN() string {
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

// Send writes a single message to MySQL.
func (m *MySQLOutboundConnector) Send(ctx context.Context, message *models.OutboundMessage) (*DeliveryResult, error) {
	if !m.BaseOutboundConnector.BaseConnector.initialized {
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

	query, values := m.buildWriteQuery(dataMap)
	log.Printf("🐬 MySQL Outbound: Executing: %s", query)

	result, err := m.db.ExecContext(ctx, query, values...)
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
func (m *MySQLOutboundConnector) SendBatch(ctx context.Context, messages []*models.OutboundMessage) ([]*DeliveryResult, error) {
	if !m.BaseOutboundConnector.BaseConnector.initialized {
		return nil, fmt.Errorf("connector not initialized")
	}

	startTime := time.Now()
	results := make([]*DeliveryResult, 0, len(messages))

	tx, err := m.db.BeginTx(ctx, nil)
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

		query, values := m.buildWriteQuery(dataMap)
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

	log.Printf("✅ MySQL Outbound: Batch complete — success: %d, failed: %d, elapsed: %v",
		successCount, failureCount, time.Since(startTime))
	return results, nil
}

// buildWriteQuery builds either INSERT or INSERT ... ON DUPLICATE KEY UPDATE
// depending on write_mode. Uses MySQLDialect (database_base.go) for
// placeholder ("?") and identifier quoting (backticks) instead of hardcoding
// them, since this codebase already defines that dialect specifically for MySQL.
func (m *MySQLOutboundConnector) buildWriteQuery(dataMap map[string]interface{}) (string, []interface{}) {
	columns := make([]string, 0, len(dataMap))
	values := make([]interface{}, 0, len(dataMap))

	// Deterministic column order
	for k, v := range dataMap {
		columns = append(columns, k)
		values = append(values, v)
	}

	quotedCols := make([]string, len(columns))
	placeholders := make([]string, len(columns))
	for i, col := range columns {
		quotedCols[i] = MySQLDialect.QuoteIdent(col)
		placeholders[i] = MySQLDialect.Placeholder(i + 1)
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		MySQLDialect.QuoteIdent(m.config.TableName),
		strings.Join(quotedCols, ", "),
		strings.Join(placeholders, ", "),
	)

	if (m.config.WriteMode == "upsert" || m.config.WriteMode == "update") && m.config.UniqueKey != "" {
		updateClauses := make([]string, 0, len(columns))
		for _, col := range columns {
			if col != m.config.UniqueKey {
				quoted := MySQLDialect.QuoteIdent(col)
				updateClauses = append(updateClauses, fmt.Sprintf("%s = VALUES(%s)", quoted, quoted))
			}
		}
		if len(updateClauses) > 0 {
			query += " ON DUPLICATE KEY UPDATE " + strings.Join(updateClauses, ", ")
		}
	}

	return query, values
}

// SupportsBatch returns true — MySQL outbound supports batching.
func (m *MySQLOutboundConnector) SupportsBatch() bool { return true }

// TestConnection pings the MySQL database with a context deadline.
func (m *MySQLOutboundConnector) TestConnection(ctx context.Context) error {
	if m.db == nil {
		return fmt.Errorf("not initialized")
	}
	return m.db.PingContext(ctx)
}

// Validate checks required configuration fields.
func (m *MySQLOutboundConnector) Validate() error {
	if !m.BaseOutboundConnector.BaseConnector.initialized {
		return fmt.Errorf("connector not initialized")
	}
	if m.config.TableName == "" {
		return fmt.Errorf("table_name is required")
	}
	if m.config.Database == "" {
		return fmt.Errorf("database is required")
	}
	if (m.config.WriteMode == "upsert" || m.config.WriteMode == "update") && m.config.UniqueKey == "" {
		return fmt.Errorf("unique_key is required when write_mode is upsert or update")
	}
	return nil
}

// Close shuts down the connector and releases the DB connection.
func (m *MySQLOutboundConnector) Close() error {
	log.Printf("🐬 MySQL Outbound: Closing")
	if m.db != nil {
		if err := m.db.Close(); err != nil {
			return fmt.Errorf("failed to close MySQL connection: %w", err)
		}
		m.db = nil
	}
	log.Printf("✅ MySQL Outbound: Closed")
	return nil
}

// GetStatus returns connector status with MySQL-specific metadata.
func (m *MySQLOutboundConnector) GetStatus() ConnectorStatus {
	status := m.BaseOutboundConnector.GetStatus()
	if m.db != nil {
		if err := m.db.Ping(); err == nil {
			status.Connected = true
		} else {
			status.Connected = false
			status.LastError = err.Error()
		}
	}
	if status.Metadata == nil {
		status.Metadata = map[string]string{}
	}
	status.Metadata["database"] = m.config.Database
	status.Metadata["host"] = m.config.Host
	status.Metadata["table"] = m.config.TableName
	status.Metadata["write_mode"] = m.config.WriteMode
	return status
}
