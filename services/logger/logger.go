// Package logger provides structured logging for the ezHealthKonnect application.
// Uses Go's standard log/slog (Go 1.21+) — no external dependency required.
//
// Usage:
//
//	logger.Info("step executed",
//	    "pipeline_id", pipelineID,
//	    "step_name", stepName,
//	    "duration_ms", elapsed.Milliseconds(),
//	)
//
// Environment:
//
//	LOG_FORMAT=json   → structured JSON (production, Splunk/Datadog/ELK compatible)
//	LOG_FORMAT=text   → human-readable (development, default)
//	LOG_LEVEL=debug   → debug|info|warn|error (default: info)
package logger

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// L is the application-wide logger. Call Init() before using it.
// It is safe for concurrent use after Init().
var L *slog.Logger

// Init configures the global logger based on environment variables.
// Call once from main() before starting any services.
func Init() {
	level := parseLevel(os.Getenv("LOG_LEVEL"))
	opts := &slog.HandlerOptions{Level: level}

	var handler slog.Handler
	if strings.ToLower(os.Getenv("LOG_FORMAT")) == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	L = slog.New(handler)

	// Also replace the default slog logger so any code using slog.Info() etc. is captured
	slog.SetDefault(L)
}

// parseLevel maps LOG_LEVEL env string to slog.Level. Defaults to Info.
func parseLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// ========================================================
// Convenience wrappers — mirror the slog package API so
// callers don't need to import both packages.
// ========================================================

func Debug(msg string, args ...any) { L.Debug(msg, args...) }
func Info(msg string, args ...any)  { L.Info(msg, args...) }
func Warn(msg string, args ...any)  { L.Warn(msg, args...) }
func Error(msg string, args ...any) { L.Error(msg, args...) }

// With returns a logger with the given attributes pre-attached.
// Use this to create component-scoped loggers, e.g.:
//
//	pipelineLog := logger.With("pipeline_id", id)
//	pipelineLog.Info("starting")
func With(args ...any) *slog.Logger { return L.With(args...) }

// ========================================================
// Context-aware logging — carries trace IDs, request IDs
// ========================================================

type contextKey string

const loggerKey contextKey = "logger"

// WithContext stores a logger in the context (e.g., with a trace_id attached).
func WithContext(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, l)
}

// FromContext retrieves the logger from context, or returns the global logger.
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(loggerKey).(*slog.Logger); ok && l != nil {
		return l
	}
	return L
}
