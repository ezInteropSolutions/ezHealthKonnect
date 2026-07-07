// services/executors/transform/cda_dedupe_executor_test.go
package transform

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/models"
	cdaparser "ezhealthkonnect/services/parsers/cda"
)

func vitalEntry(code, value string) cdadocument.CDAEntry {
	return cdadocument.CDAEntry{
		Code:          cdadocument.CDACode{Code: code, DisplayName: "Vital " + code},
		StatusCode:    "completed",
		EffectiveTime: cdadocument.CDATimeRange{Value: cdadocument.CDATime{Value: value}},
	}
}

// ── dedupeSectionEntries ──────────────────────────────────────────────────

func TestDedupeSectionEntries_RemovesExactDuplicates_KeepsFirst(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		vitalEntry("8480-6", "20200101"), // BP systolic, day 1
		vitalEntry("8480-6", "20200101"), // exact duplicate
		vitalEntry("8480-6", "20200102"), // same vital, different day — NOT a duplicate
	}
	rule := cdaDedupeIdentityRules["vitalSigns"]

	result, removed := dedupeSectionEntries(entries, rule, "first")
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if len(result) != 2 {
		t.Fatalf("got %d entries, want 2", len(result))
	}
	if result[0].EffectiveTime.Value.Value != "20200101" || result[1].EffectiveTime.Value.Value != "20200102" {
		t.Errorf("unexpected surviving entries: %+v", result)
	}
}

func TestDedupeSectionEntries_LastStrategy_KeepsLatestOccurrence(t *testing.T) {
	first := vitalEntry("8480-6", "20200101")
	first.StatusCode = "preliminary"
	second := vitalEntry("8480-6", "20200101")
	second.StatusCode = "completed" // this one should survive under "last"

	rule := cdaDedupeIdentityRules["vitalSigns"]
	result, removed := dedupeSectionEntries([]cdadocument.CDAEntry{first, second}, rule, "last")
	if removed != 1 {
		t.Fatalf("removed = %d, want 1", removed)
	}
	if len(result) != 1 || result[0].StatusCode != "completed" {
		t.Errorf("expected the LATER entry (StatusCode=completed) to survive, got %+v", result)
	}
}

func TestDedupeSectionEntries_NoDuplicates_NoneRemoved(t *testing.T) {
	entries := []cdadocument.CDAEntry{
		vitalEntry("8480-6", "20200101"),
		vitalEntry("8462-4", "20200101"), // different code (diastolic vs systolic)
	}
	rule := cdaDedupeIdentityRules["vitalSigns"]
	result, removed := dedupeSectionEntries(entries, rule, "first")
	if removed != 0 || len(result) != 2 {
		t.Fatalf("removed=%d len=%d, want removed=0 len=2", removed, len(result))
	}
}

func TestDedupeSectionEntries_UnresolvedKey_NeverDeduped(t *testing.T) {
	// Both entries have an EMPTY code — a null/unresolved key component must
	// never be treated as a match, even against another empty-keyed entry.
	blank1 := cdadocument.CDAEntry{EffectiveTime: cdadocument.CDATimeRange{Value: cdadocument.CDATime{Value: "20200101"}}}
	blank2 := cdadocument.CDAEntry{EffectiveTime: cdadocument.CDATimeRange{Value: cdadocument.CDATime{Value: "20200101"}}}
	rule := cdaDedupeIdentityRules["vitalSigns"]
	result, removed := dedupeSectionEntries([]cdadocument.CDAEntry{blank1, blank2}, rule, "first")
	if removed != 0 {
		t.Errorf("removed = %d, want 0 (unresolved keys must never be deduped)", removed)
	}
	if len(result) != 2 {
		t.Errorf("got %d entries, want 2 (both kept)", len(result))
	}
}

func TestDedupeSectionEntries_EmptyInput_ReturnsEmptyNotNil(t *testing.T) {
	rule := cdaDedupeIdentityRules["vitalSigns"]
	result, removed := dedupeSectionEntries(nil, rule, "first")
	if removed != 0 {
		t.Errorf("removed = %d, want 0", removed)
	}
	if result == nil {
		t.Error("result is nil, want an empty (but non-nil) slice")
	}
}

// ── SupportedCDADedupeSections ────────────────────────────────────────────

func TestSupportedCDADedupeSections_IncludesAllNineSections(t *testing.T) {
	got := SupportedCDADedupeSections()
	want := []string{
		"allergiesAndIntolerances", "encounters", "immunizations", "medications",
		"problems", "procedures", "results", "socialHistory", "vitalSigns",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d sections %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("section[%d] = %q, want %q (sorted order)", i, got[i], want[i])
		}
	}
}

// ── Full executor: synthetic document with genuine duplicates ────────────

