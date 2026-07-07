// services/ai/agent_tools_script.go
// verify_pipeline_script — generates a pipeline script, dry-run tests it
// against a real sample message via the existing test-pipeline execution
// path, and retries with the error fed back to the model on failure. The
// human still approves the final (verified or best-effort) script before
// it's written to the step — see Tool.Prepare / Tool.Execute split.
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"ezhealthkonnect/models"
	"ezhealthkonnect/services"
)

// maxScriptVerifyAttempts caps the generate-test-fix loop so a stubbornly
// broken request can't burn unbounded LLM calls.
const maxScriptVerifyAttempts = 3

// ScriptVerifier is the subset of *services.TransformationPipelineService
// verify_pipeline_script needs: a dry-run executor and the persistence step
// applied only after a human approves.
type ScriptVerifier interface {
	ExecutePipeline(ctx context.Context, pipeline *models.TransformationPipeline, inputData map[string]interface{}) (*models.TransformationExecutionResult, error)
	UpdateStepScript(ctx context.Context, stepID, script string) error
}

// registerScriptVerificationTool adds verify_pipeline_script onto an existing
// registry. Additive, like registerAnalyticsTools — safe regardless of call order.
func registerScriptVerificationTool(reg *ToolRegistry, llm LLMProvider, scriptGen *ScriptGeneratorService, verifier ScriptVerifier) {
	if llm == nil || scriptGen == nil || verifier == nil {
		return
	}

	reg.Register(&Tool{
		Name: "verify_pipeline_script",
		Description: "Generate a JavaScript transform(input) script for a pipeline step, test it against a sample message, and automatically fix errors before proposing it. Prefer this over propose_pipeline_script whenever a sample message is available — the script is verified to actually run before a human reviews it.",
		Parameters: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"step_id":        map[string]interface{}{"type": "string", "description": "The pipeline step ID this script is for."},
				"pipeline_id":    map[string]interface{}{"type": "string", "description": "Optional — used to show preceding-step context in the prompt."},
				"interface_id":   map[string]interface{}{"type": "string", "description": "Optional — the interface ID."},
				"message_type":   map[string]interface{}{"type": "string", "description": "Optional — e.g. ADT^A01."},
				"description":    map[string]interface{}{"type": "string", "description": "Plain-English description of what the script should do."},
				"sample_message": map[string]interface{}{"type": "string", "description": "A sample raw HL7/FHIR message to test the generated script against."},
			},
			"required": []string{"step_id", "description", "sample_message"},
		},
		EntityType:       "pipeline_step",
		RequiresApproval: true,
		Prepare: func(ctx context.Context, rawArgs json.RawMessage) (json.RawMessage, string, error) {
			var in struct {
				StepID        string `json:"step_id"`
				PipelineID    string `json:"pipeline_id"`
				InterfaceID   string `json:"interface_id"`
				MessageType   string `json:"message_type"`
				Description   string `json:"description"`
				SampleMessage string `json:"sample_message"`
			}
			if err := json.Unmarshal(rawArgs, &in); err != nil {
				return nil, "", fmt.Errorf("invalid arguments: %w", err)
			}
			if in.StepID == "" || in.Description == "" || in.SampleMessage == "" {
				return nil, "", fmt.Errorf("step_id, description, and sample_message are required")
			}

			script, verified, attempts, lastErr := verifyScriptLoop(
				ctx, llm, scriptGen, verifier,
				ScriptGenInput{
					Description: in.Description,
					StepID:      in.StepID,
					PipelineID:  in.PipelineID,
					InterfaceID: in.InterfaceID,
					MessageType: in.MessageType,
				},
				in.SampleMessage,
			)

			out := map[string]interface{}{
				"step_id":  in.StepID,
				"script":   script,
				"verified": verified,
				"attempts": attempts,
			}
			if lastErr != "" {
				out["last_error"] = lastErr
			}
			finalArgs, err := json.Marshal(out)
			return finalArgs, "", err
		},
		Execute: func(ctx context.Context, args json.RawMessage) (map[string]interface{}, error) {
			var in struct {
				StepID   string `json:"step_id"`
				Script   string `json:"script"`
				Verified bool   `json:"verified"`
				Attempts int    `json:"attempts"`
			}
			if err := json.Unmarshal(args, &in); err != nil {
				return nil, fmt.Errorf("invalid arguments: %w", err)
			}
			if err := verifier.UpdateStepScript(ctx, in.StepID, in.Script); err != nil {
				return nil, err
			}
			return map[string]interface{}{
				"step_id":  in.StepID,
				"verified": in.Verified,
				"attempts": in.Attempts,
				"status":   "applied",
			}, nil
		},
	})
}

