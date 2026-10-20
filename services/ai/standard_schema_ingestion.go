// services/ai/standard_schema_ingestion.go
//
// Ingests the schema trees of the schema-driven message standards (EDI X12,
// NCPDP SCRIPT, NCPDP Telecommunication D.0, ASTM E1394) into the ezCompanion
// knowledge base, so it can answer "which segment/field carries X", "what does
// loop 2100 contain" or "which groups does a NewRx have" from the real schema
// the engine runs on — not from hand-written summaries that drift.
//
// One generic walk driven by the standardSchemaSources table: adding a new
// standard (or version) is one table row, never a new function. Before
// 2026-10-19 only HL7, FHIR and CDA schemas were ingested; X12 relied on a
// hand-written summary (builtin_knowledge.go) and NCPDP/D.0/ASTM had nothing.
package ai

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// StandardSchemaSource names one schema tree to ingest.
type StandardSchemaSource struct {
	SourceType string // knowledge-base source_type (retrieval filter key)
	Label      string // human-readable standard name, prefixed to every chunk
	Dir        string // schema root, relative to the app's working directory
}

// standardSchemaSources is the data that drives IngestStandardSchemas.
var standardSchemaSources = []StandardSchemaSource{
	{SourceType: "edi_x12_schema", Label: "EDI X12 005010 (835, 837P, 837I, 999, 270, 271, 276, 277, 278, 834)", Dir: filepath.Join("edi", "schemas", "x12_005010")},
	{SourceType: "ncpdp_script", Label: "NCPDP SCRIPT 2017071 (e-prescribing XML)", Dir: filepath.Join("ncpdp", "schemas", "script_2017071")},
	{SourceType: "ncpdp_telecom", Label: "NCPDP Telecommunication D.0 (pharmacy claims: B1, B2, B3, E1)", Dir: filepath.Join("ncpdptelecom", "schemas", "telecom_d0")},
	{SourceType: "astm", Label: "ASTM E1394-97 (lab analyzer records H/P/O/R/C/Q/L)", Dir: filepath.Join("astm", "schemas", "e1394_97")},
}

// IngestStandardSchemas ingests every configured standard's schema tree under
// rootDir. Each standard is replaced as a unit (its own "schema:" chunks are
// cleared first), so re-running never duplicates knowledge.
func (k *KnowledgeIngestionService) IngestStandardSchemas(ctx context.Context, rootDir string) []IngestionResult {
	results := make([]IngestionResult, 0, len(standardSchemaSources))
	for _, src := range standardSchemaSources {
		results = append(results, *k.ingestStandardSchema(ctx, rootDir, src))
	}
	return results
}

func (k *KnowledgeIngestionService) ingestStandardSchema(ctx context.Context, rootDir string, src StandardSchemaSource) *IngestionResult {
	result := &IngestionResult{SourceType: src.SourceType}
	dir := filepath.Join(rootDir, src.Dir)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		result.Errors = append(result.Errors, fmt.Sprintf("directory not found: %s", dir))
		return result
	}
	if err := k.embedding.ClearSourceRefPrefix(ctx, src.SourceType, "schema"); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("clear: %v", err))
	}

	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || strings.ToLower(filepath.Ext(path)) != ".json" {
			return nil
		}
		result.FilesScanned++
		text, _, readErr := readSchemaFile(path)
		if readErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", path, readErr))
			return nil
		}
		rel, relErr := filepath.Rel(dir, path)
		if relErr != nil {
			rel = filepath.Base(path)
		}
		rel = filepath.ToSlash(strings.TrimSuffix(rel, filepath.Ext(rel)))
		n, embedErr := k.embedding.IngestText(ctx, src.SourceType, "schema:"+rel, path, standardSchemaChunkText(src, rel, text), nil)
		result.ChunksStored += n
		if embedErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s embed: %v", path, embedErr))
		}
		return nil
	})

	log.Printf("✅ AI KB — %s schema: %d files, %d chunks, %d errors",
		src.SourceType, result.FilesScanned, result.ChunksStored, len(result.Errors))
	return result
}

// standardSchemaChunkText prefixes the schema JSON with which standard and
// which part of it this is (e.g. "transactionSets/835"), so a retrieved chunk
// is self-describing even when split mid-file.
func standardSchemaChunkText(src StandardSchemaSource, rel, schemaJSON string) string {
	return fmt.Sprintf("Standard: %s\nSchema definition: %s (as used by the ezHealthKonnect engine)\n\n%s", src.Label, rel, schemaJSON)
}
