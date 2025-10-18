package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"ezhealthkonnect/models"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

// TransformationPipelineService handles transformation pipeline operations
type TransformationPipelineService struct {
	db        *sql.DB
	executors map[string]TransformationExecutor
}

// TransformationExecutor interface for step execution
type TransformationExecutor interface {
	Execute(ctx context.Context, step models.TransformationStep, input map[string]interface{}) (map[string]interface{}, error)
	GetSupportedType() string
}

// NewTransformationPipelineService creates a new transformation pipeline service
func NewTransformationPipelineService(db *sql.DB) *TransformationPipelineService {
	service := &TransformationPipelineService{
		db:        db,
		executors: make(map[string]TransformationExecutor),
	}

	// Register executors
	service.RegisterExecutor(NewValidationExecutor())
	service.RegisterExecutor(NewHL7ToFHIRMappingExecutor(db))
	service.RegisterExecutor(NewFHIRValidationExecutor())
	service.RegisterExecutor(NewCustomScriptExecutor())

	return service
}

// RegisterExecutor registers a transformation executor
func (tps *TransformationPipelineService) RegisterExecutor(executor TransformationExecutor) {
	tps.executors[executor.GetSupportedType()] = executor
}

// GetPipeline retrieves a transformation pipeline for an interface/message type
func (tps *TransformationPipelineService) GetPipeline(ctx context.Context, interfaceID string, messageType string) (*models.TransformationPipeline, error) {
	query := `
		SELECT id, interface_id, message_type, pipeline_name, enabled, version, created_at, updated_at
		FROM transformation_pipelines
		WHERE interface_id = $1 AND message_type = $2 AND enabled = true
		LIMIT 1
	`

	pipeline := &models.TransformationPipeline{}
	err := tps.db.QueryRowContext(ctx, query, interfaceID, messageType).Scan(
		&pipeline.ID,
		&pipeline.InterfaceID,
		&pipeline.MessageType,
		&pipeline.PipelineName,
		&pipeline.Enabled,
		&pipeline.Version,
		&pipeline.CreatedAt,
		&pipeline.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get pipeline: %w", err)
	}

	// Load steps
	steps, err := tps.GetPipelineSteps(ctx, pipeline.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get pipeline steps: %w", err)
	}

	pipeline.Steps = steps

	return pipeline, nil
}

// GetPipelineSteps retrieves all steps for a pipeline, ordered by layer and sequence
func (tps *TransformationPipelineService) GetPipelineSteps(ctx context.Context, pipelineID string) ([]models.TransformationStep, error) {
	query := `
		SELECT id, pipeline_id, step_name, step_type, sequence, layer, required, timeout_ms,
		       enabled, config, script_type, script_content, on_error_strategy, execution_mode,
		       created_at, updated_at
		FROM transformation_steps
		WHERE pipeline_id = $1 AND enabled = true
		ORDER BY
		    CASE layer
		        WHEN 'pre' THEN 1
		        WHEN 'core' THEN 2
		        WHEN 'post' THEN 3
		        ELSE 4
		    END,
		    sequence ASC
	`

	rows, err := tps.db.QueryContext(ctx, query, pipelineID)
	if err != nil {
		return nil, fmt.Errorf("failed to query steps: %w", err)
	}
	defer rows.Close()

	steps := []models.TransformationStep{}
	for rows.Next() {
		var step models.TransformationStep
		var configJSON []byte

		err := rows.Scan(
			&step.ID,
			&step.PipelineID,
			&step.StepName,
			&step.StepType,
			&step.Sequence,
			&step.Layer,
			&step.Required,
			&step.TimeoutMs,
			&step.Enabled,
			&configJSON,
			&step.ScriptType,
			&step.ScriptContent,
			&step.OnErrorStrategy,
			&step.ExecutionMode,
			&step.CreatedAt,
			&step.UpdatedAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan step: %w", err)
		}

		// Unmarshal config
		if len(configJSON) > 0 {
			if err := json.Unmarshal(configJSON, &step.Config); err != nil {
				return nil, fmt.Errorf("failed to unmarshal step config: %w", err)
			}
		} else {
			step.Config = make(map[string]interface{})
		}

		steps = append(steps, step)
	}

	return steps, nil
}

