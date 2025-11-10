// processing/transformation_engine.go
// Core transformation engine for processing messages through transformation pipelines

package processing

import (
	"context"
	"fmt"
	"log"
	"sort"
	"time"

	"ezhealthkonnect/services"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// TransformationEngine handles the execution of transformation pipelines
type TransformationEngine struct {
	mongoClient           *mongo.Client
	database              *mongo.Database
	pipelineCollection    *mongo.Collection
	templateCollection    *mongo.Collection
	valueMapsCollection   *mongo.Collection
	stepProcessors        map[TransformationStepType]StepProcessor
	postgresTransformService *services.PostgresTransformationService
}

// StepProcessor interface for individual transformation step handlers
type StepProcessor interface {
	Process(ctx context.Context, step *TransformationStep, context *TransformationContext) error
	Validate(step *TransformationStep) error
}

// NewTransformationEngine creates a new transformation engine
func NewTransformationEngine(mongoClient *mongo.Client, databaseName string, postgresService *services.PostgresTransformationService) *TransformationEngine {
	database := mongoClient.Database(databaseName)

	engine := &TransformationEngine{
		mongoClient:              mongoClient,
		database:                database,
		pipelineCollection:       database.Collection("transformation_pipelines"),
		templateCollection:       database.Collection("transformation_templates"),
		valueMapsCollection:      database.Collection("value_maps"),
		stepProcessors:          make(map[TransformationStepType]StepProcessor),
		postgresTransformService: postgresService,
	}

	// Register built-in step processors
	engine.registerStepProcessors()

	return engine
}

// registerStepProcessors registers all available step processors
func (engine *TransformationEngine) registerStepProcessors() {
	// Register core processors
	engine.stepProcessors[StepTypeFieldMapping] = &FieldMappingProcessor{}
	engine.stepProcessors[StepTypeValueTransform] = &ValueTransformProcessor{}
	engine.stepProcessors[StepTypeConditionalLogic] = &ConditionalLogicProcessor{}
	engine.stepProcessors[StepTypeValidation] = &ValidationProcessor{}

	// Register specialized processors
	engine.stepProcessors[StepTypeHL7Parse] = &HL7ParseProcessor{}
	engine.stepProcessors[StepTypeFHIRBuild] = &FHIRBuildProcessor{}
	engine.stepProcessors[StepTypeCodeLookup] = &CodeLookupProcessor{engine: engine}

	// Register PostgreSQL-based processors
	if engine.postgresTransformService != nil {
		engine.stepProcessors[StepTypePostgresAtomicMapping] = &PostgresAtomicMappingProcessor{
			postgresService: engine.postgresTransformService,
		}
		log.Printf("✅ PostgreSQL atomic mapping processor registered")
	}

	log.Printf("✅ Transformation engine registered %d step processors", len(engine.stepProcessors))
}

// PostgresAtomicMappingProcessor handles PostgreSQL atomic mapping transformations
type PostgresAtomicMappingProcessor struct {
	postgresService *services.PostgresTransformationService
}

// Process executes PostgreSQL atomic mapping transformation
func (pamp *PostgresAtomicMappingProcessor) Process(ctx context.Context, step *TransformationStep, transformationContext *TransformationContext) error {
	log.Printf("🔄 Starting PostgreSQL atomic mappings transformation step: %s", step.Name)

	if pamp.postgresService == nil {
		return fmt.Errorf("PostgreSQL transformation service not available")
	}

	// Extract interface ID from transformation context
	interfaceID := transformationContext.InterfaceID

	// Extract message type - try multiple sources
	messageType := "ADT^A01" // Default fallback
	if mt, exists := transformationContext.Variables["message_type"]; exists {
		if mtStr, ok := mt.(string); ok {
			messageType = mtStr
		}
	}

	// Get source data from transformation context
	sourceData := transformationContext.Variables

	// Execute PostgreSQL atomic mappings transformation
	result, err := pamp.postgresService.ExecuteTransformation(interfaceID, messageType, sourceData)
	if err != nil {
		return fmt.Errorf("PostgreSQL atomic mappings failed: %w", err)
	}

	// Store results in transformation context
	if result.Success {
		// Update target message with FHIR Patient resource
		transformationContext.TargetMessage = result.TransformedMessage
		transformationContext.Variables["transformation_metadata"] = result.TransformationMetadata
		transformationContext.Variables["fhir_resource_type"] = result.FHIRResourceType
		transformationContext.Variables["fhir_resource_id"] = result.FHIRResourceID

		// Add PostgreSQL-specific metadata to variables
		transformationContext.Variables["postgres_atomic_mappings"] = map[string]interface{}{
			"applied_mappings":   result.TransformationMetadata["mappings_applied"],
			"skipped_mappings":   result.TransformationMetadata["mappings_skipped"],
			"processing_time_ms": result.ProcessingTimeMs,
			"processing_method":  "postgres_atomic_mappings",
			"total_mappings":     result.TransformationMetadata["total_mappings"],
		}

		log.Printf("✅ PostgreSQL atomic mappings completed: %d mappings applied", len(result.TransformationMetadata["mappings_applied"].([]string)))
	} else {
		return fmt.Errorf("PostgreSQL transformation failed with validation errors: %v", result.ValidationErrors)
	}

	return nil
}

// Validate validates the transformation step configuration
func (pamp *PostgresAtomicMappingProcessor) Validate(step *TransformationStep) error {
	if step.StepType != StepTypePostgresAtomicMapping {
		return fmt.Errorf("invalid step type for PostgreSQL atomic mapping processor: %s", step.StepType)
	}

	if pamp.postgresService == nil {
		return fmt.Errorf("PostgreSQL transformation service not initialized")
	}

	return nil
}

// GetTransformationPipeline retrieves a transformation pipeline for an interface
func (engine *TransformationEngine) GetTransformationPipeline(ctx context.Context, interfaceID string) (*TransformationPipeline, error) {
	var pipeline TransformationPipeline

	filter := bson.M{
		"interface_id": interfaceID,
		"is_active":    true,
	}

	err := engine.pipelineCollection.FindOne(ctx, filter).Decode(&pipeline)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("no active transformation pipeline found for interface %s", interfaceID)
		}
		return nil, fmt.Errorf("error retrieving transformation pipeline: %w", err)
	}

	// Sort steps by order
	sort.Slice(pipeline.Steps, func(i, j int) bool {
		return pipeline.Steps[i].Order < pipeline.Steps[j].Order
	})

	return &pipeline, nil
}

