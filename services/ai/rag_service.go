// services/ai/rag_service.go
// RAG (Retrieval Augmented Generation): finds similar knowledge chunks
// via pgvector cosine search, then augments the LLM prompt with them.
package ai

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"ezhealthkonnect/services"
)

const defaultTopK = 6

// rrfK is the standard damping constant from Cormack, Clarke & Buettcher (2009)
// "Reciprocal Rank Fusion" — large enough that a single list's rank-1 item doesn't
// completely dominate a chunk that ranks well in both the dense and lexical lists.
const rrfK = 60

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
	ID         string  `json:"id,omitempty"`
	SourceType string  `json:"source_type"`
	SourceRef  string  `json:"source_ref"`
	Content    string  `json:"content"`
	// Similarity is always the genuine pgvector cosine similarity for this chunk,
	// regardless of whether it was surfaced by dense search, lexical search, or
	// both (see lexicalRetrieve) — isLowConfidence (guardrails.go) depends on this
	// staying a real cosine-similarity value, never a fused or lexical-only score.
	Similarity float64 `json:"similarity"`
}

// Retrieve finds the top-K most relevant chunks for a query. filterSourceTypes
// limits the search to specific source types (nil = all).
//
// When AIConfig.HybridRetrievalEnabled is off (the default), this runs the exact
// same pure dense vector search as before hybrid retrieval existed. When on, it
// also runs a lexical full-text search (content_tsv, added in V220) and fuses the
// two rankings via Reciprocal Rank Fusion — this helps exact-token questions
// (segment paths, error strings, connector names) that pure semantic search can
// rank below a merely-similar chunk. See fuseRRF's doc comment for why this is
// safe to add without touching guardrails.go's confidence calibration.
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

	if !services.GetAppSettings().GetAIConfig().HybridRetrievalEnabled {
		return r.denseRetrieve(ctx, embeddingStr, topK, filterSourceTypes)
	}

	// Hybrid path: widen the candidate pool on each leg so RRF has enough overlap
	// to meaningfully reorder, then fuse down to the requested topK.
	pool := max(topK*3, 15)
	denseChunks, err := r.denseRetrieve(ctx, embeddingStr, pool, filterSourceTypes)
	if err != nil {
		return nil, err
	}
	// Lexical search is treated as non-fatal, same as Retrieve is treated as
	// non-fatal by Query/QueryCapped/QueryStream below — a lexical-search problem
	// should degrade to dense-only, not break the answer.
	lexicalChunks, _ := r.lexicalRetrieve(ctx, query, embeddingStr, pool, filterSourceTypes)

	return fuseRRF(denseChunks, lexicalChunks, topK), nil
}

// denseRetrieve runs the pure pgvector cosine-similarity search — the same query
// Retrieve has always run, factored out so both the pure-dense and hybrid paths
// share one implementation.
func (r *RAGService) denseRetrieve(ctx context.Context, embeddingStr string, limit int, filterSourceTypes []string) ([]RetrievedChunk, error) {
	var (
		rows     *sql.Rows
		queryErr error
	)

	if len(filterSourceTypes) > 0 {
		placeholders := make([]string, len(filterSourceTypes))
		args := make([]interface{}, 0, len(filterSourceTypes)+2)
		args = append(args, embeddingStr, limit)
		for i, st := range filterSourceTypes {
			placeholders[i] = fmt.Sprintf("$%d", i+3)
			args = append(args, st)
		}
		sql := fmt.Sprintf(`
			SELECT id, source_type, COALESCE(source_ref,''), content,
			       1 - (embedding <=> $1::vector) AS similarity
			FROM ai_knowledge_chunks
			WHERE source_type IN (%s)
			ORDER BY embedding <=> $1::vector
			LIMIT $2
		`, strings.Join(placeholders, ","))
		rows, queryErr = r.db.QueryContext(ctx, sql, args...)
	} else {
		rows, queryErr = r.db.QueryContext(ctx, `
			SELECT id, source_type, COALESCE(source_ref,''), content,
			       1 - (embedding <=> $1::vector) AS similarity
			FROM ai_knowledge_chunks
			ORDER BY embedding <=> $1::vector
			LIMIT $2
		`, embeddingStr, limit)
	}
	if queryErr != nil {
		return nil, fmt.Errorf("similarity search: %w", queryErr)
	}
	defer rows.Close()

	var chunks []RetrievedChunk
	for rows.Next() {
		var c RetrievedChunk
		if err := rows.Scan(&c.ID, &c.SourceType, &c.SourceRef, &c.Content, &c.Similarity); err != nil {
			continue
		}
		chunks = append(chunks, c)
	}
	return chunks, nil
}

