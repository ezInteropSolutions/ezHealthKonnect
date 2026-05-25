// services/transformation_service.go
// Core transformation service that retrieves parsedJSON from MongoDB and executes HL7→FHIR transformation

package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// TransformationService orchestrates the complete transformation flow
type TransformationService struct {
	db                       *sql.DB
	mongoClient              *mongo.Client
	mongoDatabase            string
	transformPipeline        *TransformationPipelineService
	transformServiceV3       *HL7FHIRTransformServiceV3
	outputMessageService     *OutputMessageService
}

// NewTransformationService creates a new transformation service
func NewTransformationService(
	db *sql.DB,
	mongoClient *mongo.Client,
	mongoDatabase string,
) *TransformationService {
	return &TransformationService{
		db:                   db,
		mongoClient:          mongoClient,
		mongoDatabase:        mongoDatabase,
		transformPipeline:    NewTransformationPipelineService(db),
		transformServiceV3:   NewHL7FHIRTransformServiceV3(db),
		outputMessageService: NewOutputMessageService(db, mongoClient, mongoDatabase),
	}
}

// TransformStoredMessage retrieves parsedJSON from MongoDB and executes transformation
func (ts *TransformationService) TransformStoredMessage(
	ctx context.Context,
	interfaceID string,
	messageID string,
) (*TransformationResult, error) {

	log.Printf("🔄 Starting transformation for stored message: %s (interface: %s)", messageID, interfaceID)

	// STEP 1: Retrieve parsedJSON from MongoDB
	parsedJSON, messageType, err := ts.retrieveParsedJSON(ctx, interfaceID, messageID)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve parsed message: %w", err)
	}

	log.Printf("✅ Retrieved parsed JSON from MongoDB (message type: %s)", messageType)

	// STEP 2: Retrieve mapping configuration from interfaces table
	mapping, err := ts.retrieveInterfaceMapping(ctx, interfaceID)
	if err != nil {
		log.Printf("⚠️  No interface-specific mapping found, will use OOB templates: %v", err)
		// This is OK - the transform service will fallback to OOB templates
	} else {
		log.Printf("✅ Retrieved interface-specific mapping from database")
	}

	// STEP 3: Execute transformation using pipeline (which will use mappings)
	transformResult, err := ts.transformPipeline.ExecuteTransformation(
		ctx,
		messageID,
		interfaceID,
		messageType,
		parsedJSON,
	)

	if err != nil {
		return nil, fmt.Errorf("transformation failed: %w", err)
	}

	if !transformResult.Success {
		return nil, fmt.Errorf("transformation completed with errors: %s", transformResult.Error)
	}

	log.Printf("✅ Transformation completed successfully (time: %dms)", transformResult.TotalTimeMs)

	// STEP 4: Store transformed FHIR output in hybrid storage
	outputResult := &TransformationResult{
		Success:                transformResult.Success,
		TransformedMessage:     transformResult.OutputData,
		TransformationMetadata: map[string]interface{}{
			"steps":          transformResult.TransformationLog,
			"total_steps":    len(transformResult.TransformationLog),
			"mapping_source": mapping,
		},
		ValidationErrors: make(map[string]interface{}),
		ProcessingTimeMs: transformResult.TotalTimeMs,
		ErrorMessage:     transformResult.Error,
	}

	// Extract FHIR resource type and ID if available
	if resourceType, ok := transformResult.OutputData["resourceType"].(string); ok {
		outputResult.FHIRResourceType = resourceType
	}
	if id, ok := transformResult.OutputData["id"].(string); ok {
		outputResult.FHIRResourceID = id
	}

	// Store in hybrid storage (MongoDB + PostgreSQL)
	err = ts.outputMessageService.StoreTransformationResult(
		ctx,
		interfaceID,
		messageID,
		messageID, // Use messageID as correlation ID
		outputResult,
	)

	if err != nil {
		log.Printf("⚠️  Failed to store transformation output: %v", err)
		// Don't fail the transformation - we have the result
	} else {
		log.Printf("✅ Stored transformation output in hybrid storage")
	}

	return outputResult, nil
}

// retrieveParsedJSON gets the parsed JSON from MongoDB raw_messages collection
func (ts *TransformationService) retrieveParsedJSON(
	ctx context.Context,
	interfaceID string,
	messageID string,
) (map[string]interface{}, string, error) {

	collectionName := fmt.Sprintf("raw_messages_intf_%s", interfaceID)
	collection := ts.mongoClient.Database(ts.mongoDatabase).Collection(collectionName)

	// Query for the message by message_id
	filter := bson.M{"message_id": messageID}

	var result struct {
		MessageID     string                 `bson:"message_id"`
		ParsedContent map[string]interface{} `bson:"parsed_content"`
		ParsedFormat  string                 `bson:"parsed_format"`
	}

	err := collection.FindOne(ctx, filter).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, "", fmt.Errorf("message not found in MongoDB: %s", messageID)
		}
		return nil, "", fmt.Errorf("failed to query MongoDB: %w", err)
	}

	if result.ParsedContent == nil {
		return nil, "", fmt.Errorf("message %s has not been parsed yet (parsed_content is null)", messageID)
	}

	// Extract message type from parsed content
	messageType := extractMessageTypeFromParsedJSON(result.ParsedContent)

	log.Printf("📥 Retrieved parsedJSON for message %s (format: %s, type: %s)",
		messageID, result.ParsedFormat, messageType)

	return result.ParsedContent, messageType, nil
}

