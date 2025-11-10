package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"ezhealthkonnect/models"
	"fmt"
	"strings"
	"time"
)

// OutputDeliveryService orchestrates delivery of transformed messages to target destinations
// This is the main service that handles delivery with retry logic and audit logging
// NOTE: Actual connector creation happens in processing layer via CreateOutputConnector()
type OutputDeliveryService struct {
	db *sql.DB
}

// NewOutputDeliveryService creates a new output delivery service
func NewOutputDeliveryService(db *sql.DB) *OutputDeliveryService {
	return &OutputDeliveryService{
		db: db,
	}
}

// DeliverToDestination is the main entry point for delivering a message
// This method handles the complete delivery process including retry logic
func (ods *OutputDeliveryService) DeliverToDestination(request *models.DeliveryRequest) {
	fmt.Printf("\n📤 ========== OUTPUT DELIVERY STARTED ==========\n")
	fmt.Printf("Output Message ID: %s\n", request.OutputMessageID)
	fmt.Printf("Interface ID: %s\n", request.InterfaceID)
	fmt.Printf("Input Message ID: %s\n", request.InputMessageID)
	fmt.Printf("Destination Type: %s\n", request.TargetConfig.Type)
	fmt.Printf("Retry Attempt: %d/%d\n", request.RetryAttempt, request.MaxRetries)
	fmt.Printf("==============================================\n\n")

	// Execute delivery with retry logic
	result := ods.executeDeliveryWithRetry(request)

	// Update delivery status in database
	if err := ods.updateDeliveryStatus(request.OutputMessageID, result); err != nil {
		fmt.Printf("⚠️ Failed to update delivery status in database: %v\n", err)
	}

	// Update input message status after successful delivery
	if result.Success {
		if err := ods.updateInputMessageStatus(request.InterfaceID, request.InputMessageID, "delivered"); err != nil {
			fmt.Printf("⚠️ Failed to update input message status: %v\n", err)
		}
	}

	// Log delivery attempt to audit table
	if err := ods.logDeliveryAttempt(request, result); err != nil {
		fmt.Printf("⚠️ Failed to log delivery attempt: %v\n", err)
	}

	fmt.Printf("\n📤 ========== OUTPUT DELIVERY COMPLETED ==========\n")
	fmt.Printf("Status: %s\n", result.Status)
	fmt.Printf("Success: %v\n", result.Success)
	fmt.Printf("Time: %dms\n", result.DeliveryTimeMs)
	if result.ErrorMessage != "" {
		fmt.Printf("Error: %s\n", result.ErrorMessage)
	}
	fmt.Printf("=================================================\n\n")
}

// executeDeliveryWithRetry executes delivery with automatic retry on failure
func (ods *OutputDeliveryService) executeDeliveryWithRetry(request *models.DeliveryRequest) *models.LegacyDeliveryResult {
	retryPolicy := request.TargetConfig.GetRetryPolicy()

	// Create output message from request
	message := models.OutputMessage{
		ID:                   request.OutputMessageID,
		InterfaceID:          request.InterfaceID,
		CorrelationID:        request.CorrelationID,
		MessageType:          request.MessageType,
		Content:              request.TransformedData,
		ContentType:          request.TargetConfig.GetContentType(),
		SourceMessageID:      request.InputMessageID,
	}

	// Attempt delivery
	result := ods.executeDelivery(message, request.TargetConfig)

	// If delivery succeeded, return immediately
	if result.Success {
		return result
	}

	// If this was the last retry attempt, mark as permanently failed
	if request.RetryAttempt >= request.MaxRetries {
		fmt.Printf("❌ Delivery failed permanently after %d attempts\n", request.MaxRetries+1)
		result.Status = "failed"
		result.ShouldRetry = false
		return result
	}

	// Check if we should retry
	if !result.ShouldRetry {
		fmt.Printf("⚠️ Delivery failed with non-retryable error\n")
		result.Status = "failed"
		return result
	}

	// Calculate retry delay using exponential backoff
	retryDelayMs := retryPolicy.CalculateRetryDelay(request.RetryAttempt)
	nextRetryAt := time.Now().Add(time.Duration(retryDelayMs) * time.Millisecond)

	result.Status = "retrying"
	result.NextRetryAt = nextRetryAt
	result.RetryDelayS = retryDelayMs / 1000

	fmt.Printf("⏳ Scheduling retry %d/%d in %d seconds\n", request.RetryAttempt+1, request.MaxRetries, result.RetryDelayS)

	// Schedule async retry after delay
	go func() {
		time.Sleep(time.Duration(retryDelayMs) * time.Millisecond)

		fmt.Printf("🔄 Executing retry %d/%d for message %s\n", request.RetryAttempt+1, request.MaxRetries, request.OutputMessageID)

		// Create new request for retry
		retryRequest := &models.DeliveryRequest{
			OutputMessageID: request.OutputMessageID,
			InterfaceID:     request.InterfaceID,
			InputMessageID:  request.InputMessageID,
			CorrelationID:   request.CorrelationID,
			TransformedData: request.TransformedData,
			MessageType:     request.MessageType,
			TargetConfig:    request.TargetConfig,
			RetryAttempt:    request.RetryAttempt + 1,
			MaxRetries:      request.MaxRetries,
		}

		// Retry delivery
		ods.DeliverToDestination(retryRequest)
	}()

	return result
}

