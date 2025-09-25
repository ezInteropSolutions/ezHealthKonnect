// processing/message_queue.go
// Message queue service for Go processing engine

package processing

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

// MessageQueue handles message queuing and processing
type MessageQueue struct {
	db          *sql.DB
	batchSize   int
	pollInterval time.Duration
	isRunning   bool
}

// QueuedMessage represents a message in the processing queue
type QueuedMessage struct {
	ID             string                 `json:"id"`
	InterfaceID    string                 `json:"interface_id"`
	MessageID      string                 `json:"message_id"`
	MessageData    map[string]interface{} `json:"message_data"`
	MessageMetadata map[string]interface{} `json:"message_metadata"`
	MessageType    string                 `json:"message_type"`
	Status         string                 `json:"status"`
	Priority       int                    `json:"priority"`
	ScheduledFor   time.Time              `json:"scheduled_for"`
	Attempts       int                    `json:"attempts"`
	MaxAttempts    int                    `json:"max_attempts"`
	ErrorMessage   string                 `json:"error_message"`
	ErrorDetails   map[string]interface{} `json:"error_details"`
	CreatedAt      time.Time              `json:"created_at"`
}

// NewMessageQueue creates a new message queue instance
func NewMessageQueue(db *sql.DB) *MessageQueue {
	return &MessageQueue{
		db:           db,
		batchSize:    10,
		pollInterval: 5 * time.Second,
	}
}

// Start begins message queue processing
func (mq *MessageQueue) Start(ctx context.Context) error {
	if mq.isRunning {
		return fmt.Errorf("message queue is already running")
	}

	mq.isRunning = true
	log.Printf("🚀 Starting message queue processing...")

	// Start processing goroutine
	go mq.processMessages(ctx)

	return nil
}

// Stop stops message queue processing
func (mq *MessageQueue) Stop() {
	if !mq.isRunning {
		return
	}

	mq.isRunning = false
	log.Printf("🛑 Message queue processing stopped")
}

// EnqueueMessage adds a message to the processing queue
func (mq *MessageQueue) EnqueueMessage(interfaceID, messageID string, messageData map[string]interface{}, options map[string]interface{}) error {
	id := uuid.New().String()

	// Extract options with defaults
	priority := 5
	if p, ok := options["priority"].(int); ok {
		priority = p
	}

	maxAttempts := 3
	if ma, ok := options["maxAttempts"].(int); ok {
		maxAttempts = ma
	}

	scheduledFor := time.Now()
	if sf, ok := options["scheduledFor"].(time.Time); ok {
		scheduledFor = sf
	}

	messageType := ""
	if mt, ok := options["messageType"].(string); ok {
		messageType = mt
	}

	metadata := make(map[string]interface{})
	if md, ok := options["metadata"].(map[string]interface{}); ok {
		metadata = md
	}

	// Convert to JSON
	messageDataJSON, err := json.Marshal(messageData)
	if err != nil {
		return fmt.Errorf("failed to marshal message data: %w", err)
	}

	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Insert into database
	query := `
		INSERT INTO message_processing_queue (
			id, interface_id, message_id, message_data, message_metadata,
			message_type, status, priority, scheduled_for, attempts,
			max_attempts, created_at
		) VALUES ($1, $2, $3, $4, $5, $6, 'pending', $7, $8, 0, $9, $10)
	`

	_, err = mq.db.Exec(query, id, interfaceID, messageID, string(messageDataJSON),
		string(metadataJSON), messageType, priority, scheduledFor, maxAttempts, time.Now())

	if err != nil {
		return fmt.Errorf("failed to enqueue message: %w", err)
	}

	log.Printf("📨 Message enqueued: %s (interface: %s)", messageID, interfaceID)
	return nil
}

// processMessages continuously processes messages from the queue
func (mq *MessageQueue) processMessages(ctx context.Context) {
	ticker := time.NewTicker(mq.pollInterval)
	defer ticker.Stop()

	for mq.isRunning {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := mq.processNextBatch(); err != nil {
				log.Printf("❌ Failed to process message batch: %v", err)
			}
		}
	}
}

