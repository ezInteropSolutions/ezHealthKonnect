package services

// DAG-based parallel pipeline executor.
//
// Design:
//   - BuildDAG turns steps + connections into a predecessor/successor graph.
//   - isLinear: every step has ≤1 predecessor and ≤1 successor, exactly 1 root.
//     Linear pipelines use the existing sequential loop — zero overhead, no change.
//   - Parallel pipelines: goroutine-per-ready-step, semaphore caps concurrency.
//   - execContext.Message is READ-ONLY during parallel execution.
//     Each step receives shallowCopyMap(execContext.Message) as input.
//     After a step completes its output is merged back under a scheduler-held mutex.

import (
	"context"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	"ezhealthkonnect/services/executors/control"
	"ezhealthkonnect/services/logger"
	"fmt"
	"runtime"
	"sync"
	"time"
)

// ─────────────────────────────────────────────
//  Data structures
// ─────────────────────────────────────────────

// pipelineDAG is the internal representation of step dependencies.
type pipelineDAG struct {
	predecessors map[string][]string
	successors   map[string][]string
	roots        []string // stepIDs with zero predecessors
	isLinear     bool
	stepByID     map[string]*models.TransformationStep
}

// dagStepResult carries everything produced by one step execution.
type dagStepResult struct {
	stepID      string
	namespace   string
	outputData  map[string]interface{}
	stepOutput  models.StepOutput
	stepLog     models.StepExecutionLog
	err         error
	nextStepID  string   // _routing.nextStep
	skipStepIDs []string // _routing.skipSteps + branch resolver
}

// ─────────────────────────────────────────────
//  Build
// ─────────────────────────────────────────────

// BuildDAG constructs a pipelineDAG from steps and connections.
// When no connections are stored (legacy pipelines) it falls back to linear.
func BuildDAG(steps []models.TransformationStep, connections []models.PipelineConnection) *pipelineDAG {
	d := &pipelineDAG{
		predecessors: make(map[string][]string),
		successors:   make(map[string][]string),
		stepByID:     make(map[string]*models.TransformationStep),
	}

	for i := range steps {
		s := &steps[i]
		d.stepByID[s.ID] = s
		d.predecessors[s.ID] = []string{}
		d.successors[s.ID] = []string{}
	}

	for _, c := range connections {
		if _, ok := d.stepByID[c.From]; !ok {
			continue
		}
		if _, ok := d.stepByID[c.To]; !ok {
			continue
		}
		d.predecessors[c.To] = append(d.predecessors[c.To], c.From)
		d.successors[c.From] = append(d.successors[c.From], c.To)
	}

	for _, s := range steps {
		if len(d.predecessors[s.ID]) == 0 {
			d.roots = append(d.roots, s.ID)
		}
	}

	if len(connections) == 0 {
		d.isLinear = true
		return d
	}

	d.isLinear = d.detectLinear()
	return d
}

// detectLinear returns true when every step has ≤1 predecessor and ≤1 successor
// and there is exactly one root. Covers the majority of real pipelines.
func (d *pipelineDAG) detectLinear() bool {
	if len(d.roots) != 1 {
		return false
	}
	for id := range d.stepByID {
		if len(d.predecessors[id]) > 1 || len(d.successors[id]) > 1 {
			return false
		}
	}
	return true
}

// ─────────────────────────────────────────────
//  Cycle detection (called at pipeline save time)
// ─────────────────────────────────────────────

// DetectCycle uses Kahn's algorithm. Returns non-nil if a cycle exists.
func DetectCycle(steps []models.TransformationStep, connections []models.PipelineConnection) error {
	d := BuildDAG(steps, connections)

	inDegree := make(map[string]int, len(steps))
	for _, s := range steps {
		inDegree[s.ID] = len(d.predecessors[s.ID])
	}

	queue := make([]string, 0, len(d.roots))
	queue = append(queue, d.roots...)

	visited := 0
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		visited++
		for _, succ := range d.successors[cur] {
			inDegree[succ]--
			if inDegree[succ] == 0 {
				queue = append(queue, succ)
			}
		}
	}

	if visited != len(steps) {
		return fmt.Errorf("pipeline has a circular dependency — remove one of the connections forming the cycle")
	}
	return nil
}

// ─────────────────────────────────────────────
//  Parallel execution
// ─────────────────────────────────────────────