// ExecuteTransformation executes a transformation pipeline
func (tps *TransformationPipelineService) ExecuteTransformation(
	ctx context.Context,
	messageID string,
	interfaceID string,
	messageType string,
	parsedJSON map[string]interface{},
) (*models.TransformationResult, error) {

	startTime := time.Now()

	// Get pipeline
	pipeline, err := tps.GetPipeline(ctx, interfaceID, messageType)
	if err != nil {
		log.Printf("❌ GetPipeline FAILED for interface=%s, messageType=%s: %v", interfaceID, messageType, err)
		return nil, fmt.Errorf("failed to get pipeline: %w", err)
	}

	log.Printf("✅ GetPipeline SUCCESS: found %d steps for interface=%s, messageType=%s", len(pipeline.Steps), interfaceID, messageType)

	// Inject runtime context into each step's config (OOB: auto-configure steps)
	for i := range pipeline.Steps {
		if pipeline.Steps[i].Config == nil {
			pipeline.Steps[i].Config = make(map[string]interface{})
		}
		pipeline.Steps[i].Config["interface_id"] = interfaceID
		pipeline.Steps[i].Config["message_type"] = messageType
		pipeline.Steps[i].Config["message_id"] = messageID

		// DEBUG
		log.Printf("🔍 Pipeline: Injected into step %d (%s): interface_id=%s, message_type=%s",
			i, pipeline.Steps[i].StepName, interfaceID, messageType)
	}

	// Create execution record
	executionID := uuid.New().String()
	execution := &models.TransformationExecution{
		ID:          executionID,
		MessageID:   messageID,
		InterfaceID: interfaceID,
		PipelineID:  pipeline.ID,
		StartedAt:   startTime,
		Status:      "running",
		InputData:   parsedJSON,
	}

	if err := tps.createExecutionRecord(ctx, execution); err != nil {
		fmt.Printf("⚠️  Failed to create execution record: %v\n", err)
	}

	// Execute pipeline
	result, err := tps.executePipeline(ctx, pipeline, parsedJSON)

	// Update execution record
	completedAt := time.Now()
	execution.CompletedAt = &completedAt
	execution.TotalTimeMs = completedAt.Sub(startTime).Milliseconds()
	execution.OutputData = result.OutputData
	execution.ExecutionLog = result.TransformationLog

	if err != nil {
		execution.Status = "failed"
		execution.ErrorMessage = err.Error()
		execution.StepsFailed = len(result.TransformationLog)
	} else {
		execution.Status = "completed"
		execution.StepsExecuted = len(result.TransformationLog)
	}

	if updateErr := tps.updateExecutionRecord(ctx, execution); updateErr != nil {
		fmt.Printf("⚠️  Failed to update execution record: %v\n", updateErr)
	}

	return result, err
}

// executePipeline executes all steps in a pipeline
func (tps *TransformationPipelineService) executePipeline(
	ctx context.Context,
	pipeline *models.TransformationPipeline,
	input map[string]interface{},
) (*models.TransformationResult, error) {

	result := &models.TransformationResult{
		Success:           true,
		OutputData:        input,
		TransformationLog: []models.StepExecutionLog{},
		TotalTimeMs:       0,
	}

	currentData := input

	// Execute steps in order (already ordered by layer + sequence)
	for _, step := range pipeline.Steps {
		stepStartTime := time.Now()

		fmt.Printf("🔄 Executing step: %s (%s)\n", step.StepName, step.StepType)

		// Get executor for step type
		executor, exists := tps.executors[step.StepType]
		if !exists {
			return nil, fmt.Errorf("no executor registered for step type: %s", step.StepType)
		}

		// Execute step with timeout
		stepCtx, cancel := context.WithTimeout(ctx, time.Duration(step.TimeoutMs)*time.Millisecond)
		defer cancel()

		outputData, stepErr := executor.Execute(stepCtx, step, currentData)
		stepDuration := time.Now().Sub(stepStartTime)

		// Create step log
		stepLog := models.StepExecutionLog{
			StepID:      step.ID,
			StepName:    step.StepName,
			StepType:    step.StepType,
			StartedAt:   stepStartTime,
			CompletedAt: time.Now(),
			DurationMs:  stepDuration.Milliseconds(),
			Success:     stepErr == nil,
		}

		if stepErr != nil {
			stepLog.Error = stepErr.Error()
			fmt.Printf("❌ Step failed: %s - %v\n", step.StepName, stepErr)

			// Handle error based on strategy
			if step.Required && step.OnErrorStrategy == "fail" {
				result.Success = false
				result.Error = fmt.Sprintf("Required step failed: %s - %v", step.StepName, stepErr)
				result.TransformationLog = append(result.TransformationLog, stepLog)
				return result, fmt.Errorf("pipeline failed at step %s: %w", step.StepName, stepErr)
			} else if step.OnErrorStrategy == "skip" {
				fmt.Printf("⚠️  Skipping failed optional step: %s\n", step.StepName)
				// Continue with previous data
			} else if step.OnErrorStrategy == "default" {
				// Use default value from config (if provided)
				// For now, continue with previous data
				fmt.Printf("⚠️  Using default value for failed step: %s\n", step.StepName)
			}
		} else {
			// Step succeeded, update current data
			currentData = outputData
			result.OutputData = outputData
			fmt.Printf("✅ Step completed: %s (took %dms)\n", step.StepName, stepDuration.Milliseconds())
		}

		result.TransformationLog = append(result.TransformationLog, stepLog)
		result.TotalTimeMs += stepDuration.Milliseconds()
	}

	fmt.Printf("✅ Pipeline completed successfully (total: %dms)\n", result.TotalTimeMs)

	return result, nil
}

