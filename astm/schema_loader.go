// astm/schema_loader.go
// ASTMSchemaLoader loads the ASTM schema directory tree (manifest.json +
// schemas/e1394_97/records/*.json + schemas/e1394_97/messages/*.json),
// fail-fast on anything unresolved — mirroring edi.NewX12SchemaLoader's
// pattern, simplified since ASTM has no loop-ref resolution step to perform
// (see schema_types.go's own doc comment on why).
//
// Usage:
//
//	loader, err := astm.NewASTMSchemaLoader("./astm/schemas/e1394_97")
//	msg := loader.GetMessage("generic_lab_result")
//	rec := loader.GetRecord("O")
package astm

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// ASTMSchemaLoader holds the loaded, reference-resolved spec with thread-safe
// read access. Construct once at startup and share across goroutines.
type ASTMSchemaLoader struct {
	schemaDir string
	spec      *ASTMSpecDef
	mu        sync.RWMutex
}

// onDiskManifest is manifest.json's shape.
type onDiskManifest struct {
	SpecVersion string                     `json:"specVersion"`
	Messages    map[string]onDiskMessageRef `json:"messages"`
}

type onDiskMessageRef struct {
	File string `json:"file"`
	Name string `json:"name,omitempty"`
}

// NewASTMSchemaLoader loads manifest.json, every schemas/records/*.json, and
// every registered message profile from schemaDir.
func NewASTMSchemaLoader(schemaDir string) (*ASTMSchemaLoader, error) {
	l := &ASTMSchemaLoader{schemaDir: schemaDir}
	if err := l.load(); err != nil {
		return nil, err
	}
	return l, nil
}

// =====================================
// Public query API
// =====================================

// GetRecord returns the shared record definition for the given record-type
// letter (e.g. "O"). Returns nil if not defined in the schema.
func (l *ASTMSchemaLoader) GetRecord(id string) *ASTMRecordDef {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.spec.Records[id]
}

// GetMessage returns the message profile definition for the given ID.
// Returns nil if not defined in the schema.
func (l *ASTMSchemaLoader) GetMessage(id string) *ASTMMessageDef {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.spec.Messages[id]
}

// Spec returns the fully-loaded, resolved spec — the shape
// astm/record_engine.go and astm/builder walk directly.
func (l *ASTMSchemaLoader) Spec() *ASTMSpecDef {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.spec
}

// =====================================
// Internal loading
// =====================================

func (l *ASTMSchemaLoader) load() error {
	manifest, err := readManifest(filepath.Join(l.schemaDir, "manifest.json"))
	if err != nil {
		return err
	}

	records, err := readRecords(filepath.Join(l.schemaDir, "records"))
	if err != nil {
		return err
	}

	messages := make(map[string]*ASTMMessageDef, len(manifest.Messages))
	for id, ref := range manifest.Messages {
		path := filepath.Join(l.schemaDir, "messages", ref.File)
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("astm: cannot read message profile %q (%s): %w", id, path, err)
		}
		var msg ASTMMessageDef
		if err := json.Unmarshal(data, &msg); err != nil {
			return fmt.Errorf("astm: cannot parse message profile %q (%s): %w", id, path, err)
		}
		if msg.ID != id {
			return fmt.Errorf("astm: message profile file %s declares id %q, manifest expects %q", path, msg.ID, id)
		}
		messages[id] = &msg
	}

	// There are no cross-references to validate beyond the manifest-vs-file
	// id check above — record letters are a closed, hardcoded alphabet
	// (H/P/O/R/C/Q/L) astm/record_engine.go addresses directly, not a
	// variable reference a message profile declares.
	for _, mustExist := range []string{"H", "P", "O", "R", "C", "Q", "L"} {
		if _, ok := records[mustExist]; !ok {
			return fmt.Errorf("astm: schema directory %s is missing required record definition %q", l.schemaDir, mustExist)
		}
	}

	l.spec = &ASTMSpecDef{
		SpecVersion: manifest.SpecVersion,
		Records:     records,
		Messages:    messages,
	}
	return nil
}

func readManifest(path string) (*onDiskManifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("astm: cannot read manifest (%s): %w", path, err)
	}
	var manifest onDiskManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("astm: cannot parse manifest (%s): %w", path, err)
	}
	return &manifest, nil
}

// readRecords loads every JSON file in recordsDir into the shared record
// library, keyed by the ID each file declares (which must match its own
// filename, same discipline edi.readSegments uses).
func readRecords(recordsDir string) (map[string]*ASTMRecordDef, error) {
	entries, err := os.ReadDir(recordsDir)
	if err != nil {
		return nil, fmt.Errorf("astm: cannot read records directory (%s): %w", recordsDir, err)
	}

	records := make(map[string]*ASTMRecordDef, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(recordsDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("astm: cannot read record file (%s): %w", path, err)
		}
		var rec ASTMRecordDef
		if err := json.Unmarshal(data, &rec); err != nil {
			return nil, fmt.Errorf("astm: cannot parse record file (%s): %w", path, err)
		}
		expectedKey := entry.Name()[:len(entry.Name())-len(".json")]
		if rec.ID != expectedKey {
			return nil, fmt.Errorf("astm: record file %s declares id %q, filename expects %q", path, rec.ID, expectedKey)
		}
		records[rec.ID] = &rec
	}
	return records, nil
}
