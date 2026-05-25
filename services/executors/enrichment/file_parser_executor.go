package enrichment

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"ezhealthkonnect/models"
	"ezhealthkonnect/services/executors"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
)

// ===============================================================
// FILE PARSER EXECUTOR  (orchestrator — format logic lives in *_parser.go)
// ===============================================================

const (
	defaultMaxFileSizeMB = 100 // default limit when MaxFileSizeMB == 0
	hardCapFileSizeMB    = 500 // absolute maximum regardless of config
)

// FileParserExecutor parses structured files (CSV, TSV, fixed-width, Excel, Avro, Parquet)
// into an array of structured records for downstream pipeline steps.
// Format parsers are registered via the FormatParser interface (Strategy Pattern).
// Implements Strategy Pattern — concrete strategy for file parsing.
//
// Source types:
//   field         — raw file content is already in a pipeline field (default)
//   local_path    — read from the server's local filesystem (or container volume mount)
//   field_as_path — a pipeline field holds a URI; executor resolves it:
//                     s3://bucket/key  → AWS S3 (credentials from interface connectivity config or IAM role)
//                     https://...      → HTTP GET
//                     file:///...      → local filesystem
type FileParserExecutor struct {
	*executors.BaseExecutor
	db            *sql.DB                    // optional — used by field_as_path to look up connectivity credentials
	configDecrypt func([]byte) ([]byte, error) // optional — decrypts encrypted JSONB configs read from DB
}

// NewFileParserExecutor creates a new file parser executor.
// db may be nil; it is only used when sourceType == "field_as_path" and S3 credentials
// need to be resolved from the interface connectivity config.
// configDecrypt may be nil; it is called after reading source_config from the DB to
// unwrap the AES-256-GCM envelope written by CredentialStore.EncryptConfig.
func NewFileParserExecutor(db *sql.DB, configDecrypt func([]byte) ([]byte, error)) *FileParserExecutor {
	metadata := models.ExecutorMetadata{
		Name:        "File Parser",
		Description: "Smart file parser — supports CSV, TSV, fixed-width (CCLF, NACHA, X12), Excel (.xlsx/.xls), Apache Avro, Apache Parquet and OOB healthcare templates",
		Version:     "2.1.0",
		Author:      "ezHealthKonnect",
		Category:    "enrichment",
	}
	base := executors.NewBaseExecutor("file_parser", metadata)
	return &FileParserExecutor{BaseExecutor: base, db: db, configDecrypt: configDecrypt}
}

