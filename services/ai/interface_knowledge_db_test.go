package ai

// Integration test against a real database (needs DATABASE_URL; skipped
// otherwise). Regression guard for 2026-10-19: both interface queries used
// COALESCE(source_connectivity, '') on a JSONB column, which Postgres rejects
// for every row ('' is not valid JSON) — so no interface was ever ingested
// into ezCompanion's knowledge and the "current interface" context was always
// empty. A mocked driver cannot catch this; only real Postgres can.

import (
	"context"
	"database/sql"
	"os"
	"strings"
	"testing"

	_ "github.com/lib/pq"
)

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		t.Skip("DATABASE_URL not set — skipping real-database test")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func anyInterfaceID(t *testing.T, db *sql.DB) string {
	t.Helper()
	var id string
	err := db.QueryRow(`SELECT id::text FROM interfaces ORDER BY (source_connectivity IS NOT NULL) DESC LIMIT 1`).Scan(&id)
	if err == sql.ErrNoRows {
		t.Skip("no interfaces in this database")
	}
	if err != nil {
		t.Fatalf("pick interface: %v", err)
	}
	return id
}

func TestOperationalIngestion_InterfaceQueryRunsOnRealSchema(t *testing.T) {
	db := openTestDB(t)
	// EmbeddingService without a DB stores nothing — this exercises only the
	// interface query and text rendering.
	o := &OperationalIngestionService{db: db, embedding: &EmbeddingService{}}
	if err := o.IngestInterface(context.Background(), anyInterfaceID(t, db)); err != nil {
		t.Fatalf("IngestInterface failed on the real schema: %v", err)
	}
}

func TestContextBuilder_InterfaceContextRunsOnRealSchema(t *testing.T) {
	db := openTestDB(t)
	out := (&ContextBuilder{db: db}).fetchInterfaceContext(context.Background(), anyInterfaceID(t, db))
	if !strings.Contains(out, "Interface:") {
		t.Fatalf("interface context is empty — the query failed on the real schema (got %q)", out)
	}
	if strings.Contains(out, "ENC:v1:") || strings.Contains(out, "password") {
		t.Fatalf("interface context must not expose connector config: %q", out)
	}
}

// Regression guard: a source-type-filtered vector search must find the
// filtered rows even when they are a small minority of the table. Without
// pgvector's iterative HNSW scan the documentation search returned 0 rows on
// a knowledge base dominated by operational chunks (2026-10-19).
func TestDenseRetrieve_FilteredMinorityTypesStillFound(t *testing.T) {
	db := openTestDB(t)
	var docs int
	if err := db.QueryRow(`SELECT count(*) FROM ai_knowledge_chunks WHERE source_type IN ('app_docs','pipeline_step_docs')`).Scan(&docs); err != nil || docs < 2 {
		t.Skip("no documentation chunks in this database")
	}
	var probe string
	if err := db.QueryRow(`SELECT embedding::text FROM ai_knowledge_chunks WHERE source_type NOT IN ('app_docs','pipeline_step_docs') LIMIT 1`).Scan(&probe); err != nil {
		t.Skip("no non-documentation chunk to probe with")
	}
	r := &RAGService{db: db}
	got, err := r.denseRetrieve(context.Background(), probe, 2, []string{"app_docs", "pipeline_step_docs"})
	if err != nil {
		t.Fatalf("denseRetrieve: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("filtered search returned %d rows, want 2", len(got))
	}
	for _, c := range got {
		if c.SourceType != "app_docs" && c.SourceType != "pipeline_step_docs" {
			t.Errorf("filter leaked source type %q", c.SourceType)
		}
	}
}
