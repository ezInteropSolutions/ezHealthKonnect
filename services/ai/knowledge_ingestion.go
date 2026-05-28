// services/ai/knowledge_ingestion.go
// Walks local schema files (HL7, FHIR) and built-in X12/CCD definitions,
// chunks them, embeds them, and stores them in the ai_knowledge_chunks table.
package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// KnowledgeIngestionService rebuilds the AI knowledge base from schema files.
type KnowledgeIngestionService struct {
	embedding *EmbeddingService
}

// NewKnowledgeIngestionService creates a new KnowledgeIngestionService.
func NewKnowledgeIngestionService(embedding *EmbeddingService) *KnowledgeIngestionService {
	return &KnowledgeIngestionService{embedding: embedding}
}

// IngestionResult reports the outcome of ingesting one source type.
type IngestionResult struct {
	SourceType   string   `json:"source_type"`
	FilesScanned int      `json:"files_scanned"`
	ChunksStored int      `json:"chunks_stored"`
	Errors       []string `json:"errors,omitempty"`
}

// IngestAll rebuilds the entire knowledge base from all available sources.
func (k *KnowledgeIngestionService) IngestAll(ctx context.Context, schemaDir string) []IngestionResult {
	var results []IngestionResult

	if r := k.IngestHL7Schemas(ctx, filepath.Join(schemaDir, "hl7")); r != nil {
		results = append(results, *r)
	}
	if r := k.IngestFHIRSchemas(ctx, filepath.Join(schemaDir, "fhir")); r != nil {
		results = append(results, *r)
	}
	if r := k.IngestX12BuiltinKnowledge(ctx); r != nil {
		results = append(results, *r)
	}
	if r := k.IngestCCDBuiltinKnowledge(ctx); r != nil {
		results = append(results, *r)
	}
	for _, r := range k.IngestAppDocs(ctx, ".") {
		results = append(results, r)
	}
	return results
}

// IngestHL7Schemas walks schemas/hl7/ and ingests all JSON/YAML/text files.
func (k *KnowledgeIngestionService) IngestHL7Schemas(ctx context.Context, dir string) *IngestionResult {
	result := &IngestionResult{SourceType: "hl7_v2"}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		result.Errors = append(result.Errors, fmt.Sprintf("directory not found: %s", dir))
		return result
	}
	if err := k.embedding.ClearSourceType(ctx, "hl7_v2"); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("clear: %v", err))
	}

	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".json" && ext != ".txt" && ext != ".yaml" && ext != ".yml" {
			return nil
		}

		result.FilesScanned++
		text, ref, readErr := readSchemaFile(path)
		if readErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", path, readErr))
			return nil
		}

		// Prepend segment name hint for 3-letter filenames (e.g. PID.json)
		base := strings.ToUpper(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)))
		if len(base) == 3 {
			text = fmt.Sprintf("HL7 v2 Segment: %s\n\n%s", base, text)
		}

		n, embedErr := k.embedding.IngestText(ctx, "hl7_v2", ref, path, text, nil)
		if embedErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s embed: %v", path, embedErr))
			return nil
		}
		result.ChunksStored += n
		return nil
	})

	log.Printf("✅ AI KB — HL7 v2: %d files, %d chunks, %d errors",
		result.FilesScanned, result.ChunksStored, len(result.Errors))
	return result
}

// IngestFHIRSchemas walks schemas/fhir/ and ingests all JSON files.
func (k *KnowledgeIngestionService) IngestFHIRSchemas(ctx context.Context, dir string) *IngestionResult {
	result := &IngestionResult{SourceType: "fhir_r4"}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		result.Errors = append(result.Errors, fmt.Sprintf("directory not found: %s", dir))
		return result
	}
	if err := k.embedding.ClearSourceType(ctx, "fhir_r4"); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("clear: %v", err))
	}

	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".json" && ext != ".txt" {
			return nil
		}

		result.FilesScanned++
		text, ref, readErr := readSchemaFile(path)
		if readErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", path, readErr))
			return nil
		}

		n, embedErr := k.embedding.IngestText(ctx, "fhir_r4", ref, path, text, nil)
		if embedErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s embed: %v", path, embedErr))
			return nil
		}
		result.ChunksStored += n
		return nil
	})

	log.Printf("✅ AI KB — FHIR R4: %d files, %d chunks, %d errors",
		result.FilesScanned, result.ChunksStored, len(result.Errors))
	return result
}

