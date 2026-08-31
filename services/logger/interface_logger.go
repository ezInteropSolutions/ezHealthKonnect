// Package logger — interface_logger.go
//
// Provides per-interface structured loggers that write JSON to dedicated log
// files organized by interface and date:
// logs/interfaces/{interfaceID}/{yyyy}/{mm}/{dd}/interface.log
//
// Usage:
//
//	ilog := logger.ForInterface("42", "Epic ADT Listener")
//	ilog.Info("message received",
//	    "message_id", msgID,
//	    "message_type", "ADT^A01",
//	    "pipeline_id", pipelineID,
//	)
//
// Log files:
//
//	logs/application/app.log                             ← global application log (stdout + file)
//	logs/interfaces/42/2026/08/31/interface.log           ← interface-specific JSON log (file only)
//
// The date directory is resolved on every write (UTC), so a logger obtained
// once via ForInterface and kept for the life of the process automatically
// rolls into the next day's file at midnight — matching the same
// {interfaceID}/{yyyy}/{mm}/{dd}/... convention already used for per-message
// NDJSON logs in services/storage/object_storage_service.go's logKey.
//
// Both use slog.Logger so all log entries are structured and parseable by
// Splunk, Datadog, ELK, etc.  The interface-level log always uses JSON format
// regardless of LOG_FORMAT env (ensures machine parseability per interface).
package logger

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// InterfaceLogRegistry is a process-level registry of per-interface loggers.
// Each interface gets one slog.Logger backed by a dailyInterfaceWriter.
// Loggers are created on first use and reused for the lifetime of the process.
var interfaceRegistry = &interfaceLogRegistry{
	loggers: make(map[string]*slog.Logger),
	writers: make(map[string]*dailyInterfaceWriter),
}

type interfaceLogRegistry struct {
	mu      sync.RWMutex
	loggers map[string]*slog.Logger            // key = interfaceID
	writers map[string]*dailyInterfaceWriter    // key = interfaceID, kept to close on shutdown
}

// ForInterface returns a *slog.Logger for the given interface.
// The logger writes structured JSON to
// logs/interfaces/{interfaceID}/{yyyy}/{mm}/{dd}/interface.log, rolling into
// the next day's file automatically. It also pre-attaches interface_id and
// interface_name as permanent fields.
//
// Callers should NOT close this logger — it is managed by the registry.
// Call CloseAllInterfaces() on application shutdown.
func ForInterface(interfaceID, interfaceName string) *slog.Logger {
	// Fast path: already created
	interfaceRegistry.mu.RLock()
	if l, ok := interfaceRegistry.loggers[interfaceID]; ok {
		interfaceRegistry.mu.RUnlock()
		return l
	}
	interfaceRegistry.mu.RUnlock()

	// Slow path: create and register
	interfaceRegistry.mu.Lock()
	defer interfaceRegistry.mu.Unlock()

	// Double-check after acquiring write lock
	if l, ok := interfaceRegistry.loggers[interfaceID]; ok {
		return l
	}

	w := newDailyInterfaceWriter(interfaceID)
	interfaceRegistry.writers[interfaceID] = w

	// Always JSON for interface logs — ensures machine parseability
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: slog.LevelDebug, // Per-interface log captures everything; filter at ingestion layer
	})

	l := slog.New(handler).With(
		"interface_id", interfaceID,
		"interface_name", interfaceName,
	)
	interfaceRegistry.loggers[interfaceID] = l

	return l
}

// CloseAllInterfaces flushes and closes all per-interface log file handles.
// Call from main() via defer after all goroutines have stopped.
func CloseAllInterfaces() {
	interfaceRegistry.mu.Lock()
	defer interfaceRegistry.mu.Unlock()
	for id, w := range interfaceRegistry.writers {
		if err := w.Close(); err != nil {
			L.Warn("failed to close interface log", "interface_id", id, "error", err)
		}
	}
	interfaceRegistry.loggers = make(map[string]*slog.Logger)
	interfaceRegistry.writers = make(map[string]*dailyInterfaceWriter)
}

// dailyInterfaceWriter is an io.WriteCloser that transparently rolls over to
// a new logs/interfaces/{interfaceID}/{yyyy}/{mm}/{dd}/interface.log file
// whenever the UTC date changes, so a *slog.Logger obtained once via
// ForInterface and kept for the life of the process naturally lands each
// day's entries in that day's own file instead of one ever-growing file.
type dailyInterfaceWriter struct {
	interfaceID string

	mu      sync.Mutex
	date    string // yyyy-mm-dd (UTC) the current file was opened for
	file    *os.File
}

func newDailyInterfaceWriter(interfaceID string) *dailyInterfaceWriter {
	return &dailyInterfaceWriter{interfaceID: interfaceID}
}

// Write implements io.Writer. On error opening/rotating the file, it falls
// back to os.Stderr for that write so the logger never silently drops
// messages, without disturbing state for the next call.
func (w *dailyInterfaceWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	today := time.Now().UTC().Format("2006-01-02")
	if w.file == nil || w.date != today {
		if w.file != nil {
			w.file.Close()
			w.file = nil
		}
		f, err := openDatedInterfaceLogFile(w.interfaceID, today)
		if err != nil {
			fmt.Fprintf(os.Stderr, "logger: %v\n", err)
			return os.Stderr.Write(p)
		}
		w.file = f
		w.date = today
	}

	return w.file.Write(p)
}

// Close implements io.Closer.
func (w *dailyInterfaceWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

// openDatedInterfaceLogFile creates the interface's dated log directory and
// opens (or creates) that day's interface.log for appending.
func openDatedInterfaceLogFile(interfaceID, yyyyMMdd string) (*os.File, error) {
	parts := strings.SplitN(yyyyMMdd, "-", 3) // [yyyy, mm, dd]
	dir := filepath.Join("logs", "interfaces", interfaceID, parts[0], parts[1], parts[2])
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create interface log dir %s: %w", dir, err)
	}

	path := filepath.Join(dir, "interface.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open interface log %s: %w", path, err)
	}
	return f, nil
}
