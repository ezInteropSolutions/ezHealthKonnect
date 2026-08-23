// services/ai/agent_service_test.go
// Tests for AgentService's bounded reasoning loop — the mechanism that lets
// read-only tools auto-execute and feed results back to the model without a
// human in the loop, while still capping runaway tool-calling.
package ai

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// loopingToolProvider always returns a tool call for "always_loop", never a
// final answer — used to prove the bounded loop actually terminates.
type loopingToolProvider struct {
	calls int
}

func (p *loopingToolProvider) Generate(_ context.Context, _ string) (string, error) { return "", nil }
func (p *loopingToolProvider) GenerateStream(_ context.Context, _ string, _ func(string) error) error {
	return nil
}
func (p *loopingToolProvider) Embed(_ context.Context, _ string) ([]float32, error) { return nil, nil }
func (p *loopingToolProvider) Ping(_ context.Context) error                        { return nil }
func (p *loopingToolProvider) ProviderName() string                               { return "fake" }
func (p *loopingToolProvider) ChatModelName() string                              { return "fake-model" }
func (p *loopingToolProvider) ChatWithTools(_ context.Context, _ []ChatMessage, _ []ToolSpec) (ChatResult, error) {
	p.calls++
	return ChatResult{ToolCalls: []ToolCall{{Name: "always_loop", Arguments: json.RawMessage(`{}`)}}}, nil
}

func TestProposeAction_BoundedLoopTerminates(t *testing.T) {
	provider := &loopingToolProvider{}

	tools := NewToolRegistry()
	tools.Register(&Tool{
		Name:             "always_loop",
		RequiresApproval: false, // read-only — auto-executes and continues the loop
		Execute: func(_ context.Context, _ json.RawMessage) (map[string]interface{}, error) {
			return map[string]interface{}{"ok": true}, nil
		},
	})

	agent := &AgentService{llm: provider, tools: tools}

	turn, err := agent.ProposeAction(context.Background(), "sess-1", "user-1", "loop forever")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if turn.ProposedAction != nil {
		t.Fatal("a read-only tool should never produce a ProposedAction")
	}
	if turn.Answer == "" {
		t.Fatal("expected a best-effort answer when the loop cap is hit")
	}
	if provider.calls != maxAgentLoopIterations {
		t.Fatalf("expected exactly %d ChatWithTools calls (the loop cap), got %d", maxAgentLoopIterations, provider.calls)
	}
}

// fixedModelProvider returns a configurable ChatModelName plus a canned
// zero-tool-call answer, and records whether ChatWithTools was ever called —
// used to prove the model-capability gate short-circuits BEFORE reaching the
// LLM for a confirmed-unsupported model, and to inspect the returned answer
// for the unverified-model diagnostic suffix.
type fixedModelProvider struct {
	modelName           string
	content             string
	chatWithToolsCalled bool
}

func (p *fixedModelProvider) Generate(_ context.Context, _ string) (string, error) { return "", nil }
func (p *fixedModelProvider) GenerateStream(_ context.Context, _ string, _ func(string) error) error {
	return nil
}
func (p *fixedModelProvider) Embed(_ context.Context, _ string) ([]float32, error) { return nil, nil }
func (p *fixedModelProvider) Ping(_ context.Context) error                        { return nil }
func (p *fixedModelProvider) ProviderName() string                               { return "fake" }
func (p *fixedModelProvider) ChatModelName() string                              { return p.modelName }
func (p *fixedModelProvider) ChatWithTools(_ context.Context, _ []ChatMessage, _ []ToolSpec) (ChatResult, error) {
	p.chatWithToolsCalled = true
	return ChatResult{Content: p.content}, nil // zero tool calls
}

func TestProposeAction_KnownUnsupportedModel_ReturnsWarningWithoutCallingLLM(t *testing.T) {
	provider := &fixedModelProvider{modelName: "qwen2.5-coder:7b", content: "should never be seen"}
	agent := &AgentService{llm: provider, tools: NewToolRegistry()}

	turn, err := agent.ProposeAction(context.Background(), "sess-1", "user-1", "retry the failed message")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider.chatWithToolsCalled {
		t.Fatal("ChatWithTools should not be called for a confirmed-unsupported model")
	}
	if turn.ProposedAction != nil {
		t.Fatal("no action should be proposed for an unsupported model")
	}
	if !strings.Contains(turn.Answer, "qwen2.5-coder:7b") {
		t.Fatalf("expected the warning to name the model, got: %s", turn.Answer)
	}
}

