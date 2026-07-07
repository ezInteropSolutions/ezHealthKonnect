// services/transformation_pipeline_helpers.go
// Helper methods for transformation pipeline service (MVC + OOB pattern)

package services

import (
	"context"
	"encoding/json"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	"ezhealthkonnect/services/executors/control"
	"ezhealthkonnect/services/logger"
	"ezhealthkonnect/services/storage"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
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

	// Mirror processing/engine_message_processor.go's ctx.WithValue("interface_id", ...)
	// injection for REAL message processing — ExecutePipeline is a SEPARATE entry
	// point (used by TestPipeline's ad-hoc test runs AND ProcessingEngine.ReprocessMessage,
	// i.e. the UI's "Re-run" button) that never set this on ctx at all before, so any
	// executor resolving its own interface ID via ctx.Value("interface_id") (e.g.
	// cda_to_fhir_executor.go's three-tier CDA mapping-override lookup) silently fell
	// back to pure-OOB behavior for every Test Pipeline run and every Re-run, even on an
	// interface with real custom mapping overrides saved. inputData can't carry this
	// instead here — executeStepWithContext wraps it as inputData["message"], not a
	// top-level "interfaceId"/"interface_id" key, which is exactly the shape
	// cda_to_fhir_executor.go's own comment already documents checking ctx as the
	// fallback for.
	ctx = context.WithValue(ctx, "interface_id", pipeline.InterfaceID)

	// Resolve the interface's debug flag once per run and inject it into every
	// step's Config, mirroring ExecuteTransformation's own injection
	// (transformation_pipeline_service.go) — ExecutePipeline is a SEPARATE entry
	// point (used by ProcessingEngine.ReprocessMessage, i.e. the UI's "Re-run"
	// button) that never touches step.Config otherwise, so without this, CDA
	// deep-lineage capture silently stays off on every re-run even when the
	// interface has debug logging enabled. Best-effort: errors never block the
	// pipeline, deep lineage simply stays off.
	if _, deepLineage, _ := ResolveInterfaceDebugLevel(tps.db, pipeline.InterfaceID); deepLineage {
		for i := range pipeline.Steps {
			if pipeline.Steps[i].Config == nil {
				pipeline.Steps[i].Config = make(map[string]interface{})
			}
			pipeline.Steps[i].Config["_cdaDeepLineage"] = true
		}
	}

	// PHASE 2: Enrich message envelope with _semantic_index + _sensitivity_map.
	// enrichMessageEnvelope is idempotent — safe to call on already-enriched messages.
	enrichMessageEnvelope(execCtx.Message)
	// Register semantic message paths into the variable context for no-code autocomplete.
	buildInitialVariableContext(execCtx.Message, execCtx.VariableContext)

	// Extract pipeline-level defaults for error handling and retry
	pipelineRetryDefaults := executors.ParsePipelineRetryDefaults(pipeline.PipelineConfig)
	pipelineEHDefaults := executors.ParsePipelineErrorHandlingDefaults(pipeline.PipelineConfig)
	if pipelineRetryDefaults != nil {
		logger.Debug("pipeline-level retry defaults",
			"interface_id", pipeline.InterfaceID,
			"correlation_id", result.CorrelationID,
			"max_retries", pipelineRetryDefaults.MaxRetries,
			"delay_ms", pipelineRetryDefaults.DelayMs,
			"backoff_multiplier", pipelineRetryDefaults.BackoffMultiplier)
	}
	if pipelineEHDefaults != nil {
		logger.Debug("pipeline-level error handling defaults",
			"interface_id", pipeline.InterfaceID,
			"correlation_id", result.CorrelationID,
			"on_error", pipelineEHDefaults.OnError,
			"default_field", pipelineEHDefaults.DefaultField)
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
						logger.Debug("pre-marked loop child step",
							"interface_id", pipeline.InterfaceID,
							"step_id", cidStr, "parent_loop", s.StepName)
					}
				}
			}
		}
	}

	i := 0
	logger.Debug("starting pipeline execution loop",
		"interface_id", pipeline.InterfaceID,
		"correlation_id", result.CorrelationID,
		"step_count", len(pipeline.Steps))
	for i < len(pipeline.Steps) {
		step := pipeline.Steps[i]

		// Check if this step should be skipped due to exclusive branch routing
		if skipStepIDs[step.ID] {
			logger.Debug("skipping step (exclusive branch)",
				"interface_id", pipeline.InterfaceID,
				"correlation_id", result.CorrelationID,
				"step_name", step.StepName, "step_id", step.ID)
			i++
			continue
		}

		// ✅ Skip child steps already executed by a loop container
		if loopChildStepIDs[step.ID] {
			logger.Debug("skipping step (loop child)",
				"interface_id", pipeline.InterfaceID,
				"correlation_id", result.CorrelationID,
				"step_name", step.StepName, "step_id", step.ID)
			i++
			continue
		}

		if !step.Enabled {
			logger.Debug("skipping disabled step",
				"interface_id", pipeline.InterfaceID,
				"correlation_id", result.CorrelationID,
				"step_name", step.StepName)
			i++
			continue
		}

		// ALWAYS SKIP connector.inbound during pipeline execution.
		//
		// The engine's ActivateInterface() already started the listener (TCP/MLLP, etc.)
		// and routed the message into this pipeline. If we let InboundConnectorExecutor
		// call connector.Start() here it would try to bind the same port again →
		// "bind: address already in use". Instead we mark the step as successful and
		// expose the already-received message as the step output so downstream steps
		// can reference steps.<ns>.step_output.content etc.
		if step.StepType == "connector.inbound" {
			connectorType := "unknown"
			if ct, ok := step.Config["connectorType"].(string); ok {
				connectorType = ct
			}

			isTest := models.IsTestMode(ctx)
			if isTest {
				logger.Debug("test mode: skipping inbound connector",
					"interface_id", pipeline.InterfaceID,
					"correlation_id", result.CorrelationID,
					"step_name", step.StepName)
			} else {
				logger.Debug("production mode: inbound connector already running — injecting received message",
					"interface_id", pipeline.InterfaceID,
					"correlation_id", result.CorrelationID,
					"step_name", step.StepName)
			}

			// Build output: in production inject the live message; in test use placeholder.
			stepOutputData := map[string]interface{}{
				"success":        true,
				"connector_type": connectorType,
			}
			if isTest {
				stepOutputData["message"] = "Test data provided directly - inbound connector skipped in test mode"
				stepOutputData["test_mode"] = true
			} else {
				// Expose the raw message so downstream steps can access it.
				stepOutputData["content"]      = execCtx.Message["raw"]
				stepOutputData["message_type"] = execCtx.Message["messageType"]
				stepOutputData["source_type"]  = connectorType
				stepOutputData["received"]     = true
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
				StepID:     step.ID,
				StepName:   step.StepName,
				StepType:   step.StepType,
				Namespace:  namespace,
				Sequence:   i,
				OutputData: stepOutputData,
				Success:    true,
				DurationMs: 0,
			}
			execCtx.StepOutputs[namespace] = *stepLog.StepOutput
			result.ExecutionLog = append(result.ExecutionLog, stepLog)
			i++
			continue
		}

		stepStartTime := time.Now()
		logger.Debug("executing step",
			"interface_id", pipeline.InterfaceID,
			"correlation_id", result.CorrelationID,
			"step_index", i+1, "step_count", len(pipeline.Steps),
			"step_name", step.StepName, "step_type", step.StepType)

		// PHASE 3: Egress PHI check — warn before any outbound delivery step.
		if step.StepType == "connector.outbound" {
			if violations := checkEgressPHI(execCtx.Message); len(violations) > 0 {
				log.Printf("⚠️  [PHI EGRESS] Step '%s' — %d PHI field(s) not masked before outbound: %v",
					step.StepName, len(violations), violations)
				// Record in StepErrors for UI visibility (non-blocking by default)
				if result.StepErrors == nil {
					result.StepErrors = make(map[string]models.StepErrorDetail)
				}
				ns := tps.generateStepNamespace(&step, i)
				result.StepErrors[ns] = models.StepErrorDetail{
					StepID:        step.ID,
					StepName:      step.StepName,
					Namespace:     ns,
					Error:         fmt.Sprintf("PHI fields not masked before egress: %v", violations),
					ErrorStrategy: "warn",
					PHIViolations: violations,
				}
				// If step config specifies egressPolicy=block, return an error
				if policy, ok := step.Config["egressPolicy"].(string); ok && policy == "block" {
					return result, fmt.Errorf("egress blocked: PHI fields not masked before outbound step '%s': %v", step.StepName, violations)
				}
			}
		}

		// ✅ CONTAINER STEP HANDLING: Inject child executor callback
		if step.StepType == "control.loop" {
			executor := tps.executorRegistry.GetExecutor(step.StepType)
			if loopExecutor, ok := executor.(*control.LoopExecutor); ok {
				loopExecutor.SetChildExecutor(loopChildExecutor)
				logger.Debug("injected child executor into loop step",
					"interface_id", pipeline.InterfaceID,
					"correlation_id", result.CorrelationID,
					"step_name", step.StepName)
			}
		}

		// PHASE 2: Generate namespace for this step
		namespace := tps.generateStepNamespace(&step, i)
		log.Printf("   Namespace: %s", namespace)

		// Resolve retry config: step override > pipeline default > nil
		retryConfig := executors.ResolveRetryConfig(step.Config, pipelineRetryDefaults)
		if retryConfig != nil {
			logger.Debug("retry enabled for step",
				"interface_id", pipeline.InterfaceID,
				"correlation_id", result.CorrelationID,
				"step_name", step.StepName,
				"max_retries", retryConfig.MaxRetries)
		}

		// Resolve error handling config: step override > pipeline default > nil
		ehConfig := executors.ResolveErrorHandlingConfig(step.Config, pipelineEHDefaults)

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
			logger.Debug("applying error handling",
				"interface_id", pipeline.InterfaceID,
				"correlation_id", result.CorrelationID,
				"step_name", step.StepName, "error", stepErr.Error())
			output, stepErr = executors.ApplyErrorHandling(ehConfig, stepErr, output, execCtx.Message, step.StepName)
			if stepErr == nil {
				errorWasCaught = true
				logger.Debug("error caught by handler",
					"interface_id", pipeline.InterfaceID,
					"correlation_id", result.CorrelationID,
					"step_name", step.StepName)
			}
		} else if stepErr != nil {
			logger.Warn("step failed without error handler",
				"interface_id", pipeline.InterfaceID,
				"correlation_id", result.CorrelationID,
				"step_name", step.StepName, "error", stepErr.Error())
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
			logger.Info("step error caught by handler",
				"interface_id", pipeline.InterfaceID,
				"correlation_id", result.CorrelationID,
				"step_name", step.StepName,
				"error", originalErr.Error(),
				"duration_ms", stepDuration.Milliseconds())

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

		// Append a structured log entry for every pipeline step so the Logs tab
		// shows exactly which step the message reached and whether it succeeded.
		// connector.inbound is skipped (logged at receive time by the engine).
		if tps.objectStorage != nil && step.StepType != "connector.inbound" {
			ifaceIDForLog, _ := ctx.Value("interface_id").(string)
			msgIDForLog, _ := ctx.Value("message_id").(string)
			if ifaceIDForLog == "" {
				ifaceIDForLog = pipeline.InterfaceID
			}
			if msgIDForLog != "" && ifaceIDForLog != "" {
				stage, msg, level := pipelineStepLogEntry(step.StepType, step.StepName, stepErr)
				fields := map[string]interface{}{
					"step_name":   step.StepName,
					"step_type":   step.StepType,
					"duration_ms": stepDuration.Milliseconds(),
					"success":     stepErr == nil,
				}
				if stepOutput != nil {
					for k, v := range stepOutput.ExecutionDetails {
						fields[k] = v
					}
				}
				if stepErr != nil {
					fields["error"] = stepErr.Error()
				}
				entry := storage.LogEntry{
					Level:   level,
					Stage:   stage,
					Message: msg,
					Fields:  fields,
				}
				go tps.objectStorage.AppendLog(context.Background(), ifaceIDForLog, msgIDForLog, entry)
			}
		}

		// PHASE 3: DEBUG - Check what's in metadata
		log.Printf("🔍 [DEBUG] After step %d, execCtx.Metadata keys: %v", i+1, getMapKeys(execCtx.Metadata))
		if routing, ok := execCtx.Metadata["_routing"]; ok {
			log.Printf("🔍 [DEBUG] _routing exists, type: %T, value: %+v", routing, routing)
		}

		// PHASE 2: Check for conditional routing (from context metadata)
		if routingDirective, ok := execCtx.Metadata["_routing"].(map[string]interface{}); ok {
			logger.Debug("routing directive found",
				"interface_id", pipeline.InterfaceID,
				"correlation_id", result.CorrelationID,
				"step_name", step.StepName)

			// Check for skipSteps (exclusive branch support) - these steps will be skipped
			if skipSteps, ok := routingDirective["skipSteps"].([]interface{}); ok {
				for _, sid := range skipSteps {
					if stepID, ok := sid.(string); ok {
						skipStepIDs[stepID] = true
						logger.Debug("marking step to skip (exclusive branch)",
							"interface_id", pipeline.InterfaceID,
							"correlation_id", result.CorrelationID,
							"step_id", stepID)
					}
				}
				// Clear skipSteps after processing
				delete(routingDirective, "skipSteps")
			}
			// Also handle []string type (from some code paths)
			if skipSteps, ok := routingDirective["skipSteps"].([]string); ok {
				for _, stepID := range skipSteps {
					skipStepIDs[stepID] = true
					logger.Debug("marking step to skip (exclusive branch)",
						"interface_id", pipeline.InterfaceID,
						"correlation_id", result.CorrelationID,
						"step_id", stepID)
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

	// Persist execution + per-step records asynchronously (non-blocking).
	// interfaceID and messageID come from the pipeline's context fields.
	ifaceID, _ := ctx.Value("interface_id").(string)
	msgID, _ := ctx.Value("message_id").(string)
	if ifaceID == "" {
		ifaceID = pipeline.InterfaceID
	}
	if msgID != "" && pipeline.ID != "" {
		go tps.persistExecutionRecords(pipeline.ID, msgID, ifaceID, startTime, result)
	}

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

	// P7: enriched.* carry-over removed — prior step outputs are accessed via
	// steps.{namespace}.step_output.{field} (injected below from execCtx.StepOutputs).

	// Inject completed step outputs as steps.{namespace}.step_output.{field}
	// Registers BOTH the full runtime namespace (api_enrichment_b636de) AND the
	// display key used by the test controller (api_enrichment, api_enrichment_2, …)
	// so masking / routing rules that reference display keys work at runtime.
	if len(execCtx.StepOutputs) > 0 {
		// Sort by Sequence for deterministic display-counter assignment
		type soEntry struct {
			ns string
			so models.StepOutput
		}
		entries := make([]soEntry, 0, len(execCtx.StepOutputs))
		for ns, so := range execCtx.StepOutputs {
			if so.Success && so.OutputData != nil {
				entries = append(entries, soEntry{ns, so})
			}
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].so.Sequence < entries[j].so.Sequence
		})

		displayNorm := models.NewOutputNormalizer()
		displayCounts := make(map[string]int)
		stepsSnapshot := make(map[string]interface{}, len(entries)*2)
		for _, e := range entries {
			// Full runtime namespace key (e.g. api_enrichment_b636de)
			stepsSnapshot[e.ns] = map[string]interface{}{"step_output": e.so.OutputData}

			// Display key matching the test controller (api_enrichment → api_enrichment_2 …)
			displayBase := displayNorm.NormalizeKey(e.so.StepName)
			displayCounts[displayBase]++
			displayKey := displayBase
			if displayCounts[displayBase] > 1 {
				displayKey = fmt.Sprintf("%s_%d", displayBase, displayCounts[displayBase])
			}
			if displayKey != e.ns {
				stepsSnapshot[displayKey] = map[string]interface{}{"step_output": e.so.OutputData}
			}
		}
		if len(stepsSnapshot) > 0 {
			inputData["steps"] = stepsSnapshot
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
	// JSON round-trip first: converts all Go-native typed slices (e.g. []map[string]interface{}
	// from FHIR bundle createBundle) into []interface{} so the normalizer can traverse them.
	// Then normalize keys to snake_case so runtime paths match the path picker display.
	if stepOutputData, ok := output["_stepOutput"].(map[string]interface{}); ok {
		if b, err := json.Marshal(stepOutputData); err == nil {
			var generic map[string]interface{}
			if json.Unmarshal(b, &generic) == nil {
				stepOutputData = generic
			}
		}
		normalizer := models.NewOutputNormalizer()
		stepOutput.OutputData = normalizer.NormalizeStepOutput(stepOutputData)
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
			// P7: enriched.* is no longer a primary output namespace (see steps.{ns}.step_output)
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
		ReceivedAt:  &receivedAt,
		// INCLUDE payload for test mode - this is the parsed input message
		// In production, this would be a reference to MongoDB
		Payload:  inputData,
		Metadata: extractOutputMetadata(inputData, format),
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
	} else if msg, ok := outputData["message"].(map[string]interface{}); ok {
		if rt, ok := msg["resourceType"].(string); ok && rt != "" {
			// FHIR parser stores resource fields at root of execCtx.Message, which
			// executeStepWithContext wraps as outputData["message"].
			// Build a clean "parsed resource" payload: FHIR fields the user cares about
			// plus schema annotations. Strip internal pipeline execution state.
			clean := make(map[string]interface{}, len(msg))
			for k, v := range msg {
				// Drop internal pipeline state fields (all start with _)
				if len(k) > 0 && k[0] == '_' {
					continue
				}
				// Drop parser bookkeeping — redundant or not user-facing
				switch k {
				case "parsedAt", "raw", "fieldOrder", "typeName", "typeDescription":
					continue
				}
				// Keep everything else: FHIR resource fields + enhancedFields + schemaLoaded
				clean[k] = v
			}
			actualPayload = clean
			format = "fhir-r4"
			messageType = rt
			log.Printf("🎯 [Output Context] Found FHIR resource at message.resourceType=%s", rt)
		}
	}
	if actualPayload == nil {
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

		// If core fields weren't at top level, check inside the "message" wrapper.
		// executeStepWithContext wraps the parsed message as outputData["message"] so
		// after any step (e.g. data masking) the HL7 data lives one level down.
		if len(cleanPayload) == 0 {
			if msg, ok := outputData["message"].(map[string]interface{}); ok {
				for _, field := range coreFields {
					if val, exists := msg[field]; exists {
						cleanPayload[field] = val
					}
				}
				if len(cleanPayload) > 0 {
					log.Printf("📦 [Output Context] Extracted core message fields from 'message' wrapper (%d fields)", len(cleanPayload))
				}
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
		TransformedAt: &transformedAt,
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

// extractOutputMetadata extracts format-specific metadata from output.
// For FHIR: data may be the raw step output map (with FHIR under "message") or
// the cleaned FHIR payload directly — check both.
func extractOutputMetadata(data map[string]interface{}, format string) map[string]interface{} {
	metadata := make(map[string]interface{})

	switch format {
	case "fhir-r4":
		// Resolve the FHIR source: prefer root if it has resourceType, else look under "message"
		fhirSrc := data
		if _, ok := fhirSrc["resourceType"]; !ok {
			if msg, ok := data["message"].(map[string]interface{}); ok {
				fhirSrc = msg
			}
		}

		if resourceType, ok := fhirSrc["resourceType"].(string); ok {
			metadata["resource_type"] = resourceType

			if bundleType, ok := fhirSrc["type"].(string); ok {
				metadata["bundle_type"] = bundleType
			}

			if entry, ok := fhirSrc["entry"].([]interface{}); ok {
				metadata["resource_count"] = len(entry)

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

// ===============================================================
// PHASE 2 — MESSAGE ENVELOPE + SEMANTIC VOCABULARY
// ===============================================================
//
// enrichMessageEnvelope builds _semantic_index and _sensitivity_map into the
// pipeline message map when _format is present.  Called once per pipeline run
// before the step loop so every executor can use ResolveVariable transparently.
//
// Zero breaking changes: pipelines without _format skip the enrichment silently.

// semanticVocab maps semantic path → format-native path.
// Hard-coded bootstrap vocabulary (Phase 5 will move this to the DB).
var semanticVocab = map[string]map[string]string{
	"hl7v2": {
		"patient.id":             "PID.3",
		"patient.name.family":    "PID.5.1",
		"patient.name.given":     "PID.5.2",
		"patient.dob":            "PID.7",
		"patient.gender":         "PID.8",
		"patient.ssn":            "PID.19",
		"patient.mrn":            "PID.3.1",
		"patient.phone":          "PID.13.1",
		"patient.account":        "PID.18",
		"patient.address.street": "PID.11.1",
		"patient.address.city":   "PID.11.3",
		"patient.address.state":  "PID.11.4",
		"patient.address.zip":    "PID.11.5",
		"message.type":           "MSH.9.1",
		"message.id":             "MSH.10",
		"message.datetime":       "MSH.7",
		"encounter.id":           "PV1.19",
		"encounter.class":        "PV1.2",
		"provider.attending":     "PV1.7",
		"obs.value":              "OBX.5",
		"obs.units":              "OBX.6",
		"obs.status":             "OBX.11",
	},
	"fhir": {
		"patient.id":          "Patient.id",
		"patient.name.family": "Patient.name[0].family",
		"patient.name.given":  "Patient.name[0].given",
		"patient.dob":         "Patient.birthDate",
		"patient.gender":      "Patient.gender",
		"patient.ssn":         "Patient.identifier[type=SS].value",
		"patient.mrn":         "Patient.identifier[type=MR].value",
		"patient.phone":       "Patient.telecom[system=phone].value",
		"encounter.id":        "Encounter.id",
		"encounter.class":     "Encounter.class.code",
		"obs.value":           "Observation.valueQuantity.value",
		"obs.units":           "Observation.valueQuantity.unit",
	},
	"csv": {
		"patient.id":          "PATIENT_ID",
		"patient.name.family": "LAST_NAME",
		"patient.name.given":  "FIRST_NAME",
		"patient.dob":         "DOB",
		"patient.gender":      "GENDER",
		"patient.ssn":         "SSN",
		"patient.mrn":         "MRN",
		"patient.phone":       "PHONE",
		"patient.account":     "ACCOUNT_NUM",
		"obs.value":           "VALUE",
		"obs.units":           "UNIT",
	},
}

// phiSemanticPaths lists semantic paths that are PHI under HIPAA Safe Harbor.
var phiSemanticPaths = []string{
	"patient.name.family", "patient.name.given",
	"patient.dob", "patient.ssn", "patient.mrn",
	"patient.phone", "patient.account",
	"patient.address.street", "patient.address.zip",
}

// enrichMessageEnvelope adds _semantic_index and _sensitivity_map to the message.
// Uses detectMessageFormat (defined above) for format detection.
// Idempotent — re-running is safe.
func enrichMessageEnvelope(message map[string]interface{}) {
	// Respect _format if already wired by engine (parsed_format → _format)
	format, _ := message["_format"].(string)
	if format == "" {
		// Fall back to structural detection
		format = detectMessageFormat(message)
		if format != "" {
			message["_format"] = format
		}
	}
	// Normalise fhir-r4 → fhir for semantic vocab lookup
	if format == "fhir-r4" || format == "fhir-r4-bundle" {
		format = "fhir"
	}
	if format == "" {
		return // Unknown format — skip enrichment
	}

	// Build _semantic_index only if not already present
	if _, exists := message["_semantic_index"]; !exists {
		message["_semantic_index"] = buildSemanticIndex(format)
	}

	// Build _sensitivity_map only if not already present
	if _, exists := message["_sensitivity_map"]; !exists {
		message["_sensitivity_map"] = buildSensitivityMap(format)
	}
}

// buildSemanticIndex returns a map of semantic_path → native_path for the given format.
func buildSemanticIndex(format string) map[string]interface{} {
	vocab, ok := semanticVocab[format]
	if !ok {
		return map[string]interface{}{}
	}
	idx := make(map[string]interface{}, len(vocab))
	for semantic, native := range vocab {
		idx[semantic] = native
	}
	return idx
}

// buildSensitivityMap returns a map of semantic_path → "phi" | "public" for the format.
func buildSensitivityMap(format string) map[string]interface{} {
	vocab, ok := semanticVocab[format]
	if !ok {
		return map[string]interface{}{}
	}
	phiSet := make(map[string]bool, len(phiSemanticPaths))
	for _, p := range phiSemanticPaths {
		phiSet[p] = true
	}
	sm := make(map[string]interface{}, len(vocab))
	for semantic := range vocab {
		if phiSet[semantic] {
			sm[semantic] = "phi"
		} else {
			sm[semantic] = "public"
		}
	}
	return sm
}

// buildInitialVariableContext registers semantic message paths into the
// PipelineVariableContext so the no-code UI can autocomplete them.
func buildInitialVariableContext(message map[string]interface{}, vc *models.PipelineVariableContext) {
	idx, ok := message["_semantic_index"].(map[string]interface{})
	if !ok {
		return
	}
	sm, _ := message["_sensitivity_map"].(map[string]interface{})
	for semanticPath, nativePath := range idx {
		sensitivity := ""
		if sm != nil {
			sensitivity, _ = sm[semanticPath].(string)
		}
		desc := fmt.Sprintf("native: %v | sensitivity: %s", nativePath, sensitivity)
		// RegisterVariable(stepName, stepType, varName, value, varType)
		vc.RegisterVariable("message", "input", semanticPath, desc, models.VarTypeString)
	}
}

// ===============================================================
// PHASE 4 — PARALLEL EXECUTION BY SEQUENCE NUMBER
// ===============================================================
//
// Steps with the same Sequence number run in parallel goroutines.
// Simple rule: same Sequence = same "wave".
// Control/routing steps (control.loop, conditional, switch/case) are always
// run sequentially to preserve routing state integrity.
//
// The main ExecutePipeline loop handles sequential steps.
// executeParallelGroup is called by the pipeline service when it detects
// that multiple enabled non-routing steps share a sequence number.

// parallelSafeStepTypes lists step types that are safe to run in parallel.
// Control flow steps must remain sequential.
var parallelSafeTypes = map[string]bool{
	"enrichment.api":      true,
	"enrichment.database": true,
	"enrichment.script":   true,
	"field_mapping":       true,
	"data_masking":        true,
	"remove_duplicates":   true,
	"normalizer":          true,
	"deidentify":          true,
	"connector.outbound":  true,
}

// groupStepsBySequence returns steps grouped by their Sequence number, preserving order.
func groupStepsBySequence(steps []models.TransformationStep) [][]models.TransformationStep {
	if len(steps) == 0 {
		return nil
	}

	type group struct {
		seq   int
		steps []models.TransformationStep
	}

	var groups []group
	seqIdx := make(map[int]int) // sequence → index in groups

	for _, s := range steps {
		idx, exists := seqIdx[s.Sequence]
		if !exists {
			groups = append(groups, group{seq: s.Sequence, steps: []models.TransformationStep{s}})
			seqIdx[s.Sequence] = len(groups) - 1
		} else {
			groups[idx].steps = append(groups[idx].steps, s)
		}
	}

	result := make([][]models.TransformationStep, len(groups))
	for i, g := range groups {
		result[i] = g.steps
	}
	return result
}

// executeParallelGroup runs a group of steps concurrently when all are parallel-safe.
// Outputs are merged into execCtx.StepOutputs under each step's own namespace.
// If any step in the group is NOT parallel-safe, the whole group runs sequentially.
func (tps *TransformationPipelineService) executeParallelGroup(
	ctx context.Context,
	group []models.TransformationStep,
	execCtx *models.PipelineExecutionContext,
	seqOffset int,
) {
	if len(group) <= 1 {
		return // Single step — handled by main loop
	}

	// Check all steps are parallel-safe
	for _, s := range group {
		if !parallelSafeTypes[s.StepType] {
			log.Printf("⏩ Parallel group seq=%d contains non-parallel step '%s' — running sequentially", s.Sequence, s.StepType)
			return // Caller should fall back to sequential
		}
	}

	log.Printf("⚡ Running %d steps in parallel (sequence=%d)", len(group), group[0].Sequence)

	type result struct {
		output    map[string]interface{}
		stepOut   *models.StepOutput
		namespace string
		err       error
	}

	results := make([]result, len(group))
	var wg sync.WaitGroup
	for idx, step := range group {
		wg.Add(1)
		go func(i int, s models.TransformationStep) {
			defer wg.Done()
			ns := tps.generateStepNamespace(&s, seqOffset+i)
			out, stepOut, err := tps.executeStepWithContext(ctx, &s, execCtx, seqOffset+i)
			results[i] = result{output: out, stepOut: stepOut, namespace: ns, err: err}
		}(idx, step)
	}
	wg.Wait()

	// Merge results — order by group index for determinism
	for _, r := range results {
		if r.err != nil {
			log.Printf("⚠️  Parallel step '%s' error: %v", r.namespace, r.err)
			continue
		}
		if r.stepOut != nil && r.namespace != "" {
			execCtx.StepOutputs[r.namespace] = *r.stepOut
		}
		// Merge output keys into execCtx.Message (use mutex-free approach — parallel steps
		// should write to DIFFERENT output fields; overlaps are last-write-wins)
		if r.output != nil {
			for k, v := range r.output {
				execCtx.Message[k] = v
			}
		}
	}
}

// ===============================================================
// PHASE 3 — PHI EGRESS GATE
// ===============================================================

// checkEgressPHI returns the list of PHI semantic paths that are present in the
// message but have NOT been masked (i.e., not listed in _masked_fields).
// Returns nil/empty when _sensitivity_map is absent (backward compatible).
func checkEgressPHI(message map[string]interface{}) []string {
	sm, ok := message["_sensitivity_map"].(map[string]interface{})
	if !ok {
		return nil // No sensitivity map — skip check
	}

	// Build set of already-masked fields
	maskedSet := make(map[string]bool)
	if mf, ok := message["_masked_fields"].([]interface{}); ok {
		for _, f := range mf {
			maskedSet[fmt.Sprintf("%v", f)] = true
		}
	}

	var violations []string
	for path, sensitivity := range sm {
		if fmt.Sprintf("%v", sensitivity) == "phi" && !maskedSet[path] {
			violations = append(violations, path)
		}
	}
	return violations
}

// pipelineStepLogEntry returns the (stage, message, level) triple for a pipeline step log entry.
// Used to write a structured NDJSON log entry for every step so the Logs tab shows
// the full step trace and highlights exactly where a message stopped.
func pipelineStepLogEntry(stepType, stepName string, stepErr error) (stage, msg, level string) {
	switch {
	case strings.HasPrefix(stepType, "connector.outbound"):
		stage = "delivery"
		msg = fmt.Sprintf("Connector step: %s", stepName)
	case stepType == "fhir_validation" || stepType == "post.validation":
		stage = "validation"
		msg = fmt.Sprintf("FHIR validation: %s", stepName)
	case stepType == "hl7_fhir_transform" || stepType == "hl7_assemble_observations":
		stage = "transformation"
		msg = fmt.Sprintf("HL7→FHIR transform: %s", stepName)
	case stepType == "field_mapping" || strings.HasPrefix(stepType, "mapping"):
		stage = "transformation"
		msg = fmt.Sprintf("Field mapping: %s", stepName)
	case stepType == "enrichment" || strings.HasPrefix(stepType, "enrich"):
		stage = "enrichment"
		msg = fmt.Sprintf("Enrichment: %s", stepName)
	case strings.HasPrefix(stepType, "pre.logic") || strings.HasPrefix(stepType, "control"):
		stage = "step"
		msg = fmt.Sprintf("Logic step: %s", stepName)
	default:
		stage = "step"
		msg = fmt.Sprintf("Pipeline step: %s", stepName)
	}

	if stepErr != nil {
		level = "error"
		msg = fmt.Sprintf("%s failed: %v", stepName, stepErr)
	} else {
		level = "info"
	}
	return
}

