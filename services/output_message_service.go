package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// OutputMessageService handles hybrid storage of transformation output messages
// MongoDB: Full FHIR bundles and transformation details
// PostgreSQL: Metadata and delivery tracking
type OutputMessageService struct {
	db            *sql.DB
	mongoClient   *mongo.Client
	mongoDatabase string // Database name
}

// OutputMessageData represents the data structure for storing transformation output messages
type OutputMessageData struct {
	ID                          string
	InterfaceID                 string
	InputMessageID              string
	CorrelationID               string
	TransformationPipelineID    string
	MessageType                 string
	Status                      string
	Priority                    int

	// Source message metadata
	SourceMessageSize           int
	SourceMessageType           string
	SourceEncoding              string

	// Transformation results
	TransformedMessage          map[string]interface{} // JSONB data
	TransformationMetadata      map[string]interface{} // JSONB data
	TargetFormat                string
	TargetEncoding              string

	// Output message details
	OutputMessageSize           int
	FHIRResourceType            string
	FHIRResourceID              string

	// Processing metadata
	TransformationStartedAt     time.Time
	TransformationCompletedAt   time.Time
	TransformationTimeMs        int64

	// Error handling
	ErrorCount                  int
	LastErrorMessage            string
	ValidationStatus            string
	ValidationErrors            map[string]interface{} // JSONB data

	// Delivery tracking
	DeliveryStatus              string
	DeliveryAttempts            int
	DeliveryEndpoint            string
	LastDeliveryAttempt         time.Time
	DeliveryResponse            string

	// Audit fields
	CreatedAt                   time.Time
	CreatedBy                   string
}

// TransformationResult represents the result of a message transformation
type TransformationResult struct {
	Success                     bool
	TransformedMessage          map[string]interface{}
	TransformationMetadata      map[string]interface{}
	ValidationErrors           map[string]interface{}
	FHIRResourceType           string
	FHIRResourceID             string
	ProcessingTimeMs           int64
	ErrorMessage               string
}

// NewOutputMessageService creates a new hybrid output message service
func NewOutputMessageService(db *sql.DB, mongoClient *mongo.Client, mongoDatabase string) *OutputMessageService {
	return &OutputMessageService{
		db:            db,
		mongoClient:   mongoClient,
		mongoDatabase: mongoDatabase,
	}
}

// StoreTransformationResult stores transformation result in hybrid storage
// MongoDB: Full FHIR bundle with transformation details
// PostgreSQL: Metadata and delivery tracking
func (oms *OutputMessageService) StoreTransformationResult(ctx context.Context, interfaceID, inputMessageID, correlationID string, result *TransformationResult) error {
	if oms.db == nil {
		return fmt.Errorf("database connection not available")
	}

	// STEP 1: Store full FHIR bundle in MongoDB (if available)
	mongoReferenceKey := ""
	if oms.mongoClient != nil && oms.mongoDatabase != "" {
		var err error
		mongoReferenceKey, err = oms.storeOutputInMongoDB(ctx, interfaceID, inputMessageID, correlationID, result)
		if err != nil {
			// Log error but continue - PostgreSQL storage is still valuable
			fmt.Printf("⚠️  Failed to store output in MongoDB: %v\n", err)
		}
	}

	// STEP 2: Store metadata in PostgreSQL
	return oms.storeOutputInPostgreSQL(interfaceID, inputMessageID, correlationID, mongoReferenceKey, result)
}

