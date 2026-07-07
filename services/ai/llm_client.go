// services/ai/llm_client.go
// Ollama REST client — all inference stays on-premise, no PHI leaves the network.
package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"ezhealthkonnect/services"
)

// OllamaClient talks to a local Ollama instance via its REST API.
type OllamaClient struct {
	baseURL    string
	httpClient *http.Client
	ChatModel  string
	EmbedModel string
}

// NewOllamaClient creates a client, preferring DB system_settings over env vars.
// Priority: DB (system_settings key="ai") → OLLAMA_* env vars → hardcoded defaults.
// OLLAMA_HOST        (default: http://ollama:11434)
// OLLAMA_CHAT_MODEL  (default: llama3.2:3b)
// OLLAMA_EMBED_MODEL (default: nomic-embed-text)
func NewOllamaClient() *OllamaClient {
	host := os.Getenv("OLLAMA_HOST")
	if host == "" {
		host = "http://ollama:11434"
	}

	// Seed from DB settings; chatModel() re-reads on every request so the
	// stored value here is only the startup snapshot / fallback.
	dbCfg := services.GetAppSettings().GetAIConfig().Ollama

	chatModel := dbCfg.ChatModel
	if chatModel == "" {
		chatModel = os.Getenv("OLLAMA_CHAT_MODEL")
	}
	if chatModel == "" {
		chatModel = "llama3.2:3b"
	}

	embedModel := dbCfg.EmbedModel
	if embedModel == "" {
		embedModel = os.Getenv("OLLAMA_EMBED_MODEL")
	}
	if embedModel == "" {
		embedModel = "nomic-embed-text"
	}

	return &OllamaClient{
		baseURL: host,
		httpClient: &http.Client{
			Timeout: 600 * time.Second, // large models on CPU can take several minutes
		},
		ChatModel:  chatModel,
		EmbedModel: embedModel,
	}
}

// chatModel returns the active chat model name, re-reading from the settings
// cache on every call so a model switch in the UI takes effect within the
// cache TTL (~5 min) without restarting the process.
func (c *OllamaClient) chatModel() string {
	if m := services.GetAppSettings().GetAIConfig().Ollama.ChatModel; m != "" {
		return m
	}
	return c.ChatModel
}

// ─── Generate (text completion) ───────────────────────────────────────────────

type generateRequest struct {
	Model   string         `json:"model"`
	Prompt  string         `json:"prompt"`
	Stream  bool           `json:"stream"`
	Options map[string]any `json:"options,omitempty"`
}

type generateResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

// Generate sends a prompt to the chat model and returns the full response text.
func (c *OllamaClient) Generate(ctx context.Context, prompt string) (string, error) {
	body, _ := json.Marshal(generateRequest{
		Model:  c.chatModel(),
		Prompt: prompt,
		Stream: false,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama generate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama generate HTTP %d: %s", resp.StatusCode, string(b))
	}

	var result generateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("ollama generate decode: %w", err)
	}
	return result.Response, nil
}

// GenerateCapped sends a prompt with a hard token cap (num_predict) to bound latency.
func (c *OllamaClient) GenerateCapped(ctx context.Context, prompt string, maxTokens int) (string, error) {
	opts := map[string]any{"num_predict": maxTokens, "temperature": 0.1}
	body, _ := json.Marshal(generateRequest{Model: c.chatModel(), Prompt: prompt, Stream: false, Options: opts})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama generate: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("ollama generate HTTP %d: %s", resp.StatusCode, string(b))
	}
	var result generateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("ollama generate decode: %w", err)
	}
	return result.Response, nil
}