func TestCDADedupeExecutor_SyntheticDocument_RemovesDuplicateVitals(t *testing.T) {
	doc := &cdadocument.CDADocument{
		SectionsByKey: map[string]*cdadocument.CDASection{
			"vitalSigns": {
				Key: "vitalSigns",
				Entries: []cdadocument.CDAEntry{
					vitalEntry("8480-6", "20200101"),
					vitalEntry("8480-6", "20200101"), // duplicate
					vitalEntry("8480-6", "20200102"), // distinct date, not a duplicate
				},
			},
		},
	}

	exec := NewCDADedupeExecutor(nil)
	step := &models.TransformationStep{StepType: "cda.dedupe", Enabled: true, Config: map[string]interface{}{}}
	inputData := map[string]interface{}{"message": map[string]interface{}{"_cdaDocument": doc}}

	out, err := exec.Execute(context.Background(), step, inputData)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// The SAME pointer must be mutated and passed forward, inside the shared message object.
	outMsg, ok := out["message"].(map[string]interface{})
	if !ok {
		t.Fatalf("message missing or wrong type in output")
	}
	outDoc, ok := outMsg["_cdaDocument"].(*cdadocument.CDADocument)
	if !ok {
		t.Fatalf("_cdaDocument missing or wrong type in output message")
	}
	if len(outDoc.SectionsByKey["vitalSigns"].Entries) != 2 {
		t.Errorf("got %d vitalSigns entries after dedupe, want 2", len(outDoc.SectionsByKey["vitalSigns"].Entries))
	}

	stepOutput, ok := out["_stepOutput"].(map[string]interface{})
	if !ok {
		t.Fatalf("_stepOutput missing")
	}
	if stepOutput["total_removed"].(int) != 1 {
		t.Errorf("total_removed = %v, want 1", stepOutput["total_removed"])
	}
}

// ── Config overrides: replace an OOB rule, or dedupe a section with none ──

func TestCDADedupeExecutor_Override_ReplacesOOBRule(t *testing.T) {
	// OOB vitalSigns rule keys on code+date, so these two (same code,
	// DIFFERENT dates) are NOT duplicates under the OOB rule — but ARE under
	// an override that keys on code alone.
	doc := &cdadocument.CDADocument{
		SectionsByKey: map[string]*cdadocument.CDASection{
			"vitalSigns": {
				Key: "vitalSigns",
				Entries: []cdadocument.CDAEntry{
					vitalEntry("8480-6", "20200101"),
					vitalEntry("8480-6", "20200102"),
				},
			},
		},
	}

	exec := NewCDADedupeExecutor(nil)
	step := &models.TransformationStep{
		StepType: "cda.dedupe", Enabled: true,
		Config: map[string]interface{}{
			"overrides": map[string]interface{}{
				"vitalSigns": map[string]interface{}{"keyPaths": []interface{}{"code.code"}},
			},
		},
	}
	inputData := map[string]interface{}{"message": map[string]interface{}{"_cdaDocument": doc}}

	out, err := exec.Execute(context.Background(), step, inputData)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	outMsg := out["message"].(map[string]interface{})
	outDoc := outMsg["_cdaDocument"].(*cdadocument.CDADocument)
	if len(outDoc.SectionsByKey["vitalSigns"].Entries) != 1 {
		t.Errorf("got %d vitalSigns entries, want 1 (override should have collapsed both by code alone)", len(outDoc.SectionsByKey["vitalSigns"].Entries))
	}
}

func TestCDADedupeExecutor_Override_EnablesSectionWithNoOOBRule(t *testing.T) {
	// "functionalStatus" has no OOB identity rule at all — without an
	// override, it must be silently skipped even if listed in "sections".
	doc := &cdadocument.CDADocument{
		SectionsByKey: map[string]*cdadocument.CDASection{
			"functionalStatus": {
				Key: "functionalStatus",
				Entries: []cdadocument.CDAEntry{
					vitalEntry("54522-8", "20200101"),
					vitalEntry("54522-8", "20200101"), // duplicate
				},
			},
		},
	}

	exec := NewCDADedupeExecutor(nil)

	// Without an override: section is skipped entirely (no OOB rule).
	stepNoOverride := &models.TransformationStep{
		StepType: "cda.dedupe", Enabled: true,
		Config: map[string]interface{}{"sections": []interface{}{"functionalStatus"}},
	}
	out, err := exec.Execute(context.Background(), stepNoOverride, map[string]interface{}{"message": map[string]interface{}{"_cdaDocument": doc}})
	if err != nil {
		t.Fatalf("Execute (no override): %v", err)
	}
	if len(doc.SectionsByKey["functionalStatus"].Entries) != 2 {
		t.Fatalf("without an override, functionalStatus should be untouched (2 entries), got %d", len(doc.SectionsByKey["functionalStatus"].Entries))
	}
	stepOutput := out["_stepOutput"].(map[string]interface{})
	if stepOutput["total_removed"].(int) != 0 {
		t.Errorf("total_removed = %v, want 0 (section has no OOB rule and no override)", stepOutput["total_removed"])
	}

	// With an override: functionalStatus now has a user-defined identity key.
	stepWithOverride := &models.TransformationStep{
		StepType: "cda.dedupe", Enabled: true,
		Config: map[string]interface{}{
			"sections": []interface{}{"functionalStatus"},
			"overrides": map[string]interface{}{
				"functionalStatus": map[string]interface{}{"keyPaths": []interface{}{"code.code", "effectiveTime.value.value"}},
			},
		},
	}
	out2, err := exec.Execute(context.Background(), stepWithOverride, map[string]interface{}{"message": map[string]interface{}{"_cdaDocument": doc}})
	if err != nil {
		t.Fatalf("Execute (with override): %v", err)
	}
	outMsg2 := out2["message"].(map[string]interface{})
	outDoc2 := outMsg2["_cdaDocument"].(*cdadocument.CDADocument)
	if len(outDoc2.SectionsByKey["functionalStatus"].Entries) != 1 {
		t.Errorf("got %d functionalStatus entries, want 1 (override should have deduped it)", len(outDoc2.SectionsByKey["functionalStatus"].Entries))
	}
}

