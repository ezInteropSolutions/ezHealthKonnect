// edi/schema_loader.go
// X12SchemaLoader loads the X12 schema directory tree (manifest.json +
// schemas/segments/*.json + schemas/envelope.json +
// schemas/transactionSets/*.json) and resolves segment ID REFERENCES against
// the shared segment library, fail-fast on anything unresolved — mirroring
// cda/schema_loader.go's NewCDASchemaLoader/loadProfile pattern.
//
// Usage:
//
//	loader, err := edi.NewX12SchemaLoader("./edi/schemas/x12_005010")
//	txSet := loader.GetTransactionSet("835")
//	seg := loader.GetSegment("CLP")
package edi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// X12SchemaLoader holds the loaded, reference-resolved spec with thread-safe
// read access. Construct once at startup and share across goroutines.
type X12SchemaLoader struct {
	schemaDir string
	spec      *X12SpecDef
	mu        sync.RWMutex
}

// onDiskManifest is manifest.json's shape.
type onDiskManifest struct {
	SpecVersion     string                            `json:"specVersion"`
	TransactionSets map[string]onDiskTransactionSetRef `json:"transactionSets"`
}

type onDiskTransactionSetRef struct {
	File                     string `json:"file"`
	FunctionalIdentifierCode string `json:"functionalIdentifierCode,omitempty"`
	Name                     string `json:"name,omitempty"`
}

// NewX12SchemaLoader loads manifest.json, every schemas/segments/*.json,
// envelope.json, and every registered transaction set from schemaDir.
func NewX12SchemaLoader(schemaDir string) (*X12SchemaLoader, error) {
	l := &X12SchemaLoader{schemaDir: schemaDir}
	if err := l.load(); err != nil {
		return nil, err
	}
	return l, nil
}

// =====================================
// Public query API
// =====================================

// GetTransactionSet returns the transaction set definition for the given ID
// (e.g. "835"). Returns nil if not defined in the schema.
func (l *X12SchemaLoader) GetTransactionSet(id string) *X12TransactionSetDef {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.spec.TransactionSets[id]
}

// GetSegment returns the shared segment definition for the given ID (e.g.
// "CLP"). Returns nil if not defined in the schema.
func (l *X12SchemaLoader) GetSegment(id string) *X12SegmentDef {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.spec.Segments[id]
}

// Envelope returns the shared ISA/GS/GE/IEA element definitions.
func (l *X12SchemaLoader) Envelope() *X12EnvelopeDef {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.spec.Envelope
}

// Spec returns the fully-loaded, resolved spec — the shape edi/loop_engine.go
// and edi/builder walk directly.
func (l *X12SchemaLoader) Spec() *X12SpecDef {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.spec
}

// =====================================
// Internal loading
// =====================================

func (l *X12SchemaLoader) load() error {
	manifest, err := readManifest(filepath.Join(l.schemaDir, "manifest.json"))
	if err != nil {
		return err
	}

	segments, err := readSegments(filepath.Join(l.schemaDir, "segments"))
	if err != nil {
		return err
	}

	envelope, err := readEnvelope(filepath.Join(l.schemaDir, "envelope.json"))
	if err != nil {
		return err
	}

	transactionSets := make(map[string]*X12TransactionSetDef, len(manifest.TransactionSets))
	for id, ref := range manifest.TransactionSets {
		path := filepath.Join(l.schemaDir, "transactionSets", ref.File)
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("edi: cannot read transaction set %q (%s): %w", id, path, err)
		}
		var txSet X12TransactionSetDef
		if err := json.Unmarshal(data, &txSet); err != nil {
			return fmt.Errorf("edi: cannot parse transaction set %q (%s): %w", id, path, err)
		}
		if txSet.TransactionSetID != id {
			return fmt.Errorf("edi: transaction set file %s declares id %q, manifest expects %q", path, txSet.TransactionSetID, id)
		}
		if err := validateSegmentReferences(&txSet, segments); err != nil {
			return fmt.Errorf("edi: transaction set %q: %w", id, err)
		}
		transactionSets[id] = &txSet
	}

	l.spec = &X12SpecDef{
		SpecVersion:     manifest.SpecVersion,
		Segments:        segments,
		TransactionSets: transactionSets,
		Envelope:        envelope,
	}
	return nil
}

func readManifest(path string) (*onDiskManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("edi: cannot read manifest (%s): %w", path, err)
	}
	var manifest onDiskManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("edi: cannot parse manifest (%s): %w", path, err)
	}
	return &manifest, nil
}

// readSegments loads every JSON file in segmentsDir into the shared segment
// library, keyed by the ID each file declares (which must match its own
// filename, same discipline cda/schema_loader.go's section-key check uses).
func readSegments(segmentsDir string) (map[string]*X12SegmentDef, error) {
	entries, err := os.ReadDir(segmentsDir)
	if err != nil {
		return nil, fmt.Errorf("edi: cannot read segments directory (%s): %w", segmentsDir, err)
	}

	segments := make(map[string]*X12SegmentDef, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(segmentsDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("edi: cannot read segment file (%s): %w", path, err)
		}
		var seg X12SegmentDef
		if err := json.Unmarshal(data, &seg); err != nil {
			return nil, fmt.Errorf("edi: cannot parse segment file (%s): %w", path, err)
		}
		expectedKey := entry.Name()[:len(entry.Name())-len(".json")]
		if seg.ID != expectedKey {
			return nil, fmt.Errorf("edi: segment file %s declares id %q, filename expects %q", path, seg.ID, expectedKey)
		}
		segments[seg.ID] = &seg
	}
	return segments, nil
}

func readEnvelope(path string) (*X12EnvelopeDef, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("edi: cannot read envelope (%s): %w", path, err)
	}
	var envelope X12EnvelopeDef
	if err := json.Unmarshal(data, &envelope); err != nil {
		return nil, fmt.Errorf("edi: cannot parse envelope (%s): %w", path, err)
	}
	return &envelope, nil
}

// validateSegmentReferences walks a transaction set's header, loop tree, and
// trailer, failing fast if any segmentIds entry doesn't resolve against the
// shared segment library — the same "fail on an unknown reference at load
// time, not silently at parse time" discipline cda/schema_disk_types.go uses
// for its own template resolution.
func validateSegmentReferences(txSet *X12TransactionSetDef, segments map[string]*X12SegmentDef) error {
	checkIDs := func(ids []string, where string) error {
		for _, id := range ids {
			if _, ok := segments[id]; !ok {
				return fmt.Errorf("unresolved segment reference %q in %s", id, where)
			}
		}
		return nil
	}

	if err := checkIDs(txSet.HeaderSegmentIDs, "header"); err != nil {
		return err
	}
	if err := checkIDs(txSet.TrailerSegmentIDs, "trailer"); err != nil {
		return err
	}

	var walkLoops func(loops []*X12LoopDef) error
	walkLoops = func(loops []*X12LoopDef) error {
		for _, loop := range loops {
			if err := checkIDs(loop.SegmentIDs, fmt.Sprintf("loop %q", loop.ID)); err != nil {
				return err
			}
			if err := walkLoops(loop.Loops); err != nil {
				return err
			}
		}
		return nil
	}
	return walkLoops(txSet.Loops)
}
