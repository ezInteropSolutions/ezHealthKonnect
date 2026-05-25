// services/ai/rag_service.go
// RAG (Retrieval Augmented Generation): finds similar knowledge chunks
// via pgvector cosine search, then augments the LLM prompt with them.
package ai

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

const defaultTopK = 6

// RAGService retrieves knowledge chunks and builds augmented LLM prompts.
type RAGService struct {
	db      *sql.DB
	ollama  *OllamaClient  // kept for backward compat
	chat    LLMProvider    // generation provider
	embeddr LLMProvider    // embed provider (Ollama for RAG consistency)
}

// NewRAGService creates a new RAGService using OllamaClient (backward compat).
func NewRAGService(db *sql.DB, ollama *OllamaClient) *RAGService {
	return &RAGService{db: db, ollama: ollama, chat: ollama, embeddr: ollama}
}

// newRAGServiceWithProvider creates a RAGService with injected providers.
func newRAGServiceWithProvider(db *sql.DB, chat, embeddr LLMProvider) *RAGService {
	return &RAGService{db: db, chat: chat, embeddr: embeddr}
}

// RetrievedChunk is a knowledge chunk returned by similarity search.
type RetrievedChunk struct {
	SourceType string  `json:"source_type"`
	SourceRef  string  `json:"source_ref"`
	Content    string  `json:"content"`
	Similarity float64 `json:"similarity"`
}

// Retrieve finds the top-K most semantically similar chunks for a query.
// filterSourceTypes limits the search to specific source types (nil = all).
func (r *RAGService) Retrieve(ctx context.Context, query string, topK int, filterSourceTypes []string) ([]RetrievedChunk, error) {
	if r.db == nil {
		return nil, nil // no knowledge store — answer without RAG context
	}
	if topK <= 0 {
		topK = defaultTopK
	}

	embedder := r.embeddr
	if embedder == nil {
		embedder = r.ollama
	}
	queryEmbedding, err := embedder.Embed(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embed query: %w", err)
	}
	embeddingStr := float32SliceToPGVector(queryEmbedding)

	var (
		rows    *sql.Rows
		queryErr error
	)

	if len(filterSourceTypes) > 0 {
		placeholders := make([]string, len(filterSourceTypes))
		args := make([]interface{}, 0, len(filterSourceTypes)+2)
		args = append(args, embeddingStr, topK)
		for i, st := range filterSourceTypes {
			placeholders[i] = fmt.Sprintf("$%d", i+3)
			args = append(args, st)
		}
		sql := fmt.Sprintf(`
			SELECT source_type, COALESCE(source_ref,''), content,
			       1 - (embedding <=> $1::vector) AS similarity
			FROM ai_knowledge_chunks
			WHERE source_type IN (%s)
			ORDER BY embedding <=> $1::vector
			LIMIT $2
		`, strings.Join(placeholders, ","))
		rows, queryErr = r.db.QueryContext(ctx, sql, args...)
	} else {
		rows, queryErr = r.db.QueryContext(ctx, `
			SELECT source_type, COALESCE(source_ref,''), content,
			       1 - (embedding <=> $1::vector) AS similarity
			FROM ai_knowledge_chunks
			ORDER BY embedding <=> $1::vector
			LIMIT $2
		`, embeddingStr, topK)
	}
	if queryErr != nil {
		return nil, fmt.Errorf("similarity search: %w", queryErr)
	}
	defer rows.Close()

	var chunks []RetrievedChunk
	for rows.Next() {
		var c RetrievedChunk
		if err := rows.Scan(&c.SourceType, &c.SourceRef, &c.Content, &c.Similarity); err != nil {
			continue
		}
		chunks = append(chunks, c)
	}
	return chunks, nil
}

