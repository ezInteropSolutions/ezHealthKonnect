// services/ai/knowledge_sources.go
//
// Per-source change detection for ezCompanion's static knowledge (schemas,
// docs, step docs, built-in overviews).
//
// Before 2026-10-19 startup skipped ALL static ingestion whenever any static
// chunk existed ("restart optimisation"), so knowledge only loaded on a fresh
// install's first boot: an upgraded install kept the previous release's
// knowledge forever unless an admin re-ingested by hand. Now every static
// source has a content fingerprint; at startup only sources whose fingerprint
// differs from the one recorded at their last successful ingestion are
// re-ingested. An unchanged release costs one fingerprint pass; a release that
// edits one doc re-embeds only that source.
//
// Adding a knowledge source is one entry in Sources().
package ai

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// knowledgeFormatVersion is mixed into every fingerprint. Bump it when the
// way sources are turned into chunks changes (chunk size, prefix text), so
// existing installs re-ingest with the new format.
const knowledgeFormatVersion = "2026-10-19.1"

// KnowledgeSource is one independently re-ingestable unit of static knowledge.
type KnowledgeSource struct {
	// Key identifies the source in ai_knowledge_sources (stable across releases).
	Key string
	// Docs marks documentation sources (re-ingested by POST /api/ai/ingest/docs).
	Docs bool
	// Fingerprint summarises the source's current content.
	Fingerprint func() (string, error)
	// Ingest replaces the source's chunks. A nil result means the source is
	// absent on this install and was skipped.
	Ingest func(ctx context.Context) *IngestionResult
}

// KnowledgeSourceStore records the fingerprint each source was last ingested at.
type KnowledgeSourceStore interface {
	Fingerprint(ctx context.Context, key string) (fingerprint string, found bool, err error)
	Record(ctx context.Context, key, fingerprint string, result IngestionResult) error
}

// Sources lists every static knowledge source. schemaDir holds the HL7/FHIR
// dictionaries; rootDir is the app's working directory.
func (k *KnowledgeIngestionService) Sources(schemaDir, rootDir string) []KnowledgeSource {
	sources := []KnowledgeSource{
		{Key: "x12:builtin", Fingerprint: valueFingerprint(x12KnowledgeBase), Ingest: k.IngestX12BuiltinKnowledge},
		{Key: "ccd:builtin", Fingerprint: valueFingerprint(ccdKnowledgeBase), Ingest: k.IngestCCDBuiltinKnowledge},
		{
			Key:         "ccd:schema",
			Fingerprint: dirFingerprint(filepath.Join(rootDir, "cda", "schemas"), ".json"),
			Ingest: func(ctx context.Context) *IngestionResult {
				return k.IngestCDASchemas(ctx, filepath.Join(rootDir, "cda", "schemas"))
			},
		},
	}

	for _, src := range standardSchemaSources {
		src := src
		sources = append(sources, KnowledgeSource{
			Key:         src.SourceType + ":schema",
			Fingerprint: dirFingerprint(filepath.Join(rootDir, src.Dir), ".json"),
			Ingest: func(ctx context.Context) *IngestionResult {
				return k.ingestStandardSchema(ctx, rootDir, src)
			},
		})
	}

	for _, d := range appDocDirs {
		d := d
		dir := filepath.Join(rootDir, d.Dir)
		sources = append(sources, KnowledgeSource{
			Key:         "app_docs:" + d.RefPrefix,
			Docs:        true,
			Fingerprint: dirFingerprint(dir, ".md"),
			Ingest: func(ctx context.Context) *IngestionResult {
				return k.ingestDocDir(ctx, dir, "app_docs", d.RefPrefix)
			},
		})
	}

	systemDoc := filepath.Join(rootDir, "SYSTEM_DOCUMENTATION.md")
	stepDocs := filepath.Join(rootDir, "architecture", "generated", "pipeline_step_docs.json")
	sources = append(sources,
		KnowledgeSource{
			Key: "app_docs:system_docs", Docs: true,
			Fingerprint: fileFingerprint(systemDoc),
			Ingest: func(ctx context.Context) *IngestionResult {
				return k.ingestSingleDoc(ctx, systemDoc, "app_docs", "system_docs")
			},
		},
		KnowledgeSource{Key: "app_docs:builtin", Docs: true, Fingerprint: valueFingerprint(appBuiltinKnowledge), Ingest: k.IngestBuiltinAppKnowledge},
		KnowledgeSource{
			Key: "pipeline_step_docs", Docs: true,
			Fingerprint: fileFingerprint(stepDocs),
			Ingest: func(ctx context.Context) *IngestionResult {
				return k.IngestPipelineStepDocs(ctx, stepDocs)
			},
		},
	)

	// Schema digests last: the HL7 one reads ~130 MB of compressed schema, so
	// documentation refreshes first on a startup that has both to do.
	for _, src := range schemaDigestSources {
		src := src
		sources = append(sources, KnowledgeSource{
			Key:         src.Key,
			Fingerprint: dirFingerprint(filepath.Join(schemaDir, src.Dir), schemaDigestExts...),
			Ingest: func(ctx context.Context) *IngestionResult {
				return k.ingestSchemaDigest(ctx, schemaDir, src)
			},
		})
	}
	return sources
}

// appDocDir is one documentation directory walked for *.md files.
type appDocDir struct {
	Dir       string // relative to rootDir
	RefPrefix string // source_ref prefix; also the source key suffix
}

// appDocDirs drives the documentation walks. connectivity/ is excluded from
// the Docker image (.dockerignore); it is still walked when present.
var appDocDirs = []appDocDir{
	{Dir: "architecture", RefPrefix: "arch"},
	{Dir: "connectivity", RefPrefix: "connectivity"},
	{Dir: "docs", RefPrefix: "docs"},
	{Dir: "installer", RefPrefix: "installer"},
}

