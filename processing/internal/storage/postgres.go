// internal/storage/postgres.go
// PostgreSQL storage provider implementation

package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lib/pq"
	_ "github.com/lib/pq"

	"ezhealthkonnect/processing/pkg"
)

// PostgreSQLProvider implements StorageProvider for PostgreSQL
type PostgreSQLProvider struct {
	db       *sql.DB
	config   *PostgresConfig
	schema   string
	isHealthy bool
}

// NewPostgreSQLProvider creates a new PostgreSQL storage provider
func NewPostgreSQLProvider(config *PostgresConfig) *PostgreSQLProvider {
	schema := config.Schema
	if schema == "" {
		schema = "public"
	}

	return &PostgreSQLProvider{
		config: config,
		schema: schema,
	}
}

// Connect establishes connection to PostgreSQL
func (p *PostgreSQLProvider) Connect(ctx context.Context) error {
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		p.config.Host,
		p.config.Port,
		p.config.Username,
		p.config.Password,
		p.config.Database,
		p.config.SSLMode,
	)

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return NewStorageError(ErrorTypeConnection, ProviderPostgreSQL, "connect", "failed to open database connection", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(p.config.MaxConnections)
	db.SetMaxIdleConns(p.config.MaxIdleConns)
	db.SetConnMaxLifetime(p.config.ConnMaxLifetime)

	// Test connection
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return NewStorageError(ErrorTypeConnection, ProviderPostgreSQL, "ping", "failed to ping database", err)
	}

	p.db = db
	p.isHealthy = true

	// Initialize schema
	if err := p.initializeSchema(ctx); err != nil {
		return NewStorageError(ErrorTypeConnection, ProviderPostgreSQL, "schema_init", "failed to initialize schema", err)
	}

	return nil
}

// Disconnect closes the database connection
func (p *PostgreSQLProvider) Disconnect() error {
	if p.db != nil {
		p.isHealthy = false
		return p.db.Close()
	}
	return nil
}

// Ping checks database connectivity
func (p *PostgreSQLProvider) Ping(ctx context.Context) error {
	if p.db == nil {
		return ErrConnectionFailed
	}
	return p.db.PingContext(ctx)
}

// IsHealthy returns the health status
func (p *PostgreSQLProvider) IsHealthy() bool {
	return p.isHealthy && p.db != nil
}

// GetProviderType returns the provider type
func (p *PostgreSQLProvider) GetProviderType() ProviderType {
	return ProviderPostgreSQL
}

// GetConnectionInfo returns connection information
func (p *PostgreSQLProvider) GetConnectionInfo() map[string]interface{} {
	return map[string]interface{}{
		"provider": "postgresql",
		"host":     p.config.Host,
		"port":     p.config.Port,
		"database": p.config.Database,
		"schema":   p.schema,
		"healthy":  p.isHealthy,
	}
}

