package ai

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

type memSourceStore struct{ fp map[string]string }

func (m *memSourceStore) Fingerprint(_ context.Context, key string) (string, bool, error) {
	v, ok := m.fp[key]
	return v, ok, nil
}
func (m *memSourceStore) Record(_ context.Context, key, fp string, _ IngestionResult) error {
	m.fp[key] = fp
	return nil
}

func countingSource(key string, fp *string, runs *int, result IngestionResult) KnowledgeSource {
	return KnowledgeSource{
		Key:         key,
		Fingerprint: func() (string, error) { return *fp, nil },
		Ingest: func(context.Context) *IngestionResult {
			*runs++
			r := result
			return &r
		},
	}
}

func TestIngestSources_OnlyChangedSkipsUnchanged(t *testing.T) {
	store := &memSourceStore{fp: map[string]string{}}
	k := &KnowledgeIngestionService{store: store}
	fp, runs := "v1", 0
	src := []KnowledgeSource{countingSource("docs", &fp, &runs, IngestionResult{FilesScanned: 3, ChunksStored: 9})}

	k.IngestSources(context.Background(), src, true) // first boot: no record yet
	k.IngestSources(context.Background(), src, true) // unchanged restart
	if runs != 1 {
		t.Fatalf("unchanged source re-ingested: runs=%d, want 1", runs)
	}
	fp = "v2" // upgrade changed the docs
	k.IngestSources(context.Background(), src, true)
	if runs != 2 {
		t.Fatalf("changed source not re-ingested: runs=%d, want 2", runs)
	}
	k.IngestSources(context.Background(), src, false) // admin full re-ingest ignores fingerprints
	if runs != 3 {
		t.Fatalf("full ingest must run regardless: runs=%d", runs)
	}
}

func TestIngestSources_UnhealthyRunIsRetried(t *testing.T) {
	store := &memSourceStore{fp: map[string]string{}}
	k := &KnowledgeIngestionService{store: store}
	fp, runs := "v1", 0
	bad := IngestionResult{FilesScanned: 10, Errors: []string{"embed down", "embed down"}}
	src := []KnowledgeSource{countingSource("schemas", &fp, &runs, bad)}

	k.IngestSources(context.Background(), src, true)
	k.IngestSources(context.Background(), src, true)
	if runs != 2 {
		t.Fatalf("a failed ingestion must not be recorded as current: runs=%d, want 2", runs)
	}
	if _, found := store.fp["schemas"]; found {
		t.Fatal("unhealthy result was recorded")
	}
}

func TestIngestSources_NoStoreAlwaysRuns(t *testing.T) {
	k := &KnowledgeIngestionService{}
	fp, runs := "v1", 0
	src := []KnowledgeSource{countingSource("x", &fp, &runs, IngestionResult{FilesScanned: 1})}
	k.IngestSources(context.Background(), src, true)
	k.IngestSources(context.Background(), src, true)
	if runs != 2 {
		t.Fatalf("without a store every run must ingest, runs=%d", runs)
	}
}

func TestDirFingerprint_TracksContentAndFilter(t *testing.T) {
	dir := t.TempDir()
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a.md", "hello")
	write("ignored.gz", "binary")
	fp := dirFingerprint(dir, ".md")

	first, _ := fp()
	write("ignored.gz", "changed binary")
	if second, _ := fp(); second != first {
		t.Error("a file outside the extension filter must not change the fingerprint")
	}
	write("a.md", "hello, edited")
	if third, _ := fp(); third == first {
		t.Error("editing an ingested file must change the fingerprint")
	}

	absent, err := dirFingerprint(filepath.Join(dir, "missing"), ".md")()
	if err != nil || absent == "" {
		t.Errorf("missing dir should fingerprint as absent, got %q, %v", absent, err)
	}
}

func TestSources_KeysUniqueAndDocsMarked(t *testing.T) {
	k := &KnowledgeIngestionService{}
	seen := map[string]bool{}
	docs := 0
	for _, s := range k.Sources("schemas", ".") {
		if seen[s.Key] {
			t.Errorf("duplicate source key %q", s.Key)
		}
		seen[s.Key] = true
		if s.Fingerprint == nil || s.Ingest == nil {
			t.Errorf("source %q missing fingerprint or ingest", s.Key)
		}
		if s.Docs {
			docs++
		}
	}
	for _, key := range []string{"hl7_v2", "fhir_r4", "ccd:schema", "edi_x12_schema:schema", "app_docs:docs", "pipeline_step_docs"} {
		if !seen[key] {
			t.Errorf("expected source %q", key)
		}
	}
	if docs != len(appDocDirs)+3 {
		t.Errorf("docs sources = %d, want %d", docs, len(appDocDirs)+3)
	}
}
