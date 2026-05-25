package enrichment

import (
	"context"
	"ezhealthkonnect/models"
	"fmt"
	"strings"
)

// ===============================================================
// FIXED-WIDTH FORMAT PARSER
// ===============================================================

// FixedWidthFormatParser handles positional/fixed-width files (e.g., CCLF, NACHA, X12).
// Column positions are defined via config.Columns (name, start, length).
type FixedWidthFormatParser struct{}

func init() { RegisterFormatParser(&FixedWidthFormatParser{}) }

func (p *FixedWidthFormatParser) Format() string      { return "fixed_width" }
func (p *FixedWidthFormatParser) Extensions() []string { return []string{".dat", ".txt", ".fix"} }
func (p *FixedWidthFormatParser) MagicBytes() [][]byte { return nil }

func (p *FixedWidthFormatParser) Parse(
	ctx context.Context,
	data []byte,
	config *models.FileParserConfig,
) ([]map[string]interface{}, []string, error) {
	if len(config.Columns) == 0 {
		return nil, nil, fmt.Errorf("fixed_width format requires column definitions")
	}

	// Build column name list (for return value)
	columns := make([]string, len(config.Columns))
	for i, col := range config.Columns {
		columns[i] = col.Name
	}

	lines := splitLines(string(data))

	// Skip header and SkipRows lines
	startLine := config.SkipRows
	if config.HasHeader {
		startLine++
	}
	if startLine >= len(lines) {
		return []map[string]interface{}{}, columns, nil
	}
	dataLines := lines[startLine:]

	records := make([]map[string]interface{}, 0, len(dataLines))
	dataIdx := 0

	for _, line := range dataLines {
		if ctx.Err() != nil {
			break
		}
		if strings.TrimSpace(line) == "" {
			continue // skip blank lines
		}

		record := make(map[string]interface{}, len(config.Columns))
		lineRunes := []rune(line)

		for _, col := range config.Columns {
			startIdx := col.Start - 1 // convert 1-based to 0-based
			if startIdx < 0 {
				startIdx = 0
			}
			endIdx := startIdx + col.Length

			var value string
			if startIdx < len(lineRunes) {
				if endIdx > len(lineRunes) {
					endIdx = len(lineRunes)
				}
				value = string(lineRunes[startIdx:endIdx])
			}
			if config.TrimFields {
				value = strings.TrimSpace(value)
			}
			record[col.Name] = value
		}

		records = append(records, record)
		dataIdx++

		if config.MaxRecords > 0 && dataIdx >= config.MaxRecords {
			break
		}
	}

	return records, columns, nil
}

// ===============================================================
// PACKAGE-LEVEL HELPERS
// ===============================================================

// splitLines splits content into lines, normalising \r\n and \r to \n.
// Used by FixedWidthFormatParser and the auto-detector.
func splitLines(content string) []string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	lines := strings.Split(content, "\n")
	// Remove trailing empty line if content ended with a newline
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
