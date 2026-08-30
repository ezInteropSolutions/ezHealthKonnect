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
	if r := k.IngestCDASchemas(ctx, filepath.Join(".", "cda", "schemas")); r != nil {
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
		result.ChunksStored += n // count whatever succeeded even if some chunks in this file failed
		if embedErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s embed: %v", path, embedErr))
		}
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
		result.ChunksStored += n // count whatever succeeded even if some chunks in this file failed
		if embedErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s embed: %v", path, embedErr))
		}
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
		result.ChunksStored += n // count whatever succeeded even if some chunks in this entry failed
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", kb.ref, err))
		}
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
	// docs/ (INSTALL_GUIDE.md, RUNBOOK.md) and installer/ (ONE_CLICK_INSTALLER_GUIDE.md)
	// — walked the same way as architecture/, so any future doc dropped in either
	// directory is picked up automatically rather than needing a new hardcoded path.
	if r := k.ingestDocDir(ctx, filepath.Join(rootDir, "docs"), "app_docs", "docs"); r != nil {
		results = append(results, *r)
	}
	if r := k.ingestDocDir(ctx, filepath.Join(rootDir, "installer"), "app_docs", "installer"); r != nil {
		results = append(results, *r)
	}
	// SYSTEM_DOCUMENTATION.md is CLAUDE.md's own "master reference" doc, but it
	// lives at the project root next to files that should NOT be swept in
	// (README.md, STANDARDS.md, COMMERCIAL_LICENSE.md, CLAUDE.md itself) — a
	// single named file, not a directory walk.
	if r := k.ingestSingleDoc(ctx, filepath.Join(rootDir, "SYSTEM_DOCUMENTATION.md"), "app_docs", "system_docs"); r != nil {
		results = append(results, *r)
	}
	if r := k.IngestBuiltinAppKnowledge(ctx); r != nil {
		results = append(results, *r)
	}
	if r := k.IngestPipelineStepDocs(ctx, filepath.Join(rootDir, "architecture", "generated", "pipeline_step_docs.json")); r != nil {
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

	// Scoped to this walk's own ref prefix — NOT ClearSourceType(sourceType).
	// Other sub-ingestions (e.g. IngestBuiltinAppKnowledge) write under this
	// same shared sourceType too; clearing only "refPrefix:*" rows means a
	// renamed/removed file's old chunks don't linger and a re-run doesn't
	// duplicate an unchanged file's chunks, without touching anyone else's rows.
	if err := k.embedding.ClearSourceRefPrefix(ctx, sourceType, refPrefix); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("clear %s: %v", refPrefix, err))
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
		result.ChunksStored += n // count whatever succeeded even if some chunks in this file failed
		if embedErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s embed: %v", path, embedErr))
		}
		return nil
	})

	log.Printf("✅ AI KB — %s/%s: %d files, %d chunks, %d errors",
		sourceType, refPrefix, result.FilesScanned, result.ChunksStored, len(result.Errors))
	return result
}

// ingestSingleDoc ingests one specific, individually-named markdown file
// rather than every .md file under a directory — for a well-known root-level
// reference doc (SYSTEM_DOCUMENTATION.md) that sits next to files which
// should NOT be swept in wholesale (README.md, CLAUDE.md, license/legal docs).
func (k *KnowledgeIngestionService) ingestSingleDoc(ctx context.Context, path, sourceType, refPrefix string) *IngestionResult {
	result := &IngestionResult{SourceType: sourceType + ":" + refPrefix}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil // file absent — skip silently
	}

	text, ref, readErr := readSchemaFile(path)
	if readErr != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", path, readErr))
		return result
	}
	ref = refPrefix + ":" + ref

	if err := k.embedding.ClearSourceRefs(ctx, sourceType, []string{ref}); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("clear %s: %v", ref, err))
	}

	result.FilesScanned++
	n, embedErr := k.embedding.IngestText(ctx, sourceType, ref, path, text, nil)
	result.ChunksStored += n
	if embedErr != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("%s embed: %v", path, embedErr))
	}

	log.Printf("✅ AI KB — %s/%s: %d files, %d chunks, %d errors",
		sourceType, refPrefix, result.FilesScanned, result.ChunksStored, len(result.Errors))
	return result
}