// executeDelivery executes a single delivery attempt
func (ods *OutputDeliveryService) executeDelivery(message models.OutputMessage, config models.DestinationConfig) *models.LegacyDeliveryResult {
	startTime := time.Now()

	// NOTE: Connector creation now happens in processing layer via CreateOutputConnector()
	// This service focuses on delivery orchestration, retry logic, and status tracking
	// For now, return a placeholder result - actual connector creation will be wired
	// through the processing engine's output delivery flow

	// TODO: Wire to processing.CreateOutputConnector() when integration is complete
	return &models.LegacyDeliveryResult{
		Success:        false,
		Status:         "pending_integration",
		ErrorMessage:   "Output connector integration in progress - use processing layer",
		DeliveryTimeMs: time.Since(startTime).Milliseconds(),
		ShouldRetry:    false,
	}
}

// updateDeliveryStatus updates the delivery status in the output table
func (ods *OutputDeliveryService) updateDeliveryStatus(outputMessageID string, result *models.LegacyDeliveryResult) error {
	ctx := context.Background()

	// Determine table name from output message ID
	// Output message IDs follow pattern: output_<interface_id>_<timestamp>
	// We need to query output_table_metadata to find the table name

	// Find the output table containing this message
	var tableName string

	queryTableName := `
		SELECT interface_id, output_table_name
		FROM output_table_metadata
		WHERE output_table_name IN (
			SELECT tablename
			FROM pg_tables
			WHERE tablename LIKE 'output_intf_%'
		)
		LIMIT 100
	`

	rows, err := ods.db.QueryContext(ctx, queryTableName)
	if err != nil {
		return fmt.Errorf("failed to query output tables: %w", err)
	}
	defer rows.Close()

	// Find the table containing this message
	for rows.Next() {
		var ifaceID, tblName string
		if err := rows.Scan(&ifaceID, &tblName); err != nil {
			continue
		}

		// Check if message exists in this table
		checkQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE id = $1", tblName)
		var count int
		if err := ods.db.QueryRowContext(ctx, checkQuery, outputMessageID).Scan(&count); err == nil && count > 0 {
			tableName = tblName
			break
		}
	}

	if tableName == "" {
		return fmt.Errorf("could not find output table for message %s", outputMessageID)
	}

	// Update delivery status
	updateQuery := fmt.Sprintf(`
		UPDATE %s
		SET delivery_status = $1,
		    delivery_status_code = $2,
		    delivery_response = $3,
		    delivery_time_ms = $4,
		    delivery_completed_at = $5,
		    retry_count = COALESCE(retry_count, 0) + 1,
		    next_retry_at = $6,
		    updated_at = NOW()
		WHERE id = $7
	`, tableName)

	var nextRetryAt *time.Time
	if !result.NextRetryAt.IsZero() {
		nextRetryAt = &result.NextRetryAt
	}

	var completedAt *time.Time
	if result.Success {
		now := time.Now()
		completedAt = &now
	}

	_, err = ods.db.ExecContext(ctx, updateQuery,
		result.Status,
		result.StatusCode,
		result.Response,
		result.DeliveryTimeMs,
		completedAt,
		nextRetryAt,
		outputMessageID,
	)

	if err != nil {
		return fmt.Errorf("failed to update delivery status: %w", err)
	}

	fmt.Printf("✅ Updated delivery status in %s: %s\n", tableName, result.Status)
	return nil
}

