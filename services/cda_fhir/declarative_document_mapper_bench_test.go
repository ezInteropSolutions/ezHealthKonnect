// services/cda_fhir/declarative_document_mapper_bench_test.go
//
// Phase 4 Slice B/D performance check. Same 3 fixtures, same brackets, same
// isolation (XML parsing happens once outside the timed loop) as the
// now-deleted document_mapper_bench_test.go's MapDocument() benchmarks --
// see Phase 4's sprint plan for the recorded comparison table (ns/op,
// allocs/op) from when both engines still existed side by side.
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

// loadCDADocumentForBench parses a CDA testdata fixture into a
// *cdadocument.CDADocument once, outside any benchmark timing loop. Fails
// the benchmark on any error. Moved here from the now-deleted
// document_mapper_bench_test.go — this is its only remaining caller.
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

func runDeclarativeMapDocumentBenchmark(b *testing.B, cdaDoc *cdadocument.CDADocument) {
	b.Helper()

	mapper := cdafhir.NewGenericCDAFHIRMapper(nil, nil)
	ctx := context.Background()
	config := cdafhir.CDAToFHIRConfig{}

	prevOutput := log.Writer()
	log.SetOutput(io.Discard)
	defer log.SetOutput(prevOutput)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := mapper.DeclarativeMapDocument(ctx, cdaDoc, config); err != nil {
			b.Fatalf("DeclarativeMapDocument: unexpected error: %v", err)
		}
	}
}

func BenchmarkDeclarativeMapDocument_Small(b *testing.B) {
	cdaDoc := loadCDADocumentForBench(b, "minimal_ccd.xml")
	runDeclarativeMapDocumentBenchmark(b, cdaDoc)
}

func BenchmarkDeclarativeMapDocument_Medium(b *testing.B) {
	cdaDoc := loadCDADocumentForBench(b, "full_ccd_nist.xml")
	runDeclarativeMapDocumentBenchmark(b, cdaDoc)
}

func BenchmarkDeclarativeMapDocument_Large(b *testing.B) {
	cdaDoc := loadCDADocumentForBench(b, filepath.Join("corpus", "cerner_sample.xml"))
	runDeclarativeMapDocumentBenchmark(b, cdaDoc)
}