// ExecuteTransformation processes a message through a transformation pipeline
func (engine *TransformationEngine) ExecuteTransformation(ctx context.Context, interfaceID string, sourceData map[string]interface{}) (*TransformationResult, error) {
	startTime := time.Now()

	log.Printf("🔄 Starting transformation for interface %s", interfaceID)

	// Get transformation pipeline
	pipeline, err := engine.GetTransformationPipeline(ctx, interfaceID)
	if err != nil {
		return &TransformationResult{
			Success:       false,
			ErrorMessages: []string{fmt.Sprintf("Pipeline retrieval failed: %s", err.Error())},
			ProcessingTime: time.Since(startTime),
		}, err
	}

	log.Printf("📋 Found transformation pipeline '%s' with %d steps", pipeline.Name, len(pipeline.Steps))

	// Initialize transformation context
	transformationContext := &TransformationContext{
		MessageID:        fmt.Sprintf("msg_%d", time.Now().UnixNano()),
		InterfaceID:      interfaceID,
		SourceMessage:    sourceData,
		TargetMessage:    make(map[string]interface{}),
		Variables:        make(map[string]interface{}),
		ErrorMessages:    []string{},
		ProcessedSteps:   []string{},
		StartTime:        startTime,
		CurrentStepIndex: 0,
		IsComplete:       false,
	}

	// Execute transformation steps
	for i, step := range pipeline.Steps {
		if !step.IsActive {
			log.Printf("⏭️  Skipping inactive step: %s", step.Name)
			continue
		}

		transformationContext.CurrentStepIndex = i

		log.Printf("🔧 Executing step %d/%d: %s (%s)", i+1, len(pipeline.Steps), step.Name, step.StepType)

		// Check step conditions
		if !engine.evaluateStepConditions(&step, transformationContext) {
			log.Printf("⏭️  Skipping step due to unmet conditions: %s", step.Name)
			continue
		}

		// Get step processor
		processor, exists := engine.stepProcessors[step.StepType]
		if !exists {
			errorMsg := fmt.Sprintf("No processor found for step type: %s", step.StepType)
			transformationContext.ErrorMessages = append(transformationContext.ErrorMessages, errorMsg)

			if step.OnError.Action == "fail" {
				break
			}
			continue
		}

		// Execute step with error handling
		stepErr := engine.executeStepWithRetry(ctx, processor, &step, transformationContext)
		if stepErr != nil {
			errorMsg := fmt.Sprintf("Step '%s' failed: %s", step.Name, stepErr.Error())
			transformationContext.ErrorMessages = append(transformationContext.ErrorMessages, errorMsg)

			// Handle error based on step configuration
			if step.OnError.Action == "fail" {
				log.Printf("❌ Step failed, stopping transformation: %s", step.Name)
				break
			} else if step.OnError.Action == "default" && step.OnError.DefaultValue != nil {
				log.Printf("⚠️  Step failed, using default value: %s", step.Name)
				// Apply default value logic here
			} else {
				log.Printf("⚠️  Step failed, continuing: %s", step.Name)
			}
		} else {
			transformationContext.ProcessedSteps = append(transformationContext.ProcessedSteps, step.ID)
			log.Printf("✅ Step completed: %s", step.Name)
		}
	}

	// Mark transformation as complete
	transformationContext.IsComplete = true
	processingTime := time.Since(startTime)

	// Build result
	result := &TransformationResult{
		Success:         len(transformationContext.ErrorMessages) == 0,
		TransformedData: transformationContext.TargetMessage,
		ErrorMessages:   transformationContext.ErrorMessages,
		ProcessingTime:  processingTime,
		StepsExecuted:   len(transformationContext.ProcessedSteps),
		Context:         transformationContext,
	}

	if result.Success {
		log.Printf("✅ Transformation completed successfully in %v (%d steps)", processingTime, result.StepsExecuted)
	} else {
		log.Printf("❌ Transformation completed with errors in %v (%d errors)", processingTime, len(result.ErrorMessages))
	}

	return result, nil
}

