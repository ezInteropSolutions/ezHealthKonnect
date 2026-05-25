// services/ai/mock_provider.go
// MockProvider — deterministic LLMProvider for unit tests.
// Matches responses by prompt prefix; records all calls for assertion.
package ai

import (
	"context"
	"strings"
	"sync"
)

// MockProvider is a test double for LLMProvider.
// Register responses by prompt prefix; longest match wins.
// Thread-safe.
type MockProvider struct {
	mu        sync.Mutex
	responses map[string]string // prefix → response
	callLog   []string          // every prompt received
	embedVec  []float32         // fixed embed vector returned for every text
}

// NewMockProvider creates a MockProvider pre-seeded with the given responses.
// Key = prompt prefix (case-sensitive), value = full response text.
// Use an empty string key "" as a catch-all default.
func NewMockProvider(responses map[string]string) *MockProvider {
	m := &MockProvider{
		responses: make(map[string]string),
		embedVec:  make([]float32, 768), // 768-dim zero vector, same as nomic-embed-text
	}
	for k, v := range responses {
		m.responses[k] = v
	}
	return m
}

// RegisterResponse adds or replaces a prefix → response mapping.
func (m *MockProvider) RegisterResponse(prefix, response string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses[prefix] = response
}

// SetEmbedVector overrides the fixed embedding vector returned by Embed().
func (m *MockProvider) SetEmbedVector(v []float32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.embedVec = v
}

// CallCount returns how many Generate/GenerateStream calls were made.
func (m *MockProvider) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.callLog)
}

// LastPrompt returns the most recently received full prompt, or "" if none.
func (m *MockProvider) LastPrompt() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.callLog) == 0 {
		return ""
	}
	return m.callLog[len(m.callLog)-1]
}

// Calls returns a copy of the full call log.
func (m *MockProvider) Calls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]string, len(m.callLog))
	copy(out, m.callLog)
	return out
}

// match returns the response for the best (longest) matching prefix.
func (m *MockProvider) match(prompt string) string {
	best := ""
	bestLen := -1
	for prefix, resp := range m.responses {
		if prefix == "" || strings.Contains(prompt, prefix) {
			if len(prefix) > bestLen {
				best = resp
				bestLen = len(prefix)
			}
		}
	}
	if bestLen == -1 {
		return "[mock response for: " + truncate(prompt, 80) + "]"
	}
	return best
}

// ─── LLMProvider interface ────────────────────────────────────────────────────

func (m *MockProvider) Generate(_ context.Context, prompt string) (string, error) {
	m.mu.Lock()
	m.callLog = append(m.callLog, prompt)
	resp := m.match(prompt)
	m.mu.Unlock()
	return resp, nil
}

// GenerateStream splits the matched response into ~12-char token chunks.
func (m *MockProvider) GenerateStream(_ context.Context, prompt string, onToken func(string) error) error {
	m.mu.Lock()
	m.callLog = append(m.callLog, prompt)
	resp := m.match(prompt)
	m.mu.Unlock()

	const chunkSize = 12
	for i := 0; i < len(resp); i += chunkSize {
		end := i + chunkSize
		if end > len(resp) {
			end = len(resp)
		}
		if err := onToken(resp[i:end]); err != nil {
			return err
		}
	}
	return nil
}

func (m *MockProvider) Embed(_ context.Context, _ string) ([]float32, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]float32, len(m.embedVec))
	copy(out, m.embedVec)
	return out, nil
}

func (m *MockProvider) Ping(_ context.Context) error      { return nil }
func (m *MockProvider) ProviderName() string              { return "mock" }
func (m *MockProvider) ChatModelName() string             { return "mock-model" }
func (m *MockProvider) EmbedModelName() string            { return "mock-embed" }
func (m *MockProvider) ListModels(_ context.Context) ([]string, error) {
	return []string{"mock-model"}, nil
}