func TestCDADedupeExecutor_Validate_RejectsMalformedOverride(t *testing.T) {
	exec := NewCDADedupeExecutor(nil)
	cases := []struct {
		name   string
		config map[string]interface{}
	}{
		{"not an object", map[string]interface{}{"overrides": map[string]interface{}{"medications": "not-an-object"}}},
		{"missing keyPaths", map[string]interface{}{"overrides": map[string]interface{}{"medications": map[string]interface{}{}}}},
		{"empty keyPaths", map[string]interface{}{"overrides": map[string]interface{}{"medications": map[string]interface{}{"keyPaths": []interface{}{}}}}},
		{"non-string keyPaths element", map[string]interface{}{"overrides": map[string]interface{}{"medications": map[string]interface{}{"keyPaths": []interface{}{42}}}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			step := &models.TransformationStep{StepType: "cda.dedupe", Config: c.config}
			if err := exec.Validate(step); err == nil {
				t.Errorf("Validate(%s) = nil, want an error", c.name)
			}
		})
	}
}

func TestCDADedupeExecutor_Validate_AcceptsWellFormedOverride(t *testing.T) {
	exec := NewCDADedupeExecutor(nil)
	step := &models.TransformationStep{
		StepType: "cda.dedupe",
		Config: map[string]interface{}{
			"overrides": map[string]interface{}{
				"medications": map[string]interface{}{"keyPaths": []interface{}{"consumable.manufacturedProduct.manufacturedMaterial.code.code"}},
			},
		},
	}
	if err := exec.Validate(step); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

// ── Full executor against real corpus: must not false-positive on clean data ──

func repoRootForDedupeTest(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(wd, "..", "..", "..")
}

func TestCDADedupeExecutor_RealCorpus_NoFalsePositiveRemovals(t *testing.T) {
	root := repoRootForDedupeTest(t)
	svc, err := cdaparser.NewFromSchemaDir(filepath.Join(root, "cda", "schemas"))
	if err != nil {
		t.Fatalf("NewFromSchemaDir: %v", err)
	}

	for _, file := range []string{"cerner_sample.xml", "kareo_sample.xml", "mtuitive_sample.xml", "practicefusion_sample.xml"} {
		t.Run(file, func(t *testing.T) {
			rawXML, err := os.ReadFile(filepath.Join(root, "cda", "document", "testdata", "corpus", file))
			if err != nil {
				t.Fatalf("reading corpus file: %v", err)
			}
			result := svc.Parse(string(rawXML))
			if !result.Success {
				t.Fatalf("parsing corpus file: %v", result.Error)
			}
			doc, ok := result.TypedDocument.(*cdadocument.CDADocument)
			if !ok {
				t.Fatalf("no typed document produced")
			}

			exec := NewCDADedupeExecutor(nil)
			step := &models.TransformationStep{StepType: "cda.dedupe", Enabled: true, Config: map[string]interface{}{}}
			out, err := exec.Execute(context.Background(), step, map[string]interface{}{"message": map[string]interface{}{"_cdaDocument": doc}})
			if err != nil {
				t.Fatalf("Execute: %v", err)
			}
			stepOutput := out["_stepOutput"].(map[string]interface{})
			t.Logf("%s: total_removed=%v section_stats=%v", file, stepOutput["total_removed"], stepOutput["section_stats"])
			// Real, clean vendor samples should have zero genuine intra-document
			// duplicates — a non-zero removal here would indicate an
			// over-aggressive identity rule (false positive), not a real finding.
			if stepOutput["total_removed"].(int) != 0 {
				t.Errorf("%s: removed %v entries from clean corpus data — investigate for a false-positive identity match", file, stepOutput["total_removed"])
			}
		})
	}
}
