package services

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// InterfaceMessageService handles storage and retrieval of messages in interface-specific tables
type InterfaceMessageService struct {
	db *sql.DB
}

// MessageData represents the data structure for storing interface message METADATA (not raw content)
// Raw message content is stored in MongoDB for scalability
type MessageData struct {
	MessageID        string
	CorrelationID    string
	InterfaceID      string
	Status           string
	Priority         int
	ReceivedAt       time.Time
	SourceType       string
	SourceEndpoint   string
	SourceIP         string
	MessageType      string
	MessageSize      int
	MessageEncoding  string
	MongoDocumentID  string    // Reference to MongoDB document for raw content
}

// NewInterfaceMessageService creates a new interface message service
func NewInterfaceMessageService(db *sql.DB) *InterfaceMessageService {
	return &InterfaceMessageService{
		db: db,
	}
}

// StoreMessage stores a message in the interface-specific table
func (ims *InterfaceMessageService) StoreMessage(interfaceID string, messageData *MessageData) error {
	if ims.db == nil {
		return fmt.Errorf("database connection not available")
	}

	// Generate interface-specific table name
	tableName := ims.getInterfaceTableName(interfaceID)

	// Ensure the interface table exists
	if err := ims.ensureInterfaceTableExists(tableName, interfaceID); err != nil {
		return fmt.Errorf("failed to ensure interface table exists: %w", err)
	}

	// Insert message metadata only (raw content stored in MongoDB)
	query := fmt.Sprintf(`
		INSERT INTO %s (
			id, message_id, correlation_id, interface_id, status, priority,
			received_at, source_type, source_endpoint, source_ip, message_type,
			message_size, message_encoding, mongo_document_id, processing_completed_at,
			processing_time_ms, error_count, last_error_message, delivery_status,
			delivery_attempts, created_at, updated_at
		) VALUES (
			gen_random_uuid(), $1, $2, $3, $4, $5,
			$6, $7, $8, $9, $10,
			$11, $12, $13, NULL,
			NULL, 0, NULL, 'pending',
			0, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
		)`, tableName)

	_, err := ims.db.Exec(query,
		messageData.MessageID,
		messageData.CorrelationID,
		messageData.InterfaceID,
		messageData.Status,
		messageData.Priority,
		messageData.ReceivedAt,
		messageData.SourceType,
		messageData.SourceEndpoint,
		messageData.SourceIP,
		messageData.MessageType,
		messageData.MessageSize,
		messageData.MessageEncoding,
		messageData.MongoDocumentID,
	)

	if err != nil {
		return fmt.Errorf("failed to insert message into %s: %w", tableName, err)
	}

	return nil
}

// UpdateMessageStatus updates the processing status of a message
func (ims *InterfaceMessageService) UpdateMessageStatus(interfaceID, messageID, status string, processingTimeMs int64, errorMessage string) error {
	tableName := ims.getInterfaceTableName(interfaceID)

	query := fmt.Sprintf(`
		UPDATE %s
		SET status = $1,
			processing_completed_at = CURRENT_TIMESTAMP,
			processing_time_ms = $2,
			last_error_message = $3,
			error_count = CASE WHEN $1 = 'failed' THEN error_count + 1 ELSE error_count END,
			updated_at = CURRENT_TIMESTAMP
		WHERE message_id = $4
	`, tableName)

	_, err := ims.db.Exec(query, status, processingTimeMs, errorMessage, messageID)
	if err != nil {
		return fmt.Errorf("failed to update message status: %w", err)
	}

	return nil
}

// getInterfaceTableName generates the standardized interface table name
func (ims *InterfaceMessageService) getInterfaceTableName(interfaceID string) string {
	return fmt.Sprintf("messages_intf_%s", strings.ReplaceAll(interfaceID, "-", "_"))
}

