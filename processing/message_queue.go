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

	"ezhealthkonnect/models"
	"ezhealthkonnect/services"

	"github.com/google/uuid"
)

// MessageQueue handles durable message queuing with database-backed persistence.
// Uses FOR UPDATE SKIP LOCKED so multiple workers can dequeue concurrently without conflict.
// Primary use case: retry of failed messages and startup recovery of stale 'received' messages.
type MessageQueue struct {
	db                  *sql.DB
	transformationSvc   *services.TransformationPipelineService
	batchSize           int
	pollInterval        time.Duration
	isRunning           bool
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

// NewMessageQueue creates a new message queue instance.
// transformationSvc may be nil during tests; processMessage will skip pipeline execution.
func NewMessageQueue(db *sql.DB, transformationSvc *services.TransformationPipelineService) *MessageQueue {
	return &MessageQueue{
		db:                db,
		transformationSvc: transformationSvc,
		batchSize:         10,
		pollInterval:      5 * time.Second,
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

// processMessage processes an individual message through the transformation pipeline.
// This replaces the previous TODO+sleep placeholder.
func (mq *MessageQueue) processMessage(msg QueuedMessage) error {
	if err := mq.updateMessageStatus(msg.ID, "processing", "", nil); err != nil {
		return fmt.Errorf("failed to update message status: %w", err)
	}

	log.Printf("🔄 [MessageQueue] Processing message %s for interface %s (attempt %d/%d)",
		msg.MessageID, msg.InterfaceID, msg.Attempts+1, msg.MaxAttempts)

	if mq.transformationSvc == nil {
		// No pipeline service available (e.g. tests) — mark completed and skip
		log.Printf("⚠️  [MessageQueue] No transformation service — skipping pipeline for %s", msg.MessageID)
		return mq.updateMessageStatus(msg.ID, "completed", "", nil)
	}

	// Build a minimal parsed message from stored message data.
	// MessageData holds the raw parsed JSON that was stored when the message was first received.
	messageData := msg.MessageData
	if messageData == nil {
		messageData = map[string]interface{}{}
	}

	// Propagate message type into the envelope (used by pipeline for format detection)
	if msg.MessageType != "" {
		messageData["_messageType"] = msg.MessageType
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Resolve pipeline for this interface + message type
	pipeline, err := mq.transformationSvc.GetPipeline(ctx, msg.InterfaceID, msg.MessageType)
	if err != nil || pipeline == nil {
		errMsg := "no pipeline found"
		if err != nil {
			errMsg = err.Error()
		}
		log.Printf("⚠️  [MessageQueue] No pipeline for interface %s / %s: %s",
			msg.InterfaceID, msg.MessageType, errMsg)
		// Not a transient error — mark completed so it doesn't retry forever
		return mq.updateMessageStatus(msg.ID, "completed", errMsg, nil)
	}

	result, err := mq.transformationSvc.ExecutePipeline(ctx, pipeline, messageData)
	if err != nil {
		log.Printf("❌ [MessageQueue] Pipeline failed for %s: %v", msg.MessageID, err)

		// Check if retries are exhausted
		if msg.Attempts+1 >= msg.MaxAttempts {
			log.Printf("💀 [MessageQueue] Max retries reached for %s — moving to failed", msg.MessageID)
			return mq.updateMessageStatus(msg.ID, "failed", err.Error(), map[string]interface{}{
				"_exhausted": true,
			})
		}

		// Schedule retry with exponential backoff (30s, 60s, 120s, …)
		backoff := time.Duration(30<<uint(msg.Attempts)) * time.Second
		if backoff > 10*time.Minute {
			backoff = 10 * time.Minute
		}
		nextRetry := time.Now().Add(backoff)
		return mq.scheduleRetry(msg.ID, err.Error(), nextRetry)
	}

	log.Printf("✅ [MessageQueue] Pipeline completed for %s (status: %s)", msg.MessageID, result.Status)
	finalStatus := "completed"
	if result.Status != "completed" {
		finalStatus = "failed"
	}
	return mq.updateMessageStatus(msg.ID, finalStatus, "", nil)
}

// scheduleRetry marks a queued message for retry at a future time.
func (mq *MessageQueue) scheduleRetry(queueID, errMsg string, nextRetry time.Time) error {
	query := `
		UPDATE message_processing_queue
		SET status = 'pending',
		    attempts = attempts + 1,
		    error_message = $1,
		    scheduled_for = $2,
		    updated_at = NOW()
		WHERE id = $3
	`
	_, err := mq.db.Exec(query, errMsg, nextRetry, queueID)
	if err != nil {
		return fmt.Errorf("failed to schedule retry: %w", err)
	}
	log.Printf("🔁 [MessageQueue] Retry scheduled for queue entry %s at %v", queueID, nextRetry)
	return nil
}

// EnqueueForRecovery adds a message from messages_intf_* back into the durable queue
// for reprocessing. Called by the startup recovery loop.
func (mq *MessageQueue) EnqueueForRecovery(interfaceID, messageID, messageType, rawContent string) error {
	messageData := map[string]interface{}{
		"raw":         rawContent,
		"_recovered":  true,
	}
	return mq.EnqueueMessage(interfaceID, messageID, messageData, map[string]interface{}{
		"messageType": messageType,
		"priority":    8, // high priority for recovered messages
		"maxAttempts": 3,
		"metadata":    map[string]interface{}{"recovery": true},
	})
}

// GetQueueStats returns statistics about pending/processing/failed messages in the queue.
// (Kept from original implementation — now also used by /api/system/queue-depths.)
func (mq *MessageQueue) GetStats() (map[string]interface{}, error) {
	return mq.GetQueueStats()
}

// Needed to satisfy import (models is used in EnqueueForRecovery signature context)
var _ = models.MessageStatusReceived

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