// ncpdptelecom/schema_loader.go
// TelecomSchemaLoader loads the D.0 schema directory tree (manifest.json +
// segments/*.json + transactions/*.json) and resolves every segment
// reference a transaction makes against the shared segment library,
// fail-fast on anything unresolved — mirroring edi/schema_loader.go's own
// NewX12SchemaLoader pattern, simplified for D.0's flatter shape (no
// envelope/composite/loop resolution needed).
//
// Usage:
//
//	loader, err := ncpdptelecom.NewTelecomSchemaLoader("./ncpdptelecom/schemas/telecom_d0")
//	tx := loader.GetTransaction("B1", "request")
//	seg := loader.GetSegment("Patient")
package ncpdptelecom

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// TelecomSchemaLoader holds the loaded, reference-resolved spec with
// thread-safe read access. Construct once at startup and share across
// goroutines.
type TelecomSchemaLoader struct {
	schemaDir string
	spec      *TelecomSpecDef
	mu        sync.RWMutex
}

type onDiskManifest struct {
	Version      string                     `json:"version"`
	HeaderFields []TelecomFieldDef          `json:"headerFields"`
	Segments     map[string]string          `json:"segments"`
	Transactions map[string]onDiskTxManifest `json:"transactions"`
}

// onDiskTxManifest keys a transaction file by the manifest's own map key
// (e.g. "B1_request") — the file itself still carries its own real Code/
// Direction fields, checked against this key at load time for consistency.
type onDiskTxManifest struct {
	File string `json:"file"`
}

// NewTelecomSchemaLoader loads manifest.json, every registered segment file,
// and every registered transaction file from schemaDir.
func NewTelecomSchemaLoader(schemaDir string) (*TelecomSchemaLoader, error) {
	l := &TelecomSchemaLoader{schemaDir: schemaDir}
	if err := l.load(); err != nil {
		return nil, err
	}
	return l, nil
}

// =====================================
// Public query API
// =====================================

// GetSegment returns the shared segment definition for the given key (e.g.
// "Patient"). Returns nil if not defined in the schema.
func (l *TelecomSchemaLoader) GetSegment(key string) *TelecomSegmentDef {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.spec.Segments[key]
}

// GetTransaction returns the transaction definition for the given code and
// direction (e.g. "B1", "request"). Returns nil if not defined.
func (l *TelecomSchemaLoader) GetTransaction(code, direction string) *TelecomTransactionDef {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.spec.Transactions[TransactionKey(code, direction)]
}

// Spec returns the fully-loaded, resolved spec — the shape
// ncpdptelecom.ParseTransmission and ncpdptelecom/builder walk directly.
func (l *TelecomSchemaLoader) Spec() *TelecomSpecDef {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.spec
}

// =====================================
// Internal loading
// =====================================

func (l *TelecomSchemaLoader) load() error {
	manifest, err := readManifest(filepath.Join(l.schemaDir, "manifest.json"))
	if err != nil {
		return err
	}

	segments, err := readSegments(filepath.Join(l.schemaDir, "segments"), manifest.Segments)
	if err != nil {
		return err
	}

	transactions, err := readTransactions(filepath.Join(l.schemaDir, "transactions"), manifest.Transactions, segments)
	if err != nil {
		return err
	}

	spec := &TelecomSpecDef{
		Version:      manifest.Version,
		HeaderFields: manifest.HeaderFields,
		Segments:     segments,
		Transactions: transactions,
	}
	if err := spec.buildIndexes(); err != nil {
		return err
	}

	l.spec = spec
	return nil
}

func readManifest(path string) (*onDiskManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("ncpdptelecom: reading manifest %s: %w", path, err)
	}
	var m onDiskManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("ncpdptelecom: parsing manifest %s: %w", path, err)
	}
	return &m, nil
}

func readSegments(dir string, registered map[string]string) (map[string]*TelecomSegmentDef, error) {
	segments := make(map[string]*TelecomSegmentDef, len(registered))
	for key, relFile := range registered {
		path := filepath.Join(filepath.Dir(dir), relFile)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("ncpdptelecom: reading segment file %s: %w", path, err)
		}
		var seg TelecomSegmentDef
		if err := json.Unmarshal(data, &seg); err != nil {
			return nil, fmt.Errorf("ncpdptelecom: parsing segment file %s: %w", path, err)
		}
		if seg.Key != key {
			return nil, fmt.Errorf("ncpdptelecom: segment file %s declares key %q, manifest registers it as %q", path, seg.Key, key)
		}
		segments[key] = &seg
	}
	return segments, nil
}

func readTransactions(dir string, registered map[string]onDiskTxManifest, segments map[string]*TelecomSegmentDef) (map[string]*TelecomTransactionDef, error) {
	transactions := make(map[string]*TelecomTransactionDef, len(registered))
	for manifestKey, ref := range registered {
		path := filepath.Join(filepath.Dir(dir), ref.File)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("ncpdptelecom: reading transaction file %s: %w", path, err)
		}
		var tx TelecomTransactionDef
		if err := json.Unmarshal(data, &tx); err != nil {
			return nil, fmt.Errorf("ncpdptelecom: parsing transaction file %s: %w", path, err)
		}
		realKey := TransactionKey(tx.Code, tx.Direction)
		if realKey != manifestKey {
			return nil, fmt.Errorf("ncpdptelecom: transaction file %s resolves to key %q, manifest registers it as %q", path, realKey, manifestKey)
		}
		for _, segKey := range append(append([]string{}, tx.TransmissionGroupSegments...), tx.TransactionGroupSegments...) {
			if _, ok := segments[segKey]; !ok {
				return nil, fmt.Errorf("ncpdptelecom: transaction %s references unknown segment %q", manifestKey, segKey)
			}
		}
		transactions[manifestKey] = &tx
	}
	return transactions, nil
}
