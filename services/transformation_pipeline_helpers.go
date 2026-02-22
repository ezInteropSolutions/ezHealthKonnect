// services/transformation_pipeline_helpers.go
// Helper methods for transformation pipeline service (MVC + OOB pattern)

package services

import (
	"context"
	"encoding/json"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	"ezhealthkonnect/services/executors/control"
	"fmt"
	"log"
	"strings"
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
// PHASE 2: Now uses PipelineExecutionContext for proper data isolation
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
		ExecutionLog:  []models.StepExecutionLog{}, // Initialize step execution log
	}

	// PHASE 2: Create execution context with isolated step outputs
	execCtx := &models.PipelineExecutionContext{
		Message:         inputData,
		StepOutputs:     make(map[string]models.StepOutput),
		VariableContext: models.NewPipelineVariableContext(), // NO-CODE: Flat namespace registry
		Metadata: map[string]interface{}{
			"pipeline_id":    pipeline.ID,
			"correlation_id": result.CorrelationID,
			"started_at":     startTime,
		},
	}

	log.Printf("🚀 [Pipeline] Initialized execution context for pipeline: %s", pipeline.PipelineName)

	// Extract pipeline-level defaults for error handling and retry
	pipelineRetryDefaults := executors.ParsePipelineRetryDefaults(pipeline.PipelineConfig)
	pipelineEHDefaults := executors.ParsePipelineErrorHandlingDefaults(pipeline.PipelineConfig)
	if pipelineRetryDefaults != nil {
		fmt.Printf("🔄 Pipeline-level retry defaults: max=%d, delay=%dms, backoff=%.1fx\n",
			pipelineRetryDefaults.MaxRetries, pipelineRetryDefaults.DelayMs, pipelineRetryDefaults.BackoffMultiplier)
	}
	if pipelineEHDefaults != nil {
		fmt.Printf("🛡️ Pipeline-level error handling defaults: onError=%s, defaultField=%s, defaultValue=%s\n",
			pipelineEHDefaults.OnError, pipelineEHDefaults.DefaultField, pipelineEHDefaults.DefaultValue)
	}

	// Track the final output after all steps (starts with input, updated by each step)
	var finalOutput map[string]interface{} = inputData

	// Build step index for loop child step execution
	stepByID := make(map[string]*models.TransformationStep)
	for idx := range pipeline.Steps {
		stepCopy := pipeline.Steps[idx]
		stepByID[stepCopy.ID] = &stepCopy
	}

	// Create child executor callback for loop steps
	loopChildExecutor := tps.createLoopChildExecutor(ctx, stepByID, execCtx)

	// Execute steps with conditional routing support
	// skipStepIDs tracks step IDs that should be skipped (from exclusive branch routing)
	skipStepIDs := make(map[string]bool)
	loopChildStepIDs := make(map[string]bool) // Child steps executed by loop containers

	// PRE-SCAN: Build loopChildStepIDs map BEFORE execution starts
	// This prevents child steps from executing as top-level steps when they appear
	// earlier in the pipeline sequence than their parent loop step
	for _, s := range pipeline.Steps {
		if s.StepType == "control.loop" {
			if childIDs, ok := s.Config["childStepIds"].([]interface{}); ok {
				for _, cid := range childIDs {
					if cidStr, ok := cid.(string); ok {
						loopChildStepIDs[cidStr] = true
						fmt.Printf("   📌 Pre-marked loop child step: %s (parent loop: %s)\n", cidStr, s.StepName)
					}
				}
			}
		}
	}

	i := 0
	fmt.Printf("🔄🔄🔄 [ExecutePipeline] Starting execution loop with %d steps\n", len(pipeline.Steps))
	for i < len(pipeline.Steps) {
		step := pipeline.Steps[i]

		// Check if this step should be skipped due to exclusive branch routing
		if skipStepIDs[step.ID] {
			fmt.Printf("⏭️⏭️⏭️ SKIPPING step (exclusive branch): %s (ID: %s)\n", step.StepName, step.ID)
			i++
			continue
		}

		// ✅ Skip child steps already executed by a loop container
		if loopChildStepIDs[step.ID] {
			fmt.Printf("⏭️ Skipping step (executed by loop): %s (ID: %s)\n", step.StepName, step.ID)
			i++
			continue
		}

		if !step.Enabled {
			fmt.Printf("⏭️ Skipping disabled step: %s\n", step.StepName)
			i++
			continue
		}

		// TEST MODE: Skip inbound connector (user provides test data directly)
		if step.StepType == "connector.inbound" && models.IsTestMode(ctx) {
			fmt.Printf("🧪 [Test Mode] Skipping inbound connector: %s (test data provided directly)\n", step.StepName)

			connectorType := "unknown"
			if ct, ok := step.Config["connectorType"].(string); ok {
				connectorType = ct
			}

			namespace := tps.generateStepNamespace(&step, i)
			stepLog := models.StepExecutionLog{
				StepID:      step.ID,
				StepName:    step.StepName,
				StepType:    step.StepType,
				Namespace:   namespace,
				StartedAt:   time.Now(),
				CompletedAt: time.Now(),
				DurationMs:  0,
				Success:     true,
			}
			stepLog.StepOutput = &models.StepOutput{
				StepID:    step.ID,
				StepName:  step.StepName,
				StepType:  step.StepType,
				Namespace: namespace,
				Sequence:  i,
				OutputData: map[string]interface{}{
					"success":        true,
					"connector_type": connectorType,
					"message":        "Test data provided directly - inbound connector skipped in test mode",
					"test_mode":      true,
				},
				Success:    true,
				DurationMs: 0,
			}
			execCtx.StepOutputs[namespace] = *stepLog.StepOutput
			result.ExecutionLog = append(result.ExecutionLog, stepLog)
			i++
			continue
		}

		stepStartTime := time.Now()
		fmt.Printf("▶️▶️▶️ EXECUTING step %d/%d: %s (type: %s, ID: %s)\n", i+1, len(pipeline.Steps), step.StepName, step.StepType, step.ID)

		// ✅ CONTAINER STEP HANDLING: Inject child executor callback
		if step.StepType == "control.loop" {
			executor := tps.executorRegistry.GetExecutor(step.StepType)
			if loopExecutor, ok := executor.(*control.LoopExecutor); ok {
				loopExecutor.SetChildExecutor(loopChildExecutor)
				fmt.Printf("🔄 Injected child executor into loop step: %s\n", step.StepName)
			}
		}

		// PHASE 2: Generate namespace for this step
		namespace := tps.generateStepNamespace(&step, i)
		log.Printf("   Namespace: %s", namespace)

		// Debug: Log step config keys for troubleshooting error handling resolution
		configKeys := make([]string, 0, len(step.Config))
		for k := range step.Config {
			configKeys = append(configKeys, k)
		}
		fmt.Printf("🔍 Step '%s' config keys: %v\n", step.StepName, configKeys)

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
			fmt.Printf("🔍 Step '%s' has no error handling config (step-level or pipeline-level)\n", step.StepName)
		}

		// Execute step with retry (ExecuteWithRetry handles single-exec when retryConfig is nil)
		var output map[string]interface{}
		var stepOutput *models.StepOutput
		var stepErr error

		retryResult := executors.ExecuteWithRetry(ctx, retryConfig, func(attempt int) (map[string]interface{}, error) {
			o, so, e := tps.executeStepWithContext(ctx, &step, execCtx, i)
			// Store stepOutput from last attempt (for use after retry loop)
			stepOutput = so
			output = o
			return o, e
		})

		stepErr = retryResult.Err
		output = retryResult.Output
		originalErr := stepErr // preserve for logging
		stepDuration := time.Since(stepStartTime)

		// Apply error handling if step failed after all retries
		errorWasCaught := false
		if stepErr != nil && ehConfig != nil {
			fmt.Printf("🛡️ APPLYING error handling for: %s (error: %s)\n", step.StepName, stepErr.Error())
			output, stepErr = executors.ApplyErrorHandling(ehConfig, stepErr, output, execCtx.Message, step.StepName)
			if stepErr == nil {
				errorWasCaught = true
				fmt.Printf("🛡️ Error CAUGHT for: %s\n", step.StepName)
			}
		} else if stepErr != nil {
			fmt.Printf("⚠️ Step '%s' failed but NO error handler\n", step.StepName)
		}

		// Create step execution log
		var stepAliasStr string
		if step.StepAlias != nil {
			stepAliasStr = *step.StepAlias
		}

		stepLog := models.StepExecutionLog{
			StepID:      step.ID,
			StepName:    step.StepName,
			StepType:    step.StepType,
			Namespace:   namespace,
			StepAlias:   stepAliasStr,
			StartedAt:   stepStartTime,
			CompletedAt: time.Now(),
			DurationMs:  stepDuration.Milliseconds(),
			Success:     stepErr == nil,
		}

		if stepErr != nil {
			stepLog.Error = stepErr.Error()
			log.Printf("❌ Step failed: %s - %v", step.StepName, stepErr)

			// Record error in step output
			if stepOutput != nil {
				stepOutput.Success = false
				stepOutput.Error = stepErr.Error()
			}

			// Record error
			result.Errors = append(result.Errors, models.TransformationError{
				Step:      step.StepName,
				Message:   stepErr.Error(),
				Timestamp: time.Now(),
			})

			// Handle error based on strategy
			if step.OnErrorStrategy == "fail" || step.Required {
				result.ExecutionLog = append(result.ExecutionLog, stepLog)
				result.Status = "failed"
				result.CompletedAt = time.Now()
				result.ExecutionTimeNs = time.Since(startTime).Nanoseconds()
				return result, fmt.Errorf("pipeline failed at step %s: %w", step.StepName, stepErr)
			} else {
				log.Printf("⚠️  Continuing despite error (strategy: %s)", step.OnErrorStrategy)
			}
		} else if errorWasCaught {
			// Error was caught by handler — mark step as success with caught info
			fmt.Printf("🛡️ Step error caught: %s (error: %s, took %dms)\n", step.StepName, originalErr.Error(), stepDuration.Milliseconds())

			// Update step output with caught info
			if stepOutput != nil {
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
			} else {
				// Create step output if executor didn't set one
				stepOutput = &models.StepOutput{
					StepID:    step.ID,
					StepName:  step.StepName,
					StepType:  step.StepType,
					Namespace: namespace,
					Sequence:  i,
					Success:   true,
					Error:     fmt.Sprintf("caught: %s", originalErr.Error()),
					OutputData: map[string]interface{}{
						"error_caught":  originalErr.Error(),
						"error_handler": ehConfig.OnError,
					},
					DurationMs: stepDuration.Milliseconds(),
				}
				if ehConfig.DefaultField != "" && ehConfig.DefaultValue != "" {
					stepOutput.OutputData["default_applied_field"] = ehConfig.DefaultField
					stepOutput.OutputData["default_applied_value"] = ehConfig.DefaultValue
				}
			}

			// Store step output and update message
			execCtx.StepOutputs[namespace] = *stepOutput
			stepLog.StepOutput = stepOutput

			if output != nil {
				finalOutput = output
				execCtx.Message = output
			}
		} else {
			log.Printf("✅ Step completed: %s (took %dms)", step.StepName, stepDuration.Milliseconds())

			// PHASE 2: Store step output in context (isolated)
			if stepOutput != nil {
				execCtx.StepOutputs[namespace] = *stepOutput
				stepLog.StepOutput = stepOutput
				log.Printf("   Stored output in namespace: %s", namespace)

				// NO-CODE: Register variables in flat namespace for easy access
				if stepOutput.OutputData != nil {
					execCtx.VariableContext.RegisterStepOutput(step.StepName, step.StepType, stepOutput.OutputData)
					log.Printf("   📋 Registered %d variables for no-code access", len(stepOutput.OutputData))
				}
			}

			// PHASE 2: Track the final output from this step
			if output != nil {
				finalOutput = output
				log.Printf("🔍 [DEBUG] Updated finalOutput after step %s", step.StepName)
			}

			// PHASE 2: Message may have been modified by step - MERGE changes, don't replace
			// This preserves segmentGroups, observationGroups, etc. from the original parsed message
			if output != nil {
				if msg, ok := output["message"].(map[string]interface{}); ok {
					// Merge step's message output INTO existing message (preserving original keys)
					if execCtx.Message == nil {
						execCtx.Message = msg
					} else {
						for k, v := range msg {
							execCtx.Message[k] = v
						}
					}
				}

				// Preserve enriched data from output (field_mapping, etc.)
				if enriched, ok := output["enriched"].(map[string]interface{}); ok {
					if execCtx.Message == nil {
						execCtx.Message = make(map[string]interface{})
					}
					execCtx.Message["enriched"] = enriched
					log.Printf("🔍 [DEBUG] Preserved enriched data with keys: %v", getMapKeys(enriched))
				}
			}
		}

		// Add step log to execution log
		result.ExecutionLog = append(result.ExecutionLog, stepLog)

		// PHASE 3: DEBUG - Check what's in metadata
		log.Printf("🔍 [DEBUG] After step %d, execCtx.Metadata keys: %v", i+1, getMapKeys(execCtx.Metadata))
		if routing, ok := execCtx.Metadata["_routing"]; ok {
			log.Printf("🔍 [DEBUG] _routing exists, type: %T, value: %+v", routing, routing)
		}

		// PHASE 2: Check for conditional routing (from context metadata)
		fmt.Printf("🔍🔍🔍 [DEBUG] Checking for _routing in metadata. Metadata keys: %v\n", getMapKeys(execCtx.Metadata))
		if routingDirective, ok := execCtx.Metadata["_routing"].(map[string]interface{}); ok {
			fmt.Printf("🔀🔀🔀 ROUTING DIRECTIVE FOUND: %+v\n", routingDirective)

			// Check for skipSteps (exclusive branch support) - these steps will be skipped
			if skipSteps, ok := routingDirective["skipSteps"].([]interface{}); ok {
				fmt.Printf("🔀🔀🔀 skipSteps found ([]interface{}): %v\n", skipSteps)
				for _, sid := range skipSteps {
					if stepID, ok := sid.(string); ok {
						skipStepIDs[stepID] = true
						fmt.Printf("🔀🔀🔀 MARKING step %s to skip (exclusive branch)\n", stepID)
					}
				}
				// Clear skipSteps after processing
				delete(routingDirective, "skipSteps")
			}
			// Also handle []string type (from some code paths)
			if skipSteps, ok := routingDirective["skipSteps"].([]string); ok {
				fmt.Printf("🔀🔀🔀 skipSteps found ([]string): %v\n", skipSteps)
				for _, stepID := range skipSteps {
					skipStepIDs[stepID] = true
					fmt.Printf("🔀🔀🔀 MARKING step %s to skip (exclusive branch)\n", stepID)
				}
				delete(routingDirective, "skipSteps")
			}

			if nextStepId, ok := routingDirective["nextStep"].(string); ok {
				// Find step by ID
				nextIndex := tps.findStepIndexById(pipeline.Steps, nextStepId)
				log.Printf("🔍 [DEBUG] Looking for step ID %s, found at index %d (current index: %d)", nextStepId, nextIndex, i)
				if nextIndex >= 0 && nextIndex > i {
					log.Printf("🔀 Conditional routing: Jumping from step %d to step %d (ID: %s)", i+1, nextIndex+1, nextStepId)
					i = nextIndex

					// Clear routing directive after use
					delete(routingDirective, "nextStep")
					continue
				} else if nextIndex >= 0 && nextIndex <= i {
					log.Printf("⚠️  Warning: Backward routing to step %d not allowed (security: forward-only)", nextIndex+1)
				} else {
					log.Printf("⚠️  Warning: Step ID %s not found, continuing sequentially", nextStepId)
				}
			} else {
				log.Printf("🔍 [DEBUG] Routing directive exists but no nextStep found")
			}
		} else {
			log.Printf("🔍 [DEBUG] No routing directive in metadata after step %d", i+1)
		}

		// Normal sequential execution
		i++
	}

	// PHASE 2: Pipeline completed successfully - build Input/Output contexts
	result.Status = "completed"
	result.CompletedAt = time.Now()
	executionDuration := time.Since(startTime)
	result.ExecutionTimeNs = executionDuration.Nanoseconds()

	// Build Input context (what came in)
	result.Input = buildInputContext(inputData, startTime)

	// Build Output context (what went out - final transformed message)
	// Use the final output from the last executed step
	log.Printf("🔍 [DEBUG] Building output context from finalOutput (keys: %v)", getMapKeys(finalOutput))
	result.Output = buildOutputContext(finalOutput, result.CompletedAt)

	// PHASE 2: Extract delivery payload from metadata (if present)
	if val, exists := execCtx.Metadata["_deliveryPayload"]; exists {
		log.Printf("🔍 [DEBUG] _deliveryPayload exists in metadata, type: %T", val)
		if deliveryPayload, ok := val.(*models.DeliveryPayload); ok {
			result.DeliveryPayload = deliveryPayload
			log.Printf("📦 Delivery payload extracted: %s (format: %s, destination: %s)",
				deliveryPayload.PayloadID, deliveryPayload.Format, deliveryPayload.DestinationType)
		} else {
			log.Printf("⚠️  [DEBUG] Type assertion failed for _deliveryPayload")
		}
	}

	log.Printf("✅ [Pipeline] Execution complete: %d steps executed, %d step outputs stored",
		len(result.ExecutionLog), len(execCtx.StepOutputs))

	return result, nil
}

