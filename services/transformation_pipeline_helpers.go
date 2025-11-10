// services/transformation_pipeline_helpers.go
// Helper methods for transformation pipeline service (MVC + OOB pattern)

package services

import (
	"context"
	"ezhealthkonnect/models"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

// GetOrCreateDefaultPipeline gets existing pipeline or creates default one (OOB pattern)
func (tps *TransformationPipelineService) GetOrCreateDefaultPipeline(
	ctx context.Context,
	interfaceID string,
	messageType string,
) (*models.TransformationPipeline, error) {
	// Try to get existing pipeline
	pipeline, err := tps.GetPipeline(ctx, interfaceID, messageType)
	if err == nil && pipeline != nil {
		return pipeline, nil
	}

	// No pipeline found - create default based on message type
	log.Printf("📋 Creating default pipeline for interface %s, message type %s", interfaceID, messageType)

	// Determine source and target formats from interface configuration
	sourceFormat, targetFormat, err := tps.getInterfaceFormats(ctx, interfaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get interface formats: %w", err)
	}

	// Create default pipeline based on format combination
	pipeline = tps.createDefaultPipeline(interfaceID, messageType, sourceFormat, targetFormat)

	// Save pipeline to database
	err = tps.createPipeline(ctx, pipeline)
	if err != nil {
		return nil, fmt.Errorf("failed to create default pipeline: %w", err)
	}

	log.Printf("✅ Default pipeline created: %s (%d steps)", pipeline.PipelineName, len(pipeline.Steps))
	return pipeline, nil
}

// ExecutePipeline executes a transformation pipeline (MVC Controller pattern)
func (tps *TransformationPipelineService) ExecutePipeline(
	ctx context.Context,
	pipeline *models.TransformationPipeline,
	inputData map[string]interface{},
) (*models.TransformationExecutionResult, error) {
	startTime := time.Now()

	result := &models.TransformationExecutionResult{
		PipelineID:    pipeline.ID,
		CorrelationID: uuid.New().String(),
		Status:        "in_progress",
		StartedAt:     startTime,
		Errors:        []models.TransformationError{},
	}

	// Execute steps in sequence
	currentData := inputData
	for i, step := range pipeline.Steps {
		if !step.Enabled {
			log.Printf("⏭️  Skipping disabled step: %s", step.StepName)
			continue
		}

		log.Printf("▶️  Executing step %d/%d: %s (type: %s)", i+1, len(pipeline.Steps), step.StepName, step.StepType)

		// Execute step using executor registry (OOB pattern)
		output, err := tps.executorRegistry.ExecuteStep(ctx, step, currentData)
		if err != nil {
			log.Printf("❌ Step failed: %s - %v", step.StepName, err)

			// Record error
			result.Errors = append(result.Errors, models.TransformationError{
				Step:      step.StepName,
				Message:   err.Error(),
				Timestamp: time.Now(),
			})

			// Handle error based on strategy
			if step.OnErrorStrategy == "fail" || step.Required {
				result.Status = "failed"
				result.CompletedAt = time.Now()
				result.ExecutionTime = time.Since(startTime)
				return result, fmt.Errorf("pipeline failed at step %s: %w", step.StepName, err)
			} else {
				log.Printf("⚠️  Continuing despite error (strategy: %s)", step.OnErrorStrategy)
			}
		} else {
			log.Printf("✅ Step completed: %s", step.StepName)
			currentData = output // Pass output to next step
		}
	}

	// Pipeline completed successfully
	result.Output = currentData
	result.Status = "completed"
	result.CompletedAt = time.Now()
	result.ExecutionTime = time.Since(startTime)

	// Extract delivery payload if present (added by executors)
	if val, exists := currentData["_deliveryPayload"]; exists {
		log.Printf("🔍 [DEBUG] _deliveryPayload exists, type: %T", val)
		if deliveryPayload, ok := val.(*models.DeliveryPayload); ok {
			result.DeliveryPayload = deliveryPayload
			log.Printf("📦 Delivery payload extracted: %s (format: %s, destination: %s)",
				deliveryPayload.PayloadID, deliveryPayload.Format, deliveryPayload.DestinationType)

			// Remove from output (it's metadata, not transformation output)
			delete(currentData, "_deliveryPayload")
		} else {
			log.Printf("⚠️  [DEBUG] Type assertion failed for _deliveryPayload")
		}
	} else {
		log.Printf("⚠️  [DEBUG] _deliveryPayload not found in currentData")
	}

	return result, nil
}

// getInterfaceFormats retrieves source and target formats from interfaces table
func (tps *TransformationPipelineService) getInterfaceFormats(ctx context.Context, interfaceID string) (string, string, error) {
	query := `
		SELECT
			COALESCE(source_type, 'unknown') as source_format,
			COALESCE(target_type, 'unknown') as target_format
		FROM interfaces
		WHERE id = $1
	`

	var sourceFormat, targetFormat string
	err := tps.db.QueryRowContext(ctx, query, interfaceID).Scan(&sourceFormat, &targetFormat)
	if err != nil {
		return "", "", err
	}

	return sourceFormat, targetFormat, nil
}

// createDefaultPipeline creates a default pipeline based on source→target combination (OOB)
func (tps *TransformationPipelineService) createDefaultPipeline(
	interfaceID string,
	messageType string,
	sourceFormat string,
	targetFormat string,
) *models.TransformationPipeline {
	pipelineID := uuid.New().String()
	pipelineName := fmt.Sprintf("Default_%s_to_%s", sourceFormat, targetFormat)

	pipeline := &models.TransformationPipeline{
		ID:           pipelineID,
		InterfaceID:  interfaceID,
		MessageType:  messageType,
		PipelineName: pipelineName,
		Enabled:      true,
		Version:      1,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
		Steps:        []models.TransformationStep{},
	}

	// Add default steps based on format combination (OOB templates)
	if sourceFormat == "hl7v2" || sourceFormat == "tcp" || sourceFormat == "hl7" {
		if targetFormat == "fhir" || targetFormat == "fhir_rest" {
			// HL7→FHIR pipeline
			pipeline.Steps = tps.createHL7ToFHIRSteps(pipelineID, interfaceID, messageType)
		} else {
			// HL7 passthrough
			pipeline.Steps = tps.createPassthroughSteps(pipelineID, interfaceID, messageType)
		}
	} else if sourceFormat == "fhir" || sourceFormat == "fhir_rest" {
		// FHIR passthrough or validation
		pipeline.Steps = tps.createPassthroughSteps(pipelineID, interfaceID, messageType)
	} else {
		// Generic passthrough
		pipeline.Steps = tps.createPassthroughSteps(pipelineID, interfaceID, messageType)
	}

	return pipeline
}

// createHL7ToFHIRSteps creates default steps for HL7→FHIR transformation (OOB template)
func (tps *TransformationPipelineService) createHL7ToFHIRSteps(
	pipelineID string,
	interfaceID string,
	messageType string,
) []models.TransformationStep {
	return []models.TransformationStep{
		{
			ID:          uuid.New().String(),
			PipelineID:  pipelineID,
			StepName:    "HL7 to FHIR Mapping",
			StepType:    "hl7_to_fhir_mapping",
			Sequence:    100,
			Layer:       "core",
			Required:    true,
			TimeoutMs:   10000,
			Enabled:     true,
			Config: map[string]interface{}{
				"mapping_source": "wizard_configuration",
				"interface_id":   interfaceID,
				"message_type":   messageType,
			},
			OnErrorStrategy: "fail",
			ExecutionMode:   "sequential",
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		},
	}
}

// createPassthroughSteps creates default passthrough steps
func (tps *TransformationPipelineService) createPassthroughSteps(
	pipelineID string,
	interfaceID string,
	messageType string,
) []models.TransformationStep {
	return []models.TransformationStep{
		{
			ID:          uuid.New().String(),
			PipelineID:  pipelineID,
			StepName:    "Passthrough",
			StepType:    "passthrough",
			Sequence:    100,
			Layer:       "core",
			Required:    true,
			TimeoutMs:   1000,
			Enabled:     true,
			Config:      map[string]interface{}{},
			OnErrorStrategy: "fail",
			ExecutionMode:   "sequential",
			CreatedAt:       time.Now(),
			UpdatedAt:       time.Now(),
		},
	}
}

// createPipeline saves a new pipeline to the database
func (tps *TransformationPipelineService) createPipeline(ctx context.Context, pipeline *models.TransformationPipeline) error {
	// Insert pipeline
	query := `
		INSERT INTO transformation_pipelines (
			id, interface_id, message_type, pipeline_name, enabled, version, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`

	_, err := tps.db.ExecContext(ctx, query,
		pipeline.ID,
		pipeline.InterfaceID,
		pipeline.MessageType,
		pipeline.PipelineName,
		pipeline.Enabled,
		pipeline.Version,
		pipeline.CreatedAt,
		pipeline.UpdatedAt,
	)

	if err != nil {
		return fmt.Errorf("failed to insert pipeline: %w", err)
	}

	// Insert steps
	for i := range pipeline.Steps {
		err = tps.CreateStep(ctx, &pipeline.Steps[i])
		if err != nil {
			return fmt.Errorf("failed to insert step %s: %w", pipeline.Steps[i].StepName, err)
		}
	}

	return nil
}