// Execute parses file content into structured records.
func (e *FileParserExecutor) Execute(
	ctx context.Context,
	step *models.TransformationStep,
	inputData map[string]interface{},
) (map[string]interface{}, error) {
	start := time.Now()

	if err := e.PreExecute(ctx, step); err != nil {
		return inputData, err
	}

	config, err := e.parseConfig(step)
	if err != nil {
		e.PostExecute(ctx, step, err, time.Since(start))
		return inputData, err
	}

	// Streaming path: local CSV/TSV with MaxRecords > 0 → O(chunk) memory, no size gate.
	// This bypasses os.ReadFile entirely — the file descriptor is streamed row-by-row.
	// Use with a Loop step: feed next_offset back into config.offset each iteration.
	if config.SourceType == "local_path" && config.MaxRecords > 0 && !config.BatchMode {
		ext := strings.ToLower(filepath.Ext(config.FilePath))
		isCSVTSV := config.FileFormat == "csv" || config.FileFormat == "tsv" ||
			(config.FileFormat == "" && (ext == ".csv" || ext == ".tsv" || ext == ".tab")) ||
			config.FileFormat == "auto"
		if isCSVTSV {
			return e.executeStreamingLocalCSV(ctx, step, config, inputData, start)
		}
	}

	// Batch mode: list + process all matching local files, then return early
	if config.SourceType == "local_path" && config.BatchMode {
		return e.executeBatch(ctx, step, config, inputData, start)
	}

	// ── Resolve raw file content ─────────────────────────────────
	var rawContent string
	switch config.SourceType {
	case "local_path":
		rawContent, err = e.resolveLocalFile(config.FilePath, config)
		if err != nil {
			e.PostExecute(ctx, step, err, time.Since(start))
			return inputData, err
		}
		// For local paths: use magic bytes to detect binary formats regardless of
		// whatever fileFormat was configured (prevents xlsx being parsed as CSV).
		if len(rawContent) >= 4 {
			if p, ok := GetParserByMagicBytes([]byte(rawContent[:4])); ok {
				if config.FileFormat != p.Format() {
					log.Printf("   🔍 Local path: magic bytes → %s (was '%s')", p.Format(), config.FileFormat)
					config.FileFormat = p.Format()
				}
			} else if config.FileFormat == "" {
				// No binary magic bytes — infer text format from extension
				ext := strings.ToLower(filepath.Ext(config.FilePath))
				if p2, ok2 := GetParserByExtension(ext); ok2 {
					config.FileFormat = p2.Format()
				} else {
					config.FileFormat = "csv"
				}
				log.Printf("   🔍 Local path: inferred format '%s' from extension", config.FileFormat)
			}
		}

	case "field_as_path":
		// Read the URI from the pipeline field, then resolve it remotely.
		// The URI may point to S3, an HTTP endpoint, or a local path.
		rawURI, uriErr := e.resolveSourceContent(config.SourceField, inputData)
		if uriErr != nil {
			e.PostExecute(ctx, step, uriErr, time.Since(start))
			return inputData, fmt.Errorf("field_as_path: failed to read URI from field '%s': %w", config.SourceField, uriErr)
		}
		interfaceID, _ := step.Config["interface_id"].(string)
		rawContent, err = e.resolveRemoteFile(ctx, rawURI, config, interfaceID)
		if err != nil {
			e.PostExecute(ctx, step, err, time.Since(start))
			return inputData, err
		}

	default: // "field" or ""
		rawContent, err = e.resolveSourceContent(config.SourceField, inputData)
		if err != nil {
			e.PostExecute(ctx, step, err, time.Since(start))
			return inputData, fmt.Errorf("failed to resolve source content: %w", err)
		}
	}

	// ── Base64 decode if needed ──────────────────────────────────
	if config.ContentEncoding == "base64" {
		decoded, decErr := base64.StdEncoding.DecodeString(rawContent)
		if decErr != nil {
			e.PostExecute(ctx, step, decErr, time.Since(start))
			return inputData, fmt.Errorf("failed to decode base64 content: %w", decErr)
		}
		rawContent = string(decoded)
	}

	log.Printf("   📄 Raw content length: %d bytes", len(rawContent))

	// ── Auto-detect format if requested ─────────────────────────
	var detectionResult *AutoDetectResult
	if config.AutoDetect || config.FileFormat == "auto" {
		var detectErr error
		detectionResult, detectErr = e.autoDetect(rawContent)
		if detectErr != nil {
			e.PostExecute(ctx, step, detectErr, time.Since(start))
			return inputData, fmt.Errorf("auto-detection failed: %w", detectErr)
		}

		if config.FileFormat == "" || config.FileFormat == "auto" {
			config.FileFormat = detectionResult.Format
		}
		if config.Delimiter == "" && detectionResult.Delimiter != "" {
			config.Delimiter = detectionResult.Delimiter
		}
		if config.AutoDetect {
			config.HasHeader = detectionResult.HasHeader
		}

		log.Printf("   🔍 Auto-detected: format=%s, delimiter=%q, hasHeader=%v",
			detectionResult.Format, detectionResult.Delimiter, detectionResult.HasHeader)
	}

	// ── Parse content via registered format parser ───────────────
	records, columns, parseErr := e.parseContent(rawContent, config)
	if parseErr != nil {
		e.PostExecute(ctx, step, parseErr, time.Since(start))
		return inputData, fmt.Errorf("file parsing failed: %w", parseErr)
	}

	// Apply maxRecords limit (safety net — streaming parsers already stop early)
	if config.MaxRecords > 0 && len(records) > config.MaxRecords {
		records = records[:config.MaxRecords]
		log.Printf("   ✂️  Truncated to %d records (maxRecords limit)", config.MaxRecords)
	}

	log.Printf("✅ [FileParser] Parsed %d records with %d columns from %s format",
		len(records), len(columns), config.FileFormat)

	// Build metadata
	metadata := map[string]interface{}{
		"format":        config.FileFormat,
		"parse_time_ms": time.Since(start).Milliseconds(),
		"skipped_rows":  config.SkipRows,
		"has_header":    config.HasHeader,
		"source_type":   config.SourceType,
	}
	if config.SourceType == "local_path" {
		metadata["file_path"] = config.FilePath
	}
	if detectionResult != nil {
		metadata["auto_detected"] = true
		metadata["detected_format"] = detectionResult.Format
		metadata["detected_delimiter"] = detectionResult.Delimiter
		metadata["detected_has_header"] = detectionResult.HasHeader
	}
	if config.Template != "" {
		metadata["template"] = config.Template
	}

	e.SetStepOutputWithDetails(inputData, map[string]interface{}{
		"record_count": len(records),
		"column_count": len(columns),
		"columns":      columns,
		"records":      records,
	}, metadata)

	e.PostExecute(ctx, step, nil, time.Since(start))
	return inputData, nil
}

