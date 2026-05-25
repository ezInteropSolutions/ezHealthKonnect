package enrichment

import (
	"context"
	"encoding/csv"
	"errors"
	"ezhealthkonnect/models"
	"fmt"
	"io"
	"strings"
)

// ===============================================================
// CSV FORMAT PARSER
// ===============================================================

// CSVFormatParser handles comma-separated (CSV) files.
// Supports streaming row-by-row parsing when MaxRecords > 0 to avoid OOM on large files.
type CSVFormatParser struct{}

func init() { RegisterFormatParser(&CSVFormatParser{}) }

func (p *CSVFormatParser) Format() string      { return "csv" }
func (p *CSVFormatParser) Extensions() []string { return []string{".csv"} }
func (p *CSVFormatParser) MagicBytes() [][]byte { return nil } // text format — no magic bytes

func (p *CSVFormatParser) Parse(
	ctx context.Context,
	data []byte,
	config *models.FileParserConfig,
) ([]map[string]interface{}, []string, error) {
	delimiter := ','
	if config.Delimiter != "" {
		runes := []rune(config.Delimiter)
		if len(runes) == 1 {
			delimiter = runes[0]
		} else if config.Delimiter == `\t` {
			delimiter = '\t'
		}
	}
	reader := newCSVReader(string(data), rune(delimiter), config.QuoteChar)

	// Streaming path: read row-by-row when MaxRecords > 0 — O(maxRecords) memory
	if config.MaxRecords > 0 {
		return parseCSVStreaming(ctx, reader, config)
	}
	// Full path: ReadAll — fast for small files (already gated by file size check)
	return parseCSVFull(reader, config)
}

// ===============================================================
// TSV FORMAT PARSER
// ===============================================================

// TSVFormatParser handles tab-separated (TSV) files.
// Delegates to CSVFormatParser with delimiter forced to tab.
type TSVFormatParser struct {
	csv CSVFormatParser
}

func init() { RegisterFormatParser(&TSVFormatParser{}) }

func (p *TSVFormatParser) Format() string      { return "tsv" }
func (p *TSVFormatParser) Extensions() []string { return []string{".tsv", ".tab"} }
func (p *TSVFormatParser) MagicBytes() [][]byte { return nil }

func (p *TSVFormatParser) Parse(
	ctx context.Context,
	data []byte,
	config *models.FileParserConfig,
) ([]map[string]interface{}, []string, error) {
	// Force tab delimiter — shallow copy so we don't mutate caller's config
	tsvConfig := *config
	tsvConfig.Delimiter = "\t"
	return p.csv.Parse(ctx, data, &tsvConfig)
}

// ===============================================================
// INTERNAL HELPERS
// ===============================================================

func newCSVReader(content string, delimiter rune, quoteChar string) *csv.Reader {
	reader := csv.NewReader(strings.NewReader(content))
	reader.Comma = delimiter
	reader.FieldsPerRecord = -1
	if quoteChar == "" || quoteChar == `"` {
		reader.LazyQuotes = false
	} else if quoteChar == "none" {
		reader.LazyQuotes = true
	}
	return reader
}

// parseCSVFull reads the entire file into memory at once.
func parseCSVFull(reader *csv.Reader, config *models.FileParserConfig) ([]map[string]interface{}, []string, error) {
	allRows, err := reader.ReadAll()
	if err != nil {
		return nil, nil, fmt.Errorf("CSV parse error: %w", err)
	}
	return buildRecords(allRows, config)
}

// parseCSVStreaming reads row-by-row and stops at MaxRecords to minimise memory use.
func parseCSVStreaming(ctx context.Context, reader *csv.Reader, config *models.FileParserConfig) ([]map[string]interface{}, []string, error) {
	var headers []string
	var records []map[string]interface{}
	headerDone := false
	rowIdx := 0
	dataIdx := 0

	for {
		if ctx.Err() != nil {
			break // context cancelled — return what we have
		}

		row, err := reader.Read()
		if err != nil {
			break // io.EOF or parse error
		}

		// Skip rows from top
		if rowIdx < config.SkipRows {
			rowIdx++
			continue
		}

		// Consume header row
		if config.HasHeader && !headerDone {
			headers = applyTrim(row, config.TrimFields)
			headerDone = true
			rowIdx++
			continue
		}

		// Generate column names on first data row when there is no header
		if !headerDone {
			for i := range row {
				headers = append(headers, fmt.Sprintf("col_%d", i+1))
			}
			headerDone = true
		}

		records = append(records, rowToRecord(row, headers, config.TrimFields))
		dataIdx++
		rowIdx++

		if config.MaxRecords > 0 && dataIdx >= config.MaxRecords {
			break
		}
	}

	if headers == nil {
		headers = []string{}
	}
	return records, headers, nil
}

// ===============================================================
// CHUNKED STREAMING — GB-SCALE FILE PROCESSING
// ===============================================================