// verifyScriptLoop generates a script, dry-run tests it, and — on failure —
// feeds the error back to the model and retries, up to maxScriptVerifyAttempts.
func verifyScriptLoop(
	ctx context.Context,
	llm LLMProvider,
	scriptGen *ScriptGeneratorService,
	verifier ScriptVerifier,
	input ScriptGenInput,
	sampleMessage string,
) (script string, verified bool, attempts int, lastErr string) {
	for attempts = 1; attempts <= maxScriptVerifyAttempts; attempts++ {
		prompt := scriptGen.BuildScriptPrompt(ctx, input)
		if lastErr != "" {
			prompt += fmt.Sprintf(
				"\n\n### The Existing Script Above Failed\nWhen tested against the sample message, it failed with:\n%s\n\nFix it and return the complete corrected script.",
				lastErr,
			)
		}

		raw, err := llm.Generate(ctx, prompt)
		if err != nil {
			lastErr = err.Error()
			continue
		}
		script = stripCodeFences(raw)

		if execErr := testScriptAgainstSample(ctx, verifier, input.StepID, script, sampleMessage); execErr == "" {
			return script, true, attempts, ""
		} else {
			lastErr = execErr
			input.CurrentScript = script // shown back to the model as "Existing Script" next attempt
		}
	}
	return script, false, maxScriptVerifyAttempts, lastErr
}

// testScriptAgainstSample runs a candidate script through the real pipeline
// executor in test mode, against a synthetic single-step pipeline — no
// persistence, no saved pipeline/step required. Returns "" on success, or
// a description of what went wrong otherwise (either a real JS error, or —
// just as importantly — a script that ran clean but produced no output at
// all, which usually means it never actually wrote to `output` or returned
// a value, e.g. a leftover `function transform(input){...}` declaration
// that's never called).
func testScriptAgainstSample(ctx context.Context, verifier ScriptVerifier, stepID, script, sampleMessage string) string {
	parsedInput := services.ParseSampleMessage(sampleMessage)

	synthetic := &models.TransformationPipeline{
		ID: "agent-verify",
		Steps: []models.TransformationStep{
			{
				ID:       stepID,
				StepName: "verify",
				StepType: "enrichment.script",
				Sequence: 10,
				Enabled:  true,
				Config:   map[string]interface{}{"script": script},
			},
		},
	}

	result, err := verifier.ExecutePipeline(models.WithTestMode(ctx), synthetic, parsedInput)
	if err != nil {
		return err.Error()
	}
	for _, entry := range result.ExecutionLog {
		if !entry.Success && entry.Error != "" {
			return entry.Error
		}
		if entry.Success && (entry.StepOutput == nil || len(entry.StepOutput.OutputData) == 0) {
			return "the script ran without a JavaScript error but produced no output — " +
				"it must either mutate the pre-injected `output` object (e.g. output.field = ...) " +
				"or use a top-level `return {...}`; a `function transform(input){...}` declaration " +
				"that is never called will silently do nothing"
		}
	}
	return ""
}

// stripCodeFences removes a leading/trailing ```javascript ... ``` (or bare ```)
// fence, mirroring the same stripping ai-assistant.js already does client-side.
func stripCodeFences(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "```javascript")
	s = strings.TrimPrefix(s, "```js")
	s = strings.TrimPrefix(s, "```")
	s = strings.TrimSuffix(s, "```")
	return strings.TrimSpace(s)
}