// StoreMessage stores a universal message
func (p *PostgreSQLProvider) StoreMessage(ctx context.Context, message *pkg.UniversalMessage) error {
	query := `
		INSERT INTO ` + p.schema + `.messages (
			id, correlation_id, parent_id, content, content_type, encoding, size,
			source_system, source_interface, source_endpoint, source_protocol, source_ip,
			target_system, target_interface, target_endpoint, target_protocol,
			status, priority, processing_stage, received_at, processing_started,
			processing_completed, delivered_at, processing_time_ms, transformation_applied,
			error_count, retry_count, max_retries, delivery_status, delivery_attempts,
			ack_received, ack_timestamp, metadata, tags, created_at, updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16,
			$17, $18, $19, $20, $21, $22, $23, $24, $25, $26, $27, $28, $29, $30,
			$31, $32, $33, $34, $35, $36
		)
		ON CONFLICT (id) DO UPDATE SET
			status = EXCLUDED.status,
			processing_stage = EXCLUDED.processing_stage,
			processing_completed = EXCLUDED.processing_completed,
			processing_time_ms = EXCLUDED.processing_time_ms,
			error_count = EXCLUDED.error_count,
			retry_count = EXCLUDED.retry_count,
			delivery_status = EXCLUDED.delivery_status,
			delivery_attempts = EXCLUDED.delivery_attempts,
			ack_received = EXCLUDED.ack_received,
			ack_timestamp = EXCLUDED.ack_timestamp,
			metadata = EXCLUDED.metadata,
			updated_at = EXCLUDED.updated_at
	`

	metadataJSON, err := json.Marshal(message.Metadata)
	if err != nil {
		return NewStorageError(ErrorTypeWrite, ProviderPostgreSQL, "store_message", "failed to marshal metadata", err)
	}

	tagsJSON, err := json.Marshal(message.Tags)
	if err != nil {
		return NewStorageError(ErrorTypeWrite, ProviderPostgreSQL, "store_message", "failed to marshal tags", err)
	}

	_, err = p.db.ExecContext(ctx, query,
		message.ID, message.CorrelationID, message.ParentID, message.Content, message.ContentType,
		message.Encoding, message.Size, message.SourceSystem, message.SourceInterface,
		message.SourceEndpoint, message.SourceProtocol, message.SourceIP, message.TargetSystem,
		message.TargetInterface, message.TargetEndpoint, message.TargetProtocol, string(message.Status),
		message.Priority, message.ProcessingStage, message.ReceivedAt, message.ProcessingStarted,
		message.ProcessingCompleted, message.DeliveredAt, message.ProcessingTimeMs,
		message.TransformationApplied, message.ErrorCount, message.RetryCount, message.MaxRetries,
		string(message.DeliveryStatus), message.DeliveryAttempts, message.AckReceived,
		message.AckTimestamp, metadataJSON, tagsJSON, message.CreatedAt, message.UpdatedAt,
	)

	if err != nil {
		return NewStorageError(ErrorTypeWrite, ProviderPostgreSQL, "store_message", "failed to insert message", err)
	}

	// Store audit trail
	if err := p.storeAuditTrail(ctx, message.ID, message.AuditTrail); err != nil {
		// Log error but don't fail the main operation
		// TODO: Add proper logging
	}

	// Store validation results
	if err := p.storeValidationResults(ctx, message.ID, message.ValidationResults); err != nil {
		// Log error but don't fail the main operation
		// TODO: Add proper logging
	}

	// Store last error if present
	if message.LastError != nil {
		if err := p.storeMessageError(ctx, message.ID, message.LastError); err != nil {
			// Log error but don't fail the main operation
			// TODO: Add proper logging
		}
	}

	return nil
}

// GetMessage retrieves a message by ID
func (p *PostgreSQLProvider) GetMessage(ctx context.Context, messageID string) (*pkg.UniversalMessage, error) {
	query := `
		SELECT id, correlation_id, parent_id, content, content_type, encoding, size,
			   source_system, source_interface, source_endpoint, source_protocol, source_ip,
			   target_system, target_interface, target_endpoint, target_protocol,
			   status, priority, processing_stage, received_at, processing_started,
			   processing_completed, delivered_at, processing_time_ms, transformation_applied,
			   error_count, retry_count, max_retries, delivery_status, delivery_attempts,
			   ack_received, ack_timestamp, metadata, tags, created_at, updated_at
		FROM ` + p.schema + `.messages WHERE id = $1
	`

	message := &pkg.UniversalMessage{}
	var metadataJSON, tagsJSON []byte
	var status, deliveryStatus string

	err := p.db.QueryRowContext(ctx, query, messageID).Scan(
		&message.ID, &message.CorrelationID, &message.ParentID, &message.Content,
		&message.ContentType, &message.Encoding, &message.Size, &message.SourceSystem,
		&message.SourceInterface, &message.SourceEndpoint, &message.SourceProtocol,
		&message.SourceIP, &message.TargetSystem, &message.TargetInterface,
		&message.TargetEndpoint, &message.TargetProtocol, &status, &message.Priority,
		&message.ProcessingStage, &message.ReceivedAt, &message.ProcessingStarted,
		&message.ProcessingCompleted, &message.DeliveredAt, &message.ProcessingTimeMs,
		&message.TransformationApplied, &message.ErrorCount, &message.RetryCount,
		&message.MaxRetries, &deliveryStatus, &message.DeliveryAttempts,
		&message.AckReceived, &message.AckTimestamp, &metadataJSON, &tagsJSON,
		&message.CreatedAt, &message.UpdatedAt,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrMessageNotFound
		}
		return nil, NewStorageError(ErrorTypeRead, ProviderPostgreSQL, "get_message", "failed to query message", err)
	}

	// Parse enums
	message.Status = pkg.MessageStatus(status)
	message.DeliveryStatus = pkg.DeliveryStatus(deliveryStatus)

	// Parse JSON fields
	if len(metadataJSON) > 0 {
		if err := json.Unmarshal(metadataJSON, &message.Metadata); err != nil {
			return nil, NewStorageError(ErrorTypeRead, ProviderPostgreSQL, "get_message", "failed to unmarshal metadata", err)
		}
	}

	if len(tagsJSON) > 0 {
		if err := json.Unmarshal(tagsJSON, &message.Tags); err != nil {
			return nil, NewStorageError(ErrorTypeRead, ProviderPostgreSQL, "get_message", "failed to unmarshal tags", err)
		}
	}

	// Load related data
	if err := p.loadAuditTrail(ctx, message); err != nil {
		// Log error but continue
		// TODO: Add proper logging
	}

	if err := p.loadValidationResults(ctx, message); err != nil {
		// Log error but continue
		// TODO: Add proper logging
	}

	if err := p.loadMessageError(ctx, message); err != nil {
		// Log error but continue
		// TODO: Add proper logging
	}

	return message, nil
}