// createExecutionRecord creates a new execution record in the database
func (tps *TransformationPipelineService) createExecutionRecord(ctx context.Context, execution *models.TransformationExecution) error {
	query := `
		INSERT INTO transformation_executions (
			id, message_id, interface_id, pipeline_id, started_at, status,
			steps_executed, steps_failed, input_data
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
	`

	inputDataJSON, _ := json.Marshal(execution.InputData)

	_, err := tps.db.ExecContext(ctx, query,
		execution.ID,
		execution.MessageID,
		execution.InterfaceID,
		execution.PipelineID,
		execution.StartedAt,
		execution.Status,
		execution.StepsExecuted,
		execution.StepsFailed,
		inputDataJSON,
	)

	return err
}

// updateExecutionRecord updates an execution record in the database
func (tps *TransformationPipelineService) updateExecutionRecord(ctx context.Context, execution *models.TransformationExecution) error {
	query := `
		UPDATE transformation_executions
		SET completed_at = $1, total_time_ms = $2, status = $3,
		    steps_executed = $4, steps_failed = $5, output_data = $6,
		    error_message = $7, execution_log = $8
		WHERE id = $9
	`

	outputDataJSON, _ := json.Marshal(execution.OutputData)
	executionLogJSON, _ := json.Marshal(execution.ExecutionLog)

	_, err := tps.db.ExecContext(ctx, query,
		execution.CompletedAt,
		execution.TotalTimeMs,
		execution.Status,
		execution.StepsExecuted,
		execution.StepsFailed,
		outputDataJSON,
		execution.ErrorMessage,
		executionLogJSON,
		execution.ID,
	)

	return err
}

// CreatePipeline creates a new transformation pipeline
func (tps *TransformationPipelineService) CreatePipeline(ctx context.Context, pipeline *models.TransformationPipeline) error {
	pipeline.ID = uuid.New().String()
	pipeline.CreatedAt = time.Now()
	pipeline.UpdatedAt = time.Now()

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

	return err
}

// CreateStep creates a new transformation step
func (tps *TransformationPipelineService) CreateStep(ctx context.Context, step *models.TransformationStep) error {
	step.ID = uuid.New().String()
	step.CreatedAt = time.Now()
	step.UpdatedAt = time.Now()

	configJSON, err := json.Marshal(step.Config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	query := `
		INSERT INTO transformation_steps (
			id, pipeline_id, step_name, step_type, sequence, layer, required,
			timeout_ms, enabled, config, script_type, script_content,
			on_error_strategy, execution_mode, created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16)
	`

	_, err = tps.db.ExecContext(ctx, query,
		step.ID,
		step.PipelineID,
		step.StepName,
		step.StepType,
		step.Sequence,
		step.Layer,
		step.Required,
		step.TimeoutMs,
		step.Enabled,
		configJSON,
		step.ScriptType,
		step.ScriptContent,
		step.OnErrorStrategy,
		step.ExecutionMode,
		step.CreatedAt,
		step.UpdatedAt,
	)

	return err
}
