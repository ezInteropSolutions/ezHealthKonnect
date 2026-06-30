// services/interface_debug_resolver.go
// Shared resolution of the per-interface debug/log-level flag (interfaces.log_level
// with debug_logging fallback), so the logic isn't duplicated between
// processing.ProcessingEngine.createLogger and other callers (e.g. the
// transformation pipeline, which needs the same flag to gate CDA deep-lineage
// capture without an extra DB round trip per step).
package services

import (
	"context"
	"database/sql"
	"time"
)

// ResolveEffectiveLogLevel computes the effective per-interface log level and a
// convenience debugMode bool from the raw log_level/debug_logging columns.
//
// level is one of: "debug" | "info" | "warning" | "error" | "off".
// debugMode is level == "debug".
func ResolveEffectiveLogLevel(logLevel sql.NullString, debugLogging bool) (level string, debugMode bool) {
	level = logLevel.String
	if level == "" {
		// default: log everything per user preference
		level = "debug"
	}
	debugMode = level == "debug"
	return level, debugMode
}

// ResolveInterfaceDebugLevel reads interfaces.log_level (V59+) with debug_logging
// fallback (pre-V59 schema) and returns the effective level plus a convenience
// debugMode bool (level == "debug"). For callers that only need the level itself
// (not interfaceName) — e.g. gating CDA deep-lineage capture once per pipeline run.
func ResolveInterfaceDebugLevel(db *sql.DB, interfaceID string) (level string, debugMode bool, err error) {
	if db == nil {
		return "", false, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var logLevel sql.NullString
	var debugLogging bool

	query := `SELECT COALESCE(log_level, ''), debug_logging FROM interfaces WHERE id = $1`
	scanErr := db.QueryRowContext(ctx, query, interfaceID).Scan(&logLevel, &debugLogging)
	if scanErr != nil {
		// Fallback: pre-V59 schema without log_level column
		scanErr2 := db.QueryRowContext(ctx, `SELECT debug_logging FROM interfaces WHERE id = $1`,
			interfaceID).Scan(&debugLogging)
		if scanErr2 != nil {
			return "", false, scanErr2
		}
		if !debugLogging {
			return "off", false, nil
		}
		logLevel = sql.NullString{String: "debug", Valid: true}
	}

	level, debugMode = ResolveEffectiveLogLevel(logLevel, debugLogging)
	return level, debugMode, nil
}
