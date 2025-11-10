// services/message_parser_service.go
// Main parsing orchestration service (MVC pattern)

package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"ezhealthkonnect/models"
)

// MessageParserService orchestrates message parsing (OOB workflow)
type MessageParserService struct {
	formatDetector         *FormatDetector
	parserFactory          *ParserFactory
	mongoService           *MongoDBMessageService
	postgresService        *InterfaceMessageService
	transformationPipeline *TransformationPipelineService // OOB: Auto-configured transformation
	outputMessageService   *OutputMessageService          // OOB: Hybrid output storage
	outputDeliveryService  *OutputDeliveryService         // V21: Push-based delivery
	db                     *sql.DB                        // Database for querying interface config
}

// NewMessageParserService creates new parser service (OOB)
func NewMessageParserService(
	mongoService *MongoDBMessageService,
	postgresService *InterfaceMessageService,
) *MessageParserService {
	return &MessageParserService{
		formatDetector:  NewFormatDetector(),  // OOB: Auto-create
		parserFactory:   NewParserFactory(),   // OOB: Auto-register parsers
		mongoService:    mongoService,
		postgresService: postgresService,
	}
}

// ParseToJSON is the main entry point (OOB workflow)
func (mps *MessageParserService) ParseToJSON(
	ctx context.Context,
	messageID string,
	interfaceID string,
	rawContent string,
) (*models.ParserResult, error) {

	startTime := time.Now()
	log.Printf("🔄 Starting JSON conversion for message: %s", messageID)

	// STEP 1: Auto-detect format (OOB)
	detection := mps.formatDetector.DetectFormat(rawContent)
	log.Printf("📊 Format detected: %s (confidence: %.2f)", detection.DetectedFormat, detection.Confidence)

	// STEP 2: Get appropriate parser (OOB)
	parser, err := mps.parserFactory.GetParser(detection.DetectedFormat)
	if err != nil {
		return nil, fmt.Errorf("parser not available for format %s: %w", detection.DetectedFormat, err)
	}

	// STEP 3: Parse to JSON
	result, err := parser.Parse(rawContent)
	if err != nil {
		// Update status as failed
		mps.updateParsingStatus(ctx, interfaceID, messageID, &MessageStatusUpdate{
			Status:        "parsing_failed",
			ParsingStatus: "failed",
			ParsingError:  err.Error(),
		})
		return nil, fmt.Errorf("parsing failed: %w", err)
	}

	result.ParsingTime = time.Since(startTime)
	log.Printf("✅ Parsing completed in %dms", result.ParsingTime.Milliseconds())

	// STEP 4: Store parsed JSON in MongoDB
	if err := mps.storeParsedContent(ctx, interfaceID, messageID, result); err != nil {
		log.Printf("⚠️  Failed to store parsed content: %v", err)
		// Don't fail the entire operation - parsing succeeded
	}

	// STEP 5: Update status in PostgreSQL
	if err := mps.updateParsingStatus(ctx, interfaceID, messageID, &MessageStatusUpdate{
		Status:         "parsed",
		MessageType:    result.Metadata.MessageType,
		ParsedAt:       result.Metadata.ParsedAt,
		ParsingStatus:  "success",
		ParsingTimeMs:  int(result.ParsingTime.Milliseconds()),
		ParsingError:   "",
	}); err != nil {
		log.Printf("⚠️  Failed to update parsing status: %v", err)
	}

	// STEP 6: Trigger transformation pipeline (OOB - connector-agnostic)
	if mps.transformationPipeline != nil {
		go mps.executeTransformationPipeline(ctx, messageID, interfaceID, result)
	}

	return result, nil
}

// storeParsedContent updates MongoDB with parsed JSON
func (mps *MessageParserService) storeParsedContent(
	ctx context.Context,
	interfaceID string,
	messageID string,
	result *models.ParserResult,
) error {
	return mps.mongoService.UpdateParsedContent(ctx, interfaceID, messageID, &ParsedContentUpdate{
		ParsedContent:    result.ParsedJSON,
		ParsedAt:         result.Metadata.ParsedAt,
		ParsingTimeMs:    int(result.ParsingTime.Milliseconds()),
		Format:           string(result.Format),
		MessageType:      result.Metadata.MessageType,
		ValidationResult: result.ValidationResult,
	})
}

// updateParsingStatus updates PostgreSQL with parsing status
func (mps *MessageParserService) updateParsingStatus(
	ctx context.Context,
	interfaceID string,
	messageID string,
	update *MessageStatusUpdate,
) error {
	return mps.postgresService.UpdateMessageParsingStatus(interfaceID, messageID, update)
}

