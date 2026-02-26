package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	"ezhealthkonnect/services/executors/control"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
)

// TransformationPipelineService handles transformation pipeline operations
type TransformationPipelineService struct {
	db               *sql.DB
	executorRegistry *ExecutorRegistry
	branchResolver   *BranchResolverFactory            // OOP-based branch resolution
	executors        map[string]TransformationExecutor // Legacy - kept for compatibility
}

// TransformationExecutor interface for step execution
type TransformationExecutor interface {
	Execute(ctx context.Context, step models.TransformationStep, input map[string]interface{}) (map[string]interface{}, error)
	GetSupportedType() string
}

// NewTransformationPipelineService creates a new transformation pipeline service.
// credStore may be nil; if provided, encrypted connectivity configs are decrypted
// transparently by any executor that reads credentials from the database.
func NewTransformationPipelineService(db *sql.DB, credStore *CredentialStore) *TransformationPipelineService {
	service := &TransformationPipelineService{
		db:               db,
		executorRegistry: NewExecutorRegistry(db, credStore), // Use ExecutorRegistry with all built-in executors
		branchResolver:   NewBranchResolverFactory(),          // OOP-based branch resolution for conditionals
		executors:        make(map[string]TransformationExecutor),
	}

	return service
}

// RegisterExecutor registers a transformation executor
func (tps *TransformationPipelineService) RegisterExecutor(executor TransformationExecutor) {
	tps.executors[executor.GetSupportedType()] = executor
}

// GetExecutorRegistry returns the executor registry for testing
func (tps *TransformationPipelineService) GetExecutorRegistry() *ExecutorRegistry {
	return tps.executorRegistry
}

