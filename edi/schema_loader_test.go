package edi

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeJSON(t *testing.T, path string, v interface{}) {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %s: %v", path, err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", path, err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// setupTestSchemaDir writes a small synthetic on-disk schema tree matching
// the manifest.json + segments/*.json + envelope.json +
// transactionSets/*.json layout NewX12SchemaLoader expects, returning the
// root directory.
func setupTestSchemaDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	writeJSON(t, filepath.Join(root, "manifest.json"), map[string]interface{}{
		"specVersion": "TEST",
		"transactionSets": map[string]interface{}{
			"TST": map[string]interface{}{"file": "TST.json", "name": "Test Transaction Set"},
		},
	})

	writeJSON(t, filepath.Join(root, "envelope.json"), &X12EnvelopeDef{
		ISA: []*X12ElementDef{{Pos: "01", Key: "authInfoQualifier"}},
		GS:  []*X12ElementDef{{Pos: "01", Key: "functionalIDCode"}},
		GE:  []*X12ElementDef{{Pos: "01", Key: "count"}},
		IEA: []*X12ElementDef{{Pos: "01", Key: "count"}},
	})

	writeJSON(t, filepath.Join(root, "segments", "ST.json"), &X12SegmentDef{
		ID: "ST", Usage: "required",
		Elements: []*X12ElementDef{{Pos: "01", Key: "transactionSetIdentifierCode", DataType: TypeID}},
	})
	writeJSON(t, filepath.Join(root, "segments", "N1.json"), &X12SegmentDef{
		ID: "N1", Usage: "required",
		Elements: []*X12ElementDef{{Pos: "01", Key: "entityIdentifierCode", DataType: TypeID}},
	})

	writeJSON(t, filepath.Join(root, "transactionSets", "TST.json"), &X12TransactionSetDef{
		TransactionSetID: "TST",
		HeaderSegmentIDs: []string{"ST"},
		Loops:            []*X12LoopDef{{ID: "1000A", Repeat: "1", SegmentIDs: []string{"N1"}}},
	})

	return root
}

func TestNewX12SchemaLoader_LoadsAndResolves(t *testing.T) {
	root := setupTestSchemaDir(t)

	loader, err := NewX12SchemaLoader(root)
	if err != nil {
		t.Fatalf("NewX12SchemaLoader: %v", err)
	}

	txSet := loader.GetTransactionSet("TST")
	if txSet == nil {
		t.Fatal("GetTransactionSet(TST) returned nil")
	}
	if len(txSet.Loops) != 1 || txSet.Loops[0].ID != "1000A" {
		t.Errorf("unexpected loops: %#v", txSet.Loops)
	}

	seg := loader.GetSegment("N1")
	if seg == nil {
		t.Fatal("GetSegment(N1) returned nil")
	}
	if seg.Elements[0].Key != "entityIdentifierCode" {
		t.Errorf("unexpected N1 element: %#v", seg.Elements[0])
	}

	if loader.Envelope() == nil {
		t.Error("Envelope() returned nil")
	}
}

func TestNewX12SchemaLoader_FailsFastOnUnresolvedSegmentReference(t *testing.T) {
	root := setupTestSchemaDir(t)

	// Overwrite the transaction set to reference a segment ID that has no
	// corresponding file in segments/ — must fail at load time, not later.
	writeJSON(t, filepath.Join(root, "transactionSets", "TST.json"), &X12TransactionSetDef{
		TransactionSetID: "TST",
		HeaderSegmentIDs: []string{"NOPE"},
	})

	if _, err := NewX12SchemaLoader(root); err == nil {
		t.Fatal("expected an error for an unresolved segment reference, got nil")
	}
}

func TestNewX12SchemaLoader_MissingManifestIsError(t *testing.T) {
	root := t.TempDir() // empty — no manifest.json
	if _, err := NewX12SchemaLoader(root); err == nil {
		t.Fatal("expected an error for a missing manifest.json, got nil")
	}
}
