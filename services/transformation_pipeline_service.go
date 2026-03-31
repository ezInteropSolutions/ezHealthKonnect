package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	"ezhealthkonnect/services/executors/control"
	"ezhealthkonnect/services/logger"
	"ezhealthkonnect/services/storage"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// TransformationPipelineService handles transformation pipeline operations
type TransformationPipelineService struct {
	db               *sql.DB
	executorRegistry *ExecutorRegistry
	branchResolver   *BranchResolverFactory            // OOP-based branch resolution
	executors        map[string]TransformationExecutor // Legacy - kept for compatibility
	objectStorage    *storage.ObjectStorageService     // For writing connector delivery logs
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
		objectStorage:    nil, // Set via SetObjectStorage after construction
	}

	return service
}

// SetObjectStorage wires the object storage service for connector delivery logging.
func (tps *TransformationPipelineService) SetObjectStorage(svc *storage.ObjectStorageService) {
	tps.objectStorage = svc
}

// SetCodeTemplateService wires a CodeTemplateService into the script enrichment executor
// so that named JS function libraries are injected into every script step's goja VM.
func (tps *TransformationPipelineService) SetCodeTemplateService(svc *CodeTemplateService) {
	tps.executorRegistry.SetCodeTemplateService(svc)
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
		       COALESCE(connections, '[]') AS connections,
		       created_at, updated_at
		FROM transformation_pipelines
		WHERE interface_id = $1 AND message_type = $2 AND enabled = true
		LIMIT 1
	`

	pipeline := &models.TransformationPipeline{}
	var pipelineConfigJSON []byte
	var connectionsJSON []byte
	err := tps.db.QueryRowContext(ctx, query, interfaceID, messageType).Scan(
		&pipeline.ID,
		&pipeline.InterfaceID,
		&pipeline.MessageType,
		&pipeline.PipelineName,
		&pipeline.Enabled,
		&pipeline.Version,
		&pipelineConfigJSON,
		&connectionsJSON,
		&pipeline.CreatedAt,
		&pipeline.UpdatedAt,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get pipeline: %w", err)
	}

	if len(pipelineConfigJSON) > 0 {
		pipeline.PipelineConfig = make(map[string]interface{})
		json.Unmarshal(pipelineConfigJSON, &pipeline.PipelineConfig)
	}

	if len(connectionsJSON) > 0 {
		json.Unmarshal(connectionsJSON, &pipeline.Connections)
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
		logger.Error("GetPipeline failed", "interface", interfaceID, "message_type", messageType, "error", err)
		return nil, fmt.Errorf("failed to get pipeline: %w", err)
	}

	logger.Info("GetPipeline success", "steps", len(pipeline.Steps), "interface", interfaceID, "message_type", messageType)

	// Inject runtime context into each step's config (OOB: auto-configure steps)
	for i := range pipeline.Steps {
		if pipeline.Steps[i].Config == nil {
			pipeline.Steps[i].Config = make(map[string]interface{})
		}
		pipeline.Steps[i].Config["interface_id"] = interfaceID
		pipeline.Steps[i].Config["message_type"] = messageType
		pipeline.Steps[i].Config["message_id"] = messageID

		logger.Debug("pipeline context injected", "seq", i, "step", pipeline.Steps[i].StepName, "interface", interfaceID, "message_type", messageType)
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
		logger.Warn("failed to create execution record", "error", err)
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
		logger.Warn("failed to update execution record", "error", updateErr)
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
		logger.Debug("pipeline-level retry defaults", "max_retries", pipelineRetryDefaults.MaxRetries, "delay_ms", pipelineRetryDefaults.DelayMs, "backoff", pipelineRetryDefaults.BackoffMultiplier)
	}
	if pipelineEHDefaults != nil {
		logger.Debug("pipeline-level error handling defaults", "on_error", pipelineEHDefaults.OnError)
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

	// Build DAG from pipeline connections and decide execution path
	dag := BuildDAG(pipeline.Steps, pipeline.Connections)

	if !dag.isLinear {
		// ── PARALLEL PATH: use DAG scheduler ──────────────────────────────
		logger.Info("executing pipeline via DAG scheduler", "steps", len(pipeline.Steps))
		loopChildStepIDs := make(map[string]bool)
		skipStepIDsDAG := make(map[string]bool)
		// Pre-populate loop child IDs (same logic as sequential path)
		for _, step := range pipeline.Steps {
			if step.StepType == "control.loop" {
				if childIDs, ok := step.Config["childStepIds"].([]interface{}); ok {
					for _, cid := range childIDs {
						if cidStr, ok := cid.(string); ok {
							loopChildStepIDs[cidStr] = true
						}
					}
				}
				if childIDs, ok := step.Config["childStepIds"].([]string); ok {
					for _, cidStr := range childIDs {
						loopChildStepIDs[cidStr] = true
					}
				}
			}
		}
		err := tps.executeDAG(ctx, dag, pipeline, execContext, result,
			loopChildStepIDs, skipStepIDsDAG, stepByID, childExecutor)
		if err != nil {
			return result, err
		}
		return result, nil
	}

	// ── SEQUENTIAL FAST PATH (linear pipeline — no change to existing logic) ──
	logger.Info("executing pipeline sequentially", "steps", len(pipeline.Steps))

	// Execute steps in order (ordered by sequence ASC)
	// Support conditional routing via _routing.nextStep and _routing.skipSteps
	skipToStepID := ""                    // If set, skip steps until we reach this step ID
	skipStepIDs := make(map[string]bool)  // Set of step IDs to skip (for branch exclusion)
	loopChildStepIDs := make(map[string]bool) // Child steps already executed by loop containers

	for i := 0; i < len(pipeline.Steps); i++ {
		step := pipeline.Steps[i]

		// Check if this step should be skipped (branch exclusion)
		if skipStepIDs[step.ID] {
			logger.Debug("skipping step (branch excluded)", "step", step.StepName)
			continue
		}

		// ✅ Skip child steps already executed by a loop container
		if loopChildStepIDs[step.ID] {
			logger.Debug("skipping step (executed by loop)", "step", step.StepName)
			continue
		}

		// Handle conditional routing - skip steps until we reach the target
		if skipToStepID != "" {
			if step.ID != skipToStepID {
				logger.Debug("skipping step (routing)", "step", step.StepName, "waiting_for", skipToStepID)
				continue
			}
			// Found the target step, reset skip mode
			logger.Debug("reached routing target", "step", step.StepName)
			skipToStepID = ""
		}
		stepStartTime := time.Now()

		logger.Debug("executing step", "step", step.StepName, "type", step.StepType)

		// Get executor from registry (with automatic fallback to generic)
		executor := tps.executorRegistry.GetExecutor(step.StepType)
		if executor == nil {
			return nil, fmt.Errorf("no executor available for step type: %s", step.StepType)
		}

		// Normalize aliased step types (e.g. "pre.enrichment.script" → "enrichment.script")
		// so that BaseExecutor.PreExecute() doesn't fail the strict type-equality check.
		if canonical := executor.GetStepType(); canonical != "" && canonical != "generic" && canonical != step.StepType {
			logger.Debug("normalizing step type alias", "from", step.StepType, "to", canonical)
			step.StepType = canonical
		}

		// ✅ LOOP HANDLING: Inject child executor callback and mark child steps
		if step.StepType == "control.loop" {
			if loopExecutor, ok := executor.(*control.LoopExecutor); ok {
				loopExecutor.SetChildExecutor(childExecutor)
				logger.Debug("injected child executor into loop step", "step", step.StepName)
			}
			// Mark child steps so they're skipped in the main pipeline flow
			if childIDs, ok := step.Config["childStepIds"].([]interface{}); ok {
				for _, cid := range childIDs {
					if cidStr, ok := cid.(string); ok {
						loopChildStepIDs[cidStr] = true
						logger.Debug("marked child step for loop-only execution", "child_step", cidStr)
					}
				}
			}
			// Also handle []string format
			if childIDs, ok := step.Config["childStepIds"].([]string); ok {
				for _, cidStr := range childIDs {
					loopChildStepIDs[cidStr] = true
					logger.Debug("marked child step for loop-only execution", "child_step", cidStr)
				}
			}
		}

		// Debug: Log step config keys for troubleshooting error handling resolution
		configKeys := make([]string, 0, len(step.Config))
		for k := range step.Config {
			configKeys = append(configKeys, k)
		}
		logger.Debug("step config keys", "step", step.StepName, "keys", configKeys)
		if eh, ok := step.Config["errorHandling"]; ok {
			logger.Debug("step errorHandling config", "step", step.StepName, "config", eh)
		} else {
			logger.Debug("step has no errorHandling in config", "step", step.StepName)
		}
		if rt, ok := step.Config["retry"]; ok {
			logger.Debug("step retry config", "step", step.StepName, "config", rt)
		}
		logger.Debug("pipeline EH defaults", "defaults", pipelineEHDefaults)

		// Resolve retry config: step override > pipeline default > nil
		retryConfig := executors.ResolveRetryConfig(step.Config, pipelineRetryDefaults)
		if retryConfig != nil {
			logger.Debug("retry enabled for step", "step", step.StepName, "max_retries", retryConfig.MaxRetries, "delay_ms", retryConfig.DelayMs, "backoff", retryConfig.BackoffMultiplier)
		}

		// Resolve error handling config: step override > pipeline default > nil
		ehConfig := executors.ResolveErrorHandlingConfig(step.Config, pipelineEHDefaults)
		if ehConfig != nil {
			logger.Debug("error handling resolved", "step", step.StepName, "on_error", ehConfig.OnError, "default_field", ehConfig.DefaultField)
		} else {
			logger.Debug("error handling not configured", "step", step.StepName)
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
		logger.Debug("step post-exec", "step", step.StepName, "has_error", stepErr != nil, "has_eh_config", ehConfig != nil)
		if stepErr != nil && ehConfig != nil {
			logger.Debug("applying error handling", "step", step.StepName, "error", stepErr.Error())
			outputData, stepErr = executors.ApplyErrorHandling(ehConfig, stepErr, outputData, execContext.Message, step.StepName)
			if stepErr == nil {
				errorWasCaught = true // error was caught/suppressed — treat as success
				logger.Debug("error caught (suppressed)", "step", step.StepName)
			} else {
				logger.Debug("error rethrown", "step", step.StepName)
			}
		} else if stepErr != nil {
			logger.Warn("step failed with no error handler", "step", step.StepName, "error", stepErr.Error())
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
			logger.Error("step failed", "step", step.StepName, "error", stepErr)

			// Handle error based on strategy
			if step.Required && step.OnErrorStrategy == "fail" {
				result.Success = false
				result.Error = fmt.Sprintf("Required step failed: %s - %v", step.StepName, stepErr)
				result.TransformationLog = append(result.TransformationLog, stepLog)
				return result, fmt.Errorf("pipeline failed at step %s: %w", step.StepName, stepErr)
			} else if step.OnErrorStrategy == "skip" {
				logger.Warn("skipping failed optional step", "step", step.StepName)
				// Continue with previous message data
			} else if step.OnErrorStrategy == "default" {
				// Use default value from config (if provided)
				// For now, continue with previous message data
				logger.Warn("using default value for failed step", "step", step.StepName)
			}
		} else if errorWasCaught {
			// Error was caught by handler — continue pipeline with updated output
			// outputData now contains _lastError, _lastErrorStep, and any default field values
			if outputData != nil {
				execContext.Message = outputData
			}
			result.OutputData = execContext.Message
			logger.Info("step error caught", "step", step.StepName, "error", originalErr.Error(), "duration_ms", stepDuration.Milliseconds())
		} else {
			// Step succeeded, update message data (executors still modify the map directly)
			if outputData != nil {
				execContext.Message = outputData
			}
			result.OutputData = execContext.Message
			logger.Info("step completed", "step", step.StepName, "duration_ms", stepDuration.Milliseconds())

			// Check for conditional routing directive from this step
			// Supports both _routing.nextStep (jump to) and _routing.skipSteps (skip specific steps)
			if routing, ok := outputData["_routing"].(map[string]interface{}); ok {
				// Handle nextStep - jump to a specific step, skip everything in between
				if nextStepID, ok := routing["nextStep"].(string); ok && nextStepID != "" {
					logger.Debug("conditional routing: jump to step", "target", nextStepID)
					skipToStepID = nextStepID
				}

				// Handle skipSteps - mark specific steps to be skipped (branch exclusion)
				// This is used for conditional branches where FALSE branch steps should be skipped
				if stepsToSkip, ok := routing["skipSteps"].([]interface{}); ok && len(stepsToSkip) > 0 {
					for _, stepID := range stepsToSkip {
						if sid, ok := stepID.(string); ok {
							skipStepIDs[sid] = true
							logger.Debug("marking step for skip (branch exclusion)", "step", sid)
						}
					}
				}
				// Also support []string format (from Go code)
				if stepsToSkip, ok := routing["skipSteps"].([]string); ok && len(stepsToSkip) > 0 {
					for _, stepID := range stepsToSkip {
						skipStepIDs[stepID] = true
						logger.Debug("marking step for skip (branch exclusion)", "step", stepID)
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

	logger.Info("pipeline completed", "total_ms", result.TotalTimeMs, "step_outputs", len(execContext.StepOutputs))

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

		logger.Debug("executing child step", "step", step.StepName, "type", step.StepType)

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

// persistExecutionRecords saves the full execution + per-step records for a live message.
// Called from ExecutePipeline (helpers.go) in a goroutine so it doesn't block the pipeline.
func (tps *TransformationPipelineService) persistExecutionRecords(
	pipelineID string,
	messageID string,
	interfaceID string,
	startedAt time.Time,
	result *models.TransformationExecutionResult,
) {
	if tps.db == nil {
		return
	}
	ctx := context.Background()

	executionID := uuid.New().String()
	status := result.Status
	if status == "" || status == "in_progress" {
		status = "completed"
	}
	var errMsg string
	if len(result.Errors) > 0 {
		errMsg = result.Errors[0].Message
	}
	completedAt := result.CompletedAt
	totalMs := result.ExecutionTimeNs / 1_000_000

	stepsFailed := 0
	for _, s := range result.ExecutionLog {
		if !s.Success {
			stepsFailed++
		}
	}

	var outputData map[string]interface{}
	if result.Output != nil {
		outputData = map[string]interface{}{
			"size_bytes": result.Output.SizeBytes,
			"format":     result.Output.Format,
		}
	}
	outputJSON, _ := json.Marshal(outputData)
	execLogJSON, _ := json.Marshal(result.ExecutionLog)

	_, err := tps.db.ExecContext(ctx, `
		INSERT INTO transformation_executions
			(id, message_id, interface_id, pipeline_id, started_at, completed_at,
			 total_time_ms, status, steps_executed, steps_failed, output_data, error_message, execution_log)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT DO NOTHING`,
		executionID, messageID, interfaceID, pipelineID,
		startedAt, completedAt,
		totalMs, status,
		len(result.ExecutionLog), stepsFailed,
		outputJSON, errMsg, execLogJSON,
	)
	if err != nil {
		logger.Warn("persistExecutionRecords: failed to insert execution row", "error", err)
		return
	}

	// Insert one step_executions row per step
	for _, s := range result.ExecutionLog {
		stepStatus := "completed"
		if !s.Success {
			stepStatus = "failed"
		}

		var stepOutputJSON []byte
		if s.StepOutput != nil {
			// Merge OutputData + ExecutionDetails into a single JSON blob stored as step_output.
			// This is what feeds the UI — connector request/response lives in ExecutionDetails.
			merged := make(map[string]interface{})
			for k, v := range s.StepOutput.OutputData {
				merged[k] = v
			}
			for k, v := range s.StepOutput.ExecutionDetails {
				merged[k] = v
			}
			merged["success"] = s.StepOutput.Success
			merged["duration_ms"] = s.StepOutput.DurationMs
			if s.StepOutput.Error != "" {
				merged["error"] = s.StepOutput.Error
			}
			stepOutputJSON, _ = json.Marshal(merged)
		}

		completed := s.CompletedAt
		_, serr := tps.db.ExecContext(ctx, `
			INSERT INTO step_executions
				(id, execution_id, step_id, step_name, step_type,
				 started_at, completed_at, duration_ms, status,
				 error_message, namespace, step_alias, step_output)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
			ON CONFLICT DO NOTHING`,
			uuid.New().String(), executionID, s.StepID, s.StepName, s.StepType,
			s.StartedAt, completed, s.DurationMs, stepStatus,
			s.Error, s.Namespace, s.StepAlias, stepOutputJSON,
		)
		if serr != nil {
			logger.Warn("persistExecutionRecords: failed to insert step row",
				"step", s.StepName, "error", serr)
		}
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