// storeOutputInMongoDB stores ONLY FHIR bundle in MongoDB with lineage to input message
func (oms *OutputMessageService) storeOutputInMongoDB(
	ctx context.Context,
	interfaceID string,
	inputMessageID string,
	correlationID string,
	result *TransformationResult,
) (string, error) {
	collectionName := fmt.Sprintf("transformed_messages_intf_%s", strings.ReplaceAll(interfaceID, "-", "_"))
	collection := oms.mongoClient.Database(oms.mongoDatabase).Collection(collectionName)

	// Generate reference key for cross-database correlation
	referenceKey := fmt.Sprintf("mongo_%s_%s", interfaceID, inputMessageID)

	// Prepare document with ONLY FHIR output + lineage (NO duplicate input content)
	doc := bson.M{
		"_id":               referenceKey,

		// LINEAGE: Reference to input message (DO NOT duplicate input content)
		"input_message_id":  inputMessageID,
		"correlation_id":    correlationID,
		"interface_id":      interfaceID,
		"input_collection":  fmt.Sprintf("raw_messages_%s", interfaceID),

		// OUTPUT: FHIR bundle ONLY (the actual transformation result)
		"fhir_bundle":       result.TransformedMessage,

		// METADATA: Transformation details (NOT input HL7)
		"transformation_metadata": result.TransformationMetadata,
		"validation_errors":       result.ValidationErrors,

		// Resource identification from FHIR bundle
		"fhir_resource_type": result.FHIRResourceType,
		"fhir_resource_id":   result.FHIRResourceID,

		// Processing metrics
		"transformation_time_ms": result.ProcessingTimeMs,
		"success":                result.Success,
		"error_message":          result.ErrorMessage,

		// Audit timestamps
		"transformed_at": time.Now(),
	}

	_, err := collection.InsertOne(ctx, doc)
	if err != nil {
		return "", fmt.Errorf("failed to insert transformed message into MongoDB: %w", err)
	}

	fmt.Printf("✅ Stored FHIR bundle in MongoDB (lineage: input=%s, output=%s)\n", inputMessageID, referenceKey)
	return referenceKey, nil
}