// createLoopChildExecutor creates a callback for loop steps to execute child steps
func (tps *TransformationPipelineService) createLoopChildExecutor(
	ctx context.Context,
	stepByID map[string]*models.TransformationStep,
	execCtx *models.PipelineExecutionContext,
) control.ChildStepExecutor {
	return func(
		childCtx context.Context,
		stepID string,
		inputData map[string]interface{},
	) (map[string]interface{}, string, error) {
		step, exists := stepByID[stepID]
		if !exists {
			availableIDs := make([]string, 0, len(stepByID))
			for id, s := range stepByID {
				availableIDs = append(availableIDs, fmt.Sprintf("%s(%s)", id[:8], s.StepName))
			}
			log.Printf("❌ Child step %s not found. Available steps: %v", stepID, availableIDs)
			return nil, "", fmt.Errorf("child step not found: %s (available: %d steps)", stepID, len(stepByID))
		}

		log.Printf("     🔧 Executing loop child step: %s (%s)", step.StepName, step.StepType)

		// Execute the child step via the registry
		output, err := tps.executorRegistry.ExecuteStep(childCtx, *step, inputData)
		if err != nil {
			return nil, step.StepName, err
		}

		return output, step.StepName, nil
	}
}

// findStepIndexById finds the index of a step by its ID
func (tps *TransformationPipelineService) findStepIndexById(steps []models.TransformationStep, stepId string) int {
	for i, step := range steps {
		if step.ID == stepId {
			return i
		}
	}
	return -1
}

