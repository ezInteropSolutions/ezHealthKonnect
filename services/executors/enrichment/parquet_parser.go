package enrichment

import (
	"bytes"
	"context"
	"ezhealthkonnect/models"
	"fmt"
	"io"
	"strings"

	parquet "github.com/parquet-go/parquet-go"
)

// ===============================================================
// PARQUET FORMAT PARSER
// ===============================================================

// ParquetFormatParser handles Apache Parquet (.parquet) columnar files.
// Uses parquet-go v0.24 for dynamic schema reading (no pre-defined struct required).
type ParquetFormatParser struct{}

func init() { RegisterFormatParser(&ParquetFormatParser{}) }

func (p *ParquetFormatParser) Format() string      { return "parquet" }
func (p *ParquetFormatParser) Extensions() []string { return []string{".parquet"} }

// MagicBytes: Parquet files start with magic "PAR1"
func (p *ParquetFormatParser) MagicBytes() [][]byte {
	return [][]byte{{'P', 'A', 'R', '1'}}
}

func (p *ParquetFormatParser) Parse(
	ctx context.Context,
	data []byte,
	config *models.FileParserConfig,
) ([]map[string]interface{}, []string, error) {
	// OpenFile requires io.ReaderAt — bytes.NewReader implements it
	file, err := parquet.OpenFile(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, nil, fmt.Errorf("parquet open error: %w", err)
	}

	// Extract leaf column names from schema
	columnNames := extractParquetColumns(file.Root())
	if len(columnNames) == 0 {
		return []map[string]interface{}{}, []string{}, nil
	}

	var records []map[string]interface{}
	rowIdx := 0
	dataIdx := 0

	// Iterate over all row groups in the file
	for _, rg := range file.RowGroups() {
		if ctx.Err() != nil {
			break
		}

		rows := rg.Rows()
		buf := make([]parquet.Row, 64) // read in batches of 64

		for {
			if ctx.Err() != nil {
				break
			}

			n, readErr := rows.ReadRows(buf)
			for i := 0; i < n; i++ {
				// Apply SkipRows
				if rowIdx < config.SkipRows {
					rowIdx++
					continue
				}

				record := parquetRowToMap(buf[i], columnNames, config.TrimFields)
				records = append(records, record)
				dataIdx++
				rowIdx++

				if config.MaxRecords > 0 && dataIdx >= config.MaxRecords {
					goto done
				}
			}

			if readErr != nil {
				if readErr == io.EOF {
					break // end of this row group
				}
				return nil, nil, fmt.Errorf("parquet read error: %w", readErr)
			}
		}
	}

done:
	if records == nil {
		records = []map[string]interface{}{}
	}
	return records, columnNames, nil
}

// extractParquetColumns returns leaf column names from the parquet file schema.
// Leaf columns are the actual data columns (not intermediate group nodes).
func extractParquetColumns(root *parquet.Column) []string {
	var names []string
	var walk func(c *parquet.Column)
	walk = func(c *parquet.Column) {
		if c.Leaf() {
			names = append(names, c.Name())
			return
		}
		for _, child := range c.Columns() {
			walk(child)
		}
	}
	// Root is the message node — walk its children
	for _, child := range root.Columns() {
		walk(child)
	}
	return names
}

// parquetRowToMap converts a parquet.Row ([]parquet.Value) to map[string]interface{}.
// Values are mapped to column names by their column index.
// columnNames must be the leaf column names in index order.
func parquetRowToMap(row parquet.Row, columnNames []string, trim bool) map[string]interface{} {
	record := make(map[string]interface{}, len(columnNames))

	// parquet.Row is []Value; each Value has a Column() index
	for _, v := range row {
		colIdx := v.Column()
		if colIdx < 0 || colIdx >= len(columnNames) {
			continue
		}
		name := columnNames[colIdx]
		val := parquetValueToGo(v)
		if trim {
			if s, ok := val.(string); ok {
				val = strings.TrimSpace(s)
			}
		}
		record[name] = val
	}

	// Ensure all columns present (null if not in row)
	for _, name := range columnNames {
		if _, exists := record[name]; !exists {
			record[name] = nil
		}
	}

	return record
}

// parquetValueToGo converts a parquet.Value to a native Go type.
func parquetValueToGo(v parquet.Value) interface{} {
	if v.IsNull() {
		return nil
	}
	switch v.Kind() {
	case parquet.Boolean:
		return v.Boolean()
	case parquet.Int32:
		return int64(v.Int32())
	case parquet.Int64:
		return v.Int64()
	case parquet.Float:
		return float64(v.Float())
	case parquet.Double:
		return v.Double()
	case parquet.ByteArray, parquet.FixedLenByteArray:
		return string(v.ByteArray())
	default:
		return fmt.Sprintf("%v", v)
	}
}