// GenerateStream sends a prompt and calls onToken for each token as it arrives.
// Returns when generation is complete or ctx is cancelled.
func (c *OllamaClient) GenerateStream(ctx context.Context, prompt string, onToken func(string) error) error {
	body, _ := json.Marshal(generateRequest{
		Model:  c.chatModel(),
		Prompt: prompt,
		Stream: true,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ollama generate stream: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ollama generate HTTP %d: %s", resp.StatusCode, string(b))
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		var chunk generateResponse
		if err := json.Unmarshal(scanner.Bytes(), &chunk); err != nil {
			continue
		}
		if chunk.Response != "" {
			if err := onToken(chunk.Response); err != nil {
				return err // caller signalled stop (e.g. client disconnected)
			}
		}
		if chunk.Done {
			break
		}
	}
	return scanner.Err()
}

// ─── Embed ────────────────────────────────────────────────────────────────────

type embedRequest struct {
	Model string `json:"model"`
	Input string `json:"input"`
}

type embedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

// Embed returns a 768-dimensional vector for the given text (nomic-embed-text).
func (c *OllamaClient) Embed(ctx context.Context, text string) ([]float32, error) {
	body, _ := json.Marshal(embedRequest{
		Model: c.EmbedModel,
		Input: text,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama embed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ollama embed HTTP %d: %s", resp.StatusCode, string(b))
	}

	var result embedResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ollama embed decode: %w", err)
	}
	if len(result.Embeddings) == 0 {
		return nil, fmt.Errorf("ollama embed: empty response")
	}
	return result.Embeddings[0], nil
}

// ─── Chat with tools ──────────────────────────────────────────────────────────

type chatMessageWire struct {
	Role      string         `json:"role"`
	Content   string         `json:"content"`
	ToolCalls []toolCallWire `json:"tool_calls,omitempty"`
}

type toolFunctionWire struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type toolWire struct {
	Type     string           `json:"type"`
	Function toolFunctionWire `json:"function"`
}

type chatRequest struct {
	Model    string            `json:"model"`
	Messages []chatMessageWire `json:"messages"`
	Tools    []toolWire        `json:"tools,omitempty"`
	Stream   bool              `json:"stream"`
}

type toolCallWire struct {
	Function struct {
		Name      string                 `json:"name"`
		Arguments map[string]interface{} `json:"arguments"`
	} `json:"function"`
}

type chatResponse struct {
	Message struct {
		Role      string         `json:"role"`
		Content   string         `json:"content"`
		ToolCalls []toolCallWire `json:"tool_calls,omitempty"`
	} `json:"message"`
	Done bool `json:"done"`
}

// ChatWithTools sends a multi-turn chat request with an optional tool list via
// Ollama's /api/chat endpoint. Satisfies ToolCallingProvider for models that
// support tool calling (e.g. llama3.1+, llama3.2, qwen2.5).
func (c *OllamaClient) ChatWithTools(ctx context.Context, messages []ChatMessage, tools []ToolSpec) (ChatResult, error) {
	wireMessages := make([]chatMessageWire, len(messages))
	for i, m := range messages {
		wire := chatMessageWire{Role: m.Role, Content: m.Content}
		if len(m.ToolCalls) > 0 {
			wire.ToolCalls = make([]toolCallWire, len(m.ToolCalls))
			for j, tc := range m.ToolCalls {
				var args map[string]interface{}
				_ = json.Unmarshal(tc.Arguments, &args)
				wire.ToolCalls[j].Function.Name = tc.Name
				wire.ToolCalls[j].Function.Arguments = args
			}
		}
		wireMessages[i] = wire
	}

	wireTools := make([]toolWire, len(tools))
	for i, t := range tools {
		wireTools[i] = toolWire{
			Type: "function",
			Function: toolFunctionWire{
				Name:        t.Name,
				Description: t.Description,
				Parameters:  t.Parameters,
			},
		}
	}

	body, _ := json.Marshal(chatRequest{
		Model:    c.chatModel(),
		Messages: wireMessages,
		Tools:    wireTools,
		Stream:   false,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return ChatResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return ChatResult{}, fmt.Errorf("ollama chat: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return ChatResult{}, fmt.Errorf("ollama chat HTTP %d: %s", resp.StatusCode, string(b))
	}

	var result chatResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return ChatResult{}, fmt.Errorf("ollama chat decode: %w", err)
	}

	toolCalls := make([]ToolCall, 0, len(result.Message.ToolCalls))
	for _, tc := range result.Message.ToolCalls {
		argsJSON, _ := json.Marshal(tc.Function.Arguments)
		toolCalls = append(toolCalls, ToolCall{Name: tc.Function.Name, Arguments: argsJSON})
	}

	return ChatResult{Content: result.Message.Content, ToolCalls: toolCalls}, nil
}

// ─── Health ───────────────────────────────────────────────────────────────────

type tagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

// Ping checks whether Ollama is reachable.
func (c *OllamaClient) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/tags", nil)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ollama unreachable: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama ping HTTP %d", resp.StatusCode)
	}
	return nil
}

// ─── LLMProvider / EmbedProvider interface ────────────────────────────────────

// ProviderName satisfies LLMProvider.
func (c *OllamaClient) ProviderName() string { return "ollama" }

// ChatModelName satisfies LLMProvider.
func (c *OllamaClient) ChatModelName() string { return c.chatModel() }

// EmbedModelName satisfies EmbedProvider.
func (c *OllamaClient) EmbedModelName() string { return c.EmbedModel }

// ListModels returns the names of all locally available Ollama models.
func (c *OllamaClient) ListModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result tagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	names := make([]string, len(result.Models))
	for i, m := range result.Models {
		names[i] = m.Name
	}
	return names, nil
}