// lexicalRetrieve finds chunks via Postgres full-text search (content_tsv, added in
// V220), ranked by ts_rank. It reuses the already-computed query embedding so every
// returned row also carries its genuine cosine similarity — the same SELECT shape
// as denseRetrieve, just a different WHERE/ORDER BY — which is what lets fuseRRF's
// output keep an honest Similarity value no matter which leg found a chunk.
func (r *RAGService) lexicalRetrieve(ctx context.Context, query, embeddingStr string, limit int, filterSourceTypes []string) ([]RetrievedChunk, error) {
	if strings.TrimSpace(query) == "" {
		return nil, nil
	}

	var (
		rows     *sql.Rows
		queryErr error
	)
	if len(filterSourceTypes) > 0 {
		placeholders := make([]string, len(filterSourceTypes))
		args := make([]interface{}, 0, len(filterSourceTypes)+3)
		args = append(args, embeddingStr, query, limit)
		for i, st := range filterSourceTypes {
			placeholders[i] = fmt.Sprintf("$%d", i+4)
			args = append(args, st)
		}
		sql := fmt.Sprintf(`
			SELECT id, source_type, COALESCE(source_ref,''), content,
			       1 - (embedding <=> $1::vector) AS similarity
			FROM ai_knowledge_chunks
			WHERE content_tsv @@ plainto_tsquery('english', $2)
			  AND source_type IN (%s)
			ORDER BY ts_rank(content_tsv, plainto_tsquery('english', $2)) DESC
			LIMIT $3
		`, strings.Join(placeholders, ","))
		rows, queryErr = r.db.QueryContext(ctx, sql, args...)
	} else {
		rows, queryErr = r.db.QueryContext(ctx, `
			SELECT id, source_type, COALESCE(source_ref,''), content,
			       1 - (embedding <=> $1::vector) AS similarity
			FROM ai_knowledge_chunks
			WHERE content_tsv @@ plainto_tsquery('english', $2)
			ORDER BY ts_rank(content_tsv, plainto_tsquery('english', $2)) DESC
			LIMIT $3
		`, embeddingStr, query, limit)
	}
	if queryErr != nil {
		return nil, fmt.Errorf("lexical search: %w", queryErr)
	}
	defer rows.Close()

	var chunks []RetrievedChunk
	for rows.Next() {
		var c RetrievedChunk
		if err := rows.Scan(&c.ID, &c.SourceType, &c.SourceRef, &c.Content, &c.Similarity); err != nil {
			continue
		}
		chunks = append(chunks, c)
	}
	return chunks, nil
}

// fuseRRF merges two independently-ranked chunk lists (dense vector search and
// lexical full-text search) into one ranking via Reciprocal Rank Fusion: a chunk's
// fused score is the sum of 1/(rrfK + rank) across every list it appears in (rank
// is 0-based here, so +1 below). RRF only uses rank position, never the lists' raw
// scores, so it never needs cosine similarity and ts_rank to be on the same scale.
//
// Each returned chunk keeps its own Similarity value exactly as denseRetrieve or
// lexicalRetrieve set it — fusion only changes ordering, never what Similarity
// means, so isLowConfidence (guardrails.go) keeps working on a genuine cosine
// similarity with zero changes to its calibration.
func fuseRRF(dense, lexical []RetrievedChunk, topK int) []RetrievedChunk {
	scores := make(map[string]float64)
	byID := make(map[string]RetrievedChunk)
	order := make([]string, 0, len(dense)+len(lexical))

	add := func(list []RetrievedChunk) {
		for rank, c := range list {
			if c.ID == "" {
				continue // can't fuse a chunk we can't identify
			}
			if _, seen := byID[c.ID]; !seen {
				order = append(order, c.ID)
				byID[c.ID] = c
			}
			scores[c.ID] += 1.0 / float64(rrfK+rank+1)
		}
	}
	add(dense)
	add(lexical)

	sort.SliceStable(order, func(i, j int) bool {
		return scores[order[i]] > scores[order[j]]
	})

	if topK <= 0 || topK > len(order) {
		topK = len(order)
	}
	fused := make([]RetrievedChunk, 0, topK)
	for _, id := range order[:topK] {
		fused = append(fused, byID[id])
	}
	return fused
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
