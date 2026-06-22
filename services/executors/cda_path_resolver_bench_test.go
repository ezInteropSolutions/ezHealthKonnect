// services/executors/cda_path_resolver_bench_test.go
//
// Phase 1 performance baseline for the wildcard/predicate path resolver, per
// the sprint plan's Phase 1 exit criteria ("Performance baseline established
// (resolve N paths in target latency) — this number is the Phase 2+
// regression floor for the resolver specifically"). Parsing happens once
// per benchmark, outside the timed loop, mirroring
// services/cda_fhir/document_mapper_bench_test.go's existing convention for
// the MapDocument() baseline.
//
// Run via the gobuilder Docker stage (no local Go runtime in this environment):
//   docker run --rm <gobuilder-image> go test -bench=. -benchmem ./services/executors/...
package executors

import "testing"

// runResolveCDAPathBenchmark times a representative mix of plain, indexed,
// wildcard, predicate, and fallback-chain resolutions per b.N iteration
// against an already-parsed fixture.
func runResolveCDAPathBenchmark(b *testing.B, fx parsedCDAFixture) {
	b.Helper()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ResolveCDAPath(fx.documentMap,
			"sectionsByKey.allergiesAndIntolerances.entries[0].entryRelationships[typeCode=SUBJ].entry.negationInd",
			false)
		ResolveCDAPaths(fx.documentMap,
			"sectionsByKey.medications.entries[0].effectiveTimes[*].xsiType",
			false)
		ResolveCDAPath(fx.xmlMirror,
			"component.structuredBody.component[0].section.entry.act.entryRelationship[typeCode=SUBJ].observation.@negationInd",
			true)
		ResolveCDAFallback(fx.documentMap, false,
			"sectionsByKey.medications.entries[0].doseQuantity.value",
			"sectionsByKey.medications.entries[0].doseQuantity.nullFlavor",
		)
	}
}

func BenchmarkResolveCDAPath_NegationAndFrequency(b *testing.B) {
	fx := loadParsedCDAFixture(b, "negation_and_frequency.xml")
	runResolveCDAPathBenchmark(b, fx)
}

func BenchmarkResolveCDAPath_SupplyAndNullFlavors(b *testing.B) {
	fx := loadParsedCDAFixture(b, "supply_and_nullflavors.xml")
	runResolveCDAPathBenchmark(b, fx)
}

// BenchmarkResolveCDAPath_CorpusLarge_WildcardFanout measures wildcard
// fan-out cost against the largest real-world fixture committed
// (corpus/cerner_sample.xml, ~36.7 KB — same file used as the "Large"
// bracket in document_mapper_bench_test.go's MapDocument() baseline),
// resolving every entry's entryType across every section in one call.
func BenchmarkResolveCDAPath_CorpusLarge_WildcardFanout(b *testing.B) {
	fx := loadParsedCDAFixture(b, "corpus/cerner_sample.xml")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ResolveCDAPaths(fx.documentMap, "sectionsByKey.medications.entries[*].entryType", false)
	}
}