// generateStepNamespace generates a namespace for a step (PHASE 2)
// Format: "alias_shortID" or "stepName_shortID" if no alias
// Example: "empi_b4c9f1" or "validatePatient_a7f2c3"
func (tps *TransformationPipelineService) generateStepNamespace(step *models.TransformationStep, sequence int) string {
	// Use alias if provided, otherwise use step name
	var baseName string
	if step.StepAlias != nil && *step.StepAlias != "" {
		baseName = *step.StepAlias
	} else {
		baseName = step.StepName
	}

	// Clean the base name (remove spaces, special chars)
	baseName = strings.ReplaceAll(baseName, " ", "_")
	baseName = strings.ToLower(baseName)

	// Generate short ID from step ID (first 6 chars)
	shortID := step.ID
	if len(shortID) > 6 {
		shortID = shortID[:6]
	}

	return fmt.Sprintf("%s_%s", baseName, shortID)
}

// executeStepWithContext executes a step with execution context (PHASE 2)
// Returns: output map, step output, error
func (tps *TransformationPipelineService) executeStepWithContext(
	ctx context.Context,
	step *models.TransformationStep,
	execCtx *models.PipelineExecutionContext,
	sequence int,
) (map[string]interface{}, *models.StepOutput, error) {
	// PHASE 2: Prepare input data with context
	// This maintains backward compatibility with Phase 1
	inputData := map[string]interface{}{
		"message":          execCtx.Message,
		"_metadata":        execCtx.Metadata,
		"_variableContext": execCtx.VariableContext, // NO-CODE: Pass variable registry to executors
	}

	// Add routing if exists
	if routing, ok := execCtx.Metadata["_routing"]; ok {
		inputData["_routing"] = routing
	}

	// CRITICAL: Preserve enriched data from previous steps
	// This allows field_mapping output to be accessed by script_enrichment, etc.
	if execCtx.Message != nil {
		if enriched, ok := execCtx.Message["enriched"].(map[string]interface{}); ok {
			inputData["enriched"] = enriched
		}
	}

	// Execute step using existing executor registry
	output, err := tps.executorRegistry.ExecuteStep(ctx, *step, inputData)

	if err != nil {
		return output, nil, err
	}

	// PHASE 2: Extract step output from the result
	var stepAlias string
	if step.StepAlias != nil {
		stepAlias = *step.StepAlias
	}

	stepOutput := &models.StepOutput{
		StepID:     step.ID,
		StepName:   step.StepName,
		StepAlias:  stepAlias,
		StepType:   step.StepType,
		Namespace:  tps.generateStepNamespace(step, sequence),
		Sequence:   sequence,
		Success:    true,
		DurationMs: 0, // Will be set by caller
	}

	// Extract step-specific output from _stepOutput field (Phase 1 compatible)
	if stepOutputData, ok := output["_stepOutput"].(map[string]interface{}); ok {
		stepOutput.OutputData = stepOutputData
		delete(output, "_stepOutput") // Remove from output after extraction
	} else {
		// Fallback: use the entire output as step output (excluding internal fields and large message data)
		stepOutput.OutputData = make(map[string]interface{})
		for key, value := range output {
			// Skip internal/metadata fields (they start with _)
			if len(key) > 0 && key[0] == '_' {
				continue
			}
			// Skip the full message object (too large, not step-specific)
			if key == "message" || key == "enhancedSegments" || key == "raw" {
				continue
			}
			// Include enriched data - steps store their outputs here
			// Note: We do NOT skip "enriched" because that's where step outputs are stored
			stepOutput.OutputData[key] = value
		}
	}

	// Extract execution details from _executionDetails into dedicated field
	// These will be merged into step_metadata by the controller (along with duration_ms, success)
	if executionDetails, ok := output["_executionDetails"].(map[string]interface{}); ok {
		stepOutput.ExecutionDetails = executionDetails
		delete(output, "_executionDetails") // Remove from output after extraction
	}

	// Update context metadata if step modified it
	if metadata, ok := output["_metadata"].(map[string]interface{}); ok {
		execCtx.Metadata = metadata
	}

	// Update routing if step set it
	if routing, ok := output["_routing"].(map[string]interface{}); ok {
		execCtx.Metadata["_routing"] = routing
		log.Printf("🔀🔀🔀 [executeStepWithContext] ROUTING DETECTED! nextStep=%v, skipSteps=%v",
			routing["nextStep"], routing["skipSteps"])
	}

	return output, stepOutput, nil
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

// buildInputContext creates MessageContext for input data
func buildInputContext(inputData map[string]interface{}, receivedAt time.Time) *models.MessageContext {
	// Detect format and message type from input data
	format := detectMessageFormat(inputData)
	messageType := extractMessageType(inputData, format)
	version := extractVersion(inputData, format)

	// Calculate payload size
	payloadBytes, _ := json.Marshal(inputData)
	sizeBytes := int64(len(payloadBytes))

	return &models.MessageContext{
		Format:      format,
		MessageType: messageType,
		Version:     version,
		SizeBytes:   sizeBytes,
		ReceivedAt:  receivedAt,
		// INCLUDE payload for test mode - this is the parsed input message
		// In production, this would be a reference to MongoDB
		Payload: inputData,
		Metadata: map[string]interface{}{
			"dictionary_used": inputData["dictionaryUsed"],
			"schema_loaded":   inputData["schemaLoaded"],
		},
	}
}

// buildOutputContext creates MessageContext for output data
// CLEAN OUTPUT: Only includes the final transformed message (FHIR bundle, etc.)
// Step-specific outputs (enrichment data, validation results) belong in ExecutionLog, not here
func buildOutputContext(outputData map[string]interface{}, transformedAt time.Time) *models.MessageContext {
	// Determine the actual output payload - prioritize explicit transformation outputs
	var actualPayload interface{}
	var format string
	var messageType string

	// Priority 1: FHIR bundle from HL7→FHIR transformation
	if fhirBundle, hasFHIR := outputData["fhirBundle"].(map[string]interface{}); hasFHIR {
		actualPayload = fhirBundle
		format = "fhir-r4"
		messageType = "Bundle"
		log.Printf("🎯 [Output Context] Found FHIR bundle - using as payload")
	} else {
		// Priority 2: For non-FHIR outputs, extract ONLY the core message
		// Exclude internal metadata and step-specific data
		cleanPayload := make(map[string]interface{})

		// Core message fields to preserve
		coreFields := []string{
			"raw", "enhancedSegments", "segmentGroups", "observationGroups",
			"messageType", "version", "dictionaryUsed", "schemaLoaded", "segmentOrder",
		}

		for _, field := range coreFields {
			if val, exists := outputData[field]; exists {
				cleanPayload[field] = val
			}
		}

		// If we extracted some core fields, use that; otherwise mark as empty
		if len(cleanPayload) > 0 {
			actualPayload = cleanPayload
			format = detectMessageFormat(cleanPayload)
			messageType = extractMessageType(cleanPayload, format)
			log.Printf("📦 [Output Context] Extracted core message fields (%d fields)", len(cleanPayload))
		} else {
			// Fallback: no recognizable output
			actualPayload = map[string]interface{}{
				"_note": "No transformed output generated - check pipeline steps",
			}
			format = "unknown"
			messageType = ""
			log.Printf("⚠️ [Output Context] No recognizable output payload")
		}
	}

	// Calculate payload size
	payloadBytes, _ := json.Marshal(actualPayload)
	sizeBytes := int64(len(payloadBytes))

	log.Printf("✅ [Output Context] Format: %s, Type: %s, Size: %d bytes", format, messageType, sizeBytes)

	return &models.MessageContext{
		Format:        format,
		MessageType:   messageType,
		SizeBytes:     sizeBytes,
		TransformedAt: transformedAt,
		Payload:       actualPayload, // Clean output - only the transformed message
		Metadata:      extractOutputMetadata(outputData, format),
	}
}

// detectMessageFormat detects the format of a message
func detectMessageFormat(data map[string]interface{}) string {
	// Check for FHIR Bundle
	if resourceType, ok := data["resourceType"].(string); ok {
		if resourceType == "Bundle" {
			return "fhir-r4"
		}
		return "fhir-r4" // Any FHIR resource
	}

	// Check for HL7 parsed format (has enhancedSegments)
	if _, ok := data["enhancedSegments"]; ok {
		return "hl7v2"
	}

	// Check for message type field (HL7 indicator)
	if msgType, ok := data["messageType"]; ok {
		if msgTypeMap, ok := msgType.(map[string]interface{}); ok {
			if _, hasCode := msgTypeMap["code"]; hasCode {
				return "hl7v2"
			}
		}
	}

	// Default to generic JSON
	return "json"
}

// extractMessageType extracts the message type from data
func extractMessageType(data map[string]interface{}, format string) string {
	switch format {
	case "hl7v2":
		// Check for messageType field in parsed HL7
		if msgType, ok := data["messageType"].(map[string]interface{}); ok {
			if code, ok := msgType["code"].(string); ok {
				if event, ok := msgType["event"].(string); ok {
					return fmt.Sprintf("%s^%s", code, event)
				}
				return code
			}
		}

	case "fhir-r4":
		// Check for FHIR resourceType
		if resourceType, ok := data["resourceType"].(string); ok {
			return resourceType
		}
	}

	return ""
}

// extractVersion extracts the version from data
func extractVersion(data map[string]interface{}, format string) string {
	switch format {
	case "hl7v2":
		if version, ok := data["version"].(string); ok {
			return version
		}

	case "fhir-r4":
		return "4.0.1" // Default FHIR R4 version
	}

	return ""
}

// extractOutputMetadata extracts format-specific metadata from output
func extractOutputMetadata(data map[string]interface{}, format string) map[string]interface{} {
	metadata := make(map[string]interface{})

	switch format {
	case "fhir-r4":
		// Extract FHIR Bundle metadata
		if resourceType, ok := data["resourceType"].(string); ok && resourceType == "Bundle" {
			metadata["resource_type"] = resourceType

			if bundleType, ok := data["type"].(string); ok {
				metadata["bundle_type"] = bundleType
			}

			if entry, ok := data["entry"].([]interface{}); ok {
				metadata["resource_count"] = len(entry)

				// Extract resource types
				resourceTypes := make([]string, 0)
				for _, e := range entry {
					if entryMap, ok := e.(map[string]interface{}); ok {
						if resource, ok := entryMap["resource"].(map[string]interface{}); ok {
							if rt, ok := resource["resourceType"].(string); ok {
								resourceTypes = append(resourceTypes, rt)
							}
						}
					}
				}
				metadata["resource_types"] = resourceTypes
			}
		}
	}

	return metadata
}