// storeOutputInPostgreSQL stores metadata and delivery tracking in PostgreSQL
func (oms *OutputMessageService) storeOutputInPostgreSQL(
	interfaceID string,
	inputMessageID string,
	correlationID string,
	mongoReferenceKey string,
	result *TransformationResult,
) error {
	if oms.db == nil {
		return fmt.Errorf("database connection not available")
	}

	// Generate interface-specific output table name
	outputTableName, err := oms.getOrCreateOutputTable(interfaceID)
	if err != nil {
		return fmt.Errorf("failed to get output table: %w", err)
	}

	// Prepare transformation metadata with MongoDB reference
	transformationMetadata := result.TransformationMetadata
	if transformationMetadata == nil {
		transformationMetadata = make(map[string]interface{})
	}
	transformationMetadata["processing_time_ms"] = result.ProcessingTimeMs
	transformationMetadata["success"] = result.Success
	if mongoReferenceKey != "" {
		transformationMetadata["mongo_reference_key"] = mongoReferenceKey
		transformationMetadata["full_content_location"] = "mongodb"
	}

	// For hybrid storage: Store only summary in PostgreSQL, full bundle in MongoDB
	var transformedMessageJSON []byte = []byte("{}")
	if mongoReferenceKey != "" {
		// Just store reference and summary
		summary := map[string]interface{}{
			"mongo_reference": mongoReferenceKey,
			"resource_type":   result.FHIRResourceType,
			"resource_id":     result.FHIRResourceID,
			"message":         "Full FHIR bundle stored in MongoDB",
		}
		transformedMessageJSON, err = json.Marshal(summary)
		if err != nil {
			return fmt.Errorf("failed to marshal summary: %w", err)
		}
	} else if result.TransformedMessage != nil && len(result.TransformedMessage) > 0 {
		// Fallback: Store full message in PostgreSQL if MongoDB unavailable
		transformedMessageJSON, err = json.Marshal(result.TransformedMessage)
		if err != nil {
			return fmt.Errorf("failed to marshal transformed message: %w", err)
		}
	}

	var transformationMetadataJSON []byte = []byte("{}")
	if transformationMetadata != nil && len(transformationMetadata) > 0 {
		transformationMetadataJSON, err = json.Marshal(transformationMetadata)
		if err != nil {
			return fmt.Errorf("failed to marshal transformation metadata: %w", err)
		}
	}

	var validationErrorsJSON []byte = []byte("null")
	if result.ValidationErrors != nil && len(result.ValidationErrors) > 0 {
		validationErrorsJSON, err = json.Marshal(result.ValidationErrors)
		if err != nil {
			return fmt.Errorf("failed to marshal validation errors: %w", err)
		}
	}

	// Determine status based on result
	status := "transformed"
	validationStatus := "valid"
	if !result.Success {
		status = "failed"
		validationStatus = "invalid"
	} else if result.ValidationErrors != nil && len(result.ValidationErrors) > 0 {
		validationStatus = "warning"
	}

	// Calculate output message size
	outputMessageSize := len(transformedMessageJSON)

	// Extract message type safely from metadata
	messageType := "unknown"
	if result.TransformationMetadata != nil {
		if mt, ok := result.TransformationMetadata["message_type"]; ok {
			if mtStr, ok := mt.(string); ok {
				messageType = mtStr
			}
		}
	}

	// Insert transformation result using standardized schema
	query := fmt.Sprintf(`
		INSERT INTO %s (
			id, interface_id, input_message_id, correlation_id,
			message_type, status, priority,
			transformed_message, transformation_metadata, target_format, target_encoding,
			output_message_size, fhir_resource_type, fhir_resource_id,
			transformation_started_at, transformation_completed_at, transformation_time_ms,
			error_count, last_error_message, validation_status, validation_errors,
			delivery_status, delivery_attempts,
			created_at, updated_at, created_by
		) VALUES (
			gen_random_uuid(), $1, $2, $3,
			$4, $5, $6,
			$7::jsonb, $8::jsonb, $9, $10,
			$11, $12, $13,
			$14, $15, $16,
			$17, $18, $19, $20::jsonb,
			$21, $22,
			CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, $23
		) RETURNING id`, outputTableName)

	var insertedID string
	err = oms.db.QueryRow(query,
		interfaceID,                              // $1
		inputMessageID,                           // $2
		correlationID,                            // $3
		messageType,                              // $4 - Extract from metadata safely
		status,                                   // $5
		5,                                        // $6 - Default priority
		string(transformedMessageJSON),           // $7 - JSONB cast in query
		string(transformationMetadataJSON),       // $8 - JSONB cast in query
		"fhir",                                   // $9 - Default target format
		"UTF-8",                                  // $10 - Default encoding
		outputMessageSize,                        // $11
		result.FHIRResourceType,                  // $12
		result.FHIRResourceID,                    // $13
		time.Now().Add(-time.Duration(result.ProcessingTimeMs)*time.Millisecond), // $14 - Started at
		time.Now(),                               // $15 - Completed at
		result.ProcessingTimeMs,                  // $16
		0,                                        // $17 - Error count (0 for success)
		result.ErrorMessage,                      // $18
		validationStatus,                         // $19
		string(validationErrorsJSON),             // $20 - JSONB cast in query
		"pending",                                // $21 - Default delivery status
		0,                                        // $22 - Delivery attempts
		"system",                                 // $23 - Created by
	).Scan(&insertedID)

	if err != nil {
		return fmt.Errorf("failed to insert transformation result into %s: %w", outputTableName, err)
	}

	return nil
}

