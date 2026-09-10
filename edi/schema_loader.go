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

// onDiskTransactionSetRef is one manifest.json transactionSets entry. The
// map key it's registered under (id, below) is EITHER a transaction set's
// bare ST01 value (single-variant sets — "835", "999") OR a composite
// "ST01:GS08" key (multi-variant sets sharing one ST01 value — 837P/837I
// both being literally "837") — see NewX12SchemaLoader's own package doc and
// CLAUDE.md's EDI Phase 2 section. FriendlyID is the human-legible id a
// multi-variant set's own JSON file and every caller actually use ("837P"),
// distinct from the composite map key, which no caller needs to know about.
type onDiskTransactionSetRef struct {
	File       string `json:"file"`
	FriendlyID string `json:"friendlyId,omitempty"`
	Name       string `json:"name,omitempty"`
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

	sharedLoops, err := readLoops(filepath.Join(l.schemaDir, "loops"))
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

		var raw map[string]interface{}
		if err := json.Unmarshal(data, &raw); err != nil {
			return fmt.Errorf("edi: cannot parse transaction set %q (%s): %w", id, path, err)
		}
		if loopsRaw, ok := raw["loops"].([]interface{}); ok {
			resolved, err := resolveLoopRefs(loopsRaw, sharedLoops, map[string]bool{})
			if err != nil {
				return fmt.Errorf("edi: transaction set %q (%s): %w", id, path, err)
			}
			raw["loops"] = resolved
		}
		resolvedData, err := json.Marshal(raw)
		if err != nil {
			return fmt.Errorf("edi: transaction set %q (%s): re-marshaling resolved loops: %w", id, path, err)
		}

		var txSet X12TransactionSetDef
		if err := json.Unmarshal(resolvedData, &txSet); err != nil {
			return fmt.Errorf("edi: cannot parse transaction set %q (%s): %w", id, path, err)
		}

		// A single-variant set's own file declares the same id as its bare
		// manifest key ("835"). A multi-variant set's file declares the
		// human-legible FriendlyID ("837P") — the manifest key itself is the
		// internal composite "837:005010X222A1" no caller ever needs to type.
		expectedID := id
		if ref.FriendlyID != "" {
			expectedID = ref.FriendlyID
		}
		if txSet.TransactionSetID != expectedID {
			return fmt.Errorf("edi: transaction set file %s declares id %q, manifest expects %q", path, txSet.TransactionSetID, expectedID)
		}
		if err := validateSegmentReferences(&txSet, segments); err != nil {
			return fmt.Errorf("edi: transaction set %q: %w", id, err)
		}

		transactionSets[id] = &txSet
		if ref.FriendlyID != "" && ref.FriendlyID != id {
			// Registered under BOTH keys so every existing caller that looks
			// up spec.TransactionSets[someID] directly (edi/builder's
			// BuildDocument, controllers/edi_schema_controller.go) resolves a
			// friendly id with zero code changes of their own — only
			// edi/loop_engine.go's own parse-time lookup needs to know about
			// the composite form, since parsing has no caller-supplied id to
			// start from.
			transactionSets[ref.FriendlyID] = &txSet
		}
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

// readLoops loads every JSON file in loopsDir into the shared LOOP library
// (edi/schemas/x12_005010/loops/*.json), keyed by the id each file declares
// — mirroring readSegments' own discipline, one level up. The directory is
// OPTIONAL: a schema with no multi-position-reused loop subtree (835, 999
// alone) has no loops/ directory at all, and that's not an error.
func readLoops(loopsDir string) (map[string]json.RawMessage, error) {
	entries, err := os.ReadDir(loopsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]json.RawMessage{}, nil
		}
		return nil, fmt.Errorf("edi: cannot read loops directory (%s): %w", loopsDir, err)
	}

	loops := make(map[string]json.RawMessage, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(loopsDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("edi: cannot read loop file (%s): %w", path, err)
		}
		if !json.Valid(data) {
			return nil, fmt.Errorf("edi: cannot parse loop file (%s): invalid JSON", path)
		}
		// Unlike readSegments, the filename is NOT required to equal the
		// loop's own declared "id" — a shared loop template's filename is
		// deliberately a descriptive loopRef lookup key ("2300-claim"),
		// distinct from the loop's own real X12 loop id ("2300") that ends
		// up on the resolved X12LoopDef once substituted in. The filename
        // (without ".json") IS the loopRef key every transaction set
		// references it by.
		key := entry.Name()[:len(entry.Name())-len(".json")]
		loops[key] = json.RawMessage(data)
	}
	return loops, nil
}

// resolveLoopRefs walks a generic, not-yet-typed loops array (each element a
// map[string]interface{} decoded from raw JSON) and replaces any
// {"loopRef": "<id>"} entry with the shared loop's own fully-resolved
// content — a load-time substitution mirroring cda/schema_disk_types.go's
// entry-template resolution, one level up (loops instead of entries).
// X12LoopDef itself carries no LoopRef field and never sees one: this
// operates entirely on generic JSON before the final typed json.Unmarshal
// into *X12TransactionSetDef happens in load() above.
//
// Each {"loopRef": id} occurrence is resolved from sharedLoops' own raw
// bytes freshly (a fresh json.Unmarshal per occurrence), so multiple
// references to the same shared loop — 837P/837I's own 2000B and 2000C both
// referencing "2300-claim" — always get their own independently-owned tree,
// never an aliased one. resolving tracks the in-progress reference chain so
// a loop file that (mis)references itself, directly or transitively, fails
// fast at load time instead of infinite-looping.
func resolveLoopRefs(loops []interface{}, sharedLoops map[string]json.RawMessage, resolving map[string]bool) ([]interface{}, error) {
	for i, raw := range loops {
		loopObj, ok := raw.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("loop entry at index %d is not an object", i)
		}

		if ref, hasRef := loopObj["loopRef"]; hasRef {
			refID, _ := ref.(string)
			if refID == "" {
				return nil, fmt.Errorf("loopRef at index %d has no string value", i)
			}
			if resolving[refID] {
				return nil, fmt.Errorf("circular loopRef %q", refID)
			}
			sharedRaw, ok := sharedLoops[refID]
			if !ok {
				return nil, fmt.Errorf("unresolved loopRef %q", refID)
			}
			var sharedObj map[string]interface{}
			if err := json.Unmarshal(sharedRaw, &sharedObj); err != nil {
				return nil, fmt.Errorf("cannot parse shared loop %q: %w", refID, err)
			}
			resolving[refID] = true
			err := resolveLoopObjectInPlace(sharedObj, sharedLoops, resolving)
			delete(resolving, refID)
			if err != nil {
				return nil, err
			}
			loops[i] = sharedObj
			continue
		}

		if err := resolveLoopObjectInPlace(loopObj, sharedLoops, resolving); err != nil {
			return nil, err
		}
	}
	return loops, nil
}

// resolveLoopObjectInPlace resolves loopRefs within one already-typed loop
// object's own nested "loops" array, if it has one. No-op for a loop with no
// children (the vast majority — most loops don't nest further).
func resolveLoopObjectInPlace(loopObj map[string]interface{}, sharedLoops map[string]json.RawMessage, resolving map[string]bool) error {
	nested, ok := loopObj["loops"].([]interface{})
	if !ok {
		return nil
	}
	resolved, err := resolveLoopRefs(nested, sharedLoops, resolving)
	if err != nil {
		return err
	}
	loopObj["loops"] = resolved
	return nil
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
			if err := checkIDs(loop.TrailerSegmentIDs, fmt.Sprintf("loop %q trailer", loop.ID)); err != nil {
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
