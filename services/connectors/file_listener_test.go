// services/connectors/file_listener_test.go
// Coverage for isFileStable -- the size-stability check added after a real
// incident: a file named "..._tmp.xml" was read at 0 bytes because
// scanDirectory picked it up mid-write, before its writer finished, and the
// resulting empty message failed downstream with "no parser available for
// format: xml".

package connectors

import (
	"os"
	"path/filepath"
	"testing"
)

func newTestFileListener() *FileListenerConnector {
	c := NewFileListenerConnector()
	return c.(*FileListenerConnector)
}

func TestIsFileStable_FirstSight_NotStable(t *testing.T) {
	f := newTestFileListener()
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.xml")
	if err := os.WriteFile(path, []byte("<ClinicalDocument/>"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if f.isFileStable(path) {
		t.Error("a file seen for the first time must never be considered stable")
	}
	if _, tracked := f.pendingSizes[path]; !tracked {
		t.Error("first sighting must record the observed size for the next poll to compare against")
	}
}

func TestIsFileStable_StillGrowing_NotStable(t *testing.T) {
	f := newTestFileListener()
	dir := t.TempDir()
	path := filepath.Join(dir, "growing.xml")

	if err := os.WriteFile(path, []byte("<Clinical"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if f.isFileStable(path) {
		t.Fatal("first poll must never report stable")
	}

	// Simulate the writer appending more content before the next poll.
	if err := os.WriteFile(path, []byte("<ClinicalDocument>...</ClinicalDocument>"), 0644); err != nil {
		t.Fatalf("WriteFile (append): %v", err)
	}
	if f.isFileStable(path) {
		t.Error("size changed since the last poll -- must not be considered stable yet")
	}
}

func TestIsFileStable_SameSizeAcrossPolls_Stable(t *testing.T) {
	f := newTestFileListener()
	dir := t.TempDir()
	path := filepath.Join(dir, "done.xml")
	content := []byte("<ClinicalDocument>complete</ClinicalDocument>")
	if err := os.WriteFile(path, content, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	if f.isFileStable(path) {
		t.Fatal("first poll must never report stable")
	}
	if !f.isFileStable(path) {
		t.Error("same size observed on a second consecutive poll must be considered stable")
	}
	if _, stillTracked := f.pendingSizes[path]; stillTracked {
		t.Error("a confirmed-stable file must be removed from pendingSizes")
	}
}

func TestIsFileStable_StableButZeroBytes_SkippedAndMarkedProcessed(t *testing.T) {
	f := newTestFileListener()
	dir := t.TempDir()
	path := filepath.Join(dir, "medsection_tmp.xml")
	if err := os.WriteFile(path, []byte{}, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	f.isFileStable(path) // first sight
	if f.isFileStable(path) {
		t.Error("a stable-but-empty file must never be handed to processFile")
	}
	if _, marked := f.processedFiles[path]; !marked {
		t.Error("a stable-but-empty file must be marked processed so it isn't re-checked forever")
	}
}

func TestIsFileStable_FileVanished_ReturnsFalseAndClearsPending(t *testing.T) {
	f := newTestFileListener()
	dir := t.TempDir()
	path := filepath.Join(dir, "ephemeral.xml")
	if err := os.WriteFile(path, []byte("data"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	f.isFileStable(path) // record initial size

	if err := os.Remove(path); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if f.isFileStable(path) {
		t.Error("a file that no longer exists can never be stable")
	}
	if _, tracked := f.pendingSizes[path]; tracked {
		t.Error("pendingSizes must be cleared once a file vanishes, so a future same-named file starts fresh")
	}
}