// UpdateMessage updates an existing message
func (p *PostgreSQLProvider) UpdateMessage(ctx context.Context, message *pkg.UniversalMessage) error {
	query := `
		UPDATE ` + p.schema + `.messages SET
			status = $2, processing_stage = $3, processing_completed = $4,
			processing_time_ms = $5, error_count = $6, retry_count = $7,
			delivery_status = $8, delivery_attempts = $9, ack_received = $10,
			ack_timestamp = $11, metadata = $12, updated_at = $13
		WHERE id = $1
	`

	metadataJSON, err := json.Marshal(message.Metadata)
	if err != nil {
		return NewStorageError(ErrorTypeWrite, ProviderPostgreSQL, "update_message", "failed to marshal metadata", err)
	}

	result, err := p.db.ExecContext(ctx, query,
		message.ID, string(message.Status), message.ProcessingStage,
		message.ProcessingCompleted, message.ProcessingTimeMs, message.ErrorCount,
		message.RetryCount, string(message.DeliveryStatus), message.DeliveryAttempts,
		message.AckReceived, message.AckTimestamp, metadataJSON, time.Now(),
	)

	if err != nil {
		return NewStorageError(ErrorTypeWrite, ProviderPostgreSQL, "update_message", "failed to update message", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return NewStorageError(ErrorTypeWrite, ProviderPostgreSQL, "update_message", "failed to get rows affected", err)
	}

	if rowsAffected == 0 {
		return ErrMessageNotFound
	}

	return nil
}

// DeleteMessage deletes a message by ID
func (p *PostgreSQLProvider) DeleteMessage(ctx context.Context, messageID string) error {
	// Start transaction to delete message and related data
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return NewStorageError(ErrorTypeTransaction, ProviderPostgreSQL, "delete_message", "failed to begin transaction", err)
	}
	defer tx.Rollback()

	// Delete related data first
	tables := []string{"audit_trail", "validation_results", "message_errors"}
	for _, table := range tables {
		_, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s.%s WHERE message_id = $1", p.schema, table), messageID)
		if err != nil {
			return NewStorageError(ErrorTypeDelete, ProviderPostgreSQL, "delete_message", "failed to delete related data", err)
		}
	}

	// Delete main message
	result, err := tx.ExecContext(ctx, fmt.Sprintf("DELETE FROM %s.messages WHERE id = $1", p.schema), messageID)
	if err != nil {
		return NewStorageError(ErrorTypeDelete, ProviderPostgreSQL, "delete_message", "failed to delete message", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return NewStorageError(ErrorTypeDelete, ProviderPostgreSQL, "delete_message", "failed to get rows affected", err)
	}

	if rowsAffected == 0 {
		return ErrMessageNotFound
	}

	return tx.Commit()
}

// FindMessages finds messages based on filter criteria
func (p *PostgreSQLProvider) FindMessages(ctx context.Context, filter MessageFilter) ([]*pkg.UniversalMessage, error) {
	query, args := p.buildFindQuery(filter)

	rows, err := p.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, NewStorageError(ErrorTypeQuery, ProviderPostgreSQL, "find_messages", "failed to execute query", err)
	}
	defer rows.Close()

	var messages []*pkg.UniversalMessage
	for rows.Next() {
		message := &pkg.UniversalMessage{}
		var metadataJSON, tagsJSON []byte
		var status, deliveryStatus string

		err := rows.Scan(
			&message.ID, &message.CorrelationID, &message.ParentID, &message.Content,
			&message.ContentType, &message.Encoding, &message.Size, &message.SourceSystem,
			&message.SourceInterface, &message.SourceEndpoint, &message.SourceProtocol,
			&message.SourceIP, &message.TargetSystem, &message.TargetInterface,
			&message.TargetEndpoint, &message.TargetProtocol, &status, &message.Priority,
			&message.ProcessingStage, &message.ReceivedAt, &message.ProcessingStarted,
			&message.ProcessingCompleted, &message.DeliveredAt, &message.ProcessingTimeMs,
			&message.TransformationApplied, &message.ErrorCount, &message.RetryCount,
			&message.MaxRetries, &deliveryStatus, &message.DeliveryAttempts,
			&message.AckReceived, &message.AckTimestamp, &metadataJSON, &tagsJSON,
			&message.CreatedAt, &message.UpdatedAt,
		)

		if err != nil {
			return nil, NewStorageError(ErrorTypeRead, ProviderPostgreSQL, "find_messages", "failed to scan row", err)
		}

		// Parse enums and JSON
		message.Status = pkg.MessageStatus(status)
		message.DeliveryStatus = pkg.DeliveryStatus(deliveryStatus)

		if len(metadataJSON) > 0 {
			json.Unmarshal(metadataJSON, &message.Metadata)
		}

		if len(tagsJSON) > 0 {
			json.Unmarshal(tagsJSON, &message.Tags)
		}

		messages = append(messages, message)
	}

	if err := rows.Err(); err != nil {
		return nil, NewStorageError(ErrorTypeRead, ProviderPostgreSQL, "find_messages", "error iterating rows", err)
	}

	return messages, nil
}

