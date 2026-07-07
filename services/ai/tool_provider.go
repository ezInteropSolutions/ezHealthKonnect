// services/ai/tool_provider.go
// ToolCallingProvider — optional extension of LLMProvider for backends that
// support function/tool calling (e.g. Ollama's /api/chat with a tools array).
// Mirrors the EmbedProvider pattern: AIService type-asserts to detect support
// rather than requiring every LLMProvider implementation to carry this method.
package ai

import "context"

// ChatMessage is one turn in a tool-calling chat exchange.
type ChatMessage struct {
	Role    string // "system" | "user" | "assistant" | "tool"
	Content string
	// ToolCalls is set on a role="assistant" message that is echoing a tool
	// call the model previously made, so its result (the next role="tool"
	// message) has the right context when the conversation continues.
	ToolCalls []ToolCall
}

// ToolSpec describes one callable tool to the model.
type ToolSpec struct {
	Name        string
	Description string
	// Parameters is a JSON Schema object describing the tool's arguments,
	// e.g. {"type":"object","properties":{"dlq_id":{"type":"string"}},"required":["dlq_id"]}
	Parameters map[string]interface{}
}

// ToolCall is a single tool invocation the model requested.
type ToolCall struct {
	Name      string
	Arguments []byte // raw JSON object, decoded by the tool itself
}

// ChatResult is the model's response to a tool-calling chat request.
// Content is populated when the model answered directly; ToolCalls is
// populated when the model chose to invoke one or more tools instead.
type ChatResult struct {
	Content   string
	ToolCalls []ToolCall
}

// ToolCallingProvider extends LLMProvider for backends that can select from a
// list of tools instead of only generating free text.
type ToolCallingProvider interface {
	LLMProvider
	ChatWithTools(ctx context.Context, messages []ChatMessage, tools []ToolSpec) (ChatResult, error)
}
