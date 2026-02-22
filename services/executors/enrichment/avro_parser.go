package enrichment

import (
	"bytes"
	"context"
	"ezhealthkonnect/models"
	"fmt"
	"strings"

	goavro "github.com/linkedin/goavro/v2"
)

// ===============================================================
// AVRO FORMAT PARSER
// ===============================================================

// AvroFormatParser handles Apache Avro Object Container Files (.avro).
// Uses the goavro/v2 library which returns map[string]interface{} natively.
type AvroFormatParser struct{}

func init() { RegisterFormatParser(&AvroFormatParser{}) }

func (p *AvroFormatParser) Format() string      { return "avro" }
func (p *AvroFormatParser) Extensions() []string { return []string{".avro"} }

// MagicBytes: Avro OCF magic "Obj\x01"
func (p *AvroFormatParser) MagicBytes() [][]byte {
	return [][]byte{{'O', 'b', 'j', 0x01}}
}

func (p *AvroFormatParser) Parse(
	ctx context.Context,
	data []byte,
	config *models.FileParserConfig,
) ([]map[string]interface{}, []string, error) {
	ocf, err := goavro.NewOCFReader(bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("avro OCF open error: %w", err)
	}

	var records []map[string]interface{}
	columnSeen := make(map[string]bool)
	var columnSet []string

	rowIdx := 0
	dataIdx := 0

	for ocf.Scan() {
		if ctx.Err() != nil {
			break // context cancelled — return what we have
		}

		datum, readErr := ocf.Read()
		if readErr != nil {
			return nil, nil, fmt.Errorf("avro read error at row %d: %w", rowIdx, readErr)
		}

		// Apply SkipRows
		if rowIdx < config.SkipRows {
			rowIdx++
			continue
		}

		// goavro returns map[string]interface{} for Avro records natively
		m, ok := datum.(map[string]interface{})
		if !ok {
			// Non-record datum (array/map at top level) — wrap it
			m = map[string]interface{}{"value": datum}
		}

		// Build record, flattening Avro union types
		record := make(map[string]interface{}, len(m))
		for k, v := range m {
			record[k] = flattenAvroUnion(v)
			if config.TrimFields {
				if s, ok2 := record[k].(string); ok2 {
					record[k] = strings.TrimSpace(s)
				}
			}
			// Track column order (first row establishes the schema)
			if !columnSeen[k] {
				columnSeen[k] = true
				columnSet = append(columnSet, k)
			}
		}

		records = append(records, record)
		dataIdx++
		rowIdx++

		if config.MaxRecords > 0 && dataIdx >= config.MaxRecords {
			break
		}
	}

	if err := ocf.Err(); err != nil {
		return nil, nil, fmt.Errorf("avro OCF scan error: %w", err)
	}

	if columnSet == nil {
		columnSet = []string{}
	}
	return records, columnSet, nil
}

// flattenAvroUnion unwraps goavro's union representation.
// goavro encodes union values as map[string]interface{}{"TypeName": value}.
// For non-union types (string, int, float, bool, nil) this is a no-op.
func flattenAvroUnion(v interface{}) interface{} {
	if m, ok := v.(map[string]interface{}); ok && len(m) == 1 {
		for _, inner := range m {
			return inner
		}
	}
	return v
}
