// services/ai/agent_tools_test.go
// Table-driven unit tests for the Agent Mode tool registry.
// No real LLM, no DB required — fast, deterministic, CI-safe.
package ai

import (
	"context"
	"encoding/json"
	"testing"
)

// ─── Fakes ────────────────────────────────────────────────────────────────────

type fakeRedriveCall struct{ dlqID, mode string }

type fakeRedriver struct {
	calls []fakeRedriveCall
	err   error
}

func (f *fakeRedriver) RedriveMessage(_ context.Context, dlqID, mode string) error {
	f.calls = append(f.calls, fakeRedriveCall{dlqID: dlqID, mode: mode})
	return f.err
}

type fakeActivator struct {
	activated   []string
	deactivated []string
	err         error
}

func (f *fakeActivator) ActivateInterface(id string) error {
	f.activated = append(f.activated, id)
	return f.err
}

func (f *fakeActivator) DeactivateInterface(id string) error {
	f.deactivated = append(f.deactivated, id)
	return f.err
}

// ─── retry_message ────────────────────────────────────────────────────────────

func TestRetryMessageTool(t *testing.T) {
	redriver := &fakeRedriver{}
	reg := NewDefaultToolRegistry(nil, redriver, nil)

	tool, ok := reg.Get("retry_message")
	if !ok {
		t.Fatal("retry_message tool not registered when a redriver is provided")
	}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"dlq_id":"abc123","mode":"from_start"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(redriver.calls) != 1 || redriver.calls[0].dlqID != "abc123" || redriver.calls[0].mode != "from_start" {
		t.Fatalf("redriver not called with expected args: %+v", redriver.calls)
	}
	if result["status"] != "redriven" {
		t.Fatalf("expected status=redriven, got %v", result["status"])
	}

	// Default mode when omitted
	redriver.calls = nil
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"dlq_id":"xyz"}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(redriver.calls) != 1 || redriver.calls[0].mode != "from_failed_step" {
		t.Fatalf("expected default mode 'from_failed_step', got %+v", redriver.calls)
	}

	// Missing dlq_id is rejected before the redriver is ever called
	redriver.calls = nil
	if _, err := tool.Execute(context.Background(), json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected error for missing dlq_id")
	}
	if len(redriver.calls) != 0 {
		t.Fatal("redriver should not be called when arguments are invalid")
	}
}

func TestRetryMessageTool_PropagatesError(t *testing.T) {
	redriver := &fakeRedriver{err: context.DeadlineExceeded}
	reg := NewDefaultToolRegistry(nil, redriver, nil)
	tool, _ := reg.Get("retry_message")

	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"dlq_id":"abc"}`)); err == nil {
		t.Fatal("expected the redriver's error to propagate")
	}
}

// ─── activate_interface / deactivate_interface ────────────────────────────────

func TestActivateDeactivateInterfaceTools(t *testing.T) {
	activator := &fakeActivator{}
	reg := NewDefaultToolRegistry(nil, nil, activator)

	activate, ok := reg.Get("activate_interface")
	if !ok {
		t.Fatal("activate_interface tool not registered when an activator is provided")
	}
	deactivate, ok := reg.Get("deactivate_interface")
	if !ok {
		t.Fatal("deactivate_interface tool not registered when an activator is provided")
	}

	if _, err := activate.Execute(context.Background(), json.RawMessage(`{"interface_id":"iface-1"}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(activator.activated) != 1 || activator.activated[0] != "iface-1" {
		t.Fatalf("expected ActivateInterface('iface-1'), got %+v", activator.activated)
	}

	if _, err := deactivate.Execute(context.Background(), json.RawMessage(`{"interface_id":"iface-2"}`)); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(activator.deactivated) != 1 || activator.deactivated[0] != "iface-2" {
		t.Fatalf("expected DeactivateInterface('iface-2'), got %+v", activator.deactivated)
	}

	// Missing interface_id is rejected
	if _, err := activate.Execute(context.Background(), json.RawMessage(`{}`)); err == nil {
		t.Fatal("expected error for missing interface_id")
	}
}

// ─── propose_pipeline_script ──────────────────────────────────────────────────

func TestProposePipelineScriptTool(t *testing.T) {
	// db=nil: existence check is skipped, tool still validates required fields.
	reg := NewDefaultToolRegistry(nil, nil, nil)
	tool, ok := reg.Get("propose_pipeline_script")
	if !ok {
		t.Fatal("propose_pipeline_script should always be registered — it has no external dependencies")
	}

	result, err := tool.Execute(context.Background(), json.RawMessage(`{"step_id":"step-1","script":"function transform(input){return input;}"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result["step_id"] != "step-1" {
		t.Fatalf("expected step_id echoed back, got %v", result["step_id"])
	}

	if _, err := tool.Execute(context.Background(), json.RawMessage(`{"step_id":"step-1"}`)); err == nil {
		t.Fatal("expected error for missing script")
	}
}

// ─── Registry composition ─────────────────────────────────────────────────────

func TestNewDefaultToolRegistry_NilDependenciesOmitMutatingTools(t *testing.T) {
	reg := NewDefaultToolRegistry(nil, nil, nil)

	for _, name := range []string{"retry_message", "activate_interface", "deactivate_interface"} {
		if _, ok := reg.Get(name); ok {
			t.Errorf("%s should not be registered when its dependency is nil", name)
		}
	}
	if _, ok := reg.Get("propose_pipeline_script"); !ok {
		t.Error("propose_pipeline_script should always be registered")
	}
}

// ─── AIService.Agent gating ───────────────────────────────────────────────────

// MockProvider does not implement ToolCallingProvider, so Agent Mode must stay
// disabled (nil) rather than panicking or silently no-oping on tool calls.
func TestAgentNilWhenProviderLacksToolSupport(t *testing.T) {
	mock := NewMockProvider(nil)
	svc := NewAIServiceWithProvider(nil, mock, mock)
	if svc.Agent != nil {
		t.Fatal("expected Agent to be nil when the chat provider does not implement ToolCallingProvider")
	}
}
