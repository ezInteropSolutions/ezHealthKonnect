// internal/connectivity/database_connector.go
// Database connectivity handlers for SQL and NoSQL databases

package connectivity

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"ezhealthkonnect/processing/pkg"
	"github.com/google/uuid"
	_ "github.com/lib/pq" // PostgreSQL driver
)

// DatabaseInputConnector handles database message polling
type DatabaseInputConnector struct {
	*BaseConnector
	db          *sql.DB
	messageChan chan<- *pkg.UniversalMessage
	pollInterval time.Duration
	query       string
	stopChan    chan bool
	processor   *DatabaseMessageProcessor
}

// DatabaseOutputConnector handles database message insertion
type DatabaseOutputConnector struct {
	*BaseConnector
	db        *sql.DB
	tableName string
	insertSQL string
	processor *DatabaseMessageProcessor
}

// DatabaseMessageProcessor handles database-specific message processing
type DatabaseMessageProcessor struct {
	dbType     string // postgresql, mysql, mssql, mongodb
	mapping    *DatabaseMapping
	queryCache map[string]*sql.Stmt
	cacheMutex sync.RWMutex
}

// DatabaseMapping defines how messages map to database schema
type DatabaseMapping struct {
	Table         string                    `json:"table"`
	IDColumn      string                    `json:"idColumn"`
	ContentColumn string                    `json:"contentColumn"`
	TypeColumn    string                    `json:"typeColumn"`
	StatusColumn  string                    `json:"statusColumn"`
	CreatedColumn string                    `json:"createdColumn"`
	UpdatedColumn string                    `json:"updatedColumn"`
	CustomColumns map[string]string         `json:"customColumns"`
	Filters       map[string]interface{}    `json:"filters"`
}

// DatabaseRow represents a database row for message processing
type DatabaseRow struct {
	ID          string                 `json:"id"`
	Content     string                 `json:"content"`
	ContentType string                 `json:"content_type"`
	Status      string                 `json:"status"`
	CreatedAt   time.Time              `json:"created_at"`
	UpdatedAt   *time.Time             `json:"updated_at"`
	Metadata    map[string]interface{} `json:"metadata"`
}

// NewDatabaseInputConnector creates a new database input connector
func NewDatabaseInputConnector(config pkg.ConnectorConfig) (*DatabaseInputConnector, error) {
	base := NewBaseConnector(config, "database_input")

	// Parse poll interval
	pollInterval := 30 * time.Second
	if interval, exists := config.Settings["poll_interval"]; exists {
		if intervalStr, ok := interval.(string); ok {
			if parsed, err := time.ParseDuration(intervalStr); err == nil {
				pollInterval = parsed
			}
		} else if intervalFloat, ok := interval.(float64); ok {
			pollInterval = time.Duration(intervalFloat) * time.Second
		}
	}

	// Parse database mapping
	var mapping *DatabaseMapping
	if mappingData, exists := config.Settings["mapping"]; exists {
		if mappingMap, ok := mappingData.(map[string]interface{}); ok {
			if mappingBytes, err := json.Marshal(mappingMap); err == nil {
				mapping = &DatabaseMapping{}
				json.Unmarshal(mappingBytes, mapping)
			}
		}
	}

	if mapping == nil {
		// Default mapping
		mapping = &DatabaseMapping{
			Table:         "messages",
			IDColumn:      "id",
			ContentColumn: "content",
			TypeColumn:    "content_type",
			StatusColumn:  "status",
			CreatedColumn: "created_at",
			UpdatedColumn: "updated_at",
			CustomColumns: make(map[string]string),
			Filters:       map[string]interface{}{"status": "pending"},
		}
	}

	processor := &DatabaseMessageProcessor{
		dbType:     detectDatabaseType(config.Endpoint),
		mapping:    mapping,
		queryCache: make(map[string]*sql.Stmt),
	}

	// Build polling query
	query := processor.buildSelectQuery()

	connector := &DatabaseInputConnector{
		BaseConnector: base,
		pollInterval:  pollInterval,
		query:         query,
		stopChan:      make(chan bool),
		processor:     processor,
	}

	return connector, nil
}