// retrieveInterfaceMapping gets the mapping configuration from interfaces table
func (ts *TransformationService) retrieveInterfaceMapping(
	ctx context.Context,
	interfaceID string,
) (map[string]interface{}, error) {

	query := `
		SELECT transformation_mapping
		FROM interfaces
		WHERE id = $1
		  AND transformation_mapping IS NOT NULL
		  AND transformation_mapping != '{}'
	`

	var mappingJSON string
	err := ts.db.QueryRowContext(ctx, query, interfaceID).Scan(&mappingJSON)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no mapping found for interface: %s", interfaceID)
		}
		return nil, fmt.Errorf("failed to query interface mapping: %w", err)
	}

	var mapping map[string]interface{}
	if err := json.Unmarshal([]byte(mappingJSON), &mapping); err != nil {
		return nil, fmt.Errorf("failed to parse mapping JSON: %w", err)
	}

	return mapping, nil
}

// extractMessageTypeFromParsedJSON extracts message type from the parsed JSON structure
func extractMessageTypeFromParsedJSON(parsedJSON map[string]interface{}) string {
	// Try messageType.name (enhanced format)
	if messageType, ok := parsedJSON["messageType"].(map[string]interface{}); ok {
		if name, ok := messageType["name"].(string); ok {
			return name
		}
	}

	// Try direct messageType string
	if messageType, ok := parsedJSON["messageType"].(string); ok {
		return messageType
	}

	// Try basicSegments.MSH.fields.MSH.9 as fallback
	if basicSegments, ok := parsedJSON["basicSegments"].(map[string]interface{}); ok {
		if msh, ok := basicSegments["MSH"].(map[string]interface{}); ok {
			if fields, ok := msh["fields"].(map[string]interface{}); ok {
				if msh9, ok := fields["MSH.9"].(string); ok {
					return msh9
				}
			}
		}
	}

	return ""
}

// BatchTransformMessages transforms multiple stored messages
func (ts *TransformationService) BatchTransformMessages(
	ctx context.Context,
	interfaceID string,
	messageIDs []string,
) ([]*TransformationResult, error) {

	results := make([]*TransformationResult, 0, len(messageIDs))

	for _, messageID := range messageIDs {
		result, err := ts.TransformStoredMessage(ctx, interfaceID, messageID)
		if err != nil {
			log.Printf("❌ Failed to transform message %s: %v", messageID, err)
			// Continue with next message
			continue
		}
		results = append(results, result)
	}

	log.Printf("✅ Batch transformation completed: %d/%d messages transformed successfully",
		len(results), len(messageIDs))

	return results, nil
}

// TransformInterfaceMessages transforms all messages for an interface that haven't been transformed yet
func (ts *TransformationService) TransformInterfaceMessages(
	ctx context.Context,
	interfaceID string,
	limit int,
) ([]*TransformationResult, error) {

	// Query PostgreSQL for messages that have been parsed but not transformed
	query := `
		SELECT message_id
		FROM messages_intf_` + interfaceID + `
		WHERE parsing_status = 'success'
		  AND (transformation_status IS NULL OR transformation_status = 'pending')
		ORDER BY received_at ASC
	`

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := ts.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to query messages: %w", err)
	}
	defer rows.Close()

	var messageIDs []string
	for rows.Next() {
		var messageID string
		if err := rows.Scan(&messageID); err != nil {
			log.Printf("⚠️  Failed to scan message ID: %v", err)
			continue
		}
		messageIDs = append(messageIDs, messageID)
	}

	log.Printf("📋 Found %d messages to transform for interface %s", len(messageIDs), interfaceID)

	return ts.BatchTransformMessages(ctx, interfaceID, messageIDs)
}

// InitializeTransformationService creates transformation service with OOB MongoDB detection
func InitializeTransformationService(db *sql.DB) *TransformationService {
	// Try to initialize MongoDB connection service (OOB)
	mongoConnService, err := NewMongoDBConnectionService()
	if err != nil {
		fmt.Printf("ℹ️  MongoDB not configured, transformation service unavailable: %v\n", err)
		return nil
	}

	// Connect to MongoDB
	ctx := context.Background()
	err = mongoConnService.Connect(ctx)
	if err != nil {
		fmt.Printf("ℹ️  Failed to connect to MongoDB, transformation service unavailable: %v\n", err)
		return nil
	}

	mongoClient := mongoConnService.GetClient()
	mongoDatabase := mongoConnService.GetDatabase()

	transformService := NewTransformationService(db, mongoClient, mongoDatabase)

	fmt.Printf("✅ Transformation service initialized with MongoDB + PostgreSQL\n")
	return transformService
}