// CountMessages counts messages based on filter criteria
func (p *PostgreSQLProvider) CountMessages(ctx context.Context, filter MessageFilter) (int64, error) {
	query, args := p.buildCountQuery(filter)

	var count int64
	err := p.db.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, NewStorageError(ErrorTypeQuery, ProviderPostgreSQL, "count_messages", "failed to execute count query", err)
	}

	return count, nil
}

// FindMessagesByStatus finds messages by status
func (p *PostgreSQLProvider) FindMessagesByStatus(ctx context.Context, status pkg.MessageStatus, limit int) ([]*pkg.UniversalMessage, error) {
	filter := MessageFilter{
		Statuses: []pkg.MessageStatus{status},
		Limit:    limit,
		SortBy:   "received_at",
		SortOrder: SortAscending,
	}
	return p.FindMessages(ctx, filter)
}

// FindMessagesByInterface finds messages by interface ID
func (p *PostgreSQLProvider) FindMessagesByInterface(ctx context.Context, interfaceID string, limit int) ([]*pkg.UniversalMessage, error) {
	filter := MessageFilter{
		InterfaceIDs: []string{interfaceID},
		Limit:        limit,
		SortBy:       "received_at",
		SortOrder:    SortDescending,
	}
	return p.FindMessages(ctx, filter)
}

// BeginTransaction starts a new transaction
func (p *PostgreSQLProvider) BeginTransaction(ctx context.Context) (Transaction, error) {
	tx, err := p.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, NewStorageError(ErrorTypeTransaction, ProviderPostgreSQL, "begin_transaction", "failed to begin transaction", err)
	}

	return &PostgreSQLTransaction{
		tx:       tx,
		provider: p,
	}, nil
}

// PostgreSQLTransaction implements Transaction for PostgreSQL
type PostgreSQLTransaction struct {
	tx       *sql.Tx
	provider *PostgreSQLProvider
}

// Commit commits the transaction
func (t *PostgreSQLTransaction) Commit() error {
	return t.tx.Commit()
}

// Rollback rolls back the transaction
func (t *PostgreSQLTransaction) Rollback() error {
	return t.tx.Rollback()
}

// StoreMessage stores a message within the transaction
func (t *PostgreSQLTransaction) StoreMessage(ctx context.Context, message *pkg.UniversalMessage) error {
	// Implementation similar to regular StoreMessage but using t.tx instead of p.db
	// This is a simplified version - full implementation would be similar to StoreMessage
	query := `INSERT INTO ` + t.provider.schema + `.messages (...) VALUES (...)`
	_, err := t.tx.ExecContext(ctx, query /* ... args ... */)
	return err
}

// UpdateMessage updates a message within the transaction
func (t *PostgreSQLTransaction) UpdateMessage(ctx context.Context, message *pkg.UniversalMessage) error {
	// Implementation similar to regular UpdateMessage but using t.tx instead of p.db
	query := `UPDATE ` + t.provider.schema + `.messages SET ... WHERE id = $1`
	_, err := t.tx.ExecContext(ctx, query /* ... args ... */)
	return err
}

// Helper methods for building queries, managing schema, etc.
// (Implementation continues with helper methods...)