// InitializeMessageParserService creates parser service with OOB MongoDB detection
func InitializeMessageParserService(db *sql.DB) *MessageParserService {
	// Try to initialize MongoDB connection service (OOB)
	mongoConnService, err := NewMongoDBConnectionService()
	if err != nil {
		fmt.Printf("ℹ️  MongoDB not configured, JSON parser service unavailable: %v\n", err)
		return nil
	}

	// Connect to MongoDB
	ctx := context.Background()
	err = mongoConnService.Connect(ctx)
	if err != nil {
		fmt.Printf("ℹ️  Failed to connect to MongoDB, JSON parser service unavailable: %v\n", err)
		return nil
	}

	mongoClient := mongoConnService.GetClient()
	mongoDatabase := mongoConnService.GetDatabase()

	// Initialize both MongoDB and PostgreSQL services
	mongoService := NewMongoDBMessageService(mongoClient, mongoDatabase)
	postgresService := NewInterfaceMessageService(db)

	// OOB: Initialize transformation pipeline service
	transformationPipeline := NewTransformationPipelineService(db)

	// OOB: Initialize hybrid output message service
	outputMessageService := NewOutputMessageService(db, mongoClient, mongoDatabase)

	// V21: Initialize output delivery service (push-based delivery)
	outputDeliveryService := NewOutputDeliveryService(db)

	parserService := &MessageParserService{
		formatDetector:         NewFormatDetector(),
		parserFactory:          NewParserFactory(),
		mongoService:           mongoService,
		postgresService:        postgresService,
		transformationPipeline: transformationPipeline,     // OOB: Auto-configured
		outputMessageService:   outputMessageService,       // OOB: Hybrid storage
		outputDeliveryService:  outputDeliveryService,      // V21: Push-based delivery
		db:                     db,                         // V21: For querying interface config
	}

	fmt.Printf("✅ Message parser service initialized with MongoDB + PostgreSQL + Transformation Pipeline + Output Storage + Delivery\n")
	return parserService
}

// executeTransformationPipeline triggers transformation pipeline asynchronously (OOB)
func (mps *MessageParserService) executeTransformationPipeline(
	ctx context.Context,
	messageID string,
	interfaceID string,
	result *models.ParserResult,
) {
	// Add panic recovery for goroutine
	defer func() {
		if r := recover(); r != nil {
			log.Printf("❌ PANIC in executeTransformationPipeline for message %s: %v", messageID, r)
		}
	}()

	log.Printf("🔄 Starting transformation pipeline for message %s", messageID)

	// Safety check
	if result == nil {
		log.Printf("❌ ERROR: result is nil for message %s", messageID)
		return
	}

	// Extract message type from parser result
	messageType := result.Metadata.MessageType
	log.Printf("🔍 DEBUG: Extracted messageType='%s' from result.Metadata", messageType)

	if messageType == "" {
		log.Printf("⚠️  No message type found, skipping transformation for message %s", messageID)
		return
	}

	// DISABLED: Legacy transformation path
	// MVC Pipeline in engine_message_processor.go now handles ALL transformation
	// This duplicate transformation was causing the executor to receive incomplete responses
	log.Printf("📝 Skipping legacy transformation - MVC pipeline handles transformation for message %s", messageID)
}

// getInterfaceTargetConfig retrieves target configuration and output message ID for delivery
// V21: Helper function for output delivery trigger
func (mps *MessageParserService) getInterfaceTargetConfig(interfaceID, inputMessageID string) (*models.DestinationConfig, string, error) {
	ctx := context.Background()

	// Query interface target configuration
	query := `
		SELECT target_config
		FROM interfaces
		WHERE id = $1
	`

	var targetConfigJSON string
	err := mps.db.QueryRowContext(ctx, query, interfaceID).Scan(&targetConfigJSON)
	if err != nil {
		return nil, "", fmt.Errorf("failed to query interface target_config: %w", err)
	}

	// Parse target_config JSON
	var targetConfig models.DestinationConfig
	err = json.Unmarshal([]byte(targetConfigJSON), &targetConfig)
	if err != nil {
		return nil, "", fmt.Errorf("failed to parse target_config JSON: %w", err)
	}

	// Get output message ID from output table
	// First, get output table name from metadata
	metadataQuery := `
		SELECT output_table_name
		FROM output_table_metadata
		WHERE interface_id = $1
	`

	var outputTableName string
	err = mps.db.QueryRowContext(ctx, metadataQuery, interfaceID).Scan(&outputTableName)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get output table name: %w", err)
	}

	// Query output message ID from output table
	// Find the most recent output message for this input message
	outputMessageQuery := fmt.Sprintf(`
		SELECT id
		FROM %s
		WHERE input_message_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`, outputTableName)

	var outputMessageID string
	err = mps.db.QueryRowContext(ctx, outputMessageQuery, inputMessageID).Scan(&outputMessageID)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get output message ID: %w", err)
	}

	return &targetConfig, outputMessageID, nil
}