// ===============================================================
// GB-SCALE STREAMING PATH
// ===============================================================

// executeStreamingLocalCSV streams a local CSV or TSV file directly without loading
// the full content into memory. Bypasses the file size gate entirely — only the
// current chunk (MaxRecords rows) is held in memory at once.
//
// Design:
//   - Opens the file with os.Open() (O(1) — no ReadFile, no string copy)
//   - Delegates row-by-row reading to ParseCSVFromReaderChunked
//   - Supports chunked iteration via Offset + MaxRecords + has_more + next_offset
//
// Step output:
//
//	records      []map[string]interface{} — up to MaxRecords rows for this chunk
//	columns      []string                  — ordered column names
//	record_count int                       — len(records) for this chunk
//	has_more     bool                      — true if more rows exist after this chunk
//	next_offset  int                       — Offset value to use for the next iteration
//	chunk_info   map                       — diagnostic: chunk_size, offset, format, file_path
//
// Loop step integration (chunked processing of arbitrarily large files):
//
//	iteration 1: file_parser config.offset=0, maxRecords=10000  → has_more=true,  next_offset=10000
//	iteration 2: file_parser config.offset=10000, maxRecords=10000 → has_more=true, next_offset=20000
//	iteration N: ...                                             → has_more=false (done)
func (e *FileParserExecutor) executeStreamingLocalCSV(
	ctx context.Context,
	step *models.TransformationStep,
	config *models.FileParserConfig,
	inputData map[string]interface{},
	start time.Time,
) (map[string]interface{}, error) {
	if config.FilePath == "" {
		err := fmt.Errorf("filePath is required for local_path streaming")
		e.PostExecute(ctx, step, err, time.Since(start))
		return inputData, err
	}

	// Detect effective format from extension when not explicitly set
	effectiveFormat := config.FileFormat
	if effectiveFormat == "" || effectiveFormat == "auto" {
		ext := strings.ToLower(filepath.Ext(config.FilePath))
		switch ext {
		case ".tsv", ".tab":
			effectiveFormat = "tsv"
		default:
			effectiveFormat = "csv"
		}
	}

	// Shallow copy so format/delimiter adjustments don't mutate the caller's config
	streamConfig := *config
	streamConfig.FileFormat = effectiveFormat
	if effectiveFormat == "tsv" && streamConfig.Delimiter == "" {
		streamConfig.Delimiter = "\t"
	}

	// Open the file descriptor — OS pages only what the csv.Reader requests
	// No size gate: with MaxRecords > 0, memory use is bounded by chunk size
	f, err := os.Open(config.FilePath)
	if err != nil {
		err = fmt.Errorf("cannot open file '%s': %w", config.FilePath, err)
		e.PostExecute(ctx, step, err, time.Since(start))
		return inputData, err
	}
	defer f.Close()

	log.Printf("   📂 [StreamingCSV] '%s' | format=%s offset=%d maxRecords=%d",
		filepath.Base(config.FilePath), effectiveFormat, streamConfig.Offset, streamConfig.MaxRecords)

	records, columns, hasMore, parseErr := ParseCSVFromReaderChunked(ctx, f, &streamConfig)
	if parseErr != nil {
		e.PostExecute(ctx, step, parseErr, time.Since(start))
		return inputData, fmt.Errorf("streaming CSV parse error: %w", parseErr)
	}

	nextOffset := streamConfig.Offset + len(records)

	log.Printf("✅ [StreamingCSV] chunk: %d records (offset %d→%d) has_more=%v parse_time=%dms",
		len(records), streamConfig.Offset, nextOffset, hasMore, time.Since(start).Milliseconds())

	chunkInfo := map[string]interface{}{
		"chunk_size":  streamConfig.MaxRecords,
		"offset":      streamConfig.Offset,
		"next_offset": nextOffset,
		"format":      effectiveFormat,
		"file_name":   filepath.Base(config.FilePath),
		"file_path":   config.FilePath,
	}

	e.SetStepOutputWithDetails(inputData, map[string]interface{}{
		"record_count": len(records),
		"column_count": len(columns),
		"columns":      columns,
		"records":      records,
		"has_more":     hasMore,
		"next_offset":  nextOffset,
		"chunk_info":   chunkInfo,
	}, map[string]interface{}{
		"source_type":   "local_path",
		"streaming":     true,
		"format":        effectiveFormat,
		"offset":        streamConfig.Offset,
		"max_records":   streamConfig.MaxRecords,
		"has_more":      hasMore,
		"next_offset":   nextOffset,
		"parse_time_ms": time.Since(start).Milliseconds(),
	})

	e.PostExecute(ctx, step, nil, time.Since(start))
	return inputData, nil
}

