package enrichment

import (
	"context"
	"ezhealthkonnect/models"
	"fmt"
	"sync"
)

// ===============================================================
// FORMAT PARSER STRATEGY INTERFACE
// ===============================================================

// FormatParser is the Strategy interface — one implementation per file format.
// Each concrete parser self-registers via its own init() using RegisterFormatParser().
type FormatParser interface {
	// Parse reads raw file bytes and returns (records, columnNames, error).
	// data is the raw file content, already decoded from base64 if needed.
	Parse(ctx context.Context, data []byte, config *models.FileParserConfig) ([]map[string]interface{}, []string, error)

	// Format returns the canonical format name (e.g. "csv", "xlsx", "avro").
	// Used as the registry key and in error messages.
	Format() string

	// Extensions returns lowercase file extensions this parser handles (e.g. [".csv", ".txt"]).
	// Used for extension-based format inference in resolveLocalFile.
	Extensions() []string

	// MagicBytes returns magic byte sequences identifying this format (nil for text-only formats).
	// Each inner slice is a byte prefix matched at offset 0 of the file content.
	// Used by GetParserByMagicBytes for binary format auto-detection.
	MagicBytes() [][]byte
}

// ===============================================================
// FORMAT PARSER REGISTRY (Singleton)
// ===============================================================

// formatParserRegistry maps format name → FormatParser.
// Thread-safe via RWMutex. Populated at package init time; read-only at runtime.
type formatParserRegistry struct {
	mu      sync.RWMutex
	parsers map[string]FormatParser // format name → parser
	extMap  map[string]FormatParser // ".csv" → parser
}

// globalRegistry is the package-level singleton.
var globalRegistry = &formatParserRegistry{
	parsers: make(map[string]FormatParser),
	extMap:  make(map[string]FormatParser),
}

// RegisterFormatParser adds a parser to the global registry.
// Called from each concrete parser's init() function.
// Panics on duplicate format names (programming error, caught at startup).
func RegisterFormatParser(p FormatParser) {
	globalRegistry.mu.Lock()
	defer globalRegistry.mu.Unlock()

	key := p.Format()
	if _, exists := globalRegistry.parsers[key]; exists {
		panic(fmt.Sprintf("FormatParserRegistry: duplicate registration for format %q", key))
	}
	globalRegistry.parsers[key] = p

	for _, ext := range p.Extensions() {
		globalRegistry.extMap[ext] = p
	}
}

// GetFormatParser retrieves a parser by exact format name.
// Returns (parser, true) if found; (nil, false) otherwise.
func GetFormatParser(format string) (FormatParser, bool) {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()
	p, ok := globalRegistry.parsers[format]
	return p, ok
}

// GetParserByExtension returns the parser registered for a given file extension.
// Extension must be lowercase and include the dot (e.g. ".csv").
// Returns (nil, false) if no parser is registered for that extension.
func GetParserByExtension(ext string) (FormatParser, bool) {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()
	p, ok := globalRegistry.extMap[ext]
	return p, ok
}

// GetRegisteredFormats returns all registered format names.
// Used in error messages and the config schema enum.
func GetRegisteredFormats() []string {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()
	formats := make([]string, 0, len(globalRegistry.parsers))
	for k := range globalRegistry.parsers {
		formats = append(formats, k)
	}
	return formats
}

// GetParserByMagicBytes scans all registered parsers and returns the first one
// whose MagicBytes() prefix matches the beginning of data.
// Returns (nil, false) if no parser claims the magic bytes.
func GetParserByMagicBytes(data []byte) (FormatParser, bool) {
	globalRegistry.mu.RLock()
	defer globalRegistry.mu.RUnlock()
	for _, p := range globalRegistry.parsers {
		for _, magic := range p.MagicBytes() {
			if len(magic) > 0 && len(data) >= len(magic) && magicBytesEqual(data[:len(magic)], magic) {
				return p, true
			}
		}
	}
	return nil, false
}

// magicBytesEqual is a simple byte slice comparison (avoids importing bytes package here).
func magicBytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
