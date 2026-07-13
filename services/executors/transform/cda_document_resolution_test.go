// services/executors/transform/cda_document_resolution_test.go
package transform

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cdadocument "ezhealthkonnect/cda/document"
	"ezhealthkonnect/models"
	cdaparser "ezhealthkonnect/services/parsers/cda"
)

// ── findCDADocument / setCDADocument ───────────────────────────────────────

func TestFindCDADocument_MessageNested_Found(t *testing.T) {
	doc := &cdadocument.CDADocument{}
	inputData := map[string]interface{}{"message": map[string]interface{}{"_cdaDocument": doc}}
	got, ok := findCDADocument(inputData)
	if !ok || got != doc {
		t.Fatalf("findCDADocument: ok=%v got=%v, want the same doc pointer", ok, got)
	}
}

func TestFindCDADocument_TopLevelOnly_NotFound(t *testing.T) {
	// The old (broken) shape — a sibling top-level key, no "message" wrapper —
	// must NOT be found. There is no backward-compat shim for it.
	doc := &cdadocument.CDADocument{}
	inputData := map[string]interface{}{"_cdaDocument": doc}
	_, ok := findCDADocument(inputData)
	if ok {
		t.Error("findCDADocument found a top-level sibling key — the shim was deliberately removed, this should be false")
	}
}

func TestFindCDADocument_NoMessage_NotFound(t *testing.T) {
	_, ok := findCDADocument(map[string]interface{}{})
	if ok {
		t.Error("findCDADocument on empty inputData should be false")
	}
}

func TestSetCDADocument_CreatesMessageIfAbsent(t *testing.T) {
	doc := &cdadocument.CDADocument{}
	outputData := map[string]interface{}{}
	setCDADocument(outputData, doc)
	msg, ok := outputData["message"].(map[string]interface{})
	if !ok {
		t.Fatalf("setCDADocument did not create a message map")
	}
	if msg["_cdaDocument"] != interface{}(doc) {
		t.Errorf("_cdaDocument in created message map is not the same pointer")
	}
}

func TestSetCDADocument_MutatesExistingMessageInPlace(t *testing.T) {
	doc := &cdadocument.CDADocument{}
	existingMsg := map[string]interface{}{"raw": "some xml"}
	outputData := map[string]interface{}{"message": existingMsg}
	setCDADocument(outputData, doc)
	// Same map object — existing keys (e.g. "raw") must survive untouched.
	if existingMsg["raw"] != "some xml" {
		t.Error("setCDADocument replaced the message map instead of mutating it in place")
	}
	if existingMsg["_cdaDocument"] != interface{}(doc) {
		t.Error("setCDADocument did not write into the SAME message map instance")
	}
}

// ── Full chain: cda.parse -> cda.dedupe -> cda.section_to_csv ──────────────
//
// This is the exact scenario that was broken: cda.dedupe's mutation of the
// typed document must be visible to cda.section_to_csv afterward. Threads
// each step's real Execute() output as the next step's real input, mirroring
// exactly what ExecutePipeline does (execCtx.Message ends up being whatever
// the previous step left in outputData["message"]).

func repoRootForResolutionTest(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return filepath.Join(wd, "..", "..", "..")
}

