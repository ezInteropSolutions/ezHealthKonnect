// services/ai/agent_tools_script_test.go
// Tests for the self-debugging script tool: the generate-test-fix loop, the
// code-fence stripper, and the Prepare/Execute split (verify once at proposal
// time, persist the already-verified text at approval time).
package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"ezhealthkonnect/models"
)

// ─── Fakes ────────────────────────────────────────────────────────────────────

type fakeScriptLLM struct {
	responses []string
	prompts   []string
}

func (f *fakeScriptLLM) Generate(_ context.Context, prompt string) (string, error) {
	f.prompts = append(f.prompts, prompt)
	i := len(f.prompts) - 1
	if i >= len(f.responses) {
		i = len(f.responses) - 1
	}
	return f.responses[i], nil
}
func (f *fakeScriptLLM) GenerateStream(_ context.Context, _ string, _ func(string) error) error {
	return nil
}
func (f *fakeScriptLLM) Embed(_ context.Context, _ string) ([]float32, error) { return nil, nil }
func (f *fakeScriptLLM) Ping(_ context.Context) error                        { return nil }
func (f *fakeScriptLLM) ProviderName() string                               { return "fake" }
func (f *fakeScriptLLM) ChatModelName() string                              { return "fake-model" }

type fakeScriptVerifier struct {
	failFirstN  int
	execCalls   int
	updateCalls []struct{ stepID, script string }
}

func (f *fakeScriptVerifier) ExecutePipeline(_ context.Context, _ *models.TransformationPipeline, _ map[string]interface{}) (*models.TransformationExecutionResult, error) {
	f.execCalls++
	if f.execCalls <= f.failFirstN {
		return &models.TransformationExecutionResult{
			ExecutionLog: []models.StepExecutionLog{{Success: false, Error: "boom: reference error"}},
		}, nil
	}
	return &models.TransformationExecutionResult{
		ExecutionLog: []models.StepExecutionLog{{
			Success:    true,
			StepOutput: &models.StepOutput{OutputData: map[string]interface{}{"result": "ok"}},
		}},
	}, nil
}

func (f *fakeScriptVerifier) UpdateStepScript(_ context.Context, stepID, script string) error {
	f.updateCalls = append(f.updateCalls, struct{ stepID, script string }{stepID, script})
	return nil
}

// ─── stripCodeFences ──────────────────────────────────────────────────────────

func TestStripCodeFences(t *testing.T) {
	cases := map[string]string{
		"```javascript\nfunction transform(input){return input;}\n```": "function transform(input){return input;}",
		"```js\nfunction transform(input){return input;}\n```":         "function transform(input){return input;}",
		"```\nfunction transform(input){return input;}\n```":           "function transform(input){return input;}",
		"function transform(input){return input;}":                    "function transform(input){return input;}",
	}
	for in, want := range cases {
		if got := stripCodeFences(in); got != want {
			t.Errorf("stripCodeFences(%q) = %q, want %q", in, got, want)
		}
	}
}

// ─── verifyScriptLoop ─────────────────────────────────────────────────────────

func TestVerifyScriptLoop_SucceedsFirstTry(t *testing.T) {
	llm := &fakeScriptLLM{responses: []string{"function transform(input){return input;}"}}
	verifier := &fakeScriptVerifier{failFirstN: 0}
	scriptGen := newScriptGeneratorService(nil)

	script, verified, attempts, lastErr := verifyScriptLoop(context.Background(), llm, scriptGen, verifier,
		ScriptGenInput{Description: "do nothing", StepID: "step-1"}, "MSH|^~\\&|...")

	if !verified {
		t.Fatal("expected verified=true on first successful attempt")
	}
	if attempts != 1 {
		t.Fatalf("expected 1 attempt, got %d", attempts)
	}
	if lastErr != "" {
		t.Fatalf("expected no error, got %q", lastErr)
	}
	if script != "function transform(input){return input;}" {
		t.Fatalf("unexpected script: %q", script)
	}
}

func TestVerifyScriptLoop_RetriesThenSucceeds(t *testing.T) {
	llm := &fakeScriptLLM{responses: []string{
		"```javascript\nfunction transform(input){ return undeclared; }\n```",
		"function transform(input){return input;}",
	}}
	verifier := &fakeScriptVerifier{failFirstN: 1}
	scriptGen := newScriptGeneratorService(nil)

	script, verified, attempts, lastErr := verifyScriptLoop(context.Background(), llm, scriptGen, verifier,
		ScriptGenInput{Description: "do nothing", StepID: "step-1"}, "MSH|^~\\&|...")

	if !verified {
		t.Fatal("expected verified=true after the second attempt")
	}
	if attempts != 2 {
		t.Fatalf("expected 2 attempts, got %d", attempts)
	}
	if lastErr != "" {
		t.Fatalf("expected no error on the final result, got %q", lastErr)
	}
	if script != "function transform(input){return input;}" {
		t.Fatalf("unexpected final script: %q", script)
	}
	if len(llm.prompts) != 2 {
		t.Fatalf("expected 2 Generate calls, got %d", len(llm.prompts))
	}
	if !strings.Contains(llm.prompts[1], "boom: reference error") {
		t.Fatal("expected the retry prompt to include the previous attempt's error")
	}
}