// ParseCSVFromReaderChunked reads CSV data row-by-row directly from any io.Reader
// (typically an *os.File opened with os.Open) without loading the full file into memory.
//
// Memory model: O(MaxRecords × fieldsPerRow) — only the current chunk is held in memory.
// The file descriptor advances naturally; no full-file read or string conversion occurs.
//
// Chunked iteration pattern (Loop step):
//
//	chunk 1: Offset=0,     MaxRecords=10000 → records + has_more=true,  next_offset=10000
//	chunk 2: Offset=10000, MaxRecords=10000 → records + has_more=true,  next_offset=20000
//	chunk N: Offset=X,     MaxRecords=10000 → records + has_more=false (EOF reached)
//
// Each call opens a fresh file descriptor and skips Offset data rows (O(Offset) reads).
// For very large offsets (>1M rows) consider the byte-offset optimisation path:
// store the file byte position from a previous call and use os.Seek() to skip instantly.
//
// Parameters:
//
//	ctx    — context for cancellation (safe to break mid-read)
//	r      — io.Reader positioned at the start of the file
//	config — FileParserConfig; reads SkipRows, HasHeader, Delimiter, QuoteChar,
//	         TrimFields, Offset, MaxRecords
//
// Returns:
//
//	records  — up to MaxRecords data rows as []map[string]interface{}
//	columns  — ordered column names (from header row or "col_1", "col_2" …)
//	hasMore  — true if additional data rows exist beyond this chunk
//	err      — non-nil only on a real parse error (io.EOF is treated as normal termination)
func ParseCSVFromReaderChunked(
	ctx context.Context,
	r io.Reader,
	config *models.FileParserConfig,
) (records []map[string]interface{}, columns []string, hasMore bool, err error) {
	delimiter := ','
	if config.Delimiter != "" {
		runes := []rune(config.Delimiter)
		if len(runes) == 1 {
			delimiter = runes[0]
		} else if config.Delimiter == `\t` {
			delimiter = '\t'
		}
	}

	reader := csv.NewReader(r)
	reader.Comma = delimiter
	reader.FieldsPerRecord = -1
	if config.QuoteChar == "" || config.QuoteChar == `"` {
		reader.LazyQuotes = false
	} else if config.QuoteChar == "none" {
		reader.LazyQuotes = true
	}

	var headers []string
	headerDone := false
	rowIdx := 0  // absolute row index from file top (includes SkipRows + header)
	dataIdx := 0 // data row index (post-header, post-skipRows, pre-offset)

	for {
		if ctx.Err() != nil {
			break // context cancelled — return partial results
		}

		row, readErr := reader.Read()
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				err = fmt.Errorf("CSV parse error at row %d: %w", rowIdx+1, readErr)
			}
			break // io.EOF = normal termination
		}

		// Skip top rows (SkipRows is an absolute count from file top, not data rows)
		if rowIdx < config.SkipRows {
			rowIdx++
			continue
		}

		// Consume header row
		if config.HasHeader && !headerDone {
			headers = applyTrim(row, config.TrimFields)
			headerDone = true
			rowIdx++
			continue
		}

		// Auto-generate column names from the first data row when no header
		if !headerDone {
			for i := range row {
				headers = append(headers, fmt.Sprintf("col_%d", i+1))
			}
			headerDone = true
		}

		// Row-based offset: skip the first Offset data rows before collecting
		if dataIdx < config.Offset {
			dataIdx++
			rowIdx++
			continue
		}

		// Chunk boundary: collected MaxRecords rows.
		// Peek at the next row to determine hasMore without consuming it for output.
		if config.MaxRecords > 0 && len(records) >= config.MaxRecords {
			_, peekErr := reader.Read()
			hasMore = (peekErr == nil) // nil means another row exists
			break
		}

		records = append(records, rowToRecord(row, headers, config.TrimFields))
		dataIdx++
		rowIdx++
	}

	if headers == nil {
		headers = []string{}
	}
	columns = headers
	return
}

// ===============================================================
// SHARED HELPERS (used by csv_parser, excel_parser, fixed_width_parser)
// ===============================================================

// buildRecords converts a 2D string slice (from ReadAll or sheet reader) into
// a slice of row maps following SkipRows / HasHeader / TrimFields config.
func buildRecords(allRows [][]string, config *models.FileParserConfig) ([]map[string]interface{}, []string, error) {
	if len(allRows) == 0 {
		return []map[string]interface{}{}, []string{}, nil
	}

	startRow := config.SkipRows
	if startRow >= len(allRows) {
		return []map[string]interface{}{}, []string{}, nil
	}
	rows := allRows[startRow:]

	var headers []string
	dataStart := 0

	if config.HasHeader && len(rows) > 0 {
		headers = applyTrim(rows[0], config.TrimFields)
		dataStart = 1
	} else if len(rows) > 0 {
		for i := range rows[0] {
			headers = append(headers, fmt.Sprintf("col_%d", i+1))
		}
	}

	records := make([]map[string]interface{}, 0, len(rows)-dataStart)
	for _, row := range rows[dataStart:] {
		records = append(records, rowToRecord(row, headers, config.TrimFields))
	}
	return records, headers, nil
}

// applyTrim returns a copy of fields with whitespace trimmed when trim is true.
func applyTrim(fields []string, trim bool) []string {
	if !trim {
		cp := make([]string, len(fields))
		copy(cp, fields)
		return cp
	}
	out := make([]string, len(fields))
	for i, f := range fields {
		out[i] = strings.TrimSpace(f)
	}
	return out
}

// rowToRecord converts a CSV/Excel row (string slice) into a map using headers.
func rowToRecord(row []string, headers []string, trim bool) map[string]interface{} {
	record := make(map[string]interface{}, len(headers))
	for j, header := range headers {
		var value string
		if j < len(row) {
			value = row[j]
		}
		if trim {
			value = strings.TrimSpace(value)
		}
		record[header] = value
	}
	return record
}