// parseContent delegates to the registered FormatParser for the given format.
func (e *FileParserExecutor) parseContent(rawContent string, config *models.FileParserConfig) ([]map[string]interface{}, []string, error) {
	parser, ok := GetFormatParser(config.FileFormat)
	if !ok {
		registered := GetRegisteredFormats()
		return nil, nil, fmt.Errorf("unsupported file format: %s (registered: %s)", config.FileFormat, strings.Join(registered, ", "))
	}
	return parser.Parse(context.Background(), []byte(rawContent), config)
}

// resolveLocalFile reads a file from the server local filesystem with a size gate.
// os.Stat is called BEFORE os.ReadFile to reject oversized files without reading them.
func (e *FileParserExecutor) resolveLocalFile(path string, config *models.FileParserConfig) (string, error) {
	if path == "" {
		return "", fmt.Errorf("filePath is required when sourceType is 'local_path'")
	}

	// Determine effective size limit
	maxMB := config.MaxFileSizeMB
	if maxMB <= 0 {
		maxMB = defaultMaxFileSizeMB
	}
	if maxMB > hardCapFileSizeMB {
		maxMB = hardCapFileSizeMB
	}
	maxBytes := int64(maxMB) * 1024 * 1024

	// Stat BEFORE reading — fast rejection for huge files
	info, statErr := os.Stat(path)
	if statErr != nil {
		return "", fmt.Errorf("cannot access file '%s': %w", path, statErr)
	}
	if info.Size() > maxBytes {
		sizeMB := float64(info.Size()) / (1024 * 1024)
		return "", fmt.Errorf(
			"file '%s' is %.1f MB, exceeds maxFileSizeMB limit of %d MB — "+
				"increase maxFileSizeMB in step config or set maxRecords to sample the file",
			filepath.Base(path), sizeMB, maxMB,
		)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("cannot read file '%s': %w", path, err)
	}
	return string(data), nil
}

// resolveLocalPathBatch expands a glob pattern or directory+filePattern into a
// sorted list of regular file paths. Returns an error if no files match.
func (e *FileParserExecutor) resolveLocalPathBatch(config *models.FileParserConfig) ([]string, error) {
	pattern := config.FilePath
	if config.FilePattern != "" {
		pattern = filepath.Join(config.FilePath, config.FilePattern)
	}
	if pattern == "" {
		return nil, fmt.Errorf("filePath is required for batch mode")
	}

	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid glob pattern '%s': %w", pattern, err)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no files found matching pattern '%s'", pattern)
	}

	var files []string
	for _, m := range matches {
		info, statErr := os.Stat(m)
		if statErr == nil && !info.IsDir() {
			files = append(files, m)
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no regular files found matching pattern '%s' (all matches are directories)", pattern)
	}

	sort.Strings(files)
	return files, nil
}