// IngestBuiltinAppKnowledge ingests static knowledge about ezHealthKonnect's own
// pipeline step types, connector configs, and app behaviour.
// This lets ezCompanion answer "how do I..." questions accurately without exposing source code.
func (k *KnowledgeIngestionService) IngestBuiltinAppKnowledge(ctx context.Context) *IngestionResult {
	result := &IngestionResult{SourceType: "app_docs"}

	// Scoped to exactly this function's own refs — NOT ClearSourceType("app_docs").
	// ingestDocDir's architecture/connectivity doc walks write under the same
	// "app_docs" source_type just before this runs (see IngestAppDocs); a
	// blanket clear here would silently delete their fresh inserts within the
	// same request, before it ever returns.
	builtinRefs := make([]string, len(appBuiltinKnowledge))
	for i, entry := range appBuiltinKnowledge {
		builtinRefs[i] = entry.ref
	}
	if err := k.embedding.ClearSourceRefs(ctx, "app_docs", builtinRefs); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("clear: %v", err))
	}

	for _, entry := range appBuiltinKnowledge {
		result.FilesScanned++
		n, err := k.embedding.IngestText(ctx, "app_docs", entry.ref, "builtin:app", entry.content, nil)
		result.ChunksStored += n // count whatever succeeded even if some chunks in this entry failed
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", entry.ref, err))
		}
	}

	log.Printf("✅ AI KB — App Docs (builtin): %d entries, %d chunks, %d errors",
		result.FilesScanned, result.ChunksStored, len(result.Errors))
	return result
}

// IngestCCDBuiltinKnowledge ingests static, hand-written CCD/CDA narrative
// overviews (what CCD is, section LOINC codes, common XML shapes) — content a
// raw schema file doesn't carry in prose form. IngestCDASchemas below adds the
// real, authoritative schema data on top of this; the two share the "ccd"
// source_type (both are legitimately "CCD/CDA knowledge") but clear/re-ingest
// against different ref prefixes so neither wipes the other's rows regardless
// of call order — same scoping pattern IngestBuiltinAppKnowledge uses.
func (k *KnowledgeIngestionService) IngestCCDBuiltinKnowledge(ctx context.Context) *IngestionResult {
	result := &IngestionResult{SourceType: "ccd"}

	staticRefs := make([]string, len(ccdKnowledgeBase))
	for i, kb := range ccdKnowledgeBase {
		staticRefs[i] = kb.ref
	}
	if err := k.embedding.ClearSourceRefs(ctx, "ccd", staticRefs); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("clear: %v", err))
	}

	for _, kb := range ccdKnowledgeBase {
		result.FilesScanned++
		n, err := k.embedding.IngestText(ctx, "ccd", kb.ref, "builtin:ccd", kb.content, nil)
		result.ChunksStored += n // count whatever succeeded even if some chunks in this entry failed
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", kb.ref, err))
		}
	}

	log.Printf("✅ AI KB — CCD: %d entries, %d chunks, %d errors",
		result.FilesScanned, result.ChunksStored, len(result.Errors))
	return result
}

// IngestCDASchemas walks the real, authoritative C-CDA schema tree
// (cda/schemas/ — manifest.json, sections/*.json, entries/*.json, plus
// uscdi_v3.json and c32_mapping.json) and ingests it the same way
// IngestHL7Schemas/IngestFHIRSchemas ingest their schema dictionaries. This is
// the exact data cda/builder, cda/document, and the CDA validator run on at
// runtime — unlike IngestCCDBuiltinKnowledge's small hand-written overview,
// this stays accurate automatically as the schema evolves (a section edit is
// picked up on the next re-ingest) because it reads the schema files directly
// instead of summarizing them by hand.
//
// Scoped to the "schema:" ref prefix (see IngestCCDBuiltinKnowledge's doc
// comment for why) so re-running either ingestion never clears the other's rows.
func (k *KnowledgeIngestionService) IngestCDASchemas(ctx context.Context, dir string) *IngestionResult {
	result := &IngestionResult{SourceType: "ccd"}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		result.Errors = append(result.Errors, fmt.Sprintf("directory not found: %s", dir))
		return result
	}
	if err := k.embedding.ClearSourceRefPrefix(ctx, "ccd", "schema"); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("clear: %v", err))
	}

	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if strings.ToLower(filepath.Ext(path)) != ".json" {
			return nil
		}
		result.FilesScanned++
		text, ref, readErr := readSchemaFile(path)
		if readErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", path, readErr))
			return nil
		}
		ref = "schema:" + ref
		n, embedErr := k.embedding.IngestText(ctx, "ccd", ref, path, text, nil)
		result.ChunksStored += n // count whatever succeeded even if some chunks in this file failed
		if embedErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s embed: %v", path, embedErr))
		}
		return nil
	})

	log.Printf("✅ AI KB — CDA Schema: %d files, %d chunks, %d errors",
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