// UpdateDeliveryStatus updates the delivery status of an output message
func (oms *OutputMessageService) UpdateDeliveryStatus(interfaceID, inputMessageID, deliveryStatus, deliveryResponse string) error {
	outputTableName, err := oms.getOutputTableName(interfaceID)
	if err != nil {
		return fmt.Errorf("failed to get output table name: %w", err)
	}

	query := fmt.Sprintf(`
		UPDATE %s
		SET delivery_status = $1,
			delivery_attempts = delivery_attempts + 1,
			last_delivery_attempt = CURRENT_TIMESTAMP,
			delivery_response = $2,
			updated_at = CURRENT_TIMESTAMP
		WHERE input_message_id = $3 AND interface_id = $4
	`, outputTableName)

	result, err := oms.db.Exec(query, deliveryStatus, deliveryResponse, inputMessageID, interfaceID)
	if err != nil {
		return fmt.Errorf("failed to update delivery status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("no output message found for input_message_id: %s, interface_id: %s", inputMessageID, interfaceID)
	}

	return nil
}

// GetOutputMessage retrieves an output message by input message ID
func (oms *OutputMessageService) GetOutputMessage(interfaceID, inputMessageID string) (*OutputMessageData, error) {
	outputTableName, err := oms.getOutputTableName(interfaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get output table name: %w", err)
	}

	query := fmt.Sprintf(`
		SELECT id, interface_id, input_message_id, correlation_id,
			   message_type, status, priority,
			   transformed_message, transformation_metadata, target_format, target_encoding,
			   output_message_size, fhir_resource_type, fhir_resource_id,
			   transformation_started_at, transformation_completed_at, transformation_time_ms,
			   error_count, last_error_message, validation_status, validation_errors,
			   delivery_status, delivery_attempts, delivery_endpoint,
			   last_delivery_attempt, delivery_response,
			   created_at, created_by
		FROM %s
		WHERE input_message_id = $1 AND interface_id = $2
		ORDER BY created_at DESC
		LIMIT 1
	`, outputTableName)

	var outputMsg OutputMessageData
	var transformedMessageJSON, transformationMetadataJSON, validationErrorsJSON sql.NullString
	var lastDeliveryAttempt sql.NullTime

	err = oms.db.QueryRow(query, inputMessageID, interfaceID).Scan(
		&outputMsg.ID, &outputMsg.InterfaceID, &outputMsg.InputMessageID, &outputMsg.CorrelationID,
		&outputMsg.MessageType, &outputMsg.Status, &outputMsg.Priority,
		&transformedMessageJSON, &transformationMetadataJSON, &outputMsg.TargetFormat, &outputMsg.TargetEncoding,
		&outputMsg.OutputMessageSize, &outputMsg.FHIRResourceType, &outputMsg.FHIRResourceID,
		&outputMsg.TransformationStartedAt, &outputMsg.TransformationCompletedAt, &outputMsg.TransformationTimeMs,
		&outputMsg.ErrorCount, &outputMsg.LastErrorMessage, &outputMsg.ValidationStatus, &validationErrorsJSON,
		&outputMsg.DeliveryStatus, &outputMsg.DeliveryAttempts, &outputMsg.DeliveryEndpoint,
		&lastDeliveryAttempt, &outputMsg.DeliveryResponse,
		&outputMsg.CreatedAt, &outputMsg.CreatedBy,
	)

	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("output message not found for input_message_id: %s", inputMessageID)
		}
		return nil, fmt.Errorf("failed to retrieve output message: %w", err)
	}

	// Parse JSON fields
	if transformedMessageJSON.Valid {
		if err := json.Unmarshal([]byte(transformedMessageJSON.String), &outputMsg.TransformedMessage); err != nil {
			return nil, fmt.Errorf("failed to parse transformed message JSON: %w", err)
		}
	}

	if transformationMetadataJSON.Valid {
		if err := json.Unmarshal([]byte(transformationMetadataJSON.String), &outputMsg.TransformationMetadata); err != nil {
			return nil, fmt.Errorf("failed to parse transformation metadata JSON: %w", err)
		}
	}

	if validationErrorsJSON.Valid {
		if err := json.Unmarshal([]byte(validationErrorsJSON.String), &outputMsg.ValidationErrors); err != nil {
			return nil, fmt.Errorf("failed to parse validation errors JSON: %w", err)
		}
	}

	if lastDeliveryAttempt.Valid {
		outputMsg.LastDeliveryAttempt = lastDeliveryAttempt.Time
	}

	return &outputMsg, nil
}

// GetOutputMessageCount returns the total number of output messages for an interface
func (oms *OutputMessageService) GetOutputMessageCount(interfaceID string) (int, error) {
	outputTableName, err := oms.getOutputTableName(interfaceID)
	if err != nil {
		return 0, fmt.Errorf("failed to get output table name: %w", err)
	}

	var count int
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE interface_id = $1", outputTableName)
	err = oms.db.QueryRow(query, interfaceID).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to get output message count: %w", err)
	}

	return count, nil
}