func workerCap() int {
	n := runtime.NumCPU() * 2
	if n < 4 {
		n = 4
	}
	return n
}

// executeDAG runs a parallel pipeline. Called only when dag.isLinear == false.
func (tps *TransformationPipelineService) executeDAG(
	ctx context.Context,
	d *pipelineDAG,
	pipeline *models.TransformationPipeline,
	execContext *models.PipelineExecutionContext,
	result *models.TransformationResult,
	loopChildStepIDs map[string]bool,
	skipStepIDsInit map[string]bool,
	stepByID map[string]*models.TransformationStep,
	childExecutor control.ChildStepExecutor,
) error {
	totalSteps := len(pipeline.Steps)
	if totalSteps == 0 {
		return nil
	}

	pending := make(map[string]int, totalSteps)
	for id, preds := range d.predecessors {
		pending[id] = len(preds)
	}

	completed := make(map[string]bool, totalSteps)
	skipped := make(map[string]bool, totalSteps)

	// Copy initial skips (e.g. branch exclusions already known before DAG runs)
	for id := range skipStepIDsInit {
		skipped[id] = true
	}

	readyQueue := make(chan string, totalSteps)
	doneCh := make(chan dagStepResult, totalSteps)
	workerSem := make(chan struct{}, workerCap())

	// Pre-complete connector.inbound roots and enqueue other roots
	for _, rootID := range d.roots {
		step := d.stepByID[rootID]
		if step == nil {
			completed[rootID] = true
			tps.dagUnblockSuccessors(rootID, d, pending, completed, skipped, readyQueue)
			continue
		}
		if step.StepType == "connector.inbound" || loopChildStepIDs[rootID] {
			completed[rootID] = true
			tps.dagUnblockSuccessors(rootID, d, pending, completed, skipped, readyQueue)
			continue
		}
		if skipped[rootID] {
			tps.dagPropagateSkip(rootID, d, pending, completed, skipped, readyQueue)
			continue
		}
		readyQueue <- rootID
	}

	doneCount := len(completed) + len(skipped)
	mu := sync.Mutex{}

	for doneCount < totalSteps {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case stepID := <-readyQueue:
			step := d.stepByID[stepID]
			if step == nil {
				completed[stepID] = true
				doneCount++
				tps.dagUnblockSuccessors(stepID, d, pending, completed, skipped, readyQueue)
				continue
			}

			mu.Lock()
			msgSnapshot := shallowCopyMap(execContext.Message)
			mu.Unlock()

			workerSem <- struct{}{}
			go func(s *models.TransformationStep, snap map[string]interface{}) {
				defer func() { <-workerSem }()
				res := tps.dagExecuteStep(ctx, s, snap, execContext, pipeline,
					loopChildStepIDs, stepByID, childExecutor)
				doneCh <- res
			}(step, msgSnapshot)

		case r := <-doneCh:
			doneCount++

			result.TransformationLog = append(result.TransformationLog, r.stepLog)
			result.TotalTimeMs += r.stepLog.DurationMs

			if r.err != nil {
				s := d.stepByID[r.stepID]
				if s != nil && s.Required && s.OnErrorStrategy == "fail" {
					result.Success = false
					result.Error = fmt.Sprintf("Required step failed: %s - %v", s.StepName, r.err)
					return fmt.Errorf("pipeline failed at step %s: %w", s.StepName, r.err)
				}
				logger.Warn("DAG step failed (non-fatal)", "step", r.stepLog.StepName, "error", r.err)
			}

			if r.namespace != "" {
				mu.Lock()
				execContext.StepOutputs[r.namespace] = r.stepOutput
				if r.outputData != nil && r.err == nil {
					for k, v := range r.outputData {
						execContext.Message[k] = v
					}
					result.OutputData = execContext.Message
				}
				mu.Unlock()
			}

			// Apply routing directives
			if r.nextStepID != "" {
				tps.dagSkipUntil(r.stepID, r.nextStepID, d, pending, completed, skipped, readyQueue)
			}
			for _, sid := range r.skipStepIDs {
				if !completed[sid] && !skipped[sid] {
					skipped[sid] = true
					tps.dagPropagateSkip(sid, d, pending, completed, skipped, readyQueue)
				}
			}

			completed[r.stepID] = true
			tps.dagUnblockSuccessors(r.stepID, d, pending, completed, skipped, readyQueue)
		}
	}

	result.OutputData = execContext.Message
	logger.Info("DAG pipeline completed", "total_ms", result.TotalTimeMs,
		"steps", totalSteps, "outputs", len(execContext.StepOutputs))
	return nil
}

