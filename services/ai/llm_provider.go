// services/ai/llm_provider.go
// LLMProvider — unified interface for all language model backends.
// Implementations: OllamaClient (existing), OpenAIProvider, AnthropicProvider, MockProvider.
package ai

import (
	"context"
	"errors"
)

// ErrEmbedNotSupported is returned by providers that do not support embeddings (e.g. Anthropic).
// The caller should fall back to Ollama embeddings.
var ErrEmbedNotSupported = errors.New("embedding not supported by this provider; use Ollama for embeddings")

// ErrProviderDisabled is returned when AI is disabled in system settings.
var ErrProviderDisabled = errors.New("AI assistant is disabled")

// LLMProvider is the unified contract for every language model backend.
// All call sites in AIService use this interface; the concrete type is injected at startup.
type LLMProvider interface {
	// Generate returns a complete response for the given prompt (blocking).
	Generate(ctx context.Context, prompt string) (string, error)

	// GenerateStream streams tokens by calling onToken for each one.
	// Returns when generation is complete or ctx is cancelled.
	// onToken may return an error to stop generation early (e.g. client disconnect).
	GenerateStream(ctx context.Context, prompt string, onToken func(string) error) error

	// Embed returns a vector embedding for the given text.
	// Providers without embedding support must return ErrEmbedNotSupported.
	Embed(ctx context.Context, text string) ([]float32, error)

	// Ping checks reachability. Returns nil if the provider is available.
	Ping(ctx context.Context) error

	// ProviderName returns a stable lowercase identifier ("ollama", "openai", "anthropic").
	ProviderName() string

	// ChatModelName returns the model identifier used for generation.
	ChatModelName() string
}

// EmbedProvider extends LLMProvider for backends that support a dedicated embed model.
type EmbedProvider interface {
	LLMProvider
	EmbedModelName() string
	ListModels(ctx context.Context) ([]string, error)
}