// ensureInterfaceTableExists creates the interface table if it doesn't exist
func (ims *InterfaceMessageService) ensureInterfaceTableExists(tableName, interfaceID string) error {
	// Check if table exists
	var exists bool
	checkQuery := `
		SELECT EXISTS (
			SELECT FROM information_schema.tables
			WHERE table_name = $1
		);
	`
	err := ims.db.QueryRow(checkQuery, tableName).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check table existence: %w", err)
	}

	if exists {
		return nil // Table already exists
	}

	// Create the table with standardized schema (METADATA ONLY - no raw_message)
	// Raw message content is stored in MongoDB for better scalability
	createQuery := fmt.Sprintf(`
		CREATE TABLE %s (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			message_id VARCHAR(255) NOT NULL,
			correlation_id VARCHAR(255),
			interface_id UUID NOT NULL,
			status VARCHAR(50) DEFAULT 'received',
			priority INTEGER DEFAULT 5,
			received_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			source_type VARCHAR(50),
			source_endpoint VARCHAR(255),
			source_ip VARCHAR(45),
			message_type VARCHAR(100),
			message_size INTEGER,
			message_encoding VARCHAR(50) DEFAULT 'UTF-8',
			mongo_document_id VARCHAR(255),
			processing_completed_at TIMESTAMP WITH TIME ZONE,
			processing_time_ms BIGINT,
			error_count INTEGER DEFAULT 0,
			last_error_message TEXT,
			delivery_status VARCHAR(50) DEFAULT 'pending',
			delivery_attempts INTEGER DEFAULT 0,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
			updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		);
	`, tableName)

	_, err = ims.db.Exec(createQuery)
	if err != nil {
		return fmt.Errorf("failed to create interface table: %w", err)
	}

	// Create indexes for performance
	indexQueries := []string{
		fmt.Sprintf("CREATE INDEX idx_%s_message_id ON %s(message_id);", tableName, tableName),
		fmt.Sprintf("CREATE INDEX idx_%s_received_at ON %s(received_at);", tableName, tableName),
		fmt.Sprintf("CREATE INDEX idx_%s_status ON %s(status);", tableName, tableName),
	}

	for _, indexQuery := range indexQueries {
		if _, err := ims.db.Exec(indexQuery); err != nil {
			// Log warning but don't fail - indexes are performance optimization
			fmt.Printf("⚠️ Warning: Failed to create index: %v\n", err)
		}
	}

	// Register in metadata table (if it exists)
	ims.registerTableMetadata(interfaceID, tableName)

	return nil
}

// registerTableMetadata registers the table in the metadata tracking table
func (ims *InterfaceMessageService) registerTableMetadata(interfaceID, tableName string) {
	// Try to register in metadata table, but don't fail if table doesn't exist
	metadataQuery := `
		INSERT INTO interface_table_metadata (interface_id, table_name, created_at)
		VALUES ($1, $2, CURRENT_TIMESTAMP)
		ON CONFLICT (interface_id) DO UPDATE SET
			table_name = $2, created_at = CURRENT_TIMESTAMP;
	`

	_, err := ims.db.Exec(metadataQuery, interfaceID, tableName)
	if err != nil {
		// Log warning but don't fail - metadata is optional
		fmt.Printf("⚠️ Warning: Failed to register table metadata: %v\n", err)
	}
}

// GetMessageCount returns the total number of messages for an interface
func (ims *InterfaceMessageService) GetMessageCount(interfaceID string) (int, error) {
	tableName := ims.getInterfaceTableName(interfaceID)

	var count int
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", tableName)
	err := ims.db.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get message count: %w", err)
	}

	return count, nil
}

// DropInterfaceTable drops the interface-specific message table
func (ims *InterfaceMessageService) DropInterfaceTable(interfaceID string) error {
	tableName := ims.getInterfaceTableName(interfaceID)

	// Check if table exists first
	var exists bool
	checkQuery := `
		SELECT EXISTS (
			SELECT FROM information_schema.tables
			WHERE table_name = $1
		);
	`
	err := ims.db.QueryRow(checkQuery, tableName).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check table existence: %w", err)
	}

	if !exists {
		fmt.Printf("⚠️ Table %s doesn't exist, nothing to drop\n", tableName)
		return nil
	}

	// Drop the table
	dropQuery := fmt.Sprintf("DROP TABLE IF EXISTS %s CASCADE", tableName)
	_, err = ims.db.Exec(dropQuery)
	if err != nil {
		return fmt.Errorf("failed to drop interface table %s: %w", tableName, err)
	}

	// Remove from metadata table
	metadataQuery := `DELETE FROM interface_table_metadata WHERE interface_id = $1`
	_, err = ims.db.Exec(metadataQuery, interfaceID)
	if err != nil {
		// Log warning but don't fail
		fmt.Printf("⚠️ Warning: Failed to remove table metadata: %v\n", err)
	}

	fmt.Printf("🗑️ Dropped interface table: %s\n", tableName)
	return nil
}
// MessageStatusUpdate contains status update data
type MessageStatusUpdate struct {
	Status         string
	MessageType    string
	ParsedAt       time.Time
	ParsingStatus  string
	ParsingTimeMs  int
	ParsingError   string
}

// UpdateMessageStatus updates PostgreSQL with parsing status
func (ims *InterfaceMessageService) UpdateMessageParsingStatus(
	interfaceID string,
	messageID string,
	update *MessageStatusUpdate,
) error {
	tableName := fmt.Sprintf("messages_intf_%s", strings.ReplaceAll(interfaceID, "-", "_"))

	query := fmt.Sprintf(`
		UPDATE %s
		SET
			status = $1,
			message_type = $2,
			parsed_at = $3,
			parsing_status = $4,
			parsing_time_ms = $5,
			parsing_error = $6,
			updated_at = CURRENT_TIMESTAMP
		WHERE message_id = $7
	`, tableName)

	result, err := ims.db.Exec(
		query,
		update.Status,
		update.MessageType,
		update.ParsedAt,
		update.ParsingStatus,
		update.ParsingTimeMs,
		update.ParsingError,
		messageID,
	)

	if err != nil {
		return fmt.Errorf("failed to update message status: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("message not found: %s", messageID)
	}

	return nil
}