// processNextBatch processes the next batch of pending messages
func (mq *MessageQueue) processNextBatch() error {
	// Get pending messages
	query := `
		SELECT id, interface_id, message_id, message_data, message_metadata,
			   message_type, status, priority, scheduled_for, attempts,
			   max_attempts, error_message, error_details, created_at
		FROM message_processing_queue
		WHERE status = 'pending'
		AND scheduled_for <= NOW()
		ORDER BY priority DESC, scheduled_for ASC
		LIMIT $1
		FOR UPDATE SKIP LOCKED
	`

	rows, err := mq.db.Query(query, mq.batchSize)
	if err != nil {
		return fmt.Errorf("failed to query pending messages: %w", err)
	}
	defer rows.Close()

	var messages []QueuedMessage
	for rows.Next() {
		var msg QueuedMessage
		var messageDataStr, metadataStr, errorDetailsStr sql.NullString

		err := rows.Scan(
			&msg.ID, &msg.InterfaceID, &msg.MessageID, &messageDataStr,
			&metadataStr, &msg.MessageType, &msg.Status, &msg.Priority,
			&msg.ScheduledFor, &msg.Attempts, &msg.MaxAttempts,
			&msg.ErrorMessage, &errorDetailsStr, &msg.CreatedAt,
		)
		if err != nil {
			log.Printf("❌ Failed to scan message row: %v", err)
			continue
		}

		// Parse JSON fields
		if messageDataStr.Valid {
			if err := json.Unmarshal([]byte(messageDataStr.String), &msg.MessageData); err != nil {
				log.Printf("❌ Failed to parse message data for %s: %v", msg.ID, err)
				continue
			}
		}

		if metadataStr.Valid {
			if err := json.Unmarshal([]byte(metadataStr.String), &msg.MessageMetadata); err != nil {
				log.Printf("❌ Failed to parse metadata for %s: %v", msg.ID, err)
				msg.MessageMetadata = make(map[string]interface{})
			}
		}

		if errorDetailsStr.Valid {
			if err := json.Unmarshal([]byte(errorDetailsStr.String), &msg.ErrorDetails); err != nil {
				msg.ErrorDetails = make(map[string]interface{})
			}
		}

		messages = append(messages, msg)
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating message rows: %w", err)
	}

	// Process each message
	for _, msg := range messages {
		if err := mq.processMessage(msg); err != nil {
			log.Printf("❌ Failed to process message %s: %v", msg.ID, err)
		}
	}

	return nil
}

// processMessage processes an individual message
func (mq *MessageQueue) processMessage(msg QueuedMessage) error {
	// Update status to processing
	if err := mq.updateMessageStatus(msg.ID, "processing", "", nil); err != nil {
		return fmt.Errorf("failed to update message status: %w", err)
	}

	// TODO: Implement actual message processing logic
	// This is where you would:
	// 1. Get the interface worker for this interface
	// 2. Process the message through the interface pipeline
	// 3. Handle transformations, validations, etc.

	// For now, simulate processing
	log.Printf("🔄 Processing message %s for interface %s", msg.MessageID, msg.InterfaceID)

	// Simulate processing time
	time.Sleep(100 * time.Millisecond)

	// Mark as completed (in real implementation, this would be based on actual processing result)
	if err := mq.updateMessageStatus(msg.ID, "completed", "", nil); err != nil {
		return fmt.Errorf("failed to mark message as completed: %w", err)
	}

	log.Printf("✅ Message processed successfully: %s", msg.MessageID)
	return nil
}

// updateMessageStatus updates the status of a message in the queue
func (mq *MessageQueue) updateMessageStatus(messageID, status, errorMessage string, errorDetails map[string]interface{}) error {
	var errorDetailsJSON string
	if errorDetails != nil {
		details, err := json.Marshal(errorDetails)
		if err != nil {
			return fmt.Errorf("failed to marshal error details: %w", err)
		}
		errorDetailsJSON = string(details)
	}

	query := `
		UPDATE message_processing_queue
		SET status = $1, error_message = $2, error_details = $3, updated_at = NOW()
		WHERE id = $4
	`

	_, err := mq.db.Exec(query, status, errorMessage, errorDetailsJSON, messageID)
	if err != nil {
		return fmt.Errorf("failed to update message status: %w", err)
	}

	return nil
}

// GetQueueStats returns statistics about the message queue
func (mq *MessageQueue) GetQueueStats() (map[string]interface{}, error) {
	query := `
		SELECT
			status,
			COUNT(*) as count
		FROM message_processing_queue
		WHERE created_at >= CURRENT_DATE - INTERVAL '1 day'
		GROUP BY status
	`

	rows, err := mq.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query queue stats: %w", err)
	}
	defer rows.Close()

	stats := make(map[string]interface{})
	totalCount := 0

	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			continue
		}
		stats[status] = count
		totalCount += count
	}

	stats["total"] = totalCount
	stats["last_updated"] = time.Now().Format(time.RFC3339)

	return stats, nil
}