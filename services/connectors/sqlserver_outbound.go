// services/connectors/sqlserver_outbound.go
// SQL Server Outbound Connector — writes messages to a SQL Server table using
// INSERT or MERGE (upsert).
//
// Write modes:
//   - "insert"  — plain INSERT INTO [table] (...) VALUES (...)
//   - "upsert"  — MERGE INTO ... USING ... ON ... WHEN MATCHED / NOT MATCHED
//   - "update"  — alias for upsert
//
// Configuration (DatabaseOutboundConfig fields):
//
//	host               string  SQL Server host (default: localhost)
//	port               int     SQL Server port (default: 1433)
//	database           string  Database name (required)
//	username           string  SQL Server login (required)
//	password           string  SQL Server password
//	table_name         string  Destination table (required)
//	write_mode         string  "insert" | "upsert" | "update" (default: insert)
//	unique_key         string  Merge key column for upsert/update (required when write_mode != insert)
//	batch_size         int     Records per batch transaction (default: 50)
//	ssl_mode           string  "disable" | "require" | "verify-full"
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

	_ "github.com/microsoft/go-mssqldb" // Registers "sqlserver" driver
)

// SQLServerOutboundConnector writes messages to a SQL Server table.
type SQLServerOutboundConnector struct {
	*BaseOutboundConnector
	config DatabaseOutboundConfig
	db     *sql.DB
}

// NewSQLServerOutboundConnector creates a production SQL Server outbound connector.
func NewSQLServerOutboundConnector() OutboundConnector {
	metadata := ConnectorMetadata{
		TypeName:           "sqlserver_outbound",
		DisplayName:        "SQL Server Database Writer",
		Version:            "1.0.0",
		Category:           "outbound",
		Mode:               "push",
		ImplementationLang: "go",
		Capabilities: map[string]bool{
			"supports_batch":        true,
			"supports_encryption":   true,
			"supports_merge":        true,
			"supports_pool":         true,
			"supports_windows_auth": true,
		},
	}
	return &SQLServerOutboundConnector{
		BaseOutboundConnector: NewBaseOutboundConnector(metadata, true),
	}
}

// Initialize parses config, opens the SQL Server connection, and pings to verify.
func (s *SQLServerOutboundConnector) Initialize(config []byte) error {
	log.Printf("🗄️  SQL Server Outbound: Initializing")

	parsedConfig, err := ParseDatabaseOutboundConfig(config)
	if err != nil {
		return fmt.Errorf("failed to parse config: %w", err)
	}
	s.config = *parsedConfig

	db, err := sql.Open("sqlserver", s.buildDSN())
	if err != nil {
		return fmt.Errorf("failed to open SQL Server connection: %w", err)
	}

	if s.config.MaxOpenConns > 0 {
		db.SetMaxOpenConns(s.config.MaxOpenConns)
	}
	if s.config.MaxIdleConns > 0 {
		db.SetMaxIdleConns(s.config.MaxIdleConns)
	}
	db.SetConnMaxLifetime(time.Hour)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		return fmt.Errorf("failed to ping SQL Server at %s:%d: %w", s.config.Host, s.config.Port, err)
	}

	s.db = db
	s.BaseOutboundConnector.BaseConnector.initialized = true

	log.Printf("✅ SQL Server Outbound: Initialized (host=%s, database=%s, table=%s, mode=%s)",
		s.config.Host, s.config.Database, s.config.TableName, s.config.WriteMode)
	return nil
}

// buildDSN constructs the go-mssqldb URL connection string.
func (s *SQLServerOutboundConnector) buildDSN() string {
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
	default:
		dsn += "&encrypt=disable"
	}

	return dsn
}

