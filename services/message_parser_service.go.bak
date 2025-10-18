// services/message_parser_service.go
// Main parsing orchestration service (MVC pattern)

package services

import (
	"context"
	"database/sql"
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

	parserService := &MessageParserService{
		formatDetector:         NewFormatDetector(),
		parserFactory:          NewParserFactory(),
		mongoService:           mongoService,
		postgresService:        postgresService,
		transformationPipeline: transformationPipeline,   // OOB: Auto-configured
		outputMessageService:   outputMessageService,     // OOB: Hybrid storage
	}

	fmt.Printf("✅ Message parser service initialized with MongoDB + PostgreSQL + Transformation Pipeline + Output Storage\n")
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

	log.Printf("🔍 Message Parser: Calling ExecuteTransformation with messageType='%s'", messageType)

	// Read parsed JSON from MongoDB (to get properly serialized map[string]interface{})
	// This avoids Go struct types that can't be accessed as maps
	parsedJSON, err := mps.mongoService.GetParsedContent(ctx, interfaceID, messageID)
	if err != nil {
		log.Printf("❌ Failed to read parsed JSON from MongoDB for message %s: %v", messageID, err)
		return
	}
	if parsedJSON == nil {
		log.Printf("❌ No parsed JSON found in MongoDB for message %s", messageID)
		return
	}

	log.Printf("✅ Retrieved parsed JSON from MongoDB for message %s", messageID)

	// Execute transformation pipeline
	transformResult, err := mps.transformationPipeline.ExecuteTransformation(
		ctx,
		messageID,
		interfaceID,
		messageType,
		parsedJSON,
	)

	if err != nil {
		log.Printf("❌ Transformation failed for message %s: %v", messageID, err)
		return
	}

	if !transformResult.Success {
		log.Printf("❌ Transformation completed with errors for message %s: %v", messageID, transformResult.Error)
		return
	}

	log.Printf("✅ Transformation completed for message %s (success: %t, time: %dms, steps: %d)",
		messageID, transformResult.Success, transformResult.TotalTimeMs, len(transformResult.TransformationLog))

	// STEP 7: Store transformed output in hybrid storage
	if mps.outputMessageService != nil && transformResult.OutputData != nil {
		// Convert transformation log to map for storage
		transformationMetadata := map[string]interface{}{
			"steps":       transformResult.TransformationLog,
			"total_steps": len(transformResult.TransformationLog),
		}

		// Prepare transformation result for storage
		outputResult := &TransformationResult{
			Success:                transformResult.Success,
			TransformedMessage:     transformResult.OutputData,
			TransformationMetadata: transformationMetadata,
			ValidationErrors:       make(map[string]interface{}),
			ProcessingTimeMs:       transformResult.TotalTimeMs,
			ErrorMessage:          transformResult.Error,
		}

		// Extract FHIR resource type and ID if available
		if resourceType, ok := transformResult.OutputData["resourceType"].(string); ok {
			outputResult.FHIRResourceType = resourceType
		}
		if id, ok := transformResult.OutputData["id"].(string); ok {
			outputResult.FHIRResourceID = id
		}

		// Use message ID as correlation ID
		correlationID := messageID

		// Store in hybrid storage (MongoDB + PostgreSQL)
		err := mps.outputMessageService.StoreTransformationResult(
			ctx,
			interfaceID,
			messageID,
			correlationID,
			outputResult,
		)

		if err != nil {
			log.Printf("❌ Failed to store transformation output for message %s: %v", messageID, err)
		} else {
			log.Printf("✅ Stored transformation output for message %s in hybrid storage", messageID)
		}
	}
}