// getOutputTableName generates the standardized interface output table name
func (oms *OutputMessageService) getOutputTableName(interfaceID string) (string, error) {
	// Query output_table_metadata for existing table
	var tableName string
	query := `SELECT output_table_name FROM output_table_metadata WHERE interface_id = $1`
	err := oms.db.QueryRow(query, interfaceID).Scan(&tableName)

	if err != nil {
		if err == sql.ErrNoRows {
			return "", fmt.Errorf("no output table found for interface %s", interfaceID)
		}
		return "", fmt.Errorf("failed to query output table metadata: %w", err)
	}

	return tableName, nil
}

// getOrCreateOutputTable gets existing output table name or creates a new one
func (oms *OutputMessageService) getOrCreateOutputTable(interfaceID string) (string, error) {
	// First try to get existing table
	tableName, err := oms.getOutputTableName(interfaceID)
	if err == nil {
		return tableName, nil
	}

	// If table doesn't exist, create it using PostgreSQL function
	parsedUUID, err := uuid.Parse(interfaceID)
	if err != nil {
		return "", fmt.Errorf("invalid interface ID format: %w", err)
	}

	// Call PostgreSQL function to create output table
	var createdTableName string
	query := `SELECT get_interface_output_table($1)`
	err = oms.db.QueryRow(query, parsedUUID).Scan(&createdTableName)
	if err != nil {
		return "", fmt.Errorf("failed to create output table: %w", err)
	}

	return createdTableName, nil
}

// GetInterfaceOutputStats returns statistics for an interface's output messages
func (oms *OutputMessageService) GetInterfaceOutputStats(interfaceID string) (map[string]interface{}, error) {
	outputTableName, err := oms.getOutputTableName(interfaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get output table name: %w", err)
	}

	query := fmt.Sprintf(`
		SELECT
			COUNT(*) as total_messages,
			COUNT(CASE WHEN status = 'transformed' THEN 1 END) as successful_transformations,
			COUNT(CASE WHEN status = 'failed' THEN 1 END) as failed_transformations,
			COUNT(CASE WHEN delivery_status = 'delivered' THEN 1 END) as delivered_messages,
			COUNT(CASE WHEN delivery_status = 'pending' THEN 1 END) as pending_deliveries,
			COUNT(CASE WHEN delivery_status = 'failed' THEN 1 END) as failed_deliveries,
			AVG(transformation_time_ms) as avg_transformation_time_ms,
			AVG(output_message_size) as avg_output_size_bytes
		FROM %s
		WHERE interface_id = $1
	`, outputTableName)

	var stats map[string]interface{} = make(map[string]interface{})
	var totalMessages, successfulTransformations, failedTransformations int
	var deliveredMessages, pendingDeliveries, failedDeliveries int
	var avgTransformationTime sql.NullFloat64
	var avgOutputSize sql.NullFloat64

	err = oms.db.QueryRow(query, interfaceID).Scan(
		&totalMessages, &successfulTransformations, &failedTransformations,
		&deliveredMessages, &pendingDeliveries, &failedDeliveries,
		&avgTransformationTime, &avgOutputSize,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get output stats: %w", err)
	}

	stats["total_messages"] = totalMessages
	stats["successful_transformations"] = successfulTransformations
	stats["failed_transformations"] = failedTransformations
	stats["delivered_messages"] = deliveredMessages
	stats["pending_deliveries"] = pendingDeliveries
	stats["failed_deliveries"] = failedDeliveries

	if avgTransformationTime.Valid {
		stats["avg_transformation_time_ms"] = avgTransformationTime.Float64
	}
	if avgOutputSize.Valid {
		stats["avg_output_size_bytes"] = avgOutputSize.Float64
	}

	// Calculate success rates
	if totalMessages > 0 {
		stats["transformation_success_rate"] = float64(successfulTransformations) / float64(totalMessages) * 100
		stats["delivery_success_rate"] = float64(deliveredMessages) / float64(totalMessages) * 100
	}

	return stats, nil
}