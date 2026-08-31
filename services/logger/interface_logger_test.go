package logger

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// resetInterfaceRegistry clears the package-level registry and any log files
// a test created under logs/interfaces/<interfaceID>, so tests don't leak
// state into each other or the repo.
func resetInterfaceRegistry(t *testing.T, interfaceIDs ...string) {
	t.Helper()
	interfaceRegistry.mu.Lock()
	for _, w := range interfaceRegistry.writers {
		w.Close()
	}
	interfaceRegistry.loggers = make(map[string]*slog.Logger)
	interfaceRegistry.writers = make(map[string]*dailyInterfaceWriter)
	interfaceRegistry.mu.Unlock()

	t.Cleanup(func() {
		for _, id := range interfaceIDs {
			os.RemoveAll(filepath.Join("logs", "interfaces", id))
		}
	})
}

func TestOpenDatedInterfaceLogFile_BuildsYearMonthDayPath(t *testing.T) {
	interfaceID := "test-iface-path"
	t.Cleanup(func() { os.RemoveAll(filepath.Join("logs", "interfaces", interfaceID)) })

	f, err := openDatedInterfaceLogFile(interfaceID, "2026-03-07")
	if err != nil {
		t.Fatalf("openDatedInterfaceLogFile returned error: %v", err)
	}
	defer f.Close()

	wantPath := filepath.Join("logs", "interfaces", interfaceID, "2026", "03", "07", "interface.log")
	if f.Name() != wantPath {
		t.Fatalf("got path %q, want %q", f.Name(), wantPath)
	}
	if _, err := os.Stat(wantPath); err != nil {
		t.Fatalf("expected file to exist on disk: %v", err)
	}
}

func TestOpenDatedInterfaceLogFile_AppendsAcrossCalls(t *testing.T) {
	interfaceID := "test-iface-append"
	t.Cleanup(func() { os.RemoveAll(filepath.Join("logs", "interfaces", interfaceID)) })

	f1, err := openDatedInterfaceLogFile(interfaceID, "2026-03-07")
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	if _, err := f1.WriteString("line one\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	f1.Close()

	f2, err := openDatedInterfaceLogFile(interfaceID, "2026-03-07")
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	if _, err := f2.WriteString("line two\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	f2.Close()

	data, err := os.ReadFile(f2.Name())
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	got := string(data)
	if got != "line one\nline two\n" {
		t.Fatalf("expected both writes to land in the same day's file (append mode), got %q", got)
	}
}

func TestDailyInterfaceWriter_FirstWriteOpensTodaysFile(t *testing.T) {
	interfaceID := "test-iface-firstwrite"
	t.Cleanup(func() { os.RemoveAll(filepath.Join("logs", "interfaces", interfaceID)) })

	w := newDailyInterfaceWriter(interfaceID)
	defer w.Close()

	if _, err := w.Write([]byte("hello\n")); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}

	today := time.Now().UTC().Format("2006-01-02")
	wantDir := filepath.Join("logs", "interfaces", interfaceID,
		today[0:4], today[5:7], today[8:10])
	if _, err := os.Stat(filepath.Join(wantDir, "interface.log")); err != nil {
		t.Fatalf("expected today's dated file to exist: %v", err)
	}
	if w.date != today {
		t.Fatalf("writer.date = %q, want %q", w.date, today)
	}
}

// TestDailyInterfaceWriter_RollsOverWhenDateChanges exercises the same
// `w.date != today` branch a real midnight rollover would hit, by presetting
// a stale date on an already-open writer rather than mocking the clock —
// this codebase has no injected-clock abstraction, and adding one solely for
// this test would be more machinery than the behavior warrants.
func TestDailyInterfaceWriter_RollsOverWhenDateChanges(t *testing.T) {
	interfaceID := "test-iface-rollover"
	t.Cleanup(func() { os.RemoveAll(filepath.Join("logs", "interfaces", interfaceID)) })

	w := newDailyInterfaceWriter(interfaceID)
	defer w.Close()

	if _, err := w.Write([]byte("before rollover\n")); err != nil {
		t.Fatalf("first write: %v", err)
	}
	staleFile := w.file
	w.date = "1999-01-01" // force the next Write to see a mismatch against real "today"

	if _, err := w.Write([]byte("after rollover\n")); err != nil {
		t.Fatalf("second write: %v", err)
	}

	// The writer must have swapped to a new *os.File — writing to the old
	// handle should now fail since dailyInterfaceWriter.Close()d it.
	if _, err := staleFile.WriteString("x"); err == nil {
		t.Fatalf("expected the pre-rollover file handle to be closed")
	}

	today := time.Now().UTC().Format("2006-01-02")
	if w.date != today {
		t.Fatalf("writer.date after rollover = %q, want %q", w.date, today)
	}

	// This test can't actually fake the clock, so the "rollover" still
	// resolves to today's real path — the same file as before. That's the
	// correct outcome here: reopening in append mode must not lose the
	// pre-rollover line. TestOpenDatedInterfaceLogFile_BuildsYearMonthDayPath
	// separately proves the path itself is parameterized by date, which is
	// what actually produces a distinct file on a genuine day change.
	data, err := os.ReadFile(w.file.Name())
	if err != nil {
		t.Fatalf("read current file: %v", err)
	}
	if string(data) != "before rollover\nafter rollover\n" {
		t.Fatalf("expected append-mode reopen to preserve prior content, got %q", string(data))
	}
}

func TestForInterface_ReturnsCachedLoggerForSameID(t *testing.T) {
	interfaceID := "test-iface-cache"
	resetInterfaceRegistry(t, interfaceID)

	l1 := ForInterface(interfaceID, "First Name")
	l2 := ForInterface(interfaceID, "Second Name — should be ignored, first wins")
	if l1 != l2 {
		t.Fatalf("expected ForInterface to return the same cached *slog.Logger for a repeat interfaceID")
	}
}

func TestForInterface_WritesStructuredJSONWithInterfaceFields(t *testing.T) {
	interfaceID := "test-iface-json"
	resetInterfaceRegistry(t, interfaceID)

	l := ForInterface(interfaceID, "Epic ADT Listener")
	l.Info("file queued for processing", "file_name", "ccd.xml")

	today := time.Now().UTC().Format("2006-01-02")
	path := filepath.Join("logs", "interfaces", interfaceID,
		today[0:4], today[5:7], today[8:10], "interface.log")

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected interface.log to exist at %s: %v", path, err)
	}

	var entry map[string]interface{}
	// The file may contain more than one line if other tests in this process
	// used the same registry entry — only decode the first line, which is
	// this test's own write given resetInterfaceRegistry ran first.
	if err := json.Unmarshal(data[:indexOfFirstNewline(data)], &entry); err != nil {
		t.Fatalf("expected valid JSON line, got %q: %v", string(data), err)
	}

	if entry["interface_id"] != interfaceID {
		t.Errorf("interface_id = %v, want %v", entry["interface_id"], interfaceID)
	}
	if entry["interface_name"] != "Epic ADT Listener" {
		t.Errorf("interface_name = %v, want %q", entry["interface_name"], "Epic ADT Listener")
	}
	if entry["file_name"] != "ccd.xml" {
		t.Errorf("file_name = %v, want %q", entry["file_name"], "ccd.xml")
	}
	if entry["msg"] != "file queued for processing" {
		t.Errorf("msg = %v, want %q", entry["msg"], "file queued for processing")
	}
}

func indexOfFirstNewline(b []byte) int {
	for i, c := range b {
		if c == '\n' {
			return i
		}
	}
	return len(b)
}