// IngestX12BuiltinKnowledge ingests static X12 835/837/270/271 definitions.
func (k *KnowledgeIngestionService) IngestX12BuiltinKnowledge(ctx context.Context) *IngestionResult {
	result := &IngestionResult{SourceType: "x12"}

	if err := k.embedding.ClearSourceType(ctx, "x12"); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("clear: %v", err))
	}

	for _, kb := range x12KnowledgeBase {
		result.FilesScanned++
		n, err := k.embedding.IngestText(ctx, "x12", kb.ref, "builtin:x12", kb.content,
			map[string]interface{}{"format": kb.format})
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", kb.ref, err))
			continue
		}
		result.ChunksStored += n
	}

	log.Printf("✅ AI KB — X12: %d entries, %d chunks, %d errors",
		result.FilesScanned, result.ChunksStored, len(result.Errors))
	return result
}

// IngestAppDocs ingests architecture and documentation files (*.md) from the project.
// ONLY documentation is ingested — no Go or JavaScript source files are included,
// so no implementation details are exposed to users through the AI assistant.
// rootDir should be the project root (e.g. "." when running inside the container).
func (k *KnowledgeIngestionService) IngestAppDocs(ctx context.Context, rootDir string) []IngestionResult {
	var results []IngestionResult
	if r := k.ingestDocDir(ctx, filepath.Join(rootDir, "architecture"), "app_docs", "arch"); r != nil {
		results = append(results, *r)
	}
	if r := k.ingestDocDir(ctx, filepath.Join(rootDir, "connectivity"), "app_docs", "connectivity"); r != nil {
		results = append(results, *r)
	}
	if r := k.IngestBuiltinAppKnowledge(ctx); r != nil {
		results = append(results, *r)
	}
	return results
}

// ingestDocDir walks a directory and ingests all markdown (.md) files.
func (k *KnowledgeIngestionService) ingestDocDir(ctx context.Context, dir, sourceType, refPrefix string) *IngestionResult {
	result := &IngestionResult{SourceType: sourceType + ":" + refPrefix}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil // directory absent — skip silently
	}

	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(path)) != ".md" {
			return nil
		}
		result.FilesScanned++
		text, ref, readErr := readSchemaFile(path)
		if readErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", path, readErr))
			return nil
		}
		ref = refPrefix + ":" + ref
		n, embedErr := k.embedding.IngestText(ctx, sourceType, ref, path, text, nil)
		if embedErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s embed: %v", path, embedErr))
			return nil
		}
		result.ChunksStored += n
		return nil
	})

	log.Printf("✅ AI KB — %s/%s: %d files, %d chunks, %d errors",
		sourceType, refPrefix, result.FilesScanned, result.ChunksStored, len(result.Errors))
	return result
}

// IngestBuiltinAppKnowledge ingests static knowledge about ezHealthKonnect's own
// pipeline step types, connector configs, and app behaviour.
// This lets ezCompanion answer "how do I..." questions accurately without exposing source code.
func (k *KnowledgeIngestionService) IngestBuiltinAppKnowledge(ctx context.Context) *IngestionResult {
	result := &IngestionResult{SourceType: "app_docs"}

	if err := k.embedding.ClearSourceType(ctx, "app_docs"); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("clear: %v", err))
	}

	for _, entry := range appBuiltinKnowledge {
		result.FilesScanned++
		n, err := k.embedding.IngestText(ctx, "app_docs", entry.ref, "builtin:app", entry.content, nil)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", entry.ref, err))
			continue
		}
		result.ChunksStored += n
	}

	log.Printf("✅ AI KB — App Docs (builtin): %d entries, %d chunks, %d errors",
		result.FilesScanned, result.ChunksStored, len(result.Errors))
	return result
}

// IngestCCDBuiltinKnowledge ingests static CCD/CDA section definitions.
func (k *KnowledgeIngestionService) IngestCCDBuiltinKnowledge(ctx context.Context) *IngestionResult {
	result := &IngestionResult{SourceType: "ccd"}

	if err := k.embedding.ClearSourceType(ctx, "ccd"); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("clear: %v", err))
	}

	for _, kb := range ccdKnowledgeBase {
		result.FilesScanned++
		n, err := k.embedding.IngestText(ctx, "ccd", kb.ref, "builtin:ccd", kb.content, nil)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", kb.ref, err))
			continue
		}
		result.ChunksStored += n
	}

	log.Printf("✅ AI KB — CCD: %d entries, %d chunks, %d errors",
		result.FilesScanned, result.ChunksStored, len(result.Errors))
	return result
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func readSchemaFile(path string) (text, ref string, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", "", err
	}
	defer f.Close()

	b, err := io.ReadAll(f)
	if err != nil {
		return "", "", err
	}

	// Pretty-print JSON so the LLM sees readable structure
	if strings.ToLower(filepath.Ext(path)) == ".json" {
		var parsed interface{}
		if jsonErr := json.Unmarshal(b, &parsed); jsonErr == nil {
			if pretty, prettyErr := json.MarshalIndent(parsed, "", "  "); prettyErr == nil {
				b = pretty
			}
		}
	}

	base := filepath.Base(path)
	ref = strings.TrimSuffix(base, filepath.Ext(base))
	return string(b), ref, nil
}
