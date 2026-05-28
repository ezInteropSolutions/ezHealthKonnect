// services/ai/model_manager.go
// Curated model catalog and Ollama pull logic for the model browser UI.
// All models run locally — no PHI leaves the network.
package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// CatalogModel describes one entry in the recommended model catalog.
type CatalogModel struct {
	Name        string `json:"name"`         // Ollama model name (e.g. "llama3.1:8b")
	DisplayName string `json:"display_name"` // Human-friendly label
	RAMRequired string `json:"ram_required"` // Minimum RAM (e.g. "6 GB")
	DiskSize    string `json:"disk_size"`    // Download size
	BestFor     string `json:"best_for"`     // Short use-case description
	Recommended bool   `json:"recommended"`  // Show "Recommended" badge
	Tags        []string `json:"tags"`       // e.g. ["code", "reasoning", "fast"]
	Notes       string `json:"notes"`        // Extra guidance shown in UI
}

// ModelCatalog returns the curated list of Ollama models validated for ezHealthKonnect.
// Listed in ascending RAM order so users on limited hardware see their options first.
func ModelCatalog() []CatalogModel {
	return []CatalogModel{
		{
			Name:        "llama3.2:3b",
			DisplayName: "Llama 3.2 3B",
			RAMRequired: "2 GB",
			DiskSize:    "2.0 GB",
			BestFor:     "Demo / minimum viable",
			Recommended: false,
			Tags:        []string{"fast", "cpu-only"},
			Notes:       "Default. Good for quick demos on constrained hardware. Weaker reasoning — expect occasional hallucinations on complex HL7 questions.",
		},
		{
			Name:        "qwen2.5-coder:7b",
			DisplayName: "Qwen 2.5 Coder 7B",
			RAMRequired: "5 GB",
			DiskSize:    "4.7 GB",
			BestFor:     "Pipeline scripts + HL7/FHIR mappings",
			Recommended: true,
			Tags:        []string{"code", "mappings", "recommended"},
			Notes:       "Best quality-per-GB for ezHealthKonnect. Excels at JavaScript transform() generation and HL7→FHIR reasoning. Runs well on CPU-only 8 GB hosts.",
		},
		{
			Name:        "llama3.1:8b",
			DisplayName: "Llama 3.1 8B",
			RAMRequired: "6 GB",
			DiskSize:    "5.0 GB",
			BestFor:     "General reasoning + spec questions",
			Recommended: false,
			Tags:        []string{"general", "reasoning"},
			Notes:       "Well-rounded model. Good for HL7 spec Q&A, pipeline design guidance, and error diagnosis. Slower than Qwen on code tasks.",
		},
		{
			Name:        "mistral-nemo:12b",
			DisplayName: "Mistral Nemo 12B",
			RAMRequired: "8 GB",
			DiskSize:    "7.1 GB",
			BestFor:     "Best quality under 16 GB RAM",
			Recommended: false,
			Tags:        []string{"reasoning", "quality"},
			Notes:       "Strong reasoning — noticeably better than 7-8B models at multi-step HL7→FHIR transforms and pipeline debugging. Needs 8 GB free RAM.",
		},
		{
			Name:        "qwen2.5-coder:14b",
			DisplayName: "Qwen 2.5 Coder 14B",
			RAMRequired: "10 GB",
			DiskSize:    "8.9 GB",
			BestFor:     "Premium code generation",
			Recommended: false,
			Tags:        []string{"code", "premium"},
			Notes:       "Excellent pipeline script generation. Best choice when you have 16+ GB RAM and need high-quality JavaScript transform() functions.",
		},
		{
			Name:        "llama3.3:70b-instruct-q4_K_M",
			DisplayName: "Llama 3.3 70B (Q4)",
			RAMRequired: "40 GB",
			DiskSize:    "42 GB",
			BestFor:     "Production-grade — near Claude quality",
			Recommended: false,
			Tags:        []string{"large", "premium", "gpu"},
			Notes:       "Claude-class reasoning. Requires a dedicated GPU or 48+ GB RAM. Ideal for production deployments where answer quality is critical.",
		},
	}
}

// PullProgress is one progress event from an Ollama model pull.
type PullProgress struct {
	Status    string `json:"status"`
	Digest    string `json:"digest,omitempty"`
	Total     int64  `json:"total,omitempty"`
	Completed int64  `json:"completed,omitempty"`
	Percent   int    `json:"percent,omitempty"`
}

type ollamaPullRequest struct {
	Name   string `json:"name"`
	Stream bool   `json:"stream"`
}

type ollamaPullChunk struct {
	Status    string `json:"status"`
	Digest    string `json:"digest"`
	Total     int64  `json:"total"`
	Completed int64  `json:"completed"`
}

// PullModel triggers an Ollama model pull and calls onProgress for each status event.
// onProgress may return an error to stop streaming early (e.g. client disconnect).
func (c *OllamaClient) PullModel(ctx context.Context, modelName string, onProgress func(PullProgress) error) error {
	body, _ := json.Marshal(ollamaPullRequest{Name: modelName, Stream: true})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/pull", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	// Use a client without timeout — large models can take 30+ minutes to download
	pullClient := &http.Client{}
	resp, err := pullClient.Do(req)
	if err != nil {
		return fmt.Errorf("ollama pull: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ollama pull HTTP %d: %s", resp.StatusCode, string(b))
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		var chunk ollamaPullChunk
		if err := json.Unmarshal(scanner.Bytes(), &chunk); err != nil {
			continue
		}
		progress := PullProgress{
			Status:    chunk.Status,
			Digest:    chunk.Digest,
			Total:     chunk.Total,
			Completed: chunk.Completed,
		}
		if chunk.Total > 0 {
			progress.Percent = int(float64(chunk.Completed) / float64(chunk.Total) * 100)
		}
		if err := onProgress(progress); err != nil {
			return err
		}
	}
	return scanner.Err()
}