// TestVerifyScriptLoop_EmptyOutputTreatedAsFailure guards against the exact
// bug found in live testing: a script shaped like a leftover
// `function transform(input){...}` declaration (never called) runs with no
// JS error but produces empty output. That must NOT be reported as verified.
func TestVerifyScriptLoop_EmptyOutputTreatedAsFailure(t *testing.T) {
	llm := &fakeScriptLLM{responses: []string{
		"function transform(input) { return input; }", // the exact broken shape seen live
		"output.patientName = 'ok';",                   // corrected on retry
	}}
	verifier := &fakeEmptyOutputThenRealVerifier{}
	scriptGen := newScriptGeneratorService(nil)

	script, verified, attempts, lastErr := verifyScriptLoop(context.Background(), llm, scriptGen, verifier,
		ScriptGenInput{Description: "set patient name", StepID: "step-1"}, "MSH|^~\\&|...")

	if !verified {
		t.Fatalf("expected the retry to succeed once the script actually writes output, lastErr=%q", lastErr)
	}
	if attempts != 2 {
		t.Fatalf("expected the first (empty-output) attempt to be rejected and retried, got %d attempts", attempts)
	}
	if script != "output.patientName = 'ok';" {
		t.Fatalf("unexpected final script: %q", script)
	}
}

// fakeEmptyOutputThenRealVerifier simulates the exact live-tested bug: the
// first script call succeeds with no error but empty StepOutput; the second
// succeeds with real output.
type fakeEmptyOutputThenRealVerifier struct{ calls int }

func (f *fakeEmptyOutputThenRealVerifier) ExecutePipeline(_ context.Context, _ *models.TransformationPipeline, _ map[string]interface{}) (*models.TransformationExecutionResult, error) {
	f.calls++
	if f.calls == 1 {
		return &models.TransformationExecutionResult{
			ExecutionLog: []models.StepExecutionLog{{Success: true, StepOutput: &models.StepOutput{OutputData: map[string]interface{}{}}}},
		}, nil
	}
	return &models.TransformationExecutionResult{
		ExecutionLog: []models.StepExecutionLog{{Success: true, StepOutput: &models.StepOutput{OutputData: map[string]interface{}{"patientName": "ok"}}}},
	}, nil
}

func (f *fakeEmptyOutputThenRealVerifier) UpdateStepScript(_ context.Context, _, _ string) error {
	return nil
}

func TestVerifyScriptLoop_GivesUpAfterMaxAttempts(t *testing.T) {
	llm := &fakeScriptLLM{responses: []string{"function transform(input){ return undeclared; }"}}
	verifier := &fakeScriptVerifier{failFirstN: 999} // always fails
	scriptGen := newScriptGeneratorService(nil)

	_, verified, attempts, lastErr := verifyScriptLoop(context.Background(), llm, scriptGen, verifier,
		ScriptGenInput{Description: "do nothing", StepID: "step-1"}, "MSH|^~\\&|...")

	if verified {
		t.Fatal("expected verified=false when every attempt fails")
	}
	if attempts != maxScriptVerifyAttempts {
		t.Fatalf("expected the loop to stop at the cap (%d), got %d", maxScriptVerifyAttempts, attempts)
	}
	if lastErr == "" {
		t.Fatal("expected the last error to be reported so the human can review it")
	}
}

// ─── verify_pipeline_script tool: Prepare verifies once, Execute persists ─────

func TestVerifyPipelineScriptTool_PrepareThenExecute(t *testing.T) {
	llm := &fakeScriptLLM{responses: []string{"function transform(input){return input;}"}}
	verifier := &fakeScriptVerifier{failFirstN: 0}
	scriptGen := newScriptGeneratorService(nil)

	reg := NewToolRegistry()
	registerScriptVerificationTool(reg, llm, scriptGen, verifier)
	tool, ok := reg.Get("verify_pipeline_script")
	if !ok {
		t.Fatal("verify_pipeline_script should be registered when all dependencies are present")
	}
	if !tool.RequiresApproval {
		t.Fatal("verify_pipeline_script must require approval — it persists a script")
	}
	if tool.Prepare == nil {
		t.Fatal("verify_pipeline_script must declare Prepare to run the verify loop before proposal")
	}

	rawArgs := json.RawMessage(`{"step_id":"step-1","description":"do nothing","sample_message":"MSH|^~\\&|..."}`)
	finalArgs, _, err := tool.Prepare(context.Background(), rawArgs)
	if err != nil {
		t.Fatalf("unexpected Prepare error: %v", err)
	}

	var prepared map[string]interface{}
	_ = json.Unmarshal(finalArgs, &prepared)
	if prepared["verified"] != true {
		t.Fatalf("expected verified=true in prepared args, got %v", prepared)
	}
	if verifier.execCalls == 0 {
		t.Fatal("Prepare should have run the verify loop, calling ExecutePipeline at least once")
	}
	if len(verifier.updateCalls) != 0 {
		t.Fatal("Prepare must never persist — UpdateStepScript should only run at Execute (approval) time")
	}

	// Execute (approval time) persists the already-verified script.
	if _, err := tool.Execute(context.Background(), finalArgs); err != nil {
		t.Fatalf("unexpected Execute error: %v", err)
	}
	if len(verifier.updateCalls) != 1 || verifier.updateCalls[0].stepID != "step-1" {
		t.Fatalf("expected UpdateStepScript('step-1', ...) exactly once, got %+v", verifier.updateCalls)
	}
}

func TestRegisterScriptVerificationTool_NilDependenciesOmitted(t *testing.T) {
	reg := NewToolRegistry()
	registerScriptVerificationTool(reg, nil, nil, nil)
	if _, ok := reg.Get("verify_pipeline_script"); ok {
		t.Fatal("verify_pipeline_script should not be registered when its dependencies are nil")
	}
}
