// services/ai/schema_digest.go
//
// Condensed ("digest") ingestion for the HL7 v2 and FHIR schema dictionaries.
//
// The raw dictionaries cannot be embedded as-is. They are gzip-compressed
// (the old loaders only read .json/.txt, so they ingested zero files), and
// they are fully expanded: every HL7 message file repeats every segment and
// every field repeats its whole data-type tree — one v2.5.1 ADT_A01 file is
// ~20 MB uncompressed, the HL7 tree 1.5 GB compressed. At ~1.5 s per embedded
// chunk on CPU that is months of work. A digest keeps what answers real
// questions — each segment/resource once, its fields/elements with type,
// optionality and repetition, and each HL7 message's segment structure — in
// roughly a thousand chunks.
//
// Driven by schemaDigestSources: another HL7 version or FHIR profile set is
// one table row; a new schema family is one SchemaDigester implementation.
package ai

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SchemaDigestDoc is one knowledge document produced from a schema file.
type SchemaDigestDoc struct {
	Ref  string // unique within its source, e.g. "segment:PID"
	Text string
}

// SchemaDigester turns one schema file into knowledge documents. It is called
// once per file in the source directory, in sorted order; it may keep state
// across files (e.g. to emit each shared HL7 segment only once).
type SchemaDigester interface {
	DigestFile(name string, content []byte) ([]SchemaDigestDoc, error)
}

// SchemaDigestSource names one schema directory to digest.
type SchemaDigestSource struct {
	Key        string // knowledge source key (ai_knowledge_sources), also the ref prefix
	SourceType string // retrieval source_type
	Dir        string // relative to the schema root
	New        func() SchemaDigester
}

// schemaDigestSources drives digest ingestion. HL7 uses one version — field
// definitions barely change between minor versions and 2.5.1 is the most
// widely deployed; add a row to digest another version alongside it.
var schemaDigestSources = []SchemaDigestSource{
	{Key: "hl7_v2", SourceType: "hl7_v2", Dir: filepath.Join("hl7", "v2.5.1"), New: func() SchemaDigester { return newHL7v2Digester("2.5.1") }},
	{Key: "fhir_r4", SourceType: "fhir_r4", Dir: filepath.Join("fhir", "R4", "resources"), New: func() SchemaDigester { return newFHIRStructureDigester("FHIR R4 resource") }},
	{Key: "fhir_r4:us-core", SourceType: "fhir_r4", Dir: filepath.Join("fhir", "R4", "profiles", "us-core"), New: func() SchemaDigester { return newFHIRStructureDigester("US Core profile (FHIR R4)") }},
	{Key: "fhir_r4:davinci-pas", SourceType: "fhir_r4", Dir: filepath.Join("fhir", "R4", "profiles", "davinci-pas"), New: func() SchemaDigester { return newFHIRStructureDigester("Da Vinci PAS profile (FHIR R4)") }},
}

// schemaDigestExts are the schema file extensions a digest reads.
var schemaDigestExts = []string{".gz", ".json"}

// ingestSchemaDigest replaces one digest source's chunks.
func (k *KnowledgeIngestionService) ingestSchemaDigest(ctx context.Context, schemaDir string, src SchemaDigestSource) *IngestionResult {
	dir := filepath.Join(schemaDir, src.Dir)
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return nil // not shipped on this install
	}
	result := &IngestionResult{SourceType: src.Key}

	docs, files, errs := digestDir(dir, src.New())
	result.FilesScanned = files
	result.Errors = append(result.Errors, errs...)

	refPrefix := "digest:" + src.Key
	if err := k.embedding.ClearSourceRefPrefix(ctx, src.SourceType, refPrefix); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("clear: %v", err))
	}
	for _, d := range docs {
		if ctx.Err() != nil {
			result.Errors = append(result.Errors, ctx.Err().Error())
			break
		}
		n, err := k.embedding.IngestText(ctx, src.SourceType, refPrefix+":"+d.Ref, filepath.ToSlash(src.Dir), d.Text, nil)
		result.ChunksStored += n
		if err != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", d.Ref, err))
		}
	}
	log.Printf("✅ AI KB — %s digest: %d files, %d documents, %d chunks, %d errors",
		src.Key, result.FilesScanned, len(docs), result.ChunksStored, len(result.Errors))
	return result
}

// digestDir runs digester over every schema file in dir (sorted, recursive).
func digestDir(dir string, digester SchemaDigester) (docs []SchemaDigestDoc, files int, errs []string) {
	var paths []string
	_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && hasExt(path, schemaDigestExts) {
			paths = append(paths, path)
		}
		return nil
	})
	sort.Strings(paths)
	for _, path := range paths {
		files++
		content, err := readMaybeGzip(path)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		out, err := digester.DigestFile(strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), content)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", path, err))
			continue
		}
		docs = append(docs, out...)
	}
	return docs, files, errs
}

// readMaybeGzip reads a file, transparently decompressing .gz.
func readMaybeGzip(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if strings.EqualFold(filepath.Ext(path), ".gz") {
		zr, err := gzip.NewReader(f)
		if err != nil {
			return nil, err
		}
		defer zr.Close()
		return io.ReadAll(zr)
	}
	return io.ReadAll(f)
}

// firstSentence shortens a description to its first sentence, capped at max runes.
func firstSentence(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if i := strings.Index(s, ". "); i > 0 {
		s = s[:i+1]
	}
	r := []rune(s)
	if len(r) > max {
		s = string(r[:max-1]) + "…"
	}
	return s
}