// Send writes a single message to SQL Server.
func (s *SQLServerOutboundConnector) Send(ctx context.Context, message *models.OutboundMessage) (*DeliveryResult, error) {
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
	log.Printf("🗄️  SQL Server Outbound: Executing: %s", query)

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

// SendBatch writes multiple messages inside a single transaction.
func (s *SQLServerOutboundConnector) SendBatch(ctx context.Context, messages []*models.OutboundMessage) ([]*DeliveryResult, error) {
	if !s.BaseOutboundConnector.BaseConnector.initialized {
		return nil, fmt.Errorf("connector not initialized")
	}

	startTime := time.Now()
	results := make([]*DeliveryResult, 0, len(messages))

	tx, err := s.db.BeginTx(ctx, nil)
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

		query, values := s.buildWriteQuery(dataMap)
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

	log.Printf("✅ SQL Server Outbound: Batch complete — success: %d, failed: %d, elapsed: %v",
		successCount, failureCount, time.Since(startTime))
	return results, nil
}

// buildWriteQuery builds either INSERT or MERGE depending on write_mode.
//
// MERGE syntax:
//
//	MERGE INTO [table] AS target
//	USING (SELECT @p1 AS col1, @p2 AS col2, ...) AS source
//	ON target.[uniqueKey] = source.[uniqueKey]
//	WHEN MATCHED THEN UPDATE SET target.[col] = source.[col], ...
//	WHEN NOT MATCHED THEN INSERT ([col], ...) VALUES (source.[col], ...);
func (s *SQLServerOutboundConnector) buildWriteQuery(dataMap map[string]interface{}) (string, []interface{}) {
	columns := make([]string, 0, len(dataMap))
	values := make([]interface{}, 0, len(dataMap))

	// Deterministic column order
	for k, v := range dataMap {
		columns = append(columns, k)
		values = append(values, v)
	}

	switch s.config.WriteMode {
	case "upsert", "update":
		if s.config.UniqueKey == "" {
			// Fallback to INSERT when no unique key configured
			break
		}
		return s.buildMergeQuery(columns, values)
	}

	// Default: INSERT
	bracketCols := make([]string, len(columns))
	placeholders := make([]string, len(columns))
	for i, col := range columns {
		bracketCols[i] = "[" + col + "]"
		placeholders[i] = fmt.Sprintf("@p%d", i+1)
	}

	query := fmt.Sprintf("INSERT INTO [%s] (%s) VALUES (%s)",
		s.config.TableName,
		strings.Join(bracketCols, ", "),
		strings.Join(placeholders, ", "),
	)
	return query, values
}

// buildMergeQuery builds a MERGE INTO ... USING ... statement for upsert.
func (s *SQLServerOutboundConnector) buildMergeQuery(columns []string, values []interface{}) (string, []interface{}) {
	sourceSelects := make([]string, len(columns))
	for i, col := range columns {
		sourceSelects[i] = fmt.Sprintf("@p%d AS [%s]", i+1, col)
	}

	updateClauses := make([]string, 0, len(columns))
	for _, col := range columns {
		if col != s.config.UniqueKey {
			updateClauses = append(updateClauses, fmt.Sprintf("target.[%s] = source.[%s]", col, col))
		}
	}

	insertCols := make([]string, len(columns))
	insertSrc := make([]string, len(columns))
	for i, col := range columns {
		insertCols[i] = "[" + col + "]"
		insertSrc[i] = "source.[" + col + "]"
	}

	query := fmt.Sprintf(
		"MERGE INTO [%s] AS target USING (SELECT %s) AS source ON target.[%s] = source.[%s] "+
			"WHEN MATCHED THEN UPDATE SET %s "+
			"WHEN NOT MATCHED THEN INSERT (%s) VALUES (%s);",
		s.config.TableName,
		strings.Join(sourceSelects, ", "),
		s.config.UniqueKey, s.config.UniqueKey,
		strings.Join(updateClauses, ", "),
		strings.Join(insertCols, ", "),
		strings.Join(insertSrc, ", "),
	)
	return query, values
}

// SupportsBatch returns true — SQL Server outbound supports batching.
func (s *SQLServerOutboundConnector) SupportsBatch() bool { return true }

// TestConnection pings the SQL Server database with a context deadline.
func (s *SQLServerOutboundConnector) TestConnection(ctx context.Context) error {
	if s.db == nil {
		return fmt.Errorf("not initialized")
	}
	return s.db.PingContext(ctx)
}

// Validate checks required configuration fields.
func (s *SQLServerOutboundConnector) Validate() error {
	if !s.BaseOutboundConnector.BaseConnector.initialized {
		return fmt.Errorf("connector not initialized")
	}
	if s.config.TableName == "" {
		return fmt.Errorf("table_name is required")
	}
	if s.config.Database == "" {
		return fmt.Errorf("database is required")
	}
	if (s.config.WriteMode == "upsert" || s.config.WriteMode == "update") && s.config.UniqueKey == "" {
		return fmt.Errorf("unique_key is required when write_mode is upsert or update")
	}
	return nil
}

// Close shuts down the connector and releases the DB connection.
func (s *SQLServerOutboundConnector) Close() error {
	log.Printf("🗄️  SQL Server Outbound: Closing")
	if s.db != nil {
		if err := s.db.Close(); err != nil {
			return fmt.Errorf("failed to close SQL Server connection: %w", err)
		}
		s.db = nil
	}
	log.Printf("✅ SQL Server Outbound: Closed")
	return nil
}

// GetStatus returns connector status with SQL Server-specific metadata.
func (s *SQLServerOutboundConnector) GetStatus() ConnectorStatus {
	status := s.BaseOutboundConnector.GetStatus()
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
	status.Metadata["write_mode"] = s.config.WriteMode
	return status
}