// IngestSources ingests the given sources. With onlyChanged, a source whose
// current fingerprint equals its recorded one is skipped. Every successful
// ingestion records its fingerprint (when a store is configured).
func (k *KnowledgeIngestionService) IngestSources(ctx context.Context, sources []KnowledgeSource, onlyChanged bool) []IngestionResult {
	var results []IngestionResult
	for _, src := range sources {
		if ctx.Err() != nil {
			break
		}
		fp, fpErr := src.Fingerprint()
		if fpErr != nil {
			log.Printf("⚠️  AI KB — %s: fingerprint failed (%v); re-ingesting", src.Key, fpErr)
			fp = ""
		}
		if onlyChanged && fp != "" && k.store != nil {
			if recorded, found, err := k.store.Fingerprint(ctx, src.Key); err == nil && found && recorded == fp {
				continue
			}
		}

		r := src.Ingest(ctx)
		if r == nil {
			continue
		}
		results = append(results, *r)
		if k.store != nil && fp != "" && ingestionHealthy(*r) {
			if err := k.store.Record(ctx, src.Key, fp, *r); err != nil {
				log.Printf("⚠️  AI KB — %s: could not record fingerprint: %v", src.Key, err)
			}
		}
	}
	return results
}

// IngestChanged re-ingests only the static sources whose content changed
// since their last successful ingestion — the startup path.
func (k *KnowledgeIngestionService) IngestChanged(ctx context.Context, schemaDir string) []IngestionResult {
	return k.IngestSources(ctx, k.Sources(schemaDir, "."), true)
}

// ingestionHealthy decides whether a run is good enough to record as done.
// An unhealthy run (e.g. the embedding model was down) is retried at the next
// startup instead of being remembered as current.
func ingestionHealthy(r IngestionResult) bool {
	if r.FilesScanned == 0 {
		return len(r.Errors) == 0
	}
	return len(r.Errors)*20 <= r.FilesScanned // at most 5% of files failed
}

// ─── Fingerprints ────────────────────────────────────────────────────────────

func newFingerprintHash() hash.Hash {
	h := sha256.New()
	io.WriteString(h, knowledgeFormatVersion+"\x00")
	return h
}

// dirFingerprint hashes the relative path and content of every file under
// dir with one of exts — exactly the files the matching ingestion reads. A
// missing directory fingerprints as "absent" (ingestion then skips it).
func dirFingerprint(dir string, exts ...string) func() (string, error) {
	return func() (string, error) {
		h := newFingerprintHash()
		if _, err := os.Stat(dir); os.IsNotExist(err) {
			io.WriteString(h, "absent")
			return hex.EncodeToString(h.Sum(nil)), nil
		}
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !hasExt(path, exts) {
				return nil
			}
			rel, _ := filepath.Rel(dir, path)
			io.WriteString(h, filepath.ToSlash(rel)+"\x00")
			return hashFileInto(h, path)
		})
		if err != nil {
			return "", err
		}
		return hex.EncodeToString(h.Sum(nil)), nil
	}
}

// fileFingerprint hashes one file's content ("absent" when missing).
func fileFingerprint(path string) func() (string, error) {
	return func() (string, error) {
		h := newFingerprintHash()
		if _, err := os.Stat(path); os.IsNotExist(err) {
			io.WriteString(h, "absent")
		} else if err := hashFileInto(h, path); err != nil {
			return "", err
		}
		return hex.EncodeToString(h.Sum(nil)), nil
	}
}

// valueFingerprint hashes compiled-in knowledge (changes with the binary).
func valueFingerprint(v interface{}) func() (string, error) {
	return func() (string, error) {
		h := newFingerprintHash()
		fmt.Fprintf(h, "%#v", v)
		return hex.EncodeToString(h.Sum(nil)), nil
	}
}

func hashFileInto(h hash.Hash, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(h, f)
	return err
}

func hasExt(path string, exts []string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	for _, e := range exts {
		if ext == e {
			return true
		}
	}
	return false
}

// ─── Postgres store ──────────────────────────────────────────────────────────

// PostgresKnowledgeSourceStore persists fingerprints in ai_knowledge_sources (V280).
type PostgresKnowledgeSourceStore struct {
	db *sql.DB
}

// NewPostgresKnowledgeSourceStore returns a store, or nil when db is nil.
func NewPostgresKnowledgeSourceStore(db *sql.DB) KnowledgeSourceStore {
	if db == nil {
		return nil
	}
	return &PostgresKnowledgeSourceStore{db: db}
}

// Fingerprint returns the recorded fingerprint for key.
func (s *PostgresKnowledgeSourceStore) Fingerprint(ctx context.Context, key string) (string, bool, error) {
	var fp string
	err := s.db.QueryRowContext(ctx, `SELECT fingerprint FROM ai_knowledge_sources WHERE source_key = $1`, key).Scan(&fp)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return fp, true, nil
}

// Record upserts the fingerprint and outcome of a successful ingestion.
func (s *PostgresKnowledgeSourceStore) Record(ctx context.Context, key, fingerprint string, r IngestionResult) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO ai_knowledge_sources (source_key, fingerprint, files_scanned, chunks_stored, error_count, ingested_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (source_key) DO UPDATE SET
			fingerprint = EXCLUDED.fingerprint,
			files_scanned = EXCLUDED.files_scanned,
			chunks_stored = EXCLUDED.chunks_stored,
			error_count = EXCLUDED.error_count,
			ingested_at = EXCLUDED.ingested_at`,
		key, fingerprint, r.FilesScanned, r.ChunksStored, len(r.Errors), time.Now())
	return err
}
