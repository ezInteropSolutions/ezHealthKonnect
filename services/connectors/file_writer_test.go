// services/connectors/file_writer_test.go
// Coverage for the "file_encoding"/"create_subdirectories" fallback keys added
// to protect against the connectivity_types schema's old (pre-fix) field
// names -- see V222's migration comment. No real saved config currently uses
// either old name (confirmed via direct DB query before the schema fix), so
// these tests exist purely as defense in depth.
package connectors

import (
	"encoding/json"
	"testing"
)

func TestFileWriterInitialize_AcceptsLegacyFileEncodingKey(t *testing.T) {
	dir := t.TempDir()
	f := NewFileWriterConnector().(*FileWriterConnector)
	cfg, _ := json.Marshal(map[string]interface{}{
		"directory_path": dir,
		"file_encoding":  "ISO-8859-1",
	})
	if err := f.Initialize(cfg); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	if f.encoding != "ISO-8859-1" {
		t.Errorf("expected encoding to be set from legacy file_encoding key, got: %s", f.encoding)
	}
}

func TestFileWriterInitialize_CanonicalEncodingKeyTakesPriority(t *testing.T) {
	dir := t.TempDir()
	f := NewFileWriterConnector().(*FileWriterConnector)
	cfg, _ := json.Marshal(map[string]interface{}{
		"directory_path": dir,
		"encoding":       "UTF-16",
		"file_encoding":  "ISO-8859-1",
	})
	if err := f.Initialize(cfg); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	if f.encoding != "UTF-16" {
		t.Errorf("expected canonical 'encoding' key to take priority over legacy 'file_encoding', got: %s", f.encoding)
	}
}

func TestFileWriterInitialize_AcceptsLegacyCreateSubdirectoriesKey(t *testing.T) {
	dir := t.TempDir()
	f := NewFileWriterConnector().(*FileWriterConnector)
	cfg, _ := json.Marshal(map[string]interface{}{
		"directory_path":        dir,
		"create_subdirectories": false,
	})
	if err := f.Initialize(cfg); err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	if f.createSubdirs != false {
		t.Errorf("expected create_subdirs to be set from legacy create_subdirectories key, got: %v", f.createSubdirs)
	}
}
