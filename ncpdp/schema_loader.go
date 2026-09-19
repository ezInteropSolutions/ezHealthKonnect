// ncpdp/schema_loader.go
//
// Loads ncpdp/schemas/<version>/manifest.json (+ the group/transaction files
// it references) into a resolved *NCPDPSpecDef. Mirrors the load-once,
// thread-safe-read-after pattern already proven by edi.X12SchemaLoader and
// cda.CDASchemaLoader.
//
// Unlike EDI's loopRef mechanism (which deep-copies a shared loop template
// into each reference site because a loop can carry per-position overrides),
// NCPDP groups need no such flattening: a "Name" group is structurally
// identical everywhere it's referenced, so NCPDPGroupRef.GroupKey stays a
// live lookup into the shared Groups map, resolved at parse/build time, not
// at load time. This keeps the loader itself intentionally thin — read every
// file the manifest names, validate every reference resolves, done.
package ncpdp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// NCPDPSchemaLoader loads and holds one resolved NCPDPSpecDef.
type NCPDPSchemaLoader struct {
	schemaDir string
	mu        sync.RWMutex
	spec      *NCPDPSpecDef
}

// NewNCPDPSchemaLoader loads the schema at schemaDir immediately, returning
// an error if the manifest or any referenced file is missing/invalid.
func NewNCPDPSchemaLoader(schemaDir string) (*NCPDPSchemaLoader, error) {
	l := &NCPDPSchemaLoader{schemaDir: schemaDir}
	if err := l.load(); err != nil {
		return nil, err
	}
	return l, nil
}

// Spec returns the loaded spec.
func (l *NCPDPSchemaLoader) Spec() *NCPDPSpecDef {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.spec
}

// GetTransaction returns the transaction def for key, or an error if unknown.
func (l *NCPDPSchemaLoader) GetTransaction(key string) (*NCPDPTransactionDef, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	tx, ok := l.spec.Transactions[key]
	if !ok {
		return nil, fmt.Errorf("ncpdp: unknown transaction type %q", key)
	}
	return tx, nil
}

// GetGroup returns the group def for key, or an error if unknown.
func (l *NCPDPSchemaLoader) GetGroup(key string) (*NCPDPGroupDef, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	g, ok := l.spec.Groups[key]
	if !ok {
		return nil, fmt.Errorf("ncpdp: unknown group %q", key)
	}
	return g, nil
}

// onDiskManifest is the raw shape of manifest.json.
type onDiskManifest struct {
	Version        string            `json:"version"`
	MessageAttrs   NCPDPMessageAttrs `json:"messageAttrs"`
	HeaderGroupKey string            `json:"headerGroupKey"`
	Groups         map[string]string `json:"groups"`       // group key -> file path relative to schemaDir
	Transactions   map[string]string `json:"transactions"` // transaction key -> file path relative to schemaDir
}

func (l *NCPDPSchemaLoader) load() error {
	manifestPath := filepath.Join(l.schemaDir, "manifest.json")
	manifestBytes, err := os.ReadFile(manifestPath)
	if err != nil {
		return fmt.Errorf("ncpdp: reading manifest %s: %w", manifestPath, err)
	}
	var manifest onDiskManifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return fmt.Errorf("ncpdp: parsing manifest %s: %w", manifestPath, err)
	}

	groups := make(map[string]*NCPDPGroupDef, len(manifest.Groups))
	for key, relPath := range manifest.Groups {
		var g NCPDPGroupDef
		if err := l.readJSON(relPath, &g); err != nil {
			return fmt.Errorf("ncpdp: loading group %q: %w", key, err)
		}
		if g.Key == "" {
			g.Key = key
		}
		if g.Key != key {
			return fmt.Errorf("ncpdp: group file %s declares key %q, manifest registers it as %q", relPath, g.Key, key)
		}
		groups[key] = &g
	}

	transactions := make(map[string]*NCPDPTransactionDef, len(manifest.Transactions))
	for key, relPath := range manifest.Transactions {
		var t NCPDPTransactionDef
		if err := l.readJSON(relPath, &t); err != nil {
			return fmt.Errorf("ncpdp: loading transaction %q: %w", key, err)
		}
		if t.Key == "" {
			t.Key = key
		}
		if t.Key != key {
			return fmt.Errorf("ncpdp: transaction file %s declares key %q, manifest registers it as %q", relPath, t.Key, key)
		}
		transactions[key] = &t
	}

	if manifest.HeaderGroupKey != "" {
		if _, ok := groups[manifest.HeaderGroupKey]; !ok {
			return fmt.Errorf("ncpdp: headerGroupKey %q does not resolve against loaded groups", manifest.HeaderGroupKey)
		}
	}

	if err := validateGroupReferences(groups, transactions); err != nil {
		return err
	}

	l.mu.Lock()
	l.spec = &NCPDPSpecDef{
		Version:        manifest.Version,
		MessageAttrs:   manifest.MessageAttrs,
		HeaderGroupKey: manifest.HeaderGroupKey,
		Groups:         groups,
		Transactions:   transactions,
	}
	l.mu.Unlock()
	return nil
}

func (l *NCPDPSchemaLoader) readJSON(relPath string, out interface{}) error {
	fullPath := filepath.Join(l.schemaDir, relPath)
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return fmt.Errorf("reading %s: %w", fullPath, err)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("parsing %s: %w", fullPath, err)
	}
	return nil
}

// validateGroupReferences fail-fasts on any GroupRef.GroupKey that doesn't
// resolve against the loaded group library — the same discipline
// edi.validateSegmentReferences applies to segment IDs, catching a typo'd
// schema reference at load time instead of a confusing nil-pointer panic
// deep in the parser/builder at message-processing time.
func validateGroupReferences(groups map[string]*NCPDPGroupDef, transactions map[string]*NCPDPTransactionDef) error {
	checkRefs := func(source string, refs []NCPDPGroupRef) error {
		for _, ref := range refs {
			if _, ok := groups[ref.GroupKey]; !ok {
				return fmt.Errorf("ncpdp: %s references unknown group %q (via ref key %q)", source, ref.GroupKey, ref.Key)
			}
		}
		return nil
	}
	for gkey, g := range groups {
		if err := checkRefs(fmt.Sprintf("group %q", gkey), g.Groups); err != nil {
			return err
		}
	}
	for tkey, t := range transactions {
		if err := checkRefs(fmt.Sprintf("transaction %q", tkey), t.Groups); err != nil {
			return err
		}
	}
	return nil
}