// columnsMatch returns true if slices a and b are identical (same length, same order).
func columnsMatch(a, b []string) bool {
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

// executeBatch processes all files matched by the configured glob pattern and
// merges their records into a single flat output.
func (e *FileParserExecutor) executeBatch(
	ctx context.Context,
	step *models.TransformationStep,
	config *models.FileParserConfig,
	inputData map[string]interface{},
	start time.Time,
) (map[string]interface{}, error) {

	files, listErr := e.resolveLocalPathBatch(config)
	if listErr != nil {
		e.PostExecute(ctx, step, listErr, time.Since(start))
		return inputData, listErr
	}
	log.Printf("   📂 Batch mode: %d files matched", len(files))

	var allRecords []map[string]interface{}
	var referenceColumns []string
	filesProcessed := 0

	for _, filePath := range files {
		content, readErr := e.resolveLocalFile(filePath, config)
		if readErr != nil {
			batchErr := fmt.Errorf("batch: failed to read '%s': %w", filepath.Base(filePath), readErr)
			e.PostExecute(ctx, step, batchErr, time.Since(start))
			return inputData, batchErr
		}

		// Base64 decode if configured
		if config.ContentEncoding == "base64" {
			decoded, decErr := base64.StdEncoding.DecodeString(content)
			if decErr != nil {
				batchErr := fmt.Errorf("batch: failed to decode base64 in '%s': %w", filepath.Base(filePath), decErr)
				e.PostExecute(ctx, step, batchErr, time.Since(start))
				return inputData, batchErr
			}
			content = string(decoded)
		}

		// Per-file auto-detection (shallow copy so settings don't bleed between files)
		effectiveConfig := *config
		if config.AutoDetect || config.FileFormat == "auto" {
			if detResult, detectErr := e.autoDetect(content); detectErr == nil {
				if effectiveConfig.FileFormat == "" || effectiveConfig.FileFormat == "auto" {
					effectiveConfig.FileFormat = detResult.Format
				}
				if effectiveConfig.Delimiter == "" && detResult.Delimiter != "" {
					effectiveConfig.Delimiter = detResult.Delimiter
				}
				if config.AutoDetect {
					effectiveConfig.HasHeader = detResult.HasHeader
				}
			}
		} else {
			// Apply magic-byte detection per file in batch mode too
			if len(content) >= 4 {
				if p, ok := GetParserByMagicBytes([]byte(content[:4])); ok {
					effectiveConfig.FileFormat = p.Format()
				}
			}
		}

		records, columns, parseErr := e.parseContent(content, &effectiveConfig)
		if parseErr != nil {
			batchErr := fmt.Errorf("batch: failed to parse '%s': %w", filepath.Base(filePath), parseErr)
			e.PostExecute(ctx, step, batchErr, time.Since(start))
			return inputData, batchErr
		}

		// Column alignment check — first file sets the reference
		if referenceColumns == nil {
			referenceColumns = columns
		} else if !columnsMatch(referenceColumns, columns) {
			batchErr := fmt.Errorf(
				"batch: column structure mismatch in file '%s' — expected columns %v but got %v",
				filepath.Base(filePath), referenceColumns, columns,
			)
			e.PostExecute(ctx, step, batchErr, time.Since(start))
			return inputData, batchErr
		}

		// Inject _source_file column into each record
		baseName := filepath.Base(filePath)
		for _, rec := range records {
			rec["_source_file"] = baseName
			allRecords = append(allRecords, rec)
		}
		filesProcessed++
		log.Printf("   ✅ Batch: '%s' — %d records", baseName, len(records))
	}

	// Apply maxRecords cap across merged result
	if config.MaxRecords > 0 && len(allRecords) > config.MaxRecords {
		allRecords = allRecords[:config.MaxRecords]
		log.Printf("   ✂️  Batch truncated to %d records (maxRecords limit)", config.MaxRecords)
	}

	log.Printf("✅ [FileParser] Batch: %d files processed, %d total records merged", filesProcessed, len(allRecords))

	sourceFileNames := make([]string, len(files))
	for i, f := range files {
		sourceFileNames[i] = filepath.Base(f)
	}

	e.SetStepOutputWithDetails(inputData, map[string]interface{}{
		"record_count":    len(allRecords),
		"column_count":    len(referenceColumns),
		"columns":         referenceColumns,
		"records":         allRecords,
		"files_processed": filesProcessed,
		"source_files":    sourceFileNames,
	}, map[string]interface{}{
		"source_type":     "local_path",
		"batch_mode":      true,
		"files_matched":   len(files),
		"files_processed": filesProcessed,
		"parse_time_ms":   time.Since(start).Milliseconds(),
	})

	e.PostExecute(ctx, step, nil, time.Since(start))
	return inputData, nil
}

// resolveSourceContent extracts the raw file content from the pipeline input data.
func (e *FileParserExecutor) resolveSourceContent(sourceField string, inputData map[string]interface{}) (string, error) {
	if sourceField == "" {
		return "", fmt.Errorf("sourceField is required")
	}

	value := executors.GetNestedValue(inputData, sourceField)
	if value == nil {
		if v, ok := inputData[sourceField]; ok {
			value = v
		}
	}
	if value == nil {
		return "", fmt.Errorf("source field '%s' not found in input data", sourceField)
	}

	switch v := value.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	default:
		return fmt.Sprintf("%v", v), nil
	}
}