func TestProposeAction_UnknownModel_AppendsDiagnosticSuffixOnZeroToolCalls(t *testing.T) {
	provider := &fixedModelProvider{modelName: "some-custom-model:latest", content: "Here's your answer."}
	agent := &AgentService{llm: provider, tools: NewToolRegistry()}

	turn, err := agent.ProposeAction(context.Background(), "sess-1", "user-1", "what is HL7?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !provider.chatWithToolsCalled {
		t.Fatal("an unverified model should still be attempted, not short-circuited")
	}
	if !strings.Contains(turn.Answer, "Here's your answer.") {
		t.Fatalf("expected the model's own answer to be preserved, got: %s", turn.Answer)
	}
	if !strings.Contains(turn.Answer, "some-custom-model:latest") {
		t.Fatalf("expected the diagnostic suffix naming the model, got: %s", turn.Answer)
	}
}

func TestProposeAction_KnownSupportedModel_NoDiagnosticSuffix(t *testing.T) {
	provider := &fixedModelProvider{modelName: "llama3.2:3b", content: "Here's your answer."}
	agent := &AgentService{llm: provider, tools: NewToolRegistry()}

	turn, err := agent.ProposeAction(context.Background(), "sess-1", "user-1", "what is HL7?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if turn.Answer != "Here's your answer." {
		t.Fatalf("expected no diagnostic suffix for a confirmed-supported model, got: %q", turn.Answer)
	}
}

// singleShotToolProvider returns a tool call once, then a plain answer —
// proves the loop exits early (before the cap) once the model concludes.
type singleShotToolProvider struct {
	loopingToolProvider
	answered bool
}

func (p *singleShotToolProvider) ChatWithTools(ctx context.Context, messages []ChatMessage, tools []ToolSpec) (ChatResult, error) {
	if p.answered {
		return ChatResult{Content: "final answer"}, nil
	}
	p.answered = true
	return p.loopingToolProvider.ChatWithTools(ctx, messages, tools)
}

func TestProposeAction_ReadOnlyToolThenFinalAnswer(t *testing.T) {
	provider := &singleShotToolProvider{}

	tools := NewToolRegistry()
	tools.Register(&Tool{
		Name:             "always_loop",
		RequiresApproval: false,
		Execute: func(_ context.Context, _ json.RawMessage) (map[string]interface{}, error) {
			return map[string]interface{}{"ok": true}, nil
		},
	})

	agent := &AgentService{llm: provider, tools: tools}

	turn, err := agent.ProposeAction(context.Background(), "sess-1", "user-1", "what happened?")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if turn.Answer != "final answer" {
		t.Fatalf("expected the model's final answer after one tool round-trip, got %q", turn.Answer)
	}
	if provider.calls != 1 {
		t.Fatalf("expected exactly 1 ChatWithTools call before the answer, got %d", provider.calls)
	}
}

func TestProposeAction_MutatingToolStopsForApproval(t *testing.T) {
	provider := &loopingToolProvider{}

	tools := NewToolRegistry()
	tools.Register(&Tool{
		Name:             "always_loop",
		EntityType:       "widget",
		RequiresApproval: true, // must stop and propose, never auto-execute
		Execute: func(_ context.Context, _ json.RawMessage) (map[string]interface{}, error) {
			t.Fatal("Execute must not run before approval")
			return nil, nil
		},
	})

	agent := &AgentService{llm: provider, tools: tools} // db=nil: proposal isn't persisted, still returned

	turn, err := agent.ProposeAction(context.Background(), "sess-1", "user-1", "do the mutating thing")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if turn.ProposedAction == nil {
		t.Fatal("expected a ProposedAction for a RequiresApproval tool")
	}
	if turn.ProposedAction.ToolName != "always_loop" {
		t.Fatalf("unexpected tool name: %s", turn.ProposedAction.ToolName)
	}
	if provider.calls != 1 {
		t.Fatalf("expected exactly 1 ChatWithTools call — must stop at the first mutating tool call, got %d", provider.calls)
	}
}