func TestFullChain_ParseDedupeCSV_DedupedEntryIsInvisibleToCSVWithoutFix_NowVisible(t *testing.T) {
	root := repoRootForResolutionTest(t)
	rawXML, err := os.ReadFile(filepath.Join(root, "cda", "document", "testdata", "corpus", "kareo_sample.xml"))
	if err != nil {
		t.Fatalf("reading corpus file: %v", err)
	}

	// Inject a genuine duplicate of the Vital Signs section's <entry> block,
	// the same way the earlier live/manual verification did.
	marker := "2.16.840.1.113883.10.20.22.2.4.1" // Vital Signs section templateId
	raw := string(rawXML)
	idx := strings.Index(raw, marker)
	if idx == -1 {
		t.Fatal("vitals section marker not found in corpus fixture")
	}
	entryStart := strings.Index(raw[idx:], "<entry") + idx
	entryEndRel := strings.Index(raw[entryStart:], "</entry>")
	if entryEndRel == -1 {
		t.Fatal("could not find end of vitals entry block")
	}
	entryEnd := entryStart + entryEndRel + len("</entry>")
	entryBlock := raw[entryStart:entryEnd]
	withDup := raw[:entryEnd] + entryBlock + raw[entryEnd:]

	// Step 1: build cda.parse's OUTPUT directly via an externally-constructed
	// parser (same technique loadCorpusTypedDocument uses elsewhere in this
	// package) rather than calling NewCDAParseExecutor()'s Execute() — its
	// internal parserService loads schemas from a relative "./cda/schemas"
	// path that doesn't resolve under `go test`'s working directory, which
	// is an unrelated environment quirk, not something this test is about.
	// What matters here is the SHAPE cda.parse leaves behind: _cdaDocument
	// written into the shared message object via setCDADocument.
	svc, err := cdaparser.NewFromSchemaDir(filepath.Join(root, "cda", "schemas"))
	if err != nil {
		t.Fatalf("NewFromSchemaDir: %v", err)
	}
	parseResult := svc.Parse(withDup)
	if !parseResult.Success {
		t.Fatalf("parsing synthetic duplicate-vitals document: %v", parseResult.Error)
	}
	parsedDoc, ok := parseResult.TypedDocument.(*cdadocument.CDADocument)
	if !ok {
		t.Fatalf("parse produced no typed document")
	}
	step1Output := map[string]interface{}{"message": map[string]interface{}{"raw": withDup}}
	setCDADocument(step1Output, parsedDoc)

	// Step 2: cda.dedupe — its OWN input is step 1's OWN output (exactly what
	// ExecutePipeline hands the next step when a chain is working correctly).
	dedupeExec := NewCDADedupeExecutor(nil)
	dedupeStep := &models.TransformationStep{StepType: "cda.dedupe", Enabled: true, Config: map[string]interface{}{}}
	step2Output, err := dedupeExec.Execute(context.Background(), dedupeStep, step1Output)
	if err != nil {
		t.Fatalf("cda.dedupe Execute: %v", err)
	}
	dedupeStepOut := step2Output["_stepOutput"].(map[string]interface{})
	if dedupeStepOut["total_removed"].(int) != 1 {
		t.Fatalf("cda.dedupe total_removed = %v, want 1", dedupeStepOut["total_removed"])
	}

	// Step 3: cda.section_to_csv — its input is step 2's output. This is the
	// step that PREVIOUSLY still saw 2 vitals rows because the dedupe
	// mutation never reached it.
	csvExec := NewCDASectionToCSVExecutor()
	csvStep := &models.TransformationStep{StepType: "cda.section_to_csv", Enabled: true, Config: map[string]interface{}{"sections": []interface{}{"vitalSigns"}}}
	step3Output, err := csvExec.Execute(context.Background(), csvStep, step2Output)
	if err != nil {
		t.Fatalf("cda.section_to_csv Execute: %v", err)
	}

	csvStepOut := step3Output["_stepOutput"].(map[string]interface{})
	csvText := csvStepOut["csv_vitalSigns"].(string)
	lines := strings.Split(strings.TrimRight(csvText, "\n"), "\n")
	// kareo_sample.xml's single Vital Signs organizer wraps 5 component
	// observations (Height, Weight, BMI, BP Systolic, BP Diastolic), each
	// now its own CSV row (buildRowsForEntry flattens organizer components —
	// see cda_csv_templates.go's doc comment). Without the dedupe fix this
	// test guards, the injected duplicate organizer would leave BOTH copies
	// visible to cda.section_to_csv: 2 organizers x 5 components = 10 rows
	// (11 lines) instead of the 5 rows (6 lines) asserted here.
	if len(lines) != 6 {
		t.Errorf("csv_vital_signs has %d lines (want 6: header + 5 component rows from the single deduped organizer), got:\n%s", len(lines), csvText)
	}
}