// parseConfig parses and validates the step configuration.
func (e *FileParserExecutor) parseConfig(step *models.TransformationStep) (*models.FileParserConfig, error) {
	if step.Config == nil {
		return nil, fmt.Errorf("file parser configuration is required")
	}

	configJSON, err := json.Marshal(step.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}

	var config models.FileParserConfig
	if err := json.Unmarshal(configJSON, &config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate source configuration
	switch config.SourceType {
	case "local_path":
		if config.FilePath == "" {
			return nil, fmt.Errorf("filePath is required when sourceType is 'local_path'")
		}
	case "field_as_path":
		if config.SourceField == "" {
			return nil, fmt.Errorf("sourceField is required when sourceType is 'field_as_path' — set it to the pipeline field that holds the file URI")
		}
	default: // "field" or ""
		if config.SourceField == "" {
			return nil, fmt.Errorf("sourceField is required (or set sourceType='local_path' and provide filePath, or sourceType='field_as_path' for URI-based resolution)")
		}
	}

	if config.FileFormat == "" && !config.AutoDetect && !config.BatchMode {
		return nil, fmt.Errorf("fileFormat is required (use 'auto' for auto-detection, or one of: %s)", strings.Join(GetRegisteredFormats(), ", "))
	}

	if config.AutoDetect && config.FileFormat == "" {
		config.FileFormat = "auto"
	}

	// Resolve template (loads column definitions from registry)
	if config.Template != "" {
		tmpl, ok := GetTemplate(config.Template)
		if !ok {
			return nil, fmt.Errorf("unknown template '%s' — use GetTemplateList() to see available templates", config.Template)
		}
		if config.FileFormat == "" || config.FileFormat == "auto" {
			config.FileFormat = tmpl.Format
		}
		if len(config.Columns) == 0 {
			config.Columns = tmpl.Columns
		}
		if tmpl.SkipRows > 0 && config.SkipRows == 0 {
			config.SkipRows = tmpl.SkipRows
		}
		log.Printf("   📋 Template '%s' loaded: %s (%d columns)", config.Template, tmpl.Name, len(tmpl.Columns))
	}

	// Validate format against registry (skip for "auto")
	if config.FileFormat != "auto" && config.FileFormat != "" {
		if _, ok := GetFormatParser(config.FileFormat); !ok {
			return nil, fmt.Errorf("unsupported fileFormat '%s' (supported: auto, %s)",
				config.FileFormat, strings.Join(GetRegisteredFormats(), ", "))
		}
	}

	// Validate fixed_width column definitions
	if config.FileFormat == "fixed_width" && len(config.Columns) == 0 && config.Template == "" {
		return nil, fmt.Errorf("fixed_width format requires at least one column definition (or use a template)")
	}
	for i, col := range config.Columns {
		if col.Name == "" {
			return nil, fmt.Errorf("column %d: name is required", i+1)
		}
		if col.Start < 1 {
			return nil, fmt.Errorf("column '%s': start position must be >= 1", col.Name)
		}
		if col.Length < 1 {
			return nil, fmt.Errorf("column '%s': length must be >= 1", col.Name)
		}
	}

	// Apply defaults
	if config.Encoding == "" {
		config.Encoding = "utf-8"
	}

	return &config, nil
}

// ===============================================================
// AUTO-DETECTION
// ===============================================================

// AutoDetectResult holds the results of format auto-detection.
type AutoDetectResult struct {
	Format    string `json:"format"`
	Delimiter string `json:"delimiter,omitempty"`
	HasHeader bool   `json:"hasHeader"`
}

// autoDetect analyzes content to determine file format, delimiter, and header presence.
func (e *FileParserExecutor) autoDetect(content string) (*AutoDetectResult, error) {
	if len(content) == 0 {
		return nil, fmt.Errorf("cannot auto-detect empty content")
	}

	// 1. Binary format detection via registered magic bytes
	if len(content) >= 4 {
		if p, ok := GetParserByMagicBytes([]byte(content[:4])); ok {
			return &AutoDetectResult{Format: p.Format(), HasHeader: true}, nil
		}
	}

	// 2. Text format detection — analyse first 20 lines
	lines := splitLines(content)
	if len(lines) == 0 {
		return &AutoDetectResult{Format: "csv", Delimiter: ",", HasHeader: false}, nil
	}

	sampleSize := len(lines)
	if sampleSize > 20 {
		sampleSize = 20
	}
	sampleLines := lines[:sampleSize]

	// 2a. Delimiter detection
	candidates := []rune{',', '\t', '|', ';'}
	bestDelim := ','
	bestScore := -1.0

	for _, delim := range candidates {
		counts := make([]int, len(sampleLines))
		for i, line := range sampleLines {
			counts[i] = strings.Count(line, string(delim))
		}
		sum := 0
		for _, c := range counts {
			sum += c
		}
		if sum == 0 {
			continue
		}
		avg := float64(sum) / float64(len(counts))
		if avg < 1.0 {
			continue
		}
		variance := 0.0
		for _, c := range counts {
			diff := float64(c) - avg
			variance += diff * diff
		}
		variance /= float64(len(counts))
		stdDev := math.Sqrt(variance)
		consistency := 1.0 / (1.0 + stdDev)
		score := avg * consistency
		if score > bestScore {
			bestScore = score
			bestDelim = delim
		}
	}

	// 2b. Fixed-width detection — no consistent delimiter + consistent line lengths
	if bestScore < 1.0 {
		lengths := make([]int, len(sampleLines))
		for i, line := range sampleLines {
			lengths[i] = len([]rune(line))
		}
		if len(lengths) >= 2 {
			sumLen := 0
			for _, l := range lengths {
				sumLen += l
			}
			avgLen := float64(sumLen) / float64(len(lengths))
			allConsistent := true
			for _, l := range lengths {
				if math.Abs(float64(l)-avgLen) > 2.0 {
					allConsistent = false
					break
				}
			}
			if allConsistent && avgLen > 10 {
				hasHeader := e.detectHeader(sampleLines, "")
				return &AutoDetectResult{Format: "fixed_width", HasHeader: hasHeader}, nil
			}
		}
	}

	// 2c. Build result from best delimiter
	result := &AutoDetectResult{}
	switch bestDelim {
	case '\t':
		result.Format = "tsv"
		result.Delimiter = "\t"
	case ',':
		result.Format = "csv"
		result.Delimiter = ","
	default:
		result.Format = "csv"
		result.Delimiter = string(bestDelim)
	}

	result.HasHeader = e.detectHeader(sampleLines, result.Delimiter)
	log.Printf("   🔍 Auto-detected: format=%s, delimiter=%q, hasHeader=%v", result.Format, result.Delimiter, result.HasHeader)
	return result, nil
}

// detectHeader checks if the first line looks like a header row.
func (e *FileParserExecutor) detectHeader(lines []string, delimiter string) bool {
	if len(lines) < 2 {
		return false
	}

	firstLine := lines[0]
	var firstFields []string
	if delimiter != "" {
		firstFields = strings.Split(firstLine, delimiter)
	} else {
		firstFields = []string{firstLine}
	}

	allAlpha := true
	for _, field := range firstFields {
		field = strings.TrimSpace(field)
		if field == "" {
			continue
		}
		hasLetter := false
		for _, r := range field {
			if unicode.IsLetter(r) || r == '_' || r == '-' {
				hasLetter = true
				break
			}
		}
		if !hasLetter {
			allAlpha = false
			break
		}
	}
	if !allAlpha {
		return false
	}

	if delimiter != "" && len(lines) >= 2 {
		dataLine := lines[1]
		dataFields := strings.Split(dataLine, delimiter)
		hasNumeric := false
		for _, field := range dataFields {
			field = strings.TrimSpace(field)
			for _, r := range field {
				if unicode.IsDigit(r) {
					hasNumeric = true
					break
				}
			}
			if hasNumeric {
				break
			}
		}
		if hasNumeric {
			return true
		}
	}

	return allAlpha
}

// ===============================================================
// VALIDATION & SCHEMA
// ===============================================================

// Validate checks if the step configuration is valid.
func (e *FileParserExecutor) Validate(step *models.TransformationStep) error {
	_, err := e.parseConfig(step)
	return err
}

// GetConfigSchema returns the JSON schema for configuration.
func (e *FileParserExecutor) GetConfigSchema() map[string]interface{} {
	formats := append([]string{"auto"}, GetRegisteredFormats()...)
	return map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"sourceField":     map[string]interface{}{"type": "string", "description": "Pipeline field containing raw file content"},
			"sourceType":      map[string]interface{}{"type": "string", "enum": []string{"field", "local_path"}, "description": "Source type: 'field' (pipeline variable) or 'local_path' (server filesystem)"},
			"filePath":        map[string]interface{}{"type": "string", "description": "Absolute file path or glob (when sourceType=local_path)"},
			"fileFormat":      map[string]interface{}{"type": "string", "enum": formats, "description": "File format (use 'auto' for automatic detection)"},
			"delimiter":       map[string]interface{}{"type": "string", "description": "Field delimiter (CSV/TSV; default: , for CSV, \\t for TSV)"},
			"hasHeader":       map[string]interface{}{"type": "boolean", "description": "Whether first row contains column names"},
			"maxRecords":    map[string]interface{}{"type": "integer", "description": "Chunk size: stop after N records (0 = unlimited). When > 0 with local_path CSV/TSV, enables GB-scale streaming — no size gate, O(N) memory"},
			"offset":        map[string]interface{}{"type": "integer", "description": "Skip N data rows before collecting (used with maxRecords for chunked Loop iteration). Inject next_offset from previous step_output here."},
			"maxFileSizeMB": map[string]interface{}{"type": "integer", "description": "File size gate in MB (0 = default 100 MB, hard cap 500 MB). Ignored when maxRecords > 0 and format is CSV/TSV (streaming path bypasses the gate)."},
			"trimFields":      map[string]interface{}{"type": "boolean", "description": "Trim leading/trailing whitespace from values"},
			"skipRows":        map[string]interface{}{"type": "integer", "description": "Number of rows to skip from top"},
			"sheetName":       map[string]interface{}{"type": "string", "description": "xlsx/xls: sheet name to parse (default: first sheet)"},
			"sheetIndex":      map[string]interface{}{"type": "integer", "description": "xlsx/xls: 0-based sheet index (used if sheetName is empty)"},
			"contentEncoding": map[string]interface{}{"type": "string", "enum": []string{"", "base64"}, "description": "Content encoding (use 'base64' for binary content passed through JSON)"},
			"autoDetect":      map[string]interface{}{"type": "boolean", "description": "Automatically detect format, delimiter, and header"},
			"template":        map[string]interface{}{"type": "string", "description": "OOB template for fixed-width formats (e.g. 'cclf1', 'nacha_entry')"},
			"batchMode":       map[string]interface{}{"type": "boolean", "description": "Process all files matching filePath glob pattern"},
			"filePattern":     map[string]interface{}{"type": "string", "description": "Sub-glob when filePath is a directory (e.g. '*.csv')"},
			"columns": map[string]interface{}{
				"type":        "array",
				"description": "Column definitions for fixed-width format",
				"items": map[string]interface{}{
					"type":     "object",
					"required": []string{"name", "start", "length"},
					"properties": map[string]interface{}{
						"name":   map[string]interface{}{"type": "string"},
						"start":  map[string]interface{}{"type": "integer", "description": "1-based start position"},
						"length": map[string]interface{}{"type": "integer"},
					},
				},
			},
		},
		"required": []string{"sourceField"},
	}
}