// Query retrieves relevant context chunks and sends an augmented prompt to the LLM.
// If retrieval fails it degrades gracefully and answers without context.
func (r *RAGService) Query(ctx context.Context, question, systemPrompt string, topK int, filterSourceTypes []string) (string, []RetrievedChunk, error) {
	// Use a short timeout for retrieval so a missing embed model (e.g. nomic-embed-text not
	// installed in Ollama) fails fast instead of blocking until the parent context expires.
	retrieveCtx, retrieveCancel := context.WithTimeout(ctx, 8*time.Second)
	defer retrieveCancel()
	chunks, _ := r.Retrieve(retrieveCtx, question, topK, filterSourceTypes) // non-fatal

	llm := r.chat
	if llm == nil {
		llm = r.ollama
	}
	prompt := buildRAGPrompt(systemPrompt, question, chunks)
	answer, err := llm.Generate(ctx, prompt)
	if err != nil {
		return "", chunks, fmt.Errorf("llm generate: %w", err)
	}
	return answer, chunks, nil
}

// QueryCapped is like Query but caps the LLM output at maxTokens to bound latency.
// Falls back to Query if the underlying provider does not support GenerateCapped.
func (r *RAGService) QueryCapped(ctx context.Context, question, systemPrompt string, topK, maxTokens int, filterSourceTypes []string) (string, []RetrievedChunk, error) {
	retrieveCtx, retrieveCancel := context.WithTimeout(ctx, 8*time.Second)
	defer retrieveCancel()
	chunks, _ := r.Retrieve(retrieveCtx, question, topK, filterSourceTypes) // non-fatal

	prompt := buildRAGPrompt(systemPrompt, question, chunks)

	llm := r.chat
	if llm == nil {
		llm = r.ollama
	}
	// Use GenerateCapped when the provider supports it.
	type capper interface {
		GenerateCapped(ctx context.Context, prompt string, maxTokens int) (string, error)
	}
	if c, ok := llm.(capper); ok {
		answer, err := c.GenerateCapped(ctx, prompt, maxTokens)
		if err != nil {
			return "", chunks, fmt.Errorf("llm generate: %w", err)
		}
		return answer, chunks, nil
	}
	answer, err := llm.Generate(ctx, prompt)
	if err != nil {
		return "", chunks, fmt.Errorf("llm generate: %w", err)
	}
	return answer, chunks, nil
}

// QueryStream retrieves context chunks then streams tokens to onToken as they arrive.
func (r *RAGService) QueryStream(ctx context.Context, question, systemPrompt string, topK int, filterSourceTypes []string, onToken func(string) error) ([]RetrievedChunk, error) {
	retrieveCtx, retrieveCancel := context.WithTimeout(ctx, 8*time.Second)
	defer retrieveCancel()
	chunks, _ := r.Retrieve(retrieveCtx, question, topK, filterSourceTypes) // non-fatal
	llm2 := r.chat
	if llm2 == nil {
		llm2 = r.ollama
	}
	prompt := buildRAGPrompt(systemPrompt, question, chunks)
	if err := llm2.GenerateStream(ctx, prompt, onToken); err != nil {
		return chunks, fmt.Errorf("llm stream: %w", err)
	}
	return chunks, nil
}

// buildRAGPrompt assembles the full prompt: system instructions + retrieved context + question.
func buildRAGPrompt(systemPrompt, question string, chunks []RetrievedChunk) string {
	var sb strings.Builder

	if systemPrompt != "" {
		sb.WriteString(systemPrompt)
		sb.WriteString("\n\n")
	}

	if len(chunks) > 0 {
		sb.WriteString("### Relevant Healthcare Standards Reference\n\n")
		for i, c := range chunks {
			sb.WriteString(fmt.Sprintf("--- Reference %d [%s: %s] ---\n", i+1, c.SourceType, c.SourceRef))
			sb.WriteString(c.Content)
			sb.WriteString("\n\n")
		}
		sb.WriteString("---\n\n")
	}

	sb.WriteString("### Question\n\n")
	sb.WriteString(question)
	sb.WriteString("\n\n### Answer\n")
	return sb.String()
}
