// Package logger — interface_logger.go
//
// Provides per-interface structured loggers that write JSON to dedicated,
// rotated log files at logs/interfaces/{interfaceID}/interface.log
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
//	logs/application/app.log          ← global application log (stdout + file)
//	logs/interfaces/42/interface.log  ← interface-specific JSON log (file only)
//
// Both use slog.Logger so all log entries are structured and parseable by
// Splunk, Datadog, ELK, etc.  The interface-level log always uses JSON format
// regardless of LOG_FORMAT env (ensures machine parseability per interface).
package logger

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// InterfaceLogRegistry is a process-level registry of per-interface loggers.
// Each interface gets one slog.Logger backed by its own rotating file.
// Loggers are created on first use and reused for the lifetime of the process.
var interfaceRegistry = &interfaceLogRegistry{
	loggers: make(map[string]*slog.Logger),
	writers: make(map[string]io.WriteCloser),
}

type interfaceLogRegistry struct {
	mu      sync.RWMutex
	loggers map[string]*slog.Logger  // key = interfaceID
	writers map[string]io.WriteCloser // key = interfaceID, kept to close on shutdown
}

// ForInterface returns a *slog.Logger for the given interface.
// The logger writes structured JSON to logs/interfaces/{interfaceID}/interface.log.
// It also pre-attaches interface_id and interface_name as permanent fields.
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

	w := openInterfaceLogFile(interfaceID)
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
	interfaceRegistry.writers = make(map[string]io.WriteCloser)
}

// openInterfaceLogFile creates the log directory and opens the log file for appending.
// Returns an io.WriteCloser backed by an os.File.
// On error, falls back to os.Stderr so the logger never silently drops messages.
func openInterfaceLogFile(interfaceID string) io.WriteCloser {
	dir := filepath.Join("logs", "interfaces", interfaceID)
	if err := os.MkdirAll(dir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "logger: failed to create interface log dir %s: %v\n", dir, err)
		return os.Stderr
	}

	path := filepath.Join(dir, "interface.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "logger: failed to open interface log %s: %v\n", path, err)
		return os.Stderr
	}
	return f
}