// GetPipeline retrieves a transformation pipeline for an interface/message type
func (tps *TransformationPipelineService) GetPipeline(ctx context.Context, interfaceID string, messageType string) (*models.TransformationPipeline, error) {
	query := `
		SELECT id, interface_id, message_type, pipeline_name, enabled, version,
		       COALESCE(pipeline_config, '{}') AS pipeline_config,
		       created_at, updated_at
		FROM transformation_pipelines
		WHERE interface_id = $1 AND message_type = $2 AND enabled = true
		LIMIT 1
	`

	pipeline := &models.TransformationPipeline{}
	var pipelineConfigJSON []byte
	err := tps.db.QueryRowContext(ctx, query, interfaceID, messageType).Scan(
		&pipeline.ID,
		&pipeline.InterfaceID,
		&pipeline.MessageType,
		&pipeline.PipelineName,
		&pipeline.Enabled,
		&pipeline.Version,
		&pipelineConfigJSON,
		&pipeline.CreatedAt,
		&pipeline.UpdatedAt,
	)

	if err == nil && len(pipelineConfigJSON) > 0 {
		pipeline.PipelineConfig = make(map[string]interface{})
		json.Unmarshal(pipelineConfigJSON, &pipeline.PipelineConfig)
	}

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

// GetPipelineSteps retrieves all steps for a pipeline, ordered by sequence
func (tps *TransformationPipelineService) GetPipelineSteps(ctx context.Context, pipelineID string) ([]models.TransformationStep, error) {
	query := `
		SELECT id, pipeline_id, step_name, step_alias, step_type, sequence, required, timeout_ms,
		       enabled, config, script_type, script_content, on_error_strategy, execution_mode,
		       position_x, position_y, parent_conditional_step_id, branch_type, case_value,
		       parent_step_id, container_zone,
		       created_at, updated_at
		FROM transformation_steps
		WHERE pipeline_id = $1 AND enabled = true
		ORDER BY sequence ASC
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
			&step.StepAlias,
			&step.StepType,
			&step.Sequence,
			&step.Required,
			&step.TimeoutMs,
			&step.Enabled,
			&configJSON,
			&step.ScriptType,
			&step.ScriptContent,
			&step.OnErrorStrategy,
			&step.ExecutionMode,
			&step.PositionX,
			&step.PositionY,
			&step.ParentConditionalStepID,
			&step.BranchType,
			&step.CaseValue,
			&step.ParentStepID,
			&step.ContainerZone,
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

// executePipeline executes all steps in a pipeline using PipelineExecutionContext
func (tps *TransformationPipelineService) executePipeline(
	ctx context.Context,
	pipeline *models.TransformationPipeline,
	input map[string]interface{},
) (*models.TransformationResult, error) {

	// Create execution context with step output tracking
	execContext := &models.PipelineExecutionContext{
		Message:     input,
		StepOutputs: make(map[string]models.StepOutput),
		Metadata: map[string]interface{}{
			"pipeline_id":   pipeline.ID,
			"pipeline_name": pipeline.PipelineName,
			"started_at":    time.Now(),
		},
	}

	result := &models.TransformationResult{
		Success:           true,
		OutputData:        input,
		TransformationLog: []models.StepExecutionLog{},
		TotalTimeMs:       0,
	}

	// Extract pipeline-level defaults for error handling and retry
	pipelineRetryDefaults := executors.ParsePipelineRetryDefaults(pipeline.PipelineConfig)
	pipelineEHDefaults := executors.ParsePipelineErrorHandlingDefaults(pipeline.PipelineConfig)
	if pipelineRetryDefaults != nil {
		fmt.Printf("🔄 Pipeline-level retry defaults: max=%d, delay=%dms, backoff=%.1fx\n",
			pipelineRetryDefaults.MaxRetries, pipelineRetryDefaults.DelayMs, pipelineRetryDefaults.BackoffMultiplier)
	}
	if pipelineEHDefaults != nil {
		fmt.Printf("🛡️ Pipeline-level error handling defaults: onError=%s\n", pipelineEHDefaults.OnError)
	}

	// Build step index for conditional routing lookup
	stepIndex := make(map[string]int) // stepID -> index in pipeline.Steps
	stepByID := make(map[string]*models.TransformationStep) // stepID -> step
	for i, step := range pipeline.Steps {
		stepIndex[step.ID] = i
		stepCopy := step // Copy to avoid closure capture issues
		stepByID[step.ID] = &stepCopy
	}

	// Create child executor callback for loop steps
	childExecutor := tps.createChildExecutorCallback(ctx, stepByID, execContext)

	// Execute steps in order (ordered by sequence ASC)
	// Support conditional routing via _routing.nextStep and _routing.skipSteps
	skipToStepID := ""                    // If set, skip steps until we reach this step ID
	skipStepIDs := make(map[string]bool)  // Set of step IDs to skip (for branch exclusion)
	loopChildStepIDs := make(map[string]bool) // Child steps already executed by loop containers

	for i := 0; i < len(pipeline.Steps); i++ {
		step := pipeline.Steps[i]

		// Check if this step should be skipped (branch exclusion)
		if skipStepIDs[step.ID] {
			fmt.Printf("⏭️  Skipping step (branch excluded): %s\n", step.StepName)
			continue
		}

		// ✅ Skip child steps already executed by a loop container
		if loopChildStepIDs[step.ID] {
			fmt.Printf("⏭️  Skipping step (executed by loop): %s\n", step.StepName)
			continue
		}

		// Handle conditional routing - skip steps until we reach the target
		if skipToStepID != "" {
			if step.ID != skipToStepID {
				fmt.Printf("⏭️  Skipping step (routing): %s (waiting for %s)\n", step.StepName, skipToStepID)
				continue
			}
			// Found the target step, reset skip mode
			fmt.Printf("🎯 Reached routing target: %s\n", step.StepName)
			skipToStepID = ""
		}
		stepStartTime := time.Now()

		fmt.Printf("🔄 Executing step: %s (%s)\n", step.StepName, step.StepType)

		// Get executor from registry (with automatic fallback to generic)
		executor := tps.executorRegistry.GetExecutor(step.StepType)
		if executor == nil {
			return nil, fmt.Errorf("no executor available for step type: %s", step.StepType)
		}

		// ✅ LOOP HANDLING: Inject child executor callback and mark child steps
		if step.StepType == "control.loop" {
			if loopExecutor, ok := executor.(*control.LoopExecutor); ok {
				loopExecutor.SetChildExecutor(childExecutor)
				fmt.Printf("🔄 Injected child executor into loop step: %s\n", step.StepName)
			}
			// Mark child steps so they're skipped in the main pipeline flow
			if childIDs, ok := step.Config["childStepIds"].([]interface{}); ok {
				for _, cid := range childIDs {
					if cidStr, ok := cid.(string); ok {
						loopChildStepIDs[cidStr] = true
						fmt.Printf("   📌 Marked child step for loop-only execution: %s\n", cidStr)
					}
				}
			}
			// Also handle []string format
			if childIDs, ok := step.Config["childStepIds"].([]string); ok {
				for _, cidStr := range childIDs {
					loopChildStepIDs[cidStr] = true
					fmt.Printf("   📌 Marked child step for loop-only execution: %s\n", cidStr)
				}
			}
		}

		// Debug: Log step config keys for troubleshooting error handling resolution
		configKeys := make([]string, 0, len(step.Config))
		for k := range step.Config {
			configKeys = append(configKeys, k)
		}
		fmt.Printf("🔍 Step '%s' config keys: %v\n", step.StepName, configKeys)
		if eh, ok := step.Config["errorHandling"]; ok {
			fmt.Printf("🔍 Step '%s' errorHandling config: %v\n", step.StepName, eh)
		} else {
			fmt.Printf("🔍 Step '%s' has NO errorHandling in config\n", step.StepName)
		}
		if rt, ok := step.Config["retry"]; ok {
			fmt.Printf("🔍 Step '%s' retry config: %v\n", step.StepName, rt)
		}
		fmt.Printf("🔍 Pipeline EH defaults: %v\n", pipelineEHDefaults)

		// Resolve retry config: step override > pipeline default > nil
		retryConfig := executors.ResolveRetryConfig(step.Config, pipelineRetryDefaults)
		if retryConfig != nil {
			fmt.Printf("🔄 Retry enabled for: %s (max=%d, delay=%dms, backoff=%.1fx)\n",
				step.StepName, retryConfig.MaxRetries, retryConfig.DelayMs, retryConfig.BackoffMultiplier)
		}

		// Resolve error handling config: step override > pipeline default > nil
		ehConfig := executors.ResolveErrorHandlingConfig(step.Config, pipelineEHDefaults)
		if ehConfig != nil {
			fmt.Printf("🛡️ Error handling RESOLVED for: %s (onError=%s, defaultField=%s, defaultValue=%s)\n",
				step.StepName, ehConfig.OnError, ehConfig.DefaultField, ehConfig.DefaultValue)
		} else {
			fmt.Printf("⚠️ Error handling NOT resolved for: %s (ehConfig is nil)\n", step.StepName)
		}

		// Execute step with retry (ExecuteWithRetry handles single-exec when retryConfig is nil)
		// P3: shallow-copy the shared message before each attempt so step mutations are isolated.
		retryResult := executors.ExecuteWithRetry(ctx, retryConfig, func(attempt int) (map[string]interface{}, error) {
			stepCtx, cancel := context.WithTimeout(ctx, time.Duration(step.TimeoutMs)*time.Millisecond)
			defer cancel()
			return executor.Execute(stepCtx, &step, shallowCopyMap(execContext.Message))
		})

		outputData := retryResult.Output
		stepErr := retryResult.Err
		originalErr := stepErr // preserve original error for logging
		stepDuration := time.Now().Sub(stepStartTime)

		// Apply error handling if step failed after all retries
		errorWasCaught := false
		fmt.Printf("🔍 Step '%s' post-exec: stepErr=%v, ehConfig=%v\n", step.StepName, stepErr != nil, ehConfig != nil)
		if stepErr != nil && ehConfig != nil {
			fmt.Printf("🛡️ APPLYING error handling for: %s (error: %s)\n", step.StepName, stepErr.Error())
			outputData, stepErr = executors.ApplyErrorHandling(ehConfig, stepErr, outputData, execContext.Message, step.StepName)
			if stepErr == nil {
				errorWasCaught = true // error was caught/suppressed — treat as success
				fmt.Printf("🛡️ Error CAUGHT for: %s\n", step.StepName)
			} else {
				fmt.Printf("🛡️ Error RETHROWN for: %s\n", step.StepName)
			}
		} else if stepErr != nil {
			fmt.Printf("⚠️ Step '%s' failed but NO error handler: stepErr=%s\n", step.StepName, stepErr.Error())
		}

		// Generate namespace for this step
		namespace := models.GenerateStepNamespace(step.StepName, step.ID, step.StepAlias)
		alias := ""
		if step.StepAlias != nil {
			alias = *step.StepAlias
		} else {
			alias = models.GenerateDefaultAlias(step.StepName)
		}

		// Get step output from namespace (if executor set it)
		stepOutput, outputExists := execContext.StepOutputs[namespace]

		// Update step output with duration and success status
		if outputExists {
			stepOutput.DurationMs = stepDuration.Milliseconds()
			if stepErr != nil {
				stepOutput.Success = false
				stepOutput.Error = stepErr.Error()
			} else if errorWasCaught {
				// Error was caught by handler — mark as success with note
				stepOutput.Success = true
				stepOutput.Error = fmt.Sprintf("caught: %s", originalErr.Error())
				if stepOutput.OutputData == nil {
					stepOutput.OutputData = make(map[string]interface{})
				}
				stepOutput.OutputData["error_caught"] = originalErr.Error()
				stepOutput.OutputData["error_handler"] = ehConfig.OnError
				if ehConfig.DefaultField != "" && ehConfig.DefaultValue != "" {
					stepOutput.OutputData["default_applied_field"] = ehConfig.DefaultField
					stepOutput.OutputData["default_applied_value"] = ehConfig.DefaultValue
				}
			}
			execContext.StepOutputs[namespace] = stepOutput
		} else {
			// Executor didn't set output - create automatic output with basic metadata
			stepOutputData := map[string]interface{}{
				"step_type":         step.StepType,
				"execution_time_ms": stepDuration.Milliseconds(),
			}

			// For API enrichment steps, capture additional metadata automatically
			if step.StepType == "pre.enrichment.api" {
				if config, ok := step.Config["endpoint"].(string); ok {
					stepOutputData["api_endpoint"] = config
				}
				if method, ok := step.Config["method"].(string); ok {
					stepOutputData["http_method"] = method
				}
				if targetPath, ok := step.Config["targetPath"].(string); ok {
					stepOutputData["enriched_path"] = targetPath
				}
			}

			if stepErr != nil {
				// Step failed - create error output
				execContext.StepOutputs[namespace] = models.StepOutput{
					StepID:     step.ID,
					StepName:   step.StepName,
					StepAlias:  alias,
					StepType:   step.StepType,
					Namespace:  namespace,
					Sequence:   step.Sequence,
					OutputData: stepOutputData,
					Success:    false,
					Error:      stepErr.Error(),
					DurationMs: stepDuration.Milliseconds(),
				}
			} else if errorWasCaught {
				// Error was caught by handler — mark as success with note
				stepOutputData["error_caught"] = originalErr.Error()
				stepOutputData["error_handler"] = ehConfig.OnError
				if ehConfig.DefaultField != "" && ehConfig.DefaultValue != "" {
					stepOutputData["default_applied_field"] = ehConfig.DefaultField
					stepOutputData["default_applied_value"] = ehConfig.DefaultValue
				}
				execContext.StepOutputs[namespace] = models.StepOutput{
					StepID:     step.ID,
					StepName:   step.StepName,
					StepAlias:  alias,
					StepType:   step.StepType,
					Namespace:  namespace,
					Sequence:   step.Sequence,
					OutputData: stepOutputData,
					Success:    true,
					Error:      fmt.Sprintf("caught: %s", originalErr.Error()),
					DurationMs: stepDuration.Milliseconds(),
				}
			} else {
				// Step succeeded - create success output
				execContext.StepOutputs[namespace] = models.StepOutput{
					StepID:     step.ID,
					StepName:   step.StepName,
					StepAlias:  alias,
					StepType:   step.StepType,
					Namespace:  namespace,
					Sequence:   step.Sequence,
					OutputData: stepOutputData,
					Success:    true,
					DurationMs: stepDuration.Milliseconds(),
				}
			}
		}

		// Create step log with step output reference
		stepLog := models.StepExecutionLog{
			StepID:      step.ID,
			StepName:    step.StepName,
			StepAlias:   alias,
			StepType:    step.StepType,
			Namespace:   namespace,
			StartedAt:   stepStartTime,
			CompletedAt: time.Now(),
			DurationMs:  stepDuration.Milliseconds(),
			Success:     stepErr == nil,
		}

		// Attach step output to log
		if output, exists := execContext.StepOutputs[namespace]; exists {
			stepLog.StepOutput = &output
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
				// Continue with previous message data
			} else if step.OnErrorStrategy == "default" {
				// Use default value from config (if provided)
				// For now, continue with previous message data
				fmt.Printf("⚠️  Using default value for failed step: %s\n", step.StepName)
			}
		} else if errorWasCaught {
			// Error was caught by handler — continue pipeline with updated output
			// outputData now contains _lastError, _lastErrorStep, and any default field values
			if outputData != nil {
				execContext.Message = outputData
			}
			result.OutputData = execContext.Message
			fmt.Printf("🛡️ Step error caught: %s (error: %s, took %dms)\n", step.StepName, originalErr.Error(), stepDuration.Milliseconds())
		} else {
			// Step succeeded, update message data (executors still modify the map directly)
			if outputData != nil {
				execContext.Message = outputData
			}
			result.OutputData = execContext.Message
			fmt.Printf("✅ Step completed: %s (took %dms)\n", step.StepName, stepDuration.Milliseconds())

			// Check for conditional routing directive from this step
			// Supports both _routing.nextStep (jump to) and _routing.skipSteps (skip specific steps)
			if routing, ok := outputData["_routing"].(map[string]interface{}); ok {
				// Handle nextStep - jump to a specific step, skip everything in between
				if nextStepID, ok := routing["nextStep"].(string); ok && nextStepID != "" {
					fmt.Printf("🔀 Conditional routing detected: jumping to step %s\n", nextStepID)
					skipToStepID = nextStepID
				}

				// Handle skipSteps - mark specific steps to be skipped (branch exclusion)
				// This is used for conditional branches where FALSE branch steps should be skipped
				if stepsToSkip, ok := routing["skipSteps"].([]interface{}); ok && len(stepsToSkip) > 0 {
					for _, stepID := range stepsToSkip {
						if sid, ok := stepID.(string); ok {
							skipStepIDs[sid] = true
							fmt.Printf("🚫 Marking step for skip (branch exclusion): %s\n", sid)
						}
					}
				}
				// Also support []string format (from Go code)
				if stepsToSkip, ok := routing["skipSteps"].([]string); ok && len(stepsToSkip) > 0 {
					for _, stepID := range stepsToSkip {
						skipStepIDs[stepID] = true
						fmt.Printf("🚫 Marking step for skip (branch exclusion): %s\n", stepID)
					}
				}

				// OOP-BASED BRANCH RESOLUTION
				// Use BranchResolver to determine which steps to skip based on routing decision
				// Supports both IfThenElse (branchTaken) and Switch/Case (caseMatched)
				stepsToAutoSkip := tps.branchResolver.GetStepsToSkip(&step, routing, pipeline.Steps)
				for _, stepID := range stepsToAutoSkip {
					skipStepIDs[stepID] = true
				}
			}
		}

		result.TransformationLog = append(result.TransformationLog, stepLog)
		result.TotalTimeMs += stepDuration.Milliseconds()
	}

	// Final output is the transformed message
	result.OutputData = execContext.Message

	fmt.Printf("✅ Pipeline completed successfully (total: %dms, %d step outputs tracked)\n",
		result.TotalTimeMs, len(execContext.StepOutputs))

	return result, nil
}

// createChildExecutorCallback creates a callback function for loop steps to execute child steps
// This enables loops to actually execute their child steps and collect outputs
func (tps *TransformationPipelineService) createChildExecutorCallback(
	ctx context.Context,
	stepByID map[string]*models.TransformationStep,
	execContext *models.PipelineExecutionContext,
) control.ChildStepExecutor {
	return func(
		childCtx context.Context,
		stepID string,
		inputData map[string]interface{},
	) (map[string]interface{}, string, error) {
		// Look up the child step
		step, exists := stepByID[stepID]
		if !exists {
			return nil, "", fmt.Errorf("child step not found: %s", stepID)
		}

		log.Printf("     🔧 Executing child step: %s (%s)", step.StepName, step.StepType)

		// Get executor for the child step type
		executor := tps.executorRegistry.GetExecutor(step.StepType)
		if executor == nil {
			return nil, step.StepName, fmt.Errorf("no executor for child step type: %s", step.StepType)
		}

		// Execute the child step with the full input data (includes message + loop context)
		outputData, err := executor.Execute(childCtx, step, inputData)
		if err != nil {
			return nil, step.StepName, err
		}

		// Return the full output data for chaining to next child step
		// The loop executor will use the step name as the aggregation key
		return outputData, step.StepName, nil
	}
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
			id, pipeline_id, step_name, step_type, sequence, required,
			timeout_ms, enabled, config, script_type, script_content,
			on_error_strategy, execution_mode, parent_step_id, container_zone,
			created_at, updated_at
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17)
	`

	_, err = tps.db.ExecContext(ctx, query,
		step.ID,
		step.PipelineID,
		step.StepName,
		step.StepType,
		step.Sequence,
		step.Required,
		step.TimeoutMs,
		step.Enabled,
		configJSON,
		step.ScriptType,
		step.ScriptContent,
		step.OnErrorStrategy,
		step.ExecutionMode,
		step.ParentStepID,
		step.ContainerZone,
		step.CreatedAt,
		step.UpdatedAt,
	)

	return err
}

// shallowCopyMap creates a one-level-deep copy of a map so that step executions
// cannot corrupt the shared execContext.Message.  Values are copied by reference,
// which is sufficient: executors replace top-level keys (they do not mutate nested
// structures in-place) and the engine merges outputData back after success.
func shallowCopyMap(src map[string]interface{}) map[string]interface{} {
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}
