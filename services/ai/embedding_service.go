// services/ai/embedding_service.go
// Chunks text and stores embeddings in PostgreSQL via pgvector.
package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/lib/pq"
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
	var chunkErrs []string

	// A single bad chunk (rare — e.g. malformed input that still slipped past
	// chunkText's rune-boundary safety) must not cost every chunk after it: skip
	// and record the failure, then keep going, so one bad paragraph doesn't
	// silently erase the rest of the document from the knowledge base.
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
			chunkErrs = append(chunkErrs, fmt.Sprintf("chunk %d embed: %v", i, err))
			continue
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
			chunkErrs = append(chunkErrs, fmt.Sprintf("chunk %d insert: %v", i, err))
			continue
		}
		stored++
	}

	if len(chunkErrs) > 0 {
		return stored, fmt.Errorf("%d of %d chunks failed: %s", len(chunkErrs), len(chunks), strings.Join(chunkErrs, "; "))
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

// ClearSourceRefs deletes only the chunks matching the given source_type AND
// one of the given source_refs — unlike ClearSourceType, this never touches
// other refs sharing the same source_type. Several sub-ingestions (e.g. the
// architecture-docs walk, the connectivity-docs walk, and the builtin
// app-knowledge list) all write under the shared "app_docs" source_type; a
// blanket ClearSourceType("app_docs") in any one of them would silently wipe
// out chunks another one just inserted moments earlier in the same request.
func (s *EmbeddingService) ClearSourceRefs(ctx context.Context, sourceType string, refs []string) error {
	if s.db == nil || len(refs) == 0 {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM ai_knowledge_chunks WHERE source_type = $1 AND source_ref = ANY($2)`,
		sourceType, pq.Array(refs))
	return err
}

// ClearSourceRefPrefix deletes only the chunks whose source_ref starts with
// the given prefix (e.g. "arch:" or "connectivity:"), scoped to one
// source_type. Same self-scoping purpose as ClearSourceRefs, for callers that
// re-ingest a directory of files up front rather than one known, enumerable
// list — used to clear a doc-walk's own previous chunks before re-ingesting,
// so a renamed/removed file's old chunks don't linger, and a re-run doesn't
// pile up duplicates of an unchanged file's chunks.
func (s *EmbeddingService) ClearSourceRefPrefix(ctx context.Context, sourceType, refPrefix string) error {
	if s.db == nil {
		return nil
	}
	_, err := s.db.ExecContext(ctx,
		`DELETE FROM ai_knowledge_chunks WHERE source_type = $1 AND source_ref LIKE $2`,
		sourceType, refPrefix+":%")
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

// chunkText splits text into overlapping chunks of at most `size` bytes.
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
		// text[start:end] is a byte-offset slice, but any character outside
		// plain ASCII (em dashes, curly quotes, checkmarks, box-drawing —
		// all common in this codebase's docs) is encoded as 2-4 bytes in
		// UTF-8. Without this, a chunk boundary landing mid-character
		// produces a byte sequence that is invalid UTF-8 on its own even
		// though the source text is fine, which Postgres then rejects on
		// insert — snapping backward to the nearest rune boundary keeps
		// every chunk valid UTF-8 no matter where the cut falls.
		end = snapToRuneBoundary(text, end)
		chunks = append(chunks, text[start:end])
		next := snapToRuneBoundary(text, end-overlap)
		if next <= start {
			break
		}
		start = next
	}
	return chunks
}

// snapToRuneBoundary walks i backward until it lands on a UTF-8 rune
// boundary, so a byte-offset cut never splits a multi-byte character in half.
func snapToRuneBoundary(text string, i int) int {
	for i > 0 && i < len(text) && !utf8.RuneStart(text[i]) {
		i--
	}
	return i
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