// dagExecuteStep executes a single step and returns a dagStepResult.
// Mirrors the retry/error-handling logic in the sequential executor.
func (tps *TransformationPipelineService) dagExecuteStep(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
	execContext *models.PipelineExecutionContext,
	pipeline *models.TransformationPipeline,
	loopChildStepIDs map[string]bool,
	stepByID map[string]*models.TransformationStep,
	childExecutor control.ChildStepExecutor,
) dagStepResult {
	stepStartTime := time.Now()
	r := dagStepResult{stepID: step.ID}

	if loopChildStepIDs[step.ID] {
		r.stepLog = models.StepExecutionLog{
			StepID: step.ID, StepName: step.StepName, StepType: step.StepType,
			StartedAt: stepStartTime, CompletedAt: time.Now(), Success: true,
		}
		return r
	}

	exec := tps.executorRegistry.GetExecutor(step.StepType)
	if exec == nil {
		r.err = fmt.Errorf("no executor for step type: %s", step.StepType)
		r.stepLog = dagBuildFailLog(step, stepStartTime, r.err)
		return r
	}

	if canonical := exec.GetStepType(); canonical != "" && canonical != "generic" && canonical != step.StepType {
		step.StepType = canonical
	}

	// Wire child executor into loop steps
	if step.StepType == "control.loop" {
		if le, ok := exec.(*control.LoopExecutor); ok {
			le.SetChildExecutor(childExecutor)
		}
		if childIDs, ok := step.Config["childStepIds"].([]interface{}); ok {
			for _, cid := range childIDs {
				if cidStr, ok := cid.(string); ok {
					loopChildStepIDs[cidStr] = true
				}
			}
		}
	}

	pipelineRetryDefaults := executors.ParsePipelineRetryDefaults(pipeline.PipelineConfig)
	pipelineEHDefaults := executors.ParsePipelineErrorHandlingDefaults(pipeline.PipelineConfig)
	retryConfig := executors.ResolveRetryConfig(step.Config, pipelineRetryDefaults)
	ehConfig := executors.ResolveErrorHandlingConfig(step.Config, pipelineEHDefaults)

	retryResult := executors.ExecuteWithRetry(ctx, retryConfig, func(_ int) (map[string]interface{}, error) {
		stepCtx, cancel := context.WithTimeout(ctx, time.Duration(step.TimeoutMs)*time.Millisecond)
		defer cancel()
		return exec.Execute(stepCtx, step, shallowCopyMap(inputData))
	})

	outputData := retryResult.Output
	stepErr := retryResult.Err
	originalErr := stepErr
	stepDuration := time.Since(stepStartTime)

	errorWasCaught := false
	if stepErr != nil && ehConfig != nil {
		outputData, stepErr = executors.ApplyErrorHandling(ehConfig, stepErr, outputData, inputData, step.StepName)
		if stepErr == nil {
			errorWasCaught = true
		}
	}

	namespace := models.GenerateStepNamespace(step.StepName, step.ID, step.StepAlias)
	alias := models.GenerateDefaultAlias(step.StepName)
	if step.StepAlias != nil && *step.StepAlias != "" {
		alias = *step.StepAlias
	}

	stepOutput := models.StepOutput{
		StepID: step.ID, StepName: step.StepName, StepAlias: alias,
		StepType: step.StepType, Namespace: namespace, Sequence: step.Sequence,
		DurationMs: stepDuration.Milliseconds(),
	}
	// Merge executor-written output if present
	if existing, ok := execContext.StepOutputs[namespace]; ok {
		stepOutput = existing
		stepOutput.DurationMs = stepDuration.Milliseconds()
	}

	switch {
	case stepErr != nil:
		stepOutput.Success = false
		stepOutput.Error = stepErr.Error()
	case errorWasCaught:
		stepOutput.Success = true
		stepOutput.Error = fmt.Sprintf("caught: %s", originalErr.Error())
		if stepOutput.OutputData == nil {
			stepOutput.OutputData = make(map[string]interface{})
		}
		stepOutput.OutputData["error_caught"] = originalErr.Error()
	default:
		stepOutput.Success = true
		if stepOutput.OutputData == nil {
			stepOutput.OutputData = map[string]interface{}{
				"step_type": step.StepType, "execution_time_ms": stepDuration.Milliseconds(),
			}
		}
	}

	stepLog := models.StepExecutionLog{
		StepID: step.ID, StepName: step.StepName, StepAlias: alias,
		StepType: step.StepType, Namespace: namespace,
		StartedAt: stepStartTime, CompletedAt: time.Now(),
		DurationMs: stepDuration.Milliseconds(), Success: stepErr == nil,
	}
	if stepErr != nil {
		stepLog.Error = stepErr.Error()
	}
	so := stepOutput
	stepLog.StepOutput = &so

	r.namespace = namespace
	r.outputData = outputData
	r.stepOutput = stepOutput
	r.stepLog = stepLog
	r.err = stepErr

	if outputData != nil {
		if routing, ok := outputData["_routing"].(map[string]interface{}); ok {
			if next, ok := routing["nextStep"].(string); ok {
				r.nextStepID = next
			}
			if toSkip, ok := routing["skipSteps"].([]interface{}); ok {
				for _, v := range toSkip {
					if s, ok := v.(string); ok {
						r.skipStepIDs = append(r.skipStepIDs, s)
					}
				}
			}
			branchSkips := tps.branchResolver.GetStepsToSkip(step, routing, pipeline.Steps)
			r.skipStepIDs = append(r.skipStepIDs, branchSkips...)
		}
	}

	return r
}