// executeStepWithRetry executes a step with retry logic
func (engine *TransformationEngine) executeStepWithRetry(ctx context.Context, processor StepProcessor, step *TransformationStep, context *TransformationContext) error {
	maxRetries := step.OnError.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 1
	}

	var lastError error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := processor.Process(ctx, step, context)
		if err == nil {
			return nil
		}

		lastError = err
		if attempt < maxRetries {
			log.Printf("⚠️  Step '%s' attempt %d/%d failed, retrying: %s", step.Name, attempt, maxRetries, err.Error())
			time.Sleep(time.Duration(attempt) * time.Second) // Exponential backoff
		}
	}

	return lastError
}

// evaluateStepConditions checks if step conditions are met
func (engine *TransformationEngine) evaluateStepConditions(step *TransformationStep, context *TransformationContext) bool {
	if len(step.Conditions) == 0 {
		return true
	}

	// Simple condition evaluation (can be enhanced)
	for _, condition := range step.Conditions {
		fieldValue := engine.getFieldValue(condition.Field, context)

		conditionMet := false
		switch condition.Operator {
		case "eq":
			conditionMet = fieldValue == condition.Value
		case "ne":
			conditionMet = fieldValue != condition.Value
		case "exists":
			conditionMet = fieldValue != nil
		case "contains":
			if str, ok := fieldValue.(string); ok {
				if searchStr, ok := condition.Value.(string); ok {
					conditionMet = contains(str, searchStr)
				}
			}
		}

		if !conditionMet {
			return false
		}
	}

	return true
}

// getFieldValue retrieves a field value from the transformation context
func (engine *TransformationEngine) getFieldValue(fieldPath string, context *TransformationContext) interface{} {
	// Simple field path resolution (can be enhanced with JSONPath)
	if value, exists := context.SourceMessage[fieldPath]; exists {
		return value
	}
	if value, exists := context.Variables[fieldPath]; exists {
		return value
	}
	return nil
}

// GetValueMap retrieves a value map by ID
func (engine *TransformationEngine) GetValueMap(ctx context.Context, valueMapID string) (*ValueMap, error) {
	var valueMap ValueMap

	filter := bson.M{
		"_id":       valueMapID,
		"is_active": true,
	}

	err := engine.valueMapsCollection.FindOne(ctx, filter).Decode(&valueMap)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, fmt.Errorf("value map not found: %s", valueMapID)
		}
		return nil, fmt.Errorf("error retrieving value map: %w", err)
	}

	return &valueMap, nil
}

// SaveTransformationPipeline saves a transformation pipeline to MongoDB
func (engine *TransformationEngine) SaveTransformationPipeline(ctx context.Context, pipeline *TransformationPipeline) error {
	pipeline.UpdatedAt = time.Now()

	if pipeline.ID.IsZero() {
		pipeline.CreatedAt = time.Now()
		result, err := engine.pipelineCollection.InsertOne(ctx, pipeline)
		if err != nil {
			return fmt.Errorf("failed to create transformation pipeline: %w", err)
		}
		pipeline.ID = result.InsertedID.(primitive.ObjectID)
	} else {
		_, err := engine.pipelineCollection.ReplaceOne(ctx, bson.M{"_id": pipeline.ID}, pipeline)
		if err != nil {
			return fmt.Errorf("failed to update transformation pipeline: %w", err)
		}
	}

	log.Printf("✅ Saved transformation pipeline: %s", pipeline.Name)
	return nil
}