// GetConfigExample returns an example configuration.
func (e *FileParserExecutor) GetConfigExample() map[string]interface{} {
	return map[string]interface{}{
		"sourceField": "enriched.connector_result.content",
		"fileFormat":  "auto",
		"trimFields":  true,
	}
}

// ===============================================================
// VARIABLE PROVIDER INTERFACE
// ===============================================================

// GetOutputVariables returns the list of variables this executor will produce.
func (e *FileParserExecutor) GetOutputVariables(step *models.TransformationStep) []models.VariableDefinition {
	variables := []models.VariableDefinition{}

	config, err := e.parseConfig(step)
	if err != nil {
		log.Printf("⚠️  [FileParser] Failed to parse config for variable discovery: %v", err)
		return variables
	}

	basePath := "enriched.file_parser"

	variables = append(variables,
		e.BuildVariableDefinition("record_count", basePath, "Number of records in this chunk",
			executors.WithDataType("number"),
			executors.WithCategory("File Parser"),
		),
		e.BuildVariableDefinition("column_count", basePath, "Number of columns detected",
			executors.WithDataType("number"),
			executors.WithCategory("File Parser"),
		),
		e.BuildVariableDefinition("columns", basePath, "List of column names",
			executors.WithDataType("array"),
			executors.WithCategory("File Parser"),
		),
		e.BuildVariableDefinition("records", basePath, "Array of parsed records (this chunk)",
			executors.WithDataType("array"),
			executors.WithCategory("File Parser"),
		),
	)

	// Streaming/chunked iteration variables (local_path CSV/TSV with maxRecords > 0)
	if config.SourceType == "local_path" && config.MaxRecords > 0 {
		variables = append(variables,
			e.BuildVariableDefinition("has_more", basePath,
				"true if more rows exist past this chunk — use in Loop step condition",
				executors.WithDataType("boolean"),
				executors.WithCategory("File Parser — Chunked"),
			),
			e.BuildVariableDefinition("next_offset", basePath,
				"Row offset to use for the next chunk iteration (offset + record_count)",
				executors.WithDataType("number"),
				executors.WithCategory("File Parser — Chunked"),
			),
			e.BuildVariableDefinition("chunk_info", basePath,
				"Diagnostic info: chunk_size, offset, next_offset, format, file_name",
				executors.WithDataType("object"),
				executors.WithCategory("File Parser — Chunked"),
			),
		)
	}

	if config.FileFormat == "xlsx" || config.FileFormat == "xls" {
		variables = append(variables,
			e.BuildVariableDefinition("sheet_name", basePath, "Name of the parsed Excel sheet",
				executors.WithCategory("File Parser"),
			),
		)
	}

	if config.FileFormat == "fixed_width" {
		for _, col := range config.Columns {
			variables = append(variables,
				e.BuildVariableDefinition(col.Name, basePath+".records[*]",
					fmt.Sprintf("Column '%s' (pos %d, len %d)", col.Name, col.Start, col.Length),
					executors.WithCategory("File Parser Columns"),
				),
			)
		}
	}

	return variables
}