// ─────────────────────────────────────────────
//  Scheduler helpers (single-goroutine scheduler — no mutex needed here)
// ─────────────────────────────────────────────

func (tps *TransformationPipelineService) dagUnblockSuccessors(
	completedID string, d *pipelineDAG,
	pending map[string]int, completed, skipped map[string]bool,
	readyQueue chan string,
) {
	for _, succID := range d.successors[completedID] {
		if completed[succID] || skipped[succID] {
			continue
		}
		pending[succID]--
		if pending[succID] <= 0 {
			readyQueue <- succID
		}
	}
}

func (tps *TransformationPipelineService) dagPropagateSkip(
	skipID string, d *pipelineDAG,
	pending map[string]int, completed, skipped map[string]bool,
	readyQueue chan string,
) {
	worklist := []string{skipID}
	for len(worklist) > 0 {
		cur := worklist[0]
		worklist = worklist[1:]
		for _, succID := range d.successors[cur] {
			if completed[succID] || skipped[succID] {
				continue
			}
			pending[succID]--
			if pending[succID] <= 0 {
				allSkipped := true
				for _, predID := range d.predecessors[succID] {
					if !skipped[predID] {
						allSkipped = false
						break
					}
				}
				if allSkipped {
					skipped[succID] = true
					worklist = append(worklist, succID)
				} else {
					readyQueue <- succID
				}
			}
		}
	}
}

func (tps *TransformationPipelineService) dagSkipUntil(
	currentID, targetID string, d *pipelineDAG,
	pending map[string]int, completed, skipped map[string]bool,
	readyQueue chan string,
) {
	visited := make(map[string]bool)
	worklist := []string{}
	for _, succID := range d.successors[currentID] {
		if succID != targetID {
			worklist = append(worklist, succID)
		}
	}
	for len(worklist) > 0 {
		cur := worklist[0]
		worklist = worklist[1:]
		if visited[cur] || cur == targetID || completed[cur] || skipped[cur] {
			continue
		}
		visited[cur] = true
		skipped[cur] = true
		for _, succID := range d.successors[cur] {
			if succID != targetID {
				worklist = append(worklist, succID)
			}
		}
	}
	if !completed[targetID] && !skipped[targetID] {
		allDone := true
		for _, predID := range d.predecessors[targetID] {
			if !completed[predID] && !skipped[predID] {
				allDone = false
				break
			}
		}
		if allDone {
			readyQueue <- targetID
		}
	}
}

func dagBuildFailLog(step *models.TransformationStep, startedAt time.Time, err error) models.StepExecutionLog {
	return models.StepExecutionLog{
		StepID: step.ID, StepName: step.StepName, StepType: step.StepType,
		StartedAt: startedAt, CompletedAt: time.Now(),
		DurationMs: time.Since(startedAt).Milliseconds(),
		Success: false, Error: err.Error(),
	}
}