// NewDatabaseOutputConnector creates a new database output connector
func NewDatabaseOutputConnector(config pkg.ConnectorConfig) (*DatabaseOutputConnector, error) {
	base := NewBaseConnector(config, "database_output")

	// Parse database mapping
	var mapping *DatabaseMapping
	if mappingData, exists := config.Settings["mapping"]; exists {
		if mappingMap, ok := mappingData.(map[string]interface{}); ok {
			if mappingBytes, err := json.Marshal(mappingMap); err == nil {
				mapping = &DatabaseMapping{}
				json.Unmarshal(mappingBytes, mapping)
			}
		}
	}

	if mapping == nil {
		// Default mapping
		mapping = &DatabaseMapping{
			Table:         "messages",
			IDColumn:      "id",
			ContentColumn: "content",
			TypeColumn:    "content_type",
			StatusColumn:  "status",
			CreatedColumn: "created_at",
			UpdatedColumn: "updated_at",
			CustomColumns: make(map[string]string),
		}
	}

	processor := &DatabaseMessageProcessor{
		dbType:     detectDatabaseType(config.Endpoint),
		mapping:    mapping,
		queryCache: make(map[string]*sql.Stmt),
	}

	// Build insert SQL
	insertSQL := processor.buildInsertQuery()

	connector := &DatabaseOutputConnector{
		BaseConnector: base,
		tableName:     mapping.Table,
		insertSQL:     insertSQL,
		processor:     processor,
	}

	return connector, nil
}