// updateInputMessageStatus updates the input message table status after delivery
func (ods *OutputDeliveryService) updateInputMessageStatus(interfaceID, inputMessageID, status string) error {
	ctx := context.Background()

	// Build input table name (convert UUID dashes to underscores)
	inputTableName := fmt.Sprintf("messages_intf_%s",
		strings.ReplaceAll(interfaceID, "-", "_"))

	// Update input message status and delivery_status
	updateQuery := fmt.Sprintf(`
		UPDATE %s
		SET status = $1,
		    delivery_status = $2,
		    delivery_attempts = delivery_attempts + 1,
		    updated_at = NOW()
		WHERE message_id = $3
	`, inputTableName)

	result, err := ods.db.ExecContext(ctx, updateQuery, status, status, inputMessageID)
	if err != nil {
		return fmt.Errorf("failed to update input message status: %w", err)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		fmt.Printf("⚠️ Input message not found in %s: %s\n", inputTableName, inputMessageID)
	} else {
		fmt.Printf("✅ Updated input message status in %s: %s → %s\n", inputTableName, inputMessageID, status)
	}

	return nil
}

// logDeliveryAttempt logs the delivery attempt to the audit table
func (ods *OutputDeliveryService) logDeliveryAttempt(request *models.DeliveryRequest, result *models.LegacyDeliveryResult) error {
	ctx := context.Background()

	// Serialize request body (transformed data)
	requestBody, _ := json.Marshal(request.TransformedData)

	insertQuery := `
		INSERT INTO delivery_audit_log (
			output_message_id,
			interface_id,
			attempt_number,
			delivery_status,
			status_code,
			request_body,
			response_body,
			error_message,
			delivery_time_ms,
			created_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW())
	`

	_, err := ods.db.ExecContext(ctx, insertQuery,
		request.OutputMessageID,
		request.InterfaceID,
		request.RetryAttempt+1, // Attempt number starts at 1
		result.Status,
		result.StatusCode,
		string(requestBody),
		result.Response,
		result.ErrorMessage,
		result.DeliveryTimeMs,
	)

	if err != nil {
		return fmt.Errorf("failed to insert audit log: %w", err)
	}

	fmt.Printf("✅ Logged delivery attempt to audit table\n")
	return nil
}

// GetDeliveryStatus retrieves the current delivery status for an output message
func (ods *OutputDeliveryService) GetDeliveryStatus(outputMessageID string) (*DeliveryStatus, error) {
	// This would query the output table and return delivery status
	// Implementation similar to updateDeliveryStatus
	// Left as TODO for API endpoint implementation
	return nil, fmt.Errorf("not implemented")
}

// RetryFailedDelivery manually retries a failed delivery
func (ods *OutputDeliveryService) RetryFailedDelivery(outputMessageID string) error {
	// This would look up the failed message and retry delivery
	// Implementation requires querying output table for message details
	// Left as TODO for API endpoint implementation
	return fmt.Errorf("not implemented")
}

// DeliveryStatus represents the current status of a delivery
type DeliveryStatus struct {
	OutputMessageID      string
	DeliveryStatus       string
	DeliveryAttempts     int
	LastDeliveryAt       time.Time
	NextRetryAt          *time.Time
	DeliveryResponse     string
	DeliveryStatusCode   int
	DeliveryTimeMs       int64
}
