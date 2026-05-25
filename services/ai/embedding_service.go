// services/ai/embedding_service.go
// Chunks text and stores embeddings in PostgreSQL via pgvector.
package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const (
	chunkSize    = 1500 // characters per chunk — fits in nomic-embed-text context window
	chunkOverlap = 200  // overlap preserves context across chunk boundaries
)

// EmbeddingService chunks text and stores embeddings in the ai_knowledge_chunks table.
type EmbeddingService struct {
	db      *sql.DB
	ollama  *OllamaClient // kept for backward compat
	embeddr LLMProvider   // provider used for embeddings
}

// NewEmbeddingService creates a new EmbeddingService using OllamaClient (backward compat).
func NewEmbeddingService(db *sql.DB, ollama *OllamaClient) *EmbeddingService {
	return &EmbeddingService{db: db, ollama: ollama, embeddr: ollama}
}

// newEmbeddingServiceWithProvider creates an EmbeddingService with an injected embed provider.
func newEmbeddingServiceWithProvider(db *sql.DB, embeddr LLMProvider) *EmbeddingService {
	return &EmbeddingService{db: db, embeddr: embeddr}
}

// IngestText chunks text, embeds each chunk, and stores them in PostgreSQL.
// Returns the number of chunks stored.
func (s *EmbeddingService) IngestText(ctx context.Context, sourceType, sourceRef, sourceFile, text string, metadata map[string]interface{}) (int, error) {
	if s.db == nil {
		return 0, nil
	}
	chunks := chunkText(text, chunkSize, chunkOverlap)
	stored := 0

	for i, chunk := range chunks {
		if strings.TrimSpace(chunk) == "" {
			continue
		}

		embedder := s.embeddr
		if embedder == nil {
			embedder = s.ollama
		}
		embedding, err := embedder.Embed(ctx, chunk)
		if err != nil {
			return stored, fmt.Errorf("chunk %d embed: %w", i, err)
		}

		meta := map[string]interface{}{
			"chunk_index":  i,
			"total_chunks": len(chunks),
		}
		for k, v := range metadata {
			meta[k] = v
		}
		metaJSON, _ := json.Marshal(meta)

		_, err = s.db.ExecContext(ctx, `
			INSERT INTO ai_knowledge_chunks (id, source_type, source_ref, source_file, content, embedding, metadata)
			VALUES ($1, $2, $3, $4, $5, $6::vector, $7)
		`, uuid.New().String(), sourceType, sourceRef, sourceFile, chunk,
			float32SliceToPGVector(embedding), metaJSON)
		if err != nil {
			return stored, fmt.Errorf("chunk %d insert: %w", i, err)
		}
		stored++
	}
	return stored, nil
}

// ClearSourceType deletes all chunks for a given source type (before re-ingestion).
func (s *EmbeddingService) ClearSourceType(ctx context.Context, sourceType string) error {
	if s.db == nil {
		return nil
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM ai_knowledge_chunks WHERE source_type = $1`, sourceType)
	return err
}

// CountChunks returns the total number of knowledge chunks stored.
func (s *EmbeddingService) CountChunks(ctx context.Context) (int, error) {
	if s.db == nil {
		return 0, nil
	}
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM ai_knowledge_chunks`).Scan(&count)
	return count, err
}

// CountBySourceType returns chunk counts keyed by source type.
func (s *EmbeddingService) CountBySourceType(ctx context.Context) (map[string]int, error) {
	if s.db == nil {
		return nil, nil
	}
	rows, err := s.db.QueryContext(ctx, `SELECT source_type, COUNT(*) FROM ai_knowledge_chunks GROUP BY source_type ORDER BY source_type`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]int)
	for rows.Next() {
		var st string
		var count int
		if err := rows.Scan(&st, &count); err != nil {
			continue
		}
		result[st] = count
	}
	return result, nil
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// chunkText splits text into overlapping chunks of at most `size` characters.
// It tries to break at newline boundaries to preserve semantic units.
func chunkText(text string, size, overlap int) []string {
	if len(text) <= size {
		return []string{text}
	}
	var chunks []string
	start := 0
	for start < len(text) {
		end := start + size
		if end > len(text) {
			end = len(text)
		}
		// Try to break at the last newline in the second half of the window
		if end < len(text) {
			if idx := strings.LastIndex(text[start:end], "\n"); idx > size/2 {
				end = start + idx + 1
			}
		}
		chunks = append(chunks, text[start:end])
		next := end - overlap
		if next <= start {
			break
		}
		start = next
	}
	return chunks
}

// float32SliceToPGVector converts a float32 slice to the pgvector literal '[1.0,2.0,...]'.
func float32SliceToPGVector(v []float32) string {
	var sb strings.Builder
	sb.WriteByte('[')
	for i, f := range v {
		if i > 0 {
			sb.WriteByte(',')
		}
		sb.WriteString(fmt.Sprintf("%g", f))
	}
	sb.WriteByte(']')
	return sb.String()
}