// Connect establishes database connection
func (dc *DatabaseInputConnector) Connect() error {
	db, err := sql.Open(dc.processor.dbType, dc.Config.Endpoint)
	if err != nil {
		dc.RecordError(err)
		return fmt.Errorf("failed to open database connection: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		dc.RecordError(err)
		return fmt.Errorf("failed to ping database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(1 * time.Hour)

	dc.db = db
	dc.SetConnected(true)

	return nil
}

// Disconnect closes database connection
func (dc *DatabaseInputConnector) Disconnect() error {
	if dc.db != nil {
		dc.db.Close()
		dc.db = nil
	}
	dc.SetConnected(false)
	return nil
}

// TestConnection tests database connectivity
func (dc *DatabaseInputConnector) TestConnection() error {
	db, err := sql.Open(dc.processor.dbType, dc.Config.Endpoint)
	if err != nil {
		return fmt.Errorf("failed to open test connection: %w", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	return nil
}

// Connect establishes database connection (output)
func (dc *DatabaseOutputConnector) Connect() error {
	db, err := sql.Open(dc.processor.dbType, dc.Config.Endpoint)
	if err != nil {
		dc.RecordError(err)
		return fmt.Errorf("failed to open database connection: %w", err)
	}

	if err := db.Ping(); err != nil {
		db.Close()
		dc.RecordError(err)
		return fmt.Errorf("failed to ping database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(1 * time.Hour)

	dc.db = db
	dc.SetConnected(true)

	return nil
}

// Disconnect closes database connection (output)
func (dc *DatabaseOutputConnector) Disconnect() error {
	if dc.db != nil {
		dc.db.Close()
		dc.db = nil
	}
	dc.SetConnected(false)
	return nil
}

// TestConnection tests database connectivity (output)
func (dc *DatabaseOutputConnector) TestConnection() error {
	db, err := sql.Open(dc.processor.dbType, dc.Config.Endpoint)
	if err != nil {
		return fmt.Errorf("failed to open test connection: %w", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		return fmt.Errorf("database ping failed: %w", err)
	}

	return nil
}

// StartListening begins polling the database for messages
func (dc *DatabaseInputConnector) StartListening(messageChan chan<- *pkg.UniversalMessage) error {
	if err := dc.Start(dc.ctx); err != nil {
		return err
	}

	if dc.db == nil {
		if err := dc.Connect(); err != nil {
			return err
		}
	}

	dc.messageChan = messageChan

	// Start polling goroutine
	go dc.pollDatabase()

	return nil
}

// StopListening stops database polling
func (dc *DatabaseInputConnector) StopListening() error {
	select {
	case dc.stopChan <- true:
	default:
	}

	return dc.Stop()
}

// SendMessage inserts a message into the database
func (dc *DatabaseOutputConnector) SendMessage(ctx context.Context, message *pkg.UniversalMessage) error {
	if dc.db == nil {
		if err := dc.Connect(); err != nil {
			return err
		}
	}

	startTime := time.Now()

	// Prepare values for insertion
	values := dc.processor.prepareInsertValues(message)

	// Execute insert
	result, err := dc.db.ExecContext(ctx, dc.insertSQL, values...)
	if err != nil {
		dc.RecordError(err)
		return fmt.Errorf("failed to insert message: %w", err)
	}

	// Get inserted ID if available
	if insertedID, err := result.LastInsertId(); err == nil && insertedID > 0 {
		message.Metadata["database_id"] = insertedID
	}

	// Update message status
	message.Status = pkg.StatusDelivered
	now := time.Now()
	message.DeliveredAt = &now

	// Record metrics
	latency := time.Since(startTime).Milliseconds()
	dc.RecordMessage(int64(len(message.Content)), latency)

	return nil
}

// SendBatch inserts multiple messages in a batch
func (dc *DatabaseOutputConnector) SendBatch(ctx context.Context, messages []*pkg.UniversalMessage) error {
	if dc.db == nil {
		if err := dc.Connect(); err != nil {
			return err
		}
	}

	// Use transaction for batch insert
	tx, err := dc.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, dc.insertSQL)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, message := range messages {
		values := dc.processor.prepareInsertValues(message)
		if _, err := stmt.ExecContext(ctx, values...); err != nil {
			dc.RecordError(err)
			return fmt.Errorf("failed to insert message %s: %w", message.ID, err)
		}

		message.Status = pkg.StatusDelivered
		now := time.Now()
		message.DeliveredAt = &now
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// SupportsAcknowledgment returns whether database supports acknowledgments
func (dc *DatabaseOutputConnector) SupportsAcknowledgment() bool {
	return false // Database inserts are synchronous
}

// WaitForAcknowledgment is not applicable for database connectors
func (dc *DatabaseOutputConnector) WaitForAcknowledgment(messageID string, timeout time.Duration) error {
	return nil // No-op for database connectors
}

// pollDatabase polls the database for new messages
func (dc *DatabaseInputConnector) pollDatabase() {
	ticker := time.NewTicker(dc.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-dc.Context().Done():
			return
		case <-dc.stopChan:
			return
		case <-ticker.C:
			if err := dc.fetchMessages(); err != nil {
				dc.RecordError(err)
			}
		}
	}
}

// fetchMessages retrieves messages from the database
func (dc *DatabaseInputConnector) fetchMessages() error {
	rows, err := dc.db.Query(dc.query)
	if err != nil {
		return fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	var processedCount int
	for rows.Next() {
		row, err := dc.processor.scanRow(rows)
		if err != nil {
			dc.RecordError(err)
			continue
		}

		// Convert to universal message
		message := dc.processor.rowToMessage(row)

		// Send to processing channel
		select {
		case dc.messageChan <- message:
			processedCount++

			// Mark as processed in database
			if err := dc.markAsProcessed(message.ID); err != nil {
				dc.RecordError(err)
			}

		case <-time.After(5 * time.Second):
			// Channel full, skip this message
			continue
		}
	}

	if processedCount > 0 {
		dc.RecordMessage(int64(processedCount), 0)
	}

	return rows.Err()
}

// markAsProcessed updates the message status in the database
func (dc *DatabaseInputConnector) markAsProcessed(messageID string) error {
	updateSQL := fmt.Sprintf("UPDATE %s SET %s = 'processed', %s = CURRENT_TIMESTAMP WHERE %s = $1",
		dc.processor.mapping.Table,
		dc.processor.mapping.StatusColumn,
		dc.processor.mapping.UpdatedColumn,
		dc.processor.mapping.IDColumn)

	_, err := dc.db.Exec(updateSQL, messageID)
	return err
}

// DatabaseMessageProcessor methods

// buildSelectQuery builds the SELECT query for polling
func (dmp *DatabaseMessageProcessor) buildSelectQuery() string {
	var conditions []string
	var args []string

	// Add filter conditions
	for column, value := range dmp.mapping.Filters {
		switch v := value.(type) {
		case string:
			conditions = append(conditions, fmt.Sprintf("%s = '%s'", column, v))
		case int, int64, float64:
			conditions = append(conditions, fmt.Sprintf("%s = %v", column, v))
		}
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	query := fmt.Sprintf(`
		SELECT %s, %s, %s, %s, %s, %s
		FROM %s
		%s
		ORDER BY %s ASC
		LIMIT 100`,
		dmp.mapping.IDColumn,
		dmp.mapping.ContentColumn,
		dmp.mapping.TypeColumn,
		dmp.mapping.StatusColumn,
		dmp.mapping.CreatedColumn,
		dmp.mapping.UpdatedColumn,
		dmp.mapping.Table,
		whereClause,
		dmp.mapping.CreatedColumn)

	return query
}

// buildInsertQuery builds the INSERT query
func (dmp *DatabaseMessageProcessor) buildInsertQuery() string {
	columns := []string{
		dmp.mapping.IDColumn,
		dmp.mapping.ContentColumn,
		dmp.mapping.TypeColumn,
		dmp.mapping.StatusColumn,
		dmp.mapping.CreatedColumn,
	}

	placeholders := []string{"$1", "$2", "$3", "$4", "$5"}

	// Add custom columns
	for dbColumn := range dmp.mapping.CustomColumns {
		columns = append(columns, dbColumn)
		placeholders = append(placeholders, "$"+strconv.Itoa(len(placeholders)+1))
	}

	query := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)",
		dmp.mapping.Table,
		strings.Join(columns, ", "),
		strings.Join(placeholders, ", "))

	return query
}

// prepareInsertValues prepares values for insertion
func (dmp *DatabaseMessageProcessor) prepareInsertValues(message *pkg.UniversalMessage) []interface{} {
	values := []interface{}{
		message.ID,
		message.Content,
		message.ContentType,
		string(message.Status),
		message.CreatedAt,
	}

	// Add custom column values
	for msgField, dbColumn := range dmp.mapping.CustomColumns {
		var value interface{}
		switch msgField {
		case "correlation_id":
			value = message.CorrelationID
		case "source_system":
			value = message.SourceSystem
		case "source_interface":
			value = message.SourceInterface
		case "target_system":
			value = message.TargetSystem
		case "target_interface":
			value = message.TargetInterface
		case "size":
			value = message.Size
		case "priority":
			value = message.Priority
		default:
			if metaValue, exists := message.Metadata[msgField]; exists {
				value = metaValue
			} else {
				value = nil
			}
		}
		values = append(values, value)
	}

	return values
}

// scanRow scans a database row into a DatabaseRow struct
func (dmp *DatabaseMessageProcessor) scanRow(rows *sql.Rows) (*DatabaseRow, error) {
	var row DatabaseRow
	var updatedAt sql.NullTime

	err := rows.Scan(
		&row.ID,
		&row.Content,
		&row.ContentType,
		&row.Status,
		&row.CreatedAt,
		&updatedAt,
	)

	if err != nil {
		return nil, err
	}

	if updatedAt.Valid {
		row.UpdatedAt = &updatedAt.Time
	}

	row.Metadata = make(map[string]interface{})

	return &row, nil
}

// rowToMessage converts a DatabaseRow to a UniversalMessage
func (dmp *DatabaseMessageProcessor) rowToMessage(row *DatabaseRow) *pkg.UniversalMessage {
	message := pkg.NewUniversalMessage()
	message.ID = row.ID
	message.Content = row.Content
	message.ContentType = row.ContentType
	message.Status = pkg.MessageStatus(row.Status)
	message.CreatedAt = row.CreatedAt
	message.Size = int64(len(row.Content))

	if row.UpdatedAt != nil {
		message.UpdatedAt = *row.UpdatedAt
	}

	// Copy metadata
	for key, value := range row.Metadata {
		message.Metadata[key] = value
	}

	// Add database-specific metadata
	message.Metadata["database_source"] = true
	message.Metadata["database_table"] = dmp.mapping.Table

	return message
}

// detectDatabaseType detects database type from connection string
func detectDatabaseType(connectionString string) string {
	connectionString = strings.ToLower(connectionString)

	if strings.Contains(connectionString, "postgres") || strings.Contains(connectionString, "postgresql") {
		return "postgres"
	}
	if strings.Contains(connectionString, "mysql") {
		return "mysql"
	}
	if strings.Contains(connectionString, "sqlserver") || strings.Contains(connectionString, "mssql") {
		return "sqlserver"
	}
	if strings.Contains(connectionString, "sqlite") {
		return "sqlite3"
	}

	// Default to postgres
	return "postgres"
}