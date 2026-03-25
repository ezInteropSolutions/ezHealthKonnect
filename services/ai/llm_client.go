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
)

// OllamaClient talks to a local Ollama instance via its REST API.
type OllamaClient struct {
	baseURL    string
	httpClient *http.Client
	ChatModel  string
	EmbedModel string
}

// NewOllamaClient creates a client from environment variables.
// OLLAMA_HOST        (default: http://ollama:11434)
// OLLAMA_CHAT_MODEL  (default: llama3.1:8b)
// OLLAMA_EMBED_MODEL (default: nomic-embed-text)
func NewOllamaClient() *OllamaClient {
	host := os.Getenv("OLLAMA_HOST")
	if host == "" {
		host = "http://ollama:11434"
	}
	chatModel := os.Getenv("OLLAMA_CHAT_MODEL")
	if chatModel == "" {
		chatModel = "llama3.2:3b" // 3B is ~3x faster on CPU-only Docker; override via OLLAMA_CHAT_MODEL env
	}
	embedModel := os.Getenv("OLLAMA_EMBED_MODEL")
	if embedModel == "" {
		embedModel = "nomic-embed-text"
	}
	return &OllamaClient{
		baseURL: host,
		httpClient: &http.Client{
			Timeout: 300 * time.Second, // LLM generation can be slow on CPU (llama3.1:8b cold start ~2–3 min)
		},
		ChatModel:  chatModel,
		EmbedModel: embedModel,
	}
}

// ─── Generate (text completion) ───────────────────────────────────────────────

type generateRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
	Stream bool   `json:"stream"`
}

type generateResponse struct {
	Response string `json:"response"`
	Done     bool   `json:"done"`
}

// Generate sends a prompt to the chat model and returns the full response text.
func (c *OllamaClient) Generate(ctx context.Context, prompt string) (string, error) {
	body, _ := json.Marshal(generateRequest{
		Model:  c.ChatModel,
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

// GenerateStream sends a prompt and calls onToken for each token as it arrives.
// Returns when generation is complete or ctx is cancelled.
func (c *OllamaClient) GenerateStream(ctx context.Context, prompt string, onToken func(string) error) error {
	body, _ := json.Marshal(generateRequest{
		Model:  c.ChatModel,
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
func (c *OllamaClient) ChatModelName() string { return c.ChatModel }

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
