// services/cda_fhir/declarative_engine_bench_test.go
//
// Phase 2 performance baseline, per the sprint plan's exit criteria
// ("Benchmark against Phase 0's MapDocument() baseline using equivalent
// synthetic load"). Loads the same negation_and_frequency.xml fixture
// document_mapper_bench_test.go's BenchmarkMapDocument_Small sibling
// fixtures come from, parses it once outside the timed loop (same
// convention as that file), then times BuildResources running the
// allergy-negation worked example — the closest equivalent single-section,
// single-entry workload to compare against.
//
// Run via the gobuilder Docker stage (no local Go runtime in this environment):
//   docker run --rm <gobuilder-image> go test -bench=. -benchmem ./services/cda_fhir/...
package cdafhir_test

import (
	"testing"

	cdafhir "ezhealthkonnect/services/cda_fhir"
)

func BenchmarkDeclarativeEngine_AllergyNegation(b *testing.B) {
	documentMap := loadDocumentMapFixture(b, "negation_and_frequency.xml")
	engine := cdafhir.NewDeclarativeEngine()
	rule := negationVerificationStatusRule()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, errs := engine.BuildResources(documentMap, rule); len(errs) != 0 {
			b.Fatalf("unexpected errors: %+v", errs)
		}
	}
}

func BenchmarkDeclarativeEngine_FreeTextSigVsInstructionV2(b *testing.B) {
	documentMap := loadDocumentMapFixture(b, "medication_sig_instruction.xml")
	engine := cdafhir.NewDeclarativeEngine()
	rule := sigInstructionRule()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, errs := engine.BuildResources(documentMap, rule); len(errs) != 0 {
			b.Fatalf("unexpected errors: %+v", errs)
		}
	}
}
