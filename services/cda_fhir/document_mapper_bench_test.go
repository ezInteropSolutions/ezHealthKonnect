// services/cda_fhir/document_mapper_bench_test.go
//
// Phase 0 performance baseline for MapDocument() (the typed-struct CDA->FHIR path),
// per the CDA_FHIR_MAPPING_ENGINE_SPRINT_PLAN.md cross-cutting Performance checklist:
// "Baseline current MapDocument() latency per document-size bracket before Phase 1
// starts (this is the regression floor)." This file measures MapDocument() only --
// XML parsing (cdadocument.CDAParser) happens once during benchmark setup, outside
// the timed loop, since Phase 1's path resolver is a MapDocument()-internal concern,
// not a parsing concern.
//
// Run via the gobuilder Docker stage (no local Go runtime in this environment):
//   docker run --rm <gobuilder-image> go test -bench=. -benchmem ./services/cda_fhir/...
//
// Brackets, by raw XML file size (smallest real fixture -> largest real corpus sample):
//   Small  (~4.8 KB,  1 section)             minimal_ccd.xml
//   Medium (~17.8 KB, >=9 sections)           full_ccd_nist.xml
//   Large  (~36.7 KB, real vendor export)     corpus/cerner_sample.xml

package cdafhir_test

import (
	"context"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	cdaSchema "ezhealthkonnect/cda"
	cdadocument "ezhealthkonnect/cda/document"
	cdafhir "ezhealthkonnect/services/cda_fhir"

	"github.com/beevik/etree"
)

// loadCDADocumentForBench parses a CDA testdata fixture into a *cdadocument.CDADocument
// once, outside any benchmark timing loop. Fails the benchmark on any error.
func loadCDADocumentForBench(b *testing.B, relPath string) *cdadocument.CDADocument {
	b.Helper()

	_, thisFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..")

	data, err := os.ReadFile(filepath.Join(repoRoot, "cda", "document", "testdata", relPath))
	if err != nil {
		b.Fatalf("reading testdata %s: %v", relPath, err)
	}
	raw := string(data)

	doc := etree.NewDocument()
	if err := doc.ReadFromString(raw); err != nil {
		b.Fatalf("parsing XML in %s: %v", relPath, err)
	}

	loader, err := cdaSchema.NewCDASchemaLoader(filepath.Join(repoRoot, "cda", "schemas"))
	if err != nil {
		b.Fatalf("loading CDA schema: %v", err)
	}

	p := cdadocument.NewCDAParser(loader)
	return p.ParseDocument(doc.Root(), raw)
}

// runMapDocumentBenchmark times MapDocument() in isolation against an already-parsed
// document, reporting standard ns/op + B/op + allocs/op via -benchmem.
func runMapDocumentBenchmark(b *testing.B, cdaDoc *cdadocument.CDADocument) {
	b.Helper()

	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	ctx := context.Background()
	config := cdafhir.CDAToFHIRConfig{}

	// MapDocument logs one line per call (log.Printf) -- silence it for the
	// timed loop so per-iteration stdout writes aren't part of what we measure.
	prevOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(prevOutput)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := mapper.MapDocument(ctx, cdaDoc, config); err != nil {
			b.Fatalf("MapDocument: unexpected error: %v", err)
		}
	}
}

func BenchmarkMapDocument_Small(b *testing.B) {
	cdaDoc := loadCDADocumentForBench(b, "minimal_ccd.xml")
	runMapDocumentBenchmark(b, cdaDoc)
}

func BenchmarkMapDocument_Medium(b *testing.B) {
	cdaDoc := loadCDADocumentForBench(b, "full_ccd_nist.xml")
	runMapDocumentBenchmark(b, cdaDoc)
}

func BenchmarkMapDocument_Large(b *testing.B) {
	cdaDoc := loadCDADocumentForBench(b, filepath.Join("corpus", "cerner_sample.xml"))
	runMapDocumentBenchmark(b, cdaDoc)
}
